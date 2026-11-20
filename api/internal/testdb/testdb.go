// Package testdb sobe um Postgres real e descartável para testes de integração.
//
// O container é criado uma vez por processo de teste (cada pacote roda num
// processo próprio) e removido automaticamente pelo testcontainers ao final.
//
// Cada teste ganha um BANCO PRÓPRIO, clonado de um template já migrado
// (CREATE DATABASE ... TEMPLATE leva milissegundos). Criar dados próprios
// não bastava: os jobs (verificação de trocas, disputas) olham o banco
// inteiro, e um teste acabava agindo sobre as transações de outro.
package testdb

import (
	"context"
	"fmt"
	"net/url"
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

	ctx := context.Background()
	name := fmt.Sprintf("test_%d", dbSeq.Add(1))
	admin, err := pgxpool.New(ctx, withDB(dsn, "postgres"))
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	defer admin.Close()
	// Nomes vêm de um contador, não de entrada externa: seguro no SQL.
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name+" TEMPLATE "+templateDB); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	pool, err := pgxpool.New(ctx, withDB(dsn, name))
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if admin, err := pgxpool.New(ctx, withDB(dsn, "postgres")); err == nil {
			admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			admin.Close()
		}
	})
	return pool
}

const templateDB = "viceskins"

var dbSeq atomic.Int64

// withDB troca o banco da connection string.
func withDB(dsn, db string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		panic(err)
	}
	u.Path = "/" + db
	return u.String()
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
		postgres.WithDatabase(templateDB),
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
