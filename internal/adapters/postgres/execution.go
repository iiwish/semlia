package postgres

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	executionapp "github.com/iiwish/semlia/internal/application/execution"
	dist "github.com/iiwish/semlia/internal/domain/distribution"
	d "github.com/iiwish/semlia/internal/domain/execution"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ executionapp.Repository = (*Store)(nil)
var errExecutionStore = errors.New("EXECUTION_STORE_UNAVAILABLE")

func (store *Store) CheckExecutionCredential(ctx context.Context, c authapp.CredentialLimit) error {
	credential, err := store.LoadClientCredential(ctx, c.CredentialID)
	if err != nil || credential.RevokedAt != nil || !credential.ExpiresAt.After(time.Now()) || credential.WorkspaceID != c.WorkspaceID || credential.PrincipalID != c.PrincipalID || credential.ConsumerID != c.ConsumerID || credential.BindingID != c.BindingID || credential.ScopeType != c.ScopeType || credential.ScopeID != c.ScopeID || !reflect.DeepEqual(credential.AllowedActions, c.Actions) {
		return d.ErrInvalidPlan
	}
	return nil
}
func (store *Store) CheckExecutionSource(ctx context.Context, w identity.WorkspaceID, p *dist.ExecutionProvenance) error {
	if p == nil || len(p.Relations) == 0 {
		return d.ErrInvalidPlan
	}
	for _, r := range p.Relations {
		var active bool
		err := store.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_connections s
    JOIN physical_datasets d ON d.workspace_id=s.workspace_id AND d.source_connection_id=s.id
    JOIN physical_dataset_revisions dr ON dr.workspace_id=d.workspace_id AND dr.physical_dataset_id=d.id AND dr.id=d.current_revision_id
    WHERE s.workspace_id=$1 AND s.id=$2 AND s.status='active' AND s.adapter_kind='postgresql_catalog'
      AND s.normalized_locator=$3 AND d.id=$4 AND dr.id=$5 AND dr.source_revision_id=$6)`, w.UUID(), r.SourceID, r.SourceLocator, r.DatasetID, r.DatasetRevisionID, r.SourceRevisionID).Scan(&active)
		if err != nil {
			return errExecutionStore
		}
		if !active {
			return d.ErrInvalidPlan
		}
	}
	return nil
}

func executionUUID(s string) any {
	if s == "" {
		return nil
	}
	id, err := identity.ParseAny(s)
	if err == nil {
		return id.UUID()
	}
	return s
}
func (store *Store) ClaimExecution(ctx context.Context, run d.Run, limit int, version int64) (d.Run, bool, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return d.Run{}, false, errExecutionStore
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current int64
	if tx.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1 FOR UPDATE`, executionUUID(run.WorkspaceID)).Scan(&current) != nil {
		return d.Run{}, false, errExecutionStore
	}
	if current != version {
		return d.Run{}, false, d.ErrConflict
	}
	if err := expireExecutionClaims(ctx, tx, executionUUID(run.WorkspaceID)); err != nil {
		return d.Run{}, false, err
	}
	queries := dbgen.New(tx)
	w, _ := uuidFromString(executionUUID(run.WorkspaceID).(string))
	principal, _ := uuidFromString(executionUUID(run.PrincipalID).(string))
	old, err := queries.GetExecutionRunByKey(ctx, dbgen.GetExecutionRunByKeyParams{WorkspaceID: w, PrincipalID: principal, IdempotencyKey: run.IdempotencyKey})
	if err == nil {
		prior := executionRunFromRow(old)
		if prior.PlanID != run.PlanID || prior.PlanDigest != run.PlanDigest || prior.CredentialID != run.CredentialID || prior.ConsumerID != run.ConsumerID {
			return d.Run{}, false, d.ErrConflict
		}
		if tx.Commit(ctx) != nil {
			return d.Run{}, false, errExecutionStore
		}
		return prior, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return d.Run{}, false, errExecutionStore
	}
	var count int
	if tx.QueryRow(ctx, `SELECT count(*) FROM query_execution_runs WHERE workspace_id=$1 AND state='running'`, w).Scan(&count) != nil {
		return d.Run{}, false, errExecutionStore
	}
	if count >= limit {
		return d.Run{}, false, d.ErrBusy
	}
	_, err = tx.Exec(ctx, `INSERT INTO runtime_runs(id,workspace_id,kind,source_type,source_id,source_version_digest,trace_id,idempotency_key,requested_by_principal_id,state,phase,attempt,max_attempts,started_at,created_at,updated_at)
 VALUES($1,$2,'query_execution','query_execution',$3,$4,$5,$6,$7,'running','executing',1,1,$8,$8,$8)`, executionUUID(run.ID), w, run.ID, strings.TrimPrefix(run.PlanDigest, "sha256:"), run.TraceID, "execution:"+run.ID, principal, run.StartedAt)
	if err != nil {
		return d.Run{}, false, errExecutionStore
	}
	_, err = tx.Exec(ctx, `INSERT INTO query_execution_runs(id,workspace_id,runtime_run_id,semantic_query_id,resolved_plan_id,plan_digest,principal_id,consumer_id,credential_id,channel,source_connection_id,source_revision_id,adapter_version,idempotency_key,state,trace_id,started_at,deadline_at,policy_version,timeout_ms,max_rows,max_bytes)
 VALUES($1,$2,$1,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'running',$14,$15,$16,$17,$18,$19,$20)`, executionUUID(run.ID), w, executionUUID(run.QueryID), executionUUID(run.PlanID), run.PlanDigest, principal, executionUUID(run.ConsumerID), executionUUID(run.CredentialID), run.Channel, executionUUID(run.SourceID), executionUUID(run.SourceRevisionID), run.AdapterVersion, run.IdempotencyKey, run.TraceID, run.StartedAt, run.Deadline, run.PolicyVersion, run.TimeoutMS, run.MaxRows, run.MaxBytes)
	if err != nil {
		return d.Run{}, false, errExecutionStore
	}
	if tx.Commit(ctx) != nil {
		return d.Run{}, false, errExecutionStore
	}
	return run, true, nil
}
func expireExecutionClaims(ctx context.Context, tx pgx.Tx, w any) error {
	_, err := tx.Exec(ctx, `WITH lost AS (
 UPDATE query_execution_runs SET state='unknown',error_code='EXECUTION_OUTCOME_UNKNOWN',finished_at=CURRENT_TIMESTAMP
 WHERE workspace_id=$1 AND state='running' AND deadline_at<CURRENT_TIMESTAMP RETURNING runtime_run_id
 ) UPDATE runtime_runs SET state='failed',phase='outcome_unknown',error_code='EXECUTION_OUTCOME_UNKNOWN',error_summary='Outcome unavailable; this run is never automatically retried',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP,version=version+1 WHERE id IN(SELECT runtime_run_id FROM lost)`, w)
	if err != nil {
		return errExecutionStore
	}
	return nil
}
func (store *Store) FinishExecution(ctx context.Context, run d.Run) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return errExecutionStore
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE query_execution_runs SET state=$3,error_code=NULLIF($4,''),row_count=$5,byte_count=$6,result_digest=NULLIF($7,''),finished_at=$8 WHERE workspace_id=$1 AND id=$2 AND state='running'`, executionUUID(run.WorkspaceID), executionUUID(run.ID), run.State, run.ErrorCode, run.RowCount, run.ByteCount, run.ResultDigest, run.FinishedAt)
	if err != nil || tag.RowsAffected() != 1 {
		return errExecutionStore
	}
	runtimeCode := ""
	if run.State == "failed" {
		runtimeCode = run.ErrorCode
	}
	_, err = tx.Exec(ctx, `UPDATE runtime_runs SET state=$3,phase='completed',error_code=NULLIF($4,''),finished_at=$5,updated_at=$5,version=version+1 WHERE workspace_id=$1 AND id=$2`, executionUUID(run.WorkspaceID), executionUUID(run.ID), run.State, runtimeCode, run.FinishedAt)
	if err != nil {
		return errExecutionStore
	}
	audit, _ := identity.NewEventID()
	// Fixed-shape audit metadata excludes SQL, values, rows and driver diagnostics.
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,workspace_id,event_type,actor_id,payload,trace_id,created_at)
 VALUES($1,$2,'semantic.query.executed',$3,jsonb_build_object('runId',$4::text,'planId',$5::text,'planDigest',$6::text,'state',$7::text,'rowCount',$8::integer,'byteCount',$9::integer,'resultDigest',$10::text,'errorCode',$11::text),$12,$13)`, audit.UUID(), executionUUID(run.WorkspaceID), run.PrincipalID, run.ID, run.PlanID, run.PlanDigest, run.State, run.RowCount, run.ByteCount, run.ResultDigest, run.ErrorCode, run.TraceID, run.FinishedAt)
	if err != nil {
		return errExecutionStore
	}
	if tx.Commit(ctx) != nil {
		return errExecutionStore
	}
	return nil
}
func (store *Store) GetExecution(ctx context.Context, w identity.WorkspaceID, id string) (d.Run, error) {
	parsed, err := identity.ParseRunID(id)
	if err != nil {
		return d.Run{}, d.ErrNotFound
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return d.Run{}, errExecutionStore
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := expireExecutionClaims(ctx, tx, w.UUID()); err != nil {
		return d.Run{}, err
	}
	wid, _ := uuidValue(w)
	rid, _ := uuidValue(parsed)
	row, err := dbgen.New(tx).GetExecutionRun(ctx, dbgen.GetExecutionRunParams{WorkspaceID: wid, ID: rid})
	if errors.Is(err, pgx.ErrNoRows) {
		return d.Run{}, d.ErrNotFound
	}
	if err != nil {
		return d.Run{}, errExecutionStore
	}
	if tx.Commit(ctx) != nil {
		return d.Run{}, errExecutionStore
	}
	return executionRunFromRow(row), nil
}
func executionRunFromRow(r dbgen.QueryExecutionRun) d.Run {
	typed := func(prefix identity.Prefix, id pgtype.UUID) string {
		if !id.Valid {
			return ""
		}
		v, _ := identity.FromUUIDBytes(prefix, id.Bytes)
		return v.String()
	}
	run := d.Run{ID: typed(identity.Run, r.ID), WorkspaceID: typed(identity.Workspace, r.WorkspaceID), QueryID: typed(identity.SemanticQuery, r.SemanticQueryID), PlanID: typed(identity.ResolvedSemanticPlan, r.ResolvedPlanID), PlanDigest: r.PlanDigest, PrincipalID: typed(identity.Principal, r.PrincipalID), ConsumerID: typed(identity.Consumer, r.ConsumerID), CredentialID: typed(identity.ClientCredential, r.CredentialID), Channel: r.Channel, SourceID: r.SourceConnectionID.String(), SourceRevisionID: r.SourceRevisionID.String(), AdapterVersion: r.AdapterVersion, IdempotencyKey: r.IdempotencyKey, State: r.State, ErrorCode: r.ErrorCode.String, RowCount: int(r.RowCount), ByteCount: int(r.ByteCount), ResultDigest: r.ResultDigest.String, TraceID: r.TraceID, StartedAt: r.StartedAt.Time, Deadline: r.DeadlineAt.Time}
	if r.FinishedAt.Valid {
		t := r.FinishedAt.Time
		run.FinishedAt = &t
	}
	run.PolicyVersion, run.TimeoutMS, run.MaxRows, run.MaxBytes, run.CancelRequested = r.PolicyVersion, r.TimeoutMs, int(r.MaxRows), int(r.MaxBytes), r.CancelRequested
	return run
}

func (store *Store) RequestExecutionCancellation(ctx context.Context, w identity.WorkspaceID, id string) error {
	_, err := store.pool.Exec(ctx, `UPDATE query_execution_runs SET cancel_requested=true WHERE workspace_id=$1 AND id=$2 AND state='running' AND NOT cancel_requested`, w.UUID(), executionUUID(id))
	if err != nil {
		return errExecutionStore
	}
	return nil
}
func (store *Store) ExecutionCancellationRequested(ctx context.Context, w identity.WorkspaceID, id string) (bool, error) {
	var requested bool
	err := store.pool.QueryRow(ctx, `SELECT cancel_requested FROM query_execution_runs WHERE workspace_id=$1 AND id=$2`, w.UUID(), executionUUID(id)).Scan(&requested)
	if err != nil {
		return false, errExecutionStore
	}
	return requested, nil
}
