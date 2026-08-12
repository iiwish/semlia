package worker_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const postgresImage = "postgres:18-alpine"

var (
	databaseURL string
	repoRoot    = repositoryRoot()
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(
		ctx,
		postgresImage,
		tcpostgres.WithDatabase("semlia_worker_test"),
		tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start isolated PostgreSQL container: failed")
		os.Exit(1)
	}
	databaseURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "resolve isolated PostgreSQL connection: failed")
		os.Exit(1)
	}
	migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repoRoot, "migrations"))
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "create migrator")
		os.Exit(1)
	}
	if err := migrator.Up(); err != nil {
		_ = migrator.Close()
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "migrate isolated PostgreSQL")
		os.Exit(1)
	}
	_ = migrator.Close()

	code := m.Run()
	if err := testcontainers.TerminateContainer(container); err != nil && code == 0 {
		fmt.Fprintf(os.Stderr, "terminate isolated PostgreSQL container: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

func TestConcurrentClaimCreatesOneLease(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	createWorkspace(t, pool, "workspace_claim")
	now := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	job, err := store.EnqueueJob(context.Background(), jobs.EnqueueJobParams{
		ID: "job_claim", WorkspaceID: "workspace_claim", Type: "discover.source",
		Payload: []byte(`{"source":"fixture"}`), MaxAttempts: 3,
		AvailableAt: now, IdempotencyKey: "claim-once", TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan *jobs.Job, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, owner := range []string{"worker-a", "worker-b"} {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			<-start
			claimed, claimErr := store.ClaimJob(context.Background(), owner, now, time.Minute)
			results <- claimed
			errs <- claimErr
		}(owner)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var claimed []*jobs.Job
	for result := range results {
		if result != nil {
			claimed = append(claimed, result)
		}
	}
	if len(claimed) != 1 || claimed[0].ID != job.ID || claimed[0].Attempt != 1 {
		t.Fatalf("claims = %+v", claimed)
	}
	t.Logf("concurrent claim count=%d owner=%s attempt=%d", len(claimed), claimed[0].LeaseOwner, claimed[0].Attempt)
	if err := store.MarkJobSucceeded(context.Background(), job.ID, "not-the-owner", now); !errors.Is(err, pgstore.ErrLeaseLost) {
		t.Fatalf("wrong owner completion error = %v", err)
	}
	if err := store.MarkJobSucceeded(context.Background(), job.ID, claimed[0].LeaseOwner, now); err != nil {
		t.Fatalf("current owner completion: %v", err)
	}
}

func TestConcurrentEnqueueIsIdempotentPerWorkspace(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	createWorkspace(t, pool, "workspace_idempotency")
	now := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)

	start := make(chan struct{})
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			job, err := store.EnqueueJob(context.Background(), jobs.EnqueueJobParams{
				ID: fmt.Sprintf("job_idempotent_%d", i), WorkspaceID: "workspace_idempotency",
				Type: "discover.source", Payload: []byte(`{}`), MaxAttempts: 3,
				AvailableAt: now, IdempotencyKey: "same-request", TraceID: traceID,
			})
			if err == nil {
				ids <- job.ID
			}
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	unique := map[string]struct{}{}
	for id := range ids {
		unique[id] = struct{}{}
	}
	if len(unique) != 1 {
		t.Fatalf("returned job IDs = %v", unique)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM jobs
		WHERE workspace_id = 'workspace_idempotency' AND idempotency_key = 'same-request'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("idempotent rows = %d", count)
	}
	t.Logf("concurrent idempotency returned_ids=%d persisted_rows=%d", len(unique), count)
}

func TestWorkerRetriesThenDeadLettersWithStableErrorCode(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	createWorkspace(t, pool, "workspace_retry")
	now := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	worker := jobs.NewWorker(store, clock, jobs.BackoffFunc(func(int32) time.Duration {
		return 5 * time.Minute
	}), time.Minute)
	worker.Register("discover.source", func(context.Context, jobs.Job) error {
		return errors.New("postgres://user:database-secret@private.invalid/semlia")
	})
	job, err := store.EnqueueJob(context.Background(), jobs.EnqueueJobParams{
		ID: "job_retry", WorkspaceID: "workspace_retry", Type: "discover.source",
		Payload: []byte(`{}`), MaxAttempts: 2, AvailableAt: now,
		IdempotencyKey: "retry", TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}

	processed, err := worker.RunOne(context.Background(), "worker-retry")
	if err != nil || !processed {
		t.Fatalf("first run processed=%t err=%v", processed, err)
	}
	assertJobState(t, pool, job.ID, "retryable", 1, "HANDLER_FAILED", now.Add(5*time.Minute))

	clock.now = now.Add(5 * time.Minute)
	processed, err = worker.RunOne(context.Background(), "worker-retry")
	if err != nil || !processed {
		t.Fatalf("second run processed=%t err=%v", processed, err)
	}
	assertJobState(t, pool, job.ID, "dead_letter", 2, "HANDLER_FAILED", time.Time{})

	var leaked bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM jobs
			WHERE id = $1 AND row_to_json(jobs)::text LIKE '%database-secret%'
		) FROM jobs WHERE id = $1`, job.ID).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked {
		t.Fatal("raw handler error leaked into job persistence")
	}
}

func TestExpiredLeaseIsRequeuedOrDeadLettered(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	createWorkspace(t, pool, "workspace_expiry")
	now := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	for _, input := range []struct {
		id          string
		maxAttempts int32
		wantStatus  string
	}{
		{"job_expired_retry", 2, "retryable"},
		{"job_expired_dead", 1, "dead_letter"},
	} {
		if _, err := store.EnqueueJob(context.Background(), jobs.EnqueueJobParams{
			ID: input.id, WorkspaceID: "workspace_expiry", Type: "expiry.test", Payload: []byte(`{}`),
			MaxAttempts: input.maxAttempts, AvailableAt: now, IdempotencyKey: input.id, TraceID: traceID,
		}); err != nil {
			t.Fatal(err)
		}
		claimed, err := store.ClaimJob(context.Background(), input.id+"-owner", now, time.Second)
		if err != nil || claimed == nil {
			t.Fatalf("claim %s: %+v %v", input.id, claimed, err)
		}
	}
	if err := store.ReapExpiredJobs(context.Background(), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	assertJobStatus(t, pool, "job_expired_retry", "retryable")
	assertJobStatus(t, pool, "job_expired_dead", "dead_letter")
}

func TestAuditAndOutboxCommitAtomically(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	createWorkspace(t, pool, "workspace_tx")
	now := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)

	err := store.WithTx(context.Background(), func(tx *pgstore.TxStore) error {
		if err := tx.CreateAuditEvent(context.Background(), jobs.AuditEvent{
			ID: "audit_rollback", WorkspaceID: "workspace_tx", Type: "source.created",
			ActorID: "founder", Payload: []byte(`{"source":"fixture"}`), TraceID: traceID, CreatedAt: now,
		}); err != nil {
			return err
		}
		if err := tx.EnqueueOutbox(context.Background(), jobs.OutboxEvent{
			ID: "outbox_rollback", WorkspaceID: "workspace_tx", Type: "source.created",
			Payload: []byte(`{"source":"fixture"}`), MaxAttempts: 3,
			AvailableAt: now, TraceID: traceID,
		}); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected transaction rollback")
	}
	assertCounts(t, pool, 0, 0)

	if err := store.WithTx(context.Background(), func(tx *pgstore.TxStore) error {
		if err := tx.CreateAuditEvent(context.Background(), jobs.AuditEvent{
			ID: "audit_commit", WorkspaceID: "workspace_tx", Type: "source.created",
			ActorID: "founder", Payload: []byte(`{"source":"fixture"}`), TraceID: traceID, CreatedAt: now,
		}); err != nil {
			return err
		}
		return tx.EnqueueOutbox(context.Background(), jobs.OutboxEvent{
			ID: "outbox_commit", WorkspaceID: "workspace_tx", Type: "source.created",
			Payload: []byte(`{"source":"fixture"}`), MaxAttempts: 3,
			AvailableAt: now, TraceID: traceID,
		})
	}); err != nil {
		t.Fatal(err)
	}
	assertCounts(t, pool, 1, 1)
}

func TestOutboxDispatcherPublishesRetriesAndDeadLetters(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	createWorkspace(t, pool, "workspace_outbox")
	now := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	publisher := &fakePublisher{failures: map[string]int{
		"outbox_a_retry":   2,
		"outbox_z_publish": 1,
	}}
	dispatcher := jobs.NewDispatcher(store, publisher, clock, jobs.BackoffFunc(func(int32) time.Duration {
		return 2 * time.Minute
	}), time.Minute)

	if err := store.WithTx(context.Background(), func(tx *pgstore.TxStore) error {
		for _, event := range []jobs.OutboxEvent{
			{ID: "outbox_a_retry", WorkspaceID: "workspace_outbox", Type: "source.created", Payload: []byte(`{}`), MaxAttempts: 2, AvailableAt: now, TraceID: traceID},
			{ID: "outbox_z_publish", WorkspaceID: "workspace_outbox", Type: "source.updated", Payload: []byte(`{}`), MaxAttempts: 2, AvailableAt: now, TraceID: traceID},
		} {
			if err := tx.EnqueueOutbox(context.Background(), event); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	processed, err := dispatcher.RunOne(context.Background(), "dispatcher-a")
	if err != nil || !processed {
		t.Fatalf("dispatch retry processed=%t err=%v", processed, err)
	}
	assertOutboxState(t, pool, "outbox_a_retry", "retryable", 1, "PUBLISH_FAILED")

	processed, err = dispatcher.RunOne(context.Background(), "dispatcher-a")
	if err != nil || !processed {
		t.Fatalf("dispatch publish processed=%t err=%v", processed, err)
	}
	assertOutboxState(t, pool, "outbox_z_publish", "retryable", 1, "PUBLISH_FAILED")

	clock.now = now.Add(2 * time.Minute)
	processed, err = dispatcher.RunOne(context.Background(), "dispatcher-a")
	if err != nil || !processed {
		t.Fatalf("dispatch dead letter processed=%t err=%v", processed, err)
	}
	assertOutboxState(t, pool, "outbox_a_retry", "dead_letter", 2, "PUBLISH_FAILED")

	processed, err = dispatcher.RunOne(context.Background(), "dispatcher-a")
	if err != nil || !processed {
		t.Fatalf("dispatch success processed=%t err=%v", processed, err)
	}
	assertOutboxState(t, pool, "outbox_z_publish", "published", 2, "")
	if got := publisher.publishedIDs(); fmt.Sprint(got) != fmt.Sprint([]string{"outbox_z_publish"}) {
		t.Fatalf("published IDs = %v", got)
	}
	var leaked bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM outbox_events
			WHERE row_to_json(outbox_events)::text LIKE '%publisher-secret%'
		)`).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked {
		t.Fatal("raw publisher error leaked into outbox persistence")
	}
}

const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

type fakeClock struct{ now time.Time }

func (clock *fakeClock) Now() time.Time { return clock.now }

type fakePublisher struct {
	mu        sync.Mutex
	failures  map[string]int
	published []string
}

func (publisher *fakePublisher) Publish(_ context.Context, event jobs.OutboxEvent) error {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if publisher.failures[event.ID] > 0 {
		publisher.failures[event.ID]--
		return errors.New("publisher-secret")
	}
	publisher.published = append(publisher.published, event.ID)
	return nil
}

func (publisher *fakePublisher) publishedIDs() []string {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	ids := append([]string(nil), publisher.published...)
	sort.Strings(ids)
	return ids
}

func newStore(t *testing.T) (*pgstore.Pool, *pgstore.Store) {
	t.Helper()
	pool, err := pgstore.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, pgstore.NewStore(pool)
}

func resetData(t *testing.T, pool *pgstore.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "TRUNCATE outbox_events, jobs, audit_events, workspaces CASCADE"); err != nil {
		t.Fatal(err)
	}
}

func createWorkspace(t *testing.T, pool *pgstore.Pool, id string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO workspaces (id, slug, display_name) VALUES ($1, $2, $3)`, id, id, id); err != nil {
		t.Fatal(err)
	}
}

func assertJobState(t *testing.T, pool *pgstore.Pool, id, status string, attempt int32, code string, available time.Time) {
	t.Helper()
	var gotStatus, gotCode string
	var gotAttempt int32
	var gotAvailable time.Time
	if err := pool.QueryRow(context.Background(), `
		SELECT status, attempt, COALESCE(last_error_code, ''), available_at
		FROM jobs WHERE id = $1`, id).Scan(&gotStatus, &gotAttempt, &gotCode, &gotAvailable); err != nil {
		t.Fatal(err)
	}
	if gotStatus != status || gotAttempt != attempt || gotCode != code {
		t.Fatalf("job state = %s attempt=%d code=%s", gotStatus, gotAttempt, gotCode)
	}
	if !available.IsZero() && !gotAvailable.Equal(available) {
		t.Fatalf("available_at = %s, want %s", gotAvailable, available)
	}
}

func assertJobStatus(t *testing.T, pool *pgstore.Pool, id, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(context.Background(), "SELECT status FROM jobs WHERE id = $1", id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("job %s status = %s, want %s", id, got, want)
	}
}

func assertCounts(t *testing.T, pool *pgstore.Pool, audit, outbox int) {
	t.Helper()
	var gotAudit, gotOutbox int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM audit_events").Scan(&gotAudit); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM outbox_events").Scan(&gotOutbox); err != nil {
		t.Fatal(err)
	}
	if gotAudit != audit || gotOutbox != outbox {
		t.Fatalf("audit/outbox counts = %d/%d, want %d/%d", gotAudit, gotOutbox, audit, outbox)
	}
}

func assertOutboxState(t *testing.T, pool *pgstore.Pool, id, status string, attempt int32, code string) {
	t.Helper()
	var gotStatus, gotCode string
	var gotAttempt int32
	if err := pool.QueryRow(context.Background(), `
		SELECT status, attempt, COALESCE(last_error_code, '')
		FROM outbox_events WHERE id = $1`, id).Scan(&gotStatus, &gotAttempt, &gotCode); err != nil {
		t.Fatal(err)
	}
	if gotStatus != status || gotAttempt != attempt || gotCode != code {
		t.Fatalf("outbox state = %s attempt=%d code=%s", gotStatus, gotAttempt, gotCode)
	}
}

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
