package postgres

import (
	"bytes"
	"encoding/json"

	domain "github.com/iiwish/semlia/internal/domain/governance"
)

// Registry structural columns, not its extensible content alone, are authority.
func productionObjectBusinessBaseline(kind string, payload map[string]json.RawMessage) (json.RawMessage, error) {
	content, err := domain.ResolveLocalReferences(payload["content"], nil)
	if err != nil {
		return nil, domain.ErrPriorStateUnknown
	}
	var business map[string]json.RawMessage
	if json.Unmarshal(content, &business) != nil || business == nil {
		return nil, domain.ErrPriorStateUnknown
	}
	ids := map[string]string{}
	values := map[string]string{}
	switch kind {
	case domain.TargetKindPhysicalBinding:
		ids = map[string]string{"asset": "asset_id", "dataset": "dataset_id", "field": "field_id"}
		values = map[string]string{"transform": "transform"}
	case domain.TargetKindModelGrain:
		ids = map[string]string{"asset": "asset_id"}
		values = map[string]string{"expression": "grain_expression", "fields": "grain_field_refs"}
	case domain.TargetKindEntityKey:
		ids = map[string]string{"asset": "asset_id"}
		values = map[string]string{"fields": "key_field_refs", "uniqueness": "uniqueness_semantics"}
	case domain.TargetKindJoinContract:
		ids = map[string]string{"leftDataset": "left_dataset_id", "rightDataset": "right_dataset_id"}
		values = map[string]string{"joinType": "join_type", "cardinality": "cardinality", "expression": "join_expression", "notes": "contract_notes"}
		var pairs []struct {
			Left  string `json:"left"`
			Right string `json:"right"`
		}
		if json.Unmarshal(business["pairs"], &pairs) != nil || len(pairs) == 0 {
			return nil, domain.ErrPriorStateUnknown
		}
		left, right := []string{}, []string{}
		for _, pair := range pairs {
			left, right = append(left, pair.Left), append(right, pair.Right)
		}
		for column, fields := range map[string][]string{"left_field_refs": left, "right_field_refs": right} {
			expected, _ := json.Marshal(fields)
			actual, err := domain.CanonicalJSON(payload[column])
			if err != nil || !bytes.Equal(expected, actual) {
				return nil, domain.ErrPriorStateUnknown
			}
		}
	default:
		return nil, domain.ErrInvalidArgument
	}
	for field, column := range ids {
		actual := payload[column]
		value, present := business[field]
		if (!present || bytes.Equal(value, []byte("null"))) && bytes.Equal(actual, []byte("null")) {
			continue
		}
		var expectedID, actualID string
		if json.Unmarshal(value, &expectedID) != nil || json.Unmarshal(actual, &actualID) != nil {
			return nil, domain.ErrPriorStateUnknown
		}
		expectedUUID, e1 := parseUUIDOrTypeID(expectedID)
		actualUUID, e2 := parseUUIDOrTypeID(actualID)
		if e1 != nil || e2 != nil || expectedUUID != actualUUID {
			return nil, domain.ErrPriorStateUnknown
		}
	}
	for field, column := range values {
		expected, present := business[field]
		if !present {
			expected = []byte("null")
		}
		expected, e1 := domain.CanonicalJSON(expected)
		actual, e2 := domain.CanonicalJSON(payload[column])
		if e1 != nil || e2 != nil || !bytes.Equal(expected, actual) {
			return nil, domain.ErrPriorStateUnknown
		}
	}
	return content, nil
}
