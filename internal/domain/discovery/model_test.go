package discovery_test

import (
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/domain/discovery"
)

func TestCanonicalSnapshotIsOrderIndependent(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	first := discovery.Snapshot{
		AdapterKind: "catalog", AdapterVersion: "1.0.0", Locator: "catalog.json",
		ContentDigest: "sha256:content", ObservedAt: now,
		Datasets: []discovery.Dataset{
			{ExternalKey: "b", QualifiedName: "public.b", Kind: "table", Locator: "public.b"},
			{ExternalKey: "a", QualifiedName: "public.a", Kind: "table", Locator: "public.a", Fields: []discovery.Field{
				{ExternalKey: "z", Name: "z", Ordinal: 2, DataType: "text", Nullable: true},
				{ExternalKey: "id", Name: "id", Ordinal: 1, DataType: "uuid"},
			}},
		},
	}
	second := first
	second.Datasets = append([]discovery.Dataset(nil), first.Datasets...)
	second.Datasets[0], second.Datasets[1] = second.Datasets[1], second.Datasets[0]
	second.Datasets[0].Fields = append([]discovery.Field(nil), second.Datasets[0].Fields...)
	second.Datasets[0].Fields[0], second.Datasets[0].Fields[1] = second.Datasets[0].Fields[1], second.Datasets[0].Fields[0]

	if err := first.Canonicalize(); err != nil {
		t.Fatal(err)
	}
	if err := second.Canonicalize(); err != nil {
		t.Fatal(err)
	}
	if first.Datasets[0].Fingerprint != second.Datasets[0].Fingerprint ||
		first.Datasets[0].Fields[0].Fingerprint != second.Datasets[0].Fields[0].Fingerprint {
		t.Fatalf("canonical fingerprints differ: %+v %+v", first.Datasets, second.Datasets)
	}
}

func TestInputDigestIncludesFileNamesAndContent(t *testing.T) {
	input := discovery.Input{Locator: "fixture", ObservedAt: time.Now(), Files: map[string][]byte{
		"b.sql": []byte("select 2"), "a.sql": []byte("select 1"),
	}}
	reordered := discovery.Input{Locator: input.Locator, ObservedAt: input.ObservedAt, Files: map[string][]byte{
		"a.sql": []byte("select 1"), "b.sql": []byte("select 2"),
	}}
	if input.ContentDigest() != reordered.ContentDigest() {
		t.Fatal("map order changed content digest")
	}
	reordered.Files["b.sql"] = []byte("select 3")
	if input.ContentDigest() == reordered.ContentDigest() {
		t.Fatal("changed content retained digest")
	}
}
