package postgres

import (
	"context"
	"github.com/iiwish/semlia/pkg/identity"
)

func (s *Store) ProductionReleaseProjectionStatus(ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID) (string, error) {
	var status string
	err := s.pool.QueryRow(ctx, `SELECT CASE WHEN bool_or(status='dead_letter') THEN 'failed' WHEN count(*)>0 AND bool_and(status='published') THEN 'ready' ELSE 'pending' END FROM outbox_events WHERE workspace_id=$1 AND event_type='release.published' AND payload->'data'->>'releaseId'=$2`, workspace.UUID(), release.String()).Scan(&status)
	return status, err
}
