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

	SellerSendTimeout  time.Duration // prazo do vendedor para enviar a oferta
	BuyerAcceptTimeout time.Duration // prazo do comprador para aceitar a oferta

	// Verificação das trocas: "simulated" (padrão, dev) ou "steam" (P2P
	// real, híbrida: inventário público + API key opcional do vendedor).
	TradeProvider      string
	TradeCheckInterval time.Duration // cada troca é verificada no máximo nesse intervalo
	SecretsKey         string        // base64 de 32 bytes: criptografa as API keys dos vendedores

	// JOBS_MODE: "ticker" (padrão) roda os jobs em goroutines dentro da API;
	// "external" não roda nada sozinho, e um cron chama POST /internal/jobs/run
	// (serverless, como a Vercel, onde nada roda entre requisições).
	JobsMode   string
	JobsSecret string // token do cron no /internal/jobs/run (vazio = rota desligada)

	MigrateOnStart bool // aplica as migrations ao subir (quem não tem como rodar o migrate à parte)

	AdminSteamIDs []string // quem resolve disputas (ADMIN_STEAM_IDS, separados por vírgula)
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        httpAddr(),
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
			if _, err := strconv.ParseUint(id, 10, 64); err != nil || len(id) != 17 {
				errs = append(errs, fmt.Errorf("ADMIN_STEAM_IDS: %q is not a SteamID64", id))
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

	cfg.JobsSecret = os.Getenv("JOBS_SECRET")
	if cfg.JobsSecret != "" && len(cfg.JobsSecret) < 32 {
		errs = append(errs, errors.New("JOBS_SECRET must have at least 32 characters"))
	}
	switch cfg.JobsMode = getenv("JOBS_MODE", "ticker"); cfg.JobsMode {
	case "ticker":
	case "external":
		if cfg.JobsSecret == "" {
			errs = append(errs, errors.New("JOBS_SECRET is required with JOBS_MODE=external: nothing else would run the jobs"))
		}
		// A troca simulada guarda as ofertas em memória: em serverless, cada
		// instância teria as suas e as trocas nunca andariam.
		if cfg.TradeProvider == "simulated" {
			errs = append(errs, errors.New("TRADE_PROVIDER=simulated keeps offers in memory and does not work with JOBS_MODE=external; use steam"))
		}
	default:
		errs = append(errs, errors.New("JOBS_MODE must be ticker or external"))
	}
	cfg.MigrateOnStart = os.Getenv("MIGRATE_ON_START") == "true"

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// httpAddr: HTTP_ADDR, ou a porta em PORT (é o que a Vercel e outros hosts passam).
func httpAddr() string {
	if a := os.Getenv("HTTP_ADDR"); a != "" {
		return a
	}
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return ":8080"
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
