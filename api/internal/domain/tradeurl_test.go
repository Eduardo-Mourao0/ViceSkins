package domain

import (
	"errors"
	"testing"
)

func TestNormalizeTradeURL(t *testing.T) {
	// SteamID64 76561198000000123 → account id 39734395.
	steamUser := User{SteamID: "76561198000000123"}
	const canonical = "https://steamcommunity.com/tradeoffer/new/?partner=39734395&token=AbC-12_x"

	ok := []struct {
		name string
		raw  string
		user User
	}{
		{"canonical", canonical, steamUser},
		{"no trailing slash", "https://steamcommunity.com/tradeoffer/new?partner=39734395&token=AbC-12_x", steamUser},
		{"params reordered", "https://steamcommunity.com/tradeoffer/new/?token=AbC-12_x&partner=39734395", steamUser},
		{"extra params are dropped", canonical + "&utm=x", steamUser},
	}
	for _, c := range ok {
		t.Run(c.name, func(t *testing.T) {
			got, err := NormalizeTradeURL(c.raw, c.user)
			if err != nil {
				t.Fatal(err)
			}
			if got != canonical {
				t.Fatalf("got %s, want %s", got, canonical)
			}
		})
	}

	bad := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"http", "http://steamcommunity.com/tradeoffer/new/?partner=39734395&token=AbC-12_x"},
		{"lookalike host", "https://steamcommunity.com.evil.io/tradeoffer/new/?partner=39734395&token=AbC-12_x"},
		{"host suffix", "https://evilsteamcommunity.com/tradeoffer/new/?partner=39734395&token=AbC-12_x"},
		{"userinfo trick", "https://steamcommunity.com@evil.io/tradeoffer/new/?partner=39734395&token=AbC-12_x"},
		{"wrong path", "https://steamcommunity.com/id/someone/?partner=39734395&token=AbC-12_x"},
		{"missing token", "https://steamcommunity.com/tradeoffer/new/?partner=39734395"},
		{"short token", "https://steamcommunity.com/tradeoffer/new/?partner=39734395&token=abc"},
		{"bad token chars", "https://steamcommunity.com/tradeoffer/new/?partner=39734395&token=abc$1234"},
		{"partner not a number", "https://steamcommunity.com/tradeoffer/new/?partner=abc&token=AbC-12_x"},
		{"partner overflows uint32", "https://steamcommunity.com/tradeoffer/new/?partner=4294967296&token=AbC-12_x"},
		{"another account", "https://steamcommunity.com/tradeoffer/new/?partner=39734396&token=AbC-12_x"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NormalizeTradeURL(c.raw, steamUser); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}
