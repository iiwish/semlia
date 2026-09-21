-- Governed authoring agent authorization (D6 fix): generation-time input
-- validation re-checks fresh propose grants under the workspace's seeded
-- agent principal, so the agent must hold the authoring actions it acts
-- with. Workspaces only seeded a binding for the human admin, which made
-- every production generation fail with NO_MATCHING_GRANT.
BEGIN;

INSERT INTO roles (id, name, description, category)
VALUES (
    'authoring_agent',
    'Governed Authoring Agent',
    'Workspace agent principal that authors AI-generated semantic suggestions; scoped to draft authoring, never human-only governance duties.',
    'system'
)
ON CONFLICT (id) DO NOTHING;

-- The vocabulary guard blocks new actions on system roles for runtime
-- callers; migrations seed the canonical set, so it is suspended for the
-- duration of this migration only.
ALTER TABLE role_actions DISABLE TRIGGER system_role_actions_protect_vocabulary;

INSERT INTO role_actions (role_id, action)
SELECT 'authoring_agent', action
FROM (VALUES
    ('workspace.read'),
    ('asset.read'),
    ('asset.propose'),
    ('asset.edit'),
    ('evidence.read'),
    ('source.read'),
    ('binding.read'),
    ('validation.run')
) AS needed(action)
WHERE EXISTS (SELECT 1 FROM roles WHERE id = 'authoring_agent')
ON CONFLICT (role_id, action) DO NOTHING;

ALTER TABLE role_actions ENABLE TRIGGER system_role_actions_protect_vocabulary;

-- Backfill one binding per workspace. Some workspaces carry duplicated agent
-- principals from earlier seed runs, so pin exactly one agent per workspace.
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
ORDER BY workspace.id, agent.created_at, agent.id;

-- Future workspaces: bind the agent right after its principal is seeded
-- (trigger name sorts AFTER workspaces_seeding_agent_principal).
CREATE FUNCTION seed_workspace_agent_authorization()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    agent_id uuid;
    agent_owner uuid;
BEGIN
    SELECT id, owner_principal_id INTO agent_id, agent_owner FROM principals
    WHERE workspace_id = NEW.id AND kind = 'agent'
    ORDER BY created_at, id
    LIMIT 1;
    IF agent_id IS NULL THEN
        RETURN NEW;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM role_bindings rb
        WHERE rb.principal_id = agent_id AND rb.role_id = 'authoring_agent'
          AND rb.scope_type = 'workspace' AND rb.scope_id = NEW.id::text
    ) THEN
        INSERT INTO role_bindings (id, principal_id, role_id, scope_type, scope_id, granted_by, workspace_id, role_version)
        VALUES (
            semlia_seed_uuidv7('agent_authoring_binding', NEW.id::text, NEW.created_at),
            agent_id, 'authoring_agent', 'workspace', NEW.id::text,
            agent_owner, NEW.id, 1
        );
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER workspaces_seeding_agent_authorization
    AFTER INSERT ON workspaces
    FOR EACH ROW EXECUTE FUNCTION seed_workspace_agent_authorization();

COMMIT;
