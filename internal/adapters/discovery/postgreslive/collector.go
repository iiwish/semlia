package postgreslive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	domain "github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/jackc/pgx/v5"
)

const LiveAdapterVersion = "1.0.0"

type LiveCollector struct {
	connectTimeout time.Duration
	queryTimeout   time.Duration
	// allowUnsafeSourceRole relaxes only the role-capability assertion. It never
	// relaxes the session read-only assertion, so discovery statements stay
	// non-mutating even when the bound role could write.
	allowUnsafeSourceRole bool
}

type Option func(*LiveCollector)

// WithUnsafeSourceRole accepts a source role that is not provably read-only.
// This is a development-only relief for sources whose operator holds no
// dedicated read-only account. It must never be reachable from production or
// from a shared acceptance environment; the configuration loader enforces that
// gate for SEMLIA_DISCOVERY_ALLOW_UNSAFE_SOURCE.
func WithUnsafeSourceRole() Option {
	return func(collector *LiveCollector) { collector.allowUnsafeSourceRole = true }
}

func NewLiveCollector(connectTimeout, queryTimeout time.Duration, options ...Option) *LiveCollector {
	if connectTimeout <= 0 {
		connectTimeout = 5 * time.Second
	}
	if queryTimeout <= 0 {
		queryTimeout = 15 * time.Second
	}
	collector := &LiveCollector{connectTimeout: connectTimeout, queryTimeout: queryTimeout}
	for _, option := range options {
		if option != nil {
			option(collector)
		}
	}
	return collector
}

func (collector *LiveCollector) Collect(
	ctx context.Context, source domain.Source, secret domain.CredentialSecret, observedAt time.Time,
) (domain.Snapshot, error) {
	if source.Host == "" || source.Database == "" || source.Username == "" || source.Port < 1 || source.Port > 65535 ||
		secret.Password == "" || (source.SSLMode != "require" && source.SSLMode != "verify-ca" &&
		source.SSLMode != "verify-full" && source.SSLMode != "disable") {
		return domain.Snapshot{}, domain.ErrInvalidInput
	}
	connectionURL := (&url.URL{
		Scheme: "postgresql", User: url.UserPassword(source.Username, secret.Password),
		Host: net.JoinHostPort(strings.Trim(source.Host, "[]"), strconv.Itoa(source.Port)), Path: "/" + source.Database,
		RawQuery: url.Values{"sslmode": []string{source.SSLMode}}.Encode(),
	}).String()
	config, err := pgx.ParseConfig(connectionURL)
	connectionURL = ""
	if err != nil {
		return domain.Snapshot{}, domain.ErrConnectionTest
	}
	config.ConnectTimeout = collector.connectTimeout
	config.RuntimeParams["application_name"] = "semlia-discovery"
	config.RuntimeParams["default_transaction_read_only"] = "on"
	config.RuntimeParams["statement_timeout"] = strconv.FormatInt(collector.queryTimeout.Milliseconds(), 10)
	config.RuntimeParams["lock_timeout"] = "2000"
	connectCtx, cancel := context.WithTimeout(ctx, collector.connectTimeout)
	defer cancel()
	connection, err := pgx.ConnectConfig(connectCtx, config)
	if err != nil {
		return domain.Snapshot{}, domain.ErrConnectionTest
	}
	defer func() { _ = connection.Close(context.Background()) }()
	queryCtx, queryCancel := context.WithTimeout(ctx, collector.queryTimeout)
	defer queryCancel()
	tx, err := connection.BeginTx(queryCtx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return domain.Snapshot{}, domain.ErrConnectionTest
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := collector.assertReadOnlyRole(queryCtx, tx); err != nil {
		return domain.Snapshot{}, err
	}
	datasets, err := collectDatasets(queryCtx, tx, source)
	if err != nil {
		return domain.Snapshot{}, domain.ErrConnectionTest
	}
	keys, joins, err := collectConstraints(queryCtx, tx)
	if err != nil {
		return domain.Snapshot{}, domain.ErrConnectionTest
	}
	if err := tx.Commit(queryCtx); err != nil {
		return domain.Snapshot{}, domain.ErrConnectionTest
	}
	encoded, _ := json.Marshal(struct {
		Datasets []domain.Dataset         `json:"datasets"`
		Keys     []domain.KeyObservation  `json:"keys"`
		Joins    []domain.JoinObservation `json:"joins"`
	}{datasets, keys, joins})
	digest := sha256.Sum256(encoded)
	snapshot := domain.Snapshot{
		AdapterKind: "postgresql_catalog", AdapterVersion: LiveAdapterVersion,
		Locator: sourceLocator(source), ContentDigest: "sha256:" + hex.EncodeToString(digest[:]),
		ObservedAt: observedAt.UTC(), Datasets: datasets, Keys: keys, Joins: joins,
	}
	snapshot.DeclareCoverage("postgresql_catalog", "readable_non_system_schemas")
	return snapshot, snapshot.Canonicalize()
}

func (collector *LiveCollector) Test(ctx context.Context, source domain.Source, secret domain.CredentialSecret) error {
	_, err := collector.Collect(ctx, source, secret, time.Now().UTC())
	return err
}

// assertReadOnlyRole rejects source roles that could mutate the source.
//
// The capability assertion (administrative attributes and CREATE privileges) is
// skipped when the collector was built WithUnsafeSourceRole. The session
// read-only assertion below always applies: the collector connects with
// default_transaction_read_only=on inside a read-only transaction, so a role
// that happens to be privileged still cannot execute a mutating statement
// through this code path.
func (collector *LiveCollector) assertReadOnlyRole(ctx context.Context, tx pgx.Tx) error {
	if !collector.allowUnsafeSourceRole {
		var elevated, writable bool
		err := tx.QueryRow(ctx, `
SELECT (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolreplication OR r.rolbypassrls),
       (has_database_privilege(current_user, current_database(), 'CREATE') OR EXISTS (
           SELECT 1 FROM pg_namespace n
           WHERE n.nspname NOT LIKE 'pg_%' AND n.nspname <> 'information_schema'
             AND has_schema_privilege(current_user, n.oid, 'CREATE')
       ))
FROM pg_roles r WHERE r.rolname = current_user`).Scan(&elevated, &writable)
		if err != nil {
			return domain.ErrConnectionTest
		}
		if elevated || writable {
			return domain.ErrUnsafeSource
		}
	}
	var readOnly string
	if err := tx.QueryRow(ctx, "SHOW transaction_read_only").Scan(&readOnly); err != nil || readOnly != "on" {
		return domain.ErrUnsafeSource
	}
	return nil
}

func collectDatasets(ctx context.Context, tx pgx.Tx, source domain.Source) ([]domain.Dataset, error) {
	rows, err := tx.Query(ctx, `
SELECT n.nspname, c.relname,
       CASE c.relkind WHEN 'r' THEN 'table' WHEN 'p' THEN 'table' WHEN 'v' THEN 'view' WHEN 'm' THEN 'materialized_view' END,
       a.attname, a.attnum, pg_catalog.format_type(a.atttypid, a.atttypmod), NOT a.attnotnull
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
WHERE c.relkind IN ('r','p','v','m') AND n.nspname NOT LIKE 'pg_%' AND n.nspname <> 'information_schema'
  AND n.nspname <> 'gp_toolkit'
  AND has_table_privilege(current_user, c.oid, 'SELECT')
ORDER BY n.nspname, c.relname, a.attnum`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var datasets []domain.Dataset
	var current *domain.Dataset
	for rows.Next() {
		var schema, table, kind, column, dataType string
		var ordinal int
		var nullable bool
		if err := rows.Scan(&schema, &table, &kind, &column, &ordinal, &dataType, &nullable); err != nil {
			return nil, err
		}
		qualified := schema + "." + table
		key := "postgres:" + qualified
		if current == nil || current.ExternalKey != key {
			datasets = append(datasets, domain.Dataset{ExternalKey: key, QualifiedName: qualified, Kind: kind, Locator: sourceLocator(source) + "/" + qualified})
			current = &datasets[len(datasets)-1]
		}
		current.Fields = append(current.Fields, domain.Field{ExternalKey: key + "." + column, Name: column, Ordinal: ordinal, DataType: dataType, Nullable: nullable})
	}
	return datasets, rows.Err()
}

func collectConstraints(ctx context.Context, tx pgx.Tx) ([]domain.KeyObservation, []domain.JoinObservation, error) {
	rows, err := tx.Query(ctx, `
SELECT con.conname, con.contype, ns.nspname || '.' || tbl.relname,
       array_agg(att.attname ORDER BY key.ord),
       COALESCE(rns.nspname || '.' || ref.relname, ''),
       COALESCE(array_agg(ratt.attname ORDER BY key.ord) FILTER (WHERE ratt.attname IS NOT NULL), '{}')
FROM pg_constraint con
JOIN pg_class tbl ON tbl.oid = con.conrelid
JOIN pg_namespace ns ON ns.oid = tbl.relnamespace
JOIN LATERAL unnest(con.conkey) WITH ORDINALITY key(attnum, ord) ON true
JOIN pg_attribute att ON att.attrelid = tbl.oid AND att.attnum = key.attnum
LEFT JOIN pg_class ref ON ref.oid = con.confrelid
LEFT JOIN pg_namespace rns ON rns.oid = ref.relnamespace
LEFT JOIN pg_attribute ratt ON ratt.attrelid = ref.oid AND ratt.attnum = con.confkey[key.ord]
WHERE con.contype IN ('p','u','f') AND ns.nspname NOT LIKE 'pg_%' AND ns.nspname <> 'information_schema'
  AND ns.nspname <> 'gp_toolkit'
  AND has_table_privilege(current_user, tbl.oid, 'SELECT')
GROUP BY con.conname, con.contype, ns.nspname, tbl.relname, rns.nspname, ref.relname
ORDER BY ns.nspname, tbl.relname, con.conname`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var keys []domain.KeyObservation
	var joins []domain.JoinObservation
	for rows.Next() {
		var name, kind, from, to string
		var fromFields, toFields []string
		if err := rows.Scan(&name, &kind, &from, &fromFields, &to, &toFields); err != nil {
			return nil, nil, err
		}
		if kind == "f" {
			joins = append(joins, domain.JoinObservation{ConstraintName: name, FromDatasetExternalKey: "postgres:" + from, FromFieldExternalKeys: qualifyFields(from, fromFields), ToDatasetExternalKey: "postgres:" + to, ToFieldExternalKeys: qualifyFields(to, toFields)})
		} else {
			keyKind := "unique"
			if kind == "p" {
				keyKind = "primary"
			}
			keys = append(keys, domain.KeyObservation{DatasetExternalKey: "postgres:" + from, ConstraintName: name, FieldExternalKeys: qualifyFields(from, fromFields), Kind: keyKind})
		}
	}
	return keys, joins, rows.Err()
}

func qualifyFields(dataset string, fields []string) []string {
	result := make([]string, len(fields))
	for index, field := range fields {
		result[index] = "postgres:" + dataset + "." + field
	}
	return result
}

func sourceLocator(source domain.Source) string {
	return "postgresql:" + net.JoinHostPort(strings.Trim(source.Host, "[]"), strconv.Itoa(source.Port)) + "/" + source.Database
}
