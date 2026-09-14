package governance

import (
	"context"
	"strings"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type productionBusinessRuleRepository interface {
	RecordProductionBusinessRule(context.Context, domain.ProductionBusinessRuleCommand) (domain.ProductionBusinessRuleEvent, error)
	ReadProductionBusinessRules(context.Context, identity.WorkspaceID, identity.PrincipalID, identity.ProductionOperationID, int) ([]domain.ProductionBusinessRuleWitness, error)
}

func (s *ProductionService) RecordBusinessRule(ctx context.Context, cmd domain.ProductionBusinessRuleCommand) (domain.ProductionBusinessRuleEvent, error) {
	empty := domain.ProductionBusinessRuleEvent{}
	if cmd.WorkspaceID.IsZero() || cmd.OperationID.IsZero() || cmd.PrincipalID.IsZero() || cmd.ExpectedVersion < 1 || !domain.IsValidContentDigest(cmd.SetDigest) || strings.TrimSpace(cmd.TargetKey) == "" || len(cmd.TargetKey) > 128 || strings.TrimSpace(cmd.IdempotencyKey) == "" || len(cmd.IdempotencyKey) > 256 {
		return empty, domain.ErrInvalidArgument
	}
	if cmd.Action == "confirm" {
		if cmd.Declaration != "" {
			if strings.TrimSpace(cmd.Declaration) == "" || len(cmd.Declaration) > 8192 || cmd.EvidenceID != "" {
				return empty, domain.ErrInvalidArgument
			}
		} else if _, err := identity.ParseEvidenceID(cmd.EvidenceID); err != nil {
			return empty, domain.ErrInvalidArgument
		}
	} else if cmd.Action != "revoke" || cmd.EvidenceID != "" || cmd.Declaration != "" {
		return empty, domain.ErrInvalidArgument
	}
	repo, ok := s.repo.(productionBusinessRuleRepository)
	if !ok {
		return empty, domain.ErrInvariant
	}
	return repo.RecordProductionBusinessRule(ctx, cmd)
}

func (s *ProductionService) BusinessRules(ctx context.Context, w identity.WorkspaceID, p identity.PrincipalID, op identity.ProductionOperationID, version int) ([]domain.ProductionBusinessRuleWitness, error) {
	if w.IsZero() || p.IsZero() || op.IsZero() || version < 1 {
		return nil, domain.ErrInvalidArgument
	}
	repo, ok := s.repo.(productionBusinessRuleRepository)
	if !ok {
		return nil, domain.ErrInvariant
	}
	return repo.ReadProductionBusinessRules(ctx, w, p, op, version)
}
