package governance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// Stable refusal codes of the publish and rollback commands. Every refusal
// returns one of these codes on the wire and lands an auditable
// governance.release.denied fact in the same call.
const (
	RefusalBlockerFindings    = "BLOCKED_BY_FINDINGS"
	RefusalNoApprovingReview  = "NO_APPROVING_REVIEW"
	RefusalPriorStateUnknown  = "PRIOR_STATE_UNKNOWN"
	RefusalAlreadyRolledBack  = "RELEASE_ALREADY_ROLLED_BACK"
	RefusalNotLatestRelease   = "RELEASE_NOT_LATEST"
	RefusalPublisherIsAuthor  = "PUBLISHER_IS_AUTHOR"
	RefusalSoleReviewer       = "PUBLISHER_IS_SOLE_REVIEWER"
	publishDenialPolicySource = "docs/SSOT.md §8.1 G1 two-person release control (D-006)"
)

// ReleaseRefusalError is a stable, audited publish/rollback refusal that is
// not a separation-of-duties conflict (those carry the shared
// SeparationOfDutyError explainability contract).
type ReleaseRefusalError struct {
	Code    string
	Message string
	Status  int
	Details map[string]any
}

func (err *ReleaseRefusalError) Error() string { return err.Message }

// PublishingRepository is the persistence contract of the T007 governed
// release surface. Implementations commit every publish and rollback with its
// change-set application, audit facts and outbox events in one transaction.
type PublishingRepository interface {
	GetProposal(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) (domain.Proposal, error)
	ListProposalChanges(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) ([]domain.ChangeSetItem, error)
	CountBlockingResults(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) (int, error)
	ListApprovingReviews(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) ([]domain.Review, error)
	GetRelease(ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID) (domain.Release, error)
	ListReleaseAssets(ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID) ([]domain.ManifestEntry, error)
	ListReleaseObjects(ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID) ([]domain.ObjectManifestEntry, error)
	ListReleases(ctx context.Context, workspace identity.WorkspaceID, limit int, cursor *ReleaseCursor) ([]domain.Release, error)
	CountRollbacksFor(ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID) (int, error)
	MaxReleaseSequence(ctx context.Context, workspace identity.WorkspaceID) (int64, error)
	LatestAssetPinBefore(ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID, sequence int64) (identity.RevisionID, bool, error)
	PublishRelease(ctx context.Context, command PublishReleaseCommand) (domain.Release, error)
	RestoreRelease(ctx context.Context, command RestoreReleaseCommand) (domain.Release, error)
	RecordReleaseDenial(ctx context.Context, denial ReleaseDenialRecord) error
}

// ReleaseCursor is the decoded keyset position of a releases page.
type ReleaseCursor struct {
	PublishedAt time.Time
	ID          identity.ReleaseID
}

// GetReleaseRequest reads one release aggregate.
type GetReleaseRequest struct {
	WorkspaceID  identity.WorkspaceID
	ReleaseID    identity.ReleaseID
	PrincipalRef string
	TraceID      string
}

// ListReleasesRequest pages the workspace releases newest first.
type ListReleasesRequest struct {
	WorkspaceID  identity.WorkspaceID
	Limit        int
	Cursor       string
	PrincipalRef string
	TraceID      string
}

// ReleasePage is one keyset page of releases with the read model including
// manifest entries and object pins.
type ReleasePage struct {
	Items      []domain.Release
	Limit      int
	NextCursor string
}

// PublishReleaseCommand carries the whole publish transaction: the proposal
// coordinates, the pre-minted release and event identities, and the pure
// change-set application callback evaluated inside the transaction against
// the locked current content (shared by asset and object targets).
type PublishReleaseCommand struct {
	WorkspaceID    identity.WorkspaceID
	ProposalID     identity.ProposalID
	Publisher      string
	ReleaseID      identity.ReleaseID
	TraceID        string
	AppliedAt      time.Time
	ReleaseEvents  EventIDs
	ProposalEvents EventIDs
	AssetEvents    EventIDs
	ObjectAuditID  identity.EventID
	Apply          func(content json.RawMessage) (json.RawMessage, error)
}

// AssetRestore is one asset whose current-revision pointer the rollback
// switches back to its prior pinned revision.
type AssetRestore struct {
	AssetID    identity.AssetID
	RevisionID identity.RevisionID
	EventIDs   EventIDs
}

// ObjectRestore is one governed object whose content the rollback restores by
// applying the inverse change-set as a new version bump.
type ObjectRestore struct {
	ObjectType  domain.TargetObjectType
	ObjectID    string
	AuditID     identity.EventID
	ApplyUpdate func(content json.RawMessage) (json.RawMessage, error)
}

// RestoreReleaseCommand carries the whole rollback transaction: the target
// release, the restored manifest entries and object pins, the prior-state
// restores and the pre-minted event identities.
type RestoreReleaseCommand struct {
	WorkspaceID   identity.WorkspaceID
	Target        domain.Release
	Publisher     string
	ReleaseID     identity.ReleaseID
	TraceID       string
	AppliedAt     time.Time
	ReleaseEvents EventIDs
	AssetRestores []AssetRestore
	ObjectRestore *ObjectRestore
	Entries       []domain.ManifestEntry
	Objects       []domain.ObjectManifestEntry
}

// ReleaseDenialRecord is the immutable audit fact of one publish or rollback
// refusal.
type ReleaseDenialRecord struct {
	WorkspaceID  identity.WorkspaceID
	Actor        string
	ReasonCode   string
	ProposalID   *identity.ProposalID
	ReleaseID    *identity.ReleaseID
	Conflict     string
	PolicySource string
	Recovery     []string
	AuditEventID identity.EventID
	TraceID      string
	CreatedAt    time.Time
}

// PublishingService is the T007 release command surface: publish an approved
// proposal as an immutable release that applies its change-set in one
// transaction, and roll back as a new release restoring prior state. The T002
// ReleaseService cut/rollback primitives stay untouched; this service layers
// the governed loop (capability + approvals + two-person control + state
// application) on top of the same store.
type PublishingService struct {
	repository  PublishingRepository
	authorizer  authorizationapp.Evaluator
	clock       Clock
	isSupported bool
}

// NewPublishingService composes the governed release surface over one store.
// A store that does not implement PublishingRepository produces a service
// whose commands fail with the stable unsupported-invariant error instead of
// silently no-oping.
func NewPublishingService(repository any, authorizer authorizationapp.Evaluator, clock Clock) *PublishingService {
	service := &PublishingService{authorizer: authorizer, clock: clock}
	if repo, ok := repository.(PublishingRepository); ok {
		service.repository = repo
		service.isSupported = true
	}
	return service
}

func (service *PublishingService) supported() error {
	if service.repository == nil {
		return fmt.Errorf("%w: governance store does not implement PublishingRepository", domain.ErrInvariant)
	}
	return nil
}

// PublishProposalRequest is the wire-shaped publish command.
type PublishProposalRequest struct {
	WorkspaceID  identity.WorkspaceID
	ProposalID   identity.ProposalID
	PrincipalRef string
	TraceID      string
}

// PublishProposal cuts an immutable release from an approved proposal:
// gates first (state, blockers, approving review, capability, two-person
// control), then ONE transaction that applies the change-set (new asset
// revision with a current-revision switch, or governed-object version bump),
// cuts the release manifest pinning the resulting state, walks the proposal
// to released, and commits the audit facts plus the release.published (and
// for asset targets catalog.asset.changed) outbox events.
func (service *PublishingService) PublishProposal(ctx context.Context, request PublishProposalRequest) (domain.Release, error) {
	if err := service.supported(); err != nil {
		return domain.Release{}, err
	}
	if request.WorkspaceID.IsZero() || request.ProposalID.IsZero() {
		return domain.Release{}, domain.ErrInvalidArgument
	}
	proposal, err := service.repository.GetProposal(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return domain.Release{}, err
	}
	if proposal.State != domain.ProposalInReview {
		denialErr := &ReleaseRefusalError{
			Code:    "PROPOSAL_NOT_IN_REVIEW",
			Message: fmt.Sprintf("proposal is %s, publish is no longer applicable", proposal.State),
			Status:  409,
			Details: map[string]any{"proposalId": proposal.ID.String(), "state": string(proposal.State)},
		}
		return domain.Release{}, service.refuse(ctx, request.WorkspaceID, request.PrincipalRef, denialErr, &proposal.ID, nil, request.TraceID)
	}
	blockers, err := service.repository.CountBlockingResults(ctx, request.WorkspaceID, proposal.ID)
	if err != nil {
		return domain.Release{}, err
	}
	if blockers > 0 {
		denialErr := &ReleaseRefusalError{
			Code:    RefusalBlockerFindings,
			Message: "blocker findings block release candidacy",
			Status:  422,
			Details: map[string]any{"proposalId": proposal.ID.String()},
		}
		return domain.Release{}, service.refuse(ctx, request.WorkspaceID, request.PrincipalRef, denialErr, &proposal.ID, nil, request.TraceID)
	}
	approvals, err := service.repository.ListApprovingReviews(ctx, request.WorkspaceID, proposal.ID)
	if err != nil {
		return domain.Release{}, err
	}
	if len(approvals) == 0 {
		denialErr := &ReleaseRefusalError{
			Code:    RefusalNoApprovingReview,
			Message: "publishing requires at least one approving review",
			Status:  422,
			Details: map[string]any{"proposalId": proposal.ID.String()},
		}
		return domain.Release{}, service.refuse(ctx, request.WorkspaceID, request.PrincipalRef, denialErr, &proposal.ID, nil, request.TraceID)
	}
	evaluation, err := service.authorize(ctx, request.WorkspaceID, request.PrincipalRef, authorization.ActionReleasePublish, request.TraceID)
	if err != nil {
		return domain.Release{}, service.auditCapabilityDenial(ctx, request.WorkspaceID, request.PrincipalRef, evaluation, err, &proposal.ID, nil, request.TraceID)
	}
	publisher := evaluation.PrincipalID.String()
	if proposal.CreatedBy == publisher {
		return domain.Release{}, service.refuseDuty(ctx, request.WorkspaceID, publisher, RefusalPublisherIsAuthor,
			"the publisher cannot publish their own proposal", &proposal.ID, nil, request.TraceID,
			"Ask a different principal holding release.publish to publish the proposal.")
	}
	if len(approvals) == 1 && approvals[0].ReviewerPrincipalID.String() == publisher {
		return domain.Release{}, service.refuseDuty(ctx, request.WorkspaceID, publisher, RefusalSoleReviewer,
			"the sole approving reviewer cannot publish the proposal they approved",
			&proposal.ID, nil, request.TraceID,
			"Ask a second principal holding release.publish to publish, or record a second independent approving review.")
	}
	changes, err := service.repository.ListProposalChanges(ctx, request.WorkspaceID, proposal.ID)
	if err != nil {
		return domain.Release{}, err
	}
	releaseID, err := identity.NewReleaseID()
	if err != nil {
		return domain.Release{}, fmt.Errorf("mint release ID: %w", err)
	}
	releaseEvents, err := newEventIDs()
	if err != nil {
		return domain.Release{}, err
	}
	proposalEvents, err := newEventIDs()
	if err != nil {
		return domain.Release{}, err
	}
	command := PublishReleaseCommand{
		WorkspaceID: request.WorkspaceID, ProposalID: proposal.ID, Publisher: publisher,
		ReleaseID: releaseID, TraceID: request.TraceID, AppliedAt: service.clock.Now().UTC(),
		ReleaseEvents: releaseEvents, ProposalEvents: proposalEvents,
		Apply: func(content json.RawMessage) (json.RawMessage, error) {
			return domain.ApplyChangeSet(content, changes)
		},
	}
	if proposal.TargetObjectType == domain.TargetSemanticAsset {
		assetEvents, eventErr := newEventIDs()
		if eventErr != nil {
			return domain.Release{}, eventErr
		}
		command.AssetEvents = assetEvents
	} else {
		auditID, auditErr := identity.NewEventID()
		if auditErr != nil {
			return domain.Release{}, auditErr
		}
		command.ObjectAuditID = auditID
	}
	return service.repository.PublishRelease(ctx, command)
}

// RollbackReleaseRequest is the wire-shaped rollback command.
type RollbackReleaseRequest struct {
	WorkspaceID  identity.WorkspaceID
	ReleaseID    identity.ReleaseID
	PrincipalRef string
	TraceID      string
}

// RollbackRelease publishes a NEW immutable release (D-008) that references
// the target and restores the prior application state: asset targets switch
// the current-revision pointer back to the prior pinned revision, object
// targets apply the inverse change-set as a new version bump. The target
// release rows are never mutated; each release can be rolled back at most
// once, and only the latest release is rollback-eligible so the restore is
// well-defined.
func (service *PublishingService) RollbackRelease(ctx context.Context, request RollbackReleaseRequest) (domain.Release, error) {
	if err := service.supported(); err != nil {
		return domain.Release{}, err
	}
	if request.WorkspaceID.IsZero() || request.ReleaseID.IsZero() {
		return domain.Release{}, domain.ErrInvalidArgument
	}
	evaluation, err := service.authorize(ctx, request.WorkspaceID, request.PrincipalRef, authorization.ActionReleaseRollback, request.TraceID)
	if err != nil {
		return domain.Release{}, service.auditCapabilityDenial(ctx, request.WorkspaceID, request.PrincipalRef, evaluation, err, nil, &request.ReleaseID, request.TraceID)
	}
	publisher := evaluation.PrincipalID.String()
	target, err := service.repository.GetRelease(ctx, request.WorkspaceID, request.ReleaseID)
	if err != nil {
		return domain.Release{}, err
	}
	if target.Entries, err = service.repository.ListReleaseAssets(ctx, request.WorkspaceID, target.ID); err != nil {
		return domain.Release{}, err
	}
	if target.Objects, err = service.repository.ListReleaseObjects(ctx, request.WorkspaceID, target.ID); err != nil {
		return domain.Release{}, err
	}
	if target.State != domain.ReleasePublished {
		denialErr := &ReleaseRefusalError{
			Code:    "RELEASE_NOT_PUBLISHED",
			Message: fmt.Sprintf("release is %s, rollback is no longer applicable", target.State),
			Status:  409,
			Details: map[string]any{"releaseId": target.ID.String(), "state": string(target.State)},
		}
		return domain.Release{}, service.refuse(ctx, request.WorkspaceID, request.PrincipalRef, denialErr, nil, &target.ID, request.TraceID)
	}
	rollbacks, err := service.repository.CountRollbacksFor(ctx, request.WorkspaceID, target.ID)
	if err != nil {
		return domain.Release{}, err
	}
	if rollbacks > 0 {
		denialErr := &ReleaseRefusalError{
			Code:    RefusalAlreadyRolledBack,
			Message: "release has already been rolled back",
			Status:  409,
			Details: map[string]any{"releaseId": target.ID.String()},
		}
		return domain.Release{}, service.refuse(ctx, request.WorkspaceID, request.PrincipalRef, denialErr, nil, &target.ID, request.TraceID)
	}
	maxSequence, err := service.repository.MaxReleaseSequence(ctx, request.WorkspaceID)
	if err != nil {
		return domain.Release{}, err
	}
	if maxSequence != target.Sequence {
		denialErr := &ReleaseRefusalError{
			Code:    RefusalNotLatestRelease,
			Message: "only the latest release can be rolled back",
			Status:  409,
			Details: map[string]any{"releaseId": target.ID.String(), "latestSequence": maxSequence},
		}
		return domain.Release{}, service.refuse(ctx, request.WorkspaceID, request.PrincipalRef, denialErr, nil, &target.ID, request.TraceID)
	}
	var changes []domain.ChangeSetItem
	if target.OriginProposalID != nil {
		origin, proposalErr := service.repository.GetProposal(ctx, request.WorkspaceID, *target.OriginProposalID)
		if proposalErr != nil {
			return domain.Release{}, proposalErr
		}
		if origin.CreatedBy == publisher {
			return domain.Release{}, service.refuseDuty(ctx, request.WorkspaceID, publisher, RefusalPublisherIsAuthor,
				"the publisher cannot roll back the release of their own proposal", nil, &target.ID, request.TraceID,
				"Ask a different principal holding release.rollback to roll the release back.")
		}
		changes, proposalErr = service.repository.ListProposalChanges(ctx, request.WorkspaceID, origin.ID)
		if proposalErr != nil {
			return domain.Release{}, proposalErr
		}
	}
	restores, entries, objects, refusal := service.planRestore(ctx, request.WorkspaceID, target, changes)
	if refusal != nil {
		return domain.Release{}, service.refuse(ctx, request.WorkspaceID, request.PrincipalRef, refusal, target.OriginProposalID, &target.ID, request.TraceID)
	}
	releaseID, err := identity.NewReleaseID()
	if err != nil {
		return domain.Release{}, fmt.Errorf("mint rollback release ID: %w", err)
	}
	releaseEvents, err := newEventIDs()
	if err != nil {
		return domain.Release{}, err
	}
	command := RestoreReleaseCommand{
		WorkspaceID: request.WorkspaceID, Target: target, Publisher: publisher,
		ReleaseID: releaseID, TraceID: request.TraceID, AppliedAt: service.clock.Now().UTC(),
		ReleaseEvents: releaseEvents, AssetRestores: restores, Entries: entries, Objects: objects,
	}
	if len(objects) > 0 {
		auditID, auditErr := identity.NewEventID()
		if auditErr != nil {
			return domain.Release{}, auditErr
		}
		command.ObjectRestore = &ObjectRestore{
			ObjectType: target.Objects[0].ObjectType, ObjectID: target.Objects[0].ObjectID,
			AuditID: auditID,
			ApplyUpdate: func(content json.RawMessage) (json.RawMessage, error) {
				return domain.ApplyInverseChangeSet(content, changes)
			},
		}
	}
	return service.repository.RestoreRelease(ctx, command)
}

// planRestore resolves the prior application state the rollback must restore:
// assets switch back to the newest revision pinned by an earlier release
// (falling back to the originating proposal's base revision), objects restore
// content through the inverse change-set. The returned manifest entries carry
// the restored pins, with untouched entries carried over from the release
// preceding the target (T002 rollback semantics).
func (service *PublishingService) planRestore(
	ctx context.Context,
	workspace identity.WorkspaceID,
	target domain.Release,
	changes []domain.ChangeSetItem,
) ([]AssetRestore, []domain.ManifestEntry, []domain.ObjectManifestEntry, *ReleaseRefusalError) {
	restores := make([]AssetRestore, 0, len(target.Entries))
	var refusal *ReleaseRefusalError
	entries := make([]domain.ManifestEntry, 0, len(target.Entries))
	for _, entry := range target.Entries {
		prior, found, err := service.repository.LatestAssetPinBefore(ctx, workspace, entry.AssetID, target.Sequence)
		if err != nil {
			return nil, nil, nil, refusalError(RefusalPriorStateUnknown, "prior revision pin lookup failed", 500, target.ID)
		}
		if !found && target.OriginProposalID != nil {
			origin, originErr := service.repository.GetProposal(ctx, workspace, *target.OriginProposalID)
			if originErr == nil && origin.BaseRevisionID != nil && origin.AssetID != nil && *origin.AssetID == entry.AssetID {
				prior, found = *origin.BaseRevisionID, true
			}
		}
		if !found {
			refusal = refusalError(RefusalPriorStateUnknown,
				"no prior revision pin or proposal base revision exists for the release target", 422, target.ID)
			return nil, nil, nil, refusal
		}
		events, eventErr := newEventIDs()
		if eventErr != nil {
			return nil, nil, nil, refusalError(RefusalPriorStateUnknown, eventErr.Error(), 500, target.ID)
		}
		restores = append(restores, AssetRestore{AssetID: entry.AssetID, RevisionID: prior, EventIDs: events})
		entries = append(entries, domain.ManifestEntry{
			AssetID: entry.AssetID, RevisionID: prior, Compatibility: entry.Compatibility, Position: entry.Position,
		})
	}
	objects := make([]domain.ObjectManifestEntry, 0, len(target.Objects))
	for _, entry := range target.Objects {
		if len(changes) == 0 {
			refusal = refusalError(RefusalPriorStateUnknown,
				"the originating proposal change-set is required to restore governed-object content", 422, target.ID)
			return nil, nil, nil, refusal
		}
		objects = append(objects, domain.ObjectManifestEntry{
			ObjectType: entry.ObjectType, ObjectID: entry.ObjectID,
			Version: entry.Version + 1, Position: entry.Position,
		})
	}
	return restores, entries, objects, nil
}

func refusalError(code, message string, status int, release identity.ReleaseID) *ReleaseRefusalError {
	return &ReleaseRefusalError{
		Code: code, Message: message, Status: status,
		Details: map[string]any{"releaseId": release.String()},
	}
}

// GetRelease reads one release with its manifest entries and object pins.
// Read surface: the same workspace-scoped asset.read capability proposal
// reads use.
func (service *PublishingService) GetRelease(ctx context.Context, request GetReleaseRequest) (domain.Release, error) {
	if err := service.supported(); err != nil {
		return domain.Release{}, err
	}
	if request.WorkspaceID.IsZero() || request.ReleaseID.IsZero() {
		return domain.Release{}, domain.ErrInvalidArgument
	}
	if err := service.authorizeRead(ctx, request.WorkspaceID, request.PrincipalRef, request.TraceID); err != nil {
		return domain.Release{}, err
	}
	return service.loadRelease(ctx, request.WorkspaceID, request.ReleaseID)
}

// ListReleases pages workspace releases newest first with the T003-style
// keyset cursor.
func (service *PublishingService) ListReleases(ctx context.Context, request ListReleasesRequest) (ReleasePage, error) {
	if err := service.supported(); err != nil {
		return ReleasePage{}, err
	}
	if request.WorkspaceID.IsZero() {
		return ReleasePage{}, domain.ErrInvalidArgument
	}
	if err := service.authorizeRead(ctx, request.WorkspaceID, request.PrincipalRef, request.TraceID); err != nil {
		return ReleasePage{}, err
	}
	limit, err := normalizeReleaseLimit(request.Limit)
	if err != nil {
		return ReleasePage{}, err
	}
	cursor, err := decodeReleaseCursor(request.Cursor)
	if err != nil {
		return ReleasePage{}, err
	}
	items, err := service.repository.ListReleases(ctx, request.WorkspaceID, limit+1, cursor)
	if err != nil {
		return ReleasePage{}, err
	}
	page := ReleasePage{Limit: limit}
	if len(items) > limit {
		last := items[limit-1]
		page.NextCursor, err = encodeReleaseCursor(last)
		if err != nil {
			return ReleasePage{}, err
		}
		items = items[:limit]
	}
	page.Items = make([]domain.Release, 0, len(items))
	for _, item := range items {
		loaded, loadErr := service.loadRelease(ctx, request.WorkspaceID, item.ID)
		if loadErr != nil {
			return ReleasePage{}, loadErr
		}
		page.Items = append(page.Items, loaded)
	}
	return page, nil
}

func (service *PublishingService) loadRelease(
	ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID,
) (domain.Release, error) {
	releaseRow, err := service.repository.GetRelease(ctx, workspace, release)
	if err != nil {
		return domain.Release{}, err
	}
	entries, err := service.repository.ListReleaseAssets(ctx, workspace, release)
	if err != nil {
		return domain.Release{}, err
	}
	releaseRow.Entries = entries
	objects, err := service.repository.ListReleaseObjects(ctx, workspace, release)
	if err != nil {
		return domain.Release{}, err
	}
	releaseRow.Objects = objects
	return releaseRow, nil
}

func (service *PublishingService) authorizeRead(
	ctx context.Context, workspace identity.WorkspaceID, principalRef, traceID string,
) error {
	if service.authorizer == nil {
		return nil
	}
	evaluation, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: principalRef, WorkspaceID: workspace,
		Action:   authorization.ActionAssetRead,
		Resource: authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()},
		TraceID:  traceID,
	})
	if err != nil {
		return err
	}
	if !evaluation.Allowed {
		return &authorization.DenialError{Decision: evaluation}
	}
	return nil
}

func normalizeReleaseLimit(limit int) (int, error) {
	if limit == 0 {
		return 50, nil
	}
	if limit < 1 || limit > 200 {
		return 0, domain.ErrInvalidArgument
	}
	return limit, nil
}

type releaseCursorEnvelope struct {
	Kind        string    `json:"kind"`
	PublishedAt time.Time `json:"publishedAt"`
	ID          string    `json:"id"`
}

func encodeReleaseCursor(last domain.Release) (string, error) {
	encoded, err := json.Marshal(releaseCursorEnvelope{
		Kind: "release", PublishedAt: last.PublishedAt.UTC(), ID: last.ID.String(),
	})
	if err != nil {
		return "", fmt.Errorf("encode release cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeReleaseCursor(value string) (*ReleaseCursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%w: release cursor", domain.ErrInvalidArgument)
	}
	var envelope releaseCursorEnvelope
	if err := json.Unmarshal(decoded, &envelope); err != nil || envelope.Kind != "release" ||
		envelope.PublishedAt.IsZero() {
		return nil, fmt.Errorf("%w: release cursor", domain.ErrInvalidArgument)
	}
	releaseID, err := identity.ParseReleaseID(envelope.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: release cursor", domain.ErrInvalidArgument)
	}
	return &ReleaseCursor{PublishedAt: envelope.PublishedAt.UTC(), ID: releaseID}, nil
}

func (service *PublishingService) authorize(
	ctx context.Context,
	workspace identity.WorkspaceID,
	principalRef string,
	action authorization.Action,
	traceID string,
) (authorization.Decision, error) {
	if service.authorizer == nil {
		return authorization.Decision{}, fmt.Errorf(
			"%w: release commands require the authorization evaluator", domain.ErrInvariant)
	}
	evaluation, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: principalRef, WorkspaceID: workspace, Action: action,
		Resource: authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()},
		TraceID:  traceID,
	})
	if err != nil {
		return authorization.Decision{}, err
	}
	if !evaluation.Allowed {
		return evaluation, &authorization.DenialError{Decision: evaluation}
	}
	return evaluation, nil
}

func (service *PublishingService) refuse(
	ctx context.Context,
	workspace identity.WorkspaceID,
	principalRef string,
	refusal *ReleaseRefusalError,
	proposalID *identity.ProposalID,
	releaseID *identity.ReleaseID,
	traceID string,
) error {
	if recordErr := service.recordDenial(ctx, workspace, principalRef, refusal.Code, "", proposalID, releaseID, refusal.Details, traceID); recordErr != nil {
		return recordErr
	}
	return refusal
}

func (service *PublishingService) refuseDuty(
	ctx context.Context,
	workspace identity.WorkspaceID,
	actor string,
	code string,
	conflict string,
	proposalID *identity.ProposalID,
	releaseID *identity.ReleaseID,
	traceID string,
	recovery string,
) error {
	denial := &SeparationOfDutyError{
		Conflict: conflict, Scope: workspace.String(),
		Source: publishDenialPolicySource, Recovery: []string{recovery},
	}
	if recordErr := service.recordDenial(ctx, workspace, actor, code, conflict, proposalID, releaseID, nil, traceID); recordErr != nil {
		return recordErr
	}
	return denial
}

// auditCapabilityDenial records the release refusal audit fact for a
// capability denial on top of the authorization_events row the evaluator
// already wrote, then returns the original denial error.
func (service *PublishingService) auditCapabilityDenial(
	ctx context.Context,
	workspace identity.WorkspaceID,
	principalRef string,
	evaluation authorization.Decision,
	authorizeErr error,
	proposalID *identity.ProposalID,
	releaseID *identity.ReleaseID,
	traceID string,
) error {
	var denial *authorization.DenialError
	if errors.As(authorizeErr, &denial) {
		if recordErr := service.recordDenial(ctx, workspace, principalRef,
			string(denial.Decision.ReasonCode), "", proposalID, releaseID, nil, traceID); recordErr != nil {
			return recordErr
		}
	}
	return authorizeErr
}

func (service *PublishingService) recordDenial(
	ctx context.Context,
	workspace identity.WorkspaceID,
	actor string,
	reasonCode string,
	conflict string,
	proposalID *identity.ProposalID,
	releaseID *identity.ReleaseID,
	details map[string]any,
	traceID string,
) error {
	_ = details
	eventID, err := identity.NewEventID()
	if err != nil {
		return fmt.Errorf("mint release denial audit event ID: %w", err)
	}
	return service.repository.RecordReleaseDenial(ctx, ReleaseDenialRecord{
		WorkspaceID: workspace, Actor: strings.TrimSpace(actor), ReasonCode: reasonCode,
		ProposalID: proposalID, ReleaseID: releaseID, Conflict: conflict,
		PolicySource: publishDenialPolicySource, AuditEventID: eventID, TraceID: traceID,
		CreatedAt: service.clock.Now().UTC(),
	})
}
