package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) productionAssetRevisionTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, target domain.ProductionTarget, now time.Time) (domain.ManifestEntry, error) {
	entry := domain.ManifestEntry{Compatibility: json.RawMessage(`{}`)}
	asset, err := identity.ParseAssetID(target.TargetID)
	if err != nil {
		return entry, err
	}
	var current pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT current_revision_id FROM semantic_assets WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, ver.WorkspaceID.UUID(), asset.UUID()).Scan(&current); err != nil {
		return entry, err
	}
	if target.Intent == domain.ProductionIntentCreate {
		if current.Valid {
			return entry, domain.ErrBaselineConflict
		}
	} else {
		if target.BaseRevisionID == nil {
			return entry, domain.ErrBaselineConflict
		}
		base, err := identity.ParseRevisionID(*target.BaseRevisionID)
		if err != nil || formatUUID(current) != base.UUID() {
			return entry, domain.ErrBaselineConflict
		}
	}
	content, err := domain.CanonicalJSON(target.ContentJSON)
	if err != nil {
		return entry, err
	}
	digest, err := domain.DigestJSON(content)
	if err != nil {
		return entry, err
	}
	if digest != target.ContentDigest {
		return entry, domain.ErrContentMismatch
	}
	revision, err := identity.NewRevisionID()
	if err != nil {
		return entry, err
	}
	workspace, _ := uuidFromString(ver.WorkspaceID.UUID())
	assetUUID, _ := uuidFromString(asset.UUID())
	revisionUUID, _ := uuidFromString(revision.UUID())
	q := s.queries.WithTx(tx)
	sequence, err := q.NextAssetRevisionSequence(ctx, dbgen.NextAssetRevisionSequenceParams{WorkspaceID: workspace, AssetID: assetUUID})
	if err != nil {
		return entry, err
	}
	var existing pgtype.UUID
	lookupErr := tx.QueryRow(ctx, `SELECT id,sequence FROM asset_revisions WHERE workspace_id=$1 AND asset_id=$2 AND content_digest=$3`, workspace, assetUUID, digest).Scan(&existing, &sequence)
	if lookupErr == nil {
		revision, err = identity.RevisionIDFromUUIDBytes(existing.Bytes)
		if err != nil {
			return entry, err
		}
	} else {
		if !errors.Is(lookupErr, pgx.ErrNoRows) {
			return entry, lookupErr
		}
		if _, err := q.CreateAssetRevision(ctx, dbgen.CreateAssetRevisionParams{ID: revisionUUID, WorkspaceID: workspace, AssetID: assetUUID, Sequence: sequence, SchemaVersion: "1.0.0", ContentDigest: digest, Content: content, CreatedBy: ver.CreatedBy.String()}); err != nil {
			return entry, err
		}
	}
	updated, err := tx.Exec(ctx, `UPDATE semantic_assets SET current_revision_id=$3,lifecycle_state='active',updated_at=$4 WHERE workspace_id=$1 AND id=$2 AND current_revision_id IS NOT DISTINCT FROM $5`, ver.WorkspaceID.UUID(), asset.UUID(), revision.UUID(), now, current)
	if err != nil {
		return entry, err
	}
	if updated.RowsAffected() != 1 {
		return entry, domain.ErrBaselineConflict
	}
	audit, err := identity.NewEventID()
	if err != nil {
		return entry, err
	}
	outbox, err := identity.NewEventID()
	if err != nil {
		return entry, err
	}
	trace := ver.TraceID
	if trace == "" {
		trace = strings.ReplaceAll(audit.UUID(), "-", "")
	}
	if err := createCatalogMutationEvents(ctx, q, catalogEvent{WorkspaceID: ver.WorkspaceID, AssetID: asset, RevisionID: revision, AuditID: audit, OutboxID: outbox, Sequence: sequence, Action: "revision.created", Actor: ver.CreatedBy.String(), TraceID: trace, CreatedAt: now}); err != nil {
		return entry, err
	}
	entry.AssetID, entry.RevisionID = asset, revision
	return entry, nil
}

// Only these fixed structural columns are writable. The complete canonical
// business document is retained alongside them for exact release snapshots.
func productionObjectColumns(kind string, content json.RawMessage) (string, []string, []any, error) {
	var value struct {
		Asset      string          `json:"asset"`
		Dataset    string          `json:"dataset"`
		Field      *string         `json:"field"`
		Transform  *string         `json:"transform"`
		Expression string          `json:"expression"`
		Fields     json.RawMessage `json:"fields"`
		Uniqueness string          `json:"uniqueness"`
		Left       string          `json:"leftDataset"`
		Right      string          `json:"rightDataset"`
		Pairs      []struct {
			Left  string `json:"left"`
			Right string `json:"right"`
		} `json:"pairs"`
		JoinType    string  `json:"joinType"`
		Cardinality string  `json:"cardinality"`
		Notes       *string `json:"notes"`
	}
	if err := json.Unmarshal(content, &value); err != nil {
		return "", nil, nil, err
	}
	uuid := func(raw string) (any, error) {
		id, err := identity.ParseAny(raw)
		if err != nil {
			return nil, err
		}
		return id.UUID(), nil
	}
	asset, err := uuid(value.Asset)
	if kind != domain.TargetKindJoinContract && err != nil {
		return "", nil, nil, err
	}
	switch kind {
	case domain.TargetKindPhysicalBinding:
		dataset, err := uuid(value.Dataset)
		if err != nil {
			return "", nil, nil, err
		}
		var field any
		if value.Field != nil {
			field, err = uuid(*value.Field)
			if err != nil {
				return "", nil, nil, err
			}
		}
		return "physical_bindings", []string{"asset_id", "dataset_id", "field_id", "transform"}, []any{asset, dataset, field, value.Transform}, nil
	case domain.TargetKindModelGrain:
		return "model_grains", []string{"asset_id", "grain_expression", "grain_field_refs"}, []any{asset, value.Expression, value.Fields}, nil
	case domain.TargetKindEntityKey:
		return "entity_keys", []string{"asset_id", "key_field_refs", "uniqueness_semantics"}, []any{asset, value.Fields, value.Uniqueness}, nil
	case domain.TargetKindJoinContract:
		left, err := uuid(value.Left)
		if err != nil {
			return "", nil, nil, err
		}
		right, err := uuid(value.Right)
		if err != nil {
			return "", nil, nil, err
		}
		leftFields, rightFields := []string{}, []string{}
		for _, pair := range value.Pairs {
			leftFields = append(leftFields, pair.Left)
			rightFields = append(rightFields, pair.Right)
		}
		l, err := json.Marshal(leftFields)
		if err != nil {
			return "", nil, nil, err
		}
		r, err := json.Marshal(rightFields)
		if err != nil {
			return "", nil, nil, err
		}
		return "join_contracts", []string{"left_dataset_id", "right_dataset_id", "left_field_refs", "right_field_refs", "join_type", "cardinality", "join_expression", "contract_notes"}, []any{left, right, l, r, value.JoinType, value.Cardinality, value.Expression, value.Notes}, nil
	default:
		return "", nil, nil, domain.ErrInvalidArgument
	}
}

func (s *Store) productionObjectWriteTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, target domain.ProductionTarget, now time.Time) (domain.ObjectManifestEntry, error) {
	entry := domain.ObjectManifestEntry{ObjectType: domain.TargetObjectType(target.Kind)}
	id, err := parseUUIDOrTypeID(target.TargetID)
	if err != nil {
		return entry, err
	}
	entry.ObjectID = formatUUID(id)
	table, columns, values, err := productionObjectColumns(target.Kind, target.ContentJSON)
	if err != nil {
		return entry, err
	}
	digest, err := domain.DigestJSON(target.ContentJSON)
	if err != nil || digest != target.ContentDigest {
		return entry, domain.ErrContentMismatch
	}
	args := []any{ver.WorkspaceID.UUID(), id}
	args = append(args, values...)
	columns = append(columns, "content", "updated_at")
	args = append(args, target.ContentJSON, now)
	if target.Intent == domain.ProductionIntentCreate && (target.Declaration == nil || target.Declaration.ReuseIdentity == nil) {
		columns = append(columns, "created_by", "created_at")
		args = append(args, ver.CreatedBy.String(), now)
		params := []string{}
		for i := range args {
			params = append(params, fmt.Sprintf("$%d", i+1))
		}
		if err := tx.QueryRow(ctx, `INSERT INTO `+table+` (workspace_id,id,`+strings.Join(columns, ",")+`) VALUES (`+strings.Join(params, ",")+`) RETURNING version`, args...).Scan(&entry.Version); err != nil {
			return entry, governanceRepositoryError("create production object", err)
		}
	} else {
		if target.RegistryWriteVersion == nil {
			return entry, domain.ErrBaselineConflict
		}
		sets := []string{}
		for i, column := range columns {
			sets = append(sets, fmt.Sprintf("%s=$%d", column, i+3))
		}
		args = append(args, *target.RegistryWriteVersion)
		if err := tx.QueryRow(ctx, `UPDATE `+table+` SET `+strings.Join(sets, ",")+`,version=version+1 WHERE workspace_id=$1 AND id=$2 AND version=`+fmt.Sprintf("$%d", len(args))+` RETURNING version`, args...).Scan(&entry.Version); err != nil {
			return entry, governanceRepositoryError("update production object", err)
		}
	}
	if err := s.productionObjectAuditTx(ctx, tx, ver.WorkspaceID, target.Kind, target.TargetID, "published", entry.Version, ver.CreatedBy.String(), ver.TraceID, now); err != nil {
		return entry, err
	}
	return entry, nil
}
