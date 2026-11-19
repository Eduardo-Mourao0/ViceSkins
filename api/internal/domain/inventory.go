package domain

import (
	"fmt"
	"time"
)

type Sticker struct {
	Kind     string
	Name     string
	ImageURL string
}

type InventoryItem struct {
	ID             int64
	AssetID        string
	Name           string
	MarketHashName string
	Type           string
	Weapon         string
	Rarity         string
	RarityColor    string
	Exterior       string
	Float          *float64
	PaintSeed      *int32
	ImageURL       string
	Stickers       []Sticker
	Tradable       bool
	Marketable     bool
}

type Inventory struct {
	Items    []InventoryItem
	SyncedAt *time.Time // nil = nunca sincronizado
}

// CooldownError é um erro com dados: além de dizer "não pode agora",
// carrega quanto tempo falta. O handler extrai isso com errors.As.
type CooldownError struct {
	RetryAfter time.Duration
}

func (e *CooldownError) Error() string {
	return fmt.Sprintf("cooldown active, retry in %s", e.RetryAfter.Round(time.Second))
}
