package repository

// Arquivo escrito à mão (não é gerado pelo sqlc). Junta as queries geradas
// com operações que precisam de transação.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IsUniqueViolation diz se err veio de uma constraint UNIQUE específica
// (código 23505 do Postgres). Fica aqui para o service não depender de pgconn.
func IsUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// Store "embute" *Queries: todos os métodos gerados (GetUserByID, etc.)
// ficam disponíveis direto em Store, como se fossem dele.
type Store struct {
	*Queries
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Queries: New(pool), pool: pool}
}

// ExecTx roda fn dentro de uma transação. Se fn retornar erro, faz rollback.
func (s *Store) ExecTx(ctx context.Context, fn func(q *Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Depois de um Commit bem-sucedido, Rollback não faz nada.
	// Em qualquer saída antecipada (erro, panic), garante o rollback.
	defer tx.Rollback(ctx)

	if err := fn(s.Queries.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Deposit credita amount no saldo do usuário e registra no ledger, atomicamente.
func (s *Store) Deposit(ctx context.Context, userID, amountCents int64, description string) (WalletEntry, error) {
	var entry WalletEntry
	err := s.ExecTx(ctx, func(q *Queries) error {
		var err error
		entry, err = ApplyWalletEntry(ctx, q, userID, amountCents, "deposit", nil, description)
		return err
	})
	return entry, err
}

// ApplyWalletEntry é a ÚNICA forma de mexer no saldo: altera users.balance_cents
// e grava a linha do ledger com o saldo resultante. Precisa rodar dentro de uma
// transação (q vem de ExecTx), senão as duas escritas podem se separar.
func ApplyWalletEntry(ctx context.Context, q *Queries, userID, amountCents int64, kind string, txID *int64, description string) (WalletEntry, error) {
	balance, err := q.AddToBalance(ctx, AddToBalanceParams{UserID: userID, AmountCents: amountCents})
	if err != nil {
		return WalletEntry{}, fmt.Errorf("update balance: %w", err)
	}
	entry, err := q.InsertWalletEntry(ctx, InsertWalletEntryParams{
		UserID:            userID,
		AmountCents:       amountCents,
		BalanceAfterCents: balance,
		Kind:              kind,
		TransactionID:     txID,
		Description:       description,
	})
	if err != nil {
		return WalletEntry{}, fmt.Errorf("insert wallet entry: %w", err)
	}
	return entry, nil
}

// SyncInventory substitui o inventário em cache do usuário pelo que veio da Steam,
// tudo numa transação: ou atualiza tudo, ou nada.
func (s *Store) SyncInventory(ctx context.Context, ownerID int64, items []UpsertInventoryItemParams) error {
	return s.ExecTx(ctx, func(q *Queries) error {
		// Precisa ser slice vazio e não nil: nil vira NULL no Postgres,
		// e "asset_id = ANY(NULL)" não remove nada.
		present := make([]string, 0, len(items))
		for _, it := range items {
			it.OwnerID = ownerID
			if err := q.UpsertInventoryItem(ctx, it); err != nil {
				return fmt.Errorf("upsert asset %s: %w", it.AssetID, err)
			}
			present = append(present, it.AssetID)
		}

		if _, err := q.MarkMissingInventoryItemsRemoved(ctx, MarkMissingInventoryItemsRemovedParams{
			OwnerID:         ownerID,
			PresentAssetIDs: present,
		}); err != nil {
			return fmt.Errorf("mark removed items: %w", err)
		}

		if _, err := q.CancelInvalidListings(ctx, ownerID); err != nil {
			return fmt.Errorf("cancel invalid listings: %w", err)
		}

		return q.SetInventorySynced(ctx, ownerID)
	})
}
