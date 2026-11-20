package handler

import (
	"testing"
	"time"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
)

func TestTransactionResponseHidesTradeURLAndStaleDeadline(t *testing.T) {
	const buyer, seller = 1, 2
	url := "https://steamcommunity.com/tradeoffer/new/?partner=1&token=abcdefgh"
	deadline := time.Now()

	tests := []struct {
		status       domain.TxStatus
		viewer       int64
		wantURL      bool
		wantDeadline bool
	}{
		{domain.TxPending, seller, true, true}, // vendedor precisa da URL para enviar
		{domain.TxPending, buyer, false, true}, // o comprador já sabe a própria URL
		{domain.TxSent, seller, false, true},   // oferta já enviada: não precisa mais
		{domain.TxOnHold, seller, false, false},
		{domain.TxFailed, seller, false, false},
	}
	for _, tt := range tests {
		tx := domain.Transaction{BuyerID: buyer, SellerID: seller, Status: tt.status, BuyerTradeURL: &url, DeadlineAt: &deadline}
		r := toTransactionResponse(tx, tt.viewer)
		if (r.BuyerTradeURL != nil) != tt.wantURL {
			t.Errorf("%s viewer %d: buyerTradeUrl shown = %v, want %v", tt.status, tt.viewer, r.BuyerTradeURL != nil, tt.wantURL)
		}
		if (r.DeadlineAt != nil) != tt.wantDeadline {
			t.Errorf("%s: deadline shown = %v, want %v", tt.status, r.DeadlineAt != nil, tt.wantDeadline)
		}
	}
}
