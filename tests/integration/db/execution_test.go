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

func TestMigration21EmptyDownUpAndHistoryGuard(t *testing.T) {
	ctx := context.Background()
	admin := openPool(t)
	for _, history := range []bool{false, true} {
		name := "migration21_empty"
		if history {
			name = "migration21_history"
		}
		t.Run(name, func(t *testing.T) {
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
			if err := migrator.Steps(21); err != nil {
				t.Fatal(err)
			}
			pool, err := pgstore.Open(ctx, u.String())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(pool.Close)
			assertVersion(t, migrator, 21, true)
			if !history {
				if err := migrator.Steps(-1); err != nil {
					t.Fatal(err)
				}
				assertVersion(t, migrator, 20, true)
				var definition, constraint string
				if err := pool.QueryRow(ctx, `SELECT pg_get_functiondef('capture_release_object_snapshot()'::regprocedure),pg_get_constraintdef(oid) FROM pg_constraint WHERE conname='resolved_semantic_plans_execution_status_check'`).Scan(&definition, &constraint); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(definition, "ORDER BY created_at,release_id") || strings.Contains(constraint, "requires_execution_validation") {
					t.Fatal("downgrade retained migration21 function or status")
				}
				if err := migrator.Steps(1); err != nil {
					t.Fatal(err)
				}
				assertVersion(t, migrator, 21, true)
				t.Log("real migrator 21 -> 20 -> 21 restored capture function and status constraints")
				return
			}
			workspace := newWorkspaceID(t)
			release, _ := identity.NewReleaseID()
			dataset, _ := identity.NewPhysicalDatasetID()
			if _, err := pool.Exec(ctx, `INSERT INTO workspaces(id,slug,display_name) VALUES($1,'guard','Guard')`, workspace.UUID()); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO releases(id,workspace_id,sequence,manifest_digest,published_by,published_at) VALUES($1,$2,1,$3,'fixture',CURRENT_TIMESTAMP)`, release.UUID(), workspace.UUID(), distributionDigest("guard")); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO release_execution_relations VALUES($1,$2,$3,'{"immutable":"retained"}')`, workspace.UUID(), release.UUID(), dataset.UUID()); err != nil {
				t.Fatal(err)
			}
			if err := migrator.Steps(-1); err == nil {
				t.Fatal("populated downgrade succeeded")
			}
			var version int64
			var dirty bool
			err = pool.QueryRow(ctx, `SELECT version,dirty FROM schema_migrations`).Scan(&version, &dirty)
			if err != nil || !dirty {
				t.Fatalf("failed migration must require operator repair: version=%d dirty=%v error=%v", version, dirty, err)
			}
			var payload string
			if err := pool.QueryRow(ctx, `SELECT payload->>'immutable' FROM release_execution_relations`).Scan(&payload); err != nil || payload != "retained" {
				t.Fatalf("failed down lost pin: %q %v", payload, err)
			}
			var functionExists bool
			if err := pool.QueryRow(ctx, `SELECT to_regprocedure('capture_release_execution_relation()') IS NOT NULL`).Scan(&functionExists); err != nil || !functionExists {
				t.Fatal("failed down removed capture function")
			}
			t.Log("real migrator populated down refused atomically; immutable pin remains; migration dirty marker honestly requires operator repair")
		})
	}
}

func TestMigration21LegacyBindingCannotBorrowNewPhysicalPin(t *testing.T) {
	ctx := context.Background()
	admin := openPool(t)
	name := "migration21_legacy"
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Error(err)
		}
	})
	u, _ := url.Parse(databaseURL)
	u.Path = "/" + name
	migrator, err := pgstore.NewMigrator(u.String(), filepath.Join(repoRoot, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close() })
	if err := migrator.Steps(20); err != nil {
		t.Fatal(err)
	}
	pool, err := pgstore.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := seedDistributionReleases(t, pool)
	if err := migrator.Up(); err != nil {
		t.Fatal(err)
	}
	srv, _ := identity.NewSourceRevisionID()
	dr, _ := identity.NewPhysicalDatasetRevisionID()
	extra, _ := identity.NewAssetID()
	binding, _ := identity.NewPhysicalBindingID()
	release, _ := identity.NewReleaseID()
	var source string
	if err := pool.QueryRow(ctx, `SELECT source_connection_id::text FROM physical_datasets WHERE id=$1`, f.dataset2.UUID()).Scan(&source); err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		sql  string
		args []any
	}{
		{`UPDATE source_connections SET adapter_kind='postgresql_catalog' WHERE id=$1`, []any{source}},
		{`INSERT INTO source_revisions(id,workspace_id,source_connection_id,content_digest,adapter_version,observed_at) VALUES($1,$2,$3,$4,'fixture',CURRENT_TIMESTAMP)`, []any{srv.UUID(), f.workspaceID.UUID(), source, distributionDigest("legacy-source")}},
		{`INSERT INTO physical_dataset_revisions(id,workspace_id,physical_dataset_id,source_revision_id,dataset_kind,locator,content_digest) VALUES($1,$2,$3,$4,'table','analytics.orders_v2',$5)`, []any{dr.UUID(), f.workspaceID.UUID(), f.dataset2.UUID(), srv.UUID(), distributionDigest("legacy-dataset")}},
		{`UPDATE physical_datasets SET current_revision_id=$2 WHERE id=$1`, []any{f.dataset2.UUID(), dr.UUID()}},
		{`INSERT INTO semantic_assets(id,workspace_id,namespace,key,asset_type,lifecycle_state) VALUES($1,$2,'fixture','second','metric','active')`, []any{extra.UUID(), f.workspaceID.UUID()}},
		{`INSERT INTO physical_bindings(id,workspace_id,asset_id,dataset_id,created_by) VALUES($1,$2,$3,$4,'fixture')`, []any{binding.UUID(), f.workspaceID.UUID(), extra.UUID(), f.dataset2.UUID()}},
		{`INSERT INTO releases(id,workspace_id,sequence,manifest_digest,published_by,published_at) VALUES($1,$2,3,$3,'fixture',CURRENT_TIMESTAMP)`, []any{release.UUID(), f.workspaceID.UUID(), distributionDigest("legacy-carry")}},
		{`INSERT INTO release_objects(workspace_id,release_id,object_type,object_id,version,position) VALUES($1,$2,'physical_binding',$3,2,1),($1,$2,'physical_binding',$4,1,2)`, []any{f.workspaceID.UUID(), release.UUID(), f.bindingID.UUID(), binding.UUID()}},
	}
	for _, step := range steps {
		if _, err := pool.Exec(ctx, step.sql, step.args...); err != nil {
			t.Fatal(err)
		}
	}
	var oldPins, newPins, relations int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE binding_id=$2),count(*) FILTER(WHERE binding_id=$3) FROM release_execution_binding_pins WHERE release_id=$1`, release.UUID(), f.bindingID.UUID(), binding.UUID()).Scan(&oldPins, &newPins); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM release_execution_relations WHERE release_id=$1`, release.UUID()).Scan(&relations); err != nil {
		t.Fatal(err)
	}
	if oldPins != 0 || newPins != 1 || relations != 1 {
		t.Fatalf("legacy physical backfill: old=%d new=%d relations=%d", oldPins, newPins, relations)
	}
	t.Log("schema20 legacy binding remains without a pin while new same-dataset binding freezes one under schema21")
}
