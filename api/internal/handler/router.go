package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Deps struct {
	DB           Pinger
	Auth         *AuthHandler
	Inventory    *InventoryHandler
	Listings     *ListingHandler
	Market       *MarketHandler
	Wallet       *WalletHandler
	Transactions *TransactionHandler
	Tokens       TokenIssuer
	DevLogin     bool // registra o login de demonstração (só em dev)
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", Health(d.DB))

	r.Route("/auth", func(r chi.Router) {
		r.Get("/steam/login", d.Auth.Login)
		r.Get("/steam/callback", d.Auth.Callback)
		r.Post("/logout", d.Auth.Logout)

		// Desligado: as rotas nem existem (404), em vez de existirem e recusarem.
		if d.DevLogin {
			r.Get("/dev/users", d.Auth.DevUsers)
			r.Post("/dev/login", d.Auth.DevLogin)
		}
	})

	// Mercado: público, sem login.
	r.Get("/listings", d.Market.Search)
	r.Get("/listings/{id}", d.Market.Get)

	// Rotas autenticadas
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(d.Tokens))
		r.Get("/me", d.Auth.Me)
		r.Get("/me/inventory", d.Inventory.List)
		r.Post("/me/inventory/refresh", d.Inventory.Refresh)
		r.Get("/me/listings", d.Listings.ListMine)

		r.Post("/listings", d.Listings.Create)
		r.Post("/listings/{id}/cancel", d.Listings.Cancel)

		r.Get("/me/wallet", d.Wallet.Get)
		r.Post("/wallet/deposits", d.Wallet.Deposit)

		r.Post("/listings/{id}/buy", d.Transactions.Buy)
		r.Get("/me/transactions", d.Transactions.List)
		r.Get("/me/transactions/summary", d.Transactions.Summary)
		r.Get("/transactions/{id}", d.Transactions.Get)
		r.Post("/transactions/{id}/cancel", d.Transactions.Cancel)
	})

	return r
}
