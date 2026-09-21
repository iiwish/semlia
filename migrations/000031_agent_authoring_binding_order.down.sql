BEGIN;

DROP TRIGGER IF EXISTS principals_seeding_agent_authorization ON principals;
DROP FUNCTION IF EXISTS seed_agent_authorization_on_principal();
DROP TRIGGER IF EXISTS workspaces_seeding_agent_principal_authorization ON workspaces;
CREATE TRIGGER workspaces_seeding_agent_authorization
    AFTER INSERT ON workspaces
    FOR EACH ROW EXECUTE FUNCTION seed_workspace_agent_authorization();

COMMIT;
