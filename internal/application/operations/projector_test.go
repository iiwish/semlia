package operations_test

import (
	"context"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application/operations"
	domain "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/pkg/identity"
)

type projectionWriter struct {
	called int
	value  operations.Projection
}

func (writer *projectionWriter) UpsertRuntimeProjection(_ context.Context, value operations.Projection) error {
	writer.called++
	writer.value = value
	return nil
}

func TestProjectorWritesTypedProjectionWithoutJobPayload(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	runID, _ := identity.NewRunID()
	eventID, _ := identity.NewEventID()
	now := time.Now().UTC()
	projection := operations.Projection{Run: domain.RuntimeRun{
		ID: runID, WorkspaceID: workspace, Kind: domain.RunKindSemanticResolution,
		SourceType: "semantic_query", SourceID: runID.UUID(), SourceVersionDigest: string(make([]byte, 64)),
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", IdempotencyKey: "semantic-resolution", State: domain.RunSucceeded,
		MaxAttempts: 1, FinishedAt: &now, CreatedAt: now, UpdatedAt: now,
	}, Event: &domain.RuntimeRunEvent{ID: eventID, WorkspaceID: workspace, RunID: runID,
		EventKey: "completed", Type: domain.RunEventState, State: domain.RunSucceeded, CreatedAt: now}}
	projection.Run.SourceVersionDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	w := &projectionWriter{}
	if err := operations.NewProjector().Project(context.Background(), w, projection); err != nil {
		t.Fatal(err)
	}
	if w.called != 1 || w.value.Run.Kind != domain.RunKindSemanticResolution {
		t.Fatalf("projection not written: %+v", w.value)
	}
}
