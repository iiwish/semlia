package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/iiwish/semlia/pkg/identity"
)

func TestProductionAuthoringCandidateDecisionsAndReplacement(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	candidate := mustID(t, identity.NewSemanticCandidateID)
	input := productionFixtureInput(t, f, w, candidate)
	selection := input["candidates"].([]any)[0].(map[string]any)
	selection["primaryTargetKey"] = "revenue"
	selection["targetKeys"] = []any{"revenue"}
	inputJSON, _ := json.Marshal(input)
	body := completeProductionPayload(t, authoringLifecycleBody(string(inputJSON)), p)
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	r := sendProdRequest(h, http.MethodPost, path, p.String(), "decision-create", body)
	if r.Code != http.StatusCreated {
		t.Fatalf("candidate create: %d %s", r.Code, r.Body.String())
	}
	created := authoringLifecycleDecode(t, r.Body.Bytes())
	proposal := mustParseProposalID(t, created["proposalIds"].([]any)[0].(string))
	var status, projection, decision, link string
	if err := f.pool.QueryRow(context.Background(), `SELECT c.status,COALESCE(c.proposal_id::text,''),COALESCE(d.id::text,''),COALESCE(l.decision_id::text,'')
	FROM semantic_candidates c LEFT JOIN semantic_candidate_decisions d ON d.workspace_id=c.workspace_id AND d.candidate_id=c.id
	LEFT JOIN production_candidate_links l ON l.workspace_id=c.workspace_id AND l.candidate_id=c.id AND l.is_primary
	WHERE c.workspace_id=$1 AND c.id=$2`, w.UUID(), candidate.UUID()).Scan(&status, &projection, &decision, &link); err != nil {
		t.Fatal(err)
	}
	if status != "converted" || projection != proposal.UUID() || decision == "" || decision != link {
		t.Fatalf("candidate conversion not atomically linked: %s %s %s %s", status, projection, decision, link)
	}
	replacement := strings.Replace(body, `"input":`, `"expectedVersion":1,"input":`, 1)
	r = sendProdRequest(h, http.MethodPut, path+"/"+created["operationId"].(string), p.String(), "decision-replace", replacement)
	if r.Code != http.StatusOK {
		t.Fatalf("replace converted candidate: %d %s", r.Code, r.Body.String())
	}
	var decisions, priorDrafts int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM semantic_candidate_decisions WHERE workspace_id=$1 AND candidate_id=$2`, w.UUID(), candidate.UUID()).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if decisions != 2 {
		t.Fatalf("replacement must retain old decision and append linked decision: %d", decisions)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM proposals WHERE workspace_id=$1 AND id=$2 AND state='draft'`, w.UUID(), proposal.UUID()).Scan(&priorDrafts); err != nil || priorDrafts != 0 {
		t.Fatalf("superseded draft not terminated: %d %v", priorDrafts, err)
	}
}

func TestProductionAuthoringConcurrentSameKeyReplays(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
	inputJSON, _ := json.Marshal(input)
	body := completeProductionPayload(t, authoringLifecycleBody(string(inputJSON)), p)
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	start := make(chan struct{})
	type outcome struct {
		code int
		body string
	}
	results := make(chan outcome, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r := sendProdRequest(h, http.MethodPost, path, p.String(), "concurrent-same-key", body)
			results <- outcome{r.Code, r.Body.String()}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	created := 0
	op := ""
	for result := range results {
		if result.code != http.StatusCreated && result.code != http.StatusOK {
			t.Errorf("concurrent request: %d %s", result.code, result.body)
			continue
		}
		if result.code == http.StatusCreated {
			created++
		}
		parsed := authoringLifecycleDecode(t, []byte(result.body))
		id := parsed["operationId"].(string)
		if op != "" && op != id {
			t.Errorf("different operations: %s %s", op, id)
		}
		op = id
	}
	if created != 1 {
		t.Errorf("created responses=%d", created)
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM production_operations WHERE workspace_id=$1`, w.UUID()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate operations: %d %v", count, err)
	}
}

func TestProductionAuthoringConcurrentDifferentKeysClaimOneIdentity(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
	inputJSON, _ := json.Marshal(input)
	body := completeProductionPayload(t, authoringLifecycleBody(string(inputJSON)), p)
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	type outcome struct {
		code int
		body string
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			<-start
			r := sendProdRequest(h, http.MethodPost, path, p.String(), fmt.Sprintf("different-key-%d", i), body)
			results <- outcome{r.Code, r.Body.String()}
		}(i)
	}
	close(start)
	var createdID, conflict string
	for i := 0; i < 2; i++ {
		r := <-results
		switch r.code {
		case http.StatusCreated:
			if createdID != "" {
				t.Fatal("both keys created an operation")
			}
			createdID = authoringLifecycleDecode(t, []byte(r.body))["operationId"].(string)
		case http.StatusConflict:
			conflict = r.body
		default:
			t.Errorf("unexpected response: %d %s", r.code, r.body)
		}
	}
	if createdID == "" || !strings.Contains(conflict, "ALREADY_PRODUCED") || !strings.Contains(conflict, createdID) {
		t.Fatalf("claim did not recover authorized existing identity: %s %s", createdID, conflict)
	}
	var operations, proposals, assets int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM production_operations WHERE workspace_id=$1),(SELECT count(*) FROM proposals WHERE workspace_id=$1),(SELECT count(*) FROM semantic_assets WHERE workspace_id=$1)`, w.UUID()).Scan(&operations, &proposals, &assets); err != nil || operations != 1 || proposals != 1 || assets != 1 {
		t.Fatalf("duplicate atomic result: %d %d %d %v", operations, proposals, assets, err)
	}
}
