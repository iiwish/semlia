// Package knowledgecase provides explicitly synthetic commerce knowledge for tests.
package knowledgecase

import (
	"encoding/json"
	d "github.com/iiwish/semlia/internal/domain/distribution"
	s "github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"time"
)

type Case struct {
	Snapshot d.ReleaseSnapshot
	Query    d.SemanticQueryInput
	Refs     map[string]s.KnowledgeReference
}

func New() Case {
	release, _ := identity.NewReleaseID()
	source, _ := identity.NewSourceConnectionID()
	sourceRevision, _ := identity.NewSourceRevisionID()
	snapshot, _ := identity.NewSourceSnapshotID()
	c := Case{Snapshot: d.ReleaseSnapshot{ReleaseID: release, Execution: &d.ExecutionProvenance{CompilerVersion: "postgres-aggregate/v1"}}, Refs: map[string]s.KnowledgeReference{}}
	kinds := map[string]s.AssetType{"orders": s.BusinessObject, "customers": s.BusinessObject, "order_data": s.DataAsset, "customer_data": s.DataAsset, "paid": s.BusinessTerm, "old": s.BusinessTerm, "amount": s.Metric, "count": s.Metric, "average": s.Metric, "model": s.AnalysisModel}
	for _, name := range []string{"orders", "customers", "order_data", "customer_data", "paid", "old", "amount", "count", "average", "model"} {
		id, _ := identity.NewAssetID()
		revision, _ := identity.NewRevisionID()
		c.Refs[name] = s.KnowledgeReference{AssetID: id.String(), RevisionID: revision.String(), ReleaseID: release.String()}
		c.Snapshot.Assets = append(c.Snapshot.Assets, d.ReleasedAsset{AssetID: id, RevisionID: revision, Address: "commerce." + name, Name: name, AssetType: kinds[name]})
	}
	member := func(name, id string) s.KnowledgeReference { r := c.Refs[name]; r.MemberID = id; return r }
	ptr := func(r s.KnowledgeReference) *s.KnowledgeReference { return &r }
	expr := func(r s.KnowledgeReference) *s.KnowledgeExpression { return &s.KnowledgeExpression{Op: "ref", Ref: &r} }
	specs := map[string]s.KnowledgeSpec{}
	bindings := []s.MemberBinding{}
	datasetIDs := map[string]string{}
	fieldIDs := map[string]map[string]string{}
	for _, table := range []struct {
		name, data     string
		columns, types []string
	}{
		{"orders", "order_data", []string{"id", "amount", "paid_at", "customer_id", "status"}, []string{"bigint", "numeric", "timestamptz", "bigint", "text"}},
		{"customers", "customer_data", []string{"id", "region", "first_paid_at"}, []string{"bigint", "text", "timestamptz"}},
	} {
		dataset, _ := identity.NewPhysicalDatasetID()
		datasetRevision, _ := identity.NewPhysicalDatasetRevisionID()
		binding, _ := identity.NewPhysicalBindingID()
		datasetIDs[table.name] = dataset.UUID()
		fieldIDs[table.name] = map[string]string{}
		relation := d.ExecutionRelation{DatasetID: dataset.UUID(), DatasetRevisionID: datasetRevision.UUID(), SourceID: source.UUID(), SourceRevisionID: sourceRevision.UUID(), SourceLocator: "synthetic-commerce", AdapterKind: "postgresql_catalog", Schema: "knowledge_case", Relation: table.name}
		object := s.KnowledgeSpec{Grain: "one " + table.name, Keys: []string{"id"}, IdentityPolicy: "stable primary key", Lifecycle: "retained"}
		data := s.KnowledgeSpec{Grain: object.Grain, Keys: object.Keys, Coverage: "synthetic fixture, complete history", RefreshFrequency: "test fixture", Sensitivity: "synthetic", DatasetRef: &s.SourceReference{SnapshotID: snapshot.String(), Kind: "dataset", ObjectID: dataset.String(), RevisionID: datasetRevision.String()}}
		for i, column := range table.columns {
			field, _ := identity.NewPhysicalFieldID()
			revision, _ := identity.NewPhysicalFieldRevisionID()
			fieldIDs[table.name][column] = field.UUID()
			relation.Fields = append(relation.Fields, d.ExecutionField{FieldID: field.UUID(), RevisionID: revision.UUID(), Name: column, DataType: table.types[i]})
			valueType := map[string]string{"bigint": "integer", "numeric": "number", "timestamptz": "timestamp", "text": "string"}[table.types[i]]
			history := "event_time"
			if column == "region" {
				history = "current_value"
			}
			if column == "id" {
				history = "stable"
			}
			object.Members = append(object.Members, s.KnowledgeMember{ID: column, Name: column, ValueType: valueType, NullPolicy: "required", HistoryPolicy: history})
			data.Members = append(data.Members, s.KnowledgeMember{ID: column, Name: column, NullPolicy: "required", SourceFieldRef: &s.SourceReference{SnapshotID: snapshot.String(), Kind: "field", ObjectID: field.String(), RevisionID: revision.String()}})
			bindings = append(bindings, s.MemberBinding{SemanticRef: member(table.name, column), DataRef: member(table.data, column)})
		}
		specs[table.name] = object
		specs[table.data] = data
		asset, _ := identity.ParseAssetID(c.Refs[table.data].AssetID)
		c.Snapshot.Bindings = append(c.Snapshot.Bindings, d.ReleasedPhysicalBinding{ID: binding, Version: 1, AssetID: asset, DatasetID: dataset})
		c.Snapshot.Execution.BindingPins = append(c.Snapshot.Execution.BindingPins, d.ExecutionBindingPin{BindingID: binding.UUID(), Version: 1, DatasetID: dataset.UUID(), DatasetRevisionID: datasetRevision.UUID()})
		c.Snapshot.Execution.Relations = append(c.Snapshot.Execution.Relations, relation)
	}
	specs["paid"] = s.KnowledgeSpec{Capability: "predicate", SubjectRef: ptr(c.Refs["orders"]), Stage: "object", NullPolicy: "unknown_does_not_match", Predicate: &s.KnowledgeExpression{Op: "eq", Left: expr(member("orders", "status")), Right: &s.KnowledgeExpression{Op: "literal", Value: json.RawMessage(`"paid"`)}}}
	specs["old"] = s.KnowledgeSpec{Capability: "predicate", SubjectRef: ptr(c.Refs["customers"]), Stage: "object", NullPolicy: "unknown_does_not_match", Parameters: []s.KnowledgeParameter{{Name: "period_start", Type: "timestamp", Required: true}}, Predicate: &s.KnowledgeExpression{Op: "lt", Left: expr(member("customers", "first_paid_at")), Right: &s.KnowledgeExpression{Op: "parameter", Parameter: "period_start"}}}
	for _, name := range []string{"amount", "count"} {
		input, agg, unit := "amount", "sum", "CNY"
		if name == "count" {
			input, agg, unit = "id", "count_distinct", "orders"
		}
		specs[name] = s.KnowledgeSpec{Kind: "aggregate", InputRef: ptr(member("orders", input)), Aggregation: agg, Unit: unit, NullPolicy: "required", TimeAttributeRef: ptr(member("orders", "paid_at")), FilterRefs: []s.KnowledgeReference{c.Refs["paid"]}}
	}
	specs["average"] = s.KnowledgeSpec{Kind: "derived", Unit: "CNY/order", NullPolicy: "exclude", TimeAttributeRef: ptr(member("orders", "paid_at")), ZeroDenominator: "null", Rollup: "recompute_from_inputs", Expression: &s.KnowledgeExpression{Op: "divide", Left: expr(c.Refs["amount"]), Right: expr(c.Refs["count"])}}
	join, _ := identity.NewJoinContractID()
	c.Snapshot.Execution.Joins = []d.ExecutionJoin{{ID: join.String(), Version: 1, LeftDatasetID: datasetIDs["orders"], RightDatasetID: datasetIDs["customers"], JoinType: "left", Cardinality: "many_to_one", Expression: "field_pairs_equal/v1", FieldPairs: []d.ExecutionFieldPair{{LeftFieldID: fieldIDs["orders"]["customer_id"], RightFieldID: fieldIDs["customers"]["id"]}}}}
	specs["model"] = s.KnowledgeSpec{BaseObjectRef: ptr(c.Refs["orders"]), Grain: "one orders", DefaultTimeAttributeRef: ptr(member("orders", "paid_at")), PublicAttributeRefs: []s.KnowledgeReference{member("customers", "region")}, MetricRefs: []s.KnowledgeReference{c.Refs["amount"], c.Refs["count"], c.Refs["average"]}, CompatibleTermRefs: []s.KnowledgeReference{c.Refs["paid"], c.Refs["old"]}, DataAssetRefs: []s.KnowledgeReference{c.Refs["order_data"], c.Refs["customer_data"]}, MemberBindings: bindings, JoinContractIDs: []string{join.String()}}
	for i := range c.Snapshot.Assets {
		a := &c.Snapshot.Assets[i]
		a.Content, _ = json.Marshal(map[string]any{"assetType": a.AssetType, "definition": "Synthetic " + a.Name, "scope": "test", "spec": specs[a.Name]})
	}
	selector := func(name, field string) d.Selector {
		id, _ := identity.ParseAssetID(c.Refs[name].AssetID)
		return d.Selector{AssetID: &id, MemberID: field}
	}
	c.Query = d.SemanticQueryInput{SchemaVersion: d.QuerySchemaVersion, Context: d.ResolutionContext{Mode: d.ResolutionCurrent}, Intent: d.IntentAggregate, Measures: []d.Selector{selector("average", "")}, Filters: []d.Filter{{Selector: selector("old", ""), Operator: "eq", Value: json.RawMessage(`true`)}, {Selector: selector("customers", "region"), Operator: "eq", Value: json.RawMessage(`"East"`)}}, TimeRange: &d.TimeRange{Selector: selector("orders", "paid_at"), From: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Limit: 100}
	return c
}

const DataSQL = `CREATE SCHEMA knowledge_case;
CREATE TABLE knowledge_case.orders(id bigint, amount numeric, paid_at timestamptz, customer_id bigint, status text);
CREATE TABLE knowledge_case.customers(id bigint, region text, first_paid_at timestamptz);
INSERT INTO knowledge_case.customers VALUES (1,'East','2026-06-01'),(2,'East','2026-08-05'),(3,'West','2026-06-01');
INSERT INTO knowledge_case.orders VALUES (1,100,'2026-08-01',1,'paid'),(2,200,'2026-08-15',1,'paid'),(3,300,'2026-08-31',1,'paid'),(4,900,'2026-08-15',2,'paid'),(5,999,'2026-08-15',3,'paid'),(6,999,'2026-08-15',1,'cancelled'),(7,400,'2026-07-15',1,'paid'),(8,999,'2026-09-01',1,'paid');`
