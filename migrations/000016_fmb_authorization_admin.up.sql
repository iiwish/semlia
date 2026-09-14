BEGIN;

ALTER TABLE roles
    ADD COLUMN workspace_id uuid,
    ADD COLUMN active_version bigint,
    ADD CONSTRAINT roles_workspace_category CHECK (
        (category = 'system' AND workspace_id IS NULL AND active_version IS NULL)
        OR (category = 'custom' AND workspace_id IS NOT NULL AND active_version > 0)
    ),
    ADD CONSTRAINT roles_workspace_identity UNIQUE (workspace_id, id);

CREATE TABLE custom_role_versions (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    role_id text NOT NULL,
    version bigint NOT NULL CHECK (version > 0),
    name text NOT NULL CHECK (name <> '' AND length(name) <= 120),
    description text NOT NULL CHECK (description <> '' AND length(description) <= 512),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, role_id, version),
    FOREIGN KEY (workspace_id, role_id)
        REFERENCES roles (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, created_by)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT
);

CREATE TABLE custom_role_version_actions (
    workspace_id uuid NOT NULL,
    role_id text NOT NULL,
    role_version bigint NOT NULL,
    action text NOT NULL REFERENCES actions (action) ON DELETE RESTRICT,
    PRIMARY KEY (workspace_id, role_id, role_version, action),
    FOREIGN KEY (workspace_id, role_id, role_version)
        REFERENCES custom_role_versions (workspace_id, role_id, version) ON DELETE RESTRICT
);

CREATE FUNCTION validate_custom_role_active_version()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.category = 'custom' THEN
        IF NOT EXISTS (SELECT 1 FROM workspaces WHERE id = NEW.workspace_id) THEN
            RAISE EXCEPTION 'custom role workspace does not exist' USING ERRCODE = '23503';
        END IF;
        IF NOT EXISTS (
            SELECT 1 FROM custom_role_versions
            WHERE workspace_id = NEW.workspace_id AND role_id = NEW.id AND version = NEW.active_version
              AND name = NEW.name AND description = NEW.description
        ) THEN
            RAISE EXCEPTION 'custom role active version does not exist' USING ERRCODE = '23503';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE CONSTRAINT TRIGGER roles_validate_active_custom_version
    AFTER INSERT OR UPDATE OF active_version ON roles
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION validate_custom_role_active_version();

CREATE TRIGGER custom_role_versions_immutable
    BEFORE UPDATE OR DELETE ON custom_role_versions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TRIGGER custom_role_version_actions_immutable
    BEFORE UPDATE OR DELETE ON custom_role_version_actions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE FUNCTION protect_role_vocabulary()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    old_category text;
    new_category text;
BEGIN
    IF TG_TABLE_NAME = 'roles' THEN
        IF OLD.category = 'system' THEN
            RAISE EXCEPTION 'system roles are immutable' USING ERRCODE = '55000';
        END IF;
        IF NEW.id IS DISTINCT FROM OLD.id
           OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
           OR NEW.category IS DISTINCT FROM OLD.category THEN
            RAISE EXCEPTION 'custom role identity is immutable' USING ERRCODE = '55000';
        END IF;
        RETURN NEW;
    END IF;

    IF TG_OP <> 'INSERT' THEN
        SELECT category INTO old_category FROM roles WHERE id = OLD.role_id;
    END IF;
    IF TG_OP <> 'DELETE' THEN
        SELECT category INTO new_category FROM roles WHERE id = NEW.role_id;
    END IF;
    IF TG_OP = 'INSERT' AND new_category = 'system' AND EXISTS (
            SELECT 1 FROM role_actions WHERE role_id = NEW.role_id AND action = NEW.action
    ) THEN
        RETURN NEW;
    END IF;
    IF old_category IS NOT NULL OR new_category IS NOT NULL THEN
        RAISE EXCEPTION 'role actions are immutable; custom roles use versioned actions' USING ERRCODE = '55000';
    END IF;
    RAISE EXCEPTION 'role does not exist' USING ERRCODE = '23503';
END;
$$;

CREATE TRIGGER roles_protect_vocabulary
    BEFORE UPDATE ON roles
    FOR EACH ROW EXECUTE FUNCTION protect_role_vocabulary();

CREATE TRIGGER system_role_actions_protect_vocabulary
    BEFORE INSERT OR UPDATE OR DELETE ON role_actions
    FOR EACH ROW EXECUTE FUNCTION protect_role_vocabulary();

ALTER TABLE role_bindings
    ADD COLUMN workspace_id uuid,
    ADD COLUMN role_version bigint NOT NULL DEFAULT 0,
    ADD COLUMN expires_at timestamptz,
    ADD COLUMN expired_at timestamptz,
    ADD COLUMN revoked_at timestamptz,
    ADD COLUMN revoked_by uuid,
    ADD COLUMN revocation_reason text,
    ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);

UPDATE role_bindings AS binding
SET workspace_id = principal.workspace_id,
    role_version = 1
FROM principals AS principal
WHERE principal.id = binding.principal_id;

ALTER TABLE role_bindings
    DROP CONSTRAINT role_bindings_principal_id_role_id_scope_type_scope_id_key;

CREATE UNIQUE INDEX role_bindings_active_grant_key
    ON role_bindings (workspace_id, principal_id, role_id, scope_type, scope_id)
    WHERE revoked_at IS NULL AND expired_at IS NULL;

ALTER TABLE role_bindings
    ALTER COLUMN workspace_id SET NOT NULL,
    ADD CONSTRAINT role_bindings_workspace_fkey
        FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE RESTRICT,
    ADD CONSTRAINT role_bindings_workspace_principal_fkey
        FOREIGN KEY (workspace_id, principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT role_bindings_workspace_grantor_fkey
        FOREIGN KEY (workspace_id, granted_by)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT role_bindings_workspace_revoker_fkey
        FOREIGN KEY (workspace_id, revoked_by)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT role_bindings_role_version CHECK (role_version > 0),
    ADD CONSTRAINT role_bindings_lifecycle CHECK (
        (expires_at IS NULL OR expires_at > granted_at)
        AND (expired_at IS NULL OR (expires_at IS NOT NULL AND expired_at >= expires_at))
        AND (revoked_at IS NULL OR (revoked_by IS NOT NULL AND revoked_at >= granted_at))
        AND NOT (expired_at IS NOT NULL AND revoked_at IS NOT NULL)
        AND (role_id <> 'workspace_admin' OR expires_at IS NULL)
        AND (revocation_reason IS NULL OR (revoked_at IS NOT NULL AND length(revocation_reason) BETWEEN 1 AND 512))
    ),
    ADD CONSTRAINT role_bindings_workspace_scope CHECK (
        scope_type <> 'workspace' OR scope_id = workspace_id::text
    );

CREATE INDEX role_bindings_workspace_principal_state_idx
    ON role_bindings (workspace_id, principal_id, revoked_at, expires_at, id);

CREATE INDEX role_bindings_workspace_scope_state_idx
    ON role_bindings (workspace_id, scope_type, scope_id, revoked_at, expires_at, id);

-- Keep historical seed/invitation inserts compatible while making the stored
-- workspace and role version explicit. Managed writes always supply both.
CREATE FUNCTION populate_role_binding_context()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    principal_workspace uuid;
    role_category text;
    current_role_version bigint;
BEGIN
    IF NEW.workspace_id IS NULL THEN
        SELECT workspace_id INTO principal_workspace FROM principals WHERE id = NEW.principal_id;
        NEW.workspace_id := principal_workspace;
    END IF;
    IF NEW.role_version = 0 THEN
        SELECT category, active_version INTO role_category, current_role_version
        FROM roles WHERE id = NEW.role_id;
        NEW.role_version := CASE WHEN role_category = 'custom' THEN current_role_version ELSE 1 END;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER role_bindings_00_populate_context
    BEFORE INSERT ON role_bindings
    FOR EACH ROW EXECUTE FUNCTION populate_role_binding_context();

CREATE FUNCTION validate_role_binding_workspace()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    bound_category text;
    bound_workspace uuid;
    bound_active_version bigint;
BEGIN
    SELECT category, workspace_id, active_version
    INTO bound_category, bound_workspace, bound_active_version
    FROM roles WHERE id = NEW.role_id;

    IF bound_category IS NULL THEN
        RAISE EXCEPTION 'role does not exist' USING ERRCODE = '23503';
    END IF;
    IF bound_category = 'custom'
       AND (bound_workspace <> NEW.workspace_id OR bound_active_version <> NEW.role_version) THEN
        RAISE EXCEPTION 'custom role binding crosses workspace or uses a stale version' USING ERRCODE = '23514';
    END IF;
    IF bound_category = 'system' AND NEW.role_version <> 1 THEN
        RAISE EXCEPTION 'system role version must be one' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER role_bindings_validate_workspace
    BEFORE INSERT OR UPDATE OF workspace_id, role_id, role_version ON role_bindings
    FOR EACH ROW EXECUTE FUNCTION validate_role_binding_workspace();

ALTER TABLE workspace_invitations
    ADD COLUMN role_version bigint NOT NULL DEFAULT 1 CHECK (role_version > 0);

CREATE FUNCTION validate_workspace_invitation_role()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    invitation_role_category text;
    invitation_role_workspace uuid;
    invitation_role_version bigint;
BEGIN
    SELECT category, workspace_id, active_version
    INTO invitation_role_category, invitation_role_workspace, invitation_role_version
    FROM roles WHERE id = NEW.role_id;
    IF invitation_role_category = 'custom'
       AND (invitation_role_workspace <> NEW.workspace_id OR invitation_role_version <> NEW.role_version) THEN
        RAISE EXCEPTION 'invitation role crosses workspace or uses a stale version' USING ERRCODE = '23514';
    END IF;
    IF invitation_role_category = 'system' AND NEW.role_version <> 1 THEN
        RAISE EXCEPTION 'system invitation role version must be one' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER workspace_invitations_validate_role
    BEFORE INSERT OR UPDATE OF workspace_id, role_id, role_version ON workspace_invitations
    FOR EACH ROW EXECUTE FUNCTION validate_workspace_invitation_role();

-- Historical workspace seed triggers name the old full unique constraint as
-- their conflict target. The active-only index intentionally has no matching
-- full constraint, so keep those idempotent writes targetless.
CREATE OR REPLACE FUNCTION seed_workspace_authorization()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    seeded_principal_id uuid;
BEGIN
    seeded_principal_id := semlia_seed_uuidv7('principal', NEW.id::text, NEW.created_at);
    INSERT INTO principals (id, workspace_id, kind, display_name, status)
    VALUES (seeded_principal_id, NEW.id, 'human', 'Workspace Admin', 'active')
    ON CONFLICT (id) DO NOTHING;
    INSERT INTO role_bindings (id, principal_id, role_id, scope_type, scope_id)
    VALUES (
        semlia_seed_uuidv7('binding', NEW.id::text, NEW.created_at),
        seeded_principal_id, 'workspace_admin', 'workspace', NEW.id::text
    )
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION seed_workspace_local_uat_identities()
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
    ON CONFLICT DO NOTHING;
    INSERT INTO role_bindings (id, principal_id, role_id, scope_type, scope_id, granted_by)
    VALUES (
        semlia_seed_uuidv7('independent_publisher_binding', NEW.id::text, NEW.created_at),
        publisher_principal_id, 'publisher', 'workspace', NEW.id::text, admin_principal_id
    )
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$;

COMMIT;
