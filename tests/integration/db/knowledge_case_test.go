package db_test

import (
	"context"
	"encoding/json"
	d "github.com/iiwish/semlia/internal/domain/distribution"
	e "github.com/iiwish/semlia/internal/domain/execution"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/internal/testsupport/knowledgecase"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"testing"
	"time"
)

func TestFiveKnowledgeCommerceReconciliation(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, knowledgecase.DataSQL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DROP SCHEMA knowledge_case CASCADE") })
	c := knowledgecase.New()
	compile := func() e.Compiled {
		t.Helper()
		plan, refusal, err := d.BuildPlan(c.Query, c.Snapshot, identity.SemanticQueryID{}, identity.ResolvedSemanticPlanID{}, nil, time.Now())
		if err != nil || refusal != nil {
			t.Fatalf("plan: %+v %v", refusal, err)
		}
		q, err := e.Compile(plan)
		if err != nil {
			t.Fatal(err)
		}
		return q
	}
	run := func(q e.Compiled) ([][]any, bool) {
		t.Helper()
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		for _, guard := range q.Guards {
			var ok bool
			if err := tx.QueryRow(ctx, guard).Scan(&ok); err != nil {
				t.Fatal(err)
			}
			if !ok {
				return nil, false
			}
		}
		rows, err := tx.Query(ctx, q.SQL, q.Args...)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		values := [][]any{}
		for rows.Next() {
			value, err := rows.Values()
			if err != nil {
				t.Fatal(err)
			}
			values = append(values, value)
		}
		if rows.Err() != nil {
			t.Fatal(rows.Err())
		}
		return values, true
	}
	number := func(value any) float64 {
		t.Helper()
		n, ok := value.(pgtype.Numeric)
		if !ok {
			t.Fatalf("expected numeric, got %T", value)
		}
		f, err := n.Float64Value()
		if err != nil {
			t.Fatal(err)
		}
		return f.Float64
	}
	rows, valid := run(compile())
	if !valid || len(rows) != 1 || number(rows[0][0]) != 200 {
		t.Fatalf("August 600/3: %+v valid=%v", rows, valid)
	}
	c.Query.Dimensions = []d.Selector{c.Query.Filters[1].Selector}
	rows, valid = run(compile())
	if !valid || len(rows) != 1 || number(rows[0][0]) != 200 || rows[0][1] != "East" {
		t.Fatal("regional reconciliation failed", rows)
	}
	c.Query.Dimensions = nil
	c.Query.TimeRange.From = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	c.Query.TimeRange.Granularity = "month"
	rows, valid = run(compile())
	if !valid || len(rows) != 2 {
		t.Fatal("period comparison", rows)
	}
	totals := map[float64]bool{}
	for _, row := range rows {
		totals[number(row[0])] = true
	}
	if !totals[200] || !totals[400] {
		t.Fatal("ratio must recompute per period", rows)
	}
	c.Query.TimeRange.Granularity = ""
	rows, valid = run(compile())
	if !valid || number(rows[0][0]) != 250 {
		t.Fatal("grand total must be 1000/4, not average of ratios", rows)
	}
	c.Query.Filters[1].Value = json.RawMessage(`"Missing"`)
	rows, valid = run(compile())
	if !valid || len(rows) != 1 || rows[0][0] != nil {
		t.Fatal("empty / zero denominator should be null", rows)
	}
	c.Query.Filters[1].Value = json.RawMessage(`"East"`)
	if _, err := pool.Exec(ctx, `INSERT INTO knowledge_case.orders VALUES (1,100,'2026-08-01',1,'paid')`); err != nil {
		t.Fatal(err)
	}
	if _, valid = run(compile()); valid {
		t.Fatal("duplicate base object key must refuse before changing the ratio")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM knowledge_case.orders WHERE ctid=(SELECT ctid FROM knowledge_case.orders WHERE id=1 ORDER BY ctid DESC LIMIT 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO knowledge_case.customers VALUES (1,'East','2026-06-01')`); err != nil {
		t.Fatal(err)
	}
	if _, valid = run(compile()); valid {
		t.Fatal("duplicate customer key must refuse before multiplying totals")
	}
	t.Log("Synthetic PostgreSQL: August 600/3=200; region East=200; July=400/August=200; total 1000/4=250; empty=NULL; duplicate right key refused")
}

func TestKnowledgeMemberNullPoliciesApplyToGrouping(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, knowledgecase.DataSQL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DROP SCHEMA knowledge_case CASCADE") })
	if _, err := pool.Exec(ctx, `UPDATE knowledge_case.customers SET region=NULL WHERE id=3`); err != nil {
		t.Fatal(err)
	}
	for _, policy := range []string{"required", "excluded", "unknown"} {
		t.Run(policy, func(t *testing.T) {
			c := knowledgecase.New()
			c.Query.Dimensions = []d.Selector{c.Query.Filters[1].Selector}
			c.Query.Filters = c.Query.Filters[:1]
			var content map[string]json.RawMessage
			asset := &c.Snapshot.Assets[1]
			if err := json.Unmarshal(asset.Content, &content); err != nil {
				t.Fatal(err)
			}
			var spec semantic.KnowledgeSpec
			if err := json.Unmarshal(content["spec"], &spec); err != nil {
				t.Fatal(err)
			}
			for i := range spec.Members {
				if spec.Members[i].ID == "region" {
					spec.Members[i].NullPolicy = policy
				}
			}
			content["spec"], _ = json.Marshal(spec)
			asset.Content, _ = json.Marshal(content)
			plan, refusal, err := d.BuildPlan(c.Query, c.Snapshot, identity.SemanticQueryID{}, identity.ResolvedSemanticPlanID{}, nil, time.Now())
			if err != nil || refusal != nil {
				t.Fatalf("plan: %v %+v", err, refusal)
			}
			compiled, err := e.Compile(plan)
			if err != nil {
				t.Fatal(err)
			}
			valid := true
			for _, guard := range compiled.Guards {
				var passed bool
				if err := pool.QueryRow(ctx, guard).Scan(&passed); err != nil {
					t.Fatal(err)
				}
				valid = valid && passed
			}
			if policy == "required" {
				if valid {
					t.Fatal("required grouping member null was not rejected")
				}
				return
			}
			if !valid {
				t.Fatal("permitted null rejected")
			}
			rows, err := pool.Query(ctx, compiled.SQL, compiled.Args...)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			count := 0
			for rows.Next() {
				count++
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			want := 1
			if policy == "unknown" {
				want = 2
			}
			if count != want {
				t.Fatalf("policy %s: groups=%d want=%d", policy, count, want)
			}
		})
	}
}

func TestDatePeriodUsesUTCMidnightWithTimestampPredicate(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, knowledgecase.DataSQL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DROP SCHEMA knowledge_case CASCADE") })
	if _, err := pool.Exec(ctx, `ALTER TABLE knowledge_case.orders ALTER COLUMN paid_at TYPE date USING (paid_at AT TIME ZONE 'UTC')::date;
INSERT INTO knowledge_case.customers VALUES (4,'East','2026-08-01T02:00:00Z');
INSERT INTO knowledge_case.orders VALUES (9,10000,'2026-08-10',4,'paid');`); err != nil {
		t.Fatal(err)
	}
	c := knowledgecase.New()
	c.Query.TimeRange.Granularity = "month"
	for i := range c.Snapshot.Execution.Relations {
		for j := range c.Snapshot.Execution.Relations[i].Fields {
			field := &c.Snapshot.Execution.Relations[i].Fields[j]
			if field.Name == "paid_at" {
				field.DataType = "date"
			}
		}
	}
	plan, refusal, err := d.BuildPlan(c.Query, c.Snapshot, identity.SemanticQueryID{}, identity.ResolvedSemanticPlanID{}, nil, time.Now())
	if err != nil || refusal != nil {
		t.Fatalf("plan: %v %+v", err, refusal)
	}
	compiled, err := e.Compile(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, zone := range []string{"UTC", "America/New_York", "Asia/Shanghai"} {
		t.Run(zone, func(t *testing.T) {
			tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if _, err := tx.Exec(ctx, `SELECT set_config('TimeZone',$1,true)`, zone); err != nil {
				t.Fatal(err)
			}
			var ratio pgtype.Numeric
			var period time.Time
			if err := tx.QueryRow(ctx, compiled.SQL, compiled.Args...).Scan(&ratio, &period); err != nil {
				t.Fatal(err)
			}
			value, err := ratio.Float64Value()
			if err != nil || value.Float64 != 200 {
				t.Fatalf("UTC period boundary changed in %s: %v %v", zone, value.Float64, err)
			}
		})
	}
}
