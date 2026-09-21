package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ governanceapp.ProductionRepository = (*Store)(nil)

func parseUUIDOrTypeID(value string) (pgtype.UUID, error) {
	if u, err := uuidFromString(value); err == nil {
		return u, nil
	}
	parsed, err := identity.ParseAny(value)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return uuidFromString(parsed.UUID())
}

func (s *Store) CreateProductionOperationTx(
	ctx context.Context,
	op domain.ProductionOperation,
	version domain.ProductionVersion,
	targets []domain.ProductionTarget,
	links []domain.ProductionCandidateLink,
	contributors []domain.ProductionContributor,
	reservations []domain.ProductionIdentityReservation,
	cmd domain.ProductionCommand,
	claim domain.ProductionRequestClaim,
	proposals []domain.Proposal,
	assetDrafts []governanceapp.SemanticAssetDraft,
) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governanceRepositoryError("begin create production operation", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.queries.WithTx(tx)

	if err := s.validateProductionInputTx(ctx, tx, version, targets, true); err != nil {
		return err
	}
	if err := checkProductionBaselineTx(ctx, tx, version, targets); err != nil {
		return err
	}
	if err := s.checkProductionSupersedeTx(ctx, tx, version); err != nil {
		return err
	}
	opUUID, err := uuidFromString(op.ID.UUID())
	if err != nil {
		return err
	}
	wspUUID, err := uuidFromString(op.WorkspaceID.UUID())
	if err != nil {
		return err
	}
	creatorUUID, err := uuidFromString(op.CreatedBy.UUID())
	if err != nil {
		return err
	}

	var supersedesUUID pgtype.UUID
	if op.SupersedesOperationID != nil {
		supersedesUUID, err = uuidFromString(op.SupersedesOperationID.UUID())
		if err != nil {
			return err
		}
	}

	// 1. Insert production_operation
	_, err = q.InsertProductionOperation(ctx, dbgen.InsertProductionOperationParams{
		ID:                    opUUID,
		WorkspaceID:           wspUUID,
		CreatedBy:             creatorUUID,
		CurrentVersion:        int32(op.CurrentVersion),
		CreatedAt:             pgtype.Timestamptz{Time: op.CreatedAt, Valid: true},
		UpdatedAt:             pgtype.Timestamptz{Time: op.UpdatedAt, Valid: true},
		SupersedesOperationID: supersedesUUID,
	})
	if err != nil {
		return governanceRepositoryError("insert production operation", err)
	}

	// 2. Insert production_version
	err = insertProductionVersion(ctx, q, version)
	if err != nil {
		return governanceRepositoryError("insert production version", err)
	}
	if err := transferProductionSupersedeTx(ctx, tx, version, targets); err != nil {
		return err
	}
	if err := transferProductionReintroductionsTx(ctx, tx, version, targets); err != nil {
		return err
	}

	// 3. Insert reservations
	for _, res := range reservations {
		tUUID, err := uuidFromString(res.TargetID)
		if err != nil {
			// Try parsing as TypeID first
			parsed, parseErr := identity.ParseAny(res.TargetID)
			if parseErr != nil {
				return err
			}
			tUUID, err = uuidFromString(parsed.UUID())
			if err != nil {
				return err
			}
		}
		err = q.InsertProductionReservation(ctx, dbgen.InsertProductionReservationParams{
			WorkspaceID:         wspUUID,
			Kind:                res.Kind,
			IdentityKey:         res.IdentityKey,
			TargetID:            tUUID,
			CreationOperationID: opUUID,
			OwnerOperationID:    opUUID,
			CreatedAt:           pgtype.Timestamptz{Time: res.CreatedAt, Valid: true},
		})
		if err != nil {
			return productionIdentityRepositoryError("insert identity reservation", err)
		}
		var reserved, owner pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT target_id,owner_operation_id FROM production_identity_reservations WHERE workspace_id=$1 AND kind=$2 AND identity_key=$3 FOR UPDATE`, wspUUID, res.Kind, res.IdentityKey).Scan(&reserved, &owner); err != nil {
			return err
		}
		if reserved != tUUID || owner != opUUID {
			return domain.ErrIdentityConflict
		}
	}

	// 4. Insert draft semantic_assets if any
	for _, draft := range assetDrafts {
		astUUID, err := uuidFromString(draft.ID.UUID())
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO semantic_assets (
				id, workspace_id, namespace, key, asset_type, lifecycle_state, current_revision_id, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, 'draft', NULL, clock_timestamp(), clock_timestamp())
		`, astUUID, wspUUID, draft.Namespace, draft.Key, draft.AssetType)
		if err != nil {
			return productionIdentityRepositoryError("insert draft semantic asset", err)
		}
	}

	if err := insertProductionDraftProposals(ctx, tx, wspUUID, proposals); err != nil {
		return err
	}
	if err := insertProductionChanges(ctx, q, targets); err != nil {
		return err
	}
	// 6. Insert production_targets
	for _, t := range targets {
		tUUID, err := uuidFromString(t.TargetID)
		if err != nil {
			parsed, parseErr := identity.ParseAny(t.TargetID)
			if parseErr != nil {
				return err
			}
			tUUID, err = uuidFromString(parsed.UUID())
			if err != nil {
				return err
			}
		}

		var propUUID pgtype.UUID
		if t.ProposalID != nil {
			propUUID, err = uuidFromString(t.ProposalID.UUID())
			if err != nil {
				return err
			}
		}

		var baseRevUUID pgtype.UUID
		if t.BaseRevisionID != nil {
			baseRevUUID, err = parseUUIDOrTypeID(*t.BaseRevisionID)
			if err != nil {
				return err
			}
		}

		var idKey pgtype.Text
		if t.IdentityKey != nil {
			idKey = pgtype.Text{String: *t.IdentityKey, Valid: true}
		}
		var baseObjVer pgtype.Int4
		if t.BaseObjectVersion != nil {
			baseObjVer = pgtype.Int4{Int32: int32(*t.BaseObjectVersion), Valid: true}
		}
		var regWriteVer pgtype.Int4
		if t.RegistryWriteVersion != nil {
			regWriteVer = pgtype.Int4{Int32: int32(*t.RegistryWriteVersion), Valid: true}
		}

		err = q.InsertProductionTarget(ctx, dbgen.InsertProductionTargetParams{
			WorkspaceID:          wspUUID,
			OperationID:          opUUID,
			Version:              int32(t.Version),
			LocalKey:             t.LocalKey,
			Kind:                 t.Kind,
			Intent:               t.Intent,
			TargetID:             tUUID,
			IdentityKey:          idKey,
			BaseRevisionID:       baseRevUUID,
			BaseObjectVersion:    baseObjVer,
			RegistryWriteVersion: regWriteVer,
			ContentJson:          t.ContentJSON,
			CanonicalContent:     t.ContentJSON,
			ContentDigest:        t.ContentDigest,
			ProposalID:           propUUID,
			Outcome:              t.Outcome,
		})
		if err != nil {
			return governanceRepositoryError("insert production target", err)
		}
	}

	// 7. Insert production_candidate_links
	decisions, err := insertProductionDecisions(ctx, tx, version, targets, links)
	if err != nil {
		return err
	}
	for _, link := range links {
		candUUID, err := uuidFromString(link.CandidateID)
		if err != nil {
			parsed, parseErr := identity.ParseAny(link.CandidateID)
			if parseErr != nil {
				return fmt.Errorf("invalid candidate id %q: %w", link.CandidateID, parseErr)
			}
			candUUID, err = uuidFromString(parsed.UUID())
			if err != nil {
				return err
			}
		}
		err = q.InsertProductionCandidateLink(ctx, dbgen.InsertProductionCandidateLinkParams{
			DecisionID:      decisions[link.CandidateID],
			WorkspaceID:     wspUUID,
			CandidateID:     candUUID,
			CandidateDigest: link.CandidateDigest,
			OperationID:     opUUID,
			Version:         int32(link.Version),
			LocalKey:        link.LocalKey,
			IsPrimary:       link.IsPrimary,
		})
		if err != nil {
			return governanceRepositoryError("insert production candidate link", err)
		}
	}

	// 8. Insert contributors
	for _, c := range contributors {
		pUUID, err := uuidFromString(c.PrincipalID.UUID())
		if err != nil {
			return err
		}
		err = q.InsertProductionContributor(ctx, dbgen.InsertProductionContributorParams{
			WorkspaceID: wspUUID,
			OperationID: opUUID,
			PrincipalID: pUUID,
			Role:        c.Role,
			CreatedAt:   pgtype.Timestamptz{Time: c.CreatedAt, Valid: true},
		})
		if err != nil {
			return governanceRepositoryError("insert contributor", err)
		}
	}

	if err := inheritProductionCreationContributorsTx(ctx, tx, version, targets); err != nil {
		return err
	}
	// 9. Insert request claim
	err = q.InsertProductionRequestClaim(ctx, dbgen.InsertProductionRequestClaimParams{
		WorkspaceID:    wspUUID,
		BusinessDigest: claim.BusinessDigest,
		OperationID:    opUUID,
		CreatedAt:      pgtype.Timestamptz{Time: claim.CreatedAt, Valid: true},
	})
	if err != nil {
		return governanceRepositoryError("insert request claim", err)
	}

	// 10. Insert command
	if err := insertProductionMutationEvents(ctx, q, version, "created"); err != nil {
		return err
	}
	resUUID, err := parseUUIDOrTypeID(cmd.ResultID)
	if err != nil {
		return err
	}
	err = q.InsertProductionCommand(ctx, dbgen.InsertProductionCommandParams{
		WorkspaceID:      wspUUID,
		PrincipalID:      creatorUUID,
		CommandKind:      cmd.CommandKind,
		IdempotencyKey:   cmd.IdempotencyKey,
		RequestDigest:    cmd.RequestDigest,
		OperationID:      opUUID,
		OperationVersion: int32(cmd.OperationVersion),
		ResultKind:       cmd.ResultKind,
		ResultID:         resUUID,
		CommittedAt:      pgtype.Timestamptz{Time: cmd.CommittedAt, Valid: true},
	})
	if err != nil {
		return governanceRepositoryError("insert production command", err)
	}

	return tx.Commit(ctx)
}

func (s *Store) GetProductionOperation(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
) (domain.ProductionOperation, domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, []domain.ProductionContributor, error) {
	return s.GetProductionOperationVersion(ctx, workspace, opID, 0)
}

func (s *Store) GetProductionOperationVersion(
	ctx context.Context, workspace identity.WorkspaceID, opID identity.ProductionOperationID, requestedVersion int,
) (domain.ProductionOperation, domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, []domain.ProductionContributor, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	op, ver, targets, links, contributors, err := s.getProductionOperationVersionTx(ctx, tx, workspace, opID, requestedVersion)
	if err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, err
	}
	return op, ver, targets, links, contributors, nil
}

func (s *Store) getProductionOperationVersionTx(
	ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, opID identity.ProductionOperationID, requestedVersion int,
) (domain.ProductionOperation, domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, []domain.ProductionContributor, error) {
	q := s.queries.WithTx(tx)
	wspUUID, err := uuidFromString(workspace.UUID())
	if err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, err
	}
	opUUID, err := uuidFromString(opID.UUID())
	if err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, err
	}

	row, err := q.GetProductionOperation(ctx, dbgen.GetProductionOperationParams{
		WorkspaceID: wspUUID,
		ID:          opUUID,
	})
	if err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, governanceRepositoryError("get production operation", err)
	}

	creatorID, err := identity.PrincipalIDFromUUIDBytes(row.CreatedBy.Bytes)
	if err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, err
	}
	op := domain.ProductionOperation{
		ID:             opID,
		WorkspaceID:    workspace,
		CreatedBy:      creatorID,
		CurrentVersion: int(row.CurrentVersion),
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_operations WHERE workspace_id=$1 AND supersedes_operation_id=$2)`, wspUUID, opUUID).Scan(&op.Superseded); err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, err
	}

	if requestedVersion == 0 {
		requestedVersion = int(row.CurrentVersion)
	}
	if row.SupersedesOperationID.Valid {
		id, err := identity.ProductionOperationIDFromUUIDBytes(row.SupersedesOperationID.Bytes)
		if err != nil {
			return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, err
		}
		op.SupersedesOperationID = &id
	}
	// Content is selected by immutable version, independently of the current head.
	vRow, err := q.GetProductionVersion(ctx, dbgen.GetProductionVersionParams{
		WorkspaceID: wspUUID,
		OperationID: opUUID,
		Version:     int32(requestedVersion),
	})
	if err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, governanceRepositoryError("get production version", err)
	}

	vCreatorID, _ := identity.PrincipalIDFromUUIDBytes(vRow.CreatedBy.Bytes)
	if vRow.HistoryQuality != "verified" {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, domain.ErrPriorStateUnknown
	}
	var baselineHead domain.HeadReference
	if err := json.Unmarshal(vRow.BaselineHead, &baselineHead); err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, domain.ErrPriorStateUnknown
	}
	var declarations []domain.TargetDeclaration
	if err := json.Unmarshal(vRow.CanonicalDeclarations, &declarations); err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, domain.ErrPriorStateUnknown
	}
	declarationByKey := map[string]*domain.TargetDeclaration{}
	for i := range declarations {
		declarationByKey[declarations[i].LocalKey] = &declarations[i]
	}
	ver := domain.ProductionVersion{
		DeclarationsJSON: vRow.CanonicalDeclarations, BaselineJSON: vRow.CanonicalBaseline, BaselineHead: baselineHead, HistoryQuality: vRow.HistoryQuality,
		WorkspaceID:   workspace,
		OperationID:   opID,
		Version:       int(vRow.Version),
		InputJSON:     vRow.CanonicalInput,
		InputDigest:   vRow.InputDigest,
		RequestDigest: vRow.RequestDigest,
		SetDigest:     vRow.SetDigest,
		CreatedBy:     vCreatorID,
		CreatedAt:     vRow.CreatedAt.Time,
	}
	if vRow.FrozenAt.Valid {
		ver.FrozenAt = &vRow.FrozenAt.Time
	}

	// Targets
	tRows, err := q.ListProductionTargetsForVersion(ctx, dbgen.ListProductionTargetsForVersionParams{
		WorkspaceID: wspUUID,
		OperationID: opUUID,
		Version:     int32(requestedVersion),
	})
	if err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, governanceRepositoryError("list targets", err)
	}

	var targets []domain.ProductionTarget
	for _, tr := range tRows {
		targetID, err := productionTargetID(tr.Kind, tr.TargetID)
		if err != nil {
			return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, err
		}
		target := domain.ProductionTarget{
			Declaration:   declarationByKey[tr.LocalKey],
			WorkspaceID:   workspace,
			OperationID:   opID,
			Version:       int(tr.Version),
			LocalKey:      tr.LocalKey,
			Kind:          tr.Kind,
			Intent:        tr.Intent,
			TargetID:      targetID,
			ContentJSON:   tr.CanonicalContent,
			ContentDigest: tr.ContentDigest,
			Outcome:       tr.Outcome,
		}
		if tr.IdentityKey.Valid {
			target.IdentityKey = &tr.IdentityKey.String
		}
		if tr.BaseRevisionID.Valid {
			id, err := identity.RevisionIDFromUUIDBytes(tr.BaseRevisionID.Bytes)
			if err != nil {
				return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, err
			}
			value := id.String()
			target.BaseRevisionID = &value
		}
		if tr.BaseObjectVersion.Valid {
			value := int(tr.BaseObjectVersion.Int32)
			target.BaseObjectVersion = &value
		}
		if tr.RegistryWriteVersion.Valid {
			value := int(tr.RegistryWriteVersion.Int32)
			target.RegistryWriteVersion = &value
		}
		if tr.ProposalID.Valid {
			var state string
			if err := tx.QueryRow(ctx, `SELECT state FROM proposals WHERE workspace_id=$1 AND id=$2`, wspUUID, tr.ProposalID).Scan(&state); err != nil {
				return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, err
			}
			target.ProposalState = &state
			pID, err := identity.ProposalIDFromUUIDBytes(tr.ProposalID.Bytes)
			if err == nil {
				target.ProposalID = &pID
			}
		}
		targets = append(targets, target)
	}

	// Links
	lRows, err := q.ListProductionCandidateLinksForVersion(ctx, dbgen.ListProductionCandidateLinksForVersionParams{
		WorkspaceID: wspUUID,
		OperationID: opUUID,
		Version:     int32(requestedVersion),
	})
	if err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, governanceRepositoryError("list candidate links", err)
	}

	var links []domain.ProductionCandidateLink
	for _, lr := range lRows {
		links = append(links, domain.ProductionCandidateLink{
			WorkspaceID:     workspace,
			CandidateID:     formatUUID(lr.CandidateID),
			CandidateDigest: lr.CandidateDigest,
			OperationID:     opID,
			Version:         int(lr.Version),
			LocalKey:        lr.LocalKey,
			IsPrimary:       lr.IsPrimary,
		})
	}

	// Contributors
	cRows, err := q.ListProductionContributors(ctx, dbgen.ListProductionContributorsParams{
		WorkspaceID: wspUUID,
		OperationID: opUUID,
	})
	if err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, governanceRepositoryError("list contributors", err)
	}

	var contributors []domain.ProductionContributor
	for _, cr := range cRows {
		prnID, _ := identity.PrincipalIDFromUUIDBytes(cr.PrincipalID.Bytes)
		contributors = append(contributors, domain.ProductionContributor{
			WorkspaceID: workspace,
			OperationID: opID,
			PrincipalID: prnID,
			Role:        cr.Role,
			CreatedAt:   cr.CreatedAt.Time,
		})
	}

	if err := loadProductionRecovery(ctx, tx, q, &ver, targets, vRow.ActiveValidationAttemptNo); err != nil {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, err
	}
	return op, ver, targets, links, contributors, nil
}

func (s *Store) ListProductionOperations(
	ctx context.Context,
	workspace identity.WorkspaceID,
	limit int,
	offset int,
) ([]domain.ProductionOperation, error) {
	wspUUID, err := uuidFromString(workspace.UUID())
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListProductionOperations(ctx, dbgen.ListProductionOperationsParams{
		WorkspaceID: wspUUID,
		PageLimit:   int32(limit),
		PageOffset:  int32(offset),
	})
	if err != nil {
		return nil, governanceRepositoryError("list production operations", err)
	}

	var ops []domain.ProductionOperation
	for _, r := range rows {
		opID, _ := identity.ProductionOperationIDFromUUIDBytes(r.ID.Bytes)
		cID, _ := identity.PrincipalIDFromUUIDBytes(r.CreatedBy.Bytes)
		ops = append(ops, domain.ProductionOperation{
			ID:             opID,
			WorkspaceID:    workspace,
			CreatedBy:      cID,
			CurrentVersion: int(r.CurrentVersion),
			CreatedAt:      r.CreatedAt.Time,
			UpdatedAt:      r.UpdatedAt.Time,
		})
	}
	return ops, nil
}

func (s *Store) GetProductionCommand(
	ctx context.Context,
	workspace identity.WorkspaceID,
	principal identity.PrincipalID,
	cmdKind string,
	idempotencyKey string,
) (*domain.ProductionCommand, error) {
	wspUUID, err := uuidFromString(workspace.UUID())
	if err != nil {
		return nil, err
	}
	prnUUID, err := uuidFromString(principal.UUID())
	if err != nil {
		return nil, err
	}
	row, err := s.queries.GetProductionCommand(ctx, dbgen.GetProductionCommandParams{
		WorkspaceID:    wspUUID,
		PrincipalID:    prnUUID,
		CommandKind:    cmdKind,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, governanceRepositoryError("get production command", err)
	}

	opID, _ := identity.ProductionOperationIDFromUUIDBytes(row.OperationID.Bytes)
	var resID string
	if row.ResultKind == "release" {
		if rid, err := identity.ReleaseIDFromUUIDBytes(row.ResultID.Bytes); err == nil {
			resID = rid.String()
		}
	} else if row.ResultKind == "operation" {
		if oid, err := identity.ProductionOperationIDFromUUIDBytes(row.ResultID.Bytes); err == nil {
			resID = oid.String()
		}
	}
	if resID == "" {
		resID = formatUUID(row.ResultID)
	}

	return &domain.ProductionCommand{
		WorkspaceID:      workspace,
		PrincipalID:      principal,
		CommandKind:      row.CommandKind,
		IdempotencyKey:   row.IdempotencyKey,
		RequestDigest:    row.RequestDigest,
		OperationID:      opID,
		OperationVersion: int(row.OperationVersion),
		ResultKind:       row.ResultKind,
		ResultID:         resID,
		CommittedAt:      row.CommittedAt.Time,
		ResponseJSON:     row.ResponseJson,
	}, nil
}

func (s *Store) GetProductionRequestClaim(
	ctx context.Context,
	workspace identity.WorkspaceID,
	businessDigest string,
) (*domain.ProductionRequestClaim, error) {
	wspUUID, err := uuidFromString(workspace.UUID())
	if err != nil {
		return nil, err
	}
	row, err := s.queries.GetProductionRequestClaim(ctx, dbgen.GetProductionRequestClaimParams{
		WorkspaceID:    wspUUID,
		BusinessDigest: businessDigest,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, governanceRepositoryError("get production request claim", err)
	}

	opID, _ := identity.ProductionOperationIDFromUUIDBytes(row.OperationID.Bytes)
	return &domain.ProductionRequestClaim{
		WorkspaceID:    workspace,
		BusinessDigest: row.BusinessDigest,
		OperationID:    opID,
		CreatedAt:      row.CreatedAt.Time,
	}, nil
}

func (s *Store) GetProductionReservation(
	ctx context.Context,
	workspace identity.WorkspaceID,
	kind string,
	identityKey string,
) (*domain.ProductionIdentityReservation, error) {
	wspUUID, err := uuidFromString(workspace.UUID())
	if err != nil {
		return nil, err
	}
	row, err := s.queries.GetProductionReservation(ctx, dbgen.GetProductionReservationParams{
		WorkspaceID: wspUUID,
		Kind:        kind,
		IdentityKey: identityKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, governanceRepositoryError("get production reservation", err)
	}

	creationID, _ := identity.ProductionOperationIDFromUUIDBytes(row.CreationOperationID.Bytes)
	ownerID, _ := identity.ProductionOperationIDFromUUIDBytes(row.OwnerOperationID.Bytes)
	targetID, err := productionTargetID(row.Kind, row.TargetID)
	if err != nil {
		return nil, err
	}
	return &domain.ProductionIdentityReservation{
		WorkspaceID:         workspace,
		Kind:                row.Kind,
		IdentityKey:         row.IdentityKey,
		TargetID:            targetID,
		CreationOperationID: creationID,
		OwnerOperationID:    ownerID,
		CreatedAt:           row.CreatedAt.Time,
	}, nil
}

func (s *Store) ReplaceDraftTx(
	ctx context.Context,
	opID identity.ProductionOperationID,
	newVersion domain.ProductionVersion,
	targets []domain.ProductionTarget,
	links []domain.ProductionCandidateLink,
	cmd domain.ProductionCommand,
	proposals []domain.Proposal,
) error {
	return s.replaceProductionDraftTx(ctx, opID, newVersion, targets, links, cmd, proposals, nil)
}

func (s *Store) ReplaceDraftWithGenerationTx(ctx context.Context, opID identity.ProductionOperationID, newVersion domain.ProductionVersion, targets []domain.ProductionTarget, links []domain.ProductionCandidateLink, cmd domain.ProductionCommand, proposals []domain.Proposal, run identity.AgentRunID) error {
	return s.replaceProductionDraftTx(ctx, opID, newVersion, targets, links, cmd, proposals, &run)
}

func (s *Store) replaceProductionDraftTx(ctx context.Context, opID identity.ProductionOperationID, newVersion domain.ProductionVersion, targets []domain.ProductionTarget, links []domain.ProductionCandidateLink, cmd domain.ProductionCommand, proposals []domain.Proposal, suggestionRun *identity.AgentRunID) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governanceRepositoryError("begin replace draft", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	wspUUID, err := uuidFromString(newVersion.WorkspaceID.UUID())
	if err != nil {
		return err
	}
	opUUID, err := uuidFromString(opID.UUID())
	if err != nil {
		return err
	}
	creatorUUID, err := uuidFromString(newVersion.CreatedBy.UUID())
	if err != nil {
		return err
	}

	if err := s.validateProductionInputTx(ctx, tx, newVersion, targets, true); err != nil {
		return err
	}
	if err := checkProductionBaselineTx(ctx, tx, newVersion, targets); err != nil {
		return err
	}
	// Lock operation row for update
	q := s.queries.WithTx(tx)
	currentOp, err := q.GetProductionOperationForUpdate(ctx, dbgen.GetProductionOperationForUpdateParams{
		WorkspaceID: wspUUID,
		ID:          opUUID,
	})
	if err != nil {
		return governanceRepositoryError("lock production operation", err)
	}

	if int(currentOp.CurrentVersion)+1 != newVersion.Version {
		return fmt.Errorf("%w: version mismatch: current %d, next %d", domain.ErrConflict, currentOp.CurrentVersion, newVersion.Version)
	}
	if err := lockProductionDraft(ctx, tx, wspUUID, opUUID, currentOp.CurrentVersion); err != nil {
		return err
	}

	// Update operation version
	now := time.Now().UTC()
	err = q.UpdateProductionOperationVersion(ctx, dbgen.UpdateProductionOperationVersionParams{
		WorkspaceID:    wspUUID,
		ID:             opUUID,
		CurrentVersion: int32(newVersion.Version),
		UpdatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return governanceRepositoryError("update operation version", err)
	}

	// Insert new production_version
	err = insertProductionVersion(ctx, q, newVersion)
	if err != nil {
		return governanceRepositoryError("insert replacement production version", err)
	}

	// Targets & links
	if err := transferProductionReintroductionsTx(ctx, tx, newVersion, targets); err != nil {
		return err
	}
	if err := reserveReplacementTargets(ctx, tx, wspUUID, opUUID, targets); err != nil {
		return err
	}
	if err := insertProductionDraftProposals(ctx, tx, wspUUID, proposals); err != nil {
		return err
	}
	if err := insertProductionChanges(ctx, q, targets); err != nil {
		return err
	}
	for _, t := range targets {
		tUUID, err := parseUUIDOrTypeID(t.TargetID)
		if err != nil {
			return fmt.Errorf("%w: invalid target ID", domain.ErrInvalidArgument)
		}
		var propUUID pgtype.UUID
		if t.ProposalID != nil {
			propUUID, _ = uuidFromString(t.ProposalID.UUID())
		}
		var idKey pgtype.Text
		if t.IdentityKey != nil {
			idKey = pgtype.Text{String: *t.IdentityKey, Valid: true}
		}
		var baseRev pgtype.UUID
		if t.BaseRevisionID != nil {
			baseRev, err = parseUUIDOrTypeID(*t.BaseRevisionID)
			if err != nil {
				return err
			}
		}
		var baseVersion, registryVersion pgtype.Int4
		if t.BaseObjectVersion != nil {
			baseVersion = pgtype.Int4{Int32: int32(*t.BaseObjectVersion), Valid: true}
		}
		if t.RegistryWriteVersion != nil {
			registryVersion = pgtype.Int4{Int32: int32(*t.RegistryWriteVersion), Valid: true}
		}
		err = q.InsertProductionTarget(ctx, dbgen.InsertProductionTargetParams{
			BaseRevisionID: baseRev, BaseObjectVersion: baseVersion, RegistryWriteVersion: registryVersion, CanonicalContent: t.ContentJSON,
			WorkspaceID:   wspUUID,
			OperationID:   opUUID,
			Version:       int32(newVersion.Version),
			LocalKey:      t.LocalKey,
			Kind:          t.Kind,
			Intent:        t.Intent,
			TargetID:      tUUID,
			IdentityKey:   idKey,
			ContentJson:   t.ContentJSON,
			ContentDigest: t.ContentDigest,
			ProposalID:    propUUID,
			Outcome:       t.Outcome,
		})
		if err != nil {
			return governanceRepositoryError("insert replacement target", err)
		}
	}

	decisions, err := insertProductionDecisions(ctx, tx, newVersion, targets, links)
	if err != nil {
		return err
	}
	for _, link := range links {
		candUUID, err := uuidFromString(link.CandidateID)
		if err != nil {
			parsed, parseErr := identity.ParseAny(link.CandidateID)
			if parseErr != nil {
				return fmt.Errorf("invalid candidate id %q: %w", link.CandidateID, parseErr)
			}
			candUUID, err = uuidFromString(parsed.UUID())
			if err != nil {
				return err
			}
		}
		err = q.InsertProductionCandidateLink(ctx, dbgen.InsertProductionCandidateLinkParams{
			DecisionID:      decisions[link.CandidateID],
			WorkspaceID:     wspUUID,
			CandidateID:     candUUID,
			CandidateDigest: link.CandidateDigest,
			OperationID:     opUUID,
			Version:         int32(newVersion.Version),
			LocalKey:        link.LocalKey,
			IsPrimary:       link.IsPrimary,
		})
		if err != nil {
			return governanceRepositoryError("insert production candidate link", err)
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE proposals SET state='rejected',submitted_at=$4,decided_at=$4,updated_at=$4 WHERE workspace_id=$1 AND production_operation_id=$2 AND production_version=$3 AND state='draft'`, wspUUID, opUUID, currentOp.CurrentVersion, now); err != nil {
		return governanceRepositoryError("terminate replaced production drafts", err)
	}
	if err := q.InsertProductionContributor(ctx, dbgen.InsertProductionContributorParams{
		WorkspaceID: wspUUID, OperationID: opUUID, PrincipalID: creatorUUID,
		Role: domain.ContributorEditor, CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return governanceRepositoryError("insert editor contributor", err)
	}
	if err := q.InsertProductionCommand(ctx, dbgen.InsertProductionCommandParams{
		WorkspaceID: wspUUID, PrincipalID: creatorUUID, CommandKind: cmd.CommandKind,
		IdempotencyKey: cmd.IdempotencyKey, RequestDigest: cmd.RequestDigest,
		OperationID: opUUID, OperationVersion: int32(newVersion.Version),
		ResultKind: cmd.ResultKind, ResultID: opUUID,
		CommittedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return governanceRepositoryError("insert replacement command", err)
	}
	if err := insertProductionMutationEvents(ctx, q, newVersion, "replaced"); err != nil {
		return err
	}
	if suggestionRun != nil {
		if err := s.insertProductionGenerationApplicationTx(ctx, tx, newVersion, *suggestionRun); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	src := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		src[0:4], src[4:6], src[6:8], src[8:10], src[10:16])
}

func (s *Store) SubmitOperationTx(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	frozenAt time.Time,
	attempt domain.ValidationAttempt,
	runs []domain.ValidationRun,
	bindings []domain.ValidationBinding,
	proposals []domain.Proposal,
	cmd domain.ProductionCommand,
) error {
	return s.queueProductionAttemptTx(ctx, workspace, opID, version, attempt, cmd, true)
}

func (s *Store) CreateValidationAttemptTx(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	attempt domain.ValidationAttempt,
	runs []domain.ValidationRun,
	bindings []domain.ValidationBinding,
	cmd domain.ProductionCommand,
) error {
	return s.queueProductionAttemptTx(ctx, workspace, opID, version, attempt, cmd, false)
}

func (s *Store) GetValidationAttempt(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	attemptNo int,
) (*domain.ValidationAttempt, error) {
	wspUUID, err := uuidFromString(workspace.UUID())
	if err != nil {
		return nil, err
	}
	opUUID, err := uuidFromString(opID.UUID())
	if err != nil {
		return nil, err
	}

	row, err := s.queries.GetProductionValidationAttempt(ctx, dbgen.GetProductionValidationAttemptParams{
		WorkspaceID:       wspUUID,
		OperationID:       opUUID,
		ProductionVersion: int32(version),
		AttemptNo:         int32(attemptNo),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, governanceRepositoryError("get validation attempt", err)
	}
	return validationAttemptFromRow(row), nil
}

func (s *Store) GetLatestValidationAttempt(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
) (*domain.ValidationAttempt, error) {
	wspUUID, err := uuidFromString(workspace.UUID())
	if err != nil {
		return nil, err
	}
	opUUID, err := uuidFromString(opID.UUID())
	if err != nil {
		return nil, err
	}

	row, err := s.queries.GetLatestProductionValidationAttempt(ctx, dbgen.GetLatestProductionValidationAttemptParams{
		WorkspaceID:       wspUUID,
		OperationID:       opUUID,
		ProductionVersion: int32(version),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, governanceRepositoryError("get latest validation attempt", err)
	}
	return validationAttemptFromRow(row), nil
}

func (s *Store) ListValidationAttempts(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	limit int,
	offset int,
) ([]domain.ValidationAttempt, error) {
	wspUUID, err := uuidFromString(workspace.UUID())
	if err != nil {
		return nil, err
	}
	opUUID, err := uuidFromString(opID.UUID())
	if err != nil {
		return nil, err
	}

	rows, err := s.queries.ListProductionValidationAttempts(ctx, dbgen.ListProductionValidationAttemptsParams{
		WorkspaceID:       wspUUID,
		OperationID:       opUUID,
		ProductionVersion: int32(version),
		PageLimit:         int32(limit),
		PageOffset:        int32(offset),
	})
	if err != nil {
		return nil, governanceRepositoryError("list validation attempts", err)
	}
	res := make([]domain.ValidationAttempt, 0, len(rows))
	for _, r := range rows {
		res = append(res, *validationAttemptFromRow(r))
	}
	return res, nil
}

func validationAttemptFromRow(row dbgen.ProductionValidationAttempt) *domain.ValidationAttempt {
	wsp, _ := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	op, _ := identity.ProductionOperationIDFromUUIDBytes(row.OperationID.Bytes)
	creator, _ := identity.PrincipalIDFromUUIDBytes(row.CreatedBy.Bytes)

	var valDigest *string
	if row.ValidationDigest.Valid {
		valDigest = &row.ValidationDigest.String
	}
	var compAt *time.Time
	if row.CompletedAt.Valid {
		t := row.CompletedAt.Time.UTC()
		compAt = &t
	}

	return &domain.ValidationAttempt{
		WorkspaceID:          wsp,
		OperationID:          op,
		ProductionVersion:    int(row.ProductionVersion),
		AttemptNo:            int(row.AttemptNo),
		SetDigest:            row.SetDigest,
		InputDigest:          row.InputDigest,
		FreshnessWitnessJSON: row.FreshnessWitnessJson,
		FreshnessDigest:      row.FreshnessDigest,
		RequiredChecksJSON:   row.RequiredChecksJson,
		RequiredChecksDigest: row.RequiredChecksDigest,
		Status:               row.Status,
		ValidationDigest:     valDigest,
		CreatedBy:            creator,
		CreatedAt:            row.CreatedAt.Time.UTC(),
		CompletedAt:          compAt,
	}
}

func (s *Store) ReviewOperationTx(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	attemptNo int,
	setDigest string,
	validationDigest string,
	reviews []domain.Review,
	bindings []domain.ProductionReviewBinding,
	proposals []domain.Proposal,
	cmd domain.ProductionCommand,
) error {
	_, ver, targets, _, _, err := s.GetProductionOperationVersion(ctx, workspace, opID, version)
	if err != nil {
		return err
	}
	if len(reviews) == 0 || len(reviews) > 32 || len(bindings) != len(reviews) || len(proposals) != len(reviews) || cmd.WorkspaceID != workspace || cmd.OperationID != opID || cmd.OperationVersion != version || cmd.CommandKind != domain.CommandReview || setDigest != ver.SetDigest {
		return domain.ErrInvalidArgument
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return governanceRepositoryError("begin review operation", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ver.CreatedBy = cmd.PrincipalID
	if err := s.productionActionTx(ctx, tx, ver, domain.CommandReview); err != nil {
		return err
	}
	if err := s.validateProductionInputTx(ctx, tx, ver, targets, false); err != nil {
		return err
	}
	var contributor bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_contributors WHERE workspace_id=$1 AND operation_id=$2 AND principal_id=$3)`, workspace.UUID(), opID.UUID(), cmd.PrincipalID.UUID()).Scan(&contributor); err != nil {
		return err
	}
	if contributor {
		return domain.ErrSodConflict
	}
	allowFailed := reviews[0].Decision == domain.ReviewRejected
	if err := s.productionAttemptGate(ctx, tx, ver, targets, attemptNo, validationDigest, allowFailed); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, review := range reviews {
		binding, proposal := bindings[i], proposals[i]
		if review.WorkspaceID != workspace || review.ReviewerPrincipalID != cmd.PrincipalID || review.Channel != domain.ReviewExpert || review.Decision != reviews[0].Decision || (review.Decision != domain.ReviewApproved && review.Decision != domain.ReviewRejected) || seen[review.ProposalID.String()] || binding.WorkspaceID != workspace || binding.ReviewID != review.ID || binding.OperationID != opID || binding.ProductionVersion != version || binding.AttemptNo != attemptNo || binding.SetDigest != setDigest || binding.ValidationDigest != validationDigest || proposal.ID != review.ProposalID || proposal.WorkspaceID != workspace || (allowFailed && proposal.State != domain.ProposalRejected) || (!allowFailed && proposal.State != domain.ProposalInReview) {
			return domain.ErrInvalidArgument
		}
		seen[review.ProposalID.String()] = true
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM proposals WHERE workspace_id=$1 AND id=$2 AND production_operation_id=$3 AND production_version=$4 FOR UPDATE`, workspace.UUID(), review.ProposalID.UUID(), opID.UUID(), version).Scan(&state); err != nil {
			return governanceRepositoryError("load review member", err)
		}
		if state != "in_review" {
			return domain.ErrVersionConflict
		}
		var prior bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reviews r JOIN production_review_bindings b ON b.workspace_id=r.workspace_id AND b.review_id=r.id WHERE b.workspace_id=$1 AND b.operation_id=$2 AND b.production_version=$3 AND b.attempt_no=$4 AND r.proposal_id=$5 AND r.reviewer_principal_id=$6 AND r.channel=$7)`, workspace.UUID(), opID.UUID(), version, attemptNo, review.ProposalID.UUID(), cmd.PrincipalID.UUID(), string(review.Channel)).Scan(&prior); err != nil {
			return err
		}
		if prior {
			return domain.ErrConflict
		}
	}
	q := s.queries.WithTx(tx)
	wspUUID, err := uuidFromString(workspace.UUID())
	if err != nil {
		return err
	}
	opUUID, err := uuidFromString(opID.UUID())
	if err != nil {
		return err
	}

	for _, rev := range reviews {
		revUUID, err := uuidFromString(rev.ID.UUID())
		if err != nil {
			return err
		}
		pUUID, err := uuidFromString(rev.ProposalID.UUID())
		if err != nil {
			return err
		}
		revPrincipalUUID, err := uuidFromString(rev.ReviewerPrincipalID.UUID())
		if err != nil {
			return err
		}
		_, err = q.CreateReview(ctx, dbgen.CreateReviewParams{
			ID:                  revUUID,
			WorkspaceID:         wspUUID,
			ProposalID:          pUUID,
			ReviewerPrincipalID: revPrincipalUUID,
			Channel:             string(rev.Channel),
			Decision:            string(rev.Decision),
			Note:                rev.Note,
			CreatedAt:           pgtype.Timestamptz{Time: rev.CreatedAt, Valid: true},
		})
		if err != nil {
			return governanceRepositoryError("create review", err)
		}
	}

	for _, b := range bindings {
		revUUID, err := uuidFromString(b.ReviewID.UUID())
		if err != nil {
			return err
		}
		err = q.InsertProductionReviewBinding(ctx, dbgen.InsertProductionReviewBindingParams{
			WorkspaceID:       wspUUID,
			ReviewID:          revUUID,
			OperationID:       opUUID,
			ProductionVersion: int32(version),
			AttemptNo:         int32(attemptNo),
			SetDigest:         setDigest,
			ValidationDigest:  validationDigest,
			CreatedAt:         pgtype.Timestamptz{Time: b.CreatedAt, Valid: true},
		})
		if err != nil {
			return governanceRepositoryError("insert review binding", err)
		}
	}

	for _, prop := range proposals {
		propUUID, err := uuidFromString(prop.ID.UUID())
		if err != nil {
			return err
		}
		var decAt pgtype.Timestamptz
		if prop.DecidedAt != nil {
			decAt = pgtype.Timestamptz{Time: *prop.DecidedAt, Valid: true}
		}
		if prop.State == domain.ProposalInReview {
			_, err = tx.Exec(ctx, `
				UPDATE proposals
				SET state = 'in_review', updated_at = $1
				WHERE workspace_id = $2 AND id = $3 AND state = 'validating'
			`, pgtype.Timestamptz{Time: cmd.CommittedAt, Valid: true}, wspUUID, propUUID)
			if err != nil {
				return governanceRepositoryError("advance proposal to in_review", err)
			}
		} else if prop.State == domain.ProposalRejected {
			_, err = tx.Exec(ctx, `
				UPDATE proposals
				SET state = 'rejected', decided_at = $1, updated_at = $2
				WHERE workspace_id = $3 AND id = $4
			`, decAt, decAt, wspUUID, propUUID)
			if err != nil {
				return governanceRepositoryError("update proposal to rejected", err)
			}
		}
	}

	cmdPrincipalUUID, err := uuidFromString(cmd.PrincipalID.UUID())
	if err != nil {
		return err
	}
	resUUID, err := parseUUIDOrTypeID(cmd.ResultID)
	if err != nil {
		return err
	}
	err = q.InsertProductionCommand(ctx, dbgen.InsertProductionCommandParams{
		ResponseJson:     cmd.ResponseJSON,
		WorkspaceID:      wspUUID,
		PrincipalID:      cmdPrincipalUUID,
		CommandKind:      cmd.CommandKind,
		IdempotencyKey:   cmd.IdempotencyKey,
		RequestDigest:    cmd.RequestDigest,
		OperationID:      opUUID,
		OperationVersion: int32(cmd.OperationVersion),
		ResultKind:       cmd.ResultKind,
		ResultID:         resUUID,
		CommittedAt:      pgtype.Timestamptz{Time: cmd.CommittedAt, Valid: true},
	})
	if err != nil {
		return governanceRepositoryError("insert production command", err)
	}

	ver.CreatedAt = cmd.CommittedAt
	if err := insertProductionMutationEvents(ctx, q, ver, "reviewed"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) PublishOperationTx(
	ctx context.Context,
	workspace identity.WorkspaceID,
	release domain.Release,
	releaseProposals []domain.ReleaseProposal,
	manifest domain.ProductionReleaseManifest,
	beforePins []domain.ProductionReleaseBeforePin,
	bindingInputs []domain.ProductionReleaseBindingInput,
	proposals []domain.Proposal,
	cmd domain.ProductionCommand,
) error {
	// Publication accepts a command, never caller-supplied manifests or approvals.
	return domain.ErrProductionSetRequired
}

func (s *Store) RollbackOperationTx(ctx context.Context, workspace identity.WorkspaceID, release domain.Release, releaseProposals []domain.ReleaseProposal, manifest domain.ProductionReleaseManifest, beforePins []domain.ProductionReleaseBeforePin, cmd domain.ProductionCommand) error {
	return domain.ErrProductionSetRequired
}

func (s *Store) GetLatestRelease(
	ctx context.Context,
	workspace identity.WorkspaceID,
) (*domain.Release, error) {
	wspUUID, err := uuidFromString(workspace.UUID())
	if err != nil {
		return nil, err
	}
	row, err := s.queries.GetLatestRelease(ctx, wspUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, governanceRepositoryError("get latest release", err)
	}
	rel, err := releaseFromRow(row)
	if err != nil {
		return nil, err
	}
	return &rel, nil
}

func (s *Store) GetProductionRelease(
	ctx context.Context,
	workspace identity.WorkspaceID,
	releaseID identity.ReleaseID,
) (*domain.Release, *domain.ProductionReleaseManifest, []domain.ProductionReleaseBeforePin, []domain.ReleaseProposal, error) {
	wspUUID, err := uuidFromString(workspace.UUID())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	relUUID, err := uuidFromString(releaseID.UUID())
	if err != nil {
		return nil, nil, nil, nil, err
	}

	relRow, err := s.queries.GetRelease(ctx, dbgen.GetReleaseParams{
		WorkspaceID: wspUUID,
		ReleaseID:   relUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil, nil, domain.ErrNotFound
		}
		return nil, nil, nil, nil, governanceRepositoryError("get release", err)
	}
	rel, err := releaseFromRow(relRow)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	manRow, err := s.queries.GetProductionReleaseManifest(ctx, dbgen.GetProductionReleaseManifestParams{
		WorkspaceID: wspUUID,
		ReleaseID:   relUUID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil, nil, err
	}
	var man *domain.ProductionReleaseManifest
	if err == nil {
		var beforeRel *identity.ReleaseID
		if manRow.BeforeReleaseID.Valid {
			bID, _ := identity.ReleaseIDFromUUIDBytes(manRow.BeforeReleaseID.Bytes)
			beforeRel = &bID
		}
		man = &domain.ProductionReleaseManifest{
			WorkspaceID:          workspace,
			ReleaseID:            releaseID,
			BeforeReleaseID:      beforeRel,
			BeforeManifestJSON:   manRow.BeforeManifestJson,
			BeforeManifestDigest: manRow.BeforeManifestDigest,
			AfterManifestDigest:  manRow.AfterManifestDigest,
			AttributionDigest:    manRow.AttributionDigest,
			CreatedAt:            manRow.CreatedAt.Time.UTC(),
		}
	}

	pinRows, err := s.queries.ListProductionReleaseBeforePins(ctx, dbgen.ListProductionReleaseBeforePinsParams{
		WorkspaceID: wspUUID,
		ReleaseID:   relUUID,
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var beforePins []domain.ProductionReleaseBeforePin
	if err == nil {
		for _, pr := range pinRows {
			targetID, err := productionTargetID(pr.TargetKind, pr.TargetID)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			var revID *identity.RevisionID
			if pr.AssetRevisionID.Valid {
				rID, _ := identity.RevisionIDFromUUIDBytes(pr.AssetRevisionID.Bytes)
				revID = &rID
			}
			var objVer *int
			if pr.ObjectVersion.Valid {
				v := int(pr.ObjectVersion.Int32)
				objVer = &v
			}
			var cDigest *string
			if pr.ContentDigest.Valid {
				cDigest = &pr.ContentDigest.String
			}
			beforePins = append(beforePins, domain.ProductionReleaseBeforePin{
				WorkspaceID:   workspace,
				ReleaseID:     releaseID,
				TargetKind:    pr.TargetKind,
				TargetID:      targetID,
				Presence:      pr.Presence,
				RevisionID:    revID,
				ObjectVersion: objVer,
				ContentDigest: cDigest,
				CreatedAt:     pr.CreatedAt.Time.UTC(),
			})
		}
	}

	propRows, err := s.queries.ListReleaseProposals(ctx, dbgen.ListReleaseProposalsParams{
		WorkspaceID: wspUUID,
		ReleaseID:   relUUID,
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var releaseProposals []domain.ReleaseProposal
	if err == nil {
		for _, rpr := range propRows {
			pID, _ := identity.ProposalIDFromUUIDBytes(rpr.ProposalID.Bytes)
			opID, _ := identity.ProductionOperationIDFromUUIDBytes(rpr.OperationID.Bytes)
			authorID, _ := identity.PrincipalIDFromUUIDBytes(rpr.AuthorPrincipalID.Bytes)
			releaseProposals = append(releaseProposals, domain.ReleaseProposal{
				WorkspaceID:           workspace,
				ReleaseID:             releaseID,
				ProposalID:            pID,
				OperationID:           opID,
				ProductionVersion:     int(rpr.ProductionVersion),
				SetDigest:             rpr.SetDigest,
				ProposalContentDigest: rpr.ProposalContentDigest,
				AuthorPrincipalID:     authorID,
				AttemptNo:             int(rpr.AttemptNo),
				ValidationDigest:      rpr.ValidationDigest,
				ReviewIDs:             rpr.ReviewIds,
				Role:                  rpr.Role,
				CreatedAt:             rpr.CreatedAt.Time.UTC(),
			})
		}
	}

	rel.Entries, err = s.ListReleaseAssets(ctx, workspace, releaseID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	rel.Objects, err = s.ListReleaseObjects(ctx, workspace, releaseID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return &rel, man, beforePins, releaseProposals, nil
}
