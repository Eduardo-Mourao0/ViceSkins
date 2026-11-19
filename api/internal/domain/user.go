package domain

import "time"

type User struct {
	ID           int64
	SteamID      string
	DisplayName  string
	AvatarURL    string
	BalanceCents int64
	CreatedAt    time.Time
}
