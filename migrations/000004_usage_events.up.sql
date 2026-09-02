BEGIN;

CREATE TABLE usage_events (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    event_type text NOT NULL CHECK (event_type IN ('catalog.asset.read', 'catalog.search.completed')),
    idempotency_key text NOT NULL CHECK (idempotency_key <> '' AND length(idempotency_key) <= 256),
    data_version smallint NOT NULL DEFAULT 1 CHECK (data_version = 1),
    actor_id text CHECK (actor_id IS NULL OR (actor_id <> '' AND length(actor_id) <= 256)),
    asset_id uuid,
    revision_id uuid,
    channel text NOT NULL CHECK (channel IN ('api', 'agent', 'system')),
    outcome text NOT NULL CHECK (outcome IN ('succeeded', 'matched', 'zero_result', 'failed')),
    reason_code text CHECK (reason_code IS NULL OR reason_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    trace_id text NOT NULL CHECK (trace_id ~ '^[0-9a-f]{32}$'),
    search_fingerprint text CHECK (search_fingerprint IS NULL OR search_fingerprint ~ '^[0-9a-f]{64}$'),
    search_language text CHECK (search_language IS NULL OR search_language ~ '^[a-z]{2,3}(-[A-Z]{2})?$'),
    token_bucket text CHECK (token_bucket IS NULL OR token_bucket IN ('1', '2_3', '4_10', '11_plus')),
    result_bucket text CHECK (result_bucket IS NULL OR result_bucket IN ('0', '1_10', '11_100', '101_plus')),
    asset_type_filter text CHECK (asset_type_filter IS NULL OR asset_type_filter IN (
        'concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'
    )),
    lifecycle_filter text CHECK (lifecycle_filter IS NULL OR lifecycle_filter IN ('draft', 'active', 'deprecated', 'archived')),
    occurred_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at timestamptz NOT NULL,
    UNIQUE (workspace_id, event_type, idempotency_key),
    FOREIGN KEY (workspace_id, asset_id)
        REFERENCES semantic_assets (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, revision_id)
        REFERENCES asset_revisions (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT usage_events_expiry CHECK (expires_at > received_at),
    CONSTRAINT usage_events_shape CHECK (
        (
            event_type = 'catalog.asset.read'
            AND outcome = 'succeeded'
            AND asset_id IS NOT NULL
            AND revision_id IS NOT NULL
            AND search_fingerprint IS NULL
            AND search_language IS NULL
            AND token_bucket IS NULL
            AND result_bucket IS NULL
            AND asset_type_filter IS NULL
            AND lifecycle_filter IS NULL
        ) OR (
            event_type = 'catalog.search.completed'
            AND outcome IN ('matched', 'zero_result', 'failed')
            AND asset_id IS NULL
            AND revision_id IS NULL
            AND search_fingerprint IS NOT NULL
            AND token_bucket IS NOT NULL
            AND result_bucket IS NOT NULL
        )
    ),
    CONSTRAINT usage_events_failure_reason CHECK (
        (outcome = 'failed' AND reason_code IS NOT NULL)
        OR (outcome <> 'failed' AND reason_code IS NULL)
    )
);

CREATE INDEX usage_events_workspace_occurred_idx
    ON usage_events (workspace_id, occurred_at DESC, id);
CREATE INDEX usage_events_expiry_idx
    ON usage_events (expires_at, id);
CREATE INDEX usage_events_type_outcome_idx
    ON usage_events (workspace_id, event_type, outcome, occurred_at DESC);

COMMIT;
