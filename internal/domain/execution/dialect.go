package execution

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/iiwish/semlia/internal/domain/sourcekind"
)

// Dialect renders the engine-specific parts of a compiled aggregate query.
// The compiler owns query shape, validation and guards; a dialect only decides
// how parameters, casts, time truncation and time comparisons are spelled.
type Dialect interface {
	Name() string
	// FieldType maps a catalog data type to a compiler type, or "" when unsupported.
	FieldType(declared string) string
	// Placeholder renders the index-th (1-based) bound parameter of type typ.
	Placeholder(index int, typ string) string
	// Value converts a validated plan value to the driver argument for typ.
	Value(value any, typ string) any
	// Numeric casts an aggregate to an exact or floating numeric for arithmetic.
	Numeric(expression string) string
	// Period truncates a time expression of type typ to the UTC start of granularity.
	Period(expression, typ, granularity string) string
	// Compare renders left op right for operands of compiler type typ.
	Compare(left, op, right, typ string) string
	// ComparePeriod compares a time field with a truncated period expression.
	ComparePeriod(field, typ, op, period string) string
	// Contains renders a LIKE match whose pattern escapes with a backslash.
	Contains(expression, pattern string) string
	// AdapterVersion identifies the compiler and dialect pair recorded on runs.
	AdapterVersion() string
}

// DialectFor returns the dialect that executes datasets of the given catalog adapter.
func DialectFor(adapterKind string) (Dialect, bool) {
	switch sourcekind.DialectForAdapter(adapterKind) {
	case sourcekind.DialectPostgreSQL:
		return PostgreSQL{}, true
	case sourcekind.DialectSQLite:
		return SQLite{}, true
	}
	return nil, false
}

func isTime(typ string) bool { return typ == "date" || typ == "timestamp" || typ == "timestamptz" }

// PostgreSQL renders the original postgres-aggregate/v1 SQL.
type PostgreSQL struct{}

func (PostgreSQL) Name() string                     { return sourcekind.DialectPostgreSQL }
func (PostgreSQL) FieldType(declared string) string { return fieldType(declared) }
func (PostgreSQL) Placeholder(index int, typ string) string {
	return fmt.Sprintf("$%d::%s", index, typ)
}
func (PostgreSQL) Value(value any, _ string) any    { return value }
func (PostgreSQL) Numeric(expression string) string { return expression + "::numeric" }
func (PostgreSQL) Period(expression, typ, granularity string) string {
	if typ == "timestamptz" {
		expression += " AT TIME ZONE 'UTC'"
	} else if typ == "date" {
		expression += "::timestamp"
	}
	return "date_trunc('" + granularity + "', " + expression + ")"
}
func (PostgreSQL) Compare(left, op, right, _ string) string { return left + " " + op + " " + right }
func (PostgreSQL) ComparePeriod(field, typ, op, period string) string {
	if typ == "timestamptz" {
		period = "(" + period + " AT TIME ZONE 'UTC')"
	}
	return "(" + field + " " + op + " " + period + ")"
}
func (PostgreSQL) Contains(expression, pattern string) string { return expression + " LIKE " + pattern }
func (PostgreSQL) AdapterVersion() string                     { return CompilerVersion }

// SQLite renders the same plans for uploaded SQLite databases. Times are stored
// as ISO-8601 text; comparisons go through julianday() so values with different
// UTC offsets order correctly, and truncation happens in UTC like PostgreSQL.
type SQLite struct{}

func (SQLite) Name() string { return sourcekind.DialectSQLite }

// FieldType accepts PostgreSQL-style declared types first, then SQLite affinity names.
func (SQLite) FieldType(declared string) string {
	if typ := fieldType(declared); typ != "" && typ != "uuid" {
		return typ
	}
	s := strings.ToLower(strings.TrimSpace(declared))
	switch {
	case s == "" || s == "blob":
		return ""
	case s == "datetime":
		return "timestamp"
	case strings.Contains(s, "bool"):
		return "boolean"
	case strings.Contains(s, "int"):
		return "bigint"
	case strings.Contains(s, "char") || strings.Contains(s, "clob") || strings.Contains(s, "text"):
		return "text"
	case strings.Contains(s, "real") || strings.Contains(s, "floa") || strings.Contains(s, "doub"):
		return "double precision"
	case strings.HasPrefix(s, "numeric") || strings.HasPrefix(s, "decimal"):
		return "numeric"
	}
	return ""
}

func (SQLite) Placeholder(index int, _ string) string { return fmt.Sprintf("?%d", index) }

// Value turns validated plan values into native SQLite bindings.
func (SQLite) Value(value any, typ string) any {
	if t, ok := value.(time.Time); ok {
		if typ == "date" {
			return t.UTC().Format("2006-01-02")
		}
		return t.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	switch v := value.(type) {
	case bool:
		if v {
			return int64(1)
		}
		return int64(0)
	case string:
		switch typ {
		case "bigint":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return n
			}
		case "numeric", "double precision":
			if n, err := strconv.ParseFloat(v, 64); err == nil && !math.IsInf(n, 0) {
				return n
			}
		case "timestamptz":
			if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
				return t.UTC().Format("2006-01-02T15:04:05.000Z")
			}
		}
	case json.Number:
		return v.String()
	}
	return value
}

func (SQLite) Numeric(expression string) string { return "CAST(" + expression + " AS REAL)" }

func (SQLite) Period(expression, _, granularity string) string {
	switch granularity {
	case "day":
		return "strftime('%Y-%m-%dT00:00:00Z', " + expression + ")"
	case "week":
		return "strftime('%Y-%m-%dT00:00:00Z', " + expression + ", 'weekday 0', '-6 days')"
	case "month":
		return "strftime('%Y-%m-01T00:00:00Z', " + expression + ")"
	case "quarter":
		return "printf('%s-%02d-01T00:00:00Z', strftime('%Y', " + expression + "), ((CAST(strftime('%m', " + expression + ") AS INTEGER) + 2) / 3) * 3 - 2)"
	default:
		return "strftime('%Y-01-01T00:00:00Z', " + expression + ")"
	}
}

func (SQLite) Compare(left, op, right, typ string) string {
	if isTime(typ) {
		return "julianday(" + left + ") " + op + " julianday(" + right + ")"
	}
	return left + " " + op + " " + right
}

func (SQLite) ComparePeriod(field, _, op, period string) string {
	return "(julianday(" + field + ") " + op + " julianday(" + period + "))"
}

func (SQLite) Contains(expression, pattern string) string {
	return expression + " LIKE " + pattern + ` ESCAPE '\'`
}

func (SQLite) AdapterVersion() string { return "sqlite-aggregate/v1" }
