package governance_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func realDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func updateItem(path, before, after string) governance.ChangeSetItem {
	return governance.ChangeSetItem{
		FieldPath:    path,
		Op:           governance.ChangeUpdate,
		BeforeDigest: realDigest(before),
		AfterDigest:  realDigest(after),
		BeforeValue:  json.RawMessage(before),
		AfterValue:   json.RawMessage(after),
	}
}

func findingCodes(findings []governanceapp.Finding) []string {
	codes := make([]string, 0, len(findings))
	for _, finding := range findings {
		codes = append(codes, finding.Code)
	}
	return codes
}

func runValidator(t *testing.T, registry *governanceapp.Registry, id string, input governanceapp.ValidationInput) []governanceapp.Finding {
	t.Helper()
	validator, ok := registry.Get(id)
	if !ok {
		t.Fatalf("registry is missing validator %q", id)
	}
	findings, err := validator.Validate(input)
	if err != nil {
		t.Fatalf("validator %s returned error: %v", id, err)
	}
	return findings
}

func TestSchemaValidatorCleanChangeSetProducesNoFindings(t *testing.T) {
	registry := governanceapp.NewDefaultRegistry()
	input := registryInput()
	input.Changes = []governance.ChangeSetItem{updateItem("definition", `"Revenue after refunds"`, `"Revenue after refunds and chargebacks"`)}
	if findings := runValidator(t, registry, "schema", input); len(findings) != 0 {
		t.Fatalf("clean change-set produced findings: %+v", findings)
	}
}

func TestSchemaValidatorFlagsOpValueShapeViolations(t *testing.T) {
	registry := governanceapp.NewDefaultRegistry()
	input := registryInput()
	input.Changes = []governance.ChangeSetItem{{
		FieldPath: "definition", Op: governance.ChangeAdd,
		BeforeDigest: realDigest("x"), BeforeValue: json.RawMessage(`"x"`),
		AfterDigest: realDigest("y"), AfterValue: json.RawMessage(`"y"`),
	}}
	findings := runValidator(t, registry, "schema", input)
	if len(findings) == 0 || findings[0].Code != "SCHEMA_ITEM_SHAPE" || findings[0].Severity != governance.SeverityBlocker {
		t.Fatalf("add-with-before not flagged as blocker: %+v", findings)
	}
}

func TestSchemaValidatorFlagsRequiredFieldRemovalPerTargetType(t *testing.T) {
	registry := governanceapp.NewDefaultRegistry()
	cases := []struct {
		targetType governance.TargetObjectType
		fieldPath  string
	}{
		{governance.TargetSemanticAsset, "name"},
		{governance.TargetSemanticAsset, "definition"},
		{governance.TargetPhysicalBinding, "dataset"},
		{governance.TargetModelGrain, "grainExpression"},
		{governance.TargetEntityKey, "keyFieldRefs"},
		{governance.TargetJoinContract, "joinExpression"},
	}
	for _, testCase := range cases {
		input := registryInput()
		input.Proposal.TargetType = testCase.targetType
		input.Changes = []governance.ChangeSetItem{{
			FieldPath: testCase.fieldPath, Op: governance.ChangeRemove,
			BeforeDigest: realDigest("v"), BeforeValue: json.RawMessage(`"v"`),
		}}
		findings := runValidator(t, registry, "schema", input)
		if len(findings) == 0 || findings[0].Code != "SCHEMA_REQUIRED_FIELD_REMOVED" ||
			findings[0].Severity != governance.SeverityBlocker {
			t.Fatalf("%s removal of %s not flagged: %+v", testCase.targetType, testCase.fieldPath, findings)
		}
	}
}

func TestSchemaValidatorFlagsMissingSemanticAssetIdentity(t *testing.T) {
	registry := governanceapp.NewDefaultRegistry()
	input := registryInput()
	input.Proposal.AssetID = ""
	input.Proposal.BaseRevisionID = ""
	input.Changes = []governance.ChangeSetItem{updateItem("definition", `"a"`, `"b"`)}
	findings := runValidator(t, registry, "schema", input)
	if len(findings) == 0 || findings[0].Code != "SCHEMA_TARGET_IDENTITY" {
		t.Fatalf("missing asset identity not flagged: %+v", findings)
	}
}

func TestReferenceValidatorResolvesChangeSetReferences(t *testing.T) {
	registry := governanceapp.NewDefaultRegistry()
	input := registryInput()
	input.Changes = []governance.ChangeSetItem{updateItem(
		"content.relatedAsset",
		`"ast_01arz3ndektsv4rrffq69g5fav"`,
		`"ast_01arz3ndektsv4rrffq69g5fbx"`,
	)}
	input.Target.Resolutions = map[string]governanceapp.ReferenceResolution{
		"ast_01arz3ndektsv4rrffq69g5fav": {Kind: governanceapp.ReferenceKindAsset, Exists: true},
		"ast_01arz3ndektsv4rrffq69g5fbx": {Kind: governanceapp.ReferenceKindAsset, Exists: false},
	}
	findings := runValidator(t, registry, "reference", input)
	if len(findings) != 1 || findings[0].Code != "REFERENCE_UNRESOLVED" ||
		findings[0].Severity != governance.SeverityBlocker {
		t.Fatalf("unresolved reference not flagged as blocker: %+v", findings)
	}
	if !strings.Contains(findings[0].Message, "ast_01arz3ndektsv4rrffq69g5fbx") {
		t.Fatalf("finding does not name the unresolved reference: %+v", findings[0])
	}
}

func TestReferenceValidatorFlagsMissingTarget(t *testing.T) {
	registry := governanceapp.NewDefaultRegistry()
	input := registryInput()
	input.Target.AssetExists = false
	findings := runValidator(t, registry, "reference", input)
	if len(findings) == 0 || findings[0].Code != "REFERENCE_TARGET_MISSING" ||
		findings[0].Severity != governance.SeverityBlocker {
		t.Fatalf("missing target asset not flagged: %+v", findings)
	}
	governed := registryInput()
	governed.Proposal.TargetType = governance.TargetJoinContract
	governed.Proposal.AssetID = ""
	governed.Proposal.BaseRevisionID = ""
	governed.Target.AssetExists = false
	governed.Target.BaseRevisionExists = false
	governed.Target.TargetObjectExists = false
	findings = runValidator(t, registry, "reference", governed)
	if len(findings) == 0 || findings[0].Code != "REFERENCE_TARGET_MISSING" {
		t.Fatalf("missing governed target not flagged: %+v", findings)
	}
}

func TestReferenceValidatorIgnoresPlainStrings(t *testing.T) {
	registry := governanceapp.NewDefaultRegistry()
	input := registryInput()
	input.Changes = []governance.ChangeSetItem{updateItem(
		"definition", `"Revenue after refunds"`, `"Revenue after refunds and chargebacks"`,
	)}
	if findings := runValidator(t, registry, "reference", input); len(findings) != 0 {
		t.Fatalf("plain text values must not be reference-checked: %+v", findings)
	}
}

func TestStructuralValidatorFlagsDigestMismatch(t *testing.T) {
	registry := governanceapp.NewDefaultRegistry()
	input := registryInput()
	item := updateItem("definition", `"Revenue after refunds"`, `"Revenue after refunds and chargebacks"`)
	item.AfterDigest = realDigest("something else")
	input.Changes = []governance.ChangeSetItem{item}
	findings := runValidator(t, registry, "structural", input)
	if len(findings) == 0 || findings[0].Code != "STRUCTURAL_DIGEST_MISMATCH" ||
		findings[0].Severity != governance.SeverityBlocker {
		t.Fatalf("digest mismatch not flagged as blocker: %+v", findings)
	}
}

func TestStructuralValidatorSignalsNoOpUpdates(t *testing.T) {
	// SSOT §8.2 no-op detection: an update whose digests are equal is not a
	// substantive diff; the validator signals it without blocking.
	registry := governanceapp.NewDefaultRegistry()
	input := registryInput()
	item := updateItem("definition", `"Revenue after refunds"`, `"Revenue after refunds"`)
	input.Changes = []governance.ChangeSetItem{item}
	findings := runValidator(t, registry, "structural", input)
	if len(findings) != 1 || findings[0].Code != "STRUCTURAL_NO_OP" ||
		findings[0].Severity != governance.SeverityWarning {
		t.Fatalf("no-op update not signalled as warning: %+v", findings)
	}
}

func TestStructuralValidatorCleanChangeSetProducesNoFindings(t *testing.T) {
	registry := governanceapp.NewDefaultRegistry()
	input := registryInput()
	input.Changes = []governance.ChangeSetItem{
		updateItem("definition", `"Revenue after refunds"`, `"Revenue after refunds and chargebacks"`),
	}
	if findings := runValidator(t, registry, "structural", input); len(findings) != 0 {
		t.Fatalf("consistent digests produced findings: %+v", findings)
	}
}

func TestReferenceValidatorResolvesFieldReferencesAgainstPhysicalGraph(t *testing.T) {
	// The M1 physical graph backs field references: a change-set value
	// carrying a pfd_ TypeID must resolve against physical fields.
	registry := governanceapp.NewDefaultRegistry()
	fieldID := mustFieldID(t)
	missingID := mustFieldID(t)
	input := registryInput()
	input.Proposal.TargetType = governance.TargetPhysicalBinding
	input.Proposal.AssetID = ""
	input.Proposal.BaseRevisionID = ""
	input.Changes = []governance.ChangeSetItem{updateItem(
		"content.fieldRefs",
		`["`+fieldID.String()+`"]`,
		`["`+fieldID.String()+`","`+missingID.String()+`"]`,
	)}
	input.Target.AssetExists = false
	input.Target.BaseRevisionExists = false
	input.Target.TargetObjectExists = true
	input.Target.Resolutions = map[string]governanceapp.ReferenceResolution{
		fieldID.String():   {Kind: governanceapp.ReferenceKindField, Exists: true},
		missingID.String(): {Kind: governanceapp.ReferenceKindField, Exists: false},
	}
	findings := runValidator(t, registry, "reference", input)
	if len(findings) != 1 || findings[0].Code != "REFERENCE_UNRESOLVED" {
		t.Fatalf("missing field reference not flagged: %+v", findings)
	}
}

func mustFieldID(t *testing.T) identity.PhysicalFieldID {
	t.Helper()
	id, err := identity.NewPhysicalFieldID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
