package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFixtureKeepsFiveTypesAndIndependentSyntheticData(t *testing.T) {
	a, b := buildFixture(), buildFixture()
	if _, err := json.Marshal(a); err != nil {
		t.Fatal("fixture must serialize completely", err)
	}
	if a.DataSQL != b.DataSQL || strings.Count(a.DataSQL, "INSERT INTO") != 11 {
		t.Fatal("source data must remain fixed independently of minted asset identities")
	}
	if len(a.Snapshot.Assets) != 10 || len(a.Snapshot.Bindings) != 2 || len(a.Snapshot.Execution.Joins) != 1 {
		t.Fatal("incomplete five-type governed fixture")
	}
	kinds := map[string]bool{}
	for _, asset := range a.Snapshot.Assets {
		kinds[string(asset.AssetType)] = true
		var content map[string]any
		if err := json.Unmarshal(asset.Content, &content); err != nil {
			t.Fatal(err)
		}
		definition := content["definition"].(string)
		if !strings.Contains(definition, "合成") || strings.Contains(definition, "600 / 3") || strings.Contains(definition, "200 元") {
			t.Fatal("knowledge must disclose synthesis without verifier-side answers")
		}
	}
	if len(kinds) != 5 {
		t.Fatal("missing knowledge type")
	}
}
