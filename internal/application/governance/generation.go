package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/application/governance/llm"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// GenerationRepository is the read substrate of live generation: the
// deterministic target snapshot, and the workspace's seeded agent principal
// (D6) that the run and its proposal attribute to.
type GenerationRepository interface {
	GenerationAssetSnapshot(ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID) (GenerationTargetSnapshot, error)
	GenerationObjectSnapshot(ctx context.Context, workspace identity.WorkspaceID, objectType domain.TargetObjectType, objectID string) (GenerationTargetSnapshot, error)
	WorkspaceAgentPrincipal(ctx context.Context, workspace identity.WorkspaceID) (authorization.Principal, error)
}

// GenerationTargetSnapshot is the deterministic, workspace-scoped input for
// one generation prompt. Content is stored data — the prompt itself is never
// persisted (SSOT §8.6: the input survives as its hash only).
type GenerationTargetSnapshot struct {
	TargetObjectType domain.TargetObjectType `json:"targetObjectType"`
	TargetObjectID   string                  `json:"targetObjectId"`
	BaseRevisionID   string                  `json:"baseRevisionId,omitempty"`
	ObjectVersion    int                     `json:"objectVersion,omitempty"`
	Address          string                  `json:"address"`
	AssetType        string                  `json:"assetType,omitempty"`
	Lifecycle        string                  `json:"lifecycle,omitempty"`
	SchemaVersion    string                  `json:"schemaVersion,omitempty"`
	Content          json.RawMessage         `json:"content"`
}

// GenerateProposalRequest opens one governed generation. One request is one
// agent run is at most one proposal: there are no implicit retries, so a
// retry is a new explicit request (new run, new proposal).
type GenerateProposalRequest struct {
	WorkspaceID      identity.WorkspaceID
	TargetObjectType domain.TargetObjectType
	TargetObjectID   string
	Instruction      string
	// ModelSettingID optionally pins the model; when empty the workspace's
	// enabled default llm setting is used.
	ModelSettingID *identity.ModelSettingID
	PrincipalRef   string
	TraceID        string
}

// GenerationResult reports the finished §8.6 run and the agent-attributed
// proposal it produced through the T003 gate.
type GenerationResult struct {
	Run      domain.AgentRun
	Proposal ProposalDetail
}

// GenerationPromptView is the deterministic generation input. Its canonical
// JSON is exactly what input_hash digests: the versioned output schema, the
// target snapshot, the instruction and the schema reference. The agent-run
// attribution echo is server-derived metadata (taken from the run itself) and
// is injected into the rendered prompt only — never into the hashed input.
type GenerationPromptView struct {
	Schema           string                   `json:"schema"`
	TargetObjectType string                   `json:"targetObjectType"`
	TargetObjectID   string                   `json:"targetObjectId"`
	BaseRevisionID   string                   `json:"baseRevisionId,omitempty"`
	Target           GenerationTargetSnapshot `json:"target"`
	Instruction      string                   `json:"instruction"`
	OutputSchema     json.RawMessage          `json:"outputSchema"`
}

// GenerationService performs live proposal generation through the configured
// model with full SSOT §8.6 recording: model, configuration revision, input
// hash, model step, output digest, cost, duration and final state. The model
// output passes the embedded semlia.proposal-input/v1 schema gate before the
// agent-attributed proposal is created through the T003 authoring path, so
// live output flows into the normal validation/review pipeline — never
// straight to publication.
type GenerationService struct {
	repository    GenerationRepository
	config        *ModelConfigService
	runs          *AgentRunService
	authoring     *AuthoringService
	authorizer    authorizationapp.Evaluator
	clock         Clock
	clientFactory func(provider domain.ModelProvider, resolver llm.CredentialResolver, httpClient *http.Client) (llm.ProviderClient, error)
}

type GenerationOption func(*GenerationService)

// WithProviderClientFactory overrides the provider-client construction
// (tests inject transport-level configuration); production uses the additive
// llm.NewProviderClient factory.
func WithProviderClientFactory(
	factory func(provider domain.ModelProvider, resolver llm.CredentialResolver, httpClient *http.Client) (llm.ProviderClient, error),
) GenerationOption {
	return func(service *GenerationService) { service.clientFactory = factory }
}

func NewGenerationService(
	repository GenerationRepository,
	config *ModelConfigService,
	runs *AgentRunService,
	authoring *AuthoringService,
	authorizer authorizationapp.Evaluator,
	clock Clock,
	options ...GenerationOption,
) *GenerationService {
	if repository == nil || config == nil || runs == nil || authoring == nil || clock == nil {
		panic("generation repository, model config service, agent run service, authoring service and clock are required")
	}
	service := &GenerationService{
		repository: repository, config: config, runs: runs, authoring: authoring,
		authorizer: authorizer, clock: clock,
		clientFactory: llm.NewProviderClient,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

// GenerateProposal executes the whole flow. Failure at any point after the
// run starts finishes the run as failed — the degradation contract is that a
// provider or schema failure records the run and creates NO proposal.
func (service *GenerationService) GenerateProposal(ctx context.Context, request GenerateProposalRequest) (GenerationResult, error) {
	request.Instruction = strings.TrimSpace(request.Instruction)
	request.TargetObjectID = strings.TrimSpace(request.TargetObjectID)
	if request.WorkspaceID.IsZero() || !isValidGenerationTarget(request.TargetObjectType) ||
		request.TargetObjectID == "" || request.Instruction == "" || len(request.Instruction) > 4096 {
		return GenerationResult{}, domain.ErrInvalidArgument
	}
	// The CALLING principal needs asset.propose server-side for the target;
	// the generated proposal itself is authored by the workspace's seeded
	// agent principal (D6).
	if err := service.authorize(ctx, authorization.ActionAssetPropose, proposalResource(request.TargetObjectType, request.TargetObjectID, request.WorkspaceID), request); err != nil {
		return GenerationResult{}, err
	}
	snapshot, err := service.loadSnapshot(ctx, request)
	if err != nil {
		return GenerationResult{}, err
	}
	setting, provider, err := service.resolveModel(ctx, request)
	if err != nil {
		return GenerationResult{}, err
	}
	if !provider.Protocol.SupportsGeneration() {
		return GenerationResult{}, &llm.ProviderUnsupportedError{Protocol: provider.Protocol}
	}
	agentPrincipal, err := service.repository.WorkspaceAgentPrincipal(ctx, request.WorkspaceID)
	if err != nil {
		return GenerationResult{}, fmt.Errorf("load seeded agent principal: %w", err)
	}
	client, err := service.clientFactory(*provider, nil, nil)
	if err != nil {
		return GenerationResult{}, normalizeClientError(err)
	}

	inputHash, err := service.inputHash(snapshot, request.Instruction)
	if err != nil {
		return GenerationResult{}, err
	}
	run, err := service.startRun(ctx, request, setting, provider, agentPrincipal.ID, inputHash)
	if err != nil {
		return GenerationResult{}, err
	}

	response, completeErr := client.Complete(ctx, llm.CompleteRequest{
		Model: setting.Model,
		Messages: []llm.Message{{
			Role:    "user",
			Content: service.renderPrompt(snapshot, request.Instruction, inputHash, run),
		}},
		MaxTokens: setting.TokenLimit,
	})
	if completeErr != nil {
		completeErr = normalizeClientError(completeErr)
		service.recordModelStep(ctx, request, run, inputHash, contentDigestOf(completeErr.Error()), stableErrorCode(completeErr))
		service.finishFailed(ctx, request, run, response)
		return GenerationResult{}, completeErr
	}
	payload, gateErr := service.gateOutput(response.Content, run, inputHash)
	if gateErr != nil {
		service.recordModelStep(ctx, request, run, inputHash, contentDigestOf(response.Content), "AI_OUTPUT_INVALID")
		service.finishFailed(ctx, request, run, response)
		return GenerationResult{}, gateErr
	}
	service.recordModelStep(ctx, request, run, inputHash, contentDigestOf(response.Content), "")

	proposal, createErr := service.createProposal(ctx, request, run, payload)
	if createErr != nil {
		_ = service.recordToolStep(ctx, request, run, payload, contentDigestOf(createErr.Error()), "PROPOSAL_GATE_REJECTED")
		service.finishFailed(ctx, request, run, response)
		return GenerationResult{}, createErr
	}
	if err := service.recordToolStep(ctx, request, run, payload, contentDigestOf(proposal.Proposal.ID.String()), ""); err != nil {
		service.finishFailed(ctx, request, run, response)
		return GenerationResult{}, err
	}
	if err := service.finishSucceeded(ctx, request, run, payload, response); err != nil {
		return GenerationResult{}, err
	}
	finished, getErr := service.runs.GetRun(ctx, request.WorkspaceID, run.ID)
	if getErr != nil {
		return GenerationResult{}, getErr
	}
	return GenerationResult{Run: finished, Proposal: proposal}, nil
}

// loadSnapshot reads the deterministic target input. Semantic-asset targets
// pin the current revision, governed objects pin their current version.
func (service *GenerationService) loadSnapshot(ctx context.Context, request GenerateProposalRequest) (GenerationTargetSnapshot, error) {
	if request.TargetObjectType == domain.TargetSemanticAsset {
		assetID, err := identity.ParseAssetID(request.TargetObjectID)
		if err != nil {
			return GenerationTargetSnapshot{}, fmt.Errorf("%w: semantic asset target %q", domain.ErrInvalidArgument, request.TargetObjectID)
		}
		return service.repository.GenerationAssetSnapshot(ctx, request.WorkspaceID, assetID)
	}
	if !request.TargetObjectType.IsGovernedObject() {
		return GenerationTargetSnapshot{}, fmt.Errorf("%w: unsupported generation target %q", domain.ErrInvalidArgument, request.TargetObjectType)
	}
	return service.repository.GenerationObjectSnapshot(ctx, request.WorkspaceID, request.TargetObjectType, request.TargetObjectID)
}

// resolveModel returns the explicit or default llm setting with its provider.
// An explicit setting must be an enabled llm of this workspace; the default
// query already filters for enabled settings of enabled providers.
func (service *GenerationService) resolveModel(
	ctx context.Context, request GenerateProposalRequest,
) (domain.ModelSetting, *domain.ModelProvider, error) {
	var setting domain.ModelSetting
	var err error
	if request.ModelSettingID != nil {
		setting, err = service.config.GetSetting(ctx, GetModelSettingRequest{
			WorkspaceID: request.WorkspaceID, SettingID: *request.ModelSettingID,
			PrincipalRef: request.PrincipalRef, TraceID: request.TraceID,
		})
		if err != nil {
			return domain.ModelSetting{}, nil, err
		}
	} else {
		setting, err = service.config.DefaultSetting(ctx, request.WorkspaceID, domain.ModelKindLLM)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return domain.ModelSetting{}, nil, fmt.Errorf("%w: no enabled default llm model is configured", domain.ErrConflict)
			}
			return domain.ModelSetting{}, nil, err
		}
	}
	if setting.Kind != domain.ModelKindLLM || !setting.Enabled {
		return domain.ModelSetting{}, nil, fmt.Errorf(
			"%w: model setting %s is not an enabled llm", domain.ErrInvalidArgument, setting.ID.String())
	}
	provider, err := service.config.repository.GetModelProvider(ctx, request.WorkspaceID, setting.ProviderID)
	if err != nil {
		return domain.ModelSetting{}, nil, err
	}
	if !provider.Enabled {
		return domain.ModelSetting{}, nil, &llm.ProviderUnavailableError{Detail: "provider is disabled"}
	}
	return setting, &provider, nil
}

func (service *GenerationService) startRun(
	ctx context.Context,
	request GenerateProposalRequest,
	setting domain.ModelSetting,
	provider *domain.ModelProvider,
	agentPrincipal identity.PrincipalID,
	inputHash string,
) (domain.AgentRun, error) {
	return service.runs.Start(ctx, StartAgentRunRequest{
		WorkspaceID:    request.WorkspaceID,
		PrincipalID:    &agentPrincipal,
		Model:          setting.Model,
		ConfigRevision: provider.CredentialRevision + "/" + setting.ID.String(),
		InputHash:      inputHash,
	})
}

// inputHash digests the canonical generation input. CanonicalJSON re-encodes
// with sorted keys, so the digest is a deterministic function of the inputs.
func (service *GenerationService) inputHash(snapshot GenerationTargetSnapshot, instruction string) (string, error) {
	payload, err := json.Marshal(GenerationPromptView{
		Schema:           ProposalInputSchemaID,
		TargetObjectType: string(snapshot.TargetObjectType),
		TargetObjectID:   snapshot.TargetObjectID,
		BaseRevisionID:   snapshot.BaseRevisionID,
		Target:           snapshot,
		Instruction:      instruction,
		OutputSchema:     json.RawMessage(proposalInputSchemaV1),
	})
	if err != nil {
		return "", fmt.Errorf("encode generation input: %w", err)
	}
	return domain.DigestJSON(payload)
}

// renderPrompt builds the deterministic user prompt: fixed header lines, the
// canonical input payload and the exact agentAttribution echo the output must
// carry (agentRunId, model, configRevision from the run; inputHash as hashed).
func (service *GenerationService) renderPrompt(
	snapshot GenerationTargetSnapshot,
	instruction string,
	inputHash string,
	run domain.AgentRun,
) string {
	payload, err := json.Marshal(GenerationPromptView{
		Schema:           ProposalInputSchemaID,
		TargetObjectType: string(snapshot.TargetObjectType),
		TargetObjectID:   snapshot.TargetObjectID,
		BaseRevisionID:   snapshot.BaseRevisionID,
		Target:           snapshot,
		Instruction:      instruction,
		OutputSchema:     json.RawMessage(proposalInputSchemaV1),
	})
	if err != nil {
		payload = []byte("{}")
	}
	var builder strings.Builder
	builder.WriteString("Generate one governed proposal for the Semlia semantic registry.\n")
	builder.WriteString("Emit EXACTLY ONE JSON document conforming to the \"outputSchema\" included in the input below. No prose, no markdown fences.\n")
	builder.WriteString("The output's agentAttribution field must be exactly: " +
		fmt.Sprintf(`{"agentRunId":%q,"model":%q,"configRevision":%q,"inputHash":%q}`,
			run.ID.String(), run.Model, run.ConfigRevision, inputHash) + "\n")
	builder.WriteString("Base every change on the \"target\" snapshot and the instruction. Use the schema's fieldPath/op/before/after shape; the system recomputes digests from the provided values.\n\n")
	builder.Write(payload)
	return builder.String()
}

// gateOutput extracts the JSON document from the model content, validates it
// against the embedded semlia.proposal-input/v1 schema (the T003 gate),
// verifies the attribution echo against the run and normalizes the change-set
// digests from the provided values. The returned payload is what the
// authoring service receives and what the output digest hashes.
func (service *GenerationService) gateOutput(content string, run domain.AgentRun, inputHash string) (json.RawMessage, error) {
	raw, extractErr := extractJSONObject(content)
	if extractErr != nil {
		return nil, &AIOutputInvalidError{Violations: []string{"payload is not a JSON object"}}
	}
	var parsed struct {
		TargetObjectType domain.TargetObjectType `json:"targetObjectType"`
		TargetObjectID   string                  `json:"targetObjectId"`
		BaseRevisionID   string                  `json:"baseRevisionId"`
		Title            string                  `json:"title"`
		Summary          string                  `json:"summary"`
		Reason           string                  `json:"reason"`
		ChangeSet        []struct {
			FieldPath    string          `json:"fieldPath"`
			Op           domain.ChangeOp `json:"op"`
			BeforeValue  json.RawMessage `json:"beforeValue"`
			AfterValue   json.RawMessage `json:"afterValue"`
			BeforeDigest string          `json:"beforeDigest"`
			AfterDigest  string          `json:"afterDigest"`
		} `json:"changeSet"`
		AgentAttribution struct {
			AgentRunID     identity.AgentRunID `json:"agentRunId"`
			Model          string              `json:"model"`
			ConfigRevision string              `json:"configRevision"`
			InputHash      string              `json:"inputHash"`
		} `json:"agentAttribution"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, &AIOutputInvalidError{Violations: []string{"payload is not a JSON object"}}
	}
	// Attribution echo: the run identity is server-owned. A payload claiming
	// another run, model or config revision is invalid output — no proposal
	// may be attributed to a run that did not produce it (SSOT §8.6).
	attribution := parsed.AgentAttribution
	if attribution.AgentRunID != run.ID || attribution.Model != run.Model ||
		attribution.ConfigRevision != run.ConfigRevision {
		return nil, &AIOutputInvalidError{
			Violations: []string{"/agentAttribution: does not match this agent run"},
		}
	}
	// Recompute the digests from the values so the change-set is truthful;
	// when a value is absent the model-provided digest is kept (add/remove
	// shapes without inline values).
	changeSet := make([]gatedChangeSetItem, 0, len(parsed.ChangeSet))
	for _, item := range parsed.ChangeSet {
		entry := gatedChangeSetItem{
			FieldPath: item.FieldPath, Op: item.Op,
			BeforeDigest: item.BeforeDigest, AfterDigest: item.AfterDigest,
			BeforeValue: item.BeforeValue, AfterValue: item.AfterValue,
		}
		if len(item.BeforeValue) > 0 && string(item.BeforeValue) != "null" {
			if digest, digestErr := domain.DigestJSON(item.BeforeValue); digestErr == nil {
				entry.BeforeDigest = digest
			}
		}
		if len(item.AfterValue) > 0 && string(item.AfterValue) != "null" {
			if digest, digestErr := domain.DigestJSON(item.AfterValue); digestErr == nil {
				entry.AfterDigest = digest
			}
		}
		changeSet = append(changeSet, entry)
	}
	// Re-encode with the server-owned attribution values and validate the
	// final document through the versioned schema gate.
	document := map[string]any{
		"targetObjectType": parsed.TargetObjectType,
		"targetObjectId":   parsed.TargetObjectID,
		"title":            parsed.Title,
		"changeSet":        changeSet,
		"agentAttribution": map[string]string{
			"agentRunId":     run.ID.String(),
			"model":          run.Model,
			"configRevision": run.ConfigRevision,
			"inputHash":      inputHash,
		},
	}
	if parsed.BaseRevisionID != "" {
		document["baseRevisionId"] = parsed.BaseRevisionID
	}
	if parsed.Summary != "" {
		document["summary"] = parsed.Summary
	}
	if parsed.Reason != "" {
		document["reason"] = parsed.Reason
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, &AIOutputInvalidError{Violations: []string{"payload could not be normalized"}}
	}
	if err := ValidateAgentProposalInput(encoded); err != nil {
		return nil, err
	}
	return encoded, nil
}

// gatedChangeSetItem is the wire shape of one change-set item in the gated
// payload — the tag set mirrors semlia.proposal-input/v1 exactly.
type gatedChangeSetItem struct {
	FieldPath    string          `json:"fieldPath"`
	Op           domain.ChangeOp `json:"op"`
	BeforeDigest string          `json:"beforeDigest,omitempty"`
	AfterDigest  string          `json:"afterDigest,omitempty"`
	BeforeValue  json.RawMessage `json:"beforeValue,omitempty"`
	AfterValue   json.RawMessage `json:"afterValue,omitempty"`
}

// createProposal routes the gated payload through the T003 authoring path:
// the proposal is attributed to the agent run and the caller is
// re-authorized by the authoring service itself.
func (service *GenerationService) createProposal(
	ctx context.Context, request GenerateProposalRequest, run domain.AgentRun, payload json.RawMessage,
) (ProposalDetail, error) {
	var parsed struct {
		TargetObjectType domain.TargetObjectType `json:"targetObjectType"`
		TargetObjectID   string                  `json:"targetObjectId"`
		BaseRevisionID   string                  `json:"baseRevisionId"`
		Title            string                  `json:"title"`
		Summary          string                  `json:"summary"`
		Reason           string                  `json:"reason"`
		ChangeSet        []ChangeSetItemInput    `json:"changeSet"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return ProposalDetail{}, fmt.Errorf("%w: gated payload could not be decoded", domain.ErrInvariant)
	}
	createRequest := CreateAuthoringProposalRequest{
		WorkspaceID:    request.WorkspaceID,
		TargetType:     parsed.TargetObjectType,
		TargetObjectID: parsed.TargetObjectID,
		BaseRevisionID: identity.RevisionID{},
		Title:          parsed.Title,
		Summary:        parsed.Summary,
		Reason:         parsed.Reason,
		ChangeSet:      parsed.ChangeSet,
		AgentAttribution: &AgentAttribution{
			AgentRunID: run.ID, Model: run.Model,
			ConfigRevision: run.ConfigRevision, InputHash: run.InputHash,
		},
		PrincipalRef: request.PrincipalRef,
		TraceID:      request.TraceID,
	}
	if parsed.BaseRevisionID != "" {
		baseRevision, err := identity.ParseRevisionID(parsed.BaseRevisionID)
		if err != nil {
			return ProposalDetail{}, fmt.Errorf("%w: base revision %q", domain.ErrInvalidArgument, parsed.BaseRevisionID)
		}
		createRequest.BaseRevisionID = baseRevision
	}
	return service.authoring.CreateProposal(ctx, createRequest)
}

func (service *GenerationService) recordModelStep(
	ctx context.Context, request GenerateProposalRequest, run domain.AgentRun,
	inputHash, outputHash, errorCode string,
) {
	_, _ = service.runs.RecordStep(ctx, RecordStepRequest{
		WorkspaceID: request.WorkspaceID, AgentRunID: run.ID, Sequence: 1,
		Kind: domain.AgentStepModel, InputHash: inputHash, OutputHash: outputHash,
		ErrorCode: errorCode,
	})
}

func (service *GenerationService) recordToolStep(
	ctx context.Context, request GenerateProposalRequest, run domain.AgentRun,
	payload json.RawMessage, outputHash, errorCode string,
) error {
	_, err := service.runs.RecordStep(ctx, RecordStepRequest{
		WorkspaceID: request.WorkspaceID, AgentRunID: run.ID, Sequence: 2,
		Kind: domain.AgentStepTool, ToolName: "create_proposal",
		InputHash: contentDigestOf(string(payload)), OutputHash: outputHash,
		ErrorCode: errorCode,
	})
	return err
}

func (service *GenerationService) finishFailed(
	ctx context.Context, request GenerateProposalRequest, run domain.AgentRun, response llm.CompleteResponse,
) {
	_ = service.finish(ctx, request, run, domain.AgentRunFailed, "", response)
}

func (service *GenerationService) finishSucceeded(
	ctx context.Context, request GenerateProposalRequest, run domain.AgentRun, payload json.RawMessage, response llm.CompleteResponse,
) error {
	outputDigest, err := domain.DigestJSON(payload)
	if err != nil {
		return err
	}
	return service.finish(ctx, request, run, domain.AgentRunSucceeded, outputDigest, response)
}

func (service *GenerationService) finish(
	ctx context.Context,
	request GenerateProposalRequest,
	run domain.AgentRun,
	finalState domain.AgentRunState,
	outputDigest string,
	response llm.CompleteResponse,
) error {
	// Cost mapping for v1: one micro per reported token (prompt + completion),
	// so the recorded cost is the normalized provider usage. Pricing adapters
	// land with the remaining provider protocols.
	_, err := service.runs.Finish(ctx, FinishAgentRunRequest{
		WorkspaceID: request.WorkspaceID, RunID: run.ID, FinalState: finalState,
		OutputDigest: outputDigest,
		CostMicros:   response.Usage.PromptTokens + response.Usage.CompletionTokens,
	})
	return err
}

func (service *GenerationService) authorize(ctx context.Context, action authorization.Action, resource authorization.Resource, request GenerateProposalRequest) error {
	if service.authorizer == nil {
		return nil
	}
	decision, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: strings.TrimSpace(request.PrincipalRef), WorkspaceID: request.WorkspaceID,
		Action: action, Resource: resource, TraceID: request.TraceID,
	})
	if err != nil {
		return err
	}
	if decision.Allowed {
		return nil
	}
	return &authorization.DenialError{Decision: decision}
}

// isValidGenerationTarget mirrors the semlia.proposal-input/v1 target enum.
func isValidGenerationTarget(targetType domain.TargetObjectType) bool {
	switch targetType {
	case domain.TargetSemanticAsset, domain.TargetPhysicalBinding, domain.TargetModelGrain,
		domain.TargetEntityKey, domain.TargetJoinContract:
		return true
	}
	return false
}

// extractJSONObject pulls the outermost JSON object out of the model content,
// stripping optional markdown fences deterministically.
func extractJSONObject(content string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON object found")
	}
	return json.RawMessage(trimmed[start : end+1]), nil
}

func normalizeClientError(err error) error {
	var unsupported *llm.ProviderUnsupportedError
	if errors.As(err, &unsupported) {
		return unsupported
	}
	var unavailable *llm.ProviderUnavailableError
	if errors.As(err, &unavailable) {
		return unavailable
	}
	if errors.Is(err, llm.ErrProviderUnsupported) {
		return &llm.ProviderUnsupportedError{}
	}
	if errors.Is(err, llm.ErrProviderUnavailable) {
		return &llm.ProviderUnavailableError{}
	}
	return &llm.ProviderUnavailableError{Detail: "provider call failed", Err: err}
}

// stableErrorCode maps a generation failure to the wire-stable code recorded
// on the agent step (DB pattern: uppercase snake, 3..64 chars).
func stableErrorCode(err error) string {
	var unsupported *llm.ProviderUnsupportedError
	if errors.As(err, &unsupported) {
		return "PROVIDER_UNSUPPORTED"
	}
	return "PROVIDER_UNAVAILABLE"
}

// contentDigestOf hashes bounded helper content (error details, identifiers)
// into the sha256 digest form the agent-step schema requires.
func contentDigestOf(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
