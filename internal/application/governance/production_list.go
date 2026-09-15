package governance

import (
	"context"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func (s *ProductionService) ListOperationsPage(ctx context.Context, workspace identity.WorkspaceID, query domain.ProductionListQuery) ([]domain.ProductionOperation, error) {
	if workspace.IsZero() || query.Limit < 1 || query.Limit > 201 {
		return nil, domain.ErrInvalidArgument
	}
	return s.repo.ListProductionOperationsPage(ctx, workspace, query)
}
