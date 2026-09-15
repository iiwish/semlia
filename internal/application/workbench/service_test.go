package workbench_test

import (
	"context"
	"errors"
	"testing"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	workbenchapp "github.com/iiwish/semlia/internal/application/workbench"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/workbench"
	"github.com/iiwish/semlia/pkg/identity"
)

type repository struct {
	items      []domain.Item
	counts     domain.Counts
	queries    []domain.ListQuery
	update     domain.UpdateCommand
	reconciled int
	reviewed   map[string]struct{}
}

func (repo *repository) ListAttentionItems(_ context.Context, query domain.ListQuery) ([]domain.Item, error) {
	repo.queries = append(repo.queries, query)
	return append([]domain.Item(nil), repo.items...), nil
}

func (repo *repository) CountAttentionItems(_ context.Context, query domain.ListQuery) (domain.Counts, error) {
	repo.queries = append(repo.queries, query)
	return repo.counts, nil
}

func (repo *repository) GetAttentionItem(_ context.Context, _ identity.WorkspaceID, id identity.AttentionItemID) (domain.Item, error) {
	for _, item := range repo.items {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.Item{}, domain.ErrNotFound
}

func (repo *repository) ReviewedAttentionItemIDs(
	_ context.Context, _ identity.WorkspaceID, _ identity.PrincipalID, _ []identity.AttentionItemID,
) (map[string]struct{}, error) {
	return repo.reviewed, nil
}

func (repo *repository) UpdateAttentionItem(_ context.Context, command domain.UpdateCommand) (domain.Item, error) {
	repo.update = command
	item := repo.items[0]
	item.AssigneePrincipalID = command.AssigneePrincipalID
	item.State = command.State
	item.Version++
	return item, nil
}

func (repo *repository) ReconcileAttentionItems(_ context.Context, limit int) (domain.ReconcileStats, error) {
	repo.reconciled = limit
	return domain.ReconcileStats{Scanned: limit, Upserted: limit - 1, Truncated: true}, nil
}

type evaluator struct {
	principal       identity.PrincipalID
	allowed         map[authorization.Action]bool
	snapshotAllowed map[authorization.Action]bool
	requests        []authorizationapp.EvaluationRequest
	scopeType       authorization.ScopeType
	scopeID         string
}

func (eval *evaluator) Evaluate(_ context.Context, request authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	eval.requests = append(eval.requests, request)
	return authorization.Decision{Allowed: eval.allowed[request.Action], Action: request.Action,
		PrincipalID: eval.principal, ReasonCode: authorization.ReasonRoleGrant, AuthorizationVersion: 7}, nil
}

func (eval *evaluator) Snapshot(_ context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID) (authorizationapp.AccessSnapshot, error) {
	allowed := eval.allowed
	if eval.snapshotAllowed != nil {
		allowed = eval.snapshotAllowed
	}
	actions := make([]authorization.Action, 0, len(allowed))
	workspaceActions := make([]authorization.Action, 0, 2)
	for action, permitted := range allowed {
		if permitted {
			if eval.scopeType != "" && (action == authorization.ActionWorkspaceRead || action == authorization.ActionWorkspaceManage) {
				workspaceActions = append(workspaceActions, action)
			} else {
				actions = append(actions, action)
			}
		}
	}
	bindingID, _ := identity.NewBindingID()
	scopeType, scopeID := eval.scopeType, eval.scopeID
	if scopeType == "" {
		scopeType, scopeID = authorization.ScopeWorkspace, workspace.UUID()
	}
	bindings := []authorization.RoleBinding{{ID: bindingID, WorkspaceID: workspace, PrincipalID: principal,
		RoleID: "reviewer", RoleVersion: 1, ScopeType: scopeType, ScopeID: scopeID,
		GrantedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Version: 1, Actions: actions}}
	if len(workspaceActions) > 0 {
		workspaceBindingID, _ := identity.NewBindingID()
		bindings = append(bindings, authorization.RoleBinding{ID: workspaceBindingID, WorkspaceID: workspace,
			PrincipalID: principal, RoleID: "workspace_reader", RoleVersion: 1,
			ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
			GrantedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Version: 1, Actions: workspaceActions})
	}
	return authorizationapp.AccessSnapshot{
		Principal: authorization.Principal{ID: principal, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
			DisplayName: "Reviewer", Status: authorization.PrincipalActive},
		Bindings:             bindings,
		AuthorizationVersion: 7, EvaluatedAt: time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC),
	}, nil
}

func TestSnapshotRevocationClosesEvaluateToRepositoryRace(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	other, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	itemID, _ := identity.NewAttentionItemID()
	baseItem := item(t, workspace, other, now)
	baseItem.ID = itemID
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"

	t.Run("list and get recheck workspace read", func(t *testing.T) {
		repo := &repository{items: []domain.Item{baseItem}}
		eval := &evaluator{principal: principal,
			allowed:         map[authorization.Action]bool{authorization.ActionWorkspaceRead: true},
			snapshotAllowed: map[authorization.Action]bool{authorization.ActionAssetRead: true}}
		service := workbenchapp.NewService(repo, eval, workbenchapp.ClockFunc(func() time.Time { return now }))
		if _, err := service.List(context.Background(), workbenchapp.ListRequest{WorkspaceID: workspace,
			PrincipalRef: principal.String(), TraceID: traceID}); err == nil {
			t.Fatal("list proceeded after workspace.read was revoked")
		}
		if len(repo.queries) != 0 {
			t.Fatal("list reached repository after snapshot revocation")
		}
		if _, err := service.Get(context.Background(), workspace, itemID, principal.String(), traceID); err == nil {
			t.Fatal("get proceeded after workspace.read was revoked")
		}
	})

	t.Run("update rechecks workspace manage", func(t *testing.T) {
		repo := &repository{items: []domain.Item{baseItem}}
		eval := &evaluator{principal: principal,
			allowed:         map[authorization.Action]bool{authorization.ActionWorkspaceManage: true},
			snapshotAllowed: map[authorization.Action]bool{authorization.ActionAssetRead: true}}
		service := workbenchapp.NewService(repo, eval, workbenchapp.ClockFunc(func() time.Time { return now }))
		if _, err := service.Update(context.Background(), workbenchapp.UpdateRequest{WorkspaceID: workspace,
			ItemID: itemID, PrincipalRef: principal.String(), TraceID: traceID, IdempotencyKey: "revoked",
			State: domain.StateDismissed, ExpectedVersion: baseItem.Version}); err == nil {
			t.Fatal("update proceeded after workspace.manage was revoked")
		}
		if !repo.update.ItemID.IsZero() {
			t.Fatal("update reached repository after snapshot revocation")
		}
	})
}

func TestListFailsClosedForUnmappedDomainScope(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 14, 2, 0, 0, time.UTC)
	repo := &repository{items: []domain.Item{item(t, workspace, principal, now)}}
	eval := &evaluator{principal: principal, allowed: map[authorization.Action]bool{
		authorization.ActionWorkspaceRead: true, authorization.ActionAssetRead: true,
	}, scopeType: authorization.ScopeDomain, scopeID: "commerce"}
	service := workbenchapp.NewService(repo, eval, workbenchapp.ClockFunc(func() time.Time { return now }))
	if _, err := service.List(context.Background(), workbenchapp.ListRequest{WorkspaceID: workspace,
		PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}); err != nil {
		t.Fatal(err)
	}
	if grants := repo.queries[0].AccessGrants; len(grants) != 0 {
		t.Fatalf("unmapped domain scope was treated as a workbench target grant: %+v", grants)
	}
}

func TestListIsBoundedCursorScopedAndActionsAreServerAuthorized(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	other, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	repo := &repository{counts: domain.Counts{Total: 2, Open: 2, Critical: 1}, items: []domain.Item{
		item(t, workspace, other, now), item(t, workspace, other, now.Add(-time.Minute)), item(t, workspace, other, now.Add(-2*time.Minute)),
	}}
	eval := &evaluator{principal: principal, allowed: map[authorization.Action]bool{
		authorization.ActionWorkspaceRead: true, authorization.ActionWorkspaceManage: true,
		authorization.ActionAssetRead: true, authorization.ActionProposalReview: true,
	}}
	service := workbenchapp.NewService(repo, eval, workbenchapp.ClockFunc(func() time.Time { return now }))
	page, err := service.List(context.Background(), workbenchapp.ListRequest{WorkspaceID: workspace,
		PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		View: domain.ViewTeam, Search: "proposal", Sort: domain.SortPriorityDesc, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.NextCursor == "" || page.Counts.Total != 2 {
		t.Fatalf("page = %+v", page)
	}
	if got := page.Items[0].NextActions; !contains(got, domain.ActionReview) || !contains(got, domain.ActionAssign) || !contains(got, domain.ActionDismiss) {
		t.Fatalf("next actions = %v", got)
	}
	if len(repo.queries) != 2 || repo.queries[0].Limit != 3 || repo.queries[0].PrincipalID != principal ||
		len(repo.queries[0].AllowedKinds) != 2 || repo.queries[0].Search != "proposal" || repo.queries[0].AuthorizationVersion != 7 {
		t.Fatalf("repository query = %+v", repo.queries)
	}
	if len(eval.requests) != 1 || eval.requests[0].Action != authorization.ActionWorkspaceRead {
		t.Fatalf("audited authorization requests = %+v, want one workspace.read", eval.requests)
	}
	if _, err := service.List(context.Background(), workbenchapp.ListRequest{WorkspaceID: workspace,
		PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		View: domain.ViewMine, Sort: domain.SortPriorityDesc, Limit: 2, Cursor: page.NextCursor}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("cursor accepted with changed view: %v", err)
	}
}

func TestListSuppressesReviewAndPublishForTheInitiator(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 14, 5, 0, 0, time.UTC)
	review := item(t, workspace, principal, now)
	repo := &repository{items: []domain.Item{review}}
	eval := &evaluator{principal: principal, allowed: map[authorization.Action]bool{
		authorization.ActionWorkspaceRead: true, authorization.ActionAssetRead: true,
		authorization.ActionProposalReview: true,
	}}
	service := workbenchapp.NewService(repo, eval, workbenchapp.ClockFunc(func() time.Time { return now }))
	page, err := service.List(context.Background(), workbenchapp.ListRequest{WorkspaceID: workspace,
		PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err != nil {
		t.Fatal(err)
	}
	if contains(page.Items[0].NextActions, domain.ActionReview) {
		t.Fatalf("author received own review action: %v", page.Items[0].NextActions)
	}
}

func TestAssetScopedReviewerCanSeeAssetItemButCannotInvokeWorkspaceScopedReview(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	asset, _ := identity.NewAssetID()
	principal, _ := identity.NewPrincipalID()
	other, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 14, 6, 0, 0, time.UTC)
	review := item(t, workspace, other, now)
	review.TargetType, review.TargetID, review.AudienceRoleID = "asset", asset.String(), "reviewer"
	repo := &repository{items: []domain.Item{review}}
	eval := &evaluator{principal: principal, scopeType: authorization.ScopeAsset, scopeID: asset.UUID(), allowed: map[authorization.Action]bool{
		authorization.ActionWorkspaceRead: true, authorization.ActionAssetRead: true,
		authorization.ActionProposalReview: true,
	}}
	service := workbenchapp.NewService(repo, eval, workbenchapp.ClockFunc(func() time.Time { return now }))
	page, err := service.List(context.Background(), workbenchapp.ListRequest{WorkspaceID: workspace,
		PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", View: domain.ViewTeam})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("asset-scoped review item = %+v, err = %v", page, err)
	}
	if contains(page.Items[0].NextActions, domain.ActionReview) {
		t.Fatalf("asset-scoped grant incorrectly authorized workspace review: %v", page.Items[0].NextActions)
	}
}

func TestListSuppressesReviewAfterPrincipalAlreadyReviewed(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	other, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 14, 7, 0, 0, time.UTC)
	review := item(t, workspace, other, now)
	repo := &repository{items: []domain.Item{review}, reviewed: map[string]struct{}{review.ID.String(): {}}}
	eval := &evaluator{principal: principal, allowed: map[authorization.Action]bool{
		authorization.ActionWorkspaceRead: true, authorization.ActionAssetRead: true,
		authorization.ActionProposalReview: true,
	}}
	service := workbenchapp.NewService(repo, eval, workbenchapp.ClockFunc(func() time.Time { return now }))
	page, err := service.List(context.Background(), workbenchapp.ListRequest{WorkspaceID: workspace,
		PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err != nil {
		t.Fatal(err)
	}
	if contains(page.Items[0].NextActions, domain.ActionReview) {
		t.Fatalf("completed reviewer received stale review action: %v", page.Items[0].NextActions)
	}
}

func TestUpdateUsesAuthorizedActorOptimisticVersionAndImmutableAuditID(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	assignee, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 14, 10, 0, 0, time.UTC)
	repo := &repository{items: []domain.Item{item(t, workspace, principal, now)}}
	eval := &evaluator{principal: principal, allowed: map[authorization.Action]bool{
		authorization.ActionWorkspaceManage: true, authorization.ActionAssetRead: true,
	}}
	service := workbenchapp.NewService(repo, eval, workbenchapp.ClockFunc(func() time.Time { return now }))
	updated, err := service.Update(context.Background(), workbenchapp.UpdateRequest{WorkspaceID: workspace,
		ItemID: repo.items[0].ID, PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		IdempotencyKey: "assign-1", AssigneePrincipalID: &assignee, SetAssignee: true,
		State: domain.StateInProgress, ExpectedVersion: 3})
	if err != nil {
		t.Fatal(err)
	}
	if repo.update.ActorPrincipalID != principal || repo.update.ExpectedVersion != 3 || repo.update.AuditEventID.IsZero() ||
		repo.update.IdempotencyKey != "assign-1" || len(repo.update.RequestFingerprint) != 64 ||
		repo.update.AuthorizationVersion != 7 || len(repo.update.VisibilityFingerprint) != 64 ||
		updated.State != domain.StateInProgress || updated.AssigneePrincipalID == nil || *updated.AssigneePrincipalID != assignee {
		t.Fatalf("update command/result = %+v / %+v", repo.update, updated)
	}
}

func TestManageOnlyCannotReadOrMutateInvisibleItem(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	other, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 14, 12, 0, 0, time.UTC)
	value := item(t, workspace, other, now)
	value.AssigneePrincipalID = &other
	repo := &repository{items: []domain.Item{value}}
	eval := &evaluator{principal: principal, allowed: map[authorization.Action]bool{
		authorization.ActionWorkspaceRead: true, authorization.ActionWorkspaceManage: true,
	}}
	service := workbenchapp.NewService(repo, eval, workbenchapp.ClockFunc(func() time.Time { return now }))
	if _, err := service.Get(context.Background(), workspace, value.ID, principal.String(), "4bf92f3577b34da6a3ce929d0e0e4736"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("manage-only get error = %v", err)
	}
	if _, err := service.Update(context.Background(), workbenchapp.UpdateRequest{WorkspaceID: workspace,
		ItemID: value.ID, PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		IdempotencyKey: "probe", State: domain.StateDismissed, ExpectedVersion: value.Version}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("manage-only update error = %v", err)
	}
	if !repo.update.ItemID.IsZero() {
		t.Fatalf("unauthorized update reached repository: %+v", repo.update)
	}
}

func TestUpdateResponseKeepsCompletedReviewActionSuppressed(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	other, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 14, 11, 0, 0, time.UTC)
	value := item(t, workspace, other, now)
	repo := &repository{items: []domain.Item{value}, reviewed: map[string]struct{}{value.ID.String(): {}}}
	eval := &evaluator{principal: principal, allowed: map[authorization.Action]bool{
		authorization.ActionWorkspaceManage: true, authorization.ActionAssetRead: true,
		authorization.ActionProposalReview: true,
	}}
	service := workbenchapp.NewService(repo, eval, workbenchapp.ClockFunc(func() time.Time { return now }))
	updated, err := service.Update(context.Background(), workbenchapp.UpdateRequest{WorkspaceID: workspace,
		ItemID: value.ID, PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		IdempotencyKey: "reviewed-replay", State: domain.StateInProgress, ExpectedVersion: value.Version})
	if err != nil {
		t.Fatal(err)
	}
	if contains(updated.NextActions, domain.ActionReview) {
		t.Fatalf("update response revived completed review action: %v", updated.NextActions)
	}
}

func TestGetHidesReadableItemOutsideAllSupportedViews(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	other, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 14, 13, 0, 0, time.UTC)
	value := item(t, workspace, other, now)
	value.AssigneePrincipalID = &other
	repo := &repository{items: []domain.Item{value}}
	eval := &evaluator{principal: principal, allowed: map[authorization.Action]bool{
		authorization.ActionWorkspaceRead: true, authorization.ActionAssetRead: true,
	}}
	service := workbenchapp.NewService(repo, eval, workbenchapp.ClockFunc(func() time.Time { return now }))
	if _, err := service.Get(context.Background(), workspace, value.ID, principal.String(), "4bf92f3577b34da6a3ce929d0e0e4736"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("invisible assigned item get error = %v", err)
	}
}

func TestStartupReconcileIsExplicitAndBounded(t *testing.T) {
	principal, _ := identity.NewPrincipalID()
	repo := &repository{}
	service := workbenchapp.NewService(repo, &evaluator{principal: principal}, workbenchapp.ClockFunc(time.Now))
	if _, err := service.ReconcileStartup(context.Background(), 10_001); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("unbounded reconcile error = %v", err)
	}
	stats, err := service.ReconcileStartup(context.Background(), 10_000)
	if err != nil || repo.reconciled != 10_000 || stats.Scanned != 10_000 {
		t.Fatalf("reconcile stats=%+v limit=%d err=%v", stats, repo.reconciled, err)
	}
}

func item(t *testing.T, workspace identity.WorkspaceID, initiator identity.PrincipalID, updated time.Time) domain.Item {
	t.Helper()
	id, _ := identity.NewAttentionItemID()
	return domain.Item{ID: id, WorkspaceID: workspace, Kind: domain.KindReview, DedupeKey: "review:" + id.String(),
		State: domain.StateOpen, Priority: domain.PriorityHigh, Risk: domain.PriorityHigh,
		TargetType: "proposal", TargetID: "prp_01arz3ndektsv4rrffq69g5fav", TargetRoute: "/governance/proposals/prp_01arz3ndektsv4rrffq69g5fav",
		InitiatorPrincipalID: &initiator, RuleVersion: "workbench.review.v1", Title: "Review proposal", Summary: "Proposal requires review.",
		ReasonCode: "PROPOSAL_REVIEW_REQUIRED", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", OpenedAt: updated, UpdatedAt: updated, Version: 1}
}

func contains(actions []domain.Action, target domain.Action) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}
