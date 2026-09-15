package db_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	distribution "github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/pkg/identity"
)

const distributionTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

var distributionClock = distributionapp.ClockFunc(func() time.Time {
	return time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)
})

type distributionFixture struct {
	workspaceID identity.WorkspaceID
	assetID     identity.AssetID
	revision1   identity.RevisionID
	revision2   identity.RevisionID
	release1    identity.ReleaseID
	release2    identity.ReleaseID
	bindingID   identity.PhysicalBindingID
	grainID     identity.ModelGrainID
	entityKeyID identity.EntityKeyID
	dataset1    identity.PhysicalDatasetID
	dataset2    identity.PhysicalDatasetID
}

func seedDistributionReleases(t *testing.T, pool *pgstore.Pool) distributionFixture {
	t.Helper()
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	sourceID, err := identity.NewSourceConnectionID()
	if err != nil {
		t.Fatal(err)
	}
	dataset1, err := identity.NewPhysicalDatasetID()
	if err != nil {
		t.Fatal(err)
	}
	dataset2, err := identity.NewPhysicalDatasetID()
	if err != nil {
		t.Fatal(err)
	}
	assetID, err := identity.NewAssetID()
	if err != nil {
		t.Fatal(err)
	}
	revision1, err := identity.NewRevisionID()
	if err != nil {
		t.Fatal(err)
	}
	revision2, err := identity.NewRevisionID()
	if err != nil {
		t.Fatal(err)
	}
	bindingID, err := identity.NewPhysicalBindingID()
	if err != nil {
		t.Fatal(err)
	}
	grainID, err := identity.NewModelGrainID()
	if err != nil {
		t.Fatal(err)
	}
	entityKeyID, err := identity.NewEntityKeyID()
	if err != nil {
		t.Fatal(err)
	}
	release1, err := identity.NewReleaseID()
	if err != nil {
		t.Fatal(err)
	}
	release2, err := identity.NewReleaseID()
	if err != nil {
		t.Fatal(err)
	}

	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspaces (id, slug, display_name) VALUES ($1, 'distribution', 'Distribution')`, []any{workspaceID.UUID()}},
		{`INSERT INTO source_connections (id, workspace_id, adapter_kind, name, normalized_locator)
		  VALUES ($1, $2, 'postgres', 'Warehouse', 'postgres://warehouse')`, []any{sourceID.UUID(), workspaceID.UUID()}},
		{`INSERT INTO physical_datasets (id, workspace_id, source_connection_id, external_key, qualified_name)
		  VALUES ($1, $2, $3, 'orders_v1', 'analytics.orders_v1'),
		         ($4, $2, $3, 'orders_v2', 'analytics.orders_v2')`, []any{dataset1.UUID(), workspaceID.UUID(), sourceID.UUID(), dataset2.UUID()}},
		{`INSERT INTO semantic_assets (id, workspace_id, namespace, key, asset_type, lifecycle_state)
		  VALUES ($1, $2, 'commerce', 'gross_revenue', 'metric', 'active')`, []any{assetID.UUID(), workspaceID.UUID()}},
		{`INSERT INTO asset_revisions (id, workspace_id, asset_id, sequence, schema_version, content_digest, content, created_by)
		  VALUES ($1, $2, $3, 1, '1.0.0', $4, '{"name":"Gross revenue","aliases":["revenue"]}'::jsonb, 'publisher'),
		         ($5, $2, $3, 2, '1.0.0', $6, '{"name":"Gross revenue v2","aliases":["revenue"]}'::jsonb, 'publisher')`,
			[]any{revision1.UUID(), workspaceID.UUID(), assetID.UUID(), distributionDigest("revision-1"), revision2.UUID(), distributionDigest("revision-2")}},
		{`UPDATE semantic_assets SET current_revision_id = $1 WHERE workspace_id = $2 AND id = $3`, []any{revision2.UUID(), workspaceID.UUID(), assetID.UUID()}},
		{`INSERT INTO physical_bindings (id, workspace_id, asset_id, dataset_id, version, created_by)
		  VALUES ($1, $2, $3, $4, 1, 'publisher')`, []any{bindingID.UUID(), workspaceID.UUID(), assetID.UUID(), dataset1.UUID()}},
		{`INSERT INTO model_grains (id, workspace_id, asset_id, grain_expression, grain_field_refs, created_by)
		  VALUES ($1, $2, $3, 'one row per order', '[]'::jsonb, 'publisher')`, []any{grainID.UUID(), workspaceID.UUID(), assetID.UUID()}},
		{`INSERT INTO entity_keys (id, workspace_id, asset_id, key_field_refs, uniqueness_semantics, created_by)
		  VALUES ($1, $2, $3, '["order_id"]'::jsonb, 'exact', 'publisher')`, []any{entityKeyID.UUID(), workspaceID.UUID(), assetID.UUID()}},
		{`INSERT INTO releases (id, workspace_id, sequence, manifest_digest, published_by, published_at)
		  VALUES ($1, $2, 1, $3, 'publisher', '2026-09-04T08:00:00Z')`, []any{release1.UUID(), workspaceID.UUID(), distributionDigest("release-1")}},
		{`INSERT INTO release_assets (workspace_id, release_id, asset_id, revision_id, position)
		  VALUES ($1, $2, $3, $4, 1)`, []any{workspaceID.UUID(), release1.UUID(), assetID.UUID(), revision1.UUID()}},
		{`INSERT INTO release_objects (workspace_id, release_id, object_type, object_id, version, position)
		  VALUES ($1, $2, 'physical_binding', $3, 1, 1)`, []any{workspaceID.UUID(), release1.UUID(), bindingID.UUID()}},
		{`INSERT INTO release_objects (workspace_id, release_id, object_type, object_id, version, position)
		  VALUES ($1, $2, 'model_grain', $3, 1, 2), ($1, $2, 'entity_key', $4, 1, 3)`,
			[]any{workspaceID.UUID(), release1.UUID(), grainID.UUID(), entityKeyID.UUID()}},
		{`UPDATE physical_bindings SET dataset_id = $1, version = 2, updated_at = updated_at + interval '1 second'
		  WHERE workspace_id = $2 AND id = $3`, []any{dataset2.UUID(), workspaceID.UUID(), bindingID.UUID()}},
		{`INSERT INTO releases (id, workspace_id, sequence, manifest_digest, published_by, published_at)
		  VALUES ($1, $2, 2, $3, 'publisher', '2026-09-04T08:30:00Z')`, []any{release2.UUID(), workspaceID.UUID(), distributionDigest("release-2")}},
		{`INSERT INTO release_assets (workspace_id, release_id, asset_id, revision_id, position)
		  VALUES ($1, $2, $3, $4, 1)`, []any{workspaceID.UUID(), release2.UUID(), assetID.UUID(), revision2.UUID()}},
		{`INSERT INTO release_objects (workspace_id, release_id, object_type, object_id, version, position)
		  VALUES ($1, $2, 'physical_binding', $3, 2, 1)`, []any{workspaceID.UUID(), release2.UUID(), bindingID.UUID()}},
		{`INSERT INTO release_objects (workspace_id, release_id, object_type, object_id, version, position)
		  VALUES ($1, $2, 'model_grain', $3, 1, 2), ($1, $2, 'entity_key', $4, 1, 3)`,
			[]any{workspaceID.UUID(), release2.UUID(), grainID.UUID(), entityKeyID.UUID()}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seed semantic distribution: %v", err)
		}
	}
	return distributionFixture{workspaceID: workspaceID, assetID: assetID, revision1: revision1, revision2: revision2,
		release1: release1, release2: release2, bindingID: bindingID, grainID: grainID, entityKeyID: entityKeyID,
		dataset1: dataset1, dataset2: dataset2}
}

func distributionDigest(seed string) string {
	return fmt.Sprintf("sha256:%064x", seed)
}

func distributionQuery(context distribution.ResolutionContext) distribution.SemanticQueryInput {
	return distribution.SemanticQueryInput{SchemaVersion: distribution.QuerySchemaVersion, Intent: distribution.IntentAggregate,
		Measures: []distribution.Selector{{Address: "commerce.gross_revenue"}}, Context: context}
}

func TestSemanticDistributionPinsReleaseAndObjectSnapshots(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	fixture := seedDistributionReleases(t, pool)
	ctx := context.Background()
	service := distributionapp.NewService(pgstore.NewStore(pool), nil, distributionClock)

	consumer, err := service.CreateConsumer(ctx, distributionapp.CreateConsumerRequest{WorkspaceID: fixture.workspaceID,
		StableKey: "reporting-agent", Name: "Reporting agent", Kind: "agent", OwnerPrincipalRef: "local:test", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}
	pinned, err := service.CreateBinding(ctx, distributionapp.CreateBindingRequest{WorkspaceID: fixture.workspaceID,
		ConsumerID: consumer.ID, Environment: "prod", Purpose: "stable reports", Mode: distribution.BindingPinned,
		ReleaseID: &fixture.release1, CompatibilityConstraint: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("create pinned binding: %v", err)
	}

	current, err := service.Resolve(ctx, distributionapp.ResolveRequest{WorkspaceID: fixture.workspaceID,
		Input: distributionQuery(distribution.ResolutionContext{Mode: distribution.ResolutionCurrent}), Channel: "api",
		IdempotencyKey: "current-v2", PrincipalRef: "local:test", TraceID: distributionTraceID})
	if err != nil {
		t.Fatalf("resolve current release: %v", err)
	}
	assertDistributionPlan(t, current, fixture.release2, fixture.revision2, fixture.bindingID.String(), 2)
	assertResolvedObject(t, current, fixture.grainID.String(), 1)
	assertResolvedObject(t, current, fixture.entityKeyID.String(), 1)

	pinnedResult, err := service.Resolve(ctx, distributionapp.ResolveRequest{WorkspaceID: fixture.workspaceID,
		Input: distributionQuery(distribution.ResolutionContext{Mode: distribution.ResolutionBinding, BindingID: &pinned.ID}), Channel: "agent",
		IdempotencyKey: "pinned-v1", PrincipalRef: "local:test", TraceID: distributionTraceID})
	if err != nil {
		t.Fatalf("resolve pinned release: %v", err)
	}
	assertDistributionPlan(t, pinnedResult, fixture.release1, fixture.revision1, fixture.bindingID.String(), 1)
	assertResolvedObject(t, pinnedResult, fixture.grainID.String(), 1)
	assertResolvedObject(t, pinnedResult, fixture.entityKeyID.String(), 1)

	replayed, err := service.Resolve(ctx, distributionapp.ResolveRequest{WorkspaceID: fixture.workspaceID,
		Input: distributionQuery(distribution.ResolutionContext{Mode: distribution.ResolutionCurrent}), Channel: "api",
		IdempotencyKey: "current-v2", PrincipalRef: "local:test", TraceID: distributionTraceID})
	if err != nil || replayed.Query.ID != current.Query.ID || replayed.Plan == nil || replayed.Plan.ID != current.Plan.ID {
		t.Fatalf("idempotent replay = %+v, %v", replayed, err)
	}
	changed := distributionQuery(distribution.ResolutionContext{Mode: distribution.ResolutionCurrent})
	changed.Limit = 10
	if _, err := service.Resolve(ctx, distributionapp.ResolveRequest{WorkspaceID: fixture.workspaceID,
		Input: changed, Channel: "api", IdempotencyKey: "current-v2", PrincipalRef: "local:test", TraceID: distributionTraceID}); !errors.Is(err, distribution.ErrConflict) {
		t.Fatalf("changed idempotent request = %v, want conflict", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE physical_bindings SET dataset_id = $1, version = 3,
		updated_at = updated_at + interval '1 second' WHERE workspace_id = $2 AND id = $3`,
		fixture.dataset1.UUID(), fixture.workspaceID.UUID(), fixture.bindingID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE semantic_assets SET current_revision_id = $1 WHERE workspace_id = $2 AND id = $3`,
		fixture.revision1.UUID(), fixture.workspaceID.UUID(), fixture.assetID.UUID()); err != nil {
		t.Fatal(err)
	}
	afterMutation, err := service.Resolve(ctx, distributionapp.ResolveRequest{WorkspaceID: fixture.workspaceID,
		Input: distributionQuery(distribution.ResolutionContext{Mode: distribution.ResolutionExplicit, ReleaseID: &fixture.release2}), Channel: "api",
		IdempotencyKey: "explicit-v2-after-mutation", PrincipalRef: "local:test", TraceID: distributionTraceID})
	if err != nil {
		t.Fatalf("resolve immutable release after mutable updates: %v", err)
	}
	assertDistributionPlan(t, afterMutation, fixture.release2, fixture.revision2, fixture.bindingID.String(), 2)
	assertResolvedObject(t, afterMutation, fixture.grainID.String(), 1)
	assertResolvedObject(t, afterMutation, fixture.entityKeyID.String(), 1)

	stored, err := service.GetQuery(ctx, fixture.workspaceID, current.Query.ID, "local:test", distributionTraceID)
	if err != nil || stored.Plan == nil || stored.Plan.PlanDigest != current.Plan.PlanDigest {
		t.Fatalf("restart-stable stored result = %+v, %v", stored, err)
	}
	if _, err := service.Resolve(ctx, distributionapp.ResolveRequest{WorkspaceID: fixture.workspaceID,
		Input: distributionQuery(distribution.ResolutionContext{Mode: distribution.ResolutionCurrent}), Channel: "api",
		IdempotencyKey: "atomic-failure", PrincipalRef: "local:test", TraceID: "invalid-trace"}); !errors.Is(err, distribution.ErrInvariant) {
		t.Fatalf("invalid atomic record = %v, want invariant failure", err)
	}
	var failedRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM semantic_queries
		WHERE workspace_id = $1 AND idempotency_key = 'atomic-failure'`, fixture.workspaceID.UUID()).Scan(&failedRows); err != nil {
		t.Fatal(err)
	}
	if failedRows != 0 {
		t.Fatalf("failed atomic resolution left %d query rows", failedRows)
	}
	var resolutionEvents, audits int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM semantic_resolution_events WHERE workspace_id = $1),
		(SELECT count(*) FROM audit_events WHERE workspace_id = $1 AND event_type = 'semantic.resolution.resolved')`,
		fixture.workspaceID.UUID()).Scan(&resolutionEvents, &audits); err != nil {
		t.Fatal(err)
	}
	if resolutionEvents != 3 || audits != 3 {
		t.Fatalf("attribution rows events=%d audits=%d, want 3/3", resolutionEvents, audits)
	}
}

func TestSemanticDistributionCurrentFollowsRollbackRelease(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	fixture := seedDistributionReleases(t, pool)
	ctx := context.Background()
	release3, err := identity.NewReleaseID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE physical_bindings SET dataset_id = $1, version = 3,
		updated_at = updated_at + interval '1 second' WHERE workspace_id = $2 AND id = $3`,
		fixture.dataset1.UUID(), fixture.workspaceID.UUID(), fixture.bindingID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO releases
		(id, workspace_id, sequence, manifest_digest, rolled_back_to_release_id, published_by, published_at)
		VALUES ($1, $2, 3, $3, $4, 'publisher', '2026-09-04T09:00:00Z')`,
		release3.UUID(), fixture.workspaceID.UUID(), distributionDigest("rollback-release"), fixture.release2.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO release_assets
		(workspace_id, release_id, asset_id, revision_id, position) VALUES ($1, $2, $3, $4, 1)`,
		fixture.workspaceID.UUID(), release3.UUID(), fixture.assetID.UUID(), fixture.revision1.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO release_objects
		(workspace_id, release_id, object_type, object_id, version, position) VALUES ($1, $2, 'physical_binding', $3, 3, 1)`,
		fixture.workspaceID.UUID(), release3.UUID(), fixture.bindingID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO release_objects
		(workspace_id, release_id, object_type, object_id, version, position)
		VALUES ($1, $2, 'model_grain', $3, 1, 2), ($1, $2, 'entity_key', $4, 1, 3)`,
		fixture.workspaceID.UUID(), release3.UUID(), fixture.grainID.UUID(), fixture.entityKeyID.UUID()); err != nil {
		t.Fatal(err)
	}

	service := distributionapp.NewService(pgstore.NewStore(pool), nil, distributionClock)
	result, err := service.Resolve(ctx, distributionapp.ResolveRequest{WorkspaceID: fixture.workspaceID,
		Input: distributionQuery(distribution.ResolutionContext{Mode: distribution.ResolutionCurrent}), Channel: "api",
		IdempotencyKey: "current-after-rollback", PrincipalRef: "local:test", TraceID: distributionTraceID})
	if err != nil {
		t.Fatalf("resolve rollback release: %v", err)
	}
	assertDistributionPlan(t, result, release3, fixture.revision1, fixture.bindingID.String(), 3)
	assertResolvedObject(t, result, fixture.grainID.String(), 1)
	assertResolvedObject(t, result, fixture.entityKeyID.String(), 1)
}

func assertDistributionPlan(t *testing.T, result distributionapp.ResolutionResult, release identity.ReleaseID,
	revision identity.RevisionID, objectID string, objectVersion int,
) {
	t.Helper()
	if result.Refusal != nil || result.Plan == nil {
		t.Fatalf("resolution did not produce a plan: %+v", result)
	}
	plan := result.Plan
	if plan.ReleaseID != release || plan.ExecutionStatus != "not_configured" || plan.PlanDigest == "" {
		t.Fatalf("plan header = %+v", plan)
	}
	if len(plan.Assets) != 1 || plan.Assets[0].RevisionID != revision {
		t.Fatalf("resolved assets = %+v, want revision %s", plan.Assets, revision)
	}
	assertResolvedObject(t, result, objectID, objectVersion)
}

func assertResolvedObject(t *testing.T, result distributionapp.ResolutionResult, objectID string, objectVersion int) {
	t.Helper()
	if result.Plan == nil {
		t.Fatal("resolution plan is nil")
	}
	for _, object := range result.Plan.Objects {
		if object.ObjectID == objectID && object.Version == objectVersion {
			return
		}
	}
	t.Fatalf("resolved objects = %+v, want %s@%d", result.Plan.Objects, objectID, objectVersion)
}
