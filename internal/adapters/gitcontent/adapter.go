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

// ProjectRelease appends the immutable release document to the projected Git
// repository: one new file per release at releases/<sequence>-<releaseId>.json,
// committed append-only on top of the current head. No tags, remotes, reverts
// or history rewrites are ever created; an already-projected release is a
// no-op, so redelivery is idempotent.
func (adapter *Adapter) ProjectRelease(ctx context.Context, release domain.Release) (domain.Result, error) {
	if err := ctx.Err(); err != nil {
		return domain.Result{}, err
	}
	if err := validateRelease(release); err != nil {
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
	relativePath := releasePath(release)
	absolutePath, err := securejoin.SecureJoin(adapter.root, filepath.FromSlash(relativePath))
	if err != nil {
		return domain.Result{}, fmt.Errorf("%w: projection path", domain.ErrInvalid)
	}
	if _, readErr := os.Stat(absolutePath); readErr == nil {
		return currentResult(adapter.repository, relativePath), nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return domain.Result{}, fmt.Errorf("read current projection: %w", readErr)
	}
	document, err := renderRelease(release)
	if err != nil {
		return domain.Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o750); err != nil {
		return domain.Result{}, fmt.Errorf("create projection directory: %w", err)
	}
	if err := writeAtomic(absolutePath, document); err != nil {
		return domain.Result{}, err
	}
	if _, err := worktree.Add(relativePath); err != nil {
		_ = os.Remove(absolutePath)
		return domain.Result{}, fmt.Errorf("stage projection: %w", err)
	}
	when := release.PublishedAt.UTC().Truncate(time.Second)
	hash, err := worktree.Commit(
		fmt.Sprintf("project release %s %d", release.ReleaseID.String(), release.Sequence),
		&git.CommitOptions{
			Author:    &object.Signature{Name: "Semlia", Email: "projection@semlia.local", When: when},
			Committer: &object.Signature{Name: "Semlia", Email: "projection@semlia.local", When: when},
		},
	)
	if err != nil {
		_ = os.Remove(absolutePath)
		_, _ = worktree.Remove(relativePath)
		return domain.Result{}, fmt.Errorf("commit projection: %w", err)
	}
	return domain.Result{Path: relativePath, CommitHash: hash.String(), Changed: true}, nil
}

type releaseDocument struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   releaseMetadata `json:"metadata"`
	Spec       releaseSpec     `json:"spec"`
}

type releaseMetadata struct {
	ReleaseID             string    `json:"releaseId"`
	WorkspaceID           string    `json:"workspaceId"`
	Sequence              int64     `json:"sequence"`
	State                 string    `json:"state"`
	ManifestDigest        string    `json:"manifestDigest"`
	RolledBackToReleaseID string    `json:"rolledBackToReleaseId,omitempty"`
	PublishedBy           string    `json:"publishedBy"`
	PublishedAt           time.Time `json:"publishedAt"`
}

type releaseSpec struct {
	Manifest releaseManifest `json:"manifest"`
	Proposal *releaseSpecProposal
}

type releaseManifest struct {
	Assets  []releaseManifestAsset  `json:"assets"`
	Objects []releaseManifestObject `json:"objects"`
}

type releaseManifestAsset struct {
	AssetID       string          `json:"assetId"`
	RevisionID    string          `json:"revisionId"`
	Address       string          `json:"address"`
	Compatibility json.RawMessage `json:"compatibility"`
	Position      int             `json:"position"`
}

type releaseManifestObject struct {
	ObjectType string `json:"objectType"`
	ObjectID   string `json:"objectId"`
	Version    int    `json:"version"`
	Position   int    `json:"position"`
}

type releaseSpecProposal struct {
	ProposalID       string `json:"proposalId"`
	Title            string `json:"title"`
	TargetObjectType string `json:"targetObjectType"`
	TargetObjectID   string `json:"targetObjectId"`
	CreatedBy        string `json:"createdBy"`
}

func renderRelease(release domain.Release) ([]byte, error) {
	metadata := releaseMetadata{
		ReleaseID: release.ReleaseID.String(), WorkspaceID: release.WorkspaceID.String(),
		Sequence: release.Sequence, State: release.State, ManifestDigest: release.ManifestDigest,
		PublishedBy: release.PublishedBy, PublishedAt: release.PublishedAt.UTC().Truncate(time.Second),
	}
	if release.RolledBackToReleaseID != nil {
		metadata.RolledBackToReleaseID = release.RolledBackToReleaseID.String()
	}
	assets := make([]releaseManifestAsset, 0, len(release.Assets))
	for _, entry := range release.Assets {
		compatibility := entry.Compatibility
		if len(compatibility) == 0 {
			compatibility = json.RawMessage(`{}`)
		}
		assets = append(assets, releaseManifestAsset{
			AssetID: entry.AssetID.String(), RevisionID: entry.RevisionID.String(),
			Address: entry.Address.String(), Compatibility: compatibility, Position: entry.Position,
		})
	}
	objects := make([]releaseManifestObject, 0, len(release.Objects))
	for _, entry := range release.Objects {
		objects = append(objects, releaseManifestObject{
			ObjectType: entry.ObjectType, ObjectID: entry.ObjectID,
			Version: entry.Version, Position: entry.Position,
		})
	}
	var proposal *releaseSpecProposal
	if release.Proposal != nil {
		proposal = &releaseSpecProposal{
			ProposalID: release.Proposal.ProposalID.String(), Title: release.Proposal.Title,
			TargetObjectType: release.Proposal.TargetObjectType,
			TargetObjectID:   release.Proposal.TargetObjectID, CreatedBy: release.Proposal.CreatedBy,
		}
	}
	encoded, err := json.MarshalIndent(releaseDocument{
		APIVersion: "semlia.io/v1", Kind: "SemanticAssetRelease",
		Metadata: metadata, Spec: releaseSpec{Manifest: releaseManifest{Assets: assets, Objects: objects}, Proposal: proposal},
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render release projection: %w", err)
	}
	return append(encoded, '\n'), nil
}

func validateRelease(release domain.Release) error {
	if release.WorkspaceID.IsZero() || release.ReleaseID.IsZero() || release.Sequence < 1 ||
		release.State == "" || !strings.HasPrefix(release.ManifestDigest, "sha256:") ||
		release.PublishedBy == "" || release.PublishedAt.IsZero() {
		return domain.ErrInvalid
	}
	for _, entry := range release.Assets {
		if entry.AssetID.IsZero() || entry.RevisionID.IsZero() || entry.Position < 1 {
			return domain.ErrInvalid
		}
	}
	for _, entry := range release.Objects {
		if entry.ObjectType == "" || entry.ObjectID == "" || entry.Version < 1 || entry.Position < 1 {
			return domain.ErrInvalid
		}
	}
	return nil
}

func releasePath(release domain.Release) string {
	return fmt.Sprintf("releases/%d-%s.json", release.Sequence, release.ReleaseID.String())
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
