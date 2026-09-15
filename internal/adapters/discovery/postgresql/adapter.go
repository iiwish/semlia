package postgresql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/internal/domain/ingestion"
	pg_query "github.com/pganalyze/pg_query_go/v6"
	pgquery "github.com/wasilibs/go-pgquery"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	adapterKind    = "postgresql_sql"
	adapterVersion = "1.0.0"
	maxStatements  = 10_000
	maxSQLObjects  = 100_000
)

type Adapter struct{}

func (Adapter) Kind() string    { return adapterKind }
func (Adapter) Version() string { return adapterVersion }

func (Adapter) Discover(ctx context.Context, input discovery.Input) (discovery.Snapshot, error) {
	if err := input.Validate(); err != nil {
		return discovery.Snapshot{}, err
	}
	snapshot := discovery.Snapshot{
		AdapterKind: adapterKind, AdapterVersion: adapterVersion,
		ExternalRevision: input.ExternalRevision, Locator: input.Locator,
		ContentDigest: input.ContentDigest(), ObservedAt: input.ObservedAt,
	}
	paths := make([]string, 0, len(input.Files))
	for path := range input.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	objects := 0
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return discovery.Snapshot{}, err
		}
		content := input.Files[path]
		parsed, err := pgquery.Parse(string(content))
		if err != nil {
			return discovery.Snapshot{}, fmt.Errorf("%w: parse PostgreSQL SQL at %s", discovery.ErrInvalidInput, path)
		}
		if len(parsed.Stmts) > maxStatements {
			return discovery.Snapshot{}, ingestion.ErrLimitExceeded
		}
		snapshot.CodeArtifacts = append(snapshot.CodeArtifacts, discovery.CodeArtifact{
			Path: path, Language: "sql", ContentDigest: digest(content), Content: append([]byte(nil), content...),
		})
		for statementIndex, rawStatement := range parsed.Stmts {
			if err := ctx.Err(); err != nil {
				return discovery.Snapshot{}, err
			}
			statement := rawStatement.GetStmt()
			beforeDatasets, beforeLineage, beforeFindings := len(snapshot.Datasets), len(snapshot.Lineage), len(snapshot.Findings)
			switch {
			case statement.GetCreateStmt() != nil:
				snapshot.Datasets = append(snapshot.Datasets, tableDataset(statement.GetCreateStmt(), path))
			case statement.GetViewStmt() != nil:
				dataset, lineage, findings := viewDataset(statement.GetViewStmt(), path)
				snapshot.Datasets = append(snapshot.Datasets, dataset)
				snapshot.Lineage = append(snapshot.Lineage, lineage...)
				snapshot.Findings = append(snapshot.Findings, findings...)
			case statement.GetCreateTableAsStmt() != nil:
				dataset, lineage, findings := materializedViewDataset(statement.GetCreateTableAsStmt(), path)
				snapshot.Datasets = append(snapshot.Datasets, dataset)
				snapshot.Lineage = append(snapshot.Lineage, lineage...)
				snapshot.Findings = append(snapshot.Findings, findings...)
			default:
				snapshot.Findings = append(snapshot.Findings, discovery.Finding{
					Code: "UNSUPPORTED_SQL_STATEMENT", Severity: "warning",
					Locator: fmt.Sprintf("%s#statement-%d", path, statementIndex+1),
					Details: map[string]any{"node_type": statementType(statement)},
				})
			}
			objects += len(snapshot.Datasets) - beforeDatasets + len(snapshot.Lineage) - beforeLineage + len(snapshot.Findings) - beforeFindings
			for index := beforeFindings; index < len(snapshot.Findings); index++ {
				snapshot.Findings[index].CoverageKey = discovery.PathCoverageKey("sql", path)
			}
			for _, dataset := range snapshot.Datasets[beforeDatasets:] {
				objects += len(dataset.Fields)
			}
			if objects > maxSQLObjects {
				return discovery.Snapshot{}, ingestion.ErrLimitExceeded
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return discovery.Snapshot{}, err
	}
	for _, filePath := range paths {
		key := discovery.PathCoverageKey("sql", filePath)
		unit := discovery.CoverageUnit{Key: key, Selector: filePath, ConfigDigest: discovery.SnapshotDigest(struct{ Kind, Version string }{adapterKind, adapterVersion}), Status: "complete", EnumerationComplete: true, DiagnosticCodes: []string{}}
		for _, finding := range snapshot.Findings {
			if finding.CoverageKey == key {
				unit.Status = "partial"
				unit.EnumerationComplete = false
				unit.DiagnosticCodes = append(unit.DiagnosticCodes, finding.Code)
			}
		}
		snapshot.Coverage = append(snapshot.Coverage, unit)
		for i := range snapshot.Datasets {
			if strings.HasPrefix(snapshot.Datasets[i].Locator, filePath+"#") {
				snapshot.Datasets[i].CoverageKey = key
			}
		}
		for i := range snapshot.CodeArtifacts {
			if snapshot.CodeArtifacts[i].Path == filePath {
				snapshot.CodeArtifacts[i].CoverageKey = key
			}
		}
		for i := range snapshot.Lineage {
			if snapshot.Lineage[i].CodePath == filePath {
				snapshot.Lineage[i].CoverageKey = key
			}
		}
	}
	if err := snapshot.Canonicalize(); err != nil {
		return discovery.Snapshot{}, err
	}
	return snapshot, nil
}

func tableDataset(statement *pg_query.CreateStmt, path string) discovery.Dataset {
	qualifiedName := qualified(statement.GetRelation())
	dataset := discovery.Dataset{
		ExternalKey: externalKey(qualifiedName), QualifiedName: qualifiedName,
		Kind: "table", Locator: path + "#" + qualifiedName,
	}
	for _, element := range statement.GetTableElts() {
		column := element.GetColumnDef()
		if column == nil {
			continue
		}
		dataset.Fields = append(dataset.Fields, discovery.Field{
			ExternalKey: fieldExternalKey(qualifiedName, column.GetColname()), Name: column.GetColname(),
			Ordinal: len(dataset.Fields) + 1, DataType: typeName(column.GetTypeName()), Nullable: !notNull(column),
		})
	}
	return dataset
}

func viewDataset(statement *pg_query.ViewStmt, path string) (discovery.Dataset, []discovery.LineageEdge, []discovery.Finding) {
	qualifiedName := qualified(statement.GetView())
	dataset := discovery.Dataset{
		ExternalKey: externalKey(qualifiedName), QualifiedName: qualifiedName,
		Kind: "view", Locator: path + "#" + qualifiedName,
	}
	dataset.Fields = viewFields(qualifiedName, statement.GetAliases(), statement.GetQuery())
	findings := unresolvedViewFieldFinding(path, qualifiedName, dataset.Fields)
	return dataset, lineageForQuery(statement.GetQuery(), dataset.ExternalKey, path), findings
}

func materializedViewDataset(statement *pg_query.CreateTableAsStmt, path string) (discovery.Dataset, []discovery.LineageEdge, []discovery.Finding) {
	qualifiedName := qualified(statement.GetInto().GetRel())
	dataset := discovery.Dataset{
		ExternalKey: externalKey(qualifiedName), QualifiedName: qualifiedName,
		Kind: "materialized_view", Locator: path + "#" + qualifiedName,
		Fields: viewFields(qualifiedName, statement.GetInto().GetColNames(), statement.GetQuery()),
	}
	findings := unresolvedViewFieldFinding(path, qualifiedName, dataset.Fields)
	return dataset, lineageForQuery(statement.GetQuery(), dataset.ExternalKey, path), findings
}

func viewFields(qualifiedName string, aliases []*pg_query.Node, query *pg_query.Node) []discovery.Field {
	names := make([]string, 0)
	for _, alias := range aliases {
		if value := alias.GetString_(); value != nil && value.GetSval() != "" {
			names = append(names, value.GetSval())
		}
	}
	if len(names) == 0 && query.GetSelectStmt() != nil {
		for _, target := range query.GetSelectStmt().GetTargetList() {
			if value := target.GetResTarget(); value != nil && value.GetName() != "" {
				names = append(names, value.GetName())
			}
		}
	}
	fields := make([]discovery.Field, 0, len(names))
	for index, name := range names {
		fields = append(fields, discovery.Field{
			ExternalKey: fieldExternalKey(qualifiedName, name), Name: name,
			Ordinal: index + 1, DataType: "unknown", Nullable: true,
		})
	}
	return fields
}

func unresolvedViewFieldFinding(path, qualifiedName string, fields []discovery.Field) []discovery.Finding {
	if len(fields) > 0 {
		return nil
	}
	return []discovery.Finding{{
		Code: "VIEW_FIELDS_UNRESOLVED", Severity: "warning", Locator: path + "#" + qualifiedName,
		Details: map[string]any{"qualified_name": qualifiedName},
	}}
}

func lineageForQuery(query *pg_query.Node, downstreamKey, path string) []discovery.LineageEdge {
	ranges := collectRangeVariables(query)
	edges := make([]discovery.LineageEdge, 0, len(ranges))
	seen := make(map[string]struct{}, len(ranges))
	for _, relation := range ranges {
		upstreamKey := externalKey(qualified(relation))
		if upstreamKey == downstreamKey {
			continue
		}
		if _, exists := seen[upstreamKey]; exists {
			continue
		}
		seen[upstreamKey] = struct{}{}
		edges = append(edges, discovery.LineageEdge{
			UpstreamExternalKey: upstreamKey, DownstreamExternalKey: downstreamKey,
			Kind: "derived_from", CodePath: path, Confidence: 1,
		})
	}
	return edges
}

func collectRangeVariables(node *pg_query.Node) []*pg_query.RangeVar {
	if node == nil {
		return nil
	}
	var result []*pg_query.RangeVar
	var walk func(protoreflect.Message)
	walk = func(message protoreflect.Message) {
		if !message.IsValid() {
			return
		}
		if relation, ok := message.Interface().(*pg_query.RangeVar); ok {
			result = append(result, relation)
			return
		}
		message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
			switch {
			case field.IsList() && field.Kind() == protoreflect.MessageKind:
				list := value.List()
				for index := 0; index < list.Len(); index++ {
					walk(list.Get(index).Message())
				}
			case field.Kind() == protoreflect.MessageKind:
				walk(value.Message())
			}
			return true
		})
	}
	walk(node.ProtoReflect())
	return result
}

func qualified(relation *pg_query.RangeVar) string {
	if relation == nil || relation.GetRelname() == "" {
		return "public.unknown"
	}
	schema := relation.GetSchemaname()
	if schema == "" {
		schema = "public"
	}
	if relation.GetCatalogname() != "" {
		return relation.GetCatalogname() + "." + schema + "." + relation.GetRelname()
	}
	return schema + "." + relation.GetRelname()
}

func externalKey(qualifiedName string) string { return "postgres:" + qualifiedName }

func fieldExternalKey(qualifiedName, field string) string {
	return externalKey(qualifiedName) + ":" + field
}

func typeName(value *pg_query.TypeName) string {
	if value == nil {
		return "unknown"
	}
	parts := make([]string, 0, len(value.GetNames()))
	for _, part := range value.GetNames() {
		if name := part.GetString_(); name != nil {
			parts = append(parts, name.GetSval())
		}
	}
	result := strings.Join(parts, ".")
	if result == "" {
		result = "unknown"
	}
	for range value.GetArrayBounds() {
		result += "[]"
	}
	return result
}

func notNull(column *pg_query.ColumnDef) bool {
	if column.GetIsNotNull() {
		return true
	}
	for _, value := range column.GetConstraints() {
		if constraint := value.GetConstraint(); constraint != nil && constraint.GetContype() == pg_query.ConstrType_CONSTR_NOTNULL {
			return true
		}
	}
	return false
}

func statementType(node *pg_query.Node) string {
	if node == nil {
		return "unknown"
	}
	descriptor := node.ProtoReflect().Descriptor().Oneofs().ByName("node")
	if descriptor == nil {
		return "unknown"
	}
	field := node.ProtoReflect().WhichOneof(descriptor)
	if field == nil {
		return "unknown"
	}
	return string(field.Name())
}

func digest(value []byte) string {
	result := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(result[:])
}
