package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/iiwish/semlia/internal/application"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	rootdomain "github.com/iiwish/semlia/internal/domain"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/trace/noop"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type scopeBoundaryRepository struct {
	authapp.Repository
	authapp.AdminRepository
	workspace identity.WorkspaceID
	captured  []authapp.RoleBindingMutation
}

func (r *scopeBoundaryRepository) LoadPrincipal(_ context.Context, w identity.WorkspaceID, p identity.PrincipalID) (auth.Principal, error) {
	return auth.Principal{ID: p, WorkspaceID: w, Kind: auth.PrincipalHuman, Status: auth.PrincipalActive}, nil
}
func (r *scopeBoundaryRepository) LoadPrincipalBindings(context.Context, identity.PrincipalID) ([]auth.RoleBinding, error) {
	return []auth.RoleBinding{{WorkspaceID: r.workspace, RoleID: "workspace_admin", RoleVersion: 1, ScopeType: auth.ScopeWorkspace, ScopeID: r.workspace.UUID(), GrantedAt: time.Now().Add(-time.Hour), Actions: []auth.Action{auth.ActionRoleAssign, auth.ActionAssetRead, auth.ActionSemanticResolve}}}, nil
}
func (r *scopeBoundaryRepository) AuthorizationVersion(context.Context, identity.WorkspaceID) (int64, error) {
	return 1, nil
}
func (r *scopeBoundaryRepository) RecordDecision(context.Context, auth.DecisionEvent) error {
	return nil
}
func (r *scopeBoundaryRepository) GetAuthorizationRole(context.Context, identity.WorkspaceID, string) (auth.Role, error) {
	return auth.Role{ID: "consumer_developer", Version: 1, Actions: []auth.Action{auth.ActionAssetRead, auth.ActionSemanticResolve}}, nil
}
func (r *scopeBoundaryRepository) AuthorizationScopeExists(_ context.Context, w identity.WorkspaceID, resource auth.Resource) (bool, error) {
	return resource.Type == auth.ScopeWorkspace && resource.ID == w.UUID(), nil
}
func (r *scopeBoundaryRepository) ListAuthorizationRoleBindings(context.Context, identity.WorkspaceID) ([]auth.RoleBinding, error) {
	return nil, nil
}
func (r *scopeBoundaryRepository) CreateManagedRoleBinding(_ context.Context, mutation authapp.RoleBindingMutation) (auth.RoleBinding, int64, error) {
	r.captured = append(r.captured, mutation)
	return mutation.Binding, 2, nil
}

func TestRoleBindingHTTPAcceptsPublicWorkspaceScopeAndLegacyUUID(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	other, _ := identity.NewWorkspaceID()
	asset, _ := identity.NewAssetID()
	for _, test := range []struct {
		name, scope string
		status      int
	}{
		{"public-TypeID", workspace.String(), 201}, {"legacy-UUID", workspace.UUID(), 201},
		{"foreign-TypeID", other.String(), 400}, {"foreign-UUID", other.UUID(), 400}, {"wrong-prefix", asset.String(), 400}, {"malformed", "wsp_invalid", 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &scopeBoundaryRepository{workspace: workspace}
			authorizer := authapp.NewService(repo, authapp.ClockFunc(time.Now))
			handler := NewHandler(application.NewSystemService(nil, rootdomain.SystemInfo{}), slog.New(slog.NewTextHandler(io.Discard, nil)), noop.NewTracerProvider().Tracer("scope-test"), WithAuthorization(authorizer))
			payload, _ := json.Marshal(map[string]any{"principalId": principal.String(), "roleId": "consumer_developer", "expectedRoleVersion": 1, "scope": map[string]string{"type": "workspace", "id": test.scope}})
			request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspace.String()+"/authorization/role-bindings", bytes.NewReader(payload))
			request.Header.Set("X-Semlia-Principal", principal.String())
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
			if test.status == 201 && (len(repo.captured) != 1 || repo.captured[0].Binding.ScopeID != workspace.UUID()) {
				t.Fatal("HTTP boundary did not persist canonical UUID scope")
			}
			if test.status != 201 && len(repo.captured) != 0 {
				t.Fatal("invalid scope reached persistence")
			}
		})
	}
}
