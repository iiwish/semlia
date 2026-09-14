package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestProductionPublishRequiresBoundApprovalAndBusinessContent(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	path, _, created := authoringLifecycleCreate(t, f, h, w, p)
	op := created["operationId"].(string)
	confirmProductionFixtureRules(t, f, h, w, p, created)
	r := sendProdRequest(h, http.MethodPost, path+"/"+op+"/submit", p.String(), "publish-submit", fmt.Sprintf(`{"expectedVersion":1,"setDigest":%q}`, created["setDigest"]))
	if r.Code != http.StatusAccepted {
		t.Fatalf("submit %d %s", r.Code, r.Body.String())
	}
	job := jobs.Job{WorkspaceID: w, Attempt: 1, MaxAttempts: 3}
	if err := f.pool.QueryRow(context.Background(), `SELECT payload,trace_id FROM jobs WHERE workspace_id=$1 AND job_type='governance.proposal.validate'`, w.UUID()).Scan(&job.Payload, &job.TraceID); err != nil {
		t.Fatal(err)
	}
	if err := runProductionValidationJob(t, f, job); err != nil {
		t.Fatal(err)
	}
	var digest string
	if err := f.pool.QueryRow(context.Background(), `SELECT validation_digest FROM production_validation_attempts WHERE workspace_id=$1`, w.UUID()).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	publisher := authoringLifecyclePrincipal(t, f, w, "Publisher")
	pubBody := fmt.Sprintf(`{"expectedVersion":1,"setDigest":%q,"validation":{"attemptNo":1,"validationDigest":%q},"expectedHead":{"presence":"absent"}}`, created["setDigest"], digest)
	r = sendProdRequest(h, http.MethodPost, path+"/"+op+"/publish", publisher.String(), "publish-without-review", pubBody)
	if r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unreviewed operation published: %d %s", r.Code, r.Body.String())
	}
	reviewer := authoringLifecyclePrincipal(t, f, w, "Reviewer")
	proposal := mustParseProposalID(t, created["proposalIds"].([]any)[0].(string))
	rawReview := mustID(t, identity.NewReviewID)
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO reviews(id,workspace_id,proposal_id,reviewer_principal_id,channel,decision,note) VALUES($1,$2,$3,$4,'expert','approved','legacy bypass')`, rawReview.UUID(), w.UUID(), proposal.UUID(), reviewer.UUID()); err == nil || !strings.Contains(err.Error(), "production review requires production binding") {
		t.Fatalf("raw legacy review bypass: %v", err)
	}
	rawRun := mustID(t, identity.NewValidationRunID)
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO validation_runs(id,workspace_id,proposal_id,validator_id,validator_version,status) VALUES($1,$2,$3,'legacy.fake','1.0.0','running')`, rawRun.UUID(), w.UUID(), proposal.UUID()); err == nil || !strings.Contains(err.Error(), "production run requires complete production binding") {
		t.Fatalf("raw legacy validation bypass: %v", err)
	}
	body := fmt.Sprintf(`{"expectedVersion":1,"setDigest":%q,"validation":{"attemptNo":1,"validationDigest":%q},"proposalIds":[%q],"decision":"approve","note":"approved"}`, created["setDigest"], digest, created["proposalIds"].([]any)[0])
	r = sendProdRequest(h, http.MethodPost, path+"/"+op+"/reviews", reviewer.String(), "publish-review", body)
	if r.Code != http.StatusCreated {
		t.Fatalf("review %d %s", r.Code, r.Body.String())
	}
	r = sendProdRequest(h, http.MethodPost, path+"/"+op+"/publish", reviewer.String(), "publish-reviewer", pubBody)
	if r.Code != http.StatusForbidden {
		t.Fatalf("reviewer published own adopted approval: %d %s", r.Code, r.Body.String())
	}
	r = sendProdRequest(h, http.MethodPost, path+"/"+op+"/publish", publisher.String(), "publish-approved", pubBody)
	if r.Code != http.StatusCreated {
		t.Fatalf("publish %d %s", r.Code, r.Body.String())
	}
	var definition string
	if err := f.pool.QueryRow(context.Background(), `SELECT r.content->>'definition' FROM semantic_assets a JOIN asset_revisions r ON r.workspace_id=a.workspace_id AND r.id=a.current_revision_id WHERE a.workspace_id=$1 AND a.key='revenue'`, w.UUID()).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if definition != "Net revenue" {
		t.Fatalf("published different content: %q", definition)
	}
	var published map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	detailPath := fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s", w, published["releaseId"])
	reader := createPrincipalWithRoles(t, f, w, "Asset-only release reader", []string{"consumer_developer"})
	denied := sendProdRequest(h, http.MethodGet, detailPath, reader.String(), "", "")
	if denied.Code != http.StatusForbidden {
		t.Fatalf("release detail leaked source-scoped attribution: %d %s", denied.Code, denied.Body.String())
	}
	detailResponse := sendProdRequest(h, http.MethodGet, detailPath, publisher.String(), "", "")
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("release detail: %d %s", detailResponse.Code, detailResponse.Body.String())
	}
	var detail map[string]any
	if err := json.Unmarshal(detailResponse.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail["afterManifest"].(map[string]any)["assets"].([]any)) != 1 {
		t.Fatalf("detail lost published manifest: %v", detail)
	}
	if detail["projectionStatus"] == "ready" {
		t.Fatal("release detail invented completed projection before outbox delivery")
	}
	rollbackPath := fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s/rollback", w, published["releaseId"])
	rollbackBody := fmt.Sprintf(`{"expectedVersion":1,"setDigest":%q,"expectedHead":{"presence":"present","releaseId":%q,"manifestDigest":%q},"reason":"restore prior state"}`, created["setDigest"], published["releaseId"], published["manifestDigest"])
	r = sendProdRequest(h, http.MethodPost, rollbackPath, publisher.String(), "rollback-first", rollbackBody)
	if r.Code != http.StatusCreated {
		t.Fatalf("rollback %d %s", r.Code, r.Body.String())
	}
	var rolled map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &rolled); err != nil {
		t.Fatal(err)
	}
	var absent bool
	if err := f.pool.QueryRow(context.Background(), `SELECT current_revision_id IS NULL FROM semantic_assets WHERE workspace_id=$1 AND key='revenue'`, w.UUID()).Scan(&absent); err != nil {
		t.Fatal(err)
	}
	if !absent {
		t.Fatal("first-create rollback retained a published asset pointer")
	}
	changedReason := fmt.Sprintf(`{"expectedVersion":1,"setDigest":%q,"expectedHead":{"presence":"present","releaseId":%q,"manifestDigest":%q},"reason":"different reason"}`, created["setDigest"], published["releaseId"], published["manifestDigest"])
	r = sendProdRequest(h, http.MethodPost, rollbackPath, publisher.String(), "rollback-first", changedReason)
	if r.Code != http.StatusConflict {
		t.Fatalf("rollback accepted changed request under same key: %d %s", r.Code, r.Body.String())
	}
	r = sendProdRequest(h, http.MethodPost, rollbackPath, publisher.String(), "rollback-first", rollbackBody)
	if r.Code != http.StatusOK {
		t.Fatalf("rollback replay %d %s", r.Code, r.Body.String())
	}
	var replayed map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &replayed); err != nil {
		t.Fatal(err)
	}
	if replayed["operationId"] != op || replayed["releaseId"] != rolled["releaseId"] {
		t.Fatalf("unstable rollback receipt: %v", replayed)
	}
	secondPath := fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s/rollback", w, rolled["releaseId"])
	secondBody := fmt.Sprintf(`{"expectedVersion":1,"setDigest":%q,"expectedHead":{"presence":"present","releaseId":%q,"manifestDigest":%q},"reason":"restore original publication"}`, created["setDigest"], rolled["releaseId"], rolled["manifestDigest"])
	r = sendProdRequest(h, http.MethodPost, secondPath, publisher.String(), "rollback-second", secondBody)
	if r.Code != http.StatusCreated {
		t.Fatalf("second rollback %d %s", r.Code, r.Body.String())
	}
	var second map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second["manifestDigest"] != published["manifestDigest"] {
		t.Fatalf("second rollback lost complete manifest: %v", second)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT r.content->>'definition' FROM semantic_assets a JOIN asset_revisions r ON r.workspace_id=a.workspace_id AND r.id=a.current_revision_id WHERE a.workspace_id=$1 AND a.key='revenue'`, w.UUID()).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if definition != "Net revenue" {
		t.Fatalf("second rollback restored wrong content: %q", definition)
	}
}
