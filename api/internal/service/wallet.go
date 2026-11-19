package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
)

type WalletStore interface {
	GetUserByID(ctx context.Context, id int64) (repository.User, error)
	Deposit(ctx context.Context, userID, amountCents int64, description string) (repository.WalletEntry, error)
	ListWalletEntries(ctx context.Context, arg repository.ListWalletEntriesParams) ([]repository.WalletEntry, error)
}

type WalletService struct {
	store WalletStore
}

func NewWalletService(store WalletStore) *WalletService {
	return &WalletService{store: store}
}

// Deposit adiciona saldo fake (não há gateway de pagamento no MVP).
func (s *WalletService) Deposit(ctx context.Context, userID, amountCents int64) (domain.WalletEntry, error) {
	if amountCents < domain.MinDepositCents || amountCents > domain.MaxDepositCents {
		return domain.WalletEntry{}, fmt.Errorf("%w: deposit must be between %d and %d cents",
			domain.ErrInvalidInput, domain.MinDepositCents, domain.MaxDepositCents)
	}

	e, err := s.store.Deposit(ctx, userID, amountCents, "Depósito (simulado)")
	if errors.Is(err, pgx.ErrNoRows) { // AddToBalance não achou o usuário
		return domain.WalletEntry{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.WalletEntry{}, fmt.Errorf("deposit: %w", err)
	}
	return toDomainEntry(e), nil
}

// Wallet devolve o saldo e as últimas movimentações.
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
	return domain.Wallet{BalanceCents: u.BalanceCents, Entries: entries}, nil
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
