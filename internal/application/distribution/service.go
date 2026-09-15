package distribution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/pkg/identity"
)

type Clock interface{ Now() time.Time }
type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

type Repository interface {
	CreateConsumer(context.Context, domain.Consumer) (domain.Consumer, error)
	GetConsumer(context.Context, identity.WorkspaceID, identity.ConsumerID) (domain.Consumer, error)
	ListConsumers(context.Context, identity.WorkspaceID) ([]domain.Consumer, error)
	UpdateConsumer(context.Context, domain.Consumer) (domain.Consumer, error)
	CreateConsumerBinding(context.Context, domain.ConsumerBinding) (domain.ConsumerBinding, error)
	GetConsumerBinding(context.Context, identity.WorkspaceID, identity.ConsumerBindingID) (domain.ConsumerBinding, error)
	ListConsumerBindings(context.Context, identity.WorkspaceID) ([]domain.ConsumerBinding, error)
	UpdateConsumerBinding(context.Context, domain.ConsumerBinding, int) (domain.ConsumerBinding, error)
	CurrentReleaseSnapshot(context.Context, identity.WorkspaceID) (domain.ReleaseSnapshot, error)
	ReleaseSnapshot(context.Context, identity.WorkspaceID, identity.ReleaseID) (domain.ReleaseSnapshot, error)
	RecordResolution(context.Context, ResolutionRecord) error
	GetSemanticQuery(context.Context, identity.WorkspaceID, identity.SemanticQueryID) (ResolutionResult, error)
	GetResolutionByIdempotency(context.Context, identity.WorkspaceID, string, string) (ResolutionResult, error)
	GetResolvedSemanticPlan(context.Context, identity.WorkspaceID, identity.ResolvedSemanticPlanID) (domain.ResolvedSemanticPlan, error)
}

type ResolutionRecord struct {
	Query      domain.SemanticQuery
	Plan       *domain.ResolvedSemanticPlan
	Refusal    *domain.Refusal
	Validation domain.ValidationRun
}

type ResolutionResult struct {
	Query       domain.SemanticQuery
	Plan        *domain.ResolvedSemanticPlan
	Refusal     *domain.Refusal
	Validation  domain.ValidationRun
	Definitions []DefinitionSummary
}

// DefinitionSummary is the released, human-readable part of a resolved asset.
// It is derived from the same immutable snapshot as the plan, never from the
// mutable catalog pointer.
type DefinitionSummary struct {
	AssetID       identity.AssetID    `json:"assetId"`
	RevisionID    identity.RevisionID `json:"revisionId"`
	Address       string              `json:"address"`
	AssetType     string              `json:"assetType"`
	Name          string              `json:"name"`
	Definition    string              `json:"definition"`
	ContentDigest string              `json:"contentDigest"`
}

type Service struct {
	repository Repository
	authorizer authorizationapp.Evaluator
	clock      Clock
}

func NewService(repository Repository, authorizer authorizationapp.Evaluator, clock Clock) *Service {
	if repository == nil || clock == nil {
		panic("distribution repository and clock are required")
	}
	return &Service{repository: repository, authorizer: authorizer, clock: clock}
}

type CreateConsumerRequest struct {
	WorkspaceID       identity.WorkspaceID
	StableKey         string
	Name              string
	Kind              string
	OwnerPrincipalRef string
	Metadata          json.RawMessage
	PrincipalRef      string
	TraceID           string
}

func (service *Service) CreateConsumer(ctx context.Context, request CreateConsumerRequest) (domain.Consumer, error) {
	decision, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionBindingManage,
		authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()}, request.PrincipalRef, request.TraceID)
	if err != nil {
		return domain.Consumer{}, err
	}
	id, err := identity.NewConsumerID()
	if err != nil {
		return domain.Consumer{}, err
	}
	owner := strings.TrimSpace(request.OwnerPrincipalRef)
	if owner == "" && !decision.PrincipalID.IsZero() {
		owner = decision.PrincipalID.String()
	}
	now := service.clock.Now().UTC()
	consumer := domain.Consumer{ID: id, WorkspaceID: request.WorkspaceID, StableKey: strings.TrimSpace(request.StableKey),
		Name: strings.TrimSpace(request.Name), Kind: strings.TrimSpace(request.Kind), Status: domain.ConsumerActive,
		OwnerPrincipalRef: owner, Metadata: normalizeObject(request.Metadata), CreatedAt: now, UpdatedAt: now}
	if err := consumer.Validate(); err != nil {
		return domain.Consumer{}, err
	}
	return service.repository.CreateConsumer(ctx, consumer)
}

func (service *Service) ListConsumers(ctx context.Context, workspace identity.WorkspaceID, principalRef, traceID string) ([]domain.Consumer, error) {
	if _, err := service.authorize(ctx, workspace, authorization.ActionBindingRead,
		authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}, principalRef, traceID); err != nil {
		return nil, err
	}
	return service.repository.ListConsumers(ctx, workspace)
}

func (service *Service) GetConsumer(ctx context.Context, workspace identity.WorkspaceID, consumer identity.ConsumerID,
	principalRef, traceID string) (domain.Consumer, error) {
	if _, err := service.authorize(ctx, workspace, authorization.ActionBindingRead,
		authorization.Resource{Type: authorization.ScopeConsumer, ID: consumer.UUID()}, principalRef, traceID); err != nil {
		return domain.Consumer{}, err
	}
	return service.repository.GetConsumer(ctx, workspace, consumer)
}

type UpdateConsumerRequest struct {
	WorkspaceID  identity.WorkspaceID
	ConsumerID   identity.ConsumerID
	Name         string
	Status       domain.ConsumerStatus
	Metadata     json.RawMessage
	PrincipalRef string
	TraceID      string
}

func (service *Service) UpdateConsumer(ctx context.Context, request UpdateConsumerRequest) (domain.Consumer, error) {
	if _, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionBindingManage,
		authorization.Resource{Type: authorization.ScopeConsumer, ID: request.ConsumerID.UUID()}, request.PrincipalRef, request.TraceID); err != nil {
		return domain.Consumer{}, err
	}
	consumer, err := service.repository.GetConsumer(ctx, request.WorkspaceID, request.ConsumerID)
	if err != nil {
		return domain.Consumer{}, err
	}
	consumer.Name, consumer.Status, consumer.Metadata = strings.TrimSpace(request.Name), request.Status, normalizeObject(request.Metadata)
	consumer.UpdatedAt = service.clock.Now().UTC()
	if err := consumer.Validate(); err != nil {
		return domain.Consumer{}, err
	}
	return service.repository.UpdateConsumer(ctx, consumer)
}

type CreateBindingRequest struct {
	WorkspaceID             identity.WorkspaceID
	ConsumerID              identity.ConsumerID
	Environment             string
	Purpose                 string
	Mode                    domain.BindingMode
	ReleaseID               *identity.ReleaseID
	CompatibilityConstraint json.RawMessage
	ExpiresAt               *time.Time
	PrincipalRef            string
	TraceID                 string
}

func (service *Service) CreateBinding(ctx context.Context, request CreateBindingRequest) (domain.ConsumerBinding, error) {
	if _, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionBindingManage,
		authorization.Resource{Type: authorization.ScopeConsumer, ID: request.ConsumerID.UUID()}, request.PrincipalRef, request.TraceID); err != nil {
		return domain.ConsumerBinding{}, err
	}
	consumer, err := service.repository.GetConsumer(ctx, request.WorkspaceID, request.ConsumerID)
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	if consumer.Status != domain.ConsumerActive {
		return domain.ConsumerBinding{}, domain.ErrConflict
	}
	if request.Mode == domain.BindingPinned {
		if request.ReleaseID == nil {
			return domain.ConsumerBinding{}, domain.ErrInvalidArgument
		}
		if _, err := service.repository.ReleaseSnapshot(ctx, request.WorkspaceID, *request.ReleaseID); err != nil {
			return domain.ConsumerBinding{}, err
		}
	}
	id, err := identity.NewConsumerBindingID()
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	now := service.clock.Now().UTC()
	binding := domain.ConsumerBinding{ID: id, WorkspaceID: request.WorkspaceID, ConsumerID: request.ConsumerID,
		Environment: strings.TrimSpace(request.Environment), Purpose: strings.TrimSpace(request.Purpose), Mode: request.Mode,
		ReleaseID: request.ReleaseID, CompatibilityConstraint: normalizeObject(request.CompatibilityConstraint),
		ExpiresAt: request.ExpiresAt, Status: domain.BindingActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := binding.Validate(); err != nil {
		return domain.ConsumerBinding{}, err
	}
	return service.repository.CreateConsumerBinding(ctx, binding)
}

func (service *Service) ListBindings(ctx context.Context, workspace identity.WorkspaceID, principalRef, traceID string) ([]domain.ConsumerBinding, error) {
	if _, err := service.authorize(ctx, workspace, authorization.ActionBindingRead,
		authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}, principalRef, traceID); err != nil {
		return nil, err
	}
	return service.repository.ListConsumerBindings(ctx, workspace)
}

func (service *Service) GetBinding(ctx context.Context, workspace identity.WorkspaceID, binding identity.ConsumerBindingID,
	principalRef, traceID string) (domain.ConsumerBinding, error) {
	if _, err := service.authorize(ctx, workspace, authorization.ActionBindingRead,
		authorization.Resource{Type: authorization.ScopeConsumer, ID: binding.UUID()}, principalRef, traceID); err != nil {
		return domain.ConsumerBinding{}, err
	}
	return service.repository.GetConsumerBinding(ctx, workspace, binding)
}

type UpdateBindingRequest struct {
	WorkspaceID             identity.WorkspaceID
	BindingID               identity.ConsumerBindingID
	ExpectedVersion         int
	Purpose                 string
	Mode                    domain.BindingMode
	ReleaseID               *identity.ReleaseID
	CompatibilityConstraint json.RawMessage
	ExpiresAt               *time.Time
	Status                  domain.BindingStatus
	PrincipalRef            string
	TraceID                 string
}

func (service *Service) UpdateBinding(ctx context.Context, request UpdateBindingRequest) (domain.ConsumerBinding, error) {
	if _, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionBindingManage,
		authorization.Resource{Type: authorization.ScopeConsumer, ID: request.BindingID.UUID()}, request.PrincipalRef, request.TraceID); err != nil {
		return domain.ConsumerBinding{}, err
	}
	binding, err := service.repository.GetConsumerBinding(ctx, request.WorkspaceID, request.BindingID)
	if err != nil {
		return domain.ConsumerBinding{}, err
	}
	if request.Mode == domain.BindingPinned {
		if request.ReleaseID == nil {
			return domain.ConsumerBinding{}, domain.ErrInvalidArgument
		}
		if _, err := service.repository.ReleaseSnapshot(ctx, request.WorkspaceID, *request.ReleaseID); err != nil {
			return domain.ConsumerBinding{}, err
		}
	}
	binding.Purpose, binding.Mode, binding.ReleaseID = strings.TrimSpace(request.Purpose), request.Mode, request.ReleaseID
	binding.CompatibilityConstraint, binding.ExpiresAt, binding.Status = normalizeObject(request.CompatibilityConstraint), request.ExpiresAt, request.Status
	binding.UpdatedAt = service.clock.Now().UTC()
	if err := binding.Validate(); err != nil {
		return domain.ConsumerBinding{}, err
	}
	return service.repository.UpdateConsumerBinding(ctx, binding, request.ExpectedVersion)
}

type ResolveRequest struct {
	WorkspaceID    identity.WorkspaceID
	Input          domain.SemanticQueryInput
	Channel        string
	IdempotencyKey string
	PrincipalRef   string
	TraceID        string
}

func (service *Service) Resolve(ctx context.Context, request ResolveRequest) (ResolutionResult, error) {
	if limit, ok := authorizationapp.CredentialFromContext(ctx); ok {
		if request.WorkspaceID != limit.WorkspaceID || request.PrincipalRef != limit.PrincipalID.String() || request.Input.Context.Mode == domain.ResolutionExplicit || request.Input.Context.BindingID != nil && *request.Input.Context.BindingID != limit.BindingID {
			return ResolutionResult{}, credentialDenied()
		}
		request.Input.Context = domain.ResolutionContext{Mode: domain.ResolutionBinding, BindingID: &limit.BindingID}
	}
	request.Channel, request.IdempotencyKey, request.PrincipalRef = strings.TrimSpace(request.Channel),
		strings.TrimSpace(request.IdempotencyKey), strings.TrimSpace(request.PrincipalRef)
	if request.WorkspaceID.IsZero() || request.Channel == "" || len(request.Channel) > 40 ||
		request.IdempotencyKey == "" || len(request.IdempotencyKey) > 160 {
		return ResolutionResult{}, domain.ErrInvalidArgument
	}
	canonical, queryDigest, err := request.Input.Canonical()
	if err != nil {
		return ResolutionResult{}, err
	}
	decision, err := service.authorize(ctx, request.WorkspaceID, authorization.ActionSemanticResolve,
		authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()}, request.PrincipalRef, request.TraceID)
	if err != nil {
		return ResolutionResult{}, err
	}
	actor := request.PrincipalRef
	if !decision.PrincipalID.IsZero() {
		actor = decision.PrincipalID.String()
	}
	if prior, err := service.repository.GetResolutionByIdempotency(ctx, request.WorkspaceID, actor, request.IdempotencyKey); err == nil {
		if prior.Query.RequestDigest != queryDigest {
			return ResolutionResult{}, domain.ErrConflict
		}
		if err := service.checkStoredAccess(ctx, request.WorkspaceID, prior, request.PrincipalRef, request.TraceID); err != nil {
			return ResolutionResult{}, err
		}
		if err := service.restoreDefinitions(ctx, request.WorkspaceID, &prior); err != nil {
			return ResolutionResult{}, err
		}
		return prior, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return ResolutionResult{}, err
	}

	queryID, err := identity.NewSemanticQueryID()
	if err != nil {
		return ResolutionResult{}, err
	}
	now := service.clock.Now().UTC()
	query := domain.SemanticQuery{ID: queryID, WorkspaceID: request.WorkspaceID, PrincipalRef: actor,
		SchemaVersion: domain.QuerySchemaVersion, ResolverVersion: domain.ResolverVersion,
		CanonicalRequest: canonical, RequestDigest: queryDigest, Channel: request.Channel, TraceID: request.TraceID,
		IdempotencyKey: request.IdempotencyKey, CreatedAt: now, FinalizedAt: now}

	snapshot, consumer, binding, refusal, err := service.selectSnapshot(ctx, request, decision, now)
	if err != nil {
		return ResolutionResult{}, err
	}
	if consumer != nil {
		query.ConsumerID = &consumer.ID
	}
	if binding != nil {
		query.BindingID = &binding.ID
	}
	if refusal != nil {
		return service.persistRefusal(ctx, query, *refusal, now)
	}
	query.SelectedReleaseID = snapshot.ReleaseID
	if limit, ok := authorizationapp.CredentialFromContext(ctx); ok && limit.ScopeType == authorization.ScopeRelease && limit.ScopeID != snapshot.ReleaseID.String() {
		return ResolutionResult{}, credentialDenied()
	}

	selected := make([]domain.ReleasedAsset, 0, len(request.Input.Measures)+len(request.Input.Dimensions)+len(request.Input.Filters)+len(request.Input.Order)+1)
	selectors := make([]struct {
		selector domain.Selector
		typeName string
	}, 0)
	for _, selector := range request.Input.Measures {
		selectors = append(selectors, struct {
			selector domain.Selector
			typeName string
		}{selector, "measure"})
	}
	for _, selector := range request.Input.Dimensions {
		selectors = append(selectors, struct {
			selector domain.Selector
			typeName string
		}{selector, "dimension"})
	}
	for _, filter := range request.Input.Filters {
		selectors = append(selectors, struct {
			selector domain.Selector
			typeName string
		}{filter.Selector, "any"})
	}
	if request.Input.TimeRange != nil {
		selectors = append(selectors, struct {
			selector domain.Selector
			typeName string
		}{request.Input.TimeRange.Selector, "dimension"})
	}
	for _, order := range request.Input.Order {
		selectors = append(selectors, struct {
			selector domain.Selector
			typeName string
		}{order.Selector, "any"})
	}
	for _, item := range selectors {
		asset, matchRefusal, resolveErr := service.resolveSelector(ctx, request, item.selector, item.typeName, snapshot.Assets)
		if resolveErr != nil {
			return ResolutionResult{}, resolveErr
		}
		if matchRefusal != nil {
			return service.persistRefusal(ctx, query, *matchRefusal, now)
		}
		selected = append(selected, asset)
	}
	planID, err := identity.NewResolvedSemanticPlanID()
	if err != nil {
		return ResolutionResult{}, err
	}
	plan, planRefusal, err := domain.BuildPlan(request.Input, snapshot, queryID, planID, selected, now)
	if err != nil {
		return ResolutionResult{}, err
	}
	if planRefusal != nil {
		seen := map[identity.AssetID]bool{}
		for _, asset := range selected {
			if !seen[asset.AssetID] {
				planRefusal.CandidateIDs = append(planRefusal.CandidateIDs, asset.AssetID)
				seen[asset.AssetID] = true
			}
		}
		return service.persistRefusal(ctx, query, *planRefusal, now)
	}
	query.Outcome = "resolved"
	validationID, err := identity.NewQueryValidationRunID()
	if err != nil {
		return ResolutionResult{}, err
	}
	validation := domain.ValidationRun{ID: validationID, QueryID: queryID, PlanID: &planID,
		Validator: "semantic-plan", ValidatorVersion: domain.ResolverVersion, InputDigest: plan.PlanDigest,
		Status: "passed", Results: []domain.ValidationResult{}, CreatedAt: now, CompletedAt: now}
	record := ResolutionRecord{Query: query, Plan: &plan, Validation: validation}
	if err := service.repository.RecordResolution(ctx, record); err != nil {
		return ResolutionResult{}, err
	}
	return ResolutionResult{Query: query, Plan: &plan, Validation: validation, Definitions: definitionSummaries(selected)}, nil
}

func definitionSummaries(assets []domain.ReleasedAsset) []DefinitionSummary {
	result := make([]DefinitionSummary, 0, len(assets))
	seen := make(map[identity.AssetID]bool, len(assets))
	for _, asset := range assets {
		if seen[asset.AssetID] {
			continue
		}
		seen[asset.AssetID] = true
		var content struct {
			Name       string `json:"name"`
			Definition string `json:"definition"`
		}
		_ = json.Unmarshal(asset.Content, &content)
		name := strings.TrimSpace(content.Name)
		if name == "" {
			name = strings.TrimSpace(asset.Name)
		}
		result = append(result, DefinitionSummary{
			AssetID: asset.AssetID, RevisionID: asset.RevisionID, Address: asset.Address,
			AssetType: string(asset.AssetType), Name: name,
			Definition: strings.TrimSpace(content.Definition), ContentDigest: asset.ContentDigest,
		})
	}
	return result
}

func (service *Service) selectSnapshot(ctx context.Context, request ResolveRequest, decision authorization.Decision, now time.Time) (
	domain.ReleaseSnapshot, *domain.Consumer, *domain.ConsumerBinding, *domain.Refusal, error,
) {
	switch request.Input.Context.Mode {
	case domain.ResolutionCurrent:
		snapshot, err := service.repository.CurrentReleaseSnapshot(ctx, request.WorkspaceID)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ReleaseSnapshot{}, nil, nil, &domain.Refusal{Code: domain.RefusalNoRelease, Clarification: "当前工作区还没有已发布版本。"}, nil
		}
		return snapshot, nil, nil, nil, err
	case domain.ResolutionExplicit:
		snapshot, err := service.repository.ReleaseSnapshot(ctx, request.WorkspaceID, *request.Input.Context.ReleaseID)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ReleaseSnapshot{}, nil, nil, &domain.Refusal{Code: domain.RefusalNoRelease, Clarification: "指定发布版本不存在或不可访问。"}, nil
		}
		return snapshot, nil, nil, nil, err
	case domain.ResolutionBinding:
		binding, err := service.repository.GetConsumerBinding(ctx, request.WorkspaceID, *request.Input.Context.BindingID)
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ReleaseSnapshot{}, nil, nil, &domain.Refusal{Code: domain.RefusalBindingInactive, Clarification: "消费绑定不存在或不可用。"}, nil
		}
		if err != nil {
			return domain.ReleaseSnapshot{}, nil, nil, nil, err
		}
		consumer, err := service.repository.GetConsumer(ctx, request.WorkspaceID, binding.ConsumerID)
		if err != nil {
			return domain.ReleaseSnapshot{}, nil, &binding, nil, err
		}
		if consumer.Status != domain.ConsumerActive {
			return domain.ReleaseSnapshot{}, &consumer, &binding, &domain.Refusal{Code: domain.RefusalConsumerInactive, Clarification: "消费方未处于启用状态。"}, nil
		}
		if binding.Status != domain.BindingActive {
			return domain.ReleaseSnapshot{}, &consumer, &binding, &domain.Refusal{Code: domain.RefusalBindingInactive, Clarification: "消费绑定未处于启用状态。"}, nil
		}
		if binding.ExpiresAt != nil && !binding.ExpiresAt.After(now) {
			return domain.ReleaseSnapshot{}, &consumer, &binding, &domain.Refusal{Code: domain.RefusalBindingExpired, Clarification: "消费绑定已过期。"}, nil
		}
		machine, isMachine := authorizationapp.CredentialFromContext(ctx)
		if isMachine && (machine.ConsumerID != consumer.ID || machine.BindingID != binding.ID || machine.PrincipalID != decision.PrincipalID) {
			return domain.ReleaseSnapshot{}, nil, nil, nil, credentialDenied()
		}
		if !isMachine && decision.RoleID != "workspace_admin" && consumer.OwnerPrincipalRef != decision.PrincipalID.String() {
			return domain.ReleaseSnapshot{}, &consumer, &binding, &domain.Refusal{Code: domain.RefusalUnauthorizedConsumer, Clarification: "当前身份不能使用该消费绑定。"}, nil
		}
		var snapshot domain.ReleaseSnapshot
		if binding.Mode == domain.BindingPinned {
			snapshot, err = service.repository.ReleaseSnapshot(ctx, request.WorkspaceID, *binding.ReleaseID)
		} else {
			snapshot, err = service.repository.CurrentReleaseSnapshot(ctx, request.WorkspaceID)
		}
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ReleaseSnapshot{}, &consumer, &binding, &domain.Refusal{Code: domain.RefusalStaleReleaseBinding, Clarification: "绑定引用的发布版本不可用。"}, nil
		}
		if err != nil {
			return domain.ReleaseSnapshot{}, nil, nil, nil, err
		}
		var constraint struct {
			ManifestDigest string `json:"manifestDigest"`
		}
		_ = json.Unmarshal(binding.CompatibilityConstraint, &constraint)
		if constraint.ManifestDigest != "" && constraint.ManifestDigest != snapshot.ManifestDigest {
			return domain.ReleaseSnapshot{}, &consumer, &binding, &domain.Refusal{Code: domain.RefusalStaleReleaseBinding, Clarification: "发布版本不满足绑定的兼容性约束。"}, nil
		}
		return snapshot, &consumer, &binding, nil, nil
	default:
		return domain.ReleaseSnapshot{}, nil, nil, nil, domain.ErrInvalidArgument
	}
}

func (service *Service) resolveSelector(ctx context.Context, request ResolveRequest, selector domain.Selector,
	expected string, assets []domain.ReleasedAsset) (domain.ReleasedAsset, *domain.Refusal, error) {
	remaining := append([]domain.ReleasedAsset(nil), assets...)
	for len(remaining) > 0 {
		matched, refusal := domain.MatchSelector(selector, expected, remaining)
		if refusal != nil && refusal.Code == domain.RefusalNoMatch {
			if selector.AssetID != nil || strings.TrimSpace(selector.Address) != "" {
				return domain.ReleasedAsset{}, &domain.Refusal{Code: domain.RefusalUnauthorizedAsset,
					Clarification: "指定资产不存在、未发布或当前身份不可读取。"}, nil
			}
			return domain.ReleasedAsset{}, refusal, nil
		}
		if refusal != nil && refusal.Code == domain.RefusalAmbiguousMatch {
			authorized := make([]domain.ReleasedAsset, 0, len(matched.Candidates))
			for _, candidate := range matched.Candidates {
				allowed, err := service.assetAllowed(ctx, request, candidate)
				if err != nil {
					return domain.ReleasedAsset{}, nil, err
				}
				if allowed {
					authorized = append(authorized, candidate)
				}
			}
			if len(authorized) == 1 {
				return authorized[0], nil, nil
			}
			if len(authorized) > 1 {
				refusal.CandidateIDs = make([]identity.AssetID, 0, len(authorized))
				for _, candidate := range authorized {
					refusal.CandidateIDs = append(refusal.CandidateIDs, candidate.AssetID)
				}
				return domain.ReleasedAsset{}, refusal, nil
			}
			remaining = removeAssets(remaining, matched.Candidates)
			continue
		}
		allowed, err := service.assetAllowed(ctx, request, matched.Asset)
		if err != nil {
			return domain.ReleasedAsset{}, nil, err
		}
		if allowed {
			return matched.Asset, nil, nil
		}
		if selector.AssetID != nil || strings.TrimSpace(selector.Address) != "" {
			return domain.ReleasedAsset{}, &domain.Refusal{Code: domain.RefusalUnauthorizedAsset, Clarification: "当前身份不能读取指定资产。"}, nil
		}
		remaining = removeAssets(remaining, []domain.ReleasedAsset{matched.Asset})
	}
	return domain.ReleasedAsset{}, &domain.Refusal{Code: domain.RefusalNoMatch, Clarification: "没有可读取的匹配资产。"}, nil
}

func (service *Service) assetAllowed(ctx context.Context, request ResolveRequest, asset domain.ReleasedAsset) (bool, error) {
	decision, err := service.authorizeDecision(ctx, request.WorkspaceID, authorization.ActionAssetRead,
		authorization.Resource{Type: authorization.ScopeAsset, ID: asset.AssetID.UUID()}, request.PrincipalRef, request.TraceID)
	if err != nil {
		return false, err
	}
	return decision.Allowed, nil
}

func (service *Service) persistRefusal(ctx context.Context, query domain.SemanticQuery, refusal domain.Refusal, now time.Time) (ResolutionResult, error) {
	query.Outcome = "refused"
	refusal.QueryID, refusal.CreatedAt = query.ID, now
	if len(refusal.Details) == 0 {
		refusal.Details = json.RawMessage(`{}`)
	}
	validationID, err := identity.NewQueryValidationRunID()
	if err != nil {
		return ResolutionResult{}, err
	}
	validation := domain.ValidationRun{ID: validationID, QueryID: query.ID, Validator: "semantic-plan",
		ValidatorVersion: domain.ResolverVersion, InputDigest: query.RequestDigest, Status: "failed",
		Results:   []domain.ValidationResult{{Severity: "blocker", Code: string(refusal.Code), Message: refusal.Clarification, Details: json.RawMessage(`{}`)}},
		CreatedAt: now, CompletedAt: now}
	record := ResolutionRecord{Query: query, Refusal: &refusal, Validation: validation}
	if err := service.repository.RecordResolution(ctx, record); err != nil {
		return ResolutionResult{}, err
	}
	return ResolutionResult{Query: query, Refusal: &refusal, Validation: validation}, nil
}

func (service *Service) GetQuery(ctx context.Context, workspace identity.WorkspaceID, query identity.SemanticQueryID,
	principalRef, traceID string) (ResolutionResult, error) {
	if _, err := service.authorize(ctx, workspace, authorization.ActionSemanticResolve,
		authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}, principalRef, traceID); err != nil {
		return ResolutionResult{}, err
	}
	result, err := service.repository.GetSemanticQuery(ctx, workspace, query)
	if err != nil {
		return ResolutionResult{}, err
	}
	if err := service.checkStoredAccess(ctx, workspace, result, principalRef, traceID); err != nil {
		return ResolutionResult{}, err
	}
	if err := service.restoreDefinitions(ctx, workspace, &result); err != nil {
		return ResolutionResult{}, err
	}
	return result, nil
}

func (service *Service) restoreDefinitions(ctx context.Context, workspace identity.WorkspaceID, result *ResolutionResult) error {
	if result.Plan == nil {
		return nil
	}
	snapshot, err := service.repository.ReleaseSnapshot(ctx, workspace, result.Plan.ReleaseID)
	if err != nil {
		return err
	}
	assets := []domain.ReleasedAsset{}
	for _, selected := range result.Plan.Assets {
		for _, asset := range snapshot.Assets {
			if asset.AssetID == selected.AssetID {
				assets = append(assets, asset)
				break
			}
		}
	}
	result.Definitions = definitionSummaries(assets)
	return nil
}

func (service *Service) GetPlan(ctx context.Context, workspace identity.WorkspaceID, plan identity.ResolvedSemanticPlanID,
	principalRef, traceID string) (domain.ResolvedSemanticPlan, error) {
	if _, err := service.authorize(ctx, workspace, authorization.ActionSemanticResolve,
		authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}, principalRef, traceID); err != nil {
		return domain.ResolvedSemanticPlan{}, err
	}
	result, err := service.repository.GetResolvedSemanticPlan(ctx, workspace, plan)
	if err != nil {
		return domain.ResolvedSemanticPlan{}, err
	}
	if _, err := service.GetQuery(ctx, workspace, result.QueryID, principalRef, traceID); err != nil {
		return domain.ResolvedSemanticPlan{}, err
	}
	return result, nil
}

func credentialDenied() error {
	return &authorization.DenialError{Decision: authorization.Decision{ReasonCode: authorization.ReasonNoMatchingGrant}}
}

// Saved plans and refusals are immutable facts, not cached authorization.
func (service *Service) checkStoredAccess(ctx context.Context, workspace identity.WorkspaceID, result ResolutionResult, principal, trace string) error {
	return service.checkStoredAccessWithFreshness(ctx, workspace, result, principal, trace, true)
}

func (service *Service) checkStoredAccessWithFreshness(ctx context.Context, workspace identity.WorkspaceID, result ResolutionResult, principal, trace string, fresh bool) error {
	if result.Query.WorkspaceID != workspace {
		return credentialDenied()
	}
	if limit, ok := authorizationapp.CredentialFromContext(ctx); ok {
		if result.Query.PrincipalRef != limit.PrincipalID.String() || result.Query.ConsumerID == nil || *result.Query.ConsumerID != limit.ConsumerID || result.Query.BindingID == nil || *result.Query.BindingID != limit.BindingID {
			return credentialDenied()
		}
		if limit.ScopeType == authorization.ScopeRelease && result.Query.SelectedReleaseID.String() != limit.ScopeID {
			return credentialDenied()
		}
	}
	if result.Query.BindingID != nil {
		decision, err := service.authorize(ctx, workspace, authorization.ActionSemanticResolve, authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}, principal, trace)
		if err != nil {
			return err
		}
		if fresh {
			request := ResolveRequest{WorkspaceID: workspace, PrincipalRef: principal, TraceID: trace, Input: domain.SemanticQueryInput{Context: domain.ResolutionContext{Mode: domain.ResolutionBinding, BindingID: result.Query.BindingID}}}
			snapshot, _, _, refusal, err := service.selectSnapshot(ctx, request, decision, service.clock.Now().UTC())
			if err != nil {
				return err
			}
			if refusal != nil || (!result.Query.SelectedReleaseID.IsZero() && snapshot.ReleaseID != result.Query.SelectedReleaseID) {
				return credentialDenied()
			}
		} else {
			// Cancelling an existing run needs current ownership and grants, not
			// a current release capable of starting that old plan again.
			binding, err := service.repository.GetConsumerBinding(ctx, workspace, *result.Query.BindingID)
			if err != nil {
				return err
			}
			consumer, err := service.repository.GetConsumer(ctx, workspace, binding.ConsumerID)
			if err != nil {
				return err
			}
			if binding.Status != domain.BindingActive || consumer.Status != domain.ConsumerActive || (binding.ExpiresAt != nil && !binding.ExpiresAt.After(service.clock.Now())) || result.Query.ConsumerID == nil || *result.Query.ConsumerID != consumer.ID {
				return credentialDenied()
			}
			machine, isMachine := authorizationapp.CredentialFromContext(ctx)
			if isMachine && (machine.ConsumerID != consumer.ID || machine.BindingID != binding.ID || machine.PrincipalID != decision.PrincipalID) {
				return credentialDenied()
			}
			if !isMachine && decision.RoleID != "workspace_admin" && consumer.OwnerPrincipalRef != decision.PrincipalID.String() {
				return credentialDenied()
			}
		}
	}
	assets := []identity.AssetID{}
	if result.Refusal != nil && len(result.Refusal.CandidateIDs) == 0 && !result.Query.SelectedReleaseID.IsZero() {
		switch result.Refusal.Code {
		case domain.RefusalNoMatch, domain.RefusalUnauthorizedAsset:
		default:
			return credentialDenied()
		}
	}
	if result.Plan != nil {
		for _, asset := range result.Plan.Assets {
			assets = append(assets, asset.AssetID)
		}
	}
	if result.Refusal != nil {
		assets = append(assets, result.Refusal.CandidateIDs...)
	}
	for _, asset := range assets {
		if _, err := service.authorize(ctx, workspace, authorization.ActionAssetRead, authorization.Resource{Type: authorization.ScopeAsset, ID: asset.UUID()}, principal, trace); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) authorize(ctx context.Context, workspace identity.WorkspaceID, action authorization.Action,
	resource authorization.Resource, principalRef, traceID string) (authorization.Decision, error) {
	decision, err := service.authorizeDecision(ctx, workspace, action, resource, principalRef, traceID)
	if err != nil {
		return authorization.Decision{}, err
	}
	if !decision.Allowed {
		return authorization.Decision{}, &authorization.DenialError{Decision: decision}
	}
	return decision, nil
}

func (service *Service) authorizeDecision(ctx context.Context, workspace identity.WorkspaceID, action authorization.Action,
	resource authorization.Resource, principalRef, traceID string) (authorization.Decision, error) {
	if service.authorizer == nil {
		if limit, ok := authorizationapp.CredentialFromContext(ctx); ok && !limit.Allows(authorizationapp.EvaluationRequest{WorkspaceID: workspace, PrincipalRef: principalRef, Action: action, Resource: resource}) {
			return authorization.Decision{ReasonCode: authorization.ReasonNoMatchingGrant}, nil
		}
		return authorization.Decision{Allowed: true, Action: action, RoleID: "workspace_admin"}, nil
	}
	return service.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{PrincipalRef: strings.TrimSpace(principalRef),
		WorkspaceID: workspace, Action: action, Resource: resource, TraceID: traceID})
}

func normalizeObject(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return append(json.RawMessage(nil), value...)
}

func removeAssets(items, removed []domain.ReleasedAsset) []domain.ReleasedAsset {
	ids := make(map[string]bool, len(removed))
	for _, item := range removed {
		ids[item.AssetID.String()] = true
	}
	result := make([]domain.ReleasedAsset, 0, len(items))
	for _, item := range items {
		if !ids[item.AssetID.String()] {
			result = append(result, item)
		}
	}
	return result
}
