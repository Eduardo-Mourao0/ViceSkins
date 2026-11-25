package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// demoPixKey é uma chave Pix que não pode existir: o domínio .invalid é
// reservado e nunca resolve. Se alguém ler o QR no app do banco, o banco
// responde "chave não encontrada" e nenhum dinheiro sai.
const demoPixKey = "pagamento@viceskins.invalid"

// Simulated é um provedor Pix em memória. O "pagamento" acontece quando o
// usuário clica em simular (SimulatePayment), no lugar do app do banco.
type Simulated struct {
	now func() time.Time

	mu      sync.Mutex
	charges map[string]*simCharge
}

type simCharge struct {
	expiresAt time.Time
	paid      bool
}

// SimOption ajusta o provedor simulado (nos testes, o relógio).
type SimOption func(*Simulated)

func WithClock(now func() time.Time) SimOption {
	return func(s *Simulated) { s.now = now }
}

func NewSimulated(opts ...SimOption) *Simulated {
	s := &Simulated{now: time.Now, charges: map[string]*simCharge{}}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Simulated) Name() string { return "simulated" }

func (s *Simulated) CreateCharge(_ context.Context, req ChargeRequest) (Charge, error) {
	// ID aleatório, não um contador: depois de reiniciar a API um contador
	// recomeçaria do 1 e colidiria com cobranças antigas no banco.
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return Charge{}, err
	}
	id := "sim_pix_" + hex.EncodeToString(b)

	s.mu.Lock()
	s.charges[id] = &simCharge{expiresAt: req.ExpiresAt}
	s.mu.Unlock()

	code := BRCode{
		Key:          demoPixKey,
		Description:  req.Description,
		MerchantName: "VICESKINS DEMO",
		MerchantCity: "SAO PAULO",
		AmountCents:  req.AmountCents,
		TxID:         req.Reference,
	}
	return Charge{ID: id, BRCode: code.String()}, nil
}

func (s *Simulated) ChargeStatus(_ context.Context, id string) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.charges[id]
	switch {
	case !ok:
		return StatusUnknown, nil // ficou na memória de outro processo (a API reiniciou)
	case c.paid:
		return StatusPaid, nil
	case !s.now().Before(c.expiresAt):
		return StatusExpired, nil
	default:
		return StatusPending, nil
	}
}

// SimulatePayment faz o papel do usuário pagando o Pix no app do banco.
func (s *Simulated) SimulatePayment(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.charges[id]
	switch {
	case !ok:
		return ErrChargeNotFound
	case c.paid:
		return nil
	case !s.now().Before(c.expiresAt):
		return ErrChargeExpired
	}
	c.paid = true
	return nil
}
