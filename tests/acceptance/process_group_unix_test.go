//go:build darwin || linux

package acceptance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const processTreeHelperMode = "SEMLIA_ACCEPTANCE_PROCESS_TREE_HELPER"

func TestAcceptanceProcessTreeHelper(t *testing.T) {
	mode := os.Getenv(processTreeHelperMode)
	if mode == "" {
		return
	}

	childPIDPath := os.Getenv("SEMLIA_ACCEPTANCE_CHILD_PID_PATH")
	parentPIDPath := os.Getenv("SEMLIA_ACCEPTANCE_PARENT_PID_PATH")
	sentinelPath := os.Getenv("SEMLIA_ACCEPTANCE_PROCESS_SENTINEL")
	switch mode {
	case "parent":
		if err := os.WriteFile(parentPIDPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			t.Fatal(err)
		}
		child := exec.Command(os.Args[0], "-test.run=^TestAcceptanceProcessTreeHelper$", "-test.count=1")
		child.Env = append(
			filteredEnvironment(os.Environ(), processTreeHelperMode),
			processTreeHelperMode+"=child",
		)
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(childPIDPath, []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			_ = child.Process.Kill()
			t.Fatal(err)
		}
		fmt.Println("acceptance process-tree child started")
		if err := child.Wait(); err != nil {
			t.Fatal(err)
		}
	case "child":
		fmt.Println("acceptance process-tree child ready")
		time.Sleep(4 * time.Second)
		if err := os.WriteFile(sentinelPath, []byte("child survived cancellation\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown process-tree helper mode %q", mode)
	}
}

func TestAcceptanceSubprocessCancellationKillsProcessTree(t *testing.T) {
	scratch := t.TempDir()
	childPIDPath := filepath.Join(scratch, "child.pid")
	parentPIDPath := filepath.Join(scratch, "parent.pid")
	sentinelPath := filepath.Join(scratch, "survived")
	environment := append(
		filteredEnvironment(os.Environ(), processTreeHelperMode, "SEMLIA_ACCEPTANCE_CHILD_PID_PATH", "SEMLIA_ACCEPTANCE_PARENT_PID_PATH", "SEMLIA_ACCEPTANCE_PROCESS_SENTINEL"),
		processTreeHelperMode+"=parent",
		"SEMLIA_ACCEPTANCE_CHILD_PID_PATH="+childPIDPath,
		"SEMLIA_ACCEPTANCE_PARENT_PID_PATH="+parentPIDPath,
		"SEMLIA_ACCEPTANCE_PROCESS_SENTINEL="+sentinelPath,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	started := time.Now()
	output, err := executeSubprocess(
		ctx,
		scratch,
		environment,
		os.Args[0],
		"-test.run=^TestAcceptanceProcessTreeHelper$",
		"-test.count=1",
	)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatalf("cancelled process tree returned success: %s", output)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("process-tree cancellation was not prompt: elapsed=%s output=%s error=%v", elapsed, output, err)
	}
	if !strings.Contains(string(output), "acceptance process-tree child ready") {
		t.Fatalf("regression did not start the stdout-inheriting child: %s", output)
	}

	for _, pidPath := range []string{parentPIDPath, childPIDPath} {
		waitForProcessExit(t, readProcessPID(t, pidPath), 2*time.Second)
	}
	if _, statErr := os.Stat(sentinelPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cancelled child left sentinel %q: %v", sentinelPath, statErr)
	}
}

func readProcessPID(t *testing.T, path string) int {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read helper pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		t.Fatalf("parse helper pid %q: %v", content, err)
	}
	return pid
}

func waitForProcessExit(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		process, err := os.FindProcess(pid)
		if err != nil || process.Signal(syscall.Signal(0)) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("task-owned child process %d survived cancellation", pid)
}
