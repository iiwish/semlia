package governance

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func productionSuggestionFixture(t *testing.T) []byte {
	t.Helper()
	owner, err := identity.NewPrincipalID()
	if err != nil {
		t.Fatal(err)
	}
	return []byte(`{"schemaVersion":"semlia.production-suggestions/v1","targets":[{"localKey":"orders","title":"Orders","kind":"semantic_asset","intent":"create","identityKey":"sales.orders","evidenceIds":[],"changes":[],"content":{"address":"sales.orders","assetType":"entity","displayName":"Orders","definition":null,"scope":null,"ownerPrincipalId":"` + owner.String() + `"}}]}`)
}

func TestProductionGenerationOutputGate(t *testing.T) {
	raw := productionSuggestionFixture(t)
	output, canonical, err := GateProductionGenerationOutput(raw)
	if err != nil || len(output.Targets) != 1 || !json.Valid(canonical) {
		t.Fatalf("valid unresolved suggestion: %s %v", canonical, err)
	}
	for name, invalid := range map[string][]byte{
		"unknown actor":  []byte(strings.Replace(string(raw), `"targets":`, `"createdBy":"admin","targets":`, 1)),
		"duplicate key":  []byte(strings.Replace(string(raw), `"title":"Orders"`, `"title":"ignored","title":"Orders"`, 1)),
		"unknown target": []byte(strings.Replace(string(raw), `"title":"Orders"`, `"title":"Orders","approved":true`, 1)),
		"wrong schema":   []byte(strings.Replace(string(raw), `suggestions/v1`, `suggestions/v2`, 1)),
		"missing array":  []byte(strings.Replace(string(raw), `"evidenceIds":[],`, ``, 1)),
		"fake owner":     []byte(strings.Replace(string(raw), `"ownerPrincipalId":"prn_`, `"ownerPrincipalId":"ast_`, 1)),
		"null content":   []byte(`{"schemaVersion":"semlia.production-suggestions/v1","targets":[{"localKey":"orders","title":"Orders","kind":"semantic_asset","intent":"create","identityKey":"sales.orders","evidenceIds":[],"changes":[],"content":null}]}`),
		"markdown":       []byte("```json\n" + string(raw) + "\n```"),
		"trailing":       append(append([]byte{}, raw...), []byte(` {}`)...),
		"oversize":       []byte(strings.Repeat(" ", (1<<20)+1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := GateProductionGenerationOutput(invalid); !errors.Is(err, ErrAIOutputInvalid) {
				t.Fatalf("must reject: %v", err)
			}
		})
	}
}

func TestProductionGenerationBudgetFailsClosed(t *testing.T) {
	grant := domain.ProductionGenerationGrant{MaxOutputTokens: 100, MaxInputBytes: 1000, MaxCostMicros: 3000, InputMicrosPerByte: 2, OutputMicrosPerToken: 3}
	if err := grant.CheckBudget(1000, 100, 2300); err != nil {
		t.Fatal(err)
	}
	for _, values := range [][3]int64{{1001, 100, 3000}, {1000, 101, 3000}, {1000, 100, 2299}, {1000, 100, 0}, {-1, 10, 100}, {1, -1, 100}} {
		if err := grant.CheckBudget(int(values[0]), int(values[1]), values[2]); err == nil {
			t.Fatalf("budget accepted %v", values)
		}
	}
	if err := (domain.ProductionGenerationGrant{}).CheckBudget(1, 1, 1000); err == nil {
		t.Fatal("unconfigured grant accepted")
	}
}
