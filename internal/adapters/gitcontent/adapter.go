package gitcontent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	application "github.com/iiwish/semlia/internal/application/projection"
	domain "github.com/iiwish/semlia/internal/domain/projection"
)

var _ application.Writer = (*Adapter)(nil)

type Adapter struct {
	root       string
	repository *git.Repository
	mu         sync.Mutex
}

func Open(path string) (*Adapter, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%w: repository path is required", domain.ErrInvalid)
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve Git repository path: %w", err)
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create Git repository path: %w", err)
	}
	repository, err := git.PlainOpen(root)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		repository, err = git.PlainInit(root, false)
	}
	if err != nil {
		return nil, fmt.Errorf("open Git repository: %w", err)
	}
	return &Adapter{root: root, repository: repository}, nil
}

func (adapter *Adapter) ProjectAsset(ctx context.Context, asset domain.Asset) (domain.Result, error) {
	if err := ctx.Err(); err != nil {
		return domain.Result{}, err
	}
	if err := validateAsset(asset); err != nil {
		return domain.Result{}, err
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()

	worktree, err := adapter.repository.Worktree()
	if err != nil {
		return domain.Result{}, fmt.Errorf("open Git worktree: %w", err)
	}
	status, err := worktree.Status()
	if err != nil {
		return domain.Result{}, fmt.Errorf("read Git worktree status: %w", err)
	}
	if !status.IsClean() {
		return domain.Result{}, domain.ErrDirty
	}
	relativePath := assetPath(asset)
	absolutePath, err := securejoin.SecureJoin(adapter.root, filepath.FromSlash(relativePath))
	if err != nil {
		return domain.Result{}, fmt.Errorf("%w: projection path", domain.ErrInvalid)
	}
	previous, readErr := os.ReadFile(absolutePath)
	existed := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return domain.Result{}, fmt.Errorf("read current projection: %w", readErr)
	}
	if existed {
		current, parseErr := decodeCurrent(previous)
		if parseErr != nil {
			return domain.Result{}, parseErr
		}
		if current.RevisionID == asset.RevisionID.String() {
			return currentResult(adapter.repository, relativePath), nil
		}
		if asset.ExpectedBaseRevisionID == nil || current.RevisionID != asset.ExpectedBaseRevisionID.String() {
			return domain.Result{}, domain.ErrConflict
		}
	} else if asset.ExpectedBaseRevisionID != nil {
		return domain.Result{}, domain.ErrConflict
	}

	document, err := render(asset)
	if err != nil {
		return domain.Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o750); err != nil {
		return domain.Result{}, fmt.Errorf("create projection directory: %w", err)
	}
	if err := writeAtomic(absolutePath, document); err != nil {
		return domain.Result{}, err
	}
	rollback := func() {
		if existed {
			_ = writeAtomic(absolutePath, previous)
			_, _ = worktree.Add(relativePath)
			return
		}
		_ = os.Remove(absolutePath)
		_, _ = worktree.Remove(relativePath)
	}
	if _, err := worktree.Add(relativePath); err != nil {
		rollback()
		return domain.Result{}, fmt.Errorf("stage projection: %w", err)
	}
	when := asset.CreatedAt.UTC().Truncate(time.Second)
	hash, err := worktree.Commit(
		fmt.Sprintf("project %s %s", asset.Address.String(), asset.RevisionID.String()),
		&git.CommitOptions{
			Author:    &object.Signature{Name: "Semlia", Email: "projection@semlia.local", When: when},
			Committer: &object.Signature{Name: "Semlia", Email: "projection@semlia.local", When: when},
		},
	)
	if err != nil {
		rollback()
		return domain.Result{}, fmt.Errorf("commit projection: %w", err)
	}
	return domain.Result{Path: relativePath, CommitHash: hash.String(), Changed: true}, nil
}

type document struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   metadata `json:"metadata"`
	Spec       any      `json:"spec"`
}

type metadata struct {
	Address        string    `json:"address"`
	AssetID        string    `json:"assetId"`
	RevisionID     string    `json:"revisionId"`
	AssetType      string    `json:"assetType"`
	LifecycleState string    `json:"lifecycleState"`
	Sequence       int64     `json:"sequence"`
	SchemaVersion  string    `json:"schemaVersion"`
	ContentDigest  string    `json:"contentDigest"`
	CreatedBy      string    `json:"createdBy"`
	CreatedAt      time.Time `json:"createdAt"`
}

func render(asset domain.Asset) ([]byte, error) {
	var spec any
	decoder := json.NewDecoder(bytes.NewReader(asset.Content))
	decoder.UseNumber()
	if err := decoder.Decode(&spec); err != nil {
		return nil, fmt.Errorf("%w: canonical content", domain.ErrInvalid)
	}
	value := document{
		APIVersion: "semlia.io/v1", Kind: "SemanticAsset",
		Metadata: metadata{
			Address: asset.Address.String(), AssetID: asset.AssetID.String(), RevisionID: asset.RevisionID.String(),
			AssetType: string(asset.AssetType), LifecycleState: asset.LifecycleState, Sequence: asset.Sequence,
			SchemaVersion: asset.SchemaVersion, ContentDigest: asset.ContentDigest, CreatedBy: asset.CreatedBy,
			CreatedAt: asset.CreatedAt.UTC().Truncate(time.Second),
		},
		Spec: spec,
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render projection: %w", err)
	}
	return append(encoded, '\n'), nil
}

func validateAsset(asset domain.Asset) error {
	if asset.WorkspaceID.IsZero() || asset.AssetID.IsZero() || asset.RevisionID.IsZero() ||
		asset.Address.String() == "" || asset.Sequence < 1 || asset.SchemaVersion == "" ||
		!strings.HasPrefix(asset.ContentDigest, "sha256:") || asset.CreatedBy == "" || asset.CreatedAt.IsZero() {
		return domain.ErrInvalid
	}
	return nil
}

func assetPath(asset domain.Asset) string {
	segments := append([]string{"assets"}, strings.Split(asset.Address.Namespace(), ".")...)
	segments = append(segments, asset.Address.Key()+".json")
	return strings.Join(segments, "/")
}

func decodeCurrent(value []byte) (metadata, error) {
	var current document
	if err := json.Unmarshal(value, &current); err != nil || current.APIVersion != "semlia.io/v1" ||
		current.Kind != "SemanticAsset" || current.Metadata.RevisionID == "" {
		return metadata{}, fmt.Errorf("%w: current projection document", domain.ErrInvalid)
	}
	return current.Metadata, nil
}

func writeAtomic(path string, value []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".semlia-projection-*")
	if err != nil {
		return fmt.Errorf("create projection temporary file: %w", err)
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	if err := file.Chmod(0o640); err != nil {
		_ = file.Close()
		return fmt.Errorf("set projection permissions: %w", err)
	}
	if _, err := file.Write(value); err != nil {
		_ = file.Close()
		return fmt.Errorf("write projection: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync projection: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close projection: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace projection: %w", err)
	}
	return nil
}

func currentResult(repository *git.Repository, path string) domain.Result {
	result := domain.Result{Path: path}
	if head, err := repository.Head(); err == nil {
		result.CommitHash = head.Hash().String()
	}
	return result
}
