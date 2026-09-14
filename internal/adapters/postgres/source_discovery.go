package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	application "github.com/iiwish/semlia/internal/application/discovery"
	domain "github.com/iiwish/semlia/internal/domain/discovery"
	ingestiondomain "github.com/iiwish/semlia/internal/domain/ingestion"
	operationsdomain "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ application.SourceRepository = (*Store)(nil)

var (
	errActiveDiscoveryRun    = errors.New("active discovery run")
	errSourceUnavailable     = errors.New("discovery source unavailable")
	errCredentialUnavailable = errors.New("discovery credential unavailable")
	errArtifactUnavailable   = errors.New("discovery artifact set unavailable")
)

const discoverySourceColumns = `id, workspace_id, source_kind, name, metadata, artifact_paths,
status, active_credential_version, active_artifact_set_id, version, created_at, updated_at`

func (store *Store) CreateDiscoverySource(ctx context.Context, command application.CreateSourceCommand) (domain.Source, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Source{}, sourceRepositoryError("begin source create", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	workspaceID, sourceID, credentialID, err := sourceDatabaseIDs(command.Source.WorkspaceID, command.Source.ID, command.Credential.ID)
	if err != nil {
		return domain.Source{}, err
	}
	if err := lockDiscoveryWorkspace(ctx, tx, command.Source.WorkspaceID, command.ExpectedAuthorizationVersion); err != nil {
		return domain.Source{}, err
	}
	metadata, _ := json.Marshal(map[string]any{"host": command.Source.Host, "port": command.Source.Port,
		"database": command.Source.Database, "username": command.Source.Username, "sslMode": command.Source.SSLMode})
	paths, _ := json.Marshal(command.Source.ArtifactPaths)
	row := tx.QueryRow(ctx, `INSERT INTO source_connections
(id, workspace_id, adapter_kind, name, normalized_locator, credential_ref, status, metadata,
 active_credential_version, artifact_paths, created_at, updated_at)
VALUES ($1,$2,'postgresql_catalog',$3,$4,'encrypted:v1',$5,$6,$7,$8,$9,$9)
RETURNING `+discoverySourceColumns,
		sourceID, workspaceID, command.Source.Name, normalizedSourceLocator(command.Source), command.Source.Status,
		metadata, command.Source.ActiveCredentialVersion, paths, timestamp(command.Source.CreatedAt))
	created, err := scanDiscoverySource(row)
	if err != nil {
		return domain.Source{}, sourceRepositoryError("create source", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_credentials
(id, workspace_id, source_connection_id, version, key_version, algorithm, nonce, ciphertext, created_by, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, credentialID, workspaceID, sourceID, command.Credential.Version,
		command.Credential.KeyVersion, command.Credential.Algorithm, command.Credential.Nonce,
		command.Credential.Ciphertext, actorOrSystem(command.Credential.CreatedBy), timestamp(command.Credential.CreatedAt)); err != nil {
		return domain.Source{}, sourceRepositoryError("create source credential", err)
	}
	if err := insertSourceAudit(ctx, tx, command.Source.WorkspaceID, "source.created", command.Credential.CreatedBy,
		command.TraceID, map[string]any{"sourceId": command.Source.ID.String(), "credentialVersion": 1}); err != nil {
		return domain.Source{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Source{}, sourceRepositoryError("commit source create", err)
	}
	return created, nil
}

func (store *Store) GetDiscoverySource(ctx context.Context, workspace identity.WorkspaceID, source identity.SourceConnectionID) (domain.Source, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return domain.Source{}, err
	}
	sourceID, err := uuidValue(source)
	if err != nil {
		return domain.Source{}, err
	}
	return scanDiscoverySource(store.pool.QueryRow(ctx, `SELECT `+discoverySourceColumns+` FROM source_connections WHERE workspace_id=$1 AND id=$2`, workspaceID, sourceID))
}

func (store *Store) GetDiscoverySourceAuthorized(ctx context.Context, workspace identity.WorkspaceID,
	source identity.SourceConnectionID, expectedAuthorizationVersion int64,
) (domain.Source, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return domain.Source{}, err
	}
	sourceID, err := uuidValue(source)
	if err != nil {
		return domain.Source{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.Source{}, sourceRepositoryError("begin source read", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := verifyDiscoveryAuthorizationVersion(ctx, tx, workspace, expectedAuthorizationVersion); err != nil {
		return domain.Source{}, err
	}
	value, err := scanDiscoverySource(tx.QueryRow(ctx, `SELECT `+discoverySourceColumns+` FROM source_connections WHERE workspace_id=$1 AND id=$2`, workspaceID, sourceID))
	if err != nil {
		return domain.Source{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Source{}, sourceRepositoryError("commit source read", err)
	}
	return value, nil
}

func (store *Store) ListDiscoverySources(ctx context.Context, query application.ListSourcesQuery) (domain.SourcePage, error) {
	workspaceID, err := uuidValue(query.WorkspaceID)
	if err != nil {
		return domain.SourcePage{}, err
	}
	cursor, err := decodeSourceCursor(query)
	if err != nil {
		return domain.SourcePage{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.SourcePage{}, sourceRepositoryError("begin source list", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := verifyDiscoveryAuthorizationVersion(ctx, tx, query.WorkspaceID, query.AuthorizationVersion); err != nil {
		return domain.SourcePage{}, err
	}
	var sourceFilter any
	if query.SourceID != nil {
		sourceFilter, err = uuidValue(*query.SourceID)
		if err != nil {
			return domain.SourcePage{}, err
		}
	}
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM source_connections WHERE workspace_id=$1 AND ($2::uuid IS NULL OR id=$2)`, workspaceID, sourceFilter).Scan(&total); err != nil {
		return domain.SourcePage{}, sourceRepositoryError("count sources", err)
	}
	rows, err := tx.Query(ctx, `SELECT `+discoverySourceColumns+` FROM source_connections WHERE workspace_id=$1
AND ($2::uuid IS NULL OR id=$2) AND ($3::timestamptz IS NULL OR (updated_at,id)<($3,$4)) ORDER BY updated_at DESC,id DESC LIMIT $5`,
		workspaceID, sourceFilter, cursor.UpdatedAt, cursor.ID, query.Limit+1)
	if err != nil {
		return domain.SourcePage{}, sourceRepositoryError("list sources", err)
	}
	defer rows.Close()
	result := make([]domain.Source, 0, query.Limit+1)
	for rows.Next() {
		value, scanErr := scanDiscoverySource(rows)
		if scanErr != nil {
			return domain.SourcePage{}, scanErr
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return domain.SourcePage{}, err
	}
	next := ""
	if len(result) > query.Limit {
		result = result[:query.Limit]
		next = encodeSourceCursor(query, result[len(result)-1])
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.SourcePage{}, sourceRepositoryError("commit source list", err)
	}
	return domain.SourcePage{Items: result, Total: total, Limit: query.Limit, NextCursor: next}, nil
}

func (store *Store) UpdateDiscoverySource(ctx context.Context, command application.UpdateSourceCommand) (domain.Source, error) {
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return domain.Source{}, err
	}
	sourceID, err := uuidValue(command.SourceID)
	if err != nil {
		return domain.Source{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Source{}, sourceRepositoryError("begin source update", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockDiscoveryWorkspace(ctx, tx, command.WorkspaceID, command.ExpectedAuthorizationVersion); err != nil {
		return domain.Source{}, err
	}
	var paths []byte
	if command.ArtifactPaths != nil {
		paths, _ = json.Marshal(command.ArtifactPaths)
	}
	row := tx.QueryRow(ctx, `UPDATE source_connections SET name=$3, artifact_paths=COALESCE($4,artifact_paths), status=$5,
version=version+1, updated_at=$6
WHERE workspace_id=$1 AND id=$2 AND version=$7
RETURNING `+discoverySourceColumns,
		workspaceID, sourceID, command.Name, paths, command.Status, timestamp(command.UpdatedAt), command.ExpectedVersion)
	value, err := scanDiscoverySource(row)
	if err != nil {
		return domain.Source{}, sourceRepositoryError("update source", err)
	}
	if command.Status == "deleted" {
		if command.ContentRetentionUntil.IsZero() {
			return domain.Source{}, domain.ErrInvalidInput
		}
		if _, err := tx.Exec(ctx, `UPDATE artifact_object_retention r SET expires_at=GREATEST(r.expires_at,$3),updated_at=$4
FROM source_artifact_set_members member
WHERE member.workspace_id=$1 AND member.source_connection_id=$2
  AND member.content_sha256=r.content_sha256 AND r.workspace_id=$1 AND r.state='retained'`,
			workspaceID, sourceID, timestamp(command.ContentRetentionUntil), timestamp(command.UpdatedAt)); err != nil {
			return domain.Source{}, sourceRepositoryError("schedule deleted source artifact expiry", err)
		}
	}
	if err := insertSourceAudit(ctx, tx, command.WorkspaceID, "source.updated", command.Actor, command.TraceID,
		map[string]any{"sourceId": command.SourceID.String(), "status": command.Status, "version": value.Version}); err != nil {
		return domain.Source{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Source{}, sourceRepositoryError("commit source update", err)
	}
	return value, nil
}

func (store *Store) RotateDiscoveryCredential(ctx context.Context, command application.RotateCredentialCommand) (domain.Source, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Source{}, sourceRepositoryError("begin credential rotation", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	workspaceID, sourceID, credentialID, err := sourceDatabaseIDs(command.WorkspaceID, command.SourceID, command.Credential.ID)
	if err != nil {
		return domain.Source{}, err
	}
	if err := lockDiscoveryWorkspace(ctx, tx, command.WorkspaceID, command.ExpectedAuthorizationVersion); err != nil {
		return domain.Source{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_credentials
(id, workspace_id, source_connection_id, version, key_version, algorithm, nonce, ciphertext, created_by, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, credentialID, workspaceID, sourceID, command.Credential.Version,
		command.Credential.KeyVersion, command.Credential.Algorithm, command.Credential.Nonce,
		command.Credential.Ciphertext, actorOrSystem(command.Credential.CreatedBy), timestamp(command.Credential.CreatedAt)); err != nil {
		return domain.Source{}, sourceRepositoryError("insert rotated credential", err)
	}
	row := tx.QueryRow(ctx, `UPDATE source_connections SET active_credential_version=$4, version=version+1, updated_at=$5
WHERE workspace_id=$1 AND id=$2 AND active_credential_version=$3 AND version=$6 AND status <> 'deleted'
RETURNING `+discoverySourceColumns,
		workspaceID, sourceID, command.ExpectedVersion, command.Credential.Version, timestamp(command.UpdatedAt), command.ExpectedSourceVersion)
	source, err := scanDiscoverySource(row)
	if err != nil {
		return domain.Source{}, sourceRepositoryError("activate rotated credential", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE source_credentials SET retired_at=$3 WHERE workspace_id=$1 AND source_connection_id=$2 AND version=$4`,
		workspaceID, sourceID, timestamp(command.UpdatedAt), command.ExpectedVersion); err != nil {
		return domain.Source{}, sourceRepositoryError("retire source credential", err)
	}
	if err := insertSourceAudit(ctx, tx, command.WorkspaceID, "source.credential_rotated", command.Credential.CreatedBy,
		command.TraceID, map[string]any{"sourceId": command.SourceID.String(), "credentialVersion": command.Credential.Version}); err != nil {
		return domain.Source{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Source{}, sourceRepositoryError("commit credential rotation", err)
	}
	return source, nil
}

func (store *Store) LoadDiscoveryCredential(ctx context.Context, workspace identity.WorkspaceID, source identity.SourceConnectionID, version int64) (domain.CredentialEnvelope, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return domain.CredentialEnvelope{}, err
	}
	sourceID, err := uuidValue(source)
	if err != nil {
		return domain.CredentialEnvelope{}, err
	}
	var id pgtype.UUID
	var result domain.CredentialEnvelope
	err = store.pool.QueryRow(ctx, `SELECT id, key_version, algorithm, nonce, ciphertext, created_by, created_at
FROM source_credentials WHERE workspace_id=$1 AND source_connection_id=$2 AND version=$3`, workspaceID, sourceID, version).
		Scan(&id, &result.KeyVersion, &result.Algorithm, &result.Nonce, &result.Ciphertext, &result.CreatedBy, &result.CreatedAt)
	if err != nil {
		return domain.CredentialEnvelope{}, sourceRepositoryError("load source credential", err)
	}
	result.ID, err = identity.SourceCredentialIDFromUUIDBytes(id.Bytes)
	if err != nil {
		return domain.CredentialEnvelope{}, err
	}
	result.WorkspaceID, result.SourceConnectionID, result.Version = workspace, source, version
	return result, nil
}

func (store *Store) LoadDiscoverySourceCredentialAuthorized(ctx context.Context, workspace identity.WorkspaceID,
	source identity.SourceConnectionID, expectedAuthorizationVersion int64,
) (domain.Source, domain.CredentialEnvelope, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return domain.Source{}, domain.CredentialEnvelope{}, err
	}
	sourceID, err := uuidValue(source)
	if err != nil {
		return domain.Source{}, domain.CredentialEnvelope{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.Source{}, domain.CredentialEnvelope{}, sourceRepositoryError("begin source credential read", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := verifyDiscoveryAuthorizationVersion(ctx, tx, workspace, expectedAuthorizationVersion); err != nil {
		return domain.Source{}, domain.CredentialEnvelope{}, err
	}
	value, err := scanDiscoverySource(tx.QueryRow(ctx, `SELECT `+discoverySourceColumns+`
FROM source_connections WHERE workspace_id=$1 AND id=$2 AND source_kind='postgresql' AND status <> 'deleted'`, workspaceID, sourceID))
	if err != nil {
		return domain.Source{}, domain.CredentialEnvelope{}, err
	}
	var credentialID pgtype.UUID
	var envelope domain.CredentialEnvelope
	err = tx.QueryRow(ctx, `SELECT id, key_version, algorithm, nonce, ciphertext, created_by, created_at
FROM source_credentials
WHERE workspace_id=$1 AND source_connection_id=$2 AND version=$3 AND retired_at IS NULL`,
		workspaceID, sourceID, value.ActiveCredentialVersion).
		Scan(&credentialID, &envelope.KeyVersion, &envelope.Algorithm, &envelope.Nonce, &envelope.Ciphertext,
			&envelope.CreatedBy, &envelope.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Source{}, domain.CredentialEnvelope{}, domain.ErrCredential
	}
	if err != nil {
		return domain.Source{}, domain.CredentialEnvelope{}, sourceRepositoryError("load authorized source credential", err)
	}
	envelope.ID, err = identity.SourceCredentialIDFromUUIDBytes(credentialID.Bytes)
	if err != nil {
		return domain.Source{}, domain.CredentialEnvelope{}, err
	}
	envelope.WorkspaceID, envelope.SourceConnectionID, envelope.Version = workspace, source, value.ActiveCredentialVersion
	if err := tx.Commit(ctx); err != nil {
		return domain.Source{}, domain.CredentialEnvelope{}, sourceRepositoryError("commit source credential read", err)
	}
	return value, envelope, nil
}

func (store *Store) CreateQueuedDiscoveryRun(ctx context.Context, command application.CreateRunCommand) (domain.Run, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Run{}, sourceRepositoryError("begin discovery run", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := store.createQueuedDiscoveryRunTx(ctx, tx, command, enqueueIntent{kind: "manual"})
	if errors.Is(err, errActiveDiscoveryRun) || errors.Is(err, errSourceUnavailable) ||
		errors.Is(err, errCredentialUnavailable) || errors.Is(err, errArtifactUnavailable) {
		err = domain.ErrConflict
	}
	if err != nil {
		return domain.Run{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Run{}, sourceRepositoryError("commit discovery run", err)
	}
	return result, nil
}

type enqueueIntent struct {
	kind                 string
	identity             string
	skipAuthorization    bool
	serverIdempotencyKey string
	credentialVersion    *int64
}

func (store *Store) createQueuedDiscoveryRunTx(ctx context.Context, tx pgx.Tx, command application.CreateRunCommand, intent enqueueIntent) (domain.Run, error) {
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return domain.Run{}, err
	}
	sourceID, err := uuidValue(command.SourceID)
	if err != nil {
		return domain.Run{}, err
	}
	runID, err := uuidValue(command.RunID)
	if err != nil {
		return domain.Run{}, err
	}
	jobID, err := uuidValue(command.JobID)
	if err != nil {
		return domain.Run{}, err
	}
	if !intent.skipAuthorization {
		if err := lockDiscoveryWorkspace(ctx, tx, command.WorkspaceID, command.ExpectedAuthorizationVersion); err != nil {
			return domain.Run{}, err
		}
	}
	requestFingerprint := "sha256:" + runtimeDigest(command.SourceID.String(), strings.TrimSpace(command.RequestedBy), intent.kind, intent.identity)
	jobIdempotencyKey := intent.serverIdempotencyKey
	if jobIdempotencyKey == "" {
		jobIdempotencyKey = boundedRuntimeIdempotencyKey("manual:", command.IdempotencyKey)
	}
	var existingRun pgtype.UUID
	var existingFingerprint pgtype.Text
	lookupErr := tx.QueryRow(ctx, `SELECT r.id,r.request_fingerprint FROM jobs j JOIN discovery_runs r ON r.job_id=j.id
WHERE j.workspace_id=$1 AND j.idempotency_key=$2`, workspaceID, jobIdempotencyKey).Scan(&existingRun, &existingFingerprint)
	if lookupErr == nil {
		if !existingFingerprint.Valid || existingFingerprint.String != requestFingerprint {
			return domain.Run{}, domain.ErrConflict
		}
		result, getErr := scanDiscoveryRun(tx.QueryRow(ctx, discoveryRunSelect+` WHERE r.workspace_id=$1 AND r.id=$2`, workspaceID, existingRun), true)
		return result, getErr
	}
	if !errors.Is(lookupErr, pgx.ErrNoRows) {
		return domain.Run{}, sourceRepositoryError("check discovery idempotency", lookupErr)
	}
	var sourceKind, sourceStatus string
	var credentialVersion pgtype.Int8
	var artifactSetID pgtype.UUID
	var sourceMetadata, sourceArtifactPaths []byte
	var sourceVersion int64
	if err := tx.QueryRow(ctx, `SELECT source_kind,status,active_credential_version,active_artifact_set_id,metadata,artifact_paths,version
FROM source_connections WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, workspaceID, sourceID).
		Scan(&sourceKind, &sourceStatus, &credentialVersion, &artifactSetID, &sourceMetadata, &sourceArtifactPaths, &sourceVersion); err != nil {
		return domain.Run{}, sourceRepositoryError("lock discovery run source", err)
	}
	if sourceStatus != "active" {
		return domain.Run{}, errSourceUnavailable
	}
	if sourceKind == "postgresql" && !credentialVersion.Valid {
		return domain.Run{}, errCredentialUnavailable
	}
	if sourceKind == "postgresql" && intent.credentialVersion != nil {
		var available bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_credentials
WHERE workspace_id=$1 AND source_connection_id=$2 AND version=$3 AND retired_at IS NULL)`,
			workspaceID, sourceID, *intent.credentialVersion).Scan(&available); err != nil {
			return domain.Run{}, sourceRepositoryError("verify pinned discovery credential", err)
		}
		if !available {
			return domain.Run{}, errCredentialUnavailable
		}
		credentialVersion = pgtype.Int8{Int64: *intent.credentialVersion, Valid: true}
	}
	if sourceKind != "postgresql" && !artifactSetID.Valid {
		return domain.Run{}, errArtifactUnavailable
	}
	var activeRun bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM discovery_runs
WHERE workspace_id=$1 AND source_connection_id=$2 AND status IN('queued','running'))`, workspaceID, sourceID).Scan(&activeRun); err != nil {
		return domain.Run{}, sourceRepositoryError("check active discovery run", err)
	}
	if activeRun {
		return domain.Run{}, errActiveDiscoveryRun
	}
	inputVersion := ""
	if credentialVersion.Valid {
		inputVersion = strconv.FormatInt(credentialVersion.Int64, 10)
	} else {
		inputVersion = artifactSetID.String()
	}
	sourceConfig, _ := json.Marshal(map[string]any{"metadata": json.RawMessage(sourceMetadata), "artifactPaths": json.RawMessage(sourceArtifactPaths), "sourceVersion": sourceVersion})
	sourceInputFingerprint := "sha256:" + runtimeDigest(command.SourceID.String(), sourceKind, inputVersion, string(sourceConfig))
	payload, _ := json.Marshal(map[string]string{"runId": command.RunID.String()})
	if _, err := tx.Exec(ctx, `INSERT INTO jobs
(id, workspace_id, job_type, payload, max_attempts, available_at, idempotency_key, trace_id, created_at, updated_at)
VALUES ($1,$2,$3,$4,3,$5,$6,$7,$5,$5)`, jobID, workspaceID, application.DiscoveryJobType, payload,
		timestamp(command.CreatedAt), jobIdempotencyKey, command.TraceID); err != nil {
		return domain.Run{}, sourceRepositoryError("enqueue discovery job", err)
	}
	adapterVersion := map[string]string{"postgresql": "postgresql_catalog/" + LiveVersion, "file": "file_catalog/1.0.0", "sql_bundle": "postgresql_sql/1.0.0", "dbt_bundle": "dbt/1.0.0"}[sourceKind]
	row := tx.QueryRow(ctx, `INSERT INTO discovery_runs
(id, workspace_id, source_connection_id, adapter_version, status, credential_version, artifact_set_id,
	 request_fingerprint, source_input_fingerprint, source_config, job_id, requested_by, trace_id, created_at, updated_at)
	VALUES ($1,$2,$3,$4,'queued',$5,$6,$7,$8,$9,$10,$11,$12,$13,$13) RETURNING `+discoveryRunColumns,
		runID, workspaceID, sourceID, adapterVersion, credentialVersion, artifactSetID, requestFingerprint, sourceInputFingerprint, sourceConfig, jobID,
		actorOrSystem(command.RequestedBy), command.TraceID, timestamp(command.CreatedAt))
	result, err := scanDiscoveryRun(row)
	if err != nil {
		return domain.Run{}, sourceRepositoryError("create queued discovery run", err)
	}
	if artifactSetID.Valid {
		if _, err := tx.Exec(ctx, `INSERT INTO discovery_run_artifacts
(workspace_id,source_connection_id,discovery_run_id,artifact_set_id,source_artifact_id,logical_path,ordinal,content_sha256,created_at)
SELECT workspace_id,source_connection_id,$4,artifact_set_id,source_artifact_id,logical_path,ordinal,content_sha256,$5
FROM source_artifact_set_members WHERE workspace_id=$1 AND source_connection_id=$2 AND artifact_set_id=$3
ORDER BY ordinal`, workspaceID, sourceID, artifactSetID, runID, timestamp(command.CreatedAt)); err != nil {
			return domain.Run{}, sourceRepositoryError("pin discovery run artifacts", err)
		}
	}
	runtimeEvent, err := newRuntimeEvent(command.WorkspaceID, command.RunID, "queued", operationsdomain.RunEventState,
		operationsdomain.RunQueued, "queued", "", "", command.CreatedAt)
	if err != nil {
		return domain.Run{}, err
	}
	if err := projectRuntime(ctx, dbgen.New(tx), operationsdomain.RuntimeRun{ID: command.RunID, WorkspaceID: command.WorkspaceID,
		Kind: operationsdomain.RunKindDiscovery, SourceType: "discovery_run", SourceID: command.RunID.String(),
		SourceVersionDigest: strings.TrimPrefix(sourceInputFingerprint, "sha256:"),
		JobID:               &command.JobID, TraceID: command.TraceID,
		IdempotencyKey:         boundedRuntimeIdempotencyKey("runtime:discovery:", command.RunID.String()),
		RequestedByPrincipalID: principalIDPointer(command.RequestedBy), State: operationsdomain.RunQueued,
		Phase: "queued", MaxAttempts: 3, Version: 1, CreatedAt: command.CreatedAt.UTC(), UpdatedAt: command.CreatedAt.UTC()}, runtimeEvent); err != nil {
		return domain.Run{}, sourceRepositoryError("project queued discovery run", err)
	}
	if err := insertSourceAudit(ctx, tx, command.WorkspaceID, "discovery.queued", command.RequestedBy, command.TraceID,
		map[string]any{"sourceId": command.SourceID.String(), "runId": command.RunID.String(), "inputFingerprint": sourceInputFingerprint}); err != nil {
		return domain.Run{}, err
	}
	return result, nil
}

func (store *Store) ListDiscoveryRuns(ctx context.Context, query application.ListRunsQuery) (domain.RunPage, error) {
	cursor, err := decodeDiscoveryRunCursor(query)
	if err != nil {
		return domain.RunPage{}, err
	}
	workspaceID, err := uuidValue(query.WorkspaceID)
	if err != nil {
		return domain.RunPage{}, err
	}
	sourceID, err := uuidValue(query.SourceID)
	if err != nil {
		return domain.RunPage{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.RunPage{}, sourceRepositoryError("begin discovery run list", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := verifyDiscoveryAuthorizationVersion(ctx, tx, query.WorkspaceID, query.AuthorizationVersion); err != nil {
		return domain.RunPage{}, err
	}
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM discovery_runs WHERE workspace_id=$1 AND source_connection_id=$2`, workspaceID, sourceID).Scan(&total); err != nil {
		return domain.RunPage{}, sourceRepositoryError("count discovery runs", err)
	}
	rows, err := tx.Query(ctx, discoveryRunSelect+` WHERE r.workspace_id=$1 AND r.source_connection_id=$2
AND ($3::timestamptz IS NULL OR (r.created_at,r.id)<($3,$4)) ORDER BY r.created_at DESC,r.id DESC LIMIT $5`,
		workspaceID, sourceID, cursor.CreatedAt, cursor.ID, query.Limit+1)
	if err != nil {
		return domain.RunPage{}, sourceRepositoryError("list discovery runs", err)
	}
	defer rows.Close()
	result := make([]domain.Run, 0, query.Limit+1)
	for rows.Next() {
		value, scanErr := scanDiscoveryRun(rows, true)
		if scanErr != nil {
			return domain.RunPage{}, scanErr
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return domain.RunPage{}, err
	}
	next := ""
	if len(result) > query.Limit {
		result = result[:query.Limit]
		next = encodeDiscoveryRunCursor(query, result[len(result)-1])
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.RunPage{}, sourceRepositoryError("commit discovery run list", err)
	}
	return domain.RunPage{Items: result, Total: total, Limit: query.Limit, NextCursor: next}, nil
}

type discoveryRunCursor struct {
	Workspace            string       `json:"w"`
	Source               string       `json:"s"`
	Principal            string       `json:"p"`
	AuthorizationVersion int64        `json:"a"`
	CreatedAt            *time.Time   `json:"t"`
	ID                   *pgtype.UUID `json:"i"`
}

func decodeDiscoveryRunCursor(query application.ListRunsQuery) (discoveryRunCursor, error) {
	if strings.TrimSpace(query.Cursor) == "" {
		return discoveryRunCursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(query.Cursor)
	if err != nil || len(decoded) > 4096 {
		return discoveryRunCursor{}, domain.ErrInvalidInput
	}
	var cursor discoveryRunCursor
	if json.Unmarshal(decoded, &cursor) != nil || cursor.Workspace != query.WorkspaceID.String() ||
		cursor.Source != query.SourceID.String() || cursor.Principal != query.PrincipalID.String() ||
		cursor.AuthorizationVersion != query.AuthorizationVersion || cursor.CreatedAt == nil || cursor.ID == nil || !cursor.ID.Valid {
		return discoveryRunCursor{}, domain.ErrInvalidInput
	}
	return cursor, nil
}

func encodeDiscoveryRunCursor(query application.ListRunsQuery, value domain.Run) string {
	id, _ := uuidValue(value.ID)
	createdAt := value.CreatedAt.UTC()
	encoded, _ := json.Marshal(discoveryRunCursor{Workspace: query.WorkspaceID.String(), Source: query.SourceID.String(),
		Principal: query.PrincipalID.String(), AuthorizationVersion: query.AuthorizationVersion, CreatedAt: &createdAt, ID: &id})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

const LiveVersion = "1.0.0"
const discoveryRunColumns = `id, workspace_id, source_connection_id, credential_version, artifact_set_id, request_fingerprint, source_input_fingerprint, source_config, job_id, status,
error_code, stats, requested_by, trace_id, started_at, completed_at, created_at, updated_at`
const discoveryRunSelect = `SELECT ` + discoveryRunColumns + `, (SELECT snapshot_id FROM source_snapshot_runs ss WHERE ss.workspace_id=r.workspace_id AND ss.run_id=r.id) FROM discovery_runs r`

func (store *Store) LoadDiscoveryRunExecution(ctx context.Context, workspace identity.WorkspaceID, run identity.RunID) (application.RunExecution, error) {
	runValue, err := store.getControlDiscoveryRun(ctx, workspace, run)
	if err != nil {
		return application.RunExecution{}, err
	}
	source, err := store.GetDiscoverySource(ctx, workspace, runValue.SourceConnectionID)
	if err != nil {
		return application.RunExecution{}, err
	}
	result := application.RunExecution{Run: runValue, Source: source}
	if source.Kind == "postgresql" {
		if err := applyPinnedSourceConfig(&source, runValue.SourceConfig); err != nil {
			return application.RunExecution{}, err
		}
		result.Source = source
		credential, err := store.LoadDiscoveryCredential(ctx, workspace, source.ID, runValue.CredentialVersion)
		if err != nil {
			return application.RunExecution{}, err
		}
		result.Credential = credential
		return result, nil
	}
	workspaceID, _ := uuidValue(workspace)
	runID, _ := uuidValue(run)
	rows, err := store.pool.Query(ctx, `SELECT p.source_artifact_id,p.logical_path,p.ordinal,p.content_sha256,
a.byte_size,a.media_type,a.artifact_kind FROM discovery_run_artifacts p
JOIN source_artifacts a ON a.workspace_id=p.workspace_id AND a.id=p.source_artifact_id
WHERE p.workspace_id=$1 AND p.discovery_run_id=$2 ORDER BY p.ordinal`, workspaceID, runID)
	if err != nil {
		return application.RunExecution{}, sourceRepositoryError("load discovery run artifacts", err)
	}
	defer rows.Close()
	for rows.Next() {
		var member ingestiondomain.ArtifactMember
		var artifactID pgtype.UUID
		if err := rows.Scan(&artifactID, &member.LogicalPath, &member.Ordinal, &member.ContentDigest, &member.ByteSize, &member.MediaType, &member.Kind); err != nil {
			return application.RunExecution{}, sourceRepositoryError("scan discovery run artifact", err)
		}
		member.ArtifactID, _ = identity.ArtifactIDFromUUIDBytes(artifactID.Bytes)
		result.Artifacts = append(result.Artifacts, member)
	}
	if err := rows.Err(); err != nil || len(result.Artifacts) == 0 {
		if err == nil {
			err = domain.ErrConflict
		}
		return application.RunExecution{}, sourceRepositoryError("load exact discovery run artifacts", err)
	}
	return result, nil
}

func (store *Store) BeginDiscoveryRun(ctx context.Context, workspace identity.WorkspaceID, run, job identity.RunID, now time.Time) error {
	workspaceID, _ := uuidValue(workspace)
	runID, _ := uuidValue(run)
	jobID, _ := uuidValue(job)
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return sourceRepositoryError("begin discovery run transition", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockDiscoveryWorkspace(ctx, tx, workspace, 0); err != nil {
		return err
	}
	if err := lockDiscoveryJobLease(ctx, tx, workspaceID, runID, jobID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE discovery_runs SET status='running', started_at=COALESCE(started_at,$4), completed_at=NULL,
error_code=NULL, updated_at=$4 WHERE workspace_id=$1 AND id=$2 AND job_id=$3 AND status IN ('queued','running')`,
		workspaceID, runID, jobID, timestamp(now))
	if err != nil || tag.RowsAffected() != 1 {
		if err == nil {
			err = domain.ErrConflict
		}
		return sourceRepositoryError("begin exact discovery run", err)
	}
	if err := recordSourceSnapshotAttempt(ctx, tx, workspaceID, runID, now, "partial", "DISCOVERY_IN_PROGRESS", false); err != nil {
		return sourceRepositoryError("record discovery coverage attempt", err)
	}
	queries := dbgen.New(tx)
	stored, err := queries.GetOperationsRuntimeRunByJob(ctx, dbgen.GetOperationsRuntimeRunByJobParams{WorkspaceID: workspaceID, JobID: jobID})
	if err != nil {
		return sourceRepositoryError("load discovery runtime projection", err)
	}
	projected, err := runtimeRunFromRow(stored)
	if err != nil {
		return err
	}
	projected.State, projected.Phase, projected.ErrorCode, projected.ErrorSummary = operationsdomain.RunRunning, "running", "", ""
	projected.StartedAt, projected.FinishedAt, projected.UpdatedAt = timePointer(now), nil, now.UTC()
	projected.Attempt, projected.MaxAttempts, err = jobAttempt(ctx, tx, workspaceID, jobID)
	if err != nil {
		return sourceRepositoryError("load discovery job attempt", err)
	}
	event, err := newRuntimeEvent(workspace, projected.ID, fmt.Sprintf("attempt:%d:running", projected.Attempt),
		operationsdomain.RunEventState, operationsdomain.RunRunning, "running", "", "", now)
	if err != nil {
		return err
	}
	if err := projectRuntime(ctx, queries, projected, event); err != nil {
		return sourceRepositoryError("project running discovery run", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return sourceRepositoryError("commit discovery run transition", err)
	}
	return nil
}

func (store *Store) RetryOrFailDiscoveryRun(ctx context.Context, workspace identity.WorkspaceID, run, job identity.RunID, terminal bool, code string, now time.Time) error {
	workspaceID, _ := uuidValue(workspace)
	runID, _ := uuidValue(run)
	jobID, _ := uuidValue(job)
	status := "queued"
	var started, completed any = nil, nil
	var errorCode any = nil
	if terminal {
		status, started, completed, errorCode = "failed", timestamp(now), timestamp(now), code
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return sourceRepositoryError("begin discovery failure transition", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockDiscoveryWorkspace(ctx, tx, workspace, 0); err != nil {
		return err
	}
	if err := lockDiscoveryJobLease(ctx, tx, workspaceID, runID, jobID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE discovery_runs SET status=$4, started_at=$5, completed_at=$6, error_code=$7, updated_at=$8
WHERE workspace_id=$1 AND id=$2 AND job_id=$3 AND status IN ('queued','running')`, workspaceID, runID, jobID, status, started, completed, errorCode, timestamp(now))
	if err != nil || tag.RowsAffected() != 1 {
		if err == nil {
			err = domain.ErrConflict
		}
		return sourceRepositoryError("record discovery failure", err)
	}
	if err := recordSourceSnapshotAttempt(ctx, tx, workspaceID, runID, now, "failed", code, terminal); err != nil {
		return sourceRepositoryError("record failed source coverage", err)
	}
	queries := dbgen.New(tx)
	stored, err := queries.GetOperationsRuntimeRunByJob(ctx, dbgen.GetOperationsRuntimeRunByJobParams{WorkspaceID: workspaceID, JobID: jobID})
	if err != nil {
		return sourceRepositoryError("load discovery runtime projection", err)
	}
	projected, err := runtimeRunFromRow(stored)
	if err != nil {
		return err
	}
	projected.Attempt, projected.MaxAttempts, err = jobAttempt(ctx, tx, workspaceID, jobID)
	if err != nil {
		return sourceRepositoryError("load discovery job attempt", err)
	}
	projected.UpdatedAt = now.UTC()
	state, eventKey := operationsdomain.RunQueued, fmt.Sprintf("attempt:%d:retryable", projected.Attempt)
	if terminal {
		state, eventKey, projected.FinishedAt, projected.ErrorCode = operationsdomain.RunFailed,
			fmt.Sprintf("attempt:%d:failed", projected.Attempt), timePointer(now), code
		projected.ErrorSummary = "discovery run failed"
	} else {
		projected.FinishedAt, projected.ErrorCode, projected.ErrorSummary = nil, "", ""
	}
	projected.State, projected.Phase = state, string(state)
	event, err := newRuntimeEvent(workspace, projected.ID, eventKey, operationsdomain.RunEventState,
		state, projected.Phase, projected.ErrorCode, projected.ErrorSummary, now)
	if err != nil {
		return err
	}
	if err := projectRuntime(ctx, queries, projected, event); err != nil {
		return sourceRepositoryError("project discovery failure", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return sourceRepositoryError("commit discovery failure transition", err)
	}
	return nil
}

func lockDiscoveryJobLease(ctx context.Context, tx pgx.Tx, workspaceID, runID, jobID pgtype.UUID) error {
	fence, hasFence := application.RunCommitFenceFromContext(ctx)
	owner := ""
	if hasFence {
		if !fence.JobID.IsZero() {
			fenceJobID, err := uuidValue(fence.JobID)
			if err != nil || fenceJobID != jobID {
				return ErrLeaseLost
			}
		}
		owner = strings.TrimSpace(fence.LeaseOwner)
		if owner == "" {
			return ErrLeaseLost
		}
	}
	var locked pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT j.id
FROM jobs j JOIN discovery_runs r ON r.workspace_id=j.workspace_id AND r.job_id=j.id
WHERE j.workspace_id=$1 AND j.id=$2 AND r.id=$3
  AND j.status='running' AND j.leased_until > clock_timestamp()
  AND ($4 = '' OR j.lease_owner=$4)
FOR UPDATE OF j, r`, workspaceID, jobID, runID, owner).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return sourceRepositoryError("lock discovery job lease", err)
	}
	return nil
}

func (store *Store) ListDiscoveryCandidates(ctx context.Context, query application.ListCandidatesQuery) (domain.CandidatePage, error) {
	workspaceID, err := uuidValue(query.WorkspaceID)
	if err != nil {
		return domain.CandidatePage{}, err
	}
	cursor, err := decodeCandidateCursor(query)
	if err != nil {
		return domain.CandidatePage{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.CandidatePage{}, sourceRepositoryError("begin candidate list", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := verifyDiscoveryAuthorizationVersion(ctx, tx, query.WorkspaceID, query.AuthorizationVersion); err != nil {
		return domain.CandidatePage{}, err
	}
	var sourceFilter any
	if query.SourceID != nil {
		sourceFilter, err = uuidValue(*query.SourceID)
		if err != nil {
			return domain.CandidatePage{}, err
		}
	}
	var total int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM semantic_candidates WHERE workspace_id=$1 AND ($2::uuid IS NULL OR source_connection_id=$2) AND ($3='' OR status=$3)`, workspaceID, sourceFilter, query.Status).Scan(&total); err != nil {
		return domain.CandidatePage{}, sourceRepositoryError("count candidates", err)
	}
	rows, err := tx.Query(ctx, candidateSelect+` WHERE c.workspace_id=$1 AND ($2::uuid IS NULL OR c.source_connection_id=$2) AND ($3='' OR c.status=$3)
AND ($4::timestamptz IS NULL OR (c.created_at,c.id)<($4,$5)) ORDER BY c.created_at DESC,c.id DESC LIMIT $6`,
		workspaceID, sourceFilter, query.Status, cursor.CreatedAt, cursor.ID, query.Limit+1)
	if err != nil {
		return domain.CandidatePage{}, sourceRepositoryError("list candidates", err)
	}
	defer rows.Close()
	result := make([]domain.Candidate, 0, query.Limit+1)
	for rows.Next() {
		value, scanErr := scanCandidate(rows)
		if scanErr != nil {
			return domain.CandidatePage{}, scanErr
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return domain.CandidatePage{}, err
	}
	next := ""
	if len(result) > query.Limit {
		result = result[:query.Limit]
		next = encodeCandidateCursor(query, result[len(result)-1])
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CandidatePage{}, sourceRepositoryError("commit candidate list", err)
	}
	return domain.CandidatePage{Items: result, Total: total, Limit: query.Limit, NextCursor: next}, nil
}

func (store *Store) GetDiscoveryCandidate(ctx context.Context, workspace identity.WorkspaceID, candidate identity.SemanticCandidateID, expectedAuthorizationVersion int64) (domain.Candidate, error) {
	workspaceID, _ := uuidValue(workspace)
	candidateID, _ := uuidValue(candidate)
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.Candidate{}, sourceRepositoryError("begin candidate read", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := verifyDiscoveryAuthorizationVersion(ctx, tx, workspace, expectedAuthorizationVersion); err != nil {
		return domain.Candidate{}, err
	}
	value, err := scanCandidate(tx.QueryRow(ctx, candidateSelect+` WHERE c.workspace_id=$1 AND c.id=$2`, workspaceID, candidateID))
	if err != nil {
		return domain.Candidate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Candidate{}, sourceRepositoryError("commit candidate read", err)
	}
	return value, nil
}

func (store *Store) LookupDiscoveryCandidateSource(ctx context.Context, workspace identity.WorkspaceID,
	candidate identity.SemanticCandidateID,
) (identity.SourceConnectionID, error) {
	workspaceID, _ := uuidValue(workspace)
	candidateID, _ := uuidValue(candidate)
	var source pgtype.UUID
	if err := store.pool.QueryRow(ctx, `SELECT source_connection_id FROM semantic_candidates WHERE workspace_id=$1 AND id=$2`, workspaceID, candidateID).Scan(&source); err != nil {
		return identity.SourceConnectionID{}, sourceRepositoryError("lookup candidate source", err)
	}
	return identity.SourceConnectionIDFromUUIDBytes(source.Bytes)
}

const candidateSelect = `SELECT c.id,c.workspace_id,c.source_connection_id,c.source_revision_id,c.discovery_run_id,
c.candidate_key,c.candidate_kind,c.title,c.proposal_input,c.evidence,c.content_digest,c.status,c.proposal_id,c.created_at,c.updated_at FROM semantic_candidates c`

func (store *Store) DecideDiscoveryCandidate(ctx context.Context, command application.DecideCandidateCommand) (domain.CandidateDecision, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.CandidateDecision{}, sourceRepositoryError("begin candidate decision", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	workspaceID, _ := uuidValue(command.Decision.WorkspaceID)
	if err := lockDiscoveryWorkspace(ctx, tx, command.Decision.WorkspaceID, command.ExpectedAuthorizationVersion); err != nil {
		return domain.CandidateDecision{}, err
	}
	candidateID, _ := uuidValue(command.Decision.CandidateID)
	decisionID, _ := uuidValue(command.Decision.ID)
	if existing, getErr := scanCandidateDecision(tx.QueryRow(ctx, decisionSelect+` WHERE d.workspace_id=$1 AND d.idempotency_key=$2`, workspaceID, command.Decision.IdempotencyKey)); getErr == nil {
		if existing.RequestFingerprint == "" || existing.RequestFingerprint != command.Decision.RequestFingerprint {
			return domain.CandidateDecision{}, domain.ErrConflict
		}
		return existing, nil
	} else if !errors.Is(getErr, domain.ErrNotFound) {
		return domain.CandidateDecision{}, getErr
	}
	var proposal any
	if command.Decision.ProposalID != nil {
		value, convertErr := uuidValue(*command.Decision.ProposalID)
		if convertErr != nil {
			return domain.CandidateDecision{}, convertErr
		}
		proposal = value
	}
	status := "dismissed"
	if command.Decision.Action == "convert" {
		status = "converted"
	}
	tag, err := tx.Exec(ctx, `UPDATE semantic_candidates SET status=$3, proposal_id=$4, updated_at=$5
WHERE workspace_id=$1 AND id=$2 AND status=$6`, workspaceID, candidateID, status, proposal, timestamp(command.Decision.CreatedAt), command.ExpectedStatus)
	if err != nil || tag.RowsAffected() != 1 {
		if err == nil {
			err = domain.ErrConflict
		}
		return domain.CandidateDecision{}, sourceRepositoryError("decide candidate state", err)
	}
	row := tx.QueryRow(ctx, `INSERT INTO semantic_candidate_decisions
(id,workspace_id,candidate_id,action,proposal_id,actor,reason,idempotency_key,request_fingerprint,trace_id,created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id,workspace_id,candidate_id,action,proposal_id,actor,reason,idempotency_key,request_fingerprint,trace_id,created_at`,
		decisionID, workspaceID, candidateID, command.Decision.Action, proposal, actorOrSystem(command.Decision.Actor), command.Decision.Reason,
		command.Decision.IdempotencyKey, command.Decision.RequestFingerprint, command.Decision.TraceID, timestamp(command.Decision.CreatedAt))
	result, err := scanCandidateDecision(row)
	if err != nil {
		return domain.CandidateDecision{}, sourceRepositoryError("create candidate decision", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CandidateDecision{}, sourceRepositoryError("commit candidate decision", err)
	}
	return result, nil
}

const decisionSelect = `SELECT d.id,d.workspace_id,d.candidate_id,d.action,d.proposal_id,d.actor,d.reason,d.idempotency_key,d.request_fingerprint,d.trace_id,d.created_at FROM semantic_candidate_decisions d`

type sourceCursor struct {
	Workspace            string       `json:"w"`
	Principal            string       `json:"p"`
	AuthorizationVersion int64        `json:"a"`
	Source               string       `json:"s"`
	UpdatedAt            *time.Time   `json:"t"`
	ID                   *pgtype.UUID `json:"i"`
}

func decodeSourceCursor(query application.ListSourcesQuery) (sourceCursor, error) {
	if strings.TrimSpace(query.Cursor) == "" {
		return sourceCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(query.Cursor)
	if err != nil || len(raw) > 4096 {
		return sourceCursor{}, domain.ErrInvalidInput
	}
	var value sourceCursor
	if json.Unmarshal(raw, &value) != nil || value.Workspace != query.WorkspaceID.String() ||
		value.Principal != query.PrincipalID.String() || value.AuthorizationVersion != query.AuthorizationVersion ||
		value.Source != sourceFilterString(query.SourceID) ||
		value.UpdatedAt == nil || value.ID == nil || !value.ID.Valid {
		return sourceCursor{}, domain.ErrInvalidInput
	}
	return value, nil
}

func encodeSourceCursor(query application.ListSourcesQuery, value domain.Source) string {
	id, _ := uuidValue(value.ID)
	updated := value.UpdatedAt.UTC()
	raw, _ := json.Marshal(sourceCursor{Workspace: query.WorkspaceID.String(), Principal: query.PrincipalID.String(),
		AuthorizationVersion: query.AuthorizationVersion, Source: sourceFilterString(query.SourceID), UpdatedAt: &updated, ID: &id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

type candidateCursor struct {
	Workspace            string       `json:"w"`
	Principal            string       `json:"p"`
	AuthorizationVersion int64        `json:"a"`
	Source               string       `json:"x"`
	Status               string       `json:"s"`
	CreatedAt            *time.Time   `json:"t"`
	ID                   *pgtype.UUID `json:"i"`
}

func decodeCandidateCursor(query application.ListCandidatesQuery) (candidateCursor, error) {
	if strings.TrimSpace(query.Cursor) == "" {
		return candidateCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(query.Cursor)
	if err != nil || len(raw) > 4096 {
		return candidateCursor{}, domain.ErrInvalidInput
	}
	var value candidateCursor
	if json.Unmarshal(raw, &value) != nil || value.Workspace != query.WorkspaceID.String() ||
		value.Principal != query.PrincipalID.String() || value.AuthorizationVersion != query.AuthorizationVersion ||
		value.Source != sourceFilterString(query.SourceID) || value.Status != query.Status || value.CreatedAt == nil || value.ID == nil || !value.ID.Valid {
		return candidateCursor{}, domain.ErrInvalidInput
	}
	return value, nil
}

func encodeCandidateCursor(query application.ListCandidatesQuery, value domain.Candidate) string {
	id, _ := uuidValue(value.ID)
	created := value.CreatedAt.UTC()
	raw, _ := json.Marshal(candidateCursor{Workspace: query.WorkspaceID.String(), Principal: query.PrincipalID.String(),
		AuthorizationVersion: query.AuthorizationVersion, Source: sourceFilterString(query.SourceID), Status: query.Status, CreatedAt: &created, ID: &id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func sourceFilterString(value *identity.SourceConnectionID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

type rowScanner interface{ Scan(...any) error }

func scanDiscoverySource(row rowScanner) (domain.Source, error) {
	var id, workspace, artifactSet pgtype.UUID
	var credential pgtype.Int8
	var metadata, paths []byte
	var result domain.Source
	err := row.Scan(&id, &workspace, &result.Kind, &result.Name, &metadata, &paths, &result.Status,
		&credential, &artifactSet, &result.Version, &result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return domain.Source{}, sourceRepositoryError("scan source", err)
	}
	result.ID, err = identity.SourceConnectionIDFromUUIDBytes(id.Bytes)
	if err != nil {
		return domain.Source{}, err
	}
	result.WorkspaceID, err = identity.WorkspaceIDFromUUIDBytes(workspace.Bytes)
	if err != nil {
		return domain.Source{}, err
	}
	if credential.Valid {
		result.ActiveCredentialVersion = credential.Int64
	}
	if artifactSet.Valid {
		value, parseErr := identity.ArtifactSetIDFromUUIDBytes(artifactSet.Bytes)
		if parseErr != nil {
			return domain.Source{}, parseErr
		}
		result.ActiveArtifactSetID = &value
	}
	var settings struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Database string `json:"database"`
		Username string `json:"username"`
		SSLMode  string `json:"sslMode"`
	}
	if json.Unmarshal(metadata, &settings) != nil || json.Unmarshal(paths, &result.ArtifactPaths) != nil {
		return domain.Source{}, domain.ErrInvalidSnapshot
	}
	result.Host, result.Port, result.Database, result.Username, result.SSLMode = settings.Host, settings.Port, settings.Database, settings.Username, settings.SSLMode
	return result, nil
}

func (store *Store) getControlDiscoveryRun(ctx context.Context, workspace identity.WorkspaceID, run identity.RunID) (domain.Run, error) {
	workspaceID, _ := uuidValue(workspace)
	runID, _ := uuidValue(run)
	return scanDiscoveryRun(store.pool.QueryRow(ctx, discoveryRunSelect+` WHERE r.workspace_id=$1 AND r.id=$2`, workspaceID, runID), true)
}

func scanDiscoveryRun(row rowScanner, withSnapshot ...bool) (domain.Run, error) {
	var id, workspace, source, artifactSet, job pgtype.UUID
	var credential pgtype.Int8
	var fingerprint, sourceInputFingerprint, errorCode, requestedBy, traceID pgtype.Text
	var stats []byte
	var started, completed pgtype.Timestamptz
	var result domain.Run
	var snapshot pgtype.UUID
	values := []any{&id, &workspace, &source, &credential, &artifactSet, &fingerprint, &sourceInputFingerprint, &result.SourceConfig, &job, &result.Status, &errorCode, &stats, &requestedBy, &traceID, &started, &completed, &result.CreatedAt, &result.UpdatedAt}
	if len(withSnapshot) > 0 && withSnapshot[0] {
		values = append(values, &snapshot)
	}
	err := row.Scan(values...)
	if err != nil {
		return domain.Run{}, sourceRepositoryError("scan discovery run", err)
	}
	result.ID, err = identity.RunIDFromUUIDBytes(id.Bytes)
	if err != nil {
		return domain.Run{}, err
	}
	result.WorkspaceID, _ = identity.WorkspaceIDFromUUIDBytes(workspace.Bytes)
	result.SourceConnectionID, _ = identity.SourceConnectionIDFromUUIDBytes(source.Bytes)
	if snapshot.Valid {
		value, err := identity.SourceSnapshotIDFromUUIDBytes(snapshot.Bytes)
		if err != nil {
			return domain.Run{}, err
		}
		result.SnapshotID = &value
	}
	result.JobID, _ = identity.RunIDFromUUIDBytes(job.Bytes)
	if credential.Valid {
		result.CredentialVersion = credential.Int64
	}
	if artifactSet.Valid {
		value, parseErr := identity.ArtifactSetIDFromUUIDBytes(artifactSet.Bytes)
		if parseErr != nil {
			return domain.Run{}, parseErr
		}
		result.ArtifactSetID = &value
	}
	result.RequestFingerprint = fingerprint.String
	result.SourceInputFingerprint = sourceInputFingerprint.String
	result.ErrorCode = optionalText(errorCode)
	result.Stats = cloneJSON(stats)
	result.RequestedBy = optionalText(requestedBy)
	result.TraceID = optionalText(traceID)
	result.StartedAt = optionalTime(started)
	result.CompletedAt = optionalTime(completed)
	return result, nil
}

func applyPinnedSourceConfig(source *domain.Source, raw json.RawMessage) error {
	var pinned struct {
		Metadata      json.RawMessage `json:"metadata"`
		ArtifactPaths json.RawMessage `json:"artifactPaths"`
		SourceVersion int64           `json:"sourceVersion"`
	}
	var settings struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Database string `json:"database"`
		Username string `json:"username"`
		SSLMode  string `json:"sslMode"`
	}
	if json.Unmarshal(raw, &pinned) != nil || json.Unmarshal(pinned.Metadata, &settings) != nil ||
		json.Unmarshal(pinned.ArtifactPaths, &source.ArtifactPaths) != nil || pinned.SourceVersion < 1 {
		return domain.ErrInvalidSnapshot
	}
	source.Host, source.Port, source.Database, source.Username, source.SSLMode = settings.Host, settings.Port, settings.Database, settings.Username, settings.SSLMode
	source.Version = pinned.SourceVersion
	return nil
}

func scanCandidate(row rowScanner) (domain.Candidate, error) {
	var id, workspace, source, revision, run pgtype.UUID
	var proposal pgtype.UUID
	var result domain.Candidate
	err := row.Scan(&id, &workspace, &source, &revision, &run, &result.Key, &result.Kind, &result.Title, &result.ProposalInput, &result.Evidence, &result.ContentDigest, &result.Status, &proposal, &result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return domain.Candidate{}, sourceRepositoryError("scan candidate", err)
	}
	result.ID, _ = identity.SemanticCandidateIDFromUUIDBytes(id.Bytes)
	result.WorkspaceID, _ = identity.WorkspaceIDFromUUIDBytes(workspace.Bytes)
	result.SourceConnectionID, _ = identity.SourceConnectionIDFromUUIDBytes(source.Bytes)
	result.SourceRevisionID, _ = identity.SourceRevisionIDFromUUIDBytes(revision.Bytes)
	result.DiscoveryRunID, _ = identity.RunIDFromUUIDBytes(run.Bytes)
	if proposal.Valid {
		value, convertErr := identity.ProposalIDFromUUIDBytes(proposal.Bytes)
		if convertErr != nil {
			return domain.Candidate{}, convertErr
		}
		result.ProposalID = &value
	}
	return result, nil
}

func scanCandidateDecision(row rowScanner) (domain.CandidateDecision, error) {
	var id, workspace, candidate, proposal pgtype.UUID
	var result domain.CandidateDecision
	var fingerprint pgtype.Text
	err := row.Scan(&id, &workspace, &candidate, &result.Action, &proposal, &result.Actor, &result.Reason, &result.IdempotencyKey, &fingerprint, &result.TraceID, &result.CreatedAt)
	if err != nil {
		return domain.CandidateDecision{}, sourceRepositoryError("scan candidate decision", err)
	}
	result.ID, _ = identity.RunIDFromUUIDBytes(id.Bytes)
	result.WorkspaceID, _ = identity.WorkspaceIDFromUUIDBytes(workspace.Bytes)
	result.CandidateID, _ = identity.SemanticCandidateIDFromUUIDBytes(candidate.Bytes)
	result.RequestFingerprint = fingerprint.String
	if proposal.Valid {
		value, convertErr := identity.ProposalIDFromUUIDBytes(proposal.Bytes)
		if convertErr != nil {
			return domain.CandidateDecision{}, convertErr
		}
		result.ProposalID = &value
	}
	return result, nil
}

func sourceDatabaseIDs(workspace identity.WorkspaceID, source identity.SourceConnectionID, credential identity.SourceCredentialID) (pgtype.UUID, pgtype.UUID, pgtype.UUID, error) {
	w, err := uuidValue(workspace)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	s, err := uuidValue(source)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	c, err := uuidValue(credential)
	return w, s, c, err
}

func normalizedSourceLocator(source domain.Source) string {
	return net.JoinHostPort(strings.Trim(source.Host, "[]"), strconv.Itoa(source.Port)) + "/" + source.Database
}
func actorOrSystem(value string) string {
	if strings.TrimSpace(value) == "" {
		return "system"
	}
	return strings.TrimSpace(value)
}

func insertSourceAudit(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, eventType, actor, traceID string, payload any) error {
	eventID, err := identity.NewEventID()
	if err != nil {
		return err
	}
	databaseEventID, _ := uuidValue(eventID)
	workspaceID, _ := uuidValue(workspace)
	encoded, _ := json.Marshal(payload)
	if traceID == "" {
		traceID = "00000000000000000000000000000000"
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (id, workspace_id, event_type, actor_id, payload, trace_id)
		VALUES ($1, $2, $3, $4, $5, $6)`, databaseEventID, workspaceID, eventType, actorOrSystem(actor), encoded, traceID)
	return sourceRepositoryError("record source audit", err)
}

func sourceRepositoryError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrConflict) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, domain.ErrNotFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23514", "23503", "55000":
			return fmt.Errorf("%s (%s): %w", operation, pgErr.ConstraintName, domain.ErrConflict)
		}
	}
	return fmt.Errorf("%s: repository operation failed", operation)
}

func verifyDiscoveryAuthorizationVersion(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, expected int64) error {
	err := verifyArtifactAuthorizationVersion(ctx, tx, workspace, expected)
	if errors.Is(err, ingestiondomain.ErrConflict) {
		return domain.ErrConflict
	}
	return err
}

func lockDiscoveryWorkspace(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, expected int64) error {
	err := lockArtifactWorkspace(ctx, tx, workspace, expected)
	if errors.Is(err, ingestiondomain.ErrConflict) {
		return domain.ErrConflict
	}
	return err
}
