package discovery

import (
	"errors"
	"strings"
	"testing"
	"time"

	domain "github.com/iiwish/semlia/internal/domain/discovery"
)

func TestSnapshotCursorAuthenticatesEveryBoundary(t *testing.T) {
	service := &ControlService{snapshotCursorKey: newSnapshotCursorKey()}
	expected := SnapshotCursor{Version: 1, Kind: "members", Workspace: "wsp", Source: "src", Snapshot: "ssnp", Filter: "field", UpperKey: "field:upper", LastKey: "field:last", UpperTime: time.Now().UTC()}
	token := *service.encodeSnapshotCursor(expected)
	if _, err := service.decodeSnapshotCursor(token, expected); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*SnapshotCursor){func(c *SnapshotCursor) { c.Kind = "diagnostics" }, func(c *SnapshotCursor) { c.Workspace = "other" }, func(c *SnapshotCursor) { c.Source = "other" }, func(c *SnapshotCursor) { c.Snapshot = "other" }, func(c *SnapshotCursor) { c.Filter = "dataset" }} {
		other := expected
		mutate(&other)
		if _, err := service.decodeSnapshotCursor(token, other); !errors.Is(err, ErrSnapshotCursor) {
			t.Fatalf("boundary accepted: %+v err=%v", other, err)
		}
	}
	for _, bad := range []string{"", strings.Repeat("x", 2049), "x" + token[1:], token + "."} {
		if _, err := service.decodeSnapshotCursor(bad, expected); !errors.Is(err, ErrSnapshotCursor) {
			t.Fatalf("bad cursor accepted: %v", err)
		}
	}
	otherService := &ControlService{snapshotCursorKey: newSnapshotCursorKey()}
	if _, err := otherService.decodeSnapshotCursor(token, expected); !errors.Is(err, ErrSnapshotCursor) {
		t.Fatal("different signing key accepted")
	}
}

func TestSnapshotByteBudgetAccountsForJSONEncoding(t *testing.T) {
	items := make([]string, 200)
	for i := range items {
		items[i] = strings.Repeat("<", 1024)
	}
	page, more, err := boundedSnapshotItems(items, 200)
	if err != nil || !more || len(page) < 1 || len(page) >= len(items) {
		t.Fatalf("page items=%d more=%v err=%v", len(page), more, err)
	}
	if _, _, err := boundedSnapshotItems([]string{strings.Repeat("x", domain.SnapshotPageBytes)}, 1); !errors.Is(err, domain.ErrInvalidSnapshot) {
		t.Fatalf("oversized item accepted: %v", err)
	}
	page, more, err = boundedSnapshotItems([]string{"a", "b"}, 1)
	if err != nil || !more || len(page) != 1 || page[0] != "a" {
		t.Fatalf("count bound=%v more=%v err=%v", page, more, err)
	}
}
