package semantic

import (
	"encoding/json"
	"fmt"
	"time"
)

// ValidateReferences checks semantic roles in addition to reference identity.
// The loader must resolve the exact immutable release/revision triple.
func (spec KnowledgeSpec) ValidateReferences(load func(KnowledgeReference) (AssetType, KnowledgeSpec, error)) error {
	check := func(ref KnowledgeReference, expected AssetType, member bool) error {
		kind, target, err := load(ref)
		if err != nil {
			return err
		}
		if kind != expected || (ref.MemberID != "") != member {
			return fmt.Errorf("knowledge reference has incompatible role")
		}
		if member {
			for _, m := range target.Members {
				if m.ID == ref.MemberID {
					return nil
				}
			}
			return fmt.Errorf("unknown knowledge member")
		}
		return nil
	}
	for _, ref := range pointerRefs(spec.SubjectRef, spec.BaseObjectRef) {
		if err := check(ref, BusinessObject, false); err != nil {
			return err
		}
	}
	for _, ref := range append(append([]KnowledgeReference{}, spec.PublicAttributeRefs...), pointerRefs(spec.InputRef, spec.TimeAttributeRef, spec.DefaultTimeAttributeRef)...) {
		if err := check(ref, BusinessObject, true); err != nil {
			return err
		}
	}
	for _, ref := range append(append([]KnowledgeReference{}, spec.FilterRefs...), spec.CompatibleTermRefs...) {
		if err := check(ref, BusinessTerm, false); err != nil {
			return err
		}
	}
	for _, ref := range spec.MetricRefs {
		if err := check(ref, Metric, false); err != nil {
			return err
		}
	}
	for _, ref := range spec.DataAssetRefs {
		if err := check(ref, DataAsset, false); err != nil {
			return err
		}
	}
	for _, binding := range spec.MemberBindings {
		if err := check(binding.SemanticRef, BusinessObject, true); err != nil {
			return err
		}
		if err := check(binding.DataRef, DataAsset, true); err != nil {
			return err
		}
	}
	var walk func(*KnowledgeExpression, AssetType, bool, int) error
	walk = func(e *KnowledgeExpression, kind AssetType, member bool, depth int) error {
		if e == nil {
			return nil
		}
		if depth > 16 {
			return fmt.Errorf("expression depth exceeded")
		}
		if e.Ref != nil {
			if err := check(*e.Ref, kind, member); err != nil {
				return err
			}
		}
		if err := walk(e.Left, kind, member, depth+1); err != nil {
			return err
		}
		return walk(e.Right, kind, member, depth+1)
	}
	if err := walk(spec.Expression, Metric, false, 0); err != nil {
		return err
	}
	if err := walk(spec.Predicate, BusinessObject, true, 0); err != nil {
		return err
	}
	if spec.Predicate != nil {
		return spec.validatePredicateTypes(load)
	}
	return nil
}

func (spec KnowledgeSpec) validatePredicateTypes(load func(KnowledgeReference) (AssetType, KnowledgeSpec, error)) error {
	parameters := map[string]string{}
	for _, parameter := range spec.Parameters {
		parameters[parameter.Name] = parameter.Type
	}
	invalid := fmt.Errorf("predicate must be boolean with compatible subject member types")
	var infer func(*KnowledgeExpression, int) (string, error)
	infer = func(e *KnowledgeExpression, depth int) (string, error) {
		if e == nil || depth > 16 {
			return "", invalid
		}
		switch e.Op {
		case "ref":
			if e.Ref == nil || spec.SubjectRef == nil || e.Ref.AssetID != spec.SubjectRef.AssetID || e.Ref.RevisionID != spec.SubjectRef.RevisionID {
				return "", invalid
			}
			_, target, err := load(*e.Ref)
			if err != nil {
				return "", err
			}
			for _, member := range target.Members {
				if member.ID == e.Ref.MemberID && member.ValueType != "" {
					return member.ValueType, nil
				}
			}
			return "", invalid
		case "parameter":
			if parameters[e.Parameter] == "" {
				return "", invalid
			}
			return parameters[e.Parameter], nil
		case "literal":
			var value any
			if json.Unmarshal(e.Value, &value) != nil {
				return "", invalid
			}
			switch value.(type) {
			case string:
				return "string", nil
			case bool:
				return "boolean", nil
			case float64:
				return "number", nil
			}
			return "", invalid
		}
		left, err := infer(e.Left, depth+1)
		if err != nil {
			return "", err
		}
		right, err := infer(e.Right, depth+1)
		if err != nil {
			return "", err
		}
		if e.Op == "and" || e.Op == "or" {
			if left != "boolean" || right != "boolean" {
				return "", invalid
			}
			return "boolean", nil
		}
		if !optionalEnum(e.Op, "eq neq lt lte gt gte") || e.Op == "" {
			return "", invalid
		}
		// Temporal literals are typed against the referenced member, not accepted
		// as arbitrary text that PostgreSQL might coerce differently at execution.
		coerceTemporal := func(expression *KnowledgeExpression, actual, expected string) string {
			if expression.Op != "literal" || actual != "string" || (expected != "date" && expected != "timestamp") {
				return actual
			}
			var value string
			if json.Unmarshal(expression.Value, &value) != nil {
				return actual
			}
			layouts := []string{"2006-01-02"}
			if expected == "timestamp" {
				layouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"}
			}
			for _, layout := range layouts {
				if _, err := time.Parse(layout, value); err == nil {
					return expected
				}
			}
			return actual
		}
		left = coerceTemporal(e.Left, left, right)
		right = coerceTemporal(e.Right, right, left)
		numeric := func(typ string) bool { return typ == "number" || typ == "integer" }
		if left != right && !(numeric(left) && numeric(right)) {
			return "", invalid
		}
		if left == "boolean" && e.Op != "eq" && e.Op != "neq" {
			return "", invalid
		}
		return "boolean", nil
	}
	typ, err := infer(spec.Predicate, 0)
	if err != nil {
		return err
	}
	if typ != "boolean" {
		return invalid
	}
	return nil
}
