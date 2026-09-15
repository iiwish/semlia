package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	application "github.com/iiwish/semlia/internal/application/ingestion"
	domain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ application.Repository = (*Store)(nil)

const artifactColumns = `id, workspace_id, source_connection_id, artifact_kind, schema_version,
content_sha256, byte_size, media_type, original_name, registered_logical_path, validation_summary, status, failure_code,
uploaded_by_principal_id, idempotency_key, request_fingerprint, expires_at, created_at, finalized_at`

const artifactReadAvailability = `, CASE
WHEN content_sha256 IS NULL THEN 'not_stored'
WHEN EXISTS (SELECT 1 FROM artifact_object_retention r
  WHERE r.workspace_id=source_artifacts.workspace_id AND r.content_sha256=source_artifacts.content_sha256
    AND r.state IN ('reserved','retained')) THEN 'available'
ELSE 'expired' END`

func (store *Store) FindArtifactByIdempotency(ctx context.Context, workspace identity.WorkspaceID, key, fingerprint string, expectedAuthorizationVersion int64) (*domain.Artifact, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, artifactRepositoryError("begin artifact idempotency read", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockArtifactWorkspace(ctx, tx, workspace, expectedAuthorizationVersion); err != nil {
		return nil, err
	}
	workspaceID, _ := uuidValue(workspace)
	artifact, err := scanArtifactRead(tx.QueryRow(ctx, `SELECT `+artifactColumns+artifactReadAvailability+`
FROM source_artifacts WHERE workspace_id=$1 AND idempotency_key=$2`, workspaceID, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, artifactRepositoryError("find artifact idempotency", err)
	}
	if artifact.RequestFingerprint != fingerprint {
		return nil, domain.ErrConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, artifactRepositoryError("commit artifact idempotency read", err)
	}
	return &artifact, nil
}

func (store *Store) ReserveArtifactObject(ctx context.Context, command application.CreateArtifactCommand) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return artifactRepositoryError("begin artifact reservation", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockArtifactWorkspace(ctx, tx, command.Artifact.WorkspaceID, command.ExpectedAuthorizationVersion); err != nil {
		return err
	}
	if err := validateArtifactSource(ctx, tx, command); err != nil {
		return err
	}
	workspaceID, _ := uuidValue(command.Artifact.WorkspaceID)
	var size int64
	var state string
	var reservations int
	var retentionExpiry pgtype.Timestamptz
	err = tx.QueryRow(ctx, `SELECT o.byte_size,r.state,r.reservation_count,r.expires_at
FROM artifact_objects o JOIN artifact_object_retention r USING(workspace_id,content_sha256)
WHERE o.workspace_id=$1 AND o.content_sha256=$2 FOR UPDATE OF r`, workspaceID, command.Object.ContentDigest).Scan(&size, &state, &reservations, &retentionExpiry)
	if err == nil && size != command.Object.ByteSize {
		return domain.ErrConflict
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return artifactRepositoryError("load artifact reservation", err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if err := reserveArtifactQuota(ctx, tx, workspaceID, command.Object.ByteSize, command.Object.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO artifact_objects(workspace_id,content_sha256,byte_size,storage_key,created_at)
VALUES($1,$2,$3,$4,$5)`, workspaceID, command.Object.ContentDigest, command.Object.ByteSize, command.Object.StorageKey, timestamp(command.Object.CreatedAt)); err != nil {
			return artifactRepositoryError("create reserved artifact object", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO artifact_object_retention(workspace_id,content_sha256,state,reservation_count,expires_at,updated_at)
VALUES($1,$2,'reserved',1,$3,$4)`, workspaceID, command.Object.ContentDigest, timestamp(command.Object.CreatedAt.Add(time.Hour)), timestamp(command.Object.CreatedAt)); err != nil {
			return artifactRepositoryError("create artifact reservation", err)
		}
	} else if state == "deleted" {
		if err := reserveArtifactQuota(ctx, tx, workspaceID, command.Object.ByteSize, command.Object.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE artifact_object_retention SET state='reserved',reservation_count=1,expires_at=$3,
cleanup_attempts=0,cleanup_retry_at=NULL,updated_at=$4 WHERE workspace_id=$1 AND content_sha256=$2`, workspaceID, command.Object.ContentDigest, timestamp(command.Object.CreatedAt.Add(time.Hour)), timestamp(command.Object.CreatedAt)); err != nil {
			return artifactRepositoryError("revive artifact reservation", err)
		}
	} else if state == "reserved" {
		if _, err := tx.Exec(ctx, `UPDATE artifact_object_retention SET reservation_count=reservation_count+1,expires_at=GREATEST(expires_at,$3),
cleanup_attempts=0,cleanup_retry_at=NULL,updated_at=$4 WHERE workspace_id=$1 AND content_sha256=$2`, workspaceID, command.Object.ContentDigest, timestamp(command.Object.CreatedAt.Add(time.Hour)), timestamp(command.Object.CreatedAt)); err != nil {
			return artifactRepositoryError("join artifact reservation", err)
		}
	} else if state == "retained" && retentionExpiry.Valid {
		if _, err := tx.Exec(ctx, `UPDATE artifact_object_retention SET expires_at=GREATEST(expires_at,$3),cleanup_attempts=0,cleanup_retry_at=NULL,updated_at=$4
WHERE workspace_id=$1 AND content_sha256=$2 AND state='retained'`, workspaceID, command.Object.ContentDigest,
			timestamp(command.Object.CreatedAt.Add(time.Hour)), timestamp(command.Object.CreatedAt)); err != nil {
			return artifactRepositoryError("protect retained artifact reupload", err)
		}
	} else if state != "retained" {
		return domain.ErrConflict
	}
	return tx.Commit(ctx)
}

func (store *Store) ReleaseArtifactObjectReservation(ctx context.Context, command application.CreateArtifactCommand,
	deleteObject func(context.Context, domain.CleanupObject) error,
) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return artifactRepositoryError("begin release artifact reservation", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockArtifactWorkspace(ctx, tx, command.Artifact.WorkspaceID, 0); err != nil {
		return err
	}
	workspaceID, _ := uuidValue(command.Artifact.WorkspaceID)
	var state string
	var count int
	var byteSize int64
	err = tx.QueryRow(ctx, `SELECT r.state,r.reservation_count,o.byte_size
FROM artifact_object_retention r JOIN artifact_objects o USING(workspace_id,content_sha256)
WHERE r.workspace_id=$1 AND r.content_sha256=$2 FOR UPDATE OF r`, workspaceID, command.Object.ContentDigest).Scan(&state, &count, &byteSize)
	if errors.Is(err, pgx.ErrNoRows) || state == "retained" || state == "deleted" || state == "deleting" {
		return nil
	}
	if err != nil {
		return artifactRepositoryError("load artifact reservation release", err)
	}
	if state != "reserved" {
		return domain.ErrConflict
	}
	if count > 1 {
		_, err = tx.Exec(ctx, `UPDATE artifact_object_retention SET reservation_count=reservation_count-1,updated_at=$3 WHERE workspace_id=$1 AND content_sha256=$2`, workspaceID, command.Object.ContentDigest, timestamp(time.Now().UTC()))
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	var referenced bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_artifacts WHERE workspace_id=$1 AND content_sha256=$2)`, workspaceID, command.Object.ContentDigest).Scan(&referenced); err != nil {
		return err
	}
	if referenced {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE artifact_object_retention SET state='deleting',reservation_count=0,expires_at=NULL,updated_at=$3 WHERE workspace_id=$1 AND content_sha256=$2`, workspaceID, command.Object.ContentDigest, timestamp(time.Now().UTC())); err != nil {
		return err
	}
	if deleteObject == nil {
		return domain.ErrInvalid
	}
	object := domain.CleanupObject{WorkspaceID: command.Artifact.WorkspaceID, ContentDigest: command.Object.ContentDigest, ByteSize: byteSize}
	if err := deleteObject(ctx, object); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE artifact_object_retention SET state='deleted',updated_at=$3 WHERE workspace_id=$1 AND content_sha256=$2`, workspaceID, command.Object.ContentDigest, timestamp(time.Now().UTC())); err != nil {
		return artifactRepositoryError("complete released artifact deletion", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE workspace_artifact_usage SET active_object_count=active_object_count-1,active_bytes=active_bytes-$2,updated_at=$3 WHERE workspace_id=$1`, workspaceID, byteSize, timestamp(time.Now().UTC())); err != nil {
		return artifactRepositoryError("release artifact quota", err)
	}
	return tx.Commit(ctx)
}

func (store *Store) CleanupArtifactObjects(ctx context.Context, now time.Time, limit int,
	deleteObject func(context.Context, domain.CleanupObject) error,
) (int, error) {
	if deleteObject == nil || limit < 1 || limit > 100 {
		return 0, domain.ErrInvalid
	}
	processed := 0
	for processed < limit {
		found, err := store.cleanupArtifactObject(ctx, now, deleteObject)
		if err != nil {
			return processed, err
		}
		if !found {
			break
		}
		processed++
	}
	return processed, nil
}

func (store *Store) cleanupArtifactObject(ctx context.Context, now time.Time,
	deleteObject func(context.Context, domain.CleanupObject) error,
) (bool, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, artifactRepositoryError("begin artifact cleanup", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var workspace pgtype.UUID
	var object domain.CleanupObject
	var state string
	var expiresAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `SELECT r.workspace_id,r.content_sha256,o.byte_size,r.state,r.expires_at
FROM artifact_object_retention r JOIN artifact_objects o USING(workspace_id,content_sha256)
WHERE (r.state='deleting' OR (r.state='reserved' AND r.expires_at<=$1)
  OR (r.state='retained' AND r.expires_at<=$1
    AND NOT EXISTS(SELECT 1 FROM source_connections active_source JOIN source_artifact_set_members active_member
      ON active_member.workspace_id=active_source.workspace_id AND active_member.source_connection_id=active_source.id
        AND active_member.artifact_set_id=active_source.active_artifact_set_id
      WHERE active_source.workspace_id=r.workspace_id AND active_source.status<>'deleted'
        AND active_member.content_sha256=r.content_sha256)
    AND NOT EXISTS(SELECT 1 FROM discovery_runs active_run JOIN discovery_run_artifacts active_pin
      ON active_pin.workspace_id=active_run.workspace_id AND active_pin.discovery_run_id=active_run.id
      WHERE active_run.workspace_id=r.workspace_id AND active_run.status IN ('queued','running')
        AND active_pin.content_sha256=r.content_sha256))
  OR EXISTS(SELECT 1 FROM source_artifacts a WHERE a.workspace_id=r.workspace_id
    AND a.content_sha256=r.content_sha256 AND a.status='uploaded' AND a.expires_at<=$1))
AND (r.cleanup_retry_at IS NULL OR r.cleanup_retry_at<=$1)
AND NOT EXISTS(SELECT 1 FROM source_revision_artifacts pin JOIN source_snapshots snapshot
  ON snapshot.workspace_id=pin.workspace_id AND snapshot.source_revision_id=pin.source_revision_id
  WHERE pin.workspace_id=r.workspace_id AND pin.content_sha256=r.content_sha256)
AND pg_try_advisory_xact_lock(hashtextextended(r.workspace_id::text,18004))
ORDER BY r.updated_at,r.workspace_id,r.content_sha256 FOR UPDATE OF r SKIP LOCKED LIMIT 1`, timestamp(now)).
		Scan(&workspace, &object.ContentDigest, &object.ByteSize, &state, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, artifactRepositoryError("lock artifact cleanup", err)
	}
	object.WorkspaceID, _ = identity.WorkspaceIDFromUUIDBytes(workspace.Bytes)
	workspaceID := workspace
	if _, err := tx.Exec(ctx, `UPDATE source_artifacts SET status='rejected',content_sha256=NULL,
failure_code='ARTIFACT_EXPIRED',finalized_at=$3,expires_at=NULL
WHERE workspace_id=$1 AND content_sha256=$2 AND status='uploaded' AND expires_at<=$3`,
		workspaceID, object.ContentDigest, timestamp(now)); err != nil {
		return false, artifactRepositoryError("expire abandoned artifact", err)
	}
	var protected bool
	if err := tx.QueryRow(ctx, `SELECT
  EXISTS(SELECT 1 FROM source_connections s JOIN source_artifact_set_members m
    ON m.workspace_id=s.workspace_id AND m.source_connection_id=s.id AND m.artifact_set_id=s.active_artifact_set_id
    WHERE s.workspace_id=$1 AND s.status<>'deleted' AND m.content_sha256=$2)
  OR EXISTS(SELECT 1 FROM discovery_runs run JOIN discovery_run_artifacts pin
    ON pin.workspace_id=run.workspace_id AND pin.discovery_run_id=run.id
		WHERE run.workspace_id=$1 AND run.status IN ('queued','running') AND pin.content_sha256=$2)
  OR EXISTS(SELECT 1 FROM source_artifacts pending
    WHERE pending.workspace_id=$1 AND pending.status='uploaded' AND pending.content_sha256=$2)
  OR EXISTS(SELECT 1 FROM source_revision_artifacts pin JOIN source_snapshots snapshot
    ON snapshot.workspace_id=pin.workspace_id AND snapshot.source_revision_id=pin.source_revision_id
    WHERE pin.workspace_id=$1 AND pin.content_sha256=$2)`,
		workspaceID, object.ContentDigest).Scan(&protected); err != nil {
		return false, artifactRepositoryError("check artifact cleanup protection", err)
	}
	if protected || (state == "retained" && (!expiresAt.Valid || expiresAt.Time.After(now))) {
		if err := tx.Commit(ctx); err != nil {
			return false, artifactRepositoryError("commit protected artifact cleanup", err)
		}
		return true, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE artifact_object_retention SET state='deleting',reservation_count=0,
expires_at=NULL,updated_at=$3 WHERE workspace_id=$1 AND content_sha256=$2`, workspaceID, object.ContentDigest, timestamp(now)); err != nil {
		return false, artifactRepositoryError("mark artifact cleanup", err)
	}
	if err := deleteObject(ctx, object); err != nil {
		_ = tx.Rollback(ctx)
		store.deferArtifactCleanup(ctx, object, now)
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE artifact_object_retention SET state='deleted',updated_at=$3
WHERE workspace_id=$1 AND content_sha256=$2 AND state='deleting'`, workspaceID, object.ContentDigest, timestamp(now)); err != nil {
		return false, artifactRepositoryError("complete artifact cleanup", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE workspace_artifact_usage
SET active_object_count=active_object_count-1,active_bytes=active_bytes-$2,updated_at=$3 WHERE workspace_id=$1`,
		workspaceID, object.ByteSize, timestamp(now)); err != nil {
		return false, artifactRepositoryError("release artifact cleanup quota", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, artifactRepositoryError("commit artifact cleanup", err)
	}
	return true, nil
}

func (store *Store) deferArtifactCleanup(ctx context.Context, object domain.CleanupObject, now time.Time) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	workspaceID, err := uuidValue(object.WorkspaceID)
	if err != nil {
		return
	}
	_, _ = store.pool.Exec(cleanupCtx, `UPDATE artifact_object_retention
SET cleanup_attempts=LEAST(cleanup_attempts+1,1000),cleanup_retry_at=$3,updated_at=$2
WHERE workspace_id=$1 AND content_sha256=$4 AND state<>'deleted'`, workspaceID, timestamp(now), timestamp(now.Add(time.Minute)), object.ContentDigest)
}

func (store *Store) CreateArtifact(ctx context.Context, command application.CreateArtifactCommand) (domain.Artifact, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Artifact{}, artifactRepositoryError("begin artifact create", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockArtifactWorkspace(ctx, tx, command.Artifact.WorkspaceID, command.ExpectedAuthorizationVersion); err != nil {
		return domain.Artifact{}, err
	}
	if existing, err := findArtifactInTx(ctx, tx, command.Artifact.WorkspaceID, command.Artifact.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != command.Artifact.RequestFingerprint {
			return domain.Artifact{}, domain.ErrConflict
		}
		return existing, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Artifact{}, err
	}
	if err := validateArtifactSource(ctx, tx, command); err != nil {
		return domain.Artifact{}, err
	}
	workspaceID, artifactID, uploaderID, err := artifactDatabaseIDs(command.Artifact)
	if err != nil {
		return domain.Artifact{}, err
	}
	if command.Artifact.ExpiresAt == nil || !command.Artifact.ExpiresAt.After(command.Artifact.CreatedAt) {
		return domain.Artifact{}, domain.ErrInvalid
	}
	retained, err := tx.Exec(ctx, `UPDATE artifact_object_retention
SET state='retained',reservation_count=0,cleanup_attempts=0,cleanup_retry_at=NULL,
expires_at=CASE WHEN state='retained' AND expires_at IS NULL THEN NULL ELSE GREATEST(expires_at,$3::timestamptz) END,updated_at=$4
WHERE workspace_id=$1 AND content_sha256=$2 AND state IN('reserved','retained')`,
		workspaceID, command.Object.ContentDigest, timestamp(*command.Artifact.ExpiresAt), timestamp(command.Object.CreatedAt))
	if err != nil || retained.RowsAffected() != 1 {
		if err == nil {
			err = domain.ErrConflict
		}
		return domain.Artifact{}, artifactRepositoryError("retain artifact object", err)
	}
	sourceID, err := nullableUUIDValue(command.Artifact.SourceConnectionID)
	if err != nil {
		return domain.Artifact{}, err
	}
	row := tx.QueryRow(ctx, `INSERT INTO source_artifacts
(id,workspace_id,source_connection_id,artifact_kind,schema_version,content_sha256,byte_size,media_type,original_name,registered_logical_path,validation_summary,status,
 uploaded_by_principal_id,idempotency_key,request_fingerprint,expires_at,created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,COALESCE($11::jsonb,'{}'::jsonb),'uploaded',$12,$13,$14,$15,$16) RETURNING `+artifactColumns,
		artifactID, workspaceID, sourceID, command.Artifact.Kind, command.Artifact.SchemaVersion, command.Artifact.ContentDigest,
		command.Artifact.ByteSize, command.Artifact.MediaType, command.Artifact.OriginalName, nullableString(command.Artifact.LogicalPath), command.Artifact.ValidationSummary, uploaderID,
		command.Artifact.IdempotencyKey, command.Artifact.RequestFingerprint, timestamp(*command.Artifact.ExpiresAt), timestamp(command.Artifact.CreatedAt))
	created, err := scanArtifact(row)
	if err != nil {
		return domain.Artifact{}, artifactRepositoryError("create artifact", err)
	}
	details := map[string]any{"artifactId": command.Artifact.ID.String(), "kind": command.Artifact.Kind, "digest": command.Artifact.ContentDigest, "byteSize": command.Artifact.ByteSize}
	if command.Artifact.SourceConnectionID != nil {
		details["sourceId"] = command.Artifact.SourceConnectionID.String()
	}
	if err := insertSourceAudit(ctx, tx, command.Artifact.WorkspaceID, "artifact.uploaded", command.Artifact.UploadedByPrincipalID.String(), command.TraceID, details); err != nil {
		return domain.Artifact{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Artifact{}, artifactRepositoryError("commit artifact", err)
	}
	return created, nil
}

func (store *Store) RecordRejectedArtifact(ctx context.Context, command application.CreateArtifactCommand, code string) (domain.Artifact, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Artifact{}, artifactRepositoryError("begin rejected artifact", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockArtifactWorkspace(ctx, tx, command.Artifact.WorkspaceID, command.ExpectedAuthorizationVersion); err != nil {
		return domain.Artifact{}, err
	}
	if existing, err := findArtifactInTx(ctx, tx, command.Artifact.WorkspaceID, command.Artifact.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != command.Artifact.RequestFingerprint {
			return domain.Artifact{}, domain.ErrConflict
		}
		return existing, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Artifact{}, err
	}
	if err := validateArtifactSource(ctx, tx, command); err != nil {
		return domain.Artifact{}, err
	}
	workspaceID, artifactID, uploaderID, err := artifactDatabaseIDs(command.Artifact)
	if err != nil {
		return domain.Artifact{}, err
	}
	sourceID, err := nullableUUIDValue(command.Artifact.SourceConnectionID)
	if err != nil {
		return domain.Artifact{}, err
	}
	row := tx.QueryRow(ctx, `INSERT INTO source_artifacts
(id,workspace_id,source_connection_id,artifact_kind,schema_version,content_sha256,byte_size,media_type,original_name,registered_logical_path,validation_summary,status,failure_code,
 uploaded_by_principal_id,idempotency_key,request_fingerprint,created_at,finalized_at)
VALUES ($1,$2,$3,$4,$5,NULL,$6,$7,$8,$9,'{}'::jsonb,'rejected',$10,$11,$12,$13,$14,$14) RETURNING `+artifactColumns,
		artifactID, workspaceID, sourceID, command.Artifact.Kind, command.Artifact.SchemaVersion, command.Artifact.ByteSize,
		command.Artifact.MediaType, command.Artifact.OriginalName, nullableString(command.Artifact.LogicalPath), code, uploaderID, command.Artifact.IdempotencyKey,
		command.Artifact.RequestFingerprint, timestamp(command.Artifact.CreatedAt))
	created, err := scanArtifact(row)
	if err != nil {
		return domain.Artifact{}, artifactRepositoryError("record rejected artifact", err)
	}
	details := map[string]any{"artifactId": command.Artifact.ID.String(), "kind": command.Artifact.Kind, "failureCode": code, "byteSize": command.Artifact.ByteSize}
	if command.Artifact.SourceConnectionID != nil {
		details["sourceId"] = command.Artifact.SourceConnectionID.String()
	}
	if err := insertSourceAudit(ctx, tx, command.Artifact.WorkspaceID, "artifact.rejected", command.Artifact.UploadedByPrincipalID.String(), command.TraceID, details); err != nil {
		return domain.Artifact{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Artifact{}, artifactRepositoryError("commit rejected artifact", err)
	}
	return created, nil
}

func (store *Store) GetArtifact(ctx context.Context, workspace identity.WorkspaceID, artifact identity.ArtifactID) (domain.Artifact, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return domain.Artifact{}, err
	}
	artifactID, err := uuidValue(artifact)
	if err != nil {
		return domain.Artifact{}, err
	}
	value, err := scanArtifactRead(store.pool.QueryRow(ctx, `SELECT `+artifactColumns+artifactReadAvailability+` FROM source_artifacts WHERE workspace_id=$1 AND id=$2`, workspaceID, artifactID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Artifact{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Artifact{}, artifactRepositoryError("get artifact", err)
	}
	return value, nil
}

func (store *Store) ListArtifacts(ctx context.Context, query application.ListArtifactsQuery) (domain.Page[domain.Artifact], error) {
	workspaceID, err := uuidValue(query.WorkspaceID)
	if err != nil {
		return domain.Page[domain.Artifact]{}, err
	}
	cursor, err := decodeArtifactCursor(query)
	if err != nil {
		return domain.Page[domain.Artifact]{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.Page[domain.Artifact]{}, artifactRepositoryError("begin artifact page", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := verifyArtifactAuthorizationVersion(ctx, tx, query.WorkspaceID, query.AuthorizationVersion); err != nil {
		return domain.Page[domain.Artifact]{}, err
	}
	sourceID, err := nullableUUIDValue(query.SourceID)
	if err != nil {
		return domain.Page[domain.Artifact]{}, err
	}
	filters := ` workspace_id=$1 AND ($2='' OR artifact_kind=$2) AND ($3='' OR status=$3) AND ($4::uuid IS NULL OR source_connection_id=$4)`
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM source_artifacts WHERE`+filters, workspaceID, query.Kind, query.Status, sourceID).Scan(&total); err != nil {
		return domain.Page[domain.Artifact]{}, artifactRepositoryError("count artifacts", err)
	}
	rows, err := tx.Query(ctx, `SELECT `+artifactColumns+artifactReadAvailability+` FROM source_artifacts WHERE`+filters+`
AND ($5::timestamptz IS NULL OR (created_at,id) < ($5,$6)) ORDER BY created_at DESC,id DESC LIMIT $7`,
		workspaceID, query.Kind, query.Status, sourceID, cursor.CreatedAt, cursor.ID, query.Limit+1)
	if err != nil {
		return domain.Page[domain.Artifact]{}, artifactRepositoryError("list artifacts", err)
	}
	defer rows.Close()
	items := make([]domain.Artifact, 0, query.Limit+1)
	for rows.Next() {
		value, scanErr := scanArtifactRead(rows)
		if scanErr != nil {
			return domain.Page[domain.Artifact]{}, scanErr
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		return domain.Page[domain.Artifact]{}, artifactRepositoryError("iterate artifacts", err)
	}
	next := ""
	if len(items) > query.Limit {
		items = items[:query.Limit]
		next = encodeArtifactCursor(query, items[len(items)-1])
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Page[domain.Artifact]{}, artifactRepositoryError("commit artifact page", err)
	}
	return domain.Page[domain.Artifact]{Items: items, Total: total, NextCursor: next, Limit: query.Limit}, nil
}

func (store *Store) FindArtifactSetByIdempotency(ctx context.Context, workspace identity.WorkspaceID, key, fingerprint string, expectedAuthorizationVersion int64) (*domain.ArtifactSet, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, artifactRepositoryError("begin artifact set idempotency read", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockArtifactWorkspace(ctx, tx, workspace, expectedAuthorizationVersion); err != nil {
		return nil, err
	}
	workspaceID, _ := uuidValue(workspace)
	var setID pgtype.UUID
	var storedFingerprint string
	err = tx.QueryRow(ctx, `SELECT id,request_fingerprint FROM source_artifact_sets WHERE workspace_id=$1 AND idempotency_key=$2`, workspaceID, key).Scan(&setID, &storedFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, artifactRepositoryError("find artifact set idempotency", err)
	}
	if storedFingerprint != fingerprint {
		return nil, domain.ErrConflict
	}
	set, err := loadArtifactSet(ctx, tx, workspace, setID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, artifactRepositoryError("commit artifact set idempotency read", err)
	}
	return &set, nil
}

func (store *Store) FinalizeArtifactSet(ctx context.Context, command application.FinalizeArtifactSetCommand) (domain.ArtifactSet, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.ArtifactSet{}, artifactRepositoryError("begin artifact finalize", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockArtifactWorkspace(ctx, tx, command.Set.WorkspaceID, command.ExpectedAuthorizationVersion); err != nil {
		return domain.ArtifactSet{}, err
	}
	workspaceID, _ := uuidValue(command.Set.WorkspaceID)
	var existingID pgtype.UUID
	var existingFingerprint string
	err = tx.QueryRow(ctx, `SELECT id,request_fingerprint FROM source_artifact_sets WHERE workspace_id=$1 AND idempotency_key=$2`, workspaceID, command.IdempotencyKey).Scan(&existingID, &existingFingerprint)
	if err == nil {
		if existingFingerprint != command.RequestFingerprint {
			return domain.ArtifactSet{}, domain.ErrConflict
		}
		return loadArtifactSet(ctx, tx, command.Set.WorkspaceID, existingID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.ArtifactSet{}, artifactRepositoryError("check finalize idempotency", err)
	}
	sourceID, _ := uuidValue(command.Set.SourceConnectionID)
	setID, _ := uuidValue(command.Set.ID)
	creatorID, _ := uuidValue(command.Set.CreatedByPrincipalID)
	adapterKind := map[string]string{"file": "file_catalog", "sql_bundle": "postgresql_sql", "dbt_bundle": "dbt"}[command.Set.SourceKind]
	locator := "artifact-set:" + command.Set.ID.String()
	if command.ExpectedSourceVersion == nil {
		if _, err := tx.Exec(ctx, `INSERT INTO source_connections
(id,workspace_id,adapter_kind,name,normalized_locator,status,metadata,source_kind,active_credential_version,artifact_paths,version,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,'paused','{}'::jsonb,$6,NULL,'[]'::jsonb,1,$7,$7)`, sourceID, workspaceID, adapterKind, command.SourceName, locator, command.Set.SourceKind, timestamp(command.Set.CreatedAt)); err != nil {
			return domain.ArtifactSet{}, artifactRepositoryError("create artifact source", err)
		}
	} else {
		var storedKind, storedName string
		var storedVersion int64
		if err := tx.QueryRow(ctx, `SELECT source_kind,name,version FROM source_connections
WHERE workspace_id=$1 AND id=$2 AND status IN ('active','paused') FOR UPDATE`, workspaceID, sourceID).Scan(&storedKind, &storedName, &storedVersion); err != nil {
			return domain.ArtifactSet{}, artifactRepositoryError("lock artifact source", err)
		}
		if storedKind != command.Set.SourceKind || storedName != command.SourceName || storedVersion != *command.ExpectedSourceVersion {
			return domain.ArtifactSet{}, domain.ErrConflict
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_artifact_sets
(id,workspace_id,source_connection_id,source_kind,set_digest,created_by_principal_id,idempotency_key,request_fingerprint,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, setID, workspaceID, sourceID, command.Set.SourceKind, command.Set.SetDigest, creatorID, command.IdempotencyKey, command.RequestFingerprint, timestamp(command.Set.CreatedAt)); err != nil {
		return domain.ArtifactSet{}, artifactRepositoryError("create artifact set", err)
	}
	for _, member := range command.Set.Members {
		artifactID, _ := uuidValue(member.ArtifactID)
		tag, err := tx.Exec(ctx, `UPDATE source_artifacts SET source_connection_id=$3,status='validated',finalized_at=$4,expires_at=NULL
WHERE workspace_id=$1 AND id=$2 AND (source_connection_id IS NULL OR source_connection_id=$3) AND status='uploaded'
 AND content_sha256=$5 AND byte_size=$6`, workspaceID, artifactID, sourceID, timestamp(command.Set.CreatedAt), member.ContentDigest, member.ByteSize)
		if err != nil || tag.RowsAffected() != 1 {
			if err == nil {
				err = domain.ErrConflict
			}
			return domain.ArtifactSet{}, artifactRepositoryError("finalize artifact member", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO source_artifact_set_members
(workspace_id,source_connection_id,artifact_set_id,source_artifact_id,logical_path,ordinal,content_sha256,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, workspaceID, sourceID, setID, artifactID, member.LogicalPath, member.Ordinal, member.ContentDigest, timestamp(command.Set.CreatedAt)); err != nil {
			return domain.ArtifactSet{}, artifactRepositoryError("pin artifact member", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE artifact_object_retention SET state='retained',expires_at=NULL,updated_at=$3 WHERE workspace_id=$1 AND content_sha256=$2`, workspaceID, member.ContentDigest, timestamp(command.Set.CreatedAt)); err != nil {
			return domain.ArtifactSet{}, artifactRepositoryError("retain finalized artifact", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE source_connections SET active_artifact_set_id=$3,
status=CASE WHEN $5::bigint IS NULL THEN 'active' ELSE status END,
version=CASE WHEN $5::bigint IS NULL THEN version ELSE version+1 END,updated_at=$4
WHERE workspace_id=$1 AND id=$2`, workspaceID, sourceID, setID, timestamp(command.Set.CreatedAt), command.ExpectedSourceVersion); err != nil {
		return domain.ArtifactSet{}, artifactRepositoryError("activate artifact source", err)
	}
	if command.ExpectedSourceVersion != nil {
		if command.ContentRetentionUntil.IsZero() || !command.ContentRetentionUntil.After(command.Set.CreatedAt) {
			return domain.ArtifactSet{}, domain.ErrInvalid
		}
		if _, err := tx.Exec(ctx, `UPDATE artifact_object_retention r SET expires_at=GREATEST(r.expires_at,$4),updated_at=$5
FROM source_artifact_set_members old_member
JOIN source_connections source ON source.workspace_id=old_member.workspace_id
  AND source.id=old_member.source_connection_id
WHERE source.workspace_id=$1 AND source.id=$2 AND old_member.artifact_set_id<>$3
  AND old_member.content_sha256=r.content_sha256 AND r.workspace_id=$1 AND r.state='retained'
  AND NOT EXISTS (SELECT 1 FROM source_artifact_set_members current_member
    WHERE current_member.workspace_id=$1 AND current_member.artifact_set_id=$3
      AND current_member.content_sha256=old_member.content_sha256)`, workspaceID, sourceID, setID, timestamp(command.ContentRetentionUntil), timestamp(command.Set.CreatedAt)); err != nil {
			return domain.ArtifactSet{}, artifactRepositoryError("schedule superseded artifact expiry", err)
		}
	}
	if err := insertSourceAudit(ctx, tx, command.Set.WorkspaceID, "artifact_set.finalized", command.Set.CreatedByPrincipalID.String(), command.TraceID, map[string]any{"sourceId": command.Set.SourceConnectionID.String(), "artifactSetId": command.Set.ID.String(), "setDigest": command.Set.SetDigest}); err != nil {
		return domain.ArtifactSet{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ArtifactSet{}, artifactRepositoryError("commit artifact finalize", err)
	}
	return command.Set, nil
}

func (store *Store) GetArtifactSet(ctx context.Context, workspace identity.WorkspaceID, set identity.ArtifactSetID,
	expectedAuthorizationVersion int64,
) (domain.ArtifactSet, error) {
	setID, err := uuidValue(set)
	if err != nil {
		return domain.ArtifactSet{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.ArtifactSet{}, artifactRepositoryError("begin artifact set read", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := verifyArtifactAuthorizationVersion(ctx, tx, workspace, expectedAuthorizationVersion); err != nil {
		return domain.ArtifactSet{}, err
	}
	value, err := loadArtifactSet(ctx, tx, workspace, setID)
	if err != nil {
		return domain.ArtifactSet{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ArtifactSet{}, artifactRepositoryError("commit artifact set read", err)
	}
	return value, nil
}

func (store *Store) LookupArtifactSetSource(ctx context.Context, workspace identity.WorkspaceID,
	set identity.ArtifactSetID,
) (identity.SourceConnectionID, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return identity.SourceConnectionID{}, err
	}
	setID, err := uuidValue(set)
	if err != nil {
		return identity.SourceConnectionID{}, err
	}
	var sourceID pgtype.UUID
	err = store.pool.QueryRow(ctx, `SELECT source_connection_id FROM source_artifact_sets
WHERE workspace_id=$1 AND id=$2`, workspaceID, setID).Scan(&sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.SourceConnectionID{}, domain.ErrNotFound
	}
	if err != nil {
		return identity.SourceConnectionID{}, artifactRepositoryError("lookup artifact set source", err)
	}
	return identity.SourceConnectionIDFromUUIDBytes(sourceID.Bytes)
}

type artifactQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadArtifactSet(ctx context.Context, db artifactQuerier, workspace identity.WorkspaceID, setID pgtype.UUID) (domain.ArtifactSet, error) {
	workspaceID, _ := uuidValue(workspace)
	var value domain.ArtifactSet
	var id, source, creator pgtype.UUID
	err := db.QueryRow(ctx, `SELECT id,source_connection_id,source_kind,set_digest,created_by_principal_id,created_at FROM source_artifact_sets WHERE workspace_id=$1 AND id=$2`, workspaceID, setID).Scan(&id, &source, &value.SourceKind, &value.SetDigest, &creator, &value.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ArtifactSet{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ArtifactSet{}, artifactRepositoryError("get artifact set", err)
	}
	value.ID, _ = identity.ArtifactSetIDFromUUIDBytes(id.Bytes)
	value.WorkspaceID = workspace
	value.SourceConnectionID, _ = identity.SourceConnectionIDFromUUIDBytes(source.Bytes)
	value.CreatedByPrincipalID, _ = identity.PrincipalIDFromUUIDBytes(creator.Bytes)
	rows, err := db.Query(ctx, `SELECT m.source_artifact_id,m.logical_path,m.ordinal,m.content_sha256,a.byte_size,a.media_type,a.artifact_kind,
CASE WHEN r.state IN ('reserved','retained') THEN 'available' ELSE 'expired' END
FROM source_artifact_set_members m JOIN source_artifacts a ON a.workspace_id=m.workspace_id AND a.id=m.source_artifact_id
JOIN artifact_object_retention r ON r.workspace_id=m.workspace_id AND r.content_sha256=m.content_sha256
WHERE m.workspace_id=$1 AND m.artifact_set_id=$2 ORDER BY m.ordinal`, workspaceID, setID)
	if err != nil {
		return domain.ArtifactSet{}, artifactRepositoryError("list artifact set members", err)
	}
	defer rows.Close()
	for rows.Next() {
		var member domain.ArtifactMember
		var artifactID pgtype.UUID
		if err := rows.Scan(&artifactID, &member.LogicalPath, &member.Ordinal, &member.ContentDigest, &member.ByteSize, &member.MediaType, &member.Kind, &member.ContentAvailability); err != nil {
			return domain.ArtifactSet{}, err
		}
		member.ArtifactID, _ = identity.ArtifactIDFromUUIDBytes(artifactID.Bytes)
		value.Members = append(value.Members, member)
	}
	return value, rows.Err()
}

func lockArtifactWorkspace(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, expectedAuthorizationVersion int64) error {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 18004))`, workspaceID); err != nil {
		return artifactRepositoryError("lock artifact workspace", err)
	}
	var currentAuthorizationVersion int64
	if err := tx.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1 FOR UPDATE`, workspaceID).Scan(&currentAuthorizationVersion); err != nil {
		return artifactRepositoryError("lock artifact authorization version", err)
	}
	if expectedAuthorizationVersion != 0 && currentAuthorizationVersion != expectedAuthorizationVersion {
		return domain.ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workspace_artifact_usage(workspace_id) VALUES($1) ON CONFLICT DO NOTHING`, workspaceID); err != nil {
		return artifactRepositoryError("initialize artifact quota", err)
	}
	return nil
}

func verifyArtifactAuthorizationVersion(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, expectedAuthorizationVersion int64) error {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return err
	}
	var currentAuthorizationVersion int64
	if err := tx.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1`, workspaceID).Scan(&currentAuthorizationVersion); err != nil {
		return artifactRepositoryError("verify artifact authorization version", err)
	}
	if expectedAuthorizationVersion != 0 && currentAuthorizationVersion != expectedAuthorizationVersion {
		return domain.ErrConflict
	}
	return nil
}

func reserveArtifactQuota(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, size int64, now time.Time) error {
	var count int
	var bytes int64
	if err := tx.QueryRow(ctx, `SELECT active_object_count,active_bytes FROM workspace_artifact_usage WHERE workspace_id=$1 FOR UPDATE`, workspaceID).Scan(&count, &bytes); err != nil {
		return artifactRepositoryError("lock artifact quota", err)
	}
	if count >= domain.MaxWorkspaceObjects || bytes > domain.MaxWorkspaceBytes-size {
		return domain.ErrLimitExceeded
	}
	if _, err := tx.Exec(ctx, `UPDATE workspace_artifact_usage SET active_object_count=active_object_count+1,active_bytes=active_bytes+$2,updated_at=$3 WHERE workspace_id=$1`, workspaceID, size, timestamp(now)); err != nil {
		return artifactRepositoryError("reserve artifact quota", err)
	}
	return nil
}

func validateArtifactSource(ctx context.Context, tx pgx.Tx, command application.CreateArtifactCommand) error {
	if command.Artifact.SourceConnectionID == nil {
		if command.ExpectedSourceVersion != nil {
			return domain.ErrInvalid
		}
		return nil
	}
	if command.ExpectedSourceVersion == nil || *command.ExpectedSourceVersion < 1 {
		return domain.ErrInvalid
	}
	workspaceID, _ := uuidValue(command.Artifact.WorkspaceID)
	sourceID, _ := uuidValue(*command.Artifact.SourceConnectionID)
	var sourceKind string
	var version int64
	if err := tx.QueryRow(ctx, `SELECT source_kind,version FROM source_connections
WHERE workspace_id=$1 AND id=$2 AND status IN ('active','paused') FOR UPDATE`, workspaceID, sourceID).Scan(&sourceKind, &version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrConflict
		}
		return artifactRepositoryError("lock artifact source", err)
	}
	if version != *command.ExpectedSourceVersion || !artifactKindMatchesSource(command.Artifact.Kind, sourceKind) {
		return domain.ErrConflict
	}
	return nil
}

func artifactKindMatchesSource(kind domain.ArtifactKind, sourceKind string) bool {
	switch sourceKind {
	case "file":
		return kind == domain.ArtifactCSV || kind == domain.ArtifactXLSX || kind == domain.ArtifactMarkdown
	case "sql_bundle":
		return kind == domain.ArtifactSQL
	case "dbt_bundle":
		return kind == domain.ArtifactDBTManifest || kind == domain.ArtifactDBTCatalog
	default:
		return false
	}
}

func artifactSourceIDString(value *identity.SourceConnectionID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func findArtifactInTx(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, key string) (domain.Artifact, error) {
	workspaceID, _ := uuidValue(workspace)
	return scanArtifact(tx.QueryRow(ctx, `SELECT `+artifactColumns+` FROM source_artifacts WHERE workspace_id=$1 AND idempotency_key=$2`, workspaceID, key))
}
func artifactDatabaseIDs(value domain.Artifact) (pgtype.UUID, pgtype.UUID, pgtype.UUID, error) {
	workspace, err := uuidValue(value.WorkspaceID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	id, err := uuidValue(value.ID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	if value.UploadedByPrincipalID == nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, domain.ErrInvalid
	}
	uploader, err := uuidValue(*value.UploadedByPrincipalID)
	return workspace, id, uploader, err
}

func scanArtifact(row pgx.Row) (domain.Artifact, error) {
	return scanArtifactValue(row, false)
}

func scanArtifactRead(row pgx.Row) (domain.Artifact, error) {
	return scanArtifactValue(row, true)
}

func scanArtifactValue(row pgx.Row, readAvailability bool) (domain.Artifact, error) {
	var value domain.Artifact
	var id, workspace, source, uploader pgtype.UUID
	var digest, failure pgtype.Text
	var expires, finalized pgtype.Timestamptz
	var logicalPath pgtype.Text
	var validationSummary []byte
	values := []any{&id, &workspace, &source, &value.Kind, &value.SchemaVersion, &digest, &value.ByteSize, &value.MediaType, &value.OriginalName, &logicalPath, &validationSummary, &value.Status, &failure, &uploader, &value.IdempotencyKey, &value.RequestFingerprint, &expires, &value.CreatedAt, &finalized}
	if readAvailability {
		values = append(values, &value.ContentAvailability)
	}
	err := row.Scan(values...)
	if err != nil {
		return domain.Artifact{}, err
	}
	value.ID, _ = identity.ArtifactIDFromUUIDBytes(id.Bytes)
	value.WorkspaceID, _ = identity.WorkspaceIDFromUUIDBytes(workspace.Bytes)
	if source.Valid {
		v, _ := identity.SourceConnectionIDFromUUIDBytes(source.Bytes)
		value.SourceConnectionID = &v
	}
	value.ContentDigest = digest.String
	if !readAvailability {
		if digest.Valid {
			value.ContentAvailability = domain.ContentAvailable
		} else {
			value.ContentAvailability = domain.ContentNotStored
		}
	}
	value.LogicalPath = logicalPath.String
	value.FailureCode = failure.String
	if len(validationSummary) > 0 && string(validationSummary) != "{}" {
		var summary domain.ArtifactValidationSummary
		if err := json.Unmarshal(validationSummary, &summary); err != nil {
			return domain.Artifact{}, artifactRepositoryError("decode artifact validation summary", err)
		}
		value.ValidationSummary = &summary
	}
	if uploader.Valid {
		v, _ := identity.PrincipalIDFromUUIDBytes(uploader.Bytes)
		value.UploadedByPrincipalID = &v
	}
	if finalized.Valid {
		v := finalized.Time.UTC()
		value.FinalizedAt = &v
	}
	if expires.Valid {
		v := expires.Time.UTC()
		value.ExpiresAt = &v
	}
	return value, nil
}

type artifactCursor struct {
	Workspace            string                `json:"w"`
	Principal            string                `json:"p"`
	AuthorizationVersion int64                 `json:"a"`
	Kind                 domain.ArtifactKind   `json:"k"`
	Status               domain.ArtifactStatus `json:"s"`
	Source               string                `json:"o"`
	CreatedAt            *time.Time            `json:"t"`
	ID                   *pgtype.UUID          `json:"i"`
}

func decodeArtifactCursor(query application.ListArtifactsQuery) (artifactCursor, error) {
	if query.Cursor == "" {
		return artifactCursor{Workspace: query.WorkspaceID.String(), Principal: query.PrincipalID.String(), AuthorizationVersion: query.AuthorizationVersion, Kind: query.Kind, Status: query.Status, Source: artifactSourceIDString(query.SourceID)}, nil
	}
	if len(query.Cursor) > 4096 {
		return artifactCursor{}, domain.ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(query.Cursor)
	if err != nil {
		return artifactCursor{}, domain.ErrInvalid
	}
	var value artifactCursor
	if json.Unmarshal(raw, &value) != nil || value.Workspace != query.WorkspaceID.String() || value.Principal != query.PrincipalID.String() || value.AuthorizationVersion != query.AuthorizationVersion || value.Kind != query.Kind || value.Status != query.Status || value.Source != artifactSourceIDString(query.SourceID) || value.CreatedAt == nil || value.ID == nil || !value.ID.Valid {
		return artifactCursor{}, domain.ErrInvalid
	}
	return value, nil
}
func encodeArtifactCursor(query application.ListArtifactsQuery, value domain.Artifact) string {
	id, _ := uuidValue(value.ID)
	created := value.CreatedAt.UTC()
	raw, _ := json.Marshal(artifactCursor{Workspace: query.WorkspaceID.String(), Principal: query.PrincipalID.String(), AuthorizationVersion: query.AuthorizationVersion, Kind: query.Kind, Status: query.Status, Source: artifactSourceIDString(query.SourceID), CreatedAt: &created, ID: &id})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func artifactRepositoryError(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrLimitExceeded) {
		return err
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return domain.ErrConflict
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

var _ = strings.TrimSpace
