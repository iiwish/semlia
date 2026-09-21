package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type ProductionRepository interface {
	PrepareProductionBaseline(context.Context, identity.WorkspaceID, identity.PrincipalID, []domain.TargetDeclaration) (domain.ProductionBaseline, error)
	CheckProductionInput(context.Context, domain.ProductionVersion, []domain.ProductionTarget) error
	CreateProductionOperationTx(
		ctx context.Context,
		op domain.ProductionOperation,
		version domain.ProductionVersion,
		targets []domain.ProductionTarget,
		links []domain.ProductionCandidateLink,
		contributors []domain.ProductionContributor,
		reservations []domain.ProductionIdentityReservation,
		cmd domain.ProductionCommand,
		claim domain.ProductionRequestClaim,
		proposals []domain.Proposal,
		assetDrafts []SemanticAssetDraft,
	) error

	GetProductionOperation(
		ctx context.Context,
		workspace identity.WorkspaceID,
		opID identity.ProductionOperationID,
	) (domain.ProductionOperation, domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, []domain.ProductionContributor, error)

	ListProductionOperations(
		ctx context.Context,
		workspace identity.WorkspaceID,
		limit int,
		offset int,
	) ([]domain.ProductionOperation, error)
	ListProductionOperationsPage(context.Context, identity.WorkspaceID, domain.ProductionListQuery) ([]domain.ProductionOperation, error)

	GetProductionCommand(
		ctx context.Context,
		workspace identity.WorkspaceID,
		principal identity.PrincipalID,
		cmdKind string,
		idempotencyKey string,
	) (*domain.ProductionCommand, error)

	GetProductionRequestClaim(
		ctx context.Context,
		workspace identity.WorkspaceID,
		businessDigest string,
	) (*domain.ProductionRequestClaim, error)

	GetProductionReservation(
		ctx context.Context,
		workspace identity.WorkspaceID,
		kind string,
		identityKey string,
	) (*domain.ProductionIdentityReservation, error)

	ReplaceDraftTx(
		ctx context.Context,
		opID identity.ProductionOperationID,
		newVersion domain.ProductionVersion,
		targets []domain.ProductionTarget,
		links []domain.ProductionCandidateLink,
		cmd domain.ProductionCommand,
		proposals []domain.Proposal,
	) error

	SubmitOperationTx(
		ctx context.Context,
		workspace identity.WorkspaceID,
		opID identity.ProductionOperationID,
		version int,
		frozenAt time.Time,
		attempt domain.ValidationAttempt,
		runs []domain.ValidationRun,
		bindings []domain.ValidationBinding,
		proposals []domain.Proposal,
		cmd domain.ProductionCommand,
	) error

	CreateValidationAttemptTx(
		ctx context.Context,
		workspace identity.WorkspaceID,
		opID identity.ProductionOperationID,
		version int,
		attempt domain.ValidationAttempt,
		runs []domain.ValidationRun,
		bindings []domain.ValidationBinding,
		cmd domain.ProductionCommand,
	) error

	GetValidationAttempt(
		ctx context.Context,
		workspace identity.WorkspaceID,
		opID identity.ProductionOperationID,
		version int,
		attemptNo int,
	) (*domain.ValidationAttempt, error)

	GetLatestValidationAttempt(
		ctx context.Context,
		workspace identity.WorkspaceID,
		opID identity.ProductionOperationID,
		version int,
	) (*domain.ValidationAttempt, error)

	ListValidationAttempts(
		ctx context.Context,
		workspace identity.WorkspaceID,
		opID identity.ProductionOperationID,
		version int,
		limit int,
		offset int,
	) ([]domain.ValidationAttempt, error)

	ReviewOperationTx(
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
	) error

	PublishOperationTx(
		ctx context.Context,
		workspace identity.WorkspaceID,
		release domain.Release,
		releaseProposals []domain.ReleaseProposal,
		manifest domain.ProductionReleaseManifest,
		beforePins []domain.ProductionReleaseBeforePin,
		bindingInputs []domain.ProductionReleaseBindingInput,
		proposals []domain.Proposal,
		cmd domain.ProductionCommand,
	) error

	RollbackOperationTx(
		ctx context.Context,
		workspace identity.WorkspaceID,
		release domain.Release,
		releaseProposals []domain.ReleaseProposal,
		manifest domain.ProductionReleaseManifest,
		beforePins []domain.ProductionReleaseBeforePin,
		cmd domain.ProductionCommand,
	) error

	GetLatestRelease(
		ctx context.Context,
		workspace identity.WorkspaceID,
	) (*domain.Release, error)

	GetRelease(
		ctx context.Context,
		workspace identity.WorkspaceID,
		releaseID identity.ReleaseID,
	) (domain.Release, error)

	GetProductionRelease(
		ctx context.Context,
		workspace identity.WorkspaceID,
		releaseID identity.ReleaseID,
	) (*domain.Release, *domain.ProductionReleaseManifest, []domain.ProductionReleaseBeforePin, []domain.ReleaseProposal, error)
}

type SemanticAssetDraft struct {
	ID        identity.AssetID
	Namespace string
	Key       string
	AssetType string
}

type ProductionService struct {
	repo ProductionRepository
}

func NewProductionService(repo ProductionRepository) *ProductionService {
	return &ProductionService{repo: repo}
}

type CreateOperationCommand struct {
	WorkspaceID           identity.WorkspaceID
	PrincipalID           identity.PrincipalID
	IdempotencyKey        string
	SupersedesOperationID *identity.ProductionOperationID
	InputSnapshotID       *identity.SourceSnapshotID
	InputScope            json.RawMessage
	Targets               []domain.TargetDeclaration
	Candidates            []domain.CandidateDeclaration
	TraceID               string
}

type TargetResult struct {
	LocalKey   string               `json:"localKey"`
	Kind       string               `json:"kind"`
	TargetID   string               `json:"targetId"`
	ProposalID *identity.ProposalID `json:"proposalId,omitempty"`
	Outcome    string               `json:"outcome"`
}

type CandidateLinkResult struct {
	CandidateID      string   `json:"candidateId"`
	PrimaryTargetKey string   `json:"primaryTargetKey"`
	TargetKeys       []string `json:"targetKeys"`
}

type OperationResult struct {
	OperationID    identity.ProductionOperationID `json:"operationId"`
	Version        int                            `json:"version"`
	SetDigest      string                         `json:"setDigest"`
	Replayed       bool                           `json:"replayed"`
	Targets        []TargetResult                 `json:"targets"`
	CandidateLinks []CandidateLinkResult          `json:"candidateLinks"`
}

func (s *ProductionService) CreateOperation(ctx context.Context, cmd CreateOperationCommand) (*OperationResult, error) {
	result, err := s.createOperation(ctx, cmd)
	if err != nil && !cmd.WorkspaceID.IsZero() && !cmd.PrincipalID.IsZero() {
		// A competing transaction may have committed after the initial lookup.
		// Re-enter the normal replay path to compare payloads and reauthorize.
		if committed, lookupErr := s.repo.GetProductionCommand(ctx, cmd.WorkspaceID, cmd.PrincipalID, domain.CommandCreate, cmd.IdempotencyKey); lookupErr == nil && committed != nil {
			return s.createOperation(ctx, cmd)
		}
		if errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrIdentityConflict) || errors.Is(err, domain.ErrAlreadyProduced) {
			return s.createOperation(ctx, cmd)
		}
	}
	return result, err
}

func (s *ProductionService) createOperation(ctx context.Context, cmd CreateOperationCommand) (*OperationResult, error) {
	if cmd.WorkspaceID.IsZero() || cmd.PrincipalID.IsZero() || strings.TrimSpace(cmd.IdempotencyKey) == "" {
		return nil, fmt.Errorf("%w: workspace, principal, and idempotency key are required", domain.ErrInvalidArgument)
	}

	var normalizeErr error
	cmd.InputScope, normalizeErr = domain.CanonicalProductionInput(cmd.InputScope)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	cmd.Targets, cmd.Candidates, normalizeErr = domain.NormalizeProductionDeclarations(cmd.Targets, cmd.Candidates)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	if err := domain.ValidateProductionCanonicalBudget(cmd.InputScope, cmd.Targets); err != nil {
		return nil, err
	}
	if err := domain.ValidateTargetDeclarations(cmd.Targets); err != nil {
		return nil, err
	}
	if err := domain.ValidateProductionContentDeclarations(cmd.Targets); err != nil {
		return nil, err
	}
	if err := domain.ValidateCandidateDeclarations(cmd.Candidates, cmd.Targets); err != nil {
		return nil, err
	}
	if err := s.checkAuthoringInput(ctx, cmd.WorkspaceID, cmd.PrincipalID, cmd.InputScope, cmd.Targets); err != nil {
		return nil, err
	}

	// Canonicalize and digest input
	inputBytes, err := json.Marshal(struct {
		SnapshotID *string         `json:"snapshotId,omitempty"`
		Scope      json.RawMessage `json:"scope,omitempty"`
	}{
		SnapshotID: func() *string {
			if cmd.InputSnapshotID != nil {
				s := cmd.InputSnapshotID.String()
				return &s
			}
			return nil
		}(),
		Scope: cmd.InputScope,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to marshal input: %v", domain.ErrInvalidArgument, err)
	}
	inputDigest, err := domain.DigestJSON(inputBytes)
	if err != nil {
		return nil, err
	}

	reqDigest, err := domain.ComputeRequestDigest(domain.RequestDigestInput{
		WorkspaceID: cmd.WorkspaceID.String(),
		CommandKind: domain.CommandCreate,
		SupersedesOperationID: func() *string {
			if cmd.SupersedesOperationID != nil {
				s := cmd.SupersedesOperationID.String()
				return &s
			}
			return nil
		}(),
		InputDigest: inputDigest,
		Targets:     cmd.Targets,
		Candidates:  cmd.Candidates,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to compute request digest: %v", domain.ErrInvalidArgument, err)
	}

	// Check idempotency command log
	existingCmd, err := s.repo.GetProductionCommand(ctx, cmd.WorkspaceID, cmd.PrincipalID, domain.CommandCreate, cmd.IdempotencyKey)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if existingCmd != nil {
		if existingCmd.RequestDigest != reqDigest {
			return nil, fmt.Errorf("%w: idempotency key %q reused with different request payload", domain.ErrIdempotencyConflict, cmd.IdempotencyKey)
		}
		// Existing command found: verify replay
		op, ver, targets, links, _, err := s.GetOperationVersion(ctx, cmd.WorkspaceID, existingCmd.OperationID, existingCmd.OperationVersion)
		if err != nil {
			return nil, err
		}
		if err := s.AuthorizeOperationRead(ctx, cmd.PrincipalID, *ver, targets); err != nil {
			return nil, err
		}
		var targetResults []TargetResult
		for _, t := range targets {
			targetResults = append(targetResults, TargetResult{
				LocalKey:   t.LocalKey,
				Kind:       t.Kind,
				TargetID:   t.TargetID,
				ProposalID: t.ProposalID,
				Outcome:    t.Outcome,
			})
		}
		var linkResults []CandidateLinkResult
		for _, l := range links {
			linkResults = append(linkResults, CandidateLinkResult{
				CandidateID:      l.CandidateID,
				PrimaryTargetKey: l.LocalKey,
				TargetKeys:       []string{l.LocalKey},
			})
		}
		return &OperationResult{
			OperationID:    op.ID,
			Version:        ver.Version,
			SetDigest:      ver.SetDigest,
			Replayed:       true,
			Targets:        targetResults,
			CandidateLinks: linkResults,
		}, nil
	}

	// Check business digest claim before allocating new reservations
	var predecessorVersion int
	predecessorTargets := map[string]domain.ProductionTarget{}
	if cmd.SupersedesOperationID != nil {
		_, previous, targets, _, _, err := s.GetOperation(ctx, cmd.WorkspaceID, *cmd.SupersedesOperationID)
		if err != nil {
			return nil, err
		}
		if err := s.AuthorizeOperationRead(ctx, cmd.PrincipalID, *previous, targets); err != nil {
			return nil, err
		}
		predecessorVersion = previous.Version
		for _, target := range targets {
			predecessorTargets[target.TargetID] = target
		}
	}
	baseline, err := s.repo.PrepareProductionBaseline(ctx, cmd.WorkspaceID, cmd.PrincipalID, cmd.Targets)
	if err != nil {
		return nil, err
	}
	// The create request digest excludes actor and allocated IDs, but includes
	// declaration identities, bases and supersedes, unlike content-only hashes.
	baselineDigest, err := domain.DigestJSON(baseline.CanonicalJSON)
	if err != nil {
		return nil, err
	}
	businessDigest := domain.ComputeBusinessDigest(cmd.WorkspaceID.String(), inputDigest, []string{reqDigest, baselineDigest})
	claim, err := s.repo.GetProductionRequestClaim(ctx, cmd.WorkspaceID, businessDigest)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if claim != nil {
		return nil, s.alreadyProduced(ctx, cmd.WorkspaceID, cmd.PrincipalID, claim.OperationID)
	}

	// Prepare target identifiers and local mapping
	localToTargetID := make(map[string]string, len(cmd.Targets))
	var reservations []domain.ProductionIdentityReservation
	var assetDrafts []SemanticAssetDraft

	opID, err := identity.NewProductionOperationID()
	if err != nil {
		return nil, err
	}

	for _, decl := range cmd.Targets {
		var targetIDStr string
		identityKey := ""
		if decl.IdentityKey != nil {
			identityKey = *decl.IdentityKey
		}

		if decl.Intent == domain.ProductionIntentCreate {
			if decl.ReuseIdentity != nil {
				reservation, err := s.repo.GetProductionReservation(ctx, cmd.WorkspaceID, decl.Kind, identityKey)
				if err != nil {
					return nil, err
				}
				if reservation == nil || reservation.TargetID != decl.ReuseIdentity.TargetID || reservation.CreationOperationID.String() != decl.ReuseIdentity.CreationOperationID {
					return nil, domain.ErrPriorStateUnknown
				}
				reservation.OwnerOperationID = opID
				reservations = append(reservations, *reservation)
				localToTargetID[decl.LocalKey] = reservation.TargetID
				continue
			}
			if cmd.SupersedesOperationID != nil && decl.IdentityKey != nil {
				reservation, err := s.repo.GetProductionReservation(ctx, cmd.WorkspaceID, decl.Kind, *decl.IdentityKey)
				if err != nil && !errors.Is(err, domain.ErrNotFound) {
					return nil, err
				}
				if reservation != nil {
					previous, ok := predecessorTargets[reservation.TargetID]
					if !ok || reservation.OwnerOperationID != *cmd.SupersedesOperationID || previous.Kind != decl.Kind || previous.Intent != domain.ProductionIntentCreate {
						return nil, domain.ErrIdentityConflict
					}
					if previous.ProposalState != nil && *previous.ProposalState == "released" {
						return nil, domain.ErrPriorStateUnknown
					}
					reservation.OwnerOperationID = opID
					reservations = append(reservations, *reservation)
					localToTargetID[decl.LocalKey] = reservation.TargetID
					continue
				}
			}
			switch decl.Kind {
			case domain.TargetKindSemanticAsset:
				id, err := identity.NewAssetID()
				if err != nil {
					return nil, err
				}
				targetIDStr = id.String()
				if identityKey == "" {
					identityKey = fmt.Sprintf("default.%s", decl.LocalKey)
				}
				ns, k := parseNamespaceAndKey(identityKey)
				aType := "business_term"
				if len(decl.Content) > 0 {
					var cm map[string]any
					if err := json.Unmarshal(decl.Content, &cm); err == nil {
						if at, ok := cm["assetType"].(string); ok && at != "" {
							aType = at
						}
					}
				}
				assetDrafts = append(assetDrafts, SemanticAssetDraft{
					ID:        id,
					Namespace: ns,
					Key:       k,
					AssetType: aType,
				})
			case domain.TargetKindPhysicalBinding:
				id, err := identity.NewPhysicalBindingID()
				if err != nil {
					return nil, err
				}
				targetIDStr = id.String()
			case domain.TargetKindModelGrain:
				id, err := identity.NewModelGrainID()
				if err != nil {
					return nil, err
				}
				targetIDStr = id.String()
			case domain.TargetKindEntityKey:
				id, err := identity.NewEntityKeyID()
				if err != nil {
					return nil, err
				}
				targetIDStr = id.String()
			case domain.TargetKindJoinContract:
				id, err := identity.NewJoinContractID()
				if err != nil {
					return nil, err
				}
				targetIDStr = id.String()
			default:
				return nil, fmt.Errorf("%w: unsupported target kind %q", domain.ErrInvalidArgument, decl.Kind)
			}

			// Check reservation uniqueness
			if identityKey != "" {
				res, err := s.repo.GetProductionReservation(ctx, cmd.WorkspaceID, decl.Kind, identityKey)
				if err != nil && !errors.Is(err, domain.ErrNotFound) {
					return nil, err
				}
				if res != nil {
					return nil, fmt.Errorf("%w: target %q identity %q already reserved", domain.ErrIdentityConflict, decl.LocalKey, identityKey)
				}
				reservations = append(reservations, domain.ProductionIdentityReservation{
					WorkspaceID:         cmd.WorkspaceID,
					Kind:                decl.Kind,
					IdentityKey:         identityKey,
					TargetID:            targetIDStr,
					CreationOperationID: opID,
					OwnerOperationID:    opID,
					CreatedAt:           time.Now().UTC(),
				})
			}
		} else {
			// Update: identity key or existing target ID must be provided
			if decl.TargetID != nil {
				targetIDStr = *decl.TargetID
			}
			if identityKey != "" {
				res, err := s.repo.GetProductionReservation(ctx, cmd.WorkspaceID, decl.Kind, identityKey)
				if err != nil && !errors.Is(err, domain.ErrNotFound) {
					return nil, err
				}
				if res != nil {
					targetIDStr = res.TargetID
				}
			}
			if targetIDStr == "" {
				targetIDStr = identityKey
			}
		}

		localToTargetID[decl.LocalKey] = targetIDStr
	}

	// Resolve local references in content and compute target digests
	var prodTargets []domain.ProductionTarget
	var proposals []domain.Proposal
	var targetResults []TargetResult

	for _, decl := range cmd.Targets {
		target, proposal, err := buildProductionTarget(cmd.WorkspaceID, opID, cmd.PrincipalID, 1, decl, localToTargetID, baseline)
		if err != nil {
			return nil, err
		}
		prodTargets = append(prodTargets, target)
		if proposal != nil {
			proposals = append(proposals, *proposal)
		}
		targetResults = append(targetResults, TargetResult{
			LocalKey:   decl.LocalKey,
			Kind:       decl.Kind,
			TargetID:   target.TargetID,
			ProposalID: target.ProposalID,
			Outcome:    target.Outcome,
		})
	}
	if err := checkProductionPrimaryOutcomes(cmd.Candidates, prodTargets); err != nil {
		return nil, err
	}

	// Prepare candidate links
	var candidateLinks []domain.ProductionCandidateLink
	var linkResults []CandidateLinkResult
	for _, cand := range cmd.Candidates {
		for _, key := range cand.TargetKeys {
			isPrimary := (key == cand.PrimaryTargetKey)
			candidateLinks = append(candidateLinks, domain.ProductionCandidateLink{
				WorkspaceID:     cmd.WorkspaceID,
				CandidateID:     cand.CandidateID,
				CandidateDigest: cand.CandidateDigest,
				OperationID:     opID,
				Version:         1,
				LocalKey:        key,
				IsPrimary:       isPrimary,
			})
		}
		linkResults = append(linkResults, CandidateLinkResult{
			CandidateID:      cand.CandidateID,
			PrimaryTargetKey: cand.PrimaryTargetKey,
			TargetKeys:       cand.TargetKeys,
		})
	}

	// Contributors
	contributors := []domain.ProductionContributor{
		{
			WorkspaceID: cmd.WorkspaceID,
			OperationID: opID,
			PrincipalID: cmd.PrincipalID,
			Role:        domain.ContributorAuthor,
			CreatedAt:   time.Now().UTC(),
		},
	}

	setDigest, err := productionSetDigest(inputDigest, reqDigest, baseline, prodTargets)
	if err != nil {
		return nil, err
	}
	declarationsJSON, err := json.Marshal(cmd.Targets)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	op := domain.ProductionOperation{
		ID:                    opID,
		WorkspaceID:           cmd.WorkspaceID,
		CreatedBy:             cmd.PrincipalID,
		CurrentVersion:        1,
		SupersedesOperationID: cmd.SupersedesOperationID,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	ver := domain.ProductionVersion{
		SupersedesOperationID: cmd.SupersedesOperationID, SupersedesVersion: predecessorVersion,
		TraceID:          cmd.TraceID,
		DeclarationsJSON: declarationsJSON, BaselineJSON: baseline.CanonicalJSON, BaselineHead: baseline.Head, HistoryQuality: "verified",
		WorkspaceID:   cmd.WorkspaceID,
		OperationID:   opID,
		Version:       1,
		InputJSON:     inputBytes,
		InputDigest:   inputDigest,
		RequestDigest: reqDigest,
		SetDigest:     setDigest,
		CreatedBy:     cmd.PrincipalID,
		CreatedAt:     now,
	}

	reqClaim := domain.ProductionRequestClaim{
		WorkspaceID:    cmd.WorkspaceID,
		BusinessDigest: businessDigest,
		OperationID:    opID,
		CreatedAt:      now,
	}

	pCmd := domain.ProductionCommand{
		WorkspaceID:      cmd.WorkspaceID,
		PrincipalID:      cmd.PrincipalID,
		CommandKind:      domain.CommandCreate,
		IdempotencyKey:   cmd.IdempotencyKey,
		RequestDigest:    reqDigest,
		OperationID:      opID,
		OperationVersion: 1,
		ResultKind:       "operation",
		ResultID:         opID.String(),
		CommittedAt:      now,
	}

	if err := s.repo.CreateProductionOperationTx(ctx, op, ver, prodTargets, candidateLinks, contributors, reservations, pCmd, reqClaim, proposals, assetDrafts); err != nil {
		return nil, err
	}

	return &OperationResult{
		OperationID:    opID,
		Version:        1,
		SetDigest:      setDigest,
		Replayed:       false,
		Targets:        targetResults,
		CandidateLinks: linkResults,
	}, nil
}

func (s *ProductionService) GetOperation(ctx context.Context, workspace identity.WorkspaceID, opID identity.ProductionOperationID) (*domain.ProductionOperation, *domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, []domain.ProductionContributor, error) {
	if workspace.IsZero() || opID.IsZero() {
		return nil, nil, nil, nil, nil, fmt.Errorf("%w: workspace and operation ID are required", domain.ErrInvalidArgument)
	}
	op, ver, targets, links, contributors, err := s.repo.GetProductionOperation(ctx, workspace, opID)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return &op, &ver, targets, links, contributors, nil
}

func (s *ProductionService) ListOperations(ctx context.Context, workspace identity.WorkspaceID, limit int, offset int) ([]domain.ProductionOperation, error) {
	if workspace.IsZero() {
		return nil, fmt.Errorf("%w: workspace is required", domain.ErrInvalidArgument)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.repo.ListProductionOperations(ctx, workspace, limit, offset)
}

type ReplaceDraftCommand struct {
	WorkspaceID     identity.WorkspaceID
	PrincipalID     identity.PrincipalID
	OperationID     identity.ProductionOperationID
	ExpectedVersion int
	IdempotencyKey  string
	InputSnapshotID *identity.SourceSnapshotID
	InputScope      json.RawMessage
	Targets         []domain.TargetDeclaration
	Candidates      []domain.CandidateDeclaration
	SuggestionRunID *identity.AgentRunID
	TraceID         string
}

func (s *ProductionService) ReplaceDraft(ctx context.Context, cmd ReplaceDraftCommand) (*OperationResult, error) {
	result, err := s.replaceDraft(ctx, cmd)
	if err != nil && !cmd.WorkspaceID.IsZero() && !cmd.PrincipalID.IsZero() {
		if committed, lookupErr := s.repo.GetProductionCommand(ctx, cmd.WorkspaceID, cmd.PrincipalID, domain.CommandReplaceDraft, cmd.IdempotencyKey); lookupErr == nil && committed != nil {
			return s.replaceDraft(ctx, cmd)
		}
	}
	return result, err
}

func (s *ProductionService) replaceDraft(ctx context.Context, cmd ReplaceDraftCommand) (*OperationResult, error) {
	if cmd.WorkspaceID.IsZero() || cmd.PrincipalID.IsZero() || cmd.OperationID.IsZero() || strings.TrimSpace(cmd.IdempotencyKey) == "" || cmd.ExpectedVersion < 1 {
		return nil, fmt.Errorf("%w: workspace, principal, operation ID, expected version, and idempotency key are required", domain.ErrInvalidArgument)
	}
	var normalizeErr error
	cmd.InputScope, normalizeErr = domain.CanonicalProductionInput(cmd.InputScope)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	cmd.Targets, cmd.Candidates, normalizeErr = domain.NormalizeProductionDeclarations(cmd.Targets, cmd.Candidates)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	if err := domain.ValidateProductionCanonicalBudget(cmd.InputScope, cmd.Targets); err != nil {
		return nil, err
	}

	if err := domain.ValidateTargetDeclarations(cmd.Targets); err != nil {
		return nil, err
	}
	if err := domain.ValidateProductionContentDeclarations(cmd.Targets); err != nil {
		return nil, err
	}
	if err := domain.ValidateCandidateDeclarations(cmd.Candidates, cmd.Targets); err != nil {
		return nil, err
	}
	if err := s.checkAuthoringInput(ctx, cmd.WorkspaceID, cmd.PrincipalID, cmd.InputScope, cmd.Targets); err != nil {
		return nil, err
	}

	// Canonicalize input and compute digests
	inputBytes, err := json.Marshal(struct {
		SnapshotID *string         `json:"snapshotId,omitempty"`
		Scope      json.RawMessage `json:"scope,omitempty"`
	}{
		SnapshotID: func() *string {
			if cmd.InputSnapshotID != nil {
				s := cmd.InputSnapshotID.String()
				return &s
			}
			return nil
		}(),
		Scope: cmd.InputScope,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to marshal input: %v", domain.ErrInvalidArgument, err)
	}
	inputDigest, err := domain.DigestJSON(inputBytes)
	if err != nil {
		return nil, err
	}

	reqDigest, err := domain.ComputeRequestDigest(domain.RequestDigestInput{
		WorkspaceID:     cmd.WorkspaceID.String(),
		ResourceID:      cmd.OperationID.String(),
		CommandKind:     domain.CommandReplaceDraft,
		ExpectedVersion: &cmd.ExpectedVersion,
		InputDigest:     inputDigest,
		Targets:         cmd.Targets,
		Candidates:      cmd.Candidates,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to compute request digest: %v", domain.ErrInvalidArgument, err)
	}
	if cmd.SuggestionRunID != nil {
		if cmd.SuggestionRunID.IsZero() {
			return nil, domain.ErrInvalidArgument
		}
		raw, encodeErr := json.Marshal(struct {
			RequestDigest   string
			SuggestionRunID string
		}{reqDigest, cmd.SuggestionRunID.String()})
		if encodeErr != nil {
			return nil, encodeErr
		}
		reqDigest, err = domain.DigestJSON(raw)
		if err != nil {
			return nil, err
		}
	}

	// Idempotency check
	existingCmd, err := s.repo.GetProductionCommand(ctx, cmd.WorkspaceID, cmd.PrincipalID, domain.CommandReplaceDraft, cmd.IdempotencyKey)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if existingCmd != nil {
		if existingCmd.RequestDigest != reqDigest {
			return nil, fmt.Errorf("%w: idempotency key %q reused with different request payload", domain.ErrIdempotencyConflict, cmd.IdempotencyKey)
		}
		op, ver, targets, links, _, err := s.GetOperationVersion(ctx, cmd.WorkspaceID, existingCmd.OperationID, existingCmd.OperationVersion)
		if err != nil {
			return nil, err
		}
		if err := s.AuthorizeOperationRead(ctx, cmd.PrincipalID, *ver, targets); err != nil {
			return nil, err
		}
		var targetResults []TargetResult
		for _, t := range targets {
			targetResults = append(targetResults, TargetResult{
				LocalKey:   t.LocalKey,
				Kind:       t.Kind,
				TargetID:   t.TargetID,
				ProposalID: t.ProposalID,
				Outcome:    t.Outcome,
			})
		}
		var linkResults []CandidateLinkResult
		for _, l := range links {
			linkResults = append(linkResults, CandidateLinkResult{
				CandidateID:      l.CandidateID,
				PrimaryTargetKey: l.LocalKey,
				TargetKeys:       []string{l.LocalKey},
			})
		}
		return &OperationResult{
			OperationID:    op.ID,
			Version:        ver.Version,
			SetDigest:      ver.SetDigest,
			Replayed:       true,
			Targets:        targetResults,
			CandidateLinks: linkResults,
		}, nil
	}

	op, currentVersion, currentTargets, _, _, err := s.repo.GetProductionOperation(ctx, cmd.WorkspaceID, cmd.OperationID)
	if err != nil {
		return nil, err
	}
	if op.CurrentVersion != cmd.ExpectedVersion {
		return nil, fmt.Errorf("%w: expected version %d but current is %d", domain.ErrConflict, cmd.ExpectedVersion, op.CurrentVersion)
	}
	if currentVersion.FrozenAt != nil {
		return nil, fmt.Errorf("%w: frozen operation requires a successor", domain.ErrConflict)
	}
	if err := s.AuthorizeOperationRead(ctx, cmd.PrincipalID, currentVersion, currentTargets); err != nil {
		return nil, err
	}
	baseline, err := s.prepareProductionReplacementBaseline(ctx, cmd.WorkspaceID, cmd.PrincipalID, cmd.OperationID, cmd.Targets)
	if err != nil {
		return nil, err
	}

	nextVersion := op.CurrentVersion + 1
	now := time.Now().UTC()

	// Map existing targets by localKey to retain IDs if possible
	localToTargetID := make(map[string]string)
	for _, ct := range currentTargets {
		localToTargetID[ct.LocalKey] = ct.TargetID
	}

	for _, decl := range cmd.Targets {
		if decl.ReuseIdentity != nil {
			if prior, exists := localToTargetID[decl.LocalKey]; exists && prior != decl.ReuseIdentity.TargetID {
				return nil, domain.ErrIdentityConflict
			}
			localToTargetID[decl.LocalKey] = decl.ReuseIdentity.TargetID
		}
		if decl.Intent == domain.ProductionIntentUpdate && decl.TargetID != nil {
			if prior, exists := localToTargetID[decl.LocalKey]; exists && prior != *decl.TargetID {
				return nil, domain.ErrIdentityConflict
			}
			localToTargetID[decl.LocalKey] = *decl.TargetID
		}
		for _, previous := range currentTargets {
			if previous.LocalKey != decl.LocalKey {
				continue
			}
			if previous.Kind != decl.Kind || previous.Intent != decl.Intent || !sameProductionIdentityKey(previous.IdentityKey, decl.IdentityKey) {
				return nil, fmt.Errorf("%w: a retained localKey cannot change target identity", domain.ErrIdentityConflict)
			}
		}
		if _, exists := localToTargetID[decl.LocalKey]; !exists {
			var targetIDStr string
			switch decl.Kind {
			case domain.TargetKindSemanticAsset:
				id, err := identity.NewAssetID()
				if err != nil {
					return nil, err
				}
				targetIDStr = id.String()
			case domain.TargetKindPhysicalBinding:
				id, err := identity.NewPhysicalBindingID()
				if err != nil {
					return nil, err
				}
				targetIDStr = id.String()
			case domain.TargetKindModelGrain:
				id, err := identity.NewModelGrainID()
				if err != nil {
					return nil, err
				}
				targetIDStr = id.String()
			case domain.TargetKindEntityKey:
				id, err := identity.NewEntityKeyID()
				if err != nil {
					return nil, err
				}
				targetIDStr = id.String()
			case domain.TargetKindJoinContract:
				id, err := identity.NewJoinContractID()
				if err != nil {
					return nil, err
				}
				targetIDStr = id.String()
			default:
				return nil, fmt.Errorf("%w: unsupported target kind %q", domain.ErrInvalidArgument, decl.Kind)
			}
			localToTargetID[decl.LocalKey] = targetIDStr
		}
	}

	var prodTargets []domain.ProductionTarget
	var targetResults []TargetResult
	var proposals []domain.Proposal

	for _, decl := range cmd.Targets {
		pt, proposal, err := buildProductionTarget(cmd.WorkspaceID, cmd.OperationID, cmd.PrincipalID, nextVersion, decl, localToTargetID, baseline)
		if err != nil {
			return nil, err
		}
		prodTargets = append(prodTargets, pt)
		if proposal != nil {
			proposals = append(proposals, *proposal)
		}
		targetResults = append(targetResults, TargetResult{
			LocalKey:   decl.LocalKey,
			Kind:       decl.Kind,
			TargetID:   pt.TargetID,
			ProposalID: pt.ProposalID,
			Outcome:    pt.Outcome,
		})
	}
	if err := checkProductionPrimaryOutcomes(cmd.Candidates, prodTargets); err != nil {
		return nil, err
	}

	var candidateLinks []domain.ProductionCandidateLink
	var linkResults []CandidateLinkResult
	for _, cand := range cmd.Candidates {
		for _, key := range cand.TargetKeys {
			isPrimary := (key == cand.PrimaryTargetKey)
			candidateLinks = append(candidateLinks, domain.ProductionCandidateLink{
				WorkspaceID:     cmd.WorkspaceID,
				CandidateID:     cand.CandidateID,
				CandidateDigest: cand.CandidateDigest,
				OperationID:     cmd.OperationID,
				Version:         nextVersion,
				LocalKey:        key,
				IsPrimary:       isPrimary,
			})
		}
		linkResults = append(linkResults, CandidateLinkResult{
			CandidateID:      cand.CandidateID,
			PrimaryTargetKey: cand.PrimaryTargetKey,
			TargetKeys:       cand.TargetKeys,
		})
	}

	setDigest, err := productionSetDigest(inputDigest, reqDigest, baseline, prodTargets)
	if err != nil {
		return nil, err
	}
	declarationsJSON, err := json.Marshal(cmd.Targets)
	if err != nil {
		return nil, err
	}

	newVer := domain.ProductionVersion{
		TraceID:          cmd.TraceID,
		DeclarationsJSON: declarationsJSON, BaselineJSON: baseline.CanonicalJSON, BaselineHead: baseline.Head, HistoryQuality: "verified",
		WorkspaceID:   cmd.WorkspaceID,
		OperationID:   cmd.OperationID,
		Version:       nextVersion,
		InputJSON:     inputBytes,
		InputDigest:   inputDigest,
		RequestDigest: reqDigest,
		SetDigest:     setDigest,
		CreatedBy:     cmd.PrincipalID,
		CreatedAt:     now,
	}

	pCmd := domain.ProductionCommand{
		WorkspaceID:      cmd.WorkspaceID,
		PrincipalID:      cmd.PrincipalID,
		CommandKind:      domain.CommandReplaceDraft,
		IdempotencyKey:   cmd.IdempotencyKey,
		RequestDigest:    reqDigest,
		OperationID:      cmd.OperationID,
		OperationVersion: nextVersion,
		ResultKind:       "operation",
		ResultID:         cmd.OperationID.String(),
		CommittedAt:      now,
	}

	if cmd.SuggestionRunID != nil {
		repo, ok := s.repo.(productionGenerationApplicationRepository)
		if !ok {
			return nil, domain.ErrPriorStateUnknown
		}
		err = repo.ReplaceDraftWithGenerationTx(ctx, cmd.OperationID, newVer, prodTargets, candidateLinks, pCmd, proposals, *cmd.SuggestionRunID)
	} else {
		err = s.repo.ReplaceDraftTx(ctx, cmd.OperationID, newVer, prodTargets, candidateLinks, pCmd, proposals)
	}
	if err != nil {
		return nil, err
	}

	return &OperationResult{
		OperationID:    cmd.OperationID,
		Version:        nextVersion,
		SetDigest:      setDigest,
		Replayed:       false,
		Targets:        targetResults,
		CandidateLinks: linkResults,
	}, nil
}

func parseNamespaceAndKey(address string) (string, string) {
	if separator := strings.LastIndexByte(address, '.'); separator >= 0 {
		return address[:separator], address[separator+1:]
	}
	return "default", address
}

func productionProposalTitle(decl domain.TargetDeclaration) string {
	if title := strings.TrimSpace(decl.Title); title != "" {
		return title
	}
	return fmt.Sprintf("Production %s: %s", decl.Intent, decl.LocalKey)
}

func sameProductionIdentityKey(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

type SubmitOperationCommand struct {
	WorkspaceID     identity.WorkspaceID
	PrincipalID     identity.PrincipalID
	OperationID     identity.ProductionOperationID
	IdempotencyKey  string
	ExpectedVersion int
	SetDigest       string
	TraceID         string
}

type ValidateOperationCommand struct {
	WorkspaceID       identity.WorkspaceID
	PrincipalID       identity.PrincipalID
	OperationID       identity.ProductionOperationID
	IdempotencyKey    string
	ExpectedVersion   int
	SetDigest         string
	PreviousAttemptNo int
	Reason            string
	TraceID           string
}

type ReviewOperationCommand struct {
	WorkspaceID         identity.WorkspaceID
	PrincipalID         identity.PrincipalID
	OperationID         identity.ProductionOperationID
	IdempotencyKey      string
	ExpectedVersion     int
	SetDigest           string
	ValidationAttemptNo int
	ValidationDigest    string
	ProposalIDs         []identity.ProposalID
	Decision            string
	Note                string
	TraceID             string
}

type PublishOperationCommand struct {
	WorkspaceID         identity.WorkspaceID
	PrincipalID         identity.PrincipalID
	OperationID         identity.ProductionOperationID
	IdempotencyKey      string
	ExpectedVersion     int
	SetDigest           string
	ValidationAttemptNo int
	ValidationDigest    string
	ExpectedHead        domain.HeadReference
	TraceID             string
}

type RollbackOperationCommand struct {
	WorkspaceID     identity.WorkspaceID
	PrincipalID     identity.PrincipalID
	ReleaseID       identity.ReleaseID
	IdempotencyKey  string
	ExpectedVersion int
	SetDigest       string
	ExpectedHead    domain.HeadReference
	Reason          string
	TraceID         string
}

type ProductionCommandResult struct {
	OperationID         identity.ProductionOperationID `json:"operationId"`
	Version             int                            `json:"version"`
	SetDigest           string                         `json:"setDigest"`
	Replayed            bool                           `json:"replayed"`
	Outcome             string                         `json:"outcome"`
	ProposalIDs         []identity.ProposalID          `json:"proposalIds"`
	ValidationAttemptNo *int                           `json:"validationAttemptNo,omitempty"`
	ValidationRunIDs    []identity.ValidationRunID     `json:"validationRunIds"`
	ReviewIDs           []identity.ReviewID            `json:"reviewIds"`
}

type ReleaseCommandResult struct {
	ReleaseID      identity.ReleaseID             `json:"releaseId"`
	OperationID    identity.ProductionOperationID `json:"operationId"`
	Replayed       bool                           `json:"replayed"`
	ManifestDigest string                         `json:"manifestDigest"`
}

type ProductionReleaseDetail struct {
	Release          domain.Release
	Attribution      *domain.ReleaseProposal
	Protection       domain.ProductionReleaseProtection
	BeforeManifest   domain.ProductionReleaseManifest
	BeforePins       []domain.ProductionReleaseBeforePin
	ReleaseProposals []domain.ReleaseProposal
	Contributors     []domain.ProductionContributor
	ProjectionStatus string
}

func (s *ProductionService) SubmitOperation(ctx context.Context, cmd SubmitOperationCommand) (*ProductionCommandResult, error) {
	return s.queueProductionValidation(ctx, cmd, 0, "")
}

func (s *ProductionService) ValidateOperation(ctx context.Context, cmd ValidateOperationCommand) (*ProductionCommandResult, error) {
	if cmd.PreviousAttemptNo < 1 || strings.TrimSpace(cmd.Reason) == "" {
		return nil, domain.ErrInvalidArgument
	}
	return s.queueProductionValidation(ctx, SubmitOperationCommand{
		WorkspaceID: cmd.WorkspaceID, PrincipalID: cmd.PrincipalID, OperationID: cmd.OperationID,
		IdempotencyKey: cmd.IdempotencyKey, ExpectedVersion: cmd.ExpectedVersion,
		SetDigest: cmd.SetDigest, TraceID: cmd.TraceID,
	}, cmd.PreviousAttemptNo, cmd.Reason)
}

func (s *ProductionService) GetValidationAttempts(ctx context.Context, workspaceID identity.WorkspaceID, opID identity.ProductionOperationID, version int, limit int, offset int) ([]domain.ValidationAttempt, error) {
	if workspaceID.IsZero() || opID.IsZero() {
		return nil, domain.ErrInvalidArgument
	}
	return s.repo.ListValidationAttempts(ctx, workspaceID, opID, version, limit, offset)
}

func (s *ProductionService) ReviewOperation(ctx context.Context, cmd ReviewOperationCommand) (*ProductionCommandResult, error) {
	return s.reviewProduction(ctx, cmd)
}

func (s *ProductionService) PublishOperation(ctx context.Context, cmd PublishOperationCommand) (*ReleaseCommandResult, error) {
	return s.publishProduction(ctx, cmd)
}

func (s *ProductionService) RollbackOperation(ctx context.Context, cmd RollbackOperationCommand) (*ReleaseCommandResult, error) {
	return s.rollbackProduction(ctx, cmd)
}

func (s *ProductionService) GetProductionRelease(ctx context.Context, workspaceID identity.WorkspaceID, releaseID identity.ReleaseID) (*ProductionReleaseDetail, error) {
	rel, manifest, beforePins, proposals, err := s.repo.GetProductionRelease(ctx, workspaceID, releaseID)
	if err != nil {
		return nil, err
	}
	if rel == nil {
		return nil, domain.ErrNotFound
	}
	var attr *domain.ReleaseProposal
	if len(proposals) > 0 {
		attr = &proposals[0]
	}
	prot := domain.ProductionReleaseProtection{}
	if rel.ProductionRootReleaseID != nil {
		prot.RootReleaseID = *rel.ProductionRootReleaseID
	}
	if rel.ProductionRollbackDepth != nil {
		prot.RollbackDepth = *rel.ProductionRollbackDepth
	}
	prot.RollbackParentReleaseID = rel.ProductionRollbackParentID

	var man domain.ProductionReleaseManifest
	if manifest != nil {
		man = *manifest
	}

	return &ProductionReleaseDetail{
		Release:          *rel,
		Attribution:      attr,
		Protection:       prot,
		BeforeManifest:   man,
		BeforePins:       beforePins,
		ReleaseProposals: proposals,
	}, nil
}
