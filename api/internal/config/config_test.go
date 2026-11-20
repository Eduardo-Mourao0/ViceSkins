package config

import "testing"

func setBaseEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
}

func TestDevLoginRefusedWithSecureCookies(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("DEV_LOGIN", "true")
	t.Setenv("COOKIE_SECURE", "true")
	if _, err := Load(); err == nil {
		t.Fatal("expected error: DEV_LOGIN must not run with COOKIE_SECURE=true")
	}
}

func TestDefaults(t *testing.T) {
	setBaseEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DevLogin || cfg.PlatformFeeBps != 200 || cfg.TradeHoldDuration.Hours() != 168 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestTradeProvider(t *testing.T) {
	setBaseEnv(t)
	cfg, err := Load()
	if err != nil || cfg.TradeProvider != "simulated" || cfg.TradeCheckInterval.Seconds() != 5 {
		t.Fatalf("default: %+v, %v", cfg, err)
	}

	t.Setenv("TRADE_PROVIDER", "steam")
	if _, err := Load(); err == nil {
		t.Fatal("steam provider without SECRETS_KEY must be refused")
	}
	t.Setenv("SECRETS_KEY", "c2VjcmV0")
	cfg, err = Load()
	if err != nil || cfg.TradeCheckInterval.Minutes() != 2 {
		t.Fatalf("steam: %+v, %v (want 2m default interval)", cfg, err)
	}

	t.Setenv("TRADE_PROVIDER", "bots")
	if _, err := Load(); err == nil {
		t.Fatal("unknown provider must be refused")
	}
}

func TestAdminSteamIDs(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("ADMIN_STEAM_IDS", " 76561198000000001 , demo-1,,")
	cfg, err := Load()
	if err != nil || len(cfg.AdminSteamIDs) != 2 || cfg.AdminSteamIDs[1] != "demo-1" {
		t.Fatalf("%v, %v", cfg.AdminSteamIDs, err)
	}
	t.Setenv("COOKIE_SECURE", "true")
	if _, err := Load(); err == nil {
		t.Fatal("a demo admin must be refused in production (COOKIE_SECURE=true)")
	}
}
