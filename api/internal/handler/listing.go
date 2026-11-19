package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
)

type ListingService interface {
	Create(ctx context.Context, sellerID, itemID, priceCents int64) (domain.Listing, error)
	Cancel(ctx context.Context, sellerID, listingID int64) (domain.Listing, error)
	ListBySeller(ctx context.Context, sellerID int64, status domain.ListingStatus) ([]domain.Listing, error)
}

type ListingHandler struct {
	svc ListingService
}

func NewListingHandler(svc ListingService) *ListingHandler {
	return &ListingHandler{svc: svc}
}

type createListingRequest struct {
	ItemID     int64 `json:"itemId"`
	PriceCents int64 `json:"priceCents"`
}

// POST /listings
func (h *ListingHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createListingRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ItemID <= 0 {
		writeError(w, http.StatusBadRequest, "itemId is required")
		return
	}

	userID, _ := userIDFrom(r.Context())
	l, err := h.svc.Create(r.Context(), userID, req.ItemID, req.PriceCents)
	if err != nil {
		writeListingErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toListingResponse(l))
}

// POST /listings/{id}/cancel
func (h *ListingHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid listing id")
		return
	}

	userID, _ := userIDFrom(r.Context())
	l, err := h.svc.Cancel(r.Context(), userID, id)
	if err != nil {
		writeListingErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toListingResponse(l))
}

// GET /me/listings?status=active
func (h *ListingHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	status := domain.ListingStatus(r.URL.Query().Get("status"))

	listings, err := h.svc.ListBySeller(r.Context(), userID, status)
	if err != nil {
		writeListingErr(w, err)
		return
	}

	out := make([]listingResponse, 0, len(listings))
	for _, l := range listings {
		out = append(out, toListingResponse(l))
	}
	writeJSON(w, http.StatusOK, map[string]any{"listings": out})
}

func writeListingErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "this listing is not yours")
	case errors.Is(err, domain.ErrItemNotTradable):
		writeError(w, http.StatusUnprocessableEntity, "item is not tradable")
	case errors.Is(err, domain.ErrItemAlreadyListed):
		writeError(w, http.StatusConflict, "item already has an active listing")
	case errors.Is(err, domain.ErrListingNotActive):
		writeError(w, http.StatusConflict, "listing is not active")
	default:
		slog.Error("listing", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// ---- DTOs ----

type listingResponse struct {
	ID         int64                  `json:"id"`
	ItemID     int64                  `json:"itemId"`
	PriceCents int64                  `json:"priceCents"`
	Status     domain.ListingStatus   `json:"status"`
	CreatedAt  time.Time              `json:"createdAt"`
	UpdatedAt  time.Time              `json:"updatedAt"`
	Item       *inventoryItemResponse `json:"item,omitempty"` // omitempty: some do JSON quando nil
}

func toListingResponse(l domain.Listing) listingResponse {
	resp := listingResponse{
		ID:         l.ID,
		ItemID:     l.ItemID,
		PriceCents: l.PriceCents,
		Status:     l.Status,
		CreatedAt:  l.CreatedAt,
		UpdatedAt:  l.UpdatedAt,
	}
	if l.Item != nil {
		item := toInventoryItemResponse(*l.Item)
		resp.Item = &item
	}
	return resp
}
