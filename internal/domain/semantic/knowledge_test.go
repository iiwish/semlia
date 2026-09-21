package semantic

import (
	"encoding/json"
	"testing"
)

func TestKnowledgeEnumsRejectPartialTokens(t *testing.T) {
	if _, err := ParseKnowledgeSpec(BusinessTerm, []byte(`{"capability":"definition predicate"}`), false); err == nil {
		t.Fatal("combined enum token accepted")
	}
}

func TestKnowledgeSpecDraftAndRelease(t *testing.T) {
	for _, kind := range []AssetType{"business_object", "business_term", "metric", "data_asset", "analysis_model"} {
		if _, err := ParseKnowledgeSpec(kind, json.RawMessage(`null`), false); err != nil {
			t.Fatal(kind, err)
		}
		if _, err := ParseKnowledgeSpec(kind, json.RawMessage(`null`), true); err == nil {
			t.Fatal("released empty spec", kind)
		}
	}
	for _, kind := range []AssetType{"entity", "concept", "dimension", "measure", "segment", "semantic_model"} {
		if _, err := ParseKnowledgeSpec(kind, json.RawMessage(`null`), false); err == nil {
			t.Fatal("legacy type accepted", kind)
		}
	}
}

func TestKnowledgeObjectMembers(t *testing.T) {
	valid := `{"grain":"one customer","keys":["customer_id"],"identityPolicy":"canonical customer ID","lifecycle":"active or closed","members":[{"id":"customer_id","name":"Customer ID","valueType":"string","nullPolicy":"required","historyPolicy":"stable"}]}`
	if _, err := ParseKnowledgeSpec("business_object", json.RawMessage(valid), true); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		`{"sql":"select * from customers"}`,
		`{"members":[{"id":"id"},{"id":"id"}]}`,
		`{"keys":["missing"],"members":[{"id":"id"}]}`,
		`{"members":[{"id":"ID with spaces"}]}`,
		`{"kind":"derived"}`,
	} {
		if _, err := ParseKnowledgeSpec("business_object", json.RawMessage(invalid), false); err == nil {
			t.Fatal("invalid spec accepted", invalid)
		}
	}
}

func TestKnowledgeTermDefinitionOnly(t *testing.T) {
	if _, err := ParseKnowledgeSpec("business_term", json.RawMessage(`{"capability":"definition"}`), true); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseKnowledgeSpec("business_term", json.RawMessage(`{"capability":"predicate"}`), true); err == nil {
		t.Fatal("incomplete predicate publishable")
	}
}
