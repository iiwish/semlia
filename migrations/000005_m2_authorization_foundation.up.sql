BEGIN;

-- Workspace authorization version: a per-workspace monotonic counter that is
-- read by every capability decision and bumped transactionally whenever role
-- bindings change, so revocation invalidates future protected commands without
-- waiting for any time-based cache expiry (NFR-001).
ALTER TABLE workspaces
    ADD COLUMN authorization_version bigint NOT NULL DEFAULT 1
    CHECK (authorization_version > 0);

-- FR-003 permission vocabulary: stable action identifiers grouped by product
-- domain. Actions, never role names, are the authorization identifiers.
-- requires_human marks duties reserved for human principals (FR-007, SSOT 8.6:
-- agents cannot self-promote release levels or manage privileges).
CREATE TABLE actions (
    action text PRIMARY KEY CHECK (action ~ '^[a-z][a-z0-9_.]{1,63}$'),
    domain text NOT NULL CHECK (domain IN (
        'workspace', 'identity', 'access_control', 'knowledge', 'governance', 'sources', 'delivery', 'operations'
    )),
    requires_human boolean NOT NULL DEFAULT false
);

-- FR-004 system roles. Read-only system vocabulary; administrators derive
-- editable custom roles later. System roles are non-deletable (repository
-- invariant plus a database guard below).
CREATE TABLE roles (
    id text PRIMARY KEY CHECK (id ~ '^[a-z][a-z0-9_]{1,63}$'),
    name text NOT NULL CHECK (name <> '' AND length(name) <= 120),
    description text NOT NULL CHECK (description <> '' AND length(description) <= 512),
    category text NOT NULL DEFAULT 'system' CHECK (category IN ('system', 'custom'))
);

CREATE TABLE role_actions (
    role_id text NOT NULL REFERENCES roles (id) ON DELETE RESTRICT,
    action text NOT NULL REFERENCES actions (action) ON DELETE RESTRICT,
    PRIMARY KEY (role_id, action)
);

-- Principals are workspace-scoped actors. Agent principals are first-class and
-- accountable to a human owner principal in the same workspace (D6).
CREATE TABLE principals (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (kind IN ('human', 'agent')),
    display_name text NOT NULL CHECK (display_name <> '' AND length(display_name) <= 256),
    owner_principal_id uuid,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, display_name),
    FOREIGN KEY (workspace_id, owner_principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT principals_owner_accountability CHECK (
        (kind = 'agent') = (owner_principal_id IS NOT NULL)
    ),
    CONSTRAINT principals_owner_self CHECK (owner_principal_id IS NULL OR owner_principal_id <> id)
);

CREATE INDEX principals_workspace_status_idx
    ON principals (workspace_id, status, kind, id);

CREATE FUNCTION validate_principal_owner()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    owner_kind text;
BEGIN
    IF NEW.owner_principal_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT kind INTO owner_kind FROM principals
        WHERE workspace_id = NEW.workspace_id AND id = NEW.owner_principal_id;
    IF owner_kind IS NULL OR owner_kind <> 'human' THEN
        RAISE EXCEPTION 'agent principals require a human owner principal' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER principals_validate_owner
    BEFORE INSERT OR UPDATE OF owner_principal_id ON principals
    FOR EACH ROW EXECUTE FUNCTION validate_principal_owner();

-- Scoped role bindings mirroring the frontend binding model:
-- workspace|domain|asset|source|environment|release|consumer.
CREATE TABLE role_bindings (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    principal_id uuid NOT NULL REFERENCES principals (id) ON DELETE RESTRICT,
    role_id text NOT NULL REFERENCES roles (id) ON DELETE RESTRICT,
    scope_type text NOT NULL CHECK (scope_type IN (
        'workspace', 'domain', 'asset', 'source', 'environment', 'release', 'consumer'
    )),
    scope_id text NOT NULL CHECK (scope_id <> '' AND length(scope_id) <= 256),
    granted_by uuid REFERENCES principals (id) ON DELETE RESTRICT,
    granted_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (principal_id, role_id, scope_type, scope_id)
);

CREATE INDEX role_bindings_principal_idx
    ON role_bindings (principal_id, role_id);

-- Immutable authorization decision facts. Every protected command records a row
-- for allow and deny decisions alike (FR-013).
CREATE TABLE authorization_events (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    principal_id uuid REFERENCES principals (id) ON DELETE RESTRICT,
    actor text NOT NULL CHECK (actor <> '' AND length(actor) <= 256),
    action text NOT NULL CHECK (action ~ '^[a-z][a-z0-9_.]{1,63}$'),
    resource_type text NOT NULL CHECK (resource_type IN (
        'workspace', 'domain', 'asset', 'source', 'environment', 'release', 'consumer'
    )),
    resource_id text NOT NULL CHECK (resource_id <> '' AND length(resource_id) <= 256),
    decision text NOT NULL CHECK (decision IN ('allow', 'deny')),
    reason_code text NOT NULL CHECK (reason_code IN (
        'ROLE_GRANT', 'SESSION_CAPABILITY', 'NO_MATCHING_GRANT', 'PRINCIPAL_INACTIVE', 'SEPARATION_OF_DUTY'
    )),
    authorization_version bigint NOT NULL CHECK (authorization_version > 0),
    trace_id text NOT NULL CHECK (trace_id ~ '^[0-9a-f]{32}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT authorization_events_reason CHECK (
        (decision = 'allow') = (reason_code IN ('ROLE_GRANT', 'SESSION_CAPABILITY'))
    )
);

CREATE INDEX authorization_events_principal_created_idx
    ON authorization_events (principal_id, created_at DESC, id);
CREATE INDEX authorization_events_workspace_created_idx
    ON authorization_events (workspace_id, created_at DESC, id);

CREATE TRIGGER authorization_events_immutable
    BEFORE UPDATE OR DELETE ON authorization_events
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TRIGGER roles_immutable
    BEFORE DELETE ON roles
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

-- Deterministic UUIDv7 seeding helper (md5 entropy, UUIDv7 timestamp/variant
-- bits) so the workspace seed trigger produces stable principal and binding
-- identities and re-running the seed statements is idempotent. It stays part of
-- the schema because the seed trigger resolves it at runtime. PostgreSQL 17
-- compatibility per ADR-0003: no native UUIDv7 generation dependency.
CREATE FUNCTION semlia_seed_uuidv7(resource_kind text, seed text, created_value timestamptz)
RETURNS uuid
LANGUAGE plpgsql
IMMUTABLE
STRICT
AS $$
DECLARE
    millis bigint;
    timestamp_hex text;
    entropy text;
BEGIN
    millis := floor(extract(epoch FROM created_value) * 1000)::bigint;
    IF millis < 0 OR millis > 281474976710655 THEN
        RAISE EXCEPTION 'seed timestamp is outside UUIDv7 range' USING ERRCODE = '22008';
    END IF;
    timestamp_hex := lpad(to_hex(millis), 12, '0');
    entropy := md5(resource_kind || chr(31) || seed);
    RETURN (
        substr(timestamp_hex, 1, 8) || '-' ||
        substr(timestamp_hex, 9, 4) || '-7' ||
        substr(entropy, 1, 3) || '-8' ||
        substr(entropy, 4, 3) || '-' ||
        substr(entropy, 7, 12)
    )::uuid;
END;
$$;

-- Idempotent seed of the FR-003 action vocabulary and the nine FR-004 system
-- roles. The permission sets mirror web/src/data.ts, the frontend contract.
-- Re-invoking this function must not duplicate rows (tested).
CREATE FUNCTION seed_authorization_vocabulary()
RETURNS void
LANGUAGE sql
AS $$
INSERT INTO actions (action, domain, requires_human) VALUES
    ('workspace.read', 'workspace', false),
    ('workspace.manage', 'workspace', false),
    ('member.read', 'identity', false),
    ('member.manage', 'identity', true),
    ('group.manage', 'identity', true),
    ('role.read', 'access_control', false),
    ('role.manage', 'access_control', true),
    ('role.assign', 'access_control', true),
    ('authorization.inspect', 'access_control', false),
    ('asset.read', 'knowledge', false),
    ('asset.propose', 'knowledge', false),
    ('asset.edit', 'knowledge', false),
    ('evidence.read', 'knowledge', false),
    ('proposal.review', 'governance', true),
    ('validation.run', 'governance', false),
    ('release.publish', 'governance', true),
    ('release.rollback', 'governance', true),
    ('source.read', 'sources', false),
    ('source.manage', 'sources', false),
    ('ingestion.run', 'sources', false),
    ('binding.read', 'delivery', false),
    ('binding.manage', 'delivery', false),
    ('semantic.resolve', 'delivery', false),
    ('semantic.execute', 'delivery', false),
    ('audit.read', 'operations', false),
    ('runtime.read', 'operations', false),
    ('runtime.manage', 'operations', false)
ON CONFLICT (action) DO NOTHING;

INSERT INTO roles (id, name, description, category) VALUES
    ('workspace_admin', 'Workspace Admin', '管理工作区、成员、角色和全部治理配置。', 'system'),
    ('security_admin', 'Security Admin', '管理成员授权并检查有效权限，不参与业务发布。', 'system'),
    ('semantic_steward', 'Semantic Steward', '维护语义资产、证据、物理绑定和验证结果。', 'system'),
    ('asset_owner', 'Asset Owner', '对指定资产承担业务责任并发起变更。', 'system'),
    ('reviewer', 'Reviewer', '独立评审语义变更，不执行同范围生产发布。', 'system'),
    ('publisher', 'Publisher', '发布或回滚已通过独立评审的受保护版本。', 'system'),
    ('source_operator', 'Source Operator', '管理数据来源、摄取任务和物理绑定。', 'system'),
    ('consumer_developer', 'Consumer Developer', '读取已发布语义并执行面向消费端的查询。', 'system'),
    ('auditor', 'Auditor', '只读检查资产、授权、审计与运行记录。', 'system')
ON CONFLICT (id) DO NOTHING;

INSERT INTO role_actions (role_id, action) VALUES
    ('workspace_admin', 'workspace.read'), ('workspace_admin', 'workspace.manage'),
    ('workspace_admin', 'member.read'), ('workspace_admin', 'member.manage'),
    ('workspace_admin', 'group.manage'), ('workspace_admin', 'role.read'),
    ('workspace_admin', 'role.manage'), ('workspace_admin', 'role.assign'),
    ('workspace_admin', 'authorization.inspect'), ('workspace_admin', 'asset.read'),
    ('workspace_admin', 'asset.propose'), ('workspace_admin', 'asset.edit'),
    ('workspace_admin', 'evidence.read'), ('workspace_admin', 'proposal.review'),
    ('workspace_admin', 'validation.run'), ('workspace_admin', 'release.publish'),
    ('workspace_admin', 'release.rollback'), ('workspace_admin', 'source.read'),
    ('workspace_admin', 'source.manage'), ('workspace_admin', 'ingestion.run'),
    ('workspace_admin', 'binding.read'), ('workspace_admin', 'binding.manage'),
    ('workspace_admin', 'semantic.resolve'), ('workspace_admin', 'semantic.execute'),
    ('workspace_admin', 'audit.read'), ('workspace_admin', 'runtime.read'),
    ('workspace_admin', 'runtime.manage'),
    ('security_admin', 'workspace.read'), ('security_admin', 'member.read'),
    ('security_admin', 'member.manage'), ('security_admin', 'group.manage'),
    ('security_admin', 'role.read'), ('security_admin', 'role.manage'),
    ('security_admin', 'role.assign'), ('security_admin', 'authorization.inspect'),
    ('security_admin', 'audit.read'),
    ('semantic_steward', 'workspace.read'), ('semantic_steward', 'asset.read'),
    ('semantic_steward', 'asset.propose'), ('semantic_steward', 'asset.edit'),
    ('semantic_steward', 'evidence.read'), ('semantic_steward', 'validation.run'),
    ('semantic_steward', 'binding.read'), ('semantic_steward', 'binding.manage'),
    ('asset_owner', 'workspace.read'), ('asset_owner', 'asset.read'),
    ('asset_owner', 'asset.propose'), ('asset_owner', 'asset.edit'),
    ('asset_owner', 'evidence.read'), ('asset_owner', 'validation.run'),
    ('reviewer', 'workspace.read'), ('reviewer', 'asset.read'),
    ('reviewer', 'evidence.read'), ('reviewer', 'proposal.review'),
    ('reviewer', 'validation.run'), ('reviewer', 'audit.read'),
    ('publisher', 'workspace.read'), ('publisher', 'asset.read'),
    ('publisher', 'evidence.read'), ('publisher', 'release.publish'),
    ('publisher', 'release.rollback'), ('publisher', 'audit.read'),
    ('source_operator', 'workspace.read'), ('source_operator', 'source.read'),
    ('source_operator', 'source.manage'), ('source_operator', 'ingestion.run'),
    ('source_operator', 'binding.read'), ('source_operator', 'binding.manage'),
    ('source_operator', 'runtime.read'),
    ('consumer_developer', 'workspace.read'), ('consumer_developer', 'asset.read'),
    ('consumer_developer', 'evidence.read'), ('consumer_developer', 'semantic.resolve'),
    ('consumer_developer', 'semantic.execute'), ('consumer_developer', 'runtime.read'),
    ('auditor', 'workspace.read'), ('auditor', 'member.read'), ('auditor', 'role.read'),
    ('auditor', 'authorization.inspect'), ('auditor', 'asset.read'),
    ('auditor', 'evidence.read'), ('auditor', 'source.read'), ('auditor', 'binding.read'),
    ('auditor', 'audit.read'), ('auditor', 'runtime.read')
ON CONFLICT (role_id, action) DO NOTHING;
$$;

SELECT seed_authorization_vocabulary();

-- Private-incubation actor bootstrap: every workspace receives one seeded human
-- principal holding the workspace-admin binding granted by the system. The
-- binding with granted_by IS NULL identifies the workspace's default actor, so
-- M1 flows keep working unchanged for authorized actors until real
-- authentication replaces the X-Semlia-Principal header contract.
CREATE FUNCTION seed_workspace_authorization()
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
    ON CONFLICT (principal_id, role_id, scope_type, scope_id) DO NOTHING;
    RETURN NEW;
END;
$$;

CREATE TRIGGER workspaces_seed_authorization
    AFTER INSERT ON workspaces
    FOR EACH ROW EXECUTE FUNCTION seed_workspace_authorization();

-- Backfill workspaces created before this migration.
INSERT INTO principals (id, workspace_id, kind, display_name, status)
SELECT
    semlia_seed_uuidv7('principal', workspace.id::text, workspace.created_at),
    workspace.id, 'human', 'Workspace Admin', 'active'
FROM workspaces AS workspace
ON CONFLICT (id) DO NOTHING;

INSERT INTO role_bindings (id, principal_id, role_id, scope_type, scope_id)
SELECT
    semlia_seed_uuidv7('binding', workspace.id::text, workspace.created_at),
    semlia_seed_uuidv7('principal', workspace.id::text, workspace.created_at),
    'workspace_admin', 'workspace', workspace.id::text
FROM workspaces AS workspace
ON CONFLICT (principal_id, role_id, scope_type, scope_id) DO NOTHING;

COMMIT;
