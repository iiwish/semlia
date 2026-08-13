package acceptance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
)

const testcontainersFailureHelper = "SEMLIA_TESTCONTAINERS_FAILURE_HELPER"

func TestPinnedTestcontainersSessionIDUsesLibraryPublicAPI(t *testing.T) {
	const helper = "SEMLIA_TESTCONTAINERS_SESSION_HELPER"
	if os.Getenv(helper) == "1" {
		labels := testcontainers.GenericLabels()
		fmt.Printf("TESTCONTAINERS_SESSION=%s\n", testcontainers.SessionID())
		fmt.Printf("TESTCONTAINERS_SESSION_LABEL=%s\n", labels[testcontainersSessionLabelKey])
		fmt.Printf("TESTCONTAINERS_REAP_LABEL=%s\n", labels["org.testcontainers.reap"])
		return
	}
	expected := "semlia-accept-a1b2c3d4-000000000001"
	taskHome := t.TempDir()
	command := newAcceptanceCommandContext(context.Background(), os.Args[0], "-test.run=^TestPinnedTestcontainersSessionIDUsesLibraryPublicAPI$", "-test.count=1")
	command.Env = append(
		filteredEnvironment(os.Environ(), helper, "HOME", "TESTCONTAINERS_SESSION_ID", "TESTCONTAINERS_RYUK_DISABLED"),
		helper+"=1",
		"HOME="+taskHome,
		"TESTCONTAINERS_SESSION_ID="+expected,
		"TESTCONTAINERS_RYUK_DISABLED=false",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("read Testcontainers session through public API: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "TESTCONTAINERS_SESSION="+expected) {
		t.Fatalf("Testcontainers ignored pinned session ID: %s", output)
	}
	if !strings.Contains(string(output), "TESTCONTAINERS_SESSION_LABEL="+expected) {
		t.Fatalf("Testcontainers public labels omitted pinned session ownership: %s", output)
	}
	if !strings.Contains(string(output), "TESTCONTAINERS_REAP_LABEL=true") {
		t.Fatalf("Testcontainers did not enable Ryuk ownership for the isolated session: %s", output)
	}
}

func TestTestcontainersFailureCleanupProbeHelper(t *testing.T) {
	if os.Getenv(testcontainersFailureHelper) != "1" {
		return
	}
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") != "true" {
		t.Fatal("failure probe must explicitly disable Ryuk")
	}
	image := os.Getenv("SEMLIA_DOCKER_CLEANUP_PROBE_IMAGE")
	if image == "" {
		image = "alpine:3.22"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	networkName := os.Getenv("SEMLIA_TESTCONTAINERS_NETWORK_NAME")
	if networkName == "" {
		t.Fatal("failure probe requires an explicit Testcontainers network name")
	}
	if _, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{Name: networkName, Driver: "bridge"},
	}); err != nil {
		t.Fatalf("start Testcontainers failure probe network: %v", err)
	}
	fmt.Printf("TESTCONTAINERS_FAILURE_NETWORK=%s\n", networkName)
	container, err := testcontainers.Run(ctx, image, testcontainers.WithCmd("sh", "-c", "sleep 300"))
	if err != nil {
		t.Fatalf("start Testcontainers failure probe: %v", err)
	}
	fmt.Printf("TESTCONTAINERS_FAILURE_CONTAINER=%s\n", container.GetContainerID())
	<-ctx.Done()
}

func TestTestcontainersFailureCleanupProbe(t *testing.T) {
	if os.Getenv("SEMLIA_RUN_TESTCONTAINERS_CLEANUP_PROBE") != "1" {
		t.Skip("set SEMLIA_RUN_TESTCONTAINERS_CLEANUP_PROBE=1 to exercise timed-out Testcontainers cleanup")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	image := os.Getenv("SEMLIA_DOCKER_CLEANUP_PROBE_IMAGE")
	if image == "" {
		image = "alpine:3.22"
	}
	if _, err := dockerProbeOutput(ctx, "image", "inspect", image); err != nil {
		if output, pullErr := dockerProbeOutput(ctx, "pull", image); pullErr != nil {
			t.Fatalf("prepare Testcontainers cleanup probe image %q: %v\n%s", image, pullErr, output)
		}
	}

	project := "semlia-testcontainers-probe-" + randomRunID(t)
	sessionLabel := testcontainersSessionLabelKey + "=" + project
	unrelatedLabel := testcontainersSessionLabelKey + "=" + project + "-unrelated"
	networkName := project + "-network"
	unrelatedNetworkName := project + "-unrelated-network"
	volumeName := project + "-volume"
	unrelatedVolumeName := project + "-unrelated-volume"
	var unrelatedContainerID string
	t.Cleanup(func() {
		remove := func(args ...string) {
			cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancelCleanup()
			_, _ = dockerProbeOutput(cleanupCtx, args...)
		}
		if unrelatedContainerID != "" {
			remove("rm", "--force", unrelatedContainerID)
		}
		remove("network", "rm", networkName, unrelatedNetworkName)
		remove("volume", "rm", "--force", volumeName, unrelatedVolumeName)
		_ = cleanupDockerProbeContainers(sessionLabel, unrelatedLabel)
	})

	unrelatedContainerOutput, err := dockerProbeOutput(ctx, "run", "--detach", "--rm", "--label", unrelatedLabel, image, "sh", "-c", "sleep 300")
	if err != nil {
		t.Fatalf("start unrelated Testcontainers cleanup probe container: %v\n%s", err, unrelatedContainerOutput)
	}
	unrelatedContainerID = strings.TrimSpace(unrelatedContainerOutput)
	for _, resource := range []struct {
		kind  string
		name  string
		label string
	}{
		{"network", unrelatedNetworkName, unrelatedLabel},
		{"volume", volumeName, sessionLabel},
		{"volume", unrelatedVolumeName, unrelatedLabel},
	} {
		if output, err := dockerProbeOutput(ctx, resource.kind, "create", "--label", resource.label, resource.name); err != nil {
			t.Fatalf("create %s %s: %v\n%s", resource.kind, resource.name, err, output)
		}
	}
	scratch := t.TempDir()
	environment := isolatedEnvironmentFrom(os.Environ(), scratch, project, 38080, 35432)
	if err := prepareIsolatedEnvironmentDirectories(environment); err != nil {
		t.Fatal(err)
	}
	environment = append(
		filteredEnvironment(environment, "TESTCONTAINERS_RYUK_DISABLED"),
		"TESTCONTAINERS_RYUK_DISABLED=true",
		testcontainersFailureHelper+"=1",
		"SEMLIA_DOCKER_CLEANUP_PROBE_IMAGE="+image,
		"SEMLIA_TESTCONTAINERS_NETWORK_NAME="+networkName,
	)
	helperCtx, cancelHelper := context.WithTimeout(ctx, 10*time.Second)
	helperWaited := false
	helper := newAcceptanceCommandContext(helperCtx, os.Args[0], "-test.run=^TestTestcontainersFailureCleanupProbeHelper$", "-test.count=1")
	helper.Env = environment
	helper.Dir = repositoryRoot
	var helperOutput bytes.Buffer
	helper.Stdout = &helperOutput
	helper.Stderr = &helperOutput
	if err := helper.Start(); err != nil {
		cancelHelper()
		t.Fatalf("start timed-out Testcontainers helper: %v", err)
	}
	t.Cleanup(func() {
		cancelHelper()
		if !helperWaited && helper.Process != nil {
			_ = helper.Process.Kill()
			_ = helper.Wait()
			helperWaited = true
		}
	})

	waitCtx, cancelWait := context.WithTimeout(ctx, 8*time.Second)
	taskContainerID, err := waitForDockerProbeContainer(waitCtx, sessionLabel)
	cancelWait()
	if err != nil {
		_ = helper.Process.Kill()
		_ = helper.Wait()
		helperWaited = true
		t.Fatalf("wait for Testcontainers failure resource: %v\n%s", err, helperOutput.String())
	}
	waitErr := helper.Wait()
	helperWaited = true
	cancelHelper()
	if helperCtx.Err() != context.DeadlineExceeded || waitErr == nil {
		t.Fatalf("Testcontainers helper did not fail through its command timeout: context=%v wait=%v\n%s", helperCtx.Err(), waitErr, helperOutput.String())
	}
	if !strings.Contains(helperOutput.String(), "TESTCONTAINERS_FAILURE_CONTAINER="+taskContainerID) {
		t.Fatalf("timed-out helper did not report the owned container %s:\n%s", taskContainerID, helperOutput.String())
	}
	if !strings.Contains(helperOutput.String(), "TESTCONTAINERS_FAILURE_NETWORK="+networkName) {
		t.Fatalf("timed-out helper did not report the owned network %s:\n%s", networkName, helperOutput.String())
	}

	run := &freshCloneRun{root: repositoryRoot, project: project, environment: environment}
	if err := run.removeRemainingTaskDockerResources(); err != nil {
		t.Fatalf("clean timed-out Testcontainers session: %v", err)
	}
	for _, resource := range run.cleanupResourceKinds() {
		if resource.identityLabel != sessionLabel {
			continue
		}
		output, err := dockerProbeOutput(ctx, resource.listArgs...)
		if err != nil || strings.TrimSpace(output) != "" {
			t.Errorf("task session %s remains after exact cleanup: err=%v output=%q", resource.label, err, output)
		}
	}
	for _, unrelated := range []struct {
		kind string
		id   string
	}{
		{"container", unrelatedContainerID},
		{"network", unrelatedNetworkName},
		{"volume", unrelatedVolumeName},
	} {
		args := []string{"inspect", unrelated.id}
		if unrelated.kind != "container" {
			args = []string{unrelated.kind, "inspect", unrelated.id}
		}
		if output, err := dockerProbeOutput(ctx, args...); err != nil || strings.TrimSpace(output) == "" {
			t.Errorf("cleanup removed unrelated %s %s: err=%v output=%q", unrelated.kind, unrelated.id, err, output)
		}
	}
	if !errors.Is(helperCtx.Err(), context.DeadlineExceeded) {
		t.Fatalf("failure probe did not exercise a deadline: %v", helperCtx.Err())
	}
}
