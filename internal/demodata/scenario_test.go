package demodata

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	d "github.com/iiwish/semlia/internal/domain/distribution"
	e "github.com/iiwish/semlia/internal/domain/execution"
	s "github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestPreviewCSVsMatchSyntheticFixture(t *testing.T) {
	expectedOrders := [][]string{{"order_id", "customer_id", "paid_at", "paid_amount", "is_valid"}}
	for _, row := range orders() {
		expectedOrders = append(expectedOrders, []string{row.ID, row.CustomerID, row.PaidAt + "T00:00:00Z", strconv.Itoa(row.Amount), strconv.Itoa(row.IsValid)})
	}
	expectedCustomers := [][]string{{"customer_id", "region", "first_paid_at"}}
	for _, row := range customers() {
		expectedCustomers = append(expectedCustomers, []string{row.ID, row.Region, row.FirstPaidAt + "T00:00:00Z"})
	}
	for name, expected := range map[string][][]string{"orders": expectedOrders, "customers": expectedCustomers} {
		t.Run(name, func(t *testing.T) {
			file, err := os.Open("../../scripts/demo/knowledge-202609/" + name + ".csv")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			actual, err := csv.NewReader(file).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("CSV differs from the briefing-verified fixture: got %v, want %v", actual, expected)
			}
		})
	}
}

func TestScenarioCompletePublicationContracts(t *testing.T) {
	c := New()
	if len(c.Snapshot.Assets) != 10 || len(c.Refs) != 10 {
		t.Fatal("demo must cover all ten knowledge assets")
	}
	seen := map[string]s.KnowledgeReference{}
	kinds := map[s.AssetType]bool{}
	for _, asset := range c.Snapshot.Assets {
		var content struct {
			Definition string          `json:"definition"`
			Scope      string          `json:"scope"`
			Spec       json.RawMessage `json:"spec"`
		}
		if err := json.Unmarshal(asset.Content, &content); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(content.Definition, "合成演示") || !strings.Contains(content.Scope, "合成演示") || !strings.HasPrefix(asset.Address, "demo_202609.") {
			t.Fatalf("missing demo disclosure: %s", asset.Address)
		}
		spec, err := s.ParseKnowledgeSpec(asset.AssetType, content.Spec, true)
		if err != nil {
			t.Fatalf("%s: %v", asset.Address, err)
		}
		for _, ref := range spec.References() {
			previous, ok := seen[ref.AssetID]
			if !ok || ref.RevisionID != previous.RevisionID || ref.ReleaseID != previous.ReleaseID {
				t.Fatalf("%s dependency is not an earlier pinned asset: %+v", asset.Address, ref)
			}
		}
		seen[asset.AssetID.String()] = s.KnowledgeReference{AssetID: asset.AssetID.String(), RevisionID: asset.RevisionID.String(), ReleaseID: c.Snapshot.ReleaseID.String()}
		kinds[asset.AssetType] = true
		if asset.Address == "demo_202609.customers" && spec.Members[1].HistoryPolicy != "current_value" {
			t.Fatal("customer region must explicitly use current value")
		}
		if asset.Address == "demo_202609.average" && (spec.ZeroDenominator != "null" || spec.Rollup != "recompute_from_inputs") {
			t.Fatal("derived metric must preserve division and rollup semantics")
		}
	}
	if len(kinds) != 5 || len(c.Snapshot.Bindings) != 2 || len(c.Snapshot.Execution.Relations) != 2 || len(c.Snapshot.Execution.Joins) != 1 {
		t.Fatal("incomplete five-type source and join fixture")
	}
	other := New()
	if other.Snapshot.ReleaseID == c.Snapshot.ReleaseID || other.Refs["orders"] == c.Refs["orders"] || other.DataSQL != c.DataSQL {
		t.Fatal("fixture must use independent identities and identical data")
	}
}

func TestScenarioCompilesUTCQuery(t *testing.T) {
	c := New()
	plan, refusal, err := d.BuildPlan(c.Query, c.Snapshot, identity.SemanticQueryID{}, identity.ResolvedSemanticPlanID{}, nil, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || refusal != nil {
		t.Fatalf("demo plan refused: %+v %v", refusal, err)
	}
	compiled, err := e.Compile(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, clause := range []string{`SUM(`, `NULLIF(COUNT(DISTINCT`, `"semlia_demo_202609"."orders"`, `LEFT JOIN "semlia_demo_202609"."customers"`, `"is_valid" = $`, `"first_paid_at" < $`, `"region" = $`, `"paid_at" >= $`, `"paid_at" < $`} {
		if !strings.Contains(compiled.SQL, clause) {
			t.Errorf("missing %q in %s", clause, compiled.SQL)
		}
	}
	if len(plan.Assets) != 10 || plan.Model == nil || len(compiled.Guards) == 0 {
		t.Fatal("incomplete executable knowledge model")
	}
	args, err := json.Marshal(compiled.Args)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`"1"`, `"华东"`, `"2026-08-01T00:00:00Z"`, `"2026-09-01T00:00:00Z"`} {
		if !strings.Contains(string(args), value) {
			t.Errorf("missing bound query value %s: %s", value, args)
		}
	}
	if strings.Contains(compiled.SQL, "华东") || strings.Contains(compiled.SQL, "2026-08-01") {
		t.Fatal("demo query values must be parameterized")
	}
}

func TestScenarioMatchesBriefingExactly(t *testing.T) {
	raw, err := os.ReadFile("../../docs/briefings/2026-09-semantic-platform/case-example.json")
	if err != nil {
		t.Fatal(err)
	}
	var example struct {
		Customers  []customer `json:"customers"`
		Orders     []order    `json:"orders"`
		Parameters struct {
			MonthStart string `json:"month_start"`
			MonthEnd   string `json:"month_end"`
			Region     string `json:"region"`
		} `json:"parameters"`
		Expected struct {
			IncludedOrderIDs []string `json:"includedOrderIds"`
			PaidAmount       int      `json:"paidAmount"`
			OrderCount       int      `json:"orderCount"`
			CustomerCount    int      `json:"customerCount"`
			AvgOrderValue    int      `json:"avgOrderValue"`
		} `json:"expected"`
	}
	if err := json.Unmarshal(raw, &example); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(customers(), example.Customers) || !reflect.DeepEqual(orders(), example.Orders) {
		t.Fatal("synthetic rows drifted from the briefing's exact four customers and seven orders")
	}
	c := New()
	if c.Query.TimeRange.From.Format("2006-01-02") != example.Parameters.MonthStart || c.Query.TimeRange.To.Format("2006-01-02") != example.Parameters.MonthEnd || string(c.Query.Filters[1].Value) != `"`+example.Parameters.Region+`"` {
		t.Fatal("query drifted from briefing parameters")
	}
	lookup := map[string]customer{}
	for _, customer := range customers() {
		lookup[customer.ID] = customer
		if !strings.Contains(c.DataSQL, "'"+customer.FirstPaidAt+"T00:00:00Z'") {
			t.Fatal("SQL must preserve explicit UTC full-history customer timestamps")
		}
	}
	var included []string
	amount := 0
	uniqueCustomers := map[string]bool{}
	for _, order := range orders() {
		customer := lookup[order.CustomerID]
		if order.IsValid == 1 && order.PaidAt >= example.Parameters.MonthStart && order.PaidAt < example.Parameters.MonthEnd && customer.Region == example.Parameters.Region && customer.FirstPaidAt < example.Parameters.MonthStart {
			included = append(included, order.ID)
			amount += order.Amount
			uniqueCustomers[order.CustomerID] = true
		}
		if !strings.Contains(c.DataSQL, "'"+order.PaidAt+"T00:00:00Z'") {
			t.Fatal("SQL must preserve explicit UTC order timestamps")
		}
	}
	if !reflect.DeepEqual(included, example.Expected.IncludedOrderIDs) || amount != example.Expected.PaidAmount || len(included) != example.Expected.OrderCount || len(uniqueCustomers) != example.Expected.CustomerCount || amount/len(included) != example.Expected.AvgOrderValue {
		t.Fatalf("expected O01/O02/O03, 600 / 3 = 200, got %v amount=%d customers=%d", included, amount, len(uniqueCustomers))
	}
	if strings.Count(c.DataSQL, "INSERT INTO "+Schema+".orders VALUES") != 7 || strings.Count(c.DataSQL, "INSERT INTO "+Schema+".customers VALUES") != 4 {
		t.Fatal("SQL must include exactly seven orders and four customers")
	}
}
