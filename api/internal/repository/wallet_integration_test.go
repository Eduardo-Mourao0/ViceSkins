package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/testdb"
)

func TestConcurrentDepositsKeepLedgerConsistent(t *testing.T) {
	pool := testdb.New(t)
	store := repository.NewStore(pool)
	userID := testdb.NewUser(t, pool)
	ctx := context.Background()

	const n, amount = 50, int64(100)

	// 50 depósitos ao mesmo tempo no mesmo usuário.
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.Deposit(ctx, userID, amount, "test"); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("deposit failed: %v", err)
	}

	audit, err := store.GetWalletAudit(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if audit.BalanceCents != n*amount || audit.LedgerSumCents != n*amount || audit.EntryCount != n {
		t.Fatalf("audit = %+v, want balance = ledger = %d with %d entries", audit, n*amount, n)
	}

	// Cada depósito viu o saldo deixado pelo anterior: os balance_after são
	// exatamente 100, 200, ..., 5000, sem repetição (nenhuma atualização perdida).
	entries, err := store.ListWalletEntries(ctx, repository.ListWalletEntriesParams{UserID: userID, Limit: n})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, e := range entries {
		seen[e.BalanceAfterCents] = true
	}
	for i := int64(1); i <= n; i++ {
		if !seen[i*amount] {
			t.Fatalf("missing balance_after %d: lost update?", i*amount)
		}
	}
}

func TestBalanceNeverGoesNegativeAndFailedEntryRollsBack(t *testing.T) {
	pool := testdb.New(t)
	store := repository.NewStore(pool)
	userID := testdb.NewUser(t, pool)
	ctx := context.Background()

	if _, err := store.Deposit(ctx, userID, 500, "test"); err != nil {
		t.Fatal(err)
	}

	// Débito maior que o saldo: o CHECK (balance_cents >= 0) do banco barra.
	_, err := store.Deposit(ctx, userID, -800, "test")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" { // check_violation
		t.Fatalf("err = %v, want check violation", err)
	}

	// Nada da operação que falhou ficou gravado: nem saldo, nem linha no ledger.
	audit, err := store.GetWalletAudit(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if audit.BalanceCents != 500 || audit.LedgerSumCents != 500 || audit.EntryCount != 1 {
		t.Fatalf("audit = %+v, want 500/500 with 1 entry", audit)
	}
}
