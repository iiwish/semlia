package distribution_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	"github.com/iiwish/semlia/internal/domain/authorization"
	distribution "github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

var serviceClock = distributionapp.ClockFunc(func() time.Time {
	return time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
})

func TestResolvePersistsReleaseAndBindingRefusals(t *testing.T) {
	workspace := serviceID(t, identity.NewWorkspaceID)
	release := serviceID(t, identity.NewReleaseID)
	consumerID := serviceID(t, identity.NewConsumerID)
	bindingID := serviceID(t, identity.NewConsumerBindingID)
	principal := serviceID(t, identity.NewPrincipalID)
	expired := serviceClock.Now().Add(-time.Minute)
	inactiveConsumer := activeConsumer(workspace, consumerID, principal.String())
	inactiveConsumer.Status = distribution.ConsumerSuspended
	inactiveBinding := activeBinding(workspace, bindingID, consumerID, release, nil)
	inactiveBinding.Status = distribution.BindingSuspended
	staleBinding := activeBinding(workspace, bindingID, consumerID, release, nil)
	staleBinding.CompatibilityConstraint = json.RawMessage(`{"manifestDigest":"sha256:expected"}`)
	staleSnapshot := distribution.ReleaseSnapshot{WorkspaceID: workspace, ReleaseID: release, ManifestDigest: "sha256:actual"}

	tests := []struct {
		name     string
		repo     *fakeDistributionRepository
		auth     *fakeDistributionAuthorizer
		context  distribution.ResolutionContext
		wantCode distribution.RefusalCode
	}{
		{"no release", &fakeDistributionRepository{}, &fakeDistributionAuthorizer{principal: principal, role: "workspace_admin"},
			distribution.ResolutionContext{Mode: distribution.ResolutionCurrent}, distribution.RefusalNoRelease},
		{"cross-workspace release", &fakeDistributionRepository{}, &fakeDistributionAuthorizer{principal: principal, role: "workspace_admin"},
			distribution.ResolutionContext{Mode: distribution.ResolutionExplicit, ReleaseID: &release}, distribution.RefusalNoRelease},
		{"expired binding", &fakeDistributionRepository{consumer: activeConsumer(workspace, consumerID, principal.String()),
			binding: activeBinding(workspace, bindingID, consumerID, release, &expired)},
			&fakeDistributionAuthorizer{principal: principal, role: "workspace_admin"},
			distribution.ResolutionContext{Mode: distribution.ResolutionBinding, BindingID: &bindingID}, distribution.RefusalBindingExpired},
		{"inactive consumer", &fakeDistributionRepository{consumer: inactiveConsumer,
			binding: activeBinding(workspace, bindingID, consumerID, release, nil)},
			&fakeDistributionAuthorizer{principal: principal, role: "workspace_admin"},
			distribution.ResolutionContext{Mode: distribution.ResolutionBinding, BindingID: &bindingID}, distribution.RefusalConsumerInactive},
		{"inactive binding", &fakeDistributionRepository{consumer: activeConsumer(workspace, consumerID, principal.String()), binding: inactiveBinding},
			&fakeDistributionAuthorizer{principal: principal, role: "workspace_admin"},
			distribution.ResolutionContext{Mode: distribution.ResolutionBinding, BindingID: &bindingID}, distribution.RefusalBindingInactive},
		{"unauthorized consumer", &fakeDistributionRepository{consumer: activeConsumer(workspace, consumerID, "another-owner"),
			binding: activeBinding(workspace, bindingID, consumerID, release, nil)},
			&fakeDistributionAuthorizer{principal: principal, role: "consumer_developer"},
			distribution.ResolutionContext{Mode: distribution.ResolutionBinding, BindingID: &bindingID}, distribution.RefusalUnauthorizedConsumer},
		{"stale binding", &fakeDistributionRepository{consumer: activeConsumer(workspace, consumerID, principal.String()),
			binding: staleBinding, current: staleSnapshot},
			&fakeDistributionAuthorizer{principal: principal, role: "workspace_admin"},
			distribution.ResolutionContext{Mode: distribution.ResolutionBinding, BindingID: &bindingID}, distribution.RefusalStaleReleaseBinding},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := distributionapp.NewService(test.repo, test.auth, serviceClock)
			result, err := service.Resolve(context.Background(), distributionapp.ResolveRequest{WorkspaceID: workspace,
				Input: serviceQuery(test.context), Channel: "api", IdempotencyKey: test.name,
				PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
			if err != nil {
				t.Fatalf("resolve refusal %d: %v", index, err)
			}
			if result.Refusal == nil || result.Refusal.Code != test.wantCode || result.Plan != nil {
				t.Fatalf("result = %+v, want refusal %s", result, test.wantCode)
			}
			if len(test.repo.records) != 1 || test.repo.records[0].Validation.Status != "failed" {
				t.Fatalf("persisted records = %+v", test.repo.records)
			}
		})
	}
}

func TestResolveDoesNotDiscloseUnauthorizedAsset(t *testing.T) {
	workspace := serviceID(t, identity.NewWorkspaceID)
	principal := serviceID(t, identity.NewPrincipalID)
	asset := releasedServiceAsset(t, "finance.secret_revenue")
	release := serviceID(t, identity.NewReleaseID)
	repo := &fakeDistributionRepository{current: distribution.ReleaseSnapshot{WorkspaceID: workspace, ReleaseID: release,
		ManifestDigest: "sha256:release", Assets: []distribution.ReleasedAsset{asset}}}
	auth := &fakeDistributionAuthorizer{principal: principal, role: "consumer_developer", deniedAssets: map[string]bool{asset.AssetID.UUID(): true}}
	service := distributionapp.NewService(repo, auth, serviceClock)

	result, err := service.Resolve(context.Background(), distributionapp.ResolveRequest{WorkspaceID: workspace,
		Input: serviceQuery(distribution.ResolutionContext{Mode: distribution.ResolutionCurrent}), Channel: "api",
		IdempotencyKey: "secret", PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Refusal == nil || result.Refusal.Code != distribution.RefusalUnauthorizedAsset || len(result.Refusal.CandidateIDs) != 0 {
		t.Fatalf("unauthorized result disclosed a candidate: %+v", result)
	}
	missingQuery := serviceQuery(distribution.ResolutionContext{Mode: distribution.ResolutionCurrent})
	missingQuery.Measures[0] = distribution.Selector{Address: "finance.does_not_exist"}
	missing, err := service.Resolve(context.Background(), distributionapp.ResolveRequest{WorkspaceID: workspace,
		Input: missingQuery, Channel: "api", IdempotencyKey: "missing", PrincipalRef: principal.String(),
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err != nil || missing.Refusal == nil || missing.Refusal.Code != result.Refusal.Code || len(missing.Refusal.CandidateIDs) != 0 {
		t.Fatalf("missing and unauthorized exact references are distinguishable: result=%+v err=%v", missing, err)
	}
}

func TestResolveAmbiguityOnlyConsidersAuthorizedCandidates(t *testing.T) {
	workspace := serviceID(t, identity.NewWorkspaceID)
	principal := serviceID(t, identity.NewPrincipalID)
	denied := releasedServiceAsset(t, "finance.net_revenue")
	allowed := releasedServiceAsset(t, "commerce.net_revenue")
	allowed.Content = json.RawMessage(`{"name":"Net revenue","definition":"Revenue after confirmed refunds"}`)
	allowed.ContentDigest = "sha256:released-definition"
	dataset := serviceID(t, identity.NewPhysicalDatasetID)
	repo := &fakeDistributionRepository{current: distribution.ReleaseSnapshot{WorkspaceID: workspace,
		ReleaseID: serviceID(t, identity.NewReleaseID), ManifestDigest: "sha256:release",
		Assets: []distribution.ReleasedAsset{denied, allowed}, Bindings: []distribution.ReleasedPhysicalBinding{
			{ID: serviceID(t, identity.NewPhysicalBindingID), Version: 1, AssetID: denied.AssetID, DatasetID: dataset},
			{ID: serviceID(t, identity.NewPhysicalBindingID), Version: 1, AssetID: allowed.AssetID, DatasetID: dataset},
		}}}
	auth := &fakeDistributionAuthorizer{principal: principal, role: "consumer_developer", deniedAssets: map[string]bool{denied.AssetID.UUID(): true}}
	query := serviceQuery(distribution.ResolutionContext{Mode: distribution.ResolutionCurrent})
	query.Measures[0] = distribution.Selector{Search: "net revenue"}

	result, err := distributionapp.NewService(repo, auth, serviceClock).Resolve(context.Background(), distributionapp.ResolveRequest{
		WorkspaceID: workspace, Input: query, Channel: "agent", IdempotencyKey: "authorized-ambiguity",
		PrincipalRef: principal.String(), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Plan == nil || len(result.Plan.Assets) != 1 || result.Plan.Assets[0].AssetID != allowed.AssetID {
		t.Fatalf("authorized ambiguity result = %+v", result)
	}
	if len(result.Definitions) != 1 || result.Definitions[0].RevisionID != allowed.RevisionID ||
		result.Definitions[0].Definition != "Revenue after confirmed refunds" {
		t.Fatalf("released definitions = %+v", result.Definitions)
	}
}

type fakeDistributionRepository struct {
	consumer distribution.Consumer
	binding  distribution.ConsumerBinding
	current  distribution.ReleaseSnapshot
	records  []distributionapp.ResolutionRecord
}

func (repository *fakeDistributionRepository) CreateConsumer(_ context.Context, value distribution.Consumer) (distribution.Consumer, error) {
	return value, nil
}
func (repository *fakeDistributionRepository) GetConsumer(context.Context, identity.WorkspaceID, identity.ConsumerID) (distribution.Consumer, error) {
	if repository.consumer.ID.IsZero() {
		return distribution.Consumer{}, distribution.ErrNotFound
	}
	return repository.consumer, nil
}
func (repository *fakeDistributionRepository) ListConsumers(context.Context, identity.WorkspaceID) ([]distribution.Consumer, error) {
	return nil, nil
}
func (repository *fakeDistributionRepository) UpdateConsumer(_ context.Context, value distribution.Consumer) (distribution.Consumer, error) {
	return value, nil
}
func (repository *fakeDistributionRepository) CreateConsumerBinding(_ context.Context, value distribution.ConsumerBinding) (distribution.ConsumerBinding, error) {
	return value, nil
}
func (repository *fakeDistributionRepository) GetConsumerBinding(context.Context, identity.WorkspaceID, identity.ConsumerBindingID) (distribution.ConsumerBinding, error) {
	if repository.binding.ID.IsZero() {
		return distribution.ConsumerBinding{}, distribution.ErrNotFound
	}
	return repository.binding, nil
}
func (repository *fakeDistributionRepository) ListConsumerBindings(context.Context, identity.WorkspaceID) ([]distribution.ConsumerBinding, error) {
	return nil, nil
}
func (repository *fakeDistributionRepository) UpdateConsumerBinding(_ context.Context, value distribution.ConsumerBinding, _ int) (distribution.ConsumerBinding, error) {
	return value, nil
}
func (repository *fakeDistributionRepository) CurrentReleaseSnapshot(context.Context, identity.WorkspaceID) (distribution.ReleaseSnapshot, error) {
	if repository.current.ReleaseID.IsZero() {
		return distribution.ReleaseSnapshot{}, distribution.ErrNotFound
	}
	return repository.current, nil
}
func (repository *fakeDistributionRepository) ReleaseSnapshot(_ context.Context, _ identity.WorkspaceID, release identity.ReleaseID) (distribution.ReleaseSnapshot, error) {
	if repository.current.ReleaseID.IsZero() || repository.current.ReleaseID != release {
		return distribution.ReleaseSnapshot{}, distribution.ErrNotFound
	}
	return repository.current, nil
}
func (repository *fakeDistributionRepository) RecordResolution(_ context.Context, record distributionapp.ResolutionRecord) error {
	repository.records = append(repository.records, record)
	return nil
}
func (repository *fakeDistributionRepository) GetSemanticQuery(_ context.Context, _ identity.WorkspaceID, id identity.SemanticQueryID) (distributionapp.ResolutionResult, error) {
	for _, record := range repository.records {
		if record.Query.ID == id {
			return distributionapp.ResolutionResult{Query: record.Query, Plan: record.Plan, Refusal: record.Refusal, Validation: record.Validation}, nil
		}
	}
	return distributionapp.ResolutionResult{}, distribution.ErrNotFound
}
func (repository *fakeDistributionRepository) GetResolutionByIdempotency(_ context.Context, _ identity.WorkspaceID, principal, key string) (distributionapp.ResolutionResult, error) {
	for _, record := range repository.records {
		if record.Query.PrincipalRef == principal && record.Query.IdempotencyKey == key {
			return distributionapp.ResolutionResult{Query: record.Query, Plan: record.Plan, Refusal: record.Refusal, Validation: record.Validation}, nil
		}
	}
	return distributionapp.ResolutionResult{}, distribution.ErrNotFound
}
func (repository *fakeDistributionRepository) GetResolvedSemanticPlan(_ context.Context, _ identity.WorkspaceID, id identity.ResolvedSemanticPlanID) (distribution.ResolvedSemanticPlan, error) {
	for _, record := range repository.records {
		if record.Plan != nil && record.Plan.ID == id {
			return *record.Plan, nil
		}
	}
	return distribution.ResolvedSemanticPlan{}, distribution.ErrNotFound
}

type fakeDistributionAuthorizer struct {
	principal    identity.PrincipalID
	role         string
	deniedAssets map[string]bool
}

func (authorizer *fakeDistributionAuthorizer) Evaluate(_ context.Context, request authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	allowed := true
	if request.Action == authorization.ActionAssetRead && authorizer.deniedAssets[request.Resource.ID] {
		allowed = false
	}
	return authorization.Decision{Allowed: allowed, Action: request.Action, PrincipalID: authorizer.principal,
		RoleID: authorizer.role, ReasonCode: authorization.ReasonRoleGrant}, nil
}

func activeConsumer(workspace identity.WorkspaceID, id identity.ConsumerID, owner string) distribution.Consumer {
	return distribution.Consumer{ID: id, WorkspaceID: workspace, StableKey: "consumer", Name: "Consumer", Kind: "agent",
		Status: distribution.ConsumerActive, OwnerPrincipalRef: owner, Metadata: json.RawMessage(`{}`)}
}

func activeBinding(workspace identity.WorkspaceID, id identity.ConsumerBindingID, consumer identity.ConsumerID,
	release identity.ReleaseID, expires *time.Time,
) distribution.ConsumerBinding {
	return distribution.ConsumerBinding{ID: id, WorkspaceID: workspace, ConsumerID: consumer, Environment: "prod",
		Purpose: "reports", Mode: distribution.BindingPinned, ReleaseID: &release, CompatibilityConstraint: json.RawMessage(`{}`),
		ExpiresAt: expires, Status: distribution.BindingActive, Version: 1}
}

func releasedServiceAsset(t *testing.T, address string) distribution.ReleasedAsset {
	t.Helper()
	return distribution.ReleasedAsset{AssetID: serviceID(t, identity.NewAssetID), RevisionID: serviceID(t, identity.NewRevisionID),
		Address: address, Name: "Net revenue", AssetType: semantic.Metric}
}

func serviceQuery(context distribution.ResolutionContext) distribution.SemanticQueryInput {
	return distribution.SemanticQueryInput{SchemaVersion: distribution.QuerySchemaVersion, Intent: distribution.IntentAggregate,
		Measures: []distribution.Selector{{Address: "finance.secret_revenue"}}, Context: context}
}

func serviceID[T any](t *testing.T, factory func() (T, error)) T {
	t.Helper()
	value, err := factory()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

var _ distributionapp.Repository = (*fakeDistributionRepository)(nil)
var _ authorizationapp.Evaluator = (*fakeDistributionAuthorizer)(nil)

func TestReplayAndKnownIDsRecheckAssetAuthorization(t *testing.T) {
	workspace := serviceID(t, identity.NewWorkspaceID)
	principal := serviceID(t, identity.NewPrincipalID)
	asset := releasedServiceAsset(t, "finance.secret_revenue")
	repo := &fakeDistributionRepository{current: distribution.ReleaseSnapshot{WorkspaceID: workspace, ReleaseID: serviceID(t, identity.NewReleaseID), ManifestDigest: "sha256:release", Assets: []distribution.ReleasedAsset{asset}}}
	auth := &fakeDistributionAuthorizer{principal: principal, role: "workspace_admin", deniedAssets: map[string]bool{}}
	svc := distributionapp.NewService(repo, auth, serviceClock)
	query := serviceQuery(distribution.ResolutionContext{Mode: distribution.ResolutionCurrent})
	query.Intent = distribution.IntentDescribe
	request := distributionapp.ResolveRequest{WorkspaceID: workspace, Input: query, Channel: "api", IdempotencyKey: "recheck", PrincipalRef: principal.String()}
	result, err := svc.Resolve(context.Background(), request)
	if err != nil || result.Plan == nil {
		t.Fatalf("initial resolve: %+v %v", result, err)
	}
	auth.deniedAssets[asset.AssetID.UUID()] = true
	if _, err := svc.Resolve(context.Background(), request); err == nil {
		t.Error("replay bypassed revoked asset grant")
	}
	if _, err := svc.GetQuery(context.Background(), workspace, result.Query.ID, principal.String(), ""); err == nil {
		t.Error("query ID bypassed revoked asset grant")
	}
	if _, err := svc.GetPlan(context.Background(), workspace, result.Plan.ID, principal.String(), ""); err == nil {
		t.Error("plan ID bypassed revoked asset grant")
	}
}

func TestStructuralRefusalRetainsAssetAuthorizationProvenance(t *testing.T) {
	workspace := serviceID(t, identity.NewWorkspaceID)
	principal := serviceID(t, identity.NewPrincipalID)
	asset := releasedServiceAsset(t, "finance.secret_revenue")
	repo := &fakeDistributionRepository{current: distribution.ReleaseSnapshot{WorkspaceID: workspace, ReleaseID: serviceID(t, identity.NewReleaseID), ManifestDigest: "sha256:release", Assets: []distribution.ReleasedAsset{asset}}}
	auth := &fakeDistributionAuthorizer{principal: principal, role: "workspace_admin", deniedAssets: map[string]bool{}}
	svc := distributionapp.NewService(repo, auth, serviceClock)
	request := distributionapp.ResolveRequest{WorkspaceID: workspace, Input: serviceQuery(distribution.ResolutionContext{Mode: distribution.ResolutionCurrent}), Channel: "api", IdempotencyKey: "missing-binding", PrincipalRef: principal.String()}
	result, err := svc.Resolve(context.Background(), request)
	if err != nil || result.Refusal == nil || len(result.Refusal.CandidateIDs) != 1 {
		t.Fatalf("structural refusal missing provenance: %+v %v", result, err)
	}
	if _, err := svc.GetQuery(context.Background(), workspace, result.Query.ID, principal.String(), ""); err != nil {
		t.Fatal("authorized refusal read", err)
	}
	if _, err := svc.Resolve(context.Background(), request); err != nil {
		t.Fatal("authorized refusal replay", err)
	}
	auth.deniedAssets[asset.AssetID.UUID()] = true
	if _, err := svc.GetQuery(context.Background(), workspace, result.Query.ID, principal.String(), ""); err == nil {
		t.Error("structural refusal read bypassed revoked asset")
	}
	if _, err := svc.Resolve(context.Background(), request); err == nil {
		t.Error("structural refusal replay bypassed revoked asset")
	}
	auth.deniedAssets[asset.AssetID.UUID()] = false
	repo.records[0].Refusal.CandidateIDs = nil
	if _, err := svc.GetQuery(context.Background(), workspace, result.Query.ID, principal.String(), ""); err == nil {
		t.Error("legacy unscoped structural refusal did not fail closed")
	}
}
