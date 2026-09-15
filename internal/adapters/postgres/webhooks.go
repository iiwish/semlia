package postgres

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	"github.com/iiwish/semlia/internal/application/jobs"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	operations "github.com/iiwish/semlia/internal/domain/operations"
	domain "github.com/iiwish/semlia/internal/domain/webhooks"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"math/big"
	"strings"
	"time"
)

func (s *Store) SaveWebhookSubscription(ctx context.Context, sub domain.Subscription, expected int, authVersion int64, actor string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	workspace, _ := uuidValue(sub.WorkspaceID)
	id, _ := uuidValue(sub.ID)
	creator, _ := uuidValue(sub.CreatedBy)
	version, err := q.LockAuthorizationWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	if version != authVersion {
		return auth.ErrVersionConflict
	}
	if expected == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO webhook_subscriptions(id,workspace_id,name,endpoint,enabled,event_types,version,signing_version,secret_suffix,created_by_principal_id,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, workspace, sub.Name, sub.Endpoint, sub.Enabled, sub.EventTypes, sub.Version, sub.SigningVersion, sub.SecretSuffix, creator, sub.CreatedAt, sub.UpdatedAt)
	} else {
		result, e := tx.Exec(ctx, `UPDATE webhook_subscriptions SET name=$3,endpoint=$4,enabled=$5,event_types=$6,version=$7,signing_version=$8,secret_suffix=$9,updated_at=$10 WHERE id=$1 AND workspace_id=$2 AND version=$11`, id, workspace, sub.Name, sub.Endpoint, sub.Enabled, sub.EventTypes, sub.Version, sub.SigningVersion, sub.SecretSuffix, sub.UpdatedAt, expected)
		err = e
		if err == nil && result.RowsAffected() != 1 {
			return domain.ErrConflict
		}
	}
	if err != nil {
		return webhookError(err)
	}
	if len(sub.SecretEnvelope) > 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO webhook_signing_secrets(subscription_id,version,envelope,created_at) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, id, sub.SigningVersion, sub.SecretEnvelope, sub.UpdatedAt); err != nil {
			return webhookError(err)
		}
	}
	if err := createIdentityAudit(ctx, q, sub.WorkspaceID, actor, "webhook.subscription.saved", map[string]string{"subscriptionId": sub.ID.String()}, sub.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) ListWebhookSubscriptions(ctx context.Context, w identity.WorkspaceID) ([]domain.Subscription, error) {
	workspace, _ := uuidValue(w)
	rows, err := s.queries.ListWebhookSubscriptions(ctx, workspace)
	if err != nil {
		return nil, webhookError(err)
	}
	result := make([]domain.Subscription, 0, len(rows))
	for _, row := range rows {
		sub, err := webhookSubscription(row)
		if err != nil {
			return nil, err
		}
		result = append(result, sub)
	}
	return result, nil
}
func (s *Store) GetWebhookSubscription(ctx context.Context, w identity.WorkspaceID, id identity.ID) (domain.Subscription, error) {
	workspace, _ := uuidValue(w)
	value, _ := uuidValue(id)
	row, err := s.queries.GetWebhookSubscription(ctx, dbgen.GetWebhookSubscriptionParams{WorkspaceID: workspace, ID: value})
	if err != nil {
		return domain.Subscription{}, webhookError(err)
	}
	return webhookSubscription(row)
}
func (s *Store) ListWebhookDeliveries(ctx context.Context, w identity.WorkspaceID) ([]domain.Delivery, error) {
	workspace, _ := uuidValue(w)
	rows, err := s.queries.ListWebhookDeliveries(ctx, workspace)
	if err != nil {
		return nil, webhookError(err)
	}
	result := make([]domain.Delivery, 0, len(rows))
	for _, row := range rows {
		d, err := webhookDelivery(row)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, nil
}

func (s *Store) EnqueueWebhookDeliveries(ctx context.Context, event jobs.OutboxEvent, body []byte, digest string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	workspace, _ := uuidValue(event.WorkspaceID)
	eventID, _ := uuidValue(event.ID)
	// The event-level lock freezes first fanout. Retrying Git cannot subscribe a
	// newly-created endpoint retroactively to an older event.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, event.ID.String()+"/webhook"); err != nil {
		return err
	}
	mark, err := tx.Exec(ctx, `INSERT INTO webhook_fanout_receipts(event_id,created_at) VALUES($1,$2) ON CONFLICT DO NOTHING`, eventID, time.Now().UTC())
	if err != nil {
		return err
	}
	if mark.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	rows, err := q.MatchingWebhookSubscriptions(ctx, dbgen.MatchingWebhookSubscriptionsParams{WorkspaceID: workspace, Column2: event.Type})
	if err != nil {
		return webhookError(err)
	}
	for _, row := range rows {
		id, err := identity.New(identity.WebhookDelivery)
		if err != nil {
			return err
		}
		runID, err := identity.NewRunID()
		if err != nil {
			return err
		}
		subscription, err := identity.FromUUIDBytes(identity.WebhookSubscription, row.ID.Bytes)
		if err != nil {
			return err
		}
		value, _ := uuidValue(id)
		runValue, _ := uuidValue(runID)
		now := time.Now().UTC()
		_, err = tx.Exec(ctx, `INSERT INTO webhook_deliveries(id,workspace_id,subscription_id,subscription_version,signing_version,event_id,event_type,envelope,payload_digest,state,next_attempt_at,trace_id,created_at,updated_at,runtime_run_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'queued',$10,$11,$10,$10,$12) ON CONFLICT(subscription_id,event_id) DO NOTHING`, value, workspace, row.ID, row.Version, row.SigningVersion, eventID, event.Type, body, digest, now, event.TraceID, runValue)
		if err != nil {
			return webhookError(err)
		}
		delivery := domain.Delivery{ID: id, WorkspaceID: event.WorkspaceID, SubscriptionID: subscription, EventID: event.ID, EventType: event.Type, State: "queued", MaxAttempts: 8, PayloadDigest: digest, TraceID: event.TraceID, CreatedAt: now, UpdatedAt: now, RuntimeRunID: runID}
		if err := projectWebhookDelivery(ctx, q, delivery); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) ClaimWebhookDelivery(ctx context.Context, owner string, now time.Time) (*domain.Delivery, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	row, err := q.ClaimWebhookDelivery(ctx, dbgen.ClaimWebhookDeliveryParams{UpdatedAt: timestamp(now), LeaseOwner: optionalTextValue(owner)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, webhookError(err)
	}
	delivery, err := webhookDelivery(row)
	if err != nil {
		return nil, err
	}
	sub, err := q.GetWebhookSubscription(ctx, dbgen.GetWebhookSubscriptionParams{WorkspaceID: row.WorkspaceID, ID: row.SubscriptionID})
	if err != nil {
		return nil, webhookError(err)
	}
	delivery.Endpoint = sub.Endpoint
	if !sub.Enabled {
		delivery.ErrorCode = "SUBSCRIPTION_DISABLED"
	} else if int(sub.Version) != delivery.SubscriptionVersion {
		delivery.ErrorCode = "SUBSCRIPTION_CHANGED"
	} else {
		delivery.SecretEnvelope, err = q.LoadWebhookSecret(ctx, dbgen.LoadWebhookSecretParams{SubscriptionID: row.SubscriptionID, Version: row.SigningVersion})
		if err != nil {
			return nil, webhookError(err)
		}
	}
	if delivery.Attempt <= delivery.MaxAttempts {
		if err := projectWebhookDelivery(ctx, q, delivery); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &delivery, nil
}
func (s *Store) FinishWebhookDelivery(ctx context.Context, d domain.Delivery, owner string, now time.Time, status int, code string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	value, _ := uuidValue(d.ID)
	d.State = "succeeded"
	d.ErrorCode = code
	d.HTTPStatus = status
	d.UpdatedAt = now
	if code != "" {
		d.State = "queued"
		if d.Attempt >= d.MaxAttempts {
			d.State = "dead_letter"
		}
		if code == "SUBSCRIPTION_DISABLED" || code == "SUBSCRIPTION_CHANGED" {
			d.State = "cancelled"
		}
	}
	if d.Attempt > d.MaxAttempts {
		d.Attempt = d.MaxAttempts
	}
	delay := time.Duration(1<<min(d.Attempt, 8)) * time.Second
	jitter, err := rand.Int(rand.Reader, big.NewInt(int64(delay/2)))
	if err != nil {
		return err
	}
	d.NextAttemptAt = now.Add(delay*3/4 + time.Duration(jitter.Int64()))
	result, err := tx.Exec(ctx, `UPDATE webhook_deliveries SET state=$3,attempt=$4,http_status=$5,error_code=$6,next_attempt_at=$7,updated_at=$8,lease_owner=NULL,leased_until=NULL,subscription_version=$9,signing_version=$10 WHERE id=$1 AND lease_owner=$2 AND state='running' AND leased_until>$8`, value, owner, d.State, d.Attempt, status, code, d.NextAttemptAt, now, d.SubscriptionVersion, d.SigningVersion)
	if err != nil {
		return webhookError(err)
	}
	if result.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	if err := projectWebhookDelivery(ctx, q, d); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) ReplayWebhookDelivery(ctx context.Context, w identity.WorkspaceID, id identity.ID, attempt int, authVersion int64, actor string, now time.Time) (domain.Delivery, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Delivery{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	workspace, _ := uuidValue(w)
	value, _ := uuidValue(id)
	version, err := q.LockAuthorizationWorkspace(ctx, workspace)
	if err != nil {
		return domain.Delivery{}, err
	}
	if version != authVersion {
		return domain.Delivery{}, auth.ErrVersionConflict
	}
	// A single explicit replay extends the immutable delivery's total budget to
	// sixteen attempts. Event identity, envelope and subscription version never change.
	result, err := tx.Exec(ctx, `UPDATE webhook_deliveries d SET state='queued',max_attempts=16,next_attempt_at=$4,updated_at=$4,error_code='',http_status=0
 FROM webhook_subscriptions s WHERE d.id=$1 AND d.workspace_id=$2 AND d.attempt=$3 AND d.state='dead_letter' AND d.max_attempts=8
 AND s.id=d.subscription_id AND s.enabled AND s.version=d.subscription_version`, value, workspace, attempt, now)
	if err != nil {
		return domain.Delivery{}, webhookError(err)
	}
	if result.RowsAffected() != 1 {
		return domain.Delivery{}, domain.ErrConflict
	}
	row, err := q.GetWebhookDelivery(ctx, dbgen.GetWebhookDeliveryParams{WorkspaceID: workspace, ID: value})
	if err != nil {
		return domain.Delivery{}, webhookError(err)
	}
	delivery, err := webhookDelivery(row)
	if err != nil {
		return domain.Delivery{}, err
	}
	if err := projectWebhookDelivery(ctx, q, delivery); err != nil {
		return domain.Delivery{}, err
	}
	if err := createIdentityAudit(ctx, q, w, actor, "webhook.delivery.replayed", map[string]string{"deliveryId": id.String()}, now); err != nil {
		return domain.Delivery{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Delivery{}, err
	}
	return delivery, nil
}
func projectWebhookDelivery(ctx context.Context, q *dbgen.Queries, d domain.Delivery) error {
	run := operations.RuntimeRun{ID: d.RuntimeRunID, WorkspaceID: d.WorkspaceID, Kind: operations.RunKindWebhookDelivery, SourceType: "webhook_delivery", SourceID: d.ID.String(), SourceVersionDigest: strings.TrimPrefix(d.PayloadDigest, "sha256:"), TraceID: d.TraceID, IdempotencyKey: d.SubscriptionID.String() + "/" + d.EventID.String(), State: operations.RunState(d.State), Phase: "delivery", Attempt: int32(d.Attempt), MaxAttempts: int32(d.MaxAttempts), ErrorCode: d.ErrorCode, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
	if d.State == "succeeded" || d.State == "cancelled" || d.State == "dead_letter" {
		run.FinishedAt = &d.UpdatedAt
	}
	if d.State != "dead_letter" {
		run.ErrorCode = ""
	}
	if d.Attempt > 0 {
		run.StartedAt = &d.CreatedAt
	}
	event, err := newRuntimeEvent(d.WorkspaceID, d.RuntimeRunID, fmt.Sprintf("%s/%d", d.State, d.Attempt), operations.RunEventState, run.State, run.Phase, d.ErrorCode, "Webhook delivery "+d.State, d.UpdatedAt)
	if err != nil {
		return err
	}
	return projectRuntime(ctx, q, run, event)
}
func webhookSubscription(row dbgen.WebhookSubscription) (domain.Subscription, error) {
	id, err := identity.FromUUIDBytes(identity.WebhookSubscription, row.ID.Bytes)
	if err != nil {
		return domain.Subscription{}, err
	}
	workspace, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return domain.Subscription{}, err
	}
	creator, err := identity.PrincipalIDFromUUIDBytes(row.CreatedByPrincipalID.Bytes)
	if err != nil {
		return domain.Subscription{}, err
	}
	return domain.Subscription{ID: id, WorkspaceID: workspace, Name: row.Name, Endpoint: row.Endpoint, Enabled: row.Enabled, EventTypes: row.EventTypes, Version: int(row.Version), SigningVersion: int(row.SigningVersion), SecretSuffix: row.SecretSuffix, CreatedBy: creator, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}
func webhookDelivery(row dbgen.WebhookDelivery) (domain.Delivery, error) {
	id, err := identity.FromUUIDBytes(identity.WebhookDelivery, row.ID.Bytes)
	if err != nil {
		return domain.Delivery{}, err
	}
	workspace, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return domain.Delivery{}, err
	}
	sub, err := identity.FromUUIDBytes(identity.WebhookSubscription, row.SubscriptionID.Bytes)
	if err != nil {
		return domain.Delivery{}, err
	}
	event, err := identity.EventIDFromUUIDBytes(row.EventID.Bytes)
	if err != nil {
		return domain.Delivery{}, err
	}
	run, err := identity.RunIDFromUUIDBytes(row.RuntimeRunID.Bytes)
	if err != nil {
		return domain.Delivery{}, err
	}
	return domain.Delivery{ID: id, WorkspaceID: workspace, SubscriptionID: sub, SubscriptionVersion: int(row.SubscriptionVersion), SigningVersion: int(row.SigningVersion), EventID: event, EventType: row.EventType, State: row.State, Attempt: int(row.Attempt), MaxAttempts: int(row.MaxAttempts), HTTPStatus: int(row.HttpStatus), ErrorCode: row.ErrorCode, Envelope: row.Envelope, PayloadDigest: row.PayloadDigest, TraceID: row.TraceID, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, NextAttemptAt: row.NextAttemptAt.Time, RuntimeRunID: run}, nil
}
func webhookError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return fmt.Errorf("webhook repository: %w", errRepositoryOperation)
}
