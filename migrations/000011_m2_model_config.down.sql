-- Downgrade 000011: drop only the model-configuration objects introduced
-- here — the seeded agent principals, their seeding trigger and the provider
-- and model-settings tables. Nothing else is touched.

DROP TRIGGER IF EXISTS workspaces_seeding_agent_principal ON workspaces;
DROP FUNCTION IF EXISTS seed_workspace_agent_principal();

-- Remove the deterministic seeded agent principals (D6). Agent runs and
-- proposals reference principals with ON DELETE RESTRICT, so workspaces with
-- recorded AI activity keep their audit trail: those rows are not deletable
-- and the downgrade fails loudly instead of erasing attribution.
DELETE FROM principals AS principal
WHERE principal.kind = 'agent'
  AND principal.id = semlia_seed_uuidv7(
      'agent_principal', principal.workspace_id::text,
      (SELECT workspace.created_at FROM workspaces AS workspace WHERE workspace.id = principal.workspace_id)
  );

DROP INDEX IF EXISTS model_settings_default_idx;
DROP INDEX IF EXISTS model_settings_workspace_kind_idx;
DROP TABLE IF EXISTS model_settings;

DROP INDEX IF EXISTS model_providers_workspace_idx;
DROP TABLE IF EXISTS model_providers;
