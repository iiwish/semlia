package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	app "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/jobs"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func productionGenerationJobIDs(job jobs.Job) (identity.ProductionOperationID, identity.AgentRunID, error) {
	var payload struct {
		RunID       identity.AgentRunID            `json:"runId"`
		OperationID identity.ProductionOperationID `json:"operationId"`
	}
	if job.Type != app.ProductionGenerationJobType || json.Unmarshal(job.Payload, &payload) != nil || payload.RunID.IsZero() || payload.OperationID.IsZero() {
		return payload.OperationID, payload.RunID, domain.ErrInvalidArgument
	}
	return payload.OperationID, payload.RunID, nil
}

func (s *Store) ClaimProductionGeneration(ctx context.Context, job jobs.Job, grants []domain.ProductionGenerationGrant, mode string) (*domain.ProductionGenerationWork, error) {
	op, run, err := productionGenerationJobIDs(job)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var locked pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM workspaces WHERE id=$1 FOR UPDATE`, job.WorkspaceID.UUID()).Scan(&locked); err != nil {
		return nil, err
	}
	stored, err := s.loadProductionGenerationWorkTx(ctx, tx, job.WorkspaceID, op, run)
	if err != nil {
		return nil, err
	}
	if stored.JobID != job.ID {
		return nil, domain.ErrInvalidArgument
	}
	var owned bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE workspace_id=$1 AND id=$2 AND status='running' AND lease_owner=$3 AND leased_until>clock_timestamp())`, job.WorkspaceID.UUID(), job.ID.UUID(), job.LeaseOwner).Scan(&owned); err != nil {
		return nil, err
	}
	if !owned {
		return nil, errors.New("generation job lease is not held")
	}
	if stored.Status != "queued" {
		if stored.Status == "running" {
			// A reclaimed job has an irrevocable prior invocation claim. Never
			// retry the provider even when its first outcome was not persisted.
			if err := finishProductionGenerationRowsTx(ctx, tx, stored, nil, "GENERATION_OUTCOME_UNKNOWN", true); err != nil {
				return nil, err
			}
		}
		return nil, tx.Commit(ctx)
	}
	if stored.ProviderMode != mode {
		return nil, domain.ErrGenerationNotAuthorized
	}
	work, _, err := s.prepareProductionGenerationTx(ctx, tx, stored.Request, stored.AgentID)
	if err != nil {
		return nil, err
	}
	grantDigest, err := app.ProductionGenerationGrantFor(work, grants)
	if err != nil {
		return nil, err
	}
	if work.PromptDigest != stored.PromptDigest || grantDigest != stored.GrantDigest {
		return nil, domain.ErrInputStale
	}
	work.RunID = stored.RunID
	work.JobID = stored.JobID
	work.ProviderMode = stored.ProviderMode
	work.GrantDigest = stored.GrantDigest
	work.RequestDigest = stored.RequestDigest
	claim, err := identity.NewRunID()
	if err != nil {
		return nil, err
	}
	work.ClaimToken = claim.UUID()
	deadline := time.Now().UTC().Add(90 * time.Second)
	work.CallDeadline = &deadline
	work.Status = "running"
	if _, err := tx.Exec(ctx, `UPDATE production_generation_requests SET status='running',claim_token=$3,call_deadline=$4 WHERE workspace_id=$1 AND agent_run_id=$2 AND status='queued'`, job.WorkspaceID.UUID(), run.UUID(), work.ClaimToken, deadline); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &work, nil
}

func (s *Store) FinishProductionGeneration(ctx context.Context, job jobs.Job, work *domain.ProductionGenerationWork, output json.RawMessage, code string, unknown bool) error {
	op, run, err := productionGenerationJobIDs(job)
	if err != nil {
		return err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var locked pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM workspaces WHERE id=$1 FOR UPDATE`, job.WorkspaceID.UUID()).Scan(&locked); err != nil {
		return err
	}
	stored, err := s.loadProductionGenerationWorkTx(ctx, tx, job.WorkspaceID, op, run)
	if err != nil {
		return err
	}
	if stored.JobID != job.ID {
		return domain.ErrInvalidArgument
	}
	if stored.Status != "queued" && stored.Status != "running" {
		return tx.Commit(ctx)
	}
	if work == nil {
		if stored.Status != "queued" {
			return errors.New("cannot overwrite another generation invocation")
		}
		if code == "" || len(output) > 0 {
			return domain.ErrInvalidArgument
		}
	} else if stored.Status != "running" || stored.ClaimToken == "" || stored.ClaimToken != work.ClaimToken || stored.RunID != work.RunID || stored.RequestDigest != work.RequestDigest {
		return domain.ErrVersionConflict
	}
	if stored.CallDeadline != nil && time.Now().UTC().After(*stored.CallDeadline) {
		output = nil
		code = "GENERATION_OUTCOME_UNKNOWN"
		unknown = true
	}
	if len(output) > 0 && code == "" {
		fresh, ver, gateErr := s.prepareProductionGenerationTx(ctx, tx, stored.Request, stored.AgentID)
		if gateErr == nil && (fresh.PromptDigest != stored.PromptDigest || fresh.Request.InputDigest != stored.Request.InputDigest) {
			gateErr = domain.ErrInputStale
		}
		if gateErr == nil {
			var suggestions domain.ProductionSuggestions
			suggestions, output, gateErr = app.GateProductionGenerationOutput(output)
			if gateErr == nil {
				gateErr = app.CheckProductionSuggestionIdentities(fresh.Declarations, suggestions.Targets)
			}
			if gateErr == nil {
				gateErr = s.authorizeProductionGenerationOutputTx(ctx, tx, ver, suggestions.Targets, true)
			}
			if gateErr == nil {
				ver.CreatedBy = stored.AgentID
				gateErr = s.authorizeProductionGenerationOutputTx(ctx, tx, ver, suggestions.Targets, true)
			}
		}
		if gateErr != nil {
			code = app.ProductionGenerationErrorCode(gateErr)
			if errors.Is(gateErr, app.ErrAIOutputInvalid) {
				code = "AI_OUTPUT_INVALID"
			}
			if errors.Is(gateErr, domain.ErrNotFound) {
				code = "REFERENCE_NOT_FOUND"
			}
			if code == "INTERNAL_ERROR" {
				return gateErr
			}
			output = nil
		}
	}
	if err := finishProductionGenerationRowsTx(ctx, tx, stored, output, code, unknown); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) authorizeProductionGenerationOutputTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, declarations []domain.TargetDeclaration, fresh bool) error {
	var originals []domain.TargetDeclaration
	if json.Unmarshal(ver.DeclarationsJSON, &originals) != nil {
		return domain.ErrPriorStateUnknown
	}
	if err := app.CheckProductionSuggestionIdentities(originals, declarations); err != nil {
		return err
	}
	targets := make([]domain.ProductionTarget, 0, len(declarations))
	for i := range declarations {
		d := &declarations[i]
		t := domain.ProductionTarget{WorkspaceID: ver.WorkspaceID, OperationID: ver.OperationID, Version: ver.Version, LocalKey: d.LocalKey, Kind: d.Kind, Intent: d.Intent, IdentityKey: d.IdentityKey, Declaration: d, ContentJSON: d.Content, BaseRevisionID: d.BaseRevisionID, BaseObjectVersion: d.BaseObjectVersion}
		if d.TargetID != nil {
			t.TargetID = *d.TargetID
		}
		targets = append(targets, t)
	}
	if err := domain.ValidateProductionCanonicalBudget(ver.InputJSON, declarations); err != nil {
		return err
	}
	if err := s.validateProductionInputTx(ctx, tx, ver, targets, fresh, true); err != nil {
		return err
	}
	if fresh {
		baseline, err := loadProductionBaseline(ctx, tx, ver.WorkspaceID, declarations, ver.OperationID)
		if err != nil {
			return err
		}
		local := map[string]string{}
		for _, d := range declarations {
			if d.TargetID != nil {
				local[d.LocalKey] = *d.TargetID
			}
		}
		for _, d := range declarations {
			if d.Intent == domain.ProductionIntentUpdate {
				if _, _, err := domain.ReplayProductionChanges(d, baseline.Targets[d.LocalKey].Content, local); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func finishProductionGenerationRowsTx(ctx context.Context, tx pgx.Tx, work domain.ProductionGenerationWork, output json.RawMessage, code string, unknown bool) error {
	w, run, op := work.Request.WorkspaceID.UUID(), work.RunID.UUID(), work.Request.OperationID.UUID()
	status, runStatus := "failed", "failed"
	var digest *string
	if len(output) > 0 && code == "" {
		value, err := domain.DigestJSON(output)
		if err != nil {
			return err
		}
		digest = &value
		status = "succeeded"
		runStatus = "succeeded"
		mode := "live"
		if work.ProviderMode == "protocol_stub" {
			mode = "stub"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO production_generation_links(workspace_id,operation_id,agent_run_id,input_version,input_digest,model_config_revision,output_digest,provider_mode) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, w, op, run, work.Request.ExpectedVersion, work.Request.InputDigest, work.Request.ModelConfigRevision, value, mode); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO production_generation_outputs(workspace_id,agent_run_id,operation_id,input_version,schema_version,canonical_output,output_digest,canonical_bytes) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, w, run, op, work.Request.ExpectedVersion, domain.ProductionSuggestionsSchema, []byte(output), value, len(output)); err != nil {
			return err
		}
	} else if unknown {
		status = "outcome_unknown"
		code = "GENERATION_OUTCOME_UNKNOWN"
	}
	if digest == nil && code == "" {
		return domain.ErrInvalidArgument
	}
	now := time.Now().UTC()
	var errorCode *string
	if code != "" {
		errorCode = &code
	}
	step, err := identity.NewAgentStepID()
	if err != nil {
		return err
	}
	stepDigest := digest
	if stepDigest == nil {
		raw, _ := json.Marshal(map[string]string{"errorCode": code})
		d, err := domain.DigestJSON(raw)
		if err != nil {
			return err
		}
		stepDigest = &d
	}
	if _, err := tx.Exec(ctx, `INSERT INTO agent_steps(id,workspace_id,agent_run_id,sequence,kind,input_hash,output_hash,error_code,created_at) VALUES($1,$2,$3,1,'model',$4,$5,$6,$7)`, step.UUID(), w, run, work.PromptDigest, *stepDigest, errorCode, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE agent_runs SET status=$3,output_digest=$4,finished_at=GREATEST($5,started_at),duration_ms=GREATEST(0,(extract(epoch FROM ($5::timestamptz-started_at))*1000)::bigint) WHERE workspace_id=$1 AND id=$2 AND status='running'`, w, run, runStatus, digest, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE production_generation_requests SET status=$3,error_code=$4,completed_at=GREATEST($5,created_at) WHERE workspace_id=$1 AND agent_run_id=$2`, w, run, status, errorCode, now); err != nil {
		return err
	}
	return nil
}
