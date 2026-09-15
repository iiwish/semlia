package governance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/iiwish/semlia/pkg/identity"
)

func productionAssertLatePublishFailureAtomic(t *testing.T, f *fixture, h http.Handler, w identity.WorkspaceID, p identity.PrincipalID, path string, body any) {
	t.Helper()
	ctx := context.Background()
	snapshot := func() string {
		state := map[string]json.RawMessage{}
		for _, table := range []string{"semantic_assets", "asset_revisions", "physical_bindings", "model_grains", "entity_keys", "join_contracts", "proposals", "reviews", "releases", "release_assets", "release_objects", "release_object_snapshots", "release_proposals", "production_release_manifests", "production_release_before_pins", "production_release_binding_inputs", "production_release_integrity", "production_commands", "audit_events", "outbox_events"} {
			var raw []byte
			if err := f.pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb) FROM `+table+` t WHERE workspace_id=$1`, w.UUID()).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			state[table] = raw
		}
		raw, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	before := snapshot()
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION production_test_late_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected late production publication failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(ctx, `DROP FUNCTION IF EXISTS production_test_late_failure() CASCADE`) })
	if _, err := f.pool.Exec(ctx, `CREATE CONSTRAINT TRIGGER production_test_late_failure AFTER INSERT ON production_release_binding_inputs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION production_test_late_failure()`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	r := sendProdRequest(h, http.MethodPost, path, p.String(), "composite-publish", string(raw))
	if r.Code < 500 {
		t.Fatalf("late SQL failure was not propagated: %d %s", r.Code, r.Body.String())
	}
	if after := snapshot(); after != before {
		t.Fatal("late failure leaked release, content, pointers, proposal transitions, receipt or events")
	}
	if _, err := f.pool.Exec(ctx, `DROP FUNCTION production_test_late_failure() CASCADE`); err != nil {
		t.Fatal(err)
	}
}
