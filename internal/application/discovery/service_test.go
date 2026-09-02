package discovery_test

import (
	"context"
	"errors"
	"testing"
	"time"

	application "github.com/iiwish/semlia/internal/application/discovery"
	domain "github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestServiceSelectsAdapterAndPersistsSnapshot(t *testing.T) {
	repository := &fakeRepository{}
	service, err := application.NewService(repository, fakeAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Discover(context.Background(), application.Request{
		WorkspaceID: mustID(t, identity.NewWorkspaceID), SourceConnectionID: mustID(t, identity.NewSourceConnectionID),
		AdapterKind: "fixture", Input: domain.Input{Locator: "fixture", ObservedAt: time.Now(), Files: map[string][]byte{"fixture": []byte("ok")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DatasetCount != 1 || repository.calls != 1 {
		t.Fatalf("result/calls = %+v/%d", result, repository.calls)
	}
	if _, err := service.Discover(context.Background(), application.Request{AdapterKind: "missing"}); !errors.Is(err, application.ErrAdapterNotFound) {
		t.Fatalf("missing adapter error = %v", err)
	}
}

type fakeAdapter struct{}

func (fakeAdapter) Kind() string    { return "fixture" }
func (fakeAdapter) Version() string { return "1.0.0" }
func (fakeAdapter) Discover(_ context.Context, input domain.Input) (domain.Snapshot, error) {
	return domain.Snapshot{
		AdapterKind: "fixture", AdapterVersion: "1.0.0", Locator: input.Locator,
		ContentDigest: input.ContentDigest(), ObservedAt: input.ObservedAt,
		Datasets: []domain.Dataset{{ExternalKey: "one", QualifiedName: "public.one", Kind: "table", Locator: "public.one"}},
	}, nil
}

type fakeRepository struct{ calls int }

func (repository *fakeRepository) PersistDiscoverySnapshot(
	_ context.Context,
	_ identity.WorkspaceID,
	_ identity.SourceConnectionID,
	snapshot domain.Snapshot,
) (application.PersistResult, error) {
	repository.calls++
	return application.PersistResult{DatasetCount: len(snapshot.Datasets)}, nil
}

func mustID[T any](t *testing.T, create func() (T, error)) T {
	t.Helper()
	value, err := create()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
