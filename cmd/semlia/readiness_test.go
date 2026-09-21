package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/internal/application"
	ingestionapp "github.com/iiwish/semlia/internal/application/ingestion"
	ingestiondomain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

type transientScheduleRepository struct {
	ingestionapp.ScheduleRepository
	calls  int
	cancel context.CancelFunc
}

func (repository *transientScheduleRepository) ClaimDueSchedules(context.Context, string, time.Duration, int) ([]ingestiondomain.Schedule, time.Time, error) {
	repository.calls++
	if repository.calls == 1 {
		return nil, time.Now(), ingestiondomain.ErrRetryable
	}
	repository.cancel()
	return nil, time.Now(), nil
}

func TestScheduleLoopSurvivesTransientDatabaseFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repository := &transientScheduleRepository{cancel: cancel}
	service := ingestionapp.NewScheduleService(repository, nil, ingestionapp.ClockFunc(time.Now))
	if err := runScheduleLoop(ctx, service, "retry-test", time.Millisecond); err != nil || repository.calls != 2 {
		t.Fatalf("calls=%d err=%v", repository.calls, err)
	}
}

func TestConfiguredReadinessProbeWithoutDatabaseIsUnavailable(t *testing.T) {
	probe, closeProbe, err := configuredReadinessProbe(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer closeProbe()
	if err := probe.Check(context.Background()); !errors.Is(err, application.ErrDependencyUnavailable) {
		t.Fatalf("readiness error = %v", err)
	}
}

func TestReadinessRequiresSourceSnapshotMigration(t *testing.T) {
	if requiredMigrationVersion != 32 {
		t.Fatalf("required migration version = %d, want 32", requiredMigrationVersion)
	}
}

func TestSourceSnapshotReadinessRejectsSchemaDrift(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(ctx, "postgres:18-alpine", tcpostgres.WithDatabase("readiness23"), tcpostgres.WithUsername("semlia"), tcpostgres.WithPassword("integration-test-only"), tcpostgres.BasicWaitStrategies())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Error(err)
		}
	})
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := pgstore.NewMigrator(url, filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close() })
	if err := migrator.Up(); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	probe := postgresReadinessProbe{pool: pool}
	if err := probe.Check(ctx); err != nil {
		t.Fatalf("valid schema rejected: %v", err)
	}
	var tenantFK, tenantDefinition string
	if err := pool.QueryRow(ctx, `SELECT conname,pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='source_snapshot_runs'::regclass AND confrelid='discovery_runs'::regclass`).Scan(&tenantFK, &tenantDefinition); err != nil {
		t.Fatal(err)
	}
	var codeBytesDefinition string
	if err := pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='source_code_revisions'::regclass AND conname='source_code_revisions_content_bytes_check'`).Scan(&codeBytesDefinition); err != nil {
		t.Fatal(err)
	}
	t.Logf("code byte constraint: %s", codeBytesDefinition)
	const dropCodeBytesCheck = `ALTER TABLE source_code_revisions DROP CONSTRAINT source_code_revisions_content_bytes_check; ALTER TABLE source_code_revisions ADD CONSTRAINT source_code_revisions_content_bytes_check `
	for _, test := range []struct{ name, breakSQL, restoreSQL string }{
		{"old source version", `UPDATE schema_migrations SET version=23`, `UPDATE schema_migrations SET version=30`},
		{"old authoring version", `UPDATE schema_migrations SET version=24`, `UPDATE schema_migrations SET version=30`},
		{"old publishing version", `UPDATE schema_migrations SET version=25`, `UPDATE schema_migrations SET version=30`},
		{"old generation version", `UPDATE schema_migrations SET version=26`, `UPDATE schema_migrations SET version=30`},
		{"old business rule version", `UPDATE schema_migrations SET version=27`, `UPDATE schema_migrations SET version=30`},
		{"old password version", `UPDATE schema_migrations SET version=28`, `UPDATE schema_migrations SET version=30`},
		{"disabled business rule immutability", `ALTER TABLE production_business_rule_events DISABLE TRIGGER production_business_rule_events_immutable`, `ALTER TABLE production_business_rule_events ENABLE TRIGGER production_business_rule_events_immutable`},
		{"disabled business rule binding", `ALTER TABLE production_business_rule_events DISABLE TRIGGER production_business_rule_event_guard`, `ALTER TABLE production_business_rule_events ENABLE TRIGGER production_business_rule_event_guard`},
		{"missing business rule table", `ALTER TABLE production_business_rule_events RENAME TO misplaced_business_rules`, `ALTER TABLE misplaced_business_rules RENAME TO production_business_rule_events`},
		{"disabled generation guard", `ALTER TABLE production_generation_requests DISABLE TRIGGER production_generation_result_guard`, `ALTER TABLE production_generation_requests ENABLE TRIGGER production_generation_result_guard`},
		{"disabled publishing seal", `ALTER TABLE production_validation_seals DISABLE TRIGGER production_validation_seal_guard`, `ALTER TABLE production_validation_seals ENABLE TRIGGER production_validation_seal_guard`},
		{"missing canonical input", `ALTER TABLE production_versions RENAME COLUMN canonical_input TO unavailable_input`, `ALTER TABLE production_versions RENAME COLUMN unavailable_input TO canonical_input`},
		{"disabled production history trigger", `ALTER TABLE production_versions DISABLE TRIGGER production_versions_immutable`, `ALTER TABLE production_versions ENABLE TRIGGER production_versions_immutable`},
		{"dirty version", `UPDATE schema_migrations SET dirty=true`, `UPDATE schema_migrations SET dirty=false`},
		{"missing table", `ALTER TABLE source_snapshot_diagnostics RENAME TO misplaced_diagnostics`, `ALTER TABLE misplaced_diagnostics RENAME TO source_snapshot_diagnostics`},
		{"missing authoring table", `ALTER TABLE production_operations RENAME TO misplaced_operations`, `ALTER TABLE misplaced_operations RENAME TO production_operations`},
		{"missing publishing table", `ALTER TABLE production_validation_attempts RENAME TO misplaced_attempts`, `ALTER TABLE misplaced_attempts RENAME TO production_validation_attempts`},
		{"missing code bytes", `ALTER TABLE source_code_revisions RENAME COLUMN content_bytes TO unavailable_bytes`, `ALTER TABLE source_code_revisions RENAME COLUMN unavailable_bytes TO content_bytes`},
		{"disabled immutable trigger", `ALTER TABLE source_snapshots DISABLE TRIGGER source_snapshots_immutable`, `ALTER TABLE source_snapshots ENABLE TRIGGER source_snapshots_immutable`},
		{"missing tenant foreign key", `ALTER TABLE source_snapshot_runs DROP CONSTRAINT ` + pgx.Identifier{tenantFK}.Sanitize(), `ALTER TABLE source_snapshot_runs ADD CONSTRAINT ` + pgx.Identifier{tenantFK}.Sanitize() + ` ` + tenantDefinition},
		{"legacy 8 MiB code limit", dropCodeBytesCheck + `CHECK(octet_length(content_bytes) BETWEEN 1 AND 8388608)`, dropCodeBytesCheck + codeBytesDefinition},
		{"legacy 10 MiB code limit", dropCodeBytesCheck + `CHECK(octet_length(content_bytes) BETWEEN 1 AND 10485760)`, dropCodeBytesCheck + codeBytesDefinition},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, test.breakSQL); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := pool.Exec(ctx, test.restoreSQL); err != nil {
					t.Fatal(err)
				}
			}()
			if err := probe.Check(ctx); !errors.Is(err, application.ErrDependencyUnavailable) {
				t.Fatalf("schema drift accepted: %v", err)
			}
		})
	}
}

func TestConfiguredReadinessProbeRejectsInvalidURL(t *testing.T) {
	if _, _, err := configuredReadinessProbe(context.Background(), "://database-secret"); err == nil {
		t.Fatal("invalid database URL was accepted")
	}
}

func TestServerDoesNotEchoInvalidDatabaseURL(t *testing.T) {
	const databaseURL = "://database-secret"
	lookup := mapEnvironment(map[string]string{"SEMLIA_DATABASE_URL": databaseURL})
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"server"}, lookup, ioDiscard{}, &stderr); code != 1 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if bytes.Contains(stderr.Bytes(), []byte("database-secret")) {
		t.Fatalf("stderr leaked database URL: %s", stderr.String())
	}
}
