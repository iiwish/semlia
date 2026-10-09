package governance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	app "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/governance/llm"
	s "github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

// publishVocabularyTerm publishes a definition-only term through the classic proposal path and returns its pin.
func publishVocabularyTerm(t *testing.T, env *fixture, w identity.WorkspaceID, author, reviewer, publisher identity.PrincipalID, key string, content map[string]any) (s.KnowledgeReference, int, string) {
	t.Helper()
	ctx := context.Background()
	content["assetType"], content["definition"], content["scope"], content["spec"] = "business_term", "Revenue after refunds", "Synthetic vocabulary fixture", map[string]any{"capability": "definition"}
	body, _ := json.Marshal(content)
	created, err := catalogapp.NewService(env.store, catalogapp.ClockFunc(time.Now)).CreateAsset(ctx, catalogapp.CreateAssetRequest{WorkspaceID: w, Address: "vocabulary." + key, AssetType: s.BusinessTerm, SchemaVersion: "1.0.0", Content: body, CreatedBy: author.String(), TraceID: traceID})
	if err != nil {
		t.Fatal(err)
	}
	_, _, proposal := proposeToInReview(t, env, w, author, created.ID, created.CurrentRevision.ID)
	approveProposal(t, env, w, reviewer, proposal)
	response := publishProposal(t, env, w, publisher, proposal)
	if response.Code != http.StatusCreated {
		return s.KnowledgeReference{}, response.Code, response.Body.String()
	}
	snapshot, err := env.store.CurrentReleaseSnapshot(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range snapshot.Assets {
		if asset.AssetID == created.ID {
			return s.KnowledgeReference{AssetID: asset.AssetID.String(), RevisionID: asset.RevisionID.String(), ReleaseID: snapshot.ReleaseID.String()}, response.Code, ""
		}
	}
	t.Fatal("missing published vocabulary fixture")
	return s.KnowledgeReference{}, 0, ""
}

func TestAskClarifiesSharedWordingWithoutCallingTheModel(t *testing.T) {
	env, w, actor := setupDurableAsk(t)
	ctx := context.Background()
	author := createPrincipalWithRoles(t, env, w, "author", []string{"asset_owner"})
	publisher := createPrincipalWithRoles(t, env, w, "publisher", []string{"publisher"})
	reviewer := createReviewerPrincipal(t, env, w, "reviewer")

	paid, code, body := publishVocabularyTerm(t, env, w, author, reviewer, publisher, "paid_amount", map[string]any{"displayName": "有效支付金额", "aliases": []string{"销售额"}})
	if code != http.StatusCreated {
		t.Fatalf("publish paid amount: %d %s", code, body)
	}
	// The publish transaction itself refuses a shared alias nobody declared.
	if _, code, body := publishVocabularyTerm(t, env, w, author, reviewer, publisher, "booked_undeclared", map[string]any{"displayName": "下单金额", "aliases": []string{"销售额"}}); code != http.StatusUnprocessableEntity || !strings.Contains(body, "VOCABULARY_CONFLICT") || !strings.Contains(body, "「销售额」") || strings.Contains(body, "有效支付金额") {
		t.Fatalf("undeclared shared alias: %d %s", code, body)
	}
	if _, code, body := publishVocabularyTerm(t, env, w, author, reviewer, publisher, "booked_amount", map[string]any{"displayName": "下单金额", "aliases": []string{"销售额"},
		"confusableWith": []any{map[string]any{"ref": paid, "distinction": "包含未支付订单"}}}); code != http.StatusCreated {
		t.Fatalf("declared shared alias: %d %s", code, body)
	}

	var calls atomic.Int32
	var prompt atomic.Value
	svc := durableAskService(env, askCallFunc(func(_ context.Context, r llm.CompleteRequest) (llm.CompleteResponse, error) {
		calls.Add(1)
		prompt.Store(r.Messages[0].Content)
		return llm.CompleteResponse{Content: `{"schema":"semlia.ask-interpretation/v1","outcome":"clarification","clarification":"model text"}`, FinishReason: "stop"}, nil
	}))
	request := app.AskRequest{WorkspaceID: w, PrincipalRef: actor.String(), Question: "上个月销售额是多少？", IdempotencyKey: "shared-wording", TraceID: traceID}
	result, err := svc.Ask(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Interpretation.Clarification
	if calls.Load() != 0 || result.Interpretation.Outcome != "clarification" || !strings.Contains(text, "「销售额」") || !strings.Contains(text, "有效支付金额") || !strings.Contains(text, "下单金额（包含未支付订单）") {
		t.Fatalf("calls=%d clarification=%q", calls.Load(), text)
	}
	replayed, err := svc.Ask(ctx, request)
	if err != nil || calls.Load() != 0 || replayed.Run.ID != result.Run.ID || replayed.Interpretation.Clarification != text {
		t.Fatalf("replay: err=%v calls=%d %q", err, calls.Load(), replayed.Interpretation.Clarification)
	}

	// Naming one of them settles the word; the model then sees the binding.
	request.Question, request.IdempotencyKey = "有效支付金额，也就是销售额，是什么意思？", "settled-wording"
	if _, err := svc.Ask(ctx, request); err != nil {
		t.Fatal(err)
	}
	sent, _ := prompt.Load().(string)
	if calls.Load() != 1 || !strings.Contains(sent, `"questionMatches"`) || !strings.Contains(sent, `"resolvedAssetIds":["`+paid.AssetID+`"]`) || !strings.Contains(sent, `"ambiguities":[]`) {
		t.Fatalf("calls=%d prompt lacks bindings: %s", calls.Load(), sent)
	}
}
