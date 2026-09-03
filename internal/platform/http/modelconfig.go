package httpapi

import (
	"net/http"
	"strings"

	contract "github.com/iiwish/semlia/api/gen/go"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// modelProviderDetailResponse renders one provider with its nested model
// settings. No credential field exists in the wire shape — the credential
// env-var name and its revision digest are configuration metadata, not
// secrets (SSOT §12 S-002).
func modelProviderDetailResponse(detail governanceapp.ModelProviderDetail) contract.GovernanceModelProviderDetail {
	provider := detail.Provider
	result := contract.GovernanceModelProviderDetail{
		Provider: modelProviderResponse(provider),
		Models:   make([]contract.GovernanceModelSetting, 0, len(detail.Settings)),
	}
	for _, setting := range detail.Settings {
		result.Models = append(result.Models, modelSettingResponse(setting))
	}
	return result
}

func modelProviderResponse(provider domain.ModelProvider) contract.GovernanceModelProvider {
	result := contract.GovernanceModelProvider{
		Id:                 provider.ID,
		Protocol:           contract.GovernanceModelProtocol(provider.Protocol),
		DisplayName:        provider.DisplayName,
		CredentialEnv:      provider.CredentialEnv,
		CredentialRevision: provider.CredentialRevision,
		Enabled:            provider.Enabled,
		CreatedAt:          provider.CreatedAt.UTC(),
		UpdatedAt:          provider.UpdatedAt.UTC(),
	}
	if provider.BaseURL != nil {
		result.BaseUrl = provider.BaseURL
	}
	return result
}

func modelSettingResponse(setting domain.ModelSetting) contract.GovernanceModelSetting {
	result := contract.GovernanceModelSetting{
		Id:         setting.ID,
		ProviderId: setting.ProviderID,
		Kind:       contract.GovernanceModelKind(setting.Kind),
		Model:      setting.Model,
		Enabled:    setting.Enabled,
		IsDefault:  setting.IsDefault,
		Capability: setting.Capability,
		TokenLimit: setting.TokenLimit,
		CreatedAt:  setting.CreatedAt.UTC(),
		UpdatedAt:  setting.UpdatedAt.UTC(),
	}
	if setting.EmbeddingDimension != nil {
		result.EmbeddingDimension = setting.EmbeddingDimension
	}
	return result
}

func generatedProposalResponse(result governanceapp.GenerationResult) contract.GovernanceGeneratedProposal {
	run := result.Run
	agentRun := contract.GovernanceAgentRun{
		Id:             run.ID,
		Model:          run.Model,
		ConfigRevision: run.ConfigRevision,
		InputHash:      run.InputHash,
		Status:         contract.GovernanceAgentRunStatus(run.Status),
		CostMicros:     run.CostMicros,
		StartedAt:      run.StartedAt.UTC(),
	}
	if run.OutputDigest != nil {
		digest := *run.OutputDigest
		agentRun.OutputDigest = &digest
	}
	if run.FinishedAt != nil {
		finishedAt := run.FinishedAt.UTC()
		agentRun.FinishedAt = &finishedAt
	}
	if run.DurationMS != nil {
		duration := *run.DurationMS
		agentRun.DurationMs = &duration
	}
	return contract.GovernanceGeneratedProposal{
		AgentRun: agentRun,
		Proposal: governanceProposalDetailResponse(result.Proposal),
	}
}

func (handler *Handler) listModelProviders(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) string {
	details, err := handler.governance.ModelConfig().ListProviders(request.Context(), governanceapp.GetModelConfigRequest{
		WorkspaceID:  workspaceID,
		PrincipalRef: strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	items := make([]contract.GovernanceModelProviderDetail, 0, len(details))
	for _, detail := range details {
		items = append(items, modelProviderDetailResponse(detail))
	}
	writeJSON(response, http.StatusOK, contract.GovernanceModelProviderPage{Items: items})
	return ""
}

func (handler *Handler) createModelProvider(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) string {
	raw, err := readBody(request)
	if err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	var body contract.CreateGovernanceModelProviderRequest
	if err := decodeStrict(raw, &body); err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	provider, err := handler.governance.ModelConfig().CreateProvider(request.Context(), governanceapp.CreateModelProviderRequest{
		WorkspaceID:   workspaceID,
		Protocol:      domain.ModelProviderProtocol(body.Protocol),
		DisplayName:   body.DisplayName,
		BaseURL:       body.BaseUrl,
		CredentialEnv: body.CredentialEnv,
		Credential:    dereferenceString(body.Credential),
		PrincipalRef:  strings.TrimSpace(request.Header.Get(headerPrincipal)),
		TraceID:       traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	writeJSON(response, http.StatusCreated, modelProviderDetailResponse(governanceapp.ModelProviderDetail{
		Provider: provider, Settings: []domain.ModelSetting{},
	}))
	return ""
}

func (handler *Handler) getModelProvider(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
	providerID identity.ModelProviderID,
) string {
	details, err := handler.governance.ModelConfig().ListProviders(request.Context(), governanceapp.GetModelConfigRequest{
		WorkspaceID:  workspaceID,
		PrincipalRef: strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	for _, detail := range details {
		if detail.Provider.ID == providerID {
			writeJSON(response, http.StatusOK, modelProviderDetailResponse(detail))
			return ""
		}
	}
	return writeGovernanceError(response, domain.ErrNotFound, traceID)
}

func (handler *Handler) updateModelProvider(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
	providerID identity.ModelProviderID,
) string {
	raw, err := readBody(request)
	if err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	var body contract.UpdateGovernanceModelProviderRequest
	if err := decodeStrict(raw, &body); err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	updateRequest := governanceapp.UpdateModelProviderRequest{
		WorkspaceID: workspaceID, ProviderID: providerID,
		DisplayName: body.DisplayName, BaseURL: body.BaseUrl, Enabled: body.Enabled,
		PrincipalRef: strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
	}
	if body.CredentialEnv != nil {
		credentialEnv := *body.CredentialEnv
		updateRequest.CredentialEnv = &credentialEnv
	}
	if body.Credential != nil {
		credential := *body.Credential
		updateRequest.Credential = &credential
	}
	if _, err := handler.governance.ModelConfig().UpdateProvider(request.Context(), updateRequest); err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	settings, err := handler.governance.ModelConfig().ListProviders(request.Context(), governanceapp.GetModelConfigRequest{
		WorkspaceID:  workspaceID,
		PrincipalRef: strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	for _, detail := range settings {
		if detail.Provider.ID == providerID {
			writeJSON(response, http.StatusOK, modelProviderDetailResponse(detail))
			return ""
		}
	}
	return writeGovernanceError(response, domain.ErrNotFound, traceID)
}

func (handler *Handler) listModelSettings(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) string {
	details, err := handler.governance.ModelConfig().ListProviders(request.Context(), governanceapp.GetModelConfigRequest{
		WorkspaceID:  workspaceID,
		PrincipalRef: strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	items := make([]contract.GovernanceModelSetting, 0)
	for _, detail := range details {
		for _, setting := range detail.Settings {
			items = append(items, modelSettingResponse(setting))
		}
	}
	writeJSON(response, http.StatusOK, contract.GovernanceModelSettingPage{Items: items})
	return ""
}

func (handler *Handler) createModelSetting(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) string {
	raw, err := readBody(request)
	if err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	var body contract.CreateGovernanceModelSettingRequest
	if err := decodeStrict(raw, &body); err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	setting, err := handler.governance.ModelConfig().CreateSetting(request.Context(), governanceapp.CreateModelSettingRequest{
		WorkspaceID: workspaceID, ProviderID: body.ProviderId,
		Kind: domain.ModelKind(body.Kind), Model: body.Model,
		Capability: body.Capability, TokenLimit: body.TokenLimit,
		EmbeddingDimension: body.EmbeddingDimension,
		PrincipalRef:       strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	writeJSON(response, http.StatusCreated, modelSettingResponse(setting))
	return ""
}

func (handler *Handler) getModelSetting(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
	settingID identity.ModelSettingID,
) string {
	setting, err := handler.governance.ModelConfig().GetSetting(request.Context(), governanceapp.GetModelSettingRequest{
		WorkspaceID: workspaceID, SettingID: settingID,
		PrincipalRef: strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	writeJSON(response, http.StatusOK, modelSettingResponse(setting))
	return ""
}

func (handler *Handler) updateModelSetting(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
	settingID identity.ModelSettingID,
) string {
	raw, err := readBody(request)
	if err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	var body contract.UpdateGovernanceModelSettingRequest
	if err := decodeStrict(raw, &body); err != nil {
		return writeGovernanceError(response, domain.ErrInvalidArgument, traceID)
	}
	setting, err := handler.governance.ModelConfig().UpdateSetting(request.Context(), governanceapp.UpdateModelSettingRequest{
		WorkspaceID: workspaceID, SettingID: settingID,
		Model: body.Model, Capability: body.Capability, TokenLimit: body.TokenLimit,
		EmbeddingDimension: body.EmbeddingDimension, Enabled: body.Enabled,
		PrincipalRef: strings.TrimSpace(request.Header.Get(headerPrincipal)), TraceID: traceID,
	})
	if err != nil {
		return writeGovernanceError(response, err, traceID)
	}
	writeJSON(response, http.StatusOK, modelSettingResponse(setting))
	return ""
}

func (handler *Handler) generateProposal(
	request *http.Request,
	traceID string,
	workspaceID identity.WorkspaceID,
) (governanceapp.GenerationResult, error) {
	raw, err := readBody(request)
	if err != nil {
		return governanceapp.GenerationResult{}, domain.ErrInvalidArgument
	}
	var body contract.GovernanceGenerateProposalRequest
	if err := decodeStrict(raw, &body); err != nil {
		return governanceapp.GenerationResult{}, domain.ErrInvalidArgument
	}
	generateRequest := governanceapp.GenerateProposalRequest{
		WorkspaceID:      workspaceID,
		TargetObjectType: domain.TargetObjectType(body.TargetObjectType),
		TargetObjectID:   string(body.TargetObjectId),
		Instruction:      body.Instruction,
		PrincipalRef:     strings.TrimSpace(request.Header.Get(headerPrincipal)),
		TraceID:          traceID,
	}
	if body.ModelSettingId != nil {
		generateRequest.ModelSettingID = body.ModelSettingId
	}
	return handler.governance.Generation().GenerateProposal(request.Context(), generateRequest)
}
