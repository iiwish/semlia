package governance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// ReviewCommandDecision is the wire vocabulary of a human review command:
// approve records an immutable review and keeps the proposal in_review
// (approvals gate the T007 release cut); reject records the review and
// transitions the proposal to rejected with decided_at.
type ReviewCommandDecision string

const (
	ReviewCommandApprove ReviewCommandDecision = "approve"
	ReviewCommandReject  ReviewCommandDecision = "reject"
)

// Outcome maps the command decision onto the immutable review fact and the
// proposal transition it implies (nil state means "no transition"). This is
// the one shared review-recording rule for the expert and the batch path:
// the batch confirm loop applies exactly this mapping per member.
func (decision ReviewCommandDecision) Outcome() (domain.ReviewDecision, *domain.ProposalState, error) {
	switch decision {
	case ReviewCommandApprove:
		return domain.ReviewApproved, nil, nil
	case ReviewCommandReject:
		rejected := domain.ProposalRejected
		return domain.ReviewRejected, &rejected, nil
	default:
		return "", nil, fmt.Errorf("%w: review decision %q", domain.ErrInvalidArgument, decision)
	}
}

// SeparationOfDutyError is the FR-007 hard block served with a stable,
// explainable denial: it names the conflict, the affected scope, the policy
// source and the permitted recovery actions. It is audited as an immutable
// governance.review.denied fact before it reaches the wire.
type SeparationOfDutyError struct {
	Conflict   string
	Scope      string
	Source     string
	Recovery   []string
	Conflicted []identity.ProposalID
}

func (err *SeparationOfDutyError) Error() string {
	return "separation of duties: " + err.Conflict
}

// SeparationOfDutyPolicySource is the stable policy-source reference every
// FR-007 denial carries.
const SeparationOfDutyPolicySource = "docs/SSOT.md §8.4 + docs/specs/access-control/product-design.md FR-007"

const (
	recoveryExpertReview = "Ask a different principal holding proposal.review to record the review."
	recoveryRebuildBatch = "Re-create the batch without the conflicting proposals and confirm it again."
	recoveryExpertMember = "Route each conflicting proposal through the expert review channel with a different reviewer."
)

func NewAuthorSeparationOfDutyError(workspace identity.WorkspaceID) *SeparationOfDutyError {
	return &SeparationOfDutyError{
		Conflict: "the proposal author cannot review their own proposal",
		Scope:    workspace.String(),
		Source:   SeparationOfDutyPolicySource,
		Recovery: []string{recoveryExpertReview},
	}
}

// BatchEligibleProposal pairs an in_review proposal with its latest policy
// decision; batch assembly consumes exactly this shape.
type BatchEligibleProposal struct {
	Proposal domain.Proposal
	Decision domain.PolicyDecision
}

// ReviewBatchAssembly is one fully-computed batch handed to the repository:
// the batch row plus its frozen member snapshots and the pre-minted audit
// event, committed in one transaction.
type ReviewBatchAssembly struct {
	Batch        domain.ReviewBatchRecord
	Members      []domain.ReviewBatchMember
	AuditEventID identity.EventID
	Actor        string
	TraceID      string
}

// ReviewBatchCursor is the decoded keyset position of an open-batches page.
type ReviewBatchCursor struct {
	CreatedAt time.Time
	ID        identity.ReviewBatchID
}

// ReviewDenialRecord is the immutable audit fact of one FR-007 refusal.
type ReviewDenialRecord struct {
	WorkspaceID  identity.WorkspaceID
	Actor        string
	Conflict     string
	Scope        string
	PolicySource string
	Recovery     []string
	Proposals    []identity.ProposalID
	AuditEventID identity.EventID
	TraceID      string
	CreatedAt    time.Time
}

// ConfirmMemberState is one locked member with its current proposal state and
// the latest policy decision observed inside the confirm transaction.
type ConfirmMemberState struct {
	Member         domain.ReviewBatchMember
	Proposal       domain.Proposal
	LatestDecision *domain.PolicyDecision
}

// ConfirmMemberOutcome is the confirm verdict for one member: either the
// member splits out (never mixed into the batch decision, §8.4) or the batch
// decision applies to it.
type ConfirmMemberOutcome struct {
	Split       bool
	SplitReason string
}

// ConfirmReviewBatchCommand carries the confirm transaction: the reviewer of
// record, the pre-minted per-member event identities and the pure Decide
// policy evaluated over locked state inside the transaction.
type ConfirmReviewBatchCommand struct {
	WorkspaceID         identity.WorkspaceID
	BatchID             identity.ReviewBatchID
	ReviewerPrincipalID identity.PrincipalID
	Reviewer            string
	Decision            domain.ReviewDecision
	Reason              string
	DecidedAt           time.Time
	TraceID             string
	MemberEvents        map[identity.ProposalID]EventIDs
	BatchAuditEventID   identity.EventID
	Decide              func(ConfirmMemberState) (ConfirmMemberOutcome, error)
}

// ReviewBatchRepository is the persistence contract of the T006 batch
// surface. Implementations commit every batch mutation with its audit fact in
// one transaction.
type ReviewBatchRepository interface {
	CreateReviewBatches(ctx context.Context, assemblies []ReviewBatchAssembly) ([]ReviewBatchRows, error)
	GetReviewBatchWithMembers(ctx context.Context, workspace identity.WorkspaceID, batch identity.ReviewBatchID) (ReviewBatchRows, error)
	ListOpenReviewBatches(ctx context.Context, workspace identity.WorkspaceID, limit int, cursor *ReviewBatchCursor) ([]ReviewBatchRows, error)
	ListBatchEligibleProposals(ctx context.Context, workspace identity.WorkspaceID) ([]BatchEligibleProposal, error)
	ConfirmReviewBatch(ctx context.Context, command ConfirmReviewBatchCommand) (ReviewBatchRows, error)
	CountProposalReviews(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID, reviewer identity.PrincipalID, channel domain.ReviewChannel) (int, error)
	RecordReviewDenial(ctx context.Context, denial ReviewDenialRecord) error
}

// ReviewBatchRows is the raw batch aggregate read model before the §8.4
// projections (samples, exclusions, max-risk member) are derived.
type ReviewBatchRows struct {
	Batch   domain.ReviewBatchRecord
	Members []domain.ReviewBatchMember
}

// DecisionRefresher re-runs the idempotent §8.2 decision step for one
// proposal. Batch confirm re-checks every member's CURRENT decision through
// it before applying anything, so an escalation recorded after the proposal
// entered review is seen at confirm time.
type DecisionRefresher interface {
	EnsureDecisionForProposal(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) (domain.PolicyDecision, error)
}

// WithDecisionRefresher attaches the decision re-check used by batch confirm.
// Without it the confirm falls back to the latest persisted decision.
func WithDecisionRefresher(refresher DecisionRefresher) func(*AuthoringService) {
	return func(service *AuthoringService) { service.decisions = refresher }
}

type ReviewProposalRequest struct {
	WorkspaceID  identity.WorkspaceID
	ProposalID   identity.ProposalID
	Decision     ReviewCommandDecision
	Reason       string
	PrincipalRef string
	TraceID      string
}

// ReviewOutcome is the read model of one recorded expert review: the
// immutable review fact plus the proposal in its post-command state.
type ReviewOutcome struct {
	Proposal domain.Proposal
	Review   domain.Review
}

// ReviewProposal is the expert review channel: server-side proposal.review
// capability, the FR-007 author block, one immutable review per reviewer per
// decision stage, and the T002 transition on rejection. Approvals never move
// the proposal out of in_review.
func (service *AuthoringService) ReviewProposal(ctx context.Context, request ReviewProposalRequest) (ReviewOutcome, error) {
	request.Reason = strings.TrimSpace(request.Reason)
	if request.WorkspaceID.IsZero() || request.ProposalID.IsZero() ||
		len(request.Reason) == 0 || len(request.Reason) > 4096 {
		return ReviewOutcome{}, domain.ErrInvalidArgument
	}
	decision, err := service.authorizeReview(ctx, request.WorkspaceID, request.PrincipalRef, request.TraceID)
	if err != nil {
		return ReviewOutcome{}, err
	}
	proposal, err := service.proposals.GetProposal(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return ReviewOutcome{}, err
	}
	if proposal.State != domain.ProposalInReview {
		return ReviewOutcome{}, fmt.Errorf("%w: proposal is %s, review is no longer applicable",
			domain.ErrConflict, proposal.State)
	}
	reviewer := decision.PrincipalID.String()
	if proposal.CreatedBy == reviewer {
		denial := NewAuthorSeparationOfDutyError(request.WorkspaceID)
		if recordErr := service.recordReviewDenial(ctx, request.WorkspaceID, reviewer, denial, nil, request.TraceID); recordErr != nil {
			return ReviewOutcome{}, recordErr
		}
		return ReviewOutcome{}, denial
	}
	outcome, err := service.recordExpertReview(ctx, recordReviewRequest{
		WorkspaceID: request.WorkspaceID, ProposalID: request.ProposalID,
		Reviewer: decision.PrincipalID, Channel: domain.ReviewExpert,
		Decision: request.Decision, Reason: request.Reason, TraceID: request.TraceID,
	})
	if err != nil {
		return ReviewOutcome{}, err
	}
	return outcome, nil
}

type recordReviewRequest struct {
	WorkspaceID identity.WorkspaceID
	ProposalID  identity.ProposalID
	Reviewer    identity.PrincipalID
	Channel     domain.ReviewChannel
	Decision    ReviewCommandDecision
	Reason      string
	TraceID     string
}

// recordExpertReview is the shared expert-path review recorder: duplicate
// reviews by the same reviewer for the same decision stage are refused, the
// immutable review row is written through the T002 RecordReview fact, and the
// rejection transition walks the §7.5 state machine with decided_at.
func (service *AuthoringService) recordExpertReview(ctx context.Context, request recordReviewRequest) (ReviewOutcome, error) {
	reviewDecision, transition, err := request.Decision.Outcome()
	if err != nil {
		return ReviewOutcome{}, err
	}
	existing, err := service.reviewBatches().CountProposalReviews(
		ctx, request.WorkspaceID, request.ProposalID, request.Reviewer, request.Channel)
	if err != nil {
		return ReviewOutcome{}, err
	}
	if existing > 0 {
		return ReviewOutcome{}, fmt.Errorf(
			"%w: reviewer %s already recorded a %s review for this decision stage",
			domain.ErrConflict, request.Reviewer.String(), request.Channel)
	}
	proposal, err := service.proposals.GetProposal(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return ReviewOutcome{}, err
	}
	review, err := service.proposals.RecordReview(ctx, RecordReviewRequest{
		WorkspaceID: request.WorkspaceID, ProposalID: request.ProposalID,
		ReviewerPrincipalID: request.Reviewer, Channel: request.Channel,
		Decision: reviewDecision, Note: request.Reason,
	})
	if err != nil {
		return ReviewOutcome{}, err
	}
	if transition != nil {
		proposal, err = service.proposals.Transition(ctx, TransitionRequest{
			WorkspaceID: request.WorkspaceID, ProposalID: request.ProposalID,
			To: *transition, Actor: request.Reviewer.String(), TraceID: request.TraceID,
		})
		if err != nil {
			return ReviewOutcome{}, err
		}
	}
	return ReviewOutcome{Proposal: proposal, Review: review}, nil
}

type CreateReviewBatchesRequest struct {
	WorkspaceID  identity.WorkspaceID
	PrincipalRef string
	TraceID      string
}

// CreateReviewBatches assembles the §8.4 batch confirmation queue: every
// in_review proposal whose latest decision routes to batch and that is not
// already an active member of an open batch is clustered by (target object
// type, dominant diff category, matched rule). Each cluster becomes one open
// batch whose grouping rule, policy version, member snapshots, samples and
// added reasons persist in one transaction — the audit record is the data.
func (service *AuthoringService) CreateReviewBatches(ctx context.Context, request CreateReviewBatchesRequest) ([]domain.ReviewBatchDetail, error) {
	decision, err := service.authorizeReview(ctx, request.WorkspaceID, request.PrincipalRef, request.TraceID)
	if err != nil {
		return nil, err
	}
	eligible, err := service.reviewBatches().ListBatchEligibleProposals(ctx, request.WorkspaceID)
	if err != nil {
		return nil, err
	}
	assemblies, err := AssembleReviewBatches(eligible, decision.PrincipalID.String(), service.clock.Now().UTC(), request.TraceID)
	if err != nil {
		return nil, err
	}
	if len(assemblies) == 0 {
		return []domain.ReviewBatchDetail{}, nil
	}
	rows, err := service.reviewBatches().CreateReviewBatches(ctx, assemblies)
	if err != nil {
		return nil, err
	}
	details := make([]domain.ReviewBatchDetail, 0, len(rows))
	for _, row := range rows {
		details = append(details, deriveReviewBatchDetail(row.Batch, row.Members))
	}
	return details, nil
}

// AssembleReviewBatches is the pure clustering function: deterministic group
// order (target type, diff category, matched rule) and deterministic member
// order (created_at, proposal id), with the first SampleCount members of each
// cluster marked as representative samples.
func AssembleReviewBatches(
	eligible []BatchEligibleProposal, createdBy string, createdAt time.Time, traceID string,
) ([]ReviewBatchAssembly, error) {
	type cluster struct {
		key struct {
			targetType  domain.TargetObjectType
			category    domain.DiffCategory
			matchedRule string
			policyVer   string
		}
		members []BatchEligibleProposal
	}
	clusters := map[[4]string]*cluster{}
	for _, candidate := range eligible {
		if candidate.Proposal.State != domain.ProposalInReview ||
			candidate.Decision.Routing != domain.RoutingBatch {
			continue
		}
		inputs, err := domain.ParseDecisionInputs(candidate.Decision.Inputs)
		if err != nil {
			return nil, fmt.Errorf("decode decision inputs of proposal %s: %w", candidate.Proposal.ID, err)
		}
		var key [4]string
		key[0] = string(candidate.Proposal.TargetObjectType)
		key[1] = string(domain.DominantDiffCategory(inputs))
		key[2] = candidate.Decision.MatchedPolicy
		key[3] = candidate.Decision.RuleVersion
		clusterEntry := clusters[key]
		if clusterEntry == nil {
			clusterEntry = &cluster{}
			clusterEntry.key.targetType = candidate.Proposal.TargetObjectType
			clusterEntry.key.category = domain.DiffCategory(key[1])
			clusterEntry.key.matchedRule = key[2]
			clusterEntry.key.policyVer = key[3]
			clusters[key] = clusterEntry
		}
		clusterEntry.members = append(clusterEntry.members, candidate)
	}
	keys := make([][4]string, 0, len(clusters))
	for key := range clusters {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		for index := range keys[i] {
			if keys[i][index] != keys[j][index] {
				return keys[i][index] < keys[j][index]
			}
		}
		return false
	})
	assemblies := make([]ReviewBatchAssembly, 0, len(clusters))
	for _, key := range keys {
		clusterEntry := clusters[key]
		members := clusterEntry.members
		sort.Slice(members, func(i, j int) bool {
			if !members[i].Proposal.CreatedAt.Equal(members[j].Proposal.CreatedAt) {
				return members[i].Proposal.CreatedAt.Before(members[j].Proposal.CreatedAt)
			}
			return members[i].Proposal.ID.String() < members[j].Proposal.ID.String()
		})
		batchID, err := identity.NewReviewBatchID()
		if err != nil {
			return nil, fmt.Errorf("mint review batch ID: %w", err)
		}
		groupingRule := domain.ReviewGroupingRule{
			TargetObjectType: clusterEntry.key.targetType,
			DiffCategory:     clusterEntry.key.category,
			MatchedRuleID:    clusterEntry.key.matchedRule,
		}
		if err := groupingRule.Validate(); err != nil {
			return nil, err
		}
		batch := domain.ReviewBatchRecord{
			ID: batchID, WorkspaceID: members[0].Proposal.WorkspaceID,
			GroupingRule: groupingRule, PolicyVersion: clusterEntry.key.policyVer,
			Status: domain.ReviewBatchOpen, CreatedBy: createdBy,
			CreatedAt: createdAt,
		}
		memberRows := make([]domain.ReviewBatchMember, 0, len(members))
		for index, member := range members {
			addedReason := domain.ReviewAddedReason{
				MatchedRuleID: member.Decision.MatchedPolicy, RiskLevel: member.Decision.RiskLevel,
				ReasonCode: member.Decision.ReasonCode, RuleVersion: member.Decision.RuleVersion,
				InputsDigest: member.Decision.InputsDigest,
			}
			encodedReason, err := addedReason.Marshal()
			if err != nil {
				return nil, err
			}
			memberRows = append(memberRows, domain.ReviewBatchMember{
				BatchID: batchID, WorkspaceID: batch.WorkspaceID,
				ProposalID: member.Proposal.ID, AddedReason: encodedReason,
				Sample: index < domain.SampleCount, CreatedAt: batch.CreatedAt,
			})
		}
		auditEventID, err := identity.NewEventID()
		if err != nil {
			return nil, fmt.Errorf("mint review batch audit event ID: %w", err)
		}
		assemblies = append(assemblies, ReviewBatchAssembly{
			Batch: batch, Members: memberRows, AuditEventID: auditEventID,
			Actor: createdBy, TraceID: traceID,
		})
	}
	return assemblies, nil
}

type GetReviewBatchRequest struct {
	WorkspaceID  identity.WorkspaceID
	BatchID      identity.ReviewBatchID
	PrincipalRef string
	TraceID      string
}

// GetReviewBatch reads one batch with its full §8.4 audit record: members,
// representative samples, exclusions and the max-risk member.
func (service *AuthoringService) GetReviewBatch(ctx context.Context, request GetReviewBatchRequest) (domain.ReviewBatchDetail, error) {
	if _, err := service.authorizeReview(ctx, request.WorkspaceID, request.PrincipalRef, request.TraceID); err != nil {
		return domain.ReviewBatchDetail{}, err
	}
	if request.BatchID.IsZero() {
		return domain.ReviewBatchDetail{}, domain.ErrInvalidArgument
	}
	rows, err := service.reviewBatches().GetReviewBatchWithMembers(ctx, request.WorkspaceID, request.BatchID)
	if err != nil {
		return domain.ReviewBatchDetail{}, err
	}
	return deriveReviewBatchDetail(rows.Batch, rows.Members), nil
}

type ListReviewBatchesRequest struct {
	WorkspaceID  identity.WorkspaceID
	Limit        int
	Cursor       string
	PrincipalRef string
	TraceID      string
}

type ReviewBatchPage struct {
	Items      []domain.ReviewBatchDetail
	Limit      int
	NextCursor string
}

// ListOpenReviewBatches pages the workspace's undecided batches newest first
// with the T003 keyset cursor.
func (service *AuthoringService) ListOpenReviewBatches(ctx context.Context, request ListReviewBatchesRequest) (ReviewBatchPage, error) {
	if _, err := service.authorizeReview(ctx, request.WorkspaceID, request.PrincipalRef, request.TraceID); err != nil {
		return ReviewBatchPage{}, err
	}
	limit, err := normalizeProposalLimit(request.Limit)
	if err != nil {
		return ReviewBatchPage{}, err
	}
	cursor, err := decodeReviewBatchCursor(request.Cursor)
	if err != nil {
		return ReviewBatchPage{}, err
	}
	rows, err := service.reviewBatches().ListOpenReviewBatches(ctx, request.WorkspaceID, limit+1, cursor)
	if err != nil {
		return ReviewBatchPage{}, err
	}
	page := ReviewBatchPage{Limit: limit}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		if page.NextCursor, err = encodeReviewBatchCursor(last.Batch); err != nil {
			return ReviewBatchPage{}, err
		}
	}
	page.Items = make([]domain.ReviewBatchDetail, 0, len(rows))
	for _, row := range rows {
		page.Items = append(page.Items, deriveReviewBatchDetail(row.Batch, row.Members))
	}
	return page, nil
}

type ConfirmReviewBatchRequest struct {
	WorkspaceID  identity.WorkspaceID
	BatchID      identity.ReviewBatchID
	Decision     ReviewCommandDecision
	Reason       string
	PrincipalRef string
	TraceID      string
}

type ConfirmReviewBatchResult struct {
	Detail  domain.ReviewBatchDetail
	Applied []identity.ProposalID
	Split   []identity.ProposalID
}

// ConfirmReviewBatch applies the operator decision to one open batch. Before
// anything persists it re-checks every member's CURRENT policy decision and
// auto-splits members that are no longer in_review, lost their decision, or
// escalated (high risk or no longer batch-routed) — §8.4: high-risk items are
// never mixed into batch operations. The FR-007 author block refuses the
// confirmation naming the conflicting members; nothing persists on refusal.
// The decision itself records one immutable review per remaining member (the
// shared review-recording rule) plus the transition on rejection, and freezes
// the batch audit record (grouping rule, final membership, samples,
// exclusions with reasons, reviewer, policy version).
func (service *AuthoringService) ConfirmReviewBatch(ctx context.Context, request ConfirmReviewBatchRequest) (ConfirmReviewBatchResult, error) {
	request.Reason = strings.TrimSpace(request.Reason)
	if request.WorkspaceID.IsZero() || request.BatchID.IsZero() ||
		len(request.Reason) == 0 || len(request.Reason) > 4096 {
		return ConfirmReviewBatchResult{}, domain.ErrInvalidArgument
	}
	authorization, err := service.authorizeReview(ctx, request.WorkspaceID, request.PrincipalRef, request.TraceID)
	if err != nil {
		return ConfirmReviewBatchResult{}, err
	}
	rows, err := service.reviewBatches().GetReviewBatchWithMembers(ctx, request.WorkspaceID, request.BatchID)
	if err != nil {
		return ConfirmReviewBatchResult{}, err
	}
	if rows.Batch.Status != domain.ReviewBatchOpen {
		return ConfirmReviewBatchResult{}, fmt.Errorf(
			"%w: review batch is %s, confirm is no longer applicable", domain.ErrConflict, rows.Batch.Status)
	}
	reviewDecision, _, err := request.Decision.Outcome()
	if err != nil {
		return ConfirmReviewBatchResult{}, err
	}
	if err := service.refreshMemberDecisions(ctx, request.WorkspaceID, rows.Members); err != nil {
		return ConfirmReviewBatchResult{}, err
	}
	reviewer := authorization.PrincipalID.String()
	memberEvents := make(map[identity.ProposalID]EventIDs, len(rows.Members))
	for _, member := range rows.Members {
		events, eventErr := newEventIDs()
		if eventErr != nil {
			return ConfirmReviewBatchResult{}, eventErr
		}
		memberEvents[member.ProposalID] = events
	}
	batchAuditEventID, err := identity.NewEventID()
	if err != nil {
		return ConfirmReviewBatchResult{}, fmt.Errorf("mint review batch audit event ID: %w", err)
	}
	decide := func(state ConfirmMemberState) (ConfirmMemberOutcome, error) {
		return DecideBatchMemberOutcome(state, reviewer, request.WorkspaceID)
	}
	confirmed, err := service.reviewBatches().ConfirmReviewBatch(ctx, ConfirmReviewBatchCommand{
		WorkspaceID: request.WorkspaceID, BatchID: request.BatchID,
		ReviewerPrincipalID: authorization.PrincipalID, Reviewer: reviewer,
		Decision: reviewDecision, Reason: request.Reason,
		DecidedAt: service.clock.Now().UTC(), TraceID: request.TraceID,
		MemberEvents: memberEvents, BatchAuditEventID: batchAuditEventID, Decide: decide,
	})
	if err != nil {
		var denial *SeparationOfDutyError
		if errors.As(err, &denial) {
			if recordErr := service.recordReviewDenial(ctx, request.WorkspaceID, reviewer, denial, denial.Conflicted, request.TraceID); recordErr != nil {
				return ConfirmReviewBatchResult{}, recordErr
			}
		}
		return ConfirmReviewBatchResult{}, err
	}
	result := ConfirmReviewBatchResult{Detail: deriveReviewBatchDetail(confirmed.Batch, confirmed.Members)}
	for _, member := range confirmed.Members {
		if member.SplitOut {
			result.Split = append(result.Split, member.ProposalID)
		} else {
			result.Applied = append(result.Applied, member.ProposalID)
		}
	}
	return result, nil
}

// DecideBatchMemberOutcome is the pure confirm policy: split reasons are
// stable machine-readable strings and the FR-007 author block fires before
// any decision applies.
func DecideBatchMemberOutcome(
	state ConfirmMemberState, reviewer string, workspace identity.WorkspaceID,
) (ConfirmMemberOutcome, error) {
	if state.Proposal.State != domain.ProposalInReview {
		return ConfirmMemberOutcome{Split: true,
			SplitReason: "state_changed:" + string(state.Proposal.State)}, nil
	}
	if state.LatestDecision == nil {
		return ConfirmMemberOutcome{Split: true, SplitReason: "decision_missing"}, nil
	}
	if state.LatestDecision.RiskLevel == domain.RiskHigh {
		return ConfirmMemberOutcome{Split: true, SplitReason: "risk_escalated:" +
			string(state.LatestDecision.RiskLevel) + ":" + state.LatestDecision.ReasonCode}, nil
	}
	if state.LatestDecision.Routing != domain.RoutingBatch {
		return ConfirmMemberOutcome{Split: true, SplitReason: "routing_changed:" +
			string(state.LatestDecision.Routing)}, nil
	}
	if state.Proposal.CreatedBy == reviewer {
		return ConfirmMemberOutcome{}, &SeparationOfDutyError{
			Conflict:   "the reviewer authored a member of the batch and cannot confirm it",
			Scope:      workspace.String(),
			Source:     SeparationOfDutyPolicySource,
			Recovery:   []string{recoveryRebuildBatch, recoveryExpertMember},
			Conflicted: []identity.ProposalID{state.Member.ProposalID},
		}
	}
	return ConfirmMemberOutcome{}, nil
}

// refreshMemberDecisions re-runs the idempotent §8.2 decision step per member
// so the confirm re-check observes the CURRENT decision, not just the one
// recorded when the proposal entered review. Members that left in_review are
// skipped by the trigger itself; the in-transaction re-check still splits
// them out.
func (service *AuthoringService) refreshMemberDecisions(
	ctx context.Context, workspace identity.WorkspaceID, members []domain.ReviewBatchMember,
) error {
	if service.decisions == nil {
		return nil
	}
	for _, member := range members {
		if _, err := service.decisions.EnsureDecisionForProposal(ctx, workspace, member.ProposalID); err != nil {
			return err
		}
	}
	return nil
}

func (service *AuthoringService) recordReviewDenial(
	ctx context.Context, workspace identity.WorkspaceID, actor string,
	denial *SeparationOfDutyError, proposals []identity.ProposalID, traceID string,
) error {
	eventID, err := identity.NewEventID()
	if err != nil {
		return fmt.Errorf("mint review denial audit event ID: %w", err)
	}
	return service.reviewBatches().RecordReviewDenial(ctx, ReviewDenialRecord{
		WorkspaceID: workspace, Actor: actor, Conflict: denial.Conflict,
		Scope: denial.Scope, PolicySource: denial.Source, Recovery: denial.Recovery,
		Proposals: proposals, AuditEventID: eventID, TraceID: traceID,
		CreatedAt: service.clock.Now().UTC(),
	})
}

func (service *AuthoringService) authorizeReview(
	ctx context.Context, workspace identity.WorkspaceID, principalRef, traceID string,
) (authorization.Decision, error) {
	if service.authorizer == nil {
		return authorization.Decision{}, fmt.Errorf(
			"%w: review commands require the authorization evaluator", domain.ErrInvariant)
	}
	evaluation, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: principalRef, WorkspaceID: workspace,
		Action: authorization.ActionProposalReview,
		Resource: authorization.Resource{
			Type: authorization.ScopeWorkspace, ID: workspace.UUID(),
		},
		TraceID: traceID,
	})
	if err != nil {
		return authorization.Decision{}, err
	}
	if !evaluation.Allowed {
		return evaluation, &authorization.DenialError{Decision: evaluation}
	}
	return evaluation, nil
}

// reviewBatches returns the batch repository wired from the same store; a
// store that cannot persist batches makes every batch command a stable
// invariant failure instead of a silent no-op.
func (service *AuthoringService) reviewBatches() ReviewBatchRepository {
	if service.batchRepository == nil {
		panic("governance store does not implement ReviewBatchRepository")
	}
	return service.batchRepository
}

// deriveReviewBatchDetail projects the persisted batch rows onto the §8.4
// read model: samples are the persisted sample flags, exclusions are the
// split members, and the max-risk member is picked by the frozen added-reason
// risk ranks (ties resolve to the earliest member by assembly sequence).
func deriveReviewBatchDetail(batch domain.ReviewBatchRecord, members []domain.ReviewBatchMember) domain.ReviewBatchDetail {
	detail := domain.ReviewBatchDetail{Batch: batch, Members: members}
	maxRiskIdx := -1
	maxRiskRank := 0
	for index, member := range members {
		if member.Sample {
			detail.Samples = append(detail.Samples, member)
		}
		if member.SplitOut {
			detail.Exclusions = append(detail.Exclusions, member)
		}
		if rank := addedReasonRiskRank(member.AddedReason); rank > maxRiskRank {
			maxRiskRank = rank
			maxRiskIdx = index
		}
	}
	if maxRiskIdx >= 0 {
		detail.MaxRiskMember = &detail.Members[maxRiskIdx]
	}
	return detail
}

func addedReasonRiskRank(raw json.RawMessage) int {
	reason, err := domain.ParseReviewAddedReason(raw)
	if err != nil {
		return 0
	}
	return reason.RiskLevel.RiskRank()
}

func encodeReviewBatchCursor(last domain.ReviewBatchRecord) (string, error) {
	encoded, err := json.Marshal(proposalCursorEnvelope{
		Kind: "review-batch", CreatedAt: last.CreatedAt.UTC(), ID: last.ID.String(),
	})
	if err != nil {
		return "", fmt.Errorf("encode review batch cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeReviewBatchCursor(value string) (*ReviewBatchCursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%w: review batch cursor", domain.ErrInvalidArgument)
	}
	var envelope proposalCursorEnvelope
	if err := json.Unmarshal(decoded, &envelope); err != nil || envelope.Kind != "review-batch" ||
		envelope.CreatedAt.IsZero() {
		return nil, fmt.Errorf("%w: review batch cursor", domain.ErrInvalidArgument)
	}
	batchID, err := identity.ParseReviewBatchID(envelope.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: review batch cursor", domain.ErrInvalidArgument)
	}
	return &ReviewBatchCursor{CreatedAt: envelope.CreatedAt.UTC(), ID: batchID}, nil
}
