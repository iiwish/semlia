package ingestion

import (
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/iiwish/semlia/pkg/identity"
)

const (
	MaxUploadBytes       int64 = 50 << 20
	MaxArchiveBytes      int64 = 200 << 20
	MaxArchiveEntries          = 4096
	MaxArchiveEntryBytes int64 = 50 << 20
	MaxArchiveExpansion        = 100
	MaxSheets                  = 64
	MaxRows                    = 250_000
	MaxColumns                 = 512
	MaxCells                   = 5_000_000
	MaxCellBytes         int64 = 1 << 20
	MaxMarkdownBytes     int64 = 10 << 20
	MaxMarkdownNodes           = 100_000
	MaxWorkspaceObjects        = 10_000
	MaxWorkspaceBytes    int64 = 5 << 30
)

var (
	ErrInvalid       = errors.New("invalid ingestion input")
	ErrNotFound      = errors.New("ingestion resource not found")
	ErrConflict      = errors.New("ingestion state conflict")
	ErrUnsafeContent = errors.New("unsafe ingestion content")
	ErrLimitExceeded = errors.New("ingestion limit exceeded")
	ErrStore         = errors.New("artifact store unavailable")
	ErrUnsupported   = errors.New("unsupported ingestion type")
	ErrRetryable     = errors.New("temporary ingestion dependency failure")
)

type ArtifactKind string

const (
	ArtifactCSV         ArtifactKind = "csv"
	ArtifactXLSX        ArtifactKind = "xlsx"
	ArtifactMarkdown    ArtifactKind = "markdown"
	ArtifactSQL         ArtifactKind = "sql"
	ArtifactDBTManifest ArtifactKind = "dbt_manifest"
	ArtifactDBTCatalog  ArtifactKind = "dbt_catalog"
)

func (kind ArtifactKind) Valid() bool {
	switch kind {
	case ArtifactCSV, ArtifactXLSX, ArtifactMarkdown, ArtifactSQL, ArtifactDBTManifest, ArtifactDBTCatalog:
		return true
	default:
		return false
	}
}

type ArtifactStatus string

const (
	ArtifactUploaded  ArtifactStatus = "uploaded"
	ArtifactValidated ArtifactStatus = "validated"
	ArtifactRejected  ArtifactStatus = "rejected"
	ArtifactConsumed  ArtifactStatus = "consumed"
)

type ContentAvailability string

const (
	ContentAvailable ContentAvailability = "available"
	ContentExpired   ContentAvailability = "expired"
	ContentNotStored ContentAvailability = "not_stored"
)

type Artifact struct {
	ID                    identity.ArtifactID
	WorkspaceID           identity.WorkspaceID
	SourceConnectionID    *identity.SourceConnectionID
	Kind                  ArtifactKind
	SchemaVersion         string
	ContentDigest         string
	ByteSize              int64
	MediaType             string
	OriginalName          string
	LogicalPath           string
	ValidationSummary     *ArtifactValidationSummary
	ContentAvailability   ContentAvailability
	Status                ArtifactStatus
	FailureCode           string
	UploadedByPrincipalID *identity.PrincipalID
	IdempotencyKey        string
	RequestFingerprint    string
	ExpiresAt             *time.Time
	CreatedAt             time.Time
	FinalizedAt           *time.Time
}

type ArtifactValidationSummary struct {
	AdapterKind       string `json:"adapterKind"`
	AdapterVersion    string `json:"adapterVersion"`
	DatasetCount      int    `json:"datasetCount"`
	FieldCount        int    `json:"fieldCount"`
	CodeArtifactCount int    `json:"codeArtifactCount"`
	LineageCount      int    `json:"lineageCount"`
	KeyCount          int    `json:"keyCount"`
	JoinCount         int    `json:"joinCount"`
	FindingCount      int    `json:"findingCount"`
}

type Object struct {
	WorkspaceID   identity.WorkspaceID
	ContentDigest string
	ByteSize      int64
	MediaType     string
	StorageKey    string
	CreatedAt     time.Time
}

type CleanupObject struct {
	WorkspaceID   identity.WorkspaceID
	ContentDigest string
	ByteSize      int64
}

type ArtifactSet struct {
	ID                   identity.ArtifactSetID
	WorkspaceID          identity.WorkspaceID
	SourceConnectionID   identity.SourceConnectionID
	SourceKind           string
	SetDigest            string
	CreatedByPrincipalID identity.PrincipalID
	Members              []ArtifactMember
	CreatedAt            time.Time
}

type ArtifactMember struct {
	ArtifactID          identity.ArtifactID
	LogicalPath         string
	Ordinal             int
	ContentDigest       string
	ByteSize            int64
	MediaType           string
	Kind                ArtifactKind
	ContentAvailability ContentAvailability
}

type Page[T any] struct {
	Items      []T
	Total      int64
	NextCursor string
	Limit      int
}

type ArtifactStore interface {
	Put(context.Context, identity.WorkspaceID, string, io.Reader, int64) error
	Open(context.Context, identity.WorkspaceID, string) (io.ReadCloser, error)
	Delete(context.Context, identity.WorkspaceID, string) error
	Check(context.Context) error
	Configured() bool
}

type TemporaryArtifactSweeper interface {
	CleanupTemporary(context.Context, time.Time, int) (int, error)
}

type MisfirePolicy string

const (
	MisfireSkip    MisfirePolicy = "skip"
	MisfireRunOnce MisfirePolicy = "run_once"
)

func (policy MisfirePolicy) Valid() bool { return policy == MisfireSkip || policy == MisfireRunOnce }

type Schedule struct {
	ID                   identity.SourceScheduleID
	WorkspaceID          identity.WorkspaceID
	SourceConnectionID   identity.SourceConnectionID
	Expression           string
	Timezone             string
	MisfirePolicy        MisfirePolicy
	Enabled              bool
	NextRunAt            *time.Time
	NextWallClockKey     string
	LastRunAt            *time.Time
	CredentialVersion    *int64
	CreatedByPrincipalID *identity.PrincipalID
	Version              int64
	DeletedAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Occurrence struct {
	ID                 identity.ScheduleOccurrenceID
	WorkspaceID        identity.WorkspaceID
	ScheduleID         identity.SourceScheduleID
	SourceConnectionID identity.SourceConnectionID
	TriggerKind        string
	ScheduleVersion    int64
	ScheduledFor       *time.Time
	EligibleAt         time.Time
	WallClockKey       string
	State              string
	ReasonCode         string
	MisfireDisposition string
	DiscoveryRunID     *identity.RunID
	JobID              *identity.RunID
	RuntimeRunID       *identity.RunID
	ArtifactSetID      *identity.ArtifactSetID
	CredentialVersion  *int64
	SourceFingerprint  string
	RequestedBy        identity.PrincipalID
	IdempotencyKey     string
	CreatedAt          time.Time
}

var safeNamePattern = regexp.MustCompile(`[[:cntrl:]/\\]`)

func SanitizeOriginalName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 || safeNamePattern.MatchString(value) || ContainsUnsafeDisplayRune(value) {
		return "", ErrInvalid
	}
	return value, nil
}

func ContainsUnsafeDisplayRune(value string) bool {
	return strings.IndexFunc(value, func(candidate rune) bool {
		return unicode.IsControl(candidate) || unicode.In(candidate, unicode.Cf)
	}) >= 0
}
