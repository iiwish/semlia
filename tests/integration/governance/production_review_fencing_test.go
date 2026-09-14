package governance_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/iiwish/semlia/internal/application/jobs"
)

func TestProductionPublishReauthorizesReviewerAndFencesNewAttempt(t *testing.T) {
	f, h, w, author := authoringLifecycleSetup(t)
	path, _, created := authoringLifecycleCreate(t, f, h, w, author)
	reviewer := authoringLifecyclePrincipal(t, f, w, "Fenced reviewer")
	publisher := authoringLifecyclePrincipal(t, f, w, "Fenced publisher")
	digest := productionValidateAndApprove(t, f, h, w, author, reviewer, created)
	op := created["operationId"].(string)
	base := path + "/" + op
	body := map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"], "validation": map[string]any{"attemptNo": 1, "validationDigest": digest}, "expectedHead": map[string]any{"presence": "absent"}}
	if _, err := f.pool.Exec(context.Background(), `UPDATE principals SET status='suspended' WHERE workspace_id=$1 AND id=$2`, w.UUID(), reviewer.UUID()); err != nil {
		t.Fatal(err)
	}
	productionRequestResult(t, h, http.MethodPost, base+"/publish", publisher, "revoked-review-publish", body, http.StatusUnprocessableEntity)
	if _, err := f.pool.Exec(context.Background(), `UPDATE principals SET status='active' WHERE workspace_id=$1 AND id=$2`, w.UUID(), reviewer.UUID()); err != nil {
		t.Fatal(err)
	}
	productionRequestResult(t, h, http.MethodPost, base+"/validations", author, "new-attempt", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"], "previousAttemptNo": 1, "reason": "rerun complete checks"}, http.StatusAccepted)
	productionRequestResult(t, h, http.MethodPost, base+"/publish", publisher, "stale-attempt-publish", body, http.StatusUnprocessableEntity)
	job := jobs.Job{WorkspaceID: w, Attempt: 1, MaxAttempts: 3}
	if err := f.pool.QueryRow(context.Background(), `SELECT payload,trace_id FROM jobs WHERE workspace_id=$1 AND job_type='governance.proposal.validate' AND payload->>'operationId'=$2 AND payload->>'attempt'='2'`, w.UUID(), op).Scan(&job.Payload, &job.TraceID); err != nil {
		t.Fatal(err)
	}
	if err := runProductionValidationJob(t, f, job); err != nil {
		t.Fatal(err)
	}
	var nextDigest string
	if err := f.pool.QueryRow(context.Background(), `SELECT validation_digest FROM production_validation_attempts WHERE workspace_id=$1 AND attempt_no=2`, w.UUID()).Scan(&nextDigest); err != nil {
		t.Fatal(err)
	}
	body["validation"] = map[string]any{"attemptNo": 2, "validationDigest": nextDigest}
	productionRequestResult(t, h, http.MethodPost, base+"/publish", publisher, "missing-new-review", body, http.StatusUnprocessableEntity)
	productionRequestResult(t, h, http.MethodPost, base+"/reviews", reviewer, "new-review", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"], "validation": body["validation"], "proposalIds": created["proposalIds"], "decision": "approve", "note": "reviewed new exact attempt"}, http.StatusCreated)
	published := productionRequestResult(t, h, http.MethodPost, base+"/publish", publisher, "fenced-publish", body, http.StatusCreated)
	if published["releaseId"] == "" {
		t.Fatal("missing release")
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE principals SET status='suspended' WHERE workspace_id=$1 AND id=$2`, w.UUID(), publisher.UUID()); err != nil {
		t.Fatal(err)
	}
	r := sendProdRequest(h, http.MethodPost, base+"/publish", publisher.String(), "fenced-publish", fmt.Sprintf(`{"expectedVersion":1,"setDigest":%q,"validation":{"attemptNo":2,"validationDigest":%q},"expectedHead":{"presence":"absent"}}`, created["setDigest"], nextDigest))
	if r.Code != http.StatusForbidden {
		t.Fatalf("suspended publisher replayed receipt: %d %s", r.Code, r.Body.String())
	}
}
