package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	distapp "github.com/iiwish/semlia/internal/application/distribution"
	app "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/governance/llm"
	rootdomain "github.com/iiwish/semlia/internal/domain"
	dist "github.com/iiwish/semlia/internal/domain/distribution"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/trace/noop"
)

type askCallFunc func(context.Context, llm.CompleteRequest) (llm.CompleteResponse, error)

func (askCallFunc) Protocol() domain.ModelProviderProtocol { return domain.ProtocolOpenAI }
func (f askCallFunc) Complete(ctx context.Context, r llm.CompleteRequest) (llm.CompleteResponse, error) {
	return f(ctx, r)
}

func setupDurableAsk(t *testing.T) (*fixture, identity.WorkspaceID, identity.PrincipalID) {
	t.Helper()
	f := newFixture(t)
	w := createWorkspace(t, f.pool, "durable-ask")
	p, err := f.store.LoadDefaultPrincipal(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	provider, _ := createProvider(t, f, w, providerBody("openai", "Ask test provider", "", credentialEnvName))
	createSetting(t, f, w, provider, settingBody(provider, "llm", "ask-test-model", "Synthetic idempotency test", 8192, nil))
	return f, w, p.ID
}

func durableAskService(f *fixture, client llm.ProviderClient) *app.AskService {
	clock := app.ClockFunc(time.Now)
	auth := authapp.NewService(f.store, authapp.ClockFunc(time.Now))
	return app.NewAskService(f.store, app.NewModelConfigService(f.store, auth, clock), app.NewAgentRunService(f.store, clock), distapp.NewService(f.store, auth, distapp.ClockFunc(time.Now)), auth, app.WithAskProviderClientFactory(func(domain.ModelProvider, llm.CredentialResolver, *http.Client) (llm.ProviderClient, error) {
		return client, nil
	}))
}

func TestAskDurableConcurrentPrincipalAndRestartReplay(t *testing.T) {
	f, w, actor := setupDurableAsk(t)
	ctx := context.Background()
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	client := askCallFunc(func(ctx context.Context, r llm.CompleteRequest) (llm.CompleteResponse, error) {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return llm.CompleteResponse{}, ctx.Err()
			}
		}
		if r.MaxTokens != 8192 {
			t.Errorf("max tokens=%d", r.MaxTokens)
		}
		return llm.CompleteResponse{Content: `{"schema":"semlia.ask-interpretation/v1","outcome":"query","query":{"intent":"describe","measures":[{"search":"synthetic revenue"}],"dimensions":[],"filters":[],"order":[]}}`, FinishReason: "stop"}, nil
	})
	svc := durableAskService(f, client)
	r := app.AskRequest{WorkspaceID: w, PrincipalRef: actor.String(), Question: "describe synthetic revenue", IdempotencyKey: "same-key", TraceID: traceID}
	type answer struct {
		result app.AskResult
		err    error
	}
	done := make(chan answer, 1)
	go func() { result, err := svc.Ask(ctx, r); done <- answer{result, err} }()
	select {
	case <-entered:
	case early := <-done:
		t.Fatalf("request exited before provider: %v", early.err)
	case <-time.After(30 * time.Second):
		t.Fatal("provider was not entered")
	}
	_, err := durableAskService(f, client).Ask(ctx, r)
	var state *app.AskStateError
	if !errors.As(err, &state) || state.Code != "ASK_IN_PROGRESS" {
		t.Fatalf("concurrent request=%v", err)
	}
	close(release)
	first := <-done
	if first.err != nil {
		t.Fatal(first.err)
	}
	second, err := durableAskService(f, client).Ask(ctx, r)
	if err != nil || calls.Load() != 1 || second.Run.ID != first.result.Run.ID || second.Resolution.Query.ID != first.result.Resolution.Query.ID {
		t.Fatalf("restart replay: err=%v calls=%d", err, calls.Load())
	}
	changed := r
	changed.Question = "different question"
	if _, err := svc.Ask(ctx, changed); !errors.As(err, &state) || state.Code != "ASK_KEY_CONFLICT" {
		t.Fatalf("changed question=%v", err)
	}
	other := createPrincipalWithRoles(t, f, w, "Other consumer", []string{"consumer_developer"})
	r.PrincipalRef = other.String()
	third, err := svc.Ask(ctx, r)
	if err != nil || calls.Load() != 2 || third.Run.ID == first.result.Run.ID || third.Resolution.Query.ID == first.result.Resolution.Query.ID {
		t.Fatalf("principal isolation: err=%v calls=%d", err, calls.Load())
	}
	if _, err := f.pool.Exec(ctx, `UPDATE principals SET status='suspended' WHERE workspace_id=$1 AND id=$2`, w.UUID(), other.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Ask(ctx, r); err == nil || calls.Load() != 2 {
		t.Fatalf("revoked replay accepted: %v", err)
	}
	var requests, runs, queries int
	if err := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ask_requests WHERE workspace_id=$1),(SELECT count(*) FROM agent_runs WHERE workspace_id=$1),(SELECT count(*) FROM semantic_queries WHERE workspace_id=$1)`, w.UUID()).Scan(&requests, &runs, &queries); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || runs != 2 || queries != 2 {
		t.Fatalf("requests=%d runs=%d queries=%d", requests, runs, queries)
	}
}

func TestAskDurableCancellationClarificationAndFailedReplay(t *testing.T) {
	f, w, actor := setupDurableAsk(t)
	for _, scenario := range []string{"cancelled", "clarification", "failed", "truncated", "filtered"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			svc := durableAskService(f, askCallFunc(func(context.Context, llm.CompleteRequest) (llm.CompleteResponse, error) {
				calls.Add(1)
				if scenario == "cancelled" {
					cancel()
					return llm.CompleteResponse{}, context.Canceled
				}
				if scenario == "failed" {
					return llm.CompleteResponse{}, &llm.ProviderUnavailableError{}
				}
				reason := "stop"
				if scenario == "truncated" {
					reason = "length"
				}
				if scenario == "filtered" {
					reason = "content_filter"
				}
				return llm.CompleteResponse{Content: `{"schema":"semlia.ask-interpretation/v1","outcome":"clarification","clarification":"RAW_PRIVATE_SENTINEL"}`, FinishReason: reason}, nil
			}))
			r := app.AskRequest{WorkspaceID: w, PrincipalRef: actor.String(), Question: "RAW_QUESTION_SENTINEL", IdempotencyKey: scenario, TraceID: traceID}
			first, firstErr := svc.Ask(ctx, r)
			second, secondErr := svc.Ask(context.Background(), r)
			if calls.Load() != 1 || (firstErr == nil) != (secondErr == nil) {
				t.Fatalf("first=%v second=%v calls=%d", firstErr, secondErr, calls.Load())
			}
			if scenario == "clarification" && (first.Run.ID != second.Run.ID || strings.Contains(second.Interpretation.Clarification, "RAW_PRIVATE")) {
				t.Fatal("unsafe clarification replay")
			}
			var status, runStatus, stored string
			if err := f.pool.QueryRow(context.Background(), `SELECT r.status,a.status,row_to_json(r)::text FROM ask_requests r JOIN agent_runs a ON a.workspace_id=r.workspace_id AND a.id=r.agent_run_id WHERE r.workspace_id=$1 AND r.idempotency_key=$2`, w.UUID(), scenario).Scan(&status, &runStatus, &stored); err != nil {
				t.Fatal(err)
			}
			if status == "running" || runStatus == "running" || strings.Contains(stored, "RAW_") {
				t.Fatalf("unsettled or raw persisted result: %s/%s", status, runStatus)
			}
		})
	}
}

func TestAskClaimAtomicityAndExpiredFencing(t *testing.T) {
	f, w, actor := setupDurableAsk(t)
	ctx := context.Background()
	var calls atomic.Int32
	svc := durableAskService(f, askCallFunc(func(context.Context, llm.CompleteRequest) (llm.CompleteResponse, error) {
		calls.Add(1)
		return llm.CompleteResponse{}, nil
	}))
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION fail_ask_runtime_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='agent' THEN RAISE EXCEPTION 'synthetic failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_ask_runtime_test BEFORE INSERT ON runtime_runs FOR EACH ROW EXECUTE FUNCTION fail_ask_runtime_test()`); err != nil {
		t.Fatal(err)
	}
	r := app.AskRequest{WorkspaceID: w, PrincipalRef: actor.String(), Question: "atomic claim", IdempotencyKey: "atomic", TraceID: traceID}
	_, err := svc.Ask(ctx, r)
	if _, cleanupErr := f.pool.Exec(ctx, `DROP TRIGGER fail_ask_runtime_test ON runtime_runs; DROP FUNCTION fail_ask_runtime_test()`); cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
	if err == nil || calls.Load() != 0 {
		t.Fatalf("claim failure invoked provider: %v", err)
	}
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ask_requests WHERE workspace_id=$1)+(SELECT count(*) FROM agent_runs WHERE workspace_id=$1)`, w.UUID()).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial claim/run persisted: %d %v", count, err)
	}
	r.Context = dist.ResolutionContext{Mode: dist.ResolutionCurrent}
	payload, _ := json.Marshal(map[string]any{"schema": app.AskInterpretationSchemaID, "question": r.Question, "context": r.Context})
	digest, _ := domain.DigestJSON(payload)
	agent, err := f.store.WorkspaceAgentPrincipal(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.ClockFunc(func() time.Time { return time.Now().Add(-2 * time.Minute) })
	run, err := app.NewAgentRunService(f.store, clock).Start(ctx, app.StartAgentRunRequest{WorkspaceID: w, PrincipalID: &agent.ID, Model: "test", ConfigRevision: "test", InputHash: digest})
	if err != nil {
		t.Fatal(err)
	}
	token := mustID(t, identity.NewRunID)
	if _, err := f.pool.Exec(ctx, `INSERT INTO ask_requests(workspace_id,requested_by,idempotency_key,input_digest,knowledge_digest,agent_run_id,claim_token,call_deadline,status,created_at) VALUES($1,$2,$3,$4,$4,$5,$6,clock_timestamp()-interval '1 minute','running',clock_timestamp()-interval '2 minutes')`, w.UUID(), actor.UUID(), r.IdempotencyKey, digest, run.ID.UUID(), token.UUID()); err != nil {
		t.Fatal(err)
	}
	claim, err := f.store.GetAskRequest(ctx, w, actor, r.IdempotencyKey)
	if err != nil {
		t.Fatal(err)
	}
	forged := claim
	forged.ClaimToken = mustID(t, identity.NewRunID).UUID()
	if _, err := f.store.FinishAskRequest(ctx, app.AskFinishCommand{Request: forged, Status: "clarification", OutputDigest: digest}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("forged owner=%v", err)
	}
	if _, err := f.store.FinishAskRequest(ctx, app.AskFinishCommand{Request: claim, Status: "clarification", OutputDigest: digest}); err == nil {
		t.Fatal("late owner completed")
	}
	var state *app.AskStateError
	if _, err := f.store.FinishAskRequest(ctx, app.AskFinishCommand{Request: claim, Status: "clarification", OutputDigest: digest}); !errors.As(err, &state) || state.Code != "ASK_OUTCOME_UNKNOWN" {
		t.Fatalf("recorded expiry code changed for late owner: %v", err)
	}
	if _, err := svc.Ask(ctx, r); err == nil || calls.Load() != 0 {
		t.Fatalf("expired claim recalled provider: %v", err)
	}
	stored, err := f.store.GetAskRequest(ctx, w, actor, r.IdempotencyKey)
	if err != nil || stored.Status != "outcome_unknown" {
		t.Fatalf("expired record=%s %v", stored.Status, err)
	}
}

func TestAskReleasedPlanReplayAndContextChanges(t *testing.T) {
	f, w, actor := setupDurableAsk(t)
	ctx := context.Background()
	object, err := catalogapp.NewService(f.store, catalogapp.ClockFunc(time.Now)).CreateAsset(ctx, catalogapp.CreateAssetRequest{WorkspaceID: w, Address: "ask.order", AssetType: semantic.BusinessObject, Lifecycle: "active", SchemaVersion: "1.0.0", CreatedBy: actor.String(), TraceID: traceID, Content: json.RawMessage(`{"assetType":"business_object","name":"Order","definition":"Revenue after refunds","scope":"Synthetic Ask fixture","spec":{"grain":"one order","keys":["id"],"identityPolicy":"unique order id","lifecycle":"retained","members":[{"id":"id","name":"ID","valueType":"string","nullPolicy":"required","historyPolicy":"event_time"}]}}`)})
	if err != nil {
		t.Fatal(err)
	}
	asset := object.ID
	_, _, initialProposal := proposeToInReview(t, f, w, actor, asset, object.CurrentRevision.ID)
	initialReviewer := createReviewerPrincipal(t, f, w, "ask-initial-reviewer")
	initialPublisher := createPrincipalWithRoles(t, f, w, "ask-initial-publisher", []string{"publisher"})
	approveProposal(t, f, w, initialReviewer, initialProposal)
	if response := publishProposal(t, f, w, initialPublisher, initialProposal); response.Code != http.StatusCreated {
		t.Fatalf("initial release=%d %s", response.Code, response.Body.String())
	}
	snapshot, err := f.store.CurrentReleaseSnapshot(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	auth := authapp.NewService(f.store, authapp.ClockFunc(time.Now))
	plans := distapp.NewService(f.store, auth, distapp.ClockFunc(time.Now))
	consumer, err := plans.CreateConsumer(ctx, distapp.CreateConsumerRequest{WorkspaceID: w, StableKey: "ask-test", Name: "Ask integration", Kind: "application", PrincipalRef: actor.String(), OwnerPrincipalRef: actor.String(), TraceID: traceID})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := plans.CreateBinding(ctx, distapp.CreateBindingRequest{WorkspaceID: w, ConsumerID: consumer.ID, Environment: "test", Purpose: "Ask replay", Mode: dist.BindingPinned, ReleaseID: &snapshot.ReleaseID, PrincipalRef: actor.String(), TraceID: traceID})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	content, _ := json.Marshal(map[string]any{"schema": app.AskInterpretationSchemaID, "outcome": "query", "query": map[string]any{"intent": "describe", "measures": []any{}, "dimensions": []map[string]string{{"assetId": asset.String()}}, "filters": []any{}, "order": []any{}}})
	client := askCallFunc(func(context.Context, llm.CompleteRequest) (llm.CompleteResponse, error) {
		calls.Add(1)
		return llm.CompleteResponse{Content: string(content), FinishReason: "stop"}, nil
	})
	svc := durableAskService(f, client)
	contexts := []dist.ResolutionContext{{Mode: dist.ResolutionCurrent}, {Mode: dist.ResolutionExplicit, ReleaseID: &snapshot.ReleaseID}, {Mode: dist.ResolutionBinding, BindingID: &binding.ID}}
	requests := []app.AskRequest{}
	for _, contextValue := range contexts {
		r := app.AskRequest{WorkspaceID: w, PrincipalRef: actor.String(), Question: "define revenue", IdempotencyKey: string(contextValue.Mode), Context: contextValue, TraceID: traceID}
		requests = append(requests, r)
		first, err := svc.Ask(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		second, err := durableAskService(f, client).Ask(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		if first.Resolution == nil || first.Resolution.Plan == nil || second.Resolution.Plan == nil || first.Run.ID != second.Run.ID || first.Resolution.Query.ID != second.Resolution.Query.ID || first.Resolution.Plan.ID != second.Resolution.Plan.ID {
			t.Fatal("released plan replay changed identity or lacked plan")
		}
		if contextValue.Mode == dist.ResolutionBinding && (first.Resolution.Query.BindingID == nil || *first.Resolution.Query.BindingID != binding.ID) {
			t.Fatal("binding provenance lost")
		}
	}
	if _, err := f.pool.Exec(ctx, `UPDATE model_providers SET enabled=false WHERE workspace_id=$1`, w.UUID()); err != nil {
		t.Fatal(err)
	}
	for _, r := range requests {
		if _, err := durableAskService(f, client).Ask(ctx, r); err != nil {
			t.Fatalf("provider disabled replay=%v", err)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("replay calls=%d", calls.Load())
	}
	newAsset, newRevision := f.createAsset(t, w)
	_, _, proposal := proposeToInReview(t, f, w, actor, newAsset, newRevision)
	reviewer := createReviewerPrincipal(t, f, w, "ask-next-reviewer")
	publisher := createPrincipalWithRoles(t, f, w, "ask-next-publisher", []string{"publisher"})
	approveProposal(t, f, w, reviewer, proposal)
	if response := publishProposal(t, f, w, publisher, proposal); response.Code != http.StatusCreated {
		t.Fatalf("publish next release=%d %s", response.Code, response.Body.String())
	}
	var state *app.AskStateError
	if _, err := svc.Ask(ctx, requests[0]); !errors.As(err, &state) || state.Code != "ASK_CONTEXT_CHANGED" {
		t.Fatalf("current release change=%v", err)
	}
	for _, r := range requests[1:] {
		if _, err := svc.Ask(ctx, r); err != nil {
			t.Fatalf("pinned replay after current release changed=%v", err)
		}
	}
	_, err = plans.UpdateBinding(ctx, distapp.UpdateBindingRequest{WorkspaceID: w, BindingID: binding.ID, ExpectedVersion: binding.Version, Purpose: binding.Purpose, Mode: binding.Mode, ReleaseID: binding.ReleaseID, Status: dist.BindingSuspended, PrincipalRef: actor.String(), TraceID: traceID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Ask(ctx, requests[2]); err == nil {
		t.Fatal("suspended binding replayed")
	}
	if calls.Load() != 3 {
		t.Fatalf("context changes recalled provider=%d", calls.Load())
	}
}

func TestAskTerminalHTTPReplayCodeIsStable(t *testing.T) {
	f, w, actor := setupDurableAsk(t)
	for _, scenario := range []string{"provider", "invalid", "truncated", "access_changed"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			client := askCallFunc(func(ctx context.Context, _ llm.CompleteRequest) (llm.CompleteResponse, error) {
				calls.Add(1)
				if scenario == "provider" {
					return llm.CompleteResponse{}, &llm.ProviderUnavailableError{}
				}
				if scenario == "invalid" {
					return llm.CompleteResponse{Content: `{"RAW_PRIVATE_KEY":"RAW_PRIVATE_VALUE"}`, FinishReason: "stop"}, nil
				}
				if scenario == "access_changed" {
					if _, err := f.pool.Exec(ctx, `UPDATE principals SET status='suspended' WHERE workspace_id=$1 AND id=$2`, w.UUID(), actor.UUID()); err != nil {
						return llm.CompleteResponse{}, err
					}
				}
				reason := "stop"
				if scenario == "truncated" {
					reason = "length"
				}
				return llm.CompleteResponse{Content: `{"schema":"semlia.ask-interpretation/v1","outcome":"clarification","clarification":"private prose"}`, FinishReason: reason}, nil
			})
			svc := durableAskService(f, client)
			f.handler = httpapi.NewHandler(application.NewSystemService(nil, rootdomain.SystemInfo{}), slog.New(slog.NewTextHandler(io.Discard, nil)), noop.NewTracerProvider().Tracer("Ask HTTP test"), httpapi.WithAsk(svc), httpapi.WithDistribution(distapp.NewService(f.store, authapp.NewService(f.store, authapp.ClockFunc(time.Now)), distapp.ClockFunc(time.Now))))
			body, _ := json.Marshal(map[string]any{"question": "private question", "idempotencyKey": scenario, "context": map[string]string{"mode": "current"}})
			first := f.request(t, http.MethodPost, "/api/v1/workspaces/"+w.String()+"/ask", actor.String(), string(body))
			if _, err := f.pool.Exec(context.Background(), `UPDATE principals SET status='active' WHERE workspace_id=$1 AND id=$2`, w.UUID(), actor.UUID()); err != nil {
				t.Fatal(err)
			}
			second := f.request(t, http.MethodPost, "/api/v1/workspaces/"+w.String()+"/ask", actor.String(), string(body))
			var a, b struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(first.Body.Bytes(), &a); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(second.Body.Bytes(), &b); err != nil {
				t.Fatal(err)
			}
			if first.Code != second.Code || a.Code == "" || a.Code != b.Code || calls.Load() != 1 {
				t.Fatalf("first %d/%s replay %d/%s calls=%d", first.Code, a.Code, second.Code, b.Code, calls.Load())
			}
			if strings.Contains(first.Body.String(), "RAW_PRIVATE") || strings.Contains(second.Body.String(), "RAW_PRIVATE") {
				t.Fatal("unsafe schema diagnostic")
			}
		})
	}
}
