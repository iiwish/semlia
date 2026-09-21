BEGIN;

DROP TRIGGER IF EXISTS workspaces_seeding_agent_authorization ON workspaces;
DROP FUNCTION IF EXISTS seed_workspace_agent_authorization();

DELETE FROM role_bindings WHERE role_id = 'authoring_agent';
ALTER TABLE role_actions DISABLE TRIGGER system_role_actions_protect_vocabulary;
DELETE FROM role_actions WHERE role_id = 'authoring_agent';
ALTER TABLE role_actions ENABLE TRIGGER system_role_actions_protect_vocabulary;
ALTER TABLE roles DISABLE TRIGGER roles_immutable;
DELETE FROM roles WHERE id = 'authoring_agent' AND category = 'system';
ALTER TABLE roles ENABLE TRIGGER roles_immutable;

COMMIT;
