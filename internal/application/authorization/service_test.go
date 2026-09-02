package authorization_test

import (
	"context"
	"errors"
	"testing"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
)

const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

type fakeRepository struct {
	principals       map[string]authorization.Principal
	defaultPrincipal map[string]authorization.Principal
	bindings         map[string][]authorization.RoleBinding
	version          int64
	versionMissing   bool
	events           []authorization.DecisionEvent
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		principals:       map[string]authorization.Principal{},
		defaultPrincipal: map[string]authorization.Principal{},
		bindings:         map[string][]authorization.RoleBinding{},
		version:          1,
	}
}

func (repository *fakeRepository) key(workspace identity.WorkspaceID, principal identity.PrincipalID) string {
	return workspace.UUID() + "/" + principal.UUID()
}

func (repository *fakeRepository) LoadPrincipal(
	_ context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID,
) (authorization.Principal, error) {
	value, ok := repository.principals[repository.key(workspace, principal)]
	if !ok {
		return authorization.Principal{}, authorization.ErrNotFound
	}
	return value, nil
}

func (repository *fakeRepository) LoadDefaultPrincipal(
	_ context.Context, workspace identity.WorkspaceID,
) (authorization.Principal, error) {
	value, ok := repository.defaultPrincipal[workspace.UUID()]
	if !ok {
		return authorization.Principal{}, authorization.ErrNotFound
	}
	return value, nil
}

func (repository *fakeRepository) LoadPrincipalBindings(
	_ context.Context, principal identity.PrincipalID,
) ([]authorization.RoleBinding, error) {
	return repository.bindings[principal.UUID()], nil
}

func (repository *fakeRepository) AuthorizationVersion(context.Context, identity.WorkspaceID) (int64, error) {
	if repository.versionMissing {
		return 0, authorization.ErrNotFound
	}
	return repository.version, nil
}

func (repository *fakeRepository) RecordDecision(_ context.Context, event authorization.DecisionEvent) error {
	repository.events = append(repository.events, event)
	return nil
}

func evaluationRequest(
	workspace identity.WorkspaceID,
	principalRef string,
	action authorization.Action,
	resource authorization.Resource,
) authorizationapp.EvaluationRequest {
	return authorizationapp.EvaluationRequest{
		PrincipalRef: principalRef,
		WorkspaceID:  workspace,
		Action:       action,
		Resource:     resource,
		TraceID:      traceID,
	}
}

func workspaceResource(workspace identity.WorkspaceID) authorization.Resource {
	return authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}
}

func mustID[T any](t *testing.T, create func() (T, error)) T {
	t.Helper()
	value, err := create()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func humanPrincipal(t *testing.T, workspace identity.WorkspaceID) (identity.PrincipalID, authorization.Principal) {
	t.Helper()
	id := mustID(t, identity.NewPrincipalID)
	return id, authorization.Principal{
		ID: id, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Founder", Status: authorization.PrincipalActive,
		CreatedAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
	}
}

func agentPrincipal(t *testing.T, workspace identity.WorkspaceID, owner identity.PrincipalID) authorization.Principal {
	t.Helper()
	return authorization.Principal{
		ID: mustID(t, identity.NewPrincipalID), WorkspaceID: workspace, Kind: authorization.PrincipalAgent,
		DisplayName: "Governance Agent", OwnerPrincipalID: &owner,
		Status:    authorization.PrincipalActive,
		CreatedAt: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
	}
}

func adminBinding(t *testing.T, principal identity.PrincipalID, workspace identity.WorkspaceID) authorization.RoleBinding {
	t.Helper()
	return authorization.RoleBinding{
		ID: mustID(t, identity.NewBindingID), PrincipalID: principal, RoleID: "workspace_admin",
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
		Actions: []authorization.Action{authorization.ActionAssetPropose, authorization.ActionAssetEdit},
	}
}

func TestDenyByDefaultRecordsEventWithStableReasonCode(t *testing.T) {
	repository := newFakeRepository()
	service := authorizationapp.NewService(repository, authorizationapp.ClockFunc(time.Now))
	workspace := mustID(t, identity.NewWorkspaceID)
	unknown := mustID(t, identity.NewPrincipalID)

	decision, err := service.Evaluate(context.Background(), evaluationRequest(
		workspace, unknown.String(), authorization.ActionAssetPropose, workspaceResource(workspace),
	))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatal("unknown principal was allowed")
	}
	if decision.ReasonCode != authorization.ReasonNoMatchingGrant {
		t.Fatalf("reason code = %q, want NO_MATCHING_GRANT", decision.ReasonCode)
	}
	if decision.AuthorizationVersion != 1 {
		t.Fatalf("authorization version = %d, want 1", decision.AuthorizationVersion)
	}
	if len(repository.events) != 1 {
		t.Fatalf("recorded events = %d, want exactly one denial event", len(repository.events))
	}
	event := repository.events[0]
	if event.Decision != "deny" || event.ReasonCode != authorization.ReasonNoMatchingGrant ||
		event.Action != authorization.ActionAssetPropose || event.WorkspaceID != workspace ||
		event.PrincipalID != nil || event.AuthorizationVersion != 1 || event.TraceID != traceID {
		t.Fatalf("denial event = %+v", event)
	}
}

func TestCrossWorkspaceBindingNeverGrantsAccess(t *testing.T) {
	repository := newFakeRepository()
	service := authorizationapp.NewService(repository, authorizationapp.ClockFunc(time.Now))
	homeWorkspace := mustID(t, identity.NewWorkspaceID)
	otherWorkspace := mustID(t, identity.NewWorkspaceID)
	principalID, principal := humanPrincipal(t, homeWorkspace)
	repository.principals[repository.key(homeWorkspace, principalID)] = principal
	repository.bindings[principalID.UUID()] = []authorization.RoleBinding{adminBinding(t, principalID, homeWorkspace)}

	decision, err := service.Evaluate(context.Background(), evaluationRequest(
		otherWorkspace, principalID.String(), authorization.ActionAssetPropose, workspaceResource(otherWorkspace),
	))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatal("binding from another workspace granted access")
	}
	if decision.ReasonCode != authorization.ReasonNoMatchingGrant {
		t.Fatalf("reason code = %q, want NO_MATCHING_GRANT", decision.ReasonCode)
	}
}

func TestAgentPrincipalScopedCapabilityAllowsAndHumanOnlyActionRejects(t *testing.T) {
	repository := newFakeRepository()
	service := authorizationapp.NewService(repository, authorizationapp.ClockFunc(time.Now))
	workspace := mustID(t, identity.NewWorkspaceID)
	ownerID, owner := humanPrincipal(t, workspace)
	repository.principals[repository.key(workspace, ownerID)] = owner
	agent := agentPrincipal(t, workspace, ownerID)
	repository.principals[repository.key(workspace, agent.ID)] = agent
	repository.bindings[agent.ID.UUID()] = []authorization.RoleBinding{{
		ID: mustID(t, identity.NewBindingID), PrincipalID: agent.ID, RoleID: "semantic_steward",
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
		Actions: []authorization.Action{
			authorization.ActionAssetPropose, authorization.ActionReleasePublish,
		},
	}}

	allowed, err := service.Evaluate(context.Background(), evaluationRequest(
		workspace, agent.ID.String(), authorization.ActionAssetPropose, workspaceResource(workspace),
	))
	if err != nil {
		t.Fatal(err)
	}
	if !allowed.Allowed || allowed.ReasonCode != authorization.ReasonRoleGrant {
		t.Fatalf("agent scoped capability decision = %+v", allowed)
	}

	denied, err := service.Evaluate(context.Background(), evaluationRequest(
		workspace, agent.ID.String(), authorization.ActionReleasePublish, workspaceResource(workspace),
	))
	if err != nil {
		t.Fatal(err)
	}
	if denied.Allowed {
		t.Fatal("agent was allowed a human-only action")
	}
	if denied.ReasonCode != authorization.ReasonSeparationOfDuty {
		t.Fatalf("reason code = %q, want SEPARATION_OF_DUTY", denied.ReasonCode)
	}
}

func TestInactivePrincipalIsDenied(t *testing.T) {
	repository := newFakeRepository()
	service := authorizationapp.NewService(repository, authorizationapp.ClockFunc(time.Now))
	workspace := mustID(t, identity.NewWorkspaceID)
	principalID, principal := humanPrincipal(t, workspace)
	principal.Status = authorization.PrincipalSuspended
	repository.principals[repository.key(workspace, principalID)] = principal
	repository.bindings[principalID.UUID()] = []authorization.RoleBinding{adminBinding(t, principalID, workspace)}

	decision, err := service.Evaluate(context.Background(), evaluationRequest(
		workspace, principalID.String(), authorization.ActionAssetPropose, workspaceResource(workspace),
	))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.ReasonCode != authorization.ReasonPrincipalInactive {
		t.Fatalf("inactive principal decision = %+v", decision)
	}
}

func TestAllowUsesRoleGrantWorkspaceVersionAndScopedAssetResource(t *testing.T) {
	repository := newFakeRepository()
	service := authorizationapp.NewService(repository, authorizationapp.ClockFunc(time.Now))
	workspace := mustID(t, identity.NewWorkspaceID)
	asset := mustID(t, identity.NewAssetID)
	principalID, principal := humanPrincipal(t, workspace)
	repository.principals[repository.key(workspace, principalID)] = principal
	binding := adminBinding(t, principalID, workspace)
	binding.Actions = append(binding.Actions, authorization.ActionAssetEdit)
	repository.bindings[principalID.UUID()] = []authorization.RoleBinding{binding}
	repository.version = 7

	decision, err := service.Evaluate(context.Background(), evaluationRequest(
		workspace, principalID.String(), authorization.ActionAssetEdit,
		authorization.Resource{Type: authorization.ScopeAsset, ID: asset.UUID()},
	))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.ReasonCode != authorization.ReasonRoleGrant ||
		decision.AuthorizationVersion != 7 || decision.RoleID != "workspace_admin" ||
		decision.BindingID != binding.ID {
		t.Fatalf("allow decision = %+v", decision)
	}
	if len(repository.events) != 1 || repository.events[0].Decision != "allow" {
		t.Fatalf("allow decision was not audited: %+v", repository.events)
	}
}

func TestMissingHeaderFallsBackToSeededWorkspaceAdmin(t *testing.T) {
	repository := newFakeRepository()
	service := authorizationapp.NewService(repository, authorizationapp.ClockFunc(time.Now))
	workspace := mustID(t, identity.NewWorkspaceID)
	defaultID, defaultPrincipal := humanPrincipal(t, workspace)
	defaultPrincipal.DisplayName = "Workspace Admin"
	repository.defaultPrincipal[workspace.UUID()] = defaultPrincipal
	repository.bindings[defaultID.UUID()] = []authorization.RoleBinding{adminBinding(t, defaultID, workspace)}

	decision, err := service.Evaluate(context.Background(), evaluationRequest(
		workspace, "", authorization.ActionAssetPropose, workspaceResource(workspace),
	))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.PrincipalID != defaultID {
		t.Fatalf("default principal decision = %+v", decision)
	}
}

func TestUnknownActionIsDenied(t *testing.T) {
	repository := newFakeRepository()
	service := authorizationapp.NewService(repository, authorizationapp.ClockFunc(time.Now))
	workspace := mustID(t, identity.NewWorkspaceID)

	decision, err := service.Evaluate(context.Background(), evaluationRequest(
		workspace, "", authorization.Action("catalog.destroy"), workspaceResource(workspace),
	))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.ReasonCode != authorization.ReasonNoMatchingGrant {
		t.Fatalf("unknown action decision = %+v", decision)
	}
}

func TestDenialErrorCarriesStableReasonCode(t *testing.T) {
	workspace := mustID(t, identity.NewWorkspaceID)
	denial := &authorization.DenialError{Decision: authorization.Decision{
		Allowed: false, Action: authorization.ActionAssetPropose,
		ReasonCode: authorization.ReasonNoMatchingGrant, AuthorizationVersion: 3,
	}}
	if !errors.Is(denial, denial) {
		t.Fatal("denial error must be usable with errors.Is")
	}
	var target *authorization.DenialError
	if !errors.As(denial, &target) || target.Decision.ReasonCode != authorization.ReasonNoMatchingGrant {
		t.Fatalf("errors.As failed: %+v", target)
	}
	if denial.Error() == "" {
		t.Fatal("denial error message is empty")
	}
	_ = workspace
}

func TestPrincipalValidationRequiresOwnerForAgents(t *testing.T) {
	workspace := mustID(t, identity.NewWorkspaceID)
	agent := authorization.Principal{
		ID: mustID(t, identity.NewPrincipalID), WorkspaceID: workspace, Kind: authorization.PrincipalAgent,
		DisplayName: "Agent", Status: authorization.PrincipalActive,
	}
	if err := agent.Validate(); !errors.Is(err, authorization.ErrInvalidArgument) {
		t.Fatalf("agent without owner validated: %v", err)
	}
	owner := mustID(t, identity.NewPrincipalID)
	agent.OwnerPrincipalID = &owner
	if err := agent.Validate(); err != nil {
		t.Fatalf("agent with owner failed validation: %v", err)
	}
	human := authorization.Principal{
		ID: mustID(t, identity.NewPrincipalID), WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Human", Status: authorization.PrincipalActive,
	}
	if err := human.Validate(); err != nil {
		t.Fatalf("human without owner failed validation: %v", err)
	}
}
