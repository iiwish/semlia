package governance

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	"github.com/iiwish/semlia/internal/application/governance/llm"
	distributiondomain "github.com/iiwish/semlia/internal/domain/distribution"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func menuAsset(name, kind string, members ...string) distributionapp.AskCatalogEntry {
	return distributionapp.AskCatalogEntry{Type: kind, Name: name, Members: members}
}

func TestAskClarificationNamesWhatCanBeAsked(t *testing.T) {
	text := askClarificationText([]distributionapp.AskCatalogEntry{
		menuAsset("有效支付金额", "metric"),
		menuAsset("客单价", "metric"),
		menuAsset("订单", "business_object", "支付金额", "支付时间"),
		menuAsset("客户", "business_object", "区域"),
		menuAsset("订单数据", "data_asset", "physical_only"),
		menuAsset("有效订单", "business_term"),
	})
	for _, want := range []string{"系统不会猜测", "有效支付金额", "客单价", "订单（支付金额、支付时间）", "客户（区域）", "时间范围"} {
		if !strings.Contains(text, want) {
			t.Fatalf("clarification lacks %q: %s", want, text)
		}
	}
	// Only metrics and business objects are offered: never physical data assets or business terms as askable metrics.
	for _, hidden := range []string{"physical_only", "订单数据", "有效订单"} {
		if strings.Contains(text, hidden) {
			t.Fatalf("clarification discloses %q: %s", hidden, text)
		}
	}
	if utf8.RuneCountInString(text) > 500 {
		t.Fatalf("clarification exceeds the API limit: %d runes", utf8.RuneCountInString(text))
	}
}

func TestAskClarificationSaysWhenNothingIsPublishedOrAuthorized(t *testing.T) {
	for name, catalog := range map[string][]distributionapp.AskCatalogEntry{
		"nil":   nil,
		"empty": {},
	} {
		text := askClarificationText(catalog)
		if !strings.Contains(text, "没有你有权使用的已发布知识") || strings.Contains(text, "系统不会猜测") {
			t.Fatalf("%s clarification = %q", name, text)
		}
	}
}

func TestAskClarificationStaysWithinTheAPILimitForLargeCatalogs(t *testing.T) {
	assets := []distributionapp.AskCatalogEntry{}
	for index := 0; index < 40; index++ {
		assets = append(assets, menuAsset("非常长的指标名称用于测试截断行为"+string(rune('A'+index%26)), "metric"))
		assets = append(assets, menuAsset("业务对象"+string(rune('A'+index%26)), "business_object", "属性一", "属性二", "属性三", "属性四", "属性五", "属性六", "属性七", "属性八"))
	}
	text := askClarificationText(assets)
	if utf8.RuneCountInString(text) > 500 || !strings.Contains(text, "等") || !strings.HasSuffix(text, "写明时间范围或比较对象。") {
		t.Fatalf("large catalog clarification (%d runes) = %s", utf8.RuneCountInString(text), text)
	}
}

func TestAskClarificationIsServerOwnedAndReplaysIdentically(t *testing.T) {
	repository := newAskTestRepository(t)
	assetID, revisionID := askID(t, identity.NewAssetID), askID(t, identity.NewRevisionID)
	repository.current = distributiondomain.ReleaseSnapshot{
		WorkspaceID: repository.workspace, ReleaseID: askID(t, identity.NewReleaseID), ManifestDigest: contentDigestOf("release"),
		Assets: []distributiondomain.ReleasedAsset{{AssetID: assetID, RevisionID: revisionID, Address: "commerce.net_revenue",
			AssetType: "metric", Name: "Net revenue", ContentDigest: contentDigestOf("definition"),
			Content: json.RawMessage(`{"name":"Net revenue","definition":"Revenue after confirmed refunds"}`)}},
	}
	clock := ClockFunc(time.Now)
	calls := 0
	service := NewAskService(repository, NewModelConfigService(repository, nil, clock), NewAgentRunService(repository, clock), distributionapp.NewService(repository, nil, distributionapp.ClockFunc(clock)), nil, WithAskProviderClientFactory(
		func(domain.ModelProvider, llm.CredentialResolver, *http.Client) (llm.ProviderClient, error) {
			calls++
			return askProviderClient{content: `{"schema":"semlia.ask-interpretation/v1","outcome":"clarification","clarification":"private echoed question"}`}, nil
		},
	))
	request := AskRequest{WorkspaceID: repository.workspace, Question: "private echoed question", IdempotencyKey: "named", PrincipalRef: repository.agent.ID.String()}
	first, err := service.Ask(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Ask(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	text := first.Interpretation.Clarification
	if !strings.Contains(text, "Net revenue") || strings.Contains(text, "private echoed question") {
		t.Fatalf("clarification must list published names and never echo model prose: %q", text)
	}
	if second.Interpretation.Clarification != text || calls != 1 {
		t.Fatalf("replay differs or called the provider again: %q vs %q (calls=%d)", second.Interpretation.Clarification, text, calls)
	}
}

func TestAskClarificationUsesDisplayNamesNotAddresses(t *testing.T) {
	repository := newAskTestRepository(t)
	repository.current = distributiondomain.ReleaseSnapshot{
		WorkspaceID: repository.workspace, ReleaseID: askID(t, identity.NewReleaseID), ManifestDigest: contentDigestOf("release"),
		Assets: []distributiondomain.ReleasedAsset{
			{AssetID: askID(t, identity.NewAssetID), RevisionID: askID(t, identity.NewRevisionID), Address: "commerce.net_revenue", AssetType: "metric", Name: "commerce.net_revenue",
				ContentDigest: contentDigestOf("metric"), Content: json.RawMessage(`{"displayName":"净收入","name":"commerce.net_revenue","definition":"Revenue after refunds"}`)},
			{AssetID: askID(t, identity.NewAssetID), RevisionID: askID(t, identity.NewRevisionID), Address: "commerce.orders", AssetType: "business_object", Name: "commerce.orders",
				ContentDigest: contentDigestOf("object"), Content: json.RawMessage(`{"displayName":"订单","definition":"Orders","spec":{"members":[{"id":"paid_at","name":"支付时间","valueType":"timestamp"},{"id":"region_code","valueType":"string"}]}}`)},
		},
	}
	clock := ClockFunc(time.Now)
	service := NewAskService(repository, NewModelConfigService(repository, nil, clock), NewAgentRunService(repository, clock), distributionapp.NewService(repository, nil, distributionapp.ClockFunc(clock)), nil, WithAskProviderClientFactory(
		func(domain.ModelProvider, llm.CredentialResolver, *http.Client) (llm.ProviderClient, error) {
			return askProviderClient{content: `{"schema":"semlia.ask-interpretation/v1","outcome":"clarification","clarification":"ignored"}`}, nil
		},
	))
	result, err := service.Ask(context.Background(), AskRequest{WorkspaceID: repository.workspace, Question: "refund amount?", IdempotencyKey: "display", PrincipalRef: repository.agent.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	text := result.Interpretation.Clarification
	for _, want := range []string{"指标 净收入", "订单（支付时间、region_code）"} {
		if !strings.Contains(text, want) {
			t.Fatalf("clarification lacks %q: %s", want, text)
		}
	}
	if strings.Contains(text, "commerce.") {
		t.Fatalf("clarification must prefer display names over addresses: %s", text)
	}
}
