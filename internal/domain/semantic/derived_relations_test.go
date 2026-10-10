package semantic

import (
	"reflect"
	"testing"
)

func ref(asset string) KnowledgeReference {
	return KnowledgeReference{AssetID: asset, RevisionID: "rev_" + asset, ReleaseID: "rls_x", MemberID: "m"}
}

func refExpr(asset string) *KnowledgeExpression {
	r := ref(asset)
	return &KnowledgeExpression{Op: "ref", Ref: &r}
}

func derived(t *testing.T, subject string, kind AssetType, spec KnowledgeSpec) map[string]DerivedRelation {
	t.Helper()
	out := map[string]DerivedRelation{}
	for _, relation := range DeriveSpecRelations(subject, kind, spec) {
		out[string(relation.Predicate)+"->"+relation.ObjectAssetID] = relation
	}
	return out
}

func TestDeriveSpecRelationsForAggregateMetric(t *testing.T) {
	input, term, orders := ref("ast_orders"), ref("ast_paid"), ref("ast_orders")
	got := derived(t, "ast_amount", "metric", KnowledgeSpec{Kind: "aggregate", InputRef: &input, FilterRefs: []KnowledgeReference{term}, TimeAttributeRef: &orders})
	want := map[string]struct {
		plane  RelationPlane
		fields []string
	}{
		"measures->ast_orders":   {SemanticPlane, []string{"spec.inputRef"}},
		"depends_on->ast_orders": {DependencyPlane, []string{"spec.timeAttributeRef"}},
		"filters_by->ast_paid":   {SemanticPlane, []string{"spec.filterRefs"}},
	}
	if len(got) != len(want) {
		t.Fatalf("relations = %+v", got)
	}
	for key, expected := range want {
		relation, ok := got[key]
		if !ok || relation.Plane != expected.plane || !reflect.DeepEqual(relation.Fields, expected.fields) {
			t.Fatalf("%s = %+v, want plane %s fields %v", key, relation, expected.plane, expected.fields)
		}
	}
}

func TestDeriveSpecRelationsForDerivedMetricUsesExpressionRefs(t *testing.T) {
	got := derived(t, "ast_average", "metric", KnowledgeSpec{Kind: "derived", Expression: &KnowledgeExpression{
		Op: "div", Left: refExpr("ast_amount"), Right: &KnowledgeExpression{Op: "add", Left: refExpr("ast_count"), Right: refExpr("ast_amount")},
	}})
	if len(got) != 2 || got["derived_from->ast_amount"].Plane != DependencyPlane || got["derived_from->ast_count"].Predicate != DerivedFrom {
		t.Fatalf("relations = %+v", got)
	}
	if !reflect.DeepEqual(got["derived_from->ast_amount"].Fields, []string{"spec.expression"}) {
		t.Fatalf("a repeated reference must collapse to one relation: %+v", got["derived_from->ast_amount"])
	}
}

func TestDeriveSpecRelationsForBusinessTerm(t *testing.T) {
	subject := ref("ast_customers")
	got := derived(t, "ast_old", "business_term", KnowledgeSpec{SubjectRef: &subject, Predicate: &KnowledgeExpression{Op: "lt", Left: refExpr("ast_customers"), Right: refExpr("ast_orders")}})
	if got["describes->ast_customers"].Plane != SemanticPlane || got["depends_on->ast_customers"].Predicate != DependsOn || got["depends_on->ast_orders"].Predicate != DependsOn || len(got) != 3 {
		t.Fatalf("relations = %+v", got)
	}
}

func TestDeriveSpecRelationsForAnalysisModel(t *testing.T) {
	base, attr, metric, term, data, time := ref("ast_orders"), ref("ast_customers"), ref("ast_amount"), ref("ast_old"), ref("ast_order_data"), ref("ast_orders")
	got := derived(t, "ast_model", "analysis_model", KnowledgeSpec{
		BaseObjectRef: &base, PublicAttributeRefs: []KnowledgeReference{attr, base}, MetricRefs: []KnowledgeReference{metric},
		CompatibleTermRefs: []KnowledgeReference{term}, DataAssetRefs: []KnowledgeReference{data}, DefaultTimeAttributeRef: &time,
		MemberBindings:  []MemberBinding{{SemanticRef: ref("ast_orders"), DataRef: ref("ast_customer_data")}},
		JoinContractIDs: []string{"jc_1"},
	})
	for _, key := range []string{"contains->ast_orders", "contains->ast_customers", "contains->ast_amount", "filters_by->ast_old", "depends_on->ast_order_data", "depends_on->ast_orders", "depends_on->ast_customer_data"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing %s in %+v", key, got)
		}
	}
	if len(got) != 7 {
		t.Fatalf("join contracts are not assets and must not become relations: %+v", got)
	}
	if !reflect.DeepEqual(got["contains->ast_orders"].Fields, []string{"spec.baseObjectRef", "spec.publicAttributeRefs"}) {
		t.Fatalf("fields = %v", got["contains->ast_orders"].Fields)
	}
}

func TestDeriveSpecRelationsIgnoresSelfEmptyAndPhysicalOnlyTypes(t *testing.T) {
	self, empty := ref("ast_metric"), KnowledgeReference{}
	if got := DeriveSpecRelations("ast_metric", "metric", KnowledgeSpec{Kind: "aggregate", InputRef: &self, FilterRefs: []KnowledgeReference{empty}}); len(got) != 0 {
		t.Fatalf("self and empty references must not form relations: %+v", got)
	}
	for _, kind := range []AssetType{"business_object", "data_asset"} {
		if got := DeriveSpecRelations("ast_x", kind, KnowledgeSpec{Members: []KnowledgeMember{{ID: "a"}}}); len(got) != 0 {
			t.Fatalf("%s has only physical references: %+v", kind, got)
		}
	}
}

func TestDeriveSpecRelationsIsDeterministic(t *testing.T) {
	spec := KnowledgeSpec{PublicAttributeRefs: []KnowledgeReference{ref("ast_b"), ref("ast_a"), ref("ast_c")}, MetricRefs: []KnowledgeReference{ref("ast_a")}}
	first := DeriveSpecRelations("ast_model", "analysis_model", spec)
	for i := 0; i < 20; i++ {
		if !reflect.DeepEqual(first, DeriveSpecRelations("ast_model", "analysis_model", spec)) {
			t.Fatal("derivation order must be stable")
		}
	}
	if first[0].ObjectAssetID != "ast_a" || first[len(first)-1].ObjectAssetID != "ast_c" {
		t.Fatalf("relations must sort by object then predicate: %+v", first)
	}
}
