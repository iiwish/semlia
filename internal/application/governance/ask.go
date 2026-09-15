package governance

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	"github.com/iiwish/semlia/internal/application/governance/llm"
	"github.com/iiwish/semlia/internal/domain/authorization"
	distributiondomain "github.com/iiwish/semlia/internal/domain/distribution"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const AskInterpretationSchemaID = "semlia.ask-interpretation/v1"

//go:embed ask_interpretation.v1.schema.json
var askInterpretationSchemaV1 []byte

var (
	askSchemaOnce sync.Once
	askSchema     *jsonschema.Schema
	askSchemaErr  error
)

type AskPrincipalRepository interface {
	WorkspaceAgentPrincipal(context.Context, identity.WorkspaceID) (authorization.Principal, error)
}

type AskService struct {
	repository    AskPrincipalRepository
	config        *ModelConfigService
	runs          *AgentRunService
	resolver      *distributionapp.Service
	authorizer    authorizationapp.Evaluator
	clientFactory func(domain.ModelProvider, llm.CredentialResolver, *http.Client) (llm.ProviderClient, error)
}

type AskOption func(*AskService)

func WithAskProviderClientFactory(factory func(domain.ModelProvider, llm.CredentialResolver, *http.Client) (llm.ProviderClient, error)) AskOption {
	return func(service *AskService) { service.clientFactory = factory }
}

func NewAskService(
	repository AskPrincipalRepository,
	config *ModelConfigService,
	runs *AgentRunService,
	resolver *distributionapp.Service,
	authorizer authorizationapp.Evaluator,
	options ...AskOption,
) *AskService {
	if repository == nil || config == nil || runs == nil || resolver == nil {
		panic("ask repository, model config, agent run service and resolver are required")
	}
	service := &AskService{
		repository: repository, config: config, runs: runs, resolver: resolver,
		authorizer: authorizer, clientFactory: llm.NewProviderClient,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

type AskRequest struct {
	WorkspaceID    identity.WorkspaceID
	Question       string
	Context        distributiondomain.ResolutionContext
	IdempotencyKey string
	PrincipalRef   string
	TraceID        string
}

type AskInterpretation struct {
	Schema        string                                 `json:"schema"`
	Outcome       string                                 `json:"outcome"`
	Query         *distributiondomain.SemanticQueryInput `json:"query,omitempty"`
	Clarification string                                 `json:"clarification,omitempty"`
}

type AskResult struct {
	Run            domain.AgentRun
	Interpretation AskInterpretation
	Resolution     *distributionapp.ResolutionResult
}

func (service *AskService) Ask(ctx context.Context, request AskRequest) (AskResult, error) {
	request.Question = strings.TrimSpace(request.Question)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.PrincipalRef = strings.TrimSpace(request.PrincipalRef)
	if request.WorkspaceID.IsZero() || request.Question == "" || len(request.Question) > 4096 ||
		request.IdempotencyKey == "" || len(request.IdempotencyKey) > 160 {
		return AskResult{}, domain.ErrInvalidArgument
	}
	if request.Context.Mode == "" {
		request.Context.Mode = distributiondomain.ResolutionCurrent
	}
	if err := service.authorize(ctx, request); err != nil {
		return AskResult{}, err
	}
	setting, provider, err := service.defaultModel(ctx, request.WorkspaceID)
	if err != nil {
		return AskResult{}, err
	}
	agent, err := service.repository.WorkspaceAgentPrincipal(ctx, request.WorkspaceID)
	if err != nil {
		return AskResult{}, fmt.Errorf("load seeded agent principal: %w", err)
	}
	inputHash, err := askInputHash(request.Question, request.Context)
	if err != nil {
		return AskResult{}, err
	}
	run, err := service.runs.Start(ctx, StartAgentRunRequest{
		WorkspaceID: request.WorkspaceID, PrincipalID: &agent.ID, Model: setting.Model,
		ConfigRevision: provider.CredentialRevision + "/" + setting.ID.String(), InputHash: inputHash,
	})
	if err != nil {
		return AskResult{}, err
	}
	client, err := service.clientFactory(provider, nil, nil)
	if err != nil {
		service.fail(ctx, request.WorkspaceID, run, inputHash, contentDigestOf(err.Error()), stableErrorCode(err), llm.CompleteResponse{})
		return AskResult{}, normalizeClientError(err)
	}
	response, err := client.Complete(ctx, llm.CompleteRequest{
		Model:     setting.Model,
		Messages:  []llm.Message{{Role: "user", Content: renderAskPrompt(request.Question)}},
		MaxTokens: min(setting.TokenLimit, 2048),
	})
	if err != nil {
		err = normalizeClientError(err)
		service.fail(ctx, request.WorkspaceID, run, inputHash, contentDigestOf(err.Error()), stableErrorCode(err), response)
		return AskResult{}, err
	}
	interpretation, normalized, err := gateAskOutput(response.Content, request.Context)
	if err != nil {
		service.fail(ctx, request.WorkspaceID, run, inputHash, contentDigestOf(response.Content), "AI_OUTPUT_INVALID", response)
		return AskResult{}, err
	}
	_, _ = service.runs.RecordStep(ctx, RecordStepRequest{
		WorkspaceID: request.WorkspaceID, AgentRunID: run.ID, Sequence: 1,
		Kind: domain.AgentStepModel, InputHash: inputHash, OutputHash: contentDigestOf(response.Content),
	})
	if interpretation.Outcome == "clarification" {
		finished, finishErr := service.finish(ctx, request.WorkspaceID, run, normalized, response)
		return AskResult{Run: finished, Interpretation: interpretation}, finishErr
	}
	resolution, err := service.resolver.Resolve(ctx, distributionapp.ResolveRequest{
		WorkspaceID: request.WorkspaceID, Input: *interpretation.Query, Channel: "ask",
		IdempotencyKey: request.IdempotencyKey, PrincipalRef: request.PrincipalRef, TraceID: request.TraceID,
	})
	if err != nil {
		_, _ = service.runs.RecordStep(ctx, RecordStepRequest{
			WorkspaceID: request.WorkspaceID, AgentRunID: run.ID, Sequence: 2,
			Kind: domain.AgentStepTool, ToolName: "semantic_resolve", InputHash: contentDigestOf(string(normalized)),
			OutputHash: contentDigestOf(err.Error()), ErrorCode: "SEMANTIC_RESOLUTION_FAILED",
		})
		_, _ = service.runs.Finish(ctx, FinishAgentRunRequest{
			WorkspaceID: request.WorkspaceID, RunID: run.ID, FinalState: domain.AgentRunFailed,
			CostMicros: response.Usage.PromptTokens + response.Usage.CompletionTokens,
		})
		return AskResult{}, err
	}
	_, err = service.runs.RecordStep(ctx, RecordStepRequest{
		WorkspaceID: request.WorkspaceID, AgentRunID: run.ID, Sequence: 2,
		Kind: domain.AgentStepTool, ToolName: "semantic_resolve", InputHash: contentDigestOf(string(normalized)),
		OutputHash: resolution.Query.RequestDigest,
	})
	if err != nil {
		return AskResult{}, err
	}
	finished, err := service.finish(ctx, request.WorkspaceID, run, normalized, response)
	if err != nil {
		return AskResult{}, err
	}
	return AskResult{Run: finished, Interpretation: interpretation, Resolution: &resolution}, nil
}

func (service *AskService) authorize(ctx context.Context, request AskRequest) error {
	if service.authorizer == nil {
		return nil
	}
	decision, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: request.PrincipalRef, WorkspaceID: request.WorkspaceID,
		Action:   authorization.ActionSemanticResolve,
		Resource: authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()},
		TraceID:  request.TraceID,
	})
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return &authorization.DenialError{Decision: decision}
	}
	return nil
}

func (service *AskService) defaultModel(ctx context.Context, workspace identity.WorkspaceID) (domain.ModelSetting, domain.ModelProvider, error) {
	setting, err := service.config.DefaultSetting(ctx, workspace, domain.ModelKindLLM)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ModelSetting{}, domain.ModelProvider{}, fmt.Errorf("%w: no enabled default llm model is configured", domain.ErrConflict)
		}
		return domain.ModelSetting{}, domain.ModelProvider{}, err
	}
	provider, err := service.config.repository.GetModelProvider(ctx, workspace, setting.ProviderID)
	if err != nil {
		return domain.ModelSetting{}, domain.ModelProvider{}, err
	}
	if !setting.Enabled || !provider.Enabled {
		return domain.ModelSetting{}, domain.ModelProvider{}, &llm.ProviderUnavailableError{Detail: "provider or model is disabled"}
	}
	return setting, provider, nil
}

func askInputHash(question string, resolutionContext distributiondomain.ResolutionContext) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"schema": AskInterpretationSchemaID, "question": question, "context": resolutionContext,
	})
	if err != nil {
		return "", err
	}
	return domain.DigestJSON(payload)
}

func renderAskPrompt(question string) string {
	return "Interpret one natural-language question as released Semlia semantics.\n" +
		"Return EXACTLY ONE JSON object conforming to the schema below, without markdown or prose.\n" +
		"Never emit SQL, source locations, credentials, secrets, query results or invented asset identifiers. " +
		"Use search selectors when an exact released address is not known. Ask for clarification when the intent cannot be represented safely.\n\n" +
		"OUTPUT_SCHEMA:\n" + string(askInterpretationSchemaV1) + "\n\nQUESTION:\n" + question
}

func compileAskSchema() {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(askInterpretationSchemaV1))
	if err != nil {
		askSchemaErr = err
		return
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(AskInterpretationSchemaID, document); err != nil {
		askSchemaErr = err
		return
	}
	askSchema, askSchemaErr = compiler.Compile(AskInterpretationSchemaID)
}

func gateAskOutput(content string, resolutionContext distributiondomain.ResolutionContext) (AskInterpretation, json.RawMessage, error) {
	raw, err := extractJSONObject(content)
	if err != nil {
		return AskInterpretation{}, nil, &AIOutputInvalidError{Violations: []string{"payload is not a JSON object"}}
	}
	askSchemaOnce.Do(compileAskSchema)
	if askSchemaErr != nil {
		return AskInterpretation{}, nil, fmt.Errorf("load %s schema: %w", AskInterpretationSchemaID, askSchemaErr)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return AskInterpretation{}, nil, &AIOutputInvalidError{Violations: []string{"payload is not valid JSON"}}
	}
	if err := askSchema.Validate(document); err != nil {
		return AskInterpretation{}, nil, &AIOutputInvalidError{Violations: schemaViolationSummaries(err)}
	}
	var parsed struct {
		Schema        string                                 `json:"schema"`
		Outcome       string                                 `json:"outcome"`
		Query         *distributiondomain.SemanticQueryInput `json:"query"`
		Clarification string                                 `json:"clarification"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return AskInterpretation{}, nil, &AIOutputInvalidError{Violations: []string{"payload could not be decoded"}}
	}
	interpretation := AskInterpretation{Schema: parsed.Schema, Outcome: parsed.Outcome, Clarification: strings.TrimSpace(parsed.Clarification)}
	if parsed.Query != nil {
		parsed.Query.SchemaVersion = distributiondomain.QuerySchemaVersion
		parsed.Query.Context = resolutionContext
		if err := parsed.Query.Validate(); err != nil {
			return AskInterpretation{}, nil, &AIOutputInvalidError{Violations: []string{"/query: semantic query is invalid"}}
		}
		interpretation.Query = parsed.Query
	}
	normalized, err := json.Marshal(interpretation)
	if err != nil {
		return AskInterpretation{}, nil, err
	}
	return interpretation, normalized, nil
}

func (service *AskService) fail(ctx context.Context, workspace identity.WorkspaceID, run domain.AgentRun, inputHash, outputHash, code string, response llm.CompleteResponse) {
	_, _ = service.runs.RecordStep(ctx, RecordStepRequest{
		WorkspaceID: workspace, AgentRunID: run.ID, Sequence: 1, Kind: domain.AgentStepModel,
		InputHash: inputHash, OutputHash: outputHash, ErrorCode: code,
	})
	_, _ = service.runs.Finish(ctx, FinishAgentRunRequest{
		WorkspaceID: workspace, RunID: run.ID, FinalState: domain.AgentRunFailed,
		CostMicros: response.Usage.PromptTokens + response.Usage.CompletionTokens,
	})
}

func (service *AskService) finish(ctx context.Context, workspace identity.WorkspaceID, run domain.AgentRun, output json.RawMessage, response llm.CompleteResponse) (domain.AgentRun, error) {
	digest, err := domain.DigestJSON(output)
	if err != nil {
		return domain.AgentRun{}, err
	}
	return service.runs.Finish(ctx, FinishAgentRunRequest{
		WorkspaceID: workspace, RunID: run.ID, FinalState: domain.AgentRunSucceeded,
		OutputDigest: digest, CostMicros: response.Usage.PromptTokens + response.Usage.CompletionTokens,
	})
}
