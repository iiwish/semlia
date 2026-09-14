//go:build unix

package local_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/adapters/artifacts/local"
	"github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
	"golang.org/x/sys/unix"
)

func TestStoreReadinessRejectsConfiguredUnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission denial requires the production non-root execution model")
	}
	root := t.TempDir()
	store, err := local.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	if !store.Configured() {
		t.Fatal("existing directory must remain configured")
	}
	if err := store.Check(context.Background()); !errors.Is(err, ingestion.ErrStore) {
		t.Fatalf("unwritable readiness error=%v", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	previous := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previous)
	before := openDescriptorCount(t)
	for range 10 {
		if err := store.Check(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if after := openDescriptorCount(t); after != before {
		t.Fatalf("readiness leaked descriptors: before=%d after=%d", before, after)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("readiness left files=%v err=%v", entries, err)
	}
}

func TestStoreRecoversAbandonedSpoolsWithoutDeletingActiveOrCanonicalFiles(t *testing.T) {
	root := t.TempDir()
	store, err := local.New(root)
	if err != nil {
		t.Fatal(err)
	}
	spool := filepath.Join(root, ".staging")
	if err := os.MkdirAll(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	for _, name := range []string{".artifact-abandoned", ".artifact-active", ".artifact-recent"} {
		file := filepath.Join(spool, name)
		if err := os.WriteFile(file, []byte("private artifact"), 0o600); err != nil {
			t.Fatal(err)
		}
		if name != ".artifact-recent" {
			if err := os.Chtimes(file, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	active, err := os.Open(filepath.Join(spool, ".artifact-active"))
	if err != nil {
		t.Fatal(err)
	}
	defer active.Close()
	if err := unix.Flock(int(active.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(root, "canonical-object")
	if err := os.Link(filepath.Join(spool, ".artifact-abandoned"), canonical); err != nil {
		t.Fatal(err)
	}
	if removed, err := store.CleanupTemporary(context.Background(), time.Now().Add(-time.Hour), 100); err != nil || removed != 1 {
		t.Fatalf("cleanup removed=%d err=%v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(spool, ".artifact-abandoned")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("abandoned spool retained: %v", err)
	}
	for _, path := range []string{canonical, filepath.Join(spool, ".artifact-active"), filepath.Join(spool, ".artifact-recent")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	if err := unix.Flock(int(active.Fd()), unix.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if removed, err := store.CleanupTemporary(context.Background(), time.Now().Add(-time.Hour), 100); err != nil || removed != 1 {
		t.Fatalf("restart cleanup removed=%d err=%v", removed, err)
	}
}

func TestStoreDedupeClosesVerifiedFiles(t *testing.T) {
	previous := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previous)
	for _, race := range []bool{false, true} {
		t.Run(fmt.Sprintf("publication_race_%t", race), func(t *testing.T) {
			store, err := local.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			workspace, _ := identity.NewWorkspaceID()
			content := []byte("repeated immutable object")
			hash := sha256.Sum256(content)
			digest := "sha256:" + hex.EncodeToString(hash[:])
			put := func() {
				if err := store.Put(context.Background(), workspace, digest, bytes.NewReader(content), int64(len(content))); err != nil {
					t.Fatal(err)
				}
			}
			put()
			before := openDescriptorCount(t)
			for range 20 {
				if race {
					if err := store.Delete(context.Background(), workspace, digest); err != nil {
						t.Fatal(err)
					}
					reader := &publishOnRead{Reader: bytes.NewReader(content), publish: put}
					if err := store.Put(context.Background(), workspace, digest, reader, int64(len(content))); err != nil {
						t.Fatal(err)
					}
				} else {
					put()
				}
			}
			if after := openDescriptorCount(t); after != before {
				t.Fatalf("dedupe leaked file descriptors: before=%d after=%d", before, after)
			}
		})
	}
}

func openDescriptorCount(t *testing.T) int {
	t.Helper()
	fdDirectory := "/proc/self/fd"
	if _, err := os.Stat(fdDirectory); err != nil {
		fdDirectory = "/dev/fd"
	}
	directory, err := os.Open(fdDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	entries, err := directory.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

type publishOnRead struct {
	io.Reader
	publish func()
}

func (reader *publishOnRead) Read(buffer []byte) (int, error) {
	if reader.publish != nil {
		publish := reader.publish
		reader.publish = nil
		publish()
	}
	return reader.Reader.Read(buffer)
}
