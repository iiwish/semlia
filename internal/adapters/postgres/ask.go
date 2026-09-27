package postgres

import (
	"context"
	"errors"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	app "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) GetAskRequest(ctx context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID, key string) (domain.AskRequestRecord, error) {
	w, err := uuidValue(workspace)
	if err != nil {
		return domain.AskRequestRecord{}, err
	}
	p, err := uuidValue(principal)
	if err != nil {
		return domain.AskRequestRecord{}, err
	}
	row, err := s.queries.GetAskRequest(ctx, dbgen.GetAskRequestParams{WorkspaceID: w, RequestedBy: p, IdempotencyKey: key})
	if err != nil {
		return domain.AskRequestRecord{}, governanceRepositoryError("get Ask request", err)
	}
	return askRequestFromRow(row)
}

func (s *Store) ClaimAskRequest(ctx context.Context, request domain.AskRequestRecord, run domain.AgentRun) (domain.AskRequestRecord, bool, error) {
	w, err := uuidValue(request.WorkspaceID)
	if err != nil {
		return domain.AskRequestRecord{}, false, err
	}
	p, err := uuidValue(request.RequestedBy)
	if err != nil {
		return domain.AskRequestRecord{}, false, err
	}
	r, err := uuidValue(request.RunID)
	if err != nil {
		return domain.AskRequestRecord{}, false, err
	}
	var claim pgtype.UUID
	if err := claim.Scan(request.ClaimToken); err != nil {
		return domain.AskRequestRecord{}, false, domain.ErrInvalidArgument
	}
	if run.ID != request.RunID || run.WorkspaceID != request.WorkspaceID || run.InputHash != request.InputDigest || run.Status != domain.AgentRunRunning {
		return domain.AskRequestRecord{}, false, domain.ErrInvalidArgument
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.AskRequestRecord{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return domain.AskRequestRecord{}, false, err
	}
	queries := dbgen.New(tx)
	row, err := queries.ClaimAskRequest(ctx, dbgen.ClaimAskRequestParams{WorkspaceID: w, RequestedBy: p, IdempotencyKey: request.Key, InputDigest: request.InputDigest, KnowledgeDigest: request.KnowledgeDigest, AgentRunID: r, ClaimToken: claim, CallDeadline: timestamp(now.Add(90 * time.Second)), CreatedAt: timestamp(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		prior, err := s.GetAskRequest(ctx, request.WorkspaceID, request.RequestedBy, request.Key)
		return prior, false, err
	}
	if err != nil {
		return domain.AskRequestRecord{}, false, governanceRepositoryError("claim Ask request", err)
	}
	run.StartedAt, run.CreatedAt = now, now
	if _, err := createAgentRunTx(ctx, tx, run); err != nil {
		return domain.AskRequestRecord{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AskRequestRecord{}, false, governanceRepositoryError("commit Ask claim", err)
	}
	created, err := askRequestFromRow(row)
	return created, true, err
}

func (s *Store) FinishAskRequest(ctx context.Context, command app.AskFinishCommand) (domain.AgentRun, error) {
	w, err := uuidValue(command.Request.WorkspaceID)
	if err != nil {
		return domain.AgentRun{}, err
	}
	p, err := uuidValue(command.Request.RequestedBy)
	if err != nil {
		return domain.AgentRun{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.AgentRun{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	row, err := queries.LockAskRequest(ctx, dbgen.LockAskRequestParams{WorkspaceID: w, RequestedBy: p, IdempotencyKey: command.Request.Key})
	if err != nil {
		return domain.AgentRun{}, governanceRepositoryError("lock Ask request", err)
	}
	stored, err := askRequestFromRow(row)
	if err != nil {
		return domain.AgentRun{}, err
	}
	if stored.RunID != command.Request.RunID || stored.ClaimToken != command.Request.ClaimToken || stored.InputDigest != command.Request.InputDigest {
		return domain.AgentRun{}, domain.ErrConflict
	}
	if stored.Status != "running" {
		if stored.Status == "outcome_unknown" {
			return domain.AgentRun{}, &app.AskStateError{Code: "ASK_OUTCOME_UNKNOWN"}
		}
		return domain.AgentRun{}, &app.AskStateError{Code: "ASK_OUTCOME_ALREADY_RECORDED"}
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return domain.AgentRun{}, err
	}
	command.Request, command.FinishedAt = stored, now
	late := !now.Before(stored.Deadline)
	if late {
		command.Status, command.ErrorCode, command.OutputDigest, command.QueryID = "outcome_unknown", "ASK_OUTCOME_UNKNOWN", "", nil
	}
	finished, err := finishAskTx(ctx, tx, command)
	if err != nil {
		return domain.AgentRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AgentRun{}, governanceRepositoryError("commit Ask result", err)
	}
	if late {
		return finished, &app.AskStateError{Code: "ASK_OUTCOME_UNKNOWN"}
	}
	return finished, nil
}

func finishAskTx(ctx context.Context, tx pgx.Tx, command app.AskFinishCommand) (domain.AgentRun, error) {
	r := command.Request
	w, _ := uuidValue(r.WorkspaceID)
	p, _ := uuidValue(r.RequestedBy)
	runID, _ := uuidValue(r.RunID)
	var token pgtype.UUID
	if err := token.Scan(r.ClaimToken); err != nil {
		return domain.AgentRun{}, err
	}
	queries := dbgen.New(tx)
	queryID := pgtype.UUID{}
	if command.QueryID != nil {
		var err error
		queryID, err = uuidValue(*command.QueryID)
		if err != nil {
			return domain.AgentRun{}, err
		}
		query, err := queries.GetSemanticQueryRecord(ctx, dbgen.GetSemanticQueryRecordParams{WorkspaceID: w, ID: queryID})
		if err != nil {
			return domain.AgentRun{}, err
		}
		if query.PrincipalRef != r.RequestedBy.String() || query.IdempotencyKey != "ask:"+r.RunID.String() || query.Channel != "ask" {
			return domain.AgentRun{}, domain.ErrInvariant
		}
	}
	if command.ModelOutputDigest == "" {
		command.ModelOutputDigest = r.InputDigest
	}
	step, err := identity.NewAgentStepID()
	if err != nil {
		return domain.AgentRun{}, err
	}
	stepID, _ := uuidValue(step)
	if _, err := queries.CreateAgentStep(ctx, dbgen.CreateAgentStepParams{ID: stepID, WorkspaceID: w, AgentRunID: runID, Sequence: 1, Kind: "model", InputHash: r.InputDigest, OutputHash: command.ModelOutputDigest, ErrorCode: optionalTextValue(command.ErrorCode), CreatedAt: timestamp(command.FinishedAt)}); err != nil {
		return domain.AgentRun{}, err
	}
	if command.QueryID != nil {
		step, err := identity.NewAgentStepID()
		if err != nil {
			return domain.AgentRun{}, err
		}
		stepID, _ := uuidValue(step)
		if _, err := queries.CreateAgentStep(ctx, dbgen.CreateAgentStepParams{ID: stepID, WorkspaceID: w, AgentRunID: runID, Sequence: 2, Kind: "tool", ToolName: textValue("semantic_resolve"), InputHash: r.InputDigest, OutputHash: command.OutputDigest, CreatedAt: timestamp(command.FinishedAt)}); err != nil {
			return domain.AgentRun{}, err
		}
	}
	state := domain.AgentRunFailed
	var output *string
	switch command.Status {
	case "succeeded", "clarification":
		state, output = domain.AgentRunSucceeded, &command.OutputDigest
	case "cancelled":
		state = domain.AgentRunCancelled
	case "failed", "outcome_unknown":
	default:
		return domain.AgentRun{}, domain.ErrInvalidArgument
	}
	finished, err := finishAgentRunTx(ctx, tx, app.AgentRunFinishCommand{WorkspaceID: r.WorkspaceID, RunID: r.RunID, FinalState: state, OutputDigest: output, CostMicros: command.CostMicros, FinishedAt: command.FinishedAt, DurationMS: max(int64(0), command.FinishedAt.Sub(r.CreatedAt).Milliseconds())})
	if err != nil {
		return domain.AgentRun{}, err
	}
	_, err = queries.CompleteAskRequest(ctx, dbgen.CompleteAskRequestParams{WorkspaceID: w, RequestedBy: p, IdempotencyKey: r.Key, ClaimToken: token, Status: command.Status, SemanticQueryID: queryID, ErrorCode: optionalTextValue(command.ErrorCode), CompletedAt: timestamp(command.FinishedAt)})
	return finished, err
}

func (s *Store) ExpireAskRequests(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		return 0, domain.ErrInvalidArgument
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := dbgen.New(tx).ExpiredAskRequests(ctx, int32(limit))
	if err != nil {
		return 0, err
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return 0, err
	}
	for _, row := range rows {
		record, err := askRequestFromRow(row)
		if err != nil {
			return 0, err
		}
		if _, err := finishAskTx(ctx, tx, app.AskFinishCommand{Request: record, Status: "outcome_unknown", ErrorCode: "ASK_OUTCOME_UNKNOWN", FinishedAt: now}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func askRequestFromRow(row dbgen.AskRequest) (domain.AskRequestRecord, error) {
	w, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return domain.AskRequestRecord{}, err
	}
	p, err := identity.PrincipalIDFromUUIDBytes(row.RequestedBy.Bytes)
	if err != nil {
		return domain.AskRequestRecord{}, err
	}
	r, err := identity.AgentRunIDFromUUIDBytes(row.AgentRunID.Bytes)
	if err != nil {
		return domain.AskRequestRecord{}, err
	}
	token, err := identity.RunIDFromUUIDBytes(row.ClaimToken.Bytes)
	if err != nil {
		return domain.AskRequestRecord{}, err
	}
	record := domain.AskRequestRecord{WorkspaceID: w, RequestedBy: p, Key: row.IdempotencyKey, InputDigest: row.InputDigest, KnowledgeDigest: row.KnowledgeDigest, RunID: r, ClaimToken: token.UUID(), Deadline: row.CallDeadline.Time, Status: row.Status, ErrorCode: optionalText(row.ErrorCode), CreatedAt: row.CreatedAt.Time}
	if row.SemanticQueryID.Valid {
		q, err := identity.SemanticQueryIDFromUUIDBytes(row.SemanticQueryID.Bytes)
		if err != nil {
			return domain.AskRequestRecord{}, err
		}
		record.QueryID = &q
	}
	if row.CompletedAt.Valid {
		record.CompletedAt = &row.CompletedAt.Time
	}
	return record, nil
}
