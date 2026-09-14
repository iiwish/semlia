package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/discovery"
	ingestiondomain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
)

const DiscoveryJobType = "source.discovery.v1"

type Clock interface{ Now() time.Time }

type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

type SourceRepository interface {
	CreateDiscoverySource(context.Context, CreateSourceCommand) (domain.Source, error)
	GetDiscoverySource(context.Context, identity.WorkspaceID, identity.SourceConnectionID) (domain.Source, error)
	GetDiscoverySourceAuthorized(context.Context, identity.WorkspaceID, identity.SourceConnectionID, int64) (domain.Source, error)
	ListDiscoverySources(context.Context, ListSourcesQuery) (domain.SourcePage, error)
	UpdateDiscoverySource(context.Context, UpdateSourceCommand) (domain.Source, error)
	RotateDiscoveryCredential(context.Context, RotateCredentialCommand) (domain.Source, error)
	LoadDiscoveryCredential(context.Context, identity.WorkspaceID, identity.SourceConnectionID, int64) (domain.CredentialEnvelope, error)
	LoadDiscoverySourceCredentialAuthorized(context.Context, identity.WorkspaceID, identity.SourceConnectionID, int64) (domain.Source, domain.CredentialEnvelope, error)
	CreateQueuedDiscoveryRun(context.Context, CreateRunCommand) (domain.Run, error)
	ListDiscoveryRuns(context.Context, ListRunsQuery) (domain.RunPage, error)
	LoadDiscoveryRunExecution(context.Context, identity.WorkspaceID, identity.RunID) (RunExecution, error)
	BeginDiscoveryRun(context.Context, identity.WorkspaceID, identity.RunID, identity.RunID, time.Time) error
	RetryOrFailDiscoveryRun(context.Context, identity.WorkspaceID, identity.RunID, identity.RunID, bool, string, time.Time) error
	PersistDiscoveryRunSnapshot(context.Context, identity.WorkspaceID, identity.SourceConnectionID, identity.RunID, domain.Snapshot) (PersistResult, error)
	ListDiscoveryCandidates(context.Context, ListCandidatesQuery) (domain.CandidatePage, error)
	GetDiscoveryCandidate(context.Context, identity.WorkspaceID, identity.SemanticCandidateID, int64) (domain.Candidate, error)
	LookupDiscoveryCandidateSource(context.Context, identity.WorkspaceID, identity.SemanticCandidateID) (identity.SourceConnectionID, error)
	DecideDiscoveryCandidate(context.Context, DecideCandidateCommand) (domain.CandidateDecision, error)
}

type Collector interface {
	Collect(context.Context, domain.Source, domain.CredentialSecret, time.Time) (domain.Snapshot, error)
	Test(context.Context, domain.Source, domain.CredentialSecret) error
}

type CreateSourceCommand struct {
	Source                       domain.Source
	Credential                   domain.CredentialEnvelope
	TraceID                      string
	ExpectedAuthorizationVersion int64
}

type UpdateSourceCommand struct {
	WorkspaceID                  identity.WorkspaceID
	SourceID                     identity.SourceConnectionID
	Name                         string
	ArtifactPaths                []string
	Status                       string
	UpdatedAt                    time.Time
	ExpectedVersion              int64
	ExpectedAuthorizationVersion int64
	Actor                        string
	TraceID                      string
	ContentRetentionUntil        time.Time
}

type RotateCredentialCommand struct {
	WorkspaceID                  identity.WorkspaceID
	SourceID                     identity.SourceConnectionID
	ExpectedVersion              int64
	ExpectedSourceVersion        int64
	Credential                   domain.CredentialEnvelope
	UpdatedAt                    time.Time
	TraceID                      string
	ExpectedAuthorizationVersion int64
}

type CreateRunCommand struct {
	RunID                        identity.RunID
	JobID                        identity.RunID
	WorkspaceID                  identity.WorkspaceID
	SourceID                     identity.SourceConnectionID
	CredentialVersion            int64
	RequestedBy                  string
	IdempotencyKey               string
	TraceID                      string
	CreatedAt                    time.Time
	ExpectedAuthorizationVersion int64
}

type RunExecution struct {
	Run        domain.Run
	Source     domain.Source
	Credential domain.CredentialEnvelope
	Artifacts  []ingestiondomain.ArtifactMember
}

type RunCommitFence struct {
	JobID      identity.RunID
	LeaseOwner string
}

type runCommitFenceContextKey struct{}

func WithRunCommitFence(ctx context.Context, fence RunCommitFence) context.Context {
	return context.WithValue(ctx, runCommitFenceContextKey{}, fence)
}

func RunCommitFenceFromContext(ctx context.Context) (RunCommitFence, bool) {
	fence, ok := ctx.Value(runCommitFenceContextKey{}).(RunCommitFence)
	return fence, ok
}

type DecideCandidateCommand struct {
	Decision                     domain.CandidateDecision
	ExpectedStatus               string
	ExpectedAuthorizationVersion int64
}

type CreateSourceRequest struct {
	WorkspaceID   identity.WorkspaceID
	Name          string
	Host          string
	Port          int
	Database      string
	Username      string
	Password      string
	SSLMode       string
	ArtifactPaths []string
	PrincipalRef  string
	TraceID       string
}

type UpdateSourceRequest struct {
	WorkspaceID           identity.WorkspaceID
	SourceID              identity.SourceConnectionID
	Name                  string
	ArtifactPaths         []string
	Status                string
	PrincipalRef          string
	TraceID               string
	ExpectedVersion       int64
	ExpectedSourceVersion int64
}

type SourceRequest struct {
	WorkspaceID  identity.WorkspaceID
	SourceID     identity.SourceConnectionID
	PrincipalRef string
	TraceID      string
}

type ListRunsRequest struct {
	SourceRequest
	Cursor string
	Limit  int
}

type ListSourcesRequest struct {
	WorkspaceID  identity.WorkspaceID
	SourceID     *identity.SourceConnectionID
	PrincipalRef string
	TraceID      string
	Cursor       string
	Limit        int
}

type ListSourcesQuery struct {
	WorkspaceID          identity.WorkspaceID
	SourceID             *identity.SourceConnectionID
	PrincipalID          identity.PrincipalID
	AuthorizationVersion int64
	Cursor               string
	Limit                int
}

type ListRunsQuery struct {
	WorkspaceID          identity.WorkspaceID
	SourceID             identity.SourceConnectionID
	PrincipalID          identity.PrincipalID
	AuthorizationVersion int64
	Cursor               string
	Limit                int
}

type RotateCredentialRequest struct {
	SourceRequest
	Password        string
	ExpectedVersion int64
}

type StartRunRequest struct {
	SourceRequest
	IdempotencyKey string
}

type ListCandidateRequest struct {
	WorkspaceID  identity.WorkspaceID
	SourceID     *identity.SourceConnectionID
	Status       string
	PrincipalRef string
	TraceID      string
	Cursor       string
	Limit        int
}

type ListCandidatesQuery struct {
	WorkspaceID          identity.WorkspaceID
	SourceID             *identity.SourceConnectionID
	Status               string
	PrincipalID          identity.PrincipalID
	AuthorizationVersion int64
	Cursor               string
	Limit                int
}

type DecideCandidateRequest struct {
	WorkspaceID    identity.WorkspaceID
	CandidateID    identity.SemanticCandidateID
	Action         string
	ProposalID     *identity.ProposalID
	Reason         string
	IdempotencyKey string
	PrincipalRef   string
	TraceID        string
}

type ControlService struct {
	repository        SourceRepository
	cipher            *CredentialCipher
	collector         Collector
	artifacts         *ArtifactLoader
	sqlAdapter        domain.Adapter
	authorizer        authorizationapp.Evaluator
	clock             Clock
	artifactStore     ingestiondomain.ArtifactStore
	artifactAdapters  map[ingestiondomain.ArtifactKind]domain.Adapter
	artifactRetention time.Duration
	snapshotCursorKey []byte
}

func (service *ControlService) WithArtifactStore(store ingestiondomain.ArtifactStore, adapters map[ingestiondomain.ArtifactKind]domain.Adapter) *ControlService {
	service.artifactStore = store
	service.artifactAdapters = adapters
	return service
}

func NewControlService(repository SourceRepository, cipher *CredentialCipher, collector Collector,
	artifacts *ArtifactLoader, sqlAdapter domain.Adapter, authorizer authorizationapp.Evaluator, clock Clock,
) *ControlService {
	if repository == nil || artifacts == nil || clock == nil {
		panic("discovery control dependencies are required")
	}
	return &ControlService{repository: repository, cipher: cipher, collector: collector, artifacts: artifacts,
		sqlAdapter: sqlAdapter, authorizer: authorizer, clock: clock, artifactRetention: 30 * 24 * time.Hour, snapshotCursorKey: newSnapshotCursorKey()}
}

func (service *ControlService) WithArtifactRetention(retention time.Duration) *ControlService {
	if retention >= 24*time.Hour && retention <= 365*24*time.Hour {
		service.artifactRetention = retention
	}
	return service
}

func (service *ControlService) CreateSource(ctx context.Context, request CreateSourceRequest) (domain.Source, error) {
	if service.cipher == nil {
		return domain.Source{}, domain.ErrCredential
	}
	authVersion, err := service.authorizeVersion(ctx, request.WorkspaceID, authorization.ActionSourceManage,
		authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.Source{}, err
	}
	request.Name, request.Host, request.Database, request.Username, request.SSLMode = strings.TrimSpace(request.Name),
		strings.TrimSpace(request.Host), strings.TrimSpace(request.Database), strings.TrimSpace(request.Username), strings.TrimSpace(request.SSLMode)
	paths, err := normalizeArtifactPaths(request.ArtifactPaths)
	if err != nil || !validSourceInput(request) {
		return domain.Source{}, domain.ErrInvalidInput
	}
	sourceID, err := identity.NewSourceConnectionID()
	if err != nil {
		return domain.Source{}, err
	}
	credentialID, err := identity.NewSourceCredentialID()
	if err != nil {
		return domain.Source{}, err
	}
	now := service.clock.Now().UTC()
	source := domain.Source{ID: sourceID, WorkspaceID: request.WorkspaceID, Name: request.Name, Host: request.Host,
		Port: request.Port, Database: request.Database, Username: request.Username, SSLMode: request.SSLMode,
		ArtifactPaths: paths, Status: "active", ActiveCredentialVersion: 1, CreatedAt: now, UpdatedAt: now}
	nonce, ciphertext, keyVersion, err := service.cipher.Seal(request.WorkspaceID, sourceID, 1, domain.CredentialSecret{Password: request.Password})
	if err != nil {
		return domain.Source{}, err
	}
	return service.repository.CreateDiscoverySource(ctx, CreateSourceCommand{Source: source, Credential: domain.CredentialEnvelope{
		ID: credentialID, WorkspaceID: request.WorkspaceID, SourceConnectionID: sourceID, Version: 1,
		KeyVersion: keyVersion, Algorithm: "AES-256-GCM", Nonce: nonce, Ciphertext: ciphertext,
		CreatedBy: strings.TrimSpace(request.PrincipalRef), CreatedAt: now,
	}, TraceID: request.TraceID, ExpectedAuthorizationVersion: authVersion})
}

func (service *ControlService) GetSource(ctx context.Context, request SourceRequest) (domain.Source, error) {
	authVersion, err := service.authorizeVersion(ctx, request.WorkspaceID, authorization.ActionSourceRead,
		authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.Source{}, err
	}
	return service.repository.GetDiscoverySourceAuthorized(ctx, request.WorkspaceID, request.SourceID, authVersion)
}

func (service *ControlService) ListSources(ctx context.Context, request ListSourcesRequest) (domain.SourcePage, error) {
	resource := authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()}
	if request.SourceID != nil {
		resource = authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}
	}
	authVersion, err := service.authorizeVersion(ctx, request.WorkspaceID, authorization.ActionSourceRead,
		resource, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.SourcePage{}, err
	}
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(request.PrincipalRef))
	if err != nil {
		return domain.SourcePage{}, domain.ErrInvalidInput
	}
	if request.Limit == 0 {
		request.Limit = 50
	}
	if request.Limit < 1 || request.Limit > 100 {
		return domain.SourcePage{}, domain.ErrInvalidInput
	}
	return service.repository.ListDiscoverySources(ctx, ListSourcesQuery{WorkspaceID: request.WorkspaceID,
		SourceID: request.SourceID, PrincipalID: principal, AuthorizationVersion: authVersion, Cursor: request.Cursor, Limit: request.Limit})
}

func (service *ControlService) UpdateSource(ctx context.Context, request UpdateSourceRequest) (domain.Source, error) {
	authVersion, err := service.authorizeVersion(ctx, request.WorkspaceID, authorization.ActionSourceManage,
		authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.Source{}, err
	}
	paths, err := normalizeArtifactPaths(request.ArtifactPaths)
	request.Name, request.Status = strings.TrimSpace(request.Name), strings.TrimSpace(request.Status)
	if err != nil || request.ExpectedVersion < 1 || request.Name == "" || len(request.Name) > 120 || (request.Status != "active" && request.Status != "paused" && request.Status != "deleted") {
		return domain.Source{}, domain.ErrInvalidInput
	}
	return service.repository.UpdateDiscoverySource(ctx, UpdateSourceCommand{WorkspaceID: request.WorkspaceID, SourceID: request.SourceID,
		Name: request.Name, ArtifactPaths: paths, Status: request.Status, UpdatedAt: service.clock.Now().UTC(), ExpectedVersion: request.ExpectedVersion,
		ExpectedAuthorizationVersion: authVersion, Actor: strings.TrimSpace(request.PrincipalRef), TraceID: request.TraceID,
		ContentRetentionUntil: service.clock.Now().UTC().Add(service.artifactRetention)})
}

func (service *ControlService) RotateCredential(ctx context.Context, request RotateCredentialRequest) (domain.Source, error) {
	if service.cipher == nil {
		return domain.Source{}, domain.ErrCredential
	}
	authVersion, err := service.authorizeVersion(ctx, request.WorkspaceID, authorization.ActionSourceManage,
		authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.Source{}, err
	}
	if strings.TrimSpace(request.Password) == "" || request.ExpectedVersion < 1 {
		return domain.Source{}, domain.ErrInvalidInput
	}
	source, err := service.repository.GetDiscoverySourceAuthorized(ctx, request.WorkspaceID, request.SourceID, authVersion)
	if err != nil {
		return domain.Source{}, err
	}
	if source.Version != request.ExpectedVersion || source.Kind != "postgresql" {
		return domain.Source{}, domain.ErrConflict
	}
	version := source.ActiveCredentialVersion + 1
	credentialID, err := identity.NewSourceCredentialID()
	if err != nil {
		return domain.Source{}, err
	}
	nonce, ciphertext, keyVersion, err := service.cipher.Seal(request.WorkspaceID, request.SourceID, version, domain.CredentialSecret{Password: request.Password})
	if err != nil {
		return domain.Source{}, err
	}
	now := service.clock.Now().UTC()
	return service.repository.RotateDiscoveryCredential(ctx, RotateCredentialCommand{WorkspaceID: request.WorkspaceID,
		SourceID: request.SourceID, ExpectedVersion: source.ActiveCredentialVersion, ExpectedSourceVersion: request.ExpectedVersion,
		ExpectedAuthorizationVersion: authVersion, UpdatedAt: now,
		TraceID: request.TraceID, Credential: domain.CredentialEnvelope{ID: credentialID, WorkspaceID: request.WorkspaceID,
			SourceConnectionID: request.SourceID, Version: version, KeyVersion: keyVersion, Algorithm: "AES-256-GCM",
			Nonce: nonce, Ciphertext: ciphertext, CreatedBy: strings.TrimSpace(request.PrincipalRef), CreatedAt: now}})
}

func (service *ControlService) TestSource(ctx context.Context, request SourceRequest) error {
	if service.cipher == nil || service.collector == nil {
		return domain.ErrCredential
	}
	authVersion, err := service.authorizeVersion(ctx, request.WorkspaceID, authorization.ActionSourceManage,
		authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return err
	}
	source, envelope, err := service.repository.LoadDiscoverySourceCredentialAuthorized(
		ctx, request.WorkspaceID, request.SourceID, authVersion,
	)
	if err != nil {
		return err
	}
	secret, err := service.cipher.Open(envelope)
	if err != nil {
		return err
	}
	return service.collector.Test(ctx, source, secret)
}

func (service *ControlService) StartRun(ctx context.Context, request StartRunRequest) (domain.Run, error) {
	authorizationVersion, err := service.authorizeVersion(ctx, request.WorkspaceID, authorization.ActionIngestionRun,
		authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.Run{}, err
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" || len(request.IdempotencyKey) > 256 {
		return domain.Run{}, domain.ErrInvalidInput
	}
	runID, err := identity.NewRunID()
	if err != nil {
		return domain.Run{}, err
	}
	jobID, err := identity.NewRunID()
	if err != nil {
		return domain.Run{}, err
	}
	return service.repository.CreateQueuedDiscoveryRun(ctx, CreateRunCommand{RunID: runID, JobID: jobID,
		WorkspaceID: request.WorkspaceID, SourceID: request.SourceID,
		RequestedBy: strings.TrimSpace(request.PrincipalRef), IdempotencyKey: strings.TrimSpace(request.IdempotencyKey),
		TraceID: request.TraceID, CreatedAt: service.clock.Now().UTC(), ExpectedAuthorizationVersion: authorizationVersion})
}

func (service *ControlService) ListRuns(ctx context.Context, request ListRunsRequest) (domain.RunPage, error) {
	authorizationVersion, err := service.authorizeVersion(ctx, request.WorkspaceID, authorization.ActionSourceRead,
		authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.RunPage{}, err
	}
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(request.PrincipalRef))
	if err != nil {
		return domain.RunPage{}, domain.ErrInvalidInput
	}
	if request.Limit == 0 {
		request.Limit = 50
	}
	if request.Limit < 1 || request.Limit > 100 {
		return domain.RunPage{}, domain.ErrInvalidInput
	}
	return service.repository.ListDiscoveryRuns(ctx, ListRunsQuery{WorkspaceID: request.WorkspaceID, SourceID: request.SourceID,
		PrincipalID: principal, AuthorizationVersion: authorizationVersion, Cursor: request.Cursor, Limit: request.Limit})
}

func (service *ControlService) ListCandidates(ctx context.Context, request ListCandidateRequest) (domain.CandidatePage, error) {
	resource := authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()}
	if request.SourceID != nil {
		resource = authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}
	}
	authVersion, err := service.authorizeVersion(ctx, request.WorkspaceID, authorization.ActionSourceRead,
		resource, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.CandidatePage{}, err
	}
	if request.Status != "" && request.Status != "pending" && request.Status != "dismissed" && request.Status != "converted" {
		return domain.CandidatePage{}, domain.ErrInvalidInput
	}
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(request.PrincipalRef))
	if err != nil {
		return domain.CandidatePage{}, domain.ErrInvalidInput
	}
	if request.Limit == 0 {
		request.Limit = 50
	}
	if request.Limit < 1 || request.Limit > 100 {
		return domain.CandidatePage{}, domain.ErrInvalidInput
	}
	return service.repository.ListDiscoveryCandidates(ctx, ListCandidatesQuery{WorkspaceID: request.WorkspaceID,
		SourceID: request.SourceID, Status: request.Status, PrincipalID: principal, AuthorizationVersion: authVersion, Cursor: request.Cursor, Limit: request.Limit})
}

func (service *ControlService) GetCandidate(ctx context.Context, workspace identity.WorkspaceID, candidate identity.SemanticCandidateID, principalRef, traceID string) (domain.Candidate, error) {
	source, err := service.repository.LookupDiscoveryCandidateSource(ctx, workspace, candidate)
	if err != nil {
		return domain.Candidate{}, err
	}
	authVersion, err := service.authorizeVersion(ctx, workspace, authorization.ActionSourceRead,
		authorization.Resource{Type: authorization.ScopeSource, ID: source.UUID()}, &principalRef, traceID)
	if err != nil {
		return domain.Candidate{}, err
	}
	return service.repository.GetDiscoveryCandidate(ctx, workspace, candidate, authVersion)
}

func (service *ControlService) DecideCandidate(ctx context.Context, request DecideCandidateRequest) (domain.CandidateDecision, error) {
	source, err := service.repository.LookupDiscoveryCandidateSource(ctx, request.WorkspaceID, request.CandidateID)
	if err != nil {
		return domain.CandidateDecision{}, err
	}
	authVersion, err := service.authorizeVersion(ctx, request.WorkspaceID, authorization.ActionAssetPropose,
		authorization.Resource{Type: authorization.ScopeSource, ID: source.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.CandidateDecision{}, err
	}
	request.Action, request.Reason, request.IdempotencyKey = strings.TrimSpace(request.Action), strings.TrimSpace(request.Reason), strings.TrimSpace(request.IdempotencyKey)
	if request.IdempotencyKey == "" || len(request.IdempotencyKey) > 256 || len(request.Reason) > 4096 ||
		(request.Action != "dismiss" && request.Action != "convert") || (request.Action == "convert") != (request.ProposalID != nil) {
		return domain.CandidateDecision{}, domain.ErrInvalidInput
	}
	decisionID, err := identity.NewRunID()
	if err != nil {
		return domain.CandidateDecision{}, err
	}
	fingerprint := discoveryFingerprint(struct{ Workspace, Candidate, Action, Proposal, Reason, Actor string }{
		request.WorkspaceID.String(), request.CandidateID.String(), request.Action, proposalIDString(request.ProposalID), request.Reason, strings.TrimSpace(request.PrincipalRef)})
	return service.repository.DecideDiscoveryCandidate(ctx, DecideCandidateCommand{ExpectedStatus: "pending", ExpectedAuthorizationVersion: authVersion, Decision: domain.CandidateDecision{
		ID: decisionID, WorkspaceID: request.WorkspaceID, CandidateID: request.CandidateID, Action: request.Action,
		ProposalID: request.ProposalID, Actor: strings.TrimSpace(request.PrincipalRef), Reason: request.Reason,
		IdempotencyKey: request.IdempotencyKey, RequestFingerprint: fingerprint, TraceID: request.TraceID, CreatedAt: service.clock.Now().UTC(),
	}})
}

func proposalIDString(value *identity.ProposalID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func discoveryFingerprint(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func (service *ControlService) JobHandler() jobs.Handler {
	return func(ctx context.Context, job jobs.Job) (resultErr error) {
		var payload struct {
			RunID string `json:"runId"`
		}
		if json.Unmarshal(job.Payload, &payload) != nil {
			return domain.ErrInvalidInput
		}
		runID, err := identity.ParseRunID(payload.RunID)
		if err != nil {
			return domain.ErrInvalidInput
		}
		ctx = WithRunCommitFence(ctx, RunCommitFence{JobID: job.ID, LeaseOwner: job.LeaseOwner})
		defer func() {
			if recover() == nil {
				return
			}
			_ = service.repository.RetryOrFailDiscoveryRun(ctx, job.WorkspaceID, runID, job.ID,
				job.Attempt >= job.MaxAttempts, "DISCOVERY_HANDLER_PANIC", service.clock.Now().UTC())
			resultErr = errors.New("discovery handler failed")
		}()
		execution, err := service.repository.LoadDiscoveryRunExecution(ctx, job.WorkspaceID, runID)
		if err != nil {
			_ = service.repository.RetryOrFailDiscoveryRun(ctx, job.WorkspaceID, runID, job.ID, permanentDiscoveryError(err) || job.Attempt >= job.MaxAttempts, discoveryErrorCode(err), service.clock.Now().UTC())
			return err
		}
		if execution.Run.JobID != job.ID {
			return domain.ErrConflict
		}
		if execution.Run.Status == "succeeded" || execution.Run.Status == "degraded" {
			return nil
		}
		if err := service.repository.BeginDiscoveryRun(ctx, job.WorkspaceID, runID, job.ID, service.clock.Now().UTC()); err != nil {
			return err
		}
		fail := func(cause error) error {
			code := discoveryErrorCode(cause)
			permanent := permanentDiscoveryError(cause)
			_ = service.repository.RetryOrFailDiscoveryRun(ctx, job.WorkspaceID, runID, job.ID, permanent || job.Attempt >= job.MaxAttempts, code, service.clock.Now().UTC())
			if permanent {
				return jobs.Permanent(cause, code)
			}
			return cause
		}
		var snapshot domain.Snapshot
		if execution.Source.Kind == "postgresql" {
			if service.cipher == nil || service.collector == nil {
				return fail(domain.ErrCredential)
			}
			secret, openErr := service.cipher.Open(execution.Credential)
			if openErr != nil {
				return fail(openErr)
			}
			snapshot, err = service.collector.Collect(ctx, execution.Source, secret, service.clock.Now().UTC())
			if err != nil {
				return fail(err)
			}
			files, loadErr := service.artifacts.LoadContext(ctx, execution.Source.ArtifactPaths)
			if loadErr != nil {
				return fail(loadErr)
			}
			if len(files) > 0 && service.sqlAdapter != nil {
				sqlSnapshot, parseErr := service.sqlAdapter.Discover(ctx, domain.Input{Locator: "artifacts:" + execution.Source.ID.String(), ObservedAt: snapshot.ObservedAt, Files: files})
				if parseErr != nil {
					return fail(parseErr)
				}
				snapshot = mergeSnapshots(snapshot, sqlSnapshot)
			}
		} else {
			snapshot, err = service.collectArtifactSnapshot(ctx, execution)
			if err != nil {
				return fail(err)
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, err = service.repository.PersistDiscoveryRunSnapshot(ctx, job.WorkspaceID, execution.Source.ID, runID, snapshot)
		if err != nil {
			return fail(err)
		}
		return nil
	}
}

func (service *ControlService) collectArtifactSnapshot(ctx context.Context, execution RunExecution) (domain.Snapshot, error) {
	if service.artifactStore == nil || len(execution.Artifacts) == 0 {
		return domain.Snapshot{}, domain.ErrInvalidInput
	}
	files := make(map[string][]byte, len(execution.Artifacts))
	var kind ingestiondomain.ArtifactKind
	var total int64
	for _, member := range execution.Artifacts {
		if ctx.Err() != nil {
			return domain.Snapshot{}, ctx.Err()
		}
		if kind == "" {
			kind = member.Kind
		}
		total += member.ByteSize
		if total > ingestiondomain.MaxArchiveBytes {
			return domain.Snapshot{}, ingestiondomain.ErrLimitExceeded
		}
		reader, err := service.artifactStore.Open(ctx, execution.Run.WorkspaceID, member.ContentDigest)
		if err != nil {
			return domain.Snapshot{}, ingestiondomain.ErrStore
		}
		content, readErr := io.ReadAll(io.LimitReader(reader, member.ByteSize+1))
		_ = reader.Close()
		if readErr != nil || int64(len(content)) != member.ByteSize {
			return domain.Snapshot{}, ingestiondomain.ErrStore
		}
		digest := sha256.Sum256(content)
		if "sha256:"+hex.EncodeToString(digest[:]) != member.ContentDigest {
			return domain.Snapshot{}, ingestiondomain.ErrStore
		}
		files[member.LogicalPath] = content
	}
	if execution.Source.Kind == "dbt_bundle" {
		kind = ingestiondomain.ArtifactDBTManifest
	} else if execution.Source.Kind == "sql_bundle" {
		kind = ingestiondomain.ArtifactSQL
	}
	adapter := service.artifactAdapters[kind]
	if adapter == nil {
		return domain.Snapshot{}, ingestiondomain.ErrUnsupported
	}
	return adapter.Discover(ctx, domain.Input{Locator: "artifact-set:" + execution.Run.ArtifactSetID.String(), ObservedAt: service.clock.Now().UTC(), Files: files})
}

func (service *ControlService) authorize(ctx context.Context, workspace identity.WorkspaceID, action authorization.Action,
	resource authorization.Resource, principalRef, traceID string) error {
	_, err := service.authorizeVersion(ctx, workspace, action, resource, &principalRef, traceID)
	return err
}

func (service *ControlService) authorizeVersion(ctx context.Context, workspace identity.WorkspaceID, action authorization.Action,
	resource authorization.Resource, principalRef *string, traceID string) (int64, error) {
	if service.authorizer == nil {
		return 0, nil
	}
	decision, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{PrincipalRef: strings.TrimSpace(*principalRef),
		WorkspaceID: workspace, Action: action, Resource: resource, TraceID: traceID})
	if err != nil {
		return 0, err
	}
	if decision.Allowed {
		// Persist the principal resolved by this decision, not a development alias.
		if !decision.PrincipalID.IsZero() {
			*principalRef = decision.PrincipalID.String()
		}
		return decision.AuthorizationVersion, nil
	}
	return 0, &authorization.DenialError{Decision: decision}
}

func validSourceInput(request CreateSourceRequest) bool {
	if request.Name == "" || len(request.Name) > 120 || request.Host == "" || len(request.Host) > 253 ||
		request.Database == "" || len(request.Database) > 63 || request.Username == "" || len(request.Username) > 63 ||
		request.Port < 1 || request.Port > 65535 || strings.TrimSpace(request.Password) == "" {
		return false
	}
	if net.ParseIP(strings.Trim(request.Host, "[]")) == nil {
		for _, label := range strings.Split(request.Host, ".") {
			if label == "" || len(label) > 63 {
				return false
			}
		}
	}
	return request.SSLMode == "disable" || request.SSLMode == "require" || request.SSLMode == "verify-ca" || request.SSLMode == "verify-full"
}

func mergeSnapshots(live, sql domain.Snapshot) domain.Snapshot {
	existing := make(map[string]struct{}, len(live.Datasets))
	for _, dataset := range live.Datasets {
		existing[dataset.ExternalKey] = struct{}{}
	}
	for _, dataset := range sql.Datasets {
		if _, ok := existing[dataset.ExternalKey]; !ok {
			live.Datasets = append(live.Datasets, dataset)
		}
	}
	live.CodeArtifacts = append(live.CodeArtifacts, sql.CodeArtifacts...)
	live.Coverage = append(live.Coverage, sql.Coverage...)
	live.Lineage = append(live.Lineage, sql.Lineage...)
	live.Findings = append(live.Findings, sql.Findings...)
	encoded, _ := json.Marshal(struct {
		D []domain.Dataset
		C []domain.CodeArtifact
		L []domain.LineageEdge
	}{live.Datasets, live.CodeArtifacts, live.Lineage})
	digest := sha256.Sum256(encoded)
	live.ContentDigest = "sha256:" + hex.EncodeToString(digest[:])
	live.AdapterVersion = live.AdapterVersion + "+sql-" + sql.AdapterVersion
	_ = live.Canonicalize()
	return live
}

func discoveryErrorCode(err error) string {
	switch {
	case errors.Is(err, ingestiondomain.ErrUnsafeContent):
		return "ARTIFACT_UNSAFE"
	case errors.Is(err, ingestiondomain.ErrLimitExceeded):
		return "ARTIFACT_LIMIT_EXCEEDED"
	case errors.Is(err, ingestiondomain.ErrUnsupported), errors.Is(err, domain.ErrUnsupported):
		return "ARTIFACT_UNSUPPORTED"
	case errors.Is(err, domain.ErrCredential):
		return "CREDENTIAL_UNAVAILABLE"
	case errors.Is(err, domain.ErrUnsafeSource):
		return "SOURCE_ROLE_UNSAFE"
	case errors.Is(err, domain.ErrArtifactPath):
		return "ARTIFACT_PATH_REJECTED"
	case errors.Is(err, domain.ErrConnectionTest):
		return "SOURCE_CONNECTION_FAILED"
	case errors.Is(err, domain.ErrInvalidInput):
		return "DISCOVERY_INPUT_INVALID"
	case errors.Is(err, domain.ErrInvalidSnapshot):
		return "DISCOVERY_SNAPSHOT_INVALID"
	default:
		return "DISCOVERY_FAILED"
	}
}

func permanentDiscoveryError(err error) bool {
	return errors.Is(err, ingestiondomain.ErrUnsafeContent) || errors.Is(err, ingestiondomain.ErrLimitExceeded) ||
		errors.Is(err, ingestiondomain.ErrUnsupported) || errors.Is(err, domain.ErrUnsupported) ||
		errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrInvalidSnapshot) ||
		errors.Is(err, domain.ErrArtifactPath) || errors.Is(err, domain.ErrUnsafeSource)
}
