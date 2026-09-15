package postgres

import (
	"context"
	"encoding/json"
	"errors"

	app "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) insertProductionGenerationApplicationTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, run identity.AgentRunID) error {
	result, err := s.readProductionGenerationTx(ctx, tx, ver.WorkspaceID, ver.CreatedBy, ver.OperationID, run)
	if err != nil {
		return err
	}
	if result.Status != "succeeded" || result.OutputDigest == nil || result.InputVersion != ver.Version-1 || result.InputDigest != ver.InputDigest {
		return domain.ErrVersionConflict
	}
	output, _, err := app.GateProductionGenerationOutput(result.Output)
	if err != nil {
		return domain.ErrPriorStateUnknown
	}
	var applied []domain.TargetDeclaration
	if json.Unmarshal(ver.DeclarationsJSON, &applied) != nil {
		return domain.ErrInvalidArgument
	}
	_, delta, err := app.ProductionGenerationDelta(output.Targets, applied)
	if err != nil {
		return err
	}
	deltaDigest, err := domain.DigestJSON(delta)
	if err != nil {
		return err
	}
	contentDigest, err := domain.DigestJSON(ver.DeclarationsJSON)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO production_generation_applications(workspace_id,operation_id,applied_version,agent_run_id,source_version,source_output_digest,applied_content_digest,canonical_delta,delta_digest,actor_principal_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ver.WorkspaceID.UUID(), ver.OperationID.UUID(), ver.Version, run.UUID(), result.InputVersion, *result.OutputDigest, contentDigest, []byte(delta), deltaDigest, ver.CreatedBy.UUID())
	return err
}

func (s *Store) ReadProductionGenerationHistory(ctx context.Context, w identity.WorkspaceID, principal identity.PrincipalID, op identity.ProductionOperationID, version int) ([]string, []domain.ProductionGenerationApplication, error) {
	ids := []string{}
	applications := []domain.ProductionGenerationApplication{}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ver, _, err := s.productionGenerationVersionTx(ctx, tx, domain.ProductionGenerationRequest{WorkspaceID: w, PrincipalID: principal, OperationID: op, ExpectedVersion: version}, false)
	if err != nil {
		return nil, nil, err
	}
	rows, err := tx.Query(ctx, `SELECT agent_run_id FROM production_generation_requests WHERE workspace_id=$1 AND operation_id=$2 AND input_version<=$3 ORDER BY created_at,agent_run_id LIMIT 257`, w.UUID(), op.UUID(), version)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, nil, err
		}
		run, err := identity.AgentRunIDFromUUIDBytes(id.Bytes)
		if err != nil {
			rows.Close()
			return nil, nil, err
		}
		ids = append(ids, run.String())
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(ids) > 256 {
		return nil, nil, domain.ErrLimitExceeded
	}
	for _, id := range ids {
		run, err := identity.ParseAgentRunID(id)
		if err != nil {
			return nil, nil, err
		}
		if _, err := s.readProductionGenerationTx(ctx, tx, w, principal, op, run); err != nil {
			return nil, nil, err
		}
	}
	var a domain.ProductionGenerationApplication
	var run, actor pgtype.UUID
	var delta []byte
	err = tx.QueryRow(ctx, `SELECT agent_run_id,source_version,source_output_digest,applied_version,applied_content_digest,actor_principal_id,delta_digest,canonical_delta FROM production_generation_applications WHERE workspace_id=$1 AND operation_id=$2 AND applied_version=$3`, w.UUID(), op.UUID(), version).Scan(&run, &a.SourceVersion, &a.SourceOutputDigest, &a.AppliedVersion, &a.AppliedContentDigest, &actor, &a.DeltaDigest, &delta)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids, applications, tx.Commit(ctx)
	}
	if err != nil {
		return nil, nil, err
	}
	a.RunID, err = identity.AgentRunIDFromUUIDBytes(run.Bytes)
	if err != nil {
		return nil, nil, err
	}
	a.ActorPrincipalID, err = identity.PrincipalIDFromUUIDBytes(actor.Bytes)
	if err != nil {
		return nil, nil, err
	}
	original, err := s.readProductionGenerationTx(ctx, tx, w, principal, op, a.RunID)
	if err != nil {
		return nil, nil, err
	}
	digest, err := domain.DigestJSON(delta)
	if err != nil || digest != a.DeltaDigest || json.Unmarshal(delta, &a.Delta) != nil {
		return nil, nil, domain.ErrPriorStateUnknown
	}
	contentDigest, err := domain.DigestJSON(ver.DeclarationsJSON)
	if err != nil || contentDigest != a.AppliedContentDigest || original.OutputDigest == nil || *original.OutputDigest != a.SourceOutputDigest {
		return nil, nil, domain.ErrPriorStateUnknown
	}
	if a.SourceVersion != original.InputVersion || a.SourceVersion != version-1 || a.ActorPrincipalID != ver.CreatedBy || original.InputDigest != ver.InputDigest {
		return nil, nil, domain.ErrPriorStateUnknown
	}
	suggestions, _, err := app.GateProductionGenerationOutput(original.Output)
	if err != nil {
		return nil, nil, domain.ErrPriorStateUnknown
	}
	var applied []domain.TargetDeclaration
	if json.Unmarshal(ver.DeclarationsJSON, &applied) != nil {
		return nil, nil, domain.ErrPriorStateUnknown
	}
	_, expectedDelta, err := app.ProductionGenerationDelta(suggestions.Targets, applied)
	if err != nil {
		return nil, nil, domain.ErrPriorStateUnknown
	}
	expectedDigest, err := domain.DigestJSON(expectedDelta)
	if err != nil || expectedDigest != a.DeltaDigest {
		return nil, nil, domain.ErrPriorStateUnknown
	}
	applications = append(applications, a)
	return ids, applications, tx.Commit(ctx)
}
