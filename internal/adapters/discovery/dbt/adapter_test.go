package dbt_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/discovery/dbt"
	"github.com/iiwish/semlia/internal/domain/discovery"
)

func TestDBTAdapterPreservesSupportedCodeAndPathBounds(t *testing.T) {
	for _, test := range []struct {
		pathBytes, codeBytes int
		valid                bool
	}{{1024, 16, true}, {1025, 16, false}, {20, 9 << 20, true}, {20, 10 << 20, true}, {20, (10 << 20) + 1, false}} {
		t.Run(fmt.Sprintf("path-%d-code-%d", test.pathBytes, test.codeBytes), func(t *testing.T) {
			path := strings.Repeat("x", test.pathBytes-4) + ".sql"
			code := strings.Repeat(" ", test.codeBytes-8) + "select 1"
			manifest := map[string]any{
				"metadata": map[string]any{"dbt_schema_version": "https://schemas.getdbt.com/dbt/manifest/v12.json"},
				"nodes": map[string]any{"model.shop.orders": map[string]any{
					"database": "warehouse", "schema": "analytics", "name": "orders", "resource_type": "model",
					"package_name": "shop", "path": path, "original_file_path": path, "unique_id": "model.shop.orders",
					"fqn": []string{"shop", "orders"}, "alias": "orders", "checksum": map[string]string{"name": "sha256", "checksum": "abc"},
					"raw_code": code, "depends_on": map[string]any{"nodes": []string{}},
				}},
			}
			for _, key := range []string{"sources", "macros", "docs", "exposures", "metrics", "groups", "selectors", "disabled", "parent_map", "child_map", "group_map", "saved_queries", "semantic_models", "unit_tests"} {
				manifest[key] = map[string]any{}
			}
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := (dbt.Adapter{}).Discover(context.Background(), discovery.Input{Locator: "target", ObservedAt: time.Now(), Files: map[string][]byte{"manifest.json": data}})
			if !test.valid {
				if err == nil {
					t.Fatal("out-of-contract input accepted")
				}
				return
			}
			if err != nil {
				t.Fatalf("supported input rejected: %v", err)
			}
			if len(snapshot.CodeArtifacts) != 1 || snapshot.CodeArtifacts[0].Path != path || string(snapshot.CodeArtifacts[0].Content) != code || len(snapshot.Coverage) != 1 || snapshot.Coverage[0].Key != "dbt" || snapshot.Coverage[0].Selector != "manifest.json" {
				t.Fatal("supported code bytes or path not preserved")
			}
		})
	}
}

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
