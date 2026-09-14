package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	governance "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func productionPhysicalFixture(t *testing.T, f *fixture, w identity.WorkspaceID, input map[string]any) (map[string]any, map[string]any) {
	t.Helper()
	selection := input["snapshots"].([]any)[0].(map[string]any)
	snapshotID := selection["snapshotId"].(string)
	id, err := identity.ParseSourceSnapshotID(snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]map[string]any{}
	for _, kind := range []string{"dataset", "field"} {
		var objectUUID, revisionUUID string
		if err := f.pool.QueryRow(context.Background(), `SELECT object_id::text,revision_id::text FROM source_snapshot_members WHERE workspace_id=$1 AND snapshot_id=$2 AND kind=$3 ORDER BY object_id LIMIT 1`, w.UUID(), id.UUID(), kind).Scan(&objectUUID, &revisionUUID); err != nil {
			t.Fatal(err)
		}
		objectPrefix, revisionPrefix := identity.PhysicalDataset, identity.PhysicalDatasetRevision
		if kind == "field" {
			objectPrefix, revisionPrefix = identity.PhysicalField, identity.PhysicalFieldRevision
		}
		object, err := identity.FromUUID(objectPrefix, objectUUID)
		if err != nil {
			t.Fatal(err)
		}
		revision, err := identity.FromUUID(revisionPrefix, revisionUUID)
		if err != nil {
			t.Fatal(err)
		}
		refs[kind] = map[string]any{"snapshotId": snapshotID, "kind": kind, "objectId": object.String(), "revisionId": revision.String()}
	}
	return refs["dataset"], refs["field"]
}

func TestProductionAuthoringAllFivePublishedUpdates(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
	dataset, field := productionPhysicalFixture(t, f, w, input)
	asset, revision, assetContent := productionPublishedAsset(t, f, w, p)
	datasetID, _ := identity.ParsePhysicalDatasetID(dataset["objectId"].(string))
	fieldID, _ := identity.ParsePhysicalFieldID(field["objectId"].(string))
	var firstReleaseUUID string
	if err := f.pool.QueryRow(context.Background(), `SELECT id::text FROM releases WHERE workspace_id=$1 ORDER BY sequence DESC LIMIT 1`, w.UUID()).Scan(&firstReleaseUUID); err != nil {
		t.Fatal(err)
	}
	firstRelease, _ := identity.FromUUID(identity.Release, firstReleaseUUID)
	assetRef := map[string]any{"kind": "semantic_asset", "targetId": asset.String(), "releaseId": firstRelease.String(), "revisionId": revision.String()}
	contents := map[string]map[string]any{
		"physical_binding": {"asset": assetRef, "dataset": dataset, "field": field, "transform": "id"},
		"model_grain":      {"asset": assetRef, "expression": "one row per order", "fields": []any{field}},
		"entity_key":       {"asset": assetRef, "fields": []any{field}, "uniqueness": "exact"},
		"join_contract":    {"leftDataset": dataset, "rightDataset": dataset, "pairs": []any{map[string]any{"left": field, "right": field}}, "joinType": "inner", "cardinality": "one_to_one", "expression": "left.id = right.id"},
	}
	service := governanceapp.NewGovernedObjectService(f.store, governanceapp.ClockFunc(time.Now))
	reviewer := createReviewerPrincipal(t, f, w, "object-baseline-reviewer")
	publisher := createPrincipalWithRoles(t, f, w, "object-baseline-publisher", []string{"publisher"})
	targets := []any{}
	var assetWire map[string]any
	if err := json.Unmarshal(assetContent, &assetWire); err != nil {
		t.Fatal(err)
	}
	targets = append(targets, map[string]any{"localKey": "asset", "kind": "semantic_asset", "intent": "update", "targetId": asset.String(), "baseRevisionId": revision.String(), "title": "Asset update", "content": assetWire, "changes": []any{}, "evidenceIds": []any{}})
	for _, kind := range []string{"physical_binding", "model_grain", "entity_key", "join_contract"} {
		changePath := "expression"
		if kind == "physical_binding" {
			changePath = "transform"
		}
		if kind == "entity_key" {
			changePath = "uniqueness"
		}
		initialContent := map[string]any{}
		for key, value := range contents[kind] {
			if key != changePath {
				initialContent[key] = value
			}
		}
		content, _ := json.Marshal(initialContent)
		object := governance.GovernedObject{Type: governance.TargetObjectType(kind)}
		switch kind {
		case "physical_binding":
			transform := "id"
			object.PhysicalBinding = &governance.PhysicalBinding{WorkspaceID: w, AssetID: asset, DatasetID: datasetID, FieldID: &fieldID, Transform: &transform, Content: content}
			changePath = "transform"
		case "model_grain":
			object.ModelGrain = &governance.ModelGrain{WorkspaceID: w, AssetID: asset, GrainExpression: "one row per order", GrainFieldRefs: []identity.PhysicalFieldID{fieldID}, Content: content}
		case "entity_key":
			object.EntityKey = &governance.EntityKey{WorkspaceID: w, AssetID: asset, KeyFieldRefs: []identity.PhysicalFieldID{fieldID}, UniquenessSemantics: governance.UniquenessSemantics("exact"), Content: content}
			changePath = "uniqueness"
		case "join_contract":
			object.JoinContract = &governance.JoinContract{WorkspaceID: w, LeftDatasetID: datasetID, RightDatasetID: datasetID, LeftFieldRefs: []identity.PhysicalFieldID{fieldID}, RightFieldRefs: []identity.PhysicalFieldID{fieldID}, JoinType: governance.JoinType("inner"), Cardinality: governance.JoinCardinality("one_to_one"), JoinExpression: "left.id = right.id", Content: content}
		}
		created, err := service.Create(context.Background(), governanceapp.CreateGovernedObjectRequest{WorkspaceID: w, Object: object, CreatedBy: p.String()})
		if err != nil {
			t.Fatalf("create baseline %s: %v", kind, err)
		}
		var id string
		switch kind {
		case "physical_binding":
			id = created.PhysicalBinding.ID.String()
		case "model_grain":
			id = created.ModelGrain.ID.String()
		case "entity_key":
			id = created.EntityKey.ID.String()
		case "join_contract":
			id = created.JoinContract.ID.String()
		}
		value := contents[kind][changePath]
		valueJSON, _ := json.Marshal(value)
		digest := sha256Of(string(valueJSON))
		proposalBody, _ := json.Marshal(map[string]any{"targetObjectType": kind, "targetObjectId": id, "title": "Publish canonical object baseline", "reason": "Fixture baseline", "createdBy": p.String(), "changeSet": []any{map[string]any{"fieldPath": changePath, "op": "add", "afterValue": value, "afterDigest": digest}}})
		proposalPath := f.proposalsPath(t, w)
		r := f.request(t, http.MethodPost, proposalPath, p.String(), string(proposalBody))
		if r.Code != http.StatusCreated {
			t.Fatalf("baseline proposal %s: %d %s", kind, r.Code, r.Body.String())
		}
		proposalID := decodeProposalDetail(t, r.Body.Bytes())["id"].(string)
		r = f.request(t, http.MethodPost, proposalPath+"/"+proposalID+"/submit", p.String(), "")
		if r.Code != http.StatusOK {
			t.Fatalf("baseline submit %s: %d %s", kind, r.Code, r.Body.String())
		}
		f.runValidationWorker(t)
		assertProposalState(t, f, w, proposalPath, proposalID, "in_review")
		approveProposal(t, f, w, reviewer, proposalID)
		r = publishProposal(t, f, w, publisher, proposalID)
		if r.Code != http.StatusCreated {
			t.Fatalf("baseline publish %s: %d %s", kind, r.Code, r.Body.String())
		}
		targets = append(targets, map[string]any{"localKey": kind, "kind": kind, "intent": "update", "targetId": id, "baseObjectVersion": 2, "title": "Update " + kind, "content": contents[kind], "changes": []any{}, "evidenceIds": []any{}})
	}
	var releaseUUID string
	if err := f.pool.QueryRow(context.Background(), `SELECT id::text FROM releases WHERE workspace_id=$1 ORDER BY sequence DESC LIMIT 1`, w.UUID()).Scan(&releaseUUID); err != nil {
		t.Fatal(err)
	}
	release, _ := identity.FromUUID(identity.Release, releaseUUID)
	assetRef["releaseId"] = release.String()
	input["dependencies"] = []any{assetRef}
	payload := map[string]any{"input": input, "targets": targets}
	raw, _ := json.Marshal(payload)
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	r := sendProdRequest(h, http.MethodPost, path, p.String(), "five-no-change", string(raw))
	if r.Code != http.StatusOK {
		t.Fatalf("five no-change: %d %s", r.Code, r.Body.String())
	}
	result := authoringLifecycleDecode(t, r.Body.Bytes())
	if len(result["proposalIds"].([]any)) != 0 {
		t.Fatal("no-change created proposals")
	}
	for _, targetValue := range targets {
		target := targetValue.(map[string]any)
		content := target["content"].(map[string]any)
		fieldName := "expression"
		switch target["kind"] {
		case "semantic_asset":
			fieldName = "definition"
		case "physical_binding":
			fieldName = "transform"
		case "entity_key":
			fieldName = "uniqueness"
		}
		before := content[fieldName]
		after := fmt.Sprint(before) + " revised"
		if fieldName == "uniqueness" {
			after = "deduplicated"
		}
		content[fieldName] = after
		target["changes"] = []any{map[string]any{"fieldPath": fieldName, "op": "update", "beforeValue": before, "afterValue": after}}
	}
	payload["expectedVersion"] = 1
	raw, _ = json.Marshal(payload)
	r = sendProdRequest(h, http.MethodPut, path+"/"+result["operationId"].(string), p.String(), "five-update", string(raw))
	if r.Code != http.StatusOK {
		t.Fatalf("five real changes: %d %s", r.Code, r.Body.String())
	}
	if len(authoringLifecycleDecode(t, r.Body.Bytes())["proposalIds"].([]any)) != 5 {
		t.Fatal("missing changed proposals")
	}
}

func TestProductionAuthoringPhysicalParentMismatchCommitsNothing(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
	other := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
	dataset, _ := productionPhysicalFixture(t, f, w, input)
	_, field := productionPhysicalFixture(t, f, w, other)
	input["snapshots"] = append(input["snapshots"].([]any), other["snapshots"].([]any)...)
	inputJSON, _ := json.Marshal(input)
	body := completeProductionPayload(t, authoringLifecycleBody(string(inputJSON)), p)
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	payload["targets"] = append(payload["targets"].([]any), map[string]any{"localKey": "binding", "intent": "create", "kind": "physical_binding", "identityKey": "fixture.bad-parent", "title": "Binding", "content": map[string]any{"asset": map[string]any{"localKey": "revenue"}, "dataset": dataset, "field": field}, "changes": []any{}, "evidenceIds": []any{}})
	raw, _ := json.Marshal(payload)
	r := sendProdRequest(h, http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w), p.String(), "bad-parent-proof", string(raw))
	if r.Code != http.StatusUnprocessableEntity || !strings.Contains(r.Body.String(), "DEPENDENCY_INVALID") {
		t.Fatalf("unrelated field parent accepted: %d %s", r.Code, r.Body.String())
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM production_operations WHERE workspace_id=$1)+(SELECT count(*) FROM semantic_assets WHERE workspace_id=$1)+(SELECT count(*) FROM proposals WHERE workspace_id=$1)+(SELECT count(*) FROM production_commands WHERE workspace_id=$1)+(SELECT count(*) FROM production_identity_reservations WHERE workspace_id=$1)`, w.UUID()).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid parent committed business rows: %d %v", count, err)
	}
}

func TestProductionAuthoringAllFiveCreateTargetsResolveReferences(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
	dataset, field := productionPhysicalFixture(t, f, w, input)
	inputJSON, _ := json.Marshal(input)
	body := completeProductionPayload(t, authoringLifecycleBody(string(inputJSON)), p)
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	contents := map[string]map[string]any{
		"physical_binding": {"asset": map[string]any{"localKey": "revenue"}, "dataset": dataset, "field": field},
		"model_grain":      {"asset": map[string]any{"localKey": "revenue"}, "expression": "one row per order", "fields": []any{field}},
		"entity_key":       {"asset": map[string]any{"localKey": "revenue"}, "fields": []any{field}, "uniqueness": "exact"},
		"join_contract":    {"leftDataset": dataset, "rightDataset": dataset, "pairs": []any{map[string]any{"left": field, "right": field}}, "joinType": "inner", "cardinality": "one_to_one", "expression": "left.id = right.id"},
	}
	for _, kind := range []string{"physical_binding", "model_grain", "entity_key", "join_contract"} {
		payload["targets"] = append(payload["targets"].([]any), map[string]any{"localKey": kind, "intent": "create", "kind": kind, "identityKey": "fixture." + kind, "title": kind, "content": contents[kind], "changes": []any{}, "evidenceIds": []any{}})
	}
	raw, _ := json.Marshal(payload)
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	r := sendProdRequest(h, http.MethodPost, path, p.String(), "five-create", string(raw))
	if r.Code != http.StatusCreated {
		t.Fatalf("five creates: %d %s", r.Code, r.Body.String())
	}
	created := authoringLifecycleDecode(t, r.Body.Bytes())
	if len(created["proposalIds"].([]any)) != 5 {
		t.Fatal("missing real draft proposals")
	}
	var unresolved, objects int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM production_targets WHERE workspace_id=$1 AND (content_json::text LIKE '%localKey%' OR content_json::text LIKE '%revisionId%' OR content_json::text LIKE '%snapshotId%')`, w.UUID()).Scan(&unresolved); err != nil || unresolved != 0 {
		t.Fatalf("references not resolved: %d %v", unresolved, err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM physical_bindings WHERE workspace_id=$1)+(SELECT count(*) FROM model_grains WHERE workspace_id=$1)+(SELECT count(*) FROM entity_keys WHERE workspace_id=$1)+(SELECT count(*) FROM join_contracts WHERE workspace_id=$1)`, w.UUID()).Scan(&objects); err != nil || objects != 0 {
		t.Fatalf("create produced premature registry rows: %d %v", objects, err)
	}
	get := sendProdRequest(h, http.MethodGet, path+"/"+created["operationId"].(string), p.String(), "", "")
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"localKey":"revenue"`) || !strings.Contains(get.Body.String(), `"snapshotId":`) {
		t.Fatalf("recovery lost original declarations: %d %s", get.Code, get.Body.String())
	}
}
