package projection_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application/jobs"
	application "github.com/iiwish/semlia/internal/application/projection"
	domain "github.com/iiwish/semlia/internal/domain/projection"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestPublisherLoadsExactRevisionAndPassesOptimisticBase(t *testing.T) {
	workspace := mustNew(t, identity.NewWorkspaceID)
	asset := mustNew(t, identity.NewAssetID)
	base := mustNew(t, identity.NewRevisionID)
	revision := mustNew(t, identity.NewRevisionID)
	address, _ := semantic.NewAddress("commerce.revenue", "net_revenue")
	projection := domain.Asset{
		WorkspaceID: workspace, AssetID: asset, RevisionID: revision, Address: address,
		AssetType: semantic.Metric, LifecycleState: "active", Sequence: 2,
		SchemaVersion: "1.0.0", ContentDigest: "sha256:digest", Content: json.RawMessage(`{"name":"Net revenue"}`),
		CreatedBy: "founder", CreatedAt: time.Unix(100, 0).UTC(),
	}
	loader := &loaderStub{asset: projection}
	writer := &writerStub{}
	publisher := application.NewPublisher(loader, writer)
	payload, _ := json.Marshal(map[string]any{
		"specVersion": "semlia.catalog/v1", "action": "revision.created", "assetId": asset.String(),
		"revisionId": revision.String(), "baseRevisionId": base.String(), "sequence": 2,
	})
	err := publisher.Publish(context.Background(), jobs.OutboxEvent{
		WorkspaceID: workspace, Type: application.CatalogAssetChanged, Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if loader.assetID != asset || loader.revisionID != revision || writer.asset.ExpectedBaseRevisionID == nil ||
		*writer.asset.ExpectedBaseRevisionID != base {
		t.Fatalf("projection flow mismatch: loader=%s/%s writer=%+v", loader.assetID, loader.revisionID, writer.asset)
	}
}

func TestPublisherRejectsMalformedOrUnsupportedEventsBeforeLoading(t *testing.T) {
	workspace := mustNew(t, identity.NewWorkspaceID)
	loader := &loaderStub{}
	publisher := application.NewPublisher(loader, &writerStub{})
	tests := []jobs.OutboxEvent{
		{WorkspaceID: workspace, Type: "source.changed", Payload: json.RawMessage(`{}`)},
		{WorkspaceID: workspace, Type: application.CatalogAssetChanged, Payload: json.RawMessage(`{"specVersion":"semlia.catalog/v1","action":"created","extra":true}`)},
	}
	for _, event := range tests {
		if err := publisher.Publish(context.Background(), event); err == nil {
			t.Fatalf("event %s unexpectedly accepted", event.Type)
		}
	}
	if loader.calls != 0 {
		t.Fatalf("loader called %d times", loader.calls)
	}
}

type loaderStub struct {
	asset      domain.Asset
	assetID    identity.AssetID
	revisionID identity.RevisionID
	calls      int
}

func (loader *loaderStub) LoadAssetProjection(_ context.Context, _ identity.WorkspaceID, asset identity.AssetID, revision identity.RevisionID) (domain.Asset, error) {
	loader.calls++
	loader.assetID, loader.revisionID = asset, revision
	if loader.asset.AssetID.IsZero() {
		return domain.Asset{}, errors.New("not configured")
	}
	return loader.asset, nil
}

type writerStub struct{ asset domain.Asset }

func (writer *writerStub) ProjectAsset(_ context.Context, asset domain.Asset) (domain.Result, error) {
	writer.asset = asset
	return domain.Result{Changed: true}, nil
}

type identityFactory[T any] func() (T, error)

func mustNew[T any](t *testing.T, factory identityFactory[T]) T {
	t.Helper()
	value, err := factory()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
