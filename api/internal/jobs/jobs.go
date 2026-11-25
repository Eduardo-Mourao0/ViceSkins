// Package jobs roda as tarefas periódicas da API.
//
// Dois modos, com as mesmas tarefas:
//   - Every: goroutines com ticker, dentro do processo da API (Docker, VPS).
//   - RunAll: uma rodada de cada, sob demanda. É o que um cron externo chama
//     em ambiente serverless (Vercel), onde nada roda entre requisições.
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Job é uma tarefa periódica. Interval só vale no modo ticker.
type Job struct {
	Name     string
	Interval time.Duration
	Fn       func(context.Context) error
}

// Every executa job.Fn a cada job.Interval até ctx ser cancelado (shutdown).
// wg permite ao main esperar o job terminar a rodada atual antes de sair.
//
// Equivale a um @Interval() do @nestjs/schedule, mas explícito: é só uma
// goroutine com um time.Ticker.
func Every(ctx context.Context, wg *sync.WaitGroup, job Job) {
	wg.Go(func() {
		ticker := time.NewTicker(job.Interval)
		defer ticker.Stop()

		slog.Info("job started", "job", job.Name, "interval", job.Interval)
		for {
			select {
			case <-ctx.Done():
				slog.Info("job stopped", "job", job.Name)
				return
			case <-ticker.C:
				_ = run(ctx, job)
			}
		}
	})
}

// RunAll roda uma rodada de cada job, em sequência, e devolve o erro de cada
// um (nil = ok). Um job que falha não impede os outros.
func RunAll(ctx context.Context, list []Job) map[string]error {
	out := make(map[string]error, len(list))
	for _, job := range list {
		if ctx.Err() != nil {
			out[job.Name] = ctx.Err() // sem tempo: fica para a próxima rodada
			continue
		}
		out[job.Name] = run(ctx, job)
	}
	return out
}

// run isola cada rodada: um panic num job não derruba a API inteira.
func run(ctx context.Context, job Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("job panicked", "job", job.Name, "panic", r)
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	if err = job.Fn(ctx); err != nil {
		slog.Error("job failed", "job", job.Name, "err", err)
	}
	return err
}
