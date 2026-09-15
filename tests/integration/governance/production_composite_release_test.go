package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/iiwish/semlia/internal/application/jobs"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5/pgtype"
)

func productionFiveCreatePayload(t *testing.T, f *fixture, w identity.WorkspaceID, p identity.PrincipalID) map[string]any {
	t.Helper()
	input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{}, true)
	dataset, field := productionPhysicalFixture(t, f, w, input)
	inputJSON, _ := json.Marshal(input)
	var payload map[string]any
	if err := json.Unmarshal([]byte(completeProductionPayload(t, authoringLifecycleBody(string(inputJSON)), p)), &payload); err != nil {
		t.Fatal(err)
	}
	contents := map[string]map[string]any{
		"physical_binding": {"asset": map[string]any{"localKey": "revenue"}, "dataset": dataset, "field": field, "transform": "id"},
		"model_grain":      {"asset": map[string]any{"localKey": "revenue"}, "expression": "one row per order", "fields": []any{field}},
		"entity_key":       {"asset": map[string]any{"localKey": "revenue"}, "fields": []any{field}, "uniqueness": "exact"},
		"join_contract":    {"leftDataset": dataset, "rightDataset": dataset, "pairs": []any{map[string]any{"left": field, "right": field}}, "joinType": "inner", "cardinality": "one_to_one", "expression": "left.id = right.id", "notes": ""},
	}
	for _, kind := range []string{"physical_binding", "model_grain", "entity_key", "join_contract"} {
		payload["targets"] = append(payload["targets"].([]any), map[string]any{"localKey": kind, "intent": "create", "kind": kind, "identityKey": "fixture." + kind, "title": kind, "content": contents[kind], "changes": []any{}, "evidenceIds": []any{}})
	}
	return payload
}

func productionRequestResult(t *testing.T, h http.Handler, method, path string, p identity.PrincipalID, key string, body any, status int) map[string]any {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := sendProdRequest(h, method, path, p.String(), key, string(raw))
	if r.Code != status {
		t.Fatalf("%s %s: %d %s", method, path, r.Code, r.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func productionValidateAndApprove(t *testing.T, f *fixture, h http.Handler, w identity.WorkspaceID, author, reviewer identity.PrincipalID, created map[string]any) string {
	t.Helper()
	op := created["operationId"].(string)
	version := int(created["version"].(float64))
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s", w, op)
	confirmProductionFixtureRules(t, f, h, w, author, created)
	productionRequestResult(t, h, http.MethodPost, path+"/submit", author, "submit-"+op, map[string]any{"expectedVersion": version, "setDigest": created["setDigest"]}, http.StatusAccepted)
	job := jobs.Job{WorkspaceID: w, Attempt: 1, MaxAttempts: 3}
	if err := f.pool.QueryRow(context.Background(), `SELECT payload,trace_id FROM jobs WHERE workspace_id=$1 AND job_type='governance.proposal.validate' AND payload->>'operationId'=$2`, w.UUID(), op).Scan(&job.Payload, &job.TraceID); err != nil {
		t.Fatal(err)
	}
	if err := runProductionValidationJob(t, f, job); err != nil {
		t.Fatal(err)
	}
	var digest, status string
	if err := f.pool.QueryRow(context.Background(), `SELECT validation_digest,status FROM production_validation_attempts WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3 AND attempt_no=1`, w.UUID(), mustParseProductionOperationID(t, op).UUID(), version).Scan(&digest, &status); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" {
		var results []byte
		_ = f.pool.QueryRow(context.Background(), `SELECT results_json FROM production_validation_seals WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3 AND attempt_no=1`, w.UUID(), mustParseProductionOperationID(t, op).UUID(), version).Scan(&results)
		t.Fatalf("validation failed: %s", results)
	}
	productionRequestResult(t, h, http.MethodPost, path+"/reviews", reviewer, "review-"+op, map[string]any{"expectedVersion": version, "setDigest": created["setDigest"], "validation": map[string]any{"attemptNo": 1, "validationDigest": digest}, "proposalIds": created["proposalIds"], "decision": "approve", "note": "reviewed complete set"}, http.StatusCreated)
	return digest
}

func TestProductionCompositeFiveKindsPublishAndRepeatedRollback(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	reviewer := authoringLifecyclePrincipal(t, f, w, "Composite reviewer")
	publisher := authoringLifecyclePrincipal(t, f, w, "Composite publisher")
	payload := productionFiveCreatePayload(t, f, w, p)
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	created := productionRequestResult(t, h, http.MethodPost, path, p, "composite-create", payload, http.StatusCreated)
	digest := productionValidateAndApprove(t, f, h, w, p, reviewer, created)
	op := created["operationId"].(string)
	productionAssertLatePublishFailureAtomic(t, f, h, w, publisher, path+"/"+op+"/publish", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"], "validation": map[string]any{"attemptNo": 1, "validationDigest": digest}, "expectedHead": map[string]any{"presence": "absent"}})
	published := productionRequestResult(t, h, http.MethodPost, path+"/"+op+"/publish", publisher, "composite-publish", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"], "validation": map[string]any{"attemptNo": 1, "validationDigest": digest}, "expectedHead": map[string]any{"presence": "absent"}}, http.StatusCreated)
	var objects, attributions, inputs int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM release_objects WHERE workspace_id=$1),(SELECT count(*) FROM release_proposals WHERE workspace_id=$1),(SELECT count(*) FROM production_release_binding_inputs WHERE workspace_id=$1)`, w.UUID()).Scan(&objects, &attributions, &inputs); err != nil {
		t.Fatal(err)
	}
	if objects != 4 || attributions != 5 || inputs != 1 {
		t.Fatalf("incomplete composite release: objects=%d attribution=%d inputs=%d", objects, attributions, inputs)
	}
	var proposalEvents, objectEvents int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM audit_events WHERE workspace_id=$1 AND event_type='governance.proposal.released'),(SELECT count(*) FROM audit_events WHERE workspace_id=$1 AND event_type IN ('governance.physical_binding.published','governance.model_grain.published','governance.entity_key.published','governance.join_contract.published'))`, w.UUID()).Scan(&proposalEvents, &objectEvents); err != nil {
		t.Fatal(err)
	}
	if proposalEvents != 5 || objectEvents != 4 {
		t.Fatalf("missing atomic mutation events: proposals=%d objects=%d", proposalEvents, objectEvents)
	}
	productionAssertCompositeProjection(t, f, w, published, 0)
	for _, statement := range []string{
		`UPDATE production_release_manifests SET before_manifest_json='{"assets":[],"objects":[]}' WHERE workspace_id=$1`,
		`DELETE FROM production_release_before_pins WHERE workspace_id=$1`,
		`UPDATE release_proposals SET review_ids='[]' WHERE workspace_id=$1`,
		`DELETE FROM production_release_binding_inputs WHERE workspace_id=$1`,
		`UPDATE production_validation_bindings SET input_digest='sha256:0000000000000000000000000000000000000000000000000000000000000000' WHERE workspace_id=$1`,
		`DELETE FROM production_review_bindings WHERE workspace_id=$1`,
	} {
		tx, err := f.pool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := tx.Exec(context.Background(), statement, w.UUID())
		if writeErr == nil {
			_, writeErr = tx.Exec(context.Background(), `SET CONSTRAINTS ALL IMMEDIATE`)
		}
		_ = tx.Rollback(context.Background())
		if writeErr == nil {
			t.Fatalf("protected history accepted mutation: %s", statement)
		}
	}
	head := published
	for i := 1; i <= 2; i++ {
		rollbackPath := fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s/rollback", w, head["releaseId"])
		head = productionRequestResult(t, h, http.MethodPost, rollbackPath, publisher, fmt.Sprintf("composite-rollback-%d", i), map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"], "expectedHead": map[string]any{"presence": "present", "releaseId": head["releaseId"], "manifestDigest": head["manifestDigest"]}, "reason": "restore complete before manifest"}, http.StatusCreated)
	}
	if head["manifestDigest"] != published["manifestDigest"] {
		t.Fatalf("repeated rollback changed pins: %v", head)
	}
	productionAssertCompositeProjection(t, f, w, head, 2)
	var grain string
	var counter int
	if err := f.pool.QueryRow(context.Background(), `SELECT grain_expression,version FROM model_grains WHERE workspace_id=$1`, w.UUID()).Scan(&grain, &counter); err != nil {
		t.Fatal(err)
	}
	if grain != "one row per order" || counter != 2 {
		t.Fatalf("registry restore lost business/counter: %s %d", grain, counter)
	}
	absence := productionRequestResult(t, h, http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s/rollback", w, head["releaseId"]), publisher, "composite-rollback-3", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"], "expectedHead": map[string]any{"presence": "present", "releaseId": head["releaseId"], "manifestDigest": head["manifestDigest"]}, "reason": "prepare trusted absence"}, http.StatusCreated)
	ids := map[string]string{}
	originalDetail := productionRequestResult(t, h, http.MethodGet, path+"/"+op, p, "", nil, http.StatusOK)
	for _, raw := range originalDetail["targets"].([]any) {
		target := raw.(map[string]any)
		ids[target["localKey"].(string)] = target["targetId"].(string)
	}
	expectedHead := map[string]any{"presence": "present", "releaseId": absence["releaseId"], "manifestDigest": absence["manifestDigest"]}
	for _, raw := range payload["targets"].([]any) {
		target := raw.(map[string]any)
		target["reuseIdentity"] = map[string]any{"targetId": ids[target["localKey"].(string)], "creationOperationId": op, "creationReleaseId": published["releaseId"], "absenceReleaseId": absence["releaseId"], "expectedHead": expectedHead}
	}
	reintroduced := productionRequestResult(t, h, http.MethodPost, path, p, "composite-reintroduce", payload, http.StatusCreated)
	reintroducedDetail := productionRequestResult(t, h, http.MethodGet, path+"/"+reintroduced["operationId"].(string), p, "", nil, http.StatusOK)
	for _, raw := range reintroducedDetail["targets"].([]any) {
		target := raw.(map[string]any)
		if target["targetId"] != ids[target["localKey"].(string)] {
			t.Fatalf("reintroduction allocated a different identity: %v", target)
		}
	}
	reintroducedDigest := productionValidateAndApprove(t, f, h, w, p, reviewer, reintroduced)
	republished := productionRequestResult(t, h, http.MethodPost, path+"/"+reintroduced["operationId"].(string)+"/publish", publisher, "composite-republish", map[string]any{"expectedVersion": 1, "setDigest": reintroduced["setDigest"], "validation": map[string]any{"attemptNo": 1, "validationDigest": reintroducedDigest}, "expectedHead": expectedHead}, http.StatusCreated)
	if err := f.pool.QueryRow(context.Background(), `SELECT version FROM model_grains WHERE workspace_id=$1`, w.UUID()).Scan(&counter); err != nil {
		t.Fatal(err)
	}
	if counter != 3 {
		t.Fatalf("reintroduction did not CAS existing registry counter: %d", counter)
	}
	var revisionUUID pgtype.UUID
	if err := f.pool.QueryRow(context.Background(), `SELECT current_revision_id FROM semantic_assets WHERE workspace_id=$1`, w.UUID()).Scan(&revisionUUID); err != nil {
		t.Fatal(err)
	}
	revision, err := identity.RevisionIDFromUUIDBytes(revisionUUID.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{"semantic_asset": "definition", "physical_binding": "transform", "model_grain": "expression", "entity_key": "uniqueness", "join_contract": "notes"}
	afterValues := map[string]string{"semantic_asset": "Revenue after adjustments", "physical_binding": "id::text", "model_grain": "one row per adjusted order", "entity_key": "deduplicated", "join_contract": "updated join notes"}
	for _, raw := range payload["targets"].([]any) {
		target := raw.(map[string]any)
		kind := target["kind"].(string)
		delete(target, "reuseIdentity")
		delete(target, "identityKey")
		target["intent"], target["targetId"] = "update", ids[target["localKey"].(string)]
		if kind == "semantic_asset" {
			target["baseRevisionId"] = revision.String()
		} else {
			target["baseObjectVersion"] = 3
		}
		content := target["content"].(map[string]any)
		if _, ok := content["asset"]; ok {
			content["asset"] = map[string]any{"kind": "semantic_asset", "targetId": ids["revenue"], "releaseId": republished["releaseId"], "revisionId": revision.String()}
		}
		field := fields[kind]
		before := content[field]
		content[field] = afterValues[kind]
		target["changes"] = []any{map[string]any{"fieldPath": field, "op": "update", "beforeValue": before, "afterValue": content[field]}}
	}
	declarationsJSON, _ := json.Marshal(payload["targets"])
	payload["input"].(map[string]any)["dependencies"] = []any{map[string]any{"kind": "semantic_asset", "targetId": ids["revenue"], "releaseId": republished["releaseId"], "revisionId": revision.String()}}
	var declarations []domain.TargetDeclaration
	if err := json.Unmarshal(declarationsJSON, &declarations); err != nil {
		t.Fatal(err)
	}
	if err := domain.ValidateProductionContentDeclarations(declarations); err != nil {
		t.Fatalf("update declarations: %v", err)
	}
	updated := productionRequestResult(t, h, http.MethodPost, path, p, "composite-update", payload, http.StatusCreated)
	updateDigest := productionValidateAndApprove(t, f, h, w, p, reviewer, updated)
	updatedRelease := productionRequestResult(t, h, http.MethodPost, path+"/"+updated["operationId"].(string)+"/publish", publisher, "composite-update-publish", map[string]any{"expectedVersion": 1, "setDigest": updated["setDigest"], "validation": map[string]any{"attemptNo": 1, "validationDigest": updateDigest}, "expectedHead": map[string]any{"presence": "present", "releaseId": republished["releaseId"], "manifestDigest": republished["manifestDigest"]}}, http.StatusCreated)
	if err := f.pool.QueryRow(context.Background(), `SELECT grain_expression,version FROM model_grains WHERE workspace_id=$1`, w.UUID()).Scan(&grain, &counter); err != nil {
		t.Fatal(err)
	}
	if grain != afterValues["model_grain"] || counter != 4 {
		t.Fatalf("update did not persist exact business content: %s %d", grain, counter)
	}
	restored := productionRequestResult(t, h, http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s/rollback", w, updatedRelease["releaseId"]), publisher, "composite-update-rollback", map[string]any{"expectedVersion": 1, "setDigest": updated["setDigest"], "expectedHead": map[string]any{"presence": "present", "releaseId": updatedRelease["releaseId"], "manifestDigest": updatedRelease["manifestDigest"]}, "reason": "restore all five updates"}, http.StatusCreated)
	if restored["manifestDigest"] != republished["manifestDigest"] {
		t.Fatalf("update rollback did not restore exact prior manifest: %v", restored)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT grain_expression,version FROM model_grains WHERE workspace_id=$1`, w.UUID()).Scan(&grain, &counter); err != nil {
		t.Fatal(err)
	}
	if grain != "one row per order" || counter != 5 {
		t.Fatalf("update rollback lost registry content/counter: %s %d", grain, counter)
	}
	payload["input"].(map[string]any)["dependencies"] = []any{}
	var extraContent map[string]any
	for _, raw := range payload["targets"].([]any) {
		target := raw.(map[string]any)
		if target["kind"] == "semantic_asset" {
			extraContent = target["content"].(map[string]any)
		}
	}
	extraContent["address"] = "finance.extra_revenue"
	payload["targets"] = []any{map[string]any{"localKey": "extra", "kind": "semantic_asset", "intent": "create", "identityKey": "finance.extra_revenue", "title": "Extra revenue", "content": extraContent, "changes": []any{}, "evidenceIds": []any{}}}
	payload["targets"].([]any)[0].(map[string]any)["evidenceIds"] = []any{payload["input"].(map[string]any)["evidence"].([]any)[0].(map[string]any)["evidenceId"]}
	extra := productionRequestResult(t, h, http.MethodPost, path, p, "extra-create", payload, http.StatusCreated)
	extraDigest := productionValidateAndApprove(t, f, h, w, p, reviewer, extra)
	extraRelease := productionRequestResult(t, h, http.MethodPost, path+"/"+extra["operationId"].(string)+"/publish", publisher, "extra-publish", map[string]any{"expectedVersion": 1, "setDigest": extra["setDigest"], "validation": map[string]any{"attemptNo": 1, "validationDigest": extraDigest}, "expectedHead": map[string]any{"presence": "present", "releaseId": restored["releaseId"], "manifestDigest": restored["manifestDigest"]}}, http.StatusCreated)
	extraDetail := productionRequestResult(t, h, http.MethodGet, fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s", w, extraRelease["releaseId"]), publisher, "", nil, http.StatusOK)
	extraManifest := extraDetail["afterManifest"].(map[string]any)
	if len(extraManifest["assets"].([]any)) != 2 || len(extraManifest["objects"].([]any)) != 4 {
		t.Fatalf("single-target publication lost unchanged pins: %v", extraManifest)
	}
	scoped := mustID(t, identity.NewPrincipalID)
	if _, err := f.store.CreatePrincipal(context.Background(), authz.Principal{ID: scoped, WorkspaceID: w, Kind: authz.PrincipalHuman, DisplayName: "Extra-only reader", Status: authz.PrincipalActive}); err != nil {
		t.Fatal(err)
	}
	var extraAsset pgtype.UUID
	if err := f.pool.QueryRow(context.Background(), `SELECT id FROM semantic_assets WHERE workspace_id=$1 AND key='extra_revenue'`, w.UUID()).Scan(&extraAsset); err != nil {
		t.Fatal(err)
	}
	extraAssetID, err := identity.AssetIDFromUUIDBytes(extraAsset.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	source, err := identity.ParseSourceConnectionID(payload["input"].(map[string]any)["snapshots"].([]any)[0].(map[string]any)["sourceId"].(string))
	if err != nil {
		t.Fatal(err)
	}
	for _, grant := range []authz.RoleBinding{{ID: mustID(t, identity.NewBindingID), PrincipalID: scoped, RoleID: "asset_owner", ScopeType: authz.ScopeAsset, ScopeID: extraAssetID.UUID()}, {ID: mustID(t, identity.NewBindingID), PrincipalID: scoped, RoleID: "source_operator", ScopeType: authz.ScopeSource, ScopeID: source.UUID()}, {ID: mustID(t, identity.NewBindingID), PrincipalID: scoped, RoleID: "auditor", ScopeType: authz.ScopeSource, ScopeID: source.UUID()}} {
		if _, err := f.store.CreateRoleBinding(context.Background(), grant); err != nil {
			t.Fatal(err)
		}
	}
	productionRequestResult(t, h, http.MethodGet, path+"/"+extra["operationId"].(string), scoped, "", nil, http.StatusOK)
	productionRequestResult(t, h, http.MethodGet, fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s", w, extraRelease["releaseId"]), scoped, "", nil, http.StatusForbidden)
	if _, err := f.store.CreateRoleBinding(context.Background(), authz.RoleBinding{ID: mustID(t, identity.NewBindingID), PrincipalID: scoped, RoleID: "semantic_steward", ScopeType: authz.ScopeDomain, ScopeID: "finance"}); err != nil {
		t.Fatal(err)
	}
	productionRequestResult(t, h, http.MethodGet, fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s", w, extraRelease["releaseId"]), scoped, "", nil, http.StatusOK)
	extraRestored := productionRequestResult(t, h, http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s/rollback", w, extraRelease["releaseId"]), publisher, "extra-rollback", map[string]any{"expectedVersion": 1, "setDigest": extra["setDigest"], "expectedHead": map[string]any{"presence": "present", "releaseId": extraRelease["releaseId"], "manifestDigest": extraRelease["manifestDigest"]}, "reason": "restore unchanged complete manifest"}, http.StatusCreated)
	if extraRestored["manifestDigest"] != restored["manifestDigest"] {
		t.Fatalf("rollback lost unchanged pins: %v", extraRestored)
	}
}
