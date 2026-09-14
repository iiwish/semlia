package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/postgres"
	app "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/pkg/identity"
)

type failedProductionWorkLoader struct{ *postgres.Store }

func (s failedProductionWorkLoader) LoadProductionValidationWork(context.Context, identity.WorkspaceID, identity.ProductionOperationID, int, int) (app.ProductionValidationWork, error) {
	return app.ProductionValidationWork{}, errors.New("injected validation infrastructure failure")
}

func TestProductionValidationFinalFailureIsDurable(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	path, _, created := authoringLifecycleCreate(t, f, h, w, p)
	op := created["operationId"].(string)
	productionRequestResult(t, h, http.MethodPost, path+"/"+op+"/submit", p, "failure-submit", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"]}, http.StatusAccepted)
	job := jobs.Job{WorkspaceID: w, Attempt: 1, MaxAttempts: 3}
	if err := f.pool.QueryRow(context.Background(), `SELECT payload,trace_id FROM jobs WHERE workspace_id=$1 AND job_type='governance.proposal.validate'`, w.UUID()).Scan(&job.Payload, &job.TraceID); err != nil {
		t.Fatal(err)
	}
	clock := app.ClockFunc(time.Now)
	handler := app.NewValidationJobHandler(failedProductionWorkLoader{f.store}, app.NewProposalService(f.store, clock), app.NewValidationService(f.store, clock), app.NewDefaultRegistry(), app.NewPolicyService(f.store, clock, app.WithRuleSource(f.store)), clock)
	if err := handler.Handle(context.Background(), job); err == nil {
		t.Fatal("transport failure was swallowed")
	}
	var status string
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM production_validation_attempts WHERE workspace_id=$1`, w.UUID()).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("transport retry created semantic failure too early: %s", status)
	}
	job.Attempt = 3
	if err := handler.Handle(context.Background(), job); err == nil {
		t.Fatal("final transport failure was swallowed")
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM production_validation_attempts WHERE workspace_id=$1`, w.UUID()).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("final transport failure left semantic attempt %s", status)
	}
	page := productionRequestResult(t, h, http.MethodGet, path+"/"+op+"/validations?version=1", p, "", nil, http.StatusOK)
	attempt := page["items"].([]any)[0].(map[string]any)
	if attempt["validationDigest"] != nil || len(attempt["checks"].([]any)) != 4 {
		t.Fatalf("failed attempt presented incomplete/successful evidence: %v", attempt)
	}
}

func TestProductionValidationSubmitQueuesWithoutSyntheticSuccess(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	path, _, created := authoringLifecycleCreate(t, f, h, w, p)
	op := created["operationId"].(string)
	response := sendProdRequest(h, http.MethodPost, path+"/"+op+"/submit", p.String(), "queue-submit", fmt.Sprintf(`{"expectedVersion":1,"setDigest":%q}`, created["setDigest"]))
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit: %d %s", response.Code, response.Body.String())
	}
	var status string
	var completed, digest bool
	if err := f.pool.QueryRow(context.Background(), `SELECT status,completed_at IS NOT NULL,validation_digest IS NOT NULL FROM production_validation_attempts WHERE workspace_id=$1`, w.UUID()).Scan(&status, &completed, &digest); err != nil {
		t.Fatal(err)
	}
	if status != "queued" || completed || digest {
		t.Errorf("submit invented validation outcome: status=%s completed=%v digest=%v", status, completed, digest)
	}
	var jobs, premature int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM jobs WHERE workspace_id=$1 AND job_type='governance.proposal.validate' AND payload->>'operationId'=$2`, w.UUID(), op).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 {
		t.Errorf("expected one durable validation job, got %d", jobs)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM proposals WHERE workspace_id=$1 AND state<>'validating'`, w.UUID()).Scan(&premature); err != nil {
		t.Fatal(err)
	}
	if premature != 0 {
		t.Errorf("%d proposals advanced without validators", premature)
	}
}

func runProductionValidationJob(t *testing.T, f *fixture, job jobs.Job) error {
	t.Helper()
	clock := app.ClockFunc(time.Now)
	return app.NewValidationJobHandler(f.store, app.NewProposalService(f.store, clock), app.NewValidationService(f.store, clock), app.NewDefaultRegistry(), app.NewPolicyService(f.store, clock, app.WithRuleSource(f.store)), clock).Handle(context.Background(), job)
}

func TestProductionValidationWorkerRunsCompleteAttempt(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	path, _, created := authoringLifecycleCreate(t, f, h, w, p)
	op := created["operationId"].(string)
	confirmProductionFixtureRules(t, f, h, w, p, created)
	r := sendProdRequest(h, http.MethodPost, path+"/"+op+"/submit", p.String(), "worker-submit", fmt.Sprintf(`{"expectedVersion":1,"setDigest":%q}`, created["setDigest"]))
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
	var status string
	var runs, results, policies int
	if err := f.pool.QueryRow(context.Background(), `SELECT status,(SELECT count(*) FROM validation_runs WHERE workspace_id=$1),(SELECT count(*) FROM validation_results WHERE workspace_id=$1),(SELECT count(*) FROM policy_decisions WHERE workspace_id=$1) FROM production_validation_attempts WHERE workspace_id=$1`, w.UUID()).Scan(&status, &runs, &results, &policies); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || runs != 4 || results < 4 || policies != 1 {
		t.Fatalf("incomplete validation: status=%s runs=%d findings=%d policies=%d", status, runs, results, policies)
	}
	if err := runProductionValidationJob(t, f, job); err != nil {
		t.Fatalf("retry: %v", err)
	}
	reviewer := authoringLifecyclePrincipal(t, f, w, "Independent reviewer")
	var digest string
	if err := f.pool.QueryRow(context.Background(), `SELECT validation_digest FROM production_validation_attempts WHERE workspace_id=$1`, w.UUID()).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	proposal := created["proposalIds"].([]any)[0].(string)
	body := fmt.Sprintf(`{"expectedVersion":1,"setDigest":%q,"validation":{"attemptNo":1,"validationDigest":%q},"proposalIds":[%q],"decision":"approve","note":"reviewed"}`, created["setDigest"], digest, proposal)
	r = sendProdRequest(h, http.MethodPost, path+"/"+op+"/reviews", reviewer.String(), "worker-review", body)
	if r.Code != http.StatusCreated {
		t.Fatalf("review %d %s", r.Code, r.Body.String())
	}
	changed := strings.Replace(body, `"note":"reviewed"`, `"note":"different"`, 1)
	r = sendProdRequest(h, http.MethodPost, path+"/"+op+"/reviews", reviewer.String(), "worker-review", changed)
	if r.Code != http.StatusConflict {
		t.Fatalf("review changed-payload replay accepted: %d %s", r.Code, r.Body.String())
	}
	historyPath := path + "/" + op + "/validations?version=1&limit=1"
	reader := createPrincipalWithRoles(t, f, w, "Validation asset-only reader", []string{"consumer_developer"})
	r = sendProdRequest(h, http.MethodGet, historyPath, reader.String(), "", "")
	if r.Code != http.StatusForbidden {
		t.Fatalf("validation history leaked exact input to asset-only reader: %d %s", r.Code, r.Body.String())
	}
	for attempt := 2; attempt <= 3; attempt++ {
		request := fmt.Sprintf(`{"expectedVersion":1,"setDigest":%q,"previousAttemptNo":%d,"reason":"complete recheck"}`, created["setDigest"], attempt-1)
		r = sendProdRequest(h, http.MethodPost, path+"/"+op+"/validations", p.String(), fmt.Sprintf("history-attempt-%d", attempt), request)
		if r.Code != http.StatusAccepted {
			t.Fatalf("revalidate: %d %s", r.Code, r.Body.String())
		}
		if err := f.pool.QueryRow(context.Background(), `SELECT payload,trace_id FROM jobs WHERE workspace_id=$1 AND job_type='governance.proposal.validate' AND (payload->>'attempt')::int=$2`, w.UUID(), attempt).Scan(&job.Payload, &job.TraceID); err != nil {
			t.Fatal(err)
		}
		if err := runProductionValidationJob(t, f, job); err != nil {
			t.Fatal(err)
		}
		if attempt == 2 {
			page := productionRequestResult(t, h, http.MethodGet, historyPath, p, "", nil, http.StatusOK)
			items := page["items"].([]any)
			first := items[0].(map[string]any)
			if first["attemptNo"] != float64(2) || len(first["checks"].([]any)) != 4 || len(first["runIds"].([]any)) != 4 {
				t.Fatalf("incomplete history envelope: %v", page)
			}
			cursor, ok := page["nextCursor"].(string)
			if !ok || cursor == "" {
				t.Fatalf("missing history cursor: %v", page)
			}
			historyPath += "&cursor=" + url.QueryEscape(cursor)
		}
	}
	page := productionRequestResult(t, h, http.MethodGet, historyPath, p, "", nil, http.StatusOK)
	items := page["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["attemptNo"] != float64(1) || page["nextCursor"] != nil {
		t.Fatalf("history watermark repeated/skipped attempt: %v", page)
	}
	r = sendProdRequest(h, http.MethodGet, path+"/"+op+"/validations?version=1&cursor=bogus", p.String(), "", "")
	if r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "INVALID_CURSOR") {
		t.Fatalf("invalid cursor accepted: %d %s", r.Code, r.Body.String())
	}
	var finalPage map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &finalPage); err != nil {
		t.Fatal(err)
	}
}
