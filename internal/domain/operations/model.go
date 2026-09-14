package operations

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

var (
	ErrInvalidArgument      = errors.New("invalid operations argument")
	ErrNotFound             = errors.New("operations resource not found")
	ErrConflict             = errors.New("operations resource conflict")
	ErrRunActionUnsupported = errors.New("runtime run action is unsupported")
)

type RunKind string

const (
	RunKindDiscovery          RunKind = "discovery"
	RunKindValidation         RunKind = "validation"
	RunKindAgent              RunKind = "agent"
	RunKindSemanticResolution RunKind = "semantic_resolution"
	RunKindEmbeddingRebuild   RunKind = "embedding_rebuild"
	RunKindWebhookDelivery    RunKind = "webhook_delivery"
	RunKindQueryExecution     RunKind = "query_execution"
	RunKindAuditExport        RunKind = "audit_export"
)

func ValidRunKind(value RunKind) bool {
	switch value {
	case RunKindDiscovery, RunKindValidation, RunKindAgent, RunKindSemanticResolution,
		RunKindEmbeddingRebuild, RunKindWebhookDelivery, RunKindQueryExecution, RunKindAuditExport:
		return true
	default:
		return false
	}
}

type RunState string

const (
	RunQueued     RunState = "queued"
	RunRunning    RunState = "running"
	RunSucceeded  RunState = "succeeded"
	RunDegraded   RunState = "degraded"
	RunFailed     RunState = "failed"
	RunCancelled  RunState = "cancelled"
	RunDeadLetter RunState = "dead_letter"
)

func ValidRunState(value RunState) bool {
	switch value {
	case RunQueued, RunRunning, RunSucceeded, RunDegraded, RunFailed, RunCancelled, RunDeadLetter:
		return true
	default:
		return false
	}
}

type RunCapabilities struct {
	Retry  bool
	Cancel bool
}

type RuntimeRun struct {
	ID                     identity.RunID
	WorkspaceID            identity.WorkspaceID
	Kind                   RunKind
	SourceType             string
	SourceID               string
	SourceVersionDigest    string
	JobID                  *identity.RunID
	TraceID                string
	IdempotencyKey         string
	RequestedByPrincipalID *identity.PrincipalID
	State                  RunState
	Phase                  string
	ProgressCurrent        *int64
	ProgressTotal          *int64
	Attempt                int32
	MaxAttempts            int32
	StartedAt              *time.Time
	FinishedAt             *time.Time
	ErrorCode              string
	ErrorSummary           string
	Version                int64
	CreatedAt              time.Time
	UpdatedAt              time.Time
	Capabilities           RunCapabilities
}

var (
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	tracePattern  = regexp.MustCompile(`^[0-9a-f]{32}$`)
	codePattern   = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)
)

func (run RuntimeRun) Validate() error {
	if run.ID.IsZero() || run.WorkspaceID.IsZero() || !ValidRunKind(run.Kind) ||
		strings.TrimSpace(run.SourceType) == "" || strings.TrimSpace(run.SourceID) == "" ||
		!digestPattern.MatchString(run.SourceVersionDigest) || (run.TraceID != "" && !tracePattern.MatchString(run.TraceID)) ||
		strings.TrimSpace(run.IdempotencyKey) == "" || !ValidRunState(run.State) ||
		run.Attempt < 0 || run.MaxAttempts < 1 || run.Attempt > run.MaxAttempts ||
		run.CreatedAt.IsZero() || run.UpdatedAt.IsZero() {
		return ErrInvalidArgument
	}
	if (run.ProgressCurrent == nil) != (run.ProgressTotal == nil) ||
		(run.ProgressCurrent != nil && (*run.ProgressCurrent < 0 || *run.ProgressTotal <= 0 || *run.ProgressCurrent > *run.ProgressTotal)) {
		return ErrInvalidArgument
	}
	terminal := run.State == RunSucceeded || run.State == RunDegraded || run.State == RunFailed || run.State == RunCancelled || run.State == RunDeadLetter
	if terminal != (run.FinishedAt != nil) {
		return ErrInvalidArgument
	}
	if (run.State == RunFailed || run.State == RunDeadLetter) && !codePattern.MatchString(run.ErrorCode) {
		return ErrInvalidArgument
	}
	if len(run.ErrorSummary) > 512 {
		return ErrInvalidArgument
	}
	return nil
}

type RunEventType string

const (
	RunEventState      RunEventType = "state"
	RunEventPhase      RunEventType = "phase"
	RunEventProgress   RunEventType = "progress"
	RunEventDiagnostic RunEventType = "diagnostic"
)

type RuntimeRunEvent struct {
	ID              identity.EventID
	WorkspaceID     identity.WorkspaceID
	RunID           identity.RunID
	EventKey        string
	Sequence        int64
	Type            RunEventType
	Phase           string
	ProgressCurrent *int64
	ProgressTotal   *int64
	State           RunState
	ErrorCode       string
	Summary         string
	Metadata        json.RawMessage
	CreatedAt       time.Time
}

func (event RuntimeRunEvent) ValidateForProjection() error {
	if event.ID.IsZero() || event.WorkspaceID.IsZero() || event.RunID.IsZero() ||
		strings.TrimSpace(event.EventKey) == "" || event.CreatedAt.IsZero() {
		return ErrInvalidArgument
	}
	switch event.Type {
	case RunEventState, RunEventPhase, RunEventProgress, RunEventDiagnostic:
	default:
		return ErrInvalidArgument
	}
	if event.State != "" && !ValidRunState(event.State) {
		return ErrInvalidArgument
	}
	if event.ErrorCode != "" && !codePattern.MatchString(event.ErrorCode) {
		return ErrInvalidArgument
	}
	if len(event.Summary) > 512 || !validBoundedObject(event.Metadata, 8192) {
		return ErrInvalidArgument
	}
	return nil
}

type AuditEvent struct {
	ID          identity.EventID
	WorkspaceID identity.WorkspaceID
	EventType   string
	ActorID     string
	ObjectType  string
	ObjectID    string
	Channel     string
	Outcome     string
	ReasonCode  string
	Summary     string
	TraceID     string
	Details     map[string]any
	CreatedAt   time.Time
}

// ProjectAuditDetails is the single outbound audit allowlist. It never returns
// nested input payloads, credentials, prompts, headers or provider bodies.
func ProjectAuditDetails(_ string, payload json.RawMessage) map[string]any {
	var source map[string]any
	if len(payload) == 0 || json.Unmarshal(payload, &source) != nil {
		return map[string]any{}
	}
	allowed := map[string]struct{}{
		"action": {}, "artifactId": {}, "artifactSetId": {}, "assetId": {}, "bindingId": {}, "candidateId": {}, "channel": {},
		"consumerId": {}, "credentialVersion": {}, "decision": {}, "eventVersion": {},
		"membershipId": {}, "outcome": {}, "proposalId": {},
		"queryId": {}, "reasonCode": {}, "releaseId": {}, "roleId": {}, "roleVersion": {},
		"runId": {}, "scheduleId": {}, "occurrenceId": {}, "sourceId": {}, "status": {}, "version": {},
	}
	result := make(map[string]any)
	for key := range allowed {
		value, ok := source[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if len(typed) <= 256 {
				result[key] = typed
			}
		case float64, bool:
			result[key] = typed
		}
	}
	return result
}

// ProjectRuntimeMetadata keeps only bounded operational counters and states.
// Free-form job payloads, provider fields and credentials are dropped.
func ProjectRuntimeMetadata(payload json.RawMessage) json.RawMessage {
	var source map[string]any
	if len(payload) == 0 || json.Unmarshal(payload, &source) != nil {
		return json.RawMessage(`{}`)
	}
	allowed := map[string]struct{}{
		"attempt": {}, "batch": {}, "bytes": {}, "count": {}, "durationMs": {},
		"itemCount": {}, "position": {}, "rowCount": {}, "status": {}, "total": {},
	}
	result := make(map[string]any)
	for key := range allowed {
		value, ok := source[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if len(typed) <= 128 {
				result[key] = typed
			}
		case float64, bool:
			result[key] = typed
		}
	}
	projected, err := json.Marshal(result)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return projected
}

type RuntimeSettings struct {
	WorkspaceID              identity.WorkspaceID
	RetryCeiling             int32
	StatementTimeoutMS       int32
	WebhookTimeoutMS         int32
	QueryRowLimit            int32
	QueryByteLimit           int64
	RunMetadataRetentionDays int32
	Version                  int64
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

func (settings RuntimeSettings) Validate() error {
	if settings.WorkspaceID.IsZero() || settings.RetryCeiling < 1 || settings.RetryCeiling > 10 ||
		settings.StatementTimeoutMS < 100 || settings.StatementTimeoutMS > 300000 ||
		settings.WebhookTimeoutMS < 100 || settings.WebhookTimeoutMS > 120000 ||
		settings.QueryRowLimit < 1 || settings.QueryRowLimit > 100000 ||
		settings.QueryByteLimit < 1024 || settings.QueryByteLimit > 104857600 ||
		settings.RunMetadataRetentionDays < 1 || settings.RunMetadataRetentionDays > 365 ||
		settings.Version < 1 || settings.CreatedAt.IsZero() || settings.UpdatedAt.IsZero() {
		return ErrInvalidArgument
	}
	return nil
}

type DeploymentStatus struct {
	WorkerConfigured     bool
	TelemetryConfigured  bool
	OIDCConfigured       bool
	EncryptionConfigured bool
	AuditRetention       string
}

func validBoundedObject(value json.RawMessage, limit int) bool {
	if len(value) == 0 {
		return true
	}
	if len(value) > limit {
		return false
	}
	var decoded map[string]any
	return json.Unmarshal(value, &decoded) == nil
}
