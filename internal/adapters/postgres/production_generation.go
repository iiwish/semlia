package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	app "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func productionGenerationRequestDigest(r domain.ProductionGenerationRequest) (string, error) {
	raw, err := json.Marshal(struct {
		Schema    string
		Workspace string
		Operation string
		Request   domain.ProductionGenerationRequest
	}{"semlia.production-generation-request/v1", r.WorkspaceID.String(), r.OperationID.String(), r})
	if err != nil {
		return "", err
	}
	return domain.DigestJSON(raw)
}

func (s *Store) QueueProductionGeneration(ctx context.Context, r domain.ProductionGenerationRequest, grants []domain.ProductionGenerationGrant, mode string) (domain.ProductionGenerationResult, error) {
	empty := domain.ProductionGenerationResult{}
	if err := app.ValidateProductionGenerationRequest(r); err != nil {
		return empty, err
	}
	if mode != "actual_model" && mode != "protocol_stub" {
		return empty, domain.ErrInvalidArgument
	}
	digest, err := productionGenerationRequestDigest(r)
	if err != nil {
		return empty, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return empty, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Replays require current read/write scope but not fresh input or a new
	// spending grant; they cannot issue another paid request.
	ver, targets, err := s.productionGenerationVersionTx(ctx, tx, r, false)
	if err != nil {
		return empty, err
	}
	access, err := s.productionAccessTx(ctx, tx, r.WorkspaceID, r.PrincipalID)
	if err != nil {
		return empty, err
	}
	if err := authorizeProductionTargets(ctx, tx, access, r.WorkspaceID, targets, true, ver.BaselineJSON); err != nil {
		return empty, err
	}
	w, _ := uuidFromString(r.WorkspaceID.UUID())
	p, _ := uuidFromString(r.PrincipalID.UUID())
	op, _ := uuidFromString(r.OperationID.UUID())
	q := s.queries.WithTx(tx)
	previous, err := q.GetProductionCommand(ctx, dbgen.GetProductionCommandParams{WorkspaceID: w, PrincipalID: p, CommandKind: domain.CommandGenerate, IdempotencyKey: r.IdempotencyKey})
	if err == nil {
		if previous.RequestDigest != digest {
			return empty, domain.ErrIdempotencyConflict
		}
		var result domain.ProductionGenerationResult
		if json.Unmarshal(previous.ResponseJson, &result) != nil || result.RunID.IsZero() {
			return empty, domain.ErrPriorStateUnknown
		}
		result, err = s.readProductionGenerationTx(ctx, tx, r.WorkspaceID, r.PrincipalID, r.OperationID, result.RunID)
		if err != nil {
			return empty, err
		}
		result.Replayed = true
		return result, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return empty, err
	}
	work, ver, err := s.prepareProductionGenerationTx(ctx, tx, r, identity.PrincipalID{})
	if err != nil {
		return empty, err
	}
	work.GrantDigest, err = app.ProductionGenerationGrantFor(work, grants)
	if err != nil {
		return empty, err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM production_generation_requests WHERE workspace_id=$1 AND operation_id=$2`, w, op).Scan(&count); err != nil {
		return empty, err
	}
	if count >= 256 {
		return empty, domain.ErrLimitExceeded
	}
	run, err := identity.NewAgentRunID()
	if err != nil {
		return empty, err
	}
	job, err := identity.NewRunID()
	if err != nil {
		return empty, err
	}
	work.RunID, work.JobID, work.ProviderMode = run, job, mode
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `INSERT INTO agent_runs(id,workspace_id,principal_id,model,config_revision,input_hash,status,started_at,created_at) VALUES($1,$2,$3,$4,$5,$6,'running',$7,$7)`, run.UUID(), w, work.AgentID.UUID(), work.Setting.Model, r.ModelConfigRevision, work.PromptDigest, now); err != nil {
		return empty, err
	}
	payload, _ := json.Marshal(map[string]string{"runId": run.String(), "operationId": r.OperationID.String()})
	if _, err := tx.Exec(ctx, `INSERT INTO jobs(id,workspace_id,job_type,payload,max_attempts,available_at,idempotency_key,trace_id) VALUES($1,$2,$3,$4,8,$5,$6,$7)`, job.UUID(), w, app.ProductionGenerationJobType, payload, now, "production-generation/"+run.String(), r.TraceID); err != nil {
		return empty, err
	}
	requestJSON, err := json.Marshal(r)
	if err != nil {
		return empty, err
	}
	requestJSON, err = domain.CanonicalJSON(requestJSON)
	if err != nil {
		return empty, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO production_generation_requests(workspace_id,operation_id,agent_run_id,job_id,input_version,requested_by,agent_principal_id,model_setting_id,model_config_revision,input_digest,set_digest,request_digest,prompt_digest,grant_digest,canonical_request,provider_mode,status,created_at)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'queued',$17)`, w, op, run.UUID(), job.UUID(), r.ExpectedVersion, p, work.AgentID.UUID(), r.ModelSettingID.UUID(), r.ModelConfigRevision, r.InputDigest, ver.SetDigest, digest, work.PromptDigest, work.GrantDigest, requestJSON, mode, now); err != nil {
		return empty, err
	}
	for _, c := range []struct {
		principal identity.PrincipalID
		role      string
	}{{r.PrincipalID, domain.ContributorInitiator}, {work.AgentID, domain.ContributorAgent}} {
		if _, err := tx.Exec(ctx, `INSERT INTO production_contributors(workspace_id,operation_id,principal_id,role,created_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, w, op, c.principal.UUID(), c.role, now); err != nil {
			return empty, err
		}
	}
	result := domain.ProductionGenerationResult{RunID: run, OperationID: r.OperationID, InputVersion: r.ExpectedVersion, InputDigest: r.InputDigest, Status: "queued", ProviderMode: mode, Model: work.Setting.Model, ModelConfigRevision: r.ModelConfigRevision}
	response, err := json.Marshal(result)
	if err != nil {
		return empty, err
	}
	if err := q.InsertProductionCommand(ctx, dbgen.InsertProductionCommandParams{WorkspaceID: w, PrincipalID: p, CommandKind: domain.CommandGenerate, IdempotencyKey: r.IdempotencyKey, RequestDigest: digest, OperationID: op, OperationVersion: int32(r.ExpectedVersion), ResultKind: "operation", ResultID: op, CommittedAt: timestamp(now), ResponseJson: response}); err != nil {
		return empty, err
	}
	ver.CreatedAt = now
	if err := insertProductionMutationEvents(ctx, q, ver, "generation_queued"); err != nil {
		return empty, err
	}
	return result, tx.Commit(ctx)
}

func (s *Store) ReadProductionGeneration(ctx context.Context, w identity.WorkspaceID, principal identity.PrincipalID, op identity.ProductionOperationID, run identity.AgentRunID) (domain.ProductionGenerationResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.ProductionGenerationResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := s.readProductionGenerationTx(ctx, tx, w, principal, op, run)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func (s *Store) loadProductionGenerationWorkTx(ctx context.Context, tx pgx.Tx, w identity.WorkspaceID, op identity.ProductionOperationID, run identity.AgentRunID) (domain.ProductionGenerationWork, error) {
	work := domain.ProductionGenerationWork{RunID: run}
	var request []byte
	var principal, agent, job pgtype.UUID
	var claim pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT canonical_request,requested_by,agent_principal_id,job_id,status,provider_mode,request_digest,prompt_digest,grant_digest,claim_token,call_deadline FROM production_generation_requests WHERE workspace_id=$1 AND operation_id=$2 AND agent_run_id=$3`, w.UUID(), op.UUID(), run.UUID()).Scan(&request, &principal, &agent, &job, &work.Status, &work.ProviderMode, &work.RequestDigest, &work.PromptDigest, &work.GrantDigest, &claim, &work.CallDeadline)
	if err != nil {
		return work, generationRepositoryError(err)
	}
	if json.Unmarshal(request, &work.Request) != nil {
		return work, domain.ErrPriorStateUnknown
	}
	work.Request.WorkspaceID = w
	work.Request.OperationID = op
	work.Request.PrincipalID, err = identity.PrincipalIDFromUUIDBytes(principal.Bytes)
	if err != nil {
		return work, err
	}
	work.AgentID, err = identity.PrincipalIDFromUUIDBytes(agent.Bytes)
	if err != nil {
		return work, err
	}
	work.JobID, err = identity.RunIDFromUUIDBytes(job.Bytes)
	if err != nil {
		return work, err
	}
	if claim.Valid {
		work.ClaimToken = formatUUID(claim)
	}
	return work, nil
}

func (s *Store) readProductionGenerationTx(ctx context.Context, tx pgx.Tx, w identity.WorkspaceID, principal identity.PrincipalID, op identity.ProductionOperationID, run identity.AgentRunID) (domain.ProductionGenerationResult, error) {
	result := domain.ProductionGenerationResult{}
	var locked pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM workspaces WHERE id=$1 FOR UPDATE`, w.UUID()).Scan(&locked); err != nil {
		return result, generationRepositoryError(err)
	}
	work, err := s.loadProductionGenerationWorkTx(ctx, tx, w, op, run)
	if err != nil {
		return result, err
	}
	r := work.Request
	r.PrincipalID = principal
	ver, _, err := s.productionGenerationVersionTx(ctx, tx, r, false)
	if err != nil {
		return result, err
	}
	ver.CreatedBy = principal
	var status string
	if err := tx.QueryRow(ctx, `SELECT model,config_revision,status,duration_ms FROM agent_runs WHERE workspace_id=$1 AND id=$2`, w.UUID(), run.UUID()).Scan(&result.Model, &result.ModelConfigRevision, &status, &result.DurationMS); err != nil {
		return result, err
	}
	result.RunID = run
	result.OperationID = op
	result.InputVersion = work.Request.ExpectedVersion
	result.InputDigest = work.Request.InputDigest
	result.Status = work.Status
	result.ProviderMode = work.ProviderMode
	if err := tx.QueryRow(ctx, `SELECT error_code,cost_micros FROM production_generation_requests WHERE workspace_id=$1 AND agent_run_id=$2`, w.UUID(), run.UUID()).Scan(&result.ErrorCode, &result.CostMicros); err != nil {
		return result, err
	}
	if result.Status == "succeeded" {
		if status != "succeeded" {
			return result, domain.ErrPriorStateUnknown
		}
		if err := tx.QueryRow(ctx, `SELECT o.canonical_output,o.output_digest FROM production_generation_outputs o JOIN agent_runs a ON a.workspace_id=o.workspace_id AND a.id=o.agent_run_id AND a.output_digest=o.output_digest JOIN agent_steps st ON st.workspace_id=o.workspace_id AND st.agent_run_id=o.agent_run_id AND st.sequence=1 AND st.output_hash=o.output_digest WHERE o.workspace_id=$1 AND o.operation_id=$2 AND o.agent_run_id=$3 AND o.input_version=$4`, w.UUID(), op.UUID(), run.UUID(), result.InputVersion).Scan(&result.Output, &result.OutputDigest); err != nil {
			return result, domain.ErrPriorStateUnknown
		}
		digest, err := domain.DigestJSON(result.Output)
		if err != nil || result.OutputDigest == nil || digest != *result.OutputDigest {
			return result, domain.ErrPriorStateUnknown
		}
		output, _, err := app.GateProductionGenerationOutput(result.Output)
		if err != nil {
			return result, domain.ErrPriorStateUnknown
		}
		if err := s.authorizeProductionGenerationOutputTx(ctx, tx, ver, output.Targets, false); err != nil {
			return result, err
		}
	}
	return result, nil
}
