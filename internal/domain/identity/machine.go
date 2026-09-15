package identity

import (
	"github.com/iiwish/semlia/internal/domain/authorization"
	id "github.com/iiwish/semlia/pkg/identity"
	"time"
)

type ClientCredential struct {
	ID             id.ClientCredentialID   `json:"id"`
	WorkspaceID    id.WorkspaceID          `json:"workspaceId"`
	ConsumerID     id.ConsumerID           `json:"consumerId"`
	BindingID      id.ConsumerBindingID    `json:"bindingId"`
	PrincipalID    id.PrincipalID          `json:"principalId"`
	Name           string                  `json:"name"`
	TokenPrefix    string                  `json:"tokenPrefix"`
	VerifierDigest [32]byte                `json:"-"`
	AllowedActions []authorization.Action  `json:"allowedActions"`
	ScopeType      authorization.ScopeType `json:"scopeType"`
	ScopeID        string                  `json:"scopeId"`
	IssuedBy       id.PrincipalID          `json:"issuedBy"`
	IssuedAt       time.Time               `json:"issuedAt"`
	ExpiresAt      time.Time               `json:"expiresAt"`
	RevokedAt      *time.Time              `json:"revokedAt,omitempty"`
	RotatedFromID  *id.ClientCredentialID  `json:"rotatedFromId,omitempty"`
	LastUsedAt     *time.Time              `json:"lastUsedAt,omitempty"`
}

type IssuedCredential struct {
	Credential ClientCredential `json:"credential"`
	Token      string           `json:"token"`
}
