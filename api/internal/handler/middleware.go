package handler

import (
	"context"
	"net/http"
	"time"
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
