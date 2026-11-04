package db

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

//go:embed migrations_vectorstore/*.sql
var vectorstoreFS embed.FS

func Migrate(databaseURL string, useVectorstore bool, direction string) error {
	var fs embed.FS
	var dir string
	if useVectorstore {
		fs = vectorstoreFS
		dir = "migrations_vectorstore"
	} else {
		fs = migrationsFS
		dir = "migrations"
	}
	src, err := iofs.New(fs, dir)
	if err != nil {
		return fmt.Errorf("migrate source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer m.Close()
	switch direction {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			return err
		}
	case "down":
		if err := m.Steps(-1); err != nil && err != migrate.ErrNoChange {
			return err
		}
	default:
		return fmt.Errorf("unknown direction %q: want up or down", direction)
	}
	return nil
}
