package embedding

import (
	"context"
	"errors"
	"strings"

	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/embedding"
	"github.com/iiwish/semlia/pkg/identity"
)

const JobType = "embedding.rebuild"

type Provider interface {
	Configured(domain.Config) bool
	EmbedBatch(context.Context, domain.Config, []string) ([][]float32, error)
}

type StartCommand struct {
	Workspace               identity.WorkspaceID
	Principal               identity.PrincipalID
	AuthorizationVersion    int64
	IdempotencyKey, TraceID string
	Config                  domain.Config
}

type Repository interface {
	EmbeddingConfigured(context.Context) bool
	EmbeddingConfiguration(context.Context, identity.WorkspaceID) (domain.Config, error)
	// EmbeddingPublishedRelease reports whether the workspace has a published
	// release, which is the corpus a rebuild would index.
	EmbeddingPublishedRelease(context.Context, identity.WorkspaceID) (bool, error)
	StartEmbedding(context.Context, StartCommand) (domain.Index, error)
	EmbeddingStatus(context.Context, identity.WorkspaceID) (domain.Status, error)
	GetEmbedding(context.Context, identity.WorkspaceID, identity.RunID) (domain.Index, error)
	EmbeddingBatch(context.Context, jobs.Job) (domain.Index, []domain.Chunk, error)
	CheckpointEmbedding(context.Context, jobs.Job, domain.Index, []domain.Chunk, [][]float32) error
	ActivateEmbedding(context.Context, jobs.Job, domain.Index) error
	FailEmbedding(context.Context, jobs.Job, string) error
	CancelEmbedding(context.Context, identity.WorkspaceID, identity.RunID, int64) error
	SearchEmbedding(context.Context, identity.WorkspaceID, string, *domain.Index, []float32) ([]domain.Candidate, string, error)
}

type Service struct {
	repo       Repository
	provider   Provider
	authorizer authorizationapp.Evaluator
}

func NewService(repo Repository, provider Provider, authorizer authorizationapp.Evaluator) *Service {
	return &Service{repo: repo, provider: provider, authorizer: authorizer}
}

func (s *Service) authorize(ctx context.Context, workspace identity.WorkspaceID, principal, trace string, action authorization.Action, asset *identity.AssetID) (authorization.Decision, error) {
	if s.authorizer == nil {
		return authorization.Decision{}, domain.ErrNotConfigured
	}
	resource := authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}
	if asset != nil {
		resource = authorization.Resource{Type: authorization.ScopeAsset, ID: asset.UUID()}
	}
	decision, err := s.authorizer.Evaluate(ctx, authorizationapp.EvaluationRequest{WorkspaceID: workspace, PrincipalRef: principal, TraceID: trace, Action: action, Resource: resource})
	if err != nil {
		return decision, err
	}
	if !decision.Allowed {
		return decision, &authorization.DenialError{Decision: decision}
	}
	return decision, nil
}

func (s *Service) Status(ctx context.Context, w identity.WorkspaceID, p, t string) (domain.Status, error) {
	if _, err := s.authorize(ctx, w, p, t, authorization.ActionWorkspaceRead, nil); err != nil {
		return domain.Status{}, err
	}
	result, err := s.repo.EmbeddingStatus(ctx, w)
	if err != nil {
		return result, err
	}
	config, configErr := s.repo.EmbeddingConfiguration(ctx, w)
	result.Configured = configErr == nil && s.repo.EmbeddingConfigured(ctx) && s.provider != nil && s.provider.Configured(config)
	switch {
	case !result.Configured:
		result.Reason = domain.ReasonNotConfigured
	default:
		// The capability is usable; report the next blocking precondition, if any,
		// so the client can point the operator at the surface that fixes it.
		published, err := s.repo.EmbeddingPublishedRelease(ctx, w)
		if err == nil && !published {
			result.Reason = domain.ReasonNoPublishedRelease
		}
	}
	return result, nil
}

func (s *Service) Start(ctx context.Context, w identity.WorkspaceID, p, t, key string) (domain.Index, error) {
	decision, err := s.authorize(ctx, w, p, t, authorization.ActionWorkspaceManage, nil)
	if err != nil {
		return domain.Index{}, err
	}
	if decision.PrincipalID.IsZero() || len(key) < 1 || len(key) > 256 {
		return domain.Index{}, domain.ErrInvalid
	}
	config, err := s.repo.EmbeddingConfiguration(ctx, w)
	if err != nil || !s.repo.EmbeddingConfigured(ctx) || s.provider == nil || !s.provider.Configured(config) {
		return domain.Index{}, domain.ErrNotConfigured
	}
	return s.repo.StartEmbedding(ctx, StartCommand{Workspace: w, Principal: decision.PrincipalID, AuthorizationVersion: decision.AuthorizationVersion, IdempotencyKey: key, TraceID: t, Config: config})
}

func (s *Service) Cancel(ctx context.Context, w identity.WorkspaceID, id identity.RunID, p, t string) error {
	decision, err := s.authorize(ctx, w, p, t, authorization.ActionWorkspaceManage, nil)
	if err != nil {
		return err
	}
	return s.repo.CancelEmbedding(ctx, w, id, decision.AuthorizationVersion)
}

func (s *Service) JobHandler() jobs.Handler {
	return func(ctx context.Context, job jobs.Job) error {
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			index, chunks, err := s.repo.EmbeddingBatch(ctx, job)
			if err != nil {
				return err
			}
			if index.State == "cancelled" || index.State == "active" || index.State == "retired" {
				return nil
			}
			if index.State != "building" {
				return jobs.Permanent(domain.ErrConflict, "EMBEDDING_FAILED")
			}
			if len(chunks) == 0 {
				return s.repo.ActivateEmbedding(ctx, job, index)
			}
			texts := make([]string, len(chunks))
			for i, chunk := range chunks {
				texts[i] = chunk.Text
			}
			vectors, err := s.provider.EmbedBatch(ctx, index.Config, texts)
			if err == nil {
				err = domain.ValidateVectors(vectors, len(chunks), index.Dimension)
			}
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if job.Attempt >= job.MaxAttempts || errors.Is(err, domain.ErrNotConfigured) || errors.Is(err, domain.ErrInvalid) {
					if failErr := s.repo.FailEmbedding(ctx, job, "EMBEDDING_PROVIDER_FAILED"); failErr != nil {
						return failErr
					}
					return jobs.Permanent(domain.ErrProvider, "EMBEDDING_PROVIDER_FAILED")
				}
				return domain.ErrProvider
			}
			if err := s.repo.CheckpointEmbedding(ctx, job, index, chunks, vectors); err != nil {
				return err
			}
		}
	}
}

func (s *Service) Search(ctx context.Context, w identity.WorkspaceID, p, t, query string) (domain.SearchResult, error) {
	decision, err := s.authorize(ctx, w, p, t, authorization.ActionWorkspaceRead, nil)
	if err != nil {
		return domain.SearchResult{}, err
	}
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 1024 {
		return domain.SearchResult{}, domain.ErrInvalid
	}
	status, err := s.repo.EmbeddingStatus(ctx, w)
	if err != nil {
		return domain.SearchResult{}, err
	}
	var vector []float32
	reason := "no_active_index"
	if status.Active != nil && s.repo.EmbeddingConfigured(ctx) && s.provider != nil {
		vectors, embedErr := s.provider.EmbedBatch(ctx, status.Active.Config, []string{query})
		if embedErr == nil && domain.ValidateVectors(vectors, 1, status.Active.Dimension) == nil {
			vector = vectors[0]
			reason = ""
		} else {
			reason = "provider_unavailable"
		}
	}
	candidates, mode, err := s.repo.SearchEmbedding(ctx, w, query, status.Active, vector)
	if err != nil {
		return domain.SearchResult{}, err
	}
	if mode == "lexical" && reason == "" {
		reason = "release_changed"
	}
	result := domain.SearchResult{Mode: mode, FallbackReason: reason, Items: []domain.Candidate{}}
	for _, candidate := range candidates {
		allowed, err := s.authorize(ctx, w, p, t, authorization.ActionAssetRead, &candidate.AssetID)
		var denial *authorization.DenialError
		if errors.As(err, &denial) {
			continue
		}
		if err != nil {
			return domain.SearchResult{}, err
		}
		if allowed.AuthorizationVersion != decision.AuthorizationVersion {
			return domain.SearchResult{}, domain.ErrConflict
		}
		result.Items = append(result.Items, candidate)
		if len(result.Items) == 20 {
			break
		}
	}
	// Re-evaluate the workspace version after per-asset checks to reject a revoke
	// racing the response instead of serving stale grants.
	after, err := s.authorize(ctx, w, p, t, authorization.ActionWorkspaceRead, nil)
	if err != nil {
		return domain.SearchResult{}, err
	}
	if after.AuthorizationVersion != decision.AuthorizationVersion {
		return domain.SearchResult{}, domain.ErrConflict
	}
	return result, nil
}
