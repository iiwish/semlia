package jobs

import (
	"context"
	"errors"
)

var ErrPublisherNotRegistered = errors.New("outbox publisher is not registered")

type RouterPublisher struct {
	publishers map[string][]Publisher
}

func NewRouterPublisher() *RouterPublisher {
	return &RouterPublisher{publishers: make(map[string][]Publisher)}
}

func (router *RouterPublisher) Register(eventType string, publisher Publisher) {
	if eventType == "" || publisher == nil {
		return
	}
	router.publishers[eventType] = append(router.publishers[eventType], publisher)
}

func (router *RouterPublisher) Publish(ctx context.Context, event OutboxEvent) error {
	publishers := router.publishers[event.Type]
	if len(publishers) == 0 {
		return ErrPublisherNotRegistered
	}
	var result error
	for _, publisher := range publishers {
		result = errors.Join(result, publishSubscriber(ctx, publisher, event))
	}
	return result
}

func publishSubscriber(ctx context.Context, publisher Publisher, event OutboxEvent) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("subscriber failed")
		}
	}()
	return publisher.Publish(ctx, event)
}
