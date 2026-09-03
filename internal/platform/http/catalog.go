package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	contract "github.com/iiwish/semlia/api/gen/go"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/catalog"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

type routeKind int

const (
	routeUnknown routeKind = iota
	routeSystem
	routeWorkspaces
	routeCatalogAssets
	routeCatalogAsset
	routeCatalogRevisions
	routeCatalogRevision
	routeCatalogRelations
	routeDiscoveryRun
)

type matchedRoute struct {
	kind      routeKind
	label     string
	workspace string
	asset     string
	revision  string
	discovery string
	release   string
	proposal  string
	batch     string
	provider  string
	setting   string
}

func matchRoute(path string) matchedRoute {
	switch path {
	case "/health/live", "/health/ready", "/api/v1/system/info":
		return matchedRoute{kind: routeSystem, label: path}
	case "/api/v1/workspaces":
		return matchedRoute{kind: routeWorkspaces, label: path}
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 6 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "workspaces" {
		return matchedRoute{}
	}
	base := matchedRoute{workspace: parts[3]}
	if matched, ok := matchGovernanceRoute(parts, base); ok {
		return matched
	}
	if len(parts) == 6 && parts[4] == "catalog" && parts[5] == "assets" {
		base.kind, base.label = routeCatalogAssets, "/api/v1/workspaces/{workspaceId}/catalog/assets"
		return base
	}
	if len(parts) >= 7 && parts[4] == "catalog" && parts[5] == "assets" {
		base.asset = parts[6]
		switch {
		case len(parts) == 7:
			base.kind, base.label = routeCatalogAsset, "/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}"
		case len(parts) == 8 && parts[7] == "revisions":
			base.kind, base.label = routeCatalogRevisions, "/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/revisions"
		case len(parts) == 9 && parts[7] == "revisions":
			base.kind, base.label, base.revision = routeCatalogRevision, "/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/revisions/{revisionId}", parts[8]
		case len(parts) == 8 && parts[7] == "relations":
			base.kind, base.label = routeCatalogRelations, "/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/relations"
		}
		return base
	}
	if len(parts) == 6 && parts[4] == "discovery-runs" {
		base.kind, base.label, base.discovery = routeDiscoveryRun, "/api/v1/workspaces/{workspaceId}/discovery-runs/{runId}", parts[5]
		return base
	}
	return matchedRoute{}
}

func (route matchedRoute) methods() []string {
	if isGovernanceRoute(route.kind) {
		return governanceRouteMethods(route.kind)
	}
	switch route.kind {
	case routeWorkspaces, routeCatalogAssets, routeCatalogRevisions:
		return []string{http.MethodGet, http.MethodPost}
	default:
		return []string{http.MethodGet}
	}
}

func (route matchedRoute) allows(method string) bool {
	for _, allowed := range route.methods() {
		if method == allowed {
			return true
		}
	}
	return false
}

func (handler *Handler) routeCatalog(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	route matchedRoute,
) string {
	if route.kind == routeWorkspaces {
		return handler.routeWorkspaces(response, request, traceID)
	}
	workspaceID, err := identity.ParseWorkspaceID(route.workspace)
	if err != nil {
		return writeCatalogError(response, domain.ErrInvalidArgument, traceID)
	}
	switch route.kind {
	case routeCatalogAssets:
		if request.Method == http.MethodPost {
			return handler.createCatalogAsset(response, request, traceID, workspaceID)
		}
		return handler.listCatalogAssets(response, request, traceID, workspaceID)
	case routeDiscoveryRun:
		runID, parseErr := identity.ParseRunID(route.discovery)
		if parseErr != nil {
			return writeCatalogError(response, domain.ErrInvalidArgument, traceID)
		}
		value, getErr := handler.catalog.GetDiscoveryRun(request.Context(), workspaceID, runID)
		if getErr != nil {
			return writeCatalogError(response, getErr, traceID)
		}
		writeJSON(response, http.StatusOK, discoveryRunResponse(value))
		return ""
	}
	assetID, err := identity.ParseAssetID(route.asset)
	if err != nil {
		return writeCatalogError(response, domain.ErrInvalidArgument, traceID)
	}
	switch route.kind {
	case routeCatalogAsset:
		value, getErr := handler.catalog.GetAssetObserved(request.Context(), workspaceID, assetID, catalogapp.ReadObservation{
			Channel: "api", TraceID: traceID,
		})
		if getErr != nil {
			return writeCatalogError(response, getErr, traceID)
		}
		writeJSON(response, http.StatusOK, assetDetailResponse(value))
		return ""
	case routeCatalogRevisions:
		if request.Method == http.MethodPost {
			return handler.appendCatalogRevision(response, request, traceID, workspaceID, assetID)
		}
		return handler.listCatalogRevisions(response, request, traceID, workspaceID, assetID)
	case routeCatalogRevision:
		revisionID, parseErr := identity.ParseRevisionID(route.revision)
		if parseErr != nil {
			return writeCatalogError(response, domain.ErrInvalidArgument, traceID)
		}
		value, getErr := handler.catalog.GetRevision(request.Context(), workspaceID, assetID, revisionID)
		if getErr != nil {
			return writeCatalogError(response, getErr, traceID)
		}
		writeJSON(response, http.StatusOK, revisionResponse(value))
		return ""
	case routeCatalogRelations:
		return handler.listCatalogRelations(response, request, traceID, workspaceID, assetID)
	default:
		panic("catalog route is not handled")
	}
}

func (handler *Handler) routeWorkspaces(response http.ResponseWriter, request *http.Request, traceID string) string {
	if request.Method == http.MethodGet {
		items, err := handler.catalog.ListWorkspaces(request.Context())
		if err != nil {
			return writeCatalogError(response, err, traceID)
		}
		result := struct {
			Items []contract.Workspace `json:"items"`
		}{Items: make([]contract.Workspace, 0, len(items))}
		for _, item := range items {
			result.Items = append(result.Items, workspaceResponse(item))
		}
		writeJSON(response, http.StatusOK, result)
		return ""
	}
	var body contract.CreateWorkspaceRequest
	if err := decodeRequest(request, &body); err != nil {
		return writeCatalogError(response, domain.ErrInvalidArgument, traceID)
	}
	item, err := handler.catalog.CreateWorkspace(request.Context(), body.Slug, body.DisplayName, traceID)
	if err != nil {
		return writeCatalogError(response, err, traceID)
	}
	writeJSON(response, http.StatusCreated, workspaceResponse(item))
	return ""
}

func (handler *Handler) listCatalogAssets(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) string {
	limit, err := queryInteger(request, "limit")
	if err != nil {
		return writeCatalogError(response, domain.ErrInvalidArgument, traceID)
	}
	page, err := handler.catalog.ListAssets(request.Context(), catalogapp.ListAssetsRequest{
		WorkspaceID: workspaceID, Search: request.URL.Query().Get("search"),
		AssetType: semantic.AssetType(request.URL.Query().Get("assetType")),
		Lifecycle: request.URL.Query().Get("lifecycleState"), Limit: limit,
		Cursor: request.URL.Query().Get("cursor"), Channel: "api", TraceID: traceID,
	})
	if err != nil {
		return writeCatalogError(response, err, traceID)
	}
	items := make([]contract.CatalogAssetSummary, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, assetSummaryResponse(item))
	}
	result := contract.CatalogPage{Items: items, Page: contract.PageInfo{Limit: page.Limit}}
	if page.NextCursor != "" {
		result.Page.NextCursor = &page.NextCursor
	}
	writeJSON(response, http.StatusOK, result)
	return ""
}

func (handler *Handler) createCatalogAsset(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) string {
	var body contract.CreateCatalogAssetRequest
	if err := decodeRequest(request, &body); err != nil {
		return writeCatalogError(response, domain.ErrInvalidArgument, traceID)
	}
	content, _ := json.Marshal(body.Content)
	lifecycle := ""
	if body.LifecycleState != nil {
		lifecycle = string(*body.LifecycleState)
	}
	value, err := handler.catalog.CreateAsset(request.Context(), catalogapp.CreateAssetRequest{
		WorkspaceID: workspaceID, Address: body.Address, AssetType: semantic.AssetType(body.AssetType),
		Lifecycle: lifecycle, SchemaVersion: body.SchemaVersion, Content: content,
		CreatedBy: body.CreatedBy, EvidenceIDs: evidenceIDs(body.EvidenceIds),
		PrincipalRef: strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
	})
	if err != nil {
		return writeCatalogError(response, err, traceID)
	}
	writeJSON(response, http.StatusCreated, assetDetailResponse(value))
	return ""
}

func (handler *Handler) listCatalogRevisions(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
	assetID identity.AssetID,
) string {
	limit, err := queryInteger(request, "limit")
	if err != nil {
		return writeCatalogError(response, domain.ErrInvalidArgument, traceID)
	}
	page, err := handler.catalog.ListRevisions(request.Context(), catalogapp.ListRevisionsRequest{
		WorkspaceID: workspaceID, AssetID: assetID, Limit: limit, Cursor: request.URL.Query().Get("cursor"),
	})
	if err != nil {
		return writeCatalogError(response, err, traceID)
	}
	items := make([]contract.AssetRevision, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, revisionResponse(item))
	}
	result := contract.AssetRevisionPage{Items: items, Page: contract.PageInfo{Limit: page.Limit}}
	if page.NextCursor != "" {
		result.Page.NextCursor = &page.NextCursor
	}
	writeJSON(response, http.StatusOK, result)
	return ""
}

func (handler *Handler) appendCatalogRevision(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
	assetID identity.AssetID,
) string {
	var body contract.CreateAssetRevisionRequest
	if err := decodeRequest(request, &body); err != nil {
		return writeCatalogError(response, domain.ErrInvalidArgument, traceID)
	}
	content, _ := json.Marshal(body.Content)
	value, err := handler.catalog.AppendRevision(request.Context(), catalogapp.AppendRevisionRequest{
		WorkspaceID: workspaceID, AssetID: assetID, SchemaVersion: body.SchemaVersion,
		Content: content, CreatedBy: body.CreatedBy, EvidenceIDs: evidenceIDs(body.EvidenceIds),
		PrincipalRef: strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
	})
	if err != nil {
		return writeCatalogError(response, err, traceID)
	}
	writeJSON(response, http.StatusCreated, revisionResponse(value))
	return ""
}

func (handler *Handler) listCatalogRelations(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
	assetID identity.AssetID,
) string {
	depth, err := queryInteger(request, "depth")
	if err != nil {
		return writeCatalogError(response, domain.ErrInvalidArgument, traceID)
	}
	if depth == 0 {
		depth = 1
	}
	items, err := handler.catalog.ListRelations(request.Context(), domain.ListRelationsQuery{
		WorkspaceID: workspaceID, AssetID: assetID, Direction: request.URL.Query().Get("direction"),
		Plane: semantic.RelationPlane(request.URL.Query().Get("plane")), Depth: depth,
	})
	if err != nil {
		return writeCatalogError(response, err, traceID)
	}
	result := contract.AssetRelationPage{Items: make([]contract.AssetRelation, 0, len(items)), MaxDepth: depth}
	for _, item := range items {
		result.Items = append(result.Items, relationResponse(item))
	}
	writeJSON(response, http.StatusOK, result)
	return ""
}

func decodeRequest(request *http.Request, target any) error {
	reader := io.LimitReader(request.Body, (1<<20)+1)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request contains trailing JSON")
	}
	return nil
}

func queryInteger(request *http.Request, name string) (int, error) {
	value := request.URL.Query().Get(name)
	if value == "" {
		return 0, nil
	}
	result, err := strconv.Atoi(value)
	if err != nil {
		return 0, domain.ErrInvalidArgument
	}
	return result, nil
}

func evidenceIDs(value *[]identity.EvidenceID) []identity.EvidenceID {
	if value == nil {
		return nil
	}
	return append([]identity.EvidenceID(nil), (*value)...)
}

func writeCatalogError(response http.ResponseWriter, err error, traceID string) string {
	var denial *authz.DenialError
	switch {
	case errors.As(err, &denial):
		writeError(response, http.StatusForbidden, string(denial.Decision.ReasonCode),
			"the acting principal lacks the required capability", traceID, false)
		return string(denial.Decision.ReasonCode)
	case errors.Is(err, authz.ErrNotFound):
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the requested resource was not found", traceID, false)
		return "NOT_FOUND"
	case errors.Is(err, domain.ErrInvalidArgument):
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "the request is invalid", traceID, false)
		return "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrNotFound):
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the requested resource was not found", traceID, false)
		return "NOT_FOUND"
	case errors.Is(err, domain.ErrConflict):
		writeError(response, http.StatusConflict, "CONFLICT", "the request conflicts with current state", traceID, false)
		return "CONFLICT"
	case errors.Is(err, domain.ErrInvariant):
		writeError(response, http.StatusUnprocessableEntity, "INVARIANT_VIOLATION", "the request violates a registry invariant", traceID, false)
		return "INVARIANT_VIOLATION"
	default:
		writeError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "the request could not be completed", traceID, false)
		return "INTERNAL_ERROR"
	}
}

func assetSummaryResponse(value domain.AssetSummary) contract.CatalogAssetSummary {
	return contract.CatalogAssetSummary{
		Id: value.ID, Address: value.Address.String(), AssetType: contract.SemanticAssetType(value.Type),
		LifecycleState: contract.AssetLifecycleState(value.LifecycleState), CurrentRevisionId: value.CurrentRevisionID,
		Title: value.Title, Summary: value.Summary, UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func workspaceResponse(value domain.Workspace) contract.Workspace {
	return contract.Workspace{
		Id: value.ID, Slug: value.Slug, DisplayName: value.DisplayName,
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
	}
}

func assetDetailResponse(value domain.AssetDetail) contract.CatalogAssetDetail {
	summary := assetSummaryResponse(value.AssetSummary)
	result := contract.CatalogAssetDetail{
		Id: summary.Id, Address: summary.Address, AssetType: summary.AssetType,
		LifecycleState: summary.LifecycleState, CurrentRevisionId: summary.CurrentRevisionId,
		Title: summary.Title, Summary: summary.Summary, UpdatedAt: summary.UpdatedAt,
		CreatedAt: value.CreatedAt.UTC(), RelationCount: value.RelationCount,
	}
	if value.CurrentRevision != nil {
		revision := revisionResponse(*value.CurrentRevision)
		result.CurrentRevision = &revision
	}
	return result
}

func revisionResponse(value domain.Revision) contract.AssetRevision {
	content := make(map[string]any)
	_ = json.Unmarshal(value.Content, &content)
	result := contract.AssetRevision{
		Id: value.ID, AssetId: value.AssetID, Sequence: value.Sequence, SchemaVersion: value.SchemaVersion,
		ContentDigest: value.ContentDigest, Content: content, CreatedBy: value.CreatedBy,
		CreatedAt: value.CreatedAt.UTC(), Evidence: make([]contract.EvidenceArtifact, 0, len(value.Evidence)),
	}
	for _, evidence := range value.Evidence {
		metadata := make(map[string]any)
		_ = json.Unmarshal(evidence.Metadata, &metadata)
		item := contract.EvidenceArtifact{
			Id: evidence.ID, EvidenceType: contract.EvidenceArtifactEvidenceType(evidence.EvidenceType),
			SourceRevisionId: evidence.SourceRevisionID, Locator: evidence.Locator,
			ContentDigest: evidence.ContentDigest, Metadata: metadata,
			Role: contract.EvidenceArtifactRole(evidence.Role), CreatedAt: evidence.CreatedAt.UTC(),
		}
		if evidence.FieldPath != "" {
			item.FieldPath = &evidence.FieldPath
		}
		if evidence.Note != "" {
			item.Note = &evidence.Note
		}
		result.Evidence = append(result.Evidence, item)
	}
	return result
}

func relationResponse(value domain.Relation) contract.AssetRelation {
	return contract.AssetRelation{
		Id: value.ID, Depth: value.Depth, Direction: contract.AssetRelationDirection(value.Direction),
		Predicate: contract.RelationPredicate(value.Predicate), Plane: contract.RelationPlane(value.Plane),
		AssertionState: contract.RelationAssertionState(value.AssertionState),
		SubjectAssetId: value.SubjectAssetID, ObjectAssetId: value.ObjectAssetID,
		Counterpart: assetSummaryResponse(value.Counterpart), EvidenceArtifactId: value.EvidenceArtifactID,
		SourceRevisionId: value.SourceRevisionID, CreatedAt: value.CreatedAt.UTC(),
	}
}

func discoveryRunResponse(value domain.DiscoveryRun) contract.DiscoveryRun {
	stats := make(map[string]any)
	_ = json.Unmarshal(value.Stats, &stats)
	result := contract.DiscoveryRun{
		Id: value.ID, SourceConnectionId: value.SourceConnectionID, SourceRevisionId: value.SourceRevisionID,
		AdapterVersion: value.AdapterVersion, Status: contract.DiscoveryRunStatus(value.Status), Stats: stats,
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(),
		Findings: make([]contract.DiscoveryFinding, 0, len(value.Findings)),
	}
	if value.ErrorCode != "" {
		result.ErrorCode = &value.ErrorCode
	}
	if value.StartedAt != nil {
		started := value.StartedAt.UTC()
		result.StartedAt = &started
	}
	if value.CompletedAt != nil {
		completed := value.CompletedAt.UTC()
		result.CompletedAt = &completed
	}
	for _, finding := range value.Findings {
		details := make(map[string]any)
		_ = json.Unmarshal(finding.Details, &details)
		item := contract.DiscoveryFinding{
			Sequence: finding.Sequence, Code: finding.Code,
			Severity: contract.DiscoveryFindingSeverity(finding.Severity), Details: details,
		}
		if finding.Locator != "" {
			item.Locator = &finding.Locator
		}
		result.Findings = append(result.Findings, item)
	}
	return result
}
