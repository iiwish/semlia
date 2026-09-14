package postgres

import (
	"encoding/json"
	domain "github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/pkg/identity"
	"testing"
)

func TestExecutionAcceptsGovernedTypeIDFieldRefs(t *testing.T) {
	field, _ := identity.NewPhysicalFieldID()
	for _, ref := range []string{field.String(), field.UUID()} {
		parsed, err := typedField(ref)
		if err != nil || parsed != field {
			t.Fatalf("formal governed field ref rejected: %v", err)
		}
	}
}

func TestExecutionJoinUsesAllFormalRefsNotContentOverride(t *testing.T) {
	left, _ := identity.NewPhysicalFieldID()
	tenant, _ := identity.NewPhysicalFieldID()
	right, _ := identity.NewPhysicalFieldID()
	otherTenant, _ := identity.NewPhysicalFieldID()
	joinID, _ := identity.NewJoinContractID()
	ld, _ := identity.NewPhysicalDatasetID()
	rd, _ := identity.NewPhysicalDatasetID()
	join := domain.ReleasedJoinContract{ID: joinID, Version: 1, LeftDatasetID: ld, RightDatasetID: rd}
	raw, _ := json.Marshal(map[string]any{"left_field_refs": []string{left.String(), tenant.String()}, "right_field_refs": []string{right.String(), otherTenant.String()}, "join_expression": "field_pairs_equal/v1", "content": map[string]any{"execution": map[string]any{"fieldPairs": []domain.ExecutionFieldPair{{LeftFieldID: left.UUID(), RightFieldID: right.UUID()}}}}})
	got, err := executionJoinFromSnapshot(join, raw)
	if err != nil || len(got.FieldPairs) != 2 || got.FieldPairs[1].LeftFieldID != tenant.UUID() || got.FieldPairs[1].RightFieldID != otherTenant.UUID() {
		t.Fatalf("formal composite join replaced by content: %+v %v", got, err)
	}
	if got.Expression != "field_pairs_equal/v1" {
		t.Fatal("formal expression lost")
	}
}
