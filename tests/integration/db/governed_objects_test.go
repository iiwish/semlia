package db_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// governedObjectFixture seeds one workspace with the minimal M1 physical
// graph (one source, two datasets, fields on the left dataset) plus one
// metric asset, so the four governance objects have valid references.
type governedObjectFixture struct {
	WorkspaceID       identity.WorkspaceID
	AssetID           identity.AssetID
	SourceID          identity.SourceConnectionID
	SourceRevisionID  identity.SourceRevisionID
	LeftDatasetID     identity.PhysicalDatasetID
	RightDatasetID    identity.PhysicalDatasetID
	LeftFieldID       identity.PhysicalFieldID
	SecondLeftFieldID identity.PhysicalFieldID
	RightFieldID      identity.PhysicalFieldID
}

func seedGovernedObjectFixture(t *testing.T, pool *pgstore.Pool, slug string) governedObjectFixture {
	t.Helper()
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ($1, $2, $3)`, workspaceID.UUID(), slug, "Governance objects "+slug); err != nil {
		t.Fatal(err)
	}
	assetID, err := identity.NewAssetID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO semantic_assets (id, workspace_id, namespace, key, asset_type, lifecycle_state)
		VALUES ($1, $2, 'finance', $3, 'metric', 'draft')`,
		assetID.UUID(), workspaceID.UUID(), slug+"_metric"); err != nil {
		t.Fatal(err)
	}
	sourceID, err := identity.NewSourceConnectionID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO source_connections (id, workspace_id, adapter_kind, name, normalized_locator, status, metadata)
		VALUES ($1, $2, 'postgres', $3, $4, 'active', '{}'::jsonb)`,
		sourceID.UUID(), workspaceID.UUID(), slug+" warehouse", "postgres://"+slug+"/catalog"); err != nil {
		t.Fatal(err)
	}
	sourceRevisionID, err := identity.NewSourceRevisionID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO source_revisions (id, workspace_id, source_connection_id, content_digest, adapter_version, observed_at)
		VALUES ($1, $2, $3, $4, 'postgres/1.0.0', CURRENT_TIMESTAMP)`,
		sourceRevisionID.UUID(), workspaceID.UUID(), sourceID.UUID(), sha256FixtureDigest(slug+"-source")); err != nil {
		t.Fatal(err)
	}
	dataset := func(externalKey, qualifiedName string) identity.PhysicalDatasetID {
		id, datasetErr := identity.NewPhysicalDatasetID()
		if datasetErr != nil {
			t.Fatal(datasetErr)
		}
		if _, datasetErr := pool.Exec(ctx, `
			INSERT INTO physical_datasets (id, workspace_id, source_connection_id, external_key, qualified_name)
			VALUES ($1, $2, $3, $4, $5)`,
			id.UUID(), workspaceID.UUID(), sourceID.UUID(), externalKey, qualifiedName); datasetErr != nil {
			t.Fatal(datasetErr)
		}
		return id
	}
	leftDatasetID := dataset(slug+".orders", slug+" warehouse.public.orders")
	rightDatasetID := dataset(slug+".customers", slug+" warehouse.public.customers")
	field := func(datasetID identity.PhysicalDatasetID, externalKey string) identity.PhysicalFieldID {
		id, fieldErr := identity.NewPhysicalFieldID()
		if fieldErr != nil {
			t.Fatal(fieldErr)
		}
		if _, fieldErr := pool.Exec(ctx, `
			INSERT INTO physical_fields (id, workspace_id, physical_dataset_id, external_key, name)
			VALUES ($1, $2, $3, $4, $4)`,
			id.UUID(), workspaceID.UUID(), datasetID.UUID(), externalKey); fieldErr != nil {
			t.Fatal(fieldErr)
		}
		return id
	}
	return governedObjectFixture{
		WorkspaceID: workspaceID, AssetID: assetID,
		SourceID: sourceID, SourceRevisionID: sourceRevisionID,
		LeftDatasetID: leftDatasetID, RightDatasetID: rightDatasetID,
		LeftFieldID:       field(leftDatasetID, "order_id"),
		SecondLeftFieldID: field(leftDatasetID, "customer_id"),
		RightFieldID:      field(rightDatasetID, "customer_id"),
	}
}

func governedObjectServices(pool *pgstore.Pool) (*governanceapp.GovernedObjectService, *governanceapp.ProposalService) {
	store := pgstore.NewStore(pool)
	return governanceapp.NewGovernedObjectService(store, governanceClock),
		governanceapp.NewProposalService(store, governanceClock)
}

func validFixtureGrain(fixture governedObjectFixture) governance.GovernedObject {
	return governance.GovernedObject{
		Type: governance.TargetModelGrain,
		ModelGrain: &governance.ModelGrain{
			WorkspaceID:     fixture.WorkspaceID,
			AssetID:         fixture.AssetID,
			GrainExpression: "one row per order line",
			GrainFieldRefs:  []identity.PhysicalFieldID{fixture.LeftFieldID, fixture.SecondLeftFieldID},
			Content:         json.RawMessage(`{}`),
		},
	}
}

// Packet red scenario: a proposal targeting one of the four governed object
// types is rejected at submission when the target object does not exist in
// the workspace, and accepted when it does.
func TestProposalTargetingValidatesGovernedObjectExistenceAtSubmission(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	fixture := seedGovernedObjectFixture(t, pool, "targeting")
	objectService, proposalService := governedObjectServices(pool)

	missingID, err := identity.NewModelGrainID()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := proposalService.CreateGovernedProposal(ctx, governanceapp.CreateGovernedProposalRequest{
		WorkspaceID: fixture.WorkspaceID, TargetType: governance.TargetModelGrain,
		TargetObjectID: missingID.String(),
		Title:          "Tighten order grain", Summary: "Per order line", Reason: "Finance sign-off",
		CreatedBy: "steward",
	})
	if err != nil {
		t.Fatalf("draft with a nonexistent governed target must still open: %v", err)
	}
	if draft.TargetObjectType != governance.TargetModelGrain || draft.AssetID != nil || draft.BaseRevisionID != nil {
		t.Fatalf("governed-object draft = %+v", draft)
	}
	if _, err := proposalService.Submit(ctx, governanceapp.SubmitRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: draft.ID, Actor: "steward", TraceID: governanceTraceID,
	}); !errors.Is(err, governance.ErrNotFound) {
		t.Fatalf("submission with nonexistent governed target = %v, want ErrNotFound", err)
	}

	created, err := objectService.Create(ctx, governanceapp.CreateGovernedObjectRequest{
		WorkspaceID: fixture.WorkspaceID, Object: validFixtureGrain(fixture), CreatedBy: "steward",
	})
	if err != nil {
		t.Fatalf("create model grain: %v", err)
	}
	otherWorkspace := seedGovernedObjectFixture(t, pool, "targeting_other")
	crossDraft, err := proposalService.CreateGovernedProposal(ctx, governanceapp.CreateGovernedProposalRequest{
		WorkspaceID: otherWorkspace.WorkspaceID, TargetType: governance.TargetModelGrain,
		TargetObjectID: created.ModelGrain.ID.String(),
		Title:          "Cross workspace", CreatedBy: "steward",
	})
	if err != nil {
		t.Fatalf("open cross-workspace draft: %v", err)
	}
	if _, err := proposalService.Submit(ctx, governanceapp.SubmitRequest{
		WorkspaceID: otherWorkspace.WorkspaceID, ProposalID: crossDraft.ID, Actor: "steward", TraceID: governanceTraceID,
	}); !errors.Is(err, governance.ErrNotFound) {
		t.Fatalf("submission with cross-workspace governed target = %v, want ErrNotFound", err)
	}

	okDraft, err := proposalService.CreateGovernedProposal(ctx, governanceapp.CreateGovernedProposalRequest{
		WorkspaceID: fixture.WorkspaceID, TargetType: governance.TargetModelGrain,
		TargetObjectID: created.ModelGrain.ID.String(),
		Title:          "Tighten order grain", Summary: "Per order line", CreatedBy: "steward",
	})
	if err != nil {
		t.Fatalf("open governed proposal: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO proposal_changes (id, workspace_id, proposal_id, field_path, op, after_digest, after_value)
		VALUES ($1, $2, $3, 'grain', 'add', $4, '"per order line"'::jsonb)`,
		newRunID(t).UUID(), fixture.WorkspaceID.UUID(), okDraft.ID.UUID(), sha256FixtureDigest("grain")); err != nil {
		t.Fatal(err)
	}
	submitted, err := proposalService.Submit(ctx, governanceapp.SubmitRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: okDraft.ID, Actor: "steward", TraceID: governanceTraceID,
	})
	if err != nil {
		t.Fatalf("submission with existing governed target must be accepted: %v", err)
	}
	if submitted.State != governance.ProposalProposed {
		t.Fatalf("submitted governed proposal state = %s", submitted.State)
	}
}

// Packet green scenario: the four objects persist with workspace scoping,
// TypeID identity and version pinning; the partial unique index admits one
// active binding per (asset, physical target); identity columns are frozen
// and updates must bump the version (defense in depth behind the governed
// application path).
func TestGovernedObjectsPersistWithScopingTypeIDsAndVersionPinning(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	fixture := seedGovernedObjectFixture(t, pool, "persist")
	objectService, _ := governedObjectServices(pool)
	otherWorkspace := seedGovernedObjectFixture(t, pool, "persist_other")

	grain, err := objectService.Create(ctx, governanceapp.CreateGovernedObjectRequest{
		WorkspaceID: fixture.WorkspaceID, Object: validFixtureGrain(fixture), CreatedBy: "steward",
	})
	if err != nil {
		t.Fatalf("create model grain: %v", err)
	}
	evidenceID, err := identity.NewEvidenceID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO evidence_artifacts (id, workspace_id, evidence_type, locator, content_digest)
		VALUES ($1, $2, 'declared', 'docs/grain.md', $3)`,
		evidenceID.UUID(), fixture.WorkspaceID.UUID(), sha256FixtureDigest("grain-doc")); err != nil {
		t.Fatal(err)
	}
	key, err := objectService.Create(ctx, governanceapp.CreateGovernedObjectRequest{
		WorkspaceID: fixture.WorkspaceID,
		Object: governance.GovernedObject{
			Type: governance.TargetEntityKey,
			EntityKey: &governance.EntityKey{
				WorkspaceID: fixture.WorkspaceID, AssetID: fixture.AssetID,
				KeyFieldRefs:        []identity.PhysicalFieldID{fixture.LeftFieldID},
				UniquenessSemantics: governance.UniquenessDeduplicated,
				Content:             json.RawMessage(`{}`),
			},
		},
		CreatedBy: "steward",
	})
	if err != nil {
		t.Fatalf("create entity key: %v", err)
	}
	contract, err := objectService.Create(ctx, governanceapp.CreateGovernedObjectRequest{
		WorkspaceID: fixture.WorkspaceID,
		Object: governance.GovernedObject{
			Type: governance.TargetJoinContract,
			JoinContract: &governance.JoinContract{
				WorkspaceID:   fixture.WorkspaceID,
				LeftDatasetID: fixture.LeftDatasetID, RightDatasetID: fixture.RightDatasetID,
				LeftFieldRefs:  []identity.PhysicalFieldID{fixture.LeftFieldID, fixture.SecondLeftFieldID},
				RightFieldRefs: []identity.PhysicalFieldID{fixture.RightFieldID, fixture.RightFieldID},
				JoinType:       governance.JoinLeft, Cardinality: governance.CardinalityManyToOne,
				JoinExpression: "orders.customer_id = customers.customer_id",
				Content:        json.RawMessage(`{}`),
			},
		},
		CreatedBy: "steward",
	})
	if err != nil {
		t.Fatalf("create join contract: %v", err)
	}
	notes := "mirrors warehouse FK"
	contract.JoinContract.ContractNotes = &notes
	binding, err := objectService.Create(ctx, governanceapp.CreateGovernedObjectRequest{
		WorkspaceID: fixture.WorkspaceID,
		Object: governance.GovernedObject{
			Type: governance.TargetPhysicalBinding,
			PhysicalBinding: &governance.PhysicalBinding{
				WorkspaceID: fixture.WorkspaceID, AssetID: fixture.AssetID,
				DatasetID: fixture.LeftDatasetID, FieldID: &fixture.LeftFieldID,
				Transform: func() *string { value := "cast(order_id as text)"; return &value }(),
				Content:   json.RawMessage(`{}`),
			},
		},
		CreatedBy: "steward",
	})
	if err != nil {
		t.Fatalf("create physical binding: %v", err)
	}

	// TypeID identity per ADR-0003.
	if binding.PhysicalBinding.ID.Prefix() != identity.PhysicalBinding {
		t.Fatalf("binding prefix = %s, want phb", binding.PhysicalBinding.ID.Prefix())
	}
	if contract.JoinContract.ID.Prefix() != identity.JoinContract {
		t.Fatalf("contract prefix = %s, want jct", contract.JoinContract.ID.Prefix())
	}
	// Version pinning starts at 1.
	for name, version := range map[string]int{
		"binding": binding.PhysicalBinding.Version, "grain": grain.ModelGrain.Version,
		"key": key.EntityKey.Version, "contract": contract.JoinContract.Version,
	} {
		if version != 1 {
			t.Fatalf("%s initial version = %d, want 1", name, version)
		}
	}

	// Workspace scoping: reads are scoped and listings never leak.
	if _, err := objectService.Get(ctx, otherWorkspace.WorkspaceID, governance.TargetPhysicalBinding,
		binding.PhysicalBinding.ID.String()); !errors.Is(err, governance.ErrNotFound) {
		t.Fatalf("cross-workspace get = %v, want ErrNotFound", err)
	}
	listings := map[governance.TargetObjectType]int{
		governance.TargetPhysicalBinding: 1, governance.TargetModelGrain: 1,
		governance.TargetEntityKey: 1, governance.TargetJoinContract: 1,
	}
	for objectType, want := range listings {
		objects, listErr := objectService.List(ctx, fixture.WorkspaceID, objectType)
		if listErr != nil {
			t.Fatalf("list %s: %v", objectType, listErr)
		}
		if len(objects) != want {
			t.Fatalf("list %s = %d objects, want %d", objectType, len(objects), want)
		}
		otherObjects, listErr := objectService.List(ctx, otherWorkspace.WorkspaceID, objectType)
		if listErr != nil {
			t.Fatalf("list other-workspace %s: %v", objectType, listErr)
		}
		if len(otherObjects) != 0 {
			t.Fatalf("other-workspace %s listing leaked %d objects", objectType, len(otherObjects))
		}
	}

	// One ACTIVE binding per (asset, physical target): a second active
	// field-level binding for the same pair is a conflict.
	if _, err := objectService.Create(ctx, governanceapp.CreateGovernedObjectRequest{
		WorkspaceID: fixture.WorkspaceID,
		Object: governance.GovernedObject{
			Type: governance.TargetPhysicalBinding,
			PhysicalBinding: &governance.PhysicalBinding{
				WorkspaceID: fixture.WorkspaceID, AssetID: fixture.AssetID,
				DatasetID: fixture.LeftDatasetID, FieldID: &fixture.LeftFieldID,
				Content: json.RawMessage(`{}`),
			},
		},
		CreatedBy: "steward",
	}); !errors.Is(err, governance.ErrConflict) {
		t.Fatalf("duplicate active binding = %v, want ErrConflict", err)
	}
	// A retired binding does not occupy the active slot.
	if _, err := pool.Exec(ctx, `
		INSERT INTO physical_bindings (
			id, workspace_id, asset_id, dataset_id, field_id, retired_at, version, content, created_by
		) VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, 1, '{}'::jsonb, 'steward')`,
		newRunID(t).UUID(), fixture.WorkspaceID.UUID(), fixture.AssetID.UUID(),
		fixture.LeftDatasetID.UUID(), fixture.LeftFieldID.UUID()); err != nil {
		t.Fatal(err)
	}
	rebind, err := objectService.Create(ctx, governanceapp.CreateGovernedObjectRequest{
		WorkspaceID: fixture.WorkspaceID,
		Object: governance.GovernedObject{
			Type: governance.TargetPhysicalBinding,
			PhysicalBinding: &governance.PhysicalBinding{
				WorkspaceID: fixture.WorkspaceID, AssetID: fixture.AssetID,
				DatasetID: fixture.LeftDatasetID, FieldID: &fixture.SecondLeftFieldID,
				Content: json.RawMessage(`{}`),
			},
		},
		CreatedBy: "steward",
	})
	if err != nil {
		t.Fatalf("binding a distinct physical target: %v", err)
	}
	if rebind.PhysicalBinding.FieldID == nil || *rebind.PhysicalBinding.FieldID != fixture.SecondLeftFieldID {
		t.Fatalf("rebind = %+v", rebind.PhysicalBinding)
	}

	// Defense in depth: raw-SQL updates must bump the version exactly once
	// and never touch identity columns (the governed application path is the
	// only writer in production code).
	if _, err := pool.Exec(ctx, `
		UPDATE model_grains SET grain_expression = 'hacked' WHERE id = $1`,
		grain.ModelGrain.ID.UUID()); err == nil {
		t.Fatal("database accepted a governed object update without a version bump")
	}
}

// Packet red scenario: applying a governed change bumps the object version,
// updates the content, and records exactly one audit fact — with NO outbox
// event, because release publication remains the outbox signal in this task.
func TestApplyGovernedChangeBumpsVersionAndRecordsAuditFactWithoutOutbox(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	fixture := seedGovernedObjectFixture(t, pool, "apply")
	objectService, _ := governedObjectServices(pool)

	grain, err := objectService.Create(ctx, governanceapp.CreateGovernedObjectRequest{
		WorkspaceID: fixture.WorkspaceID, Object: validFixtureGrain(fixture), CreatedBy: "steward",
	})
	if err != nil {
		t.Fatalf("create model grain: %v", err)
	}
	applied, err := objectService.ApplyGovernedChange(ctx, governanceapp.ApplyGovernedChangeRequest{
		WorkspaceID: fixture.WorkspaceID, ObjectType: governance.TargetModelGrain,
		ObjectID: grain.ModelGrain.ID.String(), ExpectedVersion: 1,
		Content: json.RawMessage(`{"grain":"one row per order line","verified":true}`),
		Summary: "Tighten grain to order line", Actor: "steward", TraceID: governanceTraceID,
	})
	if err != nil {
		t.Fatalf("apply governed change: %v", err)
	}
	if applied.ModelGrain == nil || applied.ModelGrain.Version != 2 {
		t.Fatalf("applied object = %+v, want version 2", applied.ModelGrain)
	}

	var storedVersion int
	var storedContent []byte
	if err := pool.QueryRow(ctx, `
		SELECT version, content FROM model_grains WHERE id = $1`,
		grain.ModelGrain.ID.UUID()).Scan(&storedVersion, &storedContent); err != nil {
		t.Fatal(err)
	}
	if storedVersion != 2 || string(storedContent) != `{"grain": "one row per order line", "verified": true}` {
		t.Fatalf("stored governed object = version %d content %s", storedVersion, storedContent)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'governance.model_grain.applied'`,
		fixture.WorkspaceID.UUID()).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("governed object audit rows = %d, want exactly one", auditCount)
	}
	var payload []byte
	if err := pool.QueryRow(ctx, `
		SELECT payload FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'governance.model_grain.applied'`,
		fixture.WorkspaceID.UUID()).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var auditData map[string]any
	if err := json.Unmarshal(payload, &auditData); err != nil {
		t.Fatal(err)
	}
	if auditData["action"] != "applied" || auditData["specVersion"] != "semlia.governance-object/v1" ||
		auditData["summary"] != "Tighten grain to order line" {
		t.Fatalf("audit payload = %v", auditData)
	}
	var outboxRows int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM outbox_events
		WHERE workspace_id = $1 AND event_type IN (
			'governed_object.applied', 'governance.object.applied',
			'physical_binding.changed', 'model_grain.changed',
			'entity_key.changed', 'join_contract.changed')`,
		fixture.WorkspaceID.UUID()).Scan(&outboxRows); err != nil {
		t.Fatal(err)
	}
	if outboxRows != 0 {
		t.Fatalf("governed object application must stay audit-only, found %d outbox rows", outboxRows)
	}

	// Stale version expectations are rejected even without concurrency.
	if _, err := objectService.ApplyGovernedChange(ctx, governanceapp.ApplyGovernedChangeRequest{
		WorkspaceID: fixture.WorkspaceID, ObjectType: governance.TargetModelGrain,
		ObjectID: grain.ModelGrain.ID.String(), ExpectedVersion: 1,
		Content: json.RawMessage(`{}`), Summary: "stale", Actor: "steward", TraceID: governanceTraceID,
	}); !errors.Is(err, governance.ErrInvariant) {
		t.Fatalf("stale-version apply = %v, want ErrInvariant", err)
	}
	// Unknown objects are not found.
	missingID, err := identity.NewEntityKeyID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := objectService.ApplyGovernedChange(ctx, governanceapp.ApplyGovernedChangeRequest{
		WorkspaceID: fixture.WorkspaceID, ObjectType: governance.TargetEntityKey,
		ObjectID: missingID.String(), ExpectedVersion: 1,
		Content: json.RawMessage(`{}`), Summary: "missing", Actor: "steward", TraceID: governanceTraceID,
	}); !errors.Is(err, governance.ErrNotFound) {
		t.Fatalf("apply on missing object = %v, want ErrNotFound", err)
	}
}

// Packet red scenario: two concurrent governed change applications on the
// same object — exactly one wins, the other is rejected with an invariant
// error through the FOR UPDATE row lock plus the version expectation.
func TestApplyGovernedChangeRejectsConcurrentApplication(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	fixture := seedGovernedObjectFixture(t, pool, "concurrent_apply")
	objectService, _ := governedObjectServices(pool)

	contract, err := objectService.Create(ctx, governanceapp.CreateGovernedObjectRequest{
		WorkspaceID: fixture.WorkspaceID,
		Object: governance.GovernedObject{
			Type: governance.TargetJoinContract,
			JoinContract: &governance.JoinContract{
				WorkspaceID:   fixture.WorkspaceID,
				LeftDatasetID: fixture.LeftDatasetID, RightDatasetID: fixture.RightDatasetID,
				LeftFieldRefs:  []identity.PhysicalFieldID{fixture.LeftFieldID},
				RightFieldRefs: []identity.PhysicalFieldID{fixture.RightFieldID},
				JoinType:       governance.JoinInner, Cardinality: governance.CardinalityOneToOne,
				JoinExpression: "orders.customer_id = customers.customer_id",
				Content:        json.RawMessage(`{}`),
			},
		},
		CreatedBy: "steward",
	})
	if err != nil {
		t.Fatalf("create join contract: %v", err)
	}

	concurrentPool := openConcurrentPool(t, 2)
	t.Cleanup(concurrentPool.Close)
	winnerService := governanceapp.NewGovernedObjectService(pgstore.NewStore(concurrentPool), governanceClock)
	loserService := governanceapp.NewGovernedObjectService(pgstore.NewStore(concurrentPool), governanceClock)

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]error, 2)
	for applicant := 0; applicant < 2; applicant++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			service := winnerService
			if index == 1 {
				service = loserService
			}
			_, err := service.ApplyGovernedChange(context.Background(), governanceapp.ApplyGovernedChangeRequest{
				WorkspaceID: fixture.WorkspaceID, ObjectType: governance.TargetJoinContract,
				ObjectID: contract.JoinContract.ID.String(), ExpectedVersion: 1,
				Content: json.RawMessage(`{"note":"applied"}`),
				Summary: "apply", Actor: "steward", TraceID: governanceTraceID,
			})
			results[index] = err
		}(applicant)
	}
	close(start)
	wg.Wait()

	successes := 0
	for index, err := range results {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, governance.ErrInvariant) {
			t.Fatalf("applicant %d failed with %v, want ErrInvariant", index, err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent applications: %d succeeded, want exactly one winner (results=%v)", successes, results)
	}
	var version int
	if err := pool.QueryRow(ctx, `
		SELECT version FROM join_contracts WHERE id = $1`,
		contract.JoinContract.ID.UUID()).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("join contract version after concurrent applies = %d, want 2", version)
	}
}

// Packet red scenario: a populated M1+T001+T002 registry survives the
// governance-objects upgrade, the downgrade back to the T002 level, and the
// re-upgrade — with all M0/M1/T001/T002 rows preserved untouched.
func TestPopulatedM2UpgradeAndRollbackPreserveGovernedAuthoringRows(t *testing.T) {
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := migrator.Up(); err != nil {
			t.Errorf("restore latest schema: %v", err)
		}
	})
	if err := migrator.Steps(6); err != nil {
		t.Fatalf("install M1+M2 schema through governed authoring: %v", err)
	}
	assertVersion(t, migrator, 6, true)

	pool := openPool(t)
	ctx := context.Background()
	fixture := seedGovernanceAsset(t, pool, "lifecycle")
	proposalService, _, _, _, _ := governanceServices(pool)
	proposalID, changeID := submitTestProposal(t, ctx, proposalService, fixture)

	if err := migrator.Steps(1); err != nil {
		t.Fatalf("upgrade populated T002 with governance objects: %v", err)
	}
	assertVersion(t, migrator, 7, true)
	var proposalCount, changeCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM proposals WHERE id = $1`, proposalID.UUID()).Scan(&proposalCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM proposal_changes WHERE id = $1`, changeID.UUID()).Scan(&changeCount); err != nil {
		t.Fatal(err)
	}
	if proposalCount != 1 || changeCount != 1 {
		t.Fatalf("T002 rows after governance-objects upgrade = proposal %d change %d", proposalCount, changeCount)
	}
	sourceID, err := identity.NewSourceConnectionID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO source_connections (id, workspace_id, adapter_kind, name, normalized_locator, status, metadata)
		VALUES ($1, $2, 'postgres', 'Lifecycle Warehouse', 'postgres://lifecycle/catalog', 'active', '{}'::jsonb)`,
		sourceID.UUID(), fixture.WorkspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	datasetID, err := identity.NewPhysicalDatasetID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO physical_datasets (id, workspace_id, source_connection_id, external_key, qualified_name)
		VALUES ($1, $2, $3, 'public.orders', 'lifecycle.public.orders')`,
		datasetID.UUID(), fixture.WorkspaceID.UUID(), sourceID.UUID()); err != nil {
		t.Fatal(err)
	}
	bindingID, err := identity.NewPhysicalBindingID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO physical_bindings (id, workspace_id, asset_id, dataset_id, version, content, created_by)
		VALUES ($1, $2, $3, $4, 1, '{}'::jsonb, 'steward')`,
		bindingID.UUID(), fixture.WorkspaceID.UUID(), fixture.AssetID.UUID(), datasetID.UUID()); err != nil {
		t.Fatal(err)
	}

	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove governance objects schema: %v", err)
	}
	assertVersion(t, migrator, 6, true)
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM proposals WHERE id = $1`, proposalID.UUID()).Scan(&proposalCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM proposal_changes WHERE id = $1`, changeID.UUID()).Scan(&changeCount); err != nil {
		t.Fatal(err)
	}
	if proposalCount != 1 || changeCount != 1 {
		t.Fatalf("T002 rows after governance-objects downgrade = proposal %d change %d", proposalCount, changeCount)
	}
	var objectTables int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name IN (
			'physical_bindings', 'model_grains', 'entity_keys', 'join_contracts')`).Scan(&objectTables); err != nil {
		t.Fatal(err)
	}
	if objectTables != 0 {
		t.Fatalf("governance object tables survived the downgrade: %d", objectTables)
	}

	if err := migrator.Steps(1); err != nil {
		t.Fatalf("re-upgrade governance objects: %v", err)
	}
	assertVersion(t, migrator, 7, true)
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM proposals WHERE id = $1`, proposalID.UUID()).Scan(&proposalCount); err != nil {
		t.Fatal(err)
	}
	if proposalCount != 1 {
		t.Fatalf("T002 proposal rows after re-upgrade = %d", proposalCount)
	}
}
