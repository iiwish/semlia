package semantic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/iiwish/semlia/pkg/identity"
)

func vocabularyRef(t *testing.T) KnowledgeReference {
	t.Helper()
	asset, err := identity.NewAssetID()
	if err != nil {
		t.Fatal(err)
	}
	revision, err := identity.NewRevisionID()
	if err != nil {
		t.Fatal(err)
	}
	release, err := identity.NewReleaseID()
	if err != nil {
		t.Fatal(err)
	}
	return KnowledgeReference{AssetID: asset.String(), RevisionID: revision.String(), ReleaseID: release.String()}
}

func vocabularyContent(t *testing.T, fields map[string]any) json.RawMessage {
	t.Helper()
	object := map[string]any{"displayName": "有效支付金额", "definition": "已支付且未退款", "spec": map[string]any{"kind": "aggregate"}}
	for key, value := range fields {
		object[key] = value
	}
	raw, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParseVocabularyAcceptsGovernedWording(t *testing.T) {
	ref := vocabularyRef(t)
	raw := vocabularyContent(t, map[string]any{
		"aliases":        []string{"GMV", "成交额"},
		"includes":       []string{"已支付订单"},
		"excludes":       []string{"已退款订单"},
		"examples":       []string{"一笔 200 元已支付订单计 200"},
		"confusableWith": []any{map[string]any{"ref": ref, "distinction": "下单金额包含未支付订单"}},
		"sampleQuestions": []any{
			map[string]any{"question": "8月 GMV 是多少？", "expects": map[string]any{"refs": []any{ref}}},
			map[string]any{"question": "华东上月成交额"},
		},
	})
	vocabulary, err := ParseVocabulary(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(vocabulary.Aliases) != 2 || len(vocabulary.SampleQuestions) != 2 || vocabulary.ConfusableWith[0].Distinction == "" {
		t.Fatalf("vocabulary not decoded: %+v", vocabulary)
	}
	if refs := vocabulary.References(); len(refs) != 2 || refs[0] != ref || refs[1] != ref {
		t.Fatalf("references = %+v", refs)
	}
	empty, err := ParseVocabulary(json.RawMessage(`{"displayName":"x","aliases":null}`))
	if err != nil || len(empty.Aliases) != 0 {
		t.Fatalf("absent vocabulary must stay valid: %v", err)
	}
}

func TestParseVocabularyRejectsAmbiguousOrUnboundedWording(t *testing.T) {
	ref := vocabularyRef(t)
	member := ref
	member.MemberID = "amount"
	long := strings.Repeat("长", 65)
	many := make([]string, 33)
	for i := range many {
		many[i] = "alias" + strings.Repeat("x", i)
	}
	for name, fields := range map[string]map[string]any{
		"blank alias":                 {"aliases": []string{" "}},
		"oversized alias":             {"aliases": []string{long}},
		"too many aliases":            {"aliases": many},
		"duplicate after folding":     {"aliases": []string{"GMV", "ｇｍｖ"}},
		"alias repeats display name":  {"aliases": []string{"有效支付金额 "}},
		"confusion without reason":    {"confusableWith": []any{map[string]any{"ref": ref, "distinction": " "}}},
		"confusion names a member":    {"confusableWith": []any{map[string]any{"ref": member, "distinction": "不同"}}},
		"duplicate confusion":         {"confusableWith": []any{map[string]any{"ref": ref, "distinction": "a"}, map[string]any{"ref": ref, "distinction": "b"}}},
		"duplicate sample question":   {"sampleQuestions": []any{map[string]any{"question": "GMV?"}, map[string]any{"question": "gmv?"}}},
		"empty expectation":           {"sampleQuestions": []any{map[string]any{"question": "GMV?", "expects": map[string]any{"refs": []any{}}}}},
		"unknown sample field":        {"sampleQuestions": []any{map[string]any{"question": "GMV?", "answer": "1"}}},
		"aliases of the wrong shape":  {"aliases": "GMV"},
		"invalid expectation pointer": {"sampleQuestions": []any{map[string]any{"question": "GMV?", "expects": map[string]any{"refs": []any{map[string]any{"assetId": "x", "revisionId": "y", "releaseId": "z"}}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseVocabulary(vocabularyContent(t, fields)); err == nil {
				t.Fatal("invalid vocabulary accepted")
			}
		})
	}
}

func TestMemberVocabularyIsScopedToOneAttribute(t *testing.T) {
	valid := `{"members":[{"id":"region","name":"区域","valueType":"string","aliases":["大区"],"valueAliases":[{"value":"华东","aliases":["江浙沪","华东区"]},{"value":"华南","aliases":["两广"]}]},{"id":"city","name":"城市","valueType":"string"}]}`
	spec, err := ParseKnowledgeSpec(BusinessObject, json.RawMessage(valid), false)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Members[0].ValueAliases[0].Aliases[0] != "江浙沪" || spec.Members[0].Aliases[0] != "大区" {
		t.Fatalf("member vocabulary not decoded: %+v", spec.Members[0])
	}
	for name, raw := range map[string]string{
		"alias names another member":     `{"members":[{"id":"region","name":"区域","aliases":["城市"]},{"id":"city","name":"城市"}]}`,
		"two members share an alias":     `{"members":[{"id":"region","name":"区域","aliases":["地区"]},{"id":"city","name":"城市","aliases":["地区"]}]}`,
		"alias repeats member name":      `{"members":[{"id":"region","name":"区域","aliases":["区域"]}]}`,
		"value alias maps to two values": `{"members":[{"id":"region","name":"区域","valueAliases":[{"value":"华东","aliases":["东部"]},{"value":"华南","aliases":["东部"]}]}]}`,
		"value alias shadows a value":    `{"members":[{"id":"region","name":"区域","valueAliases":[{"value":"华东","aliases":["华南"]},{"value":"华南","aliases":["两广"]}]}]}`,
		"duplicate canonical value":      `{"members":[{"id":"region","name":"区域","valueAliases":[{"value":"华东","aliases":["a"]},{"value":"华东","aliases":["b"]}]}]}`,
		"value without aliases":          `{"members":[{"id":"region","name":"区域","valueAliases":[{"value":"华东","aliases":[]}]}]}`,
		"value alias repeats value":      `{"members":[{"id":"region","name":"区域","valueAliases":[{"value":"华东","aliases":["华东"]}]}]}`,
		"value aliases on a number":      `{"members":[{"id":"amount","name":"金额","valueType":"number","valueAliases":[{"value":"1","aliases":["一"]}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseKnowledgeSpec(BusinessObject, json.RawMessage(raw), false); err == nil {
				t.Fatal("invalid member vocabulary accepted")
			}
		})
	}
	if _, err := ParseKnowledgeSpec(DataAsset, json.RawMessage(`{"members":[{"id":"region","name":"区域","valueAliases":[{"value":"华东","aliases":["江浙沪"]}]}]}`), false); err == nil {
		t.Fatal("value aliases belong to business attributes, not physical members")
	}
}

func TestVocabularyConflictsRequireDeclaredConfusion(t *testing.T) {
	gmv, orders, amount := vocabularyRef(t), vocabularyRef(t), vocabularyRef(t)
	entries := []VocabularyEntry{
		{AssetID: gmv.AssetID, AssetType: Metric, DisplayName: "有效支付金额", Vocabulary: Vocabulary{Aliases: []string{"GMV", "销售额"}}},
		{AssetID: orders.AssetID, AssetType: Metric, DisplayName: "下单金额", Vocabulary: Vocabulary{Aliases: []string{"销售额"}}},
		{AssetID: amount.AssetID, AssetType: Metric, DisplayName: "ＧＭＶ"},
		// Another type may use the same word: retrieval separates metrics from objects.
		{AssetID: vocabularyRef(t).AssetID, AssetType: BusinessObject, DisplayName: "订单", Vocabulary: Vocabulary{Aliases: []string{"销售额"}}},
		// Two display names colliding is left to naming rules.
		{AssetID: vocabularyRef(t).AssetID, AssetType: Metric, DisplayName: "下单金额"},
	}
	conflicts := VocabularyConflicts(entries)
	if len(conflicts) != 2 || conflicts[0].Term != "gmv" || conflicts[1].Term != "销售额" {
		t.Fatalf("conflicts = %+v", conflicts)
	}
	if conflicts[0].Assets[0].AssetID > conflicts[0].Assets[1].AssetID {
		t.Fatal("conflict assets must be ordered for stable reporting")
	}
	byAlias := map[string]bool{}
	for _, claim := range conflicts[0].Assets {
		byAlias[claim.AssetID] = claim.ByAlias
	}
	if !byAlias[gmv.AssetID] || byAlias[amount.AssetID] {
		t.Fatalf("claim kinds = %+v", conflicts[0].Assets)
	}
	// Declaring confusion on one side is enough; the other side may not be editable in the same change.
	entries[1].Vocabulary.ConfusableWith = []Confusion{{Ref: gmv, Distinction: "下单金额包含未支付订单"}}
	entries[2].Vocabulary.ConfusableWith = []Confusion{{Ref: gmv, Distinction: "历史口径"}}
	if conflicts := VocabularyConflicts(entries); len(conflicts) != 0 {
		t.Fatalf("declared confusion still conflicts: %+v", conflicts)
	}
}

func TestNormalizeTermFoldsWidthCaseAndSpace(t *testing.T) {
	if got := NormalizeTerm("  ＧＭＶ\t 总额 "); got != "gmv 总额" {
		t.Fatalf("NormalizeTerm = %q", got)
	}
}

func TestComputationalCoreIgnoresOnlyVocabulary(t *testing.T) {
	base := `{"assetType":"business_object","displayName":"客户","definition":"合成客户","spec":{"grain":"每位客户一行","members":[{"id":"region","name":"当前区域","valueType":"string"}]}}`
	core := func(raw string) string {
		t.Helper()
		digest, err := ComputationalCore(json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		return digest
	}
	original := core(base)
	worded := core(`{"assetType":"business_object","displayName":"客户","definition":"合成客户","aliases":["顾客"],"excludes":["潜在客户"],"sampleQuestions":[{"question":"有多少客户？"}],"spec":{"members":[{"valueType":"string","name":"当前区域","id":"region","aliases":["大区"],"valueAliases":[{"value":"华东","aliases":["江浙沪"]}]}],"grain":"每位客户一行"}}`)
	if worded != original {
		t.Fatal("vocabulary or key order changed the computational core")
	}
	for name, raw := range map[string]string{
		"definition":   strings.Replace(base, "合成客户", "全部客户", 1),
		"display name": strings.Replace(base, `"displayName":"客户"`, `"displayName":"顾客"`, 1),
		"member name":  strings.Replace(base, "当前区域", "区域", 1),
		"grain":        strings.Replace(base, "每位客户一行", "每个客户版本一行", 1),
	} {
		if core(raw) == original {
			t.Fatalf("%s change kept the computational core", name)
		}
	}
	if _, err := ComputationalCore(json.RawMessage(`[]`)); err == nil {
		t.Fatal("non-object content accepted")
	}
}
