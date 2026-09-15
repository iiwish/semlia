package governance_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/iiwish/semlia/internal/domain/governance"
)

// canonicalRulesV1 loads the canonical seeded rule table for the current
// version; every rule-table assertion runs against the same rows migration
// 000008 seeds into policy_rules.
func canonicalRulesV1(t *testing.T) []governance.PolicyRule {
	t.Helper()
	rules, err := governance.CanonicalPolicyRules(governance.RiskRuleVersion)
	if err != nil {
		t.Fatalf("canonical rule table: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("canonical rule table is empty")
	}
	return rules
}

func TestRuleTableRoutesHighRiskComputationWithBlockersToExpert(t *testing.T) {
	// Packet red scenario: a computation-changing diff carrying validation
	// blockers must route expert/high with a matched rule id and reason code.
	inputs := governance.DecisionInputs{
		AssetType: "metric", TargetObjectType: "semantic_asset",
		AffectsComputation: true, BlockerCount: 2, EvidenceComplete: true,
		OwnerAssigned: "assigned", AuthorKind: "agent",
	}
	decision, err := governance.EvaluatePolicyRules(canonicalRulesV1(t), inputs)
	if err != nil {
		t.Fatalf("evaluate rule table: %v", err)
	}
	if decision.RiskLevel != governance.RiskHigh || decision.Routing != governance.RoutingExpert {
		t.Fatalf("blocker computation change routed %s/%s, want high/expert", decision.RiskLevel, decision.Routing)
	}
	if decision.MatchedPolicy == "" || decision.MatchedPolicy == governance.MatchedRiskPolicy {
		t.Fatalf("matched policy = %q, want a seeded rule id", decision.MatchedPolicy)
	}
	if decision.ReasonCode != governance.ReasonRiskBlocker {
		t.Fatalf("reason code = %q, want %q", decision.ReasonCode, governance.ReasonRiskBlocker)
	}
}

func TestRuleTableRoutesCleanDocumentationChangeToBatch(t *testing.T) {
	inputs := governance.DecisionInputs{
		AssetType: "concept", TargetObjectType: "semantic_asset",
		AffectsDefinition: true, OwnerAssigned: "assigned", AuthorKind: "human",
	}
	decision, err := governance.EvaluatePolicyRules(canonicalRulesV1(t), inputs)
	if err != nil {
		t.Fatalf("evaluate rule table: %v", err)
	}
	if decision.RiskLevel != governance.RiskLow || decision.Routing != governance.RoutingBatch {
		t.Fatalf("documentation change routed %s/%s, want low/batch", decision.RiskLevel, decision.Routing)
	}
	if decision.ReasonCode != governance.ReasonRiskLowBatch {
		t.Fatalf("reason code = %q, want %q", decision.ReasonCode, governance.ReasonRiskLowBatch)
	}
}

func TestRuleTableEvaluatesInPriorityOrderFirstMatchWins(t *testing.T) {
	// A rule set with overlapping conditions must pick the highest-priority
	// (highest number) match, deterministically.
	rules := []governance.PolicyRule{
		{
			ID: "test.v1/low", RuleVersion: "1.0", Priority: 1, Match: json.RawMessage(`{}`),
			RiskLevel: governance.RiskLow, Routing: governance.RoutingBatch,
			ReasonCode: "TEST_LOW", Explanation: "catch-all",
		},
		{
			ID: "test.v1/high", RuleVersion: "1.0", Priority: 10,
			Match:     json.RawMessage(`{"field":"blockerCount","op":"gt","value":0}`),
			RiskLevel: governance.RiskHigh, Routing: governance.RoutingExpert,
			ReasonCode: "TEST_HIGH", Explanation: "blockers",
		},
	}
	decision, err := governance.EvaluatePolicyRules(rules, governance.DecisionInputs{
		AssetType: "metric", BlockerCount: 1,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.MatchedPolicy != "test.v1/high" || decision.ReasonCode != "TEST_HIGH" {
		t.Fatalf("first match = %+v, want test.v1/high", decision)
	}
}

func TestRuleTableNoMatchFailsSafeToExpert(t *testing.T) {
	// SSOT §8.2 degradation: a rule table whose rows all miss must fail safe
	// to the expert channel with a stable reason, never to batch.
	rules := []governance.PolicyRule{
		{
			ID: "test.v1/never", RuleVersion: "1.0", Priority: 10,
			Match:     json.RawMessage(`{"field":"blockerCount","op":"gt","value":999}`),
			RiskLevel: governance.RiskLow, Routing: governance.RoutingBatch,
			ReasonCode: "TEST_NEVER", Explanation: "unreachable",
		},
	}
	decision, err := governance.EvaluatePolicyRules(rules, governance.DecisionInputs{AssetType: "metric"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.Routing != governance.RoutingExpert || decision.RiskLevel != governance.RiskHigh {
		t.Fatalf("no-match fallback routed %s/%s, want high/expert", decision.RiskLevel, decision.Routing)
	}
	if decision.ReasonCode != governance.ReasonRiskPolicyUnmatched || decision.MatchedPolicy != governance.MatchedRiskPolicy {
		t.Fatalf("fallback decision = %+v, want stable unmatched reason and default policy", decision)
	}
}

func TestRuleTableCannotExpressAutoRouting(t *testing.T) {
	// Packet lock: the routing vocabulary is expert|batch only. A rule row
	// carrying the deferred auto channel must be structurally invalid so the
	// evaluator can never emit it.
	rules := []governance.PolicyRule{
		{
			ID: "test.v1/auto", RuleVersion: "1.0", Priority: 1, Match: json.RawMessage(`{}`),
			RiskLevel: governance.RiskLow, Routing: governance.RoutingChannel("auto"),
			ReasonCode: "TEST_AUTO", Explanation: "deferred channel",
		},
	}
	if _, err := governance.EvaluatePolicyRules(rules, governance.DecisionInputs{AssetType: "metric"}); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("auto routing rule error = %v, want ErrInvalidArgument", err)
	}
	for _, channel := range []governance.RoutingChannel{governance.RoutingExpert, governance.RoutingBatch} {
		if channel == "auto" || channel == "automatic" {
			t.Fatalf("routing vocabulary leaks the auto lane: %q", channel)
		}
	}
}

func TestRuleTableRejectsMalformedRulesAndConditions(t *testing.T) {
	cases := map[string]governance.PolicyRule{
		"unknown condition field": {
			ID: "test.v1/x", RuleVersion: "1.0", Priority: 1,
			Match:     json.RawMessage(`{"field":"nonsenseField","op":"eq","value":1}`),
			RiskLevel: governance.RiskHigh, Routing: governance.RoutingExpert,
			ReasonCode: "TEST_X", Explanation: "unknown field",
		},
		"unknown operator": {
			ID: "test.v1/x", RuleVersion: "1.0", Priority: 1,
			Match:     json.RawMessage(`{"field":"blockerCount","op":"regex","value":".*"}`),
			RiskLevel: governance.RiskHigh, Routing: governance.RoutingExpert,
			ReasonCode: "TEST_X", Explanation: "unknown op",
		},
		"unknown condition key": {
			ID: "test.v1/x", RuleVersion: "1.0", Priority: 1,
			Match:     json.RawMessage(`{"field":"blockerCount","op":"eq","value":0,"secret":true}`),
			RiskLevel: governance.RiskHigh, Routing: governance.RoutingExpert,
			ReasonCode: "TEST_X", Explanation: "unknown key",
		},
		"mixed condition modes": {
			ID: "test.v1/x", RuleVersion: "1.0", Priority: 1,
			Match:     json.RawMessage(`{"all":[{"field":"blockerCount","op":"gt","value":0}],"any":[{"field":"blockerCount","op":"eq","value":1}]}`),
			RiskLevel: governance.RiskHigh, Routing: governance.RoutingExpert,
			ReasonCode: "TEST_X", Explanation: "mixed",
		},
		"empty reason code": {
			ID: "test.v1/x", RuleVersion: "1.0", Priority: 1, Match: json.RawMessage(`{}`),
			RiskLevel: governance.RiskLow, Routing: governance.RoutingBatch,
			ReasonCode: "", Explanation: "no reason",
		},
		"bad risk level": {
			ID: "test.v1/x", RuleVersion: "1.0", Priority: 1, Match: json.RawMessage(`{}`),
			RiskLevel: governance.RiskLevel("catastrophic"), Routing: governance.RoutingBatch,
			ReasonCode: "TEST_X", Explanation: "bad level",
		},
	}
	for name, rule := range cases {
		if _, err := governance.EvaluatePolicyRules([]governance.PolicyRule{rule}, governance.DecisionInputs{AssetType: "metric"}); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("%s: error = %v, want ErrInvalidArgument", name, err)
		}
	}
}

func TestEvaluateRiskRuleMatchesCanonicalRuleTable(t *testing.T) {
	// T002 compatibility: the pure versioned entry point must produce exactly
	// the decisions of the canonical rule table it is now implemented over.
	corpus := []governance.DecisionInputs{
		{AssetType: "concept", BlockerCount: 0, EvidenceComplete: true},
		{AssetType: "metric", BlockerCount: 2, EvidenceComplete: true},
		{AssetType: "metric", AffectsComputation: true, ProductionEnvironment: true},
		{AssetType: "metric", AffectsComputation: true},
		{AssetType: "policy", AffectsAccess: true},
		{AssetType: "contract", AffectsContract: true},
		{AssetType: "metric", ProductionEnvironment: true},
		{AssetType: "concept", AffectsDefinition: true, OwnerAssigned: "unassigned", AuthorKind: "human"},
	}
	rules := canonicalRulesV1(t)
	for _, inputs := range corpus {
		expected, err := governance.EvaluatePolicyRules(rules, inputs)
		if err != nil {
			t.Fatalf("table evaluation for %+v: %v", inputs, err)
		}
		actual, err := governance.EvaluateRiskRule(governance.RiskRuleVersion, inputs)
		if err != nil {
			t.Fatalf("EvaluateRiskRule for %+v: %v", inputs, err)
		}
		if !reflect.DeepEqual(expected, actual) {
			t.Fatalf("table %+#v != entry point %+v for inputs %+v", expected, actual, inputs)
		}
	}
	if _, err := governance.EvaluatePolicyRules(canonicalRulesV1(t), governance.DecisionInputs{}); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("empty inputs error = %v, want ErrInvalidArgument", err)
	}
	if _, err := governance.CanonicalPolicyRules("99.0"); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("unknown canonical version error = %v, want ErrInvalidArgument", err)
	}
}

func TestRuleTableRecomputabilityAcrossVersions(t *testing.T) {
	// Recomputability (SSOT §8.2): identical inputs under one rule version
	// always reproduce the identical decision, and a new rule version may
	// decide differently without touching the v1.0 outcome.
	inputs := governance.DecisionInputs{
		AssetType: "metric", TargetObjectType: "semantic_asset",
		AffectsComputation: true, OwnerAssigned: "assigned", AuthorKind: "human",
	}
	first, err := governance.EvaluatePolicyRules(canonicalRulesV1(t), inputs)
	if err != nil {
		t.Fatalf("first evaluation: %v", err)
	}
	second, err := governance.EvaluatePolicyRules(canonicalRulesV1(t), inputs)
	if err != nil {
		t.Fatalf("second evaluation: %v", err)
	}
	if first != second {
		t.Fatalf("identical inputs diverged: %+v vs %+v", first, second)
	}

	stricter := []governance.PolicyRule{
		{
			ID: "semlia.risk.v1.1/computation-expert", RuleVersion: "1.1", Priority: 100,
			Match:     json.RawMessage(`{"field":"affectsComputation","op":"eq","value":true}`),
			RiskLevel: governance.RiskHigh, Routing: governance.RoutingExpert,
			ReasonCode: "RISK_COMPUTATION_CHANGE_STRICT", Explanation: "v1.1 tightens computation changes to high/expert",
		},
	}
	upgraded, err := governance.EvaluatePolicyRules(stricter, inputs)
	if err != nil {
		t.Fatalf("v1.1 evaluation: %v", err)
	}
	if upgraded.RiskLevel != governance.RiskHigh || upgraded.MatchedPolicy != "semlia.risk.v1.1/computation-expert" {
		t.Fatalf("v1.1 decision = %+v, want the stricter rule outcome", upgraded)
	}
	stable, err := governance.EvaluatePolicyRules(canonicalRulesV1(t), inputs)
	if err != nil {
		t.Fatalf("v1.0 re-evaluation: %v", err)
	}
	if stable != first {
		t.Fatalf("v1.0 outcome changed after v1.1 introduction: %+v vs %+v", stable, first)
	}
}

func TestDecisionInputsClosedSchemaRejectsInvalidEnumsAndCounts(t *testing.T) {
	valid := governance.DecisionInputs{AssetType: "metric", TargetObjectType: "semantic_asset"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid inputs rejected: %v", err)
	}
	cases := map[string]governance.DecisionInputs{
		"unknown target type":   {AssetType: "metric", TargetObjectType: "dashboard"},
		"unknown owner enum":    {AssetType: "metric", OwnerAssigned: "maybe"},
		"unknown author enum":   {AssetType: "metric", AuthorKind: "robot"},
		"negative blocker":      {AssetType: "metric", BlockerCount: -1},
		"negative warning":      {AssetType: "metric", WarningCount: -1},
		"negative info":         {AssetType: "metric", InfoCount: -2},
		"negative run count":    {AssetType: "metric", ValidationRunCount: -1},
		"negative failed count": {AssetType: "metric", ValidationFailedCount: -1},
		"negative evidence":     {AssetType: "metric", EvidenceLinkCount: -1},
		"empty asset type":      {},
	}
	for name, inputs := range cases {
		if err := inputs.Validate(); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("%s: error = %v, want ErrInvalidArgument", name, err)
		}
	}
	for _, target := range []string{
		"semantic_asset", "physical_binding", "model_grain", "entity_key", "join_contract",
	} {
		if err := (governance.DecisionInputs{AssetType: "metric", TargetObjectType: target}).Validate(); err != nil {
			t.Fatalf("target type %s must be expressible: %v", target, err)
		}
	}
}

func TestDecisionInputsRoundTripKeepsNotApplicableFields(t *testing.T) {
	// The M4 placeholder categories stay explicitly present in the closed
	// schema: nil marshals as JSON null and parses back, so a future rule
	// version can populate them without a schema-shape change.
	inputs := governance.DecisionInputs{
		AssetType: "metric", TargetObjectType: "semantic_asset",
		OwnerAssigned: governance.OwnerNotApplicable, AuthorKind: governance.AuthorKindHuman,
	}
	encoded, err := json.Marshal(inputs)
	if err != nil {
		t.Fatalf("marshal inputs: %v", err)
	}
	if !containsJSONKey(encoded, "consumerCount") || !containsJSONKey(encoded, "historicalAcceptRate") {
		t.Fatalf("not-applicable placeholders absent from encoded inputs: %s", encoded)
	}
	parsed, err := governance.ParseDecisionInputs(encoded)
	if err != nil {
		t.Fatalf("parse inputs: %v", err)
	}
	if parsed.ConsumerCount != nil || parsed.HistoricalAcceptRate != nil {
		t.Fatalf("placeholders must stay nil until M4: %+v", parsed)
	}
	if _, err := governance.ParseDecisionInputs(json.RawMessage(`{"assetType":"metric","nonsense":1}`)); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("unknown field error = %v, want ErrInvalidArgument", err)
	}
}

func containsJSONKey(encoded []byte, key string) bool {
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return false
	}
	_, ok := payload[key]
	return ok
}
