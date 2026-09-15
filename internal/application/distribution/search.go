package distribution

import (
	"context"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/pkg/identity"
	"strings"
)

type SearchRequest struct {
	WorkspaceID                 identity.WorkspaceID
	PrincipalRef, TraceID, Text string
}

func (s *Service) Search(ctx context.Context, r SearchRequest) ([]DefinitionSummary, error) {
	if len(r.Text) > 240 {
		return nil, domain.ErrInvalidArgument
	}
	d, err := s.authorize(ctx, r.WorkspaceID, auth.ActionSemanticResolve, auth.Resource{Type: auth.ScopeWorkspace, ID: r.WorkspaceID.UUID()}, r.PrincipalRef, r.TraceID)
	if err != nil {
		return nil, err
	}
	request := ResolveRequest{WorkspaceID: r.WorkspaceID, PrincipalRef: r.PrincipalRef, TraceID: r.TraceID, Input: domain.SemanticQueryInput{Context: domain.ResolutionContext{Mode: domain.ResolutionCurrent}}}
	if limit, ok := authapp.CredentialFromContext(ctx); ok {
		request.Input.Context = domain.ResolutionContext{Mode: domain.ResolutionBinding, BindingID: &limit.BindingID}
	}
	snapshot, _, _, refusal, err := s.selectSnapshot(ctx, request, d, s.clock.Now().UTC())
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		return nil, credentialDenied()
	}
	if limit, ok := authapp.CredentialFromContext(ctx); ok && limit.ScopeType == auth.ScopeRelease && limit.ScopeID != snapshot.ReleaseID.String() {
		return nil, credentialDenied()
	}
	assets := []domain.ReleasedAsset{}
	needle := strings.ToLower(strings.TrimSpace(r.Text))
	for _, asset := range snapshot.Assets {
		if needle != "" && !strings.Contains(strings.ToLower(asset.Address+" "+asset.Name+" "+strings.Join(asset.Aliases, " ")), needle) {
			continue
		}
		allowed, err := s.assetAllowed(ctx, request, asset)
		if err != nil {
			return nil, err
		}
		if allowed {
			assets = append(assets, asset)
		}
		if len(assets) == 100 {
			break
		}
	}
	return definitionSummaries(assets), nil
}
