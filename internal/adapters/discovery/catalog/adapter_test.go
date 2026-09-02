package catalog_test

import (
	"context"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/discovery/catalog"
	"github.com/iiwish/semlia/internal/domain/discovery"
)

func TestCatalogAdapterMapsAndCanonicalizes(t *testing.T) {
	input := discovery.Input{
		Locator: "catalog://warehouse", ExternalRevision: "catalog-42", ObservedAt: time.Now(),
		Files: map[string][]byte{"catalog.json": []byte(`{
			"version":"1",
			"datasets":[
				{"external_key":"orders","qualified_name":"public.orders","kind":"table","locator":"public.orders","fields":[
					{"external_key":"orders:id","name":"id","ordinal":1,"data_type":"uuid","nullable":false}
				]}
			]
		}`)},
	}
	snapshot, err := (catalog.Adapter{}).Discover(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AdapterKind != "catalog" || len(snapshot.Datasets) != 1 || len(snapshot.Datasets[0].Fields) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot.Datasets[0].Fingerprint == "" || snapshot.Datasets[0].Fields[0].Fingerprint == "" {
		t.Fatal("catalog fingerprints are empty")
	}
}
