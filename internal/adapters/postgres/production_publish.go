package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	app "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) PublishProductionCommand(ctx context.Context, cmd app.PublishOperationCommand, requestDigest string) (*app.ReleaseCommandResult, error) {
	_, ver, targets, _, _, err := s.GetProductionOperationVersion(ctx, cmd.WorkspaceID, cmd.OperationID, cmd.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	if ver.FrozenAt == nil || cmd.SetDigest != ver.SetDigest {
		return nil, domain.ErrVersionConflict
	}
	ver.CreatedBy = cmd.PrincipalID
	ver.TraceID = cmd.TraceID
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.productionActionTx(ctx, tx, ver, domain.CommandPublish); err != nil {
		return nil, err
	}
	var contributor bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_contributors WHERE workspace_id=$1 AND operation_id=$2 AND principal_id=$3)`, cmd.WorkspaceID.UUID(), cmd.OperationID.UUID(), cmd.PrincipalID.UUID()).Scan(&contributor); err != nil {
		return nil, err
	}
	if contributor {
		return nil, domain.ErrSodConflict
	}
	if err := s.productionAttemptGate(ctx, tx, ver, targets, cmd.ValidationAttemptNo, cmd.ValidationDigest, false); err != nil {
		return nil, err
	}
	before, err := s.productionHeadTx(ctx, tx, cmd.WorkspaceID, cmd.ExpectedHead)
	if err != nil {
		return nil, err
	}
	reviews, err := s.productionApprovalsTx(ctx, tx, ver, targets, cmd.ValidationAttemptNo, cmd.ValidationDigest)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	id, err := identity.NewReleaseID()
	if err != nil {
		return nil, err
	}
	depth := 0
	release := domain.Release{ID: id, WorkspaceID: cmd.WorkspaceID, State: domain.ReleasePublished, PublishedBy: cmd.PrincipalID.String(), PublishedAt: now, CreatedAt: now, ProductionRootReleaseID: &id, ProductionRollbackDepth: &depth, Entries: append([]domain.ManifestEntry{}, before.Entries...), Objects: append([]domain.ObjectManifestEntry{}, before.Objects...)}
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
	var beforeID *identity.ReleaseID
	if !before.ID.IsZero() {
		beforeID = &before.ID
		if beforeDigest != before.ManifestDigest {
			return nil, domain.ErrPriorStateUnknown
		}
	}
	pins := []domain.ProductionReleaseBeforePin{}
	attributions := []domain.ReleaseProposal{}
	allReviews := []string{}
	for _, target := range targets {
		pin := domain.ProductionReleaseBeforePin{WorkspaceID: cmd.WorkspaceID, ReleaseID: id, TargetKind: target.Kind, TargetID: target.TargetID, Presence: domain.PresenceAbsent, CreatedAt: now}
		for _, asset := range before.Entries {
			if asset.AssetID.String() == target.TargetID {
				pin.Presence = domain.PresencePresent
				revision := asset.RevisionID
				pin.RevisionID = &revision
				var digest string
				if err := tx.QueryRow(ctx, `SELECT content_digest FROM asset_revisions WHERE workspace_id=$1 AND id=$2 AND asset_id=$3`, cmd.WorkspaceID.UUID(), revision.UUID(), asset.AssetID.UUID()).Scan(&digest); err != nil {
					return nil, err
				}
				pin.ContentDigest = &digest
			}
		}
		for _, object := range before.Objects {
			typed, err := object.TypedID()
			if err != nil {
				return nil, err
			}
			if typed == target.TargetID && string(object.ObjectType) == target.Kind {
				pin.Presence = domain.PresencePresent
				version := object.Version
				pin.ObjectVersion = &version
				var content []byte
				if err := tx.QueryRow(ctx, `SELECT payload FROM release_object_snapshots WHERE workspace_id=$1 AND release_id=$2 AND object_type=$3 AND object_id=$4 AND version=$5`, cmd.WorkspaceID.UUID(), before.ID.UUID(), target.Kind, object.ObjectID, object.Version).Scan(&content); err != nil {
					return nil, err
				}
				digest, err := domain.DigestJSON(content)
				if err != nil {
					return nil, err
				}
				pin.ContentDigest = &digest
			}
		}
		if (target.Intent == domain.ProductionIntentCreate && pin.Presence != domain.PresenceAbsent) || (target.Intent == domain.ProductionIntentUpdate && pin.Presence != domain.PresencePresent) {
			return nil, domain.ErrBaselineConflict
		}
		pins = append(pins, pin)
		if target.ProposalID == nil {
			continue
		}
		var author string
		var state string
		if err := tx.QueryRow(ctx, `SELECT created_by,state FROM proposals WHERE workspace_id=$1 AND id=$2 AND production_operation_id=$3 AND production_version=$4 FOR UPDATE`, cmd.WorkspaceID.UUID(), target.ProposalID.UUID(), cmd.OperationID.UUID(), ver.Version).Scan(&author, &state); err != nil {
			return nil, err
		}
		if state != "in_review" {
			return nil, domain.ErrVersionConflict
		}
		authorID, err := identity.ParsePrincipalID(author)
		if err != nil {
			return nil, err
		}
		adopted := reviews[target.ProposalID.String()]
		reviewJSON, err := json.Marshal(adopted)
		if err != nil {
			return nil, err
		}
		allReviews = append(allReviews, adopted...)
		attributions = append(attributions, domain.ReleaseProposal{WorkspaceID: cmd.WorkspaceID, ReleaseID: id, ProposalID: *target.ProposalID, OperationID: cmd.OperationID, ProductionVersion: ver.Version, SetDigest: ver.SetDigest, ProposalContentDigest: target.ContentDigest, AuthorPrincipalID: authorID, AttemptNo: cmd.ValidationAttemptNo, ValidationDigest: cmd.ValidationDigest, ReviewIDs: reviewJSON, Role: domain.RoleApplied, CreatedAt: now})
	}
	// Asset identities are reserved during authoring. Revisions precede objects
	// so local object references see the same transaction's published assets.
	for _, target := range targets {
		if target.ProposalID == nil || target.Kind != domain.TargetKindSemanticAsset {
			continue
		}
		entry, err := s.productionAssetRevisionTx(ctx, tx, ver, target, now)
		if err != nil {
			return nil, err
		}
		found := false
		max := 0
		for i, old := range release.Entries {
			if old.Position > max {
				max = old.Position
			}
			if old.AssetID == entry.AssetID {
				entry.Position = old.Position
				entry.Compatibility = old.Compatibility
				release.Entries[i] = entry
				found = true
			}
		}
		if !found {
			entry.Position = max + 1
			release.Entries = append(release.Entries, entry)
		}
	}
	for _, target := range targets {
		if target.ProposalID == nil || target.Kind == domain.TargetKindSemanticAsset {
			continue
		}
		entry, err := s.productionObjectWriteTx(ctx, tx, ver, target, now)
		if err != nil {
			return nil, err
		}
		found := false
		max := 0
		for i, old := range release.Objects {
			if old.Position > max {
				max = old.Position
			}
			if old.ObjectType == entry.ObjectType && old.ObjectID == entry.ObjectID {
				entry.Position = old.Position
				release.Objects[i] = entry
				found = true
			}
		}
		if !found {
			entry.Position = max + 1
			release.Objects = append(release.Objects, entry)
		}
	}
	if _, err := domain.ProductionManifestJSON(release.Entries, release.Objects); err != nil {
		return nil, err
	}
	payload, err := domain.ManifestDigestPayloadWithObjects(release.Entries, release.Objects)
	if err != nil {
		return nil, err
	}
	release.ManifestDigest, err = domain.DigestJSON(payload)
	if err != nil {
		return nil, err
	}
	manifest := domain.ProductionReleaseManifest{WorkspaceID: cmd.WorkspaceID, ReleaseID: id, BeforeReleaseID: beforeID, BeforeManifestJSON: beforeJSON, BeforeManifestDigest: beforeDigest, AfterManifestDigest: release.ManifestDigest, AttributionDigest: domain.ComputeAttributionDigest(cmd.OperationID.String(), ver.Version, ver.SetDigest, cmd.ValidationDigest, allReviews), CreatedAt: now}
	result := &app.ReleaseCommandResult{ReleaseID: id, OperationID: cmd.OperationID, ManifestDigest: release.ManifestDigest}
	response, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	command := domain.ProductionCommand{WorkspaceID: cmd.WorkspaceID, PrincipalID: cmd.PrincipalID, CommandKind: domain.CommandPublish, IdempotencyKey: cmd.IdempotencyKey, RequestDigest: requestDigest, OperationID: cmd.OperationID, OperationVersion: ver.Version, ResultKind: "release", ResultID: id.String(), CommittedAt: now, ResponseJSON: response}
	if err := s.persistProductionReleaseTx(ctx, tx, release, manifest, pins, attributions, command, cmd.TraceID); err != nil {
		return nil, err
	}
	if err := s.productionBindingInputsTx(ctx, tx, release, before, targets); err != nil {
		return nil, err
	}
	for _, target := range targets {
		if target.ProposalID != nil {
			if err := s.productionProposalReleasedTx(ctx, tx, ver, target, now); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, governanceRepositoryError("commit production release", err)
	}
	return result, nil
}

func (s *Store) productionHeadTx(ctx context.Context, tx pgx.Tx, w identity.WorkspaceID, expected domain.HeadReference) (domain.Release, error) {
	if err := expected.Validate(); err != nil {
		return domain.Release{}, err
	}
	q := s.queries.WithTx(tx)
	workspace, _ := uuidFromString(w.UUID())
	row, err := q.GetLatestRelease(ctx, workspace)
	if errors.Is(err, pgx.ErrNoRows) {
		if expected.Presence != domain.PresenceAbsent {
			return domain.Release{}, domain.ErrHeadConflict
		}
		return domain.Release{WorkspaceID: w, Entries: []domain.ManifestEntry{}, Objects: []domain.ObjectManifestEntry{}}, nil
	}
	if err != nil {
		return domain.Release{}, err
	}
	release, err := releaseFromRow(row)
	if err != nil {
		return release, err
	}
	if expected.Presence != domain.PresencePresent || *expected.ReleaseID != release.ID || *expected.ManifestDigest != release.ManifestDigest {
		return release, domain.ErrHeadConflict
	}
	assets, err := q.ListReleaseAssets(ctx, dbgen.ListReleaseAssetsParams{WorkspaceID: workspace, ReleaseID: row.ID})
	if err != nil {
		return release, err
	}
	for _, row := range assets {
		entry, err := manifestEntryFromRow(row)
		if err != nil {
			return release, err
		}
		release.Entries = append(release.Entries, entry)
	}
	objects, err := q.ListReleaseObjects(ctx, dbgen.ListReleaseObjectsParams{WorkspaceID: workspace, ReleaseID: row.ID})
	if err != nil {
		return release, err
	}
	for _, row := range objects {
		release.Objects = append(release.Objects, domain.ObjectManifestEntry{ObjectType: domain.TargetObjectType(row.ObjectType), ObjectID: formatUUID(row.ObjectID), Version: int(row.Version), Position: int(row.Position)})
	}
	if _, err := domain.ProductionManifestJSON(release.Entries, release.Objects); err != nil {
		return release, err
	}
	return release, nil
}

func (s *Store) productionApprovalsTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, targets []domain.ProductionTarget, attempt int, digest string) (map[string][]string, error) {
	rows, err := tx.Query(ctx, `SELECT r.id,r.proposal_id,r.reviewer_principal_id FROM production_review_bindings b JOIN reviews r ON r.workspace_id=b.workspace_id AND r.id=b.review_id WHERE b.workspace_id=$1 AND b.operation_id=$2 AND b.production_version=$3 AND b.attempt_no=$4 AND b.set_digest=$5 AND b.validation_digest=$6 AND r.decision='approved' AND r.channel='expert' ORDER BY r.proposal_id,r.id LIMIT 257`, ver.WorkspaceID.UUID(), ver.OperationID.UUID(), ver.Version, attempt, ver.SetDigest, digest)
	if err != nil {
		return nil, err
	}
	type approval struct{ review, proposal, principal pgtype.UUID }
	all := []approval{}
	for rows.Next() {
		var a approval
		if err := rows.Scan(&a.review, &a.proposal, &a.principal); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(all) > 256 {
		return nil, domain.ErrLimitExceeded
	}
	result := map[string][]string{}
	for _, a := range all {
		principal, err := identity.PrincipalIDFromUUIDBytes(a.principal.Bytes)
		if err != nil {
			return nil, err
		}
		if principal == ver.CreatedBy {
			return nil, domain.ErrSodConflict
		}
		reviewerVersion := ver
		reviewerVersion.CreatedBy = principal
		if err := s.productionActionTx(ctx, tx, reviewerVersion, domain.CommandReview); err != nil {
			return nil, domain.ErrReviewRequired
		}
		if err := s.validateProductionInputTx(ctx, tx, reviewerVersion, targets, false); err != nil {
			return nil, domain.ErrReviewRequired
		}
		var contributor bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_contributors WHERE workspace_id=$1 AND operation_id=$2 AND principal_id=$3)`, ver.WorkspaceID.UUID(), ver.OperationID.UUID(), principal.UUID()).Scan(&contributor); err != nil {
			return nil, err
		}
		if contributor {
			return nil, domain.ErrReviewRequired
		}
		proposal, err := identity.ProposalIDFromUUIDBytes(a.proposal.Bytes)
		if err != nil {
			return nil, err
		}
		review, err := identity.ReviewIDFromUUIDBytes(a.review.Bytes)
		if err != nil {
			return nil, err
		}
		result[proposal.String()] = append(result[proposal.String()], review.String())
	}
	for _, target := range targets {
		if target.ProposalID != nil && len(result[target.ProposalID.String()]) == 0 {
			return nil, domain.ErrReviewRequired
		}
	}
	for _, ids := range result {
		sort.Strings(ids)
	}
	return result, nil
}
