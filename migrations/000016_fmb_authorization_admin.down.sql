BEGIN;

DROP TRIGGER IF EXISTS workspace_invitations_validate_role ON workspace_invitations;
DROP FUNCTION IF EXISTS validate_workspace_invitation_role();
ALTER TABLE workspace_invitations DROP COLUMN IF EXISTS role_version;

DROP TRIGGER IF EXISTS role_bindings_validate_workspace ON role_bindings;
DROP FUNCTION IF EXISTS validate_role_binding_workspace();
DROP TRIGGER IF EXISTS role_bindings_00_populate_context ON role_bindings;
DROP FUNCTION IF EXISTS populate_role_binding_context();

DROP INDEX IF EXISTS role_bindings_workspace_scope_state_idx;
DROP INDEX IF EXISTS role_bindings_workspace_principal_state_idx;
DROP INDEX IF EXISTS role_bindings_active_grant_key;

DELETE FROM role_bindings
WHERE revoked_at IS NOT NULL OR expired_at IS NOT NULL OR expires_at IS NOT NULL;

DELETE FROM role_bindings AS binding
USING roles AS role
WHERE binding.role_id = role.id AND role.category = 'custom';

ALTER TABLE role_bindings
    DROP CONSTRAINT IF EXISTS role_bindings_workspace_scope,
    DROP CONSTRAINT IF EXISTS role_bindings_lifecycle,
    DROP CONSTRAINT IF EXISTS role_bindings_role_version,
    DROP CONSTRAINT IF EXISTS role_bindings_workspace_revoker_fkey,
    DROP CONSTRAINT IF EXISTS role_bindings_workspace_grantor_fkey,
    DROP CONSTRAINT IF EXISTS role_bindings_workspace_principal_fkey,
    DROP CONSTRAINT IF EXISTS role_bindings_workspace_fkey,
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS revocation_reason,
    DROP COLUMN IF EXISTS revoked_by,
    DROP COLUMN IF EXISTS revoked_at,
    DROP COLUMN IF EXISTS expired_at,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS role_version,
    DROP COLUMN IF EXISTS workspace_id;

ALTER TABLE role_bindings
    ADD CONSTRAINT role_bindings_principal_id_role_id_scope_type_scope_id_key
    UNIQUE (principal_id, role_id, scope_type, scope_id);

DROP TRIGGER IF EXISTS roles_validate_active_custom_version ON roles;
DROP FUNCTION IF EXISTS validate_custom_role_active_version();

DROP TRIGGER IF EXISTS system_role_actions_protect_vocabulary ON role_actions;
DROP TRIGGER IF EXISTS roles_protect_vocabulary ON roles;
DROP FUNCTION IF EXISTS protect_role_vocabulary();

DROP TRIGGER IF EXISTS roles_immutable ON roles;
DROP TRIGGER IF EXISTS custom_role_version_actions_immutable ON custom_role_version_actions;
DROP TRIGGER IF EXISTS custom_role_versions_immutable ON custom_role_versions;
DROP TABLE IF EXISTS custom_role_version_actions;
DROP TABLE IF EXISTS custom_role_versions;

DELETE FROM roles WHERE category = 'custom';
CREATE TRIGGER roles_immutable
    BEFORE DELETE ON roles
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

ALTER TABLE roles
    DROP CONSTRAINT IF EXISTS roles_workspace_identity,
    DROP CONSTRAINT IF EXISTS roles_workspace_category,
    DROP COLUMN IF EXISTS active_version,
    DROP COLUMN IF EXISTS workspace_id;

COMMIT;
