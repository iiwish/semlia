package identity

import (
	"errors"
	"fmt"

	"go.jetify.com/typeid"
)

var (
	ErrInvalidID      = errors.New("invalid resource ID")
	ErrUnknownPrefix  = errors.New("unknown resource prefix")
	ErrPrefixMismatch = errors.New("resource prefix mismatch")
	ErrUUIDVersion    = errors.New("resource UUID must be version 7")
	ErrZeroID         = errors.New("resource ID cannot be zero")
)

type Prefix string

const (
	Workspace               Prefix = "wsp"
	Asset                   Prefix = "ast"
	Revision                Prefix = "rev"
	Relation                Prefix = "rel"
	Ontology                Prefix = "ont"
	Evidence                Prefix = "evd"
	Release                 Prefix = "rls"
	Run                     Prefix = "run"
	Event                   Prefix = "evt"
	SourceConnection        Prefix = "src"
	SourceRevision          Prefix = "srv"
	PhysicalDataset         Prefix = "pds"
	PhysicalDatasetRevision Prefix = "pdr"
	PhysicalField           Prefix = "pfd"
	PhysicalFieldRevision   Prefix = "pfr"
	CodeArtifact            Prefix = "cod"
	LineageEdge             Prefix = "lin"
)

var registeredPrefixes = []Prefix{
	Workspace,
	Asset,
	Revision,
	Relation,
	Ontology,
	Evidence,
	Release,
	Run,
	Event,
	SourceConnection,
	SourceRevision,
	PhysicalDataset,
	PhysicalDatasetRevision,
	PhysicalField,
	PhysicalFieldRevision,
	CodeArtifact,
	LineageEdge,
}

type ID struct {
	prefix Prefix
	raw    string
	uuid   string
}

func Prefixes() []Prefix {
	return append([]Prefix(nil), registeredPrefixes...)
}

func New(prefix Prefix) (ID, error) {
	if !isRegistered(prefix) {
		return ID{}, fmt.Errorf("%w: %q", ErrUnknownPrefix, prefix)
	}
	value, err := typeid.WithPrefix(string(prefix))
	if err != nil {
		return ID{}, fmt.Errorf("%w: %v", ErrInvalidID, err)
	}
	return fromTypeID(value)
}

func Parse(expected Prefix, value string) (ID, error) {
	id, err := ParseAny(value)
	if err != nil {
		return ID{}, err
	}
	if id.prefix != expected {
		return ID{}, fmt.Errorf("%w: got %q, want %q", ErrPrefixMismatch, id.prefix, expected)
	}
	return id, nil
}

func ParseAny(value string) (ID, error) {
	parsed, err := typeid.FromString(value)
	if err != nil {
		return ID{}, fmt.Errorf("%w: %v", ErrInvalidID, err)
	}
	return fromTypeID(parsed)
}

func FromUUID(prefix Prefix, value string) (ID, error) {
	if !isRegistered(prefix) {
		return ID{}, fmt.Errorf("%w: %q", ErrUnknownPrefix, prefix)
	}
	if isZeroUUID(value) {
		return ID{}, ErrZeroID
	}
	if !isUUIDv7(value) {
		return ID{}, ErrUUIDVersion
	}
	parsed, err := typeid.FromUUIDWithPrefix(string(prefix), value)
	if err != nil {
		return ID{}, fmt.Errorf("%w: %v", ErrInvalidID, err)
	}
	return fromTypeID(parsed)
}

func FromUUIDBytes(prefix Prefix, value [16]byte) (ID, error) {
	if !isRegistered(prefix) {
		return ID{}, fmt.Errorf("%w: %q", ErrUnknownPrefix, prefix)
	}
	parsed, err := typeid.FromUUIDBytesWithPrefix(string(prefix), value[:])
	if err != nil {
		return ID{}, fmt.Errorf("%w: %v", ErrInvalidID, err)
	}
	return fromTypeID(parsed)
}

func fromTypeID(value typeid.AnyID) (ID, error) {
	prefix := Prefix(value.Prefix())
	if !isRegistered(prefix) {
		return ID{}, fmt.Errorf("%w: %q", ErrUnknownPrefix, prefix)
	}
	if value.IsZero() {
		return ID{}, ErrZeroID
	}
	uuid := value.UUID()
	if !isUUIDv7(uuid) {
		return ID{}, ErrUUIDVersion
	}
	return ID{prefix: prefix, raw: value.String(), uuid: uuid}, nil
}

func (id ID) Prefix() Prefix { return id.prefix }

func (id ID) UUID() string { return id.uuid }

func (id ID) String() string { return id.raw }

func (id ID) IsZero() bool { return id.raw == "" }

func (id ID) MarshalText() ([]byte, error) {
	if id.IsZero() {
		return nil, ErrZeroID
	}
	return []byte(id.raw), nil
}

func (id *ID) UnmarshalText(value []byte) error {
	parsed, err := ParseAny(string(value))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

type resourceKind interface {
	resourcePrefix() Prefix
}

type TypedID[K resourceKind] struct {
	value ID
}

func newTypedID[K resourceKind]() (TypedID[K], error) {
	value, err := New(prefixFor[K]())
	if err != nil {
		return TypedID[K]{}, err
	}
	return TypedID[K]{value: value}, nil
}

func parseTypedID[K resourceKind](value string) (TypedID[K], error) {
	parsed, err := Parse(prefixFor[K](), value)
	if err != nil {
		return TypedID[K]{}, err
	}
	return TypedID[K]{value: parsed}, nil
}

func typedIDFromUUIDBytes[K resourceKind](value [16]byte) (TypedID[K], error) {
	parsed, err := FromUUIDBytes(prefixFor[K](), value)
	if err != nil {
		return TypedID[K]{}, err
	}
	return TypedID[K]{value: parsed}, nil
}

func (id TypedID[K]) Prefix() Prefix { return id.value.Prefix() }

func (id TypedID[K]) UUID() string { return id.value.UUID() }

func (id TypedID[K]) String() string { return id.value.String() }

func (id TypedID[K]) IsZero() bool { return id.value.IsZero() }

func (id TypedID[K]) MarshalText() ([]byte, error) { return id.value.MarshalText() }

func (id *TypedID[K]) UnmarshalText(value []byte) error {
	parsed, err := parseTypedID[K](string(value))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

func prefixFor[K resourceKind]() Prefix {
	var kind K
	return kind.resourcePrefix()
}

type workspaceKind struct{}
type assetKind struct{}
type revisionKind struct{}
type relationKind struct{}
type ontologyKind struct{}
type evidenceKind struct{}
type releaseKind struct{}
type runKind struct{}
type eventKind struct{}
type sourceConnectionKind struct{}
type sourceRevisionKind struct{}
type physicalDatasetKind struct{}
type physicalDatasetRevisionKind struct{}
type physicalFieldKind struct{}
type physicalFieldRevisionKind struct{}
type codeArtifactKind struct{}
type lineageEdgeKind struct{}

func (workspaceKind) resourcePrefix() Prefix               { return Workspace }
func (assetKind) resourcePrefix() Prefix                   { return Asset }
func (revisionKind) resourcePrefix() Prefix                { return Revision }
func (relationKind) resourcePrefix() Prefix                { return Relation }
func (ontologyKind) resourcePrefix() Prefix                { return Ontology }
func (evidenceKind) resourcePrefix() Prefix                { return Evidence }
func (releaseKind) resourcePrefix() Prefix                 { return Release }
func (runKind) resourcePrefix() Prefix                     { return Run }
func (eventKind) resourcePrefix() Prefix                   { return Event }
func (sourceConnectionKind) resourcePrefix() Prefix        { return SourceConnection }
func (sourceRevisionKind) resourcePrefix() Prefix          { return SourceRevision }
func (physicalDatasetKind) resourcePrefix() Prefix         { return PhysicalDataset }
func (physicalDatasetRevisionKind) resourcePrefix() Prefix { return PhysicalDatasetRevision }
func (physicalFieldKind) resourcePrefix() Prefix           { return PhysicalField }
func (physicalFieldRevisionKind) resourcePrefix() Prefix   { return PhysicalFieldRevision }
func (codeArtifactKind) resourcePrefix() Prefix            { return CodeArtifact }
func (lineageEdgeKind) resourcePrefix() Prefix             { return LineageEdge }

type WorkspaceID = TypedID[workspaceKind]
type AssetID = TypedID[assetKind]
type RevisionID = TypedID[revisionKind]
type RelationID = TypedID[relationKind]
type OntologyID = TypedID[ontologyKind]
type EvidenceID = TypedID[evidenceKind]
type ReleaseID = TypedID[releaseKind]
type RunID = TypedID[runKind]
type EventID = TypedID[eventKind]
type SourceConnectionID = TypedID[sourceConnectionKind]
type SourceRevisionID = TypedID[sourceRevisionKind]
type PhysicalDatasetID = TypedID[physicalDatasetKind]
type PhysicalDatasetRevisionID = TypedID[physicalDatasetRevisionKind]
type PhysicalFieldID = TypedID[physicalFieldKind]
type PhysicalFieldRevisionID = TypedID[physicalFieldRevisionKind]
type CodeArtifactID = TypedID[codeArtifactKind]
type LineageEdgeID = TypedID[lineageEdgeKind]

func NewWorkspaceID() (WorkspaceID, error)               { return newTypedID[workspaceKind]() }
func NewAssetID() (AssetID, error)                       { return newTypedID[assetKind]() }
func NewRevisionID() (RevisionID, error)                 { return newTypedID[revisionKind]() }
func NewRelationID() (RelationID, error)                 { return newTypedID[relationKind]() }
func NewOntologyID() (OntologyID, error)                 { return newTypedID[ontologyKind]() }
func NewEvidenceID() (EvidenceID, error)                 { return newTypedID[evidenceKind]() }
func NewReleaseID() (ReleaseID, error)                   { return newTypedID[releaseKind]() }
func NewRunID() (RunID, error)                           { return newTypedID[runKind]() }
func NewEventID() (EventID, error)                       { return newTypedID[eventKind]() }
func NewSourceConnectionID() (SourceConnectionID, error) { return newTypedID[sourceConnectionKind]() }
func NewSourceRevisionID() (SourceRevisionID, error)     { return newTypedID[sourceRevisionKind]() }
func NewPhysicalDatasetID() (PhysicalDatasetID, error)   { return newTypedID[physicalDatasetKind]() }
func NewPhysicalDatasetRevisionID() (PhysicalDatasetRevisionID, error) {
	return newTypedID[physicalDatasetRevisionKind]()
}
func NewPhysicalFieldID() (PhysicalFieldID, error) { return newTypedID[physicalFieldKind]() }
func NewPhysicalFieldRevisionID() (PhysicalFieldRevisionID, error) {
	return newTypedID[physicalFieldRevisionKind]()
}
func NewCodeArtifactID() (CodeArtifactID, error) { return newTypedID[codeArtifactKind]() }
func NewLineageEdgeID() (LineageEdgeID, error)   { return newTypedID[lineageEdgeKind]() }

func ParseWorkspaceID(value string) (WorkspaceID, error) { return parseTypedID[workspaceKind](value) }
func ParseAssetID(value string) (AssetID, error)         { return parseTypedID[assetKind](value) }
func ParseRevisionID(value string) (RevisionID, error)   { return parseTypedID[revisionKind](value) }
func ParseRelationID(value string) (RelationID, error)   { return parseTypedID[relationKind](value) }
func ParseOntologyID(value string) (OntologyID, error)   { return parseTypedID[ontologyKind](value) }
func ParseEvidenceID(value string) (EvidenceID, error)   { return parseTypedID[evidenceKind](value) }
func ParseReleaseID(value string) (ReleaseID, error)     { return parseTypedID[releaseKind](value) }
func ParseRunID(value string) (RunID, error)             { return parseTypedID[runKind](value) }
func ParseEventID(value string) (EventID, error)         { return parseTypedID[eventKind](value) }
func ParseSourceConnectionID(value string) (SourceConnectionID, error) {
	return parseTypedID[sourceConnectionKind](value)
}
func ParseSourceRevisionID(value string) (SourceRevisionID, error) {
	return parseTypedID[sourceRevisionKind](value)
}
func ParsePhysicalDatasetID(value string) (PhysicalDatasetID, error) {
	return parseTypedID[physicalDatasetKind](value)
}
func ParsePhysicalDatasetRevisionID(value string) (PhysicalDatasetRevisionID, error) {
	return parseTypedID[physicalDatasetRevisionKind](value)
}
func ParsePhysicalFieldID(value string) (PhysicalFieldID, error) {
	return parseTypedID[physicalFieldKind](value)
}
func ParsePhysicalFieldRevisionID(value string) (PhysicalFieldRevisionID, error) {
	return parseTypedID[physicalFieldRevisionKind](value)
}
func ParseCodeArtifactID(value string) (CodeArtifactID, error) {
	return parseTypedID[codeArtifactKind](value)
}
func ParseLineageEdgeID(value string) (LineageEdgeID, error) {
	return parseTypedID[lineageEdgeKind](value)
}

func WorkspaceIDFromUUIDBytes(value [16]byte) (WorkspaceID, error) {
	return typedIDFromUUIDBytes[workspaceKind](value)
}
func RunIDFromUUIDBytes(value [16]byte) (RunID, error) { return typedIDFromUUIDBytes[runKind](value) }
func EventIDFromUUIDBytes(value [16]byte) (EventID, error) {
	return typedIDFromUUIDBytes[eventKind](value)
}
func AssetIDFromUUIDBytes(value [16]byte) (AssetID, error) {
	return typedIDFromUUIDBytes[assetKind](value)
}
func RevisionIDFromUUIDBytes(value [16]byte) (RevisionID, error) {
	return typedIDFromUUIDBytes[revisionKind](value)
}
func RelationIDFromUUIDBytes(value [16]byte) (RelationID, error) {
	return typedIDFromUUIDBytes[relationKind](value)
}
func OntologyIDFromUUIDBytes(value [16]byte) (OntologyID, error) {
	return typedIDFromUUIDBytes[ontologyKind](value)
}
func EvidenceIDFromUUIDBytes(value [16]byte) (EvidenceID, error) {
	return typedIDFromUUIDBytes[evidenceKind](value)
}
func SourceConnectionIDFromUUIDBytes(value [16]byte) (SourceConnectionID, error) {
	return typedIDFromUUIDBytes[sourceConnectionKind](value)
}
func SourceRevisionIDFromUUIDBytes(value [16]byte) (SourceRevisionID, error) {
	return typedIDFromUUIDBytes[sourceRevisionKind](value)
}
func PhysicalDatasetIDFromUUIDBytes(value [16]byte) (PhysicalDatasetID, error) {
	return typedIDFromUUIDBytes[physicalDatasetKind](value)
}
func PhysicalDatasetRevisionIDFromUUIDBytes(value [16]byte) (PhysicalDatasetRevisionID, error) {
	return typedIDFromUUIDBytes[physicalDatasetRevisionKind](value)
}
func PhysicalFieldIDFromUUIDBytes(value [16]byte) (PhysicalFieldID, error) {
	return typedIDFromUUIDBytes[physicalFieldKind](value)
}
func PhysicalFieldRevisionIDFromUUIDBytes(value [16]byte) (PhysicalFieldRevisionID, error) {
	return typedIDFromUUIDBytes[physicalFieldRevisionKind](value)
}
func CodeArtifactIDFromUUIDBytes(value [16]byte) (CodeArtifactID, error) {
	return typedIDFromUUIDBytes[codeArtifactKind](value)
}
func LineageEdgeIDFromUUIDBytes(value [16]byte) (LineageEdgeID, error) {
	return typedIDFromUUIDBytes[lineageEdgeKind](value)
}

func isRegistered(prefix Prefix) bool {
	for _, candidate := range registeredPrefixes {
		if prefix == candidate {
			return true
		}
	}
	return false
}

func isUUIDv7(value string) bool {
	return len(value) == 36 && value[8] == '-' && value[13] == '-' && value[14] == '7' &&
		value[18] == '-' && value[23] == '-' && (value[19] == '8' || value[19] == '9' || value[19] == 'a' || value[19] == 'b')
}

func isZeroUUID(value string) bool {
	return value == "00000000-0000-0000-0000-000000000000"
}
