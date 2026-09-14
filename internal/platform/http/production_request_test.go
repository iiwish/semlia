package httpapi

import (
	"encoding/json"
	"testing"
)

func TestProductionRequestRequiredArrays(t *testing.T) {
	valid := `{"input":{"snapshots":[],"candidates":[],"evidence":[],"dependencies":[]},"targets":[{"content":{},"changes":[],"evidenceIds":[]}]}`
	if err := validateProductionRequestShape([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"snapshots", "candidates", "evidence", "dependencies", "changes", "evidenceIds", "content"} {
		for _, null := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "-missing", true: "-null"}[null], func(t *testing.T) {
				var body map[string]any
				if err := json.Unmarshal([]byte(valid), &body); err != nil {
					t.Fatal(err)
				}
				object := body["input"].(map[string]any)
				if name == "changes" || name == "evidenceIds" || name == "content" {
					object = body["targets"].([]any)[0].(map[string]any)
				}
				if null {
					object[name] = nil
				} else {
					delete(object, name)
				}
				raw, _ := json.Marshal(body)
				if err := validateProductionRequestShape(raw); err == nil {
					t.Fatal("accepted missing or null required field")
				}
			})
		}
	}
}
