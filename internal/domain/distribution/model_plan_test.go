package distribution

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestModelPlanRefusesMissingOrAmbiguousWholeDemand(t *testing.T) {
	metric, _ := identity.NewAssetID()
	revision, _ := identity.NewRevisionID()
	release, _ := identity.NewReleaseID()
	asset := ReleasedAsset{AssetID: metric, RevisionID: revision, Address: "sales.total", AssetType: semantic.Metric}
	query := SemanticQueryInput{Intent: IntentAggregate, Measures: []Selector{{AssetID: &metric}}}
	snapshot := ReleaseSnapshot{ReleaseID: release, Assets: []ReleasedAsset{asset}, Execution: &ExecutionProvenance{CompilerVersion: "postgres-aggregate/v1"}}
	_, refusal, _ := buildKnowledgePlan(query, snapshot, identity.SemanticQueryID{}, identity.ResolvedSemanticPlanID{}, nil, time.Now())
	if refusal == nil {
		t.Fatal("query without an analysis model accepted")
	}
	for i := 0; i < 2; i++ {
		id, _ := identity.NewAssetID()
		raw, _ := json.Marshal(map[string]any{"spec": semantic.KnowledgeSpec{MetricRefs: []semantic.KnowledgeReference{{AssetID: metric.String(), RevisionID: revision.String(), ReleaseID: release.String()}}}})
		snapshot.Assets = append(snapshot.Assets, ReleasedAsset{AssetID: id, AssetType: semantic.AnalysisModel, Content: raw})
	}
	_, refusal, _ = buildKnowledgePlan(query, snapshot, identity.SemanticQueryID{}, identity.ResolvedSemanticPlanID{}, nil, time.Now())
	if refusal == nil {
		t.Fatal("incomplete models were accepted")
	}
}
