package operations_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestRuntimeKindsIncludeSemanticResolution(t *testing.T) {
	if !operations.ValidRunKind(operations.RunKindSemanticResolution) {
		t.Fatal("semantic resolution is not a valid runtime kind")
	}
}

func TestAuditProjectionDropsSecretsAndUnknownPayload(t *testing.T) {
	payload := json.RawMessage(`{
		"objectType":"source","objectId":"src_01k3exampleexampleexampleexampl",
		"outcome":"failed","reasonCode":"CONNECT_FAILED","channel":"api",
		"dsn":"postgres://user:secret@database.internal/app",
		"authorization":"Bearer top-secret","prompt":"private prompt","payload":{"token":"secret"}
	}`)
	projected := operations.ProjectAuditDetails("source.tested", payload)
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, secret := range []string{"postgres://", "top-secret", "private prompt", "database.internal", "token"} {
		if strings.Contains(text, secret) {
			t.Fatalf("redacted projection leaked %q: %s", secret, text)
		}
	}
	if _, ok := projected["objectType"]; ok || projected["outcome"] != "failed" {
		t.Fatalf("raw object identity was trusted or safe audit fields were lost: %#v", projected)
	}
}

func TestAuditProjectionAllowsSafeIngestionIdentifiersOnly(t *testing.T) {
	payload := json.RawMessage(`{
		"artifactId":"art_01k3exampleexampleexampleexampl",
		"artifactSetId":"ars_01k3exampleexampleexampleexampl",
		"scheduleId":"sch_01k3exampleexampleexampleexampl",
		"occurrenceId":"occ_01k3exampleexampleexampleexampl",
		"storageKey":"workspaces/secret/raw","content":"customer@example.com"
	}`)
	projected := operations.ProjectAuditDetails("artifact.uploaded", payload)
	for _, key := range []string{"artifactId", "artifactSetId", "scheduleId", "occurrenceId"} {
		if projected[key] == nil {
			t.Fatalf("safe ingestion identifier %q was dropped: %#v", key, projected)
		}
	}
	if _, ok := projected["storageKey"]; ok {
		t.Fatalf("storage key leaked: %#v", projected)
	}
	if _, ok := projected["content"]; ok {
		t.Fatalf("raw content leaked: %#v", projected)
	}
}

func TestRuntimeSettingsAreBoundedFutureRunDefaults(t *testing.T) {
	workspace := mustWorkspaceID(t)
	settings := operations.RuntimeSettings{WorkspaceID: workspace, RetryCeiling: 3,
		StatementTimeoutMS: 30_000, WebhookTimeoutMS: 10_000, QueryRowLimit: 10_000,
		QueryByteLimit: 10 * 1024 * 1024, RunMetadataRetentionDays: 30, Version: 1,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := settings.Validate(); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	settings.QueryRowLimit = 0
	if err := settings.Validate(); err == nil {
		t.Fatal("zero query row limit was accepted")
	}
}

func mustWorkspaceID(t *testing.T) identity.WorkspaceID {
	t.Helper()
	id, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
