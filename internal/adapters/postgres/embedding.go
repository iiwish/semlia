package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	app "github.com/iiwish/semlia/internal/application/embedding"
	"github.com/iiwish/semlia/internal/application/jobs"
	domain "github.com/iiwish/semlia/internal/domain/embedding"
	operations "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) EmbeddingConfigured(ctx context.Context) bool {
	var available bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector') AND EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='embedding_items' AND column_name='vector_value' AND udt_name='vector')`).Scan(&available)
	return err == nil && available
}

type embeddingQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func embeddingConfiguration(ctx context.Context, q embeddingQuery, w identity.WorkspaceID, lock bool) (domain.Config, error) {
	sql := `SELECT p.id,m.id,COALESCE(p.base_url,'https://api.openai.com/v1'),m.model,m.embedding_dimension,p.credential_env,p.credential_revision
FROM model_settings m JOIN model_providers p ON p.workspace_id=m.workspace_id AND p.id=m.provider_id
WHERE m.workspace_id=$1 AND m.kind='embedding' AND m.enabled AND m.is_default AND p.enabled AND p.protocol IN('openai','openai_compatible')`
	if lock {
		sql += " FOR SHARE OF p,m"
	}
	var config domain.Config
	var provider, setting pgtype.UUID
	if err := q.QueryRow(ctx, sql, w.UUID()).Scan(&provider, &setting, &config.Endpoint, &config.Model, &config.Dimension, &config.CredentialEnv, &config.CredentialRevision); err != nil {
		return config, domain.ErrNotConfigured
	}
	config.ProviderID, _ = identity.ModelProviderIDFromUUIDBytes(provider.Bytes)
	config.SettingID, _ = identity.ModelSettingIDFromUUIDBytes(setting.Bytes)
	config.ChunkVersion = domain.ChunkVersion
	if config.Dimension < 1 || config.Dimension > 4096 {
		return config, domain.ErrNotConfigured
	}
	return config, nil
}
func (s *Store) EmbeddingConfiguration(ctx context.Context, w identity.WorkspaceID) (domain.Config, error) {
	return embeddingConfiguration(ctx, s.pool, w, false)
}

// EmbeddingPublishedRelease reports whether the workspace has any published
// release. A rebuild indexes the released corpus, so without one there is
// nothing to embed and the request is blocked before any job is queued.
func (s *Store) EmbeddingPublishedRelease(ctx context.Context, w identity.WorkspaceID) (bool, error) {
	var available bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM releases WHERE workspace_id=$1 AND state='published')`, w.UUID()).Scan(&available); err != nil {
		return false, domain.ErrConflict
	}
	return available, nil
}

const embeddingColumns = `id,workspace_id,release_id,config,corpus_digest,chunk_count,vector_count,state,error_code,runtime_run_id,job_id,created_at,updated_at`

func scanEmbedding(row pgx.Row) (domain.Index, error) {
	var result domain.Index
	var id, w, release, runtime, job pgtype.UUID
	var config []byte
	err := row.Scan(&id, &w, &release, &config, &result.CorpusDigest, &result.ChunkCount, &result.VectorCount, &result.State, &result.ErrorCode, &runtime, &job, &result.CreatedAt, &result.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, domain.ErrNotFound
	}
	if err != nil {
		return result, domain.ErrConflict
	}
	if json.Unmarshal(config, &result.Config) != nil {
		return result, domain.ErrConflict
	}
	result.ID, _ = identity.RunIDFromUUIDBytes(id.Bytes)
	result.WorkspaceID, _ = identity.WorkspaceIDFromUUIDBytes(w.Bytes)
	result.ReleaseID, _ = identity.ReleaseIDFromUUIDBytes(release.Bytes)
	result.RuntimeRunID, _ = identity.RunIDFromUUIDBytes(runtime.Bytes)
	result.JobID, _ = identity.RunIDFromUUIDBytes(job.Bytes)
	result.Model = result.Config.Model
	result.Dimension = result.Config.Dimension
	return result, nil
}
func (s *Store) GetEmbedding(ctx context.Context, w identity.WorkspaceID, id identity.RunID) (domain.Index, error) {
	return scanEmbedding(s.pool.QueryRow(ctx, `SELECT `+embeddingColumns+` FROM embedding_index_versions WHERE workspace_id=$1 AND id=$2`, w.UUID(), id.UUID()))
}
func (s *Store) EmbeddingStatus(ctx context.Context, w identity.WorkspaceID) (domain.Status, error) {
	var result domain.Status
	for _, state := range []string{"active", "latest"} {
		sql := `SELECT ` + embeddingColumns + ` FROM embedding_index_versions WHERE workspace_id=$1`
		if state == "active" {
			sql += ` AND state='active'`
		}
		sql += ` ORDER BY created_at DESC,id DESC LIMIT 1`
		index, err := scanEmbedding(s.pool.QueryRow(ctx, sql, w.UUID()))
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return result, err
		}
		if state == "active" {
			result.Active = &index
		} else {
			result.Latest = &index
		}
	}
	return result, nil
}

func lockEmbeddingWorkspace(ctx context.Context, tx pgx.Tx, w identity.WorkspaceID, expected int64) error {
	var version int64
	if err := tx.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1 FOR NO KEY UPDATE`, w.UUID()).Scan(&version); err != nil {
		return domain.ErrNotFound
	}
	if expected > 0 && version != expected {
		return domain.ErrConflict
	}
	return nil
}

func (s *Store) StartEmbedding(ctx context.Context, command app.StartCommand) (domain.Index, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Index{}, domain.ErrConflict
	}
	defer tx.Rollback(ctx)
	if err := lockEmbeddingWorkspace(ctx, tx, command.Workspace, command.AuthorizationVersion); err != nil {
		return domain.Index{}, err
	}
	existing, err := scanEmbedding(tx.QueryRow(ctx, `SELECT `+embeddingColumns+` FROM embedding_index_versions WHERE workspace_id=$1 AND idempotency_key=$2`, command.Workspace.UUID(), command.IdempotencyKey))
	if err == nil {
		var actor pgtype.UUID
		if tx.QueryRow(ctx, `SELECT created_by_principal_id FROM embedding_index_versions WHERE id=$1`, existing.ID.UUID()).Scan(&actor) != nil || actor.Bytes != mustUUIDBytes(command.Principal.UUID()) {
			return domain.Index{}, domain.ErrConflict
		}
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Index{}, err
	}
	config, err := embeddingConfiguration(ctx, tx, command.Workspace, true)
	if err != nil || domain.Digest(config) != domain.Digest(command.Config) {
		return domain.Index{}, domain.ErrConflict
	}
	var release pgtype.UUID
	if tx.QueryRow(ctx, `SELECT id FROM releases WHERE workspace_id=$1 AND state='published' ORDER BY sequence DESC LIMIT 1`, command.Workspace.UUID()).Scan(&release) != nil {
		return domain.Index{}, domain.ErrNoPublishedRelease
	}
	rows, err := tx.Query(ctx, `SELECT a.id,r.id,a.namespace||'.'||a.key,r.content FROM release_assets e JOIN semantic_assets a ON a.workspace_id=e.workspace_id AND a.id=e.asset_id JOIN asset_revisions r ON r.workspace_id=e.workspace_id AND r.id=e.revision_id AND r.asset_id=e.asset_id WHERE e.workspace_id=$1 AND e.release_id=$2 ORDER BY e.position LIMIT 5001`, command.Workspace.UUID(), release)
	if err != nil {
		return domain.Index{}, domain.ErrConflict
	}
	chunks := []domain.Chunk{}
	for rows.Next() {
		var chunk domain.Chunk
		var asset, revision pgtype.UUID
		var content []byte
		if rows.Scan(&asset, &revision, &chunk.Address, &content) != nil {
			rows.Close()
			return domain.Index{}, domain.ErrConflict
		}
		chunk.AssetID, _ = identity.AssetIDFromUUIDBytes(asset.Bytes)
		chunk.RevisionID, _ = identity.RevisionIDFromUUIDBytes(revision.Bytes)
		var fields map[string]json.RawMessage
		if json.Unmarshal(content, &fields) != nil {
			rows.Close()
			return domain.Index{}, domain.ErrInvalid
		}
		chunk.Text = chunk.Address
		for _, key := range []string{"name", "title", "description"} {
			var value string
			if json.Unmarshal(fields[key], &value) == nil && strings.TrimSpace(value) != "" {
				chunk.Text += "\n" + value
			}
		}
		if len(chunk.Text) > 2048 {
			chunk.Text = chunk.Text[:2048]
			for !utf8.ValidString(chunk.Text) {
				chunk.Text = chunk.Text[:len(chunk.Text)-1]
			}
		}
		chunk.Ordinal = len(chunks) + 1
		chunk.Digest = domain.Digest(chunk.Text)
		chunks = append(chunks, chunk)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(chunks) < 1 || len(chunks) > domain.MaxChunks {
		return domain.Index{}, domain.ErrInvalid
	}
	id, _ := identity.NewRunID()
	job, _ := identity.NewRunID()
	now := time.Now().UTC()
	releaseID, _ := identity.ReleaseIDFromUUIDBytes(release.Bytes)
	payload, _ := json.Marshal(map[string]string{"indexId": id.String()})
	if _, err := tx.Exec(ctx, `INSERT INTO jobs(id,workspace_id,job_type,payload,max_attempts,available_at,idempotency_key,trace_id) VALUES($1,$2,$3,$4,3,$5,$6,$7)`, job.UUID(), command.Workspace.UUID(), app.JobType, payload, now, "embedding:"+id.String(), command.TraceID); err != nil {
		return domain.Index{}, domain.ErrConflict
	}
	corpusDigest := domain.Digest(chunks)
	event, err := newRuntimeEvent(command.Workspace, id, "queued", operations.RunEventState, operations.RunQueued, "queued", "", "", now)
	if err != nil {
		return domain.Index{}, err
	}
	runtime := operations.RuntimeRun{ID: id, WorkspaceID: command.Workspace, Kind: operations.RunKindEmbeddingRebuild, SourceType: "embedding_index", SourceID: id.String(), SourceVersionDigest: strings.TrimPrefix(corpusDigest, "sha256:"), JobID: &job, TraceID: command.TraceID, IdempotencyKey: "embedding:" + id.String(), RequestedByPrincipalID: &command.Principal, State: operations.RunQueued, Phase: "queued", MaxAttempts: 3, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := projectRuntime(ctx, dbgen.New(tx), runtime, event); err != nil {
		return domain.Index{}, domain.ErrConflict
	}
	encoded, _ := json.Marshal(config)
	if _, err := tx.Exec(ctx, `INSERT INTO embedding_index_versions(id,workspace_id,release_id,config,config_digest,corpus_digest,chunk_count,state,job_id,runtime_run_id,created_by_principal_id,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,'building',$8,$1,$9,$10)`, id.UUID(), command.Workspace.UUID(), release, encoded, domain.Digest(config), corpusDigest, len(chunks), job.UUID(), command.Principal.UUID(), command.IdempotencyKey); err != nil {
		return domain.Index{}, domain.ErrConflict
	}
	for _, chunk := range chunks {
		if _, err := tx.Exec(ctx, `INSERT INTO knowledge_chunks(workspace_id,index_version_id,ordinal,asset_id,revision_id,address,safe_text,content_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, command.Workspace.UUID(), id.UUID(), chunk.Ordinal, chunk.AssetID.UUID(), chunk.RevisionID.UUID(), chunk.Address, chunk.Text, chunk.Digest); err != nil {
			return domain.Index{}, domain.ErrConflict
		}
	}
	// Reuse only an identical pinned provider/model/dimension/config and content.
	if _, err := tx.Exec(ctx, `INSERT INTO embedding_items(workspace_id,index_version_id,ordinal,dimension,vector_value)
 SELECT c.workspace_id,c.index_version_id,c.ordinal,reusable.dimension,reusable.vector_value FROM knowledge_chunks c JOIN LATERAL (
 SELECT item.dimension,item.vector_value FROM knowledge_chunks old JOIN embedding_index_versions idx ON idx.id=old.index_version_id JOIN embedding_items item ON item.index_version_id=old.index_version_id AND item.ordinal=old.ordinal
 WHERE old.workspace_id=c.workspace_id AND old.content_digest=c.content_digest AND idx.config_digest=$2 AND idx.state IN('active','retired') ORDER BY idx.created_at DESC LIMIT 1) reusable ON true WHERE c.index_version_id=$1`, id.UUID(), domain.Digest(config)); err != nil {
		return domain.Index{}, domain.ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE embedding_index_versions SET vector_count=(SELECT count(*) FROM embedding_items WHERE index_version_id=$1) WHERE id=$1`, id.UUID()); err != nil {
		return domain.Index{}, domain.ErrConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Index{}, domain.ErrConflict
	}
	_ = releaseID
	return s.GetEmbedding(ctx, command.Workspace, id)
}

func mustUUIDBytes(value string) [16]byte { var id pgtype.UUID; _ = id.Scan(value); return id.Bytes }

func embeddingLease(ctx context.Context, tx pgx.Tx, job jobs.Job) error {
	var id pgtype.UUID
	if tx.QueryRow(ctx, `SELECT id FROM jobs WHERE workspace_id=$1 AND id=$2 AND status='running' AND lease_owner=$3 AND leased_until>clock_timestamp() FOR UPDATE`, job.WorkspaceID.UUID(), job.ID.UUID(), job.LeaseOwner).Scan(&id) != nil {
		return domain.ErrLeaseLost
	}
	return nil
}

func (s *Store) EmbeddingBatch(ctx context.Context, job jobs.Job) (domain.Index, []domain.Chunk, error) {
	index, err := scanEmbedding(s.pool.QueryRow(ctx, `SELECT `+embeddingColumns+` FROM embedding_index_versions WHERE workspace_id=$1 AND job_id=$2`, job.WorkspaceID.UUID(), job.ID.UUID()))
	if err != nil {
		return index, nil, err
	}
	if index.State != "building" {
		return index, nil, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT c.ordinal,c.asset_id,c.revision_id,c.address,c.safe_text,c.content_digest FROM knowledge_chunks c LEFT JOIN embedding_items i ON i.index_version_id=c.index_version_id AND i.ordinal=c.ordinal WHERE c.workspace_id=$1 AND c.index_version_id=$2 AND i.ordinal IS NULL ORDER BY c.ordinal LIMIT 16`, job.WorkspaceID.UUID(), index.ID.UUID())
	if err != nil {
		return index, nil, domain.ErrConflict
	}
	defer rows.Close()
	chunks := []domain.Chunk{}
	for rows.Next() {
		var c domain.Chunk
		var a, r pgtype.UUID
		if rows.Scan(&c.Ordinal, &a, &r, &c.Address, &c.Text, &c.Digest) != nil {
			return index, nil, domain.ErrConflict
		}
		c.AssetID, _ = identity.AssetIDFromUUIDBytes(a.Bytes)
		c.RevisionID, _ = identity.RevisionIDFromUUIDBytes(r.Bytes)
		chunks = append(chunks, c)
	}
	return index, chunks, rows.Err()
}

func (s *Store) CheckpointEmbedding(ctx context.Context, job jobs.Job, index domain.Index, chunks []domain.Chunk, vectors [][]float32) error {
	if domain.ValidateVectors(vectors, len(chunks), index.Dimension) != nil {
		return domain.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.ErrConflict
	}
	defer tx.Rollback(ctx)
	if err := embeddingLease(ctx, tx, job); err != nil {
		return err
	}
	current, err := scanEmbedding(tx.QueryRow(ctx, `SELECT `+embeddingColumns+` FROM embedding_index_versions WHERE workspace_id=$1 AND id=$2 AND job_id=$3 FOR UPDATE`, job.WorkspaceID.UUID(), index.ID.UUID(), job.ID.UUID()))
	if err != nil {
		return err
	}
	if current.State != "building" || domain.Digest(current.Config) != domain.Digest(index.Config) {
		return domain.ErrConflict
	}
	for i, chunk := range chunks {
		encoded, _ := json.Marshal(vectors[i])
		tag, err := tx.Exec(ctx, `INSERT INTO embedding_items(workspace_id,index_version_id,ordinal,dimension,vector_value) SELECT workspace_id,index_version_id,ordinal,$4,$5::vector FROM knowledge_chunks WHERE workspace_id=$1 AND index_version_id=$2 AND ordinal=$3 AND content_digest=$6 ON CONFLICT(index_version_id,ordinal) DO NOTHING`, job.WorkspaceID.UUID(), index.ID.UUID(), chunk.Ordinal, index.Dimension, string(encoded), chunk.Digest)
		if err != nil {
			return domain.ErrConflict
		}
		if tag.RowsAffected() == 0 {
			var exists bool
			if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM embedding_items WHERE index_version_id=$1 AND ordinal=$2)`, index.ID.UUID(), chunk.Ordinal).Scan(&exists) != nil || !exists {
				return domain.ErrConflict
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE embedding_index_versions SET vector_count=(SELECT count(*) FROM embedding_items WHERE index_version_id=$1),updated_at=clock_timestamp() WHERE id=$1`, index.ID.UUID()); err != nil {
		return domain.ErrConflict
	}
	if err := embeddingLease(ctx, tx, job); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ActivateEmbedding(ctx context.Context, job jobs.Job, index domain.Index) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.ErrConflict
	}
	defer tx.Rollback(ctx)
	if err := lockEmbeddingWorkspace(ctx, tx, job.WorkspaceID, 0); err != nil {
		return err
	}
	if err := embeddingLease(ctx, tx, job); err != nil {
		return err
	}
	current, err := scanEmbedding(tx.QueryRow(ctx, `SELECT `+embeddingColumns+` FROM embedding_index_versions WHERE workspace_id=$1 AND id=$2 AND job_id=$3 FOR UPDATE`, job.WorkspaceID.UUID(), index.ID.UUID(), job.ID.UUID()))
	if err != nil {
		return err
	}
	if current.State != "building" {
		return domain.ErrConflict
	}
	var count, bad int
	if tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE dimension<>$2 OR vector_dims(vector_value)<>$2) FROM embedding_items WHERE index_version_id=$1`, index.ID.UUID(), current.Dimension).Scan(&count, &bad) != nil || count != current.ChunkCount || bad != 0 {
		return domain.ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE embedding_index_versions SET state='retired',updated_at=clock_timestamp() WHERE workspace_id=$1 AND state='active'`, job.WorkspaceID.UUID()); err != nil {
		return domain.ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE embedding_index_versions SET state='active',vector_count=$2,updated_at=clock_timestamp() WHERE id=$1`, index.ID.UUID(), count); err != nil {
		return domain.ErrConflict
	}
	if err := embeddingTerminal(ctx, tx, current, "succeeded", ""); err != nil {
		return err
	}
	if err := embeddingLease(ctx, tx, job); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func embeddingTerminal(ctx context.Context, tx pgx.Tx, index domain.Index, state, code string) error {
	q := dbgen.New(tx)
	w, _ := uuidValue(index.WorkspaceID)
	id, _ := uuidValue(index.RuntimeRunID)
	stored, err := q.GetOperationsRuntimeRun(ctx, dbgen.GetOperationsRuntimeRunParams{WorkspaceID: w, ID: id})
	if err != nil {
		return domain.ErrConflict
	}
	runtime, err := runtimeRunFromRow(stored)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	runtime.State = operations.RunState(state)
	runtime.Phase = state
	runtime.ErrorCode = code
	runtime.FinishedAt = &now
	runtime.UpdatedAt = now
	event, err := newRuntimeEvent(index.WorkspaceID, index.RuntimeRunID, "embedding:"+state, operations.RunEventState, runtime.State, state, code, "", now)
	if err != nil {
		return err
	}
	return projectRuntime(ctx, q, runtime, event)
}

func (s *Store) FailEmbedding(ctx context.Context, job jobs.Job, code string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.ErrConflict
	}
	defer tx.Rollback(ctx)
	if err := embeddingLease(ctx, tx, job); err != nil {
		return err
	}
	index, err := scanEmbedding(tx.QueryRow(ctx, `SELECT `+embeddingColumns+` FROM embedding_index_versions WHERE workspace_id=$1 AND job_id=$2 FOR UPDATE`, job.WorkspaceID.UUID(), job.ID.UUID()))
	if err != nil {
		return err
	}
	if index.State != "building" {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE embedding_index_versions SET state='failed',error_code=$2,updated_at=clock_timestamp() WHERE id=$1`, index.ID.UUID(), code); err != nil {
		return domain.ErrConflict
	}
	if err := embeddingTerminal(ctx, tx, index, "failed", code); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) CancelEmbedding(ctx context.Context, w identity.WorkspaceID, id identity.RunID, version int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.ErrConflict
	}
	defer tx.Rollback(ctx)
	if err := lockEmbeddingWorkspace(ctx, tx, w, version); err != nil {
		return err
	}
	var job pgtype.UUID
	if tx.QueryRow(ctx, `SELECT job_id FROM embedding_index_versions WHERE workspace_id=$1 AND id=$2`, w.UUID(), id.UUID()).Scan(&job) != nil {
		return domain.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM jobs WHERE id=$1 FOR UPDATE`, job); err != nil {
		return domain.ErrConflict
	}
	index, err := scanEmbedding(tx.QueryRow(ctx, `SELECT `+embeddingColumns+` FROM embedding_index_versions WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, w.UUID(), id.UUID()))
	if err != nil {
		return err
	}
	if index.State == "cancelled" {
		return nil
	}
	if index.State != "building" {
		return domain.ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE embedding_index_versions SET state='cancelled',updated_at=clock_timestamp() WHERE id=$1`, id.UUID()); err != nil {
		return domain.ErrConflict
	}
	if err := embeddingTerminal(ctx, tx, index, "cancelled", ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SearchEmbedding(ctx context.Context, w identity.WorkspaceID, query string, index *domain.Index, vector []float32) ([]domain.Candidate, string, error) {
	mode := "lexical"
	var rows pgx.Rows
	var err error
	if index != nil && len(vector) == index.Dimension {
		encoded, _ := json.Marshal(vector)
		rows, err = s.pool.Query(ctx, `SELECT c.asset_id,c.revision_id,c.address,1-(i.vector_value <=> $3::vector) AS score
 FROM embedding_index_versions v JOIN knowledge_chunks c ON c.index_version_id=v.id JOIN embedding_items i ON i.index_version_id=c.index_version_id AND i.ordinal=c.ordinal
 JOIN release_assets r ON r.workspace_id=v.workspace_id AND r.release_id=v.release_id AND r.asset_id=c.asset_id AND r.revision_id=c.revision_id
 WHERE v.workspace_id=$1 AND v.id=$2 AND v.state='active' AND v.release_id=(SELECT id FROM releases WHERE workspace_id=$1 AND state='published' ORDER BY sequence DESC LIMIT 1)
 ORDER BY i.vector_value <=> $3::vector,c.ordinal LIMIT 100`, w.UUID(), index.ID.UUID(), string(encoded))
		if err == nil {
			mode = "vector"
		}
	}
	if rows == nil {
		rows, err = s.pool.Query(ctx, `SELECT a.id,r.id,a.namespace||'.'||a.key,0::float8 FROM releases release JOIN release_assets e ON e.workspace_id=release.workspace_id AND e.release_id=release.id JOIN semantic_assets a ON a.workspace_id=e.workspace_id AND a.id=e.asset_id JOIN asset_revisions r ON r.workspace_id=e.workspace_id AND r.id=e.revision_id AND r.asset_id=e.asset_id WHERE release.workspace_id=$1 AND release.id=(SELECT id FROM releases WHERE workspace_id=$1 AND state='published' ORDER BY sequence DESC LIMIT 1) AND (a.namespace||'.'||a.key||' '||COALESCE(r.content->>'name','')||' '||COALESCE(r.content->>'description','')) ILIKE '%'||$2||'%' ORDER BY e.position LIMIT 100`, w.UUID(), query)
	}
	if err != nil {
		return nil, mode, domain.ErrConflict
	}
	defer rows.Close()
	result := []domain.Candidate{}
	for rows.Next() {
		var candidate domain.Candidate
		var a, r pgtype.UUID
		if rows.Scan(&a, &r, &candidate.Address, &candidate.Score) != nil {
			return nil, mode, domain.ErrConflict
		}
		candidate.AssetID, _ = identity.AssetIDFromUUIDBytes(a.Bytes)
		candidate.RevisionID, _ = identity.RevisionIDFromUUIDBytes(r.Bytes)
		result = append(result, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, mode, domain.ErrConflict
	}
	if mode == "vector" && len(result) == 0 {
		rows.Close()
		return s.SearchEmbedding(ctx, w, query, nil, nil)
	}
	return result, mode, nil
}
