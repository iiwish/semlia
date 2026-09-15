package postgres

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	authzapp "github.com/iiwish/semlia/internal/application/authorization"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ authzapp.Repository = (*Store)(nil)
var _ authzapp.AdminRepository = (*Store)(nil)
var _ authzapp.BindingExpiryRepository = (*Store)(nil)

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

func (store *Store) LoadLocalUATPrincipal(
	ctx context.Context, workspace identity.WorkspaceID, seedNamespace, roleID string,
) (authz.Principal, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return authz.Principal{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	row, err := store.queries.GetLocalUATPrincipal(ctx, dbgen.GetLocalUATPrincipalParams{
		SeedNamespace: seedNamespace, WorkspaceID: workspaceID, RoleID: roleID,
	})
	if err != nil {
		return authz.Principal{}, authorizationRepositoryError("load local UAT principal", err)
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
	principal, err := store.queries.GetPrincipalByID(ctx, principalID)
	if err != nil {
		return authz.RoleBinding{}, authorizationRepositoryError("load binding principal", err)
	}
	workspaceID := principal.WorkspaceID
	workspace, err := identity.WorkspaceIDFromUUIDBytes(workspaceID.Bytes)
	if err != nil {
		return authz.RoleBinding{}, err
	}
	role, err := store.GetAuthorizationRole(ctx, workspace, binding.RoleID)
	if err != nil {
		return authz.RoleBinding{}, err
	}
	grantedBy := pgtype.UUID{}
	if binding.GrantedBy != nil {
		value, encodeErr := uuidValue(*binding.GrantedBy)
		if encodeErr != nil {
			return authz.RoleBinding{}, fmt.Errorf("encode granted-by principal ID: %w", encodeErr)
		}
		grantedBy = value
	}
	grantedAt := binding.GrantedAt
	if grantedAt.IsZero() {
		grantedAt = time.Now().UTC()
	}
	row, err := store.queries.CreateRoleBinding(ctx, dbgen.CreateRoleBindingParams{
		ID: bindingID, WorkspaceID: workspaceID, PrincipalID: principalID, RoleID: binding.RoleID,
		RoleVersion: role.Version, ScopeType: string(binding.ScopeType), ScopeID: binding.ScopeID,
		GrantedBy: grantedBy, GrantedAt: timestamp(grantedAt), ExpiresAt: authorizationOptionalTimestamp(binding.ExpiresAt),
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
	actions := map[[16]byte][]authz.Action{}
	if len(rows) > 0 {
		actionRows, listErr := store.queries.ListPrincipalRoleBindingActions(ctx, principalID)
		if listErr != nil {
			return nil, authorizationRepositoryError("list binding role actions", listErr)
		}
		for _, actionRow := range actionRows {
			actions[actionRow.BindingID.Bytes] = append(actions[actionRow.BindingID.Bytes], authz.Action(actionRow.Action))
		}
	}
	bindings := make([]authz.RoleBinding, 0, len(rows))
	for _, row := range rows {
		binding, decodeErr := roleBindingFromRow(row, actions[row.ID.Bytes])
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

func (store *Store) ListAuthorizationRoles(ctx context.Context, workspace identity.WorkspaceID) ([]authz.Role, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	rows, err := store.queries.ListAuthorizationRoles(ctx, workspaceID)
	if err != nil {
		return nil, authorizationRepositoryError("list authorization roles", err)
	}
	roleIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		roleIDs = append(roleIDs, row.ID)
	}
	actions, err := store.roleActions(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	roles := make([]authz.Role, 0, len(rows))
	for _, row := range rows {
		role, decodeErr := authorizationRole(row.ID, row.WorkspaceID, row.Name, row.Description, row.Category, row.Version, row.CreatedAt, actions[row.ID])
		if decodeErr != nil {
			return nil, decodeErr
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func (store *Store) GetAuthorizationRole(ctx context.Context, workspace identity.WorkspaceID, roleID string) (authz.Role, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return authz.Role{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	row, err := store.queries.GetAuthorizationRole(ctx, dbgen.GetAuthorizationRoleParams{RoleID: roleID, WorkspaceID: workspaceID})
	if err != nil {
		return authz.Role{}, authorizationRepositoryError("load authorization role", err)
	}
	actions, err := store.roleActions(ctx, []string{row.ID})
	if err != nil {
		return authz.Role{}, err
	}
	return authorizationRole(row.ID, row.WorkspaceID, row.Name, row.Description, row.Category, row.Version, row.CreatedAt, actions[row.ID])
}

func (store *Store) CreateCustomRole(ctx context.Context, mutation authzapp.CustomRoleMutation) (authz.Role, int64, error) {
	workspaceID, actorID, versionID, auditID, err := encodeCustomRoleMutation(mutation)
	if err != nil {
		return authz.Role{}, 0, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return authz.Role{}, 0, fmt.Errorf("begin create custom role: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	lockedVersion, err := queries.LockAuthorizationWorkspace(ctx, workspaceID)
	if err != nil {
		return authz.Role{}, 0, authorizationRepositoryError("lock authorization workspace", err)
	}
	if lockedVersion != mutation.ExpectedAuthorizationVersion {
		return authz.Role{}, 0, authz.ErrVersionConflict
	}
	if err := queries.CreateCustomRoleBase(ctx, dbgen.CreateCustomRoleBaseParams{
		RoleID: mutation.RoleID, WorkspaceID: workspaceID, Name: mutation.Name, Description: mutation.Description,
	}); err != nil {
		return authz.Role{}, 0, authorizationRepositoryError("create custom role", err)
	}
	if err := createCustomRoleVersion(ctx, queries, mutation, workspaceID, actorID, versionID); err != nil {
		return authz.Role{}, 0, err
	}
	authorizationVersion, err := queries.BumpWorkspaceAuthorizationVersion(ctx, workspaceID)
	if err != nil {
		return authz.Role{}, 0, authorizationRepositoryError("bump authorization version", err)
	}
	if err := createAuthorizationMutationAudit(ctx, queries, auditID, workspaceID, mutation.Audit,
		"authorization.custom_role.created", map[string]any{"roleId": mutation.RoleID, "version": mutation.NextVersion, "actions": mutation.Actions}); err != nil {
		return authz.Role{}, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return authz.Role{}, 0, fmt.Errorf("commit create custom role: %w", err)
	}
	return authz.Role{WorkspaceID: mutation.WorkspaceID, ID: mutation.RoleID, Name: mutation.Name,
		Description: mutation.Description, Category: "custom", Actions: append([]authz.Action(nil), mutation.Actions...),
		Version: mutation.NextVersion, CreatedAt: mutation.Audit.OccurredAt}, authorizationVersion, nil
}

func (store *Store) UpdateCustomRole(ctx context.Context, mutation authzapp.CustomRoleMutation) (authz.Role, int64, error) {
	workspaceID, actorID, versionID, auditID, err := encodeCustomRoleMutation(mutation)
	if err != nil {
		return authz.Role{}, 0, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return authz.Role{}, 0, fmt.Errorf("begin update custom role: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	lockedVersion, err := queries.LockAuthorizationWorkspace(ctx, workspaceID)
	if err != nil {
		return authz.Role{}, 0, authorizationRepositoryError("lock authorization workspace", err)
	}
	if lockedVersion != mutation.ExpectedAuthorizationVersion {
		return authz.Role{}, 0, authz.ErrVersionConflict
	}
	if err := validateCustomRoleBindingTransition(ctx, queries, mutation, workspaceID, actorID); err != nil {
		return authz.Role{}, 0, err
	}
	_, err = queries.AdvanceCustomRoleVersion(ctx, dbgen.AdvanceCustomRoleVersionParams{
		Name: mutation.Name, Description: mutation.Description,
		NextVersion: pgtype.Int8{Int64: mutation.NextVersion, Valid: true}, WorkspaceID: workspaceID,
		RoleID: mutation.RoleID, ExpectedVersion: pgtype.Int8{Int64: mutation.ExpectedVersion, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return authz.Role{}, 0, authz.ErrVersionConflict
	}
	if err != nil {
		return authz.Role{}, 0, authorizationRepositoryError("advance custom role", err)
	}
	if err := createCustomRoleVersion(ctx, queries, mutation, workspaceID, actorID, versionID); err != nil {
		return authz.Role{}, 0, err
	}
	if _, err := queries.AdvanceActiveCustomRoleBindings(ctx, dbgen.AdvanceActiveCustomRoleBindingsParams{
		NextVersion: mutation.NextVersion, WorkspaceID: workspaceID, RoleID: mutation.RoleID,
		ExpectedVersion: mutation.ExpectedVersion, Now: timestamp(mutation.Audit.OccurredAt),
	}); err != nil {
		return authz.Role{}, 0, authorizationRepositoryError("advance active custom role bindings", err)
	}
	authorizationVersion, err := queries.BumpWorkspaceAuthorizationVersion(ctx, workspaceID)
	if err != nil {
		return authz.Role{}, 0, authorizationRepositoryError("bump authorization version", err)
	}
	if err := createAuthorizationMutationAudit(ctx, queries, auditID, workspaceID, mutation.Audit,
		"authorization.custom_role.updated", map[string]any{"roleId": mutation.RoleID, "version": mutation.NextVersion, "actions": mutation.Actions}); err != nil {
		return authz.Role{}, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return authz.Role{}, 0, fmt.Errorf("commit update custom role: %w", err)
	}
	return authz.Role{WorkspaceID: mutation.WorkspaceID, ID: mutation.RoleID, Name: mutation.Name,
		Description: mutation.Description, Category: "custom", Actions: append([]authz.Action(nil), mutation.Actions...),
		Version: mutation.NextVersion, CreatedAt: mutation.Audit.OccurredAt}, authorizationVersion, nil
}

func validateCustomRoleBindingTransition(ctx context.Context, queries *dbgen.Queries, mutation authzapp.CustomRoleMutation, workspaceID, actorID pgtype.UUID) error {
	affectedRows, err := queries.ListActiveRoleBindingsForRoleAt(ctx, dbgen.ListActiveRoleBindingsForRoleAtParams{
		WorkspaceID: workspaceID, RoleID: mutation.RoleID, RoleVersion: mutation.ExpectedVersion,
		Now: timestamp(mutation.Audit.OccurredAt),
	})
	if err != nil {
		return authorizationRepositoryError("load affected custom role bindings", err)
	}
	if len(affectedRows) == 0 {
		return nil
	}
	actorBindings, err := loadTransactionPrincipalBindings(ctx, queries, actorID, mutation.Audit.OccurredAt)
	if err != nil {
		return err
	}
	workspaceScope := mutation.WorkspaceID.UUID()
	principalBindings := make(map[[16]byte][]authz.RoleBinding)
	for _, row := range affectedRows {
		affected, decodeErr := roleBindingFromRow(row, mutation.Actions)
		if decodeErr != nil {
			return decodeErr
		}
		principal, loadErr := queries.GetWorkspacePrincipal(ctx, dbgen.GetWorkspacePrincipalParams{
			WorkspaceID: workspaceID, PrincipalID: row.PrincipalID,
		})
		if loadErr != nil {
			return authorizationRepositoryError("load affected binding principal", loadErr)
		}
		if principal.Kind != string(authz.PrincipalHuman) {
			for _, action := range mutation.Actions {
				if authz.RequiresHuman(action) {
					return authz.ErrSeparationOfDuties
				}
			}
		}
		for _, action := range mutation.Actions {
			if !principalBindingGrantsAtScope(actorBindings, action, affected, workspaceScope) {
				return authz.ErrAuthorizationCeiling
			}
		}
		existing, ok := principalBindings[row.PrincipalID.Bytes]
		if !ok {
			existing, err = loadTransactionPrincipalBindings(ctx, queries, row.PrincipalID, mutation.Audit.OccurredAt)
			if err != nil {
				return err
			}
			principalBindings[row.PrincipalID.Bytes] = existing
		}
		for _, other := range existing {
			if other.ID == affected.ID || !authz.BindingsOverlap(other, affected, workspaceScope) {
				continue
			}
			if authz.ActionsConflict(other.Actions, mutation.Actions) {
				return authz.ErrSeparationOfDuties
			}
		}
	}
	return nil
}

func loadTransactionPrincipalBindings(ctx context.Context, queries *dbgen.Queries, principalID pgtype.UUID, now time.Time) ([]authz.RoleBinding, error) {
	rows, err := queries.ListActivePrincipalRoleBindingsAt(ctx, dbgen.ListActivePrincipalRoleBindingsAtParams{
		PrincipalID: principalID, Now: timestamp(now),
	})
	if err != nil {
		return nil, authorizationRepositoryError("load active principal bindings", err)
	}
	actions := make(map[[16]byte][]authz.Action)
	if len(rows) > 0 {
		actionRows, actionErr := queries.ListPrincipalRoleBindingActions(ctx, principalID)
		if actionErr != nil {
			return nil, authorizationRepositoryError("load active principal binding actions", actionErr)
		}
		for _, row := range actionRows {
			actions[row.BindingID.Bytes] = append(actions[row.BindingID.Bytes], authz.Action(row.Action))
		}
	}
	result := make([]authz.RoleBinding, 0, len(rows))
	for _, row := range rows {
		binding, decodeErr := roleBindingFromRow(row, actions[row.ID.Bytes])
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, binding)
	}
	return result, nil
}

func principalBindingGrantsAtScope(bindings []authz.RoleBinding, action authz.Action, target authz.RoleBinding, workspaceID string) bool {
	for _, binding := range bindings {
		if binding.ScopeType != authz.ScopeWorkspace || binding.ScopeID != workspaceID {
			if binding.ScopeType != target.ScopeType || binding.ScopeID != target.ScopeID {
				continue
			}
		}
		for _, granted := range binding.Actions {
			if granted == action {
				return true
			}
		}
	}
	return false
}

func (store *Store) ListAuthorizationRoleBindings(ctx context.Context, workspace identity.WorkspaceID) ([]authz.RoleBinding, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	rows, err := store.queries.ListAuthorizationRoleBindings(ctx, workspaceID)
	if err != nil {
		return nil, authorizationRepositoryError("list authorization role bindings", err)
	}
	actions := map[[16]byte][]authz.Action{}
	if len(rows) > 0 {
		actionRows, listErr := store.queries.ListWorkspaceRoleBindingActions(ctx, workspaceID)
		if listErr != nil {
			return nil, authorizationRepositoryError("list binding role actions", listErr)
		}
		for _, actionRow := range actionRows {
			actions[actionRow.BindingID.Bytes] = append(actions[actionRow.BindingID.Bytes], authz.Action(actionRow.Action))
		}
	}
	bindings := make([]authz.RoleBinding, 0, len(rows))
	for _, row := range rows {
		binding, decodeErr := roleBindingFromRow(dbgen.RoleBinding{
			ID: row.ID, PrincipalID: row.PrincipalID, RoleID: row.RoleID, ScopeType: row.ScopeType,
			ScopeID: row.ScopeID, GrantedBy: row.GrantedBy, GrantedAt: row.GrantedAt,
			WorkspaceID: row.WorkspaceID, RoleVersion: row.RoleVersion, ExpiresAt: row.ExpiresAt,
			ExpiredAt: row.ExpiredAt, RevokedAt: row.RevokedAt, RevokedBy: row.RevokedBy,
			RevocationReason: row.RevocationReason, Version: row.Version,
		}, actions[row.ID.Bytes])
		if decodeErr != nil {
			return nil, decodeErr
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}

func (store *Store) CreateManagedRoleBinding(ctx context.Context, mutation authzapp.RoleBindingMutation) (authz.RoleBinding, int64, error) {
	binding := mutation.Binding
	if err := binding.Validate(mutation.Audit.OccurredAt); err != nil {
		return authz.RoleBinding{}, 0, err
	}
	workspaceID, err := uuidValue(binding.WorkspaceID)
	if err != nil {
		return authz.RoleBinding{}, 0, err
	}
	bindingID, err := uuidValue(binding.ID)
	if err != nil {
		return authz.RoleBinding{}, 0, err
	}
	principalID, err := uuidValue(binding.PrincipalID)
	if err != nil {
		return authz.RoleBinding{}, 0, err
	}
	grantorID, err := uuidValue(*binding.GrantedBy)
	if err != nil {
		return authz.RoleBinding{}, 0, err
	}
	auditID, err := uuidValue(mutation.Audit.EventID)
	if err != nil {
		return authz.RoleBinding{}, 0, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return authz.RoleBinding{}, 0, fmt.Errorf("begin create role binding: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	lockedVersion, err := queries.LockAuthorizationWorkspace(ctx, workspaceID)
	if err != nil {
		return authz.RoleBinding{}, 0, authorizationRepositoryError("lock authorization workspace", err)
	}
	if lockedVersion != mutation.ExpectedAuthorizationVersion {
		return authz.RoleBinding{}, 0, authz.ErrVersionConflict
	}
	if _, err := queries.ExpireAuthorizationRoleBindings(ctx, dbgen.ExpireAuthorizationRoleBindingsParams{
		WorkspaceID: workspaceID, ExpiredAt: timestamp(mutation.Audit.OccurredAt),
	}); err != nil {
		return authz.RoleBinding{}, 0, authorizationRepositoryError("materialize expired role bindings", err)
	}
	currentRole, err := queries.GetAuthorizationRole(ctx, dbgen.GetAuthorizationRoleParams{
		RoleID: binding.RoleID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return authz.RoleBinding{}, 0, authorizationRepositoryError("load authorization role", err)
	}
	if currentRole.Version != binding.RoleVersion {
		return authz.RoleBinding{}, 0, authz.ErrVersionConflict
	}
	row, err := queries.CreateRoleBinding(ctx, dbgen.CreateRoleBindingParams{
		ID: bindingID, WorkspaceID: workspaceID, PrincipalID: principalID,
		RoleID: binding.RoleID, RoleVersion: binding.RoleVersion, ScopeType: string(binding.ScopeType),
		ScopeID: binding.ScopeID, GrantedBy: grantorID, GrantedAt: timestamp(binding.GrantedAt),
		ExpiresAt: authorizationOptionalTimestamp(binding.ExpiresAt),
	})
	if err != nil {
		return authz.RoleBinding{}, 0, authorizationRepositoryError("create managed role binding", err)
	}
	authorizationVersion, err := queries.BumpWorkspaceAuthorizationVersion(ctx, workspaceID)
	if err != nil {
		return authz.RoleBinding{}, 0, authorizationRepositoryError("bump authorization version", err)
	}
	if err := createAuthorizationMutationAudit(ctx, queries, auditID, workspaceID, mutation.Audit,
		"authorization.role_binding.created", map[string]any{"bindingId": binding.ID.String(), "principalId": binding.PrincipalID.String(),
			"roleId": binding.RoleID, "roleVersion": binding.RoleVersion, "scopeType": binding.ScopeType, "scopeId": binding.ScopeID}); err != nil {
		return authz.RoleBinding{}, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return authz.RoleBinding{}, 0, fmt.Errorf("commit create role binding: %w", err)
	}
	created, err := roleBindingFromRow(row, binding.Actions)
	return created, authorizationVersion, err
}

func (store *Store) RevokeManagedRoleBinding(ctx context.Context, revocation authzapp.RoleBindingRevocation) (authz.RoleBinding, int64, error) {
	workspaceID, err := uuidValue(revocation.WorkspaceID)
	if err != nil {
		return authz.RoleBinding{}, 0, err
	}
	bindingID, err := uuidValue(revocation.BindingID)
	if err != nil {
		return authz.RoleBinding{}, 0, err
	}
	revokerID, err := uuidValue(revocation.Audit.ActorID)
	if err != nil {
		return authz.RoleBinding{}, 0, err
	}
	auditID, err := uuidValue(revocation.Audit.EventID)
	if err != nil {
		return authz.RoleBinding{}, 0, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return authz.RoleBinding{}, 0, fmt.Errorf("begin revoke role binding: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	lockedVersion, err := queries.LockAuthorizationWorkspace(ctx, workspaceID)
	if err != nil {
		return authz.RoleBinding{}, 0, authorizationRepositoryError("lock authorization workspace", err)
	}
	if lockedVersion != revocation.ExpectedAuthorizationVersion {
		return authz.RoleBinding{}, 0, authz.ErrVersionConflict
	}
	current, err := queries.GetAuthorizationRoleBinding(ctx, dbgen.GetAuthorizationRoleBindingParams{WorkspaceID: workspaceID, BindingID: bindingID})
	if err != nil {
		return authz.RoleBinding{}, 0, authorizationRepositoryError("load role binding", err)
	}
	if current.Version != revocation.ExpectedVersion {
		return authz.RoleBinding{}, 0, authz.ErrVersionConflict
	}
	if current.RevokedAt.Valid {
		return authz.RoleBinding{}, 0, authz.ErrConflict
	}
	if current.ExpiredAt.Valid || current.ExpiresAt.Valid && !revocation.Audit.OccurredAt.Before(current.ExpiresAt.Time) {
		return authz.RoleBinding{}, 0, authz.ErrConflict
	}
	if current.RoleID == "workspace_admin" && current.ScopeType == string(authz.ScopeWorkspace) {
		activeAdministrator, activeErr := queries.IsActiveWorkspaceAdministrator(ctx, dbgen.IsActiveWorkspaceAdministratorParams{
			WorkspaceID: workspaceID, PrincipalID: current.PrincipalID, Now: timestamp(revocation.Audit.OccurredAt),
		})
		if activeErr != nil {
			return authz.RoleBinding{}, 0, authorizationRepositoryError("check active workspace administrator", activeErr)
		}
		count, countErr := queries.CountActiveAuthorizationAdministrators(ctx, dbgen.CountActiveAuthorizationAdministratorsParams{
			WorkspaceID: workspaceID, Now: timestamp(revocation.Audit.OccurredAt),
		})
		if countErr != nil {
			return authz.RoleBinding{}, 0, authorizationRepositoryError("count workspace administrators", countErr)
		}
		if count == 0 || activeAdministrator && count <= 1 {
			return authz.RoleBinding{}, 0, authz.ErrFinalAdministrator
		}
	}
	row, err := queries.RevokeAuthorizationRoleBinding(ctx, dbgen.RevokeAuthorizationRoleBindingParams{
		RevokedAt: timestamp(revocation.Audit.OccurredAt), RevokedBy: revokerID,
		RevocationReason: textValue(revocation.Reason), WorkspaceID: workspaceID,
		BindingID: bindingID, ExpectedVersion: revocation.ExpectedVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return authz.RoleBinding{}, 0, authz.ErrVersionConflict
	}
	if err != nil {
		return authz.RoleBinding{}, 0, authorizationRepositoryError("revoke role binding", err)
	}
	authorizationVersion, err := queries.BumpWorkspaceAuthorizationVersion(ctx, workspaceID)
	if err != nil {
		return authz.RoleBinding{}, 0, authorizationRepositoryError("bump authorization version", err)
	}
	if err := createAuthorizationMutationAudit(ctx, queries, auditID, workspaceID, revocation.Audit,
		"authorization.role_binding.revoked", map[string]any{"bindingId": revocation.BindingID.String(), "reason": revocation.Reason}); err != nil {
		return authz.RoleBinding{}, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return authz.RoleBinding{}, 0, fmt.Errorf("commit revoke role binding: %w", err)
	}
	revoked, err := roleBindingFromRow(row, nil)
	return revoked, authorizationVersion, err
}

func (store *Store) AuthorizationScopeExists(
	ctx context.Context,
	workspace identity.WorkspaceID,
	resource authz.Resource,
) (bool, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return false, err
	}
	exists, err := store.queries.AuthorizationScopeExists(ctx, dbgen.AuthorizationScopeExistsParams{
		ScopeType: string(resource.Type), ScopeID: resource.ID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return false, authorizationRepositoryError("validate authorization scope", err)
	}
	return exists, nil
}

func (store *Store) RecordPolicyDenial(ctx context.Context, denial authzapp.PolicyDenial) error {
	workspaceID, err := uuidValue(denial.WorkspaceID)
	if err != nil {
		return err
	}
	eventID, err := identity.NewEventID()
	if err != nil {
		return err
	}
	eventDBID, err := uuidValue(eventID)
	if err != nil {
		return err
	}
	traceID := denial.TraceID
	decodedTrace, traceErr := hex.DecodeString(traceID)
	if traceErr != nil || len(decodedTrace) != 16 {
		encoded := fmt.Sprintf("%x", eventDBID.Bytes)
		traceID = encoded[:32]
	}
	payload := map[string]any{"reason": denial.Reason, "action": denial.Action}
	if denial.RoleID != "" {
		payload["roleId"] = denial.RoleID
	}
	if !denial.PrincipalID.IsZero() {
		payload["principalId"] = denial.PrincipalID.String()
	}
	if !denial.BindingID.IsZero() {
		payload["bindingId"] = denial.BindingID.String()
	}
	audit := authzapp.MutationAudit{EventID: eventID, ActorID: denial.ActorID, TraceID: traceID, OccurredAt: denial.OccurredAt}
	return createAuthorizationMutationAudit(ctx, store.queries, eventDBID, workspaceID, audit, "authorization.policy_denied", payload)
}

func (store *Store) ExpireRoleBindings(ctx context.Context, request authzapp.ExpireRoleBindingsRequest) error {
	workspaceID, err := uuidValue(request.WorkspaceID)
	if err != nil {
		return err
	}
	eventID, err := uuidValue(request.EventID)
	if err != nil {
		return err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin expire role bindings: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	if _, err := queries.LockAuthorizationWorkspace(ctx, workspaceID); err != nil {
		return authorizationRepositoryError("lock authorization workspace", err)
	}
	ids, err := queries.ExpireAuthorizationRoleBindings(ctx, dbgen.ExpireAuthorizationRoleBindingsParams{
		WorkspaceID: workspaceID, ExpiredAt: timestamp(request.Now),
	})
	if err != nil {
		return authorizationRepositoryError("expire role bindings", err)
	}
	if len(ids) == 0 {
		return nil
	}
	authorizationVersion, err := queries.BumpWorkspaceAuthorizationVersion(ctx, workspaceID)
	if err != nil {
		return authorizationRepositoryError("bump authorization version", err)
	}
	audit := authzapp.MutationAudit{EventID: request.EventID, TraceID: request.TraceID, OccurredAt: request.Now}
	if err := createAuthorizationMutationAudit(ctx, queries, eventID, workspaceID, audit,
		"authorization.role_bindings.expired", map[string]any{"count": len(ids), "authorizationVersion": authorizationVersion}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit expire role bindings: %w", err)
	}
	return nil
}

func (store *Store) roleActions(ctx context.Context, roleIDs []string) (map[string][]authz.Action, error) {
	result := make(map[string][]authz.Action, len(roleIDs))
	if len(roleIDs) == 0 {
		return result, nil
	}
	rows, err := store.queries.ListRoleActionsForRoles(ctx, roleIDs)
	if err != nil {
		return nil, authorizationRepositoryError("list authorization role actions", err)
	}
	for _, row := range rows {
		result[row.RoleID] = append(result[row.RoleID], authz.Action(row.Action))
	}
	return result, nil
}

func authorizationRole(id string, workspaceID pgtype.UUID, name, description, category string, version int64,
	createdAt pgtype.Timestamptz, actions []authz.Action) (authz.Role, error) {
	role := authz.Role{ID: id, Name: name, Description: description, Category: category,
		Version: version, Actions: append([]authz.Action(nil), actions...)}
	if workspaceID.Valid {
		workspace, err := identity.WorkspaceIDFromUUIDBytes(workspaceID.Bytes)
		if err != nil {
			return authz.Role{}, err
		}
		role.WorkspaceID = workspace
	}
	if createdAt.Valid {
		role.CreatedAt = createdAt.Time
	}
	return role, role.Validate()
}

func encodeCustomRoleMutation(mutation authzapp.CustomRoleMutation) (pgtype.UUID, pgtype.UUID, pgtype.UUID, pgtype.UUID, error) {
	workspaceID, err := uuidValue(mutation.WorkspaceID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	actorID, err := uuidValue(mutation.Audit.ActorID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	versionID, err := uuidValue(mutation.VersionEventID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	auditID, err := uuidValue(mutation.Audit.EventID)
	return workspaceID, actorID, versionID, auditID, err
}

func createCustomRoleVersion(ctx context.Context, queries *dbgen.Queries, mutation authzapp.CustomRoleMutation,
	workspaceID, actorID, versionID pgtype.UUID) error {
	if err := queries.CreateCustomRoleVersion(ctx, dbgen.CreateCustomRoleVersionParams{
		ID: versionID, WorkspaceID: workspaceID, RoleID: mutation.RoleID, Version: mutation.NextVersion,
		Name: mutation.Name, Description: mutation.Description, CreatedBy: actorID,
		CreatedAt: timestamp(mutation.Audit.OccurredAt),
	}); err != nil {
		return authorizationRepositoryError("create custom role version", err)
	}
	for _, action := range mutation.Actions {
		if err := queries.CreateCustomRoleVersionAction(ctx, dbgen.CreateCustomRoleVersionActionParams{
			WorkspaceID: workspaceID, RoleID: mutation.RoleID, RoleVersion: mutation.NextVersion, Action: string(action),
		}); err != nil {
			return authorizationRepositoryError("create custom role action", err)
		}
	}
	return nil
}

func createAuthorizationMutationAudit(ctx context.Context, queries *dbgen.Queries, eventID, workspaceID pgtype.UUID,
	audit authzapp.MutationAudit, eventType string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode authorization audit payload: %w", err)
	}
	actor := pgtype.Text{}
	if !audit.ActorID.IsZero() {
		actor = textValue(audit.ActorID.String())
	}
	if err := queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		ID: eventID, WorkspaceID: workspaceID, EventType: eventType, ActorID: actor,
		Payload: encoded, TraceID: audit.TraceID, CreatedAt: timestamp(audit.OccurredAt),
	}); err != nil {
		return authorizationRepositoryError("record authorization mutation audit", err)
	}
	return nil
}

func authorizationOptionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(value.UTC())
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
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
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
	var revokedBy *identity.PrincipalID
	if row.RevokedBy.Valid {
		value, decodeErr := identity.PrincipalIDFromUUIDBytes(row.RevokedBy.Bytes)
		if decodeErr != nil {
			return authz.RoleBinding{}, decodeErr
		}
		revokedBy = &value
	}
	if actions == nil {
		actions = []authz.Action{}
	}
	return authz.RoleBinding{
		WorkspaceID: workspaceID, ID: id, PrincipalID: principalID, RoleID: row.RoleID, RoleVersion: row.RoleVersion,
		ScopeType: authz.ScopeType(row.ScopeType), ScopeID: row.ScopeID, GrantedBy: grantedBy,
		GrantedAt: row.GrantedAt.Time, ExpiresAt: timestampPointer(row.ExpiresAt), ExpiredAt: timestampPointer(row.ExpiredAt),
		RevokedAt: timestampPointer(row.RevokedAt), RevokedBy: revokedBy, RevokeReason: row.RevocationReason.String,
		Version: row.Version, Actions: actions,
	}, nil
}

func timestampPointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
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
