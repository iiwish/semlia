package governance

import (
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

// ModelProviderProtocol is the wire protocol of a configured provider. The
// ModelConfigurationView contract (web/src/ModelConfigurationView.tsx) offers
// OpenAI, Anthropic, Google Gemini and OpenAI-compatible entries; the
// openai_compatible protocol shares the OpenAI wire format. Anthropic and
// Gemini configurations persist but their live adapters are not part of v1.
type ModelProviderProtocol string

const (
	ProtocolOpenAI           ModelProviderProtocol = "openai"
	ProtocolAnthropic        ModelProviderProtocol = "anthropic"
	ProtocolGemini           ModelProviderProtocol = "gemini"
	ProtocolOpenAICompatible ModelProviderProtocol = "openai_compatible"
)

func (protocol ModelProviderProtocol) Valid() bool {
	switch protocol {
	case ProtocolOpenAI, ProtocolAnthropic, ProtocolGemini, ProtocolOpenAICompatible:
		return true
	}
	return false
}

// SupportsGeneration reports whether the protocol can serve live proposal
// generation in v1. Anthropic and Gemini persist but return the stable
// provider_unsupported error until their adapters land.
func (protocol ModelProviderProtocol) SupportsGeneration() bool {
	return protocol == ProtocolOpenAI || protocol == ProtocolOpenAICompatible
}

type ModelKind string

const (
	ModelKindLLM       ModelKind = "llm"
	ModelKindEmbedding ModelKind = "embedding"
)

func (kind ModelKind) Valid() bool {
	return kind == ModelKindLLM || kind == ModelKindEmbedding
}

// ModelProvider is one workspace-scoped provider entry of the model
// configuration surface. Credential material is structurally unrepresentable
// (SSOT §12 S-002): CredentialEnv is the NAME of the environment variable
// holding the secret and CredentialRevision is a salted sha256 digest of the
// provided secret used for revision tracking only.
type ModelProvider struct {
	ID                 identity.ModelProviderID
	WorkspaceID        identity.WorkspaceID
	Protocol           ModelProviderProtocol
	DisplayName        string
	BaseURL            *string
	CredentialEnv      string
	CredentialRevision string
	Enabled            bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ModelSetting is one model entry below a provider: an LLM candidate for
// governed generation or an embedding model for the (session-scoped) vector
// index. Exactly one enabled default per (workspace, kind) is enforced by a
// partial unique index and the SetDefault command.
type ModelSetting struct {
	ID                 identity.ModelSettingID
	WorkspaceID        identity.WorkspaceID
	ProviderID         identity.ModelProviderID
	Kind               ModelKind
	Model              string
	Enabled            bool
	IsDefault          bool
	Capability         string
	TokenLimit         int
	EmbeddingDimension *int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (provider ModelProvider) Validate() error {
	if provider.ID.IsZero() || provider.WorkspaceID.IsZero() || !provider.Protocol.Valid() {
		return ErrInvalidArgument
	}
	if provider.DisplayName == "" || len(provider.DisplayName) > 120 {
		return ErrInvalidArgument
	}
	if provider.BaseURL != nil && (*provider.BaseURL == "" || len(*provider.BaseURL) > 512) {
		return ErrInvalidArgument
	}
	if !IsValidCredentialEnvName(provider.CredentialEnv) {
		return ErrInvalidArgument
	}
	if !IsValidContentDigest(provider.CredentialRevision) {
		return ErrInvalidArgument
	}
	return nil
}

func (setting ModelSetting) Validate() error {
	if setting.ID.IsZero() || setting.WorkspaceID.IsZero() || setting.ProviderID.IsZero() ||
		!setting.Kind.Valid() {
		return ErrInvalidArgument
	}
	if setting.Model == "" || len(setting.Model) > 128 {
		return ErrInvalidArgument
	}
	if setting.Capability == "" || len(setting.Capability) > 128 {
		return ErrInvalidArgument
	}
	if setting.TokenLimit <= 0 {
		return ErrInvalidArgument
	}
	if setting.Kind == ModelKindEmbedding {
		if setting.EmbeddingDimension == nil || *setting.EmbeddingDimension <= 0 {
			return ErrInvalidArgument
		}
	} else if setting.EmbeddingDimension != nil {
		return ErrInvalidArgument
	}
	return nil
}

// IsValidCredentialEnvName locks the credential surface to environment
// variable NAMES: lowercase characters, spaces and secret-looking material
// cannot satisfy the pattern, so a pasted API key is rejected before any
// persistence happens.
func IsValidCredentialEnvName(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	if value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		switch {
		case character >= 'A' && character <= 'Z', character >= '0' && character <= '9', character == '_':
		default:
			return false
		}
	}
	return true
}
