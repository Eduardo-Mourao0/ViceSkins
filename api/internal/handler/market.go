package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
)

type MarketService interface {
	Search(ctx context.Context, q domain.ListingSearch) (domain.MarketPage, error)
	Get(ctx context.Context, id int64) (domain.MarketListing, error)
}

type MarketHandler struct {
	svc MarketService
}

func NewMarketHandler(svc MarketService) *MarketHandler {
	return &MarketHandler{svc: svc}
}

// GET /listings?q=redline&minPrice=1000&maxPrice=5000&wear=FN,MW&minFloat=0&maxFloat=0.1&sort=price_asc&page=1&pageSize=30
func (h *MarketHandler) Search(w http.ResponseWriter, r *http.Request) {
	q, err := parseListingSearch(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	page, err := h.svc.Search(r.Context(), q)
	if errors.Is(err, domain.ErrInvalidInput) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		slog.Error("search listings", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]marketListingResponse, 0, len(page.Listings))
	for _, l := range page.Listings {
		out = append(out, toMarketListingResponse(l))
	}
	writeJSON(w, http.StatusOK, marketPageResponse{
		Listings: out,
		Page:     page.Page,
		PageSize: page.PageSize,
		HasMore:  page.HasMore,
	})
}

// GET /listings/{id}
func (h *MarketHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid listing id")
		return
	}
	l, err := h.svc.Get(r.Context(), id)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		slog.Error("get listing", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toMarketListingResponse(l))
}

// parseListingSearch só converte texto → tipos. Regras (limites, valores válidos)
// ficam no service.
func parseListingSearch(v url.Values) (domain.ListingSearch, error) {
	q := domain.ListingSearch{
		Text: v.Get("q"),
		Sort: domain.ListingSort(v.Get("sort")),
	}
	var err error
	if q.MinPrice, err = optInt64(v, "minPrice"); err != nil {
		return q, err
	}
	if q.MaxPrice, err = optInt64(v, "maxPrice"); err != nil {
		return q, err
	}
	if q.MinFloat, err = optFloat(v, "minFloat"); err != nil {
		return q, err
	}
	if q.MaxFloat, err = optFloat(v, "maxFloat"); err != nil {
		return q, err
	}
	if w := v.Get("wear"); w != "" {
		q.Wear = strings.Split(w, ",")
	}
	if q.Page, err = optInt(v, "page"); err != nil {
		return q, err
	}
	if q.PageSize, err = optInt(v, "pageSize"); err != nil {
		return q, err
	}
	return q, nil
}

func optInt64(v url.Values, key string) (*int64, error) {
	s := v.Get(key)
	if s == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, errors.New(key + " must be an integer (cents)")
	}
	return &n, nil
}

func optFloat(v url.Values, key string) (*float64, error) {
	s := v.Get(key)
	if s == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, errors.New(key + " must be a number")
	}
	return &f, nil
}

func optInt(v url.Values, key string) (int, error) {
	s := v.Get(key)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, errors.New(key + " must be an integer")
	}
	return n, nil
}

// ---- DTOs ----

type publicUserResponse struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}

type marketListingResponse struct {
	listingResponse                    // embedding: os campos de listingResponse sobem para o mesmo nível no JSON
	Seller          publicUserResponse `json:"seller"`
}

type marketPageResponse struct {
	Listings []marketListingResponse `json:"listings"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"pageSize"`
	HasMore  bool                    `json:"hasMore"`
}

func toMarketListingResponse(l domain.MarketListing) marketListingResponse {
	return marketListingResponse{
		listingResponse: toListingResponse(l.Listing),
		Seller:          toPublicUserResponse(l.Seller),
	}
}

func toPublicUserResponse(u domain.PublicUser) publicUserResponse {
	return publicUserResponse{ID: u.ID, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL}
}
