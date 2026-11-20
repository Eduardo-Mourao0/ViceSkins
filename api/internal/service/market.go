package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
)

type MarketStore interface {
	SearchActiveListings(ctx context.Context, arg repository.SearchActiveListingsParams) ([]repository.SearchActiveListingsRow, error)
	GetListingDetail(ctx context.Context, id int64) (repository.GetListingDetailRow, error)
}

type MarketService struct {
	store MarketStore
}

func NewMarketService(store MarketStore) *MarketService {
	return &MarketService{store: store}
}

// Search lista as listings ativas do mercado com filtros e paginação.
func (s *MarketService) Search(ctx context.Context, q domain.ListingSearch) (domain.MarketPage, error) {
	params, err := buildSearchParams(&q)
	if err != nil {
		return domain.MarketPage{}, err
	}

	rows, err := s.store.SearchActiveListings(ctx, params)
	if err != nil {
		return domain.MarketPage{}, fmt.Errorf("search listings: %w", err)
	}

	// Pedimos um item a mais que o tamanho da página: se ele vier, existe próxima página.
	// Evita um COUNT(*) separado.
	hasMore := len(rows) > q.PageSize
	if hasMore {
		rows = rows[:q.PageSize]
	}

	out := make([]domain.MarketListing, 0, len(rows))
	for _, r := range rows {
		ml, err := toMarketListing(r.Listing, r.InventoryItem, r.SellerName, r.SellerAvatarURL)
		if err != nil {
			return domain.MarketPage{}, err
		}
		out = append(out, ml)
	}
	return domain.MarketPage{Listings: out, Page: q.Page, PageSize: q.PageSize, HasMore: hasMore}, nil
}

// Get devolve uma listing com item e vendedor, em qualquer status.
func (s *MarketService) Get(ctx context.Context, id int64) (domain.MarketListing, error) {
	r, err := s.store.GetListingDetail(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MarketListing{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.MarketListing{}, fmt.Errorf("get listing detail: %w", err)
	}
	return toMarketListing(r.Listing, r.InventoryItem, r.SellerName, r.SellerAvatarURL)
}

// buildSearchParams valida os filtros, aplica os padrões (alterando q)
// e monta os parâmetros da query.
func buildSearchParams(q *domain.ListingSearch) (repository.SearchActiveListingsParams, error) {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", domain.ErrInvalidInput, fmt.Sprintf(format, args...))
	}

	if q.Sort == "" {
		q.Sort = domain.SortNewest
	}
	if !q.Sort.Valid() {
		return repository.SearchActiveListingsParams{}, invalid("unknown sort %q", q.Sort)
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = domain.DefaultPageSize
	}
	if q.PageSize > domain.MaxPageSize {
		q.PageSize = domain.MaxPageSize
	}

	if q.MinPrice != nil && q.MaxPrice != nil && *q.MinPrice > *q.MaxPrice {
		return repository.SearchActiveListingsParams{}, invalid("minPrice is greater than maxPrice")
	}
	for _, f := range []*float64{q.MinFloat, q.MaxFloat} {
		if f != nil && (*f < 0 || *f > 1) {
			return repository.SearchActiveListingsParams{}, invalid("float must be between 0 and 1")
		}
	}
	if q.MinFloat != nil && q.MaxFloat != nil && *q.MinFloat > *q.MaxFloat {
		return repository.SearchActiveListingsParams{}, invalid("minFloat is greater than maxFloat")
	}

	exteriors := make([]string, 0, len(q.Wear)) // vazio, não nil: nil vira NULL no Postgres
	for _, w := range q.Wear {
		ext, ok := domain.Wear[strings.ToUpper(w)]
		if !ok {
			return repository.SearchActiveListingsParams{}, invalid("unknown wear %q", w)
		}
		exteriors = append(exteriors, ext)
	}

	var search *string
	if t := strings.TrimSpace(q.Text); t != "" {
		if len(t) > 100 {
			return repository.SearchActiveListingsParams{}, invalid("search text too long")
		}
		escaped := escapeLike(t)
		search = &escaped
	}

	return repository.SearchActiveListingsParams{
		Search:     search,
		MinPrice:   q.MinPrice,
		MaxPrice:   q.MaxPrice,
		Exteriors:  exteriors,
		MinFloat:   q.MinFloat,
		MaxFloat:   q.MaxFloat,
		Sort:       string(q.Sort),
		PageLimit:  int32(q.PageSize + 1), // +1 para descobrir se há próxima página
		PageOffset: int32((q.Page - 1) * q.PageSize),
	}, nil
}

// escapeLike faz %, _ e \ do usuário valerem como texto literal no ILIKE.
// Sem isso, buscar "100%" ou "_" viraria curinga.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func toMarketListing(l repository.Listing, it repository.InventoryItem, sellerName, sellerAvatar string) (domain.MarketListing, error) {
	item, err := toDomainItem(it)
	if err != nil {
		return domain.MarketListing{}, err
	}
	dl := toDomainListing(l)
	dl.Item = &item
	return domain.MarketListing{
		Listing: dl,
		Seller:  domain.PublicUser{ID: l.SellerID, DisplayName: sellerName, AvatarURL: sellerAvatar},
	}, nil
}
