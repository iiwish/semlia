// Package identity defines authenticated accounts, workspace membership and
// session authority. Authorization principals remain in domain/authorization.
package identity

import (
	"errors"
	"net/url"
	"strings"
	"time"

	publicid "github.com/iiwish/semlia/pkg/identity"
)

var (
	ErrInvalidArgument    = errors.New("invalid identity argument")
	ErrUnauthenticated    = errors.New("authentication required")
	ErrForbidden          = errors.New("identity action forbidden")
	ErrNotFound           = errors.New("identity resource not found")
	ErrConflict           = errors.New("identity resource conflict")
	ErrSessionExpired     = errors.New("session expired")
	ErrSessionRevoked     = errors.New("session revoked")
	ErrMembershipInactive = errors.New("workspace membership inactive")
	ErrOIDCTransaction    = errors.New("invalid OIDC transaction")
	ErrAdmissionDenied    = errors.New("identity is not invited")
	ErrFinalAdministrator = errors.New("final workspace administrator cannot be removed")
	ErrRateLimited        = errors.New("authentication temporarily rate limited")
)

type AccountStatus string

const (
	AccountActive    AccountStatus = "active"
	AccountSuspended AccountStatus = "suspended"
	AccountRevoked   AccountStatus = "revoked"
)

type MembershipStatus string

const (
	MembershipActive    MembershipStatus = "active"
	MembershipSuspended MembershipStatus = "suspended"
	MembershipRevoked   MembershipStatus = "revoked"
)

type InvitationStatus string

const (
	InvitationPending  InvitationStatus = "pending"
	InvitationAccepted InvitationStatus = "accepted"
	InvitationRevoked  InvitationStatus = "revoked"
	InvitationExpired  InvitationStatus = "expired"
)

type Account struct {
	ID          publicid.UserAccountID
	DisplayName string
	Status      AccountStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ExternalIdentity struct {
	ID            publicid.ExternalIdentityID
	AccountID     publicid.UserAccountID
	Issuer        string
	Subject       string
	VerifiedEmail string
	Status        AccountStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Membership struct {
	ID                   publicid.MembershipID
	WorkspaceID          publicid.WorkspaceID
	WorkspaceSlug        string
	WorkspaceDisplayName string
	AccountID            publicid.UserAccountID
	AccountDisplayName   string
	PrincipalID          publicid.PrincipalID
	PrincipalDisplayName string
	Status               MembershipStatus
	RoleIDs              []string
	AuthorizationVersion int64
	AdmittedAt           time.Time
	SuspendedAt          *time.Time
	RevokedAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Invitation struct {
	ID                   publicid.InvitationID
	WorkspaceID          publicid.WorkspaceID
	Issuer               string
	Subject              string
	Email                string
	RoleID               string
	RoleVersion          int64
	Status               InvitationStatus
	CreatedByPrincipalID *publicid.PrincipalID
	AcceptedByAccountID  *publicid.UserAccountID
	ExpiresAt            time.Time
	AcceptedAt           *time.Time
	RevokedAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Session struct {
	ID                publicid.SessionID
	Account           Account
	Memberships       []Membership
	CSRFDigest        [32]byte
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
	LastSeenAt        time.Time
	CreatedAt         time.Time
}

type LoginAttempt struct {
	StateDigest      [32]byte
	NonceDigest      [32]byte
	VerifierEnvelope []byte
	ReturnTo         string
	ExpiresAt        time.Time
	ConsumedAt       *time.Time
	CreatedAt        time.Time
}

type OIDCClaims struct {
	Issuer        string
	Subject       string
	DisplayName   string
	Email         string
	EmailVerified bool
	Nonce         string
}

func NormalizeIssuer(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrInvalidArgument
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", ErrInvalidArgument
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func NormalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	parts := strings.Split(value, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(value, " \t\r\n") || len(value) > 320 {
		return "", ErrInvalidArgument
	}
	return value, nil
}

func ValidReturnTo(value string) bool {
	if value == "" || value == "/" {
		return true
	}
	return strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") && !strings.Contains(value, "\\") && len(value) <= 1024
}
