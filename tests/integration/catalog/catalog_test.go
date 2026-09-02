package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	application "github.com/iiwish/semlia/internal/application/catalog"
	domain "github.com/iiwish/semlia/internal/domain/catalog"
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
		tcpostgres.WithDatabase("semlia_catalog_test"),
		tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start catalog PostgreSQL container: failed")
		os.Exit(1)
	}
	databaseURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "resolve catalog PostgreSQL URL: failed")
		os.Exit(1)
	}
	migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repositoryRoot(), "migrations"))
	if err != nil || migrator.Up() != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "migrate catalog PostgreSQL: failed")
		os.Exit(1)
	}
	_ = migrator.Close()
	code := testingMain.Run()
	if err := testcontainers.TerminateContainer(container); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}

func TestAssetRevisionJourneyIsAtomicAndImmutable(t *testing.T) {
	pool, store, service := fixture(t)
	workspaceID := createWorkspace(t, pool, "catalog-journey")
	evidenceID := mustID(t, identity.NewEvidenceID)
	if _, err := store.CreateEvidence(context.Background(), semantic.EvidenceArtifact{
		ID: evidenceID, WorkspaceID: workspaceID, EvidenceType: "declared",
		Locator: "docs://commerce/net-revenue", ContentDigest: digest("definition"),
		Metadata: json.RawMessage(`{"authority":"finance"}`),
	}); err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateAsset(context.Background(), application.CreateAssetRequest{
		WorkspaceID: workspaceID, Address: "commerce.net_revenue", AssetType: semantic.Metric,
		Lifecycle: "active", SchemaVersion: "1.0.0",
		Content:   json.RawMessage(`{"name":"Net revenue","definition":"Revenue after refunds"}`),
		CreatedBy: "founder", EvidenceIDs: []identity.EvidenceID{evidenceID}, TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.CurrentRevision == nil || created.CurrentRevision.Sequence != 1 || len(created.CurrentRevision.Evidence) != 1 {
		t.Fatalf("created detail = %+v", created)
	}
	firstRevisionID := created.CurrentRevision.ID
	appended, err := service.AppendRevision(context.Background(), application.AppendRevisionRequest{
		WorkspaceID: workspaceID, AssetID: created.ID, SchemaVersion: "1.0.0",
		Content:   json.RawMessage(`{"definition":"Revenue after refunds and chargebacks","name":"Net revenue"}`),
		CreatedBy: "founder", EvidenceIDs: []identity.EvidenceID{evidenceID}, TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if appended.Sequence != 2 || appended.ID == firstRevisionID {
		t.Fatalf("appended revision = %+v", appended)
	}
	first, err := service.GetRevision(context.Background(), workspaceID, created.ID, firstRevisionID)
	if err != nil || !containsJSON(first.Content, "Revenue after refunds") {
		t.Fatalf("first revision = %+v, err = %v", first, err)
	}
	detail, err := service.GetAsset(context.Background(), workspaceID, created.ID)
	if err != nil || detail.CurrentRevision == nil || detail.CurrentRevision.ID != appended.ID {
		t.Fatalf("current detail = %+v, err = %v", detail, err)
	}
	revisionPage, err := service.ListRevisions(context.Background(), application.ListRevisionsRequest{
		WorkspaceID: workspaceID, AssetID: created.ID, Limit: 1,
	})
	if err != nil || len(revisionPage.Items) != 1 || revisionPage.Items[0].Sequence != 2 || revisionPage.NextCursor == "" {
		t.Fatalf("first revision page = %+v, err = %v", revisionPage, err)
	}
	olderPage, err := service.ListRevisions(context.Background(), application.ListRevisionsRequest{
		WorkspaceID: workspaceID, AssetID: created.ID, Limit: 1, Cursor: revisionPage.NextCursor,
	})
	if err != nil || len(olderPage.Items) != 1 || olderPage.Items[0].Sequence != 1 {
		t.Fatalf("older revision page = %+v, err = %v", olderPage, err)
	}
	assertCounts(t, pool, map[string]int{
		"semantic_assets": 1, "asset_revisions": 2, "revision_evidence_links": 2,
		"audit_events": 2, "outbox_events": 2,
	})

	invalidEvidence := mustID(t, identity.NewEvidenceID)
	_, err = service.CreateAsset(context.Background(), application.CreateAssetRequest{
		WorkspaceID: workspaceID, Address: "commerce.invalid", AssetType: semantic.Metric,
		SchemaVersion: "1.0.0", Content: json.RawMessage(`{"name":"Invalid"}`),
		CreatedBy: "founder", EvidenceIDs: []identity.EvidenceID{invalidEvidence}, TraceID: traceID,
	})
	if err == nil {
		t.Fatal("missing evidence did not roll back asset creation")
	}
	assertCounts(t, pool, map[string]int{
		"semantic_assets": 1, "asset_revisions": 2, "audit_events": 2, "outbox_events": 2,
	})
}

func TestCatalogSearchPaginationAndWorkspaceIsolation(t *testing.T) {
	pool, _, service := fixture(t)
	workspaceID := createWorkspace(t, pool, "catalog-page")
	otherWorkspaceID := createWorkspace(t, pool, "catalog-other")
	for index, address := range []string{"commerce.gross_revenue", "commerce.net_revenue", "commerce.refunds", "commerce.orders", "commerce.customers"} {
		createAsset(t, service, workspaceID, address, semantic.Metric, fmt.Sprintf(`{"name":"Asset %d","definition":"Revenue governed definition %d"}`, index, index))
	}
	createAsset(t, service, otherWorkspaceID, "commerce.hidden_revenue", semantic.Metric, `{"name":"Hidden revenue"}`)

	first, err := service.ListAssets(context.Background(), application.ListAssetsRequest{
		WorkspaceID: workspaceID, AssetType: semantic.Metric, Limit: 2,
	})
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v, err = %v", first, err)
	}
	second, err := service.ListAssets(context.Background(), application.ListAssetsRequest{
		WorkspaceID: workspaceID, AssetType: semantic.Metric, Limit: 2, Cursor: first.NextCursor,
	})
	if err != nil || len(second.Items) != 2 || second.NextCursor == "" {
		t.Fatalf("second page = %+v, err = %v", second, err)
	}
	if first.Items[0].ID == second.Items[0].ID || first.Items[1].ID == second.Items[1].ID {
		t.Fatal("cursor page duplicated assets")
	}
	search, err := service.ListAssets(context.Background(), application.ListAssetsRequest{
		WorkspaceID: workspaceID, Search: "commerce.net_revenue", Limit: 10,
	})
	if err != nil || len(search.Items) != 1 || search.Items[0].Address.String() != "commerce.net_revenue" {
		t.Fatalf("exact search = %+v, err = %v", search, err)
	}
	contentSearch, err := service.ListAssets(context.Background(), application.ListAssetsRequest{
		WorkspaceID: workspaceID, Search: "governed", Limit: 10,
	})
	if err != nil || len(contentSearch.Items) != 5 {
		t.Fatalf("content search = %d, err = %v", len(contentSearch.Items), err)
	}
	hiddenSearch, err := service.ListAssets(context.Background(), application.ListAssetsRequest{
		WorkspaceID: workspaceID, Search: "hidden_revenue", Limit: 10,
	})
	if err != nil || len(hiddenSearch.Items) != 0 {
		t.Fatalf("cross-workspace search = %+v, err = %v", hiddenSearch, err)
	}
}

func TestBoundedRelationsAndDiscoveryRunProjection(t *testing.T) {
	pool, store, service := fixture(t)
	workspaceID := createWorkspace(t, pool, "catalog-relations")
	assets := make([]domain.AssetDetail, 0, 4)
	for _, address := range []string{"commerce.a", "commerce.b", "commerce.c", "commerce.d"} {
		assets = append(assets, createAsset(t, service, workspaceID, address, semantic.Concept, `{"name":"Concept"}`))
	}
	for index := 0; index < 3; index++ {
		relationID := mustID(t, identity.NewRelationID)
		if _, err := store.CreateRelation(context.Background(), semantic.RelationRecord{
			ID: relationID, WorkspaceID: workspaceID, SubjectAssetID: assets[index].ID,
			Predicate: semantic.BroaderThan, ObjectAssetID: assets[index+1].ID,
			Plane: semantic.TaxonomyPlane, AssertionState: semantic.Asserted,
			CreatedBy: "founder",
		}); err != nil {
			t.Fatal(err)
		}
	}
	relations, err := service.ListRelations(context.Background(), domain.ListRelationsQuery{
		WorkspaceID: workspaceID, AssetID: assets[0].ID, Direction: "outgoing",
		Plane: semantic.TaxonomyPlane, Depth: 3,
	})
	if err != nil || len(relations) != 3 || relations[2].Depth != 3 {
		t.Fatalf("bounded relations = %+v, err = %v", relations, err)
	}

	sourceID := mustID(t, identity.NewSourceConnectionID)
	if _, err := store.CreateSourceConnection(context.Background(), semantic.SourceConnection{
		ID: sourceID, WorkspaceID: workspaceID, AdapterKind: "fixture", Name: "Fixture",
		NormalizedLocator: "fixture://catalog", Status: "active", Metadata: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	runID := mustID(t, identity.NewRunID)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO discovery_runs (
			id, workspace_id, source_connection_id, adapter_version, status, error_code,
			stats, started_at, completed_at
		) VALUES ($1, $2, $3, '1.0.0', 'failed', 'UNSUPPORTED_ARTIFACT',
			'{"datasets":0}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, runID.UUID(), workspaceID.UUID(), sourceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO discovery_findings (discovery_run_id, sequence, code, severity, locator, details)
		VALUES ($1, 1, 'UNSUPPORTED_DBT_SCHEMA', 'error', 'target/manifest.json', '{"version":99}')`, runID.UUID()); err != nil {
		t.Fatal(err)
	}
	run, err := service.GetDiscoveryRun(context.Background(), workspaceID, runID)
	if err != nil || run.Status != "failed" || len(run.Findings) != 1 || run.Findings[0].Sequence != 1 {
		t.Fatalf("discovery run = %+v, err = %v", run, err)
	}
}

func fixture(t *testing.T) (*pgstore.Pool, *pgstore.Store, *application.Service) {
	t.Helper()
	pool, err := pgstore.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), "TRUNCATE workspaces CASCADE"); err != nil {
		t.Fatal(err)
	}
	store := pgstore.NewStore(pool)
	clock := application.ClockFunc(func() time.Time { return time.Now().UTC() })
	return pool, store, application.NewService(store, clock)
}

func createWorkspace(t *testing.T, pool *pgstore.Pool, slug string) identity.WorkspaceID {
	t.Helper()
	id := mustID(t, identity.NewWorkspaceID)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO workspaces (id, slug, display_name) VALUES ($1, $2, $2)`, id.UUID(), slug); err != nil {
		t.Fatal(err)
	}
	return id
}

func createAsset(
	t *testing.T,
	service *application.Service,
	workspace identity.WorkspaceID,
	address string,
	assetType semantic.AssetType,
	content string,
) domain.AssetDetail {
	t.Helper()
	value, err := service.CreateAsset(context.Background(), application.CreateAssetRequest{
		WorkspaceID: workspace, Address: address, AssetType: assetType, Lifecycle: "active",
		SchemaVersion: "1.0.0", Content: json.RawMessage(content), CreatedBy: "founder", TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
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

func containsJSON(value json.RawMessage, substring string) bool {
	return len(value) > 0 && string(value) != "{}" && contains(string(value), substring)
}

func contains(value, substring string) bool {
	for index := 0; index+len(substring) <= len(value); index++ {
		if value[index:index+len(substring)] == substring {
			return true
		}
	}
	return false
}

func digest(value string) string {
	return fmt.Sprintf("sha256:%064x", len(value))
}

func mustID[T any](t *testing.T, create func() (T, error)) T {
	t.Helper()
	value, err := create()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
