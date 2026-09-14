package discovery_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/discovery/catalog"
	"github.com/iiwish/semlia/internal/adapters/discovery/postgresql"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	application "github.com/iiwish/semlia/internal/application/discovery"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func snapshotFixture(t *testing.T, name, status string, at time.Time) domain.Snapshot {
	t.Helper()
	snapshot, err := (catalog.Adapter{}).Discover(context.Background(), domain.Input{
		Locator: "catalog://snapshot", ObservedAt: at,
		Files: map[string][]byte{"catalog.json": []byte(fmt.Sprintf(`{"version":"1","datasets":[{"external_key":"stable-orders","qualified_name":%q,"kind":"table","locator":"warehouse.orders","fields":[{"external_key":"stable-id","name":"id","ordinal":1,"data_type":"bigint","nullable":false}]}]}`, name))},
	})
	if err != nil {
		t.Fatal(err)
	}
	// JSON keeps these runtime regression tests executable against the old model.
	coverage := fmt.Sprintf(`{"Coverage":[{"Key":"catalog","Selector":"catalog.json","ConfigDigest":"sha256:0000000000000000000000000000000000000000000000000000000000000000","Status":%q,"EnumerationComplete":%t,"DiagnosticCodes":[]}]}`, status, status == "complete")
	if err := json.Unmarshal([]byte(coverage), &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Coverage[0].ConfigDigest = domain.SnapshotDigest("catalog-v1")
	return snapshot
}

func TestSourceSnapshotUnchangedMembersAndHistoricalNames(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspace, source := createSource(t, pool, store, "snapshot-history")
	ctx := context.Background()
	at := time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC)
	first, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshotFixture(t, "orders", "complete", at))
	if err != nil {
		t.Fatal(err)
	}
	var original string
	if err := pool.QueryRow(ctx, `SELECT snapshot_id::text FROM source_snapshot_runs WHERE workspace_id=$1 AND run_id=$2`, workspace.UUID(), first.RunID.UUID()).Scan(&original); err != nil {
		t.Fatal(err)
	}
	replay, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshotFixture(t, "orders", "complete", at.Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	var snapshots, members int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM source_snapshots),(SELECT count(*) FROM source_snapshot_members WHERE snapshot_id=$1)`, original).Scan(&snapshots, &members); err != nil {
		t.Fatal(err)
	}
	if snapshots != 1 || members != 2 || !replay.Replayed {
		t.Fatalf("replay snapshots=%d members=%d result=%+v", snapshots, members, replay)
	}
	_, err = store.PersistDiscoverySnapshot(ctx, workspace, source, snapshotFixture(t, "renamed_orders", "complete", at.Add(2*time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT historical_name FROM source_snapshot_members WHERE snapshot_id=$1 AND kind='dataset'`, original).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "orders" {
		t.Fatalf("historical name=%q", name)
	}
	if _, err := pool.Exec(ctx, `UPDATE source_snapshot_members SET historical_name='forged' WHERE snapshot_id=$1`, original); err == nil {
		t.Fatal("immutable member update accepted")
	}
	var badParents int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM source_snapshot_members f LEFT JOIN source_snapshot_members d ON d.workspace_id=f.workspace_id AND d.snapshot_id=f.snapshot_id AND d.kind='dataset' AND d.object_id=f.parent_object_id AND d.revision_id=f.parent_revision_id WHERE f.kind='field' AND d.object_id IS NULL`).Scan(&badParents); err != nil || badParents != 0 {
		t.Fatalf("bad parents=%d err=%v", badParents, err)
	}
}

func TestSourceSnapshotPartialFailedAndOutOfOrderHeads(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspace, source := createSource(t, pool, store, "snapshot-coverage")
	ctx := context.Background()
	at := time.Date(2026, 9, 9, 2, 0, 0, 0, time.UTC)
	first, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshotFixture(t, "orders", "complete", at))
	if err != nil {
		t.Fatal(err)
	}
	for index, status := range []string{"partial", "failed"} {
		snapshot := snapshotFixture(t, "orders", status, at.Add(time.Duration(index+1)*time.Hour))
		snapshot.Datasets = nil
		if _, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshot); err != nil {
			t.Fatal(err)
		}
	}
	var latest, verified, effective string
	err = pool.QueryRow(ctx, `SELECT h.latest_attempt_status,h.latest_verified_snapshot_id::text,e.snapshot_id::text FROM source_coverage_heads h JOIN source_effective_snapshots e USING(workspace_id,source_connection_id) WHERE h.workspace_id=$1`, workspace.UUID()).Scan(&latest, &verified, &effective)
	if err != nil {
		t.Fatal(err)
	}
	if latest != "failed" || verified != effective {
		t.Fatalf("latest=%s verified=%s effective=%s", latest, verified, effective)
	}
	older := snapshotFixture(t, "old_orders", "complete", at.Add(-time.Hour))
	if _, err := store.PersistDiscoverySnapshot(ctx, workspace, source, older); err != nil {
		t.Fatal(err)
	}
	var latestAfter string
	if err := pool.QueryRow(ctx, `SELECT latest_attempt_status FROM source_coverage_heads WHERE workspace_id=$1`, workspace.UUID()).Scan(&latestAfter); err != nil {
		t.Fatal(err)
	}
	if latestAfter != "failed" {
		t.Fatalf("older run overwrote newer attempt: %s (first %s)", latestAfter, first.RunID)
	}
}

func TestSourceSnapshotConcurrentReplay(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspace, source := createSource(t, pool, store, "snapshot-concurrent")
	snapshot := snapshotFixture(t, "orders", "complete", time.Now().UTC())
	errors := make(chan error, 4)
	var group sync.WaitGroup
	for range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := store.PersistDiscoverySnapshot(context.Background(), workspace, source, snapshot)
			errors <- err
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var snapshots, members int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM source_snapshots),(SELECT count(*) FROM source_snapshot_members)`).Scan(&snapshots, &members); err != nil {
		t.Fatal(err)
	}
	if snapshots != 1 || members != 2 {
		t.Fatalf("snapshots=%d members=%d", snapshots, members)
	}
}

func TestSourceSnapshotCodeBytesAndPreciseLineage(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspace, source := createSource(t, pool, store, "snapshot-code")
	content := []byte("CREATE TABLE public.orders(id bigint); CREATE VIEW public.summary(id) AS SELECT id FROM public.orders;")
	snapshot, err := (postgresql.Adapter{}).Discover(context.Background(), domain.Input{Locator: "git://schema", ObservedAt: time.Now().UTC(), Files: map[string][]byte{"schema.sql": content}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersistDiscoverySnapshot(context.Background(), workspace, source, snapshot); err != nil {
		t.Fatal(err)
	}
	var retained []byte
	if err := pool.QueryRow(context.Background(), `SELECT content_bytes FROM source_code_revisions`).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if string(retained) != string(content) {
		t.Fatal("code bytes not retained exactly")
	}
	var edges int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM source_lineage_revisions l JOIN source_snapshot_members u ON u.object_id=l.upstream_object_id AND u.revision_id=l.upstream_revision_id JOIN source_snapshot_members d ON d.snapshot_id=u.snapshot_id AND d.object_id=l.downstream_object_id AND d.revision_id=l.downstream_revision_id WHERE l.code_revision_id IS NOT NULL`).Scan(&edges); err != nil || edges != 1 {
		t.Fatalf("precise edges=%d err=%v", edges, err)
	}
	if _, err := pool.Exec(context.Background(), `DELETE FROM source_code_revisions`); err == nil {
		t.Fatal("code revision deletion accepted")
	}
}

func TestSourceSnapshotOutOfOrderDoesNotRegressCurrent(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspace, source := createSource(t, pool, store, "snapshot-ordering")
	ctx := context.Background()
	now := time.Now().UTC()
	newest, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshotFixture(t, "latest_orders", "complete", now))
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"failed", "partial", "complete"} {
		older := snapshotFixture(t, "historical_orders", status, now.Add(-time.Hour))
		if _, err := store.PersistDiscoverySnapshot(ctx, workspace, source, older); err != nil {
			t.Fatal(err)
		}
		var attempt, statusAfter, name string
		if err := pool.QueryRow(ctx, `SELECT h.latest_attempt_run_id::text,h.latest_attempt_status,d.qualified_name FROM source_coverage_heads h JOIN physical_datasets d ON d.workspace_id=h.workspace_id AND d.source_connection_id=h.source_connection_id WHERE h.workspace_id=$1`, workspace.UUID()).Scan(&attempt, &statusAfter, &name); err != nil {
			t.Fatal(err)
		}
		if attempt != newest.RunID.UUID() || statusAfter != "complete" || name != "latest_orders" {
			t.Fatalf("older %s rewrote current: run=%s status=%s name=%s", status, attempt, statusAfter, name)
		}
	}
	var historical, revisionNames int
	if err := pool.QueryRow(ctx, `SELECT count(*),count(DISTINCT revision_id) FROM source_snapshot_members WHERE kind='dataset' AND historical_name='historical_orders'`).Scan(&historical, &revisionNames); err != nil || historical != 3 || revisionNames != 1 {
		t.Fatalf("lost exact older observations: members=%d revisions=%d err=%v", historical, revisionNames, err)
	}
}

type snapshotEvaluator struct {
	pool       *pgstore.Pool
	deny, race bool
	requests   []authorizationapp.EvaluationRequest
}

func (e *snapshotEvaluator) Evaluate(ctx context.Context, request authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	e.requests = append(e.requests, request)
	var version int64
	if err := e.pool.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1`, request.WorkspaceID.UUID()).Scan(&version); err != nil {
		return authorization.Decision{}, err
	}
	if e.race {
		if _, err := e.pool.Exec(ctx, `UPDATE workspaces SET authorization_version=authorization_version+1 WHERE id=$1`, request.WorkspaceID.UUID()); err != nil {
			return authorization.Decision{}, err
		}
	}
	return authorization.Decision{Allowed: !e.deny, Action: request.Action, AuthorizationVersion: version}, nil
}

func snapshotReadService(t *testing.T, store *pgstore.Store, evaluator *snapshotEvaluator) *application.ControlService {
	t.Helper()
	loader, err := application.NewArtifactLoader("")
	if err != nil {
		t.Fatal(err)
	}
	return application.NewControlService(store, nil, nil, loader, nil, evaluator, application.ClockFunc(time.Now)).WithSnapshotCursorSecret([]byte(strings.Repeat("k", 32)))
}

func TestSourceSnapshotReadIsolationCursorsAndWatermark(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspace, source := createSource(t, pool, store, "snapshot-pages")
	otherWorkspace, foreignSource := createSource(t, pool, store, "snapshot-foreign")
	otherSource := mustID(t, identity.NewSourceConnectionID)
	ctx := context.Background()
	if _, err := store.CreateSourceConnection(ctx, semantic.SourceConnection{ID: otherSource, WorkspaceID: workspace, AdapterKind: "fixture", Name: "Other", NormalizedLocator: "fixture://other", Status: "active", Metadata: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	persist := func(w identity.WorkspaceID, s identity.SourceConnectionID, name string) application.PersistResult {
		t.Helper()
		snapshot := snapshotFixture(t, name, "complete", time.Now().UTC())
		snapshot.Findings = []domain.Finding{{Code: "TYPE_INFERRED", Severity: "info", Details: map[string]any{}}, {Code: "NAME_NORMALIZED", Severity: "info", Details: map[string]any{}}}
		result, err := store.PersistDiscoverySnapshot(ctx, w, s, snapshot)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := persist(workspace, source, "one")
	second := persist(workspace, source, "two")
	third := persist(workspace, source, "three")
	other := persist(workspace, otherSource, "other")
	foreign := persist(otherWorkspace, foreignSource, "foreign")
	evaluator := &snapshotEvaluator{pool: pool}
	service := snapshotReadService(t, store, evaluator)
	request := application.SnapshotRequest{SourceRequest: application.SourceRequest{WorkspaceID: workspace, SourceID: source, PrincipalRef: "reader"}, Limit: 1}
	page, err := service.ListSnapshots(ctx, request)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != third.SnapshotID || page.NextCursor == nil {
		t.Fatalf("first page=%+v err=%v", page, err)
	}
	newest := persist(workspace, source, "after-watermark")
	seen := []string{page.Items[0].ID}
	for page.NextCursor != nil {
		request.Cursor = *page.NextCursor
		page, err = service.ListSnapshots(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			seen = append(seen, item.ID)
		}
	}
	if strings.Join(seen, ",") != strings.Join([]string{third.SnapshotID, second.SnapshotID, first.SnapshotID}, ",") {
		t.Fatalf("watermark pages=%v includes later %s", seen, newest.SnapshotID)
	}
	request.Cursor = ""
	request.SnapshotID = first.SnapshotID
	members, err := service.ListSnapshotMembers(ctx, request)
	if err != nil || len(members.Items) != 1 || members.NextCursor == nil {
		t.Fatalf("members=%+v err=%v", members, err)
	}
	memberCursor := *members.NextCursor
	request.Cursor = memberCursor
	nextMembers, err := service.ListSnapshotMembers(ctx, request)
	if err != nil || len(nextMembers.Items) != 1 || nextMembers.NextCursor != nil || members.Items[0].Kind != "dataset" || nextMembers.Items[0].Kind != "field" || nextMembers.Items[0].ParentRevisionID != members.Items[0].RevisionID {
		t.Fatalf("member continuation=%+v err=%v", nextMembers, err)
	}
	for _, test := range []struct {
		name        string
		change      func(*application.SnapshotRequest)
		diagnostics bool
	}{
		{"filter", func(r *application.SnapshotRequest) { r.Kind = "field" }, false},
		{"snapshot", func(r *application.SnapshotRequest) { r.SnapshotID = second.SnapshotID }, false},
		{"source", func(r *application.SnapshotRequest) { r.SourceID = otherSource; r.SnapshotID = other.SnapshotID }, false},
		{"workspace", func(r *application.SnapshotRequest) {
			r.WorkspaceID = otherWorkspace
			r.SourceID = foreignSource
			r.SnapshotID = foreign.SnapshotID
		}, false},
		{"tamper", func(r *application.SnapshotRequest) { r.Cursor = "x" + r.Cursor[1:] }, false},
		{"route", func(r *application.SnapshotRequest) {}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := request
			test.change(&r)
			var err error
			if test.diagnostics {
				_, err = service.ListSnapshotDiagnostics(ctx, r)
			} else {
				_, err = service.ListSnapshotMembers(ctx, r)
			}
			if !errors.Is(err, application.ErrSnapshotCursor) {
				t.Fatalf("cursor accepted: %v", err)
			}
		})
	}
	request.Cursor = ""
	diagnostics, err := service.ListSnapshotDiagnostics(ctx, request)
	if err != nil || len(diagnostics.Items) != 1 || diagnostics.NextCursor == nil || diagnostics.Items[0].Ordinal != 1 {
		t.Fatalf("diagnostics=%+v err=%v", diagnostics, err)
	}
	request.Cursor = *diagnostics.NextCursor
	diagnostics, err = service.ListSnapshotDiagnostics(ctx, request)
	if err != nil || len(diagnostics.Items) != 1 || diagnostics.NextCursor != nil || diagnostics.Items[0].Ordinal != 2 {
		t.Fatalf("diagnostic continuation=%+v err=%v", diagnostics, err)
	}
	for _, test := range []struct {
		name      string
		workspace identity.WorkspaceID
		source    identity.SourceConnectionID
		snapshot  string
	}{
		{"known snapshot wrong source", workspace, otherSource, first.SnapshotID},
		{"known source wrong workspace", otherWorkspace, source, first.SnapshotID},
		{"known foreign snapshot", workspace, source, foreign.SnapshotID},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := len(evaluator.requests)
			_, err := service.GetSnapshot(ctx, application.SnapshotRequest{SourceRequest: application.SourceRequest{WorkspaceID: test.workspace, SourceID: test.source}, SnapshotID: test.snapshot})
			if !errors.Is(err, domain.ErrNotFound) || len(evaluator.requests) != calls {
				t.Fatalf("relation should be hidden before auth: err=%v calls=%d", err, len(evaluator.requests)-calls)
			}
		})
	}
	request.Cursor = memberCursor
	evaluator.deny = true
	_, err = service.ListSnapshotMembers(ctx, request)
	var denied *authorization.DenialError
	if !errors.As(err, &denied) {
		t.Fatalf("revoked page accepted: %v", err)
	}
	evaluator.deny = false
	evaluator.race = true
	_, err = service.ListSnapshotMembers(ctx, request)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("authorization-version race accepted: %v", err)
	}
	for _, call := range evaluator.requests {
		if call.Action != authorization.ActionSourceRead || call.Resource.Type != authorization.ScopeSource {
			t.Fatalf("wrong capability: %+v", call)
		}
	}
}

func TestSourceSnapshotMemberByteBudgetAndSourceConfig(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspace, source := createSource(t, pool, store, "snapshot-budget")
	ctx := context.Background()
	snapshot := snapshotFixture(t, "orders", "complete", time.Now().UTC())
	snapshot.Datasets = nil
	for i := range 150 {
		snapshot.Datasets = append(snapshot.Datasets, domain.Dataset{ExternalKey: fmt.Sprintf("dataset-%03d", i), QualifiedName: fmt.Sprintf("%03d", i) + strings.Repeat("<", 1000), Kind: "table", Locator: strings.Repeat("<", 1000), CoverageKey: "catalog"})
	}
	first, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	service := snapshotReadService(t, store, &snapshotEvaluator{pool: pool})
	request := application.SnapshotRequest{SourceRequest: application.SourceRequest{WorkspaceID: workspace, SourceID: source}, SnapshotID: first.SnapshotID, Limit: 200}
	page, err := service.ListSnapshotMembers(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(page)
	if err != nil || len(encoded) > domain.SnapshotPageBytes || len(page.Items) == 0 || len(page.Items) >= 150 || page.NextCursor == nil {
		t.Fatalf("unbounded page: bytes=%d items=%d cursor=%v err=%v", len(encoded), len(page.Items), page.NextCursor, err)
	}
	seen := map[string]bool{}
	pages := 0
	for {
		pages++
		for _, item := range page.Items {
			if seen[item.ObjectID] {
				t.Fatal("duplicate member after byte trim")
			}
			seen[item.ObjectID] = true
		}
		if page.NextCursor == nil {
			break
		}
		request.Cursor = *page.NextCursor
		page, err = service.ListSnapshotMembers(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ = json.Marshal(page)
		if len(encoded) > domain.SnapshotPageBytes {
			t.Fatal("continuation exceeds byte budget")
		}
	}
	if len(seen) != 150 || pages < 2 {
		t.Fatalf("members=%d pages=%d", len(seen), pages)
	}
	if _, err := pool.Exec(ctx, `UPDATE source_connections SET version=version+1,metadata='{"schema":"changed"}' WHERE workspace_id=$1 AND id=$2`, workspace.UUID(), source.UUID()); err != nil {
		t.Fatal(err)
	}
	changed, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if changed.SourceRevisionID == first.SourceRevisionID || changed.SnapshotID == first.SnapshotID || !replay.Replayed || replay.SnapshotID != changed.SnapshotID {
		t.Fatalf("config identity first=%+v changed=%+v replay=%+v", first, changed, replay)
	}
	var headCount int
	var latestRun string
	if err := pool.QueryRow(ctx, `SELECT count(*),min(latest_attempt_run_id::text) FROM source_coverage_heads WHERE workspace_id=$1 AND source_connection_id=$2`, workspace.UUID(), source.UUID()).Scan(&headCount, &latestRun); err != nil || headCount != 1 || latestRun != changed.RunID.UUID() {
		t.Fatalf("source config opened an unrelated freshness head: count=%d latest=%s err=%v", headCount, latestRun, err)
	}
}

func TestSourceSnapshotUnitHeadsDeletionAndScopeShrink(t *testing.T) {
	pool, store := newStore(t)
	resetData(t, pool)
	workspace, source := createSource(t, pool, store, "snapshot-units")
	ctx := context.Background()
	now := time.Now().UTC()
	snapshot := snapshotFixture(t, "orders", "complete", now)
	snapshot.Coverage = nil
	snapshot.Datasets[0].CoverageKey = "a"
	snapshot.DeclareCoverage("a", "a.sql")
	snapshot.DeclareCoverage("b", "b.sql")
	other := snapshot.Datasets[0]
	other.ExternalKey = "other"
	other.QualifiedName = "other"
	other.CoverageKey = "b"
	snapshot.Datasets = append(snapshot.Datasets, other)
	first, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	partial := snapshot.Clone()
	partial.ObservedAt = now.Add(time.Minute)
	partial.Datasets = nil
	partial.Coverage[1].Status = "failed"
	partial.Coverage[1].EnumerationComplete = false
	second, err := store.PersistDiscoverySnapshot(ctx, workspace, source, partial)
	if err != nil {
		t.Fatal(err)
	}
	firstID, _ := identity.ParseSourceSnapshotID(first.SnapshotID)
	secondID, _ := identity.ParseSourceSnapshotID(second.SnapshotID)
	var a, b, effective string
	if err := pool.QueryRow(ctx, `SELECT (SELECT latest_verified_snapshot_id::text FROM source_coverage_heads WHERE workspace_id=$1 AND coverage_key='a'),(SELECT latest_verified_snapshot_id::text FROM source_coverage_heads WHERE workspace_id=$1 AND coverage_key='b'),(SELECT snapshot_id::text FROM source_effective_snapshots WHERE workspace_id=$1)`, workspace.UUID()).Scan(&a, &b, &effective); err != nil || a != secondID.UUID() || b != firstID.UUID() || effective != firstID.UUID() {
		t.Fatalf("partial unit advancement a=%s b=%s effective=%s err=%v", a, b, effective, err)
	}
	shrink := snapshot.Clone()
	shrink.ObservedAt = now.Add(2 * time.Minute)
	shrink.Coverage = shrink.Coverage[:1]
	shrink.Datasets = shrink.Datasets[:1]
	third, err := store.PersistDiscoverySnapshot(ctx, workspace, source, shrink)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	var bStatus string
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM source_effective_snapshots WHERE workspace_id=$1),(SELECT latest_attempt_status FROM source_coverage_heads WHERE workspace_id=$1 AND coverage_key='b')`, workspace.UUID()).Scan(&count, &bStatus); err != nil || count != 2 || bStatus != "failed" {
		t.Fatalf("shrink erased outside unit: heads=%d b=%s err=%v third=%s", count, bStatus, err, third.SnapshotID)
	}
	empty := snapshot.Clone()
	empty.ObservedAt = now.Add(3 * time.Minute)
	empty.Datasets = nil
	deleted, err := store.PersistDiscoverySnapshot(ctx, workspace, source, empty)
	if err != nil {
		t.Fatal(err)
	}
	deletedID, _ := identity.ParseSourceSnapshotID(deleted.SnapshotID)
	var oldMembers, newMembers int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM source_snapshot_members WHERE snapshot_id=$1),(SELECT count(*) FROM source_snapshot_members WHERE snapshot_id=$2)`, firstID.UUID(), deletedID.UUID()).Scan(&oldMembers, &newMembers); err != nil || oldMembers != 4 || newMembers != 0 {
		t.Fatalf("deletion history old=%d new=%d err=%v", oldMembers, newMembers, err)
	}
}

func TestSourceSnapshotUnresolvedLineageKeepsUnitAndMaximumSeverity(t *testing.T) {
	for _, order := range []string{"adapter-only", "warning-first", "error-first"} {
		t.Run(order, func(t *testing.T) {
			pool, store := newStore(t)
			resetData(t, pool)
			workspace, source := createSource(t, pool, store, "snapshot-lineage-"+order)
			ctx := context.Background()
			snapshot, err := (postgresql.Adapter{}).Discover(ctx, domain.Input{Locator: "git://two-units", ObservedAt: time.Now().UTC(), Files: map[string][]byte{
				"a_good.sql": []byte("CREATE TABLE public.good(id bigint);"),
				"b_bad.sql":  []byte("CREATE VIEW public.bad(id) AS SELECT id FROM public.missing;"),
			}})
			if err != nil {
				t.Fatal(err)
			}
			if order != "adapter-only" {
				severities := []string{"warning", "error"}
				if order == "error-first" {
					severities[0], severities[1] = severities[1], severities[0]
				}
				for _, severity := range severities {
					finding := domain.Finding{Code: "UNRESOLVED_LINEAGE", Severity: severity, Locator: "b_bad.sql", Details: map[string]any{}}
					// Remains executable against the prior Finding model for a genuine RED.
					if err := json.Unmarshal([]byte(`{"coverageKey":"sql:b_bad.sql"}`), &finding); err != nil {
						t.Fatal(err)
					}
					snapshot.Findings = append(snapshot.Findings, finding)
				}
			}
			result, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			id, _ := identity.ParseSourceSnapshotID(result.SnapshotID)
			var quality, overall, good, bad, diagnosticKey, severity string
			var diagnosticCount, effective, verifiedHeads, lineageMembers int
			err = pool.QueryRow(ctx, `SELECT s.history_quality,s.coverage_status,
(SELECT status FROM source_snapshot_scope WHERE snapshot_id=s.id AND coverage_key='sql:a_good.sql'),
(SELECT status FROM source_snapshot_scope WHERE snapshot_id=s.id AND coverage_key='sql:b_bad.sql'),
(SELECT min(coverage_key) FROM source_snapshot_diagnostics WHERE snapshot_id=s.id AND code='UNRESOLVED_LINEAGE'),
(SELECT min(severity) FROM source_snapshot_diagnostics WHERE snapshot_id=s.id AND code='UNRESOLVED_LINEAGE'),
(SELECT count(*) FROM source_snapshot_diagnostics WHERE snapshot_id=s.id AND code='UNRESOLVED_LINEAGE'),
(SELECT count(*) FROM source_effective_snapshots WHERE workspace_id=s.workspace_id),
(SELECT count(*) FROM source_coverage_heads WHERE workspace_id=s.workspace_id AND coverage_key='sql:a_good.sql' AND latest_verified_snapshot_id=s.id),
(SELECT count(*) FROM source_snapshot_members WHERE snapshot_id=s.id AND kind='lineage')
FROM source_snapshots s WHERE id=$1`, id.UUID()).Scan(&quality, &overall, &good, &bad, &diagnosticKey, &severity, &diagnosticCount, &effective, &verifiedHeads, &lineageMembers)
			if err != nil {
				t.Fatal(err)
			}
			if quality != "verified" || overall != "partial" || good != "complete" || bad != "partial" || diagnosticKey != "sql:b_bad.sql" || severity != "blocker" || diagnosticCount != 1 || effective != 0 || verifiedHeads != 1 || lineageMembers != 0 {
				t.Fatalf("lineage safety quality=%s overall=%s good=%s bad=%s diagnostic=%s/%s count=%d effective=%d goodVerified=%d lineage=%d", quality, overall, good, bad, diagnosticKey, severity, diagnosticCount, effective, verifiedHeads, lineageMembers)
			}
		})
	}
}
