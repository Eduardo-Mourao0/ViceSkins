// Package migrate aplica as migrations embutidas no binário, ao subir a API.
//
// Para hosts onde não há como rodar o container do golang-migrate antes da
// API (serverless). Várias instâncias subindo juntas não brigam: o
// golang-migrate segura um advisory lock no Postgres enquanto migra.
package migrate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // driver "pgx5://"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/Eduardo-Mourao0/ViceSkins/api/db"
)

// Up aplica o que faltar. Sem nada novo, não faz nada.
func Up(databaseURL string) error {
	src, err := iofs.New(db.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, pgx5URL(databaseURL))
	if err != nil {
		return fmt.Errorf("connect for migrations: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// O driver pgx v5 do golang-migrate se registra com o esquema "pgx5".
func pgx5URL(u string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if rest, ok := strings.CutPrefix(u, prefix); ok {
			return "pgx5://" + rest
		}
	}
	return u
}
