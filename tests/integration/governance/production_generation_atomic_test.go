package governance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	gov "github.com/iiwish/semlia/internal/domain/governance"
)

func TestProductionGenerationConcurrentIdempotency(t *testing.T) {
	g := newProductionGenerationFixture(t)
	var wg sync.WaitGroup
	results := make(chan gov.ProductionGenerationResult, 4)
	failures := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := g.service.Generate(context.Background(), g.request)
			if err != nil {
				failures <- err
			} else {
				results <- result
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	var first string
	fresh := 0
	for result := range results {
		if first == "" {
			first = result.RunID.String()
		}
		if first != result.RunID.String() {
			t.Fatal("concurrent duplicate runs")
		}
		if !result.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatalf("new runs=%d", fresh)
	}
	assertTableCount(t, g.f.pool, "production_generation_requests", 1)
	assertTableCount(t, g.f.pool, "agent_runs", 1)
	g.run(t)
	if g.calls.Load() != 1 {
		t.Fatal("concurrent queue duplicated provider call")
	}
}

func TestProductionGenerationOutputCommitFaultRecoversWithoutRecall(t *testing.T) {
	g := newProductionGenerationFixture(t)
	queued := g.queue(t)
	ctx := context.Background()
	if _, err := g.f.pool.Exec(ctx, `CREATE FUNCTION generation_test_commit_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.status='succeeded' THEN RAISE EXCEPTION 'injected generation commit fault'; END IF; RETURN NULL; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := g.f.pool.Exec(ctx, `CREATE CONSTRAINT TRIGGER generation_test_commit_fault AFTER UPDATE ON production_generation_requests DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION generation_test_commit_fault()`); err != nil {
		t.Fatal(err)
	}
	g.run(t)
	if g.calls.Load() != 1 {
		t.Fatalf("first invocation calls=%d", g.calls.Load())
	}
	assertTableCount(t, g.f.pool, "production_generation_outputs", 0)
	assertTableCount(t, g.f.pool, "production_generation_links", 0)
	assertTableCount(t, g.f.pool, "agent_steps", 0)
	if result := g.get(t, queued.RunID); result.Status != "running" {
		t.Fatalf("failed commit fabricated final status: %+v", result)
	}
	if _, err := g.f.pool.Exec(ctx, `DROP FUNCTION generation_test_commit_fault() CASCADE`); err != nil {
		t.Fatal(err)
	}
	g.run(t)
	result := g.get(t, queued.RunID)
	if result.Status != "outcome_unknown" || g.calls.Load() != 1 {
		t.Fatalf("commit recovery duplicated model: %+v calls=%d", result, g.calls.Load())
	}
	assertTableCount(t, g.f.pool, "production_generation_outputs", 0)
}

func TestProductionGenerationQueueCommitFaultLeavesNoRun(t *testing.T) {
	g := newProductionGenerationFixture(t)
	ctx := context.Background()
	if _, err := g.f.pool.Exec(ctx, `CREATE FUNCTION generation_test_queue_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected queue commit fault'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := g.f.pool.Exec(ctx, `CREATE CONSTRAINT TRIGGER generation_test_queue_fault AFTER INSERT ON production_generation_requests DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION generation_test_queue_fault()`); err != nil {
		t.Fatal(err)
	}
	if _, err := g.service.Generate(ctx, g.request); err == nil {
		t.Fatal("queue commit failure hidden")
	}
	assertTableCount(t, g.f.pool, "production_generation_requests", 0)
	assertTableCount(t, g.f.pool, "agent_runs", 0)
	assertTableCount(t, g.f.pool, "jobs", 0)
	if _, err := g.f.pool.Exec(ctx, `DROP FUNCTION generation_test_queue_fault() CASCADE`); err != nil {
		t.Fatal(err)
	}
	g.queue(t)
	g.run(t)
	if g.calls.Load() != 1 {
		t.Fatal("retry after queue rollback did not call exactly once")
	}
}

func TestProductionGenerationApplicationCommitFaultIsAtomic(t *testing.T) {
	g := newProductionGenerationFixture(t)
	queued := g.queue(t)
	g.run(t)
	ctx := context.Background()
	if _, err := g.f.pool.Exec(ctx, `CREATE FUNCTION generation_test_apply_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected application commit fault'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := g.f.pool.Exec(ctx, `CREATE CONSTRAINT TRIGGER generation_test_apply_fault AFTER INSERT ON production_generation_applications DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION generation_test_apply_fault()`); err != nil {
		t.Fatal(err)
	}
	g.payload["expectedVersion"] = 1
	g.payload["suggestionRunId"] = queued.RunID.String()
	raw, _ := json.Marshal(g.payload)
	r := sendProdRequest(g.h, http.MethodPut, g.path(), g.actor.String(), "apply-commit-fault", string(raw))
	if r.Code < 500 {
		t.Fatalf("application fault hidden: %d %s", r.Code, r.Body.String())
	}
	assertTableCount(t, g.f.pool, "production_versions", 1)
	assertTableCount(t, g.f.pool, "proposals", 1)
	assertTableCount(t, g.f.pool, "production_generation_applications", 0)
	if _, err := g.f.pool.Exec(ctx, `DROP FUNCTION generation_test_apply_fault() CASCADE`); err != nil {
		t.Fatal(err)
	}
	r = sendProdRequest(g.h, http.MethodPut, g.path(), g.actor.String(), "apply-commit-fault", string(raw))
	if r.Code != 200 {
		t.Fatalf("application recovery: %d %s", r.Code, r.Body.String())
	}
	assertTableCount(t, g.f.pool, "production_generation_applications", 1)
	if g.calls.Load() != 1 {
		t.Fatal("application retry called model")
	}
}
