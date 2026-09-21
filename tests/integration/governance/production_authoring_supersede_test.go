package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/iiwish/semlia/pkg/identity"
)

func TestProductionAuthoringSupersedePreservesIdentityAndDecisions(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	candidate := mustID(t, identity.NewSemanticCandidateID)
	input := productionFixtureInput(t, f, w, candidate)
	selection := input["candidates"].([]any)[0].(map[string]any)
	selection["primaryTargetKey"] = "revenue"
	selection["targetKeys"] = []any{"revenue"}
	inputJSON, _ := json.Marshal(input)
	body := completeProductionPayload(t, authoringLifecycleBody(string(inputJSON)), p)
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	r := sendProdRequest(h, http.MethodPost, path, p.String(), "supersede-create", body)
	if r.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", r.Code, r.Body.String())
	}
	first := authoringLifecycleDecode(t, r.Body.Bytes())
	op := first["operationId"].(string)
	proposal := mustParseProposalID(t, first["proposalIds"].([]any)[0].(string))
	submit, _ := json.Marshal(map[string]any{"expectedVersion": 1, "setDigest": first["setDigest"]})
	r = sendProdRequest(h, http.MethodPost, path+"/"+op+"/submit", p.String(), "supersede-freeze", string(submit))
	if r.Code != http.StatusAccepted {
		t.Fatalf("freeze: %d %s", r.Code, r.Body.String())
	}
	var successor map[string]any
	if err := json.Unmarshal([]byte(body), &successor); err != nil {
		t.Fatal(err)
	}
	successor["supersedesOperationId"] = op
	successor["targets"].([]any)[0].(map[string]any)["content"].(map[string]any)["definition"] = "Corrected net revenue"
	raw, _ := json.Marshal(successor)
	r = sendProdRequest(h, http.MethodPost, path, p.String(), "supersede-successor", string(raw))
	if r.Code != http.StatusCreated {
		t.Fatalf("supersede: %d %s", r.Code, r.Body.String())
	}
	second := authoringLifecycleDecode(t, r.Body.Bytes())
	secondProposal := mustParseProposalID(t, second["proposalIds"].([]any)[0].(string))
	var oldTarget, newTarget, state, owner string
	if err := f.pool.QueryRow(context.Background(), `SELECT old.target_object_id::text,new.target_object_id::text,old.state,r.owner_operation_id::text FROM proposals old JOIN proposals new ON new.workspace_id=old.workspace_id AND new.id=$3 JOIN production_identity_reservations r ON r.workspace_id=old.workspace_id AND r.target_id=old.target_object_id WHERE old.workspace_id=$1 AND old.id=$2`, w.UUID(), proposal.UUID(), secondProposal.UUID()).Scan(&oldTarget, &newTarget, &state, &owner); err != nil {
		t.Fatal(err)
	}
	secondOp, _ := identity.ParseProductionOperationID(second["operationId"].(string))
	if oldTarget != newTarget || state != "rejected" || owner != secondOp.UUID() {
		t.Fatalf("wrong supersede transition: %s %s %s %s", oldTarget, newTarget, state, owner)
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM semantic_candidate_decisions WHERE workspace_id=$1 AND candidate_id=$2`, w.UUID(), candidate.UUID()).Scan(&count); err != nil || count != 2 {
		t.Fatalf("decision history: %d %v", count, err)
	}
	r = sendProdRequest(h, http.MethodPost, path, p.String(), "supersede-create", body)
	if r.Code != http.StatusOK || authoringLifecycleDecode(t, r.Body.Bytes())["operationId"] != op {
		t.Fatalf("old command replay: %d %s", r.Code, r.Body.String())
	}
	// The inbox must retain history without offering the replaced operation again.
	r = sendProdRequest(h, http.MethodGet, path, p.String(), "", "")
	if r.Code != http.StatusOK {
		t.Fatalf("inbox list: %d %s", r.Code, r.Body.String())
	}
	var page struct {
		Items []struct {
			ID          string   `json:"id"`
			Superseded  bool     `json:"superseded"`
			ProposalIDs []string `json:"proposalIds"`
		} `json:"items"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected both history and successor: %s", r.Body.String())
	}
	for _, item := range page.Items {
		if item.Superseded != (item.ID == op) || len(item.ProposalIDs) != 1 {
			t.Fatalf("incorrect inbox metadata: %s", r.Body.String())
		}
	}
}
