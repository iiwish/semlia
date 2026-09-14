package postgres

import (
	"context"
	"errors"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	identityapp "github.com/iiwish/semlia/internal/application/identity"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	publicid "github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (store *Store) LoadPasswordCredential(ctx context.Context, username string) (identityapp.PasswordCredential, error) {
	var c identityapp.PasswordCredential
	var id pgtype.UUID
	err := store.pool.QueryRow(ctx, `SELECT c.user_account_id,c.password_hash,c.credential_version FROM local_password_credentials c JOIN user_accounts a ON a.id=c.user_account_id WHERE c.username=$1 AND a.status='active'`, username).Scan(&id, &c.Hash, &c.Version)
	if err != nil {
		return c, identityRepositoryError("load local credential", err)
	}
	c.AccountID, err = publicid.UserAccountIDFromUUIDBytes(id.Bytes)
	return c, err
}

func (store *Store) LoadAccountPassword(ctx context.Context, account publicid.UserAccountID) (identityapp.PasswordCredential, error) {
	c := identityapp.PasswordCredential{AccountID: account}
	err := store.pool.QueryRow(ctx, `SELECT c.password_hash,c.credential_version FROM local_password_credentials c JOIN user_accounts a ON a.id=c.user_account_id WHERE c.user_account_id=$1 AND a.status='active'`, account.UUID()).Scan(&c.Hash, &c.Version)
	return c, identityRepositoryError("load account password", err)
}

func (store *Store) TakePasswordLoginBudget(ctx context.Context, account, source string, now time.Time) (bool, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Global admission bounds new budget rows even when identifiers are randomized.
	for _, bucket := range []struct {
		key   string
		limit int
		ttl   time.Duration
	}{{"global", 300, time.Minute}, {"source:" + source, 30, time.Minute}, {"account:" + account, 5, 5 * time.Minute}} {
		var attempts int
		err = tx.QueryRow(ctx, `INSERT INTO password_login_budgets(key,attempts,expires_at) VALUES($1,1,$3)
ON CONFLICT(key) DO UPDATE SET attempts=CASE WHEN password_login_budgets.expires_at<=$2 THEN 1 ELSE LEAST(password_login_budgets.attempts+1,1000000) END, expires_at=CASE WHEN password_login_budgets.expires_at<=$2 THEN $3 ELSE password_login_budgets.expires_at END RETURNING attempts`, bucket.key, now, now.Add(bucket.ttl)).Scan(&attempts)
		if err != nil {
			return false, identityRepositoryError("reserve login budget", err)
		}
		if attempts > bucket.limit {
			return false, tx.Commit(ctx)
		}
	}
	_, err = tx.Exec(ctx, `DELETE FROM password_login_budgets WHERE key IN (SELECT key FROM password_login_budgets WHERE expires_at<=$1 ORDER BY expires_at LIMIT 100)`, now)
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (store *Store) CreatePasswordSession(ctx context.Context, record identityapp.CreateSessionRecord, version int64) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current int64
	err = tx.QueryRow(ctx, `SELECT c.credential_version FROM user_accounts a JOIN local_password_credentials c ON c.user_account_id=a.id WHERE a.id=$1 AND a.status='active' FOR UPDATE OF a,c`, record.AccountID.UUID()).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && current != version {
		return domain.ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	id, _ := uuidValue(record.ID)
	account, _ := uuidValue(record.AccountID)
	_, err = dbgen.New(tx).CreateSession(ctx, dbgen.CreateSessionParams{ID: id, UserAccountID: account, TokenDigest: record.TokenDigest[:], CsrfDigest: record.CSRFDigest[:], IdleExpiresAt: timestamp(record.IdleExpiresAt), AbsoluteExpiresAt: timestamp(record.AbsoluteExpiresAt), LastSeenAt: timestamp(record.Now), CreatedAt: timestamp(record.Now)})
	if err != nil {
		return identityRepositoryError("create password session", err)
	}
	return tx.Commit(ctx)
}

func (store *Store) ReplacePassword(ctx context.Context, account publicid.UserAccountID, version int64, hash string, now time.Time) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current int64
	err = tx.QueryRow(ctx, `SELECT c.credential_version FROM user_accounts a JOIN local_password_credentials c ON c.user_account_id=a.id WHERE a.id=$1 AND a.status='active' FOR UPDATE OF a,c`, account.UUID()).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && current != version {
		return domain.ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE local_password_credentials SET password_hash=$2,credential_version=credential_version+1,updated_at=$3 WHERE user_account_id=$1`, account.UUID(), hash, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=$2,updated_at=$2 WHERE user_account_id=$1 AND revoked_at IS NULL`, account.UUID(), now); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT workspace_id,principal_id FROM workspace_memberships WHERE user_account_id=$1`, account.UUID())
	if err != nil {
		return err
	}
	type member struct {
		workspace pgtype.UUID
		principal pgtype.UUID
	}
	members := []member{}
	for rows.Next() {
		var m member
		if err = rows.Scan(&m.workspace, &m.principal); err != nil {
			rows.Close()
			return err
		}
		members = append(members, m)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, m := range members {
		w, e := publicid.WorkspaceIDFromUUIDBytes(m.workspace.Bytes)
		if e != nil {
			return e
		}
		principal, e := publicid.PrincipalIDFromUUIDBytes(m.principal.Bytes)
		if e != nil {
			return e
		}
		if e = createIdentityAudit(ctx, dbgen.New(tx), w, principal.String(), "identity.password.changed", map[string]string{"accountId": account.String()}, now); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

func (store *Store) CreateLocalAccount(ctx context.Context, record identityapp.LocalAccountRecord) (domain.Account, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return domain.Account{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "local-account:"+record.Username); err != nil {
		return domain.Account{}, err
	}
	workspace, _ := uuidValue(record.WorkspaceID)
	if record.Bootstrap {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "bootstrap::"+record.WorkspaceSlug); err != nil {
			return domain.Account{}, err
		}
		workspace, err = q.WorkspaceExistsBySlug(ctx, record.WorkspaceSlug)
		if errors.Is(err, pgx.ErrNoRows) {
			w, e := publicid.NewWorkspaceID()
			if e != nil {
				return domain.Account{}, e
			}
			workspace, _ = uuidValue(w)
			_, err = q.CreateWorkspace(ctx, dbgen.CreateWorkspaceParams{ID: workspace, Slug: record.WorkspaceSlug, DisplayName: record.WorkspaceName})
		}
		if err != nil {
			return domain.Account{}, err
		}
		record.WorkspaceID, err = publicid.WorkspaceIDFromUUIDBytes(workspace.Bytes)
		if err != nil {
			return domain.Account{}, err
		}
	}
	version, err := q.LockAuthorizationWorkspace(ctx, workspace)
	if err != nil {
		return domain.Account{}, err
	}
	if !record.Bootstrap && (record.Actor.IsZero() || version != record.AuthorizationVersion) {
		return domain.Account{}, authorization.ErrVersionConflict
	}
	role, err := q.GetAuthorizationRole(ctx, dbgen.GetAuthorizationRoleParams{RoleID: record.RoleID, WorkspaceID: workspace})
	if err != nil || role.Version != record.RoleVersion {
		return domain.Account{}, domain.ErrForbidden
	}
	if record.Bootstrap && (record.RoleID != "workspace_admin" || record.RoleVersion != 1) {
		return domain.Account{}, domain.ErrForbidden
	}
	var existing pgtype.UUID
	err = tx.QueryRow(ctx, `SELECT user_account_id FROM local_password_credentials WHERE username=$1`, record.Username).Scan(&existing)
	if err == nil {
		if !record.Bootstrap {
			return domain.Account{}, domain.ErrConflict
		}
		var valid bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workspace_memberships m JOIN user_accounts a ON a.id=m.user_account_id JOIN principals p ON p.id=m.principal_id AND p.workspace_id=m.workspace_id JOIN role_bindings b ON b.principal_id=p.id AND b.workspace_id=m.workspace_id WHERE m.workspace_id=$1 AND m.user_account_id=$2 AND m.status='active' AND a.status='active' AND p.status='active' AND b.role_id='workspace_admin' AND b.revoked_at IS NULL AND (b.expires_at IS NULL OR b.expires_at>$3))`, workspace, existing, record.Now).Scan(&valid)
		if err != nil {
			return domain.Account{}, err
		}
		if !valid {
			return domain.Account{}, domain.ErrConflict
		}
		var account domain.Account
		account.ID, err = publicid.UserAccountIDFromUUIDBytes(existing.Bytes)
		if err != nil {
			return account, err
		}
		account.DisplayName = record.DisplayName
		account.Status = domain.AccountActive
		return account, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, err
	}
	if record.Bootstrap {
		var admins int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM workspace_memberships m JOIN role_bindings b ON b.workspace_id=m.workspace_id AND b.principal_id=m.principal_id JOIN user_accounts a ON a.id=m.user_account_id WHERE m.workspace_id=$1 AND m.status='active' AND a.status='active' AND b.role_id='workspace_admin' AND b.revoked_at IS NULL AND (b.expires_at IS NULL OR b.expires_at>$2)`, workspace, record.Now).Scan(&admins)
		if err != nil {
			return domain.Account{}, err
		}
		if admins > 0 {
			return domain.Account{}, domain.ErrConflict
		}
	}
	accountID, err := publicid.NewUserAccountID()
	if err != nil {
		return domain.Account{}, err
	}
	accountValue, _ := uuidValue(accountID)
	accountRow, err := q.CreateUserAccount(ctx, dbgen.CreateUserAccountParams{ID: accountValue, DisplayName: record.DisplayName, CreatedAt: timestamp(record.Now)})
	if err != nil {
		return domain.Account{}, identityRepositoryError("create local account", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO local_password_credentials(user_account_id,username,password_hash,created_at,updated_at) VALUES($1,$2,$3,$4,$4)`, accountValue, record.Username, record.PasswordHash, record.Now)
	if err != nil {
		return domain.Account{}, identityRepositoryError("create local password", err)
	}
	principal, err := publicid.NewPrincipalID()
	if err != nil {
		return domain.Account{}, err
	}
	p, _ := uuidValue(principal)
	principalName := []rune(record.DisplayName)
	if len(principalName) > 245 {
		principalName = principalName[:245]
	}
	_, err = q.CreatePrincipal(ctx, dbgen.CreatePrincipalParams{ID: p, WorkspaceID: workspace, Kind: "human", DisplayName: string(principalName) + " [" + accountID.String()[len(accountID.String())-8:] + "]", Status: "active"})
	if err != nil {
		return domain.Account{}, err
	}
	membership, err := publicid.NewMembershipID()
	if err != nil {
		return domain.Account{}, err
	}
	m, _ := uuidValue(membership)
	actor := pgtype.UUID{}
	actorLabel := "bootstrap"
	if !record.Actor.IsZero() {
		actor, _ = uuidValue(record.Actor)
		actorLabel = record.Actor.String()
	}
	_, err = q.CreateWorkspaceMembership(ctx, dbgen.CreateWorkspaceMembershipParams{ID: m, WorkspaceID: workspace, UserAccountID: accountValue, PrincipalID: p, AdmittedByPrincipalID: actor, AdmittedAt: timestamp(record.Now)})
	if err != nil {
		return domain.Account{}, err
	}
	binding, err := publicid.NewBindingID()
	if err != nil {
		return domain.Account{}, err
	}
	b, _ := uuidValue(binding)
	_, err = q.GrantWorkspaceRole(ctx, dbgen.GrantWorkspaceRoleParams{ID: b, WorkspaceID: workspace, PrincipalID: p, RoleID: record.RoleID, RoleVersion: record.RoleVersion, GrantedBy: actor, GrantedAt: timestamp(record.Now)})
	if err != nil {
		return domain.Account{}, err
	}
	if _, err = q.BumpWorkspaceAuthorizationVersion(ctx, workspace); err != nil {
		return domain.Account{}, err
	}
	if err = createIdentityAudit(ctx, q, record.WorkspaceID, actorLabel, "identity.local_account.created", map[string]string{"accountId": accountID.String(), "principalId": principal.String(), "roleId": record.RoleID}, record.Now); err != nil {
		return domain.Account{}, err
	}
	account, err := accountFromRow(accountRow)
	if err != nil {
		return domain.Account{}, err
	}
	return account, tx.Commit(ctx)
}
