package governance

import (
	"context"
	"errors"
	"fmt"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type ReleaseRepository interface {
	GetProposal(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) (governance.Proposal, error)
	CountBlockingResults(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) (int, error)
	CountRollbacksFor(ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID) (int, error)
	GetRelease(ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID) (governance.Release, error)
	ListReleaseAssets(ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID) ([]governance.ManifestEntry, error)
	GetPreviousRelease(ctx context.Context, workspace identity.WorkspaceID, sequence int64) (governance.Release, error)
	CutRelease(ctx context.Context, command ReleaseCutCommand) (governance.Release, error)
	RollbackRelease(ctx context.Context, command ReleaseRollbackCommand) (governance.Release, error)
}

// ReleaseCutCommand carries the pre-built release aggregate, its manifest and
// both pre-minted event identity pairs (release event, optional proposal
// event) so the repository commits everything in one transaction.
type ReleaseCutCommand struct {
	Release               governance.Release
	Entries               []governance.ManifestEntry
	ProposalID            *identity.ProposalID
	TraceID               string
	ReleaseEvents         EventIDs
	ProposalAuditEventID  identity.EventID
	ProposalOutboxEventID identity.EventID
	HasProposalEvents     bool
}

type ReleaseRollbackCommand struct {
	Release       governance.Release
	Entries       []governance.ManifestEntry
	TraceID       string
	ReleaseEvents EventIDs
}

type ReleaseService struct {
	repository ReleaseRepository
	clock      Clock
}

func NewReleaseService(repository ReleaseRepository, clock Clock) *ReleaseService {
	return &ReleaseService{repository: repository, clock: clock}
}

type CutRequest struct {
	WorkspaceID identity.WorkspaceID
	ProposalID  *identity.ProposalID
	Entries     []governance.ManifestEntry
	PublishedBy string
	TraceID     string
}

// Cut publishes an immutable release: the manifest pins asset_id +
// revision_id + compatibility per entry and its digest is the sha256 of the
// canonical manifest payload. When the cut resolves a proposal it must sit in
// in_review with no reliable Blocker findings, and the proposal walks
// in_review -> released inside the same transaction.
func (service *ReleaseService) Cut(ctx context.Context, request CutRequest) (governance.Release, error) {
	if request.WorkspaceID.IsZero() || len(request.Entries) == 0 || len(request.Entries) > 1000 {
		return governance.Release{}, governance.ErrInvalidArgument
	}
	if positionsInvalid(request.Entries) {
		return governance.Release{}, governance.ErrInvalidArgument
	}
	for _, entry := range request.Entries {
		if err := entry.Validate(); err != nil {
			return governance.Release{}, err
		}
	}
	var proposal *governance.Proposal
	if request.ProposalID != nil {
		current, err := service.repository.GetProposal(ctx, request.WorkspaceID, *request.ProposalID)
		if err != nil {
			return governance.Release{}, err
		}
		if current.State != governance.ProposalInReview {
			return governance.Release{}, fmt.Errorf(
				"%w: proposal %s must be in_review before release", governance.ErrInvariant, current.State)
		}
		blockers, err := service.repository.CountBlockingResults(ctx, request.WorkspaceID, *request.ProposalID)
		if err != nil {
			return governance.Release{}, err
		}
		if blockers > 0 {
			return governance.Release{}, fmt.Errorf(
				"%w: blocker findings block release candidacy", governance.ErrInvariant)
		}
		proposal = &current
	}
	manifestPayload, err := governance.ManifestDigestPayload(request.Entries)
	if err != nil {
		return governance.Release{}, err
	}
	manifestDigest, err := governance.DigestJSON(manifestPayload)
	if err != nil {
		return governance.Release{}, err
	}
	releaseID, err := identity.NewReleaseID()
	if err != nil {
		return governance.Release{}, fmt.Errorf("mint release ID: %w", err)
	}
	releaseEvents, err := newEventIDs()
	if err != nil {
		return governance.Release{}, err
	}
	command := ReleaseCutCommand{
		Release: governance.Release{
			ID: releaseID, WorkspaceID: request.WorkspaceID, ManifestDigest: manifestDigest,
			State: governance.ReleasePublished, PublishedBy: request.PublishedBy,
			PublishedAt: service.clock.Now().UTC(), CreatedAt: service.clock.Now().UTC(),
		},
		Entries: request.Entries, ProposalID: request.ProposalID, TraceID: request.TraceID,
		ReleaseEvents: releaseEvents,
	}
	if proposal != nil {
		proposalEvents, eventErr := newEventIDs()
		if eventErr != nil {
			return governance.Release{}, eventErr
		}
		command.ProposalAuditEventID = proposalEvents.AuditEventID
		command.ProposalOutboxEventID = proposalEvents.OutboxEventID
		command.HasProposalEvents = true
	}
	return service.repository.CutRelease(ctx, command)
}

type RollbackRequest struct {
	WorkspaceID     identity.WorkspaceID
	TargetReleaseID identity.ReleaseID
	PublishedBy     string
	TraceID         string
}

// Rollback never touches the target release: it publishes a NEW release whose
// manifest restores the entries of the release preceding the target (empty
// when the target is the first release) and whose
// rolled_back_to_release_id references the target. Each release can be rolled
// back at most once; rollback policy refinements belong to T007.
func (service *ReleaseService) Rollback(ctx context.Context, request RollbackRequest) (governance.Release, error) {
	if request.WorkspaceID.IsZero() || request.TargetReleaseID.IsZero() {
		return governance.Release{}, governance.ErrInvalidArgument
	}
	target, err := service.repository.GetRelease(ctx, request.WorkspaceID, request.TargetReleaseID)
	if err != nil {
		return governance.Release{}, err
	}
	if target.State != governance.ReleasePublished {
		return governance.Release{}, governance.ErrConflict
	}
	rollbacks, err := service.repository.CountRollbacksFor(ctx, request.WorkspaceID, target.ID)
	if err != nil {
		return governance.Release{}, err
	}
	if rollbacks > 0 {
		return governance.Release{}, fmt.Errorf(
			"%w: release %s has already been rolled back", governance.ErrConflict, target.ID)
	}
	var entries []governance.ManifestEntry
	previous, err := service.repository.GetPreviousRelease(ctx, request.WorkspaceID, target.Sequence)
	switch {
	case err == nil:
		entries, err = service.repository.ListReleaseAssets(ctx, request.WorkspaceID, previous.ID)
		if err != nil {
			return governance.Release{}, err
		}
	case errors.Is(err, governance.ErrNotFound):
		entries = []governance.ManifestEntry{}
	default:
		return governance.Release{}, err
	}
	manifestPayload, err := governance.ManifestDigestPayload(entries)
	if err != nil {
		return governance.Release{}, err
	}
	manifestDigest, err := governance.DigestJSON(manifestPayload)
	if err != nil {
		return governance.Release{}, err
	}
	releaseID, err := identity.NewReleaseID()
	if err != nil {
		return governance.Release{}, fmt.Errorf("mint rollback release ID: %w", err)
	}
	releaseEvents, err := newEventIDs()
	if err != nil {
		return governance.Release{}, err
	}
	return service.repository.RollbackRelease(ctx, ReleaseRollbackCommand{
		Release: governance.Release{
			ID: releaseID, WorkspaceID: request.WorkspaceID, ManifestDigest: manifestDigest,
			State: governance.ReleasePublished, RolledBackToReleaseID: &request.TargetReleaseID,
			PublishedBy: request.PublishedBy, PublishedAt: service.clock.Now().UTC(),
			CreatedAt: service.clock.Now().UTC(),
		},
		Entries: entries, TraceID: request.TraceID, ReleaseEvents: releaseEvents,
	})
}

func (service *ReleaseService) GetRelease(
	ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID,
) (governance.Release, error) {
	return service.repository.GetRelease(ctx, workspace, release)
}

func (service *ReleaseService) ListReleaseAssets(
	ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID,
) ([]governance.ManifestEntry, error) {
	return service.repository.ListReleaseAssets(ctx, workspace, release)
}

func positionsInvalid(entries []governance.ManifestEntry) bool {
	seen := make(map[int]struct{}, len(entries))
	for _, entry := range entries {
		if _, exists := seen[entry.Position]; exists {
			return true
		}
		seen[entry.Position] = struct{}{}
	}
	return false
}
