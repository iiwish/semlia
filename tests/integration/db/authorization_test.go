package db_test

import (
	"context"
	"errors"
	"testing"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestAuthorizationVocabularySeedsNineSystemRolesIdempotently(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()

	var actionCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM actions").Scan(&actionCount); err != nil {
		t.Fatal(err)
	}
	if actionCount != 27 {
		t.Fatalf("action vocabulary size = %d, want 27 FR-003 actions", actionCount)
	}
	var roleCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM roles WHERE category = 'system'").Scan(&roleCount); err != nil {
		t.Fatal(err)
	}
	if roleCount != 9 {
		t.Fatalf("system role count = %d, want the nine FR-004 roles", roleCount)
	}
	var humanRequired int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM actions WHERE requires_human").Scan(&humanRequired); err != nil {
		t.Fatal(err)
	}
	if humanRequired == 0 {
		t.Fatal("action vocabulary must mark human-only actions for separation of duties")
	}

	var roleActions int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM role_actions").Scan(&roleActions); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "SELECT seed_authorization_vocabulary()"); err != nil {
		t.Fatalf("re-run vocabulary seeding: %v", err)
	}
	var afterActions, afterRoles, afterRoleActions int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM actions").Scan(&afterActions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM roles").Scan(&afterRoles); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM role_actions").Scan(&afterRoleActions); err != nil {
		t.Fatal(err)
	}
	if afterActions != actionCount || afterRoles != roleCount || afterRoleActions != roleActions {
		t.Fatalf("seeding is not idempotent: actions %d→%d, roles %d→%d, role_actions %d→%d",
			actionCount, afterActions, roleCount, afterRoles, roleActions, afterRoleActions)
	}

	var workspaceAdminActions int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM role_actions WHERE role_id = 'workspace_admin'`).Scan(&workspaceAdminActions); err != nil {
		t.Fatal(err)
	}
	if workspaceAdminActions != 27 {
		t.Fatalf("workspace admin actions = %d, want the full vocabulary", workspaceAdminActions)
	}
}

func TestSystemRolesCannotBeDeleted(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	store := pgstore.NewStore(pool)
	ctx := context.Background()

	if err := store.DeleteRole(ctx, "workspace_admin"); !errors.Is(err, authorization.ErrRolesImmutable) {
		t.Fatalf("repository role deletion error = %v, want ErrRolesImmutable", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM roles WHERE id = 'workspace_admin'"); err == nil {
		t.Fatal("database accepted system role deletion")
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM roles WHERE id = 'workspace_admin'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("workspace_admin survived deletion check = %d", count)
	}
}

func TestWorkspaceBootstrapSeedsDefaultPrincipalAndBinding(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ($1, 'authz-seed', 'Authorization Seed')`, workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}

	var principalID, kind, status string
	var owner *string
	if err := pool.QueryRow(ctx, `
		SELECT principal.id, principal.kind, principal.status, principal.owner_principal_id::text
		FROM principals AS principal
		JOIN role_bindings AS binding ON binding.principal_id = principal.id
		WHERE principal.workspace_id = $1
		  AND binding.role_id = 'workspace_admin'
		  AND binding.scope_type = 'workspace'
		  AND binding.scope_id = $1::text
		  AND binding.granted_by IS NULL`, workspaceID.UUID()).Scan(&principalID, &kind, &status, &owner); err != nil {
		t.Fatalf("seeded workspace-admin principal not found: %v", err)
	}
	if kind != "human" || status != "active" || owner != nil {
		t.Fatalf("seeded default principal = kind %s status %s owner %v", kind, status, owner)
	}
	resolved, err := identity.PrincipalIDFromUUIDBytes(mustUUIDBytes(t, principalID))
	if err != nil {
		t.Fatalf("seeded principal ID is not a UUIDv7 identity: %v", err)
	}
	if resolved.Prefix() != identity.Principal {
		t.Fatalf("principal prefix = %q, want prn", resolved.Prefix())
	}
}

func TestAuthorizationEventsAreImmutable(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	store := pgstore.NewStore(pool)
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ($1, 'authz-events', 'Authorization Events')`, workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	defaultPrincipal, err := store.LoadDefaultPrincipal(ctx, workspaceID)
	if err != nil {
		t.Fatalf("load seeded principal: %v", err)
	}
	eventID := newEventID(t)
	err = store.RecordDecision(ctx, authorization.DecisionEvent{
		ID: eventID, WorkspaceID: workspaceID, PrincipalID: &defaultPrincipal.ID,
		Actor: defaultPrincipal.ID.String(), Action: authorization.ActionAssetPropose,
		ResourceType: authorization.ScopeWorkspace, ResourceID: workspaceID.UUID(),
		Decision: "deny", ReasonCode: authorization.ReasonNoMatchingGrant,
		AuthorizationVersion: 1, TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE authorization_events SET decision = 'allow' WHERE id = $1", eventID.UUID()); err == nil {
		t.Fatal("authorization event update was accepted")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM authorization_events WHERE id = $1", eventID.UUID()); err == nil {
		t.Fatal("authorization event deletion was accepted")
	}
}

func TestAuthorizationVersionIsMonotonicPerWorkspace(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	store := pgstore.NewStore(pool)
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ($1, 'authz-version', 'Authorization Version')`, workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	version, err := store.AuthorizationVersion(ctx, workspaceID)
	if err != nil || version != 1 {
		t.Fatalf("initial authorization version = %d, err = %v", version, err)
	}
	bumped, err := store.BumpAuthorizationVersion(ctx, workspaceID)
	if err != nil || bumped != 2 {
		t.Fatalf("bumped authorization version = %d, err = %v", bumped, err)
	}
	version, err = store.AuthorizationVersion(ctx, workspaceID)
	if err != nil || version != 2 {
		t.Fatalf("authorization version after bump = %d, err = %v", version, err)
	}
}

func mustUUIDBytes(t *testing.T, value string) [16]byte {
	t.Helper()
	var result [16]byte
	if len(value) != 36 {
		t.Fatalf("malformed uuid %q", value)
	}
	nibbles := func(character byte) byte {
		switch {
		case character >= '0' && character <= '9':
			return character - '0'
		case character >= 'a' && character <= 'f':
			return character - 'a' + 10
		}
		t.Fatalf("malformed uuid hex %q", value)
		return 0
	}
	position := 0
	for index := 0; index < len(value); index++ {
		if value[index] == '-' {
			continue
		}
		if position%2 == 0 {
			result[position/2] = nibbles(value[index]) << 4
		} else {
			result[position/2] |= nibbles(value[index])
		}
		position++
	}
	return result
}
