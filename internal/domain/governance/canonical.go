package governance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// MaxCanonicalJSONBytes bounds every canonicalized governance payload
// (policy inputs, release manifests) before hashing.
const MaxCanonicalJSONBytes = 1 << 20

// CanonicalJSON returns the canonical encoding used for every governance
// digest: the value is decoded with precise numbers and re-encoded with
// recursively sorted object keys and no insignificant whitespace. Array order
// and number literals are preserved verbatim, so the canonical form is a
// deterministic function of the value — two callers that serialize the same
// inputs (in any key order or whitespace) hash to the same digest.
func CanonicalJSON(value json.RawMessage) (json.RawMessage, error) {
	if len(value) == 0 {
		return nil, fmt.Errorf("%w: empty JSON value", ErrInvalidArgument)
	}
	if len(value) > MaxCanonicalJSONBytes {
		return nil, fmt.Errorf("%w: JSON value exceeds %d bytes", ErrInvalidArgument, MaxCanonicalJSONBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON value", ErrInvalidArgument)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing JSON content", ErrInvalidArgument)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("%w: canonical re-encoding failed", ErrInvalidArgument)
	}
	return encoded, nil
}

// DigestJSON hashes the canonical encoding as a sha256 content digest with the
// repository-wide "sha256:" prefix.
func DigestJSON(value json.RawMessage) (string, error) {
	canonical, err := CanonicalJSON(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
