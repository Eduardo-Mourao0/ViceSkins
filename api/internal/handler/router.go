package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

type Deps struct {
	DB           Pinger
	Auth         *AuthHandler
	Inventory    *InventoryHandler
	Listings     *ListingHandler
	Market       *MarketHandler
	Wallet       *WalletHandler
	Deposits     *DepositHandler
	Transactions *TransactionHandler
	Admin        *AdminHandler
	Users        UserGetter // RequireAdmin lê o papel do banco
	Tokens       TokenIssuer
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// Limite por IP. RealIP (acima) já trocou o RemoteAddr pelo IP do cliente
	// vindo do proxy; em produção o Caddy descarta esses headers quando vêm
	// do próprio cliente, então não dá para forjar outro IP.
	r.Use(httprate.LimitByIP(600, time.Minute))
	// Ações caras ou sensíveis (login, cobrança, compra) têm limite próprio.
	strict := httprate.LimitByIP(20, time.Minute)

	r.Get("/healthz", Health(d.DB))

	r.Route("/auth", func(r chi.Router) {
		r.With(strict).Get("/steam/login", d.Auth.Login)
		r.Get("/steam/callback", d.Auth.Callback)
		r.Post("/logout", d.Auth.Logout)
	})

	// Mercado: público, sem login.
	r.Get("/listings", d.Market.Search)
	r.Get("/listings/{id}", d.Market.Get)

	// Rotas autenticadas
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(d.Tokens))
		r.Get("/me", d.Auth.Me)
		r.Put("/me/trade-url", d.Auth.SetTradeURL)
		r.With(strict).Put("/me/steam-api-key", d.Auth.SetSteamAPIKey)
		r.Delete("/me/steam-api-key", d.Auth.DeleteSteamAPIKey)
		r.Get("/me/inventory", d.Inventory.List)
		r.Post("/me/inventory/refresh", d.Inventory.Refresh)
		r.Get("/me/listings", d.Listings.ListMine)

		r.Post("/listings", d.Listings.Create)
		r.Post("/listings/{id}/cancel", d.Listings.Cancel)

		r.Get("/me/wallet", d.Wallet.Get)
		r.With(strict).Post("/wallet/deposits", d.Deposits.Create)
		r.Get("/wallet/deposits/open", d.Deposits.Open)
		r.Get("/wallet/deposits/{id}", d.Deposits.Get)
		r.Post("/wallet/deposits/{id}/simulate-payment", d.Deposits.SimulatePayment)

		r.With(strict).Post("/listings/{id}/buy", d.Transactions.Buy)
		r.Get("/me/transactions", d.Transactions.List)
		r.Get("/me/transactions/summary", d.Transactions.Summary)
		r.Get("/transactions/{id}", d.Transactions.Get)
		r.Post("/transactions/{id}/cancel", d.Transactions.Cancel)
		r.Post("/transactions/{id}/sent", d.Transactions.MarkSent)

		// Admin: papel conferido no banco a cada requisição.
		r.Group(func(r chi.Router) {
			r.Use(RequireAdmin(d.Users))
			r.Get("/admin/disputes", d.Admin.ListDisputes)
			r.Post("/admin/disputes/{id}/dismiss", d.Admin.DismissDispute)
		})
	})

	return r
}
