//go:build darwin || linux

package acceptance

import (
	"bytes"
	"errors"
	"fmt"
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

func TestFreshCloneLauncherTimesOutHangingNodeProcessGroup(t *testing.T) {
	fixture := newLauncherSupervisorFixture(t, false, nil)
	parentRecord, descendantRecord := installHangingLauncherNode(t, fixture)
	setLauncherPreflightBudgets(t, fixture.launcher, 5, 20)

	started := time.Now()
	process := startLauncherSupervisorFixture(t, fixture)
	waitForPath(t, parentRecord, 10*time.Second)
	waitForPath(t, descendantRecord, 10*time.Second)
	waitErr := process.wait(t, 6*time.Second)
	if waitErr == nil {
		t.Fatalf("launcher accepted a timed-out Node.js version probe\n%s", process.output.String())
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("launcher exceeded the 5-second Node.js phase budget: %s\n%s", elapsed, process.output.String())
	}
	output := process.output.String()
	for _, want := range []string{"Node.js version probe", "5-second phase budget", "Node.js version probe stdout", "hanging node parent", "Node.js version probe stderr", "hanging node descendant"} {
		if !strings.Contains(output, want) {
			t.Errorf("launcher timeout output missing %q\n%s", want, output)
		}
	}
	assertRecordedProcessExited(t, parentRecord)
	assertRecordedProcessExited(t, descendantRecord)
	assertLauncherRootRemoved(t, fixture)
}

func TestFreshCloneLauncherTimesOutHangingCAExportProcessGroup(t *testing.T) {
	fixture := newLauncherSupervisorFixture(t, false, nil)
	parentRecord, descendantRecord := installHangingLauncherCAExport(t, fixture)
	setLauncherPreflightBudgets(t, fixture.launcher, 5, 20)

	started := time.Now()
	process := startLauncherSupervisorFixture(t, fixture)
	waitForPath(t, parentRecord, 10*time.Second)
	waitForPath(t, descendantRecord, 10*time.Second)
	waitErr := process.wait(t, 6*time.Second)
	if waitErr == nil {
		t.Fatalf("launcher accepted a timed-out operating-system CA export\n%s", process.output.String())
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("launcher exceeded the 5-second CA export phase budget: %s\n%s", elapsed, process.output.String())
	}
	output := process.output.String()
	for _, want := range []string{"operating-system CA export", "5-second phase budget", "operating-system CA export stdout", "hanging ca parent", "operating-system CA export stderr", "hanging ca descendant"} {
		if !strings.Contains(output, want) {
			t.Errorf("launcher CA timeout output missing %q\n%s", want, output)
		}
	}
	assertRecordedProcessExited(t, parentRecord)
	assertRecordedProcessExited(t, descendantRecord)
	assertLauncherRootRemoved(t, fixture)
}

func TestFreshCloneLauncherTimesOutHangingGoProcessGroup(t *testing.T) {
	fixture := newLauncherSupervisorFixture(t, false, nil)
	parentRecord, descendantRecord := installHangingLauncherGo(t, fixture)
	setLauncherPreflightBudgets(t, fixture.launcher, 5, 20)

	started := time.Now()
	process := startLauncherSupervisorFixture(t, fixture)
	waitForPath(t, parentRecord, 10*time.Second)
	waitForPath(t, descendantRecord, 10*time.Second)
	waitErr := process.wait(t, 6*time.Second)
	if waitErr == nil {
		t.Fatalf("launcher accepted a timed-out Go version probe\n%s", process.output.String())
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("launcher exceeded the 5-second Go phase budget: %s\n%s", elapsed, process.output.String())
	}
	output := process.output.String()
	for _, want := range []string{"Go version probe", "5-second phase budget", "Go version probe stdout", "hanging go parent", "Go version probe stderr", "hanging go descendant"} {
		if !strings.Contains(output, want) {
			t.Errorf("launcher Go timeout output missing %q\n%s", want, output)
		}
	}
	assertRecordedProcessExited(t, parentRecord)
	assertRecordedProcessExited(t, descendantRecord)
	assertLauncherRootRemoved(t, fixture)
}

func TestFreshCloneLauncherBoundsCumulativeNodeCandidates(t *testing.T) {
	fixture := newLauncherSupervisorFixture(t, false, nil)
	records, descendantRecord := installCumulativeLauncherNodeCandidates(t, fixture)
	setLauncherPreflightBudgets(t, fixture.launcher, 5, 20)

	started := time.Now()
	process := startLauncherSupervisorFixture(t, fixture)
	for _, record := range records[:1] {
		waitForPath(t, record, 10*time.Second)
	}
	waitErr := process.wait(t, 7*time.Second)
	if waitErr == nil {
		t.Fatalf("launcher accepted cumulative Node candidates beyond the phase budget\n%s", process.output.String())
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("cumulative Node candidates exceeded the 5-second phase budget: %s\n%s", elapsed, process.output.String())
	}
	output := process.output.String()
	if !strings.Contains(output, "Node.js version probe") || (!strings.Contains(output, "remaining launcher preflight budget") && !strings.Contains(output, "5-second phase budget")) {
		t.Fatalf("cumulative Node phase failure was not actionable\n%s", process.output.String())
	}
	if _, err := os.Stat(records[len(records)-1]); err == nil {
		assertRecordedProcessExited(t, records[len(records)-1])
		assertRecordedProcessExited(t, descendantRecord)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	assertLauncherRootRemoved(t, fixture)
}

func TestFreshCloneLauncherBoundsAggregatePreflight(t *testing.T) {
	fixture := newLauncherSupervisorFixture(t, false, nil)
	goParent, goDescendant := installSlowAggregatePreflight(t, fixture)
	setLauncherPreflightBudgets(t, fixture.launcher, 6, 7)

	started := time.Now()
	process := startLauncherSupervisorFixture(t, fixture)
	waitForPath(t, goParent, 10*time.Second)
	waitForPath(t, goDescendant, 10*time.Second)
	waitErr := process.wait(t, 9*time.Second)
	if waitErr == nil {
		t.Fatalf("launcher accepted preflight work beyond the total budget\n%s", process.output.String())
	}
	if elapsed := time.Since(started); elapsed > 7*time.Second {
		t.Fatalf("launcher exceeded the 7-second total preflight budget: %s\n%s", elapsed, process.output.String())
	}
	if !strings.Contains(process.output.String(), "7-second total preflight budget") {
		t.Fatalf("aggregate preflight failure was not actionable\n%s", process.output.String())
	}
	assertRecordedProcessExited(t, goParent)
	assertRecordedProcessExited(t, goDescendant)
	assertLauncherRootRemoved(t, fixture)
}

func TestFreshCloneLauncherBoundsOneSecondRemainingBudget(t *testing.T) {
	fixture := newLauncherSupervisorFixture(t, false, nil)
	parentRecord, descendantRecord := installHangingLauncherNode(t, fixture)
	setLauncherPreflightBudgets(t, fixture.launcher, 15, 45)
	launcherBytes, err := os.ReadFile(fixture.launcher)
	if err != nil {
		t.Fatal(err)
	}
	launcher := replaceLauncherFragment(t, string(launcherBytes),
		"  NOW=$(/bin/date +%s) || return 1",
		"  NOW=$((PHASE_DEADLINE - 1))",
	)
	launcher = replaceLauncherFragment(t, launcher,
		"  ACTIVE_PID=$!\n  if ! /bin/kill -0",
		"  ACTIVE_PID=$!\n  while [ ! -e \""+parentRecord+"\" ]; do /bin/sleep 0.01; done\n  if ! /bin/kill -0",
	)
	if err := os.WriteFile(fixture.launcher, []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	process := startLauncherSupervisorFixture(t, fixture)
	waitErr := process.wait(t, 3*time.Second)
	var exitError *exec.ExitError
	if !errors.As(waitErr, &exitError) || exitError.ExitCode() != 124 {
		t.Fatalf("launcher exit = %v, want bounded-preflight status 124\n%s", waitErr, process.output.String())
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("launcher exceeded its one-second hard preflight budget: %s\n%s", elapsed, process.output.String())
	}
	if !strings.Contains(process.output.String(), "remaining launcher preflight budget") {
		t.Fatalf("near-deadline rejection was not actionable\n%s", process.output.String())
	}
	for _, record := range []string{parentRecord, descendantRecord} {
		if _, err := os.Stat(record); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("near-deadline launcher started command record %s: %v", record, err)
		}
	}
	assertLauncherRootRemoved(t, fixture)
}

func TestFreshCloneLauncherSignalDuringNodePreflightKillsProcessGroup(t *testing.T) {
	fixture := newLauncherSupervisorFixture(t, false, nil)
	parentRecord, descendantRecord := installHangingLauncherNode(t, fixture)
	process := startLauncherSupervisorFixture(t, fixture)
	waitForPath(t, parentRecord, 10*time.Second)
	waitForPath(t, descendantRecord, 10*time.Second)

	started := time.Now()
	if err := process.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waitErr := process.wait(t, 5*time.Second)
	var exitError *exec.ExitError
	if !errors.As(waitErr, &exitError) || exitError.ExitCode() != 143 {
		t.Fatalf("launcher exit = %v, want preflight signal status 143\n%s", waitErr, process.output.String())
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("launcher took %s to stop a signalled preflight group\n%s", elapsed, process.output.String())
	}
	assertRecordedProcessExited(t, parentRecord)
	assertRecordedProcessExited(t, descendantRecord)
	assertLauncherRootRemoved(t, fixture)
}

func TestFreshCloneLauncherDeliversPendingSignalAfterPreflightStart(t *testing.T) {
	fixture := newLauncherSupervisorFixture(t, false, nil)
	parentRecord, descendantRecord := installHangingLauncherNode(t, fixture)
	assignmentGap := filepath.Join(filepath.Dir(fixture.moduleRoot), "preflight-assignment-gap")
	launcherBytes, err := os.ReadFile(fixture.launcher)
	if err != nil {
		t.Fatal(err)
	}
	launcher := replaceLauncherFragment(t, string(launcherBytes),
		"  ACTIVE_PID=$!\n  if ! /bin/kill -0",
		"  printf ready >\""+assignmentGap+"\"\n  /bin/sleep 1\n  ACTIVE_PID=$!\n  if ! /bin/kill -0",
	)
	if err := os.WriteFile(fixture.launcher, []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}

	process := startLauncherSupervisorFixture(t, fixture)
	waitForPath(t, assignmentGap, 10*time.Second)
	waitForPath(t, parentRecord, 10*time.Second)
	waitForPath(t, descendantRecord, 10*time.Second)
	if err := process.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waitErr := process.wait(t, 5*time.Second)
	var exitError *exec.ExitError
	if !errors.As(waitErr, &exitError) || exitError.ExitCode() != 143 {
		t.Fatalf("launcher exit = %v, want pending preflight signal status 143\n%s", waitErr, process.output.String())
	}
	assertRecordedProcessExited(t, parentRecord)
	assertRecordedProcessExited(t, descendantRecord)
	assertLauncherRootRemoved(t, fixture)
}

func installHangingLauncherNode(t *testing.T, fixture launcherSupervisorFixture) (string, string) {
	t.Helper()
	toolRoot := filepath.Join(filepath.Dir(fixture.moduleRoot), "hanging-node-bin")
	if err := os.MkdirAll(toolRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	parentRecord := filepath.Join(filepath.Dir(fixture.moduleRoot), "node-parent-pid")
	descendantRecord := filepath.Join(filepath.Dir(fixture.moduleRoot), "node-descendant-pid")
	script := "#!/bin/bash\n" +
		"printf '%s\\n' \"$$\" >'" + parentRecord + "'\n" +
		"printf '%s\\n' 'hanging node parent'\n" +
		"/bin/bash -c 'trap \"\" TERM; printf \"%s\\\\n\" \"$$\" >\"$1\"; printf \"%s\\\\n\" \"hanging node descendant\" >&2; /bin/sleep 300' child '" + descendantRecord + "' &\n" +
		"trap '' TERM\n" +
		"wait\n"
	if err := os.WriteFile(filepath.Join(toolRoot, "node"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	launcherBytes, err := os.ReadFile(fixture.launcher)
	if err != nil {
		t.Fatal(err)
	}
	launcher := replaceLauncherFragment(t, string(launcherBytes), "NODE_BIN=\n", "TRUSTED_PATH="+toolRoot+"\nNODE_BIN=\n")
	if err := os.WriteFile(fixture.launcher, []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
	return parentRecord, descendantRecord
}

func installHangingLauncherCAExport(t *testing.T, fixture launcherSupervisorFixture) (string, string) {
	t.Helper()
	toolRoot := filepath.Join(filepath.Dir(fixture.moduleRoot), "hanging-ca-node-bin")
	if err := os.MkdirAll(toolRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	parentRecord := filepath.Join(filepath.Dir(fixture.moduleRoot), "ca-parent-pid")
	descendantRecord := filepath.Join(filepath.Dir(fixture.moduleRoot), "ca-descendant-pid")
	script := "#!/bin/bash\n" +
		"if [ \"${1:-}\" = --version ]; then printf '%s\\n' v24.15.0; exit 0; fi\n" +
		hangingToolBody(parentRecord, descendantRecord, "hanging ca")
	if err := os.WriteFile(filepath.Join(toolRoot, "node"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	prependLauncherTrustedPath(t, fixture.launcher, toolRoot)
	return parentRecord, descendantRecord
}

func installHangingLauncherGo(t *testing.T, fixture launcherSupervisorFixture) (string, string) {
	t.Helper()
	toolRoot := filepath.Join(filepath.Dir(fixture.moduleRoot), "hanging-go-bin")
	if err := os.MkdirAll(toolRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	certificate := filepath.Join(toolRoot, "certificate.pem")
	if err := os.WriteFile(certificate, testCertificatePEM(t), 0o600); err != nil {
		t.Fatal(err)
	}
	nodeScript := "#!/bin/bash\n" +
		"if [ \"${1:-}\" = --version ]; then printf '%s\\n' v24.15.0; else /bin/cat '" + certificate + "'; fi\n"
	if err := os.WriteFile(filepath.Join(toolRoot, "node"), []byte(nodeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	parentRecord := filepath.Join(filepath.Dir(fixture.moduleRoot), "go-parent-pid")
	descendantRecord := filepath.Join(filepath.Dir(fixture.moduleRoot), "go-descendant-pid")
	goScript := "#!/bin/bash\n" + hangingToolBody(parentRecord, descendantRecord, "hanging go")
	goPath := filepath.Join(toolRoot, "go")
	if err := os.WriteFile(goPath, []byte(goScript), 0o700); err != nil {
		t.Fatal(err)
	}
	prependLauncherTrustedPath(t, fixture.launcher, toolRoot)
	replaceLauncherGoCandidate(t, fixture.launcher, goPath)
	return parentRecord, descendantRecord
}

func installCumulativeLauncherNodeCandidates(t *testing.T, fixture launcherSupervisorFixture) ([]string, string) {
	t.Helper()
	fixtureRoot := filepath.Dir(fixture.moduleRoot)
	var toolRoots []string
	var records []string
	descendantRecord := filepath.Join(fixtureRoot, "cumulative-node-descendant-pid")
	for index := 1; index <= 3; index++ {
		toolRoot := filepath.Join(fixtureRoot, fmt.Sprintf("cumulative-node-%d", index))
		if err := os.MkdirAll(toolRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		record := filepath.Join(fixtureRoot, fmt.Sprintf("cumulative-node-%d-pid", index))
		var script string
		if index < 3 {
			script = "#!/bin/bash\nprintf '%s\\n' \"$$\" >'" + record + "'\n/bin/sleep 1\nprintf '%s\\n' v0.0." + strconv.Itoa(index) + "\n"
		} else {
			script = "#!/bin/bash\n" + hangingToolBody(record, descendantRecord, "cumulative node")
		}
		if err := os.WriteFile(filepath.Join(toolRoot, "node"), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		toolRoots = append(toolRoots, toolRoot)
		records = append(records, record)
	}
	setLauncherTrustedPath(t, fixture.launcher, strings.Join(toolRoots, ":"))
	return records, descendantRecord
}

func installSlowAggregatePreflight(t *testing.T, fixture launcherSupervisorFixture) (string, string) {
	t.Helper()
	toolRoot := filepath.Join(filepath.Dir(fixture.moduleRoot), "aggregate-preflight-bin")
	if err := os.MkdirAll(toolRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	certificate := filepath.Join(toolRoot, "certificate.pem")
	if err := os.WriteFile(certificate, testCertificatePEM(t), 0o600); err != nil {
		t.Fatal(err)
	}
	nodeScript := "#!/bin/bash\n" +
		"/bin/sleep 1\n" +
		"if [ \"${1:-}\" = --version ]; then printf '%s\\n' v24.15.0; else /bin/cat '" + certificate + "'; fi\n"
	if err := os.WriteFile(filepath.Join(toolRoot, "node"), []byte(nodeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	parentRecord := filepath.Join(filepath.Dir(fixture.moduleRoot), "aggregate-go-parent-pid")
	descendantRecord := filepath.Join(filepath.Dir(fixture.moduleRoot), "aggregate-go-descendant-pid")
	goPath := filepath.Join(toolRoot, "go")
	if err := os.WriteFile(goPath, []byte("#!/bin/bash\n"+hangingToolBody(parentRecord, descendantRecord, "aggregate go")), 0o700); err != nil {
		t.Fatal(err)
	}
	prependLauncherTrustedPath(t, fixture.launcher, toolRoot)
	replaceLauncherGoCandidate(t, fixture.launcher, goPath)
	return parentRecord, descendantRecord
}

func hangingToolBody(parentRecord, descendantRecord, label string) string {
	return "printf '%s\\n' \"$$\" >'" + parentRecord + "'\n" +
		"printf '%s\\n' '" + label + " parent'\n" +
		"/bin/bash -c 'trap \"\" TERM; printf \"%s\\\\n\" \"$$\" >\"$1\"; printf \"%s\\\\n\" \"$2 descendant\" >&2; /bin/sleep 300' child '" + descendantRecord + "' '" + label + "' &\n" +
		"trap '' TERM\n" +
		"wait\n"
}

func prependLauncherTrustedPath(t *testing.T, launcherPath, toolRoot string) {
	t.Helper()
	launcherBytes, err := os.ReadFile(launcherPath)
	if err != nil {
		t.Fatal(err)
	}
	launcher := replaceLauncherFragment(t, string(launcherBytes), "NODE_BIN=\n", "TRUSTED_PATH="+toolRoot+":${TRUSTED_PATH}\nNODE_BIN=\n")
	if err := os.WriteFile(launcherPath, []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
}

func setLauncherTrustedPath(t *testing.T, launcherPath, trustedPath string) {
	t.Helper()
	launcherBytes, err := os.ReadFile(launcherPath)
	if err != nil {
		t.Fatal(err)
	}
	launcher := replaceLauncherFragment(t, string(launcherBytes), "NODE_BIN=\n", "TRUSTED_PATH="+trustedPath+"\nNODE_BIN=\n")
	if err := os.WriteFile(launcherPath, []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
}

func replaceLauncherGoCandidate(t *testing.T, launcherPath, goPath string) {
	t.Helper()
	launcherBytes, err := os.ReadFile(launcherPath)
	if err != nil {
		t.Fatal(err)
	}
	launcher := replaceLauncherFragment(t, string(launcherBytes), "EXPECTED_GO_CANDIDATES="+launcherPhysicalGoTool(t), "EXPECTED_GO_CANDIDATES="+goPath)
	if err := os.WriteFile(launcherPath, []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
}

func setLauncherPreflightBudgets(t *testing.T, launcherPath string, phaseSeconds, totalSeconds int) {
	t.Helper()
	launcherBytes, err := os.ReadFile(launcherPath)
	if err != nil {
		t.Fatal(err)
	}
	launcher := replaceLauncherFragment(t, string(launcherBytes), "LAUNCHER_PREFLIGHT_TIMEOUT_SECONDS=15", "LAUNCHER_PREFLIGHT_TIMEOUT_SECONDS="+strconv.Itoa(phaseSeconds))
	launcher = replaceLauncherFragment(t, launcher, "LAUNCHER_PREFLIGHT_TOTAL_TIMEOUT_SECONDS=45", "LAUNCHER_PREFLIGHT_TOTAL_TIMEOUT_SECONDS="+strconv.Itoa(totalSeconds))
	if err := os.WriteFile(launcherPath, []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
}

func assertRecordedProcessExited(t *testing.T, record string) {
	t.Helper()
	pidBytes, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		t.Fatalf("parse recorded process %q: %v", pidBytes, err)
	}
	waitForProcessExit(t, pid, 3*time.Second)
}

func assertLauncherRootRemoved(t *testing.T, fixture launcherSupervisorFixture) {
	t.Helper()
	launcherRootBytes, err := os.ReadFile(fixture.launcherRecord)
	if err != nil {
		t.Fatal(err)
	}
	launcherRoot := strings.TrimSpace(string(launcherRootBytes))
	if _, err := os.Stat(launcherRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("launcher root remains after preflight exit: %v", err)
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
