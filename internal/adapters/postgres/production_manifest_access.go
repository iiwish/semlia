package postgres

import (
	"context"
	"encoding/json"
	"time"

	authapp "github.com/iiwish/semlia/internal/application/authorization"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
)

// A release detail exposes unchanged pins as well as the operation's targets.
func (s *Store) AuthorizeProductionManifestRead(ctx context.Context, w identity.WorkspaceID, release identity.ReleaseID, principal identity.PrincipalID) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var version int64
	if err := tx.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1 FOR SHARE`, w.UUID()).Scan(&version); err != nil {
		return err
	}
	access, err := authapp.NewService(s, authapp.ClockFunc(time.Now)).Snapshot(ctx, w, principal)
	if err != nil {
		return err
	}
	if access.AuthorizationVersion != version {
		return domain.ErrVersionConflict
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT asset_id::text FROM release_assets WHERE workspace_id=$1 AND release_id IN ($2,(SELECT before_release_id FROM production_release_manifests WHERE workspace_id=$1 AND release_id=$2))`, w.UUID(), release.UUID())
	if err != nil {
		return err
	}
	assets := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		assets = append(assets, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, raw := range assets {
		id, err := identity.FromUUID(identity.Asset, raw)
		if err != nil {
			return err
		}
		resource, err := productionAssetResource(ctx, tx, w, id.String())
		if err != nil {
			return err
		}
		if err := productionRequire(access, authz.ActionAssetRead, resource); err != nil {
			return err
		}
	}
	rows, err = tx.Query(ctx, `SELECT object_type,payload FROM release_object_snapshots WHERE workspace_id=$1 AND release_id IN ($2,(SELECT before_release_id FROM production_release_manifests WHERE workspace_id=$1 AND release_id=$2))`, w.UUID(), release.UUID())
	if err != nil {
		return err
	}
	type object struct {
		kind string
		raw  []byte
	}
	objects := []object{}
	for rows.Next() {
		var item object
		if err := rows.Scan(&item.kind, &item.raw); err != nil {
			rows.Close()
			return err
		}
		objects = append(objects, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range objects {
		var snapshot map[string]json.RawMessage
		if json.Unmarshal(item.raw, &snapshot) != nil {
			return domain.ErrPriorStateUnknown
		}
		content, err := productionObjectBusinessBaseline(item.kind, snapshot)
		if err != nil {
			return err
		}
		resources, err := productionBusinessResources(ctx, tx, w, item.kind, content)
		if err != nil {
			return err
		}
		for _, resource := range resources {
			action := authz.ActionAssetRead
			if item.kind == domain.TargetKindJoinContract {
				action = authz.ActionSourceRead
			}
			if err := productionRequire(access, action, resource); err != nil {
				return err
			}
			if err := productionRequire(access, authz.ActionBindingRead, resource); err != nil {
				return err
			}
		}
	}
	rows, err = tx.Query(ctx, `SELECT DISTINCT s.source_connection_id::text FROM production_release_binding_inputs b JOIN source_snapshots s ON s.workspace_id=b.workspace_id AND s.id=b.snapshot_id WHERE b.workspace_id=$1 AND b.release_id IN ($2,(SELECT before_release_id FROM production_release_manifests WHERE workspace_id=$1 AND release_id=$2))`, w.UUID(), release.UUID())
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if err := productionRequire(access, authz.ActionSourceRead, authz.Resource{Type: authz.ScopeSource, ID: id}); err != nil {
			return err
		}
	}
	return rows.Err()
}
