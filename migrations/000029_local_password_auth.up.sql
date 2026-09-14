BEGIN;

CREATE TABLE local_password_credentials (
    user_account_id uuid PRIMARY KEY REFERENCES user_accounts(id) ON DELETE RESTRICT,
    username text NOT NULL UNIQUE CHECK (username = lower(username) AND length(username) BETWEEN 3 AND 320),
    password_hash text NOT NULL CHECK (length(password_hash) BETWEEN 32 AND 512),
    credential_version bigint NOT NULL DEFAULT 1 CHECK (credential_version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE password_login_budgets (
    key text PRIMARY KEY CHECK (length(key) BETWEEN 1 AND 512),
    attempts integer NOT NULL CHECK (attempts BETWEEN 1 AND 1000000),
    expires_at timestamptz NOT NULL
);
CREATE INDEX password_login_budgets_expiry_idx ON password_login_budgets(expires_at);

COMMIT;
