package handler

import (
	"context"
	"net/http"
	"time"
)

// Pinger é definida aqui, no pacote que USA a dependência, e não no pacote do banco.
// *pgxpool.Pool satisfaz essa interface implicitamente (sem "implements").
type Pinger interface {
	Ping(ctx context.Context) error
}

func Health(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "db": "down"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "db": "up"})
	}
}
