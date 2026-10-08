package semantic

import "sort"

// DerivedRelation is a typed relation implied by the references inside one knowledge spec. It is a projection of
// published content, not a stored assertion: it cannot drift from the spec it was read from, and it is never
// confirmed by a person, so it is always presented as inferred.
type DerivedRelation struct {
	Predicate     RelationPredicate
	Plane         RelationPlane
	ObjectAssetID string
	// Fields lists the spec fields the relation was read from, sorted, e.g. "spec.inputRef".
	Fields []string
}

// DeriveSpecRelations reads the references of one knowledge spec and returns the typed relations they imply, with the
// subject being the asset that owns the spec. References to the subject itself and empty references are ignored.
// Physical source references are not semantic assets and never become relations. The result is deterministic:
// sorted by object, then predicate; repeated references collapse into one relation that lists every field.
func DeriveSpecRelations(subjectAssetID string, subjectType AssetType, spec KnowledgeSpec) []DerivedRelation {
	type key struct {
		predicate RelationPredicate
		object    string
	}
	collected := map[key]map[string]struct{}{}
	add := func(predicate RelationPredicate, field string, refs ...KnowledgeReference) {
		for _, reference := range refs {
			if reference.AssetID == "" || reference.AssetID == subjectAssetID {
				continue
			}
			k := key{predicate, reference.AssetID}
			if collected[k] == nil {
				collected[k] = map[string]struct{}{}
			}
			collected[k][field] = struct{}{}
		}
	}
	one := func(reference *KnowledgeReference) []KnowledgeReference {
		if reference == nil {
			return nil
		}
		return []KnowledgeReference{*reference}
	}

	switch subjectType {
	case Metric:
		add(Measures, "spec.inputRef", one(spec.InputRef)...)
		add(FiltersBy, "spec.filterRefs", spec.FilterRefs...)
		add(DependsOn, "spec.timeAttributeRef", one(spec.TimeAttributeRef)...)
		add(DerivedFrom, "spec.expression", expressionRefs(spec.Expression)...)
	case BusinessTerm:
		add(Describes, "spec.subjectRef", one(spec.SubjectRef)...)
		add(DependsOn, "spec.predicate", expressionRefs(spec.Predicate)...)
	case AnalysisModel:
		add(Contains, "spec.baseObjectRef", one(spec.BaseObjectRef)...)
		add(Contains, "spec.publicAttributeRefs", spec.PublicAttributeRefs...)
		add(Contains, "spec.metricRefs", spec.MetricRefs...)
		add(FiltersBy, "spec.compatibleTermRefs", spec.CompatibleTermRefs...)
		add(DependsOn, "spec.dataAssetRefs", spec.DataAssetRefs...)
		add(DependsOn, "spec.defaultTimeAttributeRef", one(spec.DefaultTimeAttributeRef)...)
		for _, binding := range spec.MemberBindings {
			// The semantic side is already covered by the model's own contents; the data side is a dependency.
			add(DependsOn, "spec.memberBindings", binding.DataRef)
		}
	}

	relations := make([]DerivedRelation, 0, len(collected))
	for k, fields := range collected {
		names := make([]string, 0, len(fields))
		for field := range fields {
			names = append(names, field)
		}
		sort.Strings(names)
		relations = append(relations, DerivedRelation{Predicate: k.predicate, Plane: relationPolicies[k.predicate].plane, ObjectAssetID: k.object, Fields: names})
	}
	sort.Slice(relations, func(i, j int) bool {
		if relations[i].ObjectAssetID != relations[j].ObjectAssetID {
			return relations[i].ObjectAssetID < relations[j].ObjectAssetID
		}
		return relations[i].Predicate < relations[j].Predicate
	})
	return relations
}

func expressionRefs(expression *KnowledgeExpression) []KnowledgeReference {
	if expression == nil {
		return nil
	}
	var refs []KnowledgeReference
	if expression.Ref != nil {
		refs = append(refs, *expression.Ref)
	}
	refs = append(refs, expressionRefs(expression.Left)...)
	return append(refs, expressionRefs(expression.Right)...)
}
