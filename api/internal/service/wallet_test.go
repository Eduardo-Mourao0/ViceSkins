package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
)

type fakeWalletStore struct{ deposits int }

func (f *fakeWalletStore) GetUserByID(context.Context, int64) (repository.User, error) {
	return repository.User{}, nil
}

func (f *fakeWalletStore) Deposit(_ context.Context, userID, amount int64, _ string) (repository.WalletEntry, error) {
	f.deposits++
	return repository.WalletEntry{UserID: userID, AmountCents: amount, BalanceAfterCents: amount, Kind: "deposit"}, nil
}

func (f *fakeWalletStore) ListWalletEntries(context.Context, repository.ListWalletEntriesParams) ([]repository.WalletEntry, error) {
	return nil, nil
}

func TestDepositValidatesAmount(t *testing.T) {
	tests := []struct {
		amount int64
		ok     bool
	}{
		{domain.MinDepositCents, true},
		{domain.MaxDepositCents, true},
		{domain.MinDepositCents - 1, false},
		{domain.MaxDepositCents + 1, false},
		{0, false},
		{-500, false}, // depósito negativo seria um saque disfarçado
	}
	for _, tt := range tests {
		store := &fakeWalletStore{}
		_, err := NewWalletService(store).Deposit(context.Background(), 1, tt.amount)

		if tt.ok && err != nil {
			t.Errorf("amount %d: unexpected err %v", tt.amount, err)
		}
		if !tt.ok && (!errors.Is(err, domain.ErrInvalidInput) || store.deposits != 0) {
			t.Errorf("amount %d: err = %v, deposits = %d; want ErrInvalidInput and no deposit", tt.amount, err, store.deposits)
		}
	}
}
