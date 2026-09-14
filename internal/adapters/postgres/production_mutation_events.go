package postgres

import (
	"context"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
)

func (s *Store) productionProposalReleasedTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, target domain.ProductionTarget, now time.Time) error {
	workspace, err := uuidValue(ver.WorkspaceID)
	if err != nil {
		return err
	}
	proposal, err := uuidValue(*target.ProposalID)
	if err != nil {
		return err
	}
	q := s.queries.WithTx(tx)
	row, err := q.TransitionProposal(ctx, dbgen.TransitionProposalParams{WorkspaceID: workspace, ProposalID: proposal, State: "released", ExpectedState: "in_review", DecidedAt: timestamp(now), UpdatedAt: timestamp(now)})
	if err != nil {
		return governanceRepositoryError("release production proposal", err)
	}
	audit, err := identity.NewEventID()
	if err != nil {
		return err
	}
	outbox, err := identity.NewEventID()
	if err != nil {
		return err
	}
	var asset *identity.AssetID
	if row.AssetID.Valid {
		id, err := identity.AssetIDFromUUIDBytes(row.AssetID.Bytes)
		if err != nil {
			return err
		}
		asset = &id
	}
	if err := createProposalMutationEvents(ctx, q, proposalEvent{WorkspaceID: ver.WorkspaceID, ProposalID: *target.ProposalID, AssetID: asset, AuditID: audit, OutboxID: outbox, Action: "released", FromState: "in_review", ToState: "released", Actor: ver.CreatedBy.String(), TraceID: ver.TraceID, CreatedAt: now}); err != nil {
		return err
	}
	return projectProposalAttention(ctx, q, row, ver.TraceID, now)
}

func (s *Store) productionObjectAuditTx(ctx context.Context, tx pgx.Tx, w identity.WorkspaceID, kind, id, action string, version int, actor, trace string, now time.Time) error {
	audit, err := identity.NewEventID()
	if err != nil {
		return err
	}
	return createGovernedObjectAuditEvent(ctx, s.queries.WithTx(tx), governedObjectEvent{WorkspaceID: w, ObjectType: kind, ObjectID: id, AuditID: audit, Action: action, Version: version, Summary: "production " + action, Actor: actor, TraceID: trace, CreatedAt: now})
}
