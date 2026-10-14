package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/iiwish/semlia/internal/adapters/sqlitefile"
	d "github.com/iiwish/semlia/internal/domain/execution"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/ncruces/go-sqlite3"
)

// SQLiteArtifact identifies the uploaded database file a source revision was discovered from.
type SQLiteArtifact struct {
	ContentDigest string
	ByteSize      int64
}

// SQLiteFiles resolves and opens the database file pinned to a source revision.
type SQLiteFiles interface {
	SQLiteRevisionArtifact(ctx context.Context, workspace identity.WorkspaceID, sourceID, sourceRevisionID string) (SQLiteArtifact, error)
	Open(ctx context.Context, workspace identity.WorkspaceID, contentDigest string) (io.ReadCloser, error)
}

// SQLite executes compiled plans against the exact uploaded file a published
// plan was discovered from. The file is opened read-only through an immutable
// VFS, so there is no credential and nothing to mutate.
type SQLite struct {
	files SQLiteFiles
	mu    sync.Mutex
	cache []cachedFile
}

type cachedFile struct {
	digest  string
	content []byte
}

const (
	maxSQLiteCacheFiles = 4
	maxSQLiteFileBytes  = 50 << 20
)

func NewSQLite(files SQLiteFiles) *SQLite { return &SQLite{files: files} }

func (s *SQLite) Execute(parent context.Context, w identity.WorkspaceID, q d.Compiled, l d.Limits) d.Output {
	fail := func(code string) d.Output { return d.Output{ErrorCode: code} }
	if !l.Valid() {
		return fail("EXECUTION_LIMITS_INVALID")
	}
	if s == nil || s.files == nil || q.Dialect != (d.SQLite{}).Name() {
		return fail("EXECUTION_NOT_CONFIGURED")
	}
	ctx, cancel := context.WithTimeout(parent, l.Timeout)
	defer cancel()
	content, code := s.load(ctx, w, q)
	if code != "" {
		return fail(classifyCode(parent, ctx, code))
	}
	conn, release, err := sqlitefile.Open(ctx, content)
	if err != nil {
		return fail("EXECUTION_SOURCE_FAILED")
	}
	defer release()
	for _, r := range q.Relations {
		if r[0] != "main" || !plainTable(conn, r[1]) {
			return fail(classifyCode(parent, ctx, "EXECUTION_RELATION_UNSAFE"))
		}
	}
	for _, guard := range q.Guards {
		valid, err := scalarBool(conn, guard)
		if err != nil || !valid {
			return fail(classifyCode(parent, ctx, "EXECUTION_GRAIN_OR_NULL_VIOLATION"))
		}
	}
	stmt, tail, err := conn.Prepare(q.SQL)
	if err != nil || strings.TrimSpace(tail) != "" || stmt == nil {
		if stmt != nil {
			_ = stmt.Close()
		}
		return fail(classifyCode(parent, ctx, "EXECUTION_SOURCE_FAILED"))
	}
	defer stmt.Close()
	if !stmt.ReadOnly() || stmt.BindCount() != len(q.Args) || stmt.ColumnCount() != len(q.Columns) {
		return fail("EXECUTION_PLAN_UNSUPPORTED")
	}
	for i, arg := range q.Args {
		if bind(stmt, i+1, arg) != nil {
			return fail("EXECUTION_PLAN_UNSUPPORTED")
		}
	}
	output := d.Output{Columns: q.Columns, Rows: [][]any{}}
	columns, _ := json.Marshal(q.Columns)
	used := len(columns) + len(`{"columns":,"rows":[]}`)
	for stmt.Step() {
		if len(output.Rows) >= l.MaxRows {
			return fail("EXECUTION_ROW_LIMIT")
		}
		row := make([]any, stmt.ColumnCount())
		for i := range row {
			value, ok := cell(stmt, i, columnType(q, i))
			if !ok {
				return fail("EXECUTION_RESULT_INVALID")
			}
			row[i] = value
		}
		encoded, err := json.Marshal(row)
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
		output.Rows = append(output.Rows, row)
	}
	if stmt.Err() != nil {
		return fail(classifyCode(parent, ctx, "EXECUTION_SOURCE_FAILED"))
	}
	output.RowCount = len(output.Rows)
	output.ByteCount = used
	output.Digest = d.Digest(struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}{output.Columns, output.Rows})
	return output
}

// load returns the verified bytes of the file pinned to the plan's source revision.
func (s *SQLite) load(ctx context.Context, w identity.WorkspaceID, q d.Compiled) ([]byte, string) {
	artifact, err := s.files.SQLiteRevisionArtifact(ctx, w, q.SourceID, q.SourceRevisionID)
	if err != nil || artifact.ByteSize < 1 || artifact.ByteSize > maxSQLiteFileBytes {
		return nil, "EXECUTION_NOT_CONFIGURED"
	}
	s.mu.Lock()
	for i, cached := range s.cache {
		if cached.digest == artifact.ContentDigest {
			s.cache = append(append([]cachedFile{cached}, s.cache[:i]...), s.cache[i+1:]...)
			s.mu.Unlock()
			return cached.content, ""
		}
	}
	s.mu.Unlock()
	reader, err := s.files.Open(ctx, w, artifact.ContentDigest)
	if err != nil {
		return nil, "EXECUTION_SOURCE_FAILED"
	}
	content, err := io.ReadAll(io.LimitReader(reader, artifact.ByteSize+1))
	_ = reader.Close()
	digest := sha256.Sum256(content)
	if err != nil || int64(len(content)) != artifact.ByteSize || "sha256:"+hex.EncodeToString(digest[:]) != artifact.ContentDigest {
		return nil, "EXECUTION_SOURCE_FAILED"
	}
	s.mu.Lock()
	s.cache = append([]cachedFile{{digest: artifact.ContentDigest, content: content}}, s.cache...)
	if len(s.cache) > maxSQLiteCacheFiles {
		s.cache = s.cache[:maxSQLiteCacheFiles]
	}
	s.mu.Unlock()
	return content, ""
}

// plainTable admits only ordinary tables: no views, virtual tables or shadow objects.
func plainTable(conn *sqlite3.Conn, name string) bool {
	stmt, _, err := conn.Prepare(`SELECT type, coalesce(sql, '') FROM sqlite_schema WHERE name = ?1`)
	if err != nil {
		return false
	}
	defer stmt.Close()
	if stmt.BindText(1, name) != nil || !stmt.Step() {
		return false
	}
	kind, sql := stmt.ColumnText(0), strings.ToUpper(strings.TrimSpace(stmt.ColumnText(1)))
	return kind == "table" && !strings.HasPrefix(name, "sqlite_") && !strings.HasPrefix(sql, "CREATE VIRTUAL TABLE")
}

func scalarBool(conn *sqlite3.Conn, sql string) (bool, error) {
	stmt, _, err := conn.Prepare(sql)
	if err != nil {
		return false, err
	}
	defer stmt.Close()
	if !stmt.ReadOnly() || !stmt.Step() {
		return false, errors.New("guard returned no row")
	}
	return stmt.ColumnType(0) == sqlite3.INTEGER && stmt.ColumnInt64(0) == 1, stmt.Err()
}

func bind(stmt *sqlite3.Stmt, index int, value any) error {
	switch v := value.(type) {
	case nil:
		return stmt.BindNull(index)
	case int64:
		return stmt.BindInt64(index, v)
	case float64:
		return stmt.BindFloat(index, v)
	case string:
		return stmt.BindText(index, v)
	}
	return errors.New("unsupported SQLite argument")
}

func columnType(q d.Compiled, i int) string {
	if i < len(q.ColumnTypes) {
		return q.ColumnTypes[i]
	}
	return ""
}

// cell renders a value the same way the PostgreSQL adapter does: numbers as
// exact strings, booleans as JSON booleans and text unchanged.
func cell(stmt *sqlite3.Stmt, i int, typ string) (any, bool) {
	switch stmt.ColumnType(i) {
	case sqlite3.NULL:
		return nil, true
	case sqlite3.INTEGER:
		if typ == "boolean" {
			return stmt.ColumnInt64(i) != 0, true
		}
		return strconv.FormatInt(stmt.ColumnInt64(i), 10), true
	case sqlite3.FLOAT:
		return decimal(stmt.ColumnFloat(i)), true
	case sqlite3.TEXT:
		return stmt.ColumnText(i), true
	}
	return nil, false
}

// decimal prints a float at 15 significant digits, the precision a double
// carries, so accumulated binary error (0.1+0.2) does not leak into answers.
func decimal(value float64) string {
	rounded, err := strconv.ParseFloat(strconv.FormatFloat(value, 'g', 15, 64), 64)
	if err != nil {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	text := strconv.FormatFloat(rounded, 'f', -1, 64)
	if text == "-0" {
		return "0"
	}
	return text
}

func classifyCode(parent, ctx context.Context, code string) string {
	if errors.Is(parent.Err(), context.Canceled) {
		return "EXECUTION_CANCELLED"
	}
	if ctx.Err() != nil {
		return "EXECUTION_TIMEOUT"
	}
	return code
}

// Router dispatches each compiled plan to the adapter of its dialect.
type Router struct {
	adapters map[string]interface {
		Execute(context.Context, identity.WorkspaceID, d.Compiled, d.Limits) d.Output
	}
}

func NewRouter(postgres *Postgres, sqlite *SQLite) *Router {
	router := &Router{adapters: map[string]interface {
		Execute(context.Context, identity.WorkspaceID, d.Compiled, d.Limits) d.Output
	}{}}
	if postgres != nil {
		router.adapters[(d.PostgreSQL{}).Name()] = postgres
	}
	if sqlite != nil {
		router.adapters[(d.SQLite{}).Name()] = sqlite
	}
	return router
}

func (r *Router) Execute(ctx context.Context, w identity.WorkspaceID, q d.Compiled, l d.Limits) d.Output {
	adapter := r.adapters[q.Dialect]
	if adapter == nil {
		return d.Output{ErrorCode: "EXECUTION_NOT_CONFIGURED"}
	}
	return adapter.Execute(ctx, w, q, l)
}
