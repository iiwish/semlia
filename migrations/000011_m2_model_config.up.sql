BEGIN;

-- M2-T009 model configuration substrate (SSOT §8.6, §12 S-002; docs/specs/
-- m2-governed-authoring/analysis.md D1; web/src/ModelConfigurationView.tsx
-- contract): workspace-scoped model providers and per-provider model settings.
-- Credential MATERIAL is structurally unrepresentable: only the environment
-- variable NAME of the secret and a salted sha256 revision digest of the
-- provided secret are persisted — never the secret itself, so the governed
-- loop can call live providers without S-002 exposure.

CREATE TABLE model_providers (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    protocol text NOT NULL CHECK (protocol IN ('openai', 'anthropic', 'gemini', 'openai_compatible')),
    display_name text NOT NULL CHECK (display_name <> '' AND length(display_name) <= 120),
    base_url text CHECK (base_url IS NULL OR (base_url <> '' AND length(base_url) <= 512)),
    credential_env text NOT NULL CHECK (credential_env ~ '^[A-Z][A-Z0-9_]{0,63}$'),
    credential_revision text NOT NULL CHECK (credential_revision ~ '^sha256:[0-9a-f]{64}$'),
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id)
);

CREATE INDEX model_providers_workspace_idx
    ON model_providers (workspace_id, created_at, id);

CREATE TABLE model_settings (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    provider_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('llm', 'embedding')),
    model text NOT NULL CHECK (model <> '' AND length(model) <= 128),
    enabled boolean NOT NULL DEFAULT true,
    is_default boolean NOT NULL DEFAULT false,
    capability text NOT NULL CHECK (capability <> '' AND length(capability) <= 128),
    token_limit integer NOT NULL CHECK (token_limit > 0),
    embedding_dimension integer CHECK (embedding_dimension IS NULL OR embedding_dimension > 0),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, provider_id)
        REFERENCES model_providers (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT model_settings_dimension_shape CHECK ((kind = 'embedding') = (embedding_dimension IS NOT NULL))
);

CREATE INDEX model_settings_workspace_kind_idx
    ON model_settings (workspace_id, kind, enabled, is_default);

-- One default per (workspace, kind): the generation flow resolves the
-- workspace default llm through this partial index.
CREATE UNIQUE INDEX model_settings_default_idx
    ON model_settings (workspace_id, kind) WHERE is_default;

-- D6 (analysis.md): agent runs and AI-authored proposals attribute to a
-- workspace-scoped seeded agent principal, owned by the seeded workspace
-- admin. The deterministic seed ID keeps the down-migration deletable.
-- Trigger name sorts AFTER workspaces_seed_authorization (same-event
-- triggers fire in name order), so the owner principal exists first.
CREATE FUNCTION seed_workspace_agent_principal()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    owner_id uuid;
    agent_id uuid;
BEGIN
    owner_id := semlia_seed_uuidv7('principal', NEW.id::text, NEW.created_at);
    agent_id := semlia_seed_uuidv7('agent_principal', NEW.id::text, NEW.created_at);
    INSERT INTO principals (id, workspace_id, kind, display_name, owner_principal_id, status)
    VALUES (agent_id, NEW.id, 'agent', 'Governed Authoring Agent', owner_id, 'active')
    ON CONFLICT (id) DO NOTHING;
    RETURN NEW;
END;
$$;

CREATE TRIGGER workspaces_seeding_agent_principal
    AFTER INSERT ON workspaces
    FOR EACH ROW EXECUTE FUNCTION seed_workspace_agent_principal();

-- Backfill workspaces created before this migration.
INSERT INTO principals (id, workspace_id, kind, display_name, owner_principal_id, status)
SELECT
    semlia_seed_uuidv7('agent_principal', workspace.id::text, workspace.created_at),
    workspace.id, 'agent', 'Governed Authoring Agent',
    semlia_seed_uuidv7('principal', workspace.id::text, workspace.created_at),
    'active'
FROM workspaces AS workspace
ON CONFLICT (id) DO NOTHING;

COMMIT;
