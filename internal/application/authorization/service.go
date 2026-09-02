// Package authorization provides the single capability-evaluation enforcement
// point reused by every M2 protected command: evaluate(principal, workspace,
// action, resource) → decision{allowed, reasonCode, authorizationVersion}.
// Deny-by-default (NFR-001): anything that does not positively resolve to an
// active principal with a matching scoped binding is denied and audited.
package authorization

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
)

// Evaluator is the contract other application services depend on.
type Evaluator interface {
	Evaluate(ctx context.Context, request EvaluationRequest) (authorization.Decision, error)
}

// EvaluationRequest carries the caller identity as presented on the wire. The
// private-incubation actor contract is the X-Semlia-Principal header holding
// the principal's public TypeID; an empty PrincipalRef selects the workspace's
// system-seeded workspace-admin principal so M1 flows keep working unchanged
// until real authentication replaces the header.
type EvaluationRequest struct {
	PrincipalRef string
	WorkspaceID  identity.WorkspaceID
	Action       authorization.Action
	Resource     authorization.Resource
	TraceID      string
}

type Repository interface {
	LoadPrincipal(ctx context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID) (authorization.Principal, error)
	LoadDefaultPrincipal(ctx context.Context, workspace identity.WorkspaceID) (authorization.Principal, error)
	LoadPrincipalBindings(ctx context.Context, principal identity.PrincipalID) ([]authorization.RoleBinding, error)
	AuthorizationVersion(ctx context.Context, workspace identity.WorkspaceID) (int64, error)
	RecordDecision(ctx context.Context, event authorization.DecisionEvent) error
}

type Clock interface{ Now() time.Time }

type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

type Service struct {
	repository Repository
	clock      Clock
}

func NewService(repository Repository, clock Clock) *Service {
	return &Service{repository: repository, clock: clock}
}

// Evaluate resolves the acting principal, matches scoped role bindings against
// the FR-003 action vocabulary, and records an authorization_events row for
// every decision, allow and deny alike (FR-012, FR-013).
func (service *Service) Evaluate(ctx context.Context, request EvaluationRequest) (authorization.Decision, error) {
	if !authorization.IsKnownAction(request.Action) {
		return service.deny(ctx, request, authorization.ReasonNoMatchingGrant, nil, "")
	}
	principal, actor, resolved, err := service.resolvePrincipal(ctx, request)
	if err != nil {
		return authorization.Decision{}, err
	}
	if !resolved {
		return service.deny(ctx, request, authorization.ReasonNoMatchingGrant, nil, actor)
	}
	if principal.Status != authorization.PrincipalActive {
		return service.deny(ctx, request, authorization.ReasonPrincipalInactive, &principal, actor)
	}
	bindings, err := service.repository.LoadPrincipalBindings(ctx, principal.ID)
	if err != nil {
		return authorization.Decision{}, err
	}
	for _, binding := range bindings {
		if !bindingGrants(binding, request.Action) || !scopeMatches(binding, request.WorkspaceID, request.Resource) {
			continue
		}
		if authorization.RequiresHuman(request.Action) && principal.Kind == authorization.PrincipalAgent {
			return service.deny(ctx, request, authorization.ReasonSeparationOfDuty, &principal, actor)
		}
		version, err := service.repository.AuthorizationVersion(ctx, request.WorkspaceID)
		if err != nil {
			return authorization.Decision{}, err
		}
		decision := authorization.Decision{
			Allowed:              true,
			Action:               request.Action,
			PrincipalID:          principal.ID,
			ReasonCode:           authorization.ReasonRoleGrant,
			AuthorizationVersion: version,
			RoleID:               binding.RoleID,
			BindingID:            binding.ID,
		}
		if err := service.record(ctx, request, decision, &principal, actor); err != nil {
			return authorization.Decision{}, err
		}
		return decision, nil
	}
	return service.deny(ctx, request, authorization.ReasonNoMatchingGrant, &principal, actor)
}

func (service *Service) resolvePrincipal(
	ctx context.Context, request EvaluationRequest,
) (authorization.Principal, string, bool, error) {
	reference := strings.TrimSpace(request.PrincipalRef)
	if reference == "" {
		principal, err := service.repository.LoadDefaultPrincipal(ctx, request.WorkspaceID)
		if err != nil {
			if isNotFound(err) {
				return authorization.Principal{}, "anonymous", false, nil
			}
			return authorization.Principal{}, "", false, err
		}
		return principal, principal.ID.String(), true, nil
	}
	principalID, err := identity.ParsePrincipalID(reference)
	if err != nil {
		return authorization.Principal{}, reference, false, nil
	}
	principal, err := service.repository.LoadPrincipal(ctx, request.WorkspaceID, principalID)
	if err != nil {
		if isNotFound(err) {
			return authorization.Principal{}, reference, false, nil
		}
		return authorization.Principal{}, "", false, err
	}
	return principal, principal.ID.String(), true, nil
}

func bindingGrants(binding authorization.RoleBinding, action authorization.Action) bool {
	for _, granted := range binding.Actions {
		if granted == action {
			return true
		}
	}
	return false
}

// scopeMatches mirrors the frontend scope rules: workspace bindings cover the
// whole workspace, matching-type bindings cover the resource ID, and domain
// bindings cover the resource's semantic domain.
func scopeMatches(binding authorization.RoleBinding, workspace identity.WorkspaceID, resource authorization.Resource) bool {
	if binding.ScopeType == authorization.ScopeWorkspace {
		return binding.ScopeID == workspace.UUID()
	}
	if binding.ScopeType == resource.Type && binding.ScopeID != "" && binding.ScopeID == resource.ID {
		return true
	}
	return binding.ScopeType == authorization.ScopeDomain && resource.DomainID != "" && binding.ScopeID == resource.DomainID
}

func (service *Service) deny(
	ctx context.Context,
	request EvaluationRequest,
	reason authorization.ReasonCode,
	principal *authorization.Principal,
	actor string,
) (authorization.Decision, error) {
	version, err := service.repository.AuthorizationVersion(ctx, request.WorkspaceID)
	if err != nil {
		return authorization.Decision{}, err
	}
	decision := authorization.Decision{
		Allowed:              false,
		Action:               request.Action,
		ReasonCode:           reason,
		AuthorizationVersion: version,
	}
	if principal != nil {
		decision.PrincipalID = principal.ID
	}
	if err := service.record(ctx, request, decision, principal, actor); err != nil {
		return authorization.Decision{}, err
	}
	return decision, nil
}

func (service *Service) record(
	ctx context.Context,
	request EvaluationRequest,
	decision authorization.Decision,
	principal *authorization.Principal,
	actor string,
) error {
	if actor == "" {
		actor = "anonymous"
	}
	event := authorization.DecisionEvent{
		WorkspaceID:          request.WorkspaceID,
		Actor:                actor,
		Action:               request.Action,
		ResourceType:         request.Resource.Type,
		ResourceID:           request.Resource.ID,
		Decision:             "deny",
		ReasonCode:           decision.ReasonCode,
		AuthorizationVersion: decision.AuthorizationVersion,
		TraceID:              request.TraceID,
	}
	if decision.Allowed {
		event.Decision = "allow"
	}
	if principal != nil {
		event.PrincipalID = &principal.ID
	}
	eventID, err := identity.NewEventID()
	if err != nil {
		return err
	}
	event.ID = eventID
	event.CreatedAt = service.clock.Now().UTC()
	return service.repository.RecordDecision(ctx, event)
}

func isNotFound(err error) bool {
	return errors.Is(err, authorization.ErrNotFound)
}
