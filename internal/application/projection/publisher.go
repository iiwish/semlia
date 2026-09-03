package projection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/iiwish/semlia/internal/application/jobs"
	domain "github.com/iiwish/semlia/internal/domain/projection"
	"github.com/iiwish/semlia/pkg/identity"
)

const CatalogAssetChanged = "catalog.asset.changed"
const ReleasePublished = "release.published"

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

type ReleaseLoader interface {
	LoadReleaseProjection(context.Context, identity.WorkspaceID, identity.ReleaseID) (domain.Release, error)
}

type ReleaseWriter interface {
	ProjectRelease(context.Context, domain.Release) (domain.Result, error)
}

// ReleasePublisher consumes the semlia.release/v1 release.published outbox
// events and drives the append-only Git release documents through the same
// writer discipline as the asset projection.
type ReleasePublisher struct {
	loader ReleaseLoader
	writer ReleaseWriter
}

func NewReleasePublisher(loader ReleaseLoader, writer ReleaseWriter) *ReleasePublisher {
	return &ReleasePublisher{loader: loader, writer: writer}
}

func (publisher *ReleasePublisher) Publish(ctx context.Context, event jobs.OutboxEvent) error {
	if publisher == nil || publisher.loader == nil || publisher.writer == nil {
		return domain.ErrInvalid
	}
	if event.Type != ReleasePublished {
		return domain.ErrUnsupported
	}
	payload, err := decodeReleaseEvent(event)
	if err != nil {
		return err
	}
	releaseID, err := identity.ParseReleaseID(payload.ReleaseID)
	if err != nil {
		return domain.ErrInvalid
	}
	release, err := publisher.loader.LoadReleaseProjection(ctx, event.WorkspaceID, releaseID)
	if err != nil {
		return err
	}
	if release.ReleaseID != releaseID || release.Sequence != payload.Sequence ||
		release.ManifestDigest != payload.ManifestDigest {
		return domain.ErrInvalid
	}
	_, err = publisher.writer.ProjectRelease(ctx, release)
	return err
}

type releaseEvent struct {
	SpecVersion           string `json:"specVersion"`
	ReleaseID             string `json:"releaseId"`
	ManifestDigest        string `json:"manifestDigest"`
	Sequence              int64  `json:"sequence"`
	Action                string `json:"action"`
	RolledBackToReleaseID string `json:"rolledBackToReleaseId,omitempty"`
}

func decodeReleaseEvent(event jobs.OutboxEvent) (releaseEvent, error) {
	envelope, err := decodeEventEnvelope(event)
	if err != nil {
		return releaseEvent{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.DisallowUnknownFields()
	var payload releaseEvent
	if err := decoder.Decode(&payload); err != nil {
		return releaseEvent{}, domain.ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return releaseEvent{}, domain.ErrInvalid
	}
	if payload.SpecVersion != "semlia.release/v1" || payload.Sequence < 1 ||
		(payload.Action != "published" && payload.Action != "rolled_back") ||
		!strings.HasPrefix(payload.ManifestDigest, "sha256:") {
		return releaseEvent{}, fmt.Errorf("%w: release event contract", domain.ErrInvalid)
	}
	if (payload.Action == "rolled_back") != (payload.RolledBackToReleaseID != "") {
		return releaseEvent{}, domain.ErrInvalid
	}
	if _, err := identity.ParseReleaseID(payload.ReleaseID); err != nil {
		return releaseEvent{}, domain.ErrInvalid
	}
	if payload.RolledBackToReleaseID != "" {
		if _, err := identity.ParseReleaseID(payload.RolledBackToReleaseID); err != nil {
			return releaseEvent{}, domain.ErrInvalid
		}
	}
	return payload, nil
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

func decodeEventEnvelope(event jobs.OutboxEvent) (eventEnvelope, error) {
	decoder := json.NewDecoder(bytes.NewReader(event.Payload))
	decoder.DisallowUnknownFields()
	var envelope eventEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return eventEnvelope{}, domain.ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return eventEnvelope{}, domain.ErrInvalid
	}
	if envelope.SpecVersion != "semlia.events/v1" || envelope.ID != event.ID.String() ||
		envelope.Type != event.Type || envelope.Source != "urn:semlia:control-plane" ||
		envelope.WorkspaceID != event.WorkspaceID.String() || envelope.TraceID != event.TraceID {
		return eventEnvelope{}, domain.ErrInvalid
	}
	if _, err := time.Parse(time.RFC3339Nano, envelope.Time); err != nil {
		return eventEnvelope{}, domain.ErrInvalid
	}
	return envelope, nil
}

func decodeCatalogEvent(event jobs.OutboxEvent) (catalogEvent, error) {
	envelope, err := decodeEventEnvelope(event)
	if err != nil {
		return catalogEvent{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
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
