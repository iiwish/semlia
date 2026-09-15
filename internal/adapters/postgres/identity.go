package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	identityapp "github.com/iiwish/semlia/internal/application/identity"
	authorization "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	publicid "github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ identityapp.Repository = (*Store)(nil)

func (store *Store) CreateLoginAttempt(ctx context.Context, attempt domain.LoginAttempt) error {
	if _, err := store.queries.DeleteExpiredOIDCLoginAttempts(ctx, timestamp(attempt.CreatedAt)); err != nil {
		return identityRepositoryError("clean OIDC login attempts", err)
	}
	err := store.queries.CreateOIDCLoginAttempt(ctx, dbgen.CreateOIDCLoginAttemptParams{
		StateDigest: attempt.StateDigest[:], NonceDigest: attempt.NonceDigest[:],
		VerifierEnvelope: attempt.VerifierEnvelope, ReturnTo: attempt.ReturnTo,
		ExpiresAt: timestamp(attempt.ExpiresAt), CreatedAt: timestamp(attempt.CreatedAt),
	})
	return identityRepositoryError("create OIDC login attempt", err)
}

func (store *Store) ConsumeLoginAttempt(ctx context.Context, digest [32]byte, now time.Time) (domain.LoginAttempt, error) {
	row, err := store.queries.ConsumeOIDCLoginAttempt(ctx, dbgen.ConsumeOIDCLoginAttemptParams{
		StateDigest: digest[:], ConsumedAt: timestamp(now),
	})
	if err != nil {
		return domain.LoginAttempt{}, identityRepositoryError("consume OIDC login attempt", err)
	}
	return loginAttemptFromRow(row)
}

func (store *Store) AdmitOIDCIdentity(ctx context.Context, claims domain.OIDCClaims, now time.Time) (domain.Account, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Account{}, fmt.Errorf("begin identity admission: %w", errRepositoryOperation)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)

	// Serialize admission for an issuer/subject pair. This prevents concurrent
	// callbacks from racing the unique identity and membership constraints.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", claims.Issuer+"::"+claims.Subject); err != nil {
		return domain.Account{}, identityRepositoryError("lock identity admission", err)
	}

	identityRow, identityErr := queries.GetExternalIdentityBySubject(ctx, dbgen.GetExternalIdentityBySubjectParams{
		Issuer: claims.Issuer, Subject: claims.Subject,
	})
	var account domain.Account
	newAccount := false
	if identityErr == nil {
		account, err = accountFromExternalIdentityRow(identityRow)
		if err != nil {
			return domain.Account{}, err
		}
		if identityRow.Status != string(domain.AccountActive) || account.Status != domain.AccountActive {
			return domain.Account{}, domain.ErrAdmissionDenied
		}
	} else if !errors.Is(identityErr, pgx.ErrNoRows) {
		return domain.Account{}, identityRepositoryError("load external identity", identityErr)
	}

	invitations, err := queries.ListMatchingPendingInvitations(ctx, dbgen.ListMatchingPendingInvitationsParams{
		Issuer: claims.Issuer, Now: timestamp(now), Subject: optionalTextValue(claims.Subject),
		EmailVerified: claims.EmailVerified, Email: optionalTextValue(claims.Email),
	})
	if err != nil {
		return domain.Account{}, identityRepositoryError("list matching invitations", err)
	}
	if identityErr != nil && len(invitations) == 0 {
		return domain.Account{}, domain.ErrAdmissionDenied
	}
	sort.Slice(invitations, func(left, right int) bool {
		return fmt.Sprintf("%x", invitations[left].WorkspaceID.Bytes) < fmt.Sprintf("%x", invitations[right].WorkspaceID.Bytes)
	})
	batchActions := make(map[[16]byte][]authorization.Action)
	for _, invitation := range invitations {
		reason, policyErr, validateErr := validateInvitationGrant(ctx, queries, invitation, account, identityErr == nil, now)
		if validateErr != nil {
			return domain.Account{}, validateErr
		}
		if policyErr == nil {
			actionRows, actionErr := queries.ListRoleActionsForRoles(ctx, []string{invitation.RoleID})
			if actionErr != nil {
				return domain.Account{}, identityRepositoryError("load batch invitation role actions", actionErr)
			}
			desired := make([]authorization.Action, 0, len(actionRows))
			for _, row := range actionRows {
				desired = append(desired, authorization.Action(row.Action))
			}
			workspaceKey := invitation.WorkspaceID.Bytes
			if authorization.ActionsConflict(batchActions[workspaceKey], desired) {
				reason, policyErr = "SEPARATION_OF_DUTIES", authorization.ErrSeparationOfDuties
			} else {
				batchActions[workspaceKey] = append(batchActions[workspaceKey], desired...)
				continue
			}
		}
		workspaceID, decodeErr := publicid.WorkspaceIDFromUUIDBytes(invitation.WorkspaceID.Bytes)
		if decodeErr != nil {
			return domain.Account{}, decodeErr
		}
		invitationID, decodeErr := publicid.InvitationIDFromUUIDBytes(invitation.ID.Bytes)
		if decodeErr != nil {
			return domain.Account{}, decodeErr
		}
		actor := "invitation"
		if invitation.CreatedByPrincipalID.Valid {
			if principalID, principalErr := publicid.PrincipalIDFromUUIDBytes(invitation.CreatedByPrincipalID.Bytes); principalErr == nil {
				actor = principalID.String()
			}
		}
		if err := createIdentityAudit(ctx, queries, workspaceID, actor, "authorization.policy_denied", map[string]string{
			"reason": reason, "action": string(authorization.ActionRoleAssign),
			"roleId": invitation.RoleID, "invitationId": invitationID.String(),
		}, now); err != nil {
			return domain.Account{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.Account{}, identityRepositoryError("commit invitation policy denial", err)
		}
		return domain.Account{}, policyErr
	}
	if identityErr != nil {
		accountID, createErr := publicid.NewUserAccountID()
		if createErr != nil {
			return domain.Account{}, createErr
		}
		externalID, createErr := publicid.NewExternalIdentityID()
		if createErr != nil {
			return domain.Account{}, createErr
		}
		accountDBID, _ := uuidValue(accountID)
		externalDBID, _ := uuidValue(externalID)
		created, createErr := queries.CreateUserAccount(ctx, dbgen.CreateUserAccountParams{
			ID: accountDBID, DisplayName: admissionDisplayName(claims), CreatedAt: timestamp(now),
		})
		if createErr != nil {
			return domain.Account{}, identityRepositoryError("create user account", createErr)
		}
		_, createErr = queries.CreateExternalIdentity(ctx, dbgen.CreateExternalIdentityParams{
			ID: externalDBID, UserAccountID: accountDBID, Issuer: claims.Issuer, Subject: claims.Subject,
			VerifiedEmail: optionalTextValue(claims.Email), CreatedAt: timestamp(now),
		})
		if createErr != nil {
			return domain.Account{}, identityRepositoryError("create external identity", createErr)
		}
		account, err = accountFromRow(created)
		if err != nil {
			return domain.Account{}, err
		}
		newAccount = true
	}

	accountID, _ := uuidValue(account.ID)
	for _, invitation := range invitations {
		if err := admitInvitation(ctx, queries, invitation, account, claims, now); err != nil {
			return domain.Account{}, err
		}
	}
	memberships, err := queries.ListAccountMemberships(ctx, accountID)
	if err != nil {
		return domain.Account{}, identityRepositoryError("verify admitted memberships", err)
	}
	if len(memberships) == 0 {
		if newAccount {
			return domain.Account{}, domain.ErrAdmissionDenied
		}
		return domain.Account{}, domain.ErrMembershipInactive
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Account{}, identityRepositoryError("commit identity admission", err)
	}
	return account, nil
}

func validateInvitationGrant(ctx context.Context, queries *dbgen.Queries, invitation dbgen.WorkspaceInvitation, account domain.Account, existingAccount bool, now time.Time) (string, error, error) {
	if _, err := queries.LockAuthorizationWorkspace(ctx, invitation.WorkspaceID); err != nil {
		return "", nil, identityRepositoryError("lock invitation admission workspace", err)
	}
	role, err := queries.GetAuthorizationRole(ctx, dbgen.GetAuthorizationRoleParams{RoleID: invitation.RoleID, WorkspaceID: invitation.WorkspaceID})
	if err != nil {
		return "STALE_INVITATION_ROLE", authorization.ErrVersionConflict, nil
	}
	if role.Version != invitation.RoleVersion {
		return "STALE_INVITATION_ROLE", authorization.ErrVersionConflict, nil
	}
	roleActionRows, err := queries.ListRoleActionsForRoles(ctx, []string{invitation.RoleID})
	if err != nil {
		return "", nil, identityRepositoryError("load invitation role actions", err)
	}
	desired := make([]authorization.Action, 0, len(roleActionRows))
	for _, row := range roleActionRows {
		desired = append(desired, authorization.Action(row.Action))
	}
	// Bootstrap invitations are the only grants without a human creator. They
	// are restricted to the immutable workspace-admin system role.
	if !invitation.CreatedByPrincipalID.Valid {
		if invitation.RoleID != "workspace_admin" || invitation.RoleVersion != 1 {
			return "HUMAN_GRANTOR_REQUIRED", authorization.ErrSeparationOfDuties, nil
		}
		return validateInvitationTargetDuties(ctx, queries, invitation, account, existingAccount, desired)
	}
	creator, err := queries.GetWorkspacePrincipal(ctx, dbgen.GetWorkspacePrincipalParams{
		WorkspaceID: invitation.WorkspaceID, PrincipalID: invitation.CreatedByPrincipalID,
	})
	if err != nil || creator.Kind != string(authorization.PrincipalHuman) || creator.Status != string(authorization.PrincipalActive) {
		return "HUMAN_GRANTOR_REQUIRED", authorization.ErrSeparationOfDuties, nil
	}
	granted, err := workspacePrincipalActions(ctx, queries, invitation.CreatedByPrincipalID, invitation.WorkspaceID)
	if err != nil {
		return "", nil, err
	}
	if _, allowed := granted[authorization.ActionRoleAssign]; !allowed {
		return "GRANTOR_NO_LONGER_AUTHORIZED", authorization.ErrAuthorizationCeiling, nil
	}
	for _, action := range desired {
		if _, allowed := granted[action]; !allowed {
			return "AUTHORIZATION_CEILING", authorization.ErrAuthorizationCeiling, nil
		}
	}
	return validateInvitationTargetDuties(ctx, queries, invitation, account, existingAccount, desired)
}

func validateInvitationTargetDuties(ctx context.Context, queries *dbgen.Queries, invitation dbgen.WorkspaceInvitation, account domain.Account, existingAccount bool, desired []authorization.Action) (string, error, error) {
	if !existingAccount {
		return "", nil, nil
	}
	accountID, err := uuidValue(account.ID)
	if err != nil {
		return "", nil, err
	}
	membership, err := queries.GetWorkspaceMembershipByAccount(ctx, dbgen.GetWorkspaceMembershipByAccountParams{
		WorkspaceID: invitation.WorkspaceID, UserAccountID: accountID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, identityRepositoryError("load invitation target membership", err)
	}
	target, err := queries.GetWorkspacePrincipal(ctx, dbgen.GetWorkspacePrincipalParams{
		WorkspaceID: invitation.WorkspaceID, PrincipalID: membership.PrincipalID,
	})
	if err != nil {
		return "", nil, identityRepositoryError("load invitation target principal", err)
	}
	if target.Kind != string(authorization.PrincipalHuman) {
		for _, action := range desired {
			if authorization.RequiresHuman(action) {
				return "HUMAN_ONLY_ACTION", authorization.ErrSeparationOfDuties, nil
			}
		}
	}
	bindings, err := invitationPrincipalBindings(ctx, queries, membership.PrincipalID)
	if err != nil {
		return "", nil, err
	}
	for _, binding := range bindings {
		if authorization.ActionsConflict(binding.Actions, desired) {
			return "SEPARATION_OF_DUTIES", authorization.ErrSeparationOfDuties, nil
		}
	}
	return "", nil, nil
}

func workspacePrincipalActions(ctx context.Context, queries *dbgen.Queries, principalID, workspaceID pgtype.UUID) (map[authorization.Action]struct{}, error) {
	bindings, err := invitationPrincipalBindings(ctx, queries, principalID)
	if err != nil {
		return nil, err
	}
	result := make(map[authorization.Action]struct{})
	workspace, err := publicid.WorkspaceIDFromUUIDBytes(workspaceID.Bytes)
	if err != nil {
		return nil, err
	}
	workspaceScope := workspace.UUID()
	for _, binding := range bindings {
		if binding.ScopeType != authorization.ScopeWorkspace || binding.ScopeID != workspaceScope {
			continue
		}
		for _, action := range binding.Actions {
			result[action] = struct{}{}
		}
	}
	return result, nil
}

func invitationPrincipalBindings(ctx context.Context, queries *dbgen.Queries, principalID pgtype.UUID) ([]authorization.RoleBinding, error) {
	rows, err := queries.ListPrincipalRoleBindings(ctx, principalID)
	if err != nil {
		return nil, identityRepositoryError("load invitation policy bindings", err)
	}
	actions := make(map[[16]byte][]authorization.Action)
	if len(rows) > 0 {
		actionRows, actionErr := queries.ListPrincipalRoleBindingActions(ctx, principalID)
		if actionErr != nil {
			return nil, identityRepositoryError("load invitation policy actions", actionErr)
		}
		for _, row := range actionRows {
			actions[row.BindingID.Bytes] = append(actions[row.BindingID.Bytes], authorization.Action(row.Action))
		}
	}
	result := make([]authorization.RoleBinding, 0, len(rows))
	for _, row := range rows {
		binding, decodeErr := roleBindingFromRow(row, actions[row.ID.Bytes])
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, binding)
	}
	return result, nil
}

func admitInvitation(ctx context.Context, queries *dbgen.Queries, invitation dbgen.WorkspaceInvitation, account domain.Account, claims domain.OIDCClaims, now time.Time) error {
	workspacePublicID, err := publicid.WorkspaceIDFromUUIDBytes(invitation.WorkspaceID.Bytes)
	if err != nil {
		return err
	}
	accountID, _ := uuidValue(account.ID)
	membership, err := queries.GetWorkspaceMembershipByAccount(ctx, dbgen.GetWorkspaceMembershipByAccountParams{
		WorkspaceID: invitation.WorkspaceID, UserAccountID: accountID,
	})
	principalID := membership.PrincipalID
	if errors.Is(err, pgx.ErrNoRows) {
		principal, createErr := publicid.NewPrincipalID()
		if createErr != nil {
			return createErr
		}
		membershipID, createErr := publicid.NewMembershipID()
		if createErr != nil {
			return createErr
		}
		principalID, _ = uuidValue(principal)
		membershipDBID, _ := uuidValue(membershipID)
		_, createErr = queries.CreatePrincipal(ctx, dbgen.CreatePrincipalParams{
			ID: principalID, WorkspaceID: invitation.WorkspaceID, Kind: "human",
			DisplayName: principalDisplayName(claims, account.ID), Status: "active",
		})
		if createErr != nil {
			return identityRepositoryError("create workspace principal", createErr)
		}
		membership, createErr = queries.CreateWorkspaceMembership(ctx, dbgen.CreateWorkspaceMembershipParams{
			ID: membershipDBID, WorkspaceID: invitation.WorkspaceID, UserAccountID: accountID,
			PrincipalID: principalID, AdmittedByPrincipalID: invitation.CreatedByPrincipalID,
			AdmittedAt: timestamp(now),
		})
		if createErr != nil {
			return identityRepositoryError("create workspace membership", createErr)
		}
	} else if err != nil {
		return identityRepositoryError("load workspace membership", err)
	} else if membership.Status != string(domain.MembershipActive) {
		if _, err := queries.SetMembershipPrincipalStatus(ctx, dbgen.SetMembershipPrincipalStatusParams{
			Status: "active", WorkspaceID: invitation.WorkspaceID, PrincipalID: principalID,
		}); err != nil {
			return identityRepositoryError("reactivate workspace principal", err)
		}
		if _, err := queries.SetWorkspaceMembershipStatus(ctx, dbgen.SetWorkspaceMembershipStatusParams{
			Status: string(domain.MembershipActive), UpdatedAt: timestamp(now),
			WorkspaceID: invitation.WorkspaceID, MembershipID: membership.ID,
		}); err != nil {
			return identityRepositoryError("reactivate workspace membership", err)
		}
	}
	bindingID, err := publicid.NewBindingID()
	if err != nil {
		return err
	}
	bindingDBID, _ := uuidValue(bindingID)
	if _, err := queries.ExpireAuthorizationRoleBindings(ctx, dbgen.ExpireAuthorizationRoleBindingsParams{
		WorkspaceID: invitation.WorkspaceID, ExpiredAt: timestamp(now),
	}); err != nil {
		return identityRepositoryError("materialize expired invitation role bindings", err)
	}
	inserted, err := queries.GrantWorkspaceRole(ctx, dbgen.GrantWorkspaceRoleParams{
		ID: bindingDBID, PrincipalID: principalID, RoleID: invitation.RoleID, RoleVersion: invitation.RoleVersion,
		WorkspaceID: invitation.WorkspaceID, GrantedBy: invitation.CreatedByPrincipalID, GrantedAt: timestamp(now),
	})
	if err != nil {
		return identityRepositoryError("grant invitation role", err)
	}
	if inserted != 1 {
		return domain.ErrConflict
	}
	if _, err := queries.AcceptWorkspaceInvitation(ctx, dbgen.AcceptWorkspaceInvitationParams{
		AccountID: accountID, AcceptedAt: timestamp(now), InvitationID: invitation.ID,
	}); err != nil {
		return identityRepositoryError("accept workspace invitation", err)
	}
	if _, err := queries.BumpWorkspaceAuthorizationVersion(ctx, invitation.WorkspaceID); err != nil {
		return identityRepositoryError("bump authorization version", err)
	}
	membershipPublicID, err := publicid.MembershipIDFromUUIDBytes(membership.ID.Bytes)
	if err != nil {
		return err
	}
	principalPublicID, err := publicid.PrincipalIDFromUUIDBytes(principalID.Bytes)
	if err != nil {
		return err
	}
	invitationPublicID, err := publicid.InvitationIDFromUUIDBytes(invitation.ID.Bytes)
	if err != nil {
		return err
	}
	if err := createIdentityAudit(ctx, queries, workspacePublicID, principalPublicID.String(), "identity.member.admitted", map[string]string{
		"accountId": account.ID.String(), "membershipId": membershipPublicID.String(), "invitationId": invitationPublicID.String(),
	}, now); err != nil {
		return err
	}
	return nil
}

func (store *Store) CreateSession(ctx context.Context, record identityapp.CreateSessionRecord) (domain.Session, error) {
	id, err := uuidValue(record.ID)
	if err != nil {
		return domain.Session{}, err
	}
	accountID, err := uuidValue(record.AccountID)
	if err != nil {
		return domain.Session{}, err
	}
	row, err := store.queries.CreateSession(ctx, dbgen.CreateSessionParams{
		ID: id, TokenDigest: record.TokenDigest[:], CsrfDigest: record.CSRFDigest[:],
		UserAccountID: accountID, IdleExpiresAt: timestamp(record.IdleExpiresAt),
		AbsoluteExpiresAt: timestamp(record.AbsoluteExpiresAt), LastSeenAt: timestamp(record.Now),
		CreatedAt: timestamp(record.Now),
	})
	if err != nil {
		return domain.Session{}, identityRepositoryError("create session", err)
	}
	return sessionFromCreatedRow(row, record.AccountID)
}

func (store *Store) LoadSession(ctx context.Context, digest [32]byte, now, idleExpiresAt time.Time) (domain.Session, error) {
	row, err := store.queries.LoadActiveSession(ctx, dbgen.LoadActiveSessionParams{TokenDigest: digest[:], Now: timestamp(now)})
	if err != nil {
		return domain.Session{}, identityRepositoryError("load session", err)
	}
	if _, err := store.queries.TouchSession(ctx, dbgen.TouchSessionParams{
		Now: timestamp(now), IdleExpiresAt: timestamp(idleExpiresAt), SessionID: row.ID,
	}); err != nil {
		return domain.Session{}, identityRepositoryError("touch session", err)
	}
	membershipRows, err := store.queries.ListAccountMemberships(ctx, row.UserAccountID)
	if err != nil {
		return domain.Session{}, identityRepositoryError("list session memberships", err)
	}
	memberships := make([]domain.Membership, 0, len(membershipRows))
	for _, membershipRow := range membershipRows {
		membership, decodeErr := accountMembershipFromRow(membershipRow)
		if decodeErr != nil {
			return domain.Session{}, decodeErr
		}
		memberships = append(memberships, membership)
	}
	return sessionFromLoadedRow(row, memberships)
}

func (store *Store) RevokeSession(ctx context.Context, digest [32]byte, now time.Time) error {
	_, err := store.queries.RevokeSessionByDigest(ctx, dbgen.RevokeSessionByDigestParams{
		RevokedAt: timestamp(now), TokenDigest: digest[:],
	})
	return identityRepositoryError("revoke session", err)
}

func (store *Store) ListWorkspaceMembers(ctx context.Context, workspace publicid.WorkspaceID) ([]domain.Membership, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, err
	}
	rows, err := store.queries.ListWorkspaceMemberships(ctx, workspaceID)
	if err != nil {
		return nil, identityRepositoryError("list workspace members", err)
	}
	items := make([]domain.Membership, 0, len(rows))
	for _, row := range rows {
		item, decodeErr := workspaceMembershipFromRow(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (store *Store) ListWorkspaceInvitations(ctx context.Context, workspace publicid.WorkspaceID) ([]domain.Invitation, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, err
	}
	rows, err := store.queries.ListWorkspaceInvitations(ctx, workspaceID)
	if err != nil {
		return nil, identityRepositoryError("list workspace invitations", err)
	}
	items := make([]domain.Invitation, 0, len(rows))
	for _, row := range rows {
		item, decodeErr := invitationFromRow(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (store *Store) CreateWorkspaceInvitation(ctx context.Context, record identityapp.CreateInvitationRecord) (domain.Invitation, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Invitation{}, fmt.Errorf("begin invitation creation: %w", errRepositoryOperation)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	id, _ := uuidValue(record.ID)
	workspaceID, _ := uuidValue(record.WorkspaceID)
	createdBy, _ := uuidValue(record.CreatedByPrincipalID)
	lockedVersion, err := queries.LockAuthorizationWorkspace(ctx, workspaceID)
	if err != nil {
		return domain.Invitation{}, identityRepositoryError("lock invitation workspace", err)
	}
	if lockedVersion != record.ExpectedMemberAuthorizationVersion || lockedVersion != record.ExpectedRoleAssignmentAuthorizationVersion {
		return domain.Invitation{}, authorization.ErrVersionConflict
	}
	role, err := queries.GetAuthorizationRole(ctx, dbgen.GetAuthorizationRoleParams{RoleID: record.RoleID, WorkspaceID: workspaceID})
	if err != nil {
		return domain.Invitation{}, identityRepositoryError("load invitation role", err)
	}
	if role.Version != record.RoleVersion {
		return domain.Invitation{}, authorization.ErrVersionConflict
	}
	creator, err := queries.GetWorkspacePrincipal(ctx, dbgen.GetWorkspacePrincipalParams{WorkspaceID: workspaceID, PrincipalID: createdBy})
	if err != nil || creator.Kind != string(authorization.PrincipalHuman) || creator.Status != string(authorization.PrincipalActive) {
		return domain.Invitation{}, authorization.ErrInvariant
	}
	row, err := queries.CreateWorkspaceInvitation(ctx, dbgen.CreateWorkspaceInvitationParams{
		ID: id, WorkspaceID: workspaceID, Issuer: record.Issuer,
		Subject: optionalTextValue(record.Subject), Email: optionalTextValue(record.Email), RoleID: record.RoleID, RoleVersion: record.RoleVersion,
		TokenDigest: record.TokenDigest[:], CreatedByPrincipalID: createdBy,
		ExpiresAt: timestamp(record.ExpiresAt), CreatedAt: timestamp(record.Now),
	})
	if err != nil {
		return domain.Invitation{}, identityRepositoryError("create workspace invitation", err)
	}
	if err := createIdentityAudit(ctx, queries, record.WorkspaceID, record.CreatedByPrincipalID.String(), "identity.invitation.created", map[string]string{
		"invitationId": record.ID.String(), "roleId": record.RoleID,
	}, record.Now); err != nil {
		return domain.Invitation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Invitation{}, identityRepositoryError("commit invitation creation", err)
	}
	return invitationFromRow(row)
}

func (store *Store) SetMembershipStatus(ctx context.Context, workspace publicid.WorkspaceID, actor publicid.PrincipalID, membership publicid.MembershipID, status domain.MembershipStatus, expectedAuthorizationVersion int64, now time.Time) (domain.Membership, error) {
	workspaceID, _ := uuidValue(workspace)
	membershipID, _ := uuidValue(membership)
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Membership{}, fmt.Errorf("begin membership update: %w", errRepositoryOperation)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	lockedVersion, err := queries.LockAuthorizationWorkspace(ctx, workspaceID)
	if err != nil {
		return domain.Membership{}, identityRepositoryError("lock workspace membership update", err)
	}
	if lockedVersion != expectedAuthorizationVersion {
		return domain.Membership{}, authorization.ErrVersionConflict
	}
	current, err := queries.GetWorkspaceMembershipForUpdate(ctx, dbgen.GetWorkspaceMembershipForUpdateParams{
		WorkspaceID: workspaceID, MembershipID: membershipID,
	})
	if err != nil {
		return domain.Membership{}, identityRepositoryError("load membership for update", err)
	}
	if current.Status == string(domain.MembershipActive) && status != domain.MembershipActive {
		admin, roleErr := queries.IsActiveWorkspaceAdministrator(ctx, dbgen.IsActiveWorkspaceAdministratorParams{
			WorkspaceID: workspaceID, PrincipalID: current.PrincipalID, Now: timestamp(now),
		})
		if roleErr != nil {
			return domain.Membership{}, identityRepositoryError("check administrator role", roleErr)
		}
		if admin {
			count, countErr := queries.CountActiveAuthorizationAdministrators(ctx, dbgen.CountActiveAuthorizationAdministratorsParams{WorkspaceID: workspaceID, Now: timestamp(now)})
			if countErr != nil {
				return domain.Membership{}, identityRepositoryError("count administrators", countErr)
			}
			if count <= 1 {
				return domain.Membership{}, domain.ErrFinalAdministrator
			}
		}
	}
	row, err := queries.SetWorkspaceMembershipStatus(ctx, dbgen.SetWorkspaceMembershipStatusParams{
		Status: string(status), UpdatedAt: timestamp(now), WorkspaceID: workspaceID, MembershipID: membershipID,
	})
	if err != nil {
		return domain.Membership{}, identityRepositoryError("set membership status", err)
	}
	if _, err := queries.SetMembershipPrincipalStatus(ctx, dbgen.SetMembershipPrincipalStatusParams{
		Status: string(status), WorkspaceID: workspaceID, PrincipalID: row.PrincipalID,
	}); err != nil {
		return domain.Membership{}, identityRepositoryError("set principal status", err)
	}
	if _, err := queries.BumpWorkspaceAuthorizationVersion(ctx, workspaceID); err != nil {
		return domain.Membership{}, identityRepositoryError("bump authorization version", err)
	}
	if err := createIdentityAudit(ctx, queries, workspace, actor.String(), "identity.membership.status_changed", map[string]string{
		"membershipId": membership.String(), "status": string(status),
	}, now); err != nil {
		return domain.Membership{}, err
	}
	details, err := queries.GetWorkspaceMembershipDetails(ctx, dbgen.GetWorkspaceMembershipDetailsParams{WorkspaceID: workspaceID, MembershipID: membershipID})
	if err != nil {
		return domain.Membership{}, identityRepositoryError("read updated membership", err)
	}
	result, err := workspaceMembershipFromRow(dbgen.ListWorkspaceMembershipsRow(details))
	if err != nil {
		return domain.Membership{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Membership{}, identityRepositoryError("commit membership update", err)
	}
	return result, nil
}

func (store *Store) BootstrapAdministrator(ctx context.Context, record identityapp.BootstrapRecord) (domain.Invitation, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Invitation{}, fmt.Errorf("begin administrator bootstrap: %w", errRepositoryOperation)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", "bootstrap::"+record.WorkspaceSlug); err != nil {
		return domain.Invitation{}, identityRepositoryError("lock administrator bootstrap", err)
	}
	workspaceID, err := queries.WorkspaceExistsBySlug(ctx, record.WorkspaceSlug)
	if errors.Is(err, pgx.ErrNoRows) {
		workspaceID, _ = uuidValue(record.WorkspaceID)
		if _, err := queries.CreateWorkspace(ctx, dbgen.CreateWorkspaceParams{
			ID: workspaceID, Slug: record.WorkspaceSlug, DisplayName: record.WorkspaceName,
		}); err != nil {
			return domain.Invitation{}, identityRepositoryError("create bootstrap workspace", err)
		}
	} else if err != nil {
		return domain.Invitation{}, identityRepositoryError("load bootstrap workspace", err)
	}
	if _, err := queries.ExpireWorkspaceInvitations(ctx, dbgen.ExpireWorkspaceInvitationsParams{
		Now: timestamp(record.Now), WorkspaceID: workspaceID,
	}); err != nil {
		return domain.Invitation{}, identityRepositoryError("expire bootstrap invitations", err)
	}
	existing, err := queries.GetPendingBootstrapInvitation(ctx, dbgen.GetPendingBootstrapInvitationParams{
		WorkspaceID: workspaceID, Issuer: record.Issuer, Email: optionalTextValue(record.Email), Now: timestamp(record.Now),
	})
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return domain.Invitation{}, identityRepositoryError("commit administrator bootstrap", err)
		}
		return invitationFromRow(existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Invitation{}, identityRepositoryError("load bootstrap invitation", err)
	}
	invitationID, _ := uuidValue(record.InvitationID)
	created, err := queries.CreateWorkspaceInvitation(ctx, dbgen.CreateWorkspaceInvitationParams{
		ID: invitationID, WorkspaceID: workspaceID, Issuer: record.Issuer,
		Email: optionalTextValue(record.Email), RoleID: "workspace_admin", RoleVersion: 1, TokenDigest: record.TokenDigest[:],
		ExpiresAt: timestamp(record.ExpiresAt), CreatedAt: timestamp(record.Now),
	})
	if err != nil {
		return domain.Invitation{}, identityRepositoryError("create bootstrap invitation", err)
	}
	workspacePublicID, err := publicid.WorkspaceIDFromUUIDBytes(workspaceID.Bytes)
	if err != nil {
		return domain.Invitation{}, err
	}
	invitationPublicID, err := publicid.InvitationIDFromUUIDBytes(created.ID.Bytes)
	if err != nil {
		return domain.Invitation{}, err
	}
	if err := createIdentityAudit(ctx, queries, workspacePublicID, "bootstrap", "identity.bootstrap.invited", map[string]string{
		"invitationId": invitationPublicID.String(), "roleId": "workspace_admin",
	}, record.Now); err != nil {
		return domain.Invitation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Invitation{}, identityRepositoryError("commit administrator bootstrap", err)
	}
	return invitationFromRow(created)
}

func loginAttemptFromRow(row dbgen.OidcLoginAttempt) (domain.LoginAttempt, error) {
	state, err := digest32(row.StateDigest)
	if err != nil {
		return domain.LoginAttempt{}, err
	}
	nonce, err := digest32(row.NonceDigest)
	if err != nil {
		return domain.LoginAttempt{}, err
	}
	return domain.LoginAttempt{StateDigest: state, NonceDigest: nonce, VerifierEnvelope: append([]byte(nil), row.VerifierEnvelope...), ReturnTo: row.ReturnTo, ExpiresAt: row.ExpiresAt.Time, ConsumedAt: optionalTime(row.ConsumedAt), CreatedAt: row.CreatedAt.Time}, nil
}

func accountFromRow(row dbgen.UserAccount) (domain.Account, error) {
	id, err := publicid.UserAccountIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.Account{}, err
	}
	return domain.Account{ID: id, DisplayName: row.DisplayName, Status: domain.AccountStatus(row.Status), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

func accountFromExternalIdentityRow(row dbgen.GetExternalIdentityBySubjectRow) (domain.Account, error) {
	id, err := publicid.UserAccountIDFromUUIDBytes(row.UserAccountID.Bytes)
	if err != nil {
		return domain.Account{}, err
	}
	return domain.Account{ID: id, DisplayName: row.DisplayName, Status: domain.AccountStatus(row.AccountStatus), CreatedAt: row.AccountCreatedAt.Time, UpdatedAt: row.AccountUpdatedAt.Time}, nil
}

func invitationFromRow(row dbgen.WorkspaceInvitation) (domain.Invitation, error) {
	id, err := publicid.InvitationIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.Invitation{}, err
	}
	workspaceID, err := publicid.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return domain.Invitation{}, err
	}
	result := domain.Invitation{ID: id, WorkspaceID: workspaceID, Issuer: row.Issuer, Subject: optionalText(row.Subject), Email: optionalText(row.Email), RoleID: row.RoleID, RoleVersion: row.RoleVersion, Status: domain.InvitationStatus(row.Status), ExpiresAt: row.ExpiresAt.Time, AcceptedAt: optionalTime(row.AcceptedAt), RevokedAt: optionalTime(row.RevokedAt), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
	if row.CreatedByPrincipalID.Valid {
		value, decodeErr := publicid.PrincipalIDFromUUIDBytes(row.CreatedByPrincipalID.Bytes)
		if decodeErr != nil {
			return domain.Invitation{}, decodeErr
		}
		result.CreatedByPrincipalID = &value
	}
	if row.AcceptedByAccountID.Valid {
		value, decodeErr := publicid.UserAccountIDFromUUIDBytes(row.AcceptedByAccountID.Bytes)
		if decodeErr != nil {
			return domain.Invitation{}, decodeErr
		}
		result.AcceptedByAccountID = &value
	}
	return result, nil
}

func sessionFromCreatedRow(row dbgen.Session, accountID publicid.UserAccountID) (domain.Session, error) {
	id, err := publicid.SessionIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.Session{}, err
	}
	csrf, err := digest32(row.CsrfDigest)
	if err != nil {
		return domain.Session{}, err
	}
	return domain.Session{ID: id, Account: domain.Account{ID: accountID}, CSRFDigest: csrf, IdleExpiresAt: row.IdleExpiresAt.Time, AbsoluteExpiresAt: row.AbsoluteExpiresAt.Time, LastSeenAt: row.LastSeenAt.Time, CreatedAt: row.CreatedAt.Time}, nil
}

func sessionFromLoadedRow(row dbgen.LoadActiveSessionRow, memberships []domain.Membership) (domain.Session, error) {
	id, err := publicid.SessionIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.Session{}, err
	}
	accountID, err := publicid.UserAccountIDFromUUIDBytes(row.UserAccountID.Bytes)
	if err != nil {
		return domain.Session{}, err
	}
	csrf, err := digest32(row.CsrfDigest)
	if err != nil {
		return domain.Session{}, err
	}
	return domain.Session{ID: id, Account: domain.Account{ID: accountID, DisplayName: row.DisplayName, Status: domain.AccountStatus(row.AccountStatus), CreatedAt: row.AccountCreatedAt.Time, UpdatedAt: row.AccountUpdatedAt.Time}, Memberships: memberships, CSRFDigest: csrf, IdleExpiresAt: row.IdleExpiresAt.Time, AbsoluteExpiresAt: row.AbsoluteExpiresAt.Time, LastSeenAt: row.LastSeenAt.Time, CreatedAt: row.CreatedAt.Time}, nil
}

func accountMembershipFromRow(row dbgen.ListAccountMembershipsRow) (domain.Membership, error) {
	return membershipValues(row.ID, row.WorkspaceID, row.UserAccountID, row.PrincipalID, row.Status, row.AdmittedAt, row.SuspendedAt, row.RevokedAt, row.CreatedAt, row.UpdatedAt, "", row.PrincipalDisplayName, row.WorkspaceSlug, row.WorkspaceDisplayName, row.RoleIds, row.AuthorizationVersion)
}

func workspaceMembershipFromRow(row dbgen.ListWorkspaceMembershipsRow) (domain.Membership, error) {
	return membershipValues(row.ID, row.WorkspaceID, row.UserAccountID, row.PrincipalID, row.Status, row.AdmittedAt, row.SuspendedAt, row.RevokedAt, row.CreatedAt, row.UpdatedAt, row.AccountDisplayName, row.PrincipalDisplayName, "", "", row.RoleIds, 0)
}

func membershipFromRow(row dbgen.WorkspaceMembership) (domain.Membership, error) {
	return membershipValues(row.ID, row.WorkspaceID, row.UserAccountID, row.PrincipalID, row.Status, row.AdmittedAt, row.SuspendedAt, row.RevokedAt, row.CreatedAt, row.UpdatedAt, "", "", "", "", nil, 0)
}

func membershipValues(idValue, workspaceValue, accountValue, principalValue pgtype.UUID, status string, admitted, suspended, revoked, created, updated pgtype.Timestamptz, accountName, principalName, workspaceSlug, workspaceName string, roles []string, authzVersion int64) (domain.Membership, error) {
	id, err := publicid.MembershipIDFromUUIDBytes(idValue.Bytes)
	if err != nil {
		return domain.Membership{}, err
	}
	workspaceID, err := publicid.WorkspaceIDFromUUIDBytes(workspaceValue.Bytes)
	if err != nil {
		return domain.Membership{}, err
	}
	accountID, err := publicid.UserAccountIDFromUUIDBytes(accountValue.Bytes)
	if err != nil {
		return domain.Membership{}, err
	}
	principalID, err := publicid.PrincipalIDFromUUIDBytes(principalValue.Bytes)
	if err != nil {
		return domain.Membership{}, err
	}
	return domain.Membership{ID: id, WorkspaceID: workspaceID, WorkspaceSlug: workspaceSlug, WorkspaceDisplayName: workspaceName, AccountID: accountID, AccountDisplayName: accountName, PrincipalID: principalID, PrincipalDisplayName: principalName, Status: domain.MembershipStatus(status), RoleIDs: append([]string(nil), roles...), AuthorizationVersion: authzVersion, AdmittedAt: admitted.Time, SuspendedAt: optionalTime(suspended), RevokedAt: optionalTime(revoked), CreatedAt: created.Time, UpdatedAt: updated.Time}, nil
}

func digest32(value []byte) ([32]byte, error) {
	var result [32]byte
	if len(value) != len(result) {
		return result, errors.New("invalid stored identity digest")
	}
	copy(result[:], value)
	return result, nil
}

func createIdentityAudit(ctx context.Context, queries *dbgen.Queries, workspace publicid.WorkspaceID, actor, eventType string, payloadValue map[string]string, now time.Time) error {
	eventID, err := publicid.NewEventID()
	if err != nil {
		return err
	}
	eventDBID, _ := uuidValue(eventID)
	workspaceDBID, _ := uuidValue(workspace)
	payload, err := json.Marshal(payloadValue)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(eventID.String()))
	traceID := hex.EncodeToString(digest[:16])
	if err := queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		ID: eventDBID, WorkspaceID: workspaceDBID, EventType: eventType,
		ActorID: optionalTextValue(actor), Payload: payload, TraceID: traceID, CreatedAt: timestamp(now),
	}); err != nil {
		return identityRepositoryError("record identity audit event", err)
	}
	return nil
}

func admissionDisplayName(claims domain.OIDCClaims) string {
	value := strings.TrimSpace(claims.DisplayName)
	if value == "" {
		value = claims.Email
	}
	if value == "" {
		value = claims.Subject
	}
	runes := []rune(value)
	if len(runes) > 256 {
		value = string(runes[:256])
	}
	return value
}

func principalDisplayName(claims domain.OIDCClaims, account publicid.UserAccountID) string {
	base := admissionDisplayName(claims)
	suffix := account.String()
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	maxBase := 256 - len(suffix) - 3
	runes := []rune(base)
	if len(runes) > maxBase {
		base = string(runes[:maxBase])
	}
	return base + " [" + suffix + "]"
}

func identityRepositoryError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, domain.ErrConflict)
		case "23503", "23514", "23502", "22001":
			return fmt.Errorf("%s: %w", operation, domain.ErrInvalidArgument)
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, domain.ErrNotFound)
	}
	return fmt.Errorf("%s: %w", operation, errRepositoryOperation)
}

func (store *Store) CreateMachinePrincipal(ctx context.Context, p authorization.Principal, version int64) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	workspace, _ := uuidValue(p.WorkspaceID)
	current, err := q.LockAuthorizationWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	if current != version {
		return authorization.ErrVersionConflict
	}
	principal, _ := uuidValue(p.ID)
	owner, _ := uuidValue(*p.OwnerPrincipalID)
	if _, err := q.CreatePrincipal(ctx, dbgen.CreatePrincipalParams{ID: principal, WorkspaceID: workspace, Kind: string(p.Kind), DisplayName: p.DisplayName, OwnerPrincipalID: owner, Status: string(p.Status)}); err != nil {
		return identityRepositoryError("create machine principal", err)
	}
	if _, err := q.BumpWorkspaceAuthorizationVersion(ctx, workspace); err != nil {
		return err
	}
	if err := createIdentityAudit(ctx, q, p.WorkspaceID, p.OwnerPrincipalID.String(), "identity.machine.created", map[string]string{"principalId": p.ID.String()}, p.CreatedAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (store *Store) SaveClientCredential(ctx context.Context, c domain.ClientCredential, version int64) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	workspace, _ := uuidValue(c.WorkspaceID)
	current, err := q.LockAuthorizationWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	if current != version {
		return authorization.ErrVersionConflict
	}
	credential, _ := uuidValue(c.ID)
	consumer, _ := uuidValue(c.ConsumerID)
	binding, _ := uuidValue(c.BindingID)
	principal, _ := uuidValue(c.PrincipalID)
	issuer, _ := uuidValue(c.IssuedBy)
	// Lock mutable identity dependencies until the credential and audit commit.
	var active bool
	err = tx.QueryRow(ctx, `SELECT p.kind='agent' AND p.status='active' AND c.status='active' AND b.status='active' AND (b.expires_at IS NULL OR b.expires_at>$5)
 FROM principals p JOIN consumers c ON c.workspace_id=p.workspace_id JOIN consumer_bindings b ON b.workspace_id=c.workspace_id AND b.consumer_id=c.id
 WHERE p.workspace_id=$1 AND p.id=$2 AND c.id=$3 AND b.id=$4 FOR SHARE OF p,c,b`, workspace, principal, consumer, binding, c.IssuedAt).Scan(&active)
	if err != nil {
		return identityRepositoryError("validate credential identity", err)
	}
	if !active {
		return domain.ErrForbidden
	}
	if _, err := tx.Exec(ctx, `INSERT INTO consumer_machine_principals(workspace_id,consumer_id,principal_id) VALUES($1,$2,$3) ON CONFLICT(workspace_id,consumer_id) DO NOTHING`, workspace, consumer, principal); err != nil {
		return identityRepositoryError("bind consumer principal", err)
	}
	rotated := pgtype.UUID{}
	if c.RotatedFromID != nil {
		rotated, _ = uuidValue(*c.RotatedFromID)
		var oldPrincipal, oldConsumer, oldBinding pgtype.UUID
		err := tx.QueryRow(ctx, `SELECT principal_id,consumer_id,binding_id FROM client_credentials WHERE workspace_id=$1 AND id=$2 AND revoked_at IS NULL AND expires_at>$3 FOR UPDATE`, workspace, rotated, c.IssuedAt).Scan(&oldPrincipal, &oldConsumer, &oldBinding)
		if err != nil {
			return identityRepositoryError("rotate credential", err)
		}
		if oldPrincipal != principal || oldConsumer != consumer || oldBinding != binding {
			return domain.ErrConflict
		}
		if _, err := q.RevokeClientCredential(ctx, dbgen.RevokeClientCredentialParams{WorkspaceID: workspace, ID: rotated, RevokedAt: timestamp(c.IssuedAt), RevokedByPrincipalID: issuer}); err != nil {
			return err
		}
	}
	actions := make([]string, 0, len(c.AllowedActions))
	for _, a := range c.AllowedActions {
		actions = append(actions, string(a))
	}
	if err := q.CreateClientCredential(ctx, dbgen.CreateClientCredentialParams{ID: credential, WorkspaceID: workspace, ConsumerID: consumer, BindingID: binding, PrincipalID: principal, Name: c.Name, TokenPrefix: c.TokenPrefix, VerifierDigest: c.VerifierDigest[:], AllowedActions: actions, ScopeType: string(c.ScopeType), ScopeID: c.ScopeID, IssuedByPrincipalID: issuer, IssuedAt: timestamp(c.IssuedAt), ExpiresAt: timestamp(c.ExpiresAt), RotatedFromID: rotated}); err != nil {
		return identityRepositoryError("save credential", err)
	}
	if err := createIdentityAudit(ctx, q, c.WorkspaceID, c.IssuedBy.String(), "identity.credential.issued", map[string]string{"credentialId": c.ID.String(), "principalId": c.PrincipalID.String(), "consumerId": c.ConsumerID.String()}, c.IssuedAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (store *Store) LoadClientCredential(ctx context.Context, id publicid.ClientCredentialID) (domain.ClientCredential, error) {
	value, _ := uuidValue(id)
	row, err := store.queries.LoadClientCredential(ctx, value)
	if err != nil {
		return domain.ClientCredential{}, identityRepositoryError("load credential", err)
	}
	return clientCredentialFromRow(row)
}
func (store *Store) ListClientCredentials(ctx context.Context, workspace publicid.WorkspaceID) ([]domain.ClientCredential, error) {
	value, _ := uuidValue(workspace)
	rows, err := store.queries.ListClientCredentials(ctx, value)
	if err != nil {
		return nil, identityRepositoryError("list credentials", err)
	}
	result := make([]domain.ClientCredential, 0, len(rows))
	for _, row := range rows {
		c, err := clientCredentialFromRow(row)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, nil
}
func (store *Store) RevokeClientCredential(ctx context.Context, workspace publicid.WorkspaceID, id publicid.ClientCredentialID, actor publicid.PrincipalID, now time.Time, version int64) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	workspaceValue, _ := uuidValue(workspace)
	value, _ := uuidValue(id)
	actorValue, _ := uuidValue(actor)
	current, err := q.LockAuthorizationWorkspace(ctx, workspaceValue)
	if err != nil {
		return err
	}
	if current != version {
		return authorization.ErrVersionConflict
	}
	count, err := q.RevokeClientCredential(ctx, dbgen.RevokeClientCredentialParams{WorkspaceID: workspaceValue, ID: value, RevokedAt: timestamp(now), RevokedByPrincipalID: actorValue})
	if err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrNotFound
	}
	if err := createIdentityAudit(ctx, q, workspace, actor.String(), "identity.credential.revoked", map[string]string{"credentialId": id.String()}, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (store *Store) TouchClientCredential(ctx context.Context, id publicid.ClientCredentialID, now time.Time) error {
	value, _ := uuidValue(id)
	return store.queries.TouchClientCredential(ctx, dbgen.TouchClientCredentialParams{ID: value, LastUsedAt: timestamp(now)})
}

func clientCredentialFromRow(row dbgen.ClientCredential) (domain.ClientCredential, error) {
	id, err := publicid.ClientCredentialIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.ClientCredential{}, err
	}
	workspace, err := publicid.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return domain.ClientCredential{}, err
	}
	consumer, err := publicid.ConsumerIDFromUUIDBytes(row.ConsumerID.Bytes)
	if err != nil {
		return domain.ClientCredential{}, err
	}
	binding, err := publicid.ConsumerBindingIDFromUUIDBytes(row.BindingID.Bytes)
	if err != nil {
		return domain.ClientCredential{}, err
	}
	principal, err := publicid.PrincipalIDFromUUIDBytes(row.PrincipalID.Bytes)
	if err != nil {
		return domain.ClientCredential{}, err
	}
	issuer, err := publicid.PrincipalIDFromUUIDBytes(row.IssuedByPrincipalID.Bytes)
	if err != nil {
		return domain.ClientCredential{}, err
	}
	digest, err := digest32(row.VerifierDigest)
	if err != nil {
		return domain.ClientCredential{}, err
	}
	c := domain.ClientCredential{ID: id, WorkspaceID: workspace, ConsumerID: consumer, BindingID: binding, PrincipalID: principal, Name: row.Name, TokenPrefix: row.TokenPrefix, VerifierDigest: digest, ScopeType: authorization.ScopeType(row.ScopeType), ScopeID: row.ScopeID, IssuedBy: issuer, IssuedAt: row.IssuedAt.Time, ExpiresAt: row.ExpiresAt.Time, RevokedAt: optionalTime(row.RevokedAt), LastUsedAt: optionalTime(row.LastUsedAt)}
	for _, action := range row.AllowedActions {
		c.AllowedActions = append(c.AllowedActions, authorization.Action(action))
	}
	if row.RotatedFromID.Valid {
		rotated, err := publicid.ClientCredentialIDFromUUIDBytes(row.RotatedFromID.Bytes)
		if err != nil {
			return c, err
		}
		c.RotatedFromID = &rotated
	}
	return c, nil
}
