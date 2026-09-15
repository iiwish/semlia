package local

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type Store struct {
	root           *os.Root
	sweepMutex     sync.Mutex
	sweepDirectory *os.File
}

func New(rootPath string) (*Store, error) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return &Store{}, nil
	}
	absolute, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, ingestion.ErrStore
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, ingestion.ErrStore
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, ingestion.ErrStore
	}
	return &Store{root: root}, nil
}

func (store *Store) Configured() bool { return store != nil && store.root != nil }

func (store *Store) Check(ctx context.Context) (resultErr error) {
	if !store.Configured() || ctx.Err() != nil {
		return ingestion.ErrStore
	}
	info, err := store.root.Stat(".")
	if err != nil || !info.IsDir() {
		return ingestion.ErrStore
	}
	// Directory existence does not prove that the configured non-root process
	// can persist data on the mounted volume. Probe without business content.
	name := ".readiness-" + randomSuffix()
	file, err := store.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ingestion.ErrStore
	}
	defer func() {
		closeErr := file.Close()
		removeErr := store.root.Remove(name)
		if closeErr != nil || removeErr != nil {
			resultErr = ingestion.ErrStore
		}
	}()
	if _, err := file.Write([]byte{0}); err != nil {
		return ingestion.ErrStore
	}
	if err := file.Sync(); err != nil || ctx.Err() != nil {
		return ingestion.ErrStore
	}
	return nil
}

func (store *Store) Put(ctx context.Context, workspace identity.WorkspaceID, digest string, content io.Reader, size int64) error {
	if !store.Configured() || workspace.IsZero() || !digestPattern.MatchString(digest) || content == nil || size < 1 || size > ingestion.MaxUploadBytes {
		return ingestion.ErrStore
	}
	target := StorageKey(workspace, digest)
	parent := path.Dir(target)
	if err := store.ensureDirectories(parent); err != nil {
		return err
	}
	if existing, err := store.verify(ctx, target, digest, size); err == nil {
		return existing.Close()
	} else if !errors.Is(err, ingestion.ErrNotFound) {
		return err
	}

	if err := store.ensureDirectories(".staging"); err != nil {
		return err
	}
	temporary := ".staging/.artifact-" + randomSuffix()
	file, err := store.root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ingestion.ErrStore
	}
	defer file.Close()
	if !tryLockTemporary(file) {
		return ingestion.ErrStore
	}
	cleanupTemporary := true
	defer func() {
		if cleanupTemporary {
			_ = store.removeTemporary(temporary, ".staging")
		}
	}()
	hash := sha256.New()
	written, copyErr := copyContext(ctx, io.MultiWriter(file, hash), io.LimitReader(content, size+1))
	syncErr := file.Sync()
	if copyErr != nil || syncErr != nil || written != size || "sha256:"+hex.EncodeToString(hash.Sum(nil)) != digest {
		return ingestion.ErrStore
	}

	// A hard link publishes without replacing an existing immutable object.
	if err := store.root.Link(temporary, target); err != nil {
		if existing, verifyErr := store.verify(ctx, target, digest, size); verifyErr == nil {
			if err := existing.Close(); err != nil {
				return ingestion.ErrStore
			}
			if err := store.removeTemporary(temporary, ".staging"); err != nil {
				return ingestion.ErrStore
			}
			cleanupTemporary = false
			return nil
		}
		return ingestion.ErrStore
	}
	if err := syncDirectory(store.root, parent); err != nil {
		return ingestion.ErrStore
	}
	if err := store.removeTemporary(temporary, ".staging"); err != nil {
		return ingestion.ErrStore
	}
	cleanupTemporary = false
	return nil
}

// CleanupTemporary uses a bounded round-robin scan; file locks protect active writers across processes.
func (store *Store) CleanupTemporary(ctx context.Context, olderThan time.Time, limit int) (int, error) {
	if !store.Configured() || limit < 1 || limit > 100 {
		return 0, ingestion.ErrInvalid
	}
	store.sweepMutex.Lock()
	defer store.sweepMutex.Unlock()
	if store.sweepDirectory == nil {
		if err := store.ensureDirectories(".staging"); err != nil {
			return 0, err
		}
		directory, err := store.root.Open(".staging")
		if err != nil {
			return 0, ingestion.ErrStore
		}
		store.sweepDirectory = directory
	}
	names, err := store.sweepDirectory.Readdirnames(limit)
	if errors.Is(err, io.EOF) || (err == nil && len(names) < limit) {
		_ = store.sweepDirectory.Close()
		store.sweepDirectory = nil
	} else if err != nil {
		return 0, ingestion.ErrStore
	}
	removed := 0
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if !strings.HasPrefix(name, ".artifact-") {
			continue
		}
		target := path.Join(".staging", name)
		info, err := store.root.Lstat(target)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return removed, ingestion.ErrStore
		}
		if !info.Mode().IsRegular() || !info.ModTime().Before(olderThan) {
			continue
		}
		file, err := store.root.Open(target)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return removed, ingestion.ErrStore
		}
		locked := tryLockTemporary(file)
		openedInfo, statErr := file.Stat()
		if locked && statErr == nil && os.SameFile(info, openedInfo) {
			if err := store.removeTemporary(target, ".staging"); err != nil {
				_ = file.Close()
				return removed, ingestion.ErrStore
			}
			removed++
		}
		_ = file.Close()
	}
	return removed, nil
}

func (store *Store) Open(ctx context.Context, workspace identity.WorkspaceID, digest string) (io.ReadCloser, error) {
	if !store.Configured() || workspace.IsZero() || !digestPattern.MatchString(digest) {
		return nil, ingestion.ErrStore
	}
	return store.verify(ctx, StorageKey(workspace, digest), digest, -1)
}

func (store *Store) Delete(_ context.Context, workspace identity.WorkspaceID, digest string) error {
	if !store.Configured() || workspace.IsZero() || !digestPattern.MatchString(digest) {
		return ingestion.ErrStore
	}
	target := StorageKey(workspace, digest)
	if err := store.root.Remove(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return ingestion.ErrStore
	}
	if err := syncDirectory(store.root, path.Dir(target)); err != nil {
		return ingestion.ErrStore
	}
	return nil
}

func (store *Store) verify(ctx context.Context, name, digest string, size int64) (*os.File, error) {
	info, err := store.root.Lstat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ingestion.ErrNotFound
		}
		return nil, ingestion.ErrStore
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, ingestion.ErrNotFound
	}
	if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > ingestion.MaxUploadBytes || (size >= 0 && info.Size() != size) {
		return nil, ingestion.ErrStore
	}
	file, err := store.root.Open(name)
	if err != nil {
		return nil, ingestion.ErrStore
	}
	hash := sha256.New()
	written, hashErr := copyContext(ctx, hash, io.LimitReader(file, ingestion.MaxUploadBytes+1))
	if hashErr != nil || written != info.Size() || "sha256:"+hex.EncodeToString(hash.Sum(nil)) != digest {
		_ = file.Close()
		return nil, ingestion.ErrStore
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, ingestion.ErrStore
	}
	return file, nil
}

func (store *Store) ensureDirectories(name string) error {
	current := ""
	for _, component := range strings.Split(name, "/") {
		if component == "" || component == "." || component == ".." {
			return ingestion.ErrStore
		}
		current = path.Join(current, component)
		info, err := store.root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := store.root.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return ingestion.ErrStore
			}
			if err := syncDirectory(store.root, path.Dir(current)); err != nil {
				return ingestion.ErrStore
			}
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ingestion.ErrStore
		}
	}
	return nil
}

func (store *Store) removeTemporary(name, parent string) error {
	if err := store.root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(store.root, parent)
}

func StorageKey(workspace identity.WorkspaceID, digest string) string {
	if workspace.IsZero() || !digestPattern.MatchString(digest) {
		return ""
	}
	return fmt.Sprintf("workspaces/%s/sha256/%s", workspace.UUID(), strings.TrimPrefix(digest, "sha256:"))
}

func randomSuffix() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(value)
}

func syncDirectory(root *os.Root, name string) error {
	directory, err := root.Open(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func copyContext(ctx context.Context, writer io.Writer, reader io.Reader) (int64, error) {
	buffer := make([]byte, 64<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		read, readErr := reader.Read(buffer)
		if read > 0 {
			written, writeErr := writer.Write(buffer[:read])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
	}
}
