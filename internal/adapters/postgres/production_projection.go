package postgres

import (
	"context"

	governance "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/projection"
	"github.com/iiwish/semlia/pkg/identity"
)

func (s *Store) productionReleaseProjection(ctx context.Context, w identity.WorkspaceID, id identity.ReleaseID) (*projection.ProductionRelease, error) {
	release, manifest, _, attrs, err := s.GetProductionRelease(ctx, w, id)
	if err != nil {
		return nil, err
	}
	if !release.IsProductionProtected() {
		return nil, nil
	}
	if manifest == nil || release.ProductionRootReleaseID == nil || release.ProductionRollbackDepth == nil || len(attrs) == 0 {
		return nil, governance.ErrPriorStateUnknown
	}
	result := &projection.ProductionRelease{RootReleaseID: release.ProductionRootReleaseID.String(), RollbackDepth: *release.ProductionRollbackDepth, BeforeManifest: cloneJSON(manifest.BeforeManifestJSON), BeforeManifestDigest: manifest.BeforeManifestDigest, AttributionDigest: manifest.AttributionDigest, Proposals: []projection.ProductionProposal{}}
	if release.ProductionRollbackParentID != nil {
		value := release.ProductionRollbackParentID.String()
		result.RollbackParentID = &value
	}
	if manifest.BeforeReleaseID != nil {
		value := manifest.BeforeReleaseID.String()
		result.BeforeReleaseID = &value
	}
	for _, attr := range attrs {
		result.Proposals = append(result.Proposals, projection.ProductionProposal{ProposalID: attr.ProposalID.String(), OperationID: attr.OperationID.String(), ProductionVersion: attr.ProductionVersion, SetDigest: attr.SetDigest, ContentDigest: attr.ProposalContentDigest, AuthorPrincipalID: attr.AuthorPrincipalID.String(), AttemptNo: attr.AttemptNo, ValidationDigest: attr.ValidationDigest, ReviewIDs: cloneJSON(attr.ReviewIDs), Role: string(attr.Role)})
	}
	return result, nil
}
