package governance

import (
	"context"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type productionValidationHistoryRepository interface {
	ProductionValidationHistory(context.Context, identity.WorkspaceID, identity.PrincipalID, identity.ProductionOperationID, int, int, int, int) ([]domain.ProductionVersion, int, bool, error)
}

func (s *ProductionService) GetProductionValidationHistory(ctx context.Context, w identity.WorkspaceID, p identity.PrincipalID, op identity.ProductionOperationID, version, limit, upper, after int) ([]domain.ProductionVersion, int, bool, error) {
	if w.IsZero() || p.IsZero() || op.IsZero() || version < 1 || limit < 1 || limit > 200 || upper < 0 || upper > 256 || after < 0 || after > 256 {
		return nil, 0, false, domain.ErrInvalidArgument
	}
	repo, ok := s.repo.(productionValidationHistoryRepository)
	if !ok {
		return nil, 0, false, domain.ErrInvariant
	}
	return repo.ProductionValidationHistory(ctx, w, p, op, version, limit, upper, after)
}
