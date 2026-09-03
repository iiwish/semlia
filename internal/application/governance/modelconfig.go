package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// ModelConfigRepository persists the workspace model-configuration surface.
type ModelConfigRepository interface {
	CreateModelProvider(ctx context.Context, provider domain.ModelProvider) (domain.ModelProvider, error)
	GetModelProvider(ctx context.Context, workspace identity.WorkspaceID, provider identity.ModelProviderID) (domain.ModelProvider, error)
	ListModelProviders(ctx context.Context, workspace identity.WorkspaceID) ([]domain.ModelProvider, error)
	UpdateModelProvider(ctx context.Context, command UpdateModelProviderCommand) (domain.ModelProvider, error)
	CreateModelSetting(ctx context.Context, setting domain.ModelSetting) (domain.ModelSetting, error)
	GetModelSetting(ctx context.Context, workspace identity.WorkspaceID, setting identity.ModelSettingID) (domain.ModelSetting, error)
	ListModelSettings(ctx context.Context, workspace identity.WorkspaceID) ([]domain.ModelSetting, error)
	UpdateModelSetting(ctx context.Context, command UpdateModelSettingCommand) (domain.ModelSetting, error)
	SetDefaultModelSetting(ctx context.Context, workspace identity.WorkspaceID, setting identity.ModelSettingID, when time.Time) (domain.ModelSetting, error)
	GetDefaultModelSetting(ctx context.Context, workspace identity.WorkspaceID, kind domain.ModelKind) (domain.ModelSetting, error)
}

type UpdateModelProviderCommand struct {
	WorkspaceID        identity.WorkspaceID
	ProviderID         identity.ModelProviderID
	DisplayName        string
	BaseURL            *string
	CredentialEnv      string
	CredentialRevision string
	Enabled            bool
	UpdatedAt          time.Time
}

type UpdateModelSettingCommand struct {
	WorkspaceID        identity.WorkspaceID
	SettingID          identity.ModelSettingID
	Model              string
	Enabled            bool
	Capability         string
	TokenLimit         int
	EmbeddingDimension *int
	UpdatedAt          time.Time
}

// CredentialRevisionDigest derives the persisted revision marker of one
// provided secret: a domain-separated sha256 digest. The secret itself is
// never persisted, logged or echoed (SSOT §12 S-002) — the digest only lets
// operators detect that a credential changed.
func CredentialRevisionDigest(credential string) string {
	sum := sha256.Sum256([]byte("semlia.credential.v1:" + credential))
	return "sha256:" + hex.EncodeToString(sum[:])
}

type CreateModelProviderRequest struct {
	WorkspaceID identity.WorkspaceID
	Protocol    domain.ModelProviderProtocol
	DisplayName string
	BaseURL     *string
	// CredentialEnv is the NAME of the environment variable holding the
	// secret; only the name is persisted.
	CredentialEnv string
	// Credential is the write-only secret material of this create call. It is
	// reduced to its revision digest and never persisted or echoed.
	Credential   string
	PrincipalRef string
	TraceID      string
}

type UpdateModelProviderRequest struct {
	WorkspaceID identity.WorkspaceID
	ProviderID  identity.ModelProviderID
	DisplayName string
	BaseURL     *string
	Enabled     bool
	// CredentialEnv/Credential are optional rotation inputs: nil/"" keeps the
	// persisted env name and revision. Secrets are never persisted or echoed.
	CredentialEnv *string
	Credential    *string
	PrincipalRef  string
	TraceID       string
}

type CreateModelSettingRequest struct {
	WorkspaceID        identity.WorkspaceID
	ProviderID         identity.ModelProviderID
	Kind               domain.ModelKind
	Model              string
	Capability         string
	TokenLimit         int
	EmbeddingDimension *int
	PrincipalRef       string
	TraceID            string
}

type UpdateModelSettingRequest struct {
	WorkspaceID        identity.WorkspaceID
	SettingID          identity.ModelSettingID
	Model              string
	Capability         string
	TokenLimit         int
	EmbeddingDimension *int
	Enabled            bool
	PrincipalRef       string
	TraceID            string
}

type SetDefaultModelSettingRequest struct {
	WorkspaceID  identity.WorkspaceID
	SettingID    identity.ModelSettingID
	PrincipalRef string
	TraceID      string
}

type GetModelConfigRequest struct {
	WorkspaceID  identity.WorkspaceID
	PrincipalRef string
	TraceID      string
}

type GetModelSettingRequest struct {
	WorkspaceID  identity.WorkspaceID
	SettingID    identity.ModelSettingID
	PrincipalRef string
	TraceID      string
}

// ModelProviderDetail is the read model of one provider with its model
// settings nested exactly as the ModelConfigurationView renders them.
type ModelProviderDetail struct {
	Provider domain.ModelProvider
	Settings []domain.ModelSetting
}

// ModelConfigService serves the persisted model-configuration surface.
//
// Authorization mapping (FR-003 vocabulary — no unlisted action is invented):
// the vocabulary has no model-config action, so mutations map to the
// administration action `workspace.manage` (granted only to workspace_admin)
// and reads map to `workspace.read`, both evaluated at workspace scope.
type ModelConfigService struct {
	repository ModelConfigRepository
	authorizer authorizationapp.Evaluator
	clock      Clock
}

func NewModelConfigService(
	repository ModelConfigRepository, authorizer authorizationapp.Evaluator, clock Clock,
) *ModelConfigService {
	if repository == nil || clock == nil {
		panic("model config repository and clock are required")
	}
	return &ModelConfigService{repository: repository, authorizer: authorizer, clock: clock}
}

func (service *ModelConfigService) authorize(ctx context.Context, action authorization.Action, workspace identity.WorkspaceID, principalRef, traceID string) error {
	if service.authorizer == nil {
		return nil
	}
	decision, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: strings.TrimSpace(principalRef),
		WorkspaceID:  workspace,
		Action:       action,
		Resource:     authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()},
		TraceID:      traceID,
	})
	if err != nil {
		return err
	}
	if decision.Allowed {
		return nil
	}
	return &authorization.DenialError{Decision: decision}
}

// CreateProvider persists one provider entry. The optional write-only secret
// is reduced to its revision digest; neither the secret nor any credential
// field is ever persisted or echoed.
func (service *ModelConfigService) CreateProvider(ctx context.Context, request CreateModelProviderRequest) (domain.ModelProvider, error) {
	if err := service.authorize(ctx, authorization.ActionWorkspaceManage, request.WorkspaceID, request.PrincipalRef, request.TraceID); err != nil {
		return domain.ModelProvider{}, err
	}
	displayName := strings.TrimSpace(request.DisplayName)
	protocol := domain.ModelProviderProtocol(request.Protocol)
	credentialEnv := strings.TrimSpace(request.CredentialEnv)
	if !protocol.Valid() || displayName == "" || len(displayName) > 120 ||
		!domain.IsValidCredentialEnvName(credentialEnv) ||
		strings.TrimSpace(request.Credential) == "" {
		return domain.ModelProvider{}, domain.ErrInvalidArgument
	}
	if request.BaseURL != nil {
		trimmed := strings.TrimSpace(*request.BaseURL)
		if trimmed == "" {
			request.BaseURL = nil
		} else {
			request.BaseURL = &trimmed
		}
	}
	if protocol == domain.ProtocolOpenAICompatible && request.BaseURL == nil {
		return domain.ModelProvider{}, domain.ErrInvalidArgument
	}
	providerID, err := identity.NewModelProviderID()
	if err != nil {
		return domain.ModelProvider{}, fmt.Errorf("mint provider ID: %w", err)
	}
	now := service.clock.Now().UTC()
	provider := domain.ModelProvider{
		ID: providerID, WorkspaceID: request.WorkspaceID, Protocol: protocol,
		DisplayName: displayName, BaseURL: request.BaseURL,
		CredentialEnv: credentialEnv, CredentialRevision: CredentialRevisionDigest(strings.TrimSpace(request.Credential)),
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	return service.repository.CreateModelProvider(ctx, provider)
}

// UpdateProvider edits the non-secret provider metadata and optionally
// rotates the credential env name and revision digest.
func (service *ModelConfigService) UpdateProvider(ctx context.Context, request UpdateModelProviderRequest) (domain.ModelProvider, error) {
	if err := service.authorize(ctx, authorization.ActionWorkspaceManage, request.WorkspaceID, request.PrincipalRef, request.TraceID); err != nil {
		return domain.ModelProvider{}, err
	}
	provider, err := service.repository.GetModelProvider(ctx, request.WorkspaceID, request.ProviderID)
	if err != nil {
		return domain.ModelProvider{}, err
	}
	displayName := strings.TrimSpace(request.DisplayName)
	if displayName == "" || len(displayName) > 120 {
		return domain.ModelProvider{}, domain.ErrInvalidArgument
	}
	if request.BaseURL != nil {
		trimmed := strings.TrimSpace(*request.BaseURL)
		if trimmed == "" {
			request.BaseURL = nil
		} else {
			request.BaseURL = &trimmed
		}
	}
	if provider.Protocol == domain.ProtocolOpenAICompatible && request.BaseURL == nil {
		return domain.ModelProvider{}, domain.ErrInvalidArgument
	}
	credentialEnv := provider.CredentialEnv
	if request.CredentialEnv != nil {
		if !domain.IsValidCredentialEnvName(strings.TrimSpace(*request.CredentialEnv)) {
			return domain.ModelProvider{}, domain.ErrInvalidArgument
		}
		credentialEnv = strings.TrimSpace(*request.CredentialEnv)
	}
	credentialRevision := provider.CredentialRevision
	if request.Credential != nil && strings.TrimSpace(*request.Credential) != "" {
		credentialRevision = CredentialRevisionDigest(strings.TrimSpace(*request.Credential))
	}
	return service.repository.UpdateModelProvider(ctx, UpdateModelProviderCommand{
		WorkspaceID: request.WorkspaceID, ProviderID: request.ProviderID,
		DisplayName: displayName, BaseURL: request.BaseURL,
		CredentialEnv: credentialEnv, CredentialRevision: credentialRevision,
		Enabled: request.Enabled, UpdatedAt: service.clock.Now().UTC(),
	})
}

// CreateSetting persists one model entry. The first enabled model of a
// (workspace, kind) becomes its default, mirroring the configuration view.
func (service *ModelConfigService) CreateSetting(ctx context.Context, request CreateModelSettingRequest) (domain.ModelSetting, error) {
	if err := service.authorize(ctx, authorization.ActionWorkspaceManage, request.WorkspaceID, request.PrincipalRef, request.TraceID); err != nil {
		return domain.ModelSetting{}, err
	}
	provider, err := service.repository.GetModelProvider(ctx, request.WorkspaceID, request.ProviderID)
	if err != nil {
		return domain.ModelSetting{}, err
	}
	if !provider.Enabled {
		return domain.ModelSetting{}, fmt.Errorf("%w: provider %s is disabled", domain.ErrConflict, provider.ID.String())
	}
	kind := domain.ModelKind(request.Kind)
	settingID, err := identity.NewModelSettingID()
	if err != nil {
		return domain.ModelSetting{}, fmt.Errorf("mint setting ID: %w", err)
	}
	now := service.clock.Now().UTC()
	setting := domain.ModelSetting{
		ID: settingID, WorkspaceID: request.WorkspaceID, ProviderID: provider.ID,
		Kind: kind, Model: strings.TrimSpace(request.Model), Enabled: true,
		Capability: strings.TrimSpace(request.Capability), TokenLimit: request.TokenLimit,
		EmbeddingDimension: request.EmbeddingDimension, CreatedAt: now, UpdatedAt: now,
	}
	if _, defaultErr := service.repository.GetDefaultModelSetting(ctx, request.WorkspaceID, kind); errors.Is(defaultErr, domain.ErrNotFound) {
		setting.IsDefault = true
	} else if defaultErr != nil {
		return domain.ModelSetting{}, defaultErr
	}
	return service.repository.CreateModelSetting(ctx, setting)
}

// UpdateSetting edits the model entry fields; the kind is fixed at create.
func (service *ModelConfigService) UpdateSetting(ctx context.Context, request UpdateModelSettingRequest) (domain.ModelSetting, error) {
	if err := service.authorize(ctx, authorization.ActionWorkspaceManage, request.WorkspaceID, request.PrincipalRef, request.TraceID); err != nil {
		return domain.ModelSetting{}, err
	}
	setting, err := service.repository.GetModelSetting(ctx, request.WorkspaceID, request.SettingID)
	if err != nil {
		return domain.ModelSetting{}, err
	}
	if setting.Kind == domain.ModelKindEmbedding && request.EmbeddingDimension == nil {
		return domain.ModelSetting{}, domain.ErrInvalidArgument
	}
	if setting.Kind != domain.ModelKindEmbedding && request.EmbeddingDimension != nil {
		return domain.ModelSetting{}, domain.ErrInvalidArgument
	}
	if request.TokenLimit <= 0 {
		return domain.ModelSetting{}, domain.ErrInvalidArgument
	}
	return service.repository.UpdateModelSetting(ctx, UpdateModelSettingCommand{
		WorkspaceID: request.WorkspaceID, SettingID: request.SettingID,
		Model: strings.TrimSpace(request.Model), Enabled: request.Enabled,
		Capability: strings.TrimSpace(request.Capability), TokenLimit: request.TokenLimit,
		EmbeddingDimension: request.EmbeddingDimension, UpdatedAt: service.clock.Now().UTC(),
	})
}

// SetDefault moves the (workspace, kind) default to one enabled setting.
func (service *ModelConfigService) SetDefault(ctx context.Context, request SetDefaultModelSettingRequest) (domain.ModelSetting, error) {
	if err := service.authorize(ctx, authorization.ActionWorkspaceManage, request.WorkspaceID, request.PrincipalRef, request.TraceID); err != nil {
		return domain.ModelSetting{}, err
	}
	setting, err := service.repository.GetModelSetting(ctx, request.WorkspaceID, request.SettingID)
	if err != nil {
		return domain.ModelSetting{}, err
	}
	if !setting.Enabled {
		return domain.ModelSetting{}, fmt.Errorf("%w: model setting %s is disabled", domain.ErrConflict, setting.ID.String())
	}
	provider, err := service.repository.GetModelProvider(ctx, request.WorkspaceID, setting.ProviderID)
	if err != nil {
		return domain.ModelSetting{}, err
	}
	if !provider.Enabled {
		return domain.ModelSetting{}, fmt.Errorf("%w: provider %s is disabled", domain.ErrConflict, provider.ID.String())
	}
	return service.repository.SetDefaultModelSetting(ctx, request.WorkspaceID, request.SettingID, service.clock.Now().UTC())
}

// ListProviders reads the providers of one workspace with their settings.
func (service *ModelConfigService) ListProviders(ctx context.Context, request GetModelConfigRequest) ([]ModelProviderDetail, error) {
	if err := service.authorize(ctx, authorization.ActionWorkspaceRead, request.WorkspaceID, request.PrincipalRef, request.TraceID); err != nil {
		return nil, err
	}
	providers, err := service.repository.ListModelProviders(ctx, request.WorkspaceID)
	if err != nil {
		return nil, err
	}
	settings, err := service.repository.ListModelSettings(ctx, request.WorkspaceID)
	if err != nil {
		return nil, err
	}
	byProvider := make(map[identity.ModelProviderID][]domain.ModelSetting, len(providers))
	for _, setting := range settings {
		byProvider[setting.ProviderID] = append(byProvider[setting.ProviderID], setting)
	}
	details := make([]ModelProviderDetail, 0, len(providers))
	for _, provider := range providers {
		details = append(details, ModelProviderDetail{Provider: provider, Settings: byProvider[provider.ID]})
	}
	return details, nil
}

// GetSetting reads one workspace-scoped model setting.
func (service *ModelConfigService) GetSetting(ctx context.Context, request GetModelSettingRequest) (domain.ModelSetting, error) {
	if err := service.authorize(ctx, authorization.ActionWorkspaceRead, request.WorkspaceID, request.PrincipalRef, request.TraceID); err != nil {
		return domain.ModelSetting{}, err
	}
	return service.repository.GetModelSetting(ctx, request.WorkspaceID, request.SettingID)
}

// DefaultSetting resolves the enabled default model of one kind; generation
// falls back to it when the request carries no explicit model setting.
func (service *ModelConfigService) DefaultSetting(ctx context.Context, workspace identity.WorkspaceID, kind domain.ModelKind) (domain.ModelSetting, error) {
	return service.repository.GetDefaultModelSetting(ctx, workspace, kind)
}

func (service *ModelConfigService) authorizeRequest(ctx context.Context, action authorization.Action, workspace identity.WorkspaceID, principalRef, traceID string) error {
	if service.authorizer == nil {
		return nil
	}
	decision, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: strings.TrimSpace(principalRef),
		WorkspaceID:  workspace,
		Action:       action,
		Resource:     authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()},
		TraceID:      traceID,
	})
	if err != nil {
		return err
	}
	if decision.Allowed {
		return nil
	}
	return &authorization.DenialError{Decision: decision}
}
