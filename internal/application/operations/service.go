package operations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/pkg/identity"
)

const (
	DefaultPageSize = 50
	MaxPageSize     = 200
	MaxExportRows   = 1000
	MaxExportBytes  = 2 * 1024 * 1024
)

var traceFilterPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type Clock interface{ Now() time.Time }
type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

type PageCursor struct {
	Time time.Time
	ID   string
}

type AuditQuery struct {
	WorkspaceID identity.WorkspaceID
	ActorID     string
	EventType   string
	ObjectType  string
	ObjectID    string
	TraceID     string
	From        *time.Time
	To          *time.Time
	After       *PageCursor
	CursorScope string
	Limit       int
}

type StoredAuditEvent struct {
	ID          identity.EventID
	WorkspaceID identity.WorkspaceID
	EventType   string
	ActorID     string
	ObjectType  string
	ObjectID    string
	Payload     json.RawMessage
	TraceID     string
	CreatedAt   time.Time
}

type RuntimeQuery struct {
	WorkspaceID identity.WorkspaceID
	Kind        domain.RunKind
	State       domain.RunState
	SourceType  string
	SourceID    string
	TraceID     string
	After       *PageCursor
	CursorScope string
	Limit       int
}

type AuditExportRecord struct {
	ID                   identity.EventID
	WorkspaceID          identity.WorkspaceID
	RuntimeRun           domain.RuntimeRun
	ArtifactID           string
	Format               string
	Filters              json.RawMessage
	RowCount             int
	ContentDigest        string
	RequestFingerprint   string
	Content              []byte
	IdempotencyKey       string
	CreatedByPrincipalID identity.PrincipalID
	TraceID              string
	CreatedAt            time.Time
	ExpiresAt            time.Time
}

type AuditExportReplayRequest struct {
	WorkspaceID          identity.WorkspaceID
	IdempotencyKey       string
	RequestFingerprint   string
	Filters              json.RawMessage
	CreatedByPrincipalID identity.PrincipalID
	Now                  time.Time
}

type AuditExport struct {
	ID            identity.EventID
	RuntimeRunID  identity.RunID
	ArtifactID    string
	Format        string
	RowCount      int
	ContentDigest string
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

type AuditExportContent struct {
	ID            identity.EventID
	Format        string
	Content       []byte
	ContentDigest string
	ExpiresAt     time.Time
}

type Repository interface {
	ListAuditEvents(context.Context, AuditQuery) ([]StoredAuditEvent, error)
	FindAuditExportReplay(context.Context, AuditExportReplayRequest) (AuditExport, bool, error)
	CreateAuditExport(context.Context, AuditExportRecord) (AuditExport, error)
	GetAuditExportContent(context.Context, identity.WorkspaceID, identity.EventID, time.Time) (AuditExportContent, error)
	ListRuntimeRuns(context.Context, RuntimeQuery) ([]domain.RuntimeRun, error)
	GetRuntimeRun(context.Context, identity.WorkspaceID, identity.RunID) (domain.RuntimeRun, error)
	ListRuntimeRunEvents(context.Context, identity.WorkspaceID, identity.RunID, int) ([]domain.RuntimeRunEvent, error)
	GetRuntimeSettings(context.Context, identity.WorkspaceID, time.Time) (domain.RuntimeSettings, error)
	UpdateRuntimeSettings(context.Context, domain.RuntimeSettings, int64) (domain.RuntimeSettings, error)
}

type Service struct {
	repository Repository
	authorizer authorizationapp.Evaluator
	clock      Clock
	deployment domain.DeploymentStatus
	exportTTL  time.Duration
}

func NewService(repository Repository, authorizer authorizationapp.Evaluator, clock Clock, deployment domain.DeploymentStatus) *Service {
	if repository == nil || clock == nil {
		panic("operations repository and clock are required")
	}
	return &Service{repository: repository, authorizer: authorizer, clock: clock, deployment: deployment, exportTTL: time.Hour}
}

type AccessRequest struct {
	WorkspaceID  identity.WorkspaceID
	PrincipalRef string
	TraceID      string
}

type AuditListRequest struct {
	AccessRequest
	ActorID     string
	EventType   string
	ObjectType  string
	ObjectID    string
	TraceFilter string
	From        *time.Time
	To          *time.Time
	Cursor      string
	Limit       int
}

type AuditPage struct {
	Items      []domain.AuditEvent
	NextCursor string
}

func (service *Service) ListAuditEvents(ctx context.Context, request AuditListRequest) (AuditPage, error) {
	if err := service.authorize(ctx, request.AccessRequest, authorization.ActionAuditRead); err != nil {
		return AuditPage{}, err
	}
	limit := pageLimit(request.Limit)
	query, err := auditQuery(request, limit+1)
	if err != nil {
		return AuditPage{}, err
	}
	rows, err := service.repository.ListAuditEvents(ctx, query)
	if err != nil {
		return AuditPage{}, err
	}
	return projectAuditPage(rows, limit, query.CursorScope), nil
}

type CreateAuditExportRequest struct {
	AuditListRequest
	IdempotencyKey string
}

func (service *Service) CreateAuditExport(ctx context.Context, request CreateAuditExportRequest) (AuditExport, error) {
	decision, err := service.authorizeDecision(ctx, request.AccessRequest, authorization.ActionAuditRead)
	if err != nil {
		return AuditExport{}, err
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" || len(request.IdempotencyKey) > 256 || decision.PrincipalID.IsZero() {
		return AuditExport{}, domain.ErrInvalidArgument
	}
	query, err := auditQuery(request.AuditListRequest, MaxExportRows+1)
	if err != nil {
		return AuditExport{}, err
	}
	query.After = nil
	filters, err := canonicalAuditFilter(query)
	if err != nil {
		return AuditExport{}, err
	}
	requestFingerprint := auditExportRequestFingerprint(filters, decision.PrincipalID)
	now := service.clock.Now().UTC()
	replayed, found, err := service.repository.FindAuditExportReplay(ctx, AuditExportReplayRequest{
		WorkspaceID: request.WorkspaceID, IdempotencyKey: request.IdempotencyKey,
		RequestFingerprint: requestFingerprint, Filters: filters,
		CreatedByPrincipalID: decision.PrincipalID, Now: now,
	})
	if err != nil {
		return AuditExport{}, err
	}
	if found {
		return replayed, nil
	}
	rows, err := service.repository.ListAuditEvents(ctx, query)
	if err != nil {
		return AuditExport{}, err
	}
	if len(rows) > MaxExportRows {
		return AuditExport{}, domain.ErrConflict
	}
	page := projectAuditPage(rows, MaxExportRows, query.CursorScope)
	content, err := json.Marshal(auditExportItems(page.Items))
	if err != nil || len(content) > MaxExportBytes {
		return AuditExport{}, domain.ErrConflict
	}
	digestBytes := sha256.Sum256(content)
	digest := hex.EncodeToString(digestBytes[:])
	exportID, err := identity.NewEventID()
	if err != nil {
		return AuditExport{}, err
	}
	runID, err := identity.NewRunID()
	if err != nil {
		return AuditExport{}, err
	}
	finished := now
	run := domain.RuntimeRun{ID: runID, WorkspaceID: request.WorkspaceID, Kind: domain.RunKindAuditExport,
		SourceType: "audit_export", SourceID: exportID.String(), SourceVersionDigest: digest,
		TraceID: request.TraceID, IdempotencyKey: derivedIdempotencyKey("runtime:audit-export:", exportID.String()),
		RequestedByPrincipalID: &decision.PrincipalID, State: domain.RunSucceeded, MaxAttempts: 1,
		FinishedAt: &finished, Version: 1, CreatedAt: now, UpdatedAt: now}
	return service.repository.CreateAuditExport(ctx, AuditExportRecord{ID: exportID, WorkspaceID: request.WorkspaceID,
		RuntimeRun: run, ArtifactID: "audit-export/" + exportID.String() + ".json", Format: "json", Filters: filters,
		RowCount: len(page.Items), ContentDigest: digest, RequestFingerprint: requestFingerprint,
		Content: content, IdempotencyKey: request.IdempotencyKey,
		CreatedByPrincipalID: decision.PrincipalID, TraceID: request.TraceID, CreatedAt: now, ExpiresAt: now.Add(service.exportTTL)})
}

func (service *Service) GetAuditExportContent(ctx context.Context, access AccessRequest, exportID identity.EventID) (AuditExportContent, error) {
	if err := service.authorize(ctx, access, authorization.ActionAuditRead); err != nil {
		return AuditExportContent{}, err
	}
	content, err := service.repository.GetAuditExportContent(ctx, access.WorkspaceID, exportID, service.clock.Now().UTC())
	if err != nil {
		return AuditExportContent{}, err
	}
	digest := sha256.Sum256(content.Content)
	if len(content.Content) > MaxExportBytes || hex.EncodeToString(digest[:]) != content.ContentDigest {
		return AuditExportContent{}, errors.New("audit export content integrity check failed")
	}
	return content, nil
}

type RuntimeListRequest struct {
	AccessRequest
	Kind        domain.RunKind
	State       domain.RunState
	SourceType  string
	SourceID    string
	TraceFilter string
	Cursor      string
	Limit       int
}

type RuntimePage struct {
	Items      []domain.RuntimeRun
	NextCursor string
}

func (service *Service) ListRuns(ctx context.Context, request RuntimeListRequest) (RuntimePage, error) {
	if err := service.authorize(ctx, request.AccessRequest, authorization.ActionRuntimeRead); err != nil {
		return RuntimePage{}, err
	}
	if request.Kind != "" && !domain.ValidRunKind(request.Kind) || request.State != "" && !domain.ValidRunState(request.State) ||
		len(strings.TrimSpace(request.SourceType)) > 64 || len(strings.TrimSpace(request.SourceID)) > 128 ||
		!validOptionalTrace(request.TraceFilter) || len(request.Cursor) > 2048 {
		return RuntimePage{}, domain.ErrInvalidArgument
	}
	cursorScope := runtimeCursorScope(request)
	after, err := decodeCursor(request.Cursor, cursorScope, identity.Run)
	if err != nil {
		return RuntimePage{}, err
	}
	limit := pageLimit(request.Limit)
	rows, err := service.repository.ListRuntimeRuns(ctx, RuntimeQuery{WorkspaceID: request.WorkspaceID,
		Kind: request.Kind, State: request.State, SourceType: strings.TrimSpace(request.SourceType),
		SourceID: strings.TrimSpace(request.SourceID), TraceID: strings.TrimSpace(request.TraceFilter), After: after,
		CursorScope: cursorScope, Limit: limit + 1})
	if err != nil {
		return RuntimePage{}, err
	}
	page := RuntimePage{Items: rows}
	if len(rows) > limit {
		page.Items = rows[:limit]
		page.NextCursor = encodeCursor(rows[limit-1].UpdatedAt, rows[limit-1].ID.UUID(), cursorScope)
	}
	return page, nil
}

type RuntimeDetail struct {
	Run    domain.RuntimeRun
	Events []domain.RuntimeRunEvent
}

func (service *Service) GetRun(ctx context.Context, access AccessRequest, runID identity.RunID) (RuntimeDetail, error) {
	if err := service.authorize(ctx, access, authorization.ActionRuntimeRead); err != nil {
		return RuntimeDetail{}, err
	}
	run, err := service.repository.GetRuntimeRun(ctx, access.WorkspaceID, runID)
	if err != nil {
		return RuntimeDetail{}, err
	}
	events, err := service.repository.ListRuntimeRunEvents(ctx, access.WorkspaceID, runID, 500)
	if err != nil {
		return RuntimeDetail{}, err
	}
	return RuntimeDetail{Run: run, Events: events}, nil
}

func (service *Service) RetryRun(ctx context.Context, access AccessRequest, runID identity.RunID) error {
	return service.unsupportedRunAction(ctx, access, runID)
}

func (service *Service) CancelRun(ctx context.Context, access AccessRequest, runID identity.RunID) error {
	return service.unsupportedRunAction(ctx, access, runID)
}

func (service *Service) unsupportedRunAction(ctx context.Context, access AccessRequest, runID identity.RunID) error {
	if err := service.authorize(ctx, access, authorization.ActionRuntimeManage); err != nil {
		return err
	}
	if _, err := service.repository.GetRuntimeRun(ctx, access.WorkspaceID, runID); err != nil {
		return err
	}
	return domain.ErrRunActionUnsupported
}

type RuntimePolicy struct {
	Settings   domain.RuntimeSettings
	Deployment domain.DeploymentStatus
}

func (service *Service) GetRuntimePolicy(ctx context.Context, access AccessRequest) (RuntimePolicy, error) {
	if err := service.authorize(ctx, access, authorization.ActionRuntimeRead); err != nil {
		return RuntimePolicy{}, err
	}
	settings, err := service.repository.GetRuntimeSettings(ctx, access.WorkspaceID, service.clock.Now().UTC())
	if err != nil {
		return RuntimePolicy{}, err
	}
	return RuntimePolicy{Settings: settings, Deployment: service.deployment}, nil
}

type UpdateRuntimeSettingsRequest struct {
	AccessRequest
	ExpectedVersion          int64
	RetryCeiling             int32
	StatementTimeoutMS       int32
	WebhookTimeoutMS         int32
	QueryRowLimit            int32
	QueryByteLimit           int64
	RunMetadataRetentionDays int32
}

func (service *Service) UpdateRuntimeSettings(ctx context.Context, request UpdateRuntimeSettingsRequest) (RuntimePolicy, error) {
	if err := service.authorize(ctx, request.AccessRequest, authorization.ActionRuntimeManage); err != nil {
		return RuntimePolicy{}, err
	}
	if request.ExpectedVersion < 1 {
		return RuntimePolicy{}, domain.ErrInvalidArgument
	}
	now := service.clock.Now().UTC()
	settings := domain.RuntimeSettings{WorkspaceID: request.WorkspaceID, RetryCeiling: request.RetryCeiling,
		StatementTimeoutMS: request.StatementTimeoutMS, WebhookTimeoutMS: request.WebhookTimeoutMS,
		QueryRowLimit: request.QueryRowLimit, QueryByteLimit: request.QueryByteLimit,
		RunMetadataRetentionDays: request.RunMetadataRetentionDays, Version: request.ExpectedVersion + 1,
		CreatedAt: now, UpdatedAt: now}
	if err := settings.Validate(); err != nil {
		return RuntimePolicy{}, err
	}
	updated, err := service.repository.UpdateRuntimeSettings(ctx, settings, request.ExpectedVersion)
	if err != nil {
		return RuntimePolicy{}, err
	}
	return RuntimePolicy{Settings: updated, Deployment: service.deployment}, nil
}

func (service *Service) authorize(ctx context.Context, access AccessRequest, action authorization.Action) error {
	_, err := service.authorizeDecision(ctx, access, action)
	return err
}

func (service *Service) authorizeDecision(ctx context.Context, access AccessRequest, action authorization.Action) (authorization.Decision, error) {
	if access.WorkspaceID.IsZero() || strings.TrimSpace(access.TraceID) == "" {
		return authorization.Decision{}, domain.ErrInvalidArgument
	}
	if service.authorizer == nil {
		decision := authorization.Decision{Allowed: false, Action: action, ReasonCode: authorization.ReasonNoMatchingGrant}
		return authorization.Decision{}, &authorization.DenialError{Decision: decision}
	}
	decision, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{WorkspaceID: access.WorkspaceID,
		Action: action, Resource: authorization.Resource{Type: authorization.ScopeWorkspace, ID: access.WorkspaceID.UUID()},
		PrincipalRef: strings.TrimSpace(access.PrincipalRef), TraceID: access.TraceID})
	if err != nil {
		return authorization.Decision{}, err
	}
	if !decision.Allowed {
		return authorization.Decision{}, &authorization.DenialError{Decision: decision}
	}
	return decision, nil
}

func auditQuery(request AuditListRequest, limit int) (AuditQuery, error) {
	request.ActorID, request.EventType = strings.TrimSpace(request.ActorID), strings.TrimSpace(request.EventType)
	request.ObjectType, request.ObjectID = strings.TrimSpace(request.ObjectType), strings.TrimSpace(request.ObjectID)
	request.TraceFilter = strings.TrimSpace(request.TraceFilter)
	if request.From != nil && request.To != nil && request.From.After(*request.To) ||
		len(request.ActorID) > 128 || len(request.EventType) > 128 || len(request.ObjectType) > 64 ||
		len(request.ObjectID) > 128 || !validOptionalTrace(request.TraceFilter) || len(request.Cursor) > 2048 {
		return AuditQuery{}, domain.ErrInvalidArgument
	}
	cursorScope := auditCursorScope(request)
	after, err := decodeCursor(request.Cursor, cursorScope, identity.Event)
	if err != nil {
		return AuditQuery{}, err
	}
	return AuditQuery{WorkspaceID: request.WorkspaceID, ActorID: request.ActorID,
		EventType: request.EventType, ObjectType: request.ObjectType, ObjectID: request.ObjectID,
		TraceID: request.TraceFilter, From: request.From, To: request.To, After: after,
		CursorScope: cursorScope, Limit: limit}, nil
}

func projectAuditPage(rows []StoredAuditEvent, requested int, cursorScope string) AuditPage {
	page := AuditPage{Items: make([]domain.AuditEvent, 0, min(len(rows), requested))}
	visible := rows
	if len(visible) > requested {
		visible = visible[:requested]
	}
	for _, row := range visible {
		details := domain.ProjectAuditDetails(row.EventType, row.Payload)
		page.Items = append(page.Items, domain.AuditEvent{ID: row.ID, WorkspaceID: row.WorkspaceID,
			EventType: row.EventType, ActorID: row.ActorID, ObjectType: row.ObjectType,
			ObjectID: row.ObjectID, Channel: stringDetail(details, "channel"),
			Outcome: stringDetail(details, "outcome"), ReasonCode: stringDetail(details, "reasonCode"),
			Summary: row.EventType, TraceID: row.TraceID, Details: details, CreatedAt: row.CreatedAt})
	}
	if len(rows) > requested && requested > 0 {
		last := rows[requested-1]
		page.NextCursor = encodeCursor(last.CreatedAt, last.ID.UUID(), cursorScope)
	}
	return page
}

func pageLimit(value int) int {
	if value <= 0 {
		return DefaultPageSize
	}
	if value > MaxPageSize {
		return MaxPageSize
	}
	return value
}

func encodeCursor(at time.Time, id, scope string) string {
	value, _ := json.Marshal(struct {
		At    string `json:"at"`
		ID    string `json:"id"`
		Scope string `json:"scope"`
	}{At: at.UTC().Format(time.RFC3339Nano), ID: id, Scope: scope})
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeCursor(value, expectedScope string, idPrefix identity.Prefix) (*PageCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, domain.ErrInvalidArgument
	}
	var cursor struct {
		At    string `json:"at"`
		ID    string `json:"id"`
		Scope string `json:"scope"`
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cursor) != nil || decoder.Decode(&struct{}{}) != io.EOF || strings.TrimSpace(cursor.ID) == "" ||
		cursor.Scope != expectedScope {
		return nil, domain.ErrInvalidArgument
	}
	at, err := time.Parse(time.RFC3339Nano, cursor.At)
	if err != nil || at.IsZero() {
		return nil, domain.ErrInvalidArgument
	}
	cursorID, err := cursorUUID(idPrefix, cursor.ID)
	if err != nil {
		return nil, domain.ErrInvalidArgument
	}
	return &PageCursor{Time: at.UTC(), ID: cursorID}, nil
}

func cursorUUID(prefix identity.Prefix, value string) (string, error) {
	if strings.Contains(value, "_") {
		parsed, err := identity.Parse(prefix, value)
		if err != nil {
			return "", err
		}
		return parsed.UUID(), nil
	}
	parsed, err := identity.FromUUID(prefix, value)
	if err != nil {
		return "", err
	}
	return parsed.UUID(), nil
}

type auditExportItem struct {
	ID         identity.EventID `json:"id"`
	EventType  string           `json:"eventType"`
	ActorID    string           `json:"actorId"`
	ObjectType string           `json:"objectType"`
	ObjectID   string           `json:"objectId"`
	Channel    string           `json:"channel"`
	Outcome    string           `json:"outcome"`
	ReasonCode string           `json:"reasonCode"`
	TraceID    string           `json:"traceId"`
	Summary    string           `json:"summary"`
	CreatedAt  time.Time        `json:"createdAt"`
}

type storedAuditFilter struct {
	ActorID    string `json:"actorId"`
	EventType  string `json:"eventType"`
	ObjectType string `json:"objectType"`
	ObjectID   string `json:"objectId"`
	TraceID    string `json:"traceId"`
	From       string `json:"from"`
	To         string `json:"to"`
}

func canonicalAuditFilter(query AuditQuery) (json.RawMessage, error) {
	return json.Marshal(storedAuditFilter{ActorID: query.ActorID, EventType: query.EventType,
		ObjectType: query.ObjectType, ObjectID: query.ObjectID, TraceID: query.TraceID,
		From: canonicalTime(query.From), To: canonicalTime(query.To)})
}

func auditExportRequestFingerprint(filter []byte, creator identity.PrincipalID) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte("audit-export-request-v1\x00json\x001000\x00"))
	_, _ = digest.Write(filter)
	_, _ = digest.Write([]byte{'\x00'})
	_, _ = digest.Write([]byte(creator.String()))
	return hex.EncodeToString(digest.Sum(nil))
}

func auditExportItems(events []domain.AuditEvent) []auditExportItem {
	items := make([]auditExportItem, 0, len(events))
	for _, event := range events {
		items = append(items, auditExportItem{ID: event.ID, EventType: event.EventType, ActorID: event.ActorID,
			ObjectType: event.ObjectType, ObjectID: event.ObjectID, Channel: event.Channel, Outcome: event.Outcome,
			ReasonCode: event.ReasonCode, TraceID: event.TraceID, Summary: event.Summary, CreatedAt: event.CreatedAt.UTC()})
	}
	return items
}

func auditCursorScope(request AuditListRequest) string {
	return cursorScope(struct {
		WorkspaceID, ActorID, EventType, ObjectType, ObjectID, TraceID, From, To string
	}{request.WorkspaceID.String(), request.ActorID, request.EventType, request.ObjectType, request.ObjectID, request.TraceFilter,
		canonicalTime(request.From), canonicalTime(request.To)})
}

func runtimeCursorScope(request RuntimeListRequest) string {
	return cursorScope(struct {
		WorkspaceID, Kind, State, SourceType, SourceID, TraceID string
	}{request.WorkspaceID.String(), string(request.Kind), string(request.State), strings.TrimSpace(request.SourceType),
		strings.TrimSpace(request.SourceID), strings.TrimSpace(request.TraceFilter)})
}

func cursorScope(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func canonicalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func validOptionalTrace(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || traceFilterPattern.MatchString(value)
}

func derivedIdempotencyKey(prefix, source string) string {
	value := prefix + strings.TrimSpace(source)
	if len(value) <= 256 {
		return value
	}
	digest := sha256.Sum256([]byte(source))
	return prefix + hex.EncodeToString(digest[:])
}

func stringDetail(details map[string]any, key string) string {
	value, _ := details[key].(string)
	return value
}

func IsNotFound(err error) bool { return errors.Is(err, domain.ErrNotFound) }
