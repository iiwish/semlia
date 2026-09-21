package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	app "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5/pgtype"
)

// AutoDraftRunContext resolves the run a candidate batch will hang off: the
// run must exist and be successful; its source revision scopes the candidate
// selection and its requester becomes the accountable draft author.
func (store *Store) AutoDraftRunContext(
	ctx context.Context, workspace identity.WorkspaceID, run identity.RunID,
) (app.AutoDraftRunContext, bool, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return app.AutoDraftRunContext{}, false, fmt.Errorf("encode workspace ID: %w", err)
	}
	runID, err := uuidValue(run)
	if err != nil {
		return app.AutoDraftRunContext{}, false, fmt.Errorf("encode run ID: %w", err)
	}
	var revision pgtype.UUID
	var requestedBy, status string
	err = store.pool.QueryRow(ctx, `SELECT source_revision_id, COALESCE(requested_by,''), status
FROM discovery_runs WHERE workspace_id=$1 AND id=$2`, workspaceID, runID).Scan(&revision, &requestedBy, &status)
	if err != nil {
		return app.AutoDraftRunContext{}, false, nil
	}
	revisionID, err := identity.SourceRevisionIDFromUUIDBytes(revision.Bytes)
	if err != nil {
		return app.AutoDraftRunContext{}, false, fmt.Errorf("decode revision ID: %w", err)
	}
	return app.AutoDraftRunContext{SourceRevisionID: revisionID, RequestedBy: requestedBy, Status: status}, true, nil
}

// AutoDraftCandidates lists pending candidates of one source revision that
// (a) sit in an allowlisted schema, (b) resolve to a verified snapshot with
// complete coverage, and (c) have no production operation linked yet.
func (store *Store) AutoDraftCandidates(
	ctx context.Context, workspace identity.WorkspaceID, revision identity.SourceRevisionID,
	schemas []string, limit int,
) ([]app.AutoDraftCandidate, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	revisionID, err := uuidValue(revision)
	if err != nil {
		return nil, fmt.Errorf("encode revision ID: %w", err)
	}
	if limit < 1 || limit > 1000 {
		limit = 1000
	}
	rows, err := store.pool.Query(ctx, `SELECT c.id, c.content_digest, c.title,
COALESCE(c.proposal_input->>'qualifiedName',''), c.candidate_kind,
s.id, s.source_connection_id, s.content_digest,
COALESCE((SELECT json_agg(scope.coverage_key ORDER BY scope.coverage_key) FROM source_snapshot_scope scope
          WHERE scope.workspace_id=s.workspace_id AND scope.snapshot_id=s.id
            AND scope.status='complete' AND scope.enumeration_complete=true), '[]'::json) AS coverage_keys
FROM semantic_candidates c
JOIN source_snapshots s ON s.workspace_id=c.workspace_id AND s.source_revision_id=c.source_revision_id
  AND s.history_quality='verified' AND s.coverage_status='complete'
WHERE c.workspace_id=$1 AND c.status='pending' AND c.proposal_id IS NULL
  AND c.source_revision_id=$2
  AND split_part(c.title,'.',1) = ANY($3)
  AND NOT EXISTS (SELECT 1 FROM production_candidate_links l
                  WHERE l.workspace_id=c.workspace_id AND l.candidate_id=c.id)
ORDER BY c.created_at, c.id
LIMIT $4`, workspaceID, revisionID, schemas, limit)
	if err != nil {
		return nil, fmt.Errorf("list auto-draft candidates: %w", err)
	}
	defer rows.Close()
	result := make([]app.AutoDraftCandidate, 0)
	for rows.Next() {
		var id, snapshot, source pgtype.UUID
		var digest, title, qualifiedName, kind, snapshotDigest string
		var coverageJSON []byte
		if err := rows.Scan(&id, &digest, &title, &qualifiedName, &kind, &snapshot, &source, &snapshotDigest, &coverageJSON); err != nil {
			return nil, fmt.Errorf("scan auto-draft candidate: %w", err)
		}
		candidateID, err := identity.SemanticCandidateIDFromUUIDBytes(id.Bytes)
		if err != nil {
			return nil, fmt.Errorf("decode candidate ID: %w", err)
		}
		snapshotID, err := identity.SourceSnapshotIDFromUUIDBytes(snapshot.Bytes)
		if err != nil {
			return nil, fmt.Errorf("decode snapshot ID: %w", err)
		}
		sourceID, err := identity.SourceConnectionIDFromUUIDBytes(source.Bytes)
		if err != nil {
			return nil, fmt.Errorf("decode source ID: %w", err)
		}
		coverageKeys := []string{}
		if err := json.Unmarshal(coverageJSON, &coverageKeys); err != nil {
			return nil, fmt.Errorf("decode coverage keys: %w", err)
		}
		result = append(result, app.AutoDraftCandidate{
			CandidateID: candidateID, CandidateDigest: digest, Title: title, QualifiedName: qualifiedName,
			CandidateKind: kind, SnapshotID: snapshotID, SourceID: sourceID, SnapshotDigest: snapshotDigest,
			CoverageKeys: coverageKeys,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate auto-draft candidates: %w", err)
	}
	return result, nil
}

// HasGenerationForOperation reports whether a generation request already
// exists for the operation, so retried batches never spend duplicate budget.
func (store *Store) HasGenerationForOperation(
	ctx context.Context, workspace identity.WorkspaceID, operation identity.ProductionOperationID,
) (bool, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return false, fmt.Errorf("encode workspace ID: %w", err)
	}
	operationID, err := uuidValue(operation)
	if err != nil {
		return false, fmt.Errorf("encode operation ID: %w", err)
	}
	var exists bool
	if err := store.pool.QueryRow(ctx, `SELECT EXISTS(
SELECT 1 FROM production_generation_requests WHERE workspace_id=$1 AND operation_id=$2)`,
		workspaceID, operationID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check existing generation: %w", err)
	}
	return exists, nil
}

// EnqueueJob already exists on the Store (store.go); the candidate auto-draft
// service uses it to schedule its batch after a discovery run finishes.
