package distribution_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/internal/testsupport/knowledgecase"
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
	c := knowledgecase.New()
	queryID := mustID(t, identity.NewSemanticQueryID)
	planID := mustID(t, identity.NewResolvedSemanticPlanID)
	first, refusal, err := distribution.BuildPlan(c.Query, c.Snapshot, queryID, planID, c.Snapshot.Assets, time.Unix(10, 0))
	if err != nil || refusal != nil {
		t.Fatalf("BuildPlan error=%v refusal=%#v", err, refusal)
	}
	selected := append([]distribution.ReleasedAsset(nil), c.Snapshot.Assets...)
	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	second, refusal, err := distribution.BuildPlan(c.Query, c.Snapshot, mustID(t, identity.NewSemanticQueryID),
		mustID(t, identity.NewResolvedSemanticPlanID), selected, time.Unix(20, 0))
	if err != nil || refusal != nil {
		t.Fatalf("second BuildPlan error=%v refusal=%#v", err, refusal)
	}
	if first.PlanDigest != second.PlanDigest || first.ExecutionStatus != "requires_execution_validation" || len(first.Objects) != 3 || first.Model == nil {
		t.Fatalf("plans are not deterministic: first=%#v second=%#v", first, second)
	}
}

func TestBuildPlanDigestIncludesIntentAndTimeRange(t *testing.T) {
	c := knowledgecase.New()
	first, refusal, err := distribution.BuildPlan(c.Query, c.Snapshot, mustID(t, identity.NewSemanticQueryID),
		mustID(t, identity.NewResolvedSemanticPlanID), c.Snapshot.Assets, time.Now())
	if err != nil || refusal != nil {
		t.Fatalf("first plan error=%v refusal=%#v", err, refusal)
	}
	c.Query.TimeRange.From = c.Query.TimeRange.From.AddDate(0, 0, 1)
	second, refusal, err := distribution.BuildPlan(c.Query, c.Snapshot, mustID(t, identity.NewSemanticQueryID),
		mustID(t, identity.NewResolvedSemanticPlanID), c.Snapshot.Assets, time.Now())
	if err != nil || refusal != nil {
		t.Fatalf("second plan error=%v refusal=%#v", err, refusal)
	}
	if first.Intent != distribution.IntentAggregate || first.TimeRange == nil || first.PlanDigest == second.PlanDigest {
		t.Fatalf("time-aware plan digests = %s and %s", first.PlanDigest, second.PlanDigest)
	}
}

func TestBuildPlanRefusesMissingModelOrBinding(t *testing.T) {
	for _, test := range []string{"model", "binding", "execution"} {
		t.Run(test, func(t *testing.T) {
			c := knowledgecase.New()
			switch test {
			case "model":
				c.Snapshot.Assets = c.Snapshot.Assets[:len(c.Snapshot.Assets)-1]
			case "binding":
				c.Snapshot.Bindings = nil
			case "execution":
				c.Snapshot.Execution = nil
			}
			_, refusal, err := distribution.BuildPlan(c.Query, c.Snapshot, mustID(t, identity.NewSemanticQueryID), mustID(t, identity.NewResolvedSemanticPlanID), c.Snapshot.Assets, time.Now())
			if err != nil || refusal == nil || refusal.Code != distribution.RefusalMissingPhysicalBinding {
				t.Fatalf("missing %s refusal=%#v error=%v", test, refusal, err)
			}
		})
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
