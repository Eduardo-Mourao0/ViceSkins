package handler

import (
	"net/http"
	"strconv"
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
	Jobs         *JobsHandler // nil = sem rota de jobs (JOBS_SECRET vazio)
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(noStore)
	// Limite por IP, alto: atrás de um proxy que não repassa o IP do cliente
	// (front e API em projetos separados na Vercel), todos os usuários
	// chegam com o mesmo IP. Ele só segura loops e abuso grosseiro.
	r.Use(httprate.LimitByIP(3000, time.Minute))
	// Login: por IP, que é o que existe antes de autenticar.
	loginLimit := httprate.LimitByIP(60, time.Minute)
	// Ações caras ou sensíveis (cobrança, compra, API key): por usuário, que
	// não depende de o proxy repassar o IP.
	strict := httprate.Limit(20, time.Minute, httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
		id, _ := userIDFrom(r.Context())
		return strconv.FormatInt(id, 10), nil
	}))

	r.Get("/healthz", Health(d.DB))
	if d.Jobs != nil {
		r.Post("/internal/jobs/run", d.Jobs.Run)
	}

	r.Route("/auth", func(r chi.Router) {
		r.With(loginLimit).Get("/steam/login", d.Auth.Login)
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

// noStore: nenhuma resposta da API vai para cache. São dados da sessão do
// usuário, e um CDN na frente (como o da Vercel em rewrites externos)
// poderia servir a resposta de um usuário para outro.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
