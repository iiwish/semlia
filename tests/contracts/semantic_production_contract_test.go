package contracts_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// This validates a planned contract, not a running API or database invariant.
func TestSemanticProductionContract(t *testing.T) {
	base := filepath.Join(root, "docs", "specs", "semantic-production")
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(filepath.Join(base, "contracts", "production.openapi.yaml"))
	if err != nil {
		t.Fatalf("load planned semantic production contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate planned semantic production OpenAPI: %v", err)
	}
	for _, path := range []string{"data-model.md", "contracts/production.md"} {
		content, err := os.ReadFile(filepath.Join(base, path))
		if err != nil || len(content) < 1000 {
			t.Errorf("detailed companion %s is missing or incomplete: %v", path, err)
		}
	}
	if doc.OpenAPI != "3.0.3" || doc.Extensions["x-semlia-status"] != "planned" {
		t.Fatal("production design must remain an explicitly planned OpenAPI 3.0.3 contract")
	}

	t.Run("operations_and_authority", func(t *testing.T) {
		prefix := "/api/v1/workspaces/{workspaceId}"
		paths := map[string][]string{
			"/sources/{sourceId}/snapshots":                                    {"GET"},
			"/sources/{sourceId}/snapshots/{snapshotId}":                       {"GET"},
			"/sources/{sourceId}/snapshots/{snapshotId}/members":               {"GET"},
			"/sources/{sourceId}/snapshots/{snapshotId}/diagnostics":           {"GET"},
			"/production-operations":                                           {"GET", "POST"},
			"/production-operations/{operationId}":                             {"GET", "PUT"},
			"/production-operations/{operationId}/submit":                      {"POST"},
			"/production-operations/{operationId}/validations":                 {"GET", "POST"},
			"/production-operations/{operationId}/reviews":                     {"POST"},
			"/production-operations/{operationId}/publish":                     {"POST"},
			"/production-operations/{operationId}/generation":                  {"POST"},
			"/production-operations/{operationId}/generation/{runId}":          {"GET"},
			"/production-operations/{operationId}/business-rule-confirmations": {"GET", "POST"},
			"/production-releases/{releaseId}":                                 {"GET"},
			"/production-releases/{releaseId}/rollback":                        {"POST"},
		}
		seen := map[string]bool{}
		for path, methods := range paths {
			item := doc.Paths.Value(prefix + path)
			if item == nil {
				t.Errorf("missing path %s", path)
				continue
			}
			for _, method := range methods {
				op := item.GetOperation(method)
				if op == nil {
					t.Errorf("missing %s %s", method, path)
					continue
				}
				if op.OperationID == "" || seen[op.OperationID] {
					t.Errorf("missing or duplicate operation ID: %s", op.OperationID)
				}
				seen[op.OperationID] = true
				var actions []string
				productionDecode(t, op.Extensions["x-semlia-actions"], &actions)
				if len(actions) == 0 || op.Extensions["x-semlia-reauthorize"] != true {
					t.Errorf("%s lacks current scoped authorization", op.OperationID)
				}
				if len(doc.Security) == 0 && (op.Security == nil || len(*op.Security) == 0) {
					t.Errorf("%s lacks security", op.OperationID)
				}
				for _, status := range []string{"401", "403", "404"} {
					if op.Responses.Value(status) == nil {
						t.Errorf("%s lacks %s response", op.OperationID, status)
					}
				}
				if method != "GET" {
					if op.RequestBody == nil || !op.RequestBody.Value.Required {
						t.Errorf("%s requires a bounded request body", op.OperationID)
					}
					if !productionHasParameter(item.Parameters, op.Parameters, "header", "Idempotency-Key") {
						t.Errorf("%s lacks server idempotency key", op.OperationID)
					}
					for _, status := range []string{"409", "413", "422"} {
						if op.Responses.Value(status) == nil {
							t.Errorf("%s lacks %s response", op.OperationID, status)
						}
					}
				}
			}
		}
		if len(doc.Paths.Map()) != len(paths) {
			t.Errorf("unexpected path expansion: got %d, want %d", len(doc.Paths.Map()), len(paths))
		}
		for _, path := range []string{"/production-operations/{operationId}/publish", "/production-releases/{releaseId}/rollback"} {
			item := doc.Paths.Value(prefix + path)
			if item != nil && item.Post != nil && item.Post.Extensions["x-semlia-human-required"] != true {
				t.Errorf("%s must preserve the human-only release duty", path)
			}
		}
	})

	t.Run("structured_invariants", func(t *testing.T) {
		want := map[string]any{
			"maxTargets": 32, "maxChangesPerTarget": 100, "maxTotalChanges": 256,
			"maxBodyBytes": 1048576, "maxModelOutputBytes": 1048576,
			"snapshotMembership": "immutable_exact", "unchangedRevision": "reuse",
			"partialCoverage": "explicit_no_deletion_inference", "legacyHistory": "unverifiable",
			"createBaseline": "absent", "updateBaseline": "exact_published_pin",
			"idempotencyScope":    []string{"workspace", "principal", "command", "key"},
			"replayAuthorization": "current", "recoveryAuthority": "server",
			"approvalAuthority": "proposal_validation_review", "publishAtomicity": "all_members_one_transaction",
			"publishRecheck":      []string{"authorization", "sod", "set_digest", "input_freshness", "baseline", "dependency_closure", "validations", "reviews"},
			"legacyMemberPublish": "reject", "rollback": "exact_before_manifest_including_absence",
			"releaseAttribution": "all_proposals", "legacyDigest": "unchanged",
			"migration": "expand_first", "lossyDown": "reject", "oldBinaryAfterActivation": "unsupported_fenced",
			"modelNetwork": "outside_transaction", "modelRetry": "explicit_no_automatic_paid_replay",
			"machineAllowlistAdditions": []string{}, "desktopViewports": []string{"1440x900", "1024x768"},
			"legacyProductionRollback":   "reject_including_descendants",
			"validationAttemptSelection": "one_complete_latest_attempt",
			"reviewValidationBinding":    "exact_attempt_digest_reapprove_after_revalidation",
			"modelOutputPersistence":     "immutable_schema_gated_body",
			"identityReintroduction":     "create_with_trusted_absence",
		}
		var got map[string]any
		productionDecode(t, doc.Extensions["x-semlia-invariants"], &got)
		var normalized map[string]any
		productionDecode(t, want, &normalized)
		if !reflect.DeepEqual(got, normalized) {
			t.Errorf("normative invariants differ\ngot: %#v\nwant: %#v", got, normalized)
		}
	})

	t.Run("create_and_update_are_disjoint", func(t *testing.T) {
		create := productionCreateTarget()
		productionAccepts(t, doc, "ProductionTarget", create, true)
		for _, key := range []string{"targetId", "baseRevisionId", "baseObjectVersion"} {
			bad := productionClone(create)
			bad[key] = productionID("ast")
			productionAccepts(t, doc, "ProductionTarget", bad, false)
		}
		update := productionClone(create)
		update["intent"] = "update"
		delete(update, "identityKey")
		update["targetId"] = productionID("ast")
		update["baseRevisionId"] = productionID("rev")
		productionAccepts(t, doc, "ProductionTarget", update, true)
		delete(update, "baseRevisionId")
		productionAccepts(t, doc, "ProductionTarget", update, false)
		update["baseObjectVersion"] = 1
		productionAccepts(t, doc, "ProductionTarget", update, false)
		update["kind"] = "entity_key"
		update["content"] = map[string]any{"asset": map[string]any{"localKey": "orders"}, "fields": []any{productionPhysicalRef("field")}, "uniqueness": "exact"}
		productionAccepts(t, doc, "ProductionTarget", update, true)
		update["baseRevisionId"] = productionID("rev")
		productionAccepts(t, doc, "ProductionTarget", update, false)
	})

	t.Run("bounded_requests_and_pages", func(t *testing.T) {
		s := productionSchema(t, doc, "CreateProductionRequest")
		targets := s.Properties["targets"].Value
		if targets.MinItems != 1 || targets.MaxItems == nil || *targets.MaxItems != 32 {
			t.Fatal("targets must be bounded to 1..32")
		}
		request := map[string]any{"input": productionInput(), "targets": []any{productionCreateTarget()}}
		productionAccepts(t, doc, "CreateProductionRequest", request, true)
		for _, field := range []string{"workspaceId", "createdBy", "approved", "publishedBy"} {
			forged := productionClone(request)
			forged[field] = productionID("prn")
			productionAccepts(t, doc, "CreateProductionRequest", forged, false)
		}
		for _, name := range []string{"CreateProductionRequest", "ReplaceProductionRequest"} {
			var total int
			productionDecode(t, productionSchema(t, doc, name).Extensions["x-semlia-max-total-changes"], &total)
			if total != 256 {
				t.Errorf("%s must declare the server-enforced aggregate change limit", name)
			}
		}
		tooMany := make([]any, 33)
		for i := range tooMany {
			tooMany[i] = productionCreateTarget()
		}
		request["targets"] = tooMany
		productionAccepts(t, doc, "CreateProductionRequest", request, false)
		changes := productionSchema(t, doc, "ChangeList")
		if changes.MaxItems == nil || *changes.MaxItems != 100 {
			t.Fatal("per-target change count must not exceed 100")
		}
		for _, name := range []string{"SnapshotPage", "SnapshotMemberPage", "DiagnosticPage", "ProductionOperationPage", "ValidationAttemptPage"} {
			page := productionSchema(t, doc, name)
			items := page.Properties["items"].Value
			if items.MaxItems == nil || *items.MaxItems != 200 || page.Properties["nextCursor"] == nil {
				t.Errorf("%s must expose bounded cursor pagination", name)
			}
		}
	})

	t.Run("exact_snapshot_membership", func(t *testing.T) {
		member := map[string]any{"kind": "dataset", "objectId": productionID("pds"), "revisionId": productionID("pdr"), "name": "public.orders", "locator": "public.orders", "contentDigest": productionDigest(), "coverageKey": "public"}
		productionAccepts(t, doc, "SnapshotMember", member, true)
		for _, field := range []string{"revisionId", "name", "coverageKey", "contentDigest"} {
			bad := productionClone(member)
			delete(bad, field)
			productionAccepts(t, doc, "SnapshotMember", bad, false)
		}
		coverage := map[string]any{"key": "public", "status": "partial", "enumerationComplete": false, "diagnosticCodes": []any{"SOURCE_TIMEOUT"}}
		productionAccepts(t, doc, "CoverageUnit", coverage, true)
		delete(coverage, "enumerationComplete")
		productionAccepts(t, doc, "CoverageUnit", coverage, false)
	})

	t.Run("frozen_governance_and_absence", func(t *testing.T) {
		for _, name := range []string{"SubmitProductionRequest", "ValidateProductionRequest", "ReviewProductionRequest", "PublishProductionRequest", "RollbackProductionRequest"} {
			s := productionSchema(t, doc, name)
			if !productionRequired(s, "expectedVersion") || !productionRequired(s, "setDigest") {
				t.Errorf("%s does not bind exact production version and set", name)
			}
		}
		attribution := productionSchema(t, doc, "ReleaseAttribution")
		if !productionRequired(attribution, "proposalIds") || !productionRequired(attribution, "operationId") || !productionRequired(attribution, "setDigest") {
			t.Fatal("release attribution must identify the complete production set")
		}
		absent := map[string]any{"kind": "semantic_asset", "targetId": productionID("ast"), "presence": "absent"}
		productionAccepts(t, doc, "BeforePin", absent, true)
		absent["revisionId"] = productionID("rev")
		productionAccepts(t, doc, "BeforePin", absent, false)
		local := map[string]any{"localKey": "orders"}
		productionAccepts(t, doc, "SemanticReference", local, true)
		local["targetId"] = productionID("ast")
		productionAccepts(t, doc, "SemanticReference", local, false)
		head := map[string]any{"presence": "absent"}
		productionAccepts(t, doc, "HeadReference", head, true)
		head["releaseId"] = productionID("rls")
		productionAccepts(t, doc, "HeadReference", head, false)
		for _, name := range []string{"PublishProductionRequest", "RollbackProductionRequest"} {
			if !productionRequired(productionSchema(t, doc, name), "expectedHead") {
				t.Errorf("%s must compare the actual workspace head", name)
			}
		}
		candidate := map[string]any{"candidateId": productionID("scd"), "digest": productionDigest(), "snapshotId": productionID("ssnp"), "targetKeys": []any{"orders"}, "primaryTargetKey": "orders"}
		productionAccepts(t, doc, "CandidateSelection", candidate, true)
		delete(candidate, "primaryTargetKey")
		productionAccepts(t, doc, "CandidateSelection", candidate, false)
		decision := map[string]any{"expectedVersion": 1, "setDigest": productionDigest(), "validation": map[string]any{"attemptNo": 1, "validationDigest": productionDigest()}, "proposalIds": []any{productionID("prp")}, "decision": "approve", "note": "Checked source evidence"}
		productionAccepts(t, doc, "ReviewProductionRequest", decision, true)
		decision["decision"] = "auto_publish"
		productionAccepts(t, doc, "ReviewProductionRequest", decision, false)
	})

	t.Run("legacy_rollback_protection_inherits", func(t *testing.T) {
		if !productionRequired(productionSchema(t, doc, "ProductionRelease"), "protection") {
			t.Error("every production release must carry inherited rollback protection")
		}
		root := map[string]any{"rootReleaseId": productionID("rls"), "rollbackDepth": 0}
		productionAccepts(t, doc, "ProductionReleaseProtection", root, true)
		child := map[string]any{"rootReleaseId": productionID("rls"), "rollbackDepth": 1, "rollbackParentReleaseId": productionID("rls")}
		productionAccepts(t, doc, "ProductionReleaseProtection", child, true)
		delete(child, "rollbackParentReleaseId")
		productionAccepts(t, doc, "ProductionReleaseProtection", child, false)
	})

	t.Run("complete_validation_attempt_binding", func(t *testing.T) {
		for _, name := range []string{"ReviewProductionRequest", "PublishProductionRequest"} {
			if !productionRequired(productionSchema(t, doc, name), "validation") {
				t.Errorf("%s must select one exact complete validation attempt", name)
			}
		}
		if !productionRequired(productionSchema(t, doc, "ValidateProductionRequest"), "previousAttemptNo") {
			t.Error("revalidation must CAS the latest attempt, not race another enqueue")
		}
		selection := map[string]any{"attemptNo": 1, "validationDigest": productionDigest()}
		productionAccepts(t, doc, "ValidationReference", selection, true)
		selection["runIds"] = []any{productionID("val")}
		productionAccepts(t, doc, "ValidationReference", selection, false)
		check := map[string]any{"runId": productionID("val"), "proposalId": productionID("prp"), "validatorId": "schema", "validatorVersion": "1.0.0", "status": "succeeded", "results": []any{map[string]any{"severity": "info", "code": "VALIDATOR_COMPLETED", "message": "Complete", "inputDigest": productionDigest(), "details": map[string]any{}}}}
		attempt := map[string]any{"attemptNo": 1, "status": "succeeded", "setDigest": productionDigest(), "freshnessDigest": productionDigest(), "requiredChecksDigest": productionDigest(), "validationDigest": productionDigest(), "runIds": []any{productionID("val")}, "checks": []any{check}, "completedAt": "2026-09-09T05:00:00Z"}
		productionAccepts(t, doc, "ValidationAttemptResult", attempt, true)
		attempt["validationDigest"] = nil
		productionAccepts(t, doc, "ValidationAttemptResult", attempt, false)
		attempt["status"] = "running"
		attempt["completedAt"] = nil
		productionAccepts(t, doc, "ValidationAttemptResult", attempt, true)
		attempt["validationDigest"] = productionDigest()
		productionAccepts(t, doc, "ValidationAttemptResult", attempt, false)
	})

	t.Run("durable_structured_generation_output", func(t *testing.T) {
		output := map[string]any{"schemaVersion": "semlia.production-suggestions/v1", "targets": []any{productionCreateTarget()}}
		productionAccepts(t, doc, "StructuredGenerationOutput", output, true)
		for _, field := range []string{"prompt", "hiddenReasoning", "providerResponse"} {
			bad := productionClone(output)
			bad[field] = "must not persist"
			productionAccepts(t, doc, "StructuredGenerationOutput", bad, false)
		}
		result := map[string]any{"runId": productionID("agr"), "operationId": productionID("prodop"), "inputVersion": 1, "inputDigest": productionDigest(), "status": "succeeded", "providerMode": "protocol_stub", "model": "stub", "modelConfigRevision": "1", "replayed": false, "outputDigest": productionDigest(), "output": output, "errorCode": nil, "costMicros": nil, "durationMs": 1}
		productionAccepts(t, doc, "GenerationResult", result, true)
		result["output"] = nil
		productionAccepts(t, doc, "GenerationResult", result, false)
		result["status"] = "failed"
		result["outputDigest"] = nil
		result["errorCode"] = "MODEL_OUTPUT_INVALID"
		productionAccepts(t, doc, "GenerationResult", result, true)
		result["output"] = output
		productionAccepts(t, doc, "GenerationResult", result, false)
		application := map[string]any{"runId": productionID("agr"), "sourceVersion": 1, "sourceOutputDigest": productionDigest(), "appliedVersion": 2, "appliedContentDigest": productionDigest(), "actorPrincipalId": productionID("prn"), "deltaDigest": productionDigest(), "delta": []any{map[string]any{"localKey": "orders", "change": map[string]any{"fieldPath": "definition", "op": "update", "beforeValue": "One accepted order", "afterValue": "One paid order"}}}}
		productionAccepts(t, doc, "GenerationApplication", application, true)
		delete(application, "actorPrincipalId")
		productionAccepts(t, doc, "GenerationApplication", application, false)
	})

	t.Run("reintroduce_absent_identity_without_fake_base", func(t *testing.T) {
		create := productionCreateTarget()
		create["reuseIdentity"] = map[string]any{"targetId": productionID("ast"), "creationOperationId": productionID("prodop"), "creationReleaseId": productionID("rls"), "absenceReleaseId": productionID("rls"), "expectedHead": map[string]any{"presence": "present", "releaseId": productionID("rls"), "manifestDigest": productionDigest()}}
		productionAccepts(t, doc, "ProductionTarget", create, true)
		create["baseRevisionId"] = productionID("rev")
		productionAccepts(t, doc, "ProductionTarget", create, false)
		delete(create, "baseRevisionId")
		create["reuseIdentity"].(map[string]any)["expectedHead"] = map[string]any{"presence": "absent"}
		productionAccepts(t, doc, "ProductionTarget", create, false)
		create["intent"] = "restore"
		productionAccepts(t, doc, "ProductionTarget", create, false)
	})
}

func productionSchema(t *testing.T, doc *openapi3.T, name string) *openapi3.Schema {
	t.Helper()
	ref := doc.Components.Schemas[name]
	if ref == nil || ref.Value == nil {
		t.Fatalf("missing schema %s", name)
	}
	return ref.Value
}

func productionAccepts(t *testing.T, doc *openapi3.T, name string, value any, want bool) {
	t.Helper()
	var normalized any
	productionDecode(t, value, &normalized)
	err := productionSchema(t, doc, name).VisitJSON(normalized)
	if (err == nil) != want {
		t.Errorf("%s accepts=%t, want %t: %v; value=%v", name, err == nil, want, err, value)
	}
}

func productionDecode(t *testing.T, value, target any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, target); err != nil {
		t.Fatal(err)
	}
}

func productionRequired(s *openapi3.Schema, name string) bool {
	for _, required := range s.Required {
		if required == name {
			return true
		}
	}
	return false
}

func productionHasParameter(path, operation openapi3.Parameters, in, name string) bool {
	for _, set := range []openapi3.Parameters{path, operation} {
		for _, ref := range set {
			if ref.Value.In == in && ref.Value.Name == name && ref.Value.Required {
				return true
			}
		}
	}
	return false
}

func productionClone(value map[string]any) map[string]any {
	clone := make(map[string]any, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}

func productionID(prefix string) string { return fmt.Sprintf("%s_01arz3ndektsv4rrffq69g5fav", prefix) }
func productionDigest() string          { return "sha256:" + strings.Repeat("a", 64) }

func productionPhysicalRef(kind string) map[string]any {
	return map[string]any{"snapshotId": productionID("ssnp"), "kind": kind, "objectId": productionID("pds"), "revisionId": productionID("pdr")}
}

func productionInput() map[string]any {
	return map[string]any{
		"snapshots":  []any{map[string]any{"sourceId": productionID("src"), "snapshotId": productionID("ssnp"), "digest": productionDigest(), "coverageKeys": []any{"public"}}},
		"candidates": []any{}, "evidence": []any{}, "dependencies": []any{},
	}
}

func productionCreateTarget() map[string]any {
	return map[string]any{
		"intent": "create", "kind": "semantic_asset", "localKey": "orders", "identityKey": "sales.orders", "title": "Orders",
		"content": map[string]any{"address": "sales.orders", "assetType": "business_object", "displayName": "Orders", "definition": "One accepted order", "scope": "Synthetic orders", "ownerPrincipalId": productionID("prn")},
		"changes": []any{}, "evidenceIds": []any{},
	}
}
