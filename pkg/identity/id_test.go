package identity_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/iiwish/semlia/pkg/identity"
)

func TestRegisteredPrefixesGenerateUUIDv7AndRoundTrip(t *testing.T) {
	for _, prefix := range identity.Prefixes() {
		t.Run(string(prefix), func(t *testing.T) {
			id, err := identity.New(prefix)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(id.String(), string(prefix)+"_") {
				t.Fatalf("ID %q does not have prefix %q", id, prefix)
			}
			if uuid := id.UUID(); len(uuid) != 36 || uuid[14] != '7' {
				t.Fatalf("UUID %q is not version 7", uuid)
			}

			parsed, err := identity.Parse(prefix, id.String())
			if err != nil {
				t.Fatal(err)
			}
			if parsed != id || parsed.UUID() != id.UUID() {
				t.Fatalf("round trip = %q/%q, want %q/%q", parsed, parsed.UUID(), id, id.UUID())
			}
		})
	}
}

func TestParseRejectsWrongPrefixMalformedAndNonV7(t *testing.T) {
	asset, err := identity.New(identity.Asset)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Parse(identity.Revision, asset.String()); !errors.Is(err, identity.ErrPrefixMismatch) {
		t.Fatalf("wrong prefix error = %v", err)
	}
	if _, err := identity.ParseAny("asset_not-base32"); !errors.Is(err, identity.ErrInvalidID) {
		t.Fatalf("malformed error = %v", err)
	}
	if _, err := identity.FromUUID(identity.Asset, "550e8400-e29b-41d4-a716-446655440000"); !errors.Is(err, identity.ErrUUIDVersion) {
		t.Fatalf("non-v7 error = %v", err)
	}
	if _, err := identity.FromUUID(identity.Asset, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, identity.ErrZeroID) {
		t.Fatalf("zero UUID error = %v", err)
	}
}

func TestJSONUsesTypeIDAndPreservesPrefixValidation(t *testing.T) {
	id, err := identity.New(identity.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `"`+id.String()+`"` {
		t.Fatalf("JSON = %s", encoded)
	}

	var decoded identity.ID
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != id {
		t.Fatalf("decoded = %q, want %q", decoded, id)
	}
	if err := json.Unmarshal([]byte(`"unknown_01arz3ndektsv4rrffq69g5fav"`), &decoded); !errors.Is(err, identity.ErrUnknownPrefix) {
		t.Fatalf("unknown prefix error = %v", err)
	}
}

func TestTypedIDRejectsAnotherRegisteredResourceType(t *testing.T) {
	asset, err := identity.NewAssetID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.ParseRevisionID(asset.String()); !errors.Is(err, identity.ErrPrefixMismatch) {
		t.Fatalf("typed prefix error = %v", err)
	}

	encoded, err := json.Marshal(asset)
	if err != nil {
		t.Fatal(err)
	}
	var decoded identity.AssetID
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != asset {
		t.Fatalf("decoded = %q, want %q", decoded, asset)
	}
}

func TestAttentionItemIDUsesDedicatedPrefixAndUUIDv7(t *testing.T) {
	id, err := identity.NewAttentionItemID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id.String(), "ati_") || id.UUID()[14] != '7' {
		t.Fatalf("attention item ID = %q / %q", id.String(), id.UUID())
	}
	parsed, err := identity.ParseAttentionItemID(id.String())
	if err != nil || parsed != id {
		t.Fatalf("attention item round trip = %q / %v", parsed, err)
	}
}

func TestProductionOperationIDUsesDedicatedPrefixAndUUIDv7(t *testing.T) {
	id, err := identity.NewProductionOperationID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id.String(), "prodop_") || id.UUID()[14] != '7' {
		t.Fatalf("production operation ID = %q / %q", id.String(), id.UUID())
	}
	parsed, err := identity.ParseProductionOperationID(id.String())
	if err != nil || parsed != id {
		t.Fatalf("production operation round trip = %q / %v", parsed, err)
	}
}

func TestIngestionIDsUseDedicatedPrefixesAndUUIDv7(t *testing.T) {
	tests := []struct {
		prefix string
		newID  func() (string, string, error)
		parse  func(string) error
	}{
		{"art_", func() (string, string, error) {
			id, err := identity.NewArtifactID()
			return id.String(), id.UUID(), err
		}, func(value string) error { _, err := identity.ParseArtifactID(value); return err }},
		{"ars_", func() (string, string, error) {
			id, err := identity.NewArtifactSetID()
			return id.String(), id.UUID(), err
		}, func(value string) error { _, err := identity.ParseArtifactSetID(value); return err }},
		{"sch_", func() (string, string, error) {
			id, err := identity.NewSourceScheduleID()
			return id.String(), id.UUID(), err
		}, func(value string) error { _, err := identity.ParseSourceScheduleID(value); return err }},
		{"occ_", func() (string, string, error) {
			id, err := identity.NewScheduleOccurrenceID()
			return id.String(), id.UUID(), err
		}, func(value string) error { _, err := identity.ParseScheduleOccurrenceID(value); return err }},
	}
	for _, test := range tests {
		value, uuid, err := test.newID()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(value, test.prefix) || uuid[14] != '7' {
			t.Fatalf("ingestion ID = %q / %q", value, uuid)
		}
		if err := test.parse(value); err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
	}
}
