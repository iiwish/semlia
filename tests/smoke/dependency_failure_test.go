package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	postgresFailureComposeQueryTimeout   = 10 * time.Second
	postgresFailureComposeControlTimeout = 30 * time.Second
	postgresFailureDegradedStatusTimeout = 30 * time.Second
	postgresRecoveryReadinessTimeout     = 90 * time.Second
	postgresFailureCleanupTimeout        = 45 * time.Second
	workerFailureObservationTimeout      = 30 * time.Second
	workerRecoveryTimeout                = 90 * time.Second
	workerRecoveryPollInterval           = 500 * time.Millisecond
	workerProbeTimeout                   = 10 * time.Second
	workerStableRunningChecks            = 5
	smokeCommandWaitDelay                = time.Second
	workerInspectFormat                  = `{"id":{{json .Id}},"running":{{json .State.Running}},"startedAt":{{json .State.StartedAt}},"restartCount":{{json .RestartCount}}}`
)

type workerContainerState struct {
	containerID  string
	running      bool
	startedAt    string
	restartCount int
}

type workerContainerProbe func(context.Context) (workerContainerState, error)

type postgresFailureCleanupOperations struct {
	startPostgres func(context.Context) error
	waitReady     func(context.Context) error
	waitWorker    func(context.Context) error
}

type errorReporter interface {
	Errorf(string, ...any)
}

type recordingErrorReporter struct {
	messages []string
}

func (reporter *recordingErrorReporter) Errorf(format string, args ...any) {
	reporter.messages = append(reporter.messages, fmt.Sprintf(format, args...))
}

func TestWaitForWorkerRecoveryWaitsForSameContainer(t *testing.T) {
	baseline := workerContainerState{
		containerID:  "worker-before",
		running:      true,
		startedAt:    "start-0",
		restartCount: 3,
	}
	states := []workerContainerState{
		{containerID: "worker-before", startedAt: "start-0", restartCount: 3},
		{containerID: "worker-before", running: true, startedAt: "start-1", restartCount: 4},
		{containerID: "worker-before", running: true, startedAt: "start-2", restartCount: 5},
		{containerID: "worker-before", running: true, startedAt: "start-2", restartCount: 5},
		{containerID: "worker-before", running: true, startedAt: "start-2", restartCount: 5},
		{containerID: "worker-before", running: true, startedAt: "start-2", restartCount: 5},
		{containerID: "worker-before", running: true, startedAt: "start-2", restartCount: 5},
	}
	retry := make(chan time.Time, len(states)-1)
	for range len(states) - 1 {
		retry <- time.Time{}
	}
	probes := 0

	err := waitForWorkerRecovery(context.Background(), baseline, retry, func(context.Context) (workerContainerState, error) {
		state := states[probes]
		probes++
		return state, nil
	})
	if err != nil {
		t.Fatalf("waitForWorkerRecovery() error = %v", err)
	}
	if probes != len(states) {
		t.Fatalf("probe count = %d, want %d", probes, len(states))
	}
}

func TestWaitForWorkerRestartRequiresNewGeneration(t *testing.T) {
	baseline := workerContainerState{
		containerID:  "worker-before",
		running:      true,
		startedAt:    "start-0",
		restartCount: 3,
	}
	states := []workerContainerState{
		{containerID: "worker-before", startedAt: "start-0", restartCount: 3},
		{containerID: "worker-before", running: true, startedAt: "start-1", restartCount: 4},
	}
	retry := make(chan time.Time, len(states)-1)
	retry <- time.Time{}
	probes := 0

	state, err := waitForWorkerRestart(context.Background(), baseline, retry, func(context.Context) (workerContainerState, error) {
		state := states[probes]
		probes++
		return state, nil
	})
	if err != nil {
		t.Fatalf("waitForWorkerRestart() error = %v", err)
	}
	if state.restartCount != 4 || state.startedAt != "start-1" {
		t.Fatalf("observed worker generation = %+v, want restartCount=4 startedAt=start-1", state)
	}
}

func TestWaitForWorkerRecoveryRejectsReplacement(t *testing.T) {
	baseline := workerContainerState{containerID: "worker-before", running: true, startedAt: "start-0", restartCount: 3}
	err := waitForWorkerRecovery(context.Background(), baseline, nil, func(context.Context) (workerContainerState, error) {
		return workerContainerState{containerID: "worker-after", running: true, startedAt: "start-1", restartCount: 4}, nil
	})
	if err == nil || !strings.Contains(err.Error(), `before="worker-before" after="worker-after"`) {
		t.Fatalf("waitForWorkerRecovery() error = %v, want container replacement details", err)
	}
}

func TestWaitForWorkerRecoveryRejectsStoppedBaseline(t *testing.T) {
	baseline := workerContainerState{containerID: "worker-before", startedAt: "start-0", restartCount: 3}
	err := waitForWorkerRecovery(context.Background(), baseline, nil, func(context.Context) (workerContainerState, error) {
		return baseline, nil
	})
	if err == nil || !strings.Contains(err.Error(), "not running before failure") {
		t.Fatalf("waitForWorkerRecovery() error = %v, want invalid baseline", err)
	}
}

func TestWaitForWorkerRecoveryIsBounded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	baseline := workerContainerState{containerID: "worker-before", running: true, startedAt: "start-0", restartCount: 3}
	err := waitForWorkerRecovery(ctx, baseline, nil, func(context.Context) (workerContainerState, error) {
		return baseline, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForWorkerRecovery() error = %v, want context cancellation", err)
	}
	if !strings.Contains(err.Error(), `restartCount=3`) {
		t.Fatalf("waitForWorkerRecovery() error = %v, want last observed generation", err)
	}
}

func TestWaitForWorkerStableAcceptsBaselineGeneration(t *testing.T) {
	baseline := workerContainerState{containerID: "worker-before", running: true, startedAt: "start-0", restartCount: 3}
	retry := make(chan time.Time, workerStableRunningChecks-1)
	for range workerStableRunningChecks - 1 {
		retry <- time.Time{}
	}
	err := waitForWorkerStable(context.Background(), baseline, retry, func(context.Context) (workerContainerState, error) {
		return baseline, nil
	})
	if err != nil {
		t.Fatalf("waitForWorkerStable() error = %v", err)
	}
}

func TestDependencyFailureBudgets(t *testing.T) {
	if postgresFailureCleanupTimeout > 45*time.Second {
		t.Fatalf("postgres cleanup timeout = %s, want at most 45s", postgresFailureCleanupTimeout)
	}
	if workerFailureObservationTimeout > 30*time.Second {
		t.Fatalf("worker failure observation timeout = %s, want at most 30s", workerFailureObservationTimeout)
	}
	if workerRecoveryTimeout > 90*time.Second {
		t.Fatalf("worker recovery timeout = %s, want at most 90s", workerRecoveryTimeout)
	}
	stableWindow := time.Duration(workerStableRunningChecks-1) * workerRecoveryPollInterval
	if stableWindow < 2*time.Second || stableWindow > 5*time.Second {
		t.Fatalf("worker stable running window = %s, want 2s..5s", stableWindow)
	}
	mainJourney := postgresFailureMainJourneyWorstCaseBudget()
	if mainJourney != 6*time.Minute+50*time.Second {
		t.Fatalf("PostgreSQL failure main-journey budget = %s, want 6m50s from the bounded operations", mainJourney)
	}
	worstCase := postgresFailureJourneyWorstCaseBudget()
	if worstCase != mainJourney+postgresFailureCleanupTimeout {
		t.Fatalf("PostgreSQL failure total budget = %s, want main %s + cleanup %s", worstCase, mainJourney, postgresFailureCleanupTimeout)
	}
	if worstCase >= smokeProcessTimeout {
		t.Fatalf("PostgreSQL failure journey budget = %s, must be below %s", worstCase, smokeProcessTimeout)
	}
	if margin := smokeProcessTimeout - worstCase; margin < 2*time.Minute {
		t.Fatalf("PostgreSQL failure journey margin = %s, want at least 2m", margin)
	}
}

func postgresFailureJourneyWorstCaseBudget() time.Duration {
	return postgresFailureMainJourneyWorstCaseBudget() + postgresFailureCleanupTimeout
}

func postgresFailureMainJourneyWorstCaseBudget() time.Duration {
	const (
		directHTTPCalls     = 4  // Baseline ready/info plus degraded root/ready.
		composeQueries      = 4  // Baseline services, two initial IDs, and recovered PostgreSQL ID.
		composeControls     = 2  // PostgreSQL stop and start.
		boundedCommandExits = 10 // Queries, controls, baseline/probe exits, and the shared cleanup exit.
		degradedStatusWaits = 2
	)
	return directHTTPCalls*smokeHTTPClientTimeout +
		composeQueries*postgresFailureComposeQueryTimeout +
		composeControls*postgresFailureComposeControlTimeout +
		workerProbeTimeout + // Initial atomic worker inspection.
		degradedStatusWaits*postgresFailureDegradedStatusTimeout +
		postgresRecoveryReadinessTimeout +
		workerFailureObservationTimeout +
		workerRecoveryTimeout +
		boundedCommandExits*smokeCommandWaitDelay
}

func TestRestorePostgresFailureAggregatesEveryError(t *testing.T) {
	startErr := errors.New("start failed")
	readyErr := errors.New("ready failed")
	workerErr := errors.New("worker failed")
	called := make([]string, 0, 3)
	ctx := context.Background()
	err := restorePostgresFailure(ctx, postgresFailureCleanupOperations{
		startPostgres: func(got context.Context) error {
			if got != ctx {
				t.Error("start received a different cleanup context")
			}
			called = append(called, "start")
			return startErr
		},
		waitReady: func(got context.Context) error {
			if got != ctx {
				t.Error("readiness received a different cleanup context")
			}
			called = append(called, "ready")
			return readyErr
		},
		waitWorker: func(got context.Context) error {
			if got != ctx {
				t.Error("worker received a different cleanup context")
			}
			called = append(called, "worker")
			return workerErr
		},
	})
	if got := strings.Join(called, ","); got != "start,ready,worker" {
		t.Fatalf("cleanup operations = %q, want every operation in order", got)
	}
	for _, want := range []string{"start postgres: start failed", "wait for readiness: ready failed", "wait for worker recovery: worker failed"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("restorePostgresFailure() error = %v, want %q", err, want)
		}
	}
}

func TestCleanupErrorIsReported(t *testing.T) {
	reporter := &recordingErrorReporter{}
	reportPostgresFailureCleanup(reporter, errors.New("restore failed"))
	if len(reporter.messages) != 1 || !strings.Contains(reporter.messages[0], "restore failed") {
		t.Fatalf("cleanup reports = %v, want restore failure", reporter.messages)
	}
}

func TestFailureJourneyRequiresRemainingDeadline(t *testing.T) {
	if err := requireFailureJourneyDeadline(time.Now().Add(postgresFailureJourneyRequiredRemaining() - time.Second)); err == nil {
		t.Fatal("deadline guard accepted insufficient remaining process time")
	}
	if err := requireFailureJourneyDeadline(time.Now().Add(postgresFailureJourneyRequiredRemaining() + time.Minute)); err != nil {
		t.Fatalf("deadline guard rejected sufficient process time: %v", err)
	}
}

func restorePostgresFailure(ctx context.Context, operations postgresFailureCleanupOperations) error {
	var problems []error
	if err := operations.startPostgres(ctx); err != nil {
		problems = append(problems, fmt.Errorf("start postgres: %w", err))
	}
	if err := operations.waitReady(ctx); err != nil {
		problems = append(problems, fmt.Errorf("wait for readiness: %w", err))
	}
	if err := operations.waitWorker(ctx); err != nil {
		problems = append(problems, fmt.Errorf("wait for worker recovery: %w", err))
	}
	return errors.Join(problems...)
}

func reportPostgresFailureCleanup(reporter errorReporter, err error) {
	if err != nil {
		reporter.Errorf("restore PostgreSQL failure journey: %v", err)
	}
}

func TestDecodeWorkerContainerState(t *testing.T) {
	output := `{"id":"worker-id","running":true,"startedAt":"2026-08-13T06:00:00Z","restartCount":7}`
	state, err := decodeWorkerContainerState(output)
	if err != nil {
		t.Fatalf("decodeWorkerContainerState() error = %v", err)
	}
	want := workerContainerState{
		containerID:  "worker-id",
		running:      true,
		startedAt:    "2026-08-13T06:00:00Z",
		restartCount: 7,
	}
	if state != want {
		t.Fatalf("decodeWorkerContainerState() = %+v, want %+v", state, want)
	}
}

func TestCommandOutputSeparatesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the supported macOS/Linux probe uses /bin/sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := commandOutput(ctx, repositoryRoot(t), "/bin/sh", "-c", `printf 'worker-id\n'; printf 'warning\n' >&2`)
	if err != nil {
		t.Fatalf("commandOutput() error = %v", err)
	}
	if output != "worker-id\n" {
		t.Fatalf("commandOutput() = %q, want stdout only", output)
	}
}

func TestCommandOutputCancellationIsBounded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the supported macOS/Linux probe uses /bin/sleep")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := commandOutput(ctx, repositoryRoot(t), "/bin/sleep", "30")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("commandOutput() error = %v, want deadline exceeded", err)
	}
	for _, want := range []string{"/bin/sleep 30", `stdout=""`, `stderr=""`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("commandOutput() error = %v, want diagnostic %q", err, want)
		}
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("cancelled command returned after %s, want at most 2s", elapsed)
	}
}

func TestComposeCommandUsesDirectDockerCLI(t *testing.T) {
	root := repositoryRoot(t)
	name, args := composeCommand(root, "ps", "--all")
	if name != "docker" {
		t.Fatalf("compose executable = %q, want docker", name)
	}
	wantPrefix := []string{"compose", "--env-file", filepath.Join(root, ".semlia", "dev.env"), "ps", "--all"}
	if strings.Join(args, "\x00") != strings.Join(wantPrefix, "\x00") {
		t.Fatalf("compose arguments = %q, want %q", args, wantPrefix)
	}
}

func testPostgresFailureAndRecovery(t *testing.T) {
	root := repositoryRoot(t)
	deadline, ok := t.Deadline()
	if !ok {
		t.Fatal("PostgreSQL failure journey requires the Makefile smoke process deadline")
	}
	if err := requireFailureJourneyDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	requireSemliaBaseline(t, root)
	postgresContainerID := requireComposeContainerID(t, root, "postgres")
	workerContainerID := requireComposeContainerID(t, root, "worker")
	workerBaseline := requireWorkerContainerState(t, root, workerContainerID)
	if err := validateWorkerBaseline(workerBaseline); err != nil {
		t.Fatalf("invalid worker baseline: %v", err)
	}
	recoveryComplete := false
	t.Cleanup(func() {
		if !recoveryComplete {
			reportPostgresFailureCleanup(t, cleanupPostgresFailure(root, workerBaseline))
		}
	})
	requireComposeOutput(t, root, postgresFailureComposeControlTimeout, "stop", "postgres")

	waitForStatus(t, "/health/live", http.StatusOK, postgresFailureDegradedStatusTimeout)
	waitForStatus(t, "/health/ready", http.StatusServiceUnavailable, postgresFailureDegradedStatusTimeout)
	get(t, "/", http.StatusOK)
	if body := get(t, "/health/ready", http.StatusServiceUnavailable); !bytes.Contains(body, []byte("DEPENDENCY_UNAVAILABLE")) {
		t.Fatalf("readiness response = %s", body)
	}
	requireWorkerRestart(t, root, workerBaseline)

	requireComposeOutput(t, root, postgresFailureComposeControlTimeout, "start", "postgres")
	waitForStatus(t, "/health/ready", http.StatusOK, postgresRecoveryReadinessTimeout)
	if recoveredID := requireComposeContainerID(t, root, "postgres"); recoveredID != postgresContainerID {
		t.Fatalf("postgres container changed during stop/start: before=%q after=%q", postgresContainerID, recoveredID)
	}
	requireWorkerRecovery(t, root, workerBaseline)
	recoveryComplete = true
}

func postgresFailureJourneyRequiredRemaining() time.Duration {
	return postgresFailureJourneyWorstCaseBudget()
}

func requireFailureJourneyDeadline(deadline time.Time) error {
	remaining := time.Until(deadline)
	required := postgresFailureJourneyRequiredRemaining()
	if remaining < required {
		return fmt.Errorf("PostgreSQL failure journey requires %s remaining before destructive stop; process deadline has %s", required, remaining.Round(time.Second))
	}
	return nil
}

func requireSemliaBaseline(t *testing.T, root string) {
	t.Helper()
	get(t, "/health/ready", http.StatusOK)
	var info struct {
		Service    string `json:"service"`
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(get(t, "/api/v1/system/info", http.StatusOK), &info); err != nil {
		t.Fatalf("decode Semlia system identity: %v", err)
	}
	if info.Service != "semlia" || info.APIVersion != "v1" {
		t.Fatalf("unexpected system identity: service=%q apiVersion=%q", info.Service, info.APIVersion)
	}
	running := strings.Fields(requireComposeOutput(t, root, postgresFailureComposeQueryTimeout, "ps", "--status", "running", "--services"))
	sort.Strings(running)
	if got, want := strings.Join(running, ","), "postgres,server,worker"; got != want {
		t.Fatalf("baseline running services = %q, want %q", got, want)
	}
}

func requireComposeContainerID(t *testing.T, root, service string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), postgresFailureComposeQueryTimeout)
	defer cancel()
	output, err := composeOutput(ctx, root, "ps", "--all", "--quiet", service)
	if err != nil {
		t.Fatal(err)
	}
	containerID, err := singleContainerID(output)
	if err != nil {
		t.Fatal(err)
	}
	if containerID == "" {
		t.Fatalf("compose service %q has no container", service)
	}
	return containerID
}

func requireComposeOutput(t *testing.T, root string, timeout time.Duration, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, err := composeOutput(ctx, root, args...)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func requireWorkerContainerState(t *testing.T, root, containerID string) workerContainerState {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), workerProbeTimeout)
	defer cancel()
	state, err := inspectWorkerContainer(ctx, root, containerID)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func requireWorkerRestart(t *testing.T, root string, baseline workerContainerState) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), workerFailureObservationTimeout)
	defer cancel()
	ticker := time.NewTicker(workerRecoveryPollInterval)
	defer ticker.Stop()
	if _, err := waitForWorkerRestart(ctx, baseline, ticker.C, func(ctx context.Context) (workerContainerState, error) {
		return inspectWorkerContainer(ctx, root, baseline.containerID)
	}); err != nil {
		t.Fatal(err)
	}
}

func requireWorkerRecovery(t *testing.T, root string, baseline workerContainerState) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), workerRecoveryTimeout)
	defer cancel()
	ticker := time.NewTicker(workerRecoveryPollInterval)
	defer ticker.Stop()
	if err := waitForWorkerRecovery(ctx, baseline, ticker.C, func(ctx context.Context) (workerContainerState, error) {
		return inspectWorkerContainer(ctx, root, baseline.containerID)
	}); err != nil {
		t.Fatal(err)
	}
}

func cleanupPostgresFailure(root string, baseline workerContainerState) error {
	ctx, cancel := context.WithTimeout(context.Background(), postgresFailureCleanupTimeout)
	defer cancel()
	return restorePostgresFailure(ctx, postgresFailureCleanupOperations{
		startPostgres: func(ctx context.Context) error {
			_, err := composeOutput(ctx, root, "start", "postgres")
			return err
		},
		waitReady: func(ctx context.Context) error {
			return waitForHTTPStatus(ctx, "/health/ready", http.StatusOK)
		},
		waitWorker: func(ctx context.Context) error {
			ticker := time.NewTicker(workerRecoveryPollInterval)
			defer ticker.Stop()
			return waitForWorkerStable(ctx, baseline, ticker.C, func(ctx context.Context) (workerContainerState, error) {
				return inspectWorkerContainer(ctx, root, baseline.containerID)
			})
		},
	})
}

func waitForWorkerRestart(ctx context.Context, baseline workerContainerState, retry <-chan time.Time, probe workerContainerProbe) (workerContainerState, error) {
	if err := validateWorkerBaseline(baseline); err != nil {
		return workerContainerState{}, err
	}
	lastState := baseline
	var lastProbeError error
	for {
		state, err := probe(ctx)
		if err != nil {
			lastProbeError = err
		} else {
			lastState = state
			lastProbeError = nil
			if err := validateWorkerIdentity(baseline, state); err != nil {
				return workerContainerState{}, err
			}
			if state.restartCount > baseline.restartCount && state.startedAt != baseline.startedAt {
				return state, nil
			}
		}

		select {
		case <-ctx.Done():
			if lastProbeError != nil {
				return workerContainerState{}, fmt.Errorf("worker container %q did not restart after PostgreSQL stopped; last probe error: %v: %w", baseline.containerID, lastProbeError, ctx.Err())
			}
			return workerContainerState{}, fmt.Errorf("worker container %q did not restart after PostgreSQL stopped; last state %s: %w", baseline.containerID, formatWorkerState(lastState), ctx.Err())
		case <-retry:
		}
	}
}

func waitForWorkerRecovery(ctx context.Context, baseline workerContainerState, retry <-chan time.Time, probe workerContainerProbe) error {
	if err := validateWorkerBaseline(baseline); err != nil {
		return err
	}
	lastState := baseline
	var lastProbeError error
	stableState := workerContainerState{}
	stableRunningChecks := 0
	for {
		state, err := probe(ctx)
		if err != nil {
			lastProbeError = err
			stableRunningChecks = 0
		} else {
			lastState = state
			lastProbeError = nil
			if err := validateWorkerIdentity(baseline, state); err != nil {
				return err
			}
			newGeneration := state.restartCount > baseline.restartCount && state.startedAt != baseline.startedAt
			if state.running && newGeneration {
				if state.restartCount == stableState.restartCount && state.startedAt == stableState.startedAt {
					stableRunningChecks++
				} else {
					stableState = state
					stableRunningChecks = 1
				}
				if stableRunningChecks == workerStableRunningChecks {
					return nil
				}
			} else {
				stableRunningChecks = 0
			}
		}

		select {
		case <-ctx.Done():
			if lastProbeError != nil {
				return fmt.Errorf("worker container %q did not stably recover; last probe error: %v: %w", baseline.containerID, lastProbeError, ctx.Err())
			}
			return fmt.Errorf("worker container %q did not stably recover; last state %s: %w", baseline.containerID, formatWorkerState(lastState), ctx.Err())
		case <-retry:
		}
	}
}

func waitForWorkerStable(ctx context.Context, baseline workerContainerState, retry <-chan time.Time, probe workerContainerProbe) error {
	if err := validateWorkerBaseline(baseline); err != nil {
		return err
	}
	lastState := baseline
	var lastProbeError error
	stableState := workerContainerState{}
	stableRunningChecks := 0
	for {
		state, err := probe(ctx)
		if err != nil {
			lastProbeError = err
			stableRunningChecks = 0
		} else {
			lastState = state
			lastProbeError = nil
			if err := validateWorkerIdentity(baseline, state); err != nil {
				return err
			}
			if state.running {
				if state.restartCount == stableState.restartCount && state.startedAt == stableState.startedAt {
					stableRunningChecks++
				} else {
					stableState = state
					stableRunningChecks = 1
				}
				if stableRunningChecks == workerStableRunningChecks {
					return nil
				}
			} else {
				stableRunningChecks = 0
			}
		}

		select {
		case <-ctx.Done():
			if lastProbeError != nil {
				return fmt.Errorf("worker container %q did not become stably running during cleanup; last probe error: %v: %w", baseline.containerID, lastProbeError, ctx.Err())
			}
			return fmt.Errorf("worker container %q did not become stably running during cleanup; last state %s: %w", baseline.containerID, formatWorkerState(lastState), ctx.Err())
		case <-retry:
		}
	}
}

func validateWorkerBaseline(state workerContainerState) error {
	if err := validateWorkerState(state); err != nil {
		return err
	}
	if !state.running {
		return errors.New("worker is not running before failure")
	}
	return nil
}

func validateWorkerState(state workerContainerState) error {
	if state.containerID == "" {
		return errors.New("worker container ID is empty")
	}
	if state.startedAt == "" {
		return errors.New("worker StartedAt is empty")
	}
	if state.restartCount < 0 {
		return fmt.Errorf("worker RestartCount is negative: %d", state.restartCount)
	}
	return nil
}

func validateWorkerIdentity(baseline, state workerContainerState) error {
	if state.containerID != baseline.containerID {
		return fmt.Errorf("worker container changed during automatic recovery: before=%q after=%q", baseline.containerID, state.containerID)
	}
	if state.restartCount < baseline.restartCount {
		return fmt.Errorf("worker restart count went backwards: before=%d after=%d", baseline.restartCount, state.restartCount)
	}
	return nil
}

func inspectWorkerContainer(ctx context.Context, root, containerID string) (workerContainerState, error) {
	probeCtx, cancel := context.WithTimeout(ctx, workerProbeTimeout)
	defer cancel()
	output, err := commandOutput(probeCtx, root, "docker", "container", "inspect", "--format", workerInspectFormat, containerID)
	if err != nil {
		return workerContainerState{}, err
	}
	state, err := decodeWorkerContainerState(output)
	if err != nil {
		return workerContainerState{}, err
	}
	if state.containerID != containerID {
		return workerContainerState{}, fmt.Errorf("docker inspected unexpected worker container: requested=%q inspected=%q", containerID, state.containerID)
	}
	return state, nil
}

func decodeWorkerContainerState(output string) (workerContainerState, error) {
	var inspected struct {
		ID           string `json:"id"`
		Running      bool   `json:"running"`
		StartedAt    string `json:"startedAt"`
		RestartCount int    `json:"restartCount"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &inspected); err != nil {
		return workerContainerState{}, fmt.Errorf("decode worker container inspect: %w", err)
	}
	state := workerContainerState{
		containerID:  inspected.ID,
		running:      inspected.Running,
		startedAt:    inspected.StartedAt,
		restartCount: inspected.RestartCount,
	}
	if err := validateWorkerState(state); err != nil {
		return workerContainerState{}, fmt.Errorf("invalid worker container inspect: %w", err)
	}
	return state, nil
}

func formatWorkerState(state workerContainerState) string {
	return fmt.Sprintf("container=%q running=%t startedAt=%q restartCount=%d", state.containerID, state.running, state.startedAt, state.restartCount)
}

func composeOutput(ctx context.Context, root string, args ...string) (string, error) {
	name, commandArgs := composeCommand(root, args...)
	return commandOutput(ctx, root, name, commandArgs...)
}

func composeCommand(root string, args ...string) (string, []string) {
	commandArgs := []string{"compose", "--env-file", filepath.Join(root, ".semlia", "dev.env")}
	return "docker", append(commandArgs, args...)
}

func commandOutput(ctx context.Context, root, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = root
	command.WaitDelay = smokeCommandWaitDelay
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%s %s: %w (stdout=%q stderr=%q)", name, strings.Join(args, " "), ctx.Err(), strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("%s %s: %w (stdout=%q stderr=%q)", name, strings.Join(args, " "), err, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func singleContainerID(output string) (string, error) {
	ids := strings.Fields(output)
	if len(ids) > 1 {
		return "", fmt.Errorf("expected one container ID, got %d: %q", len(ids), output)
	}
	if len(ids) == 0 {
		return "", nil
	}
	return ids[0], nil
}
