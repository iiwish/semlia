package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	application "github.com/iiwish/semlia/internal/application/catalog"
	usageapp "github.com/iiwish/semlia/internal/application/usage"
	domain "github.com/iiwish/semlia/internal/domain/catalog"
	"github.com/iiwish/semlia/internal/domain/semantic"
	usagedomain "github.com/iiwish/semlia/internal/domain/usage"
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
		SchemaVersion: "1.0.0", Content: json.RawMessage(`{"z":1,"threshold":9007199254740993,"name":"Net revenue"}`),
		CreatedBy: "founder", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
	})
	if err != nil {
		t.Fatal(err)
	}
	command := repository.created
	if command.Lifecycle != "draft" || command.ContentDigest == "" ||
		string(command.Content) != `{"name":"Net revenue","threshold":9007199254740993,"z":1}` || command.CreatedAt != createdAt {
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

func TestCatalogRecordsFailedSearchWithoutChangingRepositoryError(t *testing.T) {
	repository := &fakeRepository{listErr: errors.New("database unavailable")}
	usageRepository := &usageRepository{}
	clock := application.ClockFunc(func() time.Time { return time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC) })
	usageService, err := usageapp.NewService(usageRepository, usageapp.ClockFunc(clock), []byte(strings.Repeat("s", 32)))
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repository, clock, application.WithUsage(usageService))
	_, err = service.ListAssets(context.Background(), application.ListAssetsRequest{
		WorkspaceID: mustID(t, identity.NewWorkspaceID), Search: "sensitive phrase", Channel: "api",
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
	})
	if !errors.Is(err, repository.listErr) {
		t.Fatalf("catalog error = %v", err)
	}
	if len(usageRepository.events) != 1 || usageRepository.events[0].Outcome != "failed" ||
		usageRepository.events[0].ReasonCode != "CATALOG_QUERY_FAILED" {
		t.Fatalf("failed usage = %+v", usageRepository.events)
	}
}

type fakeRepository struct {
	assets         []domain.AssetSummary
	lastAssetQuery domain.ListAssetsQuery
	created        domain.CreateAssetCommand
	createCalls    int
	listErr        error
}

func (repository *fakeRepository) ListCatalogWorkspaces(context.Context) ([]domain.Workspace, error) {
	return nil, nil
}

func (repository *fakeRepository) CreateCatalogWorkspace(_ context.Context, command domain.CreateWorkspaceCommand) (domain.Workspace, error) {
	return domain.Workspace{ID: command.ID, Slug: command.Slug, DisplayName: command.DisplayName, CreatedAt: command.CreatedAt, UpdatedAt: command.CreatedAt}, nil
}

func (repository *fakeRepository) ListCatalogAssets(_ context.Context, query domain.ListAssetsQuery) ([]domain.AssetSummary, error) {
	repository.lastAssetQuery = query
	if repository.listErr != nil {
		return nil, repository.listErr
	}
	if query.Cursor == nil {
		return append([]domain.AssetSummary(nil), repository.assets...), nil
	}
	return nil, nil
}

type usageRepository struct{ events []usagedomain.Event }

func (repository *usageRepository) RecordUsageEvent(_ context.Context, event usagedomain.Event) error {
	repository.events = append(repository.events, event)
	return nil
}

func (repository *usageRepository) DeleteExpiredUsageEvents(context.Context, time.Time, int) (int64, error) {
	return 0, nil
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
