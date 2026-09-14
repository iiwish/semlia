package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iiwish/semlia/internal/application"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/jobs"
	workbenchapp "github.com/iiwish/semlia/internal/application/workbench"
	"github.com/iiwish/semlia/internal/domain"
	authorizationdomain "github.com/iiwish/semlia/internal/domain/authorization"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/sdk/trace"
)

func newFullGovernanceHandler(t *testing.T, fixture *fixture) http.Handler {
	t.Helper()
	store := fixture.store
	authorizer := authorizationapp.NewService(
		store, authorizationapp.ClockFunc(time.Now), authorizationapp.WithLocalUATIdentities(),
	)
	catalog := catalogapp.NewService(store, catalogapp.ClockFunc(func() time.Time { return time.Now().UTC() }))
	clock := governanceapp.ClockFunc(func() time.Time { return time.Now().UTC() })
	governancePolicy := governanceapp.NewPolicyService(
		store, clock, governanceapp.WithRuleSource(store))
	authoring := governanceapp.NewAuthoringService(
		store,
		governanceapp.NewProposalService(store, clock),
		governanceapp.NewAgentRunService(store, clock),
		authorizer, clock,
		governanceapp.WithValidationOrchestrator(
			governanceapp.NewValidationOrchestrator(
				governanceapp.NewProposalService(store, clock), store, clock,
			),
		),
		governanceapp.WithDecisionRefresher(
			governanceapp.NewPolicyDecisionTrigger(store, governancePolicy),
		),
	)
	modelConfig := governanceapp.NewModelConfigService(store, authorizer, clock)
	governanceapp.WithModelConfig(modelConfig)(authoring)
	governanceapp.WithGeneration(governanceapp.NewGenerationService(
		store, modelConfig,
		governanceapp.NewAgentRunService(store, clock),
		authoring, authorizer, clock,
	))(authoring)
	workbench := workbenchapp.NewService(store, authorizer, workbenchapp.ClockFunc(time.Now))
	prodService := governanceapp.NewProductionService(store)

	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	return httpapi.NewHandler(
		application.NewSystemService(
			application.ReadinessProbeFunc(func(context.Context) error { return nil }),
			domain.SystemInfo{APIVersion: "v1", SchemaVersion: "0.4.0", BuildVersion: "test-build"},
		),
		slog.New(slog.NewTextHandler(os.Stderr, nil)),
		provider.Tracer("governance-full-integration"),
		httpapi.WithCatalog(catalog),
		httpapi.WithGovernance(authoring),
		httpapi.WithWorkbench(workbench),
		httpapi.WithProduction(prodService),
		httpapi.WithAuthorization(authorizer),
	)
}

func TestProductionRelease_FullLifecycleAndGuards(t *testing.T) {
	fixture := newFixture(t)
	handler := newFullGovernanceHandler(t, fixture)
	workspace := createWorkspace(t, fixture.pool, "prod-release-workspace")

	authorID, err := identity.NewPrincipalID()
	if err != nil {
		t.Fatal(err)
	}
	reviewerID, err := identity.NewPrincipalID()
	if err != nil {
		t.Fatal(err)
	}
	publisherID, err := identity.NewPrincipalID()
	if err != nil {
		t.Fatal(err)
	}
	candID, err := identity.NewSemanticCandidateID()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for _, p := range []struct {
		id   identity.PrincipalID
		name string
		role string
	}{
		{id: authorID, name: "Author", role: "workspace_admin"},
		{id: reviewerID, name: "Reviewer", role: "reviewer"},
		{id: publisherID, name: "Publisher", role: "publisher"},
	} {
		if _, err := fixture.store.CreatePrincipal(ctx, authorizationdomain.Principal{
			ID: p.id, WorkspaceID: workspace, Kind: authorizationdomain.PrincipalHuman,
			DisplayName: p.name, Status: authorizationdomain.PrincipalActive,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.store.CreateRoleBinding(ctx, authorizationdomain.RoleBinding{
			ID: mustID(t, identity.NewBindingID), PrincipalID: p.id, RoleID: p.role,
			ScopeType: authorizationdomain.ScopeWorkspace, ScopeID: workspace.UUID(),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.store.CreateRoleBinding(ctx, authorizationdomain.RoleBinding{
			ID: mustID(t, identity.NewBindingID), PrincipalID: p.id, RoleID: "auditor", ScopeType: authorizationdomain.ScopeWorkspace, ScopeID: workspace.UUID(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	wspStr := workspace.String()
	collectionPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", wspStr)

	// Step 1: Create multi-target operation draft
	createPayload := fmt.Sprintf(`{
		"input": {
			"candidates": [
				{
					"candidateId": "%s",
					"digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
					"primaryTargetKey": "net_revenue",
					"targetKeys": ["net_revenue", "gross_revenue"]
				}
			]
		},
		"targets": [
			{
				"intent": "create",
				"kind": "semantic_asset",
				"localKey": "net_revenue",
				"identityKey": "finance.net_revenue",
				"title": "Net Revenue",
				"content": {
					"address": "finance.net_revenue",
					"assetType": "metric",
					"displayName": "Net Revenue",
					"definition": "Revenue after refunds and chargebacks"
				}
			},
			{
				"intent": "create",
				"kind": "semantic_asset",
				"localKey": "gross_revenue",
				"identityKey": "finance.gross_revenue",
				"title": "Gross Revenue",
				"content": {
					"address": "finance.gross_revenue",
					"assetType": "metric",
					"displayName": "Gross Revenue",
					"definition": "Total booked revenue before discounts"
				}
			}
		]
	}`, candID.String())

	createPayload = productionPayloadInput(t, createPayload, productionFixtureInput(t, fixture, workspace, candID, true), authorID)
	createRec := sendProdRequest(handler, http.MethodPost, collectionPath, authorID.String(), "key-rel-create-001", createPayload)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on operation creation, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var createResult map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResult); err != nil {
		t.Fatal(err)
	}
	opID := createResult["operationId"].(string)
	setDigest := createResult["setDigest"].(string)
	proposals := createResult["proposalIds"].([]any)
	if len(proposals) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(proposals))
	}
	prop1ID := proposals[0].(string)
	prop2ID := proposals[1].(string)

	// Step 2: Legacy endpoint guard: attempt submitting a proposal directly via legacy proposal submit endpoint
	legacySubmitPath := fmt.Sprintf("/api/v1/workspaces/%s/governance/proposals/%s/submit", wspStr, prop1ID)
	legacySubmitRec := sendProdRequest(handler, http.MethodPost, legacySubmitPath, authorID.String(), "key-legacy-sub-001", "")
	if legacySubmitRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict from legacy proposal submit, got %d: %s", legacySubmitRec.Code, legacySubmitRec.Body.String())
	}
	if !strings.Contains(legacySubmitRec.Body.String(), "PRODUCTION_SET_REQUIRED") {
		t.Fatalf("expected PRODUCTION_SET_REQUIRED error code, got %s", legacySubmitRec.Body.String())
	}

	// Step 3: Production submit with baseline/version conflict check
	submitPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s/submit", wspStr, opID)

	badDigestSubmitPayload := `{"expectedVersion": 1, "setDigest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"}`
	badDigestRec := sendProdRequest(handler, http.MethodPost, submitPath, authorID.String(), "key-rel-sub-bad-digest", badDigestSubmitPayload)
	if badDigestRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on bad setDigest, got %d: %s", badDigestRec.Code, badDigestRec.Body.String())
	}

	badVersionSubmitPayload := fmt.Sprintf(`{"expectedVersion": 2, "setDigest": %q}`, setDigest)
	badVersionRec := sendProdRequest(handler, http.MethodPost, submitPath, authorID.String(), "key-rel-sub-bad-ver", badVersionSubmitPayload)
	if badVersionRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on bad expectedVersion, got %d: %s", badVersionRec.Code, badVersionRec.Body.String())
	}

	// Step 4: Successful production submit
	confirmProductionFixtureRules(t, fixture, handler, workspace, authorID, createResult)
	submitPayload := fmt.Sprintf(`{"expectedVersion": 1, "setDigest": %q}`, setDigest)
	submitRec := sendProdRequest(handler, http.MethodPost, submitPath, authorID.String(), "key-rel-sub-001", submitPayload)
	if submitRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted on submit, got %d: %s", submitRec.Code, submitRec.Body.String())
	}

	var submitResult map[string]any
	if err := json.Unmarshal(submitRec.Body.Bytes(), &submitResult); err != nil {
		t.Fatal(err)
	}
	if submitResult["outcome"] != "submitted" {
		t.Errorf("expected outcome=submitted, got %v", submitResult["outcome"])
	}
	if submitResult["validationAttemptNo"].(float64) != 1 {
		t.Errorf("expected validationAttemptNo=1, got %v", submitResult["validationAttemptNo"])
	}

	// Submit replay idempotency
	submitReplayRec := sendProdRequest(handler, http.MethodPost, submitPath, authorID.String(), "key-rel-sub-001", submitPayload)
	if submitReplayRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on submit replay, got %d: %s", submitReplayRec.Code, submitReplayRec.Body.String())
	}
	var submitReplayResult map[string]any
	_ = json.Unmarshal(submitReplayRec.Body.Bytes(), &submitReplayResult)
	if submitReplayResult["replayed"] != true {
		t.Errorf("expected replayed=true on submit replay")
	}

	// Step 5: List validations
	validationsPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s/validations?version=1", wspStr, opID)
	valListRec := sendProdRequest(handler, http.MethodGet, validationsPath, authorID.String(), "", "")
	if valListRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on list validations, got %d: %s", valListRec.Code, valListRec.Body.String())
	}
	var valListPage map[string]any
	if err := json.Unmarshal(valListRec.Body.Bytes(), &valListPage); err != nil {
		t.Fatal(err)
	}
	valItems := valListPage["items"].([]any)
	if len(valItems) != 1 {
		t.Fatalf("expected 1 attempt in list, got %d", len(valItems))
	}
	attempt1 := valItems[0].(map[string]any)
	if attempt1["attemptNo"].(float64) != 1 {
		t.Errorf("expected attemptNo=1, got %v", attempt1["attemptNo"])
	}
	runAttempt := func(attempt int) string {
		job := jobs.Job{WorkspaceID: workspace, Attempt: 1, MaxAttempts: 3}
		if err := fixture.pool.QueryRow(ctx, `SELECT payload,trace_id FROM jobs WHERE workspace_id=$1 AND job_type='governance.proposal.validate' AND payload->>'operationId'=$2 AND (payload->>'attempt')::int=$3`, workspace.UUID(), opID, attempt).Scan(&job.Payload, &job.TraceID); err != nil {
			t.Fatal(err)
		}
		if err := runProductionValidationJob(t, fixture, job); err != nil {
			t.Fatal(err)
		}
		var digest string
		if err := fixture.pool.QueryRow(ctx, `SELECT validation_digest FROM production_validation_attempts WHERE workspace_id=$1 AND operation_id=$2 AND production_version=1 AND attempt_no=$3`, workspace.UUID(), mustParseProductionOperationID(t, opID).UUID(), attempt).Scan(&digest); err != nil {
			t.Fatal(err)
		}
		return digest
	}
	runAttempt(1)

	// Step 6: Trigger revalidation (Attempt 2) with CAS check
	valPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s/validations", wspStr, opID)

	// Wrong previousAttemptNo
	badCasPayload := fmt.Sprintf(`{"expectedVersion": 1, "setDigest": %q, "previousAttemptNo": 99, "reason": "recheck"}`, setDigest)
	badCasRec := sendProdRequest(handler, http.MethodPost, valPath, authorID.String(), "key-val-bad-cas", badCasPayload)
	if badCasRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on invalid previousAttemptNo, got %d: %s", badCasRec.Code, badCasRec.Body.String())
	}

	// Correct previousAttemptNo -> Attempt 2
	valPayload := fmt.Sprintf(`{"expectedVersion": 1, "setDigest": %q, "previousAttemptNo": 1, "reason": "recheck requested"}`, setDigest)
	valRec := sendProdRequest(handler, http.MethodPost, valPath, authorID.String(), "key-val-002", valPayload)
	if valRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted on trigger validate, got %d: %s", valRec.Code, valRec.Body.String())
	}
	var valResult map[string]any
	_ = json.Unmarshal(valRec.Body.Bytes(), &valResult)
	if valResult["validationAttemptNo"].(float64) != 2 {
		t.Fatalf("expected validationAttemptNo=2, got %v", valResult["validationAttemptNo"])
	}

	// Validate replay idempotency
	valReplayRec := sendProdRequest(handler, http.MethodPost, valPath, authorID.String(), "key-val-002", valPayload)
	if valReplayRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on validate replay, got %d: %s", valReplayRec.Code, valReplayRec.Body.String())
	}

	// Step 7: Segregation of Duties (SoD) Review test
	reviewsPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s/reviews", wspStr, opID)

	valDigestAttempt2 := runAttempt(2)

	// Author tries to review -> 403 SOD_CONFLICT
	authorReviewPayload := fmt.Sprintf(`{
		"expectedVersion": 1,
		"setDigest": %q,
		"validation": {
			"attemptNo": 2,
			"validationDigest": %q
		},
		"proposalIds": [%q, %q],
		"decision": "approve",
		"note": "self approval"
	}`, setDigest, valDigestAttempt2, prop1ID, prop2ID)

	authorRevRec := sendProdRequest(handler, http.MethodPost, reviewsPath, authorID.String(), "key-rev-author", authorReviewPayload)
	if authorRevRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for author review, got %d: %s", authorRevRec.Code, authorRevRec.Body.String())
	}
	if !strings.Contains(authorRevRec.Body.String(), "SOD_CONFLICT") {
		t.Fatalf("expected SOD_CONFLICT error code, got %s", authorRevRec.Body.String())
	}

	// Legacy proposal review guard
	legacyReviewPath := fmt.Sprintf("/api/v1/workspaces/%s/governance/proposals/%s/reviews", wspStr, prop1ID)
	legacyRevPayload := `{"decision": "approve", "reason": "legacy review"}`
	legacyRevRec := sendProdRequest(handler, http.MethodPost, legacyReviewPath, reviewerID.String(), "key-legacy-rev-001", legacyRevPayload)
	if legacyRevRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict from legacy proposal review, got %d: %s", legacyRevRec.Code, legacyRevRec.Body.String())
	}
	if !strings.Contains(legacyRevRec.Body.String(), "PRODUCTION_SET_REQUIRED") {
		t.Fatalf("expected PRODUCTION_SET_REQUIRED on legacy proposal review, got %s", legacyRevRec.Body.String())
	}

	// Independent Reviewer approves
	reviewerPayload := fmt.Sprintf(`{
		"expectedVersion": 1,
		"setDigest": %q,
		"validation": {
			"attemptNo": 2,
			"validationDigest": %q
		},
		"proposalIds": [%q, %q],
		"decision": "approve",
		"note": "approved by independent reviewer"
	}`, setDigest, valDigestAttempt2, prop1ID, prop2ID)

	reviewerRevRec := sendProdRequest(handler, http.MethodPost, reviewsPath, reviewerID.String(), "key-rev-reviewer-001", reviewerPayload)
	if reviewerRevRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on reviewer approve, got %d: %s", reviewerRevRec.Code, reviewerRevRec.Body.String())
	}
	var revResult map[string]any
	_ = json.Unmarshal(reviewerRevRec.Body.Bytes(), &revResult)
	if revResult["outcome"] != "reviewed" {
		t.Errorf("expected outcome=reviewed, got %v", revResult["outcome"])
	}

	// Review replay idempotency
	reviewerReplayRec := sendProdRequest(handler, http.MethodPost, reviewsPath, reviewerID.String(), "key-rev-reviewer-001", reviewerPayload)
	if reviewerReplayRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on review replay, got %d: %s", reviewerReplayRec.Code, reviewerReplayRec.Body.String())
	}

	// Step 8: Legacy publish guard
	legacyPubPath := fmt.Sprintf("/api/v1/workspaces/%s/governance/releases", wspStr)
	legacyPubPayload := fmt.Sprintf(`{"proposalId": %q}`, prop1ID)
	legacyPubRec := sendProdRequest(handler, http.MethodPost, legacyPubPath, publisherID.String(), "key-legacy-pub-001", legacyPubPayload)
	if legacyPubRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on legacy publish, got %d: %s", legacyPubRec.Code, legacyPubRec.Body.String())
	}
	if !strings.Contains(legacyPubRec.Body.String(), "PRODUCTION_SET_REQUIRED") {
		t.Fatalf("expected PRODUCTION_SET_REQUIRED on legacy publish, got %s", legacyPubRec.Body.String())
	}

	// Step 9: Publish tests
	publishPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s/publish", wspStr, opID)

	// Author tries to publish -> 403 SOD_CONFLICT
	authorPublishPayload := fmt.Sprintf(`{
		"expectedVersion": 1,
		"setDigest": %q,
		"validation": {
			"attemptNo": 2,
			"validationDigest": %q
		},
		"expectedHead": {
			"presence": "absent"
		}
	}`, setDigest, valDigestAttempt2)
	authorPubRec := sendProdRequest(handler, http.MethodPost, publishPath, authorID.String(), "key-pub-author", authorPublishPayload)
	if authorPubRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for author publish, got %d: %s", authorPubRec.Code, authorPubRec.Body.String())
	}
	if !strings.Contains(authorPubRec.Body.String(), "SOD_CONFLICT") {
		t.Fatalf("expected SOD_CONFLICT on author publish, got %s", authorPubRec.Body.String())
	}

	// Wrong expectedHead (present instead of absent) -> 409 HEAD_CONFLICT
	fakeRelID := mustID(t, identity.NewReleaseID)
	badHeadPayload := fmt.Sprintf(`{
		"expectedVersion": 1,
		"setDigest": %q,
		"validation": {
			"attemptNo": 2,
			"validationDigest": %q
		},
		"expectedHead": {
			"presence": "present",
			"releaseId": %q,
			"manifestDigest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"
		}
	}`, setDigest, valDigestAttempt2, fakeRelID.String())
	badHeadRec := sendProdRequest(handler, http.MethodPost, publishPath, publisherID.String(), "key-pub-bad-head", badHeadPayload)
	if badHeadRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on bad expectedHead, got %d: %s", badHeadRec.Code, badHeadRec.Body.String())
	}

	// Successful Publish with independent Publisher
	publishPayload := fmt.Sprintf(`{
		"expectedVersion": 1,
		"setDigest": %q,
		"validation": {
			"attemptNo": 2,
			"validationDigest": %q
		},
		"expectedHead": {
			"presence": "absent"
		}
	}`, setDigest, valDigestAttempt2)
	pubRec := sendProdRequest(handler, http.MethodPost, publishPath, publisherID.String(), "key-pub-001", publishPayload)
	if pubRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on publish, got %d: %s", pubRec.Code, pubRec.Body.String())
	}

	location := pubRec.Header().Get("Location")
	if !strings.Contains(location, "/production-releases/rls_") {
		t.Fatalf("expected Location pointing to production release, got %q", location)
	}

	var pubResult map[string]any
	if err := json.Unmarshal(pubRec.Body.Bytes(), &pubResult); err != nil {
		t.Fatal(err)
	}
	releaseID := pubResult["releaseId"].(string)
	manifestDigest := pubResult["manifestDigest"].(string)
	if !strings.HasPrefix(releaseID, "rls_") {
		t.Fatalf("expected rls_ prefix, got %q", releaseID)
	}

	// Publish replay idempotency
	pubReplayRec := sendProdRequest(handler, http.MethodPost, publishPath, publisherID.String(), "key-pub-001", publishPayload)
	if pubReplayRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on publish replay, got %d: %s", pubReplayRec.Code, pubReplayRec.Body.String())
	}
	var pubReplayResult map[string]any
	_ = json.Unmarshal(pubReplayRec.Body.Bytes(), &pubReplayResult)
	if pubReplayResult["replayed"] != true {
		t.Errorf("expected replayed=true on publish replay")
	}

	// Step 10: Verify database records after atomic publish
	relID, err := identity.ParseReleaseID(releaseID)
	if err != nil {
		t.Fatal(err)
	}
	relUUID, err := uuid.Parse(relID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	var rollbackDepth int
	var rootUUID [16]byte
	err = fixture.pool.QueryRow(context.Background(), `
		SELECT production_rollback_depth, production_root_release_id FROM releases WHERE id = $1
	`, relUUID).Scan(&rollbackDepth, &rootUUID)
	if err != nil {
		t.Fatalf("failed to query releases protection extension: %v", err)
	}
	if rollbackDepth != 0 {
		t.Errorf("expected rollback_depth=0 on first publish, got %d", rollbackDepth)
	}
	rootReleaseID, _ := identity.ReleaseIDFromUUIDBytes(rootUUID)
	if rootReleaseID.String() != releaseID {
		t.Errorf("expected root_release_id=%s, got %s", releaseID, rootReleaseID.String())
	}

	// Check before_pins in database
	var absentCount int
	err = fixture.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM production_release_before_pins WHERE release_id = $1 AND presence = 'absent'
	`, relUUID).Scan(&absentCount)
	if err != nil {
		t.Fatalf("failed to query before_pins: %v", err)
	}
	if absentCount != 2 {
		t.Errorf("expected 2 absent before_pins, got %d", absentCount)
	}

	// Step 11: Verify Catalog projection after atomic publish
	catalogPath := fmt.Sprintf("/api/v1/workspaces/%s/catalog/assets?address=finance.net_revenue", wspStr)
	catalogRec := sendProdRequest(handler, http.MethodGet, catalogPath, authorID.String(), "", "")
	if catalogRec.Code == http.StatusOK {
		if !strings.Contains(catalogRec.Body.String(), "finance.net_revenue") {
			t.Errorf("expected catalog assets to contain finance.net_revenue, got %s", catalogRec.Body.String())
		}
	}

	// Step 12: Legacy rollback guard
	legacyRollbackPath := fmt.Sprintf("/api/v1/workspaces/%s/governance/releases/%s/rollback", wspStr, releaseID)
	legacyRbRec := sendProdRequest(handler, http.MethodPost, legacyRollbackPath, publisherID.String(), "key-legacy-rb-001", "")
	if legacyRbRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on legacy rollback of protected release, got %d: %s", legacyRbRec.Code, legacyRbRec.Body.String())
	}
	if !strings.Contains(legacyRbRec.Body.String(), "PRODUCTION_SET_REQUIRED") {
		t.Fatalf("expected PRODUCTION_SET_REQUIRED on legacy rollback, got %s", legacyRbRec.Body.String())
	}

	// Step 13: Read production release
	prodReleasePath := fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s", wspStr, releaseID)
	getRelRec := sendProdRequest(handler, http.MethodGet, prodReleasePath, authorID.String(), "", "")
	if getRelRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on get production release, got %d: %s", getRelRec.Code, getRelRec.Body.String())
	}
	var prodRel map[string]any
	if err := json.Unmarshal(getRelRec.Body.Bytes(), &prodRel); err != nil {
		t.Fatal(err)
	}
	protection := prodRel["protection"].(map[string]any)
	if protection["rollbackDepth"].(float64) != 0 {
		t.Errorf("expected protection rollbackDepth=0, got %v", protection["rollbackDepth"])
	}
	beforePins := prodRel["beforePins"].([]any)
	if len(beforePins) != 2 {
		t.Errorf("expected 2 beforePins, got %d", len(beforePins))
	}

	// Step 14: Protected Rollback test
	rollbackPath := fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s/rollback", wspStr, releaseID)

	// Wrong expectedHead -> 409 HEAD_CONFLICT
	badHeadRbPayload := fmt.Sprintf(`{
		"expectedVersion": 1,
		"setDigest": %q,
		"expectedHead": {
			"presence": "present",
			"releaseId": %q,
			"manifestDigest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"
		},
		"reason": "bad head"
	}`, setDigest, releaseID)
	badHeadRbRec := sendProdRequest(handler, http.MethodPost, rollbackPath, publisherID.String(), "key-rb-bad-head", badHeadRbPayload)
	if badHeadRbRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on bad head rollback, got %d: %s", badHeadRbRec.Code, badHeadRbRec.Body.String())
	}

	// Correct expectedHead -> Successful protected rollback
	rollbackPayload := fmt.Sprintf(`{
		"expectedVersion": 1,
		"setDigest": %q,
		"expectedHead": {
			"presence": "present",
			"releaseId": %q,
			"manifestDigest": %q
		},
		"reason": "reverting newly created assets to absent"
	}`, setDigest, releaseID, manifestDigest)

	rbRec := sendProdRequest(handler, http.MethodPost, rollbackPath, publisherID.String(), "key-rb-001", rollbackPayload)
	if rbRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on rollback, got %d: %s", rbRec.Code, rbRec.Body.String())
	}

	var rbResult map[string]any
	if err := json.Unmarshal(rbRec.Body.Bytes(), &rbResult); err != nil {
		t.Fatal(err)
	}
	rollbackReleaseID := rbResult["releaseId"].(string)

	// Rollback replay idempotency
	rbReplayRec := sendProdRequest(handler, http.MethodPost, rollbackPath, publisherID.String(), "key-rb-001", rollbackPayload)
	if rbReplayRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on rollback replay, got %d: %s", rbReplayRec.Code, rbReplayRec.Body.String())
	}

	// Verify rollback release details
	getRbRelPath := fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s", wspStr, rollbackReleaseID)
	getRbRec := sendProdRequest(handler, http.MethodGet, getRbRelPath, authorID.String(), "", "")
	if getRbRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on get rollback release, got %d: %s", getRbRec.Code, getRbRec.Body.String())
	}
	var rbRel map[string]any
	if err := json.Unmarshal(getRbRec.Body.Bytes(), &rbRel); err != nil {
		t.Fatal(err)
	}
	rbProtection := rbRel["protection"].(map[string]any)
	if rbProtection["rollbackDepth"].(float64) != 1 {
		t.Errorf("expected rollback depth 1, got %v", rbProtection["rollbackDepth"])
	}
	if rbProtection["rootReleaseId"] != releaseID {
		t.Errorf("expected rootReleaseId=%s, got %v", releaseID, rbProtection["rootReleaseId"])
	}
	if rbProtection["rollbackParentReleaseId"] != releaseID {
		t.Errorf("expected rollbackParentReleaseId=%s, got %v", releaseID, rbProtection["rollbackParentReleaseId"])
	}
	rbAttribution := rbRel["attribution"].(map[string]any)
	if rbAttribution["role"] != "reverted" {
		t.Errorf("expected attribution role=reverted, got %v", rbAttribution["role"])
	}

	// Verify absence restored in afterManifest
	afterManifest := rbRel["afterManifest"].(map[string]any)
	afterAssets := afterManifest["assets"].([]any)
	if len(afterAssets) != 0 {
		t.Errorf("expected 0 assets after rolling back absent targets, got %d", len(afterAssets))
	}
}
