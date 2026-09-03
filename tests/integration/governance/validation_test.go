package governance_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func sha256Of(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validatedChangeItem(fieldPath, before, after string) string {
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		panic(err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(
		`{"fieldPath":%q,"op":"update","beforeDigest":%q,"afterDigest":%q,"beforeValue":%s,"afterValue":%s}`,
		fieldPath, sha256Of(string(beforeJSON)), sha256Of(string(afterJSON)), beforeJSON, afterJSON)
}

func validatedProposalBody(targetObjectType, targetObjectID, baseRevisionID, changeSet string) string {
	head := fmt.Sprintf(`{"targetObjectType":%q,"targetObjectId":%q,`, targetObjectType, targetObjectID)
	if baseRevisionID != "" {
		head += fmt.Sprintf(`"baseRevisionId":%q,`, baseRevisionID)
	}
	return head + `"title":"Tighten metric definition","summary":"Clarifies refunds","reason":"Audit finding","createdBy":"founder","changeSet":[` +
		changeSet + `]}`
}

func countValidationJobs(t *testing.T, environment *fixture, workspace identity.WorkspaceID) int {
	t.Helper()
	var count int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM jobs
		WHERE workspace_id = $1 AND job_type = $2`,
		workspace.UUID(), governanceapp.ValidationJobType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func assertProposalState(t *testing.T, environment *fixture, workspace identity.WorkspaceID, path, proposalID, want string) {
	t.Helper()
	detail := environment.request(t, http.MethodGet, path+"/"+proposalID, "", "")
	if detail.Code != http.StatusOK {
		t.Fatalf("proposal detail status = %d, body = %s", detail.Code, detail.Body.String())
	}
	assertJSONField(t, detail.Body.Bytes(), "state", want)
}

func TestSubmitEnqueuesExactlyOneValidationJobAndReachesInReview(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-validation-clean")
	assetID, baseRevisionID := environment.createAsset(t, workspace)
	path := environment.proposalsPath(t, workspace)

	changeSet := validatedChangeItem("definition", "Revenue after refunds", "Revenue after refunds and chargebacks")
	created := environment.request(t, http.MethodPost, path, "",
		validatedProposalBody("semantic_asset", assetID.String(), baseRevisionID.String(), changeSet))
	if created.Code != http.StatusCreated {
		t.Fatalf("proposal create status = %d, body = %s", created.Code, created.Body.String())
	}
	proposalID, _ := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)

	submitted := environment.request(t, http.MethodPost, path+"/"+proposalID+"/submit", "", "")
	if submitted.Code != http.StatusOK {
		t.Fatalf("proposal submit status = %d, body = %s", submitted.Code, submitted.Body.String())
	}
	assertJSONField(t, submitted.Body.Bytes(), "state", "validating")

	if jobs := countValidationJobs(t, environment, workspace); jobs != 1 {
		t.Fatalf("enqueued validation jobs = %d, want exactly one", jobs)
	}
	var idempotencyKey, jobStatus, payload string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT idempotency_key, status, payload::text FROM jobs
		WHERE workspace_id = $1 AND job_type = $2`,
		workspace.UUID(), governanceapp.ValidationJobType).Scan(&idempotencyKey, &jobStatus, &payload); err != nil {
		t.Fatal(err)
	}
	proposalID2, err := identity.ParseProposalID(proposalID)
	if err != nil {
		t.Fatal(err)
	}
	if want := governanceapp.ValidationJobIdempotencyKey(proposalID2, 1); idempotencyKey != want {
		t.Fatalf("idempotency key = %q, want %q", idempotencyKey, want)
	}
	if jobStatus != "queued" {
		t.Fatalf("job status = %q, want queued", jobStatus)
	}
	if !strings.Contains(payload, proposalID) || !json.Valid([]byte(payload)) {
		t.Fatalf("job payload = %s, want the proposal TypeID", payload)
	}

	environment.runValidationWorker(t)

	assertProposalState(t, environment, workspace, path, proposalID, "in_review")
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT status FROM jobs WHERE workspace_id = $1 AND job_type = $2`,
		workspace.UUID(), governanceapp.ValidationJobType).Scan(&jobStatus); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "succeeded" {
		t.Fatalf("job status after run = %q, want succeeded", jobStatus)
	}

	runs := environment.request(t, http.MethodGet, path+"/"+proposalID+"/validation-runs", "", "")
	if runs.Code != http.StatusOK {
		t.Fatalf("validation-runs status = %d, body = %s", runs.Code, runs.Body.String())
	}
	var page struct {
		Items []struct {
			ValidatorID      string `json:"validatorId"`
			ValidatorVersion string `json:"validatorVersion"`
			Status           string `json:"status"`
			Results          []struct {
				Severity    string `json:"severity"`
				Code        string `json:"code"`
				InputDigest string `json:"inputDigest"`
			} `json:"results"`
		} `json:"items"`
	}
	if err := json.Unmarshal(runs.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode validation runs: %v\n%s", err, runs.Body.String())
	}
	if len(page.Items) != 3 {
		t.Fatalf("validation runs = %d, want one per v1 validator", len(page.Items))
	}
	seen := map[string]bool{}
	for _, run := range page.Items {
		seen[run.ValidatorID] = true
		if run.ValidatorVersion != "1.0.0" {
			t.Fatalf("validator %s version = %q", run.ValidatorID, run.ValidatorVersion)
		}
		if run.Status != string(governance.ValidationSucceeded) {
			t.Fatalf("validator %s run status = %q, want succeeded", run.ValidatorID, run.Status)
		}
		if len(run.Results) == 0 {
			t.Fatalf("validator %s run without recorded results", run.ValidatorID)
		}
		for _, result := range run.Results {
			if result.Severity == string(governance.SeverityBlocker) {
				t.Fatalf("clean run recorded a blocker: %+v", result)
			}
			if !governance.IsValidContentDigest(result.InputDigest) {
				t.Fatalf("result %s carries malformed input digest %q", result.Code, result.InputDigest)
			}
		}
	}
	for _, want := range []string{"schema", "reference", "structural"} {
		if !seen[want] {
			t.Fatalf("missing %s run in %v", want, seen)
		}
	}
	if strings.Contains(runs.Body.String(), assetID.UUID()) {
		t.Fatal("validation-runs leaked the storage UUID of the asset")
	}
}

func TestValidationBlockerDegradesToHumanHandlingNeverAutoRejects(t *testing.T) {
	// Packet red scenario: a blocker finding records the failure outcome and
	// the proposal still reaches in_review — never silent pass, never
	// auto-rejection. Two workspaces carrying identical proposal inputs must
	// additionally produce identical findings (determinism over real data).
	environment := newFixture(t)
	// The same minted-but-absent asset reference goes into both workspaces so
	// the two proposals carry truly identical change-set inputs.
	missingAsset := mustID(t, identity.NewAssetID)
	runBlockerCase := func(slug string) (proposalID string, findings []map[string]any) {
		workspace := createWorkspace(t, environment.pool, slug)
		assetID, baseRevisionID := environment.createAsset(t, workspace)
		path := environment.proposalsPath(t, workspace)
		changeSet := validatedChangeItem("content.relatedAsset", "", missingAsset.String())
		created := environment.request(t, http.MethodPost, path, "",
			validatedProposalBody("semantic_asset", assetID.String(), baseRevisionID.String(), changeSet))
		if created.Code != http.StatusCreated {
			t.Fatalf("%s proposal create status = %d, body = %s", slug, created.Code, created.Body.String())
		}
		proposalID, _ = decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
		submitted := environment.request(t, http.MethodPost, path+"/"+proposalID+"/submit", "", "")
		if submitted.Code != http.StatusOK {
			t.Fatalf("%s proposal submit status = %d, body = %s", slug, submitted.Code, submitted.Body.String())
		}
		environment.runValidationWorker(t)
		assertProposalState(t, environment, workspace, path, proposalID, "in_review")

		runs := environment.request(t, http.MethodGet, path+"/"+proposalID+"/validation-runs", "", "")
		if runs.Code != http.StatusOK {
			t.Fatalf("%s validation-runs status = %d", slug, runs.Code)
		}
		var page struct {
			Items []struct {
				ValidatorID string           `json:"validatorId"`
				Status      string           `json:"status"`
				Results     []map[string]any `json:"results"`
			} `json:"items"`
		}
		if err := json.Unmarshal(runs.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode validation runs: %v", err)
		}
		var blockerStatus string
		for _, run := range page.Items {
			for _, result := range run.Results {
				if result["severity"] == string(governance.SeverityBlocker) {
					blockerStatus = run.Status
					findings = append(findings, result)
				}
			}
		}
		if len(findings) == 0 {
			t.Fatalf("%s produced no blocker finding:\n%s", slug, runs.Body.String())
		}
		if blockerStatus != string(governance.ValidationFailed) {
			t.Fatalf("%s run with blockers finished as %q, want failed", slug, blockerStatus)
		}
		return proposalID, findings
	}

	_, firstFindings := runBlockerCase("governance-validation-blocker-a")
	_, secondFindings := runBlockerCase("governance-validation-blocker-b")
	if len(firstFindings) != len(secondFindings) {
		t.Fatalf("identical inputs produced different finding counts: %d vs %d",
			len(firstFindings), len(secondFindings))
	}
	for i, finding := range firstFindings {
		other := secondFindings[i]
		for _, key := range []string{"severity", "code", "message"} {
			if finding[key] != other[key] {
				t.Fatalf("identical inputs diverged at finding %d key %s: %v vs %v", i, key, finding[key], other[key])
			}
		}
	}
}

func TestValidationRunsReadSurfaceRejectsUnknownProposal(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "governance-validation-404")
	path := environment.proposalsPath(t, workspace)
	unknown := mustID(t, identity.NewProposalID)
	response := environment.request(t, http.MethodGet, path+"/"+unknown.String()+"/validation-runs", "", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown proposal validation-runs status = %d, body = %s", response.Code, response.Body.String())
	}
	assertJSONField(t, response.Body.Bytes(), "code", "NOT_FOUND")
}
