package httpapi_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	files "github.com/iiwish/semlia/internal/adapters/discovery/files"
	app "github.com/iiwish/semlia/internal/application/ingestion"
	discovery "github.com/iiwish/semlia/internal/domain/discovery"
	domain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type previewHTTPRepository struct {
	app.Repository
	set      domain.ArtifactSet
	artifact domain.Artifact
}

func (r *previewHTTPRepository) LookupArtifactSetSource(_ context.Context, w identity.WorkspaceID, s identity.ArtifactSetID) (identity.SourceConnectionID, error) {
	if w != r.set.WorkspaceID || s != r.set.ID {
		return identity.SourceConnectionID{}, domain.ErrNotFound
	}
	return r.set.SourceConnectionID, nil
}
func (r *previewHTTPRepository) GetArtifactSet(_ context.Context, _ identity.WorkspaceID, _ identity.ArtifactSetID, _ int64) (domain.ArtifactSet, error) {
	return r.set, nil
}
func (r *previewHTTPRepository) GetArtifact(_ context.Context, _ identity.WorkspaceID, _ identity.ArtifactID) (domain.Artifact, error) {
	return r.artifact, nil
}
func TestArtifactPreviewRouteIsReadOnlyScopedAndUncached(t *testing.T) {
	w, _ := identity.NewWorkspaceID()
	s, _ := identity.NewSourceConnectionID()
	a, _ := identity.NewArtifactID()
	set, _ := identity.NewArtifactSetID()
	content := []byte("id,name\n1,sample\n")
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
	repo := &previewHTTPRepository{set: domain.ArtifactSet{ID: set, WorkspaceID: w, SourceConnectionID: s, Members: []domain.ArtifactMember{{ArtifactID: a, Kind: domain.ArtifactCSV, LogicalPath: "sample.csv", ContentDigest: digest, ByteSize: int64(len(content)), ContentAvailability: domain.ContentAvailable}}}, artifact: domain.Artifact{ID: a, WorkspaceID: w, Kind: domain.ArtifactCSV, ContentDigest: digest, ByteSize: int64(len(content)), ContentAvailability: domain.ContentAvailable}}
	service := app.NewService(repo, &ingestionHTTPStore{values: map[string][]byte{digest: content}}, nil, app.ClockFunc(time.Now), map[domain.ArtifactKind]discovery.Adapter{domain.ArtifactCSV: files.CSV()})
	handler := ingestionHTTPHandler(t, service)
	path := "/api/v1/workspaces/" + w.String() + "/ingestion/artifact-sets/" + set.String() + "/members/" + a.String() + "/preview"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
	if response.Code != 405 {
		t.Fatalf("write accepted %d", response.Code)
	}
	repo.artifact.ContentAvailability = domain.ContentExpired
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != 410 {
		t.Fatalf("expired=%d %s", response.Code, response.Body.String())
	}
}
