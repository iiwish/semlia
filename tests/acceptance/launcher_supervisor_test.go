//go:build darwin || linux

package acceptance

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type launcherSupervisorFixture struct {
	launcher       string
	moduleRoot     string
	launcherRecord string
	childRecord    string
	trapReady      string
	rootReady      string
	poisonMarker   string
}

func TestFreshCloneLauncherForwardsSignalWaitsAndRemovesRoot(t *testing.T) {
	fixture := newLauncherSupervisorFixture(t, true, nil)
	process := startLauncherSupervisorFixture(t, fixture)

	waitForPath(t, fixture.launcherRecord, 10*time.Second)
	launcherRootBytes, err := os.ReadFile(fixture.launcherRecord)
	if err != nil {
		t.Fatal(err)
	}
	launcherRoot := strings.TrimSpace(string(launcherRootBytes))
	waitForPath(t, filepath.Join(fixture.moduleRoot, "path-validated"), 30*time.Second)
	waitForPath(t, filepath.Join(fixture.moduleRoot, "probe-ready"), 30*time.Second)
	if err := process.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}

	waitErr := process.wait(t, 15*time.Second)
	var exitError *exec.ExitError
	if !errors.As(waitErr, &exitError) || exitError.ExitCode() != 143 {
		t.Fatalf("launcher exit = %v, want signal status 143\n%s", waitErr, process.output.String())
	}
	if _, err := os.Stat(filepath.Join(fixture.moduleRoot, "child-cleaned")); err != nil {
		t.Fatalf("launcher exited before the child handled its signal: %v\n%s", err, process.output.String())
	}
	if _, err := os.Stat(launcherRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("launcher root remains after supervised exit: %v\n%s", err, process.output.String())
	}
	if _, err := os.Stat(fixture.poisonMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("launcher sourced an ambient Bash startup file: %v", err)
	}
}

func TestFreshCloneLauncherDeliversSignalReceivedBeforeChildAssignment(t *testing.T) {
	fixture := newLauncherSupervisorFixture(t, true, func(t *testing.T, launcher string) string {
		t.Helper()
		return replaceLauncherFragment(t, launcher,
			"fi\n\nif [ -n \"${LAUNCHER_SIGNAL_STATUS}\" ]; then\n  exit \"${LAUNCHER_SIGNAL_STATUS}\"\nfi\nset -m",
			"fi\n\nprintf ready >\"${SEMLIA_TEST_TRAP_READY}\"\n/bin/sleep 1\nif [ -n \"${LAUNCHER_SIGNAL_STATUS}\" ]; then\n  exit \"${LAUNCHER_SIGNAL_STATUS}\"\nfi\nset -m",
		)
	})
	process := startLauncherSupervisorFixture(t, fixture)
	waitForPath(t, fixture.trapReady, 10*time.Second)
	waitForPath(t, fixture.launcherRecord, 10*time.Second)
	launcherRootBytes, err := os.ReadFile(fixture.launcherRecord)
	if err != nil {
		t.Fatal(err)
	}
	launcherRoot := strings.TrimSpace(string(launcherRootBytes))
	if err := process.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waitErr := process.wait(t, 15*time.Second)
	var exitError *exec.ExitError
	if !errors.As(waitErr, &exitError) || exitError.ExitCode() != 143 {
		t.Fatalf("launcher exit = %v, want pending signal status 143\n%s", waitErr, process.output.String())
	}
	for _, absent := range []string{fixture.childRecord, filepath.Join(fixture.moduleRoot, "path-validated"), filepath.Join(fixture.moduleRoot, "probe-ready"), filepath.Join(fixture.moduleRoot, "child-cleaned")} {
		if _, err := os.Stat(absent); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("pre-child signal launched child state %s: %v\n%s", absent, err, process.output.String())
		}
	}
	if _, err := os.Stat(launcherRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending signal left launcher root behind: %v\n%s", err, process.output.String())
	}
}

func TestFreshCloneLauncherRemovesRootWhenSignalledImmediatelyAfterCreation(t *testing.T) {
	fixture := newLauncherSupervisorFixture(t, true, func(t *testing.T, launcher string) string {
		t.Helper()
		return replaceLauncherFragment(t, launcher,
			"  printf '%s\\n' \"${LAUNCHER_ROOT}\" >\"${SEMLIA_TEST_LAUNCHER_ROOT_RECORD}\" || exit 1\nfi\n",
			"  printf '%s\\n' \"${LAUNCHER_ROOT}\" >\"${SEMLIA_TEST_LAUNCHER_ROOT_RECORD}\" || exit 1\nfi\nprintf ready >\"${SEMLIA_TEST_ROOT_READY}\"\n/bin/sleep 1\n",
		)
	})
	process := startLauncherSupervisorFixture(t, fixture)
	waitForPath(t, fixture.rootReady, 10*time.Second)
	waitForPath(t, fixture.launcherRecord, 10*time.Second)
	launcherRootBytes, err := os.ReadFile(fixture.launcherRecord)
	if err != nil {
		t.Fatal(err)
	}
	launcherRoot := strings.TrimSpace(string(launcherRootBytes))
	if err := process.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waitErr := process.wait(t, 15*time.Second)
	var exitError *exec.ExitError
	if !errors.As(waitErr, &exitError) || exitError.ExitCode() != 143 {
		t.Fatalf("launcher exit = %v, want early signal status 143\n%s", waitErr, process.output.String())
	}
	if _, err := os.Stat(launcherRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("early signal left launcher root behind: %v\n%s", err, process.output.String())
	}
}

func TestFreshCloneLauncherCleanupFailureOverridesChildSuccess(t *testing.T) {
	fixture := newLauncherSupervisorFixture(t, false, func(t *testing.T, launcher string) string {
		t.Helper()
		return replaceLauncherFragment(t, launcher,
			`  [ ! -e "${LAUNCHER_ROOT}" ]`,
			"  [ ! -e \"${LAUNCHER_ROOT}\" ]\n  return 1",
		)
	})
	process := startLauncherSupervisorFixture(t, fixture)
	waitErr := process.wait(t, 30*time.Second)
	if waitErr == nil {
		t.Fatalf("launcher reported success after its explicit cleanup failed\n%s", process.output.String())
	}
	if !strings.Contains(process.output.String(), "remove isolated acceptance launcher directory: failed") {
		t.Fatalf("launcher did not report its explicit cleanup failure\n%s", process.output.String())
	}
	launcherRootBytes, err := os.ReadFile(fixture.launcherRecord)
	if err != nil {
		t.Fatal(err)
	}
	launcherRoot := strings.TrimSpace(string(launcherRootBytes))
	if _, err := os.Stat(launcherRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("EXIT fallback left launcher root behind: %v", err)
	}
}

func newLauncherSupervisorFixture(t *testing.T, signalProbe bool, mutate func(*testing.T, string) string) launcherSupervisorFixture {
	t.Helper()
	fixtureRoot := t.TempDir()
	moduleRoot := filepath.Join(fixtureRoot, "module")
	launcherPath := filepath.Join(moduleRoot, "scripts", "acceptance", "m0-fresh-clone.sh")
	probePath := filepath.Join(moduleRoot, "tests", "acceptance", "launcher_supervisor", "probe_test.go")
	for _, path := range []string{filepath.Dir(launcherPath), filepath.Dir(probePath)} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	canonicalBytes, err := os.ReadFile(filepath.Join(repositoryRoot, "scripts", "acceptance", "m0-fresh-clone.sh"))
	if err != nil {
		t.Fatal(err)
	}
	launcher := string(canonicalBytes)
	launcher = replaceLauncherFragment(t, launcher,
		"LAUNCHER_ROOT=$(/usr/bin/mktemp -d /tmp/semlia-t008-launcher.XXXXXX) || {\n  printf '%s\\n' 'create isolated acceptance launcher directory: failed' >&2\n  exit 1\n}\n",
		"LAUNCHER_ROOT=$(/usr/bin/mktemp -d /tmp/semlia-t008-launcher.XXXXXX) || {\n  printf '%s\\n' 'create isolated acceptance launcher directory: failed' >&2\n  exit 1\n}\nif [ -n \"${SEMLIA_TEST_LAUNCHER_ROOT_RECORD:-}\" ]; then\n  printf '%s\\n' \"${LAUNCHER_ROOT}\" >\"${SEMLIA_TEST_LAUNCHER_ROOT_RECORD}\" || exit 1\nfi\n",
	)
	launcher = replaceLauncherFragment(t, launcher,
		"CHILD_PID=$!",
		"CHILD_PID=$!\nif [ -n \"${SEMLIA_TEST_CHILD_PID_RECORD:-}\" ]; then\n  printf '%s\\n' \"${CHILD_PID}\" >\"${SEMLIA_TEST_CHILD_PID_RECORD}\" || exit 1\nfi",
	)
	launcher = replaceLauncherFragment(t, launcher,
		"esac\nOLD_IFS=${IFS}",
		"esac\nEXPECTED_GO_CANDIDATES="+launcherPhysicalGoTool(t)+"\nOLD_IFS=${IFS}",
	)
	launcher = replaceLauncherFragment(t, launcher,
		"  \"SEMLIA_ACCEPTANCE_REF=${ACCEPTANCE_REF}\" \\\n",
		"  \"SEMLIA_ACCEPTANCE_REF=${ACCEPTANCE_REF}\" \\\n  \"SEMLIA_TEST_EXPECTED_PATH=${TRUSTED_PATH}\" \\\n",
	)
	launcher = replaceLauncherFragment(t, launcher,
		`"GOTOOLCHAIN=go${PINNED_GO_VERSION}"`,
		`"GOTOOLCHAIN=local"`,
	)
	if mutate != nil {
		launcher = mutate(t, launcher)
	}
	if err := os.WriteFile(launcherPath, []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
	probe, err := os.ReadFile(filepath.Join(repositoryRoot, "tests", "acceptance", "testdata", "launcher_supervisor", "probe_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !signalProbe {
		probe = []byte("package launcher_supervisor\n\nimport (\"testing\"; \"time\")\n\nfunc TestFreshCloneAcceptance(t *testing.T) { time.Sleep(100 * time.Millisecond) }\n")
	}
	if err := os.WriteFile(probePath, probe, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte("module launcherprobe\n\ngo 1.26.0\n\ntoolchain go1.26.5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	poisonMarker := filepath.Join(fixtureRoot, "ambient-bash-startup-sourced")
	poisonPath := filepath.Join(fixtureRoot, "ambient-bash-startup")
	if err := os.WriteFile(poisonPath, []byte("printf poison >\""+poisonMarker+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return launcherSupervisorFixture{
		launcher:       launcherPath,
		moduleRoot:     moduleRoot,
		launcherRecord: filepath.Join(fixtureRoot, "launcher-root"),
		childRecord:    filepath.Join(fixtureRoot, "child-pid"),
		trapReady:      filepath.Join(fixtureRoot, "trap-ready"),
		rootReady:      filepath.Join(fixtureRoot, "root-ready"),
		poisonMarker:   poisonMarker,
	}
}

func launcherPhysicalGoTool(t *testing.T) string {
	t.Helper()
	tool := filepath.Join(runtime.GOROOT(), "bin", "go")
	if info, err := os.Stat(tool); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("resolve physical Go 1.26.5 tool %s: %v", tool, err)
	}
	return tool
}

func replaceLauncherFragment(t *testing.T, launcher, old, replacement string) string {
	t.Helper()
	if strings.Count(launcher, old) != 1 {
		t.Fatalf("launcher fixture fragment count for %q = %d, want 1", old, strings.Count(launcher, old))
	}
	return strings.Replace(launcher, old, replacement, 1)
}

type launcherFixtureProcess struct {
	command *exec.Cmd
	output  bytes.Buffer
	waited  <-chan error
	reaped  bool
	fixture launcherSupervisorFixture
}

func startLauncherSupervisorFixture(t *testing.T, fixture launcherSupervisorFixture) *launcherFixtureProcess {
	t.Helper()
	command := exec.Command(fixture.launcher)
	command.Env = append(
		filteredEnvironment(os.Environ(), "BASH_ENV", "ENV", "SHELLOPTS", "BASHOPTS", "SEMLIA_TEST_LAUNCHER_ROOT_RECORD", "SEMLIA_TEST_CHILD_PID_RECORD", "SEMLIA_TEST_TRAP_READY", "SEMLIA_TEST_ROOT_READY"),
		"BASH_ENV="+filepath.Join(filepath.Dir(fixture.poisonMarker), "ambient-bash-startup"),
		"ENV="+filepath.Join(filepath.Dir(fixture.poisonMarker), "ambient-bash-startup"),
		"BASH_FUNC_exit%%=() { /usr/bin/touch '"+fixture.poisonMarker+"'; return 0; }",
		"BASH_FUNC_printf%%=() { /usr/bin/touch '"+fixture.poisonMarker+"'; return 0; }",
		"BASH_FUNC_trap%%=() { /usr/bin/touch '"+fixture.poisonMarker+"'; return 0; }",
		"BASH_FUNC_wait%%=() { /usr/bin/touch '"+fixture.poisonMarker+"'; return 0; }",
		"BASH_FUNC_set%%=() { /usr/bin/touch '"+fixture.poisonMarker+"'; return 0; }",
		"SEMLIA_ACCEPTANCE_SOURCE="+fixture.moduleRoot,
		"SEMLIA_ACCEPTANCE_REF="+strings.Repeat("a", 40),
		"SEMLIA_TEST_LAUNCHER_ROOT_RECORD="+fixture.launcherRecord,
		"SEMLIA_TEST_CHILD_PID_RECORD="+fixture.childRecord,
		"SEMLIA_TEST_TRAP_READY="+fixture.trapReady,
		"SEMLIA_TEST_ROOT_READY="+fixture.rootReady,
	)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	process := &launcherFixtureProcess{command: command, fixture: fixture}
	command.Stdout = &process.output
	command.Stderr = &process.output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	process.waited = waited
	t.Cleanup(func() {
		if process.reaped {
			return
		}
		killLauncherFixture(command.Process.Pid, fixture.childRecord)
		select {
		case <-waited:
			process.reaped = true
		case <-time.After(5 * time.Second):
		}
	})
	return process
}

func (process *launcherFixtureProcess) wait(t *testing.T, timeout time.Duration) error {
	t.Helper()
	select {
	case err := <-process.waited:
		process.reaped = true
		return err
	case <-time.After(timeout):
		killLauncherFixture(process.command.Process.Pid, process.fixture.childRecord)
		select {
		case <-process.waited:
			process.reaped = true
		case <-time.After(5 * time.Second):
		}
		t.Fatalf("launcher did not exit within %s\n%s", timeout, process.output.String())
		return nil
	}
}

func killLauncherFixture(wrapperPID int, childRecord string) {
	_ = syscall.Kill(-wrapperPID, syscall.SIGKILL)
	childBytes, err := os.ReadFile(childRecord)
	if err != nil {
		return
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(string(childBytes)))
	if err == nil && childPID > 0 {
		_ = syscall.Kill(-childPID, syscall.SIGKILL)
	}
}
