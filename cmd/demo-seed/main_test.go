package main

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/iiwish/semlia/internal/demodata"
	sem "github.com/iiwish/semlia/internal/domain/semantic"
)

func TestRemapPreservesPerAssetReleaseAndDoesNotChangeOriginal(t *testing.T) {
	scenario := demodata.New()
	object := scenario.Refs["orders"]
	metric := scenario.Refs["amount"]
	input := map[string]any{"subjectRef": object, "expression": map[string]any{"ref": metric}}
	replacements := map[string]string{object.AssetID: "actual-object", object.RevisionID: "actual-object-revision", "release:" + object.AssetID: "object-release", metric.AssetID: "actual-metric", "release:" + metric.AssetID: "metric-release"}
	result := remap(input, replacements).(map[string]any)
	a := result["subjectRef"].(map[string]any)
	b := result["expression"].(map[string]any)["ref"].(map[string]any)
	if a["assetId"] != "actual-object" || a["revisionId"] != "actual-object-revision" || a["releaseId"] != "object-release" || b["releaseId"] != "metric-release" {
		t.Fatalf("incorrect rebinding: %+v", result)
	}
	if input["subjectRef"].(sem.KnowledgeReference) != object {
		t.Fatal("mutated original fixture")
	}
}

func TestRemapSelectorDoesNotInventRelease(t *testing.T) {
	scenario := demodata.New()
	b, _ := json.Marshal(remap(scenario.Query, map[string]string{scenario.Refs["average"].AssetID: "replacement", "release:" + scenario.Refs["average"].AssetID: "release"}))
	var query map[string]any
	_ = json.Unmarshal(b, &query)
	selector := query["measures"].([]any)[0].(map[string]any)
	if selector["assetId"] != "replacement" {
		t.Fatal("selector not rebound")
	}
	if _, ok := selector["releaseId"]; ok {
		t.Fatal("added release to unversioned selector")
	}
}

func TestSQLLiteralEscapesQuotes(t *testing.T) {
	if literal("don't") != "'don''t'" {
		t.Fatal("SQL literal was not escaped")
	}
}

func TestDemoDeploymentTargetBoundaries(t *testing.T) {
	for _, tc := range []struct {
		dsn        string
		maco, want bool
	}{
		{"postgresql://semlia@127.0.0.1:5433/semlia", false, true},
		{"postgresql://semlia_test_migrator@postgresql:5432/semlia_test", true, true},
		{"postgresql://postgres@postgresql:5432/semlia_test", true, false},
		{"postgresql://semlia_test_migrator@postgresql:5432/other_project", true, false},
		{"postgresql://semlia_test_migrator@postgresql:5432/semlia_test", false, false},
		{"postgresql://semlia@127.0.0.1:5433/semlia", true, false},
	} {
		u, err := url.Parse(tc.dsn)
		if err != nil {
			t.Fatal(err)
		}
		if got := validateTarget(u, tc.maco) == nil; got != tc.want {
			t.Errorf("target %s maco=%v: accepted=%v", tc.dsn, tc.maco, got)
		}
	}
}
