package catalog_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	application "github.com/iiwish/semlia/internal/application/catalog"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/catalog"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

// relationRepository serves a small graph of typed specs plus any stored relation records.
type relationRepository struct {
	fakeRepository
	subject    domain.AssetDetail
	refs       map[string]domain.AssetRef
	referrers  []domain.ReferencingAsset
	stored     []domain.AuthorityRecord
	storedCall int
}

func (repository *relationRepository) GetCatalogAsset(context.Context, identity.WorkspaceID, identity.AssetID) (domain.AssetDetail, error) {
	return repository.subject, nil
}

func (repository *relationRepository) ListCatalogAssetRefs(_ context.Context, _ identity.WorkspaceID, ids []identity.AssetID) ([]domain.AssetRef, error) {
	var found []domain.AssetRef
	for _, id := range ids {
		if ref, ok := repository.refs[id.String()]; ok {
			found = append(found, ref)
		}
	}
	return found, nil
}

func (repository *relationRepository) ListCatalogAssetsReferencing(context.Context, identity.WorkspaceID, identity.AssetID, int) ([]domain.ReferencingAsset, error) {
	return repository.referrers, nil
}

func (repository *relationRepository) ListCatalogAuthorityRecords(_ context.Context, query domain.ListAuthorityRecordsQuery) (domain.AuthorityRecordResult, error) {
	repository.storedCall++
	items := repository.stored
	if query.Cursor != nil {
		for index, item := range items {
			if item.ID == query.Cursor.ID {
				items = items[index+1:]
				break
			}
		}
	}
	if len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return domain.AuthorityRecordResult{Items: items, Total: int64(len(repository.stored))}, nil
}

func specContent(t *testing.T, spec map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"displayName": "x", "spec": spec})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func ref(asset identity.AssetID) map[string]any {
	return map[string]any{"assetId": asset.String(), "revisionId": "rev_x", "releaseId": "rls_x", "memberId": "m"}
}

// graph: metric -> measures -> orders; model contains orders and the metric; term describes orders.
type graph struct {
	repository                       *relationRepository
	workspace                        identity.WorkspaceID
	orders, metric, model, term, dat identity.AssetID
}

func newGraph(t *testing.T) graph {
	t.Helper()
	g := graph{workspace: mustID(t, identity.NewWorkspaceID), orders: mustID(t, identity.NewAssetID), metric: mustID(t, identity.NewAssetID),
		model: mustID(t, identity.NewAssetID), term: mustID(t, identity.NewAssetID), dat: mustID(t, identity.NewAssetID)}
	g.repository = &relationRepository{refs: map[string]domain.AssetRef{
		g.orders.String(): {ID: g.orders, Type: semantic.BusinessObject, Title: "订单"},
		g.metric.String(): {ID: g.metric, Type: semantic.Metric, Title: "有效支付金额"},
		g.dat.String():    {ID: g.dat, Type: semantic.DataAsset, Title: "订单演示数据"},
	}}
	metricContent := specContent(t, map[string]any{"kind": "aggregate", "inputRef": ref(g.orders), "timeAttributeRef": ref(g.orders)})
	g.repository.referrers = []domain.ReferencingAsset{
		{AssetRef: domain.AssetRef{ID: g.metric, Type: semantic.Metric, Title: "有效支付金额"}, Content: metricContent},
		{AssetRef: domain.AssetRef{ID: g.model, Type: semantic.AnalysisModel, Title: "分析模型"}, Content: specContent(t, map[string]any{"baseObjectRef": ref(g.orders), "metricRefs": []any{ref(g.metric)}})},
		{AssetRef: domain.AssetRef{ID: g.term, Type: semantic.BusinessTerm, Title: "有效订单"}, Content: specContent(t, map[string]any{"subjectRef": ref(g.orders)})},
	}
	g.repository.subject = domain.AssetDetail{AssetSummary: domain.AssetSummary{ID: g.orders, Type: semantic.BusinessObject}, CurrentRevision: &domain.Revision{Content: specContent(t, map[string]any{})}}
	return g
}

func (g graph) page(t *testing.T, limit int, cursor string) application.AuthorityRecordPage {
	t.Helper()
	service := application.NewService(g.repository, application.ClockFunc(time.Now))
	page, err := service.ListAuthorityRecords(context.Background(), application.ListAuthorityRecordsRequest{WorkspaceID: g.workspace, AssetID: g.subjectID(), Section: "relations", Limit: limit, Cursor: cursor})
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func (g graph) subjectID() identity.AssetID { return g.repository.subject.ID }

func TestIncomingRelationsAreProjectedFromReferencingSpecs(t *testing.T) {
	g := newGraph(t)
	page := g.page(t, 100, "")
	if page.Total != 4 || len(page.Items) != 4 || page.NextCursor != "" {
		t.Fatalf("total=%d items=%d next=%q: %+v", page.Total, len(page.Items), page.NextCursor, page.Items)
	}
	// metric: measures + depends_on (time attribute); model: contains; term: describes.
	predicates := map[string]string{}
	for _, record := range page.Items {
		if record.Authority != "spec_references" || record.Status != "inferred" || record.Relation.AssertionState != "inferred" || record.Relation.Direction != "incoming" {
			t.Fatalf("projected relation must be marked inferred and incoming: %+v", record)
		}
		if record.Relation.ObjectAssetID != g.orders || record.Relation.CounterpartTitle == "" || !strings.HasPrefix(record.Relation.InferenceRule, "spec_reference:spec.") {
			t.Fatalf("relation = %+v", record.Relation)
		}
		predicates[record.RelatedID+"/"+record.Relation.Predicate] = record.Relation.CounterpartTitle
	}
	for key, title := range map[string]string{
		g.metric.String() + "/measures": "有效支付金额", g.metric.String() + "/depends_on": "有效支付金额",
		g.model.String() + "/contains": "分析模型", g.term.String() + "/describes": "有效订单",
	} {
		if predicates[key] != title {
			t.Fatalf("missing %s in %v", key, predicates)
		}
	}
}

func TestOutgoingRelationsComeFromTheAssetsOwnSpec(t *testing.T) {
	g := newGraph(t)
	g.repository.referrers = nil
	g.repository.subject = domain.AssetDetail{AssetSummary: domain.AssetSummary{ID: g.metric, Type: semantic.Metric},
		CurrentRevision: &domain.Revision{Content: specContent(t, map[string]any{"kind": "aggregate", "inputRef": ref(g.orders), "filterRefs": []any{ref(g.dat)}})}}
	page := g.page(t, 100, "")
	if page.Total != 2 {
		t.Fatalf("relations = %+v", page.Items)
	}
	for _, record := range page.Items {
		if record.Relation.Direction != "outgoing" || record.Relation.SubjectAssetID != g.metric {
			t.Fatalf("relation = %+v", record.Relation)
		}
		switch record.RelatedID {
		case g.orders.String():
			if record.Relation.Predicate != "measures" || record.Relation.Plane != "semantic" || record.Relation.InferenceRule != "spec_reference:spec.inputRef" {
				t.Fatalf("relation = %+v", record.Relation)
			}
		case g.dat.String():
			// filters_by targets only business terms; a data asset reference falls back to a plain dependency.
			if record.Relation.Predicate != "depends_on" || record.Relation.Plane != "dependency" || record.Relation.InferenceRule != "spec_reference:spec.filterRefs" {
				t.Fatalf("relation = %+v", record.Relation)
			}
		default:
			t.Fatalf("unexpected counterpart %s", record.RelatedID)
		}
	}
}

func TestRelationPagingWalksDerivedThenStoredWithoutRepeats(t *testing.T) {
	g := newGraph(t)
	for index := 0; index < 3; index++ {
		id := mustID(t, identity.NewRelationID)
		g.repository.stored = append(g.repository.stored, domain.AuthorityRecord{Kind: "relation", ID: id.String(), Authority: "semantic_relations", Status: "asserted", Label: "synonym_of",
			Relation: &domain.RelationAuthority{Direction: "outgoing", Predicate: "synonym_of", Plane: "taxonomy", AssertionState: "asserted", SubjectAssetID: g.orders, ObjectAssetID: g.model}})
	}
	for _, limit := range []int{1, 2, 3, 4, 5, 7, 100} {
		seen := map[string]bool{}
		cursor, pages := "", 0
		for {
			page := g.page(t, limit, cursor)
			pages++
			if page.Total != 7 {
				t.Fatalf("limit %d: total = %d", limit, page.Total)
			}
			for _, record := range page.Items {
				if seen[record.ID] {
					t.Fatalf("limit %d: repeated record %s", limit, record.ID)
				}
				seen[record.ID] = true
			}
			if page.NextCursor == "" {
				break
			}
			if pages > 20 {
				t.Fatalf("limit %d: paging does not terminate", limit)
			}
			cursor = page.NextCursor
		}
		if len(seen) != 7 {
			t.Fatalf("limit %d: saw %d of 7 records (%d pages)", limit, len(seen), pages)
		}
	}
}

func TestProjectedRelationsAreIdempotentAndStable(t *testing.T) {
	g := newGraph(t)
	first, second := g.page(t, 100, ""), g.page(t, 100, "")
	if !reflect.DeepEqual(first.Items, second.Items) {
		t.Fatal("projection must be deterministic across reads")
	}
	ids := map[string]bool{}
	for _, record := range first.Items {
		if ids[record.ID] || !strings.HasPrefix(record.ID, "rel_") {
			t.Fatalf("ids must be unique and typeid shaped: %s", record.ID)
		}
		ids[record.ID] = true
	}
}

func TestResumingWithAStaleDerivedCursorIsRejected(t *testing.T) {
	g := newGraph(t)
	cursor := g.page(t, 1, "").NextCursor
	if cursor == "" {
		t.Fatal("expected a cursor")
	}
	g.repository.referrers = g.repository.referrers[:1]
	service := application.NewService(g.repository, application.ClockFunc(time.Now))
	g.repository.referrers = nil
	if _, err := service.ListAuthorityRecords(context.Background(), application.ListAuthorityRecordsRequest{WorkspaceID: g.workspace, AssetID: g.subjectID(), Section: "relations", Limit: 1, Cursor: cursor}); err == nil {
		t.Fatal("a cursor into a segment that no longer contains its record must be rejected, not silently restarted")
	}
}

func TestProjectedRelationsNeverNameAssetsTheCallerCannotRead(t *testing.T) {
	g := newGraph(t)
	principal := mustID(t, identity.NewPrincipalID)
	var bindings []authorization.RoleBinding
	for _, readable := range []identity.AssetID{g.orders, g.metric} {
		bindings = append(bindings, authorization.RoleBinding{
			WorkspaceID: g.workspace, PrincipalID: principal, RoleID: "asset-reader", RoleVersion: 1,
			ScopeType: authorization.ScopeAsset, ScopeID: readable.UUID(), Actions: []authorization.Action{authorization.ActionAssetRead},
		})
	}
	evaluator := &catalogSnapshotEvaluator{
		catalogEvaluator: catalogEvaluator{decision: authorization.Decision{Allowed: true, PrincipalID: principal, Action: authorization.ActionAssetRead}},
		snapshot: authorizationapp.AccessSnapshot{
			Principal:   authorization.Principal{ID: principal, WorkspaceID: g.workspace, Kind: authorization.PrincipalHuman, Status: authorization.PrincipalActive},
			Bindings:    bindings,
			EvaluatedAt: time.Now().UTC(),
		},
	}
	service := application.NewService(g.repository, application.ClockFunc(time.Now), application.WithAuthorizer(evaluator))
	page, err := service.ListAuthorityRecords(context.Background(), application.ListAuthorityRecordsRequest{
		WorkspaceID: g.workspace, AssetID: g.orders, Section: "relations", Limit: 100, PrincipalRef: principal.String(), TraceID: strings.Repeat("e", 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("only the readable metric's two relations may appear: %+v", page.Items)
	}
	for _, record := range page.Items {
		if record.RelatedID != g.metric.String() {
			t.Fatalf("relation reveals an unreadable asset: %+v", record)
		}
	}
}
