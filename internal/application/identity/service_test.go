package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	identityapp "github.com/iiwish/semlia/internal/application/identity"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	publicid "github.com/iiwish/semlia/pkg/identity"
)

type providerStub struct {
	issuer   string
	nonce    string
	verifier string
	claims   domain.OIDCClaims
	err      error
}

func (stub *providerStub) Issuer() string { return stub.issuer }

func (stub *providerStub) AuthorizationURL(_, nonce, verifier string) string {
	stub.nonce = nonce
	stub.verifier = verifier
	return "https://identity.example/authorize"
}

func (stub *providerStub) ExchangeAndVerify(context.Context, string, string) (domain.OIDCClaims, error) {
	if stub.err != nil {
		return domain.OIDCClaims{}, stub.err
	}
	claims := stub.claims
	if claims.Nonce == "" {
		claims.Nonce = stub.nonce
	}
	return claims, nil
}

type repositoryStub struct {
	attempts       map[[32]byte]domain.LoginAttempt
	account        domain.Account
	createdSession identityapp.CreateSessionRecord
	session        domain.Session
	revoked        bool
	bootstrap      []identityapp.BootstrapRecord
}

func (stub *repositoryStub) CreateLoginAttempt(_ context.Context, attempt domain.LoginAttempt) error {
	if stub.attempts == nil {
		stub.attempts = map[[32]byte]domain.LoginAttempt{}
	}
	stub.attempts[attempt.StateDigest] = attempt
	return nil
}

func (stub *repositoryStub) ConsumeLoginAttempt(_ context.Context, digest [32]byte, now time.Time) (domain.LoginAttempt, error) {
	attempt, ok := stub.attempts[digest]
	if !ok || attempt.ConsumedAt != nil || !attempt.ExpiresAt.After(now) {
		return domain.LoginAttempt{}, domain.ErrNotFound
	}
	attempt.ConsumedAt = &now
	stub.attempts[digest] = attempt
	return attempt, nil
}

func (stub *repositoryStub) AdmitOIDCIdentity(context.Context, domain.OIDCClaims, time.Time) (domain.Account, error) {
	return stub.account, nil
}

func (stub *repositoryStub) CreateSession(_ context.Context, record identityapp.CreateSessionRecord) (domain.Session, error) {
	stub.createdSession = record
	stub.session = domain.Session{ID: record.ID, Account: stub.account, CSRFDigest: record.CSRFDigest, IdleExpiresAt: record.IdleExpiresAt, AbsoluteExpiresAt: record.AbsoluteExpiresAt, LastSeenAt: record.Now, CreatedAt: record.Now}
	return stub.session, nil
}

func (stub *repositoryStub) LoadSession(_ context.Context, digest [32]byte, _, _ time.Time) (domain.Session, error) {
	if stub.revoked || digest != stub.createdSession.TokenDigest {
		return domain.Session{}, domain.ErrNotFound
	}
	return stub.session, nil
}

func (stub *repositoryStub) RevokeSession(_ context.Context, digest [32]byte, _ time.Time) error {
	if digest == stub.createdSession.TokenDigest {
		stub.revoked = true
	}
	return nil
}

func (*repositoryStub) ListWorkspaceMembers(context.Context, publicid.WorkspaceID) ([]domain.Membership, error) {
	return nil, nil
}

func (*repositoryStub) ListWorkspaceInvitations(context.Context, publicid.WorkspaceID) ([]domain.Invitation, error) {
	return nil, nil
}

func (*repositoryStub) CreateWorkspaceInvitation(context.Context, identityapp.CreateInvitationRecord) (domain.Invitation, error) {
	return domain.Invitation{}, nil
}

func (*repositoryStub) SetMembershipStatus(context.Context, publicid.WorkspaceID, publicid.PrincipalID, publicid.MembershipID, domain.MembershipStatus, int64, time.Time) (domain.Membership, error) {
	return domain.Membership{}, nil
}

func (stub *repositoryStub) BootstrapAdministrator(_ context.Context, record identityapp.BootstrapRecord) (domain.Invitation, error) {
	stub.bootstrap = append(stub.bootstrap, record)
	return domain.Invitation{ID: record.InvitationID, WorkspaceID: record.WorkspaceID, Issuer: record.Issuer, Email: record.Email, RoleID: "workspace_admin", Status: domain.InvitationPending, ExpiresAt: record.ExpiresAt, CreatedAt: record.Now, UpdatedAt: record.Now}, nil
}

func TestOIDCTransactionIsOneTimeAndPersistsOnlyDigests(t *testing.T) {
	now := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	accountID, _ := publicid.NewUserAccountID()
	repository := &repositoryStub{account: domain.Account{ID: accountID, DisplayName: "Alpha User", Status: domain.AccountActive}}
	provider := &providerStub{issuer: "https://identity.example", claims: domain.OIDCClaims{Issuer: "https://identity.example", Subject: "subject-1", DisplayName: "Alpha User", Email: "USER@example.com", EmailVerified: true}}
	service, err := identityapp.NewService(repository, provider, bytes.Repeat([]byte{7}, 32), identityapp.WithClock(func() time.Time { return now }), identityapp.WithRandom(bytes.NewReader(bytes.Repeat([]byte{9}, 256))))
	if err != nil {
		t.Fatal(err)
	}

	started, err := service.BeginLogin(context.Background(), "/catalog")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := repository.attempts[sha256.Sum256([]byte(started.State))]; !exists {
		t.Fatal("login attempt was not stored by state digest")
	}
	completed, err := service.CompleteLogin(context.Background(), started.State, "authorization-code")
	if err != nil {
		t.Fatal(err)
	}
	if completed.ReturnTo != "/catalog" || completed.SessionToken == "" {
		t.Fatalf("login result = %+v", completed)
	}
	if repository.createdSession.TokenDigest != sha256.Sum256([]byte(completed.SessionToken)) {
		t.Fatal("session token was not persisted as a digest")
	}
	if _, err := service.CompleteLogin(context.Background(), started.State, "authorization-code"); !errors.Is(err, domain.ErrOIDCTransaction) {
		t.Fatalf("replayed callback error = %v", err)
	}

	authenticated, err := service.Authenticate(context.Background(), completed.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	if !service.ValidateCSRF(authenticated, authenticated.CSRFToken) || service.ValidateCSRF(authenticated, "wrong") {
		t.Fatal("CSRF verifier did not fail closed")
	}
	if err := service.Logout(context.Background(), completed.SessionToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), completed.SessionToken); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("revoked session error = %v", err)
	}
}

func TestOIDCNonceMismatchDoesNotCreateSession(t *testing.T) {
	accountID, _ := publicid.NewUserAccountID()
	repository := &repositoryStub{account: domain.Account{ID: accountID, Status: domain.AccountActive}}
	provider := &providerStub{issuer: "https://identity.example", claims: domain.OIDCClaims{Issuer: "https://identity.example", Subject: "subject-1", Nonce: "wrong"}}
	service, err := identityapp.NewService(repository, provider, bytes.Repeat([]byte{8}, 32), identityapp.WithRandom(bytes.NewReader(bytes.Repeat([]byte{5}, 128))))
	if err != nil {
		t.Fatal(err)
	}
	started, err := service.BeginLogin(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteLogin(context.Background(), started.State, "authorization-code"); !errors.Is(err, domain.ErrOIDCTransaction) {
		t.Fatalf("nonce mismatch error = %v", err)
	}
	if !repository.createdSession.ID.IsZero() {
		t.Fatal("nonce mismatch created a session")
	}
}

func TestBootstrapAdministratorTokenIsIdempotent(t *testing.T) {
	repository := &repositoryStub{}
	provider := &providerStub{issuer: "https://identity.example"}
	service, err := identityapp.NewService(repository, provider, bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.BootstrapAdministrator(context.Background(), "alpha", "Alpha", "Founder@Example.com")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.BootstrapAdministrator(context.Background(), "alpha", "Alpha", "founder@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if first.AcceptanceToken == "" || first.AcceptanceToken != second.AcceptanceToken {
		t.Fatalf("bootstrap tokens differ: %q != %q", first.AcceptanceToken, second.AcceptanceToken)
	}
	if len(repository.bootstrap) != 2 || repository.bootstrap[0].TokenDigest != repository.bootstrap[1].TokenDigest {
		t.Fatal("bootstrap invitation digest is not idempotent")
	}
}
