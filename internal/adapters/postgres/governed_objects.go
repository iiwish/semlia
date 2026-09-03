package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ governanceapp.GovernedObjectRepository = (*Store)(nil)

// governedObjectSpec wires one governance object table into the shared
// persistence pipeline below. The pipeline (validation, transaction
// discipline, FOR UPDATE locking, version expectation, audit-only fact) is
// written once; the spec carries only the per-table sqlc wiring and row
// mapping, so the four types share one implementation.
type governedObjectSpec struct {
	objectType governance.TargetObjectType
	prefix     identity.Prefix
	exists     func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (bool, error)
	lock       func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (int32, error)
	apply      func(ctx context.Context, queries *dbgen.Queries, params governedApplyParams) (int64, error)
	get        func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (governance.GovernedObject, error)
	list       func(ctx context.Context, queries *dbgen.Queries, workspaceID pgtype.UUID) ([]governance.GovernedObject, error)
	create     func(ctx context.Context, queries *dbgen.Queries, object governance.GovernedObject) (governance.GovernedObject, error)
}

// governedApplyParams carries the shared apply shape: one version bump of
// exactly one, guarded by the expected version under the row lock.
type governedApplyParams struct {
	WorkspaceID     pgtype.UUID
	ObjectID        pgtype.UUID
	ExpectedVersion int32
	Content         []byte
	UpdatedAt       pgtype.Timestamptz
}

var governedObjectSpecs = map[governance.TargetObjectType]governedObjectSpec{
	governance.TargetPhysicalBinding: {
		objectType: governance.TargetPhysicalBinding, prefix: identity.PhysicalBinding,
		exists: func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (bool, error) {
			return queries.PhysicalBindingExists(ctx, dbgen.PhysicalBindingExistsParams{
				WorkspaceID: workspaceID, PhysicalBindingID: objectID,
			})
		},
		lock: func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (int32, error) {
			return queries.LockPhysicalBindingForUpdate(ctx, dbgen.LockPhysicalBindingForUpdateParams{
				WorkspaceID: workspaceID, PhysicalBindingID: objectID,
			})
		},
		apply: func(ctx context.Context, queries *dbgen.Queries, params governedApplyParams) (int64, error) {
			return queries.ApplyPhysicalBindingChange(ctx, dbgen.ApplyPhysicalBindingChangeParams{
				WorkspaceID: params.WorkspaceID, PhysicalBindingID: params.ObjectID,
				ExpectedVersion: params.ExpectedVersion, Content: params.Content,
				UpdatedAt: params.UpdatedAt,
			})
		},
		get: func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (governance.GovernedObject, error) {
			row, err := queries.GetPhysicalBinding(ctx, dbgen.GetPhysicalBindingParams{
				WorkspaceID: workspaceID, PhysicalBindingID: objectID,
			})
			if err != nil {
				return governance.GovernedObject{}, err
			}
			return physicalBindingFromRow(row)
		},
		list: func(ctx context.Context, queries *dbgen.Queries, workspaceID pgtype.UUID) ([]governance.GovernedObject, error) {
			rows, err := queries.ListPhysicalBindings(ctx, workspaceID)
			if err != nil {
				return nil, err
			}
			objects := make([]governance.GovernedObject, 0, len(rows))
			for _, row := range rows {
				object, mapErr := physicalBindingFromRow(row)
				if mapErr != nil {
					return nil, mapErr
				}
				objects = append(objects, object)
			}
			return objects, nil
		},
		create: func(ctx context.Context, queries *dbgen.Queries, object governance.GovernedObject) (governance.GovernedObject, error) {
			binding := *object.PhysicalBinding
			id, err := uuidValue(binding.ID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode physical binding ID: %w", err)
			}
			workspaceID, err := uuidValue(binding.WorkspaceID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode workspace ID: %w", err)
			}
			assetID, err := uuidValue(binding.AssetID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode asset ID: %w", err)
			}
			datasetID, err := uuidValue(binding.DatasetID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode dataset ID: %w", err)
			}
			var fieldID pgtype.UUID
			if binding.FieldID != nil {
				if fieldID, err = uuidValue(*binding.FieldID); err != nil {
					return governance.GovernedObject{}, fmt.Errorf("encode field ID: %w", err)
				}
			}
			row, err := queries.CreatePhysicalBinding(ctx, dbgen.CreatePhysicalBindingParams{
				ID: id, WorkspaceID: workspaceID, AssetID: assetID, DatasetID: datasetID,
				FieldID: fieldID, Transform: optionalTextPointer(binding.Transform),
				RetiredAt: optionalTimestamp(binding.RetiredAt), Version: int32(binding.Version),
				Content: objectJSON(binding.Content), CreatedBy: binding.CreatedBy,
				CreatedAt: timestamp(binding.CreatedAt), UpdatedAt: timestamp(binding.UpdatedAt),
			})
			if err != nil {
				return governance.GovernedObject{}, err
			}
			return physicalBindingFromRow(row)
		},
	},
	governance.TargetModelGrain: {
		objectType: governance.TargetModelGrain, prefix: identity.ModelGrain,
		exists: func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (bool, error) {
			return queries.ModelGrainExists(ctx, dbgen.ModelGrainExistsParams{
				WorkspaceID: workspaceID, ModelGrainID: objectID,
			})
		},
		lock: func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (int32, error) {
			return queries.LockModelGrainForUpdate(ctx, dbgen.LockModelGrainForUpdateParams{
				WorkspaceID: workspaceID, ModelGrainID: objectID,
			})
		},
		apply: func(ctx context.Context, queries *dbgen.Queries, params governedApplyParams) (int64, error) {
			return queries.ApplyModelGrainChange(ctx, dbgen.ApplyModelGrainChangeParams{
				WorkspaceID: params.WorkspaceID, ModelGrainID: params.ObjectID,
				ExpectedVersion: params.ExpectedVersion, Content: params.Content,
				UpdatedAt: params.UpdatedAt,
			})
		},
		get: func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (governance.GovernedObject, error) {
			row, err := queries.GetModelGrain(ctx, dbgen.GetModelGrainParams{
				WorkspaceID: workspaceID, ModelGrainID: objectID,
			})
			if err != nil {
				return governance.GovernedObject{}, err
			}
			return modelGrainFromRow(row)
		},
		list: func(ctx context.Context, queries *dbgen.Queries, workspaceID pgtype.UUID) ([]governance.GovernedObject, error) {
			rows, err := queries.ListModelGrains(ctx, workspaceID)
			if err != nil {
				return nil, err
			}
			objects := make([]governance.GovernedObject, 0, len(rows))
			for _, row := range rows {
				object, mapErr := modelGrainFromRow(row)
				if mapErr != nil {
					return nil, mapErr
				}
				objects = append(objects, object)
			}
			return objects, nil
		},
		create: func(ctx context.Context, queries *dbgen.Queries, object governance.GovernedObject) (governance.GovernedObject, error) {
			grain := *object.ModelGrain
			id, err := uuidValue(grain.ID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode model grain ID: %w", err)
			}
			workspaceID, err := uuidValue(grain.WorkspaceID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode workspace ID: %w", err)
			}
			assetID, err := uuidValue(grain.AssetID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode asset ID: %w", err)
			}
			var documentedBy pgtype.UUID
			if grain.DocumentedBy != nil {
				if documentedBy, err = uuidValue(*grain.DocumentedBy); err != nil {
					return governance.GovernedObject{}, fmt.Errorf("encode documented-by ID: %w", err)
				}
			}
			refs, err := fieldRefsToJSON(grain.GrainFieldRefs)
			if err != nil {
				return governance.GovernedObject{}, err
			}
			row, err := queries.CreateModelGrain(ctx, dbgen.CreateModelGrainParams{
				ID: id, WorkspaceID: workspaceID, AssetID: assetID,
				GrainExpression: grain.GrainExpression, GrainFieldRefs: refs,
				DocumentedBy: documentedBy, Version: int32(grain.Version),
				Content: objectJSON(grain.Content), CreatedBy: grain.CreatedBy,
				CreatedAt: timestamp(grain.CreatedAt), UpdatedAt: timestamp(grain.UpdatedAt),
			})
			if err != nil {
				return governance.GovernedObject{}, err
			}
			return modelGrainFromRow(row)
		},
	},
	governance.TargetEntityKey: {
		objectType: governance.TargetEntityKey, prefix: identity.EntityKey,
		exists: func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (bool, error) {
			return queries.EntityKeyExists(ctx, dbgen.EntityKeyExistsParams{
				WorkspaceID: workspaceID, EntityKeyID: objectID,
			})
		},
		lock: func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (int32, error) {
			return queries.LockEntityKeyForUpdate(ctx, dbgen.LockEntityKeyForUpdateParams{
				WorkspaceID: workspaceID, EntityKeyID: objectID,
			})
		},
		apply: func(ctx context.Context, queries *dbgen.Queries, params governedApplyParams) (int64, error) {
			return queries.ApplyEntityKeyChange(ctx, dbgen.ApplyEntityKeyChangeParams{
				WorkspaceID: params.WorkspaceID, EntityKeyID: params.ObjectID,
				ExpectedVersion: params.ExpectedVersion, Content: params.Content,
				UpdatedAt: params.UpdatedAt,
			})
		},
		get: func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (governance.GovernedObject, error) {
			row, err := queries.GetEntityKey(ctx, dbgen.GetEntityKeyParams{
				WorkspaceID: workspaceID, EntityKeyID: objectID,
			})
			if err != nil {
				return governance.GovernedObject{}, err
			}
			return entityKeyFromRow(row)
		},
		list: func(ctx context.Context, queries *dbgen.Queries, workspaceID pgtype.UUID) ([]governance.GovernedObject, error) {
			rows, err := queries.ListEntityKeys(ctx, workspaceID)
			if err != nil {
				return nil, err
			}
			objects := make([]governance.GovernedObject, 0, len(rows))
			for _, row := range rows {
				object, mapErr := entityKeyFromRow(row)
				if mapErr != nil {
					return nil, mapErr
				}
				objects = append(objects, object)
			}
			return objects, nil
		},
		create: func(ctx context.Context, queries *dbgen.Queries, object governance.GovernedObject) (governance.GovernedObject, error) {
			key := *object.EntityKey
			id, err := uuidValue(key.ID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode entity key ID: %w", err)
			}
			workspaceID, err := uuidValue(key.WorkspaceID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode workspace ID: %w", err)
			}
			assetID, err := uuidValue(key.AssetID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode asset ID: %w", err)
			}
			refs, err := fieldRefsToJSON(key.KeyFieldRefs)
			if err != nil {
				return governance.GovernedObject{}, err
			}
			row, err := queries.CreateEntityKey(ctx, dbgen.CreateEntityKeyParams{
				ID: id, WorkspaceID: workspaceID, AssetID: assetID,
				KeyFieldRefs: refs, UniquenessSemantics: string(key.UniquenessSemantics),
				Version: int32(key.Version), Content: objectJSON(key.Content),
				CreatedBy: key.CreatedBy, CreatedAt: timestamp(key.CreatedAt), UpdatedAt: timestamp(key.UpdatedAt),
			})
			if err != nil {
				return governance.GovernedObject{}, err
			}
			return entityKeyFromRow(row)
		},
	},
	governance.TargetJoinContract: {
		objectType: governance.TargetJoinContract, prefix: identity.JoinContract,
		exists: func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (bool, error) {
			return queries.JoinContractExists(ctx, dbgen.JoinContractExistsParams{
				WorkspaceID: workspaceID, JoinContractID: objectID,
			})
		},
		lock: func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (int32, error) {
			return queries.LockJoinContractForUpdate(ctx, dbgen.LockJoinContractForUpdateParams{
				WorkspaceID: workspaceID, JoinContractID: objectID,
			})
		},
		apply: func(ctx context.Context, queries *dbgen.Queries, params governedApplyParams) (int64, error) {
			return queries.ApplyJoinContractChange(ctx, dbgen.ApplyJoinContractChangeParams{
				WorkspaceID: params.WorkspaceID, JoinContractID: params.ObjectID,
				ExpectedVersion: params.ExpectedVersion, Content: params.Content,
				UpdatedAt: params.UpdatedAt,
			})
		},
		get: func(ctx context.Context, queries *dbgen.Queries, workspaceID, objectID pgtype.UUID) (governance.GovernedObject, error) {
			row, err := queries.GetJoinContract(ctx, dbgen.GetJoinContractParams{
				WorkspaceID: workspaceID, JoinContractID: objectID,
			})
			if err != nil {
				return governance.GovernedObject{}, err
			}
			return joinContractFromRow(row)
		},
		list: func(ctx context.Context, queries *dbgen.Queries, workspaceID pgtype.UUID) ([]governance.GovernedObject, error) {
			rows, err := queries.ListJoinContracts(ctx, workspaceID)
			if err != nil {
				return nil, err
			}
			objects := make([]governance.GovernedObject, 0, len(rows))
			for _, row := range rows {
				object, mapErr := joinContractFromRow(row)
				if mapErr != nil {
					return nil, mapErr
				}
				objects = append(objects, object)
			}
			return objects, nil
		},
		create: func(ctx context.Context, queries *dbgen.Queries, object governance.GovernedObject) (governance.GovernedObject, error) {
			contract := *object.JoinContract
			id, err := uuidValue(contract.ID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode join contract ID: %w", err)
			}
			workspaceID, err := uuidValue(contract.WorkspaceID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode workspace ID: %w", err)
			}
			leftDatasetID, err := uuidValue(contract.LeftDatasetID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode left dataset ID: %w", err)
			}
			rightDatasetID, err := uuidValue(contract.RightDatasetID)
			if err != nil {
				return governance.GovernedObject{}, fmt.Errorf("encode right dataset ID: %w", err)
			}
			leftRefs, err := fieldRefsToJSON(contract.LeftFieldRefs)
			if err != nil {
				return governance.GovernedObject{}, err
			}
			rightRefs, err := fieldRefsToJSON(contract.RightFieldRefs)
			if err != nil {
				return governance.GovernedObject{}, err
			}
			row, err := queries.CreateJoinContract(ctx, dbgen.CreateJoinContractParams{
				ID: id, WorkspaceID: workspaceID,
				LeftDatasetID: leftDatasetID, RightDatasetID: rightDatasetID,
				LeftFieldRefs: leftRefs, RightFieldRefs: rightRefs,
				JoinType: string(contract.JoinType), Cardinality: string(contract.Cardinality),
				JoinExpression: contract.JoinExpression, ContractNotes: optionalTextPointer(contract.ContractNotes),
				Version: int32(contract.Version), Content: objectJSON(contract.Content),
				CreatedBy: contract.CreatedBy, CreatedAt: timestamp(contract.CreatedAt), UpdatedAt: timestamp(contract.UpdatedAt),
			})
			if err != nil {
				return governance.GovernedObject{}, err
			}
			return joinContractFromRow(row)
		},
	},
}

func governedObjectSpecFor(objectType governance.TargetObjectType) (governedObjectSpec, error) {
	spec, ok := governedObjectSpecs[objectType]
	if !ok {
		return governedObjectSpec{}, fmt.Errorf("%w: %q is not a governed object type", governance.ErrInvalidArgument, objectType)
	}
	return spec, nil
}

// governedTargetExists verifies, inside the proposal submission transaction,
// that a governed object target exists in the same workspace.
func governedTargetExists(
	ctx context.Context, queries *dbgen.Queries, objectType governance.TargetObjectType, workspaceID, objectID pgtype.UUID,
) error {
	spec, err := governedObjectSpecFor(objectType)
	if err != nil {
		return err
	}
	present, err := spec.exists(ctx, queries, workspaceID, objectID)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("%w: %s target %s does not exist in the workspace",
			governance.ErrNotFound, objectType, objectID.String())
	}
	return nil
}

// CreateGovernedObject persists a new governed object. Validation happens in
// the domain before any database write; error mapping follows the shared
// governance repository discipline.
func (store *Store) CreateGovernedObject(ctx context.Context, object governance.GovernedObject) (governance.GovernedObject, error) {
	if err := object.Validate(); err != nil {
		return governance.GovernedObject{}, err
	}
	spec, err := governedObjectSpecFor(object.Type)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	created, err := spec.create(ctx, store.queries, object)
	if err != nil {
		return governance.GovernedObject{}, governanceRepositoryError("create governed object", err)
	}
	return created, nil
}

func (store *Store) GetGovernedObject(
	ctx context.Context, workspace identity.WorkspaceID, objectType governance.TargetObjectType, objectID string,
) (governance.GovernedObject, error) {
	spec, err := governedObjectSpecFor(objectType)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.GovernedObject{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	parsedID, err := parseGovernedObjectUUID(spec.prefix, objectID)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	object, err := spec.get(ctx, store.queries, workspaceID, parsedID)
	if err != nil {
		return governance.GovernedObject{}, governanceRepositoryError("get governed object", err)
	}
	return object, nil
}

func (store *Store) ListGovernedObjects(
	ctx context.Context, workspace identity.WorkspaceID, objectType governance.TargetObjectType,
) ([]governance.GovernedObject, error) {
	spec, err := governedObjectSpecFor(objectType)
	if err != nil {
		return nil, err
	}
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	objects, err := spec.list(ctx, store.queries, workspaceID)
	if err != nil {
		return nil, governanceRepositoryError("list governed objects", err)
	}
	return objects, nil
}

// ApplyGovernedChange is the single governed mutation path: one transaction,
// FOR UPDATE row lock, expected-version check, exactly one version bump, the
// content patch and one audit-only fact committed atomically. A concurrent
// application loses the version expectation and is rejected with the
// invariant error.
func (store *Store) ApplyGovernedChange(ctx context.Context, command governanceapp.GovernedChangeCommand) (governance.GovernedObject, error) {
	spec, err := governedObjectSpecFor(command.ObjectType)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return governance.GovernedObject{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	objectID, err := parseGovernedObjectUUID(spec.prefix, command.ObjectID)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.GovernedObject{}, governanceRepositoryError("begin governed change", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	version, err := spec.lock(ctx, queries, workspaceID, objectID)
	if err != nil {
		return governance.GovernedObject{}, governanceRepositoryError("lock governed object", err)
	}
	if int(version) != command.ExpectedVersion {
		return governance.GovernedObject{}, fmt.Errorf(
			"%w: governed object version %d does not match expected %d (concurrent application)",
			governance.ErrInvariant, version, command.ExpectedVersion)
	}
	rows, err := spec.apply(ctx, queries, governedApplyParams{
		WorkspaceID: workspaceID, ObjectID: objectID, ExpectedVersion: int32(command.ExpectedVersion),
		Content: objectJSON(command.Patch.Content), UpdatedAt: timestamp(command.UpdatedAt),
	})
	if err != nil {
		return governance.GovernedObject{}, governanceRepositoryError("apply governed change", err)
	}
	if rows != 1 {
		return governance.GovernedObject{}, fmt.Errorf(
			"%w: governed change updated %d rows", governance.ErrInvariant, rows)
	}
	object, err := spec.get(ctx, queries, workspaceID, objectID)
	if err != nil {
		return governance.GovernedObject{}, governanceRepositoryError("reload governed object", err)
	}
	if err := createGovernedObjectAuditEvent(ctx, queries, governedObjectEvent{
		WorkspaceID: command.WorkspaceID, ObjectType: string(command.ObjectType),
		ObjectID: command.ObjectID, AuditID: command.AuditEventID, Action: "applied",
		Version: versionOf(object), Summary: command.Patch.Summary,
		Actor: command.Actor, TraceID: command.TraceID, CreatedAt: command.UpdatedAt,
	}); err != nil {
		return governance.GovernedObject{}, governanceRepositoryError("record governed change audit fact", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.GovernedObject{}, governanceRepositoryError("commit governed change", err)
	}
	return object, nil
}

func parseGovernedObjectUUID(prefix identity.Prefix, value string) (pgtype.UUID, error) {
	parsed, err := identity.Parse(prefix, value)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("%w: governed object ID %q: %v", governance.ErrInvalidArgument, value, err)
	}
	return uuidValue(parsed)
}

func fieldRefsToJSON(refs []identity.PhysicalFieldID) ([]byte, error) {
	if refs == nil {
		refs = []identity.PhysicalFieldID{}
	}
	return json.Marshal(refs)
}

func fieldRefsFromJSON(value []byte) ([]identity.PhysicalFieldID, error) {
	var raw []string
	if err := json.Unmarshal(value, &raw); err != nil {
		return nil, fmt.Errorf("decode physical field references: %w", err)
	}
	refs := make([]identity.PhysicalFieldID, 0, len(raw))
	for _, item := range raw {
		ref, err := identity.ParsePhysicalFieldID(item)
		if err != nil {
			return nil, fmt.Errorf("decode physical field reference %q: %w", item, err)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func physicalBindingFromRow(row dbgen.PhysicalBinding) (governance.GovernedObject, error) {
	id, err := identity.PhysicalBindingIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	assetID, err := identity.AssetIDFromUUIDBytes(row.AssetID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	datasetID, err := identity.PhysicalDatasetIDFromUUIDBytes(row.DatasetID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	binding := governance.PhysicalBinding{
		ID: id, WorkspaceID: workspaceID, AssetID: assetID, DatasetID: datasetID,
		Version: int(row.Version), Content: cloneJSON(row.Content), CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.FieldID.Valid {
		fieldID, fieldErr := identity.PhysicalFieldIDFromUUIDBytes(row.FieldID.Bytes)
		if fieldErr != nil {
			return governance.GovernedObject{}, fieldErr
		}
		binding.FieldID = &fieldID
	}
	if row.Transform.Valid {
		transform := row.Transform.String
		binding.Transform = &transform
	}
	if row.RetiredAt.Valid {
		retiredAt := row.RetiredAt.Time.UTC()
		binding.RetiredAt = &retiredAt
	}
	return governance.GovernedObject{Type: governance.TargetPhysicalBinding, PhysicalBinding: &binding}, nil
}

func modelGrainFromRow(row dbgen.ModelGrain) (governance.GovernedObject, error) {
	id, err := identity.ModelGrainIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	assetID, err := identity.AssetIDFromUUIDBytes(row.AssetID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	refs, err := fieldRefsFromJSON(row.GrainFieldRefs)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	grain := governance.ModelGrain{
		ID: id, WorkspaceID: workspaceID, AssetID: assetID,
		GrainExpression: row.GrainExpression, GrainFieldRefs: refs,
		Version: int(row.Version), Content: cloneJSON(row.Content), CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.DocumentedBy.Valid {
		evidenceID, evidenceErr := identity.EvidenceIDFromUUIDBytes(row.DocumentedBy.Bytes)
		if evidenceErr != nil {
			return governance.GovernedObject{}, evidenceErr
		}
		grain.DocumentedBy = &evidenceID
	}
	return governance.GovernedObject{Type: governance.TargetModelGrain, ModelGrain: &grain}, nil
}

func entityKeyFromRow(row dbgen.EntityKey) (governance.GovernedObject, error) {
	id, err := identity.EntityKeyIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	assetID, err := identity.AssetIDFromUUIDBytes(row.AssetID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	refs, err := fieldRefsFromJSON(row.KeyFieldRefs)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	key := governance.EntityKey{
		ID: id, WorkspaceID: workspaceID, AssetID: assetID,
		KeyFieldRefs: refs, UniquenessSemantics: governance.UniquenessSemantics(row.UniquenessSemantics),
		Version: int(row.Version), Content: cloneJSON(row.Content), CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	return governance.GovernedObject{Type: governance.TargetEntityKey, EntityKey: &key}, nil
}

func joinContractFromRow(row dbgen.JoinContract) (governance.GovernedObject, error) {
	id, err := identity.JoinContractIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	leftDatasetID, err := identity.PhysicalDatasetIDFromUUIDBytes(row.LeftDatasetID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	rightDatasetID, err := identity.PhysicalDatasetIDFromUUIDBytes(row.RightDatasetID.Bytes)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	leftRefs, err := fieldRefsFromJSON(row.LeftFieldRefs)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	rightRefs, err := fieldRefsFromJSON(row.RightFieldRefs)
	if err != nil {
		return governance.GovernedObject{}, err
	}
	contract := governance.JoinContract{
		ID: id, WorkspaceID: workspaceID,
		LeftDatasetID: leftDatasetID, RightDatasetID: rightDatasetID,
		LeftFieldRefs: leftRefs, RightFieldRefs: rightRefs,
		JoinType: governance.JoinType(row.JoinType), Cardinality: governance.JoinCardinality(row.Cardinality),
		JoinExpression: row.JoinExpression,
		Version:        int(row.Version), Content: cloneJSON(row.Content), CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.ContractNotes.Valid {
		notes := row.ContractNotes.String
		contract.ContractNotes = &notes
	}
	return governance.GovernedObject{Type: governance.TargetJoinContract, JoinContract: &contract}, nil
}

func versionOf(object governance.GovernedObject) int {
	switch object.Type {
	case governance.TargetPhysicalBinding:
		return object.PhysicalBinding.Version
	case governance.TargetModelGrain:
		return object.ModelGrain.Version
	case governance.TargetEntityKey:
		return object.EntityKey.Version
	case governance.TargetJoinContract:
		return object.JoinContract.Version
	}
	return 0
}
