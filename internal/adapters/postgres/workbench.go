package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	application "github.com/iiwish/semlia/internal/application/workbench"
	governancedomain "github.com/iiwish/semlia/internal/domain/governance"
	operationsdomain "github.com/iiwish/semlia/internal/domain/operations"
	domain "github.com/iiwish/semlia/internal/domain/workbench"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ application.Repository = (*Store)(nil)

func (store *Store) ListAttentionItems(ctx context.Context, query domain.ListQuery) ([]domain.Item, error) {
	parameters, err := attentionListParameters(query)
	if err != nil {
		return nil, err
	}
	rows, err := store.queries.ListAttentionItems(ctx, parameters)
	if err != nil {
		return nil, fmt.Errorf("list attention items: %w", err)
	}
	result := make([]domain.Item, 0, len(rows))
	for _, row := range rows {
		item, decodeErr := attentionItemFromRow(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result = append(result, item)
	}
	return result, nil
}

func (store *Store) CountAttentionItems(ctx context.Context, query domain.ListQuery) (domain.Counts, error) {
	parameters, err := attentionListParameters(query)
	if err != nil {
		return domain.Counts{}, err
	}
	row, err := store.queries.CountAttentionItems(ctx, dbgen.CountAttentionItemsParams{
		WorkspaceID: parameters.WorkspaceID, AllowedKinds: parameters.AllowedKinds,
		Kind: parameters.Kind, State: parameters.State, Priority: parameters.Priority, Risk: parameters.Risk,
		Search: parameters.Search, AccessGrants: parameters.AccessGrants, View: parameters.View,
		PrincipalID: parameters.PrincipalID,
	})
	if err != nil {
		return domain.Counts{}, fmt.Errorf("count attention items: %w", err)
	}
	return domain.Counts{Total: int(row.Total), Open: int(row.Open), InProgress: int(row.InProgress), Critical: int(row.Critical)}, nil
}

func attentionListParameters(query domain.ListQuery) (dbgen.ListAttentionItemsParams, error) {
	workspaceID, err := uuidValue(query.WorkspaceID)
	if err != nil {
		return dbgen.ListAttentionItemsParams{}, err
	}
	principalID, err := uuidValue(query.PrincipalID)
	if err != nil {
		return dbgen.ListAttentionItemsParams{}, err
	}
	grants, err := json.Marshal(query.AccessGrants)
	if err != nil {
		return dbgen.ListAttentionItemsParams{}, err
	}
	kinds := make([]string, 0, len(query.AllowedKinds))
	for _, kind := range query.AllowedKinds {
		kinds = append(kinds, string(kind))
	}
	result := dbgen.ListAttentionItemsParams{
		WorkspaceID: workspaceID, PrincipalID: principalID, AllowedKinds: kinds,
		Kind: string(query.Kind), State: string(query.State), Priority: string(query.Priority),
		Risk: string(query.Risk), Search: query.Search, AccessGrants: grants, View: string(query.View),
		Sort: string(query.Sort), PageLimit: int32(query.Limit),
	}
	if query.Cursor != nil {
		result.HasCursor = true
		result.CursorUpdatedAt = timestamp(query.Cursor.UpdatedAt)
		result.CursorID, err = uuidValue(query.Cursor.ID)
		if err != nil {
			return dbgen.ListAttentionItemsParams{}, err
		}
		result.CursorPriorityRank = int32(query.Cursor.PriorityRank)
		result.CursorDueAt = optionalTimestamp(query.Cursor.DueAt)
	}
	return result, nil
}

func (store *Store) GetAttentionItem(ctx context.Context, workspace identity.WorkspaceID, itemID identity.AttentionItemID) (domain.Item, error) {
	workspaceDBID, err := uuidValue(workspace)
	if err != nil {
		return domain.Item{}, err
	}
	itemDBID, err := uuidValue(itemID)
	if err != nil {
		return domain.Item{}, err
	}
	row, err := store.queries.GetAttentionItem(ctx, dbgen.GetAttentionItemParams{WorkspaceID: workspaceDBID, ItemID: itemDBID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Item{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Item{}, fmt.Errorf("get attention item: %w", err)
	}
	return attentionItemFromRow(row)
}

func (store *Store) ReviewedAttentionItemIDs(
	ctx context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID, itemIDs []identity.AttentionItemID,
) (map[string]struct{}, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, err
	}
	principalID, err := uuidValue(principal)
	if err != nil {
		return nil, err
	}
	ids := make([]pgtype.UUID, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		value, encodeErr := uuidValue(itemID)
		if encodeErr != nil {
			return nil, encodeErr
		}
		ids = append(ids, value)
	}
	rows, err := store.queries.ListReviewedAttentionItemIDs(ctx, dbgen.ListReviewedAttentionItemIDsParams{
		WorkspaceID: workspaceID, ItemIds: ids, PrincipalID: principalID,
	})
	if err != nil {
		return nil, fmt.Errorf("list reviewed attention items: %w", err)
	}
	result := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		id, decodeErr := identity.AttentionItemIDFromUUIDBytes(row.Bytes)
		if decodeErr != nil {
			return nil, decodeErr
		}
		result[id.String()] = struct{}{}
	}
	return result, nil
}

type attentionMutationReceipt struct {
	SpecVersion        string      `json:"specVersion"`
	IdempotencyKey     string      `json:"idempotencyKey"`
	RequestFingerprint string      `json:"requestFingerprint"`
	ActorPrincipalID   string      `json:"actorPrincipalId"`
	ObjectType         string      `json:"objectType"`
	ObjectID           string      `json:"objectId"`
	Result             domain.Item `json:"result"`
}

func (store *Store) UpdateAttentionItem(ctx context.Context, command domain.UpdateCommand) (domain.Item, error) {
	workspaceID, err := uuidValue(command.WorkspaceID)
	if err != nil {
		return domain.Item{}, err
	}
	itemID, err := uuidValue(command.ItemID)
	if err != nil {
		return domain.Item{}, err
	}
	auditID, err := uuidValue(command.AuditEventID)
	if err != nil {
		return domain.Item{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Item{}, fmt.Errorf("begin attention item update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, command.WorkspaceID.String()+"\x1fworkbench\x1f"+command.IdempotencyKey); err != nil {
		return domain.Item{}, fmt.Errorf("lock attention item idempotency: %w", err)
	}
	authorizationVersion, err := queries.LockAuthorizationWorkspace(ctx, workspaceID)
	if err != nil {
		return domain.Item{}, fmt.Errorf("lock workbench authorization version: %w", err)
	}
	if authorizationVersion != command.AuthorizationVersion {
		return domain.Item{}, domain.ErrNotFound
	}
	if err := queries.LockAttentionProjection(ctx); err != nil {
		return domain.Item{}, fmt.Errorf("lock attention projection: %w", err)
	}
	existing, err := queries.GetWorkbenchMutationAudit(ctx, dbgen.GetWorkbenchMutationAuditParams{
		WorkspaceID: workspaceID, IdempotencyKey: command.IdempotencyKey,
	})
	if err == nil {
		return replayAttentionMutation(existing, command)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Item{}, fmt.Errorf("load attention item idempotency receipt: %w", err)
	}
	locked, err := queries.LockAttentionItem(ctx, dbgen.LockAttentionItemParams{WorkspaceID: workspaceID, ItemID: itemID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Item{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Item{}, fmt.Errorf("lock attention item: %w", err)
	}
	if locked.Version != command.ExpectedVersion || locked.State == string(domain.StateResolved) {
		return domain.Item{}, domain.ErrConflict
	}
	lockedItem, err := attentionItemFromRow(locked)
	if err != nil {
		return domain.Item{}, err
	}
	if domain.VisibilityFingerprint(lockedItem) != command.VisibilityFingerprint {
		return domain.Item{}, domain.ErrNotFound
	}
	assignee, err := nullableUUIDValue(command.AssigneePrincipalID)
	if err != nil {
		return domain.Item{}, err
	}
	updated, err := queries.UpdateAttentionItem(ctx, dbgen.UpdateAttentionItemParams{
		SetAssignee: command.SetAssignee, AssigneePrincipalID: assignee, State: string(command.State),
		UpdatedAt: timestamp(command.UpdatedAt), WorkspaceID: workspaceID, ItemID: itemID,
		ExpectedVersion: command.ExpectedVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Item{}, domain.ErrConflict
	}
	if err != nil {
		return domain.Item{}, fmt.Errorf("update attention item: %w", err)
	}
	item, err := attentionItemFromRow(updated)
	if err != nil {
		return domain.Item{}, err
	}
	receipt := attentionMutationReceipt{SpecVersion: "semlia.workbench-mutation/v1",
		IdempotencyKey: command.IdempotencyKey, RequestFingerprint: command.RequestFingerprint,
		ActorPrincipalID: command.ActorPrincipalID.String(), ObjectType: "attention_item",
		ObjectID: command.ItemID.String(), Result: item}
	payload, err := json.Marshal(receipt)
	if err != nil {
		return domain.Item{}, err
	}
	if err := queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{ID: auditID, WorkspaceID: workspaceID,
		EventType: "workbench.attention_item.updated", ActorID: textValue(command.ActorPrincipalID.String()),
		Payload: payload, TraceID: command.TraceID, CreatedAt: timestamp(command.UpdatedAt)}); err != nil {
		return domain.Item{}, fmt.Errorf("persist attention item idempotency receipt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Item{}, fmt.Errorf("commit attention item update: %w", err)
	}
	return item, nil
}

func replayAttentionMutation(payload []byte, command domain.UpdateCommand) (domain.Item, error) {
	var receipt attentionMutationReceipt
	if err := json.Unmarshal(payload, &receipt); err != nil || receipt.SpecVersion != "semlia.workbench-mutation/v1" ||
		receipt.IdempotencyKey != command.IdempotencyKey || receipt.RequestFingerprint != command.RequestFingerprint ||
		receipt.ActorPrincipalID != command.ActorPrincipalID.String() || receipt.ObjectID != command.ItemID.String() {
		return domain.Item{}, domain.ErrConflict
	}
	if err := receipt.Result.Validate(); err != nil {
		return domain.Item{}, domain.ErrConflict
	}
	return receipt.Result, nil
}

type reconcileCandidate struct {
	workspaceID pgtype.UUID
	kind        domain.Kind
	dedupeKey   string
	priority    domain.Priority
	risk        domain.Priority
	targetType  string
	targetID    pgtype.UUID
	routeType   string
	routeID     pgtype.UUID
	audience    string
	initiator   string
	title       string
	summary     string
	reasonCode  string
	evidenceRef string
	traceID     string
	openedAt    time.Time
	updatedAt   time.Time
}

func (store *Store) ReconcileAttentionItems(ctx context.Context, limit int) (domain.ReconcileStats, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.ReconcileStats{}, fmt.Errorf("begin attention reconcile: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	if err := queries.LockAttentionProjection(ctx); err != nil {
		return domain.ReconcileStats{}, fmt.Errorf("lock attention projection: %w", err)
	}
	rows, err := tx.Query(ctx, reconcileCandidatesSQL, limit+1)
	if err != nil {
		return domain.ReconcileStats{}, fmt.Errorf("scan attention conditions: %w", err)
	}
	candidates := make([]reconcileCandidate, 0, limit+1)
	for rows.Next() {
		var value reconcileCandidate
		var kind, priority, risk string
		var openedAt, updatedAt pgtype.Timestamptz
		if err := rows.Scan(&value.workspaceID, &kind, &value.dedupeKey, &priority, &risk,
			&value.targetType, &value.targetID, &value.routeType, &value.routeID, &value.audience,
			&value.initiator, &value.title, &value.summary, &value.reasonCode, &value.evidenceRef,
			&value.traceID, &openedAt, &updatedAt); err != nil {
			rows.Close()
			return domain.ReconcileStats{}, fmt.Errorf("decode attention condition: %w", err)
		}
		value.kind, value.priority, value.risk = domain.Kind(kind), domain.Priority(priority), domain.Priority(risk)
		value.openedAt, value.updatedAt = openedAt.Time.UTC(), updatedAt.Time.UTC()
		candidates = append(candidates, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.ReconcileStats{}, fmt.Errorf("scan attention conditions: %w", err)
	}
	rows.Close()
	stats := domain.ReconcileStats{Scanned: len(candidates)}
	if len(candidates) > limit {
		stats.Scanned, stats.Truncated = limit, true
		candidates = candidates[:limit]
	}
	if err := populateCandidateTraces(ctx, tx, candidates); err != nil {
		return domain.ReconcileStats{}, err
	}
	active := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.traceID == "" {
			candidate.traceID = candidate.reconcileTraceID()
		}
		item, mapErr := candidate.item(ctx, queries)
		if mapErr != nil {
			return domain.ReconcileStats{}, fmt.Errorf("materialize attention condition: %w", mapErr)
		}
		if _, err := queries.UpsertAttentionItem(ctx, attentionUpsertParameters(item)); err != nil {
			return domain.ReconcileStats{}, fmt.Errorf("upsert attention item: %w", err)
		}
		active = append(active, candidate.workspaceString()+"\x1f"+candidate.dedupeKey)
		stats.Upserted++
	}
	if !stats.Truncated {
		command, err := tx.Exec(ctx, `
			UPDATE attention_items
			SET state='resolved', resolved_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP, version=version+1
			WHERE state IN ('open','in_progress') AND NOT (workspace_id::text || chr(31) || dedupe_key = ANY($1::text[]))`, active)
		if err != nil {
			return domain.ReconcileStats{}, fmt.Errorf("resolve stale attention items: %w", err)
		}
		stats.Resolved = int(command.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ReconcileStats{}, fmt.Errorf("commit attention reconcile: %w", err)
	}
	return stats, nil
}

func (candidate reconcileCandidate) workspaceString() string {
	return uuidString(candidate.workspaceID)
}

func (candidate reconcileCandidate) reconcileTraceID() string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"workbench-reconcile-v1", candidate.workspaceString(), candidate.routeType,
		candidate.routeID.String(), candidate.dedupeKey,
	}, "\x00")))
	return hex.EncodeToString(digest[:16])
}

func (candidate reconcileCandidate) item(ctx context.Context, queries *dbgen.Queries) (domain.Item, error) {
	workspace, err := identity.WorkspaceIDFromUUIDBytes(candidate.workspaceID.Bytes)
	if err != nil {
		return domain.Item{}, err
	}
	id, err := identity.NewAttentionItemID()
	if err != nil {
		return domain.Item{}, err
	}
	targetType := candidate.targetType
	var targetID string
	if candidate.routeType == "proposal" {
		proposal, loadErr := queries.GetProposal(ctx, dbgen.GetProposalParams{
			WorkspaceID: candidate.workspaceID, ProposalID: candidate.routeID,
		})
		if loadErr != nil {
			return domain.Item{}, loadErr
		}
		targetType, targetID, err = proposalAttentionTarget(ctx, queries, proposal)
	} else {
		targetID, err = publicReconcileID(candidate.targetType, candidate.targetID)
	}
	if err != nil {
		return domain.Item{}, err
	}
	routeID, err := publicReconcileID(candidate.routeType, candidate.routeID)
	if err != nil {
		return domain.Item{}, err
	}
	var initiator *identity.PrincipalID
	if candidate.initiator != "" {
		value, decodeErr := identity.ParsePrincipalID(candidate.initiator)
		if decodeErr != nil {
			if converted, convertErr := identity.FromUUID(identity.Principal, candidate.initiator); convertErr == nil {
				value, decodeErr = identity.ParsePrincipalID(converted.String())
			}
		}
		if decodeErr == nil {
			initiator = &value
		}
	}
	route, err := candidate.route(routeID, targetID)
	if err != nil {
		return domain.Item{}, err
	}
	item := domain.Item{ID: id, WorkspaceID: workspace, Kind: candidate.kind, DedupeKey: candidate.dedupeKey,
		State: domain.StateOpen, Priority: candidate.priority, Risk: candidate.risk,
		TargetType: targetType, TargetID: targetID, TargetRoute: route,
		AudienceRoleID: candidate.audience, InitiatorPrincipalID: initiator,
		RuleVersion: "workbench.conditions.v1", Title: candidate.title, Summary: candidate.summary,
		ReasonCode: candidate.reasonCode, EvidenceRef: candidate.evidenceRef, TraceID: candidate.traceID,
		OpenedAt: candidate.openedAt, UpdatedAt: candidate.updatedAt, Version: 1}
	return item, item.Validate()
}

func attentionUpsertParameters(item domain.Item) dbgen.UpsertAttentionItemParams {
	itemID, _ := uuidValue(item.ID)
	workspaceID, _ := uuidValue(item.WorkspaceID)
	assignee, _ := nullableUUIDValue(item.AssigneePrincipalID)
	initiator, _ := nullableUUIDValue(item.InitiatorPrincipalID)
	return dbgen.UpsertAttentionItemParams{ID: itemID, WorkspaceID: workspaceID, Kind: string(item.Kind),
		DedupeKey: item.DedupeKey, Priority: string(item.Priority), Risk: string(item.Risk),
		TargetType: item.TargetType, TargetID: item.TargetID, TargetRoute: item.TargetRoute,
		AssigneePrincipalID: assignee, AudienceRoleID: optionalTextValue(item.AudienceRoleID),
		InitiatorPrincipalID: initiator, RuleVersion: item.RuleVersion, Title: item.Title,
		Summary: item.Summary, ReasonCode: item.ReasonCode, EvidenceRef: optionalTextValue(item.EvidenceRef),
		TraceID: item.TraceID, OpenedAt: timestamp(item.OpenedAt), DueAt: optionalTimestamp(item.DueAt),
		UpdatedAt: timestamp(item.UpdatedAt)}
}

func attentionItemFromRow(row dbgen.AttentionItem) (domain.Item, error) {
	id, err := identity.AttentionItemIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.Item{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return domain.Item{}, err
	}
	var assignee, initiator *identity.PrincipalID
	if row.AssigneePrincipalID.Valid {
		value, decodeErr := identity.PrincipalIDFromUUIDBytes(row.AssigneePrincipalID.Bytes)
		if decodeErr != nil {
			return domain.Item{}, decodeErr
		}
		assignee = &value
	}
	if row.InitiatorPrincipalID.Valid {
		value, decodeErr := identity.PrincipalIDFromUUIDBytes(row.InitiatorPrincipalID.Bytes)
		if decodeErr != nil {
			return domain.Item{}, decodeErr
		}
		initiator = &value
	}
	item := domain.Item{ID: id, WorkspaceID: workspaceID, Kind: domain.Kind(row.Kind), DedupeKey: row.DedupeKey,
		State: domain.State(row.State), Priority: domain.Priority(row.Priority), Risk: domain.Priority(row.Risk),
		TargetType: row.TargetType, TargetID: row.TargetID, TargetRoute: row.TargetRoute,
		AssigneePrincipalID: assignee, AudienceRoleID: optionalText(row.AudienceRoleID), InitiatorPrincipalID: initiator,
		RuleVersion: row.RuleVersion, Title: row.Title, Summary: row.Summary, ReasonCode: row.ReasonCode,
		EvidenceRef: optionalText(row.EvidenceRef), TraceID: row.TraceID, OpenedAt: row.OpenedAt.Time.UTC(),
		DueAt: optionalTime(row.DueAt), ResolvedAt: optionalTime(row.ResolvedAt), UpdatedAt: row.UpdatedAt.Time.UTC(),
		Version: row.Version}
	return item, item.Validate()
}

func publicReconcileID(kind string, value pgtype.UUID) (string, error) {
	if !value.Valid {
		return "", domain.ErrInvalidArgument
	}
	var id identity.ID
	var err error
	switch kind {
	case "workspace":
		id, err = identity.FromUUIDBytes(identity.Workspace, value.Bytes)
	case "asset":
		id, err = identity.FromUUIDBytes(identity.Asset, value.Bytes)
	case "proposal":
		id, err = identity.FromUUIDBytes(identity.Proposal, value.Bytes)
	case "validation_run":
		id, err = identity.FromUUIDBytes(identity.ValidationRun, value.Bytes)
	case "source":
		id, err = identity.FromUUIDBytes(identity.SourceConnection, value.Bytes)
	case "runtime_run":
		id, err = identity.FromUUIDBytes(identity.Run, value.Bytes)
	case "consumer":
		id, err = identity.FromUUIDBytes(identity.Consumer, value.Bytes)
	case "semantic_query":
		id, err = identity.FromUUIDBytes(identity.SemanticQuery, value.Bytes)
	case "model_grain":
		id, err = identity.FromUUIDBytes(identity.ModelGrain, value.Bytes)
	case "physical_binding":
		id, err = identity.FromUUIDBytes(identity.PhysicalBinding, value.Bytes)
	case "entity_key":
		id, err = identity.FromUUIDBytes(identity.EntityKey, value.Bytes)
	case "join_contract":
		id, err = identity.FromUUIDBytes(identity.JoinContract, value.Bytes)
	default:
		return "", domain.ErrInvalidArgument
	}
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func (candidate reconcileCandidate) route(routeID, targetID string) (string, error) {
	switch candidate.routeType {
	case "proposal":
		return "/governance?proposal=" + routeID, nil
	case "source":
		return "/sources?source=" + routeID, nil
	case "runtime_run":
		return "/operations/runtime?run=" + routeID, nil
	case "semantic_query":
		bindingUUID := strings.TrimPrefix(candidate.dedupeKey, "compatibility:")
		binding, err := identity.FromUUID(identity.ConsumerBinding, bindingUUID)
		if err != nil {
			return "", err
		}
		return compatibilityRoute(targetID, binding.String(), routeID), nil
	default:
		return "/workbench", nil
	}
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	workspace, err := identity.WorkspaceIDFromUUIDBytes(value.Bytes)
	if err != nil {
		return ""
	}
	return workspace.UUID()
}

func populateCandidateTraces(ctx context.Context, tx pgx.Tx, candidates []reconcileCandidate) error {
	targets := make([]string, 0, len(candidates))
	positions := make(map[string][]int)
	for index, candidate := range candidates {
		if candidate.traceID != "" {
			continue
		}
		target, err := publicReconcileID(candidate.routeType, candidate.routeID)
		if err != nil {
			continue
		}
		if _, ok := positions[target]; !ok {
			targets = append(targets, target)
		}
		positions[target] = append(positions[target], index)
	}
	if len(targets) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (target.object_id) target.object_id, event.trace_id
		FROM audit_event_targets target
		JOIN audit_events event ON event.workspace_id=target.workspace_id AND event.id=target.audit_event_id
		WHERE target.object_id=ANY($1::text[])
		ORDER BY target.object_id, event.created_at DESC, event.id DESC`, targets)
	if err != nil {
		return fmt.Errorf("load attention condition traces: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var target, traceID string
		if err := rows.Scan(&target, &traceID); err != nil {
			return fmt.Errorf("decode attention condition trace: %w", err)
		}
		for _, position := range positions[target] {
			candidates[position].traceID = traceID
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load attention condition traces: %w", err)
	}
	return nil
}

func projectProposalAttention(ctx context.Context, queries *dbgen.Queries, proposal dbgen.Proposal, traceID string, now time.Time) error {
	if err := queries.LockAttentionProjection(ctx); err != nil {
		return err
	}
	workspace, err := identity.WorkspaceIDFromUUIDBytes(proposal.WorkspaceID.Bytes)
	if err != nil {
		return err
	}
	proposalID, err := identity.ProposalIDFromUUIDBytes(proposal.ID.Bytes)
	if err != nil {
		return err
	}
	dedupe := "review:" + proposal.ID.String()
	if proposal.State != string(governancedomain.ProposalInReview) {
		_, err := queries.ResolveAttentionItemByDedupe(ctx, dbgen.ResolveAttentionItemByDedupeParams{
			WorkspaceID: proposal.WorkspaceID, DedupeKey: dedupe, ResolvedAt: timestamp(now)})
		return err
	}
	if traceID == "" {
		traceID, _ = queries.GetLatestAuditTraceForObject(ctx, dbgen.GetLatestAuditTraceForObjectParams{
			WorkspaceID: proposal.WorkspaceID, ObjectID: proposalID.String()})
	}
	if traceID == "" {
		return nil
	}
	targetType, targetID, err := proposalAttentionTarget(ctx, queries, proposal)
	if err != nil {
		return err
	}
	priority := domain.PriorityMedium
	if optionalText(proposal.RiskLevel) == "high" {
		priority = domain.PriorityHigh
	}
	initiator := principalIDPointer(proposal.CreatedBy)
	itemID, err := identity.NewAttentionItemID()
	if err != nil {
		return err
	}
	summary := proposal.Summary
	if summary == "" {
		summary = "Proposal requires independent review."
	}
	item := domain.Item{ID: itemID, WorkspaceID: workspace, Kind: domain.KindReview, DedupeKey: dedupe,
		State: domain.StateOpen, Priority: priority, Risk: priority, TargetType: targetType, TargetID: targetID,
		TargetRoute: "/governance?proposal=" + proposalID.String(), AudienceRoleID: "reviewer",
		InitiatorPrincipalID: initiator, RuleVersion: "workbench.conditions.v1", Title: boundedText(proposal.Title, 160),
		Summary: boundedText(summary, 512), ReasonCode: "PROPOSAL_REVIEW_REQUIRED", TraceID: traceID,
		OpenedAt: proposal.UpdatedAt.Time.UTC(), UpdatedAt: now.UTC(), Version: 1}
	if err := item.Validate(); err != nil {
		return err
	}
	_, err = queries.UpsertAttentionItem(ctx, attentionUpsertParameters(item))
	return err
}

func projectValidationAttention(ctx context.Context, queries *dbgen.Queries, run dbgen.ValidationRun, now time.Time) error {
	if err := queries.LockAttentionProjection(ctx); err != nil {
		return err
	}
	proposal, err := queries.GetProposal(ctx, dbgen.GetProposalParams{WorkspaceID: run.WorkspaceID, ProposalID: run.ProposalID})
	if err != nil {
		return err
	}
	workspace, err := identity.WorkspaceIDFromUUIDBytes(run.WorkspaceID.Bytes)
	if err != nil {
		return err
	}
	proposalID, err := identity.ProposalIDFromUUIDBytes(run.ProposalID.Bytes)
	if err != nil {
		return err
	}
	dedupe := "validation:" + run.ProposalID.String()
	if run.Status != string(governancedomain.ValidationFailed) {
		_, err := queries.ResolveAttentionItemByDedupe(ctx, dbgen.ResolveAttentionItemByDedupeParams{
			WorkspaceID: run.WorkspaceID, DedupeKey: dedupe, ResolvedAt: timestamp(now)})
		return err
	}
	traceID, _ := queries.GetLatestAuditTraceForObject(ctx, dbgen.GetLatestAuditTraceForObjectParams{
		WorkspaceID: run.WorkspaceID, ObjectID: proposalID.String()})
	if traceID == "" {
		return nil
	}
	targetType, targetID, err := proposalAttentionTarget(ctx, queries, proposal)
	if err != nil {
		return err
	}
	itemID, err := identity.NewAttentionItemID()
	if err != nil {
		return err
	}
	item := domain.Item{ID: itemID, WorkspaceID: workspace, Kind: domain.KindValidation, DedupeKey: dedupe,
		State: domain.StateOpen, Priority: domain.PriorityHigh, Risk: domain.PriorityHigh,
		TargetType: targetType, TargetID: targetID, TargetRoute: "/governance?proposal=" + proposalID.String(),
		AudienceRoleID: "semantic_steward", InitiatorPrincipalID: principalIDPointer(proposal.CreatedBy),
		RuleVersion: "workbench.conditions.v1", Title: boundedText(proposal.Title, 160),
		Summary: "Validation failed and requires attention.", ReasonCode: "VALIDATION_FAILED", TraceID: traceID,
		OpenedAt: run.StartedAt.Time.UTC(), UpdatedAt: now.UTC(), Version: 1}
	if err := item.Validate(); err != nil {
		return err
	}
	_, err = queries.UpsertAttentionItem(ctx, attentionUpsertParameters(item))
	return err
}

func projectDiscoveryAttention(ctx context.Context, queries *dbgen.Queries, run dbgen.DiscoveryRun, now time.Time) error {
	if err := queries.LockAttentionProjection(ctx); err != nil {
		return err
	}
	workspace, err := identity.WorkspaceIDFromUUIDBytes(run.WorkspaceID.Bytes)
	if err != nil {
		return err
	}
	sourceID, err := identity.SourceConnectionIDFromUUIDBytes(run.SourceConnectionID.Bytes)
	if err != nil {
		return err
	}
	dedupe := "source:" + run.SourceConnectionID.String()
	if run.Status != "failed" && run.Status != "degraded" {
		_, err := queries.ResolveAttentionItemByDedupe(ctx, dbgen.ResolveAttentionItemByDedupeParams{
			WorkspaceID: run.WorkspaceID, DedupeKey: dedupe, ResolvedAt: timestamp(now)})
		return err
	}
	traceID := optionalText(run.TraceID)
	if traceID == "" {
		return nil
	}
	priority, reason, summary := domain.PriorityHigh, "DISCOVERY_DEGRADED", "Discovery completed with degraded results."
	if run.Status == "failed" {
		priority, reason, summary = domain.PriorityCritical, "DISCOVERY_FAILED", "Discovery failed."
	}
	itemID, err := identity.NewAttentionItemID()
	if err != nil {
		return err
	}
	item := domain.Item{ID: itemID, WorkspaceID: workspace, Kind: domain.KindSource, DedupeKey: dedupe,
		State: domain.StateOpen, Priority: priority, Risk: priority, TargetType: "source", TargetID: sourceID.String(),
		TargetRoute: "/sources?source=" + sourceID.String(), AudienceRoleID: "source_operator",
		InitiatorPrincipalID: principalIDPointer(optionalText(run.RequestedBy)), RuleVersion: "workbench.conditions.v1",
		Title: "Source discovery needs attention", Summary: summary, ReasonCode: reason, TraceID: traceID,
		OpenedAt: run.CreatedAt.Time.UTC(), UpdatedAt: now.UTC(), Version: 1}
	if err := item.Validate(); err != nil {
		return err
	}
	_, err = queries.UpsertAttentionItem(ctx, attentionUpsertParameters(item))
	return err
}

func projectRuntimeAttention(ctx context.Context, queries *dbgen.Queries, run operationsdomain.RuntimeRun) error {
	if run.Kind == operationsdomain.RunKindDiscovery || run.Kind == operationsdomain.RunKindValidation {
		return nil
	}
	if err := queries.LockAttentionProjection(ctx); err != nil {
		return err
	}
	workspaceID, err := uuidValue(run.WorkspaceID)
	if err != nil {
		return err
	}
	dedupe := "runtime:" + run.ID.UUID()
	if run.State != operationsdomain.RunFailed && run.State != operationsdomain.RunDeadLetter {
		_, err := queries.ResolveAttentionItemByDedupe(ctx, dbgen.ResolveAttentionItemByDedupeParams{
			WorkspaceID: workspaceID, DedupeKey: dedupe, ResolvedAt: timestamp(run.UpdatedAt)})
		return err
	}
	if run.TraceID == "" {
		return nil
	}
	itemID, err := identity.NewAttentionItemID()
	if err != nil {
		return err
	}
	reason := "RUNTIME_FAILED"
	if run.State == operationsdomain.RunDeadLetter {
		reason = "RUNTIME_DEAD_LETTER"
	}
	summary := run.ErrorSummary
	if summary == "" {
		summary = "Runtime execution requires attention."
	}
	item := domain.Item{ID: itemID, WorkspaceID: run.WorkspaceID, Kind: domain.KindRuntime, DedupeKey: dedupe,
		State: domain.StateOpen, Priority: domain.PriorityCritical, Risk: domain.PriorityCritical,
		TargetType: "workspace", TargetID: run.WorkspaceID.String(),
		TargetRoute: "/operations/runtime?run=" + run.ID.String(), InitiatorPrincipalID: run.RequestedByPrincipalID,
		RuleVersion: "workbench.conditions.v1", Title: "Runtime execution failed", Summary: boundedText(summary, 512),
		ReasonCode: reason, TraceID: run.TraceID, OpenedAt: run.CreatedAt.UTC(), UpdatedAt: run.UpdatedAt.UTC(), Version: 1}
	if err := item.Validate(); err != nil {
		return err
	}
	_, err = queries.UpsertAttentionItem(ctx, attentionUpsertParameters(item))
	return err
}

func projectCompatibilityAttention(ctx context.Context, queries *dbgen.Queries, workspace identity.WorkspaceID,
	bindingID identity.ConsumerBindingID, consumerID identity.ConsumerID, queryID identity.SemanticQueryID, principalRef, reason, summary, traceID string,
	refused bool, now time.Time,
) error {
	if err := queries.LockAttentionProjection(ctx); err != nil {
		return err
	}
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return err
	}
	dedupe := "compatibility:" + bindingID.UUID()
	if !refused {
		_, err := queries.ResolveAttentionItemByDedupe(ctx, dbgen.ResolveAttentionItemByDedupeParams{
			WorkspaceID: workspaceID, DedupeKey: dedupe, ResolvedAt: timestamp(now)})
		return err
	}
	itemID, err := identity.NewAttentionItemID()
	if err != nil {
		return err
	}
	item := domain.Item{ID: itemID, WorkspaceID: workspace, Kind: domain.KindCompatibility, DedupeKey: dedupe,
		State: domain.StateOpen, Priority: domain.PriorityHigh, Risk: domain.PriorityHigh,
		TargetType: "consumer", TargetID: consumerID.String(),
		TargetRoute:    compatibilityRoute(consumerID.String(), bindingID.String(), queryID.String()),
		AudienceRoleID: "semantic_steward", InitiatorPrincipalID: principalIDPointer(principalRef),
		RuleVersion: "workbench.conditions.v1", Title: "Semantic query was refused", Summary: boundedText(summary, 512),
		ReasonCode: reason, TraceID: traceID, OpenedAt: now.UTC(), UpdatedAt: now.UTC(), Version: 1}
	if err := item.Validate(); err != nil {
		return err
	}
	_, err = queries.UpsertAttentionItem(ctx, attentionUpsertParameters(item))
	return err
}

func boundedText(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func compatibilityRoute(consumerID, bindingID, queryID string) string {
	values := url.Values{}
	if _, err := identity.ParseConsumerID(consumerID); err == nil {
		values.Set("consumer", consumerID)
	}
	values.Set("binding", bindingID)
	values.Set("query", queryID)
	return "/delivery/compatibility?" + values.Encode()
}

func proposalAttentionTarget(ctx context.Context, queries *dbgen.Queries, proposal dbgen.Proposal) (string, string, error) {
	targetType := governancedomain.TargetObjectType(proposal.TargetObjectType)
	workspaceTarget := func() (string, string, error) {
		workspaceID, err := identity.WorkspaceIDFromUUIDBytes(proposal.WorkspaceID.Bytes)
		if err != nil {
			return "", "", err
		}
		return "workspace", workspaceID.String(), nil
	}
	if targetType == governancedomain.TargetJoinContract {
		return workspaceTarget()
	}
	assetUUID := proposal.TargetObjectID
	var err error
	switch targetType {
	case governancedomain.TargetSemanticAsset:
	case governancedomain.TargetPhysicalBinding:
		assetUUID, err = queries.GetPhysicalBindingAttentionAsset(ctx, dbgen.GetPhysicalBindingAttentionAssetParams{
			WorkspaceID: proposal.WorkspaceID, ObjectID: proposal.TargetObjectID,
		})
	case governancedomain.TargetModelGrain:
		assetUUID, err = queries.GetModelGrainAttentionAsset(ctx, dbgen.GetModelGrainAttentionAssetParams{
			WorkspaceID: proposal.WorkspaceID, ObjectID: proposal.TargetObjectID,
		})
	case governancedomain.TargetEntityKey:
		assetUUID, err = queries.GetEntityKeyAttentionAsset(ctx, dbgen.GetEntityKeyAttentionAssetParams{
			WorkspaceID: proposal.WorkspaceID, ObjectID: proposal.TargetObjectID,
		})
	default:
		return "", "", governancedomain.ErrInvariant
	}
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !assetUUID.Valid {
		return workspaceTarget()
	}
	if err != nil {
		return "", "", err
	}
	assetID, err := identity.AssetIDFromUUIDBytes(assetUUID.Bytes)
	if err != nil {
		return "", "", err
	}
	return "asset", assetID.String(), nil
}

const reconcileCandidatesSQL = `
WITH candidates AS (
    SELECT p.workspace_id, 'review'::text AS kind, 'review:' || p.id::text AS dedupe_key,
        CASE COALESCE(p.risk_level, 'medium') WHEN 'high' THEN 'high' ELSE 'medium' END AS priority,
        COALESCE(p.risk_level, 'medium') AS risk,
        CASE p.target_object_type WHEN 'semantic_asset' THEN 'asset' ELSE p.target_object_type END AS target_type,
        p.target_object_id AS target_id,
        'proposal'::text AS route_type, p.id AS route_id, 'reviewer'::text AS audience,
		p.created_by AS initiator, left(p.title, 160) AS title,
        left(CASE WHEN p.summary = '' THEN 'Proposal requires independent review.' ELSE p.summary END, 512) AS summary,
        'PROPOSAL_REVIEW_REQUIRED'::text AS reason_code, ''::text AS evidence_ref,
		''::text AS trace_id, COALESCE(p.submitted_at, p.created_at) AS opened_at, p.updated_at
	FROM proposals p
	WHERE p.state='in_review'
    UNION ALL
	SELECT run.workspace_id, 'validation', 'validation:' || run.proposal_id::text, 'high', 'high',
		CASE proposal.target_object_type WHEN 'semantic_asset' THEN 'asset' ELSE proposal.target_object_type END,
		proposal.target_object_id, 'proposal', proposal.id, 'semantic_steward', proposal.created_by,
        left(proposal.title, 160), 'Validation failed and requires attention.', 'VALIDATION_FAILED',
        '', COALESCE(runtime.trace_id, ''), run.started_at, COALESCE(run.finished_at, run.started_at)
	FROM validation_runs run
    JOIN proposals proposal ON proposal.workspace_id=run.workspace_id AND proposal.id=run.proposal_id
    LEFT JOIN runtime_runs runtime ON runtime.workspace_id=run.workspace_id AND runtime.kind='validation'
        AND runtime.source_type='validation_run' AND runtime.source_id=run.id::text
	WHERE run.status='failed'
	  AND run.id = (
	      SELECT candidate.id
	      FROM validation_runs candidate
	      WHERE candidate.workspace_id=run.workspace_id AND candidate.proposal_id=run.proposal_id
	      ORDER BY COALESCE(candidate.finished_at, candidate.started_at) DESC, candidate.id DESC
	      LIMIT 1
	  )
    UNION ALL
	SELECT run.workspace_id, 'source', 'source:' || run.source_connection_id::text,
        CASE WHEN run.status='failed' THEN 'critical' ELSE 'high' END,
        CASE WHEN run.status='failed' THEN 'critical' ELSE 'high' END,
        'source', run.source_connection_id, 'source', run.source_connection_id, 'source_operator',
		COALESCE(runtime.requested_by_principal_id::text, ''), 'Source discovery needs attention',
        CASE WHEN run.status='failed' THEN 'Discovery failed.' ELSE 'Discovery completed with degraded results.' END,
        CASE WHEN run.status='failed' THEN 'DISCOVERY_FAILED' ELSE 'DISCOVERY_DEGRADED' END,
		'', COALESCE(run.trace_id, runtime.trace_id, ''), run.created_at, run.updated_at
    FROM discovery_runs run
    LEFT JOIN runtime_runs runtime ON runtime.workspace_id=run.workspace_id AND runtime.kind='discovery'
        AND runtime.source_type='discovery_run' AND runtime.source_id=run.id::text
    WHERE run.status IN ('failed','degraded')
      AND run.id = (
          SELECT candidate.id
          FROM discovery_runs candidate
          WHERE candidate.workspace_id=run.workspace_id
            AND candidate.source_connection_id=run.source_connection_id
          ORDER BY candidate.updated_at DESC, candidate.id DESC
          LIMIT 1
      )
    UNION ALL
    SELECT run.workspace_id, 'runtime', 'runtime:' || run.id::text, 'critical', 'critical',
		'workspace', run.workspace_id, 'runtime_run', run.id, '', COALESCE(run.requested_by_principal_id::text, ''),
		'Runtime execution failed', COALESCE(NULLIF(run.error_summary, ''), 'Runtime execution requires attention.'),
        CASE WHEN run.state='dead_letter' THEN 'RUNTIME_DEAD_LETTER' ELSE 'RUNTIME_FAILED' END,
        '', COALESCE(run.trace_id, ''), run.created_at, run.updated_at
    FROM runtime_runs run
    WHERE run.state IN ('failed','dead_letter') AND run.kind NOT IN ('discovery','validation')
    UNION ALL
	SELECT query.workspace_id, 'compatibility', 'compatibility:' || query.binding_id::text, 'high', 'high',
		'consumer', binding.consumer_id, 'semantic_query', query.id, 'semantic_steward',
		query.principal_ref, 'Semantic query was refused', left(refusal.clarification, 512), refusal.reason_code,
        '', query.trace_id, refusal.created_at, refusal.created_at
	FROM semantic_refusals refusal
	JOIN semantic_queries query ON query.workspace_id=refusal.workspace_id AND query.id=refusal.query_id
	JOIN consumer_bindings binding ON binding.workspace_id=query.workspace_id AND binding.id=query.binding_id
	WHERE query.binding_id IS NOT NULL AND binding.status='active'
	  AND (binding.expires_at IS NULL OR binding.expires_at > CURRENT_TIMESTAMP)
	  AND query.id = (
	      SELECT candidate.id FROM semantic_queries candidate
	      WHERE candidate.workspace_id=query.workspace_id AND candidate.binding_id=query.binding_id
	      ORDER BY candidate.created_at DESC, candidate.id DESC
	      LIMIT 1
	  )
)
SELECT workspace_id, kind, dedupe_key, priority, risk, target_type, target_id, route_type,
    route_id, audience, initiator, title, summary, reason_code, evidence_ref, trace_id, opened_at, updated_at
FROM candidates
ORDER BY updated_at DESC, workspace_id, dedupe_key
LIMIT $1`
