package governance

import (
	"encoding/json"
	"github.com/iiwish/semlia/pkg/identity"
	"testing"
)

func TestApplyChangeSetAppliesAddUpdateRemove(t *testing.T) {
	content := json.RawMessage(`{"name":"Net revenue","definition":"Revenue after refunds","stale":"drop me"}`)
	updated, err := ApplyChangeSet(content, []ChangeSetItem{
		{FieldPath: "definition", Op: ChangeUpdate, AfterValue: json.RawMessage(`"Revenue after refunds and chargebacks"`)},
		{FieldPath: "unit", Op: ChangeAdd, AfterValue: json.RawMessage(`"USD"`)},
		{FieldPath: "stale", Op: ChangeRemove, BeforeValue: json.RawMessage(`"drop me"`)},
	})
	if err != nil {
		t.Fatalf("apply change-set: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatal(err)
	}
	if document["definition"] != "Revenue after refunds and chargebacks" ||
		document["unit"] != "USD" {
		t.Fatalf("applied content = %s", updated)
	}
	if _, exists := document["stale"]; exists {
		t.Fatalf("remove op did not delete the field: %s", updated)
	}
}

func TestApplyChangeSetRefusesInvalidAddAndUpdate(t *testing.T) {
	content := json.RawMessage(`{"definition":"Revenue after refunds"}`)
	if _, err := ApplyChangeSet(content, []ChangeSetItem{
		{FieldPath: "definition", Op: ChangeAdd, AfterValue: json.RawMessage(`"x"`)},
	}); err == nil {
		t.Fatal("add onto an existing field succeeded")
	}
	if _, err := ApplyChangeSet(content, []ChangeSetItem{
		{FieldPath: "missing", Op: ChangeUpdate, AfterValue: json.RawMessage(`"x"`)},
	}); err == nil {
		t.Fatal("update of a missing field succeeded")
	}
	if _, err := ApplyChangeSet(content, []ChangeSetItem{
		{FieldPath: "missing", Op: ChangeRemove},
	}); err == nil {
		t.Fatal("remove of a missing field succeeded")
	}
}

func TestApplyInverseChangeSetRestoresBeforeState(t *testing.T) {
	base := json.RawMessage(`{"definition":"Revenue after refunds","keep":"yes"}`)
	published, err := ApplyChangeSet(base, []ChangeSetItem{
		{FieldPath: "definition", Op: ChangeUpdate, BeforeValue: json.RawMessage(`"Revenue after refunds"`), AfterValue: json.RawMessage(`"changed"`)},
		{FieldPath: "added", Op: ChangeAdd, AfterValue: json.RawMessage(`"new"`)},
	})
	if err != nil {
		t.Fatalf("apply change-set: %v", err)
	}
	var changed map[string]any
	if err := json.Unmarshal(published, &changed); err != nil {
		t.Fatal(err)
	}
	if changed["definition"] != "changed" || changed["added"] != "new" {
		t.Fatalf("publish application = %s", published)
	}
	restored, err := ApplyInverseChangeSet(published, []ChangeSetItem{
		{FieldPath: "definition", Op: ChangeUpdate, BeforeValue: json.RawMessage(`"Revenue after refunds"`), AfterValue: json.RawMessage(`"changed"`)},
		{FieldPath: "added", Op: ChangeAdd, AfterValue: json.RawMessage(`"new"`)},
	})
	if err != nil {
		t.Fatalf("apply inverse: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(restored, &document); err != nil {
		t.Fatal(err)
	}
	if document["definition"] != "Revenue after refunds" || document["keep"] != "yes" {
		t.Fatalf("inverse restore = %s", restored)
	}
	if _, exists := document["added"]; exists {
		t.Fatalf("inverse add removal did not delete the field: %s", restored)
	}
}

func TestManifestDigestPayloadWithObjectsKeepsAssetOnlyShape(t *testing.T) {
	entries := []ManifestEntry{{AssetID: mustAssetID(t), RevisionID: mustRevisionID(t), Compatibility: json.RawMessage(`{}`), Position: 1}}
	assetOnly, err := ManifestDigestPayload(entries)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := ManifestDigestPayloadWithObjects(entries, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(assetOnly) != string(unchanged) {
		t.Fatalf("asset-only payload changed shape: %s vs %s", assetOnly, unchanged)
	}
}

func TestManifestDigestPayloadWithObjectsIncludesObjectPins(t *testing.T) {
	entries := []ManifestEntry{{AssetID: mustAssetID(t), RevisionID: mustRevisionID(t), Compatibility: json.RawMessage(`{}`), Position: 1}}
	object := ObjectManifestEntry{ObjectType: TargetJoinContract, ObjectID: mustJoinContractUUID(t), Version: 2, Position: 1}
	withObjects, err := ManifestDigestPayloadWithObjects(entries, []ObjectManifestEntry{object})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ManifestDigestPayload(entries); err != nil {
		t.Fatal(err)
	}
	digest, err := DigestJSON(withObjects)
	if err != nil {
		t.Fatal(err)
	}
	if len(digest) != len("sha256:")+64 {
		t.Fatalf("digest = %q", digest)
	}
	if _, err := ManifestDigestPayloadWithObjects(entries, []ObjectManifestEntry{
		{ObjectType: TargetSemanticAsset, ObjectID: mustJoinContractUUID(t), Version: 1, Position: 1},
	}); err == nil {
		t.Fatal("semantic_asset is not a governed object pin")
	}
}

func mustAssetID(t *testing.T) identity.AssetID {
	t.Helper()
	value, err := identity.NewAssetID()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func mustRevisionID(t *testing.T) identity.RevisionID {
	t.Helper()
	value, err := identity.NewRevisionID()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func mustJoinContractUUID(t *testing.T) string {
	t.Helper()
	value, err := identity.NewJoinContractID()
	if err != nil {
		t.Fatal(err)
	}
	return value.UUID()
}
