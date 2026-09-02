package m1_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/discovery/catalog"
	"github.com/iiwish/semlia/internal/adapters/gitcontent"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	systemapp "github.com/iiwish/semlia/internal/application"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	discoveryapp "github.com/iiwish/semlia/internal/application/discovery"
	"github.com/iiwish/semlia/internal/application/jobs"
	projectionapp "github.com/iiwish/semlia/internal/application/projection"
	usageapp "github.com/iiwish/semlia/internal/application/usage"
	systemdomain "github.com/iiwish/semlia/internal/domain"
	discoverydomain "github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/internal/domain/semantic"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.opentelemetry.io/otel/sdk/trace"
)

var databaseURL string

func TestMain(testingMain *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(
		ctx, "postgres:18-alpine", tcpostgres.WithDatabase("semlia_m1_acceptance"),
		tcpostgres.WithUsername("semlia"), tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start M1 acceptance PostgreSQL container: failed")
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

func TestExactReferenceSourceToDetailGitAndUsage(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces (id, slug, display_name) VALUES ($1, 'm1-journey', 'M1 Journey')`, workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	store := pgstore.NewStore(pool)
	sourceID, _ := identity.NewSourceConnectionID()
	const credentialRef = "vault://semlia/catalog-readonly"
	if _, err := store.CreateSourceConnection(ctx, semantic.SourceConnection{
		ID: sourceID, WorkspaceID: workspace, AdapterKind: "catalog", Name: "Warehouse catalog",
		NormalizedLocator: "git://warehouse/catalog", CredentialRef: credentialRef,
		Status: "active", Metadata: json.RawMessage(`{"mode":"read_only"}`),
	}); err != nil {
		t.Fatal(err)
	}
	discoveryService, err := discoveryapp.NewService(store, catalog.Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	const exactRef = "7f3c9a2e1d4b6a80918273645566778899aabbcc"
	artifact := []byte(`{"version":"1","datasets":[{"external_key":"warehouse.orders","qualified_name":"warehouse.public.orders","kind":"table","locator":"postgres://warehouse/public/orders","fields":[{"external_key":"order_id","name":"order_id","ordinal":1,"data_type":"uuid","nullable":false},{"external_key":"net_revenue","name":"net_revenue","ordinal":2,"data_type":"numeric","nullable":false}]}]}`)
	discovered, err := discoveryService.Discover(ctx, discoveryapp.Request{
		WorkspaceID: workspace, SourceConnectionID: sourceID, AdapterKind: "catalog",
		Input: discoverydomain.Input{
			Locator: "git://warehouse/catalog", ExternalRevision: exactRef,
			ObservedAt: time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC),
			Files:      map[string][]byte{"catalog.json": artifact},
		},
	})
	if err != nil || discovered.Status != "succeeded" || discovered.DatasetCount != 1 || discovered.FieldCount != 2 {
		t.Fatalf("discovery = %+v, err = %v", discovered, err)
	}
	evidenceID, _ := identity.NewEvidenceID()
	if _, err := store.CreateEvidence(ctx, semantic.EvidenceArtifact{
		ID: evidenceID, WorkspaceID: workspace, EvidenceType: "observed",
		SourceRevisionID: &discovered.SourceRevisionID,
		Locator:          "git://warehouse/catalog@" + exactRef + "#warehouse.public.orders.net_revenue",
		ContentDigest:    digest(artifact), Metadata: json.RawMessage(`{"authority":"warehouse"}`),
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 10, 5, 0, 0, time.UTC)
	usageService, _ := usageapp.NewService(store, usageapp.ClockFunc(func() time.Time { return now }), []byte(strings.Repeat("s", 32)))
	catalogService := catalogapp.NewService(store, catalogapp.ClockFunc(func() time.Time { return now }), catalogapp.WithUsage(usageService))
	asset, err := catalogService.CreateAsset(ctx, catalogapp.CreateAssetRequest{
		WorkspaceID: workspace, Address: "commerce.net_revenue", AssetType: semantic.Metric,
		Lifecycle: "active", SchemaVersion: "1.0.0", CreatedBy: "founder",
		TraceID: "1bf92f3577b34da6a3ce929d0e0e4736", EvidenceIDs: []identity.EvidenceID{evidenceID},
		Content: json.RawMessage(`{"name":"Net revenue","definition":"Order revenue after refunds","formula":"sum(net_revenue)"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	handler := httpapi.NewHandler(
		systemapp.NewSystemService(systemapp.ReadinessProbeFunc(func(context.Context) error { return nil }), systemdomain.SystemInfo{
			APIVersion: "v1", SchemaVersion: "0.4.0", BuildVersion: "acceptance",
		}),
		slog.New(slog.NewTextHandler(io.Discard, nil)), provider.Tracer("m1-acceptance"), httpapi.WithCatalog(catalogService),
	)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+workspace.String()+"/catalog/assets/"+asset.ID.String(), nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("detail status = %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, required := range []string{asset.ID.String(), asset.CurrentRevision.ID.String(), evidenceID.String(), discovered.SourceRevisionID.String(), exactRef} {
		if !strings.Contains(body, required) {
			t.Fatalf("detail missing %s: %s", required, body)
		}
	}
	for _, forbidden := range []string{asset.ID.UUID(), sourceID.UUID(), credentialRef} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("detail leaked %s: %s", forbidden, body)
		}
	}

	repositoryPath := t.TempDir()
	writer, err := gitcontent.Open(repositoryPath)
	if err != nil {
		t.Fatal(err)
	}
	router := jobs.NewRouterPublisher()
	router.Register(projectionapp.CatalogAssetChanged, projectionapp.NewPublisher(store, writer))
	dispatcher := jobs.NewDispatcher(store, router, jobs.ClockFunc(func() time.Time { return now.Add(time.Minute) }), jobs.BackoffFunc(func(int32) time.Duration { return 0 }), 30*time.Second)
	processed, err := dispatcher.RunOne(ctx, "m1-acceptance")
	if err != nil || !processed {
		t.Fatalf("projection delivery processed=%t err=%v", processed, err)
	}
	projectionPath := filepath.Join(repositoryPath, "assets", "commerce", "net_revenue.json")
	projected, err := os.ReadFile(projectionPath)
	if err != nil || !strings.Contains(string(projected), asset.CurrentRevision.ID.String()) {
		t.Fatalf("projection = %s, err = %v", projected, err)
	}

	var externalRevision string
	var usageCount, publishedCount int
	if err := pool.QueryRow(ctx, `SELECT external_revision FROM source_revisions WHERE id = $1`, discovered.SourceRevisionID.UUID()).Scan(&externalRevision); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM usage_events WHERE event_type = 'catalog.asset.read'`).Scan(&usageCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE status = 'published'`).Scan(&publishedCount); err != nil {
		t.Fatal(err)
	}
	if externalRevision != exactRef || usageCount != 1 || publishedCount != 1 {
		t.Fatalf("final facts ref=%s usage=%d published=%d", externalRevision, usageCount, publishedCount)
	}
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
