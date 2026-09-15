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
	principals          map[string]authorization.Principal
	defaultPrincipal    map[string]authorization.Principal
	localPrincipals     map[string]authorization.Principal
	bindings            map[string][]authorization.RoleBinding
	version             int64
	versionMissing      bool
	events              []authorization.DecisionEvent
	policyDenials       []authorizationapp.PolicyDenial
	roles               map[string]authorization.Role
	lastRoleMutation    authorizationapp.CustomRoleMutation
	bumpAfterAction     authorization.Action
	bindingLoadCalls    int
	mutateOnBindingLoad func(*fakeRepository, identity.PrincipalID)
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		principals:       map[string]authorization.Principal{},
		defaultPrincipal: map[string]authorization.Principal{},
		localPrincipals:  map[string]authorization.Principal{},
		bindings:         map[string][]authorization.RoleBinding{},
		roles:            map[string]authorization.Role{},
		version:          1,
	}
}

func (repository *fakeRepository) LoadLocalUATPrincipal(
	_ context.Context, workspace identity.WorkspaceID, seedNamespace, roleID string,
) (authorization.Principal, error) {
	value, ok := repository.localPrincipals[workspace.UUID()+"/"+seedNamespace+"/"+roleID]
	if !ok {
		return authorization.Principal{}, authorization.ErrNotFound
	}
	return value, nil
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
	repository.bindingLoadCalls++
	result := append([]authorization.RoleBinding(nil), repository.bindings[principal.UUID()]...)
	if repository.mutateOnBindingLoad != nil {
		repository.mutateOnBindingLoad(repository, principal)
	}
	return result, nil
}

func (repository *fakeRepository) AuthorizationVersion(context.Context, identity.WorkspaceID) (int64, error) {
	if repository.versionMissing {
		return 0, authorization.ErrNotFound
	}
	return repository.version, nil
}

func (repository *fakeRepository) RecordDecision(_ context.Context, event authorization.DecisionEvent) error {
	repository.events = append(repository.events, event)
	if event.Action == repository.bumpAfterAction && event.Decision == "allow" {
		repository.version++
	}
	return nil
}

func (repository *fakeRepository) ListAuthorizationRoles(context.Context, identity.WorkspaceID) ([]authorization.Role, error) {
	result := make([]authorization.Role, 0, len(repository.roles))
	for _, role := range repository.roles {
		result = append(result, role)
	}
	return result, nil
}

func (repository *fakeRepository) GetAuthorizationRole(_ context.Context, _ identity.WorkspaceID, roleID string) (authorization.Role, error) {
	role, ok := repository.roles[roleID]
	if !ok {
		return authorization.Role{}, authorization.ErrNotFound
	}
	return role, nil
}

func (repository *fakeRepository) CreateCustomRole(_ context.Context, mutation authorizationapp.CustomRoleMutation) (authorization.Role, int64, error) {
	repository.lastRoleMutation = mutation
	if mutation.ExpectedAuthorizationVersion != repository.version {
		return authorization.Role{}, 0, authorization.ErrVersionConflict
	}
	return authorization.Role{WorkspaceID: mutation.WorkspaceID, ID: mutation.RoleID, Name: mutation.Name, Description: mutation.Description, Category: "custom", Actions: mutation.Actions, Version: 1}, repository.version + 1, nil
}

func (repository *fakeRepository) UpdateCustomRole(context.Context, authorizationapp.CustomRoleMutation) (authorization.Role, int64, error) {
	return authorization.Role{}, 0, authorization.ErrNotFound
}

func (repository *fakeRepository) ListAuthorizationRoleBindings(context.Context, identity.WorkspaceID) ([]authorization.RoleBinding, error) {
	return nil, nil
}

func (repository *fakeRepository) CreateManagedRoleBinding(context.Context, authorizationapp.RoleBindingMutation) (authorization.RoleBinding, int64, error) {
	return authorization.RoleBinding{}, 0, authorization.ErrNotFound
}

func (repository *fakeRepository) RevokeManagedRoleBinding(context.Context, authorizationapp.RoleBindingRevocation) (authorization.RoleBinding, int64, error) {
	return authorization.RoleBinding{}, 0, authorization.ErrNotFound
}

func (repository *fakeRepository) AuthorizationScopeExists(context.Context, identity.WorkspaceID, authorization.Resource) (bool, error) {
	return true, nil
}

func (repository *fakeRepository) RecordPolicyDenial(_ context.Context, denial authorizationapp.PolicyDenial) error {
	repository.policyDenials = append(repository.policyDenials, denial)
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

func TestRoleCreationUsesTheAuthorizationDecisionVersionForPersistence(t *testing.T) {
	repository := newFakeRepository()
	workspace := mustID(t, identity.NewWorkspaceID)
	principalID, principal := humanPrincipal(t, workspace)
	repository.principals[repository.key(workspace, principalID)] = principal
	repository.bindings[principalID.UUID()] = []authorization.RoleBinding{{
		ID: mustID(t, identity.NewBindingID), WorkspaceID: workspace, PrincipalID: principalID,
		RoleID: "workspace_admin", ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
		Actions: []authorization.Action{authorization.ActionRoleManage, authorization.ActionAssetRead},
	}}
	repository.bumpAfterAction = authorization.ActionAssetRead
	service := authorizationapp.NewService(repository, authorizationapp.ClockFunc(time.Now))
	_, _, err := service.CreateCustomRole(context.Background(), authorizationapp.CreateCustomRoleRequest{
		AccessRequest: authorizationapp.AccessRequest{WorkspaceID: workspace, PrincipalRef: principalID.String(), TraceID: traceID},
		Name:          "Race proof", Description: "Rejects an intervening authorization mutation.",
		Actions: []authorization.Action{authorization.ActionAssetRead},
	})
	if !errors.Is(err, authorization.ErrVersionConflict) {
		t.Fatalf("intervening authorization mutation error = %v", err)
	}
	if repository.lastRoleMutation.ExpectedAuthorizationVersion != 1 || repository.version != 2 {
		t.Fatalf("mutation expected version = %d, current = %d", repository.lastRoleMutation.ExpectedAuthorizationVersion, repository.version)
	}
}

func TestMissingHeaderIsDeniedEvenWhenLocalUATIdentitiesAreEnabled(t *testing.T) {
	repository := newFakeRepository()
	workspace := mustID(t, identity.NewWorkspaceID)
	defaultID, defaultPrincipal := humanPrincipal(t, workspace)
	defaultPrincipal.DisplayName = "Workspace Admin"
	repository.defaultPrincipal[workspace.UUID()] = defaultPrincipal
	repository.bindings[defaultID.UUID()] = []authorization.RoleBinding{adminBinding(t, defaultID, workspace)}

	for _, service := range []*authorizationapp.Service{
		authorizationapp.NewService(repository, authorizationapp.ClockFunc(time.Now)),
		authorizationapp.NewService(
			repository, authorizationapp.ClockFunc(time.Now), authorizationapp.WithLocalUATIdentities(),
		),
	} {
		decision, err := service.Evaluate(context.Background(), evaluationRequest(
			workspace, "", authorization.ActionAssetPropose, workspaceResource(workspace),
		))
		if err != nil {
			t.Fatal(err)
		}
		if decision.Allowed || decision.ReasonCode != authorization.ReasonNoMatchingGrant {
			t.Fatalf("anonymous decision = %+v", decision)
		}
	}
}

func TestLocalUATIdentityAliasesResolveSeededPrincipalsOnlyWhenEnabled(t *testing.T) {
	repository := newFakeRepository()
	workspace := mustID(t, identity.NewWorkspaceID)
	adminID, admin := humanPrincipal(t, workspace)
	reviewerID, reviewer := humanPrincipal(t, workspace)
	publisherID, publisher := humanPrincipal(t, workspace)
	reviewer.DisplayName = "Independent Reviewer"
	publisher.DisplayName = "Independent Publisher"
	repository.defaultPrincipal[workspace.UUID()] = admin
	repository.localPrincipals[workspace.UUID()+"/principal/workspace_admin"] = admin
	repository.localPrincipals[workspace.UUID()+"/independent_reviewer/reviewer"] = reviewer
	repository.localPrincipals[workspace.UUID()+"/independent_publisher/publisher"] = publisher
	repository.bindings[adminID.UUID()] = []authorization.RoleBinding{adminBinding(t, adminID, workspace)}
	repository.bindings[reviewerID.UUID()] = []authorization.RoleBinding{{
		ID: mustID(t, identity.NewBindingID), PrincipalID: reviewerID, RoleID: "reviewer",
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
		Actions: []authorization.Action{authorization.ActionProposalReview},
	}}
	repository.bindings[publisherID.UUID()] = []authorization.RoleBinding{{
		ID: mustID(t, identity.NewBindingID), PrincipalID: publisherID, RoleID: "publisher",
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
		Actions: []authorization.Action{authorization.ActionReleasePublish},
	}}

	disabled := authorizationapp.NewService(repository, authorizationapp.ClockFunc(time.Now))
	denied, err := disabled.Evaluate(context.Background(), evaluationRequest(
		workspace, authorizationapp.LocalUATReviewerPrincipalRef,
		authorization.ActionProposalReview, workspaceResource(workspace),
	))
	if err != nil {
		t.Fatal(err)
	}
	if denied.Allowed {
		t.Fatalf("disabled local identity was allowed: %+v", denied)
	}

	enabled := authorizationapp.NewService(
		repository, authorizationapp.ClockFunc(time.Now), authorizationapp.WithLocalUATIdentities(),
	)
	for _, test := range []struct {
		name, reference string
		action          authorization.Action
		principal       identity.PrincipalID
	}{
		{name: "author", reference: authorizationapp.LocalUATAuthorPrincipalRef, action: authorization.ActionAssetPropose, principal: adminID},
		{name: "reviewer", reference: authorizationapp.LocalUATReviewerPrincipalRef, action: authorization.ActionProposalReview, principal: reviewerID},
		{name: "publisher", reference: authorizationapp.LocalUATPublisherPrincipalRef, action: authorization.ActionReleasePublish, principal: publisherID},
	} {
		t.Run(test.name, func(t *testing.T) {
			decision, evaluateErr := enabled.Evaluate(context.Background(), evaluationRequest(
				workspace, test.reference, test.action, workspaceResource(workspace),
			))
			if evaluateErr != nil {
				t.Fatal(evaluateErr)
			}
			if !decision.Allowed || decision.PrincipalID != test.principal {
				t.Fatalf("local identity decision = %+v", decision)
			}
		})
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

func TestSessionCapabilitiesOnlyProjectsActiveWorkspaceBindings(t *testing.T) {
	repository := newFakeRepository()
	workspace := mustID(t, identity.NewWorkspaceID)
	otherWorkspace := mustID(t, identity.NewWorkspaceID)
	principalID, principal := humanPrincipal(t, workspace)
	repository.principals[repository.key(workspace, principalID)] = principal
	now := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)
	revokedAt := now.Add(-time.Hour)
	repository.bindings[principalID.UUID()] = []authorization.RoleBinding{
		{WorkspaceID: workspace, RoleID: "reader", Actions: []authorization.Action{authorization.ActionAssetRead, authorization.ActionRoleRead}},
		{WorkspaceID: workspace, RoleID: "manager", ExpiresAt: &expiredAt, Actions: []authorization.Action{authorization.ActionRoleManage}},
		{WorkspaceID: workspace, RoleID: "assigner", RevokedAt: &revokedAt, Actions: []authorization.Action{authorization.ActionRoleAssign}},
		{WorkspaceID: otherWorkspace, RoleID: "other", Actions: []authorization.Action{authorization.ActionWorkspaceManage}},
	}
	service := authorizationapp.NewService(repository, authorizationapp.ClockFunc(func() time.Time { return now }))

	actions, roleIDs, version, err := service.SessionCapabilities(context.Background(), workspace, principalID, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 || actions[0] != authorization.ActionAssetRead || actions[1] != authorization.ActionRoleRead {
		t.Fatalf("capabilities = %v, want active workspace actions only", actions)
	}
	if version != 1 {
		t.Fatalf("authorization version = %d, want 1", version)
	}
	if len(roleIDs) != 1 || roleIDs[0] != "reader" {
		t.Fatalf("role IDs = %v", roleIDs)
	}
	if len(repository.events) != 0 {
		t.Fatalf("session projection recorded authorization decisions: %+v", repository.events)
	}
}

func TestSnapshotUsesFrozenActiveBindingsWithoutAuditing(t *testing.T) {
	repository := newFakeRepository()
	workspace := mustID(t, identity.NewWorkspaceID)
	otherWorkspace := mustID(t, identity.NewWorkspaceID)
	principalID, principal := humanPrincipal(t, workspace)
	repository.principals[repository.key(workspace, principalID)] = principal
	now := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Second)
	repository.bindings[principalID.UUID()] = []authorization.RoleBinding{
		{WorkspaceID: workspace, ID: mustID(t, identity.NewBindingID), PrincipalID: principalID,
			RoleID: "reviewer", RoleVersion: 3, ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
			GrantedAt: now.Add(-time.Hour), Version: 1, Actions: []authorization.Action{authorization.ActionAssetRead}},
		{WorkspaceID: workspace, ID: mustID(t, identity.NewBindingID), PrincipalID: principalID,
			RoleID: "expired", RoleVersion: 1, ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
			GrantedAt: now.Add(-time.Hour), ExpiresAt: &expired, Version: 1,
			Actions: []authorization.Action{authorization.ActionWorkspaceManage}},
		{WorkspaceID: otherWorkspace, ID: mustID(t, identity.NewBindingID), PrincipalID: principalID,
			RoleID: "foreign", RoleVersion: 1, ScopeType: authorization.ScopeWorkspace, ScopeID: otherWorkspace.UUID(),
			GrantedAt: now.Add(-time.Hour), Version: 1, Actions: []authorization.Action{authorization.ActionWorkspaceManage}},
	}
	repository.version = 9
	service := authorizationapp.NewService(repository, authorizationapp.ClockFunc(func() time.Time { return now }))

	snapshot, err := service.Snapshot(context.Background(), workspace, principalID)
	if err != nil {
		t.Fatal(err)
	}
	resource := authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}
	if !snapshot.Allows(authorization.ActionAssetRead, resource) || snapshot.Allows(authorization.ActionWorkspaceManage, resource) {
		t.Fatalf("snapshot grants are not frozen/active/workspace scoped: %+v", snapshot)
	}
	if got := snapshot.ActiveRoleIDs(); len(got) != 1 || got[0] != "reviewer" {
		t.Fatalf("active role IDs = %v", got)
	}
	if snapshot.AuthorizationVersion != 9 || len(repository.events) != 0 {
		t.Fatalf("snapshot version/events = %d/%d", snapshot.AuthorizationVersion, len(repository.events))
	}
}

func TestSnapshotRetriesConcurrentBindingRevocation(t *testing.T) {
	repository := newFakeRepository()
	workspace := mustID(t, identity.NewWorkspaceID)
	principalID, principal := humanPrincipal(t, workspace)
	repository.principals[repository.key(workspace, principalID)] = principal
	now := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	repository.bindings[principalID.UUID()] = []authorization.RoleBinding{{
		WorkspaceID: workspace, ID: mustID(t, identity.NewBindingID), PrincipalID: principalID,
		RoleID: "reader", RoleVersion: 1, ScopeType: authorization.ScopeWorkspace,
		ScopeID: workspace.UUID(), GrantedAt: now.Add(-time.Hour), Version: 1,
		Actions: []authorization.Action{authorization.ActionAssetRead},
	}}
	repository.mutateOnBindingLoad = func(repo *fakeRepository, principal identity.PrincipalID) {
		repo.mutateOnBindingLoad = nil
		repo.bindings[principal.UUID()] = nil
		repo.version++
	}
	service := authorizationapp.NewService(repository, authorizationapp.ClockFunc(func() time.Time { return now }))
	snapshot, err := service.Snapshot(context.Background(), workspace, principalID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Allows(authorization.ActionAssetRead, authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}) ||
		snapshot.AuthorizationVersion != 2 || repository.bindingLoadCalls != 2 {
		t.Fatalf("stale revoked grant escaped snapshot: %+v calls=%d", snapshot, repository.bindingLoadCalls)
	}
}

func TestSnapshotRetriesConcurrentRoleVersionAdvance(t *testing.T) {
	repository := newFakeRepository()
	workspace := mustID(t, identity.NewWorkspaceID)
	principalID, principal := humanPrincipal(t, workspace)
	repository.principals[repository.key(workspace, principalID)] = principal
	now := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	repository.bindings[principalID.UUID()] = []authorization.RoleBinding{{
		WorkspaceID: workspace, ID: mustID(t, identity.NewBindingID), PrincipalID: principalID,
		RoleID: "custom", RoleVersion: 1, ScopeType: authorization.ScopeWorkspace,
		ScopeID: workspace.UUID(), GrantedAt: now.Add(-time.Hour), Version: 1,
		Actions: []authorization.Action{authorization.ActionAssetRead},
	}}
	repository.mutateOnBindingLoad = func(repo *fakeRepository, principal identity.PrincipalID) {
		repo.mutateOnBindingLoad = nil
		updated := append([]authorization.RoleBinding(nil), repo.bindings[principal.UUID()]...)
		updated[0].RoleVersion = 2
		updated[0].Actions = []authorization.Action{authorization.ActionProposalReview}
		repo.bindings[principal.UUID()] = updated
		repo.version++
	}
	service := authorizationapp.NewService(repository, authorizationapp.ClockFunc(func() time.Time { return now }))
	snapshot, err := service.Snapshot(context.Background(), workspace, principalID)
	if err != nil {
		t.Fatal(err)
	}
	resource := authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}
	if snapshot.Allows(authorization.ActionAssetRead, resource) || !snapshot.Allows(authorization.ActionProposalReview, resource) ||
		snapshot.AuthorizationVersion != 2 || repository.bindingLoadCalls != 2 {
		t.Fatalf("stale role version escaped snapshot: %+v calls=%d", snapshot, repository.bindingLoadCalls)
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
