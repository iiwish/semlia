package governance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/iiwish/semlia/internal/application/jobs"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// CandidateAutoDraftJobType batches AI first-draft generation for the semantic
// candidates of a completed discovery run. Each draft is a regular governed
// production operation; the LLM only fills the suggestion (AI 建模建议) that a
// human must explicitly apply, validate, and have independently reviewed.
const CandidateAutoDraftJobType = "candidate.autodraft"

const autoDraftInstruction = "为该候选实体生成语义资产建议：补全中文名称、业务定义与统计口径；为字段补充语义说明；证据不足的业务规则保持未确认状态，不得编造确认依据。"

type CandidateAutoDraftPayload struct {
	WorkspaceID    identity.WorkspaceID `json:"workspaceId"`
	DiscoveryRunID identity.RunID       `json:"discoveryRunId"`
}

// AutoDraftCandidate is one pending candidate resolved with its verified
// source snapshot so a draft operation can be pinned without extra lookups.
type AutoDraftCandidate struct {
	CandidateID     identity.SemanticCandidateID
	CandidateDigest string
	Title           string
	QualifiedName   string
	CandidateKind   string
	SnapshotID      identity.SourceSnapshotID
	SourceID        identity.SourceConnectionID
	SnapshotDigest  string
	CoverageKeys    []string
}

type AutoDraftRunContext struct {
	SourceRevisionID identity.SourceRevisionID
	RequestedBy      string
	Status           string
}

type AutoDraftRepository interface {
	AutoDraftRunContext(ctx context.Context, workspace identity.WorkspaceID, run identity.RunID) (AutoDraftRunContext, bool, error)
	AutoDraftCandidates(ctx context.Context, workspace identity.WorkspaceID, revision identity.SourceRevisionID, schemas []string, limit int) ([]AutoDraftCandidate, error)
	HasGenerationForOperation(ctx context.Context, workspace identity.WorkspaceID, operation identity.ProductionOperationID) (bool, error)
	GetModelProvider(ctx context.Context, workspace identity.WorkspaceID, provider identity.ModelProviderID) (domain.ModelProvider, error)
	EnqueueJob(ctx context.Context, params jobs.EnqueueJobParams) (jobs.Job, error)
}

// CandidateAutoDraftConfig gates the batch: generation only runs for
// candidates whose qualified name sits in SchemaWhitelist (option B: schema
// allowlist), at most Limit candidates per discovery run.
type CandidateAutoDraftConfig struct {
	SchemaWhitelist []string
	Limit           int
}

func (config CandidateAutoDraftConfig) enabled() bool {
	return len(config.SchemaWhitelist) > 0 && config.Limit > 0
}

type CandidateAutoDraftService struct {
	repo        AutoDraftRepository
	production  *ProductionService
	generation  *ProductionGenerationService
	modelConfig *ModelConfigService
	finalize    *ProductionAutoFinalizeService
	config      CandidateAutoDraftConfig
	grants      []domain.ProductionGenerationGrant
	clock       Clock
	logger      *slog.Logger
}

func NewCandidateAutoDraftService(
	repo AutoDraftRepository,
	production *ProductionService,
	generation *ProductionGenerationService,
	modelConfig *ModelConfigService,
	config CandidateAutoDraftConfig,
	grants []domain.ProductionGenerationGrant,
	clock Clock,
	logger *slog.Logger,
	finalize *ProductionAutoFinalizeService,
) *CandidateAutoDraftService {
	if repo == nil || production == nil || generation == nil || modelConfig == nil || clock == nil {
		panic("candidate auto-draft service requires repository, production, generation, model config, and clock")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CandidateAutoDraftService{repo: repo, production: production, generation: generation,
		modelConfig: modelConfig, finalize: finalize, config: config, grants: grants, clock: clock, logger: logger}
}

// EnqueueForDiscoveryRun schedules one auto-draft batch for a completed
// discovery run. It is a no-op when the batch feature is disabled. Failures
// are logged and swallowed: auto-drafting must never fail the discovery run.
func (service *CandidateAutoDraftService) EnqueueForDiscoveryRun(ctx context.Context, workspace identity.WorkspaceID, run identity.RunID) {
	if !service.config.enabled() {
		return
	}
	jobID, err := identity.NewRunID()
	if err != nil {
		service.logger.Warn("candidate auto-draft enqueue failed", "error_code", "OPERATION_FAILED")
		return
	}
	payload, err := json.Marshal(CandidateAutoDraftPayload{WorkspaceID: workspace, DiscoveryRunID: run})
	if err != nil {
		service.logger.Warn("candidate auto-draft enqueue failed", "error_code", "OPERATION_FAILED")
		return
	}
	params := jobs.EnqueueJobParams{ID: jobID, WorkspaceID: workspace, Type: CandidateAutoDraftJobType,
		Payload: payload, MaxAttempts: 3, AvailableAt: service.clock.Now().UTC(),
		IdempotencyKey: "candidate-autodraft/" + run.String()}
	// The jobs table requires a 32-hex trace ID; batches triggered inside the
	// discovery worker carry no inbound trace, so synthesize one.
	params.TraceID = newAutoDraftTraceID()
	if _, err := service.repo.EnqueueJob(ctx, params); err != nil {
		service.logger.Warn("candidate auto-draft enqueue failed", "run", run.String(), "error_code", "OPERATION_FAILED", "error", err.Error())
		return
	}
	service.logger.Info("candidate auto-draft batch queued", "run", run.String())
}

// WrapDiscoveryHandler runs the normal discovery job and then schedules the
// auto-draft batch for the run it just processed. Enqueue failures are
// deliberately non-fatal.
func (service *CandidateAutoDraftService) WrapDiscoveryHandler(next jobs.Handler) jobs.Handler {
	return func(ctx context.Context, job jobs.Job) error {
		err := next(ctx, job)
		if err != nil {
			return err
		}
		var payload struct {
			RunID string `json:"runId"`
		}
		if json.Unmarshal(job.Payload, &payload) != nil {
			return nil
		}
		runID, parseErr := identity.ParseRunID(payload.RunID)
		if parseErr != nil {
			return nil
		}
		service.EnqueueForDiscoveryRun(ctx, job.WorkspaceID, runID)
		return nil
	}
}

// JobHandler processes one auto-draft batch. Skips (no default LLM, no grant,
// no requester identity) complete the job without error so retries do not
// spin; per-candidate failures are logged and do not abort the batch.
func (service *CandidateAutoDraftService) JobHandler() jobs.Handler {
	return func(ctx context.Context, job jobs.Job) error {
		var payload CandidateAutoDraftPayload
		if json.Unmarshal(job.Payload, &payload) != nil || payload.WorkspaceID.IsZero() || payload.DiscoveryRunID.IsZero() {
			return domain.ErrInvalidArgument
		}
		runContext, found, err := service.repo.AutoDraftRunContext(ctx, payload.WorkspaceID, payload.DiscoveryRunID)
		if err != nil {
			return err
		}
		if !found || runContext.Status != "succeeded" {
			return nil
		}
		requestedBy := strings.TrimSpace(runContext.RequestedBy)
		if requestedBy == "" {
			service.logger.Info("candidate auto-draft skipped: run has no requesting principal", "run", payload.DiscoveryRunID.String())
			return nil
		}
		principal, err := identity.ParsePrincipalID(requestedBy)
		if err != nil {
			service.logger.Info("candidate auto-draft skipped: run requester is not a workspace principal", "run", payload.DiscoveryRunID.String())
			return nil
		}
		setting, err := service.modelConfig.DefaultSetting(ctx, payload.WorkspaceID, domain.ModelKindLLM)
		if err != nil {
			service.logger.Info("candidate auto-draft skipped: workspace has no default LLM model", "workspace", payload.WorkspaceID.String())
			return nil
		}
		provider, err := service.repo.GetModelProvider(ctx, payload.WorkspaceID, setting.ProviderID)
		if err != nil {
			return err
		}
		revision, err := domain.ProductionGenerationModelRevision(setting, provider)
		if err != nil {
			return err
		}
		grant, authorized := matchingAutoDraftGrant(service.grants, payload.WorkspaceID, principal, setting.ID, revision)
		if !authorized {
			service.logger.Info("candidate auto-draft skipped: no generation grant covers the requester and default model",
				"workspace", payload.WorkspaceID.String(), "principal", principal.String())
			return nil
		}
		candidates, err := service.repo.AutoDraftCandidates(ctx, payload.WorkspaceID, runContext.SourceRevisionID,
			service.config.SchemaWhitelist, service.config.Limit)
		if err != nil {
			return err
		}
		drafted, failed := 0, 0
		for _, candidate := range candidates {
			if err := service.autoDraftCandidate(ctx, payload.WorkspaceID, payload.DiscoveryRunID, principal, setting, revision, grant, candidate); err != nil {
				failed++
				service.logger.Warn("candidate auto-draft failed", "run", payload.DiscoveryRunID.String(),
					"candidate", candidate.CandidateID.String(), "error", err.Error())
				continue
			}
			drafted++
		}
		service.logger.Info("candidate auto-draft batch complete", "run", payload.DiscoveryRunID.String(),
			"candidates", len(candidates), "drafted", drafted, "failed", failed)
		return nil
	}
}

func (service *CandidateAutoDraftService) autoDraftCandidate(
	ctx context.Context,
	workspace identity.WorkspaceID,
	run identity.RunID,
	principal identity.PrincipalID,
	setting domain.ModelSetting,
	revision string,
	grant domain.ProductionGenerationGrant,
	candidate AutoDraftCandidate,
) error {
	scope := map[string]any{
		"snapshots": []map[string]any{{
			"sourceId": candidate.SourceID.String(), "snapshotId": candidate.SnapshotID.String(),
			"digest": candidate.SnapshotDigest, "coverageKeys": candidate.CoverageKeys,
		}},
		"candidates": []map[string]any{{
			"candidateId": candidate.CandidateID.String(), "snapshotId": candidate.SnapshotID.String(),
			"digest": candidate.CandidateDigest, "targetKeys": []string{"primary"}, "primaryTargetKey": "primary",
		}},
		"evidence": []any{}, "dependencies": []any{},
	}
	scopeJSON, err := json.Marshal(scope)
	if err != nil {
		return err
	}
	address := autoDraftAssetAddress(candidate.QualifiedName)
	content := map[string]any{
		"address": address, "assetType": autoDraftAssetType(candidate.CandidateKind),
		"displayName": candidate.Title, "definition": nil, "scope": nil,
		"ownerPrincipalId": principal.String(),
	}
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return err
	}
	targets := []domain.TargetDeclaration{{
		LocalKey: "primary", Title: candidate.Title, Kind: domain.TargetKindSemanticAsset,
		Intent: domain.ProductionIntentCreate, IdentityKey: &address, Content: contentJSON,
		EvidenceIDs: []string{}, Changes: []domain.TargetChangeInput{},
	}}
	candidates := []domain.CandidateDeclaration{{
		CandidateID: candidate.CandidateID.String(), CandidateDigest: candidate.CandidateDigest,
		TargetKeys: []string{"primary"}, PrimaryTargetKey: "primary",
	}}
	result, err := service.production.CreateOperation(ctx, CreateOperationCommand{
		WorkspaceID: workspace, PrincipalID: principal,
		IdempotencyKey: "autodraft/" + run.String() + "/" + candidate.CandidateID.String(),
		InputScope:     scopeJSON, Targets: targets, Candidates: candidates,
	})
	if err != nil {
		return fmt.Errorf("create draft operation: %w", err)
	}
	hasGeneration, err := service.repo.HasGenerationForOperation(ctx, workspace, result.OperationID)
	if err != nil {
		return err
	}
	if hasGeneration {
		return nil
	}
	_, version, _, _, _, err := service.production.GetOperationVersion(ctx, workspace, result.OperationID, result.Version)
	if err != nil {
		return fmt.Errorf("read draft operation: %w", err)
	}
	queued, err := service.generation.Generate(ctx, domain.ProductionGenerationRequest{
		WorkspaceID: workspace, PrincipalID: principal, OperationID: result.OperationID,
		ExpectedVersion: result.Version, InputDigest: version.InputDigest,
		ModelSettingID: setting.ID, ModelConfigRevision: revision,
		Instruction: autoDraftInstruction, MaxOutputTokens: grant.MaxOutputTokens,
		MaxCostMicros:  grant.MaxCostMicros,
		TraceID:        newAutoDraftTraceID(),
		IdempotencyKey: "autodraft-gen/" + run.String() + "/" + candidate.CandidateID.String(),
	})
	if err != nil {
		return fmt.Errorf("queue generation: %w", err)
	}
	if service.finalize != nil && !queued.RunID.IsZero() {
		if err := service.finalize.EnqueueForAutodraft(ctx, workspace, principal, result.OperationID, result.Version, queued.RunID); err != nil {
			return fmt.Errorf("queue auto-finalize watch: %w", err)
		}
	}
	return nil
}

func (service *CandidateAutoDraftService) matchingGrant(workspace identity.WorkspaceID, principal identity.PrincipalID, setting identity.ModelSettingID, revision string) (domain.ProductionGenerationGrant, bool) {
	return matchingAutoDraftGrant(service.grants, workspace, principal, setting, revision)
}

// matchingAutoDraftGrant mirrors the claim-time authorization of the
// generation job so batches without a covering grant are skipped up front.
func matchingAutoDraftGrant(grants []domain.ProductionGenerationGrant, workspace identity.WorkspaceID, principal identity.PrincipalID, setting identity.ModelSettingID, revision string) (domain.ProductionGenerationGrant, bool) {
	for _, grant := range grants {
		if grant.WorkspaceID != workspace || grant.PrincipalID != principal || grant.ModelSettingID != setting || grant.ModelConfigRevision != revision {
			continue
		}
		if strings.TrimSpace(grant.PricingBasis) == "" || grant.MaxOutputTokens < 1 || grant.MaxCostMicros < 1 {
			continue
		}
		return grant, true
	}
	return domain.ProductionGenerationGrant{}, false
}

var autoDraftUnsafeAddressChars = regexp.MustCompile(`[^a-zA-Z0-9_.]`)

// autoDraftAssetAddress mirrors the client-side asset address derivation:
// the qualified name is sanitized, and bare names fall under semantic.
func autoDraftAssetAddress(qualifiedName string) string {
	address := autoDraftUnsafeAddressChars.ReplaceAllString(strings.TrimSpace(qualifiedName), "_")
	if !strings.Contains(address, ".") {
		if address == "" {
			address = "semantic.untitled"
		} else {
			address = "semantic." + address
		}
	}
	return address
}

func autoDraftAssetType(kind string) string {
	if kind == "join" {
		return "data_asset"
	}
	return kind
}

// newAutoDraftTraceID synthesizes the 32-hex trace ID the jobs table requires;
// batches triggered inside the discovery worker carry no inbound trace.
func newAutoDraftTraceID() string {
	trace := make([]byte, 16)
	if _, err := rand.Read(trace); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(trace)
}
