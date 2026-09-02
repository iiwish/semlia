package jobs

import (
	"context"
	"errors"
)

var ErrPublisherNotRegistered = errors.New("outbox publisher is not registered")

type RouterPublisher struct {
	publishers map[string]Publisher
}

func NewRouterPublisher() *RouterPublisher {
	return &RouterPublisher{publishers: make(map[string]Publisher)}
}

func (router *RouterPublisher) Register(eventType string, publisher Publisher) {
	if eventType == "" || publisher == nil {
		return
	}
	router.publishers[eventType] = publisher
}

func (router *RouterPublisher) Publish(ctx context.Context, event OutboxEvent) error {
	publisher := router.publishers[event.Type]
	if publisher == nil {
		return ErrPublisherNotRegistered
	}
	return publisher.Publish(ctx, event)
}
