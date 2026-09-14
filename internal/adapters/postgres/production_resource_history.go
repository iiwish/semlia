package postgres

import (
	"context"
	"encoding/json"

	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func productionBusinessResources(ctx context.Context, tx pgx.Tx, w identity.WorkspaceID, kind string, content json.RawMessage) ([]authz.Resource, error) {
	resolved, err := domain.ResolveLocalReferences(content, nil)
	if err != nil {
		return nil, domain.ErrPriorStateUnknown
	}
	var object struct {
		Asset string `json:"asset"`
		Left  string `json:"leftDataset"`
		Right string `json:"rightDataset"`
	}
	if json.Unmarshal(resolved, &object) != nil {
		return nil, domain.ErrPriorStateUnknown
	}
	if kind != domain.TargetKindJoinContract {
		resource, err := productionAssetResource(ctx, tx, w, object.Asset)
		if err != nil {
			return nil, err
		}
		return []authz.Resource{resource}, nil
	}
	resources := []authz.Resource{}
	for _, value := range []string{object.Left, object.Right} {
		id, err := identity.ParsePhysicalDatasetID(value)
		if err != nil {
			return nil, domain.ErrPriorStateUnknown
		}
		var source pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT source_connection_id FROM physical_datasets WHERE workspace_id=$1 AND id=$2`, w.UUID(), id.UUID()).Scan(&source); err != nil {
			return nil, governanceRepositoryError("read published dataset source", err)
		}
		resources = append(resources, authz.Resource{Type: authz.ScopeSource, ID: formatUUID(source)})
	}
	return resources, nil
}
