package db_test

import (
	"bytes"
	"context"
	"encoding/json"
	mcpadapter "github.com/iiwish/semlia/internal/adapters/mcp"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/internal/application"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	distapp "github.com/iiwish/semlia/internal/application/distribution"
	app "github.com/iiwish/semlia/internal/application/identity"
	rootdomain "github.com/iiwish/semlia/internal/domain"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	dist "github.com/iiwish/semlia/internal/domain/distribution"
	identitydomain "github.com/iiwish/semlia/internal/domain/identity"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/sdk/trace"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type bearerTransport struct{ token string }

func (t bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(clone)
}

func TestMachineCredentialLifecycleAndChannelParity(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	fixture := seedDistributionReleases(t, pool)
	ctx := context.Background()
	store := pgstore.NewStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	authorizer := authapp.NewService(store, authapp.ClockFunc(func() time.Time { return now }))
	machine := app.NewMachineService(store, authorizer, func() time.Time { return now })
	admin, err := store.LoadDefaultPrincipal(ctx, fixture.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := machine.CreatePrincipal(ctx, fixture.workspaceID, admin.ID.String(), distributionTraceID, "Machine integration")
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := store.LoadPrincipalBindings(ctx, p.ID)
	if err != nil || len(bindings) != 0 {
		t.Fatalf("machine inherited authority: %v %v", bindings, err)
	}
	access := authapp.AccessRequest{WorkspaceID: fixture.workspaceID, PrincipalRef: admin.ID.String(), TraceID: distributionTraceID}
	grant, _, err := authorizer.CreateBinding(ctx, authapp.CreateRoleBindingRequest{AccessRequest: access, PrincipalID: p.ID, RoleID: "consumer_developer", ExpectedRoleVersion: 1, ScopeType: auth.ScopeWorkspace, ScopeID: fixture.workspaceID.UUID()})
	if err != nil {
		t.Fatal(err)
	}
	resolver := distapp.NewService(store, authorizer, distapp.ClockFunc(func() time.Time { return now }))
	consumer, err := resolver.CreateConsumer(ctx, distapp.CreateConsumerRequest{WorkspaceID: fixture.workspaceID, StableKey: "machine", Name: "Machine", Kind: "agent", PrincipalRef: admin.ID.String(), TraceID: distributionTraceID})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := resolver.CreateBinding(ctx, distapp.CreateBindingRequest{WorkspaceID: fixture.workspaceID, ConsumerID: consumer.ID, Environment: "prod", Purpose: "machine parity", Mode: dist.BindingPinned, ReleaseID: &fixture.release1, PrincipalRef: admin.ID.String(), TraceID: distributionTraceID})
	if err != nil {
		t.Fatal(err)
	}
	issue := app.IssueMachineCredential{WorkspaceID: fixture.workspaceID, Actor: admin.ID.String(), TraceID: distributionTraceID, PrincipalID: p.ID, ConsumerID: consumer.ID, BindingID: binding.ID, Name: "Golden", AllowedActions: []auth.Action{auth.ActionSemanticResolve, auth.ActionAssetRead}, ScopeType: auth.ScopeWorkspace, ScopeID: fixture.workspaceID.String(), ExpiresAt: now.Add(time.Hour)}
	issued, err := machine.Issue(ctx, issue)
	if err != nil {
		t.Fatal(err)
	}
	var verifier []byte
	if err := pool.QueryRow(ctx, `SELECT verifier_digest FROM client_credentials WHERE id=$1`, issued.Credential.ID.UUID()).Scan(&verifier); err != nil || len(verifier) != 32 || bytes.Contains(verifier, []byte(issued.Token)) {
		t.Fatal("credential verifier privacy", err)
	}
	list, err := machine.List(ctx, fixture.workspaceID, admin.ID.String(), distributionTraceID)
	if err != nil {
		t.Fatal(err)
	}
	listed, _ := json.Marshal(list)
	if bytes.Contains(listed, []byte(issued.Token)) || bytes.Contains(listed, []byte("VerifierDigest")) {
		t.Fatal("list disclosed verifier or plaintext")
	}
	provider := trace.NewTracerProvider()
	defer provider.Shutdown(ctx)
	server := httptest.NewServer(httpapi.NewHandler(application.NewSystemService(nil, rootdomain.SystemInfo{}), slog.New(slog.NewTextHandler(io.Discard, nil)), provider.Tracer("machine-test"), httpapi.WithDistribution(resolver), httpapi.WithMachineIdentity(machine), httpapi.WithMCP(mcpadapter.NewHandler(resolver))))
	defer server.Close()
	endpoint := server.URL + "/api/v1/workspaces/" + fixture.workspaceID.String() + "/"
	query := distributionQuery(dist.ResolutionContext{Mode: dist.ResolutionCurrent})
	invoke := func(token, path string, body any, cookie bool) (int, map[string]any) {
		t.Helper()
		var payload []byte
		if body != nil {
			payload, _ = json.Marshal(body)
		}
		method := "GET"
		if body != nil {
			method = "POST"
		}
		r, _ := http.NewRequest(method, path, bytes.NewReader(payload))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Semlia-Principal", admin.ID.String())
		if cookie {
			r.AddCookie(&http.Cookie{Name: "semlia_session_dev", Value: "ambiguous"})
		}
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		var result map[string]any
		_ = json.Unmarshal(data, &result)
		return response.StatusCode, result
	}
	body := map[string]any{"query": query, "channel": "api", "idempotencyKey": "golden-rest"}
	status, rest := invoke(issued.Token, endpoint+"semantic-queries:resolve", body, false)
	if status != 200 {
		t.Fatalf("REST %d %+v", status, rest)
	}
	plan := rest["plan"].(map[string]any)
	digest := plan["planDigest"]
	if rest["consumerId"] != consumer.ID.String() || rest["bindingId"] != binding.ID.String() {
		t.Fatal("consumer context not server-derived")
	}
	if status, _ := invoke(issued.Token, endpoint+"semantic-queries:resolve", body, true); status != 401 {
		t.Fatal("mixed auth accepted")
	}
	other, _ := identity.NewWorkspaceID()
	if status, _ := invoke(issued.Token, strings.Replace(endpoint, fixture.workspaceID.String(), other.String(), 1)+"semantic-queries:resolve", body, false); status != 403 {
		t.Fatal("forged workspace accepted")
	}
	forged := query
	forged.Context = dist.ResolutionContext{Mode: dist.ResolutionExplicit, ReleaseID: &fixture.release2}
	if status, _ := invoke(issued.Token, endpoint+"semantic-queries:resolve", map[string]any{"query": forged, "channel": "api", "idempotencyKey": "forged"}, false); status != 403 {
		t.Fatal("forged release context accepted")
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "integration", Version: "1"}, nil)
	session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint + "mcp", HTTPClient: &http.Client{Transport: bearerTransport{issued.Token}}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal("MCP initialize without trace header", err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "semantic_resolve", Arguments: map[string]any{"query": query, "idempotencyKey": "golden-mcp"}})
	if err != nil || result.IsError {
		t.Fatalf("MCP resolve: %+v %v", result, err)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	var mcpResult map[string]any
	_ = json.Unmarshal(encoded, &mcpResult)
	if mcpResult["plan"].(map[string]any)["planDigest"] != digest {
		t.Fatal("MCP plan differs from REST")
	}
	if mcpResult["channel"] != "mcp" {
		t.Fatal("MCP channel missing")
	}
	root, _ := filepath.Abs("../../..")
	queryJSON, _ := json.Marshal(query)
	command := exec.CommandContext(ctx, "go", "run", "-p=1", "./cmd/semlia", "semantic", "resolve", string(queryJSON))
	command.Dir = root
	command.Env = append(os.Environ(), "SEMLIA_API_URL="+server.URL, "SEMLIA_WORKSPACE_ID="+fixture.workspaceID.String(), "SEMLIA_API_TOKEN="+issued.Token, "SEMLIA_IDEMPOTENCY_KEY=golden-cli")
	output, err := command.Output()
	if err != nil {
		t.Fatal("CLI entrypoint", err)
	}
	var cli map[string]any
	if json.Unmarshal(output, &cli) != nil || cli["plan"].(map[string]any)["planDigest"] != digest || cli["channel"] != "cli" {
		t.Fatalf("CLI parity %s", output)
	}
	script := `import { createSemanticClient } from './sdk/typescript/src/client.ts'; const client=createSemanticClient({baseUrl:process.env.SEMLIA_API_URL,workspaceId:process.env.SEMLIA_WORKSPACE_ID,bearerToken:process.env.SEMLIA_API_TOKEN});const r=await client.resolve(JSON.parse(process.env.SEMLIA_QUERY),'golden-sdk');if(r.error)throw new Error(JSON.stringify(r.error));console.log(JSON.stringify(r.data));`
	sdk := exec.CommandContext(ctx, "node", "--experimental-strip-types", "--input-type=module", "-e", script)
	sdk.Dir = root
	sdk.Env = append(command.Env, "SEMLIA_QUERY="+string(queryJSON))
	output, err = sdk.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			t.Fatalf("SDK entrypoint %s", exit.Stderr)
		}
		t.Fatal(err)
	}
	var sdkResult map[string]any
	if json.Unmarshal(output, &sdkResult) != nil || sdkResult["plan"].(map[string]any)["planDigest"] != digest || sdkResult["channel"] != "sdk" {
		t.Fatalf("SDK parity %s", output)
	}
	planPath := endpoint + "resolved-semantic-plans/" + plan["id"].(string)
	// Exercise the published methods through all four real entry points, not
	// application-service aliases. Only request-local IDs/channel fields differ.
	for _, test := range []struct {
		name, operation string
		argument        any
		field           string
	}{
		{"describe", "describe", query, "definitions"}, {"search", "search", "revenue", "items"},
		{"plan", "plan", plan["id"], "planDigest"}, {"query", "query", rest["id"], "definitions"},
		{"refusal", "resolve", func() dist.SemanticQueryInput {
			q := query
			q.Measures = []dist.Selector{{Address: "missing.asset"}}
			return q
		}(), "refusal"},
	} {
		t.Run("parity-"+test.name, func(t *testing.T) {
			argumentJSON, _ := json.Marshal(test.argument)
			argumentText := string(argumentJSON)
			if value, ok := test.argument.(string); ok {
				argumentText = value
			}
			path := endpoint
			var request any
			arguments := map[string]any{}
			switch test.operation {
			case "resolve", "describe":
				suffix := "semantic-queries:resolve"
				if test.operation == "describe" {
					suffix = "semantic-describe"
				}
				path += suffix
				request = map[string]any{"query": test.argument, "channel": "api", "idempotencyKey": "matrix-rest-" + test.name}
				arguments = map[string]any{"query": test.argument, "idempotencyKey": "matrix-mcp-" + test.name}
			case "search":
				path += "semantic-search?q=revenue"
				arguments["text"] = test.argument
			case "plan":
				path += "resolved-semantic-plans/" + argumentText
				arguments["id"] = test.argument
			case "query":
				path += "semantic-queries/" + argumentText
				arguments["id"] = test.argument
			}
			status, expected := invoke(issued.Token, path, request, false)
			if status != 200 {
				t.Fatalf("REST %d %+v", status, expected)
			}
			called, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "semantic_" + test.operation, Arguments: arguments})
			if err != nil || called.IsError {
				t.Fatalf("MCP %+v %v", called, err)
			}
			encoded, _ := json.Marshal(called.StructuredContent)
			var fromMCP map[string]any
			_ = json.Unmarshal(encoded, &fromMCP)
			cliCommand := exec.CommandContext(ctx, "go", "run", "-p=1", "./cmd/semlia", "semantic", test.operation, argumentText)
			cliCommand.Dir = root
			cliCommand.Env = append(command.Env, "SEMLIA_IDEMPOTENCY_KEY=matrix-cli-"+test.name)
			output, err := cliCommand.Output()
			if err != nil {
				t.Fatal("CLI", err)
			}
			var fromCLI map[string]any
			_ = json.Unmarshal(output, &fromCLI)
			sdkScript := `import { createSemanticClient } from './sdk/typescript/src/client.ts';const c=createSemanticClient({baseUrl:process.env.SEMLIA_API_URL,workspaceId:process.env.SEMLIA_WORKSPACE_ID,bearerToken:process.env.SEMLIA_API_TOKEN});const r=await c[process.env.SEMLIA_OPERATION](JSON.parse(process.env.SEMLIA_ARGUMENT),'matrix-sdk-'+process.env.SEMLIA_CASE);if(r.error)throw Error(JSON.stringify(r.error));console.log(JSON.stringify(r.data));`
			sdkCommand := exec.CommandContext(ctx, "node", "--experimental-strip-types", "--input-type=module", "-e", sdkScript)
			sdkCommand.Dir = root
			sdkCommand.Env = append(command.Env, "SEMLIA_OPERATION="+test.operation, "SEMLIA_ARGUMENT="+string(argumentJSON), "SEMLIA_CASE="+test.name)
			output, err = sdkCommand.Output()
			if err != nil {
				t.Fatal("SDK", err)
			}
			var fromSDK map[string]any
			_ = json.Unmarshal(output, &fromSDK)
			for name, actual := range map[string]map[string]any{"MCP": fromMCP, "CLI": fromCLI, "SDK": fromSDK} {
				if !reflect.DeepEqual(actual[test.field], expected[test.field]) {
					t.Fatalf("%s %s mismatch: %+v vs %+v", name, test.field, actual[test.field], expected[test.field])
				}
			}
			if test.name == "describe" {
				definitions := fromSDK["definitions"].([]any)
				if len(definitions) == 0 || definitions[0].(map[string]any)["assetId"] == nil || definitions[0].(map[string]any)["AssetID"] != nil {
					t.Fatal("SDK lowerCamel released definition contract missing")
				}
			}
		})
	}
	stdioCommand := exec.CommandContext(ctx, "go", "run", "-p=1", "./cmd/semlia", "mcp")
	stdioCommand.Dir = root
	stdioCommand.Env = command.Env
	stdio, err := mcpClient.Connect(ctx, &mcp.CommandTransport{Command: stdioCommand}, nil)
	if err != nil {
		t.Fatal("semlia mcp stdio initialize", err)
	}
	stdioResult, err := stdio.CallTool(ctx, &mcp.CallToolParams{Name: "semantic_resolve", Arguments: map[string]any{"query": query, "idempotencyKey": "golden-stdio"}})
	if err != nil || stdioResult.IsError {
		t.Fatalf("stdio tool %+v %v", stdioResult, err)
	}
	encoded, _ = json.Marshal(stdioResult.StructuredContent)
	var stdioData map[string]any
	_ = json.Unmarshal(encoded, &stdioData)
	if stdioData["plan"].(map[string]any)["planDigest"] != digest {
		t.Fatal("stdio plan differs")
	}
	resource, err := stdio.ReadResource(ctx, &mcp.ReadResourceParams{URI: "semlia://contract"})
	if err != nil || len(resource.Contents) != 1 {
		t.Fatal("stdio canonical resource", err)
	}
	defer stdio.Close()
	badAsset, _ := identity.NewAssetID()
	narrowRequest := issue
	narrowRequest.ScopeType = auth.ScopeAsset
	narrowRequest.ScopeID = badAsset.String()
	narrow, err := machine.Issue(ctx, narrowRequest)
	if err != nil {
		t.Fatal(err)
	}
	denialCount := func() int {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM authorization_events WHERE workspace_id=$1 AND actor=$2 AND action='asset.read' AND decision='deny'`, fixture.workspaceID.UUID(), p.ID.String()).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	beforeDenied := denialCount()
	if status, _ := invoke(narrow.Token, planPath, nil, false); status != 403 {
		t.Fatal("known plan bypassed credential scope")
	}
	if denialCount() != beforeDenied+1 {
		t.Fatal("credential-scoped denial bypassed durable authorization audit")
	}
	if status, _ := invoke(narrow.Token, endpoint+"semantic-queries:resolve", body, false); status != 403 {
		t.Fatal("replay bypassed credential scope")
	}
	for _, test := range []struct {
		name    string
		actions []auth.Action
		scope   auth.ScopeType
		scopeID string
		want    int
	}{
		{"read-only", []auth.Action{auth.ActionAssetRead}, auth.ScopeWorkspace, fixture.workspaceID.String(), 403},
		{"resolve-only", []auth.Action{auth.ActionSemanticResolve}, auth.ScopeWorkspace, fixture.workspaceID.String(), 403},
		{"wrong-release", issue.AllowedActions, auth.ScopeRelease, fixture.release2.String(), 403},
		{"bound-release", issue.AllowedActions, auth.ScopeRelease, fixture.release1.String(), 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := issue
			r.AllowedActions = test.actions
			r.ScopeType = test.scope
			r.ScopeID = test.scopeID
			token, err := machine.Issue(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			if status, _ := invoke(token.Token, planPath, nil, false); status != test.want {
				t.Fatalf("scope/action inspection status=%d want=%d", status, test.want)
			}
		})
	}
	otherPrincipal, err := machine.CreatePrincipal(ctx, fixture.workspaceID, admin.ID.String(), distributionTraceID, "Other accountable identity")
	if err != nil {
		t.Fatal(err)
	}
	rebound := issue
	rebound.PrincipalID = otherPrincipal.ID
	if _, err := machine.Issue(ctx, rebound); err == nil {
		t.Fatal("consumer machine identity reassigned")
	}
	for _, table := range []string{"consumers", "consumer_bindings"} {
		id := consumer.ID.UUID()
		versionUpdate := ""
		if table == "consumer_bindings" {
			id = binding.ID.UUID()
			versionUpdate = ", version=version+1"
		}
		if _, err := pool.Exec(ctx, `UPDATE `+table+` SET status='suspended'`+versionUpdate+` WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := machine.Authenticate(ctx, issued.Token); err == nil {
			t.Fatal("suspended " + table + " accepted")
		}
		if _, err := pool.Exec(ctx, `UPDATE `+table+` SET status='active'`+versionUpdate+` WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, revokeRace := range []bool{false, true} {
		name := "rotate-rotate"
		if revokeRace {
			name = "rotate-revoke"
		}
		t.Run(name, func(t *testing.T) {
			request := issue
			request.Name = name
			initial, err := machine.Issue(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			type outcome struct {
				issued identitydomain.IssuedCredential
				err    error
			}
			results := make(chan outcome, 2)
			start := make(chan struct{})
			go func() {
				<-start
				value, err := machine.Rotate(ctx, fixture.workspaceID, initial.Credential.ID, admin.ID.String(), distributionTraceID)
				results <- outcome{value, err}
			}()
			go func() {
				<-start
				if revokeRace {
					results <- outcome{err: machine.Revoke(ctx, fixture.workspaceID, initial.Credential.ID, admin.ID.String(), distributionTraceID)}
				} else {
					value, err := machine.Rotate(ctx, fixture.workspaceID, initial.Credential.ID, admin.ID.String(), distributionTraceID)
					results <- outcome{value, err}
				}
			}()
			close(start)
			successes := 0
			for i := 0; i < 2; i++ {
				result := <-results
				if result.err == nil {
					successes++
					if result.issued.Token != "" {
						if _, err := machine.Authenticate(ctx, result.issued.Token); err != nil {
							t.Fatal("winning rotation unusable", err)
						}
						if !result.issued.Credential.ExpiresAt.Equal(initial.Credential.ExpiresAt) {
							t.Fatal("rotation extended expiry")
						}
					}
				}
			}
			if (!revokeRace && successes != 1) || (revokeRace && successes < 1) {
				t.Fatalf("concurrent lifecycle winners=%d", successes)
			}
			if _, err := machine.Authenticate(ctx, initial.Token); err == nil {
				t.Fatal("old credential survived concurrent lifecycle")
			}
			var descendants int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM client_credentials WHERE rotated_from_id=$1`, initial.Credential.ID.UUID()).Scan(&descendants); err != nil || descendants > 1 {
				t.Fatal("rotation fork", descendants, err)
			}
		})
	}
	rotated, err := machine.Rotate(ctx, fixture.workspaceID, issued.Credential.ID, admin.ID.String(), distributionTraceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := machine.Authenticate(ctx, issued.Token); err == nil {
		t.Fatal("old token accepted after rotation")
	}
	if _, err := machine.Authenticate(ctx, rotated.Token); err != nil {
		t.Fatal(err)
	}
	if result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "semantic_plan", Arguments: map[string]any{"id": plan["id"]}}); err == nil && !result.IsError {
		t.Fatal("MCP session retained revoked token authority")
	}
	if result, err := stdio.CallTool(ctx, &mcp.CallToolParams{Name: "semantic_plan", Arguments: map[string]any{"id": plan["id"]}}); err == nil && !result.IsError {
		t.Fatal("stdio session retained revoked token authority")
	}
	if rotated.Credential.RotatedFromID == nil || *rotated.Credential.RotatedFromID != issued.Credential.ID {
		t.Fatal("rotation chain missing")
	}
	if _, _, err := authorizer.RevokeBinding(ctx, authapp.RevokeRoleBindingRequest{AccessRequest: access, BindingID: grant.ID, ExpectedVersion: grant.Version, Reason: "next-request regression"}); err != nil {
		t.Fatal(err)
	}
	if status, _ := invoke(rotated.Token, planPath, nil, false); status != 403 {
		t.Fatal("revoked grant accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE principals SET status='suspended' WHERE id=$1`, p.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := machine.Authenticate(ctx, rotated.Token); err == nil {
		t.Fatal("suspended principal accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE principals SET status='active' WHERE id=$1`, p.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if err := machine.Revoke(ctx, fixture.workspaceID, rotated.Credential.ID, admin.ID.String(), distributionTraceID); err != nil {
		t.Fatal(err)
	}
	if _, err := machine.Authenticate(ctx, rotated.Token); err == nil {
		t.Fatal("revoked credential accepted")
	}
	now = now.Add(2 * time.Hour)
	if _, err := machine.Authenticate(ctx, narrow.Token); err == nil {
		t.Fatal("expired credential accepted")
	}
	t.Log("real PostgreSQL verifier/rotation/grant/suspension/expiry + actual REST/MCP/CLI/SDK golden digest:", digest)
}
