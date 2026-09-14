package governance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	mcpadapter "github.com/iiwish/semlia/internal/adapters/mcp"
	"github.com/iiwish/semlia/internal/application"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	distapp "github.com/iiwish/semlia/internal/application/distribution"
	execapp "github.com/iiwish/semlia/internal/application/execution"
	identityapp "github.com/iiwish/semlia/internal/application/identity"
	rootdomain "github.com/iiwish/semlia/internal/domain"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	dist "github.com/iiwish/semlia/internal/domain/distribution"
	d "github.com/iiwish/semlia/internal/domain/execution"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/sdk/trace"
)

type executionBearer struct{ token string }

func (b executionBearer) RoundTrip(r *http.Request) (*http.Response, error) {
	c := r.Clone(r.Context())
	c.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(c)
}

func proveExecutionChannels(t *testing.T, env *fixture, w identity.WorkspaceID, admin identity.PrincipalID, plans *distapp.Service, service *execapp.Service, spy *executionSpy, input dist.SemanticQueryInput, publishNext func()) {
	t.Helper()
	ctx := context.Background()
	authorizer := authapp.NewService(env.store, authapp.ClockFunc(time.Now))
	machine := identityapp.NewMachineService(env.store, authorizer, time.Now)
	principal, err := machine.CreatePrincipal(ctx, w, admin.String(), traceID, "Execution integration")
	if err != nil {
		t.Fatal(err)
	}
	grant, _, err := authorizer.CreateBinding(ctx, authapp.CreateRoleBindingRequest{AccessRequest: authapp.AccessRequest{WorkspaceID: w, PrincipalRef: admin.String(), TraceID: traceID}, PrincipalID: principal.ID, RoleID: "consumer_developer", ExpectedRoleVersion: 1, ScopeType: auth.ScopeWorkspace, ScopeID: w.UUID()})
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := plans.CreateConsumer(ctx, distapp.CreateConsumerRequest{WorkspaceID: w, StableKey: "execution-agent", Name: "Execution Agent", Kind: "agent", PrincipalRef: admin.String(), TraceID: traceID})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := plans.CreateBinding(ctx, distapp.CreateBindingRequest{WorkspaceID: w, ConsumerID: consumer.ID, Environment: "test", Purpose: "execution proof", Mode: dist.BindingCurrent, PrincipalRef: admin.String(), TraceID: traceID})
	if err != nil {
		t.Fatal(err)
	}
	issue := identityapp.IssueMachineCredential{WorkspaceID: w, Actor: admin.String(), TraceID: traceID, PrincipalID: principal.ID, ConsumerID: consumer.ID, BindingID: binding.ID, Name: "Explicit execution", AllowedActions: []auth.Action{auth.ActionAssetRead, auth.ActionSemanticResolve, auth.ActionSemanticExecute}, ScopeType: auth.ScopeWorkspace, ScopeID: w.String(), ExpiresAt: time.Now().Add(time.Hour)}
	issued, err := machine.Issue(ctx, issue)
	if err != nil {
		t.Fatal(err)
	}
	limit, err := machine.Authenticate(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	machineCtx := authapp.WithCredentialLimit(ctx, limit)
	resolved, err := plans.Resolve(machineCtx, distapp.ResolveRequest{WorkspaceID: w, PrincipalRef: principal.ID.String(), TraceID: traceID, Channel: "api", IdempotencyKey: "machine-execution-plan", Input: input})
	if err != nil || resolved.Plan == nil {
		t.Fatalf("machine plan: %v", err)
	}
	projected, _ := json.Marshal(distapp.Response(resolved))
	if bytes.Contains(projected, []byte("sourceLocator")) || bytes.Contains(projected, []byte("execution_facts")) {
		t.Fatal("public plan leaked private physical IR")
	}
	provider := trace.NewTracerProvider()
	defer provider.Shutdown(ctx)
	server := httptest.NewServer(httpapi.NewHandler(application.NewSystemService(nil, rootdomain.SystemInfo{}), slog.New(slog.NewTextHandler(io.Discard, nil)), provider.Tracer("execution-channels"), httpapi.WithDistribution(plans), httpapi.WithExecution(service), httpapi.WithMachineIdentity(machine), httpapi.WithMCP(mcpadapter.NewHandler(plans, service))))
	defer server.Close()
	endpoint := server.URL + "/api/v1/workspaces/" + w.String() + "/"
	client := &http.Client{Transport: executionBearer{issued.Token}}
	invoke := func(body string) (int, d.Result) {
		t.Helper()
		r, _ := http.NewRequest("POST", endpoint+"resolved-semantic-plans/"+resolved.Plan.ID.String()+":execute", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var out d.Result
		_ = json.NewDecoder(response.Body).Decode(&out)
		return response.StatusCode, out
	}
	request := execapp.Request{WorkspaceID: w, PlanID: resolved.Plan.ID, PlanDigest: resolved.Plan.PlanDigest, IdempotencyKey: "channel-rest", Channel: "api", PrincipalRef: principal.ID.String(), TraceID: traceID}
	body, _ := json.Marshal(request)
	status, rest := invoke(string(body))
	if status != 200 || rest.Run.State != "succeeded" {
		t.Fatalf("REST execution: %d %s %s", status, rest.Run.State, rest.Run.ErrorCode)
	}
	before := spy.calls.Load()
	if status, _ := invoke(strings.TrimSuffix(string(body), "}") + `,"sql":"SELECT private"}`); status != 400 || spy.calls.Load() != before {
		t.Fatal("public raw SQL reached adapter")
	}
	otherPlan, _ := identity.NewResolvedSemanticPlanID()
	mismatch := request
	mismatch.PlanID = otherPlan
	mismatchJSON, _ := json.Marshal(mismatch)
	if status, _ := invoke(string(mismatchJSON)); status != 400 {
		t.Fatal("path/body mismatch accepted")
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "execution-proof", Version: "1"}, nil)
	session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint + "mcp", HTTPClient: client, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	called, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "semantic_execute", Arguments: map[string]any{"planId": request.PlanID.String(), "planDigest": request.PlanDigest, "idempotencyKey": "channel-mcp"}})
	if err != nil || called.IsError {
		t.Fatal("MCP execution failed", err)
	}
	raw, _ := json.Marshal(called.StructuredContent)
	var mcpResult d.Result
	if json.Unmarshal(raw, &mcpResult) != nil || mcpResult.Run.Channel != "mcp" || mcpResult.Run.ResultDigest != rest.Run.ResultDigest {
		t.Fatal("MCP result differs")
	}
	cliJSON, _ := json.Marshal(map[string]any{"planId": request.PlanID, "planDigest": request.PlanDigest, "idempotencyKey": "channel-cli"})
	command := exec.CommandContext(ctx, "go", "run", "-p=1", "./cmd/semlia", "semantic", "execute", string(cliJSON))
	command.Dir = repositoryRoot()
	command.Env = append(os.Environ(), "SEMLIA_API_URL="+server.URL, "SEMLIA_WORKSPACE_ID="+w.String(), "SEMLIA_API_TOKEN="+issued.Token)
	output, err := command.Output()
	if err != nil {
		t.Fatal("CLI execution failed", err)
	}
	var cliResult d.Result
	if json.Unmarshal(output, &cliResult) != nil || cliResult.Run.Channel != "cli" || cliResult.Run.ResultDigest != rest.Run.ResultDigest {
		t.Fatal("CLI result differs")
	}
	script := `import {createSemanticClient} from './sdk/typescript/src/client.ts';const c=createSemanticClient({baseUrl:process.env.SEMLIA_API_URL,workspaceId:process.env.SEMLIA_WORKSPACE_ID,bearerToken:process.env.SEMLIA_API_TOKEN});const r=await c.execute(process.env.PLAN_ID,process.env.PLAN_DIGEST,'channel-sdk');if(r.error){console.error('SDK_API_ERROR',typeof r.error.code==='string'?r.error.code:'UNKNOWN');process.exit(1)}console.log(JSON.stringify(r.data));`
	sdk := exec.CommandContext(ctx, "node", "--experimental-strip-types", "--input-type=module", "-e", script)
	sdk.Dir = repositoryRoot()
	sdk.Env = append(command.Env, "PLAN_ID="+request.PlanID.String(), "PLAN_DIGEST="+request.PlanDigest)
	output, err = sdk.Output()
	if err != nil {
		if failure, ok := err.(*exec.ExitError); ok {
			t.Log("SDK process diagnostic:", string(failure.Stderr))
		}
		t.Fatal("SDK execution failed", err)
	}
	var sdkResult d.Result
	if json.Unmarshal(output, &sdkResult) != nil || sdkResult.Run.Channel != "sdk" || sdkResult.Run.ResultDigest != rest.Run.ResultDigest {
		t.Fatal("SDK result differs")
	}
	assertDenied := func() {
		t.Helper()
		count := spy.calls.Load()
		if _, err := service.Execute(machineCtx, request); err == nil {
			t.Fatal("cached credential bypassed replay authorization")
		}
		if _, err := service.Get(machineCtx, w, rest.Run.ID, principal.ID.String(), traceID); err == nil {
			t.Fatal("cached credential bypassed known-run authorization")
		}
		if _, err := service.Cancel(machineCtx, w, rest.Run.ID, principal.ID.String(), traceID); err == nil {
			t.Fatal("cached credential bypassed cancellation authorization")
		}
		if spy.calls.Load() != count {
			t.Fatal("revoked context reached adapter")
		}
	}
	oldIssue := issue
	oldIssue.Name = "Resolve only"
	oldIssue.AllowedActions = []auth.Action{auth.ActionAssetRead, auth.ActionSemanticResolve}
	old, err := machine.Issue(ctx, oldIssue)
	if err != nil {
		t.Fatal(err)
	}
	oldLimit, err := machine.Authenticate(ctx, old.Token)
	if err != nil {
		t.Fatal(err)
	}
	oldCalls := spy.calls.Load()
	if _, err := service.Execute(authapp.WithCredentialLimit(ctx, oldLimit), request); err == nil || spy.calls.Load() != oldCalls {
		t.Fatal("non-opt-in credential acquired execution")
	}
	for _, target := range []struct{ table, id string }{{"consumers", consumer.ID.UUID()}, {"consumer_bindings", binding.ID.UUID()}} {
		versionClause := ""
		if target.table == "consumer_bindings" {
			versionClause = ",version=version+1"
		}
		if _, err := env.pool.Exec(ctx, "UPDATE "+target.table+" SET status='suspended'"+versionClause+" WHERE id=$1", target.id); err != nil {
			t.Fatal(err)
		}
		assertDenied()
		if _, err := env.pool.Exec(ctx, "UPDATE "+target.table+" SET status='active'"+versionClause+" WHERE id=$1", target.id); err != nil {
			t.Fatal(err)
		}
		if target.table == "consumer_bindings" {
			resolved, err = plans.Resolve(machineCtx, distapp.ResolveRequest{WorkspaceID: w, PrincipalRef: principal.ID.String(), TraceID: traceID, Channel: "api", IdempotencyKey: "machine-after-binding-restore", Input: input})
			if err != nil || resolved.Plan == nil {
				t.Fatal("restored binding plan", err)
			}
			request.PlanID, request.PlanDigest, request.IdempotencyKey = resolved.Plan.ID, resolved.Plan.PlanDigest, "machine-after-binding-restore"
			rest, err = service.Execute(machineCtx, request)
			if err != nil || rest.Run.State != "succeeded" {
				t.Fatal("restored binding independent run", err)
			}
		}
	}
	if _, err := env.pool.Exec(ctx, `UPDATE principals SET status='suspended' WHERE id=$1`, principal.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	assertDenied()
	if _, err := env.pool.Exec(ctx, `UPDATE principals SET status='active' WHERE id=$1`, principal.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `UPDATE client_credentials SET expires_at=CURRENT_TIMESTAMP-interval '1 second',issued_at=CURRENT_TIMESTAMP-interval '1 hour' WHERE id=$1`, issued.Credential.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	assertDenied()
	if _, err := env.pool.Exec(ctx, `UPDATE client_credentials SET expires_at=CURRENT_TIMESTAMP+interval '1 hour' WHERE id=$1`, issued.Credential.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `UPDATE role_bindings SET revoked_at=CURRENT_TIMESTAMP,revoked_by=$2 WHERE id=$1`, grant.ID.UUID(), admin.UUID()); err != nil {
		t.Fatal(err)
	}
	assertDenied()
	if _, _, err := authorizer.CreateBinding(ctx, authapp.CreateRoleBindingRequest{AccessRequest: authapp.AccessRequest{WorkspaceID: w, PrincipalRef: admin.String(), TraceID: traceID}, PrincipalID: principal.ID, RoleID: "consumer_developer", ExpectedRoleVersion: 1, ScopeType: auth.ScopeWorkspace, ScopeID: w.UUID()}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(machineCtx, w, rest.Run.ID, principal.ID.String(), traceID); err != nil {
		t.Fatal("restored grant did not restore independent credential check", err)
	}
	if err := machine.Revoke(ctx, w, issued.Credential.ID, admin.String(), traceID); err != nil {
		t.Fatal(err)
	}
	assertDenied()
	t.Log("real REST/MCP/CLI/SDK returned matching exact result digests; cached credential context denied suspended, expired, role-revoked and revoked known-ID/replay")
	issued, err = machine.Issue(ctx, issue)
	if err != nil {
		t.Fatal(err)
	}
	limit, err = machine.Authenticate(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	machineCtx = authapp.WithCredentialLimit(ctx, limit)
	resolved, err = plans.Resolve(machineCtx, distapp.ResolveRequest{WorkspaceID: w, PrincipalRef: principal.ID.String(), TraceID: traceID, Channel: "api", IdempotencyKey: "machine-current-cancel-plan", Input: input})
	if err != nil || resolved.Plan == nil {
		t.Fatal("machine cancel plan", err)
	}
	request.PlanID, request.PlanDigest, request.IdempotencyKey = resolved.Plan.ID, resolved.Plan.PlanDigest, "machine-current-cancel"
	lock, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err := lock.Exec(ctx, `LOCK TABLE public.execution_facts IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan d.Result, 1)
	failures := make(chan error, 1)
	go func() { result, err := service.Execute(machineCtx, request); outcomes <- result; failures <- err }()
	active := false
	for until := time.Now().Add(3 * time.Second); time.Now().Before(until); {
		if err := env.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name='semlia-execution' AND wait_event_type='Lock')`).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if active {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !active {
		t.Fatal("machine source SELECT did not become active")
	}
	var rawID string
	if err := env.pool.QueryRow(ctx, `SELECT id::text FROM query_execution_runs WHERE workspace_id=$1 AND idempotency_key='machine-current-cancel'`, w.UUID()).Scan(&rawID); err != nil {
		t.Fatal(err)
	}
	runID, _ := identity.FromUUID(identity.Run, rawID)
	publishNext()
	if _, err := service.Get(machineCtx, w, runID.String(), principal.ID.String(), traceID); err == nil {
		t.Fatal("fresh Get accepted a superseded current binding plan")
	}
	client = &http.Client{Transport: executionBearer{issued.Token}}
	cancelRun := func() (int, d.Result) {
		t.Helper()
		req, _ := http.NewRequest("POST", endpoint+"query-executions/"+runID.String()+":cancel", nil)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result d.Result
		_ = json.NewDecoder(response.Body).Decode(&result)
		return response.StatusCode, result
	}
	if _, err := env.pool.Exec(ctx, `UPDATE consumers SET status='suspended' WHERE id=$1`, consumer.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if status, _ := cancelRun(); status != 401 && status != 403 {
		t.Fatalf("suspended consumer cancellation status=%d", status)
	}
	if _, err := env.pool.Exec(ctx, `UPDATE consumers SET status='active' WHERE id=$1`, consumer.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	count := spy.calls.Load()
	status, cancellation := cancelRun()
	if status != 200 || !cancellation.Run.CancelRequested {
		t.Fatalf("current binding cancellation after publication rejected: status=%d state=%s", status, cancellation.Run.State)
	}
	select {
	case result := <-outcomes:
		if err := <-failures; err != nil || result.Run.State != "cancelled" || len(result.Rows) != 0 {
			t.Fatalf("machine source cancellation failed: %s %v", result.Run.State, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("machine cancellation did not stop source SELECT")
	}
	if spy.calls.Load() != count {
		t.Fatal("cancellation started another source query")
	}
	t.Log("current-binding machine run cancelled through REST after real newer publication; suspended consumer still denied")
}
