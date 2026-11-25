package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
)

type WalletStore interface {
	GetUserByID(ctx context.Context, id int64) (repository.User, error)
	ListWalletEntries(ctx context.Context, arg repository.ListWalletEntriesParams) ([]repository.WalletEntry, error)
	GetLockedSaleEarnings(ctx context.Context, sellerID int64) (repository.GetLockedSaleEarningsRow, error)
	GetNextHoldRelease(ctx context.Context, sellerID int64) (*time.Time, error)
}

type WalletService struct {
	store WalletStore
}

func NewWalletService(store WalletStore) *WalletService {
	return &WalletService{store: store}
}

// Wallet devolve o saldo, as últimas movimentações e o valor bloqueado em
// vendas que ainda estão na proteção de troca.
func (s *WalletService) Wallet(ctx context.Context, userID int64, limit int) (domain.Wallet, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	u, err := s.store.GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Wallet{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("get user: %w", err)
	}

	rows, err := s.store.ListWalletEntries(ctx, repository.ListWalletEntriesParams{UserID: userID, Limit: int32(limit)})
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("list wallet entries: %w", err)
	}
	entries := make([]domain.WalletEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, toDomainEntry(r))
	}
	w := domain.Wallet{BalanceCents: u.BalanceCents, Entries: entries}

	locked, err := s.store.GetLockedSaleEarnings(ctx, userID)
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("locked earnings: %w", err)
	}
	w.LockedCents, w.LockedSales = locked.LockedCents, locked.LockedSales
	if locked.LockedSales > 0 {
		if w.NextReleaseAt, err = s.store.GetNextHoldRelease(ctx, userID); err != nil {
			return domain.Wallet{}, fmt.Errorf("next hold release: %w", err)
		}
	}
	return w, nil
}

func toDomainEntry(e repository.WalletEntry) domain.WalletEntry {
	return domain.WalletEntry{
		ID:                e.ID,
		UserID:            e.UserID,
		AmountCents:       e.AmountCents,
		BalanceAfterCents: e.BalanceAfterCents,
		Kind:              domain.WalletEntryKind(e.Kind),
		TransactionID:     e.TransactionID,
		Description:       e.Description,
		CreatedAt:         e.CreatedAt,
	}
}
