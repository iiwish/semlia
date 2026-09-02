package jobs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/iiwish/semlia/internal/application/jobs"
)

func TestRouterPublisherFailsClosed(t *testing.T) {
	router := jobs.NewRouterPublisher()
	if err := router.Publish(context.Background(), jobs.OutboxEvent{Type: "unknown"}); !errors.Is(err, jobs.ErrPublisherNotRegistered) {
		t.Fatalf("unknown event error = %v", err)
	}
	called := false
	router.Register("known", publisherFunc(func(context.Context, jobs.OutboxEvent) error {
		called = true
		return nil
	}))
	if err := router.Publish(context.Background(), jobs.OutboxEvent{Type: "known"}); err != nil || !called {
		t.Fatalf("known publish called=%t err=%v", called, err)
	}
}

type publisherFunc func(context.Context, jobs.OutboxEvent) error

func (fn publisherFunc) Publish(ctx context.Context, event jobs.OutboxEvent) error {
	return fn(ctx, event)
}
