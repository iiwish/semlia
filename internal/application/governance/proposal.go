package governance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type ProposalRepository interface {
	CreateProposal(ctx context.Context, proposal governance.Proposal) (governance.Proposal, error)
	GetProposal(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) (governance.Proposal, error)
	SubmitProposal(ctx context.Context, command ProposalSubmitCommand) (governance.Proposal, error)
	TransitionProposal(ctx context.Context, command ProposalTransitionCommand) (governance.Proposal, error)
	CreateProposalChange(ctx context.Context, item governance.ChangeSetItem) (governance.ChangeSetItem, error)
	DeleteProposalChange(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID, change identity.ProposalChangeID) error
	ListProposalChanges(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) ([]governance.ChangeSetItem, error)
	CreateReview(ctx context.Context, review governance.Review) (governance.Review, error)
}

// ProposalSubmitCommand carries the pre-minted event identities so the
// repository commits the state change, the audit fact and the
// proposal.changed outbox event in one transaction.
type ProposalSubmitCommand struct {
	WorkspaceID   identity.WorkspaceID
	ProposalID    identity.ProposalID
	Actor         string
	SubmittedAt   time.Time
	TraceID       string
	AuditEventID  identity.EventID
	OutboxEventID identity.EventID
}

type ProposalTransitionCommand struct {
	WorkspaceID   identity.WorkspaceID
	ProposalID    identity.ProposalID
	To            governance.ProposalState
	Actor         string
	DecidedAt     *time.Time
	UpdatedAt     time.Time
	TraceID       string
	AuditEventID  identity.EventID
	OutboxEventID identity.EventID
}

type ProposalService struct {
	repository ProposalRepository
	clock      Clock
}

func NewProposalService(repository ProposalRepository, clock Clock) *ProposalService {
	return &ProposalService{repository: repository, clock: clock}
}

type CreateProposalRequest struct {
	WorkspaceID    identity.WorkspaceID
	AssetID        identity.AssetID
	BaseRevisionID identity.RevisionID
	Title          string
	Summary        string
	Reason         string
	CreatedBy      string
	TraceID        string
}

// CreateProposal opens a draft with a structured change-set against the
// released baseline revision. Draft creation is a local authoring act and
// emits no governance event; the §8.2 pipeline starts at Submit.
func (service *ProposalService) CreateProposal(ctx context.Context, request CreateProposalRequest) (governance.Proposal, error) {
	if request.WorkspaceID.IsZero() || request.AssetID.IsZero() || request.BaseRevisionID.IsZero() {
		return governance.Proposal{}, governance.ErrInvalidArgument
	}
	request.Title = strings.TrimSpace(request.Title)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if request.Title == "" || len(request.Title) > 256 || len(request.Summary) > 4096 ||
		len(request.Reason) > 4096 || request.CreatedBy == "" || len(request.CreatedBy) > 256 {
		return governance.Proposal{}, governance.ErrInvalidArgument
	}
	proposalID, err := identity.NewProposalID()
	if err != nil {
		return governance.Proposal{}, fmt.Errorf("mint proposal ID: %w", err)
	}
	proposal := governance.Proposal{
		ID: proposalID, WorkspaceID: request.WorkspaceID,
		AssetID: &request.AssetID, BaseRevisionID: &request.BaseRevisionID,
		TargetObjectType: governance.TargetSemanticAsset, TargetObjectID: request.AssetID.UUID(),
		State: governance.ProposalDraft, Title: request.Title,
		Summary: request.Summary, Reason: request.Reason, CreatedBy: request.CreatedBy,
		CreatedAt: service.clock.Now().UTC(), UpdatedAt: service.clock.Now().UTC(),
	}
	return service.repository.CreateProposal(ctx, proposal)
}

type CreateGovernedProposalRequest struct {
	WorkspaceID    identity.WorkspaceID
	TargetType     governance.TargetObjectType
	TargetObjectID string
	Title          string
	Summary        string
	Reason         string
	CreatedBy      string
}

// CreateGovernedProposal opens a draft against one of the four governance
// object types (physical_binding, model_grain, entity_key, join_contract).
// The target object existence is validated at submission, not here: drafts
// may reference objects that appear before the change-set is complete.
func (service *ProposalService) CreateGovernedProposal(ctx context.Context, request CreateGovernedProposalRequest) (governance.Proposal, error) {
	request.Title = strings.TrimSpace(request.Title)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if request.WorkspaceID.IsZero() || !request.TargetType.IsGovernedObject() ||
		request.Title == "" || len(request.Title) > 256 || len(request.Summary) > 4096 ||
		len(request.Reason) > 4096 || request.CreatedBy == "" || len(request.CreatedBy) > 256 {
		return governance.Proposal{}, governance.ErrInvalidArgument
	}
	targetUUID, err := ParseGovernedObjectUUID(request.TargetType, request.TargetObjectID)
	if err != nil {
		return governance.Proposal{}, err
	}
	proposalID, err := identity.NewProposalID()
	if err != nil {
		return governance.Proposal{}, fmt.Errorf("mint proposal ID: %w", err)
	}
	now := service.clock.Now().UTC()
	proposal := governance.Proposal{
		ID: proposalID, WorkspaceID: request.WorkspaceID,
		TargetObjectType: request.TargetType, TargetObjectID: targetUUID,
		State: governance.ProposalDraft, Title: request.Title,
		Summary: request.Summary, Reason: request.Reason, CreatedBy: request.CreatedBy,
		CreatedAt: now, UpdatedAt: now,
	}
	return service.repository.CreateProposal(ctx, proposal)
}

type AddChangeRequest struct {
	WorkspaceID identity.WorkspaceID
	ProposalID  identity.ProposalID
	Item        governance.ChangeSetItem
}

// AddChange appends one structured patch entry while the proposal is a draft.
// Once submitted the change-set is frozen (ErrChangeSetFrozen) and the
// database trigger enforces the same invariant under concurrency.
func (service *ProposalService) AddChange(ctx context.Context, request AddChangeRequest) (governance.ChangeSetItem, error) {
	proposal, err := service.repository.GetProposal(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return governance.ChangeSetItem{}, err
	}
	if !governance.ChangeSetEditable(proposal.State) {
		return governance.ChangeSetItem{}, governance.ErrChangeSetFrozen
	}
	item := request.Item
	item.WorkspaceID = request.WorkspaceID
	item.ProposalID = request.ProposalID
	if err := item.Validate(); err != nil {
		return governance.ChangeSetItem{}, err
	}
	itemID, err := identity.NewProposalChangeID()
	if err != nil {
		return governance.ChangeSetItem{}, fmt.Errorf("mint change-set item ID: %w", err)
	}
	item.ID = itemID
	item.CreatedAt = service.clock.Now().UTC()
	return service.repository.CreateProposalChange(ctx, item)
}

type RemoveChangeRequest struct {
	WorkspaceID identity.WorkspaceID
	ProposalID  identity.ProposalID
	ChangeID    identity.ProposalChangeID
}

func (service *ProposalService) RemoveChange(ctx context.Context, request RemoveChangeRequest) error {
	proposal, err := service.repository.GetProposal(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return err
	}
	if !governance.ChangeSetEditable(proposal.State) {
		return governance.ErrChangeSetFrozen
	}
	return service.repository.DeleteProposalChange(ctx, request.WorkspaceID, request.ProposalID, request.ChangeID)
}

func (service *ProposalService) GetProposal(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) (governance.Proposal, error) {
	return service.repository.GetProposal(ctx, workspace, proposal)
}

func (service *ProposalService) ListChanges(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) ([]governance.ChangeSetItem, error) {
	return service.repository.ListProposalChanges(ctx, workspace, proposal)
}

type SubmitRequest struct {
	WorkspaceID identity.WorkspaceID
	ProposalID  identity.ProposalID
	Actor       string
	TraceID     string
}

// Submit walks draft -> proposed, freezing the change-set, and commits the
// proposal state change together with its governance.proposal.submitted audit
// fact and proposal.changed outbox event in one transaction.
func (service *ProposalService) Submit(ctx context.Context, request SubmitRequest) (governance.Proposal, error) {
	request.Actor = strings.TrimSpace(request.Actor)
	if request.WorkspaceID.IsZero() || request.ProposalID.IsZero() || request.Actor == "" {
		return governance.Proposal{}, governance.ErrInvalidArgument
	}
	events, err := newEventIDs()
	if err != nil {
		return governance.Proposal{}, err
	}
	return service.repository.SubmitProposal(ctx, ProposalSubmitCommand{
		WorkspaceID: request.WorkspaceID, ProposalID: request.ProposalID,
		Actor: request.Actor, SubmittedAt: service.clock.Now().UTC(),
		TraceID: request.TraceID, AuditEventID: events.AuditEventID, OutboxEventID: events.OutboxEventID,
	})
}

type TransitionRequest struct {
	WorkspaceID identity.WorkspaceID
	ProposalID  identity.ProposalID
	To          governance.ProposalState
	Actor       string
	TraceID     string
}

// Transition applies a §7.5 state-machine step. released is reached only
// through the ReleaseService cut transaction, so the application refuses that
// edge here with the domain transition error.
func (service *ProposalService) Transition(ctx context.Context, request TransitionRequest) (governance.Proposal, error) {
	request.Actor = strings.TrimSpace(request.Actor)
	if request.WorkspaceID.IsZero() || request.ProposalID.IsZero() || request.Actor == "" ||
		!request.To.Valid() || request.To == governance.ProposalDraft {
		return governance.Proposal{}, governance.ErrInvalidArgument
	}
	if request.To == governance.ProposalReleased {
		return governance.Proposal{}, fmt.Errorf(
			"%w: released is reached only through the release cut transaction", governance.ErrInvalidTransition)
	}
	current, err := service.repository.GetProposal(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return governance.Proposal{}, err
	}
	if err := governance.CheckProposalTransition(current.State, request.To); err != nil {
		return governance.Proposal{}, err
	}
	events, err := newEventIDs()
	if err != nil {
		return governance.Proposal{}, err
	}
	now := service.clock.Now().UTC()
	command := ProposalTransitionCommand{
		WorkspaceID: request.WorkspaceID, ProposalID: request.ProposalID, To: request.To,
		Actor: request.Actor, UpdatedAt: now, TraceID: request.TraceID,
		AuditEventID: events.AuditEventID, OutboxEventID: events.OutboxEventID,
	}
	if request.To == governance.ProposalRejected {
		command.DecidedAt = &now
	}
	return service.repository.TransitionProposal(ctx, command)
}

type RecordReviewRequest struct {
	WorkspaceID         identity.WorkspaceID
	ProposalID          identity.ProposalID
	ReviewerPrincipalID identity.PrincipalID
	Channel             governance.ReviewChannel
	Decision            governance.ReviewDecision
	Note                string
}

// RecordReview persists an immutable §8.4 review fact; it does not move the
// proposal state on its own.
func (service *ProposalService) RecordReview(ctx context.Context, request RecordReviewRequest) (governance.Review, error) {
	review := governance.Review{
		WorkspaceID: request.WorkspaceID, ProposalID: request.ProposalID,
		ReviewerPrincipalID: request.ReviewerPrincipalID, Channel: request.Channel,
		Decision: request.Decision, Note: request.Note,
	}
	if err := review.Validate(); err != nil {
		return governance.Review{}, err
	}
	reviewID, err := identity.NewReviewID()
	if err != nil {
		return governance.Review{}, fmt.Errorf("mint review ID: %w", err)
	}
	review.ID = reviewID
	review.CreatedAt = service.clock.Now().UTC()
	return service.repository.CreateReview(ctx, review)
}
