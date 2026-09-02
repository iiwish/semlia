package postgresql_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/discovery/postgresql"
	"github.com/iiwish/semlia/internal/domain/discovery"
)

func TestPostgreSQLAdapterParsesTablesViewsAndLineage(t *testing.T) {
	input := discovery.Input{
		Locator: "git://warehouse/ddl", ExternalRevision: "deadbeef", ObservedAt: time.Now(),
		Files: map[string][]byte{"schema.sql": []byte(`
			CREATE TABLE "sales"."orders" (
				"id" uuid NOT NULL,
				"amount" numeric
			);
			CREATE VIEW sales.order_summary (order_id, total) AS
				SELECT orders.id, orders.amount FROM sales.orders;
			CREATE INDEX orders_amount_idx ON sales.orders (amount);
		`)},
	}
	snapshot, err := (postgresql.Adapter{}).Discover(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Datasets) != 2 || len(snapshot.Lineage) != 1 {
		t.Fatalf("datasets/lineage = %d/%d: %+v", len(snapshot.Datasets), len(snapshot.Lineage), snapshot)
	}
	if snapshot.Datasets[0].ExternalKey != "postgres:sales.order_summary" || snapshot.Datasets[1].ExternalKey != "postgres:sales.orders" {
		t.Fatalf("dataset keys = %q %q", snapshot.Datasets[0].ExternalKey, snapshot.Datasets[1].ExternalKey)
	}
	if snapshot.Datasets[1].Fields[0].Nullable {
		t.Fatal("NOT NULL column mapped as nullable")
	}
	if snapshot.Lineage[0].UpstreamExternalKey != "postgres:sales.orders" ||
		snapshot.Lineage[0].DownstreamExternalKey != "postgres:sales.order_summary" {
		t.Fatalf("lineage = %+v", snapshot.Lineage[0])
	}
	if len(snapshot.Findings) != 1 || snapshot.Findings[0].Code != "UNSUPPORTED_SQL_STATEMENT" {
		t.Fatalf("findings = %+v", snapshot.Findings)
	}
}

func TestPostgreSQLAdapterRejectsInvalidSQL(t *testing.T) {
	_, err := (postgresql.Adapter{}).Discover(context.Background(), discovery.Input{
		Locator: "broken.sql", ObservedAt: time.Now(), Files: map[string][]byte{"broken.sql": []byte("SELECT FROM")},
	})
	if !errors.Is(err, discovery.ErrInvalidInput) {
		t.Fatalf("error = %v", err)
	}
}

func BenchmarkPostgreSQLDDLAdapter(b *testing.B) {
	input := discovery.Input{
		Locator: "schema.sql", ObservedAt: time.Now(),
		Files: map[string][]byte{"schema.sql": []byte("CREATE TABLE public.orders (id uuid NOT NULL, amount numeric);")},
	}
	adapter := postgresql.Adapter{}
	for range b.N {
		if _, err := adapter.Discover(context.Background(), input); err != nil {
			b.Fatal(err)
		}
	}
}
