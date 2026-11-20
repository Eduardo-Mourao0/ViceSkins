package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
)

type TokenIssuer interface {
	Issue(userID int64) (string, error)
	Parse(token string) (int64, error)
	TTL() time.Duration
}

// Tipo privado como chave do context: nenhum outro pacote consegue colidir com ela.
type ctxKeyUserID struct{}

// RequireAuth lê o cookie de sessão, valida o JWT e coloca o userID no context.
// Equivale a um Guard do NestJS, mas é só uma função que embrulha o próximo handler.
func RequireAuth(tokens TokenIssuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(sessionCookie)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthenticated")
				return
			}
			userID, err := tokens.Parse(c.Value)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthenticated")
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyUserID{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func userIDFrom(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(ctxKeyUserID{}).(int64)
	return id, ok
}

// UserGetter busca o usuário (o papel muda no banco, não no token).
type UserGetter interface {
	GetUser(ctx context.Context, id int64) (domain.User, error)
}

// RequireAdmin vem depois de RequireAuth. O papel é lido do banco a cada
// requisição, não do JWT: tirar alguém de ADMIN_STEAM_IDS vale na hora,
// sem esperar o token expirar.
func RequireAdmin(users UserGetter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, _ := userIDFrom(r.Context())
			u, err := users.GetUser(r.Context(), userID)
			if err != nil || !u.IsAdmin() {
				writeError(w, http.StatusForbidden, "admin only")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
