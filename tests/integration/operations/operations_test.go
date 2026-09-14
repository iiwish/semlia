package operations_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	operationsapp "github.com/iiwish/semlia/internal/application/operations"
	"github.com/iiwish/semlia/internal/domain/authorization"
	discoverydomain "github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/internal/domain/governance"
	operationsdomain "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/pkg/identity"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestRuntimeProjectionIsDurableIdempotentAndAppendOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("semlia_operations_test"),
		tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repositoryRoot(), "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close() })
	if err := migrator.Up(); err != nil {
		t.Fatal(err)
	}
	pool, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	workspaceID, _ := identity.NewWorkspaceID()
	runID, _ := identity.NewRunID()
	eventID, _ := identity.NewEventID()
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name, created_at, updated_at)
		VALUES ($1, 'runtime-projection', 'Runtime Projection', $2, $2)`, workspaceID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	run := operationsdomain.RuntimeRun{
		ID: runID, WorkspaceID: workspaceID, Kind: operationsdomain.RunKindSemanticResolution,
		SourceType: "semantic_query", SourceID: runID.String(), SourceVersionDigest: strings.Repeat("a", 64),
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", IdempotencyKey: "semantic-resolution:durable",
		State: operationsdomain.RunQueued, MaxAttempts: 3, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	event := operationsdomain.RuntimeRunEvent{
		ID: eventID, WorkspaceID: workspaceID, RunID: runID, EventKey: "queued", Type: operationsdomain.RunEventState,
		State: operationsdomain.RunQueued, Summary: "Queued", CreatedAt: now,
	}
	projector := operationsapp.NewProjector()
	store := pgstore.NewStore(pool)
	projection := operationsapp.Projection{Run: run, Event: &event}
	if err := projector.Project(ctx, store, projection); err != nil {
		t.Fatal(err)
	}
	if err := projector.Project(ctx, store, projection); err != nil {
		t.Fatalf("replay identical projection: %v", err)
	}

	restartedStore := pgstore.NewStore(pool)
	stored, err := restartedStore.GetRuntimeRun(ctx, workspaceID, runID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != 1 || stored.Kind != operationsdomain.RunKindSemanticResolution || stored.State != operationsdomain.RunQueued {
		t.Fatalf("stored run after replay = %+v", stored)
	}
	events, err := restartedStore.ListRuntimeRunEvents(ctx, workspaceID, runID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Sequence != 1 || events[0].EventKey != "queued" {
		t.Fatalf("events after replay = %+v", events)
	}
	if _, err := pool.Exec(ctx, `UPDATE runtime_run_events SET summary='mutated' WHERE id=$1`, eventID.UUID()); err == nil {
		t.Fatal("runtime event update unexpectedly succeeded")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM runtime_run_events WHERE id=$1`, eventID.UUID()); err == nil {
		t.Fatal("runtime event delete unexpectedly succeeded")
	}

	jobID, _ := identity.NewRunID()
	jobRunID, _ := identity.NewRunID()
	jobEventID, _ := identity.NewEventID()
	startedAt := now.Add(-2 * time.Minute)
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (
			id, workspace_id, job_type, payload, status, attempt, max_attempts, available_at,
			leased_until, lease_owner, idempotency_key, trace_id, created_at, updated_at
		) VALUES ($1, $2, 'semantic.resolve', '{"opaque":"must-not-be-projected"}'::jsonb, 'running', 1, 2, $3,
			$4, 'crashed-worker', 'runtime-recovery-job', $5, $3, $3)`,
		jobID.UUID(), workspaceID.UUID(), startedAt, now.Add(-time.Second), run.TraceID); err != nil {
		t.Fatal(err)
	}
	jobRun := operationsdomain.RuntimeRun{
		ID: jobRunID, WorkspaceID: workspaceID, Kind: operationsdomain.RunKindSemanticResolution,
		SourceType: "job", SourceID: jobID.String(), SourceVersionDigest: strings.Repeat("b", 64), JobID: &jobID,
		TraceID: run.TraceID, IdempotencyKey: "semantic-resolution:recovery", State: operationsdomain.RunRunning,
		Phase: "running", Attempt: 1, MaxAttempts: 2, StartedAt: &startedAt, Version: 1, CreatedAt: startedAt, UpdatedAt: startedAt,
	}
	jobEvent := operationsdomain.RuntimeRunEvent{ID: jobEventID, WorkspaceID: workspaceID, RunID: jobRunID,
		EventKey: "attempt:1:running", Type: operationsdomain.RunEventState, State: operationsdomain.RunRunning,
		Phase: "running", Summary: "cookie=session-secret", Metadata: []byte(`{"attempt":1,"providerToken":"secret","prompt":"private"}`), CreatedAt: startedAt}
	if err := projector.Project(ctx, store, operationsapp.Projection{Run: jobRun, Event: &jobEvent}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReapExpiredJobs(ctx, now); err != nil {
		t.Fatal(err)
	}
	recovered, err := restartedStore.GetRuntimeRun(ctx, workspaceID, jobRunID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != operationsdomain.RunQueued || recovered.Attempt != 1 || recovered.FinishedAt != nil {
		t.Fatalf("runtime after lease recovery = %+v", recovered)
	}
	claimed, err := store.ClaimJob(ctx, "recovery-worker", now.Add(time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.ID != jobID || claimed.Attempt != 2 {
		t.Fatalf("claimed recovered job = %+v", claimed)
	}
	running, err := restartedStore.GetRuntimeRun(ctx, workspaceID, jobRunID)
	if err != nil {
		t.Fatal(err)
	}
	if running.State != operationsdomain.RunRunning || running.Attempt != 2 {
		t.Fatalf("runtime after reclaim = %+v", running)
	}
	if err := store.MarkJobFailed(ctx, jobID, "recovery-worker", "HANDLER_FAILED", now.Add(time.Minute), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	dead, err := restartedStore.GetRuntimeRun(ctx, workspaceID, jobRunID)
	if err != nil {
		t.Fatal(err)
	}
	if dead.State != operationsdomain.RunDeadLetter || dead.ErrorCode != "HANDLER_FAILED" || dead.FinishedAt == nil {
		t.Fatalf("runtime after exhausted retry = %+v", dead)
	}
	recoveryEvents, err := restartedStore.ListRuntimeRunEvents(ctx, workspaceID, jobRunID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recoveryEvents) != 4 {
		t.Fatalf("runtime recovery events = %+v", recoveryEvents)
	}
	for _, projected := range recoveryEvents {
		if strings.Contains(string(projected.Metadata), "opaque") || strings.Contains(string(projected.Metadata), "providerToken") ||
			strings.Contains(string(projected.Metadata), "prompt") || strings.Contains(projected.Summary, "must-not-be-projected") ||
			strings.Contains(projected.Summary, "session-secret") {
			t.Fatalf("runtime event copied job payload: %+v", projected)
		}
	}

	otherWorkspaceID, _ := identity.NewWorkspaceID()
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name, created_at, updated_at)
		VALUES ($1, 'runtime-projection-other', 'Other Runtime Projection', $2, $2)`, otherWorkspaceID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	actorID, _ := identity.NewPrincipalID()
	secondActorID, _ := identity.NewPrincipalID()
	if _, err := pool.Exec(ctx, `
		INSERT INTO principals (id, workspace_id, kind, display_name, status, created_at)
		VALUES ($1, $2, 'human', 'Operations Reader', 'active', $3)`, actorID.UUID(), workspaceID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO principals (id, workspace_id, kind, display_name, status, created_at)
		VALUES ($1, $2, 'human', 'Second Operations Reader', 'active', $3)`, secondActorID.UUID(), workspaceID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	discoverySourceID, _ := identity.NewSourceConnectionID()
	discoveryJobID, _ := identity.NewRunID()
	discoveryRunID, _ := identity.NewRunID()
	discoveryRuntimeID, _ := identity.NewRunID()
	discoveryQueuedEventID, _ := identity.NewEventID()
	if _, err := pool.Exec(ctx, `
		INSERT INTO source_connections (id, workspace_id, adapter_kind, name, normalized_locator, created_at, updated_at)
		VALUES ($1, $2, 'postgresql_catalog', 'Atomic discovery', 'postgresql://atomic-discovery', $3, $3)`,
		discoverySourceID.UUID(), workspaceID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (
			id, workspace_id, job_type, payload, status, attempt, max_attempts, available_at,
			leased_until, lease_owner, idempotency_key, trace_id, created_at, updated_at
		) VALUES ($1, $2, 'source.discover', '{}'::jsonb, 'running', 1, 2, $3,
			$4, 'discovery-worker', 'atomic-discovery-job', $5, $3, $3)`,
		discoveryJobID.UUID(), workspaceID.UUID(), now, time.Now().Add(time.Minute), run.TraceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO discovery_runs (
			id, workspace_id, source_connection_id, adapter_version, status, job_id, requested_by,
			trace_id, started_at, created_at, updated_at
		) VALUES ($1, $2, $3, '1.0.0', 'running', $4, $5, $6, $7, $7, $7)`,
		discoveryRunID.UUID(), workspaceID.UUID(), discoverySourceID.UUID(), discoveryJobID.UUID(), actorID.String(), run.TraceID, now); err != nil {
		t.Fatal(err)
	}
	discoverySnapshot := discoverydomain.Snapshot{
		AdapterKind: "postgresql_catalog", AdapterVersion: "1.0.0", ExternalRevision: "history-1",
		Locator: "postgresql://atomic-discovery", ContentDigest: "sha256:" + strings.Repeat("e", 64), ObservedAt: now.Add(time.Second),
		Findings: []discoverydomain.Finding{{Code: "PARTIAL_DISCOVERY", Severity: "warning", Locator: "catalog", Details: map[string]any{}}},
	}
	if _, err := restartedStore.PersistDiscoveryRunSnapshot(ctx, workspaceID, discoverySourceID, discoveryRunID, discoverySnapshot); err == nil {
		t.Fatal("discovery completion succeeded without its runtime projection")
	}
	var discoveryStatus string
	var revisionCount int
	if err := pool.QueryRow(ctx, `SELECT status FROM discovery_runs WHERE id=$1`, discoveryRunID.UUID()).Scan(&discoveryStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM source_revisions WHERE source_connection_id=$1`, discoverySourceID.UUID()).Scan(&revisionCount); err != nil {
		t.Fatal(err)
	}
	if discoveryStatus != "running" || revisionCount != 0 {
		t.Fatalf("failed atomic projection left discovery=%s revisions=%d", discoveryStatus, revisionCount)
	}
	discoveryStarted := now
	discoveryJobLink := discoveryJobID
	discoveryRun := operationsdomain.RuntimeRun{
		ID: discoveryRuntimeID, WorkspaceID: workspaceID, Kind: operationsdomain.RunKindDiscovery,
		SourceType: "discovery_run", SourceID: discoveryRunID.String(), SourceVersionDigest: strings.Repeat("f", 64),
		JobID: &discoveryJobLink, TraceID: run.TraceID, IdempotencyKey: "runtime:discovery:" + discoveryRunID.String(),
		RequestedByPrincipalID: &actorID, State: operationsdomain.RunRunning, Phase: "running", Attempt: 1,
		MaxAttempts: 2, StartedAt: &discoveryStarted, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	discoveryQueuedEvent := operationsdomain.RuntimeRunEvent{
		ID: discoveryQueuedEventID, WorkspaceID: workspaceID, RunID: discoveryRuntimeID, EventKey: "attempt:1:running",
		Type: operationsdomain.RunEventState, State: operationsdomain.RunRunning, Phase: "running", CreatedAt: now,
	}
	if err := projector.Project(ctx, restartedStore, operationsapp.Projection{Run: discoveryRun, Event: &discoveryQueuedEvent}); err != nil {
		t.Fatal(err)
	}
	result, err := restartedStore.PersistDiscoveryRunSnapshot(ctx, workspaceID, discoverySourceID, discoveryRunID, discoverySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "degraded" {
		t.Fatalf("discovery result = %+v", result)
	}
	if err := restartedStore.MarkJobSucceeded(ctx, discoveryJobID, "discovery-worker", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	storedDiscovery, err := restartedStore.GetRuntimeRun(ctx, workspaceID, discoveryRuntimeID)
	if err != nil {
		t.Fatal(err)
	}
	var persistedDigest string
	if err := pool.QueryRow(ctx, `SELECT content_digest FROM source_revisions WHERE workspace_id=$1 AND id=$2`, workspaceID.UUID(), result.SourceRevisionID.UUID()).Scan(&persistedDigest); err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimPrefix(persistedDigest, "sha256:")) != 64 {
		t.Fatalf("invalid persisted source digest: %q", persistedDigest)
	}
	if storedDiscovery.State != operationsdomain.RunDegraded || storedDiscovery.FinishedAt == nil || storedDiscovery.SourceVersionDigest != strings.TrimPrefix(persistedDigest, "sha256:") {
		t.Fatalf("discovery terminal projection = %+v", storedDiscovery)
	}
	auditIDs := make([]identity.EventID, 4)
	for index := range auditIDs {
		auditIDs[index], _ = identity.NewEventID()
		auditWorkspace := workspaceID
		if index == 3 {
			auditWorkspace = otherWorkspaceID
		}
		if _, err := pool.Exec(ctx, `
			WITH inserted AS (
				INSERT INTO audit_events (id, workspace_id, event_type, actor_id, payload, trace_id, created_at)
				VALUES ($1, $2, 'runtime.filtered', $3,
					'{"objectType":"runtime_run","objectId":"run_visible","channel":"api","outcome":"succeeded","token":"must-not-leak"}'::jsonb,
					$4, $5)
				RETURNING id, workspace_id, payload
			)
			INSERT INTO audit_event_targets (workspace_id, audit_event_id, object_type, object_id, ordinal)
			SELECT inserted.workspace_id, inserted.id, target.object_type, target.object_id, target.ordinal
			FROM inserted CROSS JOIN LATERAL project_audit_event_targets(inserted.payload) AS target`,
			auditIDs[index].UUID(), auditWorkspace.UUID(), actorID.String(), run.TraceID,
			now.Add(time.Duration(index)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	from, to := now.Add(-time.Second), now.Add(4*time.Minute)
	filtered, err := restartedStore.ListAuditEvents(ctx, operationsapp.AuditQuery{
		WorkspaceID: workspaceID, ActorID: actorID.String(), EventType: "runtime.filtered",
		ObjectType: "runtime_run", ObjectID: "run_visible", TraceID: run.TraceID,
		From: &from, To: &to, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 || filtered[0].ID != auditIDs[2] || filtered[1].ID != auditIDs[1] {
		t.Fatalf("filtered audit page = %+v", filtered)
	}
	next, err := restartedStore.ListAuditEvents(ctx, operationsapp.AuditQuery{
		WorkspaceID: workspaceID, ActorID: actorID.String(), EventType: "runtime.filtered",
		ObjectType: "runtime_run", ObjectID: "run_visible", TraceID: run.TraceID,
		From: &from, To: &to, After: &operationsapp.PageCursor{Time: filtered[1].CreatedAt, ID: filtered[1].ID.UUID()}, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 1 || next[0].ID != auditIDs[0] {
		t.Fatalf("filtered audit next page = %+v", next)
	}

	evaluator := &allowEvaluator{principal: actorID}
	service := operationsapp.NewService(restartedStore, evaluator, operationsapp.ClockFunc(func() time.Time { return now.Add(5 * time.Minute) }),
		operationsdomain.DeploymentStatus{AuditRetention: "deployment_managed"})
	unknownAuditID, _ := identity.NewEventID()
	if _, err := pool.Exec(ctx, `
		WITH inserted AS (
			INSERT INTO audit_events (id, workspace_id, event_type, actor_id, payload, trace_id, created_at)
			VALUES ($1, $2, 'runtime.unknown-target', $3,
				'{"objectType":"provider_secret","objectId":"secret-row","outcome":"failed","token":"must-not-leak"}'::jsonb,
				$4, $5)
			RETURNING id, workspace_id, payload
		)
		INSERT INTO audit_event_targets (workspace_id, audit_event_id, object_type, object_id, ordinal)
		SELECT inserted.workspace_id, inserted.id, target.object_type, target.object_id, target.ordinal
		FROM inserted CROSS JOIN LATERAL project_audit_event_targets(inserted.payload) AS target`,
		unknownAuditID.UUID(), workspaceID.UUID(), actorID.String(), run.TraceID, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	unknownPage, err := service.ListAuditEvents(ctx, operationsapp.AuditListRequest{
		AccessRequest: operationsapp.AccessRequest{WorkspaceID: workspaceID, PrincipalRef: actorID.String(), TraceID: run.TraceID},
		EventType:     "runtime.unknown-target", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(unknownPage.Items) != 1 {
		t.Fatalf("unknown audit target projection = %+v", unknownPage.Items)
	}
	unknownDetails, _ := json.Marshal(unknownPage.Items[0].Details)
	if unknownPage.Items[0].ObjectType != "" || unknownPage.Items[0].ObjectID != "" || strings.Contains(string(unknownDetails), "must-not-leak") {
		t.Fatalf("unknown audit target projection = %+v", unknownPage.Items)
	}
	unknownFiltered, err := restartedStore.ListAuditEvents(ctx, operationsapp.AuditQuery{
		WorkspaceID: workspaceID, ObjectType: "provider_secret", ObjectID: "secret-row", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(unknownFiltered) != 0 {
		t.Fatalf("unknown payload object was filterable: %+v", unknownFiltered)
	}
	exportRequest := operationsapp.CreateAuditExportRequest{AuditListRequest: operationsapp.AuditListRequest{
		AccessRequest: operationsapp.AccessRequest{WorkspaceID: workspaceID, PrincipalRef: actorID.String(), TraceID: run.TraceID},
		ActorID:       actorID.String(), EventType: "runtime.filtered", ObjectType: "runtime_run", ObjectID: "run_visible",
		TraceFilter: run.TraceID, From: &from, To: &to,
	}, IdempotencyKey: "runtime-filtered-export"}
	firstExport, err := service.CreateAuditExport(ctx, exportRequest)
	if err != nil {
		t.Fatal(err)
	}
	replayRequest := exportRequest
	replayRequest.ActorID = "  " + actorID.String() + "  "
	replayRequest.ObjectType = " runtime_run "
	secondExport, err := service.CreateAuditExport(ctx, replayRequest)
	if err != nil {
		t.Fatal(err)
	}
	if firstExport.ID != secondExport.ID || firstExport.RowCount != 3 || len(firstExport.ContentDigest) != 64 {
		t.Fatalf("idempotent exports = first %+v second %+v", firstExport, secondExport)
	}
	changedRequest := exportRequest
	changedRequest.EventType = "runtime.unknown-target"
	if _, err := service.CreateAuditExport(ctx, changedRequest); !errors.Is(err, operationsdomain.ErrConflict) {
		t.Fatalf("same export key accepted different canonical filter: %v", err)
	}
	secondEvaluator := &allowEvaluator{principal: secondActorID}
	secondService := operationsapp.NewService(restartedStore, secondEvaluator, operationsapp.ClockFunc(func() time.Time { return now.Add(5 * time.Minute) }),
		operationsdomain.DeploymentStatus{AuditRetention: "deployment_managed"})
	creatorRequest := exportRequest
	creatorRequest.PrincipalRef = secondActorID.String()
	if _, err := secondService.CreateAuditExport(ctx, creatorRequest); !errors.Is(err, operationsdomain.ErrConflict) {
		t.Fatalf("same export key accepted different creator: %v", err)
	}
	downloaded, err := service.GetAuditExportContent(ctx, operationsapp.AccessRequest{
		WorkspaceID: workspaceID, PrincipalRef: actorID.String(), TraceID: run.TraceID,
	}, firstExport.ID)
	if err != nil {
		t.Fatal(err)
	}
	downloadDigest := sha256.Sum256(downloaded.Content)
	if downloaded.ContentDigest != hex.EncodeToString(downloadDigest[:]) || downloaded.ContentDigest != firstExport.ContentDigest ||
		strings.Contains(string(downloaded.Content), "must-not-leak") || !strings.Contains(string(downloaded.Content), `"objectType":"runtime_run"`) {
		t.Fatalf("download content/digest = %s %s", downloaded.ContentDigest, downloaded.Content)
	}
	if _, err := service.GetAuditExportContent(ctx, operationsapp.AccessRequest{
		WorkspaceID: otherWorkspaceID, PrincipalRef: actorID.String(), TraceID: run.TraceID,
	}, firstExport.ID); !errors.Is(err, operationsdomain.ErrNotFound) {
		t.Fatalf("cross-workspace export download error = %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_exports SET expires_at=$2 WHERE id=$1`, firstExport.ID.UUID(), now.Add(6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	expiredService := operationsapp.NewService(restartedStore, evaluator, operationsapp.ClockFunc(func() time.Time { return now.Add(7 * time.Minute) }),
		operationsdomain.DeploymentStatus{AuditRetention: "deployment_managed"})
	if _, err := expiredService.CreateAuditExport(ctx, exportRequest); !errors.Is(err, operationsdomain.ErrConflict) {
		t.Fatalf("expired export idempotency key did not return stable conflict: %v", err)
	}
	if _, err := expiredService.GetAuditExportContent(ctx, operationsapp.AccessRequest{
		WorkspaceID: workspaceID, PrincipalRef: actorID.String(), TraceID: run.TraceID,
	}, firstExport.ID); !errors.Is(err, operationsdomain.ErrNotFound) {
		t.Fatalf("expired export download error = %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_exports SET expires_at=$2 WHERE id=$1`, firstExport.ID.UUID(), firstExport.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_exports SET content='tampered'::bytea WHERE id=$1`, firstExport.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetAuditExportContent(ctx, operationsapp.AccessRequest{
		WorkspaceID: workspaceID, PrincipalRef: actorID.String(), TraceID: run.TraceID,
	}, firstExport.ID); err == nil || strings.Contains(err.Error(), "tampered") {
		t.Fatalf("tampered export was returned or leaked: %v", err)
	}
	var exports, exportFacts, exportTargets int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_exports WHERE workspace_id=$1`, workspaceID.UUID()).Scan(&exports); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE workspace_id=$1 AND event_type='audit.exported'`, workspaceID.UUID()).Scan(&exportFacts); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_event_targets
		WHERE workspace_id=$1 AND object_type='audit_export' AND object_id=$2`, workspaceID.UUID(), firstExport.ID.String()).Scan(&exportTargets); err != nil {
		t.Fatal(err)
	}
	if exports != 1 || exportFacts != 1 || exportTargets != 1 || len(evaluator.requests) != 9 || len(secondEvaluator.requests) != 1 || evaluator.requests[0].Action != authorization.ActionAuditRead {
		t.Fatalf("export persistence=%d facts=%d targets=%d authorization=%+v", exports, exportFacts, exportTargets, evaluator.requests)
	}

	agentID, _ := identity.NewAgentRunID()
	inputHash := "sha256:" + strings.Repeat("c", 64)
	if _, err := restartedStore.CreateAgentRun(ctx, governance.AgentRun{
		ID: agentID, WorkspaceID: workspaceID, PrincipalID: &actorID, Model: "operations-test-model",
		ConfigRevision: "config-v1", InputHash: inputHash, Status: governance.AgentRunRunning,
		StartedAt: now, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	agentRuns, err := restartedStore.ListRuntimeRuns(ctx, operationsapp.RuntimeQuery{
		WorkspaceID: workspaceID, Kind: operationsdomain.RunKindAgent, SourceType: "agent_run", SourceID: agentID.String(), Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(agentRuns) != 1 || agentRuns[0].State != operationsdomain.RunRunning {
		t.Fatalf("agent runtime after owning create = %+v", agentRuns)
	}
	outputDigest := "sha256:" + strings.Repeat("d", 64)
	if _, err := restartedStore.FinishAgentRun(ctx, governanceapp.AgentRunFinishCommand{
		WorkspaceID: workspaceID, RunID: agentID, FinalState: governance.AgentRunSucceeded,
		OutputDigest: &outputDigest, CostMicros: 25, FinishedAt: now.Add(time.Second), DurationMS: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	finishedAgent, err := restartedStore.GetRuntimeRun(ctx, workspaceID, agentRuns[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if finishedAgent.State != operationsdomain.RunSucceeded || finishedAgent.FinishedAt == nil {
		t.Fatalf("agent runtime after owning finish = %+v", finishedAgent)
	}
	agentEvents, err := restartedStore.ListRuntimeRunEvents(ctx, workspaceID, agentRuns[0].ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(agentEvents) != 2 || agentEvents[0].State != operationsdomain.RunRunning || agentEvents[1].State != operationsdomain.RunSucceeded {
		t.Fatalf("agent runtime events = %+v", agentEvents)
	}

	settings, err := restartedStore.GetRuntimeSettings(ctx, workspaceID, now)
	if err != nil {
		t.Fatal(err)
	}
	settings.RetryCeiling, settings.Version, settings.UpdatedAt = 4, settings.Version+1, now.Add(time.Minute)
	updated, err := restartedStore.UpdateRuntimeSettings(ctx, settings, 1)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.RetryCeiling != 4 {
		t.Fatalf("updated runtime settings = %+v", updated)
	}
	if _, err := restartedStore.UpdateRuntimeSettings(ctx, settings, 1); !errors.Is(err, operationsdomain.ErrConflict) {
		t.Fatalf("stale runtime settings error = %v", err)
	}
}

type allowEvaluator struct {
	principal identity.PrincipalID
	requests  []authorizationapp.EvaluationRequest
}

func (evaluator *allowEvaluator) Evaluate(_ context.Context, request authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	evaluator.requests = append(evaluator.requests, request)
	return authorization.Decision{Allowed: true, Action: request.Action, PrincipalID: evaluator.principal,
		ReasonCode: authorization.ReasonRoleGrant, AuthorizationVersion: 1}, nil
}

func repositoryRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
