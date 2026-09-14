BEGIN;
CREATE TABLE consumer_machine_principals (
 workspace_id uuid NOT NULL, consumer_id uuid NOT NULL, principal_id uuid NOT NULL,
 PRIMARY KEY(workspace_id,consumer_id), UNIQUE(workspace_id,consumer_id,principal_id),
 FOREIGN KEY(workspace_id,consumer_id) REFERENCES consumers(workspace_id,id),
 FOREIGN KEY(workspace_id,principal_id) REFERENCES principals(workspace_id,id)
);
CREATE TRIGGER consumer_machine_principals_immutable BEFORE UPDATE OR DELETE ON consumer_machine_principals
FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TABLE client_credentials (
 id uuid PRIMARY KEY CHECK (substring(id::text,15,1)='7'),
 workspace_id uuid NOT NULL REFERENCES workspaces(id),
 consumer_id uuid NOT NULL,
 binding_id uuid NOT NULL,
 principal_id uuid NOT NULL,
 name text NOT NULL CHECK (length(name) BETWEEN 1 AND 160),
 token_prefix text NOT NULL UNIQUE,
 verifier_digest bytea NOT NULL CHECK (octet_length(verifier_digest)=32),
 verifier_version integer NOT NULL DEFAULT 1 CHECK (verifier_version=1),
 allowed_actions text[] NOT NULL CHECK (cardinality(allowed_actions)>0 AND allowed_actions <@ ARRAY['asset.read','semantic.resolve']::text[]),
 scope_type text NOT NULL CHECK (scope_type IN ('workspace','asset','release')),
 scope_id text NOT NULL,
 issued_by_principal_id uuid NOT NULL,
 issued_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL CHECK (expires_at>issued_at),
 revoked_at timestamptz,
 revoked_by_principal_id uuid,
 rotated_from_id uuid REFERENCES client_credentials(id),
 last_used_at timestamptz,
 UNIQUE(workspace_id,id),
 FOREIGN KEY(workspace_id,consumer_id) REFERENCES consumers(workspace_id,id),
 FOREIGN KEY(workspace_id,consumer_id,principal_id) REFERENCES consumer_machine_principals(workspace_id,consumer_id,principal_id),
 FOREIGN KEY(workspace_id,binding_id) REFERENCES consumer_bindings(workspace_id,id),
 FOREIGN KEY(workspace_id,principal_id) REFERENCES principals(workspace_id,id),
 FOREIGN KEY(workspace_id,issued_by_principal_id) REFERENCES principals(workspace_id,id)
);
CREATE INDEX client_credentials_list ON client_credentials(workspace_id,issued_at DESC,id);
ALTER TABLE semantic_queries DROP CONSTRAINT semantic_queries_channel_check;
ALTER TABLE semantic_queries ADD CONSTRAINT semantic_queries_channel_check CHECK(channel IN ('api','ask','agent','system','mcp','cli','sdk'));
ALTER TABLE semantic_resolution_events DROP CONSTRAINT semantic_resolution_events_channel_check;
ALTER TABLE semantic_resolution_events ADD CONSTRAINT semantic_resolution_events_channel_check CHECK(channel IN ('api','ask','agent','system','mcp','cli','sdk'));
CREATE TABLE webhook_subscriptions(
 id uuid PRIMARY KEY CHECK(substring(id::text,15,1)='7'),workspace_id uuid NOT NULL REFERENCES workspaces(id),
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 160),endpoint text NOT NULL CHECK(length(endpoint)<=2048),
 enabled boolean NOT NULL,event_types text[] NOT NULL CHECK(cardinality(event_types)>0 AND event_types <@ ARRAY['release.published','catalog.asset.changed']::text[]),
 version integer NOT NULL CHECK(version>0),signing_version integer NOT NULL CHECK(signing_version>0),secret_suffix text NOT NULL CHECK(length(secret_suffix)=4),
 created_by_principal_id uuid NOT NULL,created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,
 UNIQUE(workspace_id,id),FOREIGN KEY(workspace_id,created_by_principal_id) REFERENCES principals(workspace_id,id)
);
CREATE TABLE webhook_signing_secrets(
 subscription_id uuid NOT NULL REFERENCES webhook_subscriptions(id),version integer NOT NULL CHECK(version>0),
 envelope bytea NOT NULL CHECK(octet_length(envelope)>28),created_at timestamptz NOT NULL,
 PRIMARY KEY(subscription_id,version)
);
CREATE TRIGGER webhook_signing_secrets_immutable BEFORE UPDATE OR DELETE ON webhook_signing_secrets FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TABLE webhook_deliveries(
 id uuid PRIMARY KEY CHECK(substring(id::text,15,1)='7'),workspace_id uuid NOT NULL REFERENCES workspaces(id),
 subscription_id uuid NOT NULL,subscription_version integer NOT NULL,signing_version integer NOT NULL,
 event_id uuid NOT NULL REFERENCES outbox_events(id),event_type text NOT NULL,
 envelope bytea NOT NULL,payload_digest text NOT NULL CHECK(payload_digest ~ '^sha256:[0-9a-f]{64}$'),
 state text NOT NULL CHECK(state IN ('queued','running','succeeded','cancelled','dead_letter')),
 attempt integer NOT NULL DEFAULT 0,max_attempts integer NOT NULL DEFAULT 8 CHECK(max_attempts BETWEEN 1 AND 16),
 http_status integer NOT NULL DEFAULT 0,error_code text NOT NULL DEFAULT '',
 next_attempt_at timestamptz NOT NULL,lease_owner text,leased_until timestamptz,
 trace_id text NOT NULL,created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,
 runtime_run_id uuid NOT NULL,
 UNIQUE(subscription_id,event_id),FOREIGN KEY(workspace_id,subscription_id) REFERENCES webhook_subscriptions(workspace_id,id)
);
CREATE INDEX webhook_deliveries_claim ON webhook_deliveries(state,next_attempt_at);
CREATE INDEX webhook_deliveries_list ON webhook_deliveries(workspace_id,created_at DESC,id DESC);
CREATE TABLE webhook_fanout_receipts(event_id uuid PRIMARY KEY REFERENCES outbox_events(id),created_at timestamptz NOT NULL);
COMMIT;
