package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// failingValidationRepository injects deterministic infra failures into the
// validation job handler while delegating everything else to the real store.
type failingValidationRepository struct {
	*pgstore.Store
	failOnCalls map[string]int
	calls       map[string]int
}

func newFailingValidationRepository(store *pgstore.Store, method string, failures int) *failingValidationRepository {
	return &failingValidationRepository{
		Store:       store,
		failOnCalls: map[string]int{method: failures},
		calls:       map[string]int{},
	}
}

func (repository *failingValidationRepository) fail(name string) error {
	repository.calls[name]++
	if repository.failOnCalls[name] > 0 {
		repository.failOnCalls[name]--
		return errors.New("postgres://user:database-secret@private.invalid/semlia")
	}
	return nil
}

func (repository *failingValidationRepository) GetProposal(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) (governance.Proposal, error) {
	if err := repository.fail("GetProposal"); err != nil {
		return governance.Proposal{}, err
	}
	return repository.Store.GetProposal(ctx, workspace, proposal)
}

func (repository *failingValidationRepository) ListProposalChanges(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) ([]governance.ChangeSetItem, error) {
	if err := repository.fail("ListProposalChanges"); err != nil {
		return nil, err
	}
	return repository.Store.ListProposalChanges(ctx, workspace, proposal)
}

func seedValidatingProposal(t *testing.T, pool *pgstore.Pool, workspace identity.WorkspaceID, slug string) identity.ProposalID {
	t.Helper()
	proposalID := mustValidationTestID(t, identity.NewProposalID)
	now := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO proposals (id, workspace_id, target_object_type, target_object_id, state, title, created_by, created_at, updated_at)
		VALUES ($1, $2, 'model_grain', $3, 'draft', $4, 'founder', $5, $5)`,
		proposalID.UUID(), workspace.UUID(), proposalID.UUID(), slug+" change", now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO proposal_changes (id, workspace_id, proposal_id, field_path, op, after_digest, after_value, created_at)
		VALUES ($1, $2, $3, 'grainExpression', 'add', $4, '"order_id"', $5)`,
		mustValidationTestID(t, identity.NewProposalChangeID).UUID(), workspace.UUID(), proposalID.UUID(),
		"sha256:"+fmt.Sprintf("%064x", 10), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE proposals SET state = 'proposed', submitted_at = $2 WHERE id = $1`, proposalID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE proposals SET state = 'validating', updated_at = $2 WHERE id = $1`, proposalID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	return proposalID
}

func newValidationWorker(
	t *testing.T,
	store *pgstore.Store,
	handlerRepository governanceapp.ValidationJobRepository,
	clock *fakeClock,
) *jobs.Worker {
	t.Helper()
	handlerClock := governanceapp.ClockFunc(func() time.Time { return clock.now })
	handler := governanceapp.NewValidationJobHandler(
		handlerRepository,
		governanceapp.NewProposalService(store, handlerClock),
		governanceapp.NewValidationService(store, handlerClock),
		governanceapp.NewDefaultRegistry(),
		governanceapp.NewPolicyService(store, handlerClock, governanceapp.WithRuleSource(store)),
		handlerClock,
	)
	worker := jobs.NewWorker(store, clock, jobs.BackoffFunc(func(int32) time.Duration {
		return time.Second
	}), time.Minute)
	worker.Register(governanceapp.ValidationJobType, handler.Handle)
	return worker
}

func enqueueValidationJob(t *testing.T, store *pgstore.Store, workspace identity.WorkspaceID, proposal identity.ProposalID, maxAttempts int32) identity.RunID {
	t.Helper()
	payload, err := json.Marshal(governanceapp.ValidationJobPayload{ProposalID: proposal.String(), Attempt: 1})
	if err != nil {
		t.Fatal(err)
	}
	job, err := store.EnqueueJob(context.Background(), jobs.EnqueueJobParams{
		ID: mustValidationTestID(t, identity.NewRunID), WorkspaceID: workspace, Type: governanceapp.ValidationJobType,
		Payload: payload, MaxAttempts: maxAttempts,
		AvailableAt:    time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC),
		IdempotencyKey: governanceapp.ValidationJobIdempotencyKey(proposal, 1), TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return job.ID
}

func TestValidationJobRetriesInfraFailureThenDegradesToHumanHandling(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspaceID := newWorkspaceID(t)
	createWorkspace(t, pool, workspaceID, "worker-validation-degrade")
	proposalID := seedValidatingProposal(t, pool, workspaceID, "degrade")

	now := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	repository := newFailingValidationRepository(store, "ListProposalChanges", 2)
	worker := newValidationWorker(t, store, repository, clock)
	jobID := enqueueValidationJob(t, store, workspaceID, proposalID, 2)

	if processed, err := worker.RunOne(context.Background(), "worker-degrade"); err != nil || !processed {
		t.Fatalf("first attempt processed=%t err=%v", processed, err)
	}
	assertJobState(t, pool, jobID, "retryable", 1, "HANDLER_FAILED", now.Add(time.Second))

	clock.now = now.Add(time.Second)
	if processed, err := worker.RunOne(context.Background(), "worker-degrade"); err != nil || !processed {
		t.Fatalf("final attempt processed=%t err=%v", processed, err)
	}
	assertJobState(t, pool, jobID, "dead_letter", 2, "HANDLER_FAILED", time.Time{})

	var proposalState string
	if err := pool.QueryRow(context.Background(), `
		SELECT state FROM proposals WHERE id = $1`, proposalID.UUID()).Scan(&proposalState); err != nil {
		t.Fatal(err)
	}
	if proposalState != string(governance.ProposalInReview) {
		t.Fatalf("proposal state after infra failure = %q, want in_review", proposalState)
	}

	var runStatus, validatorID string
	var resultCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT r.status, r.validator_id, count(res.id)
		FROM validation_runs r
		LEFT JOIN validation_results res ON res.validation_run_id = r.id
		WHERE r.workspace_id = $1 AND r.proposal_id = $2
		GROUP BY r.status, r.validator_id`,
		workspaceID.UUID(), proposalID.UUID()).Scan(&runStatus, &validatorID, &resultCount); err != nil {
		t.Fatal(err)
	}
	if runStatus != string(governance.ValidationFailed) || validatorID != "orchestration" || resultCount != 1 {
		t.Fatalf("degradation run = %s/%s with %d results, want one failed orchestration record", runStatus, validatorID, resultCount)
	}
	var leaked bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM validation_results
			WHERE row_to_json(validation_results)::text LIKE '%database-secret%'
		)`).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked {
		t.Fatal("infra error leaked into validation persistence")
	}
}

func TestValidationJobCompletesWithoutTransitionWhenProposalLeftValidating(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspaceID := newWorkspaceID(t)
	createWorkspace(t, pool, workspaceID, "worker-validation-rejected")
	proposalID := mustValidationTestID(t, identity.NewProposalID)
	now := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO proposals (id, workspace_id, target_object_type, target_object_id, state, title, created_by, created_at, updated_at)
		VALUES ($1, $2, 'model_grain', $3, 'draft', 'rejected concurrently', 'founder', $4, $4)`,
		proposalID.UUID(), workspaceID.UUID(), proposalID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE proposals SET state = 'proposed', submitted_at = $2 WHERE id = $1`, proposalID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE proposals SET state = 'rejected', decided_at = $2, updated_at = $2 WHERE id = $1`, proposalID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	jobID := enqueueValidationJob(t, store, workspaceID, proposalID, 3)

	worker := newValidationWorker(t, store, store, &fakeClock{now: now})
	if processed, err := worker.RunOne(context.Background(), "worker-rejected"); err != nil || !processed {
		t.Fatalf("worker processed=%t err=%v", processed, err)
	}
	assertJobStatus(t, pool, jobID, "succeeded")

	var state string
	if err := pool.QueryRow(context.Background(), `
		SELECT state FROM proposals WHERE id = $1`, proposalID.UUID()).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != string(governance.ProposalRejected) {
		t.Fatalf("rejected proposal moved to %q", state)
	}
	var runCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM validation_runs WHERE proposal_id = $1`, proposalID.UUID()).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if runCount != 0 {
		t.Fatalf("rejected proposal accumulated %d validation runs", runCount)
	}
}

func mustValidationTestID[T any](t *testing.T, create func() (T, error)) T {
	t.Helper()
	value, err := create()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
