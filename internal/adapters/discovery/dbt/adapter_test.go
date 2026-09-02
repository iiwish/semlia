package dbt_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/discovery/dbt"
	"github.com/iiwish/semlia/internal/domain/discovery"
)

func TestDBTAdapterMapsSupportedArtifacts(t *testing.T) {
	input := discovery.Input{
		Locator: "git://analytics/target", ExternalRevision: "cafebabe", ObservedAt: time.Now(),
		Files: map[string][]byte{
			"manifest.json": []byte(`{
				"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json"},
				"sources":{"source.shop.orders":{"database":"warehouse","schema":"raw","name":"orders","resource_type":"source","package_name":"shop","path":"models/sources.yml","original_file_path":"models/sources.yml","unique_id":"source.shop.orders","fqn":["shop","orders"],"source_name":"shop","source_description":"","loader":"fixture","identifier":"orders"}},
				"nodes":{"model.shop.order_summary":{"database":"warehouse","schema":"analytics","name":"order_summary","resource_type":"model","package_name":"shop","path":"order_summary.sql","original_file_path":"models/order_summary.sql","unique_id":"model.shop.order_summary","fqn":["shop","order_summary"],"alias":"order_summary","checksum":{"name":"sha256","checksum":"abc"},"raw_code":"select * from {{ ref('orders') }}","depends_on":{"nodes":["source.shop.orders","model.missing"]}}},
				"macros":{},"docs":{},"exposures":{},"metrics":{},"groups":{},"selectors":{},"disabled":{},"parent_map":{},"child_map":{},"group_map":{},"saved_queries":{},"semantic_models":{},"unit_tests":{}
			}`),
			"catalog.json": []byte(`{
				"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/catalog/v1.json"},
				"sources":{"source.shop.orders":{"metadata":{"type":"TABLE","schema":"raw","name":"orders"},"columns":{"id":{"name":"id","index":1,"type":"uuid"}},"stats":{}}},
				"nodes":{"model.shop.order_summary":{"metadata":{"type":"VIEW","schema":"analytics","name":"order_summary"},"columns":{"id":{"name":"id","index":1,"type":"uuid"}},"stats":{}}}
			}`),
		},
	}
	snapshot, err := (dbt.Adapter{}).Discover(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Datasets) != 2 || len(snapshot.Lineage) != 1 || len(snapshot.Findings) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot.Lineage[0].UpstreamExternalKey != "dbt:source.shop.orders" {
		t.Fatalf("lineage = %+v", snapshot.Lineage[0])
	}
	if snapshot.Findings[0].Code != "UNRESOLVED_DBT_DEPENDENCY" {
		t.Fatalf("finding = %+v", snapshot.Findings[0])
	}
}

func TestDBTAdapterStopsOnUnknownSchema(t *testing.T) {
	snapshot, err := (dbt.Adapter{}).Discover(context.Background(), discovery.Input{
		Locator: "target", ObservedAt: time.Now(), Files: map[string][]byte{
			"manifest.json": []byte(`{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v99.json"}}`),
			"catalog.json":  []byte(`{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/catalog/v1.json"}}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Datasets) != 0 || len(snapshot.Findings) != 1 || !snapshot.Findings[0].Terminal {
		t.Fatalf("unsupported snapshot = %+v", snapshot)
	}
}

func TestDBTAdapterRejectsMalformedSupportedArtifacts(t *testing.T) {
	_, err := (dbt.Adapter{}).Discover(context.Background(), discovery.Input{
		Locator: "target", ObservedAt: time.Now(), Files: map[string][]byte{
			"manifest.json": []byte(`{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/manifest/v12.json"},"nodes":{},"sources":{}}`),
			"catalog.json":  []byte(`{"metadata":{"dbt_schema_version":"https://schemas.getdbt.com/dbt/catalog/v1.json"},"nodes":{},"sources":{}}`),
		},
	})
	if !errors.Is(err, discovery.ErrInvalidInput) {
		t.Fatalf("schema validation error = %v", err)
	}
}
