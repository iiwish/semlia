package distribution_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestSemanticQueryValidationUsesSpecificStableCodes(t *testing.T) {
	query := validQuery(t)
	query.Limit = 10001
	assertValidationCode(t, query.Validate(), distribution.RefusalInvalidLimit)

	query = validQuery(t)
	query.Filters = []distribution.Filter{{Selector: distribution.Selector{Search: "country"}, Operator: "sql", Value: json.RawMessage(`"CN"`)}}
	assertValidationCode(t, query.Validate(), distribution.RefusalInvalidFilter)

	query = validQuery(t)
	query.TimeRange = &distribution.TimeRange{Selector: distribution.Selector{Search: "date"},
		From: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	assertValidationCode(t, query.Validate(), distribution.RefusalInvalidTimeRange)
}

func TestSemanticQueryCanonicalDigestIsStable(t *testing.T) {
	query := validQuery(t)
	first, firstDigest, err := query.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	second, secondDigest, err := query.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || firstDigest != secondDigest {
		t.Fatalf("canonical query changed: %s %s", firstDigest, secondDigest)
	}
}

func TestMatchSelectorPrefersStableReferencesAndRefusesMaterialTie(t *testing.T) {
	assets := []distribution.ReleasedAsset{
		asset(t, "commerce.net_revenue", "Net revenue", semantic.Metric),
		asset(t, "finance.net_revenue", "Net revenue", semantic.Metric),
	}
	matched, refusal := distribution.MatchSelector(distribution.Selector{Address: assets[1].Address}, "measure", assets)
	if refusal != nil || matched.Asset.AssetID != assets[1].AssetID {
		t.Fatalf("explicit address match = %#v refusal=%#v", matched, refusal)
	}
	matched, refusal = distribution.MatchSelector(distribution.Selector{Search: "Net revenue"}, "measure", assets)
	if refusal == nil || refusal.Code != distribution.RefusalAmbiguousMatch || len(matched.Candidates) != 2 {
		t.Fatalf("ambiguous match = %#v refusal=%#v", matched, refusal)
	}
}

func TestBuildPlanPinsObjectsAndDigestDeterministically(t *testing.T) {
	query := validQuery(t)
	metric := asset(t, "commerce.net_revenue", "Net revenue", semantic.Metric)
	dimension := asset(t, "commerce.country", "Country", semantic.Entity)
	dataset := mustID(t, identity.NewPhysicalDatasetID)
	snapshot := releaseSnapshot(t, []distribution.ReleasedAsset{metric, dimension})
	snapshot.Bindings = []distribution.ReleasedPhysicalBinding{
		{ID: mustID(t, identity.NewPhysicalBindingID), Version: 2, AssetID: metric.AssetID, DatasetID: dataset},
		{ID: mustID(t, identity.NewPhysicalBindingID), Version: 1, AssetID: dimension.AssetID, DatasetID: dataset},
	}
	snapshot.Keys = []distribution.ReleasedEntityKey{{ID: mustID(t, identity.NewEntityKeyID), Version: 3, AssetID: dimension.AssetID}}
	queryID := mustID(t, identity.NewSemanticQueryID)
	planID := mustID(t, identity.NewResolvedSemanticPlanID)
	first, refusal, err := distribution.BuildPlan(query, snapshot, queryID, planID, []distribution.ReleasedAsset{metric, dimension}, time.Unix(10, 0))
	if err != nil || refusal != nil {
		t.Fatalf("BuildPlan error=%v refusal=%#v", err, refusal)
	}
	second, refusal, err := distribution.BuildPlan(query, snapshot, mustID(t, identity.NewSemanticQueryID),
		mustID(t, identity.NewResolvedSemanticPlanID), []distribution.ReleasedAsset{dimension, metric}, time.Unix(20, 0))
	if err != nil || refusal != nil {
		t.Fatalf("second BuildPlan error=%v refusal=%#v", err, refusal)
	}
	if first.PlanDigest != second.PlanDigest || first.ExecutionStatus != "not_configured" || len(first.Objects) != 3 {
		t.Fatalf("plans are not deterministic: first=%#v second=%#v", first, second)
	}
}

func TestBuildPlanDigestIncludesIntentAndTimeRange(t *testing.T) {
	metric := asset(t, "commerce.net_revenue", "Net revenue", semantic.Metric)
	timeDimension := asset(t, "commerce.order_date", "Order date", semantic.Dimension)
	dataset := mustID(t, identity.NewPhysicalDatasetID)
	snapshot := releaseSnapshot(t, []distribution.ReleasedAsset{metric, timeDimension})
	snapshot.Bindings = []distribution.ReleasedPhysicalBinding{
		{ID: mustID(t, identity.NewPhysicalBindingID), Version: 1, AssetID: metric.AssetID, DatasetID: dataset},
		{ID: mustID(t, identity.NewPhysicalBindingID), Version: 1, AssetID: timeDimension.AssetID, DatasetID: dataset},
	}
	query := distribution.SemanticQueryInput{SchemaVersion: distribution.QuerySchemaVersion, Intent: distribution.IntentAggregate,
		Measures: []distribution.Selector{{Address: metric.Address}}, Context: distribution.ResolutionContext{Mode: distribution.ResolutionCurrent},
		TimeRange: &distribution.TimeRange{Selector: distribution.Selector{Address: timeDimension.Address},
			From: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Granularity: "day"}}
	first, refusal, err := distribution.BuildPlan(query, snapshot, mustID(t, identity.NewSemanticQueryID),
		mustID(t, identity.NewResolvedSemanticPlanID), []distribution.ReleasedAsset{metric, timeDimension}, time.Now())
	if err != nil || refusal != nil {
		t.Fatalf("first plan error=%v refusal=%#v", err, refusal)
	}
	query.TimeRange.From = query.TimeRange.From.AddDate(0, 0, 1)
	second, refusal, err := distribution.BuildPlan(query, snapshot, mustID(t, identity.NewSemanticQueryID),
		mustID(t, identity.NewResolvedSemanticPlanID), []distribution.ReleasedAsset{metric, timeDimension}, time.Now())
	if err != nil || refusal != nil {
		t.Fatalf("second plan error=%v refusal=%#v", err, refusal)
	}
	if first.Intent != distribution.IntentAggregate || first.TimeRange == nil || first.PlanDigest == second.PlanDigest {
		t.Fatalf("time-aware plan digests = %s and %s", first.PlanDigest, second.PlanDigest)
	}
}

func TestBuildPlanRefusesMissingBindingJoinAndManyToManyGrain(t *testing.T) {
	query := validQuery(t)
	metric := asset(t, "commerce.net_revenue", "Net revenue", semantic.Metric)
	dimension := asset(t, "commerce.country", "Country", semantic.Dimension)
	snapshot := releaseSnapshot(t, []distribution.ReleasedAsset{metric, dimension})
	_, refusal, err := distribution.BuildPlan(query, snapshot, mustID(t, identity.NewSemanticQueryID),
		mustID(t, identity.NewResolvedSemanticPlanID), []distribution.ReleasedAsset{metric, dimension}, time.Now())
	if err != nil || refusal == nil || refusal.Code != distribution.RefusalMissingPhysicalBinding {
		t.Fatalf("missing binding refusal=%#v error=%v", refusal, err)
	}

	left, right := mustID(t, identity.NewPhysicalDatasetID), mustID(t, identity.NewPhysicalDatasetID)
	snapshot.Bindings = []distribution.ReleasedPhysicalBinding{
		{ID: mustID(t, identity.NewPhysicalBindingID), Version: 1, AssetID: metric.AssetID, DatasetID: left},
		{ID: mustID(t, identity.NewPhysicalBindingID), Version: 1, AssetID: dimension.AssetID, DatasetID: right},
	}
	_, refusal, err = distribution.BuildPlan(query, snapshot, mustID(t, identity.NewSemanticQueryID),
		mustID(t, identity.NewResolvedSemanticPlanID), []distribution.ReleasedAsset{metric, dimension}, time.Now())
	if err != nil || refusal == nil || refusal.Code != distribution.RefusalMissingJoinPath {
		t.Fatalf("missing join refusal=%#v error=%v", refusal, err)
	}
	snapshot.Joins = []distribution.ReleasedJoinContract{{ID: mustID(t, identity.NewJoinContractID), Version: 1,
		LeftDatasetID: left, RightDatasetID: right, Cardinality: governance.CardinalityManyToMany, JoinType: governance.JoinInner}}
	_, refusal, err = distribution.BuildPlan(query, snapshot, mustID(t, identity.NewSemanticQueryID),
		mustID(t, identity.NewResolvedSemanticPlanID), []distribution.ReleasedAsset{metric, dimension}, time.Now())
	if err != nil || refusal == nil || refusal.Code != distribution.RefusalIncompatibleGrain {
		t.Fatalf("grain refusal=%#v error=%v", refusal, err)
	}
}

func validQuery(t *testing.T) distribution.SemanticQueryInput {
	t.Helper()
	return distribution.SemanticQueryInput{SchemaVersion: distribution.QuerySchemaVersion,
		Intent: distribution.IntentBreakdown, Measures: []distribution.Selector{{Search: "net revenue"}},
		Dimensions: []distribution.Selector{{Search: "country"}}, Context: distribution.ResolutionContext{Mode: distribution.ResolutionCurrent}, Limit: 100}
}

func asset(t *testing.T, address, name string, assetType semantic.AssetType) distribution.ReleasedAsset {
	t.Helper()
	return distribution.ReleasedAsset{AssetID: mustID(t, identity.NewAssetID), RevisionID: mustID(t, identity.NewRevisionID),
		Address: address, Name: name, AssetType: assetType, ContentDigest: "sha256:test", Content: json.RawMessage(`{}`)}
}

func releaseSnapshot(t *testing.T, assets []distribution.ReleasedAsset) distribution.ReleaseSnapshot {
	t.Helper()
	return distribution.ReleaseSnapshot{ReleaseID: mustID(t, identity.NewReleaseID), WorkspaceID: mustID(t, identity.NewWorkspaceID),
		Sequence: 1, ManifestDigest: "sha256:release", Assets: assets}
}

func assertValidationCode(t *testing.T, err error, want distribution.RefusalCode) {
	t.Helper()
	var validation *distribution.ValidationError
	if !errors.As(err, &validation) || validation.Code != want {
		t.Fatalf("validation error=%v, want %s", err, want)
	}
}

func mustID[T any](t *testing.T, factory func() (T, error)) T {
	t.Helper()
	value, err := factory()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
