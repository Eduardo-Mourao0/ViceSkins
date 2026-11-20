package domain

import "time"

type TxStatus string

const (
	TxPending   TxStatus = "pending"   // pago pelo comprador, aguardando a oferta de troca
	TxSent      TxStatus = "sent"      // oferta de troca enviada, aguardando o comprador aceitar
	TxOnHold    TxStatus = "on_hold"   // troca aceita; Steam ainda pode reverter (proteção de 7 dias)
	TxCompleted TxStatus = "completed" // proteção acabou: vendedor recebe
	TxCancelled TxStatus = "cancelled" // cancelada antes da troca: comprador reembolsado
	TxFailed    TxStatus = "failed"    // troca recusada/revertida: comprador reembolsado
)

// transitions é a máquina de estados inteira: de cada status, para onde pode ir.
// Status sem entrada (completed, cancelled, failed) são finais.
var transitions = map[TxStatus][]TxStatus{
	TxPending: {TxSent, TxCancelled, TxFailed},
	TxSent:    {TxOnHold, TxCancelled, TxFailed},
	TxOnHold:  {TxCompleted, TxFailed},
}

func (s TxStatus) CanTransitionTo(to TxStatus) bool {
	for _, allowed := range transitions[s] {
		if allowed == to {
			return true
		}
	}
	return false
}

func (s TxStatus) IsFinal() bool {
	_, hasNext := transitions[s]
	return !hasNext
}

// Taxa da plataforma em basis points: 200 bps = 2%.
const MaxFeeBps int64 = 5_000 // 50%, só um limite de sanidade

// CalcFee calcula a taxa só com inteiros, arredondando para baixo
// (o centavo que sobra fica com o vendedor).
func CalcFee(priceCents, feeBps int64) int64 {
	return priceCents * feeBps / 10_000
}

type Transaction struct {
	ID            int64
	ListingID     int64
	BuyerID       int64
	SellerID      int64
	ItemID        int64
	PriceCents    int64
	FeeCents      int64
	Status        TxStatus
	TradeOfferID  *string
	HoldUntil     *time.Time
	FailureReason *string
	BuyerTradeURL *string    // destino da oferta, copiado na compra
	DeadlineAt    *time.Time // prazo da etapa atual (só em pending e sent)
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (t Transaction) SellerReceivesCents() int64 {
	return t.PriceCents - t.FeeCents
}

// TransactionDetail é a transação com o item e as duas partes, para exibir.
type TransactionDetail struct {
	Transaction
	Item   InventoryItem
	Buyer  PublicUser
	Seller PublicUser
}

// Filtros de status do histórico: um status específico ou um grupo.
const (
	TxFilterOpen   = "open"   // ainda em andamento
	TxFilterClosed = "closed" // finalizadas
)

var (
	OpenStatuses   = []TxStatus{TxPending, TxSent, TxOnHold}
	ClosedStatuses = []TxStatus{TxCompleted, TxCancelled, TxFailed}
)

type TxRole string

const (
	RoleBuyer  TxRole = "buyer"
	RoleSeller TxRole = "seller"
)

type TxHistoryFilter struct {
	Role   TxRole // vazio = comprador ou vendedor
	Status string // vazio, um TxStatus, "open" ou "closed"
	Before int64  // cursor: id da última transação já vista (0 = do começo)
	Limit  int
}

type TxHistoryPage struct {
	Transactions []TransactionDetail
	NextCursor   *int64 // nil = acabou
}

type TxSummary struct {
	Purchases            int64
	SpentCents           int64
	Sales                int64
	EarnedCents          int64
	PendingEarningsCents int64
	FeesPaidCents        int64
}
