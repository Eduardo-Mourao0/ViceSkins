package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/trade"
)

// TradeProvider acompanha a troca na Steam (ver o pacote trade).
//
// Start pode ser chamado mais de uma vez para a mesma transação (retry depois
// de um timeout, lease vencido), então precisa ser idempotente. Devolve o ID
// da oferta se o provider a enviou (simulado), ou "" no P2P real, em que quem
// envia é o vendedor. Erros com trade.ErrRejected são permanentes.
type TradeProvider interface {
	Start(ctx context.Context, t trade.Trade) (offerID string, err error)
	Check(ctx context.Context, t trade.Trade) (trade.Observation, error)
	Cancel(ctx context.Context, t trade.Trade) error // trade.ErrCannotCancel se não houver como
}

// KeyBox abre a API key do vendedor guardada no banco (*secrets.Box).
type KeyBox interface {
	Seal(plaintext, aad []byte) ([]byte, error)
	Open(sealed, aad []byte) ([]byte, error)
}

type PurchaseStore interface {
	ExecTx(ctx context.Context, fn func(q *repository.Queries) error) error
	GetTransaction(ctx context.Context, id int64) (repository.Transaction, error)
	ListDueHolds(ctx context.Context, arg repository.ListDueHoldsParams) ([]int64, error)
	ListExpiredTransactions(ctx context.Context, arg repository.ListExpiredTransactionsParams) ([]int64, error)
	ClaimTradeOutbox(ctx context.Context, arg repository.ClaimTradeOutboxParams) ([]repository.TradeOutbox, error)
	RescheduleTradeOutbox(ctx context.Context, arg repository.RescheduleTradeOutboxParams) error
	CloseTradeOutbox(ctx context.Context, arg repository.CloseTradeOutboxParams) error

	GetTradeContext(ctx context.Context, id int64) (repository.GetTradeContextRow, error)
	ListTradesToCheck(ctx context.Context, arg repository.ListTradesToCheckParams) ([]int64, error)
	ListClosedTradesToWatch(ctx context.Context, arg repository.ListClosedTradesToWatchParams) ([]int64, error)
	MarkTradeChecked(ctx context.Context, arg repository.MarkTradeCheckedParams) error
	ListDueDisputes(ctx context.Context, arg repository.ListDueDisputesParams) ([]int64, error)
	ListDisputes(ctx context.Context, arg repository.ListDisputesParams) ([]repository.ListDisputesRow, error)
}

type PurchaseConfig struct {
	FeeBps          int64         // taxa da plataforma em basis points (200 = 2%)
	HoldDuration    time.Duration // proteção de troca da Steam (7 dias)
	MaxSendAttempts int           // tentativas de iniciar a troca antes de desistir e estornar

	// Prazos das etapas. Perdeu o prazo → failed, com estorno ao comprador.
	SellerSendTimeout  time.Duration // pending: vendedor enviar a oferta
	BuyerAcceptTimeout time.Duration // sent: comprador aceitar

	CheckInterval time.Duration // cada troca é verificada na Steam no máximo nesse intervalo
	Keys          KeyBox        // nil: API keys de vendedor desligadas
}

const (
	defaultStepTimeout     = 12 * time.Hour
	defaultCheckInterval   = 2 * time.Minute
	defaultMaxSendAttempts = 8                // com o backoff abaixo, ~10 min tentando
	sendTimeout            = 15 * time.Second // limite de uma chamada ao TradeProvider
	outboxLease            = time.Minute      // > sendTimeout: o lease não vence no meio do envio
	retryBase              = 5 * time.Second
	retryMax               = 5 * time.Minute
)

type PurchaseService struct {
	store    PurchaseStore
	provider TradeProvider
	cfg      PurchaseConfig
	now      func() time.Time
}

func NewPurchaseService(store PurchaseStore, provider TradeProvider, cfg PurchaseConfig) *PurchaseService {
	if cfg.MaxSendAttempts <= 0 {
		cfg.MaxSendAttempts = defaultMaxSendAttempts
	}
	if cfg.SellerSendTimeout <= 0 {
		cfg.SellerSendTimeout = defaultStepTimeout
	}
	if cfg.BuyerAcceptTimeout <= 0 {
		cfg.BuyerAcceptTimeout = defaultStepTimeout
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = defaultCheckInterval
	}
	return &PurchaseService{store: store, provider: provider, cfg: cfg, now: time.Now}
}

// Buy compra uma listing. Tudo que mexe em dinheiro e estoque acontece numa
// única transação SQL, junto com o pedido de envio da oferta no outbox.
// O envio em si vem depois, fora dela.
func (s *PurchaseService) Buy(ctx context.Context, buyerID, listingID int64) (domain.Transaction, error) {
	var created repository.Transaction

	err := s.store.ExecTx(ctx, func(q *repository.Queries) error {
		// 1. Trava a listing. Ordem fixa dos locks (listing → comprador) evita deadlock.
		l, err := q.GetListingForUpdate(ctx, listingID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock listing: %w", err)
		}
		if l.SellerID == buyerID {
			return domain.ErrCannotBuyOwnListing
		}
		if l.Status != string(domain.ListingActive) {
			return domain.ErrListingNotActive
		}

		// 2. Trava o saldo do comprador. Duas compras do mesmo comprador ficam em fila.
		buyer, err := q.GetBuyerForUpdate(ctx, buyerID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock buyer: %w", err)
		}
		// Sem trade URL não há para onde mandar a oferta.
		if buyer.TradeURL == nil {
			return domain.ErrTradeURLRequired
		}
		// Com dívida (cobrança de disputa que o saldo não cobriu), não compra.
		if buyer.DebtCents > 0 {
			return domain.ErrBuyerHasDebt
		}
		if buyer.BalanceCents < l.PriceCents {
			return domain.ErrInsufficientBalance
		}

		item, err := q.GetInventoryItem(ctx, l.ItemID)
		if err != nil {
			return fmt.Errorf("get item: %w", err)
		}

		// 3. Listing vendida, transação criada, comprador debitado.
		n, err := q.MarkListingSold(ctx, l.ID)
		if err != nil {
			return fmt.Errorf("mark listing sold: %w", err)
		}
		if n != 1 { // impossível com o FOR UPDATE acima; se acontecer, aborta tudo
			return fmt.Errorf("mark listing sold: %d rows affected", n)
		}
		created, err = q.CreateTransaction(ctx, repository.CreateTransactionParams{
			ListingID:  l.ID,
			BuyerID:    buyerID,
			SellerID:   l.SellerID,
			ItemID:     l.ItemID,
			PriceCents: l.PriceCents,
			FeeCents:   domain.CalcFee(l.PriceCents, s.cfg.FeeBps),
			// Cópia da URL: trocar a URL depois não desvia esta entrega.
			BuyerTradeURL: buyer.TradeURL,
			DeadlineAt:    s.now().Add(s.cfg.SellerSendTimeout),
		})
		if err != nil {
			return fmt.Errorf("create transaction: %w", err)
		}
		if _, err = repository.ApplyWalletEntry(ctx, q, buyerID, -l.PriceCents, string(domain.EntryPurchase),
			&created.ID, "Compra: "+item.Name); err != nil {
			return err
		}

		// 4. Pedido de envio no outbox, no mesmo COMMIT: se a API cair logo
		// depois, o job de outbox envia a oferta quando ela voltar.
		return q.EnqueueTradeOffer(ctx, repository.EnqueueTradeOfferParams{TransactionID: created.ID, Now: s.now()})
	})
	if err != nil {
		return domain.Transaction{}, err
	}

	// 5. Fora da transação SQL (nunca segure locks do banco esperando I/O
	// externo): tenta enviar já, para o comprador não esperar o próximo tick.
	return s.dispatchNow(ctx, created), nil
}

// ---- Envio da oferta (outbox) ----

// dispatchNow tenta enviar a oferta da compra que acabou de ser feita.
//
// A compra já foi gravada quando chegamos aqui, então nunca devolvemos erro:
// se algo falhar, a transação fica pending e o job de outbox tenta de novo.
func (s *PurchaseService) dispatchNow(ctx context.Context, created repository.Transaction) domain.Transaction {
	// WithoutCancel: se o cliente fechar a conexão, o envio não é abortado no
	// meio. O timeout garante que a requisição não fica presa na Steam.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendTimeout)
	defer cancel()

	claimed, err := s.store.ClaimTradeOutbox(ctx, s.claimParams(&created.ID))
	if err != nil {
		slog.Error("claim trade outbox", "tx", created.ID, "err", err)
		return toDomainTransaction(created)
	}
	if len(claimed) == 0 { // o job pegou antes; ele termina o envio
		return toDomainTransaction(created)
	}
	s.dispatch(ctx, claimed[0])

	if tx, err := s.store.GetTransaction(ctx, created.ID); err == nil {
		return toDomainTransaction(tx)
	}
	return toDomainTransaction(created)
}

// DispatchPendingOffers é o job do outbox: envia as ofertas que ainda não
// saíram (API caiu depois da compra, Steam fora do ar, retry agendado).
func (s *PurchaseService) DispatchPendingOffers(ctx context.Context) error {
	// Uma linha por vez: o lease de cada uma só começa quando ela vai ser
	// enviada, então nunca vence enquanto espera na fila do próprio worker.
	for range jobBatchSize {
		claimed, err := s.store.ClaimTradeOutbox(ctx, s.claimParams(nil))
		if err != nil {
			return fmt.Errorf("claim trade outbox: %w", err)
		}
		if len(claimed) == 0 {
			return nil
		}
		s.dispatch(ctx, claimed[0])
	}
	return nil
}

func (s *PurchaseService) claimParams(txID *int64) repository.ClaimTradeOutboxParams {
	now := s.now()
	return repository.ClaimTradeOutboxParams{
		Now:           now,
		LeaseUntil:    now.Add(outboxLease),
		TransactionID: txID,
		MaxRows:       1,
	}
}

// dispatch envia a oferta de uma linha do outbox já reservada (lease).
// Sucesso → sent. Erro permanente ou tentativas esgotadas → failed, com
// estorno. Erro temporário → reagenda com backoff.
func (s *PurchaseService) dispatch(ctx context.Context, row repository.TradeOutbox) {
	tx, err := s.store.GetTransaction(ctx, row.TransactionID)
	if err != nil {
		slog.Error("dispatch: get transaction", "tx", row.TransactionID, "err", err)
		return // o lease vence e a linha volta para a fila
	}
	closeOutbox := func() {
		err := s.store.CloseTradeOutbox(ctx, repository.CloseTradeOutboxParams{TransactionID: tx.ID, Now: s.now()})
		logJobErr("close outbox", tx.ID, err)
	}
	if domain.TxStatus(tx.Status) != domain.TxPending {
		// Já saiu de pending (ex.: comprador cancelou): não há o que iniciar.
		// A Transition fecha a linha; isto só cobre dados antigos.
		closeOutbox()
		return
	}
	t, _, err := s.tradeFor(ctx, tx.ID)
	if err != nil {
		slog.Error("dispatch: trade context", "tx", tx.ID, "err", err)
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	offerID, sendErr := s.provider.Start(sendCtx, t)
	cancel()

	switch {
	case sendErr == nil && offerID == "":
		// P2P real: a oferta é do vendedor. Fica pending até o job de
		// verificação ver a oferta (ou o item) na Steam.
		closeOutbox()

	case sendErr == nil:
		_, err := s.Transition(ctx, tx.ID, domain.TxSent, OfferID(offerID))
		if errors.Is(err, domain.ErrInvalidTransition) {
			// O comprador cancelou enquanto a oferta era enviada: ela existe
			// na Steam, mas a compra não. Desfaz a oferta.
			slog.Warn("offer sent for a transaction that left pending; cancelling it", "tx", tx.ID)
			t.OfferID = offerID
			logJobErr("cancel orphan offer", tx.ID, s.provider.Cancel(ctx, t))
			return
		}
		logJobErr("mark offer sent", tx.ID, err)

	case errors.Is(sendErr, trade.ErrRejected), int(row.Attempts) >= s.cfg.MaxSendAttempts:
		slog.Warn("giving up on trade offer", "tx", tx.ID, "attempts", row.Attempts, "err", sendErr)
		_, err := s.Transition(ctx, tx.ID, domain.TxFailed, Reason("não foi possível enviar a oferta de troca"))
		logJobErr("fail transaction", tx.ID, err)

	default:
		msg := sendErr.Error()
		next := s.now().Add(retryDelay(row.Attempts))
		slog.Warn("send trade offer failed, will retry", "tx", tx.ID, "attempt", row.Attempts, "next", next, "err", sendErr)
		err := s.store.RescheduleTradeOutbox(ctx, repository.RescheduleTradeOutboxParams{
			TransactionID: tx.ID, NextAttemptAt: next, LastError: &msg,
		})
		logJobErr("reschedule outbox", tx.ID, err)
	}
}

// retryDelay é o backoff exponencial: 5s, 10s, 20s, ... até 5 min.
func retryDelay(attempt int32) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 { // evita overflow no shift; já passou do teto faz tempo
		return retryMax
	}
	return min(retryBase<<(attempt-1), retryMax)
}

// ---- Máquina de estados ----

type transitionOpts struct {
	offerID  *string
	reason   *string
	tradedAt *time.Time
	from     *domain.TxStatus
}

type TransitionOpt func(*transitionOpts)

func OfferID(id string) TransitionOpt { return func(o *transitionOpts) { o.offerID = &id } }
func Reason(r string) TransitionOpt   { return func(o *transitionOpts) { o.reason = &r } }

// TradedAt é o horário real da troca informado pela Steam: a proteção de 7
// dias conta a partir dele, não de quando a plataforma percebeu o aceite.
func TradedAt(t *time.Time) TransitionOpt { return func(o *transitionOpts) { o.tradedAt = t } }

// From exige que o status atual ainda seja o esperado. Decisões tomadas a
// partir de uma observação da Steam usam isto: se outro job mudou o status
// entre a observação e a ação, a ação é descartada (ErrInvalidTransition).
// Sem isso, um estorno decidido com o item "no vendedor" poderia ser aplicado
// a uma troca que acabou de ser aceita (on_hold → failed é válido).
func From(s domain.TxStatus) TransitionOpt { return func(o *transitionOpts) { o.from = &s } }

// Transition muda o status de uma transação e aplica os efeitos daquela
// mudança, tudo numa transação SQL:
//
//	→ on_hold              item passa para o comprador
//	→ completed            vendedor recebe (preço − taxa)
//	→ cancelled | failed   comprador é reembolsado; listing cancelada;
//	                       se o item já tinha trocado de dono, volta ao vendedor
//
// Transições fora da máquina de estados retornam domain.ErrInvalidTransition.
func (s *PurchaseService) Transition(ctx context.Context, txID int64, to domain.TxStatus, opts ...TransitionOpt) (domain.Transaction, error) {
	var o transitionOpts
	for _, opt := range opts {
		opt(&o)
	}

	var updated repository.Transaction
	err := s.store.ExecTx(ctx, func(q *repository.Queries) error {
		// Trava a transação: dois workers tentando avançar a mesma ficam em fila,
		// e o segundo vê o status já mudado.
		cur, err := q.GetTransactionForUpdate(ctx, txID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock transaction: %w", err)
		}

		from := domain.TxStatus(cur.Status)
		if o.from != nil && *o.from != from {
			return fmt.Errorf("%w: expected %s, found %s", domain.ErrInvalidTransition, *o.from, from)
		}
		if !from.CanTransitionTo(to) {
			return fmt.Errorf("%w: %s → %s", domain.ErrInvalidTransition, from, to)
		}

		params := repository.UpdateTransactionStatusParams{
			ID:            txID,
			FromStatus:    string(from),
			ToStatus:      string(to),
			TradeOfferID:  o.offerID,
			FailureReason: o.reason,
		}
		switch to {
		case domain.TxSent: // novo prazo: agora é a vez do comprador aceitar
			deadline := s.now().Add(s.cfg.BuyerAcceptTimeout)
			params.DeadlineAt = &deadline
		case domain.TxOnHold:
			start := s.now()
			// Só aceita um horário do passado (e não anterior à compra): um valor
			// estranho vindo de fora não pode encurtar nem alongar a proteção.
			// (created_at vem do relógio do banco: a margem cobre a diferença.)
			if t := o.tradedAt; t != nil && t.Before(start) && !t.Before(cur.CreatedAt.Add(-offerSinceMargin)) {
				start = *t
			}
			holdUntil := start.Add(s.cfg.HoldDuration)
			params.HoldUntil = &holdUntil
		}
		updated, err = q.UpdateTransactionStatus(ctx, params)
		if err != nil {
			return fmt.Errorf("update status: %w", err)
		}

		// Saiu de pending: o pedido de envio no outbox está resolvido (enviado,
		// cancelado ou falho). Fecha na mesma transação SQL da mudança de status.
		if from == domain.TxPending {
			if err := q.CloseTradeOutbox(ctx, repository.CloseTradeOutboxParams{TransactionID: txID, Now: s.now()}); err != nil {
				return fmt.Errorf("close outbox: %w", err)
			}
		}

		item, err := q.GetInventoryItem(ctx, cur.ItemID)
		if err != nil {
			return fmt.Errorf("get item: %w", err)
		}

		switch to {
		case domain.TxOnHold:
			// Item vai para o comprador, mas travado: não pode ser revendido
			// enquanto a troca ainda pode ser revertida.
			return q.TransferItemOwner(ctx, repository.TransferItemOwnerParams{
				ItemID: cur.ItemID, NewOwnerID: cur.BuyerID, Tradable: false,
			})

		case domain.TxCompleted:
			if err := q.SetItemTradable(ctx, repository.SetItemTradableParams{ItemID: cur.ItemID, Tradable: true}); err != nil {
				return err
			}
			if _, err := repository.ApplyWalletEntry(ctx, q, cur.SellerID, cur.PriceCents-cur.FeeCents,
				string(domain.EntrySale), &cur.ID, "Venda: "+item.Name); err != nil {
				return err
			}
			// A taxa vai para a conta da plataforma (por último na ordem de locks).
			if cur.FeeCents == 0 {
				return nil
			}
			platform, err := q.GetPlatformAccountID(ctx)
			if err != nil {
				return fmt.Errorf("platform account: %w", err)
			}
			_, err = repository.ApplyWalletEntry(ctx, q, platform, cur.FeeCents,
				string(domain.EntryFee), &cur.ID, "Taxa: "+item.Name)
			return err

		case domain.TxCancelled, domain.TxFailed:
			if _, err := repository.ApplyWalletEntry(ctx, q, cur.BuyerID, cur.PriceCents,
				string(domain.EntryRefund), &cur.ID, "Estorno: "+item.Name); err != nil {
				return err
			}
			if err := q.MarkListingCancelled(ctx, cur.ListingID); err != nil {
				return err
			}
			if from == domain.TxOnHold { // troca revertida: item volta ao vendedor
				// Defesa extra: nenhum anúncio do item pode sobrar ativo.
				if _, err := q.CancelActiveListingsForItem(ctx, cur.ItemID); err != nil {
					return err
				}
				return q.TransferItemOwner(ctx, repository.TransferItemOwnerParams{
					ItemID: cur.ItemID, NewOwnerID: cur.SellerID, Tradable: true,
				})
			}
		}
		return nil
	})
	if err != nil {
		return domain.Transaction{}, err
	}
	return toDomainTransaction(updated), nil
}

// GetForUser devolve a transação se o usuário for o comprador ou o vendedor.
func (s *PurchaseService) GetForUser(ctx context.Context, userID, txID int64) (domain.Transaction, error) {
	tx, err := s.store.GetTransaction(ctx, txID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Transaction{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("get transaction: %w", err)
	}
	if tx.BuyerID != userID && tx.SellerID != userID {
		return domain.Transaction{}, domain.ErrNotFound // não revela transações alheias
	}
	return toDomainTransaction(tx), nil
}

// Se outro processo já avançou a transação, ErrInvalidTransition é esperado.
func logJobErr(op string, txID int64, err error) {
	if err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
		slog.Error(op, "tx", txID, "err", err)
	}
}

func toDomainTransaction(t repository.Transaction) domain.Transaction {
	return domain.Transaction{
		ID:             t.ID,
		ListingID:      t.ListingID,
		BuyerID:        t.BuyerID,
		SellerID:       t.SellerID,
		ItemID:         t.ItemID,
		PriceCents:     t.PriceCents,
		FeeCents:       t.FeeCents,
		Status:         domain.TxStatus(t.Status),
		TradeOfferID:   t.TradeOfferID,
		HoldUntil:      t.HoldUntil,
		FailureReason:  t.FailureReason,
		BuyerTradeURL:  t.BuyerTradeURL,
		DeadlineAt:     t.DeadlineAt,
		LateDeliveryAt: t.LateDeliveryAt,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}
}
