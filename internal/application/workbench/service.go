package workbench

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/workbench"
	"github.com/iiwish/semlia/pkg/identity"
)

const (
	defaultLimit     = 50
	maxLimit         = 100
	maxStartupItems  = 10_000
	maxCursorEncoded = 2048
)

type Repository interface {
	ListAttentionItems(context.Context, domain.ListQuery) ([]domain.Item, error)
	CountAttentionItems(context.Context, domain.ListQuery) (domain.Counts, error)
	GetAttentionItem(context.Context, identity.WorkspaceID, identity.AttentionItemID) (domain.Item, error)
	ReviewedAttentionItemIDs(context.Context, identity.WorkspaceID, identity.PrincipalID, []identity.AttentionItemID) (map[string]struct{}, error)
	UpdateAttentionItem(context.Context, domain.UpdateCommand) (domain.Item, error)
	ReconcileAttentionItems(context.Context, int) (domain.ReconcileStats, error)
}

type Clock interface{ Now() time.Time }
type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

type Service struct {
	repository Repository
	authorizer authorizationapp.SnapshotEvaluator
	clock      Clock
}

func NewService(repository Repository, authorizer authorizationapp.SnapshotEvaluator, clock Clock) *Service {
	if repository == nil || authorizer == nil || clock == nil {
		panic("workbench repository, authorizer and clock are required")
	}
	return &Service{repository: repository, authorizer: authorizer, clock: clock}
}

type ListRequest struct {
	WorkspaceID  identity.WorkspaceID
	PrincipalRef string
	TraceID      string
	View         domain.View
	Search       string
	Kind         domain.Kind
	State        domain.State
	Priority     domain.Priority
	Risk         domain.Priority
	Sort         domain.Sort
	Limit        int
	Cursor       string
}

type Page struct {
	Items      []domain.Item
	Counts     domain.Counts
	Limit      int
	NextCursor string
}

func (service *Service) List(ctx context.Context, request ListRequest) (Page, error) {
	request.PrincipalRef = strings.TrimSpace(request.PrincipalRef)
	if request.WorkspaceID.IsZero() || request.PrincipalRef == "" || !validTraceID(request.TraceID) {
		return Page{}, domain.ErrInvalidArgument
	}
	if request.View == "" {
		request.View = domain.ViewMine
	}
	if request.Sort == "" {
		request.Sort = domain.SortUpdatedDesc
	}
	request.Search = strings.TrimSpace(request.Search)
	limit, err := normalizeLimit(request.Limit)
	if err != nil || !request.View.Valid() || !request.Sort.Valid() ||
		len(request.Search) > 256 ||
		(request.Kind != "" && !request.Kind.Valid()) || (request.State != "" && !request.State.Valid()) ||
		(request.Priority != "" && !request.Priority.Valid()) || (request.Risk != "" && !request.Risk.Valid()) {
		return Page{}, domain.ErrInvalidArgument
	}
	principal, err := service.authorize(ctx, request.WorkspaceID, request.PrincipalRef, authorization.ActionWorkspaceRead, request.TraceID)
	if err != nil {
		return Page{}, err
	}
	snapshot, err := service.authorizer.Snapshot(ctx, request.WorkspaceID, principal)
	if err != nil {
		return Page{}, err
	}
	if err := requireSnapshotAction(snapshot, authorization.ActionWorkspaceRead, request.WorkspaceID); err != nil {
		return Page{}, err
	}
	allowedKinds := allowedReadKinds(snapshot)
	filter := cursorFilter(request)
	cursor, err := decodeCursor(request.Cursor, request.WorkspaceID, principal, snapshot.AuthorizationVersion, filter, request.Sort)
	if err != nil {
		return Page{}, err
	}
	query := domain.ListQuery{WorkspaceID: request.WorkspaceID, PrincipalID: principal,
		AccessGrants: workbenchGrants(snapshot), AllowedKinds: allowedKinds, View: request.View, Search: request.Search,
		Kind: request.Kind, State: request.State, Priority: request.Priority, Risk: request.Risk, Sort: request.Sort,
		AuthorizationVersion: snapshot.AuthorizationVersion, Limit: limit + 1, Cursor: cursor}
	items, err := service.repository.ListAttentionItems(ctx, query)
	if err != nil {
		return Page{}, err
	}
	counts, err := service.repository.CountAttentionItems(ctx, query)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: items, Counts: counts, Limit: limit}
	if len(items) > limit {
		last := items[limit-1]
		page.Items = items[:limit]
		page.NextCursor, err = encodeCursor(request.WorkspaceID, principal, snapshot.AuthorizationVersion, filter, request.Sort, last)
		if err != nil {
			return Page{}, err
		}
	}
	if err := service.markReviewed(ctx, request.WorkspaceID, principal, page.Items); err != nil {
		return Page{}, err
	}
	for index := range page.Items {
		page.Items[index].NextActions = nextActions(snapshot, request.WorkspaceID, principal, page.Items[index])
	}
	return page, nil
}

func (service *Service) Get(ctx context.Context, workspace identity.WorkspaceID, itemID identity.AttentionItemID, principalRef, traceID string) (domain.Item, error) {
	principal, err := service.authorize(ctx, workspace, principalRef, authorization.ActionWorkspaceRead, traceID)
	if err != nil {
		return domain.Item{}, err
	}
	snapshot, err := service.authorizer.Snapshot(ctx, workspace, principal)
	if err != nil {
		return domain.Item{}, err
	}
	if err := requireSnapshotAction(snapshot, authorization.ActionWorkspaceRead, workspace); err != nil {
		return domain.Item{}, err
	}
	item, err := service.repository.GetAttentionItem(ctx, workspace, itemID)
	if err != nil {
		return domain.Item{}, err
	}
	readAction := readAction(item.Kind)
	if readAction == "" {
		return domain.Item{}, domain.ErrNotFound
	}
	if !snapshot.Allows(readAction, itemResource(workspace, item)) || !visibleTo(snapshot, workspace, principal, item) {
		return domain.Item{}, domain.ErrNotFound
	}
	items := []domain.Item{item}
	if err := service.markReviewed(ctx, workspace, principal, items); err != nil {
		return domain.Item{}, err
	}
	item = items[0]
	item.NextActions = nextActions(snapshot, workspace, principal, item)
	return item, nil
}

type UpdateRequest struct {
	WorkspaceID         identity.WorkspaceID
	ItemID              identity.AttentionItemID
	PrincipalRef        string
	TraceID             string
	IdempotencyKey      string
	AssigneePrincipalID *identity.PrincipalID
	SetAssignee         bool
	State               domain.State
	ExpectedVersion     int64
}

func (service *Service) Update(ctx context.Context, request UpdateRequest) (domain.Item, error) {
	request.PrincipalRef = strings.TrimSpace(request.PrincipalRef)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.WorkspaceID.IsZero() || request.ItemID.IsZero() || request.ExpectedVersion < 1 ||
		request.PrincipalRef == "" || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 256 ||
		!validTraceID(request.TraceID) || (!request.SetAssignee && request.State == "") ||
		(request.State != "" && request.State != domain.StateOpen &&
			request.State != domain.StateInProgress && request.State != domain.StateDismissed) {
		return domain.Item{}, domain.ErrInvalidArgument
	}
	actor, err := service.authorize(ctx, request.WorkspaceID, request.PrincipalRef, authorization.ActionWorkspaceManage, request.TraceID)
	if err != nil {
		return domain.Item{}, err
	}
	snapshot, err := service.authorizer.Snapshot(ctx, request.WorkspaceID, actor)
	if err != nil {
		return domain.Item{}, err
	}
	if err := requireSnapshotAction(snapshot, authorization.ActionWorkspaceManage, request.WorkspaceID); err != nil {
		return domain.Item{}, err
	}
	current, err := service.repository.GetAttentionItem(ctx, request.WorkspaceID, request.ItemID)
	if err != nil {
		return domain.Item{}, err
	}
	read := readAction(current.Kind)
	if read == "" || !snapshot.Allows(read, itemResource(request.WorkspaceID, current)) ||
		!visibleTo(snapshot, request.WorkspaceID, actor, current) {
		return domain.Item{}, domain.ErrNotFound
	}
	auditID, err := identity.NewEventID()
	if err != nil {
		return domain.Item{}, err
	}
	fingerprint, err := updateFingerprint(request)
	if err != nil {
		return domain.Item{}, err
	}
	item, err := service.repository.UpdateAttentionItem(ctx, domain.UpdateCommand{
		WorkspaceID: request.WorkspaceID, ItemID: request.ItemID, ActorPrincipalID: actor,
		AssigneePrincipalID: request.AssigneePrincipalID, SetAssignee: request.SetAssignee, State: request.State,
		ExpectedVersion: request.ExpectedVersion, TraceID: request.TraceID, UpdatedAt: service.clock.Now().UTC(), AuditEventID: auditID,
		IdempotencyKey: request.IdempotencyKey, RequestFingerprint: fingerprint,
		AuthorizationVersion: snapshot.AuthorizationVersion, VisibilityFingerprint: domain.VisibilityFingerprint(current),
	})
	if err != nil {
		return domain.Item{}, err
	}
	items := []domain.Item{item}
	if err := service.markReviewed(ctx, request.WorkspaceID, actor, items); err != nil {
		return domain.Item{}, err
	}
	item = items[0]
	item.NextActions = nextActions(snapshot, request.WorkspaceID, actor, item)
	return item, nil
}

func visibleTo(snapshot authorizationapp.AccessSnapshot, workspace identity.WorkspaceID, principal identity.PrincipalID, item domain.Item) bool {
	if item.InitiatorPrincipalID != nil && *item.InitiatorPrincipalID == principal ||
		item.AssigneePrincipalID != nil && *item.AssigneePrincipalID == principal {
		return true
	}
	if item.AssigneePrincipalID != nil {
		return false
	}
	if item.AudienceRoleID == "" {
		return true
	}
	resource := itemResource(workspace, item)
	for _, grant := range snapshot.ActiveGrants() {
		if grant.RoleID != item.AudienceRoleID {
			continue
		}
		if grant.ScopeType == authorization.ScopeWorkspace && grant.ScopeID == workspace.UUID() ||
			grant.ScopeType == resource.Type && grant.ScopeID == resource.ID ||
			grant.ScopeType == authorization.ScopeDomain && resource.DomainID != "" && grant.ScopeID == resource.DomainID {
			return true
		}
	}
	return false
}

func (service *Service) ReconcileStartup(ctx context.Context, limit int) (domain.ReconcileStats, error) {
	if limit == 0 {
		limit = maxStartupItems
	}
	if limit < 1 || limit > maxStartupItems {
		return domain.ReconcileStats{}, domain.ErrInvalidArgument
	}
	return service.repository.ReconcileAttentionItems(ctx, limit)
}

func allowedReadKinds(snapshot authorizationapp.AccessSnapshot) []domain.Kind {
	result := make([]domain.Kind, 0, 5)
	for _, candidate := range []struct {
		kind   domain.Kind
		action authorization.Action
	}{{domain.KindReview, authorization.ActionAssetRead}, {domain.KindValidation, authorization.ActionAssetRead},
		{domain.KindSource, authorization.ActionSourceRead}, {domain.KindRuntime, authorization.ActionRuntimeRead},
		{domain.KindCompatibility, authorization.ActionBindingRead}} {
		if snapshotAllowsAny(snapshot, candidate.action) {
			result = append(result, candidate.kind)
		}
	}
	return result
}

func nextActions(snapshot authorizationapp.AccessSnapshot, workspace identity.WorkspaceID, principal identity.PrincipalID, item domain.Item) []domain.Action {
	actions := []domain.Action{domain.ActionOpenTarget}
	command := commandAction(item.Kind)
	if command != "" && !item.ReviewedByPrincipal && (item.InitiatorPrincipalID == nil || *item.InitiatorPrincipalID != principal ||
		(item.Kind != domain.KindReview && item.Kind != domain.KindCompatibility)) {
		if snapshot.Allows(command, commandResource(workspace, item)) && actionName(item.Kind) != "" {
			actions = append(actions, actionName(item.Kind))
		}
	}
	if snapshot.Allows(authorization.ActionWorkspaceManage, authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}) {
		actions = append(actions, domain.ActionAssign)
		if item.State != domain.StateResolved {
			actions = append(actions, domain.ActionDismiss)
		}
	}
	return actions
}

func (service *Service) markReviewed(ctx context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID, items []domain.Item) error {
	ids := make([]identity.AttentionItemID, 0, len(items))
	for _, item := range items {
		if item.Kind == domain.KindReview {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	reviewed, err := service.repository.ReviewedAttentionItemIDs(ctx, workspace, principal, ids)
	if err != nil {
		return err
	}
	for index := range items {
		_, items[index].ReviewedByPrincipal = reviewed[items[index].ID.String()]
	}
	return nil
}

func (service *Service) authorize(ctx context.Context, workspace identity.WorkspaceID, principalRef string, action authorization.Action, traceID string) (identity.PrincipalID, error) {
	decision, err := service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{
		PrincipalRef: strings.TrimSpace(principalRef), WorkspaceID: workspace, Action: action,
		Resource: authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}, TraceID: traceID,
	})
	if err != nil {
		return identity.PrincipalID{}, err
	}
	if !decision.Allowed || decision.PrincipalID.IsZero() {
		return identity.PrincipalID{}, &authorization.DenialError{Decision: decision}
	}
	return decision.PrincipalID, nil
}

func requireSnapshotAction(snapshot authorizationapp.AccessSnapshot, action authorization.Action, workspace identity.WorkspaceID) error {
	if snapshot.Allows(action, authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}) {
		return nil
	}
	return &authorization.DenialError{Decision: authorization.Decision{
		Allowed: false, Action: action, PrincipalID: snapshot.Principal.ID,
		ReasonCode: authorization.ReasonNoMatchingGrant, AuthorizationVersion: snapshot.AuthorizationVersion,
	}}
}

func readAction(kind domain.Kind) authorization.Action {
	switch kind {
	case domain.KindReview, domain.KindValidation:
		return authorization.ActionAssetRead
	case domain.KindSource:
		return authorization.ActionSourceRead
	case domain.KindRuntime:
		return authorization.ActionRuntimeRead
	case domain.KindCompatibility:
		return authorization.ActionBindingRead
	}
	return ""
}

func commandAction(kind domain.Kind) authorization.Action {
	switch kind {
	case domain.KindReview:
		return authorization.ActionProposalReview
	case domain.KindValidation:
		return authorization.ActionValidationRun
	case domain.KindSource:
		return authorization.ActionSourceManage
	case domain.KindCompatibility:
		return authorization.ActionBindingManage
	}
	return ""
}

func actionName(kind domain.Kind) domain.Action {
	switch kind {
	case domain.KindReview:
		return domain.ActionReview
	case domain.KindValidation:
		return domain.ActionValidate
	case domain.KindSource:
		return domain.ActionManage
	case domain.KindCompatibility:
		return domain.ActionPublish
	}
	return ""
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

type cursorEnvelope struct {
	Kind                 string      `json:"kind"`
	WorkspaceID          string      `json:"workspaceId"`
	PrincipalID          string      `json:"principalId"`
	AuthorizationVersion int64       `json:"authorizationVersion"`
	Filter               string      `json:"filter"`
	Sort                 domain.Sort `json:"sort"`
	UpdatedAt            time.Time   `json:"updatedAt"`
	ID                   string      `json:"id"`
	PriorityRank         int         `json:"priorityRank"`
	DueAt                *time.Time  `json:"dueAt,omitempty"`
}

func encodeCursor(workspace identity.WorkspaceID, principal identity.PrincipalID, authorizationVersion int64, filter string, sort domain.Sort, item domain.Item) (string, error) {
	due := item.DueAt
	if due != nil {
		value := due.UTC()
		due = &value
	}
	encoded, err := json.Marshal(cursorEnvelope{Kind: "workbench", WorkspaceID: workspace.String(), PrincipalID: principal.String(), AuthorizationVersion: authorizationVersion, Filter: filter,
		Sort: sort, UpdatedAt: item.UpdatedAt.UTC(), ID: item.ID.String(), PriorityRank: priorityRank(item.Priority), DueAt: due})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeCursor(value string, workspace identity.WorkspaceID, principal identity.PrincipalID, authorizationVersion int64, filter string, sort domain.Sort) (*domain.Cursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > maxCursorEncoded {
		return nil, domain.ErrInvalidArgument
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) > maxCursorEncoded {
		return nil, domain.ErrInvalidArgument
	}
	var envelope cursorEnvelope
	if err := decodeStrictJSON(decoded, &envelope); err != nil || envelope.Kind != "workbench" ||
		envelope.WorkspaceID != workspace.String() || envelope.PrincipalID != principal.String() ||
		envelope.AuthorizationVersion != authorizationVersion || envelope.Filter != filter || envelope.Sort != sort ||
		envelope.UpdatedAt.IsZero() || envelope.PriorityRank < 1 || envelope.PriorityRank > 4 {
		return nil, domain.ErrInvalidArgument
	}
	id, err := identity.ParseAttentionItemID(envelope.ID)
	if err != nil {
		return nil, domain.ErrInvalidArgument
	}
	return &domain.Cursor{UpdatedAt: envelope.UpdatedAt.UTC(), ID: id, PriorityRank: envelope.PriorityRank, DueAt: envelope.DueAt}, nil
}

func decodeStrictJSON(value []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.ErrInvalidArgument
	}
	return nil
}

func cursorFilter(request ListRequest) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{request.WorkspaceID.String(), string(request.View), string(request.Kind),
		request.Search, string(request.State), string(request.Priority), string(request.Risk), string(request.Sort)}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func updateFingerprint(request UpdateRequest) (string, error) {
	type input struct {
		ItemID          string `json:"itemId"`
		Assignee        string `json:"assignee,omitempty"`
		SetAssignee     bool   `json:"setAssignee"`
		State           string `json:"state,omitempty"`
		ExpectedVersion int64  `json:"expectedVersion"`
	}
	value := input{ItemID: request.ItemID.String(), SetAssignee: request.SetAssignee,
		State: string(request.State), ExpectedVersion: request.ExpectedVersion}
	if request.AssigneePrincipalID != nil {
		value.Assignee = request.AssigneePrincipalID.String()
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func snapshotAllowsAny(snapshot authorizationapp.AccessSnapshot, action authorization.Action) bool {
	for _, grant := range snapshot.ActiveGrants() {
		if grant.Action == action {
			return true
		}
	}
	return false
}

func workbenchGrants(snapshot authorizationapp.AccessSnapshot) []domain.AccessGrant {
	grants := snapshot.ActiveGrants()
	result := make([]domain.AccessGrant, 0, len(grants))
	for _, grant := range grants {
		if grant.Action == authorization.ActionWorkspaceRead || grant.Action == authorization.ActionWorkspaceManage {
			continue
		}
		if grant.ScopeType == authorization.ScopeDomain {
			continue
		}
		if grant.ScopeType == authorization.ScopeWorkspace {
			result = append(result, domain.AccessGrant{RoleID: grant.RoleID, Action: string(grant.Action),
				ScopeType: "*", ScopeID: "*"})
			continue
		}
		scopeID := grant.ScopeID
		if converted, ok := publicScopeID(grant.ScopeType, grant.ScopeID); ok {
			scopeID = converted
		}
		result = append(result, domain.AccessGrant{RoleID: grant.RoleID, Action: string(grant.Action),
			ScopeType: string(grant.ScopeType), ScopeID: scopeID})
	}
	return result
}

func publicScopeID(scope authorization.ScopeType, value string) (string, bool) {
	prefix := identity.Prefix("")
	switch scope {
	case authorization.ScopeWorkspace:
		prefix = identity.Workspace
	case authorization.ScopeAsset:
		prefix = identity.Asset
	case authorization.ScopeSource:
		prefix = identity.SourceConnection
	case authorization.ScopeRelease:
		prefix = identity.Release
	case authorization.ScopeConsumer:
		prefix = identity.Consumer
	default:
		return value, false
	}
	id, err := identity.FromUUID(prefix, value)
	if err != nil {
		return value, false
	}
	return id.String(), true
}

func itemResource(workspace identity.WorkspaceID, item domain.Item) authorization.Resource {
	resource := authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}
	var scope authorization.ScopeType
	switch item.TargetType {
	case string(authorization.ScopeAsset):
		scope = authorization.ScopeAsset
	case string(authorization.ScopeSource):
		scope = authorization.ScopeSource
	case string(authorization.ScopeEnvironment):
		scope = authorization.ScopeEnvironment
	case string(authorization.ScopeRelease):
		scope = authorization.ScopeRelease
	case string(authorization.ScopeConsumer):
		scope = authorization.ScopeConsumer
	case string(authorization.ScopeDomain):
		scope = authorization.ScopeDomain
	default:
		return resource
	}
	id := item.TargetID
	if parsed, err := identity.ParseAny(id); err == nil {
		id = parsed.UUID()
	}
	resource.Type, resource.ID = scope, id
	if scope == authorization.ScopeDomain {
		resource.DomainID = id
	}
	return resource
}

func commandResource(workspace identity.WorkspaceID, item domain.Item) authorization.Resource {
	if item.Kind == domain.KindReview {
		return authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}
	}
	return itemResource(workspace, item)
}

func priorityRank(priority domain.Priority) int {
	switch priority {
	case domain.PriorityCritical:
		return 4
	case domain.PriorityHigh:
		return 3
	case domain.PriorityMedium:
		return 2
	default:
		return 1
	}
}

func validTraceID(value string) bool {
	return len(value) == 32 && value == strings.ToLower(value) && func() bool { _, err := hex.DecodeString(value); return err == nil }()
}
