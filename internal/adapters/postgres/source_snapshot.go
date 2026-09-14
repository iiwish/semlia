package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	application "github.com/iiwish/semlia/internal/application/discovery"
	domain "github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func prepareSourceSnapshot(ctx context.Context, tx pgx.Tx, workspace, source pgtype.UUID, run *identity.RunID, snapshot *domain.Snapshot) error {
	var config []byte
	if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('metadata',metadata,'artifactPaths',artifact_paths,'sourceVersion',version) FROM source_connections WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, workspace, source).Scan(&config); err != nil {
		return sourceRepositoryError("lock snapshot source", err)
	}
	if run != nil {
		if err := tx.QueryRow(ctx, `SELECT source_config FROM discovery_runs WHERE workspace_id=$1 AND source_connection_id=$2 AND id=$3`, workspace, source, run.UUID()).Scan(&config); err != nil {
			return sourceRepositoryError("read pinned snapshot config", err)
		}
	}
	for index := range snapshot.Coverage {
		snapshot.Coverage[index].ConfigDigest = domain.SnapshotDigest(struct {
			Adapter string
			Config  json.RawMessage
		}{snapshot.Coverage[index].ConfigDigest, config})
	}
	// Source revisions bind the exact declared projection, not just a file hash.
	snapshot.ContentDigest = domain.SnapshotDigest(struct {
		Schema, Adapter, Input, Scope string
		Datasets                      []domain.Dataset
		Code                          []domain.CodeArtifact
		Lineage                       []domain.LineageEdge
		Coverage                      []domain.CoverageUnit
		Findings                      []domain.Finding
	}{"semlia.source-projection/v1", snapshot.AdapterVersion, snapshot.ContentDigest, snapshot.ScopeDigest(), snapshot.Datasets, snapshot.CodeArtifacts, snapshot.Lineage, snapshot.Coverage, snapshot.Findings})
	return nil
}

func canPublishSourceProjection(ctx context.Context, tx pgx.Tx, workspace, source pgtype.UUID, run *identity.RunID, observedAt time.Time) (bool, error) {
	var runID any
	if run != nil {
		runID = run.UUID()
	}
	var publish bool
	err := tx.QueryRow(ctx, `SELECT NOT EXISTS (
 SELECT 1 FROM discovery_runs r WHERE r.workspace_id=$1 AND r.source_connection_id=$2
 AND (CASE WHEN r.job_id IS NULL THEN r.started_at ELSE r.created_at END,r.id) >
 (COALESCE((SELECT created_at FROM discovery_runs WHERE workspace_id=$1 AND id=$3::uuid),$4),COALESCE($3::uuid,'ffffffff-ffff-ffff-ffff-ffffffffffff'::uuid))
) AND ($3::uuid IS NULL OR EXISTS (
 SELECT 1 FROM discovery_runs r JOIN source_connections s ON s.workspace_id=r.workspace_id AND s.id=r.source_connection_id
 WHERE r.workspace_id=$1 AND r.id=$3::uuid AND (r.source_config->>'sourceVersion')::bigint=s.version
))`, workspace, source, runID, observedAt).Scan(&publish)
	return publish, sourceRepositoryError("order source projection", err)
}

// Attempt declarations come from the run's immutable configuration and artifact pins.
func declaredRunSnapshot(ctx context.Context, tx pgx.Tx, workspace, run pgtype.UUID, now time.Time, code string) (pgtype.UUID, domain.Snapshot, error) {
	var source pgtype.UUID
	var kind, version string
	var config []byte
	err := tx.QueryRow(ctx, `SELECT r.source_connection_id,s.source_kind,r.adapter_version,r.source_config FROM discovery_runs r JOIN source_connections s ON s.workspace_id=r.workspace_id AND s.id=r.source_connection_id WHERE r.workspace_id=$1 AND r.id=$2`, workspace, run).Scan(&source, &kind, &version, &config)
	if err != nil {
		return source, domain.Snapshot{}, err
	}
	adapter, adapterVersion, ok := strings.Cut(version, "/")
	if !ok {
		adapter, adapterVersion = kind, version
	}
	snapshot := domain.Snapshot{AdapterKind: adapter, AdapterVersion: adapterVersion, Locator: "source:" + snapshotWireID(identity.SourceConnection, source), ContentDigest: domain.SnapshotDigest(struct{ Code string }{code}), ObservedAt: now, Findings: []domain.Finding{{Code: code, Severity: "error", Terminal: true}}}
	var pinned struct {
		ArtifactPaths []string `json:"artifactPaths"`
	}
	if err = json.Unmarshal(config, &pinned); err != nil {
		return source, snapshot, err
	}
	if kind == "postgresql" {
		snapshot.DeclareCoverage("postgresql_catalog", "readable_non_system_schemas")
		if len(pinned.ArtifactPaths) > 0 {
			sql := domain.Snapshot{AdapterKind: "postgresql_sql", AdapterVersion: "1.0.0", Findings: snapshot.Findings}
			for _, path := range pinned.ArtifactPaths {
				sql.DeclareCoverage(domain.PathCoverageKey("sql", path), path)
			}
			snapshot.Coverage = append(snapshot.Coverage, sql.Coverage...)
			snapshot.AdapterVersion += "+sql-1.0.0"
		}
	} else {
		rows, readErr := tx.Query(ctx, `SELECT p.logical_path,a.artifact_kind FROM discovery_run_artifacts p JOIN source_artifacts a ON a.workspace_id=p.workspace_id AND a.id=p.source_artifact_id WHERE p.workspace_id=$1 AND p.discovery_run_id=$2 ORDER BY p.logical_path`, workspace, run)
		if readErr != nil {
			return source, snapshot, readErr
		}
		paths := []string{}
		artifactKind := ""
		for rows.Next() {
			var path string
			if err = rows.Scan(&path, &artifactKind); err != nil {
				rows.Close()
				return source, snapshot, err
			}
			paths = append(paths, path)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return source, snapshot, err
		}
		rows.Close()
		switch kind {
		case "sql_bundle":
			for _, path := range paths {
				snapshot.DeclareCoverage(domain.PathCoverageKey("sql", path), path)
			}
		case "dbt_bundle":
			snapshot.DeclareCoverage("dbt", strings.Join(paths, ";"))
		case "file":
			snapshot.AdapterKind = "file_" + artifactKind
			for _, path := range paths {
				snapshot.DeclareCoverage(domain.PathCoverageKey("file", path), path)
			}
		}
	}
	if err = snapshot.Canonicalize(); err != nil {
		return source, snapshot, err
	}
	runID, _ := identity.RunIDFromUUIDBytes(run.Bytes)
	if err = prepareSourceSnapshot(ctx, tx, workspace, source, &runID, &snapshot); err != nil {
		return source, snapshot, err
	}
	return source, snapshot, nil
}

func recordSourceSnapshotAttempt(ctx context.Context, tx pgx.Tx, workspace, run pgtype.UUID, now time.Time, status, code string, terminal bool) error {
	source, snapshot, err := declaredRunSnapshot(ctx, tx, workspace, run, now, code)
	if err != nil {
		return err
	}
	if terminal {
		revision, err := createDiscoverySourceRevision(ctx, dbgen.New(tx), workspace, source, snapshot)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE discovery_runs SET source_revision_id=$3,adapter_version=$4 WHERE workspace_id=$1 AND id=$2`, workspace, run, revision.ID, snapshot.AdapterVersion); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO source_revision_artifact_sets(workspace_id,source_connection_id,source_revision_id,artifact_set_id) SELECT workspace_id,source_connection_id,$3,artifact_set_id FROM discovery_runs WHERE workspace_id=$1 AND id=$2 AND artifact_set_id IS NOT NULL ON CONFLICT DO NOTHING`, workspace, run, revision.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO source_revision_artifacts(workspace_id,source_connection_id,source_revision_id,source_artifact_id,artifact_set_id,logical_path,ordinal,content_sha256) SELECT workspace_id,source_connection_id,$3,source_artifact_id,artifact_set_id,logical_path,ordinal,content_sha256 FROM discovery_run_artifacts WHERE workspace_id=$1 AND discovery_run_id=$2 ON CONFLICT DO NOTHING`, workspace, run, revision.ID); err != nil {
			return err
		}
		_, err = persistSourceHistory(ctx, tx, workspace, source, revision.ID, run, snapshot, snapshot.Findings, nil, nil, nil)
		return err
	}
	for _, unit := range snapshot.Coverage {
		_, err = tx.Exec(ctx, `INSERT INTO source_coverage_heads(workspace_id,source_connection_id,coverage_key,selector_digest,latest_attempt_run_id,latest_attempt_status,version) VALUES($1,$2,$3,$4,$5,$6,1) ON CONFLICT(workspace_id,source_connection_id,coverage_key,selector_digest) DO UPDATE SET latest_attempt_run_id=EXCLUDED.latest_attempt_run_id,latest_attempt_status=EXCLUDED.latest_attempt_status,version=source_coverage_heads.version+1 WHERE (SELECT (created_at,id) FROM discovery_runs WHERE workspace_id=$1 AND id=$5)>=(SELECT (created_at,id) FROM discovery_runs WHERE workspace_id=$1 AND id=source_coverage_heads.latest_attempt_run_id)`, workspace, source, unit.Key, domain.CoverageSelectorDigest(unit), run, status)
		if err != nil {
			return err
		}
	}
	return nil
}

func snapshotUUID(prefix identity.Prefix) (pgtype.UUID, error) {
	id, err := identity.New(prefix)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return uuidValue(id)
}
func snapshotWireID(prefix identity.Prefix, id pgtype.UUID) string {
	value, _ := identity.FromUUIDBytes(prefix, id.Bytes)
	return value.String()
}

func persistSourceHistory(ctx context.Context, tx pgx.Tx, workspace, source, revision, run pgtype.UUID, snapshot domain.Snapshot, findings []domain.Finding, reused *dbgen.DiscoveryRun, members []domain.SnapshotMember, datasets map[string]domain.SnapshotMember) (string, error) {
	if reused != nil {
		var id pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT snapshot_id FROM source_snapshot_runs WHERE workspace_id=$1 AND run_id=$2`, workspace, reused.ID).Scan(&id); err != nil {
			return "", sourceRepositoryError("read reused source snapshot", err)
		}
		if err := linkSourceSnapshotRun(ctx, tx, workspace, source, run, id); err != nil {
			return "", err
		}
		if err := advanceSourceSnapshotHeads(ctx, tx, workspace, source, run, id); err != nil {
			return "", err
		}
		return snapshotWireID(identity.SourceSnapshot, id), nil
	}
	codeRevisions := map[string]pgtype.UUID{}
	for _, code := range snapshot.CodeArtifacts {
		if len(code.Content) == 0 {
			findings = append(findings, domain.Finding{Code: "CODE_BYTES_UNAVAILABLE", Severity: "error", CoverageKey: code.CoverageKey})
			continue
		}
		bytesHash := sha256.Sum256(code.Content)
		blob := "sha256:" + hex.EncodeToString(bytesHash[:])
		if blob != code.ContentDigest {
			return "", domain.ErrInvalidSnapshot
		}
		digest := domain.SnapshotDigest(struct{ Schema, Adapter, Path, Language, Blob string }{"semlia.code-revision/v1", snapshot.AdapterVersion, code.Path, code.Language, blob})
		var object pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM code_artifacts WHERE workspace_id=$1 AND source_revision_id=$2 AND path=$3`, workspace, revision, code.Path).Scan(&object); err != nil {
			return "", err
		}
		newID, err := snapshotUUID(identity.SourceCodeRevision)
		if err != nil {
			return "", err
		}
		_, err = tx.Exec(ctx, `INSERT INTO source_code_revisions(id,workspace_id,code_artifact_id,source_revision_id,adapter_version,path,language,blob_oid,content_bytes,content_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(workspace_id,code_artifact_id,adapter_version,content_digest) DO NOTHING`, newID, workspace, object, revision, snapshot.AdapterVersion, code.Path, code.Language, blob, code.Content, digest)
		if err != nil {
			return "", sourceRepositoryError("retain immutable code bytes", err)
		}
		var codeID pgtype.UUID
		if err = tx.QueryRow(ctx, `SELECT id FROM source_code_revisions WHERE workspace_id=$1 AND code_artifact_id=$2 AND adapter_version=$3 AND content_digest=$4`, workspace, object, snapshot.AdapterVersion, digest).Scan(&codeID); err != nil {
			return "", err
		}
		codeRevisions[code.Path] = codeID
		members = append(members, domain.SnapshotMember{Kind: "code", ObjectID: snapshotWireID(identity.CodeArtifact, object), RevisionID: snapshotWireID(identity.SourceCodeRevision, codeID), Name: code.Path, Locator: code.Path, ContentDigest: digest, CoverageKey: code.CoverageKey})
	}
	for _, edge := range snapshot.Lineage {
		up, upOK := datasets[edge.UpstreamExternalKey]
		down, downOK := datasets[edge.DownstreamExternalKey]
		codeID := codeRevisions[edge.CodePath]
		if !upOK || !downOK || (edge.CodePath != "" && !codeID.Valid) {
			findings = append(findings, domain.Finding{Code: "UNRESOLVED_LINEAGE", Severity: "error", CoverageKey: edge.CoverageKey})
			continue
		}
		upID, _ := identity.ParseAny(up.ObjectID)
		upRev, _ := identity.ParseAny(up.RevisionID)
		downID, _ := identity.ParseAny(down.ObjectID)
		downRev, _ := identity.ParseAny(down.RevisionID)
		var object pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM lineage_edges WHERE workspace_id=$1 AND source_revision_id=$2 AND upstream_dataset_id=$3 AND downstream_dataset_id=$4 AND edge_kind=$5`, workspace, revision, upID.UUID(), downID.UUID(), edge.Kind).Scan(&object); err != nil {
			return "", err
		}
		digest := domain.SnapshotDigest(struct {
			Schema, Adapter, Upstream, UpstreamRevision, Downstream, DownstreamRevision, Kind, Code string
			Confidence                                                                              float64
		}{"semlia.lineage-revision/v1", snapshot.AdapterVersion, up.ObjectID, up.RevisionID, down.ObjectID, down.RevisionID, edge.Kind, snapshotWireID(identity.SourceCodeRevision, codeID), edge.Confidence})
		newID, err := snapshotUUID(identity.SourceLineageRevision)
		if err != nil {
			return "", err
		}
		_, err = tx.Exec(ctx, `INSERT INTO source_lineage_revisions(id,workspace_id,lineage_edge_id,source_revision_id,adapter_version,upstream_object_id,upstream_revision_id,downstream_object_id,downstream_revision_id,edge_kind,code_revision_id,confidence,content_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT(workspace_id,lineage_edge_id,adapter_version,content_digest) DO NOTHING`, newID, workspace, object, revision, snapshot.AdapterVersion, upID.UUID(), upRev.UUID(), downID.UUID(), downRev.UUID(), edge.Kind, codeID, numeric(edge.Confidence), digest)
		if err != nil {
			return "", sourceRepositoryError("retain precise lineage revision", err)
		}
		var edgeID pgtype.UUID
		if err = tx.QueryRow(ctx, `SELECT id FROM source_lineage_revisions WHERE workspace_id=$1 AND lineage_edge_id=$2 AND adapter_version=$3 AND content_digest=$4`, workspace, object, snapshot.AdapterVersion, digest).Scan(&edgeID); err != nil {
			return "", err
		}
		members = append(members, domain.SnapshotMember{Kind: "lineage", ObjectID: snapshotWireID(identity.LineageEdge, object), RevisionID: snapshotWireID(identity.SourceLineageRevision, edgeID), Name: edge.Kind, Locator: up.Name + " -> " + down.Name, ContentDigest: digest, CoverageKey: edge.CoverageKey})
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Kind+members[i].ObjectID < members[j].Kind+members[j].ObjectID })
	// Normalize diagnostics without retaining untrusted raw errors or adapter details.
	diagnostics := domain.SnapshotDiagnostics(findings, snapshot.Coverage)
	quality, status := snapshot.CoverageState()
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "blocker" {
			for i := range snapshot.Coverage {
				if snapshot.Coverage[i].Key == diagnostic.CoverageKey && snapshot.Coverage[i].Status == "complete" {
					snapshot.Coverage[i].Status = "partial"
					snapshot.Coverage[i].EnumerationComplete = false
				}
			}
		}
	}
	_, status = snapshot.CoverageState()
	digest := domain.SnapshotDigest(struct {
		Schema, Adapter, Scope, Quality, Status string
		Coverage                                []domain.CoverageUnit
		Members                                 []domain.SnapshotMember
		Diagnostics                             []domain.SnapshotDiagnostic
	}{"semlia.source-snapshot/v1", snapshot.AdapterVersion, snapshot.ScopeDigest(), quality, status, snapshot.Coverage, members, diagnostics})
	id, err := snapshotUUID(identity.SourceSnapshot)
	if err != nil {
		return "", err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO source_snapshots(id,workspace_id,source_connection_id,source_revision_id,adapter_version,scope_digest,content_digest,history_quality,coverage_status,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,clock_timestamp()) ON CONFLICT(workspace_id,source_connection_id,source_revision_id,adapter_version,scope_digest,content_digest) DO NOTHING`, id, workspace, source, revision, snapshot.AdapterVersion, snapshot.ScopeDigest(), digest, quality, status)
	if err != nil {
		return "", sourceRepositoryError("create immutable source snapshot", err)
	}
	if tag.RowsAffected() == 0 {
		if err = tx.QueryRow(ctx, `SELECT id FROM source_snapshots WHERE workspace_id=$1 AND source_connection_id=$2 AND source_revision_id=$3 AND adapter_version=$4 AND scope_digest=$5 AND content_digest=$6`, workspace, source, revision, snapshot.AdapterVersion, snapshot.ScopeDigest(), digest).Scan(&id); err != nil {
			return "", err
		}
	} else {
		for _, unit := range snapshot.Coverage {
			if _, err = tx.Exec(ctx, `INSERT INTO source_snapshot_scope(workspace_id,snapshot_id,coverage_key,selector,config_digest,status,enumeration_complete,diagnostic_codes) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, workspace, id, unit.Key, unit.Selector, unit.ConfigDigest, unit.Status, unit.EnumerationComplete, unit.DiagnosticCodes); err != nil {
				return "", err
			}
		}
		for _, member := range members {
			object, _ := identity.ParseAny(member.ObjectID)
			revision, _ := identity.ParseAny(member.RevisionID)
			var parent, parentRev any
			if member.ParentObjectID != "" {
				p, _ := identity.ParseAny(member.ParentObjectID)
				r, _ := identity.ParseAny(member.ParentRevisionID)
				parent, parentRev = p.UUID(), r.UUID()
			}
			if _, err = tx.Exec(ctx, `INSERT INTO source_snapshot_members(workspace_id,snapshot_id,kind,object_id,revision_id,historical_name,historical_locator,content_digest,coverage_key,parent_object_id,parent_revision_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, workspace, id, member.Kind, object.UUID(), revision.UUID(), member.Name, member.Locator, member.ContentDigest, member.CoverageKey, parent, parentRev); err != nil {
				return "", err
			}
		}
		for _, d := range diagnostics {
			if _, err = tx.Exec(ctx, `INSERT INTO source_snapshot_diagnostics(workspace_id,snapshot_id,ordinal,code,severity,coverage_key,locator,message) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, workspace, id, d.Ordinal, d.Code, d.Severity, d.CoverageKey, d.Locator, d.Message); err != nil {
				return "", err
			}
		}
	}
	if err = linkSourceSnapshotRun(ctx, tx, workspace, source, run, id); err != nil {
		return "", err
	}
	if err = advanceSourceSnapshotHeads(ctx, tx, workspace, source, run, id); err != nil {
		return "", err
	}
	return snapshotWireID(identity.SourceSnapshot, id), nil
}

func linkSourceSnapshotRun(ctx context.Context, tx pgx.Tx, workspace, source, run, snapshot pgtype.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO source_snapshot_runs(workspace_id,source_connection_id,run_id,snapshot_id) VALUES($1,$2,$3,$4) ON CONFLICT(workspace_id,run_id) DO NOTHING`, workspace, source, run, snapshot)
	return sourceRepositoryError("link source snapshot run", err)
}

func advanceSourceSnapshotHeads(ctx context.Context, tx pgx.Tx, workspace, source, run, id pgtype.UUID) error {
	var quality, status, scopeDigest string
	if err := tx.QueryRow(ctx, `SELECT history_quality,coverage_status,scope_digest FROM source_snapshots WHERE workspace_id=$1 AND id=$2`, workspace, id).Scan(&quality, &status, &scopeDigest); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT coverage_key,selector,config_digest,status,enumeration_complete FROM source_snapshot_scope WHERE workspace_id=$1 AND snapshot_id=$2 ORDER BY coverage_key`, workspace, id)
	if err != nil {
		return err
	}
	units := []domain.CoverageUnit{}
	for rows.Next() {
		var unit domain.CoverageUnit
		if err = rows.Scan(&unit.Key, &unit.Selector, &unit.ConfigDigest, &unit.Status, &unit.EnumerationComplete); err != nil {
			rows.Close()
			return err
		}
		units = append(units, unit)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	allCurrent := true
	for _, unit := range units {
		var verified any
		if quality == "verified" && unit.Status == "complete" && unit.EnumerationComplete {
			verified = id
		}
		selector := domain.CoverageSelectorDigest(unit)
		tag, err := tx.Exec(ctx, `INSERT INTO source_coverage_heads(workspace_id,source_connection_id,coverage_key,selector_digest,latest_attempt_run_id,latest_attempt_status,latest_verified_snapshot_id,version) VALUES($1,$2,$3,$4,$5,$6,$7,1)
ON CONFLICT(workspace_id,source_connection_id,coverage_key,selector_digest) DO UPDATE SET latest_attempt_run_id=EXCLUDED.latest_attempt_run_id,latest_attempt_status=EXCLUDED.latest_attempt_status,latest_verified_snapshot_id=COALESCE(EXCLUDED.latest_verified_snapshot_id,source_coverage_heads.latest_verified_snapshot_id),version=source_coverage_heads.version+1
WHERE (SELECT (CASE WHEN job_id IS NULL THEN started_at ELSE created_at END,id) FROM discovery_runs WHERE workspace_id=$1 AND id=$5) >= (SELECT (CASE WHEN job_id IS NULL THEN started_at ELSE created_at END,id) FROM discovery_runs WHERE workspace_id=$1 AND id=source_coverage_heads.latest_attempt_run_id)`, workspace, source, unit.Key, selector, run, unit.Status, verified)
		if err != nil {
			return sourceRepositoryError("advance coverage attempt", err)
		}
		if tag.RowsAffected() == 0 {
			allCurrent = false
		}
	}
	if quality == "verified" && status == "complete" && allCurrent {
		_, err := tx.Exec(ctx, `INSERT INTO source_effective_snapshots(workspace_id,source_connection_id,scope_digest,snapshot_id,version) VALUES($1,$2,$3,$4,1) ON CONFLICT(workspace_id,source_connection_id,scope_digest) DO UPDATE SET snapshot_id=EXCLUDED.snapshot_id,version=source_effective_snapshots.version+1 WHERE source_effective_snapshots.snapshot_id<>EXCLUDED.snapshot_id`, workspace, source, scopeDigest, id)
		if err != nil {
			return sourceRepositoryError("advance effective snapshot", err)
		}
	}
	return nil
}

var _ application.SnapshotRepository = (*Store)(nil)

func (store *Store) LookupSourceSnapshot(ctx context.Context, workspace identity.WorkspaceID, source identity.SourceConnectionID, snapshot string) error {
	var exists bool
	if snapshot == "" {
		if err := store.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_connections WHERE workspace_id=$1 AND id=$2)`, workspace.UUID(), source.UUID()).Scan(&exists); err != nil {
			return err
		}
	} else {
		id, err := identity.Parse(identity.SourceSnapshot, snapshot)
		if err != nil {
			return domain.ErrInvalidInput
		}
		if err := store.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_snapshots WHERE workspace_id=$1 AND source_connection_id=$2 AND id=$3)`, workspace.UUID(), source.UUID(), id.UUID()).Scan(&exists); err != nil {
			return err
		}
	}
	if !exists {
		return domain.ErrNotFound
	}
	return nil
}

const sourceSnapshotColumns = `s.id,s.source_connection_id,s.source_revision_id,s.adapter_version,s.scope_digest,s.content_digest,s.history_quality,s.coverage_status,s.created_at,
(SELECT count(*) FROM source_snapshot_members m WHERE m.workspace_id=s.workspace_id AND m.snapshot_id=s.id),
(SELECT count(*) FROM source_snapshot_diagnostics d WHERE d.workspace_id=s.workspace_id AND d.snapshot_id=s.id),
(SELECT COALESCE(jsonb_agg(jsonb_build_object('key',c.coverage_key,'selector',c.selector,'status',c.status,'enumerationComplete',c.enumeration_complete,'diagnosticCodes',c.diagnostic_codes) ORDER BY c.coverage_key),'[]'::jsonb) FROM source_snapshot_scope c WHERE c.workspace_id=s.workspace_id AND c.snapshot_id=s.id)`

func scanSourceSnapshot(row rowScanner) (domain.SourceSnapshot, error) {
	var value domain.SourceSnapshot
	var id, source, revision pgtype.UUID
	var coverage []byte
	if err := row.Scan(&id, &source, &revision, &value.AdapterVersion, &value.ScopeDigest, &value.ContentDigest, &value.HistoryQuality, &value.CoverageStatus, &value.CreatedAt, &value.MemberCount, &value.DiagnosticCount, &coverage); err != nil {
		return value, err
	}
	value.ID = snapshotWireID(identity.SourceSnapshot, id)
	value.SourceID = snapshotWireID(identity.SourceConnection, source)
	value.SourceRevisionID = snapshotWireID(identity.SourceRevision, revision)
	if err := json.Unmarshal(coverage, &value.Coverage); err != nil {
		return value, err
	}
	return value, nil
}

func (store *Store) ReadSourceSnapshot(ctx context.Context, query application.SnapshotReadQuery) (application.SnapshotReadResult, error) {
	result := application.SnapshotReadResult{Snapshots: []domain.SourceSnapshot{}, Members: []domain.SnapshotMember{}, Diagnostics: []domain.SnapshotDiagnostic{}, Cursor: query.Cursor}
	if query.Limit < 1 || query.Limit > 200 {
		return result, domain.ErrInvalidInput
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The workspace row lock linearizes the capability decision with every page read.
	var authVersion int64
	if err = tx.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1 FOR SHARE`, query.WorkspaceID.UUID()).Scan(&authVersion); err != nil {
		return result, sourceRepositoryError("authorize source snapshot read", err)
	}
	if query.AuthorizationVersion != authVersion {
		return result, domain.ErrConflict
	}
	workspace, source := query.WorkspaceID.UUID(), query.SourceID.UUID()
	var sourceLock pgtype.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM source_connections WHERE workspace_id=$1 AND id=$2 FOR SHARE`, workspace, source).Scan(&sourceLock); err != nil {
		return result, sourceRepositoryError("lock snapshot read scope", err)
	}
	var snapshotID string
	if query.SnapshotID != "" {
		id, parseErr := identity.Parse(identity.SourceSnapshot, query.SnapshotID)
		if parseErr != nil {
			return result, domain.ErrInvalidInput
		}
		snapshotID = id.UUID()
		if err = tx.QueryRow(ctx, `SELECT history_quality FROM source_snapshots WHERE workspace_id=$1 AND source_connection_id=$2 AND id=$3`, workspace, source, snapshotID).Scan(&result.HistoryQuality); err != nil {
			return result, sourceRepositoryError("read source snapshot relation", err)
		}
	} else {
		var id pgtype.UUID
		if err = tx.QueryRow(ctx, `SELECT id FROM source_connections WHERE workspace_id=$1 AND id=$2`, workspace, source).Scan(&id); err != nil {
			return result, sourceRepositoryError("read source relation", err)
		}
	}
	switch query.Kind {
	case "list", "detail":
		if query.Kind == "detail" {
			value, readErr := scanSourceSnapshot(tx.QueryRow(ctx, `SELECT `+sourceSnapshotColumns+` FROM source_snapshots s WHERE s.workspace_id=$1 AND s.source_connection_id=$2 AND s.id=$3`, workspace, source, snapshotID))
			if readErr != nil {
				return result, sourceRepositoryError("read immutable snapshot", readErr)
			}
			result.Snapshots = append(result.Snapshots, value)
			break
		}
		if result.Cursor.UpperKey == "" {
			err = tx.QueryRow(ctx, `SELECT created_at,id::text FROM source_snapshots WHERE workspace_id=$1 AND source_connection_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1`, workspace, source).Scan(&result.Cursor.UpperTime, &result.Cursor.UpperKey)
			if errors.Is(err, pgx.ErrNoRows) {
				return result, nil
			}
			if err != nil {
				return result, err
			}
		}
		var lastTime any
		var lastID any
		if result.Cursor.LastKey != "" {
			lastTime = result.Cursor.LastTime
			lastID = result.Cursor.LastKey
		}
		rows, readErr := tx.Query(ctx, `SELECT `+sourceSnapshotColumns+` FROM source_snapshots s WHERE s.workspace_id=$1 AND s.source_connection_id=$2 AND (s.created_at,s.id)<=($3,$4::uuid) AND ($5::timestamptz IS NULL OR (s.created_at,s.id)<($5,$6::uuid)) ORDER BY s.created_at DESC,s.id DESC LIMIT $7`, workspace, source, result.Cursor.UpperTime, result.Cursor.UpperKey, lastTime, lastID, query.Limit+1)
		if readErr != nil {
			return result, readErr
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanSourceSnapshot(rows)
			if err != nil {
				return result, err
			}
			result.Snapshots = append(result.Snapshots, value)
		}
		if err = rows.Err(); err != nil {
			return result, err
		}
		rows.Close()
	case "members":
		if result.Cursor.UpperKey == "" {
			err = tx.QueryRow(ctx, `SELECT kind||':'||object_id::text FROM source_snapshot_members WHERE workspace_id=$1 AND snapshot_id=$2 AND ($3='' OR kind=$3) ORDER BY kind DESC,object_id DESC LIMIT 1`, workspace, snapshotID, query.Filter).Scan(&result.Cursor.UpperKey)
			if errors.Is(err, pgx.ErrNoRows) {
				return result, nil
			}
			if err != nil {
				return result, err
			}
		}
		upperKind, upperID, ok := strings.Cut(result.Cursor.UpperKey, ":")
		if !ok {
			return result, application.ErrSnapshotCursor
		}
		lastKind, lastID := "", "00000000-0000-0000-0000-000000000000"
		if result.Cursor.LastKey != "" {
			lastKind, lastID, ok = strings.Cut(result.Cursor.LastKey, ":")
			if !ok {
				return result, application.ErrSnapshotCursor
			}
		}
		rows, readErr := tx.Query(ctx, `SELECT kind,object_id,revision_id,historical_name,historical_locator,content_digest,coverage_key,parent_object_id,parent_revision_id FROM source_snapshot_members WHERE workspace_id=$1 AND snapshot_id=$2 AND ($3='' OR kind=$3) AND (kind,object_id)<=($4,$5::uuid) AND (kind,object_id)>($6,$7::uuid) ORDER BY kind,object_id LIMIT $8`, workspace, snapshotID, query.Filter, upperKind, upperID, lastKind, lastID, query.Limit+1)
		if readErr != nil {
			return result, readErr
		}
		defer rows.Close()
		for rows.Next() {
			var member domain.SnapshotMember
			var object, rev, parent, parentRev pgtype.UUID
			if err = rows.Scan(&member.Kind, &object, &rev, &member.Name, &member.Locator, &member.ContentDigest, &member.CoverageKey, &parent, &parentRev); err != nil {
				return result, err
			}
			prefixes := map[string][2]identity.Prefix{"dataset": {identity.PhysicalDataset, identity.PhysicalDatasetRevision}, "field": {identity.PhysicalField, identity.PhysicalFieldRevision}, "code": {identity.CodeArtifact, identity.SourceCodeRevision}, "lineage": {identity.LineageEdge, identity.SourceLineageRevision}}
			prefix := prefixes[member.Kind]
			member.ObjectID = snapshotWireID(prefix[0], object)
			member.RevisionID = snapshotWireID(prefix[1], rev)
			if parent.Valid {
				member.ParentObjectID = snapshotWireID(identity.PhysicalDataset, parent)
				member.ParentRevisionID = snapshotWireID(identity.PhysicalDatasetRevision, parentRev)
			}
			result.Members = append(result.Members, member)
		}
		if err = rows.Err(); err != nil {
			return result, err
		}
		rows.Close()
	case "diagnostics":
		if result.Cursor.UpperKey == "" {
			var upper int
			err = tx.QueryRow(ctx, `SELECT COALESCE(max(ordinal),0) FROM source_snapshot_diagnostics WHERE workspace_id=$1 AND snapshot_id=$2`, workspace, snapshotID).Scan(&upper)
			if err != nil {
				return result, err
			}
			result.Cursor.UpperKey = strconv.Itoa(upper)
		}
		upper, parseErr := strconv.Atoi(result.Cursor.UpperKey)
		if parseErr != nil {
			return result, application.ErrSnapshotCursor
		}
		last := 0
		if result.Cursor.LastKey != "" {
			last, parseErr = strconv.Atoi(result.Cursor.LastKey)
			if parseErr != nil {
				return result, application.ErrSnapshotCursor
			}
		}
		rows, readErr := tx.Query(ctx, `SELECT ordinal,code,severity,coverage_key,locator,message FROM source_snapshot_diagnostics WHERE workspace_id=$1 AND snapshot_id=$2 AND ordinal<=$3 AND ordinal>$4 ORDER BY ordinal LIMIT $5`, workspace, snapshotID, upper, last, query.Limit+1)
		if readErr != nil {
			return result, readErr
		}
		defer rows.Close()
		for rows.Next() {
			var diagnostic domain.SnapshotDiagnostic
			if err = rows.Scan(&diagnostic.Ordinal, &diagnostic.Code, &diagnostic.Severity, &diagnostic.CoverageKey, &diagnostic.Locator, &diagnostic.Message); err != nil {
				return result, err
			}
			result.Diagnostics = append(result.Diagnostics, diagnostic)
		}
		if err = rows.Err(); err != nil {
			return result, err
		}
		rows.Close()
	default:
		return result, domain.ErrInvalidInput
	}
	if err = tx.Commit(ctx); err != nil {
		return result, sourceRepositoryError("commit source snapshot read", err)
	}
	return result, nil
}
