package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/internal/application"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/sdk/trace"
)

func newProductionHandler(t *testing.T, store *pgstore.Store) http.Handler {
	t.Helper()
	prodService := governanceapp.NewProductionService(store)
	sysService := application.NewSystemService(
		application.ReadinessProbeFunc(func(context.Context) error { return nil }),
		domain.SystemInfo{APIVersion: "v1", SchemaVersion: "0.4.0", BuildVersion: "test-build"},
	)
	tracer := trace.NewTracerProvider().Tracer("production-authoring-integration")
	return httpapi.NewHandler(
		sysService,
		slog.New(slog.NewTextHandler(os.Stderr, nil)),
		tracer,
		httpapi.WithProduction(prodService),
		httpapi.WithAuthorization(authorizationapp.NewService(store, authorizationapp.ClockFunc(time.Now))),
	)
}

func sendProdRequest(handler http.Handler, method, path, principal, idempotencyKey, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if principal != "" {
		req.Header.Set("X-Semlia-Principal", principal)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func mustParseProposalID(t *testing.T, s string) identity.ProposalID {
	t.Helper()
	id, err := identity.ParseProposalID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustParseProductionOperationID(t *testing.T, s string) identity.ProductionOperationID {
	t.Helper()
	id, err := identity.ParseProductionOperationID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestProductionAuthoring_ColdStartAndAuthoritativeRecovery(t *testing.T) {
	fixture := newFixture(t)
	handler := newProductionHandler(t, fixture.store)
	workspace := createWorkspace(t, fixture.pool, "prod-cold-start")
	principalID := authoringLifecyclePrincipal(t, fixture, workspace, "Author")
	candID, err := identity.NewSemanticCandidateID()
	if err != nil {
		t.Fatal(err)
	}

	collectionPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", workspace.String())

	createPayload := fmt.Sprintf(`{
		"input": {
			"candidates": [
				{
					"candidateId": "%s",
					"digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
					"primaryTargetKey": "net_revenue",
					"targetKeys": ["net_revenue"]
				}
			]
		},
		"targets": [
			{
				"intent": "create",
				"kind": "semantic_asset",
				"localKey": "net_revenue",
				"identityKey": "revenue_net",
				"title": "Net Revenue",
				"content": {
					"address": "finance.net_revenue",
					"assetType": "metric",
					"displayName": "Net Revenue",
					"definition": "Revenue after refunds and chargebacks"
				}
			}
		]
	}`, candID.String())

	// 1. Cold start creation
	inputFixture := productionFixtureInput(t, fixture, workspace, candID)
	createPayload = productionPayloadInput(t, createPayload, inputFixture, principalID)
	createRec := sendProdRequest(handler, http.MethodPost, collectionPath, principalID.String(), "key-cold-start-001", createPayload)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on cold start, got %d: %s", createRec.Code, createRec.Body.String())
	}

	location := createRec.Header().Get("Location")
	if !strings.Contains(location, "/production-operations/prodop_") {
		t.Fatalf("expected Location header with prodop, got %q", location)
	}

	var createResult map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResult); err != nil {
		t.Fatal(err)
	}
	if createResult["replayed"] != false {
		t.Errorf("expected replayed=false")
	}
	if createResult["outcome"] != "created" {
		t.Errorf("expected outcome=created, got %v", createResult["outcome"])
	}
	if createResult["version"].(float64) != 1 {
		t.Errorf("expected version 1, got %v", createResult["version"])
	}
	opID := createResult["operationId"].(string)
	if !strings.HasPrefix(opID, "prodop_") {
		t.Fatalf("expected prodop_ prefix, got %q", opID)
	}
	proposals, ok := createResult["proposalIds"].([]any)
	if !ok || len(proposals) != 1 {
		t.Fatalf("expected 1 proposal ID, got %v", createResult["proposalIds"])
	}
	proposalID := proposals[0].(string)
	if !strings.HasPrefix(proposalID, "prp_") {
		t.Fatalf("expected prp_ prefix, got %q", proposalID)
	}

	// Verify database state: draft asset exists with current_revision_id IS NULL
	var draftAssetCount int
	if err := fixture.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM semantic_assets WHERE workspace_id = $1 AND current_revision_id IS NULL",
		workspace.UUID()).Scan(&draftAssetCount); err != nil {
		t.Fatal(err)
	}
	if draftAssetCount != 1 {
		t.Errorf("expected 1 draft asset with current_revision_id IS NULL, got %d", draftAssetCount)
	}

	// Verify proposal state in database: intent = 'create', production_operation_id = opID, base_revision_id IS NULL
	var propIntent string
	var propProdOpID string
	var propBaseRev *string
	if err := fixture.pool.QueryRow(context.Background(),
		"SELECT intent, production_operation_id::text, base_revision_id::text FROM proposals WHERE id = $1",
		mustParseProposalID(t, proposalID).UUID()).Scan(&propIntent, &propProdOpID, &propBaseRev); err != nil {
		t.Fatal(err)
	}
	if propIntent != "create" {
		t.Errorf("expected proposal intent 'create', got %q", propIntent)
	}
	if propProdOpID != mustParseProductionOperationID(t, opID).UUID() {
		t.Errorf("expected proposal production_operation_id %q, got %q", mustParseProductionOperationID(t, opID).UUID(), propProdOpID)
	}
	if propBaseRev != nil {
		t.Errorf("expected base_revision_id to be NULL for create intent, got %v", *propBaseRev)
	}

	// Verify production target link
	var targetLocalKey string
	if err := fixture.pool.QueryRow(context.Background(),
		"SELECT local_key FROM production_targets WHERE workspace_id = $1 AND operation_id = $2 AND proposal_id = $3",
		workspace.UUID(), mustParseProductionOperationID(t, opID).UUID(), mustParseProposalID(t, proposalID).UUID()).Scan(&targetLocalKey); err != nil {
		t.Fatal(err)
	}
	if targetLocalKey != "net_revenue" {
		t.Errorf("expected target local_key 'net_revenue', got %q", targetLocalKey)
	}

	// 2. Authoritative Server Recovery (GET)
	itemPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s", workspace.String(), opID)
	getRec := sendProdRequest(handler, http.MethodGet, itemPath, principalID.String(), "", "")
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET, got %d: %s", getRec.Code, getRec.Body.String())
	}

	var opDetail map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &opDetail); err != nil {
		t.Fatal(err)
	}
	if opDetail["version"].(float64) != 1 {
		t.Errorf("expected detail version 1, got %v", opDetail["version"])
	}
	targets, ok := opDetail["targets"].([]any)
	if !ok || len(targets) != 1 {
		t.Fatalf("expected 1 target in detail, got %v", opDetail["targets"])
	}
	target0 := targets[0].(map[string]any)
	if target0["localKey"] != "net_revenue" {
		t.Errorf("expected localKey net_revenue, got %v", target0["localKey"])
	}
	if target0["outcome"] != "proposal" {
		t.Errorf("expected target outcome proposal, got %v", target0["outcome"])
	}
	if target0["proposalId"] != proposalID {
		t.Errorf("expected target proposalId %s, got %v", proposalID, target0["proposalId"])
	}

	// 3. Idempotent replay: exact same request & key returns 200 with replayed=true
	replayRec := sendProdRequest(handler, http.MethodPost, collectionPath, principalID.String(), "key-cold-start-001", createPayload)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on replay, got %d: %s", replayRec.Code, replayRec.Body.String())
	}
	var replayResult map[string]any
	if err := json.Unmarshal(replayRec.Body.Bytes(), &replayResult); err != nil {
		t.Fatal(err)
	}
	if replayResult["replayed"] != true {
		t.Errorf("expected replayed=true on replay")
	}
	if replayResult["operationId"] != opID {
		t.Errorf("expected same operationId %s, got %v", opID, replayResult["operationId"])
	}

	// 4. Same key different content returns 409 IDEMPOTENCY_CONFLICT
	diffPayload := strings.Replace(createPayload, "Revenue after refunds and chargebacks", "Changed definition", 1)
	conflictRec := sendProdRequest(handler, http.MethodPost, collectionPath, principalID.String(), "key-cold-start-001", diffPayload)
	if conflictRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for same key different body, got %d: %s", conflictRec.Code, conflictRec.Body.String())
	}
	if !strings.Contains(conflictRec.Body.String(), "IDEMPOTENCY_CONFLICT") {
		t.Errorf("expected IDEMPOTENCY_CONFLICT error code, got %s", conflictRec.Body.String())
	}

	// 5. Different key identical business content returns 409 ALREADY_PRODUCED
	dupClaimRec := sendProdRequest(handler, http.MethodPost, collectionPath, principalID.String(), "key-cold-start-002", createPayload)
	if dupClaimRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for duplicate business content, got %d: %s", dupClaimRec.Code, dupClaimRec.Body.String())
	}
	if !strings.Contains(dupClaimRec.Body.String(), "ALREADY_PRODUCED") {
		t.Errorf("expected ALREADY_PRODUCED error code, got %s", dupClaimRec.Body.String())
	}

	// 6. Draft replacement (PUT) advances version to 2
	replacePayload := fmt.Sprintf(`{
		"expectedVersion": 1,
		"input": {
			"candidates": [
				{
					"candidateId": "%s",
					"digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
					"primaryTargetKey": "net_revenue",
					"targetKeys": ["net_revenue"]
				}
			]
		},
		"targets": [
			{
				"intent": "create",
				"kind": "semantic_asset",
				"localKey": "net_revenue",
				"identityKey": "revenue_net",
				"title": "Net Revenue Updated",
				"content": {
					"address": "finance.net_revenue",
					"assetType": "metric",
					"displayName": "Net Revenue Revised",
					"definition": "Revenue after all refunds, chargebacks, and disputes"
				}
			}
		]
	}`, candID.String())
	replacePayload = productionPayloadInput(t, replacePayload, inputFixture, principalID)
	replaceRec := sendProdRequest(handler, http.MethodPut, itemPath, principalID.String(), "key-replace-001", replacePayload)
	if replaceRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on replace, got %d: %s", replaceRec.Code, replaceRec.Body.String())
	}
	var replaceResult map[string]any
	if err := json.Unmarshal(replaceRec.Body.Bytes(), &replaceResult); err != nil {
		t.Fatal(err)
	}
	if replaceResult["version"].(float64) != 2 {
		t.Errorf("expected version 2 after replace, got %v", replaceResult["version"])
	}
	if replaceResult["outcome"] != "updated" {
		t.Errorf("expected outcome=updated, got %v", replaceResult["outcome"])
	}

	// Verify database has 2 versions recorded for this operation
	var versionCount int
	if err := fixture.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM production_versions WHERE operation_id = $1",
		mustParseProductionOperationID(t, opID).UUID()).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if versionCount != 2 {
		t.Errorf("expected 2 version records in database, got %d", versionCount)
	}

	// 7. PUT with stale expectedVersion returns 409
	staleRec := sendProdRequest(handler, http.MethodPut, itemPath, principalID.String(), "key-replace-002", replacePayload) // expectedVersion is still 1
	if staleRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on stale expectedVersion, got %d: %s", staleRec.Code, staleRec.Body.String())
	}

	// 8. List operations
	listPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations?limit=10", workspace.String())
	listRec := sendProdRequest(handler, http.MethodGet, listPath, principalID.String(), "", "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on list, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var listResult map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResult); err != nil {
		t.Fatal(err)
	}
	items, ok := listResult["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 item in list, got %v", listResult["items"])
	}

	// 9. Cross-workspace isolation
	otherWorkspace := createWorkspace(t, fixture.pool, "prod-other-wsp")
	otherPrincipal := authoringLifecyclePrincipal(t, fixture, otherWorkspace, "Other author")
	otherItemPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s", otherWorkspace.String(), opID)
	crossWspRec := sendProdRequest(handler, http.MethodGet, otherItemPath, otherPrincipal.String(), "", "")
	if crossWspRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 NOT_FOUND for cross-workspace read, got %d: %s", crossWspRec.Code, crossWspRec.Body.String())
	}
}
