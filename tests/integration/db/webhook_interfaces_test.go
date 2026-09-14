package db_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	adapter "github.com/iiwish/semlia/internal/adapters/webhooks"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/application/jobs"
	app "github.com/iiwish/semlia/internal/application/webhooks"
	domain "github.com/iiwish/semlia/internal/domain/webhooks"
	"github.com/iiwish/semlia/pkg/identity"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type failingGit struct{}

func (failingGit) Publish(context.Context, jobs.OutboxEvent) error {
	return errors.New("git unavailable")
}

func TestWebhookDurableIndependentSignedRetryAndRotation(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	fixture := seedDistributionReleases(t, pool)
	ctx := context.Background()
	store := pgstore.NewStore(pool)
	authorizer := authapp.NewService(store, authapp.ClockFunc(time.Now))
	admin, err := store.LoadDefaultPrincipal(ctx, fixture.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	secrets := map[string]string{}
	calls := map[string]int{}
	bodies := map[string][][]byte{}
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		calls[r.URL.Path]++
		bodies[r.URL.Path] = append(bodies[r.URL.Path], body)
		var envelope domain.Envelope
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Error(err)
		}
		timestamp := r.Header.Get("X-Semlia-Timestamp")
		seconds, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil || time.Since(time.Unix(seconds, 0)).Abs() > time.Minute {
			t.Error("signature timestamp invalid")
		}
		expected := app.Signature(secrets[r.URL.Path], timestamp, body)
		if !hmac.Equal([]byte(expected), []byte(r.Header.Get("X-Semlia-Signature"))) {
			t.Error("real receiver signature verification failed")
		}
		if envelope.ID.String() != r.Header.Get("X-Semlia-Event-ID") || envelope.WorkspaceID != fixture.workspaceID || !bytes.Contains(envelope.Data, []byte(fixture.release1.String())) {
			t.Error("safe event identity/data missing")
		}
		if bytes.Contains(body, []byte("DO_NOT_FORWARD")) {
			t.Error("internal metadata leaked")
		}
		if r.URL.Path == "/retry" && calls[r.URL.Path] == 1 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	service, err := app.NewService(store, authorizer, adapter.NewSafeClient([]netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}), bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	create := func(path string) domain.Subscription {
		t.Helper()
		issued, err := service.Save(ctx, app.SaveRequest{WorkspaceID: fixture.workspaceID, Actor: admin.ID.String(), TraceID: distributionTraceID, Name: path, Endpoint: receiver.URL + path, Enabled: true, EventTypes: []string{"release.published"}})
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		secrets[path] = issued.SigningSecret
		mu.Unlock()
		return issued.Subscription
	}
	first := create("/ok")
	create("/retry")
	makeEvent := func() jobs.OutboxEvent {
		t.Helper()
		id := newEventID(t)
		now := time.Now().UTC()
		payload, _ := json.Marshal(map[string]any{"specVersion": "semlia.events/v1", "id": id.String(), "type": "release.published", "source": "urn:semlia:control-plane", "workspaceId": fixture.workspaceID.String(), "time": now, "traceId": distributionTraceID, "data": map[string]any{"specVersion": "semlia.release/v1", "releaseId": fixture.release1.String(), "sequence": 1, "action": "published", "manifestDigest": distributionDigest("release-1"), "privateMetadata": "DO_NOT_FORWARD"}})
		if _, err := pool.Exec(ctx, `INSERT INTO outbox_events(id,workspace_id,event_type,payload,max_attempts,trace_id,created_at,updated_at) VALUES($1,$2,'release.published',$3,8,$4,$5,$5)`, id.UUID(), fixture.workspaceID.UUID(), payload, distributionTraceID, now); err != nil {
			t.Fatal(err)
		}
		return jobs.OutboxEvent{ID: id, WorkspaceID: fixture.workspaceID, Type: "release.published", Payload: payload, TraceID: distributionTraceID, CreatedAt: now}
	}
	event := makeEvent()
	router := jobs.NewRouterPublisher()
	router.Register(event.Type, failingGit{})
	router.Register(event.Type, service)
	if err := router.Publish(ctx, event); err == nil {
		t.Fatal("Git failure lost")
	}
	if err := router.Publish(ctx, event); err == nil {
		t.Fatal("Git retry missing")
	}
	deliveries, err := service.Deliveries(ctx, fixture.workspaceID, admin.ID.String(), distributionTraceID)
	if err != nil || len(deliveries) != 2 {
		t.Fatalf("durable per-subscriber fanout %d %v", len(deliveries), err)
	}
	create("/late")
	_ = router.Publish(ctx, event)
	deliveries, _ = service.Deliveries(ctx, fixture.workspaceID, admin.ID.String(), distributionTraceID)
	if len(deliveries) != 2 {
		t.Fatal("retry retrospectively subscribed a new endpoint")
	}
	for i := 0; i < 2; i++ {
		if processed, err := service.RunOne(ctx, "hooks"); err != nil || !processed {
			t.Fatalf("delivery %d %v %v", i, processed, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE webhook_deliveries SET next_attempt_at=CURRENT_TIMESTAMP-interval '1 minute' WHERE state='queued'`); err != nil {
		t.Fatal(err)
	}
	if processed, err := service.RunOne(ctx, "hooks"); err != nil || !processed {
		rows, _ := store.ListWebhookDeliveries(ctx, fixture.workspaceID)
		t.Fatal("retry", processed, err, rows, calls)
	}
	mu.Lock()
	if calls["/ok"] != 1 || calls["/retry"] != 2 || calls["/late"] != 0 {
		t.Error("independent delivery attempts", calls)
	}
	if !bytes.Equal(bodies["/retry"][0], bodies["/retry"][1]) {
		t.Error("retry changed immutable body")
	}
	mu.Unlock()
	deliveries, _ = service.Deliveries(ctx, fixture.workspaceID, admin.ID.String(), distributionTraceID)
	for _, d := range deliveries {
		if d.State != "succeeded" {
			t.Fatal("nonterminal delivery", d)
		}
	}
	var runs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM runtime_runs WHERE workspace_id=$1 AND kind='webhook_delivery' AND state='succeeded'`, fixture.workspaceID.UUID()).Scan(&runs); err != nil || runs != 2 {
		t.Fatal("Operations projection", runs, err)
	}
	event2 := makeEvent()
	if err := service.Publish(ctx, event2); err != nil {
		t.Fatal(err)
	}
	rotated, err := service.Save(ctx, app.SaveRequest{WorkspaceID: fixture.workspaceID, Actor: admin.ID.String(), TraceID: distributionTraceID, ID: first.ID, ExpectedVersion: first.Version, Rotate: true})
	if err != nil {
		t.Fatal(err)
	}
	if rotated.SigningSecret == "" || rotated.SigningSecret == secrets["/ok"] {
		t.Fatal("signing rotation missing")
	}
	var encrypted []byte
	if err := pool.QueryRow(ctx, `SELECT envelope FROM webhook_signing_secrets WHERE subscription_id=$1 AND version=2`, first.ID.UUID()).Scan(&encrypted); err != nil || bytes.Contains(encrypted, []byte(rotated.SigningSecret)) {
		t.Fatal("plaintext signing key persisted", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := service.RunOne(ctx, "hooks"); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	if calls["/ok"] != 1 {
		t.Error("old subscription version sent after rotation")
	}
	mu.Unlock()
	deliveries, _ = service.Deliveries(ctx, fixture.workspaceID, admin.ID.String(), distributionTraceID)
	found := false
	for _, d := range deliveries {
		if d.EventID == event2.ID && d.SubscriptionID.String() == first.ID.String() {
			found = true
			if d.State != "cancelled" || d.ErrorCode != "SUBSCRIPTION_CHANGED" || d.SigningVersion != 1 {
				t.Fatal("old version cancellation/provenance", d)
			}
		}
	}
	if !found {
		t.Fatal("cancelled delivery lost")
	}
	event3 := makeEvent()
	if err := service.Publish(ctx, event3); err != nil {
		t.Fatal(err)
	}
	_, err = service.Save(ctx, app.SaveRequest{WorkspaceID: fixture.workspaceID, Actor: admin.ID.String(), TraceID: distributionTraceID, ID: first.ID, ExpectedVersion: rotated.Subscription.Version, Name: "edited", Endpoint: receiver.URL + "/new-destination", Enabled: true, EventTypes: []string{"catalog.asset.changed"}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := service.RunOne(ctx, "hooks"); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	if calls["/new-destination"] != 0 || calls["/ok"] != 1 {
		t.Error("historical event retargeted by endpoint/filter edit")
	}
	mu.Unlock()
	deliveries, _ = service.Deliveries(ctx, fixture.workspaceID, admin.ID.String(), distributionTraceID)
	for _, d := range deliveries {
		if d.EventID == event3.ID && d.SubscriptionID == first.ID && (d.State != "cancelled" || d.SubscriptionVersion != rotated.Subscription.Version) {
			t.Fatal("edited subscription lost original provenance", d)
		}
	}
	t.Log("real HTTP receiver verified HMAC/timestamp/event identity; independent retry receipts and encrypted rotation passed")
}

func TestWebhookDeadLetterReplayIsBoundedAuditedAndVersionFenced(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	fixture := seedDistributionReleases(t, pool)
	ctx := context.Background()
	store := pgstore.NewStore(pool)
	admin, err := store.LoadDefaultPrincipal(ctx, fixture.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer receiver.Close()
	svc, err := app.NewService(store, authapp.NewService(store, authapp.ClockFunc(time.Now)), adapter.NewSafeClient([]netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}), bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	issued, err := svc.Save(ctx, app.SaveRequest{WorkspaceID: fixture.workspaceID, Actor: admin.ID.String(), TraceID: distributionTraceID, Name: "dead letter", Endpoint: receiver.URL, Enabled: true, EventTypes: []string{"release.published"}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	id := newEventID(t)
	payload, _ := json.Marshal(map[string]any{"specVersion": "semlia.events/v1", "id": id.String(), "type": "release.published", "source": "urn:semlia:control-plane", "workspaceId": fixture.workspaceID.String(), "time": now, "traceId": distributionTraceID, "data": map[string]string{"releaseId": fixture.release1.String()}})
	if _, err := pool.Exec(ctx, `INSERT INTO outbox_events(id,workspace_id,event_type,payload,max_attempts,trace_id)VALUES($1,$2,'release.published',$3,8,$4)`, id.UUID(), fixture.workspaceID.UUID(), payload, distributionTraceID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Publish(ctx, jobs.OutboxEvent{ID: id, WorkspaceID: fixture.workspaceID, Type: "release.published", Payload: payload, TraceID: distributionTraceID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	run := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE webhook_deliveries SET next_attempt_at=CURRENT_TIMESTAMP-interval '1 minute' WHERE state='queued'`); err != nil {
			t.Fatal(err)
		}
		if processed, err := svc.RunOne(ctx, "worker"); err != nil || !processed {
			t.Fatal(processed, err)
		}
	}
	for i := 0; i < 8; i++ {
		run()
	}
	rows, err := store.ListWebhookDeliveries(ctx, fixture.workspaceID)
	if err != nil || len(rows) != 1 || rows[0].State != "dead_letter" || rows[0].Attempt != 8 {
		t.Fatal("dead letter missing", rows, err)
	}
	delivery := rows[0]
	retryDelay := delivery.NextAttemptAt.Sub(delivery.UpdatedAt)
	if retryDelay < 192*time.Second || retryDelay >= 320*time.Second {
		t.Fatal("retry jitter outside bounded exponential window", retryDelay)
	}
	if _, err := svc.Replay(ctx, fixture.workspaceID, delivery.ID, 7, admin.ID.String(), distributionTraceID); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("stale replay accepted", err)
	}
	replay, err := svc.Replay(ctx, fixture.workspaceID, delivery.ID, 8, admin.ID.String(), distributionTraceID)
	if err != nil {
		t.Fatal(err)
	}
	if replay.State != "queued" || replay.MaxAttempts != 16 || replay.EventID != delivery.EventID || replay.PayloadDigest != delivery.PayloadDigest || replay.SubscriptionVersion != delivery.SubscriptionVersion {
		t.Fatal("replay altered identity", replay)
	}
	if _, err := svc.Replay(ctx, fixture.workspaceID, delivery.ID, 8, admin.ID.String(), distributionTraceID); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("duplicate replay accepted", err)
	}
	for i := 0; i < 8; i++ {
		run()
	}
	if _, err := svc.Replay(ctx, fixture.workspaceID, delivery.ID, 16, admin.ID.String(), distributionTraceID); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("unbounded replay accepted", err)
	}
	var audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE workspace_id=$1 AND event_type='webhook.delivery.replayed'`, fixture.workspaceID.UUID()).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("replay audit missing", audits, err)
	}
	_ = issued
}

func TestMigration20PreservesPopulated19AndLeaseFencing(t *testing.T) {
	resetSchema(t)
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Steps(19); err != nil {
		t.Fatal(err)
	}
	pool := openPool(t)
	fixture := seedDistributionReleases(t, pool)
	if version, dirty, err := migrator.Version(); err != nil || version != 19 || dirty {
		t.Fatal("migration19 baseline", version, dirty, err)
	}
	for _, step := range []int{1, -1, 1} {
		if err := migrator.Steps(step); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM releases WHERE workspace_id=$1`, fixture.workspaceID.UUID()).Scan(&count); err != nil || count != 2 {
			t.Fatal("prior release data lost", count, err)
		}
	}
	t.Log("populated migration19 ->20 ->19 ->20 preserved immutable releases")
}

func TestWebhookExpiredOwnerCannotFinishReclaimedDelivery(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	fixture := seedDistributionReleases(t, pool)
	ctx := context.Background()
	store := pgstore.NewStore(pool)
	now := time.Now().UTC()
	admin, err := store.LoadDefaultPrincipal(ctx, fixture.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := app.NewService(store, authapp.NewService(store, authapp.ClockFunc(time.Now)), adapter.NewSafeClient(nil), bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	// Use a public syntactically-valid endpoint; no transport is called in this lease test.
	subID, _ := identity.New(identity.WebhookSubscription)
	sub := domain.Subscription{ID: subID, WorkspaceID: fixture.workspaceID, Name: "lease", Endpoint: "https://example.com/", Enabled: true, EventTypes: []string{"release.published"}, Version: 1, SigningVersion: 1, SecretSuffix: "test", SecretEnvelope: bytes.Repeat([]byte{1}, 60), CreatedBy: admin.ID, CreatedAt: now, UpdatedAt: now}
	version, err := store.AuthorizationVersion(ctx, fixture.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveWebhookSubscription(ctx, sub, 0, version, admin.ID.String()); err != nil {
		t.Fatal(err)
	}
	eventID := newEventID(t)
	payload, _ := json.Marshal(map[string]any{"specVersion": "semlia.events/v1", "id": eventID.String(), "type": "release.published", "source": "urn:semlia:control-plane", "workspaceId": fixture.workspaceID.String(), "time": now, "traceId": distributionTraceID, "data": map[string]string{"releaseId": fixture.release1.String()}})
	if _, err := pool.Exec(ctx, `INSERT INTO outbox_events(id,workspace_id,event_type,payload,max_attempts,trace_id)VALUES($1,$2,'release.published',$3,8,$4)`, eventID.UUID(), fixture.workspaceID.UUID(), payload, distributionTraceID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Publish(ctx, jobs.OutboxEvent{ID: eventID, WorkspaceID: fixture.workspaceID, Type: "release.published", Payload: payload, TraceID: distributionTraceID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	a, err := store.ClaimWebhookDelivery(ctx, "owner-a", now.Add(time.Second))
	if err != nil || a == nil {
		t.Fatal(err)
	}
	b, err := store.ClaimWebhookDelivery(ctx, "owner-b", now.Add(32*time.Second))
	if err != nil || b == nil {
		t.Fatal(err)
	}
	if err := store.FinishWebhookDelivery(ctx, *a, "owner-a", now.Add(33*time.Second), 204, ""); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("stale owner finish accepted", err)
	}
	if err := store.FinishWebhookDelivery(ctx, *b, "owner-b", now.Add(33*time.Second), 204, ""); err != nil {
		t.Fatal("new owner receipt rejected", err)
	}
	rows, err := store.ListWebhookDeliveries(ctx, fixture.workspaceID)
	if err != nil || len(rows) != 1 || rows[0].Attempt != 2 || rows[0].State != "succeeded" {
		t.Fatal("fenced receipt", rows, err)
	}
	_ = strings.Builder{}
}
