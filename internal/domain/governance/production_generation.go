package governance

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

const ProductionSuggestionsSchema = "semlia.production-suggestions/v1"

var ErrGenerationNotAuthorized = fmt.Errorf("production generation is not authorized by server configuration")

// Grant rates are operator-attested conservative upper bounds, not usage or
// a price lookup. A request may reduce these limits but cannot grant spending.
type ProductionGenerationGrant struct {
	WorkspaceID          identity.WorkspaceID    `json:"workspaceId"`
	PrincipalID          identity.PrincipalID    `json:"principalId"`
	ModelSettingID       identity.ModelSettingID `json:"modelSettingId"`
	ModelConfigRevision  string                  `json:"modelConfigRevision"`
	PricingBasis         string                  `json:"pricingBasis"`
	MaxInputBytes        int                     `json:"maxInputBytes"`
	MaxOutputTokens      int                     `json:"maxOutputTokens"`
	MaxCostMicros        int64                   `json:"maxCostMicros"`
	InputMicrosPerByte   int64                   `json:"inputMicrosPerByte"`
	OutputMicrosPerToken int64                   `json:"outputMicrosPerToken"`
}

func (g ProductionGenerationGrant) CheckBudget(inputBytes, outputTokens int, requestCeiling int64) error {
	if inputBytes < 1 || outputTokens < 1 || g.MaxInputBytes < 1 || g.MaxInputBytes > 1<<20 || g.MaxOutputTokens < 1 || g.MaxOutputTokens > 16384 || g.MaxCostMicros < 1 || g.MaxCostMicros > 1000000000 || g.InputMicrosPerByte < 1 || g.OutputMicrosPerToken < 1 || inputBytes > g.MaxInputBytes || outputTokens > g.MaxOutputTokens || requestCeiling < 1 || requestCeiling > 1000000000 {
		return ErrGenerationNotAuthorized
	}
	ceiling := min(g.MaxCostMicros, requestCeiling)
	if int64(inputBytes) > ceiling/g.InputMicrosPerByte {
		return ErrGenerationNotAuthorized
	}
	remaining := ceiling - int64(inputBytes)*g.InputMicrosPerByte
	if int64(outputTokens) > remaining/g.OutputMicrosPerToken {
		return ErrGenerationNotAuthorized
	}
	return nil
}

type ProductionSuggestions struct {
	SchemaVersion string              `json:"schemaVersion"`
	Targets       []TargetDeclaration `json:"targets"`
}

type ProductionGenerationRequest struct {
	WorkspaceID         identity.WorkspaceID           `json:"-"`
	PrincipalID         identity.PrincipalID           `json:"-"`
	OperationID         identity.ProductionOperationID `json:"-"`
	IdempotencyKey      string                         `json:"-"`
	TraceID             string                         `json:"-"`
	ExpectedVersion     int                            `json:"expectedVersion"`
	InputDigest         string                         `json:"inputDigest"`
	ModelSettingID      identity.ModelSettingID        `json:"modelSettingId"`
	ModelConfigRevision string                         `json:"modelConfigRevision"`
	Instruction         string                         `json:"instruction"`
	MaxOutputTokens     int                            `json:"maxOutputTokens"`
	MaxCostMicros       int64                          `json:"maxCostMicros"`
}

type ProductionGenerationResult struct {
	RunID               identity.AgentRunID            `json:"runId"`
	OperationID         identity.ProductionOperationID `json:"operationId"`
	InputVersion        int                            `json:"inputVersion"`
	InputDigest         string                         `json:"inputDigest"`
	Status              string                         `json:"status"`
	ProviderMode        string                         `json:"providerMode"`
	Model               string                         `json:"model"`
	ModelConfigRevision string                         `json:"modelConfigRevision"`
	Replayed            bool                           `json:"replayed"`
	OutputDigest        *string                        `json:"outputDigest"`
	Output              json.RawMessage                `json:"output"`
	ErrorCode           *string                        `json:"errorCode"`
	CostMicros          *int64                         `json:"costMicros"`
	DurationMS          *int64                         `json:"durationMs"`
}

type ProductionGenerationDelta struct {
	LocalKey string            `json:"localKey"`
	Change   TargetChangeInput `json:"change"`
}

type ProductionGenerationApplication struct {
	RunID                identity.AgentRunID         `json:"runId"`
	SourceVersion        int                         `json:"sourceVersion"`
	SourceOutputDigest   string                      `json:"sourceOutputDigest"`
	AppliedVersion       int                         `json:"appliedVersion"`
	AppliedContentDigest string                      `json:"appliedContentDigest"`
	ActorPrincipalID     identity.PrincipalID        `json:"actorPrincipalId"`
	DeltaDigest          string                      `json:"deltaDigest"`
	Delta                []ProductionGenerationDelta `json:"delta"`
}

// Work contains reconstructed, authorized input only while a worker owns it.
// The assembled model prompt and provider response envelope are not stored.
type ProductionGenerationWork struct {
	Request       ProductionGenerationRequest
	RunID         identity.AgentRunID
	AgentID       identity.PrincipalID
	JobID         identity.RunID
	Status        string
	ProviderMode  string
	RequestDigest string
	PromptDigest  string
	GrantDigest   string
	Input         json.RawMessage
	Declarations  []TargetDeclaration
	Facts         json.RawMessage
	Setting       ModelSetting
	Provider      ModelProvider
	ClaimToken    string
	CallDeadline  *time.Time
}

func ProductionGenerationModelRevision(setting ModelSetting, provider ModelProvider) (string, error) {
	data, err := json.Marshal(struct {
		Setting  ModelSetting
		Provider ModelProvider
	}{setting, provider})
	if err != nil {
		return "", err
	}
	return DigestJSON(data)
}
