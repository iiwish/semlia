package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	app "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/governance/llm"
	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/internal/domain"
	gov "github.com/iiwish/semlia/internal/domain/governance"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/sdk/trace"
)

// TestProductionAutoFinalizeChain proves the hands-free path from a queued
// generation to review-pending knowledge: apply suggestion, submit,
// validation runs, the only human step is the business-rule confirmation
// that AI drafts may never self-confirm, then re-validation succeeds and
// proposals wait for review. No operator clicks apply, submit, or validate.
func TestProductionAutoFinalizeChain(t *testing.T) {
	testProductionAutoFinalizeChain(t, false)
}

func TestProductionAutoFinalizeResumesAfterEnqueueFailure(t *testing.T) {
	testProductionAutoFinalizeChain(t, true)
}

type failRecheckEnqueueOnce struct {
	app.AutoFinalizeRepository
	failed bool
}

func (repo *failRecheckEnqueueOnce) EnqueueJob(ctx context.Context, params jobs.EnqueueJobParams) (jobs.Job, error) {
	if !repo.failed {
		repo.failed = true
		return jobs.Job{}, errors.New("injected recheck enqueue failure")
	}
	return repo.AutoFinalizeRepository.EnqueueJob(ctx, params)
}

func testProductionAutoFinalizeChain(t *testing.T, retryEnqueue bool) {
	t.Helper()
	f := newFixture(t)
	w := createWorkspace(t, f.pool, "production-autofinalize")
	author := authoringLifecyclePrincipal(t, f, w, "Chain author")
	ctx := context.Background()

	input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{}, true)
	businessEvidence := input["evidence"].([]any)[0].(map[string]any)["evidenceId"]
	target := map[string]any{"localKey": "orders", "title": "Orders", "kind": "semantic_asset",
		"intent": "create", "identityKey": "sales.orders", "changes": []any{}, "evidenceIds": []any{businessEvidence},
		"content": map[string]any{"address": "sales.orders", "assetType": "business_object", "displayName": "Orders",
			"definition": nil, "scope": nil, "ownerPrincipalId": author.String()}}
	payload := map[string]any{"input": input, "targets": []any{target}}

	system := application.NewSystemService(application.ReadinessProbeFunc(func(context.Context) error { return nil }), domain.SystemInfo{})
	handler := httpapi.NewHandler(system, slog.New(slog.NewTextHandler(io.Discard, nil)), trace.NewTracerProvider().Tracer("autofinalize-chain"),
		httpapi.WithProduction(app.NewProductionService(f.store)),
		httpapi.WithAuthorization(authorizationapp.NewService(f.store, authorizationapp.ClockFunc(time.Now))))
	body, _ := json.Marshal(payload)
	created := sendProdRequest(handler, http.MethodPost, "/api/v1/workspaces/"+w.String()+"/production-operations", author.String(), "chain-create", string(body))
	if created.Code != http.StatusCreated {
		t.Fatalf("create draft: %d %s", created.Code, created.Body.String())
	}
	var command struct {
		OperationID identity.ProductionOperationID `json:"operationId"`
		Version     int                            `json:"version"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &command); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	var stubOutput atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		output, _ := stubOutput.Load().(string)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": output}, "finish_reason": "stop"}}})
	}))
	t.Cleanup(server.Close)
	now := time.Now().UTC()
	url := server.URL
	provider, err := f.store.CreateModelProvider(ctx, gov.ModelProvider{ID: mustID(t, identity.NewModelProviderID), WorkspaceID: w, Protocol: gov.ProtocolOpenAICompatible, DisplayName: "Chain stub", BaseURL: &url, CredentialEnv: "PROTOCOL_STUB_ONLY", CredentialRevision: "sha256:" + strings.Repeat("a", 64), Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	setting, err := f.store.CreateModelSetting(ctx, gov.ModelSetting{ID: mustID(t, identity.NewModelSettingID), WorkspaceID: w, ProviderID: provider.ID, Kind: gov.ModelKindLLM, Model: "chain-fixture", Capability: "production suggestions", Enabled: true, TokenLimit: 2048, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := gov.ProductionGenerationModelRevision(setting, provider)
	if err != nil {
		t.Fatal(err)
	}
	grant := gov.ProductionGenerationGrant{WorkspaceID: w, PrincipalID: author, ModelSettingID: setting.ID, ModelConfigRevision: revision, PricingBasis: "deterministic chain test, no paid calls", MaxInputBytes: 1 << 20, MaxOutputTokens: 2048, MaxCostMicros: 2000000, InputMicrosPerByte: 1, OutputMicrosPerToken: 1}
	generation := app.NewProductionGenerationService(f.store, []gov.ProductionGenerationGrant{grant}, app.WithProductionGenerationProtocolStub(func(p gov.ModelProvider, _ llm.CredentialResolver, _ *http.Client) (llm.ProviderClient, error) {
		return llm.NewProviderClient(p, func(context.Context, string) (string, error) { return "protocol-stub-only", nil }, server.Client())
	}))

	_, ver, _, _, _, err := f.store.GetProductionOperation(ctx, w, command.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	filled := map[string]any{"localKey": "orders", "title": "Orders", "kind": "semantic_asset",
		"intent": "create", "identityKey": "sales.orders", "changes": []any{}, "evidenceIds": []any{businessEvidence},
		"content": map[string]any{"address": "sales.orders", "assetType": "business_object", "displayName": "Orders",
			"definition": "One row per customer order", "scope": "Online sales orders", "spec": syntheticObjectSpec(), "ownerPrincipalId": author.String()}}
	rawOutput, _ := json.Marshal(map[string]any{"schemaVersion": gov.ProductionSuggestionsSchema, "targets": []any{filled}})
	stubOutput.Store(string(rawOutput))

	queued, err := generation.Generate(ctx, gov.ProductionGenerationRequest{WorkspaceID: w, PrincipalID: author, OperationID: command.OperationID, ExpectedVersion: command.Version, InputDigest: ver.InputDigest, ModelSettingID: setting.ID, ModelConfigRevision: revision, Instruction: "Fill definition and scope; keep business rules unresolved.", MaxOutputTokens: 1024, MaxCostMicros: 2000000, TraceID: traceID, IdempotencyKey: "chain-generation-001"})
	if err != nil {
		t.Fatalf("queue generation: %v", err)
	}
	worker := jobs.NewWorker(f.store, jobs.ClockFunc(time.Now), jobs.BackoffFunc(func(int32) time.Duration { return time.Millisecond }), time.Minute)
	worker.Register(app.ProductionGenerationJobType, generation.JobHandler())
	if processed, err := worker.RunOne(ctx, "chain-generation"); err != nil || !processed {
		t.Fatalf("run generation: %v %v", processed, err)
	}
	result, err := generation.Get(ctx, w, author, command.OperationID, queued.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || calls.Load() != 1 {
		t.Fatalf("generation: %+v calls=%d", result, calls.Load())
	}

	var finalizeRepo app.AutoFinalizeRepository = f.store
	if retryEnqueue {
		finalizeRepo = &failRecheckEnqueueOnce{AutoFinalizeRepository: f.store}
	}
	finalizeService := app.NewProductionAutoFinalizeService(finalizeRepo, app.NewProductionService(f.store), app.ClockFunc(time.Now), slog.New(slog.NewTextHandler(io.Discard, nil)))
	finalizePayload, _ := json.Marshal(map[string]any{"workspaceId": w.String(), "operationId": command.OperationID.String(), "version": command.Version, "runId": queued.RunID.String(), "principalId": author.String(), "phase": "finalize", "round": 0})
	if retryEnqueue {
		if err := finalizeService.JobHandler()(ctx, jobs.Job{WorkspaceID: w, Payload: finalizePayload, Attempt: 1, MaxAttempts: 5}); err == nil {
			t.Fatal("expected injected enqueue failure")
		}
	}
	if err := finalizeService.JobHandler()(ctx, jobs.Job{WorkspaceID: w, Payload: finalizePayload, Attempt: 1, MaxAttempts: 5}); err != nil {
		t.Fatalf("auto-finalize: %v", err)
	}
	_, applied, _, _, _, err := f.store.GetProductionOperation(ctx, w, command.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Version != 2 || applied.FrozenAt == nil {
		t.Fatalf("draft not applied and submitted: version=%d frozen=%v", applied.Version, applied.FrozenAt != nil)
	}

	validationJob := chainValidationJob(t, f, w, command.OperationID)
	if err := runProductionValidationJob(t, f, validationJob); err != nil {
		t.Fatalf("validation run: %v", err)
	}
	if status := chainAttemptStatus(t, f, w, command.OperationID, 2, 1); status != "failed" {
		t.Fatalf("attempt 1 status: %s", status)
	}
	var seals []byte
	if err := f.pool.QueryRow(ctx, `SELECT results_json FROM production_validation_seals WHERE workspace_id=$1 AND operation_id=$2 AND production_version=2 AND attempt_no=1`, w.UUID(), command.OperationID.UUID()).Scan(&seals); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(seals), "PRODUCTION_BUSINESS_RULE_UNCONFIRMED") {
		t.Fatalf("attempt 1 must block on unconfirmed business rules: %s", seals)
	}

	createdMap := map[string]any{"operationId": command.OperationID.String()}
	confirmProductionFixtureRules(t, f, handler, w, author, createdMap)

	var recheckPayload []byte
	if err := f.pool.QueryRow(ctx, `SELECT payload FROM jobs WHERE workspace_id=$1 AND job_type='production.autofinalize' AND payload->>'operationId'=$2 AND payload->>'phase'='recheck' ORDER BY created_at DESC LIMIT 1`, w.UUID(), command.OperationID.String()).Scan(&recheckPayload); err != nil {
		t.Fatal(err)
	}
	if err := finalizeService.JobHandler()(ctx, jobs.Job{WorkspaceID: w, Payload: recheckPayload, Attempt: 1, MaxAttempts: 5}); err != nil {
		t.Fatalf("auto recheck: %v", err)
	}
	if status := chainAttemptStatus(t, f, w, command.OperationID, 2, 2); status != "queued" && status != "running" {
		t.Fatalf("attempt 2 not queued by recheck: %s", status)
	}
	validationJob = chainValidationJob(t, f, w, command.OperationID)
	if err := runProductionValidationJob(t, f, validationJob); err != nil {
		t.Fatalf("validation rerun: %v", err)
	}
	if status := chainAttemptStatus(t, f, w, command.OperationID, 2, 2); status != "succeeded" {
		t.Fatalf("attempt 2 status: %s", status)
	}
	var pending int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM proposals WHERE workspace_id=$1 AND production_operation_id=$2 AND state='in_review'`, w.UUID(), command.OperationID.UUID()).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("review-pending proposals: %d %v", pending, err)
	}
}

func chainValidationJob(t *testing.T, f *fixture, w identity.WorkspaceID, op identity.ProductionOperationID) jobs.Job {
	t.Helper()
	job := jobs.Job{WorkspaceID: w, Attempt: 1, MaxAttempts: 3}
	if err := f.pool.QueryRow(context.Background(), `SELECT payload,trace_id FROM jobs WHERE workspace_id=$1 AND job_type='governance.proposal.validate' AND payload->>'operationId'=$2 ORDER BY available_at DESC, created_at DESC LIMIT 1`, w.UUID(), op.String()).Scan(&job.Payload, &job.TraceID); err != nil {
		t.Fatal(err)
	}
	return job
}

func chainAttemptStatus(t *testing.T, f *fixture, w identity.WorkspaceID, op identity.ProductionOperationID, version, attempt int) string {
	t.Helper()
	var status string
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM production_validation_attempts WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3 AND attempt_no=$4`, w.UUID(), op.UUID(), version, attempt).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}
