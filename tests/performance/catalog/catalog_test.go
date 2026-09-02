package catalog_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	application "github.com/iiwish/semlia/internal/application/catalog"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const fixtureSize = 10_000

func TestCatalog10000Performance(t *testing.T) {
	if os.Getenv("SEMLIA_RUN_M1_BENCHMARK") != "1" {
		t.Skip("set SEMLIA_RUN_M1_BENCHMARK=1 to run the M1 performance gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(
		ctx, "postgres:18-alpine", tcpostgres.WithDatabase("semlia_catalog_benchmark"),
		tcpostgres.WithUsername("semlia"), tcpostgres.WithPassword("benchmark-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Errorf("terminate benchmark container: %v", err)
		}
	})
	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repositoryRoot(), "migrations"))
	if err != nil || migrator.Up() != nil {
		t.Fatalf("migrate benchmark database: %v", err)
	}
	_ = migrator.Close()
	pool, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	workspace, targetAddress := loadFixture(t, ctx, pool)
	service := application.NewService(pgstore.NewStore(pool), application.ClockFunc(time.Now))

	first, err := service.ListAssets(ctx, application.ListAssetsRequest{WorkspaceID: workspace, Limit: 50})
	if err != nil || len(first.Items) != 50 || first.NextCursor == "" {
		t.Fatalf("warm first page = %d cursor=%t err=%v", len(first.Items), first.NextCursor != "", err)
	}
	second, err := service.ListAssets(ctx, application.ListAssetsRequest{WorkspaceID: workspace, Limit: 50, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 50 {
		t.Fatalf("warm second page = %d err=%v", len(second.Items), err)
	}
	seen := make(map[string]struct{}, 100)
	for _, item := range append(first.Items, second.Items...) {
		if _, duplicate := seen[item.ID.String()]; duplicate {
			t.Fatalf("cursor duplicated %s", item.ID)
		}
		seen[item.ID.String()] = struct{}{}
	}
	search, err := service.ListAssets(ctx, application.ListAssetsRequest{WorkspaceID: workspace, Search: targetAddress, Limit: 10})
	if err != nil || len(search.Items) != 1 || search.Items[0].Address.String() != targetAddress {
		t.Fatalf("golden search = %+v err=%v", search, err)
	}

	pageSamples := measure(25, func() error {
		_, err := service.ListAssets(ctx, application.ListAssetsRequest{WorkspaceID: workspace, Limit: 50})
		return err
	})
	searchSamples := measure(25, func() error {
		page, err := service.ListAssets(ctx, application.ListAssetsRequest{WorkspaceID: workspace, Search: targetAddress, Limit: 10})
		if err == nil && (len(page.Items) != 1 || page.Items[0].Address.String() != targetAddress) {
			return fmt.Errorf("golden result mismatch")
		}
		return err
	})
	pageP50, pageP95 := percentile(pageSamples, 50), percentile(pageSamples, 95)
	searchP50, searchP95 := percentile(searchSamples, 50), percentile(searchSamples, 95)
	t.Logf("M1 catalog benchmark: host=%s/%s PostgreSQL=18-alpine assets=%d samples=25 page_p50=%s page_p95=%s search_p50=%s search_p95=%s",
		runtime.GOOS, runtime.GOARCH, fixtureSize, pageP50, pageP95, searchP50, searchP95)
	if pageP95 > 150*time.Millisecond {
		t.Fatalf("first-page p95 = %s, budget = 150ms", pageP95)
	}
	if searchP95 > 250*time.Millisecond {
		t.Fatalf("exact-address search p95 = %s, budget = 250ms", searchP95)
	}
}

func loadFixture(t *testing.T, ctx context.Context, pool *pgstore.Pool) (identity.WorkspaceID, string) {
	t.Helper()
	workspace, _ := identity.NewWorkspaceID()
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces (id, slug, display_name) VALUES ($1, 'benchmark', 'Benchmark')`, workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	assetRows := make([][]any, 0, fixtureSize)
	revisionRows := make([][]any, 0, fixtureSize)
	createdAt := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	targetAddress := "warehouse.table_09999"
	for index := 0; index < fixtureSize; index++ {
		assetID, _ := identity.NewAssetID()
		revisionID, _ := identity.NewRevisionID()
		key := fmt.Sprintf("table_%05d", index)
		content, _ := json.Marshal(map[string]any{
			"name":       fmt.Sprintf("Warehouse table %05d", index),
			"definition": fmt.Sprintf("Governed warehouse dataset number %05d", index),
		})
		sum := sha256.Sum256(content)
		assetRows = append(assetRows, []any{
			assetID.UUID(), workspace.UUID(), "warehouse", key, string(semantic.Entity), revisionID.UUID(), "active", createdAt, createdAt,
		})
		revisionRows = append(revisionRows, []any{
			revisionID.UUID(), workspace.UUID(), assetID.UUID(), int64(1), "1.0.0",
			"sha256:" + hex.EncodeToString(sum[:]), content, "benchmark", createdAt,
		})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"semantic_assets"}, []string{
		"id", "workspace_id", "namespace", "key", "asset_type", "current_revision_id", "lifecycle_state", "created_at", "updated_at",
	}, pgx.CopyFromRows(assetRows)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"asset_revisions"}, []string{
		"id", "workspace_id", "asset_id", "sequence", "schema_version", "content_digest", "content", "created_by", "created_at",
	}, pgx.CopyFromRows(revisionRows)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE semantic_assets; ANALYZE asset_revisions"); err != nil {
		t.Fatal(err)
	}
	return workspace, targetAddress
}

func measure(samples int, operation func() error) []time.Duration {
	result := make([]time.Duration, 0, samples)
	for index := 0; index < samples; index++ {
		started := time.Now()
		if err := operation(); err != nil {
			panic(err)
		}
		result = append(result, time.Since(started))
	}
	return result
}

func percentile(values []time.Duration, percent int) time.Duration {
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })
	index := (len(sorted)*percent + 99) / 100
	if index < 1 {
		index = 1
	}
	return sorted[index-1]
}

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
