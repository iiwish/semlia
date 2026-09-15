package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/iiwish/semlia/pkg/identity"
)

func TestProductionAuthoringPublishedBaselineUpdateAndNoChange(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	asset, revision, base := productionPublishedAsset(t, f, w, p)
	input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	var content map[string]any
	if err := json.Unmarshal(base, &content); err != nil {
		t.Fatal(err)
	}
	target := map[string]any{"intent": "update", "kind": "semantic_asset", "localKey": "revenue", "targetId": asset.String(), "baseRevisionId": revision.String(), "title": "Revenue update", "content": content, "changes": []any{}, "evidenceIds": []any{}}
	noChange, _ := json.Marshal(map[string]any{"input": input, "targets": []any{target}})
	r := sendProdRequest(h, http.MethodPost, path, p.String(), "baseline-no-change", string(noChange))
	if r.Code != http.StatusOK {
		t.Fatalf("no change: %d %s", r.Code, r.Body.String())
	}
	result := authoringLifecycleDecode(t, r.Body.Bytes())
	if result["outcome"] != "no_change" {
		t.Fatalf("no-change outcome: %s", r.Body.String())
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM proposals WHERE workspace_id=$1 AND production_operation_id IS NOT NULL`, w.UUID()).Scan(&count); err != nil || count != 0 {
		t.Fatalf("no_change created proposal: %d %v", count, err)
	}
	before := content["definition"]
	content["definition"] = "Revenue after adjustments"
	target["changes"] = []any{map[string]any{"fieldPath": "definition", "op": "update", "beforeValue": "forged baseline", "afterValue": content["definition"]}}
	invalid, _ := json.Marshal(map[string]any{"input": input, "targets": []any{target}})
	r = sendProdRequest(h, http.MethodPost, path, p.String(), "baseline-forged-before", string(invalid))
	if r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("forged beforeValue: %d %s", r.Code, r.Body.String())
	}
	target["changes"] = []any{map[string]any{"fieldPath": "definition", "op": "update", "beforeValue": before, "afterValue": content["definition"]}}
	valid, _ := json.Marshal(map[string]any{"input": input, "targets": []any{target}})
	r = sendProdRequest(h, http.MethodPost, path, p.String(), "baseline-real-update", string(valid))
	if r.Code != http.StatusCreated {
		t.Fatalf("real published update: %d %s", r.Code, r.Body.String())
	}
	result = authoringLifecycleDecode(t, r.Body.Bytes())
	proposal := mustParseProposalID(t, result["proposalIds"].([]any)[0].(string))
	var baseID, intent, op string
	if err := f.pool.QueryRow(context.Background(), `SELECT base_revision_id::text,intent,production_operation_id::text FROM proposals WHERE workspace_id=$1 AND id=$2`, w.UUID(), proposal.UUID()).Scan(&baseID, &intent, &op); err != nil || baseID != revision.UUID() || intent != "update" {
		t.Fatalf("proposal baseline: %s %s %v", baseID, intent, err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM proposal_changes WHERE workspace_id=$1 AND proposal_id=$2 AND before_digest IS NOT NULL AND after_digest IS NOT NULL`, w.UUID(), proposal.UUID()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("real changes missing: %d %v", count, err)
	}
	replay := sendProdRequest(h, http.MethodPost, path, p.String(), "baseline-real-update", string(valid))
	if replay.Code != http.StatusOK {
		t.Fatalf("update replay: %d %s", replay.Code, replay.Body.String())
	}
}
