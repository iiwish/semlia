package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

const (
	smokeTimeout              = 4 * time.Minute
	smokeProcessTimeout       = 10 * time.Minute
	smokeHTTPClientTimeout    = 5 * time.Second
	smokeStatusRequestTimeout = 2 * time.Second
	smokeStatusPollInterval   = 500 * time.Millisecond
	traceIDLength             = 32
)

var assetPattern = regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`)

type runtimeJourneyStep struct {
	name string
	run  func(*testing.T)
}

var runtimeJourneySteps = []runtimeJourneyStep{
	{name: "01 embedded Web and API baseline", run: testEmbeddedWebAndAPI},
	{name: "02 required processes", run: testRequiredProcessesAreRunning},
	{name: "03 worker restart and persistence", run: testWorkerRestartPreservesDatabaseJob},
	{name: "04 PostgreSQL failure and recovery", run: testPostgresFailureAndRecovery},
	{name: "05 shutdown persistence", run: testShutdownPreservesDataVolume},
}

func TestLocalStackRuntimeJourney(t *testing.T) {
	requireRuntimeSmoke(t)
	for _, step := range runtimeJourneySteps {
		if !t.Run(step.name, step.run) {
			t.FailNow()
		}
	}
}

func Test00LocalStackContract(t *testing.T) {
	root := repositoryRoot(t)
	for _, name := range []string{
		"compose.yaml",
		"compose.override.yaml",
		".env.example",
		"deploy/local/Dockerfile",
		"scripts/dev/compose.sh",
		"scripts/dev/ensure-env.sh",
	} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Errorf("required local stack file %s: %v", name, err)
		}
	}
	composeFile := readFile(t, filepath.Join(root, "compose.yaml"))
	for _, fragment := range []string{
		"postgres:18-alpine",
		"condition: service_healthy",
		"condition: service_completed_successfully",
		`command: ["migrate", "up"]`,
		`command: ["server"]`,
		`command: ["worker"]`,
		"postgres-data:/var/lib/postgresql",
		"git-content:/var/lib/semlia/content",
	} {
		if !strings.Contains(composeFile, fragment) {
			t.Errorf("compose.yaml does not contain %q", fragment)
		}
	}
	overrideFile := readFile(t, filepath.Join(root, "compose.override.yaml"))
	for _, fragment := range []string{"127.0.0.1:${SEMLIA_HTTP_PORT:-8080}:8080", "127.0.0.1:${SEMLIA_POSTGRES_PORT:-5433}:5432"} {
		if !strings.Contains(overrideFile, fragment) {
			t.Errorf("compose.override.yaml does not contain %q", fragment)
		}
	}
}

func TestComposeProjectResolution(t *testing.T) {
	for _, input := range []struct {
		name        string
		environment string
		file        string
		want        string
		wantError   bool
	}{
		{"environment precedence", "semlia-override", "COMPOSE_PROJECT_NAME=semlia-file\n", "semlia-override", false},
		{"file fallback", "", "COMPOSE_PROJECT_NAME=semlia-file\n", "semlia-file", false},
		{"single character", "a", "", "a", false},
		{"leading underscore", "_semlia", "COMPOSE_PROJECT_NAME=valid-file\n", "", true},
		{"leading hyphen", "-semlia", "", "", true},
		{"uppercase", "Semlia", "", "", true},
		{"shell separator", "semlia;other", "", "", true},
		{"missing", "", "", "", true},
	} {
		t.Run(input.name, func(t *testing.T) {
			got, err := resolveComposeProject(input.environment, input.file)
			if (err != nil) != input.wantError {
				t.Fatalf("resolveComposeProject() error = %v, wantError %t", err, input.wantError)
			}
			if got != input.want {
				t.Errorf("resolveComposeProject() = %q, want %q", got, input.want)
			}
		})
	}
}

func TestRuntimeJourneyContract(t *testing.T) {
	got := make([]string, 0, len(runtimeJourneySteps))
	for _, step := range runtimeJourneySteps {
		got = append(got, step.name)
	}
	want := []string{
		"01 embedded Web and API baseline",
		"02 required processes",
		"03 worker restart and persistence",
		"04 PostgreSQL failure and recovery",
		"05 shutdown persistence",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("runtime journey order = %q, want %q", got, want)
	}

	sources := readFile(t, filepath.Join(repositoryRoot(t), "tests", "smoke", "local_stack_test.go")) +
		readFile(t, filepath.Join(repositoryRoot(t), "tests", "smoke", "dependency_failure_test.go"))
	optInCall := "requireRuntime" + "Smoke(t)"
	if count := strings.Count(sources, optInCall); count != 1 {
		t.Fatalf("top-level runtime journey opt-in count = %d, want 1", count)
	}
	for _, testName := range []string{
		"Test01EmbeddedWebAndAPI",
		"Test02RequiredProcessesAreRunning",
		"Test03WorkerRestartPreservesDatabaseJob",
		"Test04PostgresFailureAndRecovery",
		"Test05ShutdownPreservesDataVolume",
	} {
		forbidden := "func " + testName + "("
		if strings.Contains(sources, forbidden) {
			t.Errorf("runtime step remains independently runnable: %s", forbidden)
		}
	}
}

func TestSmokeTargetPinsTenMinuteProcessTimeout(t *testing.T) {
	makefile := readFile(t, filepath.Join(repositoryRoot(t), "Makefile"))
	const command = `$(GO) test -timeout=10m -count=1 ./tests/smoke/...`
	if strings.Count(makefile, command) != 1 {
		t.Fatalf("Makefile smoke target must contain exactly one %q", command)
	}
}

func TestHTTPPollCancellationDoesNotDrainSynchronousTimer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	timerChannel := make(chan time.Time)
	stopCalled := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- waitForHTTPPoll(ctx, timerChannel, func() bool {
			stopCalled <- struct{}{}
			return false
		})
	}()

	select {
	case <-stopCalled:
	case <-time.After(time.Second):
		t.Fatal("waitForHTTPPoll did not stop its timer on cancellation")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waitForHTTPPoll() error = %v, want context canceled", err)
		}
	case <-time.After(250 * time.Millisecond):
		close(timerChannel)
		<-result
		t.Fatal("waitForHTTPPoll blocked draining a stopped synchronous timer")
	}
}

func testEmbeddedWebAndAPI(t *testing.T) {
	index := get(t, "/", http.StatusOK)
	if !bytes.Contains(index, []byte("<title>Semlia</title>")) {
		t.Fatal("embedded Web root does not contain the Semlia application")
	}
	asset := assetPattern.FindSubmatch(index)
	if len(asset) != 2 {
		t.Fatal("embedded Web root does not reference a built asset")
	}
	if body := get(t, string(asset[1]), http.StatusOK); len(body) == 0 {
		t.Fatal("embedded Web asset is empty")
	}

	for _, path := range []string{"/health/live", "/health/ready", "/api/v1/system/info"} {
		body := get(t, path, http.StatusOK)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		traceID, _ := payload["traceId"].(string)
		if len(traceID) != traceIDLength {
			t.Fatalf("%s trace ID length = %d, want %d", path, len(traceID), traceIDLength)
		}
	}
}

func testRequiredProcessesAreRunning(t *testing.T) {
	running := strings.Fields(compose(t, "ps", "--status", "running", "--services"))
	sort.Strings(running)
	if got, want := strings.Join(running, ","), "postgres,server,worker"; got != want {
		t.Fatalf("running services = %q, want %q", got, want)
	}
	exited := strings.Fields(compose(t, "ps", "--all", "--status", "exited", "--services"))
	if !contains(exited, "migrate") {
		t.Fatalf("exited services = %v, want completed migrate service", exited)
	}
}

func testWorkerRestartPreservesDatabaseJob(t *testing.T) {
	stamp := time.Now().UTC().UnixNano()
	workspaceID := newUUIDv7(t, identity.Workspace)
	jobID := newUUIDv7(t, identity.Run)
	workspaceSlug := fmt.Sprintf("smoke-workspace-%d", stamp)
	sql := fmt.Sprintf(`
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ('%[1]s', '%[3]s', 'Smoke Workspace');
		INSERT INTO jobs (
			id, workspace_id, job_type, payload, max_attempts,
			available_at, idempotency_key, trace_id
		) VALUES (
			'%[2]s', '%[1]s', 'smoke.persistence', '{}'::jsonb, 3,
			CURRENT_TIMESTAMP + INTERVAL '1 hour', '%[2]s',
			'4bf92f3577b34da6a3ce929d0e0e4736'
		);`, workspaceID, jobID, workspaceSlug)
	psql(t, sql)

	before := strings.TrimSpace(compose(t, "ps", "-q", "worker"))
	compose(t, "restart", "worker")
	after := strings.TrimSpace(compose(t, "ps", "-q", "worker"))
	if before == "" || before != after {
		t.Fatalf("worker container changed across restart: before=%q after=%q", before, after)
	}
	if got := strings.TrimSpace(psql(t, "SELECT count(*) FROM jobs WHERE id = '"+jobID+"';")); got != "1" {
		t.Fatalf("persisted job count = %q, want 1", got)
	}
}

func testShutdownPreservesDataVolume(t *testing.T) {
	stamp := time.Now().UTC().UnixNano()
	workspaceID := newUUIDv7(t, identity.Workspace)
	jobID := newUUIDv7(t, identity.Run)
	workspaceSlug := fmt.Sprintf("shutdown-workspace-%d", stamp)
	psql(t, fmt.Sprintf(`
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ('%[1]s', '%[3]s', 'Shutdown Workspace');
		INSERT INTO jobs (
			id, workspace_id, job_type, payload, max_attempts,
			available_at, idempotency_key, trace_id
		) VALUES (
			'%[2]s', '%[1]s', 'smoke.shutdown', '{}'::jsonb, 3,
			CURRENT_TIMESTAMP + INTERVAL '1 hour', '%[2]s',
			'4bf92f3577b34da6a3ce929d0e0e4736'
		);`, workspaceID, jobID, workspaceSlug))

	project := composeProject(t)
	volume := docker(t, "volume", "ls", "--filter", "label=com.docker.compose.project="+project, "--filter", "label=com.docker.compose.volume=postgres-data", "--quiet")
	volume = strings.TrimSpace(volume)
	if volume == "" {
		t.Fatal("compose data volume was not found")
	}
	compose(t, "down", "--remove-orphans")
	if containers := strings.TrimSpace(compose(t, "ps", "--all", "--quiet")); containers != "" {
		t.Fatalf("containers remain after shutdown: %s", containers)
	}
	if networks := strings.TrimSpace(docker(t, "network", "ls", "--filter", "label=com.docker.compose.project="+project, "--quiet")); networks != "" {
		t.Fatalf("project networks remain after shutdown: %s", networks)
	}
	if preserved := strings.TrimSpace(docker(t, "volume", "inspect", "--format", "{{.Name}}", volume)); preserved != volume {
		t.Fatalf("preserved volume = %q, want %q", preserved, volume)
	}

	compose(t, "up", "--detach", "--wait", "--wait-timeout", "240")
	waitForStatus(t, "/health/ready", http.StatusOK, 90*time.Second)
	if got := strings.TrimSpace(psql(t, "SELECT count(*) FROM jobs WHERE id = '"+jobID+"';")); got != "1" {
		t.Fatalf("job count after shutdown/start = %q, want 1", got)
	}
}

func newUUIDv7(t *testing.T, prefix identity.Prefix) string {
	t.Helper()
	value, err := identity.New(prefix)
	if err != nil {
		t.Fatalf("create %s smoke identity: %v", prefix, err)
	}
	return value.UUID()
}

func requireRuntimeSmoke(t *testing.T) {
	t.Helper()
	if os.Getenv("SEMLIA_RUN_SMOKE") != "1" {
		t.Skip("set SEMLIA_RUN_SMOKE=1 or run make smoke against the local stack")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func compose(t *testing.T, args ...string) string {
	t.Helper()
	root := repositoryRoot(t)
	return command(t, root, filepath.Join(root, "scripts", "dev", "compose.sh"), args...)
}

func composeProject(t *testing.T) string {
	t.Helper()
	root := repositoryRoot(t)
	content := readFile(t, filepath.Join(root, ".semlia", "dev.env"))
	project, err := resolveComposeProject(os.Getenv("COMPOSE_PROJECT_NAME"), content)
	if err != nil {
		t.Fatal(err)
	}
	return project
}

func resolveComposeProject(environmentValue, fileContent string) (string, error) {
	project := environmentValue
	if project == "" {
		for _, line := range strings.Split(fileContent, "\n") {
			if value, found := strings.CutPrefix(line, "COMPOSE_PROJECT_NAME="); found {
				project = value
				break
			}
		}
	}
	if project == "" {
		return "", fmt.Errorf("COMPOSE_PROJECT_NAME is missing")
	}
	for index, character := range []byte(project) {
		if index == 0 {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') {
				return "", fmt.Errorf("COMPOSE_PROJECT_NAME %q must start with a lowercase letter or digit", project)
			}
			continue
		}
		if character != '_' && character != '-' && (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return "", fmt.Errorf("COMPOSE_PROJECT_NAME %q contains an unsupported character", project)
		}
	}
	return project, nil
}

func composeWithoutFailure(t *testing.T, args ...string) {
	t.Helper()
	root := repositoryRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), smokeTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(root, "scripts", "dev", "compose.sh"), args...)
	command.Dir = root
	_ = command.Run()
}

func docker(t *testing.T, args ...string) string {
	t.Helper()
	return command(t, repositoryRoot(t), "docker", args...)
}

func psql(t *testing.T, statement string) string {
	t.Helper()
	return compose(t, "exec", "-T", "postgres", "psql", "-X", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-U", "semlia", "-d", "semlia", "-c", statement)
}

func command(t *testing.T, directory, name string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), smokeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = directory
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}

func get(t *testing.T, path string, status int) []byte {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, smokeBaseURL()+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: smokeHTTPClientTimeout}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		t.Fatalf("read GET %s: %v", path, err)
	}
	if response.StatusCode != status {
		t.Fatalf("GET %s status = %d, want %d: %s", path, response.StatusCode, status, body)
	}
	return body
}

func waitForStatus(t *testing.T, path string, status int, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := waitForHTTPStatus(ctx, path, status); err != nil {
		t.Fatal(err)
	}
}

func waitForHTTPStatus(ctx context.Context, path string, status int) error {
	client := &http.Client{Timeout: smokeStatusRequestTimeout}
	lastResult := "no response"
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, smokeBaseURL()+path, nil)
		if err != nil {
			return fmt.Errorf("create GET %s: %w", path, err)
		}
		response, requestErr := client.Do(request)
		if requestErr == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
			_ = response.Body.Close()
			lastResult = fmt.Sprintf("HTTP %d", response.StatusCode)
			if response.StatusCode == status {
				return nil
			}
		} else {
			lastResult = requestErr.Error()
		}

		timer := time.NewTimer(smokeStatusPollInterval)
		if err := waitForHTTPPoll(ctx, timer.C, timer.Stop); err != nil {
			return fmt.Errorf("%s did not reach HTTP %d; last result: %s: %w", path, status, lastResult, err)
		}
	}
}

func waitForHTTPPoll(ctx context.Context, timerChannel <-chan time.Time, stopTimer func() bool) error {
	select {
	case <-ctx.Done():
		stopTimer()
		return ctx.Err()
	case <-timerChannel:
		return nil
	}
}

func smokeBaseURL() string {
	if value := strings.TrimRight(os.Getenv("SEMLIA_SMOKE_URL"), "/"); value != "" {
		return value
	}
	return "http://127.0.0.1:8080"
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
