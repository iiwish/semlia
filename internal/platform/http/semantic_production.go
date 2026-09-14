package httpapi

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func isProductionRoute(kind routeKind) bool {
	return kind == routeProductionOperations ||
		kind == routeProductionOperation ||
		kind == routeProductionSubmit ||
		kind == routeProductionValidations ||
		kind == routeProductionReviews ||
		kind == routeProductionPublish ||
		kind == routeProductionRelease ||
		kind == routeProductionGeneration ||
		kind == routeProductionGenerationRun ||
		kind == routeProductionBusinessRules ||
		kind == routeProductionRollback
}

func (handler *Handler) routeProduction(response http.ResponseWriter, request *http.Request, traceID string, route matchedRoute) string {
	workspaceID, err := identity.ParseWorkspaceID(route.workspace)
	if err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid workspace id", traceID, false)
		return "INVALID_ARGUMENT"
	}

	switch route.kind {
	case routeProductionBusinessRules:
		return handler.productionBusinessRuleRequest(response, request, traceID, workspaceID, route)
	case routeProductionGeneration, routeProductionGenerationRun:
		return handler.productionGenerationRequest(response, request, traceID, workspaceID, route)
	case routeProductionOperations:
		switch request.Method {
		case http.MethodGet:
			return handler.listProductionOperations(response, request, traceID, workspaceID)
		case http.MethodPost:
			return handler.createProductionOperation(response, request, traceID, workspaceID)
		default:
			writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", traceID, false)
			return "METHOD_NOT_ALLOWED"
		}
	case routeProductionOperation:
		operationID, err := identity.ParseProductionOperationID(route.operation)
		if err != nil {
			writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid operation id", traceID, false)
			return "INVALID_ARGUMENT"
		}
		switch request.Method {
		case http.MethodGet:
			return handler.getProductionOperation(response, request, traceID, workspaceID, operationID)
		case http.MethodPut:
			return handler.replaceProductionDraft(response, request, traceID, workspaceID, operationID)
		default:
			writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", traceID, false)
			return "METHOD_NOT_ALLOWED"
		}
	case routeProductionSubmit:
		operationID, err := identity.ParseProductionOperationID(route.operation)
		if err != nil {
			writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid operation id", traceID, false)
			return "INVALID_ARGUMENT"
		}
		if request.Method != http.MethodPost {
			writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", traceID, false)
			return "METHOD_NOT_ALLOWED"
		}
		return handler.submitProductionOperation(response, request, traceID, workspaceID, operationID)
	case routeProductionValidations:
		operationID, err := identity.ParseProductionOperationID(route.operation)
		if err != nil {
			writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid operation id", traceID, false)
			return "INVALID_ARGUMENT"
		}
		switch request.Method {
		case http.MethodGet:
			return handler.listProductionValidationAttempts(response, request, traceID, workspaceID, operationID)
		case http.MethodPost:
			return handler.validateProductionOperation(response, request, traceID, workspaceID, operationID)
		default:
			writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", traceID, false)
			return "METHOD_NOT_ALLOWED"
		}
	case routeProductionReviews:
		operationID, err := identity.ParseProductionOperationID(route.operation)
		if err != nil {
			writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid operation id", traceID, false)
			return "INVALID_ARGUMENT"
		}
		if request.Method != http.MethodPost {
			writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", traceID, false)
			return "METHOD_NOT_ALLOWED"
		}
		return handler.reviewProductionOperation(response, request, traceID, workspaceID, operationID)
	case routeProductionPublish:
		operationID, err := identity.ParseProductionOperationID(route.operation)
		if err != nil {
			writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid operation id", traceID, false)
			return "INVALID_ARGUMENT"
		}
		if request.Method != http.MethodPost {
			writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", traceID, false)
			return "METHOD_NOT_ALLOWED"
		}
		return handler.publishProductionOperation(response, request, traceID, workspaceID, operationID)
	case routeProductionRelease:
		releaseID, err := identity.ParseReleaseID(route.release)
		if err != nil {
			writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid release id", traceID, false)
			return "INVALID_ARGUMENT"
		}
		if request.Method != http.MethodGet {
			writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", traceID, false)
			return "METHOD_NOT_ALLOWED"
		}
		return handler.getProductionRelease(response, request, traceID, workspaceID, releaseID)
	case routeProductionRollback:
		releaseID, err := identity.ParseReleaseID(route.release)
		if err != nil {
			writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid release id", traceID, false)
			return "INVALID_ARGUMENT"
		}
		if request.Method != http.MethodPost {
			writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", traceID, false)
			return "METHOD_NOT_ALLOWED"
		}
		return handler.rollbackProductionRelease(response, request, traceID, workspaceID, releaseID)
	default:
		writeError(response, http.StatusNotFound, "NOT_FOUND", "route not found", traceID, false)
		return "NOT_FOUND"
	}
}

// Wire request payloads
type CreateProductionRequestPayload struct {
	Input                 ProductionInputPayload    `json:"input"`
	Targets               []ProductionTargetPayload `json:"targets"`
	SupersedesOperationID *string                   `json:"supersedesOperationId,omitempty"`
}

type ReplaceProductionRequestPayload struct {
	ExpectedVersion int                       `json:"expectedVersion"`
	Input           ProductionInputPayload    `json:"input"`
	Targets         []ProductionTargetPayload `json:"targets"`
	SuggestionRunID *string                   `json:"suggestionRunId,omitempty"`
}

type ProductionInputPayload struct {
	Snapshots    []SnapshotSelectionPayload  `json:"snapshots"`
	Candidates   []CandidateSelectionPayload `json:"candidates"`
	Evidence     []EvidenceSelectionPayload  `json:"evidence"`
	Dependencies []json.RawMessage           `json:"dependencies"`
}

type SnapshotSelectionPayload struct {
	SourceID     string   `json:"sourceId"`
	SnapshotID   string   `json:"snapshotId"`
	Digest       string   `json:"digest"`
	CoverageKeys []string `json:"coverageKeys"`
}

type CandidateSelectionPayload struct {
	SnapshotID       string   `json:"snapshotId"`
	CandidateID      string   `json:"candidateId"`
	Digest           string   `json:"digest"`
	PrimaryTargetKey string   `json:"primaryTargetKey"`
	TargetKeys       []string `json:"targetKeys"`
}

type EvidenceSelectionPayload struct {
	SnapshotID string `json:"snapshotId,omitempty"`
	EvidenceID string `json:"evidenceId"`
	Digest     string `json:"digest"`
}

type ProductionTargetPayload struct {
	ReuseIdentity        *domain.ProductionReintroductionIdentity `json:"reuseIdentity,omitempty"`
	Intent               string                                   `json:"intent"`
	Kind                 string                                   `json:"kind"`
	LocalKey             string                                   `json:"localKey"`
	IdentityKey          *string                                  `json:"identityKey,omitempty"`
	TargetID             *string                                  `json:"targetId,omitempty"`
	BaseRevisionID       *string                                  `json:"baseRevisionId,omitempty"`
	BaseObjectVersion    *int                                     `json:"baseObjectVersion,omitempty"`
	RegistryWriteVersion *int                                     `json:"registryWriteVersion,omitempty"`
	Title                string                                   `json:"title"`
	Content              json.RawMessage                          `json:"content,omitempty"`
	Changes              []domain.TargetChangeInput               `json:"changes"`
	EvidenceIDs          []string                                 `json:"evidenceIds"`
}

// Wire response payloads
type ProductionCommandResultResponse struct {
	OperationID         string   `json:"operationId"`
	Version             int      `json:"version"`
	SetDigest           string   `json:"setDigest"`
	Replayed            bool     `json:"replayed"`
	Outcome             string   `json:"outcome"`
	ProposalIDs         []string `json:"proposalIds"`
	ValidationAttemptNo *int     `json:"validationAttemptNo"`
	ValidationRunIDs    []string `json:"validationRunIds"`
	ReviewIDs           []string `json:"reviewIds"`
}

type OperationSummaryResponse struct {
	ID             string  `json:"id"`
	CurrentVersion int     `json:"currentVersion"`
	CreatedBy      string  `json:"createdBy"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
	Frozen         bool    `json:"frozen"`
	Progress       string  `json:"progress"`
	TargetCount    int     `json:"targetCount"`
	ReleaseID      *string `json:"releaseId"`
}

type HeadReferenceResponse struct {
	Presence       string              `json:"presence"`
	ReleaseID      *identity.ReleaseID `json:"releaseId,omitempty"`
	ManifestDigest *string             `json:"manifestDigest,omitempty"`
}

type TargetResultResponse struct {
	LocalKey         string                  `json:"localKey"`
	TargetID         string                  `json:"targetId"`
	Declaration      ProductionTargetPayload `json:"declaration"`
	Outcome          string                  `json:"outcome"`
	ContentDigest    string                  `json:"contentDigest"`
	ProposalID       *string                 `json:"proposalId"`
	ProposalState    *string                 `json:"proposalState"`
	ValidationRunIDs []string                `json:"validationRunIds"`
	ReviewIDs        []string                `json:"reviewIds"`
}

type ValidationStatusResponse struct {
	Status string `json:"status"`
}

type ProductionOperationResponse struct {
	Summary                OperationSummaryResponse `json:"summary"`
	Version                int                      `json:"version"`
	Input                  ProductionInputPayload   `json:"input"`
	InputDigest            string                   `json:"inputDigest"`
	SetDigest              string                   `json:"setDigest"`
	BaselineHead           HeadReferenceResponse    `json:"baselineHead"`
	Targets                []TargetResultResponse   `json:"targets"`
	ActiveValidation       any                      `json:"activeValidation"`
	GenerationRunIDs       []string                 `json:"generationRunIds"`
	GenerationApplications []any                    `json:"generationApplications"`
	UnresolvedCodes        []string                 `json:"unresolvedCodes"`
}

type ProductionOperationPageResponse struct {
	Items      []OperationSummaryResponse `json:"items"`
	NextCursor *string                    `json:"nextCursor"`
}

func (handler *Handler) checkProductionAuth(ctx context.Context, actor string, workspaceID identity.WorkspaceID, action authz.Action, traceID string) error {
	if handler.authorization == nil {
		return nil
	}
	decision, err := handler.authorization.Evaluate(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: actor,
		WorkspaceID:  workspaceID,
		Action:       action,
		Resource: authz.Resource{
			Type: authz.ScopeWorkspace,
			ID:   workspaceID.UUID(),
		},
		TraceID: traceID,
	})
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return &authz.DenialError{Decision: decision}
	}
	return nil
}

func (handler *Handler) createProductionOperation(response http.ResponseWriter, request *http.Request, traceID string, workspaceID identity.WorkspaceID) string {
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if !validProductionIdempotencyKey(idempotencyKey) {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "Idempotency-Key header must be between 8 and 128 characters", traceID, false)
		return "INVALID_ARGUMENT"
	}

	actor := principalRef(request)
	if actor == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", traceID, false)
		return "UNAUTHENTICATED"
	}
	principalID, err := identity.ParsePrincipalID(actor)
	if err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid principal", traceID, false)
		return "INVALID_ARGUMENT"
	}

	// Authorize propose
	if err := handler.checkProductionAuthoringAccess(request.Context(), actor, workspaceID); err != nil {
		return writeProductionError(response, err, traceID)
	}

	var body CreateProductionRequestPayload
	if err := decodeStrictJSON(response, request, &body); err != nil {
		return writeProductionError(response, err, traceID)
	}

	// Convert declarations
	var targets []domain.TargetDeclaration
	for _, t := range body.Targets {
		targets = append(targets, domain.TargetDeclaration{
			LocalKey:             t.LocalKey,
			Title:                t.Title,
			TargetID:             t.TargetID,
			EvidenceIDs:          t.EvidenceIDs,
			Kind:                 t.Kind,
			Intent:               t.Intent,
			IdentityKey:          t.IdentityKey,
			BaseRevisionID:       t.BaseRevisionID,
			BaseObjectVersion:    t.BaseObjectVersion,
			RegistryWriteVersion: t.RegistryWriteVersion,
			Content:              t.Content,
			ReuseIdentity:        t.ReuseIdentity,
			Changes:              t.Changes,
		})
	}

	var candidates []domain.CandidateDeclaration
	for _, c := range body.Input.Candidates {
		candidates = append(candidates, domain.CandidateDeclaration{
			CandidateID:      c.CandidateID,
			CandidateDigest:  c.Digest,
			TargetKeys:       c.TargetKeys,
			PrimaryTargetKey: c.PrimaryTargetKey,
		})
	}

	var inputSnapshotID *identity.SourceSnapshotID
	if len(body.Input.Snapshots) > 0 {
		sid, parseErr := identity.ParseSourceSnapshotID(body.Input.Snapshots[0].SnapshotID)
		if parseErr == nil {
			inputSnapshotID = &sid
		}
	}

	scopeBytes, _ := json.Marshal(body.Input)

	var supersedesID *identity.ProductionOperationID
	if body.SupersedesOperationID != nil {
		sid, parseErr := identity.ParseProductionOperationID(*body.SupersedesOperationID)
		if parseErr != nil {
			return writeProductionError(response, domain.ErrInvalidArgument, traceID)
		}
		supersedesID = &sid
	}

	cmd := governanceapp.CreateOperationCommand{
		WorkspaceID:           workspaceID,
		PrincipalID:           principalID,
		IdempotencyKey:        idempotencyKey,
		SupersedesOperationID: supersedesID,
		InputSnapshotID:       inputSnapshotID,
		InputScope:            scopeBytes,
		Targets:               targets,
		Candidates:            candidates,
		TraceID:               traceID,
	}

	result, err := handler.production.CreateOperation(request.Context(), cmd)
	if err != nil {
		handler.logger.ErrorContext(request.Context(), "CreateOperation failed", "error", err)
		return writeProductionError(response, err, traceID)
	}

	proposalIDs := []string{}
	allNoChange := true
	for _, t := range result.Targets {
		if t.ProposalID != nil {
			proposalIDs = append(proposalIDs, t.ProposalID.String())
			allNoChange = false
		}
	}

	outcome := "created"
	if allNoChange {
		outcome = "no_change"
	}

	cmdResult := ProductionCommandResultResponse{
		OperationID:         result.OperationID.String(),
		Version:             result.Version,
		SetDigest:           result.SetDigest,
		Replayed:            result.Replayed,
		Outcome:             outcome,
		ProposalIDs:         proposalIDs,
		ValidationAttemptNo: nil,
		ValidationRunIDs:    []string{},
		ReviewIDs:           []string{},
	}

	if result.Replayed || allNoChange {
		writeJSON(response, http.StatusOK, cmdResult)
		return ""
	}

	location := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s", workspaceID.String(), result.OperationID.String())
	response.Header().Set("Location", location)
	writeJSON(response, http.StatusCreated, cmdResult)
	return ""
}

func (handler *Handler) replaceProductionDraft(response http.ResponseWriter, request *http.Request, traceID string, workspaceID identity.WorkspaceID, operationID identity.ProductionOperationID) string {
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if !validProductionIdempotencyKey(idempotencyKey) {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "Idempotency-Key header must be between 8 and 128 characters", traceID, false)
		return "INVALID_ARGUMENT"
	}

	actor := principalRef(request)
	if actor == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", traceID, false)
		return "UNAUTHENTICATED"
	}
	principalID, err := identity.ParsePrincipalID(actor)
	if err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid principal", traceID, false)
		return "INVALID_ARGUMENT"
	}

	// Authorize propose
	if err := handler.checkProductionAuthoringAccess(request.Context(), actor, workspaceID); err != nil {
		return writeProductionError(response, err, traceID)
	}

	var body ReplaceProductionRequestPayload
	if err := decodeStrictJSON(response, request, &body); err != nil {
		return writeProductionError(response, err, traceID)
	}

	var targets []domain.TargetDeclaration
	for _, t := range body.Targets {
		targets = append(targets, domain.TargetDeclaration{
			LocalKey:             t.LocalKey,
			Title:                t.Title,
			TargetID:             t.TargetID,
			EvidenceIDs:          t.EvidenceIDs,
			Kind:                 t.Kind,
			Intent:               t.Intent,
			IdentityKey:          t.IdentityKey,
			BaseRevisionID:       t.BaseRevisionID,
			BaseObjectVersion:    t.BaseObjectVersion,
			RegistryWriteVersion: t.RegistryWriteVersion,
			Content:              t.Content,
			ReuseIdentity:        t.ReuseIdentity,
			Changes:              t.Changes,
		})
	}

	var candidates []domain.CandidateDeclaration
	for _, c := range body.Input.Candidates {
		candidates = append(candidates, domain.CandidateDeclaration{
			CandidateID:      c.CandidateID,
			CandidateDigest:  c.Digest,
			TargetKeys:       c.TargetKeys,
			PrimaryTargetKey: c.PrimaryTargetKey,
		})
	}

	var inputSnapshotID *identity.SourceSnapshotID
	if len(body.Input.Snapshots) > 0 {
		sid, parseErr := identity.ParseSourceSnapshotID(body.Input.Snapshots[0].SnapshotID)
		if parseErr == nil {
			inputSnapshotID = &sid
		}
	}

	scopeBytes, _ := json.Marshal(body.Input)

	var suggestionRunID *identity.AgentRunID
	if body.SuggestionRunID != nil {
		rid, parseErr := identity.ParseAgentRunID(*body.SuggestionRunID)
		if parseErr != nil {
			return writeProductionError(response, domain.ErrInvalidArgument, traceID)
		}
		suggestionRunID = &rid
	}

	cmd := governanceapp.ReplaceDraftCommand{
		WorkspaceID:     workspaceID,
		PrincipalID:     principalID,
		OperationID:     operationID,
		ExpectedVersion: body.ExpectedVersion,
		IdempotencyKey:  idempotencyKey,
		InputSnapshotID: inputSnapshotID,
		InputScope:      scopeBytes,
		Targets:         targets,
		Candidates:      candidates,
		SuggestionRunID: suggestionRunID,
		TraceID:         traceID,
	}

	result, err := handler.production.ReplaceDraft(request.Context(), cmd)
	if err != nil {
		return writeProductionError(response, err, traceID)
	}

	proposalIDs := []string{}
	for _, t := range result.Targets {
		if t.ProposalID != nil {
			proposalIDs = append(proposalIDs, t.ProposalID.String())
		}
	}

	cmdResult := ProductionCommandResultResponse{
		OperationID:         result.OperationID.String(),
		Version:             result.Version,
		SetDigest:           result.SetDigest,
		Replayed:            result.Replayed,
		Outcome:             "updated",
		ProposalIDs:         proposalIDs,
		ValidationAttemptNo: nil,
		ValidationRunIDs:    []string{},
		ReviewIDs:           []string{},
	}

	writeJSON(response, http.StatusOK, cmdResult)
	return ""
}

func (handler *Handler) getProductionOperation(response http.ResponseWriter, request *http.Request, traceID string, workspaceID identity.WorkspaceID, operationID identity.ProductionOperationID) string {
	actor := principalRef(request)
	if actor == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", traceID, false)
		return "UNAUTHENTICATED"
	}

	if err := handler.checkProductionAuthoringAccess(request.Context(), actor, workspaceID); err != nil {
		return writeProductionError(response, err, traceID)
	}

	op, ver, targets, _, _, err := handler.production.GetOperation(request.Context(), workspaceID, operationID)
	if err != nil {
		return writeProductionError(response, err, traceID)
	}

	versionParam := request.URL.Query().Get("version")
	if versionParam != "" {
		reqVer, parseErr := strconv.Atoi(versionParam)
		if parseErr != nil || reqVer < 1 {
			writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid version query parameter", traceID, false)
			return "INVALID_ARGUMENT"
		}
		if reqVer != ver.Version {
			op, ver, targets, _, _, err = handler.production.GetOperationVersion(request.Context(), workspaceID, operationID, reqVer)
			if err != nil {
				return writeProductionError(response, err, traceID)
			}
		}
	}

	var targetResults []TargetResultResponse
	principal, parseErr := identity.ParsePrincipalID(actor)
	if parseErr != nil {
		return writeProductionError(response, domain.ErrInvalidArgument, traceID)
	}
	if err := handler.production.AuthorizeOperationRead(request.Context(), principal, *ver, targets); err != nil {
		return writeProductionError(response, err, traceID)
	}
	for _, t := range targets {
		var propIDStr *string
		propState := t.ProposalState
		if t.ProposalID != nil {
			s := t.ProposalID.String()
			propIDStr = &s
		}
		declaration := ProductionTargetPayload{}
		if d := t.Declaration; d != nil {
			declaration = ProductionTargetPayload{Intent: d.Intent, Kind: d.Kind, LocalKey: d.LocalKey,
				ReuseIdentity: d.ReuseIdentity,
				IdentityKey:   d.IdentityKey, TargetID: d.TargetID, BaseRevisionID: d.BaseRevisionID,
				BaseObjectVersion: d.BaseObjectVersion, RegistryWriteVersion: d.RegistryWriteVersion,
				Title: d.Title, Content: d.Content, Changes: d.Changes, EvidenceIDs: d.EvidenceIDs}
		}
		targetResults = append(targetResults, TargetResultResponse{
			LocalKey:         t.LocalKey,
			TargetID:         t.TargetID,
			Declaration:      declaration,
			Outcome:          t.Outcome,
			ContentDigest:    t.ContentDigest,
			ProposalID:       propIDStr,
			ProposalState:    propState,
			ValidationRunIDs: append([]string{}, t.ValidationRunIDs...),
			ReviewIDs:        append([]string{}, t.ReviewIDs...),
		})
	}

	var inputPayload ProductionInputPayload
	var storedInput struct {
		Scope json.RawMessage `json:"scope"`
	}
	if err := json.Unmarshal(ver.InputJSON, &storedInput); err != nil {
		return writeProductionError(response, err, traceID)
	}
	inputBytes := ver.InputJSON
	if len(storedInput.Scope) > 0 {
		inputBytes = storedInput.Scope
	}
	if err := json.Unmarshal(inputBytes, &inputPayload); err != nil {
		return writeProductionError(response, err, traceID)
	}

	progress := productionProgress(*ver, targets)
	activeValidation, unresolved := productionValidationRecovery(*ver)
	var releaseID *string
	if ver.ReleaseID != nil {
		value := ver.ReleaseID.String()
		releaseID = &value
	}

	res := ProductionOperationResponse{
		Summary: OperationSummaryResponse{
			ID:             op.ID.String(),
			CurrentVersion: op.CurrentVersion,
			CreatedBy:      op.CreatedBy.String(),
			CreatedAt:      op.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:      op.UpdatedAt.UTC().Format(time.RFC3339),
			Frozen:         ver.FrozenAt != nil,
			Progress:       progress,
			TargetCount:    len(targets),
			ReleaseID:      releaseID,
		},
		Version:                ver.Version,
		Input:                  inputPayload,
		InputDigest:            ver.InputDigest,
		SetDigest:              ver.SetDigest,
		BaselineHead:           HeadReferenceResponse{Presence: ver.BaselineHead.Presence, ReleaseID: ver.BaselineHead.ReleaseID, ManifestDigest: ver.BaselineHead.ManifestDigest},
		Targets:                targetResults,
		ActiveValidation:       activeValidation,
		GenerationRunIDs:       []string{},
		GenerationApplications: []any{},
		UnresolvedCodes:        unresolved,
	}
	reader, err := identity.ParsePrincipalID(actor)
	if err != nil {
		return writeProductionError(response, domain.ErrInvalidArgument, traceID)
	}
	runs, applications, err := handler.production.GenerationHistory(request.Context(), workspaceID, reader, operationID, ver.Version)
	if err != nil {
		return writeProductionError(response, err, traceID)
	}
	res.GenerationRunIDs = runs
	for _, application := range applications {
		res.GenerationApplications = append(res.GenerationApplications, application)
	}

	return writeProductionRecovery(response, res, traceID)
}

func (handler *Handler) listProductionOperations(response http.ResponseWriter, request *http.Request, traceID string, workspaceID identity.WorkspaceID) string {
	actor := principalRef(request)
	if actor == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", traceID, false)
		return "UNAUTHENTICATED"
	}

	if err := handler.checkProductionAuthoringAccess(request.Context(), actor, workspaceID); err != nil {
		return writeProductionError(response, err, traceID)
	}

	limit := 50
	if l := request.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed >= 1 && parsed <= 200 {
			limit = parsed
		} else {
			writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "limit must be between 1 and 200", traceID, false)
			return "INVALID_ARGUMENT"
		}
	}

	cursorStr := request.URL.Query().Get("cursor")
	query := domain.ProductionListQuery{Limit: limit + 1, CreatedBy: request.URL.Query().Get("createdBy"), SourceID: request.URL.Query().Get("sourceId"), CandidateID: request.URL.Query().Get("candidateId")}
	if query.CreatedBy != "" {
		if _, err := identity.ParsePrincipalID(query.CreatedBy); err != nil {
			return writeProductionError(response, domain.ErrInvalidArgument, traceID)
		}
	}
	if query.SourceID != "" {
		if _, err := identity.ParseSourceConnectionID(query.SourceID); err != nil {
			return writeProductionError(response, domain.ErrInvalidArgument, traceID)
		}
	}
	if query.CandidateID != "" {
		if _, err := identity.ParseSemanticCandidateID(query.CandidateID); err != nil {
			return writeProductionError(response, domain.ErrInvalidArgument, traceID)
		}
	}
	cursor := productionCursorData{Workspace: workspaceID.String(), Principal: actor, CreatedBy: query.CreatedBy, SourceID: query.SourceID, CandidateID: query.CandidateID}
	if cursorStr != "" {
		c, err := decodeProductionCursor(cursorStr, workspaceID.String())
		if err != nil || c.Principal != actor || c.CreatedBy != query.CreatedBy || c.SourceID != query.SourceID || c.CandidateID != query.CandidateID {
			writeError(response, http.StatusBadRequest, "INVALID_CURSOR", "invalid pagination cursor", traceID, false)
			return "INVALID_CURSOR"
		}
		cursor = c
		query.WatermarkTime, query.WatermarkID = c.WatermarkTime, c.WatermarkID
		query.AfterTime, query.AfterID = c.AfterTime, c.AfterID
	}

	ops, err := handler.production.ListOperationsPage(request.Context(), workspaceID, query)
	if err != nil {
		return writeProductionError(response, err, traceID)
	}

	var items []OperationSummaryResponse
	hasMore := false
	if len(ops) > limit {
		hasMore = true
		ops = ops[:limit]
	}
	if cursor.WatermarkID == "" && len(ops) > 0 {
		cursor.WatermarkTime, cursor.WatermarkID = ops[0].CreatedAt, ops[0].ID.String()
	}

	for _, op := range ops {
		cursor.AfterTime, cursor.AfterID = op.CreatedAt, op.ID.String()
		_, ver, targets, _, _, err := handler.production.GetOperationVersion(request.Context(), workspaceID, op.ID, op.CurrentVersion)
		if err != nil {
			return writeProductionError(response, err, traceID)
		}
		principal, err := identity.ParsePrincipalID(actor)
		if err != nil {
			return writeProductionError(response, domain.ErrInvalidArgument, traceID)
		}
		if err := handler.production.AuthorizeOperationRead(request.Context(), principal, *ver, targets); err != nil {
			var denial *authz.DenialError
			if errors.As(err, &denial) || errors.Is(err, domain.ErrNotFound) {
				continue
			}
			return writeProductionError(response, err, traceID)
		}
		var releaseID *string
		if ver.ReleaseID != nil {
			value := ver.ReleaseID.String()
			releaseID = &value
		}
		items = append(items, OperationSummaryResponse{
			ID:             op.ID.String(),
			CurrentVersion: op.CurrentVersion,
			CreatedBy:      op.CreatedBy.String(),
			CreatedAt:      op.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:      op.UpdatedAt.UTC().Format(time.RFC3339),
			Frozen:         ver.FrozenAt != nil,
			Progress:       productionProgress(*ver, targets),
			TargetCount:    len(targets),
			ReleaseID:      releaseID,
		})
	}

	if items == nil {
		items = []OperationSummaryResponse{}
	}

	var nextCursor *string
	if hasMore {
		nc := encodeProductionCursor(cursor)
		nextCursor = &nc
	}

	return writeProductionRecovery(response, ProductionOperationPageResponse{
		Items:      items,
		NextCursor: nextCursor,
	}, traceID)
}

type productionCursorData struct {
	Workspace     string    `json:"w"`
	Principal     string    `json:"p"`
	CreatedBy     string    `json:"a"`
	SourceID      string    `json:"s"`
	CandidateID   string    `json:"c"`
	WatermarkTime time.Time `json:"u"`
	WatermarkID   string    `json:"i"`
	AfterTime     time.Time `json:"t"`
	AfterID       string    `json:"l"`
}

// Process-local authentication and encryption prevents a cursor from exposing
// identifiers of rows omitted by scope filtering. Restart invalidates cursors.
var productionCursorCipher = func() cipher.AEAD {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return aead
}()

func encodeProductionCursor(data productionCursorData) string {
	raw, _ := json.Marshal(data)
	nonce := make([]byte, productionCursorCipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	sealed := productionCursorCipher.Seal(nonce, nonce, raw, []byte("production-operations/v1"))
	return base64.RawURLEncoding.EncodeToString(sealed)
}

func decodeProductionCursor(token, expectedWorkspace string) (productionCursorData, error) {
	if len(token) > 2048 {
		return productionCursorData{}, errors.New("cursor too long")
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return productionCursorData{}, err
	}
	n := productionCursorCipher.NonceSize()
	if len(sealed) < n+productionCursorCipher.Overhead() {
		return productionCursorData{}, errors.New("invalid cursor")
	}
	raw, err := productionCursorCipher.Open(nil, sealed[:n], sealed[n:], []byte("production-operations/v1"))
	if err != nil {
		return productionCursorData{}, err
	}
	var data productionCursorData
	if err := json.Unmarshal(raw, &data); err != nil {
		return productionCursorData{}, err
	}
	if data.Workspace != expectedWorkspace || data.WatermarkID == "" || data.AfterID == "" || data.WatermarkTime.IsZero() || data.AfterTime.IsZero() {
		return productionCursorData{}, errors.New("workspace mismatch")
	}
	return data, nil
}

func decodeStrictJSON(response http.ResponseWriter, request *http.Request, target any) error {
	contentType := request.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		return fmt.Errorf("%w: Content-Type must be application/json", domain.ErrInvalidArgument)
	}

	limited := io.LimitReader(request.Body, 1024*1024+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(raw) > 1024*1024 {
		return fmt.Errorf("%w: body size exceeds 1 MiB limit", domain.ErrLimitExceeded)
	}
	if !utf8.Valid(raw) {
		return fmt.Errorf("%w: invalid UTF-8", domain.ErrInvalidArgument)
	}

	if err := validateJSONDepthAndNoDuplicates(raw, 32); err != nil {
		return err
	}
	switch target.(type) {
	case *CreateProductionRequestPayload, *ReplaceProductionRequestPayload:
		if err := validateProductionRequestShape(raw); err != nil {
			return err
		}
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidArgument, err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("%w: trailing characters after JSON object", domain.ErrInvalidArgument)
	}
	return nil
}

func validateJSONDepthAndNoDuplicates(raw []byte, maxDepth int) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	type frame struct {
		isObject  bool
		expectVal bool
		keys      map[string]bool
	}
	var stack []frame

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: %v", domain.ErrInvalidArgument, err)
		}

		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				if len(stack) >= maxDepth {
					return fmt.Errorf("%w: nesting depth exceeds %d", domain.ErrLimitExceeded, maxDepth)
				}
				if len(stack) > 0 && stack[len(stack)-1].isObject {
					stack[len(stack)-1].expectVal = false
				}
				stack = append(stack, frame{isObject: true, keys: make(map[string]bool)})
			case '[':
				if len(stack) >= maxDepth {
					return fmt.Errorf("%w: nesting depth exceeds %d", domain.ErrLimitExceeded, maxDepth)
				}
				if len(stack) > 0 && stack[len(stack)-1].isObject {
					stack[len(stack)-1].expectVal = false
				}
				stack = append(stack, frame{isObject: false})
			case '}', ']':
				if len(stack) == 0 {
					return fmt.Errorf("%w: unbalanced closing delimiter", domain.ErrInvalidArgument)
				}
				stack = stack[:len(stack)-1]
				if len(stack) > 0 && stack[len(stack)-1].isObject {
					stack[len(stack)-1].expectVal = false
				}
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].isObject {
				top := &stack[len(stack)-1]
				if !top.expectVal {
					if top.keys[t] {
						return fmt.Errorf("%w: duplicate key %q", domain.ErrInvalidArgument, t)
					}
					top.keys[t] = true
					top.expectVal = true
				} else {
					top.expectVal = false
				}
			}
		default:
			if len(stack) > 0 && stack[len(stack)-1].isObject {
				stack[len(stack)-1].expectVal = false
			}
		}
	}
	return nil
}

func writeProductionError(response http.ResponseWriter, err error, traceID string) string {
	var denial *authz.DenialError
	switch {
	case errors.As(err, &denial):
		writeError(response, http.StatusForbidden, string(denial.Decision.ReasonCode), "the acting principal lacks the required capability", traceID, false)
		return string(denial.Decision.ReasonCode)
	case errors.Is(err, domain.ErrAlreadyProduced):
		var existing *domain.AlreadyProducedError
		if errors.As(err, &existing) {
			writeErrorWithDetails(response, http.StatusConflict, "ALREADY_PRODUCED", "production operation already produced", traceID, map[string]any{"operationId": existing.OperationID.String()})
		} else {
			writeError(response, http.StatusConflict, "ALREADY_PRODUCED", "production operation already produced", traceID, false)
		}
		return "ALREADY_PRODUCED"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		writeError(response, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key reused with different request payload", traceID, false)
		return "IDEMPOTENCY_CONFLICT"
	case errors.Is(err, domain.ErrIdentityConflict):
		writeError(response, http.StatusConflict, "IDENTITY_CONFLICT", "production identity conflict", traceID, false)
		return "IDENTITY_CONFLICT"
	case errors.Is(err, domain.ErrSodConflict):
		writeError(response, http.StatusForbidden, "SOD_CONFLICT", err.Error(), traceID, false)
		return "SOD_CONFLICT"
	case errors.Is(err, domain.ErrVersionConflict):
		writeError(response, http.StatusConflict, "VERSION_CONFLICT", err.Error(), traceID, false)
		return "VERSION_CONFLICT"
	case errors.Is(err, domain.ErrBaselineConflict):
		writeError(response, http.StatusConflict, "BASELINE_CONFLICT", err.Error(), traceID, false)
		return "BASELINE_CONFLICT"
	case errors.Is(err, domain.ErrHeadConflict):
		writeError(response, http.StatusConflict, "HEAD_CONFLICT", err.Error(), traceID, false)
		return "HEAD_CONFLICT"
	case errors.Is(err, domain.ErrInputStale):
		writeError(response, http.StatusConflict, "INPUT_STALE", err.Error(), traceID, false)
		return "INPUT_STALE"
	case errors.Is(err, domain.ErrProductionSetRequired):
		writeError(response, http.StatusConflict, "PRODUCTION_SET_REQUIRED", err.Error(), traceID, false)
		return "PRODUCTION_SET_REQUIRED"
	case errors.Is(err, domain.ErrValidationRequired):
		writeError(response, http.StatusUnprocessableEntity, "VALIDATION_REQUIRED", err.Error(), traceID, false)
		return "VALIDATION_REQUIRED"
	case errors.Is(err, domain.ErrReviewRequired):
		writeError(response, http.StatusUnprocessableEntity, "REVIEW_REQUIRED", err.Error(), traceID, false)
		return "REVIEW_REQUIRED"
	case errors.Is(err, domain.ErrPriorStateUnknown):
		writeError(response, http.StatusUnprocessableEntity, "PRIOR_STATE_UNKNOWN", err.Error(), traceID, false)
		return "PRIOR_STATE_UNKNOWN"
	case errors.Is(err, domain.ErrNoSubstantiveChange):
		writeError(response, http.StatusUnprocessableEntity, "NO_SUBSTANTIVE_CHANGE", err.Error(), traceID, false)
		return "NO_SUBSTANTIVE_CHANGE"
	case errors.Is(err, domain.ErrEvidenceMissing):
		writeError(response, http.StatusUnprocessableEntity, "EVIDENCE_MISSING", err.Error(), traceID, false)
		return "EVIDENCE_MISSING"
	case errors.Is(err, domain.ErrDependencyInvalid):
		writeError(response, http.StatusUnprocessableEntity, "DEPENDENCY_INVALID", err.Error(), traceID, false)
		return "DEPENDENCY_INVALID"
	case errors.Is(err, domain.ErrContentMismatch):
		writeError(response, http.StatusUnprocessableEntity, "CONTENT_MISMATCH", "desired target content does not match replay", traceID, false)
		return "CONTENT_MISMATCH"
	case errors.Is(err, domain.ErrInputIncomplete):
		writeError(response, http.StatusUnprocessableEntity, "INPUT_INCOMPLETE", "production input incomplete", traceID, false)
		return "INPUT_INCOMPLETE"
	case errors.Is(err, domain.ErrLimitExceeded):
		writeError(response, http.StatusBadRequest, "LIMIT_EXCEEDED", "production aggregate limit exceeded", traceID, false)
		return "LIMIT_EXCEEDED"
	case errors.Is(err, domain.ErrInvalidArgument):
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "the request is invalid", traceID, false)
		return "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrNotFound):
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the requested resource was not found", traceID, false)
		return "NOT_FOUND"
	case errors.Is(err, domain.ErrConflict):
		writeError(response, http.StatusConflict, "CONFLICT", "production version or state conflict", traceID, false)
		return "CONFLICT"
	default:
		slog.Error("production unhandled error", "error", err, "trace_id", traceID)
		writeError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "the request could not be completed", traceID, false)
		return "INTERNAL_ERROR"
	}
}

// SP-T004 Request Payloads
type SubmitProductionRequestPayload struct {
	ExpectedVersion int    `json:"expectedVersion"`
	SetDigest       string `json:"setDigest"`
}

type ValidateProductionRequestPayload struct {
	ExpectedVersion   int    `json:"expectedVersion"`
	SetDigest         string `json:"setDigest"`
	PreviousAttemptNo int    `json:"previousAttemptNo"`
	Reason            string `json:"reason"`
}

type ReviewProductionRequestPayload struct {
	ExpectedVersion int                        `json:"expectedVersion"`
	SetDigest       string                     `json:"setDigest"`
	Validation      ValidationReferencePayload `json:"validation"`
	ProposalIDs     []string                   `json:"proposalIds"`
	Decision        string                     `json:"decision"`
	Note            string                     `json:"note"`
}

type ValidationReferencePayload struct {
	AttemptNo        int    `json:"attemptNo"`
	ValidationDigest string `json:"validationDigest"`
}

type PublishProductionRequestPayload struct {
	ExpectedVersion int                        `json:"expectedVersion"`
	SetDigest       string                     `json:"setDigest"`
	Validation      ValidationReferencePayload `json:"validation"`
	ExpectedHead    HeadReferencePayload       `json:"expectedHead"`
}

type HeadReferencePayload struct {
	Presence       string  `json:"presence"`
	ReleaseID      *string `json:"releaseId,omitempty"`
	ManifestDigest *string `json:"manifestDigest,omitempty"`
}

type RollbackProductionRequestPayload struct {
	ExpectedVersion int                  `json:"expectedVersion"`
	SetDigest       string               `json:"setDigest"`
	ExpectedHead    HeadReferencePayload `json:"expectedHead"`
	Reason          string               `json:"reason"`
}

type ReleaseCommandResultResponse struct {
	ReleaseID      string `json:"releaseId"`
	OperationID    string `json:"operationId"`
	Replayed       bool   `json:"replayed"`
	ManifestDigest string `json:"manifestDigest"`
}

type ValidationAttemptPageResponse struct {
	OperationID string                            `json:"operationId"`
	Version     int                               `json:"version"`
	Items       []ValidationAttemptResultResponse `json:"items"`
	NextCursor  *string                           `json:"nextCursor"`
}

type ValidationAttemptResultResponse struct {
	AttemptNo            int                             `json:"attemptNo"`
	Status               string                          `json:"status"`
	SetDigest            string                          `json:"setDigest"`
	FreshnessDigest      string                          `json:"freshnessDigest"`
	RequiredChecksDigest string                          `json:"requiredChecksDigest"`
	ValidationDigest     *string                         `json:"validationDigest"`
	RunIDs               []string                        `json:"runIds"`
	Checks               []ValidationCheckResultResponse `json:"checks"`
	CompletedAt          *string                         `json:"completedAt"`
}

type ValidationCheckResultResponse struct {
	RunID            string                        `json:"runId"`
	ProposalID       string                        `json:"proposalId"`
	ValidatorID      string                        `json:"validatorId"`
	ValidatorVersion string                        `json:"validatorVersion"`
	Status           string                        `json:"status"`
	Results          []ValidationCheckItemResponse `json:"results"`
}

type ValidationCheckItemResponse struct {
	Severity    string         `json:"severity"`
	Code        string         `json:"code"`
	Message     string         `json:"message"`
	InputDigest string         `json:"inputDigest"`
	Details     map[string]any `json:"details"`
}

type ProductionReleaseResponse struct {
	ID                  string                              `json:"id"`
	Sequence            int64                               `json:"sequence"`
	PublishedBy         string                              `json:"publishedBy"`
	PublishedAt         string                              `json:"publishedAt"`
	Attribution         ReleaseAttributionResponse          `json:"attribution"`
	Protection          ProductionReleaseProtectionResponse `json:"protection"`
	BeforeHead          HeadReferencePayload                `json:"beforeHead"`
	BeforeManifest      ManifestResponse                    `json:"beforeManifest"`
	AfterManifest       ManifestResponse                    `json:"afterManifest"`
	BeforePins          []BeforePinResponse                 `json:"beforePins"`
	OriginProposalID    *string                             `json:"originProposalId"`
	OriginProposalIDs   []string                            `json:"originProposalIds"`
	RolledBackReleaseID *string                             `json:"rolledBackReleaseId"`
	ProjectionStatus    string                              `json:"projectionStatus"`
}

type ReleaseAttributionResponse struct {
	OperationID  string                     `json:"operationId"`
	Version      int                        `json:"version"`
	SetDigest    string                     `json:"setDigest"`
	Validation   ValidationReferencePayload `json:"validation"`
	ReviewIDs    []string                   `json:"reviewIds"`
	ProposalIDs  []string                   `json:"proposalIds"`
	Contributors []string                   `json:"contributors"`
	Role         string                     `json:"role"`
}

type ProductionReleaseProtectionResponse struct {
	RootReleaseID           string  `json:"rootReleaseId"`
	RollbackDepth           int     `json:"rollbackDepth"`
	RollbackParentReleaseID *string `json:"rollbackParentReleaseId,omitempty"`
}

type ManifestResponse struct {
	Assets  []ManifestAssetResponse  `json:"assets"`
	Objects []ManifestObjectResponse `json:"objects"`
	Digest  string                   `json:"digest"`
}

type ManifestAssetResponse struct {
	AssetID       string         `json:"assetId"`
	RevisionID    string         `json:"revisionId"`
	Position      int            `json:"position"`
	Compatibility map[string]any `json:"compatibility"`
}

type ManifestObjectResponse struct {
	Kind          string  `json:"kind"`
	TargetID      string  `json:"targetId"`
	ObjectVersion int     `json:"objectVersion"`
	ContentDigest *string `json:"contentDigest,omitempty"`
	Position      int     `json:"position"`
}

type BeforePinResponse struct {
	Kind          string  `json:"kind"`
	TargetID      string  `json:"targetId"`
	Presence      string  `json:"presence"`
	RevisionID    *string `json:"revisionId,omitempty"`
	ObjectVersion *int    `json:"objectVersion,omitempty"`
	ContentDigest *string `json:"contentDigest,omitempty"`
}

func (handler *Handler) submitProductionOperation(response http.ResponseWriter, request *http.Request, traceID string, workspaceID identity.WorkspaceID, operationID identity.ProductionOperationID) string {
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "Idempotency-Key header must be between 8 and 128 characters", traceID, false)
		return "INVALID_ARGUMENT"
	}

	actor := principalRef(request)
	if actor == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", traceID, false)
		return "UNAUTHENTICATED"
	}
	principalID, err := identity.ParsePrincipalID(actor)
	if err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid principal", traceID, false)
		return "INVALID_ARGUMENT"
	}

	if err := handler.checkProductionAuth(request.Context(), actor, workspaceID, authz.ActionAssetPropose, traceID); err != nil {
		return writeProductionError(response, err, traceID)
	}
	if err := handler.checkProductionAuth(request.Context(), actor, workspaceID, authz.ActionValidationRun, traceID); err != nil {
		return writeProductionError(response, err, traceID)
	}

	var body SubmitProductionRequestPayload
	if err := decodeStrictJSON(response, request, &body); err != nil {
		return writeProductionError(response, err, traceID)
	}

	cmd := governanceapp.SubmitOperationCommand{
		WorkspaceID:     workspaceID,
		PrincipalID:     principalID,
		OperationID:     operationID,
		ExpectedVersion: body.ExpectedVersion,
		SetDigest:       body.SetDigest,
		IdempotencyKey:  idempotencyKey,
		TraceID:         traceID,
	}

	result, err := handler.production.SubmitOperation(request.Context(), cmd)
	if err != nil {
		return writeProductionError(response, err, traceID)
	}

	var propIDs []string
	for _, pid := range result.ProposalIDs {
		propIDs = append(propIDs, pid.String())
	}
	var runIDs []string
	for _, rid := range result.ValidationRunIDs {
		runIDs = append(runIDs, rid.String())
	}

	cmdResult := ProductionCommandResultResponse{
		OperationID:         result.OperationID.String(),
		Version:             result.Version,
		SetDigest:           result.SetDigest,
		Replayed:            result.Replayed,
		Outcome:             result.Outcome,
		ProposalIDs:         propIDs,
		ValidationAttemptNo: result.ValidationAttemptNo,
		ValidationRunIDs:    runIDs,
		ReviewIDs:           []string{},
	}

	if result.Replayed {
		writeJSON(response, http.StatusOK, cmdResult)
		return ""
	}

	location := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s", workspaceID.String(), result.OperationID.String())
	response.Header().Set("Location", location)
	writeJSON(response, http.StatusAccepted, cmdResult)
	return ""
}

func (handler *Handler) listProductionValidationAttempts(response http.ResponseWriter, request *http.Request, traceID string, workspaceID identity.WorkspaceID, operationID identity.ProductionOperationID) string {
	return handler.productionValidationHistory(response, request, traceID, workspaceID, operationID)
}

func (handler *Handler) validateProductionOperation(response http.ResponseWriter, request *http.Request, traceID string, workspaceID identity.WorkspaceID, operationID identity.ProductionOperationID) string {
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "Idempotency-Key header must be between 8 and 128 characters", traceID, false)
		return "INVALID_ARGUMENT"
	}

	actor := principalRef(request)
	if actor == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", traceID, false)
		return "UNAUTHENTICATED"
	}
	principalID, err := identity.ParsePrincipalID(actor)
	if err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid principal", traceID, false)
		return "INVALID_ARGUMENT"
	}

	if err := handler.checkProductionAuth(request.Context(), actor, workspaceID, authz.ActionValidationRun, traceID); err != nil {
		return writeProductionError(response, err, traceID)
	}

	var body ValidateProductionRequestPayload
	if err := decodeStrictJSON(response, request, &body); err != nil {
		return writeProductionError(response, err, traceID)
	}

	cmd := governanceapp.ValidateOperationCommand{
		WorkspaceID:       workspaceID,
		PrincipalID:       principalID,
		OperationID:       operationID,
		ExpectedVersion:   body.ExpectedVersion,
		SetDigest:         body.SetDigest,
		PreviousAttemptNo: body.PreviousAttemptNo,
		Reason:            body.Reason,
		IdempotencyKey:    idempotencyKey,
		TraceID:           traceID,
	}

	result, err := handler.production.ValidateOperation(request.Context(), cmd)
	if err != nil {
		return writeProductionError(response, err, traceID)
	}

	var propIDs []string
	for _, pid := range result.ProposalIDs {
		propIDs = append(propIDs, pid.String())
	}
	var runIDs []string
	for _, rid := range result.ValidationRunIDs {
		runIDs = append(runIDs, rid.String())
	}

	cmdResult := ProductionCommandResultResponse{
		OperationID:         result.OperationID.String(),
		Version:             result.Version,
		SetDigest:           result.SetDigest,
		Replayed:            result.Replayed,
		Outcome:             result.Outcome,
		ProposalIDs:         propIDs,
		ValidationAttemptNo: result.ValidationAttemptNo,
		ValidationRunIDs:    runIDs,
		ReviewIDs:           []string{},
	}

	if result.Replayed {
		writeJSON(response, http.StatusOK, cmdResult)
		return ""
	}

	location := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s", workspaceID.String(), result.OperationID.String())
	response.Header().Set("Location", location)
	writeJSON(response, http.StatusAccepted, cmdResult)
	return ""
}

func (handler *Handler) reviewProductionOperation(response http.ResponseWriter, request *http.Request, traceID string, workspaceID identity.WorkspaceID, operationID identity.ProductionOperationID) string {
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "Idempotency-Key header must be between 8 and 128 characters", traceID, false)
		return "INVALID_ARGUMENT"
	}

	actor := principalRef(request)
	if actor == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", traceID, false)
		return "UNAUTHENTICATED"
	}
	principalID, err := identity.ParsePrincipalID(actor)
	if err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid principal", traceID, false)
		return "INVALID_ARGUMENT"
	}

	if err := handler.checkProductionAuth(request.Context(), actor, workspaceID, authz.ActionProposalReview, traceID); err != nil {
		return writeProductionError(response, err, traceID)
	}

	var body ReviewProductionRequestPayload
	if err := decodeStrictJSON(response, request, &body); err != nil {
		return writeProductionError(response, err, traceID)
	}

	var proposalIDs []identity.ProposalID
	for _, s := range body.ProposalIDs {
		pID, err := identity.ParseProposalID(s)
		if err != nil {
			writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid proposal id: "+s, traceID, false)
			return "INVALID_ARGUMENT"
		}
		proposalIDs = append(proposalIDs, pID)
	}

	cmd := governanceapp.ReviewOperationCommand{
		WorkspaceID:         workspaceID,
		PrincipalID:         principalID,
		OperationID:         operationID,
		ExpectedVersion:     body.ExpectedVersion,
		SetDigest:           body.SetDigest,
		ValidationAttemptNo: body.Validation.AttemptNo,
		ValidationDigest:    body.Validation.ValidationDigest,
		ProposalIDs:         proposalIDs,
		Decision:            body.Decision,
		Note:                body.Note,
		IdempotencyKey:      idempotencyKey,
		TraceID:             traceID,
	}

	result, err := handler.production.ReviewOperation(request.Context(), cmd)
	if err != nil {
		return writeProductionError(response, err, traceID)
	}

	var propIDs []string
	for _, pid := range result.ProposalIDs {
		propIDs = append(propIDs, pid.String())
	}
	var revIDs []string
	for _, rid := range result.ReviewIDs {
		revIDs = append(revIDs, rid.String())
	}

	cmdResult := ProductionCommandResultResponse{
		OperationID:         result.OperationID.String(),
		Version:             result.Version,
		SetDigest:           result.SetDigest,
		Replayed:            result.Replayed,
		Outcome:             result.Outcome,
		ProposalIDs:         propIDs,
		ValidationAttemptNo: result.ValidationAttemptNo,
		ValidationRunIDs:    []string{},
		ReviewIDs:           revIDs,
	}

	if result.Replayed {
		writeJSON(response, http.StatusOK, cmdResult)
		return ""
	}

	location := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s", workspaceID.String(), result.OperationID.String())
	response.Header().Set("Location", location)
	writeJSON(response, http.StatusCreated, cmdResult)
	return ""
}

func (handler *Handler) publishProductionOperation(response http.ResponseWriter, request *http.Request, traceID string, workspaceID identity.WorkspaceID, operationID identity.ProductionOperationID) string {
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "Idempotency-Key header must be between 8 and 128 characters", traceID, false)
		return "INVALID_ARGUMENT"
	}

	actor := principalRef(request)
	if actor == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", traceID, false)
		return "UNAUTHENTICATED"
	}
	principalID, err := identity.ParsePrincipalID(actor)
	if err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid principal", traceID, false)
		return "INVALID_ARGUMENT"
	}

	if err := handler.checkProductionAuth(request.Context(), actor, workspaceID, authz.ActionReleasePublish, traceID); err != nil {
		return writeProductionError(response, err, traceID)
	}

	var body PublishProductionRequestPayload
	if err := decodeStrictJSON(response, request, &body); err != nil {
		return writeProductionError(response, err, traceID)
	}

	var expectedRelID *identity.ReleaseID
	if body.ExpectedHead.ReleaseID != nil && *body.ExpectedHead.ReleaseID != "" {
		rid, err := identity.ParseReleaseID(*body.ExpectedHead.ReleaseID)
		if err != nil {
			writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid expectedHead releaseId", traceID, false)
			return "INVALID_ARGUMENT"
		}
		expectedRelID = &rid
	}

	headRef := domain.HeadReference{
		Presence:       body.ExpectedHead.Presence,
		ReleaseID:      expectedRelID,
		ManifestDigest: body.ExpectedHead.ManifestDigest,
	}

	cmd := governanceapp.PublishOperationCommand{
		WorkspaceID:         workspaceID,
		PrincipalID:         principalID,
		OperationID:         operationID,
		ExpectedVersion:     body.ExpectedVersion,
		SetDigest:           body.SetDigest,
		ValidationAttemptNo: body.Validation.AttemptNo,
		ValidationDigest:    body.Validation.ValidationDigest,
		ExpectedHead:        headRef,
		IdempotencyKey:      idempotencyKey,
		TraceID:             traceID,
	}

	result, err := handler.production.PublishOperation(request.Context(), cmd)
	if err != nil {
		return writeProductionError(response, err, traceID)
	}

	resp := ReleaseCommandResultResponse{
		ReleaseID:      result.ReleaseID.String(),
		OperationID:    result.OperationID.String(),
		Replayed:       result.Replayed,
		ManifestDigest: result.ManifestDigest,
	}

	if result.Replayed {
		writeJSON(response, http.StatusOK, resp)
		return ""
	}

	location := fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s", workspaceID.String(), result.ReleaseID.String())
	response.Header().Set("Location", location)
	writeJSON(response, http.StatusCreated, resp)
	return ""
}

func (handler *Handler) getProductionRelease(response http.ResponseWriter, request *http.Request, traceID string, workspaceID identity.WorkspaceID, releaseID identity.ReleaseID) string {
	actor := principalRef(request)
	if actor == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", traceID, false)
		return "UNAUTHENTICATED"
	}
	principal, err := identity.ParsePrincipalID(actor)
	if err != nil {
		return writeProductionError(response, domain.ErrInvalidArgument, traceID)
	}
	detail, err := handler.production.GetProductionReleaseForPrincipal(request.Context(), workspaceID, releaseID, principal)
	if err != nil {
		return writeProductionError(response, err, traceID)
	}
	rel := &detail.Release
	manifest := &detail.BeforeManifest
	beforePins := detail.BeforePins
	relProposals := detail.ReleaseProposals

	var beforeHead HeadReferencePayload
	if manifest.BeforeReleaseID != nil {
		rid := manifest.BeforeReleaseID.String()
		beforeHead = HeadReferencePayload{
			Presence:       "present",
			ReleaseID:      &rid,
			ManifestDigest: &manifest.BeforeManifestDigest,
		}
	} else {
		beforeHead = HeadReferencePayload{
			Presence: "absent",
		}
	}

	operationIDStr := ""
	versionVal := 0
	setDigestStr := ""
	valAttemptNo := 0
	valDigest := ""
	role := domain.RoleApplied
	reviewIDs := make([]string, 0)
	proposalIDs := make([]string, 0)
	contributors := make([]string, 0)
	reviewSet := map[string]bool{}

	for _, rp := range relProposals {
		proposalIDs = append(proposalIDs, rp.ProposalID.String())
		var ids []string
		if err := json.Unmarshal(rp.ReviewIDs, &ids); err != nil {
			return writeProductionError(response, domain.ErrPriorStateUnknown, traceID)
		}
		for _, id := range ids {
			reviewSet[id] = true
		}
	}
	for id := range reviewSet {
		reviewIDs = append(reviewIDs, id)
	}
	for _, contributor := range detail.Contributors {
		contributors = append(contributors, contributor.PrincipalID.String())
	}
	sort.Strings(reviewIDs)
	sort.Strings(proposalIDs)
	sort.Strings(contributors)

	if detail.Attribution != nil {
		operationIDStr = detail.Attribution.OperationID.String()
		versionVal = detail.Attribution.ProductionVersion
		setDigestStr = detail.Attribution.SetDigest
		valAttemptNo = detail.Attribution.AttemptNo
		valDigest = detail.Attribution.ValidationDigest
		role = detail.Attribution.Role
	}

	attribution := ReleaseAttributionResponse{
		OperationID: operationIDStr,
		Version:     versionVal,
		SetDigest:   setDigestStr,
		Validation: ValidationReferencePayload{
			AttemptNo:        valAttemptNo,
			ValidationDigest: valDigest,
		},
		ReviewIDs:    reviewIDs,
		ProposalIDs:  proposalIDs,
		Contributors: contributors,
		Role:         role,
	}
	if rel.RolledBackToReleaseID != nil {
		attribution.Role = domain.RoleReverted
	}

	rootRelID := rel.ID.String()
	if rel.ProductionRootReleaseID != nil {
		rootRelID = rel.ProductionRootReleaseID.String()
	}
	rollbackDepth := 0
	if rel.ProductionRollbackDepth != nil {
		rollbackDepth = *rel.ProductionRollbackDepth
	}
	var parentRelID *string
	if rel.ProductionRollbackParentID != nil {
		s := rel.ProductionRollbackParentID.String()
		parentRelID = &s
	}

	protection := ProductionReleaseProtectionResponse{
		RootReleaseID:           rootRelID,
		RollbackDepth:           rollbackDepth,
		RollbackParentReleaseID: parentRelID,
	}

	afterAssets := make([]ManifestAssetResponse, 0, len(rel.Entries))
	for _, a := range rel.Entries {
		compatibility := map[string]any{}
		if len(a.Compatibility) > 0 {
			if err := json.Unmarshal(a.Compatibility, &compatibility); err != nil {
				return writeProductionError(response, domain.ErrPriorStateUnknown, traceID)
			}
		}
		afterAssets = append(afterAssets, ManifestAssetResponse{
			AssetID:       a.AssetID.String(),
			RevisionID:    a.RevisionID.String(),
			Position:      a.Position,
			Compatibility: compatibility,
		})
	}
	afterObjects := make([]ManifestObjectResponse, 0, len(rel.Objects))
	for _, o := range rel.Objects {
		id, err := o.TypedID()
		if err != nil {
			return writeProductionError(response, domain.ErrPriorStateUnknown, traceID)
		}
		afterObjects = append(afterObjects, ManifestObjectResponse{
			Kind:          string(o.ObjectType),
			TargetID:      id,
			ObjectVersion: o.Version,
			Position:      o.Position,
		})
	}

	var beforeManifestResp ManifestResponse
	if manifest != nil {
		beforeManifestResp.Digest = manifest.BeforeManifestDigest
		if len(manifest.BeforeManifestJSON) > 0 {
			if err := json.Unmarshal(manifest.BeforeManifestJSON, &beforeManifestResp); err != nil {
				return writeProductionError(response, domain.ErrPriorStateUnknown, traceID)
			}
		}
	}

	beforePinsResp := make([]BeforePinResponse, 0, len(beforePins))
	for _, bp := range beforePins {
		var revID *string
		if bp.RevisionID != nil {
			s := bp.RevisionID.String()
			revID = &s
		}
		beforePinsResp = append(beforePinsResp, BeforePinResponse{
			Kind:          bp.TargetKind,
			TargetID:      bp.TargetID,
			Presence:      bp.Presence,
			RevisionID:    revID,
			ObjectVersion: bp.ObjectVersion,
			ContentDigest: bp.ContentDigest,
		})
	}

	var originPropID *string
	if rel.OriginProposalID != nil {
		s := rel.OriginProposalID.String()
		originPropID = &s
	}
	var rolledBackRelID *string
	if rel.RolledBackToReleaseID != nil {
		s := rel.RolledBackToReleaseID.String()
		rolledBackRelID = &s
	}

	resp := ProductionReleaseResponse{
		ID:             rel.ID.String(),
		Sequence:       rel.Sequence,
		PublishedBy:    rel.PublishedBy,
		PublishedAt:    rel.PublishedAt.Format(time.RFC3339Nano),
		Attribution:    attribution,
		Protection:     protection,
		BeforeHead:     beforeHead,
		BeforeManifest: beforeManifestResp,
		AfterManifest: ManifestResponse{
			Assets:  afterAssets,
			Objects: afterObjects,
			Digest:  rel.ManifestDigest,
		},
		BeforePins:          beforePinsResp,
		OriginProposalID:    originPropID,
		OriginProposalIDs:   proposalIDs,
		RolledBackReleaseID: rolledBackRelID,
		ProjectionStatus:    detail.ProjectionStatus,
	}

	writeJSON(response, http.StatusOK, resp)
	return ""
}

func (handler *Handler) rollbackProductionRelease(response http.ResponseWriter, request *http.Request, traceID string, workspaceID identity.WorkspaceID, releaseID identity.ReleaseID) string {
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "Idempotency-Key header must be between 8 and 128 characters", traceID, false)
		return "INVALID_ARGUMENT"
	}

	actor := principalRef(request)
	if actor == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", traceID, false)
		return "UNAUTHENTICATED"
	}
	principalID, err := identity.ParsePrincipalID(actor)
	if err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid principal", traceID, false)
		return "INVALID_ARGUMENT"
	}

	if err := handler.checkProductionAuth(request.Context(), actor, workspaceID, authz.ActionReleaseRollback, traceID); err != nil {
		return writeProductionError(response, err, traceID)
	}

	var body RollbackProductionRequestPayload
	if err := decodeStrictJSON(response, request, &body); err != nil {
		return writeProductionError(response, err, traceID)
	}

	var expectedRelID *identity.ReleaseID
	if body.ExpectedHead.ReleaseID != nil && *body.ExpectedHead.ReleaseID != "" {
		rid, err := identity.ParseReleaseID(*body.ExpectedHead.ReleaseID)
		if err != nil {
			writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid expectedHead releaseId", traceID, false)
			return "INVALID_ARGUMENT"
		}
		expectedRelID = &rid
	}

	headRef := domain.HeadReference{
		Presence:       body.ExpectedHead.Presence,
		ReleaseID:      expectedRelID,
		ManifestDigest: body.ExpectedHead.ManifestDigest,
	}

	cmd := governanceapp.RollbackOperationCommand{
		WorkspaceID:     workspaceID,
		PrincipalID:     principalID,
		ReleaseID:       releaseID,
		ExpectedVersion: body.ExpectedVersion,
		SetDigest:       body.SetDigest,
		ExpectedHead:    headRef,
		Reason:          body.Reason,
		IdempotencyKey:  idempotencyKey,
		TraceID:         traceID,
	}

	result, err := handler.production.RollbackOperation(request.Context(), cmd)
	if err != nil {
		return writeProductionError(response, err, traceID)
	}

	resp := ReleaseCommandResultResponse{
		ReleaseID:      result.ReleaseID.String(),
		OperationID:    result.OperationID.String(),
		Replayed:       result.Replayed,
		ManifestDigest: result.ManifestDigest,
	}

	if result.Replayed {
		writeJSON(response, http.StatusOK, resp)
		return ""
	}

	location := fmt.Sprintf("/api/v1/workspaces/%s/production-releases/%s", workspaceID.String(), result.ReleaseID.String())
	response.Header().Set("Location", location)
	writeJSON(response, http.StatusCreated, resp)
	return ""
}
