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
	"github.com/iiwish/semlia/internal/application/governance/llm"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

const (
	routeGovernanceProposals routeKind = iota + 100
	routeGovernanceProposal
	routeGovernanceProposalSubmit
	routeGovernanceProposalValidationRuns
	routeGovernanceProposalPolicyDecision
	routeGovernanceProposalReviews
	routeGovernanceReviewBatches
	routeGovernanceReviewBatch
	routeGovernanceReviewBatchConfirm
	routeGovernanceReleases
	routeGovernanceRelease
	routeGovernanceReleaseRollback
	routeGovernanceModelProviders
	routeGovernanceModelProvider
	routeGovernanceModelSettings
	routeGovernanceModelSetting
	routeGovernanceModelSettingDefault
	routeGovernanceGenerateProposal
)

func isGovernanceRoute(kind routeKind) bool {
	switch kind {
	case routeGovernanceProposals, routeGovernanceProposal, routeGovernanceProposalSubmit,
		routeGovernanceProposalValidationRuns, routeGovernanceProposalPolicyDecision,
		routeGovernanceProposalReviews, routeGovernanceReviewBatches,
		routeGovernanceReviewBatch, routeGovernanceReviewBatchConfirm,
		routeGovernanceReleases, routeGovernanceRelease, routeGovernanceReleaseRollback,
		routeGovernanceModelProviders, routeGovernanceModelProvider,
		routeGovernanceModelSettings, routeGovernanceModelSetting,
		routeGovernanceModelSettingDefault, routeGovernanceGenerateProposal:
		return true
	}
	return false
}

func matchGovernanceRoute(parts []string, base matchedRoute) (matchedRoute, bool) {
	if len(parts) < 6 || parts[4] != "governance" {
		return matchedRoute{}, false
	}
	switch parts[5] {
	case "review-batches":
		switch {
		case len(parts) == 6:
			base.kind, base.label = routeGovernanceReviewBatches,
				"/api/v1/workspaces/{workspaceId}/governance/review-batches"
		case len(parts) == 7:
			base.batch = parts[6]
			base.kind, base.label = routeGovernanceReviewBatch,
				"/api/v1/workspaces/{workspaceId}/governance/review-batches/{batchId}"
		case len(parts) == 8 && parts[7] == "confirm":
			base.batch = parts[6]
			base.kind, base.label = routeGovernanceReviewBatchConfirm,
				"/api/v1/workspaces/{workspaceId}/governance/review-batches/{batchId}/confirm"
		default:
			return matchedRoute{}, false
		}
		return base, true
	case "releases":
		switch {
		case len(parts) == 6:
			base.kind, base.label = routeGovernanceReleases,
				"/api/v1/workspaces/{workspaceId}/governance/releases"
		case len(parts) == 7:
			base.release = parts[6]
			base.kind, base.label = routeGovernanceRelease,
				"/api/v1/workspaces/{workspaceId}/governance/releases/{releaseId}"
		case len(parts) == 8 && parts[7] == "rollback":
			base.release = parts[6]
			base.kind, base.label = routeGovernanceReleaseRollback,
				"/api/v1/workspaces/{workspaceId}/governance/releases/{releaseId}/rollback"
		default:
			return matchedRoute{}, false
		}
		return base, true
	case "model-providers":
		switch {
		case len(parts) == 6:
			base.kind, base.label = routeGovernanceModelProviders,
				"/api/v1/workspaces/{workspaceId}/governance/model-providers"
		case len(parts) == 7:
			base.provider = parts[6]
			base.kind, base.label = routeGovernanceModelProvider,
				"/api/v1/workspaces/{workspaceId}/governance/model-providers/{providerId}"
		default:
			return matchedRoute{}, false
		}
		return base, true
	case "model-settings":
		switch {
		case len(parts) == 6:
			base.kind, base.label = routeGovernanceModelSettings,
				"/api/v1/workspaces/{workspaceId}/governance/model-settings"
		case len(parts) == 7:
			base.setting = parts[6]
			base.kind, base.label = routeGovernanceModelSetting,
				"/api/v1/workspaces/{workspaceId}/governance/model-settings/{settingId}"
		case len(parts) == 8 && parts[7] == "set-default":
			base.setting = parts[6]
			base.kind, base.label = routeGovernanceModelSettingDefault,
				"/api/v1/workspaces/{workspaceId}/governance/model-settings/{settingId}/set-default"
		default:
			return matchedRoute{}, false
		}
		return base, true
	case "agent-runs":
		if len(parts) == 7 && parts[6] == "generate-proposal" {
			base.kind, base.label = routeGovernanceGenerateProposal,
				"/api/v1/workspaces/{workspaceId}/governance/agent-runs/generate-proposal"
			return base, true
		}
		return matchedRoute{}, false
	case "proposals":
	default:
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
	case len(parts) == 8 && parts[7] == "validation-runs":
		base.kind, base.label = routeGovernanceProposalValidationRuns, "/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}/validation-runs"
	case len(parts) == 8 && parts[7] == "policy-decision":
		base.kind, base.label = routeGovernanceProposalPolicyDecision, "/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}/policy-decision"
	case len(parts) == 8 && parts[7] == "reviews":
		base.kind, base.label = routeGovernanceProposalReviews, "/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}/reviews"
	default:
		return matchedRoute{}, false
	}
	return base, true
}

func governanceRouteMethods(kind routeKind) []string {
	switch kind {
	case routeGovernanceProposals, routeGovernanceReviewBatches, routeGovernanceReleases,
		routeGovernanceModelProviders, routeGovernanceModelSettings:
		return []string{http.MethodGet, http.MethodPost}
	case routeGovernanceProposalReviews:
		return []string{http.MethodGet, http.MethodPost}
	case routeGovernanceProposalSubmit,
		routeGovernanceReviewBatchConfirm, routeGovernanceReleaseRollback,
		routeGovernanceModelSettingDefault, routeGovernanceGenerateProposal:
		return []string{http.MethodPost}
	case routeGovernanceModelProvider, routeGovernanceModelSetting:
		return []string{http.MethodGet, http.MethodPut}
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
	case routeGovernanceReleases:
		if request.Method == http.MethodPost {
			return handler.publishGovernanceRelease(response, request, traceID, workspaceID)
		}
		return handler.listGovernanceReleases(response, request, traceID, workspaceID)
	case routeGovernanceRelease:
		releaseID, parseErr := identity.ParseReleaseID(route.release)
		if parseErr != nil {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		detail, getErr := handler.governance.Publishing().GetRelease(request.Context(), governanceapp.GetReleaseRequest{
			WorkspaceID: workspaceID, ReleaseID: releaseID,
			PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if getErr != nil {
			return writeGovernanceError(response, getErr, traceID)
		}
		writeJSON(response, http.StatusOK, governanceReleaseDetailResponse(detail))
		return ""
	case routeGovernanceReleaseRollback:
		releaseID, parseErr := identity.ParseReleaseID(route.release)
		if parseErr != nil {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		raw, readErr := readBody(request)
		if readErr != nil || len(strings.TrimSpace(string(raw))) != 0 {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		release, rollbackErr := handler.governance.Publishing().RollbackRelease(request.Context(), governanceapp.RollbackReleaseRequest{
			WorkspaceID: workspaceID, ReleaseID: releaseID,
			PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if rollbackErr != nil {
			return writeGovernanceError(response, rollbackErr, traceID)
		}
		detail, getErr := handler.governance.Publishing().CommittedReleaseDetail(
			request.Context(), workspaceID, release.ID, principalRef(request), traceID,
		)
		if getErr != nil {
			detail = unavailableReleaseDetail(release)
		}
		writeJSON(response, http.StatusCreated, governanceReleaseDetailResponse(detail))
		return ""
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
			PrincipalRef: principalRef(request), TraceID: traceID,
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
			PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if submitErr != nil {
			return writeGovernanceError(response, submitErr, traceID)
		}
		writeJSON(response, http.StatusOK, governanceProposalDetailResponse(detail))
		return ""
	case routeGovernanceProposalValidationRuns:
		proposalID, parseErr := identity.ParseProposalID(route.proposal)
		if parseErr != nil {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		records, listErr := handler.governance.ListValidationRuns(request.Context(), governanceapp.ListValidationRunsRequest{
			WorkspaceID: workspaceID, ProposalID: proposalID,
			PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if listErr != nil {
			return writeGovernanceError(response, listErr, traceID)
		}
		items := make([]contract.GovernanceValidationRun, 0, len(records))
		for _, record := range records {
			items = append(items, governanceValidationRunResponse(record))
		}
		writeJSON(response, http.StatusOK, contract.GovernanceValidationRunPage{Items: items})
		return ""
	case routeGovernanceProposalPolicyDecision:
		proposalID, parseErr := identity.ParseProposalID(route.proposal)
		if parseErr != nil {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		detail, decisionErr := handler.governance.GetPolicyDecision(request.Context(), governanceapp.GetPolicyDecisionRequest{
			WorkspaceID: workspaceID, ProposalID: proposalID,
			PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if decisionErr != nil {
			return writeGovernanceError(response, decisionErr, traceID)
		}
		writeJSON(response, http.StatusOK, governancePolicyDecisionResponse(detail))
		return ""
	case routeGovernanceProposalReviews:
		proposalID, parseErr := identity.ParseProposalID(route.proposal)
		if parseErr != nil {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		if request.Method == http.MethodGet {
			reviews, listErr := handler.governance.ListProposalReviews(request.Context(), governanceapp.ListProposalReviewsRequest{
				WorkspaceID: workspaceID, ProposalID: proposalID,
				PrincipalRef: principalRef(request), TraceID: traceID,
			})
			if listErr != nil {
				return writeGovernanceError(response, listErr, traceID)
			}
			items := make([]contract.GovernanceReview, 0, len(reviews))
			for _, review := range reviews {
				items = append(items, governanceReviewResponse(review))
			}
			writeJSON(response, http.StatusOK, contract.GovernanceReviewPage{Items: items})
			return ""
		}
		outcome, reviewErr := handler.createProposalReview(request, traceID, workspaceID, proposalID)
		if reviewErr != nil {
			return writeGovernanceError(response, reviewErr, traceID)
		}
		writeJSON(response, http.StatusCreated, governanceReviewResponse(outcome.Review))
		return ""
	case routeGovernanceReviewBatches:
		if request.Method == http.MethodPost {
			return handler.createReviewBatches(response, request, traceID, workspaceID)
		}
		return handler.listReviewBatches(response, request, traceID, workspaceID)
	case routeGovernanceReviewBatch:
		batchID, parseErr := identity.ParseReviewBatchID(route.batch)
		if parseErr != nil {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		detail, getErr := handler.governance.GetReviewBatch(request.Context(), governanceapp.GetReviewBatchRequest{
			WorkspaceID: workspaceID, BatchID: batchID,
			PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if getErr != nil {
			return writeGovernanceError(response, getErr, traceID)
		}
		writeJSON(response, http.StatusOK, governanceReviewBatchDetailResponse(detail))
		return ""
	case routeGovernanceReviewBatchConfirm:
		batchID, parseErr := identity.ParseReviewBatchID(route.batch)
		if parseErr != nil {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		result, confirmErr := handler.confirmReviewBatch(request, traceID, workspaceID, batchID)
		if confirmErr != nil {
			return writeGovernanceError(response, confirmErr, traceID)
		}
		writeJSON(response, http.StatusOK, governanceReviewBatchDetailResponse(result.Detail))
		return ""
	case routeGovernanceModelProviders:
		if handler.governance.ModelConfig() == nil {
			return writeGovernanceDependencyUnavailable(response, traceID)
		}
		if request.Method == http.MethodPost {
			return handler.createModelProvider(response, request, traceID, workspaceID)
		}
		return handler.listModelProviders(response, request, traceID, workspaceID)
	case routeGovernanceModelProvider:
		providerID, parseErr := identity.ParseModelProviderID(route.provider)
		if parseErr != nil {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		if handler.governance.ModelConfig() == nil {
			return writeGovernanceDependencyUnavailable(response, traceID)
		}
		if request.Method == http.MethodPut {
			return handler.updateModelProvider(response, request, traceID, workspaceID, providerID)
		}
		return handler.getModelProvider(response, request, traceID, workspaceID, providerID)
	case routeGovernanceModelSettings:
		if handler.governance.ModelConfig() == nil {
			return writeGovernanceDependencyUnavailable(response, traceID)
		}
		if request.Method == http.MethodPost {
			return handler.createModelSetting(response, request, traceID, workspaceID)
		}
		return handler.listModelSettings(response, request, traceID, workspaceID)
	case routeGovernanceModelSetting:
		settingID, parseErr := identity.ParseModelSettingID(route.setting)
		if parseErr != nil {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		if handler.governance.ModelConfig() == nil {
			return writeGovernanceDependencyUnavailable(response, traceID)
		}
		if request.Method == http.MethodPut {
			return handler.updateModelSetting(response, request, traceID, workspaceID, settingID)
		}
		return handler.getModelSetting(response, request, traceID, workspaceID, settingID)
	case routeGovernanceModelSettingDefault:
		settingID, parseErr := identity.ParseModelSettingID(route.setting)
		if parseErr != nil {
			return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
		}
		if handler.governance.ModelConfig() == nil {
			return writeGovernanceDependencyUnavailable(response, traceID)
		}
		setting, defaultErr := handler.governance.ModelConfig().SetDefault(request.Context(), governanceapp.SetDefaultModelSettingRequest{
			WorkspaceID: workspaceID, SettingID: settingID,
			PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if defaultErr != nil {
			return writeGovernanceError(response, defaultErr, traceID)
		}
		writeJSON(response, http.StatusOK, modelSettingResponse(setting))
		return ""
	case routeGovernanceGenerateProposal:
		if handler.governance.Generation() == nil {
			return writeGovernanceDependencyUnavailable(response, traceID)
		}
		result, generateErr := handler.generateProposal(request, traceID, workspaceID)
		if generateErr != nil {
			return writeGovernanceError(response, generateErr, traceID)
		}
		writeJSON(response, http.StatusCreated, generatedProposalResponse(result))
		return ""
	default:
		panic("governance route is not handled")
	}
}

func writeGovernanceDependencyUnavailable(response http.ResponseWriter, traceID string) string {
	response.Header().Set("Retry-After", retryAfter)
	writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE",
		"the governance dependency is unavailable", traceID, true)
	return "DEPENDENCY_UNAVAILABLE"
}

func (handler *Handler) createProposalReview(
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
	proposalID identity.ProposalID,
) (governanceapp.ReviewOutcome, error) {
	raw, err := readBody(request)
	if err != nil {
		return governanceapp.ReviewOutcome{}, domain.ErrInvalidArgument
	}
	var body contract.GovernanceReviewCommandRequest
	if err := decodeStrict(raw, &body); err != nil {
		return governanceapp.ReviewOutcome{}, domain.ErrInvalidArgument
	}
	return handler.governance.ReviewProposal(request.Context(), governanceapp.ReviewProposalRequest{
		WorkspaceID: workspaceID, ProposalID: proposalID,
		Decision: governanceapp.ReviewCommandDecision(body.Decision), Reason: body.Reason,
		PrincipalRef: principalRef(request), TraceID: traceID,
	})
}

func (handler *Handler) createReviewBatches(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) string {
	raw, err := readBody(request)
	if err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	if len(strings.TrimSpace(string(raw))) != 0 {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	details, err := handler.governance.CreateReviewBatches(request.Context(), governanceapp.CreateReviewBatchesRequest{
		WorkspaceID:  workspaceID,
		PrincipalRef: principalRef(request), TraceID: traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	writeJSON(response, http.StatusOK, governanceReviewBatchPageResponse(details, 0, ""))
	return ""
}

func (handler *Handler) listReviewBatches(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) string {
	limit, err := queryInteger(request, "limit")
	if err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	page, err := handler.governance.ListOpenReviewBatches(request.Context(), governanceapp.ListReviewBatchesRequest{
		WorkspaceID: workspaceID, Limit: limit, Cursor: request.URL.Query().Get("cursor"),
		PrincipalRef: principalRef(request), TraceID: traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	writeJSON(response, http.StatusOK, governanceReviewBatchPageResponse(page.Items, page.Limit, page.NextCursor))
	return ""
}

func (handler *Handler) confirmReviewBatch(
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
	batchID identity.ReviewBatchID,
) (governanceapp.ConfirmReviewBatchResult, error) {
	raw, err := readBody(request)
	if err != nil {
		return governanceapp.ConfirmReviewBatchResult{}, domain.ErrInvalidArgument
	}
	var body contract.GovernanceReviewCommandRequest
	if err := decodeStrict(raw, &body); err != nil {
		return governanceapp.ConfirmReviewBatchResult{}, domain.ErrInvalidArgument
	}
	return handler.governance.ConfirmReviewBatch(request.Context(), governanceapp.ConfirmReviewBatchRequest{
		WorkspaceID: workspaceID, BatchID: batchID,
		Decision: governanceapp.ReviewCommandDecision(body.Decision), Reason: body.Reason,
		PrincipalRef: principalRef(request), TraceID: traceID,
	})
}

func (handler *Handler) publishGovernanceRelease(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) string {
	raw, err := readBody(request)
	if err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	var body contract.PublishGovernanceReleaseRequest
	if err := decodeStrict(raw, &body); err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	release, err := handler.governance.Publishing().PublishProposal(request.Context(), governanceapp.PublishProposalRequest{
		WorkspaceID: workspaceID, ProposalID: body.ProposalId,
		PrincipalRef: principalRef(request), TraceID: traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	detail, getErr := handler.governance.Publishing().CommittedReleaseDetail(
		request.Context(), workspaceID, release.ID, principalRef(request), traceID,
	)
	if getErr != nil {
		detail = unavailableReleaseDetail(release)
	}
	writeJSON(response, http.StatusCreated, governanceReleaseDetailResponse(detail))
	return ""
}

func (handler *Handler) listGovernanceReleases(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) string {
	limit, err := queryInteger(request, "limit")
	if err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	page, err := handler.governance.Publishing().ListReleases(request.Context(), governanceapp.ListReleasesRequest{
		WorkspaceID: workspaceID, Limit: limit, Cursor: request.URL.Query().Get("cursor"),
		PrincipalRef: principalRef(request), TraceID: traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	items := make([]contract.GovernanceRelease, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, governanceReleaseSummaryResponse(item))
	}
	total := page.Total
	result := contract.GovernanceReleasePage{Items: items, Page: contract.PageInfo{Limit: page.Limit, Total: &total}}
	if page.NextCursor != "" {
		result.Page.NextCursor = &page.NextCursor
	}
	writeJSON(response, http.StatusOK, result)
	return ""
}

func governanceReleaseSummaryResponse(release domain.Release) contract.GovernanceRelease {
	result := contract.GovernanceRelease{
		Id:             release.ID,
		Sequence:       release.Sequence,
		State:          contract.GovernanceReleaseState(release.State),
		ManifestDigest: release.ManifestDigest,
		PublishedBy:    release.PublishedBy,
		PublishedAt:    release.PublishedAt.UTC(),
		CreatedAt:      release.CreatedAt.UTC(),
	}
	if release.RolledBackToReleaseID != nil {
		target := *release.RolledBackToReleaseID
		result.RolledBackToReleaseId = &target
	}
	if release.OriginProposalID != nil {
		origin := *release.OriginProposalID
		result.OriginProposalId = &origin
	}
	return result
}

func governanceReleaseDetailResponse(value governanceapp.ReleaseDetail) contract.GovernanceReleaseDetail {
	release := value.Release
	summary := governanceReleaseSummaryResponse(release)
	detail := contract.GovernanceReleaseDetail{
		Id: summary.Id, Sequence: summary.Sequence, State: summary.State,
		ManifestDigest:        summary.ManifestDigest,
		RolledBackToReleaseId: summary.RolledBackToReleaseId,
		OriginProposalId:      summary.OriginProposalId,
		PublishedBy:           summary.PublishedBy, PublishedAt: summary.PublishedAt,
		CreatedAt: summary.CreatedAt,
		Authority: value.Authority, Availability: contract.CatalogAuthorityAvailability(value.Availability),
		ObjectAvailability:         contract.CatalogAuthorityAvailability(value.ObjectAvailability),
		ConsumerImpactAvailability: contract.CatalogAuthorityAvailability(value.ConsumerImpactAvailability),
		DiffAvailability:           contract.CatalogAuthorityAvailability(value.DiffAvailability),
		PriorPinDiff:               make([]contract.GovernanceReleaseDiffEntry, 0, len(value.PriorPinDiff)),
		CurrentRegistryDiff:        make([]contract.GovernanceReleaseDiffEntry, 0, len(value.CurrentRegistryDiff)),
		Manifest: contract.GovernanceReleaseManifest{
			Assets:  make([]contract.GovernanceReleaseManifestAsset, 0, len(release.Entries)),
			Objects: make([]contract.GovernanceReleaseManifestObject, 0, len(release.Objects)),
		},
	}
	if value.ConsumerImpact != nil {
		detail.ConsumerImpact = &contract.GovernanceReleaseConsumerImpact{
			Current: value.ConsumerImpact.Current, Pinned: value.ConsumerImpact.Pinned,
		}
	}
	mapDiff := func(entries []governanceapp.ReleaseDiffEntry) []contract.GovernanceReleaseDiffEntry {
		result := make([]contract.GovernanceReleaseDiffEntry, 0, len(entries))
		for _, entry := range entries {
			var baseline *string
			if entry.BaselineVersion != "" {
				value := entry.BaselineVersion
				baseline = &value
			}
			result = append(result, contract.GovernanceReleaseDiffEntry{
				TargetType: contract.GovernanceTargetObjectType(entry.TargetType), TargetId: entry.TargetID,
				Change: contract.GovernanceReleaseDiffEntryChange(entry.Change), BaselineVersion: baseline,
				SelectedVersion: entry.SelectedVersion,
			})
		}
		return result
	}
	detail.PriorPinDiff = mapDiff(value.PriorPinDiff)
	detail.CurrentRegistryDiff = mapDiff(value.CurrentRegistryDiff)
	for _, entry := range release.Entries {
		compatibility := entry.Compatibility
		if len(compatibility) == 0 {
			compatibility = json.RawMessage(`{}`)
		}
		detail.Manifest.Assets = append(detail.Manifest.Assets, contract.GovernanceReleaseManifestAsset{
			AssetId: entry.AssetID, RevisionId: entry.RevisionID,
			Compatibility: compatibility, Position: entry.Position,
		})
	}
	for _, entry := range release.Objects {
		typedID, err := entry.TypedID()
		if err != nil {
			continue
		}
		detail.Manifest.Objects = append(detail.Manifest.Objects, contract.GovernanceReleaseManifestObject{
			ObjectType: contract.GovernanceTargetObjectType(entry.ObjectType), ObjectId: typedID,
			Version: entry.Version, Position: entry.Position,
		})
	}
	return detail
}

func unavailableReleaseDetail(release domain.Release) governanceapp.ReleaseDetail {
	release.Objects = []domain.ObjectManifestEntry{}
	return governanceapp.ReleaseDetail{Release: release,
		Authority: "releases/release_assets/release_object_snapshots", Availability: "failed",
		ObjectAvailability: "failed", ConsumerImpactAvailability: "failed", DiffAvailability: "failed",
		PriorPinDiff: []governanceapp.ReleaseDiffEntry{}, CurrentRegistryDiff: []governanceapp.ReleaseDiffEntry{}}
}

func governancePolicyDecisionResponse(detail governanceapp.PolicyDecisionDetail) contract.GovernancePolicyDecision {
	decision := detail.Decision
	result := contract.GovernancePolicyDecision{
		RuleVersion:        decision.RuleVersion,
		InputsDigest:       decision.InputsDigest,
		RiskLevel:          contract.GovernanceRiskLevel(decision.RiskLevel),
		Routing:            contract.GovernanceRoutingChannel(decision.Routing),
		MatchedRuleId:      decision.MatchedPolicy,
		ReasonCode:         decision.ReasonCode,
		Explanation:        detail.Explanation,
		MatchedInputFields: detail.MatchedInputFields,
		CreatedAt:          decision.DecidedAt.UTC(),
	}
	if result.MatchedInputFields == nil {
		result.MatchedInputFields = []string{}
	}
	return result
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
		PrincipalRef: principalRef(request), TraceID: traceID,
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
		PrincipalRef:   principalRef(request),
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

func governanceValidationRunResponse(record governanceapp.ValidationRunRecord) contract.GovernanceValidationRun {
	run := record.Run
	result := contract.GovernanceValidationRun{
		Id:               run.ID,
		ProposalId:       run.ProposalID,
		ValidatorId:      run.ValidatorID,
		ValidatorVersion: run.ValidatorVersion,
		Status:           contract.GovernanceValidationStatus(run.Status),
		StartedAt:        run.StartedAt.UTC(),
		Results:          make([]contract.GovernanceValidationResult, 0, len(record.Results)),
	}
	if run.FinishedAt != nil {
		finishedAt := run.FinishedAt.UTC()
		result.FinishedAt = &finishedAt
	}
	for _, item := range record.Results {
		governanceResult := contract.GovernanceValidationResult{
			Id:          item.ID,
			Severity:    contract.GovernanceValidationSeverity(item.Severity),
			Code:        item.Code,
			Message:     item.Message,
			InputDigest: item.InputDigest,
			CreatedAt:   item.CreatedAt.UTC(),
		}
		if item.Details != nil {
			details := json.RawMessage(append([]byte(nil), item.Details...))
			governanceResult.Details = &details
		}
		result.Results = append(result.Results, governanceResult)
	}
	return result
}

func governanceReviewResponse(review domain.Review) contract.GovernanceReview {
	return contract.GovernanceReview{
		Id:                  review.ID,
		ProposalId:          review.ProposalID,
		ReviewerPrincipalId: review.ReviewerPrincipalID,
		Channel:             contract.GovernanceReviewChannel(review.Channel),
		Decision:            contract.GovernanceReviewRecordedDecision(review.Decision),
		Note:                review.Note,
		CreatedAt:           review.CreatedAt.UTC(),
	}
}

func governanceReviewBatchPageResponse(
	items []domain.ReviewBatchDetail, limit int, nextCursor string,
) contract.GovernanceReviewBatchPage {
	page := contract.GovernanceReviewBatchPage{
		Items: make([]contract.GovernanceReviewBatch, 0, len(items)),
		Page:  contract.PageInfo{Limit: limit},
	}
	if page.Page.Limit == 0 {
		page.Page.Limit = defaultGovernanceBatchPageLimit
	}
	for _, item := range items {
		page.Items = append(page.Items, governanceReviewBatchSummaryResponse(item))
	}
	if nextCursor != "" {
		cursor := nextCursor
		page.Page.NextCursor = &cursor
	}
	return page
}

const defaultGovernanceBatchPageLimit = 50

func governanceReviewBatchSummaryResponse(detail domain.ReviewBatchDetail) contract.GovernanceReviewBatch {
	batch := detail.Batch
	result := contract.GovernanceReviewBatch{
		Id:            batch.ID,
		Status:        contract.GovernanceReviewBatchStatus(batch.Status),
		GroupingRule:  governanceGroupingRuleResponse(batch.GroupingRule),
		PolicyVersion: batch.PolicyVersion,
		MemberCount:   len(detail.Members),
		CreatedBy:     batch.CreatedBy,
		CreatedAt:     batch.CreatedAt.UTC(),
	}
	if batch.DecidedBy != nil {
		decidedBy := *batch.DecidedBy
		result.DecidedBy = &decidedBy
	}
	if batch.DecidedAt != nil {
		decidedAt := *batch.DecidedAt
		result.DecidedAt = &decidedAt
	}
	return result
}

func governanceGroupingRuleResponse(rule domain.ReviewGroupingRule) contract.GovernanceReviewGroupingRule {
	return contract.GovernanceReviewGroupingRule{
		TargetObjectType: contract.GovernanceTargetObjectType(rule.TargetObjectType),
		DiffCategory:     contract.GovernanceDiffCategory(rule.DiffCategory),
		MatchedRuleId:    rule.MatchedRuleID,
	}
}

func governanceReviewBatchMemberResponse(member domain.ReviewBatchMember) contract.GovernanceReviewBatchMember {
	result := contract.GovernanceReviewBatchMember{
		ProposalId:  member.ProposalID,
		AddedReason: governanceAddedReasonResponse(member.AddedReason),
		Sample:      member.Sample,
		SplitOut:    member.SplitOut,
		CreatedAt:   member.CreatedAt.UTC(),
	}
	if member.Decision != nil {
		decision := contract.GovernanceReviewRecordedDecision(*member.Decision)
		result.Decision = &decision
	}
	if member.SplitReason != nil {
		reason := *member.SplitReason
		result.SplitReason = &reason
	}
	return result
}

func governanceAddedReasonResponse(raw json.RawMessage) contract.GovernanceReviewAddedReason {
	reason, err := domain.ParseReviewAddedReason(raw)
	if err != nil {
		return contract.GovernanceReviewAddedReason{}
	}
	return contract.GovernanceReviewAddedReason{
		MatchedRuleId: reason.MatchedRuleID,
		RiskLevel:     contract.GovernanceRiskLevel(reason.RiskLevel),
		ReasonCode:    reason.ReasonCode,
		RuleVersion:   reason.RuleVersion,
		InputsDigest:  reason.InputsDigest,
	}
}

func governanceReviewBatchDetailResponse(detail domain.ReviewBatchDetail) contract.GovernanceReviewBatchDetail {
	result := contract.GovernanceReviewBatchDetail{
		Id:            detail.Batch.ID,
		Status:        contract.GovernanceReviewBatchStatus(detail.Batch.Status),
		GroupingRule:  governanceGroupingRuleResponse(detail.Batch.GroupingRule),
		PolicyVersion: detail.Batch.PolicyVersion,
		MemberCount:   len(detail.Members),
		CreatedBy:     detail.Batch.CreatedBy,
		CreatedAt:     detail.Batch.CreatedAt.UTC(),
		Members:       make([]contract.GovernanceReviewBatchMember, 0, len(detail.Members)),
		Samples:       make([]contract.GovernanceReviewBatchMember, 0, len(detail.Samples)),
		Exclusions:    make([]contract.GovernanceReviewBatchMember, 0, len(detail.Exclusions)),
	}
	if detail.Batch.DecidedBy != nil {
		decidedBy := *detail.Batch.DecidedBy
		result.DecidedBy = &decidedBy
	}
	if detail.Batch.DecidedAt != nil {
		decidedAt := *detail.Batch.DecidedAt
		result.DecidedAt = &decidedAt
	}
	if detail.MaxRiskMember != nil {
		maxRisk := detail.MaxRiskMember.ProposalID
		result.MaxRiskProposalId = &maxRisk
	}
	for _, member := range detail.Members {
		result.Members = append(result.Members, governanceReviewBatchMemberResponse(member))
	}
	for _, member := range detail.Samples {
		result.Samples = append(result.Samples, governanceReviewBatchMemberResponse(member))
	}
	for _, member := range detail.Exclusions {
		result.Exclusions = append(result.Exclusions, governanceReviewBatchMemberResponse(member))
	}
	return result
}

func writeGovernanceError(response http.ResponseWriter, err error, traceID string) string {
	var denial *authz.DenialError
	var dutyConflict *governanceapp.SeparationOfDutyError
	var aiOutputInvalid *governanceapp.AIOutputInvalidError
	var releaseRefusal *governanceapp.ReleaseRefusalError
	var providerUnsupported *llm.ProviderUnsupportedError
	var providerUnavailable *llm.ProviderUnavailableError
	switch {
	case errors.As(err, &dutyConflict):
		details := map[string]any{
			"conflict":     dutyConflict.Conflict,
			"scope":        dutyConflict.Scope,
			"policySource": dutyConflict.Source,
			"recovery":     dutyConflict.Recovery,
		}
		if len(dutyConflict.Conflicted) > 0 {
			proposalIDs := make([]string, 0, len(dutyConflict.Conflicted))
			for _, proposal := range dutyConflict.Conflicted {
				proposalIDs = append(proposalIDs, proposal.String())
			}
			details["conflictingMembers"] = proposalIDs
		}
		writeErrorWithDetails(response, http.StatusForbidden, "SEPARATION_OF_DUTY",
			dutyConflict.Conflict, traceID, details)
		return "SEPARATION_OF_DUTY"
	case errors.As(err, &releaseRefusal):
		writeErrorWithDetails(response, releaseRefusal.Status, releaseRefusal.Code,
			releaseRefusal.Message, traceID, releaseRefusal.Details)
		return releaseRefusal.Code
	case errors.As(err, &denial):
		writeError(response, http.StatusForbidden, string(denial.Decision.ReasonCode),
			"the acting principal lacks the required capability", traceID, false)
		return string(denial.Decision.ReasonCode)
	case errors.As(err, &providerUnsupported):
		writeError(response, http.StatusUnprocessableEntity, "PROVIDER_UNSUPPORTED",
			"the provider protocol has no live adapter", traceID, false)
		return "PROVIDER_UNSUPPORTED"
	case errors.As(err, &providerUnavailable):
		writeError(response, http.StatusServiceUnavailable, "PROVIDER_UNAVAILABLE",
			"the configured provider is unavailable", traceID, true)
		return "PROVIDER_UNAVAILABLE"
	case errors.As(err, &aiOutputInvalid):
		writeErrorWithDetails(response, http.StatusUnprocessableEntity, "AI_OUTPUT_INVALID",
			"the agent structured output does not match the required schema", traceID,
			map[string]any{"violations": aiOutputInvalid.Violations})
		return "AI_OUTPUT_INVALID"
	case errors.Is(err, governanceapp.ErrNoSubstantiveChange):
		writeError(response, http.StatusUnprocessableEntity, "NO_SUBSTANTIVE_CHANGE",
			"the proposal change-set contains no substantive change", traceID, false)
		return "NO_SUBSTANTIVE_CHANGE"
	case errors.Is(err, domain.ErrProductionSetRequired):
		writeError(response, http.StatusConflict, "PRODUCTION_SET_REQUIRED",
			"the proposal or release belongs to a semantic production set; use the production endpoints", traceID, false)
		return "PRODUCTION_SET_REQUIRED"
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
