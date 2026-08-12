package acceptance

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

const freshCloneTimeout = 45 * time.Minute

type freshCloneConfig struct {
	source string
	ref    string
}

type freshCloneRun struct {
	t           *testing.T
	root        string
	environment []string
	project     string
	baseURL     string
}

func TestFreshCloneAcceptance(t *testing.T) {
	if os.Getenv("SEMLIA_RUN_FRESH_CLONE") != "1" {
		t.Skip("set SEMLIA_RUN_FRESH_CLONE=1 with SEMLIA_ACCEPTANCE_SOURCE and SEMLIA_ACCEPTANCE_REF")
	}

	config := freshCloneConfig{
		source: os.Getenv("SEMLIA_ACCEPTANCE_SOURCE"),
		ref:    os.Getenv("SEMLIA_ACCEPTANCE_REF"),
	}
	if err := config.validate(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), freshCloneTimeout)
	defer cancel()
	run := newFreshCloneRun(t, config)
	run.clone(ctx, config)
	run.configure(ctx)
	t.Cleanup(func() { run.cleanup() })

	run.timedCommand(ctx, "clean", "make", "clean")
	run.timedCommand(ctx, "doctor", "make", "doctor")
	run.timedCommand(ctx, "cold bootstrap", "make", "bootstrap")
	run.timedCommand(ctx, "warm bootstrap", "make", "bootstrap")
	if module := strings.TrimSpace(run.command(ctx, "go", "list", "-m")); module != "github.com/iiwish/semlia" {
		t.Fatalf("module = %q, want github.com/iiwish/semlia", module)
	}

	run.timedCommand(ctx, "development readiness", "make", "dev")
	run.measureHTTP(ctx)
	run.verifyDependencyFailureAndRecovery(ctx)
	run.timedCommand(ctx, "smoke", "make", "smoke")
	run.verifyWorkerJourney(ctx)
	run.verifyMigrationJourney(ctx)
	run.verifyContractDrift(ctx)
	run.verifyProductionFailClosed(ctx)
	run.timedCommand(ctx, "complete pull-request gate", "make", "check")
	run.timedCommand(ctx, "release bundle", "make", "release")
	run.verifyReleaseBundle(ctx)

	if status := strings.TrimSpace(run.command(ctx, "git", "status", "--short")); status != "" {
		t.Fatalf("fresh checkout was mutated:\n%s", status)
	}
}

func (config freshCloneConfig) validate() error {
	if config.source == "" {
		return fmt.Errorf("SEMLIA_ACCEPTANCE_SOURCE is required")
	}
	info, err := os.Stat(config.source)
	if err != nil {
		return fmt.Errorf("inspect SEMLIA_ACCEPTANCE_SOURCE: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("SEMLIA_ACCEPTANCE_SOURCE must be a local repository directory")
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(config.ref) {
		return fmt.Errorf("SEMLIA_ACCEPTANCE_REF must be an exact 40-character lowercase commit")
	}
	return nil
}

func newFreshCloneRun(t *testing.T, config freshCloneConfig) *freshCloneRun {
	t.Helper()
	scratch := t.TempDir()
	project := "semlia-acceptance-" + config.ref[:12]
	httpPort := reserveLocalPort(t)
	postgresPort := reserveLocalPort(t)

	environment := filteredEnvironment(os.Environ(),
		"COMPOSE_PROJECT_NAME",
		"SEMLIA_ACCEPTANCE_REF",
		"SEMLIA_ACCEPTANCE_SOURCE",
		"SEMLIA_RUN_FRESH_CLONE",
		"SEMLIA_HTTP_PORT",
		"SEMLIA_POSTGRES_PORT",
	)
	environment = append(environment,
		"GOCACHE="+filepath.Join(scratch, "go-build"),
		"GOPATH="+filepath.Join(scratch, "go-path"),
		"GOMODCACHE="+filepath.Join(scratch, "go-mod"),
		"PNPM_HOME="+filepath.Join(scratch, "pnpm-home"),
		"PNPM_STORE_DIR="+filepath.Join(scratch, "pnpm-store"),
		"npm_config_store_dir="+filepath.Join(scratch, "pnpm-store"),
		"XDG_CACHE_HOME="+filepath.Join(scratch, "cache"),
		"COMPOSE_PROJECT_NAME="+project,
		fmt.Sprintf("SEMLIA_HTTP_PORT=%d", httpPort),
		fmt.Sprintf("SEMLIA_POSTGRES_PORT=%d", postgresPort),
	)

	return &freshCloneRun{
		t:           t,
		root:        filepath.Join(scratch, "checkout"),
		environment: environment,
		project:     project,
		baseURL:     fmt.Sprintf("http://127.0.0.1:%d", httpPort),
	}
}

func (run *freshCloneRun) clone(ctx context.Context, config freshCloneConfig) {
	run.t.Helper()
	clone := exec.CommandContext(ctx, "git", "clone", "--no-checkout", "--", config.source, run.root)
	clone.Env = run.environment
	if output, err := clone.CombinedOutput(); err != nil {
		run.t.Fatalf("clone isolated checkout: %v\n%s", err, output)
	}
	if reachable := exec.CommandContext(ctx, "git", "-C", config.source, "merge-base", "--is-ancestor", config.ref, "HEAD").Run(); reachable != nil {
		run.t.Fatalf("SEMLIA_ACCEPTANCE_REF must be reachable from source HEAD: %v", reachable)
	}
	run.command(ctx, "git", "checkout", "--detach", config.ref)
	if revision := strings.TrimSpace(run.command(ctx, "git", "rev-parse", "HEAD")); revision != config.ref {
		run.t.Fatalf("checkout revision = %q, want %q", revision, config.ref)
	}
}

func (run *freshCloneRun) configure(ctx context.Context) {
	run.t.Helper()
	run.command(ctx, filepath.Join(run.root, "scripts", "dev", "ensure-env.sh"))
	path := filepath.Join(run.root, ".semlia", "dev.env")
	content, err := os.ReadFile(path)
	if err != nil {
		run.t.Fatal(err)
	}
	values := map[string]string{
		"COMPOSE_PROJECT_NAME": run.project,
		"SEMLIA_HTTP_PORT":     environmentValue(run.environment, "SEMLIA_HTTP_PORT"),
		"SEMLIA_POSTGRES_PORT": environmentValue(run.environment, "SEMLIA_POSTGRES_PORT"),
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	for index, line := range lines {
		key, _, found := strings.Cut(line, "=")
		if replacement, ok := values[key]; found && ok {
			lines[index] = key + "=" + replacement
			delete(values, key)
		}
	}
	for key, value := range values {
		lines = append(lines, key+"="+value)
	}
	sort.Strings(lines)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		run.t.Fatal(err)
	}
}

func (run *freshCloneRun) measureHTTP(ctx context.Context) {
	run.t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	started := time.Now()
	run.request(ctx, client, "/api/v1/system/info", http.StatusOK)
	run.t.Logf("first successful request: %s", time.Since(started).Round(time.Millisecond))

	for range 5 {
		run.request(ctx, client, "/api/v1/system/info", http.StatusOK)
	}
	durations := make([]time.Duration, 0, 100)
	for range 100 {
		started = time.Now()
		run.request(ctx, client, "/api/v1/system/info", http.StatusOK)
		durations = append(durations, time.Since(started))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := durations[49]
	p95 := durations[94]
	maximum := durations[len(durations)-1]
	run.t.Logf("100 sequential system-info requests: errors=0 p50=%s p95=%s max=%s", p50.Round(time.Microsecond), p95.Round(time.Microsecond), maximum.Round(time.Microsecond))
	if p95 > 250*time.Millisecond || maximum >= time.Second {
		run.t.Fatalf("HTTP latency exceeds acceptance threshold: p95=%s max=%s", p95, maximum)
	}
}

func (run *freshCloneRun) verifyDependencyFailureAndRecovery(ctx context.Context) {
	run.t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	run.compose(ctx, "stop", "postgres")
	run.waitForStatus(ctx, client, "/health/live", http.StatusOK, 30*time.Second)
	body := run.waitForStatus(ctx, client, "/health/ready", http.StatusServiceUnavailable, 30*time.Second)
	if !strings.Contains(body, `"code":"DEPENDENCY_UNAVAILABLE"`) || !strings.Contains(body, `"traceId":"`) {
		run.t.Fatalf("readiness failure lacks stable code or traceId: %s", body)
	}
	run.compose(ctx, "start", "postgres")
	run.waitForStatus(ctx, client, "/health/ready", http.StatusOK, 90*time.Second)
}

func (run *freshCloneRun) verifyMigrationJourney(ctx context.Context) {
	run.t.Helper()
	baseEnvironment := run.environment
	run.environment = append(filteredEnvironment(baseEnvironment, "SEMLIA_DATABASE_URL"),
		"SEMLIA_DATABASE_URL="+run.databaseURL(),
	)
	defer func() { run.environment = baseEnvironment }()

	run.timedCommand(ctx, "migration upgrade", "make", "db-migrate-up")
	before := strings.TrimSpace(run.command(ctx, "make", "db-migrate-version"))
	run.timedCommand(ctx, "migration downgrade", "make", "db-migrate-down")
	afterDown := strings.TrimSpace(run.command(ctx, "make", "db-migrate-version"))
	if before == afterDown {
		run.t.Fatalf("migration version did not change after downgrade: %q", before)
	}
	run.timedCommand(ctx, "migration re-upgrade", "make", "db-migrate-up")
	afterUp := strings.TrimSpace(run.command(ctx, "make", "db-migrate-version"))
	if afterUp != before {
		run.t.Fatalf("migration version after re-upgrade = %q, want %q", afterUp, before)
	}
}

func (run *freshCloneRun) verifyWorkerJourney(ctx context.Context) {
	run.t.Helper()
	baseEnvironment := run.environment
	run.environment = append(filteredEnvironment(baseEnvironment, "SEMLIA_RUN_POSTGRES_INTEGRATION", "SEMLIA_TEST_DATABASE_URL"),
		"SEMLIA_RUN_POSTGRES_INTEGRATION=1",
		"SEMLIA_TEST_DATABASE_URL="+run.databaseURL(),
	)
	defer func() { run.environment = baseEnvironment }()
	run.timedCommand(ctx, "worker lease retry dead-letter", "go", "test", "./tests/integration/worker/...", "-count=1")
}

func (run *freshCloneRun) verifyContractDrift(ctx context.Context) {
	run.t.Helper()
	path := filepath.Join(run.root, "sdk", "typescript", "src", "schema.gen.ts")
	original, err := os.ReadFile(path)
	if err != nil {
		run.t.Fatal(err)
	}
	if err := os.WriteFile(path, append(original, []byte("\n// acceptance drift\n")...), 0o644); err != nil {
		run.t.Fatal(err)
	}
	restored := false
	defer func() {
		if !restored {
			_ = os.WriteFile(path, original, 0o644)
		}
	}()

	command := exec.CommandContext(ctx, "make", "contracts-check")
	command.Dir = run.root
	command.Env = run.environment
	output, err := command.CombinedOutput()
	if err == nil {
		run.t.Fatal("contract drift gate accepted a modified generated artifact")
	}
	if !strings.Contains(string(output), "Generated artifact is stale: sdk/typescript/src/schema.gen.ts") {
		run.t.Fatalf("contract drift failure was not actionable:\n%s", output)
	}
	if err := os.WriteFile(path, original, 0o644); err != nil {
		run.t.Fatal(err)
	}
	restored = true
	run.timedCommand(ctx, "contract drift recovery", "make", "contracts-check")
	run.t.Log("contract drift gate rejected and then accepted the restored generated artifact")
}

func (run *freshCloneRun) verifyProductionFailClosed(ctx context.Context) {
	run.t.Helper()
	baseEnvironment := run.environment
	run.environment = append(filteredEnvironment(baseEnvironment,
		"SEMLIA_ENV",
		"SEMLIA_HTTP_ADDR",
		"SEMLIA_DATABASE_URL",
		"SEMLIA_ALLOWED_ORIGINS",
		"SEMLIA_SECRET_KEY",
	),
		"SEMLIA_ENV=production",
		"SEMLIA_HTTP_ADDR=127.0.0.1:0",
		"SEMLIA_DATABASE_URL=postgresql://semlia@localhost:5432/semlia?sslmode=disable",
		"SEMLIA_ALLOWED_ORIGINS=*",
		"SEMLIA_SECRET_KEY=short",
	)
	defer func() { run.environment = baseEnvironment }()

	command := exec.CommandContext(ctx, "go", "run", "./cmd/semlia", "server")
	command.Dir = run.root
	command.Env = run.environment
	output, err := command.CombinedOutput()
	if err == nil {
		run.t.Fatal("production server accepted insecure configuration")
	}
	body := string(output)
	for _, field := range []string{"SEMLIA_ALLOWED_ORIGINS", "SEMLIA_DATABASE_URL", "SEMLIA_SECRET_KEY"} {
		if !strings.Contains(body, field) {
			run.t.Fatalf("production fail-closed output missing %s: %s", field, body)
		}
	}
	run.t.Logf("production fail-closed: rejected insecure origin, database TLS and secret settings")
}

func (run *freshCloneRun) verifyReleaseBundle(ctx context.Context) {
	run.t.Helper()
	output := filepath.Join(run.root, "build", "release")
	entries, err := os.ReadDir(output)
	if err != nil {
		run.t.Fatalf("read release output: %v", err)
	}
	var archive, sbom bool
	for _, entry := range entries {
		switch {
		case strings.HasSuffix(entry.Name(), ".tar.gz"):
			archive = true
		case strings.HasSuffix(entry.Name(), ".sbom.cdx.json"):
			sbom = true
		}
	}
	if !archive || !sbom {
		run.t.Fatalf("release output missing archive or CycloneDX SBOM: archive=%t sbom=%t", archive, sbom)
	}
	checksums, err := os.ReadFile(filepath.Join(output, "SHA256SUMS"))
	if err != nil || len(strings.Fields(string(checksums))) < 4 {
		run.t.Fatalf("release output missing complete SHA256SUMS: %v", err)
	}
	run.t.Log("release output contains archive, standalone CycloneDX SBOM and checksum subjects")
}

func (run *freshCloneRun) databaseURL() string {
	run.t.Helper()
	port := environmentValue(run.environment, "SEMLIA_POSTGRES_PORT")
	content, err := os.ReadFile(filepath.Join(run.root, ".semlia", "dev.env"))
	if err != nil {
		run.t.Fatal(err)
	}
	password := ""
	for _, line := range strings.Split(string(content), "\n") {
		if value, found := strings.CutPrefix(line, "SEMLIA_POSTGRES_PASSWORD="); found {
			password = value
			break
		}
	}
	if password == "" {
		run.t.Fatal("PostgreSQL password was empty")
	}
	return fmt.Sprintf("postgresql://semlia:%s@127.0.0.1:%s/semlia?sslmode=disable", password, port)
}

func (run *freshCloneRun) request(ctx context.Context, client *http.Client, path string, wantStatus int) string {
	run.t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, run.baseURL+path, nil)
	if err != nil {
		run.t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		run.t.Fatalf("GET %s: %v", path, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		run.t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		run.t.Fatalf("GET %s status = %d, want %d: %s", path, response.StatusCode, wantStatus, body)
	}
	return string(body)
}

func (run *freshCloneRun) waitForStatus(ctx context.Context, client *http.Client, path string, status int, timeout time.Duration) string {
	run.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, run.baseURL+path, nil)
		if err != nil {
			run.t.Fatal(err)
		}
		response, err := client.Do(request)
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
			_ = response.Body.Close()
			if readErr == nil && response.StatusCode == status {
				return string(body)
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	run.t.Fatalf("%s did not reach HTTP %d within %s", path, status, timeout)
	return ""
}

func (run *freshCloneRun) timedCommand(ctx context.Context, label, name string, args ...string) {
	run.t.Helper()
	started := time.Now()
	run.command(ctx, name, args...)
	run.t.Logf("%s: %s", label, time.Since(started).Round(time.Millisecond))
}

func (run *freshCloneRun) command(ctx context.Context, name string, args ...string) string {
	run.t.Helper()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = run.root
	command.Env = run.environment
	output, err := command.CombinedOutput()
	if err != nil {
		run.t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}

func (run *freshCloneRun) compose(ctx context.Context, args ...string) string {
	run.t.Helper()
	return run.command(ctx, filepath.Join(run.root, "scripts", "dev", "compose.sh"), args...)
}

func (run *freshCloneRun) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(run.root, "scripts", "dev", "compose.sh"), "down", "--volumes", "--remove-orphans")
	command.Dir = run.root
	command.Env = run.environment
	if output, err := command.CombinedOutput(); err != nil {
		run.t.Logf("task-owned cleanup failed: %v\n%s", err, output)
	}
}

func filteredEnvironment(environment []string, remove ...string) []string {
	blocked := map[string]struct{}{}
	for _, key := range remove {
		blocked[key] = struct{}{}
	}
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := blocked[key]; !ok {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func environmentValue(environment []string, name string) string {
	prefix := name + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func reserveLocalPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
