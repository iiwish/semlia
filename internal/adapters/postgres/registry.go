package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var errRepositoryOperation = errors.New("PostgreSQL repository operation failed")

var _ semantic.RegistryRepository = (*Store)(nil)

func (store *Store) CreateSourceConnection(ctx context.Context, input semantic.SourceConnection) (semantic.SourceConnection, error) {
	id, workspaceID, err := registryIDs(input.ID, input.WorkspaceID)
	if err != nil {
		return semantic.SourceConnection{}, err
	}
	row, err := store.queries.CreateSourceConnection(ctx, dbgen.CreateSourceConnectionParams{
		ID: id, WorkspaceID: workspaceID, AdapterKind: input.AdapterKind, Name: input.Name,
		NormalizedLocator: input.NormalizedLocator, CredentialRef: optionalTextValue(input.CredentialRef),
		Status: input.Status, Metadata: objectJSON(input.Metadata),
	})
	if err != nil {
		return semantic.SourceConnection{}, repositoryError("create source connection", err)
	}
	return sourceConnectionFromRow(row)
}

func (store *Store) CreateSourceRevision(ctx context.Context, input semantic.SourceRevision) (semantic.SourceRevision, error) {
	id, workspaceID, err := registryIDs(input.ID, input.WorkspaceID)
	if err != nil {
		return semantic.SourceRevision{}, err
	}
	sourceID, err := uuidValue(input.SourceConnectionID)
	if err != nil {
		return semantic.SourceRevision{}, fmt.Errorf("encode source connection ID: %w", err)
	}
	row, err := store.queries.CreateSourceRevision(ctx, dbgen.CreateSourceRevisionParams{
		ID: id, WorkspaceID: workspaceID, SourceConnectionID: sourceID,
		ExternalRevision: optionalTextValue(input.ExternalRevision), ContentDigest: input.ContentDigest,
		AdapterVersion: input.AdapterVersion, ObservedAt: timestamp(input.ObservedAt), Metadata: objectJSON(input.Metadata),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, lookupErr := store.queries.GetSourceRevisionByDigest(ctx, dbgen.GetSourceRevisionByDigestParams{
			WorkspaceID: workspaceID, SourceConnectionID: sourceID, ContentDigest: input.ContentDigest,
		})
		if lookupErr != nil {
			return semantic.SourceRevision{}, repositoryError("resolve idempotent source revision", lookupErr)
		}
		return sourceRevisionFromRow(existing)
	}
	if err != nil {
		return semantic.SourceRevision{}, repositoryError("create source revision", err)
	}
	return sourceRevisionFromRow(dbgen.SourceRevision{
		ID: row.ID, WorkspaceID: row.WorkspaceID, SourceConnectionID: row.SourceConnectionID,
		ExternalRevision: row.ExternalRevision, ContentDigest: row.ContentDigest,
		AdapterVersion: row.AdapterVersion, ObservedAt: row.ObservedAt, Metadata: row.Metadata, CreatedAt: row.CreatedAt,
	})
}

func (store *Store) CreateAsset(ctx context.Context, input semantic.Asset) (semantic.Asset, error) {
	id, workspaceID, err := registryIDs(input.ID, input.WorkspaceID)
	if err != nil {
		return semantic.Asset{}, err
	}
	row, err := store.queries.CreateSemanticAsset(ctx, dbgen.CreateSemanticAssetParams{
		ID: id, WorkspaceID: workspaceID, Namespace: input.Address.Namespace(), Key: input.Address.Key(),
		AssetType: string(input.Type), LifecycleState: input.LifecycleState,
	})
	if err != nil {
		return semantic.Asset{}, repositoryError("create semantic asset", err)
	}
	return assetFromRow(row)
}

func (store *Store) CreateAssetRevision(ctx context.Context, input semantic.RevisionRecord) (semantic.RevisionRecord, error) {
	id, workspaceID, err := registryIDs(input.ID, input.WorkspaceID)
	if err != nil {
		return semantic.RevisionRecord{}, err
	}
	assetID, err := uuidValue(input.AssetID)
	if err != nil {
		return semantic.RevisionRecord{}, fmt.Errorf("encode asset ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return semantic.RevisionRecord{}, repositoryError("begin asset revision transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	row, err := queries.CreateAssetRevision(ctx, dbgen.CreateAssetRevisionParams{
		ID: id, WorkspaceID: workspaceID, AssetID: assetID, Sequence: input.Sequence,
		SchemaVersion: input.SchemaVersion, ContentDigest: input.ContentDigest,
		Content: objectJSON(input.Content), CreatedBy: input.CreatedBy,
	})
	if err != nil {
		return semantic.RevisionRecord{}, repositoryError("create asset revision", err)
	}
	rows, err := queries.SetCurrentAssetRevision(ctx, dbgen.SetCurrentAssetRevisionParams{
		RevisionID: id, WorkspaceID: workspaceID, AssetID: assetID,
	})
	if err != nil {
		return semantic.RevisionRecord{}, repositoryError("set current asset revision", err)
	}
	if rows != 1 {
		return semantic.RevisionRecord{}, semantic.ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return semantic.RevisionRecord{}, repositoryError("commit asset revision", err)
	}
	return revisionFromRow(row)
}

func (store *Store) CreateEvidence(ctx context.Context, input semantic.EvidenceArtifact) (semantic.EvidenceArtifact, error) {
	id, workspaceID, err := registryIDs(input.ID, input.WorkspaceID)
	if err != nil {
		return semantic.EvidenceArtifact{}, err
	}
	sourceRevisionID, err := nullableUUIDValue(input.SourceRevisionID)
	if err != nil {
		return semantic.EvidenceArtifact{}, fmt.Errorf("encode source revision ID: %w", err)
	}
	row, err := store.queries.CreateEvidenceArtifact(ctx, dbgen.CreateEvidenceArtifactParams{
		ID: id, WorkspaceID: workspaceID, EvidenceType: input.EvidenceType,
		SourceRevisionID: sourceRevisionID, Locator: input.Locator,
		ContentDigest: input.ContentDigest, Metadata: objectJSON(input.Metadata),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, lookupErr := store.queries.GetEvidenceArtifactByIdentity(ctx, dbgen.GetEvidenceArtifactByIdentityParams{
			WorkspaceID: workspaceID, EvidenceType: input.EvidenceType,
			Locator: input.Locator, ContentDigest: input.ContentDigest,
		})
		if lookupErr != nil {
			return semantic.EvidenceArtifact{}, repositoryError("resolve idempotent evidence artifact", lookupErr)
		}
		return evidenceFromRow(existing)
	}
	if err != nil {
		return semantic.EvidenceArtifact{}, repositoryError("create evidence artifact", err)
	}
	return evidenceFromRow(dbgen.EvidenceArtifact{
		ID: row.ID, WorkspaceID: row.WorkspaceID, EvidenceType: row.EvidenceType,
		SourceRevisionID: row.SourceRevisionID, Locator: row.Locator,
		ContentDigest: row.ContentDigest, Metadata: row.Metadata, CreatedAt: row.CreatedAt,
	})
}

func (store *Store) CreateRelation(ctx context.Context, input semantic.RelationRecord) (semantic.RelationRecord, error) {
	id, workspaceID, err := registryIDs(input.ID, input.WorkspaceID)
	if err != nil {
		return semantic.RelationRecord{}, err
	}
	subjectID, err := uuidValue(input.SubjectAssetID)
	if err != nil {
		return semantic.RelationRecord{}, fmt.Errorf("encode subject asset ID: %w", err)
	}
	objectID, err := uuidValue(input.ObjectAssetID)
	if err != nil {
		return semantic.RelationRecord{}, fmt.Errorf("encode object asset ID: %w", err)
	}
	sourceRevisionID, err := nullableUUIDValue(input.SourceRevisionID)
	if err != nil {
		return semantic.RelationRecord{}, fmt.Errorf("encode source revision ID: %w", err)
	}
	evidenceID, err := nullableUUIDValue(input.EvidenceArtifactID)
	if err != nil {
		return semantic.RelationRecord{}, fmt.Errorf("encode evidence ID: %w", err)
	}
	row, err := store.queries.CreateSemanticRelation(ctx, dbgen.CreateSemanticRelationParams{
		ID: id, WorkspaceID: workspaceID, SubjectAssetID: subjectID, Predicate: string(input.Predicate),
		ObjectAssetID: objectID, Plane: string(input.Plane), AssertionState: string(input.AssertionState),
		SourceRevisionID: sourceRevisionID, EvidenceArtifactID: evidenceID,
		InferenceRule: optionalTextValue(input.InferenceRule), CreatedBy: input.CreatedBy,
	})
	if err != nil {
		return semantic.RelationRecord{}, repositoryError("create semantic relation", err)
	}
	return relationFromRow(row)
}

func (store *Store) CreateOntologyRevision(ctx context.Context, input semantic.OntologyRevision) (semantic.OntologyRevision, error) {
	id, workspaceID, err := registryIDs(input.ID, input.WorkspaceID)
	if err != nil {
		return semantic.OntologyRevision{}, err
	}
	row, err := store.queries.CreateOntologyRevision(ctx, dbgen.CreateOntologyRevisionParams{
		ID: id, WorkspaceID: workspaceID, Sequence: input.Sequence, Status: input.Status,
		ContentDigest: input.ContentDigest, CreatedBy: input.CreatedBy, PublishedAt: optionalTimestamp(input.PublishedAt),
	})
	if err != nil {
		return semantic.OntologyRevision{}, repositoryError("create ontology revision", err)
	}
	return ontologyFromRow(row)
}

func (store *Store) AddOntologyRelation(
	ctx context.Context,
	workspace identity.WorkspaceID,
	ontology identity.OntologyID,
	relation identity.RelationID,
) error {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return err
	}
	ontologyID, err := uuidValue(ontology)
	if err != nil {
		return err
	}
	relationID, err := uuidValue(relation)
	if err != nil {
		return err
	}
	if err := store.queries.AddOntologyRelation(ctx, dbgen.AddOntologyRelationParams{
		WorkspaceID: workspaceID, OntologyRevisionID: ontologyID, SemanticRelationID: relationID,
	}); err != nil {
		return repositoryError("add ontology relation", err)
	}
	return nil
}

func (store *Store) PublishOntologyRevision(
	ctx context.Context,
	workspace identity.WorkspaceID,
	ontology identity.OntologyID,
	publishedAt time.Time,
) error {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return err
	}
	ontologyID, err := uuidValue(ontology)
	if err != nil {
		return err
	}
	rows, err := store.queries.PublishOntologyRevision(ctx, dbgen.PublishOntologyRevisionParams{
		PublishedAt: timestamp(publishedAt), WorkspaceID: workspaceID, OntologyRevisionID: ontologyID,
	})
	if err != nil {
		return repositoryError("publish ontology revision", err)
	}
	if rows != 1 {
		return semantic.ErrNotFound
	}
	return nil
}

func registryIDs(resource, workspace uuidIdentity) (pgtype.UUID, pgtype.UUID, error) {
	resourceID, err := uuidValue(resource)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("encode resource ID: %w", err)
	}
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	return resourceID, workspaceID, nil
}

func nullableUUIDValue[T uuidIdentity](value *T) (pgtype.UUID, error) {
	if value == nil {
		return pgtype.UUID{}, nil
	}
	return uuidValue(*value)
}

func optionalTextValue(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return textValue(value)
}

func optionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*value)
}

func objectJSON(value json.RawMessage) []byte {
	if len(value) == 0 {
		return []byte(`{}`)
	}
	return append([]byte(nil), value...)
}

func repositoryError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, semantic.ErrConflict)
		case "23503", "23514", "23502", "55000":
			return fmt.Errorf("%s: %w", operation, semantic.ErrInvariant)
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, semantic.ErrNotFound)
	}
	return fmt.Errorf("%s: %w", operation, errRepositoryOperation)
}

func sourceConnectionFromRow(row dbgen.SourceConnection) (semantic.SourceConnection, error) {
	id, err := identity.SourceConnectionIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return semantic.SourceConnection{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return semantic.SourceConnection{}, err
	}
	return semantic.SourceConnection{
		ID: id, WorkspaceID: workspaceID, AdapterKind: row.AdapterKind, Name: row.Name,
		NormalizedLocator: row.NormalizedLocator, CredentialRef: optionalText(row.CredentialRef),
		Status: row.Status, Metadata: cloneJSON(row.Metadata), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func sourceRevisionFromRow(row dbgen.SourceRevision) (semantic.SourceRevision, error) {
	id, err := identity.SourceRevisionIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return semantic.SourceRevision{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return semantic.SourceRevision{}, err
	}
	sourceID, err := identity.SourceConnectionIDFromUUIDBytes(row.SourceConnectionID.Bytes)
	if err != nil {
		return semantic.SourceRevision{}, err
	}
	return semantic.SourceRevision{
		ID: id, WorkspaceID: workspaceID, SourceConnectionID: sourceID,
		ExternalRevision: optionalText(row.ExternalRevision), ContentDigest: row.ContentDigest,
		AdapterVersion: row.AdapterVersion, ObservedAt: row.ObservedAt.Time,
		Metadata: cloneJSON(row.Metadata), CreatedAt: row.CreatedAt.Time,
	}, nil
}

func assetFromRow(row dbgen.SemanticAsset) (semantic.Asset, error) {
	id, err := identity.AssetIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return semantic.Asset{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return semantic.Asset{}, err
	}
	address, err := semantic.NewAddress(row.Namespace, row.Key)
	if err != nil {
		return semantic.Asset{}, err
	}
	var currentRevisionID *identity.RevisionID
	if row.CurrentRevisionID.Valid {
		value, decodeErr := identity.RevisionIDFromUUIDBytes(row.CurrentRevisionID.Bytes)
		if decodeErr != nil {
			return semantic.Asset{}, decodeErr
		}
		currentRevisionID = &value
	}
	return semantic.Asset{
		ID: id, WorkspaceID: workspaceID, Address: address, Type: semantic.AssetType(row.AssetType),
		LifecycleState: row.LifecycleState, CurrentRevisionID: currentRevisionID,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func revisionFromRow(row dbgen.AssetRevision) (semantic.RevisionRecord, error) {
	id, err := identity.RevisionIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return semantic.RevisionRecord{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return semantic.RevisionRecord{}, err
	}
	assetID, err := identity.AssetIDFromUUIDBytes(row.AssetID.Bytes)
	if err != nil {
		return semantic.RevisionRecord{}, err
	}
	return semantic.RevisionRecord{
		ID: id, WorkspaceID: workspaceID, AssetID: assetID, Sequence: row.Sequence,
		SchemaVersion: row.SchemaVersion, ContentDigest: row.ContentDigest,
		Content: cloneJSON(row.Content), CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time,
	}, nil
}

func evidenceFromRow(row dbgen.EvidenceArtifact) (semantic.EvidenceArtifact, error) {
	id, err := identity.EvidenceIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return semantic.EvidenceArtifact{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return semantic.EvidenceArtifact{}, err
	}
	var sourceRevisionID *identity.SourceRevisionID
	if row.SourceRevisionID.Valid {
		value, decodeErr := identity.SourceRevisionIDFromUUIDBytes(row.SourceRevisionID.Bytes)
		if decodeErr != nil {
			return semantic.EvidenceArtifact{}, decodeErr
		}
		sourceRevisionID = &value
	}
	return semantic.EvidenceArtifact{
		ID: id, WorkspaceID: workspaceID, EvidenceType: row.EvidenceType,
		SourceRevisionID: sourceRevisionID, Locator: row.Locator, ContentDigest: row.ContentDigest,
		Metadata: cloneJSON(row.Metadata), CreatedAt: row.CreatedAt.Time,
	}, nil
}

func relationFromRow(row dbgen.SemanticRelation) (semantic.RelationRecord, error) {
	id, err := identity.RelationIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return semantic.RelationRecord{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return semantic.RelationRecord{}, err
	}
	subjectID, err := identity.AssetIDFromUUIDBytes(row.SubjectAssetID.Bytes)
	if err != nil {
		return semantic.RelationRecord{}, err
	}
	objectID, err := identity.AssetIDFromUUIDBytes(row.ObjectAssetID.Bytes)
	if err != nil {
		return semantic.RelationRecord{}, err
	}
	sourceRevisionID, err := decodeOptionalSourceRevision(row.SourceRevisionID)
	if err != nil {
		return semantic.RelationRecord{}, err
	}
	evidenceID, err := decodeOptionalEvidence(row.EvidenceArtifactID)
	if err != nil {
		return semantic.RelationRecord{}, err
	}
	return semantic.RelationRecord{
		ID: id, WorkspaceID: workspaceID, SubjectAssetID: subjectID,
		Predicate: semantic.RelationPredicate(row.Predicate), ObjectAssetID: objectID,
		Plane: semantic.RelationPlane(row.Plane), AssertionState: semantic.AssertionState(row.AssertionState),
		SourceRevisionID: sourceRevisionID, EvidenceArtifactID: evidenceID,
		InferenceRule: optionalText(row.InferenceRule), CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time,
	}, nil
}

func ontologyFromRow(row dbgen.OntologyRevision) (semantic.OntologyRevision, error) {
	id, err := identity.OntologyIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return semantic.OntologyRevision{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return semantic.OntologyRevision{}, err
	}
	return semantic.OntologyRevision{
		ID: id, WorkspaceID: workspaceID, Sequence: row.Sequence, Status: row.Status,
		ContentDigest: row.ContentDigest, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time,
		PublishedAt: optionalTime(row.PublishedAt),
	}, nil
}

func decodeOptionalSourceRevision(value pgtype.UUID) (*identity.SourceRevisionID, error) {
	if !value.Valid {
		return nil, nil
	}
	id, err := identity.SourceRevisionIDFromUUIDBytes(value.Bytes)
	return &id, err
}

func decodeOptionalEvidence(value pgtype.UUID) (*identity.EvidenceID, error) {
	if !value.Valid {
		return nil, nil
	}
	id, err := identity.EvidenceIDFromUUIDBytes(value.Bytes)
	return &id, err
}
