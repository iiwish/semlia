// Package demodata describes synthetic demo knowledge without accessing storage.
package demodata

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	d "github.com/iiwish/semlia/internal/domain/distribution"
	s "github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

const Schema = "semlia_demo_202609"

const MacoSourceLocator = "postgresql:5432/semlia_test"

// NewMaco uses the registered project database, never a bundled database.
func NewMaco() Scenario {
	scenario := New()
	for i := range scenario.Snapshot.Execution.Relations {
		scenario.Snapshot.Execution.Relations[i].SourceLocator = MacoSourceLocator
	}
	return scenario
}

type Scenario struct {
	// Assets are ordered by dependency: objects, data, terms, aggregates, derived, model.
	Snapshot d.ReleaseSnapshot
	Query    d.SemanticQueryInput
	Refs     map[string]s.KnowledgeReference
	DataSQL  string
}

// New constructs an independent publication basis. The caller owns persistence
// and must remap references when publishing these assets under different IDs.
func New() Scenario {
	release, _ := identity.NewReleaseID()
	source, _ := identity.NewSourceConnectionID()
	sourceRevision, _ := identity.NewSourceRevisionID()
	snapshot, _ := identity.NewSourceSnapshotID()
	c := Scenario{Snapshot: d.ReleaseSnapshot{ReleaseID: release, Execution: &d.ExecutionProvenance{CompilerVersion: "postgres-aggregate/v1"}}, Refs: map[string]s.KnowledgeReference{}, DataSQL: dataSQL()}
	assets := []struct {
		key, name, definition string
		kind                  s.AssetType
	}{
		{"orders", "订单", "合成演示订单，以订单标识唯一识别；支付时间按 UTC 解释。is_valid 仅为演算标记，不定义企业真实有效订单规则。", s.BusinessObject},
		{"customers", "客户", "合成演示客户；区域采用当前值，不表示交易发生时的历史区域。首次有效支付时间是已确认的全历史合成属性，不从七笔样本订单推算。", s.BusinessObject},
		{"order_data", "订单演示数据", "合成演示七笔订单，仅用于口径演算，不是真实业务数据。", s.DataAsset},
		{"customer_data", "客户演示数据", "合成演示四位客户；首次有效支付时间由全历史合成设定提供。", s.DataAsset},
		{"paid", "有效订单", "合成演示规则：订单 is_valid = 1。该标记仅用于本样本演算，不定义企业真实有效订单规则。", s.BusinessTerm},
		{"old", "老客户", "合成演示规则：客户首次有效支付时间严格早于查询起始时间 period_start；首次支付属性覆盖全历史，不从本期或七笔样本订单推算。", s.BusinessTerm},
		{"amount", "有效支付金额", "合成演示指标：有效订单支付金额求和，单位为人民币元，按订单支付时间限定查询区间。", s.Metric},
		{"count", "有效订单数", "合成演示指标：有效订单标识去重计数；分母是订单数，不是客户数。", s.Metric},
		{"average", "客单价", "合成演示指标：有效支付金额 / 有效订单数；分母为零返回空值，汇总时从金额与订单数重新计算，不平均已有客单价。", s.Metric},
		{"model", "老客户客单价分析", "合成演示分析模型：订单按客户标识关联客户，采用多对一左连接。2026 年 8 月 UTC 半开区间 [2026-08-01, 2026-09-01) 内，当前区域为华东的老客户客单价为 600 / 3 = 200 元/单。", s.AnalysisModel},
	}
	for i, asset := range assets {
		id, _ := identity.NewAssetID()
		revision, _ := identity.NewRevisionID()
		c.Refs[asset.key] = s.KnowledgeReference{AssetID: id.String(), RevisionID: revision.String(), ReleaseID: release.String()}
		c.Snapshot.Assets = append(c.Snapshot.Assets, d.ReleasedAsset{AssetID: id, RevisionID: revision, Address: "demo_202609." + asset.key, Name: asset.name, AssetType: asset.kind, Position: i})
	}
	member := func(name, id string) s.KnowledgeReference { r := c.Refs[name]; r.MemberID = id; return r }
	ptr := func(r s.KnowledgeReference) *s.KnowledgeReference { return &r }
	expr := func(r s.KnowledgeReference) *s.KnowledgeExpression { return &s.KnowledgeExpression{Op: "ref", Ref: &r} }
	specs := map[string]s.KnowledgeSpec{}
	bindings := []s.MemberBinding{}
	datasetIDs := map[string]string{}
	fieldIDs := map[string]map[string]string{}
	for _, table := range []struct {
		name, data, grain      string
		columns, types, labels []string
	}{
		{"orders", "order_data", "每笔订单一行", []string{"id", "amount", "paid_at", "customer_id", "is_valid"}, []string{"text", "numeric", "timestamptz", "text", "integer"}, []string{"订单标识", "支付金额", "支付时间（UTC）", "客户标识", "演示有效标记"}},
		{"customers", "customer_data", "每位客户一行", []string{"id", "region", "first_paid_at"}, []string{"text", "text", "timestamptz"}, []string{"客户标识", "当前区域", "首次有效支付时间（全历史）"}},
	} {
		dataset, _ := identity.NewPhysicalDatasetID()
		datasetRevision, _ := identity.NewPhysicalDatasetRevisionID()
		binding, _ := identity.NewPhysicalBindingID()
		datasetIDs[table.name] = dataset.UUID()
		fieldIDs[table.name] = map[string]string{}
		relation := d.ExecutionRelation{DatasetID: dataset.UUID(), DatasetRevisionID: datasetRevision.UUID(), SourceID: source.UUID(), SourceRevisionID: sourceRevision.UUID(), SourceLocator: "127.0.0.1:5433/semlia_demo_202609", AdapterKind: "postgresql_catalog", Schema: Schema, Relation: table.name}
		object := s.KnowledgeSpec{Grain: table.grain, Keys: []string{"id"}, IdentityPolicy: "合成演示稳定主键", Lifecycle: "合成演示固定样本，完整保留"}
		coverage := "合成演示七笔订单，不代表完整订单历史"
		if table.name == "customers" {
			coverage = "合成演示四位客户；首次有效支付时间是已确认的全历史属性，不从七笔订单推算；区域为当前值"
		}
		data := s.KnowledgeSpec{Grain: table.grain, Keys: []string{"id"}, Coverage: coverage, RefreshFrequency: "固定合成演示样本，不自动刷新", Sensitivity: "纯合成数据，无真实客户信息", DatasetRef: &s.SourceReference{SnapshotID: snapshot.String(), Kind: "dataset", ObjectID: dataset.String(), RevisionID: datasetRevision.String()}}
		for i, column := range table.columns {
			field, _ := identity.NewPhysicalFieldID()
			revision, _ := identity.NewPhysicalFieldRevisionID()
			fieldIDs[table.name][column] = field.UUID()
			relation.Fields = append(relation.Fields, d.ExecutionField{FieldID: field.UUID(), RevisionID: revision.UUID(), Name: column, DataType: table.types[i]})
			valueType := map[string]string{"integer": "integer", "numeric": "number", "timestamptz": "timestamp", "text": "string"}[table.types[i]]
			history := "event_time"
			if column == "region" {
				history = "current_value"
			} else if column == "id" || column == "customer_id" {
				history = "stable"
			}
			object.Members = append(object.Members, s.KnowledgeMember{ID: column, Name: table.labels[i], ValueType: valueType, NullPolicy: "required", HistoryPolicy: history})
			data.Members = append(data.Members, s.KnowledgeMember{ID: column, Name: table.labels[i], NullPolicy: "required", SourceFieldRef: &s.SourceReference{SnapshotID: snapshot.String(), Kind: "field", ObjectID: field.String(), RevisionID: revision.String()}})
			bindings = append(bindings, s.MemberBinding{SemanticRef: member(table.name, column), DataRef: member(table.data, column)})
		}
		specs[table.name], specs[table.data] = object, data
		asset, _ := identity.ParseAssetID(c.Refs[table.data].AssetID)
		c.Snapshot.Bindings = append(c.Snapshot.Bindings, d.ReleasedPhysicalBinding{ID: binding, Version: 1, AssetID: asset, DatasetID: dataset})
		c.Snapshot.Execution.BindingPins = append(c.Snapshot.Execution.BindingPins, d.ExecutionBindingPin{BindingID: binding.UUID(), Version: 1, DatasetID: dataset.UUID(), DatasetRevisionID: datasetRevision.UUID()})
		c.Snapshot.Execution.Relations = append(c.Snapshot.Execution.Relations, relation)
	}
	specs["paid"] = s.KnowledgeSpec{Capability: "predicate", SubjectRef: ptr(c.Refs["orders"]), Stage: "object", NullPolicy: "unknown_does_not_match", Predicate: &s.KnowledgeExpression{Op: "eq", Left: expr(member("orders", "is_valid")), Right: &s.KnowledgeExpression{Op: "literal", Value: json.RawMessage(`1`)}}}
	specs["old"] = s.KnowledgeSpec{Capability: "predicate", SubjectRef: ptr(c.Refs["customers"]), Stage: "object", NullPolicy: "unknown_does_not_match", Parameters: []s.KnowledgeParameter{{Name: "period_start", Type: "timestamp", Required: true}}, Predicate: &s.KnowledgeExpression{Op: "lt", Left: expr(member("customers", "first_paid_at")), Right: &s.KnowledgeExpression{Op: "parameter", Parameter: "period_start"}}}
	for _, name := range []string{"amount", "count"} {
		input, aggregation, unit := "amount", "sum", "CNY"
		if name == "count" {
			input, aggregation, unit = "id", "count_distinct", "orders"
		}
		specs[name] = s.KnowledgeSpec{Kind: "aggregate", InputRef: ptr(member("orders", input)), Aggregation: aggregation, Unit: unit, NullPolicy: "required", TimeAttributeRef: ptr(member("orders", "paid_at")), FilterRefs: []s.KnowledgeReference{c.Refs["paid"]}}
	}
	specs["average"] = s.KnowledgeSpec{Kind: "derived", Unit: "CNY/order", NullPolicy: "exclude", TimeAttributeRef: ptr(member("orders", "paid_at")), ZeroDenominator: "null", Rollup: "recompute_from_inputs", Expression: &s.KnowledgeExpression{Op: "divide", Left: expr(c.Refs["amount"]), Right: expr(c.Refs["count"])}}
	join, _ := identity.NewJoinContractID()
	c.Snapshot.Execution.Joins = []d.ExecutionJoin{{ID: join.String(), Version: 1, LeftDatasetID: datasetIDs["orders"], RightDatasetID: datasetIDs["customers"], JoinType: "left", Cardinality: "many_to_one", Expression: "field_pairs_equal/v1", FieldPairs: []d.ExecutionFieldPair{{LeftFieldID: fieldIDs["orders"]["customer_id"], RightFieldID: fieldIDs["customers"]["id"]}}}}
	specs["model"] = s.KnowledgeSpec{BaseObjectRef: ptr(c.Refs["orders"]), Grain: "每笔订单一行", DefaultTimeAttributeRef: ptr(member("orders", "paid_at")), PublicAttributeRefs: []s.KnowledgeReference{member("customers", "region")}, MetricRefs: []s.KnowledgeReference{c.Refs["amount"], c.Refs["count"], c.Refs["average"]}, CompatibleTermRefs: []s.KnowledgeReference{c.Refs["paid"], c.Refs["old"]}, DataAssetRefs: []s.KnowledgeReference{c.Refs["order_data"], c.Refs["customer_data"]}, MemberBindings: bindings, JoinContractIDs: []string{join.String()}}
	for i, asset := range assets {
		c.Snapshot.Assets[i].Content, _ = json.Marshal(map[string]any{"assetType": asset.kind, "definition": asset.definition, "scope": "合成演示；2026 年 8 月 UTC；非真实业务数据", "spec": specs[asset.key]})
	}
	selector := func(name, field string) d.Selector {
		id, _ := identity.ParseAssetID(c.Refs[name].AssetID)
		return d.Selector{AssetID: &id, MemberID: field}
	}
	c.Query = d.SemanticQueryInput{SchemaVersion: d.QuerySchemaVersion, Context: d.ResolutionContext{Mode: d.ResolutionCurrent}, Intent: d.IntentAggregate, Measures: []d.Selector{selector("average", "")}, Filters: []d.Filter{{Selector: selector("old", ""), Operator: "eq", Value: json.RawMessage(`true`)}, {Selector: selector("customers", "region"), Operator: "eq", Value: json.RawMessage(`"华东"`)}}, TimeRange: &d.TimeRange{Selector: selector("orders", "paid_at"), From: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Limit: 100}
	return c
}

type customer struct {
	ID          string `json:"customer_id"`
	Region      string `json:"region"`
	FirstPaidAt string `json:"first_paid_at"`
}

type order struct {
	ID         string `json:"order_id"`
	CustomerID string `json:"customer_id"`
	PaidAt     string `json:"paid_at"`
	Amount     int    `json:"paid_amount"`
	IsValid    int    `json:"is_valid"`
}

func customers() []customer {
	return []customer{{"C01", "华东", "2026-06-05"}, {"C02", "华东", "2026-07-31"}, {"C03", "华东", "2026-08-03"}, {"C04", "华南", "2026-06-01"}}
}

func orders() []order {
	return []order{{"O01", "C01", "2026-08-05", 100, 1}, {"O02", "C01", "2026-08-20", 200, 1}, {"O03", "C02", "2026-08-10", 300, 1}, {"O04", "C03", "2026-08-15", 400, 1}, {"O05", "C04", "2026-08-22", 500, 1}, {"O06", "C01", "2026-07-31", 600, 1}, {"O07", "C02", "2026-08-14", 700, 0}}
}

func dataSQL() string {
	var sql strings.Builder
	fmt.Fprintf(&sql, "CREATE SCHEMA %s;\nCREATE TABLE %s.orders(id text PRIMARY KEY, amount numeric NOT NULL, paid_at timestamptz NOT NULL, customer_id text NOT NULL, is_valid integer NOT NULL);\nCREATE TABLE %s.customers(id text PRIMARY KEY, region text NOT NULL, first_paid_at timestamptz NOT NULL);\n", Schema, Schema, Schema)
	// All values come from the fixed synthetic fixture, never caller input.
	for _, customer := range customers() {
		fmt.Fprintf(&sql, "INSERT INTO %s.customers VALUES ('%s','%s','%sT00:00:00Z');\n", Schema, customer.ID, customer.Region, customer.FirstPaidAt)
	}
	for _, order := range orders() {
		fmt.Fprintf(&sql, "INSERT INTO %s.orders VALUES ('%s',%d,'%sT00:00:00Z','%s',%d);\n", Schema, order.ID, order.Amount, order.PaidAt, order.CustomerID, order.IsValid)
	}
	return sql.String()
}
