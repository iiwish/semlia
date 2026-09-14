package postgres

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) persistProductionReleaseTx(ctx context.Context, tx pgx.Tx, release domain.Release, manifest domain.ProductionReleaseManifest, pins []domain.ProductionReleaseBeforePin, attributions []domain.ReleaseProposal, cmd domain.ProductionCommand, trace string) error {
	q := s.queries.WithTx(tx)
	w, _ := uuidFromString(release.WorkspaceID.UUID())
	r, _ := uuidFromString(release.ID.UUID())
	op, _ := uuidFromString(cmd.OperationID.UUID())
	actor, _ := uuidFromString(cmd.PrincipalID.UUID())
	sequence, err := q.NextReleaseSequence(ctx, w)
	if err != nil {
		return err
	}
	var root, parent, rolled pgtype.UUID
	if release.ProductionRootReleaseID == nil || release.ProductionRollbackDepth == nil {
		return domain.ErrInvalidArgument
	}
	root, _ = uuidFromString(release.ProductionRootReleaseID.UUID())
	if release.ProductionRollbackParentID != nil {
		parent, _ = uuidFromString(release.ProductionRollbackParentID.UUID())
	}
	if release.RolledBackToReleaseID != nil {
		rolled, _ = uuidFromString(release.RolledBackToReleaseID.UUID())
	}
	if _, err := q.CreateProductionRelease(ctx, dbgen.CreateProductionReleaseParams{ID: r, WorkspaceID: w, Sequence: sequence, ManifestDigest: release.ManifestDigest, State: string(release.State), RolledBackToReleaseID: rolled, PublishedBy: release.PublishedBy, PublishedAt: timestamp(release.PublishedAt), CreatedAt: timestamp(release.CreatedAt), ProductionRootReleaseID: root, ProductionRollbackParentID: parent, ProductionRollbackDepth: pgtype.Int4{Int32: int32(*release.ProductionRollbackDepth), Valid: true}}); err != nil {
		return err
	}
	for _, rp := range attributions {
		proposal, _ := uuidFromString(rp.ProposalID.UUID())
		author, _ := uuidFromString(rp.AuthorPrincipalID.UUID())
		if err := q.InsertReleaseProposal(ctx, dbgen.InsertReleaseProposalParams{WorkspaceID: w, ReleaseID: r, ProposalID: proposal, OperationID: op, ProductionVersion: int32(rp.ProductionVersion), SetDigest: rp.SetDigest, ProposalContentDigest: rp.ProposalContentDigest, AuthorPrincipalID: author, AttemptNo: int32(rp.AttemptNo), ValidationDigest: rp.ValidationDigest, ReviewIds: rp.ReviewIDs, Role: rp.Role, CreatedAt: timestamp(release.CreatedAt)}); err != nil {
			return err
		}
	}
	var before pgtype.UUID
	if manifest.BeforeReleaseID != nil {
		before, _ = uuidFromString(manifest.BeforeReleaseID.UUID())
	}
	if err := q.InsertProductionReleaseManifest(ctx, dbgen.InsertProductionReleaseManifestParams{WorkspaceID: w, ReleaseID: r, BeforeReleaseID: before, BeforeManifestJson: manifest.BeforeManifestJSON, BeforeManifestDigest: manifest.BeforeManifestDigest, AfterManifestDigest: manifest.AfterManifestDigest, AttributionDigest: manifest.AttributionDigest, CreatedAt: timestamp(release.CreatedAt)}); err != nil {
		return err
	}
	for _, pin := range pins {
		target, err := parseUUIDOrTypeID(pin.TargetID)
		if err != nil {
			return err
		}
		var revision pgtype.UUID
		var version pgtype.Int4
		var digest pgtype.Text
		if pin.RevisionID != nil {
			revision, _ = uuidFromString(pin.RevisionID.UUID())
		}
		if pin.ObjectVersion != nil {
			version = pgtype.Int4{Int32: int32(*pin.ObjectVersion), Valid: true}
		}
		if pin.ContentDigest != nil {
			digest = pgtype.Text{String: *pin.ContentDigest, Valid: true}
		}
		if err := q.InsertProductionReleaseBeforePin(ctx, dbgen.InsertProductionReleaseBeforePinParams{WorkspaceID: w, ReleaseID: r, TargetKind: pin.TargetKind, TargetID: target, Presence: pin.Presence, AssetRevisionID: revision, ObjectVersion: version, ContentDigest: digest, CreatedAt: timestamp(release.CreatedAt)}); err != nil {
			return err
		}
	}
	for _, entry := range release.Entries {
		if err := s.persistAssetEntry(ctx, q, release.WorkspaceID, release.ID, entry, release.CreatedAt); err != nil {
			return err
		}
	}
	for _, entry := range release.Objects {
		if err := s.persistObjectEntry(ctx, q, release.WorkspaceID, release.ID, entry, release.CreatedAt); err != nil {
			return err
		}
	}
	if err := q.InsertProductionCommand(ctx, dbgen.InsertProductionCommandParams{WorkspaceID: w, PrincipalID: actor, CommandKind: cmd.CommandKind, IdempotencyKey: cmd.IdempotencyKey, RequestDigest: cmd.RequestDigest, OperationID: op, OperationVersion: int32(cmd.OperationVersion), ResultKind: "release", ResultID: r, ResponseJson: cmd.ResponseJSON, CommittedAt: timestamp(cmd.CommittedAt)}); err != nil {
		return governanceRepositoryError("record release command", err)
	}
	beforeAssets, beforeObjects, err := domain.ParseProductionManifestJSON(manifest.BeforeManifestJSON)
	if err != nil {
		return err
	}
	beforePayload, err := domain.ManifestDigestPayloadWithObjects(beforeAssets, beforeObjects)
	if err != nil {
		return err
	}
	afterPayload, err := domain.ManifestDigestPayloadWithObjects(release.Entries, release.Objects)
	if err != nil {
		return err
	}
	beforePayload, err = domain.CanonicalJSON(beforePayload)
	if err != nil {
		return err
	}
	afterPayload, err = domain.CanonicalJSON(afterPayload)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO production_release_integrity(workspace_id,release_id,before_payload,after_payload) VALUES($1,$2,$3,$4)`, w, r, []byte(beforePayload), []byte(afterPayload)); err != nil {
		return err
	}
	audit, err := identity.NewEventID()
	if err != nil {
		return err
	}
	outbox, err := identity.NewEventID()
	if err != nil {
		return err
	}
	if trace == "" {
		trace = strings.ReplaceAll(audit.UUID(), "-", "")
	}
	action := "published"
	if release.RolledBackToReleaseID != nil {
		action = "rolled_back"
	}
	return createReleaseMutationEvents(ctx, q, releaseEvent{WorkspaceID: release.WorkspaceID, ReleaseID: release.ID, AuditID: audit, OutboxID: outbox, Action: action, ManifestDigest: release.ManifestDigest, Sequence: sequence, RolledBackToReleaseID: release.RolledBackToReleaseID, Actor: release.PublishedBy, TraceID: trace, CreatedAt: release.CreatedAt})
}

func (s *Store) productionBindingInputsTx(ctx context.Context, tx pgx.Tx, release, before domain.Release, targets []domain.ProductionTarget) error {
	if !before.ID.IsZero() {
		if _, err := tx.Exec(ctx, `INSERT INTO production_release_binding_inputs(workspace_id,release_id,binding_id,binding_version,snapshot_id,dataset_revision_id,field_revision_ids,created_at)
 SELECT b.workspace_id,$3,b.binding_id,b.binding_version,b.snapshot_id,b.dataset_revision_id,b.field_revision_ids,$4 FROM production_release_binding_inputs b
 JOIN release_objects o ON o.workspace_id=b.workspace_id AND o.release_id=$3 AND o.object_type='physical_binding' AND o.object_id=b.binding_id AND o.version=b.binding_version
 WHERE b.workspace_id=$1 AND b.release_id=$2`, release.WorkspaceID.UUID(), before.ID.UUID(), release.ID.UUID(), release.CreatedAt); err != nil {
			return err
		}
	}
	for _, target := range targets {
		if target.Kind != domain.TargetKindPhysicalBinding || target.ProposalID == nil {
			continue
		}
		if target.Declaration == nil {
			return domain.ErrPriorStateUnknown
		}
		refs, err := domain.InspectProductionContent(target.Kind, target.Declaration.Content)
		if err != nil {
			return err
		}
		var snapshot, dataset pgtype.UUID
		fields := []string{}
		for _, ref := range refs.Physical {
			if ref.Kind == "dataset" {
				snapshot, err = parseUUIDOrTypeID(ref.SnapshotID)
				if err != nil {
					return err
				}
				dataset, err = parseUUIDOrTypeID(ref.RevisionID)
				if err != nil {
					return err
				}
			} else {
				fields = append(fields, ref.RevisionID)
			}
		}
		if !snapshot.Valid || !dataset.Valid {
			return domain.ErrInputIncomplete
		}
		sort.Strings(fields)
		encoded, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		binding, err := parseUUIDOrTypeID(target.TargetID)
		if err != nil {
			return err
		}
		version := 0
		for _, entry := range release.Objects {
			if entry.ObjectType == domain.TargetPhysicalBinding && entry.ObjectID == formatUUID(binding) {
				version = entry.Version
			}
		}
		if version < 1 {
			return domain.ErrInvariant
		}
		if _, err := tx.Exec(ctx, `INSERT INTO production_release_binding_inputs(workspace_id,release_id,binding_id,binding_version,snapshot_id,dataset_revision_id,field_revision_ids,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, release.WorkspaceID.UUID(), release.ID.UUID(), binding, version, snapshot, dataset, encoded, release.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}
