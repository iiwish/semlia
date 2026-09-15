package governance_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/iiwish/semlia/internal/domain/governance"
)

func TestCanonicalJSONIsKeyOrderAndWhitespaceIndependent(t *testing.T) {
	first := json.RawMessage(`{"b": 1, "a": {"y": [1, 2], "x": true}}`)
	second := json.RawMessage(`{"a":{"x":true,"y":[1,2]},"b":1}`)
	canonicalFirst, err := governance.CanonicalJSON(first)
	if err != nil {
		t.Fatalf("canonicalize first: %v", err)
	}
	canonicalSecond, err := governance.CanonicalJSON(second)
	if err != nil {
		t.Fatalf("canonicalize second: %v", err)
	}
	if string(canonicalFirst) != string(canonicalSecond) {
		t.Fatalf("canonical forms differ:\n%s\n%s", canonicalFirst, canonicalSecond)
	}
	digestFirst, err := governance.DigestJSON(first)
	if err != nil {
		t.Fatalf("digest first: %v", err)
	}
	digestSecond, err := governance.DigestJSON(second)
	if err != nil {
		t.Fatalf("digest second: %v", err)
	}
	if digestFirst != digestSecond {
		t.Fatalf("digests differ: %s != %s", digestFirst, digestSecond)
	}
	if len(digestFirst) != len("sha256:")+64 {
		t.Fatalf("digest %q is not a sha256 content digest", digestFirst)
	}
}

func TestCanonicalJSONPreservesArrayOrderAndNumberLiterals(t *testing.T) {
	arrayFirst := json.RawMessage(`[3, 1, 2]`)
	arraySecond := json.RawMessage(`[1, 2, 3]`)
	canonicalFirst, err := governance.CanonicalJSON(arrayFirst)
	if err != nil {
		t.Fatalf("canonicalize first: %v", err)
	}
	if string(canonicalFirst) != "[3,1,2]" {
		t.Fatalf("array order must be preserved, got %s", canonicalFirst)
	}
	digestFirst, err := governance.DigestJSON(arrayFirst)
	if err != nil {
		t.Fatalf("digest first: %v", err)
	}
	digestSecond, err := governance.DigestJSON(arraySecond)
	if err != nil {
		t.Fatalf("digest second: %v", err)
	}
	if digestFirst == digestSecond {
		t.Fatal("different array orders must produce different digests")
	}
	number, err := governance.CanonicalJSON(json.RawMessage(`{"n": 1.500}`))
	if err != nil {
		t.Fatalf("canonicalize number: %v", err)
	}
	if string(number) != `{"n":1.500}` {
		t.Fatalf("number literals are preserved verbatim, got %s", number)
	}
}

func TestCanonicalJSONRejectsInvalidAndOversizedInput(t *testing.T) {
	if _, err := governance.CanonicalJSON(json.RawMessage(`{broken`)); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("invalid JSON error = %v, want ErrInvalidArgument", err)
	}
	oversized := make([]byte, governance.MaxCanonicalJSONBytes+1)
	for index := range oversized {
		oversized[index] = 'a'
	}
	if _, err := governance.CanonicalJSON(oversized); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("oversized JSON error = %v, want ErrInvalidArgument", err)
	}
}

func TestDigestJSONFormatMatchesContentDigestContract(t *testing.T) {
	digest, err := governance.DigestJSON(json.RawMessage(`{"rule":"v1"}`))
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if len(digest) != 71 || digest[:7] != "sha256:" {
		t.Fatalf("digest %q must match ^sha256:[0-9a-f]{64}$", digest)
	}
	for _, character := range digest[7:] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			t.Fatalf("digest %q contains non-hexadecimal character %q", digest, character)
		}
	}
}
