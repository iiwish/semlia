package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	operationsdomain "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	_ governanceapp.ProposalRepository   = (*Store)(nil)
	_ governanceapp.ValidationRepository = (*Store)(nil)
	_ governanceapp.PolicyRepository     = (*Store)(nil)
	_ governanceapp.ReleaseRepository    = (*Store)(nil)
	_ governanceapp.AgentRunRepository   = (*Store)(nil)
)

func governanceRepositoryError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, governance.ErrConflict)
		case "23503", "23514", "23502", "55000":
			return fmt.Errorf("%s: %w", operation, governance.ErrInvariant)
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, governance.ErrNotFound)
	}
	if errors.Is(err, governance.ErrNotFound) || errors.Is(err, governance.ErrConflict) ||
		errors.Is(err, governance.ErrInvariant) || errors.Is(err, governance.ErrInvalidArgument) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return fmt.Errorf("%s: %w", operation, errRepositoryOperation)
}

func uuidFromString(value string) (pgtype.UUID, error) {
	var result pgtype.UUID
	if err := result.Scan(value); err != nil {
		return pgtype.UUID{}, fmt.Errorf("decode uuid %q: %w", value, err)
	}
	return result, nil
}

func optionalJSON(value json.RawMessage) []byte {
	if len(value) == 0 {
		return nil
	}
	return append([]byte(nil), value...)
}

func optionalTextPointer(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return textValue(*value)
}

// ---------- proposals ----------

func (store *Store) CreateProposal(ctx context.Context, proposal governance.Proposal) (governance.Proposal, error) {
	workspaceID, err := uuidValue(proposal.WorkspaceID)
	if err != nil {
		return governance.Proposal{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal.ID)
	if err != nil {
		return governance.Proposal{}, fmt.Errorf("encode proposal ID: %w", err)
	}
	var assetID, baseRevisionID pgtype.UUID
	if proposal.AssetID != nil {
		if assetID, err = uuidValue(*proposal.AssetID); err != nil {
			return governance.Proposal{}, fmt.Errorf("encode asset ID: %w", err)
		}
	}
	if proposal.BaseRevisionID != nil {
		if baseRevisionID, err = uuidValue(*proposal.BaseRevisionID); err != nil {
			return governance.Proposal{}, fmt.Errorf("encode base revision ID: %w", err)
		}
	}
	targetObjectID, err := uuidFromString(proposal.TargetObjectID)
	if err != nil {
		return governance.Proposal{}, fmt.Errorf("encode target object ID: %w", err)
	}
	row, err := store.queries.CreateProposal(ctx, dbgen.CreateProposalParams{
		ID: proposalID, WorkspaceID: workspaceID, AssetID: assetID, BaseRevisionID: baseRevisionID,
		TargetObjectType: string(proposal.TargetObjectType), TargetObjectID: targetObjectID,
		State: string(proposal.State), Title: proposal.Title, Summary: proposal.Summary,
		Reason: proposal.Reason, CreatedBy: proposal.CreatedBy,
		CreatedAt: timestamp(proposal.CreatedAt), UpdatedAt: timestamp(proposal.UpdatedAt),
	})
	if err != nil {
		return governance.Proposal{}, governanceRepositoryError("create proposal", err)
	}
	return proposalFromRow(row)
}

func (store *Store) GetProposal(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) (governance.Proposal, error) {
	row, err := store.getProposal(ctx, workspace, proposal)
	if err != nil {
		return governance.Proposal{}, err
	}
	return proposalFromRow(row)
}

// CreateProposalWithChanges commits the draft proposal and its full
// change-set in one transaction: the T003 authoring surface accepts the
// change-set inline, so a partial draft (proposal without items) must be
// unrepresentable.
func (store *Store) CreateProposalWithChanges(
	ctx context.Context, proposal governance.Proposal, items []governance.ChangeSetItem,
) (governance.Proposal, []governance.ChangeSetItem, error) {
	workspaceID, err := uuidValue(proposal.WorkspaceID)
	if err != nil {
		return governance.Proposal{}, nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal.ID)
	if err != nil {
		return governance.Proposal{}, nil, fmt.Errorf("encode proposal ID: %w", err)
	}
	var assetID, baseRevisionID, agentRunID pgtype.UUID
	if proposal.AssetID != nil {
		if assetID, err = uuidValue(*proposal.AssetID); err != nil {
			return governance.Proposal{}, nil, fmt.Errorf("encode asset ID: %w", err)
		}
	}
	if proposal.BaseRevisionID != nil {
		if baseRevisionID, err = uuidValue(*proposal.BaseRevisionID); err != nil {
			return governance.Proposal{}, nil, fmt.Errorf("encode base revision ID: %w", err)
		}
	}
	if proposal.AgentRunID != nil {
		if agentRunID, err = uuidValue(*proposal.AgentRunID); err != nil {
			return governance.Proposal{}, nil, fmt.Errorf("encode agent run ID: %w", err)
		}
	}
	targetObjectID, err := uuidFromString(proposal.TargetObjectID)
	if err != nil {
		return governance.Proposal{}, nil, fmt.Errorf("encode target object ID: %w", err)
	}
	changeIDs := make([]identity.ProposalChangeID, 0, len(items))
	for range items {
		changeID, err := identity.NewProposalChangeID()
		if err != nil {
			return governance.Proposal{}, nil, fmt.Errorf("mint change-set item ID: %w", err)
		}
		changeIDs = append(changeIDs, changeID)
	}
	changeParams := make([]dbgen.CreateProposalChangeParams, 0, len(items))
	for index, item := range items {
		changeID, err := uuidValue(changeIDs[index])
		if err != nil {
			return governance.Proposal{}, nil, fmt.Errorf("encode change-set item ID: %w", err)
		}
		changeParams = append(changeParams, dbgen.CreateProposalChangeParams{
			ID: changeID, WorkspaceID: workspaceID, ProposalID: proposalID,
			FieldPath: item.FieldPath, Op: string(item.Op),
			BeforeDigest: optionalTextContent(item.BeforeDigest), AfterDigest: optionalTextContent(item.AfterDigest),
			BeforeValue: optionalJSON(item.BeforeValue), AfterValue: optionalJSON(item.AfterValue),
			CreatedAt: timestamp(proposal.CreatedAt),
		})
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.Proposal{}, nil, governanceRepositoryError("begin proposal draft creation", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	row, err := queries.CreateProposal(ctx, dbgen.CreateProposalParams{
		ID: proposalID, WorkspaceID: workspaceID, AssetID: assetID, BaseRevisionID: baseRevisionID,
		AgentRunID:       agentRunID,
		TargetObjectType: string(proposal.TargetObjectType), TargetObjectID: targetObjectID,
		State: string(proposal.State), Title: proposal.Title, Summary: proposal.Summary,
		Reason: proposal.Reason, CreatedBy: proposal.CreatedBy,
		CreatedAt: timestamp(proposal.CreatedAt), UpdatedAt: timestamp(proposal.UpdatedAt),
	})
	if err != nil {
		return governance.Proposal{}, nil, governanceRepositoryError("create proposal", err)
	}
	created, err := proposalFromRow(row)
	if err != nil {
		return governance.Proposal{}, nil, err
	}
	persisted := make([]governance.ChangeSetItem, 0, len(changeParams))
	for _, params := range changeParams {
		if _, err := queries.CreateProposalChange(ctx, params); err != nil {
			return governance.Proposal{}, nil, governanceRepositoryError("create proposal change", err)
		}
		changeID, err := identity.ProposalChangeIDFromUUIDBytes(params.ID.Bytes)
		if err != nil {
			return governance.Proposal{}, nil, fmt.Errorf("decode change-set item ID: %w", err)
		}
		persisted = append(persisted, governance.ChangeSetItem{
			ID: changeID, WorkspaceID: proposal.WorkspaceID,
			ProposalID: proposal.ID, FieldPath: params.FieldPath, Op: governance.ChangeOp(params.Op),
			BeforeDigest: params.BeforeDigest.String, AfterDigest: params.AfterDigest.String,
			BeforeValue: params.BeforeValue, AfterValue: params.AfterValue, CreatedAt: params.CreatedAt.Time,
		})
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.Proposal{}, nil, governanceRepositoryError("commit proposal draft creation", err)
	}
	return created, persisted, nil
}

// VerifyProposalTarget rejects semantic-asset proposal targets whose base
// revision does not exist in the workspace or does not belong to the asset:
// the proposals FK enforces the same shape at storage level and this check
// turns the violation into a stable not-found before any write.
func (store *Store) VerifyProposalTarget(
	ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID, revision identity.RevisionID,
) error {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return fmt.Errorf("encode workspace ID: %w", err)
	}
	assetUUID, err := uuidValue(asset)
	if err != nil {
		return fmt.Errorf("encode asset ID: %w", err)
	}
	revisionUUID, err := uuidValue(revision)
	if err != nil {
		return fmt.Errorf("encode revision ID: %w", err)
	}
	if _, err := store.queries.GetAssetRevisionOwnership(ctx, dbgen.GetAssetRevisionOwnershipParams{
		WorkspaceID: workspaceID, AssetID: assetUUID, RevisionID: revisionUUID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: proposal target revision %s", governance.ErrNotFound, revision.String())
		}
		return governanceRepositoryError("verify proposal target", err)
	}
	return nil
}

// ListProposals returns one keyset page of workspace proposals ordered
// newest first (created_at DESC, id DESC).
func (store *Store) ListProposals(
	ctx context.Context, workspace identity.WorkspaceID, limit int, cursor *governanceapp.ProposalCursor,
) ([]governance.Proposal, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	params := dbgen.ListProposalsParams{
		WorkspaceID: workspaceID, HasCursor: cursor != nil, PageLimit: int32(limit),
	}
	if cursor != nil {
		cursorID, cursorErr := uuidValue(cursor.ID)
		if cursorErr != nil {
			return nil, fmt.Errorf("encode cursor ID: %w", cursorErr)
		}
		params.CursorCreatedAt = timestamp(cursor.CreatedAt)
		params.CursorID = cursorID
	}
	rows, err := store.queries.ListProposals(ctx, params)
	if err != nil {
		return nil, governanceRepositoryError("list proposals", err)
	}
	proposals := make([]governance.Proposal, 0, len(rows))
	for _, row := range rows {
		proposal, err := proposalFromRow(row)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, proposal)
	}
	return proposals, nil
}

func mustChangeUUID(value identity.ProposalChangeID) pgtype.UUID {
	result, err := uuidValue(value)
	if err != nil {
		panic(err)
	}
	return result
}

func (store *Store) getProposal(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) (dbgen.Proposal, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return dbgen.Proposal{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal)
	if err != nil {
		return dbgen.Proposal{}, fmt.Errorf("encode proposal ID: %w", err)
	}
	row, err := store.queries.GetProposal(ctx, dbgen.GetProposalParams{WorkspaceID: workspaceID, ProposalID: proposalID})
	if err != nil {
		return dbgen.Proposal{}, governanceRepositoryError("get proposal", err)
	}
	return row, nil
}

func (store *Store) SubmitProposal(ctx context.Context, command governanceapp.ProposalSubmitCommand) (governance.Proposal, error) {
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return governance.Proposal{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(command.ProposalID)
	if err != nil {
		return governance.Proposal{}, fmt.Errorf("encode proposal ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.Proposal{}, governanceRepositoryError("begin proposal submission", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	locked, err := queries.GetProposalForUpdate(ctx, dbgen.GetProposalForUpdateParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
	})
	if err != nil {
		return governance.Proposal{}, governanceRepositoryError("lock proposal for submission", err)
	}
	if locked.State != string(governance.ProposalDraft) {
		return governance.Proposal{}, fmt.Errorf(
			"%w: proposal is %s, not draft", governance.ErrConflict, locked.State)
	}
	// Proposals targeting the M2 governance objects must reference an object
	// that exists in the same workspace; the check runs inside the submission
	// transaction so a draft can never be proposed against a missing target.
	// semantic_asset targets are covered by the asset_id foreign key.
	if targetType := governance.TargetObjectType(locked.TargetObjectType); targetType.IsGovernedObject() {
		targetID, decodeErr := uuidFromString(locked.TargetObjectID.String())
		if decodeErr != nil {
			return governance.Proposal{}, fmt.Errorf("decode proposal target object ID: %w", decodeErr)
		}
		if verifyErr := governedTargetExists(ctx, queries, targetType, workspaceID, targetID); verifyErr != nil {
			return governance.Proposal{}, governanceRepositoryError("verify proposal target", verifyErr)
		}
	}
	changeCount, err := queries.CountProposalChanges(ctx, proposalID)
	if err != nil {
		return governance.Proposal{}, governanceRepositoryError("count proposal changes", err)
	}
	if changeCount == 0 {
		return governance.Proposal{}, fmt.Errorf(
			"%w: a proposal submits a non-empty structured change-set", governance.ErrInvariant)
	}
	submitted, err := queries.SubmitProposal(ctx, dbgen.SubmitProposalParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
		SubmittedAt: timestamp(command.SubmittedAt), UpdatedAt: timestamp(command.SubmittedAt),
	})
	if err != nil {
		return governance.Proposal{}, governanceRepositoryError("submit proposal", err)
	}
	// Governed-object proposals carry no asset; the event data includes the
	// asset only when the proposal actually targets one.
	var assetID *identity.AssetID
	if locked.AssetID.Valid {
		parsedAssetID, parseErr := identity.AssetIDFromUUIDBytes(locked.AssetID.Bytes)
		if parseErr != nil {
			return governance.Proposal{}, parseErr
		}
		assetID = &parsedAssetID
	}
	if err := createProposalMutationEvents(ctx, queries, proposalEvent{
		WorkspaceID: command.WorkspaceID, ProposalID: command.ProposalID, AssetID: assetID,
		AuditID: command.AuditEventID, OutboxID: command.OutboxEventID,
		Action: "submitted", FromState: locked.State, ToState: submitted.State,
		Actor: command.Actor, TraceID: command.TraceID, CreatedAt: command.SubmittedAt,
	}); err != nil {
		return governance.Proposal{}, governanceRepositoryError("record proposal submission events", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.Proposal{}, governanceRepositoryError("commit proposal submission", err)
	}
	return proposalFromRow(submitted)
}

func (store *Store) TransitionProposal(ctx context.Context, command governanceapp.ProposalTransitionCommand) (governance.Proposal, error) {
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return governance.Proposal{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(command.ProposalID)
	if err != nil {
		return governance.Proposal{}, fmt.Errorf("encode proposal ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.Proposal{}, governanceRepositoryError("begin proposal transition", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	locked, err := queries.GetProposalForUpdate(ctx, dbgen.GetProposalForUpdateParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
	})
	if err != nil {
		return governance.Proposal{}, governanceRepositoryError("lock proposal for transition", err)
	}
	from := governance.ProposalState(locked.State)
	if err := governance.CheckProposalTransition(from, command.To); err != nil {
		return governance.Proposal{}, err
	}
	transitioned, err := queries.TransitionProposal(ctx, dbgen.TransitionProposalParams{
		WorkspaceID: workspaceID, ProposalID: proposalID, State: string(command.To),
		DecidedAt: optionalTimestamp(command.DecidedAt), ExpectedState: locked.State,
		UpdatedAt: timestamp(command.UpdatedAt),
	})
	if err != nil {
		return governance.Proposal{}, governanceRepositoryError("transition proposal", err)
	}
	var assetID *identity.AssetID
	if locked.AssetID.Valid {
		parsedAssetID, parseErr := identity.AssetIDFromUUIDBytes(locked.AssetID.Bytes)
		if parseErr != nil {
			return governance.Proposal{}, parseErr
		}
		assetID = &parsedAssetID
	}
	action := "state_changed"
	if command.To == governance.ProposalRejected {
		action = "rejected"
	}
	if err := createProposalMutationEvents(ctx, queries, proposalEvent{
		WorkspaceID: command.WorkspaceID, ProposalID: command.ProposalID, AssetID: assetID,
		AuditID: command.AuditEventID, OutboxID: command.OutboxEventID,
		Action: action, FromState: locked.State, ToState: string(command.To),
		Actor: command.Actor, TraceID: command.TraceID, CreatedAt: command.UpdatedAt,
	}); err != nil {
		return governance.Proposal{}, governanceRepositoryError("record proposal transition events", err)
	}
	if err := projectProposalAttention(ctx, queries, transitioned, command.TraceID, command.UpdatedAt); err != nil {
		return governance.Proposal{}, governanceRepositoryError("project proposal attention", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.Proposal{}, governanceRepositoryError("commit proposal transition", err)
	}
	return proposalFromRow(transitioned)
}

func (store *Store) CreateProposalChange(ctx context.Context, item governance.ChangeSetItem) (governance.ChangeSetItem, error) {
	workspaceID, err := uuidValue(item.WorkspaceID)
	if err != nil {
		return governance.ChangeSetItem{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	itemID, err := uuidValue(item.ID)
	if err != nil {
		return governance.ChangeSetItem{}, fmt.Errorf("encode change-set item ID: %w", err)
	}
	proposalID, err := uuidValue(item.ProposalID)
	if err != nil {
		return governance.ChangeSetItem{}, fmt.Errorf("encode proposal ID: %w", err)
	}
	row, err := store.queries.CreateProposalChange(ctx, dbgen.CreateProposalChangeParams{
		ID: itemID, WorkspaceID: workspaceID, ProposalID: proposalID,
		FieldPath: item.FieldPath, Op: string(item.Op),
		BeforeDigest: optionalTextContent(item.BeforeDigest), AfterDigest: optionalTextContent(item.AfterDigest),
		BeforeValue: optionalJSON(item.BeforeValue), AfterValue: optionalJSON(item.AfterValue),
		CreatedAt: timestamp(item.CreatedAt),
	})
	if err != nil {
		return governance.ChangeSetItem{}, governanceRepositoryError("create proposal change", err)
	}
	return changeItemFromRow(row)
}

func (store *Store) DeleteProposalChange(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID, change identity.ProposalChangeID,
) error {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal)
	if err != nil {
		return fmt.Errorf("encode proposal ID: %w", err)
	}
	changeID, err := uuidValue(change)
	if err != nil {
		return fmt.Errorf("encode change-set item ID: %w", err)
	}
	rows, err := store.queries.DeleteProposalChange(ctx, dbgen.DeleteProposalChangeParams{
		WorkspaceID: workspaceID, ProposalID: proposalID, ProposalChangeID: changeID,
	})
	if err != nil {
		return governanceRepositoryError("delete proposal change", err)
	}
	if rows != 1 {
		return fmt.Errorf("%s: %w", "delete proposal change", governance.ErrNotFound)
	}
	return nil
}

func (store *Store) ListProposalChanges(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) ([]governance.ChangeSetItem, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal)
	if err != nil {
		return nil, fmt.Errorf("encode proposal ID: %w", err)
	}
	rows, err := store.queries.ListProposalChanges(ctx, dbgen.ListProposalChangesParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
	})
	if err != nil {
		return nil, governanceRepositoryError("list proposal changes", err)
	}
	items := make([]governance.ChangeSetItem, 0, len(rows))
	for _, row := range rows {
		item, mapErr := changeItemFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (store *Store) CreateReview(ctx context.Context, review governance.Review) (governance.Review, error) {
	if err := review.Validate(); err != nil {
		return governance.Review{}, err
	}
	workspaceID, err := uuidValue(review.WorkspaceID)
	if err != nil {
		return governance.Review{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	reviewID, err := uuidValue(review.ID)
	if err != nil {
		return governance.Review{}, fmt.Errorf("encode review ID: %w", err)
	}
	proposalID, err := uuidValue(review.ProposalID)
	if err != nil {
		return governance.Review{}, fmt.Errorf("encode proposal ID: %w", err)
	}
	reviewerID, err := uuidValue(review.ReviewerPrincipalID)
	if err != nil {
		return governance.Review{}, fmt.Errorf("encode reviewer principal ID: %w", err)
	}
	row, err := store.queries.CreateReview(ctx, dbgen.CreateReviewParams{
		ID: reviewID, WorkspaceID: workspaceID, ProposalID: proposalID,
		ReviewerPrincipalID: reviewerID, Channel: string(review.Channel),
		Decision: string(review.Decision), Note: review.Note, CreatedAt: timestamp(review.CreatedAt),
	})
	if err != nil {
		return governance.Review{}, governanceRepositoryError("create review", err)
	}
	return reviewFromRow(row)
}

func (store *Store) ListProposalReviews(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) ([]governance.Review, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal)
	if err != nil {
		return nil, fmt.Errorf("encode proposal ID: %w", err)
	}
	rows, err := store.queries.ListProposalReviews(ctx, dbgen.ListProposalReviewsParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
	})
	if err != nil {
		return nil, governanceRepositoryError("list proposal reviews", err)
	}
	reviews := make([]governance.Review, 0, len(rows))
	for _, row := range rows {
		review, mapErr := reviewFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

// ---------- validation ----------

func (store *Store) CreateValidationRun(ctx context.Context, run governance.ValidationRun) (governance.ValidationRun, error) {
	workspaceID, err := uuidValue(run.WorkspaceID)
	if err != nil {
		return governance.ValidationRun{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	runID, err := uuidValue(run.ID)
	if err != nil {
		return governance.ValidationRun{}, fmt.Errorf("encode validation run ID: %w", err)
	}
	proposalID, err := uuidValue(run.ProposalID)
	if err != nil {
		return governance.ValidationRun{}, fmt.Errorf("encode proposal ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.ValidationRun{}, governanceRepositoryError("begin validation run", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	row, err := queries.CreateValidationRun(ctx, dbgen.CreateValidationRunParams{
		ID: runID, WorkspaceID: workspaceID, ProposalID: proposalID,
		ValidatorID: run.ValidatorID, ValidatorVersion: run.ValidatorVersion,
		Status: string(run.Status), StartedAt: timestamp(run.StartedAt),
	})
	if err != nil {
		return governance.ValidationRun{}, governanceRepositoryError("create validation run", err)
	}
	created, err := validationRunFromRow(row)
	if err != nil {
		return governance.ValidationRun{}, err
	}
	runtimeID, err := identity.NewRunID()
	if err != nil {
		return governance.ValidationRun{}, err
	}
	event, err := newRuntimeEvent(run.WorkspaceID, runtimeID, "running", operationsdomain.RunEventState,
		operationsdomain.RunRunning, "running", "", "", run.StartedAt)
	if err != nil {
		return governance.ValidationRun{}, err
	}
	if err := projectRuntime(ctx, queries, operationsdomain.RuntimeRun{ID: runtimeID, WorkspaceID: run.WorkspaceID,
		Kind: operationsdomain.RunKindValidation, SourceType: "validation_run", SourceID: run.ID.String(),
		SourceVersionDigest: runtimeDigest(run.ProposalID.String(), run.ValidatorID, run.ValidatorVersion),
		IdempotencyKey:      boundedRuntimeIdempotencyKey("runtime:validation:", run.ID.String()), State: operationsdomain.RunRunning, Phase: "running",
		MaxAttempts: 1, StartedAt: timePointer(run.StartedAt), Version: 1,
		CreatedAt: run.StartedAt.UTC(), UpdatedAt: run.StartedAt.UTC()}, event); err != nil {
		return governance.ValidationRun{}, governanceRepositoryError("project validation run", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.ValidationRun{}, governanceRepositoryError("commit validation run", err)
	}
	return created, nil
}

func (store *Store) GetValidationRun(
	ctx context.Context, workspace identity.WorkspaceID, run identity.ValidationRunID,
) (governance.ValidationRun, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.ValidationRun{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	runID, err := uuidValue(run)
	if err != nil {
		return governance.ValidationRun{}, fmt.Errorf("encode validation run ID: %w", err)
	}
	row, err := store.queries.GetValidationRun(ctx, dbgen.GetValidationRunParams{
		WorkspaceID: workspaceID, ValidationRunID: runID,
	})
	if err != nil {
		return governance.ValidationRun{}, governanceRepositoryError("get validation run", err)
	}
	return validationRunFromRow(row)
}

func (store *Store) FinishValidationRun(ctx context.Context, command governanceapp.ValidationRunFinishCommand) (governance.ValidationRun, error) {
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return governance.ValidationRun{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	runID, err := uuidValue(command.RunID)
	if err != nil {
		return governance.ValidationRun{}, fmt.Errorf("encode validation run ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.ValidationRun{}, governanceRepositoryError("begin validation finish", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	row, err := queries.FinishValidationRun(ctx, dbgen.FinishValidationRunParams{
		WorkspaceID: workspaceID, ValidationRunID: runID, Status: string(command.Status),
		FinishedAt: timestamp(command.FinishedAt),
	})
	if err != nil {
		return governance.ValidationRun{}, governanceRepositoryError("finish validation run", err)
	}
	finished, err := validationRunFromRow(row)
	if err != nil {
		return governance.ValidationRun{}, err
	}
	stored, err := queries.GetOperationsRuntimeRunBySource(ctx, dbgen.GetOperationsRuntimeRunBySourceParams{
		WorkspaceID: workspaceID, Kind: string(operationsdomain.RunKindValidation),
		SourceType: "validation_run", SourceID: command.RunID.String()})
	if err != nil {
		return governance.ValidationRun{}, governanceRepositoryError("load validation runtime projection", err)
	}
	projected, err := runtimeRunFromRow(stored)
	if err != nil {
		return governance.ValidationRun{}, err
	}
	projected.State, projected.Phase, projected.FinishedAt, projected.UpdatedAt = operationsdomain.RunState(command.Status),
		string(command.Status), timePointer(command.FinishedAt), command.FinishedAt.UTC()
	if projected.State == operationsdomain.RunFailed {
		projected.ErrorCode, projected.ErrorSummary = "VALIDATION_FAILED", "validation run failed"
	}
	event, err := newRuntimeEvent(command.WorkspaceID, projected.ID, "state:"+string(command.Status), operationsdomain.RunEventState,
		projected.State, projected.Phase, projected.ErrorCode, projected.ErrorSummary, command.FinishedAt)
	if err != nil {
		return governance.ValidationRun{}, err
	}
	if err := projectRuntime(ctx, queries, projected, event); err != nil {
		return governance.ValidationRun{}, governanceRepositoryError("project validation finish", err)
	}
	if err := projectValidationAttention(ctx, queries, row, command.FinishedAt); err != nil {
		return governance.ValidationRun{}, governanceRepositoryError("project validation attention", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.ValidationRun{}, governanceRepositoryError("commit validation finish", err)
	}
	return finished, nil
}

func (store *Store) CreateValidationResult(ctx context.Context, result governance.ValidationResult) (governance.ValidationResult, error) {
	if err := result.Validate(); err != nil {
		return governance.ValidationResult{}, err
	}
	workspaceID, err := uuidValue(result.WorkspaceID)
	if err != nil {
		return governance.ValidationResult{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	resultID, err := uuidValue(result.ID)
	if err != nil {
		return governance.ValidationResult{}, fmt.Errorf("encode validation result ID: %w", err)
	}
	runID, err := uuidValue(result.ValidationRunID)
	if err != nil {
		return governance.ValidationResult{}, fmt.Errorf("encode validation run ID: %w", err)
	}
	row, err := store.queries.CreateValidationResult(ctx, dbgen.CreateValidationResultParams{
		ID: resultID, WorkspaceID: workspaceID, ValidationRunID: runID,
		Severity: string(result.Severity), Code: result.Code, Message: result.Message,
		InputDigest: result.InputDigest, Details: objectJSON(result.Details), CreatedAt: timestamp(result.CreatedAt),
	})
	if err != nil {
		return governance.ValidationResult{}, governanceRepositoryError("create validation result", err)
	}
	return validationResultFromRow(row)
}

func (store *Store) ListRunResults(
	ctx context.Context, workspace identity.WorkspaceID, run identity.ValidationRunID,
) ([]governance.ValidationResult, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	runID, err := uuidValue(run)
	if err != nil {
		return nil, fmt.Errorf("encode validation run ID: %w", err)
	}
	rows, err := store.queries.ListValidationRunResults(ctx, dbgen.ListValidationRunResultsParams{
		WorkspaceID: workspaceID, ValidationRunID: runID,
	})
	if err != nil {
		return nil, governanceRepositoryError("list validation results", err)
	}
	results := make([]governance.ValidationResult, 0, len(rows))
	for _, row := range rows {
		mapped, mapErr := validationResultFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		results = append(results, mapped)
	}
	return results, nil
}

func (store *Store) CountBlockingResults(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) (int, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return 0, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal)
	if err != nil {
		return 0, fmt.Errorf("encode proposal ID: %w", err)
	}
	count, err := store.queries.CountBlockingValidationResults(ctx, dbgen.CountBlockingValidationResultsParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
	})
	if err != nil {
		return 0, governanceRepositoryError("count blocking validation results", err)
	}
	return int(count), nil
}

// ---------- policy ----------

func (store *Store) CreatePolicyDecision(ctx context.Context, command governanceapp.PolicyDecisionCommand) (governance.PolicyDecision, error) {
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return governance.PolicyDecision{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	decisionID, err := uuidValue(command.ID)
	if err != nil {
		return governance.PolicyDecision{}, fmt.Errorf("encode policy decision ID: %w", err)
	}
	proposalID, err := uuidValue(command.ProposalID)
	if err != nil {
		return governance.PolicyDecision{}, fmt.Errorf("encode proposal ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.PolicyDecision{}, governanceRepositoryError("begin policy decision", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	row, err := queries.CreatePolicyDecision(ctx, dbgen.CreatePolicyDecisionParams{
		ID: decisionID, WorkspaceID: workspaceID, ProposalID: proposalID,
		RuleVersion: command.RuleVersion, Inputs: command.Inputs, InputsDigest: command.InputsDigest,
		MatchedPolicy: command.MatchedPolicy, RiskLevel: string(command.RiskLevel),
		Routing: string(command.Routing), ReasonCode: command.ReasonCode,
		DecidedAt: timestamp(command.DecidedAt), CreatedAt: timestamp(command.DecidedAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// The UNIQUE (proposal_id, rule_version, inputs_digest) index
		// collapsed a duplicate: the recomputable fact already exists, so the
		// stored decision is returned and the proposal link is left as the
		// original creation recorded it.
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return governance.PolicyDecision{}, governanceRepositoryError("rollback duplicate policy decision", rollbackErr)
		}
		existing, existingErr := store.getProposalPolicyDecisionByVersionDigest(
			ctx, command.WorkspaceID, command.ProposalID, command.RuleVersion, command.InputsDigest)
		if existingErr != nil {
			return governance.PolicyDecision{}, existingErr
		}
		return existing, nil
	}
	if err != nil {
		return governance.PolicyDecision{}, governanceRepositoryError("create policy decision", err)
	}
	if command.LinkProposal {
		if _, err := queries.LinkProposalPolicyDecision(ctx, dbgen.LinkProposalPolicyDecisionParams{
			WorkspaceID: workspaceID, ProposalID: proposalID, PolicyDecisionID: decisionID,
			RiskLevel: textValue(string(command.RiskLevel)), UpdatedAt: timestamp(command.DecidedAt),
		}); err != nil {
			return governance.PolicyDecision{}, governanceRepositoryError("link policy decision", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.PolicyDecision{}, governanceRepositoryError("commit policy decision", err)
	}
	return policyDecisionFromRow(row)
}

func (store *Store) GetPolicyDecision(
	ctx context.Context, workspace identity.WorkspaceID, decision identity.PolicyDecisionID,
) (governance.PolicyDecision, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.PolicyDecision{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	decisionID, err := uuidValue(decision)
	if err != nil {
		return governance.PolicyDecision{}, fmt.Errorf("encode policy decision ID: %w", err)
	}
	row, err := store.queries.GetPolicyDecision(ctx, dbgen.GetPolicyDecisionParams{
		WorkspaceID: workspaceID, PolicyDecisionID: decisionID,
	})
	if err != nil {
		return governance.PolicyDecision{}, governanceRepositoryError("get policy decision", err)
	}
	return policyDecisionFromRow(row)
}

// ---------- releases ----------

func (store *Store) CutRelease(ctx context.Context, command governanceapp.ReleaseCutCommand) (governance.Release, error) {
	workspaceID, err := uuidValue(command.Release.WorkspaceID)
	if err != nil {
		return governance.Release{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	releaseID, err := uuidValue(command.Release.ID)
	if err != nil {
		return governance.Release{}, fmt.Errorf("encode release ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.Release{}, governanceRepositoryError("begin release cut", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	if _, err := queries.LockWorkspace(ctx, workspaceID); err != nil {
		return governance.Release{}, governanceRepositoryError("lock workspace for release cut", err)
	}
	sequence, err := queries.NextReleaseSequence(ctx, workspaceID)
	if err != nil {
		return governance.Release{}, governanceRepositoryError("allocate release sequence", err)
	}
	var proposalID pgtype.UUID
	var originProposalID pgtype.UUID
	if command.ProposalID != nil {
		if proposalID, err = uuidValue(*command.ProposalID); err != nil {
			return governance.Release{}, fmt.Errorf("encode proposal ID: %w", err)
		}
		originProposalID = proposalID
		locked, lockErr := queries.GetProposalForUpdate(ctx, dbgen.GetProposalForUpdateParams{
			WorkspaceID: workspaceID, ProposalID: proposalID,
		})
		if lockErr != nil {
			return governance.Release{}, governanceRepositoryError("lock proposal for release", lockErr)
		}
		if locked.State != string(governance.ProposalInReview) {
			return governance.Release{}, fmt.Errorf(
				"%w: proposal is %s, not in_review", governance.ErrInvariant, locked.State)
		}
	}
	created, err := queries.CreateRelease(ctx, dbgen.CreateReleaseParams{
		ID: releaseID, WorkspaceID: workspaceID, Sequence: sequence,
		ManifestDigest: command.Release.ManifestDigest, State: string(command.Release.State),
		RolledBackToReleaseID: pgtype.UUID{}, OriginProposalID: originProposalID,
		PublishedBy: command.Release.PublishedBy,
		PublishedAt: timestamp(command.Release.PublishedAt), CreatedAt: timestamp(command.Release.CreatedAt),
	})
	if err != nil {
		return governance.Release{}, governanceRepositoryError("create release", err)
	}
	for _, entry := range command.Entries {
		assetID, entryErr := uuidValue(entry.AssetID)
		if entryErr != nil {
			return governance.Release{}, fmt.Errorf("encode manifest asset ID: %w", entryErr)
		}
		revisionID, entryErr := uuidValue(entry.RevisionID)
		if entryErr != nil {
			return governance.Release{}, fmt.Errorf("encode manifest revision ID: %w", entryErr)
		}
		if _, entryErr := queries.GetAssetRevisionOwnership(ctx, dbgen.GetAssetRevisionOwnershipParams{
			WorkspaceID: workspaceID, AssetID: assetID, RevisionID: revisionID,
		}); entryErr != nil {
			return governance.Release{}, governanceRepositoryError("verify manifest entry", entryErr)
		}
		if entryErr := queries.CreateReleaseAsset(ctx, dbgen.CreateReleaseAssetParams{
			WorkspaceID: workspaceID, ReleaseID: releaseID, AssetID: assetID, RevisionID: revisionID,
			Compatibility: objectJSON(entry.Compatibility), Position: int32(entry.Position),
			CreatedAt: timestamp(command.Release.CreatedAt),
		}); entryErr != nil {
			return governance.Release{}, governanceRepositoryError("create release asset", entryErr)
		}
	}
	if command.ProposalID != nil {
		decidedAt := command.Release.PublishedAt
		if _, err := queries.TransitionProposal(ctx, dbgen.TransitionProposalParams{
			WorkspaceID: workspaceID, ProposalID: proposalID, State: string(governance.ProposalReleased),
			DecidedAt: timestamp(decidedAt), ExpectedState: string(governance.ProposalInReview),
			UpdatedAt: timestamp(decidedAt),
		}); err != nil {
			return governance.Release{}, governanceRepositoryError("release proposal", err)
		}
		if command.HasProposalEvents {
			if err := createProposalMutationEvents(ctx, queries, proposalEvent{
				WorkspaceID: command.Release.WorkspaceID, ProposalID: *command.ProposalID,
				AuditID: command.ProposalAuditEventID, OutboxID: command.ProposalOutboxEventID,
				Action: "released", FromState: string(governance.ProposalInReview),
				ToState: string(governance.ProposalReleased), Actor: command.Release.PublishedBy,
				TraceID: command.TraceID, CreatedAt: command.Release.PublishedAt,
			}); err != nil {
				return governance.Release{}, governanceRepositoryError("record release proposal events", err)
			}
		}
	}
	if err := createReleaseMutationEvents(ctx, queries, releaseEvent{
		WorkspaceID: command.Release.WorkspaceID, ReleaseID: command.Release.ID,
		AuditID: command.ReleaseEvents.AuditEventID, OutboxID: command.ReleaseEvents.OutboxEventID,
		Action: "published", ManifestDigest: command.Release.ManifestDigest, Sequence: sequence,
		Actor: command.Release.PublishedBy, TraceID: command.TraceID, CreatedAt: command.Release.PublishedAt,
	}); err != nil {
		return governance.Release{}, governanceRepositoryError("record release events", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.Release{}, governanceRepositoryError("commit release cut", err)
	}
	release, err := releaseFromRow(created)
	if err != nil {
		return governance.Release{}, err
	}
	release.Entries = append([]governance.ManifestEntry(nil), command.Entries...)
	return release, nil
}

func (store *Store) RollbackRelease(ctx context.Context, command governanceapp.ReleaseRollbackCommand) (governance.Release, error) {
	workspaceID, err := uuidValue(command.Release.WorkspaceID)
	if err != nil {
		return governance.Release{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	releaseID, err := uuidValue(command.Release.ID)
	if err != nil {
		return governance.Release{}, fmt.Errorf("encode rollback release ID: %w", err)
	}
	targetID, err := uuidValue(*command.Release.RolledBackToReleaseID)
	if err != nil {
		return governance.Release{}, fmt.Errorf("encode rollback target ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.Release{}, governanceRepositoryError("begin release rollback", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	if _, err := queries.LockWorkspace(ctx, workspaceID); err != nil {
		return governance.Release{}, governanceRepositoryError("lock workspace for rollback", err)
	}
	sequence, err := queries.NextReleaseSequence(ctx, workspaceID)
	if err != nil {
		return governance.Release{}, governanceRepositoryError("allocate rollback sequence", err)
	}
	created, err := queries.CreateRelease(ctx, dbgen.CreateReleaseParams{
		ID: releaseID, WorkspaceID: workspaceID, Sequence: sequence,
		ManifestDigest: command.Release.ManifestDigest, State: string(command.Release.State),
		RolledBackToReleaseID: targetID, OriginProposalID: pgtype.UUID{},
		PublishedBy: command.Release.PublishedBy,
		PublishedAt: timestamp(command.Release.PublishedAt), CreatedAt: timestamp(command.Release.CreatedAt),
	})
	if err != nil {
		return governance.Release{}, governanceRepositoryError("create rollback release", err)
	}
	for _, entry := range command.Entries {
		assetID, entryErr := uuidValue(entry.AssetID)
		if entryErr != nil {
			return governance.Release{}, fmt.Errorf("encode rollback asset ID: %w", entryErr)
		}
		revisionID, entryErr := uuidValue(entry.RevisionID)
		if entryErr != nil {
			return governance.Release{}, fmt.Errorf("encode rollback revision ID: %w", entryErr)
		}
		if entryErr := queries.CreateReleaseAsset(ctx, dbgen.CreateReleaseAssetParams{
			WorkspaceID: workspaceID, ReleaseID: releaseID, AssetID: assetID, RevisionID: revisionID,
			Compatibility: objectJSON(entry.Compatibility), Position: int32(entry.Position),
			CreatedAt: timestamp(command.Release.CreatedAt),
		}); entryErr != nil {
			return governance.Release{}, governanceRepositoryError("create rollback release asset", entryErr)
		}
	}
	if err := createReleaseMutationEvents(ctx, queries, releaseEvent{
		WorkspaceID: command.Release.WorkspaceID, ReleaseID: command.Release.ID,
		AuditID: command.ReleaseEvents.AuditEventID, OutboxID: command.ReleaseEvents.OutboxEventID,
		Action: "rolled_back", ManifestDigest: command.Release.ManifestDigest, Sequence: sequence,
		RolledBackToReleaseID: command.Release.RolledBackToReleaseID,
		Actor:                 command.Release.PublishedBy, TraceID: command.TraceID, CreatedAt: command.Release.PublishedAt,
	}); err != nil {
		return governance.Release{}, governanceRepositoryError("record rollback events", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.Release{}, governanceRepositoryError("commit release rollback", err)
	}
	release, err := releaseFromRow(created)
	if err != nil {
		return governance.Release{}, err
	}
	release.Entries = append([]governance.ManifestEntry(nil), command.Entries...)
	return release, nil
}

func (store *Store) GetRelease(
	ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID,
) (governance.Release, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.Release{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	releaseID, err := uuidValue(release)
	if err != nil {
		return governance.Release{}, fmt.Errorf("encode release ID: %w", err)
	}
	row, err := store.queries.GetRelease(ctx, dbgen.GetReleaseParams{WorkspaceID: workspaceID, ReleaseID: releaseID})
	if err != nil {
		return governance.Release{}, governanceRepositoryError("get release", err)
	}
	return releaseFromRow(row)
}

func (store *Store) ListReleaseAssets(
	ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID,
) ([]governance.ManifestEntry, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	releaseID, err := uuidValue(release)
	if err != nil {
		return nil, fmt.Errorf("encode release ID: %w", err)
	}
	rows, err := store.queries.ListReleaseAssets(ctx, dbgen.ListReleaseAssetsParams{
		WorkspaceID: workspaceID, ReleaseID: releaseID,
	})
	if err != nil {
		return nil, governanceRepositoryError("list release assets", err)
	}
	entries := make([]governance.ManifestEntry, 0, len(rows))
	for _, row := range rows {
		entry, mapErr := manifestEntryFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (store *Store) GetPreviousRelease(
	ctx context.Context, workspace identity.WorkspaceID, sequence int64,
) (governance.Release, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.Release{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	row, err := store.queries.GetPreviousRelease(ctx, dbgen.GetPreviousReleaseParams{
		WorkspaceID: workspaceID, Sequence: sequence,
	})
	if err != nil {
		return governance.Release{}, governanceRepositoryError("get previous release", err)
	}
	return releaseFromRow(row)
}

func (store *Store) CountRollbacksFor(
	ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID,
) (int, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return 0, fmt.Errorf("encode workspace ID: %w", err)
	}
	releaseID, err := uuidValue(release)
	if err != nil {
		return 0, fmt.Errorf("encode release ID: %w", err)
	}
	count, err := store.queries.CountReleaseRollbacks(ctx, dbgen.CountReleaseRollbacksParams{
		WorkspaceID: workspaceID, ReleaseID: releaseID,
	})
	if err != nil {
		return 0, governanceRepositoryError("count release rollbacks", err)
	}
	return int(count), nil
}

// ---------- agent runs ----------

func (store *Store) CreateAgentRun(ctx context.Context, run governance.AgentRun) (governance.AgentRun, error) {
	workspaceID, err := uuidValue(run.WorkspaceID)
	if err != nil {
		return governance.AgentRun{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	runID, err := uuidValue(run.ID)
	if err != nil {
		return governance.AgentRun{}, fmt.Errorf("encode agent run ID: %w", err)
	}
	principalID := pgtype.UUID{}
	if run.PrincipalID != nil {
		if principalID, err = uuidValue(*run.PrincipalID); err != nil {
			return governance.AgentRun{}, fmt.Errorf("encode principal ID: %w", err)
		}
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.AgentRun{}, governanceRepositoryError("begin agent run", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	row, err := queries.CreateAgentRun(ctx, dbgen.CreateAgentRunParams{
		ID: runID, WorkspaceID: workspaceID, PrincipalID: principalID,
		Model: run.Model, ConfigRevision: run.ConfigRevision, InputHash: run.InputHash,
		Status: string(run.Status), CostMicros: run.CostMicros,
		StartedAt: timestamp(run.StartedAt), CreatedAt: timestamp(run.CreatedAt),
	})
	if err != nil {
		return governance.AgentRun{}, governanceRepositoryError("create agent run", err)
	}
	created, err := agentRunFromRow(row)
	if err != nil {
		return governance.AgentRun{}, err
	}
	runtimeID, err := identity.NewRunID()
	if err != nil {
		return governance.AgentRun{}, err
	}
	event, err := newRuntimeEvent(run.WorkspaceID, runtimeID, "running", operationsdomain.RunEventState,
		operationsdomain.RunRunning, "running", "", "", run.StartedAt)
	if err != nil {
		return governance.AgentRun{}, err
	}
	if err := projectRuntime(ctx, queries, operationsdomain.RuntimeRun{ID: runtimeID, WorkspaceID: run.WorkspaceID,
		Kind: operationsdomain.RunKindAgent, SourceType: "agent_run", SourceID: run.ID.String(),
		SourceVersionDigest: normalizedRuntimeDigest(run.InputHash), IdempotencyKey: boundedRuntimeIdempotencyKey("runtime:agent:", run.ID.String()),
		RequestedByPrincipalID: run.PrincipalID, State: operationsdomain.RunRunning, Phase: "running", MaxAttempts: 1,
		StartedAt: timePointer(run.StartedAt), Version: 1, CreatedAt: run.CreatedAt.UTC(), UpdatedAt: run.CreatedAt.UTC()}, event); err != nil {
		return governance.AgentRun{}, governanceRepositoryError("project agent run", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.AgentRun{}, governanceRepositoryError("commit agent run", err)
	}
	return created, nil
}

func (store *Store) GetAgentRun(
	ctx context.Context, workspace identity.WorkspaceID, run identity.AgentRunID,
) (governance.AgentRun, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.AgentRun{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	runID, err := uuidValue(run)
	if err != nil {
		return governance.AgentRun{}, fmt.Errorf("encode agent run ID: %w", err)
	}
	row, err := store.queries.GetAgentRun(ctx, dbgen.GetAgentRunParams{WorkspaceID: workspaceID, AgentRunID: runID})
	if err != nil {
		return governance.AgentRun{}, governanceRepositoryError("get agent run", err)
	}
	return agentRunFromRow(row)
}

func (store *Store) FinishAgentRun(ctx context.Context, command governanceapp.AgentRunFinishCommand) (governance.AgentRun, error) {
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return governance.AgentRun{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	runID, err := uuidValue(command.RunID)
	if err != nil {
		return governance.AgentRun{}, fmt.Errorf("encode agent run ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.AgentRun{}, governanceRepositoryError("begin agent finish", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	row, err := queries.FinishAgentRun(ctx, dbgen.FinishAgentRunParams{
		WorkspaceID: workspaceID, AgentRunID: runID, Status: string(command.FinalState),
		OutputDigest: optionalTextFromPointer(command.OutputDigest), CostMicros: command.CostMicros,
		FinishedAt: timestamp(command.FinishedAt), DurationMs: pgtype.Int8{Int64: command.DurationMS, Valid: true},
	})
	if err != nil {
		return governance.AgentRun{}, governanceRepositoryError("finish agent run", err)
	}
	finished, err := agentRunFromRow(row)
	if err != nil {
		return governance.AgentRun{}, err
	}
	stored, err := queries.GetOperationsRuntimeRunBySource(ctx, dbgen.GetOperationsRuntimeRunBySourceParams{
		WorkspaceID: workspaceID, Kind: string(operationsdomain.RunKindAgent), SourceType: "agent_run", SourceID: command.RunID.String()})
	if err != nil {
		return governance.AgentRun{}, governanceRepositoryError("load agent runtime projection", err)
	}
	projected, err := runtimeRunFromRow(stored)
	if err != nil {
		return governance.AgentRun{}, err
	}
	projected.State, projected.Phase, projected.FinishedAt, projected.UpdatedAt = operationsdomain.RunState(command.FinalState),
		string(command.FinalState), timePointer(command.FinishedAt), command.FinishedAt.UTC()
	if projected.State == operationsdomain.RunFailed {
		projected.ErrorCode, projected.ErrorSummary = "AGENT_FAILED", "agent run failed"
	}
	event, err := newRuntimeEvent(command.WorkspaceID, projected.ID, "state:"+string(command.FinalState), operationsdomain.RunEventState,
		projected.State, projected.Phase, projected.ErrorCode, projected.ErrorSummary, command.FinishedAt)
	if err != nil {
		return governance.AgentRun{}, err
	}
	if err := projectRuntime(ctx, queries, projected, event); err != nil {
		return governance.AgentRun{}, governanceRepositoryError("project agent finish", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.AgentRun{}, governanceRepositoryError("commit agent finish", err)
	}
	return finished, nil
}

func (store *Store) CreateAgentStep(ctx context.Context, step governance.AgentStep) (governance.AgentStep, error) {
	if err := step.Validate(); err != nil {
		return governance.AgentStep{}, err
	}
	workspaceID, err := uuidValue(step.WorkspaceID)
	if err != nil {
		return governance.AgentStep{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	stepID, err := uuidValue(step.ID)
	if err != nil {
		return governance.AgentStep{}, fmt.Errorf("encode agent step ID: %w", err)
	}
	runID, err := uuidValue(step.AgentRunID)
	if err != nil {
		return governance.AgentStep{}, fmt.Errorf("encode agent run ID: %w", err)
	}
	row, err := store.queries.CreateAgentStep(ctx, dbgen.CreateAgentStepParams{
		ID: stepID, WorkspaceID: workspaceID, AgentRunID: runID, Sequence: int32(step.Sequence),
		Kind: string(step.Kind), ToolName: optionalTextPointer(step.ToolName),
		InputHash: step.InputHash, OutputHash: step.OutputHash,
		ErrorCode: optionalTextPointer(step.ErrorCode), CreatedAt: timestamp(step.CreatedAt),
	})
	if err != nil {
		return governance.AgentStep{}, governanceRepositoryError("create agent step", err)
	}
	return agentStepFromRow(row)
}

// ---------- row mappers ----------

func proposalFromRow(row dbgen.Proposal) (governance.Proposal, error) {
	id, err := identity.ProposalIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.Proposal{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.Proposal{}, err
	}
	proposal := governance.Proposal{
		ID: id, WorkspaceID: workspaceID, TargetObjectType: governance.TargetObjectType(row.TargetObjectType),
		TargetObjectID: row.TargetObjectID.String(), State: governance.ProposalState(row.State),
		Title: row.Title, Summary: row.Summary, Reason: row.Reason,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.AssetID.Valid {
		assetID, assetErr := identity.AssetIDFromUUIDBytes(row.AssetID.Bytes)
		if assetErr != nil {
			return governance.Proposal{}, assetErr
		}
		proposal.AssetID = &assetID
	}
	if row.BaseRevisionID.Valid {
		revisionID, revisionErr := identity.RevisionIDFromUUIDBytes(row.BaseRevisionID.Bytes)
		if revisionErr != nil {
			return governance.Proposal{}, revisionErr
		}
		proposal.BaseRevisionID = &revisionID
	}
	if row.RiskLevel.Valid {
		proposal.RiskLevel = governance.RiskLevel(row.RiskLevel.String)
	}
	if row.PolicyDecisionID.Valid {
		decisionID, decisionErr := identity.PolicyDecisionIDFromUUIDBytes(row.PolicyDecisionID.Bytes)
		if decisionErr != nil {
			return governance.Proposal{}, decisionErr
		}
		proposal.PolicyDecisionID = &decisionID
	}
	if row.AgentRunID.Valid {
		agentRunID, agentErr := identity.AgentRunIDFromUUIDBytes(row.AgentRunID.Bytes)
		if agentErr != nil {
			return governance.Proposal{}, agentErr
		}
		proposal.AgentRunID = &agentRunID
	}
	if row.SubmittedAt.Valid {
		submittedAt := row.SubmittedAt.Time.UTC()
		proposal.SubmittedAt = &submittedAt
	}
	if row.DecidedAt.Valid {
		decidedAt := row.DecidedAt.Time.UTC()
		proposal.DecidedAt = &decidedAt
	}
	if row.ProductionOperationID.Valid {
		opID, opErr := identity.ProductionOperationIDFromUUIDBytes(row.ProductionOperationID.Bytes)
		if opErr != nil {
			return governance.Proposal{}, opErr
		}
		proposal.ProductionOperationID = &opID
	}
	if row.ProductionVersion.Valid {
		v := int(row.ProductionVersion.Int32)
		proposal.ProductionVersion = &v
	}
	return proposal, nil
}

func changeItemFromRow(row dbgen.ProposalChange) (governance.ChangeSetItem, error) {
	id, err := identity.ProposalChangeIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.ChangeSetItem{}, err
	}
	proposalID, err := identity.ProposalIDFromUUIDBytes(row.ProposalID.Bytes)
	if err != nil {
		return governance.ChangeSetItem{}, err
	}
	return governance.ChangeSetItem{
		ID: id, ProposalID: proposalID, FieldPath: row.FieldPath, Op: governance.ChangeOp(row.Op),
		BeforeDigest: optionalText(row.BeforeDigest), AfterDigest: optionalText(row.AfterDigest),
		BeforeValue: cloneJSON(row.BeforeValue), AfterValue: cloneJSON(row.AfterValue),
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

func reviewFromRow(row dbgen.Review) (governance.Review, error) {
	id, err := identity.ReviewIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.Review{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.Review{}, err
	}
	proposalID, err := identity.ProposalIDFromUUIDBytes(row.ProposalID.Bytes)
	if err != nil {
		return governance.Review{}, err
	}
	reviewerID, err := identity.PrincipalIDFromUUIDBytes(row.ReviewerPrincipalID.Bytes)
	if err != nil {
		return governance.Review{}, err
	}
	return governance.Review{
		ID: id, WorkspaceID: workspaceID, ProposalID: proposalID, ReviewerPrincipalID: reviewerID,
		Channel: governance.ReviewChannel(row.Channel), Decision: governance.ReviewDecision(row.Decision),
		Note: row.Note, CreatedAt: row.CreatedAt.Time,
	}, nil
}

func validationRunFromRow(row dbgen.ValidationRun) (governance.ValidationRun, error) {
	id, err := identity.ValidationRunIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.ValidationRun{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.ValidationRun{}, err
	}
	proposalID, err := identity.ProposalIDFromUUIDBytes(row.ProposalID.Bytes)
	if err != nil {
		return governance.ValidationRun{}, err
	}
	run := governance.ValidationRun{
		ID: id, WorkspaceID: workspaceID, ProposalID: proposalID,
		ValidatorID: row.ValidatorID, ValidatorVersion: row.ValidatorVersion,
		Status: governance.ValidationStatus(row.Status), StartedAt: row.StartedAt.Time,
	}
	if row.FinishedAt.Valid {
		finishedAt := row.FinishedAt.Time.UTC()
		run.FinishedAt = &finishedAt
	}
	return run, nil
}

func validationResultFromRow(row dbgen.ValidationResult) (governance.ValidationResult, error) {
	id, err := identity.ValidationResultIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.ValidationResult{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.ValidationResult{}, err
	}
	runID, err := identity.ValidationRunIDFromUUIDBytes(row.ValidationRunID.Bytes)
	if err != nil {
		return governance.ValidationResult{}, err
	}
	return governance.ValidationResult{
		ID: id, WorkspaceID: workspaceID, ValidationRunID: runID,
		Severity: governance.ValidationSeverity(row.Severity), Code: row.Code, Message: row.Message,
		InputDigest: row.InputDigest, Details: cloneJSON(row.Details), CreatedAt: row.CreatedAt.Time,
	}, nil
}

func policyDecisionFromRow(row dbgen.PolicyDecision) (governance.PolicyDecision, error) {
	id, err := identity.PolicyDecisionIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	proposalID, err := identity.ProposalIDFromUUIDBytes(row.ProposalID.Bytes)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	return governance.PolicyDecision{
		ID: id, WorkspaceID: workspaceID, ProposalID: proposalID, RuleVersion: row.RuleVersion,
		Inputs: cloneJSON(row.Inputs), InputsDigest: row.InputsDigest, MatchedPolicy: row.MatchedPolicy,
		RiskLevel: governance.RiskLevel(row.RiskLevel), Routing: governance.RoutingChannel(row.Routing),
		ReasonCode: row.ReasonCode, DecidedAt: row.DecidedAt.Time,
	}, nil
}

func releaseFromRow(row dbgen.Release) (governance.Release, error) {
	id, err := identity.ReleaseIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.Release{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.Release{}, err
	}
	release := governance.Release{
		ID: id, WorkspaceID: workspaceID, Sequence: row.Sequence, ManifestDigest: row.ManifestDigest,
		State: governance.ReleaseState(row.State), PublishedBy: row.PublishedBy,
		PublishedAt: row.PublishedAt.Time, CreatedAt: row.CreatedAt.Time,
	}
	if row.RolledBackToReleaseID.Valid {
		targetID, targetErr := identity.ReleaseIDFromUUIDBytes(row.RolledBackToReleaseID.Bytes)
		if targetErr != nil {
			return governance.Release{}, targetErr
		}
		release.RolledBackToReleaseID = &targetID
	}
	if row.OriginProposalID.Valid {
		proposalID, proposalErr := identity.ProposalIDFromUUIDBytes(row.OriginProposalID.Bytes)
		if proposalErr != nil {
			return governance.Release{}, proposalErr
		}
		release.OriginProposalID = &proposalID
	}
	if row.ProductionRootReleaseID.Valid {
		rootID, rootErr := identity.ReleaseIDFromUUIDBytes(row.ProductionRootReleaseID.Bytes)
		if rootErr != nil {
			return governance.Release{}, rootErr
		}
		release.ProductionRootReleaseID = &rootID
	}
	if row.ProductionRollbackParentID.Valid {
		parentID, parentErr := identity.ReleaseIDFromUUIDBytes(row.ProductionRollbackParentID.Bytes)
		if parentErr != nil {
			return governance.Release{}, parentErr
		}
		release.ProductionRollbackParentID = &parentID
	}
	if row.ProductionRollbackDepth.Valid {
		d := int(row.ProductionRollbackDepth.Int32)
		release.ProductionRollbackDepth = &d
	}
	return release, nil
}

func manifestEntryFromRow(row dbgen.ReleaseAsset) (governance.ManifestEntry, error) {
	assetID, err := identity.AssetIDFromUUIDBytes(row.AssetID.Bytes)
	if err != nil {
		return governance.ManifestEntry{}, err
	}
	revisionID, err := identity.RevisionIDFromUUIDBytes(row.RevisionID.Bytes)
	if err != nil {
		return governance.ManifestEntry{}, err
	}
	return governance.ManifestEntry{
		AssetID: assetID, RevisionID: revisionID, Compatibility: cloneJSON(row.Compatibility),
		Position: int(row.Position),
	}, nil
}

func agentRunFromRow(row dbgen.AgentRun) (governance.AgentRun, error) {
	id, err := identity.AgentRunIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.AgentRun{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.AgentRun{}, err
	}
	run := governance.AgentRun{
		ID: id, WorkspaceID: workspaceID, Model: row.Model, ConfigRevision: row.ConfigRevision,
		InputHash: row.InputHash, Status: governance.AgentRunState(row.Status),
		CostMicros: row.CostMicros, StartedAt: row.StartedAt.Time, CreatedAt: row.CreatedAt.Time,
	}
	if row.PrincipalID.Valid {
		principalID, principalErr := identity.PrincipalIDFromUUIDBytes(row.PrincipalID.Bytes)
		if principalErr != nil {
			return governance.AgentRun{}, principalErr
		}
		run.PrincipalID = &principalID
	}
	if row.OutputDigest.Valid {
		outputDigest := row.OutputDigest.String
		run.OutputDigest = &outputDigest
	}
	if row.FinishedAt.Valid {
		finishedAt := row.FinishedAt.Time.UTC()
		run.FinishedAt = &finishedAt
	}
	if row.DurationMs.Valid {
		durationMS := row.DurationMs.Int64
		run.DurationMS = &durationMS
	}
	return run, nil
}

func agentStepFromRow(row dbgen.AgentStep) (governance.AgentStep, error) {
	id, err := identity.AgentStepIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.AgentStep{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.AgentStep{}, err
	}
	runID, err := identity.AgentRunIDFromUUIDBytes(row.AgentRunID.Bytes)
	if err != nil {
		return governance.AgentStep{}, err
	}
	step := governance.AgentStep{
		ID: id, WorkspaceID: workspaceID, AgentRunID: runID, Sequence: int(row.Sequence),
		Kind: governance.AgentStepKind(row.Kind), InputHash: row.InputHash, OutputHash: row.OutputHash,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.ToolName.Valid {
		toolName := row.ToolName.String
		step.ToolName = &toolName
	}
	if row.ErrorCode.Valid {
		errorCode := row.ErrorCode.String
		step.ErrorCode = &errorCode
	}
	return step, nil
}

func optionalTextContent(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return textValue(value)
}

func optionalTextFromPointer(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return textValue(*value)
}
