-- name: CreateOIDCLoginAttempt :exec
INSERT INTO oidc_login_attempts (
    state_digest, nonce_digest, verifier_envelope, return_to, expires_at, created_at
) VALUES (
    sqlc.arg(state_digest), sqlc.arg(nonce_digest), sqlc.arg(verifier_envelope),
    sqlc.arg(return_to), sqlc.arg(expires_at), sqlc.arg(created_at)
);

-- name: ConsumeOIDCLoginAttempt :one
UPDATE oidc_login_attempts
SET consumed_at = sqlc.arg(consumed_at)
WHERE state_digest = sqlc.arg(state_digest)
  AND consumed_at IS NULL
  AND expires_at > sqlc.arg(consumed_at)
RETURNING *;

-- name: DeleteExpiredOIDCLoginAttempts :execrows
DELETE FROM oidc_login_attempts
WHERE expires_at <= sqlc.arg(now)::timestamptz OR consumed_at IS NOT NULL;

-- name: GetExternalIdentityBySubject :one
SELECT external_identity.*, account.display_name, account.status AS account_status,
       account.created_at AS account_created_at, account.updated_at AS account_updated_at
FROM external_identities AS external_identity
JOIN user_accounts AS account ON account.id = external_identity.user_account_id
WHERE external_identity.issuer = sqlc.arg(issuer)
  AND external_identity.subject = sqlc.arg(subject);

-- name: ListMatchingPendingInvitations :many
SELECT * FROM workspace_invitations
WHERE issuer = sqlc.arg(issuer)
  AND status = 'pending'
  AND expires_at > sqlc.arg(now)::timestamptz
  AND (
      (subject IS NOT NULL AND subject = sqlc.arg(subject))
      OR (email IS NOT NULL AND sqlc.arg(email_verified)::boolean AND email = sqlc.arg(email))
  )
ORDER BY created_at, id;

-- name: CreateUserAccount :one
INSERT INTO user_accounts (id, display_name, status, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(display_name), 'active', sqlc.arg(created_at), sqlc.arg(created_at))
RETURNING *;

-- name: CreateExternalIdentity :one
INSERT INTO external_identities (
    id, user_account_id, issuer, subject, verified_email, status, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(user_account_id), sqlc.arg(issuer), sqlc.arg(subject),
    sqlc.narg(verified_email), 'active', sqlc.arg(created_at), sqlc.arg(created_at)
)
RETURNING *;

-- name: CreateWorkspaceMembership :one
INSERT INTO workspace_memberships (
    id, workspace_id, user_account_id, principal_id, status,
    admitted_by_principal_id, admitted_at, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(user_account_id), sqlc.arg(principal_id),
    'active', sqlc.narg(admitted_by_principal_id), sqlc.arg(admitted_at),
    sqlc.arg(admitted_at), sqlc.arg(admitted_at)
)
ON CONFLICT (workspace_id, user_account_id) DO UPDATE
SET status = 'active', suspended_at = NULL, revoked_at = NULL, updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetWorkspaceMembershipByAccount :one
SELECT * FROM workspace_memberships
WHERE workspace_id = sqlc.arg(workspace_id)
  AND user_account_id = sqlc.arg(user_account_id);

-- name: GrantWorkspaceRole :execrows
INSERT INTO role_bindings (
    id, workspace_id, principal_id, role_id, role_version, scope_type, scope_id,
    granted_by, granted_at
)
VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(principal_id), sqlc.arg(role_id),
    sqlc.arg(role_version), 'workspace', sqlc.arg(workspace_id)::uuid::text,
    sqlc.narg(granted_by), sqlc.arg(granted_at)
)
ON CONFLICT DO NOTHING;

-- name: AcceptWorkspaceInvitation :one
UPDATE workspace_invitations
SET status = 'accepted', accepted_by_account_id = sqlc.arg(account_id),
    accepted_at = sqlc.arg(accepted_at), updated_at = sqlc.arg(accepted_at)
WHERE id = sqlc.arg(invitation_id) AND status = 'pending'
RETURNING *;

-- name: CreateSession :one
INSERT INTO sessions (
    id, token_digest, csrf_digest, user_account_id, idle_expires_at,
    absolute_expires_at, last_seen_at, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(token_digest), sqlc.arg(csrf_digest), sqlc.arg(user_account_id),
    sqlc.arg(idle_expires_at), sqlc.arg(absolute_expires_at), sqlc.arg(last_seen_at),
    sqlc.arg(created_at), sqlc.arg(created_at)
)
RETURNING *;

-- name: LoadActiveSession :one
SELECT session.*, account.display_name, account.status AS account_status,
       account.created_at AS account_created_at, account.updated_at AS account_updated_at
FROM sessions AS session
JOIN user_accounts AS account ON account.id = session.user_account_id
WHERE session.token_digest = sqlc.arg(token_digest)
  AND session.revoked_at IS NULL
  AND session.idle_expires_at > sqlc.arg(now)::timestamptz
  AND session.absolute_expires_at > sqlc.arg(now)::timestamptz
  AND account.status = 'active';

-- name: TouchSession :one
UPDATE sessions
SET last_seen_at = sqlc.arg(now),
    idle_expires_at = LEAST(sqlc.arg(idle_expires_at), absolute_expires_at),
    updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(session_id) AND revoked_at IS NULL
  AND idle_expires_at > sqlc.arg(now)::timestamptz
  AND absolute_expires_at > sqlc.arg(now)::timestamptz
RETURNING *;

-- name: RevokeSessionByDigest :execrows
UPDATE sessions
SET revoked_at = sqlc.arg(revoked_at), updated_at = sqlc.arg(revoked_at)
WHERE token_digest = sqlc.arg(token_digest) AND revoked_at IS NULL;

-- name: ListAccountMemberships :many
SELECT membership.*, principal.display_name AS principal_display_name,
       workspace.slug AS workspace_slug, workspace.display_name AS workspace_display_name,
       workspace.authorization_version,
       COALESCE(array_agg(binding.role_id ORDER BY binding.role_id)
           FILTER (WHERE binding.role_id IS NOT NULL), ARRAY[]::text[])::text[] AS role_ids
FROM workspace_memberships AS membership
JOIN principals AS principal
  ON principal.workspace_id = membership.workspace_id AND principal.id = membership.principal_id
JOIN workspaces AS workspace ON workspace.id = membership.workspace_id
LEFT JOIN role_bindings AS binding ON binding.principal_id = principal.id
WHERE membership.user_account_id = sqlc.arg(user_account_id)
  AND membership.status = 'active'
  AND principal.status = 'active'
GROUP BY membership.id, principal.display_name, workspace.slug, workspace.display_name,
         workspace.authorization_version
ORDER BY workspace.display_name, membership.workspace_id;

-- name: ListWorkspaceMemberships :many
SELECT membership.*, account.display_name AS account_display_name,
       principal.display_name AS principal_display_name,
       COALESCE(array_agg(binding.role_id ORDER BY binding.role_id)
           FILTER (WHERE binding.role_id IS NOT NULL), ARRAY[]::text[])::text[] AS role_ids
FROM workspace_memberships AS membership
JOIN user_accounts AS account ON account.id = membership.user_account_id
JOIN principals AS principal
  ON principal.workspace_id = membership.workspace_id AND principal.id = membership.principal_id
LEFT JOIN role_bindings AS binding ON binding.principal_id = principal.id
WHERE membership.workspace_id = sqlc.arg(workspace_id)
GROUP BY membership.id, account.display_name, principal.display_name
ORDER BY account.display_name, membership.id;

-- name: GetWorkspaceMembershipDetails :one
SELECT membership.*, account.display_name AS account_display_name,
       principal.display_name AS principal_display_name,
       COALESCE(array_agg(binding.role_id ORDER BY binding.role_id)
           FILTER (WHERE binding.role_id IS NOT NULL), ARRAY[]::text[])::text[] AS role_ids
FROM workspace_memberships AS membership
JOIN user_accounts AS account ON account.id = membership.user_account_id
JOIN principals AS principal
  ON principal.workspace_id = membership.workspace_id AND principal.id = membership.principal_id
LEFT JOIN role_bindings AS binding ON binding.principal_id = principal.id
WHERE membership.workspace_id = sqlc.arg(workspace_id) AND membership.id = sqlc.arg(membership_id)
GROUP BY membership.id, account.display_name, principal.display_name;

-- name: ListWorkspaceInvitations :many
SELECT * FROM workspace_invitations
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY created_at DESC, id DESC;

-- name: CreateWorkspaceInvitation :one
INSERT INTO workspace_invitations (
    id, workspace_id, issuer, subject, email, role_id, role_version, token_digest, status,
    created_by_principal_id, expires_at, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(issuer), sqlc.narg(subject),
    sqlc.narg(email), sqlc.arg(role_id), sqlc.arg(role_version), sqlc.arg(token_digest), 'pending',
    sqlc.narg(created_by_principal_id), sqlc.arg(expires_at), sqlc.arg(created_at), sqlc.arg(created_at)
)
RETURNING *;

-- name: GetWorkspaceMembershipForUpdate :one
SELECT * FROM workspace_memberships
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(membership_id)
FOR UPDATE;

-- name: SetWorkspaceMembershipStatus :one
UPDATE workspace_memberships
SET status = sqlc.arg(status),
    suspended_at = CASE WHEN sqlc.arg(status)::text = 'suspended' THEN sqlc.arg(updated_at)::timestamptz ELSE NULL END,
    revoked_at = CASE WHEN sqlc.arg(status)::text = 'revoked' THEN sqlc.arg(updated_at)::timestamptz ELSE NULL END,
    updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(membership_id)
RETURNING *;

-- name: SetMembershipPrincipalStatus :execrows
UPDATE principals
SET status = sqlc.arg(status)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(principal_id);

-- name: WorkspaceExistsBySlug :one
SELECT id FROM workspaces WHERE slug = sqlc.arg(slug);

-- name: GetPendingBootstrapInvitation :one
SELECT * FROM workspace_invitations
WHERE workspace_id = sqlc.arg(workspace_id)
  AND issuer = sqlc.arg(issuer)
  AND email = sqlc.arg(email)
  AND role_id = 'workspace_admin'
  AND status = 'pending'
  AND expires_at > sqlc.arg(now)::timestamptz
ORDER BY created_at, id
LIMIT 1;

-- name: ExpireWorkspaceInvitations :execrows
UPDATE workspace_invitations
SET status = 'expired', updated_at = sqlc.arg(now)
WHERE workspace_id = sqlc.arg(workspace_id)
  AND status = 'pending'
  AND expires_at <= sqlc.arg(now)::timestamptz;
-- name: CreateClientCredential :exec
INSERT INTO client_credentials(id,workspace_id,consumer_id,binding_id,principal_id,name,token_prefix,verifier_digest,allowed_actions,scope_type,scope_id,issued_by_principal_id,issued_at,expires_at,rotated_from_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15);

-- name: LoadClientCredential :one
SELECT * FROM client_credentials WHERE id=$1;

-- name: ListClientCredentials :many
SELECT * FROM client_credentials WHERE workspace_id=$1 ORDER BY issued_at DESC,id DESC LIMIT 200;

-- name: RevokeClientCredential :execrows
UPDATE client_credentials SET revoked_at=COALESCE(revoked_at,$3),revoked_by_principal_id=COALESCE(revoked_by_principal_id,$4)
WHERE workspace_id=$1 AND id=$2;

-- name: TouchClientCredential :exec
UPDATE client_credentials SET last_used_at=$2 WHERE id=$1 AND (last_used_at IS NULL OR last_used_at<$2::timestamptz-interval '1 minute');
