package handler

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"
)

// jobsBudget é o tempo de uma rodada pelo cron. Fica abaixo dos 30s de
// timeout de crons externos gratuitos: o que não couber fica para a próxima
// rodada (os jobs trabalham em lotes e cada passo é transacional).
const jobsBudget = 25 * time.Second

// JobsHandler roda os jobs sob demanda, para hosts serverless onde nada roda
// entre requisições. Quem chama é um cron externo, com o JOBS_SECRET.
type JobsHandler struct {
	secret string
	run    func(ctx context.Context) map[string]error
}

func NewJobsHandler(secret string, run func(ctx context.Context) map[string]error) *JobsHandler {
	return &JobsHandler{secret: secret, run: run}
}

// POST /internal/jobs/run   Authorization: Bearer <JOBS_SECRET>
func (h *JobsHandler) Run(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	// Comparação em tempo constante: o tempo de resposta não revela o prefixo certo.
	if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(h.secret)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	// WithoutCancel: se o cron desistir de esperar, a rodada não para no meio.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), jobsBudget)
	defer cancel()

	results := map[string]string{}
	for name, err := range h.run(ctx) {
		// Só "ok"/"error": o detalhe vai para o log, não para fora.
		results[name] = "ok"
		if err != nil {
			results[name] = "error"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
