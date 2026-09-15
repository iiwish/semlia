// Package authorization holds the M2 actor-identity and RBAC domain model:
// principals, the FR-003 action vocabulary, scoped role bindings and the
// decision contract shared by every protected command.
package authorization

import (
	"errors"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

var (
	ErrInvalidArgument      = errors.New("invalid authorization argument")
	ErrNotFound             = errors.New("authorization resource not found")
	ErrConflict             = errors.New("authorization state conflict")
	ErrVersionConflict      = errors.New("authorization version conflict")
	ErrInvariant            = errors.New("authorization invariant violated")
	ErrRolesImmutable       = errors.New("system roles cannot be edited or deleted")
	ErrAuthorizationCeiling = errors.New("authorization grant exceeds the grantor ceiling")
	ErrSeparationOfDuties   = errors.New("authorization binding violates separation of duties")
	ErrFinalAdministrator   = errors.New("the final workspace administrator cannot be removed")
)

// Action is a stable FR-003 permission identifier. Role names never authorize;
// actions do.
type Action string

const (
	ActionWorkspaceRead        Action = "workspace.read"
	ActionWorkspaceManage      Action = "workspace.manage"
	ActionMemberRead           Action = "member.read"
	ActionMemberManage         Action = "member.manage"
	ActionGroupManage          Action = "group.manage"
	ActionRoleRead             Action = "role.read"
	ActionRoleManage           Action = "role.manage"
	ActionRoleAssign           Action = "role.assign"
	ActionAuthorizationInspect Action = "authorization.inspect"
	ActionAssetRead            Action = "asset.read"
	ActionAssetPropose         Action = "asset.propose"
	ActionAssetEdit            Action = "asset.edit"
	ActionEvidenceRead         Action = "evidence.read"
	ActionProposalReview       Action = "proposal.review"
	ActionValidationRun        Action = "validation.run"
	ActionReleasePublish       Action = "release.publish"
	ActionReleaseRollback      Action = "release.rollback"
	ActionSourceRead           Action = "source.read"
	ActionSourceManage         Action = "source.manage"
	ActionIngestionRun         Action = "ingestion.run"
	ActionBindingRead          Action = "binding.read"
	ActionBindingManage        Action = "binding.manage"
	ActionSemanticResolve      Action = "semantic.resolve"
	ActionSemanticExecute      Action = "semantic.execute"
	ActionAuditRead            Action = "audit.read"
	ActionRuntimeRead          Action = "runtime.read"
	ActionRuntimeManage        Action = "runtime.manage"
)

type actionDescriptor struct {
	domain        string
	requiresHuman bool
}

// actionVocabulary mirrors the seeded actions table and web/src/types.ts
// PermissionAction union. requires_human marks duties reserved for human
// principals: privilege administration (FR-007) and protected release
// governance (SSOT §8.6: agents cannot self-promote release levels).
var actionVocabulary = map[Action]actionDescriptor{
	ActionWorkspaceRead:        {domain: "workspace"},
	ActionWorkspaceManage:      {domain: "workspace"},
	ActionMemberRead:           {domain: "identity"},
	ActionMemberManage:         {domain: "identity", requiresHuman: true},
	ActionGroupManage:          {domain: "identity", requiresHuman: true},
	ActionRoleRead:             {domain: "access_control"},
	ActionRoleManage:           {domain: "access_control", requiresHuman: true},
	ActionRoleAssign:           {domain: "access_control", requiresHuman: true},
	ActionAuthorizationInspect: {domain: "access_control"},
	ActionAssetRead:            {domain: "knowledge"},
	ActionAssetPropose:         {domain: "knowledge"},
	ActionAssetEdit:            {domain: "knowledge"},
	ActionEvidenceRead:         {domain: "knowledge"},
	ActionProposalReview:       {domain: "governance", requiresHuman: true},
	ActionValidationRun:        {domain: "governance"},
	ActionReleasePublish:       {domain: "governance", requiresHuman: true},
	ActionReleaseRollback:      {domain: "governance", requiresHuman: true},
	ActionSourceRead:           {domain: "sources"},
	ActionSourceManage:         {domain: "sources"},
	ActionIngestionRun:         {domain: "sources"},
	ActionBindingRead:          {domain: "delivery"},
	ActionBindingManage:        {domain: "delivery"},
	ActionSemanticResolve:      {domain: "delivery"},
	ActionSemanticExecute:      {domain: "delivery"},
	ActionAuditRead:            {domain: "operations"},
	ActionRuntimeRead:          {domain: "operations"},
	ActionRuntimeManage:        {domain: "operations"},
}

func IsKnownAction(action Action) bool {
	_, ok := actionVocabulary[action]
	return ok
}

func RequiresHuman(action Action) bool {
	return actionVocabulary[action].requiresHuman
}

// ReasonCode values match the web/src/authorization.tsx contract exactly. The
// backend never invents codes beyond this set.
type ReasonCode string

const (
	ReasonRoleGrant         ReasonCode = "ROLE_GRANT"
	ReasonSessionCapability ReasonCode = "SESSION_CAPABILITY"
	ReasonNoMatchingGrant   ReasonCode = "NO_MATCHING_GRANT"
	ReasonPrincipalInactive ReasonCode = "PRINCIPAL_INACTIVE"
	ReasonSeparationOfDuty  ReasonCode = "SEPARATION_OF_DUTY"
)

func (code ReasonCode) Valid() bool {
	switch code {
	case ReasonRoleGrant, ReasonSessionCapability, ReasonNoMatchingGrant, ReasonPrincipalInactive, ReasonSeparationOfDuty:
		return true
	}
	return false
}

// ScopeType matches the web/src/types.ts AuthorizationScopeType union.
type ScopeType string

const (
	ScopeWorkspace   ScopeType = "workspace"
	ScopeDomain      ScopeType = "domain"
	ScopeAsset       ScopeType = "asset"
	ScopeSource      ScopeType = "source"
	ScopeEnvironment ScopeType = "environment"
	ScopeRelease     ScopeType = "release"
	ScopeConsumer    ScopeType = "consumer"
)

func (scope ScopeType) Valid() bool {
	switch scope {
	case ScopeWorkspace, ScopeDomain, ScopeAsset, ScopeSource, ScopeEnvironment, ScopeRelease, ScopeConsumer:
		return true
	}
	return false
}

type PrincipalKind string

const (
	PrincipalHuman PrincipalKind = "human"
	PrincipalAgent PrincipalKind = "agent"
)

type PrincipalStatus string

const (
	PrincipalActive    PrincipalStatus = "active"
	PrincipalSuspended PrincipalStatus = "suspended"
	PrincipalRevoked   PrincipalStatus = "revoked"
)

type Principal struct {
	ID               identity.PrincipalID
	WorkspaceID      identity.WorkspaceID
	Kind             PrincipalKind
	DisplayName      string
	OwnerPrincipalID *identity.PrincipalID
	Status           PrincipalStatus
	CreatedAt        time.Time
}

func (principal Principal) Validate() error {
	if principal.ID.IsZero() || principal.WorkspaceID.IsZero() || principal.DisplayName == "" {
		return ErrInvalidArgument
	}
	switch principal.Kind {
	case PrincipalAgent:
		if principal.OwnerPrincipalID == nil || principal.OwnerPrincipalID.IsZero() {
			return ErrInvalidArgument
		}
	case PrincipalHuman:
		if principal.OwnerPrincipalID != nil {
			return ErrInvalidArgument
		}
	default:
		return ErrInvalidArgument
	}
	switch principal.Status {
	case PrincipalActive, PrincipalSuspended, PrincipalRevoked:
	default:
		return ErrInvalidArgument
	}
	return nil
}

type Role struct {
	WorkspaceID identity.WorkspaceID
	ID          string
	Name        string
	Description string
	Category    string
	Actions     []Action
	Version     int64
	CreatedAt   time.Time
}

type RoleBinding struct {
	WorkspaceID  identity.WorkspaceID
	ID           identity.BindingID
	PrincipalID  identity.PrincipalID
	RoleID       string
	RoleVersion  int64
	ScopeType    ScopeType
	ScopeID      string
	GrantedBy    *identity.PrincipalID
	GrantedAt    time.Time
	ExpiresAt    *time.Time
	ExpiredAt    *time.Time
	RevokedAt    *time.Time
	RevokedBy    *identity.PrincipalID
	RevokeReason string
	Version      int64
	Actions      []Action
}

type BindingStatus string

const (
	BindingActive  BindingStatus = "active"
	BindingExpired BindingStatus = "expired"
	BindingRevoked BindingStatus = "revoked"
)

func (binding RoleBinding) StatusAt(now time.Time) BindingStatus {
	if binding.RevokedAt != nil {
		return BindingRevoked
	}
	if binding.ExpiredAt != nil || binding.ExpiresAt != nil && !now.Before(*binding.ExpiresAt) {
		return BindingExpired
	}
	return BindingActive
}

func (role Role) Validate() error {
	if role.ID == "" || role.Name == "" || role.Description == "" || len(role.Actions) == 0 {
		return ErrInvalidArgument
	}
	switch role.Category {
	case "system":
		if !role.WorkspaceID.IsZero() || role.Version != 1 {
			return ErrInvalidArgument
		}
	case "custom":
		if role.WorkspaceID.IsZero() || role.Version < 1 {
			return ErrInvalidArgument
		}
	default:
		return ErrInvalidArgument
	}
	seen := make(map[Action]struct{}, len(role.Actions))
	for _, action := range role.Actions {
		if !IsKnownAction(action) {
			return ErrInvalidArgument
		}
		if _, duplicate := seen[action]; duplicate {
			return ErrInvalidArgument
		}
		seen[action] = struct{}{}
	}
	return nil
}

func (binding RoleBinding) Validate(now time.Time) error {
	if binding.WorkspaceID.IsZero() || binding.ID.IsZero() || binding.PrincipalID.IsZero() ||
		binding.RoleID == "" || binding.RoleVersion < 1 || !binding.ScopeType.Valid() ||
		binding.ScopeID == "" || binding.GrantedAt.IsZero() || binding.Version < 1 {
		return ErrInvalidArgument
	}
	if binding.ScopeType == ScopeWorkspace && binding.ScopeID != binding.WorkspaceID.UUID() {
		return ErrInvalidArgument
	}
	if binding.ExpiresAt != nil && !binding.ExpiresAt.After(binding.GrantedAt) {
		return ErrInvalidArgument
	}
	if binding.ExpiredAt != nil && binding.ExpiresAt == nil {
		return ErrInvalidArgument
	}
	if binding.RevokedAt != nil && binding.RevokedAt.Before(binding.GrantedAt) {
		return ErrInvalidArgument
	}
	if binding.StatusAt(now) == BindingRevoked && binding.RevokedBy == nil {
		return ErrInvalidArgument
	}
	return nil
}

// ActionsConflict expresses the protected review/publish separation without
// depending on role display names, so custom roles cannot bypass the rule.
func ActionsConflict(left, right []Action) bool {
	leftReviews, leftPublishes := actionDuties(left)
	rightReviews, rightPublishes := actionDuties(right)
	return leftReviews && rightPublishes || leftPublishes && rightReviews
}

func actionDuties(actions []Action) (reviews, publishes bool) {
	for _, action := range actions {
		switch action {
		case ActionProposalReview:
			reviews = true
		case ActionReleasePublish, ActionReleaseRollback:
			publishes = true
		}
	}
	return reviews, publishes
}

// BindingsOverlap is intentionally conservative. Workspace grants overlap
// every narrower resource in that workspace; otherwise exact scopes overlap.
func BindingsOverlap(left, right RoleBinding, workspaceID string) bool {
	if left.ScopeType == ScopeWorkspace && left.ScopeID == workspaceID ||
		right.ScopeType == ScopeWorkspace && right.ScopeID == workspaceID {
		return true
	}
	return left.ScopeType == right.ScopeType && left.ScopeID == right.ScopeID
}

// Resource is the target a command acts on, expressed in scope coordinates.
type Resource struct {
	Type     ScopeType
	ID       string
	DomainID string
}

type Decision struct {
	Allowed              bool
	Action               Action
	PrincipalID          identity.PrincipalID
	ReasonCode           ReasonCode
	AuthorizationVersion int64
	RoleID               string
	BindingID            identity.BindingID
}

// DenialError carries the audited decision to the HTTP boundary so protected
// commands answer with 403 plus the stable frontend reason code (FR-011,
// FR-012: a backend denial overrides any rendered UI state).
type DenialError struct {
	Decision Decision
}

func (e *DenialError) Error() string {
	return "authorization denied: " + string(e.Decision.ReasonCode)
}

// DecisionEvent is the immutable audit fact recorded for every decision.
type DecisionEvent struct {
	ID                   identity.EventID
	WorkspaceID          identity.WorkspaceID
	PrincipalID          *identity.PrincipalID
	Actor                string
	Action               Action
	ResourceType         ScopeType
	ResourceID           string
	Decision             string
	ReasonCode           ReasonCode
	AuthorizationVersion int64
	TraceID              string
	CreatedAt            time.Time
}

// SystemRoleIDs lists the nine FR-004 system roles in binding order.
var SystemRoleIDs = []string{
	"workspace_admin",
	"security_admin",
	"semantic_steward",
	"asset_owner",
	"reviewer",
	"publisher",
	"source_operator",
	"consumer_developer",
	"auditor",
}
