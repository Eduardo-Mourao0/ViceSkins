package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/auth"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/config"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/handler"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/jobs"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/migrate"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/payment"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/secrets"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/service"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/steam"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/trade"
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

	if cfg.MigrateOnStart {
		if err := migrate.Up(cfg.DatabaseURL); err != nil {
			return err
		}
		slog.Info("migrations up to date")
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

	// Papel de admin vem da configuração: quem sai da lista perde o papel.
	if err := repository.New(pool).SyncAdmins(ctx, cfg.AdminSteamIDs); err != nil {
		return fmt.Errorf("sync admins: %w", err)
	}

	// Montagem manual das dependências (o que o container de DI do Nest faz sozinho).
	store := repository.NewStore(pool)
	steamClient := steam.NewClient(cfg.SteamAPIKey, cfg.PublicAPIURL, cfg.PublicAPIURL+"/auth/steam/callback")
	tokens := auth.NewTokens(cfg.JWTSecret, 7*24*time.Hour)
	// Interface nil de verdade: guardar um *secrets.Box nil numa interface daria
	// uma interface NÃO nil (tipo preenchido, valor nil), e o `keys == nil`
	// dos services nunca seria verdadeiro.
	var keys service.KeyBox
	if cfg.SecretsKey != "" {
		box, err := secrets.NewBox(cfg.SecretsKey)
		if err != nil {
			return fmt.Errorf("SECRETS_KEY: %w", err)
		}
		keys = box
	}

	var provider service.TradeProvider
	var listingOpts []service.ListingOption
	switch cfg.TradeProvider {
	case "steam":
		// 2s entre consultas de inventário: o endpoint tem rate limit agressivo.
		provider = trade.NewSteam(steamClient, 2*time.Second)
		listingOpts = append(listingOpts, service.RequireAPIKeyForFloatless())
	default:
		provider = trade.NewSimulated(cfg.SimAcceptAfter)
	}
	slog.Info("trade verification", "provider", cfg.TradeProvider, "check_interval", cfg.TradeCheckInterval)

	authSvc := service.NewAuthService(store, steamClient, keys)
	inventorySvc := service.NewInventoryService(store, steamClient, 5*time.Minute)
	listingSvc := service.NewListingService(store, listingOpts...)
	purchaseSvc := service.NewPurchaseService(store, provider, service.PurchaseConfig{
		FeeBps:             cfg.PlatformFeeBps,
		HoldDuration:       cfg.TradeHoldDuration,
		SellerSendTimeout:  cfg.SellerSendTimeout,
		BuyerAcceptTimeout: cfg.BuyerAcceptTimeout,
		CheckInterval:      cfg.TradeCheckInterval,
		Keys:               keys,
	})

	// Pix fictício: só existe o provedor simulado por enquanto.
	depositSvc := service.NewDepositService(store, payment.NewSimulated())

	jobList := []jobs.Job{
		{Name: "trade-outbox", Interval: 3 * time.Second, Fn: purchaseSvc.DispatchPendingOffers},
		// O job roda com frequência, mas cada troca só é verificada a cada
		// TRADE_CHECK_INTERVAL (controle por last_checked_at no banco).
		{Name: "trade-watch", Interval: 5 * time.Second, Fn: purchaseSvc.WatchTrades},
		{Name: "hold-release", Interval: 30 * time.Second, Fn: purchaseSvc.ReleaseDueHolds},
		{Name: "expire-deadlines", Interval: 30 * time.Second, Fn: purchaseSvc.ExpireDeadlines},
		{Name: "late-delivery-watch", Interval: time.Minute, Fn: purchaseSvc.WatchClosedTrades},
		{Name: "dispute-release", Interval: time.Minute, Fn: purchaseSvc.ReleaseDueDisputes},
		{Name: "deposit-expiry", Interval: 30 * time.Second, Fn: depositSvc.ExpireDeposits},
	}

	// Jobs em background (JOBS_MODE=ticker). Ao sair de run(), os defers rodam
	// em ordem inversa: primeiro param os jobs e esperamos a rodada atual,
	// depois fecha o pool. Com JOBS_MODE=external, quem roda é o cron.
	jobsCtx, stopJobs := context.WithCancel(ctx)
	var jobsWG sync.WaitGroup
	defer func() {
		stopJobs()
		jobsWG.Wait()
	}()
	if cfg.JobsMode == "ticker" {
		for _, j := range jobList {
			jobs.Every(jobsCtx, &jobsWG, j)
		}
	} else {
		slog.Info("jobs run on demand: POST /internal/jobs/run", "jobs", len(jobList))
	}
	var jobsHandler *handler.JobsHandler
	if cfg.JobsSecret != "" {
		jobsHandler = handler.NewJobsHandler(cfg.JobsSecret, func(ctx context.Context) map[string]error {
			return jobs.RunAll(ctx, jobList)
		})
	}

	router := handler.NewRouter(handler.Deps{
		DB:           pool,
		Auth:         handler.NewAuthHandler(authSvc, tokens, cfg.CookieSecure, cfg.AuthRedirectURL),
		Inventory:    handler.NewInventoryHandler(inventorySvc),
		Listings:     handler.NewListingHandler(listingSvc),
		Market:       handler.NewMarketHandler(service.NewMarketService(store)),
		Wallet:       handler.NewWalletHandler(service.NewWalletService(store)),
		Deposits:     handler.NewDepositHandler(depositSvc),
		Transactions: handler.NewTransactionHandler(purchaseSvc, service.NewHistoryService(store)),
		Admin:        handler.NewAdminHandler(purchaseSvc),
		Users:        authSvc,
		Tokens:       tokens,
		Jobs:         jobsHandler,
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
