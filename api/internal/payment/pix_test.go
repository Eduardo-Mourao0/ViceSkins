package payment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCRC16(t *testing.T) {
	// Vetor de referência do CRC-16/CCITT-FALSE.
	if got := crc16("123456789"); got != 0x29B1 {
		t.Fatalf("crc16 = %04X, want 29B1", got)
	}
}

func TestBRCode(t *testing.T) {
	code := BRCode{
		Key: "pagamento@viceskins.invalid", MerchantName: "VICESKINS DEMO", MerchantCity: "SAO PAULO",
		AmountCents: 15050, TxID: "VSDEP42",
	}.String()

	for _, want := range []string{
		"000201",
		"0014br.gov.bcb.pix0127pagamento@viceskins.invalid",
		"5406150.50", // valor com ponto e 2 casas
		"5802BR",
		"5914VICESKINS DEMO",
		"62110507VSDEP42",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("missing %q in %s", want, code)
		}
	}

	// Os 4 últimos caracteres são o CRC de todo o resto, "6304" incluído.
	body, crc := code[:len(code)-4], code[len(code)-4:]
	if !strings.HasSuffix(body, "6304") {
		t.Fatalf("CRC field missing: %s", code)
	}
	if want := fmt.Sprintf("%04X", crc16(body)); crc != want {
		t.Fatalf("crc = %s, want %s", crc, want)
	}
}

func TestSimulatedLifecycle(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	s := NewSimulated(WithClock(func() time.Time { return now }))

	c, err := s.CreateCharge(ctx, ChargeRequest{Reference: "VSDEP1", AmountCents: 1000, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := s.ChargeStatus(ctx, c.ID); st != StatusPending {
		t.Fatalf("status = %v, want pending", st)
	}
	if err := s.SimulatePayment(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.ChargeStatus(ctx, c.ID); st != StatusPaid {
		t.Fatalf("status = %v, want paid", st)
	}

	expired, _ := s.CreateCharge(ctx, ChargeRequest{Reference: "VSDEP2", AmountCents: 1000, ExpiresAt: now.Add(time.Minute)})
	now = now.Add(2 * time.Minute)
	if err := s.SimulatePayment(ctx, expired.ID); !errors.Is(err, ErrChargeExpired) {
		t.Fatalf("err = %v, want ErrChargeExpired", err)
	}
	if st, _ := s.ChargeStatus(ctx, expired.ID); st != StatusExpired {
		t.Fatalf("status = %v, want expired", st)
	}
	if st, _ := s.ChargeStatus(ctx, "nope"); st != StatusUnknown {
		t.Fatalf("unknown charge status = %v", st)
	}
}
