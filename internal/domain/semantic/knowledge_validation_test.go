package semantic_test

import (
	"encoding/json"
	"testing"

	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/internal/testsupport/knowledgecase"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestKnowledgePublicationRejectsBlankSemantics(t *testing.T) {
	c := knowledgecase.New()
	fields := map[semantic.AssetType][]string{
		semantic.BusinessObject: {"grain", "identityPolicy", "lifecycle"},
		semantic.Metric:         {"unit"},
		semantic.DataAsset:      {"grain", "coverage", "refreshFrequency", "sensitivity"},
		semantic.AnalysisModel:  {"grain"},
	}
	for _, asset := range c.Snapshot.Assets {
		for _, field := range fields[asset.AssetType] {
			t.Run(asset.Name+"/"+field, func(t *testing.T) {
				var content map[string]json.RawMessage
				if err := json.Unmarshal(asset.Content, &content); err != nil {
					t.Fatal(err)
				}
				var spec map[string]any
				if err := json.Unmarshal(content["spec"], &spec); err != nil {
					t.Fatal(err)
				}
				spec[field] = " \t\n "
				raw, err := json.Marshal(spec)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := semantic.ParseKnowledgeSpec(asset.AssetType, raw, true); err == nil {
					t.Fatal("blank publication semantics accepted")
				}
				if _, err := semantic.ParseKnowledgeSpec(asset.AssetType, raw, false); err != nil {
					t.Fatalf("incomplete draft must remain editable: %v", err)
				}
			})
		}
	}
}

func TestKnowledgePredicateRequiresSubjectAndCompatibleTypes(t *testing.T) {
	c := knowledgecase.New()
	loader := func(ref semantic.KnowledgeReference) (semantic.AssetType, semantic.KnowledgeSpec, error) {
		for _, asset := range c.Snapshot.Assets {
			if asset.AssetID.String() == ref.AssetID {
				var content struct {
					Spec semantic.KnowledgeSpec `json:"spec"`
				}
				if err := json.Unmarshal(asset.Content, &content); err != nil {
					t.Fatal(err)
				}
				return asset.AssetType, content.Spec, nil
			}
		}
		t.Fatal("unexpected reference", ref)
		return "", semantic.KnowledgeSpec{}, nil
	}
	refExpr := func(name, member string) *semantic.KnowledgeExpression {
		ref := c.Refs[name]
		ref.MemberID = member
		return &semantic.KnowledgeExpression{Op: "ref", Ref: &ref}
	}
	literal := func(raw string) *semantic.KnowledgeExpression {
		return &semantic.KnowledgeExpression{Op: "literal", Value: json.RawMessage(raw)}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*semantic.KnowledgeSpec)
		valid  bool
	}{
		{"valid string comparison", func(*semantic.KnowledgeSpec) {}, true},
		{"wrong subject", func(s *semantic.KnowledgeSpec) { s.Predicate.Left = refExpr("customers", "region") }, false},
		{"wrong subject revision", func(s *semantic.KnowledgeSpec) {
			id, _ := identity.NewRevisionID()
			s.Predicate.Left.Ref.RevisionID = id.String()
		}, false},
		{"numeric literal for string", func(s *semantic.KnowledgeSpec) { s.Predicate.Right = literal(`42`) }, false},
		{"undeclared parameter", func(s *semantic.KnowledgeSpec) {
			s.Predicate.Right = &semantic.KnowledgeExpression{Op: "parameter", Parameter: "missing"}
		}, false},
		{"wrong parameter type", func(s *semantic.KnowledgeSpec) {
			s.Parameters = []semantic.KnowledgeParameter{{Name: "status", Type: "number"}}
			s.Predicate.Right = &semantic.KnowledgeExpression{Op: "parameter", Parameter: "status"}
		}, false},
		{"non boolean root", func(s *semantic.KnowledgeSpec) { s.Predicate = refExpr("orders", "amount") }, false},
		{"non boolean conjunction", func(s *semantic.KnowledgeSpec) { s.Predicate.Op = "and" }, false},
		{"valid numeric comparison", func(s *semantic.KnowledgeSpec) {
			s.Predicate.Left = refExpr("orders", "amount")
			s.Predicate.Right = literal(`42`)
		}, true},
		{"valid timestamp literal", func(s *semantic.KnowledgeSpec) {
			s.Predicate.Left = refExpr("orders", "paid_at")
			s.Predicate.Right = literal(`"2026-08-01T00:00:00Z"`)
		}, true},
		{"invalid timestamp literal", func(s *semantic.KnowledgeSpec) {
			s.Predicate.Left = refExpr("orders", "paid_at")
			s.Predicate.Right = literal(`"not a date"`)
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, spec, _ := loader(c.Refs["paid"])
			tc.mutate(&spec)
			err := spec.ValidateReferences(loader)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}
