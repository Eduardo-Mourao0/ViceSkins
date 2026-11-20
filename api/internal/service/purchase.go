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

// TradeProvider envia e acompanha a oferta de troca na Steam.
// No MVP a implementação é trade.Simulated.
//
// SendOffer pode ser chamado mais de uma vez para a mesma transação (retry
// depois de um timeout, lease vencido), então precisa ser idempotente por
// trade.Offer.TransactionID. Erros com trade.ErrRejected são permanentes;
// os outros são tentados de novo.
type TradeProvider interface {
	SendOffer(ctx context.Context, o trade.Offer) (offerID string, err error)
	OfferState(ctx context.Context, offerID string) (trade.OfferState, error)
	CancelOffer(ctx context.Context, offerID string) error
}

type PurchaseStore interface {
	ExecTx(ctx context.Context, fn func(q *repository.Queries) error) error
	GetTransaction(ctx context.Context, id int64) (repository.Transaction, error)
	GetInventoryItem(ctx context.Context, id int64) (repository.InventoryItem, error)
	ListTransactionIDsByStatus(ctx context.Context, arg repository.ListTransactionIDsByStatusParams) ([]int64, error)
	ListDueHolds(ctx context.Context, arg repository.ListDueHoldsParams) ([]int64, error)
	ListExpiredTransactions(ctx context.Context, arg repository.ListExpiredTransactionsParams) ([]int64, error)
	ClaimTradeOutbox(ctx context.Context, arg repository.ClaimTradeOutboxParams) ([]repository.TradeOutbox, error)
	RescheduleTradeOutbox(ctx context.Context, arg repository.RescheduleTradeOutboxParams) error
	CloseTradeOutbox(ctx context.Context, arg repository.CloseTradeOutboxParams) error
}

type PurchaseConfig struct {
	FeeBps          int64         // taxa da plataforma em basis points (200 = 2%)
	HoldDuration    time.Duration // proteção de troca da Steam (7 dias)
	MaxSendAttempts int           // tentativas de enviar a oferta antes de desistir e estornar

	// Prazos das etapas. Perdeu o prazo → failed, com estorno ao comprador.
	SellerSendTimeout  time.Duration // pending: vendedor enviar a oferta
	BuyerAcceptTimeout time.Duration // sent: comprador aceitar
}

const (
	defaultStepTimeout     = 12 * time.Hour
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
	if domain.TxStatus(tx.Status) != domain.TxPending {
		// Já saiu de pending (ex.: comprador cancelou): não há o que enviar.
		// A Transition fecha a linha; isto só cobre dados antigos.
		logJobErr("close stale outbox", tx.ID, s.store.CloseTradeOutbox(ctx, repository.CloseTradeOutboxParams{TransactionID: tx.ID, Now: s.now()}))
		return
	}
	item, err := s.store.GetInventoryItem(ctx, tx.ItemID)
	if err != nil {
		slog.Error("dispatch: get item", "tx", tx.ID, "err", err)
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	offerID, sendErr := s.provider.SendOffer(sendCtx, trade.Offer{
		TransactionID: tx.ID,
		AssetID:       item.AssetID,
		SellerID:      tx.SellerID,
		BuyerID:       tx.BuyerID,
		BuyerTradeURL: deref(tx.BuyerTradeURL),
	})
	cancel()

	switch {
	case sendErr == nil:
		_, err := s.Transition(ctx, tx.ID, domain.TxSent, OfferID(offerID))
		if errors.Is(err, domain.ErrInvalidTransition) {
			// O comprador cancelou enquanto a oferta era enviada: ela existe
			// na Steam, mas a compra não. Desfaz a oferta.
			slog.Warn("offer sent for a transaction that left pending; cancelling it", "tx", tx.ID)
			logJobErr("cancel orphan offer", tx.ID, s.provider.CancelOffer(ctx, offerID))
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
	offerID *string
	reason  *string
}

type TransitionOpt func(*transitionOpts)

func OfferID(id string) TransitionOpt { return func(o *transitionOpts) { o.offerID = &id } }
func Reason(r string) TransitionOpt   { return func(o *transitionOpts) { o.reason = &r } }

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
			holdUntil := s.now().Add(s.cfg.HoldDuration)
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
			_, err := repository.ApplyWalletEntry(ctx, q, cur.SellerID, cur.PriceCents-cur.FeeCents,
				string(domain.EntrySale), &cur.ID, "Venda: "+item.Name)
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

// CancelByBuyer deixa o comprador desistir enquanto a troca não foi aceita.
func (s *PurchaseService) CancelByBuyer(ctx context.Context, buyerID, txID int64) (domain.Transaction, error) {
	tx, err := s.store.GetTransaction(ctx, txID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Transaction{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("get transaction: %w", err)
	}
	if tx.SellerID == buyerID {
		return domain.Transaction{}, domain.ErrForbidden // vendedor vê, mas não cancela
	}
	if tx.BuyerID != buyerID {
		return domain.Transaction{}, domain.ErrNotFound // não revela transações alheias
	}
	return s.abort(ctx, tx, domain.TxCancelled, "cancelada pelo comprador")
}

// abort encerra uma transação em andamento (→ cancelled ou failed, com estorno).
//
// Se a oferta já foi enviada, ela é cancelada na Steam ANTES do estorno. Se o
// cancelamento falhar, nada muda: senão o comprador poderia aceitar a troca
// e ficar com o item e o dinheiro.
func (s *PurchaseService) abort(ctx context.Context, tx repository.Transaction, to domain.TxStatus, reason string) (domain.Transaction, error) {
	if domain.TxStatus(tx.Status) == domain.TxSent && tx.TradeOfferID != nil {
		if err := s.provider.CancelOffer(ctx, *tx.TradeOfferID); err != nil {
			return domain.Transaction{}, fmt.Errorf("cancel trade offer: %w", err)
		}
	}
	return s.Transition(ctx, tx.ID, to, Reason(reason))
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

// ---- Usados pelos jobs em background ----

const jobBatchSize = 50

// ProcessSentOffers consulta as ofertas enviadas: aceita → on_hold; recusada → failed.
func (s *PurchaseService) ProcessSentOffers(ctx context.Context) error {
	ids, err := s.store.ListTransactionIDsByStatus(ctx, repository.ListTransactionIDsByStatusParams{
		Status:  string(domain.TxSent),
		MaxRows: jobBatchSize,
	})
	if err != nil {
		return fmt.Errorf("list sent transactions: %w", err)
	}

	for _, id := range ids {
		tx, err := s.store.GetTransaction(ctx, id)
		if err != nil || tx.TradeOfferID == nil {
			continue
		}
		state, err := s.provider.OfferState(ctx, *tx.TradeOfferID)
		if err != nil {
			slog.Warn("offer state", "tx", id, "err", err)
			continue
		}
		switch state {
		case trade.OfferAccepted:
			_, err = s.Transition(ctx, id, domain.TxOnHold)
		case trade.OfferDeclined:
			_, err = s.Transition(ctx, id, domain.TxFailed, Reason("oferta de troca recusada ou expirada"))
		}
		logJobErr("process sent offer", id, err)
	}
	return nil
}

// ExpireDeadlines encerra as transações que perderam o prazo da etapa atual:
// vendedor não enviou a oferta (pending) ou comprador não aceitou (sent).
// O comprador é estornado. Se cancelar a oferta na Steam falhar, a transação
// fica como está e é tentada de novo no próximo tick.
func (s *PurchaseService) ExpireDeadlines(ctx context.Context) error {
	ids, err := s.store.ListExpiredTransactions(ctx, repository.ListExpiredTransactionsParams{
		Now: s.now(), MaxRows: jobBatchSize,
	})
	if err != nil {
		return fmt.Errorf("list expired transactions: %w", err)
	}
	for _, id := range ids {
		tx, err := s.store.GetTransaction(ctx, id)
		if err != nil {
			slog.Error("expire: get transaction", "tx", id, "err", err)
			continue
		}
		reason := "o vendedor não enviou a oferta de troca no prazo"
		if domain.TxStatus(tx.Status) == domain.TxSent {
			reason = "o comprador não aceitou a oferta de troca no prazo"
		}
		_, err = s.abort(ctx, tx, domain.TxFailed, reason)
		logJobErr("expire transaction", id, err)
	}
	return nil
}

// ReleaseDueHolds conclui as transações cujo período de proteção acabou.
func (s *PurchaseService) ReleaseDueHolds(ctx context.Context) error {
	ids, err := s.store.ListDueHolds(ctx, repository.ListDueHoldsParams{Now: s.now(), MaxRows: jobBatchSize})
	if err != nil {
		return fmt.Errorf("list due holds: %w", err)
	}
	for _, id := range ids {
		_, err := s.Transition(ctx, id, domain.TxCompleted)
		logJobErr("release hold", id, err)
	}
	return nil
}

// Se outro processo já avançou a transação, ErrInvalidTransition é esperado.
func logJobErr(op string, txID int64, err error) {
	if err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
		slog.Error(op, "tx", txID, "err", err)
	}
}

func toDomainTransaction(t repository.Transaction) domain.Transaction {
	return domain.Transaction{
		ID:            t.ID,
		ListingID:     t.ListingID,
		BuyerID:       t.BuyerID,
		SellerID:      t.SellerID,
		ItemID:        t.ItemID,
		PriceCents:    t.PriceCents,
		FeeCents:      t.FeeCents,
		Status:        domain.TxStatus(t.Status),
		TradeOfferID:  t.TradeOfferID,
		HoldUntil:     t.HoldUntil,
		FailureReason: t.FailureReason,
		BuyerTradeURL: t.BuyerTradeURL,
		DeadlineAt:    t.DeadlineAt,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
