package authorization

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	domain "github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
)

type AdminRepository interface {
	ListAuthorizationRoles(context.Context, identity.WorkspaceID) ([]domain.Role, error)
	GetAuthorizationRole(context.Context, identity.WorkspaceID, string) (domain.Role, error)
	CreateCustomRole(context.Context, CustomRoleMutation) (domain.Role, int64, error)
	UpdateCustomRole(context.Context, CustomRoleMutation) (domain.Role, int64, error)
	ListAuthorizationRoleBindings(context.Context, identity.WorkspaceID) ([]domain.RoleBinding, error)
	CreateManagedRoleBinding(context.Context, RoleBindingMutation) (domain.RoleBinding, int64, error)
	RevokeManagedRoleBinding(context.Context, RoleBindingRevocation) (domain.RoleBinding, int64, error)
	AuthorizationScopeExists(context.Context, identity.WorkspaceID, domain.Resource) (bool, error)
}

type BindingExpiryRepository interface {
	ExpireRoleBindings(context.Context, ExpireRoleBindingsRequest) error
}

type PolicyDenialRepository interface {
	RecordPolicyDenial(context.Context, PolicyDenial) error
}

type PolicyDenial struct {
	WorkspaceID identity.WorkspaceID
	ActorID     identity.PrincipalID
	TraceID     string
	OccurredAt  time.Time
	Reason      string
	Action      domain.Action
	RoleID      string
	PrincipalID identity.PrincipalID
	BindingID   identity.BindingID
}

type InvitationGrantPreparation struct {
	RoleVersion          int64
	AuthorizationVersion int64
}

type MutationAudit struct {
	EventID    identity.EventID
	ActorID    identity.PrincipalID
	TraceID    string
	OccurredAt time.Time
}

type CustomRoleMutation struct {
	WorkspaceID                  identity.WorkspaceID
	ExpectedAuthorizationVersion int64
	RoleID                       string
	ExpectedVersion              int64
	NextVersion                  int64
	Name                         string
	Description                  string
	Actions                      []domain.Action
	VersionEventID               identity.EventID
	Audit                        MutationAudit
}

type RoleBindingMutation struct {
	Binding                      domain.RoleBinding
	ExpectedAuthorizationVersion int64
	Audit                        MutationAudit
}

type RoleBindingRevocation struct {
	WorkspaceID                  identity.WorkspaceID
	BindingID                    identity.BindingID
	ExpectedVersion              int64
	ExpectedAuthorizationVersion int64
	Reason                       string
	Audit                        MutationAudit
}

type ExpireRoleBindingsRequest struct {
	WorkspaceID identity.WorkspaceID
	Now         time.Time
	TraceID     string
	EventID     identity.EventID
}

type AccessRequest struct {
	WorkspaceID  identity.WorkspaceID
	PrincipalRef string
	TraceID      string
}

type CreateCustomRoleRequest struct {
	AccessRequest
	Name        string
	Description string
	Actions     []domain.Action
}

type UpdateCustomRoleRequest struct {
	AccessRequest
	RoleID          string
	ExpectedVersion int64
	Name            string
	Description     string
	Actions         []domain.Action
}

type CreateRoleBindingRequest struct {
	AccessRequest
	PrincipalID         identity.PrincipalID
	RoleID              string
	ExpectedRoleVersion int64
	ScopeType           domain.ScopeType
	ScopeID             string
	ExpiresAt           *time.Time
}

type RevokeRoleBindingRequest struct {
	AccessRequest
	BindingID       identity.BindingID
	ExpectedVersion int64
	Reason          string
}

type InspectRequest struct {
	AccessRequest
	TargetPrincipalRef string
	Action             domain.Action
	Resource           domain.Resource
}

// SessionCapabilities returns active roles, actions, and their current workspace
// version. It is presentation data only; protected commands still call Evaluate.
func (service *Service) SessionCapabilities(
	ctx context.Context,
	workspaceID identity.WorkspaceID,
	principalID identity.PrincipalID,
	traceID string,
) ([]domain.Action, []string, int64, error) {
	if workspaceID.IsZero() || principalID.IsZero() {
		return nil, nil, 0, domain.ErrInvalidArgument
	}
	if expirer, ok := service.repository.(BindingExpiryRepository); ok {
		eventID, err := identity.NewEventID()
		if err != nil {
			return nil, nil, 0, err
		}
		if err := expirer.ExpireRoleBindings(ctx, ExpireRoleBindingsRequest{
			WorkspaceID: workspaceID, Now: service.clock.Now().UTC(), TraceID: traceID, EventID: eventID,
		}); err != nil {
			return nil, nil, 0, err
		}
	}
	principal, err := service.repository.LoadPrincipal(ctx, workspaceID, principalID)
	if err != nil {
		return nil, nil, 0, err
	}
	if principal.Status != domain.PrincipalActive {
		version, versionErr := service.repository.AuthorizationVersion(ctx, workspaceID)
		return []domain.Action{}, []string{}, version, versionErr
	}
	bindings, err := service.repository.LoadPrincipalBindings(ctx, principalID)
	if err != nil {
		return nil, nil, 0, err
	}
	now := service.clock.Now().UTC()
	seen := make(map[domain.Action]struct{})
	seenRoles := make(map[string]struct{})
	for _, binding := range bindings {
		if binding.WorkspaceID != workspaceID || binding.StatusAt(now) != domain.BindingActive {
			continue
		}
		seenRoles[binding.RoleID] = struct{}{}
		for _, action := range binding.Actions {
			seen[action] = struct{}{}
		}
	}
	result := make([]domain.Action, 0, len(seen))
	for action := range seen {
		result = append(result, action)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	roleIDs := make([]string, 0, len(seenRoles))
	for roleID := range seenRoles {
		roleIDs = append(roleIDs, roleID)
	}
	sort.Strings(roleIDs)
	version, err := service.repository.AuthorizationVersion(ctx, workspaceID)
	if err != nil {
		return nil, nil, 0, err
	}
	return result, roleIDs, version, nil
}

func (service *Service) ListRoles(ctx context.Context, request AccessRequest) ([]domain.Role, error) {
	if request.WorkspaceID.IsZero() {
		return nil, domain.ErrInvalidArgument
	}
	if _, err := service.require(ctx, request, domain.ActionRoleRead, workspaceResource(request.WorkspaceID)); err != nil {
		return nil, err
	}
	repository, err := service.adminRepository()
	if err != nil {
		return nil, err
	}
	return repository.ListAuthorizationRoles(ctx, request.WorkspaceID)
}

func (service *Service) GetRole(ctx context.Context, request AccessRequest, roleID string) (domain.Role, error) {
	roleID = strings.TrimSpace(roleID)
	if request.WorkspaceID.IsZero() || roleID == "" {
		return domain.Role{}, domain.ErrInvalidArgument
	}
	if _, err := service.require(ctx, request, domain.ActionRoleRead, workspaceResource(request.WorkspaceID)); err != nil {
		return domain.Role{}, err
	}
	repository, err := service.adminRepository()
	if err != nil {
		return domain.Role{}, err
	}
	return repository.GetAuthorizationRole(ctx, request.WorkspaceID, roleID)
}

func (service *Service) CreateCustomRole(ctx context.Context, request CreateCustomRoleRequest) (domain.Role, int64, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	actions, err := normalizedActions(request.Actions)
	if request.WorkspaceID.IsZero() || request.Name == "" || request.Description == "" || err != nil {
		return domain.Role{}, 0, domain.ErrInvalidArgument
	}
	decision, err := service.require(ctx, request.AccessRequest, domain.ActionRoleManage, workspaceResource(request.WorkspaceID))
	if err != nil {
		return domain.Role{}, 0, err
	}
	if err := service.ensureAuthorizationCeiling(ctx, request.AccessRequest, actions, workspaceResource(request.WorkspaceID)); err != nil {
		return domain.Role{}, 0, service.policyError(ctx, decision, request.AccessRequest, "AUTHORIZATION_CEILING", domain.ActionRoleManage, "", identity.PrincipalID{}, identity.BindingID{}, err)
	}
	versionEventID, audit, roleID, err := service.newCustomRoleIdentity(decision.PrincipalID, request.TraceID)
	if err != nil {
		return domain.Role{}, 0, err
	}
	repository, err := service.adminRepository()
	if err != nil {
		return domain.Role{}, 0, err
	}
	return repository.CreateCustomRole(ctx, CustomRoleMutation{
		WorkspaceID: request.WorkspaceID, RoleID: roleID, NextVersion: 1,
		ExpectedAuthorizationVersion: decision.AuthorizationVersion,
		Name:                         request.Name, Description: request.Description, Actions: actions,
		VersionEventID: versionEventID, Audit: audit,
	})
}

func (service *Service) UpdateCustomRole(ctx context.Context, request UpdateCustomRoleRequest) (domain.Role, int64, error) {
	request.RoleID = strings.TrimSpace(request.RoleID)
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	actions, err := normalizedActions(request.Actions)
	if request.WorkspaceID.IsZero() || request.RoleID == "" || request.ExpectedVersion < 1 ||
		request.Name == "" || request.Description == "" || err != nil {
		return domain.Role{}, 0, domain.ErrInvalidArgument
	}
	decision, err := service.require(ctx, request.AccessRequest, domain.ActionRoleManage, workspaceResource(request.WorkspaceID))
	if err != nil {
		return domain.Role{}, 0, err
	}
	repository, err := service.adminRepository()
	if err != nil {
		return domain.Role{}, 0, err
	}
	current, err := repository.GetAuthorizationRole(ctx, request.WorkspaceID, request.RoleID)
	if err != nil {
		return domain.Role{}, 0, err
	}
	if current.Category == "system" {
		return domain.Role{}, 0, domain.ErrRolesImmutable
	}
	if current.Version != request.ExpectedVersion {
		return domain.Role{}, 0, domain.ErrVersionConflict
	}
	if err := service.ensureAuthorizationCeiling(ctx, request.AccessRequest, actions, workspaceResource(request.WorkspaceID)); err != nil {
		return domain.Role{}, 0, service.policyError(ctx, decision, request.AccessRequest, "AUTHORIZATION_CEILING", domain.ActionRoleManage, request.RoleID, identity.PrincipalID{}, identity.BindingID{}, err)
	}
	versionEventID, audit, _, err := service.newCustomRoleIdentity(decision.PrincipalID, request.TraceID)
	if err != nil {
		return domain.Role{}, 0, err
	}
	updated, version, err := repository.UpdateCustomRole(ctx, CustomRoleMutation{
		WorkspaceID: request.WorkspaceID, RoleID: request.RoleID,
		ExpectedAuthorizationVersion: decision.AuthorizationVersion,
		ExpectedVersion:              request.ExpectedVersion, NextVersion: request.ExpectedVersion + 1,
		Name: request.Name, Description: request.Description, Actions: actions,
		VersionEventID: versionEventID, Audit: audit,
	})
	if errors.Is(err, domain.ErrAuthorizationCeiling) {
		err = service.policyError(ctx, decision, request.AccessRequest, "AFFECTED_BINDING_AUTHORIZATION_CEILING", domain.ActionRoleManage, request.RoleID, identity.PrincipalID{}, identity.BindingID{}, err)
	}
	if errors.Is(err, domain.ErrSeparationOfDuties) {
		err = service.policyError(ctx, decision, request.AccessRequest, "AFFECTED_BINDING_POLICY", domain.ActionRoleManage, request.RoleID, identity.PrincipalID{}, identity.BindingID{}, err)
	}
	return updated, version, err
}

func (service *Service) ListBindings(ctx context.Context, request AccessRequest) ([]domain.RoleBinding, error) {
	if request.WorkspaceID.IsZero() {
		return nil, domain.ErrInvalidArgument
	}
	if _, err := service.require(ctx, request, domain.ActionRoleAssign, workspaceResource(request.WorkspaceID)); err != nil {
		return nil, err
	}
	repository, err := service.adminRepository()
	if err != nil {
		return nil, err
	}
	return repository.ListAuthorizationRoleBindings(ctx, request.WorkspaceID)
}

func (service *Service) CreateBinding(ctx context.Context, request CreateRoleBindingRequest) (domain.RoleBinding, int64, error) {
	if request.WorkspaceID.IsZero() || request.PrincipalID.IsZero() || strings.TrimSpace(request.RoleID) == "" ||
		request.ExpectedRoleVersion < 1 || !request.ScopeType.Valid() || strings.TrimSpace(request.ScopeID) == "" {
		return domain.RoleBinding{}, 0, domain.ErrInvalidArgument
	}
	if request.ScopeType == domain.ScopeWorkspace && request.ScopeID != request.WorkspaceID.UUID() {
		return domain.RoleBinding{}, 0, domain.ErrInvalidArgument
	}
	decision, err := service.require(ctx, request.AccessRequest, domain.ActionRoleAssign, workspaceResource(request.WorkspaceID))
	if err != nil {
		return domain.RoleBinding{}, 0, err
	}
	repository, err := service.adminRepository()
	if err != nil {
		return domain.RoleBinding{}, 0, err
	}
	target, err := service.repository.LoadPrincipal(ctx, request.WorkspaceID, request.PrincipalID)
	if err != nil {
		return domain.RoleBinding{}, 0, err
	}
	if target.Status != domain.PrincipalActive {
		return domain.RoleBinding{}, 0, domain.ErrInvariant
	}
	role, err := repository.GetAuthorizationRole(ctx, request.WorkspaceID, request.RoleID)
	if err != nil {
		return domain.RoleBinding{}, 0, err
	}
	if role.Version != request.ExpectedRoleVersion {
		return domain.RoleBinding{}, 0, domain.ErrVersionConflict
	}
	if role.ID == "workspace_admin" && request.ExpiresAt != nil {
		return domain.RoleBinding{}, 0, service.policyError(ctx, decision, request.AccessRequest, "ADMIN_BINDING_MUST_NOT_EXPIRE", domain.ActionRoleAssign, role.ID, request.PrincipalID, identity.BindingID{}, domain.ErrFinalAdministrator)
	}
	resource := domain.Resource{Type: request.ScopeType, ID: request.ScopeID}
	exists, err := repository.AuthorizationScopeExists(ctx, request.WorkspaceID, resource)
	if err != nil {
		return domain.RoleBinding{}, 0, err
	}
	if !exists {
		return domain.RoleBinding{}, 0, domain.ErrNotFound
	}
	if err := service.ensureAuthorizationCeiling(ctx, request.AccessRequest, role.Actions, resource); err != nil {
		return domain.RoleBinding{}, 0, service.policyError(ctx, decision, request.AccessRequest, "AUTHORIZATION_CEILING", domain.ActionRoleAssign, role.ID, request.PrincipalID, identity.BindingID{}, err)
	}
	if target.Kind != domain.PrincipalHuman {
		for _, action := range role.Actions {
			if domain.RequiresHuman(action) {
				return domain.RoleBinding{}, 0, service.policyError(ctx, decision, request.AccessRequest, "HUMAN_ONLY_ACTION", domain.ActionRoleAssign, role.ID, request.PrincipalID, identity.BindingID{}, domain.ErrSeparationOfDuties)
			}
		}
	}
	now := service.clock.Now().UTC()
	if request.ExpiresAt != nil && !request.ExpiresAt.After(now) {
		return domain.RoleBinding{}, 0, domain.ErrInvalidArgument
	}
	existing, err := repository.ListAuthorizationRoleBindings(ctx, request.WorkspaceID)
	if err != nil {
		return domain.RoleBinding{}, 0, err
	}
	candidate := domain.RoleBinding{
		WorkspaceID: request.WorkspaceID, PrincipalID: request.PrincipalID,
		RoleID: role.ID, RoleVersion: role.Version, ScopeType: request.ScopeType,
		ScopeID: request.ScopeID, GrantedBy: &decision.PrincipalID, GrantedAt: now,
		ExpiresAt: request.ExpiresAt, Version: 1, Actions: append([]domain.Action(nil), role.Actions...),
	}
	for _, binding := range existing {
		if binding.PrincipalID != candidate.PrincipalID || binding.StatusAt(now) != domain.BindingActive ||
			!domain.BindingsOverlap(binding, candidate, request.WorkspaceID.UUID()) {
			continue
		}
		if domain.ActionsConflict(binding.Actions, role.Actions) {
			return domain.RoleBinding{}, 0, service.policyError(ctx, decision, request.AccessRequest, "SEPARATION_OF_DUTIES", domain.ActionRoleAssign, role.ID, request.PrincipalID, identity.BindingID{}, domain.ErrSeparationOfDuties)
		}
	}
	bindingID, err := identity.NewBindingID()
	if err != nil {
		return domain.RoleBinding{}, 0, err
	}
	candidate.ID = bindingID
	audit, err := service.newMutationAudit(decision.PrincipalID, request.TraceID)
	if err != nil {
		return domain.RoleBinding{}, 0, err
	}
	return repository.CreateManagedRoleBinding(ctx, RoleBindingMutation{
		Binding: candidate, ExpectedAuthorizationVersion: decision.AuthorizationVersion, Audit: audit,
	})
}

func (service *Service) RevokeBinding(ctx context.Context, request RevokeRoleBindingRequest) (domain.RoleBinding, int64, error) {
	request.Reason = strings.TrimSpace(request.Reason)
	if request.WorkspaceID.IsZero() || request.BindingID.IsZero() || request.ExpectedVersion < 1 || request.Reason == "" {
		return domain.RoleBinding{}, 0, domain.ErrInvalidArgument
	}
	decision, err := service.require(ctx, request.AccessRequest, domain.ActionRoleAssign, workspaceResource(request.WorkspaceID))
	if err != nil {
		return domain.RoleBinding{}, 0, err
	}
	audit, err := service.newMutationAudit(decision.PrincipalID, request.TraceID)
	if err != nil {
		return domain.RoleBinding{}, 0, err
	}
	repository, err := service.adminRepository()
	if err != nil {
		return domain.RoleBinding{}, 0, err
	}
	revoked, version, err := repository.RevokeManagedRoleBinding(ctx, RoleBindingRevocation{
		WorkspaceID: request.WorkspaceID, BindingID: request.BindingID,
		ExpectedVersion: request.ExpectedVersion, ExpectedAuthorizationVersion: decision.AuthorizationVersion,
		Reason: request.Reason, Audit: audit,
	})
	if errors.Is(err, domain.ErrFinalAdministrator) {
		err = service.policyError(ctx, decision, request.AccessRequest, "FINAL_ACTIVE_ADMIN", domain.ActionRoleAssign, "workspace_admin", identity.PrincipalID{}, request.BindingID, err)
	}
	return revoked, version, err
}

// PrepareInvitationGrant applies the same role-assignment ceiling used by
// direct bindings and freezes the role plus authorization versions that the
// identity transaction must recheck before persisting an invitation.
func (service *Service) PrepareInvitationGrant(ctx context.Context, access AccessRequest, roleID string) (InvitationGrantPreparation, error) {
	roleID = strings.TrimSpace(roleID)
	if access.WorkspaceID.IsZero() || roleID == "" {
		return InvitationGrantPreparation{}, domain.ErrInvalidArgument
	}
	decision, err := service.require(ctx, access, domain.ActionRoleAssign, workspaceResource(access.WorkspaceID))
	if err != nil {
		return InvitationGrantPreparation{}, err
	}
	repository, err := service.adminRepository()
	if err != nil {
		return InvitationGrantPreparation{}, err
	}
	role, err := repository.GetAuthorizationRole(ctx, access.WorkspaceID, roleID)
	if err != nil {
		return InvitationGrantPreparation{}, err
	}
	if err := service.ensureAuthorizationCeiling(ctx, access, role.Actions, workspaceResource(access.WorkspaceID)); err != nil {
		return InvitationGrantPreparation{}, service.policyError(ctx, decision, access, "AUTHORIZATION_CEILING", domain.ActionRoleAssign, role.ID, identity.PrincipalID{}, identity.BindingID{}, err)
	}
	return InvitationGrantPreparation{RoleVersion: role.Version, AuthorizationVersion: decision.AuthorizationVersion}, nil
}

func (service *Service) RecordPolicyDenial(ctx context.Context, denial PolicyDenial) error {
	repository, ok := service.repository.(PolicyDenialRepository)
	if !ok {
		return errors.New("authorization policy audit repository is unavailable")
	}
	if denial.OccurredAt.IsZero() {
		denial.OccurredAt = service.clock.Now().UTC()
	}
	return repository.RecordPolicyDenial(ctx, denial)
}

func (service *Service) policyError(ctx context.Context, decision domain.Decision, access AccessRequest, reason string, action domain.Action, roleID string, principalID identity.PrincipalID, bindingID identity.BindingID, policyErr error) error {
	auditErr := service.RecordPolicyDenial(ctx, PolicyDenial{
		WorkspaceID: access.WorkspaceID, ActorID: decision.PrincipalID, TraceID: access.TraceID,
		OccurredAt: service.clock.Now().UTC(), Reason: reason, Action: action,
		RoleID: roleID, PrincipalID: principalID, BindingID: bindingID,
	})
	if auditErr != nil {
		return errors.Join(policyErr, auditErr)
	}
	return policyErr
}

func (service *Service) Inspect(ctx context.Context, request InspectRequest) (domain.Decision, error) {
	if request.WorkspaceID.IsZero() || strings.TrimSpace(request.TargetPrincipalRef) == "" ||
		!domain.IsKnownAction(request.Action) || !request.Resource.Type.Valid() || request.Resource.ID == "" {
		return domain.Decision{}, domain.ErrInvalidArgument
	}
	if _, err := service.require(ctx, request.AccessRequest, domain.ActionAuthorizationInspect, workspaceResource(request.WorkspaceID)); err != nil {
		return domain.Decision{}, err
	}
	return service.Evaluate(ctx, EvaluationRequest{
		PrincipalRef: request.TargetPrincipalRef, WorkspaceID: request.WorkspaceID,
		Action: request.Action, Resource: request.Resource, TraceID: request.TraceID,
	})
}

func (service *Service) require(ctx context.Context, access AccessRequest, action domain.Action, resource domain.Resource) (domain.Decision, error) {
	decision, err := service.Evaluate(ctx, EvaluationRequest{
		PrincipalRef: access.PrincipalRef, WorkspaceID: access.WorkspaceID,
		Action: action, Resource: resource, TraceID: access.TraceID,
	})
	if err != nil {
		return domain.Decision{}, err
	}
	if !decision.Allowed {
		return domain.Decision{}, &domain.DenialError{Decision: decision}
	}
	return decision, nil
}

func (service *Service) ensureAuthorizationCeiling(ctx context.Context, access AccessRequest, actions []domain.Action, resource domain.Resource) error {
	for _, action := range actions {
		decision, err := service.Evaluate(ctx, EvaluationRequest{
			PrincipalRef: access.PrincipalRef, WorkspaceID: access.WorkspaceID,
			Action: action, Resource: resource, TraceID: access.TraceID,
		})
		if err != nil {
			return err
		}
		if !decision.Allowed {
			return domain.ErrAuthorizationCeiling
		}
	}
	return nil
}

func (service *Service) adminRepository() (AdminRepository, error) {
	repository, ok := service.repository.(AdminRepository)
	if !ok {
		return nil, errors.New("authorization administration repository is unavailable")
	}
	return repository, nil
}

func (service *Service) newCustomRoleIdentity(actor identity.PrincipalID, traceID string) (identity.EventID, MutationAudit, string, error) {
	versionEventID, err := identity.NewEventID()
	if err != nil {
		return identity.EventID{}, MutationAudit{}, "", err
	}
	audit, err := service.newMutationAudit(actor, traceID)
	if err != nil {
		return identity.EventID{}, MutationAudit{}, "", err
	}
	roleID := "custom_" + strings.ReplaceAll(versionEventID.UUID(), "-", "")
	return versionEventID, audit, roleID, nil
}

func (service *Service) newMutationAudit(actor identity.PrincipalID, traceID string) (MutationAudit, error) {
	eventID, err := identity.NewEventID()
	if err != nil {
		return MutationAudit{}, err
	}
	return MutationAudit{EventID: eventID, ActorID: actor, TraceID: traceID, OccurredAt: service.clock.Now().UTC()}, nil
}

func normalizedActions(actions []domain.Action) ([]domain.Action, error) {
	if len(actions) == 0 {
		return nil, domain.ErrInvalidArgument
	}
	seen := make(map[domain.Action]struct{}, len(actions))
	result := make([]domain.Action, 0, len(actions))
	for _, action := range actions {
		if !domain.IsKnownAction(action) {
			return nil, domain.ErrInvalidArgument
		}
		if _, duplicate := seen[action]; duplicate {
			continue
		}
		seen[action] = struct{}{}
		result = append(result, action)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result, nil
}

func workspaceResource(workspace identity.WorkspaceID) domain.Resource {
	return domain.Resource{Type: domain.ScopeWorkspace, ID: workspace.UUID()}
}
