//go:build darwin || linux

package acceptance

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const acceptanceLockHelperMode = "SEMLIA_ACCEPTANCE_LOCK_HELPER_MODE"

func TestAcceptanceAdvisoryLockHelper(t *testing.T) {
	mode := os.Getenv(acceptanceLockHelperMode)
	if mode == "" {
		return
	}
	path := os.Getenv("SEMLIA_ACCEPTANCE_LOCK_HELPER_PATH")
	release, err := createAcceptanceLock(path)
	if err != nil {
		t.Fatalf("LOCK_CONFLICT: %v", err)
	}
	defer func() {
		if err := release(); err != nil {
			t.Errorf("release helper lock: %v", err)
		}
	}()

	switch mode {
	case "hold":
		if err := os.WriteFile(os.Getenv("SEMLIA_ACCEPTANCE_LOCK_HELPER_READY"), []byte("locked\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(time.Hour)
		}
	case "try":
		return
	default:
		t.Fatalf("unknown lock helper mode %q", mode)
	}
}

func TestAcceptanceHostLockIsCrashSafeAcrossProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acceptance.lock")
	holderTMPDIR := t.TempDir()
	contenderTMPDIR := t.TempDir()
	holder, holderOutput := startAcceptanceLockHolder(t, path, holderTMPDIR)

	output, err := runAcceptanceLockHelper(path, contenderTMPDIR, "try", "")
	if err == nil || !strings.Contains(string(output), "LOCK_CONFLICT") {
		killAndWait(holder)
		t.Fatalf("second process bypassed the lock with a different TMPDIR: err=%v output=%s holder=%s", err, output, holderOutput.String())
	}
	if err := holder.Process.Kill(); err != nil {
		t.Fatalf("SIGKILL lock holder: %v", err)
	}
	if err := holder.Wait(); err == nil {
		t.Fatal("SIGKILLed lock holder returned success")
	}

	output, err = runAcceptanceLockHelper(path, contenderTMPDIR, "try", "")
	if err != nil {
		t.Fatalf("process crash did not automatically release advisory lock: %v\n%s", err, output)
	}
}

func TestAcceptanceLockReleaseCannotAffectSuccessor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acceptance.lock")
	firstRelease, err := createAcceptanceLock(path)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	if err := firstRelease(); err != nil {
		t.Fatalf("release first lock: %v", err)
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("release removed the persistent advisory-lock inode: info=%v err=%v", info, err)
	}
	secondRelease, err := createAcceptanceLock(path)
	if err != nil {
		t.Fatalf("acquire successor lock: %v", err)
	}
	defer func() {
		if err := secondRelease(); err != nil {
			t.Errorf("release successor lock: %v", err)
		}
	}()

	if err := firstRelease(); err != nil {
		t.Fatalf("repeat old release: %v", err)
	}
	thirdRelease, err := createAcceptanceLock(path)
	if err == nil {
		_ = thirdRelease()
		t.Fatal("an old release closure removed or unlocked its successor")
	}
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("successor contention error = %v, want os.ErrExist", err)
	}
}

func TestAcceptanceLockRejectsFilesystemAliasWithoutMutatingTarget(t *testing.T) {
	directory := t.TempDir()
	victim := filepath.Join(directory, "victim")
	const original = "do not truncate\n"
	if err := os.WriteFile(victim, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		link func(string, string) error
	}{
		{name: "symbolic link", link: os.Symlink},
		{name: "hard link", link: os.Link},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(directory, strings.ReplaceAll(test.name, " ", "-"))
			if err := test.link(victim, path); err != nil {
				t.Fatal(err)
			}
			if release, err := createAcceptanceLock(path); err == nil {
				_ = release()
				t.Fatal("filesystem alias was accepted as the advisory-lock inode")
			}
			content, err := os.ReadFile(victim)
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != original {
				t.Fatalf("rejected alias mutated target: %q", content)
			}
		})
	}
}

func startAcceptanceLockHolder(t *testing.T, path, temporaryDirectory string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready")
	command := exec.Command(os.Args[0], "-test.run=^TestAcceptanceAdvisoryLockHelper$", "-test.count=1")
	command.Env = append(
		filteredEnvironment(os.Environ(), acceptanceLockHelperMode, "SEMLIA_ACCEPTANCE_LOCK_HELPER_PATH", "SEMLIA_ACCEPTANCE_LOCK_HELPER_READY", "TMPDIR"),
		acceptanceLockHelperMode+"=hold",
		"SEMLIA_ACCEPTANCE_LOCK_HELPER_PATH="+path,
		"SEMLIA_ACCEPTANCE_LOCK_HELPER_READY="+ready,
		"TMPDIR="+temporaryDirectory,
	)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatalf("start advisory lock holder: %v", err)
	}
	t.Cleanup(func() { killAndWait(command) })

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(ready); err == nil {
			return command, &output
		}
		if command.ProcessState != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	killAndWait(command)
	t.Fatalf("advisory lock holder did not become ready: %s", output.String())
	return nil, nil
}

func runAcceptanceLockHelper(path, temporaryDirectory, mode, ready string) ([]byte, error) {
	command := exec.Command(os.Args[0], "-test.run=^TestAcceptanceAdvisoryLockHelper$", "-test.count=1")
	command.Env = append(
		filteredEnvironment(os.Environ(), acceptanceLockHelperMode, "SEMLIA_ACCEPTANCE_LOCK_HELPER_PATH", "SEMLIA_ACCEPTANCE_LOCK_HELPER_READY", "TMPDIR"),
		acceptanceLockHelperMode+"="+mode,
		"SEMLIA_ACCEPTANCE_LOCK_HELPER_PATH="+path,
		"SEMLIA_ACCEPTANCE_LOCK_HELPER_READY="+ready,
		"TMPDIR="+temporaryDirectory,
	)
	return command.CombinedOutput()
}

func killAndWait(command *exec.Cmd) {
	if command == nil || command.Process == nil || command.ProcessState != nil {
		return
	}
	_ = command.Process.Kill()
	_ = command.Wait()
}
