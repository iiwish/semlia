package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	app "github.com/iiwish/semlia/internal/application/governance"
	gov "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestProductionGenerationQueueGates(t *testing.T) {
	for _, name := range []string{"no grant", "model revision", "input digest", "budget", "version", "agent revoked", "caller revoked"} {
		t.Run(name, func(t *testing.T) {
			g := newProductionGenerationFixture(t)
			service := g.service
			switch name {
			case "no grant":
				service = app.NewProductionGenerationService(g.f.store, nil)
			case "model revision":
				g.request.ModelConfigRevision = "sha256:" + strings.Repeat("b", 64)
			case "input digest":
				g.request.InputDigest = "sha256:" + strings.Repeat("b", 64)
			case "budget":
				g.request.MaxCostMicros = 1
			case "version":
				g.request.ExpectedVersion = 2
			case "agent revoked", "caller revoked":
				actor := g.agent
				if name == "caller revoked" {
					actor = g.actor
				}
				if _, err := g.f.pool.Exec(context.Background(), `UPDATE principals SET status='suspended' WHERE id=$1`, actor.UUID()); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := service.Generate(context.Background(), g.request); err == nil {
				t.Fatal("unauthorized generation queued")
			}
			if g.calls.Load() != 0 {
				t.Fatal("rejected queue called provider")
			}
			assertTableCount(t, g.f.pool, "production_generation_requests", 0)
			assertTableCount(t, g.f.pool, "agent_runs", 0)
		})
	}
}

func TestProductionGenerationReauthorizesBeforeAndAfterCall(t *testing.T) {
	for _, phase := range []string{"queued", "in flight"} {
		for _, subject := range []string{"agent", "caller", "model"} {
			t.Run(phase+"/"+subject, func(t *testing.T) {
				g := newProductionGenerationFixture(t)
				queued := g.queue(t)
				revoke := func() {
					var err error
					if subject == "model" {
						_, err = g.f.pool.Exec(context.Background(), `UPDATE model_settings SET enabled=false WHERE id=$1`, g.request.ModelSettingID.UUID())
					} else {
						actor := g.agent
						if subject == "caller" {
							actor = g.actor
						}
						_, err = g.f.pool.Exec(context.Background(), `UPDATE principals SET status='suspended' WHERE id=$1`, actor.UUID())
					}
					if err != nil {
						t.Error(err)
					}
				}
				if phase == "queued" {
					revoke()
				} else {
					g.beforeResponse = revoke
				}
				g.run(t)
				var status string
				if err := g.f.pool.QueryRow(context.Background(), `SELECT status FROM production_generation_requests WHERE agent_run_id=$1`, queued.RunID.UUID()).Scan(&status); err != nil {
					t.Fatal(err)
				}
				if status != "failed" {
					t.Fatalf("revoked run status=%s", status)
				}
				want := int32(0)
				if phase == "in flight" {
					want = 1
				}
				if g.calls.Load() != want {
					t.Fatalf("provider calls=%d want=%d", g.calls.Load(), want)
				}
				assertTableCount(t, g.f.pool, "production_generation_outputs", 0)
				assertTableCount(t, g.f.pool, "production_generation_links", 0)
			})
		}
	}
}

func TestProductionGenerationHTTPRejectsAuthorityInjection(t *testing.T) {
	g := newProductionGenerationFixture(t)
	raw, _ := json.Marshal(g.request)
	for _, key := range []string{"principalId", "workspaceId", "operationId", "providerMode", "grant", "approved", "costMicros"} {
		t.Run(key, func(t *testing.T) {
			body := strings.TrimSuffix(string(raw), "}") + `,"` + key + `":"forged"}`
			r := sendProdRequest(g.h, http.MethodPost, g.path()+"/generation", g.actor.String(), g.request.IdempotencyKey, body)
			if r.Code != 400 {
				t.Fatalf("authority injection %d %s", r.Code, r.Body.String())
			}
		})
	}
	assertTableCount(t, g.f.pool, "production_generation_requests", 0)
}

func TestProductionGenerationReclaimedInvocationNeverCallsAgain(t *testing.T) {
	g := newProductionGenerationFixture(t)
	queued := g.queue(t)
	job, err := g.f.store.ClaimJob(context.Background(), "lost-worker", time.Now(), time.Minute)
	if err != nil || job == nil {
		t.Fatalf("claim: %v", err)
	}
	work, err := g.f.store.ClaimProductionGeneration(context.Background(), *job, []gov.ProductionGenerationGrant{g.grant}, "protocol_stub")
	if err != nil || work == nil {
		t.Fatalf("invocation claim: %v", err)
	}
	if _, err := g.f.pool.Exec(context.Background(), `UPDATE jobs SET leased_until=clock_timestamp()-interval '1 second' WHERE id=$1`, job.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	g.run(t)
	result := g.get(t, queued.RunID)
	if result.Status != "outcome_unknown" || result.ErrorCode == nil || *result.ErrorCode != "GENERATION_OUTCOME_UNKNOWN" || g.calls.Load() != 0 {
		t.Fatalf("recovery re-invoked provider: %+v", result)
	}
	if err := g.f.store.FinishProductionGeneration(context.Background(), *job, work, json.RawMessage(g.output), "", false); err != nil {
		t.Fatal(err)
	}
	if result = g.get(t, queued.RunID); result.Status != "outcome_unknown" {
		t.Fatal("late output overwrote unknown result")
	}
	assertTableCount(t, g.f.pool, "production_generation_outputs", 0)
}

func TestProductionGenerationIdempotencyConflict(t *testing.T) {
	g := newProductionGenerationFixture(t)
	g.queue(t)
	g.request.Instruction += " Changed"
	if _, err := g.service.Generate(context.Background(), g.request); !errors.Is(err, gov.ErrIdempotencyConflict) {
		t.Fatalf("key reused: %v", err)
	}
	assertTableCount(t, g.f.pool, "production_generation_requests", 1)
	assertTableCount(t, g.f.pool, "agent_runs", 1)
}

func TestProductionGenerationRejectsInventedTargetIdentity(t *testing.T) {
	g := newProductionGenerationFixture(t)
	g.output = strings.Replace(g.output, `"identityKey":"sales.orders"`, `"identityKey":"sales.forged"`, 1)
	queued := g.queue(t)
	g.run(t)
	result := g.get(t, queued.RunID)
	if result.Status != "failed" || result.ErrorCode == nil || *result.ErrorCode != "AI_OUTPUT_INVALID" {
		t.Fatalf("forged identity: %+v", result)
	}
	assertTableCount(t, g.f.pool, "production_generation_outputs", 0)
}

func TestProductionGenerationApplicationRejectsWrongRun(t *testing.T) {
	g := newProductionGenerationFixture(t)
	g.payload["expectedVersion"] = 1
	g.payload["suggestionRunId"] = mustID(t, identity.NewAgentRunID).String()
	raw, _ := json.Marshal(g.payload)
	r := sendProdRequest(g.h, http.MethodPut, g.path(), g.actor.String(), "apply-wrong-run-001", string(raw))
	if r.Code != 404 {
		t.Fatalf("unknown run application: %d %s", r.Code, r.Body.String())
	}
	assertTableCount(t, g.f.pool, "production_versions", 1)
	assertTableCount(t, g.f.pool, "production_generation_applications", 0)
}
