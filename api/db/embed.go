// Package db guarda as migrations dentro do binário da API (MIGRATE_ON_START).
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
