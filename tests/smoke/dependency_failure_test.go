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
	postgresCleanupTimeout          = 15 * time.Second
	workerFailureObservationTimeout = 30 * time.Second
	workerRecoveryTimeout           = 90 * time.Second
	workerRecoveryPollInterval      = 500 * time.Millisecond
	workerProbeTimeout              = 10 * time.Second
	workerStableRunningChecks       = 5
	workerInspectFormat             = `{"id":{{json .Id}},"running":{{json .State.Running}},"startedAt":{{json .State.StartedAt}},"restartCount":{{json .RestartCount}}}`
)

type workerContainerState struct {
	containerID  string
	running      bool
	startedAt    string
	restartCount int
}

type workerContainerProbe func(context.Context) (workerContainerState, error)

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

func TestDependencyFailureBudgets(t *testing.T) {
	if postgresCleanupTimeout > 15*time.Second {
		t.Fatalf("postgres cleanup timeout = %s, want at most 15s", postgresCleanupTimeout)
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

func Test04PostgresFailureAndRecovery(t *testing.T) {
	requireRuntimeSmoke(t)

	root := repositoryRoot(t)
	requireSemliaBaseline(t)
	postgresContainerID := requireComposeContainerID(t, root, "postgres")
	workerContainerID := requireComposeContainerID(t, root, "worker")
	workerBaseline := requireWorkerContainerState(t, root, workerContainerID)
	if err := validateWorkerBaseline(workerBaseline); err != nil {
		t.Fatalf("invalid worker baseline: %v", err)
	}
	postgresStopped := false
	t.Cleanup(func() {
		if postgresStopped {
			bestEffortStartPostgres(root)
		}
	})
	compose(t, "stop", "postgres")
	postgresStopped = true

	waitForStatus(t, "/health/live", http.StatusOK, 30*time.Second)
	waitForStatus(t, "/health/ready", http.StatusServiceUnavailable, 30*time.Second)
	get(t, "/", http.StatusOK)
	if body := get(t, "/health/ready", http.StatusServiceUnavailable); !bytes.Contains(body, []byte("DEPENDENCY_UNAVAILABLE")) {
		t.Fatalf("readiness response = %s", body)
	}
	requireWorkerRestart(t, root, workerBaseline)

	compose(t, "start", "postgres")
	postgresStopped = false
	waitForStatus(t, "/health/ready", http.StatusOK, 90*time.Second)
	if recoveredID := requireComposeContainerID(t, root, "postgres"); recoveredID != postgresContainerID {
		t.Fatalf("postgres container changed during stop/start: before=%q after=%q", postgresContainerID, recoveredID)
	}
	requireWorkerRecovery(t, root, workerBaseline)
}

func requireSemliaBaseline(t *testing.T) {
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
	running := strings.Fields(compose(t, "ps", "--status", "running", "--services"))
	sort.Strings(running)
	if got, want := strings.Join(running, ","), "postgres,server,worker"; got != want {
		t.Fatalf("baseline running services = %q, want %q", got, want)
	}
}

func requireComposeContainerID(t *testing.T, root, service string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), workerProbeTimeout)
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

func bestEffortStartPostgres(root string) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresCleanupTimeout)
	defer cancel()
	_, _ = composeOutput(ctx, root, "start", "postgres")
}

func composeOutput(ctx context.Context, root string, args ...string) (string, error) {
	return commandOutput(ctx, root, filepath.Join(root, "scripts", "dev", "compose.sh"), args...)
}

func commandOutput(ctx context.Context, root, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = root
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
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
