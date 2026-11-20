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
type TradeProvider interface {
	SendOffer(ctx context.Context, o trade.Offer) (offerID string, err error)
	OfferState(ctx context.Context, offerID string) (trade.OfferState, error)
}

type PurchaseStore interface {
	ExecTx(ctx context.Context, fn func(q *repository.Queries) error) error
	GetTransaction(ctx context.Context, id int64) (repository.Transaction, error)
	ListTransactionIDsByStatus(ctx context.Context, arg repository.ListTransactionIDsByStatusParams) ([]int64, error)
	ListDueHolds(ctx context.Context, arg repository.ListDueHoldsParams) ([]int64, error)
}

type PurchaseConfig struct {
	FeeBps       int64         // taxa da plataforma em basis points (200 = 2%)
	HoldDuration time.Duration // proteção de troca da Steam (7 dias)
}

type PurchaseService struct {
	store    PurchaseStore
	provider TradeProvider
	cfg      PurchaseConfig
	now      func() time.Time
}

func NewPurchaseService(store PurchaseStore, provider TradeProvider, cfg PurchaseConfig) *PurchaseService {
	return &PurchaseService{store: store, provider: provider, cfg: cfg, now: time.Now}
}

// Buy compra uma listing. Tudo que mexe em dinheiro e estoque acontece numa
// única transação SQL; o envio da oferta de troca vem depois, fora dela.
func (s *PurchaseService) Buy(ctx context.Context, buyerID, listingID int64) (domain.Transaction, error) {
	var created repository.Transaction
	var assetID string

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
		balance, err := q.GetUserBalanceForUpdate(ctx, buyerID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock buyer: %w", err)
		}
		if balance < l.PriceCents {
			return domain.ErrInsufficientBalance
		}

		item, err := q.GetInventoryItem(ctx, l.ItemID)
		if err != nil {
			return fmt.Errorf("get item: %w", err)
		}
		assetID = item.AssetID

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
		})
		if err != nil {
			return fmt.Errorf("create transaction: %w", err)
		}
		_, err = repository.ApplyWalletEntry(ctx, q, buyerID, -l.PriceCents, string(domain.EntryPurchase),
			&created.ID, "Compra: "+item.Name)
		return err
	})
	if err != nil {
		return domain.Transaction{}, err
	}

	// 4. Fora da transação SQL: nunca segure locks do banco esperando I/O externo.
	return s.sendOffer(ctx, created, assetID)
}

// sendOffer chama o TradeProvider e avança pending → sent (ou → failed com reembolso).
//
// A compra já foi gravada quando chegamos aqui, então nunca devolvemos erro:
// se algo falhar, o comprador recebe a transação como está (pending) e pode
// cancelá-la, o que gera o estorno.
func (s *PurchaseService) sendOffer(ctx context.Context, tx repository.Transaction, assetID string) (domain.Transaction, error) {
	var next domain.Transaction
	offerID, err := s.provider.SendOffer(ctx, trade.Offer{
		TransactionID: tx.ID,
		AssetID:       assetID,
		SellerID:      tx.SellerID,
		BuyerID:       tx.BuyerID,
	})
	if err != nil {
		slog.Warn("send trade offer failed", "tx", tx.ID, "err", err)
		next, err = s.Transition(ctx, tx.ID, domain.TxFailed, Reason("não foi possível enviar a oferta de troca"))
	} else {
		next, err = s.Transition(ctx, tx.ID, domain.TxSent, OfferID(offerID))
	}
	if err != nil {
		slog.Error("transition after purchase", "tx", tx.ID, "err", err)
		return toDomainTransaction(tx), nil
	}
	return next, nil
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
		if to == domain.TxOnHold {
			holdUntil := s.now().Add(s.cfg.HoldDuration)
			params.HoldUntil = &holdUntil
		}
		updated, err = q.UpdateTransactionStatus(ctx, params)
		if err != nil {
			return fmt.Errorf("update status: %w", err)
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
	return s.Transition(ctx, txID, domain.TxCancelled, Reason("cancelada pelo comprador"))
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
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}
