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
	assertVersion(t, migrator, 1, true)
	if err := migrator.Up(); err != nil {
		t.Fatalf("repeat upgrade: %v", err)
	}

	pool := openPool(t)
	firstInventory := tableInventory(t, pool)
	wantTables := []string{"audit_events", "jobs", "outbox_events", "schema_migrations", "workspaces"}
	if strings.Join(firstInventory, ",") != strings.Join(wantTables, ",") {
		t.Fatalf("table inventory = %v, want %v", firstInventory, wantTables)
	}
	t.Logf("%s migration version 1 inventory: %v", postgresImage, firstInventory)
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

func TestClaimQueriesUsePartialIndexes(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ('workspace_index_test', 'index-test', 'Index Test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (
			id, workspace_id, job_type, payload, max_attempts,
			available_at, idempotency_key, trace_id
		) VALUES (
			'job_index_test', 'workspace_index_test', 'index.test', '{}'::jsonb, 3,
			CURRENT_TIMESTAMP, 'index-test', '4bf92f3577b34da6a3ce929d0e0e4736'
		)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_events (
			id, workspace_id, event_type, payload, max_attempts, available_at, trace_id
		) VALUES (
			'outbox_index_test', 'workspace_index_test', 'index.test', '{}'::jsonb, 3,
			CURRENT_TIMESTAMP, '4bf92f3577b34da6a3ce929d0e0e4736'
		)`); err != nil {
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
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ('workspace_audit', 'workspace-audit', 'Workspace Audit')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_events (
			id, workspace_id, event_type, actor_id, payload, trace_id
		) VALUES (
			'audit_immutable', 'workspace_audit', 'source.created', 'founder',
			'{}'::jsonb, '4bf92f3577b34da6a3ce929d0e0e4736'
		)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE audit_events SET event_type = 'source.changed' WHERE id = 'audit_immutable'"); err == nil {
		t.Fatal("audit update was accepted")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM audit_events WHERE id = 'audit_immutable'"); err == nil {
		t.Fatal("audit delete was accepted")
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE id = 'audit_immutable'").Scan(&count); err != nil {
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
		SELECT tc.table_name
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
	want := []string{"audit_events", "jobs", "outbox_events"}
	if strings.Join(tables, ",") != strings.Join(want, ",") {
		t.Fatalf("workspace foreign keys = %v, want %v", tables, want)
	}
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
