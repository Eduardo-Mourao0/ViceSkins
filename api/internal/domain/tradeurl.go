package domain

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
)

// steamID64Base converte SteamID64 (o "76561198...") em SteamID32 (account
// ID), que é o que vai no parâmetro partner da trade URL.
const steamID64Base uint64 = 76561197960265728

// O token da trade URL tem 8 caracteres: letras, dígitos, '-' ou '_'.
var tradeTokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]{8}$`)

// NormalizeTradeURL valida a trade URL e devolve a forma canônica:
//
//	https://steamcommunity.com/tradeoffer/new/?partner=<account id>&token=<token>
//
// O partner precisa ser a conta do próprio usuário: sem isso, alguém poderia
// cadastrar a URL de outra pessoa e mandar os itens comprados para ela.
func NormalizeTradeURL(raw string, owner User) (string, error) {
	invalid := func(reason string) error {
		return fmt.Errorf("%w: trade url %s", ErrInvalidInput, reason)
	}
	if len(raw) > 200 {
		return "", invalid("is too long")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", invalid("is not a valid url")
	}
	// Host exato: "steamcommunity.com.evil.com" ou "evilsteamcommunity.com" não passam.
	if u.Scheme != "https" || u.Host != "steamcommunity.com" ||
		(u.Path != "/tradeoffer/new/" && u.Path != "/tradeoffer/new") {
		return "", invalid("must start with https://steamcommunity.com/tradeoffer/new/")
	}

	q := u.Query()
	partner, err := strconv.ParseUint(q.Get("partner"), 10, 32)
	if err != nil || partner == 0 {
		return "", invalid("has an invalid partner")
	}
	token := q.Get("token")
	if !tradeTokenRe.MatchString(token) {
		return "", invalid("has an invalid token")
	}

	id64, err := strconv.ParseUint(owner.SteamID, 10, 64)
	if err != nil || id64 < steamID64Base || id64-steamID64Base != partner {
		return "", invalid("belongs to another steam account")
	}

	return fmt.Sprintf("https://steamcommunity.com/tradeoffer/new/?partner=%d&token=%s", partner, token), nil
}
