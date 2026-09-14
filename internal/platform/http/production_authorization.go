package httpapi

import (
	"context"

	authz "github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
)

// The route checks active membership; the repository authorizes every concrete
// target/source under its workspace fence, including scoped rather than broad grants.
func (handler *Handler) checkProductionAuthoringAccess(ctx context.Context, actor string, workspace identity.WorkspaceID) error {
	denied := &authz.DenialError{Decision: authz.Decision{ReasonCode: authz.ReasonNoMatchingGrant}}
	if handler.authorization == nil {
		return denied
	}
	principal, err := identity.ParsePrincipalID(actor)
	if err != nil {
		return denied
	}
	snapshot, err := handler.authorization.Snapshot(ctx, workspace, principal)
	if err != nil {
		return err
	}
	for _, grant := range snapshot.ActiveGrants() {
		if grant.Action == authz.ActionAssetRead || grant.Action == authz.ActionBindingRead {
			return nil
		}
	}
	return denied
}
