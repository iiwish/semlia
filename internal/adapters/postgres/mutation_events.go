package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	"github.com/iiwish/semlia/pkg/identity"
)

// mutationEvent is the shared audit+outbox plumbing extracted from the M1
// catalog pattern: every governance mutation commits one audit_events fact
// and one strict semlia.events/v1 outbox event (validated with
// DisallowUnknownFields by the projection publisher) in the SAME transaction
// as its state change. The payloadData map is shared verbatim by both sinks,
// exactly like createCatalogMutationEvents did for M1.
type mutationEvent struct {
	WorkspaceID identity.WorkspaceID
	AuditID     identity.EventID
	OutboxID    identity.EventID
	AuditType   string
	OutboxType  string
	SpecVersion string
	Action      string
	Actor       string
	TraceID     string
	CreatedAt   time.Time
	Data        map[string]any
}

func createMutationEvents(ctx context.Context, queries *dbgen.Queries, event mutationEvent) error {
	workspaceID, err := uuidValue(event.WorkspaceID)
	if err != nil {
		return err
	}
	auditID, err := uuidValue(event.AuditID)
	if err != nil {
		return err
	}
	outboxID, err := uuidValue(event.OutboxID)
	if err != nil {
		return err
	}
	createdAt := event.CreatedAt.UTC()
	payloadData := buildMutationPayload(event.Data, event.SpecVersion, event.Action)
	auditPayload, err := json.Marshal(payloadData)
	if err != nil {
		return err
	}
	if err := queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		ID: auditID, WorkspaceID: workspaceID, EventType: event.AuditType,
		ActorID: textValue(event.Actor), Payload: auditPayload, TraceID: event.TraceID,
		CreatedAt: timestamp(createdAt),
	}); err != nil {
		return fmt.Errorf("create mutation audit event: %w", err)
	}
	outboxPayload, err := json.Marshal(map[string]any{
		"specVersion": "semlia.events/v1",
		"id":          event.OutboxID.String(),
		"type":        event.OutboxType,
		"source":      "urn:semlia:control-plane",
		"workspaceId": event.WorkspaceID.String(),
		"time":        createdAt.Format(time.RFC3339Nano),
		"traceId":     event.TraceID,
		"data":        payloadData,
	})
	if err != nil {
		return err
	}
	if err := queries.EnqueueOutboxEvent(ctx, dbgen.EnqueueOutboxEventParams{
		ID: outboxID, WorkspaceID: workspaceID, EventType: event.OutboxType, Payload: outboxPayload,
		MaxAttempts: 8, AvailableAt: timestamp(createdAt), TraceID: event.TraceID,
	}); err != nil {
		return fmt.Errorf("enqueue mutation outbox event: %w", err)
	}
	return nil
}

// buildMutationPayload is the shared data+specVersion+action payload builder
// used by every governance audit fact and outbox event.
func buildMutationPayload(data map[string]any, specVersion, action string) map[string]any {
	payloadData := make(map[string]any, len(data)+2)
	for key, value := range data {
		payloadData[key] = value
	}
	payloadData["specVersion"] = specVersion
	payloadData["action"] = action
	return payloadData
}

// governedObjectEvent is the audit-only fact of one governed object change
// application. There is deliberately no outbox event: release publication
// remains the outbox signal, and governed object changes surface through
// audit queries and the release manifests built on top of the pinned
// versions (packet M2-T008).
type governedObjectEvent struct {
	WorkspaceID identity.WorkspaceID
	ObjectType  string
	ObjectID    string
	AuditID     identity.EventID
	Action      string
	Version     int
	Summary     string
	Actor       string
	TraceID     string
	CreatedAt   time.Time
}

func createGovernedObjectAuditEvent(ctx context.Context, queries *dbgen.Queries, event governedObjectEvent) error {
	workspaceID, err := uuidValue(event.WorkspaceID)
	if err != nil {
		return err
	}
	auditID, err := uuidValue(event.AuditID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(buildMutationPayload(map[string]any{
		"objectType": event.ObjectType,
		"objectId":   event.ObjectID,
		"version":    event.Version,
		"summary":    event.Summary,
	}, "semlia.governance-object/v1", event.Action))
	if err != nil {
		return err
	}
	return queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		ID: auditID, WorkspaceID: workspaceID,
		EventType: "governance." + event.ObjectType + "." + event.Action,
		ActorID:   textValue(event.Actor), Payload: payload, TraceID: event.TraceID,
		CreatedAt: timestamp(event.CreatedAt.UTC()),
	})
}

type proposalEvent struct {
	WorkspaceID identity.WorkspaceID
	ProposalID  identity.ProposalID
	AssetID     *identity.AssetID
	AuditID     identity.EventID
	OutboxID    identity.EventID
	Action      string
	FromState   string
	ToState     string
	Actor       string
	TraceID     string
	CreatedAt   time.Time
}

func createProposalMutationEvents(ctx context.Context, queries *dbgen.Queries, event proposalEvent) error {
	data := map[string]any{
		"proposalId": event.ProposalID.String(),
		"fromState":  event.FromState,
		"toState":    event.ToState,
	}
	if event.AssetID != nil {
		data["assetId"] = event.AssetID.String()
	}
	return createMutationEvents(ctx, queries, mutationEvent{
		WorkspaceID: event.WorkspaceID, AuditID: event.AuditID, OutboxID: event.OutboxID,
		AuditType: "governance.proposal." + event.Action, OutboxType: "proposal.changed",
		SpecVersion: "semlia.proposal/v1", Action: event.Action, Actor: event.Actor,
		TraceID: event.TraceID, CreatedAt: event.CreatedAt, Data: data,
	})
}

type releaseEvent struct {
	WorkspaceID           identity.WorkspaceID
	ReleaseID             identity.ReleaseID
	AuditID               identity.EventID
	OutboxID              identity.EventID
	Action                string
	ManifestDigest        string
	Sequence              int64
	RolledBackToReleaseID *identity.ReleaseID
	Actor                 string
	TraceID               string
	CreatedAt             time.Time
}

func createReleaseMutationEvents(ctx context.Context, queries *dbgen.Queries, event releaseEvent) error {
	data := map[string]any{
		"releaseId":      event.ReleaseID.String(),
		"manifestDigest": event.ManifestDigest,
		"sequence":       event.Sequence,
	}
	if event.RolledBackToReleaseID != nil {
		data["rolledBackToReleaseId"] = event.RolledBackToReleaseID.String()
	}
	return createMutationEvents(ctx, queries, mutationEvent{
		WorkspaceID: event.WorkspaceID, AuditID: event.AuditID, OutboxID: event.OutboxID,
		AuditType: "governance.release." + event.Action, OutboxType: "release.published",
		SpecVersion: "semlia.release/v1", Action: event.Action, Actor: event.Actor,
		TraceID: event.TraceID, CreatedAt: event.CreatedAt, Data: data,
	})
}
