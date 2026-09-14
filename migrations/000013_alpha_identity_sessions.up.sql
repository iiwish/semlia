BEGIN;

CREATE TABLE user_accounts (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    display_name text NOT NULL CHECK (display_name <> '' AND length(display_name) <= 256),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX user_accounts_status_idx ON user_accounts (status, id);

CREATE TABLE external_identities (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    user_account_id uuid NOT NULL REFERENCES user_accounts (id) ON DELETE RESTRICT,
    issuer text NOT NULL CHECK (issuer ~ '^https?://[^[:space:]]+$' AND length(issuer) <= 512),
    subject text NOT NULL CHECK (subject <> '' AND length(subject) <= 512),
    verified_email text CHECK (
        verified_email IS NULL OR
        (verified_email = lower(verified_email) AND verified_email ~ '^[^[:space:]@]+@[^[:space:]@]+$' AND length(verified_email) <= 320)
    ),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (issuer, subject),
    UNIQUE (user_account_id, issuer)
);

CREATE INDEX external_identities_account_idx
    ON external_identities (user_account_id, status, id);

CREATE TABLE workspace_memberships (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    user_account_id uuid NOT NULL REFERENCES user_accounts (id) ON DELETE RESTRICT,
    principal_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'revoked')),
    admitted_by_principal_id uuid,
    admitted_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    suspended_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, user_account_id),
    UNIQUE (workspace_id, principal_id),
    FOREIGN KEY (workspace_id, principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, admitted_by_principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT workspace_memberships_state_time CHECK (
        (status = 'active' AND suspended_at IS NULL AND revoked_at IS NULL)
        OR (status = 'suspended' AND suspended_at IS NOT NULL AND revoked_at IS NULL)
        OR (status = 'revoked' AND revoked_at IS NOT NULL)
    )
);

CREATE INDEX workspace_memberships_account_idx
    ON workspace_memberships (user_account_id, status, workspace_id);
CREATE INDEX workspace_memberships_workspace_idx
    ON workspace_memberships (workspace_id, status, id);

CREATE TABLE workspace_invitations (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    issuer text NOT NULL CHECK (issuer ~ '^https?://[^[:space:]]+$' AND length(issuer) <= 512),
    subject text CHECK (subject IS NULL OR (subject <> '' AND length(subject) <= 512)),
    email text CHECK (
        email IS NULL OR
        (email = lower(email) AND email ~ '^[^[:space:]@]+@[^[:space:]@]+$' AND length(email) <= 320)
    ),
    role_id text NOT NULL REFERENCES roles (id) ON DELETE RESTRICT,
    token_digest bytea NOT NULL UNIQUE CHECK (octet_length(token_digest) = 32),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'revoked', 'expired')),
    created_by_principal_id uuid,
    accepted_by_account_id uuid REFERENCES user_accounts (id) ON DELETE RESTRICT,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, created_by_principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT workspace_invitations_admission_shape CHECK (
        subject IS NOT NULL OR email IS NOT NULL
    ),
    CONSTRAINT workspace_invitations_state_time CHECK (
        (status = 'pending' AND accepted_at IS NULL AND revoked_at IS NULL AND accepted_by_account_id IS NULL)
        OR (status = 'accepted' AND accepted_at IS NOT NULL AND accepted_by_account_id IS NOT NULL AND revoked_at IS NULL)
        OR (status = 'revoked' AND revoked_at IS NOT NULL AND accepted_at IS NULL AND accepted_by_account_id IS NULL)
        OR (status = 'expired' AND accepted_at IS NULL AND accepted_by_account_id IS NULL)
    )
);

CREATE UNIQUE INDEX workspace_invitations_pending_subject_idx
    ON workspace_invitations (workspace_id, issuer, subject)
    WHERE status = 'pending' AND subject IS NOT NULL;
CREATE UNIQUE INDEX workspace_invitations_pending_email_idx
    ON workspace_invitations (workspace_id, issuer, email)
    WHERE status = 'pending' AND email IS NOT NULL;
CREATE INDEX workspace_invitations_workspace_idx
    ON workspace_invitations (workspace_id, status, expires_at, id);

CREATE TABLE oidc_login_attempts (
    state_digest bytea PRIMARY KEY CHECK (octet_length(state_digest) = 32),
    nonce_digest bytea NOT NULL CHECK (octet_length(nonce_digest) = 32),
    verifier_envelope bytea NOT NULL CHECK (octet_length(verifier_envelope) >= 32),
    return_to text NOT NULL DEFAULT '/' CHECK (return_to ~ '^/[^\\]*$' AND length(return_to) <= 1024),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX oidc_login_attempts_expiry_idx
    ON oidc_login_attempts (expires_at) WHERE consumed_at IS NULL;

CREATE TABLE sessions (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    token_digest bytea NOT NULL UNIQUE CHECK (octet_length(token_digest) = 32),
    csrf_digest bytea NOT NULL CHECK (octet_length(csrf_digest) = 32),
    user_account_id uuid NOT NULL REFERENCES user_accounts (id) ON DELETE RESTRICT,
    idle_expires_at timestamptz NOT NULL,
    absolute_expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT sessions_expiry_order CHECK (
        created_at <= last_seen_at
        AND last_seen_at <= idle_expires_at
        AND idle_expires_at <= absolute_expires_at
    )
);

CREATE INDEX sessions_account_active_idx
    ON sessions (user_account_id, absolute_expires_at, id)
    WHERE revoked_at IS NULL;
CREATE INDEX sessions_expiry_idx
    ON sessions (absolute_expires_at, idle_expires_at)
    WHERE revoked_at IS NULL;

COMMIT;
