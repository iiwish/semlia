package governance

import (
	"context"
	"testing"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
)

type committedAccessEvaluator struct {
	decision authorization.Decision
	snapshot authorizationapp.AccessSnapshot
}

type evaluateOnlyCommittedAccess struct {
	decision authorization.Decision
}

func (evaluator *evaluateOnlyCommittedAccess) Evaluate(
	_ context.Context, request authorizationapp.EvaluationRequest,
) (authorization.Decision, error) {
	evaluator.decision.Action = request.Action
	return evaluator.decision, nil
}

func (evaluator *committedAccessEvaluator) Evaluate(
	_ context.Context, request authorizationapp.EvaluationRequest,
) (authorization.Decision, error) {
	evaluator.decision.Action = request.Action
	return evaluator.decision, nil
}

func (evaluator *committedAccessEvaluator) Snapshot(
	_ context.Context, _ identity.WorkspaceID, _ identity.PrincipalID,
) (authorizationapp.AccessSnapshot, error) {
	return evaluator.snapshot, nil
}

func TestCommittedReleaseSensitiveSectionsUsePostDecisionSnapshot(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	bindingID, _ := identity.NewBindingID()
	now := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	evaluator := &committedAccessEvaluator{
		decision: authorization.Decision{
			Allowed: true, PrincipalID: principal, ReasonCode: authorization.ReasonRoleGrant,
			AuthorizationVersion: 7,
		},
		snapshot: authorizationapp.AccessSnapshot{
			Principal: authorization.Principal{
				ID: principal, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
				Status: authorization.PrincipalActive,
			},
			AuthorizationVersion: 8,
			EvaluatedAt:          now,
			Bindings: []authorization.RoleBinding{{
				ID: bindingID, WorkspaceID: workspace, PrincipalID: principal,
				RoleID: "custom_publisher", RoleVersion: 1, ScopeType: authorization.ScopeWorkspace,
				ScopeID: workspace.UUID(), GrantedAt: now.Add(-time.Minute), Version: 1,
			}},
		},
	}
	service := &PublishingService{authorizer: evaluator}
	allowed, err := service.authorizeCommittedBindingRead(context.Background(), workspace, principal.String(), "trace")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("binding.read revoked after the command decision still exposed sensitive release sections")
	}
	evaluator.snapshot.Bindings[0].Actions = []authorization.Action{authorization.ActionBindingRead}
	allowed, err = service.authorizeCommittedBindingRead(context.Background(), workspace, principal.String(), "trace")
	if err != nil || !allowed {
		t.Fatalf("active binding.read snapshot allowed=%v err=%v", allowed, err)
	}
}

func TestCommittedReleaseSensitiveSectionsFailClosedWithoutSnapshot(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	service := &PublishingService{authorizer: &evaluateOnlyCommittedAccess{decision: authorization.Decision{
		Allowed: true, PrincipalID: principal, ReasonCode: authorization.ReasonRoleGrant,
	}}}
	allowed, err := service.authorizeCommittedBindingRead(context.Background(), workspace, principal.String(), "trace")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("evaluator without an authoritative snapshot exposed binding-protected release sections")
	}
}
