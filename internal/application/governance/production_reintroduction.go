package governance

import (
	"context"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type productionReintroductionRepository interface {
	PrepareProductionBaselineForOperation(context.Context, identity.WorkspaceID, identity.PrincipalID, identity.ProductionOperationID, []domain.TargetDeclaration) (domain.ProductionBaseline, error)
}

func (s *ProductionService) prepareProductionReplacementBaseline(ctx context.Context, w identity.WorkspaceID, p identity.PrincipalID, op identity.ProductionOperationID, targets []domain.TargetDeclaration) (domain.ProductionBaseline, error) {
	if repo, ok := s.repo.(productionReintroductionRepository); ok {
		return repo.PrepareProductionBaselineForOperation(ctx, w, p, op, targets)
	}
	return s.repo.PrepareProductionBaseline(ctx, w, p, targets)
}
