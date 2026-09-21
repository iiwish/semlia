package governance_test

import (
	"context"
	"encoding/json"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	s "github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"net/http"
	"testing"
	"time"
)

func publishExecutionKnowledge(t *testing.T, env *fixture, w identity.WorkspaceID, author, reviewer, publisher identity.PrincipalID, dataset identity.PhysicalDatasetID, dr identity.PhysicalDatasetRevisionID, amount, paid identity.PhysicalFieldID, amountRevision, paidRevision identity.PhysicalFieldRevisionID) (identity.AssetID, identity.AssetID) {
	t.Helper()
	ctx := context.Background()
	publish := func(name string, kind s.AssetType, spec s.KnowledgeSpec) s.KnowledgeReference {
		body, _ := json.Marshal(map[string]any{"assetType": kind, "name": name, "definition": "Revenue after refunds", "scope": "Synthetic precision fixture", "spec": spec})
		catalog := catalogapp.NewService(env.store, catalogapp.ClockFunc(time.Now))
		created, err := catalog.CreateAsset(ctx, catalogapp.CreateAssetRequest{WorkspaceID: w, Address: "execution." + name, AssetType: kind, SchemaVersion: "1.0.0", Content: body, CreatedBy: author.String(), TraceID: traceID})
		if err != nil {
			t.Fatal(err)
		}
		_, _, proposal := proposeToInReview(t, env, w, author, created.ID, created.CurrentRevision.ID)
		approveProposal(t, env, w, reviewer, proposal)
		if response := publishProposal(t, env, w, publisher, proposal); response.Code != http.StatusCreated {
			t.Fatalf("publish %s: %d %s", name, response.Code, response.Body.String())
		}
		snapshot, err := env.store.CurrentReleaseSnapshot(ctx, w)
		if err != nil {
			t.Fatal(err)
		}
		for _, asset := range snapshot.Assets {
			if asset.AssetID == created.ID {
				return s.KnowledgeReference{AssetID: asset.AssetID.String(), RevisionID: asset.RevisionID.String(), ReleaseID: snapshot.ReleaseID.String()}
			}
		}
		t.Fatal("missing published fixture")
		return s.KnowledgeReference{}
	}
	object := publish("fact", s.BusinessObject, s.KnowledgeSpec{Grain: "one fixture row", Keys: []string{"amount"}, IdentityPolicy: "unique synthetic amount", Lifecycle: "retained", Members: []s.KnowledgeMember{{ID: "amount", Name: "Amount", ValueType: "number", NullPolicy: "required", HistoryPolicy: "event_time"}, {ID: "paid_at", Name: "Paid time", ValueType: "timestamp", NullPolicy: "required", HistoryPolicy: "event_time"}}})
	snapshot, _ := identity.NewSourceSnapshotID()
	if _, err := env.pool.Exec(ctx, `INSERT INTO source_snapshots(id,workspace_id,source_connection_id,source_revision_id,adapter_version,scope_digest,content_digest,history_quality,coverage_status) SELECT $1,$2,s.source_connection_id,s.id,'fixture/1',$4,$4,'verified','complete' FROM source_revisions s JOIN physical_dataset_revisions d ON d.workspace_id=s.workspace_id AND d.source_revision_id=s.id WHERE d.id=$3`, snapshot.UUID(), w.UUID(), dr.UUID(), digestOf("execution-snapshot")); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO source_snapshot_scope(workspace_id,snapshot_id,coverage_key,selector,config_digest,status,enumeration_complete,diagnostic_codes) VALUES($1,$2,'public','public',$3,'complete',true,'{}')`, w.UUID(), snapshot.UUID(), digestOf("execution-scope")); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO source_snapshot_members(workspace_id,snapshot_id,kind,object_id,revision_id,historical_name,historical_locator,content_digest,coverage_key) VALUES($1,$2,'dataset',$3,$4,'execution_facts','public.execution_facts',$5,'public')`, w.UUID(), snapshot.UUID(), dataset.UUID(), dr.UUID(), digestOf("execution-dataset")); err != nil {
		t.Fatal(err)
	}
	for _, member := range []struct {
		id       identity.PhysicalFieldID
		revision identity.PhysicalFieldRevisionID
		name     string
	}{{amount, amountRevision, "amount"}, {paid, paidRevision, "paid_at"}} {
		if _, err := env.pool.Exec(ctx, `INSERT INTO source_snapshot_members(workspace_id,snapshot_id,kind,object_id,revision_id,historical_name,historical_locator,content_digest,coverage_key,parent_object_id,parent_revision_id) VALUES($1,$2,'field',$3,$4,$5,$5,$6,'public',$7,$8)`, w.UUID(), snapshot.UUID(), member.id.UUID(), member.revision.UUID(), member.name, digestOf(member.name), dataset.UUID(), dr.UUID()); err != nil {
			t.Fatal(err)
		}
	}
	data := publish("data", s.DataAsset, s.KnowledgeSpec{Grain: "one fixture row", Keys: []string{"amount"}, Coverage: "synthetic", RefreshFrequency: "fixture", Sensitivity: "synthetic", DatasetRef: &s.SourceReference{SnapshotID: snapshot.String(), Kind: "dataset", ObjectID: dataset.String(), RevisionID: dr.String()}, Members: []s.KnowledgeMember{{ID: "amount", Name: "Amount", NullPolicy: "required", SourceFieldRef: &s.SourceReference{SnapshotID: snapshot.String(), Kind: "field", ObjectID: amount.String(), RevisionID: amountRevision.String()}}, {ID: "paid_at", Name: "Paid time", NullPolicy: "required", SourceFieldRef: &s.SourceReference{SnapshotID: snapshot.String(), Kind: "field", ObjectID: paid.String(), RevisionID: paidRevision.String()}}}})
	amountRef, timeRef := object, object
	amountRef.MemberID = "amount"
	timeRef.MemberID = "paid_at"
	metric := publish("total", s.Metric, s.KnowledgeSpec{Kind: "aggregate", InputRef: &amountRef, Aggregation: "sum", Unit: "CNY", NullPolicy: "required", TimeAttributeRef: &timeRef})
	dataAmount, dataTime := data, data
	dataAmount.MemberID = "amount"
	dataTime.MemberID = "paid_at"
	publish("model", s.AnalysisModel, s.KnowledgeSpec{Grain: "one fixture row", BaseObjectRef: &object, DefaultTimeAttributeRef: &timeRef, PublicAttributeRefs: []s.KnowledgeReference{timeRef}, MetricRefs: []s.KnowledgeReference{metric}, DataAssetRefs: []s.KnowledgeReference{data}, MemberBindings: []s.MemberBinding{{SemanticRef: amountRef, DataRef: dataAmount}, {SemanticRef: timeRef, DataRef: dataTime}}})
	assetID, _ := identity.ParseAssetID(metric.AssetID)
	dataID, _ := identity.ParseAssetID(data.AssetID)
	return assetID, dataID
}
