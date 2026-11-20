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
	CreatedAt    time.Time
}

// Contas de demonstração (seed) têm steam_id "demo-*": não existem na Steam.
const DemoSteamIDPrefix = "demo-"

func (u User) IsDemo() bool {
	return strings.HasPrefix(u.SteamID, DemoSteamIDPrefix)
}
