// Package jobs roda tarefas periódicas em background, dentro do mesmo processo da API.
package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Every executa fn a cada interval até ctx ser cancelado (shutdown da API).
// wg permite ao main esperar o job terminar a rodada atual antes de sair.
//
// Equivale a um @Interval() do @nestjs/schedule, mas explícito: é só uma
// goroutine com um time.Ticker.
func Every(ctx context.Context, wg *sync.WaitGroup, name string, interval time.Duration, fn func(context.Context) error) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("job started", "job", name, "interval", interval)
		for {
			select {
			case <-ctx.Done():
				slog.Info("job stopped", "job", name)
				return
			case <-ticker.C:
				run(ctx, name, fn)
			}
		}
	}()
}

// run isola cada rodada: um panic num job não derruba a API inteira.
func run(ctx context.Context, name string, fn func(context.Context) error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("job panicked", "job", name, "panic", r)
		}
	}()
	if err := fn(ctx); err != nil {
		slog.Error("job failed", "job", name, "err", err)
	}
}
