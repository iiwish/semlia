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

// ErrNoSubstantiveChange rejects proposals whose change-set normalizes to
// nothing: every update carries equal before/after digests and no add or
// remove remains. SSOT §8.2 keeps such results out of the human queue, so the
// authoring service refuses them at create and re-checks at submit before the
// draft can reach proposed.
var ErrNoSubstantiveChange = errors.New("proposal change-set contains no substantive change")

const (
	defaultProposalLimit = 50
	maxProposalLimit     = 200
)

// AuthoringRepository is the persistence contract of the T003 authoring
// surface: one transactional draft creation, workspace-scoped keyset listing
// and the semantic-asset target check backing proposal create.
type AuthoringRepository interface {
	CreateProposalWithChanges(ctx context.Context, proposal domain.Proposal, items []domain.ChangeSetItem) (domain.Proposal, []domain.ChangeSetItem, error)
	ListProposals(ctx context.Context, workspace identity.WorkspaceID, limit int, cursor *ProposalCursor) ([]domain.Proposal, error)
	VerifyProposalTarget(ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID, revision identity.RevisionID) error
	ListProposalValidationRuns(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) ([]domain.ValidationRun, error)
	ListRunResults(ctx context.Context, workspace identity.WorkspaceID, run identity.ValidationRunID) ([]domain.ValidationResult, error)
	GetLatestProposalPolicyDecision(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) (domain.PolicyDecision, error)
	GetPolicyRule(ctx context.Context, ruleVersion, ruleID string) (domain.PolicyRule, error)
}

// ProposalCursor is the decoded keyset position of a proposals page.
type ProposalCursor struct {
	CreatedAt time.Time
	ID        identity.ProposalID
}

// AgentAttribution references the persisted §8.6 agent run that produced the
// payload. The contract is a pre-created run: the authoring service validates
// that the run exists in the workspace and that the model, config revision and
// input hash match the persisted record before attributing the proposal.
type AgentAttribution struct {
	AgentRunID     identity.AgentRunID
	Model          string
	ConfigRevision string
	InputHash      string
}

// ChangeSetItemInput is one wire change-set entry before identity minting.
type ChangeSetItemInput struct {
	FieldPath    string
	Op           domain.ChangeOp
	BeforeDigest string
	AfterDigest  string
	BeforeValue  json.RawMessage
	AfterValue   json.RawMessage
}

type CreateAuthoringProposalRequest struct {
	WorkspaceID      identity.WorkspaceID
	TargetType       domain.TargetObjectType
	TargetObjectID   string
	BaseRevisionID   identity.RevisionID
	Title            string
	Summary          string
	Reason           string
	ChangeSet        []ChangeSetItemInput
	AgentAttribution *AgentAttribution
	CreatedBy        string
	PrincipalRef     string
	TraceID          string
}

type SubmitAuthoringProposalRequest struct {
	WorkspaceID  identity.WorkspaceID
	ProposalID   identity.ProposalID
	PrincipalRef string
	TraceID      string
}

type GetAuthoringProposalRequest struct {
	WorkspaceID  identity.WorkspaceID
	ProposalID   identity.ProposalID
	PrincipalRef string
	TraceID      string
}

type ListAuthoringProposalsRequest struct {
	WorkspaceID  identity.WorkspaceID
	Limit        int
	Cursor       string
	PrincipalRef string
	TraceID      string
}

// ProposalDetail is the read model of one proposal aggregate: the proposal
// with its target expressed as the wire TypeID, plus the frozen-or-editable
// change-set. Review facts stay absent until the review task ships them.
type ProposalDetail struct {
	Proposal           domain.Proposal
	TargetObjectType   domain.TargetObjectType
	TargetObjectTypeID string
	Changes            []domain.ChangeSetItem
}

type ProposalPage struct {
	Items      []domain.Proposal
	Limit      int
	NextCursor string
}

type AuthoringService struct {
	repository      AuthoringRepository
	batchRepository ReviewBatchRepository
	proposals       *ProposalService
	agentRuns       *AgentRunService
	authorizer      authorizationapp.Evaluator
	clock           Clock
	validation      *ValidationOrchestrator
	decisions       DecisionRefresher
}

type AuthoringOption func(*AuthoringService)

// WithValidationOrchestrator attaches the T004 validation orchestration:
// submit walks the proposal into validating and enqueues exactly one
// deterministic validation job. Submit fails closed without it.
func WithValidationOrchestrator(orchestrator *ValidationOrchestrator) AuthoringOption {
	return func(service *AuthoringService) { service.validation = orchestrator }
}

func NewAuthoringService(
	repository AuthoringRepository,
	proposals *ProposalService,
	agentRuns *AgentRunService,
	authorizer authorizationapp.Evaluator,
	clock Clock,
	options ...AuthoringOption,
) *AuthoringService {
	if repository == nil || proposals == nil || agentRuns == nil || clock == nil {
		panic("authoring repository, proposal service, agent run service and clock are required")
	}
	service := &AuthoringService{
		repository: repository, proposals: proposals, agentRuns: agentRuns,
		authorizer: authorizer, clock: clock,
	}
	if batches, ok := repository.(ReviewBatchRepository); ok {
		service.batchRepository = batches
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

// CreateProposal opens a governed draft: target resolution, §8.6 agent-run
// attribution validation, server-side capability evaluation, the
// no-substantive-change gate and one transactional write of the proposal plus
// its change-set. Nothing persists before every gate passes.
func (service *AuthoringService) CreateProposal(ctx context.Context, request CreateAuthoringProposalRequest) (ProposalDetail, error) {
	request.Title = strings.TrimSpace(request.Title)
	request.Summary = strings.TrimSpace(request.Summary)
	request.Reason = strings.TrimSpace(request.Reason)
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if request.WorkspaceID.IsZero() || request.Title == "" || len(request.Title) > 256 ||
		len(request.Summary) > 4096 || len(request.Reason) > 4096 ||
		len(request.CreatedBy) > 256 || len(request.ChangeSet) > 100 {
		return ProposalDetail{}, domain.ErrInvalidArgument
	}
	targetUUID, err := service.resolveTarget(ctx, request)
	if err != nil {
		return ProposalDetail{}, err
	}
	attribution, err := service.resolveAttribution(ctx, request)
	if err != nil {
		return ProposalDetail{}, err
	}
	// §8.6 authorship of record: an agent-attributed proposal is authored by
	// its agent run, never by a client-claimed human identity.
	if attribution.RunID != nil {
		request.CreatedBy = attribution.CreatedBy
	} else if request.CreatedBy == "" {
		return ProposalDetail{}, domain.ErrInvalidArgument
	}
	if err := service.authorize(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: request.PrincipalRef,
		WorkspaceID:  request.WorkspaceID,
		Action:       authorization.ActionAssetPropose,
		Resource:     proposalResource(request.TargetType, targetUUID, request.WorkspaceID),
		TraceID:      request.TraceID,
	}); err != nil {
		return ProposalDetail{}, err
	}
	items, err := buildChangeSetItems(request.WorkspaceID, request.ChangeSet)
	if err != nil {
		return ProposalDetail{}, err
	}
	if !hasSubstantiveChange(items) {
		return ProposalDetail{}, ErrNoSubstantiveChange
	}
	proposalID, err := identity.NewProposalID()
	if err != nil {
		return ProposalDetail{}, fmt.Errorf("mint proposal ID: %w", err)
	}
	proposal := domain.Proposal{
		ID: proposalID, WorkspaceID: request.WorkspaceID,
		TargetObjectType: request.TargetType, TargetObjectID: targetUUID,
		State: domain.ProposalDraft, Title: request.Title, Summary: request.Summary,
		Reason: request.Reason, AgentRunID: attribution.RunID, CreatedBy: request.CreatedBy,
		CreatedAt: service.clock.Now().UTC(), UpdatedAt: service.clock.Now().UTC(),
	}
	if request.TargetType == domain.TargetSemanticAsset {
		assetID, parseErr := identity.ParseAssetID(request.TargetObjectID)
		if parseErr != nil {
			return ProposalDetail{}, domain.ErrInvalidArgument
		}
		proposal.AssetID = &assetID
		proposal.BaseRevisionID = &request.BaseRevisionID
	}
	created, changes, err := service.repository.CreateProposalWithChanges(ctx, proposal, items)
	if err != nil {
		return ProposalDetail{}, err
	}
	return ProposalDetail{
		Proposal: created, TargetObjectType: created.TargetObjectType,
		TargetObjectTypeID: request.TargetObjectID, Changes: changes,
	}, nil
}

// SubmitProposal re-checks the persisted change-set and walks draft ->
// proposed, then the T004 orchestration moves the proposal into validating
// and enqueues exactly one deterministic validation job. A change-set that
// normalizes to empty never enters the human queue (SSOT §8.2): the proposal
// stays a draft and the stable ErrNoSubstantiveChange error is returned
// without any state write. A crashed submit (proposed or validating state,
// job enqueue not yet completed) resumes its orchestration instead of
// failing, so a client retry cannot strand a proposal between states.
func (service *AuthoringService) SubmitProposal(ctx context.Context, request SubmitAuthoringProposalRequest) (ProposalDetail, error) {
	proposal, err := service.proposals.GetProposal(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return ProposalDetail{}, err
	}
	if err := service.authorize(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: request.PrincipalRef,
		WorkspaceID:  request.WorkspaceID,
		Action:       authorization.ActionAssetPropose,
		Resource:     proposalResource(proposal.TargetObjectType, proposal.TargetObjectID, request.WorkspaceID),
		TraceID:      request.TraceID,
	}); err != nil {
		return ProposalDetail{}, err
	}
	changes, err := service.proposals.ListChanges(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return ProposalDetail{}, err
	}
	if !hasSubstantiveChange(changes) {
		return ProposalDetail{}, ErrNoSubstantiveChange
	}
	actor := strings.TrimSpace(request.PrincipalRef)
	if actor == "" {
		actor = proposal.CreatedBy
	}
	switch proposal.State {
	case domain.ProposalDraft:
		proposal, err = service.proposals.Submit(ctx, SubmitRequest{
			WorkspaceID: request.WorkspaceID, ProposalID: request.ProposalID,
			Actor: actor, TraceID: request.TraceID,
		})
		if err != nil {
			return ProposalDetail{}, err
		}
	case domain.ProposalProposed, domain.ProposalValidating:
		// Resume the interrupted submit; the orchestrator is idempotent.
	default:
		return ProposalDetail{}, fmt.Errorf(
			"%w: proposal is %s, submit is no longer applicable",
			domain.ErrConflict, proposal.State)
	}
	if service.validation == nil {
		return ProposalDetail{}, fmt.Errorf(
			"%w: submit requires the validation orchestrator", domain.ErrInvariant)
	}
	submitted, err := service.validation.BeginValidation(ctx, BeginValidationRequest{
		WorkspaceID: request.WorkspaceID, ProposalID: request.ProposalID,
		Actor: actor, TraceID: request.TraceID,
	})
	if err != nil {
		return ProposalDetail{}, err
	}
	targetTypeID, err := ProposalTargetObjectTypeID(submitted)
	if err != nil {
		return ProposalDetail{}, err
	}
	return ProposalDetail{
		Proposal: submitted, TargetObjectType: submitted.TargetObjectType,
		TargetObjectTypeID: targetTypeID, Changes: changes,
	}, nil
}

// GetProposal reads one workspace-scoped proposal with its change-set.
func (service *AuthoringService) GetProposal(ctx context.Context, request GetAuthoringProposalRequest) (ProposalDetail, error) {
	if err := service.authorize(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: request.PrincipalRef,
		WorkspaceID:  request.WorkspaceID,
		Action:       authorization.ActionAssetRead,
		Resource:     authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()},
		TraceID:      request.TraceID,
	}); err != nil {
		return ProposalDetail{}, err
	}
	proposal, err := service.proposals.GetProposal(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return ProposalDetail{}, err
	}
	changes, err := service.proposals.ListChanges(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return ProposalDetail{}, err
	}
	targetTypeID, err := ProposalTargetObjectTypeID(proposal)
	if err != nil {
		return ProposalDetail{}, err
	}
	return ProposalDetail{
		Proposal: proposal, TargetObjectType: proposal.TargetObjectType,
		TargetObjectTypeID: targetTypeID, Changes: changes,
	}, nil
}

// ListProposals pages workspace-scoped proposals newest first with an opaque
// keyset cursor mirroring the M1 catalog list.
func (service *AuthoringService) ListProposals(ctx context.Context, request ListAuthoringProposalsRequest) (ProposalPage, error) {
	if err := service.authorize(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: request.PrincipalRef,
		WorkspaceID:  request.WorkspaceID,
		Action:       authorization.ActionAssetRead,
		Resource:     authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()},
		TraceID:      request.TraceID,
	}); err != nil {
		return ProposalPage{}, err
	}
	limit, err := normalizeProposalLimit(request.Limit)
	if err != nil {
		return ProposalPage{}, err
	}
	cursor, err := decodeProposalCursor(request.Cursor)
	if err != nil {
		return ProposalPage{}, err
	}
	items, err := service.repository.ListProposals(ctx, request.WorkspaceID, limit+1, cursor)
	if err != nil {
		return ProposalPage{}, err
	}
	page := ProposalPage{Items: items, Limit: limit}
	if len(items) > limit {
		last := items[limit-1]
		page.Items = items[:limit]
		if page.NextCursor, err = encodeProposalCursor(last); err != nil {
			return ProposalPage{}, err
		}
	}
	return page, nil
}

// PolicyDecisionDetail is the read model of one proposal's policy decision:
// the immutable decision fact plus the matched rule's explanation and the
// input categories that drove it. No opaque score exists anywhere in the
// surface (SSOT §8.3).
type PolicyDecisionDetail struct {
	Decision           domain.PolicyDecision
	Explanation        string
	MatchedInputFields []string
}

type GetPolicyDecisionRequest struct {
	WorkspaceID  identity.WorkspaceID
	ProposalID   identity.ProposalID
	PrincipalRef string
	TraceID      string
}

// GetPolicyDecision reads the latest policy decision of one proposal with
// its explainability context. A proposal without a decision (validation has
// not completed yet) reads as not-found.
func (service *AuthoringService) GetPolicyDecision(ctx context.Context, request GetPolicyDecisionRequest) (PolicyDecisionDetail, error) {
	if err := service.authorize(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: request.PrincipalRef,
		WorkspaceID:  request.WorkspaceID,
		Action:       authorization.ActionAssetRead,
		Resource:     authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()},
		TraceID:      request.TraceID,
	}); err != nil {
		return PolicyDecisionDetail{}, err
	}
	if _, err := service.proposals.GetProposal(ctx, request.WorkspaceID, request.ProposalID); err != nil {
		return PolicyDecisionDetail{}, err
	}
	decision, err := service.repository.GetLatestProposalPolicyDecision(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return PolicyDecisionDetail{}, err
	}
	detail := PolicyDecisionDetail{Decision: decision}
	if rule, ruleErr := service.repository.GetPolicyRule(ctx, decision.RuleVersion, decision.MatchedPolicy); ruleErr == nil {
		detail.Explanation = rule.Explanation
		if fields, fieldsErr := domain.PolicyRuleMatchedInputFields(rule.Match); fieldsErr == nil {
			detail.MatchedInputFields = fields
		}
	}
	return detail, nil
}

// ValidationRunRecord pairs one validation run with its recorded results.
type ValidationRunRecord struct {
	Run     domain.ValidationRun
	Results []domain.ValidationResult
}

type ListValidationRunsRequest struct {
	WorkspaceID  identity.WorkspaceID
	ProposalID   identity.ProposalID
	PrincipalRef string
	TraceID      string
}

// ListValidationRuns is the T004 read surface: the deterministic validation
// runs of one proposal with every recorded result, ordered by run start.
func (service *AuthoringService) ListValidationRuns(ctx context.Context, request ListValidationRunsRequest) ([]ValidationRunRecord, error) {
	if err := service.authorize(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: request.PrincipalRef,
		WorkspaceID:  request.WorkspaceID,
		Action:       authorization.ActionAssetRead,
		Resource:     authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()},
		TraceID:      request.TraceID,
	}); err != nil {
		return nil, err
	}
	if _, err := service.proposals.GetProposal(ctx, request.WorkspaceID, request.ProposalID); err != nil {
		return nil, err
	}
	runs, err := service.repository.ListProposalValidationRuns(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return nil, err
	}
	records := make([]ValidationRunRecord, 0, len(runs))
	for _, run := range runs {
		results, err := service.repository.ListRunResults(ctx, request.WorkspaceID, run.ID)
		if err != nil {
			return nil, err
		}
		records = append(records, ValidationRunRecord{Run: run, Results: results})
	}
	return records, nil
}

// resolveTarget validates the target coordinates without touching governed
// object existence: drafts may reference governed objects that appear before
// submission (T002 contract), while semantic-asset targets must already exist
// and the base revision must belong to the asset.
func (service *AuthoringService) resolveTarget(ctx context.Context, request CreateAuthoringProposalRequest) (string, error) {
	switch request.TargetType {
	case domain.TargetSemanticAsset:
		assetID, err := identity.ParseAssetID(request.TargetObjectID)
		if err != nil {
			return "", fmt.Errorf("%w: semantic asset target %q", domain.ErrInvalidArgument, request.TargetObjectID)
		}
		if request.BaseRevisionID.IsZero() {
			return "", fmt.Errorf("%w: semantic asset proposals require baseRevisionId", domain.ErrInvalidArgument)
		}
		if err := service.repository.VerifyProposalTarget(ctx, request.WorkspaceID, assetID, request.BaseRevisionID); err != nil {
			return "", err
		}
		return assetID.UUID(), nil
	default:
		if !request.TargetType.IsGovernedObject() {
			return "", fmt.Errorf("%w: unsupported proposal target type %q", domain.ErrInvalidArgument, request.TargetType)
		}
		return ParseGovernedObjectUUID(request.TargetType, request.TargetObjectID)
	}
}

// AttributionResult carries the validated agent-run reference and the
// authorship of record derived from it.
type AttributionResult struct {
	RunID     *identity.AgentRunID
	CreatedBy string
}

// resolveAttribution enforces the pre-created agent-run contract: the run
// must exist in the workspace and the attribution must match its persisted
// model, config revision and input hash, so a proposal can never be
// attributed to a run that did not produce its input (SSOT §8.6).
func (service *AuthoringService) resolveAttribution(ctx context.Context, request CreateAuthoringProposalRequest) (AttributionResult, error) {
	if request.AgentAttribution == nil {
		return AttributionResult{}, nil
	}
	attribution := *request.AgentAttribution
	if attribution.AgentRunID.IsZero() || attribution.Model == "" || attribution.ConfigRevision == "" ||
		!domain.IsValidContentDigest(attribution.InputHash) {
		return AttributionResult{}, fmt.Errorf("%w: incomplete agent attribution", domain.ErrInvalidArgument)
	}
	run, err := service.agentRuns.GetRun(ctx, request.WorkspaceID, attribution.AgentRunID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return AttributionResult{}, fmt.Errorf("%w: agent run %s not found in workspace", domain.ErrNotFound, attribution.AgentRunID.String())
		}
		return AttributionResult{}, err
	}
	if run.Model != attribution.Model || run.ConfigRevision != attribution.ConfigRevision ||
		run.InputHash != attribution.InputHash {
		return AttributionResult{}, fmt.Errorf("%w: agent attribution does not match run %s", domain.ErrInvalidArgument, attribution.AgentRunID.String())
	}
	if run.PrincipalID != nil {
		return AttributionResult{RunID: &run.ID, CreatedBy: run.PrincipalID.String()}, nil
	}
	return AttributionResult{RunID: &run.ID, CreatedBy: "agent-run:" + run.ID.String()}, nil
}

func (service *AuthoringService) authorize(ctx context.Context, request authorizationapp.EvaluationRequest) error {
	if service.authorizer == nil {
		return nil
	}
	decision, err := service.authorizer.Evaluate(ctx, request)
	if err != nil {
		return err
	}
	if decision.Allowed {
		return nil
	}
	return &authorization.DenialError{Decision: decision}
}

// proposalResource maps a proposal target into authorization scope
// coordinates: semantic assets authorize at asset scope, governance objects
// at their workspace scope (no object-level scope exists in the vocabulary).
func proposalResource(targetType domain.TargetObjectType, targetUUID string, workspace identity.WorkspaceID) authorization.Resource {
	if targetType == domain.TargetSemanticAsset {
		return authorization.Resource{Type: authorization.ScopeAsset, ID: targetUUID}
	}
	return authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}
}

func buildChangeSetItems(workspace identity.WorkspaceID, inputs []ChangeSetItemInput) ([]domain.ChangeSetItem, error) {
	items := make([]domain.ChangeSetItem, 0, len(inputs))
	for _, input := range inputs {
		item := domain.ChangeSetItem{
			WorkspaceID: workspace, FieldPath: input.FieldPath, Op: input.Op,
			BeforeDigest: input.BeforeDigest, AfterDigest: input.AfterDigest,
			BeforeValue: json.RawMessage(append([]byte(nil), input.BeforeValue...)),
			AfterValue:  json.RawMessage(append([]byte(nil), input.AfterValue...)),
		}
		if err := item.Validate(); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// hasSubstantiveChange implements the SSOT §8.2 gate: an update is
// substantive only when its digests differ; adds and removes always are. An
// empty change-set normalizes to nothing.
func hasSubstantiveChange(items []domain.ChangeSetItem) bool {
	for _, item := range items {
		switch item.Op {
		case domain.ChangeAdd, domain.ChangeRemove:
			return true
		case domain.ChangeUpdate:
			if item.BeforeDigest != item.AfterDigest {
				return true
			}
		}
	}
	return false
}

// ProposalTargetObjectTypeID converts the stored target coordinates back to the
// wire TypeID: storage UUIDs never leave the service boundary (ADR-0003).
func ProposalTargetObjectTypeID(proposal domain.Proposal) (string, error) {
	prefixes := map[domain.TargetObjectType]identity.Prefix{
		domain.TargetPhysicalBinding: identity.PhysicalBinding,
		domain.TargetModelGrain:      identity.ModelGrain,
		domain.TargetEntityKey:       identity.EntityKey,
		domain.TargetJoinContract:    identity.JoinContract,
	}
	if proposal.TargetObjectType == domain.TargetSemanticAsset {
		if proposal.AssetID == nil {
			return "", fmt.Errorf("%w: semantic asset proposal without asset reference", domain.ErrInvariant)
		}
		return proposal.AssetID.String(), nil
	}
	prefix, ok := prefixes[proposal.TargetObjectType]
	if !ok {
		return "", fmt.Errorf("%w: unsupported proposal target type %q", domain.ErrInvariant, proposal.TargetObjectType)
	}
	targetID, err := identity.FromUUID(prefix, proposal.TargetObjectID)
	if err != nil {
		return "", fmt.Errorf("%w: proposal target %q: %v", domain.ErrInvariant, proposal.TargetObjectID, err)
	}
	return targetID.String(), nil
}

func normalizeProposalLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultProposalLimit, nil
	}
	if limit < 1 || limit > maxProposalLimit {
		return 0, domain.ErrInvalidArgument
	}
	return limit, nil
}

type proposalCursorEnvelope struct {
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

func encodeProposalCursor(last domain.Proposal) (string, error) {
	encoded, err := json.Marshal(proposalCursorEnvelope{
		Kind: "proposal", CreatedAt: last.CreatedAt.UTC(), ID: last.ID.String(),
	})
	if err != nil {
		return "", fmt.Errorf("encode proposal cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeProposalCursor(value string) (*ProposalCursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%w: proposal cursor", domain.ErrInvalidArgument)
	}
	var envelope proposalCursorEnvelope
	if err := json.Unmarshal(decoded, &envelope); err != nil || envelope.Kind != "proposal" || envelope.CreatedAt.IsZero() {
		return nil, fmt.Errorf("%w: proposal cursor", domain.ErrInvalidArgument)
	}
	proposalID, err := identity.ParseProposalID(envelope.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: proposal cursor", domain.ErrInvalidArgument)
	}
	return &ProposalCursor{CreatedAt: envelope.CreatedAt.UTC(), ID: proposalID}, nil
}
