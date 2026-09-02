package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Migrator struct {
	migration *migrate.Migrate
}

func NewMigrator(databaseURL, migrationsPath string) (*Migrator, error) {
	absolutePath, err := filepath.Abs(migrationsPath)
	if err != nil {
		return nil, fmt.Errorf("resolve migration directory: %w", err)
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open migration database: %w", err)
	}
	driver, err := migratepostgres.WithInstance(database, &migratepostgres.Config{})
	if err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("create migration driver: %w", err)
	}
	sourceURL := (&url.URL{Scheme: "file", Path: absolutePath}).String()
	migration, err := migrate.NewWithDatabaseInstance(sourceURL, "postgres", driver)
	if err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	return &Migrator{migration: migration}, nil
}

func (migrator *Migrator) Up() error {
	if err := migrator.migration.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

func (migrator *Migrator) Down() error {
	if err := migrator.migration.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("revert migrations: %w", err)
	}
	return nil
}

func (migrator *Migrator) Steps(count int) error {
	if err := migrator.migration.Steps(count); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply %d migration steps: %w", count, err)
	}
	return nil
}

func (migrator *Migrator) Version() (uint, bool, error) {
	version, dirty, err := migrator.migration.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read migration version: %w", err)
	}
	return version, dirty, nil
}

func (migrator *Migrator) Close() error {
	sourceErr, databaseErr := migrator.migration.Close()
	if sourceErr != nil {
		return fmt.Errorf("close migration source: %w", sourceErr)
	}
	if databaseErr != nil {
		return fmt.Errorf("close migration database: %w", databaseErr)
	}
	return nil
}
