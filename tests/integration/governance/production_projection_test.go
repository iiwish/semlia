package governance_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/iiwish/semlia/internal/adapters/gitcontent"
	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/internal/application/projection"
	catalogdomain "github.com/iiwish/semlia/internal/domain/catalog"
	"github.com/iiwish/semlia/pkg/identity"
)

func productionAssertCompositeProjection(t *testing.T, f *fixture, w identity.WorkspaceID, head map[string]any, depth int) {
	t.Helper()
	id, err := identity.ParseReleaseID(head["releaseId"].(string))
	if err != nil {
		t.Fatal(err)
	}
	projected, err := f.store.LoadReleaseProjection(context.Background(), w, id)
	if err != nil {
		t.Fatal(err)
	}
	if projected.Production == nil || len(projected.Production.Proposals) != 5 || projected.Production.RollbackDepth != depth {
		t.Fatalf("projection lost complete protected attribution: %+v", projected.Production)
	}
	for _, attr := range projected.Production.Proposals {
		if attr.OperationID == "" || attr.ValidationDigest == "" || attr.AttemptNo != 1 || string(attr.ReviewIDs) == "[]" {
			t.Fatalf("projection lost approval bindings: %+v", attr)
		}
	}
	root := t.TempDir()
	writer, err := gitcontent.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	// Consume every persisted release event, including intermediate rollbacks.
	rows, err := f.pool.Query(context.Background(), `SELECT payload,trace_id FROM outbox_events WHERE workspace_id=$1 AND event_type='release.published' ORDER BY (payload->'data'->>'sequence')::bigint`, w.UUID())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	publisher := projection.NewReleasePublisher(f.store, writer)
	count := 0
	for rows.Next() {
		event := jobs.OutboxEvent{WorkspaceID: w, Type: projection.ReleasePublished}
		if err := rows.Scan(&event.Payload, &event.TraceID); err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(event.Payload, &envelope); err != nil {
			t.Fatal(err)
		}
		event.ID, err = identity.ParseEventID(envelope.ID)
		if err != nil {
			t.Fatal(err)
		}
		for attempt := 0; attempt < 2; attempt++ {
			if err := publisher.Publish(context.Background(), event); err != nil {
				t.Fatalf("release outbox %d replay %d: %v; payload=%s", count, attempt, err, event.Payload)
			}
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != depth+1 {
		t.Fatalf("release outbox count=%d want=%d", count, depth+1)
	}
	result, err := writer.ProjectRelease(context.Background(), projected)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, result.Path))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Spec struct {
			Production struct {
				Proposals     []any `json:"proposals"`
				RollbackDepth int   `json:"rollbackDepth"`
			} `json:"production"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Spec.Production.Proposals) != 5 || doc.Spec.Production.RollbackDepth != depth {
		t.Fatalf("Git document dropped production metadata: %s", raw)
	}
	replay, err := writer.ProjectRelease(context.Background(), projected)
	if err != nil || replay.Changed {
		t.Fatalf("projection replay not stable: %+v %v", replay, err)
	}
	for _, asset := range projected.Assets {
		for _, section := range []string{"physical_bindings", "join_contracts"} {
			_, err := f.store.ListCatalogAuthorityRecords(context.Background(), catalogdomain.ListAuthorityRecordsQuery{WorkspaceID: w, AssetID: asset.AssetID, Section: section, Limit: 50})
			if err != nil {
				t.Fatalf("published catalog authority %s: %v", section, err)
			}
		}
	}
}
