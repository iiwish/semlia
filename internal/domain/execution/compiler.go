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
	"github.com/iiwish/semlia/internal/domain/semantic"
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
	excludedFields := []string{}
	fieldKey := func(asset, member string) string {
		if member != "" {
			return asset + "." + member
		}
		return asset
	}
	for _, b := range ir.Bindings {
		key := fieldKey(b.AssetID, b.MemberID)
		if b.Expression != "" || fields[key].sql != "" {
			return q, ErrInvalidPlan
		}
		f, ok := fieldSQL(b.DatasetID, b.FieldID)
		if !ok {
			return q, ErrInvalidPlan
		}
		f.aggregate = b.Aggregation
		if b.NullPolicy == "required" {
			r := relations[b.DatasetID]
			q.Guards = append(q.Guards, "SELECT NOT EXISTS (SELECT 1 FROM "+pgx.Identifier{r.Schema, r.Relation}.Sanitize()+" AS "+pgx.Identifier{aliases[b.DatasetID]}.Sanitize()+" WHERE "+f.sql+" IS NULL)")
		} else if b.NullPolicy != "" && b.NullPolicy != "exclude" {
			return q, ErrInvalidPlan
		}
		if b.NullPolicy == "exclude" && b.Aggregation == "" {
			excludedFields = append(excludedFields, f.sql+" IS NOT NULL")
		}
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
		fields[key] = f
	}
	calculations := map[string]*semantic.KnowledgeExpression{}
	for _, calc := range ir.Calculations {
		if calc.AssetID == "" || calculations[calc.AssetID] != nil || fields[calc.AssetID].sql != "" {
			return q, ErrInvalidPlan
		}
		calculations[calc.AssetID] = calc.Expression
	}
	visiting := map[string]bool{}
	var expression func(*semantic.KnowledgeExpression, int) (boundField, error)
	var metric func(string, int) (boundField, error)
	metric = func(id string, depth int) (boundField, error) {
		if depth > 32 || visiting[id] {
			return boundField{}, ErrInvalidPlan
		}
		if field, ok := fields[id]; ok {
			if field.aggregate == "" || (field.typ != "numeric" && field.typ != "bigint" && field.typ != "double precision") {
				return boundField{}, ErrInvalidPlan
			}
			return field, nil
		}
		visiting[id] = true
		field, err := expression(calculations[id], depth+1)
		delete(visiting, id)
		if err == nil {
			fields[id] = field
		}
		return field, err
	}
	expression = func(e *semantic.KnowledgeExpression, depth int) (boundField, error) {
		if e == nil || depth > 32 {
			return boundField{}, ErrInvalidPlan
		}
		if e.Op == "ref" {
			if e.Ref == nil || e.Ref.MemberID != "" || e.Left != nil || e.Right != nil {
				return boundField{}, ErrInvalidPlan
			}
			return metric(e.Ref.AssetID, depth+1)
		}
		op := map[string]string{"add": "+", "subtract": "-", "multiply": "*", "divide": "/"}[e.Op]
		if op == "" || e.Ref != nil || e.Parameter != "" || len(e.Value) > 0 {
			return boundField{}, ErrInvalidPlan
		}
		left, err := expression(e.Left, depth+1)
		if err != nil {
			return boundField{}, err
		}
		right, err := expression(e.Right, depth+1)
		if err != nil || left.dataset != right.dataset {
			return boundField{}, ErrInvalidPlan
		}
		rhs := right.sql
		if e.Op == "divide" {
			rhs = "NULLIF(" + rhs + ", 0)"
		}
		return boundField{sql: "(" + left.sql + "::numeric " + op + " " + rhs + ")", typ: "numeric", dataset: left.dataset, aggregate: "derived"}, nil
	}
	for id := range calculations {
		if _, err := metric(id, 0); err != nil {
			return q, err
		}
	}
	lookup := func(s d.Selector) (boundField, string, bool) {
		for _, a := range p.Assets {
			if (s.AssetID != nil && *s.AssetID == a.AssetID) || (s.Address != "" && strings.EqualFold(s.Address, a.Address)) {
				f, ok := fields[fieldKey(a.AssetID.String(), s.MemberID)]
				name := a.Address
				if s.MemberID != "" {
					name += "." + s.MemberID
				}
				return f, name, ok
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
	if ir.Model != nil && len(ir.Keys) != 1 {
		return q, ErrInvalidPlan
	}
	for _, key := range ir.Keys {
		if key.DatasetID != root || len(key.FieldIDs) == 0 || len(key.FieldIDs) > 16 {
			return q, ErrInvalidPlan
		}
		columns, nulls := []string{}, []string{}
		seen := map[string]bool{}
		for _, id := range key.FieldIDs {
			field, ok := fieldSQL(root, id)
			if !ok || seen[id] {
				return q, ErrInvalidPlan
			}
			seen[id] = true
			columns = append(columns, field.sql)
			nulls = append(nulls, field.sql+" IS NULL")
		}
		q.Guards = append(q.Guards, "SELECT NOT EXISTS (SELECT 1 FROM "+from+" WHERE "+strings.Join(nulls, " OR ")+")",
			"SELECT NOT EXISTS (SELECT 1 FROM "+from+" GROUP BY "+strings.Join(columns, ", ")+" HAVING COUNT(*) > 1)")
	}
	// A dataset cannot select its role by whichever approved edge is seen first.
	incoming := map[string]bool{}
	for _, join := range ir.Joins {
		if relations[join.LeftDatasetID].DatasetID == "" || relations[join.RightDatasetID].DatasetID == "" || join.RightDatasetID == root || incoming[join.RightDatasetID] {
			return q, ErrInvalidPlan
		}
		incoming[join.RightDatasetID] = true
	}
	connected := map[string]bool{root: true}
	// Every many-to-one path is guarded against actual right-side duplicates
	// in the adapter's read-only repeatable-read transaction before aggregation.
	for len(connected) < len(relations) {
		advanced := false
		for _, j := range ir.Joins {
			if !connected[j.LeftDatasetID] || connected[j.RightDatasetID] {
				continue
			}
			if j.Expression != "field_pairs_equal/v1" || (j.Cardinality != "one_to_one" && j.Cardinality != "many_to_one") || (j.JoinType != "inner" && j.JoinType != "left") || len(j.FieldPairs) == 0 || len(j.FieldPairs) > 8 || relations[j.RightDatasetID].DatasetID == "" {
				return q, ErrInvalidPlan
			}
			on := []string{}
			uniqueKeys, nonnull := []string{}, []string{}
			for _, pair := range j.FieldPairs {
				l, lok := fieldSQL(j.LeftDatasetID, pair.LeftFieldID)
				r, rok := fieldSQL(j.RightDatasetID, pair.RightFieldID)
				if !lok || !rok || l.typ != r.typ {
					return q, ErrInvalidPlan
				}
				on = append(on, l.sql+" = "+r.sql)
				uniqueKeys = append(uniqueKeys, r.sql)
				nonnull = append(nonnull, r.sql+" IS NOT NULL")
			}
			q.Guards = append(q.Guards, "SELECT NOT EXISTS (SELECT 1 FROM "+relationSQL(j.RightDatasetID)+" WHERE "+strings.Join(nonnull, " AND ")+" GROUP BY "+strings.Join(uniqueKeys, ", ")+" HAVING COUNT(*) > 1)")
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
	where, having := excludedFields, []string{}
	periodSQL := ""
	if p.TimeRange != nil && p.TimeRange.Granularity != "" {
		f, name, ok := lookup(p.TimeRange.Selector)
		if !ok || f.aggregate != "" || (f.typ != "date" && f.typ != "timestamp" && f.typ != "timestamptz") {
			return q, ErrInvalidPlan
		}
		switch p.TimeRange.Granularity {
		case "day", "week", "month", "quarter", "year":
		default:
			return q, ErrInvalidPlan
		}
		value := f.sql
		if f.typ == "timestamptz" {
			value += " AT TIME ZONE 'UTC'"
		} else if f.typ == "date" {
			value += "::timestamp"
		}
		periodSQL = "date_trunc('" + p.TimeRange.Granularity + "', " + value + ")"
		selectSQL = append(selectSQL, periodSQL+" AS "+pgx.Identifier{name + ".period"}.Sanitize())
		groupSQL = append(groupSQL, periodSQL)
		q.Columns = append(q.Columns, name+".period")
	}
	var predicate func(*semantic.KnowledgeExpression, int) (string, error)
	predicate = func(e *semantic.KnowledgeExpression, depth int) (string, error) {
		if e == nil || depth > 16 {
			return "", ErrInvalidPlan
		}
		if e.Op == "and" || e.Op == "or" {
			left, err := predicate(e.Left, depth+1)
			if err != nil {
				return "", err
			}
			right, err := predicate(e.Right, depth+1)
			if err != nil {
				return "", err
			}
			return "(" + left + " " + strings.ToUpper(e.Op) + " " + right + ")", nil
		}
		op := map[string]string{"eq": "=", "neq": "<>", "lt": "<", "lte": "<=", "gt": ">", "gte": ">="}[e.Op]
		if op == "" || e.Left == nil || e.Left.Op != "ref" || e.Left.Ref == nil || e.Right == nil {
			return "", ErrInvalidPlan
		}
		field, ok := fields[fieldKey(e.Left.Ref.AssetID, e.Left.Ref.MemberID)]
		if !ok || field.aggregate != "" {
			return "", ErrInvalidPlan
		}
		var value any
		switch e.Right.Op {
		case "literal":
			decoder := json.NewDecoder(bytes.NewReader(e.Right.Value))
			decoder.UseNumber()
			if decoder.Decode(&value) != nil {
				return "", ErrInvalidPlan
			}
		case "parameter":
			if e.Right.Parameter != "period_start" || p.TimeRange == nil {
				return "", ErrInvalidPlan
			}
			if periodSQL != "" {
				if field.typ != "date" && field.typ != "timestamp" && field.typ != "timestamptz" {
					return "", ErrInvalidPlan
				}
				period := periodSQL
				if field.typ == "timestamptz" {
					period = "(" + period + " AT TIME ZONE 'UTC')"
				}
				return "(" + field.sql + " " + op + " " + period + ")", nil
			}
			switch field.typ {
			case "date":
				value = p.TimeRange.From.Format("2006-01-02")
			case "timestamp":
				value = p.TimeRange.From.UTC().Format("2006-01-02T15:04:05.999999999")
			case "timestamptz":
				value = p.TimeRange.From.Format(time.RFC3339Nano)
			default:
				return "", ErrInvalidPlan
			}
		default:
			return "", ErrInvalidPlan
		}
		typed, ok := typedValue(value, field.typ)
		if !ok {
			return "", ErrInvalidPlan
		}
		return "(" + field.sql + " " + op + " " + bind(typed, field.typ) + ")", nil
	}
	for _, expression := range ir.Predicates {
		clause, err := predicate(expression, 0)
		if err != nil {
			return q, err
		}
		where = append(where, clause)
	}
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
		if !ok || f.aggregate != "" || (f.typ != "date" && f.typ != "timestamp" && f.typ != "timestamptz") || !p.TimeRange.From.Before(p.TimeRange.To) {
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
