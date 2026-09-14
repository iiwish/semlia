package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	app "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
)

func (s *Store) RollbackProductionCommand(ctx context.Context, cmd app.RollbackOperationCommand, requestDigest string) (*app.ReleaseCommandResult, error) {
	target, manifest, pins, attributions, err := s.GetProductionRelease(ctx, cmd.WorkspaceID, cmd.ReleaseID)
	if err != nil {
		return nil, err
	}
	if !target.IsProductionProtected() {
		return nil, domain.ErrProductionSetRequired
	}
	if manifest == nil || len(attributions) == 0 {
		return nil, domain.ErrPriorStateUnknown
	}
	first := attributions[0]
	if first.ProductionVersion != cmd.ExpectedVersion || first.SetDigest != cmd.SetDigest {
		return nil, domain.ErrVersionConflict
	}
	_, ver, targets, _, _, err := s.GetProductionOperationVersion(ctx, cmd.WorkspaceID, first.OperationID, first.ProductionVersion)
	if err != nil {
		return nil, err
	}
	ver.CreatedBy, ver.TraceID = cmd.PrincipalID, cmd.TraceID
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.productionActionTx(ctx, tx, ver, domain.CommandRollback); err != nil {
		return nil, err
	}
	// Historical reads reauthorize the complete original inputs without asking
	// old source snapshots to be current ingestion heads again.
	if err := s.validateProductionInputTx(ctx, tx, ver, targets, false); err != nil {
		return nil, err
	}
	var contributor bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_contributors WHERE workspace_id=$1 AND operation_id=$2 AND principal_id=$3)`, cmd.WorkspaceID.UUID(), first.OperationID.UUID(), cmd.PrincipalID.UUID()).Scan(&contributor); err != nil {
		return nil, err
	}
	if contributor {
		return nil, domain.ErrSodConflict
	}
	for _, attr := range attributions {
		if attr.OperationID != first.OperationID || attr.ProductionVersion != first.ProductionVersion || attr.SetDigest != first.SetDigest || attr.ValidationDigest != first.ValidationDigest || attr.AttemptNo != first.AttemptNo {
			return nil, domain.ErrPriorStateUnknown
		}
		if attr.AuthorPrincipalID == cmd.PrincipalID {
			return nil, domain.ErrSodConflict
		}
		var reviews []string
		if json.Unmarshal(attr.ReviewIDs, &reviews) != nil || len(reviews) == 0 {
			return nil, domain.ErrPriorStateUnknown
		}
		for _, raw := range reviews {
			review, err := identity.ParseReviewID(raw)
			if err != nil {
				return nil, domain.ErrPriorStateUnknown
			}
			var actor string
			if err := tx.QueryRow(ctx, `SELECT r.reviewer_principal_id::text FROM reviews r JOIN production_review_bindings b ON b.workspace_id=r.workspace_id AND b.review_id=r.id WHERE r.workspace_id=$1 AND r.id=$2 AND r.proposal_id=$3 AND r.decision='approved' AND r.channel='expert' AND b.operation_id=$4 AND b.production_version=$5 AND b.attempt_no=$6 AND b.set_digest=$7 AND b.validation_digest=$8`, cmd.WorkspaceID.UUID(), review.UUID(), attr.ProposalID.UUID(), attr.OperationID.UUID(), attr.ProductionVersion, attr.AttemptNo, attr.SetDigest, attr.ValidationDigest).Scan(&actor); err != nil {
				return nil, domain.ErrPriorStateUnknown
			}
			if actor == cmd.PrincipalID.UUID() {
				return nil, domain.ErrSodConflict
			}
		}
	}
	var sealed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_validation_seals WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3 AND attempt_no=$4 AND validation_digest=$5)`, cmd.WorkspaceID.UUID(), first.OperationID.UUID(), first.ProductionVersion, first.AttemptNo, first.ValidationDigest).Scan(&sealed); err != nil {
		return nil, err
	}
	if !sealed {
		return nil, domain.ErrPriorStateUnknown
	}
	existing, err := s.GetProductionCommand(ctx, cmd.WorkspaceID, cmd.PrincipalID, domain.CommandRollback, cmd.IdempotencyKey)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		if existing.RequestDigest != requestDigest {
			return nil, domain.ErrIdempotencyConflict
		}
		var result app.ReleaseCommandResult
		if len(existing.ResponseJSON) == 0 || json.Unmarshal(existing.ResponseJSON, &result) != nil {
			return nil, domain.ErrPriorStateUnknown
		}
		result.Replayed = true
		return &result, nil
	}
	before, err := s.productionHeadTx(ctx, tx, cmd.WorkspaceID, cmd.ExpectedHead)
	if err != nil {
		return nil, err
	}
	if before.ID != target.ID {
		return nil, domain.ErrHeadConflict
	}
	var trusted bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_release_integrity WHERE workspace_id=$1 AND release_id=$2)`, cmd.WorkspaceID.UUID(), target.ID.UUID()).Scan(&trusted); err != nil {
		return nil, err
	}
	if !trusted {
		return nil, domain.ErrPriorStateUnknown
	}
	assets, objects, err := domain.ParseProductionManifestJSON(manifest.BeforeManifestJSON)
	if err != nil {
		return nil, domain.ErrPriorStateUnknown
	}
	restoredPayload, err := domain.ManifestDigestPayloadWithObjects(assets, objects)
	if err != nil {
		return nil, err
	}
	restoredDigest, err := domain.DigestJSON(restoredPayload)
	if err != nil {
		return nil, err
	}
	if restoredDigest != manifest.BeforeManifestDigest || manifest.AfterManifestDigest != before.ManifestDigest {
		return nil, domain.ErrPriorStateUnknown
	}
	if target.ProductionRollbackDepth == nil || *target.ProductionRollbackDepth >= 2147483647 || target.ProductionRootReleaseID == nil {
		return nil, domain.ErrLimitExceeded
	}
	beforeJSON, err := domain.ProductionManifestJSON(before.Entries, before.Objects)
	if err != nil {
		return nil, err
	}
	beforePayload, err := domain.ManifestDigestPayloadWithObjects(before.Entries, before.Objects)
	if err != nil {
		return nil, err
	}
	beforeDigest, err := domain.DigestJSON(beforePayload)
	if err != nil {
		return nil, err
	}
	if beforeDigest != before.ManifestDigest {
		return nil, domain.ErrPriorStateUnknown
	}
	id, err := identity.NewReleaseID()
	if err != nil {
		return nil, err
	}
	now, depth := time.Now().UTC(), *target.ProductionRollbackDepth+1
	release := domain.Release{ID: id, WorkspaceID: cmd.WorkspaceID, State: domain.ReleasePublished, ManifestDigest: restoredDigest, Entries: assets, Objects: objects, PublishedBy: cmd.PrincipalID.String(), PublishedAt: now, CreatedAt: now, RolledBackToReleaseID: &target.ID, ProductionRootReleaseID: target.ProductionRootReleaseID, ProductionRollbackParentID: &target.ID, ProductionRollbackDepth: &depth}
	newPins, err := s.productionRollbackPinsTx(ctx, tx, before, release, targets, pins)
	if err != nil {
		return nil, err
	}
	if err := s.productionRestoreManifestTx(ctx, tx, before, release, manifest.BeforeReleaseID, targets, cmd.TraceID); err != nil {
		return nil, err
	}
	for i := range attributions {
		attributions[i].ReleaseID = id
		attributions[i].Role = domain.RoleReverted
		attributions[i].CreatedAt = now
	}
	newManifest := domain.ProductionReleaseManifest{WorkspaceID: cmd.WorkspaceID, ReleaseID: id, BeforeReleaseID: &target.ID, BeforeManifestJSON: beforeJSON, BeforeManifestDigest: before.ManifestDigest, AfterManifestDigest: restoredDigest, AttributionDigest: manifest.AttributionDigest, CreatedAt: now}
	result := &app.ReleaseCommandResult{ReleaseID: id, OperationID: first.OperationID, ManifestDigest: restoredDigest}
	response, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	command := domain.ProductionCommand{WorkspaceID: cmd.WorkspaceID, PrincipalID: cmd.PrincipalID, CommandKind: domain.CommandRollback, IdempotencyKey: cmd.IdempotencyKey, RequestDigest: requestDigest, OperationID: first.OperationID, OperationVersion: first.ProductionVersion, ResultKind: "release", ResultID: id.String(), ResponseJSON: response, CommittedAt: now}
	if err := s.persistProductionReleaseTx(ctx, tx, release, newManifest, newPins, attributions, command, cmd.TraceID); err != nil {
		return nil, err
	}
	if manifest.BeforeReleaseID != nil {
		if err := s.productionBindingInputsTx(ctx, tx, release, domain.Release{ID: *manifest.BeforeReleaseID}, nil); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, governanceRepositoryError("commit production rollback", err)
	}
	return result, nil
}
