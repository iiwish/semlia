package local_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/iiwish/semlia/internal/adapters/artifacts/local"
	"github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestStorePersistsWorkspaceScopedDigestAndRejectsSymlink(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := local.New(root)
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := identity.NewWorkspaceID()
	content := []byte("workspace-owned artifact")
	hash := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(hash[:])
	if err := store.Put(ctx, workspace, digest, bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatal(err)
	}
	reader, err := store.Open(ctx, workspace, digest)
	if err != nil {
		t.Fatal(err)
	}
	read, _ := io.ReadAll(reader)
	_ = reader.Close()
	if !bytes.Equal(read, content) {
		t.Fatalf("content = %q", read)
	}
	other, _ := identity.NewWorkspaceID()
	if _, err := store.Open(ctx, other, digest); !errors.Is(err, ingestion.ErrNotFound) {
		t.Fatalf("cross-workspace open = %v", err)
	}
	path := filepath.Join(root, filepath.FromSlash(local.StorageKey(workspace, digest)))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(ctx, workspace, digest); !errors.Is(err, ingestion.ErrNotFound) {
		t.Fatalf("symlink open = %v", err)
	}
}

func TestStoreRejectsDigestAndSizeMismatch(t *testing.T) {
	store, _ := local.New(t.TempDir())
	workspace, _ := identity.NewWorkspaceID()
	content := []byte("payload")
	digest := "sha256:" + string(bytes.Repeat([]byte{'0'}, 64))
	if err := store.Put(context.Background(), workspace, digest, bytes.NewReader(content), int64(len(content))); !errors.Is(err, ingestion.ErrStore) {
		t.Fatalf("digest mismatch = %v", err)
	}
}

func TestStoreDetectsExistingAndReadTimeCorruption(t *testing.T) {
	root := t.TempDir()
	store, _ := local.New(root)
	workspace, _ := identity.NewWorkspaceID()
	content := []byte("original")
	hash := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(hash[:])
	if err := store.Put(context.Background(), workspace, digest, bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatal(err)
	}
	objectPath := filepath.Join(root, filepath.FromSlash(local.StorageKey(workspace, digest)))
	if err := os.WriteFile(objectPath, []byte("corrupt!"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(context.Background(), workspace, digest); !errors.Is(err, ingestion.ErrStore) {
		t.Fatalf("corrupt open = %v", err)
	}
	if err := store.Put(context.Background(), workspace, digest, bytes.NewReader(content), int64(len(content))); !errors.Is(err, ingestion.ErrStore) {
		t.Fatalf("corrupt dedupe = %v", err)
	}
}

func TestStoreConcurrentPutPublishesOneVerifiedObject(t *testing.T) {
	store, _ := local.New(t.TempDir())
	workspace, _ := identity.NewWorkspaceID()
	content := bytes.Repeat([]byte("a"), 1<<20)
	hash := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(hash[:])
	errorsCh := make(chan error, 8)
	for range 8 {
		go func() {
			errorsCh <- store.Put(context.Background(), workspace, digest, bytes.NewReader(content), int64(len(content)))
		}()
	}
	for range 8 {
		if err := <-errorsCh; err != nil {
			t.Fatal(err)
		}
	}
	reader, err := store.Open(context.Background(), workspace, digest)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	actual, _ := io.ReadAll(reader)
	if !bytes.Equal(actual, content) {
		t.Fatal("published object differs")
	}
}
