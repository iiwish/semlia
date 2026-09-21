package postgres

import (
	"context"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) ListProductionOperationsPage(ctx context.Context, workspace identity.WorkspaceID, query domain.ProductionListQuery) ([]domain.ProductionOperation, error) {
	var actor, candidate, upper, after pgtype.UUID
	var err error
	for text, dest := range map[string]*pgtype.UUID{query.CreatedBy: &actor, query.CandidateID: &candidate} {
		if text != "" {
			*dest, err = parseUUIDOrTypeID(text)
			if err != nil {
				return nil, domain.ErrInvalidArgument
			}
		}
	}
	if query.WatermarkID != "" {
		upper, err = parseUUIDOrTypeID(query.WatermarkID)
		if err != nil || query.WatermarkTime.IsZero() {
			return nil, domain.ErrInvalidArgument
		}
	}
	if query.AfterID != "" {
		after, err = parseUUIDOrTypeID(query.AfterID)
		if err != nil || query.AfterTime.IsZero() {
			return nil, domain.ErrInvalidArgument
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT o.id,o.created_by,o.current_version,o.created_at,o.updated_at,
	EXISTS(SELECT 1 FROM production_operations successor WHERE successor.workspace_id=o.workspace_id AND successor.supersedes_operation_id=o.id)
	FROM production_operations o JOIN production_versions v ON v.workspace_id=o.workspace_id AND v.operation_id=o.id AND v.version=o.current_version
	WHERE o.workspace_id=$1 AND v.history_quality='verified'
	AND ($2::uuid IS NULL OR o.created_by=$2)
	AND ($3::text='' OR COALESCE(v.input_json->'scope',v.input_json)->'snapshots' @> jsonb_build_array(jsonb_build_object('sourceId',$3::text)))
	AND ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM production_candidate_links l WHERE l.workspace_id=o.workspace_id AND l.operation_id=o.id AND l.version=o.current_version AND l.candidate_id=$4))
	AND ($5::uuid IS NULL OR (o.created_at,o.id)<=($6,$5))
	AND ($7::uuid IS NULL OR (o.created_at,o.id)<($8,$7))
	ORDER BY o.created_at DESC,o.id DESC LIMIT $9`, workspace.UUID(), actor, query.SourceID, candidate, upper, query.WatermarkTime, after, query.AfterTime, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ProductionOperation{}
	for rows.Next() {
		var id, creator pgtype.UUID
		var operation domain.ProductionOperation
		if err := rows.Scan(&id, &creator, &operation.CurrentVersion, &operation.CreatedAt, &operation.UpdatedAt, &operation.Superseded); err != nil {
			return nil, err
		}
		operation.ID, err = identity.ProductionOperationIDFromUUIDBytes(id.Bytes)
		if err != nil {
			return nil, err
		}
		operation.CreatedBy, err = identity.PrincipalIDFromUUIDBytes(creator.Bytes)
		if err != nil {
			return nil, err
		}
		operation.WorkspaceID = workspace
		result = append(result, operation)
	}
	return result, rows.Err()
}
