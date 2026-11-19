package handler

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
)

type InventoryService interface {
	List(ctx context.Context, userID int64) (domain.Inventory, error)
	Refresh(ctx context.Context, userID int64) (domain.Inventory, error)
}

type InventoryHandler struct {
	svc InventoryService
}

func NewInventoryHandler(svc InventoryService) *InventoryHandler {
	return &InventoryHandler{svc: svc}
}

// GET /me/inventory
func (h *InventoryHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	inv, err := h.svc.List(r.Context(), userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toInventoryResponse(inv))
}

// POST /me/inventory/refresh
func (h *InventoryHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	inv, err := h.svc.Refresh(r.Context(), userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toInventoryResponse(inv))
}

func (h *InventoryHandler) writeErr(w http.ResponseWriter, err error) {
	var cd *domain.CooldownError
	switch {
	case errors.As(err, &cd):
		secs := int(math.Ceil(cd.RetryAfter.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":             "inventory refresh on cooldown",
			"retryAfterSeconds": secs,
		})
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusUnauthorized, "unauthenticated")
	case errors.Is(err, domain.ErrInventoryPrivate):
		writeError(w, http.StatusUnprocessableEntity, "your steam inventory is private")
	case errors.Is(err, domain.ErrUpstreamRateLimited):
		writeError(w, http.StatusServiceUnavailable, "steam is rate limiting requests, try again later")
	default:
		slog.Error("inventory", "err", err)
		writeError(w, http.StatusBadGateway, "could not load inventory")
	}
}

// ---- DTOs ----

type stickerResponse struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
}

type inventoryItemResponse struct {
	ID             int64             `json:"id"`
	AssetID        string            `json:"assetId"`
	Name           string            `json:"name"`
	MarketHashName string            `json:"marketHashName"`
	Type           string            `json:"type"`
	Weapon         string            `json:"weapon"`
	Rarity         string            `json:"rarity"`
	RarityColor    string            `json:"rarityColor"`
	Exterior       string            `json:"exterior"`
	Float          *float64          `json:"float"`     // null quando não se aplica
	PaintSeed      *int32            `json:"paintSeed"` // null quando não se aplica
	ImageURL       string            `json:"imageUrl"`
	Stickers       []stickerResponse `json:"stickers"`
	Tradable       bool              `json:"tradable"`
	Marketable     bool              `json:"marketable"`
}

type inventoryResponse struct {
	Items    []inventoryItemResponse `json:"items"`
	SyncedAt *time.Time              `json:"syncedAt"`
}

func toInventoryResponse(inv domain.Inventory) inventoryResponse {
	items := make([]inventoryItemResponse, 0, len(inv.Items))
	for _, it := range inv.Items {
		stickers := make([]stickerResponse, 0, len(it.Stickers))
		for _, s := range it.Stickers {
			stickers = append(stickers, stickerResponse(s))
		}
		items = append(items, inventoryItemResponse{
			ID:             it.ID,
			AssetID:        it.AssetID,
			Name:           it.Name,
			MarketHashName: it.MarketHashName,
			Type:           it.Type,
			Weapon:         it.Weapon,
			Rarity:         it.Rarity,
			RarityColor:    it.RarityColor,
			Exterior:       it.Exterior,
			Float:          it.Float,
			PaintSeed:      it.PaintSeed,
			ImageURL:       it.ImageURL,
			Stickers:       stickers,
			Tradable:       it.Tradable,
			Marketable:     it.Marketable,
		})
	}
	return inventoryResponse{Items: items, SyncedAt: inv.SyncedAt}
}
