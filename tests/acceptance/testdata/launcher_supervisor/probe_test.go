package launcher_supervisor

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
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
	if got := os.Getenv("NODE_USE_SYSTEM_CA"); got != "1" {
		t.Fatalf("launcher probe NODE_USE_SYSTEM_CA = %q, want 1", got)
	}
	rootInfo, err := os.Lstat(launcherRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || rootInfo.Mode().Perm() != 0o700 {
		t.Fatalf("launcher probe root is not a private physical directory: info=%v err=%v", rootInfo, err)
	}
	bundle := os.Getenv("NODE_EXTRA_CA_CERTS")
	if want := filepath.Join(launcherRoot, "system-ca.pem"); bundle != want {
		t.Fatalf("launcher probe NODE_EXTRA_CA_CERTS = %q, want %q", bundle, want)
	}
	bundleInfo, err := os.Lstat(bundle)
	if err != nil || !bundleInfo.Mode().IsRegular() || bundleInfo.Size() <= 0 || bundleInfo.Mode().Perm() != 0o600 {
		t.Fatalf("launcher probe CA bundle is not a private physical regular file: info=%v err=%v", bundleInfo, err)
	}
	body, err := os.ReadFile(bundle)
	if err != nil {
		t.Fatal(err)
	}
	rest := body
	certificates := 0
	for {
		rest = bytes.TrimSpace(rest)
		if len(rest) == 0 {
			break
		}
		if !bytes.HasPrefix(rest, []byte("-----BEGIN CERTIFICATE-----")) {
			t.Fatal("launcher probe CA bundle contains data outside certificate PEM blocks")
		}
		block, remaining := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			t.Fatal("launcher probe CA bundle contains a non-certificate PEM block")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			t.Fatalf("launcher probe CA bundle contains an invalid certificate: %v", err)
		}
		certificates++
		rest = remaining
	}
	if certificates == 0 {
		t.Fatal("launcher probe CA bundle contains no certificates")
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
