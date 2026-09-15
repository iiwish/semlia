package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	authapp "github.com/iiwish/semlia/internal/application/authorization"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestProductionAuthoringScopedGrantsAndOriginalReplayRevocation(t *testing.T) {
	f, h, w, admin := authoringLifecycleSetup(t)
	firstInput := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
	secondInput := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
	p := mustID(t, identity.NewPrincipalID)
	if _, err := f.store.CreatePrincipal(context.Background(), authz.Principal{ID: p, WorkspaceID: w, Kind: authz.PrincipalHuman, DisplayName: "Scoped author", Status: authz.PrincipalActive}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CreateRoleBinding(context.Background(), authz.RoleBinding{ID: mustID(t, identity.NewBindingID), PrincipalID: p, RoleID: "asset_owner", ScopeType: authz.ScopeDomain, ScopeID: "finance"}); err != nil {
		t.Fatal(err)
	}
	var firstBinding authz.RoleBinding
	for i, input := range []map[string]any{firstInput, secondInput} {
		source, _ := identity.ParseSourceConnectionID(input["snapshots"].([]any)[0].(map[string]any)["sourceId"].(string))
		binding, err := f.store.CreateRoleBinding(context.Background(), authz.RoleBinding{ID: mustID(t, identity.NewBindingID), PrincipalID: p, RoleID: "source_operator", ScopeType: authz.ScopeSource, ScopeID: source.UUID()})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstBinding = binding
		}
	}
	inputJSON, _ := json.Marshal(firstInput)
	firstBody := completeProductionPayload(t, authoringLifecycleBody(string(inputJSON)), p)
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	r := sendProdRequest(h, http.MethodPost, path, p.String(), "scope-create", firstBody)
	if r.Code != http.StatusCreated {
		t.Fatalf("scoped create: %d %s", r.Code, r.Body.String())
	}
	op := authoringLifecycleDecode(t, r.Body.Bytes())["operationId"].(string)
	inputJSON, _ = json.Marshal(secondInput)
	secondBody := completeProductionPayload(t, authoringLifecycleBody(string(inputJSON)), p)
	secondBody = strings.Replace(secondBody, `"input":`, `"expectedVersion":1,"input":`, 1)
	r = sendProdRequest(h, http.MethodPut, path+"/"+op, p.String(), "scope-replace", secondBody)
	if r.Code != http.StatusOK {
		t.Fatalf("scoped replace: %d %s", r.Code, r.Body.String())
	}
	service := authapp.NewService(f.store, authapp.ClockFunc(time.Now))
	if _, _, err := service.RevokeBinding(context.Background(), authapp.RevokeRoleBindingRequest{AccessRequest: authapp.AccessRequest{WorkspaceID: w, PrincipalRef: admin.String(), TraceID: traceID}, BindingID: firstBinding.ID, ExpectedVersion: firstBinding.Version, Reason: "Revoke old source"}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path, key, body string
		status                  int
	}{
		{http.MethodGet, path + "/" + op, "", "", http.StatusOK},
		{http.MethodGet, path + "/" + op + "?version=1", "", "", http.StatusForbidden},
		{http.MethodPost, path, "scope-create", firstBody, http.StatusForbidden},
		{http.MethodPut, path + "/" + op, "scope-replace", secondBody, http.StatusOK},
	} {
		r := sendProdRequest(h, test.method, test.path, p.String(), test.key, test.body)
		if r.Code != test.status {
			t.Errorf("%s %s: %d %s", test.method, test.path, r.Code, r.Body.String())
		}
	}
}
