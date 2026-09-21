package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	usageapp "github.com/iiwish/semlia/internal/application/usage"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/catalog"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

const (
	defaultLimit                = 50
	maxLimit                    = 200
	maxDepth                    = 3
	defaultAuthorityRecordLimit = 100
	maxAuthorityRecordLimit     = 200
)

type Repository interface {
	ListCatalogWorkspaces(context.Context) ([]domain.Workspace, error)
	CreateCatalogWorkspace(context.Context, domain.CreateWorkspaceCommand) (domain.Workspace, error)
	ListCatalogAssets(context.Context, domain.ListAssetsQuery) ([]domain.AssetSummary, error)
	CountCatalogAssets(context.Context, domain.ListAssetsQuery) (int64, error)
	GetCatalogAsset(context.Context, identity.WorkspaceID, identity.AssetID) (domain.AssetDetail, error)
	ListCatalogAuthorityRecords(context.Context, domain.ListAuthorityRecordsQuery) (domain.AuthorityRecordResult, error)
	CreateCatalogAsset(context.Context, domain.CreateAssetCommand) (domain.AssetDetail, error)
	ListCatalogRevisions(context.Context, domain.ListRevisionsQuery) ([]domain.Revision, error)
	CountCatalogRevisions(context.Context, identity.WorkspaceID, identity.AssetID) (int64, error)
	GetCatalogRevision(context.Context, identity.WorkspaceID, identity.AssetID, identity.RevisionID) (domain.Revision, error)
	AppendCatalogRevision(context.Context, domain.AppendRevisionCommand) (domain.Revision, error)
	ListCatalogRelations(context.Context, domain.ListRelationsQuery) ([]domain.Relation, error)
	GetCatalogDiscoveryRun(context.Context, identity.WorkspaceID, identity.RunID) (domain.DiscoveryRun, error)
}

type Clock interface{ Now() time.Time }
type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

type Service struct {
	repository Repository
	clock      Clock
	usage      *usageapp.Service
	authorizer authorizationapp.Evaluator
}

type Option func(*Service)

func WithUsage(service *usageapp.Service) Option {
	return func(catalog *Service) { catalog.usage = service }
}

// WithAuthorizer attaches the single capability-evaluation enforcement point.
// Production wiring always sets it; when it is absent the service runs in the
// pre-M2 mode used by unit-test harnesses, mirroring the optional usage
// service. Enforcement is server-side only (FR-012): client capability state
// can never satisfy these checks.
func WithAuthorizer(evaluator authorizationapp.Evaluator) Option {
	return func(catalog *Service) { catalog.authorizer = evaluator }
}

func NewService(repository Repository, clock Clock, options ...Option) *Service {
	if repository == nil || clock == nil {
		panic("catalog repository and clock are required")
	}
	service := &Service{repository: repository, clock: clock}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

type ListAssetsRequest struct {
	WorkspaceID  identity.WorkspaceID
	Search       string
	AssetType    semantic.AssetType
	Lifecycle    string
	Limit        int
	Cursor       string
	ActorID      string
	Channel      string
	TraceID      string
	PrincipalRef string
}

type AssetPage struct {
	Items      []domain.AssetSummary
	Limit      int
	Total      int64
	NextCursor string
}

type CreateAssetRequest struct {
	WorkspaceID   identity.WorkspaceID
	Address       string
	AssetType     semantic.AssetType
	Lifecycle     string
	SchemaVersion string
	Content       json.RawMessage
	CreatedBy     string
	EvidenceIDs   []identity.EvidenceID
	PrincipalRef  string
	TraceID       string
}

type AppendRevisionRequest struct {
	WorkspaceID   identity.WorkspaceID
	AssetID       identity.AssetID
	SchemaVersion string
	Content       json.RawMessage
	CreatedBy     string
	EvidenceIDs   []identity.EvidenceID
	PrincipalRef  string
	TraceID       string
}

type ListRevisionsRequest struct {
	WorkspaceID  identity.WorkspaceID
	AssetID      identity.AssetID
	Limit        int
	Cursor       string
	PrincipalRef string
	TraceID      string
}

type GetRevisionRequest struct {
	WorkspaceID  identity.WorkspaceID
	AssetID      identity.AssetID
	RevisionID   identity.RevisionID
	PrincipalRef string
	TraceID      string
}

type ListRelationsRequest struct {
	WorkspaceID  identity.WorkspaceID
	AssetID      identity.AssetID
	Direction    string
	Plane        semantic.RelationPlane
	Depth        int
	Limit        int
	PrincipalRef string
	TraceID      string
}

type RevisionPage struct {
	Items      []domain.Revision
	Limit      int
	Total      int64
	NextCursor string
}

type ListAuthorityRecordsRequest struct {
	WorkspaceID  identity.WorkspaceID
	AssetID      identity.AssetID
	Section      string
	Limit        int
	Cursor       string
	PrincipalRef string
	TraceID      string
}

type AuthorityRecordPage struct {
	Items      []domain.AuthorityRecord
	Limit      int
	Total      int64
	NextCursor string
}

func (service *Service) ListWorkspaces(ctx context.Context) ([]domain.Workspace, error) {
	return service.repository.ListCatalogWorkspaces(ctx)
}

func (service *Service) CreateWorkspace(ctx context.Context, slug, displayName, traceID string) (domain.Workspace, error) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	displayName = strings.TrimSpace(displayName)
	if !validWorkspaceSlug(slug) || displayName == "" || len(displayName) > 120 || !validTraceID(traceID) {
		return domain.Workspace{}, domain.ErrInvalidArgument
	}
	workspaceID, err := identity.NewWorkspaceID()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create workspace ID: %w", err)
	}
	auditID, err := identity.NewEventID()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create workspace audit ID: %w", err)
	}
	return service.repository.CreateCatalogWorkspace(ctx, domain.CreateWorkspaceCommand{
		ID: workspaceID, AuditEventID: auditID, Slug: slug, DisplayName: displayName,
		TraceID: traceID, CreatedAt: service.clock.Now().UTC(),
	})
}

func validWorkspaceSlug(value string) bool {
	if len(value) < 1 || len(value) > 63 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func (service *Service) ListAssets(ctx context.Context, request ListAssetsRequest) (AssetPage, error) {
	request.Search = strings.TrimSpace(request.Search)
	limit, err := normalizeLimit(request.Limit)
	if err != nil || len(request.Search) > 256 || !validAssetType(request.AssetType, true) || !validLifecycle(request.Lifecycle) {
		return AssetPage{}, domain.ErrInvalidArgument
	}
	if err := service.authorize(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: request.PrincipalRef,
		WorkspaceID:  request.WorkspaceID,
		Action:       authz.ActionAssetRead,
		Resource:     authz.Resource{Type: authz.ScopeWorkspace, ID: request.WorkspaceID.UUID()},
		TraceID:      request.TraceID,
	}); err != nil {
		return AssetPage{}, err
	}
	filter := assetFilterDigest(request.Search, request.AssetType, request.Lifecycle)
	cursor, err := decodeAssetCursor(request.Cursor, filter)
	if err != nil {
		return AssetPage{}, err
	}
	items, err := service.repository.ListCatalogAssets(ctx, domain.ListAssetsQuery{
		WorkspaceID: request.WorkspaceID, Search: request.Search, AssetType: request.AssetType,
		Lifecycle: request.Lifecycle, Limit: limit + 1, Cursor: cursor,
	})
	if err != nil {
		service.recordSearch(ctx, request, 0, true)
		return AssetPage{}, err
	}
	total, err := service.repository.CountCatalogAssets(ctx, domain.ListAssetsQuery{
		WorkspaceID: request.WorkspaceID, Search: request.Search, AssetType: request.AssetType,
		Lifecycle: request.Lifecycle,
	})
	if err != nil {
		service.recordSearch(ctx, request, 0, true)
		return AssetPage{}, err
	}
	page := AssetPage{Items: items, Limit: limit, Total: total}
	if len(items) > limit {
		last := items[limit-1]
		page.Items = items[:limit]
		page.NextCursor = encodeCursor(assetCursorEnvelope{
			Kind: "asset", Filter: filter, Rank: last.Rank, UpdatedAt: last.UpdatedAt, ID: last.ID.String(),
		})
	}
	service.recordSearch(ctx, request, len(page.Items), false)
	return page, nil
}

func (service *Service) CreateAsset(ctx context.Context, request CreateAssetRequest) (domain.AssetDetail, error) {
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	address, err := parseAddress(request.Address)
	if err != nil || !validAssetType(request.AssetType, false) || !validLifecycle(request.Lifecycle) ||
		request.CreatedBy == "" || len(request.CreatedBy) > 256 || len(request.EvidenceIDs) > 100 || !validTraceID(request.TraceID) {
		return domain.AssetDetail{}, domain.ErrInvalidArgument
	}
	// Asset create authoring action: creating a semantic asset proposes new
	// governed content, so the command maps to the FR-003 asset.propose
	// identifier evaluated at workspace scope.
	if err := service.authorize(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: request.PrincipalRef,
		WorkspaceID:  request.WorkspaceID,
		Action:       authz.ActionAssetPropose,
		Resource:     authz.Resource{Type: authz.ScopeWorkspace, ID: request.WorkspaceID.UUID()},
		TraceID:      request.TraceID,
	}); err != nil {
		return domain.AssetDetail{}, err
	}
	content, digest, err := canonicalContent(request.Content, request.SchemaVersion)
	if err != nil {
		return domain.AssetDetail{}, err
	}
	assetID, err := identity.NewAssetID()
	if err != nil {
		return domain.AssetDetail{}, err
	}
	revisionID, auditID, outboxID, err := newRevisionEventIDs()
	if err != nil {
		return domain.AssetDetail{}, err
	}
	lifecycle := request.Lifecycle
	if lifecycle == "" {
		lifecycle = "draft"
	}
	return service.repository.CreateCatalogAsset(ctx, domain.CreateAssetCommand{
		WorkspaceID: request.WorkspaceID, AssetID: assetID, RevisionID: revisionID,
		AuditEventID: auditID, OutboxEventID: outboxID, Address: address,
		AssetType: request.AssetType, Lifecycle: lifecycle, SchemaVersion: request.SchemaVersion,
		ContentDigest: digest, Content: content, CreatedBy: request.CreatedBy,
		EvidenceIDs: uniqueEvidence(request.EvidenceIDs), TraceID: request.TraceID,
		CreatedAt: service.clock.Now().UTC(),
	})
}

func (service *Service) GetAsset(ctx context.Context, workspace identity.WorkspaceID, asset identity.AssetID) (domain.AssetDetail, error) {
	return service.GetAssetObserved(ctx, workspace, asset, ReadObservation{})
}

// authorize runs the shared M2 capability evaluation. Denials carry the audited
// decision so the HTTP boundary can answer with 403 and the stable reason code.
func (service *Service) authorize(ctx context.Context, request authorizationapp.EvaluationRequest) error {
	if service.authorizer == nil {
		return nil
	}
	decision, err := service.evaluate(ctx, request)
	if err != nil {
		return err
	}
	return authorizationDecisionError(decision)
}

func (service *Service) evaluate(ctx context.Context, request authorizationapp.EvaluationRequest) (authz.Decision, error) {
	if service.authorizer == nil {
		return authz.Decision{Allowed: true, Action: request.Action}, nil
	}
	return service.authorizer.Evaluate(ctx, request)
}

func authorizationDecisionError(decision authz.Decision) error {
	if decision.Allowed {
		return nil
	}
	return &authz.DenialError{Decision: decision}
}

type ReadObservation struct {
	ActorID      string
	Channel      string
	TraceID      string
	PrincipalRef string
}

func (service *Service) GetAssetObserved(
	ctx context.Context,
	workspace identity.WorkspaceID,
	asset identity.AssetID,
	observation ReadObservation,
) (domain.AssetDetail, error) {
	if workspace.IsZero() || asset.IsZero() {
		return domain.AssetDetail{}, domain.ErrInvalidArgument
	}
	decision, err := service.evaluate(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: observation.PrincipalRef,
		WorkspaceID:  workspace,
		Action:       authz.ActionAssetRead,
		Resource:     authz.Resource{Type: authz.ScopeAsset, ID: asset.UUID()},
		TraceID:      observation.TraceID,
	})
	if err != nil {
		return domain.AssetDetail{}, err
	}
	if err := authorizationDecisionError(decision); err != nil {
		return domain.AssetDetail{}, err
	}
	detail, err := service.repository.GetCatalogAsset(ctx, workspace, asset)
	if err != nil {
		return domain.AssetDetail{}, err
	}
	snapshot := authorizationapp.AccessSnapshot{}
	hasSectionAccess := service.authorizer == nil
	if snapshotEvaluator, ok := service.authorizer.(authorizationapp.SnapshotEvaluator); ok && !decision.PrincipalID.IsZero() {
		loaded, snapshotErr := snapshotEvaluator.Snapshot(ctx, workspace, decision.PrincipalID)
		if snapshotErr != nil {
			return domain.AssetDetail{}, snapshotErr
		}
		snapshot, hasSectionAccess = loaded, true
		if !snapshot.Allows(authz.ActionAssetRead, authz.Resource{Type: authz.ScopeAsset, ID: asset.UUID()}) {
			return domain.AssetDetail{}, domain.ErrNotFound
		}
		applyAuthoritySectionAuthorization(&detail, snapshot, asset)
	} else if service.authorizer != nil {
		applyAuthoritySectionAuthorization(&detail, authorizationapp.AccessSnapshot{}, asset)
	}
	if err := service.populateAuthorityRecordPages(ctx, workspace, asset, &detail, snapshot, hasSectionAccess); err != nil {
		return domain.AssetDetail{}, err
	}
	if service.usage != nil && detail.CurrentRevision != nil && validTraceID(observation.TraceID) {
		_ = service.usage.RecordAssetRead(ctx, usageapp.ReadInput{
			WorkspaceID: workspace, AssetID: asset, RevisionID: detail.CurrentRevision.ID,
			ActorID: observation.ActorID, Channel: observation.Channel, TraceID: observation.TraceID,
		})
	}
	return detail, nil
}

func applyAuthoritySectionAuthorization(
	detail *domain.AssetDetail,
	snapshot authorizationapp.AccessSnapshot,
	asset identity.AssetID,
) {
	resource := authz.Resource{Type: authz.ScopeAsset, ID: asset.UUID()}
	for index := range detail.AuthoritySections {
		action := authoritySectionAction(detail.AuthoritySections[index].Kind)
		if action == "" || snapshot.Allows(action, resource) {
			continue
		}
		if detail.AuthoritySections[index].Kind == "evidence" && detail.CurrentRevision != nil {
			detail.CurrentRevision.Evidence = []domain.Evidence{}
		}
		detail.AuthoritySections[index].Availability = domain.AvailabilityForbidden
		detail.AuthoritySections[index].RevisionID = nil
		detail.AuthoritySections[index].ReleaseID = nil
		detail.AuthoritySections[index].ReleaseSequence = nil
		detail.AuthoritySections[index].Values = map[string]int64{}
		detail.AuthoritySections[index].Records = []domain.AuthorityRecord{}
		detail.AuthoritySections[index].RecordsLimit = defaultAuthorityRecordLimit
		detail.AuthoritySections[index].RecordsTotal = 0
		detail.AuthoritySections[index].RecordsNextCursor = ""
	}
}

func authoritySectionAction(section string) authz.Action {
	switch section {
	case "evidence", "validation":
		return authz.ActionEvidenceRead
	case "physical_bindings", "join_contracts", "lineage", "consumer_impact":
		return authz.ActionBindingRead
	default:
		return ""
	}
}

func authorityRecordSection(section string) bool {
	switch section {
	case "relations", "physical_bindings", "join_contracts", "validation", "lineage", "consumer_impact":
		return true
	default:
		return false
	}
}

func (service *Service) populateAuthorityRecordPages(
	ctx context.Context,
	workspace identity.WorkspaceID,
	asset identity.AssetID,
	detail *domain.AssetDetail,
	snapshot authorizationapp.AccessSnapshot,
	hasSectionAccess bool,
) error {
	resource := authz.Resource{Type: authz.ScopeAsset, ID: asset.UUID()}
	for index := range detail.AuthoritySections {
		section := &detail.AuthoritySections[index]
		section.RecordsLimit = defaultAuthorityRecordLimit
		if !authorityRecordSection(section.Kind) || section.Availability == domain.AvailabilityForbidden {
			continue
		}
		action := authoritySectionAction(section.Kind)
		if action != "" && service.authorizer != nil && (!hasSectionAccess || !snapshot.Allows(action, resource)) {
			continue
		}
		page, err := service.listAuthorityRecordPage(ctx, workspace, asset, section.Kind,
			defaultAuthorityRecordLimit, nil)
		if err != nil {
			return err
		}
		section.Records, section.RecordsNextCursor = page.Items, page.NextCursor
		redactAuthorityRecordEvidence(section.Records, service.authorizer == nil ||
			hasSectionAccess && snapshot.Allows(authz.ActionEvidenceRead, resource))
		section.RecordsTotal = page.Total
		if section.Kind == "lineage" && len(page.Items) > 0 {
			section.Availability = domain.AvailabilityAvailable
		}
	}
	return nil
}

func (service *Service) ListAuthorityRecords(
	ctx context.Context, request ListAuthorityRecordsRequest,
) (AuthorityRecordPage, error) {
	if request.WorkspaceID.IsZero() || request.AssetID.IsZero() || !authorityRecordSection(request.Section) {
		return AuthorityRecordPage{}, domain.ErrInvalidArgument
	}
	limit := request.Limit
	if limit == 0 {
		limit = defaultAuthorityRecordLimit
	}
	if limit < 1 || limit > maxAuthorityRecordLimit {
		return AuthorityRecordPage{}, domain.ErrInvalidArgument
	}
	decision, err := service.evaluate(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: request.PrincipalRef, WorkspaceID: request.WorkspaceID,
		Action:   authz.ActionAssetRead,
		Resource: authz.Resource{Type: authz.ScopeAsset, ID: request.AssetID.UUID()}, TraceID: request.TraceID,
	})
	if err != nil {
		return AuthorityRecordPage{}, err
	}
	if err := authorizationDecisionError(decision); err != nil {
		return AuthorityRecordPage{}, domain.ErrNotFound
	}
	evidenceAllowed := service.authorizer == nil
	if service.authorizer != nil {
		snapshotEvaluator, ok := service.authorizer.(authorizationapp.SnapshotEvaluator)
		if !ok || decision.PrincipalID.IsZero() {
			return AuthorityRecordPage{}, domain.ErrNotFound
		}
		snapshot, snapshotErr := snapshotEvaluator.Snapshot(ctx, request.WorkspaceID, decision.PrincipalID)
		if snapshotErr != nil {
			return AuthorityRecordPage{}, snapshotErr
		}
		resource := authz.Resource{Type: authz.ScopeAsset, ID: request.AssetID.UUID()}
		if !snapshot.Allows(authz.ActionAssetRead, resource) {
			return AuthorityRecordPage{}, domain.ErrNotFound
		}
		if action := authoritySectionAction(request.Section); action != "" && !snapshot.Allows(action, resource) {
			return AuthorityRecordPage{}, domain.ErrNotFound
		}
		evidenceAllowed = snapshot.Allows(authz.ActionEvidenceRead, resource)
	}
	cursor, err := decodeAuthorityRecordCursor(request.Cursor, request.WorkspaceID, request.AssetID, request.Section)
	if err != nil {
		return AuthorityRecordPage{}, err
	}
	page, err := service.listAuthorityRecordPage(ctx, request.WorkspaceID, request.AssetID, request.Section, limit, cursor)
	if err != nil {
		return AuthorityRecordPage{}, err
	}
	redactAuthorityRecordEvidence(page.Items, evidenceAllowed)
	return page, nil
}

func redactAuthorityRecordEvidence(records []domain.AuthorityRecord, allowed bool) {
	if allowed {
		return
	}
	for index := range records {
		if records[index].ModelGrain != nil {
			records[index].ModelGrain.DocumentedBy = nil
		}
	}
}

func (service *Service) listAuthorityRecordPage(
	ctx context.Context,
	workspace identity.WorkspaceID,
	asset identity.AssetID,
	section string,
	limit int,
	cursor *domain.AuthorityRecordCursor,
) (AuthorityRecordPage, error) {
	result, err := service.repository.ListCatalogAuthorityRecords(ctx, domain.ListAuthorityRecordsQuery{
		WorkspaceID: workspace, AssetID: asset, Section: section, Limit: limit + 1, Cursor: cursor,
	})
	if err != nil {
		return AuthorityRecordPage{}, err
	}
	page := AuthorityRecordPage{Items: result.Items, Limit: limit, Total: result.Total}
	if len(result.Items) > limit {
		last := result.Items[limit-1]
		page.Items = result.Items[:limit]
		page.NextCursor = encodeCursor(authorityRecordCursorEnvelope{Kind: "catalog-authority-records",
			WorkspaceID: workspace.String(), AssetID: asset.String(), Section: section,
			RecordKind: last.Kind, RecordID: last.ID})
	}
	return page, nil
}

func (service *Service) recordSearch(ctx context.Context, request ListAssetsRequest, resultCount int, failed bool) {
	if service.usage == nil || request.Search == "" || !validTraceID(request.TraceID) {
		return
	}
	_ = service.usage.RecordSearch(ctx, usageapp.SearchInput{
		WorkspaceID: request.WorkspaceID, Query: request.Search, AssetTypeFilter: string(request.AssetType),
		LifecycleFilter: request.Lifecycle, ResultCount: resultCount, Failed: failed,
		ReasonCode: "CATALOG_QUERY_FAILED", ActorID: request.ActorID, Channel: request.Channel, TraceID: request.TraceID,
	})
}

func (service *Service) ListRevisions(ctx context.Context, request ListRevisionsRequest) (RevisionPage, error) {
	limit, err := normalizeLimit(request.Limit)
	if err != nil || request.WorkspaceID.IsZero() || request.AssetID.IsZero() {
		return RevisionPage{}, domain.ErrInvalidArgument
	}
	if err := service.authorizeAssetReadSafe(ctx, request.WorkspaceID, request.AssetID, request.PrincipalRef, request.TraceID); err != nil {
		return RevisionPage{}, err
	}
	cursor, err := decodeRevisionCursor(request.Cursor, request.AssetID)
	if err != nil {
		return RevisionPage{}, err
	}
	items, err := service.repository.ListCatalogRevisions(ctx, domain.ListRevisionsQuery{
		WorkspaceID: request.WorkspaceID, AssetID: request.AssetID, Limit: limit + 1, Cursor: cursor,
	})
	if err != nil {
		return RevisionPage{}, err
	}
	total, err := service.repository.CountCatalogRevisions(ctx, request.WorkspaceID, request.AssetID)
	if err != nil {
		return RevisionPage{}, err
	}
	page := RevisionPage{Items: items, Limit: limit, Total: total}
	if len(items) > limit {
		last := items[limit-1]
		page.Items = items[:limit]
		page.NextCursor = encodeCursor(revisionCursorEnvelope{
			Kind: "revision", AssetID: request.AssetID.String(), Sequence: last.Sequence, ID: last.ID.String(),
		})
	}
	return page, nil
}

func (service *Service) GetRevision(ctx context.Context, request GetRevisionRequest) (domain.Revision, error) {
	if request.WorkspaceID.IsZero() || request.AssetID.IsZero() || request.RevisionID.IsZero() {
		return domain.Revision{}, domain.ErrInvalidArgument
	}
	if err := service.authorizeAssetReadSafe(ctx, request.WorkspaceID, request.AssetID, request.PrincipalRef, request.TraceID); err != nil {
		return domain.Revision{}, err
	}
	return service.repository.GetCatalogRevision(ctx, request.WorkspaceID, request.AssetID, request.RevisionID)
}

func (service *Service) AppendRevision(ctx context.Context, request AppendRevisionRequest) (domain.Revision, error) {
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if request.CreatedBy == "" || len(request.CreatedBy) > 256 || len(request.EvidenceIDs) > 100 || !validTraceID(request.TraceID) {
		return domain.Revision{}, domain.ErrInvalidArgument
	}
	// Revision append authoring action: appending an immutable revision edits
	// the asset's governed content, so the command maps to the FR-003
	// asset.edit identifier evaluated against the asset resource.
	if err := service.authorize(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: request.PrincipalRef,
		WorkspaceID:  request.WorkspaceID,
		Action:       authz.ActionAssetEdit,
		Resource:     authz.Resource{Type: authz.ScopeAsset, ID: request.AssetID.UUID()},
		TraceID:      request.TraceID,
	}); err != nil {
		return domain.Revision{}, err
	}
	content, digest, err := canonicalContent(request.Content, request.SchemaVersion)
	if err != nil {
		return domain.Revision{}, err
	}
	revisionID, auditID, outboxID, err := newRevisionEventIDs()
	if err != nil {
		return domain.Revision{}, err
	}
	return service.repository.AppendCatalogRevision(ctx, domain.AppendRevisionCommand{
		WorkspaceID: request.WorkspaceID, AssetID: request.AssetID, RevisionID: revisionID,
		AuditEventID: auditID, OutboxEventID: outboxID, SchemaVersion: request.SchemaVersion,
		ContentDigest: digest, Content: content, CreatedBy: request.CreatedBy,
		EvidenceIDs: uniqueEvidence(request.EvidenceIDs), TraceID: request.TraceID,
		CreatedAt: service.clock.Now().UTC(),
	})
}

func (service *Service) ListRelations(ctx context.Context, request ListRelationsRequest) ([]domain.Relation, error) {
	if request.Direction == "" {
		request.Direction = "both"
	}
	if request.Depth == 0 {
		request.Depth = 1
	}
	limit, err := normalizeLimit(request.Limit)
	if err != nil {
		return nil, err
	}
	if request.WorkspaceID.IsZero() || request.AssetID.IsZero() || request.Depth < 1 || request.Depth > maxDepth ||
		(request.Direction != "incoming" && request.Direction != "outgoing" && request.Direction != "both") ||
		(request.Plane != "" && request.Plane != semantic.TaxonomyPlane && request.Plane != semantic.SemanticPlane && request.Plane != semantic.DependencyPlane) {
		return nil, domain.ErrInvalidArgument
	}
	if err := service.authorizeAssetReadSafe(ctx, request.WorkspaceID, request.AssetID, request.PrincipalRef, request.TraceID); err != nil {
		return nil, err
	}
	items, err := service.repository.ListCatalogRelations(ctx, domain.ListRelationsQuery{
		WorkspaceID: request.WorkspaceID, AssetID: request.AssetID, Direction: request.Direction,
		Plane: request.Plane, Depth: request.Depth, Limit: limit + 1,
	})
	if err != nil {
		return nil, err
	}
	if len(items) > limit {
		return nil, domain.ErrInvariant
	}
	// The legacy graph projection is asset.read only. Evidence identifiers are
	// available through evidence-authorized catalog surfaces, never this route.
	for index := range items {
		items[index].EvidenceArtifactID = nil
	}
	return items, nil
}

func (service *Service) authorizeAssetReadSafe(
	ctx context.Context,
	workspace identity.WorkspaceID,
	asset identity.AssetID,
	principalRef string,
	traceID string,
) error {
	err := service.authorize(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: principalRef,
		WorkspaceID:  workspace,
		Action:       authz.ActionAssetRead,
		Resource:     authz.Resource{Type: authz.ScopeAsset, ID: asset.UUID()},
		TraceID:      traceID,
	})
	var denial *authz.DenialError
	if errors.As(err, &denial) {
		return domain.ErrNotFound
	}
	return err
}

func (service *Service) GetDiscoveryRun(ctx context.Context, workspace identity.WorkspaceID, run identity.RunID) (domain.DiscoveryRun, error) {
	return service.repository.GetCatalogDiscoveryRun(ctx, workspace, run)
}

func normalizeLimit(value int) (int, error) {
	if value == 0 {
		return defaultLimit, nil
	}
	if value < 1 || value > maxLimit {
		return 0, domain.ErrInvalidArgument
	}
	return value, nil
}

func canonicalContent(content json.RawMessage, schemaVersion string) (json.RawMessage, string, error) {
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if !semanticVersion(schemaVersion) || len(content) == 0 || len(content) > 1<<20 || decoder.Decode(&object) != nil || object == nil {
		return nil, "", domain.ErrInvalidArgument
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, "", domain.ErrInvalidArgument
	}
	if kind, present := object["assetType"]; present {
		name, ok := kind.(string)
		if !ok {
			return nil, "", domain.ErrInvalidArgument
		}
		raw, _ := json.Marshal(object["spec"])
		if _, err := semantic.ParseKnowledgeSpec(semantic.AssetType(name), raw, false); err != nil {
			return nil, "", domain.ErrInvalidArgument
		}
	} else if _, present := object["spec"]; present {
		return nil, "", domain.ErrInvalidArgument
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return nil, "", domain.ErrInvalidArgument
	}
	digest := sha256.Sum256(encoded)
	return encoded, "sha256:" + hex.EncodeToString(digest[:]), nil
}

func parseAddress(value string) (semantic.Address, error) {
	index := strings.LastIndex(value, ".")
	if index < 1 || index == len(value)-1 {
		return semantic.Address{}, domain.ErrInvalidArgument
	}
	address, err := semantic.NewAddress(value[:index], value[index+1:])
	if err != nil {
		return semantic.Address{}, domain.ErrInvalidArgument
	}
	return address, nil
}

func validAssetType(value semantic.AssetType, optional bool) bool {
	if value == "" {
		return optional
	}
	switch value {
	case semantic.BusinessObject, semantic.BusinessTerm, semantic.AnalysisModel, semantic.DataAsset, semantic.Metric:
		return true
	default:
		return false
	}
}

func validLifecycle(value string) bool {
	if value == "" {
		return true
	}
	switch value {
	case "draft", "active", "deprecated", "archived":
		return true
	default:
		return false
	}
}

func semanticVersion(value string) bool {
	return semantic.AssetRevision{Sequence: 1, SchemaVersion: value, ContentDigest: "sha256:x"}.Validate() == nil
}

func validTraceID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func uniqueEvidence(values []identity.EvidenceID) []identity.EvidenceID {
	seen := make(map[string]struct{}, len(values))
	result := make([]identity.EvidenceID, 0, len(values))
	for _, value := range values {
		key := value.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func newRevisionEventIDs() (identity.RevisionID, identity.EventID, identity.EventID, error) {
	revision, err := identity.NewRevisionID()
	if err != nil {
		return identity.RevisionID{}, identity.EventID{}, identity.EventID{}, err
	}
	audit, err := identity.NewEventID()
	if err != nil {
		return identity.RevisionID{}, identity.EventID{}, identity.EventID{}, err
	}
	outbox, err := identity.NewEventID()
	return revision, audit, outbox, err
}

func assetFilterDigest(search string, assetType semantic.AssetType, lifecycle string) string {
	sum := sha256.Sum256([]byte(search + "\x00" + string(assetType) + "\x00" + lifecycle))
	return hex.EncodeToString(sum[:])
}

type assetCursorEnvelope struct {
	Kind      string    `json:"k"`
	Filter    string    `json:"f"`
	Rank      float64   `json:"r"`
	UpdatedAt time.Time `json:"u"`
	ID        string    `json:"i"`
}

type revisionCursorEnvelope struct {
	Kind     string `json:"k"`
	AssetID  string `json:"a"`
	Sequence int64  `json:"s"`
	ID       string `json:"i"`
}

type authorityRecordCursorEnvelope struct {
	Kind        string `json:"k"`
	WorkspaceID string `json:"w"`
	AssetID     string `json:"a"`
	Section     string `json:"s"`
	RecordKind  string `json:"rk"`
	RecordID    string `json:"ri"`
}

func encodeCursor(value any) string {
	encoded, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeAssetCursor(value, filter string) (*domain.AssetCursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 2048 {
		return nil, domain.ErrInvalidArgument
	}
	var envelope assetCursorEnvelope
	if err := decodeCursor(value, &envelope); err != nil || envelope.Kind != "asset" || envelope.Filter != filter || envelope.UpdatedAt.IsZero() {
		return nil, domain.ErrInvalidArgument
	}
	id, err := identity.ParseAssetID(envelope.ID)
	if err != nil {
		return nil, domain.ErrInvalidArgument
	}
	return &domain.AssetCursor{Rank: envelope.Rank, UpdatedAt: envelope.UpdatedAt.UTC(), ID: id}, nil
}

func decodeRevisionCursor(value string, asset identity.AssetID) (*domain.RevisionCursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 2048 {
		return nil, domain.ErrInvalidArgument
	}
	var envelope revisionCursorEnvelope
	if err := decodeCursor(value, &envelope); err != nil || envelope.Kind != "revision" || envelope.AssetID != asset.String() || envelope.Sequence < 1 {
		return nil, domain.ErrInvalidArgument
	}
	id, err := identity.ParseRevisionID(envelope.ID)
	if err != nil {
		return nil, domain.ErrInvalidArgument
	}
	return &domain.RevisionCursor{Sequence: envelope.Sequence, ID: id}, nil
}

func decodeAuthorityRecordCursor(
	value string,
	workspace identity.WorkspaceID,
	asset identity.AssetID,
	section string,
) (*domain.AuthorityRecordCursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 2048 {
		return nil, domain.ErrInvalidArgument
	}
	var envelope authorityRecordCursorEnvelope
	if err := decodeCursor(value, &envelope); err != nil || envelope.Kind != "catalog-authority-records" ||
		envelope.WorkspaceID != workspace.String() || envelope.AssetID != asset.String() || envelope.Section != section {
		return nil, domain.ErrInvalidArgument
	}
	expected, ok := authorityRecordPrefix(envelope.RecordKind)
	if !ok {
		return nil, domain.ErrInvalidArgument
	}
	if _, err := identity.Parse(expected, envelope.RecordID); err != nil {
		return nil, domain.ErrInvalidArgument
	}
	return &domain.AuthorityRecordCursor{Kind: envelope.RecordKind, ID: envelope.RecordID}, nil
}

func authorityRecordPrefix(kind string) (identity.Prefix, bool) {
	switch kind {
	case "relation":
		return identity.Relation, true
	case "physical_binding":
		return identity.PhysicalBinding, true
	case "model_grain":
		return identity.ModelGrain, true
	case "entity_key":
		return identity.EntityKey, true
	case "join_contract":
		return identity.JoinContract, true
	case "validation_run":
		return identity.ValidationRun, true
	case "lineage":
		return identity.LineageEdge, true
	case "consumer_binding":
		return identity.ConsumerBinding, true
	default:
		return "", false
	}
}

func decodeCursor(value string, target any) error {
	encoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(encoded) > 2048 {
		return domain.ErrInvalidArgument
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing cursor content")
	}
	return nil
}

func ParseAssetID(value string) (identity.AssetID, error) {
	id, err := identity.ParseAssetID(value)
	if err != nil {
		return identity.AssetID{}, fmt.Errorf("%w: asset ID", domain.ErrInvalidArgument)
	}
	return id, nil
}
