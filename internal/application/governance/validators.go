package governance

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/iiwish/semlia/internal/domain/governance"
)

// schemaRequiredFields are the structural field paths whose removal destroys
// the identity or meaning of the target type (semantic-asset design §3.1
// minimum contracts and the T008 governed-object structural columns).
var schemaRequiredFields = map[governance.TargetObjectType][]string{
	governance.TargetSemanticAsset:   {"name", "definition"},
	governance.TargetPhysicalBinding: {"dataset"},
	governance.TargetModelGrain:      {"grainExpression", "grainFieldRefs"},
	governance.TargetEntityKey:       {"keyFieldRefs", "uniquenessSemantics"},
	governance.TargetJoinContract:    {"leftDataset", "rightDataset", "leftFieldRefs", "rightFieldRefs", "joinType", "cardinality", "joinExpression"},
}

const maxFieldPathLength = 512

// looseTypeIDShape matches strings that look like a TypeID; the strict parse
// happens in the snapshot builder, which records the malformed verdict.
var looseTypeIDShape = regexp.MustCompile(`^[a-z]{2,10}_[0-7][0-9a-hjkmnp-tv-z]{25}$`)

type schemaValidator struct{}

func (validator *schemaValidator) ID() string      { return ValidatorIDSchema }
func (validator *schemaValidator) Version() string { return ValidatorVersion1 }

// Validate re-checks the change-set shape as deterministic findings: op vs
// value/digest pairing, field-path bounds, target identity and required-field
// removals per target type. The domain rejects malformed items at write time;
// this validator proves the frozen change-set still satisfies the contract
// when the run executes.
func (validator *schemaValidator) Validate(input ValidationInput) ([]Finding, error) {
	digest, err := ValidationInputDigest(validator.ID(), validator.Version(), input)
	if err != nil {
		return nil, err
	}
	findings := make([]Finding, 0)
	if input.Proposal.TargetType == governance.TargetSemanticAsset &&
		(input.Proposal.AssetID == "" || input.Proposal.BaseRevisionID == "") {
		findings = append(findings, blockerFinding(digest, "SCHEMA_TARGET_IDENTITY",
			"semantic asset proposals must carry an asset reference and a base revision"))
	}
	for index, item := range input.Changes {
		if shapeFinding, ok := schemaItemShapeFinding(digest, index, item); ok {
			findings = append(findings, shapeFinding)
			continue
		}
		if strings.TrimSpace(item.FieldPath) == "" || len(item.FieldPath) > maxFieldPathLength {
			findings = append(findings, blockerFinding(digest, "SCHEMA_FIELD_PATH",
				fmt.Sprintf("change-set item %d carries an empty or oversized field path", index)))
			continue
		}
		if item.Op != governance.ChangeRemove {
			continue
		}
		for _, required := range schemaRequiredFields[input.Proposal.TargetType] {
			if item.FieldPath == required {
				findings = append(findings, blockerFinding(digest, "SCHEMA_REQUIRED_FIELD_REMOVED",
					fmt.Sprintf("removing required field %q breaks the %s contract", item.FieldPath, input.Proposal.TargetType)))
			}
		}
	}
	return findings, nil
}

// schemaItemShapeFinding mirrors ChangeSetItem.Validate: add carries only an
// after side, remove only a before side, update both, and every digest is
// paired with its value.
func schemaItemShapeFinding(digest string, index int, item governance.ChangeSetItem) (Finding, bool) {
	valid := false
	switch item.Op {
	case governance.ChangeAdd:
		valid = item.BeforeDigest == "" && item.BeforeValue == nil &&
			item.AfterDigest != "" && item.AfterValue != nil
	case governance.ChangeUpdate:
		valid = item.BeforeDigest != "" && item.AfterDigest != "" &&
			item.BeforeValue != nil && item.AfterValue != nil
	case governance.ChangeRemove:
		valid = item.AfterDigest == "" && item.AfterValue == nil &&
			item.BeforeDigest != "" && item.BeforeValue != nil
	}
	if valid {
		return Finding{}, false
	}
	return blockerFinding(digest, "SCHEMA_ITEM_SHAPE",
		fmt.Sprintf("change-set item %d on %q does not pair op %q with its digest and value sides", index, item.FieldPath, item.Op)), true
}

type referenceValidator struct{}

func (validator *referenceValidator) ID() string      { return ValidatorIDReference }
func (validator *referenceValidator) Version() string { return ValidatorVersion1 }

// Validate resolves the target and every TypeID token found in the frozen
// change-set values against the workspace snapshot. A missing supported
// reference is a blocker; prefixes without a resolution mapping stay a
// warning so an unresolvable-but-harmless token degrades to human handling
// instead of hard-failing a valid proposal.
func (validator *referenceValidator) Validate(input ValidationInput) ([]Finding, error) {
	digest, err := ValidationInputDigest(validator.ID(), validator.Version(), input)
	if err != nil {
		return nil, err
	}
	findings := make([]Finding, 0)
	switch input.Proposal.TargetType {
	case governance.TargetSemanticAsset:
		if !input.Target.AssetExists {
			findings = append(findings, blockerFinding(digest, "REFERENCE_TARGET_MISSING",
				"the proposal target asset does not exist in the workspace"))
		}
		if !input.Target.BaseRevisionExists {
			findings = append(findings, blockerFinding(digest, "REFERENCE_TARGET_MISSING",
				"the proposal base revision does not exist in the workspace"))
		}
	default:
		if !input.Target.TargetObjectExists {
			findings = append(findings, blockerFinding(digest, "REFERENCE_TARGET_MISSING",
				fmt.Sprintf("the proposal target %s does not exist in the workspace", input.Proposal.TargetType)))
		}
	}
	checked := map[string]bool{}
	for _, item := range input.Changes {
		for _, token := range append(referenceTokens(item.BeforeValue), referenceTokens(item.AfterValue)...) {
			if checked[token] {
				continue
			}
			checked[token] = true
			resolution, resolved := input.Target.Resolutions[token]
			if !resolved {
				// The snapshot builder must resolve every token; an absent
				// verdict fails closed instead of silently passing.
				findings = append(findings, blockerFinding(digest, "REFERENCE_UNRESOLVED",
					fmt.Sprintf("reference %q could not be resolved in the workspace", token)))
				continue
			}
			if resolution.Exists {
				continue
			}
			switch resolution.Kind {
			case ReferenceKindMalformed:
				findings = append(findings, blockerFinding(digest, "REFERENCE_MALFORMED",
					fmt.Sprintf("reference %q is not a well-formed identifier", token)))
			case ReferenceKindUnsupported:
				findings = append(findings, warningFinding(digest, "REFERENCE_UNSUPPORTED_KIND",
					fmt.Sprintf("reference %q uses a prefix without workspace resolution; confirm manually", token)))
			default:
				findings = append(findings, blockerFinding(digest, "REFERENCE_UNRESOLVED",
					fmt.Sprintf("reference %q does not exist in the workspace", token)))
			}
		}
	}
	return findings, nil
}

// referenceTokens walks a JSON value and collects whole strings that look
// like TypeIDs. Values embedded in larger strings (URLs, prose) never match.
func referenceTokens(value json.RawMessage) []string {
	if len(value) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil
	}
	tokens := make([]string, 0)
	var walk func(node any)
	walk = func(node any) {
		switch typed := node.(type) {
		case string:
			if looseTypeIDShape.MatchString(typed) {
				tokens = append(tokens, typed)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sortStrings(keys)
			for _, key := range keys {
				walk(typed[key])
			}
		}
	}
	walk(decoded)
	return tokens
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

type structuralValidator struct{}

func (validator *structuralValidator) ID() string      { return ValidatorIDStructural }
func (validator *structuralValidator) Version() string { return ValidatorVersion1 }

// Validate checks diff sanity: every provided digest must equal the sha256
// of its paired canonical value, and no-op updates are signalled (SSOT §8.2)
// without blocking — a set that normalizes to nothing is refused at submit,
// but a no-op item inside a substantive set is a quality gap, not a blocker.
func (validator *structuralValidator) Validate(input ValidationInput) ([]Finding, error) {
	digest, err := ValidationInputDigest(validator.ID(), validator.Version(), input)
	if err != nil {
		return nil, err
	}
	findings := make([]Finding, 0)
	for index, item := range input.Changes {
		findings = append(findings, structuralDigestFindings(digest, index, item)...)
		if item.Op == governance.ChangeUpdate && item.BeforeDigest == item.AfterDigest &&
			item.BeforeDigest != "" {
			findings = append(findings, warningFinding(digest, "STRUCTURAL_NO_OP",
				fmt.Sprintf("update on %q carries identical before and after digests", item.FieldPath)))
		}
	}
	return findings, nil
}

func structuralDigestFindings(digest string, index int, item governance.ChangeSetItem) []Finding {
	findings := make([]Finding, 0)
	for side, value := range map[string]json.RawMessage{"before": item.BeforeValue, "after": item.AfterValue} {
		sideDigest := item.BeforeDigest
		if side == "after" {
			sideDigest = item.AfterDigest
		}
		if sideDigest == "" || value == nil {
			continue
		}
		computed, err := governance.DigestJSON(value)
		if err != nil {
			findings = append(findings, blockerFinding(digest, "STRUCTURAL_DIGEST_MISMATCH",
				fmt.Sprintf("change-set item %d on %q has a malformed %s value", index, item.FieldPath, side)))
			continue
		}
		if computed != sideDigest {
			findings = append(findings, blockerFinding(digest, "STRUCTURAL_DIGEST_MISMATCH",
				fmt.Sprintf("change-set item %d on %q has a %s digest that does not match its value", index, item.FieldPath, side)))
		}
	}
	return findings
}

func blockerFinding(digest, code, message string) Finding {
	return Finding{Severity: governance.SeverityBlocker, Code: code, Message: message, InputDigest: digest}
}

func warningFinding(digest, code, message string) Finding {
	return Finding{Severity: governance.SeverityWarning, Code: code, Message: message, InputDigest: digest}
}
