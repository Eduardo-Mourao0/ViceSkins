package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Deps struct {
	DB        Pinger
	Auth      *AuthHandler
	Inventory *InventoryHandler
	Tokens    TokenIssuer
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
	})

	// Rotas autenticadas
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(d.Tokens))
		r.Get("/me", d.Auth.Me)
		r.Get("/me/inventory", d.Inventory.List)
		r.Post("/me/inventory/refresh", d.Inventory.Refresh)
	})

	return r
}
