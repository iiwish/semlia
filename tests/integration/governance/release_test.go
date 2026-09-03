package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/authorization"
	governance "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func releasesPath(t *testing.T, workspace identity.WorkspaceID) string {
	t.Helper()
	return "/api/v1/workspaces/" + workspace.String() + "/governance/releases"
}

func authorProposalBody(assetID, baseRevisionID, author string) string {
	return fmt.Sprintf(
		`{"targetObjectType":"semantic_asset","targetObjectId":%q,"baseRevisionId":%q,"title":"Tighten metric definition","summary":"Clarifies refunds","reason":"Audit finding","changeSet":[%s],"createdBy":%q}`,
		assetID, baseRevisionID,
		validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks"),
		author)
}

func (environment *fixture) createAssetWithAddress(
	t *testing.T, workspace identity.WorkspaceID, address string,
) (identity.AssetID, identity.RevisionID) {
	t.Helper()
	catalog := catalogapp.NewService(
		environment.store,
		catalogapp.ClockFunc(func() time.Time { return time.Now().UTC() }),
	)
	created, err := catalog.CreateAsset(context.Background(), catalogapp.CreateAssetRequest{
		WorkspaceID: workspace, Address: address, AssetType: semantic.Metric,
		Lifecycle: "active", SchemaVersion: "1.0.0",
		Content:   json.RawMessage(`{"name":"Revenue variant","definition":"Revenue baseline"}`),
		CreatedBy: "founder", TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return created.ID, created.CurrentRevision.ID
}

func createAgentPrincipal(
	t *testing.T, environment *fixture, workspace identity.WorkspaceID,
	owner identity.PrincipalID, name string,
) identity.PrincipalID {
	t.Helper()
	agentID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: agentID, WorkspaceID: workspace, Kind: authorization.PrincipalAgent,
		DisplayName: name, OwnerPrincipalID: &owner, Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.store.CreateRoleBinding(context.Background(), authorization.RoleBinding{
		ID: mustID(t, identity.NewBindingID), PrincipalID: agentID, RoleID: "publisher",
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
	}); err != nil {
		t.Fatal(err)
	}
	return agentID
}

func proposeToInReview(
	t *testing.T, environment *fixture, workspace identity.WorkspaceID,
	author identity.PrincipalID, assetID identity.AssetID, baseRevisionID identity.RevisionID,
) (identity.AssetID, identity.RevisionID, string) {
	t.Helper()
	path := environment.proposalsPath(t, workspace)
	created := environment.request(t, http.MethodPost, path, author.String(),
		authorProposalBody(assetID.String(), baseRevisionID.String(), author.String()))
	if created.Code != http.StatusCreated {
		t.Fatalf("proposal create status = %d, body = %s", created.Code, created.Body.String())
	}
	proposalID, _ := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
	submitted := environment.request(t, http.MethodPost, path+"/"+proposalID+"/submit", author.String(), "")
	if submitted.Code != http.StatusOK {
		t.Fatalf("proposal submit status = %d, body = %s", submitted.Code, submitted.Body.String())
	}
	environment.runValidationWorker(t)
	assertProposalState(t, environment, workspace, path, proposalID, "in_review")
	return assetID, baseRevisionID, proposalID
}

func approveProposal(
	t *testing.T, environment *fixture, workspace identity.WorkspaceID,
	reviewer identity.PrincipalID, proposalID string,
) {
	t.Helper()
	review := environment.request(t, http.MethodPost,
		environment.proposalsPath(t, workspace)+"/"+proposalID+"/reviews",
		reviewer.String(), reviewBody("approve", "independent review of the change"))
	if review.Code != http.StatusCreated {
		t.Fatalf("review status = %d, body = %s", review.Code, review.Body.String())
	}
}

func publishProposal(
	t *testing.T, environment *fixture, workspace identity.WorkspaceID,
	publisher identity.PrincipalID, proposalID string,
) *httptest.ResponseRecorder {
	t.Helper()
	return environment.request(t, http.MethodPost, releasesPath(t, workspace),
		publisher.String(), fmt.Sprintf(`{"proposalId":%q}`, proposalID))
}

func rollbackRelease(
	t *testing.T, environment *fixture, workspace identity.WorkspaceID,
	publisher identity.PrincipalID, releaseID string,
) *httptest.ResponseRecorder {
	t.Helper()
	return environment.request(t, http.MethodPost,
		releasesPath(t, workspace)+"/"+releaseID+"/rollback", publisher.String(), "")
}

func releaseDeniedAuditCount(t *testing.T, environment *fixture, workspace identity.WorkspaceID) int {
	t.Helper()
	var count int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'governance.release.denied'`,
		workspace.UUID()).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestPublishRequiresApprovingReviewAndAuditsTheRefusal(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "release-no-approval")
	author := createPrincipalWithRoles(t, environment, workspace, "author", []string{"asset_owner"})
	publisher := createPrincipalWithRoles(t, environment, workspace, "publisher", []string{"publisher"})
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	_, _, proposalID := proposeToInReview(t, environment, workspace, author, assetID, baseRevisionID)

	published := publishProposal(t, environment, workspace, publisher, proposalID)
	if published.Code != http.StatusUnprocessableEntity {
		t.Fatalf("publish without approval status = %d, want 422, body = %s", published.Code, published.Body.String())
	}
	assertJSONField(t, published.Body.Bytes(), "code", "NO_APPROVING_REVIEW")
	assertTableCount(t, environment.pool, "releases", 0)
	assertProposalState(t, environment, workspace, environment.proposalsPath(t, workspace), proposalID, "in_review")
	if releaseDeniedAuditCount(t, environment, workspace) != 1 {
		t.Fatal("publish refusal without approval was not audited")
	}
}

func TestPublishRefusesBlockedProposalAndMissingCapability(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "release-blocked")
	author := createPrincipalWithRoles(t, environment, workspace, "author", []string{"asset_owner"})
	publisher := createPrincipalWithRoles(t, environment, workspace, "publisher", []string{"publisher"})
	reviewer := createReviewerPrincipal(t, environment, workspace, "reviewer")

	// Capability gates run after the approval gate: an approved proposal with
	// no blockers reaches the release.publish evaluation.
	approvedAsset, approvedBase := environment.createAsset(t, workspace)
	_, _, approved := proposeToInReview(t, environment, workspace, author, approvedAsset, approvedBase)
	approveProposal(t, environment, workspace, reviewer, approved)

	unbound := createPrincipalWithRoles(t, environment, workspace, "unbound publisher", []string{"reviewer"})
	denied := publishProposal(t, environment, workspace, unbound, approved)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("publish without capability status = %d, want 403, body = %s", denied.Code, denied.Body.String())
	}
	assertJSONField(t, denied.Body.Bytes(), "code", "NO_MATCHING_GRANT")

	agent := createAgentPrincipal(t, environment, workspace, publisher, "release agent")
	agentDenied := publishProposal(t, environment, workspace, agent, approved)
	if agentDenied.Code != http.StatusForbidden {
		t.Fatalf("agent publish status = %d, want 403, body = %s", agentDenied.Code, agentDenied.Body.String())
	}
	assertJSONField(t, agentDenied.Body.Bytes(), "code", "SEPARATION_OF_DUTY")

	// A blocker finding anywhere on the proposal blocks the release cut.
	blockedAsset, blockedBase := environment.createAssetWithAddress(t, workspace, "commerce.gross_revenue")
	_, _, blockedProposal := proposeToInReview(t, environment, workspace, author, blockedAsset, blockedBase)
	approveProposal(t, environment, workspace, reviewer, blockedProposal)
	seedBlockerFinding(t, environment, workspace, blockedProposal)

	blocked := publishProposal(t, environment, workspace, publisher, blockedProposal)
	if blocked.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blocked publish status = %d, want 422, body = %s", blocked.Code, blocked.Body.String())
	}
	assertJSONField(t, blocked.Body.Bytes(), "code", "BLOCKED_BY_FINDINGS")
	assertTableCount(t, environment.pool, "releases", 0)
}

func TestPublishRefusesProposalAuthorAndSoleApprovingReviewer(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "release-sod")
	author := createPrincipalWithRoles(t, environment, workspace, "author publisher",
		[]string{"asset_owner", "publisher"})
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	_, _, proposalID := proposeToInReview(t, environment, workspace, author, assetID, baseRevisionID)

	reviewer := createPrincipalWithRoles(t, environment, workspace, "reviewer publisher",
		[]string{"reviewer", "publisher"})
	approveProposal(t, environment, workspace, reviewer, proposalID)

	authorPublish := publishProposal(t, environment, workspace, author, proposalID)
	if authorPublish.Code != http.StatusForbidden {
		t.Fatalf("author publish status = %d, want 403, body = %s", authorPublish.Code, authorPublish.Body.String())
	}
	assertJSONField(t, authorPublish.Body.Bytes(), "code", "SEPARATION_OF_DUTY")

	reviewerPublish := publishProposal(t, environment, workspace, reviewer, proposalID)
	if reviewerPublish.Code != http.StatusForbidden {
		t.Fatalf("sole reviewer publish status = %d, want 403, body = %s", reviewerPublish.Code, reviewerPublish.Body.String())
	}
	assertJSONField(t, reviewerPublish.Body.Bytes(), "code", "SEPARATION_OF_DUTY")
	assertTableCount(t, environment.pool, "releases", 0)
	if releaseDeniedAuditCount(t, environment, workspace) != 2 {
		t.Fatal("SoD refusals were not audited individually")
	}

	secondReviewer := createReviewerPrincipal(t, environment, workspace, "second reviewer")
	approveProposal(t, environment, workspace, secondReviewer, proposalID)
	published := publishProposal(t, environment, workspace, reviewer, proposalID)
	if published.Code != http.StatusCreated {
		t.Fatalf("publish with two approvals status = %d, body = %s", published.Code, published.Body.String())
	}
}

func TestPublishAssetProposalAppendsRevisionAndCutsRelease(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "release-publish-asset")
	author := createPrincipalWithRoles(t, environment, workspace, "author", []string{"asset_owner"})
	publisher := createPrincipalWithRoles(t, environment, workspace, "publisher", []string{"publisher"})
	reviewer := createReviewerPrincipal(t, environment, workspace, "reviewer")
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	_, _, proposalID := proposeToInReview(t, environment, workspace, author, assetID, baseRevisionID)
	approveProposal(t, environment, workspace, reviewer, proposalID)

	published := publishProposal(t, environment, workspace, publisher, proposalID)
	if published.Code != http.StatusCreated {
		t.Fatalf("publish status = %d, body = %s", published.Code, published.Body.String())
	}
	assertProposalState(t, environment, workspace, environment.proposalsPath(t, workspace), proposalID, "released")

	var release struct {
		ID             string `json:"id"`
		Sequence       int64  `json:"sequence"`
		State          string `json:"state"`
		ManifestDigest string `json:"manifestDigest"`
		OriginProposal string `json:"originProposalId"`
		Manifest       struct {
			Assets []struct {
				AssetID       string          `json:"assetId"`
				RevisionID    string          `json:"revisionId"`
				Position      int             `json:"position"`
				Compatibility json.RawMessage `json:"compatibility"`
			} `json:"assets"`
		} `json:"manifest"`
	}
	if err := json.Unmarshal(published.Body.Bytes(), &release); err != nil {
		t.Fatalf("decode release detail: %v\n%s", err, published.Body.String())
	}
	if !strings.HasPrefix(release.ID, "rls_") {
		t.Fatalf("release id = %q, want rls_ TypeID", release.ID)
	}
	if release.Sequence != 1 || release.State != "published" {
		t.Fatalf("release = %+v", release)
	}
	if release.OriginProposal != proposalID {
		t.Fatalf("origin proposal = %q, want %q", release.OriginProposal, proposalID)
	}
	if len(release.Manifest.Assets) != 1 {
		t.Fatalf("manifest assets = %v", release.Manifest.Assets)
	}
	pinned := release.Manifest.Assets[0]
	if pinned.AssetID != assetID.String() || pinned.RevisionID == baseRevisionID.String() {
		t.Fatalf("pinned entry = %+v, base %s", pinned, baseRevisionID)
	}

	detail, err := environment.store.GetCatalogAsset(context.Background(), workspace, assetID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.CurrentRevision == nil || detail.CurrentRevision.ID.String() != pinned.RevisionID {
		t.Fatal("publish did not switch the current revision to the pinned revision")
	}
	if detail.CurrentRevision.Sequence != 2 {
		t.Fatalf("published revision sequence = %d, want 2", detail.CurrentRevision.Sequence)
	}
	var content map[string]any
	if err := json.Unmarshal(detail.CurrentRevision.Content, &content); err != nil {
		t.Fatal(err)
	}
	if content["definition"] != "Revenue after refunds and chargebacks" {
		t.Fatalf("published content = %s", detail.CurrentRevision.Content)
	}

	entries := []governance.ManifestEntry{{
		AssetID: assetID, RevisionID: detail.CurrentRevision.ID,
		Compatibility: pinned.Compatibility, Position: pinned.Position,
	}}
	payload, err := governance.ManifestDigestPayload(entries)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := governance.DigestJSON(payload)
	if err != nil {
		t.Fatal(err)
	}
	if digest != release.ManifestDigest {
		t.Fatalf("manifest digest = %s, want %s", release.ManifestDigest, digest)
	}

	var auditFacts, outboxRows int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'governance.release.published'`,
		workspace.UUID()).Scan(&auditFacts); err != nil {
		t.Fatal(err)
	}
	if auditFacts != 1 {
		t.Fatalf("release audit facts = %d", auditFacts)
	}
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM outbox_events
		WHERE workspace_id = $1 AND event_type = 'release.published'`,
		workspace.UUID()).Scan(&outboxRows); err != nil {
		t.Fatal(err)
	}
	if outboxRows != 1 {
		t.Fatalf("release.published outbox rows = %d, want 1", outboxRows)
	}
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM outbox_events
		WHERE workspace_id = $1 AND event_type = 'catalog.asset.changed'
		  AND payload->'data'->>'action' = 'revision.created'`,
		workspace.UUID()).Scan(&outboxRows); err != nil {
		t.Fatal(err)
	}
	if outboxRows != 1 {
		t.Fatalf("catalog.asset.changed revision events = %d, want 1", outboxRows)
	}
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM outbox_events
		WHERE workspace_id = $1 AND event_type = 'proposal.changed'
		  AND payload->'data'->>'action' = 'released'`,
		workspace.UUID()).Scan(&outboxRows); err != nil {
		t.Fatal(err)
	}
	if outboxRows != 1 {
		t.Fatalf("released proposal.changed outbox rows = %d, want 1 (payload sample: %v)",
			outboxRows, environment.outboxSamples(t, workspace, "proposal.changed"))
	}
	if strings.Contains(published.Body.String(), assetID.UUID()) {
		t.Fatal("release detail leaked a storage UUID")
	}
}

func objectProposalBody(contractID, author string) string {
	return fmt.Sprintf(
		`{"targetObjectType":"join_contract","targetObjectId":%q,"title":"Annotate join contract","reason":"Consumer ask","createdBy":%q,"changeSet":[{"fieldPath":"notes","op":"add","afterDigest":%q,"afterValue":"Verified against warehouse v2"}]}`,
		contractID, author, sha256Of(`"Verified against warehouse v2"`))
}

func objectProposalToInReview(
	t *testing.T, environment *fixture, workspace identity.WorkspaceID,
	author identity.PrincipalID, reviewer identity.PrincipalID, contractID string,
) string {
	t.Helper()
	path := environment.proposalsPath(t, workspace)
	created := environment.request(t, http.MethodPost, path, author.String(), objectProposalBody(contractID, author.String()))
	if created.Code != http.StatusCreated {
		t.Fatalf("object proposal create status = %d, body = %s", created.Code, created.Body.String())
	}
	proposalID, _ := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
	submitted := environment.request(t, http.MethodPost, path+"/"+proposalID+"/submit", author.String(), "")
	if submitted.Code != http.StatusOK {
		t.Fatalf("object proposal submit status = %d", submitted.Code)
	}
	environment.runValidationWorker(t)
	assertProposalState(t, environment, workspace, path, proposalID, "in_review")
	approveProposal(t, environment, workspace, reviewer, proposalID)
	return proposalID
}

func TestPublishObjectProposalBumpsGovernedObjectVersion(t *testing.T) {
	environment := newFixture(t)
	workspace, contractID := seedJoinContract(t, environment, "release-publish-object")
	author := createPrincipalWithRoles(t, environment, workspace, "author", []string{"asset_owner"})
	publisher := createPrincipalWithRoles(t, environment, workspace, "publisher", []string{"publisher"})
	reviewer := createReviewerPrincipal(t, environment, workspace, "reviewer")

	objectService := governanceapp.NewGovernedObjectService(
		environment.store, governanceapp.ClockFunc(func() time.Time { return time.Now().UTC() }))
	before, err := objectService.Get(context.Background(), workspace, governance.TargetJoinContract, contractID.String())
	if err != nil {
		t.Fatal(err)
	}

	proposalID := objectProposalToInReview(t, environment, workspace, author, reviewer, contractID.String())
	published := publishProposal(t, environment, workspace, publisher, proposalID)
	if published.Code != http.StatusCreated {
		t.Fatalf("object publish status = %d, body = %s", published.Code, published.Body.String())
	}
	var release struct {
		Manifest struct {
			Objects []struct {
				ObjectType string `json:"objectType"`
				ObjectID   string `json:"objectId"`
				Version    int    `json:"version"`
			} `json:"objects"`
			Assets []json.RawMessage `json:"assets"`
		} `json:"manifest"`
	}
	if err := json.Unmarshal(published.Body.Bytes(), &release); err != nil {
		t.Fatalf("decode object release: %v\n%s", err, published.Body.String())
	}
	if len(release.Manifest.Objects) != 1 || len(release.Manifest.Assets) != 0 {
		t.Fatalf("object release manifest = %+v", release.Manifest)
	}
	pin := release.Manifest.Objects[0]
	if pin.ObjectType != "join_contract" || pin.ObjectID != contractID.String() || pin.Version != 2 {
		t.Fatalf("object pin = %+v", pin)
	}

	after, err := objectService.Get(context.Background(), workspace, governance.TargetJoinContract, contractID.String())
	if err != nil {
		t.Fatal(err)
	}
	if after.JoinContract.Version != 2 || before.JoinContract.Version != 1 {
		t.Fatalf("object versions before/after = %d/%d, want 1/2", before.JoinContract.Version, after.JoinContract.Version)
	}
	var content map[string]any
	if err := json.Unmarshal(after.JoinContract.Content, &content); err != nil {
		t.Fatal(err)
	}
	if content["notes"] != "Verified against warehouse v2" {
		t.Fatalf("object content = %s", after.JoinContract.Content)
	}
}

func TestRollbackRestoresPriorRevisionPointerAsNewRelease(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "release-rollback-asset")
	author := createPrincipalWithRoles(t, environment, workspace, "author", []string{"asset_owner"})
	publisher := createPrincipalWithRoles(t, environment, workspace, "publisher", []string{"publisher"})
	reviewer := createReviewerPrincipal(t, environment, workspace, "reviewer")
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	_, _, proposalID := proposeToInReview(t, environment, workspace, author, assetID, baseRevisionID)
	approveProposal(t, environment, workspace, reviewer, proposalID)
	published := publishProposal(t, environment, workspace, publisher, proposalID)
	if published.Code != http.StatusCreated {
		t.Fatalf("publish status = %d, body = %s", published.Code, published.Body.String())
	}
	var release struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(published.Body.Bytes(), &release); err != nil {
		t.Fatal(err)
	}

	rolledBack := rollbackRelease(t, environment, workspace, publisher, release.ID)
	if rolledBack.Code != http.StatusCreated {
		t.Fatalf("rollback status = %d, body = %s", rolledBack.Code, rolledBack.Body.String())
	}
	var rollback struct {
		ID             string `json:"id"`
		Sequence       int64  `json:"sequence"`
		RolledBackTo   string `json:"rolledBackToReleaseId"`
		OriginProposal string `json:"originProposalId"`
		Manifest       struct {
			Assets []struct {
				AssetID    string `json:"assetId"`
				RevisionID string `json:"revisionId"`
			} `json:"assets"`
		} `json:"manifest"`
	}
	if err := json.Unmarshal(rolledBack.Body.Bytes(), &rollback); err != nil {
		t.Fatalf("decode rollback release: %v\n%s", err, rolledBack.Body.String())
	}
	if rollback.Sequence != 2 || rollback.RolledBackTo != release.ID || rollback.OriginProposal != "" {
		t.Fatalf("rollback release = %+v", rollback)
	}
	if len(rollback.Manifest.Assets) != 1 || rollback.Manifest.Assets[0].RevisionID != baseRevisionID.String() {
		t.Fatalf("rollback manifest = %+v, want base revision pin", rollback.Manifest)
	}

	detail, err := environment.store.GetCatalogAsset(context.Background(), workspace, assetID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.CurrentRevision == nil || detail.CurrentRevision.ID.String() != baseRevisionID.String() {
		t.Fatal("rollback did not restore the prior revision pointer")
	}

	var targetState string
	targetTyped := mustID(t, func() (identity.ReleaseID, error) { return identity.ParseReleaseID(release.ID) })
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT state FROM releases WHERE workspace_id = $1 AND id = $2`,
		workspace.UUID(), targetTyped.UUID()).Scan(&targetState); err != nil {
		t.Fatal(err)
	}
	if targetState != "published" {
		t.Fatalf("target release state = %s after rollback", targetState)
	}

	second := rollbackRelease(t, environment, workspace, publisher, release.ID)
	if second.Code != http.StatusConflict {
		t.Fatalf("second rollback status = %d, want 409, body = %s", second.Code, second.Body.String())
	}
	assertJSONField(t, second.Body.Bytes(), "code", "RELEASE_ALREADY_ROLLED_BACK")

	var restoreEvents int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM outbox_events
		WHERE workspace_id = $1 AND event_type = 'catalog.asset.changed'
		  AND payload->'data'->>'action' = 'revision.created'`,
		workspace.UUID()).Scan(&restoreEvents); err != nil {
		t.Fatal(err)
	}
	if restoreEvents != 2 {
		t.Fatalf("catalog events after publish+rollback = %d, want 2", restoreEvents)
	}
	var rollbackAudit int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'governance.release.rolled_back'`,
		workspace.UUID()).Scan(&rollbackAudit); err != nil {
		t.Fatal(err)
	}
	if rollbackAudit != 1 {
		t.Fatalf("rollback audit facts = %d", rollbackAudit)
	}
}

func TestRollbackRestoresGovernedObjectContent(t *testing.T) {
	environment := newFixture(t)
	workspace, contractID := seedJoinContract(t, environment, "release-rollback-object")
	author := createPrincipalWithRoles(t, environment, workspace, "author", []string{"asset_owner"})
	publisher := createPrincipalWithRoles(t, environment, workspace, "publisher", []string{"publisher"})
	reviewer := createReviewerPrincipal(t, environment, workspace, "reviewer")

	proposalID := objectProposalToInReview(t, environment, workspace, author, reviewer, contractID.String())
	published := publishProposal(t, environment, workspace, publisher, proposalID)
	if published.Code != http.StatusCreated {
		t.Fatalf("object publish status = %d, body = %s", published.Code, published.Body.String())
	}
	var release struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(published.Body.Bytes(), &release); err != nil {
		t.Fatal(err)
	}

	rolledBack := rollbackRelease(t, environment, workspace, publisher, release.ID)
	if rolledBack.Code != http.StatusCreated {
		t.Fatalf("rollback status = %d, body = %s", rolledBack.Code, rolledBack.Body.String())
	}
	var rollback struct {
		Manifest struct {
			Objects []struct {
				ObjectID string `json:"objectId"`
				Version  int    `json:"version"`
			} `json:"objects"`
		} `json:"manifest"`
	}
	if err := json.Unmarshal(rolledBack.Body.Bytes(), &rollback); err != nil {
		t.Fatal(err)
	}
	if len(rollback.Manifest.Objects) != 1 || rollback.Manifest.Objects[0].Version != 3 {
		t.Fatalf("rollback object pin = %+v, want restored version 3", rollback.Manifest.Objects)
	}

	objectService := governanceapp.NewGovernedObjectService(
		environment.store, governanceapp.ClockFunc(func() time.Time { return time.Now().UTC() }))
	restored, err := objectService.Get(context.Background(), workspace, governance.TargetJoinContract, contractID.String())
	if err != nil {
		t.Fatal(err)
	}
	if restored.JoinContract.Version != 3 {
		t.Fatalf("restored version = %d, want 3", restored.JoinContract.Version)
	}
	var content map[string]any
	if err := json.Unmarshal(restored.JoinContract.Content, &content); err != nil {
		t.Fatal(err)
	}
	if _, exists := content["notes"]; exists {
		t.Fatalf("inverse patch did not remove the added field: %s", restored.JoinContract.Content)
	}
}

func TestRollbackRequiresCapabilityAndRespectsAuthorSoD(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "release-rollback-sod")
	author := createPrincipalWithRoles(t, environment, workspace, "author publisher",
		[]string{"asset_owner", "publisher"})
	publisher := createPrincipalWithRoles(t, environment, workspace, "publisher", []string{"publisher"})
	reviewer := createReviewerPrincipal(t, environment, workspace, "reviewer")
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	_, _, proposalID := proposeToInReview(t, environment, workspace, author, assetID, baseRevisionID)
	approveProposal(t, environment, workspace, reviewer, proposalID)
	published := publishProposal(t, environment, workspace, publisher, proposalID)
	if published.Code != http.StatusCreated {
		t.Fatalf("publish status = %d", published.Code)
	}
	var release struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(published.Body.Bytes(), &release); err != nil {
		t.Fatal(err)
	}

	unbound := createPrincipalWithRoles(t, environment, workspace, "steward", []string{"semantic_steward"})
	denied := rollbackRelease(t, environment, workspace, unbound, release.ID)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("rollback without capability status = %d, want 403", denied.Code)
	}
	assertJSONField(t, denied.Body.Bytes(), "code", "NO_MATCHING_GRANT")

	agent := createAgentPrincipal(t, environment, workspace, publisher, "rollback agent")
	agentDenied := rollbackRelease(t, environment, workspace, agent, release.ID)
	if agentDenied.Code != http.StatusForbidden {
		t.Fatalf("agent rollback status = %d, want 403, body = %s", agentDenied.Code, agentDenied.Body.String())
	}
	assertJSONField(t, agentDenied.Body.Bytes(), "code", "SEPARATION_OF_DUTY")

	authorDenied := rollbackRelease(t, environment, workspace, author, release.ID)
	if authorDenied.Code != http.StatusForbidden {
		t.Fatalf("author rollback status = %d, want 403, body = %s", authorDenied.Code, authorDenied.Body.String())
	}
	assertJSONField(t, authorDenied.Body.Bytes(), "code", "SEPARATION_OF_DUTY")
	assertTableCount(t, environment.pool, "releases", 1)
}

func TestReleasesListAndDetailKeysetPagination(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "release-list")
	author := createPrincipalWithRoles(t, environment, workspace, "author", []string{"asset_owner"})
	publisher := createPrincipalWithRoles(t, environment, workspace, "publisher", []string{"publisher"})
	reviewer := createReviewerPrincipal(t, environment, workspace, "reviewer")

	firstAsset, firstBase := environment.createAssetWithAddress(t, workspace, "commerce.list_one")
	_, _, firstProposal := proposeToInReview(t, environment, workspace, author, firstAsset, firstBase)
	approveProposal(t, environment, workspace, reviewer, firstProposal)
	first := publishProposal(t, environment, workspace, publisher, firstProposal)
	if first.Code != http.StatusCreated {
		t.Fatalf("first publish status = %d", first.Code)
	}
	secondAsset, secondBase := environment.createAssetWithAddress(t, workspace, "commerce.list_two")
	_, _, secondProposal := proposeToInReview(t, environment, workspace, author, secondAsset, secondBase)
	approveProposal(t, environment, workspace, reviewer, secondProposal)
	if second := publishProposal(t, environment, workspace, publisher, secondProposal); second.Code != http.StatusCreated {
		t.Fatalf("second publish status = %d", second.Code)
	}

	firstPage := environment.request(t, http.MethodGet, releasesPath(t, workspace)+"?limit=1", publisher.String(), "")
	if firstPage.Code != http.StatusOK {
		t.Fatalf("release list status = %d, body = %s", firstPage.Code, firstPage.Body.String())
	}
	var page struct {
		Items []struct {
			ID       string `json:"id"`
			Sequence int64  `json:"sequence"`
		} `json:"items"`
		Page struct {
			NextCursor string `json:"nextCursor"`
		} `json:"page"`
	}
	if err := json.Unmarshal(firstPage.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Page.NextCursor == "" {
		t.Fatalf("first page = %+v", page)
	}
	secondPage := environment.request(t, http.MethodGet, releasesPath(t, workspace)+"?limit=1&cursor="+page.Page.NextCursor,
		publisher.String(), "")
	if secondPage.Code != http.StatusOK {
		t.Fatalf("second page status = %d", secondPage.Code)
	}
	var next struct {
		Items []struct {
			Sequence int64 `json:"sequence"`
		} `json:"items"`
		Page struct {
			NextCursor string `json:"nextCursor"`
		} `json:"page"`
	}
	if err := json.Unmarshal(secondPage.Body.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].Sequence == page.Items[0].Sequence {
		t.Fatalf("second page = %+v", next)
	}
	if next.Page.NextCursor != "" {
		t.Fatal("last page must not carry a cursor")
	}

	detail := environment.request(t, http.MethodGet, releasesPath(t, workspace)+"/"+page.Items[0].ID,
		publisher.String(), "")
	if detail.Code != http.StatusOK {
		t.Fatalf("release detail status = %d, body = %s", detail.Code, detail.Body.String())
	}
	if !strings.Contains(detail.Body.String(), page.Items[0].ID) {
		t.Fatal("release detail without the release id")
	}
}

func (environment *fixture) outboxSamples(t *testing.T, workspace identity.WorkspaceID, eventType string) []string {
	t.Helper()
	rows, err := environment.pool.Query(context.Background(), `
		SELECT payload FROM outbox_events
		WHERE workspace_id = $1 AND event_type = $2`,
		workspace.UUID(), eventType)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	samples := make([]string, 0, 4)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		samples = append(samples, payload)
	}
	return samples
}
