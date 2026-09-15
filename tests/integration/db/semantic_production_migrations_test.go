package db_test

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
)

func semanticProductionDatabase(t *testing.T, name string, version int) (*pgstore.Pool, *pgstore.Migrator) {
	t.Helper()
	ctx := context.Background()
	admin := openPool(t)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	migrator, err := pgstore.NewMigrator(u.String(), filepath.Join(repoRoot, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close() })
	if err = migrator.Steps(version); err != nil {
		t.Fatal(err)
	}
	pool, err := pgstore.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, migrator
}

func seedSourceHistoryRun(t *testing.T, pool *pgstore.Pool) (identity.WorkspaceID, identity.SourceConnectionID, identity.SourceRevisionID, identity.RunID) {
	t.Helper()
	ctx := context.Background()
	workspace, _ := identity.NewWorkspaceID()
	source, _ := identity.NewSourceConnectionID()
	revision, _ := identity.NewSourceRevisionID()
	run, _ := identity.NewRunID()
	for _, step := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspaces(id,slug,display_name) VALUES($1,'snapshot-migration','Snapshot migration')`, []any{workspace.UUID()}},
		{`INSERT INTO source_connections(id,workspace_id,adapter_kind,name,normalized_locator) VALUES($1,$2,'catalog','Original name','catalog://legacy')`, []any{source.UUID(), workspace.UUID()}},
		{`INSERT INTO source_revisions(id,workspace_id,source_connection_id,content_digest,adapter_version,observed_at) VALUES($1,$2,$3,$4,'1.0.0',CURRENT_TIMESTAMP)`, []any{revision.UUID(), workspace.UUID(), source.UUID(), distributionDigest("legacy")}},
		{`INSERT INTO discovery_runs(id,workspace_id,source_connection_id,source_revision_id,adapter_version,status,started_at,completed_at) VALUES($1,$2,$3,$4,'1.0.0','succeeded',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, []any{run.UUID(), workspace.UUID(), source.UUID(), revision.UUID()}},
	} {
		if _, err := pool.Exec(ctx, step.sql, step.args...); err != nil {
			t.Fatal(err)
		}
	}
	return workspace, source, revision, run
}

func TestSemanticProductionMigrationEmptyRollback(t *testing.T) {
	pool, migrator := semanticProductionDatabase(t, "source22_empty", 22)
	assertVersion(t, migrator, 22, true)
	if err := migrator.Steps(-1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 21, true)
	var exists bool
	if err := pool.QueryRow(context.Background(), `SELECT to_regclass('source_snapshots') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatalf("snapshot table remains=%v err=%v", exists, err)
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 22, true)
}

func TestSemanticProductionMigrationBusinessRuleLifecycle(t *testing.T) {
	pool, migrator := semanticProductionDatabase(t, "business_rule28_lifecycle", 27)
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 28, true)
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM production_business_rule_events`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("migration fabricated confirmations: %d %v", count, err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 27, true)
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 28, true)
}

func TestSemanticProductionMigrationGenerationLifecycle(t *testing.T) {
	pool, migrator := semanticProductionDatabase(t, "generation27_lifecycle", 27)
	var exists bool
	if err := pool.QueryRow(context.Background(), `SELECT to_regclass('production_generation_requests') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("generation schema: %v %v", exists, err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 26, true)
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 27, true)
}

func TestSemanticProductionMigrationAuthoringCanonicalIntegrity(t *testing.T) {
	pool, migrator := semanticProductionDatabase(t, "authoring25_integrity", 24)
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 25, true)
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns WHERE table_name='production_versions' AND column_name IN ('canonical_input','canonical_declarations','canonical_baseline','baseline_head','history_quality')`).Scan(&count); err != nil || count != 5 {
		t.Fatalf("canonical history columns: %d, %v", count, err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 24, true)
}

func TestSemanticProductionMigrationAuthoringLegacyHistory(t *testing.T) {
	pool, migrator := semanticProductionDatabase(t, "authoring25_legacy", 24)
	ctx := context.Background()
	workspace, _ := identity.New(identity.Workspace)
	user, _ := identity.New(identity.UserAccount)
	op, _ := identity.New(identity.ProductionOperation)
	for _, step := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspaces(id,slug,display_name) VALUES($1,'authoring-legacy','Authoring legacy')`, []any{workspace.UUID()}},
		{`INSERT INTO user_accounts(id,display_name) VALUES($1,'Legacy author')`, []any{user.UUID()}},
		{`INSERT INTO production_operations(id,workspace_id,created_by,current_version) VALUES($1,$2,$3,1)`, []any{op.UUID(), workspace.UUID(), user.UUID()}},
		{`INSERT INTO production_versions(workspace_id,operation_id,version,input_json,input_digest,request_digest,set_digest,created_by) VALUES($1,$2,1,'{}',$3,$3,$3,$4)`, []any{workspace.UUID(), op.UUID(), distributionDigest("legacy-authoring"), user.UUID()}},
	} {
		if _, err := pool.Exec(ctx, step.sql, step.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 25, true)
	var quality string
	var unknown bool
	if err := pool.QueryRow(ctx, `SELECT history_quality,canonical_input IS NULL AND canonical_declarations IS NULL AND canonical_baseline IS NULL FROM production_versions WHERE operation_id=$1`, op.UUID()).Scan(&quality, &unknown); err != nil || quality != "unverifiable" || !unknown {
		t.Fatalf("legacy evidence fabricated: quality=%s unknown=%v err=%v", quality, unknown, err)
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 26, true)
	if err := pool.QueryRow(ctx, `SELECT history_quality,canonical_input IS NULL AND canonical_declarations IS NULL AND canonical_baseline IS NULL FROM production_versions WHERE operation_id=$1`, op.UUID()).Scan(&quality, &unknown); err != nil || quality != "unverifiable" || !unknown {
		t.Fatalf("release integrity migration fabricated legacy proof: %s %v %v", quality, unknown, err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("removing unused integrity layer failed: %v", err)
	}
	assertVersion(t, migrator, 25, true)
	if err := migrator.Steps(-1); err == nil || !strings.Contains(err.Error(), "DOWN_MIGRATION_UNSAFE") {
		t.Fatalf("unsafe downgrade accepted: %v", err)
	}
	var count int
	var dirty bool
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM production_versions WHERE operation_id=$1),(SELECT dirty FROM schema_migrations)`, op.UUID()).Scan(&count, &dirty); err != nil || count != 1 || !dirty {
		t.Fatalf("downgrade lost history or dirty marker: count=%d dirty=%v err=%v", count, dirty, err)
	}
}

func TestSemanticProductionMigrationLegacyQualityAndAtomicRollbackRefusal(t *testing.T) {
	pool, migrator := semanticProductionDatabase(t, "source22_legacy", 21)
	workspace, source, _, run := seedSourceHistoryRun(t, pool)
	if _, err := pool.Exec(context.Background(), `UPDATE source_connections SET name='Current name' WHERE workspace_id=$1 AND id=$2`, workspace.UUID(), source.UUID()); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	var quality, status string
	var members int
	if err := pool.QueryRow(context.Background(), `SELECT s.history_quality,s.coverage_status,(SELECT count(*) FROM source_snapshot_members m WHERE m.workspace_id=s.workspace_id AND m.snapshot_id=s.id) FROM source_snapshots s JOIN source_snapshot_runs r ON r.workspace_id=s.workspace_id AND r.snapshot_id=s.id WHERE r.run_id=$1`, run.UUID()).Scan(&quality, &status, &members); err != nil {
		t.Fatal(err)
	}
	if quality != "unverifiable" || status != "partial" || members != 0 {
		t.Fatalf("fabricated legacy quality=%s status=%s members=%d", quality, status, members)
	}
	if err := migrator.Steps(-1); err == nil || !strings.Contains(err.Error(), "ERROR: DOWN_MIGRATION_UNSAFE:") {
		t.Fatalf("unrepresentable history must return DOWN_MIGRATION_UNSAFE: %v", err)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM source_snapshot_runs WHERE run_id=$1`, run.UUID()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("failed downgrade lost history=%d err=%v", count, err)
	}
	var dirty bool
	if err := pool.QueryRow(context.Background(), `SELECT dirty FROM schema_migrations`).Scan(&dirty); err != nil || !dirty {
		t.Fatalf("failed downgrade dirty=%v err=%v", dirty, err)
	}
}

func TestSemanticProductionMigrationRejectsIncompleteVerifiedSeal(t *testing.T) {
	pool, _ := semanticProductionDatabase(t, "source22_constraints", 22)
	workspace, source, revision, run := seedSourceHistoryRun(t, pool)
	snapshot, _ := identity.New(identity.SourceSnapshot)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(context.Background(), `INSERT INTO source_snapshots(id,workspace_id,source_connection_id,source_revision_id,adapter_version,scope_digest,content_digest,history_quality,coverage_status) VALUES($1,$2,$3,$4,'1.0.0',$5,$5,'verified','complete')`, snapshot.UUID(), workspace.UUID(), source.UUID(), revision.UUID(), distributionDigest("forged")); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), `INSERT INTO source_snapshot_runs(workspace_id,source_connection_id,run_id,snapshot_id) VALUES($1,$2,$3,$4)`, workspace.UUID(), source.UUID(), run.UUID(), snapshot.UUID()); err != nil {
		return
	}
	if err = tx.Commit(context.Background()); err == nil {
		t.Fatal("verified complete snapshot sealed without any declared coverage")
	}
}

func TestSemanticProductionMigrationCoverageAttemptBlocksDowngrade(t *testing.T) {
	pool, migrator := semanticProductionDatabase(t, "source22_attempt_only", 22)
	workspace, source, _, run := seedSourceHistoryRun(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE discovery_runs SET status='running',completed_at=NULL WHERE id=$1`, run.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO source_coverage_heads(workspace_id,source_connection_id,coverage_key,selector_digest,latest_attempt_run_id,latest_attempt_status,version) VALUES($1,$2,'catalog',$3,$4,'partial',1)`, workspace.UUID(), source.UUID(), distributionDigest("selector"), run.UUID()); err != nil {
		t.Fatal(err)
	}
	var heads, snapshots int
	var before string
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM source_coverage_heads),(SELECT count(*) FROM source_snapshots),(SELECT md5(row_to_json(h)::text) FROM source_coverage_heads h)`).Scan(&heads, &snapshots, &before); err != nil || heads != 1 || snapshots != 0 {
		t.Fatalf("attempt fixture heads=%d snapshots=%d err=%v", heads, snapshots, err)
	}
	inventory := strings.Join(tableInventory(t, pool), ",")
	if err := migrator.Steps(-1); err == nil || !strings.Contains(err.Error(), "ERROR: DOWN_MIGRATION_UNSAFE:") {
		t.Fatalf("authoritative attempt downgrade was not refused: %v", err)
	}
	var after string
	var dirty bool
	if err := pool.QueryRow(ctx, `SELECT (SELECT md5(row_to_json(h)::text) FROM source_coverage_heads h),(SELECT dirty FROM schema_migrations)`).Scan(&after, &dirty); err != nil || after != before || !dirty {
		t.Fatalf("refusal changed attempt or hid dirty state before=%s after=%s dirty=%v err=%v", before, after, dirty, err)
	}
	if got := strings.Join(tableInventory(t, pool), ","); got != inventory {
		t.Fatalf("unsafe downgrade changed business table inventory: %s", got)
	}
}

func TestSemanticProductionAuthoringMigrationLifecycle(t *testing.T) {
	_, migrator := semanticProductionDatabase(t, "authoring23_lifecycle", 23)
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("empty authoring migration 23 downgrade failed: %v", err)
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatalf("re-applying authoring migration 23 failed: %v", err)
	}
}

func TestSemanticProductionAuthoringMigrationBlocksUnsafeDowngrade(t *testing.T) {
	pool, migrator := semanticProductionDatabase(t, "authoring23_unsafe", 23)
	ctx := context.Background()
	workspace, _ := identity.New(identity.Workspace)
	user, _ := identity.New(identity.UserAccount)
	opID, _ := identity.New(identity.ProductionOperation)
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces(id,slug,display_name) VALUES($1,'authoring-down','Authoring down')`, workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_accounts(id,display_name) VALUES($1,'Author')`, user.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO production_operations(id,workspace_id,created_by,current_version) VALUES($1,$2,$3,1)`, opID.UUID(), workspace.UUID(), user.UUID()); err != nil {
		t.Fatal(err)
	}

	if err := migrator.Steps(-1); err == nil || !strings.Contains(err.Error(), "ERROR: DOWN_MIGRATION_UNSAFE:") {
		t.Fatalf("unrepresentable authoring history must return DOWN_MIGRATION_UNSAFE: %v", err)
	}
}

func TestSemanticProductionPublishingMigrationLifecycle(t *testing.T) {
	_, migrator := semanticProductionDatabase(t, "publishing24_lifecycle", 24)
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("empty publishing migration 24 downgrade failed: %v", err)
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatalf("re-applying publishing migration 24 failed: %v", err)
	}
}

func TestSemanticProductionReleaseIntegrityMigrationLifecycle(t *testing.T) {
	pool, migrator := semanticProductionDatabase(t, "publishing26_lifecycle", 26)
	var seals, proof bool
	if err := pool.QueryRow(context.Background(), `SELECT to_regclass('production_validation_seals') IS NOT NULL,to_regclass('production_release_integrity') IS NOT NULL`).Scan(&seals, &proof); err != nil || !seals || !proof {
		t.Fatalf("missing integrity schema: %v %v %v", seals, proof, err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 25, true)
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 26, true)
}

func TestSemanticProductionPublishingMigrationBlocksUnsafeDowngrade(t *testing.T) {
	pool, migrator := semanticProductionDatabase(t, "publishing24_unsafe", 24)
	ctx := context.Background()
	workspace, _ := identity.New(identity.Workspace)
	user, _ := identity.New(identity.UserAccount)
	opID, _ := identity.New(identity.ProductionOperation)
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces(id,slug,display_name) VALUES($1,'publishing-down','Publishing down')`, workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_accounts(id,display_name) VALUES($1,'Publisher')`, user.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO production_operations(id,workspace_id,created_by,current_version) VALUES($1,$2,$3,1)`, opID.UUID(), workspace.UUID(), user.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO production_versions(workspace_id,operation_id,version,input_json,input_digest,request_digest,set_digest,created_by)
VALUES($1,$2,1,'{}','sha256:1111111111111111111111111111111111111111111111111111111111111111','sha256:1111111111111111111111111111111111111111111111111111111111111111','sha256:1111111111111111111111111111111111111111111111111111111111111111',$3)`, workspace.UUID(), opID.UUID(), user.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO production_validation_attempts(workspace_id,operation_id,production_version,attempt_no,set_digest,input_digest,freshness_witness_json,freshness_digest,required_checks_json,required_checks_digest,status,created_by)
VALUES($1,$2,1,1,'sha256:1111111111111111111111111111111111111111111111111111111111111111','sha256:1111111111111111111111111111111111111111111111111111111111111111','{}','sha256:1111111111111111111111111111111111111111111111111111111111111111','[]','sha256:1111111111111111111111111111111111111111111111111111111111111111','queued',$3)`, workspace.UUID(), opID.UUID(), user.UUID()); err != nil {
		t.Fatal(err)
	}

	if err := migrator.Steps(-1); err == nil || !strings.Contains(err.Error(), "ERROR: DOWN_MIGRATION_UNSAFE:") {
		t.Fatalf("unrepresentable publishing history must return DOWN_MIGRATION_UNSAFE: %v", err)
	}
}
