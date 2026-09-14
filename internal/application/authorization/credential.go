package authorization

import (
	"context"
	domain "github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
)

// CredentialLimit is an upper bound, never a grant. It is installed only after
// server-side credential verification; every operation still evaluates grants.
type CredentialLimit struct {
	WorkspaceID  identity.WorkspaceID
	PrincipalID  identity.PrincipalID
	ConsumerID   identity.ConsumerID
	BindingID    identity.ConsumerBindingID
	CredentialID identity.ClientCredentialID
	Actions      []domain.Action
	ScopeType    domain.ScopeType
	ScopeID      string
}
type credentialLimitKey struct{}

func WithCredentialLimit(ctx context.Context, limit CredentialLimit) context.Context {
	return context.WithValue(ctx, credentialLimitKey{}, limit)
}
func CredentialFromContext(ctx context.Context) (CredentialLimit, bool) {
	value, ok := ctx.Value(credentialLimitKey{}).(CredentialLimit)
	return value, ok
}

func (limit CredentialLimit) Allows(request EvaluationRequest) bool {
	if request.WorkspaceID != limit.WorkspaceID || request.PrincipalRef != limit.PrincipalID.String() {
		return false
	}
	found := false
	for _, action := range limit.Actions {
		if request.Action == action {
			found = true
		}
	}
	if !found {
		return false
	}
	if limit.ScopeType == domain.ScopeWorkspace {
		return limit.ScopeID == limit.WorkspaceID.String()
	}
	// Resolve is a request gate. Its selected release and every returned asset
	// are separately checked by the canonical distribution service.
	if (request.Action == domain.ActionSemanticResolve || request.Action == domain.ActionSemanticExecute) && request.Resource.Type == domain.ScopeWorkspace {
		return true
	}
	// The execution service checks every selected asset and the exact release
	// separately; source access still requires a current principal grant.
	if request.Action == domain.ActionSemanticExecute && request.Resource.Type == domain.ScopeSource {
		return true
	}
	if limit.ScopeType == domain.ScopeRelease {
		return request.Action == domain.ActionAssetRead || request.Action == domain.ActionSemanticExecute
	}
	parsed, err := identity.ParseAny(limit.ScopeID)
	return err == nil && request.Resource.Type == limit.ScopeType && request.Resource.ID == parsed.UUID()
}
