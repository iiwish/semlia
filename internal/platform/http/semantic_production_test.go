package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	coredomain "github.com/iiwish/semlia/internal/domain"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/trace/noop"
)

type mockProductionRepo struct {
	mu           sync.Mutex
	operations   map[string]domain.ProductionOperation
	versions     map[string]domain.ProductionVersion
	targets      map[string][]domain.ProductionTarget
	links        map[string][]domain.ProductionCandidateLink
	contributors map[string][]domain.ProductionContributor
	commands     map[string]domain.ProductionCommand
	claims       map[string]domain.ProductionRequestClaim
	reservations map[string]domain.ProductionIdentityReservation
}

func (m *mockProductionRepo) CheckProductionInput(context.Context, domain.ProductionVersion, []domain.ProductionTarget) error {
	return nil
}

func (m *mockProductionRepo) PrepareProductionBaseline(context.Context, identity.WorkspaceID, identity.PrincipalID, []domain.TargetDeclaration) (domain.ProductionBaseline, error) {
	return domain.ProductionBaseline{Head: domain.HeadReference{Presence: "absent"}, CanonicalJSON: json.RawMessage(`{"head":{"presence":"absent"},"pins":[]}`), Targets: map[string]domain.ProductionTargetBaseline{}}, nil
}

func newMockProductionRepo() *mockProductionRepo {
	return &mockProductionRepo{
		operations:   make(map[string]domain.ProductionOperation),
		versions:     make(map[string]domain.ProductionVersion),
		targets:      make(map[string][]domain.ProductionTarget),
		links:        make(map[string][]domain.ProductionCandidateLink),
		contributors: make(map[string][]domain.ProductionContributor),
		commands:     make(map[string]domain.ProductionCommand),
		claims:       make(map[string]domain.ProductionRequestClaim),
		reservations: make(map[string]domain.ProductionIdentityReservation),
	}
}

func (m *mockProductionRepo) CreateProductionOperationTx(
	ctx context.Context,
	op domain.ProductionOperation,
	ver domain.ProductionVersion,
	targets []domain.ProductionTarget,
	links []domain.ProductionCandidateLink,
	contribs []domain.ProductionContributor,
	reservations []domain.ProductionIdentityReservation,
	cmd domain.ProductionCommand,
	claim domain.ProductionRequestClaim,
	proposals []domain.Proposal,
	assetDrafts []governanceapp.SemanticAssetDraft,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	opKey := fmt.Sprintf("%s:%s", op.WorkspaceID, op.ID)
	m.operations[opKey] = op
	m.versions[opKey] = ver
	m.targets[opKey] = targets
	m.links[opKey] = links
	m.contributors[opKey] = contribs

	cmdKey := fmt.Sprintf("%s:%s:%s:%s", cmd.WorkspaceID, cmd.PrincipalID, cmd.CommandKind, cmd.IdempotencyKey)
	m.commands[cmdKey] = cmd

	claimKey := fmt.Sprintf("%s:%s", claim.WorkspaceID, claim.BusinessDigest)
	m.claims[claimKey] = claim

	for _, res := range reservations {
		resKey := fmt.Sprintf("%s:%s:%s", res.WorkspaceID, res.Kind, res.IdentityKey)
		m.reservations[resKey] = res
	}

	return nil
}

func (m *mockProductionRepo) GetProductionOperation(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
) (domain.ProductionOperation, domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, []domain.ProductionContributor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", workspace, opID)
	op, ok := m.operations[key]
	if !ok {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, domain.ErrNotFound
	}
	ver := m.versions[key]
	targets := m.targets[key]
	links := m.links[key]
	contribs := m.contributors[key]

	return op, ver, targets, links, contribs, nil
}

func (m *mockProductionRepo) GetProductionOperationVersion(ctx context.Context, workspace identity.WorkspaceID, opID identity.ProductionOperationID, version int) (domain.ProductionOperation, domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, []domain.ProductionContributor, error) {
	op, ver, targets, links, contributors, err := m.GetProductionOperation(ctx, workspace, opID)
	if err != nil || ver.Version != version {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, domain.ErrNotFound
	}
	return op, ver, targets, links, contributors, nil
}

func (m *mockProductionRepo) ListProductionOperationsPage(ctx context.Context, workspace identity.WorkspaceID, query domain.ProductionListQuery) ([]domain.ProductionOperation, error) {
	return m.ListProductionOperations(ctx, workspace, query.Limit, 0)
}

func (m *mockProductionRepo) ListProductionOperations(
	ctx context.Context,
	workspace identity.WorkspaceID,
	limit int,
	offset int,
) ([]domain.ProductionOperation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []domain.ProductionOperation
	for _, op := range m.operations {
		if op.WorkspaceID == workspace {
			result = append(result, op)
		}
	}
	if offset >= len(result) {
		return nil, nil
	}
	end := offset + limit
	if end > len(result) {
		end = len(result)
	}
	return result[offset:end], nil
}

func (m *mockProductionRepo) GetProductionCommand(
	ctx context.Context,
	workspace identity.WorkspaceID,
	principal identity.PrincipalID,
	cmdKind string,
	idempotencyKey string,
) (*domain.ProductionCommand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s:%s", workspace, principal, cmdKind, idempotencyKey)
	cmd, ok := m.commands[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &cmd, nil
}

func (m *mockProductionRepo) GetProductionRequestClaim(
	ctx context.Context,
	workspace identity.WorkspaceID,
	businessDigest string,
) (*domain.ProductionRequestClaim, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", workspace, businessDigest)
	claim, ok := m.claims[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &claim, nil
}

func (m *mockProductionRepo) GetProductionReservation(
	ctx context.Context,
	workspace identity.WorkspaceID,
	kind string,
	identityKey string,
) (*domain.ProductionIdentityReservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s", workspace, kind, identityKey)
	res, ok := m.reservations[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &res, nil
}

func (m *mockProductionRepo) ReplaceDraftTx(
	ctx context.Context,
	opID identity.ProductionOperationID,
	newVer domain.ProductionVersion,
	targets []domain.ProductionTarget,
	links []domain.ProductionCandidateLink,
	cmd domain.ProductionCommand,
	proposals []domain.Proposal,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	opKey := fmt.Sprintf("%s:%s", newVer.WorkspaceID, opID)
	op, ok := m.operations[opKey]
	if !ok {
		return domain.ErrNotFound
	}
	op.CurrentVersion = newVer.Version
	op.UpdatedAt = time.Now().UTC()
	m.operations[opKey] = op
	m.versions[opKey] = newVer
	m.targets[opKey] = targets
	m.links[opKey] = links

	cmdKey := fmt.Sprintf("%s:%s:%s:%s", cmd.WorkspaceID, cmd.PrincipalID, cmd.CommandKind, cmd.IdempotencyKey)
	m.commands[cmdKey] = cmd

	return nil
}

func (m *mockProductionRepo) SubmitOperationTx(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	submittedAt time.Time,
	attempt domain.ValidationAttempt,
	runs []domain.ValidationRun,
	bindings []domain.ValidationBinding,
	proposals []domain.Proposal,
	cmd domain.ProductionCommand,
) error {
	return nil
}

func (m *mockProductionRepo) CreateValidationAttemptTx(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	attempt domain.ValidationAttempt,
	runs []domain.ValidationRun,
	bindings []domain.ValidationBinding,
	cmd domain.ProductionCommand,
) error {
	return nil
}

func (m *mockProductionRepo) GetValidationAttempt(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	attemptNo int,
) (*domain.ValidationAttempt, error) {
	return nil, domain.ErrNotFound
}

func (m *mockProductionRepo) GetLatestValidationAttempt(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
) (*domain.ValidationAttempt, error) {
	return nil, domain.ErrNotFound
}

func (m *mockProductionRepo) ListValidationAttempts(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	limit int,
	offset int,
) ([]domain.ValidationAttempt, error) {
	return nil, nil
}

func (m *mockProductionRepo) ReviewOperationTx(
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
	return nil
}

func (m *mockProductionRepo) PublishOperationTx(
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
	return nil
}

func (m *mockProductionRepo) RollbackOperationTx(
	ctx context.Context,
	workspace identity.WorkspaceID,
	release domain.Release,
	releaseProposals []domain.ReleaseProposal,
	manifest domain.ProductionReleaseManifest,
	beforePins []domain.ProductionReleaseBeforePin,
	cmd domain.ProductionCommand,
) error {
	return nil
}

func (m *mockProductionRepo) GetLatestRelease(
	ctx context.Context,
	workspace identity.WorkspaceID,
) (*domain.Release, error) {
	return nil, nil
}

func (m *mockProductionRepo) GetRelease(
	ctx context.Context,
	workspace identity.WorkspaceID,
	releaseID identity.ReleaseID,
) (domain.Release, error) {
	return domain.Release{}, domain.ErrNotFound
}

func (m *mockProductionRepo) GetProductionRelease(
	ctx context.Context,
	workspace identity.WorkspaceID,
	releaseID identity.ReleaseID,
) (*domain.Release, *domain.ProductionReleaseManifest, []domain.ProductionReleaseBeforePin, []domain.ReleaseProposal, error) {
	return nil, nil, nil, nil, domain.ErrNotFound
}

type productionHTTPAuthorizationRepository struct {
	authapp.Repository
	workspace identity.WorkspaceID
	principal identity.PrincipalID
}

func (r *productionHTTPAuthorizationRepository) LoadPrincipal(_ context.Context, w identity.WorkspaceID, p identity.PrincipalID) (authz.Principal, error) {
	if w != r.workspace || p != r.principal {
		return authz.Principal{}, authz.ErrNotFound
	}
	return authz.Principal{ID: p, WorkspaceID: w, Kind: authz.PrincipalHuman, Status: authz.PrincipalActive}, nil
}
func (r *productionHTTPAuthorizationRepository) LoadPrincipalBindings(context.Context, identity.PrincipalID) ([]authz.RoleBinding, error) {
	return []authz.RoleBinding{{WorkspaceID: r.workspace, PrincipalID: r.principal, RoleID: "workspace_admin", RoleVersion: 1, ScopeType: authz.ScopeWorkspace, ScopeID: r.workspace.UUID(), GrantedAt: time.Now().Add(-time.Hour), Actions: []authz.Action{authz.ActionAssetRead, authz.ActionAssetPropose, authz.ActionBindingRead, authz.ActionBindingManage}}}, nil
}
func (*productionHTTPAuthorizationRepository) AuthorizationVersion(context.Context, identity.WorkspaceID) (int64, error) {
	return 1, nil
}
func (*productionHTTPAuthorizationRepository) RecordDecision(context.Context, authz.DecisionEvent) error {
	return nil
}

func setupTestServer(t *testing.T, w identity.WorkspaceID, p identity.PrincipalID) (http.Handler, *mockProductionRepo) {
	t.Helper()
	repo := newMockProductionRepo()
	prodService := governanceapp.NewProductionService(repo)
	sysService := application.NewSystemService(nil, coredomain.SystemInfo{
		APIVersion:    "v1",
		SchemaVersion: "0.1.0",
		BuildVersion:  "test-build",
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tracer := noop.NewTracerProvider().Tracer("test")

	authorizer := authapp.NewService(&productionHTTPAuthorizationRepository{workspace: w, principal: p}, authapp.ClockFunc(time.Now))
	handler := httpapi.NewHandler(sysService, logger, tracer, httpapi.WithProduction(prodService), httpapi.WithAuthorization(authorizer))
	return handler, repo
}

func mustWorkspaceID(t *testing.T) identity.WorkspaceID {
	t.Helper()
	id, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustPrincipalID(t *testing.T) identity.PrincipalID {
	t.Helper()
	id, err := identity.NewPrincipalID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestProductionOperations_IdempotencyAndValidation(t *testing.T) {
	wspID := mustWorkspaceID(t)
	principalID := mustPrincipalID(t)
	handler, _ := setupTestServer(t, wspID, principalID)

	path := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", wspID)

	t.Run("endpoint disabled returns 503 DEPENDENCY_UNAVAILABLE", func(t *testing.T) {
		sysService := application.NewSystemService(nil, coredomain.SystemInfo{
			APIVersion:    "v1",
			SchemaVersion: "0.1.0",
			BuildVersion:  "test-build",
		})
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		tracer := noop.NewTracerProvider().Tracer("test")
		disabledHandler := httpapi.NewHandler(sysService, logger, tracer) // no WithProduction

		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Semlia-Principal", principalID.String())
		rec := httptest.NewRecorder()
		disabledHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 when disabled, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing idempotency key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Semlia-Principal", principalID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing idempotency key, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("short idempotency key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "short")
		req.Header.Set("X-Semlia-Principal", principalID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for short idempotency key, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing authentication", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "valid-key-12345")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for unauthenticated request, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("trailing characters after json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"input":{},"targets":[]}extra`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "valid-key-12345")
		req.Header.Set("X-Semlia-Principal", principalID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for trailing json, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("duplicate keys rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"input":{},"input":{},"targets":[]}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "valid-key-12345")
		req.Header.Set("X-Semlia-Principal", principalID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for duplicate keys, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestProductionOperations_CreateGetReplaceList(t *testing.T) {
	wspID := mustWorkspaceID(t)
	principalID := mustPrincipalID(t)
	handler, _ := setupTestServer(t, wspID, principalID)

	createPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations", wspID)

	createPayload := fmt.Sprintf(`{
		"input": {
			"snapshots": [], "evidence": [], "dependencies": [],
			"candidates": [
				{
					"candidateId": "scd_01arz3ndektsv4rrffq69g5fav",
					"digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
					"primaryTargetKey": "net_revenue",
					"targetKeys": ["net_revenue"]
				}
			]
		},
		"targets": [
			{
				"intent": "create",
				"kind": "semantic_asset",
				"localKey": "net_revenue",
				"identityKey": "finance.net_revenue",
				"title": "Net Revenue",
				"changes": [], "evidenceIds": [],
				"content": {"address":"finance.net_revenue","assetType":"metric","displayName":"Net Revenue","definition": "Revenue after refunds","scope":null,"ownerPrincipalId":%q}
			}
		]
	}`, principalID.String())

	var opID string

	// 1. Create operation
	t.Run("create operation succeeds", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, createPath, bytes.NewBufferString(createPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "create-key-00001")
		req.Header.Set("X-Semlia-Principal", principalID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}

		location := rec.Header().Get("Location")
		if !strings.Contains(location, "/production-operations/prodop_") {
			t.Fatalf("expected Location header with prodop, got %q", location)
		}

		var result map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["replayed"] != false {
			t.Errorf("expected replayed=false")
		}
		if result["version"].(float64) != 1 {
			t.Errorf("expected version 1, got %v", result["version"])
		}
		opID = result["operationId"].(string)
		if !strings.HasPrefix(opID, "prodop_") {
			t.Fatalf("expected prodop_ prefix, got %q", opID)
		}
	})

	// 2. Same-key Replay
	t.Run("idempotent replay returns 200 with replayed=true", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, createPath, bytes.NewBufferString(createPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "create-key-00001")
		req.Header.Set("X-Semlia-Principal", principalID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for replay, got %d: %s", rec.Code, rec.Body.String())
		}

		var result map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["replayed"] != true {
			t.Errorf("expected replayed=true on replay")
		}
		if result["operationId"] != opID {
			t.Errorf("expected same operationId %s, got %v", opID, result["operationId"])
		}
	})

	// 3. Different key same business content -> ALREADY_PRODUCED (409)
	t.Run("different key duplicate content yields ALREADY_PRODUCED", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, createPath, bytes.NewBufferString(createPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "create-key-00002")
		req.Header.Set("X-Semlia-Principal", principalID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict for already produced, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "ALREADY_PRODUCED") {
			t.Errorf("expected ALREADY_PRODUCED error code, got %s", rec.Body.String())
		}
	})

	// 4. GET operation
	t.Run("get operation returns authoritative draft", func(t *testing.T) {
		getPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s", wspID, opID)
		req := httptest.NewRequest(http.MethodGet, getPath, nil)
		req.Header.Set("X-Semlia-Principal", principalID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var opRes map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &opRes); err != nil {
			t.Fatal(err)
		}
		if opRes["version"].(float64) != 1 {
			t.Errorf("expected version 1, got %v", opRes["version"])
		}
		summary := opRes["summary"].(map[string]any)
		if summary["id"] != opID {
			t.Errorf("expected summary id %s, got %v", opID, summary["id"])
		}
		targets := opRes["targets"].([]any)
		if len(targets) != 1 {
			t.Fatalf("expected 1 target, got %d", len(targets))
		}
		target0 := targets[0].(map[string]any)
		if target0["localKey"] != "net_revenue" {
			t.Errorf("expected localKey net_revenue, got %v", target0["localKey"])
		}
	})

	// 5. PUT replace draft
	t.Run("replace draft advances version to 2", func(t *testing.T) {
		replacePath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s", wspID, opID)
		replacePayload := fmt.Sprintf(`{
			"expectedVersion": 1,
			"input": {
				"snapshots": [], "evidence": [], "dependencies": [],
				"candidates": [
					{
						"candidateId": "scd_01arz3ndektsv4rrffq69g5fav",
						"digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
						"primaryTargetKey": "net_revenue",
						"targetKeys": ["net_revenue"]
					}
				]
			},
			"targets": [
				{
					"intent": "create",
					"kind": "semantic_asset",
					"localKey": "net_revenue",
					"identityKey": "finance.net_revenue",
					"title": "Net Revenue Updated",
					"changes": [], "evidenceIds": [],
					"content": {"address":"finance.net_revenue","assetType":"metric","displayName":"Net Revenue","definition": "Revenue after refunds and disputes","scope":null,"ownerPrincipalId":%q}
				}
			]
		}`, principalID.String())
		req := httptest.NewRequest(http.MethodPut, replacePath, bytes.NewBufferString(replacePayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "replace-key-00001")
		req.Header.Set("X-Semlia-Principal", principalID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for replace, got %d: %s", rec.Code, rec.Body.String())
		}

		var result map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["version"].(float64) != 2 {
			t.Errorf("expected version 2 after replace, got %v", result["version"])
		}
		if result["outcome"] != "updated" {
			t.Errorf("expected outcome updated, got %v", result["outcome"])
		}
	})

	// 6. PUT with expectedVersion mismatch returns 409
	t.Run("replace draft with wrong expectedVersion returns 409", func(t *testing.T) {
		replacePath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations/%s", wspID, opID)
		badPayload := strings.Replace(createPayload, `"input":`, `"expectedVersion":1,"input":`, 1)
		req := httptest.NewRequest(http.MethodPut, replacePath, bytes.NewBufferString(badPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "replace-key-00002")
		req.Header.Set("X-Semlia-Principal", principalID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// 7. List operations
	t.Run("list operations returns page", func(t *testing.T) {
		listPath := fmt.Sprintf("/api/v1/workspaces/%s/production-operations?limit=10", wspID)
		req := httptest.NewRequest(http.MethodGet, listPath, nil)
		req.Header.Set("X-Semlia-Principal", principalID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var page map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		items := page["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("expected 1 operation, got %d", len(items))
		}
	})
}
