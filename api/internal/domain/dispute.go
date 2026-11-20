package domain

import "time"

// Disputa: o item chegou ao comprador DEPOIS de a compra ter sido estornada
// (ele aceitou uma oferta que ficou aberta). Ele ficaria com item e dinheiro.
//
// Resolução automática: o comprador é cobrado de novo (o que faltar no saldo
// vira dívida, e a plataforma garante o vendedor), e o vendedor recebe depois
// da proteção de troca, como numa venda normal. Um admin pode arquivar a
// disputa se a detecção estiver errada.
type DisputeStatus string

const (
	DisputeHolding   DisputeStatus = "holding"   // comprador cobrado; aguardando a proteção de troca
	DisputeSettled   DisputeStatus = "settled"   // vendedor pago
	DisputeReversed  DisputeStatus = "reversed"  // a troca tardia foi revertida: cobrança desfeita
	DisputeDismissed DisputeStatus = "dismissed" // arquivada por um admin: cobrança desfeita
)

type Dispute struct {
	ID            int64
	TransactionID int64
	Status        DisputeStatus
	DetectedAt    time.Time
	RefundedAt    time.Time
	Evidence      string // "fingerprint" (item visto no inventário) ou "offer" (oferta aceita, via API key)
	TradedAt      *time.Time
	ChargedCents  int64 // cobrado do saldo do comprador
	CoveredCents  int64 // virou dívida: a plataforma garante
	HoldUntil     time.Time
	ResolvedAt    *time.Time
	ResolvedBy    *int64
	Note          *string
	CreatedAt     time.Time
}

// DisputeView é a disputa com o contexto para o admin decidir.
type DisputeView struct {
	Dispute
	Transaction    Transaction
	ItemName       string
	ItemImageURL   string
	BuyerName      string
	BuyerDebtCents int64
	SellerName     string
}
