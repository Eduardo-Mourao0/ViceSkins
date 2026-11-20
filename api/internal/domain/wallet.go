package domain

import "time"

type WalletEntryKind string

const (
	EntryDeposit  WalletEntryKind = "deposit"  // crédito: depósito (fake no MVP)
	EntryPurchase WalletEntryKind = "purchase" // débito: comprou um item
	EntrySale     WalletEntryKind = "sale"     // crédito: vendeu um item (já sem a taxa)
	EntryRefund   WalletEntryKind = "refund"   // crédito: compra cancelada/falhou
	EntryFee      WalletEntryKind = "fee"      // plataforma: taxa de uma venda concluída

	// Dupla entrada com a conta da plataforma (ver migration 000010).
	EntryDisputeCharge  WalletEntryKind = "dispute_charge"  // débito: item recebido depois do estorno
	EntryDisputeCollect WalletEntryKind = "dispute_collect" // plataforma: recebe essa cobrança
	EntryDisputePayout  WalletEntryKind = "dispute_payout"  // plataforma: paga o vendedor da disputa
	EntryDisputeRefund  WalletEntryKind = "dispute_refund"  // disputa desfeita: cobrança devolvida
	EntryDebtPayment    WalletEntryKind = "debt_payment"    // débito: pagamento de dívida
	EntryDebtRecovery   WalletEntryKind = "debt_recovery"   // plataforma: recebe o pagamento da dívida
)

// Limites do depósito fake, em centavos.
const (
	MinDepositCents int64 = 100       // R$ 1,00
	MaxDepositCents int64 = 1_000_000 // R$ 10.000,00
)

// WalletEntry é uma linha do ledger. Nunca é alterada nem apagada:
// correções viram novas linhas (ex.: um refund).
type WalletEntry struct {
	ID                int64
	UserID            int64
	AmountCents       int64 // > 0 crédito, < 0 débito
	BalanceAfterCents int64
	Kind              WalletEntryKind
	TransactionID     *int64
	Description       string
	CreatedAt         time.Time
}

type Wallet struct {
	BalanceCents int64
	Entries      []WalletEntry

	// Vendas em proteção de troca: valor reservado ao vendedor que só entra
	// no saldo quando a proteção acaba (não está em BalanceCents).
	LockedCents   int64
	LockedSales   int64
	NextReleaseAt *time.Time
}
