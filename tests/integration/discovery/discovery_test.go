package discovery_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/discovery/catalog"
	"github.com/iiwish/semlia/internal/adapters/discovery/dbt"
	"github.com/iiwish/semlia/internal/adapters/discovery/postgresql"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	application "github.com/iiwish/semlia/internal/application/discovery"
	domain "github.com/iiwish/semlia/internal/domain/discovery"
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
		ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("semlia_discovery_test"),
		tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start discovery PostgreSQL container: failed")
		os.Exit(1)
	}
	databaseURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "resolve discovery PostgreSQL URL: failed")
		os.Exit(1)
	}
	migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repositoryRoot(), "migrations"))
	if err != nil || migrator.Up() != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "migrate discovery PostgreSQL: failed")
		os.Exit(1)
	}
	_ = migrator.Close()
	code := testingMain.Run()
	if err := testcontainers.TerminateContainer(container); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}

func TestIncrementalDiscoveryReplayAndChangedShape(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspaceID, sourceID := createSource(t, pool, store, "sql")
	service, err := application.NewService(store, postgresql.Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	input := domain.Input{
		Locator: "git://warehouse/schema", ExternalRevision: "commit-1", ObservedAt: observedAt,
		Files: map[string][]byte{"schema.sql": []byte(`
			CREATE TABLE public.orders (id uuid NOT NULL, amount numeric);
			CREATE VIEW public.order_summary (id, amount) AS SELECT id, amount FROM public.orders;
		`)},
	}
	first, err := service.Discover(context.Background(), application.Request{
		WorkspaceID: workspaceID, SourceConnectionID: sourceID, AdapterKind: "postgresql_sql", Input: input,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || first.DatasetCount != 2 || first.FieldCount != 4 || first.LineageCount != 1 {
		t.Fatalf("first discovery = %+v", first)
	}
	replay, err := service.Discover(context.Background(), application.Request{
		WorkspaceID: workspaceID, SourceConnectionID: sourceID, AdapterKind: "postgresql_sql", Input: input,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.SourceRevisionID != first.SourceRevisionID || replay.RunID != first.RunID {
		t.Fatalf("replay = %+v, first = %+v", replay, first)
	}
	assertCounts(t, pool, map[string]int{
		"source_revisions": 1, "discovery_runs": 1, "physical_datasets": 2,
		"physical_dataset_revisions": 2, "physical_fields": 4,
		"physical_field_revisions": 4, "lineage_edges": 1,
	})

	input.ExternalRevision = "commit-2"
	input.ObservedAt = observedAt.Add(time.Minute)
	input.Files["schema.sql"] = []byte(`
		CREATE TABLE public.orders (id uuid NOT NULL, amount numeric, currency text);
		CREATE VIEW public.order_summary (id, amount) AS SELECT id, amount FROM public.orders;
	`)
	changed, err := service.Discover(context.Background(), application.Request{
		WorkspaceID: workspaceID, SourceConnectionID: sourceID, AdapterKind: "postgresql_sql", Input: input,
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Replayed || changed.SourceRevisionID == first.SourceRevisionID {
		t.Fatalf("changed discovery = %+v", changed)
	}
	assertCounts(t, pool, map[string]int{
		"source_revisions": 2, "discovery_runs": 2, "physical_datasets": 2,
		"physical_dataset_revisions": 3, "physical_fields": 5,
		"physical_field_revisions": 7, "lineage_edges": 2,
	})
}

func TestPossibleRenameAndUnresolvedLineageBecomeFindings(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspaceID, sourceID := createSource(t, pool, store, "catalog")
	service, err := application.NewService(store, catalog.Adapter{}, postgresql.Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	base := domain.Input{
		Locator: "catalog://warehouse", ExternalRevision: "catalog-1", ObservedAt: time.Now(),
		Files: map[string][]byte{"catalog.json": []byte(`{"version":"1","datasets":[{"external_key":"old-orders","qualified_name":"public.orders","kind":"table","locator":"public.orders"}]}`)},
	}
	if _, err := service.Discover(context.Background(), application.Request{
		WorkspaceID: workspaceID, SourceConnectionID: sourceID, AdapterKind: "catalog", Input: base,
	}); err != nil {
		t.Fatal(err)
	}
	base.ExternalRevision = "catalog-2"
	base.ObservedAt = base.ObservedAt.Add(time.Minute)
	base.Files["catalog.json"] = []byte(`{"version":"1","datasets":[{"external_key":"new-orders","qualified_name":"public.orders","kind":"table","locator":"public.orders"}]}`)
	renamed, err := service.Discover(context.Background(), application.Request{
		WorkspaceID: workspaceID, SourceConnectionID: sourceID, AdapterKind: "catalog", Input: base,
	})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.FindingCount != 1 {
		t.Fatalf("rename result = %+v", renamed)
	}
	assertFinding(t, pool, renamed.RunID, "POSSIBLE_RENAME")

	sqlInput := domain.Input{
		Locator: "git://warehouse/views", ExternalRevision: "view-1", ObservedAt: base.ObservedAt.Add(time.Minute),
		Files: map[string][]byte{"view.sql": []byte(`CREATE VIEW public.missing_view (id) AS SELECT id FROM external.missing;`)},
	}
	unresolved, err := service.Discover(context.Background(), application.Request{
		WorkspaceID: workspaceID, SourceConnectionID: sourceID, AdapterKind: "postgresql_sql", Input: sqlInput,
	})
	if err != nil {
		t.Fatal(err)
	}
	if unresolved.LineageCount != 0 || unresolved.FindingCount != 1 {
		t.Fatalf("unresolved result = %+v", unresolved)
	}
	assertFinding(t, pool, unresolved.RunID, "UNRESOLVED_LINEAGE")
}

func TestDiscoveryTransactionRollsBackInvalidLineage(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspaceID, sourceID := createSource(t, pool, store, "rollback")
	snapshot := domain.Snapshot{
		AdapterKind: "fixture", AdapterVersion: "1.0.0", Locator: "fixture",
		ContentDigest: "sha256:" + repeat("a", 64), ObservedAt: time.Now(),
		Datasets: []domain.Dataset{{ExternalKey: "one", QualifiedName: "public.one", Kind: "table", Locator: "public.one"}},
		Lineage: []domain.LineageEdge{{
			UpstreamExternalKey: "one", DownstreamExternalKey: "one", Kind: "derived_from", Confidence: 1,
		}},
	}
	if _, err := store.PersistDiscoverySnapshot(context.Background(), workspaceID, sourceID, snapshot); err == nil {
		t.Fatal("invalid self-lineage snapshot was committed")
	}
	_, otherSourceID := createSource(t, pool, store, "other-workspace")
	snapshot.Lineage = nil
	snapshot.ContentDigest = "sha256:" + repeat("b", 64)
	if _, err := store.PersistDiscoverySnapshot(context.Background(), workspaceID, otherSourceID, snapshot); err == nil {
		t.Fatal("cross-workspace discovery snapshot was committed")
	}
	assertCounts(t, pool, map[string]int{
		"source_revisions": 0, "discovery_runs": 0, "physical_datasets": 0,
		"physical_dataset_revisions": 0, "lineage_edges": 0,
	})
}

func TestUnsupportedDBTVersionPersistsFailedRunWithoutProjection(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspaceID, sourceID := createSource(t, pool, store, "dbt")
	service, err := application.NewService(store, dbt.Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Discover(context.Background(), application.Request{
		WorkspaceID: workspaceID, SourceConnectionID: sourceID, AdapterKind: "dbt",
		Input: domain.Input{
			Locator: "git://analytics/target", ExternalRevision: "dbt-unknown", ObservedAt: time.Now(),
			Files: map[string][]byte{
				"manifest.json": []byte(`{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v99.json"}}`),
				"catalog.json":  []byte(`{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/catalog/v1.json"}}`),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || result.DatasetCount != 0 || result.FindingCount != 1 {
		t.Fatalf("dbt unsupported result = %+v", result)
	}
	assertFinding(t, pool, result.RunID, "UNSUPPORTED_DBT_SCHEMA")
	assertCounts(t, pool, map[string]int{
		"source_revisions": 1, "discovery_runs": 1, "discovery_findings": 1,
		"physical_datasets": 0, "physical_dataset_revisions": 0,
	})
}

func newStore(t *testing.T) (*pgstore.Pool, *pgstore.Store) {
	t.Helper()
	pool, err := pgstore.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, pgstore.NewStore(pool)
}

func resetData(t *testing.T, pool *pgstore.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "TRUNCATE workspaces CASCADE"); err != nil {
		t.Fatal(err)
	}
}

func createSource(
	t *testing.T,
	pool *pgstore.Pool,
	store *pgstore.Store,
	suffix string,
) (identity.WorkspaceID, identity.SourceConnectionID) {
	t.Helper()
	workspaceID := mustID(t, identity.NewWorkspaceID)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO workspaces (id, slug, display_name) VALUES ($1, $2, $2)`, workspaceID.UUID(), "discovery-"+suffix); err != nil {
		t.Fatal(err)
	}
	sourceID := mustID(t, identity.NewSourceConnectionID)
	if _, err := store.CreateSourceConnection(context.Background(), semantic.SourceConnection{
		ID: sourceID, WorkspaceID: workspaceID, AdapterKind: "fixture", Name: "Fixture",
		NormalizedLocator: "fixture://" + suffix, Status: "active", Metadata: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	return workspaceID, sourceID
}

func assertCounts(t *testing.T, pool *pgstore.Pool, expected map[string]int) {
	t.Helper()
	for table, want := range expected {
		var got int
		if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s count = %d, want %d", table, got, want)
		}
	}
}

func assertFinding(t *testing.T, pool *pgstore.Pool, runID identity.RunID, code string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(context.Background(), `
		SELECT code FROM discovery_findings WHERE discovery_run_id = $1`, runID.UUID()).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != code {
		t.Fatalf("finding code = %s, want %s", got, code)
	}
}

func mustID[T any](t *testing.T, create func() (T, error)) T {
	t.Helper()
	value, err := create()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func repeat(value string, count int) string {
	result := ""
	for range count {
		result += value
	}
	return result
}

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
