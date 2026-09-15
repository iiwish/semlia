package governance

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func validAgentProposalPayload() string {
	return `{
		"targetObjectType": "semantic_asset",
		"targetObjectId": "ast_01arz3ndektsv4rrffq69g5fav",
		"baseRevisionId": "rev_01arz3ndektsv4rrffq69g5fav",
		"title": "Tighten metric definition",
		"summary": "Clarifies the refunds treatment",
		"reason": "Audit finding AF-22",
		"changeSet": [{
			"fieldPath": "definition",
			"op": "update",
			"beforeDigest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"afterDigest": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"beforeValue": "Revenue after refunds",
			"afterValue": "Revenue after refunds and chargebacks"
		}],
		"agentAttribution": {
			"agentRunId": "agr_01arz3ndektsv4rrffq69g5fav",
			"model": "semlia-test-model",
			"configRevision": "config-7",
			"inputHash": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
		}
	}`
}

func TestValidateAgentProposalInputAcceptsWellFormedPayload(t *testing.T) {
	if err := ValidateAgentProposalInput([]byte(validAgentProposalPayload())); err != nil {
		t.Fatalf("well-formed agent payload rejected: %v", err)
	}
}

func TestValidateAgentProposalInputAcceptsEveryTargetType(t *testing.T) {
	for _, targetType := range []string{"semantic_asset", "physical_binding", "model_grain", "entity_key", "join_contract"} {
		payload := strings.Replace(validAgentProposalPayload(), `"semantic_asset",
		"targetObjectId": "ast_`, fmt.Sprintf(`"%s",
		"targetObjectId": "ast_`, targetType), 1)
		if targetType != "semantic_asset" {
			payload = strings.Replace(payload, `"baseRevisionId": "rev_01arz3ndektsv4rrffq69g5fav",`, "", 1)
		}
		if err := ValidateAgentProposalInput([]byte(payload)); err != nil {
			t.Fatalf("target type %s rejected: %v", targetType, err)
		}
	}
}

func TestValidateAgentProposalInputRejectsMalformedPayloads(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string) string
	}{
		{
			name: "missing required field",
			mutate: func(payload string) string {
				return strings.Replace(payload, `"title": "Tighten metric definition",`, "", 1)
			},
		},
		{
			name: "unknown field",
			mutate: func(payload string) string {
				return strings.Replace(payload, `"targetObjectType": "semantic_asset",`, `"prompt":"you are an agent","targetObjectType": "semantic_asset",`, 1)
			},
		},
		{
			name: "wrong type",
			mutate: func(payload string) string {
				return strings.Replace(payload, `"changeSet": [{`, `"changeSet": "one item", "items": [{"deleted": true, `, 1)
			},
		},
		{
			name:   "unknown change-set op",
			mutate: func(payload string) string { return strings.Replace(payload, `"op": "update"`, `"op": "replace"`, 1) },
		},
		{
			name: "digest without sha256 prefix",
			mutate: func(payload string) string {
				return strings.Replace(payload, "afterDigest\": \"sha256:", "afterDigest\": \"md5:", 1)
			},
		},
		{
			name:   "add item carrying a before side",
			mutate: func(payload string) string { return strings.Replace(payload, `"op": "update"`, `"op": "add"`, 1) },
		},
		{
			name:   "ungoverned target id prefix",
			mutate: func(payload string) string { return strings.Replace(payload, "ast_01arz", "run_01arz", 1) },
		},
		{
			name:   "missing agent attribution",
			mutate: func(payload string) string { return strings.Replace(payload, `"agentAttribution"`, `"attribution"`, 1) },
		},
		{
			name: "semantic asset target without base revision",
			mutate: func(payload string) string {
				return strings.Replace(payload, `"baseRevisionId": "rev_01arz3ndektsv4rrffq69g5fav",`, "", 1)
			},
		},
		{
			name: "attribution missing input hash",
			mutate: func(payload string) string {
				return strings.Replace(payload, `"inputHash": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"`, `"inputHash": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccX"`, 1)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateAgentProposalInput([]byte(test.mutate(validAgentProposalPayload())))
			if err == nil {
				t.Fatal("malformed agent payload accepted")
			}
			if !errors.Is(err, ErrAIOutputInvalid) {
				t.Fatalf("error = %v, want ErrAIOutputInvalid", err)
			}
			var invalid *AIOutputInvalidError
			if !errors.As(err, &invalid) || len(invalid.Violations) == 0 {
				t.Fatalf("violations missing: %v", err)
			}
		})
	}
}

func TestValidateAgentProposalInputRejectsNonJSON(t *testing.T) {
	if err := ValidateAgentProposalInput([]byte("not json at all")); !errors.Is(err, ErrAIOutputInvalid) {
		t.Fatalf("error = %v, want ErrAIOutputInvalid", err)
	}
}

func TestAgentPayloadHasAttribution(t *testing.T) {
	if !AgentPayloadHasAttribution([]byte(validAgentProposalPayload())) {
		t.Fatal("attributed payload not detected")
	}
	for _, payload := range []string{
		`{"title":"Human draft","changeSet":[{"fieldPath":"a","op":"add","afterDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","afterValue":"x"}],"createdBy":"founder"}`,
		`{"agentAttribution":null}`,
		`not json`,
	} {
		if AgentPayloadHasAttribution([]byte(payload)) {
			t.Fatalf("payload detected as attributed: %s", payload)
		}
	}
}
