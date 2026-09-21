package governance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/discovery/catalog"
	"github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestProductionGenerationRechecksSourceAndDraft(t *testing.T) {
	for _, phase := range []string{"queued", "in flight"} {
		for _, change := range []string{"source advanced", "source permission revoked", "draft replaced"} {
			t.Run(phase+"/"+change, func(t *testing.T) {
				g := newProductionGenerationFixture(t)
				queued := g.queue(t)
				mutate := func() {
					ctx := context.Background()
					switch change {
					case "source advanced":
						source, err := identity.ParseSourceConnectionID(g.payload["input"].(map[string]any)["snapshots"].([]any)[0].(map[string]any)["sourceId"].(string))
						if err != nil {
							t.Error(err)
							return
						}
						snapshot, err := (catalog.Adapter{}).Discover(ctx, discovery.Input{Locator: "catalog://production", ObservedAt: time.Now().UTC(), Files: map[string][]byte{"catalog.json": []byte(`{"version":"1","datasets":[{"external_key":"orders","qualified_name":"orders","kind":"table","locator":"warehouse.orders","fields":[{"external_key":"order-id","name":"id","ordinal":1,"data_type":"text","nullable":false}]}]}`)}})
						if err != nil {
							t.Error(err)
							return
						}
						if _, err := g.f.store.PersistDiscoverySnapshot(ctx, g.w, source, snapshot); err != nil {
							t.Error(err)
						}
					case "source permission revoked":
						// The agent holds source access through both the legacy
						// source_operator grant and the authoring_agent system
						// binding; revoke both so the recheck truly loses scope.
						if _, err := g.f.pool.Exec(ctx, `UPDATE role_bindings SET revoked_at=clock_timestamp(),revoked_by=$2,revocation_reason='Generation source scope revoked',version=version+1 WHERE principal_id=$1 AND role_id IN ('source_operator','authoring_agent')`, g.agent.UUID(), g.actor.UUID()); err != nil {
							t.Error(err)
						}
					case "draft replaced":
						g.payload["expectedVersion"] = 1
						raw, _ := json.Marshal(g.payload)
						r := sendProdRequest(g.h, http.MethodPut, g.path(), g.actor.String(), "replace-during-generation", string(raw))
						if r.Code != 200 {
							t.Errorf("replace: %d %s", r.Code, r.Body.String())
						}
					}
				}
				if phase == "queued" {
					mutate()
				} else {
					g.beforeResponse = mutate
				}
				g.run(t)
				result := g.get(t, queued.RunID)
				if result.Status != "failed" || result.ErrorCode == nil {
					t.Fatalf("stale generation: %+v", result)
				}
				want := "INPUT_STALE"
				if change == "source permission revoked" {
					want = "FORBIDDEN"
				}
				if change == "draft replaced" {
					want = "VERSION_CONFLICT"
				}
				if *result.ErrorCode != want {
					t.Fatalf("failure=%s want=%s", *result.ErrorCode, want)
				}
				calls := int32(0)
				if phase == "in flight" {
					calls = 1
				}
				if g.calls.Load() != calls {
					t.Fatal("unexpected provider calls")
				}
				assertTableCount(t, g.f.pool, "production_generation_outputs", 0)
			})
		}
	}
}
