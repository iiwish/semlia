package governance_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func mustPhysicalFieldRef(t *testing.T) identity.PhysicalFieldID {
	t.Helper()
	ref, err := identity.NewPhysicalFieldID()
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func mustPhysicalDatasetRef(t *testing.T) identity.PhysicalDatasetID {
	t.Helper()
	ref, err := identity.NewPhysicalDatasetID()
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestPhysicalBindingValidationRequiresAssetAndPhysicalReference(t *testing.T) {
	assetID, err := identity.NewAssetID()
	if err != nil {
		t.Fatal(err)
	}
	datasetID := mustPhysicalDatasetRef(t)
	fieldID := mustPhysicalFieldRef(t)
	valid := governance.PhysicalBinding{
		ID: func() identity.PhysicalBindingID {
			id, idErr := identity.NewPhysicalBindingID()
			if idErr != nil {
				t.Fatal(idErr)
			}
			return id
		}(),
		WorkspaceID: func() identity.WorkspaceID {
			id, idErr := identity.NewWorkspaceID()
			if idErr != nil {
				t.Fatal(idErr)
			}
			return id
		}(),
		AssetID:   assetID,
		DatasetID: datasetID,
		FieldID:   &fieldID,
		Content:   json.RawMessage(`{}`),
		CreatedBy: "steward",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid physical binding rejected: %v", err)
	}
	if valid.ID.Prefix() != identity.PhysicalBinding {
		t.Fatalf("physical binding prefix = %s, want phb", valid.ID.Prefix())
	}

	// Packet red scenario: a binding without the asset side or without a
	// physical reference is structurally invalid and must be rejected in the
	// domain before any persistence happens.
	cases := map[string]func(*governance.PhysicalBinding){
		"missing asset":          func(binding *governance.PhysicalBinding) { binding.AssetID = identity.AssetID{} },
		"missing dataset":        func(binding *governance.PhysicalBinding) { binding.DatasetID = identity.PhysicalDatasetID{} },
		"missing workspace":      func(binding *governance.PhysicalBinding) { binding.WorkspaceID = identity.WorkspaceID{} },
		"missing created_by":     func(binding *governance.PhysicalBinding) { binding.CreatedBy = "" },
		"non-object content":     func(binding *governance.PhysicalBinding) { binding.Content = json.RawMessage(`[]`) },
		"malformed json content": func(binding *governance.PhysicalBinding) { binding.Content = json.RawMessage(`{`) },
	}
	for name, mutate := range cases {
		binding := valid
		mutate(&binding)
		if err := binding.Validate(); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("%s: physical binding accepted (%v), want ErrInvalidArgument", name, err)
		}
	}
}

func TestModelGrainValidationRequiresGrainExpressionAndFieldRefs(t *testing.T) {
	assetID, err := identity.NewAssetID()
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	grainID, err := identity.NewModelGrainID()
	if err != nil {
		t.Fatal(err)
	}
	if grainID.Prefix() != identity.ModelGrain {
		t.Fatalf("model grain prefix = %s, want mgn", grainID.Prefix())
	}
	valid := governance.ModelGrain{
		ID:              grainID,
		WorkspaceID:     workspaceID,
		AssetID:         assetID,
		GrainExpression: "one row per order line",
		GrainFieldRefs:  []identity.PhysicalFieldID{mustPhysicalFieldRef(t), mustPhysicalFieldRef(t)},
		Content:         json.RawMessage(`{}`),
		CreatedBy:       "steward",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid model grain rejected: %v", err)
	}
	// Evidence links are optional but must reference evidence artifacts when set.
	evidenceID, err := identity.NewEvidenceID()
	if err != nil {
		t.Fatal(err)
	}
	documented := valid
	documented.DocumentedBy = &evidenceID
	if err := documented.Validate(); err != nil {
		t.Fatalf("documented model grain rejected: %v", err)
	}

	cases := map[string]func(*governance.ModelGrain){
		"empty grain expression": func(grain *governance.ModelGrain) { grain.GrainExpression = "   " },
		"missing grain fields":   func(grain *governance.ModelGrain) { grain.GrainFieldRefs = nil },
		"missing asset":          func(grain *governance.ModelGrain) { grain.AssetID = identity.AssetID{} },
		"missing workspace":      func(grain *governance.ModelGrain) { grain.WorkspaceID = identity.WorkspaceID{} },
		"non-object content":     func(grain *governance.ModelGrain) { grain.Content = json.RawMessage(`"x"`) },
		"too long grain expressio": func(grain *governance.ModelGrain) {
			grain.GrainExpression = makeString(4097)
		},
	}
	for name, mutate := range cases {
		grain := valid
		mutate(&grain)
		if err := grain.Validate(); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("%s: model grain accepted (%v), want ErrInvalidArgument", name, err)
		}
	}
}

func TestEntityKeyValidationRequiresKeyFieldsAndLegalUniquenessSemantics(t *testing.T) {
	assetID, err := identity.NewAssetID()
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	keyID, err := identity.NewEntityKeyID()
	if err != nil {
		t.Fatal(err)
	}
	if keyID.Prefix() != identity.EntityKey {
		t.Fatalf("entity key prefix = %s, want eky", keyID.Prefix())
	}
	valid := governance.EntityKey{
		ID:                  keyID,
		WorkspaceID:         workspaceID,
		AssetID:             assetID,
		KeyFieldRefs:        []identity.PhysicalFieldID{mustPhysicalFieldRef(t)},
		UniquenessSemantics: governance.UniquenessExact,
		Content:             json.RawMessage(`{}`),
		CreatedBy:           "steward",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid entity key rejected: %v", err)
	}
	deduplicated := valid
	deduplicated.UniquenessSemantics = governance.UniquenessDeduplicated
	if err := deduplicated.Validate(); err != nil {
		t.Fatalf("deduplicated entity key rejected: %v", err)
	}

	cases := map[string]func(*governance.EntityKey){
		"missing key fields":           func(key *governance.EntityKey) { key.KeyFieldRefs = nil },
		"illegal uniqueness semantics": func(key *governance.EntityKey) { key.UniquenessSemantics = "fuzzy" },
		"empty uniqueness semantics":   func(key *governance.EntityKey) { key.UniquenessSemantics = "" },
		"missing asset":                func(key *governance.EntityKey) { key.AssetID = identity.AssetID{} },
		"missing workspace":            func(key *governance.EntityKey) { key.WorkspaceID = identity.WorkspaceID{} },
		"non-object content":           func(key *governance.EntityKey) { key.Content = json.RawMessage(`3`) },
	}
	for name, mutate := range cases {
		key := valid
		mutate(&key)
		if err := key.Validate(); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("%s: entity key accepted (%v), want ErrInvalidArgument", name, err)
		}
	}
}

func TestJoinContractValidationRequiresBothSidesMatchingCountsAndLegalVocabulary(t *testing.T) {
	workspaceID, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	contractID, err := identity.NewJoinContractID()
	if err != nil {
		t.Fatal(err)
	}
	if contractID.Prefix() != identity.JoinContract {
		t.Fatalf("join contract prefix = %s, want jct", contractID.Prefix())
	}
	valid := governance.JoinContract{
		ID:             contractID,
		WorkspaceID:    workspaceID,
		LeftDatasetID:  mustPhysicalDatasetRef(t),
		RightDatasetID: mustPhysicalDatasetRef(t),
		LeftFieldRefs:  []identity.PhysicalFieldID{mustPhysicalFieldRef(t), mustPhysicalFieldRef(t)},
		RightFieldRefs: []identity.PhysicalFieldID{mustPhysicalFieldRef(t), mustPhysicalFieldRef(t)},
		JoinType:       governance.JoinInner,
		Cardinality:    governance.CardinalityOneToMany,
		JoinExpression: "left.id = right.left_id",
		Content:        json.RawMessage(`{}`),
		CreatedBy:      "steward",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid join contract rejected: %v", err)
	}
	notes := "validated against warehouse DDL"
	valid.ContractNotes = &notes
	if err := valid.Validate(); err != nil {
		t.Fatalf("join contract with notes rejected: %v", err)
	}

	cases := map[string]func(*governance.JoinContract){
		"missing left side":       func(contract *governance.JoinContract) { contract.LeftDatasetID = identity.PhysicalDatasetID{} },
		"missing right side":      func(contract *governance.JoinContract) { contract.RightDatasetID = identity.PhysicalDatasetID{} },
		"empty left refs":         func(contract *governance.JoinContract) { contract.LeftFieldRefs = nil },
		"empty right refs":        func(contract *governance.JoinContract) { contract.RightFieldRefs = nil },
		"mismatched ref counts":   func(contract *governance.JoinContract) { contract.RightFieldRefs = contract.RightFieldRefs[:1] },
		"illegal join type":       func(contract *governance.JoinContract) { contract.JoinType = "cross" },
		"illegal cardinality":     func(contract *governance.JoinContract) { contract.Cardinality = "one_to_zero" },
		"missing join expression": func(contract *governance.JoinContract) { contract.JoinExpression = "  " },
		"missing workspace":       func(contract *governance.JoinContract) { contract.WorkspaceID = identity.WorkspaceID{} },
		"non-object content":      func(contract *governance.JoinContract) { contract.Content = json.RawMessage(`true`) },
		"empty contract notes":    func(contract *governance.JoinContract) { empty := ""; contract.ContractNotes = &empty },
	}
	for name, mutate := range cases {
		contract := valid
		contract.ContractNotes = nil
		mutate(&contract)
		if err := contract.Validate(); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("%s: join contract accepted (%v), want ErrInvalidArgument", name, err)
		}
	}
}

func TestGovernedObjectVocabularyIncludesPhysicalBinding(t *testing.T) {
	for _, objectType := range []governance.TargetObjectType{
		governance.TargetPhysicalBinding, governance.TargetModelGrain,
		governance.TargetEntityKey, governance.TargetJoinContract,
	} {
		if !objectType.IsGovernedObject() {
			t.Fatalf("%s must be a governed object type", objectType)
		}
	}
	if governance.TargetSemanticAsset.IsGovernedObject() {
		t.Fatal("semantic_asset targets follow the asset pipeline, not the governed object pipeline")
	}
	if governance.TargetObjectType("deployed").IsGovernedObject() {
		t.Fatal("unknown object types must not be governed objects")
	}
}

func TestGovernedObjectEnvelopeValidatesExactlyOnePayload(t *testing.T) {
	assetID, err := identity.NewAssetID()
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	grain := governance.ModelGrain{
		WorkspaceID:     workspaceID,
		AssetID:         assetID,
		GrainExpression: "one row per order",
		GrainFieldRefs:  []identity.PhysicalFieldID{mustPhysicalFieldRef(t)},
		Content:         json.RawMessage(`{}`),
		CreatedBy:       "steward",
	}
	envelope := governance.GovernedObject{Type: governance.TargetModelGrain, ModelGrain: &grain}
	if err := envelope.Validate(); err != nil {
		t.Fatalf("valid governed object envelope rejected: %v", err)
	}

	empty := governance.GovernedObject{Type: governance.TargetModelGrain}
	if err := empty.Validate(); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("envelope without payload accepted (%v), want ErrInvalidArgument", err)
	}
	mismatched := governance.GovernedObject{Type: governance.TargetEntityKey, ModelGrain: &grain}
	if err := mismatched.Validate(); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("envelope with mismatched payload accepted (%v), want ErrInvalidArgument", err)
	}
	unknown := governance.GovernedObject{Type: governance.TargetSemanticAsset, ModelGrain: &grain}
	if err := unknown.Validate(); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("envelope with non-governed type accepted (%v), want ErrInvalidArgument", err)
	}
}

func TestGovernedPatchValidatesContentObjectAndSummary(t *testing.T) {
	valid := governance.GovernedPatch{Content: json.RawMessage(`{"grain":"per order"}`), Summary: "tighten grain"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid governed patch rejected: %v", err)
	}
	cases := map[string]governance.GovernedPatch{
		"non-object content": {Content: json.RawMessage(`[1]`), Summary: "s"},
		"malformed content":  {Content: json.RawMessage(`{`), Summary: "s"},
		"empty summary":      {Content: json.RawMessage(`{}`), Summary: "  "},
		"oversized summary":  {Content: json.RawMessage(`{}`), Summary: makeString(513)},
	}
	for name, patch := range cases {
		if err := patch.Validate(); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("%s: governed patch accepted (%v), want ErrInvalidArgument", name, err)
		}
	}
}

func makeString(length int) string {
	buffer := make([]byte, length)
	for index := range buffer {
		buffer[index] = 'a'
	}
	return string(buffer)
}
