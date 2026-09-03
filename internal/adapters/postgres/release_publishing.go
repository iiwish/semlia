package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ governanceapp.PublishingRepository = (*Store)(nil)

func (store *Store) ListApprovingReviews(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) ([]governance.Review, error) {
	workspaceID, proposalID, err := governanceIDs(workspace, proposal)
	if err != nil {
		return nil, err
	}
	rows, err := store.queries.ListApprovingReviews(ctx, dbgen.ListApprovingReviewsParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
	})
	if err != nil {
		return nil, governanceRepositoryError("list approving reviews", err)
	}
	reviews := make([]governance.Review, 0, len(rows))
	for _, row := range rows {
		mapped, mapErr := reviewFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		reviews = append(reviews, mapped)
	}
	return reviews, nil
}

func (store *Store) ListReleaseObjects(
	ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID,
) ([]governance.ObjectManifestEntry, error) {
	workspaceID, releaseID, err := releaseIDs(workspace, release)
	if err != nil {
		return nil, err
	}
	rows, err := store.queries.ListReleaseObjects(ctx, dbgen.ListReleaseObjectsParams{
		WorkspaceID: workspaceID, ReleaseID: releaseID,
	})
	if err != nil {
		return nil, governanceRepositoryError("list release objects", err)
	}
	entries := make([]governance.ObjectManifestEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, governance.ObjectManifestEntry{
			ObjectType: governance.TargetObjectType(row.ObjectType), ObjectID: row.ObjectID.String(),
			Version: int(row.Version), Position: int(row.Position),
		})
	}
	return entries, nil
}

func (store *Store) ListReleases(
	ctx context.Context, workspace identity.WorkspaceID, limit int, cursor *governanceapp.ReleaseCursor,
) ([]governance.Release, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	params := dbgen.ListReleasesParams{
		WorkspaceID: workspaceID, HasCursor: cursor != nil, PageLimit: int32(limit),
	}
	if cursor != nil {
		params.CursorPublishedAt = timestamp(cursor.PublishedAt)
		if params.CursorID, err = uuidValue(cursor.ID); err != nil {
			return nil, fmt.Errorf("encode cursor ID: %w", err)
		}
	}
	rows, err := store.queries.ListReleases(ctx, params)
	if err != nil {
		return nil, governanceRepositoryError("list releases", err)
	}
	releases := make([]governance.Release, 0, len(rows))
	for _, row := range rows {
		release, mapErr := releaseFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		releases = append(releases, release)
	}
	return releases, nil
}

func (store *Store) MaxReleaseSequence(ctx context.Context, workspace identity.WorkspaceID) (int64, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return 0, fmt.Errorf("encode workspace ID: %w", err)
	}
	return store.queries.GetMaxReleaseSequence(ctx, workspaceID)
}

func (store *Store) LatestAssetPinBefore(
	ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID, sequence int64,
) (identity.RevisionID, bool, error) {
	workspaceID, assetID, err := catalogIDs(workspace, asset)
	if err != nil {
		return identity.RevisionID{}, false, err
	}
	pinned, err := store.queries.GetLatestAssetPinBefore(ctx, dbgen.GetLatestAssetPinBeforeParams{
		WorkspaceID: workspaceID, AssetID: assetID, Sequence: sequence,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.RevisionID{}, false, nil
	}
	if err != nil {
		return identity.RevisionID{}, false, governanceRepositoryError("find latest asset pin", err)
	}
	revisionID, err := identity.RevisionIDFromUUIDBytes(pinned.Bytes)
	if err != nil {
		return identity.RevisionID{}, false, err
	}
	return revisionID, true, nil
}

func (store *Store) RecordReleaseDenial(ctx context.Context, denial governanceapp.ReleaseDenialRecord) error {
	workspaceID, err := uuidValue(denial.WorkspaceID)
	if err != nil {
		return fmt.Errorf("encode workspace ID: %w", err)
	}
	auditID, err := uuidValue(denial.AuditEventID)
	if err != nil {
		return fmt.Errorf("encode denial audit event ID: %w", err)
	}
	payload := map[string]any{
		"specVersion":  "semlia.release/v1",
		"action":       "denied",
		"reasonCode":   denial.ReasonCode,
		"policySource": denial.PolicySource,
	}
	if denial.Conflict != "" {
		payload["conflict"] = denial.Conflict
	}
	if denial.ProposalID != nil {
		payload["proposalId"] = denial.ProposalID.String()
	}
	if denial.ReleaseID != nil {
		payload["releaseId"] = denial.ReleaseID.String()
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return store.queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		ID: auditID, WorkspaceID: workspaceID, EventType: "governance.release.denied",
		ActorID: textValue(denial.Actor), Payload: encoded, TraceID: denial.TraceID,
		CreatedAt: timestamp(denial.CreatedAt),
	})
}

// PublishRelease commits the whole governed publish in one transaction: the
// locked change-set application (new asset revision with current-revision
// switch, or governed-object version bump), the immutable release row with
// its manifest pins, the proposal walk to released, and every audit fact plus
// outbox event (catalog.asset.changed, proposal.changed, release.published).
func (store *Store) PublishRelease(
	ctx context.Context, command governanceapp.PublishReleaseCommand,
) (governance.Release, error) {
	workspaceID, proposalID, err := governanceIDs(command.WorkspaceID, command.ProposalID)
	if err != nil {
		return governance.Release{}, err
	}
	releaseID, err := uuidValue(command.ReleaseID)
	if err != nil {
		return governance.Release{}, fmt.Errorf("encode release ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.Release{}, governanceRepositoryError("begin release publish", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	if _, err := queries.LockWorkspace(ctx, workspaceID); err != nil {
		return governance.Release{}, governanceRepositoryError("lock workspace for release publish", err)
	}
	locked, err := queries.GetProposalForUpdate(ctx, dbgen.GetProposalForUpdateParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
	})
	if err != nil {
		return governance.Release{}, governanceRepositoryError("lock proposal for release publish", err)
	}
	if locked.State != string(governance.ProposalInReview) {
		return governance.Release{}, fmt.Errorf(
			"%w: proposal is %s, not in_review", governance.ErrInvariant, locked.State)
	}
	proposal, err := proposalFromRow(locked)
	if err != nil {
		return governance.Release{}, err
	}

	var entries []governance.ManifestEntry
	var objects []governance.ObjectManifestEntry
	switch proposal.TargetObjectType {
	case governance.TargetSemanticAsset:
		assetEntry, applyErr := store.publishAssetRevision(ctx, queries, command, proposal)
		if applyErr != nil {
			return governance.Release{}, applyErr
		}
		entries = append(entries, assetEntry)
	case governance.TargetPhysicalBinding, governance.TargetModelGrain,
		governance.TargetEntityKey, governance.TargetJoinContract:
		objectEntry, applyErr := store.publishGovernedObject(ctx, queries, command, proposal, command.ObjectAuditID)
		if applyErr != nil {
			return governance.Release{}, applyErr
		}
		objects = append(objects, objectEntry)
	default:
		return governance.Release{}, fmt.Errorf(
			"%w: proposal target type %q is not publishable", governance.ErrInvariant, proposal.TargetObjectType)
	}

	manifestPayload, err := governance.ManifestDigestPayloadWithObjects(entries, objects)
	if err != nil {
		return governance.Release{}, err
	}
	manifestDigest, err := governance.DigestJSON(manifestPayload)
	if err != nil {
		return governance.Release{}, err
	}
	sequence, err := queries.NextReleaseSequence(ctx, workspaceID)
	if err != nil {
		return governance.Release{}, governanceRepositoryError("allocate release sequence", err)
	}
	created, err := queries.CreateRelease(ctx, dbgen.CreateReleaseParams{
		ID: releaseID, WorkspaceID: workspaceID, Sequence: sequence,
		ManifestDigest: manifestDigest, State: string(governance.ReleasePublished),
		OriginProposalID: proposalID, PublishedBy: command.Publisher,
		PublishedAt: timestamp(command.AppliedAt), CreatedAt: timestamp(command.AppliedAt),
	})
	if err != nil {
		return governance.Release{}, governanceRepositoryError("create published release", err)
	}
	for _, entry := range entries {
		if err := store.persistAssetEntry(ctx, queries, command.WorkspaceID, command.ReleaseID, entry, command.AppliedAt); err != nil {
			return governance.Release{}, err
		}
	}
	for _, object := range objects {
		if err := store.persistObjectEntry(ctx, queries, command.WorkspaceID, command.ReleaseID, object, command.AppliedAt); err != nil {
			return governance.Release{}, err
		}
	}
	decidedAt := command.AppliedAt
	if _, err := queries.TransitionProposal(ctx, dbgen.TransitionProposalParams{
		WorkspaceID: workspaceID, ProposalID: proposalID, State: string(governance.ProposalReleased),
		DecidedAt: timestamp(decidedAt), ExpectedState: string(governance.ProposalInReview),
		UpdatedAt: timestamp(decidedAt),
	}); err != nil {
		return governance.Release{}, governanceRepositoryError("release proposal", err)
	}
	var assetID *identity.AssetID
	if locked.AssetID.Valid {
		parsedAssetID, parseErr := identity.AssetIDFromUUIDBytes(locked.AssetID.Bytes)
		if parseErr != nil {
			return governance.Release{}, parseErr
		}
		assetID = &parsedAssetID
	}
	if err := createProposalMutationEvents(ctx, queries, proposalEvent{
		WorkspaceID: command.WorkspaceID, ProposalID: command.ProposalID, AssetID: assetID,
		AuditID: command.ProposalEvents.AuditEventID, OutboxID: command.ProposalEvents.OutboxEventID,
		Action: "released", FromState: string(governance.ProposalInReview),
		ToState: string(governance.ProposalReleased), Actor: command.Publisher,
		TraceID: command.TraceID, CreatedAt: command.AppliedAt,
	}); err != nil {
		return governance.Release{}, governanceRepositoryError("record released proposal events", err)
	}
	if err := createReleaseMutationEvents(ctx, queries, releaseEvent{
		WorkspaceID: command.WorkspaceID, ReleaseID: command.ReleaseID,
		AuditID: command.ReleaseEvents.AuditEventID, OutboxID: command.ReleaseEvents.OutboxEventID,
		Action: "published", ManifestDigest: manifestDigest, Sequence: sequence,
		Actor: command.Publisher, TraceID: command.TraceID, CreatedAt: command.AppliedAt,
	}); err != nil {
		return governance.Release{}, governanceRepositoryError("record release publish events", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.Release{}, governanceRepositoryError("commit release publish", err)
	}
	release, err := releaseFromRow(created)
	if err != nil {
		return governance.Release{}, err
	}
	release.Entries = entries
	release.Objects = objects
	return release, nil
}

func (store *Store) publishAssetRevision(
	ctx context.Context,
	queries *dbgen.Queries,
	command governanceapp.PublishReleaseCommand,
	proposal governance.Proposal,
) (governance.ManifestEntry, error) {
	if proposal.AssetID == nil || proposal.BaseRevisionID == nil {
		return governance.ManifestEntry{}, fmt.Errorf(
			"%w: semantic asset proposal without asset or base revision", governance.ErrInvariant)
	}
	workspaceID, assetID, err := catalogIDs(command.WorkspaceID, *proposal.AssetID)
	if err != nil {
		return governance.ManifestEntry{}, err
	}
	baseRevisionID, err := uuidValue(*proposal.BaseRevisionID)
	if err != nil {
		return governance.ManifestEntry{}, fmt.Errorf("encode base revision ID: %w", err)
	}
	lockedAsset, err := queries.GetCatalogAssetForUpdate(ctx, dbgen.GetCatalogAssetForUpdateParams{
		WorkspaceID: workspaceID, AssetID: assetID,
	})
	if err != nil {
		return governance.ManifestEntry{}, governanceRepositoryError("lock catalog asset for publish", err)
	}
	if !lockedAsset.CurrentRevisionID.Valid || lockedAsset.CurrentRevisionID != baseRevisionID {
		return governance.ManifestEntry{}, fmt.Errorf(
			"%w: proposal base revision is no longer the current revision of the asset", governance.ErrConflict)
	}
	current, err := queries.GetAssetRevisionContent(ctx, dbgen.GetAssetRevisionContentParams{
		WorkspaceID: workspaceID, AssetID: assetID, RevisionID: baseRevisionID,
	})
	if err != nil {
		return governance.ManifestEntry{}, governanceRepositoryError("read proposal base revision", err)
	}
	newContent, err := command.Apply(current.Content)
	if err != nil {
		return governance.ManifestEntry{}, err
	}
	digest := sha256.Sum256(newContent)
	revisionID, err := identity.NewRevisionID()
	if err != nil {
		return governance.ManifestEntry{}, fmt.Errorf("mint published revision ID: %w", err)
	}
	revisionUUID, err := uuidValue(revisionID)
	if err != nil {
		return governance.ManifestEntry{}, err
	}
	sequence, err := queries.NextAssetRevisionSequence(ctx, dbgen.NextAssetRevisionSequenceParams{
		WorkspaceID: workspaceID, AssetID: assetID,
	})
	if err != nil {
		return governance.ManifestEntry{}, governanceRepositoryError("allocate asset revision sequence", err)
	}
	if _, err := queries.CreateAssetRevision(ctx, dbgen.CreateAssetRevisionParams{
		ID: revisionUUID, WorkspaceID: workspaceID, AssetID: assetID, Sequence: sequence,
		SchemaVersion: current.SchemaVersion, ContentDigest: "sha256:" + hex.EncodeToString(digest[:]),
		Content: newContent, CreatedBy: command.Publisher,
	}); err != nil {
		return governance.ManifestEntry{}, governanceRepositoryError("append published asset revision", err)
	}
	if rows, err := queries.SetCurrentAssetRevision(ctx, dbgen.SetCurrentAssetRevisionParams{
		RevisionID: revisionUUID, WorkspaceID: workspaceID, AssetID: assetID,
	}); err != nil {
		return governance.ManifestEntry{}, governanceRepositoryError("switch current asset revision", err)
	} else if rows != 1 {
		return governance.ManifestEntry{}, fmt.Errorf(
			"%w: current revision switch updated %d rows", governance.ErrInvariant, rows)
	}
	baseRevisionTyped, err := identity.RevisionIDFromUUIDBytes(baseRevisionID.Bytes)
	if err != nil {
		return governance.ManifestEntry{}, err
	}
	if err := createCatalogMutationEvents(ctx, queries, catalogEvent{
		WorkspaceID: command.WorkspaceID, AssetID: *proposal.AssetID, RevisionID: revisionID,
		BaseRevisionID: &baseRevisionTyped, AuditID: command.AssetEvents.AuditEventID,
		OutboxID: command.AssetEvents.OutboxEventID, Sequence: sequence, Action: "revision.created",
		Actor: command.Publisher, TraceID: command.TraceID, CreatedAt: command.AppliedAt,
	}); err != nil {
		return governance.ManifestEntry{}, err
	}
	return governance.ManifestEntry{
		AssetID: *proposal.AssetID, RevisionID: revisionID,
		Compatibility: json.RawMessage(`{}`), Position: 1,
	}, nil
}

func (store *Store) publishGovernedObject(
	ctx context.Context,
	queries *dbgen.Queries,
	command governanceapp.PublishReleaseCommand,
	proposal governance.Proposal,
	auditID identity.EventID,
) (governance.ObjectManifestEntry, error) {
	spec, err := governedObjectSpecFor(proposal.TargetObjectType)
	if err != nil {
		return governance.ObjectManifestEntry{}, err
	}
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return governance.ObjectManifestEntry{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	objectID, err := uuidFromString(proposal.TargetObjectID)
	if err != nil {
		return governance.ObjectManifestEntry{}, err
	}
	version, err := spec.lock(ctx, queries, workspaceID, objectID)
	if err != nil {
		return governance.ObjectManifestEntry{}, governanceRepositoryError("lock governed object for publish", err)
	}
	object, err := spec.get(ctx, queries, workspaceID, objectID)
	if err != nil {
		return governance.ObjectManifestEntry{}, governanceRepositoryError("read governed object for publish", err)
	}
	newContent, err := command.Apply(governedObjectContent(object))
	if err != nil {
		return governance.ObjectManifestEntry{}, err
	}
	rows, err := spec.apply(ctx, queries, governedApplyParams{
		WorkspaceID: workspaceID, ObjectID: objectID,
		ExpectedVersion: int32(version), Content: objectJSON(newContent),
		UpdatedAt: timestamp(command.AppliedAt),
	})
	if err != nil {
		return governance.ObjectManifestEntry{}, governanceRepositoryError("apply governed change for publish", err)
	}
	if rows != 1 {
		return governance.ObjectManifestEntry{}, fmt.Errorf(
			"%w: governed change updated %d rows", governance.ErrInvariant, rows)
	}
	if err := createGovernedObjectAuditEvent(ctx, queries, governedObjectEvent{
		WorkspaceID: command.WorkspaceID, ObjectType: string(proposal.TargetObjectType),
		ObjectID: proposal.TargetObjectID, AuditID: auditID, Action: "applied",
		Version: int(version) + 1, Summary: "published through release",
		Actor: command.Publisher, TraceID: command.TraceID, CreatedAt: command.AppliedAt,
	}); err != nil {
		return governance.ObjectManifestEntry{}, err
	}
	return governance.ObjectManifestEntry{
		ObjectType: proposal.TargetObjectType, ObjectID: proposal.TargetObjectID,
		Version: int(version) + 1, Position: 1,
	}, nil
}

// RestoreRelease commits the rollback-as-new-release in one transaction: the
// prior-state restores (asset revision pointer switches with their
// catalog.asset.changed events, governed-object inverse patches with their
// audit facts), the new immutable release row referencing the target, the
// manifest pins, and the audit fact plus release.published outbox event. The
// target release rows are never touched.
func (store *Store) RestoreRelease(
	ctx context.Context, command governanceapp.RestoreReleaseCommand,
) (governance.Release, error) {
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return governance.Release{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	releaseID, err := uuidValue(command.ReleaseID)
	if err != nil {
		return governance.Release{}, fmt.Errorf("encode rollback release ID: %w", err)
	}
	targetID, err := uuidValue(command.Target.ID)
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
	rollbacks, err := queries.CountReleaseRollbacks(ctx, dbgen.CountReleaseRollbacksParams{
		WorkspaceID: workspaceID, ReleaseID: targetID,
	})
	if err != nil {
		return governance.Release{}, governanceRepositoryError("re-check rollback count", err)
	}
	if rollbacks > 0 {
		return governance.Release{}, fmt.Errorf(
			"%w: release %s has already been rolled back", governance.ErrConflict, command.Target.ID)
	}
	for _, restore := range command.AssetRestores {
		if err := store.restoreAssetRevision(ctx, queries, command, restore); err != nil {
			return governance.Release{}, err
		}
	}
	objects := make([]governance.ObjectManifestEntry, 0, len(command.Objects))
	if command.ObjectRestore != nil {
		restored, restoreErr := store.restoreGovernedObject(ctx, queries, command, *command.ObjectRestore)
		if restoreErr != nil {
			return governance.Release{}, restoreErr
		}
		objects = append(objects, restored)
	}
	manifestPayload, err := governance.ManifestDigestPayloadWithObjects(command.Entries, command.Objects)
	if err != nil {
		return governance.Release{}, err
	}
	manifestDigest, err := governance.DigestJSON(manifestPayload)
	if err != nil {
		return governance.Release{}, err
	}
	sequence, err := queries.NextReleaseSequence(ctx, workspaceID)
	if err != nil {
		return governance.Release{}, governanceRepositoryError("allocate rollback sequence", err)
	}
	created, err := queries.CreateRelease(ctx, dbgen.CreateReleaseParams{
		ID: releaseID, WorkspaceID: workspaceID, Sequence: sequence,
		ManifestDigest: manifestDigest, State: string(governance.ReleasePublished),
		RolledBackToReleaseID: targetID, OriginProposalID: pgtype.UUID{},
		PublishedBy: command.Publisher,
		PublishedAt: timestamp(command.AppliedAt), CreatedAt: timestamp(command.AppliedAt),
	})
	if err != nil {
		return governance.Release{}, governanceRepositoryError("create rollback release", err)
	}
	for _, entry := range command.Entries {
		if err := store.persistAssetEntry(ctx, queries, command.WorkspaceID, command.ReleaseID, entry, command.AppliedAt); err != nil {
			return governance.Release{}, err
		}
	}
	for _, object := range objects {
		if err := store.persistObjectEntry(ctx, queries, command.WorkspaceID, command.ReleaseID, object, command.AppliedAt); err != nil {
			return governance.Release{}, err
		}
	}
	if err := createReleaseMutationEvents(ctx, queries, releaseEvent{
		WorkspaceID: command.WorkspaceID, ReleaseID: command.ReleaseID,
		AuditID: command.ReleaseEvents.AuditEventID, OutboxID: command.ReleaseEvents.OutboxEventID,
		Action: "rolled_back", ManifestDigest: manifestDigest, Sequence: sequence,
		RolledBackToReleaseID: &command.Target.ID,
		Actor:                 command.Publisher, TraceID: command.TraceID, CreatedAt: command.AppliedAt,
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
	release.Objects = objects
	return release, nil
}

func (store *Store) restoreAssetRevision(
	ctx context.Context,
	queries *dbgen.Queries,
	command governanceapp.RestoreReleaseCommand,
	restore governanceapp.AssetRestore,
) error {
	workspaceID, assetID, err := catalogIDs(command.WorkspaceID, restore.AssetID)
	if err != nil {
		return err
	}
	revisionID, err := uuidValue(restore.RevisionID)
	if err != nil {
		return fmt.Errorf("encode restore revision ID: %w", err)
	}
	lockedAsset, err := queries.GetCatalogAssetForUpdate(ctx, dbgen.GetCatalogAssetForUpdateParams{
		WorkspaceID: workspaceID, AssetID: assetID,
	})
	if err != nil {
		return governanceRepositoryError("lock catalog asset for rollback", err)
	}
	if _, err := queries.GetAssetRevisionOwnership(ctx, dbgen.GetAssetRevisionOwnershipParams{
		WorkspaceID: workspaceID, AssetID: assetID, RevisionID: revisionID,
	}); err != nil {
		return governanceRepositoryError("verify rollback revision ownership", err)
	}
	if rows, err := queries.SetCurrentAssetRevision(ctx, dbgen.SetCurrentAssetRevisionParams{
		RevisionID: revisionID, WorkspaceID: workspaceID, AssetID: assetID,
	}); err != nil {
		return governanceRepositoryError("restore current asset revision", err)
	} else if rows != 1 {
		return fmt.Errorf("%w: rollback pointer switch updated %d rows", governance.ErrInvariant, rows)
	}
	prior, err := queries.GetAssetRevisionContent(ctx, dbgen.GetAssetRevisionContentParams{
		WorkspaceID: workspaceID, AssetID: assetID, RevisionID: revisionID,
	})
	if err != nil {
		return governanceRepositoryError("read restored revision", err)
	}
	var baseRevisionID *identity.RevisionID
	if lockedAsset.CurrentRevisionID.Valid {
		decoded, decodeErr := identity.RevisionIDFromUUIDBytes(lockedAsset.CurrentRevisionID.Bytes)
		if decodeErr != nil {
			return decodeErr
		}
		baseRevisionID = &decoded
	}
	return createCatalogMutationEvents(ctx, queries, catalogEvent{
		WorkspaceID: command.WorkspaceID, AssetID: restore.AssetID, RevisionID: restore.RevisionID,
		BaseRevisionID: baseRevisionID, AuditID: restore.EventIDs.AuditEventID,
		OutboxID: restore.EventIDs.OutboxEventID, Sequence: prior.Sequence, Action: "revision.created",
		Actor: command.Publisher, TraceID: command.TraceID, CreatedAt: command.AppliedAt,
	})
}

func (store *Store) restoreGovernedObject(
	ctx context.Context,
	queries *dbgen.Queries,
	command governanceapp.RestoreReleaseCommand,
	restore governanceapp.ObjectRestore,
) (governance.ObjectManifestEntry, error) {
	spec, err := governedObjectSpecFor(restore.ObjectType)
	if err != nil {
		return governance.ObjectManifestEntry{}, err
	}
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return governance.ObjectManifestEntry{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	objectID, err := uuidFromString(restore.ObjectID)
	if err != nil {
		return governance.ObjectManifestEntry{}, err
	}
	version, err := spec.lock(ctx, queries, workspaceID, objectID)
	if err != nil {
		return governance.ObjectManifestEntry{}, governanceRepositoryError("lock governed object for rollback", err)
	}
	object, err := spec.get(ctx, queries, workspaceID, objectID)
	if err != nil {
		return governance.ObjectManifestEntry{}, governanceRepositoryError("read governed object for rollback", err)
	}
	newContent, err := restore.ApplyUpdate(governedObjectContent(object))
	if err != nil {
		return governance.ObjectManifestEntry{}, err
	}
	rows, err := spec.apply(ctx, queries, governedApplyParams{
		WorkspaceID: workspaceID, ObjectID: objectID,
		ExpectedVersion: int32(version), Content: objectJSON(newContent),
		UpdatedAt: timestamp(command.AppliedAt),
	})
	if err != nil {
		return governance.ObjectManifestEntry{}, governanceRepositoryError("apply governed change for rollback", err)
	}
	if rows != 1 {
		return governance.ObjectManifestEntry{}, fmt.Errorf(
			"%w: rollback governed change updated %d rows", governance.ErrInvariant, rows)
	}
	if err := createGovernedObjectAuditEvent(ctx, queries, governedObjectEvent{
		WorkspaceID: command.WorkspaceID, ObjectType: string(restore.ObjectType),
		ObjectID: restore.ObjectID, AuditID: restore.AuditID, Action: "applied",
		Version: int(version) + 1, Summary: "restored through release rollback",
		Actor: command.Publisher, TraceID: command.TraceID, CreatedAt: command.AppliedAt,
	}); err != nil {
		return governance.ObjectManifestEntry{}, err
	}
	return governance.ObjectManifestEntry{
		ObjectType: restore.ObjectType, ObjectID: restore.ObjectID,
		Version: int(version) + 1, Position: 1,
	}, nil
}

func (store *Store) persistAssetEntry(
	ctx context.Context,
	queries *dbgen.Queries,
	workspace identity.WorkspaceID,
	release identity.ReleaseID,
	entry governance.ManifestEntry,
	createdAt time.Time,
) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	workspaceID, releaseID, err := releaseIDs(workspace, release)
	if err != nil {
		return err
	}
	assetID, err := uuidValue(entry.AssetID)
	if err != nil {
		return fmt.Errorf("encode manifest asset ID: %w", err)
	}
	revisionID, err := uuidValue(entry.RevisionID)
	if err != nil {
		return fmt.Errorf("encode manifest revision ID: %w", err)
	}
	if _, err := queries.GetAssetRevisionOwnership(ctx, dbgen.GetAssetRevisionOwnershipParams{
		WorkspaceID: workspaceID, AssetID: assetID, RevisionID: revisionID,
	}); err != nil {
		return governanceRepositoryError("verify manifest entry", err)
	}
	if err := queries.CreateReleaseAsset(ctx, dbgen.CreateReleaseAssetParams{
		WorkspaceID: workspaceID, ReleaseID: releaseID, AssetID: assetID, RevisionID: revisionID,
		Compatibility: objectJSON(entry.Compatibility), Position: int32(entry.Position),
		CreatedAt: timestamp(createdAt),
	}); err != nil {
		return governanceRepositoryError("create release asset", err)
	}
	return nil
}

func (store *Store) persistObjectEntry(
	ctx context.Context,
	queries *dbgen.Queries,
	workspace identity.WorkspaceID,
	release identity.ReleaseID,
	object governance.ObjectManifestEntry,
	createdAt time.Time,
) error {
	if err := object.Validate(); err != nil {
		return err
	}
	workspaceID, releaseID, err := releaseIDs(workspace, release)
	if err != nil {
		return err
	}
	objectID, err := uuidFromString(object.ObjectID)
	if err != nil {
		return fmt.Errorf("encode manifest object ID: %w", err)
	}
	if err := queries.CreateReleaseObject(ctx, dbgen.CreateReleaseObjectParams{
		WorkspaceID: workspaceID, ReleaseID: releaseID, ObjectType: string(object.ObjectType),
		ObjectID: objectID, Version: int32(object.Version), Position: int32(object.Position),
		CreatedAt: timestamp(createdAt),
	}); err != nil {
		return governanceRepositoryError("create release object", err)
	}
	return nil
}

func governanceIDs(workspace identity.WorkspaceID, proposal identity.ProposalID) (pgtype.UUID, pgtype.UUID, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal)
	return workspaceID, proposalID, err
}

func releaseIDs(workspace identity.WorkspaceID, release identity.ReleaseID) (pgtype.UUID, pgtype.UUID, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	releaseID, err := uuidValue(release)
	return workspaceID, releaseID, err
}

func governedObjectContent(object governance.GovernedObject) json.RawMessage {
	switch object.Type {
	case governance.TargetPhysicalBinding:
		return object.PhysicalBinding.Content
	case governance.TargetModelGrain:
		return object.ModelGrain.Content
	case governance.TargetEntityKey:
		return object.EntityKey.Content
	case governance.TargetJoinContract:
		return object.JoinContract.Content
	}
	return nil
}
