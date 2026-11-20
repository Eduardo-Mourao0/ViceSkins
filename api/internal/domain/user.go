package domain

import (
	"strings"
	"time"
)

type User struct {
	ID           int64
	SteamID      string
	DisplayName  string
	AvatarURL    string
	BalanceCents int64
	TradeURL     *string // nil até o usuário cadastrar
	// API key da Steam (opcional): só se expõe se existe e os últimos 4 dígitos.
	HasSteamAPIKey   bool
	SteamAPIKeyLast4 *string

	Role      string // "user" ou "admin"
	DebtCents int64  // o que deve à plataforma; com dívida, não compra
	CreatedAt time.Time
}

// Contas de demonstração (seed) têm steam_id "demo-*": não existem na Steam.
const DemoSteamIDPrefix = "demo-"

func (u User) IsDemo() bool {
	return strings.HasPrefix(u.SteamID, DemoSteamIDPrefix)
}

const RoleAdmin = "admin"

func (u User) IsAdmin() bool { return u.Role == RoleAdmin }
