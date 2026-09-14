package jobs

import (
	"context"
	"errors"
	"testing"
)

type publisherFunc func(context.Context, OutboxEvent) error

func (f publisherFunc) Publish(ctx context.Context, event OutboxEvent) error { return f(ctx, event) }
func TestRouterFanoutDoesNotBlockOtherSubscribers(t *testing.T) {
	router := NewRouterPublisher()
	called := 0
	router.Register("release.published", publisherFunc(func(context.Context, OutboxEvent) error { called++; return errors.New("git unavailable") }))
	router.Register("release.published", publisherFunc(func(context.Context, OutboxEvent) error { called++; return nil }))
	if err := router.Publish(context.Background(), OutboxEvent{Type: "release.published"}); err == nil || called != 2 {
		t.Fatalf("fanout error=%v called=%d", err, called)
	}
}
