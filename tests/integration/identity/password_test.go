package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	app "github.com/iiwish/semlia/internal/application/identity"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestLocalPasswordLifecycleAndStaleLogin(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := pgstore.NewStore(pool)
	authorizer := authapp.NewService(store, authapp.ClockFunc(time.Now))
	service, err := app.NewLocalService(store, bytes.Repeat([]byte{8}, 32), app.WithAuthorizer(authorizer))
	if err != nil {
		t.Fatal(err)
	}
	password := "Only synthetic test secret 9284"
	account, err := service.BootstrapLocalAdministrator(ctx, "password-test", "Password test", "local-password@example.com", password)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.BootstrapLocalAdministrator(ctx, "password-test", "Password test", "local-password@example.com", "different sufficiently long secret")
	if err != nil || again.ID != account.ID {
		t.Fatal("bootstrap not idempotent", err)
	}
	if _, err = service.PasswordLogin(ctx, "local-password@example.com", "wrong", "127.0.0.1"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatal("bad password admitted", err)
	}
	login, err := service.PasswordLogin(ctx, "LOCAL-PASSWORD@example.com", password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.Authenticate(ctx, login.SessionToken)
	if err != nil || len(session.Session.Memberships) != 1 {
		t.Fatal("missing membership", err)
	}
	membership := session.Session.Memberships[0]
	const traceID = "1234567890abcdef1234567890abcdef"
	member, err := service.CreatePasswordMember(ctx, membership.WorkspaceID, membership.PrincipalID, "local-member@example.com", "Local member", "Synthetic member password 7582", "consumer_developer", traceID)
	if err != nil || member.ID.IsZero() {
		t.Fatal("member creation failed", err)
	}
	memberLogin, err := service.PasswordLogin(ctx, "local-member@example.com", "Synthetic member password 7582", "127.0.0.8")
	if err != nil {
		t.Fatal(err)
	}
	memberSession, err := service.Authenticate(ctx, memberLogin.SessionToken)
	if err != nil || len(memberSession.Session.Memberships) != 1 {
		t.Fatal("member admission failed", err)
	}
	memberMembership := memberSession.Session.Memberships[0]
	if _, err := service.CreatePasswordMember(ctx, membership.WorkspaceID, memberMembership.PrincipalID, "forbidden-member@example.com", "Denied", "Synthetic member password 7582", "workspace_admin", traceID); err == nil {
		t.Fatal("consumer created an administrator")
	}
	if _, err := service.SetMembershipStatus(ctx, membership.WorkspaceID, membership.PrincipalID, memberMembership.ID, domain.MembershipSuspended, traceID); err != nil {
		t.Fatal("member suspension", err)
	}
	if suspended, err := service.Authenticate(ctx, memberLogin.SessionToken); err != nil || len(suspended.Session.Memberships) != 0 {
		t.Fatal("suspended workspace membership remains usable", err)
	}
	credential, err := store.LoadPasswordCredential(ctx, "local-password@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.ChangePassword(ctx, account.ID, password, "Replacement synthetic phrase 7429", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Authenticate(ctx, login.SessionToken); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatal("old session survives password change", err)
	}
	id, _ := identity.NewSessionID()
	now := time.Now().UTC()
	err = store.CreatePasswordSession(ctx, app.CreateSessionRecord{ID: id, AccountID: account.ID, TokenDigest: sha256.Sum256([]byte("stale")), CSRFDigest: sha256.Sum256([]byte("csrf")), Now: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour)}, credential.Version)
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatal("stale verified password minted session", err)
	}
	login, err = service.PasswordLogin(ctx, "local-password@example.com", "Replacement synthetic phrase 7429", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Logout(ctx, login.SessionToken); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Authenticate(ctx, login.SessionToken); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatal("logout did not revoke", err)
	}
	for i := 0; i < 5; i++ {
		_, _ = service.PasswordLogin(ctx, "missing-account@example.com", "incorrect", "127.0.0.2")
	}
	if _, err = service.PasswordLogin(ctx, "missing-account@example.com", "incorrect", "127.0.0.3"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatal("account budget not enforced across sources", err)
	}
	otherService, err := app.NewLocalService(pgstore.NewStore(pool), bytes.Repeat([]byte{8}, 32), app.WithAuthorizer(authorizer))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = otherService.PasswordLogin(ctx, "missing-account@example.com", "incorrect", "127.0.0.4"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatal("account budget not enforced across service instances", err)
	}
}
