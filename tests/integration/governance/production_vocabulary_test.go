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
	"github.com/jackc/pgx/v5/pgtype"
)

func vocabularyProductionPayload(t *testing.T, input map[string]any, owner identity.PrincipalID, key, name string, vocabulary map[string]any, dependencies []any) map[string]any {
	t.Helper()
	content := map[string]any{"address": "finance." + key, "assetType": "metric", "displayName": name, "definition": name + " definition"}
	for field, value := range vocabulary {
		content[field] = value
	}
	inputCopy := map[string]any{}
	for field, value := range input {
		inputCopy[field] = value
	}
	inputCopy["dependencies"] = dependencies
	payload := map[string]any{"input": inputCopy, "targets": []any{map[string]any{"intent": "create", "kind": "semantic_asset", "localKey": key, "title": name, "content": content}}}
	completeProductionTargets(payload, owner)
	return payload
}

func TestProductionVocabularyConflictBlocksUntilConfusionIsDeclared(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	reviewer := authoringLifecyclePrincipal(t, f, w, "Vocabulary reviewer")
	publisher := authoringLifecyclePrincipal(t, f, w, "Vocabulary publisher")
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{}, true)
	ctx := context.Background()

	first := productionRequestResult(t, h, http.MethodPost, path, p, "vocabulary-first", vocabularyProductionPayload(t, input, p, "revenue", "有效支付金额", map[string]any{
		"aliases": []any{"GMV", "成交额"}, "excludes": []any{"已退款订单"},
		"sampleQuestions": []any{map[string]any{"question": "8月 GMV 是多少？"}},
	}, []any{}), http.StatusCreated)
	firstDigest := productionValidateAndApprove(t, f, h, w, p, reviewer, first)
	published := productionRequestResult(t, h, http.MethodPost, path+"/"+first["operationId"].(string)+"/publish", publisher, "vocabulary-first-publish", map[string]any{"expectedVersion": 1, "setDigest": first["setDigest"], "validation": map[string]any{"attemptNo": 1, "validationDigest": firstDigest}, "expectedHead": map[string]any{"presence": "absent"}}, http.StatusCreated)
	var aliases []byte
	var assetUUID, revisionUUID pgtype.UUID
	if err := f.pool.QueryRow(ctx, `SELECT a.id,r.id,r.content->'aliases' FROM semantic_assets a JOIN asset_revisions r ON r.workspace_id=a.workspace_id AND r.id=a.current_revision_id WHERE a.workspace_id=$1 AND a.key='revenue'`, w.UUID()).Scan(&assetUUID, &revisionUUID, &aliases); err != nil {
		t.Fatal(err)
	}
	if string(aliases) != `["GMV", "成交额"]` {
		t.Fatalf("published revision lost governed aliases: %s", aliases)
	}

	// A folded duplicate of a published alias names two metrics: validation blocks it before review.
	conflicting := productionRequestResult(t, h, http.MethodPost, path, p, "vocabulary-conflict", vocabularyProductionPayload(t, input, p, "booked", "下单金额", map[string]any{"aliases": []any{"ｇｍｖ"}}, []any{}), http.StatusCreated)
	confirmProductionFixtureRules(t, f, h, w, p, conflicting)
	op := conflicting["operationId"].(string)
	productionRequestResult(t, h, http.MethodPost, path+"/"+op+"/submit", p, "vocabulary-conflict-submit", map[string]any{"expectedVersion": 1, "setDigest": conflicting["setDigest"]}, http.StatusAccepted)
	job := jobs.Job{WorkspaceID: w, Attempt: 1, MaxAttempts: 3}
	if err := f.pool.QueryRow(ctx, `SELECT payload,trace_id FROM jobs WHERE workspace_id=$1 AND job_type='governance.proposal.validate' AND payload->>'operationId'=$2`, w.UUID(), op).Scan(&job.Payload, &job.TraceID); err != nil {
		t.Fatal(err)
	}
	if err := runProductionValidationJob(t, f, job); err != nil {
		t.Fatal(err)
	}
	var status string
	var results []byte
	if err := f.pool.QueryRow(ctx, `SELECT a.status,s.results_json FROM production_validation_attempts a JOIN production_validation_seals s ON s.workspace_id=a.workspace_id AND s.operation_id=a.operation_id AND s.production_version=a.production_version AND s.attempt_no=a.attempt_no WHERE a.workspace_id=$1 AND a.operation_id=$2 AND a.attempt_no=1`, w.UUID(), mustParseProductionOperationID(t, op).UUID()).Scan(&status, &results); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || !strings.Contains(string(results), "PRODUCTION_VOCABULARY_CONFLICT") || !strings.Contains(string(results), "「gmv」") {
		t.Fatalf("conflicting alias not blocked: %s %s", status, results)
	}
	if strings.Contains(string(results), "有效支付金额") {
		t.Fatalf("conflict finding disclosed the other asset: %s", results)
	}

	// Declaring the neighbour confusable, with how they differ, accepts the shared alias. The blocked operation keeps
	// its identity, so the accepted metric uses a fresh address.
	asset, err := identity.AssetIDFromUUIDBytes(assetUUID.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := identity.RevisionIDFromUUIDBytes(revisionUUID.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	ref := map[string]any{"assetId": asset.String(), "revisionId": revision.String(), "releaseId": published["releaseId"]}
	dependency := map[string]any{"kind": "semantic_asset", "targetId": asset.String(), "releaseId": published["releaseId"], "revisionId": revision.String()}
	declared := productionRequestResult(t, h, http.MethodPost, path, p, "vocabulary-declared", vocabularyProductionPayload(t, input, p, "ordered", "下单金额", map[string]any{
		"aliases":        []any{"ｇｍｖ"},
		"confusableWith": []any{map[string]any{"ref": ref, "distinction": "下单金额包含未支付和已退款订单"}},
	}, []any{dependency}), http.StatusCreated)
	declaredDigest := productionValidateAndApprove(t, f, h, w, p, reviewer, declared)
	productionRequestResult(t, h, http.MethodPost, path+"/"+declared["operationId"].(string)+"/publish", publisher, "vocabulary-declared-publish", map[string]any{"expectedVersion": 1, "setDigest": declared["setDigest"], "validation": map[string]any{"attemptNo": 1, "validationDigest": declaredDigest}, "expectedHead": map[string]any{"presence": "present", "releaseId": published["releaseId"], "manifestDigest": published["manifestDigest"]}}, http.StatusCreated)
	var confusable []byte
	if err := f.pool.QueryRow(ctx, `SELECT r.content->'confusableWith' FROM semantic_assets a JOIN asset_revisions r ON r.workspace_id=a.workspace_id AND r.id=a.current_revision_id WHERE a.workspace_id=$1 AND a.key='ordered'`, w.UUID()).Scan(&confusable); err != nil {
		t.Fatal(err)
	}
	var stored []struct {
		Ref struct {
			ReleaseID string `json:"releaseId"`
		} `json:"ref"`
	}
	if err := json.Unmarshal(confusable, &stored); err != nil || len(stored) != 1 || stored[0].Ref.ReleaseID != published["releaseId"] {
		t.Fatalf("confusable reference lost its pin: %s %v", confusable, err)
	}
}
