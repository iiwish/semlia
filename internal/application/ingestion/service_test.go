package ingestion

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	discoverydomain "github.com/iiwish/semlia/internal/domain/discovery"
	domain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
)

type serviceRepository struct {
	artifacts           map[identity.ArtifactID]domain.Artifact
	artifactReceipt     *domain.Artifact
	artifactFingerprint string
	set                 domain.ArtifactSet
	setFingerprint      string
	setSource           identity.SourceConnectionID
	getSetAuthVersion   int64
	cleanupObject       *domain.CleanupObject
}

func (repo *serviceRepository) FindArtifactByIdempotency(_ context.Context, _ identity.WorkspaceID, _ string, fingerprint string, _ int64) (*domain.Artifact, error) {
	if repo.artifactReceipt == nil {
		return nil, nil
	}
	if repo.artifactFingerprint != fingerprint {
		return nil, domain.ErrConflict
	}
	value := *repo.artifactReceipt
	return &value, nil
}
func (repo *serviceRepository) CreateArtifact(_ context.Context, command CreateArtifactCommand) (domain.Artifact, error) {
	repo.artifacts[command.Artifact.ID] = command.Artifact
	value := command.Artifact
	repo.artifactReceipt = &value
	repo.artifactFingerprint = command.Artifact.RequestFingerprint
	return command.Artifact, nil
}
func (*serviceRepository) ReserveArtifactObject(context.Context, CreateArtifactCommand) error {
	return nil
}
func (*serviceRepository) ReleaseArtifactObjectReservation(context.Context, CreateArtifactCommand, func(context.Context, domain.CleanupObject) error) error {
	return nil
}
func (repo *serviceRepository) CleanupArtifactObjects(ctx context.Context, _ time.Time, _ int, remove func(context.Context, domain.CleanupObject) error) (int, error) {
	if repo.cleanupObject == nil {
		return 0, nil
	}
	if err := remove(ctx, *repo.cleanupObject); err != nil {
		return 0, err
	}
	return 1, nil
}
func (repo *serviceRepository) RecordRejectedArtifact(_ context.Context, command CreateArtifactCommand, _ string) (domain.Artifact, error) {
	if repo.artifactReceipt != nil {
		if repo.artifactFingerprint != command.Artifact.RequestFingerprint {
			return domain.Artifact{}, domain.ErrConflict
		}
		return *repo.artifactReceipt, nil
	}
	value := command.Artifact
	repo.artifactReceipt = &value
	repo.artifactFingerprint = command.Artifact.RequestFingerprint
	return command.Artifact, nil
}
func (repo *serviceRepository) GetArtifact(_ context.Context, _ identity.WorkspaceID, id identity.ArtifactID) (domain.Artifact, error) {
	value, ok := repo.artifacts[id]
	if !ok {
		return domain.Artifact{}, domain.ErrNotFound
	}
	return value, nil
}
func (*serviceRepository) ListArtifacts(context.Context, ListArtifactsQuery) (domain.Page[domain.Artifact], error) {
	return domain.Page[domain.Artifact]{}, nil
}
func (repo *serviceRepository) FinalizeArtifactSet(_ context.Context, command FinalizeArtifactSetCommand) (domain.ArtifactSet, error) {
	repo.set = command.Set
	repo.setFingerprint = command.RequestFingerprint
	return command.Set, nil
}
func (repo *serviceRepository) FindArtifactSetByIdempotency(_ context.Context, _ identity.WorkspaceID, _ string, fingerprint string, _ int64) (*domain.ArtifactSet, error) {
	if repo.setFingerprint == "" {
		return nil, nil
	}
	if repo.setFingerprint != fingerprint {
		return nil, domain.ErrConflict
	}
	value := repo.set
	return &value, nil
}
func (repo *serviceRepository) GetArtifactSet(_ context.Context, _ identity.WorkspaceID, _ identity.ArtifactSetID, authVersion int64) (domain.ArtifactSet, error) {
	repo.getSetAuthVersion = authVersion
	return repo.set, nil
}
func (repo *serviceRepository) LookupArtifactSetSource(context.Context, identity.WorkspaceID, identity.ArtifactSetID) (identity.SourceConnectionID, error) {
	return repo.setSource, nil
}

type memoryArtifactStore struct{ values map[string][]byte }

func (store *memoryArtifactStore) Put(_ context.Context, _ identity.WorkspaceID, digest string, input io.Reader, _ int64) error {
	value, err := io.ReadAll(input)
	store.values[digest] = value
	return err
}
func (store *memoryArtifactStore) Open(_ context.Context, _ identity.WorkspaceID, digest string) (io.ReadCloser, error) {
	value, ok := store.values[digest]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(value)), nil
}
func (*memoryArtifactStore) Delete(context.Context, identity.WorkspaceID, string) error { return nil }
func (*memoryArtifactStore) Check(context.Context) error                                { return nil }
func (*memoryArtifactStore) Configured() bool                                           { return true }

type serviceEvaluator struct {
	requests []authorizationapp.EvaluationRequest
	version  int64
}

func (value *serviceEvaluator) Evaluate(_ context.Context, request authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	value.requests = append(value.requests, request)
	return authorization.Decision{Allowed: true, Action: request.Action, ReasonCode: authorization.ReasonRoleGrant,
		AuthorizationVersion: value.version}, nil
}

type sqlLoader map[string][]byte

func (loader sqlLoader) LoadContext(_ context.Context, paths []string) (map[string][]byte, error) {
	result := make(map[string][]byte, len(paths))
	for _, path := range paths {
		result[path] = append([]byte(nil), loader[path]...)
	}
	return result, nil
}

type sqlAdapter struct{}

func (sqlAdapter) Kind() string    { return "postgresql_sql" }
func (sqlAdapter) Version() string { return "test/v1" }
func (sqlAdapter) Discover(_ context.Context, input discoverydomain.Input) (discoverydomain.Snapshot, error) {
	return discoverydomain.Snapshot{AdapterKind: "postgresql_sql", AdapterVersion: "test/v1", Locator: input.Locator,
		ContentDigest: input.ContentDigest(), ObservedAt: input.ObservedAt}, nil
}

type failingAdapter struct{ err error }

func (adapter failingAdapter) Kind() string    { return "dbt" }
func (adapter failingAdapter) Version() string { return "test/v1" }
func (adapter failingAdapter) Discover(context.Context, discoverydomain.Input) (discoverydomain.Snapshot, error) {
	return discoverydomain.Snapshot{}, adapter.err
}

func TestStageNormalizesMalformedAdapterInputToStableRejection(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	repo := &serviceRepository{artifacts: map[identity.ArtifactID]domain.Artifact{}}
	service := NewService(repo, &memoryArtifactStore{values: map[string][]byte{}}, &serviceEvaluator{version: 3}, ClockFunc(time.Now),
		map[domain.ArtifactKind]discoverydomain.Adapter{domain.ArtifactDBTManifest: failingAdapter{err: discoverydomain.ErrInvalidInput}})
	artifact, err := service.Stage(context.Background(), StageRequest{WorkspaceID: workspace, Kind: domain.ArtifactDBTManifest,
		OriginalName: "manifest.json", MediaType: "application/json", Content: bytes.NewReader([]byte("{")), ContentLength: 1,
		IdempotencyKey: "malformed", PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if !errors.Is(err, domain.ErrUnsafeContent) || artifact.Status != domain.ArtifactRejected ||
		artifact.FailureCode != "ARTIFACT_UNSAFE" || artifact.ContentAvailability != domain.ContentNotStored {
		t.Fatalf("artifact=%+v err=%v", artifact, err)
	}
}

func TestStageForExistingSourceUsesSourceScopeAndBindsArtifact(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	source, _ := identity.NewSourceConnectionID()
	repo := &serviceRepository{artifacts: map[identity.ArtifactID]domain.Artifact{}}
	evaluator := &serviceEvaluator{version: 11}
	service := NewService(repo, &memoryArtifactStore{values: map[string][]byte{}}, evaluator, ClockFunc(time.Now),
		map[domain.ArtifactKind]discoverydomain.Adapter{domain.ArtifactCSV: sqlAdapter{}})
	version := int64(4)
	content := []byte("id,name\n1,Alice\n")

	artifact, err := service.Stage(context.Background(), StageRequest{WorkspaceID: workspace, SourceID: &source,
		ExpectedSourceVersion: &version, Kind: domain.ArtifactCSV, OriginalName: "people.csv", MediaType: "text/csv",
		Content: bytes.NewReader(content), ContentLength: int64(len(content)), IdempotencyKey: "source-stage",
		PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SourceConnectionID == nil || *artifact.SourceConnectionID != source {
		t.Fatalf("staged artifact source=%v", artifact.SourceConnectionID)
	}
	if artifact.ExpiresAt == nil || artifact.ExpiresAt.Sub(artifact.CreatedAt) != 30*24*time.Hour {
		t.Fatalf("staged retention created=%s expires=%v", artifact.CreatedAt, artifact.ExpiresAt)
	}
	if len(evaluator.requests) != 1 || evaluator.requests[0].Resource.Type != authorization.ScopeSource || evaluator.requests[0].Resource.ID != source.UUID() {
		t.Fatalf("authorization requests=%+v", evaluator.requests)
	}
}

func TestStageIdempotencyIsBoundToActingPrincipal(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principalA, _ := identity.NewPrincipalID()
	principalB, _ := identity.NewPrincipalID()
	repo := &serviceRepository{artifacts: map[identity.ArtifactID]domain.Artifact{}}
	service := NewService(repo, &memoryArtifactStore{values: map[string][]byte{}}, &serviceEvaluator{version: 9}, ClockFunc(time.Now),
		map[domain.ArtifactKind]discoverydomain.Adapter{domain.ArtifactCSV: sqlAdapter{}})
	content := []byte("id,name\n1,Alice\n")
	request := StageRequest{WorkspaceID: workspace, Kind: domain.ArtifactCSV, OriginalName: "people.csv", MediaType: "text/csv",
		ContentLength: int64(len(content)), IdempotencyKey: "shared-stage-key", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}
	request.PrincipalRef = principalA.String()
	request.Content = bytes.NewReader(content)
	first, err := service.Stage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PrincipalRef = principalB.String()
	request.Content = bytes.NewReader(content)
	if _, err := service.Stage(context.Background(), request); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("cross-principal stage replay err=%v, want conflict", err)
	}
	if repo.artifactReceipt == nil || repo.artifactReceipt.ID != first.ID || repo.artifactReceipt.UploadedByPrincipalID == nil || *repo.artifactReceipt.UploadedByPrincipalID != principalA {
		t.Fatalf("stored stage receipt changed: %+v", repo.artifactReceipt)
	}
}

func TestRejectedStageIdempotencyIsBoundToActingPrincipal(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principalA, _ := identity.NewPrincipalID()
	principalB, _ := identity.NewPrincipalID()
	repo := &serviceRepository{artifacts: map[identity.ArtifactID]domain.Artifact{}}
	service := NewService(repo, &memoryArtifactStore{values: map[string][]byte{}}, &serviceEvaluator{version: 9}, ClockFunc(time.Now),
		map[domain.ArtifactKind]discoverydomain.Adapter{domain.ArtifactDBTManifest: failingAdapter{err: discoverydomain.ErrInvalidInput}})
	request := StageRequest{WorkspaceID: workspace, Kind: domain.ArtifactDBTManifest, OriginalName: "manifest.json", MediaType: "application/json",
		ContentLength: 1, IdempotencyKey: "shared-rejected-key", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}
	request.PrincipalRef = principalA.String()
	request.Content = bytes.NewReader([]byte("{"))
	if _, err := service.Stage(context.Background(), request); !errors.Is(err, domain.ErrUnsafeContent) {
		t.Fatalf("initial rejection err=%v", err)
	}
	request.PrincipalRef = principalB.String()
	request.Content = bytes.NewReader([]byte("{"))
	if _, err := service.Stage(context.Background(), request); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("cross-principal rejected replay err=%v, want conflict", err)
	}
}

func TestFinalizeAndSQLRegistrationIdempotencyAreBoundToActingPrincipal(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principalA, _ := identity.NewPrincipalID()
	principalB, _ := identity.NewPrincipalID()
	repo := &serviceRepository{artifacts: map[identity.ArtifactID]domain.Artifact{}}
	store := &memoryArtifactStore{values: map[string][]byte{}}
	service := NewService(repo, store, &serviceEvaluator{version: 9}, ClockFunc(time.Now),
		map[domain.ArtifactKind]discoverydomain.Adapter{domain.ArtifactCSV: sqlAdapter{}, domain.ArtifactSQL: sqlAdapter{}}).
		WithSQLArtifactLoader(sqlLoader{"models/orders.sql": []byte("select 1")})
	content := []byte("id,name\n1,Alice\n")
	artifact, err := service.Stage(context.Background(), StageRequest{WorkspaceID: workspace, Kind: domain.ArtifactCSV,
		OriginalName: "people.csv", MediaType: "text/csv", Content: bytes.NewReader(content), ContentLength: int64(len(content)),
		IdempotencyKey: "stage-for-finalize", PrincipalRef: principalA.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err != nil {
		t.Fatal(err)
	}
	finalize := FinalizeRequest{WorkspaceID: workspace, SourceName: "People", ArtifactIDs: []identity.ArtifactID{artifact.ID},
		IdempotencyKey: "shared-finalize-key", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", PrincipalRef: principalA.String()}
	if _, err := service.Finalize(context.Background(), finalize); err != nil {
		t.Fatal(err)
	}
	finalize.PrincipalRef = principalB.String()
	if _, err := service.Finalize(context.Background(), finalize); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("cross-principal finalize replay err=%v, want conflict", err)
	}

	registerRepo := &serviceRepository{artifacts: map[identity.ArtifactID]domain.Artifact{}}
	registerService := NewService(registerRepo, &memoryArtifactStore{values: map[string][]byte{}}, &serviceEvaluator{version: 9}, ClockFunc(time.Now),
		map[domain.ArtifactKind]discoverydomain.Adapter{domain.ArtifactSQL: sqlAdapter{}}).
		WithSQLArtifactLoader(sqlLoader{"models/orders.sql": []byte("select 1")})
	register := RegisterSQLRequest{WorkspaceID: workspace, SourceName: "SQL", Paths: []string{"models/orders.sql"},
		IdempotencyKey: "shared-register-key", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", PrincipalRef: principalA.String()}
	if _, err := registerService.RegisterSQL(context.Background(), register); err != nil {
		t.Fatal(err)
	}
	register.PrincipalRef = principalB.String()
	if _, err := registerService.RegisterSQL(context.Background(), register); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("cross-principal SQL replay err=%v, want conflict", err)
	}
}

func TestRegisterSQLReusesOneSourceAuthorizationForInternalStageAndFinalize(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	source, _ := identity.NewSourceConnectionID()
	repo := &serviceRepository{artifacts: map[identity.ArtifactID]domain.Artifact{}, setSource: source}
	evaluator := &serviceEvaluator{version: 17}
	service := NewService(repo, &memoryArtifactStore{values: map[string][]byte{}}, evaluator,
		ClockFunc(func() time.Time { return time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC) }),
		map[domain.ArtifactKind]discoverydomain.Adapter{domain.ArtifactSQL: sqlAdapter{}}).
		WithSQLArtifactLoader(sqlLoader{"models/orders.sql": []byte("select 1")})
	expectedVersion := int64(3)

	set, err := service.RegisterSQL(context.Background(), RegisterSQLRequest{WorkspaceID: workspace, SourceName: "SQL models",
		Paths: []string{"models/orders.sql"}, IdempotencyKey: "register-1", PrincipalRef: principal.String(),
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SourceID: &source, ExpectedSourceVersion: &expectedVersion})
	if err != nil {
		t.Fatal(err)
	}
	if set.SourceConnectionID != source || len(set.Members) != 1 || set.Members[0].LogicalPath != "models/orders.sql" {
		t.Fatalf("set = %+v", set)
	}
	if len(evaluator.requests) != 1 || evaluator.requests[0].Resource.Type != authorization.ScopeSource || evaluator.requests[0].Resource.ID != source.UUID() {
		t.Fatalf("authorization requests = %+v", evaluator.requests)
	}
}

func TestGetSetCarriesAuthorizationVersionIntoFencedRead(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	source, _ := identity.NewSourceConnectionID()
	setID, _ := identity.NewArtifactSetID()
	repo := &serviceRepository{setSource: source, set: domain.ArtifactSet{ID: setID, WorkspaceID: workspace, SourceConnectionID: source}}
	evaluator := &serviceEvaluator{version: 23}
	service := NewService(repo, &memoryArtifactStore{values: map[string][]byte{}}, evaluator, ClockFunc(time.Now), nil)

	if _, err := service.GetSet(context.Background(), workspace, setID, principal.String(), "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	if repo.getSetAuthVersion != 23 {
		t.Fatalf("artifact set read authorization version = %d, want 23", repo.getSetAuthVersion)
	}
}

type blockingDeleteStore struct{ memoryArtifactStore }

func (*blockingDeleteStore) Delete(ctx context.Context, _ identity.WorkspaceID, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestRecoverStorageCancelsBlockedDeleteAndReturns(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	repo := &serviceRepository{cleanupObject: &domain.CleanupObject{
		WorkspaceID: workspace, ContentDigest: "sha256:" + strings.Repeat("a", 64),
	}}
	service := NewService(repo, &blockingDeleteStore{}, nil, ClockFunc(time.Now), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := service.RecoverStorage(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("recover blocked delete error=%v", err)
	}
}
