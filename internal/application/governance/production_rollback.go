package governance

import (
	"context"
	"encoding/json"
	"strings"

	domain "github.com/iiwish/semlia/internal/domain/governance"
)

type productionRollbackRepository interface {
	RollbackProductionCommand(context.Context, RollbackOperationCommand, string) (*ReleaseCommandResult, error)
}

func (s *ProductionService) rollbackProduction(ctx context.Context, cmd RollbackOperationCommand) (*ReleaseCommandResult, error) {
	if cmd.WorkspaceID.IsZero() || cmd.PrincipalID.IsZero() || cmd.ReleaseID.IsZero() || strings.TrimSpace(cmd.IdempotencyKey) == "" || cmd.ExpectedVersion < 1 || !domain.IsValidContentDigest(cmd.SetDigest) || strings.TrimSpace(cmd.Reason) == "" {
		return nil, domain.ErrInvalidArgument
	}
	if err := cmd.ExpectedHead.Validate(); err != nil {
		return nil, err
	}
	if cmd.ExpectedHead.Presence != domain.PresencePresent || *cmd.ExpectedHead.ReleaseID != cmd.ReleaseID {
		return nil, domain.ErrHeadConflict
	}
	body := cmd
	body.TraceID, body.IdempotencyKey = "", ""
	raw, err := json.Marshal(struct {
		Schema  string
		Command RollbackOperationCommand
	}{"production.rollback.v1", body})
	if err != nil {
		return nil, err
	}
	digest, err := domain.DigestJSON(raw)
	if err != nil {
		return nil, err
	}
	repo, ok := s.repo.(productionRollbackRepository)
	if !ok {
		return nil, domain.ErrInvariant
	}
	return repo.RollbackProductionCommand(ctx, cmd, digest)
}
