package execution_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/execution"
	d "github.com/iiwish/semlia/internal/domain/distribution"
	e "github.com/iiwish/semlia/internal/domain/execution"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	sqlite3 "github.com/ncruces/go-sqlite3"
)

// Orders straddle UTC month boundaries on purpose: 2026-08-01 07:30+08:00 is
// still July in UTC, which is how the PostgreSQL dialect buckets periods too.
const shopSchema = `
CREATE TABLE customers (id INTEGER PRIMARY KEY, region text NOT NULL);
CREATE TABLE orders (id INTEGER PRIMARY KEY, customer_id bigint NOT NULL REFERENCES customers, amount numeric(12,2) NOT NULL,
  paid_at timestamptz NOT NULL, is_valid boolean NOT NULL, note text);
CREATE VIEW valid_orders AS SELECT * FROM orders WHERE is_valid = 1;
INSERT INTO customers VALUES (1, 'east'), (2, 'south');
INSERT INTO orders VALUES
  (1, 1, 100.10, '2026-07-15 10:00:00+08:00', 1, 'gift 100%'),
  (2, 1, 200.20, '2026-08-01 07:30:00+08:00', 1, NULL),
  (3, 2, 300.30, '2026-08-02 12:00:00+08:00', 1, 'plain'),
  (4, 2, 999.99, '2026-08-03 12:00:00+08:00', 0, NULL),
  (5, 1, 0.1, '2026-08-20 09:00:00+08:00', 1, NULL),
  (6, 1, 0.2, '2026-08-21 09:00:00+08:00', 1, NULL),
  (7, 2, 50.00, '2026-09-01 00:00:00Z', 1, NULL);`

type memoryFiles struct {
	content  []byte
	digest   string
	opens    int
	executor *execution.SQLite
}

func newFiles(t *testing.T, sql string) *memoryFiles {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	conn, err := sqlite3.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec(sql); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return &memoryFiles{content: content, digest: "sha256:" + hex.EncodeToString(sum[:])}
}

func (f *memoryFiles) SQLiteRevisionArtifact(_ context.Context, _ identity.WorkspaceID, source, revision string) (execution.SQLiteArtifact, error) {
	if source != "source" || revision != "revision" {
		return execution.SQLiteArtifact{}, errors.New("unknown revision")
	}
	return execution.SQLiteArtifact{ContentDigest: f.digest, ByteSize: int64(len(f.content))}, nil
}

func (f *memoryFiles) Open(context.Context, identity.WorkspaceID, string) (io.ReadCloser, error) {
	f.opens++
	return io.NopCloser(bytes.NewReader(f.content)), nil
}

type shopPlan struct {
	plan                                 d.ResolvedSemanticPlan
	total, count, average, order, region identity.AssetID
}

func newShopPlan(t *testing.T, relation string) shopPlan {
	t.Helper()
	ids := make([]identity.AssetID, 5)
	for i := range ids {
		ids[i], _ = identity.NewAssetID()
	}
	release, _ := identity.NewReleaseID()
	s := shopPlan{total: ids[0], count: ids[1], average: ids[2], order: ids[3], region: ids[4]}
	relations := []d.ExecutionRelation{
		{DatasetID: "orders", SourceID: "source", SourceRevisionID: "revision", SourceLocator: "artifact-set:test", AdapterKind: "sqlite_catalog", Schema: "main", Relation: relation,
			Fields: []d.ExecutionField{{FieldID: "id", Name: "id", DataType: "integer"}, {FieldID: "customer_id", Name: "customer_id", DataType: "bigint"},
				{FieldID: "amount", Name: "amount", DataType: "numeric(12,2)"}, {FieldID: "paid_at", Name: "paid_at", DataType: "timestamptz"},
				{FieldID: "is_valid", Name: "is_valid", DataType: "boolean"}, {FieldID: "note", Name: "note", DataType: "text"}}},
		{DatasetID: "customers", SourceID: "source", SourceRevisionID: "revision", SourceLocator: "artifact-set:test", AdapterKind: "sqlite_catalog", Schema: "main", Relation: "customers",
			Fields: []d.ExecutionField{{FieldID: "id", Name: "id", DataType: "integer"}, {FieldID: "region", Name: "region", DataType: "text"}}},
	}
	s.plan = d.ResolvedSemanticPlan{ReleaseID: release, Intent: d.IntentAggregate, Limit: 100,
		Assets: []d.ResolvedAsset{{AssetID: s.total, Address: "sales.total"}, {AssetID: s.count, Address: "sales.count"}, {AssetID: s.average, Address: "sales.average"},
			{AssetID: s.order, Address: "sales.order"}, {AssetID: s.region, Address: "sales.customer"}},
		Measures: []d.Selector{{AssetID: &s.total}},
		Execution: &d.ExecutionProvenance{CompilerVersion: e.CompilerVersion, Relations: relations,
			Bindings: []d.ExecutionBinding{
				{AssetID: s.total.String(), DatasetID: "orders", FieldID: "amount", Aggregation: "sum"},
				{AssetID: s.count.String(), DatasetID: "orders", FieldID: "id", Aggregation: "count_distinct"},
				{AssetID: s.order.String(), MemberID: "paid_at", DatasetID: "orders", FieldID: "paid_at"},
				{AssetID: s.order.String(), MemberID: "is_valid", DatasetID: "orders", FieldID: "is_valid"},
				{AssetID: s.order.String(), MemberID: "note", DatasetID: "orders", FieldID: "note"},
				{AssetID: s.region.String(), MemberID: "region", DatasetID: "customers", FieldID: "region"},
			},
			Calculations: []d.ExecutionCalculation{{AssetID: s.average.String(), Expression: &semantic.KnowledgeExpression{Op: "divide",
				Left:  &semantic.KnowledgeExpression{Op: "ref", Ref: &semantic.KnowledgeReference{AssetID: s.total.String()}},
				Right: &semantic.KnowledgeExpression{Op: "ref", Ref: &semantic.KnowledgeReference{AssetID: s.count.String()}}}}},
			Joins: []d.ExecutionJoin{{Expression: "field_pairs_equal/v1", LeftDatasetID: "orders", RightDatasetID: "customers", JoinType: "inner", Cardinality: "many_to_one",
				FieldPairs: []d.ExecutionFieldPair{{LeftFieldID: "customer_id", RightFieldID: "id"}}}},
		}}
	return s
}

func (s shopPlan) validFilter() d.Filter {
	return d.Filter{Selector: d.Selector{AssetID: &s.order, MemberID: "is_valid"}, Operator: "eq", Value: json.RawMessage(`true`)}
}

func run(t *testing.T, files *memoryFiles, plan d.ResolvedSemanticPlan) (e.Compiled, e.Output) {
	t.Helper()
	compiled, err := e.Compile(plan)
	if err != nil {
		t.Fatal(err)
	}
	if files.executor == nil {
		files.executor = execution.NewSQLite(files)
	}
	router := execution.NewRouter(nil, files.executor)
	workspace, _ := identity.NewWorkspaceID()
	return compiled, router.Execute(context.Background(), workspace, compiled, e.DefaultLimits())
}

func TestSQLiteExecutesMonthlyRegionBreakdownLikePostgreSQL(t *testing.T) {
	files := newFiles(t, shopSchema)
	s := newShopPlan(t, "orders")
	s.plan.Intent = d.IntentBreakdown
	s.plan.Measures = []d.Selector{{AssetID: &s.total}, {AssetID: &s.average}}
	s.plan.Grouping = []d.Selector{{AssetID: &s.region, MemberID: "region"}}
	s.plan.Filters = []d.Filter{s.validFilter()}
	s.plan.TimeRange = &d.TimeRange{Selector: d.Selector{AssetID: &s.order, MemberID: "paid_at"}, Granularity: "month",
		From: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	s.plan.Order = []d.Order{{Selector: d.Selector{AssetID: &s.order, MemberID: "paid_at"}, Direction: "asc"}, {Selector: d.Selector{AssetID: &s.region, MemberID: "region"}, Direction: "asc"}}
	compiled, output := run(t, files, s.plan)
	if output.ErrorCode != "" {
		t.Fatalf("%s: %s", output.ErrorCode, compiled.SQL)
	}
	if compiled.Dialect != "sqlite" || compiled.AdapterVersion != "sqlite-aggregate/v1" || strings.Contains(compiled.SQL, "::") || strings.Contains(compiled.SQL, "$1") {
		t.Fatalf("PostgreSQL syntax leaked into SQLite SQL: %s", compiled.SQL)
	}
	want := [][]any{
		// 2026-08-01 07:30+08:00 is July in UTC, so it joins the July bucket.
		{"300.3", "150.15", "east", "2026-07-01T00:00:00Z"},
		{"0.3", "0.15", "east", "2026-08-01T00:00:00Z"},
		{"300.3", "300.3", "south", "2026-08-01T00:00:00Z"},
	}
	if !reflect.DeepEqual(output.Rows, want) || output.RowCount != 3 || output.Digest == "" {
		t.Fatalf("rows = %#v\nSQL = %s", output.Rows, compiled.SQL)
	}
}

func TestSQLiteFiltersBindTypedValuesAndEscapeContains(t *testing.T) {
	files := newFiles(t, shopSchema)
	s := newShopPlan(t, "orders")
	s.plan.Measures = []d.Selector{{AssetID: &s.count}}
	s.plan.Filters = []d.Filter{{Selector: d.Selector{AssetID: &s.order, MemberID: "note"}, Operator: "contains", Value: json.RawMessage(`"100%"`)}}
	_, output := run(t, files, s.plan)
	if output.ErrorCode != "" || !reflect.DeepEqual(output.Rows, [][]any{{"1"}}) {
		t.Fatalf("contains: %+v", output)
	}
	s.plan.Filters = []d.Filter{s.validFilter(), {Selector: d.Selector{AssetID: &s.order, MemberID: "paid_at"}, Operator: "gte", Value: json.RawMessage(`"2026-08-31T16:00:00Z"`)}}
	_, output = run(t, files, s.plan)
	if output.ErrorCode != "" || !reflect.DeepEqual(output.Rows, [][]any{{"1"}}) {
		t.Fatalf("timestamptz comparison across offsets: %+v", output)
	}
	if files.opens != 1 {
		t.Fatalf("file opened %d times; want a cached copy", files.opens)
	}
}

func TestSQLiteRejectsViewsDuplicateJoinKeysAndForeignBytes(t *testing.T) {
	s := newShopPlan(t, "valid_orders")
	if _, output := run(t, newFiles(t, shopSchema), s.plan); output.ErrorCode != "EXECUTION_RELATION_UNSAFE" {
		t.Fatalf("view executed: %+v", output)
	}
	duplicate := strings.Replace(strings.Replace(shopSchema, "id INTEGER PRIMARY KEY, region", "id bigint, region", 1), "REFERENCES customers", "", 1) +
		"INSERT INTO customers VALUES (1, 'west');"
	if _, output := run(t, newFiles(t, duplicate), newShopPlan(t, "orders").plan); output.ErrorCode != "EXECUTION_GRAIN_OR_NULL_VIOLATION" {
		t.Fatalf("fan-out join executed: %+v", output)
	}
	tampered := newFiles(t, shopSchema)
	tampered.digest = "sha256:" + strings.Repeat("0", 64)
	if _, output := run(t, tampered, newShopPlan(t, "orders").plan); output.ErrorCode != "EXECUTION_SOURCE_FAILED" {
		t.Fatalf("unverified bytes executed: %+v", output)
	}
}

func TestRouterKeepsDialectsApart(t *testing.T) {
	compiled, err := e.Compile(newShopPlan(t, "orders").plan)
	if err != nil {
		t.Fatal(err)
	}
	postgres, err := execution.NewPostgresWithLookup("", func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := identity.NewWorkspaceID()
	if output := postgres.Execute(context.Background(), workspace, compiled, e.DefaultLimits()); output.ErrorCode != "EXECUTION_NOT_CONFIGURED" {
		t.Fatalf("PostgreSQL adapter accepted SQLite SQL: %+v", output)
	}
	if output := execution.NewRouter(postgres, nil).Execute(context.Background(), workspace, compiled, e.DefaultLimits()); output.ErrorCode != "EXECUTION_NOT_CONFIGURED" {
		t.Fatalf("router without SQLite executed: %+v", output)
	}
}

func TestSQLitePeriodsTruncateInUTC(t *testing.T) {
	conn, err := sqlite3.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, test := range []struct{ value, granularity, want string }{
		{"2026-08-01 07:30:00+08:00", "day", "2026-07-31T00:00:00Z"},
		{"2026-08-02", "week", "2026-07-27T00:00:00Z"}, // Sunday belongs to the week starting Monday
		{"2026-08-03", "week", "2026-08-03T00:00:00Z"}, // Monday starts its own week
		{"2026-08-15T10:00:00Z", "month", "2026-08-01T00:00:00Z"},
		{"2026-03-31", "quarter", "2026-01-01T00:00:00Z"},
		{"2026-11-30", "quarter", "2026-10-01T00:00:00Z"},
		{"2026-01-01 03:00:00+08:00", "year", "2025-01-01T00:00:00Z"},
	} {
		stmt, _, err := conn.Prepare("SELECT " + (e.SQLite{}).Period("?1", "timestamptz", test.granularity))
		if err != nil {
			t.Fatal(err)
		}
		if err := stmt.BindText(1, test.value); err != nil || !stmt.Step() || stmt.ColumnText(0) != test.want {
			t.Errorf("%s %s = %q, want %q", test.granularity, test.value, stmt.ColumnText(0), test.want)
		}
		_ = stmt.Close()
	}
}
