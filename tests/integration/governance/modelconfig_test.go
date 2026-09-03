package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	authorization "github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

const credentialSecret = "sk-live-integration-secret-9f2a"
const credentialEnvName = "SEMLIA_TEST_OPENAI_KEY"

// credentialNonEcho walks every column of the table rows and refuses any
// occurrence of the secret material (SSOT §12 S-002 lock).
func assertCredentialNeverPersisted(
	t *testing.T, pool *pgstore.Pool, table, workspaceUUID, secret string,
) {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		"SELECT * FROM "+table+" WHERE workspace_id = $1", workspaceUUID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	fieldDescriptions := rows.FieldDescriptions()
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			t.Fatal(err)
		}
		for index, value := range values {
			text := fmt.Sprintf("%v", value)
			if strings.Contains(text, secret) {
				t.Fatalf("%s column %s persisted credential material", table, fieldDescriptions[index].Name)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func assertCredentialNeverEchoed(t *testing.T, body []byte, secret string) {
	t.Helper()
	if strings.Contains(string(body), secret) {
		t.Fatalf("response echoed credential material: %s", body)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, body)
	}
	for _, field := range []string{"credential", "apiKey", "api_key", "secret"} {
		if _, ok := payload[field]; ok {
			t.Fatalf("response carries credential field %q: %s", field, body)
		}
	}
}

func assertQueryCount(t *testing.T, pool *pgstore.Pool, query string, args []any, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("query %q count = %d, want %d", query, got, want)
	}
}

// openAICompatibleStub captures the provider call and answers with one chat
// completion whose content is built by the callback.
func openAICompatibleStub(
	t *testing.T,
	answer func(model, prompt string, receivedBearer *string) string,
) (*httptest.Server, *string) {
	t.Helper()
	bearer := new(string)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		*bearer = request.Header.Get("Authorization")
		raw, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
		if err != nil {
			http.Error(response, "read failed", http.StatusBadRequest)
			return
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil || len(payload.Messages) == 0 {
			http.Error(response, "bad request", http.StatusBadRequest)
			return
		}
		completion := map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": answer(payload.Model, payload.Messages[0].Content, bearer)},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 120, "completion_tokens": 80},
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(completion)
	}))
	t.Cleanup(server.Close)
	return server, bearer
}

// attributionEcho extracts the exact agentAttribution JSON the prompt orders
// the model to echo — how a compliant model reproduces the server-owned run
// identity without knowing it in advance.
func attributionEcho(prompt string) string {
	marker := `{"agentRunId":`
	start := strings.Index(prompt, marker)
	if start < 0 {
		return ""
	}
	end := strings.Index(prompt[start:], "\n")
	if end < 0 {
		return prompt[start:]
	}
	return strings.TrimSpace(prompt[start : start+end])
}

func validProposalContent(attribution, targetObjectID, baseRevisionID string) string {
	return fmt.Sprintf(`{
		"targetObjectType":"semantic_asset",
		"targetObjectId":%q,
		"baseRevisionId":%q,
		"title":"Tighten net revenue definition",
		"summary":"Adds chargebacks to the refund adjustment.",
		"reason":"Finance audit 2026-09.",
		"changeSet":[{"fieldPath":"definition","op":"update","beforeValue":"Revenue after refunds","afterValue":"Revenue after refunds and chargebacks"}],
		"agentAttribution":%s
	}`, targetObjectID, baseRevisionID, attribution)
}

func createProvider(t *testing.T, environment *fixture, workspace identity.WorkspaceID, body string) (identity.ModelProviderID, []byte) {
	t.Helper()
	path := "/api/v1/workspaces/" + workspace.String() + "/governance/model-providers"
	response := environment.request(t, http.MethodPost, path, "", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("create provider status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Provider struct {
			ID string `json:"id"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode provider detail: %v\n%s", err, response.Body.String())
	}
	providerID, err := identity.ParseModelProviderID(payload.Provider.ID)
	if err != nil {
		t.Fatalf("provider id %q: %v", payload.Provider.ID, err)
	}
	return providerID, response.Body.Bytes()
}

func createSetting(t *testing.T, environment *fixture, workspace identity.WorkspaceID, provider identity.ModelProviderID, body string) identity.ModelSettingID {
	t.Helper()
	path := "/api/v1/workspaces/" + workspace.String() + "/governance/model-settings"
	response := environment.request(t, http.MethodPost, path, "", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("create setting status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode setting: %v\n%s", err, response.Body.String())
	}
	settingID, err := identity.ParseModelSettingID(payload.ID)
	if err != nil {
		t.Fatalf("setting id %q: %v", payload.ID, err)
	}
	return settingID
}

func providerBody(protocol, displayName, baseUrl, credentialEnv string) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, `{"protocol":%q,"displayName":%q,`, protocol, displayName)
	if baseUrl != "" {
		fmt.Fprintf(builder, `"baseUrl":%q,`, baseUrl)
	}
	fmt.Fprintf(builder, `"credentialEnv":%q,"credential":%q}`, credentialEnv, credentialSecret)
	return builder.String()
}

func settingBody(provider identity.ModelProviderID, kind, model, capability string, tokenLimit int, dimension *int) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, `{"providerId":%q,"kind":%q,"model":%q,"capability":%q,"tokenLimit":%d`,
		provider.String(), kind, model, capability, tokenLimit)
	if dimension != nil {
		fmt.Fprintf(builder, `,"embeddingDimension":%d`, *dimension)
	}
	builder.WriteString("}")
	return builder.String()
}

func TestModelProviderConfigPersistsAndNeverExposesCredentials(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "model-config")
	workspaceUUID := workspace.UUID()

	providerID, body := createProvider(t, environment, workspace, providerBody(
		"openai", "OpenAI Enterprise", "", credentialEnvName))
	assertCredentialNeverEchoed(t, body, credentialSecret)

	var revision string
	if err := environment.pool.QueryRow(context.Background(),
		"SELECT credential_revision, credential_env FROM model_providers WHERE id = $1",
		providerID.UUID()).Scan(&revision, new(string)); err != nil {
		t.Fatal(err)
	}
	if revision != governanceapp.CredentialRevisionDigest(credentialSecret) {
		t.Fatalf("credential revision = %s, want the salted digest of the secret", revision)
	}
	assertCredentialNeverPersisted(t, environment.pool, "model_providers", workspaceUUID, credentialSecret)

	getResponse := environment.request(t, http.MethodGet,
		"/api/v1/workspaces/"+workspace.String()+"/governance/model-providers/"+providerID.String(), "", "")
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get provider status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	assertCredentialNeverEchoed(t, getResponse.Body.Bytes(), credentialSecret)

	rotated := "sk-rotated-secret-77aa"
	rotate := fmt.Sprintf(`{"displayName":"OpenAI Enterprise","enabled":true,"credentialEnv":%q,"credential":%q}`,
		credentialEnvName, rotated)
	updateResponse := environment.request(t, http.MethodPut,
		"/api/v1/workspaces/"+workspace.String()+"/governance/model-providers/"+providerID.String(), "", rotate)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update provider status = %d, body = %s", updateResponse.Code, updateResponse.Body.String())
	}
	assertCredentialNeverEchoed(t, updateResponse.Body.Bytes(), rotated)
	assertCredentialNeverPersisted(t, environment.pool, "model_providers", workspaceUUID, rotated)

	// A pasted secret where the env-var NAME belongs is rejected before any
	// persistence: the credential surface only accepts NAMEs.
	reject := fmt.Sprintf(`{"protocol":"openai","displayName":"Broken","credentialEnv":%q,"credential":"x"}`,
		credentialSecret)
	rejectResponse := environment.request(t, http.MethodPost,
		"/api/v1/workspaces/"+workspace.String()+"/governance/model-providers", "", reject)
	if rejectResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid credentialEnv status = %d, want 400, body = %s", rejectResponse.Code, rejectResponse.Body.String())
	}
	assertCredentialNeverPersisted(t, environment.pool, "model_providers", workspaceUUID, credentialSecret+"Broken")
}

func TestModelSettingsPersistWithScopedDefaults(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "model-settings")
	other := createWorkspace(t, environment.pool, "model-settings-other")

	providerID, _ := createProvider(t, environment, workspace, providerBody(
		"openai_compatible", "Internal Router", "https://models.example.com/v1", credentialEnvName))
	firstSetting := createSetting(t, environment, workspace, providerID,
		settingBody(providerID, "llm", "gpt-4.1", "视觉 · 推理", 128000, nil))
	secondProvider, _ := createProvider(t, environment, workspace, providerBody(
		"openai", "OpenAI Fallback", "", credentialEnvName))
	secondSetting := createSetting(t, environment, workspace, secondProvider,
		settingBody(secondProvider, "llm", "o3", "推理", 200000, nil))
	dimension := 3072
	embeddingSetting := createSetting(t, environment, workspace, providerID,
		settingBody(providerID, "embedding", "text-embedding-3-large", "多语言检索", 8191, &dimension))

	// The first enabled llm became the default; the second did not.
	listResponse := environment.request(t, http.MethodGet,
		"/api/v1/workspaces/"+workspace.String()+"/governance/model-settings", "", "")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list settings status = %d", listResponse.Code)
	}
	var page struct {
		Items []struct {
			ID        string `json:"id"`
			Kind      string `json:"kind"`
			IsDefault bool   `json:"isDefault"`
			Enabled   bool   `json:"enabled"`
			Model     string `json:"model"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("settings = %d, want 3", len(page.Items))
	}
	defaults := map[string]int{}
	for _, item := range page.Items {
		if item.IsDefault {
			defaults[item.Kind]++
			if item.Kind == "llm" && (item.ID != firstSetting.String() || item.Model != "gpt-4.1") {
				t.Fatalf("unexpected llm default %s (%s)", item.ID, item.Model)
			}
			if item.Kind == "embedding" && item.ID != embeddingSetting.String() {
				t.Fatalf("unexpected embedding default %s", item.ID)
			}
		}
	}
	if defaults["llm"] != 1 || defaults["embedding"] != 1 {
		t.Fatalf("defaults per kind = %v, want exactly one llm and one embedding", defaults)
	}

	// set-default moves the flag; disabling the default is refused.
	moveResponse := environment.request(t, http.MethodPost,
		"/api/v1/workspaces/"+workspace.String()+"/governance/model-settings/"+secondSetting.String()+"/set-default", "", "")
	if moveResponse.Code != http.StatusOK {
		t.Fatalf("set default status = %d, body = %s", moveResponse.Code, moveResponse.Body.String())
	}
	assertQueryCount(t, environment.pool,
		"SELECT count(*) FROM model_settings WHERE workspace_id = $1 AND kind = 'llm' AND is_default",
		[]any{workspace.UUID()}, 1)
	var defaultID string
	if err := environment.pool.QueryRow(context.Background(),
		"SELECT id::text FROM model_settings WHERE workspace_id = $1 AND kind = 'llm' AND is_default",
		workspace.UUID()).Scan(&defaultID); err != nil {
		t.Fatal(err)
	}
	if defaultID != secondSetting.UUID() {
		t.Fatalf("default moved to %s, want %s", defaultID, secondSetting.UUID())
	}

	// Workspace scoping: the other workspace sees nothing.
	otherResponse := environment.request(t, http.MethodGet,
		"/api/v1/workspaces/"+other.String()+"/governance/model-providers", "", "")
	var otherPage struct {
		Items []any `json:"items"`
	}
	if err := json.Unmarshal(otherResponse.Body.Bytes(), &otherPage); err != nil {
		t.Fatal(err)
	}
	if len(otherPage.Items) != 0 {
		t.Fatalf("other workspace providers = %d, want 0", len(otherPage.Items))
	}
}

func TestModelConfigRequiresCapabilities(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "model-config-authz")
	unbound := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: unbound, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Unbound Human", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}

	createResponse := environment.request(t, http.MethodPost,
		"/api/v1/workspaces/"+workspace.String()+"/governance/model-providers", unbound.String(),
		providerBody("openai", "Forbidden", "", credentialEnvName))
	assertJSONField(t, createResponse.Body.Bytes(), "code", "NO_MATCHING_GRANT")

	readResponse := environment.request(t, http.MethodGet,
		"/api/v1/workspaces/"+workspace.String()+"/governance/model-providers", unbound.String(), "")
	if readResponse.Code != http.StatusForbidden {
		t.Fatalf("unauthorized read status = %d, want 403", readResponse.Code)
	}
}

func TestGenerateProposalRecordsFailedRunWithoutProposal(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "generation-failure")
	assetID, _ := environment.createAsset(t, workspace)

	unreachable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachable.Close()
	providerID, _ := createProvider(t, environment, workspace, providerBody(
		"openai_compatible", "Unreachable Router", unreachable.URL, credentialEnvName))
	createSetting(t, environment, workspace, providerID,
		settingBody(providerID, "llm", "gpt-4.1", "推理", 4096, nil))

	generate := fmt.Sprintf(
		`{"targetObjectType":"semantic_asset","targetObjectId":%q,"instruction":"Tighten the definition"}`,
		assetID.String())
	response := environment.request(t, http.MethodPost,
		"/api/v1/workspaces/"+workspace.String()+"/governance/agent-runs/generate-proposal", "", generate)
	assertJSONField(t, response.Body.Bytes(), "code", "PROVIDER_UNAVAILABLE")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("generation status = %d, want 503", response.Code)
	}

	var run struct {
		Status    string  `json:"status"`
		Duration  *int64  `json:"durationMs"`
		Output    *string `json:"outputDigest"`
		InputHash string  `json:"inputHash"`
		Cost      int64   `json:"costMicros"`
	}
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT status, duration_ms, output_digest, input_hash, cost_micros FROM agent_runs
	`).Scan(&run.Status, &run.Duration, &run.Output, &run.InputHash, &run.Cost); err != nil {
		t.Fatal(err)
	}
	if run.Status != "failed" || run.Duration == nil || run.Output != nil ||
		!strings.HasPrefix(run.InputHash, "sha256:") || run.Cost != 0 {
		t.Fatalf("failed run record = %+v", run)
	}
	var errorCode string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT error_code FROM agent_steps WHERE kind = 'model'
	`).Scan(&errorCode); err != nil {
		t.Fatal(err)
	}
	if errorCode != "PROVIDER_UNAVAILABLE" {
		t.Fatalf("step error code = %s", errorCode)
	}
	assertQueryCount(t, environment.pool, "SELECT count(*) FROM proposals", nil, 0)
}

func TestGenerateProposalWithLiveProviderCreatesAttributedProposal(t *testing.T) {
	t.Setenv(credentialEnvName, credentialSecret)
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "generation-live")
	assetID, revisionID := environment.createAsset(t, workspace)

	var seenModel *string
	server, bearer := openAICompatibleStub(t, func(model, prompt string, received *string) string {
		modelCopy := model
		seenModel = &modelCopy
		if !strings.Contains(prompt, "semlia.proposal-input/v1") || !strings.Contains(prompt, "Tighten the definition") {
			t.Errorf("prompt misses schema id or instruction: %.200s", prompt)
		}
		return validProposalContent(attributionEcho(prompt), assetID.String(), revisionID.String())
	})
	providerID, _ := createProvider(t, environment, workspace, providerBody(
		"openai_compatible", "Enterprise Router", server.URL, credentialEnvName))
	settingID := createSetting(t, environment, workspace, providerID,
		settingBody(providerID, "llm", "gpt-4.1", "推理", 4096, nil))

	generate := fmt.Sprintf(
		`{"targetObjectType":"semantic_asset","targetObjectId":%q,"instruction":"Tighten the definition","modelSettingId":%q}`,
		assetID.String(), settingID.String())
	response := environment.request(t, http.MethodPost,
		"/api/v1/workspaces/"+workspace.String()+"/governance/agent-runs/generate-proposal", "", generate)
	if response.Code != http.StatusCreated {
		t.Fatalf("generation status = %d, body = %s", response.Code, response.Body.String())
	}
	assertCredentialNeverEchoed(t, response.Body.Bytes(), credentialSecret)
	if *bearer != "Bearer "+credentialSecret {
		t.Fatalf("provider bearer = %q", *bearer)
	}
	if seenModel == nil || *seenModel != "gpt-4.1" {
		t.Fatalf("provider received model %v", seenModel)
	}

	var result struct {
		AgentRun struct {
			ID             string `json:"id"`
			Model          string `json:"model"`
			ConfigRevision string `json:"configRevision"`
			InputHash      string `json:"inputHash"`
			Status         string `json:"status"`
			OutputDigest   string `json:"outputDigest"`
			CostMicros     int64  `json:"costMicros"`
			DurationMs     *int64 `json:"durationMs"`
		} `json:"agentRun"`
		Proposal struct {
			ID        string  `json:"id"`
			AgentRun  *string `json:"agentRunId"`
			CreatedBy string  `json:"createdBy"`
			State     string  `json:"state"`
		} `json:"proposal"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode generation result: %v\n%s", err, response.Body.String())
	}
	run := result.AgentRun
	if run.Status != "succeeded" || run.OutputDigest == "" || run.DurationMs == nil ||
		run.CostMicros != 200 || !strings.HasPrefix(run.InputHash, "sha256:") {
		t.Fatalf("agent run record = %+v", run)
	}
	if run.ConfigRevision != governanceapp.CredentialRevisionDigest(credentialSecret)+"/"+settingID.String() {
		t.Fatalf("config revision = %s", run.ConfigRevision)
	}
	if result.Proposal.AgentRun == nil || *result.Proposal.AgentRun != run.ID {
		t.Fatalf("proposal attribution = %+v", result.Proposal)
	}

	// The proposal is authored by the workspace's seeded agent principal (D6).
	agentPrincipal, err := environment.store.WorkspaceAgentPrincipal(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if result.Proposal.CreatedBy != agentPrincipal.ID.String() {
		t.Fatalf("proposal createdBy = %s, want the seeded agent principal %s",
			result.Proposal.CreatedBy, agentPrincipal.ID.String())
	}

	// Full §8.6 record: model step + create_proposal tool step on one run.
	var stepCount int
	runUUID := mustID(t, func() (identity.AgentRunID, error) {
		return identity.ParseAgentRunID(run.ID)
	}).UUID()
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM agent_steps WHERE agent_run_id = $1
	`, runUUID).Scan(&stepCount); err != nil {
		t.Fatal(err)
	}
	if stepCount != 2 {
		t.Fatalf("agent steps = %d, want model + tool", stepCount)
	}

	// The generated proposal is submittable through the T003 endpoint and
	// walks into the normal validation pipeline.
	submitResponse := environment.request(t, http.MethodPost,
		"/api/v1/workspaces/"+workspace.String()+"/governance/proposals/"+result.Proposal.ID+"/submit", "", "")
	if submitResponse.Code != http.StatusOK {
		t.Fatalf("submit status = %d, body = %s", submitResponse.Code, submitResponse.Body.String())
	}
	assertJSONField(t, submitResponse.Body.Bytes(), "state", "validating")
}

func TestGenerateProposalRejectsInvalidModelOutput(t *testing.T) {
	t.Setenv(credentialEnvName, credentialSecret)
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "generation-invalid")
	assetID, revisionID := environment.createAsset(t, workspace)

	server, _ := openAICompatibleStub(t, func(_, prompt string, _ *string) string {
		return fmt.Sprintf(`{"targetObjectType":"semantic_asset","targetObjectId":%q,
			"baseRevisionId":%q,"title":"Broken output","agentAttribution":%s}`,
			assetID.String(), revisionID.String(), attributionEcho(prompt))
	})
	providerID, _ := createProvider(t, environment, workspace, providerBody(
		"openai_compatible", "Broken Router", server.URL, credentialEnvName))
	createSetting(t, environment, workspace, providerID,
		settingBody(providerID, "llm", "gpt-4.1", "推理", 4096, nil))

	generate := fmt.Sprintf(
		`{"targetObjectType":"semantic_asset","targetObjectId":%q,"instruction":"Tighten the definition"}`,
		assetID.String())
	response := environment.request(t, http.MethodPost,
		"/api/v1/workspaces/"+workspace.String()+"/governance/agent-runs/generate-proposal", "", generate)
	assertJSONField(t, response.Body.Bytes(), "code", "AI_OUTPUT_INVALID")
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("generation status = %d, want 422", response.Code)
	}
	var runStatus string
	if err := environment.pool.QueryRow(context.Background(),
		"SELECT status FROM agent_runs").Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if runStatus != "failed" {
		t.Fatalf("run status = %s, want failed", runStatus)
	}
	var stepError *string
	if err := environment.pool.QueryRow(context.Background(),
		"SELECT error_code FROM agent_steps WHERE kind = 'model'").Scan(&stepError); err != nil {
		t.Fatal(err)
	}
	if stepError == nil || *stepError != "AI_OUTPUT_INVALID" {
		t.Fatalf("step error = %v", stepError)
	}
	assertQueryCount(t, environment.pool, "SELECT count(*) FROM proposals", nil, 0)
}

func TestGenerateProposalRejectsUnsupportedProviderProtocol(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "generation-unsupported")
	assetID, _ := environment.createAsset(t, workspace)

	providerID, _ := createProvider(t, environment, workspace, providerBody(
		"anthropic", "Claude Enterprise", "", credentialEnvName))
	createSetting(t, environment, workspace, providerID,
		settingBody(providerID, "llm", "claude-sonnet-4", "视觉 · 推理", 4096, nil))

	generate := fmt.Sprintf(
		`{"targetObjectType":"semantic_asset","targetObjectId":%q,"instruction":"Tighten the definition"}`,
		assetID.String())
	response := environment.request(t, http.MethodPost,
		"/api/v1/workspaces/"+workspace.String()+"/governance/agent-runs/generate-proposal", "", generate)
	assertJSONField(t, response.Body.Bytes(), "code", "PROVIDER_UNSUPPORTED")
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("generation status = %d, want 422", response.Code)
	}
	// No provider execution attempt exists for an unsupported protocol, so
	// no agent run is opened — there is nothing to record.
	assertQueryCount(t, environment.pool, "SELECT count(*) FROM agent_runs", nil, 0)
	assertQueryCount(t, environment.pool, "SELECT count(*) FROM proposals", nil, 0)
}

func TestGenerateProposalRequiresAssetProposeServerSide(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "generation-authz")
	assetID, _ := environment.createAsset(t, workspace)
	unbound := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: unbound, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Unbound Human", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}

	providerID, _ := createProvider(t, environment, workspace, providerBody(
		"openai_compatible", "Denied Router", "https://models.example.com/v1", credentialEnvName))
	createSetting(t, environment, workspace, providerID,
		settingBody(providerID, "llm", "gpt-4.1", "推理", 4096, nil))

	generate := fmt.Sprintf(
		`{"targetObjectType":"semantic_asset","targetObjectId":%q,"instruction":"Tighten the definition"}`,
		assetID.String())
	response := environment.request(t, http.MethodPost,
		"/api/v1/workspaces/"+workspace.String()+"/governance/agent-runs/generate-proposal", unbound.String(), generate)
	if response.Code != http.StatusForbidden {
		t.Fatalf("generation status = %d, want 403, body = %s", response.Code, response.Body.String())
	}
	assertJSONField(t, response.Body.Bytes(), "code", "NO_MATCHING_GRANT")
	assertQueryCount(t, environment.pool, "SELECT count(*) FROM agent_runs", nil, 0)
	assertQueryCount(t, environment.pool, "SELECT count(*) FROM proposals", nil, 0)
}

func TestSeededAgentAttributedProposalPathRemainsFunctional(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "generation-regression")
	assetID, revisionID := environment.createAsset(t, workspace)

	// The T003 path: a pre-created run (no live provider) attributed to the
	// workspace's seeded agent principal, then a proposal through the T003
	// endpoint carrying the matching agentAttribution block.
	agentPrincipal, err := environment.store.WorkspaceAgentPrincipal(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	clock := governanceapp.ClockFunc(func() time.Time { return time.Now().UTC() })
	runs := governanceapp.NewAgentRunService(environment.store, clock)
	run, err := runs.Start(context.Background(), governanceapp.StartAgentRunRequest{
		WorkspaceID:    workspace,
		PrincipalID:    &agentPrincipal.ID,
		Model:          "gpt-4.1",
		ConfigRevision: governanceapp.CredentialRevisionDigest(credentialSecret) + "/seeded",
		InputHash:      "sha256:" + strings.Repeat("ab", 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Finish(context.Background(), governanceapp.FinishAgentRunRequest{
		WorkspaceID: workspace, RunID: run.ID, FinalState: governance.AgentRunSucceeded,
		OutputDigest: "sha256:" + strings.Repeat("cd", 32), CostMicros: 10,
	}); err != nil {
		t.Fatal(err)
	}

	path := environment.proposalsPath(t, workspace)
	body := fmt.Sprintf(`{"targetObjectType":"semantic_asset","targetObjectId":%q,"baseRevisionId":%q,%s,
		"title":"Agent draft","agentAttribution":
		{"agentRunId":%q,"model":"gpt-4.1","configRevision":%q,"inputHash":"sha256:%s"}}`,
		assetID.String(), revisionID.String(), substantiveChangeSet, run.ID.String(),
		governanceapp.CredentialRevisionDigest(credentialSecret)+"/seeded", strings.Repeat("ab", 32))
	response := environment.request(t, http.MethodPost, path, "", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("seeded agent proposal status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		AgentRunID string `json:"agentRunId"`
		CreatedBy  string `json:"createdBy"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.AgentRunID != run.ID.String() {
		t.Fatalf("agentRunId = %s, want %s", payload.AgentRunID, run.ID.String())
	}
	if payload.CreatedBy != agentPrincipal.ID.String() {
		t.Fatalf("createdBy = %s, want the seeded agent principal", payload.CreatedBy)
	}
}
