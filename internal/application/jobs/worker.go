package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	HandlerFailed   = "HANDLER_FAILED"
	HandlerNotFound = "HANDLER_NOT_FOUND"
	HandlerPanic    = "HANDLER_PANIC"
)

type Worker struct {
	repository    Repository
	clock         Clock
	backoff       Backoff
	leaseDuration time.Duration
	handlers      map[string]Handler
}

func NewWorker(repository Repository, clock Clock, backoff Backoff, leaseDuration time.Duration) *Worker {
	return &Worker{
		repository:    repository,
		clock:         clock,
		backoff:       backoff,
		leaseDuration: leaseDuration,
		handlers:      make(map[string]Handler),
	}
}

func (worker *Worker) Register(jobType string, handler Handler) {
	if jobType == "" || handler == nil {
		return
	}
	worker.handlers[jobType] = handler
}

func (worker *Worker) RunOne(ctx context.Context, owner string) (bool, error) {
	if owner == "" {
		return false, errors.New("worker owner is required")
	}
	if worker.repository == nil || worker.clock == nil || worker.backoff == nil || worker.leaseDuration <= 0 {
		return false, errors.New("worker is not configured")
	}

	now := worker.clock.Now().UTC()
	if err := worker.repository.ReapExpiredJobs(ctx, now); err != nil {
		return false, fmt.Errorf("reap expired jobs: %w", err)
	}
	job, err := worker.repository.ClaimJob(ctx, owner, now, worker.leaseDuration)
	if err != nil {
		return false, fmt.Errorf("claim job: %w", err)
	}
	if job == nil {
		return false, nil
	}

	errorCode, permanent, executionErr := worker.executeWithLease(ctx, owner, *job)
	if executionErr != nil {
		return true, executionErr
	}
	finishedAt := worker.clock.Now().UTC()
	if errorCode == "" {
		if err := worker.repository.MarkJobSucceeded(ctx, job.ID, owner, finishedAt); err != nil {
			return true, fmt.Errorf("mark job succeeded: %w", err)
		}
		return true, nil
	}

	delay := worker.backoff.Delay(job.Attempt)
	if delay < 0 {
		delay = 0
	}
	if err := worker.repository.MarkJobFailedWithDisposition(ctx, job.ID, owner, errorCode, permanent, finishedAt.Add(delay), finishedAt); err != nil {
		return true, fmt.Errorf("mark job failed: %w", err)
	}
	return true, nil
}

func (worker *Worker) executeWithLease(ctx context.Context, owner string, job Job) (string, bool, error) {
	handlerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		code      string
		permanent bool
	}
	done := make(chan result, 1)
	go func() {
		code, permanent := worker.execute(handlerCtx, job)
		done <- result{code: code, permanent: permanent}
	}()

	heartbeatEvery := worker.leaseDuration / 3
	if heartbeatEvery <= 0 {
		heartbeatEvery = time.Millisecond
	}
	ticker := time.NewTicker(heartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case completed := <-done:
			return completed.code, completed.permanent, nil
		case <-ticker.C:
			renewedAt := worker.clock.Now().UTC()
			if err := worker.repository.ExtendJobLease(handlerCtx, job.ID, owner, renewedAt, worker.leaseDuration); err != nil {
				cancel()
				select {
				case <-done:
				case <-time.After(heartbeatEvery):
				}
				return "", false, fmt.Errorf("extend job lease: %w", err)
			}
		case <-ctx.Done():
			cancel()
			select {
			case <-done:
			case <-time.After(heartbeatEvery):
			}
			return "", false, ctx.Err()
		}
	}
}

func (worker *Worker) Run(ctx context.Context, owner string, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		return errors.New("poll interval must be positive")
	}
	for {
		processed, err := worker.RunOne(ctx, owner)
		if err != nil {
			return err
		}
		if processed {
			continue
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil
		case <-timer.C:
		}
	}
}

func (worker *Worker) execute(ctx context.Context, job Job) (code string, permanent bool) {
	handler := worker.handlers[job.Type]
	if handler == nil {
		return HandlerNotFound, true
	}
	defer func() {
		if recover() != nil {
			code, permanent = HandlerPanic, false
		}
	}()
	if err := handler(ctx, job); err != nil {
		var failure *HandlerError
		if errors.As(err, &failure) {
			if failure.Code != "" {
				return failure.Code, failure.Permanent
			}
			return HandlerFailed, failure.Permanent
		}
		return HandlerFailed, false
	}
	return "", false
}
