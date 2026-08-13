package launcher_supervisor

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestFreshCloneAcceptance(t *testing.T) {
	launcherRoot := os.Getenv("SEMLIA_ACCEPTANCE_LAUNCHER_ROOT")
	sourceRoot := os.Getenv("SEMLIA_ACCEPTANCE_SOURCE")
	if launcherRoot == "" || sourceRoot == "" {
		t.Fatal("launcher probe requires isolated launcher and source roots")
	}
	wantPath := os.Getenv("SEMLIA_TEST_EXPECTED_PATH")
	if wantPath == "" || os.Getenv("PATH") != wantPath {
		t.Fatalf("launcher probe PATH = %q, want exact canonical PATH %q", os.Getenv("PATH"), wantPath)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "path-validated"), []byte("validated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		time.Sleep(250 * time.Millisecond)
		if _, err := os.Stat(launcherRoot); err != nil {
			t.Errorf("launcher root was removed before child cleanup: %v", err)
			return
		}
		if err := os.WriteFile(filepath.Join(sourceRoot, "child-cleaned"), []byte("cleaned\n"), 0o600); err != nil {
			t.Error(err)
		}
	})
	if err := os.WriteFile(filepath.Join(sourceRoot, "probe-ready"), []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
