package discovery

import (
	"context"
	"errors"
	"fmt"

	domain "github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/pkg/identity"
)

var ErrAdapterNotFound = errors.New("discovery adapter not found")

type PersistResult struct {
	SnapshotID       string
	SourceRevisionID identity.SourceRevisionID
	RunID            identity.RunID
	Status           string
	Replayed         bool
	DatasetCount     int
	FieldCount       int
	LineageCount     int
	FindingCount     int
}

type Repository interface {
	PersistDiscoverySnapshot(
		context.Context,
		identity.WorkspaceID,
		identity.SourceConnectionID,
		domain.Snapshot,
	) (PersistResult, error)
}

type Request struct {
	WorkspaceID        identity.WorkspaceID
	SourceConnectionID identity.SourceConnectionID
	AdapterKind        string
	Input              domain.Input
}

type Service struct {
	repository Repository
	adapters   map[string]domain.Adapter
}

func NewService(repository Repository, adapters ...domain.Adapter) (*Service, error) {
	if repository == nil || len(adapters) == 0 {
		return nil, errors.New("discovery repository and adapters are required")
	}
	registry := make(map[string]domain.Adapter, len(adapters))
	for _, adapter := range adapters {
		if adapter == nil || adapter.Kind() == "" || adapter.Version() == "" {
			return nil, errors.New("discovery adapter identity is required")
		}
		if _, exists := registry[adapter.Kind()]; exists {
			return nil, fmt.Errorf("duplicate discovery adapter %q", adapter.Kind())
		}
		registry[adapter.Kind()] = adapter
	}
	return &Service{repository: repository, adapters: registry}, nil
}

func (service *Service) Discover(ctx context.Context, request Request) (PersistResult, error) {
	adapter, exists := service.adapters[request.AdapterKind]
	if !exists {
		return PersistResult{}, fmt.Errorf("%w: %s", ErrAdapterNotFound, request.AdapterKind)
	}
	snapshot, err := adapter.Discover(ctx, request.Input)
	if err != nil {
		return PersistResult{}, err
	}
	if snapshot.AdapterKind != adapter.Kind() || snapshot.AdapterVersion != adapter.Version() {
		return PersistResult{}, domain.ErrInvalidSnapshot
	}
	return service.repository.PersistDiscoverySnapshot(ctx, request.WorkspaceID, request.SourceConnectionID, snapshot)
}
