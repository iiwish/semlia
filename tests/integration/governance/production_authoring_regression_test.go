package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	governance "github.com/iiwish/semlia/internal/domain/governance"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/sdk/trace"
)

// Lifecycle regressions isolate persistence; input provenance has separate acceptance probes.
func authoringLifecycleSetup(t *testing.T) (*fixture, http.Handler, identity.WorkspaceID, identity.PrincipalID) {
	t.Helper()
	f := newFixture(t)
	w := createWorkspace(t, f.pool, "acceptance-t003")
	p := authoringLifecyclePrincipal(t, f, w, "Author")
	a := authorizationapp.NewService(f.store, authorizationapp.ClockFunc(time.Now))
	tp := trace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	h := httpapi.NewHandler(
		application.NewSystemService(application.ReadinessProbeFunc(func(context.Context) error { return nil }), domain.SystemInfo{}),
		slog.New(slog.NewTextHandler(io.Discard, nil)), tp.Tracer("acceptance-t003"),
		httpapi.WithAuthorization(a), httpapi.WithProduction(governanceapp.NewProductionService(f.store)),
	)
	return f, h, w, p
}

func authoringLifecyclePrincipal(t *testing.T, f *fixture, w identity.WorkspaceID, name string) identity.PrincipalID {
	t.Helper()
	p := mustID(t, identity.NewPrincipalID)
	if _, err := f.store.CreatePrincipal(context.Background(), authz.Principal{
		ID: p, WorkspaceID: w, Kind: authz.PrincipalHuman, DisplayName: name, Status: authz.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CreateRoleBinding(context.Background(), authz.RoleBinding{
		ID: mustID(t, identity.NewBindingID), PrincipalID: p, RoleID: "workspace_admin", ScopeType: authz.ScopeWorkspace, ScopeID: w.UUID(),
	}); err != nil {
		t.Fatal(err)
	}
	return p
}

func authoringLifecycleBody(input string) string {
	return fmt.Sprintf(`{"input":%s,"targets":[{"intent":"create","kind":"semantic_asset","localKey":"revenue","identityKey":"finance.revenue","title":"Revenue","content":{"address":"finance.revenue","assetType":"metric","displayName":"Revenue","definition":"Net revenue"}}]}`, input)
}

func authoringLifecycleDecode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func authoringLifecycleCreate(t *testing.T, f *fixture, h http.Handler, w identity.WorkspaceID, p identity.PrincipalID) (string, string, map[string]any) {
	t.Helper()
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	body := authoringLifecycleBody(`{}`)
	body = productionPayloadInput(t, body, productionFixtureInput(t, f, w, identity.SemanticCandidateID{}, true), p)
	r := sendProdRequest(h, http.MethodPost, path, p.String(), "acceptance-create", body)
	if r.Code != http.StatusCreated {
		t.Fatalf("setup create: %d %s", r.Code, r.Body.String())
	}
	return path, body, authoringLifecycleDecode(t, r.Body.Bytes())
}

func TestProductionAuthoringRegressionUnknownInputMustNotCommit(t *testing.T) {
	for _, kind := range []string{"candidate", "snapshot"} {
		t.Run(kind, func(t *testing.T) {
			f, h, w, p := authoringLifecycleSetup(t)
			var input string
			if kind == "candidate" {
				id := mustID(t, identity.NewSemanticCandidateID)
				fixed := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
				snapshotID := fixed["snapshots"].([]any)[0].(map[string]any)["snapshotId"]
				fixed["candidates"] = []any{map[string]any{"candidateId": id.String(), "snapshotId": snapshotID, "digest": "sha256:" + strings.Repeat("1", 64), "primaryTargetKey": "revenue", "targetKeys": []string{"revenue"}}}
				encoded, err := json.Marshal(fixed)
				if err != nil {
					t.Fatal(err)
				}
				input = string(encoded)
			} else {
				id := mustID(t, identity.NewSourceSnapshotID)
				source := mustID(t, identity.NewSourceConnectionID)
				input = fmt.Sprintf(`{"snapshots":[{"sourceId":%q,"snapshotId":%q,"digest":"sha256:%s","coverageKeys":["schema:public"]}],"candidates":[],"evidence":[],"dependencies":[]}`, source.String(), id.String(), strings.Repeat("1", 64))
			}
			r := sendProdRequest(h, http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w), p.String(), "acceptance-unknown", completeProductionPayload(t, authoringLifecycleBody(input), p))
			if r.Code != http.StatusNotFound {
				t.Errorf("unknown %s must reach existence check and return 404; got %d %s", kind, r.Code, r.Body.String())
			}
			var count int
			if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM production_operations WHERE workspace_id=$1", w.UUID()).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Errorf("unknown %s committed %d operation(s)", kind, count)
			}
		})
	}
}

func TestProductionAuthoringRegressionReplaceMustPersistProposalsCommandAndContributor(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	path, body, created := authoringLifecycleCreate(t, f, h, w, p)
	editor := authoringLifecyclePrincipal(t, f, w, "Editor")
	opID := created["operationId"].(string)
	itemPath := path + "/" + opID
	replacement := strings.Replace(body, `"input":`, `"expectedVersion":1,"input":`, 1)
	r := sendProdRequest(h, http.MethodPut, itemPath, editor.String(), "acceptance-replace", replacement)
	if r.Code != http.StatusOK {
		t.Fatalf("replace: %d %s", r.Code, r.Body.String())
	}
	result := authoringLifecycleDecode(t, r.Body.Bytes())
	for _, raw := range result["proposalIds"].([]any) {
		id := mustParseProposalID(t, raw.(string))
		var count int
		if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM proposals WHERE workspace_id=$1 AND id=$2", w.UUID(), id.UUID()).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("successful PUT returned nonexistent proposal %s", id)
		}
	}
	for _, check := range []struct {
		name, query string
		args        []any
	}{
		{"idempotency command", "SELECT count(*) FROM production_commands WHERE workspace_id=$1 AND command_kind='replace_draft' AND idempotency_key='acceptance-replace'", []any{w.UUID()}},
		{"editor contributor", "SELECT count(*) FROM production_contributors WHERE workspace_id=$1 AND operation_id=$2 AND principal_id=$3", []any{w.UUID(), mustParseProductionOperationID(t, opID).UUID(), editor.UUID()}},
	} {
		var count int
		if err := f.pool.QueryRow(context.Background(), check.query, check.args...).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("successful PUT did not persist %s (count=%d)", check.name, count)
		}
	}
	replay := sendProdRequest(h, http.MethodPut, itemPath, editor.String(), "acceptance-replace", replacement)
	if replay.Code != http.StatusOK {
		t.Errorf("same-key PUT replay must succeed; got %d %s", replay.Code, replay.Body.String())
	}
}

func TestProductionAuthoringRegressionHistoricalReadAndOriginalReplay(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	path, body, created := authoringLifecycleCreate(t, f, h, w, p)
	itemPath := path + "/" + created["operationId"].(string)
	replacement := strings.Replace(body, `"input":`, `"expectedVersion":1,"input":`, 1)
	r := sendProdRequest(h, http.MethodPut, itemPath, p.String(), "acceptance-replace", replacement)
	if r.Code != http.StatusOK {
		t.Fatalf("replace: %d %s", r.Code, r.Body.String())
	}
	history := sendProdRequest(h, http.MethodGet, itemPath+"?version=1", p.String(), "", "")
	if history.Code != http.StatusOK {
		t.Errorf("stored version 1 must remain readable; got %d %s", history.Code, history.Body.String())
	}
	replay := sendProdRequest(h, http.MethodPost, path, p.String(), "acceptance-create", body)
	if replay.Code != http.StatusOK {
		t.Fatalf("create replay: %d %s", replay.Code, replay.Body.String())
	}
	replayed := authoringLifecycleDecode(t, replay.Body.Bytes())
	if replayed["version"] != created["version"] || replayed["setDigest"] != created["setDigest"] {
		t.Errorf("create replay returned current version/digest instead of original commit: original=%v/%v replay=%v/%v", created["version"], created["setDigest"], replayed["version"], replayed["setDigest"])
	}
}

func TestProductionAuthoringRegressionFrozenDraftMustRejectReplacement(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	path, body, created := authoringLifecycleCreate(t, f, h, w, p)
	op := mustParseProductionOperationID(t, created["operationId"].(string))
	// Establish a legal frozen version without depending on T004's validator.
	if _, err := f.pool.Exec(context.Background(), "UPDATE production_versions SET frozen_at=NOW() WHERE workspace_id=$1 AND operation_id=$2", w.UUID(), op.UUID()); err != nil {
		t.Fatal(err)
	}
	replacement := strings.Replace(body, `"input":`, `"expectedVersion":1,"input":`, 1)
	r := sendProdRequest(h, http.MethodPut, path+"/"+op.String(), p.String(), "acceptance-replace", replacement)
	if r.Code != http.StatusConflict {
		t.Errorf("frozen operation must reject PUT with 409; got %d %s", r.Code, r.Body.String())
	}
}

func TestProductionAuthoringRegressionAuthorizationControl(t *testing.T) {
	f, h, w, _ := authoringLifecycleSetup(t)
	p := mustID(t, identity.NewPrincipalID)
	if _, err := f.store.CreatePrincipal(context.Background(), authz.Principal{
		ID: p, WorkspaceID: w, Kind: authz.PrincipalHuman, DisplayName: "No roles", Status: authz.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	r := sendProdRequest(h, http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w), p.String(), "acceptance-no-role", authoringLifecycleBody(`{}`))
	if r.Code != http.StatusForbidden {
		t.Fatalf("control: principal without roles must be denied; got %d %s", r.Code, r.Body.String())
	}
}

func TestProductionAuthoringRegressionSourcePermissionRequired(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	path, body, created := authoringLifecycleCreate(t, f, h, w, p)
	limited := createPrincipalWithRoles(t, f, w, "No source scope", []string{"asset_owner"})
	for _, request := range []struct{ method, path, key, body string }{
		{http.MethodGet, path + "/" + created["operationId"].(string), "", ""},
		{http.MethodPost, path, "source-denied-create", body},
	} {
		result := sendProdRequest(h, request.method, request.path, limited.String(), request.key, request.body)
		if result.Code != http.StatusForbidden {
			t.Errorf("%s source denial: %d %s", request.method, result.Code, result.Body.String())
		}
	}
	list := sendProdRequest(h, http.MethodGet, path, limited.String(), "", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
	if items := authoringLifecycleDecode(t, list.Body.Bytes())["items"].([]any); len(items) != 0 {
		t.Fatalf("list leaked source-bound operation: %v", items)
	}
}

func TestProductionAuthoringRegressionReplacementFailureRollsBack(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	_, _, created := authoringLifecycleCreate(t, f, h, w, p)
	op := mustParseProductionOperationID(t, created["operationId"].(string))
	_, version, targets, _, _, err := f.store.GetProductionOperation(context.Background(), w, op)
	if err != nil {
		t.Fatal(err)
	}
	version.Version = 2
	proposalID := mustID(t, identity.NewProposalID)
	assetID, err := identity.ParseAssetID(targets[0].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	targets[0].Version = 2
	targets[0].ProposalID = &proposalID
	proposal := governance.Proposal{ID: proposalID, WorkspaceID: w, AssetID: &assetID, TargetObjectType: governance.TargetObjectType(targets[0].Kind), TargetObjectID: targets[0].TargetID, State: governance.ProposalDraft, Title: "Replacement", CreatedBy: p.String(), Intent: "create", CreationContent: targets[0].ContentJSON, ProductionOperationID: &op, ProductionVersion: &version.Version}
	// The command CHECK fails after proposals/targets are inserted, exercising
	// rollback of the entire transaction rather than only early validation.
	err = f.store.ReplaceDraftTx(context.Background(), op, version, targets, nil, governance.ProductionCommand{WorkspaceID: w, PrincipalID: p, CommandKind: "invalid-command", IdempotencyKey: "rollback-proof", RequestDigest: version.RequestDigest, OperationID: op, OperationVersion: 2, ResultKind: "operation", ResultID: op.String()}, []governance.Proposal{proposal})
	if err == nil {
		t.Fatal("invalid terminal command unexpectedly committed")
	}
	var current, versions, proposals, commands int
	err = f.pool.QueryRow(context.Background(), `SELECT current_version,(SELECT count(*) FROM production_versions WHERE operation_id=$1),(SELECT count(*) FROM proposals WHERE production_operation_id=$1),(SELECT count(*) FROM production_commands WHERE operation_id=$1) FROM production_operations WHERE id=$1`, op.UUID()).Scan(&current, &versions, &proposals, &commands)
	if err != nil {
		t.Fatal(err)
	}
	if current != 1 || versions != 1 || proposals != 1 || commands != 1 {
		t.Fatalf("partial replacement: current=%d versions=%d proposals=%d commands=%d", current, versions, proposals, commands)
	}
}

func TestProductionAuthoringRegressionPublishedAssetTargetMustNotCauseServerError(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	oldAuthor := createPrincipalWithRoles(t, f, w, "legacy-author", []string{"asset_owner"})
	publisher := createPrincipalWithRoles(t, f, w, "legacy-publisher", []string{"publisher"})
	reviewer := createReviewerPrincipal(t, f, w, "legacy-reviewer")
	asset, base := f.createAsset(t, w)
	_, _, proposalID := proposeToInReview(t, f, w, oldAuthor, asset, base)
	approveProposal(t, f, w, reviewer, proposalID)
	published := publishProposal(t, f, w, publisher, proposalID)
	if published.Code != http.StatusCreated {
		t.Fatalf("legacy baseline publish: %d %s", published.Code, published.Body.String())
	}
	release := authoringLifecycleDecode(t, published.Body.Bytes())
	pinned := release["manifest"].(map[string]any)["assets"].([]any)[0].(map[string]any)
	// This reduced input may be rejected as incomplete, but its existing target
	// must not be discarded and turned into an internal empty-UUID failure.
	body := fmt.Sprintf(`{"input":{},"targets":[{"intent":"update","kind":"semantic_asset","localKey":"revenue","targetId":%q,"baseRevisionId":%q,"title":"Update revenue","content":{"address":"commerce.net_revenue","assetType":"metric","displayName":"Net revenue","definition":"Revised definition","scope":null,"ownerPrincipalId":%q},"changes":[{"fieldPath":"definition","op":"update","beforeValue":"Revenue after refunds and chargebacks","afterValue":"Revised definition"}],"evidenceIds":[]}]}`, asset.String(), pinned["revisionId"].(string), p.String())
	r := sendProdRequest(h, http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w), p.String(), "acceptance-update", body)
	if r.Code >= http.StatusInternalServerError {
		t.Errorf("known target update must either succeed or reject incomplete input with 4xx, not fail internally; got %d %s", r.Code, r.Body.String())
	}
}
