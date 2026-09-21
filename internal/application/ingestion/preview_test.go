package ingestion

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/iiwish/semlia/internal/adapters/discovery/files"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	discovery "github.com/iiwish/semlia/internal/domain/discovery"
	domain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
	"testing"
	"time"
)

func TestPreviewRequiresImmutableMembershipAvailabilityAndDigest(t *testing.T) {
	w, _ := identity.NewWorkspaceID()
	source, _ := identity.NewSourceConnectionID()
	a, _ := identity.NewArtifactID()
	set, _ := identity.NewArtifactSetID()
	content := []byte("id,name\n1,example\n")
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
	artifact := domain.Artifact{ID: a, WorkspaceID: w, Kind: domain.ArtifactCSV, ContentDigest: digest, ByteSize: int64(len(content)), ContentAvailability: domain.ContentAvailable}
	repo := &serviceRepository{setSource: source, artifacts: map[identity.ArtifactID]domain.Artifact{a: artifact}, set: domain.ArtifactSet{ID: set, WorkspaceID: w, SourceConnectionID: source, Members: []domain.ArtifactMember{{ArtifactID: a, Kind: domain.ArtifactCSV, LogicalPath: "sample.csv", ContentDigest: digest, ByteSize: int64(len(content)), ContentAvailability: domain.ContentAvailable}}}}
	store := &memoryArtifactStore{values: map[string][]byte{digest: content}}
	evaluator := &serviceEvaluator{version: 7}
	svc := NewService(repo, store, evaluator, ClockFunc(time.Now), map[domain.ArtifactKind]discovery.Adapter{domain.ArtifactCSV: files.CSV()})
	p, err := svc.Preview(context.Background(), w, set, a, "admin", "")
	if err != nil || p.ContentDigest != digest || len(evaluator.requests) != 2 {
		t.Fatalf("preview=%+v err=%v auth=%d", p, err, len(evaluator.requests))
	}
	other, _ := identity.NewArtifactID()
	if _, err = svc.Preview(context.Background(), w, set, other, "admin", ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("nonmember: %v", err)
	}
	otherWorkspace, _ := identity.NewWorkspaceID()
	if _, err = svc.Preview(context.Background(), otherWorkspace, set, a, "admin", ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("workspace: %v", err)
	}
	expired := time.Now().Add(-time.Hour)
	artifact.ExpiresAt = &expired
	repo.artifacts[a] = artifact
	if _, err = svc.Preview(context.Background(), w, set, a, "admin", ""); !errors.Is(err, domain.ErrContentUnavailable) {
		t.Fatalf("expired: %v", err)
	}
	artifact.ExpiresAt = nil
	repo.artifacts[a] = artifact
	store.values[digest] = []byte("id,name\n1,tampered\n")
	if _, err = svc.Preview(context.Background(), w, set, a, "admin", ""); !errors.Is(err, domain.ErrStore) {
		t.Fatalf("tampered: %v", err)
	}
	store.values[digest] = content
	for _, allowed := range []int{0, 1} {
		svc.authorizer = &revokedPreviewEvaluator{allowed: allowed}
		p, err = svc.Preview(context.Background(), w, set, a, "admin", "")
		var denial *authorization.DenialError
		if !errors.As(err, &denial) || len(p.Sheets) != 0 {
			t.Fatalf("revocation returned content: %+v, %v", p, err)
		}
	}
}

type revokedPreviewEvaluator struct{ calls, allowed int }

func (e *revokedPreviewEvaluator) Evaluate(_ context.Context, r authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	e.calls++
	return authorization.Decision{Allowed: e.calls <= e.allowed, Action: r.Action}, nil
}
