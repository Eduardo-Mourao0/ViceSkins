package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
)

type DisputeService interface {
	ListDisputes(ctx context.Context, status string) ([]domain.DisputeView, error)
	DismissDispute(ctx context.Context, adminID, disputeID int64, note string) error
}

type AdminHandler struct {
	disputes DisputeService
}

func NewAdminHandler(disputes DisputeService) *AdminHandler {
	return &AdminHandler{disputes: disputes}
}

// GET /admin/disputes?status=holding|settled|reversed|dismissed
func (h *AdminHandler) ListDisputes(w http.ResponseWriter, r *http.Request) {
	list, err := h.disputes.ListDisputes(r.Context(), r.URL.Query().Get("status"))
	if errors.Is(err, domain.ErrInvalidInput) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		slog.Error("list disputes", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]disputeResponse, 0, len(list))
	for _, d := range list {
		out = append(out, toDisputeResponse(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"disputes": out})
}

// POST /admin/disputes/{id}/dismiss {"note": "..."}
func (h *AdminHandler) DismissDispute(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	adminID, _ := userIDFrom(r.Context())
	err := h.disputes.DismissDispute(r.Context(), adminID, id, req.Note)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, domain.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "dispute is already resolved")
	default:
		slog.Error("dismiss dispute", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

type disputeResponse struct {
	ID             int64      `json:"id"`
	Status         string     `json:"status"`
	DetectedAt     time.Time  `json:"detectedAt"`
	RefundedAt     time.Time  `json:"refundedAt"`
	Evidence       string     `json:"evidence"`
	TradedAt       *time.Time `json:"tradedAt"`
	ChargedCents   int64      `json:"chargedCents"`
	CoveredCents   int64      `json:"coveredCents"`
	HoldUntil      time.Time  `json:"holdUntil"`
	ResolvedAt     *time.Time `json:"resolvedAt"`
	ResolvedBy     *int64     `json:"resolvedBy"`
	Note           *string    `json:"note"`
	TransactionID  int64      `json:"transactionId"`
	PriceCents     int64      `json:"priceCents"`
	SellerReceives int64      `json:"sellerReceivesCents"`
	FailureReason  *string    `json:"failureReason"`
	ItemName       string     `json:"itemName"`
	ItemImageURL   string     `json:"itemImageUrl"`
	BuyerName      string     `json:"buyerName"`
	BuyerDebtCents int64      `json:"buyerDebtCents"`
	SellerName     string     `json:"sellerName"`
}

func toDisputeResponse(d domain.DisputeView) disputeResponse {
	return disputeResponse{
		ID:             d.ID,
		Status:         string(d.Status),
		DetectedAt:     d.DetectedAt,
		RefundedAt:     d.RefundedAt,
		Evidence:       d.Evidence,
		TradedAt:       d.TradedAt,
		ChargedCents:   d.ChargedCents,
		CoveredCents:   d.CoveredCents,
		HoldUntil:      d.HoldUntil,
		ResolvedAt:     d.ResolvedAt,
		ResolvedBy:     d.ResolvedBy,
		Note:           d.Note,
		TransactionID:  d.Transaction.ID,
		PriceCents:     d.Transaction.PriceCents,
		SellerReceives: d.Transaction.SellerReceivesCents(),
		FailureReason:  d.Transaction.FailureReason,
		ItemName:       d.ItemName,
		ItemImageURL:   d.ItemImageURL,
		BuyerName:      d.BuyerName,
		BuyerDebtCents: d.BuyerDebtCents,
		SellerName:     d.SellerName,
	}
}
