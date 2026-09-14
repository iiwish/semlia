package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	app "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/governance/llm"
	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/internal/domain"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	gov "github.com/iiwish/semlia/internal/domain/governance"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/sdk/trace"
)

func TestProductionModelCallReservationRejectsUnknownAndConcurrent(t *testing.T) {
	for _, result := range []string{"", `{`, `{"status":"outcome_unknown"}`, `{"status":"running"}`, `{"status":"failed","errorCode":"GENERATION_OUTCOME_UNKNOWN"}`} {
		t.Run(result, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "call-01.reserved"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			if result != "" {
				if err := os.WriteFile(filepath.Join(root, "call-01-result.json"), []byte(result), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, release, err := reserveProductionModelCall(root); err == nil {
				release()
				t.Fatal("unresolved prior invocation allowed another reservation")
			}
			if _, err := os.Stat(filepath.Join(root, "call-02.reserved")); !os.IsNotExist(err) {
				t.Fatal("rejected invocation consumed another slot")
			}
		})
	}
	t.Run("active invocation", func(t *testing.T) {
		root := t.TempDir()
		slot, release, err := reserveProductionModelCall(root)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if err := os.WriteFile(slot+"-result.json", []byte(`{"status":"succeeded"}`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, unlock, err := reserveProductionModelCall(root); err == nil {
			unlock()
			t.Fatal("active invocation allowed concurrent reservation")
		}
		release()
		if _, unlock, err := reserveProductionModelCall(root); err != nil {
			t.Fatal("completed invocation blocked next authorized call", err)
		} else {
			unlock()
		}
	})
	t.Run("thirty completed calls", func(t *testing.T) {
		root := t.TempDir()
		for n := 1; n <= 30; n++ {
			prefix := filepath.Join(root, fmt.Sprintf("call-%02d", n))
			if err := os.WriteFile(prefix+".reserved", nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(prefix+"-result.json", []byte(`{"status":"succeeded"}`), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, release, err := reserveProductionModelCall(root); err == nil {
			release()
			t.Fatal("thirty-call budget exceeded")
		}
	})
}

type productionGenerationFixture struct {
	f              *fixture
	w              identity.WorkspaceID
	actor, agent   identity.PrincipalID
	op             identity.ProductionOperationID
	h              http.Handler
	service        *app.ProductionGenerationService
	request        gov.ProductionGenerationRequest
	grant          gov.ProductionGenerationGrant
	payload        map[string]any
	output         string
	calls          atomic.Int32
	beforeResponse func()
}

func newProductionGenerationFixture(t *testing.T) *productionGenerationFixture {
	t.Helper()
	g := &productionGenerationFixture{f: newFixture(t)}
	g.w = createWorkspace(t, g.f.pool, "production-generation")
	g.actor = authoringLifecyclePrincipal(t, g.f, g.w, "Generation author")
	agent, agentErr := g.f.store.WorkspaceAgentPrincipal(context.Background(), g.w)
	if agentErr != nil {
		t.Fatal(agentErr)
	}
	g.agent = agent.ID
	if _, err := g.f.store.CreateRoleBinding(context.Background(), auth.RoleBinding{ID: mustID(t, identity.NewBindingID), PrincipalID: g.agent, RoleID: "semantic_steward", ScopeType: auth.ScopeWorkspace, ScopeID: g.w.UUID()}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.f.store.CreateRoleBinding(context.Background(), auth.RoleBinding{ID: mustID(t, identity.NewBindingID), PrincipalID: g.agent, RoleID: "source_operator", ScopeType: auth.ScopeWorkspace, ScopeID: g.w.UUID()}); err != nil {
		t.Fatal(err)
	}
	input := productionFixtureInput(t, g.f, g.w, identity.SemanticCandidateID{})
	target := map[string]any{"localKey": "orders", "title": "Orders", "kind": "semantic_asset", "intent": "create", "identityKey": "sales.orders", "changes": []any{}, "evidenceIds": []any{}, "content": map[string]any{"address": "sales.orders", "assetType": "entity", "displayName": "Orders", "definition": nil, "scope": nil, "ownerPrincipalId": g.actor.String()}}
	g.payload = map[string]any{"input": input, "targets": []any{target}}
	body, _ := json.Marshal(g.payload)
	created := sendProdRequest(newProductionHandler(t, g.f.store), http.MethodPost, "/api/v1/workspaces/"+g.w.String()+"/production-operations", g.actor.String(), "generation-fixture", string(body))
	if created.Code != 201 {
		t.Fatalf("create generation skeleton: %d %s", created.Code, created.Body.String())
	}
	var command struct {
		OperationID identity.ProductionOperationID `json:"operationId"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &command); err != nil {
		t.Fatal(err)
	}
	g.op = command.OperationID
	_, ver, _, _, _, err := g.f.store.GetProductionOperation(context.Background(), g.w, g.op)
	if err != nil {
		t.Fatal(err)
	}
	target["content"].(map[string]any)["definition"] = "One order per identifier"
	target["content"].(map[string]any)["scope"] = "Sales orders"
	output, _ := json.Marshal(map[string]any{"schemaVersion": gov.ProductionSuggestionsSchema, "targets": []any{target}})
	g.output = string(output)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.calls.Add(1)
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer protocol-stub-only" {
			t.Errorf("unexpected protocol request")
		}
		var body struct {
			Messages  []llm.Message `json:"messages"`
			MaxTokens int           `json:"max_tokens"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Messages) != 2 || body.Messages[0].Role != "system" || !strings.Contains(body.Messages[0].Content, "untrusted") || !strings.Contains(body.Messages[1].Content, "sourceFacts") || body.MaxTokens != 1024 {
			t.Error("invalid bounded prompt")
		}
		if g.beforeResponse != nil {
			g.beforeResponse()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": g.output}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 100}})
	}))
	t.Cleanup(server.Close)
	now := time.Now().UTC()
	url := server.URL
	provider, err := g.f.store.CreateModelProvider(context.Background(), gov.ModelProvider{ID: mustID(t, identity.NewModelProviderID), WorkspaceID: g.w, Protocol: gov.ProtocolOpenAICompatible, DisplayName: "Protocol stub", BaseURL: &url, CredentialEnv: "PROTOCOL_STUB_ONLY", CredentialRevision: "sha256:" + strings.Repeat("a", 64), Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	setting, err := g.f.store.CreateModelSetting(context.Background(), gov.ModelSetting{ID: mustID(t, identity.NewModelSettingID), WorkspaceID: g.w, ProviderID: provider.ID, Kind: gov.ModelKindLLM, Model: "protocol-fixture", Capability: "production suggestions", Enabled: true, TokenLimit: 2048, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := gov.ProductionGenerationModelRevision(setting, provider)
	if err != nil {
		t.Fatal(err)
	}
	g.grant = gov.ProductionGenerationGrant{WorkspaceID: g.w, PrincipalID: g.actor, ModelSettingID: setting.ID, ModelConfigRevision: revision, PricingBasis: "deterministic protocol test, no paid calls", MaxInputBytes: 1 << 20, MaxOutputTokens: 2048, MaxCostMicros: 2000000, InputMicrosPerByte: 1, OutputMicrosPerToken: 1}
	g.service = app.NewProductionGenerationService(g.f.store, []gov.ProductionGenerationGrant{g.grant}, app.WithProductionGenerationProtocolStub(func(p gov.ModelProvider, _ llm.CredentialResolver, _ *http.Client) (llm.ProviderClient, error) {
		return llm.NewProviderClient(p, func(context.Context, string) (string, error) { return "protocol-stub-only", nil }, server.Client())
	}))
	system := application.NewSystemService(application.ReadinessProbeFunc(func(context.Context) error { return nil }), domain.SystemInfo{APIVersion: "v1", SchemaVersion: "0.4.0", BuildVersion: "protocol-stub"})
	g.h = httpapi.NewHandler(system, slog.New(slog.NewTextHandler(io.Discard, nil)), trace.NewTracerProvider().Tracer("production-generation"), httpapi.WithProduction(app.NewProductionService(g.f.store)), httpapi.WithProductionGeneration(g.service), httpapi.WithAuthorization(authapp.NewService(g.f.store, authapp.ClockFunc(time.Now))))
	g.request = gov.ProductionGenerationRequest{WorkspaceID: g.w, PrincipalID: g.actor, OperationID: g.op, IdempotencyKey: "generation-run-001", ExpectedVersion: 1, InputDigest: ver.InputDigest, ModelSettingID: setting.ID, ModelConfigRevision: revision, Instruction: "Suggest a definition; do not invent missing business rules.", MaxOutputTokens: 1024, MaxCostMicros: 2000000, TraceID: traceID}
	return g
}

func (g *productionGenerationFixture) path() string {
	return "/api/v1/workspaces/" + g.w.String() + "/production-operations/" + g.op.String()
}
func (g *productionGenerationFixture) queue(t *testing.T) gov.ProductionGenerationResult {
	t.Helper()
	raw, _ := json.Marshal(g.request)
	r := sendProdRequest(g.h, http.MethodPost, g.path()+"/generation", g.actor.String(), g.request.IdempotencyKey, string(raw))
	if r.Code != 202 {
		t.Fatalf("generation queue: %d %s", r.Code, r.Body.String())
	}
	var result gov.ProductionGenerationResult
	if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func (g *productionGenerationFixture) run(t *testing.T) {
	t.Helper()
	worker := jobs.NewWorker(g.f.store, jobs.ClockFunc(time.Now), jobs.BackoffFunc(func(int32) time.Duration { return time.Millisecond }), time.Minute)
	worker.Register(app.ProductionGenerationJobType, g.service.JobHandler())
	processed, err := worker.RunOne(context.Background(), "production-generation-test")
	if err != nil || !processed {
		t.Fatalf("generation worker: %v %v", processed, err)
	}
}
func (g *productionGenerationFixture) get(t *testing.T, run identity.AgentRunID) gov.ProductionGenerationResult {
	t.Helper()
	r := sendProdRequest(g.h, http.MethodGet, g.path()+"/generation/"+run.String(), g.actor.String(), "", "")
	if r.Code != 200 {
		t.Fatalf("read generation: %d %s", r.Code, r.Body.String())
	}
	var result gov.ProductionGenerationResult
	if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProductionGenerationQueuedRunAndReplay(t *testing.T) {
	g := newProductionGenerationFixture(t)
	queued := g.queue(t)
	if queued.Status != "queued" || string(queued.Output) != "null" || queued.OutputDigest != nil || queued.ProviderMode != "protocol_stub" || g.calls.Load() != 0 {
		t.Fatalf("queue is not generation success: %+v", queued)
	}
	g.run(t)
	result := g.get(t, queued.RunID)
	if result.Status != "succeeded" || result.OutputDigest == nil || len(result.Output) == 0 || result.CostMicros != nil || result.DurationMS == nil || g.calls.Load() != 1 {
		t.Fatalf("generation result: %+v calls=%d", result, g.calls.Load())
	}
	raw, _ := json.Marshal(g.request)
	replay := sendProdRequest(g.h, http.MethodPost, g.path()+"/generation", g.actor.String(), g.request.IdempotencyKey, string(raw))
	if replay.Code != 200 || !strings.Contains(replay.Body.String(), `"replayed":true`) || g.calls.Load() != 1 {
		t.Fatalf("generation replay: %d %s", replay.Code, replay.Body.String())
	}
	assertTableCount(t, g.f.pool, "production_generation_outputs", 1)
	assertTableCount(t, g.f.pool, "releases", 0)
	assertTableCount(t, g.f.pool, "proposals", 1)
	_, ver, _, _, _, err := g.f.store.GetProductionOperation(context.Background(), g.w, g.op)
	if err != nil {
		t.Fatal(err)
	}
	if ver.Version != 1 || ver.FrozenAt != nil {
		t.Fatal("generation automatically changed draft")
	}
}

func TestProductionGenerationInvalidOutputCreatesNoSuccess(t *testing.T) {
	g := newProductionGenerationFixture(t)
	g.output = `{"approved":true,"schemaVersion":"semlia.production-suggestions/v1","targets":[]}`
	queued := g.queue(t)
	g.run(t)
	result := g.get(t, queued.RunID)
	if result.Status != "failed" || result.ErrorCode == nil || *result.ErrorCode != "AI_OUTPUT_INVALID" || string(result.Output) != "null" || result.OutputDigest != nil {
		t.Fatalf("invalid output: %+v", result)
	}
	assertTableCount(t, g.f.pool, "production_generation_outputs", 0)
	assertTableCount(t, g.f.pool, "proposals", 1)
}

func TestProductionGenerationHumanApplicationIsVersioned(t *testing.T) {
	g := newProductionGenerationFixture(t)
	queued := g.queue(t)
	g.run(t)
	original := g.get(t, queued.RunID)
	if original.Status != "succeeded" {
		t.Fatalf("generation did not succeed: %+v", original)
	}
	g.payload["expectedVersion"] = 1
	g.payload["suggestionRunId"] = queued.RunID.String()
	g.payload["targets"].([]any)[0].(map[string]any)["content"].(map[string]any)["definition"] = "Human-corrected order definition"
	raw, _ := json.Marshal(g.payload)
	response := sendProdRequest(g.h, http.MethodPut, g.path(), g.actor.String(), "apply-suggestion-001", string(raw))
	if response.Code != 200 {
		t.Fatalf("apply suggestion: %d %s", response.Code, response.Body.String())
	}
	read := sendProdRequest(g.h, http.MethodGet, g.path()+"?version=2", g.actor.String(), "", "")
	if read.Code != 200 {
		t.Fatalf("read applied version: %d %s", read.Code, read.Body.String())
	}
	var applied struct {
		GenerationRunIDs []string                              `json:"generationRunIds"`
		Applications     []gov.ProductionGenerationApplication `json:"generationApplications"`
	}
	if json.Unmarshal(read.Body.Bytes(), &applied) != nil || len(applied.Applications) != 1 || len(applied.GenerationRunIDs) != 1 {
		t.Fatalf("missing application provenance: %s", read.Body.String())
	}
	a := applied.Applications[0]
	if a.SourceVersion != 1 || a.AppliedVersion != 2 || a.RunID != queued.RunID || a.ActorPrincipalID != g.actor || len(a.Delta) != 1 || a.Delta[0].Change.FieldPath != "content.definition" {
		t.Fatalf("wrong human delta: %+v", a)
	}
	if again := g.get(t, queued.RunID); string(again.Output) != string(original.Output) || *again.OutputDigest != *original.OutputDigest {
		t.Fatal("human correction changed original output")
	}
	replay := sendProdRequest(g.h, http.MethodPut, g.path(), g.actor.String(), "apply-suggestion-001", string(raw))
	if replay.Code != 200 || !strings.Contains(replay.Body.String(), `"replayed":true`) {
		t.Fatalf("application replay: %d %s", replay.Code, replay.Body.String())
	}
	assertTableCount(t, g.f.pool, "production_generation_applications", 1)
	assertTableCount(t, g.f.pool, "releases", 0)
}
