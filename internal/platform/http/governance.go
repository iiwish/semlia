package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	contract "github.com/iiwish/semlia/api/gen/go"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

const (
	routeGovernanceProposals routeKind = iota + 100
	routeGovernanceProposal
	routeGovernanceProposalSubmit
)

func isGovernanceRoute(kind routeKind) bool {
	switch kind {
	case routeGovernanceProposals, routeGovernanceProposal, routeGovernanceProposalSubmit:
		return true
	}
	return false
}

func matchGovernanceRoute(parts []string, base matchedRoute) (matchedRoute, bool) {
	if len(parts) < 6 || parts[4] != "governance" || parts[5] != "proposals" {
		return matchedRoute{}, false
	}
	if len(parts) == 6 {
		base.kind, base.label = routeGovernanceProposals, "/api/v1/workspaces/{workspaceId}/governance/proposals"
		return base, true
	}
	base.proposal = parts[6]
	switch {
	case len(parts) == 7:
		base.kind, base.label = routeGovernanceProposal, "/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}"
	case len(parts) == 8 && parts[7] == "submit":
		base.kind, base.label = routeGovernanceProposalSubmit, "/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}/submit"
	default:
		return matchedRoute{}, false
	}
	return base, true
}

func governanceRouteMethods(kind routeKind) []string {
	switch kind {
	case routeGovernanceProposals:
		return []string{http.MethodGet, http.MethodPost}
	case routeGovernanceProposalSubmit:
		return []string{http.MethodPost}
	default:
		return []string{http.MethodGet}
	}
}

func (handler *Handler) routeGovernance(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	route matchedRoute,
) string {
	workspaceID, err := identity.ParseWorkspaceID(route.workspace)
	if err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	switch route.kind {
	case routeGovernanceProposals:
		if request.Method == http.MethodPost {
			return handler.createGovernanceProposal(response, request, traceID, workspaceID)
		}
		return handler.listGovernanceProposals(response, request, traceID, workspaceID)
	case routeGovernanceProposal:
		proposalID, parseErr := identity.ParseProposalID(route.proposal)
		if parseErr != nil {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		detail, getErr := handler.governance.GetProposal(request.Context(), governanceapp.GetAuthoringProposalRequest{
			WorkspaceID: workspaceID, ProposalID: proposalID,
			PrincipalRef: strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
		})
		if getErr != nil {
			return writeGovernanceError(response, getErr, traceID)
		}
		writeJSON(response, http.StatusOK, governanceProposalDetailResponse(detail))
		return ""
	case routeGovernanceProposalSubmit:
		proposalID, parseErr := identity.ParseProposalID(route.proposal)
		if parseErr != nil {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		detail, submitErr := handler.governance.SubmitProposal(request.Context(), governanceapp.SubmitAuthoringProposalRequest{
			WorkspaceID: workspaceID, ProposalID: proposalID,
			PrincipalRef: strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
		})
		if submitErr != nil {
			return writeGovernanceError(response, submitErr, traceID)
		}
		writeJSON(response, http.StatusOK, governanceProposalDetailResponse(detail))
		return ""
	default:
		panic("governance route is not handled")
	}
}

func (handler *Handler) listGovernanceProposals(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) string {
	limit, err := queryInteger(request, "limit")
	if err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	page, err := handler.governance.ListProposals(request.Context(), governanceapp.ListAuthoringProposalsRequest{
		WorkspaceID: workspaceID, Limit: limit, Cursor: request.URL.Query().Get("cursor"),
		PrincipalRef: strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	items := make([]contract.GovernanceProposalSummary, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, governanceProposalSummaryResponse(item))
	}
	result := contract.GovernanceProposalPage{Items: items, Page: contract.PageInfo{Limit: page.Limit}}
	if page.NextCursor != "" {
		result.Page.NextCursor = &page.NextCursor
	}
	writeJSON(response, http.StatusOK, result)
	return ""
}

func (handler *Handler) createGovernanceProposal(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) string {
	raw, err := readBody(request)
	if err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	// SSOT §8.6: agent structured output is schema-validated before anything
	// else touches it — no authorization evaluation, no domain call, no write.
	if governanceapp.AgentPayloadHasAttribution(raw) {
		if err := governanceapp.ValidateAgentProposalInput(raw); err != nil {
			return writeGovernanceError(response, err, traceID)
		}
	}
	var body contract.CreateGovernanceProposalRequest
	if err := decodeStrict(raw, &body); err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	changeSet, err := governanceChangeSetItems(body.ChangeSet)
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	createRequest := governanceapp.CreateAuthoringProposalRequest{
		WorkspaceID:    workspaceID,
		TargetType:     domain.TargetObjectType(body.TargetObjectType),
		TargetObjectID: string(body.TargetObjectId),
		Title:          body.Title,
		Summary:        dereferenceString(body.Summary),
		Reason:         dereferenceString(body.Reason),
		ChangeSet:      changeSet,
		CreatedBy:      dereferenceString(body.CreatedBy),
		PrincipalRef:   strings.TrimSpace(request.Header.Get(headerPrincipal)),
		TraceID:        traceID,
	}
	if body.BaseRevisionId != nil {
		createRequest.BaseRevisionID = *body.BaseRevisionId
	}
	if body.AgentAttribution != nil {
		createRequest.AgentAttribution = &governanceapp.AgentAttribution{
			AgentRunID:     body.AgentAttribution.AgentRunId,
			Model:          body.AgentAttribution.Model,
			ConfigRevision: body.AgentAttribution.ConfigRevision,
			InputHash:      body.AgentAttribution.InputHash,
		}
	}
	detail, err := handler.governance.CreateProposal(request.Context(), createRequest)
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	writeJSON(response, http.StatusCreated, governanceProposalDetailResponse(detail))
	return ""
}

func readBody(request *http.Request) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(request.Body, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, errors.New("request body exceeds the size limit")
	}
	return raw, nil
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
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

func governanceChangeSetItems(items []contract.GovernanceChangeSetItem) ([]governanceapp.ChangeSetItemInput, error) {
	inputs := make([]governanceapp.ChangeSetItemInput, 0, len(items))
	for _, item := range items {
		input := governanceapp.ChangeSetItemInput{
			FieldPath: item.FieldPath,
			Op:        domain.ChangeOp(item.Op),
		}
		if item.BeforeDigest != nil {
			input.BeforeDigest = string(*item.BeforeDigest)
		}
		if item.AfterDigest != nil {
			input.AfterDigest = string(*item.AfterDigest)
		}
		if item.BeforeValue != nil {
			input.BeforeValue = append([]byte(nil), *item.BeforeValue...)
		}
		if item.AfterValue != nil {
			input.AfterValue = append([]byte(nil), *item.AfterValue...)
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func governanceProposalSummaryResponse(value domain.Proposal) contract.GovernanceProposalSummary {
	result := contract.GovernanceProposalSummary{
		Id:               value.ID,
		TargetObjectType: contract.GovernanceTargetObjectType(value.TargetObjectType),
		State:            contract.GovernanceProposalState(value.State),
		Title:            value.Title,
		Summary:          value.Summary,
		Reason:           value.Reason,
		CreatedBy:        value.CreatedBy,
		CreatedAt:        value.CreatedAt.UTC(),
		UpdatedAt:        value.UpdatedAt.UTC(),
	}
	if targetID, err := governanceapp.ProposalTargetObjectTypeID(value); err == nil {
		result.TargetObjectId = targetID
	}
	if value.BaseRevisionID != nil {
		result.BaseRevisionId = value.BaseRevisionID
	}
	if value.AgentRunID != nil {
		result.AgentRunId = value.AgentRunID
	}
	if value.AssetID != nil {
		result.AssetId = value.AssetID
	}
	if value.SubmittedAt != nil {
		submittedAt := *value.SubmittedAt
		result.SubmittedAt = &submittedAt
	}
	if value.DecidedAt != nil {
		decidedAt := *value.DecidedAt
		result.DecidedAt = &decidedAt
	}
	return result
}

func governanceProposalDetailResponse(value governanceapp.ProposalDetail) contract.GovernanceProposalDetail {
	summary := governanceProposalSummaryResponse(value.Proposal)
	result := contract.GovernanceProposalDetail{
		Id: summary.Id, TargetObjectType: summary.TargetObjectType,
		TargetObjectId: summary.TargetObjectId, AssetId: summary.AssetId,
		BaseRevisionId: summary.BaseRevisionId, State: summary.State,
		Title: summary.Title, Summary: summary.Summary, Reason: summary.Reason,
		AgentRunId: summary.AgentRunId, CreatedBy: summary.CreatedBy,
		SubmittedAt: summary.SubmittedAt, DecidedAt: summary.DecidedAt,
		CreatedAt: summary.CreatedAt, UpdatedAt: summary.UpdatedAt,
		ChangeSet: make([]contract.GovernanceChangeSetItemRecord, 0, len(value.Changes)),
	}
	for _, item := range value.Changes {
		result.ChangeSet = append(result.ChangeSet, governanceChangeItemResponse(item))
	}
	return result
}

func governanceChangeItemResponse(item domain.ChangeSetItem) contract.GovernanceChangeSetItemRecord {
	record := contract.GovernanceChangeSetItemRecord{
		Id: item.ID, FieldPath: item.FieldPath,
		Op: contract.GovernanceChangeOp(item.Op), CreatedAt: item.CreatedAt.UTC(),
	}
	if item.BeforeDigest != "" {
		beforeDigest := contract.GovernanceChangeDigest(item.BeforeDigest)
		record.BeforeDigest = &beforeDigest
	}
	if item.AfterDigest != "" {
		afterDigest := contract.GovernanceChangeDigest(item.AfterDigest)
		record.AfterDigest = &afterDigest
	}
	if item.BeforeValue != nil {
		beforeValue := json.RawMessage(append([]byte(nil), item.BeforeValue...))
		record.BeforeValue = &beforeValue
	}
	if item.AfterValue != nil {
		afterValue := json.RawMessage(append([]byte(nil), item.AfterValue...))
		record.AfterValue = &afterValue
	}
	return record
}

func writeGovernanceError(response http.ResponseWriter, err error, traceID string) string {
	var denial *authz.DenialError
	var aiOutputInvalid *governanceapp.AIOutputInvalidError
	switch {
	case errors.As(err, &denial):
		writeError(response, http.StatusForbidden, string(denial.Decision.ReasonCode),
			"the acting principal lacks the required capability", traceID, false)
		return string(denial.Decision.ReasonCode)
	case errors.As(err, &aiOutputInvalid):
		writeErrorWithDetails(response, http.StatusUnprocessableEntity, "AI_OUTPUT_INVALID",
			"the agent structured output does not match semlia.proposal-input/v1", traceID,
			map[string]any{"violations": aiOutputInvalid.Violations})
		return "AI_OUTPUT_INVALID"
	case errors.Is(err, governanceapp.ErrNoSubstantiveChange):
		writeError(response, http.StatusUnprocessableEntity, "NO_SUBSTANTIVE_CHANGE",
			"the proposal change-set contains no substantive change", traceID, false)
		return "NO_SUBSTANTIVE_CHANGE"
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
		writeError(response, http.StatusUnprocessableEntity, "INVARIANT_VIOLATION", "the request violates a governance invariant", traceID, false)
		return "INVARIANT_VIOLATION"
	default:
		writeError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "the request could not be completed", traceID, false)
		return "INTERNAL_ERROR"
	}
}

func writeErrorWithDetails(response http.ResponseWriter, status int, code, message, traceID string, details map[string]any) {
	retryable := false
	payload := contract.ErrorResponse{
		Code:      code,
		Message:   message,
		TraceId:   traceID,
		Details:   details,
		Retryable: &retryable,
	}
	writeJSON(response, status, payload)
}
