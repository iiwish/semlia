package governance

import (
	"encoding/json"
	"fmt"

	"github.com/iiwish/semlia/internal/domain/governance"
)

// The deterministic validator registry (SSOT §8.2: schema, reference and
// structural checks run as pure deterministic code before any AI or human
// judgment). Validator ids and versions are stable: runs record the exact
// version that produced them, so the registry never rewrites or removes an
// entry — new validators are added, old ones keep their version forever.

// ValidatorIDSchema, ValidatorIDReference and ValidatorIDStructural are the
// v1 registry entries. Severity vocabulary follows semantic-asset design
// §3.3: blocker | warning | info | not_applicable, where only a blocker
// blocks release candidacy.
const (
	ValidatorIDSchema     = "schema"
	ValidatorIDReference  = "reference"
	ValidatorIDStructural = "structural"

	ValidatorVersion1 = "1.0.0"
)

// Finding is one validator output. InputDigest is the canonical digest of the
// run inputs the validator executed over, so every persisted result is
// reproducible (SSOT NFR-003).
type Finding struct {
	Severity    governance.ValidationSeverity
	Code        string
	Message     string
	Details     json.RawMessage
	InputDigest string
}

// ReferenceKind classifies a reference the snapshot builder resolved.
type ReferenceKind string

const (
	ReferenceKindAsset          ReferenceKind = "asset"
	ReferenceKindRevision       ReferenceKind = "revision"
	ReferenceKindDataset        ReferenceKind = "dataset"
	ReferenceKindField          ReferenceKind = "field"
	ReferenceKindGovernedObject ReferenceKind = "governed_object"
	ReferenceKindMalformed      ReferenceKind = "malformed"
	ReferenceKindUnsupported    ReferenceKind = "unsupported"
)

// ReferenceResolution is the workspace-scoped existence verdict for one
// TypeID token found in a change-set value. Resolution happens in the
// adapter (I/O); validators only read the verdict.
type ReferenceResolution struct {
	Kind   ReferenceKind
	Exists bool
}

// ValidationProposalSnapshot is the semantic projection of a proposal:
// identifiers and target coordinates only, never volatile bookkeeping, so
// identical logical inputs always hash to the same digest.
type ValidationProposalSnapshot struct {
	ProposalID     string
	TargetType     governance.TargetObjectType
	TargetObjectID string
	AssetID        string
	BaseRevisionID string
}

// ValidationTargetSnapshot carries the resolved existence facts the pure
// validators consume: target presence plus one resolution per TypeID token
// found in the change-set values.
type ValidationTargetSnapshot struct {
	AssetExists        bool
	BaseRevisionExists bool
	TargetObjectExists bool
	Resolutions        map[string]ReferenceResolution
}

// ValidationInput is the complete deterministic input of one validation pass:
// the proposal snapshot, the target snapshot and the frozen change-set.
type ValidationInput struct {
	Proposal ValidationProposalSnapshot
	Target   ValidationTargetSnapshot
	Changes  []governance.ChangeSetItem
}

// Validator is one registry entry: a pure function over the input snapshot.
type Validator interface {
	ID() string
	Version() string
	Validate(input ValidationInput) ([]Finding, error)
}

// Registry holds the executable validators in their stable run order.
type Registry struct {
	ordered []Validator
	byID    map[string]Validator
}

func NewRegistry(validators ...Validator) *Registry {
	registry := &Registry{byID: make(map[string]Validator, len(validators))}
	for _, validator := range validators {
		if validator == nil || validator.ID() == "" {
			continue
		}
		if _, exists := registry.byID[validator.ID()]; exists {
			continue
		}
		registry.ordered = append(registry.ordered, validator)
		registry.byID[validator.ID()] = validator
	}
	return registry
}

// NewDefaultRegistry returns the v1 registry: schema, reference, structural.
func NewDefaultRegistry() *Registry {
	return NewRegistry(
		&schemaValidator{},
		&referenceValidator{},
		&structuralValidator{},
	)
}

// Get resolves one validator id; an unknown id is refused (never silently
// accepted by the run-recording path).
func (registry *Registry) Get(id string) (Validator, bool) {
	validator, ok := registry.byID[id]
	return validator, ok
}

// IDs returns the stable registry order.
func (registry *Registry) IDs() []string {
	ids := make([]string, 0, len(registry.ordered))
	for _, validator := range registry.ordered {
		ids = append(ids, validator.ID())
	}
	return ids
}

// All returns the validators in stable run order.
func (registry *Registry) All() []Validator {
	return append([]Validator(nil), registry.ordered...)
}

// validationInputEnvelope is the canonical digest input: change-set items are
// projected onto their semantic fields (no ids, no timestamps, no volatile
// bookkeeping) so the digest is a pure function of the logical inputs.
type validationInputEnvelope struct {
	ValidatorID      string                     `json:"validatorId"`
	ValidatorVersion string                     `json:"validatorVersion"`
	Proposal         ValidationProposalSnapshot `json:"proposal"`
	Target           ValidationTargetSnapshot   `json:"target"`
	Changes          []validationChangeEnvelope `json:"changes"`
}

type validationChangeEnvelope struct {
	FieldPath    string              `json:"fieldPath"`
	Op           governance.ChangeOp `json:"op"`
	BeforeDigest string              `json:"beforeDigest"`
	AfterDigest  string              `json:"afterDigest"`
	BeforeValue  json.RawMessage     `json:"beforeValue,omitempty"`
	AfterValue   json.RawMessage     `json:"afterValue,omitempty"`
}

// ValidationInputDigest hashes the canonical encoding of one validator run
// over one input snapshot as a repository-wide sha256 content digest.
func ValidationInputDigest(validatorID, validatorVersion string, input ValidationInput) (string, error) {
	changes := make([]validationChangeEnvelope, 0, len(input.Changes))
	for _, item := range input.Changes {
		changes = append(changes, validationChangeEnvelope{
			FieldPath: item.FieldPath, Op: item.Op,
			BeforeDigest: item.BeforeDigest, AfterDigest: item.AfterDigest,
			BeforeValue: item.BeforeValue, AfterValue: item.AfterValue,
		})
	}
	encoded, err := json.Marshal(validationInputEnvelope{
		ValidatorID: validatorID, ValidatorVersion: validatorVersion,
		Proposal: input.Proposal, Target: input.Target, Changes: changes,
	})
	if err != nil {
		return "", fmt.Errorf("encode validation input: %w", err)
	}
	digest, err := governance.DigestJSON(encoded)
	if err != nil {
		return "", fmt.Errorf("digest validation input: %w", err)
	}
	return digest, nil
}
