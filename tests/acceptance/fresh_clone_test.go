package acceptance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

	"go.yaml.in/yaml/v3"
)

const (
	freshCloneTimeout             = 45 * time.Minute
	outerAcceptanceTimeout        = 100 * time.Minute // Journey plus worst-case diagnostics, cleanup phases, and shutdown margin.
	coldBootstrapLimit            = 10 * time.Minute
	warmBootstrapLimit            = 2 * time.Minute
	pullRequestGateLimit          = 10 * time.Minute
	developmentDiagnosticsTimeout = 30 * time.Second
	dockerTeardownTimeout         = 2 * time.Minute
	dockerInspectionTimeout       = 30 * time.Second
	goModCacheCleanupTimeout      = 10 * time.Minute
	checkoutCleanupTimeout        = 3 * time.Minute
	acceptanceDockerLabelKey      = "io.semlia.acceptance.run"
	composeServiceContainerLimit  = 4 // compose.yaml defines postgres, migrate, server, and worker.
	composeNetworkLimit           = 1 // compose.yaml defines the backend network.
	composeVolumeLimit            = 1 // compose.yaml defines the postgres-data volume.
	toolContainerLimit            = 3 // Two security scans plus one release SBOM invocation.
)

var acceptanceHostLock = filepath.Join(os.TempDir(), "semlia-t008-acceptance.lock")
var errCommandDurationLimit = errors.New("command duration limit exceeded")

type freshCloneConfig struct {
	source string
	ref    string
}

type cleanupCommandExecutor func(context.Context, string, []string, string, ...string) ([]byte, error)

type cleanupResourceKind struct {
	label         string
	listArgs      []string
	removeArgs    []string
	identityLabel string
	maxIdentities int
}

type freshCloneRun struct {
	t               *testing.T
	root            string
	environment     []string
	project         string
	baseURL         string
	cleanupExecutor cleanupCommandExecutor
}

type cleanupReport struct {
	dockerProblems map[string]error
	otherProblems  []error
}

func (report *cleanupReport) addOther(err error) {
	if err != nil {
		report.otherProblems = append(report.otherProblems, err)
	}
}

func (report *cleanupReport) setDockerProblem(label string, err error) {
	if report.dockerProblems == nil {
		report.dockerProblems = make(map[string]error)
	}
	if err == nil {
		delete(report.dockerProblems, label)
	} else {
		report.dockerProblems[label] = err
	}
}

func (report *cleanupReport) err() error {
	problems := append([]error{}, report.otherProblems...)
	for _, label := range []string{"containers", "networks", "volumes", "tool containers"} {
		if problem := report.dockerProblems[label]; problem != nil {
			problems = append(problems, problem)
		}
	}
	return errors.Join(problems...)
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
	run.timedCommandWithLimit(ctx, "complete pull-request gate", pullRequestGateLimit, "make", "check")
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
		"SEMLIA_DOCKER_RESOURCE_LABEL="+acceptanceDockerLabelKey+"="+project,
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

func TestAcceptanceEnvironmentPinsTaskDockerResourceLabel(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	environment := isolatedEnvironmentFrom(
		[]string{"PATH=/usr/bin", "SEMLIA_DOCKER_RESOURCE_LABEL=attacker.label=unrelated"},
		t.TempDir(),
		project,
		38080,
		35432,
	)
	values := environmentValues(environment, "SEMLIA_DOCKER_RESOURCE_LABEL")
	want := "io.semlia.acceptance.run=" + project
	if len(values) != 1 || values[0] != want {
		t.Fatalf("SEMLIA_DOCKER_RESOURCE_LABEL values = %v, want exactly %q", values, want)
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
	if phase := os.Getenv("SEMLIA_ACCEPTANCE_SLEEP_PHASE"); phase != "" {
		fmt.Printf("==> %s\n", phase)
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

func TestTimedCommandWithLimitReportsLastMakeCheckPhase(t *testing.T) {
	run := &freshCloneRun{
		t:    t,
		root: t.TempDir(),
		environment: append(
			filteredEnvironment(os.Environ(), "SEMLIA_ACCEPTANCE_SLEEP_HELPER", "SEMLIA_ACCEPTANCE_SLEEP_PHASE"),
			"SEMLIA_ACCEPTANCE_SLEEP_HELPER=1",
			"SEMLIA_ACCEPTANCE_SLEEP_PHASE=dependency and secret scan",
		),
	}
	_, err := run.executeTimedCommandWithLimit(
		context.Background(),
		"complete pull-request gate",
		75*time.Millisecond,
		os.Args[0],
		"-test.run=^TestAcceptanceSleepingCommandHelper$",
		"-test.count=1",
	)
	if err == nil {
		t.Fatal("hung pull-request gate was not cancelled")
	}
	if !strings.Contains(err.Error(), "last reported phase: dependency and secret scan") {
		t.Fatalf("pull-request timeout did not identify its last phase: %v", err)
	}
}

func TestFreshCloneLauncherReservesCleanupEnvelope(t *testing.T) {
	launcher := readRepositoryFile(t, "scripts/acceptance/m0-fresh-clone.sh")
	if strings.Count(launcher, "-timeout=100m") != 1 {
		t.Fatalf("canonical launcher must set exactly one 100m outer timeout:\n%s", launcher)
	}
	if !strings.Contains(readRepositoryFile(t, "tests/acceptance/fresh_clone_test.go"), `"test.timeout": outerAcceptanceTimeout.String()`) {
		t.Fatal("fresh-clone invocation validator does not enforce the canonical outer timeout")
	}
	resourceKinds := (&freshCloneRun{
		project:     "semlia-accept-a1b2c3d4-000000000001",
		environment: []string{"SEMLIA_DOCKER_RESOURCE_LABEL=" + acceptanceDockerLabelKey + "=semlia-accept-a1b2c3d4-000000000001"},
	}).cleanupResourceKinds()
	cleanupEnvelope := dockerTeardownTimeout +
		2*time.Duration(len(resourceKinds))*dockerInspectionTimeout + // Initial inspection and final exact relist.
		goModCacheCleanupTimeout + checkoutCleanupTimeout +
		time.Duration(len(resourceKinds))*(dockerInspectionTimeout+dockerTeardownTimeout+dockerInspectionTimeout) // Pre-cache list, removal, and failed-removal relist.
	maximumCleanupSubprocessCount := 3 + 5*len(resourceKinds) // Compose down, two cache cleanups, and five per-kind list/remove phases.
	for _, resource := range resourceKinds {
		cleanupEnvelope += time.Duration(resource.maxIdentities) * (dockerInspectionTimeout + dockerInspectionTimeout)
		maximumCleanupSubprocessCount += 2 * resource.maxIdentities // Identity inspection plus a conservative disappearance relist.
	}
	const developmentDiagnosticsEnvelope = 2 * developmentDiagnosticsTimeout
	const developmentDiagnosticsCommandCount = 2
	const journeyCancellationCommandCount = 1
	processWaitEnvelope := time.Duration(maximumCleanupSubprocessCount+developmentDiagnosticsCommandCount+journeyCancellationCommandCount) * acceptanceCommandWaitDelay
	const shutdownMargin = 5 * time.Minute
	minimumOuterTimeout := freshCloneTimeout + developmentDiagnosticsEnvelope + cleanupEnvelope + processWaitEnvelope + shutdownMargin
	if outerAcceptanceTimeout <= minimumOuterTimeout {
		t.Fatalf("outer timeout %s cannot contain journey %s + diagnostics %s + cleanup %s + process wait %s + shutdown margin %s", outerAcceptanceTimeout, freshCloneTimeout, developmentDiagnosticsEnvelope, cleanupEnvelope, processWaitEnvelope, shutdownMargin)
	}
}

func TestFreshCloneInvocationRejectsNoncanonicalOuterTimeout(t *testing.T) {
	command := exec.Command(
		os.Args[0],
		"-test.run=^TestFreshCloneAcceptance$",
		"-test.count=1",
		"-test.timeout=99m",
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
	want := `fresh-clone acceptance requires -timeout="1h40m0s"`
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
			switch {
			case strings.HasSuffix(name, "compose.sh"):
				return nil, context.DeadlineExceeded
			case name == "go":
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
	if len(calls) != 12 {
		t.Fatalf("cleanup calls = %d, want 12: %#v", len(calls), calls)
	}
	if calls[7].name != "go" || calls[7].arguments != "clean -modcache" {
		t.Fatalf("Go cache cleanup did not run after Docker timeout: %#v", calls)
	}
	if calls[8].name != "make" || calls[8].arguments != "clean" {
		t.Fatalf("checkout cleanup did not run after Go cache failure: %#v", calls)
	}
	expected := []time.Duration{
		dockerTeardownTimeout,
		dockerInspectionTimeout,
		dockerInspectionTimeout,
		dockerInspectionTimeout,
		dockerInspectionTimeout,
		dockerInspectionTimeout,
		dockerInspectionTimeout,
		goModCacheCleanupTimeout,
		checkoutCleanupTimeout,
		dockerInspectionTimeout,
		dockerInspectionTimeout,
		dockerInspectionTimeout,
	}
	for index, current := range calls {
		if current.remaining <= expected[index]-time.Second || current.remaining > expected[index] {
			t.Errorf("cleanup call %d budget = %s, want fresh budget near %s", index+1, current.remaining, expected[index])
		}
	}
}

func TestCleanupRunsAfterJourneyContextExpires(t *testing.T) {
	var calls []string
	run := &freshCloneRun{
		root:    t.TempDir(),
		project: "semlia-accept-a1b2c3d4-000000000001",
		cleanupExecutor: func(ctx context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			if err := ctx.Err(); err != nil {
				t.Fatalf("cleanup inherited canceled parent context: %v", err)
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("cleanup command has no independent deadline")
			}
			calls = append(calls, name+" "+strings.Join(args, " "))
			return nil, nil
		},
	}
	parentContext, cancelParent := context.WithCancel(context.Background())
	deferredCleanup := func() error { return run.cleanup() }
	cancelParent()
	if !errors.Is(parentContext.Err(), context.Canceled) {
		t.Fatal("test did not expire its parent acceptance context")
	}
	if err := deferredCleanup(); err != nil {
		t.Fatalf("cleanup after journey cancellation: %v", err)
	}
	logText := strings.Join(calls, "\n")
	for _, command := range []string{
		"compose.sh down --volumes --remove-orphans",
		"docker ps --all --filter label=com.docker.compose.project=semlia-accept-a1b2c3d4-000000000001",
		"docker network ls --filter label=com.docker.compose.project=semlia-accept-a1b2c3d4-000000000001",
		"docker volume ls --filter label=com.docker.compose.project=semlia-accept-a1b2c3d4-000000000001",
		"go clean -modcache",
		"make clean",
	} {
		if !strings.Contains(logText, command) {
			t.Errorf("cleanup did not run %q after journey cancellation:\n%s", command, logText)
		}
	}
}

func TestCleanupRemovesDockerResourcesBeforeCachesAndFinallyRelists(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	composeLabel := "com.docker.compose.project=" + project
	toolLabel := acceptanceDockerLabelKey + "=" + project
	containers := map[string]string{
		"abcdef123456": composeLabel,
		"123456abcdef": toolLabel,
	}
	networkExists := true
	volumeExists := true
	var calls []string
	run := &freshCloneRun{
		root:        t.TempDir(),
		project:     project,
		environment: []string{"SEMLIA_DOCKER_RESOURCE_LABEL=" + toolLabel},
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			joined := name + " " + strings.Join(args, " ")
			calls = append(calls, joined)
			if name != "docker" || len(args) == 0 {
				return nil, nil
			}
			switch args[0] {
			case "ps":
				filter := strings.TrimPrefix(argumentAfter(args, "--filter"), "label=")
				var identifiers []string
				for identifier, label := range containers {
					if label == filter {
						identifiers = append(identifiers, identifier)
					}
				}
				sort.Strings(identifiers)
				return []byte(strings.Join(identifiers, "\n")), nil
			case "inspect":
				identifier := args[len(args)-1]
				key, value, _ := strings.Cut(containers[identifier], "=")
				return json.Marshal(map[string]string{key: value})
			case "rm":
				for _, identifier := range args[2:] {
					delete(containers, identifier)
				}
			case "network":
				if len(args) > 1 && args[1] == "ls" && networkExists {
					return []byte("task-network\n"), nil
				}
				if len(args) > 2 && args[1] == "rm" && args[2] == "task-network" {
					networkExists = false
				}
			case "volume":
				if len(args) > 1 && args[1] == "ls" && volumeExists {
					return []byte("task-volume\n"), nil
				}
				if len(args) > 3 && args[1] == "rm" && args[3] == "task-volume" {
					volumeExists = false
				}
			}
			return nil, nil
		},
	}

	if err := run.cleanup(); err != nil {
		t.Fatal(err)
	}
	goIndex := commandIndex(calls, "go clean -modcache")
	makeIndex := commandIndex(calls, "make clean")
	for _, removal := range []string{
		"docker rm --force abcdef123456",
		"docker network rm task-network",
		"docker volume rm --force task-volume",
		"docker rm --force 123456abcdef",
	} {
		removeIndex := commandIndex(calls, removal)
		if removeIndex < 0 || removeIndex >= goIndex || removeIndex >= makeIndex {
			t.Fatalf("task-owned Docker removal %q did not precede cache cleanup:\n%s", removal, strings.Join(calls, "\n"))
		}
	}
	for _, relist := range []string{
		"docker ps --all --filter label=" + composeLabel,
		"docker network ls --filter label=" + composeLabel,
		"docker volume ls --filter label=" + composeLabel,
		"docker ps --all --filter label=" + toolLabel,
	} {
		if !commandAfter(calls, makeIndex, relist) {
			t.Fatalf("final exact relist %q did not run after checkout cleanup:\n%s", relist, strings.Join(calls, "\n"))
		}
	}
}

func TestCleanupPreservesEarlyFailuresAfterFinalDockerVerification(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	composeLabel := "com.docker.compose.project=" + project
	listCalls := map[string]int{}
	run := &freshCloneRun{
		root:    t.TempDir(),
		project: project,
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			joined := strings.Join(args, " ")
			switch {
			case strings.HasSuffix(name, "compose.sh"):
				return []byte("compose unavailable"), errors.New("compose failed")
			case name != "docker":
				return nil, nil
			case strings.HasPrefix(joined, "ps --all"):
				filter := argumentAfter(args, "--filter")
				key := "containers|" + filter
				listCalls[key]++
				if filter == "label="+composeLabel && listCalls[key] == 1 {
					return []byte("daemon unavailable"), errors.New("initial list failed")
				}
				return nil, nil
			case strings.HasPrefix(joined, "network ls"):
				filter := argumentAfter(args, "--filter")
				key := "networks|" + filter
				listCalls[key]++
				if listCalls[key] <= 3 {
					return []byte("task-network\n"), nil
				}
				return nil, nil
			case strings.HasPrefix(joined, "network rm task-network"):
				return []byte("network busy"), errors.New("remove failed")
			default:
				return nil, nil
			}
		},
	}

	err := run.cleanup()
	if err == nil {
		t.Fatal("early cleanup failures were erased by final successful relists")
	}
	for _, fragment := range []string{
		"compose down: compose failed",
		"initial inspect task-owned containers: initial list failed",
		"remove task-owned networks task-network: remove failed",
	} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("cleanup lost early failure %q: %v", fragment, err)
		}
	}
}

func commandIndex(commands []string, fragment string) int {
	for index, command := range commands {
		if strings.Contains(command, fragment) {
			return index
		}
	}
	return -1
}

func commandAfter(commands []string, index int, fragment string) bool {
	for current := index + 1; current < len(commands); current++ {
		if strings.Contains(commands[current], fragment) {
			return true
		}
	}
	return false
}

func TestCleanupFallbackRemovesOnlyProjectLabeledResources(t *testing.T) {
	type call struct {
		name string
		args []string
	}
	var calls []call
	listed := map[string]bool{"containers": false, "networks": false, "volumes": false}
	run := &freshCloneRun{
		root:    t.TempDir(),
		project: "semlia-accept-a1b2c3d4-000000000001",
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			calls = append(calls, call{name: name, args: append([]string{}, args...)})
			joined := strings.Join(args, " ")
			switch {
			case name == "docker" && strings.HasPrefix(joined, "inspect --format"):
				return []byte(`{"com.docker.compose.project":"semlia-accept-a1b2c3d4-000000000001"}` + "\n"), nil
			case name == "docker" && strings.HasPrefix(joined, "ps --all"):
				if !listed["containers"] {
					listed["containers"] = true
					return []byte("abcdef123456\n"), nil
				}
			case name == "docker" && strings.HasPrefix(joined, "network ls"):
				if !listed["networks"] {
					listed["networks"] = true
					return []byte("task-network\n"), nil
				}
			case name == "docker" && strings.HasPrefix(joined, "volume ls"):
				if !listed["volumes"] {
					listed["volumes"] = true
					return []byte("task-volume\n"), nil
				}
			case name == "docker" && (strings.HasPrefix(joined, "rm ") || strings.HasPrefix(joined, "network rm ") || strings.HasPrefix(joined, "volume rm ")):
				if strings.Contains(joined, "unrelated-") {
					t.Fatalf("cleanup targeted unrelated resource: %s", joined)
				}
			}
			return nil, nil
		},
	}
	if err := run.removeRemainingTaskDockerResources(); err != nil {
		t.Fatal(err)
	}
	allCalls := make([]string, 0, len(calls))
	for _, current := range calls {
		joined := current.name + " " + strings.Join(current.args, " ")
		allCalls = append(allCalls, joined)
		if strings.Contains(joined, "unrelated") {
			t.Fatalf("cleanup targeted an unrelated resource: %s", joined)
		}
	}
	commands := strings.Join(allCalls, "\n")
	for _, expected := range []string{
		"docker ps --all --filter label=com.docker.compose.project=semlia-accept-a1b2c3d4-000000000001 --format {{.ID}}",
		"docker rm --force abcdef123456",
		"docker network rm task-network",
		"docker volume rm --force task-volume",
	} {
		if !strings.Contains(commands, expected) {
			t.Errorf("cleanup fallback missing %q:\n%s", expected, commands)
		}
	}
}

func TestCleanupPreservesInitialDockerTimeoutAfterFallbackRecovers(t *testing.T) {
	var listCalls int
	volumeRemoved := false
	run := &freshCloneRun{
		root:    t.TempDir(),
		project: "semlia-accept-a1b2c3d4-000000000001",
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			joined := strings.Join(args, " ")
			switch {
			case strings.HasSuffix(name, "compose.sh"):
				return nil, context.DeadlineExceeded
			case name != "docker":
				return nil, nil
			case strings.HasPrefix(joined, "ps --all") || strings.HasPrefix(joined, "network ls") || strings.HasPrefix(joined, "volume ls"):
				listCalls++
				if listCalls <= 3 {
					return nil, context.DeadlineExceeded
				}
				if strings.HasPrefix(joined, "volume ls") && !volumeRemoved {
					return []byte("task-volume\n"), nil
				}
				return nil, nil
			case strings.HasPrefix(joined, "volume rm --force task-volume"):
				volumeRemoved = true
				return nil, nil
			default:
				return nil, nil
			}
		},
	}

	err := run.cleanup()
	if err == nil {
		t.Fatal("compose timeout was incorrectly erased after exact fallback")
	}
	if !strings.Contains(err.Error(), "compose down: context deadline exceeded") {
		t.Fatalf("cleanup lost the compose timeout: %v", err)
	}
	for _, retained := range []string{"initial inspect task-owned containers", "initial inspect task-owned networks", "initial inspect task-owned volumes"} {
		if !strings.Contains(err.Error(), retained) {
			t.Errorf("cleanup lost initial Docker error %q after fallback recovered: %v", retained, err)
		}
	}
	if strings.Contains(err.Error(), "task-owned volumes remain") {
		t.Fatalf("cleanup reported a final resource leak after fallback removed the volume: %v", err)
	}
}

func TestCleanupFallbackRejectsInvalidDockerIdentifiers(t *testing.T) {
	run := &freshCloneRun{
		root:    t.TempDir(),
		project: "semlia-accept-a1b2c3d4-000000000001",
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" {
				return []byte("not-a-container-id\n"), nil
			}
			if name == "docker" && len(args) > 0 && args[0] == "rm" {
				t.Fatalf("cleanup attempted removal using an invalid identifier: %v", args)
			}
			return nil, nil
		},
	}
	err := run.removeRemainingTaskDockerResources()
	if err == nil || !strings.Contains(err.Error(), `invalid identifier "not-a-container-id"`) {
		t.Fatalf("invalid Docker identifier was not rejected: %v", err)
	}
}

func TestCleanupComposeContainerLimitMatchesCanonicalTopology(t *testing.T) {
	compose := readRepositoryFile(t, "compose.yaml")
	var topology struct {
		Services map[string]any `yaml:"services"`
		Networks map[string]any `yaml:"networks"`
		Volumes  map[string]any `yaml:"volumes"`
	}
	if err := yaml.Unmarshal([]byte(compose), &topology); err != nil {
		t.Fatalf("parse canonical Compose topology: %v", err)
	}
	canonicalServices := []string{"postgres", "migrate", "server", "worker"}
	if len(topology.Services) != composeServiceContainerLimit {
		t.Fatalf("canonical Compose services = %d, cleanup container limit = %d", len(topology.Services), composeServiceContainerLimit)
	}
	for _, service := range canonicalServices {
		if _, ok := topology.Services[service]; !ok {
			t.Fatalf("canonical Compose topology is missing service %q", service)
		}
	}
	if len(topology.Networks) != composeNetworkLimit || len(topology.Volumes) != composeVolumeLimit {
		t.Fatalf("canonical Compose networks/volumes = %d/%d, cleanup limits = %d/%d", len(topology.Networks), len(topology.Volumes), composeNetworkLimit, composeVolumeLimit)
	}

	project := "semlia-accept-a1b2c3d4-000000000001"
	composeLabel := "com.docker.compose.project=" + project
	containers := map[string]bool{
		"000000000001": true,
		"000000000002": true,
		"000000000003": true,
		"000000000004": true,
	}
	run := &freshCloneRun{
		root:    t.TempDir(),
		project: project,
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			if name != "docker" || len(args) == 0 {
				return nil, nil
			}
			switch args[0] {
			case "ps":
				var identifiers []string
				for identifier := range containers {
					identifiers = append(identifiers, identifier)
				}
				sort.Strings(identifiers)
				return []byte(strings.Join(identifiers, "\n")), nil
			case "inspect":
				return json.Marshal(map[string]string{"com.docker.compose.project": project})
			case "rm":
				for _, identifier := range args[2:] {
					delete(containers, identifier)
				}
			}
			return nil, nil
		},
	}
	if err := run.removeRemainingTaskDockerResources(); err != nil {
		t.Fatalf("cleanup rejected the four canonical Compose service containers: %v", err)
	}
	if len(containers) != 0 {
		t.Fatalf("canonical Compose containers remain after cleanup: %v", containers)
	}

	fiveContainers := "000000000001\n000000000002\n000000000003\n000000000004\n000000000005\n"
	run.cleanupExecutor = func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
		if name == "docker" && len(args) > 0 && args[0] == "ps" && argumentAfter(args, "--filter") == "label="+composeLabel {
			return []byte(fiveContainers), nil
		}
		if name == "docker" && len(args) > 0 && args[0] == "rm" {
			t.Fatalf("cleanup attempted removal after the Compose service maximum was exceeded: %v", args)
		}
		return nil, nil
	}
	err := run.removeRemainingTaskDockerResources()
	if err == nil || !strings.Contains(err.Error(), "refuse to exceed task maximum 4") {
		t.Fatalf("more containers than the canonical Compose topology were not rejected: %v", err)
	}
}

func TestCleanupToolContainerLimitIsIndependentFromComposeTopology(t *testing.T) {
	securityScript := readRepositoryFile(t, "scripts/ci/security-check.sh")
	sbomScript := readRepositoryFile(t, "scripts/release/sbom.sh")
	toolInvocations := strings.Count(securityScript, "docker_run \\\n") + strings.Count(sbomScript, "docker_run \\\n")
	if toolInvocations != toolContainerLimit {
		t.Fatalf("canonical security/SBOM Docker invocations = %d, cleanup tool container limit = %d", toolInvocations, toolContainerLimit)
	}

	project := "semlia-accept-a1b2c3d4-000000000001"
	resourceLabel := acceptanceDockerLabelKey + "=" + project
	run := &freshCloneRun{
		root:        t.TempDir(),
		project:     project,
		environment: []string{"SEMLIA_DOCKER_RESOURCE_LABEL=" + resourceLabel},
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) > 0 && args[0] == "ps" && argumentAfter(args, "--filter") == "label="+resourceLabel {
				return []byte("000000000001\n000000000002\n000000000003\n000000000004\n"), nil
			}
			if name == "docker" && len(args) > 0 && args[0] == "rm" {
				t.Fatalf("cleanup attempted tool removal after the independent maximum was exceeded: %v", args)
			}
			return nil, nil
		},
	}
	err := run.removeRemainingTaskDockerResources()
	if err == nil || !strings.Contains(err.Error(), "task-owned tool containers returned 4 resources; refuse to exceed task maximum 3") {
		t.Fatalf("tool container maximum followed Compose topology instead of its own contract: %v", err)
	}
}

func TestCleanupOwnsLabeledDaemonContainers(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	resourceLabel := "io.semlia.acceptance.run=" + project
	containers := map[string]map[string]string{
		"abcdef123456": {"io.semlia.acceptance.run": project},
		"123456abcdef": {"io.semlia.acceptance.run": "unrelated-run"},
	}
	var inspected, removed []string
	run := &freshCloneRun{
		root:        t.TempDir(),
		project:     project,
		environment: []string{"SEMLIA_DOCKER_RESOURCE_LABEL=" + resourceLabel},
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			if name != "docker" || len(args) == 0 {
				return nil, nil
			}
			switch args[0] {
			case "ps":
				filter := argumentAfter(args, "--filter")
				label, found := strings.CutPrefix(filter, "label=")
				if !found {
					return nil, nil
				}
				key, value, found := strings.Cut(label, "=")
				if !found {
					return nil, nil
				}
				var matches []string
				for identifier, labels := range containers {
					if labels[key] == value {
						matches = append(matches, identifier)
					}
				}
				sort.Strings(matches)
				return []byte(strings.Join(matches, "\n")), nil
			case "inspect":
				identifier := args[len(args)-1]
				inspected = append(inspected, identifier)
				return json.Marshal(containers[identifier])
			case "rm":
				for _, identifier := range args[2:] {
					removed = append(removed, identifier)
					delete(containers, identifier)
				}
				return nil, nil
			default:
				return nil, nil
			}
		},
	}

	if err := run.cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, ok := containers["abcdef123456"]; ok {
		t.Fatal("task-owned scanner container survived cleanup")
	}
	if _, ok := containers["123456abcdef"]; !ok {
		t.Fatal("cleanup removed an unrelated scanner container")
	}
	if strings.Join(inspected, ",") != "abcdef123456" {
		t.Fatalf("inspected containers = %v, want only task-owned container", inspected)
	}
	if strings.Join(removed, ",") != "abcdef123456" {
		t.Fatalf("removed containers = %v, want only task-owned container", removed)
	}
}

func TestCleanupTreatsAutoRemovedLabeledContainerAsResolved(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	resourceLabel := acceptanceDockerLabelKey + "=" + project
	containerExists := true
	run := &freshCloneRun{
		root:        t.TempDir(),
		project:     project,
		environment: []string{"SEMLIA_DOCKER_RESOURCE_LABEL=" + resourceLabel},
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			if name != "docker" || len(args) == 0 {
				return nil, nil
			}
			switch args[0] {
			case "ps":
				if argumentAfter(args, "--filter") == "label="+resourceLabel && containerExists {
					return []byte("abcdef123456\n"), nil
				}
			case "inspect":
				containerExists = false
				return nil, errors.New("No such object: abcdef123456")
			case "rm":
				t.Fatal("cleanup attempted to remove a container that Docker had already auto-removed")
			}
			return nil, nil
		},
	}

	if err := run.cleanup(); err != nil {
		t.Fatalf("auto-removed task-owned container was not treated as resolved: %v", err)
	}
}

func TestCleanupTreatsFailedRemoveAsResolvedAfterExactRelist(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	composeLabel := "com.docker.compose.project=" + project
	containerExists := true
	listCalls := 0
	run := &freshCloneRun{
		root:    t.TempDir(),
		project: project,
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			if name != "docker" || len(args) == 0 {
				return nil, nil
			}
			switch args[0] {
			case "ps":
				if argumentAfter(args, "--filter") == "label="+composeLabel {
					listCalls++
					if containerExists {
						return []byte("abcdef123456\n"), nil
					}
				}
			case "inspect":
				return json.Marshal(map[string]string{"com.docker.compose.project": project})
			case "rm":
				containerExists = false
				return []byte("No such container"), errors.New("remove raced with --rm")
			}
			return nil, nil
		},
	}

	if err := run.removeRemainingTaskDockerResources(); err != nil {
		t.Fatalf("failed remove was not resolved after exact label relist confirmed absence: %v", err)
	}
	if listCalls != 2 {
		t.Fatalf("exact Compose label list calls = %d, want initial list plus failed-remove relist", listCalls)
	}
}

func TestCleanupPreservesFailedRemoveWhenExactRelistStillFindsResource(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	composeLabel := "com.docker.compose.project=" + project
	listCalls := 0
	run := &freshCloneRun{
		root:    t.TempDir(),
		project: project,
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			if name != "docker" || len(args) == 0 {
				return nil, nil
			}
			switch args[0] {
			case "ps":
				if argumentAfter(args, "--filter") == "label="+composeLabel {
					listCalls++
					return []byte("abcdef123456\n"), nil
				}
			case "inspect":
				return json.Marshal(map[string]string{"com.docker.compose.project": project})
			case "rm":
				return []byte("container busy"), errors.New("remove failed")
			}
			return nil, nil
		},
	}

	err := run.removeRemainingTaskDockerResources()
	if err == nil || !strings.Contains(err.Error(), "remove task-owned containers abcdef123456: remove failed") {
		t.Fatalf("failed removal of a still-listed resource was erased: %v", err)
	}
	if listCalls != 2 {
		t.Fatalf("exact Compose label list calls = %d, want initial list plus failed-remove relist", listCalls)
	}
}

func TestCleanupPreservesFailedRemoveWhenExactRelistFails(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	composeLabel := "com.docker.compose.project=" + project
	listCalls := 0
	run := &freshCloneRun{
		root:    t.TempDir(),
		project: project,
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			if name != "docker" || len(args) == 0 {
				return nil, nil
			}
			switch args[0] {
			case "ps":
				if argumentAfter(args, "--filter") == "label="+composeLabel {
					listCalls++
					if listCalls == 1 {
						return []byte("abcdef123456\n"), nil
					}
					return []byte("daemon unavailable"), errors.New("relist failed")
				}
			case "inspect":
				return json.Marshal(map[string]string{"com.docker.compose.project": project})
			case "rm":
				return []byte("container busy"), errors.New("remove failed")
			}
			return nil, nil
		},
	}

	err := run.removeRemainingTaskDockerResources()
	if err == nil || !strings.Contains(err.Error(), "remove task-owned containers abcdef123456: remove failed") || !strings.Contains(err.Error(), "confirm task-owned containers after failed removal: relist failed") {
		t.Fatalf("failed removal or its failed exact relist was erased: %v", err)
	}
}

func TestCleanupPreservesInspectFailureWhenContainerStillListed(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	resourceLabel := acceptanceDockerLabelKey + "=" + project
	removeCalled := false
	run := &freshCloneRun{
		root:        t.TempDir(),
		project:     project,
		environment: []string{"SEMLIA_DOCKER_RESOURCE_LABEL=" + resourceLabel},
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			if name != "docker" || len(args) == 0 {
				return nil, nil
			}
			switch args[0] {
			case "ps":
				if argumentAfter(args, "--filter") == "label="+resourceLabel {
					return []byte("abcdef123456\n"), nil
				}
			case "inspect":
				return []byte("daemon unavailable"), errors.New("inspect failed")
			case "rm":
				removeCalled = true
			}
			return nil, nil
		},
	}

	err := run.cleanup()
	if err == nil || !strings.Contains(err.Error(), "inspect identity for task-owned tool containers abcdef123456") {
		t.Fatalf("live-container inspect failure was erased: %v", err)
	}
	if removeCalled {
		t.Fatal("cleanup removed a container whose task identity could not be verified")
	}
}

func TestDockerCleanupLifecycleProbe(t *testing.T) {
	if os.Getenv("SEMLIA_RUN_DOCKER_CLEANUP_PROBE") != "1" {
		t.Skip("set SEMLIA_RUN_DOCKER_CLEANUP_PROBE=1 to exercise daemon-side container cleanup")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	image := os.Getenv("SEMLIA_DOCKER_CLEANUP_PROBE_IMAGE")
	if image == "" {
		image = "alpine:3.22"
	}
	if _, err := dockerProbeOutput(ctx, "image", "inspect", image); err != nil {
		if output, pullErr := dockerProbeOutput(ctx, "pull", image); pullErr != nil {
			t.Fatalf("prepare Docker cleanup probe image %q: %v\n%s", image, pullErr, output)
		}
	}

	project := "semlia-cleanup-probe-" + randomRunID(t)
	taskLabel := acceptanceDockerLabelKey + "=" + project
	unrelatedLabel := acceptanceDockerLabelKey + "=" + project + "-unrelated"
	t.Cleanup(func() {
		bestEffortRemoveDockerProbeContainers(t, taskLabel, unrelatedLabel)
	})

	unrelatedID, err := dockerProbeOutput(ctx,
		"run", "--detach", "--rm", "--label", unrelatedLabel,
		image, "sh", "-c", "sleep 300",
	)
	if err != nil {
		t.Fatalf("start unrelated Docker cleanup probe container: %v\n%s", err, unrelatedID)
	}
	unrelatedID = strings.TrimSpace(unrelatedID)

	foreground := exec.CommandContext(ctx,
		"docker", "run", "--rm", "--label", taskLabel,
		image, "sh", "-c", "sleep 300",
	)
	var foregroundOutput strings.Builder
	foreground.Stdout = &foregroundOutput
	foreground.Stderr = &foregroundOutput
	if err := foreground.Start(); err != nil {
		t.Fatalf("start attached Docker cleanup probe CLI: %v", err)
	}
	foregroundWaited := false
	t.Cleanup(func() {
		if !foregroundWaited && foreground.Process != nil {
			_ = foreground.Process.Kill()
			_ = foreground.Wait()
			foregroundWaited = true
		}
	})

	startContext, cancelStart := context.WithTimeout(ctx, 20*time.Second)
	taskID, err := waitForDockerProbeContainer(startContext, taskLabel)
	cancelStart()
	if err != nil {
		_ = foreground.Process.Kill()
		_ = foreground.Wait()
		foregroundWaited = true
		t.Fatalf("wait for task-labeled Docker cleanup probe container: %v\n%s", err, foregroundOutput.String())
	}
	if err := foreground.Process.Kill(); err != nil {
		t.Fatalf("kill attached Docker CLI: %v", err)
	}
	_ = foreground.Wait()
	foregroundWaited = true

	survivalContext, cancelSurvival := context.WithTimeout(ctx, 5*time.Second)
	survivingID, err := waitForDockerProbeContainer(survivalContext, taskLabel)
	cancelSurvival()
	if err != nil {
		t.Fatalf("daemon-side container did not survive its killed CLI: %v", err)
	}
	if survivingID != taskID {
		t.Fatalf("daemon-side task container changed after CLI death: got %q, want %q", survivingID, taskID)
	}

	run := &freshCloneRun{
		root:        repositoryRoot,
		project:     project,
		environment: append(filteredEnvironment(os.Environ(), "SEMLIA_DOCKER_RESOURCE_LABEL"), "SEMLIA_DOCKER_RESOURCE_LABEL="+taskLabel),
	}
	if err := run.removeRemainingTaskDockerResources(); err != nil {
		t.Fatalf("remove task-labeled daemon container: %v", err)
	}
	if output, err := dockerProbeOutput(ctx, "ps", "--all", "--filter", "label="+taskLabel, "--format", "{{.ID}}"); err != nil || strings.TrimSpace(output) != "" {
		t.Fatalf("task-labeled daemon container remains after cleanup: err=%v output=%q", err, output)
	}
	if output, err := dockerProbeOutput(ctx, "inspect", "--format", "{{.Id}}", unrelatedID); err != nil || strings.TrimSpace(output) == "" {
		t.Fatalf("cleanup removed unrelated labeled container %q: err=%v output=%q", unrelatedID, err, output)
	}
}

func waitForDockerProbeContainer(ctx context.Context, label string) (string, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		output, err := dockerProbeOutput(ctx, "ps", "--all", "--filter", "label="+label, "--format", "{{.ID}}")
		if err != nil {
			return "", err
		}
		identifiers := strings.Fields(output)
		if len(identifiers) == 1 {
			return identifiers[0], nil
		}
		if len(identifiers) > 1 {
			return "", fmt.Errorf("label %q matched %d containers", label, len(identifiers))
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func dockerProbeOutput(ctx context.Context, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "docker", args...)
	output, err := command.CombinedOutput()
	return string(output), err
}

func bestEffortRemoveDockerProbeContainers(t *testing.T, labels ...string) {
	t.Helper()
	for _, label := range labels {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		output, err := dockerProbeOutput(ctx, "ps", "--all", "--filter", "label="+label, "--format", "{{.ID}}")
		if err != nil {
			cancel()
			t.Logf("list Docker cleanup probe containers for %q: %v", label, err)
			continue
		}
		identifiers := strings.Fields(output)
		if len(identifiers) == 0 {
			cancel()
			continue
		}
		if removeOutput, removeErr := dockerProbeOutput(ctx, append([]string{"rm", "--force"}, identifiers...)...); removeErr != nil {
			t.Logf("remove Docker cleanup probe containers for %q: %v: %s", label, removeErr, strings.TrimSpace(removeOutput))
		}
		cancel()
	}
}

func argumentAfter(arguments []string, name string) string {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == name {
			return arguments[index+1]
		}
	}
	return ""
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
	output, commandErr := run.executeCommandOutput(limitedContext, name, args...)
	duration := time.Since(started)

	if errors.Is(context.Cause(limitedContext), errCommandDurationLimit) {
		thresholdErr := fmt.Errorf("%s exceeded %s after %s; command cancelled: %w", label, limit, duration, context.DeadlineExceeded)
		if phase := lastReportedPhase(output); phase != "" {
			thresholdErr = fmt.Errorf("%w; last reported phase: %s", thresholdErr, phase)
		}
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
	output, err := run.executeCommandOutput(ctx, name, args...)
	return string(output), err
}

func (run *freshCloneRun) executeCommandOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := acceptanceCommandContext(ctx, name, args...)
	command.Dir = run.root
	command.Env = run.environment
	output, err := command.CombinedOutput()
	if err == nil {
		return output, nil
	}

	details := strings.TrimSpace(run.redactTaskSecrets(string(output)))
	if details != "" {
		details = "\n" + details
	}
	if name == "make" && len(args) == 1 && args[0] == "dev" {
		details += "\n\nCompose diagnostics (redacted):\n" + run.developmentDiagnostics()
	}
	return output, fmt.Errorf("%s %s: %w%s", name, strings.Join(args, " "), err, details)
}

func lastReportedPhase(output []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(output), "\r", "\n"), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if phase, found := strings.CutPrefix(line, "==> "); found {
			return strings.TrimSpace(phase)
		}
	}
	return ""
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
	report := &cleanupReport{}
	if output, err := run.cleanupCommand(
		dockerTeardownTimeout,
		filepath.Join(run.root, "scripts", "dev", "compose.sh"),
		"down", "--volumes", "--remove-orphans",
	); err != nil {
		report.addOther(fmt.Errorf("compose down: %w: %s", err, strings.TrimSpace(run.redactTaskSecrets(string(output)))))
	}

	run.inspectTaskDockerResources(report)
	if err := run.removeRemainingTaskDockerResources(); err != nil {
		report.addOther(err)
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
			report.addOther(fmt.Errorf("clean %s: %w: %s", cleanup.label, err, strings.TrimSpace(run.redactTaskSecrets(string(output)))))
		}
	}
	run.verifyTaskDockerResourcesRemoved(report)
	return report.err()
}

func (run *freshCloneRun) inspectTaskDockerResources(report *cleanupReport) {
	for _, resource := range run.cleanupResourceKinds() {
		output, err := run.cleanupCommand(dockerInspectionTimeout, "docker", resource.listArgs...)
		if err != nil {
			report.addOther(fmt.Errorf("initial inspect task-owned %s: %w: %s", resource.label, err, strings.TrimSpace(run.redactTaskSecrets(string(output)))))
			continue
		}
		if _, identifierErr := validateDockerResourceIdentifiers(resource, output); identifierErr != nil {
			report.addOther(fmt.Errorf("initial %w", identifierErr))
		}
	}
}

func (run *freshCloneRun) verifyTaskDockerResourcesRemoved(report *cleanupReport) {
	for _, resource := range run.cleanupResourceKinds() {
		output, err := run.cleanupCommand(dockerInspectionTimeout, "docker", resource.listArgs...)
		if err != nil {
			report.setDockerProblem(resource.label, fmt.Errorf("final verify task-owned %s cleanup: %w: %s", resource.label, err, strings.TrimSpace(run.redactTaskSecrets(string(output)))))
			continue
		}
		identifiers, identifierErr := validateDockerResourceIdentifiers(resource, output)
		if identifierErr != nil {
			report.setDockerProblem(resource.label, fmt.Errorf("final %w", identifierErr))
			continue
		}
		if len(identifiers) != 0 {
			report.setDockerProblem(resource.label, fmt.Errorf("task-owned %s remain after final exact relist: %s", resource.label, strings.Join(identifiers, ",")))
			continue
		}
		report.setDockerProblem(resource.label, nil)
	}
}

func (run *freshCloneRun) cleanupResourceKinds() []cleanupResourceKind {
	composeLabel := "com.docker.compose.project=" + run.project
	resources := []cleanupResourceKind{
		{label: "containers", listArgs: []string{"ps", "--all", "--filter", "label=" + composeLabel, "--format", "{{.ID}}"}, removeArgs: []string{"rm", "--force"}, identityLabel: composeLabel, maxIdentities: composeServiceContainerLimit},
		{label: "networks", listArgs: []string{"network", "ls", "--filter", "label=" + composeLabel, "--format", "{{.ID}}"}, removeArgs: []string{"network", "rm"}, maxIdentities: composeNetworkLimit},
		{label: "volumes", listArgs: []string{"volume", "ls", "--filter", "label=" + composeLabel, "--format", "{{.Name}}"}, removeArgs: []string{"volume", "rm", "--force"}, maxIdentities: composeVolumeLimit},
	}
	if taskLabel := environmentValue(run.environment, "SEMLIA_DOCKER_RESOURCE_LABEL"); taskLabel != "" {
		resources = append(resources, cleanupResourceKind{
			label:         "tool containers",
			listArgs:      []string{"ps", "--all", "--filter", "label=" + taskLabel, "--format", "{{.ID}}"},
			removeArgs:    []string{"rm", "--force"},
			identityLabel: taskLabel,
			maxIdentities: toolContainerLimit,
		})
	}
	return resources
}

func validateDockerResourceIdentifiers(resource cleanupResourceKind, output []byte) ([]string, error) {
	identifiers := strings.Fields(string(output))
	if resource.maxIdentities > 0 && len(identifiers) > resource.maxIdentities {
		return nil, fmt.Errorf("inspect task-owned %s returned %d resources; refuse to exceed task maximum %d", resource.label, len(identifiers), resource.maxIdentities)
	}
	for _, identifier := range identifiers {
		valid := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`).MatchString(identifier)
		if resource.identityLabel != "" {
			valid = regexp.MustCompile(`^[a-f0-9]{12,64}$`).MatchString(identifier)
		}
		if !valid {
			return nil, fmt.Errorf("inspect task-owned %s returned invalid identifier %q", resource.label, identifier)
		}
	}
	return identifiers, nil
}

func (run *freshCloneRun) verifyDockerResourceIdentity(resource cleanupResourceKind, identifiers []string) ([]string, error) {
	if resource.identityLabel == "" || len(identifiers) == 0 {
		return identifiers, nil
	}
	key, value, found := strings.Cut(resource.identityLabel, "=")
	if !found || key == "" || value == "" {
		return nil, fmt.Errorf("task-owned %s has invalid identity label %q", resource.label, resource.identityLabel)
	}
	verified := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		output, err := run.cleanupCommand(dockerInspectionTimeout, "docker", "inspect", "--format", "{{json .Config.Labels}}", identifier)
		if err != nil {
			if run.dockerResourceNoLongerExists(resource, identifier) {
				continue
			}
			return nil, fmt.Errorf("inspect identity for task-owned %s %s: %w: %s", resource.label, identifier, err, strings.TrimSpace(run.redactTaskSecrets(string(output))))
		}
		labels := map[string]string{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(string(output))), &labels); err != nil {
			return nil, fmt.Errorf("inspect identity for task-owned %s %s: parse labels: %w", resource.label, identifier, err)
		}
		if labels[key] != value {
			return nil, fmt.Errorf("refuse to remove %s %s: label %s=%q, want %q", resource.label, identifier, key, labels[key], value)
		}
		verified = append(verified, identifier)
	}
	return verified, nil
}

func (run *freshCloneRun) dockerResourceNoLongerExists(resource cleanupResourceKind, identifier string) bool {
	output, err := run.cleanupCommand(dockerInspectionTimeout, "docker", resource.listArgs...)
	if err != nil {
		return false
	}
	identifiers, err := validateDockerResourceIdentifiers(resource, output)
	if err != nil {
		return false
	}
	for _, remaining := range identifiers {
		if remaining == identifier {
			return false
		}
	}
	return true
}

func (run *freshCloneRun) removeRemainingTaskDockerResources() error {
	var problems []error
	for _, resource := range run.cleanupResourceKinds() {
		output, err := run.cleanupCommand(dockerInspectionTimeout, "docker", resource.listArgs...)
		if err != nil {
			problems = append(problems, fmt.Errorf("reinspect task-owned %s: %w: %s", resource.label, err, strings.TrimSpace(run.redactTaskSecrets(string(output)))))
			continue
		}
		identifiers, identifierErr := validateDockerResourceIdentifiers(resource, output)
		if identifierErr != nil {
			problems = append(problems, identifierErr)
			continue
		}
		if len(identifiers) == 0 {
			continue
		}
		verifiedIdentifiers, identityErr := run.verifyDockerResourceIdentity(resource, identifiers)
		if identityErr != nil {
			problems = append(problems, identityErr)
			continue
		}
		if len(verifiedIdentifiers) == 0 {
			continue
		}
		removeArgs := append(append([]string{}, resource.removeArgs...), verifiedIdentifiers...)
		if removeOutput, removeErr := run.cleanupCommand(dockerTeardownTimeout, "docker", removeArgs...); removeErr != nil {
			removeProblem := fmt.Errorf("remove task-owned %s %s: %w: %s", resource.label, strings.Join(verifiedIdentifiers, ","), removeErr, strings.TrimSpace(run.redactTaskSecrets(string(removeOutput))))
			confirmOutput, confirmErr := run.cleanupCommand(dockerInspectionTimeout, "docker", resource.listArgs...)
			if confirmErr != nil {
				problems = append(problems, errors.Join(
					removeProblem,
					fmt.Errorf("confirm task-owned %s after failed removal: %w: %s", resource.label, confirmErr, strings.TrimSpace(run.redactTaskSecrets(string(confirmOutput)))),
				))
				continue
			}
			remaining, identifierErr := validateDockerResourceIdentifiers(resource, confirmOutput)
			if identifierErr != nil {
				problems = append(problems, errors.Join(removeProblem, fmt.Errorf("confirm failed removal: %w", identifierErr)))
				continue
			}
			if len(remaining) != 0 {
				problems = append(problems, removeProblem)
			}
			continue
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
