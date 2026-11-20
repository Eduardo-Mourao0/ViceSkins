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
