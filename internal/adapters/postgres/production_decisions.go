package postgres

import (
	"context"
	"fmt"
	"strings"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func productionDecisionPredecessor(version domain.ProductionVersion) (identity.ProductionOperationID, int) {
	if version.Version > 1 {
		return version.OperationID, version.Version - 1
	}
	if version.SupersedesOperationID != nil {
		return *version.SupersedesOperationID, version.SupersedesVersion
	}
	return identity.ProductionOperationID{}, 0
}

func (s *Store) productionRulesTx(ctx context.Context, tx pgx.Tx) ([]domain.PolicyRule, error) {
	if _, err := tx.Exec(ctx, `LOCK TABLE policy_rules IN SHARE MODE`); err != nil {
		return nil, err
	}
	rows, err := s.queries.WithTx(tx).ListPolicyRules(ctx, domain.RiskRuleVersion)
	if err != nil {
		return nil, err
	}
	rules := make([]domain.PolicyRule, 0, len(rows))
	for _, row := range rows {
		rule, err := policyRuleFromRow(row)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// A replacement appends a decision; immutable links retain the earlier decision.
func insertProductionDecisions(ctx context.Context, tx pgx.Tx, version domain.ProductionVersion, targets []domain.ProductionTarget, links []domain.ProductionCandidateLink) (map[string]pgtype.UUID, error) {
	decisions := make(map[string]pgtype.UUID)
	byKey := make(map[string]domain.ProductionTarget)
	for _, target := range targets {
		byKey[target.LocalKey] = target
	}
	for _, link := range links {
		if !link.IsPrimary {
			continue
		}
		target, ok := byKey[link.LocalKey]
		if !ok {
			return nil, domain.ErrInvalidArgument
		}
		if target.Outcome == domain.ProductionOutcomeNoChange {
			continue
		}
		if target.ProposalID == nil {
			return nil, domain.ErrInvalidArgument
		}
		candidate, err := parseUUIDOrTypeID(link.CandidateID)
		if err != nil {
			return nil, err
		}
		var status string
		var projection, previous pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT status,proposal_id FROM semantic_candidates WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, version.WorkspaceID.UUID(), candidate).Scan(&status, &projection); err != nil {
			return nil, err
		}
		if status != "pending" {
			previousOperation, previousVersion := productionDecisionPredecessor(version)
			if status != "converted" || previousVersion < 1 {
				return nil, domain.ErrAlreadyProduced
			}
			if err := tx.QueryRow(ctx, `SELECT l.decision_id FROM production_candidate_links l
			JOIN semantic_candidate_decisions d ON d.workspace_id=l.workspace_id AND d.id=l.decision_id
			WHERE l.workspace_id=$1 AND l.operation_id=$2 AND l.version=$3 AND l.candidate_id=$4 AND l.is_primary AND d.proposal_id=$5`, version.WorkspaceID.UUID(), previousOperation.UUID(), previousVersion, candidate, projection).Scan(&previous); err != nil || !previous.Valid {
				return nil, domain.ErrAlreadyProduced
			}
		}
		event, err := identity.NewEventID()
		if err != nil {
			return nil, err
		}
		decision, _ := uuidFromString(event.UUID())
		trace := version.TraceID
		if trace == "" {
			trace = strings.ReplaceAll(event.UUID(), "-", "")
		}
		key := fmt.Sprintf("production:%s:%d:%s", version.OperationID.String(), version.Version, link.CandidateID)
		if _, err := tx.Exec(ctx, `INSERT INTO semantic_candidate_decisions
		(id,workspace_id,candidate_id,action,proposal_id,actor,idempotency_key,trace_id,request_fingerprint,previous_decision_id,created_at)
		VALUES($1,$2,$3,'convert',$4,$5,$6,$7,$8,$9,$10)`, decision, version.WorkspaceID.UUID(), candidate, target.ProposalID.UUID(), version.CreatedBy.String(), key, trace, version.RequestDigest, previous, version.CreatedAt); err != nil {
			return nil, governanceRepositoryError("insert production candidate decision", err)
		}
		result, err := tx.Exec(ctx, `UPDATE semantic_candidates SET status='converted',proposal_id=$3,updated_at=$4 WHERE workspace_id=$1 AND id=$2 AND status=$5 AND proposal_id IS NOT DISTINCT FROM $6`, version.WorkspaceID.UUID(), candidate, target.ProposalID.UUID(), version.CreatedAt, status, projection)
		if err != nil {
			return nil, err
		}
		if result.RowsAffected() != 1 {
			return nil, domain.ErrConflict
		}
		decisions[link.CandidateID] = decision
	}
	return decisions, nil
}

func insertProductionMutationEvents(ctx context.Context, q *dbgen.Queries, version domain.ProductionVersion, action string) error {
	audit, err := identity.NewEventID()
	if err != nil {
		return err
	}
	outbox, err := identity.NewEventID()
	if err != nil {
		return err
	}
	trace := version.TraceID
	if trace == "" {
		trace = strings.ReplaceAll(audit.UUID(), "-", "")
	}
	data := map[string]any{"operationId": version.OperationID.String(), "version": version.Version, "setDigest": version.SetDigest}
	if version.SupersedesOperationID != nil {
		data["supersedesOperationId"] = version.SupersedesOperationID.String()
		data["supersedesVersion"] = version.SupersedesVersion
	}
	return createMutationEvents(ctx, q, mutationEvent{
		WorkspaceID: version.WorkspaceID, AuditID: audit, OutboxID: outbox,
		AuditType: "production.operation." + action, OutboxType: "production.operation.changed",
		SpecVersion: "semlia.production/v1", Action: action, Actor: version.CreatedBy.String(),
		TraceID: trace, CreatedAt: version.CreatedAt,
		Data: data,
	})
}
