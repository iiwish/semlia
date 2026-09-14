package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	authapp "github.com/iiwish/semlia/internal/application/authorization"
	app "github.com/iiwish/semlia/internal/application/governance"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const businessRuleColumns = `sequence,workspace_id,operation_id,production_version,target_key,action,set_digest,content_digest,evidence_id,COALESCE(evidence_digest,''),evidence_origin,principal_id,authorization_version,created_at`

func scanBusinessRule(row pgx.Row) (domain.ProductionBusinessRuleEvent, error) {
	var e domain.ProductionBusinessRuleEvent
	var w, op, p, ev pgtype.UUID
	err := row.Scan(&e.Sequence, &w, &op, &e.ProductionVersion, &e.TargetKey, &e.Action, &e.SetDigest, &e.ContentDigest, &ev, &e.EvidenceDigest, &e.EvidenceOrigin, &p, &e.AuthorizationVersion, &e.CreatedAt)
	if err != nil {
		return e, err
	}
	e.WorkspaceID, err = identity.WorkspaceIDFromUUIDBytes(w.Bytes)
	if err != nil {
		return e, err
	}
	e.OperationID, err = identity.ProductionOperationIDFromUUIDBytes(op.Bytes)
	if err != nil {
		return e, err
	}
	e.PrincipalID, err = identity.PrincipalIDFromUUIDBytes(p.Bytes)
	if err != nil {
		return e, err
	}
	if ev.Valid {
		id, err := identity.EvidenceIDFromUUIDBytes(ev.Bytes)
		if err != nil {
			return e, err
		}
		e.EvidenceID = id.String()
	}
	return e, nil
}

func (s *Store) authorizeBusinessRuleTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, targets []domain.ProductionTarget, target domain.ProductionTarget) (int64, error) {
	if err := s.validateProductionInputTx(ctx, tx, ver, targets, false); err != nil {
		return 0, err
	}
	if !domain.RequiresProductionBusinessRule(target) {
		return 0, domain.ErrInvalidArgument
	}
	access, err := authapp.NewService(s, authapp.ClockFunc(time.Now)).Snapshot(ctx, ver.WorkspaceID, ver.CreatedBy)
	if err != nil {
		return 0, err
	}
	if access.Principal.Kind != authz.PrincipalHuman || access.Principal.Status != authz.PrincipalActive {
		return 0, &authz.DenialError{Decision: authz.Decision{ReasonCode: authz.ReasonNoMatchingGrant}}
	}
	if err := authorizeProductionTargets(ctx, tx, access, ver.WorkspaceID, []domain.ProductionTarget{target}, true, ver.BaselineJSON); err != nil {
		return 0, err
	}
	return access.AuthorizationVersion, nil
}

func businessRuleEvidenceTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, target domain.ProductionTarget, evidence string) (string, error) {
	if target.Declaration == nil {
		return "", domain.ErrEvidenceMissing
	}
	linked := false
	for _, id := range target.Declaration.EvidenceIDs {
		if id == evidence {
			linked = true
		}
	}
	if !linked {
		return "", domain.ErrEvidenceMissing
	}
	var wrapper struct {
		Scope json.RawMessage `json:"scope"`
	}
	if err := json.Unmarshal(ver.InputJSON, &wrapper); err != nil {
		return "", err
	}
	inputJSON := ver.InputJSON
	if len(wrapper.Scope) > 0 {
		inputJSON = wrapper.Scope
	}
	var input productionInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return "", err
	}
	id, err := identity.ParseEvidenceID(evidence)
	if err != nil {
		return "", domain.ErrEvidenceMissing
	}
	var kind, digest string
	if err := tx.QueryRow(ctx, `SELECT evidence_type,content_digest FROM evidence_artifacts WHERE workspace_id=$1 AND id=$2`, ver.WorkspaceID.UUID(), id.UUID()).Scan(&kind, &digest); err != nil {
		return "", governanceRepositoryError("business rule evidence", err)
	}
	if kind != "declared" {
		return "", domain.ErrEvidenceMissing
	}
	for _, pin := range input.Evidence {
		if pin.EvidenceID == evidence && pin.Digest == digest {
			return digest, nil
		}
	}
	return "", domain.ErrEvidenceMissing
}

func (s *Store) RecordProductionBusinessRule(ctx context.Context, cmd domain.ProductionBusinessRuleCommand) (domain.ProductionBusinessRuleEvent, error) {
	empty := domain.ProductionBusinessRuleEvent{}
	_, ver, targets, _, _, err := s.GetProductionOperationVersion(ctx, cmd.WorkspaceID, cmd.OperationID, cmd.ExpectedVersion)
	if err != nil {
		return empty, err
	}
	var target domain.ProductionTarget
	for _, t := range targets {
		if t.LocalKey == cmd.TargetKey {
			target = t
		}
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return empty, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ver.CreatedBy = cmd.PrincipalID
	authorizationVersion, err := s.authorizeBusinessRuleTx(ctx, tx, ver, targets, target)
	if err != nil {
		return empty, err
	}
	var current int
	if err := tx.QueryRow(ctx, `SELECT current_version FROM production_operations WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, cmd.WorkspaceID.UUID(), cmd.OperationID.UUID()).Scan(&current); err != nil {
		return empty, err
	}
	if current != cmd.ExpectedVersion || ver.SetDigest != cmd.SetDigest {
		return empty, domain.ErrVersionConflict
	}
	var published bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM release_proposals WHERE workspace_id=$1 AND operation_id=$2)`, cmd.WorkspaceID.UUID(), cmd.OperationID.UUID()).Scan(&published); err != nil {
		return empty, err
	}
	if published {
		return empty, domain.ErrVersionConflict
	}
	raw, err := json.Marshal(struct {
		Operation string                               `json:"operationId"`
		Command   domain.ProductionBusinessRuleCommand `json:"command"`
	}{cmd.OperationID.String(), cmd})
	if err != nil {
		return empty, err
	}
	requestDigest, err := domain.DigestJSON(raw)
	if err != nil {
		return empty, err
	}
	var previousDigest string
	err = tx.QueryRow(ctx, `SELECT request_digest FROM production_business_rule_events WHERE workspace_id=$1 AND principal_id=$2 AND idempotency_key=$3`, cmd.WorkspaceID.UUID(), cmd.PrincipalID.UUID(), cmd.IdempotencyKey).Scan(&previousDigest)
	if err == nil {
		if previousDigest != requestDigest {
			return empty, domain.ErrIdempotencyConflict
		}
		e, err := scanBusinessRule(tx.QueryRow(ctx, `SELECT `+businessRuleColumns+` FROM production_business_rule_events WHERE workspace_id=$1 AND principal_id=$2 AND idempotency_key=$3`, cmd.WorkspaceID.UUID(), cmd.PrincipalID.UUID(), cmd.IdempotencyKey))
		e.Replayed = true
		return e, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return empty, err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM production_business_rule_events WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3`, cmd.WorkspaceID.UUID(), cmd.OperationID.UUID(), cmd.ExpectedVersion).Scan(&count); err != nil {
		return empty, err
	}
	if count >= 256 {
		return empty, domain.ErrLimitExceeded
	}
	var evidenceID, evidenceDigest any
	evidenceOrigin := "none"
	if cmd.Action == "confirm" {
		var content struct {
			Definition *string `json:"definition"`
			Scope      *string `json:"scope"`
		}
		if json.Unmarshal(target.ContentJSON, &content) != nil || content.Definition == nil || content.Scope == nil {
			return empty, domain.ErrInvalidArgument
		}
		if cmd.Declaration != "" {
			id, digest, err := createBusinessRuleDeclarationTx(ctx, tx, ver, target, cmd.Declaration)
			if err != nil {
				return empty, err
			}
			evidenceID, evidenceDigest, evidenceOrigin = id.UUID(), digest, "human_declaration"
		} else {
			digest, err := businessRuleEvidenceTx(ctx, tx, ver, target, cmd.EvidenceID)
			if err != nil {
				return empty, err
			}
			id, _ := identity.ParseEvidenceID(cmd.EvidenceID)
			evidenceID, evidenceDigest, evidenceOrigin = id.UUID(), digest, "selected_evidence"
		}
	} else if cmd.Action != "revoke" {
		return empty, domain.ErrInvalidArgument
	}
	if _, err := tx.Exec(ctx, `INSERT INTO production_contributors(workspace_id,operation_id,principal_id,role) VALUES($1,$2,$3,'editor') ON CONFLICT DO NOTHING`, cmd.WorkspaceID.UUID(), cmd.OperationID.UUID(), cmd.PrincipalID.UUID()); err != nil {
		return empty, err
	}
	event, err := scanBusinessRule(tx.QueryRow(ctx, `INSERT INTO production_business_rule_events(workspace_id,operation_id,production_version,target_key,action,set_digest,content_digest,evidence_id,evidence_digest,evidence_origin,principal_id,authorization_version,idempotency_key,request_digest)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING `+businessRuleColumns, cmd.WorkspaceID.UUID(), cmd.OperationID.UUID(), cmd.ExpectedVersion, cmd.TargetKey, cmd.Action, ver.SetDigest, target.ContentDigest, evidenceID, evidenceDigest, evidenceOrigin, cmd.PrincipalID.UUID(), authorizationVersion, cmd.IdempotencyKey, requestDigest))
	if err != nil {
		return empty, err
	}
	ver.CreatedAt = event.CreatedAt
	if err := insertProductionMutationEvents(ctx, s.queries.WithTx(tx), ver, "business_rule_"+cmd.Action); err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return event, nil
}

func (s *Store) productionBusinessRulesTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, targets []domain.ProductionTarget) ([]domain.ProductionBusinessRuleWitness, error) {
	rows, err := tx.Query(ctx, `SELECT `+businessRuleColumns+` FROM (SELECT DISTINCT ON(target_key) * FROM production_business_rule_events WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3 ORDER BY target_key,sequence DESC) latest ORDER BY target_key`, ver.WorkspaceID.UUID(), ver.OperationID.UUID(), ver.Version)
	if err != nil {
		return nil, err
	}
	events := []domain.ProductionBusinessRuleEvent{}
	for rows.Next() {
		e, err := scanBusinessRule(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		events = append(events, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := []domain.ProductionBusinessRuleWitness{}
	for _, event := range events {
		valid := false
		for _, target := range targets {
			if target.LocalKey != event.TargetKey || event.Action != "confirm" || event.SetDigest != ver.SetDigest || event.ContentDigest != target.ContentDigest {
				continue
			}
			actorVersion := ver
			actorVersion.CreatedBy = event.PrincipalID
			_, err := s.authorizeBusinessRuleTx(ctx, tx, actorVersion, targets, target)
			if err != nil {
				if productionValidationFailure(err) == "" {
					return nil, err
				}
				continue
			}
			var digest string
			if event.EvidenceOrigin == "human_declaration" {
				digest, err = businessRuleDeclarationDigestTx(ctx, tx, actorVersion, target, event.EvidenceID)
			} else if event.EvidenceOrigin == "selected_evidence" {
				digest, err = businessRuleEvidenceTx(ctx, tx, ver, target, event.EvidenceID)
			} else {
				continue
			}
			if err != nil {
				if productionValidationFailure(err) == "" {
					return nil, err
				}
				continue
			}
			valid = digest == event.EvidenceDigest
		}
		result = append(result, domain.ProductionBusinessRuleWitness{Event: event, Valid: valid})
	}
	return result, nil
}

func (s *Store) productionFreshnessTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, targets []domain.ProductionTarget, rules []domain.PolicyRule) (json.RawMessage, map[string]bool, error) {
	witnesses, err := s.productionBusinessRulesTx(ctx, tx, ver, targets)
	if err != nil {
		return nil, nil, err
	}
	confirmed := map[string]bool{}
	for _, w := range witnesses {
		confirmed[w.Event.TargetKey] = w.Valid
	}
	raw, err := app.ProductionFreshnessWitness(ver, rules, witnesses...)
	return raw, confirmed, err
}

func (s *Store) ReadProductionBusinessRules(ctx context.Context, w identity.WorkspaceID, p identity.PrincipalID, op identity.ProductionOperationID, version int) ([]domain.ProductionBusinessRuleWitness, error) {
	_, ver, targets, _, _, err := s.GetProductionOperationVersion(ctx, w, op, version)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ver.CreatedBy = p
	if err := s.validateProductionInputTx(ctx, tx, ver, targets, false); err != nil {
		return nil, err
	}
	witnesses, err := s.productionBusinessRulesTx(ctx, tx, ver, targets)
	if err != nil {
		return nil, err
	}
	// The read response exposes the original declaration for independent review;
	// validation witnesses pin its digest without duplicating the statement.
	for i := range witnesses {
		if witnesses[i].Event.EvidenceOrigin != "human_declaration" {
			continue
		}
		id, err := identity.ParseEvidenceID(witnesses[i].Event.EvidenceID)
		if err != nil {
			return nil, domain.ErrEvidenceMissing
		}
		if err := tx.QueryRow(ctx, `SELECT metadata->>'statement' FROM evidence_artifacts WHERE workspace_id=$1 AND id=$2`, w.UUID(), id.UUID()).Scan(&witnesses[i].Declaration); err != nil {
			return nil, err
		}
	}
	return witnesses, nil
}
