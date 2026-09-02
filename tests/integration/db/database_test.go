package db_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const postgresImage = "postgres:18-alpine"

var (
	databaseURL string
	repoRoot    = repositoryRoot()
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := tcpostgres.Run(
		ctx,
		postgresImage,
		tcpostgres.WithDatabase("semlia_test"),
		tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start isolated PostgreSQL container: failed")
		os.Exit(1)
	}

	databaseURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "resolve isolated PostgreSQL connection: failed")
		os.Exit(1)
	}

	code := m.Run()
	if err := testcontainers.TerminateContainer(container); err != nil && code == 0 {
		fmt.Fprintf(os.Stderr, "terminate isolated PostgreSQL container: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

func TestMigrationLifecycleAndTenantSchema(t *testing.T) {
	migrator := newMigrator(t)

	if err := migrator.Up(); err != nil {
		t.Fatalf("upgrade empty database: %v", err)
	}
	assertVersion(t, migrator, 3, true)
	if err := migrator.Up(); err != nil {
		t.Fatalf("repeat upgrade: %v", err)
	}

	pool := openPool(t)
	firstInventory := tableInventory(t, pool)
	wantTables := []string{
		"asset_revisions", "audit_events", "code_artifacts", "discovery_findings", "discovery_runs",
		"evidence_artifacts", "jobs", "lineage_edges", "ontology_revision_relations", "ontology_revisions",
		"outbox_events", "physical_dataset_revisions", "physical_datasets", "physical_field_revisions",
		"physical_fields", "relation_type_policies", "resource_aliases", "revision_evidence_links",
		"schema_migrations", "semantic_assets", "semantic_relations", "source_connections", "source_revisions",
		"workspaces",
	}
	if strings.Join(firstInventory, ",") != strings.Join(wantTables, ",") {
		t.Fatalf("table inventory = %v, want %v", firstInventory, wantTables)
	}
	t.Logf("%s migration version 3 inventory: %v", postgresImage, firstInventory)
	assertTenantForeignKeys(t, pool)

	if err := migrator.Down(); err != nil {
		t.Fatalf("downgrade database: %v", err)
	}
	assertVersion(t, migrator, 0, false)
	if err := migrator.Up(); err != nil {
		t.Fatalf("upgrade database again: %v", err)
	}
	if got := tableInventory(t, pool); strings.Join(got, ",") != strings.Join(firstInventory, ",") {
		t.Fatalf("inventory after up/down/up = %v, want %v", got, firstInventory)
	}
}

func TestPopulatedM0UpgradeAndRollbackPreserveFoundationRows(t *testing.T) {
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := migrator.Up(); err != nil {
			t.Errorf("restore latest schema: %v", err)
		}
	})
	if err := migrator.Steps(1); err != nil {
		t.Fatalf("install M0 schema: %v", err)
	}
	assertVersion(t, migrator, 1, true)

	pool := openPool(t)
	ctx := context.Background()
	createdAt := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name, created_at, updated_at)
		VALUES ('legacy_workspace', 'legacy-workspace', 'Legacy Workspace', $1, $1)`, createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_events (id, workspace_id, event_type, actor_id, payload, trace_id, created_at)
		VALUES ('legacy_audit', 'legacy_workspace', 'source.created', 'founder', '{}'::jsonb, $2, $1)`, createdAt, "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (
			id, workspace_id, job_type, payload, status, attempt, max_attempts, available_at,
			idempotency_key, last_error_code, trace_id, created_at, updated_at
		) VALUES (
			'legacy_job', 'legacy_workspace', 'discover.source', '{}'::jsonb, 'retryable', 1, 3, $1,
			'legacy-job', 'HANDLER_FAILED', $2, $1, $1
		)`, createdAt, "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_events (
			id, workspace_id, event_type, payload, status, attempt, max_attempts, available_at,
			trace_id, created_at, updated_at, published_at
		) VALUES (
			'legacy_outbox', 'legacy_workspace', 'source.created', '{}'::jsonb, 'published', 1, 3, $1,
			$2, $1, $1, $1
		)`, createdAt, "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}

	if err := migrator.Steps(2); err != nil {
		t.Fatalf("upgrade populated M0: %v", err)
	}
	assertVersion(t, migrator, 3, true)

	for _, table := range []string{"workspaces", "audit_events", "jobs", "outbox_events"} {
		var count int
		query := fmt.Sprintf("SELECT count(*) FROM %s WHERE id IS NOT NULL AND substring(id::text, 15, 1) = '7'", table)
		if err := pool.QueryRow(ctx, query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s UUIDv7 rows = %d", table, count)
		}
	}
	var relationshipCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM workspaces workspace
		JOIN audit_events audit ON audit.workspace_id = workspace.id
		JOIN jobs job ON job.workspace_id = workspace.id
		JOIN outbox_events outbox ON outbox.workspace_id = workspace.id
		WHERE workspace.legacy_id = 'legacy_workspace'
		  AND audit.legacy_id = 'legacy_audit'
		  AND job.legacy_id = 'legacy_job' AND job.status = 'retryable' AND job.attempt = 1
		  AND outbox.legacy_id = 'legacy_outbox' AND outbox.status = 'published' AND outbox.attempt = 1`).Scan(&relationshipCount); err != nil {
		t.Fatal(err)
	}
	if relationshipCount != 1 {
		t.Fatalf("preserved foundation relationships = %d", relationshipCount)
	}

	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove M1 schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("rollback UUIDv7 conversion: %v", err)
	}
	assertVersion(t, migrator, 1, true)
	for table, legacyID := range map[string]string{
		"workspaces": "legacy_workspace", "audit_events": "legacy_audit", "jobs": "legacy_job", "outbox_events": "legacy_outbox",
	} {
		var count int
		query := fmt.Sprintf("SELECT count(*) FROM %s WHERE id = $1", table)
		if err := pool.QueryRow(ctx, query, legacyID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s legacy row count after rollback = %d", table, count)
		}
	}
}

func TestPostgres17MigrationLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(
		ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("semlia_pg17_test"),
		tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Errorf("terminate PostgreSQL 17 container: %v", err)
		}
	})
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := pgstore.NewMigrator(url, filepath.Join(repoRoot, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close() })
	if err := migrator.Up(); err != nil {
		t.Fatalf("PostgreSQL 17 upgrade: %v", err)
	}
	assertVersion(t, migrator, 3, true)
	pool, err := pgstore.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	workspaceID := newWorkspaceID(t)
	createRegistryWorkspace(t, pool, workspaceID, "pg17")
	sourceID, err := identity.NewSourceConnectionID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pgstore.NewStore(pool).CreateSourceConnection(ctx, semantic.SourceConnection{
		ID: sourceID, WorkspaceID: workspaceID, AdapterKind: "postgres", Name: "PostgreSQL 17",
		NormalizedLocator: "postgres://pg17/catalog", Status: "active", Metadata: []byte(`{}`),
	}); err != nil {
		t.Fatalf("PostgreSQL 17 repository write: %v", err)
	}
	if err := migrator.Down(); err != nil {
		t.Fatalf("PostgreSQL 17 downgrade: %v", err)
	}
	if err := migrator.Up(); err != nil {
		t.Fatalf("PostgreSQL 17 re-upgrade: %v", err)
	}
}

func TestClaimQueriesUsePartialIndexes(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	jobID := newRunID(t)
	outboxID := newEventID(t)

	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ($1, 'index-test', 'Index Test')`, workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (
			id, workspace_id, job_type, payload, max_attempts,
			available_at, idempotency_key, trace_id
		) VALUES (
			$1, $2, 'index.test', '{}'::jsonb, 3,
			CURRENT_TIMESTAMP, 'index-test', '4bf92f3577b34da6a3ce929d0e0e4736'
		)`, jobID.UUID(), workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_events (
			id, workspace_id, event_type, payload, max_attempts, available_at, trace_id
		) VALUES (
			$1, $2, 'index.test', '{}'::jsonb, 3,
			CURRENT_TIMESTAMP, '4bf92f3577b34da6a3ce929d0e0e4736'
		)`, outboxID.UUID(), workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "SET enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}

	assertPlanUsesIndex(t, pool, `
		SELECT id FROM jobs
		WHERE status IN ('queued', 'retryable') AND available_at <= CURRENT_TIMESTAMP
		ORDER BY available_at, created_at, id
		FOR UPDATE SKIP LOCKED LIMIT 1`, "jobs_claimable_idx")
	assertPlanUsesIndex(t, pool, `
		SELECT id FROM outbox_events
		WHERE status IN ('queued', 'retryable') AND available_at <= CURRENT_TIMESTAMP
		ORDER BY available_at, created_at, id
		FOR UPDATE SKIP LOCKED LIMIT 1`, "outbox_events_claimable_idx")
}

func TestAuditEventsAreImmutable(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	eventID := newEventID(t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ($1, 'workspace-audit', 'Workspace Audit')`, workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_events (
			id, workspace_id, event_type, actor_id, payload, trace_id
		) VALUES (
			$1, $2, 'source.created', 'founder',
			'{}'::jsonb, '4bf92f3577b34da6a3ce929d0e0e4736'
		)`, eventID.UUID(), workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE audit_events SET event_type = 'source.changed' WHERE id = $1", eventID.UUID()); err == nil {
		t.Fatal("audit update was accepted")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM audit_events WHERE id = $1", eventID.UUID()); err == nil {
		t.Fatal("audit delete was accepted")
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE id = $1", eventID.UUID()).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("immutable audit rows = %d", count)
	}
}

func TestWorkerStartupDoesNotRunMigrations(t *testing.T) {
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := migrator.Up(); err != nil {
			t.Errorf("restore migrated schema: %v", err)
		}
	})

	pool := openPool(t)
	before := tableInventory(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", "./cmd/semlia", "worker")
	command.Dir = repoRoot
	command.Env = append(os.Environ(),
		"SEMLIA_ENV=test",
		"SEMLIA_DATABASE_URL="+databaseURL,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("worker unexpectedly started against an unmigrated database")
	}
	if ctx.Err() != nil {
		t.Fatal("worker hung instead of failing on the missing schema")
	}
	if !strings.Contains(string(output), "worker error: operation failed\n") {
		t.Fatalf("worker output = %q", output)
	}
	if strings.Contains(string(output), "integration-test-only") {
		t.Fatal("worker output leaked database credentials")
	}
	after := tableInventory(t, pool)
	if strings.Join(after, ",") != strings.Join(before, ",") {
		t.Fatalf("worker startup changed schema: before=%v after=%v", before, after)
	}
}

func newMigrator(t *testing.T) *pgstore.Migrator {
	t.Helper()
	migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repoRoot, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := migrator.Close(); err != nil {
			t.Errorf("close migrator: %v", err)
		}
	})
	return migrator
}

func resetSchema(t *testing.T) {
	t.Helper()
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Up(); err != nil {
		t.Fatal(err)
	}
}

func openPool(t *testing.T) *pgstore.Pool {
	t.Helper()
	pool, err := pgstore.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func assertVersion(t *testing.T, migrator *pgstore.Migrator, want uint, exists bool) {
	t.Helper()
	got, dirty, err := migrator.Version()
	if !exists {
		if err != nil {
			t.Fatalf("read empty migration version: %v", err)
		}
		if got != 0 || dirty {
			t.Fatalf("empty migration version = %d dirty=%t", got, dirty)
		}
		return
	}
	if err != nil || got != want || dirty {
		t.Fatalf("migration version = %d dirty=%t err=%v, want %d clean", got, dirty, err, want)
	}
}

func tableInventory(t *testing.T, pool *pgstore.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(tables)
	return tables
}

func assertTenantForeignKeys(t *testing.T, pool *pgstore.Pool) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT DISTINCT tc.table_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.constraint_schema = kcu.constraint_schema
		JOIN information_schema.constraint_column_usage ccu
		  ON tc.constraint_name = ccu.constraint_name
		 AND tc.constraint_schema = ccu.constraint_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND kcu.column_name = 'workspace_id'
		  AND ccu.table_name = 'workspaces'
		ORDER BY tc.table_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	want := []string{
		"audit_events", "evidence_artifacts", "jobs", "ontology_revisions", "outbox_events",
		"semantic_assets", "semantic_relations", "source_connections",
	}
	if strings.Join(tables, ",") != strings.Join(want, ",") {
		t.Fatalf("workspace foreign keys = %v, want %v", tables, want)
	}
}

func newWorkspaceID(t *testing.T) identity.WorkspaceID {
	t.Helper()
	id, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newRunID(t *testing.T) identity.RunID {
	t.Helper()
	id, err := identity.NewRunID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newEventID(t *testing.T) identity.EventID {
	t.Helper()
	id, err := identity.NewEventID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertPlanUsesIndex(t *testing.T, pool *pgstore.Pool, query, index string) {
	t.Helper()
	rows, err := pool.Query(context.Background(), "EXPLAIN "+query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if !strings.Contains(plan.String(), index) {
		t.Fatalf("query plan does not use %s:\n%s", index, plan.String())
	}
	t.Logf("%s plan:\n%s", index, plan.String())
}

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
