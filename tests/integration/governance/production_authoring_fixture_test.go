package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/discovery/catalog"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	discovery "github.com/iiwish/semlia/internal/domain/discovery"
	governance "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

// The source history is produced by the real discovery projection, not by
// inserting a fabricated source snapshot or bypassing its sealing triggers.
func productionFixtureInput(t *testing.T, f *fixture, workspace identity.WorkspaceID, candidate identity.SemanticCandidateID, businessEvidence ...bool) map[string]any {
	t.Helper()
	ctx := context.Background()
	source := mustID(t, identity.NewSourceConnectionID)
	if _, err := f.store.CreateSourceConnection(ctx, semantic.SourceConnection{ID: source, WorkspaceID: workspace, AdapterKind: "fixture", Name: "Production source", NormalizedLocator: "fixture://" + source.String(), Status: "active", Metadata: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (catalog.Adapter{}).Discover(ctx, discovery.Input{Locator: "catalog://production", ObservedAt: time.Now().UTC(), Files: map[string][]byte{"catalog.json": []byte(`{"version":"1","datasets":[{"external_key":"orders","qualified_name":"orders","kind":"table","locator":"warehouse.orders","fields":[{"external_key":"order-id","name":"id","ordinal":1,"data_type":"bigint","nullable":false}]}]}`)}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.store.PersistDiscoverySnapshot(ctx, workspace, source, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var snapshotUUID, revisionUUID, digest string
	if err := f.pool.QueryRow(ctx, `SELECT s.id::text,s.source_revision_id::text,s.content_digest FROM source_snapshots s JOIN source_snapshot_runs r ON r.workspace_id=s.workspace_id AND r.snapshot_id=s.id WHERE r.workspace_id=$1 AND r.run_id=$2`, workspace.UUID(), result.RunID.UUID()).Scan(&snapshotUUID, &revisionUUID, &digest); err != nil {
		t.Fatal(err)
	}
	uuidValue, err := uuid.Parse(snapshotUUID)
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.SourceSnapshotIDFromUUIDBytes(uuidValue)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.pool.Query(ctx, `SELECT coverage_key FROM source_snapshot_scope WHERE workspace_id=$1 AND snapshot_id=$2 ORDER BY coverage_key`, workspace.UUID(), snapshotUUID)
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
	input := map[string]any{"snapshots": []any{map[string]any{"sourceId": source.String(), "snapshotId": id.String(), "digest": digest, "coverageKeys": keys}}, "candidates": []any{}, "evidence": []any{}, "dependencies": []any{}}
	if len(businessEvidence) > 0 && businessEvidence[0] {
		// Synthetic business declaration, not an observation or a trusted confirmation.
		metadata := json.RawMessage(`{"disclosure":"Synthetic test business rules; the human confirmation API is required."}`)
		digest, err := governance.DigestJSON(metadata)
		if err != nil {
			t.Fatal(err)
		}
		revisionBytes, err := uuid.Parse(revisionUUID)
		if err != nil {
			t.Fatal(err)
		}
		revision, err := identity.SourceRevisionIDFromUUIDBytes(revisionBytes)
		if err != nil {
			t.Fatal(err)
		}
		evidence, err := f.store.CreateEvidence(ctx, semantic.EvidenceArtifact{ID: mustID(t, identity.NewEvidenceID), WorkspaceID: workspace, EvidenceType: "declared", SourceRevisionID: &revision, Locator: "fixture://business-rules/" + source.String(), ContentDigest: digest, Metadata: metadata})
		if err != nil {
			t.Fatal(err)
		}
		input["evidence"] = []any{map[string]any{"evidenceId": evidence.ID.String(), "snapshotId": id.String(), "digest": digest}}
	}
	if !candidate.IsZero() {
		content := []byte(`{"schemaVersion":"semlia.proposal-input/v1","candidateKind":"entity","qualifiedName":"orders"}`)
		candidateDigest, err := governance.DigestJSON(content)
		if err != nil {
			t.Fatal(err)
		}
		evidence, _ := json.Marshal([]map[string]string{{"sourceRevisionId": revisionUUID, "discoveryRunId": result.RunID.String(), "locator": "warehouse.orders"}})
		if _, err := f.pool.Exec(ctx, `INSERT INTO semantic_candidates(id,workspace_id,source_connection_id,source_revision_id,discovery_run_id,candidate_key,candidate_kind,title,proposal_input,evidence,content_digest) VALUES($1,$2,$3,$4,$5,'orders','entity','Orders',$6,$7,$8)`, candidate.UUID(), workspace.UUID(), source.UUID(), revisionUUID, result.RunID.UUID(), content, evidence, candidateDigest); err != nil {
			t.Fatal(err)
		}
		input["candidates"] = []any{map[string]any{"candidateId": candidate.String(), "snapshotId": id.String(), "digest": candidateDigest}}
	}
	return input
}

func productionPublishedAsset(t *testing.T, f *fixture, w identity.WorkspaceID, author identity.PrincipalID) (identity.AssetID, identity.RevisionID, json.RawMessage) {
	t.Helper()
	content, _ := json.Marshal(map[string]any{"address": "finance.revenue", "assetType": "metric", "displayName": "Revenue", "definition": "Revenue after refunds", "scope": "Finance", "ownerPrincipalId": author.String()})
	service := catalogapp.NewService(f.store, catalogapp.ClockFunc(time.Now))
	created, err := service.CreateAsset(context.Background(), catalogapp.CreateAssetRequest{WorkspaceID: w, Address: "finance.revenue", AssetType: semantic.Metric, Lifecycle: "active", SchemaVersion: "1.0.0", Content: content, CreatedBy: author.String(), TraceID: traceID})
	if err != nil {
		t.Fatal(err)
	}
	reviewer := createReviewerPrincipal(t, f, w, "baseline-reviewer")
	publisher := createPrincipalWithRoles(t, f, w, "baseline-publisher", []string{"publisher"})
	_, _, proposal := proposeToInReview(t, f, w, author, created.ID, created.CurrentRevision.ID)
	approveProposal(t, f, w, reviewer, proposal)
	r := publishProposal(t, f, w, publisher, proposal)
	if r.Code != 201 {
		t.Fatalf("publish trusted baseline: %d %s", r.Code, r.Body.String())
	}
	detail, err := f.store.GetCatalogAsset(context.Background(), w, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	return created.ID, detail.CurrentRevision.ID, detail.CurrentRevision.Content
}

func productionPayloadInput(t *testing.T, body string, fixtureInput map[string]any, owner identity.PrincipalID) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	bytes, err := json.Marshal(fixtureInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(bytes, &input); err != nil {
		t.Fatal(err)
	}
	if selections, ok := payload["input"].(map[string]any)["candidates"].([]any); ok && len(selections) > 0 {
		pins := input["candidates"].([]any)
		if len(pins) != len(selections) {
			t.Fatal("candidate fixture shape mismatch")
		}
		for i, selection := range selections {
			pin := pins[i].(map[string]any)
			value := selection.(map[string]any)
			pin["primaryTargetKey"] = value["primaryTargetKey"]
			pin["targetKeys"] = value["targetKeys"]
		}
	} else {
		input["candidates"] = []any{}
	}
	payload["input"] = input
	completeProductionTargets(payload, owner)
	bytes, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}

func completeProductionTargets(payload map[string]any, owner identity.PrincipalID) {
	for _, value := range payload["targets"].([]any) {
		target := value.(map[string]any)
		if target["kind"] == "semantic_asset" {
			content := target["content"].(map[string]any)
			content["scope"] = "Synthetic production fixture scope"
			content["ownerPrincipalId"] = owner.String()
			if target["intent"] == "create" {
				target["identityKey"] = content["address"]
			}
		}
		if _, exists := target["changes"]; !exists {
			target["changes"] = []any{}
		}
		if _, exists := target["evidenceIds"]; !exists {
			target["evidenceIds"] = []any{}
		}
		if target["kind"] == "semantic_asset" {
			if input, ok := payload["input"].(map[string]any); ok {
				if evidence, ok := input["evidence"].([]any); ok && len(evidence) > 0 {
					target["evidenceIds"] = []any{evidence[0].(map[string]any)["evidenceId"]}
				}
			}
		}
	}
}

// Positive release fixtures explicitly perform the same human HTTP confirmation
// as a client. Negative validation fixtures never call this helper.
func confirmProductionFixtureRules(t *testing.T, f *fixture, h http.Handler, w identity.WorkspaceID, p identity.PrincipalID, created map[string]any) {
	t.Helper()
	op := mustParseProductionOperationID(t, created["operationId"].(string))
	_, ver, targets, _, _, err := f.store.GetProductionOperation(context.Background(), w, op)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if !governance.RequiresProductionBusinessRule(target) {
			continue
		}
		if target.Declaration == nil || len(target.Declaration.EvidenceIDs) == 0 {
			t.Fatalf("positive fixture %s has no declared business evidence", target.LocalKey)
		}
		productionRequestResult(t, h, http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s/business-rule-confirmations", w, op), p, fmt.Sprintf("confirm-fixture-%s-%d-%s", op, ver.Version, target.LocalKey), map[string]any{"expectedVersion": ver.Version, "setDigest": ver.SetDigest, "targetKey": target.LocalKey, "action": "confirm", "evidenceId": target.Declaration.EvidenceIDs[0]}, http.StatusCreated)
	}
}

func completeProductionPayload(t *testing.T, body string, owner identity.PrincipalID) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	completeProductionTargets(payload, owner)
	bytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}
