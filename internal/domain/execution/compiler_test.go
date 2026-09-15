package execution_test

import (
	"encoding/json"
	"strings"
	"testing"

	d "github.com/iiwish/semlia/internal/domain/distribution"
	e "github.com/iiwish/semlia/internal/domain/execution"
	"github.com/iiwish/semlia/pkg/identity"
)

func executablePlan(t *testing.T) d.ResolvedSemanticPlan {
	t.Helper()
	asset, _ := identity.NewAssetID()
	revision, _ := identity.NewRevisionID()
	release, _ := identity.NewReleaseID()
	return d.ResolvedSemanticPlan{ReleaseID: release, Intent: d.IntentAggregate,
		Assets:   []d.ResolvedAsset{{AssetID: asset, RevisionID: revision, Address: "sales.total"}},
		Measures: []d.Selector{{AssetID: &asset}}, Limit: 10,
		Execution: &d.ExecutionProvenance{CompilerVersion: e.CompilerVersion, Bindings: []d.ExecutionBinding{{AssetID: asset.String(), DatasetID: "dataset", FieldID: "field", Aggregation: "sum"}},
			Relations: []d.ExecutionRelation{{DatasetID: "dataset", SourceID: "source", SourceRevisionID: "revision", AdapterKind: "postgresql_catalog", Schema: "public", Relation: "sales", Fields: []d.ExecutionField{{FieldID: "field", Name: "amount", DataType: "numeric"}}}}}}
}

func TestCompilerParameterizedAndLegacyClosed(t *testing.T) {
	p := executablePlan(t)
	p.Filters = []d.Filter{{Selector: p.Measures[0], Operator: "gte", Value: json.RawMessage(`100`)}}
	q, err := e.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q.SQL, "100") || !strings.Contains(q.SQL, "$1") || len(q.Args) != 1 {
		t.Fatalf("not parameterized: %#v", q)
	}
	p.Execution = nil
	if _, err := e.Compile(p); err == nil {
		t.Fatal("legacy plan executed without frozen provenance")
	}
}

func TestCompilerRejectsUnsupportedContracts(t *testing.T) {
	for _, mutate := range []func(*d.ResolvedSemanticPlan){
		func(p *d.ResolvedSemanticPlan) {
			p.Execution.Bindings[0].Aggregation = "sum(amount); DELETE FROM sales"
		},
		func(p *d.ResolvedSemanticPlan) { p.Execution.Relations[0].Fields[0].DataType = "custom_udt" },
		func(p *d.ResolvedSemanticPlan) { p.Execution.Relations[0].SourceRevisionID = "" },
		func(p *d.ResolvedSemanticPlan) { p.Execution.Bindings[0].Expression = "amount * 2" },
	} {
		p := executablePlan(t)
		mutate(&p)
		if _, err := e.Compile(p); err == nil {
			t.Fatal("unsafe contract accepted")
		}
	}
}

func TestNumericDigestsAndArgumentsPreservePrecision(t *testing.T) {
	for _, pair := range [][2]string{{"9007199254740992", "9007199254740993"}, {"0.123456789012345678901", "0.123456789012345678902"}} {
		digests := []string{}
		requests := []string{}
		p := executablePlan(t)
		for _, value := range pair {
			p.Filters = []d.Filter{{Selector: p.Measures[0], Operator: "gte", Value: json.RawMessage(value)}}
			digests = append(digests, d.PlanDigest(p))
			q := d.SemanticQueryInput{SchemaVersion: d.QuerySchemaVersion, Intent: d.IntentAggregate, Measures: p.Measures, Filters: p.Filters, Context: d.ResolutionContext{Mode: d.ResolutionCurrent}}
			_, digest, err := q.Canonical()
			if err != nil {
				t.Fatal(err)
			}
			requests = append(requests, digest)
			compiled, err := e.Compile(p)
			if err != nil || compiled.Args[0] != value {
				t.Fatalf("precision changed: %v %v", compiled.Args, err)
			}
		}
		if digests[0] == "" || digests[0] == digests[1] || requests[0] == requests[1] {
			t.Fatal("distinct numeric values share digest")
		}
	}
}

func TestCompilerTypmodsAndTypedInvalidValues(t *testing.T) {
	for _, test := range []struct {
		typ, value string
		valid      bool
	}{{"numeric(18,2)", "123.45", true}, {"numeric(18,2);SELECT 1", "123.45", false}, {"bigint", "1.5", false}, {"bigint", "9223372036854775808", false}, {"uuid", `"not-a-uuid"`, false}, {"date", `"2026-02-31"`, false}, {"character varying(64)", `"exact text"`, true}} {
		p := executablePlan(t)
		p.Execution.Relations[0].Fields[0].DataType = test.typ
		if test.typ == "uuid" || test.typ == "date" || strings.HasPrefix(test.typ, "character") {
			p.Execution.Bindings[0].Aggregation = "max"
		}
		p.Filters = []d.Filter{{Selector: p.Measures[0], Operator: "eq", Value: json.RawMessage(test.value)}}
		if test.typ == "bigint" {
			p.Execution.Bindings[0].Aggregation = "max"
		}
		_, err := e.Compile(p)
		if (err == nil) != test.valid {
			t.Fatalf("type %s value %s: %v", test.typ, test.value, err)
		}
	}
}

func TestCompositeJoinUsesEveryKeyAndRejectsUnsafeContracts(t *testing.T) {
	p := executablePlan(t)
	p.Execution.Relations[0].Fields = append(p.Execution.Relations[0].Fields, d.ExecutionField{FieldID: "tenant-left", Name: "tenant_id", DataType: "bigint"}, d.ExecutionField{FieldID: "customer-left", Name: "customer_id", DataType: "bigint"})
	p.Execution.Relations = append(p.Execution.Relations, d.ExecutionRelation{DatasetID: "customers", SourceID: "source", SourceRevisionID: "revision", AdapterKind: "postgresql_catalog", Schema: "public", Relation: "customers", Fields: []d.ExecutionField{{FieldID: "tenant-right", Name: "tenant_id", DataType: "bigint"}, {FieldID: "customer-right", Name: "id", DataType: "bigint"}}})
	p.Execution.Joins = []d.ExecutionJoin{{ID: "join", Version: 1, LeftDatasetID: "dataset", RightDatasetID: "customers", JoinType: "left", Cardinality: "one_to_one", Expression: "field_pairs_equal/v1", FieldPairs: []d.ExecutionFieldPair{{LeftFieldID: "tenant-left", RightFieldID: "tenant-right"}, {LeftFieldID: "customer-left", RightFieldID: "customer-right"}}}}
	compiled, err := e.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compiled.SQL, `"t0"."tenant_id" = "t1"."tenant_id" AND "t0"."customer_id" = "t1"."id"`) {
		t.Fatal("composite join lost a governed key")
	}
	originalDigest := d.PlanDigest(p)
	p.Execution.Joins[0].FieldPairs = p.Execution.Joins[0].FieldPairs[1:]
	if d.PlanDigest(p) == originalDigest {
		t.Fatal("formal join keys absent from digest")
	}
	for _, cardinality := range []string{"one_to_many", "many_to_one", "many_to_many"} {
		p.Execution.Joins[0].Cardinality = cardinality
		if _, err := e.Compile(p); err == nil {
			t.Fatal("unproven aggregate grain accepted")
		}
	}
	p.Execution.Joins[0].Cardinality = "one_to_one"
	p.Execution.Joins[0].Expression = "tenant_id=tenant_id OR true"
	if _, err := e.Compile(p); err == nil {
		t.Fatal("free join expression accepted")
	}
	p.Execution.Joins[0].Expression = "field_pairs_equal/v1"
	p.Execution.Joins[0].FieldPairs[0].RightFieldID = "customer-left"
	if _, err := e.Compile(p); err == nil {
		t.Fatal("join field from wrong dataset accepted")
	}
}
