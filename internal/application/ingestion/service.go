package ingestion

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	discoverydomain "github.com/iiwish/semlia/internal/domain/discovery"
	domain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
)

type Clock interface{ Now() time.Time }
type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

type SQLArtifactLoader interface {
	LoadContext(context.Context, []string) (map[string][]byte, error)
}

type Repository interface {
	FindArtifactByIdempotency(context.Context, identity.WorkspaceID, string, string, int64) (*domain.Artifact, error)
	CreateArtifact(context.Context, CreateArtifactCommand) (domain.Artifact, error)
	ReserveArtifactObject(context.Context, CreateArtifactCommand) error
	ReleaseArtifactObjectReservation(context.Context, CreateArtifactCommand, func(context.Context, domain.CleanupObject) error) error
	CleanupArtifactObjects(context.Context, time.Time, int, func(context.Context, domain.CleanupObject) error) (int, error)
	RecordRejectedArtifact(context.Context, CreateArtifactCommand, string) (domain.Artifact, error)
	GetArtifact(context.Context, identity.WorkspaceID, identity.ArtifactID) (domain.Artifact, error)
	ListArtifacts(context.Context, ListArtifactsQuery) (domain.Page[domain.Artifact], error)
	FinalizeArtifactSet(context.Context, FinalizeArtifactSetCommand) (domain.ArtifactSet, error)
	FindArtifactSetByIdempotency(context.Context, identity.WorkspaceID, string, string, int64) (*domain.ArtifactSet, error)
	GetArtifactSet(context.Context, identity.WorkspaceID, identity.ArtifactSetID, int64) (domain.ArtifactSet, error)
	LookupArtifactSetSource(context.Context, identity.WorkspaceID, identity.ArtifactSetID) (identity.SourceConnectionID, error)
}

type CreateArtifactCommand struct {
	Artifact                     domain.Artifact
	Object                       domain.Object
	StorageKey                   string
	ExpectedSourceVersion        *int64
	ExpectedAuthorizationVersion int64
	TraceID                      string
}

type ListArtifactsQuery struct {
	WorkspaceID          identity.WorkspaceID
	SourceID             *identity.SourceConnectionID
	Kind                 domain.ArtifactKind
	Status               domain.ArtifactStatus
	Cursor               string
	Limit                int
	PrincipalID          identity.PrincipalID
	AuthorizationVersion int64
}

type FinalizeArtifactSetCommand struct {
	Set                          domain.ArtifactSet
	SourceName                   string
	ExpectedSourceVersion        *int64
	IdempotencyKey               string
	RequestFingerprint           string
	TraceID                      string
	ExpectedAuthorizationVersion int64
	ContentRetentionUntil        time.Time
}

type StageRequest struct {
	WorkspaceID           identity.WorkspaceID
	SourceID              *identity.SourceConnectionID
	ExpectedSourceVersion *int64
	Kind                  domain.ArtifactKind
	OriginalName          string
	MediaType             string
	Content               io.Reader
	ContentLength         int64
	IdempotencyKey        string
	PrincipalRef          string
	TraceID               string
	TrustedLogicalPath    string
}

type FinalizeRequest struct {
	WorkspaceID               identity.WorkspaceID
	SourceName                string
	ArtifactIDs               []identity.ArtifactID
	IdempotencyKey            string
	PrincipalRef              string
	TraceID                   string
	SourceID                  *identity.SourceConnectionID
	ExpectedSourceVersion     *int64
	TrustedRequestFingerprint string
}

type RegisterSQLRequest struct {
	WorkspaceID           identity.WorkspaceID
	SourceName            string
	Paths                 []string
	IdempotencyKey        string
	PrincipalRef          string
	TraceID               string
	SourceID              *identity.SourceConnectionID
	ExpectedSourceVersion *int64
}

type Service struct {
	repository        Repository
	store             domain.ArtifactStore
	authorizer        authorizationapp.Evaluator
	clock             Clock
	adapters          map[domain.ArtifactKind]discoverydomain.Adapter
	sqlLoader         SQLArtifactLoader
	stageSlots        chan struct{}
	finalizeSlots     chan struct{}
	artifactRetention time.Duration
}

func (service *Service) WithSQLArtifactLoader(loader SQLArtifactLoader) *Service {
	service.sqlLoader = loader
	return service
}

func NewService(repository Repository, store domain.ArtifactStore, authorizer authorizationapp.Evaluator, clock Clock,
	adapters map[domain.ArtifactKind]discoverydomain.Adapter) *Service {
	return &Service{repository: repository, store: store, authorizer: authorizer, clock: clock, adapters: adapters,
		stageSlots: make(chan struct{}, 2), finalizeSlots: make(chan struct{}, 1), artifactRetention: 30 * 24 * time.Hour}
}

func (service *Service) WithArtifactRetention(retention time.Duration) *Service {
	if retention >= 24*time.Hour && retention <= 365*24*time.Hour {
		service.artifactRetention = retention
	}
	return service
}

func (service *Service) WithConcurrency(stage, finalize int) *Service {
	if stage > 0 && stage <= 32 {
		service.stageSlots = make(chan struct{}, stage)
	}
	if finalize > 0 && finalize <= 8 {
		service.finalizeSlots = make(chan struct{}, finalize)
	}
	return service
}

func (service *Service) Stage(ctx context.Context, request StageRequest) (domain.Artifact, error) {
	resource := authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()}
	if request.SourceID != nil {
		if request.ExpectedSourceVersion == nil || *request.ExpectedSourceVersion < 1 {
			return domain.Artifact{}, domain.ErrInvalid
		}
		resource = authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}
	} else if request.ExpectedSourceVersion != nil {
		return domain.Artifact{}, domain.ErrInvalid
	}
	authorizationVersion, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionSourceManage,
		resource, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.Artifact{}, err
	}
	return service.stageAuthorized(ctx, request, authorizationVersion)
}

func (service *Service) stageAuthorized(ctx context.Context, request StageRequest,
	authorizationVersion int64,
) (domain.Artifact, error) {
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(request.PrincipalRef))
	if err != nil || !request.Kind.Valid() || request.Content == nil || request.ContentLength < 1 || request.ContentLength > domain.MaxUploadBytes ||
		strings.TrimSpace(request.IdempotencyKey) == "" || len(request.IdempotencyKey) > 256 || !service.store.Configured() {
		return domain.Artifact{}, domain.ErrInvalid
	}
	if err := acquire(ctx, service.stageSlots); err != nil {
		return domain.Artifact{}, err
	}
	defer release(service.stageSlots)
	name, err := domain.SanitizeOriginalName(request.OriginalName)
	request.MediaType = canonicalMediaType(request.MediaType)
	logicalPath, pathErr := trustedLogicalPath(request.Kind, request.TrustedLogicalPath)
	if err != nil || pathErr != nil || !validMediaName(request.Kind, request.MediaType, name) {
		return domain.Artifact{}, domain.ErrUnsafeContent
	}
	content, err := io.ReadAll(io.LimitReader(request.Content, domain.MaxUploadBytes+1))
	if int64(len(content)) > domain.MaxUploadBytes {
		return domain.Artifact{}, domain.ErrLimitExceeded
	}
	if err != nil || int64(len(content)) != request.ContentLength {
		return domain.Artifact{}, domain.ErrInvalid
	}
	hash := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(hash[:])
	fingerprint := digestJSON(struct {
		Principal     string              `json:"principalId"`
		Kind          domain.ArtifactKind `json:"kind"`
		Name          string              `json:"name"`
		MediaType     string              `json:"mediaType"`
		LogicalPath   string              `json:"logicalPath,omitempty"`
		Digest        string              `json:"digest"`
		SourceID      string              `json:"sourceId,omitempty"`
		SourceVersion *int64              `json:"sourceVersion,omitempty"`
	}{principal.String(), request.Kind, name, request.MediaType, logicalPath, digest, sourceIDString(request.SourceID), request.ExpectedSourceVersion})
	artifactID, err := identity.NewArtifactID()
	if err != nil {
		return domain.Artifact{}, err
	}
	now := service.clock.Now().UTC()
	expiresAt := now.Add(service.artifactRetention)
	artifact := domain.Artifact{ID: artifactID, WorkspaceID: request.WorkspaceID, SourceConnectionID: request.SourceID, Kind: request.Kind,
		SchemaVersion: schemaVersion(request.Kind), ContentDigest: digest, ByteSize: int64(len(content)),
		MediaType: request.MediaType, OriginalName: name, LogicalPath: logicalPath, Status: domain.ArtifactUploaded,
		UploadedByPrincipalID: &principal, IdempotencyKey: strings.TrimSpace(request.IdempotencyKey),
		RequestFingerprint: fingerprint, ContentAvailability: domain.ContentAvailable, ExpiresAt: &expiresAt, CreatedAt: now}
	validationSummary, validationErr := service.validate(ctx, artifact, content)
	if validationErr != nil {
		artifact.ContentDigest = ""
		artifact.ContentAvailability = domain.ContentNotStored
		artifact.Status = domain.ArtifactRejected
		artifact.FailureCode = ingestionErrorCode(validationErr)
		artifact.ExpiresAt = nil
		artifact.FinalizedAt = &now
		stored, recordErr := service.repository.RecordRejectedArtifact(ctx, CreateArtifactCommand{Artifact: artifact,
			ExpectedSourceVersion: request.ExpectedSourceVersion, ExpectedAuthorizationVersion: authorizationVersion, TraceID: request.TraceID}, artifact.FailureCode)
		if recordErr != nil {
			return domain.Artifact{}, recordErr
		}
		return stored, validationErr
	}
	artifact.ValidationSummary = &validationSummary
	existing, err := service.repository.FindArtifactByIdempotency(ctx, request.WorkspaceID, artifact.IdempotencyKey, fingerprint, authorizationVersion)
	if err != nil {
		return domain.Artifact{}, err
	}
	if existing != nil {
		return *existing, nil
	}
	storageKey := storageKey(request.WorkspaceID, digest)
	command := CreateArtifactCommand{Artifact: artifact,
		Object: domain.Object{WorkspaceID: request.WorkspaceID, ContentDigest: digest, ByteSize: int64(len(content)),
			MediaType: request.MediaType, StorageKey: storageKey, CreatedAt: now}, StorageKey: storageKey,
		ExpectedSourceVersion: request.ExpectedSourceVersion, ExpectedAuthorizationVersion: authorizationVersion, TraceID: request.TraceID}
	if err := service.repository.ReserveArtifactObject(ctx, command); err != nil {
		return domain.Artifact{}, err
	}
	if err := service.store.Put(ctx, request.WorkspaceID, digest, bytes.NewReader(content), int64(len(content))); err != nil {
		_ = service.releaseReservation(ctx, command)
		return domain.Artifact{}, domain.ErrStore
	}
	created, err := service.repository.CreateArtifact(ctx, command)
	if err != nil {
		_ = service.releaseReservation(ctx, command)
		return domain.Artifact{}, err
	}
	return created, nil
}

func (service *Service) Finalize(ctx context.Context, request FinalizeRequest) (domain.ArtifactSet, error) {
	resource := authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()}
	if request.SourceID != nil {
		resource = authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}
	}
	authorizationVersion, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionSourceManage,
		resource, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.ArtifactSet{}, err
	}
	return service.finalizeAuthorized(ctx, request, authorizationVersion)
}

func (service *Service) finalizeAuthorized(ctx context.Context, request FinalizeRequest,
	authorizationVersion int64,
) (domain.ArtifactSet, error) {
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(request.PrincipalRef))
	request.SourceName, request.IdempotencyKey = strings.TrimSpace(request.SourceName), strings.TrimSpace(request.IdempotencyKey)
	if err != nil || request.SourceName == "" || len(request.SourceName) > 120 || len(request.ArtifactIDs) == 0 || len(request.ArtifactIDs) > domain.MaxArchiveEntries ||
		request.IdempotencyKey == "" || len(request.IdempotencyKey) > 256 {
		return domain.ArtifactSet{}, domain.ErrInvalid
	}
	if domain.ContainsUnsafeDisplayRune(request.SourceName) {
		return domain.ArtifactSet{}, domain.ErrInvalid
	}
	if err := acquire(ctx, service.finalizeSlots); err != nil {
		return domain.ArtifactSet{}, err
	}
	defer release(service.finalizeSlots)
	fingerprint := digestJSON(struct {
		Principal             string                `json:"principalId"`
		Name                  string                `json:"name"`
		SourceID              string                `json:"sourceId,omitempty"`
		ExpectedSourceVersion *int64                `json:"expectedSourceVersion,omitempty"`
		Artifacts             []identity.ArtifactID `json:"artifacts"`
	}{principal.String(), request.SourceName, sourceIDString(request.SourceID), request.ExpectedSourceVersion, request.ArtifactIDs})
	if request.TrustedRequestFingerprint != "" {
		fingerprint = request.TrustedRequestFingerprint
	}
	existingSet, err := service.repository.FindArtifactSetByIdempotency(ctx, request.WorkspaceID, request.IdempotencyKey, fingerprint, authorizationVersion)
	if err != nil {
		return domain.ArtifactSet{}, err
	}
	if existingSet != nil {
		return *existingSet, nil
	}
	members := make([]domain.ArtifactMember, 0, len(request.ArtifactIDs))
	kinds := make(map[domain.ArtifactKind]int)
	seen := make(map[identity.ArtifactID]struct{}, len(request.ArtifactIDs))
	var setBytes int64
	for index, id := range request.ArtifactIDs {
		if _, duplicate := seen[id]; duplicate {
			return domain.ArtifactSet{}, domain.ErrInvalid
		}
		seen[id] = struct{}{}
		artifact, err := service.repository.GetArtifact(ctx, request.WorkspaceID, id)
		if err != nil || artifact.Status != domain.ArtifactUploaded || artifact.ContentDigest == "" {
			return domain.ArtifactSet{}, domain.ErrConflict
		}
		if request.SourceID == nil && artifact.SourceConnectionID != nil {
			return domain.ArtifactSet{}, domain.ErrConflict
		}
		if request.SourceID != nil && (artifact.SourceConnectionID == nil || *artifact.SourceConnectionID != *request.SourceID) {
			return domain.ArtifactSet{}, domain.ErrConflict
		}
		logicalPath := artifact.LogicalPath
		if logicalPath == "" {
			logicalPath = logicalArtifactPath(artifact)
		}
		setBytes += artifact.ByteSize
		if setBytes > domain.MaxArchiveBytes {
			return domain.ArtifactSet{}, domain.ErrLimitExceeded
		}
		kinds[artifact.Kind]++
		members = append(members, domain.ArtifactMember{ArtifactID: id, LogicalPath: logicalPath, Ordinal: index + 1,
			ContentDigest: artifact.ContentDigest, ByteSize: artifact.ByteSize, MediaType: artifact.MediaType, Kind: artifact.Kind,
			ContentAvailability: domain.ContentAvailable})
	}
	sourceKind, adapterKind, err := classifySet(kinds, len(members))
	if err != nil {
		return domain.ArtifactSet{}, err
	}
	files := make(map[string][]byte, len(members))
	for _, member := range members {
		reader, err := service.store.Open(ctx, request.WorkspaceID, member.ContentDigest)
		if err != nil {
			return domain.ArtifactSet{}, domain.ErrStore
		}
		content, readErr := io.ReadAll(io.LimitReader(reader, member.ByteSize+1))
		_ = reader.Close()
		if readErr != nil || int64(len(content)) != member.ByteSize {
			return domain.ArtifactSet{}, domain.ErrStore
		}
		if _, exists := files[member.LogicalPath]; exists {
			return domain.ArtifactSet{}, domain.ErrInvalid
		}
		files[member.LogicalPath] = content
	}
	adapter := service.adapters[adapterKind]
	if adapter == nil {
		return domain.ArtifactSet{}, domain.ErrUnsupported
	}
	input := discoverydomain.Input{Locator: "artifact-set:validation", ObservedAt: service.clock.Now().UTC(), Files: files}
	snapshot, err := adapter.Discover(ctx, input)
	if err != nil || terminalFinding(snapshot) {
		if err == nil {
			err = domain.ErrUnsupported
		}
		return domain.ArtifactSet{}, normalizeAdapterError(err)
	}
	setID, err := identity.NewArtifactSetID()
	if err != nil {
		return domain.ArtifactSet{}, err
	}
	var sourceID identity.SourceConnectionID
	if request.SourceID != nil {
		sourceID = *request.SourceID
		if request.ExpectedSourceVersion == nil || *request.ExpectedSourceVersion < 1 {
			return domain.ArtifactSet{}, domain.ErrInvalid
		}
	} else {
		sourceID, err = identity.NewSourceConnectionID()
		if err != nil {
			return domain.ArtifactSet{}, err
		}
	}
	setDigest := digestMembers(members)
	set := domain.ArtifactSet{ID: setID, WorkspaceID: request.WorkspaceID, SourceConnectionID: sourceID,
		SourceKind: sourceKind, SetDigest: setDigest, CreatedByPrincipalID: principal, Members: members, CreatedAt: service.clock.Now().UTC()}
	return service.repository.FinalizeArtifactSet(ctx, FinalizeArtifactSetCommand{Set: set, SourceName: request.SourceName,
		IdempotencyKey: request.IdempotencyKey, RequestFingerprint: fingerprint, TraceID: request.TraceID,
		ExpectedAuthorizationVersion: authorizationVersion, ExpectedSourceVersion: request.ExpectedSourceVersion,
		ContentRetentionUntil: service.clock.Now().UTC().Add(service.artifactRetention)})
}

func (service *Service) RegisterSQL(ctx context.Context, request RegisterSQLRequest) (domain.ArtifactSet, error) {
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if service == nil || service.sqlLoader == nil || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 256 || len(request.Paths) == 0 || len(request.Paths) > 200 {
		return domain.ArtifactSet{}, domain.ErrInvalid
	}
	paths, err := normalizedSQLRegistrationPaths(request.Paths)
	if err != nil {
		return domain.ArtifactSet{}, domain.ErrInvalid
	}
	resource := authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()}
	if request.SourceID != nil {
		resource = authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}
	}
	authorizationVersion, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionSourceManage,
		resource, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.ArtifactSet{}, err
	}
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(request.PrincipalRef))
	if err != nil {
		return domain.ArtifactSet{}, domain.ErrInvalid
	}
	fingerprint := digestJSON(struct {
		Workspace             string   `json:"workspaceId"`
		Principal             string   `json:"principalId"`
		SourceName            string   `json:"sourceName"`
		SourceID              string   `json:"sourceId,omitempty"`
		ExpectedSourceVersion *int64   `json:"expectedSourceVersion,omitempty"`
		Paths                 []string `json:"paths"`
	}{request.WorkspaceID.String(), principal.String(), strings.TrimSpace(request.SourceName), sourceIDString(request.SourceID), request.ExpectedSourceVersion, paths})
	if existing, replayErr := service.repository.FindArtifactSetByIdempotency(ctx, request.WorkspaceID, request.IdempotencyKey, fingerprint, authorizationVersion); replayErr != nil || existing != nil {
		if existing != nil {
			return *existing, replayErr
		}
		return domain.ArtifactSet{}, replayErr
	}
	files, err := service.sqlLoader.LoadContext(ctx, paths)
	if err != nil {
		return domain.ArtifactSet{}, domain.ErrUnsafeContent
	}
	artifactIDs := make([]identity.ArtifactID, 0, len(paths))
	for _, logicalPath := range paths {
		content := files[logicalPath]
		stageKey := "sql-stage:" + strings.TrimPrefix(digestJSON(struct {
			Registration string `json:"registration"`
			Path         string `json:"path"`
		}{request.IdempotencyKey, logicalPath}), "sha256:")
		artifact, stageErr := service.stageAuthorized(ctx, StageRequest{
			WorkspaceID: request.WorkspaceID, SourceID: request.SourceID, ExpectedSourceVersion: request.ExpectedSourceVersion,
			Kind: domain.ArtifactSQL, OriginalName: path.Base(logicalPath),
			MediaType: "application/sql", Content: bytes.NewReader(content), ContentLength: int64(len(content)),
			IdempotencyKey: stageKey, PrincipalRef: request.PrincipalRef, TraceID: request.TraceID,
			TrustedLogicalPath: logicalPath,
		}, authorizationVersion)
		if stageErr != nil {
			return domain.ArtifactSet{}, stageErr
		}
		artifactIDs = append(artifactIDs, artifact.ID)
	}
	return service.finalizeAuthorized(ctx, FinalizeRequest{
		WorkspaceID: request.WorkspaceID, SourceName: request.SourceName, ArtifactIDs: artifactIDs,
		IdempotencyKey: request.IdempotencyKey, PrincipalRef: request.PrincipalRef, TraceID: request.TraceID,
		SourceID: request.SourceID, ExpectedSourceVersion: request.ExpectedSourceVersion,
		TrustedRequestFingerprint: fingerprint,
	}, authorizationVersion)
}

func normalizedSQLRegistrationPaths(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		cleaned := path.Clean(strings.TrimSpace(value))
		if cleaned == "." || path.IsAbs(cleaned) || cleaned != strings.TrimSpace(value) || strings.HasPrefix(cleaned, "../") ||
			strings.Contains(cleaned, "\\") || domain.ContainsUnsafeDisplayRune(cleaned) || !strings.EqualFold(path.Ext(cleaned), ".sql") || len(cleaned) > 1024 {
			return nil, domain.ErrInvalid
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	sort.Strings(result)
	if len(result) == 0 || len(result) > 200 {
		return nil, domain.ErrInvalid
	}
	return result, nil
}

func (service *Service) releaseReservation(ctx context.Context, command CreateArtifactCommand) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return service.repository.ReleaseArtifactObjectReservation(cleanupCtx, command, func(deleteCtx context.Context, object domain.CleanupObject) error {
		return service.store.Delete(deleteCtx, object.WorkspaceID, object.ContentDigest)
	})
}

func (service *Service) RecoverStorage(ctx context.Context, limit int) (int, error) {
	if service == nil || service.repository == nil || service.store == nil || service.clock == nil || limit < 1 || limit > 100 {
		return 0, domain.ErrInvalid
	}
	now := service.clock.Now().UTC()
	temporaryRemoved := 0
	if sweeper, ok := service.store.(domain.TemporaryArtifactSweeper); ok {
		var err error
		temporaryRemoved, err = sweeper.CleanupTemporary(ctx, now.Add(-time.Hour), limit)
		if err != nil || temporaryRemoved == limit {
			return temporaryRemoved, err
		}
	}
	removed, err := service.repository.CleanupArtifactObjects(ctx, now, limit-temporaryRemoved,
		func(deleteCtx context.Context, object domain.CleanupObject) error {
			boundedCtx, cancel := context.WithTimeout(deleteCtx, 15*time.Second)
			defer cancel()
			return service.store.Delete(boundedCtx, object.WorkspaceID, object.ContentDigest)
		})
	return temporaryRemoved + removed, err
}

func (service *Service) List(ctx context.Context, query ListArtifactsQuery, principalRef, traceID string) (domain.Page[domain.Artifact], error) {
	resource := authorization.Resource{Type: authorization.ScopeWorkspace, ID: query.WorkspaceID.UUID()}
	if query.SourceID != nil {
		resource = authorization.Resource{Type: authorization.ScopeSource, ID: query.SourceID.UUID()}
	}
	authorizationVersion, err := service.authorize(ctx, query.WorkspaceID, authorization.ActionSourceRead,
		resource, &principalRef, traceID)
	if err != nil {
		return domain.Page[domain.Artifact]{}, err
	}
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.Limit < 1 || query.Limit > 100 {
		return domain.Page[domain.Artifact]{}, domain.ErrInvalid
	}
	principal, err := identity.ParsePrincipalID(strings.TrimSpace(principalRef))
	if err != nil {
		return domain.Page[domain.Artifact]{}, domain.ErrInvalid
	}
	query.PrincipalID, query.AuthorizationVersion = principal, authorizationVersion
	return service.repository.ListArtifacts(ctx, query)
}

func (service *Service) GetSet(ctx context.Context, workspace identity.WorkspaceID, set identity.ArtifactSetID,
	principalRef, traceID string,
) (domain.ArtifactSet, error) {
	if service == nil || service.repository == nil {
		return domain.ArtifactSet{}, domain.ErrInvalid
	}
	source, err := service.repository.LookupArtifactSetSource(ctx, workspace, set)
	if err != nil {
		return domain.ArtifactSet{}, err
	}
	authorizationVersion, err := service.authorize(ctx, workspace, authorization.ActionSourceRead,
		authorization.Resource{Type: authorization.ScopeSource, ID: source.UUID()}, &principalRef, traceID)
	if err != nil {
		return domain.ArtifactSet{}, err
	}
	return service.repository.GetArtifactSet(ctx, workspace, set, authorizationVersion)
}

func (service *Service) validate(ctx context.Context, artifact domain.Artifact, content []byte) (domain.ArtifactValidationSummary, error) {
	adapter := service.adapters[artifact.Kind]
	if adapter == nil {
		return domain.ArtifactValidationSummary{}, domain.ErrUnsupported
	}
	logicalName := logicalArtifactPath(artifact)
	snapshot, err := adapter.Discover(ctx, discoverydomain.Input{Locator: "artifact:validation", ObservedAt: service.clock.Now().UTC(), Files: map[string][]byte{logicalName: content}})
	if err != nil {
		return domain.ArtifactValidationSummary{}, normalizeAdapterError(err)
	}
	if terminalFinding(snapshot) {
		return domain.ArtifactValidationSummary{}, domain.ErrUnsupported
	}
	fieldCount := 0
	for _, dataset := range snapshot.Datasets {
		fieldCount += len(dataset.Fields)
	}
	return domain.ArtifactValidationSummary{AdapterKind: snapshot.AdapterKind, AdapterVersion: snapshot.AdapterVersion,
		DatasetCount: len(snapshot.Datasets), FieldCount: fieldCount, CodeArtifactCount: len(snapshot.CodeArtifacts),
		LineageCount: len(snapshot.Lineage), KeyCount: len(snapshot.Keys), JoinCount: len(snapshot.Joins), FindingCount: len(snapshot.Findings)}, nil
}

func normalizeAdapterError(err error) error {
	switch {
	case err == nil, errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, domain.ErrLimitExceeded):
		return domain.ErrLimitExceeded
	case errors.Is(err, domain.ErrUnsupported), errors.Is(err, discoverydomain.ErrUnsupported):
		return domain.ErrUnsupported
	case errors.Is(err, domain.ErrUnsafeContent), errors.Is(err, discoverydomain.ErrInvalidInput),
		errors.Is(err, discoverydomain.ErrInvalidSnapshot):
		return domain.ErrUnsafeContent
	default:
		return err
	}
}

func (service *Service) authorize(ctx context.Context, workspace identity.WorkspaceID, action authorization.Action,
	resource authorization.Resource, principalRef *string, traceID string) (int64, error) {
	if service == nil || service.repository == nil || service.store == nil || service.clock == nil {
		return 0, domain.ErrInvalid
	}
	if service.authorizer == nil {
		return 0, nil
	}
	decision, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{PrincipalRef: strings.TrimSpace(*principalRef),
		WorkspaceID: workspace, Action: action, Resource: resource, TraceID: traceID})
	if err != nil {
		return 0, err
	}
	if !decision.Allowed {
		return 0, &authorization.DenialError{Decision: decision}
	}
	// Persist the principal resolved by this decision, not a development alias.
	if !decision.PrincipalID.IsZero() {
		*principalRef = decision.PrincipalID.String()
	}
	return decision.AuthorizationVersion, nil
}

func validMediaName(kind domain.ArtifactKind, mediaType, name string) bool {
	mediaType = canonicalMediaType(mediaType)
	extension := strings.ToLower(path.Ext(name))
	switch kind {
	case domain.ArtifactCSV:
		return extension == ".csv" && mediaType == "text/csv"
	case domain.ArtifactXLSX:
		return extension == ".xlsx" && mediaType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case domain.ArtifactMarkdown:
		return (extension == ".md" || extension == ".markdown") && mediaType == "text/markdown"
	case domain.ArtifactSQL:
		return extension == ".sql" && (mediaType == "application/sql" || mediaType == "text/plain")
	case domain.ArtifactDBTManifest:
		return name == "manifest.json" && mediaType == "application/json"
	case domain.ArtifactDBTCatalog:
		return name == "catalog.json" && mediaType == "application/json"
	default:
		return false
	}
}

func canonicalMediaType(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
}

func logicalArtifactPath(artifact domain.Artifact) string {
	switch artifact.Kind {
	case domain.ArtifactDBTManifest:
		return "manifest.json"
	case domain.ArtifactDBTCatalog:
		return "catalog.json"
	default:
		return artifact.OriginalName
	}
}

func trustedLogicalPath(kind domain.ArtifactKind, value string) (string, error) {
	if value == "" {
		if kind == domain.ArtifactSQL {
			return "", domain.ErrInvalid
		}
		return "", nil
	}
	if kind != domain.ArtifactSQL || path.IsAbs(value) || strings.Contains(value, "\\") || strings.ContainsRune(value, '\x00') {
		return "", domain.ErrInvalid
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned != value || strings.HasPrefix(cleaned, "../") || len(cleaned) > 1024 {
		return "", domain.ErrInvalid
	}
	return cleaned, nil
}

func sourceIDString(value *identity.SourceConnectionID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func classifySet(kinds map[domain.ArtifactKind]int, count int) (string, domain.ArtifactKind, error) {
	if count == 1 && kinds[domain.ArtifactCSV]+kinds[domain.ArtifactXLSX]+kinds[domain.ArtifactMarkdown] == 1 {
		for _, kind := range []domain.ArtifactKind{domain.ArtifactCSV, domain.ArtifactXLSX, domain.ArtifactMarkdown} {
			if kinds[kind] == 1 {
				return "file", kind, nil
			}
		}
	}
	if count >= 1 && kinds[domain.ArtifactSQL] == count && count <= 200 {
		return "sql_bundle", domain.ArtifactSQL, nil
	}
	if kinds[domain.ArtifactDBTManifest] == 1 && kinds[domain.ArtifactDBTCatalog] <= 1 && count == kinds[domain.ArtifactDBTManifest]+kinds[domain.ArtifactDBTCatalog] {
		return "dbt_bundle", domain.ArtifactDBTManifest, nil
	}
	return "", "", domain.ErrInvalid
}

func terminalFinding(snapshot discoverydomain.Snapshot) bool {
	for _, finding := range snapshot.Findings {
		if finding.Terminal {
			return true
		}
	}
	return false
}

func schemaVersion(kind domain.ArtifactKind) string {
	switch kind {
	case domain.ArtifactDBTManifest:
		return "dbt-manifest-v12"
	case domain.ArtifactDBTCatalog:
		return "dbt-catalog-v1"
	default:
		return "semlia-artifact-v1"
	}
}

func digestMembers(members []domain.ArtifactMember) string {
	hash := sha256.New()
	for _, member := range members {
		_, _ = fmt.Fprintf(hash, "%d\x00%s\x00%s\x00%s\x00", member.Ordinal, member.LogicalPath, member.Kind, member.ContentDigest)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func digestJSON(value any) string {
	encoded, _ := json.Marshal(value)
	hash := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func storageKey(workspace identity.WorkspaceID, digest string) string {
	return "workspaces/" + workspace.UUID() + "/sha256/" + strings.TrimPrefix(digest, "sha256:")
}

func ingestionErrorCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrLimitExceeded):
		return "ARTIFACT_LIMIT_EXCEEDED"
	case errors.Is(err, domain.ErrUnsupported):
		return "ARTIFACT_UNSUPPORTED"
	default:
		return "ARTIFACT_UNSAFE"
	}
}

func acquire(ctx context.Context, slots chan struct{}) error {
	select {
	case slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func release(slots chan struct{}) { <-slots }
