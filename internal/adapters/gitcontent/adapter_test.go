package gitcontent_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/gitcontent"
	domain "github.com/iiwish/semlia/internal/domain/projection"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestProjectAssetGoldenReplayAndOptimisticAppend(t *testing.T) {
	root := t.TempDir()
	adapter, err := gitcontent.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	first := assetFixture(t, 1)
	result, err := adapter.ProjectAsset(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || len(result.CommitHash) != 40 || result.Path != "assets/commerce/revenue/net_revenue.json" {
		t.Fatalf("first result = %+v", result)
	}
	bytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.Path)))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		`"apiVersion": "semlia.io/v1"`, `"kind": "SemanticAsset"`,
		`"address": "commerce.revenue.net_revenue"`, `"assetId": "` + first.AssetID.String() + `"`,
		`"revisionId": "` + first.RevisionID.String() + `"`, `"name": "Net revenue"`,
		`"threshold": 9007199254740993`,
	} {
		if !strings.Contains(string(bytes), fragment) {
			t.Fatalf("golden document missing %s:\n%s", fragment, bytes)
		}
	}
	if bytes[len(bytes)-1] != '\n' {
		t.Fatal("projection has no trailing newline")
	}

	replay, err := adapter.ProjectAsset(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Changed || replay.CommitHash != result.CommitHash {
		t.Fatalf("replay result = %+v, first = %+v", replay, result)
	}

	second := assetFixture(t, 2)
	second.WorkspaceID, second.AssetID, second.Address = first.WorkspaceID, first.AssetID, first.Address
	second.ExpectedBaseRevisionID = &first.RevisionID
	secondResult, err := adapter.ProjectAsset(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if !secondResult.Changed || secondResult.CommitHash == result.CommitHash {
		t.Fatalf("append result = %+v", secondResult)
	}

	stale := assetFixture(t, 3)
	stale.WorkspaceID, stale.AssetID, stale.Address = first.WorkspaceID, first.AssetID, first.Address
	stale.ExpectedBaseRevisionID = &first.RevisionID
	before, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.Path)))
	if _, err := adapter.ProjectAsset(context.Background(), stale); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale base error = %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.Path)))
	if string(before) != string(after) {
		t.Fatal("stale base modified projection")
	}
}

func TestProjectAssetRejectsDirtyWorktreeWithoutModification(t *testing.T) {
	root := t.TempDir()
	adapter, err := gitcontent.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manual.txt"), []byte("manual\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	asset := assetFixture(t, 1)
	if _, err := adapter.ProjectAsset(context.Background(), asset); !errors.Is(err, domain.ErrDirty) {
		t.Fatalf("dirty error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "assets")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dirty projection created assets path: %v", err)
	}
}

func assetFixture(t *testing.T, sequence int64) domain.Asset {
	t.Helper()
	workspace, _ := identity.NewWorkspaceID()
	asset, _ := identity.NewAssetID()
	revision, _ := identity.NewRevisionID()
	address, _ := semantic.NewAddress("commerce.revenue", "net_revenue")
	return domain.Asset{
		WorkspaceID: workspace, AssetID: asset, RevisionID: revision, Address: address,
		AssetType: semantic.Metric, LifecycleState: "active", Sequence: sequence,
		SchemaVersion: "1.0.0", ContentDigest: "sha256:0123456789", Content: json.RawMessage(`{"summary":"Gross less refunds","name":"Net revenue","threshold":9007199254740993}`),
		CreatedBy: "founder", CreatedAt: time.Date(2026, 9, 2, 3, 4, 5, 999, time.UTC),
	}
}
