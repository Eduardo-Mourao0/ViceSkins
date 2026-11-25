// Package payment fala com o provedor de pagamentos (Pix).
//
// Por enquanto só existe o Simulated: o dinheiro é fictício, mas o fluxo é o
// de um gateway real (cobrança com QR Code, expiração, confirmação do
// pagamento). Plugar um gateway (AbacatePay, Asaas, Mercado Pago) é
// implementar a mesma interface que o service usa.
package payment

import (
	"errors"
	"time"
)

// ChargeRequest descreve uma cobrança Pix.
type ChargeRequest struct {
	Reference   string // nosso identificador (o externalId no gateway)
	AmountCents int64
	ExpiresAt   time.Time
	Description string
}

// Charge é a cobrança criada no provedor.
type Charge struct {
	ID     string
	BRCode string // Pix copia e cola; o QR Code é este texto codificado
}

type Status int

const (
	StatusUnknown Status = iota // o provedor não conhece a cobrança
	StatusPending
	StatusPaid
	StatusExpired
)

var (
	ErrChargeNotFound = errors.New("charge not found")
	ErrChargeExpired  = errors.New("charge expired")
)
