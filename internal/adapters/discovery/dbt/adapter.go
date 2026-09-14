package dbt

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/semlia/internal/domain/discovery"
	ingestiondomain "github.com/iiwish/semlia/internal/domain/ingestion"
)

const (
	adapterKind       = "dbt"
	adapterVersion    = "1.0.0"
	manifestSchemaV12 = "https://schemas.getdbt.com/dbt/manifest/v12.json"
	catalogSchemaV1   = "https://schemas.getdbt.com/dbt/catalog/v1.json"
	maxDBTNodes       = 10_000
	maxDBTJSONTokens  = 500_000
	maxDBTDepth       = 64
	maxDBTIdentity    = 512
	maxDBTText        = 1024
	maxDBTPath        = 1024
	maxDBTRawCode     = 10 << 20
)

type Adapter struct{}

type CatalogAdapter struct{}

func Catalog() CatalogAdapter { return CatalogAdapter{} }

func (Adapter) Kind() string    { return adapterKind }
func (Adapter) Version() string { return adapterVersion }

func (CatalogAdapter) Kind() string    { return "dbt_catalog_validation" }
func (CatalogAdapter) Version() string { return adapterVersion }

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

func (Adapter) Discover(ctx context.Context, input discovery.Input) (discovery.Snapshot, error) {
	if err := input.Validate("manifest.json"); err != nil || ctx.Err() != nil {
		return discovery.Snapshot{}, err
	}
	if err := preflightDBTJSON(ctx, input.Files["manifest.json"]); err != nil {
		return discovery.Snapshot{}, err
	}
	var manifest manifest
	if err := json.Unmarshal(input.Files["manifest.json"], &manifest); err != nil {
		return discovery.Snapshot{}, fmt.Errorf("%w: dbt manifest JSON", discovery.ErrInvalidInput)
	}
	var catalog catalog
	catalogJSON, hasCatalog := input.Files["catalog.json"]
	if hasCatalog {
		if err := preflightDBTJSON(ctx, catalogJSON); err != nil {
			return discovery.Snapshot{}, err
		}
		if err := json.Unmarshal(catalogJSON, &catalog); err != nil {
			return discovery.Snapshot{}, fmt.Errorf("%w: dbt catalog JSON", discovery.ErrInvalidInput)
		}
	}
	snapshot := discovery.Snapshot{
		AdapterKind: adapterKind, AdapterVersion: adapterVersion,
		ExternalRevision: input.ExternalRevision, Locator: input.Locator,
		ContentDigest: input.ContentDigest(), ObservedAt: input.ObservedAt,
	}
	if manifest.Metadata.SchemaVersion != manifestSchemaV12 || (hasCatalog && catalog.Metadata.SchemaVersion != catalogSchemaV1) {
		snapshot.Findings = []discovery.Finding{{
			Code: "UNSUPPORTED_DBT_SCHEMA", Severity: "error", Locator: input.Locator, Terminal: true,
			Details: map[string]any{
				"manifest_schema": manifest.Metadata.SchemaVersion,
				"catalog_schema":  catalog.Metadata.SchemaVersion,
			},
		}}
		selector := "manifest.json"
		if hasCatalog {
			selector = "catalog.json;manifest.json"
		}
		snapshot.DeclareCoverage("dbt", selector)
		if err := snapshot.Canonicalize(); err != nil {
			return discovery.Snapshot{}, err
		}
		return snapshot, nil
	}
	if err := validateDBTModels(manifest, catalog, hasCatalog); err != nil {
		return discovery.Snapshot{}, err
	}
	if err := validatePublishedSchemas(input.Files["manifest.json"], catalogJSON, hasCatalog); err != nil {
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
				Content:       []byte(manifestNode.RawCode),
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
	selector := "manifest.json"
	if hasCatalog {
		selector = "catalog.json;manifest.json"
	}
	snapshot.DeclareCoverage("dbt", selector)
	if err := snapshot.Canonicalize(); err != nil {
		return discovery.Snapshot{}, err
	}
	return snapshot, nil
}

func (CatalogAdapter) Discover(ctx context.Context, input discovery.Input) (discovery.Snapshot, error) {
	if err := input.Validate("catalog.json"); err != nil || ctx.Err() != nil || len(input.Files) != 1 {
		return discovery.Snapshot{}, discovery.ErrInvalidInput
	}
	content := input.Files["catalog.json"]
	if err := preflightDBTJSON(ctx, content); err != nil {
		return discovery.Snapshot{}, err
	}
	var value catalog
	if json.Unmarshal(content, &value) != nil || value.Metadata.SchemaVersion != catalogSchemaV1 {
		return discovery.Snapshot{}, discovery.ErrInvalidInput
	}
	if err := validateDBTModels(manifest{}, value, true); err != nil {
		return discovery.Snapshot{}, err
	}
	if err := validateCatalogSchema(content); err != nil {
		return discovery.Snapshot{}, err
	}
	result := discovery.Snapshot{AdapterKind: "dbt_catalog_validation", AdapterVersion: adapterVersion,
		ExternalRevision: input.ExternalRevision, Locator: input.Locator, ContentDigest: input.ContentDigest(), ObservedAt: input.ObservedAt}
	if err := result.Canonicalize(); err != nil {
		return discovery.Snapshot{}, err
	}
	return result, nil
}

func preflightDBTJSON(ctx context.Context, content []byte) error {
	if len(content) == 0 || int64(len(content)) > 50<<20 || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return discovery.ErrInvalidInput
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	depth, tokens := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return discovery.ErrInvalidInput
		}
		tokens++
		if tokens > maxDBTJSONTokens {
			return ingestiondomain.ErrLimitExceeded
		}
		switch value := token.(type) {
		case json.Delim:
			if value == '{' || value == '[' {
				depth++
				if depth > maxDBTDepth {
					return ingestiondomain.ErrLimitExceeded
				}
			} else {
				depth--
			}
		case string:
			if len(value) > maxDBTRawCode {
				return ingestiondomain.ErrLimitExceeded
			}
		}
	}
	if depth != 0 || tokens == 0 {
		return discovery.ErrInvalidInput
	}
	return nil
}

func validateDBTModels(document manifest, catalogDocument catalog, hasCatalog bool) error {
	if len(document.Nodes)+len(document.Sources) > maxDBTNodes || len(catalogDocument.Nodes)+len(catalogDocument.Sources) > maxDBTNodes {
		return ingestiondomain.ErrLimitExceeded
	}
	for key := range document.Nodes {
		if _, duplicate := document.Sources[key]; duplicate {
			return discovery.ErrInvalidInput
		}
	}
	dependencies := 0
	for key, value := range mergeNodes(document.Nodes, document.Sources) {
		if !safeDBTIdentity(key) || (value.UniqueID != "" && value.UniqueID != key) ||
			!safeDBTText(value.Database) || !safeDBTText(value.Schema) || !safeDBTText(value.Name) ||
			!safeDBTText(value.Alias) || len(value.ResourceType) > 64 || len(value.RawCode) > maxDBTRawCode ||
			(value.OriginalFilePath != "" && !safeDBTPath(value.OriginalFilePath)) || len(value.DependsOn.Nodes) > 10_000 {
			return discovery.ErrInvalidInput
		}
		dependencies += len(value.DependsOn.Nodes)
		if dependencies > 100_000 {
			return ingestiondomain.ErrLimitExceeded
		}
		for _, dependency := range value.DependsOn.Nodes {
			if !safeDBTIdentity(dependency) {
				return discovery.ErrInvalidInput
			}
		}
	}
	if !hasCatalog {
		return nil
	}
	for key := range catalogDocument.Nodes {
		if _, duplicate := catalogDocument.Sources[key]; duplicate {
			return discovery.ErrInvalidInput
		}
	}
	columns := 0
	for key, value := range mergeCatalogNodes(catalogDocument.Nodes, catalogDocument.Sources) {
		if !safeDBTIdentity(key) {
			return discovery.ErrInvalidInput
		}
		if len(value.Columns) > ingestiondomain.MaxColumns {
			return ingestiondomain.ErrLimitExceeded
		}
		columns += len(value.Columns)
		if columns > 100_000 {
			return ingestiondomain.ErrLimitExceeded
		}
		for columnKey, column := range value.Columns {
			if !safeDBTIdentity(columnKey) || !safeDBTText(column.Name) || !safeDBTText(column.Type) || column.Index < 0 || column.Index > 512 {
				return discovery.ErrInvalidInput
			}
		}
	}
	return nil
}

func safeDBTIdentity(value string) bool {
	return value != "" && len(value) <= maxDBTIdentity && strings.IndexFunc(value, unsafeDBTRune) < 0
}

func safeDBTText(value string) bool {
	return len(value) <= maxDBTText && strings.IndexFunc(value, unsafeDBTRune) < 0
}

func safeDBTPath(value string) bool {
	cleaned := path.Clean(value)
	return len(value) <= maxDBTPath && cleaned == value && !path.IsAbs(value) && cleaned != "." &&
		!strings.HasPrefix(cleaned, "../") && !strings.Contains(value, "\\") && strings.IndexFunc(value, unsafeDBTRune) < 0
}

func unsafeDBTRune(value rune) bool { return unicode.IsControl(value) || unicode.In(value, unicode.Cf) }

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
