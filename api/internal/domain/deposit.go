package domain

import "time"

type DepositStatus string

const (
	DepositPending DepositStatus = "pending"
	DepositPaid    DepositStatus = "paid"
	DepositExpired DepositStatus = "expired"
)

// Deposit é um depósito via Pix: o saldo só entra quando a cobrança é paga.
type Deposit struct {
	ID          int64
	UserID      int64
	AmountCents int64
	Status      DepositStatus
	BRCode      string // Pix copia e cola
	ExpiresAt   time.Time
	PaidAt      *time.Time
	CreatedAt   time.Time
}
