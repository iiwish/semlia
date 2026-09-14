package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	application "github.com/iiwish/semlia/internal/application/catalog"
	domain "github.com/iiwish/semlia/internal/domain/catalog"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ application.Repository = (*Store)(nil)

func (store *Store) ListCatalogAssets(ctx context.Context, query domain.ListAssetsQuery) ([]domain.AssetSummary, error) {
	workspaceID, err := uuidValue(query.WorkspaceID)
	if err != nil {
		return nil, err
	}
	parameters := dbgen.ListCatalogAssetsParams{
		WorkspaceID: workspaceID, Search: query.Search, AssetType: string(query.AssetType),
		LifecycleState: query.Lifecycle, PageLimit: int32(query.Limit),
	}
	if query.Cursor != nil {
		parameters.HasCursor = true
		parameters.CursorRank = query.Cursor.Rank
		parameters.CursorUpdatedAt = timestamp(query.Cursor.UpdatedAt)
		parameters.CursorID, err = uuidValue(query.Cursor.ID)
		if err != nil {
			return nil, err
		}
	}
	rows, err := store.queries.ListCatalogAssets(ctx, parameters)
	if err != nil {
		return nil, repositoryError("list catalog assets", err)
	}
	result := make([]domain.AssetSummary, 0, len(rows))
	for _, row := range rows {
		value, decodeErr := catalogSummary(
			row.ID, row.Namespace, row.Key, row.AssetType, row.LifecycleState,
			row.CurrentRevisionID, row.Title, row.Summary, row.UpdatedAt, row.SearchRank,
		)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, value)
	}
	return result, nil
}

func (store *Store) CountCatalogAssets(ctx context.Context, query domain.ListAssetsQuery) (int64, error) {
	workspaceID, err := uuidValue(query.WorkspaceID)
	if err != nil {
		return 0, err
	}
	count, err := store.queries.CountCatalogAssets(ctx, dbgen.CountCatalogAssetsParams{
		WorkspaceID: workspaceID, Search: query.Search, AssetType: string(query.AssetType),
		LifecycleState: query.Lifecycle,
	})
	if err != nil {
		return 0, repositoryError("count catalog assets", err)
	}
	return count, nil
}

func (store *Store) GetCatalogAsset(ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID) (domain.AssetDetail, error) {
	workspaceID, assetID, err := catalogIDs(workspace, asset)
	if err != nil {
		return domain.AssetDetail{}, err
	}
	row, err := store.queries.GetCatalogAsset(ctx, dbgen.GetCatalogAssetParams{WorkspaceID: workspaceID, AssetID: assetID})
	if err != nil {
		return domain.AssetDetail{}, repositoryError("get catalog asset", err)
	}
	summary, err := catalogSummary(
		row.ID, row.Namespace, row.Key, row.AssetType, row.LifecycleState,
		row.CurrentRevisionID, row.Title, row.Summary, row.UpdatedAt, 0,
	)
	if err != nil {
		return domain.AssetDetail{}, err
	}
	detail := domain.AssetDetail{AssetSummary: summary, CreatedAt: row.CreatedAt.Time, RelationCount: int(row.RelationCount)}
	if row.RevisionID.Valid {
		revision := dbgen.AssetRevision{
			ID: row.RevisionID, WorkspaceID: row.WorkspaceID, AssetID: row.ID,
			Sequence: row.RevisionSequence.Int64, SchemaVersion: row.SchemaVersion.String,
			ContentDigest: row.ContentDigest.String, Content: row.Content,
			CreatedBy: row.CreatedBy.String, CreatedAt: row.RevisionCreatedAt,
		}
		mapped, mapErr := store.catalogRevision(ctx, workspaceID, revision)
		if mapErr != nil {
			return domain.AssetDetail{}, mapErr
		}
		detail.CurrentRevision = &mapped
	}
	facts, err := store.queries.GetCatalogAssetAuthorityFacts(ctx, dbgen.GetCatalogAssetAuthorityFactsParams{
		WorkspaceID: workspaceID, AssetID: assetID,
	})
	if err != nil {
		return domain.AssetDetail{}, repositoryError("get catalog asset authority facts", err)
	}
	detail.RelationCount = int(facts.RelationCount)
	detail.AuthoritySections, err = catalogAuthoritySections(facts)
	if err != nil {
		return domain.AssetDetail{}, err
	}
	lineageAvailability := domain.AvailabilityNotConfigured
	if facts.LineageCount > 0 {
		lineageAvailability = domain.AvailabilityAvailable
	}
	detail.AuthoritySections = append(detail.AuthoritySections, domain.AuthoritySection{
		Kind: "lineage", Authority: "lineage_edges", Availability: lineageAvailability,
		Values: map[string]int64{"lineageCount": facts.LineageCount}, Records: []domain.AuthorityRecord{},
	})
	return detail, nil
}

func (store *Store) ListCatalogAuthorityRecords(
	ctx context.Context, query domain.ListAuthorityRecordsQuery,
) (domain.AuthorityRecordResult, error) {
	workspaceID, assetID, err := catalogIDs(query.WorkspaceID, query.AssetID)
	if err != nil {
		return domain.AuthorityRecordResult{}, err
	}
	params := dbgen.ListCatalogAssetAuthorityRecordsParams{
		WorkspaceID: workspaceID, AssetID: assetID, SectionKind: query.Section,
		PageLimit: int32(query.Limit),
	}
	if query.Cursor != nil {
		params.HasCursor = true
		params.CursorRecordKind = query.Cursor.Kind
		cursor, parseErr := identity.ParseAny(query.Cursor.ID)
		if parseErr != nil {
			return domain.AuthorityRecordResult{}, domain.ErrInvalidArgument
		}
		params.CursorRecordID, err = uuidValue(cursor)
		if err != nil {
			return domain.AuthorityRecordResult{}, domain.ErrInvalidArgument
		}
	}
	rows, err := store.queries.ListCatalogAssetAuthorityRecords(ctx, params)
	if err != nil {
		return domain.AuthorityRecordResult{}, repositoryError("list catalog asset authority records", err)
	}
	records := make([]domain.AuthorityRecord, 0, len(rows))
	for _, row := range rows {
		record, mapErr := catalogAuthorityRecord(row)
		if mapErr != nil {
			return domain.AuthorityRecordResult{}, mapErr
		}
		records = append(records, record)
	}
	result := domain.AuthorityRecordResult{Items: records}
	if len(rows) > 0 {
		result.Total = rows[0].TotalCount
	}
	return result, nil
}

func catalogAuthorityRecord(row dbgen.ListCatalogAssetAuthorityRecordsRow) (domain.AuthorityRecord, error) {
	prefix := identity.Prefix("")
	switch row.RecordKind {
	case "relation":
		prefix = identity.Relation
	case "physical_binding":
		prefix = identity.PhysicalBinding
	case "model_grain":
		prefix = identity.ModelGrain
	case "entity_key":
		prefix = identity.EntityKey
	case "join_contract":
		prefix = identity.JoinContract
	case "validation_run":
		prefix = identity.ValidationRun
	case "lineage":
		prefix = identity.LineageEdge
	case "consumer_binding":
		prefix = identity.ConsumerBinding
	default:
		return domain.AuthorityRecord{}, domain.ErrInvariant
	}
	id, err := identity.FromUUIDBytes(prefix, row.RecordID.Bytes)
	if err != nil {
		return domain.AuthorityRecord{}, err
	}
	releaseID, releaseSequence, err := catalogReleaseBasis(row.ReleaseID, row.ReleaseSequence)
	if err != nil {
		return domain.AuthorityRecord{}, err
	}
	relatedID, err := catalogAuthorityRelatedID(row.RecordKind, row.RelatedID)
	if err != nil {
		return domain.AuthorityRecord{}, err
	}
	record := domain.AuthorityRecord{Kind: row.RecordKind, ID: id.String(), Authority: row.Authority,
		Status: row.Status, Label: row.Label, RelatedID: relatedID, Version: int(row.Version),
		ReleaseID: releaseID, ReleaseSequence: releaseSequence}
	if err := catalogAuthorityDetails(&record, row.Details); err != nil {
		return domain.AuthorityRecord{}, err
	}
	return record, nil
}

func catalogAuthorityRelatedID(kind, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	prefix := identity.PhysicalDataset
	switch kind {
	case "relation":
		prefix = identity.Asset
	case "validation_run":
		prefix = identity.Proposal
	case "consumer_binding":
		prefix = identity.Consumer
	case "physical_binding", "join_contract", "lineage":
		prefix = identity.PhysicalDataset
	default:
		return "", nil
	}
	id, err := identity.FromUUID(prefix, value)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func catalogAuthorityDetails(record *domain.AuthorityRecord, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	switch record.Kind {
	case "relation":
		var value struct {
			Direction, Predicate, Plane, AssertionState, SubjectAssetID, ObjectAssetID string
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return domain.ErrInvariant
		}
		subject, err := assetIDFromStorage(value.SubjectAssetID)
		if err != nil {
			return err
		}
		object, err := assetIDFromStorage(value.ObjectAssetID)
		if err != nil {
			return err
		}
		record.Relation = &domain.RelationAuthority{Direction: value.Direction,
			Predicate: value.Predicate, Plane: value.Plane, AssertionState: value.AssertionState,
			SubjectAssetID: subject, ObjectAssetID: object}
	case "physical_binding":
		var value struct {
			AssetID, DatasetID, FieldID, Transform string
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return domain.ErrInvariant
		}
		assetID, err := assetIDFromStorage(value.AssetID)
		if err != nil {
			return err
		}
		datasetID, err := datasetIDFromStorage(value.DatasetID)
		if err != nil {
			return err
		}
		var fieldID *identity.PhysicalFieldID
		if value.FieldID != "" {
			field, fieldErr := fieldIDFromStorage(value.FieldID)
			if fieldErr != nil {
				return fieldErr
			}
			fieldID = &field
		}
		record.PhysicalBinding = &domain.PhysicalBindingAuthority{
			AssetID: assetID, DatasetID: datasetID, FieldID: fieldID, Transform: value.Transform,
		}
	case "model_grain":
		var value struct {
			AssetID, GrainExpression, DocumentedBy string
			GrainFieldRefs                         []string
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return domain.ErrInvariant
		}
		assetID, err := assetIDFromStorage(value.AssetID)
		if err != nil {
			return err
		}
		refs, err := fieldIDsFromStorage(value.GrainFieldRefs)
		if err != nil {
			return err
		}
		var documentedBy *identity.EvidenceID
		if value.DocumentedBy != "" {
			id, idErr := identity.FromUUID(identity.Evidence, value.DocumentedBy)
			if idErr != nil {
				return idErr
			}
			parsed, parseErr := identity.ParseEvidenceID(id.String())
			if parseErr != nil {
				return parseErr
			}
			documentedBy = &parsed
		}
		record.ModelGrain = &domain.ModelGrainAuthority{AssetID: assetID,
			GrainExpression: value.GrainExpression, GrainFieldRefs: refs, DocumentedBy: documentedBy}
	case "entity_key":
		var value struct {
			AssetID, UniquenessSemantics string
			KeyFieldRefs                 []string
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return domain.ErrInvariant
		}
		assetID, err := assetIDFromStorage(value.AssetID)
		if err != nil {
			return err
		}
		refs, err := fieldIDsFromStorage(value.KeyFieldRefs)
		if err != nil {
			return err
		}
		record.EntityKey = &domain.EntityKeyAuthority{AssetID: assetID,
			KeyFieldRefs: refs, UniquenessSemantics: value.UniquenessSemantics}
	case "join_contract":
		var value struct {
			Direction, LeftDatasetID, RightDatasetID, JoinType, Cardinality, JoinExpression string
			LeftFieldRefs, RightFieldRefs                                                   []string
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return domain.ErrInvariant
		}
		leftDataset, err := datasetIDFromStorage(value.LeftDatasetID)
		if err != nil {
			return err
		}
		rightDataset, err := datasetIDFromStorage(value.RightDatasetID)
		if err != nil {
			return err
		}
		leftRefs, err := fieldIDsFromStorage(value.LeftFieldRefs)
		if err != nil {
			return err
		}
		rightRefs, err := fieldIDsFromStorage(value.RightFieldRefs)
		if err != nil {
			return err
		}
		record.JoinContract = &domain.JoinContractAuthority{Direction: value.Direction,
			LeftDatasetID: leftDataset, RightDatasetID: rightDataset,
			LeftFieldRefs: leftRefs, RightFieldRefs: rightRefs, JoinType: value.JoinType,
			Cardinality: value.Cardinality, JoinExpression: value.JoinExpression}
	case "lineage":
		var value struct {
			Direction, UpstreamDatasetID, DownstreamDatasetID string
			EdgeKind, SourceRevisionID, CodeArtifactID        string
			Confidence                                        float64
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return domain.ErrInvariant
		}
		upstream, err := datasetIDFromStorage(value.UpstreamDatasetID)
		if err != nil {
			return err
		}
		downstream, err := datasetIDFromStorage(value.DownstreamDatasetID)
		if err != nil {
			return err
		}
		sourceRevision, err := sourceRevisionIDFromStorage(value.SourceRevisionID)
		if err != nil {
			return err
		}
		var codeArtifact *identity.CodeArtifactID
		if value.CodeArtifactID != "" {
			id, idErr := identity.FromUUID(identity.CodeArtifact, value.CodeArtifactID)
			if idErr != nil {
				return idErr
			}
			parsed, parseErr := identity.ParseCodeArtifactID(id.String())
			if parseErr != nil {
				return parseErr
			}
			codeArtifact = &parsed
		}
		record.Lineage = &domain.LineageAuthority{Direction: value.Direction,
			UpstreamDatasetID: upstream, DownstreamDatasetID: downstream, EdgeKind: value.EdgeKind,
			SourceRevisionID: sourceRevision, CodeArtifactID: codeArtifact, Confidence: value.Confidence}
	case "consumer_binding":
		var value struct {
			ConsumerID, EffectiveReleaseID, Environment, Purpose, Mode, Status string
			CompatibilityConstraint                                            json.RawMessage
			ExpiresAt                                                          *time.Time
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return domain.ErrInvariant
		}
		id, err := identity.FromUUID(identity.Consumer, value.ConsumerID)
		if err != nil {
			return err
		}
		consumerID, err := identity.ParseConsumerID(id.String())
		if err != nil {
			return err
		}
		effectiveRelease, err := identity.FromUUID(identity.Release, value.EffectiveReleaseID)
		if err != nil {
			return err
		}
		effectiveReleaseID, err := identity.ParseReleaseID(effectiveRelease.String())
		if err != nil {
			return err
		}
		if len(value.CompatibilityConstraint) == 0 {
			value.CompatibilityConstraint = json.RawMessage(`{}`)
		}
		record.ConsumerBinding = &domain.ConsumerBindingAuthority{ConsumerID: consumerID, EffectiveReleaseID: effectiveReleaseID,
			Environment: value.Environment, Purpose: value.Purpose, Mode: value.Mode, Status: value.Status,
			CompatibilityConstraint: value.CompatibilityConstraint, ExpiresAt: value.ExpiresAt}
	}
	return nil
}

func assetIDFromStorage(value string) (identity.AssetID, error) {
	id, err := identity.FromUUID(identity.Asset, value)
	if err != nil {
		return identity.AssetID{}, err
	}
	return identity.ParseAssetID(id.String())
}

func datasetIDFromStorage(value string) (identity.PhysicalDatasetID, error) {
	id, err := identity.FromUUID(identity.PhysicalDataset, value)
	if err != nil {
		return identity.PhysicalDatasetID{}, err
	}
	return identity.ParsePhysicalDatasetID(id.String())
}

func fieldIDFromStorage(value string) (identity.PhysicalFieldID, error) {
	if strings.HasPrefix(value, "pfd_") {
		return identity.ParsePhysicalFieldID(value)
	}
	id, err := identity.FromUUID(identity.PhysicalField, value)
	if err != nil {
		return identity.PhysicalFieldID{}, err
	}
	return identity.ParsePhysicalFieldID(id.String())
}

func sourceRevisionIDFromStorage(value string) (identity.SourceRevisionID, error) {
	id, err := identity.FromUUID(identity.SourceRevision, value)
	if err != nil {
		return identity.SourceRevisionID{}, err
	}
	return identity.ParseSourceRevisionID(id.String())
}

func fieldIDsFromStorage(values []string) ([]identity.PhysicalFieldID, error) {
	result := make([]identity.PhysicalFieldID, 0, len(values))
	for _, value := range values {
		id, err := fieldIDFromStorage(value)
		if err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}

func catalogAuthoritySections(facts dbgen.GetCatalogAssetAuthorityFactsRow) ([]domain.AuthoritySection, error) {
	var currentRevision *identity.RevisionID
	if facts.CurrentRevisionID.Valid {
		value, err := identity.RevisionIDFromUUIDBytes(facts.CurrentRevisionID.Bytes)
		if err != nil {
			return nil, err
		}
		currentRevision = &value
	}
	var releasedRevision *identity.RevisionID
	if facts.ReleasedRevisionID.Valid {
		value, err := identity.RevisionIDFromUUIDBytes(facts.ReleasedRevisionID.Bytes)
		if err != nil {
			return nil, err
		}
		releasedRevision = &value
	}
	var releaseID *identity.ReleaseID
	if facts.ReleaseID.Valid {
		value, err := identity.ReleaseIDFromUUIDBytes(facts.ReleaseID.Bytes)
		if err != nil {
			return nil, err
		}
		releaseID = &value
	}
	var releaseSequence *int64
	if facts.ReleaseSequence.Valid {
		value := facts.ReleaseSequence.Int64
		releaseSequence = &value
	}
	physicalReleaseID, physicalReleaseSequence, err := catalogReleaseBasis(facts.PhysicalReleaseID, facts.PhysicalReleaseSequence)
	if err != nil {
		return nil, err
	}
	joinReleaseID, joinReleaseSequence, err := catalogReleaseBasis(facts.JoinReleaseID, facts.JoinReleaseSequence)
	if err != nil {
		return nil, err
	}
	releasedAvailability := domain.AvailabilityNotReleased
	if releaseID != nil {
		releasedAvailability = domain.AvailabilityAvailable
	}
	physicalAvailability := domain.AvailabilityNotReleased
	if physicalReleaseID != nil {
		physicalAvailability = domain.AvailabilityAvailable
	} else if releaseID != nil {
		physicalAvailability = domain.AvailabilityNotConfigured
	}
	joinAvailability := domain.AvailabilityNotReleased
	if joinReleaseID != nil {
		joinAvailability = domain.AvailabilityAvailable
	} else if releaseID != nil {
		joinAvailability = domain.AvailabilityNotConfigured
	}
	validationAvailability := domain.AvailabilityNotReleased
	var validationRevisionID *identity.RevisionID
	var validationReleaseID *identity.ReleaseID
	var validationReleaseSequence *int64
	if releaseID != nil {
		validationAvailability = domain.AvailabilityNotConfigured
		if facts.ValidationRunCount > 0 {
			validationAvailability = domain.AvailabilityAvailable
			validationRevisionID = releasedRevision
			validationReleaseID = releaseID
			validationReleaseSequence = releaseSequence
		}
	}
	definitionAvailability := domain.AvailabilityFailed
	if currentRevision != nil {
		definitionAvailability = domain.AvailabilityAvailable
	}
	return []domain.AuthoritySection{
		{Kind: "definition", Authority: "asset_revisions", Availability: definitionAvailability, RevisionID: currentRevision, Values: map[string]int64{}, Records: []domain.AuthorityRecord{}},
		{Kind: "released_state", Authority: "release_assets", Availability: releasedAvailability, RevisionID: releasedRevision, ReleaseID: releaseID, ReleaseSequence: releaseSequence, Values: map[string]int64{}, Records: []domain.AuthorityRecord{}},
		{Kind: "relations", Authority: "semantic_relations", Availability: domain.AvailabilityAvailable, Values: map[string]int64{"relationCount": facts.RelationCount}, Records: []domain.AuthorityRecord{}},
		{Kind: "physical_bindings", Authority: "release_object_snapshots", Availability: physicalAvailability, ReleaseID: physicalReleaseID, ReleaseSequence: physicalReleaseSequence, Values: map[string]int64{"bindingCount": facts.PhysicalBindingCount, "grainCount": facts.ModelGrainCount, "entityKeyCount": facts.EntityKeyCount}, Records: []domain.AuthorityRecord{}},
		{Kind: "join_contracts", Authority: "release_object_snapshots", Availability: joinAvailability, ReleaseID: joinReleaseID, ReleaseSequence: joinReleaseSequence, Values: map[string]int64{"contractCount": facts.JoinContractCount}, Records: []domain.AuthorityRecord{}},
		{Kind: "validation", Authority: "validation_runs", Availability: validationAvailability, RevisionID: validationRevisionID, ReleaseID: validationReleaseID, ReleaseSequence: validationReleaseSequence, Values: map[string]int64{"runCount": facts.ValidationRunCount, "blockerCount": facts.ValidationBlockerCount, "warningCount": facts.ValidationWarningCount}, Records: []domain.AuthorityRecord{}},
		{Kind: "evidence", Authority: "revision_evidence_links", Availability: definitionAvailability, RevisionID: currentRevision, Values: map[string]int64{"evidenceCount": facts.EvidenceCount}, Records: []domain.AuthorityRecord{}},
		{Kind: "trust", Authority: "trust_configuration", Availability: domain.AvailabilityNotConfigured, Values: map[string]int64{}, Records: []domain.AuthorityRecord{}},
		{Kind: "consumer_impact", Authority: "consumer_bindings", Availability: releasedAvailability, RevisionID: releasedRevision, ReleaseID: releaseID, ReleaseSequence: releaseSequence, Values: map[string]int64{"currentConsumerCount": facts.CurrentConsumerCount, "pinnedConsumerCount": facts.PinnedConsumerCount}, Records: []domain.AuthorityRecord{}},
	}, nil
}

func catalogReleaseBasis(value pgtype.UUID, sequence int64) (*identity.ReleaseID, *int64, error) {
	if !value.Valid {
		return nil, nil, nil
	}
	releaseID, err := identity.ReleaseIDFromUUIDBytes(value.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return &releaseID, &sequence, nil
}

func (store *Store) CreateCatalogAsset(ctx context.Context, command domain.CreateAssetCommand) (domain.AssetDetail, error) {
	workspaceID, assetID, err := catalogIDs(command.WorkspaceID, command.AssetID)
	if err != nil {
		return domain.AssetDetail{}, err
	}
	revisionID, err := uuidValue(command.RevisionID)
	if err != nil {
		return domain.AssetDetail{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.AssetDetail{}, repositoryError("begin catalog asset transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	if _, err := queries.CreateSemanticAsset(ctx, dbgen.CreateSemanticAssetParams{
		ID: assetID, WorkspaceID: workspaceID, Namespace: command.Address.Namespace(), Key: command.Address.Key(),
		AssetType: string(command.AssetType), LifecycleState: command.Lifecycle,
	}); err != nil {
		return domain.AssetDetail{}, repositoryError("create catalog asset", err)
	}
	if _, err := queries.CreateAssetRevision(ctx, dbgen.CreateAssetRevisionParams{
		ID: revisionID, WorkspaceID: workspaceID, AssetID: assetID, Sequence: 1,
		SchemaVersion: command.SchemaVersion, ContentDigest: command.ContentDigest,
		Content: command.Content, CreatedBy: command.CreatedBy,
	}); err != nil {
		return domain.AssetDetail{}, repositoryError("create initial asset revision", err)
	}
	if err := linkEvidence(ctx, queries, workspaceID, revisionID, command.EvidenceIDs); err != nil {
		return domain.AssetDetail{}, err
	}
	rows, err := queries.SetCurrentAssetRevision(ctx, dbgen.SetCurrentAssetRevisionParams{
		RevisionID: revisionID, WorkspaceID: workspaceID, AssetID: assetID,
	})
	if err != nil {
		return domain.AssetDetail{}, repositoryError("select initial asset revision", err)
	}
	if rows != 1 {
		return domain.AssetDetail{}, semantic.ErrInvariant
	}
	if err := createCatalogMutationEvents(ctx, queries, catalogEvent{
		WorkspaceID: command.WorkspaceID, AssetID: command.AssetID, RevisionID: command.RevisionID,
		AuditID: command.AuditEventID, OutboxID: command.OutboxEventID, Sequence: 1,
		Action: "created", Actor: command.CreatedBy, TraceID: command.TraceID, CreatedAt: command.CreatedAt,
	}); err != nil {
		return domain.AssetDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AssetDetail{}, repositoryError("commit catalog asset", err)
	}
	return store.GetCatalogAsset(ctx, command.WorkspaceID, command.AssetID)
}

func (store *Store) ListCatalogRevisions(ctx context.Context, query domain.ListRevisionsQuery) ([]domain.Revision, error) {
	workspaceID, assetID, err := catalogIDs(query.WorkspaceID, query.AssetID)
	if err != nil {
		return nil, err
	}
	if _, err := store.queries.GetCatalogAsset(ctx, dbgen.GetCatalogAssetParams{WorkspaceID: workspaceID, AssetID: assetID}); err != nil {
		return nil, repositoryError("get catalog asset for revision list", err)
	}
	parameters := dbgen.ListCatalogAssetRevisionsParams{
		WorkspaceID: workspaceID, AssetID: assetID, PageLimit: int32(query.Limit),
	}
	if query.Cursor != nil {
		parameters.HasCursor = true
		parameters.CursorSequence = query.Cursor.Sequence
		parameters.CursorID, err = uuidValue(query.Cursor.ID)
		if err != nil {
			return nil, err
		}
	}
	rows, err := store.queries.ListCatalogAssetRevisions(ctx, parameters)
	if err != nil {
		return nil, repositoryError("list catalog asset revisions", err)
	}
	result := make([]domain.Revision, 0, len(rows))
	for _, row := range rows {
		value, mapErr := store.catalogRevision(ctx, workspaceID, row)
		if mapErr != nil {
			return nil, mapErr
		}
		result = append(result, value)
	}
	return result, nil
}

func (store *Store) CountCatalogRevisions(
	ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID,
) (int64, error) {
	workspaceID, assetID, err := catalogIDs(workspace, asset)
	if err != nil {
		return 0, err
	}
	count, err := store.queries.CountCatalogAssetRevisions(ctx, dbgen.CountCatalogAssetRevisionsParams{
		WorkspaceID: workspaceID, AssetID: assetID,
	})
	if err != nil {
		return 0, repositoryError("count catalog asset revisions", err)
	}
	return count, nil
}

func (store *Store) GetCatalogRevision(
	ctx context.Context,
	workspace identity.WorkspaceID,
	asset identity.AssetID,
	revision identity.RevisionID,
) (domain.Revision, error) {
	workspaceID, assetID, err := catalogIDs(workspace, asset)
	if err != nil {
		return domain.Revision{}, err
	}
	revisionID, err := uuidValue(revision)
	if err != nil {
		return domain.Revision{}, err
	}
	row, err := store.queries.GetCatalogAssetRevision(ctx, dbgen.GetCatalogAssetRevisionParams{
		WorkspaceID: workspaceID, AssetID: assetID, RevisionID: revisionID,
	})
	if err != nil {
		return domain.Revision{}, repositoryError("get catalog asset revision", err)
	}
	return store.catalogRevision(ctx, workspaceID, row)
}

func (store *Store) AppendCatalogRevision(ctx context.Context, command domain.AppendRevisionCommand) (domain.Revision, error) {
	workspaceID, assetID, err := catalogIDs(command.WorkspaceID, command.AssetID)
	if err != nil {
		return domain.Revision{}, err
	}
	revisionID, err := uuidValue(command.RevisionID)
	if err != nil {
		return domain.Revision{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Revision{}, repositoryError("begin asset revision transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	lockedAsset, err := queries.GetCatalogAssetForUpdate(ctx, dbgen.GetCatalogAssetForUpdateParams{
		WorkspaceID: workspaceID, AssetID: assetID,
	})
	if err != nil {
		return domain.Revision{}, repositoryError("lock catalog asset", err)
	}
	var baseRevisionID *identity.RevisionID
	if lockedAsset.CurrentRevisionID.Valid {
		value, decodeErr := identity.RevisionIDFromUUIDBytes(lockedAsset.CurrentRevisionID.Bytes)
		if decodeErr != nil {
			return domain.Revision{}, decodeErr
		}
		baseRevisionID = &value
	}
	sequence, err := queries.NextAssetRevisionSequence(ctx, dbgen.NextAssetRevisionSequenceParams{
		WorkspaceID: workspaceID, AssetID: assetID,
	})
	if err != nil {
		return domain.Revision{}, repositoryError("allocate asset revision sequence", err)
	}
	if _, err := queries.CreateAssetRevision(ctx, dbgen.CreateAssetRevisionParams{
		ID: revisionID, WorkspaceID: workspaceID, AssetID: assetID, Sequence: sequence,
		SchemaVersion: command.SchemaVersion, ContentDigest: command.ContentDigest,
		Content: command.Content, CreatedBy: command.CreatedBy,
	}); err != nil {
		return domain.Revision{}, repositoryError("append asset revision", err)
	}
	if err := linkEvidence(ctx, queries, workspaceID, revisionID, command.EvidenceIDs); err != nil {
		return domain.Revision{}, err
	}
	rows, err := queries.SetCurrentAssetRevision(ctx, dbgen.SetCurrentAssetRevisionParams{
		RevisionID: revisionID, WorkspaceID: workspaceID, AssetID: assetID,
	})
	if err != nil {
		return domain.Revision{}, repositoryError("select appended asset revision", err)
	}
	if rows != 1 {
		return domain.Revision{}, semantic.ErrInvariant
	}
	if err := createCatalogMutationEvents(ctx, queries, catalogEvent{
		WorkspaceID: command.WorkspaceID, AssetID: command.AssetID, RevisionID: command.RevisionID,
		BaseRevisionID: baseRevisionID,
		AuditID:        command.AuditEventID, OutboxID: command.OutboxEventID, Sequence: sequence,
		Action: "revision.created", Actor: command.CreatedBy, TraceID: command.TraceID, CreatedAt: command.CreatedAt,
	}); err != nil {
		return domain.Revision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Revision{}, repositoryError("commit appended asset revision", err)
	}
	return store.GetCatalogRevision(ctx, command.WorkspaceID, command.AssetID, command.RevisionID)
}

func (store *Store) ListCatalogRelations(ctx context.Context, query domain.ListRelationsQuery) ([]domain.Relation, error) {
	workspaceID, rootID, err := catalogIDs(query.WorkspaceID, query.AssetID)
	if err != nil {
		return nil, err
	}
	if _, err := store.queries.GetCatalogAsset(ctx, dbgen.GetCatalogAssetParams{WorkspaceID: workspaceID, AssetID: rootID}); err != nil {
		return nil, repositoryError("get catalog asset for relation list", err)
	}
	frontier := []identity.AssetID{query.AssetID}
	visitedNodes := map[string]struct{}{query.AssetID.String(): {}}
	seenRelations := make(map[string]struct{})
	result := make([]domain.Relation, 0)
	for depth := 1; depth <= query.Depth && len(frontier) > 0; depth++ {
		next := make([]identity.AssetID, 0)
		for _, node := range frontier {
			if len(result) >= query.Limit {
				break
			}
			nodeID, encodeErr := uuidValue(node)
			if encodeErr != nil {
				return nil, encodeErr
			}
			direction := "both"
			if depth == 1 {
				direction = query.Direction
			}
			rows, listErr := store.queries.ListDirectCatalogAssetRelations(ctx, dbgen.ListDirectCatalogAssetRelationsParams{
				AssetID: nodeID, WorkspaceID: workspaceID, Direction: direction, Plane: string(query.Plane),
				PageLimit: int32(query.Limit - len(result)),
			})
			if listErr != nil {
				return nil, repositoryError("list direct catalog relations", listErr)
			}
			for _, row := range rows {
				counterpartID, decodeErr := identity.AssetIDFromUUIDBytes(row.CounterpartID.Bytes)
				if decodeErr != nil {
					return nil, decodeErr
				}
				if _, exists := visitedNodes[counterpartID.String()]; !exists {
					visitedNodes[counterpartID.String()] = struct{}{}
					next = append(next, counterpartID)
				}
				relationID, decodeErr := identity.RelationIDFromUUIDBytes(row.ID.Bytes)
				if decodeErr != nil {
					return nil, decodeErr
				}
				if _, exists := seenRelations[relationID.String()]; exists {
					continue
				}
				seenRelations[relationID.String()] = struct{}{}
				mapped, mapErr := catalogRelation(row, relationID, counterpartID, depth)
				if mapErr != nil {
					return nil, mapErr
				}
				result = append(result, mapped)
				if len(result) >= query.Limit {
					break
				}
			}
		}
		frontier = next
	}
	return result, nil
}

func (store *Store) GetCatalogDiscoveryRun(ctx context.Context, workspace identity.WorkspaceID, run identity.RunID) (domain.DiscoveryRun, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return domain.DiscoveryRun{}, err
	}
	runID, err := uuidValue(run)
	if err != nil {
		return domain.DiscoveryRun{}, err
	}
	row, err := store.queries.GetCatalogDiscoveryRun(ctx, dbgen.GetCatalogDiscoveryRunParams{WorkspaceID: workspaceID, RunID: runID})
	if err != nil {
		return domain.DiscoveryRun{}, repositoryError("get discovery run", err)
	}
	findings, err := store.queries.GetCatalogDiscoveryFindings(ctx, runID)
	if err != nil {
		return domain.DiscoveryRun{}, repositoryError("get discovery findings", err)
	}
	return catalogDiscoveryRun(row, findings)
}

func (store *Store) catalogRevision(ctx context.Context, workspaceID pgtype.UUID, row dbgen.AssetRevision) (domain.Revision, error) {
	id, err := identity.RevisionIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.Revision{}, err
	}
	assetID, err := identity.AssetIDFromUUIDBytes(row.AssetID.Bytes)
	if err != nil {
		return domain.Revision{}, err
	}
	evidenceRows, err := store.queries.GetRevisionEvidence(ctx, dbgen.GetRevisionEvidenceParams{
		WorkspaceID: workspaceID, RevisionID: row.ID,
	})
	if err != nil {
		return domain.Revision{}, repositoryError("get revision evidence", err)
	}
	evidence := make([]domain.Evidence, 0, len(evidenceRows))
	for _, evidenceRow := range evidenceRows {
		mapped, mapErr := catalogEvidence(evidenceRow)
		if mapErr != nil {
			return domain.Revision{}, mapErr
		}
		evidence = append(evidence, mapped)
	}
	return domain.Revision{
		ID: id, AssetID: assetID, Sequence: row.Sequence, SchemaVersion: row.SchemaVersion,
		ContentDigest: row.ContentDigest, Content: cloneJSON(row.Content), CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt.Time, Evidence: evidence,
	}, nil
}

func catalogIDs(workspace identity.WorkspaceID, asset identity.AssetID) (pgtype.UUID, pgtype.UUID, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	assetID, err := uuidValue(asset)
	return workspaceID, assetID, err
}

func catalogSummary(
	id pgtype.UUID,
	namespace, key, assetType, lifecycle string,
	currentRevisionID pgtype.UUID,
	title, summary string,
	updatedAt pgtype.Timestamptz,
	rank float64,
) (domain.AssetSummary, error) {
	assetID, err := identity.AssetIDFromUUIDBytes(id.Bytes)
	if err != nil {
		return domain.AssetSummary{}, err
	}
	address, err := semantic.NewAddress(namespace, key)
	if err != nil {
		return domain.AssetSummary{}, err
	}
	var revisionID *identity.RevisionID
	if currentRevisionID.Valid {
		value, decodeErr := identity.RevisionIDFromUUIDBytes(currentRevisionID.Bytes)
		if decodeErr != nil {
			return domain.AssetSummary{}, decodeErr
		}
		revisionID = &value
	}
	return domain.AssetSummary{
		ID: assetID, Address: address, Type: semantic.AssetType(assetType), LifecycleState: lifecycle,
		CurrentRevisionID: revisionID, Title: title, Summary: summary, UpdatedAt: updatedAt.Time, Rank: rank,
	}, nil
}

func catalogEvidence(row dbgen.GetRevisionEvidenceRow) (domain.Evidence, error) {
	id, err := identity.EvidenceIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.Evidence{}, err
	}
	var sourceRevisionID *identity.SourceRevisionID
	if row.SourceRevisionID.Valid {
		value, decodeErr := identity.SourceRevisionIDFromUUIDBytes(row.SourceRevisionID.Bytes)
		if decodeErr != nil {
			return domain.Evidence{}, decodeErr
		}
		sourceRevisionID = &value
	}
	return domain.Evidence{
		ID: id, EvidenceType: row.EvidenceType, SourceRevisionID: sourceRevisionID,
		Locator: row.Locator, ContentDigest: row.ContentDigest, Metadata: cloneJSON(row.Metadata),
		Role: row.Role, FieldPath: optionalText(row.FieldPath), Note: optionalText(row.Note), CreatedAt: row.CreatedAt.Time,
	}, nil
}

func catalogRelation(
	row dbgen.ListDirectCatalogAssetRelationsRow,
	relationID identity.RelationID,
	counterpartID identity.AssetID,
	depth int,
) (domain.Relation, error) {
	subjectID, err := identity.AssetIDFromUUIDBytes(row.SubjectAssetID.Bytes)
	if err != nil {
		return domain.Relation{}, err
	}
	objectID, err := identity.AssetIDFromUUIDBytes(row.ObjectAssetID.Bytes)
	if err != nil {
		return domain.Relation{}, err
	}
	counterpart, err := catalogSummary(
		row.CounterpartID, row.Namespace, row.Key, row.AssetType, row.LifecycleState,
		row.CurrentRevisionID, row.CounterpartTitle, row.CounterpartSummary, row.UpdatedAt, 0,
	)
	if err != nil {
		return domain.Relation{}, err
	}
	sourceRevisionID, err := decodeOptionalSourceRevision(row.SourceRevisionID)
	if err != nil {
		return domain.Relation{}, err
	}
	evidenceID, err := decodeOptionalEvidence(row.EvidenceArtifactID)
	if err != nil {
		return domain.Relation{}, err
	}
	return domain.Relation{
		ID: relationID, Depth: depth, Direction: row.Direction,
		Predicate: semantic.RelationPredicate(row.Predicate), Plane: semantic.RelationPlane(row.Plane),
		AssertionState: semantic.AssertionState(row.AssertionState), SubjectAssetID: subjectID,
		ObjectAssetID: objectID, Counterpart: counterpart, SourceRevisionID: sourceRevisionID,
		EvidenceArtifactID: evidenceID, CreatedAt: row.CreatedAt.Time,
	}, nil
}

func catalogDiscoveryRun(row dbgen.DiscoveryRun, findings []dbgen.DiscoveryFinding) (domain.DiscoveryRun, error) {
	id, err := identity.RunIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.DiscoveryRun{}, err
	}
	sourceID, err := identity.SourceConnectionIDFromUUIDBytes(row.SourceConnectionID.Bytes)
	if err != nil {
		return domain.DiscoveryRun{}, err
	}
	sourceRevisionID, err := decodeOptionalSourceRevision(row.SourceRevisionID)
	if err != nil {
		return domain.DiscoveryRun{}, err
	}
	result := domain.DiscoveryRun{
		ID: id, SourceConnectionID: sourceID, SourceRevisionID: sourceRevisionID,
		AdapterVersion: row.AdapterVersion, Status: row.Status, ErrorCode: optionalText(row.ErrorCode),
		Stats: cloneJSON(row.Stats), StartedAt: optionalTime(row.StartedAt), CompletedAt: optionalTime(row.CompletedAt),
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		Findings: make([]domain.DiscoveryFinding, 0, len(findings)),
	}
	for _, finding := range findings {
		result.Findings = append(result.Findings, domain.DiscoveryFinding{
			Sequence: int(finding.Sequence), Code: finding.Code, Severity: finding.Severity,
			Locator: optionalText(finding.Locator), Details: cloneJSON(finding.Details),
		})
	}
	return result, nil
}

func linkEvidence(
	ctx context.Context,
	queries *dbgen.Queries,
	workspaceID, revisionID pgtype.UUID,
	evidenceIDs []identity.EvidenceID,
) error {
	for _, evidence := range evidenceIDs {
		evidenceID, err := uuidValue(evidence)
		if err != nil {
			return err
		}
		rows, err := queries.LinkRevisionEvidence(ctx, dbgen.LinkRevisionEvidenceParams{
			WorkspaceID: workspaceID, RevisionID: revisionID, EvidenceID: evidenceID,
		})
		if err != nil {
			return repositoryError("link revision evidence", err)
		}
		if rows != 1 {
			return semantic.ErrInvariant
		}
	}
	return nil
}

type catalogEvent struct {
	WorkspaceID    identity.WorkspaceID
	AssetID        identity.AssetID
	RevisionID     identity.RevisionID
	BaseRevisionID *identity.RevisionID
	AuditID        identity.EventID
	OutboxID       identity.EventID
	Sequence       int64
	Action         string
	Actor          string
	TraceID        string
	CreatedAt      time.Time
}

func createCatalogMutationEvents(ctx context.Context, queries *dbgen.Queries, event catalogEvent) error {
	payloadData := map[string]any{
		"assetId": event.AssetID.String(), "revisionId": event.RevisionID.String(), "sequence": event.Sequence,
	}
	if event.BaseRevisionID != nil {
		payloadData["baseRevisionId"] = event.BaseRevisionID.String()
	}
	if err := createMutationEvents(ctx, queries, mutationEvent{
		WorkspaceID: event.WorkspaceID, AuditID: event.AuditID, OutboxID: event.OutboxID,
		AuditType: "catalog.asset." + event.Action, OutboxType: "catalog.asset.changed",
		SpecVersion: "semlia.catalog/v1", Action: event.Action, Actor: event.Actor,
		TraceID: event.TraceID, CreatedAt: event.CreatedAt, Data: payloadData,
	}); err != nil {
		return repositoryError("create catalog mutation events", err)
	}
	return nil
}
