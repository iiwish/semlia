package embedding_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	adapter "github.com/iiwish/semlia/internal/adapters/embedding"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	app "github.com/iiwish/semlia/internal/application/embedding"
	governance "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/embedding"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

type evaluator struct {
	principal   identity.PrincipalID
	version     int64
	deny        bool
	denyAsset   bool
	bumpOnAsset bool
}

func (e *evaluator) Evaluate(_ context.Context, r authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	if e.bumpOnAsset && r.Action == authorization.ActionAssetRead {
		e.version++
	}
	return authorization.Decision{Allowed: !e.deny && !(e.denyAsset && r.Action == authorization.ActionAssetRead), PrincipalID: e.principal, AuthorizationVersion: e.version, Action: r.Action}, nil
}

func TestPersistentEmbeddingCheckpointActivationReuseAndReleasedSearch(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "pgvector/pgvector:pg17", tcpostgres.WithDatabase("semlia_embedding_test"), tcpostgres.WithUsername("semlia"), tcpostgres.WithPassword("test-only"), tcpostgres.BasicWaitStrategies())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })
	database, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	migrator, err := pgstore.NewMigrator(database, filepath.Join(root, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	defer migrator.Close()
	if err := migrator.Steps(18); err != nil {
		t.Fatal(err)
	}
	pool, err := pgstore.Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	priorWorkspace, _ := identity.NewWorkspaceID()
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces(id,slug,display_name) VALUES($1,'before-19','Before 19')`, priorWorkspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	var preserved int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspaces WHERE id=$1`, priorWorkspace.UUID()).Scan(&preserved); err != nil || preserved != 1 {
		t.Fatalf("migration18 state lost: %d %v", preserved, err)
	}
	store := pgstore.NewStore(pool)
	if store.EmbeddingConfigured(ctx) {
		t.Fatal("metadata migration must not silently enable pgvector")
	}
	if _, err := pool.Exec(ctx, `CREATE EXTENSION vector`); err != nil {
		t.Fatal(err)
	}
	if store.EmbeddingConfigured(ctx) {
		t.Fatal("extension without vector column reported configured")
	}
	enable, err := os.ReadFile(filepath.Join(root, "deploy/local/enable-pgvector.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(enable)); err != nil {
		t.Fatal(err)
	}
	if !store.EmbeddingConfigured(ctx) {
		t.Fatal("real pgvector schema unavailable")
	}
	w, _ := identity.NewWorkspaceID()
	p, _ := identity.NewPrincipalID()
	a, _ := identity.NewAssetID()
	r, _ := identity.NewRevisionID()
	release, _ := identity.NewReleaseID()
	provider, _ := identity.NewModelProviderID()
	setting, _ := identity.NewModelSettingID()
	var calls atomic.Int32
	var failProvider atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if failProvider.Load() {
			out.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Input []string `json:"input"`
			Model string   `json:"model"`
		}
		if json.NewDecoder(req.Body).Decode(&body) != nil {
			t.Error("bad batch")
		}
		data := []map[string]any{}
		for i := range body.Input {
			data = append(data, map[string]any{"index": i, "embedding": []float32{1, 2}})
		}
		_ = json.NewEncoder(out).Encode(map[string]any{"model": body.Model, "data": data})
	}))
	defer server.Close()
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspaces(id,slug,display_name) VALUES($1,'embedding-test','Embedding')`, []any{w.UUID()}},
		{`INSERT INTO principals(id,workspace_id,kind,display_name,status) VALUES($1,$2,'human','Embedding admin','active')`, []any{p.UUID(), w.UUID()}},
		{`INSERT INTO semantic_assets(id,workspace_id,namespace,key,asset_type,lifecycle_state) VALUES($1,$2,'sales','revenue','metric','active')`, []any{a.UUID(), w.UUID()}},
		{`INSERT INTO asset_revisions(id,workspace_id,asset_id,sequence,schema_version,content_digest,content,created_by) VALUES($1,$2,$3,1,'1.0.0',$4,'{"name":"Revenue","description":"Released revenue"}','author')`, []any{r.UUID(), w.UUID(), a.UUID(), domain.Digest("revision")}},
		{`INSERT INTO releases(id,workspace_id,sequence,manifest_digest,published_by,published_at) VALUES($1,$2,1,$3,'publisher',clock_timestamp())`, []any{release.UUID(), w.UUID(), domain.Digest("release")}},
		{`INSERT INTO release_assets(workspace_id,release_id,asset_id,revision_id,position) VALUES($1,$2,$3,$4,1)`, []any{w.UUID(), release.UUID(), a.UUID(), r.UUID()}},
		{`INSERT INTO model_providers(id,workspace_id,protocol,display_name,base_url,credential_env,credential_revision) VALUES($1,$2,'openai_compatible','Test',$3,'TEST_EMBED_KEY',$4)`, []any{provider.UUID(), w.UUID(), server.URL, governance.CredentialRevisionDigest("test-key")}},
		{`INSERT INTO model_settings(id,workspace_id,provider_id,kind,model,is_default,capability,token_limit,embedding_dimension) VALUES($1,$2,$3,'embedding','test-model',true,'semantic',8192,2)`, []any{setting.UUID(), w.UUID(), provider.UUID()}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	var version int64
	if pool.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1`, w.UUID()).Scan(&version) != nil {
		t.Fatal("auth version")
	}
	auth := &evaluator{principal: p, version: version}
	client := adapter.NewClient(server.Client(), func(string) string { return "test-key" })
	service := app.NewService(store, client, auth)
	first, err := service.Start(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736", "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736", "overlap"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("concurrent intent=%v", err)
	}
	replay, err := service.Start(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736", "first")
	if err != nil || replay.ID != first.ID {
		t.Fatalf("intent replay=%+v err=%v", replay, err)
	}
	job, err := store.ClaimJob(ctx, "checkpoint-worker", time.Now().UTC(), time.Minute)
	if err != nil || job == nil {
		t.Fatalf("job=%v err=%v", job, err)
	}
	index, chunks, err := store.EmbeddingBatch(ctx, *job)
	if err != nil || len(chunks) != 1 {
		t.Fatalf("batch=%v %v", chunks, err)
	}
	if err := store.ActivateEmbedding(ctx, *job, index); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("incomplete activation=%v", err)
	}
	if err := store.CheckpointEmbedding(ctx, *job, index, chunks, [][]float32{{1}}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("dimension=%v", err)
	}
	if err := store.CheckpointEmbedding(ctx, *job, index, chunks, [][]float32{{1, 2}}); err != nil {
		t.Fatal(err)
	}
	restarted := pgstore.NewStore(pool)
	index, chunks, err = restarted.EmbeddingBatch(ctx, *job)
	if err != nil || len(chunks) != 0 {
		t.Fatalf("checkpoint replay=%v %v", chunks, err)
	}
	lost := *job
	lost.LeaseOwner = "old-owner"
	if err := restarted.ActivateEmbedding(ctx, lost, index); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("lease=%v", err)
	}
	if err := restarted.ActivateEmbedding(ctx, *job, index); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkJobSucceeded(ctx, job.ID, job.LeaseOwner, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	result, err := service.Search(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736", "revenue")
	if err != nil || result.Mode != "vector" || len(result.Items) != 1 || result.Items[0].RevisionID != r {
		t.Fatalf("search=%+v err=%v", result, err)
	}
	auth.deny = true
	if _, err := service.Search(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736", "revenue"); err == nil {
		t.Fatal("denied search")
	}
	auth.deny = false
	auth.denyAsset = true
	filtered, err := service.Search(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736", "revenue")
	if err != nil || len(filtered.Items) != 0 {
		t.Fatalf("asset authorization filter=%+v %v", filtered, err)
	}
	auth.denyAsset = false
	auth.bumpOnAsset = true
	if _, err := service.Search(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736", "revenue"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("auth version fence=%v", err)
	}
	auth.bumpOnAsset = false
	auth.version = version
	second, err := service.Start(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736", "second")
	if err != nil || second.VectorCount != 1 {
		t.Fatalf("reuse=%+v err=%v", second, err)
	}
	if err := service.Cancel(ctx, w, second.ID, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	worker := jobs.NewWorker(store, jobs.ClockFunc(time.Now), jobs.BackoffFunc(func(int32) time.Duration { return 0 }), time.Minute)
	worker.Register(app.JobType, service.JobHandler())
	if ok, err := worker.RunOne(ctx, "cancelled-worker"); err != nil || !ok {
		t.Fatalf("cancel job=%v %v", ok, err)
	}
	status, err := service.Status(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil || status.Active.ID != first.ID || status.Latest.State != "cancelled" {
		t.Fatalf("cancel active=%+v err=%v", status, err)
	}
	t.Logf("real pgvector, checkpoint, cancellation, vector reuse and search passed; deterministic transport calls=%d", calls.Load())
	for _, lane := range []string{"claim", "fail"} {
		t.Run(lane+" cancellation workspace FK compatibility", func(t *testing.T) {
			index, err := service.Start(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736", lane)
			if err != nil {
				t.Fatal(err)
			}
			holder, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback(ctx)
			var pid int
			if err := holder.QueryRow(ctx, `SELECT pg_backend_pid() FROM jobs WHERE id=$1 FOR UPDATE`, index.JobID.UUID()).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- service.Cancel(ctx, w, index.ID, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736") }()
			deadline := time.Now().Add(3 * time.Second)
			for {
				var blocked bool
				if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, pid).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("cancellation did not reach job barrier")
				}
				time.Sleep(10 * time.Millisecond)
			}
			_, lockErr := holder.Exec(ctx, `SELECT id FROM workspaces WHERE id=$1 FOR KEY SHARE NOWAIT`, w.UUID())
			_ = holder.Rollback(ctx)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if lockErr != nil {
				t.Fatalf("job holder cannot acquire runtime event workspace FK: %v", lockErr)
			}
		})
	}
	// Drain cancelled jobs from the barrier cases before testing a real provider batch.
	for i := 0; i < 2; i++ {
		if _, err := worker.RunOne(ctx, "drain-cancelled"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE model_settings SET model='new-model' WHERE id=$1`, setting.UUID()); err != nil {
		t.Fatal(err)
	}
	third, err := service.Start(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736", "third")
	if err != nil || third.VectorCount != 0 {
		t.Fatalf("model identity must not reuse=%+v %v", third, err)
	}
	if _, err := worker.RunOne(ctx, "provider-worker"); err != nil {
		t.Fatal(err)
	}
	status, err = service.Status(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil || status.Active.ID != third.ID || status.Active.VectorCount != 1 {
		t.Fatalf("real batch activation=%+v %v", status, err)
	}
	// A query generated for the retired model cannot search the new active index.
	_, mode, err := store.SearchEmbedding(ctx, w, "revenue", &first, []float32{1, 2})
	if err != nil || mode != "lexical" {
		t.Fatalf("generation switch fence=%s %v", mode, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE model_settings SET model='failing-model' WHERE id=$1`, setting.UUID()); err != nil {
		t.Fatal(err)
	}
	failed, err := service.Start(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736", "failed")
	if err != nil {
		t.Fatal(err)
	}
	failProvider.Store(true)
	for i := 0; i < 3; i++ {
		if _, err := worker.RunOne(ctx, "failing-worker"); err != nil {
			t.Fatal(err)
		}
	}
	status, err = service.Status(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil || status.Active.ID != third.ID || status.Latest.ID != failed.ID || status.Latest.State != "failed" {
		t.Fatalf("failure preserves active=%+v %v", status, err)
	}
	down, err := os.ReadFile(filepath.Join(root, "migrations/000019_fmb_embedding_index.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err == nil {
		t.Fatal("rollback removed embedding history")
	}
	_ = tx.Rollback(ctx)
	failProvider.Store(false)
	newRelease, _ := identity.NewReleaseID()
	if _, err := pool.Exec(ctx, `INSERT INTO releases(id,workspace_id,sequence,manifest_digest,published_by,published_at) VALUES($1,$2,2,$3,'publisher',clock_timestamp())`, newRelease.UUID(), w.UUID(), domain.Digest("empty-release")); err != nil {
		t.Fatal(err)
	}
	result, err = service.Search(ctx, w, p.String(), "4bf92f3577b34da6a3ce929d0e0e4736", "revenue")
	if err != nil || result.Mode != "lexical" || result.FallbackReason != "release_changed" || len(result.Items) != 0 {
		t.Fatalf("new release must exclude old revision=%+v %v", result, err)
	}
}
