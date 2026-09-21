package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	ingestionapp "github.com/iiwish/semlia/internal/application/ingestion"
	"github.com/iiwish/semlia/internal/domain/authorization"
	ingestiondomain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
)

const headerIdempotencyKey = "Idempotency-Key"

func isIngestionRoute(kind routeKind) bool {
	return kind >= routeIngestionArtifacts && kind <= routeScheduleOccurrences
}

func (handler *Handler) routeIngestion(response http.ResponseWriter, request *http.Request, traceID string, route matchedRoute) string {
	workspaceID, err := identity.ParseWorkspaceID(route.workspace)
	if err != nil {
		return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
	}
	switch route.kind {
	case routeIngestionArtifactPreview:
		setID, parseErr := identity.ParseArtifactSetID(route.artifactSet)
		artifactID, artifactErr := identity.ParseArtifactID(route.artifact)
		if parseErr != nil || artifactErr != nil {
			return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
		}
		preview, getErr := handler.ingestion.Preview(request.Context(), workspaceID, setID, artifactID, principalRef(request), traceID)
		if getErr != nil {
			return writeIngestionError(response, getErr, traceID)
		}
		response.Header().Set("Cache-Control", "no-store")
		writeJSON(response, http.StatusOK, preview)
		return ""
	case routeIngestionArtifactSet:
		setID, parseErr := identity.ParseArtifactSetID(route.artifactSet)
		if parseErr != nil {
			return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
		}
		set, getErr := handler.ingestion.GetSet(request.Context(), workspaceID, setID, principalRef(request), traceID)
		if getErr != nil {
			return writeIngestionError(response, getErr, traceID)
		}
		writeJSON(response, http.StatusOK, artifactSetResponse(set))
		return ""
	case routeIngestionArtifacts:
		if request.Method == http.MethodGet {
			limit, parseErr := boundedQueryLimit(request, 50, 100)
			if parseErr != nil {
				return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
			}
			sourceID, sourceErr := optionalSourceID(request.URL.Query().Get("sourceId"))
			if sourceErr != nil {
				return writeIngestionError(response, sourceErr, traceID)
			}
			page, listErr := handler.ingestion.List(request.Context(), ingestionapp.ListArtifactsQuery{
				WorkspaceID: workspaceID, Kind: ingestiondomain.ArtifactKind(request.URL.Query().Get("kind")),
				Status: ingestiondomain.ArtifactStatus(request.URL.Query().Get("status")), SourceID: sourceID,
				Cursor: request.URL.Query().Get("cursor"), Limit: limit,
			}, principalRef(request), traceID)
			if listErr != nil {
				return writeIngestionError(response, listErr, traceID)
			}
			writeJSON(response, http.StatusOK, artifactPageResponse(page))
			return ""
		}
		kind := ingestiondomain.ArtifactKind(strings.TrimSpace(request.Header.Get("X-Artifact-Kind")))
		if kind == ingestiondomain.ArtifactSQL || (kind != ingestiondomain.ArtifactCSV && kind != ingestiondomain.ArtifactXLSX &&
			kind != ingestiondomain.ArtifactMarkdown && kind != ingestiondomain.ArtifactDBTManifest && kind != ingestiondomain.ArtifactDBTCatalog) {
			return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
		}
		if request.ContentLength < 1 || request.ContentLength > ingestiondomain.MaxUploadBytes {
			return writeIngestionError(response, ingestiondomain.ErrLimitExceeded, traceID)
		}
		sourceID, sourceErr := optionalSourceID(request.Header.Get("X-Source-Id"))
		if sourceErr != nil {
			return writeIngestionError(response, sourceErr, traceID)
		}
		expectedSourceVersion, versionErr := optionalExpectedSourceVersion(request.Header.Get("X-Expected-Source-Version"))
		if versionErr != nil || (sourceID == nil) != (expectedSourceVersion == nil) {
			return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
		}
		request.Body = http.MaxBytesReader(response, request.Body, ingestiondomain.MaxUploadBytes+1)
		artifact, stageErr := handler.ingestion.Stage(request.Context(), ingestionapp.StageRequest{
			WorkspaceID: workspaceID, SourceID: sourceID, ExpectedSourceVersion: expectedSourceVersion,
			Kind: kind, OriginalName: request.Header.Get("X-File-Name"),
			MediaType: request.Header.Get("Content-Type"), Content: request.Body, ContentLength: request.ContentLength,
			IdempotencyKey: request.Header.Get(headerIdempotencyKey), PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if stageErr != nil {
			return writeIngestionError(response, stageErr, traceID)
		}
		writeJSON(response, http.StatusCreated, artifactResponse(artifact))
		return ""
	case routeIngestionFinalize:
		var body finalizeArtifactSetRequest
		if decodeRequest(request, &body) != nil {
			return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
		}
		artifactIDs, parseErr := parseArtifactIDs(body.ArtifactIDs)
		if parseErr != nil {
			return writeIngestionError(response, parseErr, traceID)
		}
		sourceID, parseErr := optionalSourceID(body.SourceID)
		if parseErr != nil {
			return writeIngestionError(response, parseErr, traceID)
		}
		set, finalizeErr := handler.ingestion.Finalize(request.Context(), ingestionapp.FinalizeRequest{
			WorkspaceID: workspaceID, SourceName: body.SourceName, ArtifactIDs: artifactIDs,
			IdempotencyKey: request.Header.Get(headerIdempotencyKey), PrincipalRef: principalRef(request), TraceID: traceID,
			SourceID: sourceID, ExpectedSourceVersion: body.ExpectedSourceVersion,
		})
		if finalizeErr != nil {
			return writeIngestionError(response, finalizeErr, traceID)
		}
		writeJSON(response, http.StatusCreated, artifactSetResponse(set))
		return ""
	case routeIngestionSQLRegistrations:
		var body sqlRegistrationRequest
		if decodeRequest(request, &body) != nil {
			return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
		}
		sourceID, parseErr := optionalSourceID(body.SourceID)
		if parseErr != nil {
			return writeIngestionError(response, parseErr, traceID)
		}
		set, registerErr := handler.ingestion.RegisterSQL(request.Context(), ingestionapp.RegisterSQLRequest{
			WorkspaceID: workspaceID, SourceName: body.SourceName, Paths: body.Paths,
			IdempotencyKey: request.Header.Get(headerIdempotencyKey), PrincipalRef: principalRef(request), TraceID: traceID,
			SourceID: sourceID, ExpectedSourceVersion: body.ExpectedSourceVersion,
		})
		if registerErr != nil {
			return writeIngestionError(response, registerErr, traceID)
		}
		writeJSON(response, http.StatusCreated, artifactSetResponse(set))
		return ""
	case routeSourceSchedules:
		sourceID, parseErr := identity.ParseSourceConnectionID(route.source)
		if parseErr != nil {
			return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
		}
		if request.Method == http.MethodGet {
			limit, limitErr := boundedQueryLimit(request, 50, 100)
			if limitErr != nil {
				return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
			}
			page, listErr := handler.schedules.List(request.Context(), ingestionapp.ListSchedulesQuery{
				WorkspaceID: workspaceID, SourceID: &sourceID, Cursor: request.URL.Query().Get("cursor"), Limit: limit,
			}, principalRef(request), traceID)
			if listErr != nil {
				return writeIngestionError(response, listErr, traceID)
			}
			writeJSON(response, http.StatusOK, schedulePageResponse(page))
			return ""
		}
		var body createScheduleRequest
		if decodeRequest(request, &body) != nil {
			return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
		}
		value, createErr := handler.schedules.Create(request.Context(), ingestionapp.CreateScheduleRequest{
			WorkspaceID: workspaceID, SourceID: sourceID, Expression: body.Expression, Timezone: body.Timezone,
			MisfirePolicy: ingestiondomain.MisfirePolicy(body.MisfirePolicy), IdempotencyKey: request.Header.Get(headerIdempotencyKey),
			PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if createErr != nil {
			return writeIngestionError(response, createErr, traceID)
		}
		writeJSON(response, http.StatusCreated, scheduleResponse(value))
		return ""
	}

	scheduleID, err := identity.ParseSourceScheduleID(route.schedule)
	if err != nil {
		return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
	}
	command := func(expected int64) ingestionapp.ScheduleCommandRequest {
		return ingestionapp.ScheduleCommandRequest{WorkspaceID: workspaceID, ScheduleID: scheduleID, ExpectedVersion: expected,
			IdempotencyKey: request.Header.Get(headerIdempotencyKey), PrincipalRef: principalRef(request), TraceID: traceID}
	}
	switch route.kind {
	case routeSchedule:
		if request.Method == http.MethodGet {
			value, getErr := handler.schedules.Get(request.Context(), workspaceID, scheduleID, principalRef(request), traceID)
			if getErr != nil {
				return writeIngestionError(response, getErr, traceID)
			}
			writeJSON(response, http.StatusOK, scheduleResponse(value))
			return ""
		}
		var body updateScheduleRequest
		if decodeRequest(request, &body) != nil {
			return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
		}
		if request.Method == http.MethodDelete {
			value, deleteErr := handler.schedules.Delete(request.Context(), command(body.ExpectedVersion))
			if deleteErr != nil {
				return writeIngestionError(response, deleteErr, traceID)
			}
			writeJSON(response, http.StatusOK, scheduleResponse(value))
			return ""
		}
		value, updateErr := handler.schedules.Update(request.Context(), ingestionapp.UpdateScheduleRequest{
			WorkspaceID: workspaceID, ScheduleID: scheduleID, Expression: body.Expression, Timezone: body.Timezone,
			MisfirePolicy: ingestiondomain.MisfirePolicy(body.MisfirePolicy), Enabled: body.Enabled,
			ExpectedVersion: body.ExpectedVersion, IdempotencyKey: request.Header.Get(headerIdempotencyKey),
			PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if updateErr != nil {
			return writeIngestionError(response, updateErr, traceID)
		}
		writeJSON(response, http.StatusOK, scheduleResponse(value))
		return ""
	case routeSchedulePause, routeScheduleResume, routeScheduleRunNow:
		var body scheduleCommandRequest
		if decodeRequest(request, &body) != nil {
			return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
		}
		if route.kind == routeScheduleRunNow {
			occurrence, runErr := handler.schedules.RunNow(request.Context(), command(body.ExpectedVersion))
			if runErr != nil {
				return writeIngestionError(response, runErr, traceID)
			}
			writeJSON(response, http.StatusAccepted, occurrenceResponse(occurrence))
			return ""
		}
		value, mutateErr := handler.schedules.SetEnabled(request.Context(), command(body.ExpectedVersion), route.kind == routeScheduleResume)
		if mutateErr != nil {
			return writeIngestionError(response, mutateErr, traceID)
		}
		writeJSON(response, http.StatusOK, scheduleResponse(value))
		return ""
	case routeScheduleOccurrences:
		limit, limitErr := boundedQueryLimit(request, 50, 100)
		if limitErr != nil {
			return writeIngestionError(response, ingestiondomain.ErrInvalid, traceID)
		}
		page, listErr := handler.schedules.ListOccurrences(request.Context(), ingestionapp.ListOccurrencesQuery{
			WorkspaceID: workspaceID, ScheduleID: scheduleID, Cursor: request.URL.Query().Get("cursor"), Limit: limit,
		}, principalRef(request), traceID)
		if listErr != nil {
			return writeIngestionError(response, listErr, traceID)
		}
		writeJSON(response, http.StatusOK, occurrencePageResponse(page))
		return ""
	}
	panic("ingestion route is not handled")
}

type finalizeArtifactSetRequest struct {
	SourceName            string   `json:"sourceName"`
	SourceID              string   `json:"sourceId"`
	ExpectedSourceVersion *int64   `json:"expectedSourceVersion"`
	ArtifactIDs           []string `json:"artifactIds"`
}
type sqlRegistrationRequest struct {
	SourceName            string   `json:"sourceName"`
	SourceID              string   `json:"sourceId"`
	ExpectedSourceVersion *int64   `json:"expectedSourceVersion"`
	Paths                 []string `json:"paths"`
}
type createScheduleRequest struct {
	Expression    string `json:"expression"`
	Timezone      string `json:"timezone"`
	MisfirePolicy string `json:"misfirePolicy"`
}
type updateScheduleRequest struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Expression      string `json:"expression"`
	Timezone        string `json:"timezone"`
	MisfirePolicy   string `json:"misfirePolicy"`
	Enabled         bool   `json:"enabled"`
}
type scheduleCommandRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

func optionalExpectedSourceVersion(value string) (*int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 1 {
		return nil, ingestiondomain.ErrInvalid
	}
	return &parsed, nil
}

func artifactResponse(value ingestiondomain.Artifact) map[string]any {
	result := map[string]any{"id": value.ID.String(), "kind": value.Kind, "schemaVersion": value.SchemaVersion,
		"byteSize": value.ByteSize, "mediaType": value.MediaType, "originalName": value.OriginalName,
		"status": value.Status, "contentAvailability": value.ContentAvailability, "createdAt": value.CreatedAt.UTC()}
	if value.ContentDigest != "" {
		result["contentDigest"] = value.ContentDigest
	}
	if value.SourceConnectionID != nil {
		result["sourceId"] = value.SourceConnectionID.String()
	}
	if value.FailureCode != "" {
		result["failureCode"] = value.FailureCode
	}
	if value.FinalizedAt != nil {
		result["finalizedAt"] = value.FinalizedAt.UTC()
	}
	if value.ExpiresAt != nil {
		result["expiresAt"] = value.ExpiresAt.UTC()
	}
	if value.ValidationSummary != nil {
		result["validationSummary"] = value.ValidationSummary
	}
	return result
}

func artifactPageResponse(page ingestiondomain.Page[ingestiondomain.Artifact]) map[string]any {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, artifactResponse(item))
	}
	result := map[string]any{"items": items, "total": page.Total, "limit": page.Limit}
	if page.NextCursor != "" {
		result["nextCursor"] = page.NextCursor
	}
	return result
}

func artifactSetResponse(value ingestiondomain.ArtifactSet) map[string]any {
	members := make([]map[string]any, 0, len(value.Members))
	for _, member := range value.Members {
		members = append(members, map[string]any{"artifactId": member.ArtifactID.String(), "logicalPath": member.LogicalPath,
			"ordinal": member.Ordinal, "contentDigest": member.ContentDigest, "byteSize": member.ByteSize, "mediaType": member.MediaType,
			"kind": member.Kind, "contentAvailability": member.ContentAvailability})
	}
	return map[string]any{"id": value.ID.String(), "sourceId": value.SourceConnectionID.String(), "sourceKind": value.SourceKind,
		"setDigest": value.SetDigest, "members": members, "createdAt": value.CreatedAt.UTC()}
}

func scheduleResponse(value ingestiondomain.Schedule) map[string]any {
	result := map[string]any{"id": value.ID.String(), "sourceId": value.SourceConnectionID.String(), "expression": value.Expression,
		"timezone": value.Timezone, "misfirePolicy": value.MisfirePolicy, "enabled": value.Enabled, "version": value.Version,
		"createdAt": value.CreatedAt.UTC(), "updatedAt": value.UpdatedAt.UTC()}
	if value.NextRunAt != nil {
		result["nextRunAt"] = value.NextRunAt.UTC()
	}
	if value.NextWallClockKey != "" {
		result["nextWallClockKey"] = value.NextWallClockKey
	}
	if value.LastRunAt != nil {
		result["lastRunAt"] = value.LastRunAt.UTC()
	}
	if value.CredentialVersion != nil {
		result["credentialVersion"] = *value.CredentialVersion
	}
	if value.DeletedAt != nil {
		result["deletedAt"] = value.DeletedAt.UTC()
	}
	return result
}

func schedulePageResponse(page ingestiondomain.Page[ingestiondomain.Schedule]) map[string]any {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, scheduleResponse(item))
	}
	result := map[string]any{"items": items, "total": page.Total, "limit": page.Limit}
	if page.NextCursor != "" {
		result["nextCursor"] = page.NextCursor
	}
	return result
}

func occurrenceResponse(value ingestiondomain.Occurrence) map[string]any {
	result := map[string]any{"id": value.ID.String(), "scheduleId": value.ScheduleID.String(), "sourceId": value.SourceConnectionID.String(),
		"triggerKind": value.TriggerKind, "scheduleVersion": value.ScheduleVersion, "eligibleAt": value.EligibleAt.UTC(),
		"wallClockKey": value.WallClockKey, "state": value.State, "idempotencyKey": value.IdempotencyKey, "createdAt": value.CreatedAt.UTC()}
	if value.ScheduledFor != nil {
		result["scheduledFor"] = value.ScheduledFor.UTC()
	}
	if value.ReasonCode != "" {
		result["reasonCode"] = value.ReasonCode
	}
	if value.MisfireDisposition != "" {
		result["misfireDisposition"] = value.MisfireDisposition
	}
	if value.DiscoveryRunID != nil {
		result["discoveryRunId"] = value.DiscoveryRunID.String()
	}
	if value.JobID != nil {
		result["jobId"] = value.JobID.String()
	}
	if value.RuntimeRunID != nil {
		result["runtimeRunId"] = value.RuntimeRunID.String()
		result["operationsPath"] = "/operations/runtime?run=" + value.RuntimeRunID.String()
	}
	if value.ArtifactSetID != nil {
		result["artifactSetId"] = value.ArtifactSetID.String()
	}
	if value.CredentialVersion != nil {
		result["credentialVersion"] = *value.CredentialVersion
	}
	if value.SourceFingerprint != "" {
		result["sourceFingerprint"] = value.SourceFingerprint
	}
	return result
}

func occurrencePageResponse(page ingestiondomain.Page[ingestiondomain.Occurrence]) map[string]any {
	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, occurrenceResponse(item))
	}
	result := map[string]any{"items": items, "total": page.Total, "limit": page.Limit}
	if page.NextCursor != "" {
		result["nextCursor"] = page.NextCursor
	}
	return result
}

func parseArtifactIDs(values []string) ([]identity.ArtifactID, error) {
	result := make([]identity.ArtifactID, 0, len(values))
	for _, value := range values {
		id, err := identity.ParseArtifactID(value)
		if err != nil {
			return nil, ingestiondomain.ErrInvalid
		}
		result = append(result, id)
	}
	return result, nil
}

func optionalSourceID(value string) (*identity.SourceConnectionID, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	id, err := identity.ParseSourceConnectionID(value)
	if err != nil {
		return nil, ingestiondomain.ErrInvalid
	}
	return &id, nil
}

func boundedQueryLimit(request *http.Request, fallback, maximum int) (int, error) {
	raw := strings.TrimSpace(request.URL.Query().Get("limit"))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > maximum {
		return 0, ingestiondomain.ErrInvalid
	}
	return value, nil
}

func writeIngestionError(response http.ResponseWriter, err error, traceID string) string {
	var denial *authorization.DenialError
	switch {
	case errors.As(err, &denial):
		writeError(response, http.StatusForbidden, string(denial.Decision.ReasonCode), "the acting principal lacks the required capability", traceID, false)
		return string(denial.Decision.ReasonCode)
	case errors.Is(err, ingestiondomain.ErrInvalid):
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "the request is invalid", traceID, false)
		return "INVALID_ARGUMENT"
	case errors.Is(err, ingestiondomain.ErrUnsafeContent), errors.Is(err, ingestiondomain.ErrUnsupported):
		writeError(response, http.StatusUnprocessableEntity, "ARTIFACT_REJECTED", "the artifact was rejected", traceID, false)
		return "ARTIFACT_REJECTED"
	case errors.Is(err, ingestiondomain.ErrLimitExceeded):
		writeError(response, http.StatusRequestEntityTooLarge, "ARTIFACT_LIMIT_EXCEEDED", "the artifact exceeds an ingestion limit", traceID, false)
		return "ARTIFACT_LIMIT_EXCEEDED"
	case errors.Is(err, ingestiondomain.ErrNotFound):
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the ingestion resource was not found", traceID, false)
		return "NOT_FOUND"
	case errors.Is(err, ingestiondomain.ErrContentUnavailable):
		writeError(response, http.StatusGone, "CONTENT_UNAVAILABLE", "artifact content has expired or was not retained", traceID, false)
		return "CONTENT_UNAVAILABLE"
	case errors.Is(err, ingestiondomain.ErrConflict):
		writeError(response, http.StatusConflict, "CONFLICT", "the ingestion resource changed or conflicts", traceID, false)
		return "CONFLICT"
	case errors.Is(err, ingestiondomain.ErrStore):
		writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "artifact storage is unavailable", traceID, true)
		return "DEPENDENCY_UNAVAILABLE"
	default:
		writeError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "the request could not be completed", traceID, false)
		return "INTERNAL_ERROR"
	}
}
