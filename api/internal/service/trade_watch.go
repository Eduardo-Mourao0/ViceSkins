package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/trade"
)

// Verificação das trocas P2P.
//
// O provider só diz o que viu na Steam (trade.Observation). As decisões são
// daqui, e são conservadoras: na dúvida, nada acontece e a próxima rodada
// verifica de novo. As três regras que protegem o dinheiro:
//
//  1. Estorno só com o item comprovadamente no vendedor. Nunca se devolve o
//     dinheiro de um item que pode já ter sido entregue.
//  2. Entrega só com prova positiva: o item visto no comprador, ou a oferta
//     aceita (com API key) sem o item de volta no vendedor.
//  3. Pagamento ao vendedor só se, no fim da proteção, o item não voltou
//     para ele. Sem resposta da Steam, não paga.

const (
	jobBatchSize = 50

	// Depois de um estorno, uma oferta que ficou aberta ainda pode ser aceita.
	// A transação é vigiada por este período para detectar entrega tardia.
	lateDeliveryWatch = 7 * 24 * time.Hour

	// Margem para diferença de relógio entre a Steam e a API ao buscar as
	// ofertas enviadas desde a compra.
	offerSinceMargin = 5 * time.Minute
)

func apiKeyAAD(userID int64) []byte {
	return []byte("steam-api-key:" + strconv.FormatInt(userID, 10))
}

// tradeFor monta o que o provider precisa, já com a API key do vendedor aberta.
func (s *PurchaseService) tradeFor(ctx context.Context, txID int64) (trade.Trade, domain.TxStatus, error) {
	c, err := s.store.GetTradeContext(ctx, txID)
	if errors.Is(err, pgx.ErrNoRows) {
		return trade.Trade{}, "", domain.ErrNotFound
	}
	if err != nil {
		return trade.Trade{}, "", fmt.Errorf("trade context: %w", err)
	}
	t := trade.Trade{
		TransactionID: c.ID,
		Item: trade.Item{
			AssetID:        c.AssetID,
			MarketHashName: c.MarketHashName,
			Float:          c.FloatValue,
			PaintSeed:      c.PaintSeed,
		},
		SellerSteamID: c.SellerSteamID,
		BuyerSteamID:  c.BuyerSteamID,
		Since:         c.CreatedAt.Add(-offerSinceMargin),
	}
	if c.BuyerTradeURL != nil {
		t.BuyerTradeURL = *c.BuyerTradeURL
	}
	if c.TradeOfferID != nil {
		t.OfferID = *c.TradeOfferID
	}
	if len(c.SellerAPIKeyEnc) > 0 && s.cfg.Keys != nil {
		key, err := s.cfg.Keys.Open(c.SellerAPIKeyEnc, apiKeyAAD(c.SellerID))
		if err != nil {
			// Segue só com o inventário. Nunca loga a key nem o valor cifrado.
			slog.Error("cannot open seller api key", "user", c.SellerID)
		} else {
			t.SellerAPIKey = string(key)
		}
	}
	return t, domain.TxStatus(c.Status), nil
}

// delivered: há prova de que o comprador recebeu o item (regra 2).
func delivered(o trade.Observation) bool {
	return o.Item == trade.ItemWithBuyer || (o.Offer == trade.OfferAccepted && o.Item != trade.ItemWithSeller)
}

// ---- Jobs ----

// WatchTrades verifica as trocas em andamento e avança cada uma conforme o
// que a Steam mostra. Cada transação é verificada no máximo a cada
// CheckInterval (rate limit do inventário).
func (s *PurchaseService) WatchTrades(ctx context.Context) error {
	ids, err := s.store.ListTradesToCheck(ctx, repository.ListTradesToCheckParams{
		Cutoff: s.now().Add(-s.cfg.CheckInterval), MaxRows: jobBatchSize,
	})
	if err != nil {
		return fmt.Errorf("list trades to check: %w", err)
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return nil // shutdown: termina a rodada sem começar outra verificação
		}
		s.watchTrade(ctx, id)
	}
	return nil
}

func (s *PurchaseService) watchTrade(ctx context.Context, id int64) {
	t, status, obs, ok := s.observe(ctx, id)
	if !ok {
		return
	}

	var err error
	switch status {
	case domain.TxPending, domain.TxSent:
		switch {
		case delivered(obs):
			_, err = s.Transition(ctx, id, domain.TxOnHold, From(status), TradedAt(obs.TradedAt), offerIDOpt(obs))
		case status == domain.TxPending && obs.Offer == trade.OfferPending && obs.OfferID != "":
			_, err = s.Transition(ctx, id, domain.TxSent, From(status), OfferID(obs.OfferID))
		case status == domain.TxSent && obs.Offer == trade.OfferDeclined && obs.Item == trade.ItemWithSeller:
			_, err = s.Transition(ctx, id, domain.TxFailed, From(status), Reason("oferta de troca recusada ou cancelada na Steam"))
		}
	case domain.TxOnHold:
		if obs.Item == trade.ItemWithSeller {
			_, err = s.Transition(ctx, id, domain.TxFailed, From(status), Reason("troca revertida: o item voltou para o vendedor"))
		}
	}
	logJobErr("watch trade", t.TransactionID, err)
}

// observe marca a transação como verificada e consulta o provider.
// ok=false: não deu para saber nada agora (já foi logado).
func (s *PurchaseService) observe(ctx context.Context, id int64) (trade.Trade, domain.TxStatus, trade.Observation, bool) {
	// Marca antes de consultar: uma Steam fora do ar não faz o job martelar
	// a mesma transação a cada tick.
	if err := s.store.MarkTradeChecked(ctx, repository.MarkTradeCheckedParams{ID: id, Now: s.now()}); err != nil {
		slog.Error("mark trade checked", "tx", id, "err", err)
		return trade.Trade{}, "", trade.Observation{}, false
	}
	t, status, err := s.tradeFor(ctx, id)
	if err != nil {
		slog.Error("trade context", "tx", id, "err", err)
		return trade.Trade{}, "", trade.Observation{}, false
	}
	obs, err := s.provider.Check(ctx, t)
	if err != nil {
		slog.Warn("check trade on steam", "tx", id, "err", err)
		return trade.Trade{}, "", trade.Observation{}, false
	}
	return t, status, obs, true
}

func offerIDOpt(o trade.Observation) TransitionOpt {
	if o.OfferID == "" {
		return func(*transitionOpts) {}
	}
	return OfferID(o.OfferID)
}

// ReleaseDueHolds paga o vendedor das trocas cuja proteção acabou, depois de
// conferir na Steam que a troca não foi revertida (regra 3).
func (s *PurchaseService) ReleaseDueHolds(ctx context.Context) error {
	ids, err := s.store.ListDueHolds(ctx, repository.ListDueHoldsParams{Now: s.now(), MaxRows: jobBatchSize})
	if err != nil {
		return fmt.Errorf("list due holds: %w", err)
	}
	for _, id := range ids {
		_, status, obs, ok := s.observe(ctx, id)
		if !ok {
			continue // sem confirmação, não paga: tenta na próxima rodada
		}
		switch obs.Item {
		case trade.ItemWithBuyer, trade.ItemElsewhere:
			// Não voltou para o vendedor: a troca se manteve. (Elsewhere: o
			// comprador já pode ter repassado o item depois da proteção.)
			_, err = s.Transition(ctx, id, domain.TxCompleted, From(status))
		case trade.ItemWithSeller:
			_, err = s.Transition(ctx, id, domain.TxFailed, From(status), Reason("troca revertida: o item voltou para o vendedor"))
		default:
			slog.Warn("hold due but trade location unknown; not paying yet", "tx", id)
			continue
		}
		logJobErr("release hold", id, err)
	}
	return nil
}

// ExpireDeadlines encerra as transações que perderam o prazo da etapa atual:
// vendedor não enviou a oferta (pending) ou comprador não aceitou (sent).
// Antes de estornar, confere na Steam: se o item já foi entregue, a
// transação avança; se não dá para confirmar que ele está no vendedor, espera.
func (s *PurchaseService) ExpireDeadlines(ctx context.Context) error {
	ids, err := s.store.ListExpiredTransactions(ctx, repository.ListExpiredTransactionsParams{
		Now: s.now(), MaxRows: jobBatchSize,
	})
	if err != nil {
		return fmt.Errorf("list expired transactions: %w", err)
	}
	for _, id := range ids {
		t, status, obs, ok := s.observe(ctx, id)
		if !ok {
			continue
		}
		if delivered(obs) { // chegou no limite: vale a entrega
			_, err := s.Transition(ctx, id, domain.TxOnHold, From(status), TradedAt(obs.TradedAt), offerIDOpt(obs))
			logJobErr("expire: deliver", id, err)
			continue
		}
		if obs.Item != trade.ItemWithSeller {
			slog.Warn("deadline passed but item is not confirmed at the seller; waiting", "tx", id, "item", obs.Item)
			continue
		}
		reason := "o vendedor não enviou a oferta de troca no prazo"
		if status == domain.TxSent {
			reason = "o comprador não aceitou a oferta de troca no prazo"
		}
		_, err := s.abort(ctx, t, status, domain.TxFailed, reason, false)
		logJobErr("expire transaction", id, err)
	}
	return nil
}

// WatchClosedTrades vigia as trocas estornadas há pouco tempo. Se o item
// aparece com o comprador depois do estorno (ele aceitou uma oferta que ficou
// aberta), a transação é marcada para análise: o comprador teria item e
// dinheiro. Nenhum dinheiro é movido automaticamente.
func (s *PurchaseService) WatchClosedTrades(ctx context.Context) error {
	now := s.now()
	ids, err := s.store.ListClosedTradesToWatch(ctx, repository.ListClosedTradesToWatchParams{
		Since: now.Add(-lateDeliveryWatch), Cutoff: now.Add(-s.cfg.CheckInterval), MaxRows: jobBatchSize,
	})
	if err != nil {
		return fmt.Errorf("list closed trades: %w", err)
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return nil
		}
		_, _, obs, ok := s.observe(ctx, id)
		if !ok || !delivered(obs) {
			continue
		}
		slog.Warn("late delivery: item reached the buyer after the refund", "tx", id)
		err := s.store.MarkLateDelivery(ctx, repository.MarkLateDeliveryParams{ID: id, Now: s.now()})
		logJobErr("mark late delivery", id, err)
	}
	return nil
}

// ---- Ações das partes ----

// MarkSentBySeller: sem API key a plataforma não vê a oferta; o vendedor
// avisa que enviou. Isso só muda o prazo (agora é a vez do comprador) e
// impede o comprador de cancelar sozinho. A entrega continua sendo
// confirmada pelo inventário.
func (s *PurchaseService) MarkSentBySeller(ctx context.Context, sellerID, txID int64) (domain.Transaction, error) {
	tx, err := s.store.GetTransaction(ctx, txID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Transaction{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("get transaction: %w", err)
	}
	if tx.BuyerID == sellerID {
		return domain.Transaction{}, domain.ErrForbidden
	}
	if tx.SellerID != sellerID {
		return domain.Transaction{}, domain.ErrNotFound
	}
	return s.Transition(ctx, txID, domain.TxSent)
}

// CancelByBuyer deixa o comprador desistir enquanto a troca não aconteceu.
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
	status := domain.TxStatus(tx.Status)
	if !status.CanTransitionTo(domain.TxCancelled) {
		return domain.Transaction{}, fmt.Errorf("%w: %s → %s", domain.ErrInvalidTransition, status, domain.TxCancelled)
	}

	t, status, err := s.tradeFor(ctx, txID)
	if err != nil {
		return domain.Transaction{}, err
	}
	obs, err := s.provider.Check(ctx, t)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("%w: %v", domain.ErrTradeUnverified, err)
	}
	if delivered(obs) {
		return domain.Transaction{}, fmt.Errorf("%w: the item was already delivered", domain.ErrInvalidTransition)
	}
	if obs.Item != trade.ItemWithSeller {
		return domain.Transaction{}, domain.ErrTradeUnverified // regra 1
	}
	return s.abort(ctx, t, status, domain.TxCancelled, "cancelada pelo comprador", true)
}

// abort encerra uma transação em andamento (→ cancelled ou failed, com estorno).
//
// Uma oferta aberta é cancelada na Steam ANTES do estorno; se o cancelamento
// falhar, nada muda. Sem API key não há como cancelar:
//   - o comprador não pode desistir de uma oferta já enviada (ele mesmo pode
//     recusá-la na Steam, e o prazo encerra a compra);
//   - na expiração, segue: se o comprador aceitar depois, WatchClosedTrades
//     marca a transação para análise.
func (s *PurchaseService) abort(ctx context.Context, t trade.Trade, status, to domain.TxStatus, reason string, byBuyer bool) (domain.Transaction, error) {
	err := s.provider.Cancel(ctx, t)
	switch {
	case errors.Is(err, trade.ErrCannotCancel):
		if byBuyer && status == domain.TxSent {
			return domain.Transaction{}, domain.ErrCannotCancelOffer
		}
	case err != nil:
		return domain.Transaction{}, fmt.Errorf("cancel trade offer: %w", err)
	}
	return s.Transition(ctx, t.TransactionID, to, From(status), Reason(reason))
}
