package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestRegistryRepositoryRoundTripAndInvariants(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	store := pgstore.NewStore(pool)
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	otherWorkspaceID := newWorkspaceID(t)
	createRegistryWorkspace(t, pool, workspaceID, "registry")
	createRegistryWorkspace(t, pool, otherWorkspaceID, "other")

	sourceID := mustIdentity(t, identity.NewSourceConnectionID)
	source, err := store.CreateSourceConnection(ctx, semantic.SourceConnection{
		ID: sourceID, WorkspaceID: workspaceID, AdapterKind: "postgres", Name: "Warehouse",
		NormalizedLocator: "postgres://warehouse/catalog", CredentialRef: "vault/warehouse/read",
		Status: "active", Metadata: []byte(`{"environment":"test"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.ID != sourceID || source.WorkspaceID != workspaceID || source.CredentialRef != "vault/warehouse/read" {
		t.Fatalf("source round trip = %+v", source)
	}

	sourceRevisionID := mustIdentity(t, identity.NewSourceRevisionID)
	sourceRevision, err := store.CreateSourceRevision(ctx, semantic.SourceRevision{
		ID: sourceRevisionID, WorkspaceID: workspaceID, SourceConnectionID: sourceID,
		ExternalRevision: "lsn:42", ContentDigest: digest("1"), AdapterVersion: "postgres/1.0.0",
		ObservedAt: time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC), Metadata: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.CreateSourceRevision(ctx, semantic.SourceRevision{
		ID: mustIdentity(t, identity.NewSourceRevisionID), WorkspaceID: workspaceID, SourceConnectionID: sourceID,
		ContentDigest: digest("1"), AdapterVersion: "postgres/1.0.0",
		ObservedAt: time.Date(2026, 9, 2, 8, 1, 0, 0, time.UTC), Metadata: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != sourceRevision.ID {
		t.Fatalf("idempotent source revision = %s, want %s", replayed.ID, sourceRevision.ID)
	}
	if _, err := store.CreateSourceRevision(ctx, semantic.SourceRevision{
		ID: mustIdentity(t, identity.NewSourceRevisionID), WorkspaceID: otherWorkspaceID,
		SourceConnectionID: sourceID, ContentDigest: digest("2"), AdapterVersion: "postgres/1.0.0",
		ObservedAt: time.Now(), Metadata: []byte(`{}`),
	}); !errors.Is(err, semantic.ErrInvariant) {
		t.Fatalf("cross-workspace source revision error = %v", err)
	}

	metric := createRegistryAsset(t, store, workspaceID, "net_revenue", semantic.Metric)
	entity := createRegistryAsset(t, store, workspaceID, "order", semantic.Entity)
	if _, err := store.CreateAsset(ctx, metric); !errors.Is(err, semantic.ErrConflict) {
		t.Fatalf("duplicate semantic address error = %v", err)
	}
	revisionID := mustIdentity(t, identity.NewRevisionID)
	revision, err := store.CreateAssetRevision(ctx, semantic.RevisionRecord{
		ID: revisionID, WorkspaceID: workspaceID, AssetID: metric.ID, Sequence: 1,
		SchemaVersion: "1.0.0", ContentDigest: digest("3"), Content: []byte(`{"name":"Net revenue"}`),
		CreatedBy: "founder",
	})
	if err != nil {
		t.Fatal(err)
	}
	if revision.ID != revisionID || revision.AssetID != metric.ID {
		t.Fatalf("revision round trip = %+v", revision)
	}
	var currentRevision string
	if err := pool.QueryRow(ctx, "SELECT current_revision_id::text FROM semantic_assets WHERE id = $1", metric.ID.UUID()).Scan(&currentRevision); err != nil {
		t.Fatal(err)
	}
	if currentRevision != revisionID.UUID() {
		t.Fatalf("current revision = %s, want %s", currentRevision, revisionID.UUID())
	}

	evidenceID := mustIdentity(t, identity.NewEvidenceID)
	evidence, err := store.CreateEvidence(ctx, semantic.EvidenceArtifact{
		ID: evidenceID, WorkspaceID: workspaceID, EvidenceType: "observed",
		SourceRevisionID: &sourceRevisionID, Locator: "catalog.public.orders",
		ContentDigest: digest("4"), Metadata: []byte(`{"column":"revenue"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.ID != evidenceID || evidence.SourceRevisionID == nil || *evidence.SourceRevisionID != sourceRevisionID {
		t.Fatalf("evidence round trip = %+v", evidence)
	}

	relationID := mustIdentity(t, identity.NewRelationID)
	relation, err := store.CreateRelation(ctx, semantic.RelationRecord{
		ID: relationID, WorkspaceID: workspaceID, SubjectAssetID: metric.ID,
		Predicate: semantic.Measures, ObjectAssetID: entity.ID, Plane: semantic.SemanticPlane,
		AssertionState: semantic.Asserted, SourceRevisionID: &sourceRevisionID,
		EvidenceArtifactID: &evidenceID, CreatedBy: "founder",
	})
	if err != nil {
		t.Fatal(err)
	}
	if relation.ID != relationID || relation.SubjectAssetID != metric.ID || relation.ObjectAssetID != entity.ID {
		t.Fatalf("relation round trip = %+v", relation)
	}
	if _, err := store.CreateRelation(ctx, semantic.RelationRecord{
		ID: mustIdentity(t, identity.NewRelationID), WorkspaceID: workspaceID,
		SubjectAssetID: metric.ID, Predicate: semantic.Measures, ObjectAssetID: entity.ID,
		Plane: semantic.TaxonomyPlane, AssertionState: semantic.Asserted, CreatedBy: "founder",
	}); !errors.Is(err, semantic.ErrInvariant) {
		t.Fatalf("invalid relation policy error = %v", err)
	}

	candidateID := mustIdentity(t, identity.NewRelationID)
	if _, err := store.CreateRelation(ctx, semantic.RelationRecord{
		ID: candidateID, WorkspaceID: workspaceID, SubjectAssetID: metric.ID,
		Predicate: semantic.Measures, ObjectAssetID: entity.ID, Plane: semantic.SemanticPlane,
		AssertionState: semantic.Candidate, CreatedBy: "founder",
	}); err != nil {
		t.Fatal(err)
	}
	candidateOntologyID := createOntology(t, store, workspaceID, 1, "5")
	if err := store.AddOntologyRelation(ctx, workspaceID, candidateOntologyID, candidateID); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishOntologyRevision(ctx, workspaceID, candidateOntologyID, time.Now()); !errors.Is(err, semantic.ErrInvariant) {
		t.Fatalf("candidate ontology publication error = %v", err)
	}

	publishedOntologyID := createOntology(t, store, workspaceID, 2, "6")
	if err := store.AddOntologyRelation(ctx, workspaceID, publishedOntologyID, relationID); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishOntologyRevision(ctx, workspaceID, publishedOntologyID, time.Now()); err != nil {
		t.Fatal(err)
	}
	otherOntologyID := createOntology(t, store, otherWorkspaceID, 1, "7")
	if err := store.AddOntologyRelation(ctx, otherWorkspaceID, otherOntologyID, relationID); !errors.Is(err, semantic.ErrInvariant) {
		t.Fatalf("cross-workspace ontology membership error = %v", err)
	}

	for _, mutation := range []struct {
		name  string
		query string
		id    string
	}{
		{"source revision", "UPDATE source_revisions SET external_revision = 'changed' WHERE id = $1", sourceRevisionID.UUID()},
		{"asset revision", "UPDATE asset_revisions SET created_by = 'changed' WHERE id = $1", revisionID.UUID()},
		{"evidence", "DELETE FROM evidence_artifacts WHERE id = $1", evidenceID.UUID()},
		{"relation", "DELETE FROM semantic_relations WHERE id = $1", relationID.UUID()},
	} {
		t.Run("immutable "+mutation.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, mutation.query, mutation.id); err == nil {
				t.Fatalf("%s mutation was accepted", mutation.name)
			}
		})
	}
}

func TestPhysicalObservationSchemaInvariants(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	store := pgstore.NewStore(pool)
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	otherWorkspaceID := newWorkspaceID(t)
	createRegistryWorkspace(t, pool, workspaceID, "physical")
	createRegistryWorkspace(t, pool, otherWorkspaceID, "physical-other")
	sourceID := mustIdentity(t, identity.NewSourceConnectionID)
	if _, err := store.CreateSourceConnection(ctx, semantic.SourceConnection{
		ID: sourceID, WorkspaceID: workspaceID, AdapterKind: "postgres", Name: "Warehouse",
		NormalizedLocator: "postgres://warehouse/physical", Status: "active", Metadata: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	sourceRevisionID := mustIdentity(t, identity.NewSourceRevisionID)
	if _, err := store.CreateSourceRevision(ctx, semantic.SourceRevision{
		ID: sourceRevisionID, WorkspaceID: workspaceID, SourceConnectionID: sourceID,
		ContentDigest: digest("8"), AdapterVersion: "postgres/1.0.0", ObservedAt: time.Now(), Metadata: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	datasetID := mustIdentity(t, identity.NewPhysicalDatasetID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO physical_datasets (id, workspace_id, source_connection_id, external_key, qualified_name)
		VALUES ($1, $2, $3, 'public.orders', 'warehouse.public.orders')`,
		datasetID.UUID(), workspaceID.UUID(), sourceID.UUID()); err != nil {
		t.Fatal(err)
	}
	datasetRevisionID := mustIdentity(t, identity.NewPhysicalDatasetRevisionID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO physical_dataset_revisions (
			id, workspace_id, physical_dataset_id, source_revision_id, dataset_kind, locator, content_digest, metadata
		) VALUES ($1, $2, $3, $4, 'table', 'public.orders', $5, '{}'::jsonb)`,
		datasetRevisionID.UUID(), workspaceID.UUID(), datasetID.UUID(), sourceRevisionID.UUID(), digest("9")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE physical_datasets SET current_revision_id = $1 WHERE id = $2", datasetRevisionID.UUID(), datasetID.UUID()); err != nil {
		t.Fatal(err)
	}
	fieldID := mustIdentity(t, identity.NewPhysicalFieldID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO physical_fields (id, workspace_id, physical_dataset_id, external_key, name)
		VALUES ($1, $2, $3, 'order_id', 'order_id')`, fieldID.UUID(), workspaceID.UUID(), datasetID.UUID()); err != nil {
		t.Fatal(err)
	}
	fieldRevisionID := mustIdentity(t, identity.NewPhysicalFieldRevisionID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO physical_field_revisions (
			id, workspace_id, physical_field_id, dataset_revision_id, ordinal, data_type, nullable, metadata
		) VALUES ($1, $2, $3, $4, 1, 'uuid', false, '{}'::jsonb)`,
		fieldRevisionID.UUID(), workspaceID.UUID(), fieldID.UUID(), datasetRevisionID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE physical_fields SET current_revision_id = $1 WHERE id = $2", fieldRevisionID.UUID(), fieldID.UUID()); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, "UPDATE physical_dataset_revisions SET locator = 'changed' WHERE id = $1", datasetRevisionID.UUID()); err == nil {
		t.Fatal("physical dataset revision mutation was accepted")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM physical_field_revisions WHERE id = $1", fieldRevisionID.UUID()); err == nil {
		t.Fatal("physical field revision deletion was accepted")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO physical_datasets (id, workspace_id, source_connection_id, external_key, qualified_name)
		VALUES ($1, $2, $3, 'cross', 'cross.workspace')`,
		mustIdentity(t, identity.NewPhysicalDatasetID).UUID(), otherWorkspaceID.UUID(), sourceID.UUID()); err == nil {
		t.Fatal("cross-workspace physical dataset was accepted")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO code_artifacts (id, workspace_id, source_revision_id, path, language, content_digest)
		VALUES ($1, $2, $3, '../secret.sql', 'sql', $4)`,
		mustIdentity(t, identity.NewCodeArtifactID).UUID(), workspaceID.UUID(), sourceRevisionID.UUID(), digest("a")); err == nil {
		t.Fatal("path traversal code artifact was accepted")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO lineage_edges (
			id, workspace_id, source_revision_id, upstream_dataset_id, downstream_dataset_id, edge_kind, confidence
		) VALUES ($1, $2, $3, $4, $4, 'derived_from', 1)`,
		mustIdentity(t, identity.NewLineageEdgeID).UUID(), workspaceID.UUID(), sourceRevisionID.UUID(), datasetID.UUID()); err == nil {
		t.Fatal("self lineage edge was accepted")
	}
}

func createRegistryWorkspace(t *testing.T, pool *pgstore.Pool, id identity.WorkspaceID, slug string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO workspaces (id, slug, display_name) VALUES ($1, $2, $2)`, id.UUID(), slug); err != nil {
		t.Fatal(err)
	}
}

func createRegistryAsset(
	t *testing.T,
	store *pgstore.Store,
	workspaceID identity.WorkspaceID,
	key string,
	assetType semantic.AssetType,
) semantic.Asset {
	t.Helper()
	address, err := semantic.NewAddress("commerce", key)
	if err != nil {
		t.Fatal(err)
	}
	asset := semantic.Asset{
		ID: mustIdentity(t, identity.NewAssetID), WorkspaceID: workspaceID,
		Address: address, Type: assetType, LifecycleState: "draft",
	}
	created, err := store.CreateAsset(context.Background(), asset)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func createOntology(
	t *testing.T,
	store *pgstore.Store,
	workspaceID identity.WorkspaceID,
	sequence int64,
	digestCharacter string,
) identity.OntologyID {
	t.Helper()
	id := mustIdentity(t, identity.NewOntologyID)
	created, err := store.CreateOntologyRevision(context.Background(), semantic.OntologyRevision{
		ID: id, WorkspaceID: workspaceID, Sequence: sequence, Status: "draft",
		ContentDigest: digest(digestCharacter), CreatedBy: "founder",
	})
	if err != nil {
		t.Fatal(err)
	}
	return created.ID
}

func mustIdentity[T any](t *testing.T, create func() (T, error)) T {
	t.Helper()
	id, err := create()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func digest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}
