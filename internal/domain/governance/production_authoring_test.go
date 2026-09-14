package governance_test

import (
	"encoding/json"
	"strings"
	"testing"

	governance "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestProductionCanonicalInputRejectsNonStringSource(t *testing.T) {
	for _, source := range []string{`{}`, `[]`, `42`, `null`} {
		raw := `{"snapshots":[{"snapshotId":"first","sourceId":` + source + `},{"snapshotId":"second","sourceId":` + source + `}]}`
		if _, err := governance.CanonicalProductionInput(json.RawMessage(raw)); err == nil {
			t.Fatalf("non-string source accepted: %s", source)
		}
	}
}

func TestProductionReferenceResolutionPreservesNumberLiterals(t *testing.T) {
	input := json.RawMessage(`{"large":9007199254740993,"decimal":1.0,"exponent":1e+3}`)
	resolved, err := governance.ResolveLocalReferences(input, nil)
	if err != nil {
		t.Fatal(err)
	}
	want, err := governance.CanonicalJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(resolved) != string(want) {
		t.Fatalf("numeric content changed: %s != %s", resolved, want)
	}
}

func TestProductionContentSchema(t *testing.T) {
	principal, _ := identity.NewPrincipalID()
	valid := `{"address":"finance.revenue","assetType":"metric","displayName":"Revenue","definition":null,"scope":null,"ownerPrincipalId":"` + principal.String() + `"}`
	key := "finance.revenue"
	target := governance.TargetDeclaration{LocalKey: "revenue", Title: "Revenue", Kind: governance.TargetKindSemanticAsset, Intent: governance.ProductionIntentCreate, IdentityKey: &key, Content: json.RawMessage(valid)}
	if err := governance.ValidateProductionContentDeclarations([]governance.TargetDeclaration{target}); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		strings.Replace(valid, `"displayName":"Revenue",`, "", 1),
		strings.Replace(valid, `"assetType":"metric"`, `"assetType":"made_up"`, 1),
		strings.Replace(valid, `"definition":null`, `"definition":12`, 1),
		strings.Replace(valid, `"scope":null`, `"scope":null,"approved":true`, 1),
	} {
		target.Content = json.RawMessage(raw)
		if err := governance.ValidateProductionContentDeclarations([]governance.TargetDeclaration{target}); err == nil {
			t.Fatalf("invalid schema accepted: %s", raw)
		}
	}
}

func TestProductionRequestDigestIgnoresSetOrder(t *testing.T) {
	a := governance.RequestDigestInput{WorkspaceID: "workspace", Targets: []governance.TargetDeclaration{
		{LocalKey: "b", Title: " B ", EvidenceIDs: []string{"z", "a"}}, {LocalKey: "a", Title: "A"},
	}}
	b := governance.RequestDigestInput{WorkspaceID: "workspace", Targets: []governance.TargetDeclaration{
		{LocalKey: "a", Title: "A"}, {LocalKey: "b", Title: "B", EvidenceIDs: []string{"a", "z"}},
	}}
	da, ea := governance.ComputeRequestDigest(a)
	db, eb := governance.ComputeRequestDigest(b)
	if ea != nil || eb != nil || da != db {
		t.Fatalf("unordered declaration digest differs: %s %s %v %v", da, db, ea, eb)
	}
}

func TestProductionRequestRejectsDuplicateEvidenceAndOversize(t *testing.T) {
	for _, target := range []governance.TargetDeclaration{
		{LocalKey: "a", EvidenceIDs: []string{"same", "same"}},
		{LocalKey: "a", Content: json.RawMessage(`{"text":"` + strings.Repeat("x", governance.MaxCanonicalInputBytes) + `"}`)},
	} {
		if _, err := governance.ComputeRequestDigest(governance.RequestDigestInput{Targets: []governance.TargetDeclaration{target}}); err == nil {
			t.Fatal("invalid declaration accepted")
		}
	}
}

func TestProductionCanonicalInputSets(t *testing.T) {
	a := json.RawMessage(`{"snapshots":[{"sourceId":"s","snapshotId":"b","coverageKeys":["z","a"]},{"sourceId":"s","snapshotId":"a","coverageKeys":["b"]}],"candidates":[],"evidence":[],"dependencies":[]}`)
	b := json.RawMessage(`{"snapshots":[{"sourceId":"s","snapshotId":"a","coverageKeys":["b"]},{"sourceId":"s","snapshotId":"b","coverageKeys":["a","z"]}],"candidates":[],"evidence":[],"dependencies":[]}`)
	x, err := governance.CanonicalProductionInput(a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := governance.CanonicalProductionInput(b)
	if err != nil || string(x) != string(y) {
		t.Fatalf("set order changed input: %s %s %v", x, y, err)
	}
	for _, raw := range []string{`{"snapshots":[],"snapshots":[]}`, `{"evidence":[{"evidenceId":"x"},{"evidenceId":"x"}]}`, `{"snapshots":[{"snapshotId":"x","coverageKeys":["x","x"]}]}`} {
		if _, err := governance.CanonicalProductionInput(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted ambiguous input: %s", raw)
		}
	}
}

func TestProductionStructuredReferences(t *testing.T) {
	input := json.RawMessage(`{"asset":{"localKey":"order"},"definition":"local:order"}`)
	got, err := governance.ResolveLocalReferences(input, map[string]string{"order": "asset_id"})
	if err != nil || string(got) != `{"asset":"asset_id","definition":"local:order"}` {
		t.Fatalf("structured reference or business text changed incorrectly: %s, %v", got, err)
	}
	if _, err := governance.ResolveLocalReferences(json.RawMessage(`{"asset":{"localKey":"missing"}}`), nil); err == nil {
		t.Fatal("missing structured reference accepted")
	}
}

func TestProductionCandidateTargetKeysRejectDuplicates(t *testing.T) {
	err := governance.ValidateCandidateDeclarations([]governance.CandidateDeclaration{{
		CandidateID: "candidate", CandidateDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		PrimaryTargetKey: "asset", TargetKeys: []string{"asset", "asset"},
	}}, []governance.TargetDeclaration{{LocalKey: "asset"}})
	if err == nil {
		t.Fatal("duplicate candidate target keys accepted")
	}
}
