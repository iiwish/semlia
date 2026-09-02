package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	application "github.com/iiwish/semlia/internal/application/catalog"
	domain "github.com/iiwish/semlia/internal/domain/catalog"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestAssetCursorIsDeterministicAndBoundToFilters(t *testing.T) {
	repository := &fakeRepository{}
	workspace := mustID(t, identity.NewWorkspaceID)
	for index := 0; index < 3; index++ {
		assetID := mustID(t, identity.NewAssetID)
		address, _ := semantic.NewAddress("commerce", []string{"alpha", "beta", "gamma"}[index])
		repository.assets = append(repository.assets, domain.AssetSummary{
			ID: assetID, Address: address, Type: semantic.Metric, LifecycleState: "active",
			UpdatedAt: time.Date(2026, 9, 2, 10, index, 0, 0, time.UTC), Rank: float64(3 - index),
		})
	}
	service := application.NewService(repository, application.ClockFunc(time.Now))
	first, err := service.ListAssets(context.Background(), application.ListAssetsRequest{
		WorkspaceID: workspace, Search: "revenue", AssetType: semantic.Metric, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	if _, err := service.ListAssets(context.Background(), application.ListAssetsRequest{
		WorkspaceID: workspace, Search: "different", AssetType: semantic.Metric,
		Limit: 2, Cursor: first.NextCursor,
	}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("filter-mismatched cursor error = %v", err)
	}
	second, err := service.ListAssets(context.Background(), application.ListAssetsRequest{
		WorkspaceID: workspace, Search: "revenue", AssetType: semantic.Metric,
		Limit: 2, Cursor: first.NextCursor,
	})
	if err != nil || repository.lastAssetQuery.Cursor == nil {
		t.Fatalf("second page/query = %+v / %+v / %v", second, repository.lastAssetQuery, err)
	}
}

func TestCreateAssetCanonicalizesContentAndDefaultsLifecycle(t *testing.T) {
	repository := &fakeRepository{}
	createdAt := time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC)
	service := application.NewService(repository, application.ClockFunc(func() time.Time { return createdAt }))
	workspace := mustID(t, identity.NewWorkspaceID)
	_, err := service.CreateAsset(context.Background(), application.CreateAssetRequest{
		WorkspaceID: workspace, Address: "commerce.net_revenue", AssetType: semantic.Metric,
		SchemaVersion: "1.0.0", Content: json.RawMessage(`{"z":1,"name":"Net revenue"}`),
		CreatedBy: "founder", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
	})
	if err != nil {
		t.Fatal(err)
	}
	command := repository.created
	if command.Lifecycle != "draft" || command.ContentDigest == "" ||
		string(command.Content) != `{"name":"Net revenue","z":1}` || command.CreatedAt != createdAt {
		t.Fatalf("create command = %+v, content = %s", command, command.Content)
	}
	if command.AssetID.String() == "" || command.RevisionID.String() == "" ||
		command.AuditEventID == command.OutboxEventID {
		t.Fatalf("mutation IDs = %+v", command)
	}
}

func TestInvalidMutationFailsBeforeRepository(t *testing.T) {
	repository := &fakeRepository{}
	service := application.NewService(repository, application.ClockFunc(time.Now))
	_, err := service.CreateAsset(context.Background(), application.CreateAssetRequest{
		WorkspaceID: mustID(t, identity.NewWorkspaceID), Address: "Bad.Address", AssetType: semantic.Metric,
		SchemaVersion: "1", Content: json.RawMessage(`{}`), CreatedBy: "founder", TraceID: "bad",
	})
	if !errors.Is(err, domain.ErrInvalidArgument) || repository.createCalls != 0 {
		t.Fatalf("invalid result = %v, calls = %d", err, repository.createCalls)
	}
}

type fakeRepository struct {
	assets         []domain.AssetSummary
	lastAssetQuery domain.ListAssetsQuery
	created        domain.CreateAssetCommand
	createCalls    int
}

func (repository *fakeRepository) ListCatalogAssets(_ context.Context, query domain.ListAssetsQuery) ([]domain.AssetSummary, error) {
	repository.lastAssetQuery = query
	if query.Cursor == nil {
		return append([]domain.AssetSummary(nil), repository.assets...), nil
	}
	return nil, nil
}

func (repository *fakeRepository) GetCatalogAsset(context.Context, identity.WorkspaceID, identity.AssetID) (domain.AssetDetail, error) {
	return domain.AssetDetail{}, nil
}

func (repository *fakeRepository) CreateCatalogAsset(_ context.Context, command domain.CreateAssetCommand) (domain.AssetDetail, error) {
	repository.created = command
	repository.createCalls++
	return domain.AssetDetail{}, nil
}

func (repository *fakeRepository) ListCatalogRevisions(context.Context, domain.ListRevisionsQuery) ([]domain.Revision, error) {
	return nil, nil
}

func (repository *fakeRepository) GetCatalogRevision(context.Context, identity.WorkspaceID, identity.AssetID, identity.RevisionID) (domain.Revision, error) {
	return domain.Revision{}, nil
}

func (repository *fakeRepository) AppendCatalogRevision(context.Context, domain.AppendRevisionCommand) (domain.Revision, error) {
	return domain.Revision{}, nil
}

func (repository *fakeRepository) ListCatalogRelations(context.Context, domain.ListRelationsQuery) ([]domain.Relation, error) {
	return nil, nil
}

func (repository *fakeRepository) GetCatalogDiscoveryRun(context.Context, identity.WorkspaceID, identity.RunID) (domain.DiscoveryRun, error) {
	return domain.DiscoveryRun{}, nil
}

func mustID[T any](t *testing.T, create func() (T, error)) T {
	t.Helper()
	value, err := create()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
