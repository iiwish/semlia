package governance

import (
	"context"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type productionActionRepository interface {
	CheckProductionAction(context.Context, domain.ProductionVersion, []domain.ProductionTarget, string) error
}

func (s *ProductionService) checkProductionAction(ctx context.Context, principal identity.PrincipalID, ver domain.ProductionVersion, targets []domain.ProductionTarget, command string) error {
	repo, ok := s.repo.(productionActionRepository)
	if !ok {
		return domain.ErrInvariant
	}
	ver.CreatedBy = principal
	return repo.CheckProductionAction(ctx, ver, targets, command)
}
