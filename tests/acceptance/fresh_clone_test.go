package acceptance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
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

const (
	freshCloneTimeout             = 45 * time.Minute
	outerAcceptanceTimeout        = 70 * time.Minute // Journey plus worst-case diagnostics, cleanup phases, and shutdown margin.
	coldBootstrapLimit            = 10 * time.Minute
	warmBootstrapLimit            = 2 * time.Minute
	developmentDiagnosticsTimeout = 30 * time.Second
	dockerTeardownTimeout         = 2 * time.Minute
	dockerInspectionTimeout       = 30 * time.Second
	goModCacheCleanupTimeout      = 10 * time.Minute
	checkoutCleanupTimeout        = 3 * time.Minute
)

var acceptanceHostLock = filepath.Join(os.TempDir(), "semlia-t008-acceptance.lock")
var errCommandDurationLimit = errors.New("command duration limit exceeded")

type freshCloneConfig struct {
	source string
	ref    string
}

type cleanupCommandExecutor func(context.Context, string, []string, string, ...string) ([]byte, error)

type freshCloneRun struct {
	t               *testing.T
	root            string
	environment     []string
	project         string
	baseURL         string
	cleanupExecutor cleanupCommandExecutor
}

func TestFreshCloneAcceptance(t *testing.T) {
	if os.Getenv("SEMLIA_RUN_FRESH_CLONE") != "1" {
		t.Skip("set SEMLIA_ACCEPTANCE_SOURCE and SEMLIA_ACCEPTANCE_REF, then run scripts/acceptance/m0-fresh-clone.sh")
	}
	if err := validateFreshCloneInvocation(); err != nil {
		t.Fatal(err)
	}

	config := freshCloneConfig{
		source: os.Getenv("SEMLIA_ACCEPTANCE_SOURCE"),
		ref:    os.Getenv("SEMLIA_ACCEPTANCE_REF"),
	}
	if err := config.validate(); err != nil {
		t.Fatal(err)
	}
	releaseLock := acquireAcceptanceLock(t)
	t.Cleanup(releaseLock)

	ctx, cancel := context.WithTimeout(context.Background(), freshCloneTimeout)
	defer cancel()
	onboardingStarted := time.Now()
	run := newFreshCloneRun(t, config)
	run.clone(ctx, config)
	run.configure(ctx)
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			if err := run.cleanup(); err != nil {
				t.Errorf("clean task-owned resources after failure: %v", err)
			}
		}
	})

	run.timedCommand(ctx, "clean", "make", "clean")
	run.timedCommand(ctx, "doctor", "make", "doctor")
	run.timedCommandWithLimit(ctx, "cold bootstrap", coldBootstrapLimit, "make", "bootstrap")
	run.timedCommandWithLimit(ctx, "warm bootstrap", warmBootstrapLimit, "make", "bootstrap")
	if module := strings.TrimSpace(run.command(ctx, "go", "list", "-m")); module != "github.com/iiwish/semlia" {
		t.Fatalf("module = %q, want github.com/iiwish/semlia", module)
	}

	run.timedCommand(ctx, "development readiness", "make", "dev")
	run.measureHTTP(ctx, onboardingStarted)
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
	cleanupErr := run.cleanup()
	cleaned = true
	if cleanupErr != nil {
		t.Fatalf("clean task-owned resources: %v", cleanupErr)
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
	return newFreshCloneRunWithID(t, config, randomRunID(t))
}

func newFreshCloneRunWithID(t *testing.T, config freshCloneConfig, runID string) *freshCloneRun {
	t.Helper()
	scratch := t.TempDir()
	httpPort, postgresPort := reserveLocalPorts(t)
	project := acceptanceProjectName(config.ref, runID)
	environment := isolatedEnvironment(scratch, project, httpPort, postgresPort)
	return &freshCloneRun{
		t:           t,
		root:        filepath.Join(scratch, "checkout"),
		environment: environment,
		project:     project,
		baseURL:     fmt.Sprintf("http://127.0.0.1:%d", httpPort),
	}
}

func acceptanceProjectName(ref, runID string) string {
	return "semlia-accept-" + ref[:8] + "-" + strings.ToLower(runID)
}

func randomRunID(t *testing.T) string {
	t.Helper()
	value := make([]byte, 6)
	if _, err := rand.Read(value); err != nil {
		t.Fatalf("generate acceptance run identity: %v", err)
	}
	return hex.EncodeToString(value)
}

func isolatedEnvironment(scratch, project string, httpPort, postgresPort int) []string {
	return isolatedEnvironmentFrom(os.Environ(), scratch, project, httpPort, postgresPort)
}

func isolatedEnvironmentFrom(base []string, scratch, project string, httpPort, postgresPort int) []string {
	environment := filterAcceptanceEnvironment(base)
	return append(environment,
		"GOENV=off",
		"GOFLAGS=",
		"GOWORK=off",
		"GOCACHE="+filepath.Join(scratch, "go-build"),
		"GOPATH="+filepath.Join(scratch, "go-path"),
		"GOMODCACHE="+filepath.Join(scratch, "go-mod"),
		"PNPM_HOME="+filepath.Join(scratch, "pnpm-home"),
		"pnpm_config_store_dir="+filepath.Join(scratch, "pnpm-store"),
		"XDG_CACHE_HOME="+filepath.Join(scratch, "cache"),
		"COMPOSE_PROJECT_NAME="+project,
		"SEMLIA_SECURITY_IMAGE=semlia:security",
		fmt.Sprintf("SEMLIA_HTTP_PORT=%d", httpPort),
		fmt.Sprintf("SEMLIA_POSTGRES_PORT=%d", postgresPort),
	)
}

func filterAcceptanceEnvironment(environment []string) []string {
	blockedExact := map[string]struct{}{
		"GO": {}, "GOARCH": {}, "GOCACHE": {}, "GOENV": {}, "GOFLAGS": {}, "GOMODCACHE": {}, "GOOS": {}, "GOPATH": {}, "GOWORK": {},
		"BASHOPTS": {}, "BASH_ENV": {}, "ENV": {}, "SHELLOPTS": {}, "GNUMAKEFLAGS": {}, "MAKE": {}, "MAKEFILES": {}, "MAKEFLAGS": {}, "MAKELEVEL": {}, "MAKEOVERRIDES": {}, "MFLAGS": {}, "PNPM": {},
		"PNPM_CONFIG_STORE_DIR": {}, "PNPM_HOME": {}, "PNPM_STORE_DIR": {}, "XDG_CACHE_HOME": {},
		"npm_config_store_dir": {}, "pnpm_config_store_dir": {},
	}
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		_, exactBlocked := blockedExact[key]
		if strings.HasPrefix(key, "SEMLIA_") || strings.HasPrefix(key, "COMPOSE_") || strings.HasPrefix(key, "GIT_") || strings.HasPrefix(key, "TESTCONTAINERS_") || exactBlocked {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func validateFreshCloneInvocation() error {
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"GOENV", os.Getenv("GOENV"), "off"},
		{"GOFLAGS", os.Getenv("GOFLAGS"), ""},
		{"GOWORK", os.Getenv("GOWORK"), "off"},
	}
	for _, check := range checks {
		if check.got != check.want {
			return fmt.Errorf("fresh-clone acceptance requires %s=%q; use scripts/acceptance/m0-fresh-clone.sh", check.name, check.want)
		}
	}
	for name, want := range map[string]string{
		"test.run":     "^TestFreshCloneAcceptance$",
		"test.count":   "1",
		"test.timeout": outerAcceptanceTimeout.String(),
	} {
		value := flag.Lookup(name)
		if value == nil || value.Value.String() != want {
			return fmt.Errorf("fresh-clone acceptance requires -%s=%q; use scripts/acceptance/m0-fresh-clone.sh", strings.TrimPrefix(name, "test."), want)
		}
	}
	return nil
}

func acquireAcceptanceLock(t *testing.T) func() {
	t.Helper()
	file, err := os.OpenFile(acceptanceHostLock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("another T008 acceptance run is active on this Docker host (%s): %v", acceptanceHostLock, err)
	}
	if _, err := fmt.Fprintf(file, "pid=%d\n", os.Getpid()); err != nil {
		_ = file.Close()
		_ = os.Remove(acceptanceHostLock)
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(acceptanceHostLock)
		t.Fatal(err)
	}
	return func() {
		if err := os.Remove(acceptanceHostLock); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove T008 host lock: %v", err)
		}
	}
}

func TestAcceptanceProjectNameUsesPerRunIdentity(t *testing.T) {
	config := freshCloneConfig{ref: strings.Repeat("a", 40)}
	first := newFreshCloneRunWithID(t, config, "000000000001")
	second := newFreshCloneRunWithID(t, config, "000000000002")
	if first.project == second.project {
		t.Fatalf("parallel runs share Compose project %q", first.project)
	}
	for _, project := range []string{first.project, second.project} {
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`).MatchString(project) {
			t.Errorf("invalid Compose project %q", project)
		}
	}
	if environmentValue(first.environment, "SEMLIA_HTTP_PORT") == environmentValue(first.environment, "SEMLIA_POSTGRES_PORT") {
		t.Error("HTTP and PostgreSQL received the same host port")
	}
}

func TestAcceptanceEnvironmentRejectsPoisonedControls(t *testing.T) {
	poisoned := []string{
		"PATH=/usr/bin", "HOME=/tmp/home", "SEMLIA_RUN_SMOKE=1", "SEMLIA_RELEASE_DIR=/outside",
		"SEMLIA_SECURITY_IMAGE=wrong", "SEMLIA_POSTGRES_PASSWORD=wrong", "COMPOSE_FILE=/outside/compose.yaml",
		"COMPOSE_PROFILES=wrong", "COMPOSE_PATH_SEPARATOR=;", "GIT_DIR=/outside/git", "GIT_CONFIG_COUNT=1",
		"GOENV=/outside/go.env", "GOFLAGS=-run=^$", "GOWORK=/outside/go.work", "GOCACHE=/outside/go-cache",
		"GOPATH=/outside/go-path", "GOMODCACHE=/outside/go-mod", "GO=false", "GOOS=plan9", "GOARCH=386",
		"BASHOPTS=extdebug", "BASH_ENV=/outside/bash-env", "ENV=/outside/env", "SHELLOPTS=noexec", "GNUMAKEFLAGS=-i", "MAKE=true", "MAKEFILES=/outside/makefile",
		"MAKEFLAGS=-i", "MAKELEVEL=99", "MAKEOVERRIDES=ACCEPTANCE_SENTINEL=poisoned", "MFLAGS=-k", "PNPM=false", "PNPM_CONFIG_STORE_DIR=/outside/pnpm",
		"PNPM_STORE_DIR=/outside/legacy-pnpm", "npm_config_store_dir=/outside/npm", "pnpm_config_store_dir=/outside/lower-pnpm",
		"TESTCONTAINERS_RYUK_DISABLED=true",
	}
	got := filterAcceptanceEnvironment(poisoned)
	if strings.Join(got, "\n") != "PATH=/usr/bin\nHOME=/tmp/home" {
		t.Fatalf("filtered environment retained control variables: %v", got)
	}
}

func TestAcceptanceEnvironmentPinsGoControls(t *testing.T) {
	poisoned := []string{
		"PATH=/usr/bin", "GOENV=/outside/go.env", "GOFLAGS=-run=^$", "GOWORK=/outside/go.work",
		"GO=false", "GOOS=plan9", "GOARCH=386", "BASHOPTS=extdebug", "BASH_ENV=/outside/bash-env", "ENV=/outside/env", "SHELLOPTS=noexec",
		"GNUMAKEFLAGS=-i", "MAKE=true", "MAKEFILES=/outside/makefile", "MAKELEVEL=99", "MAKEOVERRIDES=poisoned", "PNPM=false",
	}
	environment := isolatedEnvironmentFrom(poisoned, t.TempDir(), "semlia-accept-a1b2c3d4-000000000001", 38080, 35432)
	for name, want := range map[string]string{
		"GOENV":   "off",
		"GOFLAGS": "",
		"GOWORK":  "off",
	} {
		values := environmentValues(environment, name)
		if len(values) != 1 || values[0] != want {
			t.Errorf("%s values = %v, want exactly %q", name, values, want)
		}
	}
	for _, name := range []string{"GO", "GOOS", "GOARCH", "BASHOPTS", "BASH_ENV", "ENV", "SHELLOPTS", "GNUMAKEFLAGS", "MAKE", "MAKEFILES", "MAKEFLAGS", "MAKELEVEL", "MAKEOVERRIDES", "MFLAGS", "PNPM"} {
		if values := environmentValues(environment, name); len(values) != 0 {
			t.Errorf("%s must not be inherited: %v", name, values)
		}
	}
}

func TestAcceptanceToolCachesStayOutsideCheckout(t *testing.T) {
	scratch := t.TempDir()
	environment := isolatedEnvironment(scratch, "semlia-accept-a1b2c3d4-000000000001", 38080, 35432)
	checkout := filepath.Join(scratch, "checkout") + string(os.PathSeparator)

	for _, name := range []string{"GOCACHE", "GOPATH", "GOMODCACHE", "PNPM_HOME", "pnpm_config_store_dir", "XDG_CACHE_HOME"} {
		value := environmentValue(environment, name)
		if value == "" {
			t.Errorf("%s is not isolated", name)
			continue
		}
		if strings.HasPrefix(value+string(os.PathSeparator), checkout) {
			t.Errorf("%s=%q is inside the source checkout and can contaminate source/security gates", name, value)
		}
	}
}

func TestAcceptancePnpmStoreUsesTaskCache(t *testing.T) {
	scratch := t.TempDir()
	environment := isolatedEnvironmentFrom(
		[]string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "pnpm_config_store_dir=/outside/pnpm"},
		scratch,
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	command := exec.Command("pnpm", "store", "path")
	command.Stdin = strings.NewReader("y\n")
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve isolated pnpm store: %v: %s", err, output)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	got := filepath.Clean(strings.TrimSpace(lines[len(lines)-1]))
	wantRoot := filepath.Clean(filepath.Join(scratch, "pnpm-store"))
	if got != wantRoot && !strings.HasPrefix(got, wantRoot+string(os.PathSeparator)) {
		t.Fatalf("pnpm store = %q, want task cache below %q", got, wantRoot)
	}
	checkout := filepath.Join(scratch, "checkout") + string(os.PathSeparator)
	if strings.HasPrefix(got+string(os.PathSeparator), checkout) {
		t.Fatalf("pnpm store %q contaminates the source checkout", got)
	}
}

func TestAcceptanceMakeIgnoresInheritedMakefiles(t *testing.T) {
	scratch := t.TempDir()
	project := filepath.Join(scratch, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	poison := filepath.Join(scratch, "poison.mk")
	if err := os.WriteFile(poison, []byte("override ACCEPTANCE_SENTINEL := poisoned\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	makefile := "ACCEPTANCE_SENTINEL := clean\n.PHONY: verify\nverify:\n\t@printf '%s\\n' '$(ACCEPTANCE_SENTINEL)'\n"
	if err := os.WriteFile(filepath.Join(project, "Makefile"), []byte(makefile), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := isolatedEnvironmentFrom(
		[]string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "MAKEFILES=" + poison},
		filepath.Join(scratch, "tool-cache"),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	command := exec.Command("make", "--no-print-directory", "verify")
	command.Dir = project
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run make with isolated environment: %v: %s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "clean" {
		t.Fatalf("make output = %q, want clean; inherited MAKEFILES changed command behavior", got)
	}
}

func TestFreshCloneLauncherRejectsInvalidInputsBeforeBashEnvironmentCanExit(t *testing.T) {
	scratch := t.TempDir()
	poison := filepath.Join(scratch, "bash-env.sh")
	if err := os.WriteFile(poison, []byte("exit 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(filepath.Join(repositoryRoot, "scripts", "acceptance", "m0-fresh-clone.sh"))
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"BASH_ENV=" + poison,
		"ENV=" + poison,
		"SEMLIA_ACCEPTANCE_SOURCE=",
		"SEMLIA_ACCEPTANCE_REF=not-a-commit",
	}
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("canonical launcher falsely accepted invalid source/ref through BASH_ENV: %s", output)
	}
	if !strings.Contains(string(output), "SEMLIA_ACCEPTANCE_SOURCE") {
		t.Fatalf("launcher failure does not identify invalid source: %v: %s", err, output)
	}
}

func TestFreshCloneLauncherRejectsInvalidInputsBeforeShellOptionsCanSkipBody(t *testing.T) {
	command := exec.Command(filepath.Join(repositoryRoot, "scripts", "acceptance", "m0-fresh-clone.sh"))
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"SHELLOPTS=noexec",
		"BASHOPTS=extdebug",
		"SEMLIA_ACCEPTANCE_SOURCE=",
		"SEMLIA_ACCEPTANCE_REF=not-a-commit",
	}
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("canonical launcher falsely accepted invalid source/ref through inherited shell options: %s", output)
	}
	if !strings.Contains(string(output), "SEMLIA_ACCEPTANCE_SOURCE") {
		t.Fatalf("launcher failure does not identify invalid source: %v: %s", err, output)
	}
}

func TestAcceptanceBootstrapPerformanceThresholds(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		duration time.Duration
		limit    time.Duration
		wantErr  bool
	}{
		{name: "cold below limit", label: "cold bootstrap", duration: 9*time.Minute + 59*time.Second, limit: coldBootstrapLimit},
		{name: "cold at limit", label: "cold bootstrap", duration: 10 * time.Minute, limit: coldBootstrapLimit},
		{name: "cold above limit", label: "cold bootstrap", duration: 10*time.Minute + time.Nanosecond, limit: coldBootstrapLimit, wantErr: true},
		{name: "warm below limit", label: "warm bootstrap", duration: 90 * time.Second, limit: warmBootstrapLimit},
		{name: "warm at limit", label: "warm bootstrap", duration: 2 * time.Minute, limit: warmBootstrapLimit},
		{name: "warm above limit", label: "warm bootstrap", duration: 2*time.Minute + time.Nanosecond, limit: warmBootstrapLimit, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCommandDuration(test.label, test.duration, test.limit)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateCommandDuration() error = %v, wantErr %t", err, test.wantErr)
			}
			if err != nil && (!strings.Contains(err.Error(), test.label) || !strings.Contains(err.Error(), test.limit.String())) {
				t.Fatalf("threshold failure is not actionable: %v", err)
			}
		})
	}
}

func TestAcceptanceSleepingCommandHelper(t *testing.T) {
	if os.Getenv("SEMLIA_ACCEPTANCE_SLEEP_HELPER") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
}

func TestTimedCommandWithLimitCancelsHungCommand(t *testing.T) {
	run := &freshCloneRun{
		t:           t,
		root:        t.TempDir(),
		environment: append(filteredEnvironment(os.Environ(), "SEMLIA_ACCEPTANCE_SLEEP_HELPER"), "SEMLIA_ACCEPTANCE_SLEEP_HELPER=1"),
	}
	limit := 75 * time.Millisecond
	started := time.Now()
	duration, err := run.executeTimedCommandWithLimit(
		context.Background(),
		"cold bootstrap",
		limit,
		os.Args[0],
		"-test.run=^TestAcceptanceSleepingCommandHelper$",
		"-test.count=1",
	)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("hung bootstrap command was not cancelled at its NFR limit")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation error = %v, want context deadline exceeded", err)
	}
	for _, fragment := range []string{"cold bootstrap", limit.String(), "command cancelled"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("cancellation error missing %q: %v", fragment, err)
		}
	}
	if duration < limit || elapsed < limit {
		t.Fatalf("command cancelled too early: duration=%s elapsed=%s limit=%s", duration, elapsed, limit)
	}
	if duration > 2*time.Second || elapsed > 2*time.Second {
		t.Fatalf("command cancellation was not prompt: duration=%s elapsed=%s", duration, elapsed)
	}
}

func TestFreshCloneLauncherReservesCleanupEnvelope(t *testing.T) {
	launcher := readRepositoryFile(t, "scripts/acceptance/m0-fresh-clone.sh")
	if strings.Count(launcher, "-timeout=70m") != 1 {
		t.Fatalf("canonical launcher must set exactly one 70m outer timeout:\n%s", launcher)
	}
	if !strings.Contains(readRepositoryFile(t, "tests/acceptance/fresh_clone_test.go"), `"test.timeout": outerAcceptanceTimeout.String()`) {
		t.Fatal("fresh-clone invocation validator does not enforce the canonical outer timeout")
	}
}

func TestFreshCloneInvocationRejectsNoncanonicalOuterTimeout(t *testing.T) {
	command := exec.Command(
		os.Args[0],
		"-test.run=^TestFreshCloneAcceptance$",
		"-test.count=1",
		"-test.timeout=69m",
	)
	command.Env = append(
		filterAcceptanceEnvironment(os.Environ()),
		"GOENV=off",
		"GOFLAGS=",
		"GOWORK=off",
		"SEMLIA_RUN_FRESH_CLONE=1",
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("fresh-clone invocation accepted noncanonical outer timeout:\n%s", output)
	}
	want := `fresh-clone acceptance requires -timeout="1h10m0s"`
	if !strings.Contains(string(output), want) {
		t.Fatalf("outer-timeout failure is not actionable; want %q:\n%s", want, output)
	}
}

func TestDevelopmentReadinessFailureCapturesRedactedComposeDiagnostics(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	composeDir := filepath.Join(root, "scripts", "dev")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".semlia"), 0o755); err != nil {
		t.Fatal(err)
	}
	makeStub := "#!/bin/sh\nprintf 'primary failure secret=%s\\n' \"$SEMLIA_SECRET_KEY\"\nexit 17\n"
	if err := os.WriteFile(filepath.Join(bin, "make"), []byte(makeStub), 0o755); err != nil {
		t.Fatal(err)
	}
	composeStub := `#!/bin/sh
case "$*" in
  "ps --all")
    printf 'migrate exited postgres unhealthy secret=%s\n' "$SEMLIA_SECRET_KEY"
    ;;
  "logs --no-color migrate postgres")
    cat .semlia/dev.env
    printf 'migrate failed postgres rejected secret=%s\n' "$SEMLIA_SECRET_KEY"
    ;;
  *)
    printf 'unexpected compose arguments: %s\n' "$*" >&2
    exit 64
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(composeDir, "compose.sh"), []byte(composeStub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".semlia", "dev.env"), []byte("SEMLIA_POSTGRES_PASSWORD=file-secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path := bin + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", path)
	run := &freshCloneRun{
		t:           t,
		root:        root,
		environment: []string{"PATH=" + path, "SEMLIA_SECRET_KEY=task-secret-value"},
		project:     "semlia-accept-a1b2c3d4-000000000001",
	}
	_, err := run.executeCommand(context.Background(), "make", "dev")
	if err == nil {
		t.Fatal("make dev failure was converted to success after diagnostics")
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 17 {
		t.Fatalf("original make dev exit was not preserved: %T %v", err, err)
	}
	message := err.Error()
	for _, fragment := range []string{
		"make dev: exit status 17",
		"primary failure",
		"compose ps --all",
		"migrate exited postgres unhealthy",
		"compose logs --no-color migrate postgres",
		"migrate failed postgres rejected",
		"[REDACTED]",
	} {
		if !strings.Contains(message, fragment) {
			t.Errorf("development failure missing %q:\n%s", fragment, message)
		}
	}
	for _, secret := range []string{"task-secret-value", "file-secret-value"} {
		if strings.Contains(message, secret) {
			t.Errorf("development diagnostics leaked task secret %q:\n%s", secret, message)
		}
	}
}

func TestDevelopmentDiagnosticsFailurePreservesPrimaryExit(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	composeDir := filepath.Join(root, "scripts", "dev")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "make"), []byte("#!/bin/sh\nexit 17\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(composeDir, "compose.sh"), []byte("#!/bin/sh\nprintf 'diagnostic failure\\n' >&2\nexit 23\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	path := bin + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", path)
	run := &freshCloneRun{t: t, root: root, environment: []string{"PATH=" + path}}
	_, err := run.executeCommand(context.Background(), "make", "dev")
	if err == nil {
		t.Fatal("make dev failure was converted to success when diagnostics failed")
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 17 {
		t.Fatalf("diagnostic failure replaced primary exit: %T %v", err, err)
	}
	if got := strings.Count(err.Error(), "diagnostic command failed: exit status 23"); got != 2 {
		t.Fatalf("diagnostic failures = %d, want 2:\n%s", got, err)
	}
}

func TestCleanupUsesIndependentPhaseBudgets(t *testing.T) {
	type call struct {
		name      string
		arguments string
		remaining time.Duration
	}
	var calls []call
	run := &freshCloneRun{
		root:    t.TempDir(),
		project: "semlia-accept-a1b2c3d4-000000000001",
		cleanupExecutor: func(ctx context.Context, dir string, environment []string, name string, args ...string) ([]byte, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatalf("cleanup command %s %s has no deadline", name, strings.Join(args, " "))
			}
			if err := ctx.Err(); err != nil {
				t.Fatalf("cleanup command %s %s inherited an exhausted context: %v", name, strings.Join(args, " "), err)
			}
			calls = append(calls, call{name: name, arguments: strings.Join(args, " "), remaining: time.Until(deadline)})
			switch len(calls) {
			case 1:
				return nil, context.DeadlineExceeded
			case 5:
				return []byte("large cache cleanup failed"), errors.New("cache failure")
			default:
				return nil, nil
			}
		},
	}
	err := run.cleanup()
	if err == nil {
		t.Fatal("cleanup failures were not reported")
	}
	for _, fragment := range []string{"compose down", "context deadline exceeded", "clean Go module cache", "cache failure", "large cache cleanup failed"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("cleanup failure missing %q: %v", fragment, err)
		}
	}
	if len(calls) != 6 {
		t.Fatalf("cleanup calls = %d, want 6: %#v", len(calls), calls)
	}
	if calls[4].name != "go" || calls[4].arguments != "clean -modcache" {
		t.Fatalf("Go cache cleanup did not run after Docker timeout: %#v", calls)
	}
	if calls[5].name != "make" || calls[5].arguments != "clean" {
		t.Fatalf("checkout cleanup did not run after Go cache failure: %#v", calls)
	}
	expected := []time.Duration{
		dockerTeardownTimeout,
		dockerInspectionTimeout,
		dockerInspectionTimeout,
		dockerInspectionTimeout,
		goModCacheCleanupTimeout,
		checkoutCleanupTimeout,
	}
	for index, current := range calls {
		if current.remaining <= expected[index]-time.Second || current.remaining > expected[index] {
			t.Errorf("cleanup call %d budget = %s, want fresh budget near %s", index+1, current.remaining, expected[index])
		}
	}
}

func (run *freshCloneRun) clone(ctx context.Context, config freshCloneConfig) {
	run.t.Helper()
	clone := acceptanceCommandContext(ctx, "git", "clone", "--no-checkout", "--", config.source, run.root)
	clone.Env = run.environment
	if output, err := clone.CombinedOutput(); err != nil {
		run.t.Fatalf("clone isolated checkout: %v\n%s", err, output)
	}
	ancestor := acceptanceCommandContext(ctx, "git", "-C", config.source, "merge-base", "--is-ancestor", config.ref, "HEAD")
	ancestor.Env = run.environment
	if reachable := ancestor.Run(); reachable != nil {
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

func (run *freshCloneRun) measureHTTP(ctx context.Context, onboardingStarted time.Time) {
	run.t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	started := time.Now()
	run.request(ctx, client, "/api/v1/system/info", http.StatusOK)
	run.t.Logf("first successful request: %s", time.Since(started).Round(time.Millisecond))
	onboardingDuration := time.Since(onboardingStarted)
	run.t.Logf("clone to first successful request: %s", onboardingDuration.Round(time.Millisecond))
	if onboardingDuration > 15*time.Minute {
		run.t.Fatalf("clone to first successful request exceeded 15 minutes: %s", onboardingDuration)
	}

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
	run.timedCommand(
		ctx,
		"migration upgrade, migration downgrade, migration re-upgrade (isolated PostgreSQL)",
		"go", "test", "./tests/integration/db/...", "-run", "^TestMigrationLifecycleAndTenantSchema$", "-count=1",
	)
}

func (run *freshCloneRun) verifyWorkerJourney(ctx context.Context) {
	run.t.Helper()
	run.timedCommand(
		ctx,
		"worker lease retry dead-letter (isolated PostgreSQL)",
		"go", "test", "./tests/integration/worker/...",
		"-run", "^(TestConcurrentClaimCreatesOneLease|TestWorkerRetriesThenDeadLettersWithStableErrorCode)$", "-count=1",
	)
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

	command := acceptanceCommandContext(ctx, "make", "contracts-check")
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

	command := acceptanceCommandContext(ctx, "go", "run", "./cmd/semlia", "server")
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

func (run *freshCloneRun) timedCommand(ctx context.Context, label, name string, args ...string) time.Duration {
	run.t.Helper()
	started := time.Now()
	run.command(ctx, name, args...)
	duration := time.Since(started)
	run.t.Logf("%s: %s", label, duration.Round(time.Millisecond))
	return duration
}

func (run *freshCloneRun) timedCommandWithLimit(ctx context.Context, label string, limit time.Duration, name string, args ...string) {
	run.t.Helper()
	duration, err := run.executeTimedCommandWithLimit(ctx, label, limit, name, args...)
	run.t.Logf("%s: %s", label, duration.Round(time.Millisecond))
	if err != nil {
		run.t.Fatal(err)
	}
}

func (run *freshCloneRun) executeTimedCommandWithLimit(ctx context.Context, label string, limit time.Duration, name string, args ...string) (time.Duration, error) {
	limitedContext, cancel := context.WithTimeoutCause(ctx, limit, errCommandDurationLimit)
	defer cancel()
	started := time.Now()
	_, commandErr := run.executeCommand(limitedContext, name, args...)
	duration := time.Since(started)

	if errors.Is(context.Cause(limitedContext), errCommandDurationLimit) {
		thresholdErr := fmt.Errorf("%s exceeded %s after %s; command cancelled: %w", label, limit, duration, context.DeadlineExceeded)
		return duration, errors.Join(thresholdErr, commandErr)
	}
	if commandErr != nil {
		return duration, commandErr
	}
	return duration, validateCommandDuration(label, duration, limit)
}

func validateCommandDuration(label string, duration, limit time.Duration) error {
	if duration > limit {
		return fmt.Errorf("%s exceeded %s: %s", label, limit, duration)
	}
	return nil
}

func (run *freshCloneRun) command(ctx context.Context, name string, args ...string) string {
	run.t.Helper()
	output, err := run.executeCommand(ctx, name, args...)
	if err != nil {
		run.t.Fatal(err)
	}
	return output
}

func (run *freshCloneRun) executeCommand(ctx context.Context, name string, args ...string) (string, error) {
	command := acceptanceCommandContext(ctx, name, args...)
	command.Dir = run.root
	command.Env = run.environment
	output, err := command.CombinedOutput()
	if err == nil {
		return string(output), nil
	}

	details := strings.TrimSpace(run.redactTaskSecrets(string(output)))
	if details != "" {
		details = "\n" + details
	}
	if name == "make" && len(args) == 1 && args[0] == "dev" {
		details += "\n\nCompose diagnostics (redacted):\n" + run.developmentDiagnostics()
	}
	return "", fmt.Errorf("%s %s: %w%s", name, strings.Join(args, " "), err, details)
}

func (run *freshCloneRun) developmentDiagnostics() string {
	type diagnostic struct {
		label string
		args  []string
	}
	diagnostics := []diagnostic{
		{label: "compose ps --all", args: []string{"ps", "--all"}},
		{label: "compose logs --no-color migrate postgres", args: []string{"logs", "--no-color", "migrate", "postgres"}},
	}
	var report strings.Builder
	for index, diagnostic := range diagnostics {
		if index > 0 {
			report.WriteString("\n")
		}
		fmt.Fprintf(&report, "$ %s\n", diagnostic.label)
		ctx, cancel := context.WithTimeout(context.Background(), developmentDiagnosticsTimeout)
		output, err := executeSubprocess(
			ctx,
			run.root,
			run.environment,
			filepath.Join(run.root, "scripts", "dev", "compose.sh"),
			diagnostic.args...,
		)
		cancel()
		redacted := strings.TrimSpace(run.redactTaskSecrets(string(output)))
		if redacted != "" {
			report.WriteString(redacted)
			report.WriteString("\n")
		}
		if err != nil {
			fmt.Fprintf(&report, "diagnostic command failed: %v\n", err)
		}
	}
	return strings.TrimSpace(report.String())
}

func (run *freshCloneRun) redactTaskSecrets(output string) string {
	secrets := make(map[string]struct{})
	collect := func(entries []string) {
		for _, entry := range entries {
			entry = strings.TrimSpace(entry)
			if entry == "" || strings.HasPrefix(entry, "#") {
				continue
			}
			key, value, found := strings.Cut(entry, "=")
			if !found || !isSensitiveEnvironmentKey(key) {
				continue
			}
			value = strings.Trim(strings.TrimSpace(value), `"'`)
			if value != "" {
				secrets[value] = struct{}{}
			}
		}
	}
	collect(run.environment)
	if content, err := os.ReadFile(filepath.Join(run.root, ".semlia", "dev.env")); err == nil {
		collect(strings.Split(string(content), "\n"))
	}

	values := make([]string, 0, len(secrets))
	for secret := range secrets {
		values = append(values, secret)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, secret := range values {
		output = strings.ReplaceAll(output, secret, "[REDACTED]")
	}
	return output
}

func isSensitiveEnvironmentKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	for _, marker := range []string{"PASSWORD", "SECRET", "TOKEN", "CREDENTIAL", "DATABASE_URL", "PRIVATE_KEY", "API_KEY"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func (run *freshCloneRun) compose(ctx context.Context, args ...string) string {
	run.t.Helper()
	return run.command(ctx, filepath.Join(run.root, "scripts", "dev", "compose.sh"), args...)
}

func (run *freshCloneRun) cleanup() error {
	var problems []error
	if output, err := run.cleanupCommand(
		dockerTeardownTimeout,
		filepath.Join(run.root, "scripts", "dev", "compose.sh"),
		"down", "--volumes", "--remove-orphans",
	); err != nil {
		problems = append(problems, fmt.Errorf("compose down: %w: %s", err, strings.TrimSpace(run.redactTaskSecrets(string(output)))))
	}

	checks := []struct {
		label string
		args  []string
	}{
		{"containers", []string{"ps", "--all", "--filter", "label=com.docker.compose.project=" + run.project, "--format", "{{.ID}}"}},
		{"networks", []string{"network", "ls", "--filter", "label=com.docker.compose.project=" + run.project, "--format", "{{.ID}}"}},
		{"volumes", []string{"volume", "ls", "--filter", "label=com.docker.compose.project=" + run.project, "--format", "{{.Name}}"}},
	}
	for _, check := range checks {
		output, err := run.cleanupCommand(dockerInspectionTimeout, "docker", check.args...)
		if err != nil {
			problems = append(problems, fmt.Errorf("inspect task-owned %s: %w: %s", check.label, err, strings.TrimSpace(run.redactTaskSecrets(string(output)))))
			continue
		}
		if remaining := strings.TrimSpace(string(output)); remaining != "" {
			problems = append(problems, fmt.Errorf("task-owned %s remain: %s", check.label, remaining))
		}
	}
	for _, cleanup := range []struct {
		label   string
		timeout time.Duration
		name    string
		args    []string
	}{
		{"Go module cache", goModCacheCleanupTimeout, "go", []string{"clean", "-modcache"}},
		{"generated checkout state", checkoutCleanupTimeout, "make", []string{"clean"}},
	} {
		if output, err := run.cleanupCommand(cleanup.timeout, cleanup.name, cleanup.args...); err != nil {
			problems = append(problems, fmt.Errorf("clean %s: %w: %s", cleanup.label, err, strings.TrimSpace(run.redactTaskSecrets(string(output)))))
		}
	}
	return errors.Join(problems...)
}

func (run *freshCloneRun) cleanupCommand(timeout time.Duration, name string, args ...string) ([]byte, error) {
	executor := run.cleanupExecutor
	if executor == nil {
		executor = executeSubprocess
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return executor(ctx, run.root, run.environment, name, args...)
}

func executeSubprocess(ctx context.Context, dir string, environment []string, name string, args ...string) ([]byte, error) {
	command := acceptanceCommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = environment
	return command.CombinedOutput()
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

func environmentValues(environment []string, name string) []string {
	prefix := name + "="
	var values []string
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			values = append(values, strings.TrimPrefix(entry, prefix))
		}
	}
	return values
}

func reserveLocalPorts(t *testing.T) (int, int) {
	t.Helper()
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	return first.Addr().(*net.TCPAddr).Port, second.Addr().(*net.TCPAddr).Port
}
