package governance

import (
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

// AskRequestRecord contains attribution and result references, never model text.
type AskRequestRecord struct {
	WorkspaceID     identity.WorkspaceID
	RequestedBy     identity.PrincipalID
	Key             string
	InputDigest     string
	KnowledgeDigest string
	RunID           identity.AgentRunID
	ClaimToken      string
	Deadline        time.Time
	Status          string
	QueryID         *identity.SemanticQueryID
	ErrorCode       string
	CreatedAt       time.Time
	CompletedAt     *time.Time
}
