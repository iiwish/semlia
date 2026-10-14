package sqlite_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/discovery/sqlite"
	"github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/internal/domain/ingestion"
	sqlite3 "github.com/ncruces/go-sqlite3"
)

// fixture builds a small database file with a composite key, a unique index,
// a foreign key to an implicit primary key, a view and a virtual-table-free schema.
func fixture(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	conn, err := sqlite3.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	err = conn.Exec(`
CREATE TABLE customers (id INTEGER PRIMARY KEY, email TEXT NOT NULL, region TEXT);
CREATE UNIQUE INDEX customers_email ON customers (email);
CREATE INDEX customers_region ON customers (region);
CREATE TABLE orders (id INTEGER PRIMARY KEY, customer_id bigint NOT NULL REFERENCES customers, amount numeric(12, 2) NOT NULL, paid_at timestamptz, note);
CREATE TABLE order_lines (order_id bigint NOT NULL REFERENCES orders(id), line_no integer NOT NULL, sku TEXT, PRIMARY KEY (order_id, line_no));
CREATE VIEW paid_orders AS SELECT id, amount FROM orders WHERE paid_at IS NOT NULL;
INSERT INTO customers VALUES (1, 'a@example.test', 'east');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func discover(content []byte) (discovery.Snapshot, error) {
	return (sqlite.Adapter{}).Discover(context.Background(), discovery.Input{Locator: "artifact-set:test", ObservedAt: time.Now(), Files: map[string][]byte{"shop.db": content}})
}

func TestSQLiteAdapterReadsCatalogKeysAndJoins(t *testing.T) {
	snapshot, err := discover(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]discovery.Dataset{}
	for _, dataset := range snapshot.Datasets {
		byName[dataset.QualifiedName] = dataset
	}
	if len(byName) != 4 || byName["main.paid_orders"].Kind != "view" || byName["main.orders"].Kind != "table" {
		t.Fatalf("datasets = %+v", snapshot.Datasets)
	}
	types := map[string]string{}
	nullable := map[string]bool{}
	for _, field := range byName["main.orders"].Fields {
		types[field.Name], nullable[field.Name] = field.DataType, field.Nullable
	}
	if types["amount"] != "numeric(12,2)" || types["paid_at"] != "timestamptz" || types["note"] != "blob" || nullable["id"] || !nullable["paid_at"] {
		t.Fatalf("orders fields = %+v", byName["main.orders"].Fields)
	}
	keys := map[string]string{}
	for _, key := range snapshot.Keys {
		keys[key.ConstraintName] = key.Kind
	}
	if keys["customers_pkey"] != "primary" || keys["customers_email"] != "unique" || keys["order_lines_pkey"] != "primary" || keys["orders_pkey"] != "primary" || len(keys) != 4 {
		t.Fatalf("keys = %+v", snapshot.Keys)
	}
	if len(snapshot.Joins) != 2 {
		t.Fatalf("joins = %+v", snapshot.Joins)
	}
	for _, join := range snapshot.Joins {
		if join.FromDatasetExternalKey == "sqlite:main.orders" && (join.ToFieldExternalKeys[0] != "sqlite:main.customers.id" || join.FromFieldExternalKeys[0] != "sqlite:main.orders.customer_id") {
			t.Fatalf("implicit primary key reference not resolved: %+v", join)
		}
	}
	if snapshot.AdapterKind != "sqlite_catalog" || len(snapshot.Coverage) != 1 || snapshot.Coverage[0].Status != "complete" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestSQLiteAdapterIsDeterministicAndReadOnly(t *testing.T) {
	content := fixture(t)
	original := append([]byte(nil), content...)
	first, err := discover(content)
	if err != nil {
		t.Fatal(err)
	}
	second, err := discover(content)
	if err != nil || first.ContentDigest != second.ContentDigest {
		t.Fatalf("digest changed: %v", err)
	}
	if string(content) != string(original) {
		t.Fatal("discovery modified the uploaded bytes")
	}
}

func TestSQLiteAdapterRejectsNonDatabaseAndCorruptFiles(t *testing.T) {
	if _, err := discover([]byte("id,name\n1,a\n")); !errors.Is(err, ingestion.ErrUnsafeContent) {
		t.Fatalf("csv accepted: %v", err)
	}
	content := fixture(t)
	for i := 4096; i < len(content); i++ {
		content[i] = 0xff
	}
	if _, err := discover(content); err == nil {
		t.Fatal("corrupt database accepted")
	}
}
