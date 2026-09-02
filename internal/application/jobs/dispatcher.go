package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	PublishFailed        = "PUBLISH_FAILED"
	PublishPanic         = "PUBLISH_PANIC"
	PublishNotRegistered = "PUBLISH_NOT_REGISTERED"
)

type Dispatcher struct {
	repository    OutboxRepository
	publisher     Publisher
	clock         Clock
	backoff       Backoff
	leaseDuration time.Duration
}

func NewDispatcher(
	repository OutboxRepository,
	publisher Publisher,
	clock Clock,
	backoff Backoff,
	leaseDuration time.Duration,
) *Dispatcher {
	return &Dispatcher{
		repository:    repository,
		publisher:     publisher,
		clock:         clock,
		backoff:       backoff,
		leaseDuration: leaseDuration,
	}
}

// RunOne claims one outbox event and drives it to a terminal state.
//
// Unregistered event types complete in a single attempt: the router answers
// ErrPublisherNotRegistered (fail-closed for registered types), and because an
// event with no registered subscriber needs no delivery, the dispatcher marks
// the event published instead of scheduling retries. This is the M2-T002
// dispatcher policy for proposal.changed / release.published, which are
// enqueued by the governance commits before T003 wires their first
// publishers — unregistered types must neither retry forever nor poison the
// dispatch loop with dead letters.
func (dispatcher *Dispatcher) RunOne(ctx context.Context, owner string) (bool, error) {
	if owner == "" {
		return false, errors.New("dispatcher owner is required")
	}
	if dispatcher.repository == nil || dispatcher.publisher == nil || dispatcher.clock == nil || dispatcher.backoff == nil || dispatcher.leaseDuration <= 0 {
		return false, errors.New("dispatcher is not configured")
	}

	now := dispatcher.clock.Now().UTC()
	if err := dispatcher.repository.ReapExpiredOutboxEvents(ctx, now); err != nil {
		return false, fmt.Errorf("reap expired outbox events: %w", err)
	}
	event, err := dispatcher.repository.ClaimOutboxEvent(ctx, owner, now, dispatcher.leaseDuration)
	if err != nil {
		return false, fmt.Errorf("claim outbox event: %w", err)
	}
	if event == nil {
		return false, nil
	}

	errorCode := dispatcher.publish(ctx, *event)
	finishedAt := dispatcher.clock.Now().UTC()
	if errorCode == "" {
		if err := dispatcher.repository.MarkOutboxEventPublished(ctx, event.ID, owner, finishedAt); err != nil {
			return true, fmt.Errorf("mark outbox event published: %w", err)
		}
		return true, nil
	}
	if errorCode == PublishNotRegistered {
		if err := dispatcher.repository.MarkOutboxEventPublished(ctx, event.ID, owner, finishedAt); err != nil {
			return true, fmt.Errorf("mark unregistered outbox event completed: %w", err)
		}
		return true, nil
	}
	delay := dispatcher.backoff.Delay(event.Attempt)
	if delay < 0 {
		delay = 0
	}
	if err := dispatcher.repository.MarkOutboxEventFailed(ctx, event.ID, owner, errorCode, finishedAt.Add(delay), finishedAt); err != nil {
		return true, fmt.Errorf("mark outbox event failed: %w", err)
	}
	return true, nil
}

func (dispatcher *Dispatcher) Run(ctx context.Context, owner string, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		return errors.New("poll interval must be positive")
	}
	for {
		processed, err := dispatcher.RunOne(ctx, owner)
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

func (dispatcher *Dispatcher) publish(ctx context.Context, event OutboxEvent) (code string) {
	defer func() {
		if recover() != nil {
			code = PublishPanic
		}
	}()
	if err := dispatcher.publisher.Publish(ctx, event); err != nil {
		if errors.Is(err, ErrPublisherNotRegistered) {
			return PublishNotRegistered
		}
		return PublishFailed
	}
	return ""
}
