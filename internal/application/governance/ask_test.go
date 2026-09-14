package governance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	"github.com/iiwish/semlia/internal/application/governance/llm"
	"github.com/iiwish/semlia/internal/domain/authorization"
	distributiondomain "github.com/iiwish/semlia/internal/domain/distribution"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestGateAskOutputBuildsServerOwnedSemanticQuery(t *testing.T) {
	contextValue := distributiondomain.ResolutionContext{Mode: distributiondomain.ResolutionCurrent}
	interpretation, normalized, err := gateAskOutput(`{
  "schema":"semlia.ask-interpretation/v1",
  "outcome":"query",
  "query":{"intent":"describe","measures":[{"search":"净收入"}],"dimensions":[],"filters":[],"order":[]}
}`, contextValue)
	if err != nil {
		t.Fatalf("gate output: %v", err)
	}
	if interpretation.Query == nil || interpretation.Query.SchemaVersion != distributiondomain.QuerySchemaVersion ||
		interpretation.Query.Context.Mode != distributiondomain.ResolutionCurrent {
		t.Fatalf("server-owned query fields missing: %+v", interpretation.Query)
	}
	if strings.Contains(string(normalized), "sql") || !json.Valid(normalized) {
		t.Fatalf("normalized payload = %s", normalized)
	}
}

func TestGateAskOutputAllowsClarificationWithoutResolution(t *testing.T) {
	interpretation, _, err := gateAskOutput(`{
  "schema":"semlia.ask-interpretation/v1",
  "outcome":"clarification",
  "clarification":"请说明要查询的指标或业务概念。"
}`, distributiondomain.ResolutionContext{Mode: distributiondomain.ResolutionCurrent})
	if err != nil || interpretation.Query != nil || interpretation.Clarification == "" {
		t.Fatalf("clarification = %+v, err = %v", interpretation, err)
	}
}

func TestGateAskOutputRejectsRawSQLCredentialsAndInvalidQuery(t *testing.T) {
	tests := []string{
		`{"schema":"semlia.ask-interpretation/v1","outcome":"query","query":{"intent":"describe","measures":[{"search":"收入"}],"dimensions":[],"filters":[],"order":[],"sql":"select * from orders"}}`,
		`{"schema":"semlia.ask-interpretation/v1","outcome":"query","query":{"intent":"describe","measures":[{"search":"收入","credential":"secret"}],"dimensions":[],"filters":[],"order":[]}}`,
		`{"schema":"semlia.ask-interpretation/v1","outcome":"query","query":{"intent":"aggregate","measures":[],"dimensions":[],"filters":[],"order":[]}}`,
	}
	for index, payload := range tests {
		if _, _, err := gateAskOutput(payload, distributiondomain.ResolutionContext{Mode: distributiondomain.ResolutionCurrent}); !errors.Is(err, ErrAIOutputInvalid) {
			t.Fatalf("case %d error = %v, want AI output invalid", index, err)
		}
	}
}

func TestAskProviderFailurePersistsHashOnlyFailedRun(t *testing.T) {
	repository := newAskTestRepository(t)
	clock := ClockFunc(func() time.Time { return time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC) })
	config := NewModelConfigService(repository, nil, clock)
	runs := NewAgentRunService(repository, clock)
	resolver := distributionapp.NewService(repository, nil, distributionapp.ClockFunc(clock))
	service := NewAskService(repository, config, runs, resolver, nil, WithAskProviderClientFactory(
		func(domain.ModelProvider, llm.CredentialResolver, *http.Client) (llm.ProviderClient, error) {
			return askProviderClient{err: &llm.ProviderUnavailableError{Detail: "test unavailable"}}, nil
		},
	))
	question := "显示净收入，不要保存这段原始问题"
	_, err := service.Ask(context.Background(), AskRequest{
		WorkspaceID: repository.workspace, Question: question,
		Context:        distributiondomain.ResolutionContext{Mode: distributiondomain.ResolutionCurrent},
		IdempotencyKey: "ask-provider-failure", PrincipalRef: "principal", TraceID: "trace",
	})
	var unavailable *llm.ProviderUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("ask error = %v", err)
	}
	if len(repository.runs) != 1 || repository.runs[0].Status != domain.AgentRunFailed {
		t.Fatalf("runs = %+v", repository.runs)
	}
	if repository.runs[0].InputHash == "" || strings.Contains(repository.runs[0].InputHash, question) || len(repository.steps) != 1 {
		t.Fatalf("raw question reached attribution record: run=%+v steps=%+v", repository.runs[0], repository.steps)
	}
}

func TestAskAuthorizesBeforeStartingProviderWork(t *testing.T) {
	repository := newAskTestRepository(t)
	clock := ClockFunc(func() time.Time { return time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC) })
	config := NewModelConfigService(repository, nil, clock)
	runs := NewAgentRunService(repository, clock)
	resolver := distributionapp.NewService(repository, nil, distributionapp.ClockFunc(clock))
	createdClient := false
	service := NewAskService(repository, config, runs, resolver, askDenyAuthorizer{}, WithAskProviderClientFactory(
		func(domain.ModelProvider, llm.CredentialResolver, *http.Client) (llm.ProviderClient, error) {
			createdClient = true
			return askProviderClient{}, nil
		},
	))
	_, err := service.Ask(context.Background(), AskRequest{
		WorkspaceID: repository.workspace, Question: "What is revenue?",
		Context:        distributiondomain.ResolutionContext{Mode: distributiondomain.ResolutionCurrent},
		IdempotencyKey: "ask-denied", PrincipalRef: "principal", TraceID: "trace",
	})
	var denial *authorization.DenialError
	if !errors.As(err, &denial) || createdClient || len(repository.runs) != 0 {
		t.Fatalf("err=%v client=%v runs=%d", err, createdClient, len(repository.runs))
	}
}

func TestAskUsesSharedResolverAndReturnsReleasedDefinition(t *testing.T) {
	repository := newAskTestRepository(t)
	assetID := askID(t, identity.NewAssetID)
	revisionID := askID(t, identity.NewRevisionID)
	repository.current = distributiondomain.ReleaseSnapshot{
		WorkspaceID: repository.workspace, ReleaseID: askID(t, identity.NewReleaseID), ManifestDigest: contentDigestOf("release"),
		Assets: []distributiondomain.ReleasedAsset{{AssetID: assetID, RevisionID: revisionID, Address: "commerce.net_revenue",
			AssetType: "metric", Name: "Net revenue", ContentDigest: contentDigestOf("definition"),
			Content: json.RawMessage(`{"name":"Net revenue","definition":"Revenue after confirmed refunds"}`)}},
	}
	clock := ClockFunc(func() time.Time { return time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC) })
	config := NewModelConfigService(repository, nil, clock)
	runs := NewAgentRunService(repository, clock)
	resolver := distributionapp.NewService(repository, nil, distributionapp.ClockFunc(clock))
	service := NewAskService(repository, config, runs, resolver, nil, WithAskProviderClientFactory(
		func(domain.ModelProvider, llm.CredentialResolver, *http.Client) (llm.ProviderClient, error) {
			return askProviderClient{content: `{"schema":"semlia.ask-interpretation/v1","outcome":"query","query":{"intent":"describe","measures":[{"search":"Net revenue"}],"dimensions":[],"filters":[],"order":[]}}`}, nil
		},
	))
	result, err := service.Ask(context.Background(), AskRequest{
		WorkspaceID: repository.workspace, Question: "What is net revenue?",
		Context:        distributiondomain.ResolutionContext{Mode: distributiondomain.ResolutionCurrent},
		IdempotencyKey: "ask-success", PrincipalRef: "principal", TraceID: "trace",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Status != domain.AgentRunSucceeded || result.Resolution == nil || result.Resolution.Plan == nil ||
		result.Resolution.Query.Channel != "ask" || len(result.Resolution.Definitions) != 1 ||
		result.Resolution.Definitions[0].Definition != "Revenue after confirmed refunds" {
		t.Fatalf("Ask result = %+v", result)
	}
	if len(repository.steps) != 2 || repository.record.Query.ID != result.Resolution.Query.ID {
		t.Fatalf("attribution steps=%+v resolution=%+v", repository.steps, repository.record)
	}
}

type askProviderClient struct {
	content string
	err     error
}

type askDenyAuthorizer struct{}

func (askDenyAuthorizer) Evaluate(_ context.Context, request authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	return authorization.Decision{Allowed: false, Action: request.Action, ReasonCode: authorization.ReasonNoMatchingGrant}, nil
}

func (askProviderClient) Protocol() domain.ModelProviderProtocol { return domain.ProtocolOpenAI }
func (client askProviderClient) Complete(context.Context, llm.CompleteRequest) (llm.CompleteResponse, error) {
	return llm.CompleteResponse{Content: client.content}, client.err
}

type askTestRepository struct {
	workspace identity.WorkspaceID
	provider  domain.ModelProvider
	setting   domain.ModelSetting
	agent     authorization.Principal
	runs      []domain.AgentRun
	steps     []domain.AgentStep
	current   distributiondomain.ReleaseSnapshot
	record    distributionapp.ResolutionRecord
}

func newAskTestRepository(t *testing.T) *askTestRepository {
	workspace := askID(t, identity.NewWorkspaceID)
	providerID := askID(t, identity.NewModelProviderID)
	settingID := askID(t, identity.NewModelSettingID)
	agentID := askID(t, identity.NewPrincipalID)
	return &askTestRepository{
		workspace: workspace,
		provider: domain.ModelProvider{ID: providerID, WorkspaceID: workspace, Protocol: domain.ProtocolOpenAI,
			DisplayName: "OpenAI", CredentialEnv: "OPENAI_API_KEY", CredentialRevision: contentDigestOf("credential"), Enabled: true},
		setting: domain.ModelSetting{ID: settingID, WorkspaceID: workspace, ProviderID: providerID, Kind: domain.ModelKindLLM,
			Model: "test-model", Enabled: true, IsDefault: true, Capability: "ask", TokenLimit: 1024},
		agent: authorization.Principal{ID: agentID, WorkspaceID: workspace, Kind: authorization.PrincipalAgent,
			DisplayName: "Semlia Agent", Status: authorization.PrincipalActive},
	}
}

func askID[T any](t *testing.T, create func() (T, error)) T {
	t.Helper()
	value, err := create()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func (repo *askTestRepository) WorkspaceAgentPrincipal(context.Context, identity.WorkspaceID) (authorization.Principal, error) {
	return repo.agent, nil
}
func (repo *askTestRepository) CreateAgentRun(_ context.Context, run domain.AgentRun) (domain.AgentRun, error) {
	repo.runs = append(repo.runs, run)
	return run, nil
}
func (repo *askTestRepository) GetAgentRun(_ context.Context, _ identity.WorkspaceID, id identity.AgentRunID) (domain.AgentRun, error) {
	for _, run := range repo.runs {
		if run.ID == id {
			return run, nil
		}
	}
	return domain.AgentRun{}, domain.ErrNotFound
}
func (repo *askTestRepository) FinishAgentRun(_ context.Context, command AgentRunFinishCommand) (domain.AgentRun, error) {
	for index := range repo.runs {
		if repo.runs[index].ID == command.RunID {
			repo.runs[index].Status = command.FinalState
			repo.runs[index].OutputDigest = command.OutputDigest
			repo.runs[index].CostMicros = command.CostMicros
			repo.runs[index].FinishedAt = &command.FinishedAt
			repo.runs[index].DurationMS = &command.DurationMS
			return repo.runs[index], nil
		}
	}
	return domain.AgentRun{}, domain.ErrNotFound
}
func (repo *askTestRepository) CreateAgentStep(_ context.Context, step domain.AgentStep) (domain.AgentStep, error) {
	repo.steps = append(repo.steps, step)
	return step, nil
}

func (repo *askTestRepository) CreateModelProvider(context.Context, domain.ModelProvider) (domain.ModelProvider, error) {
	panic("unused")
}
func (repo *askTestRepository) GetModelProvider(context.Context, identity.WorkspaceID, identity.ModelProviderID) (domain.ModelProvider, error) {
	return repo.provider, nil
}
func (repo *askTestRepository) ListModelProviders(context.Context, identity.WorkspaceID) ([]domain.ModelProvider, error) {
	panic("unused")
}
func (repo *askTestRepository) UpdateModelProvider(context.Context, UpdateModelProviderCommand) (domain.ModelProvider, error) {
	panic("unused")
}
func (repo *askTestRepository) CreateModelSetting(context.Context, domain.ModelSetting) (domain.ModelSetting, error) {
	panic("unused")
}
func (repo *askTestRepository) GetModelSetting(context.Context, identity.WorkspaceID, identity.ModelSettingID) (domain.ModelSetting, error) {
	panic("unused")
}
func (repo *askTestRepository) ListModelSettings(context.Context, identity.WorkspaceID) ([]domain.ModelSetting, error) {
	panic("unused")
}
func (repo *askTestRepository) UpdateModelSetting(context.Context, UpdateModelSettingCommand) (domain.ModelSetting, error) {
	panic("unused")
}
func (repo *askTestRepository) SetDefaultModelSetting(context.Context, identity.WorkspaceID, identity.ModelSettingID, time.Time) (domain.ModelSetting, error) {
	panic("unused")
}
func (repo *askTestRepository) GetDefaultModelSetting(context.Context, identity.WorkspaceID, domain.ModelKind) (domain.ModelSetting, error) {
	return repo.setting, nil
}

func (repo *askTestRepository) CreateConsumer(context.Context, distributiondomain.Consumer) (distributiondomain.Consumer, error) {
	panic("unused")
}
func (repo *askTestRepository) GetConsumer(context.Context, identity.WorkspaceID, identity.ConsumerID) (distributiondomain.Consumer, error) {
	panic("unused")
}
func (repo *askTestRepository) ListConsumers(context.Context, identity.WorkspaceID) ([]distributiondomain.Consumer, error) {
	panic("unused")
}
func (repo *askTestRepository) UpdateConsumer(context.Context, distributiondomain.Consumer) (distributiondomain.Consumer, error) {
	panic("unused")
}
func (repo *askTestRepository) CreateConsumerBinding(context.Context, distributiondomain.ConsumerBinding) (distributiondomain.ConsumerBinding, error) {
	panic("unused")
}
func (repo *askTestRepository) GetConsumerBinding(context.Context, identity.WorkspaceID, identity.ConsumerBindingID) (distributiondomain.ConsumerBinding, error) {
	panic("unused")
}
func (repo *askTestRepository) ListConsumerBindings(context.Context, identity.WorkspaceID) ([]distributiondomain.ConsumerBinding, error) {
	panic("unused")
}
func (repo *askTestRepository) UpdateConsumerBinding(context.Context, distributiondomain.ConsumerBinding, int) (distributiondomain.ConsumerBinding, error) {
	panic("unused")
}
func (repo *askTestRepository) CurrentReleaseSnapshot(context.Context, identity.WorkspaceID) (distributiondomain.ReleaseSnapshot, error) {
	if repo.current.ReleaseID.IsZero() {
		return distributiondomain.ReleaseSnapshot{}, distributiondomain.ErrNotFound
	}
	return repo.current, nil
}
func (repo *askTestRepository) ReleaseSnapshot(context.Context, identity.WorkspaceID, identity.ReleaseID) (distributiondomain.ReleaseSnapshot, error) {
	return distributiondomain.ReleaseSnapshot{}, distributiondomain.ErrNotFound
}
func (repo *askTestRepository) RecordResolution(_ context.Context, record distributionapp.ResolutionRecord) error {
	repo.record = record
	return nil
}
func (repo *askTestRepository) GetSemanticQuery(context.Context, identity.WorkspaceID, identity.SemanticQueryID) (distributionapp.ResolutionResult, error) {
	return distributionapp.ResolutionResult{}, distributiondomain.ErrNotFound
}
func (repo *askTestRepository) GetResolutionByIdempotency(context.Context, identity.WorkspaceID, string, string) (distributionapp.ResolutionResult, error) {
	return distributionapp.ResolutionResult{}, distributiondomain.ErrNotFound
}
func (repo *askTestRepository) GetResolvedSemanticPlan(context.Context, identity.WorkspaceID, identity.ResolvedSemanticPlanID) (distributiondomain.ResolvedSemanticPlan, error) {
	return distributiondomain.ResolvedSemanticPlan{}, distributiondomain.ErrNotFound
}
