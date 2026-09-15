// Package authorization provides the single capability-evaluation enforcement
// point reused by every M2 protected command: evaluate(principal, workspace,
// action, resource) → decision{allowed, reasonCode, authorizationVersion}.
// Deny-by-default (NFR-001): anything that does not positively resolve to an
// active principal with a matching scoped binding is denied and audited.
package authorization

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
)

// Evaluator is the contract other application services depend on.
type Evaluator interface {
	Evaluate(ctx context.Context, request EvaluationRequest) (authorization.Decision, error)
}

// SnapshotEvaluator performs one audited request-level decision and then
// exposes the same principal's frozen-version grants for pure, read-only
// checks over a collection. Snapshot never records authorization events or
// expires bindings as a side effect.
type SnapshotEvaluator interface {
	Evaluator
	Snapshot(ctx context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID) (AccessSnapshot, error)
}

type AccessSnapshot struct {
	Principal            authorization.Principal
	Bindings             []authorization.RoleBinding
	AuthorizationVersion int64
	EvaluatedAt          time.Time
}

type FrozenGrant struct {
	RoleID    string
	Action    authorization.Action
	ScopeType authorization.ScopeType
	ScopeID   string
}

func (snapshot AccessSnapshot) Allows(action authorization.Action, resource authorization.Resource) bool {
	if snapshot.Principal.Status != authorization.PrincipalActive || !authorization.IsKnownAction(action) {
		return false
	}
	if authorization.RequiresHuman(action) && snapshot.Principal.Kind == authorization.PrincipalAgent {
		return false
	}
	for _, binding := range snapshot.Bindings {
		if binding.WorkspaceID != snapshot.Principal.WorkspaceID ||
			!bindingGrants(binding, action, snapshot.EvaluatedAt) ||
			!scopeMatches(binding, snapshot.Principal.WorkspaceID, resource) {
			continue
		}
		return true
	}
	return false
}

// ActiveRoleIDs is used only as an audience-membership projection. Permission
// checks remain in Allows and never move into caller SQL.
func (snapshot AccessSnapshot) ActiveRoleIDs() []string {
	seen := make(map[string]struct{}, len(snapshot.Bindings))
	for _, binding := range snapshot.Bindings {
		if binding.WorkspaceID != snapshot.Principal.WorkspaceID ||
			binding.StatusAt(snapshot.EvaluatedAt) != authorization.BindingActive || binding.RoleID == "" {
			continue
		}
		seen[binding.RoleID] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for roleID := range seen {
		result = append(result, roleID)
	}
	sort.Strings(result)
	return result
}

func (snapshot AccessSnapshot) ActiveGrants() []FrozenGrant {
	if snapshot.Principal.Status != authorization.PrincipalActive {
		return nil
	}
	result := make([]FrozenGrant, 0, len(snapshot.Bindings))
	for _, binding := range snapshot.Bindings {
		if binding.WorkspaceID != snapshot.Principal.WorkspaceID ||
			binding.StatusAt(snapshot.EvaluatedAt) != authorization.BindingActive {
			continue
		}
		for _, action := range binding.Actions {
			if authorization.RequiresHuman(action) && snapshot.Principal.Kind == authorization.PrincipalAgent {
				continue
			}
			result = append(result, FrozenGrant{RoleID: binding.RoleID, Action: action,
				ScopeType: binding.ScopeType, ScopeID: binding.ScopeID})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].RoleID != result[right].RoleID {
			return result[left].RoleID < result[right].RoleID
		}
		if result[left].Action != result[right].Action {
			return result[left].Action < result[right].Action
		}
		if result[left].ScopeType != result[right].ScopeType {
			return result[left].ScopeType < result[right].ScopeType
		}
		return result[left].ScopeID < result[right].ScopeID
	})
	return result
}

// EvaluationRequest carries the caller identity as presented on the wire. The
// private-incubation actor contract is the X-Semlia-Principal header holding
// the principal's public TypeID. Development can explicitly enable symbolic
// local-UAT aliases; otherwise a missing or malformed identity is denied.
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
	LoadLocalUATPrincipal(ctx context.Context, workspace identity.WorkspaceID, seedNamespace, roleID string) (authorization.Principal, error)
	LoadPrincipalBindings(ctx context.Context, principal identity.PrincipalID) ([]authorization.RoleBinding, error)
	AuthorizationVersion(ctx context.Context, workspace identity.WorkspaceID) (int64, error)
	RecordDecision(ctx context.Context, event authorization.DecisionEvent) error
}

type Clock interface{ Now() time.Time }

type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

type Service struct {
	repository         Repository
	clock              Clock
	localUATIdentities bool
}

type Option func(*Service)

const (
	LocalUATAuthorPrincipalRef     = "local-author"
	LocalUATReviewerPrincipalRef   = "local-reviewer"
	LocalUATPublisherPrincipalRef  = "local-publisher"
	localUATAuthorSeedNamespace    = "principal"
	localUATReviewerSeedNamespace  = "independent_reviewer"
	localUATPublisherSeedNamespace = "independent_publisher"
)

// WithLocalUATIdentities enables explicit author, reviewer, and publisher
// aliases for the local, development-only acceptance journey. It is not an
// authentication mechanism.
func WithLocalUATIdentities() Option {
	return func(service *Service) { service.localUATIdentities = true }
}

func NewService(repository Repository, clock Clock, options ...Option) *Service {
	service := &Service{repository: repository, clock: clock}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

// Evaluate resolves the acting principal, matches scoped role bindings against
// the FR-003 action vocabulary, and records an authorization_events row for
// every decision, allow and deny alike (FR-012, FR-013).
func (service *Service) Evaluate(ctx context.Context, request EvaluationRequest) (authorization.Decision, error) {
	if limit, ok := CredentialFromContext(ctx); ok && !limit.Allows(request) {
		return service.deny(ctx, request, authorization.ReasonNoMatchingGrant, nil, request.PrincipalRef)
	}
	if !authorization.IsKnownAction(request.Action) {
		return service.deny(ctx, request, authorization.ReasonNoMatchingGrant, nil, "")
	}
	if expirer, ok := service.repository.(BindingExpiryRepository); ok && !request.WorkspaceID.IsZero() {
		eventID, err := identity.NewEventID()
		if err != nil {
			return authorization.Decision{}, err
		}
		if err := expirer.ExpireRoleBindings(ctx, ExpireRoleBindingsRequest{
			WorkspaceID: request.WorkspaceID, Now: service.clock.Now().UTC(),
			TraceID: request.TraceID, EventID: eventID,
		}); err != nil {
			return authorization.Decision{}, err
		}
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
		if !bindingGrants(binding, request.Action, service.clock.Now().UTC()) || !scopeMatches(binding, request.WorkspaceID, request.Resource) {
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

func (service *Service) Snapshot(
	ctx context.Context, workspace identity.WorkspaceID, principalID identity.PrincipalID,
) (AccessSnapshot, error) {
	if workspace.IsZero() || principalID.IsZero() {
		return AccessSnapshot{}, authorization.ErrInvalidArgument
	}
	// A snapshot spans several repository reads. Bind each attempt to the
	// workspace authorization version on both sides so a concurrent revoke or
	// immutable role-version advance can never label stale grants as current.
	// Three attempts keep contention bounded; callers retry the request if the
	// workspace continues changing.
	for attempt := 0; attempt < 3; attempt++ {
		before, err := service.repository.AuthorizationVersion(ctx, workspace)
		if err != nil {
			return AccessSnapshot{}, err
		}
		principal, err := service.repository.LoadPrincipal(ctx, workspace, principalID)
		if err != nil {
			return AccessSnapshot{}, err
		}
		bindings, err := service.repository.LoadPrincipalBindings(ctx, principal.ID)
		if err != nil {
			return AccessSnapshot{}, err
		}
		after, err := service.repository.AuthorizationVersion(ctx, workspace)
		if err != nil {
			return AccessSnapshot{}, err
		}
		if before != after {
			continue
		}
		snapshot := AccessSnapshot{
			Principal: principal, Bindings: bindings, AuthorizationVersion: after,
			EvaluatedAt: service.clock.Now().UTC(),
		}
		return snapshot, nil
	}
	return AccessSnapshot{}, authorization.ErrVersionConflict
}

func (service *Service) resolvePrincipal(
	ctx context.Context, request EvaluationRequest,
) (authorization.Principal, string, bool, error) {
	reference := strings.TrimSpace(request.PrincipalRef)
	if reference == "" {
		return authorization.Principal{}, "anonymous", false, nil
	}
	if service.localUATIdentities {
		seedNamespace, roleID := "", ""
		switch reference {
		case LocalUATAuthorPrincipalRef:
			seedNamespace, roleID = localUATAuthorSeedNamespace, "workspace_admin"
		case LocalUATReviewerPrincipalRef:
			seedNamespace, roleID = localUATReviewerSeedNamespace, "reviewer"
		case LocalUATPublisherPrincipalRef:
			seedNamespace, roleID = localUATPublisherSeedNamespace, "publisher"
		}
		if seedNamespace != "" {
			return service.resolveLocalUATPrincipal(ctx, request.WorkspaceID, reference, seedNamespace, roleID)
		}
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

func (service *Service) resolveLocalUATPrincipal(
	ctx context.Context,
	workspace identity.WorkspaceID,
	reference, seedNamespace, roleID string,
) (authorization.Principal, string, bool, error) {
	principal, err := service.repository.LoadLocalUATPrincipal(ctx, workspace, seedNamespace, roleID)
	if err != nil {
		if isNotFound(err) {
			return authorization.Principal{}, reference, false, nil
		}
		return authorization.Principal{}, "", false, err
	}
	return principal, principal.ID.String(), true, nil
}

func bindingGrants(binding authorization.RoleBinding, action authorization.Action, now time.Time) bool {
	if binding.StatusAt(now) != authorization.BindingActive {
		return false
	}
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
