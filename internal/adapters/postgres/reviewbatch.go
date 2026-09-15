package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ governanceapp.ReviewBatchRepository = (*Store)(nil)

func reviewBatchRepositoryError(operation string, err error) error {
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

func (store *Store) CreateReviewBatches(
	ctx context.Context, assemblies []governanceapp.ReviewBatchAssembly,
) ([]governanceapp.ReviewBatchRows, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, reviewBatchRepositoryError("begin review batch assembly", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	rows := make([]governanceapp.ReviewBatchRows, 0, len(assemblies))
	for _, assembly := range assemblies {
		workspaceUUID, err := uuidValue(assembly.Batch.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("encode workspace ID: %w", err)
		}
		batchUUID, err := uuidValue(assembly.Batch.ID)
		if err != nil {
			return nil, fmt.Errorf("encode review batch ID: %w", err)
		}
		row, err := queries.CreateReviewBatch(ctx, dbgen.CreateReviewBatchParams{
			ID: batchUUID, WorkspaceID: workspaceUUID,
			GroupingRule:  mustGroupingRuleJSON(assembly.Batch.GroupingRule),
			PolicyVersion: assembly.Batch.PolicyVersion, CreatedBy: assembly.Batch.CreatedBy,
			CreatedAt: timestamp(assembly.Batch.CreatedAt),
		})
		if err != nil {
			return nil, reviewBatchRepositoryError("create review batch", err)
		}
		for _, member := range assembly.Members {
			proposalUUID, err := uuidValue(member.ProposalID)
			if err != nil {
				return nil, fmt.Errorf("encode member proposal ID: %w", err)
			}
			if err := queries.CreateReviewBatchMember(ctx, dbgen.CreateReviewBatchMemberParams{
				ReviewBatchID: batchUUID, WorkspaceID: workspaceUUID, ProposalID: proposalUUID,
				AddedReason: objectJSON(member.AddedReason), Sample: member.Sample,
				CreatedAt: timestamp(member.CreatedAt),
			}); err != nil {
				return nil, reviewBatchRepositoryError("create review batch member", err)
			}
		}
		if err := createReviewBatchAuditEvent(ctx, queries, reviewBatchEvent{
			WorkspaceID: assembly.Batch.WorkspaceID, BatchID: assembly.Batch.ID,
			AuditID: assembly.AuditEventID, Action: "assembled",
			GroupingRule: assembly.Batch.GroupingRule, PolicyVersion: assembly.Batch.PolicyVersion,
			MemberCount: len(assembly.Members), Actor: assembly.Actor,
			TraceID: assembly.TraceID, CreatedAt: assembly.Batch.CreatedAt,
		}); err != nil {
			return nil, reviewBatchRepositoryError("record review batch assembly audit", err)
		}
		batch, err := reviewBatchFromRow(row)
		if err != nil {
			return nil, err
		}
		members, err := txReviewBatchMembers(ctx, queries, assembly.Batch.WorkspaceID, assembly.Batch.ID)
		if err != nil {
			return nil, err
		}
		rows = append(rows, governanceapp.ReviewBatchRows{Batch: batch, Members: members})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, reviewBatchRepositoryError("commit review batch assembly", err)
	}
	return rows, nil
}

func (store *Store) GetReviewBatchWithMembers(
	ctx context.Context, workspace identity.WorkspaceID, batch identity.ReviewBatchID,
) (governanceapp.ReviewBatchRows, error) {
	workspaceUUID, err := uuidValue(workspace)
	if err != nil {
		return governanceapp.ReviewBatchRows{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	batchUUID, err := uuidValue(batch)
	if err != nil {
		return governanceapp.ReviewBatchRows{}, fmt.Errorf("encode review batch ID: %w", err)
	}
	row, err := store.queries.GetReviewBatch(ctx, dbgen.GetReviewBatchParams{
		WorkspaceID: workspaceUUID, ReviewBatchID: batchUUID,
	})
	if err != nil {
		return governanceapp.ReviewBatchRows{}, reviewBatchRepositoryError("get review batch", err)
	}
	record, err := reviewBatchFromRow(row)
	if err != nil {
		return governanceapp.ReviewBatchRows{}, err
	}
	members, err := store.reviewBatchMembers(ctx, workspace, batch)
	if err != nil {
		return governanceapp.ReviewBatchRows{}, err
	}
	return governanceapp.ReviewBatchRows{Batch: record, Members: members}, nil
}

func (store *Store) ListOpenReviewBatches(
	ctx context.Context, workspace identity.WorkspaceID, limit int, cursor *governanceapp.ReviewBatchCursor,
) ([]governanceapp.ReviewBatchRows, error) {
	workspaceUUID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	params := dbgen.ListOpenReviewBatchesParams{
		WorkspaceID: workspaceUUID, HasCursor: cursor != nil, PageLimit: int32(limit),
	}
	if cursor != nil {
		cursorID, cursorErr := uuidValue(cursor.ID)
		if cursorErr != nil {
			return nil, fmt.Errorf("encode cursor ID: %w", cursorErr)
		}
		params.CursorCreatedAt = timestamp(cursor.CreatedAt)
		params.CursorID = cursorID
	}
	batchRows, err := store.queries.ListOpenReviewBatches(ctx, params)
	if err != nil {
		return nil, reviewBatchRepositoryError("list open review batches", err)
	}
	rows := make([]governanceapp.ReviewBatchRows, 0, len(batchRows))
	for _, batchRow := range batchRows {
		batch, err := reviewBatchFromRow(batchRow)
		if err != nil {
			return nil, err
		}
		members, err := store.reviewBatchMembers(ctx, workspace, batch.ID)
		if err != nil {
			return nil, err
		}
		rows = append(rows, governanceapp.ReviewBatchRows{Batch: batch, Members: members})
	}
	return rows, nil
}

func (store *Store) ListBatchEligibleProposals(
	ctx context.Context, workspace identity.WorkspaceID,
) ([]governanceapp.BatchEligibleProposal, error) {
	workspaceUUID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	rows, err := store.queries.ListBatchEligibleProposals(ctx, workspaceUUID)
	if err != nil {
		return nil, reviewBatchRepositoryError("list batch eligible proposals", err)
	}
	eligible := make([]governanceapp.BatchEligibleProposal, 0, len(rows))
	for _, row := range rows {
		proposal, err := eligibleProposalFromRow(row)
		if err != nil {
			return nil, err
		}
		eligible = append(eligible, proposal)
	}
	return eligible, nil
}

func (store *Store) CountProposalReviews(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
	reviewer identity.PrincipalID, channel governance.ReviewChannel,
) (int, error) {
	workspaceUUID, err := uuidValue(workspace)
	if err != nil {
		return 0, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalUUID, err := uuidValue(proposal)
	if err != nil {
		return 0, fmt.Errorf("encode proposal ID: %w", err)
	}
	reviewerUUID, err := uuidValue(reviewer)
	if err != nil {
		return 0, fmt.Errorf("encode reviewer principal ID: %w", err)
	}
	count, err := store.queries.CountProposalReviewsByReviewer(ctx, dbgen.CountProposalReviewsByReviewerParams{
		WorkspaceID: workspaceUUID, ProposalID: proposalUUID,
		ReviewerPrincipalID: reviewerUUID, Channel: string(channel),
	})
	if err != nil {
		return 0, reviewBatchRepositoryError("count proposal reviews", err)
	}
	return int(count), nil
}

func (store *Store) RecordReviewDenial(ctx context.Context, denial governanceapp.ReviewDenialRecord) error {
	workspaceUUID, err := uuidValue(denial.WorkspaceID)
	if err != nil {
		return fmt.Errorf("encode workspace ID: %w", err)
	}
	auditID, err := uuidValue(denial.AuditEventID)
	if err != nil {
		return fmt.Errorf("encode audit event ID: %w", err)
	}
	proposals := make([]string, 0, len(denial.Proposals))
	for _, proposal := range denial.Proposals {
		proposals = append(proposals, proposal.String())
	}
	payload, err := json.Marshal(map[string]any{
		"specVersion":  "semlia.governance-review/v1",
		"action":       "denied",
		"conflict":     denial.Conflict,
		"scope":        denial.Scope,
		"policySource": denial.PolicySource,
		"recovery":     denial.Recovery,
		"proposals":    proposals,
	})
	if err != nil {
		return fmt.Errorf("encode review denial payload: %w", err)
	}
	if err := store.queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		ID: auditID, WorkspaceID: workspaceUUID, EventType: "governance.review.denied",
		ActorID: textValue(denial.Actor), Payload: payload, TraceID: denial.TraceID,
		CreatedAt: timestamp(denial.CreatedAt),
	}); err != nil {
		return reviewBatchRepositoryError("record review denial audit", err)
	}
	return nil
}

// ConfirmReviewBatch runs the whole §8.4 confirm in one transaction: the
// open-batch check, the locked per-member re-check through the pure Decide
// policy, the immutable review rows following the shared outcome rule, the
// rejection transitions, the split-out bookkeeping, the sample recompute over
// the confirmed membership and the batch decision itself.
func (store *Store) ConfirmReviewBatch(
	ctx context.Context, command governanceapp.ConfirmReviewBatchCommand,
) (governanceapp.ReviewBatchRows, error) {
	if command.Decision != governance.ReviewApproved && command.Decision != governance.ReviewRejected {
		return governanceapp.ReviewBatchRows{}, fmt.Errorf(
			"%w: review batch decision %q", governance.ErrInvalidArgument, command.Decision)
	}
	workspaceUUID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return governanceapp.ReviewBatchRows{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	batchUUID, err := uuidValue(command.BatchID)
	if err != nil {
		return governanceapp.ReviewBatchRows{}, fmt.Errorf("encode review batch ID: %w", err)
	}
	reviewerUUID, err := uuidValue(command.ReviewerPrincipalID)
	if err != nil {
		return governanceapp.ReviewBatchRows{}, fmt.Errorf("encode reviewer principal ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governanceapp.ReviewBatchRows{}, reviewBatchRepositoryError("begin review batch confirm", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	locked, err := queries.GetReviewBatchForUpdate(ctx, dbgen.GetReviewBatchForUpdateParams{
		WorkspaceID: workspaceUUID, ReviewBatchID: batchUUID,
	})
	if err != nil {
		return governanceapp.ReviewBatchRows{}, reviewBatchRepositoryError("lock review batch", err)
	}
	if locked.Status != string(governance.ReviewBatchOpen) {
		return governanceapp.ReviewBatchRows{}, fmt.Errorf(
			"%w: review batch is %s, confirm is no longer applicable",
			governance.ErrConflict, locked.Status)
	}
	memberRows, err := queries.LockReviewBatchMembersWithProposals(ctx, dbgen.LockReviewBatchMembersWithProposalsParams{
		WorkspaceID: workspaceUUID, ReviewBatchID: batchUUID,
	})
	if err != nil {
		return governanceapp.ReviewBatchRows{}, reviewBatchRepositoryError("lock review batch members", err)
	}
	if len(memberRows) == 0 {
		return governanceapp.ReviewBatchRows{}, fmt.Errorf(
			"%w: review batch without members cannot be confirmed", governance.ErrInvariant)
	}
	batchID, err := identity.ReviewBatchIDFromUUIDBytes(locked.ID.Bytes)
	if err != nil {
		return governanceapp.ReviewBatchRows{}, err
	}
	outcomes := make([]governanceapp.ConfirmMemberOutcome, 0, len(memberRows))
	applied := 0
	for _, memberRow := range memberRows {
		proposal, member, err := lockedMemberStateFromRow(batchID, memberRow)
		if err != nil {
			return governanceapp.ReviewBatchRows{}, err
		}
		latest, err := latestDecisionOrNil(ctx, queries, workspaceUUID, memberRow.ProposalID)
		if err != nil {
			return governanceapp.ReviewBatchRows{}, err
		}
		outcome, err := command.Decide(governanceapp.ConfirmMemberState{
			Member: member, Proposal: proposal, LatestDecision: latest,
		})
		if err != nil {
			return governanceapp.ReviewBatchRows{}, err
		}
		if outcome.Split && outcome.SplitReason == "" {
			return governanceapp.ReviewBatchRows{}, fmt.Errorf(
				"%w: member split without a split reason", governance.ErrInvalidArgument)
		}
		outcomes = append(outcomes, outcome)
		if !outcome.Split {
			applied++
		}
	}
	if applied == 0 {
		return governanceapp.ReviewBatchRows{}, fmt.Errorf(
			"%w: every member was split out, so no batch decision applies", governance.ErrInvariant)
	}
	sampleSequence := 0
	for index, memberRow := range memberRows {
		proposalID, err := identity.ProposalIDFromUUIDBytes(memberRow.ProposalID.Bytes)
		if err != nil {
			return governanceapp.ReviewBatchRows{}, err
		}
		outcome := outcomes[index]
		params := dbgen.UpdateReviewBatchMemberOutcomeParams{
			WorkspaceID: workspaceUUID, ReviewBatchID: batchUUID, ProposalID: memberRow.ProposalID,
			Sample: false, SplitOut: outcome.Split,
		}
		if outcome.Split {
			params.SplitReason = textValue(outcome.SplitReason)
		} else {
			params.Sample = sampleSequence < governance.SampleCount
			sampleSequence++
			params.Decision = textValue(string(command.Decision))
			reviewID, err := identity.NewReviewID()
			if err != nil {
				return governanceapp.ReviewBatchRows{}, fmt.Errorf("mint review ID: %w", err)
			}
			reviewUUID, err := uuidValue(reviewID)
			if err != nil {
				return governanceapp.ReviewBatchRows{}, fmt.Errorf("encode review ID: %w", err)
			}
			if _, err := queries.CreateReview(ctx, dbgen.CreateReviewParams{
				ID: reviewUUID, WorkspaceID: workspaceUUID, ProposalID: memberRow.ProposalID,
				ReviewerPrincipalID: reviewerUUID, Channel: string(governance.ReviewBatch),
				Decision: string(command.Decision), Note: command.Reason, CreatedAt: timestamp(command.DecidedAt),
			}); err != nil {
				return governanceapp.ReviewBatchRows{}, reviewBatchRepositoryError("create batch review", err)
			}
			if command.Decision == governance.ReviewRejected {
				transitioned, err := queries.TransitionProposal(ctx, dbgen.TransitionProposalParams{
					WorkspaceID: workspaceUUID, ProposalID: memberRow.ProposalID,
					State: string(governance.ProposalRejected), DecidedAt: timestamp(command.DecidedAt),
					ExpectedState: memberRow.ProposalState, UpdatedAt: timestamp(command.DecidedAt),
				})
				if err != nil {
					return governanceapp.ReviewBatchRows{}, reviewBatchRepositoryError("reject batch member", err)
				}
				if err := projectProposalAttention(ctx, queries, transitioned, command.TraceID, command.DecidedAt); err != nil {
					return governanceapp.ReviewBatchRows{}, reviewBatchRepositoryError("project batch rejection attention", err)
				}
				events := command.MemberEvents[proposalID]
				if err := createProposalMutationEvents(ctx, queries, proposalEvent{
					WorkspaceID: command.WorkspaceID, ProposalID: proposalID,
					AuditID: events.AuditEventID, OutboxID: events.OutboxEventID,
					Action: "rejected", FromState: memberRow.ProposalState,
					ToState: string(governance.ProposalRejected), Actor: command.Reviewer,
					TraceID: command.TraceID, CreatedAt: command.DecidedAt,
				}); err != nil {
					return governanceapp.ReviewBatchRows{}, reviewBatchRepositoryError("record batch rejection events", err)
				}
			}
		}
		if err := queries.UpdateReviewBatchMemberOutcome(ctx, params); err != nil {
			return governanceapp.ReviewBatchRows{}, reviewBatchRepositoryError("update review batch member outcome", err)
		}
	}
	batchStatus := string(governance.ReviewBatchConfirmed)
	if command.Decision == governance.ReviewRejected {
		batchStatus = string(governance.ReviewBatchRejected)
	}
	confirmedRow, err := queries.ConfirmReviewBatch(ctx, dbgen.ConfirmReviewBatchParams{
		WorkspaceID: workspaceUUID, ReviewBatchID: batchUUID, Status: batchStatus,
		DecidedBy: textValue(command.Reviewer), DecidedAt: timestamp(command.DecidedAt),
	})
	if err != nil {
		return governanceapp.ReviewBatchRows{}, reviewBatchRepositoryError("confirm review batch", err)
	}
	groupingRule, err := reviewGroupingRuleFromJSON(locked.GroupingRule)
	if err != nil {
		return governanceapp.ReviewBatchRows{}, err
	}
	if err := createReviewBatchAuditEvent(ctx, queries, reviewBatchEvent{
		WorkspaceID: command.WorkspaceID, BatchID: command.BatchID,
		AuditID: command.BatchAuditEventID, Action: batchStatus,
		GroupingRule: groupingRule, PolicyVersion: locked.PolicyVersion,
		MemberCount: len(memberRows), Actor: command.Reviewer,
		TraceID: command.TraceID, CreatedAt: command.DecidedAt,
	}); err != nil {
		return governanceapp.ReviewBatchRows{}, reviewBatchRepositoryError("record review batch confirm audit", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return governanceapp.ReviewBatchRows{}, reviewBatchRepositoryError("commit review batch confirm", err)
	}
	batch, err := reviewBatchFromRow(confirmedRow)
	if err != nil {
		return governanceapp.ReviewBatchRows{}, err
	}
	members, err := store.reviewBatchMembers(ctx, command.WorkspaceID, command.BatchID)
	if err != nil {
		return governanceapp.ReviewBatchRows{}, err
	}
	return governanceapp.ReviewBatchRows{Batch: batch, Members: members}, nil
}

func (store *Store) reviewBatchMembers(
	ctx context.Context, workspace identity.WorkspaceID, batch identity.ReviewBatchID,
) ([]governance.ReviewBatchMember, error) {
	workspaceUUID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	batchUUID, err := uuidValue(batch)
	if err != nil {
		return nil, fmt.Errorf("encode review batch ID: %w", err)
	}
	rows, err := store.queries.ListReviewBatchMembers(ctx, dbgen.ListReviewBatchMembersParams{
		WorkspaceID: workspaceUUID, ReviewBatchID: batchUUID,
	})
	if err != nil {
		return nil, reviewBatchRepositoryError("list review batch members", err)
	}
	members := make([]governance.ReviewBatchMember, 0, len(rows))
	for _, row := range rows {
		member, err := reviewBatchMemberFromRow(batch, row)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, nil
}

func txReviewBatchMembers(
	ctx context.Context, queries *dbgen.Queries, workspace identity.WorkspaceID, batch identity.ReviewBatchID,
) ([]governance.ReviewBatchMember, error) {
	workspaceUUID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	batchUUID, err := uuidValue(batch)
	if err != nil {
		return nil, fmt.Errorf("encode review batch ID: %w", err)
	}
	rows, err := queries.ListReviewBatchMembers(ctx, dbgen.ListReviewBatchMembersParams{
		WorkspaceID: workspaceUUID, ReviewBatchID: batchUUID,
	})
	if err != nil {
		return nil, reviewBatchRepositoryError("list review batch members", err)
	}
	members := make([]governance.ReviewBatchMember, 0, len(rows))
	for _, row := range rows {
		member, err := reviewBatchMemberFromRow(batch, row)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, nil
}

func latestDecisionOrNil(
	ctx context.Context, queries *dbgen.Queries, workspaceUUID, proposalUUID pgtype.UUID,
) (*governance.PolicyDecision, error) {
	row, err := queries.GetLatestProposalPolicyDecision(ctx, dbgen.GetLatestProposalPolicyDecisionParams{
		WorkspaceID: workspaceUUID, ProposalID: proposalUUID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, reviewBatchRepositoryError("read latest policy decision", err)
	}
	decision, err := policyDecisionFromRow(row)
	if err != nil {
		return nil, err
	}
	return &decision, nil
}

func lockedMemberStateFromRow(
	batch identity.ReviewBatchID, row dbgen.LockReviewBatchMembersWithProposalsRow,
) (governance.Proposal, governance.ReviewBatchMember, error) {
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.Proposal{}, governance.ReviewBatchMember{}, err
	}
	proposalID, err := identity.ProposalIDFromUUIDBytes(row.ProposalID.Bytes)
	if err != nil {
		return governance.Proposal{}, governance.ReviewBatchMember{}, err
	}
	proposal := governance.Proposal{
		ID: proposalID, WorkspaceID: workspaceID,
		TargetObjectType: governance.TargetObjectType(row.ProposalTargetType),
		TargetObjectID:   pgUUIDString(row.ProposalTargetObjectID),
		State:            governance.ProposalState(row.ProposalState),
		Title:            row.ProposalTitle, CreatedBy: row.ProposalCreatedBy,
		CreatedAt: row.ProposalCreatedAt.Time,
	}
	member := governance.ReviewBatchMember{
		BatchID: batch, WorkspaceID: workspaceID, ProposalID: proposalID,
		AddedReason: cloneJSON(row.AddedReason), Sample: row.Sample, SplitOut: row.SplitOut,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.Decision.Valid {
		decision := governance.ReviewDecision(row.Decision.String)
		member.Decision = &decision
	}
	if row.SplitReason.Valid {
		reason := row.SplitReason.String
		member.SplitReason = &reason
	}
	return proposal, member, nil
}

type reviewBatchEvent struct {
	WorkspaceID   identity.WorkspaceID
	BatchID       identity.ReviewBatchID
	AuditID       identity.EventID
	Action        string
	GroupingRule  governance.ReviewGroupingRule
	PolicyVersion string
	MemberCount   int
	Actor         string
	TraceID       string
	CreatedAt     time.Time
}

func createReviewBatchAuditEvent(ctx context.Context, queries *dbgen.Queries, event reviewBatchEvent) error {
	workspaceUUID, err := uuidValue(event.WorkspaceID)
	if err != nil {
		return err
	}
	auditID, err := uuidValue(event.AuditID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(buildMutationPayload(map[string]any{
		"reviewBatchId": event.BatchID.String(),
		"groupingRule":  event.GroupingRule,
		"policyVersion": event.PolicyVersion,
		"memberCount":   event.MemberCount,
	}, "semlia.review-batch/v1", event.Action))
	if err != nil {
		return err
	}
	return queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		ID: auditID, WorkspaceID: workspaceUUID, EventType: "governance.review_batch." + event.Action,
		ActorID: textValue(event.Actor), Payload: payload, TraceID: event.TraceID,
		CreatedAt: timestamp(event.CreatedAt.UTC()),
	})
}

func reviewBatchFromRow(row dbgen.ReviewBatch) (governance.ReviewBatchRecord, error) {
	batchID, err := identity.ReviewBatchIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.ReviewBatchRecord{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.ReviewBatchRecord{}, err
	}
	groupingRule, err := reviewGroupingRuleFromJSON(row.GroupingRule)
	if err != nil {
		return governance.ReviewBatchRecord{}, err
	}
	record := governance.ReviewBatchRecord{
		ID: batchID, WorkspaceID: workspaceID, GroupingRule: groupingRule,
		PolicyVersion: row.PolicyVersion, Status: governance.ReviewBatchStatus(row.Status),
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time,
	}
	if row.DecidedBy.Valid {
		decidedBy := row.DecidedBy.String
		record.DecidedBy = &decidedBy
	}
	if row.DecidedAt.Valid {
		decidedAt := row.DecidedAt.Time.UTC()
		record.DecidedAt = &decidedAt
	}
	return record, nil
}

func reviewBatchMemberFromRow(batch identity.ReviewBatchID, row dbgen.ReviewBatchMember) (governance.ReviewBatchMember, error) {
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.ReviewBatchMember{}, err
	}
	proposalID, err := identity.ProposalIDFromUUIDBytes(row.ProposalID.Bytes)
	if err != nil {
		return governance.ReviewBatchMember{}, err
	}
	member := governance.ReviewBatchMember{
		BatchID: batch, WorkspaceID: workspaceID, ProposalID: proposalID,
		AddedReason: cloneJSON(row.AddedReason), Sample: row.Sample, SplitOut: row.SplitOut,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.Decision.Valid {
		decision := governance.ReviewDecision(row.Decision.String)
		member.Decision = &decision
	}
	if row.SplitReason.Valid {
		reason := row.SplitReason.String
		member.SplitReason = &reason
	}
	return member, nil
}

func eligibleProposalFromRow(row dbgen.ListBatchEligibleProposalsRow) (governanceapp.BatchEligibleProposal, error) {
	id, err := identity.ProposalIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governanceapp.BatchEligibleProposal{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governanceapp.BatchEligibleProposal{}, err
	}
	proposal := governance.Proposal{
		ID: id, WorkspaceID: workspaceID, TargetObjectType: governance.TargetObjectType(row.TargetObjectType),
		TargetObjectID: pgUUIDString(row.TargetObjectID),
		State:          governance.ProposalState(row.State),
		Title:          row.Title, Summary: row.Summary, Reason: row.Reason,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.AssetID.Valid {
		assetID, err := identity.AssetIDFromUUIDBytes(row.AssetID.Bytes)
		if err != nil {
			return governanceapp.BatchEligibleProposal{}, err
		}
		proposal.AssetID = &assetID
	}
	if row.AgentRunID.Valid {
		agentRunID, err := identity.AgentRunIDFromUUIDBytes(row.AgentRunID.Bytes)
		if err != nil {
			return governanceapp.BatchEligibleProposal{}, err
		}
		proposal.AgentRunID = &agentRunID
	}
	if row.RiskLevel.Valid {
		proposal.RiskLevel = governance.RiskLevel(row.RiskLevel.String)
	}
	if row.SubmittedAt.Valid {
		submittedAt := row.SubmittedAt.Time.UTC()
		proposal.SubmittedAt = &submittedAt
	}
	decisionID, err := identity.PolicyDecisionIDFromUUIDBytes(row.PolicyDecisionID.Bytes)
	if err != nil {
		return governanceapp.BatchEligibleProposal{}, err
	}
	decision := governance.PolicyDecision{
		ID: decisionID, WorkspaceID: workspaceID, ProposalID: id, RuleVersion: row.DecisionRuleVersion,
		Inputs: cloneJSON(row.DecisionInputs), InputsDigest: row.DecisionInputsDigest,
		MatchedPolicy: row.DecisionMatchedPolicy, RiskLevel: governance.RiskLevel(row.DecisionRiskLevel),
		Routing: governance.RoutingChannel(row.DecisionRouting), ReasonCode: row.DecisionReasonCode,
	}
	return governanceapp.BatchEligibleProposal{Proposal: proposal, Decision: decision}, nil
}

func reviewGroupingRuleFromJSON(raw []byte) (governance.ReviewGroupingRule, error) {
	var rule governance.ReviewGroupingRule
	if err := json.Unmarshal(raw, &rule); err != nil {
		return governance.ReviewGroupingRule{}, fmt.Errorf("decode review batch grouping rule: %w", err)
	}
	if err := rule.Validate(); err != nil {
		return governance.ReviewGroupingRule{}, err
	}
	return rule, nil
}

func mustGroupingRuleJSON(rule governance.ReviewGroupingRule) []byte {
	if err := rule.Validate(); err != nil {
		panic(err)
	}
	encoded, err := json.Marshal(rule)
	if err != nil {
		panic(err)
	}
	return encoded
}

func pgUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return value.String()
}
