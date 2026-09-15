package postgres

import (
	"context"
	"encoding/json"
	"time"

	authapp "github.com/iiwish/semlia/internal/application/authorization"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type productionInput struct {
	Snapshots []struct {
		SourceID     string   `json:"sourceId"`
		SnapshotID   string   `json:"snapshotId"`
		Digest       string   `json:"digest"`
		CoverageKeys []string `json:"coverageKeys"`
	} `json:"snapshots"`
	Candidates []struct {
		CandidateID string `json:"candidateId"`
		SnapshotID  string `json:"snapshotId"`
		Digest      string `json:"digest"`
	} `json:"candidates"`
	Evidence []struct {
		EvidenceID string `json:"evidenceId"`
		SnapshotID string `json:"snapshotId"`
		Digest     string `json:"digest"`
	} `json:"evidence"`
	Dependencies []json.RawMessage `json:"dependencies"`
}

func (s *Store) CheckProductionInput(ctx context.Context, version domain.ProductionVersion, targets []domain.ProductionTarget) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.validateProductionInputTx(ctx, tx, version, targets, false); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func productionRequire(access authapp.AccessSnapshot, action authz.Action, resource authz.Resource) error {
	if !access.Allows(action, resource) {
		return &authz.DenialError{Decision: authz.Decision{ReasonCode: authz.ReasonNoMatchingGrant}}
	}
	return nil
}

func (s *Store) productionAccessTx(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, principal identity.PrincipalID) (authapp.AccessSnapshot, error) {
	// Reuse the connection holding the workspace lock. Borrowing another pool
	// connection here can deadlock when other transactions wait for that lock.
	store := &Store{queries: s.queries.WithTx(tx)}
	return authapp.NewService(store, authapp.ClockFunc(time.Now)).Snapshot(ctx, workspace, principal)
}

// Authorization mutations serialize on the workspace row. Source ingestion
// uses the same boundary, keeping selected coverage heads stable until commit.
func (s *Store) validateProductionInputTx(ctx context.Context, tx pgx.Tx, version domain.ProductionVersion, targets []domain.ProductionTarget, fresh bool, existingVersion ...bool) error {
	w := version.WorkspaceID
	var authVersion int64
	if err := tx.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1 FOR UPDATE`, w.UUID()).Scan(&authVersion); err != nil {
		return governanceRepositoryError("lock production workspace", err)
	}
	access, err := s.productionAccessTx(ctx, tx, w, version.CreatedBy)
	if err != nil {
		return err
	}
	if access.AuthorizationVersion != authVersion {
		return domain.ErrConflict
	}
	workspace := authz.Resource{Type: authz.ScopeWorkspace, ID: w.UUID()}
	authorizeWrite := fresh
	if len(existingVersion) > 1 && !existingVersion[1] {
		authorizeWrite = false
	}
	if err := authorizeProductionTargets(ctx, tx, access, w, targets, authorizeWrite, version.BaselineJSON); err != nil {
		return err
	}
	var wrapper struct {
		Scope json.RawMessage `json:"scope"`
	}
	if err := json.Unmarshal(version.InputJSON, &wrapper); err != nil {
		return domain.ErrInvalidArgument
	}
	data := version.InputJSON
	if len(wrapper.Scope) > 0 {
		data = wrapper.Scope
	}
	var input productionInput
	if err := json.Unmarshal(data, &input); err != nil {
		return domain.ErrInvalidArgument
	}
	if len(input.Snapshots) == 0 {
		return domain.ErrInputIncomplete
	}
	if len(input.Snapshots) > 32 || len(input.Candidates) > 256 || len(input.Evidence) > 256 || len(input.Dependencies) > 256 {
		return domain.ErrLimitExceeded
	}
	if err := validateProductionDependencies(ctx, tx, access, version, targets, input.Dependencies, fresh); err != nil {
		return err
	}
	snapshots := map[string]productionSnapshotPin{}
	selectedUnits := map[string]string{}
	for _, selected := range input.Snapshots {
		id, err := identity.ParseSourceSnapshotID(selected.SnapshotID)
		if err != nil || !domain.IsValidContentDigest(selected.Digest) {
			return domain.ErrInvalidArgument
		}
		if _, exists := snapshots[id.String()]; exists {
			return domain.ErrInvalidArgument
		}
		pin := productionSnapshotPin{coverage: map[string]bool{}}
		var digest, quality string
		if err := tx.QueryRow(ctx, `SELECT source_connection_id,source_revision_id,content_digest,history_quality FROM source_snapshots WHERE workspace_id=$1 AND id=$2`, w.UUID(), id.UUID()).Scan(&pin.source, &pin.revision, &digest, &quality); err != nil {
			return governanceRepositoryError("load production snapshot", err)
		}
		sourceID, err := identity.ParseSourceConnectionID(selected.SourceID)
		if err != nil || sourceID.UUID() != formatUUID(pin.source) {
			return domain.ErrInvalidArgument
		}
		if err := productionRequire(access, authz.ActionSourceRead, authz.Resource{Type: authz.ScopeSource, ID: formatUUID(pin.source)}); err != nil {
			return err
		}
		if digest != selected.Digest {
			return domain.ErrInputStale
		}
		if quality != "verified" || len(selected.CoverageKeys) == 0 {
			return domain.ErrInputIncomplete
		}
		seen := map[string]bool{}
		for _, key := range selected.CoverageKeys {
			if seen[key] {
				return domain.ErrInvalidArgument
			}
			seen[key] = true
			unit := formatUUID(pin.source) + "/" + key
			if prior, exists := selectedUnits[unit]; exists && prior != id.String() {
				return domain.ErrInputStale
			}
			selectedUnits[unit] = id.String()
			pin.coverage[key] = true
			var status, selector string
			var complete bool
			if err := tx.QueryRow(ctx, `SELECT status,enumeration_complete,selector FROM source_snapshot_scope WHERE workspace_id=$1 AND snapshot_id=$2 AND coverage_key=$3`, w.UUID(), id.UUID(), key).Scan(&status, &complete, &selector); err != nil {
				return governanceRepositoryError("load production coverage", err)
			}
			if status != "complete" || !complete {
				return domain.ErrInputIncomplete
			}
			if fresh {
				var head pgtype.UUID
				if err := tx.QueryRow(ctx, `SELECT latest_attempt_status,latest_verified_snapshot_id FROM source_coverage_heads WHERE workspace_id=$1 AND source_connection_id=$2 AND coverage_key=$3 AND selector_digest='sha256:'||encode(sha256(convert_to($4::text,'UTF8')),'hex')`, w.UUID(), pin.source, key, selector).Scan(&status, &head); err != nil {
					return governanceRepositoryError("load production coverage head", err)
				}
				if status != "complete" || formatUUID(head) != id.UUID() {
					return domain.ErrInputStale
				}
			}
		}
		snapshots[id.String()] = pin
	}
	var competing bool
	selectedRows := make([]map[string]any, 0, len(input.Snapshots))
	for _, selected := range input.Snapshots {
		selectedRows = append(selectedRows, map[string]any{"snapshot_id": mustSnapshotUUID(selected.SnapshotID), "coverage_keys": selected.CoverageKeys})
	}
	selectedJSON, err := json.Marshal(selectedRows)
	if err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_snapshot_members m
	 JOIN jsonb_to_recordset($2::jsonb) AS selected(snapshot_id uuid,coverage_keys jsonb)
	 ON selected.snapshot_id=m.snapshot_id AND selected.coverage_keys ? m.coverage_key
	 WHERE m.workspace_id=$1 GROUP BY m.kind,m.object_id
	 HAVING count(DISTINCT m.revision_id)>1 OR count(DISTINCT COALESCE(m.parent_revision_id::text,''))>1)`, w.UUID(), selectedJSON).Scan(&competing); err != nil {
		return err
	}
	if competing {
		return domain.ErrInputStale
	}
	evidence := map[string]bool{}
	for _, selected := range input.Evidence {
		id, err := identity.ParseEvidenceID(selected.EvidenceID)
		if err != nil || !domain.IsValidContentDigest(selected.Digest) || evidence[selected.EvidenceID] {
			return domain.ErrInvalidArgument
		}
		evidence[selected.EvidenceID] = true
		var revision pgtype.UUID
		var digest string
		if err := tx.QueryRow(ctx, `SELECT source_revision_id,content_digest FROM evidence_artifacts WHERE workspace_id=$1 AND id=$2`, w.UUID(), id.UUID()).Scan(&revision, &digest); err != nil {
			return governanceRepositoryError("load production evidence", err)
		}
		if digest != selected.Digest {
			return domain.ErrEvidenceMissing
		}
		resource := workspace
		if revision.Valid {
			pin, ok := snapshots[selected.SnapshotID]
			if !ok || pin.revision != revision {
				return domain.ErrEvidenceMissing
			}
			resource = authz.Resource{Type: authz.ScopeSource, ID: formatUUID(pin.source)}
		}
		if err := productionRequire(access, authz.ActionEvidenceRead, resource); err != nil {
			return err
		}
	}
	if err := validateProductionPhysicalReferences(ctx, tx, w, targets, snapshots, evidence); err != nil {
		return err
	}
	seenCandidates := map[string]bool{}
	for _, selected := range input.Candidates {
		id, err := identity.ParseSemanticCandidateID(selected.CandidateID)
		if err != nil || !domain.IsValidContentDigest(selected.Digest) || seenCandidates[selected.CandidateID] {
			return domain.ErrInvalidArgument
		}
		seenCandidates[selected.CandidateID] = true
		var source, revision, run pgtype.UUID
		var digest, status string
		var candidateEvidence []byte
		if err := tx.QueryRow(ctx, `SELECT source_connection_id,source_revision_id,discovery_run_id,content_digest,status,evidence FROM semantic_candidates WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, w.UUID(), id.UUID()).Scan(&source, &revision, &run, &digest, &status, &candidateEvidence); err != nil {
			return governanceRepositoryError("load production candidate", err)
		}
		pin, ok := snapshots[selected.SnapshotID]
		if !ok || pin.source != source || pin.revision != revision || digest != selected.Digest {
			return domain.ErrInputStale
		}
		var linked bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_snapshot_runs WHERE workspace_id=$1 AND run_id=$2 AND snapshot_id=$3)`, w.UUID(), run, mustSnapshotUUID(selected.SnapshotID)).Scan(&linked); err != nil {
			return err
		}
		if !linked {
			return domain.ErrInputStale
		}
		var facts []struct {
			SourceRevisionID string `json:"sourceRevisionId"`
			DiscoveryRunID   string `json:"discoveryRunId"`
			Locator          string `json:"locator"`
			EvidenceID       string `json:"evidenceId"`
		}
		if err := json.Unmarshal(candidateEvidence, &facts); err != nil || len(facts) == 0 {
			return domain.ErrEvidenceMissing
		}
		coverageKeys := make([]string, 0, len(pin.coverage))
		for key := range pin.coverage {
			coverageKeys = append(coverageKeys, key)
		}
		for _, fact := range facts {
			factRevision, revisionErr := parseUUIDOrTypeID(fact.SourceRevisionID)
			factRun, runErr := parseUUIDOrTypeID(fact.DiscoveryRunID)
			if revisionErr != nil || runErr != nil || factRevision != revision || factRun != run || fact.Locator == "" || (fact.EvidenceID != "" && !evidence[fact.EvidenceID]) {
				return domain.ErrEvidenceMissing
			}
			var covered bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_snapshot_members WHERE workspace_id=$1 AND snapshot_id=$2 AND historical_locator=$3 AND coverage_key=ANY($4::text[]))`, w.UUID(), mustSnapshotUUID(selected.SnapshotID), fact.Locator, coverageKeys).Scan(&covered); err != nil {
				return err
			}
			if !covered {
				return domain.ErrEvidenceMissing
			}
		}
		if fresh && status != "pending" {
			var ownPrevious bool
			previousOperation, previousVersion := productionDecisionPredecessor(version)
			if len(existingVersion) > 0 && existingVersion[0] {
				previousOperation, previousVersion = version.OperationID, version.Version
			}
			if status == "converted" && previousVersion > 0 {
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_candidate_links l
				JOIN semantic_candidate_decisions d ON d.workspace_id=l.workspace_id AND d.id=l.decision_id
				JOIN semantic_candidates c ON c.workspace_id=l.workspace_id AND c.id=l.candidate_id AND c.proposal_id=d.proposal_id
				WHERE l.workspace_id=$1 AND l.operation_id=$2 AND l.version=$3 AND l.candidate_id=$4 AND l.is_primary)`, w.UUID(), previousOperation.UUID(), previousVersion, id.UUID()).Scan(&ownPrevious); err != nil {
					return err
				}
			}
			if !ownPrevious {
				return domain.ErrAlreadyProduced
			}
		}
	}
	return nil
}

func mustSnapshotUUID(value string) string {
	id, err := identity.ParseSourceSnapshotID(value)
	if err != nil {
		return ""
	}
	return id.UUID()
}
