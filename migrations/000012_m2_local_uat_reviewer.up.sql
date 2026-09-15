BEGIN;

-- Private-incubation workspaces receive stable independent reviewer and
-- publisher principals. Symbolic browser aliases are accepted only when the
-- server's development-only UAT flag is enabled; persisted facts always
-- record real principal TypeIDs.
CREATE FUNCTION seed_workspace_local_uat_identities()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    admin_principal_id uuid;
    reviewer_principal_id uuid;
    publisher_principal_id uuid;
BEGIN
    admin_principal_id := semlia_seed_uuidv7('principal', NEW.id::text, NEW.created_at);
    reviewer_principal_id := semlia_seed_uuidv7('independent_reviewer', NEW.id::text, NEW.created_at);
    publisher_principal_id := semlia_seed_uuidv7('independent_publisher', NEW.id::text, NEW.created_at);

    INSERT INTO principals (id, workspace_id, kind, display_name, status)
    VALUES (reviewer_principal_id, NEW.id, 'human', 'Independent Reviewer', 'active')
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO principals (id, workspace_id, kind, display_name, status)
    VALUES (publisher_principal_id, NEW.id, 'human', 'Independent Publisher', 'active')
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO role_bindings (id, principal_id, role_id, scope_type, scope_id, granted_by)
    VALUES (
        semlia_seed_uuidv7('independent_reviewer_binding', NEW.id::text, NEW.created_at),
        reviewer_principal_id, 'reviewer', 'workspace', NEW.id::text, admin_principal_id
    )
    ON CONFLICT (principal_id, role_id, scope_type, scope_id) DO NOTHING;

    INSERT INTO role_bindings (id, principal_id, role_id, scope_type, scope_id, granted_by)
    VALUES (
        semlia_seed_uuidv7('independent_publisher_binding', NEW.id::text, NEW.created_at),
        publisher_principal_id, 'publisher', 'workspace', NEW.id::text, admin_principal_id
    )
    ON CONFLICT (principal_id, role_id, scope_type, scope_id) DO NOTHING;

    RETURN NEW;
END;
$$;

CREATE TRIGGER workspaces_seed_local_uat_identities
    AFTER INSERT ON workspaces
    FOR EACH ROW EXECUTE FUNCTION seed_workspace_local_uat_identities();

INSERT INTO principals (id, workspace_id, kind, display_name, status)
SELECT
    semlia_seed_uuidv7('independent_reviewer', workspace.id::text, workspace.created_at),
    workspace.id, 'human', 'Independent Reviewer', 'active'
FROM workspaces AS workspace
ON CONFLICT (id) DO NOTHING;

INSERT INTO principals (id, workspace_id, kind, display_name, status)
SELECT
    semlia_seed_uuidv7('independent_publisher', workspace.id::text, workspace.created_at),
    workspace.id, 'human', 'Independent Publisher', 'active'
FROM workspaces AS workspace
ON CONFLICT (id) DO NOTHING;

INSERT INTO role_bindings (id, principal_id, role_id, scope_type, scope_id, granted_by)
SELECT
    semlia_seed_uuidv7('independent_reviewer_binding', workspace.id::text, workspace.created_at),
    semlia_seed_uuidv7('independent_reviewer', workspace.id::text, workspace.created_at),
    'reviewer', 'workspace', workspace.id::text,
    semlia_seed_uuidv7('principal', workspace.id::text, workspace.created_at)
FROM workspaces AS workspace
ON CONFLICT (principal_id, role_id, scope_type, scope_id) DO NOTHING;

INSERT INTO role_bindings (id, principal_id, role_id, scope_type, scope_id, granted_by)
SELECT
    semlia_seed_uuidv7('independent_publisher_binding', workspace.id::text, workspace.created_at),
    semlia_seed_uuidv7('independent_publisher', workspace.id::text, workspace.created_at),
    'publisher', 'workspace', workspace.id::text,
    semlia_seed_uuidv7('principal', workspace.id::text, workspace.created_at)
FROM workspaces AS workspace
ON CONFLICT (principal_id, role_id, scope_type, scope_id) DO NOTHING;

COMMIT;
