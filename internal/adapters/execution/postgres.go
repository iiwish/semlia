package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	d "github.com/iiwish/semlia/internal/domain/execution"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Deployment references are separate from discovery credentials. No DSN is
// loaded from a source catalog row or accepted by an execution request.
type SourceReference struct {
	WorkspaceID string `json:"workspaceId"`
	SourceID    string `json:"sourceId"`
	DSNEnv      string `json:"dsnEnv"`
}
type Postgres struct {
	references     map[string]string
	lookup         func(string) (string, bool)
	allowPlaintext bool
}

// WithLocalPlaintext is an explicit deployment-only opt-in for local sources.
func (p *Postgres) WithLocalPlaintext() *Postgres { p.allowPlaintext = true; return p }

func NewPostgres(config string) (*Postgres, error) {
	return NewPostgresWithLookup(config, os.LookupEnv)
}
func NewPostgresWithLookup(config string, lookup func(string) (string, bool)) (*Postgres, error) {
	refs := []SourceReference{}
	if strings.TrimSpace(config) != "" {
		dec := json.NewDecoder(strings.NewReader(config))
		dec.DisallowUnknownFields()
		if dec.Decode(&refs) != nil {
			return nil, d.ErrNotConfigured
		}
	}
	p := &Postgres{references: map[string]string{}, lookup: lookup}
	for _, ref := range refs {
		w, err := identity.ParseWorkspaceID(ref.WorkspaceID)
		if err != nil {
			return nil, d.ErrNotConfigured
		}
		src, err := identity.ParseSourceConnectionID(ref.SourceID)
		if err != nil || !regexp.MustCompile(`^SEMLIA_EXECUTION_DSN_[A-Z0-9_]+$`).MatchString(ref.DSNEnv) {
			return nil, d.ErrNotConfigured
		}
		key := w.String() + "/" + src.UUID()
		if p.references[key] != "" {
			return nil, d.ErrNotConfigured
		}
		p.references[key] = ref.DSNEnv
	}
	return p, nil
}
func (p *Postgres) Execute(parent context.Context, w identity.WorkspaceID, q d.Compiled, l d.Limits) d.Output {
	fail := func(code string) d.Output { return d.Output{ErrorCode: code} }
	if !l.Valid() {
		return fail("EXECUTION_LIMITS_INVALID")
	}
	env := p.references[w.String()+"/"+q.SourceID]
	if env == "" {
		return fail("EXECUTION_NOT_CONFIGURED")
	}
	dsn, ok := p.lookup(env)
	if !ok || dsn == "" {
		return fail("EXECUTION_NOT_CONFIGURED")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fail("EXECUTION_CREDENTIAL_INVALID")
	}
	locator := net.JoinHostPort(strings.Trim(cfg.Host, "[]"), strconv.Itoa(int(cfg.Port))) + "/" + cfg.Database
	if locator != q.SourceLocator || cfg.User == "" || strings.HasPrefix(cfg.Host, "/") || len(cfg.Fallbacks) > 0 || (cfg.TLSConfig == nil && !p.allowPlaintext) || (cfg.TLSConfig != nil && cfg.TLSConfig.InsecureSkipVerify) {
		return fail("EXECUTION_SOURCE_MISMATCH")
	}
	// A DSN must not install SQL tracing or change the compiler's search path.
	cfg.Tracer = nil
	cfg.DefaultQueryExecMode = pgx.QueryExecModeExec
	cfg.RuntimeParams = map[string]string{"application_name": "semlia-execution", "default_transaction_read_only": "on", "search_path": "pg_catalog", "statement_timeout": strconv.FormatInt(l.Timeout.Milliseconds(), 10), "lock_timeout": strconv.FormatInt(l.Timeout.Milliseconds(), 10)}
	cfg.ConnectTimeout = l.Timeout
	ctx, cancel := context.WithTimeout(parent, l.Timeout)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return fail(classify(parent, ctx, err))
	}
	defer func() {
		closeCtx, c := context.WithTimeout(context.Background(), time.Second)
		defer c()
		_ = conn.Close(closeCtx)
	}()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return fail(classify(parent, ctx, err))
	}
	defer func() {
		rollbackCtx, c := context.WithTimeout(context.Background(), time.Second)
		defer c()
		_ = tx.Rollback(rollbackCtx)
	}()
	if err := verifyReadOnlyRole(ctx, tx); err != nil {
		if ctx.Err() != nil {
			return fail(classify(parent, ctx, err))
		}
		return fail("EXECUTION_ROLE_NOT_READ_ONLY")
	}
	// Only ordinary PostgreSQL tables can enter this compiler contract.
	for _, r := range q.Relations {
		var kind string
		err := tx.QueryRow(ctx, `SELECT c.relkind::text FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2 AND pg_catalog.has_table_privilege(current_user,c.oid,'SELECT')`, r[0], r[1]).Scan(&kind)
		if err != nil || (kind != "r" && kind != "p") {
			if ctx.Err() != nil {
				return fail(classify(parent, ctx, err))
			}
			return fail("EXECUTION_RELATION_UNSAFE")
		}
	}
	rows, err := tx.Query(ctx, q.SQL, q.Args...)
	if err != nil {
		return fail(classify(parent, ctx, err))
	}
	defer rows.Close()
	output := d.Output{Columns: q.Columns, Rows: [][]any{}}
	columns, _ := json.Marshal(q.Columns)
	used := len(columns) + len(`{"columns":,"rows":[]}`)
	for rows.Next() {
		if len(output.Rows) >= l.MaxRows {
			return fail("EXECUTION_ROW_LIMIT")
		}
		for _, raw := range rows.RawValues() {
			if len(raw) > l.MaxBytes-used {
				return fail("EXECUTION_BYTE_LIMIT")
			}
		}
		values, err := rows.Values()
		if err != nil {
			return fail("EXECUTION_RESULT_INVALID")
		}
		for i, value := range values {
			if id, ok := value.([16]byte); ok && rows.FieldDescriptions()[i].DataTypeOID == pgtype.UUIDOID {
				values[i] = pgtype.UUID{Bytes: id, Valid: true}
			}
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			return fail("EXECUTION_RESULT_INVALID")
		}
		var normalized []any
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.UseNumber()
		if decoder.Decode(&normalized) != nil {
			return fail("EXECUTION_RESULT_INVALID")
		}
		// Decimal/integer cells cross JSON clients as exact strings, not doubles.
		for i, value := range normalized {
			if number, ok := value.(json.Number); ok {
				normalized[i] = number.String()
			}
		}
		encoded, err = json.Marshal(normalized)
		if err != nil {
			return fail("EXECUTION_RESULT_INVALID")
		}
		used += len(encoded)
		if len(output.Rows) > 0 {
			used++
		}
		if used > l.MaxBytes {
			return fail("EXECUTION_BYTE_LIMIT")
		}
		output.Rows = append(output.Rows, normalized)
	}
	if err := rows.Err(); err != nil {
		return fail(classify(parent, ctx, err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(classify(parent, ctx, err))
	}
	output.RowCount = len(output.Rows)
	output.ByteCount = used
	output.Digest = d.Digest(struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}{output.Columns, output.Rows})
	return output
}
func classify(parent, ctx context.Context, err error) string {
	if errors.Is(parent.Err(), context.Canceled) {
		return "EXECUTION_CANCELLED"
	}
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		return "EXECUTION_TIMEOUT"
	}
	return "EXECUTION_SOURCE_FAILED"
}
func verifyReadOnlyRole(ctx context.Context, tx pgx.Tx) error {
	var unsafe bool
	err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles r WHERE pg_catalog.pg_has_role(current_user,r.oid,'MEMBER') AND (r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_roles r WHERE pg_catalog.pg_has_role(current_user,r.oid,'MEMBER') AND (
   pg_catalog.has_database_privilege(r.oid,current_database(),'CREATE,TEMP')
   OR EXISTS(SELECT 1 FROM pg_catalog.pg_namespace n WHERE pg_catalog.has_schema_privilege(r.oid,n.oid,'CREATE'))
   OR EXISTS(SELECT 1 FROM pg_catalog.pg_class c WHERE c.relkind IN('r','p','v','m','f') AND c.oid<>'pg_catalog.pg_settings'::regclass AND
     (pg_catalog.has_table_privilege(r.oid,c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER') OR pg_catalog.has_any_column_privilege(r.oid,c.oid,'INSERT,UPDATE,REFERENCES')))
   OR EXISTS(SELECT 1 FROM pg_catalog.pg_class c WHERE CASE WHEN c.relkind='S' THEN pg_catalog.has_sequence_privilege(r.oid,c.oid,'USAGE,UPDATE') ELSE false END)))
 OR pg_catalog.has_database_privilege(current_user,current_database(),'CREATE')
 OR pg_catalog.has_database_privilege(current_user,current_database(),'TEMP')
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_namespace n WHERE pg_catalog.has_schema_privilege(current_user,n.oid,'CREATE'))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_class c WHERE c.relkind IN ('r','p','v','m','f') AND c.oid<>'pg_catalog.pg_settings'::regclass AND
   (pg_catalog.has_table_privilege(current_user,c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
    OR pg_catalog.has_any_column_privilege(current_user,c.oid,'INSERT,UPDATE,REFERENCES')))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_class c WHERE CASE WHEN c.relkind='S' THEN pg_catalog.has_sequence_privilege(current_user,c.oid,'USAGE,UPDATE') ELSE false END)
 OR current_setting('transaction_read_only')<>'on'
 OR current_setting('log_statement')<>'none'
 OR current_setting('log_min_duration_statement')<>'-1'
 OR current_setting('log_min_duration_sample')<>'-1'
 OR current_setting('log_parameter_max_length_on_error')<>'0'
 OR current_setting('log_min_error_statement')<>'panic'`).Scan(&unsafe)
	if err != nil || unsafe {
		return errors.New("EXECUTION_ROLE_NOT_READ_ONLY")
	}
	return nil
}
