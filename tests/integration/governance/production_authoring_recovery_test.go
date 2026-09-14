package governance_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/iiwish/semlia/pkg/identity"
)

func TestProductionAuthoringRecoveryCursorWatermarkAndFilters(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
	inputJSON, _ := json.Marshal(input)
	body := completeProductionPayload(t, authoringLifecycleBody(string(inputJSON)), p)
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	create := func(name string) string {
		t.Helper()
		r := sendProdRequest(h, http.MethodPost, path, p.String(), "recovery-"+name, strings.ReplaceAll(body, "revenue", name))
		if r.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", name, r.Code, r.Body.String())
		}
		return authoringLifecycleDecode(t, r.Body.Bytes())["operationId"].(string)
	}
	want := map[string]bool{}
	for _, name := range []string{"first", "second", "third"} {
		want[create(name)] = true
	}
	source := input["snapshots"].([]any)[0].(map[string]any)["sourceId"].(string)
	filter := "?limit=1&sourceId=" + source + "&createdBy=" + p.String()
	page := sendProdRequest(h, http.MethodGet, path+filter, p.String(), "", "")
	if page.Code != http.StatusOK {
		t.Fatalf("first page: %d %s", page.Code, page.Body.String())
	}
	decoded := authoringLifecycleDecode(t, page.Body.Bytes())
	cursor := decoded["nextCursor"].(string)
	first := decoded["items"].([]any)[0].(map[string]any)["id"].(string)
	delete(want, first)
	later := create("later")
	for remaining := 0; cursor != ""; remaining++ {
		if remaining > 3 {
			t.Fatal("cursor did not terminate")
		}
		page = sendProdRequest(h, http.MethodGet, path+filter+"&cursor="+url.QueryEscape(cursor), p.String(), "", "")
		if page.Code != http.StatusOK {
			t.Fatalf("next page: %d %s", page.Code, page.Body.String())
		}
		decoded = authoringLifecycleDecode(t, page.Body.Bytes())
		for _, item := range decoded["items"].([]any) {
			id := item.(map[string]any)["id"].(string)
			if !want[id] || id == later {
				t.Fatalf("repeated or post-watermark operation: %s", id)
			}
			delete(want, id)
		}
		cursor, _ = decoded["nextCursor"].(string)
	}
	if len(want) != 0 {
		t.Fatalf("missing operations: %v", want)
	}
	initial := authoringLifecycleDecode(t, sendProdRequest(h, http.MethodGet, path+filter, p.String(), "", "").Body.Bytes())
	cursor = initial["nextCursor"].(string)
	for _, query := range []string{"?limit=1&cursor=" + url.QueryEscape(cursor), filter + "&cursor=" + url.QueryEscape("X"+cursor)} {
		r := sendProdRequest(h, http.MethodGet, path+query, p.String(), "", "")
		if r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "INVALID_CURSOR") {
			t.Fatalf("changed filter or forged cursor accepted: %d %s", r.Code, r.Body.String())
		}
	}
	other := mustID(t, identity.NewSourceConnectionID)
	r := sendProdRequest(h, http.MethodGet, path+"?sourceId="+other.String(), p.String(), "", "")
	if r.Code != http.StatusOK || len(authoringLifecycleDecode(t, r.Body.Bytes())["items"].([]any)) != 0 {
		t.Fatalf("source filter ignored: %d %s", r.Code, r.Body.String())
	}
}
