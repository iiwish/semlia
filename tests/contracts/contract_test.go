package contracts_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

var root = repositoryRoot()

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve contract test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func TestRequiredContractArtifactsExist(t *testing.T) {
	required := []string{
		"api/openapi/semlia.v1.yaml",
		"api/gen/go/types.gen.go",
		"sdk/typescript/src/schema.gen.ts",
		"sdk/typescript/src/client.ts",
		"scripts/generate-contracts.sh",
	}

	var missing []string
	for _, path := range required {
		info, err := os.Stat(filepath.Join(root, path))
		if err != nil || !info.Mode().IsRegular() {
			missing = append(missing, path)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing contract artifacts: %s", strings.Join(missing, ", "))
	}
}

func TestCanonicalSpecificationIsValidAndMinimal(t *testing.T) {
	doc := loadSpecification(t)
	if doc.OpenAPI != "3.0.3" {
		t.Errorf("OpenAPI version = %q, want 3.0.3", doc.OpenAPI)
	}
	if doc.Info.Version != "0.4.0" {
		t.Errorf("contract bundle version = %q, want 0.4.0", doc.Info.Version)
	}
	if version := doc.Extensions["x-semlia-contract-version"]; version == nil {
		t.Error("missing x-semlia-contract-version")
	}

	requiredSchemas := []string{
		"ApiVersion",
		"ResourceId",
		"WorkspaceId",
		"Workspace",
		"CreateWorkspaceRequest",
		"SemanticAssetId",
		"AssetRevisionId",
		"SemanticRelationId",
		"OntologyRevisionId",
		"EvidenceArtifactId",
		"RunId",
		"EventId",
		"SourceConnectionId",
		"SourceRevisionId",
		"PhysicalDatasetId",
		"PhysicalDatasetRevisionId",
		"PhysicalFieldId",
		"PhysicalFieldRevisionId",
		"CodeArtifactId",
		"LineageEdgeId",
		"SemanticAddress",
		"SemanticAssetType",
		"RelationPredicate",
		"RelationAssertionState",
		"Timestamp",
		"TraceId",
		"PageInfo",
		"ErrorResponse",
		"EventEnvelope",
		"HealthResponse",
		"SystemInfo",
		"CatalogAssetSummary",
		"CatalogAssetDetail",
		"CatalogPage",
		"AssetRevision",
		"AssetRevisionPage",
		"AssetRelationPage",
		"DiscoveryRun",
		"CreateCatalogAssetRequest",
		"CreateAssetRevisionRequest",
		"GovernanceProposalId",
		"GovernanceAgentRunId",
		"GovernanceTargetObjectType",
		"GovernanceTargetObjectId",
		"GovernanceProposalState",
		"GovernanceChangeOp",
		"GovernanceChangeDigest",
		"GovernanceChangeSetItem",
		"GovernanceProposalAgentAttribution",
		"CreateGovernanceProposalRequest",
		"GovernanceProposalSummary",
		"GovernanceProposalDetail",
		"GovernanceProposalPage",
		"GovernanceValidationRunId",
		"GovernanceValidationResultId",
		"GovernanceValidationStatus",
		"GovernanceValidationSeverity",
		"GovernanceValidationResult",
		"GovernanceValidationRun",
		"GovernanceValidationRunPage",
		"GovernanceRiskLevel",
		"GovernanceRoutingChannel",
		"GovernancePolicyDecision",
		"GovernanceReviewerPrincipalId",
		"GovernanceReviewBatchId",
		"GovernanceReviewDecision",
		"GovernanceReviewRecordedDecision",
		"GovernanceReviewChannel",
		"GovernanceReviewCommandRequest",
		"GovernanceReview",
		"GovernanceDiffCategory",
		"GovernanceReviewGroupingRule",
		"GovernanceReviewAddedReason",
		"GovernanceReviewBatchStatus",
		"GovernanceReviewBatchMember",
		"GovernanceReviewBatch",
		"GovernanceReviewBatchDetail",
		"GovernanceReviewBatchPage",
		"GovernanceReleaseId",
		"GovernanceReleaseState",
		"GovernanceReleaseManifestAsset",
		"GovernanceReleaseManifestObject",
		"GovernanceReleaseManifest",
		"GovernanceRelease",
		"GovernanceReleaseDetail",
		"GovernanceReleasePage",
		"PublishGovernanceReleaseRequest",
	}
	for _, name := range requiredSchemas {
		if doc.Components.Schemas[name] == nil {
			t.Errorf("missing schema %s", name)
		}
	}

	requiredOperations := map[string]string{
		"/health/live":        "getLiveness",
		"/health/ready":       "getReadiness",
		"/api/v1/system/info": "getSystemInfo",
		"/api/v1/workspaces/{workspaceId}/catalog/assets":                                    "listCatalogAssets",
		"/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}":                          "getCatalogAsset",
		"/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/revisions":                "listAssetRevisions",
		"/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/revisions/{revisionId}":   "getAssetRevision",
		"/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/relations":                "listAssetRelations",
		"/api/v1/workspaces/{workspaceId}/discovery-runs/{runId}":                            "getDiscoveryRun",
		"/api/v1/workspaces/{workspaceId}/governance/proposals":                              "listGovernanceProposals",
		"/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}":                 "getGovernanceProposal",
		"/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}/validation-runs": "listGovernanceValidationRuns",
		"/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}/policy-decision": "getGovernancePolicyDecision",
		"/api/v1/workspaces/{workspaceId}/governance/review-batches":                         "listGovernanceReviewBatches",
		"/api/v1/workspaces/{workspaceId}/governance/review-batches/{batchId}":               "getGovernanceReviewBatch",
		"/api/v1/workspaces/{workspaceId}/governance/releases":                               "listGovernanceReleases",
		"/api/v1/workspaces/{workspaceId}/governance/releases/{releaseId}":                   "getGovernanceRelease",
	}
	for path, operationID := range requiredOperations {
		item := doc.Paths.Find(path)
		if item == nil || item.Get == nil {
			t.Errorf("missing GET operation for %s", path)
			continue
		}
		if item.Get.OperationID != operationID {
			t.Errorf("operation ID for %s = %q, want %q", path, item.Get.OperationID, operationID)
		}
	}
	assets := doc.Paths.Find("/api/v1/workspaces/{workspaceId}/catalog/assets")
	if assets == nil || assets.Post == nil || assets.Post.OperationID != "createCatalogAsset" {
		t.Error("missing POST createCatalogAsset operation")
	}
	revisions := doc.Paths.Find("/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/revisions")
	if revisions == nil || revisions.Post == nil || revisions.Post.OperationID != "createAssetRevision" {
		t.Error("missing POST createAssetRevision operation")
	}
	proposals := doc.Paths.Find("/api/v1/workspaces/{workspaceId}/governance/proposals")
	if proposals == nil || proposals.Post == nil || proposals.Post.OperationID != "createGovernanceProposal" {
		t.Error("missing POST createGovernanceProposal operation")
	}
	submit := doc.Paths.Find("/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}/submit")
	if submit == nil || submit.Post == nil || submit.Post.OperationID != "submitGovernanceProposal" {
		t.Error("missing POST submitGovernanceProposal operation")
	}
	reviews := doc.Paths.Find("/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}/reviews")
	if reviews == nil || reviews.Post == nil || reviews.Post.OperationID != "createGovernanceProposalReview" {
		t.Error("missing POST createGovernanceProposalReview operation")
	}
	batchAssembly := doc.Paths.Find("/api/v1/workspaces/{workspaceId}/governance/review-batches")
	if batchAssembly == nil || batchAssembly.Post == nil || batchAssembly.Post.OperationID != "createGovernanceReviewBatches" {
		t.Error("missing POST createGovernanceReviewBatches operation")
	}
	confirm := doc.Paths.Find("/api/v1/workspaces/{workspaceId}/governance/review-batches/{batchId}/confirm")
	if confirm == nil || confirm.Post == nil || confirm.Post.OperationID != "confirmGovernanceReviewBatch" {
		t.Error("missing POST confirmGovernanceReviewBatch operation")
	}
	publish := doc.Paths.Find("/api/v1/workspaces/{workspaceId}/governance/releases")
	if publish == nil || publish.Post == nil || publish.Post.OperationID != "publishGovernanceRelease" {
		t.Error("missing POST publishGovernanceRelease operation")
	}
	rollback := doc.Paths.Find("/api/v1/workspaces/{workspaceId}/governance/releases/{releaseId}/rollback")
	if rollback == nil || rollback.Post == nil || rollback.Post.OperationID != "rollbackGovernanceRelease" {
		t.Error("missing POST rollbackGovernanceRelease operation")
	}

}

func TestSharedSchemaValidationMatrix(t *testing.T) {
	doc := loadSpecification(t)
	tests := []struct {
		name       string
		schema     string
		payload    string
		shouldPass bool
	}{
		{"resource ID", "ResourceId", `"ast_01arz3ndektsv4rrffq69g5fav"`, true},
		{"resource ID uppercase", "ResourceId", `"ast_01ARZ3NDEKTSV4RRFFQ69G5FAV"`, false},
		{"resource ID without prefix", "ResourceId", `"01arz3ndektsv4rrffq69g5fav"`, false},
		{"resource ID invalid high suffix", "ResourceId", `"ast_81arz3ndektsv4rrffq69g5fav"`, false},
		{"workspace ID", "WorkspaceId", `"wsp_01arz3ndektsv4rrffq69g5fav"`, true},
		{"workspace ID wrong prefix", "WorkspaceId", `"ast_01arz3ndektsv4rrffq69g5fav"`, false},
		{"semantic address", "SemanticAddress", `"commerce.net_revenue"`, true},
		{"semantic address uppercase", "SemanticAddress", `"Commerce.net_revenue"`, false},
		{"timestamp", "Timestamp", `"2026-08-08T08:00:00Z"`, true},
		{"timestamp with offset", "Timestamp", `"2026-08-08T16:00:00+08:00"`, false},
		{"timestamp without timezone", "Timestamp", `"2026-08-08T08:00:00"`, false},
		{"page info", "PageInfo", `{"limit":50,"nextCursor":"opaque"}`, true},
		{"page info limit zero", "PageInfo", `{"limit":0}`, false},
		{"page info extra field", "PageInfo", `{"limit":50,"page":2}`, false},
		{"error response", "ErrorResponse", `{"code":"DEPENDENCY_UNAVAILABLE","message":"database unavailable","traceId":"4bf92f3577b34da6a3ce929d0e0e4736","details":{},"retryable":true}`, true},
		{"error response missing details", "ErrorResponse", `{"code":"DEPENDENCY_UNAVAILABLE","message":"database unavailable","traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}`, false},
		{"error response invalid code", "ErrorResponse", `{"code":"dependency-unavailable","message":"database unavailable","traceId":"4bf92f3577b34da6a3ce929d0e0e4736","details":{}}`, false},
		{"event envelope", "EventEnvelope", `{"specVersion":"semlia.events/v1","id":"evt_01arz3ndektsv4rrffq69g5fav","type":"system.readiness.changed","source":"urn:semlia:control-plane","workspaceId":"wsp_01arz3ndektsv4rrffq69g5fav","time":"2026-08-08T08:00:00Z","traceId":"4bf92f3577b34da6a3ce929d0e0e4736","data":{"ready":true}}`, true},
		{"event envelope wrong ID type", "EventEnvelope", `{"specVersion":"semlia.events/v1","id":"run_01arz3ndektsv4rrffq69g5fav","type":"system.readiness.changed","source":"urn:semlia:control-plane","time":"2026-08-08T08:00:00Z","traceId":"4bf92f3577b34da6a3ce929d0e0e4736","data":{}}`, false},
		{"event envelope invalid type", "EventEnvelope", `{"specVersion":"semlia.events/v1","id":"evt_01arz3ndektsv4rrffq69g5fav","type":"SystemReady","source":"urn:semlia:control-plane","time":"2026-08-08T08:00:00Z","traceId":"4bf92f3577b34da6a3ce929d0e0e4736","data":{}}`, false},
		{"event envelope extra field", "EventEnvelope", `{"specVersion":"semlia.events/v1","id":"evt_01arz3ndektsv4rrffq69g5fav","type":"system.readiness.changed","source":"urn:semlia:control-plane","time":"2026-08-08T08:00:00Z","traceId":"4bf92f3577b34da6a3ce929d0e0e4736","data":{},"secret":"no"}`, false},
		{"health response", "HealthResponse", `{"status":"ready","traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}`, true},
		{"health response invalid status", "HealthResponse", `{"status":"down","traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}`, false},
		{"system info", "SystemInfo", `{"service":"semlia","apiVersion":"v1","schemaVersion":"0.2.0","buildVersion":"dev","traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}`, true},
		{"system info invalid API version", "SystemInfo", `{"service":"semlia","apiVersion":"1","schemaVersion":"0.2.0","buildVersion":"dev","traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}`, false},
		{"catalog asset detail", "CatalogAssetDetail", `{"id":"ast_01arz3ndektsv4rrffq69g5fav","address":"commerce.net_revenue","assetType":"metric","lifecycleState":"active","title":"Net revenue","summary":"Revenue after refunds","updatedAt":"2026-09-02T08:00:00Z","createdAt":"2026-09-01T08:00:00Z","relationCount":2}`, true},
		{"catalog asset detail extra field", "CatalogAssetDetail", `{"id":"ast_01arz3ndektsv4rrffq69g5fav","address":"commerce.net_revenue","assetType":"metric","lifecycleState":"active","title":"Net revenue","summary":"Revenue after refunds","updatedAt":"2026-09-02T08:00:00Z","createdAt":"2026-09-01T08:00:00Z","relationCount":2,"databaseUuid":"hidden"}`, false},
		{"create catalog asset", "CreateCatalogAssetRequest", `{"address":"commerce.net_revenue","assetType":"metric","schemaVersion":"1.0.0","content":{"name":"Net revenue"},"createdBy":"founder"}`, true},
		{"create catalog asset unknown field", "CreateCatalogAssetRequest", `{"address":"commerce.net_revenue","assetType":"metric","schemaVersion":"1.0.0","content":{},"createdBy":"founder","credential":"secret"}`, false},
		{"create governance proposal", "CreateGovernanceProposalRequest", `{"targetObjectType":"semantic_asset","targetObjectId":"ast_01arz3ndektsv4rrffq69g5fav","baseRevisionId":"rev_01arz3ndektsv4rrffq69g5fav","title":"Tighten metric definition","summary":"Clarifies refunds","reason":"Audit finding","changeSet":[{"fieldPath":"definition","op":"update","beforeDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","afterDigest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","beforeValue":"Revenue after refunds","afterValue":"Revenue after refunds and chargebacks"}],"createdBy":"founder"}`, true},
		{"create governance proposal unknown field", "CreateGovernanceProposalRequest", `{"targetObjectType":"join_contract","targetObjectId":"jct_01arz3ndektsv4rrffq69g5fav","title":"Fix join","changeSet":[{"fieldPath":"joinExpression","op":"update","beforeDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","afterDigest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}],"createdBy":"founder","prompt":"raw text"}`, false},
		{"create governance proposal missing change set", "CreateGovernanceProposalRequest", `{"targetObjectType":"entity_key","targetObjectId":"eky_01arz3ndektsv4rrffq69g5fav","title":"Add key","createdBy":"founder"}`, false},
		{"governance proposal summary", "GovernanceProposalSummary", `{"id":"prp_01arz3ndektsv4rrffq69g5fav","targetObjectType":"semantic_asset","targetObjectId":"ast_01arz3ndektsv4rrffq69g5fav","state":"proposed","title":"Tighten metric definition","summary":"Clarifies refunds","reason":"Audit finding","createdBy":"founder","createdAt":"2026-09-03T08:00:00Z","updatedAt":"2026-09-03T08:00:00Z"}`, true},
		{"governance proposal summary storage uuid", "GovernanceProposalSummary", `{"id":"prp_01arz3ndektsv4rrffq69g5fav","targetObjectType":"semantic_asset","targetObjectId":"0192c0e2-1234-7abc-9def-0123456789ab","state":"draft","title":"x","createdBy":"founder","createdAt":"2026-09-03T08:00:00Z","updatedAt":"2026-09-03T08:00:00Z"}`, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref := doc.Components.Schemas[test.schema]
			if ref == nil || ref.Value == nil {
				t.Fatalf("missing resolved schema %s", test.schema)
			}
			var payload any
			if err := json.Unmarshal([]byte(test.payload), &payload); err != nil {
				t.Fatal(err)
			}
			err := ref.Value.VisitJSON(payload, openapi3.EnableFormatValidation(), openapi3.MultiErrors())
			if test.shouldPass && err != nil {
				t.Fatalf("expected valid payload: %v", err)
			}
			if !test.shouldPass && err == nil {
				t.Fatal("expected invalid payload")
			}
		})
	}
}

func TestCompatibilityPolicyDetectsBreakingChanges(t *testing.T) {
	base := filepath.Join(root, "tests", "contracts", "fixtures", "base.yaml")
	compatible := filepath.Join(root, "tests", "contracts", "fixtures", "compatible.yaml")
	breaking := filepath.Join(root, "tests", "contracts", "fixtures", "breaking.yaml")

	compatibleCommand := exec.Command("go", "tool", "oasdiff", "breaking", "--fail-on", "WARN", base, compatible)
	if output, err := compatibleCommand.CombinedOutput(); err != nil {
		t.Fatalf("compatible change rejected: %v\n%s", err, output)
	}

	breakingCommand := exec.Command("go", "tool", "oasdiff", "breaking", "--fail-on", "WARN", base, breaking)
	output, err := breakingCommand.CombinedOutput()
	if err == nil {
		t.Fatalf("breaking change accepted:\n%s", output)
	}
	if !strings.Contains(strings.ToLower(string(output)), "removed") {
		t.Fatalf("breaking output does not explain deletion:\n%s", output)
	}
}

func TestGeneratedArtifactsArePortable(t *testing.T) {
	checks := map[string][]string{
		"api/gen/go/types.gen.go": {
			"Code generated by github.com/oapi-codegen/oapi-codegen/v2 version v2.8.0 DO NOT EDIT.",
			"type ErrorResponse struct",
			"type EventEnvelope struct",
			"type WorkspaceId = identity.WorkspaceID",
			"type SemanticAssetId = identity.AssetID",
		},
		"sdk/typescript/src/schema.gen.ts": {
			"This file was auto-generated by openapi-typescript.",
			"export interface paths",
			"EventEnvelope:",
		},
	}
	for path, required := range checks {
		content, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		text := string(content)
		for _, phrase := range required {
			if !strings.Contains(text, phrase) {
				t.Errorf("%s missing %q", path, phrase)
			}
		}
		if strings.Contains(text, "/Users/") || strings.Contains(text, "/home/") {
			t.Errorf("%s contains a personal absolute path", path)
		}
	}
}

func loadSpecification(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromFile(filepath.Join(root, "api", "openapi", "semlia.v1.yaml"))
	if err != nil {
		t.Fatalf("load OpenAPI document: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate OpenAPI document: %v", err)
	}
	return doc
}
