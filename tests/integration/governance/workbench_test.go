package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/iiwish/semlia/internal/domain/authorization"
	workbenchdomain "github.com/iiwish/semlia/internal/domain/workbench"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestWorkbenchPostgresHTTPReplayVisibilityAndReadPurity(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "workbench-http")
	assetID, _ := environment.createAsset(t, workspace)
	admin, err := environment.store.LoadDefaultPrincipal(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	itemID := mustID(t, identity.NewAttentionItemID)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO attention_items (
			id,workspace_id,kind,dedupe_key,state,priority,risk,target_type,target_id,target_route,
			assignee_principal_id,audience_role_id,initiator_principal_id,rule_version,title,summary,
			reason_code,trace_id,opened_at,updated_at,version
		) VALUES ($1,$2,'review',$3,'open','high','high','asset',$4,$5,$6,'reviewer',$6,
			'workbench.conditions.v1','Review asset','Review is required','PROPOSAL_REVIEW_REQUIRED',$7,$8,$8,1)`,
		itemID.UUID(), workspace.UUID(), "review:"+itemID.UUID(), assetID.String(),
		"/governance?proposal=prp_01arz3ndektsv4rrffq69g5fav", admin.ID.UUID(), traceID, now); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/workspaces/" + workspace.String() + "/workbench/items"
	listed := environment.request(t, http.MethodGet, base+"?view=initiated", admin.ID.String(), "")
	if listed.Code != http.StatusOK {
		t.Fatalf("workbench list status = %d, body = %s", listed.Code, listed.Body.String())
	}
	var page struct {
		Items  []map[string]any `json:"items"`
		Counts struct {
			Total int `json:"total"`
			Open  int `json:"open"`
		} `json:"counts"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Counts.Total != 1 || page.Counts.Open != 1 || page.Items[0]["id"] != itemID.String() {
		t.Fatalf("workbench page = %+v", page)
	}

	get := environment.request(t, http.MethodGet, base+"/"+itemID.String(), admin.ID.String(), "")
	if get.Code != http.StatusOK {
		t.Fatalf("workbench get status = %d, body = %s", get.Code, get.Body.String())
	}
	var version int64
	var state string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT version,state FROM attention_items WHERE workspace_id=$1 AND id=$2`,
		workspace.UUID(), itemID.UUID()).Scan(&version, &state); err != nil {
		t.Fatal(err)
	}
	if version != 1 || state != "open" {
		t.Fatalf("GET mutated attention item: version=%d state=%s", version, state)
	}

	patch := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPatch, base+"/"+itemID.String(), strings.NewReader(body))
		request.Header.Set("X-Semlia-Principal", admin.ID.String())
		request.Header.Set("Idempotency-Key", "workbench-replay")
		response := httptest.NewRecorder()
		environment.handler.ServeHTTP(response, request)
		return response
	}
	body := `{"state":"in_progress","expectedVersion":1}`
	updated := patch(body)
	if updated.Code != http.StatusOK {
		t.Fatalf("workbench patch status = %d, body = %s", updated.Code, updated.Body.String())
	}
	replayed := patch(body)
	if replayed.Code != http.StatusOK || replayed.Body.String() != updated.Body.String() {
		t.Fatalf("workbench replay = %d %s, first = %s", replayed.Code, replayed.Body.String(), updated.Body.String())
	}
	mismatch := patch(`{"state":"dismissed","expectedVersion":2}`)
	if mismatch.Code != http.StatusConflict {
		t.Fatalf("workbench changed replay status = %d, body = %s", mismatch.Code, mismatch.Body.String())
	}
	var receipts int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE workspace_id=$1 AND event_type='workbench.attention_item.updated'
		  AND payload->>'idempotencyKey'='workbench-replay'`, workspace.UUID()).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 {
		t.Fatalf("workbench mutation receipts = %d, want 1", receipts)
	}

	other := createWorkspace(t, environment.pool, "workbench-other")
	otherAdmin, err := environment.store.LoadDefaultPrincipal(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	cross := environment.request(t, http.MethodGet,
		"/api/v1/workspaces/"+other.String()+"/workbench/items/"+itemID.String(), otherAdmin.ID.String(), "")
	if cross.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace get status = %d, body = %s", cross.Code, cross.Body.String())
	}
}

func TestWorkbenchReconcileUsesLatestDiscoveryRunAndStaysResolvedAfterRestart(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "workbench-reconcile")
	sourceID := mustID(t, identity.NewSourceConnectionID)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO source_connections (id,workspace_id,adapter_kind,name,normalized_locator,status,metadata)
		VALUES ($1,$2,'postgres','Warehouse','postgres://warehouse','active','{}')`,
		sourceID.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	failedID := mustID(t, identity.NewRunID)
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO discovery_runs (id,workspace_id,source_connection_id,adapter_version,status,error_code,stats,started_at,completed_at,created_at,updated_at,trace_id)
		VALUES ($1,$2,$3,'1.0.0','failed','NETWORK','{}',$4,$4,$4,$4,$5)`,
		failedID.UUID(), workspace.UUID(), sourceID.UUID(), now, nil); err != nil {
		t.Fatal(err)
	}
	stats, err := environment.store.ReconcileAttentionItems(context.Background(), 100)
	if err != nil || stats.Upserted != 1 {
		t.Fatalf("failed-run reconcile = %+v, err = %v", stats, err)
	}
	var itemUUID string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT id::text FROM attention_items WHERE workspace_id=$1 AND dedupe_key=$2`,
		workspace.UUID(), "source:"+sourceID.UUID()).Scan(&itemUUID); err != nil {
		t.Fatal(err)
	}
	publicItemID, err := identity.FromUUID(identity.AttentionItem, itemUUID)
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := identity.ParseAttentionItemID(publicItemID.String())
	if err != nil {
		t.Fatal(err)
	}
	oldItem, err := environment.store.GetAttentionItem(context.Background(), workspace, itemID)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldItem.TraceID) != 32 {
		t.Fatalf("legacy condition trace id = %q, want stable 32-character reconcile marker", oldItem.TraceID)
	}
	if _, err := environment.store.ReconcileAttentionItems(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	stableItem, err := environment.store.GetAttentionItem(context.Background(), workspace, itemID)
	if err != nil {
		t.Fatal(err)
	}
	if stableItem.Version != oldItem.Version || !stableItem.OpenedAt.Equal(oldItem.OpenedAt) {
		t.Fatalf("stable reconcile changed item: before=%+v after=%+v", oldItem, stableItem)
	}
	newFailureID := mustID(t, identity.NewRunID)
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO discovery_runs (id,workspace_id,source_connection_id,adapter_version,status,error_code,stats,started_at,completed_at,created_at,updated_at,trace_id)
		VALUES ($1,$2,$3,'1.0.0','failed','TIMEOUT','{}',$4,$4,$4,$4,$5)`,
		newFailureID.UUID(), workspace.UUID(), sourceID.UUID(), now.Add(30*time.Second), strings.Repeat("a", 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.store.ReconcileAttentionItems(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	admin, err := environment.store.LoadDefaultPrincipal(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	authorizationVersion, err := environment.store.AuthorizationVersion(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.store.UpdateAttentionItem(context.Background(), workbenchdomain.UpdateCommand{
		WorkspaceID: workspace, ItemID: itemID, ActorPrincipalID: admin.ID,
		State: workbenchdomain.StateDismissed, ExpectedVersion: oldItem.Version,
		TraceID: traceID, UpdatedAt: now.Add(45 * time.Second), AuditEventID: mustID(t, identity.NewEventID),
		IdempotencyKey: "stale-incident", RequestFingerprint: strings.Repeat("f", 64),
		AuthorizationVersion: authorizationVersion, VisibilityFingerprint: workbenchdomain.VisibilityFingerprint(oldItem),
	}); !errors.Is(err, workbenchdomain.ErrConflict) {
		t.Fatalf("stale incident mutation error = %v, want conflict", err)
	}
	succeededID := mustID(t, identity.NewRunID)
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO discovery_runs (id,workspace_id,source_connection_id,adapter_version,status,stats,started_at,completed_at,created_at,updated_at,trace_id)
		VALUES ($1,$2,$3,'1.0.0','succeeded','{}',$4,$4,$4,$4,$5)`,
		succeededID.UUID(), workspace.UUID(), sourceID.UUID(), now.Add(time.Minute), traceID); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := environment.store.ReconcileAttentionItems(context.Background(), 100); err != nil {
			t.Fatalf("restart reconcile %d: %v", attempt, err)
		}
	}
	var state string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT state FROM attention_items WHERE workspace_id=$1 AND dedupe_key=$2`,
		workspace.UUID(), "source:"+sourceID.UUID()).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "resolved" {
		t.Fatalf("latest successful discovery reopened historical failure: %s", state)
	}
}

func TestWorkbenchReconcileDefaultsEmptyRuntimeFailureSummary(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "workbench-empty-runtime-summary")
	runID := mustID(t, identity.NewRunID)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO runtime_runs (
			id,workspace_id,kind,source_type,source_id,source_version_digest,idempotency_key,
			state,max_attempts,finished_at,error_code,error_summary,created_at,updated_at
		) VALUES ($1::uuid,$2,'semantic_resolution','manual',$1::uuid::text,$3,'empty-runtime-summary',
			'failed',1,$4,'EXECUTION_FAILED','',$4,$4)`,
		runID.UUID(), workspace.UUID(), strings.Repeat("a", 64), now); err != nil {
		t.Fatal(err)
	}
	stats, err := environment.store.ReconcileAttentionItems(context.Background(), 100)
	if err != nil || stats.Upserted != 1 {
		t.Fatalf("empty-summary runtime reconcile = %+v, err = %v", stats, err)
	}
	var summary string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT summary FROM attention_items WHERE workspace_id=$1 AND dedupe_key=$2`,
		workspace.UUID(), "runtime:"+runID.UUID()).Scan(&summary); err != nil {
		t.Fatal(err)
	}
	if summary != "Runtime execution requires attention." {
		t.Fatalf("runtime attention summary = %q", summary)
	}
}

func TestWorkbenchReconcileUsesBindingConsumerForCompatibilityDeepLink(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "workbench-compatibility-route")
	consumerID := mustID(t, identity.NewConsumerID)
	bindingID := mustID(t, identity.NewConsumerBindingID)
	queryID := mustID(t, identity.NewSemanticQueryID)
	otherQueryID := mustID(t, identity.NewSemanticQueryID)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO consumers (
			id,workspace_id,stable_key,name,kind,status,owner_principal_ref,created_at,updated_at
		) VALUES ($1,$2,'compat-route','Compatibility route','application','active','owner',$3,$3)`,
		consumerID.UUID(), workspace.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO consumer_bindings (
			id,workspace_id,consumer_id,environment,purpose,mode,status,version,created_at,updated_at
		) VALUES ($1,$2,$3,'prod','compatibility test','current','active',1,$4,$4)`,
		bindingID.UUID(), workspace.UUID(), consumerID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	for index, candidate := range []identity.SemanticQueryID{queryID, otherQueryID} {
		if _, err := environment.pool.Exec(context.Background(), `
			INSERT INTO semantic_queries (
				id,workspace_id,principal_ref,consumer_id,binding_id,schema_version,resolver_version,
				canonical_request,request_digest,channel,trace_id,idempotency_key,outcome,created_at,finalized_at
			) VALUES ($1,$2,'caller',NULL,$3,'1.0.0','test','{}',$4,'api',$5,$6,'refused',$7,$7)`,
			candidate.UUID(), workspace.UUID(), bindingID.UUID(), "sha256:"+strings.Repeat("a", 64), traceID,
			fmt.Sprintf("compat-route-%d", index), now); err != nil {
			t.Fatal(err)
		}
		if _, err := environment.pool.Exec(context.Background(), `
			INSERT INTO semantic_refusals (
				workspace_id,query_id,reason_code,authorized_candidate_ids,clarification,details,created_at
			) VALUES ($1,$2,'POLICY_REFUSED','[]','No compatible plan','{}',$3)`,
			workspace.UUID(), candidate.UUID(), now); err != nil {
			t.Fatal(err)
		}
	}
	latestQueryID := queryID
	if otherQueryID.UUID() > queryID.UUID() {
		latestQueryID = otherQueryID
	}
	if _, err := environment.store.ReconcileAttentionItems(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	var targetType, targetID, route string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT target_type,target_id,target_route FROM attention_items
		WHERE workspace_id=$1 AND dedupe_key=$2`, workspace.UUID(), "compatibility:"+bindingID.UUID()).
		Scan(&targetType, &targetID, &route); err != nil {
		t.Fatal(err)
	}
	wantRoute := "/delivery/compatibility?binding=" + bindingID.String() +
		"&consumer=" + consumerID.String() + "&query=" + latestQueryID.String()
	if targetType != "consumer" || targetID != consumerID.String() || route != wantRoute {
		t.Fatalf("compatibility target=(%s,%s) route=%q want consumer %s route %q",
			targetType, targetID, route, consumerID, wantRoute)
	}
}

func TestWorkbenchProjectionTruncatesUTF8ByCharactersConsistently(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "workbench-utf8")
	assetID, revisionID := environment.createAsset(t, workspace)
	summary := strings.Repeat("数", 513)
	var payload map[string]any
	if err := json.Unmarshal([]byte(proposalCreateBody("semantic_asset", assetID.String(), revisionID.String())), &payload); err != nil {
		t.Fatal(err)
	}
	payload["summary"] = summary
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	created := environment.request(t, http.MethodPost, environment.proposalsPath(t, workspace), "", string(body))
	if created.Code != http.StatusCreated {
		t.Fatalf("create UTF-8 proposal = %d, body = %s", created.Code, created.Body.String())
	}
	proposalID, _ := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
	submitted := environment.request(t, http.MethodPost,
		environment.proposalsPath(t, workspace)+"/"+proposalID+"/submit", "", "")
	if submitted.Code != http.StatusOK {
		t.Fatalf("submit UTF-8 proposal = %d, body = %s", submitted.Code, submitted.Body.String())
	}
	environment.runValidationWorker(t)
	proposal, err := identity.ParseProposalID(proposalID)
	if err != nil {
		t.Fatal(err)
	}
	var projected string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT summary FROM attention_items WHERE workspace_id=$1 AND dedupe_key=$2`,
		workspace.UUID(), "review:"+proposal.UUID()).Scan(&projected); err != nil {
		t.Fatal(err)
	}
	if utf8.RuneCountInString(projected) != 512 || !utf8.ValidString(projected) {
		t.Fatalf("live projection summary runes=%d valid=%v", utf8.RuneCountInString(projected), utf8.ValidString(projected))
	}
	if _, err := environment.store.ReconcileAttentionItems(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	var reconciled string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT summary FROM attention_items WHERE workspace_id=$1 AND dedupe_key=$2`,
		workspace.UUID(), "review:"+proposal.UUID()).Scan(&reconciled); err != nil {
		t.Fatal(err)
	}
	if reconciled != projected {
		t.Fatal("live and reconcile UTF-8 truncation semantics diverged")
	}
}

func TestGovernedObjectAttentionUsesAuthoritativeAssetReadScopeAndWorkspaceCommandScope(t *testing.T) {
	environment := newFixture(t)
	workspace := createWorkspace(t, environment.pool, "workbench-object-scope")
	assetID, _ := environment.createAsset(t, workspace)
	sourceID := mustID(t, identity.NewSourceConnectionID)
	datasetID := mustID(t, identity.NewPhysicalDatasetID)
	bindingID := mustID(t, identity.NewPhysicalBindingID)
	now := time.Now().UTC()
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO source_connections (id,workspace_id,adapter_kind,name,normalized_locator,status,metadata)
		VALUES ($1,$2,'postgres','Warehouse','postgres://workbench-object','active','{}')`,
		sourceID.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO physical_datasets (id,workspace_id,source_connection_id,external_key,qualified_name)
		VALUES ($1,$2,$3,'public.orders','warehouse.public.orders')`,
		datasetID.UUID(), workspace.UUID(), sourceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO physical_bindings (id,workspace_id,asset_id,dataset_id,version,content,created_by,created_at,updated_at)
		VALUES ($1,$2,$3,$4,1,'{}','steward',$5,$5)`,
		bindingID.UUID(), workspace.UUID(), assetID.UUID(), datasetID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	path := environment.proposalsPath(t, workspace)
	created := environment.request(t, http.MethodPost, path, "",
		proposalCreateBody("physical_binding", bindingID.String(), ""))
	if created.Code != http.StatusCreated {
		t.Fatalf("physical binding proposal create = %d, body = %s", created.Code, created.Body.String())
	}
	proposalID, _ := decodeProposalDetail(t, created.Body.Bytes())["id"].(string)
	submitted := environment.request(t, http.MethodPost, path+"/"+proposalID+"/submit", "", "")
	if submitted.Code != http.StatusOK {
		t.Fatalf("physical binding proposal submit = %d, body = %s", submitted.Code, submitted.Body.String())
	}
	environment.runValidationWorker(t)
	proposalTyped, err := identity.ParseProposalID(proposalID)
	if err != nil {
		t.Fatal(err)
	}
	var targetType, targetID string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT target_type,target_id FROM attention_items WHERE workspace_id=$1 AND dedupe_key=$2`,
		workspace.UUID(), "review:"+proposalTyped.UUID()).Scan(&targetType, &targetID); err != nil {
		t.Fatal(err)
	}
	if targetType != "asset" || targetID != assetID.String() {
		t.Fatalf("governed object attention target = %s/%s, want asset/%s", targetType, targetID, assetID)
	}

	principalID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: principalID, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Asset reviewer", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	for _, binding := range []authorization.RoleBinding{
		{ID: mustID(t, identity.NewBindingID), PrincipalID: principalID, RoleID: "security_admin",
			ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID()},
		{ID: mustID(t, identity.NewBindingID), PrincipalID: principalID, RoleID: "reviewer",
			ScopeType: authorization.ScopeAsset, ScopeID: assetID.UUID()},
	} {
		if _, err := environment.store.CreateRoleBinding(context.Background(), binding); err != nil {
			t.Fatal(err)
		}
	}
	listed := environment.request(t, http.MethodGet,
		"/api/v1/workspaces/"+workspace.String()+"/workbench/items?view=team", principalID.String(), "")
	if listed.Code != http.StatusOK {
		t.Fatalf("asset-scoped workbench list = %d, body = %s", listed.Code, listed.Body.String())
	}
	var page struct {
		Items []struct {
			ID          string   `json:"id"`
			NextActions []string `json:"nextActions"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("asset-scoped items = %+v", page.Items)
	}
	for _, action := range page.Items[0].NextActions {
		if action == "review" {
			t.Fatalf("asset-scoped reviewer received workspace-scoped review command: %+v", page.Items[0])
		}
	}
}
