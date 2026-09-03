package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// ProposalValidationOutcomes is the persisted T004 validation state the
// decision inputs consume: run statuses plus severity counts.
type ProposalValidationOutcomes struct {
	RunCount     int
	FailedCount  int
	BlockerCount int
	WarningCount int
	InfoCount    int
}

// PolicyAssetFacts are the target-asset facts the collectors need: the asset
// type, the evidence links recorded against the asset's revisions, and the
// current revision content carrying the owner field.
type PolicyAssetFacts struct {
	AssetType              string
	EvidenceLinkCount      int
	CurrentRevisionContent json.RawMessage
}

// DecisionFactsRepository is everything the decision inputs collectors read:
// persisted proposal state and per-target facts. Implementations map every
// read onto workspace-scoped queries; nothing else influences the inputs.
type DecisionFactsRepository interface {
	GetProposal(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) (governance.Proposal, error)
	ListProposalChanges(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) ([]governance.ChangeSetItem, error)
	SummarizeProposalValidationOutcomes(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) (ProposalValidationOutcomes, error)
	GetPolicyAssetFacts(ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID) (PolicyAssetFacts, error)
	GetPolicyGovernedObjectAsset(ctx context.Context, workspace identity.WorkspaceID, objectUUID string) (identity.AssetID, error)
	GetPolicyRevisionOwnerContent(ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID, revision identity.RevisionID) (json.RawMessage, error)
}

// TargetInputs is the per-target slice of the decision inputs: the governed
// type under review, its evidence support and its owner content. Assetless
// targets (join contracts) carry the not-applicable vocabulary.
type TargetInputs struct {
	AssetType         string
	EvidenceLinkCount int
	OwnerContent      json.RawMessage
}

// TargetInputsCollector computes the target slice for exactly one proposal
// target type. New governance object types join by adding a collector
// registration, never by extending the trigger with conditionals.
type TargetInputsCollector interface {
	CollectTargetInputs(ctx context.Context, workspace identity.WorkspaceID, proposal governance.Proposal) (TargetInputs, error)
}

// InputsCollectorRegistry resolves one collector per proposal target type.
type InputsCollectorRegistry struct {
	collectors map[governance.TargetObjectType]TargetInputsCollector
}

func NewInputsCollectorRegistry(facts DecisionFactsRepository) *InputsCollectorRegistry {
	assetScoped := &assetScopedObjectInputsCollector{facts: facts}
	return &InputsCollectorRegistry{
		collectors: map[governance.TargetObjectType]TargetInputsCollector{
			governance.TargetSemanticAsset:   &semanticAssetInputsCollector{facts: facts},
			governance.TargetPhysicalBinding: assetScoped,
			governance.TargetModelGrain:      assetScoped,
			governance.TargetEntityKey:       assetScoped,
			governance.TargetJoinContract:    &joinContractInputsCollector{},
		},
	}
}

func (registry *InputsCollectorRegistry) For(
	targetType governance.TargetObjectType,
) (TargetInputsCollector, error) {
	collector, ok := registry.collectors[targetType]
	if !ok {
		return nil, fmt.Errorf("%w: no decision inputs collector for target type %q", governance.ErrInvalidArgument, targetType)
	}
	return collector, nil
}

// semanticAssetInputsCollector reads the proposal's base revision: the
// immutable baseline the structured change applies to.
type semanticAssetInputsCollector struct {
	facts DecisionFactsRepository
}

func (collector *semanticAssetInputsCollector) CollectTargetInputs(
	ctx context.Context, workspace identity.WorkspaceID, proposal governance.Proposal,
) (TargetInputs, error) {
	if proposal.AssetID == nil || proposal.BaseRevisionID == nil {
		return TargetInputs{}, fmt.Errorf(
			"%w: semantic-asset proposal %s without asset references",
			governance.ErrInvariant, proposal.ID)
	}
	facts, err := collector.facts.GetPolicyAssetFacts(ctx, workspace, *proposal.AssetID)
	if err != nil {
		return TargetInputs{}, err
	}
	owner, err := collector.facts.GetPolicyRevisionOwnerContent(ctx, workspace, *proposal.AssetID, *proposal.BaseRevisionID)
	if err != nil {
		return TargetInputs{}, err
	}
	return TargetInputs{
		AssetType: facts.AssetType, EvidenceLinkCount: facts.EvidenceLinkCount, OwnerContent: owner,
	}, nil
}

// assetScopedObjectInputsCollector serves the three asset-anchored governance
// objects: the object resolves to its asset, then the asset facts apply.
type assetScopedObjectInputsCollector struct {
	facts DecisionFactsRepository
}

func (collector *assetScopedObjectInputsCollector) CollectTargetInputs(
	ctx context.Context, workspace identity.WorkspaceID, proposal governance.Proposal,
) (TargetInputs, error) {
	assetID, err := collector.facts.GetPolicyGovernedObjectAsset(ctx, workspace, proposal.TargetObjectID)
	if err != nil {
		return TargetInputs{}, err
	}
	facts, err := collector.facts.GetPolicyAssetFacts(ctx, workspace, assetID)
	if err != nil {
		return TargetInputs{}, err
	}
	return TargetInputs{
		AssetType: facts.AssetType, EvidenceLinkCount: facts.EvidenceLinkCount,
		OwnerContent: facts.CurrentRevisionContent,
	}, nil
}

// joinContractInputsCollector serves the assetless target: no asset type, no
// asset evidence surface and no owner field exist, so every asset-scoped
// category is explicitly not applicable.
type joinContractInputsCollector struct{}

func (collector *joinContractInputsCollector) CollectTargetInputs(
	_ context.Context, _ identity.WorkspaceID, _ governance.Proposal,
) (TargetInputs, error) {
	return TargetInputs{AssetType: governance.DecisionInputNotApplicable}, nil
}

// DiffCategories is the closed structured-diff classification of a change-set
// (SSOT §8.3): which field-path categories the change touches.
type DiffCategories struct {
	Computation bool
	Definition  bool
	Relations   bool
	Access      bool
	Contract    bool
}

// Closed field-path prefixes per category, matched against the lowercased
// first dot-segment of the field path. Unclassified prefixes count as
// definition/documentation: only explicitly risk-bearing fields move the
// routing inputs, and mislabeled ownership or metadata must not fake a
// computation change.
var diffCategoryPrefixes = []struct {
	prefixes []string
	category func(*DiffCategories)
}{
	{
		prefixes: []string{
			"expression", "formula", "computation", "calculation", "sql", "transform",
			"grain", "join", "cardinality", "uniqueness", "key", "binding", "aggregate", "timegrain",
		},
		category: func(categories *DiffCategories) { categories.Computation = true },
	},
	{
		prefixes: []string{
			"definition", "name", "title", "summary", "description", "alias", "synonym", "example",
			"doc", "display", "label", "owner", "steward", "maintainer", "tag", "content", "relatedasset",
		},
		category: func(categories *DiffCategories) { categories.Definition = true },
	},
	{
		prefixes: []string{"relation", "depends", "derived", "contains", "describes", "replaces"},
		category: func(categories *DiffCategories) { categories.Relations = true },
	},
	{
		prefixes: []string{"permission", "access", "acl", "visibility", "confidentiality", "authz", "privacy"},
		category: func(categories *DiffCategories) { categories.Access = true },
	},
	{
		prefixes: []string{"contract", "compatibility", "sla", "quality", "migration", "deprecation", "consumer"},
		category: func(categories *DiffCategories) { categories.Contract = true },
	},
}

// ClassifyDiffCategories maps every change-set field path onto the closed
// category set. The classification is a deterministic function of the
// field-path strings alone: no locale, no ordering, no environment.
func ClassifyDiffCategories(items []governance.ChangeSetItem) DiffCategories {
	var categories DiffCategories
	classified := false
	for _, item := range items {
		segment := item.FieldPath
		if dot := strings.Index(segment, "."); dot >= 0 {
			segment = segment[:dot]
		}
		segment = strings.ToLower(segment)
		for _, group := range diffCategoryPrefixes {
			for _, prefix := range group.prefixes {
				if strings.HasPrefix(segment, prefix) {
					group.category(&categories)
					classified = true
					break
				}
			}
		}
	}
	if len(items) > 0 && !classified {
		categories.Definition = true
	}
	return categories
}

func ownerAssignedFromContent(content json.RawMessage) string {
	if len(content) == 0 {
		return governance.OwnerNotApplicable
	}
	var payload struct {
		Owner *string `json:"owner"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		return governance.OwnerUnassigned
	}
	if payload.Owner != nil && strings.TrimSpace(*payload.Owner) != "" {
		return governance.OwnerAssigned
	}
	return governance.OwnerUnassigned
}

// BuildDecisionInputs assembles the closed, versioned DecisionInputs payload
// from persisted state only (SSOT §8.2/§8.3). It is a pure function of its
// arguments: identical persisted state always yields byte-identical inputs
// and therefore an identical inputs_digest. The not-yet-computable §8.3
// categories (consumer counts, historical acceptance rates) stay explicit
// not-applicable placeholders until M4 populates them under a new rule
// version.
func BuildDecisionInputs(
	proposal governance.Proposal,
	changes []governance.ChangeSetItem,
	outcomes ProposalValidationOutcomes,
	target TargetInputs,
) (governance.DecisionInputs, error) {
	if proposal.TargetObjectType == "" {
		return governance.DecisionInputs{}, fmt.Errorf("%w: proposal without target type", governance.ErrInvalidArgument)
	}
	categories := ClassifyDiffCategories(changes)
	authorKind := governance.AuthorKindHuman
	if proposal.AgentRunID != nil {
		authorKind = governance.AuthorKindAgent
	}
	assetType := target.AssetType
	if assetType == "" {
		assetType = governance.DecisionInputNotApplicable
	}
	// M2 persists no environment dimension: without a production marker the
	// input is deterministically false while staying rule-addressable.
	inputs := governance.DecisionInputs{
		AssetType:             assetType,
		TargetObjectType:      string(proposal.TargetObjectType),
		AffectsComputation:    categories.Computation,
		AffectsDefinition:     categories.Definition,
		AffectsRelations:      categories.Relations,
		AffectsAccess:         categories.Access,
		AffectsContract:       categories.Contract,
		ProductionEnvironment: false,
		ValidationRunCount:    outcomes.RunCount,
		ValidationFailedCount: outcomes.FailedCount,
		BlockerCount:          outcomes.BlockerCount,
		WarningCount:          outcomes.WarningCount,
		InfoCount:             outcomes.InfoCount,
		EvidenceLinkCount:     target.EvidenceLinkCount,
		EvidenceComplete:      target.EvidenceLinkCount > 0,
		OwnerAssigned:         ownerAssignedFromContent(target.OwnerContent),
		AuthorKind:            authorKind,
	}
	if err := inputs.Validate(); err != nil {
		return governance.DecisionInputs{}, err
	}
	return inputs, nil
}
