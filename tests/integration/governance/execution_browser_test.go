package governance_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	distapp "github.com/iiwish/semlia/internal/application/distribution"
	execapp "github.com/iiwish/semlia/internal/application/execution"
	app "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/governance/llm"
	rootdomain "github.com/iiwish/semlia/internal/domain"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/sdk/trace"
)

type deterministicExecutionChat struct{ content string }

func (deterministicExecutionChat) Protocol() domain.ModelProviderProtocol {
	return domain.ProtocolOpenAI
}
func (c deterministicExecutionChat) Complete(context.Context, llm.CompleteRequest) (llm.CompleteResponse, error) {
	return llm.CompleteResponse{Content: c.content, FinishReason: "stop"}, nil
}

func proveLiveAskExecution(t *testing.T, env *fixture, w identity.WorkspaceID, actor identity.PrincipalID, asset identity.AssetID, plans *distapp.Service, execution *execapp.Service) {
	t.Helper()
	ctx := context.Background()
	clock := app.ClockFunc(time.Now)
	auth := authapp.NewService(env.store, authapp.ClockFunc(time.Now))
	provider, _ := createProvider(t, env, w, providerBody("openai", "Deterministic local Chat", "", credentialEnvName))
	createSetting(t, env, w, provider, settingBody(provider, "llm", "deterministic-local-proof", "Local proof only", 2048, nil))
	content, _ := json.Marshal(map[string]any{"schema": app.AskInterpretationSchemaID, "outcome": "query", "query": map[string]any{"intent": "aggregate", "measures": []map[string]any{{"assetId": asset.String()}}, "dimensions": []any{}, "filters": []any{}, "order": []any{}}})
	ask := app.NewAskService(env.store, app.NewModelConfigService(env.store, auth, clock), app.NewAgentRunService(env.store, clock), plans, auth, app.WithAskProviderClientFactory(func(domain.ModelProvider, llm.CredentialResolver, *http.Client) (llm.ProviderClient, error) {
		return deterministicExecutionChat{string(content)}, nil
	}))
	traces := trace.NewTracerProvider()
	defer traces.Shutdown(ctx)
	handler := httpapi.NewHandler(application.NewSystemService(nil, rootdomain.SystemInfo{}), slog.New(slog.NewTextHandler(io.Discard, nil)), traces.Tracer("local-ask-proof"), httpapi.WithAsk(ask), httpapi.WithDistribution(plans), httpapi.WithExecution(execution))
	// This isolated fixture has a disclosed local actor, not a production login.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Semlia-Principal", actor.String())
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	command := exec.CommandContext(ctx, "node", "web/e2e-live/execution-browser.mjs")
	command.Dir = repositoryRoot()
	command.Env = append(os.Environ(), "SEMLIA_EXECUTION_BROWSER_API="+server.URL, "SEMLIA_EXECUTION_TEST_WORKSPACE="+w.String())
	output, err := command.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal("live Ask browser proof failed", err)
	}
}
