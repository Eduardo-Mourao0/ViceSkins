package domain

import "time"

// ListingStatus é um tipo próprio baseado em string. Funciona como um enum:
// o compilador não deixa passar uma string qualquer onde se espera ListingStatus
// sem conversão explícita.
type ListingStatus string

const (
	ListingActive    ListingStatus = "active"
	ListingSold      ListingStatus = "sold"
	ListingCancelled ListingStatus = "cancelled"
)

func (s ListingStatus) Valid() bool {
	switch s {
	case ListingActive, ListingSold, ListingCancelled:
		return true
	}
	return false
}

// Limites de preço de uma listing, em centavos.
const (
	MinListingPriceCents int64 = 10          // 0,10
	MaxListingPriceCents int64 = 100_000_000 // 1.000.000,00
)

type Listing struct {
	ID         int64
	ItemID     int64
	SellerID   int64
	PriceCents int64
	Status     ListingStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Item       *InventoryItem // preenchido só quando a consulta traz o item junto
}
