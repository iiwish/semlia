package ingestion

import (
	"context"
	"errors"
	"testing"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
)

type dueScheduleRepository struct {
	ScheduleRepository
	schedules []domain.Schedule
	now       time.Time
	completed int
}

type scheduleAuthorizationRepository struct {
	ScheduleRepository
	source identity.SourceConnectionID
}

func (repo *scheduleAuthorizationRepository) LookupScheduleSource(context.Context, identity.WorkspaceID, identity.SourceScheduleID) (identity.SourceConnectionID, error) {
	return repo.source, nil
}

type scheduleActionEvaluator struct {
	requests []authorizationapp.EvaluationRequest
	version  int64
}

type resolvedScheduleEvaluator struct {
	principal identity.PrincipalID
	allowed   bool
}

func (value resolvedScheduleEvaluator) Evaluate(_ context.Context, request authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	return authorization.Decision{Allowed: value.allowed, PrincipalID: value.principal, Action: request.Action, AuthorizationVersion: 7}, nil
}

type resolvedScheduleRepository struct {
	ScheduleRepository
	schedule domain.Schedule
	mutation MutateScheduleCommand
}

func (repo *resolvedScheduleRepository) LookupScheduleSource(context.Context, identity.WorkspaceID, identity.SourceScheduleID) (identity.SourceConnectionID, error) {
	return repo.schedule.SourceConnectionID, nil
}
func (repo *resolvedScheduleRepository) GetSchedule(context.Context, identity.WorkspaceID, identity.SourceScheduleID, int64) (domain.Schedule, error) {
	return repo.schedule, nil
}
func (repo *resolvedScheduleRepository) MutateSchedule(_ context.Context, command MutateScheduleCommand) (domain.Schedule, error) {
	repo.mutation = command
	return command.Schedule, nil
}

func TestScheduleAliasCommandsAuthorizeBeforeParsingAndPersistResolvedActor(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	source, _ := identity.NewSourceConnectionID()
	id, _ := identity.NewSourceScheduleID()
	principal, _ := identity.NewPrincipalID()
	for _, action := range []string{"pause", "resume", "delete"} {
		for _, allowed := range []bool{true, false} {
			t.Run(action+map[bool]string{true: "-allowed", false: "-denied"}[allowed], func(t *testing.T) {
				repo := &resolvedScheduleRepository{schedule: domain.Schedule{ID: id, WorkspaceID: workspace, SourceConnectionID: source, Expression: "0 0 1 1 *", Timezone: "UTC", MisfirePolicy: domain.MisfireSkip, Version: 1}}
				service := NewScheduleService(repo, resolvedScheduleEvaluator{principal: principal, allowed: allowed}, ClockFunc(time.Now))
				request := ScheduleCommandRequest{WorkspaceID: workspace, ScheduleID: id, PrincipalRef: "local-author", ExpectedVersion: 1, IdempotencyKey: action}
				var err error
				if action == "delete" {
					_, err = service.Delete(context.Background(), request)
				} else {
					_, err = service.SetEnabled(context.Background(), request, action == "resume")
				}
				if !allowed {
					var denied *authorization.DenialError
					if !errors.As(err, &denied) || !repo.mutation.ActorPrincipalID.IsZero() {
						t.Fatalf("denial=%v mutation=%+v", err, repo.mutation)
					}
					return
				}
				if err != nil || repo.mutation.ActorPrincipalID != principal || repo.mutation.ExpectedAuthorizationVersion != 7 {
					t.Fatalf("error=%v mutation=%+v", err, repo.mutation)
				}
			})
		}
	}
}

func (evaluator *scheduleActionEvaluator) Evaluate(_ context.Context, request authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	evaluator.requests = append(evaluator.requests, request)
	allowed := request.Action != authorization.ActionIngestionRun
	return authorization.Decision{Allowed: allowed, Action: request.Action, AuthorizationVersion: evaluator.version,
		ReasonCode: authorization.ReasonRoleGrant}, nil
}

func (repo *dueScheduleRepository) ClaimDueSchedules(context.Context, string, time.Duration, int) ([]domain.Schedule, time.Time, error) {
	return repo.schedules, repo.now, nil
}

func (repo *dueScheduleRepository) CompleteDueSchedule(_ context.Context, command CompleteDueScheduleCommand) (domain.Occurrence, error) {
	repo.completed++
	return command.Occurrence, nil
}

func TestNextNominalOccurrenceUsesEarlierFallBackOffsetOnce(t *testing.T) {
	now := time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC)
	value, err := NextNominalOccurrence("30 1 * * *", "America/New_York", "", now)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	if value.ScheduledFor == nil || !value.ScheduledFor.Equal(want) || value.WallClockKey != "2026-11-01T01:30" {
		t.Fatalf("occurrence = %+v, want earlier instant %s", value, want)
	}
}

func TestNextNominalOccurrenceJumpsAcrossSparseAndImpossibleCalendars(t *testing.T) {
	started := time.Now()
	yearly, err := NextNominalOccurrence("0 0 1 1 *", "UTC", "", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if err != nil || yearly.WallClockKey != "2027-01-01T00:00" {
		t.Fatalf("yearly occurrence=%+v err=%v", yearly, err)
	}
	if _, err := NextNominalOccurrence("0 0 31 2 *", "UTC", "", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); !errors.Is(err, domain.ErrUnsupported) {
		t.Fatalf("impossible calendar error=%v", err)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("sparse cron resolution took %s", elapsed)
	}
}

func TestNextNominalOccurrenceRecordsSpringGap(t *testing.T) {
	now := time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC)
	value, err := NextNominalOccurrence("30 2 * * *", "America/New_York", "", now)
	if err != nil {
		t.Fatal(err)
	}
	wantEligible := time.Date(2026, 3, 8, 7, 0, 0, 0, time.UTC)
	if !value.DSTGap || value.ScheduledFor != nil || value.WallClockKey != "2026-03-08T02:30" || !value.EligibleAt.Equal(wantEligible) {
		t.Fatalf("gap occurrence = %+v", value)
	}
	next, err := NextNominalOccurrence("30 2 * * *", "America/New_York", value.WallClockKey, value.EligibleAt)
	if err != nil || next.ScheduledFor == nil || next.WallClockKey != "2026-03-09T02:30" {
		t.Fatalf("next occurrence = %+v err=%v", next, err)
	}
}

func TestNextNominalOccurrenceHandlesHalfHourDSTGap(t *testing.T) {
	value, err := NextNominalOccurrence("15 2 * * *", "Australia/Lord_Howe", "", time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 3, 15, 30, 0, 0, time.UTC)
	if !value.DSTGap || !value.EligibleAt.Equal(want) {
		t.Fatalf("half-hour gap = %+v, want boundary %s", value, want)
	}
}

func TestCronDayOfMonthAndWeekdaySemantics(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	everyDay, err := NextNominalOccurrence("0 0 */1 * 1", "UTC", "", now)
	if err != nil || everyDay.WallClockKey != "2026-09-07T00:00" {
		t.Fatalf("wildcard day step = %+v err=%v", everyDay, err)
	}
	either, err := NextNominalOccurrence("0 0 15 * 1", "UTC", "", now)
	if err != nil || either.WallClockKey != "2026-09-07T00:00" {
		t.Fatalf("restricted day OR weekday = %+v err=%v", either, err)
	}
}

func TestParseScheduleRejectsSecondsAndUnknownTimezone(t *testing.T) {
	for _, test := range []struct{ expression, timezone string }{
		{"0 0 0 * * *", "UTC"},
		{"60 * * * *", "UTC"},
		{"* * * * *", "Mars/Olympus"},
	} {
		if ParseSchedule(test.expression, test.timezone) == nil {
			t.Fatalf("accepted expression=%q timezone=%q", test.expression, test.timezone)
		}
	}
}

func TestProcessDueIsolatesMalformedPersistedSchedule(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	source, _ := identity.NewSourceConnectionID()
	invalidID, _ := identity.NewSourceScheduleID()
	validID, _ := identity.NewSourceScheduleID()
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	next := now
	repo := &dueScheduleRepository{now: now, schedules: []domain.Schedule{
		{ID: invalidID, WorkspaceID: workspace, SourceConnectionID: source, Expression: "0 0 31 2 *", Timezone: "UTC",
			MisfirePolicy: domain.MisfireSkip, Enabled: true, NextRunAt: &next, NextWallClockKey: "2026-02-31T00:00", CreatedByPrincipalID: &principal, Version: 1},
		{ID: validID, WorkspaceID: workspace, SourceConnectionID: source, Expression: "* * * * *", Timezone: "UTC",
			MisfirePolicy: domain.MisfireSkip, Enabled: true, NextRunAt: &next, NextWallClockKey: "2026-09-05T12:00", CreatedByPrincipalID: &principal, Version: 1},
	}}
	service := NewScheduleService(repo, nil, ClockFunc(func() time.Time { return now }))
	processed, err := service.ProcessDue(context.Background(), "scheduler-instance", 10, time.Minute)
	if err != nil || processed != 1 || repo.completed != 1 {
		t.Fatalf("processed=%d completed=%d err=%v", processed, repo.completed, err)
	}
}

func TestEnablingSchedulesRequiresIngestionRunInAdditionToSourceManage(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	source, _ := identity.NewSourceConnectionID()
	schedule, _ := identity.NewSourceScheduleID()
	repository := &scheduleAuthorizationRepository{source: source}
	evaluator := &scheduleActionEvaluator{version: 9}
	service := NewScheduleService(repository, evaluator, ClockFunc(time.Now))

	_, err := service.Create(context.Background(), CreateScheduleRequest{WorkspaceID: workspace, SourceID: source,
		Expression: "0 * * * *", Timezone: "UTC", MisfirePolicy: domain.MisfireSkip, IdempotencyKey: "create",
		PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err == nil || len(evaluator.requests) != 2 || evaluator.requests[0].Action != authorization.ActionSourceManage ||
		evaluator.requests[1].Action != authorization.ActionIngestionRun {
		t.Fatalf("create err=%v requests=%+v", err, evaluator.requests)
	}

	evaluator.requests = nil
	_, err = service.Update(context.Background(), UpdateScheduleRequest{WorkspaceID: workspace, ScheduleID: schedule,
		Expression: "0 * * * *", Timezone: "UTC", MisfirePolicy: domain.MisfireSkip, Enabled: true, ExpectedVersion: 1,
		IdempotencyKey: "update", PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err == nil || len(evaluator.requests) != 2 || evaluator.requests[1].Action != authorization.ActionIngestionRun {
		t.Fatalf("enable update err=%v requests=%+v", err, evaluator.requests)
	}

	evaluator.requests = nil
	_, err = service.SetEnabled(context.Background(), ScheduleCommandRequest{WorkspaceID: workspace, ScheduleID: schedule,
		ExpectedVersion: 1, IdempotencyKey: "resume", PrincipalRef: principal.String(),
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}, true)
	if err == nil || len(evaluator.requests) != 2 || evaluator.requests[1].Action != authorization.ActionIngestionRun {
		t.Fatalf("resume err=%v requests=%+v", err, evaluator.requests)
	}
}
