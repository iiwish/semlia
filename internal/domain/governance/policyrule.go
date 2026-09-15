package governance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"sync"
)

// reasonRuleVersionPattern mirrors the policy rule/decision rule_version
// database checks: "<major>.<minor>", never a free-form label.
var reasonRuleVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// policyReasonCodePattern mirrors the policy_decisions.reason_code database
// check: stable uppercase machine-readable reason identifiers.
var policyReasonCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)

// PolicyRule is one versioned row of the rule-table policy engine (decision
// D3: cel-go is not adopted; rules are data evaluated by this deterministic
// Go evaluator). Rules are immutable per rule version: a new rule semantics
// is a new version, never an in-place edit (SSOT §8.2).
type PolicyRule struct {
	ID          string          `json:"id"`
	RuleVersion string          `json:"ruleVersion"`
	Priority    int             `json:"priority"`
	Match       json.RawMessage `json:"match"`
	RiskLevel   RiskLevel       `json:"riskLevel"`
	Routing     RoutingChannel  `json:"routing"`
	ReasonCode  string          `json:"reasonCode"`
	Explanation string          `json:"explanation"`
}

// ruleCondition is the closed match-condition expression evaluated over the
// canonical DecisionInputs field map. Exactly one mode may be set:
//   - match-all: no field and no groups (`{}`) — always true,
//   - leaf:      {"field","op","value"},
//   - group:     {"all":[...]} conjunction or {"any":[...]} disjunction.
//
// Unknown keys, mixed modes and unknown operators are rejected, so a stored
// rule row cannot smuggle an open-ended expression language.
type ruleCondition struct {
	All   []ruleCondition `json:"all"`
	Any   []ruleCondition `json:"any"`
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

const (
	conditionOpEq    = "eq"
	conditionOpNe    = "ne"
	conditionOpGt    = "gt"
	conditionOpGte   = "gte"
	conditionOpLt    = "lt"
	conditionOpLte   = "lte"
	conditionOpIn    = "in"
	conditionOpNotIn = "notIn"
)

var conditionOperators = map[string]bool{
	conditionOpEq: true, conditionOpNe: true, conditionOpGt: true, conditionOpGte: true,
	conditionOpLt: true, conditionOpLte: true, conditionOpIn: true, conditionOpNotIn: true,
}

// decisionInputFieldNames are the only fields a rule condition may reference.
// They are derived from the closed DecisionInputs schema itself, so the
// condition vocabulary can never drift from the inputs the digest covers.
var decisionInputFieldNames = sync.OnceValue(func() map[string]bool {
	encoded, err := json.Marshal(DecisionInputs{})
	if err != nil {
		panic(fmt.Sprintf("marshal empty decision inputs: %v", err))
	}
	var names map[string]any
	if err := json.Unmarshal(encoded, &names); err != nil {
		panic(fmt.Sprintf("decode empty decision inputs: %v", err))
	}
	fields := make(map[string]bool, len(names))
	for name := range names {
		fields[name] = true
	}
	return fields
})

// ruleInputFields holds the field map of one evaluation, decoded with precise
// numbers so comparisons stay deterministic.
type ruleInputFields struct {
	values map[string]any
}

func newRuleInputFields(inputs DecisionInputs) (ruleInputFields, error) {
	encoded, err := json.Marshal(inputs)
	if err != nil {
		return ruleInputFields{}, fmt.Errorf("%w: encode decision inputs", ErrInvalidArgument)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var values map[string]any
	if err := decoder.Decode(&values); err != nil {
		return ruleInputFields{}, fmt.Errorf("%w: decode decision inputs", ErrInvalidArgument)
	}
	return ruleInputFields{values: values}, nil
}

func decodeRuleCondition(raw json.RawMessage) (ruleCondition, error) {
	if len(raw) == 0 {
		return ruleCondition{}, fmt.Errorf("%w: empty rule match condition", ErrInvalidArgument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var condition ruleCondition
	if err := decoder.Decode(&condition); err != nil {
		return ruleCondition{}, fmt.Errorf("%w: rule match condition", ErrInvalidArgument)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ruleCondition{}, fmt.Errorf("%w: trailing rule match condition", ErrInvalidArgument)
	}
	if err := validateRuleCondition(condition); err != nil {
		return ruleCondition{}, err
	}
	return condition, nil
}

func validateRuleCondition(condition ruleCondition) error {
	modes := 0
	if len(condition.All) > 0 {
		modes++
	}
	if len(condition.Any) > 0 {
		modes++
	}
	if condition.Field != "" || condition.Op != "" {
		modes++
	}
	if modes > 1 {
		return fmt.Errorf("%w: rule condition mixes all/any/leaf modes", ErrInvalidArgument)
	}
	if modes == 0 {
		// Match-all: the seeded catch-all row.
		if condition.Value != nil {
			return fmt.Errorf("%w: match-all condition carries a value", ErrInvalidArgument)
		}
		return nil
	}
	if condition.Field != "" || condition.Op != "" {
		if condition.Field == "" || condition.Op == "" {
			return fmt.Errorf("%w: leaf rule condition requires field and op", ErrInvalidArgument)
		}
		if !conditionOperators[condition.Op] {
			return fmt.Errorf("%w: unknown rule condition op %q", ErrInvalidArgument, condition.Op)
		}
		if !decisionInputFieldNames()[condition.Field] {
			return fmt.Errorf("%w: rule condition references unknown input field %q", ErrInvalidArgument, condition.Field)
		}
		if len(condition.Value) == 0 || !json.Valid(condition.Value) {
			return fmt.Errorf("%w: leaf rule condition requires a JSON value", ErrInvalidArgument)
		}
		return nil
	}
	for _, child := range condition.All {
		if err := validateRuleCondition(child); err != nil {
			return err
		}
	}
	for _, child := range condition.Any {
		if err := validateRuleCondition(child); err != nil {
			return err
		}
	}
	return nil
}

// matches evaluates the condition against the input field map. Errors mark
// structurally invalid conditions that validation should already have
// rejected; they fail the evaluation instead of silently skipping the rule.
func (condition ruleCondition) matches(fields ruleInputFields) (bool, error) {
	if len(condition.All) == 0 && len(condition.Any) == 0 && condition.Field == "" && condition.Op == "" {
		return true, nil
	}
	if len(condition.All) > 0 {
		for _, child := range condition.All {
			matched, err := child.matches(fields)
			if err != nil || !matched {
				return false, err
			}
		}
		return true, nil
	}
	if len(condition.Any) > 0 {
		for _, child := range condition.Any {
			matched, err := child.matches(fields)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil
			}
		}
		return false, nil
	}
	input, ok := fields.values[condition.Field]
	if !ok {
		return false, fmt.Errorf("%w: rule condition field %q is missing", ErrInvalidArgument, condition.Field)
	}
	expected, err := decodeConditionValue(condition.Value)
	if err != nil {
		return false, err
	}
	switch condition.Op {
	case conditionOpEq:
		return reflect.DeepEqual(input, expected), nil
	case conditionOpNe:
		return !reflect.DeepEqual(input, expected), nil
	case conditionOpIn, conditionOpNotIn:
		candidates, ok := expected.([]any)
		if !ok {
			return false, fmt.Errorf("%w: %s condition requires an array value", ErrInvalidArgument, condition.Op)
		}
		member := false
		for _, candidate := range candidates {
			if reflect.DeepEqual(input, candidate) {
				member = true
				break
			}
		}
		if condition.Op == conditionOpIn {
			return member, nil
		}
		return !member, nil
	}
	comparison, err := compareConditionValues(input, expected)
	if err != nil {
		return false, err
	}
	switch condition.Op {
	case conditionOpGt:
		return comparison > 0, nil
	case conditionOpGte:
		return comparison >= 0, nil
	case conditionOpLt:
		return comparison < 0, nil
	case conditionOpLte:
		return comparison <= 0, nil
	}
	return false, fmt.Errorf("%w: unknown rule condition op %q", ErrInvalidArgument, condition.Op)
}

func decodeConditionValue(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("%w: rule condition value", ErrInvalidArgument)
	}
	return value, nil
}

// compareConditionValues orders numbers numerically and strings
// lexicographically; mixing types is a rule-authoring error, not a match.
func compareConditionValues(input, expected any) (int, error) {
	inputNumber, inputIsNumber := input.(json.Number)
	expectedNumber, expectedIsNumber := expected.(json.Number)
	if inputIsNumber && expectedIsNumber {
		inputValue, inputErr := inputNumber.Float64()
		expectedValue, expectedErr := expectedNumber.Float64()
		if inputErr != nil || expectedErr != nil {
			return 0, fmt.Errorf("%w: rule condition numbers are not comparable", ErrInvalidArgument)
		}
		switch {
		case inputValue < expectedValue:
			return -1, nil
		case inputValue > expectedValue:
			return 1, nil
		default:
			return 0, nil
		}
	}
	inputString, inputIsString := input.(string)
	expectedString, expectedIsString := expected.(string)
	if inputIsString && expectedIsString {
		return bytes.Compare([]byte(inputString), []byte(expectedString)), nil
	}
	return 0, fmt.Errorf("%w: rule condition compares incompatible types", ErrInvalidArgument)
}

// Validate enforces the closed rule shape: identity, versioned coordinates,
// the expert|batch-only routing vocabulary and a parseable closed condition.
// The deferred auto lane is unrepresentable — a rule carrying it is invalid,
// which is the evaluator-level half of the vocabulary lock.
func (rule PolicyRule) Validate() error {
	if rule.ID == "" || len(rule.ID) > 128 {
		return fmt.Errorf("%w: policy rule id", ErrInvalidArgument)
	}
	if !reasonRuleVersionPattern.MatchString(rule.RuleVersion) {
		return fmt.Errorf("%w: policy rule version %q", ErrInvalidArgument, rule.RuleVersion)
	}
	switch rule.RiskLevel {
	case RiskLow, RiskMedium, RiskHigh:
	default:
		return fmt.Errorf("%w: policy rule risk level %q", ErrInvalidArgument, rule.RiskLevel)
	}
	switch rule.Routing {
	case RoutingExpert, RoutingBatch:
	default:
		return fmt.Errorf("%w: policy rule routing %q", ErrInvalidArgument, rule.Routing)
	}
	if !policyReasonCodePattern.MatchString(rule.ReasonCode) {
		return fmt.Errorf("%w: policy rule reason code %q", ErrInvalidArgument, rule.ReasonCode)
	}
	if rule.Explanation == "" || len(rule.Explanation) > 512 {
		return fmt.Errorf("%w: policy rule explanation", ErrInvalidArgument)
	}
	if _, err := decodeRuleCondition(rule.Match); err != nil {
		return err
	}
	return nil
}

// EvaluatePolicyRules evaluates rule rows in priority order (a higher
// priority number is evaluated first; ties broken by rule id) and returns the
// first match. No match fails safe to the expert channel with a stable
// reason (SSOT §8.2 degradation): an unroutable change is always routed to
// human review, never silently to batch. The function is pure: identical
// rules and inputs always reproduce the identical decision.
func EvaluatePolicyRules(rules []PolicyRule, inputs DecisionInputs) (RiskDecision, error) {
	if err := inputs.Validate(); err != nil {
		return RiskDecision{}, err
	}
	ordered := append([]PolicyRule(nil), rules...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Priority != ordered[j].Priority {
			return ordered[i].Priority > ordered[j].Priority
		}
		return ordered[i].ID < ordered[j].ID
	})
	fields, err := newRuleInputFields(inputs)
	if err != nil {
		return RiskDecision{}, err
	}
	for _, rule := range ordered {
		if err := rule.Validate(); err != nil {
			return RiskDecision{}, fmt.Errorf("policy rule %s: %w", rule.ID, err)
		}
		condition, err := decodeRuleCondition(rule.Match)
		if err != nil {
			return RiskDecision{}, fmt.Errorf("policy rule %s: %w", rule.ID, err)
		}
		matched, err := condition.matches(fields)
		if err != nil {
			return RiskDecision{}, fmt.Errorf("policy rule %s: %w", rule.ID, err)
		}
		if matched {
			return RiskDecision{
				MatchedPolicy: rule.ID, RiskLevel: rule.RiskLevel,
				Routing: rule.Routing, ReasonCode: rule.ReasonCode,
			}, nil
		}
	}
	return PolicyFallbackDecision(), nil
}

// PolicyFallbackDecision is the fail-safe outcome for an unmatched or
// unevaluable rule table: expert review with the stable unmatched reason.
func PolicyFallbackDecision() RiskDecision {
	return RiskDecision{
		MatchedPolicy: MatchedRiskPolicy, RiskLevel: RiskHigh,
		Routing: RoutingExpert, ReasonCode: ReasonRiskPolicyUnmatched,
	}
}

// PolicyRuleMatchedInputFields returns the deduplicated, sorted
// DecisionInputs field names the closed match condition references: the input
// categories that drove a decision. A match-all condition names none.
func PolicyRuleMatchedInputFields(match json.RawMessage) ([]string, error) {
	condition, err := decodeRuleCondition(match)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	collectConditionFields(condition, seen)
	fields := make([]string, 0, len(seen))
	for field := range seen {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields, nil
}

func collectConditionFields(condition ruleCondition, seen map[string]bool) {
	if condition.Field != "" {
		seen[condition.Field] = true
	}
	for _, child := range condition.All {
		collectConditionFields(child, seen)
	}
	for _, child := range condition.Any {
		collectConditionFields(child, seen)
	}
}

// CanonicalPolicyRules returns the seeded rule table for one rule version.
// It is the source of truth migration 000008 seeds into policy_rules and the
// table EvaluateRiskRule evaluates; the integration suite locks the database
// rows and this table together.
func CanonicalPolicyRules(version string) ([]PolicyRule, error) {
	switch version {
	case RiskRuleVersion:
		return canonicalPolicyRulesV1(), nil
	default:
		return nil, fmt.Errorf("%w: unknown policy rule version %q", ErrInvalidArgument, version)
	}
}

func canonicalPolicyRulesV1() []PolicyRule {
	return []PolicyRule{
		{
			ID: "semlia.risk.v1/blockers", RuleVersion: RiskRuleVersion, Priority: 100,
			Match:     json.RawMessage(`{"field":"blockerCount","op":"gt","value":0}`),
			RiskLevel: RiskHigh, Routing: RoutingExpert, ReasonCode: ReasonRiskBlocker,
			Explanation: "Validation blockers require expert review before the change can proceed (SSOT §8.4).",
		},
		{
			ID: "semlia.risk.v1/production-computation", RuleVersion: RiskRuleVersion, Priority: 90,
			Match: json.RawMessage(`{"all":[{"field":"affectsComputation","op":"eq","value":true},` +
				`{"field":"productionEnvironment","op":"eq","value":true}]}`),
			RiskLevel: RiskHigh, Routing: RoutingExpert, ReasonCode: ReasonRiskProductionComputation,
			Explanation: "Computation changes in production always take the expert channel (SSOT §8.4).",
		},
		{
			ID: "semlia.risk.v1/access-or-contract", RuleVersion: RiskRuleVersion, Priority: 80,
			Match: json.RawMessage(`{"any":[{"field":"affectsAccess","op":"eq","value":true},` +
				`{"field":"affectsContract","op":"eq","value":true}]}`),
			RiskLevel: RiskHigh, Routing: RoutingExpert, ReasonCode: ReasonRiskAccessOrContractChange,
			Explanation: "Access-scope and consumption-contract changes always take the expert channel (SSOT §8.4).",
		},
		{
			ID: "semlia.risk.v1/computation", RuleVersion: RiskRuleVersion, Priority: 70,
			Match:     json.RawMessage(`{"field":"affectsComputation","op":"eq","value":true}`),
			RiskLevel: RiskMedium, Routing: RoutingExpert, ReasonCode: ReasonRiskComputationChange,
			Explanation: "Computation or expression changes are reviewed item by item by the asset owner.",
		},
		{
			ID: "semlia.risk.v1/production-change", RuleVersion: RiskRuleVersion, Priority: 60,
			Match:     json.RawMessage(`{"field":"productionEnvironment","op":"eq","value":true}`),
			RiskLevel: RiskMedium, Routing: RoutingExpert, ReasonCode: ReasonRiskProductionChange,
			Explanation: "Production-affecting changes are reviewed item by item.",
		},
		{
			ID: "semlia.risk.v1/low-risk", RuleVersion: RiskRuleVersion, Priority: 10,
			Match:     json.RawMessage(`{}`),
			RiskLevel: RiskLow, Routing: RoutingBatch, ReasonCode: ReasonRiskLowBatch,
			Explanation: "No risk category matched; the change joins the batch confirmation channel (SSOT §8.4).",
		},
	}
}
