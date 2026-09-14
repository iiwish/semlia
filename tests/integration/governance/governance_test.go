package governance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func decodeProposalDetail(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode proposal detail: %v\n%s", err, body)
	}
	return payload
}

func TestGovernanceProposalJourneyOverHTTP(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-journey")
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	basePath := environment.proposalsPath(t, workspace)
	created := environment.request(t, http.MethodPost, basePath, "", proposalCreateBody("semantic_asset", assetID.String(), baseRevisionID.String()))
	if created.Code != http.StatusCreated {
		t.Fatalf("proposal create status = %d, body = %s", created.Code, created.Body.String())
	}
	proposal := decodeProposalDetail(t, created.Body.Bytes())
	proposalID, _ := proposal["id"].(string)
	if !strings.HasPrefix(proposalID, "prp_") {
		t.Fatalf("proposal id = %q, want prp_ prefix", proposalID)
	}
	assertJSONField(t, created.Body.Bytes(), "state", "draft")
	assertJSONField(t, created.Body.Bytes(), "targetObjectId", assetID.String())
	author, err := environment.store.LoadDefaultPrincipal(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONField(t, created.Body.Bytes(), "createdBy", author.ID.String())
	changeSet, ok := proposal["changeSet"].([]any)
	if !ok || len(changeSet) != 1 {
		t.Fatalf("created change-set = %v", proposal["changeSet"])
	}
	changeItem, _ := changeSet[0].(map[string]any)
	if itemID, _ := changeItem["id"].(string); !strings.HasPrefix(itemID, "chg_") {
		t.Fatalf("change-set item id = %q, want chg_ prefix", itemID)
	}
	if changeItem["afterValue"] != "Revenue after refunds and chargebacks" {
		t.Fatalf("change-set item afterValue = %v", changeItem["afterValue"])
	}

	submitted := environment.request(t, http.MethodPost, basePath+"/"+proposalID+"/submit", "", "")
	if submitted.Code != http.StatusOK {
		t.Fatalf("proposal submit status = %d, body = %s", submitted.Code, submitted.Body.String())
	}
	// T004 orchestration: the submit endpoint walks draft -> proposed ->
	// validating and enqueues exactly one validation job, so the returned
	// state is already validating.
	assertJSONField(t, submitted.Body.Bytes(), "state", "validating")
	if submittedAt := decodeProposalDetail(t, submitted.Body.Bytes())["submittedAt"]; submittedAt == nil {
		t.Fatal("submitted proposal without submittedAt")
	}

	detail := environment.request(t, http.MethodGet, basePath+"/"+proposalID, "", "")
	if detail.Code != http.StatusOK {
		t.Fatalf("proposal detail status = %d, body = %s", detail.Code, detail.Body.String())
	}
	assertJSONField(t, detail.Body.Bytes(), "state", "validating")
	assertJSONField(t, detail.Body.Bytes(), "title", "Tighten metric definition")
	storageLeak := strings.Contains(detail.Body.String(), assetID.UUID())
	if storageLeak {
		t.Fatal("proposal detail leaked the storage UUID of the target")
	}

	secondCreate := environment.request(t, http.MethodPost, environment.proposalsPath(t, workspace), "",
		proposalCreateBody("semantic_asset", assetID.String(), baseRevisionID.String()))
	if secondCreate.Code != http.StatusCreated {
		t.Fatalf("second proposal create status = %d, body = %s", secondCreate.Code, secondCreate.Body.String())
	}
	firstPage := environment.request(t, http.MethodGet, basePath+"?limit=1", "", "")
	if firstPage.Code != http.StatusOK {
		t.Fatalf("proposal list status = %d, body = %s", firstPage.Code, firstPage.Body.String())
	}
	var page map[string]any
	if err := json.Unmarshal(firstPage.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	items, _ := page["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("first list page items = %v", page["items"])
	}
	pageInfo, _ := page["page"].(map[string]any)
	nextCursor, _ := pageInfo["nextCursor"].(string)
	if nextCursor == "" {
		t.Fatal("first list page without nextCursor")
	}
	secondPage := environment.request(t, http.MethodGet,
		environment.proposalsPath(t, workspace)+"?limit=1&cursor="+nextCursor, "", "")
	if secondPage.Code != http.StatusOK {
		t.Fatalf("second list page status = %d, body = %s", secondPage.Code, secondPage.Body.String())
	}
	var second map[string]any
	if err := json.Unmarshal(secondPage.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	secondItems, _ := second["items"].([]any)
	if len(secondItems) != 1 {
		t.Fatalf("second list page items = %v", second["items"])
	}
	if first, _ := items[0].(map[string]any)["id"]; first == secondItems[0] {
		t.Fatal("cursor page repeated the same proposal")
	}
	if second["page"].(map[string]any)["nextCursor"] != nil {
		t.Fatal("last page must not carry a nextCursor")
	}

	var submitAllow, readAllow int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM authorization_events
		WHERE workspace_id = $1 AND action = 'asset.propose' AND decision = 'allow'`,
		workspace.UUID()).Scan(&submitAllow); err != nil {
		t.Fatal(err)
	}
	if submitAllow != 3 {
		t.Fatalf("asset.propose allow events = %d, want one per create and submit", submitAllow)
	}
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM authorization_events
		WHERE workspace_id = $1 AND action = 'asset.read' AND decision = 'allow'`,
		workspace.UUID()).Scan(&readAllow); err != nil {
		t.Fatal(err)
	}
	if readAllow < 2 {
		t.Fatalf("asset.read allow events = %d, want one per detail and list read", readAllow)
	}
	var auditFacts int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'governance.proposal.submitted'`,
		workspace.UUID()).Scan(&auditFacts); err != nil {
		t.Fatal(err)
	}
	if auditFacts != 1 {
		t.Fatalf("submission audit facts = %d, want exactly one", auditFacts)
	}
}

func TestGovernanceProposalOverJoinContractTarget(t *testing.T) {
	environment := newFixture(t)
	workspace, contractID := seedJoinContract(t, environment, "governance-contract")
	path := environment.proposalsPath(t, workspace)

	created := environment.request(t, http.MethodPost, path, "", proposalCreateBody("join_contract", contractID.String(), ""))
	if created.Code != http.StatusCreated {
		t.Fatalf("join-contract proposal create status = %d, body = %s", created.Code, created.Body.String())
	}
	assertJSONField(t, created.Body.Bytes(), "state", "draft")
	assertJSONField(t, created.Body.Bytes(), "targetObjectType", "join_contract")
	assertJSONField(t, created.Body.Bytes(), "targetObjectId", contractID.String())
	proposalID, _ := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)

	submitted := environment.request(t, http.MethodPost, environment.proposalsPath(t, workspace)+"/"+proposalID+"/submit", "", "")
	if submitted.Code != http.StatusOK {
		t.Fatalf("join-contract proposal submit status = %d, body = %s", submitted.Code, submitted.Body.String())
	}
	assertJSONField(t, submitted.Body.Bytes(), "state", "validating")
	assertJSONField(t, submitted.Body.Bytes(), "targetObjectId", contractID.String())
	environment.runValidationWorker(t)
	proposalTyped, err := identity.ParseProposalID(proposalID)
	if err != nil {
		t.Fatal(err)
	}
	var targetType, targetID string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT target_type,target_id FROM attention_items WHERE workspace_id=$1 AND dedupe_key=$2`,
		workspace.UUID(), "review:"+proposalTyped.UUID()).Scan(&targetType, &targetID); err != nil {
		t.Fatal(err)
	}
	if targetType != "workspace" || targetID != workspace.String() {
		t.Fatalf("assetless join attention target = %s/%s, want workspace/%s", targetType, targetID, workspace)
	}
}

func TestMalformedAgentPayloadIsRejectedWithZeroDomainWrites(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-ai-gate")
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	path := environment.proposalsPath(t, workspace)

	valid := `{"targetObjectType":"semantic_asset","targetObjectId":"` + assetID.String() +
		`","baseRevisionId":"` + baseRevisionID.String() + `","title":"Agent draft","changeSet":[{"fieldPath":"definition","op":"update","beforeDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","afterDigest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","beforeValue":"before","afterValue":"after"}],"agentAttribution":{"agentRunId":"` +
		mustID(t, identity.NewAgentRunID).String() + `","model":"semlia-test-model","configRevision":"config-1","inputHash":"` + digestOf("input") + `"}}`

	tests := []struct {
		name   string
		mutate func(string) string
	}{
		{
			name:   "missing required field",
			mutate: func(payload string) string { return strings.Replace(payload, `"title":"Agent draft",`, "", 1) },
		},
		{
			name: "unknown field",
			mutate: func(payload string) string {
				return strings.Replace(payload, `{"targetObjectType"`, `{"prompt":"raw agent instructions","targetObjectType"`, 1)
			},
		},
		{
			name: "wrong type",
			mutate: func(payload string) string {
				return strings.Replace(payload, `"changeSet":[{`, `"changeSet":{"fieldPath":"definition"}, "items":[{`, 1)
			},
		},
		{
			name: "ungoverned digest shape",
			mutate: func(payload string) string {
				return strings.Replace(payload, "afterDigest\":\"sha256:", "afterDigest\":\"md5:", 1)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := environment.request(t, http.MethodPost, path, "", test.mutate(valid))
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("malformed agent payload status = %d, body = %s", response.Code, response.Body.String())
			}
			assertJSONField(t, response.Body.Bytes(), "code", "AI_OUTPUT_INVALID")
			var payload map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			details, _ := payload["details"].(map[string]any)
			if len(details["violations"].([]any)) == 0 {
				t.Fatalf("422 without violation summaries: %s", response.Body.String())
			}
		})
	}

	assertTableCount(t, environment.pool, "proposals", 0)
	assertTableCount(t, environment.pool, "proposal_changes", 0)
	assertTableCount(t, environment.pool, "agent_runs", 0)
	assertTableCount(t, environment.pool, "authorization_events", 0)
}

func TestAgentAttributedProposalReferencesPersistedAgentRun(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-agent-run")
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	path := environment.proposalsPath(t, workspace)

	agentRuns := governanceapp.NewAgentRunService(environment.store, governanceapp.ClockFunc(func() time.Time { return time.Now().UTC() }))
	run, err := agentRuns.Start(context.Background(), governanceapp.StartAgentRunRequest{
		WorkspaceID: workspace, Model: "semlia-test-model", ConfigRevision: "config-7", InputHash: digestOf("canonical input"),
	})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"targetObjectType":"semantic_asset","targetObjectId":"` + assetID.String() +
		`","baseRevisionId":"` + baseRevisionID.String() + `","title":"Agent draft","changeSet":[{"fieldPath":"definition","op":"update","beforeDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","afterDigest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","beforeValue":"before","afterValue":"after"}],"agentAttribution":{"agentRunId":"` +
		run.ID.String() + `","model":"semlia-test-model","configRevision":"config-7","inputHash":"` + digestOf("canonical input") + `"}}`

	created := environment.request(t, http.MethodPost, path, "", body)
	if created.Code != http.StatusCreated {
		t.Fatalf("agent-attributed create status = %d, body = %s", created.Code, created.Body.String())
	}
	assertJSONField(t, created.Body.Bytes(), "agentRunId", run.ID.String())
	assertJSONField(t, created.Body.Bytes(), "createdBy", "agent-run:"+run.ID.String())
	proposalID, _ := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
	parsedProposalID := mustID(t, func() (identity.ProposalID, error) {
		return identity.ParseProposalID(proposalID)
	})
	var linkedRun string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT ar.id FROM proposals p
		JOIN agent_runs ar ON ar.workspace_id = p.workspace_id AND ar.id = p.agent_run_id
		WHERE p.workspace_id = $1 AND p.id = $2`,
		workspace.UUID(), parsedProposalID.UUID()).Scan(&linkedRun); err != nil {
		t.Fatalf("proposal not linked to its agent run: %v", err)
	}

	mismatched := strings.Replace(body, `"model":"semlia-test-model"`, `"model":"other-model"`, 1)
	mismatch := environment.request(t, http.MethodPost, path, "", mismatched)
	if mismatch.Code != http.StatusBadRequest {
		t.Fatalf("mismatched attribution status = %d, body = %s", mismatch.Code, mismatch.Body.String())
	}
	assertJSONField(t, mismatch.Body.Bytes(), "code", "INVALID_ARGUMENT")

	unknownRun := strings.Replace(body, run.ID.String(), mustID(t, identity.NewAgentRunID).String(), 1)
	missing := environment.request(t, http.MethodPost, path, "", unknownRun)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unknown agent run status = %d, body = %s", missing.Code, missing.Body.String())
	}

	unchanged, err := environment.store.GetProposal(context.Background(), workspace, parsedProposalID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.State != governance.ProposalDraft {
		t.Fatalf("proposal state after rejected attributions = %s", unchanged.State)
	}
}

func TestNoSubstantiveChangeNeverEntersTheHumanQueue(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-noop")
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	path := environment.proposalsPath(t, workspace)

	noOpBody := `{"targetObjectType":"semantic_asset","targetObjectId":"` + assetID.String() +
		`","baseRevisionId":"` + baseRevisionID.String() + `","title":"No-op draft","changeSet":[{"fieldPath":"definition","op":"update","beforeDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","afterDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","beforeValue":"same","afterValue":"same"}],"createdBy":"founder"}`
	created := environment.request(t, http.MethodPost, path, "", noOpBody)
	if created.Code != http.StatusUnprocessableEntity {
		t.Fatalf("no-op create status = %d, body = %s", created.Code, created.Body.String())
	}
	assertJSONField(t, created.Body.Bytes(), "code", "NO_SUBSTANTIVE_CHANGE")

	emptyBody := `{"targetObjectType":"join_contract","targetObjectId":"` + mustID(t, identity.NewJoinContractID).String() +
		`","title":"Empty draft","changeSet":[],"createdBy":"founder"}`
	empty := environment.request(t, http.MethodPost, path, "", emptyBody)
	if empty.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty change-set create status = %d, body = %s", empty.Code, empty.Body.String())
	}
	assertJSONField(t, empty.Body.Bytes(), "code", "NO_SUBSTANTIVE_CHANGE")

	// The submit-side gate: a draft whose persisted items all normalize to
	// no-ops (assembled through the change-set APIs, which have no HTTP
	// surface in this task) must refuse submission and stay a draft.
	clock := governanceapp.ClockFunc(func() time.Time { return time.Now().UTC() })
	proposals := governanceapp.NewProposalService(environment.store, clock)
	draft, err := proposals.CreateProposal(context.Background(), governanceapp.CreateProposalRequest{
		WorkspaceID: workspace, AssetID: assetID, BaseRevisionID: baseRevisionID,
		Title: "No-op draft", CreatedBy: "founder",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proposals.AddChange(context.Background(), governanceapp.AddChangeRequest{
		WorkspaceID: workspace, ProposalID: draft.ID,
		Item: governance.ChangeSetItem{
			FieldPath: "definition", Op: governance.ChangeUpdate,
			BeforeDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			AfterDigest:  "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			BeforeValue:  json.RawMessage(`"same"`), AfterValue: json.RawMessage(`"same"`),
		},
	}); err != nil {
		t.Fatal(err)
	}
	authoring := governanceapp.NewAuthoringService(environment.store, proposals,
		governanceapp.NewAgentRunService(environment.store, clock), nil, clock)
	if _, err := authoring.SubmitProposal(context.Background(), governanceapp.SubmitAuthoringProposalRequest{
		WorkspaceID: workspace, ProposalID: draft.ID, TraceID: traceID,
	}); err != governanceapp.ErrNoSubstantiveChange {
		t.Fatalf("submit gate error = %v, want ErrNoSubstantiveChange", err)
	}
	stillDraft, err := environment.store.GetProposal(context.Background(), workspace, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stillDraft.State != governance.ProposalDraft {
		t.Fatalf("no-substantive-change proposal transitioned to %s", stillDraft.State)
	}
}

func TestUnboundPrincipalIsDeniedProposalCreationWithAuditRow(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-authz")
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	path := environment.proposalsPath(t, workspace)
	unbound := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: unbound, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Unbound Member", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}

	created := environment.request(t, http.MethodPost, path, unbound.String(),
		proposalCreateBody("semantic_asset", assetID.String(), baseRevisionID.String()))
	if created.Code != http.StatusForbidden {
		t.Fatalf("unbound create status = %d, body = %s", created.Code, created.Body.String())
	}
	assertJSONField(t, created.Body.Bytes(), "code", "NO_MATCHING_GRANT")
	assertTableCount(t, environment.pool, "proposals", 0)

	listed := environment.request(t, http.MethodGet, path, unbound.String(), "")
	if listed.Code != http.StatusForbidden {
		t.Fatalf("unbound list status = %d, body = %s", listed.Code, listed.Body.String())
	}
	assertJSONField(t, listed.Body.Bytes(), "code", "NO_MATCHING_GRANT")

	var decision, reasonCode, resourceType string
	var resourceID string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT decision, reason_code, resource_type, resource_id FROM authorization_events
		WHERE workspace_id = $1 AND principal_id = $2 AND action = 'asset.propose'`,
		workspace.UUID(), unbound.UUID()).Scan(&decision, &reasonCode, &resourceType, &resourceID); err != nil {
		t.Fatalf("denial decision event not recorded: %v", err)
	}
	if decision != "deny" || reasonCode != "NO_MATCHING_GRANT" || resourceType != "asset" || resourceID != assetID.UUID() {
		t.Fatalf("recorded denial = %s %s %s %s", decision, reasonCode, resourceType, resourceID)
	}
}
