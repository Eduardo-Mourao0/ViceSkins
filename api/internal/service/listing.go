package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
)

const constraintOneActivePerItem = "listings_one_active_per_item"

type ListingStore interface {
	GetUserByID(ctx context.Context, id int64) (repository.User, error)
	GetInventoryItem(ctx context.Context, id int64) (repository.InventoryItem, error)
	CreateListing(ctx context.Context, arg repository.CreateListingParams) (repository.Listing, error)
	GetListing(ctx context.Context, id int64) (repository.Listing, error)
	CancelListing(ctx context.Context, arg repository.CancelListingParams) (repository.Listing, error)
	ListListingsBySeller(ctx context.Context, arg repository.ListListingsBySellerParams) ([]repository.ListListingsBySellerRow, error)
}

type ListingService struct {
	store                     ListingStore
	requireAPIKeyForFloatless bool
}

// ListingOption configura o ListingService (functional options, como as da Transition).
type ListingOption func(*ListingService)

// RequireAPIKeyForFloatless: com a verificação real pela Steam, itens sem
// float (caixas, stickers...) não têm impressão digital para serem
// reconhecidos no inventário do comprador. Só dá para acompanhar a troca
// pela oferta, o que exige a API key do vendedor.
func RequireAPIKeyForFloatless() ListingOption {
	return func(s *ListingService) { s.requireAPIKeyForFloatless = true }
}

func NewListingService(store ListingStore, opts ...ListingOption) *ListingService {
	s := &ListingService{store: store}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Create anuncia um item do inventário do vendedor.
func (s *ListingService) Create(ctx context.Context, sellerID, itemID, priceCents int64) (domain.Listing, error) {
	if priceCents < domain.MinListingPriceCents || priceCents > domain.MaxListingPriceCents {
		return domain.Listing{}, fmt.Errorf("%w: price must be between %d and %d cents",
			domain.ErrInvalidInput, domain.MinListingPriceCents, domain.MaxListingPriceCents)
	}

	// Sem trade URL o vendedor não recebe devoluções, e o comprador não tem
	// como confirmar a conta dele: anunciar exige a URL cadastrada.
	u, err := s.store.GetUserByID(ctx, sellerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("get seller: %w", err)
	}
	if u.TradeURL == nil {
		return domain.Listing{}, domain.ErrTradeURLRequired
	}

	item, err := s.store.GetInventoryItem(ctx, itemID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("get item: %w", err)
	}
	// Item de outra pessoa ou que já saiu do inventário: responde como "não existe",
	// para não revelar nada sobre itens alheios.
	if item.OwnerID != sellerID || item.RemovedAt != nil {
		return domain.Listing{}, domain.ErrNotFound
	}
	if !item.Tradable || !item.Marketable {
		return domain.Listing{}, domain.ErrItemNotTradable
	}
	if s.requireAPIKeyForFloatless && item.FloatValue == nil && len(u.SteamAPIKeyEnc) == 0 {
		return domain.Listing{}, domain.ErrAPIKeyRequired
	}

	l, err := s.store.CreateListing(ctx, repository.CreateListingParams{
		ItemID:     itemID,
		SellerID:   sellerID,
		PriceCents: priceCents,
	})
	// Não checamos "já está listado?" antes do INSERT: entre a checagem e o insert
	// outra requisição poderia passar. Deixamos o índice único decidir.
	if repository.IsUniqueViolation(err, constraintOneActivePerItem) {
		return domain.Listing{}, domain.ErrItemAlreadyListed
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("create listing: %w", err)
	}
	return toDomainListing(l), nil
}

// Cancel cancela uma listing ativa do próprio vendedor.
func (s *ListingService) Cancel(ctx context.Context, sellerID, listingID int64) (domain.Listing, error) {
	l, err := s.store.CancelListing(ctx, repository.CancelListingParams{ID: listingID, SellerID: sellerID})
	if err == nil {
		return toDomainListing(l), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Listing{}, fmt.Errorf("cancel listing: %w", err)
	}

	// O UPDATE não pegou nenhuma linha. Descobre o motivo para dar o erro certo.
	cur, err := s.store.GetListing(ctx, listingID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.Listing{}, domain.ErrNotFound
	case err != nil:
		return domain.Listing{}, fmt.Errorf("get listing: %w", err)
	case cur.SellerID != sellerID:
		return domain.Listing{}, domain.ErrForbidden
	default:
		return domain.Listing{}, fmt.Errorf("%w: status is %s", domain.ErrListingNotActive, cur.Status)
	}
}

// ListBySeller lista as listings do vendedor; status vazio = todas.
func (s *ListingService) ListBySeller(ctx context.Context, sellerID int64, status domain.ListingStatus) ([]domain.Listing, error) {
	var statusFilter *string
	if status != "" {
		if !status.Valid() {
			return nil, fmt.Errorf("%w: unknown status %q", domain.ErrInvalidInput, status)
		}
		st := string(status)
		statusFilter = &st
	}

	rows, err := s.store.ListListingsBySeller(ctx, repository.ListListingsBySellerParams{
		SellerID: sellerID,
		Status:   statusFilter,
	})
	if err != nil {
		return nil, fmt.Errorf("list listings: %w", err)
	}

	out := make([]domain.Listing, 0, len(rows))
	for _, r := range rows {
		item, err := toDomainItem(r.InventoryItem)
		if err != nil {
			return nil, err
		}
		l := toDomainListing(r.Listing)
		l.Item = &item
		out = append(out, l)
	}
	return out, nil
}

func toDomainListing(l repository.Listing) domain.Listing {
	return domain.Listing{
		ID:         l.ID,
		ItemID:     l.ItemID,
		SellerID:   l.SellerID,
		PriceCents: l.PriceCents,
		Status:     domain.ListingStatus(l.Status),
		CreatedAt:  l.CreatedAt,
		UpdatedAt:  l.UpdatedAt,
	}
}
