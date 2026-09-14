package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application"
	discoveryapp "github.com/iiwish/semlia/internal/application/discovery"
	"github.com/iiwish/semlia/internal/domain"
	"github.com/iiwish/semlia/internal/domain/authorization"
	discoverydomain "github.com/iiwish/semlia/internal/domain/discovery"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/trace/noop"
)

type snapshotHTTPRepository struct {
	discoveryapp.SourceRepository
	workspace identity.WorkspaceID
	source    identity.SourceConnectionID
	snapshot  string
	reads     int
}

func (repository *snapshotHTTPRepository) LookupSourceSnapshot(_ context.Context, workspace identity.WorkspaceID, source identity.SourceConnectionID, snapshot string) error {
	if workspace != repository.workspace || source != repository.source || (snapshot != "" && snapshot != repository.snapshot) {
		return discoverydomain.ErrNotFound
	}
	return nil
}
func (repository *snapshotHTTPRepository) ReadSourceSnapshot(_ context.Context, query discoveryapp.SnapshotReadQuery) (discoveryapp.SnapshotReadResult, error) {
	repository.reads++
	return discoveryapp.SnapshotReadResult{Snapshots: []discoverydomain.SourceSnapshot{{ID: repository.snapshot, SourceID: repository.source.String(), HistoryQuality: "verified", Coverage: []discoverydomain.CoverageUnit{}}}, Members: []discoverydomain.SnapshotMember{}, Diagnostics: []discoverydomain.SnapshotDiagnostic{}, HistoryQuality: "verified", Cursor: query.Cursor}, nil
}

func TestSourceSnapshotRoutesReauthorizeAndHideWrongRelations(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	source, _ := identity.NewSourceConnectionID()
	snapshot, _ := identity.New(identity.SourceSnapshot)
	other, _ := identity.NewWorkspaceID()
	repository := &snapshotHTTPRepository{workspace: workspace, source: source, snapshot: snapshot.String()}
	loader, err := discoveryapp.NewArtifactLoader("")
	if err != nil {
		t.Fatal(err)
	}
	evaluator := &stubEvaluator{decision: denialDecision()}
	control := discoveryapp.NewControlService(repository, nil, nil, loader, nil, evaluator, discoveryapp.ClockFunc(time.Now))
	system := application.NewSystemService(application.UnavailableReadinessProbe{}, domain.SystemInfo{APIVersion: "v1", SchemaVersion: "0.1.0", BuildVersion: "test"})
	handler := httpapi.NewHandler(system, slog.New(slog.NewTextHandler(io.Discard, nil)), noop.NewTracerProvider().Tracer("snapshot-test"), httpapi.WithDiscovery(control))
	base := "/api/v1/workspaces/" + workspace.String() + "/sources/" + source.String() + "/snapshots"
	for _, suffix := range []string{"", "/" + snapshot.String(), "/" + snapshot.String() + "/members", "/" + snapshot.String() + "/diagnostics"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, base+suffix, nil))
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d body=%s", suffix, response.Code, response.Body)
		}
		if evaluator.captured.Action != authorization.ActionSourceRead || evaluator.captured.Resource.ID != source.UUID() {
			t.Fatalf("wrong authorization: %+v", evaluator.captured)
		}
		wrong := httptest.NewRecorder()
		handler.ServeHTTP(wrong, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+other.String()+"/sources/"+source.String()+"/snapshots"+suffix, nil))
		if wrong.Code != http.StatusNotFound {
			t.Fatalf("cross workspace status=%d body=%s", wrong.Code, wrong.Body)
		}
	}
	if repository.reads != 0 {
		t.Fatalf("denied content reads=%d", repository.reads)
	}
	evaluator.decision = authorization.Decision{Allowed: true, AuthorizationVersion: 2}
	for _, suffix := range []string{"", "/" + snapshot.String(), "/" + snapshot.String() + "/members", "/" + snapshot.String() + "/diagnostics"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, base+suffix, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("allowed status=%d body=%s", response.Code, response.Body)
		}
	}
}
