package usage_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	usageapp "github.com/iiwish/semlia/internal/application/usage"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var databaseURL string

func TestMain(testingMain *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(
		ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("semlia_usage_test"), tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"), tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start usage PostgreSQL container: failed")
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

func TestCatalogUsagePrivacyIdempotencyAndRetention(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE workspaces CASCADE"); err != nil {
		t.Fatal(err)
	}
	workspace, _ := identity.NewWorkspaceID()
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces (id, slug, display_name) VALUES ($1, 'usage', 'Usage')`, workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	store := pgstore.NewStore(pool)
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	usageService, err := usageapp.NewService(store, usageapp.ClockFunc(func() time.Time { return now }), []byte(strings.Repeat("s", 32)))
	if err != nil {
		t.Fatal(err)
	}
	catalog := catalogapp.NewService(store, catalogapp.ClockFunc(func() time.Time { return now }), catalogapp.WithUsage(usageService))
	created, err := catalog.CreateAsset(ctx, catalogapp.CreateAssetRequest{
		WorkspaceID: workspace, Address: "commerce.net_revenue", AssetType: semantic.Metric,
		Lifecycle: "active", SchemaVersion: "1.0.0", CreatedBy: "founder",
		TraceID: "1bf92f3577b34da6a3ce929d0e0e4736", Content: json.RawMessage(`{"name":"Net revenue"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	searchTrace := "2bf92f3577b34da6a3ce929d0e0e4736"
	search := catalogapp.ListAssetsRequest{
		WorkspaceID: workspace, Search: "Confidential Revenue Phrase", AssetType: semantic.Metric,
		Lifecycle: "active", Channel: "api", TraceID: searchTrace,
	}
	if _, err := catalog.ListAssets(ctx, search); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ListAssets(ctx, search); err != nil {
		t.Fatal(err)
	}
	readTrace := "3bf92f3577b34da6a3ce929d0e0e4736"
	if _, err := catalog.GetAssetObserved(ctx, workspace, created.ID, catalogapp.ReadObservation{Channel: "api", TraceID: readTrace}); err != nil {
		t.Fatal(err)
	}

	var count int
	var persisted string
	if err := pool.QueryRow(ctx, `SELECT count(*), string_agg(row_to_json(usage_events)::text, '') FROM usage_events`).Scan(&count, &persisted); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("usage count = %d, want idempotent search + read", count)
	}
	if strings.Contains(strings.ToLower(persisted), "confidential") || strings.Contains(strings.ToLower(persisted), "revenue phrase") {
		t.Fatalf("raw search text persisted: %s", persisted)
	}
	if !strings.Contains(persisted, created.CurrentRevision.ID.UUID()) || !strings.Contains(persisted, "zero_result") {
		t.Fatalf("usage attribution missing: %s", persisted)
	}

	oldTime := now.Add(-100 * 24 * time.Hour)
	oldService, _ := usageapp.NewService(store, usageapp.ClockFunc(func() time.Time { return oldTime }), []byte(strings.Repeat("s", 32)))
	if err := oldService.RecordSearch(ctx, usageapp.SearchInput{
		WorkspaceID: workspace, Query: "expired query", ResultCount: 1, Channel: "system",
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
	}); err != nil {
		t.Fatal(err)
	}
	deleted, err := usageService.DeleteExpired(ctx, 1)
	if err != nil || deleted != 1 {
		t.Fatalf("retention deleted = %d, err = %v", deleted, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM usage_events`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("retained usage count = %d, err = %v", count, err)
	}
}

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
