-- Fix 030 trigger ordering: AFTER INSERT triggers on workspaces fire in
-- trigger-name order, and workspaces_seeding_agent_authorization sorts
-- BEFORE workspaces_seeding_agent_principal, so the 030 trigger ran before
-- the agent principal existed and seeded zero bindings. Rename the trigger
-- so it sorts AFTER the principal seeder, backfill workspaces missed in
-- between, and add a principals-level trigger so late agent inserts bind
-- regardless of workspaces trigger order.
BEGIN;

DROP TRIGGER IF EXISTS workspaces_seeding_agent_authorization ON workspaces;

CREATE TRIGGER workspaces_seeding_agent_principal_authorization
    AFTER INSERT ON workspaces
    FOR EACH ROW EXECUTE FUNCTION seed_workspace_agent_authorization();

-- Backfill bindings for workspaces whose agent principal exists but holds
-- no authoring_agent workspace binding (covers databases migrated with 030
-- where the misordered trigger seeded nothing).
INSERT INTO role_bindings (id, principal_id, role_id, scope_type, scope_id, granted_by, workspace_id, role_version)
SELECT DISTINCT ON (workspace.id)
    semlia_seed_uuidv7('agent_authoring_binding', workspace.id::text, workspace.created_at),
    agent.id,
    'authoring_agent',
    'workspace',
    workspace.id::text,
    agent.owner_principal_id,
    workspace.id,
    1
FROM workspaces AS workspace
JOIN principals AS agent
  ON agent.workspace_id = workspace.id AND agent.kind = 'agent'
WHERE NOT EXISTS (
    SELECT 1 FROM role_bindings rb
    WHERE rb.workspace_id = workspace.id AND rb.role_id = 'authoring_agent'
      AND rb.scope_type = 'workspace' AND rb.scope_id = workspace.id::text
)
ORDER BY workspace.id, agent.created_at, agent.id
ON CONFLICT (id) DO NOTHING;

-- Order-independent safety net: bind whenever an agent principal appears.
CREATE OR REPLACE FUNCTION seed_agent_authorization_on_principal()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.kind IS DISTINCT FROM 'agent' THEN
        RETURN NEW;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM role_bindings rb
        WHERE rb.principal_id = NEW.id AND rb.role_id = 'authoring_agent'
          AND rb.scope_type = 'workspace' AND rb.scope_id = NEW.workspace_id::text
    ) THEN
        INSERT INTO role_bindings (id, principal_id, role_id, scope_type, scope_id, granted_by, workspace_id, role_version)
        VALUES (
            semlia_seed_uuidv7('agent_authoring_binding_principal', NEW.workspace_id::text, NEW.created_at),
            NEW.id, 'authoring_agent', 'workspace', NEW.workspace_id::text,
            NEW.owner_principal_id, NEW.workspace_id, 1
        )
        ON CONFLICT (id) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS principals_seeding_agent_authorization ON principals;
CREATE TRIGGER principals_seeding_agent_authorization
    AFTER INSERT ON principals
    FOR EACH ROW EXECUTE FUNCTION seed_agent_authorization_on_principal();

COMMIT;
