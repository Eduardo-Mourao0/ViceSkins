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

type PurchaseService interface {
	Buy(ctx context.Context, buyerID, listingID int64) (domain.Transaction, error)
	GetForUser(ctx context.Context, userID, txID int64) (domain.Transaction, error)
	CancelByBuyer(ctx context.Context, buyerID, txID int64) (domain.Transaction, error)
}

type TransactionHandler struct {
	svc PurchaseService
}

func NewTransactionHandler(svc PurchaseService) *TransactionHandler {
	return &TransactionHandler{svc: svc}
}

// POST /listings/{id}/buy
func (h *TransactionHandler) Buy(w http.ResponseWriter, r *http.Request) {
	listingID, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, _ := userIDFrom(r.Context())
	tx, err := h.svc.Buy(r.Context(), userID, listingID)
	if err != nil {
		writeTransactionErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTransactionResponse(tx, userID))
}

// GET /transactions/{id}
func (h *TransactionHandler) Get(w http.ResponseWriter, r *http.Request) {
	txID, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, _ := userIDFrom(r.Context())
	tx, err := h.svc.GetForUser(r.Context(), userID, txID)
	if err != nil {
		writeTransactionErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTransactionResponse(tx, userID))
}

// POST /transactions/{id}/cancel
func (h *TransactionHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	txID, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, _ := userIDFrom(r.Context())
	tx, err := h.svc.CancelByBuyer(r.Context(), userID, txID)
	if err != nil {
		writeTransactionErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTransactionResponse(tx, userID))
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func writeTransactionErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "only the buyer can do this")
	case errors.Is(err, domain.ErrCannotBuyOwnListing):
		writeError(w, http.StatusUnprocessableEntity, "you cannot buy your own listing")
	case errors.Is(err, domain.ErrInsufficientBalance):
		writeError(w, http.StatusUnprocessableEntity, "insufficient balance")
	case errors.Is(err, domain.ErrListingNotActive):
		writeError(w, http.StatusConflict, "listing is no longer available")
	case errors.Is(err, domain.ErrInvalidTransition):
		writeError(w, http.StatusConflict, err.Error())
	default:
		slog.Error("transaction", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

type transactionResponse struct {
	ID                  int64           `json:"id"`
	ListingID           int64           `json:"listingId"`
	ItemID              int64           `json:"itemId"`
	BuyerID             int64           `json:"buyerId"`
	SellerID            int64           `json:"sellerId"`
	Role                string          `json:"role"` // "buyer" ou "seller", do ponto de vista de quem pediu
	PriceCents          int64           `json:"priceCents"`
	FeeCents            int64           `json:"feeCents"`
	SellerReceivesCents int64           `json:"sellerReceivesCents"`
	Status              domain.TxStatus `json:"status"`
	TradeOfferID        *string         `json:"tradeOfferId"`
	HoldUntil           *time.Time      `json:"holdUntil"`
	FailureReason       *string         `json:"failureReason"`
	CreatedAt           time.Time       `json:"createdAt"`
	UpdatedAt           time.Time       `json:"updatedAt"`
}

func toTransactionResponse(t domain.Transaction, viewerID int64) transactionResponse {
	role := "buyer"
	if t.SellerID == viewerID {
		role = "seller"
	}
	return transactionResponse{
		ID:                  t.ID,
		ListingID:           t.ListingID,
		ItemID:              t.ItemID,
		BuyerID:             t.BuyerID,
		SellerID:            t.SellerID,
		Role:                role,
		PriceCents:          t.PriceCents,
		FeeCents:            t.FeeCents,
		SellerReceivesCents: t.SellerReceivesCents(),
		Status:              t.Status,
		TradeOfferID:        t.TradeOfferID,
		HoldUntil:           t.HoldUntil,
		FailureReason:       t.FailureReason,
		CreatedAt:           t.CreatedAt,
		UpdatedAt:           t.UpdatedAt,
	}
}
