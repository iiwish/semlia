package governance

import "testing"

func TestProductionAddressUsesLastNamespaceSeparator(t *testing.T) {
	for _, test := range []struct{ address, namespace, key string }{
		{"commerce.finance.revenue", "commerce.finance", "revenue"},
		{"finance.revenue", "finance", "revenue"},
		{"revenue", "default", "revenue"},
	} {
		namespace, key := parseNamespaceAndKey(test.address)
		if namespace != test.namespace || key != test.key {
			t.Fatalf("%s: got %s / %s", test.address, namespace, key)
		}
	}
}
