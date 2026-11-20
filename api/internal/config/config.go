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
	DevLogin        bool // login sem Steam nas contas demo da seed

	PlatformFeeBps    int64         // taxa da plataforma em basis points (200 = 2%)
	TradeHoldDuration time.Duration // proteção de troca antes de pagar o vendedor
	SimAcceptAfter    time.Duration // simulação: tempo até o comprador "aceitar" a troca

	SellerSendTimeout  time.Duration // prazo do vendedor para enviar a oferta
	BuyerAcceptTimeout time.Duration // prazo do comprador para aceitar a oferta

	// Verificação das trocas: "simulated" (padrão, dev/demo) ou "steam" (P2P
	// real, híbrida: inventário público + API key opcional do vendedor).
	TradeProvider      string
	TradeCheckInterval time.Duration // cada troca é verificada no máximo nesse intervalo
	SecretsKey         string        // base64 de 32 bytes: criptografa as API keys dos vendedores

	AdminSteamIDs []string // quem resolve disputas (ADMIN_STEAM_IDS, separados por vírgula)
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
		DevLogin:        os.Getenv("DEV_LOGIN") == "true",
	}

	var errs []error
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	// COOKIE_SECURE=true indica produção (HTTPS): ali o login demo nunca pode existir.
	if cfg.DevLogin && cfg.CookieSecure {
		errs = append(errs, errors.New("DEV_LOGIN must not be enabled together with COOKIE_SECURE=true"))
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
	if cfg.SellerSendTimeout, err = parseDuration("SELLER_SEND_TIMEOUT", "12h"); err != nil || cfg.SellerSendTimeout == 0 {
		errs = append(errs, errors.New("SELLER_SEND_TIMEOUT must be a positive duration"))
	}
	if cfg.BuyerAcceptTimeout, err = parseDuration("BUYER_ACCEPT_TIMEOUT", "12h"); err != nil || cfg.BuyerAcceptTimeout == 0 {
		errs = append(errs, errors.New("BUYER_ACCEPT_TIMEOUT must be a positive duration"))
	}
	if cfg.SimAcceptAfter, err = parseDuration("SIM_TRADE_ACCEPT_AFTER", "15s"); err != nil {
		errs = append(errs, err)
	}

	for _, id := range strings.Split(os.Getenv("ADMIN_STEAM_IDS"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			cfg.AdminSteamIDs = append(cfg.AdminSteamIDs, id)
			// Contas demo entram sem senha: nunca podem ser admin em produção.
			if cfg.CookieSecure && strings.HasPrefix(id, "demo-") {
				errs = append(errs, errors.New("ADMIN_STEAM_IDS must not contain demo accounts with COOKIE_SECURE=true"))
			}
		}
	}
	cfg.SecretsKey = os.Getenv("SECRETS_KEY")
	switch cfg.TradeProvider = getenv("TRADE_PROVIDER", "simulated"); cfg.TradeProvider {
	case "simulated":
	case "steam":
		if cfg.SecretsKey == "" {
			errs = append(errs, errors.New("SECRETS_KEY is required with TRADE_PROVIDER=steam"))
		}
	default:
		errs = append(errs, errors.New("TRADE_PROVIDER must be simulated or steam"))
	}
	// Na Steam, o inventário tem rate limit agressivo: verificar com calma.
	defaultCheck := "5s"
	if cfg.TradeProvider == "steam" {
		defaultCheck = "2m"
	}
	if cfg.TradeCheckInterval, err = parseDuration("TRADE_CHECK_INTERVAL", defaultCheck); err != nil || cfg.TradeCheckInterval == 0 {
		errs = append(errs, errors.New("TRADE_CHECK_INTERVAL must be a positive duration"))
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
