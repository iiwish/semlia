package ingestion

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
)

type ScheduleRepository interface {
	FindScheduleByCreateIdempotency(context.Context, identity.WorkspaceID, string, string, int64) (*domain.Schedule, error)
	CreateSchedule(context.Context, CreateScheduleCommand) (domain.Schedule, error)
	GetSchedule(context.Context, identity.WorkspaceID, identity.SourceScheduleID, int64) (domain.Schedule, error)
	LookupScheduleSource(context.Context, identity.WorkspaceID, identity.SourceScheduleID) (identity.SourceConnectionID, error)
	ListSchedules(context.Context, ListSchedulesQuery) (domain.Page[domain.Schedule], error)
	MutateSchedule(context.Context, MutateScheduleCommand) (domain.Schedule, error)
	RunScheduleNow(context.Context, RunScheduleNowCommand) (domain.Occurrence, error)
	ClaimDueSchedules(context.Context, string, time.Duration, int) ([]domain.Schedule, time.Time, error)
	CompleteDueSchedule(context.Context, CompleteDueScheduleCommand) (domain.Occurrence, error)
	ListScheduleOccurrences(context.Context, ListOccurrencesQuery) (domain.Page[domain.Occurrence], error)
}

type CreateScheduleCommand struct {
	Schedule                     domain.Schedule
	IdempotencyKey               string
	RequestFingerprint           string
	TraceID                      string
	ExpectedAuthorizationVersion int64
}

type ListSchedulesQuery struct {
	WorkspaceID          identity.WorkspaceID
	SourceID             *identity.SourceConnectionID
	Enabled              *bool
	Cursor               string
	Limit                int
	PrincipalID          identity.PrincipalID
	AuthorizationVersion int64
}

type MutateScheduleCommand struct {
	Schedule                     domain.Schedule
	Action                       string
	IdempotencyKey               string
	RequestFingerprint           string
	ExpectedVersion              int64
	ExpectedAuthorizationVersion int64
	TraceID                      string
	ActorPrincipalID             identity.PrincipalID
}

type RunScheduleNowCommand struct {
	WorkspaceID                  identity.WorkspaceID
	ScheduleID                   identity.SourceScheduleID
	OccurrenceID                 identity.ScheduleOccurrenceID
	RunID                        identity.RunID
	JobID                        identity.RunID
	RequestedBy                  identity.PrincipalID
	IdempotencyKey               string
	RequestFingerprint           string
	ExpectedAuthorizationVersion int64
	ExpectedVersion              int64
	TraceID                      string
	CreatedAt                    time.Time
}

type CompleteDueScheduleCommand struct {
	Schedule   domain.Schedule
	Occurrence domain.Occurrence
	Next       NominalOccurrence
	LeaseOwner string
	Now        time.Time
	TraceID    string
	RunID      identity.RunID
	JobID      identity.RunID
}

type ListOccurrencesQuery struct {
	WorkspaceID          identity.WorkspaceID
	ScheduleID           identity.SourceScheduleID
	Cursor               string
	Limit                int
	PrincipalID          identity.PrincipalID
	AuthorizationVersion int64
}

type CreateScheduleRequest struct {
	WorkspaceID    identity.WorkspaceID
	SourceID       identity.SourceConnectionID
	Expression     string
	Timezone       string
	MisfirePolicy  domain.MisfirePolicy
	IdempotencyKey string
	PrincipalRef   string
	TraceID        string
}

type UpdateScheduleRequest struct {
	WorkspaceID     identity.WorkspaceID
	ScheduleID      identity.SourceScheduleID
	Expression      string
	Timezone        string
	MisfirePolicy   domain.MisfirePolicy
	Enabled         bool
	ExpectedVersion int64
	IdempotencyKey  string
	PrincipalRef    string
	TraceID         string
}

type ScheduleCommandRequest struct {
	WorkspaceID     identity.WorkspaceID
	ScheduleID      identity.SourceScheduleID
	ExpectedVersion int64
	IdempotencyKey  string
	PrincipalRef    string
	TraceID         string
}

type ScheduleService struct {
	repository ScheduleRepository
	authorizer authorizationapp.Evaluator
	clock      Clock
}

func NewScheduleService(repository ScheduleRepository, authorizer authorizationapp.Evaluator, clock Clock) *ScheduleService {
	return &ScheduleService{repository: repository, authorizer: authorizer, clock: clock}
}

func (service *ScheduleService) Create(ctx context.Context, request CreateScheduleRequest) (domain.Schedule, error) {
	authVersion, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionSourceManage,
		authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.Schedule{}, err
	}
	if err := service.authorizeAtVersion(ctx, request.WorkspaceID, authorization.ActionIngestionRun,
		authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}, request.PrincipalRef, request.TraceID, authVersion); err != nil {
		return domain.Schedule{}, err
	}
	request.Expression, request.Timezone, request.IdempotencyKey = strings.TrimSpace(request.Expression), strings.TrimSpace(request.Timezone), strings.TrimSpace(request.IdempotencyKey)
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(request.PrincipalRef))
	if err != nil || !request.MisfirePolicy.Valid() || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 256 ||
		len(request.Expression) > 128 || len(request.Timezone) > 128 || ParseSchedule(request.Expression, request.Timezone) != nil {
		return domain.Schedule{}, domain.ErrInvalid
	}
	fingerprint := digestJSON(struct {
		WorkspaceID   string               `json:"workspaceId"`
		SourceID      string               `json:"sourceId"`
		PrincipalID   string               `json:"principalId"`
		Expression    string               `json:"expression"`
		Timezone      string               `json:"timezone"`
		MisfirePolicy domain.MisfirePolicy `json:"misfirePolicy"`
	}{request.WorkspaceID.String(), request.SourceID.String(), principal.String(), request.Expression, request.Timezone, request.MisfirePolicy})
	if existing, findErr := service.repository.FindScheduleByCreateIdempotency(ctx, request.WorkspaceID, request.IdempotencyKey, fingerprint, authVersion); findErr != nil || existing != nil {
		if existing != nil {
			return *existing, findErr
		}
		return domain.Schedule{}, findErr
	}
	next, err := NextNominalOccurrence(request.Expression, request.Timezone, "", service.clock.Now().UTC())
	if err != nil {
		return domain.Schedule{}, err
	}
	id, err := identity.NewSourceScheduleID()
	if err != nil {
		return domain.Schedule{}, err
	}
	now := service.clock.Now().UTC()
	schedule := domain.Schedule{ID: id, WorkspaceID: request.WorkspaceID, SourceConnectionID: request.SourceID,
		Expression: request.Expression, Timezone: request.Timezone, MisfirePolicy: request.MisfirePolicy, Enabled: true,
		NextRunAt: &next.EligibleAt, NextWallClockKey: next.WallClockKey, CreatedByPrincipalID: &principal,
		Version: 1, CreatedAt: now, UpdatedAt: now}
	return service.repository.CreateSchedule(ctx, CreateScheduleCommand{Schedule: schedule, IdempotencyKey: request.IdempotencyKey,
		RequestFingerprint: fingerprint, TraceID: request.TraceID, ExpectedAuthorizationVersion: authVersion})
}

func (service *ScheduleService) List(ctx context.Context, query ListSchedulesQuery, principalRef, traceID string) (domain.Page[domain.Schedule], error) {
	resource := authorization.Resource{Type: authorization.ScopeWorkspace, ID: query.WorkspaceID.UUID()}
	if query.SourceID != nil {
		resource = authorization.Resource{Type: authorization.ScopeSource, ID: query.SourceID.UUID()}
	}
	authVersion, err := service.authorize(ctx, query.WorkspaceID, authorization.ActionSourceRead,
		resource, &principalRef, traceID)
	if err != nil {
		return domain.Page[domain.Schedule]{}, err
	}
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(principalRef))
	if err != nil {
		return domain.Page[domain.Schedule]{}, domain.ErrInvalid
	}
	query.PrincipalID, query.AuthorizationVersion = principal, authVersion
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.Limit < 1 || query.Limit > 100 {
		return domain.Page[domain.Schedule]{}, domain.ErrInvalid
	}
	return service.repository.ListSchedules(ctx, query)
}

func (service *ScheduleService) Get(ctx context.Context, workspace identity.WorkspaceID, scheduleID identity.SourceScheduleID, principalRef, traceID string) (domain.Schedule, error) {
	sourceID, err := service.repository.LookupScheduleSource(ctx, workspace, scheduleID)
	if err != nil {
		return domain.Schedule{}, err
	}
	authVersion, err := service.authorize(ctx, workspace, authorization.ActionSourceRead,
		authorization.Resource{Type: authorization.ScopeSource, ID: sourceID.UUID()}, &principalRef, traceID)
	if err != nil {
		return domain.Schedule{}, err
	}
	return service.repository.GetSchedule(ctx, workspace, scheduleID, authVersion)
}

func (service *ScheduleService) Update(ctx context.Context, request UpdateScheduleRequest) (domain.Schedule, error) {
	request.Expression, request.Timezone, request.IdempotencyKey = strings.TrimSpace(request.Expression), strings.TrimSpace(request.Timezone), strings.TrimSpace(request.IdempotencyKey)
	if request.ExpectedVersion < 1 || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 256 || !request.MisfirePolicy.Valid() ||
		len(request.Expression) > 128 || len(request.Timezone) > 128 || ParseSchedule(request.Expression, request.Timezone) != nil {
		return domain.Schedule{}, domain.ErrInvalid
	}
	sourceID, err := service.repository.LookupScheduleSource(ctx, request.WorkspaceID, request.ScheduleID)
	if err != nil {
		return domain.Schedule{}, err
	}
	authVersion, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionSourceManage,
		authorization.Resource{Type: authorization.ScopeSource, ID: sourceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.Schedule{}, err
	}
	if request.Enabled {
		if err := service.authorizeAtVersion(ctx, request.WorkspaceID, authorization.ActionIngestionRun,
			authorization.Resource{Type: authorization.ScopeSource, ID: sourceID.UUID()}, request.PrincipalRef, request.TraceID, authVersion); err != nil {
			return domain.Schedule{}, err
		}
	}
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(request.PrincipalRef))
	if err != nil {
		return domain.Schedule{}, domain.ErrInvalid
	}
	current, err := service.repository.GetSchedule(ctx, request.WorkspaceID, request.ScheduleID, authVersion)
	if err != nil {
		return domain.Schedule{}, err
	}
	next, err := NextNominalOccurrence(request.Expression, request.Timezone, "", service.clock.Now().UTC())
	if err != nil {
		return domain.Schedule{}, err
	}
	current.Expression, current.Timezone, current.MisfirePolicy, current.Enabled = request.Expression, request.Timezone, request.MisfirePolicy, request.Enabled
	current.NextRunAt, current.NextWallClockKey = nil, ""
	if request.Enabled {
		current.NextRunAt, current.NextWallClockKey = &next.EligibleAt, next.WallClockKey
	}
	current.UpdatedAt = service.clock.Now().UTC()
	fingerprint := digestJSON(struct {
		Workspace  string               `json:"workspaceId"`
		Schedule   string               `json:"scheduleId"`
		Principal  string               `json:"principal"`
		Action     string               `json:"action"`
		Expression string               `json:"expression"`
		Timezone   string               `json:"timezone"`
		Misfire    domain.MisfirePolicy `json:"misfirePolicy"`
		Enabled    bool                 `json:"enabled"`
		Expected   int64                `json:"expectedVersion"`
	}{request.WorkspaceID.String(), request.ScheduleID.String(), strings.TrimSpace(request.PrincipalRef), "update", request.Expression, request.Timezone, request.MisfirePolicy, request.Enabled, request.ExpectedVersion})
	return service.repository.MutateSchedule(ctx, MutateScheduleCommand{Schedule: current, Action: "update", IdempotencyKey: request.IdempotencyKey,
		RequestFingerprint: fingerprint, ExpectedVersion: request.ExpectedVersion, ExpectedAuthorizationVersion: authVersion,
		TraceID: request.TraceID, ActorPrincipalID: principal})
}

func (service *ScheduleService) SetEnabled(ctx context.Context, request ScheduleCommandRequest, enabled bool) (domain.Schedule, error) {
	sourceID, err := service.repository.LookupScheduleSource(ctx, request.WorkspaceID, request.ScheduleID)
	if err != nil {
		return domain.Schedule{}, err
	}
	authVersion, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionSourceManage,
		authorization.Resource{Type: authorization.ScopeSource, ID: sourceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.Schedule{}, err
	}
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(request.PrincipalRef))
	if err != nil {
		return domain.Schedule{}, domain.ErrInvalid
	}
	if enabled {
		if err := service.authorizeAtVersion(ctx, request.WorkspaceID, authorization.ActionIngestionRun,
			authorization.Resource{Type: authorization.ScopeSource, ID: sourceID.UUID()}, request.PrincipalRef, request.TraceID, authVersion); err != nil {
			return domain.Schedule{}, err
		}
	}
	current, err := service.repository.GetSchedule(ctx, request.WorkspaceID, request.ScheduleID, authVersion)
	if err != nil {
		return domain.Schedule{}, err
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.ExpectedVersion < 1 || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 256 {
		return domain.Schedule{}, domain.ErrInvalid
	}
	action := "pause"
	current.NextRunAt, current.NextWallClockKey = nil, ""
	if enabled {
		action = "resume"
		next, nextErr := NextNominalOccurrence(current.Expression, current.Timezone, "", service.clock.Now().UTC())
		if nextErr != nil {
			return domain.Schedule{}, nextErr
		}
		current.NextRunAt, current.NextWallClockKey = &next.EligibleAt, next.WallClockKey
	}
	current.Enabled, current.UpdatedAt = enabled, service.clock.Now().UTC()
	fingerprint := digestJSON(struct {
		Workspace string `json:"workspaceId"`
		Schedule  string `json:"scheduleId"`
		Principal string `json:"principal"`
		Action    string `json:"action"`
		Expected  int64  `json:"expectedVersion"`
	}{request.WorkspaceID.String(), request.ScheduleID.String(), strings.TrimSpace(request.PrincipalRef), action, request.ExpectedVersion})
	return service.repository.MutateSchedule(ctx, MutateScheduleCommand{Schedule: current, Action: action, IdempotencyKey: request.IdempotencyKey,
		RequestFingerprint: fingerprint, ExpectedVersion: request.ExpectedVersion, ExpectedAuthorizationVersion: authVersion,
		TraceID: request.TraceID, ActorPrincipalID: principal})
}

func (service *ScheduleService) RunNow(ctx context.Context, request ScheduleCommandRequest) (domain.Occurrence, error) {
	sourceID, err := service.repository.LookupScheduleSource(ctx, request.WorkspaceID, request.ScheduleID)
	if err != nil {
		return domain.Occurrence{}, err
	}
	authVersion, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionIngestionRun,
		authorization.Resource{Type: authorization.ScopeSource, ID: sourceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.Occurrence{}, err
	}
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(request.PrincipalRef))
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if err != nil || request.ExpectedVersion < 1 || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 256 {
		return domain.Occurrence{}, domain.ErrInvalid
	}
	occurrenceID, err := identity.NewScheduleOccurrenceID()
	if err != nil {
		return domain.Occurrence{}, err
	}
	runID, err := identity.NewRunID()
	if err != nil {
		return domain.Occurrence{}, err
	}
	jobID, err := identity.NewRunID()
	if err != nil {
		return domain.Occurrence{}, err
	}
	fingerprint := digestJSON(struct {
		Workspace string `json:"workspaceId"`
		Schedule  string `json:"scheduleId"`
		Principal string `json:"principal"`
		Action    string `json:"action"`
		Expected  int64  `json:"expectedVersion"`
	}{request.WorkspaceID.String(), request.ScheduleID.String(), principal.String(), "run_now", request.ExpectedVersion})
	return service.repository.RunScheduleNow(ctx, RunScheduleNowCommand{WorkspaceID: request.WorkspaceID, ScheduleID: request.ScheduleID,
		OccurrenceID: occurrenceID, RunID: runID, JobID: jobID, RequestedBy: principal, IdempotencyKey: request.IdempotencyKey,
		RequestFingerprint: fingerprint, ExpectedAuthorizationVersion: authVersion, ExpectedVersion: request.ExpectedVersion, TraceID: request.TraceID, CreatedAt: service.clock.Now().UTC()})
}

func (service *ScheduleService) Delete(ctx context.Context, request ScheduleCommandRequest) (domain.Schedule, error) {
	sourceID, err := service.repository.LookupScheduleSource(ctx, request.WorkspaceID, request.ScheduleID)
	if err != nil {
		return domain.Schedule{}, err
	}
	authVersion, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionSourceManage,
		authorization.Resource{Type: authorization.ScopeSource, ID: sourceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.Schedule{}, err
	}
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(request.PrincipalRef))
	if err != nil {
		return domain.Schedule{}, domain.ErrInvalid
	}
	current, err := service.repository.GetSchedule(ctx, request.WorkspaceID, request.ScheduleID, authVersion)
	if err != nil {
		return domain.Schedule{}, err
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.ExpectedVersion < 1 || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 256 {
		return domain.Schedule{}, domain.ErrInvalid
	}
	now := service.clock.Now().UTC()
	current.Enabled, current.NextRunAt, current.NextWallClockKey, current.DeletedAt, current.UpdatedAt = false, nil, "", &now, now
	fingerprint := digestJSON(struct {
		Workspace string `json:"workspaceId"`
		Schedule  string `json:"scheduleId"`
		Principal string `json:"principal"`
		Action    string `json:"action"`
		Expected  int64  `json:"expectedVersion"`
	}{request.WorkspaceID.String(), request.ScheduleID.String(), strings.TrimSpace(request.PrincipalRef), "delete", request.ExpectedVersion})
	return service.repository.MutateSchedule(ctx, MutateScheduleCommand{Schedule: current, Action: "delete", IdempotencyKey: request.IdempotencyKey,
		RequestFingerprint: fingerprint, ExpectedVersion: request.ExpectedVersion, ExpectedAuthorizationVersion: authVersion,
		TraceID: request.TraceID, ActorPrincipalID: principal})
}

func (service *ScheduleService) ListOccurrences(ctx context.Context, query ListOccurrencesQuery, principalRef, traceID string) (domain.Page[domain.Occurrence], error) {
	sourceID, err := service.repository.LookupScheduleSource(ctx, query.WorkspaceID, query.ScheduleID)
	if err != nil {
		return domain.Page[domain.Occurrence]{}, err
	}
	authVersion, err := service.authorize(ctx, query.WorkspaceID, authorization.ActionSourceRead,
		authorization.Resource{Type: authorization.ScopeSource, ID: sourceID.UUID()}, &principalRef, traceID)
	if err != nil {
		return domain.Page[domain.Occurrence]{}, err
	}
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(principalRef))
	if err != nil {
		return domain.Page[domain.Occurrence]{}, domain.ErrInvalid
	}
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.Limit < 1 || query.Limit > 100 {
		return domain.Page[domain.Occurrence]{}, domain.ErrInvalid
	}
	query.PrincipalID, query.AuthorizationVersion = principal, authVersion
	return service.repository.ListScheduleOccurrences(ctx, query)
}

func (service *ScheduleService) ProcessDue(ctx context.Context, owner string, limit int, lease time.Duration) (int, error) {
	if strings.TrimSpace(owner) == "" || limit < 1 || limit > 100 || lease <= 0 {
		return 0, domain.ErrInvalid
	}
	schedules, now, err := service.repository.ClaimDueSchedules(ctx, owner, lease, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, schedule := range schedules {
		if schedule.CreatedByPrincipalID == nil {
			continue
		}
		current, resolveErr := occurrenceAt(schedule.Timezone, schedule.NextWallClockKey)
		if resolveErr != nil {
			continue
		}
		next, nextErr := NextNominalOccurrence(schedule.Expression, schedule.Timezone, schedule.NextWallClockKey, now)
		if nextErr != nil {
			continue
		}
		occurrenceID, idErr := identity.NewScheduleOccurrenceID()
		if idErr != nil {
			return processed, idErr
		}
		runID, idErr := identity.NewRunID()
		if idErr != nil {
			return processed, idErr
		}
		jobID, idErr := identity.NewRunID()
		if idErr != nil {
			return processed, idErr
		}
		state, reason, disposition := "enqueued", "", "on_time"
		if current.DSTGap {
			state, reason, disposition = "skipped", "DST_GAP", ""
		} else if schedule.NextRunAt != nil && now.After(schedule.NextRunAt.Add(time.Minute)) {
			if schedule.MisfirePolicy == domain.MisfireSkip {
				state, reason, disposition = "skipped", "MISFIRE_SKIPPED", ""
			} else {
				disposition = "coalesced"
			}
			next, nextErr = NextNominalOccurrence(schedule.Expression, schedule.Timezone, "", now)
			if nextErr != nil {
				continue
			}
		}
		occurrence := domain.Occurrence{ID: occurrenceID, WorkspaceID: schedule.WorkspaceID, ScheduleID: schedule.ID,
			SourceConnectionID: schedule.SourceConnectionID, TriggerKind: "scheduled", ScheduleVersion: schedule.Version,
			ScheduledFor: current.ScheduledFor, EligibleAt: current.EligibleAt, WallClockKey: current.WallClockKey,
			State: state, ReasonCode: reason, MisfireDisposition: disposition, RequestedBy: *schedule.CreatedByPrincipalID,
			IdempotencyKey: "scheduled:" + schedule.ID.String() + ":v" + strconv.FormatInt(schedule.Version, 10) + ":" + current.WallClockKey, CreatedAt: now}
		traceID := stableTraceID(occurrence.ID.String())
		if _, err := service.repository.CompleteDueSchedule(ctx, CompleteDueScheduleCommand{Schedule: schedule, Occurrence: occurrence,
			Next: next, LeaseOwner: owner, Now: now, TraceID: traceID, RunID: runID, JobID: jobID}); err != nil {
			if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNotFound) {
				continue
			}
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func stableTraceID(value string) string {
	return strings.TrimPrefix(digestJSON(value), "sha256:")[:32]
}

func occurrenceAt(timezone, wallKey string) (NominalOccurrence, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return NominalOccurrence{}, domain.ErrInvalid
	}
	nominal, err := time.ParseInLocation("2006-01-02T15:04", wallKey, time.UTC)
	if err != nil {
		return NominalOccurrence{}, domain.ErrInvalid
	}
	instants, err := wallClockInstants(location, nominal)
	if err != nil {
		return NominalOccurrence{}, err
	}
	if len(instants) == 0 {
		eligible, gapErr := postGapBoundary(location, nominal)
		return NominalOccurrence{WallClockKey: wallKey, EligibleAt: eligible, DSTGap: true}, gapErr
	}
	earlier := instants[0].UTC()
	return NominalOccurrence{WallClockKey: wallKey, ScheduledFor: &earlier, EligibleAt: earlier}, nil
}

func (service *ScheduleService) authorize(ctx context.Context, workspace identity.WorkspaceID, action authorization.Action,
	resource authorization.Resource, principalRef *string, traceID string) (int64, error) {
	if service == nil || service.repository == nil || service.clock == nil {
		return 0, domain.ErrInvalid
	}
	if service.authorizer == nil {
		return 0, nil
	}
	decision, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{PrincipalRef: strings.TrimSpace(*principalRef),
		WorkspaceID: workspace, Action: action, Resource: resource, TraceID: traceID})
	if err != nil {
		return 0, err
	}
	if !decision.Allowed {
		return 0, &authorization.DenialError{Decision: decision}
	}
	// Persist the principal resolved by this decision, not a development alias.
	if !decision.PrincipalID.IsZero() {
		*principalRef = decision.PrincipalID.String()
	}
	return decision.AuthorizationVersion, nil
}

func (service *ScheduleService) authorizeAtVersion(ctx context.Context, workspace identity.WorkspaceID, action authorization.Action,
	resource authorization.Resource, principalRef, traceID string, expectedVersion int64,
) error {
	version, err := service.authorize(ctx, workspace, action, resource, &principalRef, traceID)
	if err != nil {
		return err
	}
	if version != expectedVersion {
		return domain.ErrConflict
	}
	return nil
}
