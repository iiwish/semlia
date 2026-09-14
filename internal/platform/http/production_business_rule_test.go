package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProductionBusinessRulePayloadStrictShape(t *testing.T) {
	base := `{"expectedVersion":1,"setDigest":"sha256:` + strings.Repeat("a", 64) + `","targetKey":"revenue","action":"confirm","evidenceId":"evd_01arz3ndektsv4rrffq69g5fav"}`
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		{"confirm", base, true},
		{"human declaration", strings.Replace(base, `"evidenceId":"evd_01arz3ndektsv4rrffq69g5fav"`, `"declaration":"Explicit human declaration"`, 1), true},
		{"ambiguous declaration", strings.TrimSuffix(base, "}") + `,"declaration":"must not override evidence"}`, false},
		{"revoke", strings.Replace(strings.Replace(base, `"confirm"`, `"revoke"`, 1), `,"evidenceId":"evd_01arz3ndektsv4rrffq69g5fav"`, "", 1), true},
		{"duplicate", strings.Replace(base, `"action":"confirm"`, `"action":"revoke","action":"confirm"`, 1), false},
		{"null", strings.Replace(base, `"expectedVersion":1`, `"expectedVersion":null`, 1), false},
		{"missing evidence", strings.Replace(base, `,"evidenceId":"evd_01arz3ndektsv4rrffq69g5fav"`, "", 1), false},
		{"forged actor", strings.TrimSuffix(base, "}") + `,"principalId":"forged"}`, false},
		{"revoke with evidence", strings.Replace(base, `"confirm"`, `"revoke"`, 1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var payload productionBusinessRulePayload
			err := json.Unmarshal([]byte(test.body), &payload)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v want=%v error=%v", err == nil, test.valid, err)
			}
		})
	}
}
