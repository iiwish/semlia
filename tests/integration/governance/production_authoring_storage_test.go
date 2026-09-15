package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestProductionAuthoringStorageFencesAndEvents(t *testing.T) {
	f, h, w, p := authoringLifecycleSetup(t)
	input := productionFixtureInput(t, f, w, identity.SemanticCandidateID{})
	inputJSON, _ := json.Marshal(input)
	body := completeProductionPayload(t, authoringLifecycleBody(string(inputJSON)), p)
	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w)
	r := sendProdRequest(h, http.MethodPost, path, p.String(), "storage-create", body)
	if r.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", r.Code, r.Body.String())
	}
	created := authoringLifecycleDecode(t, r.Body.Bytes())
	proposal := mustParseProposalID(t, created["proposalIds"].([]any)[0].(string))
	for _, test := range []struct{ name, sql string }{
		{"content", `UPDATE proposals SET title='tampered' WHERE workspace_id=$1 AND id=$2`},
		{"legacy submit", `UPDATE proposals SET state='proposed',submitted_at=clock_timestamp() WHERE workspace_id=$1 AND id=$2`},
		{"catalog lifecycle", `UPDATE semantic_assets SET lifecycle_state='active' WHERE workspace_id=$1 AND id=(SELECT target_object_id FROM proposals WHERE workspace_id=$1 AND id=$2)`},
		{"late changes", `INSERT INTO proposal_changes(id,workspace_id,proposal_id,field_path,op,after_value,after_digest) SELECT uuidv7(),workspace_id,id,'definition','add','"bypass"','sha256:'||repeat('0',64) FROM proposals WHERE workspace_id=$1 AND id=$2`},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.pool.Exec(context.Background(), test.sql, w.UUID(), proposal.UUID())
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" || !strings.Contains(pgErr.Message, "production") {
				t.Fatalf("missing production storage fence: %v", err)
			}
		})
	}
	for _, table := range []string{"audit_events", "outbox_events"} {
		var count int
		if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table+` WHERE workspace_id=$1 AND event_type LIKE 'production.operation.%'`, w.UUID()).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s atomic event count=%d: %v", table, count, err)
		}
	}
	r = sendProdRequest(h, http.MethodPost, path, p.String(), "storage-create", body)
	if r.Code != http.StatusOK {
		t.Fatalf("replay: %d %s", r.Code, r.Body.String())
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE workspace_id=$1 AND event_type='production.operation.created'`, w.UUID()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("replay duplicated audit fact: %d %v", count, err)
	}
}
