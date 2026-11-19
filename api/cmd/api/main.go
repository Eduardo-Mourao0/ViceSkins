package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/auth"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/config"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/handler"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/service"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/steam"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// run concentra a inicialização e devolve erro em vez de chamar os.Exit/log.Fatal
// espalhado. Assim os defers (fechar pool etc.) sempre executam.
func run() error {
	// ctx é cancelado quando chega SIGINT/SIGTERM (Ctrl+C ou docker stop).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create db pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	if cfg.SteamAPIKey == "" {
		slog.Warn("STEAM_API_KEY not set: steam login will fail when fetching the profile")
	}

	// Montagem manual das dependências (o que o container de DI do Nest faz sozinho).
	queries := repository.New(pool)
	steamClient := steam.NewClient(cfg.SteamAPIKey, cfg.PublicAPIURL, cfg.PublicAPIURL+"/auth/steam/callback")
	tokens := auth.NewTokens(cfg.JWTSecret, 7*24*time.Hour)
	authSvc := service.NewAuthService(queries, steamClient)

	router := handler.NewRouter(handler.Deps{
		DB:     pool,
		Auth:   handler.NewAuthHandler(authSvc, tokens, cfg.CookieSecure, cfg.AuthRedirectURL),
		Tokens: tokens,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("api listening", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
