package postgres

import (
	"context"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	application "github.com/iiwish/semlia/internal/application/usage"
	domain "github.com/iiwish/semlia/internal/domain/usage"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ application.Repository = (*Store)(nil)

func (store *Store) RecordUsageEvent(ctx context.Context, event domain.Event) error {
	id, err := uuidValue(event.ID)
	if err != nil {
		return err
	}
	workspaceID, err := uuidValue(event.WorkspaceID)
	if err != nil {
		return err
	}
	assetID := pgtype.UUID{}
	if event.AssetID != nil {
		assetID, err = uuidValue(*event.AssetID)
		if err != nil {
			return err
		}
	}
	revisionID := pgtype.UUID{}
	if event.RevisionID != nil {
		revisionID, err = uuidValue(*event.RevisionID)
		if err != nil {
			return err
		}
	}
	_, err = store.queries.CreateUsageEvent(ctx, dbgen.CreateUsageEventParams{
		ID: id, WorkspaceID: workspaceID, EventType: event.EventType,
		IdempotencyKey: event.IdempotencyKey, DataVersion: event.DataVersion,
		ActorID: optionalTextValue(event.ActorID), AssetID: assetID, RevisionID: revisionID,
		Channel: event.Channel, Outcome: event.Outcome, ReasonCode: optionalTextValue(event.ReasonCode),
		TraceID: event.TraceID, SearchFingerprint: optionalTextValue(event.SearchFingerprint),
		SearchLanguage: optionalTextValue(event.SearchLanguage), TokenBucket: optionalTextValue(event.TokenBucket),
		ResultBucket: optionalTextValue(event.ResultBucket), AssetTypeFilter: optionalTextValue(string(event.AssetTypeFilter)),
		LifecycleFilter: optionalTextValue(event.LifecycleFilter), OccurredAt: timestamp(event.OccurredAt),
		ReceivedAt: timestamp(event.ReceivedAt), ExpiresAt: timestamp(event.ExpiresAt),
	})
	if err != nil {
		return repositoryError("record usage event", err)
	}
	return nil
}

func (store *Store) DeleteExpiredUsageEvents(ctx context.Context, expiredAt time.Time, limit int) (int64, error) {
	rows, err := store.queries.DeleteExpiredUsageEvents(ctx, dbgen.DeleteExpiredUsageEventsParams{
		ExpiredAt: timestamp(expiredAt), DeleteLimit: int32(limit),
	})
	if err != nil {
		return 0, repositoryError("delete expired usage events", err)
	}
	return rows, nil
}
