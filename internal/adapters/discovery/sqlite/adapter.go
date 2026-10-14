// Package sqlite discovers the catalog of an uploaded SQLite database file:
// tables, views, columns, primary and unique keys, and foreign keys.
package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/iiwish/semlia/internal/adapters/sqlitefile"
	"github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/ncruces/go-sqlite3"
)

const (
	AdapterKind    = "sqlite_catalog"
	adapterVersion = "1.0.0"
	// Schema is the only attached database an uploaded file exposes.
	Schema      = "main"
	maxDatasets = 10_000
	maxFields   = 100_000
)

type Adapter struct{}

func (Adapter) Kind() string    { return AdapterKind }
func (Adapter) Version() string { return adapterVersion }

// CoverageKey is the coverage unit declared for one uploaded database file.
func CoverageKey(name string) string { return discovery.PathCoverageKey("sqlite", name) }

func (Adapter) Discover(ctx context.Context, input discovery.Input) (discovery.Snapshot, error) {
	if err := input.Validate(); err != nil || len(input.Files) != 1 || ctx.Err() != nil {
		return discovery.Snapshot{}, discovery.ErrInvalidInput
	}
	var name string
	var content []byte
	for name, content = range input.Files {
	}
	if int64(len(content)) > ingestion.MaxUploadBytes {
		return discovery.Snapshot{}, ingestion.ErrLimitExceeded
	}
	conn, release, err := sqlitefile.Open(ctx, content)
	if err != nil {
		return discovery.Snapshot{}, ingestion.ErrUnsafeContent
	}
	defer release()
	if err := quickCheck(conn); err != nil {
		return discovery.Snapshot{}, err
	}
	datasets, keys, joins, err := collect(ctx, conn, input.Locator)
	if err != nil {
		return discovery.Snapshot{}, err
	}
	encoded, _ := json.Marshal(struct {
		Datasets []discovery.Dataset         `json:"datasets"`
		Keys     []discovery.KeyObservation  `json:"keys"`
		Joins    []discovery.JoinObservation `json:"joins"`
	}{datasets, keys, joins})
	digest := sha256.Sum256(encoded)
	snapshot := discovery.Snapshot{AdapterKind: AdapterKind, AdapterVersion: adapterVersion, ExternalRevision: input.ExternalRevision,
		Locator: input.Locator, ContentDigest: "sha256:" + hex.EncodeToString(digest[:]), ObservedAt: input.ObservedAt.UTC(),
		Datasets: datasets, Keys: keys, Joins: joins}
	for i := range snapshot.Datasets {
		snapshot.Datasets[i].CoverageKey = CoverageKey(name)
	}
	snapshot.DeclareCoverage(CoverageKey(name), name)
	if err := snapshot.Canonicalize(); err != nil {
		return discovery.Snapshot{}, err
	}
	return snapshot, nil
}

func quickCheck(conn *sqlite3.Conn) error {
	stmt, _, err := conn.Prepare("PRAGMA quick_check")
	if err != nil {
		return ingestion.ErrUnsafeContent
	}
	defer stmt.Close()
	if !stmt.Step() || stmt.ColumnText(0) != "ok" || stmt.Step() {
		return ingestion.ErrUnsafeContent
	}
	return nil
}

func externalKey(table string) string { return "sqlite:" + Schema + "." + table }

type column struct {
	name, declared string
	notNull        bool
	pk             int
}

func collect(ctx context.Context, conn *sqlite3.Conn, locator string) ([]discovery.Dataset, []discovery.KeyObservation, []discovery.JoinObservation, error) {
	type relation struct{ name, kind string }
	relations := []relation{}
	err := query(conn, `SELECT name, type FROM sqlite_schema
WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite\_%' ESCAPE '\'
  AND NOT (type = 'table' AND upper(ltrim(sql)) LIKE 'CREATE VIRTUAL TABLE%')
ORDER BY name`, nil, func(stmt *sqlite3.Stmt) error {
		relations = append(relations, relation{stmt.ColumnText(0), stmt.ColumnText(1)})
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	if len(relations) > maxDatasets {
		return nil, nil, nil, ingestion.ErrLimitExceeded
	}
	datasets := []discovery.Dataset{}
	keys := []discovery.KeyObservation{}
	joins := []discovery.JoinObservation{}
	primary := map[string][]string{}
	fieldCount := 0
	for _, rel := range relations {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		columns := []column{}
		err := query(conn, `SELECT name, type, "notnull", pk FROM pragma_table_xinfo(?) WHERE hidden <> 1 ORDER BY cid`, []string{rel.name}, func(stmt *sqlite3.Stmt) error {
			columns = append(columns, column{name: stmt.ColumnText(0), declared: stmt.ColumnText(1), notNull: stmt.ColumnInt(2) != 0, pk: stmt.ColumnInt(3)})
			return nil
		})
		if err != nil {
			return nil, nil, nil, err
		}
		fieldCount += len(columns)
		if fieldCount > maxFields {
			return nil, nil, nil, ingestion.ErrLimitExceeded
		}
		key := externalKey(rel.name)
		qualified := Schema + "." + rel.name
		dataset := discovery.Dataset{ExternalKey: key, QualifiedName: qualified, Kind: rel.kind, Locator: locator + "/" + qualified}
		pkColumns := []column{}
		for i, c := range columns {
			// INTEGER PRIMARY KEY aliases the rowid and can never be NULL.
			rowid := c.pk == 1 && strings.EqualFold(strings.TrimSpace(c.declared), "integer")
			dataset.Fields = append(dataset.Fields, discovery.Field{ExternalKey: key + "." + c.name, Name: c.name, Ordinal: i + 1,
				DataType: dataType(c.declared), Nullable: !c.notNull && !rowid})
			if c.pk > 0 {
				pkColumns = append(pkColumns, c)
			}
		}
		datasets = append(datasets, dataset)
		if rel.kind != "table" {
			continue
		}
		if len(pkColumns) > 0 {
			sort.Slice(pkColumns, func(a, b int) bool { return pkColumns[a].pk < pkColumns[b].pk })
			names := []string{}
			for _, c := range pkColumns {
				names = append(names, c.name)
			}
			primary[rel.name] = names
			keys = append(keys, discovery.KeyObservation{DatasetExternalKey: key, ConstraintName: rel.name + "_pkey", FieldExternalKeys: qualify(key, names), Kind: "primary"})
		}
		unique, err := uniqueKeys(conn, rel.name)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, index := range unique {
			keys = append(keys, discovery.KeyObservation{DatasetExternalKey: key, ConstraintName: index.name, FieldExternalKeys: qualify(key, index.columns), Kind: "unique"})
		}
	}
	for _, rel := range relations {
		if rel.kind != "table" {
			continue
		}
		type reference struct {
			parent   string
			from, to []string
		}
		references := map[int]*reference{}
		order := []int{}
		err := query(conn, `SELECT id, "table", "from", coalesce("to", '') FROM pragma_foreign_key_list(?) ORDER BY id, seq`, []string{rel.name}, func(stmt *sqlite3.Stmt) error {
			id := stmt.ColumnInt(0)
			ref := references[id]
			if ref == nil {
				ref = &reference{parent: stmt.ColumnText(1)}
				references[id] = ref
				order = append(order, id)
			}
			ref.from = append(ref.from, stmt.ColumnText(2))
			ref.to = append(ref.to, stmt.ColumnText(3))
			return nil
		})
		if err != nil {
			return nil, nil, nil, err
		}
		for _, id := range order {
			ref := references[id]
			if ref.to[0] == "" {
				// A foreign key without columns references the parent's primary key.
				if len(primary[ref.parent]) != len(ref.from) {
					continue
				}
				ref.to = primary[ref.parent]
			}
			joins = append(joins, discovery.JoinObservation{ConstraintName: fmt.Sprintf("%s_fkey_%d", rel.name, id),
				FromDatasetExternalKey: externalKey(rel.name), FromFieldExternalKeys: qualify(externalKey(rel.name), ref.from),
				ToDatasetExternalKey: externalKey(ref.parent), ToFieldExternalKeys: qualify(externalKey(ref.parent), ref.to)})
		}
	}
	return datasets, keys, joins, nil
}

type index struct {
	name    string
	columns []string
}

func uniqueKeys(conn *sqlite3.Conn, table string) ([]index, error) {
	names := []string{}
	err := query(conn, `SELECT name FROM pragma_index_list(?) WHERE "unique" = 1 AND partial = 0 AND origin IN ('u', 'c') ORDER BY name`, []string{table}, func(stmt *sqlite3.Stmt) error {
		names = append(names, stmt.ColumnText(0))
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := []index{}
	for _, name := range names {
		item := index{name: name}
		expression := false
		err := query(conn, `SELECT cid, coalesce(name, '') FROM pragma_index_info(?) ORDER BY seqno`, []string{name}, func(stmt *sqlite3.Stmt) error {
			if stmt.ColumnInt(0) < 0 {
				expression = true
			}
			item.columns = append(item.columns, stmt.ColumnText(1))
			return nil
		})
		if err != nil {
			return nil, err
		}
		if !expression && len(item.columns) > 0 {
			result = append(result, item)
		}
	}
	return result, nil
}

// dataType keeps the declared column type, lowercased, as the field type.
// Columns without a declared type have no affinity and are reported as "blob".
func dataType(declared string) string {
	declared = strings.ToLower(strings.Join(strings.Fields(declared), " "))
	declared = strings.ReplaceAll(declared, ", ", ",")
	if declared == "" {
		return "blob"
	}
	return declared
}

func qualify(dataset string, fields []string) []string {
	result := make([]string, len(fields))
	for i, field := range fields {
		result[i] = dataset + "." + field
	}
	return result
}

func query(conn *sqlite3.Conn, sql string, args []string, row func(*sqlite3.Stmt) error) error {
	stmt, _, err := conn.Prepare(sql)
	if err != nil {
		return ingestion.ErrUnsafeContent
	}
	defer stmt.Close()
	for i, arg := range args {
		if err := stmt.BindText(i+1, arg); err != nil {
			return ingestion.ErrUnsafeContent
		}
	}
	for stmt.Step() {
		if err := row(stmt); err != nil {
			return err
		}
	}
	if stmt.Err() != nil {
		return ingestion.ErrUnsafeContent
	}
	return nil
}
