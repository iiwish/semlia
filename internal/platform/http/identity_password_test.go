package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identityapp "github.com/iiwish/semlia/internal/application/identity"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	publicid "github.com/iiwish/semlia/pkg/identity"
)

type passwordHTTPStub struct {
	identityServiceStub
	calls   int
	source  string
	err     error
	changes int
}

func (s *passwordHTTPStub) PasswordLogin(_ context.Context, _, _, source string) (identityapp.LoginComplete, error) {
	s.calls++
	s.source = source
	return identityapp.LoginComplete{SessionToken: "new-opaque-session", ExpiresAt: time.Now().Add(time.Hour)}, s.err
}
func (s *passwordHTTPStub) ChangePassword(context.Context, publicid.UserAccountID, string, string, string) error {
	s.changes++
	return nil
}
func (*passwordHTTPStub) CreatePasswordMember(context.Context, publicid.WorkspaceID, publicid.PrincipalID, string, string, string, string, string) (domain.Account, error) {
	return domain.Account{}, nil
}

func TestPasswordLoginOriginAndSessionCookie(t *testing.T) {
	for _, origin := range []string{"", "https://evil.example", "https://app.example.com"} {
		t.Run(origin, func(t *testing.T) {
			stub := &passwordHTTPStub{}
			h := newIdentityHandler(t, stub, []string{"https://app.example.com"})
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader(`{"username":"user@example.com","password":"a sufficiently long password"}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", origin)
			r.Header.Set("X-Forwarded-For", "attacker-controlled")
			r.RemoteAddr = "127.0.0.1:12345"
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if origin != "https://app.example.com" {
				if w.Code != 403 || stub.calls != 0 {
					t.Fatalf("untrusted origin: %d, calls %d", w.Code, stub.calls)
				}
				return
			}
			if w.Code != 204 || stub.calls != 1 || stub.source != "127.0.0.1" {
				t.Fatalf("login: %d, calls %d, source %q", w.Code, stub.calls, stub.source)
			}
			cookies := w.Result().Cookies()
			if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Value != "new-opaque-session" {
				t.Fatal("missing secure session attributes")
			}
		})
	}
}

func TestPasswordLoginRateLimitDoesNotSetCookie(t *testing.T) {
	stub := &passwordHTTPStub{err: domain.ErrRateLimited}
	h := newIdentityHandler(t, stub, []string{"https://app.example.com"})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader(`{"username":"user@example.com","password":"a sufficiently long password"}`))
	r.Header.Set("Origin", "https://app.example.com")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" || len(w.Result().Cookies()) != 0 {
		t.Fatalf("rate limit response %d", w.Code)
	}
}

func TestPasswordChangeRequiresOriginAndCSRF(t *testing.T) {
	for _, test := range []struct {
		name, origin, csrf string
		want               int
	}{
		{"missing origin", "", "csrf-value", 403},
		{"untrusted origin", "https://evil.example", "csrf-value", 403},
		{"missing csrf", "https://app.example.com", "", 403},
		{"wrong csrf", "https://app.example.com", "wrong", 403},
		{"valid", "https://app.example.com", "csrf-value", 204},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &passwordHTTPStub{identityServiceStub: *authenticatedIdentityStub(t)}
			h := newIdentityHandler(t, stub, []string{"https://app.example.com"})
			r := httptest.NewRequest(http.MethodPost, "/api/v1/session/password", strings.NewReader(`{"currentPassword":"current long password","newPassword":"replacement long password"}`))
			r.AddCookie(&http.Cookie{Name: "semlia_session_dev", Value: "opaque-session-token"})
			r.Header.Set("Origin", test.origin)
			r.Header.Set("X-Semlia-CSRF", test.csrf)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != test.want {
				t.Fatalf("status = %d, want %d", w.Code, test.want)
			}
			if test.want != 204 && stub.changes != 0 {
				t.Fatal("untrusted request changed password")
			}
			if test.want == 204 {
				cookies := w.Result().Cookies()
				if stub.changes != 1 || len(cookies) != 1 || cookies[0].MaxAge >= 0 || cookies[0].Value != "" {
					t.Fatal("password change must clear session cookie")
				}
			}
		})
	}
}

func TestPasswordLoginRejectsMalformedAndOversizedBodies(t *testing.T) {
	for _, test := range []struct{ name, contentType, body string }{
		{"form", "application/x-www-form-urlencoded", "username=user@example.com&password=test"},
		{"invalid json", "application/json", "{"},
		{"oversized", "application/json", `{"username":"user@example.com","password":"` + strings.Repeat("x", 8192) + `"}`},
		{"trailing json", "application/json", `{} {}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &passwordHTTPStub{}
			h := newIdentityHandler(t, stub, []string{"https://app.example.com"})
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader(test.body))
			r.Header.Set("Origin", "https://app.example.com")
			r.Header.Set("Content-Type", test.contentType)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 400 || stub.calls != 0 || len(w.Result().Cookies()) != 0 {
				t.Fatalf("invalid body reached login: status %d, calls %d", w.Code, stub.calls)
			}
		})
	}
}
