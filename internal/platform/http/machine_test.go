package httpapi

import (
	"context"
	"fmt"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/pkg/identity"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMachineRateLimitsCredentialWorkspaceAndExpiresStorage(t *testing.T) {
	now := time.Unix(120, 0)
	limiter := &machineRateLimiter{windows: map[string]rateWindow{}}
	for i := 0; i < 120; i++ {
		if !limiter.allow("one", "workspace", now) {
			t.Fatal("premature credential limit")
		}
	}
	if limiter.allow("one", "workspace", now) {
		t.Fatal("credential cap missing")
	}
	for credential := 1; credential < 10; credential++ {
		for i := 0; i < 120; i++ {
			if !limiter.allow(fmt.Sprint(credential), "workspace", now) {
				t.Fatal("premature workspace limit")
			}
		}
	}
	if limiter.allow("another", "workspace", now) {
		t.Fatal("workspace cap missing")
	}
	if !limiter.allow("one", "workspace", now.Add(time.Minute)) {
		t.Fatal("minute expiry did not reset limits")
	}
	for i := 0; i < 12000; i++ {
		limiter.allow(fmt.Sprint(i), fmt.Sprint(i), now.Add(time.Minute))
	}
	if len(limiter.windows) > 10002 {
		t.Fatal("unbounded limiter storage", len(limiter.windows))
	}
	if !limiter.allow("fresh", "fresh", now.Add(2*time.Minute)) || len(limiter.windows) != 2 {
		t.Fatal("expired identities retained")
	}
}

type machineAuthStub struct {
	MachineIdentityService
	limit authapp.CredentialLimit
}

func (s machineAuthStub) Authenticate(context.Context, string) (authapp.CredentialLimit, error) {
	return s.limit, nil
}

func TestMachineRateLimitHTTPReturns429RetryAfter(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	credential, _ := identity.NewClientCredentialID()
	principal, _ := identity.NewPrincipalID()
	handler := &Handler{machine: machineAuthStub{limit: authapp.CredentialLimit{WorkspaceID: workspace, CredentialID: credential, PrincipalID: principal}}, machineLimits: &machineRateLimiter{windows: map[string]rateWindow{"c:" + credential.String(): {minute: time.Now().Unix() / 60, count: 120}}}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+workspace.String()+"/semantic-search", nil)
	request.Header.Set("Authorization", "Bearer secret-not-logged")
	response := httptest.NewRecorder()
	_, code, ok := handler.authenticateRequest(response, request, "4bf92f3577b34da6a3ce929d0e0e4736", matchedRoute{kind: routeSemanticSearch, workspace: workspace.String()})
	if ok || code != "RATE_LIMITED" || response.Code != 429 || response.Header().Get("Retry-After") != "60" {
		t.Fatal("rate response", ok, code, response.Code, response.Header())
	}
	if strings.Contains(response.Body.String(), "secret-not-logged") {
		t.Fatal("rate error leaked bearer")
	}
}

func TestCredentialRotationRejectsUnsupportedGraceBody(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	credential, _ := identity.NewClientCredentialID()
	handler := &Handler{machine: machineAuthStub{}}
	request := httptest.NewRequest(http.MethodPost, "/rotate", strings.NewReader(`{"graceSeconds":60}`))
	response := httptest.NewRecorder()
	handler.routeMachine(response, request, "4bf92f3577b34da6a3ce929d0e0e4736", matchedRoute{kind: routeClientCredentialRotate, query: credential.String()}, workspace)
	if response.Code != 400 {
		t.Fatal("unsupported nonzero grace was silently accepted", response.Code)
	}
}
