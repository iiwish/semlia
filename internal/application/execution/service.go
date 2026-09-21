package execution

import (
	"context"
	"errors"
	"strings"
	"time"

	authapp "github.com/iiwish/semlia/internal/application/authorization"
	distapp "github.com/iiwish/semlia/internal/application/distribution"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	dist "github.com/iiwish/semlia/internal/domain/distribution"
	d "github.com/iiwish/semlia/internal/domain/execution"
	ops "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/pkg/identity"
)

type Request struct {
	WorkspaceID    identity.WorkspaceID            `json:"-"`
	PlanID         identity.ResolvedSemanticPlanID `json:"planId"`
	PlanDigest     string                          `json:"planDigest"`
	IdempotencyKey string                          `json:"idempotencyKey"`
	Channel        string                          `json:"channel,omitempty"`
	PrincipalRef   string                          `json:"-"`
	TraceID        string                          `json:"-"`
}
type Plans interface {
	ExecutionPlan(context.Context, identity.WorkspaceID, identity.ResolvedSemanticPlanID, string, string) (distapp.ResolutionResult, error)
	ExecutionAccess(context.Context, identity.WorkspaceID, identity.ResolvedSemanticPlanID, string, string) (distapp.ResolutionResult, error)
}
type Repository interface {
	CheckExecutionSource(context.Context, identity.WorkspaceID, *dist.ExecutionProvenance) error
	CheckExecutionCredential(context.Context, authapp.CredentialLimit) error
	ClaimExecution(context.Context, d.Run, int, int64) (d.Run, bool, error)
	FinishExecution(context.Context, d.Run) error
	GetExecution(context.Context, identity.WorkspaceID, string) (d.Run, error)
	GetRuntimeSettings(context.Context, identity.WorkspaceID, time.Time) (ops.RuntimeSettings, error)
	RequestExecutionCancellation(context.Context, identity.WorkspaceID, string) error
	ExecutionCancellationRequested(context.Context, identity.WorkspaceID, string) (bool, error)
}
type Adapter interface {
	Execute(context.Context, identity.WorkspaceID, d.Compiled, d.Limits) d.Output
}
type Service struct {
	repo       Repository
	plans      Plans
	authorizer authapp.Evaluator
	adapter    Adapter
	limits     d.Limits
	now        func() time.Time
}

func NewService(repo Repository, plans Plans, authorizer authapp.Evaluator, adapter Adapter, limits d.Limits) *Service {
	if repo == nil || plans == nil || authorizer == nil || adapter == nil || !limits.Valid() {
		panic("execution requires repository, plans, authorizer, adapter and bounded limits")
	}
	return &Service{repo: repo, plans: plans, authorizer: authorizer, adapter: adapter, limits: limits, now: time.Now}
}
func (s *Service) validate(ctx context.Context, r Request) (distapp.ResolutionResult, auth.Decision, error) {
	if c, ok := authapp.CredentialFromContext(ctx); ok {
		if !c.Allows(authapp.EvaluationRequest{WorkspaceID: r.WorkspaceID, PrincipalRef: r.PrincipalRef, Action: auth.ActionSemanticExecute, Resource: auth.Resource{Type: auth.ScopeWorkspace, ID: r.WorkspaceID.UUID()}}) {
			return distapp.ResolutionResult{}, auth.Decision{}, denied()
		}
		if err := s.repo.CheckExecutionCredential(ctx, c); err != nil {
			return distapp.ResolutionResult{}, auth.Decision{}, denied()
		}
	}
	decision, err := s.authorizer.Evaluate(ctx, authapp.EvaluationRequest{WorkspaceID: r.WorkspaceID, PrincipalRef: r.PrincipalRef, TraceID: r.TraceID, Action: auth.ActionSemanticExecute, Resource: auth.Resource{Type: auth.ScopeWorkspace, ID: r.WorkspaceID.UUID()}})
	if err != nil {
		return distapp.ResolutionResult{}, decision, err
	}
	if !decision.Allowed {
		return distapp.ResolutionResult{}, decision, denied()
	}
	result, err := s.plans.ExecutionPlan(ctx, r.WorkspaceID, r.PlanID, r.PrincipalRef, r.TraceID)
	if err != nil {
		return result, decision, err
	}
	if result.Plan == nil || result.Plan.Execution == nil || result.Plan.PlanDigest != r.PlanDigest {
		return result, decision, d.ErrInvalidPlan
	}
	if c, ok := authapp.CredentialFromContext(ctx); ok && c.ScopeType == auth.ScopeRelease && c.ScopeID != result.Plan.ReleaseID.String() {
		return result, decision, denied()
	}
	seen := map[string]bool{}
	for _, rel := range result.Plan.Execution.Relations {
		if seen[rel.SourceID] {
			continue
		}
		seen[rel.SourceID] = true
		source, err := identity.FromUUID(identity.SourceConnection, rel.SourceID)
		if err != nil {
			return result, decision, d.ErrInvalidPlan
		}
		a, err := s.authorizer.Evaluate(ctx, authapp.EvaluationRequest{WorkspaceID: r.WorkspaceID, PrincipalRef: r.PrincipalRef, TraceID: r.TraceID, Action: auth.ActionSemanticExecute, Resource: auth.Resource{Type: auth.ScopeSource, ID: source.UUID()}})
		if err != nil {
			return result, decision, err
		}
		if !a.Allowed {
			return result, decision, denied()
		}
	}
	if err := s.repo.CheckExecutionSource(ctx, r.WorkspaceID, result.Plan.Execution); err != nil {
		return result, decision, err
	}
	return result, decision, nil
}
func (s *Service) Execute(ctx context.Context, r Request) (d.Result, error) {
	ctx, cancelExecution := context.WithTimeout(ctx, s.limits.Timeout)
	defer cancelExecution()
	if r.WorkspaceID.IsZero() || r.PlanID.IsZero() || len(r.PlanDigest) != 71 || len(r.IdempotencyKey) < 1 || len(r.IdempotencyKey) > 128 || strings.TrimSpace(r.IdempotencyKey) != r.IdempotencyKey {
		return d.Result{}, dist.ErrInvalidArgument
	}
	if r.Channel == "" {
		r.Channel = "api"
	}
	switch r.Channel {
	case "api", "mcp", "cli", "sdk", "web":
	default:
		return d.Result{}, dist.ErrInvalidArgument
	}
	resolved, decision, err := s.validate(ctx, r)
	if err != nil {
		return d.Result{}, err
	}
	settings, err := s.repo.GetRuntimeSettings(ctx, r.WorkspaceID, s.now())
	if err != nil {
		return d.Result{}, errors.New("EXECUTION_POLICY_UNAVAILABLE")
	}
	limits := s.limits
	limits.Timeout = min(limits.Timeout, time.Duration(settings.StatementTimeoutMS)*time.Millisecond)
	limits.MaxRows = min(limits.MaxRows, int(settings.QueryRowLimit))
	limits.MaxBytes = min(limits.MaxBytes, int(settings.QueryByteLimit))
	if !limits.Valid() {
		return d.Result{}, errors.New("EXECUTION_POLICY_UNAVAILABLE")
	}
	if resolved.Plan.Limit > 0 {
		limits.MaxRows = min(limits.MaxRows, resolved.Plan.Limit)
	}
	// The workspace policy shortens the original absolute request deadline.
	ctx, policyCancel := context.WithDeadline(ctx, s.now().Add(limits.Timeout))
	defer policyCancel()
	query, err := d.Compile(*resolved.Plan)
	if err != nil {
		return d.Result{}, err
	}
	id, err := identity.NewRunID()
	if err != nil {
		return d.Result{}, err
	}
	now := s.now().UTC()
	deadline, _ := ctx.Deadline()
	run := d.Run{ID: id.String(), WorkspaceID: r.WorkspaceID.String(), QueryID: resolved.Query.ID.String(), PlanID: r.PlanID.String(), PlanDigest: r.PlanDigest, PrincipalID: decision.PrincipalID.String(), Channel: r.Channel, SourceID: query.SourceID, SourceRevisionID: query.SourceRevisionID, AdapterVersion: d.CompilerVersion, IdempotencyKey: r.IdempotencyKey, State: "running", StartedAt: now, Deadline: deadline.Add(5 * time.Second), TraceID: r.TraceID}
	run.PolicyVersion, run.TimeoutMS, run.MaxRows, run.MaxBytes = settings.Version, limits.Timeout.Milliseconds(), limits.MaxRows, limits.MaxBytes
	if resolved.Query.ConsumerID != nil {
		run.ConsumerID = resolved.Query.ConsumerID.String()
	}
	if c, ok := authapp.CredentialFromContext(ctx); ok {
		run.CredentialID = c.CredentialID.String()
	}
	run, owner, err := s.repo.ClaimExecution(ctx, run, s.limits.MaxConcurrent, decision.AuthorizationVersion)
	if err != nil {
		return d.Result{}, err
	}
	if !owner {
		return publicResult(d.Result{Run: run, Replay: true, Availability: "metadata_only"}), nil
	}
	// A claim is durable before any source connection. Failed/unknown claims are
	// never reclaimed; the caller must intentionally use a new key for a new run.
	output := d.Output{}
	if _, _, err = s.validate(ctx, r); err != nil {
		output.ErrorCode = "EXECUTION_AUTHORIZATION_CHANGED"
	} else {
		watchCtx, stopWatch := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-watchCtx.Done():
					return
				case <-ticker.C:
					requested, checkErr := s.repo.ExecutionCancellationRequested(watchCtx, r.WorkspaceID, run.ID)
					if checkErr != nil || requested {
						cancelExecution()
						return
					}
				}
			}
		}()
		output = s.adapter.Execute(ctx, r.WorkspaceID, query, limits)
		stopWatch()
		<-done
	}
	finished := s.now().UTC()
	run.FinishedAt = &finished
	run.State = "succeeded"
	run.RowCount = output.RowCount
	run.ByteCount = output.ByteCount
	run.ResultDigest = output.Digest
	if output.ErrorCode != "" {
		run.State = "failed"
		run.ErrorCode = output.ErrorCode
		run.ResultDigest = ""
		if output.ErrorCode == "EXECUTION_CANCELLED" {
			run.State = "cancelled"
		}
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.repo.FinishExecution(persistCtx, run); err != nil {
		return d.Result{}, errors.New("EXECUTION_OUTCOME_UNKNOWN")
	}
	run, err = s.repo.GetExecution(persistCtx, r.WorkspaceID, run.ID)
	if err != nil {
		return d.Result{}, errors.New("EXECUTION_OUTCOME_UNKNOWN")
	}
	result := d.Result{Run: run, Availability: "unavailable"}
	if run.State == "succeeded" {
		result.SQL = query.SQL
		result.Parameters = query.Args
		result.Availability = "ephemeral"
		result.Columns = output.Columns
		result.Rows = output.Rows
	}
	return publicResult(result), nil
}
func (s *Service) Get(ctx context.Context, w identity.WorkspaceID, id, principal, trace string) (d.Result, error) {
	return s.getAuthorized(ctx, w, id, principal, trace, true)
}
func (s *Service) getAuthorized(ctx context.Context, w identity.WorkspaceID, id, principal, trace string, fresh bool) (d.Result, error) {
	// Do not expose even metadata before the workspace capability check.
	decision, err := s.authorizer.Evaluate(ctx, authapp.EvaluationRequest{WorkspaceID: w, PrincipalRef: principal, TraceID: trace, Action: auth.ActionSemanticExecute, Resource: auth.Resource{Type: auth.ScopeWorkspace, ID: w.UUID()}})
	if err != nil {
		return d.Result{}, err
	}
	if !decision.Allowed {
		return d.Result{}, denied()
	}
	run, err := s.repo.GetExecution(ctx, w, id)
	if err != nil {
		return d.Result{}, err
	}
	plan, err := identity.ParseResolvedSemanticPlanID(run.PlanID)
	if err != nil {
		return d.Result{}, d.ErrInvalidPlan
	}
	if fresh {
		if _, _, err := s.validate(ctx, Request{WorkspaceID: w, PlanID: plan, PlanDigest: run.PlanDigest, PrincipalRef: principal, TraceID: trace}); err != nil {
			return d.Result{}, err
		}
	} else {
		if c, ok := authapp.CredentialFromContext(ctx); ok {
			if s.repo.CheckExecutionCredential(ctx, c) != nil || !c.Allows(authapp.EvaluationRequest{WorkspaceID: w, PrincipalRef: principal, Action: auth.ActionSemanticExecute, Resource: auth.Resource{Type: auth.ScopeWorkspace, ID: w.UUID()}}) {
				return d.Result{}, denied()
			}
		}
		access, err := s.plans.ExecutionAccess(ctx, w, plan, principal, trace)
		if err != nil {
			return d.Result{}, err
		}
		if c, ok := authapp.CredentialFromContext(ctx); ok && c.ScopeType == auth.ScopeRelease && c.ScopeID != access.Plan.ReleaseID.String() {
			return d.Result{}, denied()
		}
		if access.Plan.Execution == nil {
			return d.Result{}, d.ErrInvalidPlan
		}
		for _, rel := range access.Plan.Execution.Relations {
			source, err := identity.FromUUID(identity.SourceConnection, rel.SourceID)
			if err != nil {
				return d.Result{}, d.ErrInvalidPlan
			}
			decision, err := s.authorizer.Evaluate(ctx, authapp.EvaluationRequest{WorkspaceID: w, PrincipalRef: principal, TraceID: trace, Action: auth.ActionSemanticExecute, Resource: auth.Resource{Type: auth.ScopeSource, ID: source.UUID()}})
			if err != nil {
				return d.Result{}, err
			}
			if !decision.Allowed {
				return d.Result{}, denied()
			}
		}
	}
	if run.PrincipalID != decision.PrincipalID.String() {
		return d.Result{}, denied()
	}
	if c, ok := authapp.CredentialFromContext(ctx); ok && run.CredentialID != c.CredentialID.String() {
		return d.Result{}, denied()
	}
	return publicResult(d.Result{Run: run, Availability: "metadata_only", Replay: true}), nil
}
func (s *Service) Cancel(ctx context.Context, w identity.WorkspaceID, id, principal, trace string) (d.Result, error) {
	result, err := s.getAuthorized(ctx, w, id, principal, trace, false)
	if err != nil {
		return d.Result{}, err
	}
	if result.Run.State != "running" {
		return result, nil
	}
	if err := s.repo.RequestExecutionCancellation(ctx, w, id); err != nil {
		return d.Result{}, err
	}
	return s.getAuthorized(ctx, w, id, principal, trace, false)
}
func publicResult(result d.Result) d.Result {
	for _, item := range []struct {
		value  *string
		prefix identity.Prefix
	}{{&result.Run.SourceID, identity.SourceConnection}, {&result.Run.SourceRevisionID, identity.SourceRevision}} {
		if id, err := identity.FromUUID(item.prefix, *item.value); err == nil {
			*item.value = id.String()
		}
	}
	return result
}
func denied() error {
	return &auth.DenialError{Decision: auth.Decision{ReasonCode: auth.ReasonNoMatchingGrant}}
}
