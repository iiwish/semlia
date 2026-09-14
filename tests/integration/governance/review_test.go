package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func reviewBatchesPath(t *testing.T, workspace identity.WorkspaceID) string {
	t.Helper()
	return "/api/v1/workspaces/" + workspace.String() + "/governance/review-batches"
}

func reviewPath(t *testing.T, environment *fixture, workspace identity.WorkspaceID, proposalID string) string {
	t.Helper()
	t.Helper()
	return environment.proposalsPath(t, workspace) + "/" + proposalID + "/reviews"
}

func reviewBody(decision, reason string) string {
	return fmt.Sprintf(`{"decision":%q,"reason":%q}`, decision, reason)
}

func countReviews(t *testing.T, environment *fixture, proposalID string) int {
	t.Helper()
	proposalTyped, err := identity.ParseProposalID(proposalID)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := environment.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM reviews WHERE proposal_id = $1`, proposalTyped.UUID()).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func proposalDecidedAt(t *testing.T, environment *fixture, workspace identity.WorkspaceID, proposalID string) *time.Time {
	t.Helper()
	proposalTyped, err := identity.ParseProposalID(proposalID)
	if err != nil {
		t.Fatal(err)
	}
	var decidedAt *time.Time
	if err := environment.pool.QueryRow(context.Background(),
		`SELECT decided_at FROM proposals WHERE workspace_id = $1 AND id = $2`,
		workspace.UUID(), proposalTyped.UUID()).Scan(&decidedAt); err != nil {
		t.Fatal(err)
	}
	return decidedAt
}

func createPrincipalWithRoles(
	t *testing.T, environment *fixture, workspace identity.WorkspaceID, name string, roles []string,
) identity.PrincipalID {
	t.Helper()
	principalID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: principalID, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: name, Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		if _, err := environment.store.CreateRoleBinding(context.Background(), authorization.RoleBinding{
			ID: mustID(t, identity.NewBindingID), PrincipalID: principalID, RoleID: role,
			ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return principalID
}

func createReviewerPrincipal(t *testing.T, environment *fixture, workspace identity.WorkspaceID, name string) identity.PrincipalID {
	t.Helper()
	return createPrincipalWithRoles(t, environment, workspace, name, []string{"reviewer"})
}

func createAuthorWithReviewCapability(t *testing.T, environment *fixture, workspace identity.WorkspaceID, name string) identity.PrincipalID {
	t.Helper()
	return createPrincipalWithRoles(t, environment, workspace, name, []string{"asset_owner", "reviewer"})
}

func seedBlockerFinding(t *testing.T, environment *fixture, workspace identity.WorkspaceID, proposalID string) {
	t.Helper()
	proposalTyped, err := identity.ParseProposalID(proposalID)
	if err != nil {
		t.Fatal(err)
	}
	runID := mustID(t, identity.NewValidationRunID)
	now := time.Now().UTC()
	if _, err := environment.store.CreateValidationRun(context.Background(), governance.ValidationRun{
		ID: runID, WorkspaceID: workspace, ProposalID: proposalTyped,
		ValidatorID: "manual.soak", ValidatorVersion: "1.0.0", Status: governance.ValidationRunning,
		StartedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.store.CreateValidationResult(context.Background(), governance.ValidationResult{
		ID: mustID(t, identity.NewValidationResultID), WorkspaceID: workspace,
		ValidationRunID: runID, Severity: governance.SeverityBlocker, Code: "DEFINITION_BLOCKED",
		Message:     "blocker recorded after batch assembly",
		InputDigest: digestOf("blocked"), Details: json.RawMessage(`{}`),
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.store.FinishValidationRun(context.Background(), governanceapp.ValidationRunFinishCommand{
		WorkspaceID: workspace, RunID: runID, Status: governance.ValidationSucceeded,
		FinishedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

// seedJoinContractInWorkspace plants the minimal M1 physical graph inside an
// existing workspace and returns the join contract TypeID, so a second
// proposal cluster can target a governed object.
func seedJoinContractInWorkspace(t *testing.T, environment *fixture, workspace identity.WorkspaceID) string {
	t.Helper()
	ctx := context.Background()
	sourceID := mustID(t, identity.NewSourceConnectionID)
	if _, err := environment.pool.Exec(ctx, `
		INSERT INTO source_connections (id, workspace_id, adapter_kind, name, normalized_locator, status, metadata)
		VALUES ($1, $2, 'postgres', $3, $4, 'active', '{}'::jsonb)`,
		sourceID.UUID(), workspace.UUID(), "batch warehouse", "postgres://batch/catalog"); err != nil {
		t.Fatal(err)
	}
	sourceRevisionID := mustID(t, identity.NewSourceRevisionID)
	if _, err := environment.pool.Exec(ctx, `
		INSERT INTO source_revisions (id, workspace_id, source_connection_id, content_digest, adapter_version, observed_at)
		VALUES ($1, $2, $3, $4, 'postgres/1.0.0', CURRENT_TIMESTAMP)`,
		sourceRevisionID.UUID(), workspace.UUID(), sourceID.UUID(), digestOf("batch-source")); err != nil {
		t.Fatal(err)
	}
	dataset := func(externalKey, qualifiedName string) identity.PhysicalDatasetID {
		id := mustID(t, identity.NewPhysicalDatasetID)
		if _, err := environment.pool.Exec(ctx, `
			INSERT INTO physical_datasets (id, workspace_id, source_connection_id, external_key, qualified_name)
			VALUES ($1, $2, $3, $4, $5)`,
			id.UUID(), workspace.UUID(), sourceID.UUID(), externalKey, qualifiedName); err != nil {
			t.Fatal(err)
		}
		return id
	}
	leftDatasetID := dataset("batch.orders", "batch warehouse.public.orders")
	rightDatasetID := dataset("batch.customers", "batch warehouse.public.customers")
	field := func(datasetID identity.PhysicalDatasetID, externalKey string) identity.PhysicalFieldID {
		id := mustID(t, identity.NewPhysicalFieldID)
		if _, err := environment.pool.Exec(ctx, `
			INSERT INTO physical_fields (id, workspace_id, physical_dataset_id, external_key, name)
			VALUES ($1, $2, $3, $4, $4)`,
			id.UUID(), workspace.UUID(), datasetID.UUID(), externalKey); err != nil {
			t.Fatal(err)
		}
		return id
	}
	leftFieldID := field(leftDatasetID, "customer_id")
	rightFieldID := field(rightDatasetID, "customer_id")
	objectService := governanceapp.NewGovernedObjectService(
		environment.store, governanceapp.ClockFunc(func() time.Time { return time.Now().UTC() }),
	)
	contract, err := objectService.Create(ctx, governanceapp.CreateGovernedObjectRequest{
		WorkspaceID: workspace,
		Object: governance.GovernedObject{
			Type: governance.TargetJoinContract,
			JoinContract: &governance.JoinContract{
				WorkspaceID:   workspace,
				LeftDatasetID: leftDatasetID, RightDatasetID: rightDatasetID,
				LeftFieldRefs:  []identity.PhysicalFieldID{leftFieldID},
				RightFieldRefs: []identity.PhysicalFieldID{rightFieldID},
				JoinType:       governance.JoinLeft, Cardinality: governance.CardinalityManyToOne,
				JoinExpression: "orders.customer_id = customers.customer_id",
				Content:        json.RawMessage(`{}`),
			},
		},
		CreatedBy: "steward",
	})
	if err != nil {
		t.Fatal(err)
	}
	return contract.JoinContract.ID.String()
}

func decodeBatchID(t *testing.T, body []byte) string {
	t.Helper()
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("batch items = %d, want one: %s", len(page.Items), body)
	}
	return page.Items[0].ID
}

// submitBatchProposals drives `count` proposals of one cluster through the
// full journey against a single asset: create, submit, validation completion
// and the in_review assertion, returning the wire proposal ids in order.
func submitBatchProposals(
	t *testing.T, environment *fixture, workspace identity.WorkspaceID, changeSet string, count int,
) []string {
	t.Helper()
	assetID, revisionID := environment.createAsset(t, workspace)
	path := environment.proposalsPath(t, workspace)
	ids := make([]string, 0, count)
	for range count {
		ids = append(ids, submitProposalAgainstAsset(t, environment, workspace, path, assetID, revisionID, changeSet))
	}
	return ids
}

func submitProposalAgainstAsset(
	t *testing.T, environment *fixture, workspace identity.WorkspaceID, path string,
	assetID identity.AssetID, revisionID identity.RevisionID, changeSet string,
) string {
	t.Helper()
	created := environment.request(t, http.MethodPost, path, "",
		validatedProposalBody("semantic_asset", assetID.String(), revisionID.String(), changeSet))
	if created.Code != http.StatusCreated {
		t.Fatalf("proposal create status = %d, body = %s", created.Code, created.Body.String())
	}
	proposalID, _ := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
	submitted := environment.request(t, http.MethodPost, path+"/"+proposalID+"/submit", "", "")
	if submitted.Code != http.StatusOK {
		t.Fatalf("proposal submit status = %d, body = %s", submitted.Code, submitted.Body.String())
	}
	environment.runValidationWorker(t)
	assertProposalState(t, environment, workspace, path, proposalID, "in_review")
	return proposalID
}

func TestExpertApprovalRecordsImmutableReviewAndKeepsProposalInReview(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-review-approve")
	changeSet := validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks")
	path, proposalID := submitValidatedProposal(t, environment, workspace, changeSet)

	created := environment.request(t, http.MethodPost, reviewPath(t, environment, workspace, proposalID), authorizationapp.LocalUATReviewerPrincipalRef,
		reviewBody("approve", "Definition change verified against the revenue runbook"))
	if created.Code != http.StatusCreated {
		t.Fatalf("expert approve status = %d, body = %s", created.Code, created.Body.String())
	}
	var review struct {
		ID                  string `json:"id"`
		ProposalID          string `json:"proposalId"`
		ReviewerPrincipalID string `json:"reviewerPrincipalId"`
		Channel             string `json:"channel"`
		Decision            string `json:"decision"`
		Note                string `json:"note"`
		CreatedAt           string `json:"createdAt"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &review); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(review.ID, "rvw_") || review.ProposalID != proposalID ||
		!strings.HasPrefix(review.ReviewerPrincipalID, "prn_") {
		t.Fatalf("review record = %+v, want TypeIDs for review and reviewer", review)
	}
	if review.Channel != "expert" || review.Decision != "approved" ||
		review.Note != "Definition change verified against the revenue runbook" || review.CreatedAt == "" {
		t.Fatalf("review record = %+v", review)
	}
	listed := environment.request(t, http.MethodGet, reviewPath(t, environment, workspace, proposalID), "", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("review list status = %d, body = %s", listed.Code, listed.Body.String())
	}
	var page struct {
		Items []struct {
			ID         string `json:"id"`
			ProposalID string `json:"proposalId"`
			Decision   string `json:"decision"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != review.ID ||
		page.Items[0].ProposalID != proposalID || page.Items[0].Decision != "approved" {
		t.Fatalf("review page = %+v", page.Items)
	}
	assertProposalState(t, environment, workspace, path, proposalID, "in_review")
	if decidedAt := proposalDecidedAt(t, environment, workspace, proposalID); decidedAt != nil {
		t.Fatalf("approval must not transition the proposal: decided_at = %v", decidedAt)
	}

	duplicate := environment.request(t, http.MethodPost, reviewPath(t, environment, workspace, proposalID), authorizationapp.LocalUATReviewerPrincipalRef,
		reviewBody("approve", "second review of the same stage"))
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate review status = %d, want 409, body = %s", duplicate.Code, duplicate.Body.String())
	}
	assertJSONField(t, duplicate.Body.Bytes(), "code", "CONFLICT")
	if countReviews(t, environment, proposalID) != 1 {
		t.Fatal("duplicate review created a second row for the same decision stage")
	}
}

func TestExpertRejectionTransitionsProposalToRejected(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-review-reject")
	changeSet := validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks")
	path, proposalID := submitValidatedProposal(t, environment, workspace, changeSet)

	rejected := environment.request(t, http.MethodPost, reviewPath(t, environment, workspace, proposalID), authorizationapp.LocalUATReviewerPrincipalRef,
		reviewBody("reject", "The change conflicts with the finance definition of record"))
	if rejected.Code != http.StatusCreated {
		t.Fatalf("expert reject status = %d, body = %s", rejected.Code, rejected.Body.String())
	}
	assertProposalState(t, environment, workspace, path, proposalID, "rejected")
	if decidedAt := proposalDecidedAt(t, environment, workspace, proposalID); decidedAt == nil {
		t.Fatal("rejection must persist decided_at on the proposal")
	}
	if countReviews(t, environment, proposalID) != 1 {
		t.Fatal("rejection must record exactly one review row")
	}
	followUp := environment.request(t, http.MethodPost, reviewPath(t, environment, workspace, proposalID), authorizationapp.LocalUATReviewerPrincipalRef,
		reviewBody("approve", "too late"))
	if followUp.Code != http.StatusConflict {
		t.Fatalf("review of a rejected proposal status = %d, want 409", followUp.Code)
	}
}

func TestExpertReviewRequiresProposalReviewCapabilityAndAuditsDenial(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-review-authz")
	changeSet := validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks")
	_, proposalID := submitValidatedProposal(t, environment, workspace, changeSet)
	unbound := createReviewerPrincipal(t, environment, workspace, "unbound reviewer")
	if _, err := environment.pool.Exec(context.Background(), `
		DELETE FROM role_bindings WHERE principal_id = $1`, unbound.UUID()); err != nil {
		t.Fatal(err)
	}

	denied := environment.request(t, http.MethodPost, reviewPath(t, environment, workspace, proposalID), unbound.String(),
		reviewBody("approve", "not allowed"))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("unauthorized review status = %d, want 403", denied.Code)
	}
	assertJSONField(t, denied.Body.Bytes(), "code", "NO_MATCHING_GRANT")
	if countReviews(t, environment, proposalID) != 0 {
		t.Fatal("unauthorized review recorded a review row")
	}
	var decision, reason string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT decision, reason_code FROM authorization_events
		WHERE workspace_id = $1 AND principal_id = $2 AND action = 'proposal.review'`,
		workspace.UUID(), unbound.UUID()).Scan(&decision, &reason); err != nil {
		t.Fatalf("capability denial not audited: %v", err)
	}
	if decision != "deny" || reason != "NO_MATCHING_GRANT" {
		t.Fatalf("audited denial = %s/%s", decision, reason)
	}
}

func TestProposalAuthorCannotReviewTheirOwnProposal(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-review-sod")
	author := createAuthorWithReviewCapability(t, environment, workspace, "author reviewer")
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	path := environment.proposalsPath(t, workspace)

	body := fmt.Sprintf(
		`{"targetObjectType":"semantic_asset","targetObjectId":%q,"baseRevisionId":%q,"title":"Self review probe","changeSet":[%s],"createdBy":%q}`,
		assetID.String(), baseRevisionID.String(),
		validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks"),
		author.String())
	created := environment.request(t, http.MethodPost, path, author.String(), body)
	if created.Code != http.StatusCreated {
		t.Fatalf("author proposal create status = %d, body = %s", created.Code, created.Body.String())
	}
	assertJSONField(t, created.Body.Bytes(), "createdBy", author.String())
	proposalID, _ := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
	submitted := environment.request(t, http.MethodPost, path+"/"+proposalID+"/submit", author.String(), "")
	if submitted.Code != http.StatusOK {
		t.Fatalf("author proposal submit status = %d", submitted.Code)
	}
	environment.runValidationWorker(t)
	assertProposalState(t, environment, workspace, path, proposalID, "in_review")

	denied := environment.request(t, http.MethodPost, reviewPath(t, environment, workspace, proposalID), author.String(),
		reviewBody("approve", "reviewing my own change"))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("self review status = %d, want 403, body = %s", denied.Code, denied.Body.String())
	}
	assertJSONField(t, denied.Body.Bytes(), "code", "SEPARATION_OF_DUTY")
	var payload struct {
		Details struct {
			Conflict     string   `json:"conflict"`
			Scope        string   `json:"scope"`
			PolicySource string   `json:"policySource"`
			Recovery     []string `json:"recovery"`
		} `json:"details"`
	}
	if err := json.Unmarshal(denied.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Details.Conflict == "" || payload.Details.Scope != workspace.String() ||
		!strings.Contains(payload.Details.PolicySource, "FR-007") || len(payload.Details.Recovery) == 0 {
		t.Fatalf("FR-007 denial = %+v, must name conflict, scope, policy source and recovery", payload.Details)
	}
	assertProposalState(t, environment, workspace, path, proposalID, "in_review")
	if countReviews(t, environment, proposalID) != 0 {
		t.Fatal("self review recorded a review row")
	}
	var conflict, scope, source string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT payload->>'conflict', payload->>'scope', payload->>'policySource'
		FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'governance.review.denied'`,
		workspace.UUID()).Scan(&conflict, &scope, &source); err != nil {
		t.Fatalf("SoD denial not audited: %v", err)
	}
	if conflict == "" || scope != workspace.String() || !strings.Contains(source, "FR-007") {
		t.Fatalf("audited SoD denial = %s/%s/%s", conflict, scope, source)
	}

	independent := createReviewerPrincipal(t, environment, workspace, "independent reviewer")
	approved := environment.request(t, http.MethodPost, reviewPath(t, environment, workspace, proposalID), independent.String(),
		reviewBody("approve", "independent verification"))
	if approved.Code != http.StatusCreated {
		t.Fatalf("independent reviewer status = %d, body = %s", approved.Code, approved.Body.String())
	}
	if countReviews(t, environment, proposalID) != 1 {
		t.Fatal("independent reviewer review row missing")
	}
}

func TestBatchCreationClustersEligibleProposalsAndPersistsAuditRecord(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-batch-create")
	definition := validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks")
	path := environment.proposalsPath(t, workspace)
	assetID, revisionID := environment.createAsset(t, workspace)
	firstLow := submitProposalAgainstAsset(t, environment, workspace, path, assetID, revisionID, definition)
	secondLow := submitProposalAgainstAsset(t, environment, workspace, path, assetID, revisionID, definition)
	expertProposal := submitProposalAgainstAsset(t, environment, workspace, path, assetID, revisionID,
		validatedChangeItem("expression", "sum(refunds)", "gross_revenue - sum(refunds)"))

	created := environment.request(t, http.MethodPost, reviewBatchesPath(t, workspace), "", "")
	if created.Code != http.StatusOK {
		t.Fatalf("batch assembly status = %d, body = %s", created.Code, created.Body.String())
	}
	var page struct {
		Items []struct {
			ID           string `json:"id"`
			Status       string `json:"status"`
			GroupingRule struct {
				TargetObjectType string `json:"targetObjectType"`
				DiffCategory     string `json:"diffCategory"`
				MatchedRuleID    string `json:"matchedRuleId"`
			} `json:"groupingRule"`
			PolicyVersion string `json:"policyVersion"`
			MemberCount   int    `json:"memberCount"`
			CreatedBy     string `json:"createdBy"`
			DecidedAt     any    `json:"decidedAt"`
		} `json:"items"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("assembled batches = %d, want one cluster (expert proposal excluded): %s",
			len(page.Items), created.Body.String())
	}
	batch := page.Items[0]
	if !strings.HasPrefix(batch.ID, "rvb_") || batch.Status != "open" || batch.DecidedAt != nil {
		t.Fatalf("batch summary = %+v, want an open rvb_ batch without decision", batch)
	}
	if batch.GroupingRule.TargetObjectType != "semantic_asset" ||
		batch.GroupingRule.DiffCategory != "definition" ||
		batch.GroupingRule.MatchedRuleID != "semlia.risk.v1/low-risk" {
		t.Fatalf("grouping rule = %+v", batch.GroupingRule)
	}
	if batch.PolicyVersion != "1.0" || batch.MemberCount != 2 || !strings.HasPrefix(batch.CreatedBy, "prn_") {
		t.Fatalf("batch summary = %+v, want policy version, both members and the creator of record", batch)
	}

	detail := environment.request(t, http.MethodGet, reviewBatchesPath(t, workspace)+"/"+batch.ID, "", "")
	if detail.Code != http.StatusOK {
		t.Fatalf("batch detail status = %d", detail.Code)
	}
	var record struct {
		Members []struct {
			ProposalID  string `json:"proposalId"`
			AddedReason struct {
				MatchedRuleID string `json:"matchedRuleId"`
				RiskLevel     string `json:"riskLevel"`
				ReasonCode    string `json:"reasonCode"`
				RuleVersion   string `json:"ruleVersion"`
				InputsDigest  string `json:"inputsDigest"`
			} `json:"addedReason"`
			Sample   bool `json:"sample"`
			SplitOut bool `json:"splitOut"`
		} `json:"members"`
		Samples    []any `json:"samples"`
		Exclusions []any `json:"exclusions"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if len(record.Members) != 2 || len(record.Samples) != 2 || len(record.Exclusions) != 0 {
		t.Fatalf("batch record = %d members, %d samples, %d exclusions",
			len(record.Members), len(record.Samples), len(record.Exclusions))
	}
	memberProposals := map[string]bool{}
	for _, member := range record.Members {
		memberProposals[member.ProposalID] = true
		if !member.Sample || member.SplitOut {
			t.Fatalf("open batch member must be an unsplit sample: %+v", member)
		}
		if member.AddedReason.MatchedRuleID != "semlia.risk.v1/low-risk" ||
			member.AddedReason.RiskLevel != "low" || member.AddedReason.ReasonCode != "RISK_LOW_BATCH" ||
			member.AddedReason.RuleVersion != "1.0" || !strings.HasPrefix(member.AddedReason.InputsDigest, "sha256:") {
			t.Fatalf("added reason snapshot = %+v", member.AddedReason)
		}
	}
	if !memberProposals[firstLow] || !memberProposals[secondLow] || memberProposals[expertProposal] {
		t.Fatalf("membership %v must be exactly the batch-routed proposals", memberProposals)
	}
	if strings.Contains(detail.Body.String(), workspace.UUID()) {
		t.Fatal("review batch surface leaked a storage UUID")
	}

	batchTyped, err := identity.ParseReviewBatchID(batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	var groupingRule, policyVersion, status string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT grouping_rule::text, policy_version, status FROM review_batches WHERE id = $1`,
		batchTyped.UUID()).Scan(&groupingRule, &policyVersion, &status); err != nil {
		t.Fatal(err)
	}
	if status != "open" || policyVersion != "1.0" || !strings.Contains(groupingRule, "semlia.risk.v1/low-risk") {
		t.Fatalf("persisted batch = %s/%s/%s", groupingRule, policyVersion, status)
	}
	var memberRows, snapshotRows int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*), count(*) FILTER (WHERE added_reason ? 'inputsDigest')
		FROM review_batch_members WHERE review_batch_id = $1`,
		batchTyped.UUID()).Scan(&memberRows, &snapshotRows); err != nil {
		t.Fatal(err)
	}
	if memberRows != 2 || snapshotRows != 2 {
		t.Fatalf("persisted member snapshots = %d/%d", memberRows, snapshotRows)
	}
	var auditFacts int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'governance.review_batch.assembled'`,
		workspace.UUID()).Scan(&auditFacts); err != nil {
		t.Fatal(err)
	}
	if auditFacts != 1 {
		t.Fatalf("assembly audit facts = %d", auditFacts)
	}

	second := environment.request(t, http.MethodPost, reviewBatchesPath(t, workspace), "", "")
	if second.Code != http.StatusOK {
		t.Fatalf("second assembly status = %d", second.Code)
	}
	if !strings.Contains(second.Body.String(), `"items":[]`) {
		t.Fatalf("second assembly must return an empty page: %s", second.Body.String())
	}
}

func TestBatchConfirmAppliesDecisionAndAutoSplitsEscalatedMember(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-batch-confirm")
	definition := validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks")
	members := submitBatchProposals(t, environment, workspace, definition, 2)
	calmMember, escalatedMember := members[0], members[1]
	path := environment.proposalsPath(t, workspace)

	assembled := environment.request(t, http.MethodPost, reviewBatchesPath(t, workspace), "", "")
	if assembled.Code != http.StatusOK {
		t.Fatalf("batch assembly status = %d", assembled.Code)
	}
	batchID := decodeBatchID(t, assembled.Body.Bytes())
	seedBlockerFinding(t, environment, workspace, escalatedMember)

	confirmed := environment.request(t, http.MethodPost, reviewBatchesPath(t, workspace)+"/"+batchID+"/confirm",
		authorizationapp.LocalUATReviewerPrincipalRef, reviewBody("approve", "Representative diffs verified"))
	if confirmed.Code != http.StatusOK {
		t.Fatalf("batch confirm status = %d, body = %s", confirmed.Code, confirmed.Body.String())
	}
	var record struct {
		Status    string `json:"status"`
		DecidedBy string `json:"decidedBy"`
		DecidedAt string `json:"decidedAt"`
		MaxRisk   string `json:"maxRiskProposalId"`
		Members   []struct {
			ProposalID  string `json:"proposalId"`
			Decision    any    `json:"decision"`
			Sample      bool   `json:"sample"`
			SplitOut    bool   `json:"splitOut"`
			SplitReason any    `json:"splitReason"`
		} `json:"members"`
		Exclusions []struct {
			ProposalID  string `json:"proposalId"`
			SplitReason string `json:"splitReason"`
			Decision    any    `json:"decision"`
		} `json:"exclusions"`
		Samples []struct {
			ProposalID string `json:"proposalId"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(confirmed.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Status != "confirmed" || record.DecidedBy == "" || record.DecidedAt == "" {
		t.Fatalf("confirmed batch = %+v, want status, reviewer and decided_at", record)
	}
	if len(record.Members) != 2 || len(record.Exclusions) != 1 || len(record.Samples) != 1 {
		t.Fatalf("confirmed record = %d members, %d exclusions, %d samples",
			len(record.Members), len(record.Exclusions), len(record.Samples))
	}
	exclusion := record.Exclusions[0]
	if exclusion.ProposalID != escalatedMember || exclusion.Decision != nil ||
		!strings.HasPrefix(exclusion.SplitReason, "risk_escalated:high:RISK_BLOCKER") {
		t.Fatalf("exclusion = %+v, want the escalated member split with a reason", exclusion)
	}
	for _, member := range record.Members {
		if member.ProposalID == escalatedMember {
			if !member.SplitOut || member.Decision != nil {
				t.Fatalf("escalated member = %+v, want split out without a decision", member)
			}
		} else if member.ProposalID == calmMember {
			if member.SplitOut || member.Decision != "approved" || !member.Sample {
				t.Fatalf("confirmed member = %+v, want an approved sample", member)
			}
		}
	}
	if record.Samples[0].ProposalID != calmMember {
		t.Fatalf("samples = %+v, want only the confirmed membership represented", record.Samples)
	}
	if record.MaxRisk != calmMember {
		t.Fatalf("max risk member = %q, want the only remaining member", record.MaxRisk)
	}

	assertProposalState(t, environment, workspace, path, escalatedMember, "in_review")
	assertProposalState(t, environment, workspace, path, calmMember, "in_review")
	if decidedAt := proposalDecidedAt(t, environment, workspace, calmMember); decidedAt != nil {
		t.Fatalf("batch approval must not transition the member: decided_at = %v", decidedAt)
	}
	if countReviews(t, environment, escalatedMember) != 0 {
		t.Fatal("split member must not receive the batch decision")
	}
	if countReviews(t, environment, calmMember) != 1 {
		t.Fatal("confirmed member must own exactly one batch review")
	}

	double := environment.request(t, http.MethodPost, reviewBatchesPath(t, workspace)+"/"+batchID+"/confirm",
		"", reviewBody("approve", "again"))
	if double.Code != http.StatusConflict {
		t.Fatalf("double confirm status = %d, want 409", double.Code)
	}

	individual := environment.request(t, http.MethodPost, reviewPath(t, environment, workspace, escalatedMember), authorizationapp.LocalUATReviewerPrincipalRef,
		reviewBody("approve", "handled individually after the split"))
	if individual.Code != http.StatusCreated {
		t.Fatalf("individual review after split status = %d, body = %s", individual.Code, individual.Body.String())
	}
}

func TestBatchConfirmRejectionTransitionsMembersToRejected(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-batch-reject")
	definition := validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks")
	members := submitBatchProposals(t, environment, workspace, definition, 2)
	first, second := members[0], members[1]
	path := environment.proposalsPath(t, workspace)

	assembled := environment.request(t, http.MethodPost, reviewBatchesPath(t, workspace), "", "")
	batchID := decodeBatchID(t, assembled.Body.Bytes())
	rejected := environment.request(t, http.MethodPost, reviewBatchesPath(t, workspace)+"/"+batchID+"/confirm",
		authorizationapp.LocalUATReviewerPrincipalRef, reviewBody("reject", "The family of changes is withdrawn"))
	if rejected.Code != http.StatusOK {
		t.Fatalf("batch reject status = %d, body = %s", rejected.Code, rejected.Body.String())
	}
	assertJSONField(t, rejected.Body.Bytes(), "status", "rejected")
	for _, proposalID := range []string{first, second} {
		assertProposalState(t, environment, workspace, path, proposalID, "rejected")
		if decidedAt := proposalDecidedAt(t, environment, workspace, proposalID); decidedAt == nil {
			t.Fatalf("rejected member %s without decided_at", proposalID)
		}
		if countReviews(t, environment, proposalID) != 1 {
			t.Fatalf("rejected member %s without its immutable review row", proposalID)
		}
		proposalTyped, err := identity.ParseProposalID(proposalID)
		if err != nil {
			t.Fatal(err)
		}
		var attentionState string
		if err := environment.pool.QueryRow(context.Background(), `
			SELECT state FROM attention_items WHERE workspace_id=$1 AND dedupe_key=$2`,
			workspace.UUID(), "review:"+proposalTyped.UUID()).Scan(&attentionState); err != nil {
			t.Fatalf("load rejected member attention: %v", err)
		}
		if attentionState != "resolved" {
			t.Fatalf("rejected member %s attention state = %s", proposalID, attentionState)
		}
	}
	empty := environment.request(t, http.MethodGet, reviewBatchesPath(t, workspace), "", "")
	if !strings.Contains(empty.Body.String(), `"items":[]`) {
		t.Fatalf("decided batches must leave the open queue: %s", empty.Body.String())
	}
}

func TestBatchConfirmRefusesReviewerAuthoredMemberNamingTheConflict(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-batch-sod")
	author := createAuthorWithReviewCapability(t, environment, workspace, "batch author")
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	path := environment.proposalsPath(t, workspace)
	body := fmt.Sprintf(
		`{"targetObjectType":"semantic_asset","targetObjectId":%q,"baseRevisionId":%q,"title":"Authored batch member","changeSet":[%s],"createdBy":%q}`,
		assetID.String(), baseRevisionID.String(),
		validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks"),
		author.String())
	created := environment.request(t, http.MethodPost, path, author.String(), body)
	if created.Code != http.StatusCreated {
		t.Fatalf("author proposal create status = %d", created.Code)
	}
	proposalID, _ := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
	submitted := environment.request(t, http.MethodPost, path+"/"+proposalID+"/submit", author.String(), "")
	if submitted.Code != http.StatusOK {
		t.Fatalf("author proposal submit status = %d", submitted.Code)
	}
	environment.runValidationWorker(t)

	assembled := environment.request(t, http.MethodPost, reviewBatchesPath(t, workspace), "", "")
	batchID := decodeBatchID(t, assembled.Body.Bytes())

	refused := environment.request(t, http.MethodPost, reviewBatchesPath(t, workspace)+"/"+batchID+"/confirm",
		author.String(), reviewBody("approve", "confirming my own batch"))
	if refused.Code != http.StatusForbidden {
		t.Fatalf("author confirm status = %d, want 403, body = %s", refused.Code, refused.Body.String())
	}
	assertJSONField(t, refused.Body.Bytes(), "code", "SEPARATION_OF_DUTY")
	var payload struct {
		Details struct {
			ConflictingMembers []string `json:"conflictingMembers"`
			PolicySource       string   `json:"policySource"`
			Recovery           []string `json:"recovery"`
		} `json:"details"`
	}
	if err := json.Unmarshal(refused.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Details.ConflictingMembers) != 1 || payload.Details.ConflictingMembers[0] != proposalID ||
		!strings.Contains(payload.Details.PolicySource, "FR-007") || len(payload.Details.Recovery) == 0 {
		t.Fatalf("refusal = %+v, must name the conflicting member with recovery", payload.Details)
	}
	detail := environment.request(t, http.MethodGet, reviewBatchesPath(t, workspace)+"/"+batchID, "", "")
	assertJSONField(t, detail.Body.Bytes(), "status", "open")
	if countReviews(t, environment, proposalID) != 0 {
		t.Fatal("refused confirm recorded a review row")
	}

	confirmed := environment.request(t, http.MethodPost, reviewBatchesPath(t, workspace)+"/"+batchID+"/confirm",
		"", reviewBody("approve", "confirmed by an independent reviewer"))
	if confirmed.Code != http.StatusOK {
		t.Fatalf("independent confirm status = %d, body = %s", confirmed.Code, confirmed.Body.String())
	}
	assertJSONField(t, confirmed.Body.Bytes(), "status", "confirmed")
}

func TestReviewBatchEndpointsRequireProposalReviewCapability(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-batch-authz")
	unbound := createReviewerPrincipal(t, environment, workspace, "unbound operator")
	if _, err := environment.pool.Exec(context.Background(),
		`DELETE FROM role_bindings WHERE principal_id = $1`, unbound.UUID()); err != nil {
		t.Fatal(err)
	}

	created := environment.request(t, http.MethodPost, reviewBatchesPath(t, workspace), unbound.String(), "")
	if created.Code != http.StatusForbidden || !strings.Contains(created.Body.String(), "NO_MATCHING_GRANT") {
		t.Fatalf("unauthorized assembly status = %d, body = %s", created.Code, created.Body.String())
	}
	listed := environment.request(t, http.MethodGet, reviewBatchesPath(t, workspace), unbound.String(), "")
	if listed.Code != http.StatusForbidden {
		t.Fatalf("unauthorized list status = %d, want 403", listed.Code)
	}
	unknown := environment.request(t, http.MethodGet,
		reviewBatchesPath(t, workspace)+"/"+mustID(t, identity.NewReviewBatchID).String(), unbound.String(), "")
	if unknown.Code != http.StatusForbidden {
		t.Fatalf("unauthorized detail status = %d, want 403", unknown.Code)
	}
	var denies int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM authorization_events
		WHERE workspace_id = $1 AND principal_id = $2 AND action = 'proposal.review' AND decision = 'deny'`,
		workspace.UUID(), unbound.UUID()).Scan(&denies); err != nil {
		t.Fatal(err)
	}
	if denies < 3 {
		t.Fatalf("capability denials audited = %d, want one per refused command", denies)
	}
}

func TestOpenBatchListSeparatesClustersAndPagesWithKeysetCursor(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-batch-keyset")
	definition := validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks")
	submitBatchProposals(t, environment, workspace, definition, 1)
	contractID := seedJoinContractInWorkspace(t, environment, workspace)
	joinBody := fmt.Sprintf(
		`{"targetObjectType":"join_contract","targetObjectId":%q,"title":"Align join documentation","changeSet":[%s],"createdBy":"founder"}`,
		contractID, validatedChangeItem("definition", "orders.customer_id = customers.customer_id",
			"orders.customer_id = customers.customer_id (verified)"))
	path := environment.proposalsPath(t, workspace)
	created := environment.request(t, http.MethodPost, path, "", joinBody)
	if created.Code != http.StatusCreated {
		t.Fatalf("join proposal create status = %d, body = %s", created.Code, created.Body.String())
	}
	joinProposalID, _ := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
	submitted := environment.request(t, http.MethodPost, path+"/"+joinProposalID+"/submit", "", "")
	if submitted.Code != http.StatusOK {
		t.Fatalf("join proposal submit status = %d", submitted.Code)
	}
	environment.runValidationWorker(t)
	assertProposalState(t, environment, workspace, path, joinProposalID, "in_review")

	assembled := environment.request(t, http.MethodPost, reviewBatchesPath(t, workspace), "", "")
	if assembled.Code != http.StatusOK {
		t.Fatalf("assembly status = %d, body = %s", assembled.Code, assembled.Body.String())
	}
	var page struct {
		Items []struct {
			ID           string `json:"id"`
			GroupingRule struct {
				TargetObjectType string `json:"targetObjectType"`
			} `json:"groupingRule"`
		} `json:"items"`
	}
	if err := json.Unmarshal(assembled.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("clusters = %d, want one per target object type: %s", len(page.Items), assembled.Body.String())
	}
	targets := map[string]bool{}
	for _, item := range page.Items {
		targets[item.GroupingRule.TargetObjectType] = true
	}
	if !targets["semantic_asset"] || !targets["join_contract"] {
		t.Fatalf("cluster targets = %v", targets)
	}

	firstPage := environment.request(t, http.MethodGet, reviewBatchesPath(t, workspace)+"?limit=1", "", "")
	if firstPage.Code != http.StatusOK {
		t.Fatalf("first list page status = %d", firstPage.Code)
	}
	var list struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Page struct {
			NextCursor *string `json:"nextCursor"`
		} `json:"page"`
	}
	if err := json.Unmarshal(firstPage.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Page.NextCursor == nil {
		t.Fatalf("first list page = %+v, want one batch and a cursor", list)
	}
	secondPage := environment.request(t, http.MethodGet,
		reviewBatchesPath(t, workspace)+"?limit=1&cursor="+*list.Page.NextCursor, "", "")
	if secondPage.Code != http.StatusOK {
		t.Fatalf("second list page status = %d", secondPage.Code)
	}
	var second struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(secondPage.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID == list.Items[0].ID {
		t.Fatalf("second list page = %+v, want the other cluster", second)
	}
}
