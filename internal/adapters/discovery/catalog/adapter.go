package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/iiwish/semlia/internal/domain/discovery"
)

const (
	adapterKind    = "catalog"
	adapterVersion = "1.0.0"
)

type Adapter struct{}

func (Adapter) Kind() string    { return adapterKind }
func (Adapter) Version() string { return adapterVersion }

type document struct {
	Version  string    `json:"version"`
	Datasets []dataset `json:"datasets"`
}

type dataset struct {
	ExternalKey   string  `json:"external_key"`
	QualifiedName string  `json:"qualified_name"`
	Kind          string  `json:"kind"`
	Locator       string  `json:"locator"`
	Fields        []field `json:"fields"`
}

type field struct {
	ExternalKey string `json:"external_key"`
	Name        string `json:"name"`
	Ordinal     int    `json:"ordinal"`
	DataType    string `json:"data_type"`
	Nullable    bool   `json:"nullable"`
}

func (Adapter) Discover(_ context.Context, input discovery.Input) (discovery.Snapshot, error) {
	if err := input.Validate("catalog.json"); err != nil {
		return discovery.Snapshot{}, err
	}
	var document document
	decoder := json.NewDecoder(bytes.NewReader(input.Files["catalog.json"]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return discovery.Snapshot{}, fmt.Errorf("%w: catalog JSON", discovery.ErrInvalidInput)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return discovery.Snapshot{}, fmt.Errorf("%w: trailing catalog JSON", discovery.ErrInvalidInput)
	}
	if document.Version != "1" {
		return discovery.Snapshot{}, fmt.Errorf("%w: catalog version %q", discovery.ErrUnsupported, document.Version)
	}
	snapshot := discovery.Snapshot{
		AdapterKind: adapterKind, AdapterVersion: adapterVersion,
		ExternalRevision: input.ExternalRevision, Locator: input.Locator,
		ContentDigest: input.ContentDigest(), ObservedAt: input.ObservedAt,
		Datasets: make([]discovery.Dataset, 0, len(document.Datasets)),
	}
	for _, sourceDataset := range document.Datasets {
		dataset := discovery.Dataset{
			ExternalKey: sourceDataset.ExternalKey, QualifiedName: sourceDataset.QualifiedName,
			Kind: sourceDataset.Kind, Locator: sourceDataset.Locator,
			Fields: make([]discovery.Field, 0, len(sourceDataset.Fields)),
		}
		for _, sourceField := range sourceDataset.Fields {
			dataset.Fields = append(dataset.Fields, discovery.Field{
				ExternalKey: sourceField.ExternalKey, Name: sourceField.Name, Ordinal: sourceField.Ordinal,
				DataType: sourceField.DataType, Nullable: sourceField.Nullable,
			})
		}
		snapshot.Datasets = append(snapshot.Datasets, dataset)
	}
	if err := snapshot.Canonicalize(); err != nil {
		return discovery.Snapshot{}, err
	}
	return snapshot, nil
}
