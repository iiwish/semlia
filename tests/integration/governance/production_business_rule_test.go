package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	authapp "github.com/iiwish/semlia/internal/application/authorization"
	app "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/jobs"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	gov "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func runBusinessRuleAttempt(t *testing.T, f *fixture, w identity.WorkspaceID, op identity.ProductionOperationID, version, attempt int) string {
	t.Helper()
	job := jobs.Job{WorkspaceID: w, Attempt: 1, MaxAttempts: 3}
	if err := f.pool.QueryRow(context.Background(), `SELECT payload,trace_id FROM jobs WHERE workspace_id=$1 AND job_type='governance.proposal.validate' AND payload->>'operationId'=$2 AND payload->>'attempt'=$3`, w.UUID(), op.String(), fmt.Sprint(attempt)).Scan(&job.Payload, &job.TraceID); err != nil {
		t.Fatal(err)
	}
	if err := runProductionValidationJob(t, f, job); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM production_validation_attempts WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3 AND attempt_no=$4`, w.UUID(), op.UUID(), version, attempt).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func ruleCommandFor(t *testing.T, f *fixture, w identity.WorkspaceID, created map[string]any) map[string]any {
	t.Helper()
	op := mustParseProductionOperationID(t, created["operationId"].(string))
	_, ver, targets, _, _, err := f.store.GetProductionOperation(context.Background(), w, op)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{"expectedVersion": ver.Version, "setDigest": ver.SetDigest, "targetKey": targets[0].LocalKey, "action": "confirm", "evidenceId": targets[0].Declaration.EvidenceIDs[0]}
}

func TestProductionBusinessRuleHumanDeclarationPublishesWithoutSeededEvidence(t *testing.T) {
	f, h, w, author := authoringLifecycleSetup(t)
	input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
	body := productionPayloadInput(t, authoringLifecycleBody(`{}`), input, author)
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	r := sendProdRequest(h, http.MethodPost, path, author.String(), "declare-draft", body)
	if r.Code != http.StatusCreated {
		t.Fatalf("draft: %d %s", r.Code, r.Body.String())
	}
	created := authoringLifecycleDecode(t, r.Body.Bytes())
	op := mustParseProductionOperationID(t, created["operationId"].(string))
	base := path + "/" + op.String()
	_, ver, targets, _, _, err := f.store.GetProductionOperation(context.Background(), w, op)
	if err != nil {
		t.Fatal(err)
	}
	assertTableCount(t, f.pool, "evidence_artifacts", 0)
	confirmed := productionRequestResult(t, h, http.MethodPost, base+"/business-rule-confirmations", author, "declare-rule", map[string]any{"expectedVersion": 1, "setDigest": ver.SetDigest, "targetKey": targets[0].LocalKey, "action": "confirm", "declaration": "As the authorized synthetic business owner, I declare Net revenue and the stated fixture scope to be the business rule for this test."}, http.StatusCreated)
	if confirmed["evidenceOrigin"] != "human_declaration" || confirmed["principalId"] != author.String() {
		t.Fatalf("declaration has no human provenance: %v", confirmed)
	}
	read := productionRequestResult(t, h, http.MethodGet, base+"/business-rule-confirmations?version=1", author, "", nil, http.StatusOK)
	if declaration, ok := read["items"].([]any)[0].(map[string]any)["declaration"].(string); !ok || !strings.Contains(declaration, "synthetic business owner") {
		t.Fatalf("declaration not reviewable: %v", read)
	}
	assertTableCount(t, f.pool, "evidence_artifacts", 1)
	productionRequestResult(t, h, http.MethodPost, base+"/submit", author, "submit-declared-rule", map[string]any{"expectedVersion": 1, "setDigest": ver.SetDigest}, http.StatusAccepted)
	if status := runBusinessRuleAttempt(t, f, w, op, 1, 1); status != "succeeded" {
		t.Fatal(status)
	}
	attempt, err := f.store.GetValidationAttempt(context.Background(), w, op, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := authoringLifecyclePrincipal(t, f, w, "Declaration reviewer")
	publisher := authoringLifecyclePrincipal(t, f, w, "Declaration publisher")
	validation := map[string]any{"attemptNo": 1, "validationDigest": *attempt.ValidationDigest}
	productionRequestResult(t, h, http.MethodPost, base+"/reviews", reviewer, "review-declaration", map[string]any{"expectedVersion": 1, "setDigest": ver.SetDigest, "validation": validation, "proposalIds": created["proposalIds"], "decision": "approve", "note": "reviewed the synthetic human business declaration"}, http.StatusCreated)
	productionRequestResult(t, h, http.MethodPost, base+"/publish", publisher, "publish-declaration", map[string]any{"expectedVersion": 1, "setDigest": ver.SetDigest, "validation": validation, "expectedHead": map[string]any{"presence": "absent"}}, http.StatusCreated)
}

func TestProductionBusinessRuleEvidenceIsNotConfirmation(t *testing.T) {
	for _, kind := range []string{"declared", "observed", "inferred"} {
		t.Run(kind, func(t *testing.T) {
			f, h, w, p := authoringLifecycleSetup(t)
			input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
			// Deliberately forged metadata must not become a server confirmation.
			metadata := json.RawMessage(`{"businessRule":"Revenue excludes refunds","attestation":{"confirmed":true,"actor":"human"}}`)
			digest, _ := gov.DigestJSON(metadata)
			evidence, err := f.store.CreateEvidence(context.Background(), semantic.EvidenceArtifact{ID: mustID(t, identity.NewEvidenceID), WorkspaceID: w, EvidenceType: kind, Locator: "fixture://forged-rule", ContentDigest: digest, Metadata: metadata})
			if err != nil {
				t.Fatal(err)
			}
			input["evidence"] = []any{map[string]any{"evidenceId": evidence.ID.String(), "digest": digest}}
			body := productionPayloadInput(t, authoringLifecycleBody(`{}`), input, p)
			path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
			created := authoringLifecycleDecode(t, sendProdRequest(h, http.MethodPost, path, p.String(), "rule-draft", body).Body.Bytes())
			op := mustParseProductionOperationID(t, created["operationId"].(string))
			base := path + "/" + op.String()
			if kind != "declared" {
				productionRequestResult(t, h, http.MethodPost, base+"/business-rule-confirmations", p, "cannot-confirm-observation", ruleCommandFor(t, f, w, created), http.StatusUnprocessableEntity)
			}
			productionRequestResult(t, h, http.MethodPost, base+"/submit", p, "rule-submit", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"]}, http.StatusAccepted)
			if status := runBusinessRuleAttempt(t, f, w, op, 1, 1); status != "failed" {
				t.Fatalf("%s metadata passed as human rule: %s", kind, status)
			}
			var raw []byte
			if err := f.pool.QueryRow(context.Background(), `SELECT results_json FROM production_validation_seals WHERE operation_id=$1`, op.UUID()).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), "PRODUCTION_BUSINESS_RULE_UNCONFIRMED") {
				t.Fatalf("missing rule blocker: %s", raw)
			}
			assertTableCount(t, f.pool, "production_business_rule_events", 0)
		})
	}
}

func TestProductionBusinessRuleConfirmationAuthorizationAndReplay(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	path, _, created := authoringLifecycleCreate(t, f, h, w, p)
	base := path + "/" + created["operationId"].(string)
	endpoint := base + "/business-rule-confirmations"
	body := ruleCommandFor(t, f, w, created)
	observer := createPrincipalWithRoles(t, f, w, "Rule observer", []string{"consumer_developer"})
	productionRequestResult(t, h, http.MethodPost, endpoint, observer, "observer-confirm", body, http.StatusForbidden)
	agent, err := f.store.WorkspaceAgentPrincipal(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"semantic_steward", "source_operator"} {
		if _, err := f.store.CreateRoleBinding(context.Background(), authz.RoleBinding{ID: mustID(t, identity.NewBindingID), PrincipalID: agent.ID, RoleID: role, ScopeType: authz.ScopeWorkspace, ScopeID: w.UUID()}); err != nil {
			t.Fatal(err)
		}
	}
	productionRequestResult(t, h, http.MethodPost, endpoint, agent.ID, "agent-confirm", body, http.StatusForbidden)
	forged := map[string]any{}
	for k, v := range body {
		forged[k] = v
	}
	forged["principalId"] = p.String()
	productionRequestResult(t, h, http.MethodPost, endpoint, p, "forged-confirm", forged, http.StatusBadRequest)
	confirmed := productionRequestResult(t, h, http.MethodPost, endpoint, p, "human-confirm", body, http.StatusCreated)
	replay := productionRequestResult(t, h, http.MethodPost, endpoint, p, "human-confirm", body, http.StatusOK)
	if replay["sequence"] != confirmed["sequence"] || replay["replayed"] != true || confirmed["principalId"] != p.String() {
		t.Fatalf("unstable actor-bound receipt: %v %v", confirmed, replay)
	}
	changed := map[string]any{}
	for k, v := range body {
		changed[k] = v
	}
	changed["action"] = "revoke"
	delete(changed, "evidenceId")
	productionRequestResult(t, h, http.MethodPost, endpoint, p, "human-confirm", changed, http.StatusConflict)
	productionRequestResult(t, h, http.MethodGet, endpoint+"?version=1", p, "", nil, http.StatusOK)
	if _, err := f.pool.Exec(context.Background(), `UPDATE principals SET status='suspended' WHERE id=$1`, p.UUID()); err != nil {
		t.Fatal(err)
	}
	productionRequestResult(t, h, http.MethodPost, endpoint, p, "human-confirm", body, http.StatusForbidden)
	assertTableCount(t, f.pool, "production_business_rule_events", 1)
	down, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000028_production_business_rules.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), string(down)); err == nil || !strings.Contains(err.Error(), "DOWN_MIGRATION_UNSAFE") {
		t.Fatalf("unsafe business rule downgrade: %v", err)
	}
	assertTableCount(t, f.pool, "production_business_rule_events", 1)
	for _, sql := range []string{`UPDATE production_business_rule_events SET content_digest=request_digest`, `DELETE FROM production_business_rule_events`} {
		if _, err := f.pool.Exec(context.Background(), sql); err == nil {
			t.Fatalf("mutable confirmation: %s", sql)
		}
	}
}

func TestProductionBusinessRuleRevokedGrantCannotReplayOrPublish(t *testing.T) {
	f, h, w, author := authoringLifecycleSetup(t)
	path, _, created := authoringLifecycleCreate(t, f, h, w, author)
	confirmer := mustID(t, identity.NewPrincipalID)
	if _, err := f.store.CreatePrincipal(context.Background(), authz.Principal{ID: confirmer, WorkspaceID: w, Kind: authz.PrincipalHuman, DisplayName: "Scoped business confirmer", Status: authz.PrincipalActive}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CreateRoleBinding(context.Background(), authz.RoleBinding{ID: mustID(t, identity.NewBindingID), PrincipalID: confirmer, RoleID: "auditor", ScopeType: authz.ScopeWorkspace, ScopeID: w.UUID()}); err != nil {
		t.Fatal(err)
	}
	binding, err := f.store.CreateRoleBinding(context.Background(), authz.RoleBinding{ID: mustID(t, identity.NewBindingID), PrincipalID: confirmer, RoleID: "asset_owner", ScopeType: authz.ScopeDomain, ScopeID: "finance"})
	if err != nil {
		t.Fatal(err)
	}
	op := mustParseProductionOperationID(t, created["operationId"].(string))
	base := path + "/" + op.String()
	endpoint := base + "/business-rule-confirmations"
	body := ruleCommandFor(t, f, w, created)
	productionRequestResult(t, h, http.MethodPost, endpoint, confirmer, "scoped-confirm", body, http.StatusCreated)
	productionRequestResult(t, h, http.MethodPost, base+"/submit", author, "submit-scoped-rule", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"]}, http.StatusAccepted)
	if status := runBusinessRuleAttempt(t, f, w, op, 1, 1); status != "succeeded" {
		t.Fatal(status)
	}
	attempt, err := f.store.GetValidationAttempt(context.Background(), w, op, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	reviewer := authoringLifecyclePrincipal(t, f, w, "Grant reviewer")
	publisher := authoringLifecyclePrincipal(t, f, w, "Grant publisher")
	validation := map[string]any{"attemptNo": 1, "validationDigest": *attempt.ValidationDigest}
	productionRequestResult(t, h, http.MethodPost, base+"/reviews", reviewer, "review-scoped", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"], "validation": validation, "proposalIds": created["proposalIds"], "decision": "approve", "note": "independent rule review"}, http.StatusCreated)
	service := authapp.NewService(f.store, authapp.ClockFunc(time.Now))
	if _, _, err := service.RevokeBinding(context.Background(), authapp.RevokeRoleBindingRequest{AccessRequest: authapp.AccessRequest{WorkspaceID: w, PrincipalRef: author.String(), TraceID: traceID}, BindingID: binding.ID, ExpectedVersion: binding.Version, Reason: "withdraw business rule authority"}); err != nil {
		t.Fatal(err)
	}
	productionRequestResult(t, h, http.MethodPost, endpoint, confirmer, "scoped-confirm", body, http.StatusForbidden)
	productionRequestResult(t, h, http.MethodPost, base+"/publish", publisher, "publish-revoked-rule-grant", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"], "validation": validation, "expectedHead": map[string]any{"presence": "absent"}}, http.StatusUnprocessableEntity)
	assertTableCount(t, f.pool, "releases", 0)
}

func TestProductionBusinessRuleVersionDoesNotInheritConfirmation(t *testing.T) {
	for _, change := range []bool{false, true} {
		t.Run(fmt.Sprint(change), func(t *testing.T) {
			f, h, w, p := authoringLifecycleSetup(t)
			path, raw, created := authoringLifecycleCreate(t, f, h, w, p)
			confirmProductionFixtureRules(t, f, h, w, p, created)
			base := path + "/" + created["operationId"].(string)
			old := ruleCommandFor(t, f, w, created)
			var body map[string]any
			if err := json.Unmarshal([]byte(raw), &body); err != nil {
				t.Fatal(err)
			}
			body["expectedVersion"] = 1
			if change {
				body["targets"].([]any)[0].(map[string]any)["content"].(map[string]any)["definition"] = "A different synthetic rule"
			}
			updated := productionRequestResult(t, h, http.MethodPut, base, p, "rule-edit", body, http.StatusOK)
			productionRequestResult(t, h, http.MethodPost, base+"/business-rule-confirmations", p, "stale-rule", old, http.StatusConflict)
			productionRequestResult(t, h, http.MethodPost, base+"/submit", p, "rule-submit-v2", map[string]any{"expectedVersion": 2, "setDigest": updated["setDigest"]}, http.StatusAccepted)
			if status := runBusinessRuleAttempt(t, f, w, mustParseProductionOperationID(t, created["operationId"].(string)), 2, 1); status != "failed" {
				t.Fatalf("version inherited rule: %s", status)
			}
		})
	}
}

func TestProductionBusinessRuleConcurrentReplayAndAtomicFailure(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	path, _, created := authoringLifecycleCreate(t, f, h, w, p)
	endpoint := path + "/" + created["operationId"].(string) + "/business-rule-confirmations"
	actor := authoringLifecyclePrincipal(t, f, w, "Atomic business confirmer")
	body := ruleCommandFor(t, f, w, created)
	delete(body, "evidenceId")
	body["declaration"] = "I explicitly declare this synthetic definition and scope."
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION fail_business_rule_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected rule commit failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS fail_business_rule_test() CASCADE`)
	})
	if _, err := f.pool.Exec(context.Background(), `CREATE CONSTRAINT TRIGGER fail_business_rule_test AFTER INSERT ON production_business_rule_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_business_rule_test()`); err != nil {
		t.Fatal(err)
	}
	var beforeAudit, beforeOutbox int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM audit_events),(SELECT count(*) FROM outbox_events)`).Scan(&beforeAudit, &beforeOutbox); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	r := sendProdRequest(h, http.MethodPost, endpoint, actor.String(), "atomic-rule", string(raw))
	if r.Code < 500 {
		t.Fatalf("commit fault swallowed: %d %s", r.Code, r.Body.String())
	}
	assertTableCount(t, f.pool, "production_business_rule_events", 0)
	assertTableCount(t, f.pool, "evidence_artifacts", 1)
	assertTableCount(t, f.pool, "audit_events", beforeAudit)
	assertTableCount(t, f.pool, "outbox_events", beforeOutbox)
	var contributor bool
	if err := f.pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM production_contributors WHERE principal_id=$1)`, actor.UUID()).Scan(&contributor); err != nil || contributor {
		t.Fatalf("failed transaction left contributor: %v %v", contributor, err)
	}
	if _, err := f.pool.Exec(context.Background(), `DROP FUNCTION fail_business_rule_test() CASCADE`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := sendProdRequest(h, http.MethodPost, endpoint, actor.String(), "atomic-rule", string(raw))
			statuses <- r.Code
		}()
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[http.StatusCreated] != 1 || counts[http.StatusOK] != 1 {
		t.Fatalf("concurrent confirmation results: %v", counts)
	}
	assertTableCount(t, f.pool, "production_business_rule_events", 1)
	assertTableCount(t, f.pool, "evidence_artifacts", 2)
	assertTableCount(t, f.pool, "audit_events", beforeAudit+1)
	assertTableCount(t, f.pool, "outbox_events", beforeOutbox+1)
}

func TestProductionBusinessRuleRevocationFencesWorkerCompletion(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	path, _, created := authoringLifecycleCreate(t, f, h, w, p)
	confirmProductionFixtureRules(t, f, h, w, p, created)
	op := mustParseProductionOperationID(t, created["operationId"].(string))
	base := path + "/" + op.String()
	productionRequestResult(t, h, http.MethodPost, base+"/submit", p, "rule-submit", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"]}, http.StatusAccepted)
	work, err := f.store.LoadProductionValidationWork(context.Background(), w, op, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := app.EvaluateProductionValidation(work, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if completion.Attempt.Status != "succeeded" {
		t.Fatalf("positive work: %+v", completion.Checks)
	}
	body := ruleCommandFor(t, f, w, created)
	body["action"] = "revoke"
	delete(body, "evidenceId")
	productionRequestResult(t, h, http.MethodPost, base+"/business-rule-confirmations", p, "revoke-in-flight", body, http.StatusCreated)
	if err := f.store.CompleteProductionValidation(context.Background(), completion); err != nil {
		t.Fatal(err)
	}
	attempt, err := f.store.GetValidationAttempt(context.Background(), w, op, 1, 1)
	if err != nil || attempt.Status != "failed" {
		t.Fatalf("stale worker completion: %+v %v", attempt, err)
	}
	body["action"] = "confirm"
	body["evidenceId"] = ruleCommandFor(t, f, w, created)["evidenceId"]
	productionRequestResult(t, h, http.MethodPost, base+"/business-rule-confirmations", p, "reconfirm", body, http.StatusCreated)
	productionRequestResult(t, h, http.MethodPost, base+"/validations", p, "rerun-rule", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"], "previousAttemptNo": 1, "reason": "human reconfirmed rule"}, http.StatusAccepted)
	if status := runBusinessRuleAttempt(t, f, w, op, 1, 2); status != "succeeded" {
		t.Fatalf("reconfirmed rule failed: %s", status)
	}
}

func TestProductionBusinessRuleConfirmerCannotReviewOrPublish(t *testing.T) {
	f, h, w, author := authoringLifecycleSetup(t)
	path, _, created := authoringLifecycleCreate(t, f, h, w, author)
	confirmer := authoringLifecyclePrincipal(t, f, w, "Business confirmer")
	reviewer := authoringLifecyclePrincipal(t, f, w, "Rule reviewer")
	publisher := authoringLifecyclePrincipal(t, f, w, "Rule publisher")
	op := mustParseProductionOperationID(t, created["operationId"].(string))
	base := path + "/" + op.String()
	confirmProductionFixtureRules(t, f, h, w, confirmer, created)
	productionRequestResult(t, h, http.MethodPost, base+"/submit", author, "rule-submit", map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"]}, http.StatusAccepted)
	if status := runBusinessRuleAttempt(t, f, w, op, 1, 1); status != "succeeded" {
		t.Fatal(status)
	}
	attempt, err := f.store.GetValidationAttempt(context.Background(), w, op, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	review := map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"], "validation": map[string]any{"attemptNo": 1, "validationDigest": *attempt.ValidationDigest}, "proposalIds": created["proposalIds"], "decision": "approve", "note": "business rule reviewed"}
	productionRequestResult(t, h, http.MethodPost, base+"/reviews", confirmer, "self-review", review, http.StatusForbidden)
	productionRequestResult(t, h, http.MethodPost, base+"/reviews", reviewer, "independent-review", review, http.StatusCreated)
	publish := map[string]any{"expectedVersion": 1, "setDigest": created["setDigest"], "validation": review["validation"], "expectedHead": map[string]any{"presence": "absent"}}
	productionRequestResult(t, h, http.MethodPost, base+"/publish", confirmer, "self-publish", publish, http.StatusForbidden)
	if _, err := f.pool.Exec(context.Background(), `UPDATE principals SET status='suspended' WHERE id=$1`, confirmer.UUID()); err != nil {
		t.Fatal(err)
	}
	productionRequestResult(t, h, http.MethodPost, base+"/publish", publisher, "inactive-confirmer", publish, http.StatusUnprocessableEntity)
	if _, err := f.pool.Exec(context.Background(), `UPDATE principals SET status='active' WHERE id=$1`, confirmer.UUID()); err != nil {
		t.Fatal(err)
	}
	revoke := ruleCommandFor(t, f, w, created)
	revoke["action"] = "revoke"
	delete(revoke, "evidenceId")
	productionRequestResult(t, h, http.MethodPost, base+"/business-rule-confirmations", confirmer, "revoke-reviewed", revoke, http.StatusCreated)
	productionRequestResult(t, h, http.MethodPost, base+"/publish", publisher, "revoked-confirmation", publish, http.StatusUnprocessableEntity)
	assertTableCount(t, f.pool, "releases", 0)
}
