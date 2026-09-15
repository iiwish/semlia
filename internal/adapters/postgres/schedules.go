package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	discoveryapp "github.com/iiwish/semlia/internal/application/discovery"
	application "github.com/iiwish/semlia/internal/application/ingestion"
	domain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ application.ScheduleRepository = (*Store)(nil)

const scheduleColumns = `id,workspace_id,source_connection_id,expression,timezone,misfire_policy,enabled,
next_run_at,next_wall_clock_key,last_run_at,credential_version,created_by_principal_id,version,deleted_at,created_at,updated_at`

const occurrenceColumns = `id,workspace_id,schedule_id,source_connection_id,trigger_kind,schedule_version,
scheduled_for,eligible_at,wall_clock_key,state,reason_code,misfire_disposition,discovery_run_id,job_id,runtime_run_id,
artifact_set_id,credential_version,source_fingerprint,requested_by_principal_id,idempotency_key,created_at`

func (store *Store) FindScheduleByCreateIdempotency(ctx context.Context, workspace identity.WorkspaceID, key, fingerprint string, authVersion int64) (*domain.Schedule, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, scheduleRepositoryError("begin schedule replay", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockArtifactWorkspace(ctx, tx, workspace, authVersion); err != nil {
		return nil, err
	}
	workspaceID, _ := uuidValue(workspace)
	var storedFingerprint string
	var scheduleID pgtype.UUID
	err = tx.QueryRow(ctx, `SELECT id,create_request_fingerprint FROM source_schedules WHERE workspace_id=$1 AND create_idempotency_key=$2`, workspaceID, key).Scan(&scheduleID, &storedFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, scheduleRepositoryError("find schedule replay", err)
	}
	if storedFingerprint != fingerprint {
		return nil, domain.ErrConflict
	}
	value, err := scanSchedule(tx.QueryRow(ctx, `SELECT `+scheduleColumns+` FROM source_schedules WHERE workspace_id=$1 AND id=$2`, workspaceID, scheduleID))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, scheduleRepositoryError("commit schedule replay", err)
	}
	return &value, nil
}

func (store *Store) CreateSchedule(ctx context.Context, command application.CreateScheduleCommand) (domain.Schedule, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Schedule{}, scheduleRepositoryError("begin schedule create", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockArtifactWorkspace(ctx, tx, command.Schedule.WorkspaceID, command.ExpectedAuthorizationVersion); err != nil {
		return domain.Schedule{}, err
	}
	workspaceID, scheduleID, sourceID, creatorID, err := scheduleIDs(command.Schedule)
	if err != nil {
		return domain.Schedule{}, err
	}
	var existingID pgtype.UUID
	var existingFingerprint string
	err = tx.QueryRow(ctx, `SELECT id,create_request_fingerprint FROM source_schedules WHERE workspace_id=$1 AND create_idempotency_key=$2`, workspaceID, command.IdempotencyKey).Scan(&existingID, &existingFingerprint)
	if err == nil {
		if existingFingerprint != command.RequestFingerprint {
			return domain.Schedule{}, domain.ErrConflict
		}
		return scanSchedule(tx.QueryRow(ctx, `SELECT `+scheduleColumns+` FROM source_schedules WHERE workspace_id=$1 AND id=$2`, workspaceID, existingID))
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Schedule{}, scheduleRepositoryError("check schedule create replay", err)
	}
	row := tx.QueryRow(ctx, `INSERT INTO source_schedules
(id,workspace_id,source_connection_id,expression,timezone,misfire_policy,enabled,next_run_at,next_wall_clock_key,
 credential_version,created_by_principal_id,create_idempotency_key,create_request_fingerprint,version,created_at,updated_at)
SELECT $1,$2,$3,$4,$5,$6,true,$7,$8,
 CASE WHEN s.source_kind='postgresql' THEN s.active_credential_version ELSE NULL END,$9,$10,$11,1,$12,$12
FROM source_connections s WHERE s.workspace_id=$2 AND s.id=$3 AND s.status<>'deleted'
RETURNING `+scheduleColumns, scheduleID, workspaceID, sourceID, command.Schedule.Expression, command.Schedule.Timezone,
		command.Schedule.MisfirePolicy, timestamp(*command.Schedule.NextRunAt), command.Schedule.NextWallClockKey, creatorID,
		command.IdempotencyKey, command.RequestFingerprint, timestamp(command.Schedule.CreatedAt))
	created, err := scanSchedule(row)
	if err != nil {
		return domain.Schedule{}, scheduleRepositoryError("create schedule", err)
	}
	if err := insertSourceAudit(ctx, tx, command.Schedule.WorkspaceID, "schedule.created", command.Schedule.CreatedByPrincipalID.String(), command.TraceID,
		map[string]any{"scheduleId": command.Schedule.ID.String(), "sourceId": command.Schedule.SourceConnectionID.String(), "expression": command.Schedule.Expression, "timezone": command.Schedule.Timezone}); err != nil {
		return domain.Schedule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Schedule{}, scheduleRepositoryError("commit schedule create", err)
	}
	return created, nil
}

func (store *Store) LookupScheduleSource(ctx context.Context, workspace identity.WorkspaceID, schedule identity.SourceScheduleID) (identity.SourceConnectionID, error) {
	workspaceID, scheduleID, err := scheduleLookupIDs(workspace, schedule)
	if err != nil {
		return identity.SourceConnectionID{}, err
	}
	var sourceID pgtype.UUID
	if err := store.pool.QueryRow(ctx, `SELECT source_connection_id FROM source_schedules WHERE workspace_id=$1 AND id=$2`, workspaceID, scheduleID).Scan(&sourceID); err != nil {
		return identity.SourceConnectionID{}, scheduleRepositoryError("lookup schedule source", err)
	}
	return identity.SourceConnectionIDFromUUIDBytes(sourceID.Bytes)
}

func (store *Store) GetSchedule(ctx context.Context, workspace identity.WorkspaceID, schedule identity.SourceScheduleID, authVersion int64) (domain.Schedule, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.Schedule{}, scheduleRepositoryError("begin schedule read", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := verifyArtifactAuthorizationVersion(ctx, tx, workspace, authVersion); err != nil {
		return domain.Schedule{}, err
	}
	workspaceID, scheduleID, err := scheduleLookupIDs(workspace, schedule)
	if err != nil {
		return domain.Schedule{}, err
	}
	value, err := scanSchedule(tx.QueryRow(ctx, `SELECT `+scheduleColumns+` FROM source_schedules WHERE workspace_id=$1 AND id=$2`, workspaceID, scheduleID))
	if err != nil {
		return domain.Schedule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Schedule{}, scheduleRepositoryError("commit schedule read", err)
	}
	return value, nil
}

func (store *Store) ListSchedules(ctx context.Context, query application.ListSchedulesQuery) (domain.Page[domain.Schedule], error) {
	cursor, err := decodeScheduleCursor(query)
	if err != nil {
		return domain.Page[domain.Schedule]{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.Page[domain.Schedule]{}, scheduleRepositoryError("begin schedule list", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := verifyArtifactAuthorizationVersion(ctx, tx, query.WorkspaceID, query.AuthorizationVersion); err != nil {
		return domain.Page[domain.Schedule]{}, err
	}
	workspaceID, _ := uuidValue(query.WorkspaceID)
	var sourceID pgtype.UUID
	if query.SourceID != nil {
		sourceID, err = uuidValue(*query.SourceID)
		if err != nil {
			return domain.Page[domain.Schedule]{}, err
		}
	}
	filters := `workspace_id=$1 AND deleted_at IS NULL AND ($2::uuid IS NULL OR source_connection_id=$2) AND ($3::boolean IS NULL OR enabled=$3)`
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM source_schedules WHERE `+filters, workspaceID, sourceID, query.Enabled).Scan(&total); err != nil {
		return domain.Page[domain.Schedule]{}, scheduleRepositoryError("count schedules", err)
	}
	rows, err := tx.Query(ctx, `SELECT `+scheduleColumns+` FROM source_schedules WHERE `+filters+`
AND ($4::timestamptz IS NULL OR (updated_at,id)<($4,$5)) ORDER BY updated_at DESC,id DESC LIMIT $6`, workspaceID, sourceID, query.Enabled, cursor.UpdatedAt, cursor.ID, query.Limit+1)
	if err != nil {
		return domain.Page[domain.Schedule]{}, scheduleRepositoryError("list schedules", err)
	}
	defer rows.Close()
	items := make([]domain.Schedule, 0, query.Limit+1)
	for rows.Next() {
		value, scanErr := scanSchedule(rows)
		if scanErr != nil {
			return domain.Page[domain.Schedule]{}, scanErr
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		return domain.Page[domain.Schedule]{}, scheduleRepositoryError("iterate schedules", err)
	}
	next := ""
	if len(items) > query.Limit {
		items = items[:query.Limit]
		next = encodeScheduleCursor(query, items[len(items)-1])
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Page[domain.Schedule]{}, scheduleRepositoryError("commit schedule list", err)
	}
	return domain.Page[domain.Schedule]{Items: items, Total: total, Limit: query.Limit, NextCursor: next}, nil
}

func (store *Store) MutateSchedule(ctx context.Context, command application.MutateScheduleCommand) (domain.Schedule, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Schedule{}, scheduleRepositoryError("begin schedule mutation", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockArtifactWorkspace(ctx, tx, command.Schedule.WorkspaceID, command.ExpectedAuthorizationVersion); err != nil {
		return domain.Schedule{}, err
	}
	workspaceID, scheduleID, _, _, err := scheduleIDs(command.Schedule)
	if err != nil {
		return domain.Schedule{}, err
	}
	receiptKey := scheduleCommandIdempotencyKey(command.Action, command.Schedule.ID, command.IdempotencyKey)
	if replay, replayErr := scheduleReceipt(ctx, tx, workspaceID, receiptKey, command.Action, command.RequestFingerprint, scheduleID); replayErr != nil || replay != nil {
		if replayErr != nil {
			return domain.Schedule{}, replayErr
		}
		return *replay, nil
	}
	deletedAt := any(nil)
	if command.Action == "delete" {
		deletedAt = timestamp(command.Schedule.UpdatedAt)
	}
	row := tx.QueryRow(ctx, `UPDATE source_schedules SET expression=$3,timezone=$4,misfire_policy=$5,enabled=$6,
next_run_at=$7,next_wall_clock_key=$8,lease_owner=NULL,leased_until=NULL,deleted_at=$9,version=version+1,updated_at=$10,
credential_version=CASE WHEN $12='update' THEN (
 SELECT CASE WHEN source_kind='postgresql' THEN active_credential_version ELSE NULL END
 FROM source_connections WHERE workspace_id=$1 AND id=source_schedules.source_connection_id
) ELSE credential_version END
WHERE workspace_id=$1 AND id=$2 AND version=$11 AND deleted_at IS NULL RETURNING `+scheduleColumns,
		workspaceID, scheduleID, command.Schedule.Expression, command.Schedule.Timezone, command.Schedule.MisfirePolicy,
		command.Schedule.Enabled, optionalTimestamp(command.Schedule.NextRunAt), optionalTextValue(command.Schedule.NextWallClockKey),
		deletedAt, timestamp(command.Schedule.UpdatedAt), command.ExpectedVersion, command.Action)
	value, err := scanSchedule(row)
	if err != nil {
		return domain.Schedule{}, scheduleRepositoryError("mutate exact schedule", err)
	}
	resultSchedule, _ := json.Marshal(value)
	if _, err := tx.Exec(ctx, `INSERT INTO source_schedule_command_receipts
(workspace_id,schedule_id,command,idempotency_key,request_fingerprint,result_schedule_version,result_schedule,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, workspaceID, scheduleID, command.Action, receiptKey, command.RequestFingerprint,
		value.Version, resultSchedule, timestamp(command.Schedule.UpdatedAt)); err != nil {
		return domain.Schedule{}, scheduleRepositoryError("record schedule mutation receipt", err)
	}
	if err := insertSourceAudit(ctx, tx, command.Schedule.WorkspaceID, "schedule."+command.Action,
		command.ActorPrincipalID.String(), command.TraceID, map[string]any{
			"scheduleId": command.Schedule.ID.String(), "sourceId": command.Schedule.SourceConnectionID.String(),
			"action": command.Action, "resultVersion": value.Version,
		}); err != nil {
		return domain.Schedule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Schedule{}, scheduleRepositoryError("commit schedule mutation", err)
	}
	return value, nil
}

func (store *Store) RunScheduleNow(ctx context.Context, command application.RunScheduleNowCommand) (domain.Occurrence, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Occurrence{}, scheduleRepositoryError("begin schedule run now", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockArtifactWorkspace(ctx, tx, command.WorkspaceID, command.ExpectedAuthorizationVersion); err != nil {
		return domain.Occurrence{}, err
	}
	workspaceID, scheduleID, err := scheduleLookupIDs(command.WorkspaceID, command.ScheduleID)
	if err != nil {
		return domain.Occurrence{}, err
	}
	receiptKey := scheduleCommandIdempotencyKey("run_now", command.ScheduleID, command.IdempotencyKey)
	if occurrenceID, replay, replayErr := occurrenceReceipt(ctx, tx, workspaceID, receiptKey, "run_now", command.RequestFingerprint, scheduleID); replayErr != nil || replay {
		if replayErr != nil {
			return domain.Occurrence{}, replayErr
		}
		return scanOccurrence(tx.QueryRow(ctx, `SELECT `+occurrenceColumns+` FROM source_schedule_occurrences WHERE workspace_id=$1 AND id=$2`, workspaceID, occurrenceID))
	}
	schedule, err := scanSchedule(tx.QueryRow(ctx, `SELECT `+scheduleColumns+` FROM source_schedules WHERE workspace_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, workspaceID, scheduleID))
	if err != nil {
		return domain.Occurrence{}, err
	}
	if schedule.Version != command.ExpectedVersion {
		return domain.Occurrence{}, domain.ErrConflict
	}
	run, err := store.createQueuedDiscoveryRunTx(ctx, tx, discoveryapp.CreateRunCommand{RunID: command.RunID, JobID: command.JobID,
		WorkspaceID: command.WorkspaceID, SourceID: schedule.SourceConnectionID, RequestedBy: command.RequestedBy.String(),
		IdempotencyKey: command.IdempotencyKey, TraceID: command.TraceID, CreatedAt: command.CreatedAt},
		enqueueIntent{kind: "run_now", identity: command.ScheduleID.String(), skipAuthorization: true, credentialVersion: schedule.CredentialVersion,
			serverIdempotencyKey: boundedRuntimeIdempotencyKey("schedule-run-now:"+command.ScheduleID.String()+":", command.IdempotencyKey)})
	if err != nil && !errors.Is(err, errActiveDiscoveryRun) && !errors.Is(err, errCredentialUnavailable) &&
		!errors.Is(err, errArtifactUnavailable) && !errors.Is(err, errSourceUnavailable) {
		return domain.Occurrence{}, err
	}
	local := command.CreatedAt.In(mustLocation(schedule.Timezone))
	occurrence := domain.Occurrence{ID: command.OccurrenceID, WorkspaceID: command.WorkspaceID, ScheduleID: command.ScheduleID,
		SourceConnectionID: schedule.SourceConnectionID, TriggerKind: "run_now", ScheduleVersion: schedule.Version,
		ScheduledFor: &command.CreatedAt, EligibleAt: command.CreatedAt, WallClockKey: local.Format("2006-01-02T15:04"), State: "enqueued",
		RequestedBy: command.RequestedBy, IdempotencyKey: runNowOccurrenceIdempotencyKey(command.ScheduleID, command.IdempotencyKey), CreatedAt: command.CreatedAt}
	switch {
	case errors.Is(err, errActiveDiscoveryRun):
		occurrence.State, occurrence.ReasonCode = "skipped", "OVERLAP_ACTIVE_RUN"
	case errors.Is(err, errCredentialUnavailable):
		occurrence.State, occurrence.ReasonCode = "skipped", "CREDENTIAL_UNAVAILABLE"
	case errors.Is(err, errArtifactUnavailable):
		occurrence.State, occurrence.ReasonCode = "skipped", "ARTIFACT_SET_UNAVAILABLE"
	case errors.Is(err, errSourceUnavailable):
		occurrence.State, occurrence.ReasonCode = "skipped", "SOURCE_UNAVAILABLE"
	default:
		occurrence.DiscoveryRunID, occurrence.JobID, occurrence.RuntimeRunID = &run.ID, &run.JobID, &run.ID
		occurrence.ArtifactSetID, occurrence.CredentialVersion, occurrence.SourceFingerprint = run.ArtifactSetID, optionalCredential(run.CredentialVersion), run.SourceInputFingerprint
	}
	if err := insertOccurrence(ctx, tx, occurrence); err != nil {
		return domain.Occurrence{}, err
	}
	if err := insertSourceAudit(ctx, tx, occurrence.WorkspaceID, "schedule.occurrence", occurrence.RequestedBy.String(),
		command.TraceID, map[string]any{"scheduleId": occurrence.ScheduleID.String(), "occurrenceId": occurrence.ID.String(),
			"trigger": occurrence.TriggerKind, "state": occurrence.State, "reasonCode": occurrence.ReasonCode}); err != nil {
		return domain.Occurrence{}, err
	}
	occurrenceID, _ := uuidValue(occurrence.ID)
	resultSchedule, _ := json.Marshal(schedule)
	if _, err := tx.Exec(ctx, `INSERT INTO source_schedule_command_receipts
(workspace_id,schedule_id,command,idempotency_key,request_fingerprint,result_schedule_version,result_schedule,occurrence_id,created_at)
VALUES($1,$2,'run_now',$3,$4,$5,$6,$7,$8)`, workspaceID, scheduleID, receiptKey, command.RequestFingerprint,
		schedule.Version, resultSchedule, occurrenceID, timestamp(command.CreatedAt)); err != nil {
		return domain.Occurrence{}, scheduleRepositoryError("record run-now receipt", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Occurrence{}, scheduleRepositoryError("commit run now", err)
	}
	return occurrence, nil
}

func (store *Store) ClaimDueSchedules(ctx context.Context, owner string, lease time.Duration, limit int) ([]domain.Schedule, time.Time, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, time.Time{}, scheduleRepositoryError("begin due schedule claim", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var databaseNow time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&databaseNow); err != nil {
		return nil, time.Time{}, scheduleRepositoryError("read scheduler database time", err)
	}
	rows, err := tx.Query(ctx, `WITH due AS (
SELECT id FROM source_schedules WHERE enabled AND deleted_at IS NULL AND next_run_at<=$1
AND (leased_until IS NULL OR leased_until<=$1) ORDER BY next_run_at,id FOR UPDATE SKIP LOCKED LIMIT $2
)
UPDATE source_schedules s SET lease_owner=$3,leased_until=$4 FROM due
WHERE s.id=due.id RETURNING s.id,s.workspace_id,s.source_connection_id,s.expression,s.timezone,s.misfire_policy,s.enabled,
s.next_run_at,s.next_wall_clock_key,s.last_run_at,s.credential_version,s.created_by_principal_id,s.version,s.deleted_at,s.created_at,s.updated_at`,
		timestamp(databaseNow), limit, owner, timestamp(databaseNow.Add(lease)))
	if err != nil {
		return nil, time.Time{}, scheduleRepositoryError("claim due schedules", err)
	}
	result := make([]domain.Schedule, 0, limit)
	for rows.Next() {
		value, scanErr := scanSchedule(rows)
		if scanErr != nil {
			rows.Close()
			return nil, time.Time{}, scanErr
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, time.Time{}, err
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, time.Time{}, scheduleRepositoryError("commit due schedule claim", err)
	}
	return result, databaseNow.UTC(), nil
}

func (store *Store) CompleteDueSchedule(ctx context.Context, command application.CompleteDueScheduleCommand) (result domain.Occurrence, resultErr error) {
	defer func() { resultErr = scheduleRepositoryError("complete due schedule", resultErr) }()
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Occurrence{}, scheduleRepositoryError("begin due schedule", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Scheduled delegation uses the same workspace-before-schedule lock order as user commands.
	if err := lockArtifactWorkspace(ctx, tx, command.Schedule.WorkspaceID, 0); err != nil {
		return domain.Occurrence{}, err
	}
	workspaceID, scheduleID, err := scheduleLookupIDs(command.Schedule.WorkspaceID, command.Schedule.ID)
	if err != nil {
		return domain.Occurrence{}, err
	}
	var version int64
	if err := tx.QueryRow(ctx, `SELECT version FROM source_schedules WHERE workspace_id=$1 AND id=$2 AND enabled
AND deleted_at IS NULL AND lease_owner=$3 AND leased_until>clock_timestamp() AND next_wall_clock_key=$4 FOR UPDATE`,
		workspaceID, scheduleID, command.LeaseOwner, command.Occurrence.WallClockKey).Scan(&version); err != nil || version != command.Schedule.Version {
		if err == nil {
			err = domain.ErrConflict
		}
		return domain.Occurrence{}, scheduleRepositoryError("fence due schedule", err)
	}
	occurrence := command.Occurrence
	if existing, err := scanOccurrence(tx.QueryRow(ctx, `SELECT `+occurrenceColumns+` FROM source_schedule_occurrences WHERE workspace_id=$1 AND schedule_id=$2 AND schedule_version=$3 AND trigger_kind='scheduled' AND wall_clock_key=$4`, workspaceID, scheduleID, occurrence.ScheduleVersion, occurrence.WallClockKey)); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Occurrence{}, err
	}
	if occurrence.State == "enqueued" {
		run, enqueueErr := store.createQueuedDiscoveryRunTx(ctx, tx, discoveryapp.CreateRunCommand{RunID: command.RunID, JobID: command.JobID,
			WorkspaceID: command.Schedule.WorkspaceID, SourceID: command.Schedule.SourceConnectionID,
			RequestedBy: "system", IdempotencyKey: occurrence.IdempotencyKey,
			TraceID: command.TraceID, CreatedAt: command.Now}, enqueueIntent{kind: "scheduled", identity: command.Schedule.ID.String(),
			credentialVersion: command.Schedule.CredentialVersion,
			skipAuthorization: true, serverIdempotencyKey: boundedRuntimeIdempotencyKey("schedule:", occurrence.IdempotencyKey)})
		switch {
		case errors.Is(enqueueErr, errActiveDiscoveryRun):
			occurrence.State, occurrence.ReasonCode, occurrence.MisfireDisposition = "skipped", "OVERLAP_ACTIVE_RUN", ""
		case errors.Is(enqueueErr, errCredentialUnavailable):
			occurrence.State, occurrence.ReasonCode, occurrence.MisfireDisposition = "skipped", "CREDENTIAL_UNAVAILABLE", ""
		case errors.Is(enqueueErr, errArtifactUnavailable):
			occurrence.State, occurrence.ReasonCode, occurrence.MisfireDisposition = "skipped", "ARTIFACT_SET_UNAVAILABLE", ""
		case errors.Is(enqueueErr, errSourceUnavailable):
			occurrence.State, occurrence.ReasonCode, occurrence.MisfireDisposition = "skipped", "SOURCE_UNAVAILABLE", ""
		case enqueueErr != nil:
			return domain.Occurrence{}, enqueueErr
		default:
			occurrence.DiscoveryRunID, occurrence.JobID, occurrence.RuntimeRunID = &run.ID, &run.JobID, &run.ID
			occurrence.ArtifactSetID, occurrence.CredentialVersion, occurrence.SourceFingerprint = run.ArtifactSetID, optionalCredential(run.CredentialVersion), run.SourceInputFingerprint
		}
	}
	if err := insertOccurrence(ctx, tx, occurrence); err != nil {
		return domain.Occurrence{}, err
	}
	if err := insertSourceAudit(ctx, tx, occurrence.WorkspaceID, "schedule.occurrence", "system",
		command.TraceID, map[string]any{"scheduleId": occurrence.ScheduleID.String(), "occurrenceId": occurrence.ID.String(),
			"trigger": occurrence.TriggerKind, "state": occurrence.State, "reasonCode": occurrence.ReasonCode}); err != nil {
		return domain.Occurrence{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_schedules SET next_run_at=$3,next_wall_clock_key=$4,last_run_at=$5,
lease_owner=NULL,leased_until=NULL,version=version+1,updated_at=$5 WHERE workspace_id=$1 AND id=$2`, workspaceID, scheduleID,
		timestamp(command.Next.EligibleAt), command.Next.WallClockKey, timestamp(command.Now)); err != nil {
		return domain.Occurrence{}, scheduleRepositoryError("advance due schedule", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Occurrence{}, scheduleRepositoryError("commit due schedule", err)
	}
	return occurrence, nil
}

func (store *Store) ListScheduleOccurrences(ctx context.Context, query application.ListOccurrencesQuery) (domain.Page[domain.Occurrence], error) {
	cursor, err := decodeOccurrenceCursor(query)
	if err != nil {
		return domain.Page[domain.Occurrence]{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.Page[domain.Occurrence]{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := verifyArtifactAuthorizationVersion(ctx, tx, query.WorkspaceID, query.AuthorizationVersion); err != nil {
		return domain.Page[domain.Occurrence]{}, err
	}
	workspaceID, scheduleID, err := scheduleLookupIDs(query.WorkspaceID, query.ScheduleID)
	if err != nil {
		return domain.Page[domain.Occurrence]{}, err
	}
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM source_schedule_occurrences WHERE workspace_id=$1 AND schedule_id=$2`, workspaceID, scheduleID).Scan(&total); err != nil {
		return domain.Page[domain.Occurrence]{}, err
	}
	rows, err := tx.Query(ctx, `SELECT `+occurrenceColumns+` FROM source_schedule_occurrences WHERE workspace_id=$1 AND schedule_id=$2
AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4)) ORDER BY created_at DESC,id DESC LIMIT $5`, workspaceID, scheduleID, cursor.CreatedAt, cursor.ID, query.Limit+1)
	if err != nil {
		return domain.Page[domain.Occurrence]{}, err
	}
	defer rows.Close()
	items := make([]domain.Occurrence, 0, query.Limit+1)
	for rows.Next() {
		value, scanErr := scanOccurrence(rows)
		if scanErr != nil {
			return domain.Page[domain.Occurrence]{}, scanErr
		}
		items = append(items, value)
	}
	next := ""
	if len(items) > query.Limit {
		items = items[:query.Limit]
		next = encodeOccurrenceCursor(query, items[len(items)-1])
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Page[domain.Occurrence]{}, err
	}
	return domain.Page[domain.Occurrence]{Items: items, Total: total, Limit: query.Limit, NextCursor: next}, nil
}

func insertOccurrence(ctx context.Context, tx pgx.Tx, value domain.Occurrence) error {
	workspaceID, scheduleID, _ := scheduleLookupIDs(value.WorkspaceID, value.ScheduleID)
	id, _ := uuidValue(value.ID)
	sourceID, _ := uuidValue(value.SourceConnectionID)
	requestedBy, _ := uuidValue(value.RequestedBy)
	_, err := tx.Exec(ctx, `INSERT INTO source_schedule_occurrences
(id,workspace_id,schedule_id,source_connection_id,trigger_kind,schedule_version,scheduled_for,eligible_at,wall_clock_key,
 state,reason_code,misfire_disposition,discovery_run_id,job_id,runtime_run_id,artifact_set_id,credential_version,
 source_fingerprint,requested_by_principal_id,idempotency_key,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`,
		id, workspaceID, scheduleID, sourceID, value.TriggerKind, value.ScheduleVersion, optionalTimestamp(value.ScheduledFor), timestamp(value.EligibleAt), value.WallClockKey,
		value.State, optionalTextValue(value.ReasonCode), optionalTextValue(value.MisfireDisposition), optionalUUID(value.DiscoveryRunID), optionalUUID(value.JobID), optionalUUID(value.RuntimeRunID),
		optionalArtifactSetUUID(value.ArtifactSetID), value.CredentialVersion, optionalTextValue(value.SourceFingerprint), requestedBy, value.IdempotencyKey, timestamp(value.CreatedAt))
	if err != nil {
		return scheduleRepositoryError("insert schedule occurrence", err)
	}
	return nil
}

func scanSchedule(row rowScanner) (domain.Schedule, error) {
	var id, workspace, source, creator pgtype.UUID
	var next, last, deleted pgtype.Timestamptz
	var wall pgtype.Text
	var credential pgtype.Int8
	var value domain.Schedule
	if err := row.Scan(&id, &workspace, &source, &value.Expression, &value.Timezone, &value.MisfirePolicy, &value.Enabled,
		&next, &wall, &last, &credential, &creator, &value.Version, &deleted, &value.CreatedAt, &value.UpdatedAt); err != nil {
		return domain.Schedule{}, scheduleRepositoryError("scan schedule", err)
	}
	value.ID, _ = identity.SourceScheduleIDFromUUIDBytes(id.Bytes)
	value.WorkspaceID, _ = identity.WorkspaceIDFromUUIDBytes(workspace.Bytes)
	value.SourceConnectionID, _ = identity.SourceConnectionIDFromUUIDBytes(source.Bytes)
	created, _ := identity.PrincipalIDFromUUIDBytes(creator.Bytes)
	value.CreatedByPrincipalID = &created
	value.NextRunAt, value.LastRunAt, value.DeletedAt = optionalTime(next), optionalTime(last), optionalTime(deleted)
	value.NextWallClockKey = wall.String
	if credential.Valid {
		v := credential.Int64
		value.CredentialVersion = &v
	}
	return value, nil
}

func scanOccurrence(row rowScanner) (domain.Occurrence, error) {
	var id, workspace, schedule, source, requestedBy pgtype.UUID
	var scheduled pgtype.Timestamptz
	var reason, misfire, sourceFingerprint pgtype.Text
	var run, job, runtime, artifactSet pgtype.UUID
	var credential pgtype.Int8
	var value domain.Occurrence
	if err := row.Scan(&id, &workspace, &schedule, &source, &value.TriggerKind, &value.ScheduleVersion, &scheduled,
		&value.EligibleAt, &value.WallClockKey, &value.State, &reason, &misfire, &run, &job, &runtime, &artifactSet, &credential,
		&sourceFingerprint, &requestedBy, &value.IdempotencyKey, &value.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Occurrence{}, domain.ErrNotFound
		}
		return domain.Occurrence{}, scheduleRepositoryError("scan occurrence", err)
	}
	value.ID, _ = identity.ScheduleOccurrenceIDFromUUIDBytes(id.Bytes)
	value.WorkspaceID, _ = identity.WorkspaceIDFromUUIDBytes(workspace.Bytes)
	value.ScheduleID, _ = identity.SourceScheduleIDFromUUIDBytes(schedule.Bytes)
	value.SourceConnectionID, _ = identity.SourceConnectionIDFromUUIDBytes(source.Bytes)
	value.RequestedBy, _ = identity.PrincipalIDFromUUIDBytes(requestedBy.Bytes)
	value.ScheduledFor, value.ReasonCode, value.MisfireDisposition = optionalTime(scheduled), reason.String, misfire.String
	value.SourceFingerprint = sourceFingerprint.String
	value.DiscoveryRunID, value.JobID, value.RuntimeRunID = runIDPointer(run), runIDPointer(job), runIDPointer(runtime)
	if artifactSet.Valid {
		v, _ := identity.ArtifactSetIDFromUUIDBytes(artifactSet.Bytes)
		value.ArtifactSetID = &v
	}
	if credential.Valid {
		v := credential.Int64
		value.CredentialVersion = &v
	}
	return value, nil
}

func scheduleReceipt(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, key, action, fingerprint string, scheduleID pgtype.UUID) (*domain.Schedule, error) {
	var storedSchedule pgtype.UUID
	var storedAction, storedFingerprint string
	var result []byte
	err := tx.QueryRow(ctx, `SELECT schedule_id,command,request_fingerprint,result_schedule FROM source_schedule_command_receipts WHERE workspace_id=$1 AND idempotency_key=$2`, workspaceID, key).Scan(&storedSchedule, &storedAction, &storedFingerprint, &result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, scheduleRepositoryError("read schedule receipt", err)
	}
	if storedSchedule != scheduleID || storedAction != action || storedFingerprint != fingerprint {
		return nil, domain.ErrConflict
	}
	var schedule domain.Schedule
	if json.Unmarshal(result, &schedule) != nil || schedule.ID.IsZero() {
		return nil, scheduleRepositoryError("decode schedule receipt", domain.ErrConflict)
	}
	return &schedule, nil
}

func occurrenceReceipt(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, key, action, fingerprint string, scheduleID pgtype.UUID) (pgtype.UUID, bool, error) {
	var occurrenceID, storedSchedule pgtype.UUID
	var storedAction, storedFingerprint string
	err := tx.QueryRow(ctx, `SELECT occurrence_id,schedule_id,command,request_fingerprint FROM source_schedule_command_receipts WHERE workspace_id=$1 AND idempotency_key=$2`, workspaceID, key).Scan(&occurrenceID, &storedSchedule, &storedAction, &storedFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, false, nil
	}
	if err != nil {
		return pgtype.UUID{}, false, scheduleRepositoryError("read occurrence receipt", err)
	}
	if !occurrenceID.Valid || storedSchedule != scheduleID || storedAction != action || storedFingerprint != fingerprint {
		return pgtype.UUID{}, false, domain.ErrConflict
	}
	return occurrenceID, true, nil
}

func scheduleCommandIdempotencyKey(action string, scheduleID identity.SourceScheduleID, clientKey string) string {
	return boundedRuntimeIdempotencyKey("schedule-command:"+action+":"+scheduleID.String()+":", clientKey)
}

func runNowOccurrenceIdempotencyKey(scheduleID identity.SourceScheduleID, clientKey string) string {
	return boundedRuntimeIdempotencyKey("occurrence:run-now:"+scheduleID.String()+":", clientKey)
}

type scheduleCursor struct {
	Workspace string       `json:"w"`
	Principal string       `json:"p"`
	Auth      int64        `json:"a"`
	Source    string       `json:"s"`
	Enabled   *bool        `json:"e"`
	UpdatedAt *time.Time   `json:"t"`
	ID        *pgtype.UUID `json:"i"`
}

func decodeScheduleCursor(query application.ListSchedulesQuery) (scheduleCursor, error) {
	source := ""
	if query.SourceID != nil {
		source = query.SourceID.String()
	}
	base := scheduleCursor{Workspace: query.WorkspaceID.String(), Principal: query.PrincipalID.String(), Auth: query.AuthorizationVersion, Source: source, Enabled: query.Enabled}
	if query.Cursor == "" {
		return base, nil
	}
	if len(query.Cursor) > 4096 {
		return scheduleCursor{}, domain.ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(query.Cursor)
	if err != nil {
		return scheduleCursor{}, domain.ErrInvalid
	}
	var value scheduleCursor
	if json.Unmarshal(raw, &value) != nil || value.Workspace != base.Workspace || value.Principal != base.Principal || value.Auth != base.Auth || value.Source != base.Source || !equalBool(value.Enabled, base.Enabled) || value.UpdatedAt == nil || value.ID == nil || !value.ID.Valid {
		return scheduleCursor{}, domain.ErrInvalid
	}
	return value, nil
}
func encodeScheduleCursor(query application.ListSchedulesQuery, value domain.Schedule) string {
	id, _ := uuidValue(value.ID)
	updated := value.UpdatedAt.UTC()
	source := ""
	if query.SourceID != nil {
		source = query.SourceID.String()
	}
	raw, _ := json.Marshal(scheduleCursor{Workspace: query.WorkspaceID.String(), Principal: query.PrincipalID.String(), Auth: query.AuthorizationVersion, Source: source, Enabled: query.Enabled, UpdatedAt: &updated, ID: &id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

type occurrenceCursor struct {
	Workspace, Principal, Schedule string
	Auth                           int64
	CreatedAt                      *time.Time
	ID                             *pgtype.UUID
}

func decodeOccurrenceCursor(query application.ListOccurrencesQuery) (occurrenceCursor, error) {
	base := occurrenceCursor{Workspace: query.WorkspaceID.String(), Principal: query.PrincipalID.String(), Schedule: query.ScheduleID.String(), Auth: query.AuthorizationVersion}
	if query.Cursor == "" {
		return base, nil
	}
	if len(query.Cursor) > 4096 {
		return occurrenceCursor{}, domain.ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(query.Cursor)
	if err != nil {
		return occurrenceCursor{}, domain.ErrInvalid
	}
	var value occurrenceCursor
	if json.Unmarshal(raw, &value) != nil || value.Workspace != base.Workspace || value.Principal != base.Principal || value.Schedule != base.Schedule || value.Auth != base.Auth || value.CreatedAt == nil || value.ID == nil || !value.ID.Valid {
		return occurrenceCursor{}, domain.ErrInvalid
	}
	return value, nil
}
func encodeOccurrenceCursor(query application.ListOccurrencesQuery, value domain.Occurrence) string {
	id, _ := uuidValue(value.ID)
	created := value.CreatedAt.UTC()
	raw, _ := json.Marshal(occurrenceCursor{Workspace: query.WorkspaceID.String(), Principal: query.PrincipalID.String(), Schedule: query.ScheduleID.String(), Auth: query.AuthorizationVersion, CreatedAt: &created, ID: &id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func scheduleIDs(value domain.Schedule) (pgtype.UUID, pgtype.UUID, pgtype.UUID, pgtype.UUID, error) {
	w, e := uuidValue(value.WorkspaceID)
	if e != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, e
	}
	i, e := uuidValue(value.ID)
	if e != nil {
		return w, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, e
	}
	s, e := uuidValue(value.SourceConnectionID)
	if e != nil {
		return w, i, pgtype.UUID{}, pgtype.UUID{}, e
	}
	if value.CreatedByPrincipalID == nil {
		return w, i, s, pgtype.UUID{}, domain.ErrInvalid
	}
	p, e := uuidValue(*value.CreatedByPrincipalID)
	return w, i, s, p, e
}
func scheduleLookupIDs(workspace identity.WorkspaceID, schedule identity.SourceScheduleID) (pgtype.UUID, pgtype.UUID, error) {
	w, e := uuidValue(workspace)
	if e != nil {
		return pgtype.UUID{}, pgtype.UUID{}, e
	}
	i, e := uuidValue(schedule)
	return w, i, e
}
func optionalUUID(value *identity.RunID) any {
	if value == nil {
		return nil
	}
	v, _ := uuidValue(*value)
	return v
}
func optionalArtifactSetUUID(value *identity.ArtifactSetID) any {
	if value == nil {
		return nil
	}
	v, _ := uuidValue(*value)
	return v
}
func optionalCredential(value int64) *int64 {
	if value < 1 {
		return nil
	}
	return &value
}
func runIDPointer(value pgtype.UUID) *identity.RunID {
	if !value.Valid {
		return nil
	}
	v, _ := identity.RunIDFromUUIDBytes(value.Bytes)
	return &v
}
func uuidMust(value identity.SourceConnectionID) pgtype.UUID { v, _ := uuidValue(value); return v }
func equalBool(left, right *bool) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
func mustLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return location
}
func scheduleRepositoryError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrInvalid) || errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		switch pgError.Code {
		case "40001", "40P01", "55P03", "08006", "08003", "57P01", "57P02", "57P03":
			return errors.Join(domain.ErrRetryable, fmt.Errorf("%s: %w", operation, err))
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
