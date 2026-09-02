package projection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/iiwish/semlia/internal/application/jobs"
	domain "github.com/iiwish/semlia/internal/domain/projection"
	"github.com/iiwish/semlia/pkg/identity"
)

const CatalogAssetChanged = "catalog.asset.changed"

type Loader interface {
	LoadAssetProjection(context.Context, identity.WorkspaceID, identity.AssetID, identity.RevisionID) (domain.Asset, error)
}

type Writer interface {
	ProjectAsset(context.Context, domain.Asset) (domain.Result, error)
}

type Publisher struct {
	loader Loader
	writer Writer
}

func NewPublisher(loader Loader, writer Writer) *Publisher {
	return &Publisher{loader: loader, writer: writer}
}

func (publisher *Publisher) Publish(ctx context.Context, event jobs.OutboxEvent) error {
	if publisher == nil || publisher.loader == nil || publisher.writer == nil {
		return domain.ErrInvalid
	}
	if event.Type != CatalogAssetChanged {
		return domain.ErrUnsupported
	}
	payload, err := decodeCatalogEvent(event)
	if err != nil {
		return err
	}
	assetID, err := identity.ParseAssetID(payload.AssetID)
	if err != nil {
		return domain.ErrInvalid
	}
	revisionID, err := identity.ParseRevisionID(payload.RevisionID)
	if err != nil {
		return domain.ErrInvalid
	}
	var baseRevisionID *identity.RevisionID
	if payload.BaseRevisionID != "" {
		value, parseErr := identity.ParseRevisionID(payload.BaseRevisionID)
		if parseErr != nil {
			return domain.ErrInvalid
		}
		baseRevisionID = &value
	}
	asset, err := publisher.loader.LoadAssetProjection(ctx, event.WorkspaceID, assetID, revisionID)
	if err != nil {
		return err
	}
	if asset.Sequence != payload.Sequence || asset.AssetID != assetID || asset.RevisionID != revisionID {
		return domain.ErrInvalid
	}
	asset.ExpectedBaseRevisionID = baseRevisionID
	_, err = publisher.writer.ProjectAsset(ctx, asset)
	return err
}

type catalogEvent struct {
	SpecVersion    string `json:"specVersion"`
	Action         string `json:"action"`
	AssetID        string `json:"assetId"`
	RevisionID     string `json:"revisionId"`
	BaseRevisionID string `json:"baseRevisionId,omitempty"`
	Sequence       int64  `json:"sequence"`
}

type eventEnvelope struct {
	SpecVersion string          `json:"specVersion"`
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Source      string          `json:"source"`
	WorkspaceID string          `json:"workspaceId"`
	Time        string          `json:"time"`
	TraceID     string          `json:"traceId"`
	Data        json.RawMessage `json:"data"`
}

func decodeCatalogEvent(event jobs.OutboxEvent) (catalogEvent, error) {
	decoder := json.NewDecoder(bytes.NewReader(event.Payload))
	decoder.DisallowUnknownFields()
	var envelope eventEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return catalogEvent{}, domain.ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return catalogEvent{}, domain.ErrInvalid
	}
	if envelope.SpecVersion != "semlia.events/v1" || envelope.ID != event.ID.String() ||
		envelope.Type != event.Type || envelope.Source != "urn:semlia:control-plane" ||
		envelope.WorkspaceID != event.WorkspaceID.String() || envelope.TraceID != event.TraceID {
		return catalogEvent{}, domain.ErrInvalid
	}
	if _, err := time.Parse(time.RFC3339Nano, envelope.Time); err != nil {
		return catalogEvent{}, domain.ErrInvalid
	}
	decoder = json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.DisallowUnknownFields()
	var payload catalogEvent
	if err := decoder.Decode(&payload); err != nil {
		return catalogEvent{}, domain.ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return catalogEvent{}, domain.ErrInvalid
	}
	if payload.SpecVersion != "semlia.catalog/v1" || payload.Sequence < 1 ||
		(payload.Action != "created" && payload.Action != "revision.created") {
		return catalogEvent{}, fmt.Errorf("%w: catalog event contract", domain.ErrInvalid)
	}
	if payload.Action == "created" && payload.BaseRevisionID != "" {
		return catalogEvent{}, domain.ErrInvalid
	}
	if payload.Action == "revision.created" && payload.BaseRevisionID == "" {
		return catalogEvent{}, domain.ErrInvalid
	}
	return payload, nil
}
