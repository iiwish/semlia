package operations_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/application/operations"
	"github.com/iiwish/semlia/internal/domain/authorization"
	operationsdomain "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/pkg/identity"
)

type serviceRepository struct {
	auditRows       []operations.StoredAuditEvent
	auditQueries    []operations.AuditQuery
	exportRecord    operations.AuditExportRecord
	download        operations.AuditExportContent
	runtimeRows     []operationsdomain.RuntimeRun
	runtimeQueries  []operations.RuntimeQuery
	replay          *operations.AuditExport
	replayErr       error
	replayRequests  []operations.AuditExportReplayRequest
	auditListErr    error
	updatedSettings operationsdomain.RuntimeSettings
	expectedVersion int64
	updateErr       error
}

func (repo *serviceRepository) GetAuditExportContent(context.Context, identity.WorkspaceID, identity.EventID, time.Time) (operations.AuditExportContent, error) {
	return repo.download, nil
}

func (repo *serviceRepository) ListAuditEvents(_ context.Context, query operations.AuditQuery) ([]operations.StoredAuditEvent, error) {
	repo.auditQueries = append(repo.auditQueries, query)
	if repo.auditListErr != nil {
		return nil, repo.auditListErr
	}
	return append([]operations.StoredAuditEvent(nil), repo.auditRows...), nil
}

func (repo *serviceRepository) FindAuditExportReplay(_ context.Context, request operations.AuditExportReplayRequest) (operations.AuditExport, bool, error) {
	repo.replayRequests = append(repo.replayRequests, request)
	if repo.replayErr != nil {
		return operations.AuditExport{}, false, repo.replayErr
	}
	if repo.replay == nil {
		return operations.AuditExport{}, false, nil
	}
	return *repo.replay, true, nil
}

func (repo *serviceRepository) CreateAuditExport(_ context.Context, record operations.AuditExportRecord) (operations.AuditExport, error) {
	repo.exportRecord = record
	return operations.AuditExport{ID: record.ID, RuntimeRunID: record.RuntimeRun.ID, ArtifactID: record.ArtifactID,
		Format: record.Format, RowCount: record.RowCount, ContentDigest: record.ContentDigest,
		CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt}, nil
}

func (repo *serviceRepository) ListRuntimeRuns(_ context.Context, query operations.RuntimeQuery) ([]operationsdomain.RuntimeRun, error) {
	repo.runtimeQueries = append(repo.runtimeQueries, query)
	return append([]operationsdomain.RuntimeRun(nil), repo.runtimeRows...), nil
}

func (*serviceRepository) GetRuntimeRun(context.Context, identity.WorkspaceID, identity.RunID) (operationsdomain.RuntimeRun, error) {
	return operationsdomain.RuntimeRun{}, operationsdomain.ErrNotFound
}

func (*serviceRepository) ListRuntimeRunEvents(context.Context, identity.WorkspaceID, identity.RunID, int) ([]operationsdomain.RuntimeRunEvent, error) {
	return nil, nil
}

func (*serviceRepository) GetRuntimeSettings(context.Context, identity.WorkspaceID, time.Time) (operationsdomain.RuntimeSettings, error) {
	return operationsdomain.RuntimeSettings{}, nil
}

func (repo *serviceRepository) UpdateRuntimeSettings(_ context.Context, settings operationsdomain.RuntimeSettings, expected int64) (operationsdomain.RuntimeSettings, error) {
	repo.updatedSettings, repo.expectedVersion = settings, expected
	if repo.updateErr != nil {
		return operationsdomain.RuntimeSettings{}, repo.updateErr
	}
	return settings, nil
}

type serviceEvaluator struct {
	principal identity.PrincipalID
	requests  []authorizationapp.EvaluationRequest
}

func (evaluator *serviceEvaluator) Evaluate(_ context.Context, request authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	evaluator.requests = append(evaluator.requests, request)
	return authorization.Decision{Allowed: true, Action: request.Action, PrincipalID: evaluator.principal,
		ReasonCode: authorization.ReasonRoleGrant, AuthorizationVersion: 7}, nil
}

func TestAuditListUsesAuthorizedFiltersCursorAndRedactedProjection(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 11, 0, 0, 0, time.UTC)
	rows := make([]operations.StoredAuditEvent, 3)
	for index := range rows {
		eventID, _ := identity.NewEventID()
		rows[index] = operations.StoredAuditEvent{ID: eventID, WorkspaceID: workspace, EventType: "source.tested",
			ActorID: principal.String(), ObjectType: "source", ObjectID: "src_visible",
			TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", CreatedAt: now.Add(-time.Duration(index) * time.Minute),
			Payload: json.RawMessage(`{"objectType":"source","objectId":"src_visible","channel":"web","outcome":"failed","reasonCode":"TEST_FAILED","dsn":"postgres://user:secret@db/app","authorization":"Bearer private","prompt":"private prompt","nested":{"token":"secret"}}`)}
	}
	repository := &serviceRepository{auditRows: rows}
	evaluator := &serviceEvaluator{principal: principal}
	service := operations.NewService(repository, evaluator, operations.ClockFunc(func() time.Time { return now }), operationsdomain.DeploymentStatus{})
	access := operations.AccessRequest{WorkspaceID: workspace, PrincipalRef: principal.String(), TraceID: rows[0].TraceID}
	request := operations.AuditListRequest{AccessRequest: access, ActorID: principal.String(), EventType: "source.tested",
		ObjectType: "source", ObjectID: "src_visible", TraceFilter: rows[0].TraceID, Limit: 2}

	page, err := service.ListAuditEvents(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("page = %+v", page)
	}
	if len(repository.auditQueries) != 1 || repository.auditQueries[0].Limit != 3 || repository.auditQueries[0].ObjectID != "src_visible" {
		t.Fatalf("repository query = %+v", repository.auditQueries)
	}
	for _, secret := range []string{"dsn", "postgres://", "secret", "authorization", "Bearer", "prompt", "nested", "token"} {
		encoded, _ := json.Marshal(page.Items)
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("projected audit page leaked %q: %s", secret, encoded)
		}
	}
	if got := page.Items[0]; got.ObjectType != "source" || got.ObjectID != "src_visible" || got.Outcome != "failed" || got.ReasonCode != "TEST_FAILED" {
		t.Fatalf("projected event = %+v", got)
	}
	request.Cursor = page.NextCursor
	if _, err := service.ListAuditEvents(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if repository.auditQueries[1].After == nil || !repository.auditQueries[1].After.Time.Equal(rows[1].CreatedAt) || repository.auditQueries[1].After.ID != rows[1].ID.UUID() {
		t.Fatalf("decoded cursor = %+v", repository.auditQueries[1].After)
	}
	request.EventType = "different.event"
	if _, err := service.ListAuditEvents(context.Background(), request); !errors.Is(err, operationsdomain.ErrInvalidArgument) {
		t.Fatalf("cursor accepted with changed filter: %v", err)
	}
	if len(evaluator.requests) != 3 || evaluator.requests[0].Action != authorization.ActionAuditRead || evaluator.requests[0].WorkspaceID != workspace {
		t.Fatalf("authorization requests = %+v", evaluator.requests)
	}
}

func TestAuditExportUsesSameBoundedRedactedFilterAndRecordsRuntime(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	eventID, _ := identity.NewEventID()
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	repository := &serviceRepository{auditRows: []operations.StoredAuditEvent{{
		ID: eventID, WorkspaceID: workspace, EventType: "role.denied", ActorID: principal.String(),
		ObjectType: "role", ObjectID: "role_visible",
		Payload: json.RawMessage(`{"objectType":"role","objectId":"role_visible","outcome":"denied","reasonCode":"NO_MATCHING_GRANT","cookie":"private"}`),
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", CreatedAt: now,
	}}}
	evaluator := &serviceEvaluator{principal: principal}
	service := operations.NewService(repository, evaluator, operations.ClockFunc(func() time.Time { return now }), operationsdomain.DeploymentStatus{})
	result, err := service.CreateAuditExport(context.Background(), operations.CreateAuditExportRequest{
		AuditListRequest: operations.AuditListRequest{AccessRequest: operations.AccessRequest{WorkspaceID: workspace,
			PrincipalRef: principal.String(), TraceID: repository.auditRows[0].TraceID}, ObjectType: "role", ObjectID: "role_visible"},
		IdempotencyKey: "export-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RowCount != 1 || result.Format != "json" || result.ContentDigest == "" {
		t.Fatalf("export = %+v", result)
	}
	record := repository.exportRecord
	if record.RuntimeRun.Kind != operationsdomain.RunKindAuditExport || record.RuntimeRun.State != operationsdomain.RunSucceeded || record.RuntimeRun.FinishedAt == nil || record.RowCount != 1 {
		t.Fatalf("export record = %+v", record)
	}
	if len(record.RequestFingerprint) != 64 || !strings.Contains(string(record.Filters), `"actorId":""`) || strings.Contains(string(record.Filters), "T12:00:00+00:00") {
		t.Fatalf("canonical export request = fingerprint:%q filters:%s", record.RequestFingerprint, record.Filters)
	}
	if len(repository.auditQueries) != 1 || repository.auditQueries[0].Limit != operations.MaxExportRows+1 || repository.auditQueries[0].After != nil {
		t.Fatalf("export query = %+v", repository.auditQueries)
	}
	if strings.Contains(string(record.Filters), "cookie") || strings.Contains(string(record.Content), "private") ||
		strings.Contains(string(record.Content), "WorkspaceID") || strings.Contains(string(record.Content), "Details") {
		t.Fatalf("unsafe export metadata = %+v", record)
	}
	digest := sha256.Sum256(record.Content)
	if record.ContentDigest != hex.EncodeToString(digest[:]) || !strings.Contains(string(record.Content), `"objectType":"role"`) {
		t.Fatalf("export content/digest mismatch: %s %s", record.ContentDigest, record.Content)
	}
	if evaluator.requests[0].Action != authorization.ActionAuditRead {
		t.Fatalf("authorization action = %q", evaluator.requests[0].Action)
	}
}

func TestAuditExportReplayReturnsPersistedSnapshotBeforeListingCurrentAuditRows(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	exportID, _ := identity.NewEventID()
	runtimeID, _ := identity.NewRunID()
	now := time.Date(2026, 9, 5, 12, 10, 0, 0, time.UTC)
	persisted := operations.AuditExport{ID: exportID, RuntimeRunID: runtimeID, ArtifactID: "audit-export/old.json",
		Format: "json", RowCount: 2, ContentDigest: strings.Repeat("a", 64), CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	for _, test := range []struct {
		name      string
		auditRows []operations.StoredAuditEvent
		listErr   error
	}{
		{name: "current collection grew", auditRows: make([]operations.StoredAuditEvent, operations.MaxExportRows+1)},
		{name: "current collection read fails", listErr: errors.New("audit store unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &serviceRepository{replay: &persisted, auditRows: test.auditRows, auditListErr: test.listErr}
			service := operations.NewService(repository, &serviceEvaluator{principal: principal}, operations.ClockFunc(func() time.Time { return now }), operationsdomain.DeploymentStatus{})
			result, err := service.CreateAuditExport(context.Background(), operations.CreateAuditExportRequest{
				AuditListRequest: operations.AuditListRequest{AccessRequest: operations.AccessRequest{
					WorkspaceID: workspace, PrincipalRef: principal.String(), TraceID: strings.Repeat("a", 32)},
					ActorID: "  actor  ", ObjectType: " source "},
				IdempotencyKey: "existing-export",
			})
			if err != nil || result.ID != persisted.ID {
				t.Fatalf("replay result=%+v error=%v", result, err)
			}
			if len(repository.auditQueries) != 0 || len(repository.replayRequests) != 1 {
				t.Fatalf("replay order queries=%+v preflight=%+v", repository.auditQueries, repository.replayRequests)
			}
			preflight := repository.replayRequests[0]
			if preflight.CreatedByPrincipalID != principal || len(preflight.RequestFingerprint) != 64 ||
				!strings.Contains(string(preflight.Filters), `"actorId":"actor"`) || !strings.Contains(string(preflight.Filters), `"objectType":"source"`) {
				t.Fatalf("preflight = %+v", preflight)
			}
		})
	}
}

func TestAuditAndRuntimeCursorsBindWorkspaceAndValidateExpectedUUIDKind(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	otherWorkspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 12, 15, 0, 0, time.UTC)
	traceID := strings.Repeat("a", 32)
	auditRows := make([]operations.StoredAuditEvent, 2)
	for index := range auditRows {
		eventID, _ := identity.NewEventID()
		auditRows[index] = operations.StoredAuditEvent{ID: eventID, WorkspaceID: workspace,
			EventType: "audit.cursor", TraceID: traceID, CreatedAt: now.Add(-time.Duration(index) * time.Second)}
	}
	runtimeRows := make([]operationsdomain.RuntimeRun, 2)
	for index := range runtimeRows {
		runID, _ := identity.NewRunID()
		runtimeRows[index] = operationsdomain.RuntimeRun{ID: runID, WorkspaceID: workspace,
			Kind: operationsdomain.RunKindValidation, SourceType: "validation_run", SourceID: runID.String(),
			SourceVersionDigest: strings.Repeat("b", 64), IdempotencyKey: "cursor-" + runID.String(),
			State: operationsdomain.RunQueued, MaxAttempts: 1, Version: 1, CreatedAt: now, UpdatedAt: now.Add(-time.Duration(index) * time.Second)}
	}
	repository := &serviceRepository{auditRows: auditRows, runtimeRows: runtimeRows}
	service := operations.NewService(repository, &serviceEvaluator{principal: principal}, operations.ClockFunc(func() time.Time { return now }), operationsdomain.DeploymentStatus{})
	access := operations.AccessRequest{WorkspaceID: workspace, PrincipalRef: principal.String(), TraceID: traceID}
	auditPage, err := service.ListAuditEvents(context.Background(), operations.AuditListRequest{AccessRequest: access, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	runtimePage, err := service.ListRuns(context.Background(), operations.RuntimeListRequest{AccessRequest: access, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	for kind, cursor := range map[string]string{"audit": auditPage.NextCursor, "runtime": runtimePage.NextCursor} {
		decoded, decodeErr := base64.RawURLEncoding.DecodeString(cursor)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		var envelope map[string]any
		if json.Unmarshal(decoded, &envelope) != nil {
			t.Fatal("decode cursor envelope")
		}
		envelope["id"] = "00000000-0000-0000-0000-000000000000"
		tampered, _ := json.Marshal(envelope)
		tamperedCursor := base64.RawURLEncoding.EncodeToString(tampered)
		var listErr error
		if kind == "audit" {
			_, listErr = service.ListAuditEvents(context.Background(), operations.AuditListRequest{AccessRequest: access, Cursor: tamperedCursor, Limit: 1})
		} else {
			_, listErr = service.ListRuns(context.Background(), operations.RuntimeListRequest{AccessRequest: access, Cursor: tamperedCursor, Limit: 1})
		}
		if !errors.Is(listErr, operationsdomain.ErrInvalidArgument) {
			t.Fatalf("%s cursor with invalid UUID error = %v", kind, listErr)
		}
	}
	otherAccess := access
	otherAccess.WorkspaceID = otherWorkspace
	if _, err := service.ListAuditEvents(context.Background(), operations.AuditListRequest{AccessRequest: otherAccess, Cursor: auditPage.NextCursor, Limit: 1}); !errors.Is(err, operationsdomain.ErrInvalidArgument) {
		t.Fatalf("audit cursor crossed workspace: %v", err)
	}
	if _, err := service.ListRuns(context.Background(), operations.RuntimeListRequest{AccessRequest: otherAccess, Cursor: runtimePage.NextCursor, Limit: 1}); !errors.Is(err, operationsdomain.ErrInvalidArgument) {
		t.Fatalf("runtime cursor crossed workspace: %v", err)
	}
	if len(repository.auditQueries) != 1 || len(repository.runtimeQueries) != 1 {
		t.Fatalf("invalid cursors reached repository: audit=%+v runtime=%+v", repository.auditQueries, repository.runtimeQueries)
	}
}

func TestAuditAndRuntimeFiltersRejectUnboundedOrNonCanonicalInputs(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 12, 30, 0, 0, time.UTC)
	repository := &serviceRepository{}
	service := operations.NewService(repository, &serviceEvaluator{principal: principal}, operations.ClockFunc(func() time.Time { return now }), operationsdomain.DeploymentStatus{})
	access := operations.AccessRequest{WorkspaceID: workspace, PrincipalRef: principal.String(), TraceID: strings.Repeat("a", 32)}
	after, before := now.Add(time.Minute), now
	invalidAudit := []operations.AuditListRequest{
		{AccessRequest: access, ActorID: strings.Repeat("a", 129)},
		{AccessRequest: access, EventType: strings.Repeat("e", 129)},
		{AccessRequest: access, ObjectType: strings.Repeat("o", 65)},
		{AccessRequest: access, ObjectID: strings.Repeat("i", 129)},
		{AccessRequest: access, TraceFilter: "not-a-trace"},
		{AccessRequest: access, Cursor: strings.Repeat("c", 2049)},
		{AccessRequest: access, From: &after, To: &before},
	}
	for index, request := range invalidAudit {
		if _, err := service.ListAuditEvents(context.Background(), request); !errors.Is(err, operationsdomain.ErrInvalidArgument) {
			t.Fatalf("invalid audit filter %d error = %v", index, err)
		}
	}
	if _, err := service.CreateAuditExport(context.Background(), operations.CreateAuditExportRequest{
		AuditListRequest: operations.AuditListRequest{AccessRequest: access}, IdempotencyKey: strings.Repeat("k", 257),
	}); !errors.Is(err, operationsdomain.ErrInvalidArgument) {
		t.Fatalf("oversized export idempotency key error = %v", err)
	}
	invalidRuntime := []operations.RuntimeListRequest{
		{AccessRequest: access, SourceType: strings.Repeat("s", 65)},
		{AccessRequest: access, SourceID: strings.Repeat("i", 129)},
		{AccessRequest: access, TraceFilter: "not-a-trace"},
		{AccessRequest: access, Cursor: strings.Repeat("c", 2049)},
	}
	for index, request := range invalidRuntime {
		if _, err := service.ListRuns(context.Background(), request); !errors.Is(err, operationsdomain.ErrInvalidArgument) {
			t.Fatalf("invalid runtime filter %d error = %v", index, err)
		}
	}
	if len(repository.auditQueries) != 0 {
		t.Fatalf("invalid filters reached repository: %+v", repository.auditQueries)
	}
}

func TestRuntimeSettingsRejectStaleVersionWithoutChangingDeploymentBoundary(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	now := time.Date(2026, 9, 5, 13, 0, 0, 0, time.UTC)
	repository := &serviceRepository{updateErr: operationsdomain.ErrConflict}
	evaluator := &serviceEvaluator{principal: principal}
	deployment := operationsdomain.DeploymentStatus{WorkerConfigured: false, TelemetryConfigured: true,
		OIDCConfigured: true, EncryptionConfigured: true, AuditRetention: "deployment_managed"}
	service := operations.NewService(repository, evaluator, operations.ClockFunc(func() time.Time { return now }), deployment)
	_, err := service.UpdateRuntimeSettings(context.Background(), operations.UpdateRuntimeSettingsRequest{
		AccessRequest:   operations.AccessRequest{WorkspaceID: workspace, PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"},
		ExpectedVersion: 3, RetryCeiling: 4, StatementTimeoutMS: 30000, WebhookTimeoutMS: 10000,
		QueryRowLimit: 1000, QueryByteLimit: 1048576, RunMetadataRetentionDays: 30,
	})
	if !errors.Is(err, operationsdomain.ErrConflict) {
		t.Fatalf("update error = %v", err)
	}
	if repository.expectedVersion != 3 || repository.updatedSettings.Version != 4 {
		t.Fatalf("repository update = version %d expected %d", repository.updatedSettings.Version, repository.expectedVersion)
	}
	if evaluator.requests[0].Action != authorization.ActionRuntimeManage {
		t.Fatalf("authorization action = %q", evaluator.requests[0].Action)
	}
}
