package distribution_test

import (
	"encoding/json"
	"reflect"
	"testing"

	d "github.com/iiwish/semlia/internal/domain/distribution"
	s "github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/internal/testsupport/knowledgecase"
	"github.com/iiwish/semlia/pkg/identity"
)

// retrievalCase decorates the synthetic commerce release with governed Chinese wording and adds a confusable
// "下单金额" metric that shares the alias "销售额" with the paid amount.
func retrievalCase(t *testing.T) (knowledgecase.Case, []d.AskCard) {
	t.Helper()
	c := knowledgecase.New()
	booked, _ := identity.NewAssetID()
	bookedRevision, _ := identity.NewRevisionID()
	bookedRef := s.KnowledgeReference{AssetID: booked.String(), RevisionID: bookedRevision.String(), ReleaseID: c.Snapshot.ReleaseID.String()}
	wording := map[string]map[string]any{
		"orders":    {"displayName": "订单"},
		"customers": {"displayName": "客户"},
		"paid":      {"displayName": "已支付订单"},
		"old":       {"displayName": "老客户", "aliases": []string{"老客"}},
		"amount": {"displayName": "有效支付金额", "aliases": []string{"GMV", "销售额"}, "excludes": []string{"已退款订单"},
			"confusableWith": []any{map[string]any{"ref": bookedRef, "distinction": "只计已支付订单"}}},
		"count":   {"displayName": "有效订单数"},
		"average": {"displayName": "客单价"},
		"model":   {"displayName": "订单分析模型"},
	}
	for i := range c.Snapshot.Assets {
		a := &c.Snapshot.Assets[i]
		var content map[string]any
		if err := json.Unmarshal(a.Content, &content); err != nil {
			t.Fatal(err)
		}
		for key, value := range wording[a.Name] {
			content[key] = value
		}
		if a.Name == "customers" {
			spec := content["spec"].(map[string]any)
			for _, raw := range spec["members"].([]any) {
				member := raw.(map[string]any)
				if member["id"] == "region" {
					member["name"] = "当前区域"
					member["aliases"] = []string{"大区"}
					member["valueAliases"] = []any{map[string]any{"value": "East", "aliases": []string{"江浙沪", "华东"}}}
				}
			}
		}
		a.Content, _ = json.Marshal(content)
	}
	bookedContent, _ := json.Marshal(map[string]any{"assetType": "metric", "displayName": "下单金额", "definition": "全部下单金额", "aliases": []string{"销售额"},
		"confusableWith": []any{map[string]any{"ref": c.Refs["amount"], "distinction": "包含未支付和已退款订单"}},
		"spec":           s.KnowledgeSpec{Kind: "aggregate", Aggregation: "sum", Unit: "CNY", NullPolicy: "required", InputRef: &s.KnowledgeReference{AssetID: c.Refs["orders"].AssetID, RevisionID: c.Refs["orders"].RevisionID, ReleaseID: c.Refs["orders"].ReleaseID, MemberID: "amount"}}})
	c.Snapshot.Assets = append(c.Snapshot.Assets, d.ReleasedAsset{AssetID: booked, RevisionID: bookedRevision, Address: "commerce.booked", Name: "booked", AssetType: s.Metric, Content: bookedContent})
	c.Refs["booked"] = bookedRef
	return c, d.CompileAskCards(c.Snapshot.Assets)
}

var wideBudget = d.AskBudget{MaxCards: 128, MaxBytes: 64 * 1024}

func hitFor(t *testing.T, selection d.AskSelection, text string) d.AskMatch {
	t.Helper()
	for _, match := range selection.Matches {
		if match.Text == text {
			return match
		}
	}
	t.Fatalf("no match for %q in %+v", text, selection.Matches)
	return d.AskMatch{}
}

func TestAskCardsCarryGovernedWording(t *testing.T) {
	c, cards := retrievalCase(t)
	byID := map[string]d.AskCard{}
	for _, card := range cards {
		byID[card.AssetID] = card
	}
	amount := byID[c.Refs["amount"].AssetID]
	if amount.Name != "有效支付金额" || amount.Unit != "CNY" || !reflect.DeepEqual(amount.Aliases, []string{"GMV", "销售额"}) || amount.Excludes[0] != "已退款订单" {
		t.Fatalf("amount card = %+v", amount)
	}
	if len(amount.ConfusableWith) != 1 || amount.ConfusableWith[0].Name != "下单金额" {
		t.Fatalf("confusable neighbour not named: %+v", amount.ConfusableWith)
	}
	// A neighbour the caller cannot read is dropped rather than leaked.
	partial := d.CompileAskCards(c.Snapshot.Assets[:len(c.Snapshot.Assets)-1])
	for _, card := range partial {
		if card.AssetID == c.Refs["amount"].AssetID && len(card.ConfusableWith) != 0 {
			t.Fatalf("unreadable confusable neighbour disclosed: %+v", card.ConfusableWith)
		}
	}
}

func TestSelectAskKnowledgeBindsWordingAndModel(t *testing.T) {
	c, cards := retrievalCase(t)
	selection := d.SelectAskKnowledge("2026年8月江浙沪老客的 GMV 和客单价是多少？", cards, wideBudget)
	if len(selection.Ambiguities) != 0 {
		t.Fatalf("unexpected ambiguity: %+v", selection.Ambiguities)
	}
	value := hitFor(t, selection, "江浙沪")
	if len(value.Hits) != 1 || value.Hits[0].MemberID != "region" || value.Hits[0].Value != "East" || value.Hits[0].Via != "value_alias" {
		t.Fatalf("value alias not resolved: %+v", value)
	}
	if hit := hitFor(t, selection, "gmv").Hits[0]; hit.AssetID != c.Refs["amount"].AssetID || hit.Via != "alias" {
		t.Fatalf("metric alias not resolved: %+v", hit)
	}
	// "老客户" would also contain "客户"; the longer governed phrase wins and "客" is not double counted.
	if hit := hitFor(t, selection, "老客").Hits[0]; hit.AssetID != c.Refs["old"].AssetID {
		t.Fatalf("term alias not resolved: %+v", hit)
	}
	if len(selection.CoveringModels) != 1 || selection.CoveringModels[0].AssetID != c.Refs["model"].AssetID {
		t.Fatalf("covering models = %+v", selection.CoveringModels)
	}
	// Retrieval ranks named assets first and closes over what they depend on and the models that use them.
	position := map[string]int{}
	for i, id := range selection.Recalled {
		position[id] = i
	}
	for _, name := range []string{"amount", "average", "old", "customers", "model", "count", "orders", "paid"} {
		if _, ok := position[c.Refs[name].AssetID]; !ok {
			t.Fatalf("%s not recalled: %v", name, selection.Recalled)
		}
	}
	if position[c.Refs["amount"].AssetID] > position[c.Refs["model"].AssetID] || position[c.Refs["model"].AssetID] > position[c.Refs["order_data"].AssetID] {
		t.Fatalf("recall order does not rank named knowledge first: %v", selection.Recalled)
	}
	if selection.OmittedAssets != 0 || len(selection.Cards) != len(cards) {
		t.Fatalf("a catalog within budget must be sent whole: omitted=%d cards=%d", selection.OmittedAssets, len(selection.Cards))
	}
	again := d.SelectAskKnowledge("2026年8月江浙沪老客的 GMV 和客单价是多少？", cards, wideBudget)
	left, _ := json.Marshal(selection)
	right, _ := json.Marshal(again)
	if string(left) != string(right) {
		t.Fatal("retrieval is not deterministic")
	}
}

func TestSelectAskKnowledgeReportsUnsettledAmbiguity(t *testing.T) {
	c, cards := retrievalCase(t)
	selection := d.SelectAskKnowledge("上个月销售额是多少", cards, wideBudget)
	if len(selection.Ambiguities) != 1 || selection.Ambiguities[0].Text != "销售额" || len(selection.Ambiguities[0].Candidates) != 2 {
		t.Fatalf("ambiguities = %+v", selection.Ambiguities)
	}
	names := []string{}
	for _, candidate := range selection.Ambiguities[0].Candidates {
		names = append(names, candidate.Name)
	}
	if len(names) != 2 || !reflect.DeepEqual(selection.Ambiguities[0].Distinctions, []string{"只计已支付订单", "包含未支付和已退款订单"}) && !reflect.DeepEqual(selection.Ambiguities[0].Distinctions, []string{"包含未支付和已退款订单", "只计已支付订单"}) {
		t.Fatalf("candidates=%v distinctions=%v", names, selection.Ambiguities[0].Distinctions)
	}
	if len(selection.CoveringModels) != 0 {
		t.Fatalf("an ambiguous metric must not pick a model: %+v", selection.CoveringModels)
	}

	// Naming one candidate elsewhere in the question settles the shared word.
	settled := d.SelectAskKnowledge("有效支付金额，也就是销售额，上个月是多少", cards, wideBudget)
	if len(settled.Ambiguities) != 0 {
		t.Fatalf("settled ambiguity still reported: %+v", settled.Ambiguities)
	}
	if match := hitFor(t, settled, "销售额"); !reflect.DeepEqual(match.ResolvedAssetIDs, []string{c.Refs["amount"].AssetID}) {
		t.Fatalf("resolution = %+v", match.ResolvedAssetIDs)
	}
	if len(settled.CoveringModels) != 1 {
		t.Fatalf("settled metric should pick its model: %+v", settled.CoveringModels)
	}
	// A display name beats an alias for the same phrase.
	byName := d.SelectAskKnowledge("下单金额是多少", cards, wideBudget)
	if len(byName.Ambiguities) != 0 || hitFor(t, byName, "下单金额").Hits[0].AssetID != c.Refs["booked"].AssetID {
		t.Fatalf("display name did not win: %+v", byName)
	}
}

func TestSelectAskKnowledgeBudgetKeepsRecalledKnowledgeFirst(t *testing.T) {
	c, cards := retrievalCase(t)
	selection := d.SelectAskKnowledge("客单价", cards, d.AskBudget{MaxCards: 3, MaxBytes: 64 * 1024})
	if len(selection.Cards) != 3 || selection.OmittedAssets != len(cards)-3 {
		t.Fatalf("budget not applied: cards=%d omitted=%d", len(selection.Cards), selection.OmittedAssets)
	}
	if selection.Cards[0].AssetID != c.Refs["average"].AssetID {
		t.Fatalf("named metric not first: %s", selection.Cards[0].Name)
	}
	// Partial wording still recalls the asset through character overlap.
	fuzzy := d.SelectAskKnowledge("支付金额趋势", cards, d.AskBudget{MaxCards: 1, MaxBytes: 64 * 1024})
	if len(fuzzy.Matches) != 0 || len(fuzzy.Cards) != 1 || fuzzy.Cards[0].AssetID != c.Refs["amount"].AssetID {
		t.Fatalf("fuzzy recall = %+v", fuzzy.Cards)
	}
}

func TestSelectAskKnowledgeRespectsLatinWordBoundaries(t *testing.T) {
	_, cards := retrievalCase(t)
	if selection := d.SelectAskKnowledge("show gmvx for August", cards, wideBudget); len(selection.Matches) != 0 {
		t.Fatalf("alias matched inside a longer word: %+v", selection.Matches)
	}
	if selection := d.SelectAskKnowledge("show GMV for August", cards, wideBudget); len(selection.Matches) != 1 {
		t.Fatalf("bounded alias not matched: %+v", selection.Matches)
	}
}

func TestCheckSampleQuestionsReplaysRetrieval(t *testing.T) {
	c, _ := retrievalCase(t)
	for i := range c.Snapshot.Assets {
		a := &c.Snapshot.Assets[i]
		if a.Name != "amount" {
			continue
		}
		var content map[string]any
		_ = json.Unmarshal(a.Content, &content)
		content["sampleQuestions"] = []any{
			map[string]any{"question": "8月 GMV 是多少？", "expects": map[string]any{"refs": []any{c.Refs["model"]}}},
			map[string]any{"question": "上个月销售额是多少？"},
			map[string]any{"question": "库存周转天数是多少？"},
		}
		a.Content, _ = json.Marshal(content)
	}
	cards := d.CompileAskCards(c.Snapshot.Assets)
	failures := d.CheckSampleQuestions(cards, map[string]bool{c.Refs["amount"].AssetID: true}, wideBudget)
	if len(failures) != 2 {
		t.Fatalf("failures = %+v", failures)
	}
	if failures[0].Question != "上个月销售额是多少？" || !failures[0].Ambiguous {
		t.Fatalf("ambiguous sample not reported: %+v", failures[0])
	}
	if failures[1].Question != "库存周转天数是多少？" || len(failures[1].Missing) != 1 || failures[1].Missing[0] != c.Refs["amount"].AssetID {
		t.Fatalf("unrecalled sample not reported: %+v", failures[1])
	}
	if others := d.CheckSampleQuestions(cards, map[string]bool{c.Refs["count"].AssetID: true}, wideBudget); len(others) != 0 {
		t.Fatalf("only owners are checked: %+v", others)
	}
}
