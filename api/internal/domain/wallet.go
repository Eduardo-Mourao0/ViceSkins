package domain

import "time"

type WalletEntryKind string

const (
	EntryDeposit  WalletEntryKind = "deposit"  // crédito: depósito (fake no MVP)
	EntryPurchase WalletEntryKind = "purchase" // débito: comprou um item
	EntrySale     WalletEntryKind = "sale"     // crédito: vendeu um item (já sem a taxa)
	EntryRefund   WalletEntryKind = "refund"   // crédito: compra cancelada/falhou
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
