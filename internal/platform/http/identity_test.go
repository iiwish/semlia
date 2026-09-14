package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application"
	identityapp "github.com/iiwish/semlia/internal/application/identity"
	systemdomain "github.com/iiwish/semlia/internal/domain"
	identitydomain "github.com/iiwish/semlia/internal/domain/identity"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	publicid "github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/sdk/trace"
)

type identityServiceStub struct {
	authenticated identityapp.AuthenticatedSession
	authErr       error
	csrf          string
	logoutCalls   int
	completeErr   error
}

func (*identityServiceStub) BeginLogin(context.Context, string) (identityapp.LoginStart, error) {
	return identityapp.LoginStart{}, nil
}

func (stub *identityServiceStub) CompleteLogin(context.Context, string, string) (identityapp.LoginComplete, error) {
	return identityapp.LoginComplete{}, stub.completeErr
}

func (stub *identityServiceStub) Authenticate(context.Context, string) (identityapp.AuthenticatedSession, error) {
	return stub.authenticated, stub.authErr
}

func (stub *identityServiceStub) ValidateCSRF(_ identityapp.AuthenticatedSession, presented string) bool {
	return presented == stub.csrf
}

func (stub *identityServiceStub) Logout(context.Context, string) error {
	stub.logoutCalls++
	return nil
}

func (*identityServiceStub) ListMembers(context.Context, publicid.WorkspaceID, string, string) ([]identitydomain.Membership, error) {
	return nil, nil
}

func (*identityServiceStub) ListInvitations(context.Context, publicid.WorkspaceID, string, string) ([]identitydomain.Invitation, error) {
	return nil, nil
}

func (*identityServiceStub) Invite(context.Context, publicid.WorkspaceID, publicid.PrincipalID, identityapp.InvitationInput, string) (identityapp.IssuedInvitation, error) {
	return identityapp.IssuedInvitation{}, nil
}

func (*identityServiceStub) SetMembershipStatus(context.Context, publicid.WorkspaceID, publicid.PrincipalID, publicid.MembershipID, identitydomain.MembershipStatus, string) (identitydomain.Membership, error) {
	return identitydomain.Membership{}, nil
}

func TestSessionModeRejectsPrincipalHeaderWithoutCookie(t *testing.T) {
	handler := newIdentityHandler(t, &identityServiceStub{}, []string{"https://app.example.com"})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	request.Header.Set("X-Semlia-Principal", "prn_spoofed")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	assertJSONField(t, response.Body.Bytes(), "code", "UNAUTHENTICATED")
}

func TestOIDCCallbackFailureRedirectsBrowserToSessionErrorState(t *testing.T) {
	handler := newIdentityHandler(t, &identityServiceStub{}, []string{"https://app.example.com"})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?error=access_denied", nil)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	location := response.Header().Get("Location")
	if !strings.Contains(location, "auth_error=provider_error") || !strings.Contains(location, "auth_error_description=") {
		t.Fatalf("location = %q", location)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
}

func TestOIDCCallbackFailureKeepsJSONAPIContract(t *testing.T) {
	handler := newIdentityHandler(t, &identityServiceStub{}, []string{"https://app.example.com"})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?error=access_denied", nil)
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	assertJSONField(t, response.Body.Bytes(), "code", "OIDC_TRANSACTION_INVALID")
}

func TestSessionResponseReturnsCSRFOnlyInHeader(t *testing.T) {
	stub := authenticatedIdentityStub(t)
	handler := newIdentityHandler(t, stub, []string{"https://app.example.com"})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	request.AddCookie(&http.Cookie{Name: "semlia_session_dev", Value: "opaque-session-token"})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("X-Semlia-CSRF"); got != stub.csrf {
		t.Fatalf("CSRF header = %q", got)
	}
	for _, secret := range []string{stub.csrf, "opaque-session-token"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("session body leaked secret %q: %s", secret, response.Body.String())
		}
	}
}

func TestUnsafeSessionRequestRequiresOriginAndCSRF(t *testing.T) {
	for _, test := range []struct {
		name, origin, csrf string
		want               int
	}{
		{name: "missing origin", csrf: "csrf-value", want: http.StatusForbidden},
		{name: "untrusted origin", origin: "https://evil.example", csrf: "csrf-value", want: http.StatusForbidden},
		{name: "missing csrf", origin: "https://app.example.com", want: http.StatusForbidden},
		{name: "valid", origin: "https://app.example.com", csrf: "csrf-value", want: http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := authenticatedIdentityStub(t)
			handler := newIdentityHandler(t, stub, []string{"https://app.example.com"})
			request := httptest.NewRequest(http.MethodDelete, "/api/v1/session", nil)
			request.AddCookie(&http.Cookie{Name: "semlia_session_dev", Value: "opaque-session-token"})
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.csrf != "" {
				request.Header.Set("X-Semlia-CSRF", test.csrf)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.want {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if test.want == http.StatusNoContent && stub.logoutCalls != 1 {
				t.Fatalf("logout calls = %d", stub.logoutCalls)
			}
		})
	}
}

func TestCrossWorkspaceMembershipIsForbidden(t *testing.T) {
	stub := authenticatedIdentityStub(t)
	other, err := publicid.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	handler := newIdentityHandler(t, stub, []string{"https://app.example.com"})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+other.String()+"/members", nil)
	request.AddCookie(&http.Cookie{Name: "semlia_session_dev", Value: "opaque-session-token"})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func authenticatedIdentityStub(t *testing.T) *identityServiceStub {
	t.Helper()
	account, _ := publicid.NewUserAccountID()
	session, _ := publicid.NewSessionID()
	membership, _ := publicid.NewMembershipID()
	workspace, _ := publicid.NewWorkspaceID()
	principal, _ := publicid.NewPrincipalID()
	return &identityServiceStub{csrf: "csrf-value", authenticated: identityapp.AuthenticatedSession{
		CSRFToken: "csrf-value",
		Session: identitydomain.Session{
			ID: session, Account: identitydomain.Account{ID: account, DisplayName: "Alpha User", Status: identitydomain.AccountActive},
			Memberships:       []identitydomain.Membership{{ID: membership, WorkspaceID: workspace, WorkspaceSlug: "alpha", WorkspaceDisplayName: "Alpha", AccountID: account, PrincipalID: principal, Status: identitydomain.MembershipActive, RoleIDs: []string{"workspace_admin"}, AuthorizationVersion: 3}},
			AbsoluteExpiresAt: time.Now().Add(time.Hour),
		},
	}}
}

func newIdentityHandler(t *testing.T, identity httpapi.IdentityService, origins []string) http.Handler {
	t.Helper()
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	system := application.NewSystemService(
		application.ReadinessProbeFunc(func(context.Context) error { return nil }),
		systemdomain.SystemInfo{APIVersion: "v1", SchemaVersion: "0.6.0", BuildVersion: "test"},
	)
	return httpapi.NewHandler(system, nil, provider.Tracer("identity-test"), httpapi.WithIdentity(identity, httpapi.IdentityHTTPConfig{AllowedOrigins: origins}))
}
