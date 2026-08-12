package smoke

import (
	"bytes"
	"context"
	"encoding/json"
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
)

const (
	smokeTimeout  = 4 * time.Minute
	traceIDLength = 32
)

var assetPattern = regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`)

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

func Test01EmbeddedWebAndAPI(t *testing.T) {
	requireRuntimeSmoke(t)

	index := get(t, "/", http.StatusOK)
	if !bytes.Contains(index, []byte("Semlia System Status")) {
		t.Fatal("embedded Web root does not contain the Semlia status application")
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

func Test02RequiredProcessesAreRunning(t *testing.T) {
	requireRuntimeSmoke(t)

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

func Test03WorkerRestartPreservesDatabaseJob(t *testing.T) {
	requireRuntimeSmoke(t)

	stamp := time.Now().UTC().UnixNano()
	workspaceID := fmt.Sprintf("smoke_workspace_%d", stamp)
	jobID := fmt.Sprintf("smoke_job_%d", stamp)
	sql := fmt.Sprintf(`
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ('%[1]s', '%[1]s', 'Smoke Workspace');
		INSERT INTO jobs (
			id, workspace_id, job_type, payload, max_attempts,
			available_at, idempotency_key, trace_id
		) VALUES (
			'%[2]s', '%[1]s', 'smoke.persistence', '{}'::jsonb, 3,
			CURRENT_TIMESTAMP + INTERVAL '1 hour', '%[2]s',
			'4bf92f3577b34da6a3ce929d0e0e4736'
		);`, workspaceID, jobID)
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

func Test05ShutdownPreservesDataVolume(t *testing.T) {
	requireRuntimeSmoke(t)

	stamp := time.Now().UTC().UnixNano()
	workspaceID := fmt.Sprintf("shutdown_workspace_%d", stamp)
	jobID := fmt.Sprintf("shutdown_job_%d", stamp)
	psql(t, fmt.Sprintf(`
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ('%[1]s', '%[1]s', 'Shutdown Workspace');
		INSERT INTO jobs (
			id, workspace_id, job_type, payload, max_attempts,
			available_at, idempotency_key, trace_id
		) VALUES (
			'%[2]s', '%[1]s', 'smoke.shutdown', '{}'::jsonb, 3,
			CURRENT_TIMESTAMP + INTERVAL '1 hour', '%[2]s',
			'4bf92f3577b34da6a3ce929d0e0e4736'
		);`, workspaceID, jobID))

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
	client := &http.Client{Timeout: 5 * time.Second}
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
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, smokeBaseURL()+path, nil)
		response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
			_ = response.Body.Close()
			if response.StatusCode == status {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s did not reach HTTP %d within %s", path, status, timeout)
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
