package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestProductionConcurrentPublishHasOneHeadWinner(t *testing.T) {
	f, h, w, author := authoringLifecycleSetup(t)
	path, body, first := authoringLifecycleCreate(t, f, h, w, author)
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	target := payload["targets"].([]any)[0].(map[string]any)
	target["identityKey"] = "finance.other_revenue"
	target["content"].(map[string]any)["address"] = "finance.other_revenue"
	second := productionRequestResult(t, h, http.MethodPost, path, author, "second-create", payload, http.StatusCreated)
	reviewer := authoringLifecyclePrincipal(t, f, w, "Concurrent reviewer")
	publisher := authoringLifecyclePrincipal(t, f, w, "Concurrent publisher")
	operations := []map[string]any{first, second}
	requests := make([]string, 2)
	for i, created := range operations {
		digest := productionValidateAndApprove(t, f, h, w, author, reviewer, created)
		requests[i] = fmt.Sprintf(`{"expectedVersion":1,"setDigest":%q,"validation":{"attemptNo":1,"validationDigest":%q},"expectedHead":{"presence":"absent"}}`, created["setDigest"], digest)
	}
	start := make(chan struct{})
	type result struct {
		code int
		body string
	}
	results := make(chan result, 2)
	for i, created := range operations {
		go func(i int, op string) {
			<-start
			r := sendProdRequest(h, http.MethodPost, path+"/"+op+"/publish", publisher.String(), fmt.Sprintf("concurrent-publish-%d", i), requests[i])
			results <- result{r.Code, r.Body.String()}
		}(i, created["operationId"].(string))
	}
	close(start)
	winners, conflicts := 0, 0
	replies := []result{<-results, <-results}
	for _, r := range replies {
		switch r.code {
		case http.StatusCreated:
			winners++
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("unexpected concurrent publish: %d %s", r.code, r.body)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("head CAS winners=%d conflicts=%d", winners, conflicts)
	}
	var releases, assets, receipts int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM releases WHERE workspace_id=$1),(SELECT count(*) FROM semantic_assets WHERE workspace_id=$1 AND current_revision_id IS NOT NULL),(SELECT count(*) FROM production_commands WHERE workspace_id=$1 AND command_kind='publish')`, w.UUID()).Scan(&releases, &assets, &receipts); err != nil {
		t.Fatal(err)
	}
	if releases != 1 || assets != 1 || receipts != 1 {
		t.Fatalf("loser leaked writes: releases=%d assets=%d receipts=%d", releases, assets, receipts)
	}
}
