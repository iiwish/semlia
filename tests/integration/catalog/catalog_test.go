package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestProductionDisplayNameAppearsInCatalog(t *testing.T) {
	pool, _, service := fixture(t)
	workspaceID := createWorkspace(t, pool, "catalog-production-name")
	created, err := service.CreateAsset(context.Background(), application.CreateAssetRequest{
		WorkspaceID: workspaceID, Address: "demo.net_revenue", AssetType: semantic.Metric,
		Lifecycle: "active", SchemaVersion: "1.0.0", CreatedBy: "catalog-test", TraceID: traceID,
		Content: json.RawMessage(`{"displayName":"Synthetic net revenue","name":"Legacy name","definition":"Synthetic revenue after refunds"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != "Synthetic net revenue" {
		t.Fatalf("detail title = %q", created.Title)
	}
	page, err := service.ListAssets(context.Background(), application.ListAssetsRequest{WorkspaceID: workspaceID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "Synthetic net revenue" {
		t.Fatalf("catalog title mismatch: %+v", page.Items)
	}
}

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
	first, err := service.GetRevision(context.Background(), application.GetRevisionRequest{
		WorkspaceID: workspaceID, AssetID: created.ID, RevisionID: firstRevisionID,
	})
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
	if err != nil || len(revisionPage.Items) != 1 || revisionPage.Items[0].Sequence != 2 ||
		revisionPage.NextCursor == "" || revisionPage.Total != 2 {
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
	assertCatalogEventEnvelope(t, pool, workspaceID)

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

func TestAssetAuthorityUsesIndependentObjectPinsAndExactConsumerRelease(t *testing.T) {
	pool, _, service := fixture(t)
	workspace := createWorkspace(t, pool, "catalog-authority")
	created, err := service.CreateAsset(context.Background(), application.CreateAssetRequest{
		WorkspaceID: workspace, Address: "commerce.net_revenue", AssetType: semantic.Metric,
		Lifecycle: "active", SchemaVersion: "1.0.0", Content: json.RawMessage(`{"name":"Net revenue"}`),
		CreatedBy: "founder", TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	assetRelease := mustID(t, identity.NewReleaseID)
	objectRelease := mustID(t, identity.NewReleaseID)
	bindingID := mustID(t, identity.NewPhysicalBindingID)
	grainID := mustID(t, identity.NewModelGrainID)
	keyID := mustID(t, identity.NewEntityKeyID)
	joinID := mustID(t, identity.NewJoinContractID)
	datasetID := mustID(t, identity.NewPhysicalDatasetID)
	otherDatasetID := mustID(t, identity.NewPhysicalDatasetID)
	fieldID := mustID(t, identity.NewPhysicalFieldID)
	sourceID := mustID(t, identity.NewSourceConnectionID)
	sourceRevisionID := mustID(t, identity.NewSourceRevisionID)
	lineageID := mustID(t, identity.NewLineageEdgeID)
	consumerID := mustID(t, identity.NewConsumerID)
	consumerBindingID := mustID(t, identity.NewConsumerBindingID)
	pinnedConsumerBindingID := mustID(t, identity.NewConsumerBindingID)
	now := time.Now().UTC()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO releases (id,workspace_id,sequence,manifest_digest,state,published_by,published_at,created_at)
		VALUES ($1,$2,1,$3,'published','publisher',$4,$4),
		       ($5,$2,2,$6,'published','publisher',$4 + interval '1 minute',$4 + interval '1 minute')`,
		assetRelease.UUID(), workspace.UUID(), digest("asset-release"), now, objectRelease.UUID(), digest("object-release")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO release_assets (workspace_id,release_id,asset_id,revision_id,compatibility,position,created_at)
		VALUES ($1,$2,$3,$4,'{}',1,$5)`, workspace.UUID(), assetRelease.UUID(), created.ID.UUID(),
		created.CurrentRevision.ID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO release_object_snapshots (workspace_id,release_id,object_type,object_id,version,payload,position,created_at)
		VALUES ($1,$2,'physical_binding',$3,1,jsonb_build_object(
		           'asset_id',$4::text,'dataset_id',$5::text,'field_id',$6::text,'transform','sum(amount)'),1,$7),
		       ($1,$2,'model_grain',$8,1,jsonb_build_object(
		           'asset_id',$4::text,'grain_expression','one row per invoice',
		           'grain_field_refs',jsonb_build_array($6::text)),2,$7),
		       ($1,$2,'entity_key',$9,1,jsonb_build_object(
		           'asset_id',$4::text,'key_field_refs',jsonb_build_array($6::text),
		           'uniqueness_semantics','exact'),3,$7),
		       ($1,$2,'join_contract',$10,1,jsonb_build_object(
		           'left_dataset_id',$5::text,'right_dataset_id',$11::text,
		           'left_field_refs',jsonb_build_array($6::text),'right_field_refs',jsonb_build_array($6::text),
		           'join_type','left','cardinality','many_to_one','join_expression','left.id = right.id'),4,$7)`,
		workspace.UUID(), objectRelease.UUID(), bindingID.UUID(), created.ID.UUID(), datasetID.UUID(), fieldID.UUID(), now,
		grainID.UUID(), keyID.UUID(), joinID.UUID(), otherDatasetID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO source_connections (id,workspace_id,adapter_kind,name,normalized_locator,status,metadata)
		VALUES ($1,$2,'postgres','Catalog authority source','postgres://authority','active','{}')`,
		sourceID.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO source_revisions (id,workspace_id,source_connection_id,content_digest,adapter_version,observed_at)
		VALUES ($1,$2,$3,$4,'1.0.0',$5)`, sourceRevisionID.UUID(), workspace.UUID(), sourceID.UUID(),
		digest("authority-source"), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO physical_datasets (id,workspace_id,source_connection_id,external_key,qualified_name)
		VALUES ($1,$2,$3,'left','warehouse.left'),($4,$2,$3,'right','warehouse.right')`,
		datasetID.UUID(), workspace.UUID(), sourceID.UUID(), otherDatasetID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO lineage_edges (id,workspace_id,source_revision_id,upstream_dataset_id,downstream_dataset_id,edge_kind,confidence,created_at)
		VALUES ($1,$2,$3,$4,$5,'derived_from',0.875,$6)`,
		lineageID.UUID(), workspace.UUID(), sourceRevisionID.UUID(), datasetID.UUID(), otherDatasetID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO consumers (id,workspace_id,stable_key,name,kind,status,owner_principal_ref,created_at,updated_at)
		VALUES ($1,$2,'catalog-authority','Catalog authority','application','active','owner',$3,$3)`,
		consumerID.UUID(), workspace.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO consumer_bindings (id,workspace_id,consumer_id,environment,purpose,mode,release_id,status,version,created_at,updated_at)
		VALUES ($1,$2,$3,'prod','current','current',NULL,'active',1,$4,$4),
		       ($5,$2,$3,'archive','pinned','pinned',$6,'active',1,$4,$4)`,
		consumerBindingID.UUID(), workspace.UUID(), consumerID.UUID(), now,
		pinnedConsumerBindingID.UUID(), assetRelease.UUID()); err != nil {
		t.Fatal(err)
	}
	current, err := service.AppendRevision(context.Background(), application.AppendRevisionRequest{
		WorkspaceID: workspace, AssetID: created.ID, SchemaVersion: "1.1.0",
		Content: json.RawMessage(`{"name":"Net revenue v2"}`), CreatedBy: "founder", TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetAsset(context.Background(), workspace, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	sections := make(map[string]domain.AuthoritySection, len(detail.AuthoritySections))
	for _, section := range detail.AuthoritySections {
		sections[section.Kind] = section
	}
	physical := sections["physical_bindings"]
	if physical.Availability != domain.AvailabilityAvailable || physical.ReleaseID == nil ||
		*physical.ReleaseID != objectRelease || physical.Values["bindingCount"] != 1 ||
		physical.Values["grainCount"] != 1 || physical.Values["entityKeyCount"] != 1 || len(physical.Records) != 3 {
		t.Fatalf("independent object authority = %+v", physical)
	}
	records := make(map[string]domain.AuthorityRecord, len(physical.Records))
	for _, record := range physical.Records {
		records[record.Kind] = record
	}
	if record := records["physical_binding"]; record.ID != bindingID.String() || record.PhysicalBinding == nil ||
		record.PhysicalBinding.DatasetID != datasetID || record.PhysicalBinding.Transform != "sum(amount)" {
		t.Fatalf("physical binding authority = %+v", record)
	}
	if record := records["model_grain"]; record.ID != grainID.String() || record.ModelGrain == nil ||
		record.ModelGrain.GrainExpression != "one row per invoice" || len(record.ModelGrain.GrainFieldRefs) != 1 {
		t.Fatalf("model grain authority = %+v", record)
	}
	if record := records["entity_key"]; record.ID != keyID.String() || record.EntityKey == nil ||
		record.EntityKey.UniquenessSemantics != "exact" || len(record.EntityKey.KeyFieldRefs) != 1 {
		t.Fatalf("entity key authority = %+v", record)
	}
	joins := sections["join_contracts"]
	if len(joins.Records) != 1 || joins.Records[0].ID != joinID.String() || joins.Records[0].JoinContract == nil ||
		joins.Records[0].JoinContract.Direction != "outgoing" || joins.Records[0].JoinContract.RightDatasetID != otherDatasetID ||
		joins.Records[0].JoinContract.JoinType != "left" || joins.Records[0].JoinContract.JoinExpression != "left.id = right.id" {
		t.Fatalf("join authority = %+v", joins)
	}
	lineage := sections["lineage"]
	if lineage.RevisionID != nil || lineage.ReleaseID != nil || len(lineage.Records) != 1 ||
		lineage.Records[0].Lineage == nil || lineage.Records[0].Lineage.SourceRevisionID != sourceRevisionID ||
		lineage.Records[0].Lineage.Confidence != 0.875 {
		t.Fatalf("lineage authority = %+v", lineage)
	}
	if evidence := sections["evidence"]; evidence.RevisionID == nil || *evidence.RevisionID != current.ID ||
		evidence.ReleaseID != nil || evidence.ReleaseSequence != nil {
		t.Fatalf("current evidence authority = %+v, current revision = %s", evidence, current.ID)
	}
	if validation := sections["validation"]; validation.Availability != domain.AvailabilityNotConfigured ||
		validation.Values["runCount"] != 0 || len(validation.Records) != 0 {
		t.Fatalf("zero-run validation authority = %+v", validation)
	}
	impact := sections["consumer_impact"]
	if impact.Values["currentConsumerCount"] != 0 || impact.Values["pinnedConsumerCount"] != 1 ||
		len(impact.Records) != 1 || impact.Records[0].ID != pinnedConsumerBindingID.String() ||
		impact.Records[0].ConsumerBinding == nil ||
		impact.Records[0].ConsumerBinding.EffectiveReleaseID != assetRelease ||
		impact.Records[0].ConsumerBinding.Mode != "pinned" {
		t.Fatalf("consumer binding authority = %+v", impact)
	}
}

func TestAuthorityRecordsAreCursorPagedAndLegacyGraphFailsExplicitlyAtLimit(t *testing.T) {
	pool, _, service := fixture(t)
	workspace := createWorkspace(t, pool, "catalog-authority-paging")
	created, err := service.CreateAsset(context.Background(), application.CreateAssetRequest{
		WorkspaceID: workspace, Address: "commerce.paging_root", AssetType: semantic.Metric,
		Lifecycle: "active", SchemaVersion: "1.0.0", Content: json.RawMessage(`{"name":"Paging root"}`),
		CreatedBy: "founder", TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := pool.Exec(context.Background(), `
		WITH peers AS MATERIALIZED (
			SELECT semlia_seed_uuidv7('authority-peer', $1::uuid::text || ':' || value::text,
			                           $3::timestamptz + value * interval '1 microsecond') AS id,
			       value
			FROM generate_series(1, 205) AS value
		), inserted AS (
			INSERT INTO semantic_assets (id,workspace_id,namespace,key,asset_type,lifecycle_state,created_at,updated_at)
			SELECT id,$1::uuid,'commerce','peer_' || value::text,'metric','active',$3,$3 FROM peers
			RETURNING id
		)
		INSERT INTO semantic_relations (
			id,workspace_id,subject_asset_id,predicate,object_asset_id,plane,assertion_state,created_by,created_at
		)
		SELECT semlia_seed_uuidv7('authority-relation', $1::uuid::text || ':' || inserted.id::text, $3),
		       $1::uuid,$2::uuid,'synonym_of',inserted.id,'taxonomy','asserted','test',$3
		FROM inserted`, workspace.UUID(), created.ID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetAsset(context.Background(), workspace, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var relations domain.AuthoritySection
	for _, section := range detail.AuthoritySections {
		if section.Kind == "relations" {
			relations = section
			break
		}
	}
	if len(relations.Records) != 100 || relations.RecordsTotal != 205 || relations.RecordsNextCursor == "" ||
		relations.Records[0].Relation == nil {
		t.Fatalf("first authority page = %+v", relations)
	}
	second, err := service.ListAuthorityRecords(context.Background(), application.ListAuthorityRecordsRequest{
		WorkspaceID: workspace, AssetID: created.ID, Section: "relations", Limit: 100,
		Cursor: relations.RecordsNextCursor,
	})
	if err != nil || len(second.Items) != 100 || second.Total != 205 || second.NextCursor == "" {
		t.Fatalf("second authority page = %+v, err = %v", second, err)
	}
	third, err := service.ListAuthorityRecords(context.Background(), application.ListAuthorityRecordsRequest{
		WorkspaceID: workspace, AssetID: created.ID, Section: "relations", Limit: 100,
		Cursor: second.NextCursor,
	})
	if err != nil || len(third.Items) != 5 || third.Total != 205 || third.NextCursor != "" {
		t.Fatalf("third authority page = %+v, err = %v", third, err)
	}
	if _, err := service.ListRelations(context.Background(), application.ListRelationsRequest{
		WorkspaceID: workspace, AssetID: created.ID, Depth: 1, Limit: 50,
	}); !errors.Is(err, domain.ErrInvariant) {
		t.Fatalf("legacy graph overflow error = %v, want explicit invariant", err)
	}
}

func TestReleasedAssetWithoutGovernedObjectsReportsNotConfigured(t *testing.T) {
	pool, _, service := fixture(t)
	workspace := createWorkspace(t, pool, "catalog-authority-empty-object-sections")
	created, err := service.CreateAsset(context.Background(), application.CreateAssetRequest{
		WorkspaceID: workspace, Address: "commerce.release_only", AssetType: semantic.Metric,
		Lifecycle: "active", SchemaVersion: "1.0.0", Content: json.RawMessage(`{"name":"Release only"}`),
		CreatedBy: "founder", TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := service.GetAsset(context.Background(), workspace, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range before.AuthoritySections {
		if (section.Kind == "physical_bindings" || section.Kind == "join_contracts") &&
			section.Availability != domain.AvailabilityNotReleased {
			t.Fatalf("unreleased %s availability = %s", section.Kind, section.Availability)
		}
	}
	releaseID := mustID(t, identity.NewReleaseID)
	now := time.Now().UTC()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO releases (id,workspace_id,sequence,manifest_digest,state,published_by,published_at,created_at)
		VALUES ($1,$2,1,$3,'published','publisher',$4,$4)`,
		releaseID.UUID(), workspace.UUID(), "sha256:"+strings.Repeat("1", 64), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO release_assets (workspace_id,release_id,asset_id,revision_id,compatibility,position,created_at)
		VALUES ($1,$2,$3,$4,'{}',1,$5)`,
		workspace.UUID(), releaseID.UUID(), created.ID.UUID(), created.CurrentRevision.ID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	after, err := service.GetAsset(context.Background(), workspace, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range after.AuthoritySections {
		if section.Kind != "physical_bindings" && section.Kind != "join_contracts" &&
			section.Kind != "validation" && section.Kind != "trust" {
			continue
		}
		if section.Availability != domain.AvailabilityNotConfigured || section.ReleaseID != nil ||
			section.ReleaseSequence != nil || len(section.Records) != 0 {
			t.Fatalf("released empty %s authority = %+v", section.Kind, section)
		}
	}
}

func TestWorkspaceBootstrapIsAuditedAndListable(t *testing.T) {
	pool, _, service := fixture(t)
	created, err := service.CreateWorkspace(context.Background(), "semantic-core", "Semantic Core", traceID)
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.ListWorkspaces(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("workspaces = %+v, err = %v", items, err)
	}
	var auditCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'workspace.created'`, created.ID.UUID()).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("workspace audit count = %d", auditCount)
	}
	if _, err := service.CreateWorkspace(context.Background(), "semantic-core", "Duplicate", traceID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func assertCatalogEventEnvelope(t *testing.T, pool *pgstore.Pool, workspace identity.WorkspaceID) {
	t.Helper()
	var eventType, traceID string
	var payload []byte
	if err := pool.QueryRow(context.Background(), `
		SELECT event_type, trace_id, payload
		FROM outbox_events
		WHERE workspace_id = $1
		ORDER BY created_at, id
		LIMIT 1`, workspace.UUID()).Scan(&eventType, &traceID, &payload); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		SpecVersion string          `json:"specVersion"`
		ID          string          `json:"id"`
		Type        string          `json:"type"`
		Source      string          `json:"source"`
		WorkspaceID string          `json:"workspaceId"`
		TraceID     string          `json:"traceId"`
		Data        json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SpecVersion != "semlia.events/v1" || envelope.Type != eventType ||
		envelope.WorkspaceID != workspace.String() || envelope.TraceID != traceID ||
		envelope.Source != "urn:semlia:control-plane" || envelope.ID == "" || len(envelope.Data) == 0 {
		t.Fatalf("event envelope = %+v", envelope)
	}
	if _, err := identity.ParseEventID(envelope.ID); err != nil {
		t.Fatalf("event envelope ID = %s: %v", envelope.ID, err)
	}
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
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" || first.Total != 5 {
		t.Fatalf("first page = %+v, err = %v", first, err)
	}
	second, err := service.ListAssets(context.Background(), application.ListAssetsRequest{
		WorkspaceID: workspaceID, AssetType: semantic.Metric, Limit: 2, Cursor: first.NextCursor,
	})
	if err != nil || len(second.Items) != 2 || second.NextCursor == "" || second.Total != 5 {
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

func TestCatalogTenThousandAssetsRemainCursorPagedWithServerTotal(t *testing.T) {
	pool, _, service := fixture(t)
	workspaceID := createWorkspace(t, pool, "catalog-10k")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO semantic_assets (
			id,workspace_id,namespace,key,asset_type,lifecycle_state,created_at,updated_at
		)
		SELECT semlia_seed_uuidv7(
				'catalog-10k', $1::uuid::text || ':' || value::text,
				$2::timestamptz + value * interval '1 microsecond'
			),
			$1::uuid,'scale','asset_' || lpad(value::text,5,'0'),'metric','active',
			$2::timestamptz + value * interval '1 microsecond',
			$2::timestamptz + value * interval '1 microsecond'
		FROM generate_series(1,10000) AS value`, workspaceID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	first, err := service.ListAssets(context.Background(), application.ListAssetsRequest{
		WorkspaceID: workspaceID, AssetType: semantic.Metric, Limit: 100,
	})
	if err != nil || len(first.Items) != 100 || first.Total != 10000 || first.NextCursor == "" {
		t.Fatalf("10k first page count=%d total=%d cursor=%t err=%v",
			len(first.Items), first.Total, first.NextCursor != "", err)
	}
	second, err := service.ListAssets(context.Background(), application.ListAssetsRequest{
		WorkspaceID: workspaceID, AssetType: semantic.Metric, Limit: 100, Cursor: first.NextCursor,
	})
	if err != nil || len(second.Items) != 100 || second.Total != 10000 || second.NextCursor == "" {
		t.Fatalf("10k second page count=%d total=%d cursor=%t err=%v",
			len(second.Items), second.Total, second.NextCursor != "", err)
	}
	if first.Items[len(first.Items)-1].ID == second.Items[0].ID {
		t.Fatal("10k cursor repeated the page boundary asset")
	}
}

func TestBoundedRelationsAndDiscoveryRunProjection(t *testing.T) {
	pool, store, service := fixture(t)
	workspaceID := createWorkspace(t, pool, "catalog-relations")
	assets := make([]domain.AssetDetail, 0, 4)
	for _, address := range []string{"commerce.a", "commerce.b", "commerce.c", "commerce.d"} {
		assets = append(assets, createAsset(t, service, workspaceID, address, semantic.BusinessTerm, `{"name":"Concept"}`))
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
	relations, err := service.ListRelations(context.Background(), application.ListRelationsRequest{
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

func TestRelationPlanesProjectHierarchySemanticAndImpact(t *testing.T) {
	pool, store, service := fixture(t)
	workspaceID := createWorkspace(t, pool, "catalog-relation-planes")
	root := createAsset(t, service, workspaceID, "commerce.net_revenue", semantic.Metric, `{"name":"Net revenue"}`)
	taxonomyTarget := createAsset(t, service, workspaceID, "commerce.gross_revenue", semantic.Metric, `{"name":"Gross revenue"}`)
	semanticTarget := createAsset(t, service, workspaceID, "commerce.order", semantic.BusinessObject, `{"name":"Order"}`)
	impactTarget := createAsset(t, service, workspaceID, "commerce.recognized_revenue", semantic.Metric, `{"name":"Recognized revenue"}`)

	relations := []semantic.RelationRecord{
		{
			ID: mustID(t, identity.NewRelationID), WorkspaceID: workspaceID,
			SubjectAssetID: root.ID, Predicate: semantic.BroaderThan, ObjectAssetID: taxonomyTarget.ID,
			Plane: semantic.TaxonomyPlane, AssertionState: semantic.Asserted, CreatedBy: "founder",
		},
		{
			ID: mustID(t, identity.NewRelationID), WorkspaceID: workspaceID,
			SubjectAssetID: root.ID, Predicate: semantic.Measures, ObjectAssetID: semanticTarget.ID,
			Plane: semantic.SemanticPlane, AssertionState: semantic.Asserted, CreatedBy: "founder",
		},
		{
			ID: mustID(t, identity.NewRelationID), WorkspaceID: workspaceID,
			SubjectAssetID: root.ID, Predicate: semantic.DerivedFrom, ObjectAssetID: impactTarget.ID,
			Plane: semantic.DependencyPlane, AssertionState: semantic.Asserted, CreatedBy: "founder",
		},
	}
	for _, relation := range relations {
		if _, err := store.CreateRelation(context.Background(), relation); err != nil {
			t.Fatal(err)
		}
	}

	expected := map[semantic.RelationPlane]semantic.RelationPredicate{
		semantic.TaxonomyPlane:   semantic.BroaderThan,
		semantic.SemanticPlane:   semantic.Measures,
		semantic.DependencyPlane: semantic.DerivedFrom,
	}
	for plane, predicate := range expected {
		projected, err := service.ListRelations(context.Background(), application.ListRelationsRequest{
			WorkspaceID: workspaceID, AssetID: root.ID, Direction: "outgoing", Plane: plane, Depth: 1,
		})
		if err != nil {
			t.Fatalf("project %s relations: %v", plane, err)
		}
		if len(projected) != 1 || projected[0].Plane != plane || projected[0].Predicate != predicate {
			t.Fatalf("%s projection = %+v, want only %s", plane, projected, predicate)
		}
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
