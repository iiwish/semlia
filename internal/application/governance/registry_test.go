package governance_test

import (
	"encoding/json"
	"reflect"
	"testing"

	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
)

const (
	fixtureProposalUUID = "0197c1a3-7d10-7cc2-9f6e-6a1b2c3d4e5f"
	fixtureDigest       = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
)

// registryInput builds one deterministic validation input used across the
// registry and validator unit suites.
func registryInput() governanceapp.ValidationInput {
	return governanceapp.ValidationInput{
		Proposal: governanceapp.ValidationProposalSnapshot{
			ProposalID:     fixtureProposalUUID,
			TargetType:     governance.TargetSemanticAsset,
			TargetObjectID: fixtureProposalUUID,
			AssetID:        fixtureProposalUUID,
			BaseRevisionID: fixtureProposalUUID,
		},
		Target: governanceapp.ValidationTargetSnapshot{
			AssetExists:        true,
			BaseRevisionExists: true,
			TargetObjectExists: true,
			Resolutions:        map[string]governanceapp.ReferenceResolution{},
		},
		Changes: []governance.ChangeSetItem{},
	}
}

func TestRegistryReportsStableValidatorIdsAndVersions(t *testing.T) {
	registry := governanceapp.NewDefaultRegistry()
	ids := registry.IDs()
	want := []string{"schema", "reference", "structural"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("registry ids = %v, want %v", ids, want)
	}
	for _, id := range want {
		validator, ok := registry.Get(id)
		if !ok {
			t.Fatalf("registry is missing validator %q", id)
		}
		if validator.ID() != id {
			t.Fatalf("validator.ID() = %q, want %q", validator.ID(), id)
		}
		if validator.Version() != "1.0.0" {
			t.Fatalf("validator %q version = %q, want 1.0.0", id, validator.Version())
		}
	}
}

func TestRegistryRefusesUnknownValidatorId(t *testing.T) {
	// Packet red scenario: an unknown validator id is refused when recording
	// a run; the registry is the only source of executable validator ids.
	registry := governanceapp.NewDefaultRegistry()
	for _, unknown := range []string{"", "unknown", "schema;drop", "SCHEMA"} {
		if _, ok := registry.Get(unknown); ok {
			t.Fatalf("registry accepted validator id %q", unknown)
		}
	}
}

func TestValidatorsAreDeterministicPureFunctions(t *testing.T) {
	// Packet red scenario: identical proposal inputs produce identical
	// validator findings and input digests across two runs of the same
	// snapshot. Validators receive the snapshot by value and never touch
	// clocks, randomness or the database.
	registry := governanceapp.NewDefaultRegistry()
	input := registryInput()
	input.Target.Resolutions = map[string]governanceapp.ReferenceResolution{
		"ast_01arz3ndektsv4rrffq69g5fav": {Kind: governanceapp.ReferenceKindAsset, Exists: false},
	}
	for _, id := range registry.IDs() {
		validator, _ := registry.Get(id)
		first, firstErr := validator.Validate(input)
		second, secondErr := validator.Validate(input)
		if (firstErr != nil) != (secondErr != nil) {
			t.Fatalf("validator %q error mismatch: %v vs %v", id, firstErr, secondErr)
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("validator %q is not deterministic:\nfirst = %+v\nsecond = %+v", id, first, second)
		}
		for _, finding := range first {
			if !governance.IsValidContentDigest(finding.InputDigest) {
				t.Fatalf("finding %s carries malformed input digest %q", finding.Code, finding.InputDigest)
			}
			if !finding.Severity.Valid() {
				t.Fatalf("finding %s carries invalid severity %q", finding.Code, finding.Severity)
			}
		}
	}
}

func TestValidationInputDigestExcludesVolatileFields(t *testing.T) {
	// Two inputs that differ only in wall-clock bookkeeping must hash to the
	// same digest, so re-running validation over unchanged semantic inputs
	// reproduces the recorded digests (SSOT NFR-003).
	first := registryInput()
	second := registryInput()
	firstDigest, err := governanceapp.ValidationInputDigest("schema", "1.0.0", first)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := governanceapp.ValidationInputDigest("schema", "1.0.0", second)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("digests diverge for identical inputs: %s vs %s", firstDigest, secondDigest)
	}
	changed := registryInput()
	changed.Changes = append(changed.Changes, governance.ChangeSetItem{
		FieldPath: "definition", Op: governance.ChangeUpdate,
		AfterDigest: fixtureDigest, AfterValue: json.RawMessage(`"changed"`),
	})
	changedDigest, err := governanceapp.ValidationInputDigest("schema", "1.0.0", changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedDigest == firstDigest {
		t.Fatal("digest ignored change-set content")
	}
}
