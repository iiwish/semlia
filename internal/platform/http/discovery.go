package httpapi

import (
	"errors"
	"net/http"
	"strings"

	discoveryapp "github.com/iiwish/semlia/internal/application/discovery"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/pkg/identity"
)

func isDiscoveryControlRoute(kind routeKind) bool {
	switch kind {
	case routeSources, routeSource, routeSourceCredential, routeSourceTest, routeSourceRuns,
		routeSourceSnapshots, routeSourceSnapshot, routeSourceSnapshotMembers, routeSourceSnapshotDiagnostics,
		routeCandidates, routeCandidate, routeCandidateDecisions:
		return true
	default:
		return false
	}
}

func (handler *Handler) routeDiscoveryControl(response http.ResponseWriter, request *http.Request, traceID string, route matchedRoute) string {
	workspaceID, err := identity.ParseWorkspaceID(route.workspace)
	if err != nil {
		return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
	}
	if route.kind == routeSources {
		if request.Method == http.MethodGet {
			limit, limitErr := boundedQueryLimit(request, 50, 100)
			if limitErr != nil {
				return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
			}
			sourceFilter, filterErr := optionalSourceID(request.URL.Query().Get("sourceId"))
			if filterErr != nil {
				return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
			}
			page, listErr := handler.discovery.ListSources(request.Context(), discoveryapp.ListSourcesRequest{
				WorkspaceID: workspaceID, SourceID: sourceFilter, PrincipalRef: principalRef(request), TraceID: traceID,
				Cursor: request.URL.Query().Get("cursor"), Limit: limit})
			if listErr != nil {
				return writeDiscoveryError(response, listErr, traceID)
			}
			result := map[string]any{"items": sourceResponses(page.Items), "total": page.Total, "limit": page.Limit}
			if page.NextCursor != "" {
				result["nextCursor"] = page.NextCursor
			}
			writeJSON(response, http.StatusOK, result)
			return ""
		}
		var body sourceWriteRequest
		if decodeRequest(request, &body) != nil {
			return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
		}
		item, createErr := handler.discovery.CreateSource(request.Context(), discoveryapp.CreateSourceRequest{
			WorkspaceID: workspaceID, Name: body.Name, Host: body.Host, Port: body.Port, Database: body.Database,
			Username: body.Username, Password: body.Password, SSLMode: body.SSLMode,
			ArtifactPaths: body.ArtifactPaths, PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if createErr != nil {
			return writeDiscoveryError(response, createErr, traceID)
		}
		writeJSON(response, http.StatusCreated, sourceResponse(item))
		return ""
	}
	sourceID, sourceErr := identity.ParseSourceConnectionID(route.source)
	if route.kind >= routeSource && route.kind <= routeSourceRuns && sourceErr != nil {
		return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
	}
	base := discoveryapp.SourceRequest{WorkspaceID: workspaceID, SourceID: sourceID, PrincipalRef: principalRef(request), TraceID: traceID}
	if route.kind >= routeSourceSnapshots && route.kind <= routeSourceSnapshotDiagnostics {
		if sourceErr != nil {
			return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
		}
		limit, limitErr := boundedQueryLimit(request, 50, 200)
		if limitErr != nil {
			return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
		}
		for key, values := range request.URL.Query() {
			if len(values) != 1 || (key != "limit" && key != "cursor" && (key != "kind" || route.kind != routeSourceSnapshotMembers)) {
				return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
			}
		}
		query := discoveryapp.SnapshotRequest{SourceRequest: base, SnapshotID: route.snapshot, Kind: request.URL.Query().Get("kind"), Cursor: request.URL.Query().Get("cursor"), Limit: limit}
		var result any
		var readErr error
		switch route.kind {
		case routeSourceSnapshots:
			result, readErr = handler.discovery.ListSnapshots(request.Context(), query)
		case routeSourceSnapshot:
			result, readErr = handler.discovery.GetSnapshot(request.Context(), query)
		case routeSourceSnapshotMembers:
			result, readErr = handler.discovery.ListSnapshotMembers(request.Context(), query)
		case routeSourceSnapshotDiagnostics:
			result, readErr = handler.discovery.ListSnapshotDiagnostics(request.Context(), query)
		}
		if readErr != nil {
			return writeDiscoveryError(response, readErr, traceID)
		}
		writeJSON(response, http.StatusOK, result)
		return ""
	}
	switch route.kind {
	case routeSource:
		if request.Method == http.MethodGet {
			item, getErr := handler.discovery.GetSource(request.Context(), base)
			if getErr != nil {
				return writeDiscoveryError(response, getErr, traceID)
			}
			writeJSON(response, http.StatusOK, sourceResponse(item))
			return ""
		}
		if request.Method == http.MethodDelete {
			var body struct {
				ExpectedVersion int64 `json:"expectedVersion"`
			}
			if decodeRequest(request, &body) != nil {
				return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
			}
			current, getErr := handler.discovery.GetSource(request.Context(), base)
			if getErr != nil {
				return writeDiscoveryError(response, getErr, traceID)
			}
			item, deleteErr := handler.discovery.UpdateSource(request.Context(), discoveryapp.UpdateSourceRequest{WorkspaceID: workspaceID, SourceID: sourceID,
				Name: current.Name, ArtifactPaths: current.ArtifactPaths, Status: "deleted", PrincipalRef: principalRef(request), TraceID: traceID,
				ExpectedVersion: body.ExpectedVersion})
			if deleteErr != nil {
				return writeDiscoveryError(response, deleteErr, traceID)
			}
			writeJSON(response, http.StatusOK, sourceResponse(item))
			return ""
		}
		var body sourceUpdateRequest
		if decodeRequest(request, &body) != nil {
			return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
		}
		item, updateErr := handler.discovery.UpdateSource(request.Context(), discoveryapp.UpdateSourceRequest{WorkspaceID: workspaceID, SourceID: sourceID,
			Name: body.Name, ArtifactPaths: body.ArtifactPaths, Status: body.Status, PrincipalRef: principalRef(request), TraceID: traceID,
			ExpectedVersion: body.ExpectedVersion})
		if updateErr != nil {
			return writeDiscoveryError(response, updateErr, traceID)
		}
		writeJSON(response, http.StatusOK, sourceResponse(item))
		return ""
	case routeSourceCredential:
		var body struct {
			Password        string `json:"password"`
			ExpectedVersion int64  `json:"expectedVersion"`
		}
		if decodeRequest(request, &body) != nil {
			return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
		}
		item, rotateErr := handler.discovery.RotateCredential(request.Context(), discoveryapp.RotateCredentialRequest{SourceRequest: base, Password: body.Password, ExpectedVersion: body.ExpectedVersion})
		if rotateErr != nil {
			return writeDiscoveryError(response, rotateErr, traceID)
		}
		writeJSON(response, http.StatusOK, sourceResponse(item))
		return ""
	case routeSourceTest:
		if testErr := handler.discovery.TestSource(request.Context(), base); testErr != nil {
			return writeDiscoveryError(response, testErr, traceID)
		}
		writeJSON(response, http.StatusOK, map[string]any{"status": "succeeded"})
		return ""
	case routeSourceRuns:
		if request.Method == http.MethodGet {
			limit, limitErr := boundedQueryLimit(request, 50, 100)
			if limitErr != nil {
				return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
			}
			page, listErr := handler.discovery.ListRuns(request.Context(), discoveryapp.ListRunsRequest{SourceRequest: base,
				Cursor: request.URL.Query().Get("cursor"), Limit: limit})
			if listErr != nil {
				return writeDiscoveryError(response, listErr, traceID)
			}
			result := map[string]any{"items": runResponses(page.Items), "total": page.Total, "limit": page.Limit}
			if page.NextCursor != "" {
				result["nextCursor"] = page.NextCursor
			}
			writeJSON(response, http.StatusOK, result)
			return ""
		}
		var body struct {
			IdempotencyKey string `json:"idempotencyKey"`
		}
		if decodeRequest(request, &body) != nil {
			return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
		}
		item, startErr := handler.discovery.StartRun(request.Context(), discoveryapp.StartRunRequest{SourceRequest: base, IdempotencyKey: body.IdempotencyKey})
		if startErr != nil {
			return writeDiscoveryError(response, startErr, traceID)
		}
		writeJSON(response, http.StatusAccepted, runResponse(item))
		return ""
	case routeCandidates:
		limit, limitErr := boundedQueryLimit(request, 50, 100)
		if limitErr != nil {
			return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
		}
		sourceFilter, filterErr := optionalSourceID(request.URL.Query().Get("sourceId"))
		if filterErr != nil {
			return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
		}
		page, listErr := handler.discovery.ListCandidates(request.Context(), discoveryapp.ListCandidateRequest{
			WorkspaceID: workspaceID, SourceID: sourceFilter, Status: request.URL.Query().Get("status"), PrincipalRef: principalRef(request), TraceID: traceID,
			Cursor: request.URL.Query().Get("cursor"), Limit: limit})
		if listErr != nil {
			return writeDiscoveryError(response, listErr, traceID)
		}
		result := map[string]any{"items": candidateResponses(page.Items), "total": page.Total, "limit": page.Limit}
		if page.NextCursor != "" {
			result["nextCursor"] = page.NextCursor
		}
		writeJSON(response, http.StatusOK, result)
		return ""
	case routeCandidate, routeCandidateDecisions:
		candidateID, parseErr := identity.ParseSemanticCandidateID(route.candidate)
		if parseErr != nil {
			return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
		}
		if route.kind == routeCandidate {
			item, getErr := handler.discovery.GetCandidate(request.Context(), workspaceID, candidateID, principalRef(request), traceID)
			if getErr != nil {
				return writeDiscoveryError(response, getErr, traceID)
			}
			writeJSON(response, http.StatusOK, candidateResponse(item))
			return ""
		}
		var body candidateDecisionRequest
		if decodeRequest(request, &body) != nil {
			return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
		}
		var proposalID *identity.ProposalID
		if strings.TrimSpace(body.ProposalID) != "" {
			value, parseErr := identity.ParseProposalID(body.ProposalID)
			if parseErr != nil {
				return writeDiscoveryError(response, domain.ErrInvalidInput, traceID)
			}
			proposalID = &value
		}
		decision, decisionErr := handler.discovery.DecideCandidate(request.Context(), discoveryapp.DecideCandidateRequest{WorkspaceID: workspaceID, CandidateID: candidateID, Action: body.Action, ProposalID: proposalID, Reason: body.Reason, IdempotencyKey: body.IdempotencyKey, PrincipalRef: principalRef(request), TraceID: traceID})
		if decisionErr != nil {
			return writeDiscoveryError(response, decisionErr, traceID)
		}
		writeJSON(response, http.StatusCreated, decisionResponse(decision))
		return ""
	}
	panic("discovery control route is not handled")
}

type sourceWriteRequest struct {
	Name          string   `json:"name"`
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	Database      string   `json:"database"`
	Username      string   `json:"username"`
	Password      string   `json:"password"`
	SSLMode       string   `json:"sslMode"`
	ArtifactPaths []string `json:"artifactPaths"`
}
type sourceUpdateRequest struct {
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	ArtifactPaths   []string `json:"artifactPaths"`
	ExpectedVersion int64    `json:"expectedVersion"`
}
type candidateDecisionRequest struct {
	Action         string `json:"action"`
	ProposalID     string `json:"proposalId"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotencyKey"`
}

func sourceResponse(item domain.Source) map[string]any {
	adapters := map[string]string{"postgresql": "postgresql_catalog", "file": "file_catalog", "sql_bundle": "postgresql_sql", "dbt_bundle": "dbt"}
	result := map[string]any{"id": item.ID.String(), "name": item.Name, "sourceKind": item.Kind,
		"adapterKind": adapters[item.Kind], "status": item.Status, "version": item.Version,
		"createdAt": item.CreatedAt.UTC(), "updatedAt": item.UpdatedAt.UTC()}
	if item.Kind == "postgresql" {
		result["host"], result["port"], result["database"] = item.Host, item.Port, item.Database
		result["username"], result["sslMode"], result["credentialVersion"] = item.Username, item.SSLMode, item.ActiveCredentialVersion
		if len(item.ArtifactPaths) > 0 {
			result["artifactPaths"] = item.ArtifactPaths
		}
	} else if item.ActiveArtifactSetID != nil {
		result["activeArtifactSetId"] = item.ActiveArtifactSetID.String()
	}
	return result
}
func sourceResponses(items []domain.Source) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, sourceResponse(item))
	}
	return result
}
func runResponse(item domain.Run) map[string]any {
	result := map[string]any{"id": item.ID.String(), "sourceConnectionId": item.SourceConnectionID.String(),
		"status": item.Status, "stats": item.Stats, "operationsPath": "/operations/runtime?run=" + item.ID.String(),
		"createdAt": item.CreatedAt.UTC(), "updatedAt": item.UpdatedAt.UTC()}
	result["snapshotId"] = item.SnapshotID
	if item.SourceInputFingerprint != "" {
		result["sourceInputFingerprint"] = item.SourceInputFingerprint
	}
	if item.CredentialVersion > 0 {
		result["credentialVersion"] = item.CredentialVersion
	}
	if item.ArtifactSetID != nil {
		result["artifactSetId"] = item.ArtifactSetID.String()
	}
	if item.ErrorCode != "" {
		result["errorCode"] = item.ErrorCode
	}
	if item.StartedAt != nil {
		result["startedAt"] = item.StartedAt.UTC()
	}
	if item.CompletedAt != nil {
		result["completedAt"] = item.CompletedAt.UTC()
	}
	return result
}
func runResponses(items []domain.Run) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, runResponse(item))
	}
	return result
}
func candidateResponse(item domain.Candidate) map[string]any {
	result := map[string]any{"id": item.ID.String(), "sourceConnectionId": item.SourceConnectionID.String(), "sourceRevisionId": item.SourceRevisionID.String(), "discoveryRunId": item.DiscoveryRunID.String(), "candidateKey": item.Key, "candidateKind": item.Kind, "title": item.Title, "proposalInput": item.ProposalInput, "evidence": item.Evidence, "contentDigest": item.ContentDigest, "status": item.Status, "createdAt": item.CreatedAt.UTC(), "updatedAt": item.UpdatedAt.UTC()}
	if item.ProposalID != nil {
		result["proposalId"] = item.ProposalID.String()
	}
	return result
}
func candidateResponses(items []domain.Candidate) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, candidateResponse(item))
	}
	return result
}
func decisionResponse(item domain.CandidateDecision) map[string]any {
	result := map[string]any{"id": item.ID.String(), "candidateId": item.CandidateID.String(), "action": item.Action, "actor": item.Actor, "reason": item.Reason, "idempotencyKey": item.IdempotencyKey, "createdAt": item.CreatedAt.UTC()}
	if item.ProposalID != nil {
		result["proposalId"] = item.ProposalID.String()
	}
	return result
}

func writeDiscoveryError(response http.ResponseWriter, err error, traceID string) string {
	var denial *authorization.DenialError
	switch {
	case errors.As(err, &denial):
		writeError(response, http.StatusForbidden, string(denial.Decision.ReasonCode), "the acting principal lacks the required capability", traceID, false)
		return string(denial.Decision.ReasonCode)
	case errors.Is(err, discoveryapp.ErrSnapshotCursor):
		writeError(response, http.StatusBadRequest, "INVALID_CURSOR", "the snapshot cursor is invalid", traceID, false)
		return "INVALID_CURSOR"
	case errors.Is(err, domain.ErrForbidden):
		writeError(response, http.StatusForbidden, "FORBIDDEN", "the acting principal lacks the required capability", traceID, false)
		return "FORBIDDEN"
	case errors.Is(err, domain.ErrInvalidInput), errors.Is(err, domain.ErrArtifactPath):
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "the request is invalid", traceID, false)
		return "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrNotFound):
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the requested resource was not found", traceID, false)
		return "NOT_FOUND"
	case errors.Is(err, domain.ErrConflict):
		writeError(response, http.StatusConflict, "CONFLICT", "the request conflicts with current state", traceID, false)
		return "CONFLICT"
	case errors.Is(err, domain.ErrUnsafeSource):
		writeError(response, http.StatusUnprocessableEntity, "SOURCE_ROLE_UNSAFE", "the PostgreSQL source role is not read-only", traceID, false)
		return "SOURCE_ROLE_UNSAFE"
	case errors.Is(err, domain.ErrCredential):
		writeError(response, http.StatusUnprocessableEntity, "CREDENTIAL_UNAVAILABLE", "the source credential cannot be used", traceID, false)
		return "CREDENTIAL_UNAVAILABLE"
	case errors.Is(err, domain.ErrConnectionTest):
		writeError(response, http.StatusServiceUnavailable, "SOURCE_CONNECTION_FAILED", "the source connection could not be verified", traceID, true)
		return "SOURCE_CONNECTION_FAILED"
	default:
		writeError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "the request could not be completed", traceID, false)
		return "INTERNAL_ERROR"
	}
}
