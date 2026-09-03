package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	application "github.com/iiwish/semlia/internal/application/projection"
	"github.com/iiwish/semlia/internal/domain/governance"
	domain "github.com/iiwish/semlia/internal/domain/projection"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	_ application.Loader        = (*Store)(nil)
	_ application.ReleaseLoader = (*Store)(nil)
)

func (store *Store) LoadAssetProjection(
	ctx context.Context,
	workspace identity.WorkspaceID,
	asset identity.AssetID,
	revision identity.RevisionID,
) (domain.Asset, error) {
	workspaceID, assetID, err := catalogIDs(workspace, asset)
	if err != nil {
		return domain.Asset{}, err
	}
	revisionID, err := uuidValue(revision)
	if err != nil {
		return domain.Asset{}, err
	}
	row, err := store.queries.GetAssetProjection(ctx, dbgen.GetAssetProjectionParams{
		WorkspaceID: workspaceID, AssetID: assetID, RevisionID: revisionID,
	})
	if err != nil {
		return domain.Asset{}, repositoryError("load asset projection", err)
	}
	address, err := semantic.NewAddress(row.Namespace, row.Key)
	if err != nil {
		return domain.Asset{}, err
	}
	return domain.Asset{
		WorkspaceID: workspace, AssetID: asset, RevisionID: revision, Address: address,
		AssetType: semantic.AssetType(row.AssetType), LifecycleState: row.LifecycleState,
		Sequence: row.Sequence, SchemaVersion: row.SchemaVersion, ContentDigest: row.ContentDigest,
		Content: cloneJSON(row.Content), CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (store *Store) LoadReleaseProjection(
	ctx context.Context,
	workspace identity.WorkspaceID,
	release identity.ReleaseID,
) (domain.Release, error) {
	workspaceID, releaseID, err := releaseIDs(workspace, release)
	if err != nil {
		return domain.Release{}, err
	}
	row, err := store.queries.GetReleaseProjection(ctx, dbgen.GetReleaseProjectionParams{
		WorkspaceID: workspaceID, ReleaseID: releaseID,
	})
	if err != nil {
		return domain.Release{}, repositoryError("load release projection", err)
	}
	result := domain.Release{
		WorkspaceID: workspace, ReleaseID: release, Sequence: row.Sequence,
		ManifestDigest: row.ManifestDigest, State: row.State, PublishedBy: row.PublishedBy,
		PublishedAt: row.PublishedAt.Time,
	}
	if row.RolledBackToReleaseID.Valid {
		targetID, decodeErr := identity.ReleaseIDFromUUIDBytes(row.RolledBackToReleaseID.Bytes)
		if decodeErr != nil {
			return domain.Release{}, decodeErr
		}
		result.RolledBackToReleaseID = &targetID
	}
	assetRows, err := store.queries.ListReleaseAssetProjection(ctx, dbgen.ListReleaseAssetProjectionParams{
		WorkspaceID: workspaceID, ReleaseID: releaseID,
	})
	if err != nil {
		return domain.Release{}, repositoryError("list release asset projection", err)
	}
	assets := make([]domain.ManifestAssetProjection, 0, len(assetRows))
	for _, assetRow := range assetRows {
		assetID, decodeErr := identity.AssetIDFromUUIDBytes(assetRow.AssetID.Bytes)
		if decodeErr != nil {
			return domain.Release{}, decodeErr
		}
		revisionID, decodeErr := identity.RevisionIDFromUUIDBytes(assetRow.RevisionID.Bytes)
		if decodeErr != nil {
			return domain.Release{}, decodeErr
		}
		address, decodeErr := semantic.NewAddress(assetRow.Namespace, assetRow.Key)
		if decodeErr != nil {
			return domain.Release{}, decodeErr
		}
		assets = append(assets, domain.ManifestAssetProjection{
			AssetID: assetID, RevisionID: revisionID, Address: address,
			Compatibility: cloneJSON(assetRow.Compatibility), Position: int(assetRow.Position),
		})
	}
	objectRows, err := store.queries.ListReleaseObjectProjection(ctx, dbgen.ListReleaseObjectProjectionParams{
		WorkspaceID: workspaceID, ReleaseID: releaseID,
	})
	if err != nil {
		return domain.Release{}, repositoryError("list release object projection", err)
	}
	objects := make([]domain.ManifestObjectProjection, 0, len(objectRows))
	for _, objectRow := range objectRows {
		prefix, prefixErr := governance.TargetObjectType(objectRow.ObjectType).Prefix()
		if prefixErr != nil {
			return domain.Release{}, prefixErr
		}
		typedID, decodeErr := identity.FromUUID(prefix, objectRow.ObjectID.String())
		if decodeErr != nil {
			return domain.Release{}, decodeErr
		}
		objects = append(objects, domain.ManifestObjectProjection{
			ObjectType: objectRow.ObjectType, ObjectID: typedID.String(),
			Version: int(objectRow.Version), Position: int(objectRow.Position),
		})
	}
	var proposal *domain.ReleaseProposalMeta
	if row.OriginProposalID.Valid {
		proposalRow, proposalErr := store.queries.GetReleaseOriginProposalProjection(ctx, dbgen.GetReleaseOriginProposalProjectionParams{
			WorkspaceID: workspaceID, ProposalID: row.OriginProposalID,
		})
		if proposalErr != nil {
			return domain.Release{}, repositoryError("load release origin proposal", proposalErr)
		}
		proposalID, decodeErr := identity.ProposalIDFromUUIDBytes(proposalRow.ID.Bytes)
		if decodeErr != nil {
			return domain.Release{}, decodeErr
		}
		targetTypeID, decodeErr := proposalTargetTypeID(
			governance.TargetObjectType(proposalRow.TargetObjectType), proposalRow.AssetID, proposalRow.TargetObjectID,
		)
		if decodeErr != nil {
			return domain.Release{}, decodeErr
		}
		proposal = &domain.ReleaseProposalMeta{
			ProposalID: proposalID, Title: proposalRow.Title,
			TargetObjectType: proposalRow.TargetObjectType, TargetObjectID: targetTypeID,
			CreatedBy: proposalRow.CreatedBy,
		}
	}
	result.Assets = assets
	result.Objects = objects
	result.Proposal = proposal
	return result, nil
}

func proposalTargetTypeID(
	targetType governance.TargetObjectType,
	assetID pgtype.UUID,
	targetObjectID pgtype.UUID,
) (string, error) {
	if targetType == governance.TargetSemanticAsset {
		if !assetID.Valid {
			return "", fmt.Errorf("%w: semantic asset proposal without asset reference", governance.ErrInvariant)
		}
		parsed, err := identity.AssetIDFromUUIDBytes(assetID.Bytes)
		if err != nil {
			return "", err
		}
		return parsed.String(), nil
	}
	prefix, err := targetType.Prefix()
	if err != nil {
		return "", err
	}
	typed, err := identity.FromUUID(prefix, uuidToString(targetObjectID))
	if err != nil {
		return "", err
	}
	return typed.String(), nil
}

func uuidToString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}
