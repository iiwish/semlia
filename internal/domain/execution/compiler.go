package execution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	d "github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/jackc/pgx/v5"
)

type boundField struct{ sql, typ, dataset, aggregate string }

// Compile accepts server-owned plans only. No expression string is SQL.
func Compile(p d.ResolvedSemanticPlan) (Compiled, error) {
	if p.Execution == nil || p.Execution.CompilerVersion != CompilerVersion || len(p.Measures) == 0 || p.Intent == d.IntentDescribe || p.Limit < 0 || p.Limit > 10000 {
		return Compiled{}, ErrInvalidPlan
	}
	ir := p.Execution
	if len(ir.Relations) == 0 || len(ir.Relations) > 16 || len(ir.Bindings) > 96 {
		return Compiled{}, ErrInvalidPlan
	}
	q := Compiled{}
	relations := map[string]d.ExecutionRelation{}
	aliases := map[string]string{}
	for i, r := range ir.Relations {
		if r.DatasetID == "" || r.SourceID == "" || r.SourceRevisionID == "" || r.AdapterKind != "postgresql_catalog" || !identifier(r.Schema) || !identifier(r.Relation) || relations[r.DatasetID].DatasetID != "" {
			return q, ErrInvalidPlan
		}
		if i == 0 {
			q.SourceID = r.SourceID
			q.SourceRevisionID = r.SourceRevisionID
			q.SourceLocator = r.SourceLocator
		} else if q.SourceID != r.SourceID || q.SourceRevisionID != r.SourceRevisionID || q.SourceLocator != r.SourceLocator {
			return q, ErrInvalidPlan
		}
		relations[r.DatasetID] = r
		aliases[r.DatasetID] = fmt.Sprintf("t%d", i)
		q.Relations = append(q.Relations, [2]string{r.Schema, r.Relation})
	}
	fieldSQL := func(dataset, field string) (boundField, bool) {
		r, ok := relations[dataset]
		if !ok {
			return boundField{}, false
		}
		for _, f := range r.Fields {
			if f.FieldID == field && identifier(f.Name) {
				typ := fieldType(f.DataType)
				return boundField{sql: pgx.Identifier{aliases[dataset], f.Name}.Sanitize(), typ: typ, dataset: dataset}, typ != ""
			}
		}
		return boundField{}, false
	}
	fields := map[string]boundField{}
	for _, b := range ir.Bindings {
		if b.Expression != "" || fields[b.AssetID].sql != "" {
			return q, ErrInvalidPlan
		}
		f, ok := fieldSQL(b.DatasetID, b.FieldID)
		if !ok {
			return q, ErrInvalidPlan
		}
		f.aggregate = b.Aggregation
		switch b.Aggregation {
		case "":
		case "sum", "avg":
			if f.typ != "numeric" && f.typ != "bigint" && f.typ != "double precision" {
				return q, ErrInvalidPlan
			}
			f.sql = strings.ToUpper(b.Aggregation) + "(" + f.sql + ")"
			f.typ = "numeric"
		case "count", "count_distinct":
			if b.Aggregation == "count_distinct" {
				f.sql = "COUNT(DISTINCT " + f.sql + ")"
			} else {
				f.sql = "COUNT(" + f.sql + ")"
			}
			f.typ = "bigint"
		case "min", "max":
			if f.typ == "boolean" {
				return q, ErrInvalidPlan
			}
			f.sql = strings.ToUpper(b.Aggregation) + "(" + f.sql + ")"
		default:
			return q, ErrInvalidPlan
		}
		fields[b.AssetID] = f
	}
	lookup := func(s d.Selector) (boundField, string, bool) {
		for _, a := range p.Assets {
			if (s.AssetID != nil && *s.AssetID == a.AssetID) || (s.Address != "" && strings.EqualFold(s.Address, a.Address)) {
				f, ok := fields[a.AssetID.String()]
				return f, a.Address, ok
			}
		}
		return boundField{}, "", false
	}
	selectSQL, groupSQL := []string{}, []string{}
	selected := map[string]bool{}
	root := ""
	for _, s := range p.Measures {
		f, name, ok := lookup(s)
		if !ok || f.aggregate == "" {
			return q, ErrInvalidPlan
		}
		if root == "" {
			root = f.dataset
		} else if root != f.dataset {
			return q, ErrInvalidPlan
		}
		selectSQL = append(selectSQL, f.sql+" AS "+pgx.Identifier{name}.Sanitize())
		q.Columns = append(q.Columns, name)
		selected[f.sql] = true
	}
	for _, s := range p.Grouping {
		f, name, ok := lookup(s)
		if !ok || f.aggregate != "" {
			return q, ErrInvalidPlan
		}
		selectSQL = append(selectSQL, f.sql+" AS "+pgx.Identifier{name}.Sanitize())
		groupSQL = append(groupSQL, f.sql)
		q.Columns = append(q.Columns, name)
		selected[f.sql] = true
	}
	relationSQL := func(id string) string {
		r := relations[id]
		return pgx.Identifier{r.Schema, r.Relation}.Sanitize() + " AS " + pgx.Identifier{aliases[id]}.Sanitize()
	}
	from := relationSQL(root)
	connected := map[string]bool{root: true}
	// v1 only executes one-to-one inner/left joins; other cardinalities cannot
	// prove aggregate grain safety and remain explicitly unsupported.
	for len(connected) < len(relations) {
		advanced := false
		for _, j := range ir.Joins {
			if !connected[j.LeftDatasetID] || connected[j.RightDatasetID] {
				continue
			}
			if j.Expression != "field_pairs_equal/v1" || j.Cardinality != "one_to_one" || (j.JoinType != "inner" && j.JoinType != "left") || len(j.FieldPairs) == 0 || len(j.FieldPairs) > 8 || relations[j.RightDatasetID].DatasetID == "" {
				return q, ErrInvalidPlan
			}
			on := []string{}
			for _, pair := range j.FieldPairs {
				l, lok := fieldSQL(j.LeftDatasetID, pair.LeftFieldID)
				r, rok := fieldSQL(j.RightDatasetID, pair.RightFieldID)
				if !lok || !rok || l.typ != r.typ {
					return q, ErrInvalidPlan
				}
				on = append(on, l.sql+" = "+r.sql)
			}
			from += " " + strings.ToUpper(j.JoinType) + " JOIN " + relationSQL(j.RightDatasetID) + " ON " + strings.Join(on, " AND ")
			connected[j.RightDatasetID] = true
			advanced = true
		}
		if !advanced {
			return q, ErrInvalidPlan
		}
	}
	bind := func(v any, typ string) string {
		q.Args = append(q.Args, v)
		return fmt.Sprintf("$%d::%s", len(q.Args), typ)
	}
	where, having := []string{}, []string{}
	for _, filter := range p.Filters {
		f, _, ok := lookup(filter.Selector)
		if !ok {
			return q, ErrInvalidPlan
		}
		dec := json.NewDecoder(bytes.NewReader(filter.Value))
		dec.UseNumber()
		var v any
		if dec.Decode(&v) != nil {
			return q, ErrInvalidPlan
		}
		op := map[string]string{"eq": "=", "neq": "<>", "gt": ">", "gte": ">=", "lt": "<", "lte": "<=", "contains": "LIKE", "in": "IN", "not_in": "NOT IN"}[filter.Operator]
		if op == "" {
			return q, ErrInvalidPlan
		}
		var clause string
		if filter.Operator == "in" || filter.Operator == "not_in" {
			values, ok := v.([]any)
			if !ok || len(values) == 0 || len(values) > 100 {
				return q, ErrInvalidPlan
			}
			params := []string{}
			for _, item := range values {
				value, ok := typedValue(item, f.typ)
				if !ok {
					return q, ErrInvalidPlan
				}
				params = append(params, bind(value, f.typ))
			}
			clause = f.sql + " " + op + " (" + strings.Join(params, ", ") + ")"
		} else {
			value, ok := typedValue(v, f.typ)
			if !ok {
				return q, ErrInvalidPlan
			}
			if filter.Operator == "contains" {
				if f.typ != "text" {
					return q, ErrInvalidPlan
				}
				value = "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(value.(string)) + "%"
			}
			clause = f.sql + " " + op + " " + bind(value, f.typ)
		}
		if f.aggregate != "" {
			having = append(having, clause)
		} else {
			where = append(where, clause)
		}
	}
	if p.TimeRange != nil {
		f, _, ok := lookup(p.TimeRange.Selector)
		if !ok || f.aggregate != "" || (f.typ != "date" && f.typ != "timestamp" && f.typ != "timestamptz") || p.TimeRange.Granularity != "" || !p.TimeRange.From.Before(p.TimeRange.To) {
			return q, ErrInvalidPlan
		}
		where = append(where, f.sql+" >= "+bind(p.TimeRange.From, f.typ), f.sql+" < "+bind(p.TimeRange.To, f.typ))
	}
	q.SQL = "SELECT " + strings.Join(selectSQL, ", ") + " FROM " + from
	if len(where) > 0 {
		q.SQL += " WHERE " + strings.Join(where, " AND ")
	}
	if len(groupSQL) > 0 {
		q.SQL += " GROUP BY " + strings.Join(groupSQL, ", ")
	}
	if len(having) > 0 {
		q.SQL += " HAVING " + strings.Join(having, " AND ")
	}
	ordering := []string{}
	for _, o := range p.Order {
		f, _, ok := lookup(o.Selector)
		if !ok || !selected[f.sql] || (o.Direction != "asc" && o.Direction != "desc") {
			return q, ErrInvalidPlan
		}
		ordering = append(ordering, f.sql+" "+strings.ToUpper(o.Direction))
	}
	if len(ordering) > 0 {
		q.SQL += " ORDER BY " + strings.Join(ordering, ", ")
	}
	// Fetch one extra row to detect overflow. The adapter imposes its own cap.
	limit := p.Limit
	if limit == 0 {
		limit = 1000
	}
	q.SQL += fmt.Sprintf(" LIMIT %d", limit+1)
	return q, nil
}
func identifier(s string) bool { return s != "" && len(s) <= 63 && !strings.ContainsRune(s, 0) }
func fieldType(s string) string {
	s = strings.ToLower(s)
	if match := numericType.FindStringSubmatch(s); match != nil {
		precision, _ := strconv.Atoi(match[1])
		scale, _ := strconv.Atoi(match[2])
		if precision <= 1000 && scale <= precision {
			return "numeric"
		}
		return ""
	}
	if match := textType.FindStringSubmatch(s); match != nil {
		size, _ := strconv.Atoi(match[1])
		if size <= 10485760 {
			return "text"
		}
		return ""
	}
	switch strings.ToLower(s) {
	case "smallint", "integer", "bigint", "int2", "int4", "int8":
		return "bigint"
	case "numeric", "decimal":
		return "numeric"
	case "real", "double precision", "float4", "float8":
		return "double precision"
	case "text", "varchar", "character varying", "char", "character", "name":
		return "text"
	case "boolean", "bool":
		return "boolean"
	case "date":
		return "date"
	case "timestamp", "timestamp without time zone":
		return "timestamp"
	case "timestamptz", "timestamp with time zone":
		return "timestamptz"
	case "uuid":
		return "uuid"
	}
	return ""
}

var numericType = regexp.MustCompile(`^(?:numeric|decimal)\(([1-9][0-9]{0,3}),([0-9]{1,4})\)$`)
var textType = regexp.MustCompile(`^(?:character varying|varchar|character|char)\(([1-9][0-9]{0,7})\)$`)
var uuidValuePattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func typedValue(v any, typ string) (any, bool) {
	switch typ {
	case "bigint", "numeric", "double precision":
		n, ok := v.(json.Number)
		if !ok || len(n.String()) > 512 {
			return nil, false
		}
		if typ == "bigint" {
			_, err := strconv.ParseInt(n.String(), 10, 64)
			return n.String(), err == nil
		}
		if typ == "double precision" {
			number, err := strconv.ParseFloat(n.String(), 64)
			return n.String(), err == nil && !math.IsInf(number, 0) && !math.IsNaN(number)
		}
		if index := strings.IndexAny(n.String(), "eE"); index >= 0 {
			exponent, err := strconv.Atoi(n.String()[index+1:])
			if err != nil || exponent > 1000 || exponent < -1000 {
				return nil, false
			}
		}
		return n.String(), true
	case "boolean":
		b, ok := v.(bool)
		return b, ok
	default:
		s, ok := v.(string)
		if !ok || len(s) > 4096 || strings.ContainsRune(s, 0) {
			return nil, false
		}
		switch typ {
		case "uuid":
			return s, uuidValuePattern.MatchString(s)
		case "date":
			_, err := time.Parse("2006-01-02", s)
			return s, err == nil
		case "timestamp":
			_, err := time.Parse("2006-01-02T15:04:05.999999999", s)
			return s, err == nil
		case "timestamptz":
			_, err := time.Parse(time.RFC3339Nano, s)
			return s, err == nil
		}
		return s, true
	}
}
