package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	PublishFailed = "PUBLISH_FAILED"
	PublishPanic  = "PUBLISH_PANIC"
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
	delay := dispatcher.backoff.Delay(event.Attempt)
	if delay < 0 {
		delay = 0
	}
	if err := dispatcher.repository.MarkOutboxEventFailed(ctx, event.ID, owner, errorCode, finishedAt.Add(delay), finishedAt); err != nil {
		return true, fmt.Errorf("mark outbox event failed: %w", err)
	}
	return true, nil
}

func (dispatcher *Dispatcher) publish(ctx context.Context, event OutboxEvent) (code string) {
	defer func() {
		if recover() != nil {
			code = PublishPanic
		}
	}()
	if err := dispatcher.publisher.Publish(ctx, event); err != nil {
		return PublishFailed
	}
	return ""
}
