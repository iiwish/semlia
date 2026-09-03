package governance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/internal/application"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/internal/domain"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.opentelemetry.io/otel/sdk/trace"
)

const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

var databaseURL string

func TestMain(testingMain *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(
		ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("semlia_governance_http_test"),
		tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start governance PostgreSQL container: failed")
		os.Exit(1)
	}
	databaseURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "resolve governance PostgreSQL URL: failed")
		os.Exit(1)
	}
	migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repositoryRoot(), "migrations"))
	if err != nil || migrator.Up() != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "migrate governance PostgreSQL: failed")
		os.Exit(1)
	}
	_ = migrator.Close()
	code := testingMain.Run()
	if err := testcontainers.TerminateContainer(container); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}

type fixture struct {
	pool    *pgstore.Pool
	store   *pgstore.Store
	handler http.Handler
}

func newFixture(t *testing.T) *fixture {
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
	authorizer := authorizationapp.NewService(store, authorizationapp.ClockFunc(time.Now))
	catalog := catalogapp.NewService(store, catalogapp.ClockFunc(func() time.Time { return time.Now().UTC() }))
	clock := governanceapp.ClockFunc(func() time.Time { return time.Now().UTC() })
	authoring := governanceapp.NewAuthoringService(
		store,
		governanceapp.NewProposalService(store, clock),
		governanceapp.NewAgentRunService(store, clock),
		authorizer, clock,
		governanceapp.WithValidationOrchestrator(
			governanceapp.NewValidationOrchestrator(
				governanceapp.NewProposalService(store, clock), store, clock,
			),
		),
	)
	var logs bytes.Buffer
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	handler := httpapi.NewHandler(
		application.NewSystemService(
			application.ReadinessProbeFunc(func(context.Context) error { return nil }),
			domain.SystemInfo{APIVersion: "v1", SchemaVersion: "0.4.0", BuildVersion: "test-build"},
		),
		slog.New(slog.NewTextHandler(&logs, nil)),
		provider.Tracer("governance-integration"),
		httpapi.WithCatalog(catalog),
		httpapi.WithGovernance(authoring),
	)
	return &fixture{pool: pool, store: store, handler: handler}
}

func (environment *fixture) request(t *testing.T, method, path, principalRef, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if principalRef != "" {
		request.Header.Set("X-Semlia-Principal", principalRef)
	}
	response := httptest.NewRecorder()
	environment.handler.ServeHTTP(response, request)
	return response
}

func (environment *fixture) proposalsPath(t *testing.T, workspace identity.WorkspaceID) string {
	t.Helper()
	return "/api/v1/workspaces/" + workspace.String() + "/governance/proposals"
}

// runValidationWorker drains one enqueued validation job with the production handler wiring.
func (environment *fixture) runValidationWorker(t *testing.T) {
	t.Helper()
	clock := governanceapp.ClockFunc(func() time.Time { return time.Now().UTC() })
	handler := governanceapp.NewValidationJobHandler(
		environment.store,
		governanceapp.NewProposalService(environment.store, clock),
		governanceapp.NewValidationService(environment.store, clock),
		governanceapp.NewDefaultRegistry(),
		clock,
	)
	worker := jobs.NewWorker(
		environment.store, jobs.ClockFunc(time.Now),
		jobs.BackoffFunc(func(int32) time.Duration { return time.Minute }), time.Minute,
	)
	worker.Register(governanceapp.ValidationJobType, handler.Handle)
	processed, err := worker.RunOne(context.Background(), "governance-test-worker")
	if err != nil {
		t.Fatalf("worker run: %v", err)
	}
	if !processed {
		t.Fatal("worker found no validation job to process")
	}
}

func (environment *fixture) createAsset(t *testing.T, workspace identity.WorkspaceID) (identity.AssetID, identity.RevisionID) {
	t.Helper()
	catalog := catalogapp.NewService(
		environment.store,
		catalogapp.ClockFunc(func() time.Time { return time.Now().UTC() }),
	)
	created, err := catalog.CreateAsset(context.Background(), catalogapp.CreateAssetRequest{
		WorkspaceID: workspace, Address: "commerce.net_revenue", AssetType: semantic.Metric,
		Lifecycle: "active", SchemaVersion: "1.0.0",
		Content:   json.RawMessage(`{"name":"Net revenue","definition":"Revenue after refunds"}`),
		CreatedBy: "founder", TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return created.ID, created.CurrentRevision.ID
}

func createWorkspace(t *testing.T, pool *pgstore.Pool, slug string) identity.WorkspaceID {
	t.Helper()
	workspaceID := mustID(t, identity.NewWorkspaceID)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO workspaces (id, slug, display_name) VALUES ($1, $2, $2)`,
		workspaceID.UUID(), slug); err != nil {
		t.Fatal(err)
	}
	return workspaceID
}

func digestOf(value string) string {
	return fmt.Sprintf("sha256:%064x", len(value))
}

// seedJoinContract plants the minimal M1 physical graph (one source, two
// datasets, matching fields) and one join contract, so the governance-object
// proposal target has a real persisted object behind its TypeID.
func seedJoinContract(t *testing.T, environment *fixture, slug string) (identity.WorkspaceID, identity.JoinContractID) {
	t.Helper()
	ctx := context.Background()
	workspaceID := createWorkspace(t, environment.pool, slug)
	sourceID := mustID(t, identity.NewSourceConnectionID)
	if _, err := environment.pool.Exec(ctx, `
		INSERT INTO source_connections (id, workspace_id, adapter_kind, name, normalized_locator, status, metadata)
		VALUES ($1, $2, 'postgres', $3, $4, 'active', '{}'::jsonb)`,
		sourceID.UUID(), workspaceID.UUID(), "contract warehouse", "postgres://contract/catalog"); err != nil {
		t.Fatal(err)
	}
	sourceRevisionID := mustID(t, identity.NewSourceRevisionID)
	if _, err := environment.pool.Exec(ctx, `
		INSERT INTO source_revisions (id, workspace_id, source_connection_id, content_digest, adapter_version, observed_at)
		VALUES ($1, $2, $3, $4, 'postgres/1.0.0', CURRENT_TIMESTAMP)`,
		sourceRevisionID.UUID(), workspaceID.UUID(), sourceID.UUID(), digestOf("contract-source")); err != nil {
		t.Fatal(err)
	}
	dataset := func(externalKey, qualifiedName string) identity.PhysicalDatasetID {
		id := mustID(t, identity.NewPhysicalDatasetID)
		if _, err := environment.pool.Exec(ctx, `
			INSERT INTO physical_datasets (id, workspace_id, source_connection_id, external_key, qualified_name)
			VALUES ($1, $2, $3, $4, $5)`,
			id.UUID(), workspaceID.UUID(), sourceID.UUID(), externalKey, qualifiedName); err != nil {
			t.Fatal(err)
		}
		return id
	}
	leftDatasetID := dataset("contract.orders", "contract warehouse.public.orders")
	rightDatasetID := dataset("contract.customers", "contract warehouse.public.customers")
	field := func(datasetID identity.PhysicalDatasetID, externalKey string) identity.PhysicalFieldID {
		id := mustID(t, identity.NewPhysicalFieldID)
		if _, err := environment.pool.Exec(ctx, `
			INSERT INTO physical_fields (id, workspace_id, physical_dataset_id, external_key, name)
			VALUES ($1, $2, $3, $4, $4)`,
			id.UUID(), workspaceID.UUID(), datasetID.UUID(), externalKey); err != nil {
			t.Fatal(err)
		}
		return id
	}
	leftFieldID := field(leftDatasetID, "customer_id")
	rightFieldID := field(rightDatasetID, "customer_id")
	objectService := governanceapp.NewGovernedObjectService(
		environment.store, governanceapp.ClockFunc(func() time.Time { return time.Now().UTC() }),
	)
	contract, err := objectService.Create(ctx, governanceapp.CreateGovernedObjectRequest{
		WorkspaceID: workspaceID,
		Object: governance.GovernedObject{
			Type: governance.TargetJoinContract,
			JoinContract: &governance.JoinContract{
				WorkspaceID:   workspaceID,
				LeftDatasetID: leftDatasetID, RightDatasetID: rightDatasetID,
				LeftFieldRefs:  []identity.PhysicalFieldID{leftFieldID},
				RightFieldRefs: []identity.PhysicalFieldID{rightFieldID},
				JoinType:       governance.JoinLeft, Cardinality: governance.CardinalityManyToOne,
				JoinExpression: "orders.customer_id = customers.customer_id",
				Content:        json.RawMessage(`{}`),
			},
		},
		CreatedBy: "steward",
	})
	if err != nil {
		t.Fatal(err)
	}
	return workspaceID, contract.JoinContract.ID
}

const substantiveChangeSet = `"changeSet":[{"fieldPath":"definition","op":"update","beforeDigest":"` +
	`sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","afterDigest":"` +
	`sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","beforeValue":"Revenue after refunds","afterValue":"Revenue after refunds and chargebacks"}]`

func proposalCreateBody(targetObjectType, targetObjectID, baseRevisionID string) string {
	body := fmt.Sprintf(
		`{"targetObjectType":%q,"targetObjectId":%q,%s,"title":"Tighten metric definition","summary":"Clarifies refunds","reason":"Audit finding","createdBy":"founder"}`,
		targetObjectType, targetObjectID, substantiveChangeSet)
	if baseRevisionID != "" {
		body = fmt.Sprintf(
			`{"targetObjectType":%q,"targetObjectId":%q,"baseRevisionId":%q,%s,"title":"Tighten metric definition","summary":"Clarifies refunds","reason":"Audit finding","createdBy":"founder"}`,
			targetObjectType, targetObjectID, baseRevisionID, substantiveChangeSet)
	}
	return body
}

func assertJSONField(t *testing.T, body []byte, field string, want any) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, body)
	}
	if payload[field] != want {
		t.Fatalf("%s = %v, want %v\n%s", field, payload[field], want, body)
	}
}

func assertTableCount(t *testing.T, pool *pgstore.Pool, table string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
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

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
