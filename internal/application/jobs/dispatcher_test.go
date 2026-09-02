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

const fakeTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

type fakeOutboxRepository struct {
	mu          sync.Mutex
	queued      []jobs.OutboxEvent
	published   []identity.EventID
	failed      []identity.EventID
	failNextGet bool
}

func (repository *fakeOutboxRepository) ReapExpiredOutboxEvents(context.Context, time.Time) error {
	return nil
}

func (repository *fakeOutboxRepository) ClaimOutboxEvent(
	_ context.Context, owner string, now time.Time, lease time.Duration,
) (*jobs.OutboxEvent, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.failNextGet {
		repository.failNextGet = false
		return nil, errors.New("claim boom")
	}
	for index, event := range repository.queued {
		if event.Status == "queued" && !event.AvailableAt.After(now) {
			claimed := repository.queued[index]
			claimed.Status = "publishing"
			claimed.Attempt++
			claimed.LeaseOwner = owner
			leasedUntil := now.Add(lease)
			claimed.LeasedUntil = &leasedUntil
			repository.queued[index] = claimed
			return &claimed, nil
		}
	}
	return nil, nil
}

func (repository *fakeOutboxRepository) MarkOutboxEventPublished(
	_ context.Context, id identity.EventID, _ string, publishedAt time.Time,
) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.published = append(repository.published, id)
	repository.markLocked(id, func(event *jobs.OutboxEvent) {
		event.Status = "published"
		event.PublishedAt = &publishedAt
		event.LastErrorCode = ""
	})
	return nil
}

func (repository *fakeOutboxRepository) MarkOutboxEventFailed(
	_ context.Context, id identity.EventID, _ string, errorCode string, availableAt, _ time.Time,
) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.failed = append(repository.failed, id)
	repository.markLocked(id, func(event *jobs.OutboxEvent) {
		if event.Attempt >= event.MaxAttempts {
			event.Status = "dead_letter"
		} else {
			event.Status = "retryable"
			event.AvailableAt = availableAt
		}
		event.LastErrorCode = errorCode
	})
	return nil
}

func (repository *fakeOutboxRepository) markLocked(id identity.EventID, mutate func(*jobs.OutboxEvent)) {
	for index := range repository.queued {
		if repository.queued[index].ID == id {
			mutate(&repository.queued[index])
			return
		}
	}
}

func (repository *fakeOutboxRepository) eventByID(id identity.EventID) (jobs.OutboxEvent, bool) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for _, event := range repository.queued {
		if event.ID == id {
			return event, true
		}
	}
	return jobs.OutboxEvent{}, false
}

func (repository *fakeOutboxRepository) publishedIDs() []identity.EventID {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return append([]identity.EventID(nil), repository.published...)
}

type recordingPublisher struct {
	mu   sync.Mutex
	seen []string
	err  error
}

func (publisher *recordingPublisher) Publish(_ context.Context, event jobs.OutboxEvent) error {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	publisher.seen = append(publisher.seen, event.Type)
	return publisher.err
}

func (publisher *recordingPublisher) publishedTypes() []string {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	return append([]string(nil), publisher.seen...)
}

func enqueueFakeEvent(t *testing.T, repository *fakeOutboxRepository, eventType string) identity.EventID {
	t.Helper()
	id, err := identity.NewEventID()
	if err != nil {
		t.Fatal(err)
	}
	repository.mu.Lock()
	repository.queued = append(repository.queued, jobs.OutboxEvent{
		ID: id, Type: eventType, Payload: []byte(`{}`), Status: "queued",
		Attempt: 0, MaxAttempts: 3, AvailableAt: time.Unix(0, 0).UTC(), TraceID: fakeTraceID,
	})
	repository.mu.Unlock()
	return id
}

// TestUnregisteredEventTypeCompletesWithoutRetry locks the dispatcher policy
// for M2-T002: proposal.changed and release.published are enqueued by the
// governance commits before any publisher is registered (T003 wires the first
// consumers). The router stays fail-closed, and the dispatcher must complete
// these events in exactly one attempt instead of burning retries and
// dead-lettering them; a registered type keeps the ordinary publish path.
func TestUnregisteredEventTypeCompletesWithoutRetry(t *testing.T) {
	repository := &fakeOutboxRepository{}
	router := jobs.NewRouterPublisher()
	publisher := &recordingPublisher{}
	router.Register("catalog.asset.changed", publisher)
	clock := jobs.ClockFunc(func() time.Time { return time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC) })
	dispatcher := jobs.NewDispatcher(repository, router, clock, jobs.BackoffFunc(func(int32) time.Duration {
		return time.Hour
	}), time.Minute)

	skipID := enqueueFakeEvent(t, repository, "proposal.changed")
	publishID := enqueueFakeEvent(t, repository, "catalog.asset.changed")

	for range 2 {
		processed, err := dispatcher.RunOne(context.Background(), "dispatcher-skip")
		if err != nil || !processed {
			t.Fatalf("RunOne processed=%t err=%v", processed, err)
		}
	}

	event, ok := repository.eventByID(skipID)
	if !ok {
		t.Fatal("skipped event disappeared")
	}
	if event.Status != "published" {
		t.Fatalf("unregistered event status = %s, want published in one attempt (no retry churn)", event.Status)
	}
	if event.Attempt != 1 || event.LastErrorCode != "" {
		t.Fatalf("unregistered event attempt=%d code=%q, want a single clean attempt", event.Attempt, event.LastErrorCode)
	}
	ids := repository.publishedIDs()
	if len(ids) != 2 || (ids[0] != skipID && ids[1] != skipID) {
		t.Fatalf("published IDs = %v, want both events completed", ids)
	}
	if got := publisher.publishedTypes(); len(got) != 1 || got[0] != "catalog.asset.changed" {
		t.Fatalf("publisher saw %v, want only the registered type", got)
	}
	published, ok := repository.eventByID(publishID)
	if !ok || published.Status != "published" {
		t.Fatalf("registered event status = %+v", published)
	}
}

// TestPublisherFailureStillRetries keeps the ordinary failure path honest:
// only ErrPublisherNotRegistered is a completion signal, any other publisher
// error schedules a retry and eventually dead-letters.
func TestPublisherFailureStillRetries(t *testing.T) {
	repository := &fakeOutboxRepository{}
	router := jobs.NewRouterPublisher()
	router.Register("catalog.asset.changed", &recordingPublisher{err: errors.New("downstream broken")})
	clock := jobs.ClockFunc(func() time.Time { return time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC) })
	dispatcher := jobs.NewDispatcher(repository, router, clock, jobs.BackoffFunc(func(int32) time.Duration {
		return time.Hour
	}), time.Minute)

	failingID := enqueueFakeEvent(t, repository, "catalog.asset.changed")
	repository.queued[0].MaxAttempts = 1

	if _, err := dispatcher.RunOne(context.Background(), "dispatcher-retry"); err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	event, ok := repository.eventByID(failingID)
	if !ok {
		t.Fatal("failing event disappeared")
	}
	if event.Status != "dead_letter" || event.LastErrorCode != jobs.PublishFailed {
		t.Fatalf("failing event = status %s code %q, want dead_letter/PUBLISH_FAILED", event.Status, event.LastErrorCode)
	}
}
