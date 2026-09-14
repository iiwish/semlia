package workbench

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

var (
	ErrInvalidArgument = errors.New("invalid workbench argument")
	ErrNotFound        = errors.New("workbench item not found")
	ErrConflict        = errors.New("workbench item conflict")
)

type Kind string

const (
	KindReview        Kind = "review"
	KindValidation    Kind = "validation"
	KindSource        Kind = "source"
	KindRuntime       Kind = "runtime"
	KindCompatibility Kind = "compatibility"
)

type State string

const (
	StateOpen       State = "open"
	StateInProgress State = "in_progress"
	StateResolved   State = "resolved"
	StateDismissed  State = "dismissed"
)

type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

type View string

const (
	ViewMine      View = "mine"
	ViewTeam      View = "team"
	ViewInitiated View = "initiated"
)

type Sort string

const (
	SortUpdatedDesc  Sort = "updated_desc"
	SortPriorityDesc Sort = "priority_desc"
	SortDueAsc       Sort = "due_asc"
)

type Action string

const (
	ActionReview     Action = "review"
	ActionValidate   Action = "run_validation"
	ActionManage     Action = "manage_source"
	ActionPublish    Action = "publish"
	ActionAssign     Action = "assign"
	ActionDismiss    Action = "dismiss"
	ActionOpenTarget Action = "open_target"
)

type Item struct {
	ID                   identity.AttentionItemID
	WorkspaceID          identity.WorkspaceID
	Kind                 Kind
	DedupeKey            string
	State                State
	Priority             Priority
	Risk                 Priority
	TargetType           string
	TargetID             string
	TargetRoute          string
	AssigneePrincipalID  *identity.PrincipalID
	AudienceRoleID       string
	InitiatorPrincipalID *identity.PrincipalID
	RuleVersion          string
	Title                string
	Summary              string
	ReasonCode           string
	EvidenceRef          string
	TraceID              string
	OpenedAt             time.Time
	DueAt                *time.Time
	ResolvedAt           *time.Time
	UpdatedAt            time.Time
	Version              int64
	NextActions          []Action
	ReviewedByPrincipal  bool
}

func (item Item) Validate() error {
	if item.ID.IsZero() || item.WorkspaceID.IsZero() || !item.Kind.Valid() || !item.State.Valid() ||
		!item.Priority.Valid() || !item.Risk.Valid() || strings.TrimSpace(item.DedupeKey) == "" || len(item.DedupeKey) > 256 ||
		strings.TrimSpace(item.TargetType) == "" || strings.TrimSpace(item.TargetID) == "" ||
		!strings.HasPrefix(item.TargetRoute, "/") || item.RuleVersion == "" || item.Title == "" || item.Summary == "" ||
		item.ReasonCode == "" || len(item.TraceID) != 32 || item.OpenedAt.IsZero() || item.UpdatedAt.IsZero() || item.Version < 1 {
		return ErrInvalidArgument
	}
	return nil
}

func (kind Kind) Valid() bool {
	switch kind {
	case KindReview, KindValidation, KindSource, KindRuntime, KindCompatibility:
		return true
	}
	return false
}

func (state State) Valid() bool {
	switch state {
	case StateOpen, StateInProgress, StateResolved, StateDismissed:
		return true
	}
	return false
}

func (priority Priority) Valid() bool {
	switch priority {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityCritical:
		return true
	}
	return false
}

func (view View) Valid() bool {
	switch view {
	case ViewMine, ViewTeam, ViewInitiated:
		return true
	}
	return false
}

func (sort Sort) Valid() bool {
	switch sort {
	case SortUpdatedDesc, SortPriorityDesc, SortDueAsc:
		return true
	}
	return false
}

type Counts struct {
	Total      int
	Open       int
	InProgress int
	Critical   int
}

type Cursor struct {
	UpdatedAt    time.Time
	ID           identity.AttentionItemID
	PriorityRank int
	DueAt        *time.Time
}

type ListQuery struct {
	WorkspaceID          identity.WorkspaceID
	PrincipalID          identity.PrincipalID
	AccessGrants         []AccessGrant
	AllowedKinds         []Kind
	View                 View
	Search               string
	Kind                 Kind
	State                State
	Priority             Priority
	Risk                 Priority
	Sort                 Sort
	AuthorizationVersion int64
	Limit                int
	Cursor               *Cursor
}

type AccessGrant struct {
	RoleID    string `json:"roleId"`
	Action    string `json:"action"`
	ScopeType string `json:"scopeType"`
	ScopeID   string `json:"scopeId"`
}

type UpdateCommand struct {
	WorkspaceID           identity.WorkspaceID
	ItemID                identity.AttentionItemID
	ActorPrincipalID      identity.PrincipalID
	AssigneePrincipalID   *identity.PrincipalID
	SetAssignee           bool
	State                 State
	ExpectedVersion       int64
	TraceID               string
	UpdatedAt             time.Time
	AuditEventID          identity.EventID
	IdempotencyKey        string
	RequestFingerprint    string
	AuthorizationVersion  int64
	VisibilityFingerprint string
}

func VisibilityFingerprint(item Item) string {
	assignee, initiator := "", ""
	if item.AssigneePrincipalID != nil {
		assignee = item.AssigneePrincipalID.String()
	}
	if item.InitiatorPrincipalID != nil {
		initiator = item.InitiatorPrincipalID.String()
	}
	dueAt := ""
	if item.DueAt != nil {
		dueAt = item.DueAt.UTC().Format(time.RFC3339Nano)
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{string(item.Kind), string(item.Priority), string(item.Risk),
		item.TargetType, item.TargetID, item.TargetRoute, assignee, item.AudienceRoleID, initiator,
		item.RuleVersion, item.Title, item.Summary, item.ReasonCode, item.EvidenceRef, item.TraceID,
		item.OpenedAt.UTC().Format(time.RFC3339Nano), dueAt, item.UpdatedAt.UTC().Format(time.RFC3339Nano)}, "\x00")))
	return hex.EncodeToString(digest[:])
}

type ReconcileStats struct {
	Scanned   int
	Upserted  int
	Resolved  int
	Truncated bool
}
