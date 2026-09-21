package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/discovery/catalog"
	"github.com/iiwish/semlia/internal/adapters/gitcontent"
	app "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/jobs"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/internal/domain/discovery"
	gov "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

// Both the opt-in model run and deterministic acceptance use real discovery.
func commerceProductionPayload(t *testing.T, f *fixture, w identity.WorkspaceID, actor identity.PrincipalID) map[string]any {
	t.Helper()
	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(), "tests/fixtures/semantic-production/commerce.catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	source := mustID(t, identity.NewSourceConnectionID)
	_, err = f.store.CreateSourceConnection(ctx, semantic.SourceConnection{ID: source, WorkspaceID: w, AdapterKind: "catalog", Name: "Synthetic commerce", NormalizedLocator: "fixture://" + source.String(), Status: "active", Metadata: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := (catalog.Adapter{}).Discover(ctx, discovery.Input{Locator: "catalog://commerce", ObservedAt: time.Now().UTC(), Files: map[string][]byte{"catalog.json": raw}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.store.PersistDiscoverySnapshot(ctx, w, source, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var sid, digest string
	if err := f.pool.QueryRow(ctx, `SELECT s.id::text,s.content_digest FROM source_snapshots s JOIN source_snapshot_runs r ON r.workspace_id=s.workspace_id AND r.snapshot_id=s.id WHERE r.workspace_id=$1 AND r.run_id=$2`, w.UUID(), run.RunID.UUID()).Scan(&sid, &digest); err != nil {
		t.Fatal(err)
	}
	snapshotID, err := identity.FromUUID(identity.SourceSnapshot, sid)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.pool.Query(ctx, `SELECT coverage_key FROM source_snapshot_scope WHERE workspace_id=$1 AND snapshot_id=$2 ORDER BY coverage_key`, w.UUID(), sid)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"snapshots": []any{map[string]any{"sourceId": source.String(), "snapshotId": snapshotID.String(), "digest": digest, "coverageKeys": keys}}, "candidates": []any{}, "evidence": []any{}, "dependencies": []any{}}
	targets := []any{}
	for _, name := range []string{"orders", "customers", "revenue"} {
		kind := "business_object"
		var spec any = syntheticObjectSpec()
		if name == "revenue" {
			// This fixture proves authoring/review, not query execution.
			kind = "business_term"
			spec = map[string]any{"capability": "definition"}
		}
		targets = append(targets, map[string]any{"localKey": name, "title": name, "kind": "semantic_asset", "intent": "create", "identityKey": "commerce." + name, "changes": []any{}, "evidenceIds": []any{}, "content": map[string]any{"address": "commerce." + name, "assetType": kind, "spec": spec, "displayName": name, "definition": nil, "scope": nil, "ownerPrincipalId": actor.String()}})
	}
	ref := func(kind, dataset, field string) map[string]any {
		t.Helper()
		var objectUUID, revisionUUID string
		err := f.pool.QueryRow(ctx, `SELECT m.object_id::text,m.revision_id::text FROM source_snapshot_members m LEFT JOIN source_snapshot_members parent ON parent.workspace_id=m.workspace_id AND parent.snapshot_id=m.snapshot_id AND parent.kind='dataset' AND parent.object_id=m.parent_object_id WHERE m.workspace_id=$1 AND m.snapshot_id=$2 AND m.kind=$3 AND (($3='dataset' AND m.historical_name=$4) OR ($3='field' AND parent.historical_name=$4 AND m.historical_name=$5))`, w.UUID(), sid, kind, "public."+dataset, field).Scan(&objectUUID, &revisionUUID)
		if err != nil {
			t.Fatal(err)
		}
		op, rp := identity.PhysicalDataset, identity.PhysicalDatasetRevision
		if kind == "field" {
			op, rp = identity.PhysicalField, identity.PhysicalFieldRevision
		}
		object, err := identity.FromUUID(op, objectUUID)
		if err != nil {
			t.Fatal(err)
		}
		revision, err := identity.FromUUID(rp, revisionUUID)
		if err != nil {
			t.Fatal(err)
		}
		return map[string]any{"snapshotId": snapshotID.String(), "kind": kind, "objectId": object.String(), "revisionId": revision.String()}
	}
	add := func(key, kind string, content map[string]any) {
		targets = append(targets, map[string]any{"localKey": key, "title": key, "kind": kind, "intent": "create", "identityKey": "commerce." + key, "changes": []any{}, "evidenceIds": []any{}, "content": content})
	}
	for _, name := range []string{"orders", "customers", "revenue"} {
		dataset := name
		if name == "revenue" {
			dataset = "orders"
		}
		content := map[string]any{"asset": map[string]any{"localKey": name}, "dataset": ref("dataset", dataset, "")}
		if name == "revenue" {
			content["field"], content["transform"] = ref("field", "orders", "amount"), "amount"
		}
		add(name+"_binding", "physical_binding", content)
	}
	for _, name := range []string{"orders", "customers"} {
		field := "order_id"
		if name == "customers" {
			field = "customer_id"
		}
		add(name+"_key", "entity_key", map[string]any{"asset": map[string]any{"localKey": name}, "fields": []any{ref("field", name, field)}, "uniqueness": "exact"})
	}
	add("revenue_grain", "model_grain", map[string]any{"asset": map[string]any{"localKey": "revenue"}, "expression": "one row per order_id", "fields": []any{ref("field", "orders", "order_id")}})
	add("orders_customers_join", "join_contract", map[string]any{"leftDataset": ref("dataset", "orders", ""), "rightDataset": ref("dataset", "customers", ""), "pairs": []any{map[string]any{"left": ref("field", "orders", "customer_id"), "right": ref("field", "customers", "customer_id")}}, "joinType": "inner", "cardinality": "many_to_one", "expression": "orders.customer_id = customers.customer_id", "notes": "Synthetic proposed relationship; requires validation and human review."})
	return map[string]any{"input": input, "targets": targets}
}

func TestSemanticProductionCommerceColdStart(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	for _, table := range []string{"semantic_assets", "proposals", "releases", "consumers"} {
		assertTableCount(t, f.pool, table, 0)
	}
	payload := commerceProductionPayload(t, f, w, p)
	path := "/api/v1/workspaces/" + w.String() + "/production-operations"
	created := productionRequestResult(t, h, http.MethodPost, path, p, "commerce-cold-start", payload, http.StatusCreated)
	if len(created["proposalIds"].([]any)) != 10 {
		t.Fatal("cold start did not create ten composite targets")
	}
	replay := productionRequestResult(t, h, http.MethodPost, path, p, "commerce-cold-start", payload, http.StatusOK)
	if replay["operationId"] != created["operationId"] {
		t.Fatal("cold start replay changed identity")
	}
	assertTableCount(t, f.pool, "releases", 0)
}

func commerceReleaseAfterConfirmation(t *testing.T, f *fixture, h http.Handler, w identity.WorkspaceID, p identity.PrincipalID, applied map[string]any) map[string]any {
	t.Helper()
	op := mustParseProductionOperationID(t, applied["operationId"].(string))
	version := int(applied["version"].(float64))
	base := "/api/v1/workspaces/" + w.String() + "/production-operations/" + op.String()
	productionRequestResult(t, h, http.MethodPost, base+"/submit", p, "commerce-submit-unconfirmed", map[string]any{"expectedVersion": version, "setDigest": applied["setDigest"]}, http.StatusAccepted)
	if status := runBusinessRuleAttempt(t, f, w, op, version, 1); status != "failed" {
		t.Fatal("unconfirmed model-derived content passed validation")
	}
	var failure json.RawMessage
	if err := f.pool.QueryRow(context.Background(), `SELECT results_json FROM production_validation_seals WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3 AND attempt_no=1`, w.UUID(), op.UUID(), version).Scan(&failure); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(failure), "PRODUCTION_BUSINESS_RULE_UNCONFIRMED") {
		t.Fatal("missing explicit unconfirmed-rule blocker")
	}
	for _, key := range []string{"orders", "customers", "revenue"} {
		productionRequestResult(t, h, http.MethodPost, base+"/business-rule-confirmations", p, "commerce-confirm-"+key, map[string]any{"expectedVersion": version, "setDigest": applied["setDigest"], "targetKey": key, "action": "confirm", "declaration": "Synthetic human confirmation of the reviewed commerce gold answer: paid revenue excludes cancelled orders; order and customer identifiers define the entity grain."}, http.StatusCreated)
	}
	productionRequestResult(t, h, http.MethodPost, base+"/validations", p, "commerce-revalidate", map[string]any{"expectedVersion": version, "setDigest": applied["setDigest"], "previousAttemptNo": 1, "reason": "Human explicitly confirmed corrected commercial policy."}, http.StatusAccepted)
	if status := runBusinessRuleAttempt(t, f, w, op, version, 2); status != "succeeded" {
		var findings json.RawMessage
		_ = f.pool.QueryRow(context.Background(), `SELECT results_json FROM production_validation_seals WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3 AND attempt_no=2`, w.UUID(), op.UUID(), version).Scan(&findings)
		t.Fatalf("confirmed commerce failed validation: %s", findings)
	}
	attempt, err := f.store.GetValidationAttempt(context.Background(), w, op, version, 2)
	if err != nil {
		t.Fatal(err)
	}
	validation := map[string]any{"attemptNo": 2, "validationDigest": *attempt.ValidationDigest}
	review := map[string]any{"expectedVersion": version, "setDigest": applied["setDigest"], "validation": validation, "proposalIds": applied["proposalIds"], "decision": "approve", "note": "Independent synthetic review of all ten targets and corrected paid-only revenue transform."}
	productionRequestResult(t, h, http.MethodPost, base+"/reviews", p, "commerce-self-review", review, http.StatusForbidden)
	reviewer := authoringLifecyclePrincipal(t, f, w, "Commerce reviewer")
	publisher := authoringLifecyclePrincipal(t, f, w, "Commerce publisher")
	productionRequestResult(t, h, http.MethodPost, base+"/reviews", reviewer, "commerce-independent-review", review, http.StatusCreated)
	published := productionRequestResult(t, h, http.MethodPost, base+"/publish", publisher, "commerce-publish", map[string]any{"expectedVersion": version, "setDigest": applied["setDigest"], "validation": validation, "expectedHead": map[string]any{"presence": "absent"}}, http.StatusCreated)
	rid, err := identity.ParseReleaseID(published["releaseId"].(string))
	if err != nil {
		t.Fatal(err)
	}
	projection, err := f.store.LoadReleaseProjection(context.Background(), w, rid)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Assets) != 3 || projection.Production == nil || len(projection.Production.Proposals) != 10 {
		t.Fatal("published model-derived set lost assets or attribution")
	}
	_, _, persistedTargets, _, _, err := f.store.GetProductionOperation(context.Background(), w, op)
	if err != nil {
		t.Fatal(err)
	}
	publishedBusiness := map[string]any{}
	for _, target := range persistedTargets {
		if target.LocalKey == "revenue" {
			assetID, err := identity.ParseAssetID(target.TargetID)
			if err != nil {
				t.Fatal(err)
			}
			asset, err := f.store.GetCatalogAsset(context.Background(), w, assetID)
			if err != nil {
				t.Fatal(err)
			}
			var content map[string]any
			if err := json.Unmarshal(asset.CurrentRevision.Content, &content); err != nil {
				t.Fatal(err)
			}
			if content["definition"] != "Sum of paid order amounts, excluding cancelled orders." {
				t.Fatal("published revenue dropped human correction")
			}
			publishedBusiness["revenue"] = content
		}
		if target.LocalKey == "revenue_binding" {
			bindingID, err := identity.ParsePhysicalBindingID(target.TargetID)
			if err != nil {
				t.Fatal(err)
			}
			var transform string
			if err := f.pool.QueryRow(context.Background(), `SELECT content->>'transform' FROM physical_bindings WHERE workspace_id=$1 AND id=$2`, w.UUID(), bindingID.UUID()).Scan(&transform); err != nil {
				t.Fatal(err)
			}
			if transform != "CASE WHEN status = 'paid' THEN amount ELSE 0 END" {
				t.Fatal("published transform dropped human correction")
			}
			publishedBusiness["transform"] = transform
		}
	}
	writer, err := gitcontent.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projected, err := writer.ProjectRelease(context.Background(), projection)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := writer.ProjectRelease(context.Background(), projection)
	if err != nil || replayed.Changed {
		t.Fatal("model-derived projection replay changed content")
	}
	assertTableCount(t, f.pool, "consumers", 0)
	return map[string]any{"unconfirmedFindings": failure, "successfulAttempt": attempt, "published": published, "publishedBusinessContent": publishedBusiness, "projection": projected, "consumerCount": 0}
}

func TestSemanticProductionCommerceHumanCorrectionPublishes(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	payload := commerceProductionPayload(t, f, w, p)
	goldRaw, err := os.ReadFile(filepath.Join(repositoryRoot(), "tests/fixtures/semantic-production/commerce.gold.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gold map[string]json.RawMessage
	if err := json.Unmarshal(goldRaw, &gold); err != nil {
		t.Fatal(err)
	}
	for _, raw := range payload["targets"].([]any) {
		target := raw.(map[string]any)
		content := target["content"].(map[string]any)
		key := target["localKey"].(string)
		if target["kind"] == "semantic_asset" {
			var answer map[string]string
			if err := json.Unmarshal(gold[key], &answer); err != nil {
				t.Fatal(err)
			}
			content["definition"], content["scope"] = answer["definition"], answer["scope"]
		}
		if key == "revenue_binding" {
			var value string
			if err := json.Unmarshal(gold["transform"], &value); err != nil {
				t.Fatal(err)
			}
			content["transform"] = value
		}
	}
	created := productionRequestResult(t, h, http.MethodPost, "/api/v1/workspaces/"+w.String()+"/production-operations", p, "commerce-human-fixture", payload, http.StatusCreated)
	first := commerceReleaseAfterConfirmation(t, f, h, w, p, created)
	commerceChangedSourceProduction(t, f, h, w, p, payload, created, first["published"].(map[string]any))
}

func commerceChangedSourceProduction(t *testing.T, f *fixture, h http.Handler, w identity.WorkspaceID, p identity.PrincipalID, payload, created, first map[string]any) {
	t.Helper()
	ctx := context.Background()
	path := "/api/v1/workspaces/" + w.String() + "/production-operations"
	oldSelection := payload["input"].(map[string]any)["snapshots"].([]any)[0].(map[string]any)
	source, err := identity.ParseSourceConnectionID(oldSelection["sourceId"].(string))
	if err != nil {
		t.Fatal(err)
	}
	oldSnapshot, err := identity.ParseSourceSnapshotID(oldSelection["snapshotId"].(string))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(), "tests/fixtures/semantic-production/commerce.catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalogData map[string]any
	if err := json.Unmarshal(raw, &catalogData); err != nil {
		t.Fatal(err)
	}
	for _, rawDataset := range catalogData["datasets"].([]any) {
		dataset := rawDataset.(map[string]any)
		if dataset["external_key"] != "orders" {
			continue
		}
		for _, rawField := range dataset["fields"].([]any) {
			field := rawField.(map[string]any)
			if field["name"] == "amount" {
				field["data_type"] = "numeric(18,2)"
			}
		}
	}
	changedRaw, err := json.Marshal(catalogData)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := (catalog.Adapter{}).Discover(ctx, discovery.Input{Locator: "catalog://commerce", ObservedAt: time.Now().UTC(), Files: map[string][]byte{"catalog.json": changedRaw}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.store.PersistDiscoverySnapshot(ctx, w, source, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var sid, digest string
	if err := f.pool.QueryRow(ctx, `SELECT s.id::text,s.content_digest FROM source_snapshots s JOIN source_snapshot_runs r ON r.workspace_id=s.workspace_id AND r.snapshot_id=s.id WHERE r.workspace_id=$1 AND r.run_id=$2`, w.UUID(), run.RunID.UUID()).Scan(&sid, &digest); err != nil {
		t.Fatal(err)
	}
	newSnapshot, err := identity.FromUUID(identity.SourceSnapshot, sid)
	if err != nil {
		t.Fatal(err)
	}
	var unchangedCustomers bool
	if err := f.pool.QueryRow(ctx, `SELECT old.revision_id=current.revision_id AND old.historical_name=current.historical_name FROM source_snapshot_members old JOIN source_snapshot_members current ON current.workspace_id=old.workspace_id AND current.kind=old.kind AND current.object_id=old.object_id WHERE old.workspace_id=$1 AND old.snapshot_id=$2 AND current.snapshot_id=$3 AND old.kind='dataset' AND old.historical_name='public.customers'`, w.UUID(), oldSnapshot.UUID(), sid).Scan(&unchangedCustomers); err != nil || !unchangedCustomers {
		t.Fatal("source rescan lost unchanged customers history")
	}
	detail := productionRequestResult(t, h, http.MethodGet, path+"/"+created["operationId"].(string), p, "", nil, http.StatusOK)
	ids := map[string]string{}
	for _, rawTarget := range detail["targets"].([]any) {
		target := rawTarget.(map[string]any)
		ids[target["localKey"].(string)] = target["targetId"].(string)
	}
	assetID, err := identity.ParseAssetID(ids["revenue"])
	if err != nil {
		t.Fatal(err)
	}
	asset, err := f.store.GetCatalogAsset(ctx, w, assetID)
	if err != nil {
		t.Fatal(err)
	}
	dependency := map[string]any{"kind": "semantic_asset", "targetId": assetID.String(), "releaseId": first["releaseId"], "revisionId": asset.CurrentRevision.ID.String()}
	targets := []any{}
	for _, rawTarget := range payload["targets"].([]any) {
		target := rawTarget.(map[string]any)
		key := target["localKey"].(string)
		if key != "revenue" && key != "revenue_binding" {
			continue
		}
		copyRaw, _ := json.Marshal(target)
		var updated map[string]any
		if err := json.Unmarshal(copyRaw, &updated); err != nil {
			t.Fatal(err)
		}
		delete(updated, "identityKey")
		updated["intent"], updated["targetId"] = "update", ids[key]
		content := updated["content"].(map[string]any)
		fieldPath := "definition"
		if key == "revenue" {
			updated["baseRevisionId"] = asset.CurrentRevision.ID.String()
		} else {
			updated["baseObjectVersion"] = 1
			fieldPath = "transform"
			content["asset"] = dependency
			for _, name := range []string{"dataset", "field"} {
				ref := content[name].(map[string]any)
				prefix, rp := identity.PhysicalDataset, identity.PhysicalDatasetRevision
				if name == "field" {
					prefix, rp = identity.PhysicalField, identity.PhysicalFieldRevision
				}
				object, err := identity.Parse(prefix, ref["objectId"].(string))
				if err != nil {
					t.Fatal(err)
				}
				var revisionUUID string
				if err := f.pool.QueryRow(ctx, `SELECT revision_id::text FROM source_snapshot_members WHERE workspace_id=$1 AND snapshot_id=$2 AND kind=$3 AND object_id=$4`, w.UUID(), sid, name, object.UUID()).Scan(&revisionUUID); err != nil {
					t.Fatal(err)
				}
				revision, err := identity.FromUUID(rp, revisionUUID)
				if err != nil {
					t.Fatal(err)
				}
				ref["snapshotId"], ref["revisionId"] = newSnapshot.String(), revision.String()
			}
		}
		before := content[fieldPath]
		if key == "revenue" {
			content[fieldPath] = "Sum of paid order amounts normalized to numeric(18,2), excluding cancelled orders."
		} else {
			content[fieldPath] = "CASE WHEN status = 'paid' THEN amount::numeric(18,2) ELSE 0 END"
		}
		updated["changes"] = []any{map[string]any{"fieldPath": fieldPath, "op": "update", "beforeValue": before, "afterValue": content[fieldPath]}}
		targets = append(targets, updated)
	}
	newInput := map[string]any{"snapshots": []any{map[string]any{"sourceId": source.String(), "snapshotId": newSnapshot.String(), "digest": digest, "coverageKeys": oldSelection["coverageKeys"]}}, "candidates": []any{}, "evidence": []any{}, "dependencies": []any{dependency}}
	updated := productionRequestResult(t, h, http.MethodPost, path, p, "commerce-changed-source", map[string]any{"input": newInput, "targets": targets}, http.StatusCreated)
	base := path + "/" + updated["operationId"].(string)
	productionRequestResult(t, h, http.MethodPost, base+"/business-rule-confirmations", p, "commerce-changed-confirm", map[string]any{"expectedVersion": 1, "setDigest": updated["setDigest"], "targetKey": "revenue", "action": "confirm", "declaration": "Synthetic owner confirms paid-only revenue uses the observed numeric(18,2) precision and still excludes cancelled orders."}, http.StatusCreated)
	productionRequestResult(t, h, http.MethodPost, base+"/submit", p, "commerce-changed-submit", map[string]any{"expectedVersion": 1, "setDigest": updated["setDigest"]}, http.StatusAccepted)
	op := mustParseProductionOperationID(t, updated["operationId"].(string))
	if status := runBusinessRuleAttempt(t, f, w, op, 1, 1); status != "succeeded" {
		t.Fatal("changed-source validation failed")
	}
	attempt, err := f.store.GetValidationAttempt(ctx, w, op, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	validation := map[string]any{"attemptNo": 1, "validationDigest": *attempt.ValidationDigest}
	reviewer := authoringLifecyclePrincipal(t, f, w, "Changed source reviewer")
	publisher := authoringLifecyclePrincipal(t, f, w, "Changed source publisher")
	productionRequestResult(t, h, http.MethodPost, base+"/reviews", reviewer, "commerce-changed-review", map[string]any{"expectedVersion": 1, "setDigest": updated["setDigest"], "validation": validation, "proposalIds": updated["proposalIds"], "decision": "approve", "note": "Reviewed new physical precision, unchanged customer snapshot and exact prior metric baseline."}, http.StatusCreated)
	second := productionRequestResult(t, h, http.MethodPost, base+"/publish", publisher, "commerce-changed-publish", map[string]any{"expectedVersion": 1, "setDigest": updated["setDigest"], "validation": validation, "expectedHead": map[string]any{"presence": "present", "releaseId": first["releaseId"], "manifestDigest": first["manifestDigest"]}}, http.StatusCreated)
	restored := productionRequestResult(t, h, http.MethodPost, "/api/v1/workspaces/"+w.String()+"/production-releases/"+second["releaseId"].(string)+"/rollback", publisher, "commerce-changed-rollback", map[string]any{"expectedVersion": 1, "setDigest": updated["setDigest"], "expectedHead": map[string]any{"presence": "present", "releaseId": second["releaseId"], "manifestDigest": second["manifestDigest"]}, "reason": "Restore exact initial commerce content after changed-source acceptance."}, http.StatusCreated)
	if restored["manifestDigest"] != first["manifestDigest"] {
		t.Fatal("changed-source rollback lost original immutable pins")
	}
	replay := productionRequestResult(t, h, http.MethodPost, path, p, "commerce-changed-source", map[string]any{"input": newInput, "targets": targets}, http.StatusOK)
	if replay["operationId"] != updated["operationId"] {
		t.Fatal("changed-source replay created a duplicate operation")
	}
	t.Logf("commerce source snapshots %s -> %s; customers unchanged; releases %s -> %s -> rollback %s; replay operation %s", oldSnapshot, newSnapshot, first["releaseId"], second["releaseId"], restored["releaseId"], updated["operationId"])
}

func TestSemanticProductionConfiguredModel(t *testing.T) {
	if os.Getenv("SEMLIA_RUN_PRODUCTION_MODEL") != "1" {
		t.Skip("actual model requires explicit opt-in and synthetic-data authorization")
	}
	endpoint, model, secret := os.Getenv("SEMLIA_ACCEPTANCE_MODEL_URL"), os.Getenv("SEMLIA_ACCEPTANCE_MODEL_NAME"), os.Getenv("SEMLIA_ACCEPTANCE_MODEL_SECRET")
	root := os.Getenv("SEMLIA_MODEL_EVIDENCE_ROOT")
	if endpoint == "" || model == "" || secret == "" || !filepath.IsAbs(root) {
		t.Fatal("missing configured model or absolute owned evidence directory")
	}
	if marker, err := os.ReadFile(filepath.Join(root, ".owner")); err != nil || strings.TrimSpace(string(marker)) != "SP-T007" {
		t.Fatal("unowned model evidence directory")
	}
	f, h, w, p := authoringLifecycleSetup(t)
	payload := commerceProductionPayload(t, f, w, p)
	path := "/api/v1/workspaces/" + w.String() + "/production-operations"
	created := productionRequestResult(t, h, http.MethodPost, path, p, "commerce-actual-model", payload, http.StatusCreated)
	op := mustParseProductionOperationID(t, created["operationId"].(string))
	agent, err := f.store.WorkspaceAgentPrincipal(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"semantic_steward", "source_operator"} {
		_, err = f.store.CreateRoleBinding(context.Background(), auth.RoleBinding{ID: mustID(t, identity.NewBindingID), PrincipalID: agent.ID, RoleID: role, ScopeType: auth.ScopeWorkspace, ScopeID: w.UUID()})
		if err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	provider, err := f.store.CreateModelProvider(context.Background(), gov.ModelProvider{ID: mustID(t, identity.NewModelProviderID), WorkspaceID: w, Protocol: gov.ProtocolOpenAICompatible, DisplayName: "Authorized actual model", BaseURL: &endpoint, CredentialEnv: "SEMLIA_ACCEPTANCE_MODEL_SECRET", CredentialRevision: app.CredentialRevisionDigest(secret), Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal("create isolated model provider failed")
	}
	limit, err := strconv.Atoi(os.Getenv("SEMLIA_ACCEPTANCE_MODEL_CONTEXT"))
	if err != nil || limit < 16384 {
		t.Fatal("invalid model context limit")
	}
	setting, err := f.store.CreateModelSetting(context.Background(), gov.ModelSetting{ID: mustID(t, identity.NewModelSettingID), WorkspaceID: w, ProviderID: provider.ID, Kind: gov.ModelKindLLM, Model: model, Capability: "production suggestions", Enabled: true, TokenLimit: limit, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := gov.ProductionGenerationModelRevision(setting, provider)
	if err != nil {
		t.Fatal(err)
	}
	grant := gov.ProductionGenerationGrant{WorkspaceID: w, PrincipalID: p, ModelSettingID: setting.ID, ModelConfigRevision: revision, PricingBasis: "User authorized up to 30 synthetic calls without a monetary cap; conservative application ceiling, not actual price", MaxInputBytes: 1 << 20, MaxOutputTokens: 16384, MaxCostMicros: 1000000000, InputMicrosPerByte: 1, OutputMicrosPerToken: 1}
	service := app.NewProductionGenerationService(f.store, []gov.ProductionGenerationGrant{grant})
	_, ver, _, _, _, err := f.store.GetProductionOperation(context.Background(), w, op)
	if err != nil {
		t.Fatal(err)
	}
	instruction := "Suggest structural definitions and scope for the three semantic targets and inspect their seven related binding, key, grain and join skeletons using only the supplied physical evidence. Return all ten targets. Preserve target identities, local references and owner references. Revenue business rules are not supplied; keep unsupported commercial policy unresolved. Do not invent paid/cancelled status meaning. Return JSON only."
	request := gov.ProductionGenerationRequest{WorkspaceID: w, PrincipalID: p, OperationID: op, IdempotencyKey: "commerce-model-call", ExpectedVersion: 1, InputDigest: ver.InputDigest, ModelSettingID: setting.ID, ModelConfigRevision: revision, Instruction: instruction, MaxOutputTokens: 16384, MaxCostMicros: 1000000000, TraceID: traceID}
	queued, err := service.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	slot, releaseSlot, err := reserveProductionModelCall(root)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSlot()
	write := func(suffix string, value any) {
		t.Helper()
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), secret) {
			t.Fatal("refusing credential-bearing evidence")
		}
		if err := os.WriteFile(slot+suffix, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("-input.json", map[string]any{"payload": payload, "instruction": instruction, "model": model, "contextLimit": limit, "maxOutputTokens": request.MaxOutputTokens, "goldIncluded": false})
	worker := jobs.NewWorker(f.store, jobs.ClockFunc(time.Now), jobs.BackoffFunc(func(int32) time.Duration { return time.Millisecond }), time.Minute)
	worker.Register(app.ProductionGenerationJobType, service.JobHandler())
	processed, runErr := worker.RunOne(context.Background(), "commerce-actual-model")
	result, readErr := service.Get(context.Background(), w, p, op, queued.RunID)
	if readErr != nil {
		t.Fatal("cannot read model outcome; do not retry")
	}
	write("-result.json", result)
	if runErr != nil || !processed {
		t.Fatal("model worker failed; outcome retained, do not retry")
	}
	if result.ProviderMode != "actual_model" || result.Status != "succeeded" {
		t.Fatalf("actual model status=%s code=%v; inspect retained result, no automatic retry", result.Status, result.ErrorCode)
	}
	var output struct {
		Targets []map[string]any `json:"targets"`
	}
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Targets) != 10 {
		t.Fatal("actual model omitted or added composite targets")
	}
	goldRaw, err := os.ReadFile(filepath.Join(repositoryRoot(), "tests/fixtures/semantic-production/commerce.gold.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gold map[string]json.RawMessage
	if err := json.Unmarshal(goldRaw, &gold); err != nil {
		t.Fatal(err)
	}
	for _, target := range output.Targets {
		key, ok := target["localKey"].(string)
		if !ok {
			t.Fatal("model target has no local key")
		}
		if target["kind"] != "semantic_asset" {
			if key == "revenue_binding" {
				var transform string
				if err := json.Unmarshal(gold["transform"], &transform); err != nil {
					t.Fatal(err)
				}
				target["content"].(map[string]any)["transform"] = transform
			}
			continue
		}
		var answer map[string]string
		if err := json.Unmarshal(gold[key], &answer); err != nil {
			t.Fatal("unknown model target")
		}
		content, ok := target["content"].(map[string]any)
		if !ok {
			t.Fatal("model target has no content")
		}
		content["definition"], content["scope"] = answer["definition"], answer["scope"]
	}
	payload["expectedVersion"], payload["suggestionRunId"], payload["targets"] = 1, queued.RunID.String(), output.Targets
	applied := productionRequestResult(t, h, http.MethodPut, path+"/"+op.String(), p, "commerce-human-correction", payload, http.StatusOK)
	write("-human-correction.json", map[string]any{"payload": payload, "result": applied, "disclosure": "Synthetic human answer applied through the production API; not model output or human attestation."})
	appliedDetail := productionRequestResult(t, h, http.MethodGet, path+"/"+op.String(), p, "", nil, http.StatusOK)
	for _, rawTarget := range appliedDetail["targets"].([]any) {
		stored := rawTarget.(map[string]any)["declaration"].(map[string]any)
		var expected map[string]any
		for _, target := range output.Targets {
			if target["localKey"] == stored["localKey"] {
				expected = target
				break
			}
		}
		if expected == nil {
			t.Fatal("server introduced unexpected target")
		}
		want, _ := json.Marshal(expected["content"])
		got, _ := json.Marshal(stored["content"])
		wantDigest, _ := gov.DigestJSON(want)
		gotDigest, _ := gov.DigestJSON(got)
		if wantDigest != gotDigest {
			t.Fatal("server did not preserve exact human-corrected content")
		}
	}
	write("-applied-operation.json", appliedDetail)
	again, err := service.Get(context.Background(), w, p, op, queued.RunID)
	if err != nil || string(again.Output) != string(result.Output) {
		t.Fatal("human correction overwrote raw output")
	}
	replay, err := service.Generate(context.Background(), request)
	if err != nil || replay.RunID != queued.RunID {
		t.Fatal("model replay changed run")
	}
	assertTableCount(t, f.pool, "production_generation_applications", 1)
	assertTableCount(t, f.pool, "releases", 0)
	write("-release.json", commerceReleaseAfterConfirmation(t, f, h, w, p, applied))
	t.Logf("actual model result retained at %s-result.json; human correction version=%v; independent review and publication passed", slot, applied["version"])
}

func reserveProductionModelCall(root string) (string, func(), error) {
	lock := filepath.Join(root, ".model-call-lock")
	file, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", nil, fmt.Errorf("model invocation lock unavailable; inspect prior invocation: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", nil, err
	}
	var once sync.Once
	release := func() { once.Do(func() { _ = os.Remove(lock) }) }
	reserved := false
	defer func() {
		if !reserved {
			release()
		}
	}()
	// A durable reservation without a known terminal result must never be retried.
	for n := 1; n <= 30; n++ {
		name := filepath.Join(root, fmt.Sprintf("call-%02d.reserved", n))
		if _, err := os.Stat(name); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return "", nil, err
		}
		raw, err := os.ReadFile(strings.TrimSuffix(name, ".reserved") + "-result.json")
		var result struct {
			Status    string `json:"status"`
			ErrorCode string `json:"errorCode"`
		}
		if err != nil || json.Unmarshal(raw, &result) != nil ||
			(result.Status != "succeeded" && result.Status != "failed") || result.ErrorCode == "GENERATION_OUTCOME_UNKNOWN" {
			return "", nil, fmt.Errorf("prior model invocation %d has no known terminal result; do not retry", n)
		}
	}
	for n := 1; n <= 30; n++ {
		name := filepath.Join(root, fmt.Sprintf("call-%02d.reserved", n))
		file, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		if err := file.Close(); err != nil {
			return "", nil, err
		}
		reserved = true
		return strings.TrimSuffix(name, ".reserved"), release, nil
	}
	return "", nil, fmt.Errorf("30 actual-call slots exhausted")
}
