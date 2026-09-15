package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	domain "github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/internal/domain/governance"
	operationsdomain "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func (store *Store) CreateConsumer(ctx context.Context, consumer domain.Consumer) (domain.Consumer, error) {
	id, workspaceID, err := distributionIDs(consumer.ID, consumer.WorkspaceID)
	if err != nil {
		return domain.Consumer{}, err
	}
	row, err := store.queries.CreateDistributionConsumer(ctx, dbgen.CreateDistributionConsumerParams{
		ID: id, WorkspaceID: workspaceID, StableKey: consumer.StableKey, Name: consumer.Name, Kind: consumer.Kind,
		Status: string(consumer.Status), OwnerPrincipalRef: consumer.OwnerPrincipalRef,
		Metadata: objectJSON(consumer.Metadata), CreatedAt: timestamp(consumer.CreatedAt),
	})
	if err != nil {
		return domain.Consumer{}, distributionRepositoryError("create consumer", err)
	}
	return consumerFromRow(row)
}

func (store *Store) GetConsumer(ctx context.Context, workspace identity.WorkspaceID, consumer identity.ConsumerID) (domain.Consumer, error) {
	id, workspaceID, err := distributionIDs(consumer, workspace)
	if err != nil {
		return domain.Consumer{}, err
	}
	row, err := store.queries.GetDistributionConsumer(ctx, dbgen.GetDistributionConsumerParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return domain.Consumer{}, distributionRepositoryError("get consumer", err)
	}
	return consumerFromRow(row)
}

func (store *Store) ListConsumers(ctx context.Context, workspace identity.WorkspaceID) ([]domain.Consumer, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, err
	}
	rows, err := store.queries.ListDistributionConsumers(ctx, workspaceID)
	if err != nil {
		return nil, distributionRepositoryError("list consumers", err)
	}
	items := make([]domain.Consumer, 0, len(rows))
	for _, row := range rows {
		item, decodeErr := consumerFromRow(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (store *Store) UpdateConsumer(ctx context.Context, consumer domain.Consumer) (domain.Consumer, error) {
	id, workspaceID, err := distributionIDs(consumer.ID, consumer.WorkspaceID)
	if err != nil {
		return domain.Consumer{}, err
	}
	row, err := store.queries.UpdateDistributionConsumer(ctx, dbgen.UpdateDistributionConsumerParams{
		WorkspaceID: workspaceID, ID: id, Name: consumer.Name, Status: string(consumer.Status),
		Metadata: objectJSON(consumer.Metadata), UpdatedAt: timestamp(consumer.UpdatedAt),
	})
	if err != nil {
		return domain.Consumer{}, distributionRepositoryError("update consumer", err)
	}
	return consumerFromRow(row)
}

func (store *Store) CreateConsumerBinding(ctx context.Context, binding domain.ConsumerBinding) (domain.ConsumerBinding, error) {
	id, workspaceID, err := distributionIDs(binding.ID, binding.WorkspaceID)
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	consumerID, err := uuidValue(binding.ConsumerID)
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	releaseID, err := nullableUUIDValue(binding.ReleaseID)
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	row, err := store.queries.CreateDistributionBinding(ctx, dbgen.CreateDistributionBindingParams{
		ID: id, WorkspaceID: workspaceID, ConsumerID: consumerID, Environment: binding.Environment,
		Purpose: binding.Purpose, Mode: string(binding.Mode), ReleaseID: releaseID,
		CompatibilityConstraint: objectJSON(binding.CompatibilityConstraint), ExpiresAt: optionalTimestamp(binding.ExpiresAt),
		Status: string(binding.Status), Version: int32(binding.Version), CreatedAt: timestamp(binding.CreatedAt),
	})
	if err != nil {
		return domain.ConsumerBinding{}, distributionRepositoryError("create consumer binding", err)
	}
	return bindingFromRow(row)
}

func (store *Store) GetConsumerBinding(ctx context.Context, workspace identity.WorkspaceID, binding identity.ConsumerBindingID) (domain.ConsumerBinding, error) {
	id, workspaceID, err := distributionIDs(binding, workspace)
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	row, err := store.queries.GetDistributionBinding(ctx, dbgen.GetDistributionBindingParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return domain.ConsumerBinding{}, distributionRepositoryError("get consumer binding", err)
	}
	return bindingFromRow(row)
}

func (store *Store) ListConsumerBindings(ctx context.Context, workspace identity.WorkspaceID) ([]domain.ConsumerBinding, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return nil, err
	}
	rows, err := store.queries.ListDistributionBindings(ctx, workspaceID)
	if err != nil {
		return nil, distributionRepositoryError("list consumer bindings", err)
	}
	items := make([]domain.ConsumerBinding, 0, len(rows))
	for _, row := range rows {
		item, decodeErr := bindingFromRow(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (store *Store) UpdateConsumerBinding(ctx context.Context, binding domain.ConsumerBinding, expectedVersion int) (domain.ConsumerBinding, error) {
	id, workspaceID, err := distributionIDs(binding.ID, binding.WorkspaceID)
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	releaseID, err := nullableUUIDValue(binding.ReleaseID)
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.ConsumerBinding{}, distributionRepositoryError("begin update consumer binding", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	row, err := queries.UpdateDistributionBinding(ctx, dbgen.UpdateDistributionBindingParams{
		WorkspaceID: workspaceID, ID: id, Purpose: binding.Purpose, Mode: string(binding.Mode), ReleaseID: releaseID,
		CompatibilityConstraint: objectJSON(binding.CompatibilityConstraint), ExpiresAt: optionalTimestamp(binding.ExpiresAt),
		Status: string(binding.Status), UpdatedAt: timestamp(binding.UpdatedAt), Version: int32(expectedVersion),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ConsumerBinding{}, fmt.Errorf("update consumer binding: %w", domain.ErrConflict)
		}
		return domain.ConsumerBinding{}, distributionRepositoryError("update consumer binding", err)
	}
	updated, err := bindingFromRow(row)
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	if updated.Status != domain.BindingActive || updated.ExpiresAt != nil && !updated.ExpiresAt.After(updated.UpdatedAt) {
		if err := projectCompatibilityAttention(ctx, queries, updated.WorkspaceID, updated.ID, updated.ConsumerID,
			identity.SemanticQueryID{}, "", "", "", "", false, updated.UpdatedAt); err != nil {
			return domain.ConsumerBinding{}, distributionRepositoryError("resolve compatibility attention", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ConsumerBinding{}, distributionRepositoryError("commit consumer binding update", err)
	}
	return updated, nil
}

func (store *Store) CurrentReleaseSnapshot(ctx context.Context, workspace identity.WorkspaceID) (domain.ReleaseSnapshot, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return domain.ReleaseSnapshot{}, err
	}
	row, err := store.queries.GetCurrentDistributionRelease(ctx, workspaceID)
	if err != nil {
		return domain.ReleaseSnapshot{}, distributionRepositoryError("get current distribution release", err)
	}
	return store.loadReleaseSnapshot(ctx, releaseHeader{row.ID, row.WorkspaceID, row.Sequence, row.ManifestDigest, row.PublishedAt})
}

func (store *Store) ReleaseSnapshot(ctx context.Context, workspace identity.WorkspaceID, release identity.ReleaseID) (domain.ReleaseSnapshot, error) {
	id, workspaceID, err := distributionIDs(release, workspace)
	if err != nil {
		return domain.ReleaseSnapshot{}, err
	}
	row, err := store.queries.GetDistributionRelease(ctx, dbgen.GetDistributionReleaseParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return domain.ReleaseSnapshot{}, distributionRepositoryError("get distribution release", err)
	}
	return store.loadReleaseSnapshot(ctx, releaseHeader{row.ID, row.WorkspaceID, row.Sequence, row.ManifestDigest, row.PublishedAt})
}

type releaseHeader struct {
	id             pgtype.UUID
	workspaceID    pgtype.UUID
	sequence       int64
	manifestDigest string
	publishedAt    pgtype.Timestamptz
}

func (store *Store) loadReleaseSnapshot(ctx context.Context, header releaseHeader) (domain.ReleaseSnapshot, error) {
	releaseID, err := identity.ReleaseIDFromUUIDBytes(header.id.Bytes)
	if err != nil {
		return domain.ReleaseSnapshot{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(header.workspaceID.Bytes)
	if err != nil {
		return domain.ReleaseSnapshot{}, err
	}
	assetRows, err := store.queries.ListDistributionReleaseAssets(ctx, dbgen.ListDistributionReleaseAssetsParams{
		WorkspaceID: header.workspaceID, ReleaseID: header.id,
	})
	if err != nil {
		return domain.ReleaseSnapshot{}, distributionRepositoryError("list distribution release assets", err)
	}
	snapshot := domain.ReleaseSnapshot{ReleaseID: releaseID, WorkspaceID: workspaceID, Sequence: header.sequence,
		ManifestDigest: header.manifestDigest, PublishedAt: header.publishedAt.Time.UTC(), Assets: make([]domain.ReleasedAsset, 0, len(assetRows))}
	for _, row := range assetRows {
		assetID, decodeErr := identity.AssetIDFromUUIDBytes(row.AssetID.Bytes)
		if decodeErr != nil {
			return domain.ReleaseSnapshot{}, decodeErr
		}
		revisionID, decodeErr := identity.RevisionIDFromUUIDBytes(row.RevisionID.Bytes)
		if decodeErr != nil {
			return domain.ReleaseSnapshot{}, decodeErr
		}
		name, aliases := releasedAssetNames(row.Namespace+"."+row.Key, row.Content)
		snapshot.Assets = append(snapshot.Assets, domain.ReleasedAsset{AssetID: assetID, RevisionID: revisionID,
			Address: row.Namespace + "." + row.Key, AssetType: semantic.AssetType(row.AssetType), Name: name,
			Aliases: aliases, Content: cloneJSON(row.Content), ContentDigest: row.ContentDigest, Position: int(row.Position)})
	}
	objectRows, err := store.queries.ListDistributionReleaseObjectSnapshots(ctx, dbgen.ListDistributionReleaseObjectSnapshotsParams{
		WorkspaceID: header.workspaceID, ReleaseID: header.id,
	})
	if err != nil {
		return domain.ReleaseSnapshot{}, distributionRepositoryError("list release object snapshots", err)
	}
	for _, row := range objectRows {
		if err := appendReleasedObject(&snapshot, row); err != nil {
			return domain.ReleaseSnapshot{}, err
		}
	}
	physicalRows, err := store.queries.ListReleaseExecutionRelations(ctx, dbgen.ListReleaseExecutionRelationsParams{WorkspaceID: header.workspaceID, ReleaseID: header.id})
	if err != nil {
		return domain.ReleaseSnapshot{}, distributionRepositoryError("load execution provenance", err)
	}
	if len(physicalRows) > 0 {
		snapshot.Execution = &domain.ExecutionProvenance{CompilerVersion: "postgres-aggregate/v1"}
		pins, err := store.queries.ListReleaseExecutionBindingPins(ctx, dbgen.ListReleaseExecutionBindingPinsParams{WorkspaceID: header.workspaceID, ReleaseID: header.id})
		if err != nil {
			return domain.ReleaseSnapshot{}, distributionRepositoryError("load binding execution pins", err)
		}
		for _, pin := range pins {
			snapshot.Execution.BindingPins = append(snapshot.Execution.BindingPins, domain.ExecutionBindingPin{BindingID: pin.BindingID.String(), Version: int(pin.Version), DatasetID: pin.DatasetID.String(), DatasetRevisionID: pin.DatasetRevisionID})
		}
		for _, payload := range physicalRows {
			var relation domain.ExecutionRelation
			if json.Unmarshal(payload, &relation) != nil {
				return domain.ReleaseSnapshot{}, domain.ErrInvariant
			}
			snapshot.Execution.Relations = append(snapshot.Execution.Relations, relation)
		}
		for _, row := range objectRows {
			if row.ObjectType == "join_contract" {
				for _, j := range snapshot.Joins {
					if j.ID.UUID() == row.ObjectID.String() {
						join, err := executionJoinFromSnapshot(j, row.Payload)
						if err != nil {
							return domain.ReleaseSnapshot{}, err
						}
						snapshot.Execution.Joins = append(snapshot.Execution.Joins, join)
					}
				}
			}
		}
	}
	return snapshot, nil
}

func executionJoinFromSnapshot(join domain.ReleasedJoinContract, raw []byte) (domain.ExecutionJoin, error) {
	var payload struct {
		Left       []string `json:"left_field_refs"`
		Right      []string `json:"right_field_refs"`
		Expression string   `json:"join_expression"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return domain.ExecutionJoin{}, domain.ErrInvariant
	}
	pairs := []domain.ExecutionFieldPair{}
	if len(payload.Left) == len(payload.Right) {
		for i, left := range payload.Left {
			l, le := typedField(left)
			r, re := typedField(payload.Right[i])
			if le != nil || re != nil {
				return domain.ExecutionJoin{}, domain.ErrInvariant
			}
			pairs = append(pairs, domain.ExecutionFieldPair{LeftFieldID: l.UUID(), RightFieldID: r.UUID()})
		}
	}
	return domain.ExecutionJoin{ID: join.ID.String(), Version: join.Version, LeftDatasetID: join.LeftDatasetID.UUID(), RightDatasetID: join.RightDatasetID.UUID(), JoinType: string(join.JoinType), Cardinality: string(join.Cardinality), FieldPairs: pairs, Expression: payload.Expression}, nil
}

func (store *Store) RecordResolution(ctx context.Context, record distributionapp.ResolutionRecord) error {
	workspaceID, err := uuidValue(record.Query.WorkspaceID)
	if err != nil {
		return err
	}
	queryID, err := uuidValue(record.Query.ID)
	if err != nil {
		return err
	}
	consumerID, err := nullableUUIDValue(record.Query.ConsumerID)
	if err != nil {
		return err
	}
	bindingID, err := nullableUUIDValue(record.Query.BindingID)
	if err != nil {
		return err
	}
	var releaseID pgtype.UUID
	if !record.Query.SelectedReleaseID.IsZero() {
		releaseID, err = uuidValue(record.Query.SelectedReleaseID)
		if err != nil {
			return err
		}
	}
	validationID, err := uuidValue(record.Validation.ID)
	if err != nil {
		return err
	}
	var validationPlanID pgtype.UUID
	if record.Validation.PlanID != nil {
		validationPlanID, err = uuidValue(*record.Validation.PlanID)
		if err != nil {
			return err
		}
	}
	eventID, err := identity.NewEventID()
	if err != nil {
		return err
	}
	eventDBID, _ := uuidValue(eventID)
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return distributionRepositoryError("begin semantic resolution", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	if err := queries.CreateSemanticQuery(ctx, dbgen.CreateSemanticQueryParams{
		ID: queryID, WorkspaceID: workspaceID, PrincipalRef: record.Query.PrincipalRef,
		ConsumerID: consumerID, BindingID: bindingID, SelectedReleaseID: releaseID,
		SchemaVersion: record.Query.SchemaVersion, ResolverVersion: record.Query.ResolverVersion,
		CanonicalRequest: objectJSON(record.Query.CanonicalRequest), RequestDigest: record.Query.RequestDigest,
		Channel: record.Query.Channel, TraceID: record.Query.TraceID, IdempotencyKey: record.Query.IdempotencyKey,
		Outcome: record.Query.Outcome, CreatedAt: timestamp(record.Query.CreatedAt), FinalizedAt: timestamp(record.Query.FinalizedAt),
	}); err != nil {
		return distributionRepositoryError("create semantic query", err)
	}
	if record.Plan != nil {
		planID, _ := uuidValue(record.Plan.ID)
		canonicalPlan, marshalErr := json.Marshal(record.Plan)
		if marshalErr != nil {
			return marshalErr
		}
		if err := queries.CreateResolvedSemanticPlan(ctx, dbgen.CreateResolvedSemanticPlanParams{
			ID: planID, WorkspaceID: workspaceID, QueryID: queryID, ReleaseID: releaseID,
			ResolverVersion: record.Plan.ResolverVersion, CanonicalPlan: canonicalPlan,
			PlanDigest: record.Plan.PlanDigest, ExecutionStatus: record.Plan.ExecutionStatus, CreatedAt: timestamp(record.Plan.CreatedAt),
		}); err != nil {
			return distributionRepositoryError("create resolved semantic plan", err)
		}
	}
	if record.Refusal != nil {
		candidateIDs := make([]string, 0, len(record.Refusal.CandidateIDs))
		for _, id := range record.Refusal.CandidateIDs {
			candidateIDs = append(candidateIDs, id.String())
		}
		sort.Strings(candidateIDs)
		candidateJSON, _ := json.Marshal(candidateIDs)
		if err := queries.CreateSemanticRefusal(ctx, dbgen.CreateSemanticRefusalParams{
			WorkspaceID: workspaceID, QueryID: queryID, ReasonCode: string(record.Refusal.Code),
			AuthorizedCandidateIds: candidateJSON, Clarification: record.Refusal.Clarification,
			Details: objectJSON(record.Refusal.Details), CreatedAt: timestamp(record.Refusal.CreatedAt),
		}); err != nil {
			return distributionRepositoryError("create semantic refusal", err)
		}
	}
	if err := queries.CreateQueryValidationRun(ctx, dbgen.CreateQueryValidationRunParams{
		ID: validationID, WorkspaceID: workspaceID, QueryID: queryID, PlanID: validationPlanID,
		Validator: record.Validation.Validator, ValidatorVersion: record.Validation.ValidatorVersion,
		InputDigest: record.Validation.InputDigest, Status: record.Validation.Status,
		CreatedAt: timestamp(record.Validation.CreatedAt), CompletedAt: timestamp(record.Validation.CompletedAt),
	}); err != nil {
		return distributionRepositoryError("create query validation run", err)
	}
	for index, result := range record.Validation.Results {
		if err := queries.CreateQueryValidationResult(ctx, dbgen.CreateQueryValidationResultParams{
			ValidationRunID: validationID, Position: int32(index + 1), Severity: result.Severity,
			Code: result.Code, Message: result.Message, Details: objectJSON(result.Details),
		}); err != nil {
			return distributionRepositoryError("create query validation result", err)
		}
	}
	reason := pgtype.Text{}
	if record.Refusal != nil {
		reason = textValue(string(record.Refusal.Code))
	}
	if err := queries.CreateSemanticResolutionEvent(ctx, dbgen.CreateSemanticResolutionEventParams{
		ID: eventDBID, WorkspaceID: workspaceID, QueryID: queryID, PrincipalRef: record.Query.PrincipalRef,
		ConsumerID: consumerID, BindingID: bindingID, ReleaseID: releaseID, Channel: record.Query.Channel,
		Outcome: record.Query.Outcome, ReasonCode: reason, TraceID: record.Query.TraceID, OccurredAt: timestamp(record.Query.FinalizedAt),
	}); err != nil {
		return distributionRepositoryError("create semantic resolution event", err)
	}
	auditPayload, _ := json.Marshal(map[string]any{"queryId": record.Query.ID.String(), "requestDigest": record.Query.RequestDigest,
		"releaseId": record.Query.SelectedReleaseID.String(), "resolverVersion": record.Query.ResolverVersion,
		"outcome": record.Query.Outcome, "reasonCode": optionalText(reason)})
	if err := queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{ID: eventDBID, WorkspaceID: workspaceID,
		EventType: "semantic.resolution." + record.Query.Outcome, ActorID: textValue(record.Query.PrincipalRef),
		Payload: auditPayload, TraceID: record.Query.TraceID, CreatedAt: timestamp(record.Query.FinalizedAt)}); err != nil {
		return distributionRepositoryError("create semantic resolution audit", err)
	}
	runtimeID, err := identity.NewRunID()
	if err != nil {
		return err
	}
	runtimeEvent, err := newRuntimeEvent(record.Query.WorkspaceID, runtimeID, "finalized", operationsdomain.RunEventState,
		operationsdomain.RunSucceeded, "finalized", "", "", record.Query.FinalizedAt)
	if err != nil {
		return err
	}
	if err := projectRuntime(ctx, queries, operationsdomain.RuntimeRun{ID: runtimeID, WorkspaceID: record.Query.WorkspaceID,
		Kind: operationsdomain.RunKindSemanticResolution, SourceType: "semantic_query", SourceID: record.Query.ID.String(),
		SourceVersionDigest: normalizedRuntimeDigest(record.Query.RequestDigest), TraceID: record.Query.TraceID,
		IdempotencyKey:         boundedRuntimeIdempotencyKey("runtime:semantic-resolution:", record.Query.ID.String()),
		RequestedByPrincipalID: principalIDPointer(record.Query.PrincipalRef), State: operationsdomain.RunSucceeded,
		Phase: "finalized", MaxAttempts: 1, StartedAt: timePointer(record.Query.CreatedAt), FinishedAt: timePointer(record.Query.FinalizedAt),
		Version: 1, CreatedAt: record.Query.CreatedAt.UTC(), UpdatedAt: record.Query.FinalizedAt.UTC()}, runtimeEvent); err != nil {
		return distributionRepositoryError("project semantic resolution", err)
	}
	if record.Query.BindingID != nil {
		bindingRow, bindingErr := queries.GetDistributionBinding(ctx, dbgen.GetDistributionBindingParams{
			WorkspaceID: workspaceID, ID: bindingID,
		})
		if bindingErr != nil {
			return distributionRepositoryError("load compatibility attention binding", bindingErr)
		}
		binding, bindingErr := bindingFromRow(bindingRow)
		if bindingErr != nil {
			return bindingErr
		}
		refused, refusalReason, refusalSummary := false, "", ""
		if record.Refusal != nil {
			refused, refusalReason, refusalSummary = true, string(record.Refusal.Code), record.Refusal.Clarification
		}
		if err := projectCompatibilityAttention(ctx, queries, record.Query.WorkspaceID, *record.Query.BindingID,
			binding.ConsumerID, record.Query.ID, record.Query.PrincipalRef, refusalReason, refusalSummary, record.Query.TraceID,
			refused, record.Query.FinalizedAt); err != nil {
			return distributionRepositoryError("project compatibility attention", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return distributionRepositoryError("commit semantic resolution", err)
	}
	return nil
}

func (store *Store) GetSemanticQuery(ctx context.Context, workspace identity.WorkspaceID, query identity.SemanticQueryID) (distributionapp.ResolutionResult, error) {
	id, workspaceID, err := distributionIDs(query, workspace)
	if err != nil {
		return distributionapp.ResolutionResult{}, err
	}
	row, err := store.queries.GetSemanticQueryRecord(ctx, dbgen.GetSemanticQueryRecordParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return distributionapp.ResolutionResult{}, distributionRepositoryError("get semantic query", err)
	}
	return store.loadResolutionResult(ctx, row)
}

func (store *Store) GetResolutionByIdempotency(ctx context.Context, workspace identity.WorkspaceID, principalRef, key string) (distributionapp.ResolutionResult, error) {
	workspaceID, err := uuidValue(workspace)
	if err != nil {
		return distributionapp.ResolutionResult{}, err
	}
	row, err := store.queries.GetSemanticQueryByIdempotency(ctx, dbgen.GetSemanticQueryByIdempotencyParams{
		WorkspaceID: workspaceID, PrincipalRef: principalRef, IdempotencyKey: key,
	})
	if err != nil {
		return distributionapp.ResolutionResult{}, distributionRepositoryError("get semantic query by idempotency", err)
	}
	return store.loadResolutionResult(ctx, row)
}

func (store *Store) GetResolvedSemanticPlan(ctx context.Context, workspace identity.WorkspaceID, plan identity.ResolvedSemanticPlanID) (domain.ResolvedSemanticPlan, error) {
	id, workspaceID, err := distributionIDs(plan, workspace)
	if err != nil {
		return domain.ResolvedSemanticPlan{}, err
	}
	row, err := store.queries.GetResolvedSemanticPlanRecord(ctx, dbgen.GetResolvedSemanticPlanRecordParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return domain.ResolvedSemanticPlan{}, distributionRepositoryError("get resolved semantic plan", err)
	}
	return planFromRow(row)
}

func (store *Store) loadResolutionResult(ctx context.Context, row dbgen.SemanticQuery) (distributionapp.ResolutionResult, error) {
	query, err := semanticQueryFromRow(row)
	if err != nil {
		return distributionapp.ResolutionResult{}, err
	}
	result := distributionapp.ResolutionResult{Query: query}
	if query.Outcome == "resolved" {
		planRow, getErr := store.queries.GetResolvedSemanticPlanByQuery(ctx, dbgen.GetResolvedSemanticPlanByQueryParams{WorkspaceID: row.WorkspaceID, QueryID: row.ID})
		if getErr != nil {
			return distributionapp.ResolutionResult{}, distributionRepositoryError("get query plan", getErr)
		}
		plan, decodeErr := planFromRow(planRow)
		if decodeErr != nil {
			return distributionapp.ResolutionResult{}, decodeErr
		}
		result.Plan = &plan
	} else {
		refusalRow, getErr := store.queries.GetSemanticRefusalByQuery(ctx, dbgen.GetSemanticRefusalByQueryParams{WorkspaceID: row.WorkspaceID, QueryID: row.ID})
		if getErr != nil {
			return distributionapp.ResolutionResult{}, distributionRepositoryError("get query refusal", getErr)
		}
		refusal, decodeErr := refusalFromRow(refusalRow)
		if decodeErr != nil {
			return distributionapp.ResolutionResult{}, decodeErr
		}
		result.Refusal = &refusal
	}
	validationRow, err := store.queries.GetQueryValidationRunByQuery(ctx, dbgen.GetQueryValidationRunByQueryParams{WorkspaceID: row.WorkspaceID, QueryID: row.ID})
	if err != nil {
		return distributionapp.ResolutionResult{}, distributionRepositoryError("get query validation", err)
	}
	validation, err := validationFromRow(validationRow)
	if err != nil {
		return distributionapp.ResolutionResult{}, err
	}
	resultRows, err := store.queries.ListQueryValidationResults(ctx, validationRow.ID)
	if err != nil {
		return distributionapp.ResolutionResult{}, distributionRepositoryError("list query validation results", err)
	}
	for _, item := range resultRows {
		validation.Results = append(validation.Results, domain.ValidationResult{Severity: item.Severity, Code: item.Code,
			Message: item.Message, Details: cloneJSON(item.Details)})
	}
	result.Validation = validation
	return result, nil
}

func consumerFromRow(row dbgen.Consumer) (domain.Consumer, error) {
	id, err := identity.ConsumerIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.Consumer{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return domain.Consumer{}, err
	}
	return domain.Consumer{ID: id, WorkspaceID: workspaceID, StableKey: row.StableKey, Name: row.Name, Kind: row.Kind,
		Status: domain.ConsumerStatus(row.Status), OwnerPrincipalRef: row.OwnerPrincipalRef, Metadata: cloneJSON(row.Metadata),
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}, nil
}

func bindingFromRow(row dbgen.ConsumerBinding) (domain.ConsumerBinding, error) {
	id, err := identity.ConsumerBindingIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	consumerID, err := identity.ConsumerIDFromUUIDBytes(row.ConsumerID.Bytes)
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	var releaseID *identity.ReleaseID
	if row.ReleaseID.Valid {
		value, decodeErr := identity.ReleaseIDFromUUIDBytes(row.ReleaseID.Bytes)
		if decodeErr != nil {
			return domain.ConsumerBinding{}, decodeErr
		}
		releaseID = &value
	}
	return domain.ConsumerBinding{ID: id, WorkspaceID: workspaceID, ConsumerID: consumerID,
		Environment: row.Environment, Purpose: row.Purpose, Mode: domain.BindingMode(row.Mode), ReleaseID: releaseID,
		CompatibilityConstraint: cloneJSON(row.CompatibilityConstraint), ExpiresAt: optionalTime(row.ExpiresAt),
		Status: domain.BindingStatus(row.Status), Version: int(row.Version), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}, nil
}

func semanticQueryFromRow(row dbgen.SemanticQuery) (domain.SemanticQuery, error) {
	id, err := identity.SemanticQueryIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.SemanticQuery{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return domain.SemanticQuery{}, err
	}
	query := domain.SemanticQuery{ID: id, WorkspaceID: workspaceID, PrincipalRef: row.PrincipalRef,
		SchemaVersion: row.SchemaVersion, ResolverVersion: row.ResolverVersion, CanonicalRequest: cloneJSON(row.CanonicalRequest),
		RequestDigest: row.RequestDigest, Channel: row.Channel, TraceID: row.TraceID, IdempotencyKey: row.IdempotencyKey,
		Outcome: row.Outcome, CreatedAt: row.CreatedAt.Time.UTC(), FinalizedAt: row.FinalizedAt.Time.UTC()}
	if row.ConsumerID.Valid {
		value, decodeErr := identity.ConsumerIDFromUUIDBytes(row.ConsumerID.Bytes)
		if decodeErr != nil {
			return domain.SemanticQuery{}, decodeErr
		}
		query.ConsumerID = &value
	}
	if row.BindingID.Valid {
		value, decodeErr := identity.ConsumerBindingIDFromUUIDBytes(row.BindingID.Bytes)
		if decodeErr != nil {
			return domain.SemanticQuery{}, decodeErr
		}
		query.BindingID = &value
	}
	if row.SelectedReleaseID.Valid {
		query.SelectedReleaseID, err = identity.ReleaseIDFromUUIDBytes(row.SelectedReleaseID.Bytes)
	}
	return query, err
}

func planFromRow(row dbgen.ResolvedSemanticPlan) (domain.ResolvedSemanticPlan, error) {
	var plan domain.ResolvedSemanticPlan
	if err := json.Unmarshal(row.CanonicalPlan, &plan); err != nil {
		return domain.ResolvedSemanticPlan{}, fmt.Errorf("decode resolved semantic plan: %w", err)
	}
	return plan, nil
}

func refusalFromRow(row dbgen.SemanticRefusal) (domain.Refusal, error) {
	queryID, err := identity.SemanticQueryIDFromUUIDBytes(row.QueryID.Bytes)
	if err != nil {
		return domain.Refusal{}, err
	}
	var raw []string
	if err := json.Unmarshal(row.AuthorizedCandidateIds, &raw); err != nil {
		return domain.Refusal{}, err
	}
	ids := make([]identity.AssetID, 0, len(raw))
	for _, value := range raw {
		id, parseErr := identity.ParseAssetID(value)
		if parseErr != nil {
			return domain.Refusal{}, parseErr
		}
		ids = append(ids, id)
	}
	return domain.Refusal{QueryID: queryID, Code: domain.RefusalCode(row.ReasonCode), CandidateIDs: ids,
		Clarification: row.Clarification, Details: cloneJSON(row.Details), CreatedAt: row.CreatedAt.Time.UTC()}, nil
}

func validationFromRow(row dbgen.QueryValidationRun) (domain.ValidationRun, error) {
	id, err := identity.QueryValidationRunIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return domain.ValidationRun{}, err
	}
	queryID, err := identity.SemanticQueryIDFromUUIDBytes(row.QueryID.Bytes)
	if err != nil {
		return domain.ValidationRun{}, err
	}
	validation := domain.ValidationRun{ID: id, QueryID: queryID, Validator: row.Validator,
		ValidatorVersion: row.ValidatorVersion, InputDigest: row.InputDigest, Status: row.Status,
		Results: []domain.ValidationResult{}, CreatedAt: row.CreatedAt.Time.UTC(), CompletedAt: row.CompletedAt.Time.UTC()}
	if row.PlanID.Valid {
		planID, decodeErr := identity.ResolvedSemanticPlanIDFromUUIDBytes(row.PlanID.Bytes)
		if decodeErr != nil {
			return domain.ValidationRun{}, decodeErr
		}
		validation.PlanID = &planID
	}
	return validation, nil
}

func appendReleasedObject(snapshot *domain.ReleaseSnapshot, row dbgen.ListDistributionReleaseObjectSnapshotsRow) error {
	switch row.ObjectType {
	case string(governance.TargetPhysicalBinding):
		var payload struct {
			ID, AssetID, DatasetID string  `json:"-"`
			FieldID                *string `json:"field_id"`
			RetiredAt              *string `json:"retired_at"`
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(row.Payload, &raw); err != nil {
			return err
		}
		_ = json.Unmarshal(raw["id"], &payload.ID)
		_ = json.Unmarshal(raw["asset_id"], &payload.AssetID)
		_ = json.Unmarshal(raw["dataset_id"], &payload.DatasetID)
		_ = json.Unmarshal(raw["field_id"], &payload.FieldID)
		_ = json.Unmarshal(raw["retired_at"], &payload.RetiredAt)
		var transform string
		_ = json.Unmarshal(raw["transform"], &transform)
		id, err := typedPhysicalBinding(payload.ID)
		if err != nil {
			return err
		}
		assetID, err := typedAsset(payload.AssetID)
		if err != nil {
			return err
		}
		datasetID, err := typedDataset(payload.DatasetID)
		if err != nil {
			return err
		}
		var fieldID *identity.PhysicalFieldID
		if payload.FieldID != nil {
			value, parseErr := typedField(*payload.FieldID)
			if parseErr != nil {
				return parseErr
			}
			fieldID = &value
		}
		snapshot.Bindings = append(snapshot.Bindings, domain.ReleasedPhysicalBinding{ID: id, Version: int(row.Version),
			AssetID: assetID, DatasetID: datasetID, FieldID: fieldID, Retired: payload.RetiredAt != nil, Transform: transform})
	case string(governance.TargetModelGrain):
		var payload struct {
			ID              string `json:"id"`
			AssetID         string `json:"asset_id"`
			GrainExpression string `json:"grain_expression"`
		}
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return err
		}
		id, err := typedGrain(payload.ID)
		if err != nil {
			return err
		}
		assetID, err := typedAsset(payload.AssetID)
		if err != nil {
			return err
		}
		snapshot.Grains = append(snapshot.Grains, domain.ReleasedModelGrain{ID: id, Version: int(row.Version), AssetID: assetID, GrainExpression: payload.GrainExpression})
	case string(governance.TargetEntityKey):
		var payload struct {
			ID      string `json:"id"`
			AssetID string `json:"asset_id"`
		}
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return err
		}
		id, err := typedEntityKey(payload.ID)
		if err != nil {
			return err
		}
		assetID, err := typedAsset(payload.AssetID)
		if err != nil {
			return err
		}
		snapshot.Keys = append(snapshot.Keys, domain.ReleasedEntityKey{ID: id, Version: int(row.Version), AssetID: assetID})
	case string(governance.TargetJoinContract):
		var payload struct {
			ID, LeftDatasetID, RightDatasetID     string
			Cardinality, JoinType, JoinExpression string
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(row.Payload, &raw); err != nil {
			return err
		}
		_ = json.Unmarshal(raw["id"], &payload.ID)
		_ = json.Unmarshal(raw["left_dataset_id"], &payload.LeftDatasetID)
		_ = json.Unmarshal(raw["right_dataset_id"], &payload.RightDatasetID)
		_ = json.Unmarshal(raw["cardinality"], &payload.Cardinality)
		_ = json.Unmarshal(raw["join_type"], &payload.JoinType)
		_ = json.Unmarshal(raw["join_expression"], &payload.JoinExpression)
		id, err := typedJoin(payload.ID)
		if err != nil {
			return err
		}
		left, err := typedDataset(payload.LeftDatasetID)
		if err != nil {
			return err
		}
		right, err := typedDataset(payload.RightDatasetID)
		if err != nil {
			return err
		}
		snapshot.Joins = append(snapshot.Joins, domain.ReleasedJoinContract{ID: id, Version: int(row.Version),
			LeftDatasetID: left, RightDatasetID: right, Cardinality: governance.JoinCardinality(payload.Cardinality),
			JoinType: governance.JoinType(payload.JoinType), JoinExpression: payload.JoinExpression})
	}
	return nil
}

func releasedAssetNames(address string, content []byte) (string, []string) {
	var payload struct {
		Name    string   `json:"name"`
		Title   string   `json:"title"`
		Aliases []string `json:"aliases"`
	}
	_ = json.Unmarshal(content, &payload)
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = strings.TrimSpace(payload.Title)
	}
	if name == "" {
		name = address
	}
	return name, payload.Aliases
}

func distributionIDs(resource uuidIdentity, workspace identity.WorkspaceID) (pgtype.UUID, pgtype.UUID, error) {
	id, err := uuidValue(resource)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	workspaceID, err := uuidValue(workspace)
	return id, workspaceID, err
}

func distributionRepositoryError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, domain.ErrConflict)
		case "23503", "23514", "23502", "55000":
			return fmt.Errorf("%s: %w", operation, domain.ErrInvariant)
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, domain.ErrNotFound)
	}
	return fmt.Errorf("%s: database operation failed", operation)
}

func typedPhysicalBinding(value string) (identity.PhysicalBindingID, error) {
	id, err := identity.FromUUID(identity.PhysicalBinding, value)
	if err != nil {
		return identity.PhysicalBindingID{}, err
	}
	return identity.ParsePhysicalBindingID(id.String())
}
func typedGrain(value string) (identity.ModelGrainID, error) {
	id, err := identity.FromUUID(identity.ModelGrain, value)
	if err != nil {
		return identity.ModelGrainID{}, err
	}
	return identity.ParseModelGrainID(id.String())
}
func typedEntityKey(value string) (identity.EntityKeyID, error) {
	id, err := identity.FromUUID(identity.EntityKey, value)
	if err != nil {
		return identity.EntityKeyID{}, err
	}
	return identity.ParseEntityKeyID(id.String())
}
func typedJoin(value string) (identity.JoinContractID, error) {
	id, err := identity.FromUUID(identity.JoinContract, value)
	if err != nil {
		return identity.JoinContractID{}, err
	}
	return identity.ParseJoinContractID(id.String())
}
func typedAsset(value string) (identity.AssetID, error) {
	id, err := identity.FromUUID(identity.Asset, value)
	if err != nil {
		return identity.AssetID{}, err
	}
	return identity.ParseAssetID(id.String())
}
func typedDataset(value string) (identity.PhysicalDatasetID, error) {
	id, err := identity.FromUUID(identity.PhysicalDataset, value)
	if err != nil {
		return identity.PhysicalDatasetID{}, err
	}
	return identity.ParsePhysicalDatasetID(id.String())
}
func typedField(value string) (identity.PhysicalFieldID, error) {
	if id, err := identity.ParsePhysicalFieldID(value); err == nil {
		return id, nil
	}
	id, err := identity.FromUUID(identity.PhysicalField, value)
	if err != nil {
		return identity.PhysicalFieldID{}, err
	}
	return identity.ParsePhysicalFieldID(id.String())
}
