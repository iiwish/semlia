package smoke

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	workerRecoveryTimeout      = 90 * time.Second
	workerRecoveryPollInterval = 500 * time.Millisecond
	workerProbeTimeout         = 10 * time.Second
	workerStableRunningChecks  = 5
)

type workerContainerState struct {
	containerID string
	running     bool
}

type workerContainerProbe func(context.Context) (workerContainerState, error)

func TestWaitForWorkerRecoveryWaitsForSameContainer(t *testing.T) {
	states := []workerContainerState{
		{containerID: "worker-before"},
		{containerID: "worker-before", running: true},
		{containerID: "worker-before"},
		{containerID: "worker-before", running: true},
		{containerID: "worker-before", running: true},
		{containerID: "worker-before", running: true},
		{containerID: "worker-before", running: true},
		{containerID: "worker-before", running: true},
	}
	retry := make(chan time.Time, len(states)-1)
	for range len(states) - 1 {
		retry <- time.Time{}
	}
	probes := 0

	err := waitForWorkerRecovery(context.Background(), "worker-before", retry, func(context.Context) (workerContainerState, error) {
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

func TestWaitForWorkerRecoveryRejectsReplacement(t *testing.T) {
	err := waitForWorkerRecovery(context.Background(), "worker-before", nil, func(context.Context) (workerContainerState, error) {
		return workerContainerState{containerID: "worker-after", running: true}, nil
	})
	if err == nil || !strings.Contains(err.Error(), `before="worker-before" after="worker-after"`) {
		t.Fatalf("waitForWorkerRecovery() error = %v, want container replacement details", err)
	}
}

func TestWaitForWorkerRecoveryIsBounded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForWorkerRecovery(ctx, "worker-before", nil, func(context.Context) (workerContainerState, error) {
		return workerContainerState{containerID: "worker-before"}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForWorkerRecovery() error = %v, want context cancellation", err)
	}
	if !strings.Contains(err.Error(), `container="worker-before" running=false`) {
		t.Fatalf("waitForWorkerRecovery() error = %v, want last observed state", err)
	}
}

func Test04PostgresFailureAndRecovery(t *testing.T) {
	requireRuntimeSmoke(t)

	root := repositoryRoot(t)
	postgresContainerID := strings.TrimSpace(compose(t, "ps", "-q", "postgres"))
	workerContainerID := strings.TrimSpace(compose(t, "ps", "--all", "-q", "worker"))
	if postgresContainerID == "" || workerContainerID == "" {
		t.Fatalf("required container IDs before failure: postgres=%q worker=%q", postgresContainerID, workerContainerID)
	}
	t.Cleanup(func() {
		composeWithoutFailure(t, "start", "postgres")
		waitForStatus(t, "/health/ready", http.StatusOK, 90*time.Second)
		requireWorkerRecovery(t, root, workerContainerID)
	})
	compose(t, "stop", "postgres")

	waitForStatus(t, "/health/live", http.StatusOK, 30*time.Second)
	waitForStatus(t, "/health/ready", http.StatusServiceUnavailable, 30*time.Second)
	get(t, "/", http.StatusOK)
	if body := get(t, "/health/ready", http.StatusServiceUnavailable); !bytes.Contains(body, []byte("DEPENDENCY_UNAVAILABLE")) {
		t.Fatalf("readiness response = %s", body)
	}

	compose(t, "start", "postgres")
	waitForStatus(t, "/health/ready", http.StatusOK, 90*time.Second)
	if recoveredID := strings.TrimSpace(compose(t, "ps", "-q", "postgres")); recoveredID != postgresContainerID {
		t.Fatalf("postgres container changed during stop/start: before=%q after=%q", postgresContainerID, recoveredID)
	}
	requireWorkerRecovery(t, root, workerContainerID)
}

func requireWorkerRecovery(t *testing.T, root, containerID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), workerRecoveryTimeout)
	defer cancel()
	ticker := time.NewTicker(workerRecoveryPollInterval)
	defer ticker.Stop()
	if err := waitForWorkerRecovery(ctx, containerID, ticker.C, func(ctx context.Context) (workerContainerState, error) {
		return probeWorkerContainer(ctx, root)
	}); err != nil {
		t.Fatal(err)
	}
}

func waitForWorkerRecovery(ctx context.Context, containerID string, retry <-chan time.Time, probe workerContainerProbe) error {
	if containerID == "" {
		return errors.New("worker container ID before failure is empty")
	}
	lastState := workerContainerState{containerID: containerID}
	var lastProbeError error
	stableRunningChecks := 0
	for {
		state, err := probe(ctx)
		if err != nil {
			lastProbeError = err
			stableRunningChecks = 0
		} else {
			lastState = state
			lastProbeError = nil
			if state.containerID != "" && state.containerID != containerID {
				return fmt.Errorf("worker container changed during automatic recovery: before=%q after=%q", containerID, state.containerID)
			}
			if state.containerID == containerID && state.running {
				stableRunningChecks++
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
				return fmt.Errorf("worker container %q did not return to running; last probe error: %v: %w", containerID, lastProbeError, ctx.Err())
			}
			return fmt.Errorf("worker container %q did not return to running; last state container=%q running=%t: %w", containerID, lastState.containerID, lastState.running, ctx.Err())
		case <-retry:
		}
	}
}

func probeWorkerContainer(ctx context.Context, root string) (workerContainerState, error) {
	probeCtx, cancel := context.WithTimeout(ctx, workerProbeTimeout)
	defer cancel()

	allOutput, err := composeOutput(probeCtx, root, "ps", "--all", "--quiet", "worker")
	if err != nil {
		return workerContainerState{}, err
	}
	containerID, err := singleContainerID(allOutput)
	if err != nil || containerID == "" {
		return workerContainerState{containerID: containerID}, err
	}
	runningOutput, err := composeOutput(probeCtx, root, "ps", "--status", "running", "--quiet", "worker")
	if err != nil {
		return workerContainerState{}, err
	}
	runningID, err := singleContainerID(runningOutput)
	if err != nil {
		return workerContainerState{}, err
	}
	if runningID != "" && runningID != containerID {
		return workerContainerState{}, fmt.Errorf("compose reported inconsistent worker IDs: current=%q running=%q", containerID, runningID)
	}
	return workerContainerState{containerID: containerID, running: runningID == containerID}, nil
}

func composeOutput(ctx context.Context, root string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, filepath.Join(root, "scripts", "dev", "compose.sh"), args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("compose %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func singleContainerID(output string) (string, error) {
	ids := strings.Fields(output)
	if len(ids) > 1 {
		return "", fmt.Errorf("expected one worker container, got %d: %q", len(ids), output)
	}
	if len(ids) == 0 {
		return "", nil
	}
	return ids[0], nil
}
