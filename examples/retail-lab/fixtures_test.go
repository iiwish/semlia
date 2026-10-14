// Package retaillab keeps the Retail Lab fixtures accepted by the real ingestion adapters.
package retaillab_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/discovery/dbt"
	"github.com/iiwish/semlia/internal/adapters/discovery/files"
	"github.com/iiwish/semlia/internal/adapters/discovery/postgresql"
	"github.com/iiwish/semlia/internal/domain/discovery"
)

func read(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func input(contents map[string][]byte) discovery.Input {
	return discovery.Input{Locator: "retail-lab", ObservedAt: time.Now(), Files: contents}
}

func TestDocumentsAndSpreadsheetsPassFileIngestion(t *testing.T) {
	cases := map[string]files.Adapter{"docs/*.md": files.Markdown(), "files/*.csv": files.CSV(), "files/*.xlsx": files.XLSX()}
	for pattern, adapter := range cases {
		paths, _ := filepath.Glob(pattern)
		if len(paths) == 0 {
			t.Fatalf("no fixtures match %s", pattern)
		}
		for _, path := range paths {
			name := filepath.Base(path)
			snapshot, err := adapter.Discover(context.Background(), input(map[string][]byte{name: read(t, path)}))
			if err != nil || len(snapshot.Datasets) == 0 {
				t.Errorf("%s rejected: %v", path, err)
				continue
			}
			if _, err := adapter.Preview(context.Background(), name, read(t, path)); err != nil {
				t.Errorf("%s preview failed: %v", path, err)
			}
		}
	}
}

func TestSQLBundleYieldsViewsAndLineageWithoutFindings(t *testing.T) {
	paths, _ := filepath.Glob("sql/mart/*.sql")
	contents := map[string][]byte{}
	for _, path := range paths {
		contents[path] = read(t, path)
	}
	snapshot, err := (postgresql.Adapter{}).Discover(context.Background(), input(contents))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Datasets) != 7 || len(snapshot.Lineage) == 0 || len(snapshot.Findings) != 0 {
		t.Fatalf("datasets=%d lineage=%d findings=%+v", len(snapshot.Datasets), len(snapshot.Lineage), snapshot.Findings)
	}
	schema, err := (postgresql.Adapter{}).Discover(context.Background(), input(map[string][]byte{"db/schema.sql": read(t, "db/schema.sql")}))
	if err != nil || len(schema.Datasets) != 14 {
		t.Fatalf("schema datasets=%d err=%v", len(schema.Datasets), err)
	}
}

func TestDBTManifestAndCatalogPassSchemaValidation(t *testing.T) {
	snapshot, err := (dbt.Adapter{}).Discover(context.Background(), input(map[string][]byte{
		"manifest.json": read(t, "dbt/manifest.json"), "catalog.json": read(t, "dbt/catalog.json")}))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Datasets) != 12 || len(snapshot.Lineage) == 0 || len(snapshot.Findings) != 0 {
		t.Fatalf("datasets=%d lineage=%d findings=%+v", len(snapshot.Datasets), len(snapshot.Lineage), snapshot.Findings)
	}
}
