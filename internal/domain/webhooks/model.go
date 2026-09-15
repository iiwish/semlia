package webhooks

import (
	"encoding/json"
	"errors"
	"github.com/iiwish/semlia/pkg/identity"
	"time"
)

var ErrInvalidArgument = errors.New("invalid webhook argument")
var ErrNotFound = errors.New("webhook not found")
var ErrConflict = errors.New("webhook version conflict")

type Subscription struct {
	ID             identity.ID          `json:"id"`
	WorkspaceID    identity.WorkspaceID `json:"workspaceId"`
	Name           string               `json:"name"`
	Endpoint       string               `json:"endpoint"`
	Enabled        bool                 `json:"enabled"`
	EventTypes     []string             `json:"eventTypes"`
	Version        int                  `json:"version"`
	SigningVersion int                  `json:"signingVersion"`
	SecretSuffix   string               `json:"secretSuffix"`
	SecretEnvelope []byte               `json:"-"`
	CreatedBy      identity.PrincipalID `json:"createdBy"`
	CreatedAt      time.Time            `json:"createdAt"`
	UpdatedAt      time.Time            `json:"updatedAt"`
}
type IssuedSubscription struct {
	Subscription  Subscription `json:"subscription"`
	SigningSecret string       `json:"signingSecret"`
}
type Envelope struct {
	Version     string               `json:"version"`
	ID          identity.EventID     `json:"id"`
	Type        string               `json:"type"`
	WorkspaceID identity.WorkspaceID `json:"workspaceId"`
	OccurredAt  time.Time            `json:"occurredAt"`
	TraceID     string               `json:"traceId"`
	Data        json.RawMessage      `json:"data"`
}
type Delivery struct {
	ID                  identity.ID          `json:"id"`
	WorkspaceID         identity.WorkspaceID `json:"workspaceId"`
	SubscriptionID      identity.ID          `json:"subscriptionId"`
	SubscriptionVersion int                  `json:"subscriptionVersion"`
	SigningVersion      int                  `json:"signingVersion"`
	EventID             identity.EventID     `json:"eventId"`
	EventType           string               `json:"eventType"`
	State               string               `json:"state"`
	Attempt             int                  `json:"attempt"`
	MaxAttempts         int                  `json:"maxAttempts"`
	HTTPStatus          int                  `json:"httpStatus"`
	ErrorCode           string               `json:"errorCode,omitempty"`
	Endpoint            string               `json:"-"`
	SecretEnvelope      []byte               `json:"-"`
	Envelope            json.RawMessage      `json:"-"`
	PayloadDigest       string               `json:"payloadDigest"`
	TraceID             string               `json:"traceId"`
	CreatedAt           time.Time            `json:"createdAt"`
	UpdatedAt           time.Time            `json:"updatedAt"`
	NextAttemptAt       time.Time            `json:"nextAttemptAt"`
	RuntimeRunID        identity.RunID       `json:"runtimeRunId"`
}
