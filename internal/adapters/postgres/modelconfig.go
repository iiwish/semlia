package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ governanceapp.ModelConfigRepository = (*Store)(nil)
var _ governanceapp.GenerationRepository = (*Store)(nil)

// ---------- model providers ----------

func (store *Store) CreateModelProvider(
	ctx context.Context, provider governance.ModelProvider,
) (governance.ModelProvider, error) {
	if err := provider.Validate(); err != nil {
		return governance.ModelProvider{}, err
	}
	workspaceID, err := uuidValue(provider.WorkspaceID)
	if err != nil {
		return governance.ModelProvider{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	providerID, err := uuidValue(provider.ID)
	if err != nil {
		return governance.ModelProvider{}, fmt.Errorf("encode provider ID: %w", err)
	}
	row, err := store.queries.CreateModelProvider(ctx, dbgen.CreateModelProviderParams{
		ID: providerID, WorkspaceID: workspaceID, Protocol: string(provider.Protocol),
		DisplayName: provider.DisplayName, BaseUrl: optionalTextPointer(provider.BaseURL),
		CredentialEnv: provider.CredentialEnv, CredentialRevision: provider.CredentialRevision,
		Enabled:   provider.Enabled,
		CreatedAt: timestamp(provider.CreatedAt), UpdatedAt: timestamp(provider.UpdatedAt),
	})
	if err != nil {
		return governance.ModelProvider{}, governanceRepositoryError("create model provider", err)
	}
	return modelProviderFromRow(row)
}

func (store *Store) GetModelProvider(
	ctx context.Context, workspace identity.WorkspaceID, provider identity.ModelProviderID,
) (governance.ModelProvider, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.ModelProvider{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	providerID, err := uuidValue(provider)
	if err != nil {
		return governance.ModelProvider{}, fmt.Errorf("encode provider ID: %w", err)
	}
	row, err := store.queries.GetModelProvider(ctx, dbgen.GetModelProviderParams{
		WorkspaceID: workspaceID, ProviderID: providerID,
	})
	if err != nil {
		return governance.ModelProvider{}, governanceRepositoryError("get model provider", err)
	}
	return modelProviderFromRow(row)
}

func (store *Store) ListModelProviders(
	ctx context.Context, workspace identity.WorkspaceID,
) ([]governance.ModelProvider, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	rows, err := store.queries.ListModelProviders(ctx, workspaceID)
	if err != nil {
		return nil, governanceRepositoryError("list model providers", err)
	}
	providers := make([]governance.ModelProvider, 0, len(rows))
	for _, row := range rows {
		provider, decodeErr := modelProviderFromRow(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

func (store *Store) UpdateModelProvider(
	ctx context.Context, command governanceapp.UpdateModelProviderCommand,
) (governance.ModelProvider, error) {
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return governance.ModelProvider{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	providerID, err := uuidValue(command.ProviderID)
	if err != nil {
		return governance.ModelProvider{}, fmt.Errorf("encode provider ID: %w", err)
	}
	row, err := store.queries.UpdateModelProvider(ctx, dbgen.UpdateModelProviderParams{
		WorkspaceID: workspaceID, ProviderID: providerID,
		DisplayName: command.DisplayName, BaseUrl: optionalTextPointer(command.BaseURL),
		CredentialEnv: command.CredentialEnv, CredentialRevision: command.CredentialRevision,
		Enabled: command.Enabled, UpdatedAt: timestamp(command.UpdatedAt),
	})
	if err != nil {
		return governance.ModelProvider{}, governanceRepositoryError("update model provider", err)
	}
	return modelProviderFromRow(row)
}

// ---------- model settings ----------

func (store *Store) CreateModelSetting(
	ctx context.Context, setting governance.ModelSetting,
) (governance.ModelSetting, error) {
	if err := setting.Validate(); err != nil {
		return governance.ModelSetting{}, err
	}
	workspaceID, err := uuidValue(setting.WorkspaceID)
	if err != nil {
		return governance.ModelSetting{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	settingID, err := uuidValue(setting.ID)
	if err != nil {
		return governance.ModelSetting{}, fmt.Errorf("encode setting ID: %w", err)
	}
	providerID, err := uuidValue(setting.ProviderID)
	if err != nil {
		return governance.ModelSetting{}, fmt.Errorf("encode provider ID: %w", err)
	}
	row, err := store.queries.CreateModelSetting(ctx, dbgen.CreateModelSettingParams{
		ID: settingID, WorkspaceID: workspaceID, ProviderID: providerID,
		Kind: string(setting.Kind), Model: setting.Model, Enabled: setting.Enabled,
		IsDefault: setting.IsDefault, Capability: setting.Capability,
		TokenLimit: int32(setting.TokenLimit), EmbeddingDimension: optionalInt4(setting.EmbeddingDimension),
		CreatedAt: timestamp(setting.CreatedAt), UpdatedAt: timestamp(setting.UpdatedAt),
	})
	if err != nil {
		return governance.ModelSetting{}, governanceRepositoryError("create model setting", err)
	}
	return modelSettingFromRow(row)
}

func (store *Store) GetModelSetting(
	ctx context.Context, workspace identity.WorkspaceID, setting identity.ModelSettingID,
) (governance.ModelSetting, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.ModelSetting{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	settingID, err := uuidValue(setting)
	if err != nil {
		return governance.ModelSetting{}, fmt.Errorf("encode setting ID: %w", err)
	}
	row, err := store.queries.GetModelSetting(ctx, dbgen.GetModelSettingParams{
		WorkspaceID: workspaceID, SettingID: settingID,
	})
	if err != nil {
		return governance.ModelSetting{}, governanceRepositoryError("get model setting", err)
	}
	return modelSettingFromRow(row)
}

func (store *Store) ListModelSettings(
	ctx context.Context, workspace identity.WorkspaceID,
) ([]governance.ModelSetting, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, fmt.Errorf("encode workspace ID: %w", err)
	}
	rows, err := store.queries.ListModelSettings(ctx, workspaceID)
	if err != nil {
		return nil, governanceRepositoryError("list model settings", err)
	}
	settings := make([]governance.ModelSetting, 0, len(rows))
	for _, row := range rows {
		setting, decodeErr := modelSettingFromRow(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		settings = append(settings, setting)
	}
	return settings, nil
}

func (store *Store) UpdateModelSetting(
	ctx context.Context, command governanceapp.UpdateModelSettingCommand,
) (governance.ModelSetting, error) {
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return governance.ModelSetting{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	settingID, err := uuidValue(command.SettingID)
	if err != nil {
		return governance.ModelSetting{}, fmt.Errorf("encode setting ID: %w", err)
	}
	row, err := store.queries.UpdateModelSetting(ctx, dbgen.UpdateModelSettingParams{
		WorkspaceID: workspaceID, SettingID: settingID,
		Model: command.Model, Enabled: command.Enabled, Capability: command.Capability,
		TokenLimit: int32(command.TokenLimit), EmbeddingDimension: optionalInt4(command.EmbeddingDimension),
		UpdatedAt: timestamp(command.UpdatedAt),
	})
	if err != nil {
		return governance.ModelSetting{}, governanceRepositoryError("update model setting", err)
	}
	return modelSettingFromRow(row)
}

// SetDefaultModelSetting moves the (workspace, kind) default inside one
// transaction: the previous default row is cleared and the target becomes the
// only default, so the partial unique index invariant can never be violated.
func (store *Store) SetDefaultModelSetting(
	ctx context.Context, workspace identity.WorkspaceID, setting identity.ModelSettingID, when time.Time,
) (governance.ModelSetting, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.ModelSetting{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	settingID, err := uuidValue(setting)
	if err != nil {
		return governance.ModelSetting{}, fmt.Errorf("encode setting ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governance.ModelSetting{}, governanceRepositoryError("begin model default switch", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	current, err := queries.GetModelSetting(ctx, dbgen.GetModelSettingParams{
		WorkspaceID: workspaceID, SettingID: settingID,
	})
	if err != nil {
		return governance.ModelSetting{}, governanceRepositoryError("get model setting", err)
	}
	if err := queries.ClearModelSettingDefaults(ctx, dbgen.ClearModelSettingDefaultsParams{
		WorkspaceID: workspaceID, Kind: current.Kind, SettingID: settingID, UpdatedAt: timestamp(when),
	}); err != nil {
		return governance.ModelSetting{}, governanceRepositoryError("clear model defaults", err)
	}
	row, err := queries.SetModelSettingDefault(ctx, dbgen.SetModelSettingDefaultParams{
		WorkspaceID: workspaceID, SettingID: settingID, UpdatedAt: timestamp(when),
	})
	if err != nil {
		return governance.ModelSetting{}, governanceRepositoryError("set model default", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return governance.ModelSetting{}, governanceRepositoryError("commit model default switch", err)
	}
	return modelSettingFromRow(row)
}

func (store *Store) GetDefaultModelSetting(
	ctx context.Context, workspace identity.WorkspaceID, kind governance.ModelKind,
) (governance.ModelSetting, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governance.ModelSetting{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	row, err := store.queries.GetDefaultModelSetting(ctx, dbgen.GetDefaultModelSettingParams{
		WorkspaceID: workspaceID, Kind: string(kind),
	})
	if err != nil {
		return governance.ModelSetting{}, governanceRepositoryError("get default model setting", err)
	}
	return modelSettingFromRow(row)
}

// ---------- generation substrate ----------

// GenerationAssetSnapshot loads the deterministic generation input for one
// semantic asset: address coordinates plus the current revision identity and
// content. An asset without a current revision cannot be proposed against.
func (store *Store) GenerationAssetSnapshot(
	ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID,
) (governanceapp.GenerationTargetSnapshot, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return governanceapp.GenerationTargetSnapshot{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	assetID, err := uuidValue(asset)
	if err != nil {
		return governanceapp.GenerationTargetSnapshot{}, fmt.Errorf("encode asset ID: %w", err)
	}
	row, err := store.queries.GetCatalogAsset(ctx, dbgen.GetCatalogAssetParams{
		WorkspaceID: workspaceID, AssetID: assetID,
	})
	if err != nil {
		return governanceapp.GenerationTargetSnapshot{}, governanceRepositoryError("load generation asset", err)
	}
	if !row.CurrentRevisionID.Valid {
		return governanceapp.GenerationTargetSnapshot{}, fmt.Errorf(
			"generation asset %s: %w", asset.String(), governance.ErrInvariant)
	}
	revisionID, err := identity.RevisionIDFromUUIDBytes(row.RevisionID.Bytes)
	if err != nil {
		return governanceapp.GenerationTargetSnapshot{}, err
	}
	return governanceapp.GenerationTargetSnapshot{
		TargetObjectType: governance.TargetSemanticAsset,
		TargetObjectID:   asset.String(),
		BaseRevisionID:   revisionID.String(),
		Address:          row.Namespace + "." + row.Key,
		AssetType:        row.AssetType,
		Lifecycle:        row.LifecycleState,
		SchemaVersion:    row.SchemaVersion.String,
		Content:          append([]byte(nil), row.Content...),
	}, nil
}

// GenerationObjectSnapshot loads the deterministic generation input for one
// governed object (physical binding, model grain, entity key or join
// contract) through the existing T008 read path.
func (store *Store) GenerationObjectSnapshot(
	ctx context.Context, workspace identity.WorkspaceID,
	objectType governance.TargetObjectType, objectID string,
) (governanceapp.GenerationTargetSnapshot, error) {
	object, err := store.GetGovernedObject(ctx, workspace, objectType, objectID)
	if err != nil {
		return governanceapp.GenerationTargetSnapshot{}, err
	}
	snapshot := governanceapp.GenerationTargetSnapshot{
		TargetObjectType: objectType,
	}
	var content json.RawMessage
	switch {
	case object.PhysicalBinding != nil:
		snapshot.TargetObjectID = object.PhysicalBinding.ID.String()
		snapshot.ObjectVersion = object.PhysicalBinding.Version
		content = object.PhysicalBinding.Content
	case object.ModelGrain != nil:
		snapshot.TargetObjectID = object.ModelGrain.ID.String()
		snapshot.ObjectVersion = object.ModelGrain.Version
		content = object.ModelGrain.Content
	case object.EntityKey != nil:
		snapshot.TargetObjectID = object.EntityKey.ID.String()
		snapshot.ObjectVersion = object.EntityKey.Version
		content = object.EntityKey.Content
	case object.JoinContract != nil:
		snapshot.TargetObjectID = object.JoinContract.ID.String()
		snapshot.ObjectVersion = object.JoinContract.Version
		content = object.JoinContract.Content
	default:
		return governanceapp.GenerationTargetSnapshot{}, governance.ErrInvariant
	}
	snapshot.Content = append([]byte(nil), content...)
	return snapshot, nil
}

// WorkspaceAgentPrincipal loads the workspace's seeded agent principal (D6):
// the deterministic 'agent'-kind principal every live generation run and its
// proposals attribute to.
func (store *Store) WorkspaceAgentPrincipal(
	ctx context.Context, workspace identity.WorkspaceID,
) (authz.Principal, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return authz.Principal{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	row, err := store.queries.GetWorkspaceAgentPrincipal(ctx, workspaceID)
	if err != nil {
		return authz.Principal{}, governanceRepositoryError("load workspace agent principal", err)
	}
	return principalFromRow(row)
}

// ---------- row mappers ----------

func modelProviderFromRow(row dbgen.ModelProvider) (governance.ModelProvider, error) {
	id, err := identity.ModelProviderIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.ModelProvider{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.ModelProvider{}, err
	}
	var baseURL *string
	if row.BaseUrl.Valid {
		value := row.BaseUrl.String
		baseURL = &value
	}
	return governance.ModelProvider{
		ID: id, WorkspaceID: workspaceID, Protocol: governance.ModelProviderProtocol(row.Protocol),
		DisplayName: row.DisplayName, BaseURL: baseURL,
		CredentialEnv: row.CredentialEnv, CredentialRevision: row.CredentialRevision,
		Enabled: row.Enabled, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func modelSettingFromRow(row dbgen.ModelSetting) (governance.ModelSetting, error) {
	id, err := identity.ModelSettingIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return governance.ModelSetting{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return governance.ModelSetting{}, err
	}
	providerID, err := identity.ModelProviderIDFromUUIDBytes(row.ProviderID.Bytes)
	if err != nil {
		return governance.ModelSetting{}, err
	}
	var dimension *int
	if row.EmbeddingDimension.Valid {
		value := int(row.EmbeddingDimension.Int32)
		dimension = &value
	}
	return governance.ModelSetting{
		ID: id, WorkspaceID: workspaceID, ProviderID: providerID,
		Kind: governance.ModelKind(row.Kind), Model: row.Model, Enabled: row.Enabled,
		IsDefault: row.IsDefault, Capability: row.Capability, TokenLimit: int(row.TokenLimit),
		EmbeddingDimension: dimension, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func optionalInt4(value *int) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*value), Valid: true}
}
