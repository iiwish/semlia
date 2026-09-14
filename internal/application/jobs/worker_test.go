package jobs_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/pkg/identity"
)

type workerRepository struct {
	mu          sync.Mutex
	job         *jobs.Job
	extendError error
	extensions  int
	succeeded   int
	failed      int
	permanent   bool
}

func (repository *workerRepository) ReapExpiredJobs(context.Context, time.Time) error { return nil }
func (repository *workerRepository) ClaimJob(_ context.Context, owner string, now time.Time, lease time.Duration) (*jobs.Job, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.job == nil {
		return nil, nil
	}
	claimed := *repository.job
	claimed.LeaseOwner = owner
	until := now.Add(lease)
	claimed.LeasedUntil = &until
	repository.job = nil
	return &claimed, nil
}
func (repository *workerRepository) ExtendJobLease(context.Context, identity.RunID, string, time.Time, time.Duration) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.extensions++
	return repository.extendError
}
func (repository *workerRepository) MarkJobSucceeded(context.Context, identity.RunID, string, time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.succeeded++
	return nil
}
func (repository *workerRepository) MarkJobFailedWithDisposition(_ context.Context, _ identity.RunID, _, _ string, permanent bool, _, _ time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.failed++
	repository.permanent = permanent
	return nil
}

func TestWorkerRenewsLeaseDuringLongHandler(t *testing.T) {
	jobID, _ := identity.NewRunID()
	workspace, _ := identity.NewWorkspaceID()
	repository := &workerRepository{job: &jobs.Job{ID: jobID, WorkspaceID: workspace, Type: "long", Attempt: 1, MaxAttempts: 2}}
	worker := jobs.NewWorker(repository, jobs.ClockFunc(time.Now), jobs.BackoffFunc(func(int32) time.Duration { return 0 }), 30*time.Millisecond)
	worker.Register("long", func(ctx context.Context, _ jobs.Job) error {
		select {
		case <-time.After(45 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	processed, err := worker.RunOne(context.Background(), "worker-a")
	if err != nil || !processed || repository.extensions < 2 || repository.succeeded != 1 {
		t.Fatalf("processed=%v err=%v extensions=%d succeeded=%d", processed, err, repository.extensions, repository.succeeded)
	}
}

func TestWorkerAbandonsLeaseLostHandlerWithoutTerminalWrite(t *testing.T) {
	jobID, _ := identity.NewRunID()
	workspace, _ := identity.NewWorkspaceID()
	repository := &workerRepository{job: &jobs.Job{ID: jobID, WorkspaceID: workspace, Type: "stuck", Attempt: 1, MaxAttempts: 2}, extendError: errors.New("lease lost")}
	worker := jobs.NewWorker(repository, jobs.ClockFunc(time.Now), jobs.BackoffFunc(func(int32) time.Duration { return 0 }), 30*time.Millisecond)
	worker.Register("stuck", func(context.Context, jobs.Job) error {
		time.Sleep(time.Second)
		return nil
	})
	started := time.Now()
	processed, err := worker.RunOne(context.Background(), "worker-a")
	if err == nil || !processed || time.Since(started) > 100*time.Millisecond {
		t.Fatalf("processed=%v err=%v duration=%s", processed, err, time.Since(started))
	}
	if repository.succeeded != 0 || repository.failed != 0 {
		t.Fatalf("terminal writes succeeded=%d failed=%d", repository.succeeded, repository.failed)
	}
}

func TestWorkerMarksPermanentHandlerFailureWithoutRetryDisposition(t *testing.T) {
	jobID, _ := identity.NewRunID()
	workspace, _ := identity.NewWorkspaceID()
	repository := &workerRepository{job: &jobs.Job{ID: jobID, WorkspaceID: workspace, Type: "unsafe", Attempt: 1, MaxAttempts: 3}}
	worker := jobs.NewWorker(repository, jobs.ClockFunc(time.Now), jobs.BackoffFunc(func(int32) time.Duration { return 0 }), time.Second)
	worker.Register("unsafe", func(context.Context, jobs.Job) error {
		return jobs.Permanent(errors.New("unsafe input"), "ARTIFACT_UNSAFE")
	})
	processed, err := worker.RunOne(context.Background(), "worker-a")
	if err != nil || !processed || repository.failed != 1 || !repository.permanent {
		t.Fatalf("processed=%v err=%v failed=%d permanent=%v", processed, err, repository.failed, repository.permanent)
	}
}
