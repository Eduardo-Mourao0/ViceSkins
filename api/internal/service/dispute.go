package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/trade"
)

// Disputas de entrega tardia (ver domain.Dispute).
//
// Ordem dos locks: disputa → transação → usuário → plataforma (abrir trava a
// transação e só então cria a disputa, que ainda não existe). A conta da
// plataforma é sempre a última, como no resto do código.

// openDispute abre a disputa de uma transação estornada cujo item apareceu
// com o comprador. Cobra o comprador de novo, na hora: o que houver de saldo
// vai para a plataforma; o que faltar vira dívida dele. O item passa para o
// comprador, travado durante a proteção da troca tardia.
// Devolve false se outro worker já abriu (ou se a transação não se aplica).
func (s *PurchaseService) openDispute(ctx context.Context, txID int64, obs trade.Observation) (bool, error) {
	opened := false
	err := s.store.ExecTx(ctx, func(q *repository.Queries) error {
		cur, err := q.GetTransactionForUpdate(ctx, txID)
		if err != nil {
			return fmt.Errorf("lock transaction: %w", err)
		}
		status := domain.TxStatus(cur.Status)
		if cur.LateDeliveryAt != nil || (status != domain.TxCancelled && status != domain.TxFailed) {
			return nil // outro worker chegou antes, ou não houve estorno
		}

		money, err := q.GetUserMoneyForUpdate(ctx, cur.BuyerID)
		if err != nil {
			return fmt.Errorf("lock buyer: %w", err)
		}
		charged := min(money.BalanceCents, cur.PriceCents)
		covered := cur.PriceCents - charged

		item, err := q.GetInventoryItem(ctx, cur.ItemID)
		if err != nil {
			return fmt.Errorf("get item: %w", err)
		}
		if charged > 0 {
			if err := repository.TransferWithPlatform(ctx, q, cur.BuyerID, -charged,
				string(domain.EntryDisputeCharge), string(domain.EntryDisputeCollect), &cur.ID,
				"Cobrança: "+item.Name+" recebido depois do estorno"); err != nil {
				return err
			}
		}
		if covered > 0 {
			if err := q.AddDebt(ctx, repository.AddDebtParams{UserID: cur.BuyerID, AmountCents: covered}); err != nil {
				return err
			}
		}

		now := s.now()
		// A troca tardia também tem proteção: o vendedor só recebe no fim dela.
		start := now
		if t := obs.TradedAt; t != nil && t.Before(now) && !t.Before(cur.UpdatedAt.Add(-offerSinceMargin)) {
			start = *t
		}
		evidence := "fingerprint"
		if obs.Item != trade.ItemWithBuyer {
			evidence = "offer"
		}
		if _, err := q.CreateDispute(ctx, repository.CreateDisputeParams{
			TransactionID: cur.ID,
			DetectedAt:    now,
			RefundedAt:    cur.UpdatedAt,
			Evidence:      evidence,
			TradedAt:      obs.TradedAt,
			ChargedCents:  charged,
			CoveredCents:  covered,
			HoldUntil:     start.Add(s.cfg.HoldDuration),
		}); err != nil {
			return fmt.Errorf("create dispute: %w", err)
		}
		if err := q.MarkLateDelivery(ctx, repository.MarkLateDeliveryParams{ID: cur.ID, Now: now}); err != nil {
			return err
		}

		// O item agora é do comprador. Se o vendedor o anunciou de novo, o
		// anúncio cai: o item não está mais com ele.
		if _, err := q.CancelActiveListingsForItem(ctx, cur.ItemID); err != nil {
			return err
		}
		if err := q.TransferItemOwner(ctx, repository.TransferItemOwnerParams{
			ItemID: cur.ItemID, NewOwnerID: cur.BuyerID, Tradable: false,
		}); err != nil {
			return err
		}
		opened = true
		return nil
	})
	return opened, err
}

// ReleaseDueDisputes conclui as disputas cuja proteção acabou, com a mesma
// conferência das vendas normais: se o item não voltou para o vendedor, ele
// recebe (preço − taxa) da plataforma; se voltou, a cobrança é desfeita.
func (s *PurchaseService) ReleaseDueDisputes(ctx context.Context) error {
	ids, err := s.store.ListDueDisputes(ctx, repository.ListDueDisputesParams{Now: s.now(), MaxRows: jobBatchSize})
	if err != nil {
		return fmt.Errorf("list due disputes: %w", err)
	}
	for _, id := range ids {
		if err := s.releaseDispute(ctx, id); err != nil {
			slog.Error("release dispute", "dispute", id, "err", err)
		}
	}
	return nil
}

func (s *PurchaseService) releaseDispute(ctx context.Context, disputeID int64) error {
	var txID int64
	err := s.store.ExecTx(ctx, func(q *repository.Queries) error {
		d, err := q.GetDisputeForUpdate(ctx, disputeID)
		txID = d.TransactionID
		return err
	})
	if err != nil {
		return err
	}

	t, _, err := s.tradeFor(ctx, txID)
	if err != nil {
		return err
	}
	obs, err := s.provider.Check(ctx, t)
	if err != nil {
		slog.Warn("dispute due but steam unavailable; not paying yet", "dispute", disputeID, "err", err)
		return nil
	}
	switch obs.Item {
	case trade.ItemWithBuyer, trade.ItemElsewhere:
		return s.settleDispute(ctx, disputeID)
	case trade.ItemWithSeller:
		return s.undoDispute(ctx, disputeID, domain.DisputeReversed, nil, nil)
	default:
		return nil // sem confirmação: tenta de novo
	}
}

// settleDispute: a troca tardia se manteve. A plataforma paga o vendedor
// integralmente, mesmo que parte da cobrança tenha virado dívida do comprador
// (é a garantia da plataforma).
func (s *PurchaseService) settleDispute(ctx context.Context, disputeID int64) error {
	return s.store.ExecTx(ctx, func(q *repository.Queries) error {
		d, cur, err := lockDispute(ctx, q, disputeID)
		if err != nil || d.Status != string(domain.DisputeHolding) {
			return err
		}
		n, err := q.ResolveDispute(ctx, repository.ResolveDisputeParams{ID: d.ID, Status: string(domain.DisputeSettled), Now: s.now()})
		if err != nil || n != 1 {
			return err
		}
		item, err := q.GetInventoryItem(ctx, cur.ItemID)
		if err != nil {
			return err
		}
		if err := repository.TransferWithPlatform(ctx, q, cur.SellerID, cur.PriceCents-cur.FeeCents,
			string(domain.EntrySale), string(domain.EntryDisputePayout), &cur.ID, "Venda (entrega tardia): "+item.Name); err != nil {
			return err
		}
		return q.SetItemTradable(ctx, repository.SetItemTradableParams{ItemID: cur.ItemID, Tradable: true})
	})
}

// undoDispute desfaz a cobrança (troca revertida ou disputa arquivada): devolve
// ao comprador o que ele pagou e cancela a dívida que ainda resta. O item
// volta ao vendedor.
func (s *PurchaseService) undoDispute(ctx context.Context, disputeID int64, to domain.DisputeStatus, adminID *int64, note *string) error {
	return s.store.ExecTx(ctx, func(q *repository.Queries) error {
		d, cur, err := lockDispute(ctx, q, disputeID)
		if err != nil {
			return err
		}
		if d.Status != string(domain.DisputeHolding) {
			return fmt.Errorf("%w: dispute is %s", domain.ErrInvalidTransition, d.Status)
		}
		if _, err := q.ResolveDispute(ctx, repository.ResolveDisputeParams{
			ID: d.ID, Status: string(to), Now: s.now(), ResolvedBy: adminID, Note: note,
		}); err != nil {
			return err
		}

		money, err := q.GetUserMoneyForUpdate(ctx, cur.BuyerID)
		if err != nil {
			return err
		}
		// Parte da dívida pode já ter sido paga com depósitos: o que foi pago
		// volta; o que ainda é devido é perdoado.
		forgiven := min(d.CoveredCents, money.DebtCents)
		refund := d.ChargedCents + (d.CoveredCents - forgiven)
		if forgiven > 0 {
			if err := q.AddDebt(ctx, repository.AddDebtParams{UserID: cur.BuyerID, AmountCents: -forgiven}); err != nil {
				return err
			}
		}
		if refund > 0 {
			if err := repository.TransferWithPlatform(ctx, q, cur.BuyerID, refund,
				string(domain.EntryDisputeRefund), string(domain.EntryDisputeRefund), &cur.ID,
				"Estorno: cobrança de entrega tardia desfeita"); err != nil {
				return err
			}
		}
		if _, err := q.CancelActiveListingsForItem(ctx, cur.ItemID); err != nil {
			return err
		}
		return q.TransferItemOwner(ctx, repository.TransferItemOwnerParams{
			ItemID: cur.ItemID, NewOwnerID: cur.SellerID, Tradable: true,
		})
	})
}

// lockDispute trava a disputa e depois a transação dela.
func lockDispute(ctx context.Context, q *repository.Queries, disputeID int64) (repository.Dispute, repository.Transaction, error) {
	d, err := q.GetDisputeForUpdate(ctx, disputeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.Dispute{}, repository.Transaction{}, domain.ErrNotFound
	}
	if err != nil {
		return repository.Dispute{}, repository.Transaction{}, err
	}
	cur, err := q.GetTransactionForUpdate(ctx, d.TransactionID)
	if err != nil {
		return repository.Dispute{}, repository.Transaction{}, err
	}
	return d, cur, nil
}

// ---- Admin ----

// DismissDispute: o admin conclui que a detecção estava errada. A cobrança é
// desfeita e o item volta ao vendedor.
func (s *PurchaseService) DismissDispute(ctx context.Context, adminID, disputeID int64, note string) error {
	if note == "" {
		return fmt.Errorf("%w: a note explaining the decision is required", domain.ErrInvalidInput)
	}
	return s.undoDispute(ctx, disputeID, domain.DisputeDismissed, &adminID, &note)
}

func (s *PurchaseService) ListDisputes(ctx context.Context, status string) ([]domain.DisputeView, error) {
	var filter *string
	if status != "" {
		switch domain.DisputeStatus(status) {
		case domain.DisputeHolding, domain.DisputeSettled, domain.DisputeReversed, domain.DisputeDismissed:
			filter = &status
		default:
			return nil, fmt.Errorf("%w: unknown dispute status %q", domain.ErrInvalidInput, status)
		}
	}
	rows, err := s.store.ListDisputes(ctx, repository.ListDisputesParams{Status: filter, MaxRows: 100})
	if err != nil {
		return nil, fmt.Errorf("list disputes: %w", err)
	}
	out := make([]domain.DisputeView, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.DisputeView{
			Dispute:        toDomainDispute(r.Dispute),
			Transaction:    toDomainTransaction(r.Transaction),
			ItemName:       r.ItemName,
			ItemImageURL:   r.ItemIconURL,
			BuyerName:      r.BuyerName,
			BuyerDebtCents: r.BuyerDebtCents,
			SellerName:     r.SellerName,
		})
	}
	return out, nil
}

func toDomainDispute(d repository.Dispute) domain.Dispute {
	return domain.Dispute{
		ID:            d.ID,
		TransactionID: d.TransactionID,
		Status:        domain.DisputeStatus(d.Status),
		DetectedAt:    d.DetectedAt,
		RefundedAt:    d.RefundedAt,
		Evidence:      d.Evidence,
		TradedAt:      d.TradedAt,
		ChargedCents:  d.ChargedCents,
		CoveredCents:  d.CoveredCents,
		HoldUntil:     d.HoldUntil,
		ResolvedAt:    d.ResolvedAt,
		ResolvedBy:    d.ResolvedBy,
		Note:          d.Note,
		CreatedAt:     d.CreatedAt,
	}
}
