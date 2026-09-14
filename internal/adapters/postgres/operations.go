package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	operationsapp "github.com/iiwish/semlia/internal/application/operations"
	domain "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ operationsapp.Repository = (*Store)(nil)
var _ operationsapp.ProjectionWriter = (*Store)(nil)

func (store *Store) ListAuditEvents(ctx context.Context, query operationsapp.AuditQuery) ([]operationsapp.StoredAuditEvent, error) {
	workspaceID, err := uuidValue(query.WorkspaceID)
	if err != nil {
		return nil, err
	}
	params := dbgen.ListOperationsAuditEventsParams{WorkspaceID: workspaceID,
		HasActor: query.ActorID != "", ActorID: optionalTextValue(query.ActorID),
		HasEventType: query.EventType != "", EventType: query.EventType,
		HasObjectType: query.ObjectType != "", ObjectType: query.ObjectType,
		HasObjectID: query.ObjectID != "", ObjectID: query.ObjectID,
		HasTrace: query.TraceID != "", TraceID: query.TraceID,
		HasFrom: query.From != nil, FromTime: optionalTimeValue(query.From),
		HasTo: query.To != nil, ToTime: optionalTimeValue(query.To), PageLimit: int32(query.Limit)}
	if query.After != nil {
		params.HasCursor, params.CursorTime = true, timestamp(query.After.Time)
		if err := params.CursorID.Scan(query.After.ID); err != nil {
			return nil, domain.ErrInvalidArgument
		}
	}
	rows, err := store.queries.ListOperationsAuditEvents(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list operations audit events: %w", err)
	}
	result := make([]operationsapp.StoredAuditEvent, 0, len(rows))
	for _, row := range rows {
		id, decodeErr := identity.EventIDFromUUIDBytes(row.ID.Bytes)
		if decodeErr != nil {
			return nil, decodeErr
		}
		workspace, decodeErr := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, operationsapp.StoredAuditEvent{ID: id, WorkspaceID: workspace,
			EventType: row.EventType, ActorID: optionalText(row.ActorID), Payload: cloneJSON(row.Payload),
			ObjectType: row.ObjectType, ObjectID: row.ObjectID, TraceID: row.TraceID, CreatedAt: row.CreatedAt.Time.UTC()})
	}
	return result, nil
}

func (store *Store) ListRuntimeRuns(ctx context.Context, query operationsapp.RuntimeQuery) ([]domain.RuntimeRun, error) {
	workspaceID, err := uuidValue(query.WorkspaceID)
	if err != nil {
		return nil, err
	}
	params := dbgen.ListOperationsRuntimeRunsParams{WorkspaceID: workspaceID,
		HasKind: query.Kind != "", Kind: string(query.Kind), HasState: query.State != "", State: string(query.State),
		HasSourceType: query.SourceType != "", SourceType: query.SourceType,
		HasSourceID: query.SourceID != "", SourceID: query.SourceID,
		HasTrace: query.TraceID != "", TraceID: optionalTextValue(query.TraceID), PageLimit: int32(query.Limit)}
	if query.After != nil {
		params.HasCursor, params.CursorTime = true, timestamp(query.After.Time)
		if err := params.CursorID.Scan(query.After.ID); err != nil {
			return nil, domain.ErrInvalidArgument
		}
	}
	rows, err := store.queries.ListOperationsRuntimeRuns(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list runtime runs: %w", err)
	}
	result := make([]domain.RuntimeRun, 0, len(rows))
	for _, row := range rows {
		item, decodeErr := runtimeRunFromRow(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, item)
	}
	return result, nil
}

func (store *Store) GetRuntimeRun(ctx context.Context, workspace identity.WorkspaceID, run identity.RunID) (domain.RuntimeRun, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return domain.RuntimeRun{}, err
	}
	runID, err := uuidValue(run)
	if err != nil {
		return domain.RuntimeRun{}, err
	}
	row, err := store.queries.GetOperationsRuntimeRun(ctx, dbgen.GetOperationsRuntimeRunParams{WorkspaceID: workspaceID, ID: runID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RuntimeRun{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.RuntimeRun{}, fmt.Errorf("get runtime run: %w", err)
	}
	return runtimeRunFromRow(row)
}

func (store *Store) ListRuntimeRunEvents(ctx context.Context, workspace identity.WorkspaceID, run identity.RunID, limit int) ([]domain.RuntimeRunEvent, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, err
	}
	runID, err := uuidValue(run)
	if err != nil {
		return nil, err
	}
	rows, err := store.queries.ListOperationsRuntimeRunEvents(ctx, dbgen.ListOperationsRuntimeRunEventsParams{
		WorkspaceID: workspaceID, RuntimeRunID: runID, PageLimit: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("list runtime run events: %w", err)
	}
	result := make([]domain.RuntimeRunEvent, 0, len(rows))
	for _, row := range rows {
		item, decodeErr := runtimeEventFromRow(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, item)
	}
	return result, nil
}

func (store *Store) GetRuntimeSettings(ctx context.Context, workspace identity.WorkspaceID, now time.Time) (domain.RuntimeSettings, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return domain.RuntimeSettings{}, err
	}
	row, err := store.queries.EnsureRuntimeSettings(ctx, dbgen.EnsureRuntimeSettingsParams{WorkspaceID: workspaceID, Now: timestamp(now)})
	if err != nil {
		return domain.RuntimeSettings{}, fmt.Errorf("ensure runtime settings: %w", err)
	}
	return runtimeSettingsFromRow(row)
}

func (store *Store) UpdateRuntimeSettings(ctx context.Context, settings domain.RuntimeSettings, expectedVersion int64) (domain.RuntimeSettings, error) {
	workspaceID, err := uuidValue(settings.WorkspaceID)
	if err != nil {
		return domain.RuntimeSettings{}, err
	}
	row, err := store.queries.UpdateRuntimeSettings(ctx, dbgen.UpdateRuntimeSettingsParams{
		WorkspaceID: workspaceID, ExpectedVersion: expectedVersion, RetryCeiling: settings.RetryCeiling,
		StatementTimeoutMs: settings.StatementTimeoutMS, WebhookTimeoutMs: settings.WebhookTimeoutMS,
		QueryRowLimit: settings.QueryRowLimit, QueryByteLimit: settings.QueryByteLimit,
		RunMetadataRetentionDays: settings.RunMetadataRetentionDays, UpdatedAt: timestamp(settings.UpdatedAt)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RuntimeSettings{}, domain.ErrConflict
	}
	if err != nil {
		return domain.RuntimeSettings{}, fmt.Errorf("update runtime settings: %w", err)
	}
	return runtimeSettingsFromRow(row)
}

func (store *Store) FindAuditExportReplay(ctx context.Context, request operationsapp.AuditExportReplayRequest) (operationsapp.AuditExport, bool, error) {
	workspaceID, err := uuidValue(request.WorkspaceID)
	if err != nil {
		return operationsapp.AuditExport{}, false, err
	}
	existing, err := store.queries.GetAuditExportByIdempotency(ctx, dbgen.GetAuditExportByIdempotencyParams{
		WorkspaceID: workspaceID, IdempotencyKey: request.IdempotencyKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return operationsapp.AuditExport{}, false, nil
	}
	if err != nil {
		return operationsapp.AuditExport{}, false, fmt.Errorf("get audit export replay: %w", err)
	}
	if err := validateAuditExportReplay(existing, request); err != nil {
		return operationsapp.AuditExport{}, false, err
	}
	replayed, err := auditExportFromRow(existing)
	return replayed, true, err
}

func (store *Store) CreateAuditExport(ctx context.Context, record operationsapp.AuditExportRecord) (operationsapp.AuditExport, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return operationsapp.AuditExport{}, fmt.Errorf("begin audit export: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	workspaceID, err := uuidValue(record.WorkspaceID)
	if err != nil {
		return operationsapp.AuditExport{}, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, record.WorkspaceID.String()+"\x1f"+record.IdempotencyKey); err != nil {
		return operationsapp.AuditExport{}, fmt.Errorf("lock audit export idempotency: %w", err)
	}
	if existing, getErr := queries.GetAuditExportByIdempotency(ctx, dbgen.GetAuditExportByIdempotencyParams{
		WorkspaceID: workspaceID, IdempotencyKey: record.IdempotencyKey}); getErr == nil {
		if replayErr := validateAuditExportReplay(existing, operationsapp.AuditExportReplayRequest{
			WorkspaceID: record.WorkspaceID, IdempotencyKey: record.IdempotencyKey,
			RequestFingerprint: record.RequestFingerprint, Filters: record.Filters,
			CreatedByPrincipalID: record.CreatedByPrincipalID, Now: record.CreatedAt,
		}); replayErr != nil {
			return operationsapp.AuditExport{}, replayErr
		}
		return auditExportFromRow(existing)
	} else if !errors.Is(getErr, pgx.ErrNoRows) {
		return operationsapp.AuditExport{}, fmt.Errorf("get audit export idempotency: %w", getErr)
	}
	writer := transactionProjectionWriter{queries: queries}
	if err := operationsapp.NewProjector().Project(ctx, writer, operationsapp.Projection{Run: record.RuntimeRun}); err != nil {
		return operationsapp.AuditExport{}, fmt.Errorf("project audit export run: %w", err)
	}
	exportID, _ := uuidValue(record.ID)
	runID, _ := uuidValue(record.RuntimeRun.ID)
	principalID, _ := uuidValue(record.CreatedByPrincipalID)
	row, err := queries.CreateAuditExport(ctx, dbgen.CreateAuditExportParams{ID: exportID, WorkspaceID: workspaceID,
		RuntimeRunID: runID, ArtifactID: record.ArtifactID, Format: record.Format, Filters: record.Filters,
		RowCount: int32(record.RowCount), ContentDigest: record.ContentDigest,
		RequestFingerprint: record.RequestFingerprint, IdempotencyKey: record.IdempotencyKey,
		CreatedByPrincipalID: principalID, Content: append([]byte(nil), record.Content...),
		CreatedAt: timestamp(record.CreatedAt), ExpiresAt: timestamp(record.ExpiresAt)})
	if err != nil {
		return operationsapp.AuditExport{}, fmt.Errorf("create audit export: %w", err)
	}
	auditID, err := identity.NewEventID()
	if err != nil {
		return operationsapp.AuditExport{}, err
	}
	auditDBID, _ := uuidValue(auditID)
	payload, _ := json.Marshal(map[string]any{"objectType": "audit_export", "objectId": record.ID.String(),
		"runId": record.RuntimeRun.ID.String(), "outcome": "succeeded"})
	if err := queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{ID: auditDBID, WorkspaceID: workspaceID,
		EventType: "audit.exported", ActorID: textValue(record.CreatedByPrincipalID.String()), Payload: payload,
		TraceID: record.TraceID, CreatedAt: timestamp(record.CreatedAt)}); err != nil {
		return operationsapp.AuditExport{}, fmt.Errorf("create audit export event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return operationsapp.AuditExport{}, fmt.Errorf("commit audit export: %w", err)
	}
	return auditExportFromRow(row)
}

func (store *Store) GetAuditExportContent(ctx context.Context, workspace identity.WorkspaceID, export identity.EventID, now time.Time) (operationsapp.AuditExportContent, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return operationsapp.AuditExportContent{}, err
	}
	exportID, err := uuidValue(export)
	if err != nil {
		return operationsapp.AuditExportContent{}, err
	}
	row, err := store.queries.GetAuditExportContent(ctx, dbgen.GetAuditExportContentParams{
		WorkspaceID: workspaceID, ID: exportID, Now: timestamp(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return operationsapp.AuditExportContent{}, domain.ErrNotFound
	}
	if err != nil {
		return operationsapp.AuditExportContent{}, fmt.Errorf("get audit export content: %w", err)
	}
	digest := sha256.Sum256(row.Content)
	if hex.EncodeToString(digest[:]) != row.ContentDigest {
		return operationsapp.AuditExportContent{}, errors.New("audit export content integrity check failed")
	}
	id, err := identity.EventIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return operationsapp.AuditExportContent{}, err
	}
	return operationsapp.AuditExportContent{ID: id, Format: row.Format, Content: append([]byte(nil), row.Content...),
		ContentDigest: row.ContentDigest, ExpiresAt: row.ExpiresAt.Time.UTC()}, nil
}

func (store *Store) UpsertRuntimeProjection(ctx context.Context, projection operationsapp.Projection) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin runtime projection: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := (transactionProjectionWriter{queries: dbgen.New(tx)}).UpsertRuntimeProjection(ctx, projection); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit runtime projection: %w", err)
	}
	return nil
}

type transactionProjectionWriter struct{ queries *dbgen.Queries }

func (writer transactionProjectionWriter) UpsertRuntimeProjection(ctx context.Context, projection operationsapp.Projection) error {
	runParams, err := runtimeRunParams(projection.Run)
	if err != nil {
		return err
	}
	stored, err := writer.queries.UpsertRuntimeRun(ctx, runParams)
	if err != nil {
		return fmt.Errorf("upsert runtime run: %w", err)
	}
	if stored.IdempotencyKey != projection.Run.IdempotencyKey {
		return domain.ErrConflict
	}
	storedRunID, err := identity.RunIDFromUUIDBytes(stored.ID.Bytes)
	if err != nil {
		return err
	}
	projectedRun, err := runtimeRunFromRow(stored)
	if err != nil {
		return err
	}
	if projection.Event != nil {
		event := *projection.Event
		event.RunID = storedRunID
		eventParams, encodeErr := runtimeEventParams(event)
		if encodeErr != nil {
			return encodeErr
		}
		if err := writer.queries.AppendRuntimeRunEvent(ctx, eventParams); err != nil {
			return fmt.Errorf("append runtime run event: %w", err)
		}
	}
	if err := projectRuntimeAttention(ctx, writer.queries, projectedRun); err != nil {
		return fmt.Errorf("project runtime attention: %w", err)
	}
	return nil
}

func runtimeRunParams(run domain.RuntimeRun) (dbgen.UpsertRuntimeRunParams, error) {
	id, err := uuidValue(run.ID)
	if err != nil {
		return dbgen.UpsertRuntimeRunParams{}, err
	}
	workspaceID, err := uuidValue(run.WorkspaceID)
	if err != nil {
		return dbgen.UpsertRuntimeRunParams{}, err
	}
	jobID := pgtype.UUID{}
	if run.JobID != nil {
		jobID, err = uuidValue(*run.JobID)
		if err != nil {
			return dbgen.UpsertRuntimeRunParams{}, err
		}
	}
	principalID := pgtype.UUID{}
	if run.RequestedByPrincipalID != nil {
		principalID, err = uuidValue(*run.RequestedByPrincipalID)
		if err != nil {
			return dbgen.UpsertRuntimeRunParams{}, err
		}
	}
	return dbgen.UpsertRuntimeRunParams{ID: id, WorkspaceID: workspaceID, Kind: string(run.Kind),
		SourceType: run.SourceType, SourceID: run.SourceID, SourceVersionDigest: run.SourceVersionDigest,
		JobID: jobID, TraceID: optionalTextValue(run.TraceID), IdempotencyKey: run.IdempotencyKey,
		RequestedByPrincipalID: principalID, State: string(run.State), Phase: optionalTextValue(run.Phase),
		ProgressCurrent: optionalInt64Value(run.ProgressCurrent), ProgressTotal: optionalInt64Value(run.ProgressTotal),
		Attempt: run.Attempt, MaxAttempts: run.MaxAttempts, StartedAt: optionalTimeValue(run.StartedAt),
		FinishedAt: optionalTimeValue(run.FinishedAt), ErrorCode: optionalTextValue(run.ErrorCode),
		ErrorSummary: optionalTextValue(redactDiagnostic(run.ErrorSummary)), Version: max(run.Version, 1),
		CreatedAt: timestamp(run.CreatedAt), UpdatedAt: timestamp(run.UpdatedAt)}, nil
}

func runtimeEventParams(event domain.RuntimeRunEvent) (dbgen.AppendRuntimeRunEventParams, error) {
	id, err := uuidValue(event.ID)
	if err != nil {
		return dbgen.AppendRuntimeRunEventParams{}, err
	}
	workspaceID, err := uuidValue(event.WorkspaceID)
	if err != nil {
		return dbgen.AppendRuntimeRunEventParams{}, err
	}
	runID, err := uuidValue(event.RunID)
	if err != nil {
		return dbgen.AppendRuntimeRunEventParams{}, err
	}
	metadata := domain.ProjectRuntimeMetadata(event.Metadata)
	return dbgen.AppendRuntimeRunEventParams{ID: id, WorkspaceID: workspaceID, RuntimeRunID: runID,
		EventKey: event.EventKey, EventType: string(event.Type), Phase: optionalTextValue(event.Phase),
		ProgressCurrent: optionalInt64Value(event.ProgressCurrent), ProgressTotal: optionalInt64Value(event.ProgressTotal),
		State: optionalTextValue(string(event.State)), ErrorCode: optionalTextValue(event.ErrorCode),
		Summary: optionalTextValue(redactDiagnostic(event.Summary)), Metadata: metadata, CreatedAt: timestamp(event.CreatedAt)}, nil
}

func runtimeRunFromRow(row dbgen.RuntimeRun) (domain.RuntimeRun, error) {
	id, err := identity.RunIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.RuntimeRun{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return domain.RuntimeRun{}, err
	}
	result := domain.RuntimeRun{ID: id, WorkspaceID: workspaceID, Kind: domain.RunKind(row.Kind),
		SourceType: row.SourceType, SourceID: row.SourceID, SourceVersionDigest: row.SourceVersionDigest,
		TraceID: optionalText(row.TraceID), IdempotencyKey: row.IdempotencyKey, State: domain.RunState(row.State),
		Phase: optionalText(row.Phase), ProgressCurrent: optionalInt64(row.ProgressCurrent),
		ProgressTotal: optionalInt64(row.ProgressTotal), Attempt: row.Attempt, MaxAttempts: row.MaxAttempts,
		StartedAt: optionalTime(row.StartedAt), FinishedAt: optionalTime(row.FinishedAt), ErrorCode: optionalText(row.ErrorCode),
		ErrorSummary: optionalText(row.ErrorSummary), Version: row.Version, CreatedAt: row.CreatedAt.Time.UTC(),
		UpdatedAt: row.UpdatedAt.Time.UTC(), Capabilities: domain.RunCapabilities{}}
	if row.JobID.Valid {
		jobID, decodeErr := identity.RunIDFromUUIDBytes(row.JobID.Bytes)
		if decodeErr != nil {
			return domain.RuntimeRun{}, decodeErr
		}
		result.JobID = &jobID
	}
	if row.RequestedByPrincipalID.Valid {
		principal, decodeErr := identity.PrincipalIDFromUUIDBytes(row.RequestedByPrincipalID.Bytes)
		if decodeErr != nil {
			return domain.RuntimeRun{}, decodeErr
		}
		result.RequestedByPrincipalID = &principal
	}
	return result, nil
}

func runtimeEventFromRow(row dbgen.RuntimeRunEvent) (domain.RuntimeRunEvent, error) {
	id, err := identity.EventIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.RuntimeRunEvent{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return domain.RuntimeRunEvent{}, err
	}
	runID, err := identity.RunIDFromUUIDBytes(row.RuntimeRunID.Bytes)
	if err != nil {
		return domain.RuntimeRunEvent{}, err
	}
	return domain.RuntimeRunEvent{ID: id, WorkspaceID: workspaceID, RunID: runID, EventKey: row.EventKey,
		Sequence: row.Sequence, Type: domain.RunEventType(row.EventType), Phase: optionalText(row.Phase),
		ProgressCurrent: optionalInt64(row.ProgressCurrent), ProgressTotal: optionalInt64(row.ProgressTotal),
		State: domain.RunState(optionalText(row.State)), ErrorCode: optionalText(row.ErrorCode),
		Summary: optionalText(row.Summary), Metadata: cloneJSON(row.Metadata), CreatedAt: row.CreatedAt.Time.UTC()}, nil
}

func runtimeSettingsFromRow(row dbgen.RuntimeSetting) (domain.RuntimeSettings, error) {
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return domain.RuntimeSettings{}, err
	}
	return domain.RuntimeSettings{WorkspaceID: workspaceID, RetryCeiling: row.RetryCeiling,
		StatementTimeoutMS: row.StatementTimeoutMs, WebhookTimeoutMS: row.WebhookTimeoutMs,
		QueryRowLimit: row.QueryRowLimit, QueryByteLimit: row.QueryByteLimit,
		RunMetadataRetentionDays: row.RunMetadataRetentionDays, Version: row.Version,
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}, nil
}

func auditExportFromRow(row dbgen.AuditExport) (operationsapp.AuditExport, error) {
	id, err := identity.EventIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return operationsapp.AuditExport{}, err
	}
	runID, err := identity.RunIDFromUUIDBytes(row.RuntimeRunID.Bytes)
	if err != nil {
		return operationsapp.AuditExport{}, err
	}
	return operationsapp.AuditExport{ID: id, RuntimeRunID: runID, ArtifactID: row.ArtifactID,
		Format: row.Format, RowCount: int(row.RowCount), ContentDigest: row.ContentDigest,
		CreatedAt: row.CreatedAt.Time.UTC(), ExpiresAt: row.ExpiresAt.Time.UTC()}, nil
}

func optionalInt64Value(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func optionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func optionalTimeValue(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*value)
}

// Runtime diagnostics are codes plus bounded summaries, never provider output.
func redactDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		value = value[:512]
	}
	for _, marker := range []string{
		"postgres://", "postgresql://", "bearer", "authorization", "cookie", "password", "passwd",
		"token", "secret", "private key", "signing", "prompt", "dsn",
	} {
		if index := strings.Index(strings.ToLower(value), strings.ToLower(marker)); index >= 0 {
			return "diagnostic details redacted"
		}
	}
	return value
}

func projectRuntime(ctx context.Context, queries *dbgen.Queries, run domain.RuntimeRun, event *domain.RuntimeRunEvent) error {
	return operationsapp.NewProjector().Project(ctx, transactionProjectionWriter{queries: queries}, operationsapp.Projection{Run: run, Event: event})
}

func newRuntimeEvent(workspace identity.WorkspaceID, run identity.RunID, key string, eventType domain.RunEventType,
	state domain.RunState, phase, errorCode, summary string, now time.Time,
) (*domain.RuntimeRunEvent, error) {
	eventID, err := identity.NewEventID()
	if err != nil {
		return nil, err
	}
	return &domain.RuntimeRunEvent{ID: eventID, WorkspaceID: workspace, RunID: run, EventKey: key,
		Type: eventType, State: state, Phase: phase, ErrorCode: errorCode,
		Summary: redactDiagnostic(summary), Metadata: json.RawMessage(`{}`), CreatedAt: now.UTC()}, nil
}

func runtimeDigest(values ...string) string {
	digest := sha256.New()
	for _, value := range values {
		_, _ = digest.Write([]byte(value))
		_, _ = digest.Write([]byte{0})
	}
	return fmt.Sprintf("%x", digest.Sum(nil))
}

func normalizedRuntimeDigest(value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), "sha256:")
	if len(value) == 64 {
		return value
	}
	return runtimeDigest(value)
}

func boundedRuntimeIdempotencyKey(prefix, source string) string {
	value := prefix + strings.TrimSpace(source)
	if len(value) <= 256 {
		return value
	}
	return prefix + runtimeDigest(source)
}

func canonicalJSONEqual(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	leftCanonical, leftErr := json.Marshal(leftValue)
	rightCanonical, rightErr := json.Marshal(rightValue)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftCanonical, rightCanonical)
}

func validateAuditExportReplay(existing dbgen.AuditExport, request operationsapp.AuditExportReplayRequest) error {
	principalID, err := uuidValue(request.CreatedByPrincipalID)
	if err != nil {
		return err
	}
	if !existing.ExpiresAt.Time.After(request.Now) || existing.RequestFingerprint != request.RequestFingerprint ||
		existing.CreatedByPrincipalID != principalID || !canonicalJSONEqual(existing.Filters, request.Filters) {
		return domain.ErrConflict
	}
	return nil
}

func principalIDPointer(value string) *identity.PrincipalID {
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	return &principal
}

func timePointer(value time.Time) *time.Time {
	result := value.UTC()
	return &result
}

func jobAttempt(ctx context.Context, tx pgx.Tx, workspaceID, jobID pgtype.UUID) (int32, int32, error) {
	var attempt, maxAttempts int32
	err := tx.QueryRow(ctx, `SELECT attempt, max_attempts FROM jobs WHERE workspace_id=$1 AND id=$2`, workspaceID, jobID).
		Scan(&attempt, &maxAttempts)
	return attempt, maxAttempts, err
}

func projectJobTerminalState(ctx context.Context, queries *dbgen.Queries, workspaceID, jobID pgtype.UUID,
	state domain.RunState, errorCode string, now time.Time,
) error {
	stored, err := queries.GetOperationsRuntimeRunByJob(ctx, dbgen.GetOperationsRuntimeRunByJobParams{
		WorkspaceID: workspaceID, JobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load linked runtime run: %w", err)
	}
	run, err := runtimeRunFromRow(stored)
	if err != nil {
		return err
	}
	if terminalRuntimeState(run.State) && run.State != state {
		return nil
	}
	if run.Kind == domain.RunKindEmbeddingRebuild && state == domain.RunDeadLetter {
		if err := queries.FailExpiredEmbedding(ctx, dbgen.FailExpiredEmbeddingParams{WorkspaceID: workspaceID, JobID: jobID}); err != nil {
			return err
		}
	}
	job, err := queries.GetJobByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load linked job attempt: %w", err)
	}
	attempt, maxAttempts := job.Attempt, job.MaxAttempts
	run.Attempt, run.MaxAttempts, run.State, run.UpdatedAt = attempt, maxAttempts, state, now.UTC()
	run.Phase, run.ErrorCode, run.ErrorSummary, run.FinishedAt = string(state), "", "", nil
	if state == domain.RunRunning && run.StartedAt == nil {
		run.StartedAt = timePointer(now)
	} else if state == domain.RunSucceeded {
		run.FinishedAt = timePointer(now)
	} else if state == domain.RunDeadLetter {
		run.FinishedAt, run.ErrorCode, run.ErrorSummary = timePointer(now), errorCode, "runtime job exhausted its retry ceiling"
	}
	event, err := newRuntimeEvent(run.WorkspaceID, run.ID, fmt.Sprintf("attempt:%d:%s", attempt, state),
		domain.RunEventState, state, run.Phase, run.ErrorCode, run.ErrorSummary, now)
	if err != nil {
		return err
	}
	return projectRuntime(ctx, queries, run, event)
}

func terminalRuntimeState(state domain.RunState) bool {
	switch state {
	case domain.RunSucceeded, domain.RunDegraded, domain.RunFailed, domain.RunCancelled, domain.RunDeadLetter:
		return true
	default:
		return false
	}
}
