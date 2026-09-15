package ingestion_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	localartifacts "github.com/iiwish/semlia/internal/adapters/artifacts/local"
	dbtartifact "github.com/iiwish/semlia/internal/adapters/discovery/dbt"
	fileartifact "github.com/iiwish/semlia/internal/adapters/discovery/files"
	sqlartifact "github.com/iiwish/semlia/internal/adapters/discovery/postgresql"
	discoveryapp "github.com/iiwish/semlia/internal/application/discovery"
	ingestionapp "github.com/iiwish/semlia/internal/application/ingestion"
	"github.com/iiwish/semlia/internal/application/jobs"
	discoverydomain "github.com/iiwish/semlia/internal/domain/discovery"
	ingestiondomain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
)

type snapshotBeginObserver struct {
	discoverydomain.Adapter
	beforeDiscover func()
}

func (adapter snapshotBeginObserver) Discover(ctx context.Context, input discoverydomain.Input) (discoverydomain.Snapshot, error) {
	adapter.beforeDiscover()
	return adapter.Adapter.Discover(ctx, input)
}

func TestSourceSnapshotLongPathsBeginAndPersistConsistently(t *testing.T) {
	longPath := strings.Repeat("nested/", 140) + strings.Repeat("x", 40) + ".sql"
	if len(longPath) != 1024 {
		t.Fatalf("fixture path bytes=%d", len(longPath))
	}
	manifest := map[string]any{
		"metadata": map[string]any{"dbt_schema_version": "https://schemas.getdbt.com/dbt/manifest/v12.json"},
		"nodes": map[string]any{"model.shop.orders": map[string]any{
			"database": "warehouse", "schema": "analytics", "name": "orders", "resource_type": "model",
			"package_name": "shop", "path": longPath, "original_file_path": longPath, "unique_id": "model.shop.orders",
			"fqn": []string{"shop", "orders"}, "alias": "orders", "checksum": map[string]string{"name": "sha256", "checksum": "abc"},
			"raw_code": "select 1 as id", "depends_on": map[string]any{"nodes": []string{}},
		}},
	}
	for _, key := range []string{"sources", "macros", "docs", "exposures", "metrics", "groups", "selectors", "disabled", "parent_map", "child_map", "group_map", "saved_queries", "semantic_models", "unit_tests"} {
		manifest[key] = map[string]any{}
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	largeCode := strings.Repeat(" ", (10<<20)-8) + "select 1"
	manifest["nodes"].(map[string]any)["model.shop.orders"].(map[string]any)["raw_code"] = largeCode
	largeManifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	longName := strings.Repeat("x", 251) + ".csv"
	largeSQL := strings.Repeat(" ", (50<<20)-len("CREATE TABLE orders (id bigint);")) + "CREATE TABLE orders (id bigint);"
	for _, test := range []struct {
		name, sourceKind, originalName, logicalPath, selector, mediaType string
		kind                                                             ingestiondomain.ArtifactKind
		adapter                                                          discoverydomain.Adapter
		content                                                          []byte
		code                                                             string
	}{
		{"sql", "sql_bundle", "schema.sql", longPath, longPath, "application/sql", ingestiondomain.ArtifactSQL, sqlartifact.Adapter{}, []byte("CREATE TABLE orders (id bigint);"), "CREATE TABLE orders (id bigint);"},
		{"sql-max-upload", "sql_bundle", "schema.sql", longPath, longPath, "application/sql", ingestiondomain.ArtifactSQL, sqlartifact.Adapter{}, []byte(largeSQL), largeSQL},
		{"dbt", "dbt_bundle", "manifest.json", "manifest.json", "manifest.json", "application/json", ingestiondomain.ArtifactDBTManifest, dbtartifact.Adapter{}, manifestBytes, "select 1 as id"},
		{"dbt-max-code", "dbt_bundle", "manifest.json", "manifest.json", "manifest.json", "application/json", ingestiondomain.ArtifactDBTManifest, dbtartifact.Adapter{}, largeManifestBytes, largeCode},
		{"file-max-name", "file", longName, longName, longName, "text/csv", ingestiondomain.ArtifactCSV, fileartifact.CSV(), []byte("id\n1\n"), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			pool, store := openStore(t)
			workspace := createWorkspace(t, pool, "snapshot-long-path-"+test.name)
			principal, _ := identity.NewPrincipalID()
			if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status) VALUES($1,$2,'human','Snapshot runner','active')`, principal.UUID(), workspace.UUID()); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			artifact := artifactCommand(t, workspace, &principal, test.content, now)
			artifact.Artifact.Kind, artifact.Artifact.OriginalName, artifact.Artifact.MediaType = test.kind, test.originalName, test.mediaType
			artifact.Object.MediaType = test.mediaType
			storage, err := localartifacts.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err = storage.Put(ctx, workspace, artifact.Object.ContentDigest, bytes.NewReader(test.content), int64(len(test.content))); err != nil {
				t.Fatal(err)
			}
			if err = store.ReserveArtifactObject(ctx, artifact); err != nil {
				t.Fatal(err)
			}
			if _, err = store.CreateArtifact(ctx, artifact); err != nil {
				t.Fatal(err)
			}
			source, _ := identity.NewSourceConnectionID()
			set := artifactSetCommand(t, workspace, source, principal, artifact, "long-path-set", now, nil, now.Add(24*time.Hour))
			set.Set.SourceKind, set.Set.Members[0].LogicalPath = test.sourceKind, test.logicalPath
			if _, err = store.FinalizeArtifactSet(ctx, set); err != nil {
				t.Fatal(err)
			}
			var attemptKey string
			observed := false
			adapter := snapshotBeginObserver{Adapter: test.adapter, beforeDiscover: func() {
				observed = true
				var status, selectorDigest string
				var snapshots int
				if err := pool.QueryRow(ctx, `SELECT h.coverage_key,h.latest_attempt_status,h.selector_digest,(SELECT count(*) FROM source_snapshots s WHERE s.workspace_id=h.workspace_id AND s.source_connection_id=h.source_connection_id) FROM source_coverage_heads h WHERE h.workspace_id=$1 AND h.source_connection_id=$2`, workspace.UUID(), source.UUID()).Scan(&attemptKey, &status, &selectorDigest, &snapshots); err != nil {
					t.Fatal(err)
				}
				if status != "partial" || snapshots != 0 || len(attemptKey) > 256 || selectorDigest != discoverydomain.CoverageSelectorDigest(discoverydomain.CoverageUnit{Selector: test.selector}) {
					t.Fatalf("begin key=%q status=%s selectorDigest=%s snapshots=%d", attemptKey, status, selectorDigest, snapshots)
				}
			}}
			loader, _ := discoveryapp.NewArtifactLoader("")
			control := discoveryapp.NewControlService(store, nil, nil, loader, nil, nil, discoveryapp.ClockFunc(time.Now)).WithArtifactStore(storage, map[ingestiondomain.ArtifactKind]discoverydomain.Adapter{test.kind: adapter})
			run, err := control.StartRun(ctx, discoveryapp.StartRunRequest{SourceRequest: discoveryapp.SourceRequest{WorkspaceID: workspace, SourceID: source, PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}, IdempotencyKey: "long-path-run"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `UPDATE jobs SET status='succeeded',completed_at=$2,updated_at=$2 WHERE id<>$1 AND status IN('queued','retryable')`, run.JobID.UUID(), now); err != nil {
				t.Fatal(err)
			}
			worker := jobs.NewWorker(store, jobs.ClockFunc(time.Now), jobs.BackoffFunc(func(int32) time.Duration { return 0 }), time.Minute)
			worker.Register(discoveryapp.DiscoveryJobType, control.JobHandler())
			if done, err := worker.RunOne(ctx, "snapshot-long-path"); err != nil || !done {
				t.Fatalf("run=%v err=%v", done, err)
			}
			if !observed {
				t.Fatal("valid long path never reached adapter; BeginDiscoveryRun stranded the run")
			}
			var status, coverageKey, selector, headStatus, codePath string
			var headCount int
			if err = pool.QueryRow(ctx, `SELECT r.status,u.coverage_key,u.selector,h.latest_attempt_status,COALESCE(c.path,''),(SELECT count(*) FROM source_coverage_heads WHERE workspace_id=r.workspace_id AND source_connection_id=r.source_connection_id) FROM discovery_runs r JOIN source_snapshot_runs l ON l.workspace_id=r.workspace_id AND l.run_id=r.id JOIN source_snapshot_scope u ON u.workspace_id=l.workspace_id AND u.snapshot_id=l.snapshot_id JOIN source_coverage_heads h ON h.workspace_id=u.workspace_id AND h.source_connection_id=r.source_connection_id AND h.coverage_key=u.coverage_key LEFT JOIN source_code_revisions c ON c.workspace_id=l.workspace_id AND c.source_revision_id=r.source_revision_id WHERE r.id=$1`, run.ID.UUID()).Scan(&status, &coverageKey, &selector, &headStatus, &codePath, &headCount); err != nil {
				t.Fatal(err)
			}
			wantCodePath := longPath
			if test.code == "" {
				wantCodePath = ""
			}
			if status != "succeeded" || coverageKey != attemptKey || selector != test.selector || headStatus != "complete" || codePath != wantCodePath || headCount != 1 {
				t.Fatalf("persist status=%s key=%q beginKey=%q selector=%q head=%s codePath=%q heads=%d", status, coverageKey, attemptKey, selector, headStatus, codePath, headCount)
			}
			if test.name == "sql" || test.name == "dbt" || test.name == "file-max-name" {
				if err := storage.Delete(ctx, workspace, artifact.Object.ContentDigest); err != nil {
					t.Fatal(err)
				}
				failed, err := control.StartRun(ctx, discoveryapp.StartRunRequest{SourceRequest: discoveryapp.SourceRequest{WorkspaceID: workspace, SourceID: source, PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}, IdempotencyKey: "long-path-missing-bytes"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = pool.Exec(ctx, `UPDATE jobs SET max_attempts=1 WHERE id=$1`, failed.JobID.UUID()); err != nil {
					t.Fatal(err)
				}
				if done, err := worker.RunOne(ctx, "snapshot-long-path-failure"); err != nil || !done {
					t.Fatalf("failed run=%v err=%v", done, err)
				}
				var preserved bool
				if err = pool.QueryRow(ctx, `SELECT r.status,u.coverage_key,u.selector,h.latest_attempt_status,h.latest_verified_snapshot_id=(SELECT snapshot_id FROM source_snapshot_runs WHERE workspace_id=r.workspace_id AND run_id=$2) FROM discovery_runs r JOIN source_snapshot_runs l ON l.workspace_id=r.workspace_id AND l.run_id=r.id JOIN source_snapshot_scope u ON u.workspace_id=l.workspace_id AND u.snapshot_id=l.snapshot_id JOIN source_coverage_heads h ON h.workspace_id=u.workspace_id AND h.source_connection_id=r.source_connection_id AND h.coverage_key=u.coverage_key WHERE r.id=$1`, failed.ID.UUID(), run.ID.UUID()).Scan(&status, &coverageKey, &selector, &headStatus, &preserved); err != nil {
					t.Fatal(err)
				}
				if status != "failed" || headStatus != "failed" || coverageKey != attemptKey || selector != test.selector || !preserved {
					t.Fatalf("failed attempt lost scope/history: run=%s key=%q selector=%q head=%s preserved=%v", status, coverageKey, selector, headStatus, preserved)
				}
			}
			if test.code == "" {
				return
			}
			var retained []byte
			var blob string
			if err = pool.QueryRow(ctx, `SELECT content_bytes,blob_oid FROM source_code_revisions WHERE workspace_id=$1`, workspace.UUID()).Scan(&retained, &blob); err != nil {
				t.Fatal(err)
			}
			if string(retained) != test.code || blob != digestText(test.code) {
				t.Fatalf("retained code bytes=%d want=%d digest=%s want=%s", len(retained), len(test.code), blob, digestText(test.code))
			}
			if test.name == "sql-max-upload" {
				uploads := ingestionapp.NewService(store, storage, nil, ingestionapp.ClockFunc(time.Now), map[ingestiondomain.ArtifactKind]discoverydomain.Adapter{test.kind: test.adapter})
				request := ingestionapp.StageRequest{WorkspaceID: workspace, PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", Kind: test.kind, OriginalName: "schema.sql", TrustedLogicalPath: longPath, MediaType: "application/sql", ContentLength: ingestiondomain.MaxUploadBytes + 1, Content: strings.NewReader("x"), IdempotencyKey: "overflow-length"}
				if _, err := uploads.Stage(ctx, request); !errors.Is(err, ingestiondomain.ErrInvalid) {
					t.Fatalf("upload above existing content-length limit: %v", err)
				}
				request.ContentLength, request.Content, request.IdempotencyKey = ingestiondomain.MaxUploadBytes, io.MultiReader(strings.NewReader(test.code), strings.NewReader("x")), "overflow-body"
				if _, err := uploads.Stage(ctx, request); !errors.Is(err, ingestiondomain.ErrLimitExceeded) {
					t.Fatalf("upload body above existing limit: %v", err)
				}
			}
		})
	}
}

func TestSourceSnapshotPinsHistoricalArtifactBytesDuringCleanup(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "snapshot-retention")
	principal, _ := identity.NewPrincipalID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status) VALUES($1,$2,'human','Snapshot runner','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	content := []byte("id,name\n1,Alice\n")
	artifact := artifactCommand(t, workspace, &principal, content, now)
	storage, err := localartifacts.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = storage.Put(ctx, workspace, artifact.Object.ContentDigest, bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatal(err)
	}
	if err = store.ReserveArtifactObject(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateArtifact(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	source, _ := identity.NewSourceConnectionID()
	set := artifactSetCommand(t, workspace, source, principal, artifact, "snapshot-set", now, nil, now.Add(24*time.Hour))
	if _, err = store.FinalizeArtifactSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	loader, _ := discoveryapp.NewArtifactLoader("")
	control := discoveryapp.NewControlService(store, nil, nil, loader, nil, nil, discoveryapp.ClockFunc(time.Now)).WithArtifactStore(storage, map[ingestiondomain.ArtifactKind]discoverydomain.Adapter{ingestiondomain.ArtifactCSV: fileartifact.CSV()})
	run, err := control.StartRun(ctx, discoveryapp.StartRunRequest{SourceRequest: discoveryapp.SourceRequest{WorkspaceID: workspace, SourceID: source, PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}, IdempotencyKey: "snapshot-retained-run"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE jobs SET status='succeeded',completed_at=$2,updated_at=$2 WHERE id<>$1 AND status IN('queued','retryable')`, run.JobID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	worker := jobs.NewWorker(store, jobs.ClockFunc(time.Now), jobs.BackoffFunc(func(int32) time.Duration { return 0 }), time.Minute)
	worker.Register(discoveryapp.DiscoveryJobType, control.JobHandler())
	if done, err := worker.RunOne(ctx, "snapshot-retention"); err != nil || !done {
		t.Fatalf("run=%v err=%v", done, err)
	}
	var snapshots int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM source_snapshot_runs WHERE workspace_id=$1 AND run_id=$2`, workspace.UUID(), run.ID.UUID()).Scan(&snapshots); err != nil || snapshots != 1 {
		t.Fatalf("snapshot count=%d err=%v", snapshots, err)
	}
	current, err := store.GetDiscoverySource(ctx, workspace, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateDiscoverySource(ctx, discoveryapp.UpdateSourceCommand{WorkspaceID: workspace, SourceID: source, Name: current.Name, Status: "deleted", ExpectedVersion: current.Version, Actor: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", UpdatedAt: now, ContentRetentionUntil: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	orphanContent := []byte("unreferenced-stage")
	orphan := artifactCommand(t, workspace, &principal, orphanContent, now)
	if err = storage.Put(ctx, workspace, orphan.Object.ContentDigest, bytes.NewReader(orphanContent), int64(len(orphanContent))); err != nil {
		t.Fatal(err)
	}
	if err = store.ReserveArtifactObject(ctx, orphan); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateArtifact(ctx, orphan); err != nil {
		t.Fatal(err)
	}
	deleted := []string{}
	_, err = store.CleanupArtifactObjects(ctx, now.Add(60*24*time.Hour), 100, func(ctx context.Context, object ingestiondomain.CleanupObject) error {
		if object.WorkspaceID == workspace {
			deleted = append(deleted, object.ContentDigest)
		}
		return storage.Delete(ctx, object.WorkspaceID, object.ContentDigest)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != orphan.Object.ContentDigest {
		t.Fatalf("cleanup deleted=%v, must delete only unreferenced object", deleted)
	}
	reader, err := storage.Open(ctx, workspace, artifact.Object.ContentDigest)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(retained, content) {
		t.Fatalf("retained bytes=%q err=%v", retained, err)
	}
}

func TestSourceSnapshotWorkerFailureInvalidatesFreshnessWithoutLosingHistory(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "snapshot-worker-failure")
	principal, _ := identity.NewPrincipalID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status) VALUES($1,$2,'human','Snapshot runner','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	content := []byte("id,name\n1,Alice\n")
	artifact := artifactCommand(t, workspace, &principal, content, now)
	storage, err := localartifacts.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = storage.Put(ctx, workspace, artifact.Object.ContentDigest, bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatal(err)
	}
	if err = store.ReserveArtifactObject(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateArtifact(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	source, _ := identity.NewSourceConnectionID()
	set := artifactSetCommand(t, workspace, source, principal, artifact, "worker-failure-set", now, nil, now.Add(24*time.Hour))
	if _, err = store.FinalizeArtifactSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	loader, _ := discoveryapp.NewArtifactLoader("")
	control := discoveryapp.NewControlService(store, nil, nil, loader, nil, nil, discoveryapp.ClockFunc(time.Now)).WithArtifactStore(storage, map[ingestiondomain.ArtifactKind]discoverydomain.Adapter{ingestiondomain.ArtifactCSV: fileartifact.CSV()})
	worker := jobs.NewWorker(store, jobs.ClockFunc(time.Now), jobs.BackoffFunc(func(int32) time.Duration { return 0 }), time.Minute)
	worker.Register(discoveryapp.DiscoveryJobType, control.JobHandler())
	start := func(key string) discoverydomain.Run {
		t.Helper()
		run, err := control.StartRun(ctx, discoveryapp.StartRunRequest{SourceRequest: discoveryapp.SourceRequest{WorkspaceID: workspace, SourceID: source, PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}, IdempotencyKey: key})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `UPDATE jobs SET status='succeeded',completed_at=$2,updated_at=$2 WHERE id<>$1 AND status IN('queued','retryable')`, run.JobID.UUID(), time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		return run
	}
	first := start("worker-first")
	if done, err := worker.RunOne(ctx, "snapshot-worker"); err != nil || !done {
		t.Fatalf("first done=%v err=%v", done, err)
	}
	var original string
	if err := pool.QueryRow(ctx, `SELECT snapshot_id::text FROM source_snapshot_runs WHERE run_id=$1`, first.ID.UUID()).Scan(&original); err != nil {
		t.Fatal(err)
	}
	repeated := start("worker-repeated")
	if done, err := worker.RunOne(ctx, "snapshot-worker"); err != nil || !done {
		t.Fatalf("repeated done=%v err=%v", done, err)
	}
	var repeatedSnapshot string
	if err := pool.QueryRow(ctx, `SELECT snapshot_id::text FROM source_snapshot_runs WHERE run_id=$1`, repeated.ID.UUID()).Scan(&repeatedSnapshot); err != nil || repeatedSnapshot != original {
		t.Fatalf("run replay snapshot=%s want=%s err=%v", repeatedSnapshot, original, err)
	}
	if err := storage.Delete(ctx, workspace, artifact.Object.ContentDigest); err != nil {
		t.Fatal(err)
	}
	failed := start("worker-missing-bytes")
	if _, err := pool.Exec(ctx, `UPDATE jobs SET max_attempts=1 WHERE id=$1`, failed.JobID.UUID()); err != nil {
		t.Fatal(err)
	}
	if done, err := worker.RunOne(ctx, "snapshot-worker"); err != nil || !done {
		t.Fatalf("failure done=%v err=%v", done, err)
	}
	var runStatus, quality, status, latest, verified, effective string
	err = pool.QueryRow(ctx, `SELECT r.status,s.history_quality,s.coverage_status,h.latest_attempt_status,h.latest_verified_snapshot_id::text,e.snapshot_id::text FROM discovery_runs r JOIN source_snapshot_runs l ON l.workspace_id=r.workspace_id AND l.run_id=r.id JOIN source_snapshots s ON s.workspace_id=l.workspace_id AND s.id=l.snapshot_id JOIN source_coverage_heads h ON h.workspace_id=r.workspace_id AND h.source_connection_id=r.source_connection_id JOIN source_effective_snapshots e ON e.workspace_id=r.workspace_id AND e.source_connection_id=r.source_connection_id WHERE r.id=$1`, failed.ID.UUID()).Scan(&runStatus, &quality, &status, &latest, &verified, &effective)
	if err != nil || runStatus != "failed" || quality != "verified" || status != "failed" || latest != "failed" || verified != original || effective != original {
		t.Fatalf("failed snapshot run=%s quality=%s coverage=%s latest=%s verified=%s effective=%s err=%v", runStatus, quality, status, latest, verified, effective, err)
	}
	loaded, err := store.LoadDiscoveryRunExecution(ctx, workspace, failed.ID)
	if err != nil || loaded.Run.SnapshotID == nil {
		t.Fatalf("failed run snapshot ID=%+v err=%v", loaded, err)
	}
}
