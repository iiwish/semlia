package catalog_test

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"

	application "github.com/iiwish/semlia/internal/application/catalog"
	domain "github.com/iiwish/semlia/internal/domain/catalog"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestCatalogSearchContractMixedMatchesAndCursors(t *testing.T) {
	pool, _, service := fixture(t)
	ctx := context.Background()
	workspace := createWorkspace(t, pool, "search-contract")
	otherWorkspace := createWorkspace(t, pool, "search-contract-other")
	const query = "catalog.signal"
	create := func(workspace identity.WorkspaceID, address string, kind semantic.AssetType, lifecycle, definition string) domain.AssetDetail {
		t.Helper()
		content, err := json.Marshal(map[string]string{"displayName": "Entry", "name": "Legacy", "definition": definition})
		if err != nil {
			t.Fatal(err)
		}
		asset, err := service.CreateAsset(ctx, application.CreateAssetRequest{
			WorkspaceID: workspace, Address: address, AssetType: kind, Lifecycle: lifecycle,
			SchemaVersion: "1.0.0", Content: content, CreatedBy: "search-contract", TraceID: traceID,
		})
		if err != nil {
			t.Fatal(err)
		}
		return asset
	}
	exact := create(workspace, query, semantic.Metric, "active", query)
	prefix := create(workspace, "catalog.signal_extra", semantic.Metric, "archived", "quiet")
	fulltext := create(workspace, "catalog.fulltext", semantic.Metric, "active", query)
	newContent := create(workspace, "catalog.new_content", semantic.BusinessObject, "active", "quiet")
	substring := create(workspace, "xcatalog.signal", semantic.BusinessObject, "active", "quiet")
	oldContent := create(workspace, "catalog.old_content", semantic.Metric, "active", query)
	create(workspace, "catalog.unrelated", semantic.Metric, "active", "quiet")
	create(otherWorkspace, query, semantic.Metric, "active", query)
	create(otherWorkspace, "other.fulltext", semantic.BusinessObject, "active", query)
	for _, change := range []struct {
		asset      domain.AssetDetail
		definition string
	}{{newContent, query}, {oldContent, "quiet"}} {
		content, err := json.Marshal(map[string]string{"displayName": "Entry", "name": "Legacy", "definition": change.definition})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.AppendRevision(ctx, application.AppendRevisionRequest{
			WorkspaceID: workspace, AssetID: change.asset.ID, SchemaVersion: "1.0.0",
			Content: content, CreatedBy: "search-contract", TraceID: traceID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	missingID := mustID(t, identity.NewAssetID)
	if _, err := pool.Exec(ctx, `INSERT INTO semantic_assets
		(id, workspace_id, namespace, key, asset_type, lifecycle_state)
		VALUES ($1, $2, 'missing.catalog', 'signal', 'metric', 'draft')`, missingID.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	updatedAt := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE semantic_assets SET updated_at = $2 WHERE workspace_id = $1`, workspace.UUID(), updatedAt); err != nil {
		t.Fatal(err)
	}
	matched := []string{query, "catalog.signal_extra", "catalog.fulltext", "catalog.new_content", "xcatalog.signal", "missing.catalog.signal"}
	for _, test := range []struct {
		name, search, lifecycle string
		kind                    semantic.AssetType
		workspace               identity.WorkspaceID
		want                    []string
	}{
		{"all_match_modes", query, "", "", workspace, matched},
		{"trim_and_case", "  CATALOG.SIGNAL  ", "", "", workspace, matched},
		{"active_only", query, "active", "", workspace, []string{query, "catalog.fulltext", "catalog.new_content", "xcatalog.signal"}},
		{"metric_only", query, "", semantic.Metric, workspace, []string{query, "catalog.signal_extra", "catalog.fulltext", "missing.catalog.signal"}},
		{"combined_filters", query, "active", semantic.Metric, workspace, []string{query, "catalog.fulltext"}},
		{"other_workspace", query, "", "", otherWorkspace, []string{query, "other.fulltext"}},
		{"no_match", "absentlexeme", "", "", workspace, nil},
		{"empty_search", "", "", "", workspace, append(append([]string{}, matched...), "catalog.old_content", "catalog.unrelated")},
	} {
		t.Run(test.name, func(t *testing.T) {
			page, err := service.ListAssets(ctx, application.ListAssetsRequest{
				WorkspaceID: test.workspace, Search: test.search, AssetType: test.kind, Lifecycle: test.lifecycle, Limit: 50,
			})
			if err != nil {
				t.Fatal(err)
			}
			assertSearchAddresses(t, page, test.want)
		})
	}

	page, err := service.ListAssets(ctx, application.ListAssetsRequest{WorkspaceID: workspace, Search: query, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[identity.AssetID]domain.AssetSummary{}
	for _, item := range page.Items {
		byID[item.ID] = item
		if !item.UpdatedAt.Equal(updatedAt) {
			t.Fatal("fixture requires tied update timestamps")
		}
	}
	if byID[exact.ID].Rank != 3 || byID[prefix.ID].Rank != 2 || byID[substring.ID].Rank != 0 || byID[missingID].Rank != 0 {
		t.Fatal("exact, prefix and substring rank tiers changed")
	}
	if byID[fulltext.ID].Rank <= 0 || byID[fulltext.ID].Rank >= 2 || byID[fulltext.ID].Rank != byID[newContent.ID].Rank {
		t.Fatal("equal current content must have equal full-text rank below address prefixes")
	}
	if missing := byID[missingID]; missing.CurrentRevisionID != nil || missing.Title != "signal" || missing.Summary != "" {
		t.Fatalf("missing revision fallback changed: %+v", missing)
	}
	ftsIDs := []identity.AssetID{fulltext.ID, newContent.ID}
	zeroIDs := []identity.AssetID{substring.ID, missingID}
	for _, ids := range [][]identity.AssetID{ftsIDs, zeroIDs} {
		sort.Slice(ids, func(i, j int) bool { return ids[i].UUID() > ids[j].UUID() })
	}
	wantIDs := append([]identity.AssetID{exact.ID, prefix.ID}, append(ftsIDs, zeroIDs...)...)
	var actualIDs []identity.AssetID
	for _, item := range page.Items {
		actualIDs = append(actualIDs, item.ID)
	}
	if !reflect.DeepEqual(actualIDs, wantIDs) {
		t.Fatalf("rank/update/id ordering = %v, want %v", actualIDs, wantIDs)
	}
	var cursor string
	seen := map[identity.AssetID]bool{}
	for index, want := range wantIDs {
		next, err := service.ListAssets(ctx, application.ListAssetsRequest{WorkspaceID: workspace, Search: query, Limit: 1, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if next.Total != int64(len(wantIDs)) || len(next.Items) != 1 || next.Items[0].ID != want || seen[next.Items[0].ID] {
			t.Fatalf("page %d changed total/order or repeated an asset: %+v", index, next)
		}
		seen[want] = true
		if next.Items[0].Rank != byID[want].Rank {
			t.Fatal("cursor changed the rank")
		}
		cursor = next.NextCursor
		if (cursor == "") != (index == len(wantIDs)-1) {
			t.Fatalf("page %d has an incorrect continuation", index)
		}
	}
}

func TestCatalogSearchContractPreservesAddressWildcards(t *testing.T) {
	pool, _, service := fixture(t)
	workspace := createWorkspace(t, pool, "search-wildcards")
	for _, address := range []string{"wild.code_one", "wild.codexone", "wild.code_long_one", "wild.unrelated"} {
		createAsset(t, service, workspace, address, semantic.Metric, `{"name":"Entry","definition":"quiet"}`)
	}
	for _, test := range []struct {
		search string
		want   []string
	}{
		{"wild.code_one", []string{"wild.code_one", "wild.codexone"}},
		{"wild.code%one", []string{"wild.code_one", "wild.codexone", "wild.code_long_one"}},
		{`wild.code\_one`, []string{"wild.code_one"}},
	} {
		t.Run(test.search, func(t *testing.T) {
			page, err := service.ListAssets(context.Background(), application.ListAssetsRequest{WorkspaceID: workspace, Search: test.search, Limit: 50})
			if err != nil {
				t.Fatal(err)
			}
			assertSearchAddresses(t, page, test.want)
		})
	}
}

func assertSearchAddresses(t *testing.T, page application.AssetPage, want []string) {
	t.Helper()
	got := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		got = append(got, item.Address.String())
	}
	expected := append([]string{}, want...)
	sort.Strings(got)
	sort.Strings(expected)
	if !reflect.DeepEqual(got, expected) || page.Total != int64(len(want)) || page.NextCursor != "" {
		t.Fatalf("addresses=%v total=%d cursor=%t, want=%v total=%d", got, page.Total, page.NextCursor != "", expected, len(want))
	}
}
