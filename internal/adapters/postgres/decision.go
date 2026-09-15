package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

var _ governanceapp.DecisionFactsRepository = (*Store)(nil)

// ---------- decision read surface ----------

func (store *Store) getProposalPolicyDecisionByVersionDigest(
	ctx context.Context,
	workspace identity.WorkspaceID,
	proposal identity.ProposalID,
	ruleVersion string,
	digest string,
) (governance.PolicyDecision, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.PolicyDecision{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal)
	if err != nil {
		return governance.PolicyDecision{}, fmt.Errorf("encode proposal ID: %w", err)
	}
	row, err := store.queries.GetProposalPolicyDecisionByVersionDigest(ctx, dbgen.GetProposalPolicyDecisionByVersionDigestParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
		RuleVersion: ruleVersion, InputsDigest: digest,
	})
	if err != nil {
		return governance.PolicyDecision{}, governanceRepositoryError("get policy decision by version digest", err)
	}
	return policyDecisionFromRow(row)
}

func (store *Store) GetLatestProposalPolicyDecision(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) (governance.PolicyDecision, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.PolicyDecision{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal)
	if err != nil {
		return governance.PolicyDecision{}, fmt.Errorf("encode proposal ID: %w", err)
	}
	row, err := store.queries.GetLatestProposalPolicyDecision(ctx, dbgen.GetLatestProposalPolicyDecisionParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
	})
	if err != nil {
		return governance.PolicyDecision{}, governanceRepositoryError("get latest policy decision", err)
	}
	return policyDecisionFromRow(row)
}

// ---------- policy rule vocabulary ----------

func policyRuleFromRow(row dbgen.PolicyRule) (governance.PolicyRule, error) {
	return governance.PolicyRule{
		ID: row.RuleID, RuleVersion: row.RuleVersion, Priority: int(row.Priority),
		Match: cloneJSON(row.Match), RiskLevel: governance.RiskLevel(row.OutcomeRiskLevel),
		Routing: governance.RoutingChannel(row.OutcomeRouting), ReasonCode: row.OutcomeReasonCode,
		Explanation: row.OutcomeExplanation,
	}, nil
}

func (store *Store) ListPolicyRules(
	ctx context.Context, ruleVersion string,
) ([]governance.PolicyRule, error) {
	rows, err := store.queries.ListPolicyRules(ctx, ruleVersion)
	if err != nil {
		return nil, governanceRepositoryError("list policy rules", err)
	}
	rules := make([]governance.PolicyRule, 0, len(rows))
	for _, row := range rows {
		rule, mapErr := policyRuleFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (store *Store) GetPolicyRule(
	ctx context.Context, ruleVersion, ruleID string,
) (governance.PolicyRule, error) {
	row, err := store.queries.GetPolicyRule(ctx, dbgen.GetPolicyRuleParams{
		RuleVersion: ruleVersion, RuleID: ruleID,
	})
	if err != nil {
		return governance.PolicyRule{}, governanceRepositoryError("get policy rule", err)
	}
	return policyRuleFromRow(row)
}

// ---------- decision input facts ----------

func (store *Store) SummarizeProposalValidationOutcomes(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) (governanceapp.ProposalValidationOutcomes, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governanceapp.ProposalValidationOutcomes{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal)
	if err != nil {
		return governanceapp.ProposalValidationOutcomes{}, fmt.Errorf("encode proposal ID: %w", err)
	}
	row, err := store.queries.SummarizeProposalValidationOutcomes(ctx, dbgen.SummarizeProposalValidationOutcomesParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
	})
	if err != nil {
		return governanceapp.ProposalValidationOutcomes{}, governanceRepositoryError("summarize validation outcomes", err)
	}
	return governanceapp.ProposalValidationOutcomes{
		RunCount: int(row.RunCount), FailedCount: int(row.FailedCount),
		BlockerCount: int(row.BlockerCount), WarningCount: int(row.WarningCount), InfoCount: int(row.InfoCount),
	}, nil
}

func (store *Store) GetPolicyAssetFacts(
	ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID,
) (governanceapp.PolicyAssetFacts, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governanceapp.PolicyAssetFacts{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	assetIDValue, err := uuidValue(asset)
	if err != nil {
		return governanceapp.PolicyAssetFacts{}, fmt.Errorf("encode asset ID: %w", err)
	}
	row, err := store.queries.GetPolicyAssetFacts(ctx, dbgen.GetPolicyAssetFactsParams{
		WorkspaceID: workspaceID, AssetID: assetIDValue,
	})
	if err != nil {
		return governanceapp.PolicyAssetFacts{}, governanceRepositoryError("get policy asset facts", err)
	}
	var content json.RawMessage
	if len(row.CurrentContent) > 0 {
		content = append(json.RawMessage(nil), row.CurrentContent...)
	}
	return governanceapp.PolicyAssetFacts{
		AssetType: row.AssetType, EvidenceLinkCount: int(row.EvidenceLinkCount),
		CurrentRevisionContent: content,
	}, nil
}

func (store *Store) GetPolicyGovernedObjectAsset(
	ctx context.Context, workspace identity.WorkspaceID, objectUUID string,
) (identity.AssetID, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return identity.AssetID{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	objectID, err := uuidFromString(objectUUID)
	if err != nil {
		return identity.AssetID{}, fmt.Errorf("encode governed object ID: %w", err)
	}
	assetUUID, err := store.queries.GetPolicyGovernedObjectAsset(ctx, dbgen.GetPolicyGovernedObjectAssetParams{
		WorkspaceID: workspaceID, ObjectID: objectID,
	})
	if err != nil {
		return identity.AssetID{}, governanceRepositoryError("get governed object asset", err)
	}
	return identity.AssetIDFromUUIDBytes(assetUUID.Bytes)
}

func (store *Store) GetPolicyRevisionOwnerContent(
	ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID, revision identity.RevisionID,
) (json.RawMessage, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	assetIDValue, err := uuidValue(asset)
	if err != nil {
		return nil, fmt.Errorf("encode asset ID: %w", err)
	}
	revisionID, err := uuidValue(revision)
	if err != nil {
		return nil, fmt.Errorf("encode revision ID: %w", err)
	}
	row, err := store.queries.GetPolicyRevisionOwnerContent(ctx, dbgen.GetPolicyRevisionOwnerContentParams{
		WorkspaceID: workspaceID, AssetID: assetIDValue, RevisionID: revisionID,
	})
	if err != nil {
		return nil, governanceRepositoryError("get revision owner content", err)
	}
	if len(row) == 0 {
		return nil, nil
	}
	return append(json.RawMessage(nil), row...), nil
}
