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
	CancelByBuyer(ctx context.Context, buyerID, txID int64) (domain.Transaction, error)
	MarkSentBySeller(ctx context.Context, sellerID, txID int64) (domain.Transaction, error)
}

type HistoryService interface {
	List(ctx context.Context, userID int64, f domain.TxHistoryFilter) (domain.TxHistoryPage, error)
	Get(ctx context.Context, userID, txID int64) (domain.TransactionDetail, error)
	Summary(ctx context.Context, userID int64) (domain.TxSummary, error)
}

type TransactionHandler struct {
	svc     PurchaseService
	history HistoryService
}

func NewTransactionHandler(svc PurchaseService, history HistoryService) *TransactionHandler {
	return &TransactionHandler{svc: svc, history: history}
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
	h.writeDetail(w, r, txID, http.StatusOK)
}

// POST /transactions/{id}/cancel
func (h *TransactionHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	txID, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, _ := userIDFrom(r.Context())
	if _, err := h.svc.CancelByBuyer(r.Context(), userID, txID); err != nil {
		writeTransactionErr(w, err)
		return
	}
	h.writeDetail(w, r, txID, http.StatusOK)
}

// POST /transactions/{id}/sent: o vendedor avisa que enviou a oferta
func (h *TransactionHandler) MarkSent(w http.ResponseWriter, r *http.Request) {
	txID, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, _ := userIDFrom(r.Context())
	if _, err := h.svc.MarkSentBySeller(r.Context(), userID, txID); err != nil {
		writeTransactionErr(w, err)
		return
	}
	h.writeDetail(w, r, txID, http.StatusOK)
}

// GET /me/transactions?role=buyer|seller&status=open|closed|<status>&before=<id>&limit=20
func (h *TransactionHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := domain.TxHistoryFilter{Role: domain.TxRole(q.Get("role")), Status: q.Get("status")}
	var err error
	if f.Before, err = optInt64Value(q.Get("before")); err != nil {
		writeError(w, http.StatusBadRequest, "before must be an integer")
		return
	}
	limit, err := optInt64Value(q.Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "limit must be an integer")
		return
	}
	f.Limit = int(limit)

	userID, _ := userIDFrom(r.Context())
	page, err := h.history.List(r.Context(), userID, f)
	if err != nil {
		writeTransactionErr(w, err)
		return
	}
	out := make([]transactionDetailResponse, 0, len(page.Transactions))
	for _, t := range page.Transactions {
		out = append(out, toTransactionDetailResponse(t, userID))
	}
	writeJSON(w, http.StatusOK, map[string]any{"transactions": out, "nextCursor": page.NextCursor})
}

// GET /me/transactions/summary
func (h *TransactionHandler) Summary(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	s, err := h.history.Summary(r.Context(), userID)
	if err != nil {
		writeTransactionErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{
		"purchases":            s.Purchases,
		"spentCents":           s.SpentCents,
		"sales":                s.Sales,
		"earnedCents":          s.EarnedCents,
		"pendingEarningsCents": s.PendingEarningsCents,
		"feesPaidCents":        s.FeesPaidCents,
	})
}

func (h *TransactionHandler) writeDetail(w http.ResponseWriter, r *http.Request, txID int64, status int) {
	userID, _ := userIDFrom(r.Context())
	d, err := h.history.Get(r.Context(), userID, txID)
	if err != nil {
		writeTransactionErr(w, err)
		return
	}
	writeJSON(w, status, toTransactionDetailResponse(d, userID))
}

func optInt64Value(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
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
		writeError(w, http.StatusForbidden, "your role in this transaction cannot do this")
	case errors.Is(err, domain.ErrCannotBuyOwnListing):
		writeError(w, http.StatusUnprocessableEntity, "you cannot buy your own listing")
	case errors.Is(err, domain.ErrTradeURLRequired):
		writeError(w, http.StatusUnprocessableEntity, "set your steam trade url before buying")
	case errors.Is(err, domain.ErrInsufficientBalance):
		writeError(w, http.StatusUnprocessableEntity, "insufficient balance")
	case errors.Is(err, domain.ErrListingNotActive):
		writeError(w, http.StatusConflict, "listing is no longer available")
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrCannotCancelOffer):
		writeError(w, http.StatusConflict, "the seller already sent the trade offer: decline it on steam and the purchase ends at the deadline")
	case errors.Is(err, domain.ErrTradeUnverified):
		writeError(w, http.StatusServiceUnavailable, "could not confirm on steam that the item is still with the seller; try again in a few minutes")
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
	DeadlineAt          *time.Time      `json:"deadlineAt"`     // prazo da etapa atual (pending/sent)
	BuyerTradeURL       *string         `json:"buyerTradeUrl"`  // só para o vendedor, enquanto precisa enviar
	LateDeliveryAt      *time.Time      `json:"lateDeliveryAt"` // item chegou depois do estorno: em análise
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
		DeadlineAt:          activeDeadline(t),
		BuyerTradeURL:       tradeURLForViewer(t, viewerID),
		LateDeliveryAt:      t.LateDeliveryAt,
		CreatedAt:           t.CreatedAt,
		UpdatedAt:           t.UpdatedAt,
	}
}

type transactionDetailResponse struct {
	transactionResponse
	Item   inventoryItemResponse `json:"item"`
	Buyer  publicUserResponse    `json:"buyer"`
	Seller publicUserResponse    `json:"seller"`
}

func toTransactionDetailResponse(d domain.TransactionDetail, viewerID int64) transactionDetailResponse {
	return transactionDetailResponse{
		transactionResponse: toTransactionResponse(d.Transaction, viewerID),
		Item:                toInventoryItemResponse(d.Item),
		Buyer:               toPublicUserResponse(d.Buyer),
		Seller:              toPublicUserResponse(d.Seller),
	}
}

// O prazo só vale enquanto a transação espera alguém agir.
func activeDeadline(t domain.Transaction) *time.Time {
	if t.Status == domain.TxPending || t.Status == domain.TxSent {
		return t.DeadlineAt
	}
	return nil
}

// A trade URL do comprador é dado sensível (quem tem pode mandar ofertas para
// ele): só o vendedor vê, e só enquanto precisa dela para enviar o item.
func tradeURLForViewer(t domain.Transaction, viewerID int64) *string {
	if t.SellerID == viewerID && t.Status == domain.TxPending {
		return t.BuyerTradeURL
	}
	return nil
}
