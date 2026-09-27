package governance_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	governance "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestReplayProductionChangesPreservesVersionedKnowledgeSpecs(t *testing.T) {
	ref := baselineKnowledgeReference(t)
	nextRef := ref
	nextRef.RevisionID = baselineID(t, identity.NewRevisionID)
	memberRef, nextMemberRef := ref, nextRef
	memberRef.MemberID, nextMemberRef.MemberID = "amount", "corrected_amount"
	source := baselineSourceReference(t, "dataset")
	nextSource := baselineSourceReference(t, "dataset")
	field := baselineSourceReference(t, "field")
	nextField := baselineSourceReference(t, "field")
	field.SnapshotID, nextField.SnapshotID = source.SnapshotID, nextSource.SnapshotID

	for _, test := range []struct {
		kind          string
		before, after semantic.KnowledgeSpec
	}{
		{"analysis_model", semantic.KnowledgeSpec{BaseObjectRef: &ref, MetricRefs: []semantic.KnowledgeReference{ref}, MemberBindings: []semantic.MemberBinding{{SemanticRef: memberRef, DataRef: memberRef}}}, semantic.KnowledgeSpec{BaseObjectRef: &nextRef, MetricRefs: []semantic.KnowledgeReference{nextRef}, MemberBindings: []semantic.MemberBinding{{SemanticRef: memberRef, DataRef: nextMemberRef}}}},
		{"metric", semantic.KnowledgeSpec{Kind: "aggregate", InputRef: &memberRef, Aggregation: "sum"}, semantic.KnowledgeSpec{Kind: "aggregate", InputRef: &nextMemberRef, Aggregation: "sum"}},
		{"business_term", semantic.KnowledgeSpec{Capability: "predicate", SubjectRef: &ref}, semantic.KnowledgeSpec{Capability: "predicate", SubjectRef: &nextRef}},
		{"data_asset", semantic.KnowledgeSpec{DatasetRef: &source, Members: []semantic.KnowledgeMember{{ID: "amount", SourceFieldRef: &field}}}, semantic.KnowledgeSpec{DatasetRef: &nextSource, Members: []semantic.KnowledgeMember{{ID: "amount", SourceFieldRef: &nextField}}}},
		{"business_object", semantic.KnowledgeSpec{Members: []semantic.KnowledgeMember{{ID: "amount", Name: "Amount"}}}, semantic.KnowledgeSpec{Members: []semantic.KnowledgeMember{{ID: "amount", Name: "Corrected amount"}}}},
	} {
		t.Run(test.kind, func(t *testing.T) {
			target, baseline := baselineSpecUpdate(t, test.kind, test.before, test.after)
			if _, err := governance.InspectProductionContent(target.Kind, baseline); err != nil {
				t.Fatalf("invalid baseline fixture: %v", err)
			}
			if _, err := governance.InspectProductionContent(target.Kind, target.Content); err != nil {
				t.Fatalf("invalid desired fixture: %v", err)
			}
			items, unchanged, err := governance.ReplayProductionChanges(target, baseline)
			if err != nil {
				t.Fatalf("valid structured correction rejected: %v", err)
			}
			if unchanged || len(items) != 1 {
				t.Fatalf("correction not represented: unchanged=%v items=%d", unchanged, len(items))
			}
			for _, pair := range [][2]json.RawMessage{{items[0].BeforeValue, target.Changes[0].BeforeValue}, {items[0].AfterValue, target.Changes[0].AfterValue}} {
				want, err := governance.CanonicalJSON(pair[1])
				if err != nil || !bytes.Equal(pair[0], want) {
					t.Fatalf("versioned spec was rewritten: got=%s want=%s err=%v", pair[0], want, err)
				}
			}
			got, err := governance.ApplyChangeSet(baseline, items)
			want, canonicalErr := governance.CanonicalJSON(target.Content)
			if err != nil || canonicalErr != nil || !bytes.Equal(got, want) {
				t.Fatalf("replayed content differs: %v / %v", err, canonicalErr)
			}
		})
	}
}

func TestReplayProductionSpecCorrectionRejectsMismatchedBaselineAndContent(t *testing.T) {
	ref := baselineKnowledgeReference(t)
	before := semantic.KnowledgeSpec{MetricRefs: []semantic.KnowledgeReference{ref}}
	after := semantic.KnowledgeSpec{MetricRefs: []semantic.KnowledgeReference{ref}, Grain: "one order"}
	target, baseline := baselineSpecUpdate(t, "analysis_model", before, after)
	target.Changes[0].BeforeValue = baselineJSON(t, after)
	if _, _, err := governance.ReplayProductionChanges(target, baseline); !errors.Is(err, governance.ErrContentMismatch) {
		t.Fatalf("incorrect beforeValue accepted: %v", err)
	}
	target, baseline = baselineSpecUpdate(t, "analysis_model", before, after)
	target.Changes[0].AfterValue = baselineJSON(t, before)
	if _, _, err := governance.ReplayProductionChanges(target, baseline); !errors.Is(err, governance.ErrContentMismatch) {
		t.Fatalf("afterValue differing from target content accepted: %v", err)
	}
	target, baseline = baselineSpecUpdate(t, "analysis_model", before, after)
	target.Changes[0].FieldPath = "spec.metricRefs"
	if _, _, err := governance.ReplayProductionChanges(target, baseline); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("non-atomic spec path accepted: %v", err)
	}
}

func TestProductionSpecCorrectionKeepsContentValidation(t *testing.T) {
	for _, spec := range []json.RawMessage{
		json.RawMessage(`{"metricRefs":[{"localKey":"metric"}]}`),
		json.RawMessage(`{"metricRefs":[{"assetId":"invalid","revisionId":"invalid","releaseId":"invalid"}]}`),
		json.RawMessage(`{"unknownField":true}`),
		json.RawMessage(`[]`),
	} {
		target, _ := baselineSpecUpdate(t, "analysis_model", semantic.KnowledgeSpec{}, spec)
		if _, err := governance.InspectProductionContent(target.Kind, target.Content); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("invalid spec bypassed content validation: %s, %v", spec, err)
		}
	}
}

func TestReplayProductionChangesResolvesNonSpecLocalReferences(t *testing.T) {
	oldID, newID := baselineID(t, identity.NewAssetID), baselineID(t, identity.NewAssetID)
	baseline := baselineJSON(t, map[string]any{"asset": oldID})
	target := governance.TargetDeclaration{Kind: governance.TargetKindPhysicalBinding,
		Content: baselineJSON(t, map[string]any{"asset": map[string]string{"localKey": "next"}}),
		Changes: []governance.TargetChangeInput{{FieldPath: "asset", Op: string(governance.ChangeUpdate), BeforeValue: baselineJSON(t, oldID), AfterValue: json.RawMessage(`{"localKey":"next"}`)}},
	}
	items, unchanged, err := governance.ReplayProductionChanges(target, baseline, map[string]string{"next": newID})
	if err != nil || unchanged || len(items) != 1 || !bytes.Equal(items[0].AfterValue, baselineJSON(t, newID)) {
		t.Fatalf("non-spec local reference was not resolved: %+v, %v", items, err)
	}
	if _, _, err := governance.ReplayProductionChanges(target, baseline); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("unknown local reference accepted: %v", err)
	}
}

func baselineSpecUpdate(t *testing.T, kind string, before, after any) (governance.TargetDeclaration, json.RawMessage) {
	t.Helper()
	content := map[string]any{"address": "synthetic.correction", "assetType": kind, "displayName": "Synthetic correction", "definition": nil, "scope": nil, "ownerPrincipalId": baselineID(t, identity.NewPrincipalID), "spec": before}
	baseline := baselineJSON(t, content)
	content["spec"] = after
	return governance.TargetDeclaration{Kind: governance.TargetKindSemanticAsset, Content: baselineJSON(t, content), Changes: []governance.TargetChangeInput{{FieldPath: "spec", Op: string(governance.ChangeUpdate), BeforeValue: baselineJSON(t, before), AfterValue: baselineJSON(t, after)}}}, baseline
}

func baselineKnowledgeReference(t *testing.T) semantic.KnowledgeReference {
	t.Helper()
	return semantic.KnowledgeReference{AssetID: baselineID(t, identity.NewAssetID), RevisionID: baselineID(t, identity.NewRevisionID), ReleaseID: baselineID(t, identity.NewReleaseID)}
}

func baselineSourceReference(t *testing.T, kind string) semantic.SourceReference {
	t.Helper()
	ref := semantic.SourceReference{SnapshotID: baselineID(t, identity.NewSourceSnapshotID), Kind: kind}
	if kind == "dataset" {
		ref.ObjectID = baselineID(t, identity.NewPhysicalDatasetID)
		ref.RevisionID = baselineID(t, identity.NewPhysicalDatasetRevisionID)
	} else {
		ref.ObjectID = baselineID(t, identity.NewPhysicalFieldID)
		ref.RevisionID = baselineID(t, identity.NewPhysicalFieldRevisionID)
	}
	return ref
}

func baselineID[T interface{ String() string }](t *testing.T, create func() (T, error)) string {
	t.Helper()
	id, err := create()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func baselineJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
