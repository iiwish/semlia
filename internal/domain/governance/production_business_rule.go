package governance

import (
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

type ProductionBusinessRuleCommand struct {
	WorkspaceID     identity.WorkspaceID           `json:"-"`
	OperationID     identity.ProductionOperationID `json:"-"`
	PrincipalID     identity.PrincipalID           `json:"-"`
	IdempotencyKey  string                         `json:"-"`
	ExpectedVersion int                            `json:"expectedVersion"`
	SetDigest       string                         `json:"setDigest"`
	TargetKey       string                         `json:"targetKey"`
	Action          string                         `json:"action"`
	EvidenceID      string                         `json:"evidenceId,omitempty"`
	Declaration     string                         `json:"declaration,omitempty"`
}

type ProductionBusinessRuleEvent struct {
	Sequence             int64                          `json:"sequence"`
	WorkspaceID          identity.WorkspaceID           `json:"workspaceId"`
	OperationID          identity.ProductionOperationID `json:"operationId"`
	ProductionVersion    int                            `json:"productionVersion"`
	TargetKey            string                         `json:"targetKey"`
	Action               string                         `json:"action"`
	SetDigest            string                         `json:"setDigest"`
	ContentDigest        string                         `json:"contentDigest"`
	EvidenceID           string                         `json:"evidenceId,omitempty"`
	EvidenceDigest       string                         `json:"evidenceDigest,omitempty"`
	EvidenceOrigin       string                         `json:"evidenceOrigin"`
	PrincipalID          identity.PrincipalID           `json:"principalId"`
	AuthorizationVersion int64                          `json:"authorizationVersion"`
	CreatedAt            time.Time                      `json:"createdAt"`
	Replayed             bool                           `json:"replayed"`
}

type ProductionBusinessRuleWitness struct {
	Event       ProductionBusinessRuleEvent `json:"event"`
	Valid       bool                        `json:"valid"`
	Declaration string                      `json:"declaration,omitempty"`
}

func RequiresProductionBusinessRule(target ProductionTarget) bool {
	return target.Kind == TargetKindSemanticAsset && target.ProposalID != nil
}
