package postgres

import (
	"context"
	"fmt"
	"strings"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// The T004 validation support surface: idempotent per-validator run lookup,
// the proposal-scoped run list for the read surface and workspace-scoped
// reference resolution over the M1 physical graph, semantic registry and
// T008 governance objects.

func (store *Store) GetProposalValidationRun(
	ctx context.Context,
	workspace identity.WorkspaceID,
	proposal identity.ProposalID,
	validatorID, validatorVersion string,
) (governance.ValidationRun, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.ValidationRun{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal)
	if err != nil {
		return governance.ValidationRun{}, fmt.Errorf("encode proposal ID: %w", err)
	}
	row, err := store.queries.GetProposalValidationRun(ctx, dbgen.GetProposalValidationRunParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
		ValidatorID: validatorID, ValidatorVersion: validatorVersion,
	})
	if err != nil {
		return governance.ValidationRun{}, governanceRepositoryError("get proposal validation run", err)
	}
	return validationRunFromRow(row)
}

func (store *Store) ListProposalValidationRuns(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) ([]governance.ValidationRun, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	proposalID, err := uuidValue(proposal)
	if err != nil {
		return nil, fmt.Errorf("encode proposal ID: %w", err)
	}
	rows, err := store.queries.ListProposalValidationRuns(ctx, dbgen.ListProposalValidationRunsParams{
		WorkspaceID: workspaceID, ProposalID: proposalID,
	})
	if err != nil {
		return nil, governanceRepositoryError("list proposal validation runs", err)
	}
	runs := make([]governance.ValidationRun, 0, len(rows))
	for _, row := range rows {
		mapped, mapErr := validationRunFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		runs = append(runs, mapped)
	}
	return runs, nil
}

func (store *Store) ValidationAssetExists(
	ctx context.Context, workspace identity.WorkspaceID, assetUUID string,
) (bool, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return false, fmt.Errorf("encode workspace ID: %w", err)
	}
	assetID, err := uuidFromString(assetUUID)
	if err != nil {
		return false, fmt.Errorf("encode asset ID: %w", err)
	}
	present, err := store.queries.SemanticAssetExists(ctx, dbgen.SemanticAssetExistsParams{
		WorkspaceID: workspaceID, AssetID: assetID,
	})
	if err != nil {
		return false, governanceRepositoryError("check semantic asset existence", err)
	}
	return present, nil
}

func (store *Store) ValidationAssetRevisionExists(
	ctx context.Context, workspace identity.WorkspaceID, assetUUID, revisionUUID string,
) (bool, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return false, fmt.Errorf("encode workspace ID: %w", err)
	}
	revisionID, err := uuidFromString(revisionUUID)
	if err != nil {
		return false, fmt.Errorf("encode revision ID: %w", err)
	}
	present, err := store.queries.AssetRevisionExists(ctx, dbgen.AssetRevisionExistsParams{
		WorkspaceID: workspaceID, RevisionID: revisionID,
	})
	if err != nil {
		return false, governanceRepositoryError("check asset revision existence", err)
	}
	return present, nil
}

func (store *Store) ValidationGovernedObjectExists(
	ctx context.Context, workspace identity.WorkspaceID,
	objectType governance.TargetObjectType, objectUUID string,
) (bool, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return false, fmt.Errorf("encode workspace ID: %w", err)
	}
	objectID, err := uuidFromString(objectUUID)
	if err != nil {
		return false, fmt.Errorf("encode governed object ID: %w", err)
	}
	spec, err := governedObjectSpecFor(objectType)
	if err != nil {
		return false, err
	}
	present, err := spec.exists(ctx, store.queries, workspaceID, objectID)
	if err != nil {
		return false, governanceRepositoryError("check governed object existence", err)
	}
	return present, nil
}

// validationRevisionExists reports whether an asset revision row exists in
// the workspace, independent of its asset.
func (store *Store) validationRevisionExists(
	ctx context.Context, workspace identity.WorkspaceID, revisionUUID string,
) (bool, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return false, fmt.Errorf("encode workspace ID: %w", err)
	}
	revisionID, err := uuidFromString(revisionUUID)
	if err != nil {
		return false, fmt.Errorf("encode revision ID: %w", err)
	}
	present, err := store.queries.AssetRevisionExists(ctx, dbgen.AssetRevisionExistsParams{
		WorkspaceID: workspaceID, RevisionID: revisionID,
	})
	if err != nil {
		return false, governanceRepositoryError("check asset revision existence", err)
	}
	return present, nil
}

// ResolveValidationReference resolves one TypeID token found in a change-set
// value against the workspace. Unknown prefixes and malformed tokens get
// explicit verdicts so the reference validator can fail closed without
// guessing.
func (store *Store) ResolveValidationReference(
	ctx context.Context, workspace identity.WorkspaceID, token string,
) (governanceapp.ReferenceResolution, error) {
	prefix, _, found := strings.Cut(token, "_")
	if !found {
		return governanceapp.ReferenceResolution{Kind: governanceapp.ReferenceKindMalformed}, nil
	}
	switch identity.Prefix(prefix) {
	case identity.Asset:
		parsed, err := identity.ParseAssetID(token)
		if err != nil {
			return governanceapp.ReferenceResolution{Kind: governanceapp.ReferenceKindMalformed}, nil
		}
		return store.resolveReference(ctx, governanceapp.ReferenceKindAsset, func() (bool, error) {
			return store.ValidationAssetExists(ctx, workspace, parsed.UUID())
		})
	case identity.Revision:
		parsed, err := identity.ParseRevisionID(token)
		if err != nil {
			return governanceapp.ReferenceResolution{Kind: governanceapp.ReferenceKindMalformed}, nil
		}
		return store.resolveReference(ctx, governanceapp.ReferenceKindRevision, func() (bool, error) {
			return store.validationRevisionExists(ctx, workspace, parsed.UUID())
		})
	case identity.PhysicalDataset:
		parsed, err := identity.ParsePhysicalDatasetID(token)
		if err != nil {
			return governanceapp.ReferenceResolution{Kind: governanceapp.ReferenceKindMalformed}, nil
		}
		return store.resolveReference(ctx, governanceapp.ReferenceKindDataset, func() (bool, error) {
			return store.validationPhysicalDatasetExists(ctx, workspace, parsed.UUID())
		})
	case identity.PhysicalField:
		parsed, err := identity.ParsePhysicalFieldID(token)
		if err != nil {
			return governanceapp.ReferenceResolution{Kind: governanceapp.ReferenceKindMalformed}, nil
		}
		return store.resolveReference(ctx, governanceapp.ReferenceKindField, func() (bool, error) {
			return store.validationPhysicalFieldExists(ctx, workspace, parsed.UUID())
		})
	case identity.PhysicalBinding, identity.ModelGrain, identity.EntityKey, identity.JoinContract:
		objectType, typeErr := validationObjectTypeForPrefix(identity.Prefix(prefix))
		if typeErr != nil {
			return governanceapp.ReferenceResolution{Kind: governanceapp.ReferenceKindUnsupported}, nil
		}
		objectUUID, parseErr := validationObjectUUID(identity.Prefix(prefix), token)
		if parseErr != nil {
			return governanceapp.ReferenceResolution{Kind: governanceapp.ReferenceKindMalformed}, nil
		}
		return store.resolveReference(ctx, governanceapp.ReferenceKindGovernedObject, func() (bool, error) {
			return store.ValidationGovernedObjectExists(ctx, workspace, objectType, objectUUID)
		})
	default:
		return governanceapp.ReferenceResolution{Kind: governanceapp.ReferenceKindUnsupported}, nil
	}
}

func (store *Store) resolveReference(
	ctx context.Context,
	kind governanceapp.ReferenceKind,
	check func() (bool, error),
) (governanceapp.ReferenceResolution, error) {
	present, err := check()
	if err != nil {
		return governanceapp.ReferenceResolution{}, fmt.Errorf("resolve %s reference: %w", kind, err)
	}
	return governanceapp.ReferenceResolution{Kind: kind, Exists: present}, nil
}

func (store *Store) validationPhysicalDatasetExists(
	ctx context.Context, workspace identity.WorkspaceID, datasetUUID string,
) (bool, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return false, fmt.Errorf("encode workspace ID: %w", err)
	}
	datasetID, err := uuidFromString(datasetUUID)
	if err != nil {
		return false, fmt.Errorf("encode dataset ID: %w", err)
	}
	present, err := store.queries.PhysicalDatasetExists(ctx, dbgen.PhysicalDatasetExistsParams{
		WorkspaceID: workspaceID, PhysicalDatasetID: datasetID,
	})
	if err != nil {
		return false, governanceRepositoryError("check physical dataset existence", err)
	}
	return present, nil
}

func (store *Store) validationPhysicalFieldExists(
	ctx context.Context, workspace identity.WorkspaceID, fieldUUID string,
) (bool, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return false, fmt.Errorf("encode workspace ID: %w", err)
	}
	fieldID, err := uuidFromString(fieldUUID)
	if err != nil {
		return false, fmt.Errorf("encode field ID: %w", err)
	}
	present, err := store.queries.PhysicalFieldExists(ctx, dbgen.PhysicalFieldExistsParams{
		WorkspaceID: workspaceID, PhysicalFieldID: fieldID,
	})
	if err != nil {
		return false, governanceRepositoryError("check physical field existence", err)
	}
	return present, nil
}

func validationObjectTypeForPrefix(prefix identity.Prefix) (governance.TargetObjectType, error) {
	switch prefix {
	case identity.PhysicalBinding:
		return governance.TargetPhysicalBinding, nil
	case identity.ModelGrain:
		return governance.TargetModelGrain, nil
	case identity.EntityKey:
		return governance.TargetEntityKey, nil
	case identity.JoinContract:
		return governance.TargetJoinContract, nil
	default:
		return "", fmt.Errorf("prefix %q is not a governed object", prefix)
	}
}

func validationObjectUUID(prefix identity.Prefix, token string) (string, error) {
	switch prefix {
	case identity.PhysicalBinding:
		parsed, err := identity.ParsePhysicalBindingID(token)
		if err != nil {
			return "", err
		}
		return parsed.UUID(), nil
	case identity.ModelGrain:
		parsed, err := identity.ParseModelGrainID(token)
		if err != nil {
			return "", err
		}
		return parsed.UUID(), nil
	case identity.EntityKey:
		parsed, err := identity.ParseEntityKeyID(token)
		if err != nil {
			return "", err
		}
		return parsed.UUID(), nil
	case identity.JoinContract:
		parsed, err := identity.ParseJoinContractID(token)
		if err != nil {
			return "", err
		}
		return parsed.UUID(), nil
	default:
		return "", fmt.Errorf("prefix %q is not a governed object", prefix)
	}
}
