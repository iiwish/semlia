package governance

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

// The four M2 governance object types (SSOT §17 M2, D-014) join the T002
// proposal target vocabulary. They are structurally validated payloads, not
// workflow machines: per semantic-asset design §4.8 status dimensions stay
// with assets and proposals, so none of these types carries a lifecycle or
// workflow enum — only the row version, which the governed change application
// path bumps.
const (
	TargetPhysicalBinding TargetObjectType = "physical_binding"
)

// maxFieldRefs bounds one physical-reference array; joins and keys with more
// than 64 columns are modeling errors, not contracts.
const maxFieldRefs = 64

// UniquenessSemantics describes how the physical source realizes an entity
// key: exact rows or rows that require deduplication.
type UniquenessSemantics string

const (
	UniquenessExact        UniquenessSemantics = "exact"
	UniquenessDeduplicated UniquenessSemantics = "deduplicated"
)

func (semantics UniquenessSemantics) Valid() bool {
	switch semantics {
	case UniquenessExact, UniquenessDeduplicated:
		return true
	}
	return false
}

// JoinType is the closed SQL join vocabulary of a JoinContract.
type JoinType string

const (
	JoinInner JoinType = "inner"
	JoinLeft  JoinType = "left"
	JoinRight JoinType = "right"
	JoinFull  JoinType = "full"
)

func (joinType JoinType) Valid() bool {
	switch joinType {
	case JoinInner, JoinLeft, JoinRight, JoinFull:
		return true
	}
	return false
}

// JoinCardinality is the closed row-multiplicity vocabulary of a JoinContract.
type JoinCardinality string

const (
	CardinalityOneToOne   JoinCardinality = "one_to_one"
	CardinalityOneToMany  JoinCardinality = "one_to_many"
	CardinalityManyToOne  JoinCardinality = "many_to_one"
	CardinalityManyToMany JoinCardinality = "many_to_many"
)

func (cardinality JoinCardinality) Valid() bool {
	switch cardinality {
	case CardinalityOneToOne, CardinalityOneToMany, CardinalityManyToOne, CardinalityManyToMany:
		return true
	}
	return false
}

// IsGovernedObject reports whether the proposal target type is one of the
// four governance objects targeted through the governed-object pipeline.
func (targetType TargetObjectType) IsGovernedObject() bool {
	switch targetType {
	case TargetPhysicalBinding, TargetModelGrain, TargetEntityKey, TargetJoinContract:
		return true
	}
	return false
}

// Prefix maps the governed object type onto its identity TypeID prefix.
func (targetType TargetObjectType) Prefix() (identity.Prefix, error) {
	switch targetType {
	case TargetPhysicalBinding:
		return identity.PhysicalBinding, nil
	case TargetModelGrain:
		return identity.ModelGrain, nil
	case TargetEntityKey:
		return identity.EntityKey, nil
	case TargetJoinContract:
		return identity.JoinContract, nil
	default:
		return "", ErrInvalidArgument
	}
}

func validContentObject(content json.RawMessage) bool {
	if len(content) == 0 {
		return true
	}
	if !json.Valid(content) {
		return false
	}
	var decoded any
	if err := json.Unmarshal(content, &decoded); err != nil {
		return false
	}
	_, isObject := decoded.(map[string]any)
	return isObject
}

func nonEmptyText(value string, maxLength int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= maxLength
}

func validFieldRefs(refs []identity.PhysicalFieldID) bool {
	return len(refs) > 0 && len(refs) <= maxFieldRefs
}

// PhysicalBinding ties one semantic asset to one M1 physical target: a
// dataset plus an optional field refinement (D-014). RetiredAt marks
// superseded bindings; a nil RetiredAt is an active binding, and the storage
// layer admits at most one active binding per (asset, physical target).
type PhysicalBinding struct {
	ID          identity.PhysicalBindingID
	WorkspaceID identity.WorkspaceID
	AssetID     identity.AssetID
	DatasetID   identity.PhysicalDatasetID
	FieldID     *identity.PhysicalFieldID
	Transform   *string
	RetiredAt   *time.Time
	Version     int
	Content     json.RawMessage
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Validate enforces the packet structural rule: a binding requires the asset
// side and at least one physical reference (the dataset; the field refines it).
func (binding PhysicalBinding) Validate() error {
	if binding.ID.IsZero() {
		return ErrInvalidArgument
	}
	if err := binding.validateSemantic(); err != nil {
		return err
	}
	if binding.CreatedBy == "" || len(binding.CreatedBy) > 256 {
		return ErrInvalidArgument
	}
	return nil
}

// validateSemantic checks the structural content before any identity is
// minted; Validate() additionally requires the persisted-record fields.
func (binding PhysicalBinding) validateSemantic() error {
	if binding.WorkspaceID.IsZero() || binding.AssetID.IsZero() || binding.DatasetID.IsZero() {
		return ErrInvalidArgument
	}
	if binding.Transform != nil && !nonEmptyText(*binding.Transform, 4096) {
		return ErrInvalidArgument
	}
	if !validContentObject(binding.Content) {
		return ErrInvalidArgument
	}
	return nil
}

// ModelGrain documents the grain of a semantic asset: the grain statement
// plus the physical fields the grain is measurable on.
type ModelGrain struct {
	ID              identity.ModelGrainID
	WorkspaceID     identity.WorkspaceID
	AssetID         identity.AssetID
	GrainExpression string
	GrainFieldRefs  []identity.PhysicalFieldID
	DocumentedBy    *identity.EvidenceID
	Version         int
	Content         json.RawMessage
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Validate enforces the packet structural rule: a model grain requires a
// non-empty grain statement and non-empty grain field references.
func (grain ModelGrain) Validate() error {
	if grain.ID.IsZero() {
		return ErrInvalidArgument
	}
	if err := grain.validateSemantic(); err != nil {
		return err
	}
	if grain.CreatedBy == "" || len(grain.CreatedBy) > 256 {
		return ErrInvalidArgument
	}
	return nil
}

func (grain ModelGrain) validateSemantic() error {
	if grain.WorkspaceID.IsZero() || grain.AssetID.IsZero() {
		return ErrInvalidArgument
	}
	if !nonEmptyText(grain.GrainExpression, 4096) {
		return ErrInvalidArgument
	}
	if !validFieldRefs(grain.GrainFieldRefs) {
		return ErrInvalidArgument
	}
	if !validContentObject(grain.Content) {
		return ErrInvalidArgument
	}
	return nil
}

// EntityKey is the uniqueness contract of an entity asset: the physical key
// fields plus whether the source yields exact or deduplicated uniqueness.
type EntityKey struct {
	ID                  identity.EntityKeyID
	WorkspaceID         identity.WorkspaceID
	AssetID             identity.AssetID
	KeyFieldRefs        []identity.PhysicalFieldID
	UniquenessSemantics UniquenessSemantics
	Version             int
	Content             json.RawMessage
	CreatedBy           string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Validate enforces the packet structural rule: an entity key requires
// non-empty key field references and a legal uniqueness semantics.
func (key EntityKey) Validate() error {
	if key.ID.IsZero() {
		return ErrInvalidArgument
	}
	if err := key.validateSemantic(); err != nil {
		return err
	}
	if key.CreatedBy == "" || len(key.CreatedBy) > 256 {
		return ErrInvalidArgument
	}
	return nil
}

func (key EntityKey) validateSemantic() error {
	if key.WorkspaceID.IsZero() || key.AssetID.IsZero() {
		return ErrInvalidArgument
	}
	if !validFieldRefs(key.KeyFieldRefs) {
		return ErrInvalidArgument
	}
	if !key.UniquenessSemantics.Valid() {
		return ErrInvalidArgument
	}
	if !validContentObject(key.Content) {
		return ErrInvalidArgument
	}
	return nil
}

// JoinContract is a governed join between two physical datasets (D-014) with
// explicit field pairs, join type and row multiplicity.
type JoinContract struct {
	ID             identity.JoinContractID
	WorkspaceID    identity.WorkspaceID
	LeftDatasetID  identity.PhysicalDatasetID
	RightDatasetID identity.PhysicalDatasetID
	LeftFieldRefs  []identity.PhysicalFieldID
	RightFieldRefs []identity.PhysicalFieldID
	JoinType       JoinType
	Cardinality    JoinCardinality
	JoinExpression string
	ContractNotes  *string
	Version        int
	Content        json.RawMessage
	CreatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Validate enforces the packet structural rule: a join contract requires both
// sides, matching reference counts, and legal join type/cardinality.
func (contract JoinContract) Validate() error {
	if contract.ID.IsZero() {
		return ErrInvalidArgument
	}
	if err := contract.validateSemantic(); err != nil {
		return err
	}
	if contract.CreatedBy == "" || len(contract.CreatedBy) > 256 {
		return ErrInvalidArgument
	}
	return nil
}

func (contract JoinContract) validateSemantic() error {
	if contract.WorkspaceID.IsZero() ||
		contract.LeftDatasetID.IsZero() || contract.RightDatasetID.IsZero() {
		return ErrInvalidArgument
	}
	if !validFieldRefs(contract.LeftFieldRefs) || !validFieldRefs(contract.RightFieldRefs) {
		return ErrInvalidArgument
	}
	if len(contract.LeftFieldRefs) != len(contract.RightFieldRefs) {
		return ErrInvalidArgument
	}
	if !contract.JoinType.Valid() || !contract.Cardinality.Valid() {
		return ErrInvalidArgument
	}
	if !nonEmptyText(contract.JoinExpression, 4096) {
		return ErrInvalidArgument
	}
	if contract.ContractNotes != nil && !nonEmptyText(*contract.ContractNotes, 4096) {
		return ErrInvalidArgument
	}
	if !validContentObject(contract.Content) {
		return ErrInvalidArgument
	}
	return nil
}

// GovernedObject is the transport envelope of the four governance object
// types: exactly one payload is set and it matches Type. It lets the
// governed-object pipeline (create, read, list, apply) stay one shared
// implementation instead of four divergent ones.
type GovernedObject struct {
	Type            TargetObjectType
	PhysicalBinding *PhysicalBinding
	ModelGrain      *ModelGrain
	EntityKey       *EntityKey
	JoinContract    *JoinContract
}

// Validate checks the envelope shape and defers to the semantic payload
// validation, so the envelope also works for pre-persist payloads whose
// identity is minted by the application service.
func (object GovernedObject) Validate() error {
	if !object.Type.IsGovernedObject() {
		return ErrInvalidArgument
	}
	set := 0
	if object.PhysicalBinding != nil {
		set++
		if object.Type != TargetPhysicalBinding {
			return ErrInvalidArgument
		}
		if err := object.PhysicalBinding.validateSemantic(); err != nil {
			return err
		}
	}
	if object.ModelGrain != nil {
		set++
		if object.Type != TargetModelGrain {
			return ErrInvalidArgument
		}
		if err := object.ModelGrain.validateSemantic(); err != nil {
			return err
		}
	}
	if object.EntityKey != nil {
		set++
		if object.Type != TargetEntityKey {
			return ErrInvalidArgument
		}
		if err := object.EntityKey.validateSemantic(); err != nil {
			return err
		}
	}
	if object.JoinContract != nil {
		set++
		if object.Type != TargetJoinContract {
			return ErrInvalidArgument
		}
		if err := object.JoinContract.validateSemantic(); err != nil {
			return err
		}
	}
	if set != 1 {
		return ErrInvalidArgument
	}
	return nil
}

// GovernedPatch is the payload of one governed change application: the new
// flexible content object plus the human-readable summary recorded in the
// audit fact. Structural columns change through their own proposal packets;
// the patch carries the flexible content only.
type GovernedPatch struct {
	Content json.RawMessage
	Summary string
}

// Validate requires a JSON object payload and a bounded non-empty summary.
func (patch GovernedPatch) Validate() error {
	if !validContentObject(patch.Content) || len(patch.Content) == 0 {
		return ErrInvalidArgument
	}
	if !nonEmptyText(patch.Summary, 512) {
		return ErrInvalidArgument
	}
	return nil
}
