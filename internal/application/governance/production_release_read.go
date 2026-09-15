package governance

import (
	"context"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type productionReleaseReader interface {
	ProductionReleaseProjectionStatus(context.Context, identity.WorkspaceID, identity.ReleaseID) (string, error)
	AuthorizeProductionManifestRead(context.Context, identity.WorkspaceID, identity.ReleaseID, identity.PrincipalID) error
}

func (s *ProductionService) GetProductionReleaseForPrincipal(ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID, principal identity.PrincipalID) (*ProductionReleaseDetail, error) {
	detail, err := s.GetProductionRelease(ctx, workspace, release)
	if err != nil {
		return nil, err
	}
	if !detail.Release.IsProductionProtected() || len(detail.ReleaseProposals) == 0 {
		return nil, domain.ErrPriorStateUnknown
	}
	seen := map[string]bool{}
	for _, attr := range detail.ReleaseProposals {
		_, ver, targets, _, contributors, err := s.GetOperationVersion(ctx, workspace, attr.OperationID, attr.ProductionVersion)
		if err != nil {
			return nil, err
		}
		if err := s.AuthorizeOperationRead(ctx, principal, *ver, targets); err != nil {
			return nil, err
		}
		for _, contributor := range contributors {
			key := contributor.PrincipalID.String()
			if !seen[key] {
				seen[key] = true
				detail.Contributors = append(detail.Contributors, contributor)
			}
		}
	}
	repo, ok := s.repo.(productionReleaseReader)
	if !ok {
		return nil, domain.ErrInvariant
	}
	if err := repo.AuthorizeProductionManifestRead(ctx, workspace, release, principal); err != nil {
		return nil, err
	}
	detail.ProjectionStatus, err = repo.ProductionReleaseProjectionStatus(ctx, workspace, release)
	if err != nil {
		return nil, err
	}
	return detail, nil
}
