package governance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type policyDecisionResponse struct {
	RuleVersion        string   `json:"ruleVersion"`
	InputsDigest       string   `json:"inputsDigest"`
	RiskLevel          string   `json:"riskLevel"`
	Routing            string   `json:"routing"`
	MatchedRuleId      string   `json:"matchedRuleId"`
	ReasonCode         string   `json:"reasonCode"`
	Explanation        string   `json:"explanation"`
	MatchedInputFields []string `json:"matchedInputFields"`
	CreatedAt          string   `json:"createdAt"`
}

func readPolicyDecision(t *testing.T, environment *fixture, path, proposalID string) (int, policyDecisionResponse, string) {
	t.Helper()
	response := environment.request(t, http.MethodGet, path+"/"+proposalID+"/policy-decision", "", "")
	var decision policyDecisionResponse
	if response.Code == http.StatusOK {
		if err := json.Unmarshal(response.Body.Bytes(), &decision); err != nil {
			t.Fatalf("decode policy decision: %v\n%s", err, response.Body.String())
		}
	}
	return response.Code, decision, response.Body.String()
}

func countPolicyDecisions(t *testing.T, environment *fixture, proposalID string) int {
	t.Helper()
	proposalTyped, err := identity.ParseProposalID(proposalID)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := environment.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM policy_decisions WHERE proposal_id = $1`, proposalTyped.UUID()).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// submitValidatedProposal drives one proposal from draft through validation
// completion and returns the proposals path plus the wire proposal id.
func submitValidatedProposal(t *testing.T, environment *fixture, workspace identity.WorkspaceID, changeSet string) (string, string) {
	t.Helper()
	path := environment.proposalsPath(t, workspace)
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	created := environment.request(t, http.MethodPost, path, "",
		validatedProposalBody("semantic_asset", assetID.String(), baseRevisionID.String(), changeSet))
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
	return path, proposalID
}

func TestPolicyDecisionPersistedOnReviewAndReadableOverHTTP(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-policy-clean")
	changeSet := validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks")
	path, proposalID := submitValidatedProposal(t, environment, workspace, changeSet)

	if decisions := countPolicyDecisions(t, environment, proposalID); decisions != 1 {
		t.Fatalf("policy decision rows = %d, want exactly one", decisions)
	}
	code, decision, raw := readPolicyDecision(t, environment, path, proposalID)
	if code != http.StatusOK {
		t.Fatalf("policy-decision status = %d", code)
	}
	if decision.RuleVersion != governance.RiskRuleVersion {
		t.Fatalf("rule version = %q, want %q", decision.RuleVersion, governance.RiskRuleVersion)
	}
	if !strings.HasPrefix(decision.InputsDigest, "sha256:") || len(decision.InputsDigest) != len("sha256:")+64 {
		t.Fatalf("inputs digest = %q, want a sha256 content digest", decision.InputsDigest)
	}
	if decision.RiskLevel != string(governance.RiskLow) || decision.Routing != string(governance.RoutingBatch) {
		t.Fatalf("documentation change routed %s/%s, want low/batch", decision.RiskLevel, decision.Routing)
	}
	if decision.MatchedRuleId != "semlia.risk.v1/low-risk" || decision.ReasonCode != governance.ReasonRiskLowBatch {
		t.Fatalf("matched rule/reason = %q/%q, want the seeded low-risk rule", decision.MatchedRuleId, decision.ReasonCode)
	}
	if decision.Explanation == "" || decision.CreatedAt == "" {
		t.Fatalf("decision must carry the rule explanation and created_at: %+v", decision)
	}
	if strings.Contains(raw, workspace.UUID()) {
		t.Fatal("policy decision surface leaked a storage UUID")
	}

	var riskLevel string
	var linkedDecision string
	proposalTyped, err := identity.ParseProposalID(proposalID)
	if err != nil {
		t.Fatal(err)
	}
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT risk_level::text, policy_decision_id::text FROM proposals
		WHERE workspace_id = $1 AND id = $2`,
		workspace.UUID(), proposalTyped.UUID()).Scan(&riskLevel, &linkedDecision); err != nil {
		t.Fatal(err)
	}
	if riskLevel != string(governance.RiskLow) || linkedDecision == "" {
		t.Fatalf("proposal link = risk %q decision %q, want low and a linked decision", riskLevel, linkedDecision)
	}
}

func TestPolicyDecisionIsIdempotentAcrossRepeatedExecutions(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-policy-idempotent")
	changeSet := validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks")
	path, proposalID := submitValidatedProposal(t, environment, workspace, changeSet)
	_, firstDecision, _ := readPolicyDecision(t, environment, path, proposalID)

	// A repeated execution over the same in_review proposal (a crashed job
	// retried after the transition applied) must not create a second row.
	proposalTyped, err := identity.ParseProposalID(proposalID)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(governanceapp.ValidationJobPayload{ProposalID: proposalID, Attempt: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.store.EnqueueJob(context.Background(), jobs.EnqueueJobParams{
		ID: mustID(t, identity.NewRunID), WorkspaceID: workspace, Type: governanceapp.ValidationJobType,
		Payload: payload, MaxAttempts: 3, AvailableAt: time.Now().UTC(),
		IdempotencyKey: governanceapp.ValidationJobIdempotencyKey(proposalTyped, 2), TraceID: traceID,
	}); err != nil {
		t.Fatal(err)
	}
	environment.runValidationWorker(t)

	if decisions := countPolicyDecisions(t, environment, proposalID); decisions != 1 {
		t.Fatalf("policy decision rows after re-execution = %d, want exactly one", decisions)
	}
	_, secondDecision, _ := readPolicyDecision(t, environment, path, proposalID)
	if secondDecision.InputsDigest != firstDecision.InputsDigest ||
		secondDecision.MatchedRuleId != firstDecision.MatchedRuleId ||
		secondDecision.ReasonCode != firstDecision.ReasonCode {
		t.Fatalf("re-execution changed the decision: %+v vs %+v", firstDecision, secondDecision)
	}
}

func TestPolicyDecisionComputationChangeRoutesExpert(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-policy-computation")
	changeSet := validatedChangeItem("expression", "sum(refunds)", "gross_revenue - sum(refunds)")
	path, proposalID := submitValidatedProposal(t, environment, workspace, changeSet)

	code, decision, _ := readPolicyDecision(t, environment, path, proposalID)
	if code != http.StatusOK {
		t.Fatalf("policy-decision status = %d", code)
	}
	if decision.RiskLevel != string(governance.RiskMedium) || decision.Routing != string(governance.RoutingExpert) {
		t.Fatalf("computation change routed %s/%s, want medium/expert", decision.RiskLevel, decision.Routing)
	}
	if decision.MatchedRuleId != "semlia.risk.v1/computation" || decision.ReasonCode != governance.ReasonRiskComputationChange {
		t.Fatalf("matched rule = %q/%q, want the seeded computation rule", decision.MatchedRuleId, decision.ReasonCode)
	}
	found := false
	for _, field := range decision.MatchedInputFields {
		if field == "affectsComputation" {
			found = true
		}
	}
	if !found {
		t.Fatalf("matched input fields %v must name the category that drove the decision", decision.MatchedInputFields)
	}
}

func TestPolicyDecisionBlockersRouteExpertWithIdenticalDigestsAcrossWorkspaces(t *testing.T) {
	environment := newFixture(t)
	missingAsset := mustID(t, identity.NewAssetID)
	digests := map[string]string{}
	for _, slug := range []string{"governance-policy-blocker-a", "governance-policy-blocker-b"} {
		workspace := createWorkspace(t, environment.pool, slug)
		changeSet := validatedChangeItem("content.relatedAsset", "", missingAsset.String())
		path, proposalID := submitValidatedProposal(t, environment, workspace, changeSet)

		code, decision, _ := readPolicyDecision(t, environment, path, proposalID)
		if code != http.StatusOK {
			t.Fatalf("%s policy-decision status = %d", slug, code)
		}
		if decision.RiskLevel != string(governance.RiskHigh) || decision.Routing != string(governance.RoutingExpert) ||
			decision.ReasonCode != governance.ReasonRiskBlocker {
			t.Fatalf("%s blocker change routed %s/%s/%s, want high/expert/RISK_BLOCKER",
				slug, decision.RiskLevel, decision.Routing, decision.ReasonCode)
		}
		digests[slug] = decision.InputsDigest
	}
	if digests["governance-policy-blocker-a"] == "" || digests["governance-policy-blocker-a"] != digests["governance-policy-blocker-b"] {
		t.Fatalf("identical persisted state must produce identical inputs digests: %v", digests)
	}
}

func TestPolicyDecisionReadSurfaceReturns404WithoutDecision(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-policy-404")
	path := environment.proposalsPath(t, workspace)
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	changeSet := validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks")
	created := environment.request(t, http.MethodPost, path, "",
		validatedProposalBody("semantic_asset", assetID.String(), baseRevisionID.String(), changeSet))
	if created.Code != http.StatusCreated {
		t.Fatalf("proposal create status = %d", created.Code)
	}
	proposalID, _ := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)

	if code, _, _ := readPolicyDecision(t, environment, path, proposalID); code != http.StatusNotFound {
		t.Fatalf("pre-decision policy-decision status = %d, want 404", code)
	}
	unknown := mustID(t, identity.NewProposalID)
	if code, _, _ := readPolicyDecision(t, environment, path, unknown.String()); code != http.StatusNotFound {
		t.Fatalf("unknown proposal policy-decision status = %d, want 404", code)
	}
}

func TestPolicyRuleVocabularyCannotExpressAutoLane(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-policy-vocabulary")
	proposalID := mustID(t, identity.NewProposalID)
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO proposals (id, workspace_id, target_object_type, target_object_id, state, title, created_by, submitted_at, created_at, updated_at)
		VALUES ($1, $2, 'model_grain', $1, 'in_review', 'vocabulary probe', 'founder', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		proposalID.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO policy_rules (rule_id, rule_version, priority, match, outcome_risk_level, outcome_routing, outcome_reason_code, outcome_explanation)
		VALUES ('semlia.risk.v1/auto', '1.0', 1, '{}'::jsonb, 'low', 'auto', 'RISK_AUTO', 'deferred channel')`); err == nil {
		t.Fatal("policy_rules must not express the deferred auto routing lane")
	}
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO policy_decisions (id, workspace_id, proposal_id, rule_version, inputs, inputs_digest,
			matched_policy, risk_level, routing, reason_code, decided_at)
		VALUES (gen_random_uuid(), $1, $2, '1.0', '{}'::jsonb, 'sha256:' || repeat('a', 64),
			'semlia.risk.v1/auto', 'low', 'auto', 'RISK_AUTO', CURRENT_TIMESTAMP)`,
		workspace.UUID(), proposalID.UUID()); err == nil {
		t.Fatal("policy_decisions must not express the deferred auto routing lane")
	}
}

func TestSeededPolicyRulesMatchCanonicalTableAndReproduceDecisions(t *testing.T) {
	environment := newFixture(t)
	rows, err := environment.pool.Query(context.Background(), `
		SELECT rule_id, rule_version, priority, match::text, outcome_risk_level, outcome_routing,
			outcome_reason_code, outcome_explanation
		FROM policy_rules WHERE rule_version = $1 ORDER BY priority DESC, rule_id`, governance.RiskRuleVersion)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type seededRule struct {
		RuleID      string
		RuleVersion string
		Priority    int32
		Match       string
		RiskLevel   string
		Routing     string
		ReasonCode  string
		Explanation string
	}
	var seeded []seededRule
	for rows.Next() {
		var rule seededRule
		if err := rows.Scan(&rule.RuleID, &rule.RuleVersion, &rule.Priority, &rule.Match,
			&rule.RiskLevel, &rule.Routing, &rule.ReasonCode, &rule.Explanation); err != nil {
			t.Fatal(err)
		}
		seeded = append(seeded, rule)
	}
	canonical, err := governance.CanonicalPolicyRules(governance.RiskRuleVersion)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeded) != len(canonical) {
		t.Fatalf("seeded %d rules, canonical table has %d", len(seeded), len(canonical))
	}
	for index, rule := range canonical {
		row := seeded[index]
		rowMatch, err := governance.CanonicalJSON(json.RawMessage(row.Match))
		if err != nil {
			t.Fatal(err)
		}
		ruleMatch, err := governance.CanonicalJSON(rule.Match)
		if err != nil {
			t.Fatal(err)
		}
		if row.RuleID != rule.ID || row.RuleVersion != rule.RuleVersion || int(row.Priority) != rule.Priority ||
			string(rowMatch) != string(ruleMatch) || row.RiskLevel != string(rule.RiskLevel) ||
			row.Routing != string(rule.Routing) || row.ReasonCode != rule.ReasonCode || row.Explanation != rule.Explanation {
			t.Fatalf("seeded rule %d = %+v, want canonical %+v", index, row, rule)
		}
	}

	// The DB-loaded rule table reproduces the pure domain decisions for a
	// corpus of persisted-shape inputs.
	dbRules := make([]governance.PolicyRule, 0, len(seeded))
	for index := range canonical {
		dbRules = append(dbRules, governance.PolicyRule{
			ID: seeded[index].RuleID, RuleVersion: seeded[index].RuleVersion, Priority: int(seeded[index].Priority),
			Match: json.RawMessage(seeded[index].Match), RiskLevel: governance.RiskLevel(seeded[index].RiskLevel),
			Routing: governance.RoutingChannel(seeded[index].Routing), ReasonCode: seeded[index].ReasonCode,
			Explanation: seeded[index].Explanation,
		})
	}
	corpus := []governance.DecisionInputs{
		{AssetType: "business_term", TargetObjectType: "semantic_asset", AffectsDefinition: true, AuthorKind: "human"},
		{AssetType: "metric", TargetObjectType: "semantic_asset", AffectsComputation: true, BlockerCount: 1},
		{AssetType: "metric", TargetObjectType: "semantic_asset", AffectsComputation: true, ProductionEnvironment: true},
		{AssetType: "contract", TargetObjectType: "semantic_asset", AffectsContract: true, OwnerAssigned: "assigned"},
		{AssetType: governance.DecisionInputNotApplicable, TargetObjectType: "join_contract", AuthorKind: "agent"},
	}
	for _, inputs := range corpus {
		expected, err := governance.EvaluateRiskRule(governance.RiskRuleVersion, inputs)
		if err != nil {
			t.Fatalf("domain evaluation for %+v: %v", inputs, err)
		}
		actual, err := governance.EvaluatePolicyRules(dbRules, inputs)
		if err != nil {
			t.Fatalf("db-loaded evaluation for %+v: %v", inputs, err)
		}
		if expected != actual {
			t.Fatalf("db rules %+v diverge from domain %+v for inputs %+v", actual, expected, inputs)
		}
	}
}

func TestPolicyDecisionsAreAppendOnlyImmutable(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-policy-immutable")
	changeSet := validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks")
	_, proposalID := submitValidatedProposal(t, environment, workspace, changeSet)
	if decisions := countPolicyDecisions(t, environment, proposalID); decisions != 1 {
		t.Fatalf("policy decision rows = %d, want exactly one", decisions)
	}
	if _, err := environment.pool.Exec(context.Background(),
		`UPDATE policy_decisions SET risk_level = 'low' WHERE proposal_id = $1`, proposalID); err == nil {
		t.Fatal("policy decisions must not be updatable (recomputability §8.2)")
	}
	if _, err := environment.pool.Exec(context.Background(),
		`DELETE FROM policy_decisions WHERE proposal_id = $1`, proposalID); err == nil {
		t.Fatal("policy decisions must not be deletable")
	}
}
