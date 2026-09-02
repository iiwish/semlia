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
