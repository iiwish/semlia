// Package identity implements OIDC login, opaque sessions and controlled
// workspace admission.
package identity

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"crypto/hkdf"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	publicid "github.com/iiwish/semlia/pkg/identity"
)

const (
	defaultLoginTTL       = 10 * time.Minute
	defaultSessionIdleTTL = 8 * time.Hour
	defaultSessionTTL     = 24 * time.Hour
	defaultInvitationTTL  = 7 * 24 * time.Hour
	protectorVersion      = byte(1)
)

type Clock func() time.Time

type OIDCProvider interface {
	Issuer() string
	AuthorizationURL(state, nonce, verifier string) string
	ExchangeAndVerify(context.Context, string, string) (domain.OIDCClaims, error)
}

type Protector interface {
	Seal(plaintext, associatedData []byte) ([]byte, error)
	Open(envelope, associatedData []byte) ([]byte, error)
}

type Repository interface {
	CreateLoginAttempt(context.Context, domain.LoginAttempt) error
	ConsumeLoginAttempt(context.Context, [32]byte, time.Time) (domain.LoginAttempt, error)
	AdmitOIDCIdentity(context.Context, domain.OIDCClaims, time.Time) (domain.Account, error)
	CreateSession(context.Context, CreateSessionRecord) (domain.Session, error)
	LoadSession(context.Context, [32]byte, time.Time, time.Time) (domain.Session, error)
	RevokeSession(context.Context, [32]byte, time.Time) error
	ListWorkspaceMembers(context.Context, publicid.WorkspaceID) ([]domain.Membership, error)
	ListWorkspaceInvitations(context.Context, publicid.WorkspaceID) ([]domain.Invitation, error)
	CreateWorkspaceInvitation(context.Context, CreateInvitationRecord) (domain.Invitation, error)
	SetMembershipStatus(context.Context, publicid.WorkspaceID, publicid.PrincipalID, publicid.MembershipID, domain.MembershipStatus, int64, time.Time) (domain.Membership, error)
	BootstrapAdministrator(context.Context, BootstrapRecord) (domain.Invitation, error)
}

type Authorizer interface {
	Evaluate(context.Context, authorizationapp.EvaluationRequest) (authorization.Decision, error)
	PrepareInvitationGrant(context.Context, authorizationapp.AccessRequest, string) (authorizationapp.InvitationGrantPreparation, error)
	RecordPolicyDenial(context.Context, authorizationapp.PolicyDenial) error
}

type CreateSessionRecord struct {
	ID                publicid.SessionID
	AccountID         publicid.UserAccountID
	TokenDigest       [32]byte
	CSRFDigest        [32]byte
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
	Now               time.Time
}

type CreateInvitationRecord struct {
	ID                                         publicid.InvitationID
	WorkspaceID                                publicid.WorkspaceID
	Issuer                                     string
	Subject                                    string
	Email                                      string
	RoleID                                     string
	RoleVersion                                int64
	ExpectedMemberAuthorizationVersion         int64
	ExpectedRoleAssignmentAuthorizationVersion int64
	TokenDigest                                [32]byte
	CreatedByPrincipalID                       publicid.PrincipalID
	ExpiresAt                                  time.Time
	Now                                        time.Time
}

type BootstrapRecord struct {
	WorkspaceID   publicid.WorkspaceID
	WorkspaceSlug string
	WorkspaceName string
	InvitationID  publicid.InvitationID
	Issuer        string
	Email         string
	TokenDigest   [32]byte
	ExpiresAt     time.Time
	Now           time.Time
}

type LoginStart struct {
	AuthorizationURL string
	State            string
}

type LoginComplete struct {
	SessionToken string
	ReturnTo     string
	ExpiresAt    time.Time
}

type AuthenticatedSession struct {
	Session   domain.Session
	CSRFToken string
}

type InvitationInput struct {
	Subject string
	Email   string
	RoleID  string
	TTL     time.Duration
}

type IssuedInvitation struct {
	Invitation      domain.Invitation
	AcceptanceToken string
}

type Service struct {
	repository     Repository
	provider       OIDCProvider
	protector      Protector
	authorizer     Authorizer
	clock          Clock
	random         io.Reader
	csrfKey        []byte
	bootstrapKey   []byte
	loginTTL       time.Duration
	sessionIdleTTL time.Duration
	sessionTTL     time.Duration
	invitationTTL  time.Duration
	passwords      PasswordRepository
	dummyHash      string
}

type Option func(*Service)

func WithClock(clock Clock) Option { return func(service *Service) { service.clock = clock } }

func WithRandom(reader io.Reader) Option { return func(service *Service) { service.random = reader } }

func WithAuthorizer(authorizer Authorizer) Option {
	return func(service *Service) { service.authorizer = authorizer }
}

func WithSessionDurations(idle, absolute time.Duration) Option {
	return func(service *Service) {
		service.sessionIdleTTL = idle
		service.sessionTTL = absolute
	}
}

func NewService(repository Repository, provider OIDCProvider, rootSecret []byte, options ...Option) (*Service, error) {
	if repository == nil || provider == nil || len(rootSecret) < 32 {
		return nil, errors.New("identity repository, OIDC provider and 32-byte root secret are required")
	}
	service, err := newSessionService(repository, provider, rootSecret, options...)
	if err != nil {
		return nil, err
	}
	if passwords, ok := repository.(PasswordRepository); ok {
		service.passwords = passwords
		service.dummyHash, err = HashPassword("Unusable random-account placeholder " + hex.EncodeToString(service.csrfKey))
	}
	return service, err
}

func newSessionService(repository Repository, provider OIDCProvider, rootSecret []byte, options ...Option) (*Service, error) {
	protector, err := NewAESProtector(rootSecret)
	if err != nil {
		return nil, err
	}
	csrfKey, err := deriveKey(rootSecret, "semlia/alpha/csrf/v1")
	if err != nil {
		return nil, err
	}
	bootstrapKey, err := deriveKey(rootSecret, "semlia/alpha/bootstrap/v1")
	if err != nil {
		return nil, err
	}
	service := &Service{
		repository: repository, provider: provider, protector: protector,
		clock: time.Now, random: rand.Reader, csrfKey: csrfKey, bootstrapKey: bootstrapKey,
		loginTTL: defaultLoginTTL, sessionIdleTTL: defaultSessionIdleTTL,
		sessionTTL: defaultSessionTTL, invitationTTL: defaultInvitationTTL,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	if service.clock == nil || service.random == nil || service.sessionIdleTTL <= 0 || service.sessionTTL < service.sessionIdleTTL {
		return nil, errors.New("invalid identity service options")
	}
	return service, nil
}

func (service *Service) BeginLogin(ctx context.Context, returnTo string) (LoginStart, error) {
	if service.provider == nil {
		return LoginStart{}, domain.ErrForbidden
	}
	if !domain.ValidReturnTo(returnTo) {
		return LoginStart{}, domain.ErrInvalidArgument
	}
	if returnTo == "" {
		returnTo = "/"
	}
	state, err := service.token(32)
	if err != nil {
		return LoginStart{}, err
	}
	nonce, err := service.token(32)
	if err != nil {
		return LoginStart{}, err
	}
	verifier, err := service.token(32)
	if err != nil {
		return LoginStart{}, err
	}
	stateDigest := sha256.Sum256([]byte(state))
	nonceDigest := sha256.Sum256([]byte(nonce))
	envelope, err := service.protector.Seal([]byte(verifier), stateDigest[:])
	if err != nil {
		return LoginStart{}, err
	}
	now := service.clock().UTC()
	if err := service.repository.CreateLoginAttempt(ctx, domain.LoginAttempt{
		StateDigest: stateDigest, NonceDigest: nonceDigest, VerifierEnvelope: envelope,
		ReturnTo: returnTo, ExpiresAt: now.Add(service.loginTTL), CreatedAt: now,
	}); err != nil {
		return LoginStart{}, err
	}
	return LoginStart{AuthorizationURL: service.provider.AuthorizationURL(state, nonce, verifier), State: state}, nil
}

func (service *Service) CompleteLogin(ctx context.Context, state, code string) (LoginComplete, error) {
	if service.provider == nil {
		return LoginComplete{}, domain.ErrForbidden
	}
	if strings.TrimSpace(state) == "" || strings.TrimSpace(code) == "" {
		return LoginComplete{}, domain.ErrOIDCTransaction
	}
	now := service.clock().UTC()
	stateDigest := sha256.Sum256([]byte(state))
	attempt, err := service.repository.ConsumeLoginAttempt(ctx, stateDigest, now)
	if err != nil {
		return LoginComplete{}, normalizeTransactionError(err)
	}
	verifier, err := service.protector.Open(attempt.VerifierEnvelope, stateDigest[:])
	if err != nil {
		return LoginComplete{}, domain.ErrOIDCTransaction
	}
	claims, err := service.provider.ExchangeAndVerify(ctx, code, string(verifier))
	clear(verifier)
	if err != nil {
		return LoginComplete{}, fmt.Errorf("%w: provider verification failed", domain.ErrOIDCTransaction)
	}
	issuer, err := domain.NormalizeIssuer(claims.Issuer)
	if err != nil || issuer != service.provider.Issuer() || strings.TrimSpace(claims.Subject) == "" {
		return LoginComplete{}, domain.ErrOIDCTransaction
	}
	nonceDigest := sha256.Sum256([]byte(claims.Nonce))
	if subtle.ConstantTimeCompare(nonceDigest[:], attempt.NonceDigest[:]) != 1 {
		return LoginComplete{}, domain.ErrOIDCTransaction
	}
	claims.Issuer = issuer
	if claims.EmailVerified {
		claims.Email, err = domain.NormalizeEmail(claims.Email)
		if err != nil {
			return LoginComplete{}, domain.ErrOIDCTransaction
		}
	} else {
		claims.Email = ""
	}
	account, err := service.repository.AdmitOIDCIdentity(ctx, claims, now)
	if err != nil {
		return LoginComplete{}, err
	}
	token, err := service.token(32)
	if err != nil {
		return LoginComplete{}, err
	}
	csrf := service.csrfToken(token)
	tokenDigest := sha256.Sum256([]byte(token))
	csrfDigest := sha256.Sum256([]byte(csrf))
	sessionID, err := publicid.NewSessionID()
	if err != nil {
		return LoginComplete{}, err
	}
	absoluteExpiry := now.Add(service.sessionTTL)
	_, err = service.repository.CreateSession(ctx, CreateSessionRecord{
		ID: sessionID, AccountID: account.ID, TokenDigest: tokenDigest, CSRFDigest: csrfDigest,
		IdleExpiresAt: now.Add(service.sessionIdleTTL), AbsoluteExpiresAt: absoluteExpiry, Now: now,
	})
	if err != nil {
		return LoginComplete{}, err
	}
	return LoginComplete{SessionToken: token, ReturnTo: attempt.ReturnTo, ExpiresAt: absoluteExpiry}, nil
}

func (service *Service) Authenticate(ctx context.Context, token string) (AuthenticatedSession, error) {
	if strings.TrimSpace(token) == "" {
		return AuthenticatedSession{}, domain.ErrUnauthenticated
	}
	now := service.clock().UTC()
	tokenDigest := sha256.Sum256([]byte(token))
	session, err := service.repository.LoadSession(ctx, tokenDigest, now, now.Add(service.sessionIdleTTL))
	if err != nil {
		return AuthenticatedSession{}, normalizeSessionError(err)
	}
	csrf := service.csrfToken(token)
	digest := sha256.Sum256([]byte(csrf))
	if subtle.ConstantTimeCompare(digest[:], session.CSRFDigest[:]) != 1 {
		return AuthenticatedSession{}, domain.ErrUnauthenticated
	}
	return AuthenticatedSession{Session: session, CSRFToken: csrf}, nil
}

func (service *Service) ValidateCSRF(authenticated AuthenticatedSession, presented string) bool {
	digest := sha256.Sum256([]byte(presented))
	return presented != "" && subtle.ConstantTimeCompare(digest[:], authenticated.Session.CSRFDigest[:]) == 1
}

func (service *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	digest := sha256.Sum256([]byte(token))
	return service.repository.RevokeSession(ctx, digest, service.clock().UTC())
}

func (service *Service) ListMembers(ctx context.Context, workspace publicid.WorkspaceID, principalRef, traceID string) ([]domain.Membership, error) {
	if _, err := service.authorize(ctx, workspace, principalRef, authorization.ActionMemberRead, traceID); err != nil {
		return nil, err
	}
	return service.repository.ListWorkspaceMembers(ctx, workspace)
}

func (service *Service) ListInvitations(ctx context.Context, workspace publicid.WorkspaceID, principalRef, traceID string) ([]domain.Invitation, error) {
	if _, err := service.authorize(ctx, workspace, principalRef, authorization.ActionMemberRead, traceID); err != nil {
		return nil, err
	}
	return service.repository.ListWorkspaceInvitations(ctx, workspace)
}

func (service *Service) Invite(ctx context.Context, workspace publicid.WorkspaceID, principal publicid.PrincipalID, input InvitationInput, traceID string) (IssuedInvitation, error) {
	if service.provider == nil {
		return IssuedInvitation{}, domain.ErrForbidden
	}
	memberDecision, err := service.authorize(ctx, workspace, principal.String(), authorization.ActionMemberManage, traceID)
	if err != nil {
		return IssuedInvitation{}, err
	}
	subject := strings.TrimSpace(input.Subject)
	email, err := domain.NormalizeEmail(input.Email)
	if err != nil || (subject == "" && email == "") || strings.TrimSpace(input.RoleID) == "" {
		return IssuedInvitation{}, domain.ErrInvalidArgument
	}
	invitationID, err := publicid.NewInvitationID()
	if err != nil {
		return IssuedInvitation{}, err
	}
	token, err := service.token(32)
	if err != nil {
		return IssuedInvitation{}, err
	}
	digest := sha256.Sum256([]byte(token))
	ttl := input.TTL
	if ttl <= 0 {
		ttl = service.invitationTTL
	}
	now := service.clock().UTC()
	if service.authorizer == nil {
		return IssuedInvitation{}, domain.ErrForbidden
	}
	preparation, err := service.authorizer.PrepareInvitationGrant(ctx, authorizationapp.AccessRequest{
		WorkspaceID: workspace, PrincipalRef: principal.String(), TraceID: traceID,
	}, strings.TrimSpace(input.RoleID))
	if err != nil {
		return IssuedInvitation{}, err
	}
	if memberDecision.AuthorizationVersion != preparation.AuthorizationVersion {
		return IssuedInvitation{}, authorization.ErrVersionConflict
	}
	invitation, err := service.repository.CreateWorkspaceInvitation(ctx, CreateInvitationRecord{
		ID: invitationID, WorkspaceID: workspace, Issuer: service.provider.Issuer(), Subject: subject,
		Email: email, RoleID: strings.TrimSpace(input.RoleID), RoleVersion: preparation.RoleVersion,
		ExpectedMemberAuthorizationVersion:         memberDecision.AuthorizationVersion,
		ExpectedRoleAssignmentAuthorizationVersion: preparation.AuthorizationVersion, TokenDigest: digest,
		CreatedByPrincipalID: principal, ExpiresAt: now.Add(ttl), Now: now,
	})
	if err != nil {
		return IssuedInvitation{}, err
	}
	return IssuedInvitation{Invitation: invitation, AcceptanceToken: token}, nil
}

func (service *Service) SetMembershipStatus(ctx context.Context, workspace publicid.WorkspaceID, principal publicid.PrincipalID, membership publicid.MembershipID, status domain.MembershipStatus, traceID string) (domain.Membership, error) {
	if status != domain.MembershipActive && status != domain.MembershipSuspended && status != domain.MembershipRevoked {
		return domain.Membership{}, domain.ErrInvalidArgument
	}
	decision, err := service.authorize(ctx, workspace, principal.String(), authorization.ActionMemberManage, traceID)
	if err != nil {
		return domain.Membership{}, err
	}
	updated, err := service.repository.SetMembershipStatus(ctx, workspace, principal, membership, status, decision.AuthorizationVersion, service.clock().UTC())
	if errors.Is(err, domain.ErrFinalAdministrator) && service.authorizer != nil {
		auditErr := service.authorizer.RecordPolicyDenial(ctx, authorizationapp.PolicyDenial{
			WorkspaceID: workspace, ActorID: principal, TraceID: traceID, OccurredAt: service.clock().UTC(),
			Reason: "FINAL_ACTIVE_ADMIN", Action: authorization.ActionMemberManage,
		})
		if auditErr != nil {
			err = errors.Join(err, auditErr)
		}
	}
	return updated, err
}

func (service *Service) BootstrapAdministrator(ctx context.Context, slug, name, email string) (IssuedInvitation, error) {
	if service.provider == nil {
		return IssuedInvitation{}, domain.ErrForbidden
	}
	issuer, err := domain.NormalizeIssuer(service.provider.Issuer())
	if err != nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(name) == "" {
		return IssuedInvitation{}, domain.ErrInvalidArgument
	}
	email, err = domain.NormalizeEmail(email)
	if err != nil || email == "" {
		return IssuedInvitation{}, domain.ErrInvalidArgument
	}
	workspaceID, err := publicid.NewWorkspaceID()
	if err != nil {
		return IssuedInvitation{}, err
	}
	invitationID, err := publicid.NewInvitationID()
	if err != nil {
		return IssuedInvitation{}, err
	}
	token := service.bootstrapToken(issuer, strings.TrimSpace(slug), email)
	digest := sha256.Sum256([]byte(token))
	now := service.clock().UTC()
	invitation, err := service.repository.BootstrapAdministrator(ctx, BootstrapRecord{
		WorkspaceID: workspaceID, WorkspaceSlug: strings.TrimSpace(slug), WorkspaceName: strings.TrimSpace(name),
		InvitationID: invitationID, Issuer: issuer, Email: email, TokenDigest: digest,
		ExpiresAt: now.Add(service.invitationTTL), Now: now,
	})
	if err != nil {
		return IssuedInvitation{}, err
	}
	return IssuedInvitation{Invitation: invitation, AcceptanceToken: token}, nil
}

func (service *Service) authorize(ctx context.Context, workspace publicid.WorkspaceID, principalRef string, action authorization.Action, traceID string) (authorization.Decision, error) {
	if service.authorizer == nil {
		return authorization.Decision{}, domain.ErrForbidden
	}
	decision, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{
		WorkspaceID: workspace, PrincipalRef: principalRef, Action: action,
		Resource: authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}, TraceID: traceID,
	})
	if err != nil {
		return authorization.Decision{}, err
	}
	if !decision.Allowed {
		return decision, domain.ErrForbidden
	}
	return decision, nil
}

func (service *Service) token(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := io.ReadFull(service.random, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (service *Service) csrfToken(sessionToken string) string {
	mac := hmac.New(sha256.New, service.csrfKey)
	_, _ = mac.Write([]byte(sessionToken))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (service *Service) bootstrapToken(issuer, slug, email string) string {
	mac := hmac.New(sha256.New, service.bootstrapKey)
	_, _ = mac.Write([]byte(issuer))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(slug))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(email))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type aesProtector struct{ aead cipher.AEAD }

func NewAESProtector(rootSecret []byte) (Protector, error) {
	key, err := deriveKey(rootSecret, "semlia/alpha/oidc-login-verifier/v1")
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &aesProtector{aead: aead}, nil
}

func (protector *aesProtector) Seal(plaintext, associatedData []byte) ([]byte, error) {
	nonce := make([]byte, protector.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	result := make([]byte, 1, 1+len(nonce)+len(plaintext)+protector.aead.Overhead())
	result[0] = protectorVersion
	result = append(result, nonce...)
	result = protector.aead.Seal(result, nonce, plaintext, associatedData)
	return result, nil
}

func (protector *aesProtector) Open(envelope, associatedData []byte) ([]byte, error) {
	if len(envelope) < 1+protector.aead.NonceSize()+protector.aead.Overhead() || envelope[0] != protectorVersion {
		return nil, errors.New("invalid protected envelope")
	}
	nonce := envelope[1 : 1+protector.aead.NonceSize()]
	return protector.aead.Open(nil, nonce, envelope[1+protector.aead.NonceSize():], associatedData)
}

func deriveKey(rootSecret []byte, context string) ([]byte, error) {
	key, err := hkdf.Key(sha256.New, rootSecret, nil, context, 32)
	if err != nil {
		return nil, fmt.Errorf("derive identity key: %w", err)
	}
	return key, nil
}

func normalizeTransactionError(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrOIDCTransaction
	}
	return err
}

func normalizeSessionError(err error) error {
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrSessionExpired) || errors.Is(err, domain.ErrSessionRevoked) {
		return domain.ErrUnauthenticated
	}
	return err
}
