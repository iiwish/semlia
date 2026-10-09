package governance_test

import (
	"errors"
	"strings"
	"testing"

	governance "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestProductionContentCarriesGovernedVocabulary(t *testing.T) {
	confusable, expected := baselineKnowledgeReference(t), baselineKnowledgeReference(t)
	content := map[string]any{"address": "synthetic.gmv", "assetType": "metric", "displayName": "有效支付金额", "definition": nil, "scope": nil, "ownerPrincipalId": baselineID(t, identity.NewPrincipalID),
		"aliases": []string{"GMV"}, "includes": []string{"已支付订单"}, "excludes": []string{"已退款订单"}, "examples": []string{"200 元订单计 200"},
		"confusableWith":  []any{map[string]any{"ref": confusable, "distinction": "下单金额包含未支付订单"}},
		"sampleQuestions": []any{map[string]any{"question": "8月 GMV 是多少？", "expects": map[string]any{"refs": []any{expected}}}}}
	refs, err := governance.InspectProductionContent(governance.TargetKindSemanticAsset, baselineJSON(t, content))
	if err != nil {
		t.Fatal(err)
	}
	// Vocabulary pointers are dependencies like spec references: pinned, authorized and present in the head.
	found := map[string]bool{}
	for _, ref := range refs.Published {
		found[ref.TargetID] = true
	}
	if !found[confusable.AssetID] || !found[expected.AssetID] {
		t.Fatalf("vocabulary references not reported: %+v", refs.Published)
	}
	content["aliases"] = []string{"有效支付金额"}
	if _, err := governance.InspectProductionContent(governance.TargetKindSemanticAsset, baselineJSON(t, content)); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("alias repeating the display name accepted: %v", err)
	}
	delete(content, "aliases")
	content["synonyms"] = []string{"GMV"}
	if _, err := governance.InspectProductionContent(governance.TargetKindSemanticAsset, baselineJSON(t, content)); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("unknown vocabulary key accepted: %v", err)
	}
}

func TestProductionVocabularyFieldsAreRevisable(t *testing.T) {
	for _, field := range strings.Fields(semantic.VocabularyFields) {
		before := map[string]any{"address": "synthetic.gmv", "assetType": "metric", "displayName": "有效支付金额", "definition": nil, "scope": nil, "ownerPrincipalId": baselineID(t, identity.NewPrincipalID)}
		after := map[string]any{}
		for key, value := range before {
			after[key] = value
		}
		value := any([]string{"GMV"})
		switch field {
		case "confusableWith":
			value = []any{map[string]any{"ref": baselineKnowledgeReference(t), "distinction": "不同口径"}}
		case "sampleQuestions":
			value = []any{map[string]any{"question": "GMV 是多少？"}}
		}
		after[field] = value
		target := governance.TargetDeclaration{Kind: governance.TargetKindSemanticAsset, Content: baselineJSON(t, after), Changes: []governance.TargetChangeInput{{FieldPath: field, Op: string(governance.ChangeAdd), AfterValue: baselineJSON(t, value)}}}
		items, _, err := governance.ReplayProductionChanges(target, baselineJSON(t, before))
		if err != nil {
			t.Fatalf("%s revision rejected: %v", field, err)
		}
		if field == "confusableWith" && !strings.Contains(string(items[0].AfterValue), `"releaseId"`) {
			t.Fatalf("confusable reference lost its release pin: %s", items[0].AfterValue)
		}
	}
}

func TestVocabularyConflictErrorNamesOnlyTerms(t *testing.T) {
	err := &governance.VocabularyConflictError{Conflicts: []semantic.VocabularyConflict{
		{Term: "gmv", AssetType: semantic.Metric, Assets: [2]semantic.VocabularyClaim{{AssetID: "a", DisplayName: "隐藏指标"}, {AssetID: "b", DisplayName: "有效支付金额", ByAlias: true}}},
		{Term: "gmv", AssetType: semantic.Metric},
		{Term: "销售额", AssetType: semantic.Metric},
	}}
	if !errors.Is(err, governance.ErrDependencyInvalid) {
		t.Fatal("conflict must stay a dependency failure for existing callers")
	}
	message := err.Message()
	if !strings.Contains(message, "「gmv」、「销售额」") || strings.Contains(message, "隐藏指标") || strings.Count(message, "gmv") != 1 {
		t.Fatalf("message = %q", message)
	}
}
