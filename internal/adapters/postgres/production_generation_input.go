package postgres

import (
	"context"
	"encoding/json"
	"errors"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	app "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) productionGenerationVersionTx(ctx context.Context, tx pgx.Tx, r domain.ProductionGenerationRequest, fresh bool) (domain.ProductionVersion, []domain.ProductionTarget, error) {
	var locked pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM workspaces WHERE id=$1 FOR UPDATE`, r.WorkspaceID.UUID()).Scan(&locked); err != nil {
		return domain.ProductionVersion{}, nil, governanceRepositoryError("lock generation workspace", err)
	}
	op, ver, targets, _, _, err := s.getProductionOperationVersionTx(ctx, tx, r.WorkspaceID, r.OperationID, r.ExpectedVersion)
	if err != nil {
		return ver, nil, err
	}
	authorizedVersion := ver
	authorizedVersion.CreatedBy = r.PrincipalID
	if err := s.validateProductionInputTx(ctx, tx, authorizedVersion, targets, fresh, true); err != nil {
		return ver, nil, err
	}
	if fresh {
		if op.CurrentVersion != r.ExpectedVersion || ver.InputDigest != r.InputDigest {
			return ver, nil, domain.ErrVersionConflict
		}
		w, _ := uuidFromString(r.WorkspaceID.UUID())
		operation, _ := uuidFromString(r.OperationID.UUID())
		if err := lockProductionDraft(ctx, tx, w, operation, int32(r.ExpectedVersion)); err != nil {
			return ver, nil, domain.ErrVersionConflict
		}
		if err := checkProductionBaselineTx(ctx, tx, ver, targets); err != nil {
			return ver, nil, err
		}
	}
	return ver, targets, nil
}

func (s *Store) productionGenerationModelTx(ctx context.Context, tx pgx.Tx, r domain.ProductionGenerationRequest) (domain.ModelSetting, domain.ModelProvider, error) {
	w, _ := uuidFromString(r.WorkspaceID.UUID())
	settingID, _ := uuidFromString(r.ModelSettingID.UUID())
	// Model updates do not serialize on the workspace row. Hold their rows
	// through queue/claim/result commit, and digest every configuration field.
	var providerID pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT provider_id FROM model_settings WHERE workspace_id=$1 AND id=$2 FOR SHARE`, w, settingID).Scan(&providerID); err != nil {
		return domain.ModelSetting{}, domain.ModelProvider{}, governanceRepositoryError("load generation model", err)
	}
	var locked pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM model_providers WHERE workspace_id=$1 AND id=$2 FOR SHARE`, w, providerID).Scan(&locked); err != nil {
		return domain.ModelSetting{}, domain.ModelProvider{}, governanceRepositoryError("load generation provider", err)
	}
	q := s.queries.WithTx(tx)
	settingRow, err := q.GetModelSetting(ctx, dbgen.GetModelSettingParams{WorkspaceID: w, SettingID: settingID})
	if err != nil {
		return domain.ModelSetting{}, domain.ModelProvider{}, err
	}
	setting, err := modelSettingFromRow(settingRow)
	if err != nil {
		return setting, domain.ModelProvider{}, err
	}
	providerRow, err := q.GetModelProvider(ctx, dbgen.GetModelProviderParams{WorkspaceID: w, ProviderID: providerID})
	if err != nil {
		return setting, domain.ModelProvider{}, err
	}
	provider, err := modelProviderFromRow(providerRow)
	if err != nil {
		return setting, provider, err
	}
	revision, err := domain.ProductionGenerationModelRevision(setting, provider)
	if err != nil {
		return setting, provider, err
	}
	if !setting.Enabled || !provider.Enabled || setting.Kind != domain.ModelKindLLM || !provider.Protocol.SupportsGeneration() || r.MaxOutputTokens > setting.TokenLimit || revision != r.ModelConfigRevision {
		return setting, provider, domain.ErrGenerationNotAuthorized
	}
	return setting, provider, nil
}

func (s *Store) prepareProductionGenerationTx(ctx context.Context, tx pgx.Tx, request domain.ProductionGenerationRequest, agent identity.PrincipalID) (domain.ProductionGenerationWork, domain.ProductionVersion, error) {
	work := domain.ProductionGenerationWork{Request: request, AgentID: agent}
	ver, targets, err := s.productionGenerationVersionTx(ctx, tx, request, true)
	if err != nil {
		return work, ver, err
	}
	ver.CreatedBy = request.PrincipalID
	if agent.IsZero() {
		store := &Store{queries: s.queries.WithTx(tx)}
		principal, err := store.WorkspaceAgentPrincipal(ctx, request.WorkspaceID)
		if err != nil {
			return work, ver, err
		}
		work.AgentID = principal.ID
	}
	agentVersion := ver
	agentVersion.CreatedBy = work.AgentID
	if err := s.validateProductionInputTx(ctx, tx, agentVersion, targets, true, true); err != nil {
		return work, ver, err
	}
	work.Setting, work.Provider, err = s.productionGenerationModelTx(ctx, tx, request)
	if err != nil {
		return work, ver, err
	}
	var wrapper struct {
		Scope json.RawMessage `json:"scope"`
	}
	if json.Unmarshal(ver.InputJSON, &wrapper) != nil || len(wrapper.Scope) == 0 {
		return work, ver, domain.ErrInputIncomplete
	}
	work.Input = wrapper.Scope
	if json.Unmarshal(ver.DeclarationsJSON, &work.Declarations) != nil {
		return work, ver, domain.ErrPriorStateUnknown
	}
	work.Facts, err = productionGenerationFactsTx(ctx, tx, request.WorkspaceID, work.Input)
	if err != nil {
		return work, ver, err
	}
	work.PromptDigest, err = app.ProductionGenerationPromptDigest(work)
	return work, ver, err
}

func productionGenerationFactsTx(ctx context.Context, tx pgx.Tx, w identity.WorkspaceID, inputRaw json.RawMessage) (json.RawMessage, error) {
	var input productionInput
	if err := json.Unmarshal(inputRaw, &input); err != nil {
		return nil, err
	}
	// Candidates per snapshot, so fact aggregation can be scoped to the
	// objects the draft actually references. Coverage units on large sources
	// (tens of thousands of members) otherwise blow the 1MB prompt budget for
	// every generation, manual and batched alike.
	candidateIDsBySnapshot := map[string][]string{}
	for _, candidate := range input.Candidates {
		id, err := identity.ParseSemanticCandidateID(candidate.CandidateID)
		if err != nil {
			return nil, err
		}
		candidateIDsBySnapshot[candidate.SnapshotID] = append(candidateIDsBySnapshot[candidate.SnapshotID], id.UUID())
	}
	members := []map[string]any{}
	for _, snapshot := range input.Snapshots {
		entries, err := snapshotGenerationMembersTx(ctx, tx, w, snapshot, candidateIDsBySnapshot[snapshot.SnapshotID])
		if err != nil {
			return nil, err
		}
		members = append(members, entries...)
	}
	candidates := []json.RawMessage{}
	for _, candidate := range input.Candidates {
		id, err := identity.ParseSemanticCandidateID(candidate.CandidateID)
		if err != nil {
			return nil, err
		}
		var value json.RawMessage
		if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('candidateId',$3::text,'title',title,'kind',candidate_kind,'suggestion',proposal_input,'evidence',evidence) FROM semantic_candidates WHERE workspace_id=$1 AND id=$2`, w.UUID(), id.UUID(), id.String()).Scan(&value); err != nil {
			return nil, err
		}
		candidates = append(candidates, value)
	}
	evidence := []json.RawMessage{}
	for _, selected := range input.Evidence {
		id, err := identity.ParseEvidenceID(selected.EvidenceID)
		if err != nil {
			return nil, err
		}
		var value json.RawMessage
		// Only explicit business-rule fields are exposed, not arbitrary metadata
		// carrying connector credentials or operational configuration.
		if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('evidenceId',$3::text,'type',evidence_type,'locator',locator,'digest',content_digest,'businessRule',metadata->'businessRule','rule',metadata->'rule','attestation',metadata->'attestation') FROM evidence_artifacts WHERE workspace_id=$1 AND id=$2`, w.UUID(), id.UUID(), id.String()).Scan(&value); err != nil {
			return nil, err
		}
		evidence = append(evidence, value)
	}
	raw, err := json.Marshal(map[string]any{"members": members, "candidates": candidates, "evidence": evidence})
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, domain.ErrLimitExceeded
	}
	return raw, nil
}

// generationMembersQuery returns the member projection shared by the scoped
// and unscoped fact aggregation paths.
const generationMembersQuery = `SELECT m.kind,m.object_id,m.revision_id,m.historical_name,m.historical_locator,m.content_digest,m.parent_object_id,m.parent_revision_id,
		CASE m.kind WHEN 'dataset' THEN jsonb_build_object('datasetKind',d.dataset_kind)
		WHEN 'field' THEN jsonb_build_object('dataType',f.data_type,'nullable',f.nullable,'ordinal',f.ordinal)
		WHEN 'code' THEN CASE WHEN octet_length(c.content_bytes)<=1048576 THEN jsonb_build_object('language',c.language,'content',convert_from(c.content_bytes,'UTF8')) END
		WHEN 'lineage' THEN jsonb_build_object('kind',l.edge_kind) END
		FROM source_snapshot_members m
		LEFT JOIN physical_dataset_revisions d ON m.kind='dataset' AND d.workspace_id=m.workspace_id AND d.id=m.revision_id
		LEFT JOIN physical_field_revisions f ON m.kind='field' AND f.workspace_id=m.workspace_id AND f.id=m.revision_id
		LEFT JOIN source_code_revisions c ON m.kind='code' AND c.workspace_id=m.workspace_id AND c.id=m.revision_id
		LEFT JOIN source_lineage_revisions l ON m.kind='lineage' AND l.workspace_id=m.workspace_id AND l.id=m.revision_id
		WHERE m.workspace_id=$1 AND m.snapshot_id=$2 AND m.coverage_key=ANY($3::text[])`

// snapshotGenerationMembersTx aggregates the pinned members of one snapshot.
// When the input references candidates, aggregation is scoped to the
// candidates' datasets and their direct fields: coverage units on large
// sources hold tens of thousands of members and would exceed the 1MB prompt
// budget regardless of provider. Sources whose candidates do not map to
// members keep the previous whole-unit behavior via the fallback query.
func snapshotGenerationMembersTx(
	ctx context.Context, tx pgx.Tx, w identity.WorkspaceID,
	snapshot struct {
		SourceID     string   `json:"sourceId"`
		SnapshotID   string   `json:"snapshotId"`
		Digest       string   `json:"digest"`
		CoverageKeys []string `json:"coverageKeys"`
	},
	candidateIDs []string,
) ([]map[string]any, error) {
	collect := func(rows pgx.Rows) ([]map[string]any, bool, error) {
		defer rows.Close()
		entries := []map[string]any{}
		total := 0
		for rows.Next() {
			var kind, name, locator, digest string
			var id, revision, parent, parentRevision pgtype.UUID
			var content json.RawMessage
			if err := rows.Scan(&kind, &id, &revision, &name, &locator, &digest, &parent, &parentRevision, &content); err != nil {
				return nil, false, err
			}
			prefixes := map[string][2]identity.Prefix{"dataset": {identity.PhysicalDataset, identity.PhysicalDatasetRevision}, "field": {identity.PhysicalField, identity.PhysicalFieldRevision}, "code": {identity.CodeArtifact, identity.SourceCodeRevision}, "lineage": {identity.LineageEdge, identity.SourceLineageRevision}}
			p, ok := prefixes[kind]
			if !ok || len(content) == 0 {
				return nil, false, domain.ErrInputIncomplete
			}
			entry := map[string]any{"snapshotId": snapshot.SnapshotID, "kind": kind, "objectId": snapshotWireID(p[0], id), "revisionId": snapshotWireID(p[1], revision), "name": name, "locator": locator, "digest": digest, "content": content}
			if parent.Valid {
				entry["parentObjectId"] = snapshotWireID(identity.PhysicalDataset, parent)
				entry["parentRevisionId"] = snapshotWireID(identity.PhysicalDatasetRevision, parentRevision)
			}
			raw, err := json.Marshal(entry)
			if err != nil {
				return nil, false, err
			}
			total += len(raw)
			entries = append(entries, entry)
			if total > 1<<20 || len(entries) > 2000 {
				return nil, false, domain.ErrLimitExceeded
			}
		}
		return entries, len(entries) > 0, rows.Err()
	}

	if len(candidateIDs) > 0 {
		rows, err := tx.Query(ctx, generationMembersQuery+`
		AND (
			m.historical_name IN (
				SELECT split_part(c2.candidate_key, ':', 3) FROM semantic_candidates c2
				WHERE c2.workspace_id=$1 AND c2.id=ANY($4::uuid[])
			)
			OR m.parent_object_id IN (
				SELECT m2.object_id FROM source_snapshot_members m2
				WHERE m2.workspace_id=$1 AND m2.snapshot_id=$2
				  AND m2.historical_name IN (
					SELECT split_part(c2.candidate_key, ':', 3) FROM semantic_candidates c2
					WHERE c2.workspace_id=$1 AND c2.id=ANY($4::uuid[])
				  )
			)
		) ORDER BY m.kind,m.object_id LIMIT 2001`,
			w.UUID(), mustSnapshotUUID(snapshot.SnapshotID), snapshot.CoverageKeys, candidateIDs)
		if err != nil {
			return nil, err
		}
		entries, found, err := collect(rows)
		if err != nil {
			return nil, err
		}
		if found {
			return entries, nil
		}
	}

	rows, err := tx.Query(ctx, generationMembersQuery+` ORDER BY m.kind,m.object_id LIMIT 2001`,
		w.UUID(), mustSnapshotUUID(snapshot.SnapshotID), snapshot.CoverageKeys)
	if err != nil {
		return nil, err
	}
	entries, _, err := collect(rows)
	return entries, err
}

func generationRepositoryError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}
