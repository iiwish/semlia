package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	application "github.com/iiwish/semlia/internal/application/discovery"
	domain "github.com/iiwish/semlia/internal/domain/discovery"
	operationsdomain "github.com/iiwish/semlia/internal/domain/operations"
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
	return store.persistDiscoverySnapshot(ctx, workspace, source, snapshot, nil, nil)
}

func (store *Store) PersistDiscoveryRunSnapshot(
	ctx context.Context,
	workspace identity.WorkspaceID,
	source identity.SourceConnectionID,
	run identity.RunID,
	snapshot domain.Snapshot,
) (application.PersistResult, error) {
	fence, _ := application.RunCommitFenceFromContext(ctx)
	return store.persistDiscoverySnapshot(ctx, workspace, source, snapshot, &run, &fence)
}

func (store *Store) persistDiscoverySnapshot(
	ctx context.Context,
	workspace identity.WorkspaceID,
	source identity.SourceConnectionID,
	snapshot domain.Snapshot,
	requestedRun *identity.RunID,
	fence *application.RunCommitFence,
) (application.PersistResult, error) {
	snapshot = snapshot.Clone()
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
	if err := lockDiscoveryWorkspace(ctx, tx, workspace, 0); err != nil {
		return application.PersistResult{}, err
	}
	queries := dbgen.New(tx)
	if err := prepareSourceSnapshot(ctx, tx, workspaceID, sourceID, requestedRun, &snapshot); err != nil {
		return application.PersistResult{}, err
	}
	publishCurrent, err := canPublishSourceProjection(ctx, tx, workspaceID, sourceID, requestedRun, snapshot.ObservedAt)
	if err != nil {
		return application.PersistResult{}, err
	}

	sourceRevision, err := createDiscoverySourceRevision(ctx, queries, workspaceID, sourceID, snapshot)
	if err != nil {
		return application.PersistResult{}, err
	}
	// Serialize projection publication for this exact revision, including retry
	// after a rolled-back projection. Only committed successful runs are reusable.
	if _, err := tx.Exec(ctx, `SELECT id FROM source_revisions WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, workspaceID, sourceRevision.ID); err != nil {
		return application.PersistResult{}, repositoryError("lock discovery projection", err)
	}
	if !sourceRevision.Created && requestedRun == nil {
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
	var reusedRun *dbgen.DiscoveryRun
	if !sourceRevision.Created && requestedRun != nil {
		completed, lookupErr := queries.GetSuccessfulDiscoveryRun(ctx, dbgen.GetSuccessfulDiscoveryRunParams{
			WorkspaceID: workspaceID, SourceRevisionID: sourceRevision.ID, AdapterVersion: snapshot.AdapterVersion,
		})
		if lookupErr == nil {
			reusedRun = &completed
		} else if lookupErr != pgx.ErrNoRows {
			return application.PersistResult{}, repositoryError("resolve exact discovery projection", lookupErr)
		}
	}

	findings := append([]domain.Finding(nil), snapshot.Findings...)
	fieldCount, lineageCount := 0, 0
	var members []domain.SnapshotMember
	observedDatasets := map[string]domain.SnapshotMember{}
	{
		datasetIDs := make(map[string]pgtype.UUID, len(snapshot.Datasets))
		codeArtifactIDs := make(map[string]pgtype.UUID, len(snapshot.CodeArtifacts))
		for _, dataset := range snapshot.Datasets {
			if possibleRename, lookupErr := queries.FindPhysicalDatasetByQualifiedName(ctx, dbgen.FindPhysicalDatasetByQualifiedNameParams{
				WorkspaceID: workspaceID, SourceConnectionID: sourceID,
				QualifiedName: dataset.QualifiedName, ExternalKey: dataset.ExternalKey,
			}); lookupErr == nil {
				findings = append(findings, domain.Finding{
					Code: "POSSIBLE_RENAME", Severity: "warning", Locator: dataset.Locator, CoverageKey: dataset.CoverageKey,
					Details: map[string]any{"previous_external_key": possibleRename.ExternalKey, "new_external_key": dataset.ExternalKey},
				})
			} else if lookupErr != pgx.ErrNoRows {
				return application.PersistResult{}, repositoryError("detect possible dataset rename", lookupErr)
			}
			datasetRow, datasetRevisionID, persistErr := persistDataset(
				ctx, queries, workspaceID, sourceID, sourceRevision.ID, dataset, publishCurrent,
			)
			if persistErr != nil {
				return application.PersistResult{}, persistErr
			}
			datasetIDs[dataset.ExternalKey] = datasetRow.ID
			member := domain.SnapshotMember{Kind: "dataset", ObjectID: snapshotWireID(identity.PhysicalDataset, datasetRow.ID), RevisionID: snapshotWireID(identity.PhysicalDatasetRevision, datasetRevisionID), Name: dataset.QualifiedName, Locator: dataset.Locator, ContentDigest: dataset.Fingerprint, CoverageKey: dataset.CoverageKey}
			members = append(members, member)
			observedDatasets[dataset.ExternalKey] = member
			for _, field := range dataset.Fields {
				fieldID, fieldRev, err := persistField(ctx, queries, workspaceID, datasetRow.ID, datasetRevisionID, field, publishCurrent)
				if err != nil {
					return application.PersistResult{}, err
				}
				members = append(members, domain.SnapshotMember{Kind: "field", ObjectID: snapshotWireID(identity.PhysicalField, fieldID), RevisionID: snapshotWireID(identity.PhysicalFieldRevision, fieldRev), Name: field.Name, Locator: dataset.Locator + "#" + field.Name, ContentDigest: field.Fingerprint, CoverageKey: dataset.CoverageKey, ParentObjectID: member.ObjectID, ParentRevisionID: member.RevisionID})
				fieldCount++
			}
		}
		if reusedRun == nil {
			for _, artifact := range snapshot.CodeArtifacts {
				row, persistErr := persistCodeArtifact(ctx, queries, workspaceID, sourceRevision.ID, artifact)
				if persistErr != nil {
					return application.PersistResult{}, persistErr
				}
				codeArtifactIDs[artifact.Path] = row.ID
			}
			for _, edge := range snapshot.Lineage {
				upstreamID, upstreamExists := datasetIDs[edge.UpstreamExternalKey]
				downstreamID, downstreamExists := datasetIDs[edge.DownstreamExternalKey]
				if !upstreamExists || !downstreamExists {
					findings = append(findings, domain.Finding{
						Code: "UNRESOLVED_LINEAGE", Severity: "warning", Locator: edge.CodePath, CoverageKey: edge.CoverageKey,
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
		}
	}

	status, errorCode := discoveryRunState(findings)
	runID := identity.RunID{}
	if requestedRun == nil {
		runID, err = identity.NewRunID()
		if err != nil {
			return application.PersistResult{}, err
		}
	} else {
		runID = *requestedRun
	}
	databaseRunID, err := uuidValue(runID)
	if err != nil {
		return application.PersistResult{}, err
	}
	stats := map[string]int{
		"datasets": len(snapshot.Datasets), "fields": fieldCount,
		"lineage": lineageCount, "findings": len(findings),
	}
	if reusedRun != nil {
		status, errorCode = reusedRun.Status, reusedRun.ErrorCode.String
		if err := json.Unmarshal(reusedRun.Stats, &stats); err != nil {
			return application.PersistResult{}, repositoryError("decode reused discovery stats", err)
		}
	}
	statsJSON, _ := json.Marshal(stats)
	runUUID := databaseRunID
	if requestedRun == nil {
		run, createErr := queries.CreateCompletedDiscoveryRun(ctx, dbgen.CreateCompletedDiscoveryRunParams{
			ID: databaseRunID, WorkspaceID: workspaceID, SourceConnectionID: sourceID,
			SourceRevisionID: sourceRevision.ID, AdapterVersion: snapshot.AdapterVersion,
			Status: status, ErrorCode: optionalTextValue(errorCode), Stats: statsJSON,
			CompletedAt: timestamp(snapshot.ObservedAt),
		})
		if createErr != nil {
			return application.PersistResult{}, repositoryError("create discovery run", createErr)
		}
		runUUID = run.ID
	} else {
		command, updateErr := tx.Exec(ctx, `UPDATE discovery_runs
SET source_revision_id = $3, adapter_version = $4, status = $5, error_code = $6,
    stats = $7, completed_at = $8, updated_at = $8, projection_reused = $10
WHERE workspace_id = $1 AND id = $2 AND source_connection_id = $9 AND status = 'running'`,
			workspaceID, databaseRunID, sourceRevision.ID, snapshot.AdapterVersion, status,
			optionalTextValue(errorCode), statsJSON, timestamp(snapshot.ObservedAt), sourceID, reusedRun != nil)
		if updateErr != nil || command.RowsAffected() != 1 {
			return application.PersistResult{}, repositoryError("complete exact discovery run", updateErr)
		}
	}
	if reusedRun != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO discovery_findings (discovery_run_id, sequence, code, severity, locator, details)
SELECT $1, sequence, code, severity, locator, details FROM discovery_findings WHERE discovery_run_id=$2`, runUUID, reusedRun.ID); err != nil {
			return application.PersistResult{}, repositoryError("reuse discovery findings", err)
		}
		findings = nil
	}
	for index, finding := range findings {
		details, marshalErr := json.Marshal(finding.Details)
		if marshalErr != nil {
			return application.PersistResult{}, domain.ErrInvalidSnapshot
		}
		if err := queries.CreateDiscoveryFinding(ctx, dbgen.CreateDiscoveryFindingParams{
			DiscoveryRunID: runUUID, Sequence: int32(index + 1), Code: finding.Code,
			Severity: finding.Severity, Locator: optionalTextValue(finding.Locator), Details: details,
		}); err != nil {
			return application.PersistResult{}, repositoryError("create discovery finding", err)
		}
	}
	if requestedRun != nil && sourceRevision.Created {
		if _, err := tx.Exec(ctx, `INSERT INTO source_revision_artifact_sets
(workspace_id, source_connection_id, source_revision_id, artifact_set_id)
SELECT workspace_id, source_connection_id, $3, artifact_set_id
FROM discovery_runs
WHERE workspace_id=$1 AND id=$2 AND artifact_set_id IS NOT NULL`, workspaceID, runUUID, sourceRevision.ID); err != nil {
			return application.PersistResult{}, repositoryError("pin discovery revision artifact set", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO source_revision_artifacts
(workspace_id, source_connection_id, source_revision_id, source_artifact_id, artifact_set_id,
 logical_path, ordinal, content_sha256)
SELECT workspace_id, source_connection_id, $3, source_artifact_id, artifact_set_id,
       logical_path, ordinal, content_sha256
FROM discovery_run_artifacts
WHERE workspace_id=$1 AND discovery_run_id=$2
ON CONFLICT DO NOTHING`, workspaceID, runUUID, sourceRevision.ID); err != nil {
			return application.PersistResult{}, repositoryError("pin discovery revision artifacts", err)
		}
	}
	if requestedRun != nil {
		if status != "failed" {
			if _, err := tx.Exec(ctx, `UPDATE source_artifacts artifact SET status='consumed'
			FROM discovery_run_artifacts pin
			WHERE pin.workspace_id=$1 AND pin.discovery_run_id=$2
			  AND artifact.workspace_id=pin.workspace_id AND artifact.id=pin.source_artifact_id
			  AND artifact.source_connection_id=pin.source_connection_id
			  AND artifact.content_sha256=pin.content_sha256 AND artifact.status='validated'`, workspaceID, runUUID); err != nil {
				return application.PersistResult{}, repositoryError("consume exact discovery artifacts", err)
			}
		}
		if err := persistDiscoveryObservations(ctx, tx, workspaceID, runUUID, sourceRevision.ID, snapshot); err != nil {
			return application.PersistResult{}, err
		}
		if status != "failed" && reusedRun == nil {
			if err := persistSemanticCandidates(ctx, tx, workspaceID, sourceID, runUUID, sourceRevision.ID, snapshot); err != nil {
				return application.PersistResult{}, err
			}
		}
		stored, loadErr := queries.GetOperationsRuntimeRunBySource(ctx, dbgen.GetOperationsRuntimeRunBySourceParams{
			WorkspaceID: workspaceID, Kind: string(operationsdomain.RunKindDiscovery),
			SourceType: "discovery_run", SourceID: requestedRun.String(),
		})
		if loadErr != nil {
			return application.PersistResult{}, repositoryError("load discovery terminal projection", loadErr)
		}
		projected, decodeErr := runtimeRunFromRow(stored)
		if decodeErr != nil {
			return application.PersistResult{}, decodeErr
		}
		projected.State, projected.Phase = operationsdomain.RunState(status), status
		projected.SourceVersionDigest = normalizedRuntimeDigest(snapshot.ContentDigest)
		projected.FinishedAt, projected.UpdatedAt = timePointer(snapshot.ObservedAt), snapshot.ObservedAt.UTC()
		projected.ErrorCode, projected.ErrorSummary = "", ""
		if projected.State == operationsdomain.RunFailed {
			projected.ErrorCode, projected.ErrorSummary = errorCode, "discovery run failed"
		}
		event, eventErr := newRuntimeEvent(workspace, projected.ID, "completed:"+status,
			operationsdomain.RunEventState, projected.State, status, projected.ErrorCode,
			projected.ErrorSummary, snapshot.ObservedAt)
		if eventErr != nil {
			return application.PersistResult{}, eventErr
		}
		if projectErr := projectRuntime(ctx, queries, projected, event); projectErr != nil {
			return application.PersistResult{}, repositoryError("project discovery terminal state", projectErr)
		}
		storedRun, attentionErr := queries.GetCatalogDiscoveryRun(ctx, dbgen.GetCatalogDiscoveryRunParams{
			WorkspaceID: workspaceID, RunID: runUUID,
		})
		if attentionErr != nil {
			return application.PersistResult{}, repositoryError("load discovery attention source", attentionErr)
		}
		if attentionErr := projectDiscoveryAttention(ctx, queries, storedRun, snapshot.ObservedAt); attentionErr != nil {
			return application.PersistResult{}, repositoryError("project discovery attention", attentionErr)
		}
	}
	if requestedRun != nil {
		var jobID pgtype.UUID
		if fence != nil && !fence.JobID.IsZero() && strings.TrimSpace(fence.LeaseOwner) != "" {
			jobID, err = uuidValue(fence.JobID)
			if err != nil {
				return application.PersistResult{}, err
			}
		} else {
			err = tx.QueryRow(ctx, `SELECT job_id FROM discovery_runs
WHERE workspace_id=$1 AND id=$2 AND source_connection_id=$3`, workspaceID, databaseRunID, sourceID).Scan(&jobID)
			if err != nil {
				return application.PersistResult{}, repositoryError("load discovery commit job", err)
			}
		}
		if err := lockDiscoveryJobLease(ctx, tx, workspaceID, databaseRunID, jobID); err != nil {
			return application.PersistResult{}, repositoryError("fence discovery commit", err)
		}
	}
	snapshotID, err := persistSourceHistory(ctx, tx, workspaceID, sourceID, sourceRevision.ID, runUUID, snapshot, findings, reusedRun, members, observedDatasets)
	if err != nil {
		return application.PersistResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return application.PersistResult{}, repositoryError("commit discovery snapshot", err)
	}
	result, err := discoveryResult(sourceRevision.ID, runUUID, status, reusedRun != nil, stats)
	result.SnapshotID = snapshotID
	return result, err
}

func persistDiscoveryObservations(
	ctx context.Context, tx pgx.Tx, workspaceID, runID, sourceRevisionID pgtype.UUID, snapshot domain.Snapshot,
) error {
	for _, key := range snapshot.Keys {
		fields, _ := json.Marshal(key.FieldExternalKeys)
		if _, err := tx.Exec(ctx, `INSERT INTO physical_key_observations
(workspace_id, discovery_run_id, source_revision_id, dataset_external_key, constraint_name, field_external_keys, key_kind)
VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, workspaceID, runID, sourceRevisionID,
			key.DatasetExternalKey, key.ConstraintName, fields, key.Kind); err != nil {
			return repositoryError("persist physical key observation", err)
		}
	}
	for _, join := range snapshot.Joins {
		fromFields, _ := json.Marshal(join.FromFieldExternalKeys)
		toFields, _ := json.Marshal(join.ToFieldExternalKeys)
		if _, err := tx.Exec(ctx, `INSERT INTO join_observations
(workspace_id, discovery_run_id, source_revision_id, constraint_name, from_dataset_external_key,
 from_field_external_keys, to_dataset_external_key, to_field_external_keys)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, workspaceID, runID, sourceRevisionID,
			join.ConstraintName, join.FromDatasetExternalKey, fromFields, join.ToDatasetExternalKey, toFields); err != nil {
			return repositoryError("persist join observation", err)
		}
	}
	return nil
}

func persistSemanticCandidates(
	ctx context.Context, tx pgx.Tx, workspaceID, sourceID, runID, sourceRevisionID pgtype.UUID, snapshot domain.Snapshot,
) error {
	sourceRevisionTypeID, err := identity.SourceRevisionIDFromUUIDBytes(sourceRevisionID.Bytes)
	if err != nil {
		return err
	}
	runTypeID, err := identity.RunIDFromUUIDBytes(runID.Bytes)
	if err != nil {
		return err
	}
	for _, dataset := range snapshot.Datasets {
		candidateID, err := identity.NewSemanticCandidateID()
		if err != nil {
			return err
		}
		databaseCandidateID, err := uuidValue(candidateID)
		if err != nil {
			return err
		}
		proposalInput, _ := json.Marshal(map[string]any{
			"schemaVersion": "semlia.proposal-input/v1", "candidateKind": "data_asset",
			"qualifiedName": dataset.QualifiedName, "fields": dataset.Fields,
		})
		evidence, _ := json.Marshal([]map[string]string{{
			"sourceRevisionId": sourceRevisionTypeID.String(), "discoveryRunId": runTypeID.String(), "locator": dataset.Locator,
		}})
		digestBytes := sha256.Sum256(append(append([]byte(dataset.ExternalKey+"\x00"), proposalInput...), evidence...))
		digest := "sha256:" + hex.EncodeToString(digestBytes[:])
		if _, err := tx.Exec(ctx, `INSERT INTO semantic_candidates
(id, workspace_id, source_connection_id, source_revision_id, discovery_run_id, candidate_key,
 candidate_kind, title, proposal_input, evidence, content_digest)
VALUES ($1,$2,$3,$4,$5,$6,'data_asset',$7,$8,$9,$10) ON CONFLICT (discovery_run_id, candidate_key) DO NOTHING`,
			databaseCandidateID, workspaceID, sourceID, sourceRevisionID, runID, "data_asset:"+dataset.ExternalKey,
			dataset.QualifiedName, proposalInput, evidence, digest); err != nil {
			return repositoryError("persist semantic candidate", err)
		}
	}
	return nil
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
	result, err := discoveryResult(sourceRevisionID, run.ID, run.Status, true, stats)
	if err != nil {
		return result, err
	}
	snapshotID, err := queries.GetSourceSnapshotForRun(ctx, dbgen.GetSourceSnapshotForRunParams{WorkspaceID: workspaceID, RunID: run.ID})
	if err != nil {
		return result, err
	}
	result.SnapshotID = snapshotWireID(identity.SourceSnapshot, snapshotID)
	return result, nil
}

func persistDataset(
	ctx context.Context,
	queries *dbgen.Queries,
	workspaceID, sourceID, sourceRevisionID pgtype.UUID,
	dataset domain.Dataset,
	publishCurrent bool,
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
		PublishCurrent: publishCurrent,
	})
	if err != nil {
		return dbgen.PhysicalDataset{}, pgtype.UUID{}, repositoryError("upsert physical dataset", err)
	}
	revision, err := queries.GetPhysicalDatasetRevisionByDigest(ctx, dbgen.GetPhysicalDatasetRevisionByDigestParams{
		WorkspaceID: workspaceID, PhysicalDatasetID: row.ID, ContentDigest: dataset.Fingerprint,
	})
	if err != nil && err != pgx.ErrNoRows {
		return dbgen.PhysicalDataset{}, pgtype.UUID{}, repositoryError("get observed physical dataset revision", err)
	}
	if err == pgx.ErrNoRows {
		revisionID, err := identity.NewPhysicalDatasetRevisionID()
		if err != nil {
			return dbgen.PhysicalDataset{}, pgtype.UUID{}, err
		}
		databaseRevisionID, err := uuidValue(revisionID)
		if err != nil {
			return dbgen.PhysicalDataset{}, pgtype.UUID{}, err
		}
		metadata, _ := json.Marshal(map[string]string{"fingerprint": dataset.Fingerprint})
		revision, err = queries.CreatePhysicalDatasetRevision(ctx, dbgen.CreatePhysicalDatasetRevisionParams{
			ID: databaseRevisionID, WorkspaceID: workspaceID, PhysicalDatasetID: row.ID,
			SourceRevisionID: sourceRevisionID, DatasetKind: dataset.Kind, Locator: dataset.Locator,
			ContentDigest: dataset.Fingerprint, Metadata: metadata,
		})
		if err != nil {
			return dbgen.PhysicalDataset{}, pgtype.UUID{}, repositoryError("create physical dataset revision", err)
		}
	}
	if publishCurrent {
		rows, err := queries.SetCurrentPhysicalDatasetRevision(ctx, dbgen.SetCurrentPhysicalDatasetRevisionParams{
			RevisionID: revision.ID, WorkspaceID: workspaceID, PhysicalDatasetID: row.ID,
		})
		if err != nil || rows != 1 {
			return dbgen.PhysicalDataset{}, pgtype.UUID{}, repositoryError("set current physical dataset revision", err)
		}
	}
	return row, revision.ID, nil
}

func persistField(
	ctx context.Context,
	queries *dbgen.Queries,
	workspaceID, datasetID, datasetRevisionID pgtype.UUID,
	field domain.Field,
	publishCurrent bool,
) (pgtype.UUID, pgtype.UUID, error) {
	newID, err := identity.NewPhysicalFieldID()
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	databaseID, err := uuidValue(newID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	row, err := queries.UpsertPhysicalField(ctx, dbgen.UpsertPhysicalFieldParams{
		ID: databaseID, WorkspaceID: workspaceID, PhysicalDatasetID: datasetID,
		ExternalKey: field.ExternalKey, Name: field.Name,
		PublishCurrent: publishCurrent,
	})
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, repositoryError("upsert physical field", err)
	}
	revision, err := queries.GetPhysicalFieldRevisionByDigest(ctx, dbgen.GetPhysicalFieldRevisionByDigestParams{
		WorkspaceID: workspaceID, PhysicalFieldID: row.ID, DatasetRevisionID: datasetRevisionID, ContentDigest: field.Fingerprint,
	})
	if err != nil && err != pgx.ErrNoRows {
		return pgtype.UUID{}, pgtype.UUID{}, repositoryError("get observed physical field revision", err)
	}
	if err == pgx.ErrNoRows {
		revisionID, err := identity.NewPhysicalFieldRevisionID()
		if err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, err
		}
		databaseRevisionID, err := uuidValue(revisionID)
		if err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, err
		}
		metadata, _ := json.Marshal(map[string]string{"fingerprint": field.Fingerprint})
		revision, err = queries.CreatePhysicalFieldRevision(ctx, dbgen.CreatePhysicalFieldRevisionParams{
			ID: databaseRevisionID, WorkspaceID: workspaceID, PhysicalFieldID: row.ID,
			DatasetRevisionID: datasetRevisionID, Ordinal: int32(field.Ordinal),
			DataType: field.DataType, Nullable: field.Nullable, Metadata: metadata,
		})
		if err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, repositoryError("create physical field revision", err)
		}
	}
	if publishCurrent {
		rows, err := queries.SetCurrentPhysicalFieldRevision(ctx, dbgen.SetCurrentPhysicalFieldRevisionParams{
			RevisionID: revision.ID, WorkspaceID: workspaceID, PhysicalFieldID: row.ID,
		})
		if err != nil || rows != 1 {
			return pgtype.UUID{}, pgtype.UUID{}, repositoryError("set current physical field revision", err)
		}
	}
	return row.ID, revision.ID, nil
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
	degraded := false
	for _, finding := range findings {
		if finding.Terminal {
			return "failed", "UNSUPPORTED_ARTIFACT"
		}
		if finding.Severity == "warning" || finding.Severity == "error" {
			degraded = true
		}
	}
	if degraded {
		return "degraded", ""
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
