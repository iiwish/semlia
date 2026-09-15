package governance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	domain "github.com/iiwish/semlia/internal/domain/governance"
)

type productionPublishRepository interface {
	PublishProductionCommand(context.Context, PublishOperationCommand, string) (*ReleaseCommandResult, error)
}

func (s *ProductionService) publishProduction(ctx context.Context, cmd PublishOperationCommand) (*ReleaseCommandResult, error) {
	if cmd.WorkspaceID.IsZero() || cmd.PrincipalID.IsZero() || cmd.OperationID.IsZero() || strings.TrimSpace(cmd.IdempotencyKey) == "" || cmd.ExpectedVersion < 1 || cmd.ValidationAttemptNo < 1 || cmd.ValidationAttemptNo > 256 || !domain.IsValidContentDigest(cmd.SetDigest) || !domain.IsValidContentDigest(cmd.ValidationDigest) {
		return nil, domain.ErrInvalidArgument
	}
	if err := cmd.ExpectedHead.Validate(); err != nil {
		return nil, err
	}
	body := cmd
	body.TraceID = ""
	body.IdempotencyKey = ""
	raw, err := json.Marshal(struct {
		Schema  string
		Command PublishOperationCommand
	}{"production.publish.v1", body})
	if err != nil {
		return nil, err
	}
	digest, err := domain.DigestJSON(raw)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.GetProductionCommand(ctx, cmd.WorkspaceID, cmd.PrincipalID, domain.CommandPublish, cmd.IdempotencyKey)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if existing != nil && existing.RequestDigest != digest {
		return nil, domain.ErrIdempotencyConflict
	}
	_, ver, targets, _, _, err := s.GetOperationVersion(ctx, cmd.WorkspaceID, cmd.OperationID, cmd.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	if err := s.checkProductionAction(ctx, cmd.PrincipalID, *ver, targets, domain.CommandPublish); err != nil {
		return nil, err
	}
	if existing != nil {
		if len(existing.ResponseJSON) == 0 {
			return nil, domain.ErrPriorStateUnknown
		}
		var result ReleaseCommandResult
		if err := json.Unmarshal(existing.ResponseJSON, &result); err != nil {
			return nil, err
		}
		result.Replayed = true
		return &result, nil
	}
	repo, ok := s.repo.(productionPublishRepository)
	if !ok {
		return nil, domain.ErrInvariant
	}
	result, err := repo.PublishProductionCommand(ctx, cmd, digest)
	if err != nil {
		if existing, lookupErr := s.repo.GetProductionCommand(ctx, cmd.WorkspaceID, cmd.PrincipalID, domain.CommandPublish, cmd.IdempotencyKey); lookupErr == nil && existing != nil {
			return s.publishProduction(ctx, cmd)
		}
	}
	return result, err
}
