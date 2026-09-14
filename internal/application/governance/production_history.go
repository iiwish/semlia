package governance

import (
	"context"
	"encoding/json"
	"fmt"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type productionHistoryRepository interface {
	GetProductionOperationVersion(context.Context, identity.WorkspaceID, identity.ProductionOperationID, int) (domain.ProductionOperation, domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, []domain.ProductionContributor, error)
}

func (s *ProductionService) checkAuthoringInput(ctx context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID, input json.RawMessage, declarations []domain.TargetDeclaration) error {
	targets := make([]domain.ProductionTarget, 0, len(declarations))
	for _, decl := range declarations {
		targetID := ""
		if decl.TargetID != nil {
			targetID = *decl.TargetID
		}
		targets = append(targets, domain.ProductionTarget{LocalKey: decl.LocalKey, Kind: decl.Kind, Intent: decl.Intent, TargetID: targetID, IdentityKey: decl.IdentityKey, ContentJSON: decl.Content, Declaration: &decl})
	}
	return s.repo.CheckProductionInput(ctx, domain.ProductionVersion{WorkspaceID: workspace, CreatedBy: principal, InputJSON: input}, targets)
}

func (s *ProductionService) AuthorizeOperationRead(ctx context.Context, principal identity.PrincipalID, version domain.ProductionVersion, targets []domain.ProductionTarget) error {
	version.CreatedBy = principal
	return s.repo.CheckProductionInput(ctx, version, targets)
}

func (s *ProductionService) alreadyProduced(ctx context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID, operation identity.ProductionOperationID) error {
	_, version, targets, _, _, err := s.GetOperation(ctx, workspace, operation)
	if err != nil {
		return err
	}
	if err := s.AuthorizeOperationRead(ctx, principal, *version, targets); err != nil {
		return err
	}
	return &domain.AlreadyProducedError{OperationID: operation}
}

func (s *ProductionService) GetOperationVersion(ctx context.Context, workspace identity.WorkspaceID, opID identity.ProductionOperationID, version int) (*domain.ProductionOperation, *domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, []domain.ProductionContributor, error) {
	if workspace.IsZero() || opID.IsZero() || version < 1 {
		return nil, nil, nil, nil, nil, fmt.Errorf("%w: workspace, operation and positive version required", domain.ErrInvalidArgument)
	}
	repo, ok := s.repo.(productionHistoryRepository)
	if !ok {
		return nil, nil, nil, nil, nil, fmt.Errorf("%w: production history unavailable", domain.ErrNotFound)
	}
	op, ver, targets, links, contributors, err := repo.GetProductionOperationVersion(ctx, workspace, opID, version)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return &op, &ver, targets, links, contributors, nil
}
