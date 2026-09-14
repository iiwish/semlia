package webhooks

import (
	"bytes"
	"context"
	domain "github.com/iiwish/semlia/internal/domain/webhooks"
	"testing"
	"time"
)

type leaseConflictRepository struct {
	Repository
	claims, finishes int
}

func (r *leaseConflictRepository) ClaimWebhookDelivery(context.Context, string, time.Time) (*domain.Delivery, error) {
	r.claims++
	if r.claims > 2 {
		return nil, nil
	}
	return &domain.Delivery{Attempt: 1, MaxAttempts: 8, ErrorCode: "SUBSCRIPTION_DISABLED"}, nil
}
func (r *leaseConflictRepository) FinishWebhookDelivery(context.Context, domain.Delivery, string, time.Time, int, string) error {
	r.finishes++
	if r.finishes == 1 {
		return domain.ErrConflict
	}
	return nil
}
func TestStaleWebhookLeaseDoesNotStopNextDelivery(t *testing.T) {
	repo := &leaseConflictRepository{}
	service, err := NewService(repo, nil, nil, bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		processed, err := service.RunOne(context.Background(), "worker")
		if err != nil || !processed {
			t.Fatalf("attempt %d halted worker: %v %v", i, processed, err)
		}
	}
	if repo.claims != 2 || repo.finishes != 2 {
		t.Fatal("subsequent delivery did not finish")
	}
}
