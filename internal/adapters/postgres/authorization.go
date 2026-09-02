package postgres

import (
	"context"
	"errors"
	"fmt"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	authzapp "github.com/iiwish/semlia/internal/application/authorization"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ authzapp.Repository = (*Store)(nil)

func (store *Store) LoadPrincipal(
	ctx context.Context,
	workspace identity.WorkspaceID,
	principal identity.PrincipalID,
) (authz.Principal, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return authz.Principal{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	principalID, err := uuidValue(principal)
	if err != nil {
		return authz.Principal{}, fmt.Errorf("encode principal ID: %w", err)
	}
	row, err := store.queries.GetWorkspacePrincipal(ctx, dbgen.GetWorkspacePrincipalParams{
		WorkspaceID: workspaceID, PrincipalID: principalID,
	})
	if err != nil {
		return authz.Principal{}, authorizationRepositoryError("load principal", err)
	}
	return principalFromRow(row)
}

func (store *Store) LoadDefaultPrincipal(
	ctx context.Context, workspace identity.WorkspaceID,
) (authz.Principal, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return authz.Principal{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	row, err := store.queries.GetDefaultWorkspacePrincipal(ctx, workspaceID)
	if err != nil {
		return authz.Principal{}, authorizationRepositoryError("load default principal", err)
	}
	return principalFromRow(row)
}

func (store *Store) CreatePrincipal(ctx context.Context, principal authz.Principal) (authz.Principal, error) {
	if err := principal.Validate(); err != nil {
		return authz.Principal{}, err
	}
	workspaceID, err := uuidValue(principal.WorkspaceID)
	if err != nil {
		return authz.Principal{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	principalID, err := uuidValue(principal.ID)
	if err != nil {
		return authz.Principal{}, fmt.Errorf("encode principal ID: %w", err)
	}
	ownerID := pgtype.UUID{}
	if principal.OwnerPrincipalID != nil {
		owner, ownerErr := uuidValue(*principal.OwnerPrincipalID)
		if ownerErr != nil {
			return authz.Principal{}, fmt.Errorf("encode owner principal ID: %w", ownerErr)
		}
		ownerRow, ownerErr := store.queries.GetWorkspacePrincipal(ctx, dbgen.GetWorkspacePrincipalParams{
			WorkspaceID: workspaceID, PrincipalID: owner,
		})
		if ownerErr != nil {
			return authz.Principal{}, authorizationRepositoryError("load principal owner", ownerErr)
		}
		if ownerRow.Kind != string(authz.PrincipalHuman) || ownerRow.Status != string(authz.PrincipalActive) {
			return authz.Principal{}, fmt.Errorf("%w: agent owner must be an active human principal", authz.ErrInvariant)
		}
		ownerID = owner
	}
	row, err := store.queries.CreatePrincipal(ctx, dbgen.CreatePrincipalParams{
		ID: principalID, WorkspaceID: workspaceID, Kind: string(principal.Kind),
		DisplayName: principal.DisplayName, OwnerPrincipalID: ownerID, Status: string(principal.Status),
	})
	if err != nil {
		return authz.Principal{}, authorizationRepositoryError("create principal", err)
	}
	return principalFromRow(row)
}

func (store *Store) CreateRoleBinding(ctx context.Context, binding authz.RoleBinding) (authz.RoleBinding, error) {
	if binding.ID.IsZero() || binding.PrincipalID.IsZero() || binding.RoleID == "" ||
		!binding.ScopeType.Valid() || binding.ScopeID == "" {
		return authz.RoleBinding{}, authz.ErrInvalidArgument
	}
	bindingID, err := uuidValue(binding.ID)
	if err != nil {
		return authz.RoleBinding{}, fmt.Errorf("encode binding ID: %w", err)
	}
	principalID, err := uuidValue(binding.PrincipalID)
	if err != nil {
		return authz.RoleBinding{}, fmt.Errorf("encode principal ID: %w", err)
	}
	grantedBy := pgtype.UUID{}
	if binding.GrantedBy != nil {
		value, encodeErr := uuidValue(*binding.GrantedBy)
		if encodeErr != nil {
			return authz.RoleBinding{}, fmt.Errorf("encode granted-by principal ID: %w", encodeErr)
		}
		grantedBy = value
	}
	row, err := store.queries.CreateRoleBinding(ctx, dbgen.CreateRoleBindingParams{
		ID: bindingID, PrincipalID: principalID, RoleID: binding.RoleID,
		ScopeType: string(binding.ScopeType), ScopeID: binding.ScopeID, GrantedBy: grantedBy,
	})
	if err != nil {
		return authz.RoleBinding{}, authorizationRepositoryError("create role binding", err)
	}
	return roleBindingFromRow(row, nil)
}

func (store *Store) LoadPrincipalBindings(
	ctx context.Context, principal identity.PrincipalID,
) ([]authz.RoleBinding, error) {
	principalID, err := uuidValue(principal)
	if err != nil {
		return nil, fmt.Errorf("encode principal ID: %w", err)
	}
	rows, err := store.queries.ListPrincipalRoleBindings(ctx, principalID)
	if err != nil {
		return nil, authorizationRepositoryError("list principal role bindings", err)
	}
	roleIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if len(roleIDs) == 0 || roleIDs[len(roleIDs)-1] != row.RoleID {
			roleIDs = append(roleIDs, row.RoleID)
		}
	}
	actions := map[string][]authz.Action{}
	if len(roleIDs) > 0 {
		actionRows, listErr := store.queries.ListRoleActionsForRoles(ctx, roleIDs)
		if listErr != nil {
			return nil, authorizationRepositoryError("list role actions", listErr)
		}
		for _, actionRow := range actionRows {
			actions[actionRow.RoleID] = append(actions[actionRow.RoleID], authz.Action(actionRow.Action))
		}
	}
	bindings := make([]authz.RoleBinding, 0, len(rows))
	for _, row := range rows {
		binding, decodeErr := roleBindingFromRow(dbgen.RoleBinding{
			ID: row.ID, PrincipalID: row.PrincipalID, RoleID: row.RoleID, ScopeType: row.ScopeType,
			ScopeID: row.ScopeID, GrantedBy: row.GrantedBy, GrantedAt: row.GrantedAt,
		}, actions[row.RoleID])
		if decodeErr != nil {
			return nil, decodeErr
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}

func (store *Store) AuthorizationVersion(
	ctx context.Context, workspace identity.WorkspaceID,
) (int64, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return 0, fmt.Errorf("encode workspace ID: %w", err)
	}
	version, err := store.queries.GetWorkspaceAuthorizationVersion(ctx, workspaceID)
	if err != nil {
		return 0, authorizationRepositoryError("load authorization version", err)
	}
	return version, nil
}

func (store *Store) BumpAuthorizationVersion(
	ctx context.Context, workspace identity.WorkspaceID,
) (int64, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return 0, fmt.Errorf("encode workspace ID: %w", err)
	}
	version, err := store.queries.BumpWorkspaceAuthorizationVersion(ctx, workspaceID)
	if err != nil {
		return 0, authorizationRepositoryError("bump authorization version", err)
	}
	return version, nil
}

func (store *Store) RecordDecision(ctx context.Context, event authz.DecisionEvent) error {
	eventID, err := uuidValue(event.ID)
	if err != nil {
		return fmt.Errorf("encode event ID: %w", err)
	}
	workspaceID, err := uuidValue(event.WorkspaceID)
	if err != nil {
		return fmt.Errorf("encode workspace ID: %w", err)
	}
	principalID := pgtype.UUID{}
	if event.PrincipalID != nil {
		value, encodeErr := uuidValue(*event.PrincipalID)
		if encodeErr != nil {
			return fmt.Errorf("encode principal ID: %w", encodeErr)
		}
		principalID = value
	}
	if !event.ResourceType.Valid() || event.Actor == "" || event.Decision == "" || !event.ReasonCode.Valid() {
		return authz.ErrInvalidArgument
	}
	if err := store.queries.CreateAuthorizationEvent(ctx, dbgen.CreateAuthorizationEventParams{
		ID: eventID, WorkspaceID: workspaceID, PrincipalID: principalID, Actor: event.Actor,
		Action: string(event.Action), ResourceType: string(event.ResourceType), ResourceID: event.ResourceID,
		Decision: event.Decision, ReasonCode: string(event.ReasonCode),
		AuthorizationVersion: event.AuthorizationVersion, TraceID: event.TraceID, CreatedAt: timestamp(event.CreatedAt),
	}); err != nil {
		return authorizationRepositoryError("record authorization decision", err)
	}
	return nil
}

// DeleteRole refuses every deletion: the nine FR-004 system roles are
// non-deletable and non-editable (FR-004). The database trigger rejects direct
// SQL deletions as defense in depth.
func (store *Store) DeleteRole(ctx context.Context, roleID string) error {
	if roleID == "" {
		return authz.ErrInvalidArgument
	}
	return authz.ErrRolesImmutable
}

func principalFromRow(row dbgen.Principal) (authz.Principal, error) {
	id, err := identity.PrincipalIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return authz.Principal{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return authz.Principal{}, err
	}
	var owner *identity.PrincipalID
	if row.OwnerPrincipalID.Valid {
		value, decodeErr := identity.PrincipalIDFromUUIDBytes(row.OwnerPrincipalID.Bytes)
		if decodeErr != nil {
			return authz.Principal{}, decodeErr
		}
		owner = &value
	}
	return authz.Principal{
		ID: id, WorkspaceID: workspaceID, Kind: authz.PrincipalKind(row.Kind),
		DisplayName: row.DisplayName, OwnerPrincipalID: owner,
		Status: authz.PrincipalStatus(row.Status), CreatedAt: row.CreatedAt.Time,
	}, nil
}

func roleBindingFromRow(row dbgen.RoleBinding, actions []authz.Action) (authz.RoleBinding, error) {
	id, err := identity.BindingIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return authz.RoleBinding{}, err
	}
	principalID, err := identity.PrincipalIDFromUUIDBytes(row.PrincipalID.Bytes)
	if err != nil {
		return authz.RoleBinding{}, err
	}
	var grantedBy *identity.PrincipalID
	if row.GrantedBy.Valid {
		value, decodeErr := identity.PrincipalIDFromUUIDBytes(row.GrantedBy.Bytes)
		if decodeErr != nil {
			return authz.RoleBinding{}, decodeErr
		}
		grantedBy = &value
	}
	if actions == nil {
		actions = []authz.Action{}
	}
	return authz.RoleBinding{
		ID: id, PrincipalID: principalID, RoleID: row.RoleID, ScopeType: authz.ScopeType(row.ScopeType),
		ScopeID: row.ScopeID, GrantedBy: grantedBy, GrantedAt: row.GrantedAt.Time, Actions: actions,
	}, nil
}

func authorizationRepositoryError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, authz.ErrConflict)
		case "23503", "23514", "23502", "55000":
			return fmt.Errorf("%s: %w", operation, authz.ErrInvariant)
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, authz.ErrNotFound)
	}
	return fmt.Errorf("%s: %w", operation, errRepositoryOperation)
}
