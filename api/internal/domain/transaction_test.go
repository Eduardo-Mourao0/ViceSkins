package domain

import "testing"

func TestTxStatusTransitions(t *testing.T) {
	all := []TxStatus{TxPending, TxSent, TxOnHold, TxCompleted, TxCancelled, TxFailed}

	// Tabela completa: todos os pares (de, para). Só estes podem ser true.
	valid := map[[2]TxStatus]bool{
		{TxPending, TxSent}:      true,
		{TxPending, TxOnHold}:    true,
		{TxPending, TxCancelled}: true,
		{TxPending, TxFailed}:    true,
		{TxSent, TxOnHold}:       true,
		{TxSent, TxCancelled}:    true,
		{TxSent, TxFailed}:       true,
		{TxOnHold, TxCompleted}:  true,
		{TxOnHold, TxFailed}:     true,
	}

	for _, from := range all {
		for _, to := range all {
			got := from.CanTransitionTo(to)
			if want := valid[[2]TxStatus{from, to}]; got != want {
				t.Errorf("%s → %s: got %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestFinalStatuses(t *testing.T) {
	for _, s := range []TxStatus{TxCompleted, TxCancelled, TxFailed} {
		if !s.IsFinal() {
			t.Errorf("%s should be final", s)
		}
	}
	for _, s := range []TxStatus{TxPending, TxSent, TxOnHold} {
		if s.IsFinal() {
			t.Errorf("%s should not be final", s)
		}
	}
}

func TestCalcFee(t *testing.T) {
	tests := []struct{ price, bps, want int64 }{
		{10_000, 200, 200}, // R$ 100,00 a 2% = R$ 2,00
		{1_550, 200, 31},   // R$ 15,50 a 2% = 31 centavos
		{49, 200, 0},       // 0,98 centavo arredonda para baixo
		{10_000, 0, 0},
	}
	for _, tt := range tests {
		if got := CalcFee(tt.price, tt.bps); got != tt.want {
			t.Errorf("CalcFee(%d, %d) = %d, want %d", tt.price, tt.bps, got, tt.want)
		}
	}
}
