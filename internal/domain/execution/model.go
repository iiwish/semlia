package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

const CompilerVersion = "postgres-aggregate/v1"

var ErrInvalidPlan = errors.New("EXECUTION_PLAN_UNSUPPORTED")
var ErrNotConfigured = errors.New("EXECUTION_NOT_CONFIGURED")
var ErrConflict = errors.New("EXECUTION_IDEMPOTENCY_CONFLICT")
var ErrBusy = errors.New("EXECUTION_CONCURRENCY_LIMIT")
var ErrNotFound = errors.New("EXECUTION_NOT_FOUND")

type Limits struct {
	Timeout       time.Duration
	MaxRows       int
	MaxBytes      int
	MaxConcurrent int
}

func DefaultLimits() Limits {
	return Limits{Timeout: 10 * time.Second, MaxRows: 1000, MaxBytes: 1024 * 1024, MaxConcurrent: 2}
}
func (l Limits) Valid() bool {
	return l.Timeout > 0 && l.Timeout <= 30*time.Second && l.MaxRows > 0 && l.MaxRows <= 10000 && l.MaxBytes > 0 && l.MaxBytes <= 8*1024*1024 && l.MaxConcurrent > 0 && l.MaxConcurrent <= 8
}

type Compiled struct {
	Guards           []string
	SQL              string
	Args             []any
	Columns          []string
	SourceID         string
	SourceRevisionID string
	SourceLocator    string
	Relations        [][2]string
}
type Run struct {
	ID               string     `json:"id"`
	WorkspaceID      string     `json:"workspaceId"`
	QueryID          string     `json:"queryId"`
	PlanID           string     `json:"planId"`
	PlanDigest       string     `json:"planDigest"`
	PrincipalID      string     `json:"principalId"`
	ConsumerID       string     `json:"consumerId,omitempty"`
	CredentialID     string     `json:"credentialId,omitempty"`
	Channel          string     `json:"channel"`
	SourceID         string     `json:"sourceId"`
	SourceRevisionID string     `json:"sourceRevisionId"`
	AdapterVersion   string     `json:"adapterVersion"`
	PolicyVersion    int64      `json:"policyVersion"`
	TimeoutMS        int64      `json:"timeoutMs"`
	MaxRows          int        `json:"maxRows"`
	MaxBytes         int        `json:"maxBytes"`
	CancelRequested  bool       `json:"cancelRequested"`
	State            string     `json:"state"`
	ErrorCode        string     `json:"errorCode,omitempty"`
	RowCount         int        `json:"rowCount"`
	ByteCount        int        `json:"byteCount"`
	ResultDigest     string     `json:"resultDigest,omitempty"`
	StartedAt        time.Time  `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
	TraceID          string     `json:"traceId"`
	IdempotencyKey   string     `json:"-"`
	Deadline         time.Time  `json:"-"`
}

// Rows are ephemeral and deliberately separate from the persistence contract.
type Result struct {
	SQL          string     `json:"sql,omitempty"`
	Parameters   []any      `json:"parameters,omitempty"`
	DataTime     *time.Time `json:"dataTime,omitempty"`
	Run          Run        `json:"run"`
	Availability string     `json:"availability"`
	Columns      []string   `json:"columns,omitempty"`
	Rows         [][]any    `json:"rows,omitempty"`
	Replay       bool       `json:"replay"`
}
type Output struct {
	Columns   []string
	Rows      [][]any
	RowCount  int
	ByteCount int
	Digest    string
	ErrorCode string
}

func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}
