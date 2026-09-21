package projection_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"crypto/sha256"
	"encoding/hex"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/iiwish/semlia/internal/adapters/gitcontent"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/jobs"
	projectionapp "github.com/iiwish/semlia/internal/application/projection"
	"github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestGovernedLoopPublishesAndRollsBackWithAppendOnlyProjection(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE workspaces CASCADE"); err != nil {
		t.Fatal(err)
	}
	workspace := createWorkspace(t, pool, "governed-loop")
	store := pgstore.NewStore(pool)
	clock := governanceapp.ClockFunc(clockTime)

	// Governed-loop services: authoring (with publishing composed), review and
	// the production validation handler.
	authorizer := authorizationapp.NewService(
		store, authorizationapp.ClockFunc(clockTime), authorizationapp.WithLocalUATIdentities(),
	)
	governancePolicy := governanceapp.NewPolicyService(store, clock, governanceapp.WithRuleSource(store))
	authoring := governanceapp.NewAuthoringService(
		store,
		governanceapp.NewProposalService(store, clock),
		governanceapp.NewAgentRunService(store, clock),
		authorizer, clock,
		governanceapp.WithValidationOrchestrator(
			governanceapp.NewValidationOrchestrator(
				governanceapp.NewProposalService(store, clock), store, clock,
			),
		),
		governanceapp.WithDecisionRefresher(
			governanceapp.NewPolicyDecisionTrigger(store, governancePolicy),
		),
	)

	// People: an author, an independent approver and a publisher (all human).
	author := createPrincipal(t, store, workspace, "author", []string{"asset_owner"}, false)
	approver := createPrincipal(t, store, workspace, "approver", []string{"reviewer"}, false)
	publisher := createPrincipal(t, store, workspace, "publisher", []string{"publisher"}, false)

	created, err := catalogapp.NewService(store, catalogapp.ClockFunc(func() time.Time { return clockTime() })).CreateAsset(ctx, catalogapp.CreateAssetRequest{
		WorkspaceID: workspace, Address: "commerce.revenue.net_revenue", AssetType: semantic.BusinessTerm,
		Lifecycle: "active", SchemaVersion: "1.0.0", CreatedBy: author.String(), TraceID: traceID,
		Content: json.RawMessage(`{"assetType":"business_term","name":"Net revenue","definition":"Revenue less refunds","scope":"Synthetic finance fixture","spec":{"capability":"definition"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := authoring.CreateProposal(ctx, governanceapp.CreateAuthoringProposalRequest{
		WorkspaceID: workspace, TargetType: governance.TargetSemanticAsset,
		TargetObjectID: created.ID.String(), BaseRevisionID: created.CurrentRevision.ID,
		Title: "Tighten net revenue", Reason: "Finance review",
		ChangeSet: []governanceapp.ChangeSetItemInput{{
			FieldPath: "definition", Op: governance.ChangeUpdate,
			BeforeDigest: sha256Of(`"Revenue less refunds"`), AfterDigest: sha256Of(`"Revenue less refunds and chargebacks"`),
			BeforeValue: json.RawMessage(`"Revenue less refunds"`), AfterValue: json.RawMessage(`"Revenue less refunds and chargebacks"`),
		}},
		CreatedBy: author.String(), PrincipalRef: author.String(), TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authoring.SubmitProposal(ctx, governanceapp.SubmitAuthoringProposalRequest{
		WorkspaceID: workspace, ProposalID: proposal.Proposal.ID,
		PrincipalRef: author.String(), TraceID: traceID,
	}); err != nil {
		t.Fatal(err)
	}
	drainValidation(t, store, clock)
	inReview, err := store.GetProposal(ctx, workspace, proposal.Proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inReview.State != governance.ProposalInReview {
		t.Fatalf("proposal state after validation = %s, want in_review", inReview.State)
	}
	if _, err := authoring.ReviewProposal(ctx, governanceapp.ReviewProposalRequest{
		WorkspaceID: workspace, ProposalID: proposal.Proposal.ID,
		Decision: governanceapp.ReviewCommandApprove, Reason: "independent approval",
		PrincipalRef: approver.String(), TraceID: traceID,
	}); err != nil {
		t.Fatal(err)
	}

	// Real local Git repository wired exactly like the production worker.
	repositoryPath := t.TempDir()
	writer, err := gitcontent.Open(repositoryPath)
	if err != nil {
		t.Fatal(err)
	}
	router := jobs.NewRouterPublisher()
	router.Register(projectionapp.CatalogAssetChanged, projectionapp.NewPublisher(store, writer))
	router.Register(projectionapp.ReleasePublished, projectionapp.NewReleasePublisher(store, writer))
	dispatcher := jobs.NewDispatcher(store, router,
		jobs.ClockFunc(func() time.Time { return clockTime() }),
		jobs.BackoffFunc(func(int32) time.Duration { return 0 }), 30*time.Second)

	publishing := authoring.Publishing()
	release, err := publishing.PublishProposal(ctx, governanceapp.PublishProposalRequest{
		WorkspaceID: workspace, ProposalID: proposal.Proposal.ID,
		PrincipalRef: publisher.String(), TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if release.Sequence != 1 || len(release.Entries) != 1 {
		t.Fatalf("release = %+v", release)
	}
	if err := drainOutbox(t, dispatcher); err != nil {
		t.Fatalf("project publish: %v", err)
	}

	// The release document is projected next to the updated asset content.
	releaseDocPath := filepath.Join(repositoryPath, "releases", fmt.Sprintf("1-%s.json", release.ID.String()))
	projected, err := os.ReadFile(releaseDocPath)
	if err != nil {
		t.Fatalf("release document not projected: %v", err)
	}
	var document struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Metadata   struct {
			ReleaseID      string `json:"releaseId"`
			Sequence       int64  `json:"sequence"`
			ManifestDigest string `json:"manifestDigest"`
			State          string `json:"state"`
			PublishedBy    string `json:"publishedBy"`
		} `json:"metadata"`
		Spec struct {
			Manifest struct {
				Assets []struct {
					AssetID    string `json:"assetId"`
					RevisionID string `json:"revisionId"`
				} `json:"assets"`
			} `json:"manifest"`
			Proposal *struct {
				ProposalID string `json:"proposalId"`
			} `json:"proposal"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(projected, &document); err != nil {
		t.Fatalf("decode release document: %v\n%s", err, projected)
	}
	if document.Kind != "SemanticAssetRelease" || document.APIVersion != "semlia.io/v1" {
		t.Fatalf("release document header = %s/%s", document.APIVersion, document.Kind)
	}
	if document.Metadata.ReleaseID != release.ID.String() || document.Metadata.State != "published" ||
		document.Metadata.ManifestDigest != release.ManifestDigest {
		t.Fatalf("release document metadata = %+v", document.Metadata)
	}
	if len(document.Spec.Manifest.Assets) != 1 ||
		document.Spec.Manifest.Assets[0].RevisionID != release.Entries[0].RevisionID.String() {
		t.Fatalf("release document manifest = %+v", document.Spec.Manifest)
	}
	if document.Spec.Proposal == nil || document.Spec.Proposal.ProposalID != proposal.Proposal.ID.String() {
		t.Fatalf("release document proposal = %+v", document.Spec.Proposal)
	}

	assetDocPath := filepath.Join(repositoryPath, "assets", "commerce", "revenue", "net_revenue.json")
	assetDocument, err := os.ReadFile(assetDocPath)
	if err != nil || !contains(string(assetDocument), release.Entries[0].RevisionID.String()) {
		t.Fatalf("asset content projection did not follow the published revision: %s, err %v", assetDocument, err)
	}

	// Rollback as a new release restores the prior revision pointer and is
	// projected the same append-only way.
	rollback, err := publishing.RollbackRelease(ctx, governanceapp.RollbackReleaseRequest{
		WorkspaceID: workspace, ReleaseID: release.ID,
		PrincipalRef: publisher.String(), TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rollback.Sequence != 2 || rollback.RolledBackToReleaseID == nil ||
		*rollback.RolledBackToReleaseID != release.ID {
		t.Fatalf("rollback release = %+v", rollback)
	}
	if err := drainOutbox(t, dispatcher); err != nil {
		t.Fatalf("project rollback: %v", err)
	}
	rollbackDocPath := filepath.Join(repositoryPath, "releases", fmt.Sprintf("2-%s.json", rollback.ID.String()))
	rollbackDocument, err := os.ReadFile(rollbackDocPath)
	if err != nil {
		t.Fatalf("rollback release document not projected: %v", err)
	}
	if !contains(string(rollbackDocument), rollback.RolledBackToReleaseID.String()) {
		t.Fatalf("rollback document without the target reference: %s", rollbackDocument)
	}
	restored, err := os.ReadFile(assetDocPath)
	if err != nil || !contains(string(restored), created.CurrentRevision.ID.String()) {
		t.Fatalf("asset content projection did not follow the rollback: %s, err %v", restored, err)
	}

	// Audit and outbox assertions over the whole loop.
	var publishAudit, rollbackAudit int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'governance.release.published'`,
		workspace.UUID()).Scan(&publishAudit); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'governance.release.rolled_back'`,
		workspace.UUID()).Scan(&rollbackAudit); err != nil {
		t.Fatal(err)
	}
	if publishAudit != 1 || rollbackAudit != 1 {
		t.Fatalf("release audit facts = %d/%d, want 1/1", publishAudit, rollbackAudit)
	}

	// Append-only proof: the commit graph is linear, the only ref is the
	// default branch, and there are no tags or remotes.
	assertAppendOnlyGraph(t, repositoryPath, 5)
}

func assertAppendOnlyGraph(t *testing.T, repositoryPath string, wantCommits int) {
	t.Helper()
	repository, err := git.PlainOpen(repositoryPath)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repository.Head()
	if err != nil {
		t.Fatal(err)
	}
	if head.Name() != plumbing.ReferenceName("refs/heads/master") {
		t.Fatalf("head ref = %s, want the untouched default branch", head.Name())
	}
	refs, err := repository.References()
	if err != nil {
		t.Fatal(err)
	}
	branchCount := 0
	if err := refs.ForEach(func(reference *plumbing.Reference) error {
		if reference.Name() == plumbing.HEAD {
			return nil
		}
		branchCount++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if branchCount != 1 {
		t.Fatalf("repository has %d branch refs, want only the default branch", branchCount)
	}
	tags, err := repository.Tags()
	if err != nil {
		t.Fatal(err)
	}
	if err := tags.ForEach(func(reference *plumbing.Reference) error {
		t.Fatalf("repository created a tag %s", reference.Name())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	remotes, err := repository.Remotes()
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 0 {
		t.Fatalf("repository created %d remotes, want none", len(remotes))
	}

	commits := make([]*object.Commit, 0, wantCommits)
	iterator, err := repository.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		t.Fatal(err)
	}
	if err := iterator.ForEach(func(commit *object.Commit) error {
		commits = append(commits, commit)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(commits) != wantCommits {
		t.Fatalf("commit count = %d, want %d", len(commits), wantCommits)
	}
	for index, commit := range commits {
		if index == len(commits)-1 {
			if commit.NumParents() != 0 {
				t.Fatalf("root commit %s has %d parents", commit.Hash, commit.NumParents())
			}
			continue
		}
		if commit.NumParents() != 1 || commit.ParentHashes[0] != commits[index+1].Hash {
			t.Fatalf("commit %s breaks the linear append-only graph", commit.Hash)
		}
	}
}

func createPrincipal(
	t *testing.T, store *pgstore.Store, workspace identity.WorkspaceID,
	name string, roles []string, agent bool,
) identity.PrincipalID {
	t.Helper()
	principalID, err := identity.NewPrincipalID()
	if err != nil {
		t.Fatal(err)
	}
	principal := authorization.Principal{
		ID: principalID, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: name, Status: authorization.PrincipalActive,
	}
	if agent {
		owner, ownerErr := identity.NewPrincipalID()
		if ownerErr != nil {
			t.Fatal(ownerErr)
		}
		principal.Kind = authorization.PrincipalAgent
		principal.OwnerPrincipalID = &owner
	}
	if _, err := store.CreatePrincipal(context.Background(), principal); err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		bindingID, bindingErr := identity.NewBindingID()
		if bindingErr != nil {
			t.Fatal(bindingErr)
		}
		if _, err := store.CreateRoleBinding(context.Background(), authorization.RoleBinding{
			ID: bindingID, PrincipalID: principalID, RoleID: role,
			ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return principalID
}

func drainValidation(t *testing.T, store *pgstore.Store, clock governanceapp.Clock) {
	t.Helper()
	handler := governanceapp.NewValidationJobHandler(
		store,
		governanceapp.NewProposalService(store, clock),
		governanceapp.NewValidationService(store, clock),
		governanceapp.NewDefaultRegistry(),
		governanceapp.NewPolicyService(store, clock, governanceapp.WithRuleSource(store)),
		clock,
	)
	worker := jobs.NewWorker(store, jobs.ClockFunc(time.Now),
		jobs.BackoffFunc(func(int32) time.Duration { return time.Minute }), time.Minute)
	worker.Register(governanceapp.ValidationJobType, handler.Handle)
	processed, err := worker.RunOne(context.Background(), "governed-loop-test")
	if err != nil {
		t.Fatalf("validation worker: %v", err)
	}
	if !processed {
		t.Fatal("validation worker found no job")
	}
}

func drainOutbox(t *testing.T, dispatcher *jobs.Dispatcher) error {
	t.Helper()
	pool, err := pgstore.Open(context.Background(), databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	for range 8 {
		processed, err := dispatcher.RunOne(context.Background(), "governed-loop-test")
		if err != nil {
			return err
		}
		if !processed {
			break
		}
	}
	rows, err := pool.Query(context.Background(), `
		SELECT event_type, status, COALESCE(last_error_code, '') FROM outbox_events
		WHERE status <> 'published'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var failures []string
	for rows.Next() {
		var eventType, status, errorCode string
		if err := rows.Scan(&eventType, &status, &errorCode); err != nil {
			return err
		}
		failures = append(failures, eventType+"/"+status+"/"+errorCode)
	}
	if len(failures) > 0 {
		return fmt.Errorf("undelivered outbox events: %v", failures)
	}
	return nil
}

func sha256Of(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func clockTime() time.Time {
	return time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
}
