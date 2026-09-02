package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	domain "github.com/iiwish/semlia/internal/domain/catalog"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
)

func (store *Store) ListCatalogWorkspaces(ctx context.Context) ([]domain.Workspace, error) {
	rows, err := store.queries.ListWorkspaces(ctx)
	if err != nil {
		return nil, repositoryError("list workspaces", err)
	}
	items := make([]domain.Workspace, 0, len(rows))
	for _, row := range rows {
		item, decodeErr := catalogWorkspace(row)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode workspace: %w", decodeErr)
		}
		items = append(items, item)
	}
	return items, nil
}

func (store *Store) CreateCatalogWorkspace(ctx context.Context, command domain.CreateWorkspaceCommand) (domain.Workspace, error) {
	workspaceID, err := uuidValue(command.ID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	auditID, err := uuidValue(command.AuditEventID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("encode workspace audit ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("begin workspace transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	row, err := queries.CreateWorkspace(ctx, dbgen.CreateWorkspaceParams{
		ID: workspaceID, Slug: command.Slug, DisplayName: command.DisplayName,
	})
	if err != nil {
		return domain.Workspace{}, repositoryError("create workspace", err)
	}
	payload, _ := json.Marshal(map[string]string{
		"workspaceId": command.ID.String(), "slug": command.Slug, "displayName": command.DisplayName,
	})
	if err := queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		ID: auditID, WorkspaceID: workspaceID, EventType: "workspace.created", Payload: payload,
		TraceID: command.TraceID, CreatedAt: timestamp(command.CreatedAt),
	}); err != nil {
		return domain.Workspace{}, repositoryError("create workspace audit event", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Workspace{}, fmt.Errorf("commit workspace transaction: %w", err)
	}
	return catalogWorkspace(row)
}

func catalogWorkspace(row dbgen.Workspace) (domain.Workspace, error) {
	id, err := identity.WorkspaceIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.Workspace{}, err
	}
	return domain.Workspace{
		ID: id, Slug: row.Slug, DisplayName: row.DisplayName,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}
