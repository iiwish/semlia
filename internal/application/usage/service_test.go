package usage_test

import (
	"context"
	"strings"
	"testing"
	"time"

	application "github.com/iiwish/semlia/internal/application/usage"
	domain "github.com/iiwish/semlia/internal/domain/usage"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestSearchClassificationIsPrivacyBoundedAndWorkspaceScoped(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	repository := &repositoryStub{}
	service, err := application.NewService(repository, application.ClockFunc(func() time.Time { return now }), []byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	firstWorkspace, _ := identity.NewWorkspaceID()
	secondWorkspace, _ := identity.NewWorkspaceID()
	input := application.SearchInput{
		WorkspaceID: firstWorkspace, Query: "  Net   Revenue  ", AssetTypeFilter: "metric",
		LifecycleFilter: "active", ResultCount: 12, Channel: "api",
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
	}
	if err := service.RecordSearch(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	first := repository.events[0]
	if first.Outcome != "matched" || first.TokenBucket != "2_3" || first.ResultBucket != "11_100" ||
		len(first.SearchFingerprint) != 64 || strings.Contains(first.SearchFingerprint, "revenue") {
		t.Fatalf("classified event = %+v", first)
	}
	input.WorkspaceID, input.TraceID = secondWorkspace, "5bf92f3577b34da6a3ce929d0e0e4736"
	if err := service.RecordSearch(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if repository.events[1].SearchFingerprint == first.SearchFingerprint {
		t.Fatal("same query has the same fingerprint across workspaces")
	}
}

func TestReadAndFailedSearchUseClosedAttribution(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	repository := &repositoryStub{}
	service, _ := application.NewService(repository, application.ClockFunc(func() time.Time { return now }), []byte(strings.Repeat("s", 32)))
	workspace, _ := identity.NewWorkspaceID()
	asset, _ := identity.NewAssetID()
	revision, _ := identity.NewRevisionID()
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	if err := service.RecordAssetRead(context.Background(), application.ReadInput{
		WorkspaceID: workspace, AssetID: asset, RevisionID: revision, Channel: "agent", TraceID: traceID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordSearch(context.Background(), application.SearchInput{
		WorkspaceID: workspace, Query: "revenue", Failed: true, ReasonCode: "CATALOG_QUERY_FAILED",
		Channel: "api", TraceID: "5bf92f3577b34da6a3ce929d0e0e4736",
	}); err != nil {
		t.Fatal(err)
	}
	if repository.events[0].EventType != domain.AssetRead || repository.events[0].AssetID == nil ||
		repository.events[0].SearchFingerprint != "" {
		t.Fatalf("read event = %+v", repository.events[0])
	}
	if repository.events[1].Outcome != "failed" || repository.events[1].ReasonCode != "CATALOG_QUERY_FAILED" ||
		repository.events[1].ResultBucket != "0" {
		t.Fatalf("failed search = %+v", repository.events[1])
	}
}

type repositoryStub struct{ events []domain.Event }

func (repository *repositoryStub) RecordUsageEvent(_ context.Context, event domain.Event) error {
	repository.events = append(repository.events, event)
	return nil
}

func (repository *repositoryStub) DeleteExpiredUsageEvents(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
