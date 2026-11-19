// Package testdb sobe um Postgres real e descartável para testes de integração.
//
// O container é criado uma vez por processo de teste (cada pacote roda num
// processo próprio) e removido automaticamente pelo testcontainers ao final.
// Os testes ficam isolados criando seus próprios dados (usuários com IDs únicos).
package testdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

var (
	once     sync.Once
	dsn      string
	startErr error
	seq      atomic.Int64
)

// New devolve um pool conectado ao Postgres de teste, com as migrations aplicadas.
// Pulado com `go test -short`.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: needs Docker (skipped with -short)")
	}
	once.Do(start)
	if startErr != nil {
		t.Fatalf("start test database: %v", startErr)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// NewUser cria um usuário único para o teste e devolve o id.
func NewUser(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	n := seq.Add(1)
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (steam_id, display_name) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("test-%d-%d", time.Now().UnixNano(), n), fmt.Sprintf("user%d", n),
	).Scan(&id)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return id
}

func start() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	c, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("viceskins"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		startErr = fmt.Errorf("run postgres container: %w", err)
		return
	}
	if dsn, err = c.ConnectionString(ctx, "sslmode=disable"); err != nil {
		startErr = err
		return
	}
	startErr = migrate(ctx, dsn)
}

// migrate aplica os *.up.sql em ordem, como o golang-migrate faria.
func migrate(ctx context.Context, dsn string) error {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	files, err := filepath.Glob(filepath.Join(migrationsDir(), "*.up.sql"))
	if err != nil || len(files) == 0 {
		return fmt.Errorf("no migrations found in %s (err: %v)", migrationsDir(), err)
	}
	sort.Strings(files)
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply %s: %w", filepath.Base(f), err)
		}
	}
	return nil
}

// migrationsDir acha api/db/migrations a partir deste arquivo-fonte,
// funcionando não importa de qual pacote o teste rode.
func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "db", "migrations")
}
