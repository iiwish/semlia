package sourcekind_test

import (
	"reflect"
	"testing"

	"github.com/iiwish/semlia/internal/domain/sourcekind"
)

func TestRegistryDescribesEveryStoredSourceKind(t *testing.T) {
	for name, adapter := range map[string]string{"postgresql": "postgresql_catalog", "sqlite": "sqlite_catalog", "file": "file_catalog", "sql_bundle": "postgresql_sql", "dbt_bundle": "dbt"} {
		if sourcekind.AdapterKind(name) != adapter {
			t.Errorf("%s adapter = %q, want %q", name, sourcekind.AdapterKind(name), adapter)
		}
	}
	if !sourcekind.IsLive("postgresql") || sourcekind.IsLive("sqlite") || sourcekind.AdapterKind("mysql") != "" {
		t.Fatal("ingestion modes or unknown kinds misreported")
	}
	if !sourcekind.Accepts("sqlite", "sqlite") || sourcekind.Accepts("file", "sqlite") || !sourcekind.Accepts("dbt_bundle", "dbt_catalog") || sourcekind.Accepts("postgresql", "sql") {
		t.Fatal("artifact acceptance is wrong")
	}
	if got := sourcekind.ExecutableAdapters(); !reflect.DeepEqual(got, []string{"postgresql_catalog", "sqlite_catalog"}) {
		t.Fatalf("executable adapters = %v", got)
	}
	if sourcekind.DialectForAdapter("sqlite_catalog") != sourcekind.DialectSQLite || sourcekind.DialectForAdapter("file_catalog") != "" {
		t.Fatal("dialect mapping is wrong")
	}
}
