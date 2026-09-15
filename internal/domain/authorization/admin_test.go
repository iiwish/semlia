package authorization_test

import (
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/domain/authorization"
)

func TestRoleBindingLifecycleIsTimeAware(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	expires := now.Add(time.Minute)
	binding := authorization.RoleBinding{ExpiresAt: &expires}
	if status := binding.StatusAt(now); status != authorization.BindingActive {
		t.Fatalf("status = %q, want active", status)
	}
	if status := binding.StatusAt(expires); status != authorization.BindingExpired {
		t.Fatalf("status = %q, want expired", status)
	}
	revoked := now.Add(-time.Minute)
	binding.RevokedAt = &revoked
	if status := binding.StatusAt(now); status != authorization.BindingRevoked {
		t.Fatalf("status = %q, want revoked", status)
	}
}

func TestRoleActionsConflictAcrossReviewAndPublish(t *testing.T) {
	if !authorization.ActionsConflict(
		[]authorization.Action{authorization.ActionProposalReview},
		[]authorization.Action{authorization.ActionReleasePublish},
	) {
		t.Fatal("review and publish actions must conflict")
	}
	if authorization.ActionsConflict(
		[]authorization.Action{authorization.ActionAssetRead},
		[]authorization.Action{authorization.ActionReleasePublish},
	) {
		t.Fatal("read-only role must not conflict with publisher")
	}
}

func TestScopeOverlapIsConservative(t *testing.T) {
	workspace := authorization.RoleBinding{ScopeType: authorization.ScopeWorkspace, ScopeID: "workspace-1"}
	asset := authorization.RoleBinding{ScopeType: authorization.ScopeAsset, ScopeID: "asset-1"}
	otherAsset := authorization.RoleBinding{ScopeType: authorization.ScopeAsset, ScopeID: "asset-2"}
	if !authorization.BindingsOverlap(workspace, asset, "workspace-1") {
		t.Fatal("workspace binding must overlap its asset binding")
	}
	if authorization.BindingsOverlap(asset, otherAsset, "workspace-1") {
		t.Fatal("different asset bindings must not overlap")
	}
}
