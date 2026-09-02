package dbt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/iiwish/semlia/internal/domain/discovery"
)

const (
	adapterKind       = "dbt"
	adapterVersion    = "1.0.0"
	manifestSchemaV12 = "https://schemas.getdbt.com/dbt/manifest/v12.json"
	catalogSchemaV1   = "https://schemas.getdbt.com/dbt/catalog/v1.json"
)

type Adapter struct{}

func (Adapter) Kind() string    { return adapterKind }
func (Adapter) Version() string { return adapterVersion }

type metadata struct {
	SchemaVersion string `json:"dbt_schema_version"`
}

type manifest struct {
	Metadata metadata        `json:"metadata"`
	Nodes    map[string]node `json:"nodes"`
	Sources  map[string]node `json:"sources"`
}

type node struct {
	Database         string    `json:"database"`
	Schema           string    `json:"schema"`
	Name             string    `json:"name"`
	Alias            string    `json:"alias"`
	UniqueID         string    `json:"unique_id"`
	ResourceType     string    `json:"resource_type"`
	OriginalFilePath string    `json:"original_file_path"`
	RawCode          string    `json:"raw_code"`
	DependsOn        dependsOn `json:"depends_on"`
}

type dependsOn struct {
	Nodes []string `json:"nodes"`
}

type catalog struct {
	Metadata metadata               `json:"metadata"`
	Nodes    map[string]catalogNode `json:"nodes"`
	Sources  map[string]catalogNode `json:"sources"`
}

type catalogNode struct {
	Columns map[string]catalogColumn `json:"columns"`
}

type catalogColumn struct {
	Name  string `json:"name"`
	Index int    `json:"index"`
	Type  string `json:"type"`
}

func (Adapter) Discover(_ context.Context, input discovery.Input) (discovery.Snapshot, error) {
	if err := input.Validate("manifest.json", "catalog.json"); err != nil {
		return discovery.Snapshot{}, err
	}
	var manifest manifest
	if err := json.Unmarshal(input.Files["manifest.json"], &manifest); err != nil {
		return discovery.Snapshot{}, fmt.Errorf("%w: dbt manifest JSON", discovery.ErrInvalidInput)
	}
	var catalog catalog
	if err := json.Unmarshal(input.Files["catalog.json"], &catalog); err != nil {
		return discovery.Snapshot{}, fmt.Errorf("%w: dbt catalog JSON", discovery.ErrInvalidInput)
	}
	snapshot := discovery.Snapshot{
		AdapterKind: adapterKind, AdapterVersion: adapterVersion,
		ExternalRevision: input.ExternalRevision, Locator: input.Locator,
		ContentDigest: input.ContentDigest(), ObservedAt: input.ObservedAt,
	}
	if manifest.Metadata.SchemaVersion != manifestSchemaV12 || catalog.Metadata.SchemaVersion != catalogSchemaV1 {
		snapshot.Findings = []discovery.Finding{{
			Code: "UNSUPPORTED_DBT_SCHEMA", Severity: "error", Locator: input.Locator, Terminal: true,
			Details: map[string]any{
				"manifest_schema": manifest.Metadata.SchemaVersion,
				"catalog_schema":  catalog.Metadata.SchemaVersion,
			},
		}}
		if err := snapshot.Canonicalize(); err != nil {
			return discovery.Snapshot{}, err
		}
		return snapshot, nil
	}
	if err := validatePublishedSchemas(input.Files["manifest.json"], input.Files["catalog.json"]); err != nil {
		return discovery.Snapshot{}, err
	}

	manifestNodes := mergeNodes(manifest.Nodes, manifest.Sources)
	catalogNodes := mergeCatalogNodes(catalog.Nodes, catalog.Sources)
	known := make(map[string]struct{})
	for uniqueID, manifestNode := range manifestNodes {
		if !supportedResource(manifestNode.ResourceType) {
			continue
		}
		if manifestNode.UniqueID == "" {
			manifestNode.UniqueID = uniqueID
		}
		dataset := datasetFromNode(manifestNode, catalogNodes[uniqueID])
		snapshot.Datasets = append(snapshot.Datasets, dataset)
		known[manifestNode.UniqueID] = struct{}{}
		if manifestNode.OriginalFilePath != "" && manifestNode.RawCode != "" {
			snapshot.CodeArtifacts = append(snapshot.CodeArtifacts, discovery.CodeArtifact{
				Path: manifestNode.OriginalFilePath, Language: "sql",
				ContentDigest: digest([]byte(manifestNode.RawCode)),
			})
		}
	}
	for uniqueID, manifestNode := range manifestNodes {
		if manifestNode.UniqueID == "" {
			manifestNode.UniqueID = uniqueID
		}
		if _, exists := known[manifestNode.UniqueID]; !exists {
			continue
		}
		for _, dependency := range manifestNode.DependsOn.Nodes {
			if _, exists := known[dependency]; !exists {
				snapshot.Findings = append(snapshot.Findings, discovery.Finding{
					Code: "UNRESOLVED_DBT_DEPENDENCY", Severity: "warning", Locator: manifestNode.OriginalFilePath,
					Details: map[string]any{"node": manifestNode.UniqueID, "dependency": dependency},
				})
				continue
			}
			snapshot.Lineage = append(snapshot.Lineage, discovery.LineageEdge{
				UpstreamExternalKey: dbtExternalKey(dependency), DownstreamExternalKey: dbtExternalKey(manifestNode.UniqueID),
				Kind: "derived_from", CodePath: manifestNode.OriginalFilePath, Confidence: 1,
			})
		}
	}
	if err := snapshot.Canonicalize(); err != nil {
		return discovery.Snapshot{}, err
	}
	return snapshot, nil
}

func datasetFromNode(value node, catalogValue catalogNode) discovery.Dataset {
	name := value.Alias
	if name == "" {
		name = value.Name
	}
	qualifiedParts := make([]string, 0, 3)
	for _, part := range []string{value.Database, value.Schema, name} {
		if part != "" {
			qualifiedParts = append(qualifiedParts, part)
		}
	}
	dataset := discovery.Dataset{
		ExternalKey: dbtExternalKey(value.UniqueID), QualifiedName: strings.Join(qualifiedParts, "."),
		Kind: datasetKind(value.ResourceType), Locator: value.OriginalFilePath,
	}
	columns := make([]catalogColumn, 0, len(catalogValue.Columns))
	for key, column := range catalogValue.Columns {
		if column.Name == "" {
			column.Name = key
		}
		columns = append(columns, column)
	}
	sort.Slice(columns, func(left, right int) bool {
		if columns[left].Index == columns[right].Index {
			return columns[left].Name < columns[right].Name
		}
		return columns[left].Index < columns[right].Index
	})
	for index, column := range columns {
		ordinal := column.Index
		if ordinal < 1 {
			ordinal = index + 1
		}
		dataset.Fields = append(dataset.Fields, discovery.Field{
			ExternalKey: dataset.ExternalKey + ":" + column.Name, Name: column.Name,
			Ordinal: ordinal, DataType: column.Type, Nullable: true,
		})
	}
	return dataset
}

func mergeNodes(left, right map[string]node) map[string]node {
	result := make(map[string]node, len(left)+len(right))
	for key, value := range left {
		result[key] = value
	}
	for key, value := range right {
		result[key] = value
	}
	return result
}

func mergeCatalogNodes(left, right map[string]catalogNode) map[string]catalogNode {
	result := make(map[string]catalogNode, len(left)+len(right))
	for key, value := range left {
		result[key] = value
	}
	for key, value := range right {
		result[key] = value
	}
	return result
}

func supportedResource(value string) bool {
	switch value {
	case "model", "seed", "snapshot", "source":
		return true
	default:
		return false
	}
}

func datasetKind(value string) string {
	if value == "source" {
		return "external"
	}
	return "model"
}

func dbtExternalKey(uniqueID string) string { return "dbt:" + uniqueID }

func digest(value []byte) string {
	result := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(result[:])
}
