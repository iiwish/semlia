package governance_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/iiwish/semlia/internal/domain/governance"
)

func TestValidateTargetDeclarations(t *testing.T) {
	t.Run("valid create targets", func(t *testing.T) {
		targets := []governance.TargetDeclaration{
			{
				LocalKey: "order_asset",
				Kind:     governance.TargetKindSemanticAsset,
				Intent:   governance.ProductionIntentCreate,
				Content:  json.RawMessage(`{"name":"Orders","definition":"Core order"}`),
			},
			{
				LocalKey: "order_binding",
				Kind:     governance.TargetKindPhysicalBinding,
				Intent:   governance.ProductionIntentCreate,
				Content:  json.RawMessage(`{"asset":"local:order_asset","dataset":"pds_01"}`),
			},
		}
		if err := governance.ValidateTargetDeclarations(targets); err != nil {
			t.Fatalf("expected valid targets, got: %v", err)
		}
	})

	t.Run("empty targets rejected", func(t *testing.T) {
		err := governance.ValidateTargetDeclarations(nil)
		if !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("expected ErrInvalidArgument, got: %v", err)
		}
	})

	t.Run("duplicate localKey rejected", func(t *testing.T) {
		targets := []governance.TargetDeclaration{
			{LocalKey: "order", Kind: governance.TargetKindSemanticAsset, Intent: governance.ProductionIntentCreate, Content: json.RawMessage(`{}`)},
			{LocalKey: "order", Kind: governance.TargetKindModelGrain, Intent: governance.ProductionIntentCreate, Content: json.RawMessage(`{}`)},
		}
		err := governance.ValidateTargetDeclarations(targets)
		if !errors.Is(err, governance.ErrInvalidArgument) || !strings.Contains(err.Error(), "duplicate localKey") {
			t.Fatalf("expected duplicate localKey error, got: %v", err)
		}
	})

	t.Run("exceeding max targets rejected", func(t *testing.T) {
		targets := make([]governance.TargetDeclaration, 33)
		for i := 0; i < 33; i++ {
			targets[i] = governance.TargetDeclaration{
				LocalKey: string(rune('a'+i%26)) + string(rune('0'+i)),
				Kind:     governance.TargetKindSemanticAsset,
				Intent:   governance.ProductionIntentCreate,
				Content:  json.RawMessage(`{}`),
			}
		}
		err := governance.ValidateTargetDeclarations(targets)
		if !errors.Is(err, governance.ErrLimitExceeded) {
			t.Fatalf("expected ErrLimitExceeded, got: %v", err)
		}
	})

	t.Run("exceeding max changes rejected", func(t *testing.T) {
		changes := make([]governance.TargetChangeInput, 101)
		for i := 0; i < 101; i++ {
			changes[i] = governance.TargetChangeInput{FieldPath: "definition", Op: "update"}
		}
		targets := []governance.TargetDeclaration{
			{
				LocalKey: "order",
				Kind:     governance.TargetKindSemanticAsset,
				Intent:   governance.ProductionIntentUpdate,
				Changes:  changes,
			},
		}
		err := governance.ValidateTargetDeclarations(targets)
		if !errors.Is(err, governance.ErrLimitExceeded) {
			t.Fatalf("expected ErrLimitExceeded, got: %v", err)
		}
	})
}

func TestValidateCandidateDeclarations(t *testing.T) {
	targets := []governance.TargetDeclaration{
		{LocalKey: "order", Kind: governance.TargetKindSemanticAsset, Intent: governance.ProductionIntentCreate, Content: json.RawMessage(`{}`)},
	}

	t.Run("valid candidate link", func(t *testing.T) {
		candidates := []governance.CandidateDeclaration{
			{
				CandidateID:      "cand_1",
				CandidateDigest:  "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				TargetKeys:       []string{"order"},
				PrimaryTargetKey: "order",
			},
		}
		if err := governance.ValidateCandidateDeclarations(candidates, targets); err != nil {
			t.Fatalf("expected valid candidate link, got: %v", err)
		}
	})

	t.Run("missing primaryTargetKey rejected", func(t *testing.T) {
		candidates := []governance.CandidateDeclaration{
			{
				CandidateID:     "cand_1",
				CandidateDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				TargetKeys:      []string{"order"},
			},
		}
		if err := governance.ValidateCandidateDeclarations(candidates, targets); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("expected ErrInvalidArgument, got: %v", err)
		}
	})

	t.Run("unknown targetKey rejected", func(t *testing.T) {
		candidates := []governance.CandidateDeclaration{
			{
				CandidateID:      "cand_1",
				CandidateDigest:  "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				TargetKeys:       []string{"unknown_target"},
				PrimaryTargetKey: "unknown_target",
			},
		}
		if err := governance.ValidateCandidateDeclarations(candidates, targets); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("expected ErrInvalidArgument, got: %v", err)
		}
	})
}

func TestResolveLocalReferences(t *testing.T) {
	mapping := map[string]string{
		"order":    "ast_01h7v8",
		"customer": "ast_01h7v9",
	}

	raw := json.RawMessage(`{
		"binding": {"localKey":"order"},
		"items": [{"localKey":"customer"}, "regular-value"],
		"nested": {
			"target": {"localKey":"order"}
		}
	}`)

	resolved, err := governance.ResolveLocalReferences(raw, mapping)
	if err != nil {
		t.Fatalf("unexpected error resolving references: %v", err)
	}

	var data struct {
		Binding string   `json:"binding"`
		Items   []string `json:"items"`
		Nested  struct {
			Target string `json:"target"`
		} `json:"nested"`
	}
	if err := json.Unmarshal(resolved, &data); err != nil {
		t.Fatalf("unmarshal resolved json: %v", err)
	}

	if data.Binding != "ast_01h7v8" || data.Items[0] != "ast_01h7v9" || data.Items[1] != "regular-value" || data.Nested.Target != "ast_01h7v8" {
		t.Fatalf("unexpected resolved values: %+v", data)
	}

	// Unresolvable key returns error
	rawUnresolvable := json.RawMessage(`{"ref": {"localKey":"missing_key"}}`)
	if _, err := governance.ResolveLocalReferences(rawUnresolvable, mapping); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument on missing local ref, got: %v", err)
	}
}

func TestComputeBusinessDigest(t *testing.T) {
	d1 := governance.ComputeBusinessDigest("wsp_1", "sha256:input1", []string{"sha256:t2", "sha256:t1"})
	d2 := governance.ComputeBusinessDigest("wsp_1", "sha256:input1", []string{"sha256:t1", "sha256:t2"})
	if d1 != d2 {
		t.Fatalf("business digest should be order-insensitive for targets: %s vs %s", d1, d2)
	}
	if !strings.HasPrefix(d1, "sha256:") {
		t.Fatalf("invalid digest prefix: %s", d1)
	}
}
