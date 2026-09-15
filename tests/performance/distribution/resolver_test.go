package distribution_test

import (
	"fmt"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

const resolverFixtureSize = 10_000

func TestResolver10000AssetP95(t *testing.T) {
	assets := make([]distribution.ReleasedAsset, 0, resolverFixtureSize)
	for index := 0; index < resolverFixtureSize; index++ {
		assetID, err := identity.NewAssetID()
		if err != nil {
			t.Fatal(err)
		}
		revisionID, err := identity.NewRevisionID()
		if err != nil {
			t.Fatal(err)
		}
		assets = append(assets, distribution.ReleasedAsset{AssetID: assetID, RevisionID: revisionID,
			Address: fmt.Sprintf("warehouse.metric_%05d", index), Name: fmt.Sprintf("Warehouse metric %05d", index),
			AssetType: semantic.Metric, Position: index + 1})
	}
	target := assets[len(assets)-1]
	datasetID, _ := identity.NewPhysicalDatasetID()
	bindingID, _ := identity.NewPhysicalBindingID()
	releaseID, _ := identity.NewReleaseID()
	workspaceID, _ := identity.NewWorkspaceID()
	snapshot := distribution.ReleaseSnapshot{ReleaseID: releaseID, WorkspaceID: workspaceID, Assets: assets,
		Bindings: []distribution.ReleasedPhysicalBinding{{ID: bindingID, Version: 1, AssetID: target.AssetID, DatasetID: datasetID}}}
	query := distribution.SemanticQueryInput{SchemaVersion: distribution.QuerySchemaVersion, Intent: distribution.IntentAggregate,
		Measures: []distribution.Selector{{Address: target.Address}}, Context: distribution.ResolutionContext{Mode: distribution.ResolutionCurrent}}

	measure := func() error {
		matched, refusal := distribution.MatchSelector(query.Measures[0], "measure", snapshot.Assets)
		if refusal != nil || matched.Asset.AssetID != target.AssetID {
			return fmt.Errorf("golden selector mismatch")
		}
		queryID, _ := identity.NewSemanticQueryID()
		planID, _ := identity.NewResolvedSemanticPlanID()
		plan, refusal, err := distribution.BuildPlan(query, snapshot, queryID, planID, []distribution.ReleasedAsset{matched.Asset}, time.Now())
		if err != nil || refusal != nil || plan.PlanDigest == "" {
			return fmt.Errorf("golden plan mismatch: refusal=%v err=%v", refusal, err)
		}
		return nil
	}
	if err := measure(); err != nil {
		t.Fatal(err)
	}
	samples := make([]time.Duration, 0, 25)
	for index := 0; index < 25; index++ {
		started := time.Now()
		if err := measure(); err != nil {
			t.Fatal(err)
		}
		samples = append(samples, time.Since(started))
	}
	sort.Slice(samples, func(left, right int) bool { return samples[left] < samples[right] })
	p95 := samples[23]
	t.Logf("semantic resolver benchmark: host=%s/%s assets=%d samples=25 p50=%s p95=%s",
		runtime.GOOS, runtime.GOARCH, resolverFixtureSize, samples[12], p95)
	if p95 >= time.Second {
		t.Fatalf("10,000-asset resolver p95 = %s, budget < 1s", p95)
	}
}
