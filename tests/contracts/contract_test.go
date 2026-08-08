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
	if doc.Info.Version != "0.1.0" {
		t.Errorf("contract bundle version = %q, want 0.1.0", doc.Info.Version)
	}
	if version := doc.Extensions["x-semlia-contract-version"]; version == nil {
		t.Error("missing x-semlia-contract-version")
	}

	requiredSchemas := []string{
		"ApiVersion",
		"ResourceId",
		"Timestamp",
		"TraceId",
		"PageInfo",
		"ErrorResponse",
		"EventEnvelope",
		"HealthResponse",
		"SystemInfo",
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

	for name := range doc.Components.Schemas {
		lower := strings.ToLower(name)
		for _, forbidden := range []string{"asset", "proposal", "review", "release", "metric", "dimension", "measure"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("M1 business schema leaked into M0 contract: %s", name)
			}
		}
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
		{"resource ID", "ResourceId", `"event_01ARZ3NDEKTSV4RRFFQ69G5FAV"`, true},
		{"resource ID lowercase ULID", "ResourceId", `"event_01arz3ndektsv4rrffq69g5fav"`, false},
		{"resource ID without prefix", "ResourceId", `"01ARZ3NDEKTSV4RRFFQ69G5FAV"`, false},
		{"timestamp", "Timestamp", `"2026-08-08T08:00:00Z"`, true},
		{"timestamp with offset", "Timestamp", `"2026-08-08T16:00:00+08:00"`, false},
		{"timestamp without timezone", "Timestamp", `"2026-08-08T08:00:00"`, false},
		{"page info", "PageInfo", `{"limit":50,"nextCursor":"opaque"}`, true},
		{"page info limit zero", "PageInfo", `{"limit":0}`, false},
		{"page info extra field", "PageInfo", `{"limit":50,"page":2}`, false},
		{"error response", "ErrorResponse", `{"code":"DEPENDENCY_UNAVAILABLE","message":"database unavailable","traceId":"4bf92f3577b34da6a3ce929d0e0e4736","details":{},"retryable":true}`, true},
		{"error response missing details", "ErrorResponse", `{"code":"DEPENDENCY_UNAVAILABLE","message":"database unavailable","traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}`, false},
		{"error response invalid code", "ErrorResponse", `{"code":"dependency-unavailable","message":"database unavailable","traceId":"4bf92f3577b34da6a3ce929d0e0e4736","details":{}}`, false},
		{"event envelope", "EventEnvelope", `{"specVersion":"semlia.events/v1","id":"event_01ARZ3NDEKTSV4RRFFQ69G5FAV","type":"system.readiness.changed","source":"urn:semlia:control-plane","time":"2026-08-08T08:00:00Z","traceId":"4bf92f3577b34da6a3ce929d0e0e4736","data":{"ready":true}}`, true},
		{"event envelope invalid type", "EventEnvelope", `{"specVersion":"semlia.events/v1","id":"event_01ARZ3NDEKTSV4RRFFQ69G5FAV","type":"SystemReady","source":"urn:semlia:control-plane","time":"2026-08-08T08:00:00Z","traceId":"4bf92f3577b34da6a3ce929d0e0e4736","data":{}}`, false},
		{"event envelope extra field", "EventEnvelope", `{"specVersion":"semlia.events/v1","id":"event_01ARZ3NDEKTSV4RRFFQ69G5FAV","type":"system.readiness.changed","source":"urn:semlia:control-plane","time":"2026-08-08T08:00:00Z","traceId":"4bf92f3577b34da6a3ce929d0e0e4736","data":{},"secret":"no"}`, false},
		{"health response", "HealthResponse", `{"status":"ready","traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}`, true},
		{"health response invalid status", "HealthResponse", `{"status":"down","traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}`, false},
		{"system info", "SystemInfo", `{"service":"semlia","apiVersion":"v1","schemaVersion":"0.1.0","buildVersion":"dev","traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}`, true},
		{"system info invalid API version", "SystemInfo", `{"service":"semlia","apiVersion":"1","schemaVersion":"0.1.0","buildVersion":"dev","traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}`, false},
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
