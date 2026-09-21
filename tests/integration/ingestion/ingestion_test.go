package ingestion_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	localartifacts "github.com/iiwish/semlia/internal/adapters/artifacts/local"
	fileartifact "github.com/iiwish/semlia/internal/adapters/discovery/files"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	ingestionappdiscovery "github.com/iiwish/semlia/internal/application/discovery"
	ingestionapp "github.com/iiwish/semlia/internal/application/ingestion"
	"github.com/iiwish/semlia/internal/application/jobs"
	discoverydomain "github.com/iiwish/semlia/internal/domain/discovery"
	ingestiondomain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var databaseURL string

func TestMain(testingMain *testing.M) {
	if explicitURL := os.Getenv("SEMLIA_INGESTION_TEST_DATABASE_URL"); explicitURL != "" {
		parsed, err := url.Parse(explicitURL)
		name := strings.TrimPrefix(parsedPath(parsed), "/")
		if err != nil || !strings.HasPrefix(name, "semlia_") || !strings.HasSuffix(name, "_test") || strings.Contains(name, "/") {
			fmt.Fprintln(os.Stderr, "explicit ingestion test database must be a dedicated semlia_*_test database")
			os.Exit(1)
		}
		databaseURL = explicitURL
		migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repositoryRoot(), "migrations"))
		if err != nil || migrator.Up() != nil {
			fmt.Fprintln(os.Stderr, "migrate explicit ingestion test database: failed")
			os.Exit(1)
		}
		_ = migrator.Close()
		os.Exit(testingMain.Run())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("semlia_ingestion_test"), tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"), tcpostgres.BasicWaitStrategies())
	if err != nil {
		fmt.Fprintln(os.Stderr, "start ingestion PostgreSQL container: failed")
		os.Exit(1)
	}
	databaseURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		os.Exit(1)
	}
	migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repositoryRoot(), "migrations"))
	if err != nil || migrator.Up() != nil {
		_ = testcontainers.TerminateContainer(container)
		os.Exit(1)
	}
	_ = migrator.Close()
	code := testingMain.Run()
	if err := testcontainers.TerminateContainer(container); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}

func parsedPath(value *url.URL) string {
	if value == nil {
		return ""
	}
	return value.Path
}

func TestArtifactCleanupHoldsFenceAcrossPhysicalDelete(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "cleanup-fence")
	command := artifactCommand(t, workspace, nil, []byte("fenced-object"), time.Now().UTC().Add(-2*time.Hour))
	if err := store.ReserveArtifactObject(ctx, command); err != nil {
		t.Fatal(err)
	}

	deleteEntered := make(chan struct{})
	allowDelete := make(chan struct{})
	cleanupDone := make(chan error, 1)
	go func() {
		_, err := store.CleanupArtifactObjects(ctx, time.Now().UTC(), 10, func(context.Context, ingestiondomain.CleanupObject) error {
			close(deleteEntered)
			<-allowDelete
			return nil
		})
		cleanupDone <- err
	}()
	<-deleteEntered

	reviveDone := make(chan error, 1)
	revive := artifactCommand(t, workspace, nil, []byte("fenced-object"), time.Now().UTC())
	go func() { reviveDone <- store.ReserveArtifactObject(ctx, revive) }()
	select {
	case err := <-reviveDone:
		t.Fatalf("reservation crossed the in-flight delete fence: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	secondDeletes := 0
	secondCompleted, err := store.CleanupArtifactObjects(ctx, time.Now().UTC(), 10, func(context.Context, ingestiondomain.CleanupObject) error {
		secondDeletes++
		return nil
	})
	if err != nil || secondCompleted != 0 || secondDeletes != 0 {
		t.Fatalf("second cleanup claimed in-flight object: completed=%d deletes=%d err=%v", secondCompleted, secondDeletes, err)
	}
	close(allowDelete)
	if err := <-cleanupDone; err != nil {
		t.Fatal(err)
	}
	if err := <-reviveDone; err != nil {
		t.Fatalf("post-delete reservation failed: %v", err)
	}
	assertRetention(t, pool, workspace, command.Object.ContentDigest, "reserved", 1, 1)
}

func TestArtifactCleanupRetriesAfterPhysicalDeleteTransactionRollback(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "cleanup-retry")
	command := artifactCommand(t, workspace, nil, []byte("retry-object"), time.Now().UTC().Add(-2*time.Hour))
	if err := store.ReserveArtifactObject(ctx, command); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CleanupArtifactObjects(ctx, time.Now().UTC(), 10, func(context.Context, ingestiondomain.CleanupObject) error {
		return ingestiondomain.ErrStore
	}); !errors.Is(err, ingestiondomain.ErrStore) {
		t.Fatalf("failed delete = %v", err)
	}
	assertRetention(t, pool, workspace, command.Object.ContentDigest, "reserved", 1, 1)
	completed, err := store.CleanupArtifactObjects(ctx, time.Now().UTC().Add(2*time.Minute), 10, func(context.Context, ingestiondomain.CleanupObject) error { return nil })
	if err != nil || completed != 1 {
		t.Fatalf("cleanup retry: completed=%d err=%v", completed, err)
	}
	assertRetention(t, pool, workspace, command.Object.ContentDigest, "deleted", 0, 0)
}

func TestArtifactCleanupFailureDoesNotRollbackOrBlockOtherObjects(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "cleanup-isolation")
	principal, _ := identity.NewPrincipalID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status)
VALUES($1,$2,'human','Uploader','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Add(-48 * time.Hour)
	first := artifactCommand(t, workspace, &principal, []byte("cleanup-first"), base)
	second := artifactCommand(t, workspace, &principal, []byte("cleanup-second"), base.Add(time.Minute))
	for _, command := range []ingestionapp.CreateArtifactCommand{first, second} {
		if err := store.ReserveArtifactObject(ctx, command); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateArtifact(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	if _, err := store.CleanupArtifactObjects(ctx, now, 100, func(_ context.Context, object ingestiondomain.CleanupObject) error {
		if object.ContentDigest == first.Object.ContentDigest {
			return ingestiondomain.ErrStore
		}
		return nil
	}); !errors.Is(err, ingestiondomain.ErrStore) {
		t.Fatalf("first cleanup error=%v", err)
	}
	deletedSecond := false
	if _, err := store.CleanupArtifactObjects(ctx, now, 100, func(_ context.Context, object ingestiondomain.CleanupObject) error {
		if object.ContentDigest == second.Object.ContentDigest {
			deletedSecond = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !deletedSecond {
		t.Fatal("later cleanup candidate remained blocked by failed object")
	}
	assertRetention(t, pool, workspace, first.Object.ContentDigest, "retained", 0, 1)
}

func TestExpiredDuplicateStageDoesNotDeleteCanonicalObject(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "cleanup-canonical")
	principal, _ := identity.NewPrincipalID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status) VALUES($1,$2,'human','Uploader','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	content := []byte("canonical-object")
	first := artifactCommand(t, workspace, &principal, content, time.Now().UTC().Add(-48*time.Hour))
	if err := store.ReserveArtifactObject(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateArtifact(ctx, first); err != nil {
		t.Fatal(err)
	}
	source, _ := identity.NewSourceConnectionID()
	if _, err := pool.Exec(ctx, `INSERT INTO source_connections
(id,workspace_id,adapter_kind,name,normalized_locator,status,metadata,source_kind,version)
VALUES($1,$2,'file_catalog','Canonical file','artifact:test','paused','{}'::jsonb,'file',1)`, source.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE source_artifacts SET status='validated',source_connection_id=$2,finalized_at=clock_timestamp(),expires_at=NULL WHERE id=$1`, first.Artifact.ID.UUID(), source.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE artifact_object_retention SET expires_at=NULL WHERE workspace_id=$1 AND content_sha256=$2`, workspace.UUID(), first.Object.ContentDigest); err != nil {
		t.Fatal(err)
	}
	second := artifactCommand(t, workspace, &principal, content, time.Now().UTC().Add(-48*time.Hour))
	if err := store.ReserveArtifactObject(ctx, second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateArtifact(ctx, second); err != nil {
		t.Fatal(err)
	}
	deleted := false
	completed, err := store.CleanupArtifactObjects(ctx, time.Now().UTC(), 10, func(context.Context, ingestiondomain.CleanupObject) error {
		deleted = true
		return nil
	})
	if err != nil || completed != 1 || deleted {
		t.Fatalf("canonical object cleanup: completed=%d deleted=%t err=%v", completed, deleted, err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM source_artifacts WHERE id=$1`, second.Artifact.ID.UUID()).Scan(&status); err != nil || status != "rejected" {
		t.Fatalf("expired duplicate status=%q err=%v", status, err)
	}
	assertRetention(t, pool, workspace, first.Object.ContentDigest, "retained", 0, 1)
}

func TestContentAddressedObjectDeduplicatesAcrossArtifactMediaKinds(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "cross-media-dedupe")
	principal, _ := identity.NewPrincipalID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status)
VALUES($1,$2,'human','Uploader','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	content := []byte("name\nvalue\n")
	first := artifactCommand(t, workspace, &principal, content, time.Now().UTC())
	second := artifactCommand(t, workspace, &principal, content, time.Now().UTC().Add(time.Second))
	second.Artifact.Kind = ingestiondomain.ArtifactMarkdown
	second.Artifact.MediaType = "text/markdown"
	second.Artifact.OriginalName = "input.md"
	second.Object.MediaType = "text/markdown"
	for _, command := range []ingestionapp.CreateArtifactCommand{first, second} {
		if err := store.ReserveArtifactObject(ctx, command); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateArtifact(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	var objects, artifacts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM artifact_objects WHERE workspace_id=$1`, workspace.UUID()).Scan(&objects); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM source_artifacts WHERE workspace_id=$1`, workspace.UUID()).Scan(&artifacts); err != nil {
		t.Fatal(err)
	}
	if objects != 1 || artifacts != 2 {
		t.Fatalf("objects=%d artifacts=%d", objects, artifacts)
	}
}

func TestSourceScopedArtifactStageFencesVersionAndExpiresOnce(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "source-scoped-stage")
	principal, _ := identity.NewPrincipalID()
	source, _ := identity.NewSourceConnectionID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status)
VALUES($1,$2,'human','Uploader','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_connections
(id,workspace_id,adapter_kind,name,normalized_locator,status,metadata,source_kind,version)
VALUES($1,$2,'file_catalog','Scoped file','artifact:test','paused','{}'::jsonb,'file',1)`, source.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	version := int64(1)
	command := artifactCommand(t, workspace, &principal, []byte("id\n1\n"), time.Now().UTC().Add(-48*time.Hour))
	command.Artifact.SourceConnectionID = &source
	command.ExpectedSourceVersion = &version
	if err := store.ReserveArtifactObject(ctx, command); err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateArtifact(ctx, command)
	if err != nil || created.SourceConnectionID == nil || *created.SourceConnectionID != source {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE source_connections SET version=2 WHERE id=$1`, source.UUID()); err != nil {
		t.Fatal(err)
	}
	stale := artifactCommand(t, workspace, &principal, []byte("id\n2\n"), time.Now().UTC())
	stale.Artifact.SourceConnectionID = &source
	stale.ExpectedSourceVersion = &version
	if err := store.ReserveArtifactObject(ctx, stale); !errors.Is(err, ingestiondomain.ErrConflict) {
		t.Fatalf("stale source reservation error=%v", err)
	}

	deleted := 0
	processed, err := store.CleanupArtifactObjects(ctx, time.Now().UTC(), 10, func(context.Context, ingestiondomain.CleanupObject) error {
		deleted++
		return nil
	})
	if err != nil || processed != 1 || deleted != 1 {
		t.Fatalf("first cleanup processed=%d deleted=%d err=%v", processed, deleted, err)
	}
	processed, err = store.CleanupArtifactObjects(ctx, time.Now().UTC(), 10, func(context.Context, ingestiondomain.CleanupObject) error {
		deleted++
		return nil
	})
	if err != nil || processed != 0 || deleted != 1 {
		t.Fatalf("repeated cleanup processed=%d deleted=%d err=%v", processed, deleted, err)
	}
	var status, failure string
	var storedSource string
	if err := pool.QueryRow(ctx, `SELECT status,failure_code,source_connection_id::text FROM source_artifacts WHERE id=$1`,
		command.Artifact.ID.UUID()).Scan(&status, &failure, &storedSource); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" || failure != "ARTIFACT_EXPIRED" || storedSource != source.UUID() {
		t.Fatalf("expired artifact status=%s failure=%s source=%s", status, failure, storedSource)
	}
	assertRetention(t, pool, workspace, command.Object.ContentDigest, "deleted", 0, 0)
}

func TestSourceScopedUploadFinalizesOnlyIntoItsAuthorizedSource(t *testing.T) {
	ctx := context.Background()
	pool, repository := openStore(t)
	workspace := createWorkspace(t, pool, "source-scoped-finalize")
	principal, _ := identity.NewPrincipalID()
	source, _ := identity.NewSourceConnectionID()
	otherSource, _ := identity.NewSourceConnectionID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status)
VALUES($1,$2,'human','Uploader','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	for id, name := range map[identity.SourceConnectionID]string{source: "Allowed", otherSource: "Other"} {
		if _, err := pool.Exec(ctx, `INSERT INTO source_connections
(id,workspace_id,adapter_kind,name,normalized_locator,status,metadata,source_kind,version)
VALUES($1,$2,'file_catalog',$3,$4,'paused','{}'::jsonb,'file',1)`, id.UUID(), workspace.UUID(), name, "artifact:"+id.String()); err != nil {
			t.Fatal(err)
		}
	}
	artifactStore, err := localartifacts.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := ingestionapp.NewService(repository, artifactStore, nil, ingestionapp.ClockFunc(time.Now),
		map[ingestiondomain.ArtifactKind]discoverydomain.Adapter{ingestiondomain.ArtifactCSV: fileartifact.CSV()})
	version := int64(1)
	content := []byte("id,name\n1,Alice\n")
	artifact, err := service.Stage(ctx, ingestionapp.StageRequest{WorkspaceID: workspace, SourceID: &source,
		ExpectedSourceVersion: &version, Kind: ingestiondomain.ArtifactCSV, OriginalName: "people.csv", MediaType: "text/csv",
		Content: bytes.NewReader(content), ContentLength: int64(len(content)), IdempotencyKey: "source-upload",
		PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Finalize(ctx, ingestionapp.FinalizeRequest{WorkspaceID: workspace, SourceName: "Other",
		SourceID: &otherSource, ExpectedSourceVersion: &version, ArtifactIDs: []identity.ArtifactID{artifact.ID},
		IdempotencyKey: "foreign-finalize", PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}); !errors.Is(err, ingestiondomain.ErrConflict) {
		t.Fatalf("foreign-source finalize error=%v", err)
	}
	set, err := service.Finalize(ctx, ingestionapp.FinalizeRequest{WorkspaceID: workspace, SourceName: "Allowed",
		SourceID: &source, ExpectedSourceVersion: &version, ArtifactIDs: []identity.ArtifactID{artifact.ID},
		IdempotencyKey: "allowed-finalize", PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err != nil || set.SourceConnectionID != source {
		t.Fatalf("set=%+v err=%v", set, err)
	}
	preview, err := service.Preview(ctx, workspace, set.ID, artifact.ID, principal.String(), "4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil || len(preview.Sheets) != 1 || preview.Sheets[0].Rows[0].Cells[1] != "Alice" || preview.ContentDigest != artifact.ContentDigest {
		t.Fatalf("immutable preview=%+v err=%v", preview, err)
	}
	foreignWorkspace := createWorkspace(t, pool, "preview-foreign")
	if _, err := service.Preview(ctx, foreignWorkspace, set.ID, artifact.ID, principal.String(), ""); !errors.Is(err, ingestiondomain.ErrNotFound) {
		t.Fatalf("cross workspace preview=%v", err)
	}
	version = 2
	unsafe := []byte("name\n=cmd\n")
	rejected, err := service.Stage(ctx, ingestionapp.StageRequest{WorkspaceID: workspace, SourceID: &source,
		ExpectedSourceVersion: &version, Kind: ingestiondomain.ArtifactCSV, OriginalName: "unsafe.csv", MediaType: "text/csv",
		Content: bytes.NewReader(unsafe), ContentLength: int64(len(unsafe)), IdempotencyKey: "source-rejected",
		PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if !errors.Is(err, ingestiondomain.ErrUnsafeContent) || rejected.SourceConnectionID == nil || *rejected.SourceConnectionID != source {
		t.Fatalf("rejected=%+v err=%v", rejected, err)
	}
	page, err := service.List(ctx, ingestionapp.ListArtifactsQuery{WorkspaceID: workspace, SourceID: &source, Limit: 10}, principal.String(), "4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil || page.Total != 2 {
		t.Fatalf("source page=%+v err=%v", page, err)
	}
	otherPage, err := service.List(ctx, ingestionapp.ListArtifactsQuery{WorkspaceID: workspace, SourceID: &otherSource, Limit: 10}, principal.String(), "4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil || otherPage.Total != 0 {
		t.Fatalf("other source page=%+v err=%v", otherPage, err)
	}
}

func TestSharedDigestCleanupWaitsForLatestStagedDeadline(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "staggered-stage-expiry")
	principal, _ := identity.NewPrincipalID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status)
VALUES($1,$2,'human','Uploader','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Second)
	content := []byte("id\n1\n")
	first := artifactCommand(t, workspace, &principal, content, base)
	second := artifactCommand(t, workspace, &principal, content, base.Add(time.Minute))
	firstExpiry, secondExpiry := base.Add(24*time.Hour), base.Add(30*24*time.Hour)
	first.Artifact.ExpiresAt, second.Artifact.ExpiresAt = &firstExpiry, &secondExpiry
	for _, command := range []ingestionapp.CreateArtifactCommand{first, second} {
		if err := store.ReserveArtifactObject(ctx, command); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateArtifact(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	deleted := 0
	_, err := store.CleanupArtifactObjects(ctx, firstExpiry.Add(time.Minute), 100, func(_ context.Context, object ingestiondomain.CleanupObject) error {
		if object.WorkspaceID == workspace && object.ContentDigest == first.Object.ContentDigest {
			deleted++
		}
		return nil
	})
	if err != nil || deleted != 0 {
		t.Fatalf("early cleanup deleted=%d err=%v", deleted, err)
	}
	assertRetention(t, pool, workspace, first.Object.ContentDigest, "retained", 0, 1)
	var firstStatus, secondStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM source_artifacts WHERE id=$1`, first.Artifact.ID.UUID()).Scan(&firstStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM source_artifacts WHERE id=$1`, second.Artifact.ID.UUID()).Scan(&secondStatus); err != nil {
		t.Fatal(err)
	}
	if firstStatus != "rejected" || secondStatus != "uploaded" {
		t.Fatalf("early statuses first=%s second=%s", firstStatus, secondStatus)
	}
	_, err = store.CleanupArtifactObjects(ctx, secondExpiry.Add(time.Minute), 100, func(_ context.Context, object ingestiondomain.CleanupObject) error {
		if object.WorkspaceID == workspace && object.ContentDigest == first.Object.ContentDigest {
			deleted++
		}
		return nil
	})
	if err != nil || deleted != 1 {
		t.Fatalf("final cleanup deleted=%d err=%v", deleted, err)
	}
	assertRetention(t, pool, workspace, first.Object.ContentDigest, "deleted", 0, 0)
}

func TestSupersededArtifactContentExpiresWhileMetadataAndActiveSetRemain(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "superseded-retention")
	principal, _ := identity.NewPrincipalID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status) VALUES($1,$2,'human','Uploader','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	first := artifactCommand(t, workspace, &principal, []byte("version-one"), now.Add(-72*time.Hour))
	if err := store.ReserveArtifactObject(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateArtifact(ctx, first); err != nil {
		t.Fatal(err)
	}
	source, _ := identity.NewSourceConnectionID()
	firstSet := artifactSetCommand(t, workspace, source, principal, first, "set-one", now.Add(-71*time.Hour), nil, now.Add(-24*time.Hour))
	if _, err := store.FinalizeArtifactSet(ctx, firstSet); err != nil {
		t.Fatal(err)
	}

	second := artifactCommand(t, workspace, &principal, []byte("version-two"), now.Add(-48*time.Hour))
	if err := store.ReserveArtifactObject(ctx, second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateArtifact(ctx, second); err != nil {
		t.Fatal(err)
	}
	version := int64(1)
	secondSet := artifactSetCommand(t, workspace, source, principal, second, "set-two", now.Add(-47*time.Hour), &version, now.Add(-23*time.Hour))
	if _, err := store.FinalizeArtifactSet(ctx, secondSet); err != nil {
		t.Fatal(err)
	}
	reupload := artifactCommand(t, workspace, &principal, []byte("version-one"), now)
	if err := store.ReserveArtifactObject(ctx, reupload); err != nil {
		t.Fatal(err)
	}
	protectedDeletes := 0
	if processed, err := store.CleanupArtifactObjects(ctx, now, 10, func(_ context.Context, _ ingestiondomain.CleanupObject) error {
		protectedDeletes++
		return nil
	}); err != nil || processed != 0 || protectedDeletes != 0 {
		t.Fatalf("reupload protection processed=%d deletes=%d err=%v", processed, protectedDeletes, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE artifact_object_retention SET expires_at=$3 WHERE workspace_id=$1 AND content_sha256=$2`,
		workspace.UUID(), first.Object.ContentDigest, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	deleted := make([]string, 0, 1)
	processed, err := store.CleanupArtifactObjects(ctx, now, 10, func(_ context.Context, object ingestiondomain.CleanupObject) error {
		deleted = append(deleted, object.ContentDigest)
		return nil
	})
	if err != nil || processed != 1 || len(deleted) != 1 || deleted[0] != first.Object.ContentDigest {
		t.Fatalf("cleanup processed=%d deleted=%v err=%v", processed, deleted, err)
	}
	oldArtifact, err := store.GetArtifact(ctx, workspace, first.Artifact.ID)
	if err != nil || oldArtifact.ContentDigest != first.Object.ContentDigest || oldArtifact.ContentAvailability != ingestiondomain.ContentExpired {
		t.Fatalf("old metadata availability=%q digest=%q err=%v", oldArtifact.ContentAvailability, oldArtifact.ContentDigest, err)
	}
	activeSet, err := store.GetArtifactSet(ctx, workspace, secondSet.Set.ID, 0)
	if err != nil || len(activeSet.Members) != 1 || activeSet.Members[0].ContentAvailability != ingestiondomain.ContentAvailable {
		t.Fatalf("active set=%+v err=%v", activeSet, err)
	}
	assertRetention(t, pool, workspace, first.Object.ContentDigest, "deleted", 0, 1)
	assertRetention(t, pool, workspace, second.Object.ContentDigest, "retained", 0, 1)
}

func TestQueuedRunPinProtectsSupersededRawContentUntilTerminal(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "run-pin-retention")
	principal, _ := identity.NewPrincipalID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status) VALUES($1,$2,'human','Runner','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	first := artifactCommand(t, workspace, &principal, []byte("run-pinned-v1"), now.Add(-72*time.Hour))
	if err := store.ReserveArtifactObject(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateArtifact(ctx, first); err != nil {
		t.Fatal(err)
	}
	source, _ := identity.NewSourceConnectionID()
	firstSet := artifactSetCommand(t, workspace, source, principal, first, "run-pin-set-one", now.Add(-71*time.Hour), nil, now.Add(-24*time.Hour))
	if _, err := store.FinalizeArtifactSet(ctx, firstSet); err != nil {
		t.Fatal(err)
	}
	runID, _ := identity.NewRunID()
	jobID, _ := identity.NewRunID()
	if _, err := store.CreateQueuedDiscoveryRun(ctx, ingestionappdiscovery.CreateRunCommand{
		RunID: runID, JobID: jobID, WorkspaceID: workspace, SourceID: source, RequestedBy: principal.String(),
		IdempotencyKey: "run-pin", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", CreatedAt: now.Add(-48 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	second := artifactCommand(t, workspace, &principal, []byte("run-pinned-v2"), now.Add(-47*time.Hour))
	if err := store.ReserveArtifactObject(ctx, second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateArtifact(ctx, second); err != nil {
		t.Fatal(err)
	}
	version := int64(1)
	secondSet := artifactSetCommand(t, workspace, source, principal, second, "run-pin-set-two", now.Add(-46*time.Hour), &version, now.Add(-time.Hour))
	if _, err := store.FinalizeArtifactSet(ctx, secondSet); err != nil {
		t.Fatal(err)
	}
	deleted := 0
	if processed, err := store.CleanupArtifactObjects(ctx, now, 10, func(context.Context, ingestiondomain.CleanupObject) error { deleted++; return nil }); err != nil || processed != 0 || deleted != 0 {
		t.Fatalf("queued pin cleanup processed=%d deleted=%d err=%v", processed, deleted, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE discovery_runs SET status='failed',started_at=$2,completed_at=$2,error_code='TEST_TERMINAL',updated_at=$2 WHERE id=$1`, runID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if processed, err := store.CleanupArtifactObjects(ctx, now, 10, func(context.Context, ingestiondomain.CleanupObject) error { deleted++; return nil }); err != nil || processed != 1 || deleted != 1 {
		t.Fatalf("terminal pin cleanup processed=%d deleted=%d err=%v", processed, deleted, err)
	}
}

func TestSharedDigestRetentionStartsAtLastSourceReferenceRemoval(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "shared-retention")
	principal, _ := identity.NewPrincipalID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status) VALUES($1,$2,'human','Owner','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Second)
	content := []byte("id\n1\n")
	first := artifactCommand(t, workspace, &principal, content, base)
	second := artifactCommand(t, workspace, &principal, content, base.Add(time.Minute))
	for _, command := range []ingestionapp.CreateArtifactCommand{first, second} {
		if err := store.ReserveArtifactObject(ctx, command); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateArtifact(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	sourceA, _ := identity.NewSourceConnectionID()
	sourceB, _ := identity.NewSourceConnectionID()
	if _, err := store.FinalizeArtifactSet(ctx, artifactSetCommand(t, workspace, sourceA, principal, first, "shared-a", base.Add(2*time.Minute), nil, base.Add(30*24*time.Hour))); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinalizeArtifactSet(ctx, artifactSetCommand(t, workspace, sourceB, principal, second, "shared-b", base.Add(3*time.Minute), nil, base.Add(30*24*time.Hour))); err != nil {
		t.Fatal(err)
	}
	deleteA := base.Add(24 * time.Hour)
	if _, err := store.UpdateDiscoverySource(ctx, ingestionappdiscovery.UpdateSourceCommand{WorkspaceID: workspace,
		SourceID: sourceA, Name: "Versioned file", Status: "deleted", ExpectedVersion: 1, UpdatedAt: deleteA,
		Actor: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", ContentRetentionUntil: deleteA.Add(30 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	deleteB := base.Add(40 * 24 * time.Hour)
	if _, err := store.UpdateDiscoverySource(ctx, ingestionappdiscovery.UpdateSourceCommand{WorkspaceID: workspace,
		SourceID: sourceB, Name: "Versioned file", Status: "deleted", ExpectedVersion: 1, UpdatedAt: deleteB,
		Actor: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", ContentRetentionUntil: deleteB.Add(30 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var expiresAt time.Time
	if err := pool.QueryRow(ctx, `SELECT expires_at FROM artifact_object_retention WHERE workspace_id=$1 AND content_sha256=$2`,
		workspace.UUID(), first.Object.ContentDigest).Scan(&expiresAt); err != nil {
		t.Fatal(err)
	}
	if !expiresAt.Equal(deleteB.Add(30 * 24 * time.Hour)) {
		t.Fatalf("shared digest expires_at=%s, want %s", expiresAt, deleteB.Add(30*24*time.Hour))
	}
}

func TestArtifactDiscoveryWorkerDoesNotRequireCredentialSecret(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "artifact-worker-no-secret")
	principal, _ := identity.NewPrincipalID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status) VALUES($1,$2,'human','Runner','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	content := []byte("id,name\n1,Alice\n")
	artifact := artifactCommand(t, workspace, &principal, content, now)
	artifactStore, err := localartifacts.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := artifactStore.Put(ctx, workspace, artifact.Object.ContentDigest, bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveArtifactObject(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateArtifact(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	otherContent := []byte("id,name\n2,Bob\n")
	other := artifactCommand(t, workspace, &principal, otherContent, now.Add(time.Second))
	if err := artifactStore.Put(ctx, workspace, other.Object.ContentDigest, bytes.NewReader(otherContent), int64(len(otherContent))); err != nil {
		t.Fatal(err)
	}
	if err := store.ReserveArtifactObject(ctx, other); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateArtifact(ctx, other); err != nil {
		t.Fatal(err)
	}
	source, _ := identity.NewSourceConnectionID()
	set := artifactSetCommand(t, workspace, source, principal, artifact, "no-secret-set", now, nil, now.Add(30*24*time.Hour))
	if _, err := store.FinalizeArtifactSet(ctx, set); err != nil {
		t.Fatal(err)
	}
	runID, _ := identity.NewRunID()
	jobID, _ := identity.NewRunID()
	if _, err := store.CreateQueuedDiscoveryRun(ctx, ingestionappdiscovery.CreateRunCommand{
		RunID: runID, JobID: jobID, WorkspaceID: workspace, SourceID: source, RequestedBy: principal.String(),
		IdempotencyKey: "no-secret-run", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET status='succeeded',completed_at=$2,updated_at=$2 WHERE id<>$1 AND status IN('queued','retryable')`, jobID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	loader, err := ingestionappdiscovery.NewArtifactLoader("")
	if err != nil {
		t.Fatal(err)
	}
	control := ingestionappdiscovery.NewControlService(store, nil, nil, loader, nil, nil,
		ingestionappdiscovery.ClockFunc(time.Now)).WithArtifactStore(artifactStore,
		map[ingestiondomain.ArtifactKind]discoverydomain.Adapter{ingestiondomain.ArtifactCSV: fileartifact.CSV()})
	worker := jobs.NewWorker(store, jobs.ClockFunc(time.Now), jobs.BackoffFunc(func(int32) time.Duration { return 0 }), time.Minute)
	worker.Register(ingestionappdiscovery.DiscoveryJobType, control.JobHandler())
	processed, err := worker.RunOne(ctx, "artifact-worker-no-secret")
	if err != nil || !processed {
		t.Fatalf("artifact worker processed=%v err=%v", processed, err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM discovery_runs WHERE workspace_id=$1 AND id=$2`, workspace.UUID(), runID.UUID()).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" && status != "degraded" {
		t.Fatalf("artifact discovery status=%s", status)
	}
	var consumed, untouched string
	if err := pool.QueryRow(ctx, `SELECT status FROM source_artifacts WHERE id=$1`, artifact.Artifact.ID.UUID()).Scan(&consumed); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM source_artifacts WHERE id=$1`, other.Artifact.ID.UUID()).Scan(&untouched); err != nil {
		t.Fatal(err)
	}
	if consumed != "consumed" || untouched != "uploaded" {
		t.Fatalf("exact artifact statuses consumed=%s untouched=%s", consumed, untouched)
	}
	var beforeCandidates, beforeDatasetRevisions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM semantic_candidates WHERE workspace_id=$1`, workspace.UUID()).Scan(&beforeCandidates); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM physical_dataset_revisions WHERE workspace_id=$1`, workspace.UUID()).Scan(&beforeDatasetRevisions); err != nil {
		t.Fatal(err)
	}
	if beforeCandidates == 0 {
		t.Fatal("first manual run must create candidates")
	}
	schedules := ingestionapp.NewScheduleService(store, nil, ingestionapp.ClockFunc(time.Now))
	schedule, err := schedules.Create(ctx, ingestionapp.CreateScheduleRequest{WorkspaceID: workspace, SourceID: source,
		Expression: "0 0 1 1 *", Timezone: "UTC", MisfirePolicy: ingestiondomain.MisfireSkip, PrincipalRef: principal.String(), IdempotencyKey: "repeat-schedule", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err != nil {
		t.Fatal(err)
	}
	occurrence, err := schedules.RunNow(ctx, ingestionapp.ScheduleCommandRequest{WorkspaceID: workspace, ScheduleID: schedule.ID,
		ExpectedVersion: schedule.Version, PrincipalRef: principal.String(), IdempotencyKey: "repeat-run-now", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err != nil || occurrence.DiscoveryRunID == nil {
		t.Fatalf("occurrence=%+v err=%v", occurrence, err)
	}
	if processed, err := worker.RunOne(ctx, "repeat-scheduled-artifact"); err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	var reused bool
	var scheduledRevision, manualRevision string
	if err := pool.QueryRow(ctx, `SELECT status,projection_reused,source_revision_id::text FROM discovery_runs WHERE id=$1`, occurrence.DiscoveryRunID.UUID()).Scan(&status, &reused, &scheduledRevision); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" && status != "degraded" || !reused {
		t.Fatalf("scheduled status=%s reused=%v", status, reused)
	}
	if err := pool.QueryRow(ctx, `SELECT source_revision_id::text FROM discovery_runs WHERE id=$1`, runID.UUID()).Scan(&manualRevision); err != nil {
		t.Fatal(err)
	}
	if scheduledRevision != manualRevision {
		t.Fatalf("different revisions %s %s", manualRevision, scheduledRevision)
	}
	if occurrence.RuntimeRunID == nil || occurrence.JobID == nil {
		t.Fatalf("missing owning runtime/job: %+v", occurrence)
	}
	projected, err := store.GetRuntimeRun(ctx, workspace, *occurrence.RuntimeRunID)
	if err != nil || projected.SourceID != occurrence.DiscoveryRunID.String() || projected.SourceType != "discovery_run" ||
		projected.JobID == nil || *projected.JobID != *occurrence.JobID || string(projected.State) != status || projected.FinishedAt == nil {
		t.Fatalf("exact terminal runtime=%+v err=%v", projected, err)
	}
	var afterCandidates, afterDatasetRevisions, canonicalRuns int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM semantic_candidates WHERE workspace_id=$1`, workspace.UUID()).Scan(&afterCandidates); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM physical_dataset_revisions WHERE workspace_id=$1`, workspace.UUID()).Scan(&afterDatasetRevisions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM discovery_runs WHERE workspace_id=$1 AND status IN('succeeded','degraded') AND NOT projection_reused`, workspace.UUID()).Scan(&canonicalRuns); err != nil {
		t.Fatal(err)
	}
	if beforeCandidates != afterCandidates || beforeDatasetRevisions != afterDatasetRevisions || canonicalRuns != 1 {
		t.Fatalf("candidates %d/%d datasets %d/%d canonical=%d", beforeCandidates, afterCandidates, beforeDatasetRevisions, afterDatasetRevisions, canonicalRuns)
	}
	snapshot, err := fileartifact.CSV().Discover(ctx, discoverydomain.Input{Locator: "artifact-set:" + set.Set.ID.String(), ObservedAt: time.Now().UTC(), Files: map[string][]byte{set.Set.Members[0].LogicalPath: content}})
	if err != nil {
		t.Fatal(err)
	}
	leaseRun, err := control.StartRun(ctx, ingestionappdiscovery.StartRunRequest{SourceRequest: ingestionappdiscovery.SourceRequest{WorkspaceID: workspace, SourceID: source, PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}, IdempotencyKey: "reused-lost-lease"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET status='running',attempt=1,lease_owner='reuse-fence',leased_until=clock_timestamp()+interval '1 minute' WHERE id=$1`, leaseRun.JobID.UUID()); err != nil {
		t.Fatal(err)
	}
	fenced := ingestionappdiscovery.WithRunCommitFence(ctx, ingestionappdiscovery.RunCommitFence{JobID: leaseRun.JobID, LeaseOwner: "reuse-fence"})
	if err := store.BeginDiscoveryRun(fenced, workspace, leaseRun.ID, leaseRun.JobID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET leased_until=clock_timestamp()-interval '1 second' WHERE id=$1`, leaseRun.JobID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersistDiscoveryRunSnapshot(fenced, workspace, source, leaseRun.ID, snapshot); err == nil {
		t.Fatalf("lost reuse lease error=%v", err)
	}
	var rollbackStatus, runtimeStatus string
	var rollbackReuse bool
	if err := pool.QueryRow(ctx, `SELECT status,projection_reused FROM discovery_runs WHERE id=$1`, leaseRun.ID.UUID()).Scan(&rollbackStatus, &rollbackReuse); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM runtime_runs WHERE workspace_id=$1 AND source_type='discovery_run' AND source_id=$2`, workspace.UUID(), leaseRun.ID.String()).Scan(&runtimeStatus); err != nil {
		t.Fatal(err)
	}
	if rollbackStatus != "running" || runtimeStatus != "running" || rollbackReuse {
		t.Fatalf("lost lease committed run=%s reused=%v runtime=%s", rollbackStatus, rollbackReuse, runtimeStatus)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET leased_until=clock_timestamp()+interval '1 minute' WHERE id=$1`, leaseRun.JobID.UUID()); err != nil {
		t.Fatal(err)
	}
	leaseRetry, err := store.PersistDiscoveryRunSnapshot(fenced, workspace, source, leaseRun.ID, snapshot)
	if err != nil || !leaseRetry.Replayed || leaseRetry.RunID != leaseRun.ID {
		t.Fatalf("lease retry=%+v err=%v", leaseRetry, err)
	}
	if err := store.MarkJobSucceeded(ctx, leaseRun.JobID, "reuse-fence", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join(repositoryRoot(), "migrations", "000018_fmb_file_ingestion_schedules.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, downErr := connection.Exec(ctx, string(down))
	_, rollbackErr := connection.Exec(ctx, "ROLLBACK")
	connection.Release()
	var pgErr *pgconn.PgError
	if !errors.As(downErr, &pgErr) || pgErr.Code != "55000" || !strings.Contains(pgErr.Message, "reused discovery projection history") || rollbackErr != nil {
		t.Fatalf("down error=%v rollback=%v", downErr, rollbackErr)
	}
	// A different adapter version must publish its own projection. Competing
	// publishers must all observe that one committed canonical result.
	snapshot.AdapterVersion += "/concurrent-test"
	type projectionResult struct {
		result ingestionappdiscovery.PersistResult
		err    error
	}
	results := make(chan projectionResult, 4)
	start := make(chan struct{})
	for range 4 {
		go func() {
			<-start
			result, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshot)
			results <- projectionResult{result: result, err: err}
		}()
	}
	close(start)
	var canonical identity.RunID
	newProjections := 0
	for range 4 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if canonical.IsZero() {
			canonical = result.result.RunID
		}
		if result.result.RunID != canonical || result.result.RunID == runID {
			t.Fatalf("projection identity=%+v", result.result)
		}
		if !result.result.Replayed {
			newProjections++
		}
	}
	if newProjections != 1 {
		t.Fatalf("concurrent canonical publications=%d", newProjections)
	}
	// A failed projection is not evidence of a successful publication. Retrying
	// the exact revision/version must run projection again before it is reusable.
	snapshot.AdapterVersion += "/failed-test"
	snapshot.Findings = []discoverydomain.Finding{{Code: "TEST_TERMINAL", Severity: "error", Terminal: true, Details: map[string]any{}}}
	failed, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshot)
	if err != nil || failed.Status != "failed" || failed.Replayed {
		t.Fatalf("failed=%+v err=%v", failed, err)
	}
	snapshot.Findings = nil
	succeeded, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshot)
	if err != nil || succeeded.Status != "succeeded" || succeeded.Replayed || succeeded.RunID == failed.RunID {
		t.Fatalf("retry=%+v err=%v", succeeded, err)
	}
	replayed, err := store.PersistDiscoverySnapshot(ctx, workspace, source, snapshot)
	if err != nil || !replayed.Replayed || replayed.RunID != succeeded.RunID {
		t.Fatalf("replay=%+v err=%v", replayed, err)
	}
}

func TestScheduledPostgreSQLRunUsesPinnedCredentialAndSkipsRetiredPin(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "schedule-credential-pin")
	principal, _ := identity.NewPrincipalID()
	source, _ := identity.NewSourceConnectionID()
	credential1, _ := identity.NewSourceCredentialID()
	credential2, _ := identity.NewSourceCredentialID()
	now := time.Now().UTC().Add(-5 * time.Second).Truncate(time.Second)
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status) VALUES($1,$2,'human','Scheduler','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_connections
(id,workspace_id,adapter_kind,name,normalized_locator,credential_ref,status,metadata,source_kind,version)
VALUES($1,$2,'postgresql_catalog','Warehouse','postgresql://warehouse/db','encrypted:v1','active',
'{"host":"warehouse","port":5432,"database":"db","username":"reader","sslMode":"require"}'::jsonb,'postgresql',1)`, source.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_credentials
(id,workspace_id,source_connection_id,version,key_version,algorithm,nonce,ciphertext,created_by)
VALUES($1,$2,$3,1,1,'AES-256-GCM',$4,$5,'system')`, credential1.UUID(), workspace.UUID(), source.UUID(), make([]byte, 12), make([]byte, 17)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE source_connections SET active_credential_version=1 WHERE id=$1`, source.UUID()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	scheduleID, _ := identity.NewSourceScheduleID()
	wall := now
	schedule := ingestiondomain.Schedule{ID: scheduleID, WorkspaceID: workspace, SourceConnectionID: source,
		Expression: "* * * * *", Timezone: "UTC", MisfirePolicy: ingestiondomain.MisfireSkip, Enabled: true,
		NextRunAt: &wall, NextWallClockKey: wall.Format("2006-01-02T15:04"), CreatedByPrincipalID: &principal,
		Version: 1, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)}
	created, err := store.CreateSchedule(ctx, ingestionapp.CreateScheduleCommand{Schedule: schedule, IdempotencyKey: "create-pin",
		RequestFingerprint: digestText("create-pin"), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err != nil || created.CredentialVersion == nil || *created.CredentialVersion != 1 {
		t.Fatalf("created schedule pin=%v err=%v", created.CredentialVersion, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_credentials
(id,workspace_id,source_connection_id,version,key_version,algorithm,nonce,ciphertext,created_by)
VALUES($1,$2,$3,2,1,'AES-256-GCM',$4,$5,'system')`, credential2.UUID(), workspace.UUID(), source.UUID(), make([]byte, 12), make([]byte, 17)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE source_connections SET active_credential_version=2,version=version+1 WHERE id=$1`, source.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE source_credentials SET retired_at=clock_timestamp() WHERE source_connection_id=$1 AND version=1`, source.UUID()); err != nil {
		t.Fatal(err)
	}
	service := ingestionapp.NewScheduleService(store, nil, ingestionapp.ClockFunc(time.Now))
	processed, err := service.ProcessDue(ctx, "scheduler-test-owner", 10, time.Minute)
	if err != nil || processed != 1 {
		t.Fatalf("process due: processed=%d err=%v", processed, err)
	}
	var reason, state string
	var jobs int
	if err := pool.QueryRow(ctx, `SELECT state,reason_code FROM source_schedule_occurrences WHERE schedule_id=$1`, scheduleID.UUID()).Scan(&state, &reason); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE workspace_id=$1 AND job_type='source.discovery'`, workspace.UUID()).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if state != "skipped" || reason != "CREDENTIAL_UNAVAILABLE" || jobs != 0 {
		t.Fatalf("occurrence state=%s reason=%s jobs=%d", state, reason, jobs)
	}
}

func TestScheduledExecutionUsesSystemActorAfterCreatorSuspension(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "schedule-system-actor")
	principal, _ := identity.NewPrincipalID()
	source, _ := identity.NewSourceConnectionID()
	credential, _ := identity.NewSourceCredentialID()
	now := time.Now().UTC().Add(-5 * time.Second).Truncate(time.Second)
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status)
VALUES($1,$2,'human','Former scheduler owner','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_connections
(id,workspace_id,adapter_kind,name,normalized_locator,credential_ref,status,metadata,source_kind,version)
VALUES($1,$2,'postgresql_catalog','Warehouse','postgresql://warehouse/db','encrypted:v1','active',
'{"host":"warehouse","port":5432,"database":"db","username":"reader","sslMode":"require"}'::jsonb,'postgresql',1)`,
		source.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_credentials
(id,workspace_id,source_connection_id,version,key_version,algorithm,nonce,ciphertext,created_by)
VALUES($1,$2,$3,1,1,'AES-256-GCM',$4,$5,'system')`, credential.UUID(), workspace.UUID(), source.UUID(), make([]byte, 12), make([]byte, 17)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE source_connections SET active_credential_version=1 WHERE workspace_id=$1 AND id=$2`, workspace.UUID(), source.UUID()); err != nil {
		t.Fatal(err)
	}
	scheduleID, _ := identity.NewSourceScheduleID()
	wall := now
	schedule := ingestiondomain.Schedule{ID: scheduleID, WorkspaceID: workspace, SourceConnectionID: source,
		Expression: "* * * * *", Timezone: "UTC", MisfirePolicy: ingestiondomain.MisfireSkip, Enabled: true,
		NextRunAt: &wall, NextWallClockKey: wall.Format("2006-01-02T15:04"), CreatedByPrincipalID: &principal,
		Version: 1, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)}
	if _, err := store.CreateSchedule(ctx, ingestionapp.CreateScheduleCommand{Schedule: schedule, IdempotencyKey: "create-system-actor",
		RequestFingerprint: digestText("create-system-actor"), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE principals SET status='suspended' WHERE workspace_id=$1 AND id=$2`, workspace.UUID(), principal.UUID()); err != nil {
		t.Fatal(err)
	}
	service := ingestionapp.NewScheduleService(store, nil, ingestionapp.ClockFunc(time.Now))
	processed, err := service.ProcessDue(ctx, "scheduler-system-actor", 10, time.Minute)
	if err != nil || processed != 1 {
		t.Fatalf("process due: processed=%d err=%v", processed, err)
	}
	var occurrenceRequestedBy, runRequestedBy string
	if err := pool.QueryRow(ctx, `SELECT requested_by_principal_id::text FROM source_schedule_occurrences WHERE schedule_id=$1`, scheduleID.UUID()).Scan(&occurrenceRequestedBy); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT requested_by FROM discovery_runs WHERE workspace_id=$1 AND source_connection_id=$2 ORDER BY created_at DESC LIMIT 1`,
		workspace.UUID(), source.UUID()).Scan(&runRequestedBy); err != nil {
		t.Fatal(err)
	}
	if occurrenceRequestedBy != principal.UUID() || runRequestedBy != "system" {
		t.Fatalf("delegator=%s run actor=%s", occurrenceRequestedBy, runRequestedBy)
	}
	rows, err := pool.Query(ctx, `SELECT event_type,actor_id FROM audit_events
WHERE workspace_id=$1 AND event_type IN ('schedule.occurrence','discovery.queued') ORDER BY event_type`, workspace.UUID())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]string{}
	for rows.Next() {
		var eventType, actor string
		if err := rows.Scan(&eventType, &actor); err != nil {
			t.Fatal(err)
		}
		seen[eventType] = actor
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen["schedule.occurrence"] != "system" || seen["discovery.queued"] != "system" {
		t.Fatalf("scheduled audit actors=%v", seen)
	}
}

func TestScheduledOccurrenceIdentityIncludesScheduleDefinitionVersion(t *testing.T) {
	ctx := context.Background()
	pool, _ := openStore(t)
	workspace := createWorkspace(t, pool, "schedule-definition-identity")
	principal, _ := identity.NewPrincipalID()
	source, _ := identity.NewSourceConnectionID()
	schedule, _ := identity.NewSourceScheduleID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status)
VALUES($1,$2,'human','Scheduler','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_connections
(id,workspace_id,adapter_kind,name,normalized_locator,status,metadata,source_kind,version)
VALUES($1,$2,'postgresql_catalog','Scheduled source','postgresql://schedule/db','paused','{}'::jsonb,'postgresql',1)`,
		source.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	wallKey := now.Format("2006-01-02T15:04")
	if _, err := pool.Exec(ctx, `INSERT INTO source_schedules
(id,workspace_id,source_connection_id,expression,timezone,misfire_policy,enabled,next_run_at,next_wall_clock_key,
 created_by_principal_id,create_idempotency_key,create_request_fingerprint,version,created_at,updated_at)
VALUES($1,$2,$3,'0 10 * * *','UTC','skip',true,$4,$5,$6,'schedule-definition',$7,1,$4,$4)`,
		schedule.UUID(), workspace.UUID(), source.UUID(), now, wallKey, principal.UUID(), "sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	insertSkipped := func(version int64, idempotency string) {
		t.Helper()
		occurrence, _ := identity.NewScheduleOccurrenceID()
		if _, err := pool.Exec(ctx, `INSERT INTO source_schedule_occurrences
(id,workspace_id,schedule_id,source_connection_id,trigger_kind,schedule_version,scheduled_for,eligible_at,wall_clock_key,
 state,reason_code,requested_by_principal_id,idempotency_key,created_at)
VALUES($1,$2,$3,$4,'scheduled',$5,$6,$6,$7,'skipped','SOURCE_UNAVAILABLE',$8,$9,$6)`,
			occurrence.UUID(), workspace.UUID(), schedule.UUID(), source.UUID(), version, now, wallKey, principal.UUID(), idempotency); err != nil {
			t.Fatal(err)
		}
	}
	insertSkipped(1, "scheduled-v1")
	if _, err := pool.Exec(ctx, `UPDATE source_schedules SET timezone='America/New_York',version=2 WHERE id=$1`, schedule.UUID()); err != nil {
		t.Fatal(err)
	}
	insertSkipped(2, "scheduled-v2")
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM source_schedule_occurrences WHERE schedule_id=$1 AND wall_clock_key=$2`,
		schedule.UUID(), wallKey).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("occurrences=%d, want distinct facts for two schedule definitions", count)
	}
}

func TestDueScheduleAcquiresWorkspaceBeforeScheduleRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "schedule-lock-order")
	principal, _ := identity.NewPrincipalID()
	source, _ := identity.NewSourceConnectionID()
	schedule, _ := identity.NewSourceScheduleID()
	if _, err := pool.Exec(ctx, `INSERT INTO principals(id,workspace_id,kind,display_name,status) VALUES($1,$2,'human','Scheduler','active')`, principal.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_connections (id,workspace_id,adapter_kind,name,normalized_locator,status,metadata,source_kind,version) VALUES($1,$2,'postgresql_catalog','Source','postgresql://schedule/db','paused','{}'::jsonb,'postgresql',1)`, source.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Second)
	if _, err := pool.Exec(ctx, `INSERT INTO source_schedules
(id,workspace_id,source_connection_id,expression,timezone,misfire_policy,enabled,next_run_at,next_wall_clock_key,created_by_principal_id,create_idempotency_key,create_request_fingerprint,version,created_at,updated_at)
VALUES($1,$2,$3,'* * * * *','UTC','skip',true,$4,$5,$6,'lock-order',$7,1,$4,$4)`, schedule.UUID(), workspace.UUID(), source.UUID(), now, now.Format("2006-01-02T15:04"), principal.UUID(), digestText("lock-order")); err != nil {
		t.Fatal(err)
	}
	claimed, databaseNow, err := store.ClaimDueSchedules(ctx, "lock-order-worker", time.Minute, 100)
	if err != nil {
		t.Fatal(err)
	}
	var due ingestiondomain.Schedule
	for _, candidate := range claimed {
		if candidate.ID == schedule {
			due = candidate
		}
	}
	if due.ID.IsZero() {
		t.Fatal("schedule was not claimed")
	}
	transaction, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback(context.Background())
	var blockerPID int
	if err := transaction.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(ctx, `SELECT id FROM workspaces WHERE id=$1 FOR UPDATE`, workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	occurrenceID, _ := identity.NewScheduleOccurrenceID()
	done := make(chan error, 1)
	go func() {
		_, err := store.CompleteDueSchedule(ctx, ingestionapp.CompleteDueScheduleCommand{Schedule: due,
			Occurrence: ingestiondomain.Occurrence{ID: occurrenceID, WorkspaceID: workspace, ScheduleID: schedule, SourceConnectionID: source, TriggerKind: "scheduled", ScheduleVersion: due.Version, ScheduledFor: &now, EligibleAt: databaseNow, WallClockKey: due.NextWallClockKey, State: "skipped", ReasonCode: "SOURCE_UNAVAILABLE", RequestedBy: principal, IdempotencyKey: "lock-order-occurrence", CreatedAt: databaseNow},
			LeaseOwner: "lock-order-worker", Now: databaseNow, Next: ingestionapp.NominalOccurrence{EligibleAt: databaseNow.Add(time.Hour), WallClockKey: databaseNow.Add(time.Hour).Format("2006-01-02T15:04")}, TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
		done <- err
	}()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, blockerPID).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("scheduler completed before barrier: %v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	// A user mutation already holding the workspace must still be able to lock its schedule.
	if _, err := transaction.Exec(ctx, `SELECT id FROM source_schedules WHERE id=$1 FOR UPDATE NOWAIT`, schedule.UUID()); err != nil {
		t.Fatalf("scheduler inverted workspace/schedule lock order: %v", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSourcePaginationBindsWorkspacePrincipalAuthorizationAndFilter(t *testing.T) {
	ctx := context.Background()
	pool, store := openStore(t)
	workspace := createWorkspace(t, pool, "source-page")
	principal, _ := identity.NewPrincipalID()
	var authVersion int64
	if err := pool.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1`, workspace.UUID()).Scan(&authVersion); err != nil {
		t.Fatal(err)
	}
	ids := make([]identity.SourceConnectionID, 0, 3)
	for index := 0; index < 3; index++ {
		id, _ := identity.NewSourceConnectionID()
		ids = append(ids, id)
		at := time.Now().UTC().Add(time.Duration(index) * time.Minute)
		if _, err := pool.Exec(ctx, `INSERT INTO source_connections
(id,workspace_id,adapter_kind,name,normalized_locator,status,metadata,source_kind,artifact_paths,version,created_at,updated_at)
VALUES($1,$2,'postgresql_catalog',$3,$4,'paused','{}'::jsonb,'postgresql','[]'::jsonb,1,$5,$5)`,
			id.UUID(), workspace.UUID(), fmt.Sprintf("Source %d", index), "postgresql://source/db/"+id.String(), at); err != nil {
			t.Fatal(err)
		}
	}
	query := ingestionappdiscovery.ListSourcesQuery{WorkspaceID: workspace, PrincipalID: principal,
		AuthorizationVersion: authVersion, Limit: 2}
	first, err := store.ListDiscoverySources(ctx, query)
	if err != nil || first.Total != 3 || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page=%+v err=%v", first, err)
	}
	query.Cursor = first.NextCursor
	second, err := store.ListDiscoverySources(ctx, query)
	if err != nil || second.Total != 3 || len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("second page=%+v err=%v", second, err)
	}
	query.Cursor, query.SourceID = "", &ids[0]
	filtered, err := store.ListDiscoverySources(ctx, query)
	if err != nil || filtered.Total != 1 || len(filtered.Items) != 1 || filtered.Items[0].ID != ids[0] {
		t.Fatalf("filtered page=%+v err=%v", filtered, err)
	}
	query.SourceID, query.Cursor = nil, first.NextCursor
	if _, err := pool.Exec(ctx, `UPDATE workspaces SET authorization_version=authorization_version+1 WHERE id=$1`, workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListDiscoverySources(ctx, query); !errors.Is(err, discoverydomain.ErrConflict) {
		t.Fatalf("stale cursor err=%v", err)
	}
}

func TestScheduleFixtureDoesNotLeaveClaimableWork(t *testing.T) {
	t.Run("owner", TestScheduledExecutionUsesSystemActorAfterCreatorSuspension)
	pool, _ := openStore(t)
	var enabled int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM source_schedules s JOIN workspaces w ON w.id=s.workspace_id WHERE w.slug LIKE 'schedule-system-actor-%' AND s.enabled`).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if enabled != 0 {
		t.Fatalf("completed fixtures left %d enabled schedules", enabled)
	}
}

func openStore(t *testing.T) (*pgxpool.Pool, *pgstore.Store) {
	t.Helper()
	pool, err := pgstore.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, pgstore.NewStore(pool)
}

func createWorkspace(t *testing.T, pool *pgxpool.Pool, slug string) identity.WorkspaceID {
	t.Helper()
	workspace, _ := identity.NewWorkspaceID()
	if _, err := pool.Exec(context.Background(), `INSERT INTO workspaces(id,slug,display_name) VALUES($1,$2,$3)`, workspace.UUID(), slug+"-"+workspace.String(), slug); err != nil {
		t.Fatal(err)
	}
	// The scheduler claims across workspaces in the shared test database.
	// Retire only this fixture's schedules before its pool is closed.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, `UPDATE source_schedules SET enabled=false,next_run_at=NULL,next_wall_clock_key=NULL,lease_owner=NULL,leased_until=NULL WHERE workspace_id=$1`, workspace.UUID()); err != nil {
			t.Errorf("retire fixture schedules: %v", err)
		}
	})
	return workspace
}

func artifactCommand(t *testing.T, workspace identity.WorkspaceID, principal *identity.PrincipalID, content []byte, createdAt time.Time) ingestionapp.CreateArtifactCommand {
	t.Helper()
	hash := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(hash[:])
	id, _ := identity.NewArtifactID()
	uploader := identity.PrincipalID{}
	if principal != nil {
		uploader = *principal
	}
	return ingestionapp.CreateArtifactCommand{
		Artifact: ingestiondomain.Artifact{ID: id, WorkspaceID: workspace, Kind: ingestiondomain.ArtifactCSV,
			SchemaVersion: "semlia-artifact-v1", ContentDigest: digest, ByteSize: int64(len(content)), MediaType: "text/csv",
			OriginalName: "input.csv", Status: ingestiondomain.ArtifactUploaded, UploadedByPrincipalID: &uploader,
			IdempotencyKey: id.String(), RequestFingerprint: "sha256:" + hex.EncodeToString(hash[:]),
			ExpiresAt: timePointer(createdAt.Add(24 * time.Hour)), CreatedAt: createdAt},
		Object: ingestiondomain.Object{WorkspaceID: workspace, ContentDigest: digest, ByteSize: int64(len(content)),
			MediaType: "text/csv", StorageKey: "workspaces/" + workspace.UUID() + "/sha256/" + hex.EncodeToString(hash[:]), CreatedAt: createdAt},
	}
}

func timePointer(value time.Time) *time.Time { return &value }

func artifactSetCommand(t *testing.T, workspace identity.WorkspaceID, source identity.SourceConnectionID,
	principal identity.PrincipalID, artifact ingestionapp.CreateArtifactCommand, key string, createdAt time.Time,
	expectedVersion *int64, retentionUntil time.Time,
) ingestionapp.FinalizeArtifactSetCommand {
	t.Helper()
	setID, _ := identity.NewArtifactSetID()
	member := ingestiondomain.ArtifactMember{ArtifactID: artifact.Artifact.ID, LogicalPath: artifact.Artifact.OriginalName,
		Ordinal: 1, ContentDigest: artifact.Object.ContentDigest, ByteSize: artifact.Object.ByteSize,
		MediaType: artifact.Object.MediaType, Kind: artifact.Artifact.Kind}
	return ingestionapp.FinalizeArtifactSetCommand{
		Set: ingestiondomain.ArtifactSet{ID: setID, WorkspaceID: workspace, SourceConnectionID: source, SourceKind: "file",
			SetDigest: digestText(key + artifact.Object.ContentDigest), CreatedByPrincipalID: principal,
			Members: []ingestiondomain.ArtifactMember{member}, CreatedAt: createdAt},
		SourceName: "Versioned file", ExpectedSourceVersion: expectedVersion, IdempotencyKey: key,
		RequestFingerprint: digestText("request-" + key), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		ContentRetentionUntil: retentionUntil,
	}
}

func assertRetention(t *testing.T, pool *pgxpool.Pool, workspace identity.WorkspaceID, digest, wantState string, wantReservations, wantUsage int) {
	t.Helper()
	var state string
	var reservations, usage int
	if err := pool.QueryRow(context.Background(), `SELECT state,reservation_count FROM artifact_object_retention WHERE workspace_id=$1 AND content_sha256=$2`, workspace.UUID(), digest).Scan(&state, &reservations); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT active_object_count FROM workspace_artifact_usage WHERE workspace_id=$1`, workspace.UUID()).Scan(&usage); err != nil {
		t.Fatal(err)
	}
	if state != wantState || reservations != wantReservations || usage != wantUsage {
		t.Fatalf("retention state=%s reservations=%d usage=%d, want %s/%d/%d", state, reservations, usage, wantState, wantReservations, wantUsage)
	}
}

func digestText(value string) string {
	hash := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(hash[:])
}

func repositoryRoot() string {
	_, current, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", ".."))
}
