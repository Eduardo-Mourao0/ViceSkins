package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
)

type WalletService interface {
	Wallet(ctx context.Context, userID int64, limit int) (domain.Wallet, error)
}

type WalletHandler struct {
	svc WalletService
}

func NewWalletHandler(svc WalletService) *WalletHandler {
	return &WalletHandler{svc: svc}
}

// GET /me/wallet?limit=50
func (h *WalletHandler) Get(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit")) // inválido vira 0 → service usa o padrão
	userID, _ := userIDFrom(r.Context())

	wallet, err := h.svc.Wallet(r.Context(), userID, limit)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if err != nil {
		slog.Error("wallet", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	entries := make([]walletEntryResponse, 0, len(wallet.Entries))
	for _, e := range wallet.Entries {
		entries = append(entries, toWalletEntryResponse(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"balanceCents": wallet.BalanceCents,
		"entries":      entries,
		// Vendas em proteção de troca: ainda não estão no saldo.
		"lockedCents":   wallet.LockedCents,
		"lockedSales":   wallet.LockedSales,
		"nextReleaseAt": wallet.NextReleaseAt,
	})
}

type walletEntryResponse struct {
	ID                int64                  `json:"id"`
	AmountCents       int64                  `json:"amountCents"`
	BalanceAfterCents int64                  `json:"balanceAfterCents"`
	Kind              domain.WalletEntryKind `json:"kind"`
	TransactionID     *int64                 `json:"transactionId"`
	Description       string                 `json:"description"`
	CreatedAt         time.Time              `json:"createdAt"`
}

func toWalletEntryResponse(e domain.WalletEntry) walletEntryResponse {
	return walletEntryResponse{
		ID:                e.ID,
		AmountCents:       e.AmountCents,
		BalanceAfterCents: e.BalanceAfterCents,
		Kind:              e.Kind,
		TransactionID:     e.TransactionID,
		Description:       e.Description,
		CreatedAt:         e.CreatedAt,
	}
}
