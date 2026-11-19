package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	PublicAPIURL    string // URL pública da API (realm do OpenID)
	AuthRedirectURL string // para onde o navegador vai após o login
	JWTSecret       string
	SteamAPIKey     string
	CookieSecure    bool
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
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
