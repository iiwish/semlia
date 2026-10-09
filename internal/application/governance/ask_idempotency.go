package governance

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	"github.com/iiwish/semlia/internal/application/governance/llm"
	distributiondomain "github.com/iiwish/semlia/internal/domain/distribution"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type AskStateError struct{ Code string }

func (err *AskStateError) Error() string { return err.Code }

type AskFinishCommand struct {
	Request           domain.AskRequestRecord
	Status            string
	QueryID           *identity.SemanticQueryID
	ErrorCode         string
	OutputDigest      string
	ModelOutputDigest string
	// ServerTool names the server step that settled the request without a model call, such as disambiguation.
	ServerTool string
	CostMicros int64
	FinishedAt time.Time
}

type AskRequestRepository interface {
	GetAskRequest(context.Context, identity.WorkspaceID, identity.PrincipalID, string) (domain.AskRequestRecord, error)
	ClaimAskRequest(context.Context, domain.AskRequestRecord, domain.AgentRun) (domain.AskRequestRecord, bool, error)
	FinishAskRequest(context.Context, AskFinishCommand) (domain.AgentRun, error)
	ExpireAskRequests(context.Context, int) (int, error)
}

func (service *AskService) replay(ctx context.Context, request AskRequest, record domain.AskRequestRecord, digest string) (AskResult, error) {
	if record.InputDigest != digest {
		return AskResult{}, &AskStateError{Code: "ASK_KEY_CONFLICT"}
	}
	if record.Status == "running" {
		if !service.runs.clock.Now().Before(record.Deadline) {
			if _, err := service.repository.ExpireAskRequests(ctx, 100); err != nil {
				return AskResult{}, err
			}
			current, err := service.repository.GetAskRequest(ctx, record.WorkspaceID, record.RequestedBy, record.Key)
			if err != nil {
				return AskResult{}, err
			}
			if current.Status != "running" {
				return service.replay(ctx, request, current, digest)
			}
		}
		return AskResult{}, &AskStateError{Code: "ASK_IN_PROGRESS"}
	}
	if record.Status != "succeeded" && record.Status != "clarification" {
		return AskResult{}, askTerminalError(record.ErrorCode)
	}
	knowledge, err := service.resolver.AskKnowledge(ctx, distributionapp.ResolveRequest{WorkspaceID: request.WorkspaceID, PrincipalRef: request.PrincipalRef, TraceID: request.TraceID, Input: distributiondomain.SemanticQueryInput{Context: request.Context}}, request.Question)
	if err != nil {
		return AskResult{}, err
	}
	if knowledge.Digest != record.KnowledgeDigest {
		return AskResult{}, &AskStateError{Code: "ASK_CONTEXT_CHANGED"}
	}
	run, err := service.runs.GetRun(ctx, request.WorkspaceID, record.RunID)
	if err != nil {
		return AskResult{}, err
	}
	result := AskResult{Run: run, Interpretation: AskInterpretation{Schema: AskInterpretationSchemaID, Outcome: "clarification", Clarification: askServerClarification(knowledge)}}
	if record.Status == "clarification" {
		return result, nil
	}
	if record.QueryID == nil {
		return AskResult{}, domain.ErrInvariant
	}
	resolution, err := service.resolver.GetQuery(ctx, request.WorkspaceID, *record.QueryID, request.PrincipalRef, request.TraceID)
	if err != nil {
		return AskResult{}, err
	}
	var query distributiondomain.SemanticQueryInput
	if err := json.Unmarshal(resolution.Query.CanonicalRequest, &query); err != nil {
		return AskResult{}, domain.ErrInvariant
	}
	result.Interpretation = AskInterpretation{Schema: AskInterpretationSchemaID, Outcome: "query", Query: &query}
	result.Resolution = &resolution
	return result, nil
}

func askTerminalError(code string) error {
	switch code {
	case "PROVIDER_UNSUPPORTED":
		return &llm.ProviderUnsupportedError{}
	case "PROVIDER_UNAVAILABLE":
		return &llm.ProviderUnavailableError{}
	case "AI_OUTPUT_INVALID":
		return &AIOutputInvalidError{Violations: []string{"model output was rejected"}}
	default:
		return &AskStateError{Code: code}
	}
}

func (service *AskService) settle(ctx context.Context, claim domain.AskRequestRecord, status, code, outputDigest, modelDigest string, queryID *identity.SemanticQueryID, response llm.CompleteResponse) (domain.AgentRun, error) {
	return service.finish(ctx, AskFinishCommand{Request: claim, Status: status, ErrorCode: code, OutputDigest: outputDigest, ModelOutputDigest: modelDigest, QueryID: queryID, CostMicros: response.Usage.PromptTokens + response.Usage.CompletionTokens})
}

func (service *AskService) finish(ctx context.Context, command AskFinishCommand) (domain.AgentRun, error) {
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	command.FinishedAt = service.runs.clock.Now().UTC()
	return service.repository.FinishAskRequest(finishCtx, command)
}

func (service *AskService) failure(ctx context.Context, claim domain.AskRequestRecord, original error, code, modelDigest string, response llm.CompleteResponse) (AskResult, error) {
	status := "failed"
	if errors.Is(ctx.Err(), context.Canceled) {
		status, code = "cancelled", "ASK_CANCELLED"
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		status, code = "outcome_unknown", "ASK_OUTCOME_UNKNOWN"
	}
	if _, finishErr := service.settle(ctx, claim, status, code, "", modelDigest, nil, response); finishErr != nil {
		return AskResult{}, finishErr
	}
	var invalid *AIOutputInvalidError
	if code == "AI_OUTPUT_INVALID" && errors.As(original, &invalid) {
		return AskResult{}, invalid
	}
	return AskResult{}, askTerminalError(code)
}

func (service *AskService) ReconcileExpired(ctx context.Context) error {
	_, err := service.repository.ExpireAskRequests(ctx, 100)
	return err
}
