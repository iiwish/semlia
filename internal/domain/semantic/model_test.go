package semantic_test

import (
	"errors"
	"testing"

	"github.com/iiwish/semlia/internal/domain/semantic"
)

func TestSemanticAddress(t *testing.T) {
	address, err := semantic.NewAddress("commerce", "net_revenue")
	if err != nil {
		t.Fatal(err)
	}
	if address.String() != "commerce.net_revenue" {
		t.Fatalf("address = %q", address)
	}

	for _, test := range []struct{ namespace, key string }{
		{"", "net_revenue"},
		{"Commerce", "net_revenue"},
		{"commerce", "net revenue"},
		{"commerce", "net.revenue"},
	} {
		if _, err := semantic.NewAddress(test.namespace, test.key); !errors.Is(err, semantic.ErrInvalidAddress) {
			t.Errorf("NewAddress(%q, %q) error = %v", test.namespace, test.key, err)
		}
	}
}

func TestRevisionRequiresPositiveSequenceAndContentDigest(t *testing.T) {
	revision := semantic.AssetRevision{Sequence: 0, SchemaVersion: "1.0.0", ContentDigest: "sha256:abc"}
	if err := revision.Validate(); !errors.Is(err, semantic.ErrInvalidRevision) {
		t.Fatalf("sequence error = %v", err)
	}
	revision.Sequence = 1
	revision.ContentDigest = ""
	if err := revision.Validate(); !errors.Is(err, semantic.ErrInvalidRevision) {
		t.Fatalf("digest error = %v", err)
	}
}

func TestRelationPoliciesValidatePlaneStateAndEndpoints(t *testing.T) {
	valid := semantic.Relation{
		Predicate:      semantic.Measures,
		Plane:          semantic.SemanticPlane,
		AssertionState: semantic.Asserted,
		SubjectType:    semantic.Metric,
		ObjectType:     semantic.Entity,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid relation: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*semantic.Relation)
	}{
		{"wrong plane", func(relation *semantic.Relation) { relation.Plane = semantic.TaxonomyPlane }},
		{"wrong target", func(relation *semantic.Relation) { relation.ObjectType = semantic.Segment }},
		{"unknown state", func(relation *semantic.Relation) { relation.AssertionState = "approved" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			relation := valid
			test.mutate(&relation)
			if err := relation.Validate(); !errors.Is(err, semantic.ErrInvalidRelation) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestTaxonomyRelationsRequireMatchingTypes(t *testing.T) {
	relation := semantic.Relation{
		Predicate:      semantic.EquivalentTo,
		Plane:          semantic.TaxonomyPlane,
		AssertionState: semantic.Inferred,
		SubjectType:    semantic.Concept,
		ObjectType:     semantic.Concept,
	}
	if err := relation.Validate(); err != nil {
		t.Fatal(err)
	}
	relation.ObjectType = semantic.Metric
	if err := relation.Validate(); !errors.Is(err, semantic.ErrInvalidRelation) {
		t.Fatalf("mismatched taxonomy relation error = %v", err)
	}
}
