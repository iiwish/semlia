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
	identitydomain "github.com/iiwish/semlia/internal/domain/identity"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

type routeKind int

const (
	routeUnknown routeKind = iota
	routeSystem
	routeAuthLogin
	routePasswordLogin
	routeAuthMethods
	routePasswordChange
	routeAuthCallback
	routeSession
	routeWorkspaces
	routeWorkspaceMembers
	routeWorkspaceInvitations
	routeWorkspaceMembership
	routeCatalogAssets
	routeCatalogAsset
	routeCatalogAuthoritySection
	routeCatalogRevisions
	routeCatalogRevision
	routeCatalogRelations
	routeDiscoveryRun
	routeSources
	routeSource
	routeSourceCredential
	routeSourceTest
	routeSourceRuns
	routeSourceSnapshots
	routeSourceSnapshot
	routeSourceSnapshotMembers
	routeSourceSnapshotDiagnostics
	routeCandidates
	routeCandidate
	routeCandidateDecisions
	routeIngestionArtifacts
	routeIngestionArtifactSet
	routeIngestionFinalize
	routeIngestionSQLRegistrations
	routeSourceSchedules
	routeSchedule
	routeSchedulePause
	routeScheduleResume
	routeScheduleRunNow
	routeScheduleOccurrences
	routeProductionOperations
	routeProductionOperation
	routeProductionSubmit
	routeProductionValidations
	routeProductionReviews
	routeProductionPublish
	routeProductionRelease
	routeProductionRollback
	routeProductionGeneration
	routeProductionGenerationRun
	routeProductionBusinessRules
)

type matchedRoute struct {
	kind        routeKind
	label       string
	operation   string
	workspace   string
	asset       string
	revision    string
	discovery   string
	source      string
	snapshot    string
	candidate   string
	consumer    string
	binding     string
	query       string
	plan        string
	release     string
	proposal    string
	batch       string
	provider    string
	setting     string
	membership  string
	run         string
	export      string
	attention   string
	section     string
	schedule    string
	artifactSet string
}

func matchRoute(path string) matchedRoute {
	switch path {
	case "/health/live", "/health/ready", "/api/v1/system/info":
		return matchedRoute{kind: routeSystem, label: path}
	case "/api/v1/auth/login":
		return matchedRoute{kind: routeAuthLogin, label: path}
	case "/api/v1/auth/password/login":
		return matchedRoute{kind: routePasswordLogin, label: path}
	case "/api/v1/auth/methods":
		return matchedRoute{kind: routeAuthMethods, label: path}
	case "/api/v1/session/password":
		return matchedRoute{kind: routePasswordChange, label: path}
	case "/api/v1/auth/callback":
		return matchedRoute{kind: routeAuthCallback, label: path}
	case "/api/v1/session":
		return matchedRoute{kind: routeSession, label: path}
	case "/api/v1/workspaces":
		return matchedRoute{kind: routeWorkspaces, label: path}
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "workspaces" {
		return matchedRoute{}
	}
	base := matchedRoute{workspace: parts[3]}
	if len(parts) == 5 && parts[4] == "embedding-index" {
		base.kind, base.label = routeEmbeddingStatus, "/api/v1/workspaces/{workspaceId}/embedding-index"
		return base
	}
	if len(parts) == 5 && parts[4] == "embedding-search" {
		base.kind, base.label = routeEmbeddingSearch, "/api/v1/workspaces/{workspaceId}/embedding-search"
		return base
	}
	if len(parts) == 7 && parts[4] == "embedding-index" && parts[6] == "cancel" {
		base.kind, base.run, base.label = routeEmbeddingCancel, parts[5], "/api/v1/workspaces/{workspaceId}/embedding-index/{indexId}/cancel"
		return base
	}
	if len(parts) == 5 && parts[4] == "members" {
		base.kind, base.label = routeWorkspaceMembers, "/api/v1/workspaces/{workspaceId}/members"
		return base
	}
	if len(parts) == 5 && parts[4] == "invitations" {
		base.kind, base.label = routeWorkspaceInvitations, "/api/v1/workspaces/{workspaceId}/invitations"
		return base
	}
	if len(parts) == 6 && parts[4] == "members" {
		base.kind, base.label, base.membership = routeWorkspaceMembership, "/api/v1/workspaces/{workspaceId}/members/{membershipId}", parts[5]
		return base
	}
	if matched, ok := matchGovernanceRoute(parts, base); ok {
		return matched
	}
	if matched, ok := matchDistributionRoute(parts, base); ok {
		return matched
	}
	if len(parts) >= 5 && parts[4] == "production-operations" {
		if len(parts) == 5 {
			base.kind, base.label = routeProductionOperations, "/api/v1/workspaces/{workspaceId}/production-operations"
			return base
		}
		base.operation = parts[5]
		if len(parts) == 8 && parts[6] == "generation" {
			base.kind, base.label, base.run = routeProductionGenerationRun, "/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/generation/{runId}", parts[7]
			return base
		}
		if len(parts) == 6 {
			base.kind, base.label = routeProductionOperation, "/api/v1/workspaces/{workspaceId}/production-operations/{operationId}"
			return base
		}
		if len(parts) == 7 {
			switch parts[6] {
			case "business-rule-confirmations":
				base.kind, base.label = routeProductionBusinessRules, "/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/business-rule-confirmations"
				return base
			case "generation":
				base.kind, base.label = routeProductionGeneration, "/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/generation"
				return base
			case "submit":
				base.kind, base.label = routeProductionSubmit, "/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/submit"
				return base
			case "validations":
				base.kind, base.label = routeProductionValidations, "/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/validations"
				return base
			case "reviews":
				base.kind, base.label = routeProductionReviews, "/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/reviews"
				return base
			case "publish":
				base.kind, base.label = routeProductionPublish, "/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/publish"
				return base
			}
		}
	}
	if len(parts) >= 5 && parts[4] == "production-releases" {
		if len(parts) == 6 {
			base.kind, base.label, base.release = routeProductionRelease, "/api/v1/workspaces/{workspaceId}/production-releases/{releaseId}", parts[5]
			return base
		}
		if len(parts) == 7 && parts[6] == "rollback" {
			base.kind, base.label, base.release = routeProductionRollback, "/api/v1/workspaces/{workspaceId}/production-releases/{releaseId}/rollback", parts[5]
			return base
		}
	}
	if len(parts) == 5 && parts[4] == "sources" {
		base.kind, base.label = routeSources, "/api/v1/workspaces/{workspaceId}/sources"
		return base
	}
	if len(parts) >= 6 && parts[4] == "sources" {
		base.source = parts[5]
		switch {
		case len(parts) == 6:
			base.kind, base.label = routeSource, "/api/v1/workspaces/{workspaceId}/sources/{sourceId}"
		case len(parts) == 7 && parts[6] == "credential":
			base.kind, base.label = routeSourceCredential, "/api/v1/workspaces/{workspaceId}/sources/{sourceId}/credential"
		case len(parts) == 7 && parts[6] == "test":
			base.kind, base.label = routeSourceTest, "/api/v1/workspaces/{workspaceId}/sources/{sourceId}/test"
		case len(parts) == 7 && parts[6] == "discovery-runs":
			base.kind, base.label = routeSourceRuns, "/api/v1/workspaces/{workspaceId}/sources/{sourceId}/discovery-runs"
		case len(parts) == 7 && parts[6] == "snapshots":
			base.kind, base.label = routeSourceSnapshots, "/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots"
		case len(parts) == 8 && parts[6] == "snapshots":
			base.kind, base.snapshot, base.label = routeSourceSnapshot, parts[7], "/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots/{snapshotId}"
		case len(parts) == 9 && parts[6] == "snapshots" && parts[8] == "members":
			base.kind, base.snapshot, base.label = routeSourceSnapshotMembers, parts[7], "/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots/{snapshotId}/members"
		case len(parts) == 9 && parts[6] == "snapshots" && parts[8] == "diagnostics":
			base.kind, base.snapshot, base.label = routeSourceSnapshotDiagnostics, parts[7], "/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots/{snapshotId}/diagnostics"
		case len(parts) == 7 && parts[6] == "schedules":
			base.kind, base.label = routeSourceSchedules, "/api/v1/workspaces/{workspaceId}/sources/{sourceId}/schedules"
		}
		return base
	}
	if len(parts) == 6 && parts[4] == "ingestion" {
		switch parts[5] {
		case "artifacts":
			base.kind, base.label = routeIngestionArtifacts, "/api/v1/workspaces/{workspaceId}/ingestion/artifacts"
		case "artifact-sets:finalize":
			base.kind, base.label = routeIngestionFinalize, "/api/v1/workspaces/{workspaceId}/ingestion/artifact-sets:finalize"
		case "sql-registrations":
			base.kind, base.label = routeIngestionSQLRegistrations, "/api/v1/workspaces/{workspaceId}/ingestion/sql-registrations"
		}
		return base
	}
	if len(parts) == 7 && parts[4] == "ingestion" && parts[5] == "artifact-sets" {
		base.kind, base.label, base.artifactSet = routeIngestionArtifactSet,
			"/api/v1/workspaces/{workspaceId}/ingestion/artifact-sets/{artifactSetId}", parts[6]
		return base
	}
	if len(parts) >= 6 && parts[4] == "schedules" {
		base.schedule = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(parts[5], ":pause"), ":resume"), ":run-now")
		switch {
		case len(parts) == 6 && strings.HasSuffix(parts[5], ":pause"):
			base.kind, base.label = routeSchedulePause, "/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}:pause"
		case len(parts) == 6 && strings.HasSuffix(parts[5], ":resume"):
			base.kind, base.label = routeScheduleResume, "/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}:resume"
		case len(parts) == 6 && strings.HasSuffix(parts[5], ":run-now"):
			base.kind, base.label = routeScheduleRunNow, "/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}:run-now"
		case len(parts) == 6:
			base.kind, base.label = routeSchedule, "/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}"
		case len(parts) == 7 && parts[6] == "occurrences":
			base.kind, base.label = routeScheduleOccurrences, "/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}/occurrences"
		}
		return base
	}
	if len(parts) == 5 && parts[4] == "semantic-candidates" {
		base.kind, base.label = routeCandidates, "/api/v1/workspaces/{workspaceId}/semantic-candidates"
		return base
	}
	if len(parts) >= 6 && parts[4] == "semantic-candidates" {
		base.candidate = parts[5]
		if len(parts) == 6 {
			base.kind, base.label = routeCandidate, "/api/v1/workspaces/{workspaceId}/semantic-candidates/{candidateId}"
		} else if len(parts) == 7 && parts[6] == "decisions" {
			base.kind, base.label = routeCandidateDecisions, "/api/v1/workspaces/{workspaceId}/semantic-candidates/{candidateId}/decisions"
		}
		return base
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
		case len(parts) == 9 && parts[7] == "authority":
			base.kind, base.label, base.section = routeCatalogAuthoritySection,
				"/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/authority/{sectionKind}", parts[8]
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
	if isDistributionRoute(route.kind) {
		return distributionRouteMethods(route.kind)
	}
	switch route.kind {
	case routeProductionOperations:
		return []string{http.MethodGet, http.MethodPost}
	case routeProductionOperation:
		return []string{http.MethodGet, http.MethodPut}
	case routeProductionSubmit:
		return []string{http.MethodPost}
	case routeProductionValidations:
		return []string{http.MethodGet, http.MethodPost}
	case routeProductionReviews:
		return []string{http.MethodPost}
	case routeProductionPublish:
		return []string{http.MethodPost}
	case routeProductionRelease:
		return []string{http.MethodGet}
	case routeProductionRollback:
		return []string{http.MethodPost}
	case routeProductionGeneration:
		return []string{http.MethodPost}
	case routeProductionGenerationRun:
		return []string{http.MethodGet}
	case routeProductionBusinessRules:
		return []string{http.MethodGet, http.MethodPost}
	case routeEmbeddingStatus:
		return []string{http.MethodGet, http.MethodPost}
	case routeEmbeddingCancel:
		return []string{http.MethodPost}
	case routeAuthLogin, routeAuthCallback, routeAuthMethods:
		return []string{http.MethodGet}
	case routePasswordLogin, routePasswordChange:
		return []string{http.MethodPost}
	case routeSession:
		return []string{http.MethodGet, http.MethodDelete}
	case routeWorkspaceMembers:
		return []string{http.MethodGet, http.MethodPost}
	case routeWorkspaceInvitations:
		return []string{http.MethodGet, http.MethodPost}
	case routeWorkspaceMembership:
		return []string{http.MethodPatch}
	case routeSources, routeSourceRuns, routeSourceSchedules, routeIngestionArtifacts:
		return []string{http.MethodGet, http.MethodPost}
	case routeIngestionFinalize, routeIngestionSQLRegistrations, routeSchedulePause, routeScheduleResume, routeScheduleRunNow:
		return []string{http.MethodPost}
	case routeSchedule:
		return []string{http.MethodGet, http.MethodPatch, http.MethodDelete}
	case routeScheduleOccurrences, routeIngestionArtifactSet:
		return []string{http.MethodGet}
	case routeSource:
		return []string{http.MethodGet, http.MethodPatch, http.MethodDelete}
	case routeSourceSnapshots, routeSourceSnapshot, routeSourceSnapshotMembers, routeSourceSnapshotDiagnostics:
		return []string{http.MethodGet}
	case routeSourceCredential:
		return []string{http.MethodPut}
	case routeSourceTest, routeCandidateDecisions:
		return []string{http.MethodPost}
	case routeCandidates, routeCandidate:
		return []string{http.MethodGet}
	case routeWorkspaces, routeCatalogAssets, routeCatalogRevisions:
		return []string{http.MethodGet, http.MethodPost}
	case routeWorkbenchItem:
		return []string{http.MethodGet, http.MethodPatch}
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
			Channel: "api", TraceID: traceID, PrincipalRef: principalRef(request),
		})
		if getErr != nil {
			return writeCatalogError(response, getErr, traceID)
		}
		writeJSON(response, http.StatusOK, assetDetailResponse(value))
		return ""
	case routeCatalogAuthoritySection:
		limit, limitErr := queryInteger(request, "limit")
		if limitErr != nil {
			return writeCatalogError(response, domain.ErrInvalidArgument, traceID)
		}
		page, listErr := handler.catalog.ListAuthorityRecords(request.Context(), catalogapp.ListAuthorityRecordsRequest{
			WorkspaceID: workspaceID, AssetID: assetID, Section: route.section, Limit: limit,
			Cursor: request.URL.Query().Get("cursor"), PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if listErr != nil {
			return writeCatalogError(response, listErr, traceID)
		}
		writeJSON(response, http.StatusOK, authorityRecordPageResponse(page))
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
		value, getErr := handler.catalog.GetRevision(request.Context(), catalogapp.GetRevisionRequest{
			WorkspaceID: workspaceID, AssetID: assetID, RevisionID: revisionID,
			PrincipalRef: principalRef(request), TraceID: traceID,
		})
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
		if current, authenticated := identityFromRequest(request); authenticated {
			allowed := make(map[string]struct{}, len(current.authenticated.Session.Memberships))
			for _, membership := range current.authenticated.Session.Memberships {
				allowed[membership.WorkspaceID.String()] = struct{}{}
			}
			filtered := items[:0]
			for _, item := range items {
				if _, ok := allowed[item.ID.String()]; ok {
					filtered = append(filtered, item)
				}
			}
			items = filtered
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
	if _, authenticated := identityFromRequest(request); authenticated {
		return writeIdentityError(response, identitydomain.ErrForbidden, traceID)
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
		PrincipalRef: principalRef(request),
	})
	if err != nil {
		return writeCatalogError(response, err, traceID)
	}
	items := make([]contract.CatalogAssetSummary, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, assetSummaryResponse(item))
	}
	total := page.Total
	result := contract.CatalogPage{Items: items, Page: contract.PageInfo{Limit: page.Limit, Total: &total}}
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
		PrincipalRef: principalRef(request), TraceID: traceID,
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
		PrincipalRef: principalRef(request), TraceID: traceID,
	})
	if err != nil {
		return writeCatalogError(response, err, traceID)
	}
	items := make([]contract.AssetRevision, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, revisionResponse(item))
	}
	total := page.Total
	result := contract.AssetRevisionPage{Items: items, Page: contract.PageInfo{Limit: page.Limit, Total: &total}}
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
		PrincipalRef: principalRef(request), TraceID: traceID,
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
	limit, err := queryInteger(request, "limit")
	if err != nil {
		return writeCatalogError(response, domain.ErrInvalidArgument, traceID)
	}
	items, err := handler.catalog.ListRelations(request.Context(), catalogapp.ListRelationsRequest{
		WorkspaceID: workspaceID, AssetID: assetID, Direction: request.URL.Query().Get("direction"),
		Plane: semantic.RelationPlane(request.URL.Query().Get("plane")), Depth: depth, Limit: limit,
		PrincipalRef: principalRef(request), TraceID: traceID,
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
		AuthoritySections: make([]contract.CatalogAuthoritySection, 0, len(value.AuthoritySections)),
	}
	for _, section := range value.AuthoritySections {
		records := make([]contract.CatalogAuthorityRecord, 0, len(section.Records))
		for _, record := range section.Records {
			records = append(records, authorityRecordResponse(record))
		}
		recordPage := contract.CatalogAuthorityRecordPageInfo{Limit: section.RecordsLimit, Total: section.RecordsTotal}
		if section.RecordsNextCursor != "" {
			recordPage.NextCursor = &section.RecordsNextCursor
		}
		result.AuthoritySections = append(result.AuthoritySections, contract.CatalogAuthoritySection{
			Kind: contract.CatalogAuthoritySectionKind(section.Kind), Authority: section.Authority,
			Availability: contract.CatalogAuthorityAvailability(section.Availability),
			RevisionId:   section.RevisionID, ReleaseId: section.ReleaseID,
			ReleaseSequence: section.ReleaseSequence, Values: section.Values, Records: records,
			RecordsPage: recordPage,
		})
	}
	if value.CurrentRevision != nil {
		revision := revisionResponse(*value.CurrentRevision)
		result.CurrentRevision = &revision
	}
	return result
}

func authorityRecordPageResponse(value catalogapp.AuthorityRecordPage) contract.CatalogAuthorityRecordPage {
	items := make([]contract.CatalogAuthorityRecord, 0, len(value.Items))
	for _, record := range value.Items {
		items = append(items, authorityRecordResponse(record))
	}
	page := contract.CatalogAuthorityRecordPageInfo{Limit: value.Limit, Total: value.Total}
	if value.NextCursor != "" {
		page.NextCursor = &value.NextCursor
	}
	return contract.CatalogAuthorityRecordPage{Items: items, Page: page}
}

func authorityRecordResponse(record domain.AuthorityRecord) contract.CatalogAuthorityRecord {
	result := contract.CatalogAuthorityRecord{
		Kind: contract.CatalogAuthorityRecordKind(record.Kind), Id: record.ID,
		Authority: record.Authority, Status: record.Status, Label: record.Label,
		RelatedId: optionalStringPointer(record.RelatedID), Version: record.Version,
		ReleaseId: record.ReleaseID, ReleaseSequence: record.ReleaseSequence,
	}
	if record.Relation != nil {
		value := record.Relation
		result.Relation = &contract.CatalogRelationAuthority{
			Direction: contract.CatalogRelationAuthorityDirection(value.Direction),
			Predicate: contract.RelationPredicate(value.Predicate), Plane: contract.RelationPlane(value.Plane),
			AssertionState: contract.RelationAssertionState(value.AssertionState),
			SubjectAssetId: value.SubjectAssetID, ObjectAssetId: value.ObjectAssetID,
		}
	}
	if record.PhysicalBinding != nil {
		value := record.PhysicalBinding
		result.PhysicalBinding = &contract.CatalogPhysicalBindingAuthority{
			AssetId: value.AssetID, DatasetId: value.DatasetID, FieldId: value.FieldID,
			Transform: optionalStringPointer(value.Transform),
		}
	}
	if record.ModelGrain != nil {
		value := record.ModelGrain
		result.ModelGrain = &contract.CatalogModelGrainAuthority{AssetId: value.AssetID,
			GrainExpression: value.GrainExpression, GrainFieldRefs: value.GrainFieldRefs,
			DocumentedBy: value.DocumentedBy}
	}
	if record.EntityKey != nil {
		value := record.EntityKey
		result.EntityKey = &contract.CatalogEntityKeyAuthority{AssetId: value.AssetID,
			KeyFieldRefs:        value.KeyFieldRefs,
			UniquenessSemantics: contract.CatalogEntityKeyAuthorityUniquenessSemantics(value.UniquenessSemantics)}
	}
	if record.JoinContract != nil {
		value := record.JoinContract
		result.JoinContract = &contract.CatalogJoinContractAuthority{
			Direction:     contract.CatalogJoinContractAuthorityDirection(value.Direction),
			LeftDatasetId: value.LeftDatasetID, RightDatasetId: value.RightDatasetID,
			LeftFieldRefs: value.LeftFieldRefs, RightFieldRefs: value.RightFieldRefs,
			JoinType:       contract.CatalogJoinContractAuthorityJoinType(value.JoinType),
			Cardinality:    contract.CatalogJoinContractAuthorityCardinality(value.Cardinality),
			JoinExpression: value.JoinExpression,
		}
	}
	if record.Lineage != nil {
		value := record.Lineage
		result.Lineage = &contract.CatalogLineageAuthority{
			Direction:         contract.CatalogLineageAuthorityDirection(value.Direction),
			UpstreamDatasetId: value.UpstreamDatasetID, DownstreamDatasetId: value.DownstreamDatasetID,
			EdgeKind:         contract.CatalogLineageAuthorityEdgeKind(value.EdgeKind),
			SourceRevisionId: value.SourceRevisionID, CodeArtifactId: value.CodeArtifactID,
			Confidence: value.Confidence,
		}
	}
	if record.ConsumerBinding != nil {
		value := record.ConsumerBinding
		constraint := make(map[string]any)
		_ = json.Unmarshal(value.CompatibilityConstraint, &constraint)
		result.ConsumerBinding = &contract.CatalogConsumerBindingAuthority{
			ConsumerId: value.ConsumerID, EffectiveReleaseId: value.EffectiveReleaseID,
			Environment: value.Environment, Purpose: value.Purpose,
			Mode:                    contract.CatalogConsumerBindingAuthorityMode(value.Mode),
			Status:                  contract.CatalogConsumerBindingAuthorityStatus(value.Status),
			CompatibilityConstraint: constraint, ExpiresAt: value.ExpiresAt,
		}
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
