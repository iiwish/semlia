package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	application "github.com/iiwish/semlia/internal/application/discovery"
	domain "github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ application.Repository = (*Store)(nil)

func (store *Store) PersistDiscoverySnapshot(
	ctx context.Context,
	workspace identity.WorkspaceID,
	source identity.SourceConnectionID,
	snapshot domain.Snapshot,
) (application.PersistResult, error) {
	if err := snapshot.Canonicalize(); err != nil {
		return application.PersistResult{}, err
	}
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return application.PersistResult{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	sourceID, err := uuidValue(source)
	if err != nil {
		return application.PersistResult{}, fmt.Errorf("encode source connection ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return application.PersistResult{}, repositoryError("begin discovery transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)

	sourceRevision, err := createDiscoverySourceRevision(ctx, queries, workspaceID, sourceID, snapshot)
	if err != nil {
		return application.PersistResult{}, err
	}
	if !sourceRevision.Created {
		replayed, replayErr := replayedDiscoveryResult(ctx, queries, workspaceID, sourceRevision.ID, snapshot.AdapterVersion)
		if replayErr == nil {
			if err := tx.Commit(ctx); err != nil {
				return application.PersistResult{}, repositoryError("commit replayed discovery", err)
			}
			return replayed, nil
		}
		if replayErr != pgx.ErrNoRows {
			return application.PersistResult{}, repositoryError("resolve replayed discovery", replayErr)
		}
	}

	datasetIDs := make(map[string]pgtype.UUID, len(snapshot.Datasets))
	codeArtifactIDs := make(map[string]pgtype.UUID, len(snapshot.CodeArtifacts))
	findings := append([]domain.Finding(nil), snapshot.Findings...)
	fieldCount := 0
	for _, dataset := range snapshot.Datasets {
		if possibleRename, lookupErr := queries.FindPhysicalDatasetByQualifiedName(ctx, dbgen.FindPhysicalDatasetByQualifiedNameParams{
			WorkspaceID: workspaceID, SourceConnectionID: sourceID,
			QualifiedName: dataset.QualifiedName, ExternalKey: dataset.ExternalKey,
		}); lookupErr == nil {
			findings = append(findings, domain.Finding{
				Code: "POSSIBLE_RENAME", Severity: "warning", Locator: dataset.Locator,
				Details: map[string]any{"previous_external_key": possibleRename.ExternalKey, "new_external_key": dataset.ExternalKey},
			})
		} else if lookupErr != pgx.ErrNoRows {
			return application.PersistResult{}, repositoryError("detect possible dataset rename", lookupErr)
		}
		datasetRow, datasetRevisionID, persistErr := persistDataset(
			ctx, queries, workspaceID, sourceID, sourceRevision.ID, dataset,
		)
		if persistErr != nil {
			return application.PersistResult{}, persistErr
		}
		datasetIDs[dataset.ExternalKey] = datasetRow.ID
		for _, field := range dataset.Fields {
			if err := persistField(ctx, queries, workspaceID, datasetRow.ID, datasetRevisionID, field); err != nil {
				return application.PersistResult{}, err
			}
			fieldCount++
		}
	}
	for _, artifact := range snapshot.CodeArtifacts {
		row, persistErr := persistCodeArtifact(ctx, queries, workspaceID, sourceRevision.ID, artifact)
		if persistErr != nil {
			return application.PersistResult{}, persistErr
		}
		codeArtifactIDs[artifact.Path] = row.ID
	}
	lineageCount := 0
	for _, edge := range snapshot.Lineage {
		upstreamID, upstreamExists := datasetIDs[edge.UpstreamExternalKey]
		downstreamID, downstreamExists := datasetIDs[edge.DownstreamExternalKey]
		if !upstreamExists || !downstreamExists {
			findings = append(findings, domain.Finding{
				Code: "UNRESOLVED_LINEAGE", Severity: "warning", Locator: edge.CodePath,
				Details: map[string]any{
					"upstream_external_key":   edge.UpstreamExternalKey,
					"downstream_external_key": edge.DownstreamExternalKey,
				},
			})
			continue
		}
		lineageID, createErr := identity.NewLineageEdgeID()
		if createErr != nil {
			return application.PersistResult{}, createErr
		}
		databaseLineageID, createErr := uuidValue(lineageID)
		if createErr != nil {
			return application.PersistResult{}, createErr
		}
		rows, createErr := queries.CreateLineageEdge(ctx, dbgen.CreateLineageEdgeParams{
			ID: databaseLineageID, WorkspaceID: workspaceID, SourceRevisionID: sourceRevision.ID,
			UpstreamDatasetID: upstreamID, DownstreamDatasetID: downstreamID, EdgeKind: edge.Kind,
			CodeArtifactID: codeArtifactIDs[edge.CodePath], Confidence: numeric(edge.Confidence),
		})
		if createErr != nil {
			return application.PersistResult{}, repositoryError("create lineage edge", createErr)
		}
		lineageCount += int(rows)
	}

	status, errorCode := discoveryRunState(findings)
	runID, err := identity.NewRunID()
	if err != nil {
		return application.PersistResult{}, err
	}
	databaseRunID, err := uuidValue(runID)
	if err != nil {
		return application.PersistResult{}, err
	}
	stats := map[string]int{
		"datasets": len(snapshot.Datasets), "fields": fieldCount,
		"lineage": lineageCount, "findings": len(findings),
	}
	statsJSON, _ := json.Marshal(stats)
	run, err := queries.CreateCompletedDiscoveryRun(ctx, dbgen.CreateCompletedDiscoveryRunParams{
		ID: databaseRunID, WorkspaceID: workspaceID, SourceConnectionID: sourceID,
		SourceRevisionID: sourceRevision.ID, AdapterVersion: snapshot.AdapterVersion,
		Status: status, ErrorCode: optionalTextValue(errorCode), Stats: statsJSON,
		CompletedAt: timestamp(snapshot.ObservedAt),
	})
	if err != nil {
		return application.PersistResult{}, repositoryError("create discovery run", err)
	}
	for index, finding := range findings {
		details, marshalErr := json.Marshal(finding.Details)
		if marshalErr != nil {
			return application.PersistResult{}, domain.ErrInvalidSnapshot
		}
		if err := queries.CreateDiscoveryFinding(ctx, dbgen.CreateDiscoveryFindingParams{
			DiscoveryRunID: run.ID, Sequence: int32(index + 1), Code: finding.Code,
			Severity: finding.Severity, Locator: optionalTextValue(finding.Locator), Details: details,
		}); err != nil {
			return application.PersistResult{}, repositoryError("create discovery finding", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return application.PersistResult{}, repositoryError("commit discovery snapshot", err)
	}
	return discoveryResult(sourceRevision.ID, run.ID, status, false, stats)
}

func createDiscoverySourceRevision(
	ctx context.Context,
	queries *dbgen.Queries,
	workspaceID, sourceID pgtype.UUID,
	snapshot domain.Snapshot,
) (dbgen.CreateDiscoverySourceRevisionRow, error) {
	newID, err := identity.NewSourceRevisionID()
	if err != nil {
		return dbgen.CreateDiscoverySourceRevisionRow{}, err
	}
	databaseID, err := uuidValue(newID)
	if err != nil {
		return dbgen.CreateDiscoverySourceRevisionRow{}, err
	}
	metadata, _ := json.Marshal(map[string]string{"adapter_kind": snapshot.AdapterKind, "locator": snapshot.Locator})
	row, err := queries.CreateDiscoverySourceRevision(ctx, dbgen.CreateDiscoverySourceRevisionParams{
		ID: databaseID, WorkspaceID: workspaceID, SourceConnectionID: sourceID,
		ExternalRevision: optionalTextValue(snapshot.ExternalRevision), ContentDigest: snapshot.ContentDigest,
		AdapterVersion: snapshot.AdapterVersion, ObservedAt: timestamp(snapshot.ObservedAt), Metadata: metadata,
	})
	if err == pgx.ErrNoRows {
		existing, lookupErr := queries.GetSourceRevisionByDigest(ctx, dbgen.GetSourceRevisionByDigestParams{
			WorkspaceID: workspaceID, SourceConnectionID: sourceID, ContentDigest: snapshot.ContentDigest,
		})
		if lookupErr != nil {
			return dbgen.CreateDiscoverySourceRevisionRow{}, repositoryError("resolve concurrent source revision", lookupErr)
		}
		return dbgen.CreateDiscoverySourceRevisionRow{
			ID: existing.ID, WorkspaceID: existing.WorkspaceID, SourceConnectionID: existing.SourceConnectionID,
			ExternalRevision: existing.ExternalRevision, ContentDigest: existing.ContentDigest,
			AdapterVersion: existing.AdapterVersion, ObservedAt: existing.ObservedAt,
			Metadata: existing.Metadata, CreatedAt: existing.CreatedAt, Created: false,
		}, nil
	}
	if err != nil {
		return dbgen.CreateDiscoverySourceRevisionRow{}, repositoryError("create discovery source revision", err)
	}
	return row, nil
}

func replayedDiscoveryResult(
	ctx context.Context,
	queries *dbgen.Queries,
	workspaceID, sourceRevisionID pgtype.UUID,
	adapterVersion string,
) (application.PersistResult, error) {
	run, err := queries.GetSuccessfulDiscoveryRun(ctx, dbgen.GetSuccessfulDiscoveryRunParams{
		WorkspaceID: workspaceID, SourceRevisionID: sourceRevisionID, AdapterVersion: adapterVersion,
	})
	if err != nil {
		return application.PersistResult{}, err
	}
	stats := make(map[string]int)
	if err := json.Unmarshal(run.Stats, &stats); err != nil {
		return application.PersistResult{}, err
	}
	return discoveryResult(sourceRevisionID, run.ID, run.Status, true, stats)
}

func persistDataset(
	ctx context.Context,
	queries *dbgen.Queries,
	workspaceID, sourceID, sourceRevisionID pgtype.UUID,
	dataset domain.Dataset,
) (dbgen.PhysicalDataset, pgtype.UUID, error) {
	newID, err := identity.NewPhysicalDatasetID()
	if err != nil {
		return dbgen.PhysicalDataset{}, pgtype.UUID{}, err
	}
	databaseID, err := uuidValue(newID)
	if err != nil {
		return dbgen.PhysicalDataset{}, pgtype.UUID{}, err
	}
	row, err := queries.UpsertPhysicalDataset(ctx, dbgen.UpsertPhysicalDatasetParams{
		ID: databaseID, WorkspaceID: workspaceID, SourceConnectionID: sourceID,
		ExternalKey: dataset.ExternalKey, QualifiedName: dataset.QualifiedName,
	})
	if err != nil {
		return dbgen.PhysicalDataset{}, pgtype.UUID{}, repositoryError("upsert physical dataset", err)
	}
	current, err := queries.GetCurrentPhysicalDatasetRevision(ctx, dbgen.GetCurrentPhysicalDatasetRevisionParams{
		WorkspaceID: workspaceID, PhysicalDatasetID: row.ID,
	})
	if err == nil && current.ContentDigest == dataset.Fingerprint {
		return row, current.ID, nil
	}
	if err != nil && err != pgx.ErrNoRows {
		return dbgen.PhysicalDataset{}, pgtype.UUID{}, repositoryError("get current physical dataset revision", err)
	}
	revisionID, err := identity.NewPhysicalDatasetRevisionID()
	if err != nil {
		return dbgen.PhysicalDataset{}, pgtype.UUID{}, err
	}
	databaseRevisionID, err := uuidValue(revisionID)
	if err != nil {
		return dbgen.PhysicalDataset{}, pgtype.UUID{}, err
	}
	metadata, _ := json.Marshal(map[string]string{"fingerprint": dataset.Fingerprint})
	revision, err := queries.CreatePhysicalDatasetRevision(ctx, dbgen.CreatePhysicalDatasetRevisionParams{
		ID: databaseRevisionID, WorkspaceID: workspaceID, PhysicalDatasetID: row.ID,
		SourceRevisionID: sourceRevisionID, DatasetKind: dataset.Kind, Locator: dataset.Locator,
		ContentDigest: dataset.Fingerprint, Metadata: metadata,
	})
	if err != nil {
		return dbgen.PhysicalDataset{}, pgtype.UUID{}, repositoryError("create physical dataset revision", err)
	}
	rows, err := queries.SetCurrentPhysicalDatasetRevision(ctx, dbgen.SetCurrentPhysicalDatasetRevisionParams{
		RevisionID: revision.ID, WorkspaceID: workspaceID, PhysicalDatasetID: row.ID,
	})
	if err != nil || rows != 1 {
		return dbgen.PhysicalDataset{}, pgtype.UUID{}, repositoryError("set current physical dataset revision", err)
	}
	return row, revision.ID, nil
}

func persistField(
	ctx context.Context,
	queries *dbgen.Queries,
	workspaceID, datasetID, datasetRevisionID pgtype.UUID,
	field domain.Field,
) error {
	newID, err := identity.NewPhysicalFieldID()
	if err != nil {
		return err
	}
	databaseID, err := uuidValue(newID)
	if err != nil {
		return err
	}
	row, err := queries.UpsertPhysicalField(ctx, dbgen.UpsertPhysicalFieldParams{
		ID: databaseID, WorkspaceID: workspaceID, PhysicalDatasetID: datasetID,
		ExternalKey: field.ExternalKey, Name: field.Name,
	})
	if err != nil {
		return repositoryError("upsert physical field", err)
	}
	current, err := queries.GetCurrentPhysicalFieldRevision(ctx, dbgen.GetCurrentPhysicalFieldRevisionParams{
		WorkspaceID: workspaceID, PhysicalFieldID: row.ID,
	})
	if err == nil && fieldFingerprint(current.Metadata) == field.Fingerprint && current.DatasetRevisionID == datasetRevisionID {
		return nil
	}
	if err != nil && err != pgx.ErrNoRows {
		return repositoryError("get current physical field revision", err)
	}
	revisionID, err := identity.NewPhysicalFieldRevisionID()
	if err != nil {
		return err
	}
	databaseRevisionID, err := uuidValue(revisionID)
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{"fingerprint": field.Fingerprint})
	revision, err := queries.CreatePhysicalFieldRevision(ctx, dbgen.CreatePhysicalFieldRevisionParams{
		ID: databaseRevisionID, WorkspaceID: workspaceID, PhysicalFieldID: row.ID,
		DatasetRevisionID: datasetRevisionID, Ordinal: int32(field.Ordinal),
		DataType: field.DataType, Nullable: field.Nullable, Metadata: metadata,
	})
	if err != nil {
		return repositoryError("create physical field revision", err)
	}
	rows, err := queries.SetCurrentPhysicalFieldRevision(ctx, dbgen.SetCurrentPhysicalFieldRevisionParams{
		RevisionID: revision.ID, WorkspaceID: workspaceID, PhysicalFieldID: row.ID,
	})
	if err != nil || rows != 1 {
		return repositoryError("set current physical field revision", err)
	}
	return nil
}

func persistCodeArtifact(
	ctx context.Context,
	queries *dbgen.Queries,
	workspaceID, sourceRevisionID pgtype.UUID,
	artifact domain.CodeArtifact,
) (dbgen.CodeArtifact, error) {
	id, err := identity.NewCodeArtifactID()
	if err != nil {
		return dbgen.CodeArtifact{}, err
	}
	databaseID, err := uuidValue(id)
	if err != nil {
		return dbgen.CodeArtifact{}, err
	}
	row, err := queries.UpsertCodeArtifact(ctx, dbgen.UpsertCodeArtifactParams{
		ID: databaseID, WorkspaceID: workspaceID, SourceRevisionID: sourceRevisionID,
		Path: artifact.Path, BlobOid: optionalTextValue(artifact.BlobOID),
		Language: artifact.Language, ContentDigest: artifact.ContentDigest,
	})
	if err != nil {
		return dbgen.CodeArtifact{}, repositoryError("upsert code artifact", err)
	}
	return row, nil
}

func discoveryRunState(findings []domain.Finding) (string, string) {
	for _, finding := range findings {
		if finding.Terminal {
			return "failed", "UNSUPPORTED_ARTIFACT"
		}
	}
	return "succeeded", ""
}

func discoveryResult(
	sourceRevisionUUID, runUUID pgtype.UUID,
	status string,
	replayed bool,
	stats map[string]int,
) (application.PersistResult, error) {
	sourceRevisionID, err := identity.SourceRevisionIDFromUUIDBytes(sourceRevisionUUID.Bytes)
	if err != nil {
		return application.PersistResult{}, err
	}
	runID, err := identity.RunIDFromUUIDBytes(runUUID.Bytes)
	if err != nil {
		return application.PersistResult{}, err
	}
	return application.PersistResult{
		SourceRevisionID: sourceRevisionID, RunID: runID, Status: status, Replayed: replayed,
		DatasetCount: stats["datasets"], FieldCount: stats["fields"],
		LineageCount: stats["lineage"], FindingCount: stats["findings"],
	}, nil
}

func fieldFingerprint(metadata []byte) string {
	value := make(map[string]string)
	_ = json.Unmarshal(metadata, &value)
	return value["fingerprint"]
}

func numeric(value float64) pgtype.Numeric {
	scaled := int64(value * 1000)
	return pgtype.Numeric{Int: big.NewInt(scaled), Exp: -3, Valid: true}
}
