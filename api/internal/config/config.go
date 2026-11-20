package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	PublicAPIURL    string // URL pública da API (realm do OpenID)
	AuthRedirectURL string // para onde o navegador vai após o login
	JWTSecret       string
	SteamAPIKey     string
	CookieSecure    bool

	PlatformFeeBps    int64         // taxa da plataforma em basis points (200 = 2%)
	TradeHoldDuration time.Duration // proteção de troca antes de pagar o vendedor
	SimAcceptAfter    time.Duration // simulação: tempo até o comprador "aceitar" a troca
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		PublicAPIURL:    strings.TrimRight(getenv("PUBLIC_API_URL", "http://localhost:8080"), "/"),
		AuthRedirectURL: getenv("AUTH_REDIRECT_URL", "http://localhost:3000"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		SteamAPIKey:     os.Getenv("STEAM_API_KEY"),
		CookieSecure:    os.Getenv("COOKIE_SECURE") == "true",
	}

	var errs []error
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(cfg.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must have at least 32 characters"))
	}

	var err error
	if cfg.PlatformFeeBps, err = strconv.ParseInt(getenv("PLATFORM_FEE_BPS", "200"), 10, 64); err != nil ||
		cfg.PlatformFeeBps < 0 || cfg.PlatformFeeBps > 5_000 {
		errs = append(errs, errors.New("PLATFORM_FEE_BPS must be an integer between 0 and 5000"))
	}
	if cfg.TradeHoldDuration, err = parseDuration("TRADE_HOLD_DURATION", "168h"); err != nil {
		errs = append(errs, err)
	}
	if cfg.SimAcceptAfter, err = parseDuration("SIM_TRADE_ACCEPT_AFTER", "15s"); err != nil {
		errs = append(errs, err)
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parseDuration(key, fallback string) (time.Duration, error) {
	d, err := time.ParseDuration(getenv(key, fallback))
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%s must be a duration like 15s, 2m or 168h", key)
	}
	return d, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
