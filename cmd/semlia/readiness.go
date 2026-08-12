package main

import (
	"context"
	"strings"

	"github.com/iiwish/semlia/internal/application"
	"github.com/jackc/pgx/v5/pgxpool"
)

const requiredMigrationVersion = 1

type postgresReadinessProbe struct {
	pool *pgxpool.Pool
}

func configuredReadinessProbe(ctx context.Context, databaseURL string) (application.ReadinessProbe, func(), error) {
	if strings.TrimSpace(databaseURL) == "" {
		return application.UnavailableReadinessProbe{}, func() {}, nil
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, nil, err
	}
	poolConfig.MaxConns = 2
	poolConfig.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, nil, err
	}
	return postgresReadinessProbe{pool: pool}, pool.Close, nil
}

func (probe postgresReadinessProbe) Check(ctx context.Context) error {
	var version int
	var dirty bool
	if err := probe.pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty); err != nil {
		return application.ErrDependencyUnavailable
	}
	if version != requiredMigrationVersion || dirty {
		return application.ErrDependencyUnavailable
	}
	return nil
}
