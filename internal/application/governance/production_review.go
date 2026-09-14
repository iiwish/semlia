package governance

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func (s *ProductionService) reviewProduction(ctx context.Context, cmd ReviewOperationCommand) (*ProductionCommandResult, error) {
	if cmd.WorkspaceID.IsZero() || cmd.PrincipalID.IsZero() || cmd.OperationID.IsZero() || strings.TrimSpace(cmd.IdempotencyKey) == "" || cmd.ExpectedVersion < 1 || cmd.ValidationAttemptNo < 1 || cmd.ValidationAttemptNo > 256 || !domain.IsValidContentDigest(cmd.SetDigest) || !domain.IsValidContentDigest(cmd.ValidationDigest) || len(cmd.ProposalIDs) == 0 || len(cmd.ProposalIDs) > 32 || len(cmd.Note) > 4096 || (cmd.Decision != "approve" && cmd.Decision != "reject") {
		return nil, domain.ErrInvalidArgument
	}
	cmd.ProposalIDs = append([]identity.ProposalID(nil), cmd.ProposalIDs...)
	sort.Slice(cmd.ProposalIDs, func(i, j int) bool { return cmd.ProposalIDs[i].String() < cmd.ProposalIDs[j].String() })
	for i, id := range cmd.ProposalIDs {
		if id.IsZero() || (i > 0 && id == cmd.ProposalIDs[i-1]) {
			return nil, domain.ErrInvalidArgument
		}
	}
	body := cmd
	body.TraceID = ""
	body.IdempotencyKey = ""
	encoded, err := json.Marshal(struct {
		Schema  string
		Command ReviewOperationCommand
	}{"production.review.v1", body})
	if err != nil {
		return nil, err
	}
	digest, err := domain.DigestJSON(encoded)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.GetProductionCommand(ctx, cmd.WorkspaceID, cmd.PrincipalID, domain.CommandReview, cmd.IdempotencyKey)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if existing != nil && existing.RequestDigest != digest {
		return nil, domain.ErrIdempotencyConflict
	}
	_, ver, targets, _, contributors, err := s.GetOperationVersion(ctx, cmd.WorkspaceID, cmd.OperationID, cmd.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	if err := s.checkProductionAction(ctx, cmd.PrincipalID, *ver, targets, domain.CommandReview); err != nil {
		return nil, err
	}
	if existing != nil {
		if len(existing.ResponseJSON) == 0 {
			return nil, domain.ErrPriorStateUnknown
		}
		var result ProductionCommandResult
		if err := json.Unmarshal(existing.ResponseJSON, &result); err != nil {
			return nil, err
		}
		result.Replayed = true
		return &result, nil
	}
	if ver.FrozenAt == nil || ver.SetDigest != cmd.SetDigest {
		return nil, domain.ErrVersionConflict
	}
	for _, c := range contributors {
		if c.PrincipalID == cmd.PrincipalID {
			return nil, domain.ErrSodConflict
		}
	}
	a, err := s.repo.GetLatestValidationAttempt(ctx, cmd.WorkspaceID, cmd.OperationID, ver.Version)
	if err != nil {
		return nil, err
	}
	if a == nil || a.AttemptNo != cmd.ValidationAttemptNo || a.ValidationDigest == nil || *a.ValidationDigest != cmd.ValidationDigest || a.CompletedAt == nil || (a.Status != domain.ValidationStatusSucceeded && (cmd.Decision != "reject" || a.Status != domain.ValidationStatusFailed)) {
		return nil, domain.ErrValidationRequired
	}
	members := map[string]bool{}
	for _, target := range targets {
		if target.ProposalID != nil {
			members[target.ProposalID.String()] = true
		}
	}
	now := time.Now().UTC()
	result := ProductionCommandResult{OperationID: cmd.OperationID, Version: ver.Version, SetDigest: ver.SetDigest, Outcome: "reviewed", ProposalIDs: cmd.ProposalIDs, ValidationAttemptNo: &cmd.ValidationAttemptNo, ValidationRunIDs: []identity.ValidationRunID{}, ReviewIDs: []identity.ReviewID{}}
	reviews := []domain.Review{}
	bindings := []domain.ProductionReviewBinding{}
	proposals := []domain.Proposal{}
	for _, id := range cmd.ProposalIDs {
		if !members[id.String()] {
			return nil, domain.ErrInvalidArgument
		}
		rid, err := identity.NewReviewID()
		if err != nil {
			return nil, err
		}
		result.ReviewIDs = append(result.ReviewIDs, rid)
		decision, state := domain.ReviewApproved, domain.ProposalInReview
		var decided *time.Time
		if cmd.Decision == "reject" {
			decision, state = domain.ReviewRejected, domain.ProposalRejected
			decided = &now
		}
		reviews = append(reviews, domain.Review{ID: rid, WorkspaceID: cmd.WorkspaceID, ProposalID: id, ReviewerPrincipalID: cmd.PrincipalID, Channel: domain.ReviewExpert, Decision: decision, Note: cmd.Note, CreatedAt: now})
		bindings = append(bindings, domain.ProductionReviewBinding{WorkspaceID: cmd.WorkspaceID, ReviewID: rid, OperationID: cmd.OperationID, ProductionVersion: ver.Version, AttemptNo: a.AttemptNo, SetDigest: ver.SetDigest, ValidationDigest: cmd.ValidationDigest, CreatedAt: now})
		proposals = append(proposals, domain.Proposal{ID: id, WorkspaceID: cmd.WorkspaceID, State: state, DecidedAt: decided})
	}
	response, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	command := domain.ProductionCommand{ResponseJSON: response, WorkspaceID: cmd.WorkspaceID, PrincipalID: cmd.PrincipalID, CommandKind: domain.CommandReview, IdempotencyKey: cmd.IdempotencyKey, RequestDigest: digest, OperationID: cmd.OperationID, OperationVersion: ver.Version, ResultKind: "operation", ResultID: cmd.OperationID.String(), CommittedAt: now}
	if err := s.repo.ReviewOperationTx(ctx, cmd.WorkspaceID, cmd.OperationID, ver.Version, a.AttemptNo, ver.SetDigest, cmd.ValidationDigest, reviews, bindings, proposals, command); err != nil {
		if existing, lookupErr := s.repo.GetProductionCommand(ctx, cmd.WorkspaceID, cmd.PrincipalID, domain.CommandReview, cmd.IdempotencyKey); lookupErr == nil && existing != nil {
			return s.reviewProduction(ctx, cmd)
		}
		return nil, err
	}
	return &result, nil
}
