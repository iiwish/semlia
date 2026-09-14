package discovery

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

var (
	ErrNotFound       = errors.New("discovery resource not found")
	ErrConflict       = errors.New("discovery state conflict")
	ErrForbidden      = errors.New("discovery command forbidden")
	ErrCredential     = errors.New("source credential is unavailable")
	ErrUnsafeSource   = errors.New("source role is not read-only")
	ErrArtifactPath   = errors.New("SQL artifact path is not allowed")
	ErrConnectionTest = errors.New("source connection test failed")
)

type Source struct {
	ID                      identity.SourceConnectionID
	WorkspaceID             identity.WorkspaceID
	Kind                    string
	Name                    string
	Host                    string
	Port                    int
	Database                string
	Username                string
	SSLMode                 string
	ArtifactPaths           []string
	Status                  string
	ActiveCredentialVersion int64
	ActiveArtifactSetID     *identity.ArtifactSetID
	Version                 int64
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type CredentialSecret struct {
	Password string `json:"password"`
}

type CredentialEnvelope struct {
	ID                 identity.SourceCredentialID
	WorkspaceID        identity.WorkspaceID
	SourceConnectionID identity.SourceConnectionID
	Version            int64
	KeyVersion         int
	Algorithm          string
	Nonce              []byte
	Ciphertext         []byte
	CreatedBy          string
	CreatedAt          time.Time
}

type Run struct {
	SnapshotID             *identity.SourceSnapshotID
	ID                     identity.RunID
	WorkspaceID            identity.WorkspaceID
	SourceConnectionID     identity.SourceConnectionID
	CredentialVersion      int64
	ArtifactSetID          *identity.ArtifactSetID
	RequestFingerprint     string
	SourceInputFingerprint string
	SourceConfig           json.RawMessage
	JobID                  identity.RunID
	Status                 string
	ErrorCode              string
	Stats                  json.RawMessage
	RequestedBy            string
	TraceID                string
	StartedAt              *time.Time
	CompletedAt            *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type RunPage struct {
	Items      []Run
	Total      int64
	Limit      int
	NextCursor string
}

type SourcePage struct {
	Items      []Source
	Total      int64
	Limit      int
	NextCursor string
}

type Candidate struct {
	ID                 identity.SemanticCandidateID
	WorkspaceID        identity.WorkspaceID
	SourceConnectionID identity.SourceConnectionID
	SourceRevisionID   identity.SourceRevisionID
	DiscoveryRunID     identity.RunID
	Key                string
	Kind               string
	Title              string
	ProposalInput      json.RawMessage
	Evidence           json.RawMessage
	ContentDigest      string
	Status             string
	ProposalID         *identity.ProposalID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CandidatePage struct {
	Items      []Candidate
	Total      int64
	Limit      int
	NextCursor string
}

type CandidateDecision struct {
	ID                 identity.RunID
	WorkspaceID        identity.WorkspaceID
	CandidateID        identity.SemanticCandidateID
	Action             string
	ProposalID         *identity.ProposalID
	Actor              string
	Reason             string
	IdempotencyKey     string
	RequestFingerprint string
	TraceID            string
	CreatedAt          time.Time
}
