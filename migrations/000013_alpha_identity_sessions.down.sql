BEGIN;

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS oidc_login_attempts;
DROP TABLE IF EXISTS workspace_invitations;
DROP TABLE IF EXISTS workspace_memberships;
DROP TABLE IF EXISTS external_identities;
DROP TABLE IF EXISTS user_accounts;

COMMIT;
