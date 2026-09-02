package postgres

import (
	"context"
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
	return detail, nil
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
