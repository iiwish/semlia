package postgres

import (
	"context"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	application "github.com/iiwish/semlia/internal/application/projection"
	domain "github.com/iiwish/semlia/internal/domain/projection"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

var _ application.Loader = (*Store)(nil)

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
