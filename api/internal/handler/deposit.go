package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
)

type DepositService interface {
	Create(ctx context.Context, userID, amountCents int64) (domain.Deposit, error)
	Get(ctx context.Context, userID, id int64) (domain.Deposit, error)
	Open(ctx context.Context, userID int64) (*domain.Deposit, error)
	SimulatePayment(ctx context.Context, userID, id int64) (domain.Deposit, error)
	CanSimulate() bool
}

type DepositHandler struct {
	svc DepositService
}

func NewDepositHandler(svc DepositService) *DepositHandler {
	return &DepositHandler{svc: svc}
}

// POST /wallet/deposits {"amountCents": 5000} → cobrança Pix
func (h *DepositHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AmountCents int64 `json:"amountCents"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	userID, _ := userIDFrom(r.Context())
	d, err := h.svc.Create(r.Context(), userID, req.AmountCents)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, h.toResponse(d))
}

// GET /wallet/deposits/open → o Pix em aberto, ou 204 se não houver
func (h *DepositHandler) Open(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	d, err := h.svc.Open(r.Context(), userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	if d == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, h.toResponse(*d))
}

// GET /wallet/deposits/{id}
func (h *DepositHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, _ := userIDFrom(r.Context())
	d, err := h.svc.Get(r.Context(), userID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toResponse(d))
}

// POST /wallet/deposits/{id}/simulate-payment (só com o provedor simulado)
func (h *DepositHandler) SimulatePayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, _ := userIDFrom(r.Context())
	d, err := h.svc.SimulatePayment(r.Context(), userID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toResponse(d))
}

func (h *DepositHandler) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "deposit not found")
	case errors.Is(err, domain.ErrTooManyOpenDeposits):
		writeError(w, http.StatusConflict, "too many open pix deposits, pay or wait for one to expire")
	case errors.Is(err, domain.ErrDepositExpired):
		writeError(w, http.StatusConflict, "this pix has expired")
	case errors.Is(err, domain.ErrFeatureDisabled):
		writeError(w, http.StatusNotFound, "payment simulation is not available")
	default:
		slog.Error("deposit", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

type depositResponse struct {
	ID          int64                `json:"id"`
	AmountCents int64                `json:"amountCents"`
	Status      domain.DepositStatus `json:"status"`
	BRCode      string               `json:"brCode"`
	ExpiresAt   time.Time            `json:"expiresAt"`
	PaidAt      *time.Time           `json:"paidAt"`
	CreatedAt   time.Time            `json:"createdAt"`
	CanSimulate bool                 `json:"canSimulate"`
}

func (h *DepositHandler) toResponse(d domain.Deposit) depositResponse {
	return depositResponse{
		ID:          d.ID,
		AmountCents: d.AmountCents,
		Status:      d.Status,
		BRCode:      d.BRCode,
		ExpiresAt:   d.ExpiresAt,
		PaidAt:      d.PaidAt,
		CreatedAt:   d.CreatedAt,
		CanSimulate: h.svc.CanSimulate() && d.Status == domain.DepositPending,
	}
}
