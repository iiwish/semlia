package projection_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/gitcontent"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	"github.com/iiwish/semlia/internal/application/jobs"
	projectionapp "github.com/iiwish/semlia/internal/application/projection"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

var databaseURL string

func TestMain(testingMain *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(
		ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("semlia_projection_test"),
		tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start projection PostgreSQL container: failed")
		os.Exit(1)
	}
	databaseURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		os.Exit(1)
	}
	migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repositoryRoot(), "migrations"))
	if err != nil || migrator.Up() != nil {
		_ = testcontainers.TerminateContainer(container)
		os.Exit(1)
	}
	_ = migrator.Close()
	code := testingMain.Run()
	if err := testcontainers.TerminateContainer(container); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}

func TestOutboxProjectionFailureRecoveryAndWorkspaceIsolation(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE workspaces CASCADE"); err != nil {
		t.Fatal(err)
	}
	workspace := createWorkspace(t, pool, "projection-main")
	otherWorkspace := createWorkspace(t, pool, "projection-other")
	store := pgstore.NewStore(pool)
	clockTime := time.Date(2026, 9, 2, 6, 7, 8, 0, time.UTC)
	service := catalogapp.NewService(store, catalogapp.ClockFunc(func() time.Time { return clockTime }))
	created, err := service.CreateAsset(ctx, catalogapp.CreateAssetRequest{
		WorkspaceID: workspace, Address: "commerce.revenue.net_revenue", AssetType: semantic.Metric,
		Lifecycle: "active", SchemaVersion: "1.0.0", CreatedBy: "founder", TraceID: traceID,
		Content: json.RawMessage(`{"name":"Net revenue","definition":"Revenue less refunds"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadAssetProjection(ctx, otherWorkspace, created.ID, created.CurrentRevision.ID); err == nil {
		t.Fatal("cross-workspace projection load succeeded")
	}

	repositoryPath := t.TempDir()
	writer, err := gitcontent.Open(repositoryPath)
	if err != nil {
		t.Fatal(err)
	}
	dirtyPath := filepath.Join(repositoryPath, "manual.txt")
	if err := os.WriteFile(dirtyPath, []byte("uncommitted\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	publisher := projectionapp.NewPublisher(store, writer)
	dispatcher := jobs.NewDispatcher(
		store, publisher, jobs.ClockFunc(func() time.Time { return clockTime }),
		jobs.BackoffFunc(func(int32) time.Duration { return 0 }), 30*time.Second,
	)
	processed, err := dispatcher.RunOne(ctx, "projection-test")
	if err != nil || !processed {
		t.Fatalf("failed delivery = processed %t, err %v", processed, err)
	}
	assertOutbox(t, pool, "retryable", 1, jobs.PublishFailed)
	if _, err := os.Stat(filepath.Join(repositoryPath, "assets")); !os.IsNotExist(err) {
		t.Fatalf("failed delivery left projection content: %v", err)
	}

	if err := os.Remove(dirtyPath); err != nil {
		t.Fatal(err)
	}
	processed, err = dispatcher.RunOne(ctx, "projection-test")
	if err != nil || !processed {
		t.Fatalf("recovered delivery = processed %t, err %v", processed, err)
	}
	assertOutbox(t, pool, "published", 2, "")
	projectedPath := filepath.Join(repositoryPath, "assets", "commerce", "revenue", "net_revenue.json")
	projected, err := os.ReadFile(projectedPath)
	if err != nil || !contains(string(projected), created.CurrentRevision.ID.String()) {
		t.Fatalf("projected content = %s, err %v", projected, err)
	}
}

func assertOutbox(t *testing.T, pool *pgstore.Pool, status string, attempt int, errorCode string) {
	t.Helper()
	var gotStatus, gotErrorCode string
	var gotAttempt int
	if err := pool.QueryRow(context.Background(), `
		SELECT status, attempt, COALESCE(last_error_code, '')
		FROM outbox_events ORDER BY created_at, id LIMIT 1`).Scan(&gotStatus, &gotAttempt, &gotErrorCode); err != nil {
		t.Fatal(err)
	}
	if gotStatus != status || gotAttempt != attempt || gotErrorCode != errorCode {
		t.Fatalf("outbox = %s/%d/%s, want %s/%d/%s", gotStatus, gotAttempt, gotErrorCode, status, attempt, errorCode)
	}
}

func createWorkspace(t *testing.T, pool *pgstore.Pool, slug string) identity.WorkspaceID {
	t.Helper()
	id, _ := identity.NewWorkspaceID()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO workspaces (id, slug, display_name) VALUES ($1, $2, $2)`, id.UUID(), slug); err != nil {
		t.Fatal(err)
	}
	return id
}

func contains(value, substring string) bool {
	for index := 0; index+len(substring) <= len(value); index++ {
		if value[index:index+len(substring)] == substring {
			return true
		}
	}
	return false
}

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
