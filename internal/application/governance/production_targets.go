package governance

import (
	"encoding/json"
	"time"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func buildProductionTarget(workspace identity.WorkspaceID, operation identity.ProductionOperationID, actor identity.PrincipalID, version int, decl domain.TargetDeclaration, localIDs map[string]string, baseline domain.ProductionBaseline) (domain.ProductionTarget, *domain.Proposal, error) {
	target := domain.ProductionTarget{WorkspaceID: workspace, OperationID: operation, Version: version, LocalKey: decl.LocalKey, Kind: decl.Kind, Intent: decl.Intent, TargetID: localIDs[decl.LocalKey], IdentityKey: decl.IdentityKey, BaseRevisionID: decl.BaseRevisionID, BaseObjectVersion: decl.BaseObjectVersion, Declaration: &decl, Outcome: domain.ProductionOutcomeProposal}
	var err error
	target.ContentJSON, err = domain.ResolveLocalReferences(decl.Content, localIDs)
	if err != nil {
		return target, nil, err
	}
	target.ContentDigest, err = domain.DigestJSON(target.ContentJSON)
	if err != nil {
		return target, nil, err
	}
	if decl.Intent == domain.ProductionIntentUpdate {
		base, ok := baseline.Targets[decl.LocalKey]
		if !ok {
			return target, nil, domain.ErrPriorStateUnknown
		}
		var noChange bool
		target.Changes, noChange, err = domain.ReplayProductionChanges(decl, base.Content, localIDs)
		if err != nil {
			return target, nil, err
		}
		target.RegistryWriteVersion = base.RegistryWriteVersion
		if noChange {
			target.Outcome = domain.ProductionOutcomeNoChange
			return target, nil, nil
		}
	}
	id, err := identity.NewProposalID()
	if err != nil {
		return target, nil, err
	}
	target.ProposalID = &id
	now := time.Now().UTC()
	proposal := domain.Proposal{ID: id, WorkspaceID: workspace, TargetObjectType: domain.TargetObjectType(decl.Kind), TargetObjectID: target.TargetID, State: domain.ProposalDraft, Title: decl.Title, CreatedBy: actor.String(), Intent: decl.Intent, ProductionOperationID: &operation, ProductionVersion: &version, CreatedAt: now, UpdatedAt: now, BaseObjectVersion: target.BaseObjectVersion}
	if decl.Kind == domain.TargetKindSemanticAsset {
		asset, err := identity.ParseAssetID(target.TargetID)
		if err != nil {
			return target, nil, domain.ErrInvalidArgument
		}
		proposal.AssetID = &asset
		if decl.BaseRevisionID != nil {
			revision, err := identity.ParseRevisionID(*decl.BaseRevisionID)
			if err != nil {
				return target, nil, domain.ErrInvalidArgument
			}
			proposal.BaseRevisionID = &revision
		}
	}
	if decl.Intent == domain.ProductionIntentCreate {
		proposal.CreationContent = target.ContentJSON
		if decl.ReuseIdentity != nil {
			target.RegistryWriteVersion = baseline.Targets[decl.LocalKey].RegistryWriteVersion
			proposal.ReintroductionCreationReleaseID = &decl.ReuseIdentity.CreationReleaseID
			proposal.ReintroductionAbsenceReleaseID = &decl.ReuseIdentity.AbsenceReleaseID
		}
	}
	for i := range target.Changes {
		changeID, err := identity.NewProposalChangeID()
		if err != nil {
			return target, nil, err
		}
		target.Changes[i].ID = changeID
		target.Changes[i].WorkspaceID = workspace
		target.Changes[i].ProposalID = id
		target.Changes[i].CreatedAt = now
	}
	return target, &proposal, nil
}

func productionSetDigest(inputDigest, requestDigest string, baseline domain.ProductionBaseline, targets []domain.ProductionTarget) (string, error) {
	// Declaration bytes and source input are bound by requestDigest. Include
	// resolved content, stable IDs, exact head and server registry CAS tokens.
	type member struct {
		LocalKey             string               `json:"localKey"`
		Kind                 string               `json:"kind"`
		Intent               string               `json:"intent"`
		TargetID             string               `json:"targetId"`
		ProposalID           *identity.ProposalID `json:"proposalId"`
		ContentDigest        string               `json:"contentDigest"`
		RegistryWriteVersion *int                 `json:"registryWriteVersion"`
		Outcome              string               `json:"outcome"`
	}
	members := make([]member, 0, len(targets))
	for _, t := range targets {
		members = append(members, member{t.LocalKey, t.Kind, t.Intent, t.TargetID, t.ProposalID, t.ContentDigest, t.RegistryWriteVersion, t.Outcome})
	}
	data, err := json.Marshal(struct {
		Schema        string          `json:"schema"`
		InputDigest   string          `json:"inputDigest"`
		RequestDigest string          `json:"requestDigest"`
		Baseline      json.RawMessage `json:"baseline"`
		Members       []member        `json:"members"`
	}{"semlia.production/v1", inputDigest, requestDigest, baseline.CanonicalJSON, members})
	if err != nil {
		return "", err
	}
	return domain.DigestJSON(data)
}

func checkProductionPrimaryOutcomes(candidates []domain.CandidateDeclaration, targets []domain.ProductionTarget) error {
	changed := map[string]bool{}
	for _, target := range targets {
		if target.ProposalID != nil {
			changed[target.LocalKey] = true
		}
	}
	if len(changed) == 0 {
		return nil
	}
	for _, candidate := range candidates {
		if !changed[candidate.PrimaryTargetKey] {
			return domain.ErrContentMismatch
		}
	}
	return nil
}
