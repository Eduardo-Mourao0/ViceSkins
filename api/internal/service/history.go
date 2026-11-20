package service

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
)

type HistoryStore interface {
	ListUserTransactions(ctx context.Context, arg repository.ListUserTransactionsParams) ([]repository.ListUserTransactionsRow, error)
	GetTransactionDetail(ctx context.Context, id int64) (repository.GetTransactionDetailRow, error)
	GetUserTransactionSummary(ctx context.Context, userID int64) (repository.GetUserTransactionSummaryRow, error)
}

type HistoryService struct {
	store HistoryStore
}

func NewHistoryService(store HistoryStore) *HistoryService {
	return &HistoryService{store: store}
}

const (
	defaultHistoryLimit = 20
	maxHistoryLimit     = 50
)

// List devolve as transações do usuário (comprador ou vendedor), mais novas primeiro.
func (s *HistoryService) List(ctx context.Context, userID int64, f domain.TxHistoryFilter) (domain.TxHistoryPage, error) {
	params, limit, err := buildHistoryParams(userID, f)
	if err != nil {
		return domain.TxHistoryPage{}, err
	}

	rows, err := s.store.ListUserTransactions(ctx, params)
	if err != nil {
		return domain.TxHistoryPage{}, fmt.Errorf("list transactions: %w", err)
	}

	// Pedimos limit+1: se veio o extra, existe próxima página.
	var next *int64
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1].Transaction.ID
		next = &last
	}

	out := make([]domain.TransactionDetail, 0, len(rows))
	for _, r := range rows {
		d, err := toTransactionDetail(r.Transaction, r.InventoryItem, r.BuyerName, r.BuyerAvatarURL, r.SellerName, r.SellerAvatarURL)
		if err != nil {
			return domain.TxHistoryPage{}, err
		}
		out = append(out, d)
	}
	return domain.TxHistoryPage{Transactions: out, NextCursor: next}, nil
}

// Get devolve o detalhe se o usuário for comprador ou vendedor; senão, ErrNotFound.
func (s *HistoryService) Get(ctx context.Context, userID, txID int64) (domain.TransactionDetail, error) {
	r, err := s.store.GetTransactionDetail(ctx, txID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TransactionDetail{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.TransactionDetail{}, fmt.Errorf("get transaction detail: %w", err)
	}
	if r.Transaction.BuyerID != userID && r.Transaction.SellerID != userID {
		return domain.TransactionDetail{}, domain.ErrNotFound // não revela transações alheias
	}
	return toTransactionDetail(r.Transaction, r.InventoryItem, r.BuyerName, r.BuyerAvatarURL, r.SellerName, r.SellerAvatarURL)
}

func (s *HistoryService) Summary(ctx context.Context, userID int64) (domain.TxSummary, error) {
	r, err := s.store.GetUserTransactionSummary(ctx, userID)
	if err != nil {
		return domain.TxSummary{}, fmt.Errorf("transaction summary: %w", err)
	}
	return domain.TxSummary(r), nil // mesmos campos, mesmos tipos: conversão direta
}

func buildHistoryParams(userID int64, f domain.TxHistoryFilter) (repository.ListUserTransactionsParams, int, error) {
	invalid := func(msg string) error { return fmt.Errorf("%w: %s", domain.ErrInvalidInput, msg) }

	var role *string
	switch f.Role {
	case "":
	case domain.RoleBuyer, domain.RoleSeller:
		r := string(f.Role)
		role = &r
	default:
		return repository.ListUserTransactionsParams{}, 0, invalid("role must be buyer or seller")
	}

	statuses := []string{} // vazio (não nil) = todos
	switch f.Status {
	case "":
	case domain.TxFilterOpen:
		statuses = toStrings(domain.OpenStatuses)
	case domain.TxFilterClosed:
		statuses = toStrings(domain.ClosedStatuses)
	default:
		st := domain.TxStatus(f.Status)
		if !isKnownStatus(st) {
			return repository.ListUserTransactionsParams{}, 0, invalid("unknown status " + f.Status)
		}
		statuses = []string{f.Status}
	}

	limit := f.Limit
	if limit < 1 {
		limit = defaultHistoryLimit
	}
	if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}

	var before *int64
	if f.Before < 0 {
		return repository.ListUserTransactionsParams{}, 0, invalid("before must be positive")
	}
	if f.Before > 0 {
		before = &f.Before
	}

	return repository.ListUserTransactionsParams{
		UserID:   userID,
		Role:     role,
		Statuses: statuses,
		BeforeID: before,
		MaxRows:  int32(limit + 1),
	}, limit, nil
}

// Não usa append(OpenStatuses, ClosedStatuses...): se a slice global tivesse
// capacidade sobrando, o append escreveria no array dela e a corromperia.
func isKnownStatus(s domain.TxStatus) bool {
	return slices.Contains(domain.OpenStatuses, s) || slices.Contains(domain.ClosedStatuses, s)
}

func toStrings(ss []domain.TxStatus) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = string(s)
	}
	return out
}

func toTransactionDetail(t repository.Transaction, it repository.InventoryItem,
	buyerName, buyerAvatar, sellerName, sellerAvatar string) (domain.TransactionDetail, error) {
	item, err := toDomainItem(it)
	if err != nil {
		return domain.TransactionDetail{}, err
	}
	return domain.TransactionDetail{
		Transaction: toDomainTransaction(t),
		Item:        item,
		Buyer:       domain.PublicUser{ID: t.BuyerID, DisplayName: buyerName, AvatarURL: buyerAvatar},
		Seller:      domain.PublicUser{ID: t.SellerID, DisplayName: sellerName, AvatarURL: sellerAvatar},
	}, nil
}
