package acceptance

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
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
	dockerResourceRemovalTimeout  = 45 * time.Second
	dockerIdentityTimeout         = 15 * time.Second
	nodeSystemCAProbeTimeout      = 15 * time.Second
	acceptanceDockerLabelKey      = "io.semlia.acceptance.run"
	testcontainersSessionLabelKey = "org.testcontainers.sessionId"
	acceptanceHostLock            = "/tmp/semlia-t008-acceptance.lock"
	composeServiceContainerLimit  = 4 // compose.yaml defines postgres, migrate, server, and worker.
	composeNetworkLimit           = 1 // compose.yaml defines the backend network.
	composeVolumeLimit            = 1 // compose.yaml defines the postgres-data volume.
	toolContainerLimit            = 3 // Two security scans plus one release SBOM invocation.
	testcontainersContainerLimit  = 3 // Two integration PostgreSQL containers plus one shared Ryuk container.
	testcontainersNetworkLimit    = 1 // No current journey network; one failure-probe network is bounded and owned.
	testcontainersVolumeLimit     = 1 // No current journey volume; one failure-probe volume is bounded and owned.
	pinnedGoVersion               = "1.26.5"
)

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
	inspectFormat string
	maxIdentities int
}

type freshCloneRun struct {
	t               *testing.T
	scratchBase     string
	scratch         string
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
	labels := make([]string, 0, len(report.dockerProblems))
	for label := range report.dockerProblems {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	for _, label := range labels {
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

	journeyContext, stopSignals := acceptanceJourneySignalContext(context.Background())
	defer stopSignals()
	ctx, cancel := context.WithTimeout(journeyContext, freshCloneTimeout)
	defer cancel()
	onboardingStarted := time.Now()
	run := newFreshCloneRun(t, config)
	if err := run.validateGoToolchain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := run.validateNodeSystemCA(ctx); err != nil {
		t.Fatal(err)
	}
	run.clone(ctx, config)
	if err := run.prepareDockerCLI(ctx); err != nil {
		t.Fatal(err)
	}
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
	if config.source == "" || !filepath.IsAbs(config.source) {
		return fmt.Errorf("SEMLIA_ACCEPTANCE_SOURCE must be an absolute local repository directory")
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
	run := newFreshCloneRunWithID(t, config, randomRunID(t))
	registerTaskScratchCleanup(t, run.scratchBase, run.scratch)
	return run
}

func newFreshCloneRunWithID(t *testing.T, config freshCloneConfig, runID string) *freshCloneRun {
	t.Helper()
	scratch := t.TempDir()
	scratchBase := filepath.Dir(scratch)
	httpPort, postgresPort := reserveLocalPorts(t)
	project := acceptanceProjectName(config.ref, runID)
	environment := isolatedEnvironment(scratch, project, httpPort, postgresPort)
	if err := prepareIsolatedEnvironmentDirectories(environment); err != nil {
		t.Fatalf("prepare isolated acceptance environment: %v", err)
	}
	return &freshCloneRun{
		t:           t,
		scratchBase: scratchBase,
		scratch:     scratch,
		root:        filepath.Join(scratch, "checkout"),
		environment: environment,
		project:     project,
		baseURL:     fmt.Sprintf("http://127.0.0.1:%d", httpPort),
	}
}

func registerTaskScratchCleanup(t *testing.T, scratchBase, scratch string) {
	t.Helper()
	t.Cleanup(func() {
		if err := makeTaskScratchRemovable(scratchBase, scratch); err != nil {
			t.Errorf("make task scratch removable: %v", err)
		}
	})
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
	launcherRoot := environmentValue(base, "SEMLIA_ACCEPTANCE_LAUNCHER_ROOT")
	if launcherRoot == "" || !filepath.IsAbs(launcherRoot) || filepath.Clean(launcherRoot) != launcherRoot {
		launcherRoot = ""
	}
	environment := filterAcceptanceEnvironment(base)
	environment = append(environment,
		"PATH="+trustedAcceptancePathFrom(base),
		"HOME="+filepath.Join(scratch, "home"),
		"TMPDIR="+filepath.Join(scratch, "tmp"),
		"DOCKER_CONFIG="+filepath.Join(scratch, "docker-config"),
		"XDG_CONFIG_HOME="+filepath.Join(scratch, "config"),
		"XDG_CACHE_HOME="+filepath.Join(scratch, "cache"),
		"XDG_DATA_HOME="+filepath.Join(scratch, "data"),
		"XDG_STATE_HOME="+filepath.Join(scratch, "state"),
		"COREPACK_HOME="+filepath.Join(scratch, "corepack"),
		"NODE_USE_SYSTEM_CA=1",
		"npm_config_userconfig="+filepath.Join(scratch, "npmrc"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GOENV=off",
		"GOFLAGS=",
		"GOTOOLCHAIN=go"+pinnedGoVersion,
		"GOWORK=off",
		"GOCACHE="+filepath.Join(scratch, "go-build"),
		"GOPATH="+filepath.Join(scratch, "go-path"),
		"GOMODCACHE="+filepath.Join(scratch, "go-mod"),
		"PNPM_HOME="+filepath.Join(scratch, "pnpm-home"),
		"pnpm_config_store_dir="+filepath.Join(scratch, "pnpm-store"),
		"COMPOSE_PROJECT_NAME="+project,
		"SEMLIA_SECURITY_IMAGE=semlia:security",
		"SEMLIA_DOCKER_RESOURCE_LABEL="+acceptanceDockerLabelKey+"="+project,
		"TESTCONTAINERS_SESSION_ID="+project,
		"TESTCONTAINERS_RYUK_DISABLED=false",
		"TESTCONTAINERS_RYUK_CONTAINER_PRIVILEGED=false",
		"RYUK_CONNECTION_TIMEOUT=1m",
		"RYUK_RECONNECTION_TIMEOUT=10s",
		"RYUK_VERBOSE=false",
		fmt.Sprintf("SEMLIA_HTTP_PORT=%d", httpPort),
		fmt.Sprintf("SEMLIA_POSTGRES_PORT=%d", postgresPort),
	)
	if launcherRoot != "" {
		environment = append(environment,
			"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT="+launcherRoot,
			"NODE_EXTRA_CA_CERTS="+filepath.Join(launcherRoot, "system-ca.pem"),
		)
	}
	return environment
}

func filterAcceptanceEnvironment(environment []string) []string {
	blockedExact := map[string]struct{}{
		"GO": {}, "GOARCH": {}, "GOCACHE": {}, "GOENV": {}, "GOFLAGS": {}, "GOMODCACHE": {}, "GOOS": {}, "GOPATH": {}, "GOTOOLCHAIN": {}, "GOWORK": {},
		"BASHOPTS": {}, "BASH_ENV": {}, "ENV": {}, "SHELLOPTS": {}, "GNUMAKEFLAGS": {}, "MAKE": {}, "MAKEFILES": {}, "MAKEFLAGS": {}, "MAKELEVEL": {}, "MAKEOVERRIDES": {}, "MFLAGS": {}, "PNPM": {},
		"HOME": {}, "TMPDIR": {}, "NODE_EXTRA_CA_CERTS": {}, "NODE_OPTIONS": {}, "NODE_PATH": {}, "NODE_TLS_REJECT_UNAUTHORIZED": {}, "NODE_USE_BUNDLED_CA": {}, "NODE_USE_OPENSSL_CA": {}, "NODE_USE_SYSTEM_CA": {}, "OPENSSL_CONF": {}, "OPENSSL_MODULES": {}, "PNPM_CONFIG_STORE_DIR": {}, "PNPM_HOME": {}, "PNPM_STORE_DIR": {}, "SSL_CERT_FILE": {}, "SSL_CERT_DIR": {}, "SSLKEYLOGFILE": {}, "XDG_CACHE_HOME": {},
		"npm_config_store_dir": {}, "pnpm_config_store_dir": {},
		"PATH": {},
	}
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		_, exactBlocked := blockedExact[key]
		upperKey := strings.ToUpper(key)
		prefixBlocked := false
		for _, prefix := range []string{"BASH_FUNC_", "SEMLIA_", "COMPOSE_", "GIT_", "DOCKER_", "TESTCONTAINERS_", "RYUK_", "NODE_", "OPENSSL_", "SSL_", "NPM_", "PNPM_", "COREPACK_", "PLAYWRIGHT_", "XDG_"} {
			if strings.HasPrefix(upperKey, prefix) {
				prefixBlocked = true
				break
			}
		}
		if prefixBlocked || exactBlocked {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func trustedAcceptancePath() string {
	return strings.Join([]string{
		"/usr/local/go/bin",
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/home/linuxbrew/.linuxbrew/bin",
		"/home/linuxbrew/.linuxbrew/sbin",
		"/opt/local/bin",
		"/opt/local/sbin",
		"/usr/local/bin",
		"/usr/local/sbin",
		"/usr/bin",
		"/bin",
		"/usr/sbin",
		"/sbin",
		"/run/current-system/sw/bin",
		"/nix/var/nix/profiles/default/bin",
		"/snap/bin",
		"/var/lib/snapd/snap/bin",
	}, string(os.PathListSeparator))
}

func trustedAcceptancePathFrom(environment []string) string {
	directories := make([]string, 0, len(trustedHostedToolDirectories())+len(filepath.SplitList(trustedAcceptancePath())))
	seen := make(map[string]struct{}, cap(directories))
	for _, rawDirectory := range filepath.SplitList(environmentValue(environment, "PATH")) {
		if rawDirectory != filepath.Clean(rawDirectory) || !trustedDynamicToolDirectory(rawDirectory) {
			continue
		}
		directory := rawDirectory
		if _, exists := seen[directory]; exists {
			continue
		}
		directories = append(directories, directory)
		seen[directory] = struct{}{}
	}
	for _, directory := range filepath.SplitList(trustedAcceptancePath()) {
		if _, exists := seen[directory]; exists {
			continue
		}
		directories = append(directories, directory)
		seen[directory] = struct{}{}
	}
	return strings.Join(directories, string(os.PathListSeparator))
}

func trustedDynamicToolDirectory(directory string) bool {
	if !filepath.IsAbs(directory) {
		return false
	}
	for _, trusted := range trustedHostedToolDirectories() {
		if directory == trusted {
			return true
		}
	}
	return false
}

func trustedHostedToolDirectories() []string {
	return []string{
		"/home/runner/setup-pnpm/node_modules/.bin",
		"/Users/runner/setup-pnpm/node_modules/.bin",
		"/opt/hostedtoolcache/node/24.15.0/x64/bin",
		"/opt/hostedtoolcache/node/24.15.0/arm64/bin",
		"/Users/runner/hostedtoolcache/node/24.15.0/x64/bin",
		"/Users/runner/hostedtoolcache/node/24.15.0/arm64/bin",
		"/opt/hostedtoolcache/go/1.26.5/x64/bin",
		"/opt/hostedtoolcache/go/1.26.5/arm64/bin",
		"/Users/runner/hostedtoolcache/go/1.26.5/x64/bin",
		"/Users/runner/hostedtoolcache/go/1.26.5/arm64/bin",
	}
}

func canonicalLauncherTrustedPath() string {
	return strings.Join(append(trustedHostedToolDirectories(), filepath.SplitList(trustedAcceptancePath())...), string(os.PathListSeparator))
}

func validateFreshCloneToolPath(got string) error {
	want := canonicalLauncherTrustedPath()
	if got != want {
		return fmt.Errorf("fresh-clone acceptance received PATH=%q, want exact canonical PATH=%q; use scripts/acceptance/m0-fresh-clone.sh", got, want)
	}
	return nil
}

func TestFreshCloneInvocationRejectsAnyAmbientToolPath(t *testing.T) {
	want := canonicalLauncherTrustedPath()
	poison := t.TempDir()
	for _, test := range []struct {
		name string
		path string
	}{
		{name: "ambient prefix", path: poison + string(os.PathListSeparator) + want},
		{name: "ambient suffix", path: want + string(os.PathListSeparator) + poison},
		{name: "duplicate canonical path", path: want + string(os.PathListSeparator) + want},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateFreshCloneToolPath(test.path)
			if err == nil {
				t.Fatalf("accepted noncanonical PATH %q", test.path)
			}
			for _, detail := range []string{fmt.Sprintf("%q", test.path), fmt.Sprintf("%q", want)} {
				if !strings.Contains(err.Error(), detail) {
					t.Errorf("PATH error missing actionable detail %s: %v", detail, err)
				}
			}
		})
	}
}

func TestGoBootstrapReexecResetsGoTestPathAndRejectsAmbientPrefix(t *testing.T) {
	bootstrap := supportedGoBootstrap1263(t)
	scratch := t.TempDir()
	moduleRoot := filepath.Join(scratch, "module")
	poisonBin := filepath.Join(scratch, "poison-bin")
	for _, directory := range []string{moduleRoot, poisonBin, filepath.Join(scratch, "home"), filepath.Join(scratch, "go-build"), filepath.Join(scratch, "go-path")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(poisonBin, "semlia-ambient-poison"), []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte("module semliapathprobe\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := `package semliapathprobe

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func TestEffectivePath(t *testing.T) {
	fmt.Printf("PROBE_RUNTIME=%s\n", runtime.Version())
	fmt.Printf("PROBE_PATH=%s\n", os.Getenv("PATH"))
	if runtime.Version() != "go1.26.5" {
		t.Fatalf("runtime = %q, want go1.26.5", runtime.Version())
	}
	if got, want := os.Getenv("PATH"), os.Getenv("SEMLIA_EXPECTED_PATH"); got != want {
		t.Fatalf("PATH = %q, want %q", got, want)
	}
	if path, err := exec.LookPath("semlia-ambient-poison"); err == nil {
		t.Fatalf("ambient poison remains executable at %s", path)
	}
}
`
	if err := os.WriteFile(filepath.Join(moduleRoot, "path_test.go"), []byte(probe), 0o600); err != nil {
		t.Fatal(err)
	}

	canonicalPath := canonicalLauncherTrustedPath()
	command := exec.Command(
		bootstrap,
		"-C", moduleRoot,
		"test",
		"-exec=/usr/bin/env PATH="+canonicalPath,
		"-run=^TestEffectivePath$",
		"-count=1",
		"-v",
	)
	command.Env = append(
		filteredEnvironment(filterAcceptanceEnvironment(os.Environ()), "GOROOT", "GOTOOLCHAIN_INTERNAL_SWITCH_VERSION", "GOTOOLCHAIN_INTERNAL_SWITCH_COUNT"),
		"HOME="+filepath.Join(scratch, "home"),
		"PATH="+poisonBin+string(os.PathListSeparator)+canonicalPath,
		"GOENV=off",
		"GOFLAGS=",
		"GOWORK=off",
		"GOTOOLCHAIN=go1.26.5",
		"GOCACHE="+filepath.Join(scratch, "go-build"),
		"GOPATH="+filepath.Join(scratch, "go-path"),
		"GOMODCACHE="+activeGoModCache(t),
		"SEMLIA_EXPECTED_PATH="+canonicalPath,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run Go 1.26.3 to 1.26.5 PATH regression: %v\n%s", err, output)
	}
	for _, want := range []string{"PROBE_RUNTIME=go1.26.5", "PROBE_PATH=" + canonicalPath} {
		if !strings.Contains(string(output), want) {
			t.Errorf("nested Go test probe missing %q:\n%s", want, output)
		}
	}
}

func supportedGoBootstrap1263(t *testing.T) string {
	t.Helper()
	var candidates []string
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		candidates = []string{"/opt/homebrew/bin/go"}
	case "darwin/amd64", "linux/amd64", "linux/arm64":
		candidates = []string{"/usr/local/go/bin/go"}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err != nil || info.Mode()&0o111 == 0 {
			continue
		}
		command := exec.Command(candidate, "version")
		command.Env = []string{
			"HOME=" + t.TempDir(),
			"PATH=" + canonicalLauncherTrustedPath(),
			"GOENV=off",
			"GOFLAGS=",
			"GOWORK=off",
			"GOTOOLCHAIN=local",
		}
		if output, err := command.Output(); err == nil && strings.HasPrefix(string(output), "go version go1.26.3 ") {
			return candidate
		}
	}
	t.Skip("supported Go 1.26.3 bootstrap is not installed on this host")
	return ""
}

func activeGoModCache(t *testing.T) string {
	t.Helper()
	goTool := filepath.Join(runtime.GOROOT(), "bin", "go")
	command := exec.Command(goTool, "env", "GOMODCACHE")
	command.Env = append(
		filteredEnvironment(os.Environ(), "GOROOT", "GOENV", "GOFLAGS", "GOTOOLCHAIN", "GOWORK", "GOTOOLCHAIN_INTERNAL_SWITCH_VERSION", "GOTOOLCHAIN_INTERNAL_SWITCH_COUNT"),
		"GOENV=off",
		"GOFLAGS=",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
	)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("resolve active Go module cache: %v", err)
	}
	cache := strings.TrimSpace(string(output))
	if !filepath.IsAbs(cache) {
		t.Fatalf("active Go module cache is not absolute: %q", cache)
	}
	return cache
}

func prepareIsolatedEnvironmentDirectories(environment []string) error {
	for _, name := range []string{
		"HOME", "TMPDIR", "DOCKER_CONFIG", "XDG_CONFIG_HOME", "XDG_CACHE_HOME",
		"XDG_DATA_HOME", "XDG_STATE_HOME", "COREPACK_HOME", "GOCACHE", "GOPATH",
		"GOMODCACHE", "PNPM_HOME", "pnpm_config_store_dir",
	} {
		path := environmentValue(environment, name)
		if path == "" || !filepath.IsAbs(path) {
			return fmt.Errorf("%s must be an absolute task-local path", name)
		}
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", name, err)
		}
	}
	npmrc := environmentValue(environment, "npm_config_userconfig")
	if npmrc == "" || !filepath.IsAbs(npmrc) {
		return fmt.Errorf("npm_config_userconfig must be an absolute task-local path")
	}
	if err := os.WriteFile(npmrc, nil, 0o600); err != nil {
		return fmt.Errorf("create isolated npm configuration: %w", err)
	}
	return nil
}

func (run *freshCloneRun) validateGoToolchain(ctx context.Context) error {
	command, resolveErr := acceptanceCommandContext(ctx, run.environment, "go", "version")
	if resolveErr != nil {
		return fmt.Errorf("validate isolated Go toolchain: %w", resolveErr)
	}
	command.Dir = filepath.Dir(run.root)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err != nil {
		return fmt.Errorf("validate isolated Go toolchain: %w (stdout=%q stderr=%q)", err, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}
	if err := validateGoToolchainOutput(stdout.String(), stderr.String(), runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("validate isolated Go toolchain: %w", err)
	}
	return nil
}

func (run *freshCloneRun) validateNodeSystemCA(ctx context.Context) error {
	if err := validateTaskCABundle(run.environment); err != nil {
		return fmt.Errorf("validate isolated Node.js CA bundle: %w", err)
	}
	const probe = `const tls=require("node:tls");const bundled=tls.getCACertificates("bundled");const extra=tls.getCACertificates("extra");const defaults=tls.getCACertificates("default");const defaultSet=new Set(defaults);process.stdout.write(JSON.stringify({nodeVersion:process.versions.node,platform:process.platform,architecture:process.arch,nodeUseSystemCA:process.env.NODE_USE_SYSTEM_CA,bundledCertificateCount:bundled.length,extraCertificateCount:extra.length,defaultCertificateCount:defaults.length,extraCertificatesMerged:extra.every(certificate=>defaultSet.has(certificate))})+"\n")`
	probeCtx, cancel := context.WithTimeout(ctx, nodeSystemCAProbeTimeout)
	defer cancel()
	command, resolveErr := acceptanceCommandContext(probeCtx, run.environment, "node", "-e", probe)
	if resolveErr != nil {
		return fmt.Errorf("validate isolated Node.js system CA capability: %w", resolveErr)
	}
	command.Dir = filepath.Dir(run.root)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if probeCtx.Err() != nil {
			return fmt.Errorf("validate isolated Node.js system CA capability: command failed: %v: %w (stdout=%q stderr=%q)", err, probeCtx.Err(), strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
		}
		return fmt.Errorf("validate isolated Node.js system CA capability: %w (stdout=%q stderr=%q)", err, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}
	if err := validateNodeSystemCAOutput(stdout.String(), stderr.String()); err != nil {
		return fmt.Errorf("validate isolated Node.js system CA capability: %w", err)
	}
	return nil
}

type nodeSystemCAProbeResult struct {
	NodeVersion             string `json:"nodeVersion"`
	Platform                string `json:"platform"`
	Architecture            string `json:"architecture"`
	NodeUseSystemCA         string `json:"nodeUseSystemCA"`
	BundledCertificateCount int    `json:"bundledCertificateCount"`
	ExtraCertificateCount   int    `json:"extraCertificateCount"`
	DefaultCertificateCount int    `json:"defaultCertificateCount"`
	ExtraCertificatesMerged bool   `json:"extraCertificatesMerged"`
}

func validateNodeSystemCAOutput(stdout, stderr string) error {
	return validateNodeSystemCAOutputForTarget(stdout, stderr, runtime.GOOS, runtime.GOARCH)
}

func validateNodeSystemCAOutputForTarget(stdout, stderr, goos, goarch string) error {
	trimmedStdout := strings.TrimSpace(stdout)
	trimmedStderr := strings.TrimSpace(stderr)
	if trimmedStderr != "" {
		return fmt.Errorf("unexpected stderr=%q (stdout=%q)", trimmedStderr, trimmedStdout)
	}
	decoder := json.NewDecoder(strings.NewReader(trimmedStdout))
	decoder.DisallowUnknownFields()
	var result nodeSystemCAProbeResult
	if err := decoder.Decode(&result); err != nil {
		return fmt.Errorf("parse exact probe output %q: %w", trimmedStdout, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("probe output contains trailing data: %q", trimmedStdout)
	}
	wantPlatform, wantArchitecture, err := expectedNodeTarget(goos, goarch)
	if err != nil {
		return err
	}
	if result.NodeUseSystemCA != "1" || result.ExtraCertificateCount <= 0 || !result.ExtraCertificatesMerged || result.NodeVersion != "24.15.0" || result.Platform != wantPlatform || result.Architecture != wantArchitecture {
		return fmt.Errorf("NODE_USE_SYSTEM_CA=%q bundledCertificateCount=%d extraCertificateCount=%d defaultCertificateCount=%d extraCertificatesMerged=%t; Node %s on %s/%s requires the non-empty task CA bundle in the default trust set", result.NodeUseSystemCA, result.BundledCertificateCount, result.ExtraCertificateCount, result.DefaultCertificateCount, result.ExtraCertificatesMerged, result.NodeVersion, result.Platform, result.Architecture)
	}
	return nil
}

func expectedNodeTarget(goos, goarch string) (string, string, error) {
	if goos != "darwin" && goos != "linux" {
		return "", "", fmt.Errorf("unsupported Node.js CA probe platform %q", goos)
	}
	architecture := ""
	switch goarch {
	case "amd64":
		architecture = "x64"
	case "arm64":
		architecture = "arm64"
	default:
		return "", "", fmt.Errorf("unsupported Node.js CA probe architecture %q", goarch)
	}
	return goos, architecture, nil
}

func validateTaskCABundle(environment []string) error {
	launcherRoot := environmentValue(environment, "SEMLIA_ACCEPTANCE_LAUNCHER_ROOT")
	bundle := environmentValue(environment, "NODE_EXTRA_CA_CERTS")
	if launcherRoot == "" || !filepath.IsAbs(launcherRoot) || filepath.Clean(launcherRoot) != launcherRoot {
		return fmt.Errorf("SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=%q is not an absolute clean path", launcherRoot)
	}
	rootInfo, err := os.Lstat(launcherRoot)
	if err != nil {
		return fmt.Errorf("inspect acceptance launcher root: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || rootInfo.Mode().Perm() != 0o700 {
		return fmt.Errorf("acceptance launcher root must be a private physical directory: mode=%s", rootInfo.Mode())
	}
	want := filepath.Join(launcherRoot, "system-ca.pem")
	if bundle != want {
		return fmt.Errorf("NODE_EXTRA_CA_CERTS=%q, want task-owned %q", bundle, want)
	}
	pathInfo, err := os.Lstat(bundle)
	if err != nil {
		return fmt.Errorf("inspect task CA bundle: %w", err)
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Size() <= 0 || pathInfo.Mode().Perm() != 0o600 {
		return fmt.Errorf("task CA bundle must be a non-empty private physical regular file: mode=%s size=%d", pathInfo.Mode(), pathInfo.Size())
	}
	file, err := os.Open(bundle)
	if err != nil {
		return fmt.Errorf("open task CA bundle: %w", err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened task CA bundle: %w", err)
	}
	if !os.SameFile(pathInfo, openedInfo) || !openedInfo.Mode().IsRegular() || openedInfo.Size() <= 0 || openedInfo.Mode().Perm() != 0o600 {
		return fmt.Errorf("opened task CA bundle identity or permissions changed during validation")
	}
	body, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("read task CA bundle: %w", err)
	}
	rest := body
	certificates := 0
	for {
		rest = bytes.TrimSpace(rest)
		if len(rest) == 0 {
			break
		}
		if !bytes.HasPrefix(rest, []byte("-----BEGIN CERTIFICATE-----")) {
			return fmt.Errorf("task CA bundle contains data outside certificate PEM blocks")
		}
		block, remaining := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return fmt.Errorf("task CA bundle contains non-certificate PEM or trailing data")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return fmt.Errorf("task CA bundle contains an invalid certificate: %w", err)
		}
		certificates++
		rest = remaining
	}
	if certificates == 0 {
		return fmt.Errorf("task CA bundle contains no certificates")
	}
	return nil
}

func testCertificatePEM(t *testing.T) []byte {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Unix(1, 0),
		NotAfter:              time.Unix(2, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func writeTestCABundle(t *testing.T, launcherRoot string, body []byte, mode os.FileMode) string {
	t.Helper()
	if err := os.Chmod(launcherRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(launcherRoot, "system-ca.pem")
	if err := os.WriteFile(bundle, body, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bundle, mode); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func TestValidateTaskCABundle(t *testing.T) {
	certificate := testCertificatePEM(t)
	privateKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("not public")})
	for _, test := range []struct {
		name    string
		setup   func(*testing.T, string, string) []string
		wantErr bool
	}{
		{name: "valid", setup: func(t *testing.T, root, bundle string) []string {
			writeTestCABundle(t, root, certificate, 0o600)
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + bundle}
		}},
		{name: "wrong bundle path", wantErr: true, setup: func(t *testing.T, root, bundle string) []string {
			writeTestCABundle(t, root, certificate, 0o600)
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + filepath.Join(filepath.Dir(root), "outside.pem")}
		}},
		{name: "symlink bundle", wantErr: true, setup: func(t *testing.T, root, bundle string) []string {
			outside := filepath.Join(filepath.Dir(root), "outside.pem")
			if err := os.WriteFile(outside, certificate, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, bundle); err != nil {
				t.Fatal(err)
			}
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + bundle}
		}},
		{name: "directory bundle", wantErr: true, setup: func(t *testing.T, root, bundle string) []string {
			if err := os.Mkdir(bundle, 0o700); err != nil {
				t.Fatal(err)
			}
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + bundle}
		}},
		{name: "empty bundle", wantErr: true, setup: func(t *testing.T, root, bundle string) []string {
			writeTestCABundle(t, root, nil, 0o600)
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + bundle}
		}},
		{name: "read only owner mode", wantErr: true, setup: func(t *testing.T, root, bundle string) []string {
			writeTestCABundle(t, root, certificate, 0o400)
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + bundle}
		}},
		{name: "group readable mode", wantErr: true, setup: func(t *testing.T, root, bundle string) []string {
			writeTestCABundle(t, root, certificate, 0o640)
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + bundle}
		}},
		{name: "leading garbage", wantErr: true, setup: func(t *testing.T, root, bundle string) []string {
			writeTestCABundle(t, root, append([]byte("garbage\n"), certificate...), 0o600)
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + bundle}
		}},
		{name: "interstitial garbage", wantErr: true, setup: func(t *testing.T, root, bundle string) []string {
			body := append(append(append([]byte{}, certificate...), []byte("garbage\n")...), certificate...)
			writeTestCABundle(t, root, body, 0o600)
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + bundle}
		}},
		{name: "trailing garbage", wantErr: true, setup: func(t *testing.T, root, bundle string) []string {
			writeTestCABundle(t, root, append(append([]byte{}, certificate...), []byte("garbage\n")...), 0o600)
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + bundle}
		}},
		{name: "private key", wantErr: true, setup: func(t *testing.T, root, bundle string) []string {
			writeTestCABundle(t, root, privateKey, 0o600)
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + bundle}
		}},
		{name: "invalid certificate", wantErr: true, setup: func(t *testing.T, root, bundle string) []string {
			writeTestCABundle(t, root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid")}), 0o600)
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + bundle}
		}},
		{name: "nonprivate launcher root", wantErr: true, setup: func(t *testing.T, root, bundle string) []string {
			writeTestCABundle(t, root, certificate, 0o600)
			if err := os.Chmod(root, 0o755); err != nil {
				t.Fatal(err)
			}
			return []string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + bundle}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "launcher")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			bundle := filepath.Join(root, "system-ca.pem")
			err := validateTaskCABundle(test.setup(t, root, bundle))
			if (err != nil) != test.wantErr {
				t.Fatalf("validateTaskCABundle() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}

	t.Run("symlink launcher root", func(t *testing.T) {
		parent := t.TempDir()
		target := filepath.Join(parent, "target")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		writeTestCABundle(t, target, certificate, 0o600)
		root := filepath.Join(parent, "launcher")
		if err := os.Symlink(target, root); err != nil {
			t.Fatal(err)
		}
		err := validateTaskCABundle([]string{"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root, "NODE_EXTRA_CA_CERTS=" + filepath.Join(root, "system-ca.pem")})
		if err == nil {
			t.Fatal("validateTaskCABundle() accepted a symlink launcher root")
		}
	})
}

func TestValidateNodeSystemCAOutput(t *testing.T) {
	valid := `{"nodeVersion":"24.15.0","platform":"darwin","architecture":"arm64","nodeUseSystemCA":"1","bundledCertificateCount":145,"extraCertificateCount":4,"defaultCertificateCount":149,"extraCertificatesMerged":true}`
	duplicateOnlyValid := `{"nodeVersion":"24.15.0","platform":"linux","architecture":"x64","nodeUseSystemCA":"1","bundledCertificateCount":145,"extraCertificateCount":2,"defaultCertificateCount":145,"extraCertificatesMerged":true}`
	for _, test := range []struct {
		name    string
		stdout  string
		stderr  string
		goos    string
		goarch  string
		wantErr bool
	}{
		{name: "arm64 certificates merged", stdout: valid + "\n", goos: "darwin", goarch: "arm64"},
		{name: "amd64 maps to x64 with duplicate certificates", stdout: duplicateOnlyValid + "\n", goos: "linux", goarch: "amd64"},
		{name: "wrong Node architecture", stdout: strings.Replace(duplicateOnlyValid, `"architecture":"x64"`, `"architecture":"amd64"`, 1), goos: "linux", goarch: "amd64", wantErr: true},
		{name: "unsupported Go architecture", stdout: duplicateOnlyValid, goos: "linux", goarch: "riscv64", wantErr: true},
		{name: "unsupported platform", stdout: duplicateOnlyValid, goos: "windows", goarch: "amd64", wantErr: true},
		{name: "empty extra store", stdout: strings.Replace(valid, `"extraCertificateCount":4`, `"extraCertificateCount":0`, 1), goos: "darwin", goarch: "arm64", wantErr: true},
		{name: "extra certificates not merged", stdout: strings.Replace(valid, `"extraCertificatesMerged":true`, `"extraCertificatesMerged":false`, 1), goos: "darwin", goarch: "arm64", wantErr: true},
		{name: "unpinned environment", stdout: strings.Replace(valid, `"nodeUseSystemCA":"1"`, `"nodeUseSystemCA":"0"`, 1), goos: "darwin", goarch: "arm64", wantErr: true},
		{name: "wrong Node version", stdout: strings.Replace(valid, `"nodeVersion":"24.15.0"`, `"nodeVersion":"24.14.0"`, 1), goos: "darwin", goarch: "arm64", wantErr: true},
		{name: "malformed", stdout: `{`, goos: "darwin", goarch: "arm64", wantErr: true},
		{name: "unknown field", stdout: strings.TrimSuffix(valid, "}") + `,"unexpected":true}`, goos: "darwin", goarch: "arm64", wantErr: true},
		{name: "trailing data", stdout: valid + "\n{}", goos: "darwin", goarch: "arm64", wantErr: true},
		{name: "noisy stderr", stdout: valid, stderr: "warning", goos: "darwin", goarch: "arm64", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateNodeSystemCAOutputForTarget(test.stdout, test.stderr, test.goos, test.goarch)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateNodeSystemCAOutput() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestAcceptanceEnvironmentPinsNodeSystemCAAndScrubsTLSOverrides(t *testing.T) {
	poisonedNames := []string{
		"NODE_USE_SYSTEM_CA", "NODE_EXTRA_CA_CERTS", "NODE_OPTIONS", "NODE_PATH", "NODE_TLS_REJECT_UNAUTHORIZED", "NODE_USE_BUNDLED_CA", "NODE_USE_OPENSSL_CA",
		"OPENSSL_CONF", "OPENSSL_ENGINES", "OPENSSL_MODULES", "SSL_CERT_FILE", "SSL_CERT_DIR", "SSLKEYLOGFILE",
	}
	launcherRoot := t.TempDir()
	writeTestCABundle(t, launcherRoot, testCertificatePEM(t), 0o600)
	poisoned := []string{"PATH=" + os.Getenv("PATH"), "SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + launcherRoot, "NODE_EXTRA_CA_CERTS=/outside/poison"}
	for _, name := range poisonedNames {
		if name == "NODE_EXTRA_CA_CERTS" {
			continue
		}
		poisoned = append(poisoned, name+"=/outside/poison")
	}
	environment := isolatedEnvironmentFrom(poisoned, t.TempDir(), "semlia-accept-a1b2c3d4-000000000001", 38080, 35432)
	if values := environmentValues(environment, "NODE_USE_SYSTEM_CA"); len(values) != 1 || values[0] != "1" {
		t.Fatalf("NODE_USE_SYSTEM_CA values = %v, want exactly 1", values)
	}
	if values := environmentValues(environment, "NODE_EXTRA_CA_CERTS"); len(values) != 1 || values[0] != filepath.Join(launcherRoot, "system-ca.pem") {
		t.Fatalf("NODE_EXTRA_CA_CERTS values = %v, want the outer validated bundle for subsequent strict validation", values)
	}
	for _, name := range poisonedNames[2:] {
		if values := environmentValues(environment, name); len(values) != 0 {
			t.Errorf("isolated environment retained %s=%v", name, values)
		}
	}
}

func TestValidateNodeSystemCACapability(t *testing.T) {
	launcherRoot := t.TempDir()
	if err := os.Chmod(launcherRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(launcherRoot, "scratch")
	if err := os.MkdirAll(scratch, 0o700); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(launcherRoot, "system-ca.pem")
	if err := exportSystemCABundleForTest(bundle); err != nil {
		t.Skipf("host does not expose a non-empty Node system CA set: %v", err)
	}
	base := append(filteredEnvironment(os.Environ(), "SEMLIA_ACCEPTANCE_LAUNCHER_ROOT", "NODE_EXTRA_CA_CERTS"), "SEMLIA_ACCEPTANCE_LAUNCHER_ROOT="+launcherRoot, "NODE_EXTRA_CA_CERTS="+bundle)
	environment := isolatedEnvironmentFrom(base, scratch, "semlia-accept-a1b2c3d4-000000000001", 38080, 35432)
	if err := prepareIsolatedEnvironmentDirectories(environment); err != nil {
		t.Fatal(err)
	}
	run := &freshCloneRun{root: filepath.Join(scratch, "checkout"), environment: environment}
	ctx, cancel := context.WithTimeout(context.Background(), nodeSystemCAProbeTimeout)
	defer cancel()
	if err := run.validateNodeSystemCA(ctx); err != nil {
		t.Fatal(err)
	}
}

func exportSystemCABundleForTest(bundle string) error {
	environment := []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + trustedAcceptancePathFrom(os.Environ()),
		"NODE_USE_SYSTEM_CA=1",
	}
	node, err := acceptanceExecutablePath(environment, "node")
	if err != nil {
		return err
	}
	command := exec.Command(node, "-e", `const tls=require("node:tls");const certificates=tls.getCACertificates("system");if(certificates.length===0)process.exit(42);process.stdout.write(certificates.join("\n")+"\n")`)
	command.Env = environment
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if err != nil {
		return fmt.Errorf("export host system CA bundle: %w (stderr=%q)", err, strings.TrimSpace(stderr.String()))
	}
	if stderr.Len() != 0 {
		return fmt.Errorf("export host system CA bundle produced stderr=%q", strings.TrimSpace(stderr.String()))
	}
	return os.WriteFile(bundle, stdout.Bytes(), 0o600)
}

func TestValidateNodeSystemCACapabilityIsBounded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("supported macOS/Linux probe uses a shell stub")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	nodePath := filepath.Join(root, "node")
	if err := os.WriteFile(nodePath, []byte("#!/bin/sh\n/bin/sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "system-ca.pem")
	if err := os.WriteFile(bundle, []byte(`-----BEGIN CERTIFICATE-----
MIIEUTCCAzmgAwIBAgIQfK9pCiW3Of57m0R6wXjF7jANBgkqhkiG9w0BAQsFADBi
MQswCQYDVQQGEwJVUzETMBEGA1UEChMKQXBwbGUgSW5jLjEmMCQGA1UECxMdQXBw
bGUgQ2VydGlmaWNhdGlvbiBBdXRob3JpdHkxFjAUBgNVBAMTDUFwcGxlIFJvb3Qg
Q0EwHhcNMjAwMjE5MTgxMzQ3WhcNMzAwMjIwMDAwMDAwWjB1MUQwQgYDVQQDDDtB
cHBsZSBXb3JsZHdpZGUgRGV2ZWxvcGVyIFJlbGF0aW9ucyBDZXJ0aWZpY2F0aW9u
IEF1dGhvcml0eTELMAkGA1UECwwCRzMxEzARBgNVBAoMCkFwcGxlIEluYy4xCzAJ
BgNVBAYTAlVTMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA2PWJ/KhZ
C4fHTJEuLVaQ03gdpDDppUjvC0O/LYT7JF1FG+XrWTYSXFRknmxiLbTGl8rMPPbW
BpH85QKmHGq0edVny6zpPwcR4YS8Rx1mjjmi6LRJ7TrS4RBgeo6TjMrA2gzAg9Dj
+ZHWp4zIwXPirkbRYp2SqJBgN31ols2N4Pyb+ni743uvLRfdW/6AWSN1F7gSwe0b
5TTO/iK1nkmw5VW/j4SiPKi6xYaVFuQAyZ8D0MyzOhZ71gVcnetHrg21LYwOaU1A
0EtMOwSejSGxrC5DVDDOwYqGlJhL32oNP/77HK6XF8J4CjDgXx9UO0m3JQAaN4LS
VpelUkl8YDib7wIDAQABo4HvMIHsMBIGA1UdEwEB/wQIMAYBAf8CAQAwHwYDVR0j
BBgwFoAUK9BpR5R2Cf70a40uQKb3R01/CF4wRAYIKwYBBQUHAQEEODA2MDQGCCsG
AQUFBzABhihodHRwOi8vb2NzcC5hcHBsZS5jb20vb2NzcDAzLWFwcGxlcm9vdGNh
MC4GA1UdHwQnMCUwI6AhoB+GHWh0dHA6Ly9jcmwuYXBwbGUuY29tL3Jvb3QuY3Js
MB0GA1UdDgQWBBQJ/sAVkPmvZAqSErkmKGMMl+ynsjAOBgNVHQ8BAf8EBAMCAQYw
EAYKKoZIhvdjZAYCAQQCBQAwDQYJKoZIhvcNAQELBQADggEBAK1lE+j24IF3RAJH
Qr5fpTkg6mKp/cWQyXMT1Z6b0KoPjY3L7QHPbChAW8dVJEH4/M/BtSPp3Ozxb8qA
HXfCxGFJJWevD8o5Ja3T43rMMygNDi6hV0Bz+uZcrgZRKe3jhQxPYdwyFot30ETK
XXIDMUacrptAGvr04NM++i+MZp+XxFRZ79JI9AeZSWBZGcfdlNHAwWx/eCHvDOs7
bJmCS1JgOLU5gm3sUjFTvg+RTElJdI+mUcuER04ddSduvfnSXPN/wmwLCTbiZOTC
NwMUGdXqapSqqdv+9poIZ4vvK7iqF0mDr8/LvOnP6pVxsLRFoszlh6oKw0E6eVza
UDSdlTs=
-----END CERTIFICATE-----
`), 0o600); err != nil {
		t.Fatal(err)
	}
	run := &freshCloneRun{root: filepath.Join(root, "checkout"), environment: []string{"PATH=" + root, "NODE_USE_SYSTEM_CA=1", "NODE_EXTRA_CA_CERTS=" + bundle, "SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + root}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := run.validateNodeSystemCA(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("validateNodeSystemCA() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > acceptanceCommandWaitDelay+time.Second {
		t.Fatalf("cancelled Node system CA probe returned after %s", elapsed)
	}
}

func TestFreshCloneLauncherPinsSystemCAWithoutTLSBypass(t *testing.T) {
	launcherBytes, err := os.ReadFile(filepath.Join(repositoryRoot, "scripts", "acceptance", "m0-fresh-clone.sh"))
	if err != nil {
		t.Fatal(err)
	}
	launcher := string(launcherBytes)
	if count := strings.Count(launcher, "NODE_USE_SYSTEM_CA=1"); count != 3 {
		t.Fatalf("launcher NODE_USE_SYSTEM_CA pin count = %d, want version probe, CA export, and child pins", count)
	}
	if count := strings.Count(launcher, `"NODE_EXTRA_CA_CERTS=${SYSTEM_CA_BUNDLE}"`); count != 1 {
		t.Fatalf("launcher task CA bundle pin count = %d, want exactly 1", count)
	}
	for _, forbidden := range []string{"NODE_TLS_REJECT_UNAUTHORIZED", "SSL_CERT_FILE=", "SSL_CERT_DIR=", "strict-ssl=false", "strict_ssl=false"} {
		if strings.Contains(launcher, forbidden) {
			t.Errorf("launcher contains forbidden TLS override %q", forbidden)
		}
	}
}

func validateGoToolchainOutput(stdout, stderr, goos, goarch string) error {
	wantStdout := fmt.Sprintf("go version go%s %s/%s", pinnedGoVersion, goos, goarch)
	gotStdout := trimOneLineEnding(stdout)
	if gotStdout != wantStdout {
		return fmt.Errorf("stdout=%q stderr=%q, want stdout=%q", strings.TrimSpace(stdout), strings.TrimSpace(stderr), wantStdout)
	}
	gotStderr := trimOneLineEnding(stderr)
	wantColdDiagnostic := fmt.Sprintf("go: downloading go%s (%s/%s)", pinnedGoVersion, goos, goarch)
	if gotStderr != "" && gotStderr != wantColdDiagnostic {
		return fmt.Errorf("stdout=%q unexpected stderr=%q; allowed cold-toolchain stderr=%q", strings.TrimSpace(stdout), strings.TrimSpace(stderr), wantColdDiagnostic)
	}
	return nil
}

func trimOneLineEnding(value string) string {
	if strings.HasSuffix(value, "\r\n") {
		return strings.TrimSuffix(value, "\r\n")
	}
	return strings.TrimSuffix(value, "\n")
}

func TestValidateGoToolchainOutput(t *testing.T) {
	wantStdout := "go version go" + pinnedGoVersion + " darwin/arm64\n"
	wantColdDiagnostic := "go: downloading go" + pinnedGoVersion + " (darwin/arm64)\n"
	for _, test := range []struct {
		name    string
		stdout  string
		stderr  string
		wantErr bool
	}{
		{name: "warm", stdout: wantStdout},
		{name: "cold pinned toolchain", stdout: wantStdout, stderr: wantColdDiagnostic},
		{name: "wrong version", stdout: "go version go1.26.4 darwin/arm64\n", wantErr: true},
		{name: "wrong platform", stdout: "go version go" + pinnedGoVersion + " linux/amd64\n", wantErr: true},
		{name: "unexpected stderr", stdout: wantStdout, stderr: "go: warning: unexpected\n", wantErr: true},
		{name: "extra download stderr", stdout: wantStdout, stderr: wantColdDiagnostic + "go: downloading example.invalid/module v1.0.0\n", wantErr: true},
		{name: "extra stdout", stdout: wantStdout + "extra\n", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateGoToolchainOutput(test.stdout, test.stderr, "darwin", "arm64")
			if (err != nil) != test.wantErr {
				t.Fatalf("validateGoToolchainOutput() error = %v, wantErr %t", err, test.wantErr)
			}
			if test.wantErr {
				for _, want := range []string{fmt.Sprintf("%q", strings.TrimSpace(test.stdout)), fmt.Sprintf("%q", strings.TrimSpace(test.stderr))} {
					if want != "" && !strings.Contains(err.Error(), want) {
						t.Errorf("error %q missing diagnostic %q", err, want)
					}
				}
			}
		})
	}
}

func TestMakeTaskScratchRemovableDoesNotFollowSymlinks(t *testing.T) {
	parent := t.TempDir()
	scratch := filepath.Join(parent, "scratch")
	readOnlyDirectory := filepath.Join(scratch, "go-mod", "toolchain")
	if err := os.MkdirAll(readOnlyDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	readOnlyFile := filepath.Join(readOnlyDirectory, "LICENSE")
	if err := os.WriteFile(readOnlyFile, []byte("license"), 0o444); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.WriteFile(outside, []byte("sentinel"), 0o400); err != nil {
		t.Fatal(err)
	}
	outsideBefore, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(readOnlyDirectory, "escape")); err != nil {
		t.Fatal(err)
	}
	outsideDirectory := filepath.Join(parent, "outside-directory")
	if err := os.Mkdir(outsideDirectory, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDirectory, filepath.Join(readOnlyDirectory, "directory-escape")); err != nil {
		t.Fatal(err)
	}
	outsideDirectoryBefore, err := os.Stat(outsideDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readOnlyDirectory, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(scratch, "go-mod"), 0o555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(scratch, 0o555); err != nil {
		t.Fatal(err)
	}

	if err := makeTaskScratchRemovable(parent, scratch); err != nil {
		t.Fatal(err)
	}
	outsideAfter, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := outsideAfter.Mode().Perm(), outsideBefore.Mode().Perm(); got != want {
		t.Fatalf("scratch cleanup changed external symlink target mode: got %04o, want %04o", got, want)
	}
	outsideDirectoryAfter, err := os.Stat(outsideDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := outsideDirectoryAfter.Mode().Perm(), outsideDirectoryBefore.Mode().Perm(); got != want {
		t.Fatalf("scratch cleanup changed external directory symlink target mode: got %04o, want %04o", got, want)
	}
	if err := os.RemoveAll(scratch); err != nil {
		t.Fatalf("remove scratch after making read-only tree removable: %v", err)
	}
	if body, err := os.ReadFile(outside); err != nil || string(body) != "sentinel" {
		t.Fatalf("external symlink target changed: body=%q err=%v", body, err)
	}
}

func TestMakeTaskScratchRemovableRejectsUnsafePaths(t *testing.T) {
	parent := t.TempDir()
	sibling := t.TempDir()
	symlinkScratch := filepath.Join(parent, "symlink-scratch")
	if err := os.Symlink(sibling, symlinkScratch); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		base    string
		scratch string
	}{
		{base: parent, scratch: ""},
		{base: parent, scratch: "."},
		{base: parent, scratch: parent + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "unclean"},
		{base: parent, scratch: string(os.PathSeparator)},
		{base: parent, scratch: sibling},
		{base: parent, scratch: symlinkScratch},
		{base: string(os.PathSeparator), scratch: parent},
	} {
		if err := makeTaskScratchRemovable(test.base, test.scratch); err == nil {
			t.Fatalf("unsafe scratch base=%q path=%q was accepted", test.base, test.scratch)
		}
	}
}

func TestNewFreshCloneRunRegistersEarlyScratchCleanup(t *testing.T) {
	subtestPassed := t.Run("readonly scratch", func(t *testing.T) {
		config := freshCloneConfig{ref: strings.Repeat("a", 40)}
		run := newFreshCloneRun(t, config)
		readOnlyDirectory := filepath.Join(run.scratch, "go-mod", "golang.org", "toolchain")
		if err := os.MkdirAll(readOnlyDirectory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(readOnlyDirectory, "LICENSE"), []byte("license"), 0o444); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{readOnlyDirectory, filepath.Dir(readOnlyDirectory), filepath.Dir(filepath.Dir(readOnlyDirectory)), filepath.Join(run.scratch, "go-mod"), run.scratch} {
			if err := os.Chmod(path, 0o555); err != nil {
				t.Fatal(err)
			}
		}
	})
	if !subtestPassed {
		t.Fatal("early task scratch cleanup failed for a pre-clone read-only toolchain tree")
	}
}

func makeTaskScratchRemovable(scratchBase, scratch string) error {
	if scratchBase == "" || !filepath.IsAbs(scratchBase) || filepath.Clean(scratchBase) != scratchBase || filepath.Dir(scratchBase) == scratchBase {
		return fmt.Errorf("unsafe task scratch base %q", scratchBase)
	}
	if scratch == "" || !filepath.IsAbs(scratch) || filepath.Clean(scratch) != scratch || filepath.Dir(scratch) == scratch {
		return fmt.Errorf("unsafe task scratch path %q", scratch)
	}
	baseInfo, err := os.Lstat(scratchBase)
	if err != nil {
		return fmt.Errorf("inspect task scratch base %q: %w", scratchBase, err)
	}
	if baseInfo.Mode()&os.ModeSymlink != 0 || !baseInfo.IsDir() {
		return fmt.Errorf("task scratch base %q is not a physical directory", scratchBase)
	}
	relative, err := filepath.Rel(scratchBase, scratch)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("task scratch %q is not a strict descendant of %q", scratch, scratchBase)
	}
	info, err := os.Lstat(scratch)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect task scratch %q: %w", scratch, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("task scratch %q is not a physical directory", scratch)
	}
	return filepath.WalkDir(scratch, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if err := os.Chmod(path, info.Mode().Perm()|0o700); err != nil {
			return fmt.Errorf("make task scratch path removable %q: %w", path, err)
		}
		return nil
	})
}

type dockerCLIPluginMetadata struct {
	SchemaVersion    string `json:"SchemaVersion"`
	Vendor           string `json:"Vendor"`
	Version          string `json:"Version"`
	ShortDescription string `json:"ShortDescription"`
}

func (run *freshCloneRun) prepareDockerCLI(ctx context.Context) error {
	dockerOutput, err := executeSubprocess(ctx, run.root, run.environment, "docker", "version")
	if err != nil {
		return fmt.Errorf("validate isolated Docker CLI and daemon: %w: %s", err, strings.TrimSpace(run.redactTaskSecrets(string(dockerOutput))))
	}
	if _, composeErr := executeSubprocess(ctx, run.root, run.environment, "docker", "compose", "version"); composeErr == nil {
		return nil
	}

	plugin, err := acceptanceExecutablePath(run.environment, "docker-compose")
	if err != nil {
		return fmt.Errorf("locate a supported Docker Compose capability for the isolated Docker configuration: %w", err)
	}
	metadataOutput, metadataErr := executeSubprocess(ctx, run.root, run.environment, plugin, "docker-cli-plugin-metadata")
	if metadataErr != nil {
		return fmt.Errorf("validate Docker Compose plugin metadata: %w: %s", metadataErr, strings.TrimSpace(string(metadataOutput)))
	}
	metadata := dockerCLIPluginMetadata{}
	if err := json.Unmarshal(metadataOutput, &metadata); err != nil {
		return fmt.Errorf("validate Docker Compose plugin metadata: parse JSON: %w", err)
	}
	if metadata.SchemaVersion == "" || metadata.Vendor == "" || metadata.Version == "" || !strings.Contains(strings.ToLower(metadata.ShortDescription), "compose") {
		return fmt.Errorf("validate Docker Compose plugin metadata: incomplete capability metadata: %+v", metadata)
	}

	pluginDirectory := filepath.Join(environmentValue(run.environment, "DOCKER_CONFIG"), "cli-plugins")
	if err := os.MkdirAll(pluginDirectory, 0o700); err != nil {
		return fmt.Errorf("create task-local Docker CLI plugin directory: %w", err)
	}
	installedPlugin := filepath.Join(pluginDirectory, "docker-compose")
	if _, err := os.Lstat(installedPlugin); err == nil {
		return fmt.Errorf("task-local Docker Compose plugin path already exists")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect task-local Docker Compose plugin path: %w", err)
	}
	if err := os.Symlink(plugin, installedPlugin); err != nil {
		return fmt.Errorf("link validated Docker Compose capability into task-local configuration: %w", err)
	}
	composeOutput, composeErr := executeSubprocess(ctx, run.root, run.environment, "docker", "compose", "version")
	if composeErr != nil {
		return fmt.Errorf("validate task-local Docker Compose capability: %w: %s", composeErr, strings.TrimSpace(string(composeOutput)))
	}
	return nil
}

func validateFreshCloneInvocation() error {
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"GOENV", os.Getenv("GOENV"), "off"},
		{"GOFLAGS", os.Getenv("GOFLAGS"), ""},
		{"GOTOOLCHAIN", os.Getenv("GOTOOLCHAIN"), "go" + pinnedGoVersion},
		{"GOWORK", os.Getenv("GOWORK"), "off"},
		{"NODE_USE_SYSTEM_CA", os.Getenv("NODE_USE_SYSTEM_CA"), "1"},
	}
	for _, check := range checks {
		if check.got != check.want {
			return fmt.Errorf("fresh-clone acceptance requires %s=%q; use scripts/acceptance/m0-fresh-clone.sh", check.name, check.want)
		}
	}
	launcherRoot := os.Getenv("SEMLIA_ACCEPTANCE_LAUNCHER_ROOT")
	if launcherRoot == "" || !filepath.IsAbs(launcherRoot) {
		return fmt.Errorf("fresh-clone acceptance requires an absolute SEMLIA_ACCEPTANCE_LAUNCHER_ROOT; use scripts/acceptance/m0-fresh-clone.sh")
	}
	paths := map[string]string{
		"HOME":                  "home",
		"TMPDIR":                "tmp",
		"DOCKER_CONFIG":         "docker-config",
		"XDG_CONFIG_HOME":       "config",
		"XDG_CACHE_HOME":        "cache",
		"XDG_DATA_HOME":         "data",
		"XDG_STATE_HOME":        "state",
		"COREPACK_HOME":         "corepack",
		"npm_config_userconfig": "npmrc",
		"GOCACHE":               "go-build",
		"GOPATH":                "go-path",
		"GOMODCACHE":            "go-mod",
	}
	for name, relative := range paths {
		want := filepath.Join(launcherRoot, relative)
		if got := os.Getenv(name); got != want {
			return fmt.Errorf("fresh-clone acceptance requires %s=%q, got %q; use scripts/acceptance/m0-fresh-clone.sh", name, want, got)
		}
	}
	if got, want := os.Getenv("NODE_EXTRA_CA_CERTS"), filepath.Join(launcherRoot, "system-ca.pem"); got != want {
		return fmt.Errorf("fresh-clone acceptance requires NODE_EXTRA_CA_CERTS=%q, got %q; use scripts/acceptance/m0-fresh-clone.sh", want, got)
	}
	if err := validateTaskCABundle(os.Environ()); err != nil {
		return fmt.Errorf("fresh-clone acceptance requires a valid task CA bundle: %w", err)
	}
	if err := validateFreshCloneToolPath(os.Getenv("PATH")); err != nil {
		return err
	}
	if got := os.Getenv("GIT_CONFIG_NOSYSTEM"); got != "1" {
		return fmt.Errorf("fresh-clone acceptance requires GIT_CONFIG_NOSYSTEM=%q; use scripts/acceptance/m0-fresh-clone.sh", "1")
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

func TestFreshCloneInvocationRejectsMissingOrWrongNodeSystemCA(t *testing.T) {
	const helper = "SEMLIA_NODE_SYSTEM_CA_INVOCATION_HELPER"
	if os.Getenv(helper) == "1" {
		if err := validateFreshCloneInvocation(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}
	launcherRoot := t.TempDir()
	base := freshCloneInvocationTestEnvironment(t, launcherRoot)
	base = filteredEnvironment(base, "NODE_USE_SYSTEM_CA")
	base = append(base, helper+"=1")
	for _, value := range []string{"", "0", "true"} {
		command := exec.Command(os.Args[0], "-test.run=^TestFreshCloneInvocationRejectsMissingOrWrongNodeSystemCA$", "-test.count=1", "-test.timeout=100m")
		command.Env = append([]string{}, base...)
		if value != "" {
			command.Env = append(command.Env, "NODE_USE_SYSTEM_CA="+value)
		}
		output, err := command.CombinedOutput()
		if err == nil || !strings.Contains(string(output), `fresh-clone acceptance requires NODE_USE_SYSTEM_CA="1"`) {
			t.Errorf("NODE_USE_SYSTEM_CA=%q rejection: err=%v output=%s", value, err, output)
		}
	}
}

func freshCloneInvocationTestEnvironment(t *testing.T, launcherRoot string) []string {
	t.Helper()
	writeTestCABundle(t, launcherRoot, testCertificatePEM(t), 0o600)
	return []string{
		"PATH=" + canonicalLauncherTrustedPath(),
		"HOME=" + filepath.Join(launcherRoot, "home"),
		"TMPDIR=" + filepath.Join(launcherRoot, "tmp"),
		"DOCKER_CONFIG=" + filepath.Join(launcherRoot, "docker-config"),
		"XDG_CONFIG_HOME=" + filepath.Join(launcherRoot, "config"),
		"XDG_CACHE_HOME=" + filepath.Join(launcherRoot, "cache"),
		"XDG_DATA_HOME=" + filepath.Join(launcherRoot, "data"),
		"XDG_STATE_HOME=" + filepath.Join(launcherRoot, "state"),
		"COREPACK_HOME=" + filepath.Join(launcherRoot, "corepack"),
		"npm_config_userconfig=" + filepath.Join(launcherRoot, "npmrc"),
		"GIT_CONFIG_NOSYSTEM=1", "GOENV=off", "GOFLAGS=", "GOTOOLCHAIN=go" + pinnedGoVersion, "GOWORK=off",
		"GOCACHE=" + filepath.Join(launcherRoot, "go-build"), "GOPATH=" + filepath.Join(launcherRoot, "go-path"), "GOMODCACHE=" + filepath.Join(launcherRoot, "go-mod"),
		"NODE_USE_SYSTEM_CA=1", "NODE_EXTRA_CA_CERTS=" + filepath.Join(launcherRoot, "system-ca.pem"),
		"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT=" + launcherRoot,
	}
}

func TestFreshCloneInvocationRejectsInvalidCABundle(t *testing.T) {
	const helper = "SEMLIA_CA_BUNDLE_INVOCATION_HELPER"
	if os.Getenv(helper) == "1" {
		if err := validateFreshCloneInvocation(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}
	for _, test := range []struct {
		name string
		edit func(*testing.T, string, []string) []string
		want string
	}{
		{name: "missing path", want: "requires NODE_EXTRA_CA_CERTS", edit: func(t *testing.T, root string, environment []string) []string {
			return filteredEnvironment(environment, "NODE_EXTRA_CA_CERTS")
		}},
		{name: "wrong path", want: "requires NODE_EXTRA_CA_CERTS", edit: func(t *testing.T, root string, environment []string) []string {
			return append(filteredEnvironment(environment, "NODE_EXTRA_CA_CERTS"), "NODE_EXTRA_CA_CERTS="+filepath.Join(filepath.Dir(root), "outside.pem"))
		}},
		{name: "wrong mode", want: "requires a valid task CA bundle", edit: func(t *testing.T, root string, environment []string) []string {
			if err := os.Chmod(filepath.Join(root, "system-ca.pem"), 0o400); err != nil {
				t.Fatal(err)
			}
			return environment
		}},
		{name: "invalid content", want: "requires a valid task CA bundle", edit: func(t *testing.T, root string, environment []string) []string {
			if err := os.WriteFile(filepath.Join(root, "system-ca.pem"), []byte("not a certificate\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return environment
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			launcherRoot := t.TempDir()
			environment := test.edit(t, launcherRoot, freshCloneInvocationTestEnvironment(t, launcherRoot))
			command := exec.Command(os.Args[0], "-test.run=^TestFreshCloneInvocationRejectsInvalidCABundle$", "-test.count=1", "-test.timeout=100m")
			command.Env = append(environment, helper+"=1")
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.want) {
				t.Fatalf("invalid CA bundle rejection: err=%v want=%q output=%s", err, test.want, output)
			}
		})
	}
}

func acquireAcceptanceLock(t *testing.T) func() {
	t.Helper()
	release, err := createAcceptanceLock(acceptanceHostLock)
	if err != nil {
		t.Fatalf("another T008 acceptance run is active on this Docker host (%s): %v", acceptanceHostLock, err)
	}
	return func() {
		if err := release(); err != nil {
			t.Errorf("release T008 host lock: %v", err)
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

func TestFreshCloneConfigRequiresAbsoluteSource(t *testing.T) {
	config := freshCloneConfig{source: ".", ref: strings.Repeat("a", 40)}
	err := config.validate()
	if err == nil || !strings.Contains(err.Error(), "absolute local repository directory") {
		t.Fatalf("relative acceptance source was accepted: %v", err)
	}
}

func TestAcceptanceEnvironmentRejectsPoisonedControls(t *testing.T) {
	poisoned := []string{
		"PATH=/usr/bin", "HOME=/tmp/home", "SEMLIA_RUN_SMOKE=1", "SEMLIA_RELEASE_DIR=/outside",
		"SEMLIA_SECURITY_IMAGE=wrong", "SEMLIA_POSTGRES_PASSWORD=wrong", "COMPOSE_FILE=/outside/compose.yaml",
		"COMPOSE_PROFILES=wrong", "COMPOSE_PATH_SEPARATOR=;", "GIT_DIR=/outside/git", "GIT_CONFIG_COUNT=1",
		"DOCKER_CONFIG=/outside/docker", "DOCKER_CONTEXT=attacker", "DOCKER_HOST=tcp://attacker.invalid:2375",
		"GOENV=/outside/go.env", "GOFLAGS=-run=^$", "GOWORK=/outside/go.work", "GOTOOLCHAIN=attacker", "GOCACHE=/outside/go-cache",
		"GOPATH=/outside/go-path", "GOMODCACHE=/outside/go-mod", "GO=false", "GOOS=plan9", "GOARCH=386",
		"BASHOPTS=extdebug", "BASH_ENV=/outside/bash-env", "ENV=/outside/env", "SHELLOPTS=noexec", "GNUMAKEFLAGS=-i", "MAKE=true", "MAKEFILES=/outside/makefile",
		"MAKEFLAGS=-i", "MAKELEVEL=99", "MAKEOVERRIDES=ACCEPTANCE_SENTINEL=poisoned", "MFLAGS=-k", "PNPM=false", "PNPM_CONFIG_STORE_DIR=/outside/pnpm",
		"PNPM_STORE_DIR=/outside/legacy-pnpm", "PNPM_CONFIG_SCRIPT_SHELL=/outside/fake-shell", "npm_config_store_dir=/outside/npm", "npm_config_script_shell=/outside/fake-shell", "pnpm_config_store_dir=/outside/lower-pnpm",
		"PNPM_SCRIPT_SRC_DIR=/outside/source", "npm_lifecycle_event=test", "NODE_OPTIONS=--require=/outside/fake.js", "NODE_PATH=/outside/node_modules", "PLAYWRIGHT_BROWSERS_PATH=/outside/browsers",
		"COREPACK_HOME=/outside/corepack", "XDG_CONFIG_HOME=/outside/config", "XDG_DATA_HOME=/outside/data", "XDG_STATE_HOME=/outside/state",
		"BASH_FUNC_make%%=() { return 0; }", "BASH_FUNC_go%%=() { return 0; }", "BASH_FUNC_docker%%=() { return 0; }", "BASH_FUNC_pnpm%%=() { return 0; }",
		"TESTCONTAINERS_RYUK_DISABLED=true", "TESTCONTAINERS_SESSION_ID=attacker", "RYUK_CONNECTION_TIMEOUT=1ns",
	}
	got := filterAcceptanceEnvironment(poisoned)
	if len(got) != 0 {
		t.Fatalf("filtered environment retained control variables: %v", got)
	}
}

func TestAcceptanceEnvironmentRejectsExportedBashFunctionPoisoningInNestedTools(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("locate bash required by the acceptance toolchain: %v", err)
	}
	bin := t.TempDir()
	path := bin + string(os.PathListSeparator) + os.Getenv("PATH")
	for _, tool := range []string{"make", "go", "docker", "pnpm"} {
		tool := tool
		t.Run(tool, func(t *testing.T) {
			stub := "#!/bin/sh\nprintf 'real-" + tool + "\\n'\nexit 41\n"
			if err := os.WriteFile(filepath.Join(bin, tool), []byte(stub), 0o755); err != nil {
				t.Fatal(err)
			}
			functionEntry := "BASH_FUNC_" + tool + "%%=() { printf 'poisoned-" + tool + "\\n'; return 0; }"
			poisonedEnvironment := []string{
				"PATH=" + path,
				"HOME=" + t.TempDir(),
				functionEntry,
			}

			poisoned := exec.Command(bash, "-c", tool)
			poisoned.Env = poisonedEnvironment
			poisonedOutput, poisonedErr := poisoned.CombinedOutput()
			if poisonedErr != nil || strings.TrimSpace(string(poisonedOutput)) != "poisoned-"+tool {
				t.Fatalf("test precondition did not reproduce exported-function fake green: err=%v output=%q", poisonedErr, poisonedOutput)
			}

			isolated := exec.Command(bash, "-c", tool)
			isolated.Env = append(filterAcceptanceEnvironment(poisonedEnvironment), "PATH="+path)
			isolatedOutput, isolatedErr := isolated.CombinedOutput()
			var exitError *exec.ExitError
			if !errors.As(isolatedErr, &exitError) || exitError.ExitCode() != 41 {
				t.Fatalf("nested %s did not execute the real failing binary: err=%v output=%q", tool, isolatedErr, isolatedOutput)
			}
			if strings.TrimSpace(string(isolatedOutput)) != "real-"+tool {
				t.Fatalf("nested %s output = %q, want real binary marker", tool, isolatedOutput)
			}
		})
	}
}

func TestAcceptanceHostLockPathIgnoresTMPDIR(t *testing.T) {
	const helperEnvironment = "T008_ACCEPTANCE_LOCK_PATH_HELPER"
	if os.Getenv(helperEnvironment) == "1" {
		fmt.Printf("LOCK_PATH=%s\n", acceptanceHostLock)
		return
	}

	var paths []string
	for _, temporaryDirectory := range []string{t.TempDir(), t.TempDir()} {
		command := exec.Command(os.Args[0], "-test.run=^TestAcceptanceHostLockPathIgnoresTMPDIR$", "-test.count=1")
		command.Env = append(
			filteredEnvironment(os.Environ(), "TMPDIR", helperEnvironment),
			"TMPDIR="+temporaryDirectory,
			helperEnvironment+"=1",
		)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("resolve acceptance lock with TMPDIR=%q: %v\n%s", temporaryDirectory, err, output)
		}
		var lockPath string
		for _, line := range strings.Split(string(output), "\n") {
			if value, found := strings.CutPrefix(strings.TrimSpace(line), "LOCK_PATH="); found {
				lockPath = value
				break
			}
		}
		if lockPath == "" {
			t.Fatalf("lock-path helper returned no path:\n%s", output)
		}
		paths = append(paths, lockPath)
	}
	if paths[0] != paths[1] {
		t.Fatalf("different TMPDIR values bypass the Docker-host lock: %q != %q", paths[0], paths[1])
	}
	if paths[0] != "/tmp/semlia-t008-acceptance.lock" {
		t.Fatalf("Docker-host lock path = %q, want fixed ordinary-user-writable /tmp path", paths[0])
	}
}

func TestAcceptanceEnvironmentPinsGoControls(t *testing.T) {
	poisoned := []string{
		"PATH=/usr/bin", "GOENV=/outside/go.env", "GOFLAGS=-run=^$", "GOWORK=/outside/go.work", "GOTOOLCHAIN=local",
		"GO=false", "GOOS=plan9", "GOARCH=386", "BASHOPTS=extdebug", "BASH_ENV=/outside/bash-env", "ENV=/outside/env", "SHELLOPTS=noexec",
		"GNUMAKEFLAGS=-i", "MAKE=true", "MAKEFILES=/outside/makefile", "MAKELEVEL=99", "MAKEOVERRIDES=poisoned", "PNPM=false",
	}
	environment := isolatedEnvironmentFrom(poisoned, t.TempDir(), "semlia-accept-a1b2c3d4-000000000001", 38080, 35432)
	for name, want := range map[string]string{
		"GOENV":       "off",
		"GOFLAGS":     "",
		"GOWORK":      "off",
		"GOTOOLCHAIN": "go" + pinnedGoVersion,
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

func TestAcceptanceEnvironmentPinsTrustedToolPath(t *testing.T) {
	poisonedPath := t.TempDir()
	environment := isolatedEnvironmentFrom(
		[]string{"PATH=" + poisonedPath, "HOME=" + os.Getenv("HOME")},
		t.TempDir(),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	values := environmentValues(environment, "PATH")
	if len(values) != 1 || values[0] != trustedAcceptancePath() {
		t.Fatalf("isolated PATH values = %v, want exactly the trusted platform path", values)
	}
	if strings.Contains(values[0], poisonedPath) {
		t.Fatalf("isolated PATH retained ambient directory %q", poisonedPath)
	}
}

func TestAcceptanceEnvironmentRejectsHostedToolLookalikes(t *testing.T) {
	poisoned := t.TempDir()
	basePath := strings.Join([]string{
		poisoned,
		filepath.Join(poisoned, "opt", "hostedtoolcache", "go", "1.26.5", "x64", "bin"),
		"/opt/hostedtoolcache/go/latest/x64/bin",
		"/opt/hostedtoolcache/go/1.26.5/x64/bin/..",
		"/opt/hostedtoolcache/arbitrary/1.26.5/x64/bin",
	}, string(os.PathListSeparator))
	environment := isolatedEnvironmentFrom(
		[]string{"PATH=" + basePath},
		t.TempDir(),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	if got := environmentValue(environment, "PATH"); got != trustedAcceptancePath() {
		t.Fatalf("isolated PATH trusted hosted-tool lookalike: %q", got)
	}
}

func TestAcceptanceEnvironmentAllowsExactHostedToolPaths(t *testing.T) {
	hostedGo := "/opt/hostedtoolcache/go/1.26.5/x64/bin"
	hostedNode := "/opt/hostedtoolcache/node/24.15.0/x64/bin"
	hostedPnpm := "/home/runner/setup-pnpm/node_modules/.bin"
	basePath := strings.Join([]string{hostedGo, hostedNode, hostedPnpm, hostedGo}, string(os.PathListSeparator))
	environment := isolatedEnvironmentFrom(
		[]string{"PATH=" + basePath},
		t.TempDir(),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	want := strings.Join([]string{hostedGo, hostedNode, hostedPnpm, trustedAcceptancePath()}, string(os.PathListSeparator))
	if got := environmentValue(environment, "PATH"); got != want {
		t.Fatalf("isolated hosted-tool PATH = %q, want %q", got, want)
	}
}

func TestAcceptanceEnvironmentAllowsLinuxGitHubHostedToolPaths(t *testing.T) {
	basePath := strings.Join([]string{
		"/opt/hostedtoolcache/go/1.26.5/x64/bin",
		"/opt/hostedtoolcache/node/24.15.0/x64/bin",
		"/home/runner/setup-pnpm/node_modules/.bin",
	}, string(os.PathListSeparator))
	environment := isolatedEnvironmentFrom(
		[]string{"PATH=" + basePath},
		t.TempDir(),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	want := strings.Join([]string{basePath, trustedAcceptancePath()}, string(os.PathListSeparator))
	if got := environmentValue(environment, "PATH"); got != want {
		t.Fatalf("Linux GitHub-hosted PATH = %q, want %q", got, want)
	}
}

func TestAcceptanceEnvironmentPrioritizesExactHostedToolsOverGenericFallbacks(t *testing.T) {
	hostedNode := "/opt/hostedtoolcache/node/24.15.0/x64/bin"
	hostedPnpm := "/home/runner/setup-pnpm/node_modules/.bin"
	environment := isolatedEnvironmentFrom(
		[]string{"PATH=" + strings.Join([]string{"/usr/local/bin", hostedNode, hostedPnpm}, string(os.PathListSeparator))},
		t.TempDir(),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	directories := filepath.SplitList(environmentValue(environment, "PATH"))
	positions := make(map[string]int, len(directories))
	for index, directory := range directories {
		positions[directory] = index
	}
	for _, hosted := range []string{hostedNode, hostedPnpm} {
		if positions[hosted] >= positions["/usr/local/bin"] {
			t.Fatalf("hosted tool directory %q appears after generic fallback in %v", hosted, directories)
		}
	}
}

func TestAcceptanceEnvironmentSupportsActiveToolchainPath(t *testing.T) {
	environment := isolatedEnvironmentFrom(
		os.Environ(),
		t.TempDir(),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	for _, tool := range []string{"make", "go", "docker", "pnpm", "node", "git"} {
		path, err := acceptanceExecutablePath(environment, tool)
		if err != nil {
			t.Errorf("isolated active toolchain does not provide %s: %v", tool, err)
			continue
		}
		if !filepath.IsAbs(path) {
			t.Errorf("%s resolved to non-absolute path %q", tool, path)
		}
	}
}

func TestAcceptanceGoToolchainValidationRejectsWrongVersion(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	goPath := filepath.Join(bin, "go")
	platform := runtime.GOOS + "/" + runtime.GOARCH
	if err := os.WriteFile(goPath, []byte("#!/bin/sh\nprintf 'go version go1.26.4 "+platform+"\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := &freshCloneRun{root: filepath.Join(root, "checkout"), environment: []string{"PATH=" + bin}}
	err := run.validateGoToolchain(context.Background())
	if err == nil || !strings.Contains(err.Error(), "want stdout=\"go version go"+pinnedGoVersion+" "+platform+"\"") {
		t.Fatalf("wrong Go toolchain version was accepted: %v", err)
	}
	coldDiagnostic := "go: downloading go" + pinnedGoVersion + " (" + platform + ")"
	if err := os.WriteFile(goPath, []byte("#!/bin/sh\nprintf '"+coldDiagnostic+"\\n' >&2\nprintf 'go version go"+pinnedGoVersion+" "+platform+"\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run.validateGoToolchain(context.Background()); err != nil {
		t.Fatalf("cold pinned Go toolchain was rejected: %v", err)
	}
}

func TestAcceptanceEnvironmentPinsTaskHomeAndConfiguration(t *testing.T) {
	scratch := t.TempDir()
	hostHome := t.TempDir()
	environment := isolatedEnvironmentFrom(
		[]string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + hostHome,
			"DOCKER_CONFIG=" + filepath.Join(hostHome, ".docker"),
			"XDG_CONFIG_HOME=" + filepath.Join(hostHome, ".config"),
			"COREPACK_HOME=" + filepath.Join(hostHome, ".cache", "corepack"),
			"npm_config_userconfig=" + filepath.Join(hostHome, ".npmrc"),
		},
		scratch,
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	wants := map[string]string{
		"HOME":                  filepath.Join(scratch, "home"),
		"TMPDIR":                filepath.Join(scratch, "tmp"),
		"DOCKER_CONFIG":         filepath.Join(scratch, "docker-config"),
		"XDG_CONFIG_HOME":       filepath.Join(scratch, "config"),
		"XDG_CACHE_HOME":        filepath.Join(scratch, "cache"),
		"XDG_DATA_HOME":         filepath.Join(scratch, "data"),
		"XDG_STATE_HOME":        filepath.Join(scratch, "state"),
		"COREPACK_HOME":         filepath.Join(scratch, "corepack"),
		"npm_config_userconfig": filepath.Join(scratch, "npmrc"),
		"GIT_CONFIG_NOSYSTEM":   "1",
	}
	for name, want := range wants {
		values := environmentValues(environment, name)
		if len(values) != 1 || values[0] != want {
			t.Errorf("%s values = %v, want exactly %q", name, values, want)
		}
	}
	for _, value := range environment {
		if strings.Contains(value, hostHome) {
			t.Errorf("isolated environment retained host home path: %q", value)
		}
	}
}

func TestAcceptancePnpmIgnoresHostScriptShell(t *testing.T) {
	hostConfig := t.TempDir()
	fakeShell := filepath.Join(hostConfig, "fake-shell")
	marker := filepath.Join(hostConfig, "fake-shell-ran")
	if err := os.WriteFile(fakeShell, []byte("#!/bin/sh\n: > \"$ACCEPTANCE_FAKE_SCRIPT_SHELL_MARKER\"\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pnpmConfig := filepath.Join(hostConfig, "pnpm")
	if err := os.MkdirAll(pnpmConfig, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pnpmConfig, "config.yaml"), []byte("scriptShell: "+fakeShell+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ambient := exec.Command("pnpm", "--filter", "@semlia/web", "test")
	ambient.Dir = repositoryRoot
	ambient.Env = append(
		filteredEnvironment(os.Environ(), "XDG_CONFIG_HOME", "ACCEPTANCE_FAKE_SCRIPT_SHELL_MARKER"),
		"XDG_CONFIG_HOME="+hostConfig,
		"ACCEPTANCE_FAKE_SCRIPT_SHELL_MARKER="+marker,
	)
	if output, err := ambient.CombinedOutput(); err != nil {
		t.Fatalf("test precondition did not reproduce host scriptShell fake green: %v\n%s", err, output)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("test precondition did not execute the poisoned host scriptShell: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	environment := isolatedEnvironmentFrom(
		ambient.Env,
		t.TempDir(),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	command, err := acceptanceCommandContext(context.Background(), environment, "pnpm", "--filter", "@semlia/web", "test")
	if err != nil {
		t.Fatal(err)
	}
	command.Dir = repositoryRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated pnpm did not execute the real Semlia frontend tests: err=%v output=%s", err, output)
	}
	for _, evidence := range []string{"Test Files", "Tests", "passed"} {
		if !strings.Contains(string(output), evidence) {
			t.Errorf("real Semlia frontend test output missing %q:\n%s", evidence, output)
		}
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("isolated pnpm reused poisoned host scriptShell: %v", err)
	}
}

func TestAcceptanceGitIgnoresHostGlobalConfiguration(t *testing.T) {
	hostHome := t.TempDir()
	hooks := filepath.Join(hostHome, "hooks")
	if err := os.MkdirAll(hooks, 0o700); err != nil {
		t.Fatal(err)
	}
	config := "[core]\n\thooksPath = " + hooks + "\n\tignoreStat = true\n"
	if err := os.WriteFile(filepath.Join(hostHome, ".gitconfig"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := isolatedEnvironmentFrom(
		append(filteredEnvironment(os.Environ(), "HOME"), "HOME="+hostHome),
		t.TempDir(),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	for _, key := range []string{"core.hooksPath", "core.ignoreStat"} {
		command, err := acceptanceCommandContext(context.Background(), environment, "git", "config", "--global", "--get", key)
		if err != nil {
			t.Fatal(err)
		}
		if output, err := command.CombinedOutput(); err == nil || strings.TrimSpace(string(output)) != "" {
			t.Fatalf("isolated git inherited host global %s: err=%v output=%q", key, err, output)
		}
	}
}

func TestAcceptanceGitCloneDoesNotPropagateAssumeUnchanged(t *testing.T) {
	scratch := t.TempDir()
	source := filepath.Join(scratch, "source")
	destination := filepath.Join(scratch, "destination")
	environment := isolatedEnvironmentFrom(
		os.Environ(),
		filepath.Join(scratch, "environment"),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	if err := prepareIsolatedEnvironmentDirectories(environment); err != nil {
		t.Fatal(err)
	}
	runGit := func(dir string, args ...string) string {
		t.Helper()
		command, err := acceptanceCommandContext(context.Background(), environment, "git", args...)
		if err != nil {
			t.Fatal(err)
		}
		command.Dir = dir
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
		}
		return strings.TrimSpace(string(output))
	}
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(source, "init", "--quiet")
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("tracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(source, "add", "tracked.txt")
	runGit(source, "-c", "user.name=Semlia Acceptance", "-c", "user.email=acceptance@invalid", "commit", "--quiet", "-m", "initial")
	runGit(source, "update-index", "--assume-unchanged", "tracked.txt")
	if marker := runGit(source, "ls-files", "-v", "tracked.txt"); marker == "" || marker[0] < 'a' || marker[0] > 'z' {
		t.Fatalf("test precondition did not mark source index assume-unchanged: %q", marker)
	}
	runGit(scratch, "clone", "--quiet", "--no-checkout", "--", source, destination)
	runGit(destination, "checkout", "--quiet", "--detach", "HEAD")
	if marker := runGit(destination, "ls-files", "-v", "tracked.txt"); marker == "" || marker[0] < 'A' || marker[0] > 'Z' {
		t.Fatalf("fresh acceptance clone propagated assume-unchanged state: %q", marker)
	}
}

func TestAcceptanceGitIgnoresHostCheckoutHookThatHidesChanges(t *testing.T) {
	scratch := t.TempDir()
	source := filepath.Join(scratch, "source")
	hostHome := filepath.Join(scratch, "host-home")
	hooks := filepath.Join(hostHome, "hooks")
	for _, directory := range []string{source, hooks} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "Makefile"), []byte("verify:\n\t@echo real\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hook := `#!/bin/sh
printf 'verify:
	@echo fake
' > Makefile
git update-index --assume-unchanged Makefile
`
	if err := os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostHome, ".gitconfig"), []byte("[core]\n\thooksPath = "+hooks+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	setupEnvironment := isolatedEnvironmentFrom(
		os.Environ(),
		filepath.Join(scratch, "setup-environment"),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	if err := prepareIsolatedEnvironmentDirectories(setupEnvironment); err != nil {
		t.Fatal(err)
	}
	run := func(environment []string, directory, name string, args ...string) string {
		t.Helper()
		command, err := acceptanceCommandContext(context.Background(), environment, name, args...)
		if err != nil {
			t.Fatal(err)
		}
		command.Dir = directory
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
		}
		return strings.TrimSpace(string(output))
	}
	run(setupEnvironment, source, "git", "init", "--quiet")
	run(setupEnvironment, source, "git", "add", "Makefile")
	run(setupEnvironment, source, "git", "-c", "user.name=Semlia Acceptance", "-c", "user.email=acceptance@invalid", "commit", "--quiet", "-m", "initial")
	ref := run(setupEnvironment, source, "git", "rev-parse", "HEAD")

	ambientEnvironment := append(
		filteredEnvironment(os.Environ(), "HOME", "XDG_CONFIG_HOME", "GIT_CONFIG_NOSYSTEM"),
		"HOME="+hostHome,
		"XDG_CONFIG_HOME="+filepath.Join(hostHome, ".config"),
		"GIT_CONFIG_NOSYSTEM=1",
	)
	cloneAndCheckout := func(environment []string, destination string) {
		t.Helper()
		run(environment, scratch, "git", "clone", "--quiet", "--no-checkout", "--", source, destination)
		run(environment, destination, "git", "checkout", "--quiet", "--detach", ref)
	}
	ambientCheckout := filepath.Join(scratch, "ambient-checkout")
	cloneAndCheckout(ambientEnvironment, ambientCheckout)
	if got := run(ambientEnvironment, ambientCheckout, "make", "--no-print-directory", "verify"); got != "fake" {
		t.Fatalf("test precondition did not reproduce hidden host-hook mutation: %q", got)
	}
	if status := run(ambientEnvironment, ambientCheckout, "git", "status", "--short"); status != "" {
		t.Fatalf("test precondition hook did not hide its mutation: %s", status)
	}
	if marker := run(ambientEnvironment, ambientCheckout, "git", "ls-files", "-v", "Makefile"); marker == "" || marker[0] < 'a' || marker[0] > 'z' {
		t.Fatalf("test precondition hook did not set assume-unchanged: %q", marker)
	}

	isolatedEnvironment := isolatedEnvironmentFrom(
		ambientEnvironment,
		filepath.Join(scratch, "isolated-environment"),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	if err := prepareIsolatedEnvironmentDirectories(isolatedEnvironment); err != nil {
		t.Fatal(err)
	}
	isolatedCheckout := filepath.Join(scratch, "isolated-checkout")
	cloneAndCheckout(isolatedEnvironment, isolatedCheckout)
	if got := run(isolatedEnvironment, isolatedCheckout, "make", "--no-print-directory", "verify"); got != "real" {
		t.Fatalf("isolated checkout executed host hook or replaced Makefile: %q", got)
	}
	if status := run(isolatedEnvironment, isolatedCheckout, "git", "status", "--short"); status != "" {
		t.Fatalf("isolated checkout is unexpectedly dirty: %s", status)
	}
	if marker := run(isolatedEnvironment, isolatedCheckout, "git", "ls-files", "-v", "Makefile"); marker == "" || marker[0] < 'A' || marker[0] > 'Z' {
		t.Fatalf("isolated checkout inherited assume-unchanged state: %q", marker)
	}
}

func TestAcceptanceEnvironmentPinsTestcontainersSessionAndRyuk(t *testing.T) {
	scratch := t.TempDir()
	project := "semlia-accept-a1b2c3d4-000000000001"
	hostHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(hostHome, ".testcontainers.properties"), []byte("session.id=attacker\nryuk.disabled=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := isolatedEnvironmentFrom(
		[]string{
			"PATH=" + os.Getenv("PATH"), "HOME=" + hostHome,
			"TESTCONTAINERS_SESSION_ID=attacker", "TESTCONTAINERS_RYUK_DISABLED=true",
			"TESTCONTAINERS_RYUK_CONTAINER_PRIVILEGED=true", "RYUK_CONNECTION_TIMEOUT=1ns",
			"RYUK_RECONNECTION_TIMEOUT=1ns", "RYUK_VERBOSE=true",
		},
		scratch,
		project,
		38080,
		35432,
	)
	wants := map[string]string{
		"TESTCONTAINERS_SESSION_ID":                project,
		"TESTCONTAINERS_RYUK_DISABLED":             "false",
		"TESTCONTAINERS_RYUK_CONTAINER_PRIVILEGED": "false",
		"RYUK_CONNECTION_TIMEOUT":                  "1m",
		"RYUK_RECONNECTION_TIMEOUT":                "10s",
		"RYUK_VERBOSE":                             "false",
	}
	for name, want := range wants {
		values := environmentValues(environment, name)
		if len(values) != 1 || values[0] != want {
			t.Errorf("%s values = %v, want exactly %q", name, values, want)
		}
	}
	if got := environmentValue(environment, "HOME"); strings.HasPrefix(got, hostHome) {
		t.Fatalf("testcontainers can still read host properties through HOME=%q", got)
	}
}

func TestAcceptanceCommandResolutionFailsClosed(t *testing.T) {
	relative := filepath.Join("relative", "bin")
	if path, err := acceptanceExecutablePath([]string{"PATH=" + relative}, "go"); err == nil {
		t.Fatalf("relative isolated PATH resolved executable %q", path)
	}
	if path, err := acceptanceExecutablePath([]string{"PATH=" + t.TempDir()}, "definitely-not-a-semlia-tool"); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("missing executable resolution = %q, %v; want exec.ErrNotFound", path, err)
	}
	if path, err := acceptanceExecutablePath([]string{"PATH=" + trustedAcceptancePath()}, filepath.Join("relative", "tool")); err == nil {
		t.Fatalf("relative explicit command resolved to %q", path)
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
	command, resolveErr := acceptanceCommandContext(context.Background(), environment, "pnpm", "store", "path")
	if resolveErr != nil {
		t.Fatalf("resolve isolated pnpm: %v", resolveErr)
	}
	command.Stdin = strings.NewReader("y\n")
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
	command, resolveErr := acceptanceCommandContext(context.Background(), environment, "make", "--no-print-directory", "verify")
	if resolveErr != nil {
		t.Fatalf("resolve isolated make: %v", resolveErr)
	}
	command.Dir = project
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

func TestFreshCloneLauncherRejectsInvalidInputsThroughPoisonedPathAndFunctions(t *testing.T) {
	bin := t.TempDir()
	functionPoisonMarker := filepath.Join(t.TempDir(), "imported-shell-function-ran")
	for _, tool := range []string{"dirname", "env", "go"} {
		stub := "#!/bin/sh\nexit 0\n"
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name   string
		source string
		ref    string
		want   string
	}{
		{name: "invalid source", source: "", ref: "not-a-commit", want: "SEMLIA_ACCEPTANCE_SOURCE"},
		{name: "invalid ref", source: repositoryRoot, ref: "not-a-commit", want: "SEMLIA_ACCEPTANCE_REF must be an exact 40-character lowercase commit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(filepath.Join(repositoryRoot, "scripts", "acceptance", "m0-fresh-clone.sh"))
			command.Env = []string{
				"PATH=" + bin,
				"HOME=" + os.Getenv("HOME"),
				"BASH_FUNC_cd%%=() { return 0; }",
				"BASH_FUNC_pwd%%=() { printf '/tmp\\n'; return 0; }",
				"BASH_FUNC_dirname%%=() { printf '/tmp\\n'; return 0; }",
				"BASH_FUNC_env%%=() { return 0; }",
				"BASH_FUNC_go%%=() { return 0; }",
				"BASH_FUNC_exit%%=() { /usr/bin/touch '" + functionPoisonMarker + "'; return 0; }",
				"BASH_FUNC_printf%%=() { /usr/bin/touch '" + functionPoisonMarker + "'; return 0; }",
				"BASH_FUNC_trap%%=() { /usr/bin/touch '" + functionPoisonMarker + "'; return 0; }",
				"BASH_FUNC_wait%%=() { /usr/bin/touch '" + functionPoisonMarker + "'; return 0; }",
				"BASH_FUNC_set%%=() { /usr/bin/touch '" + functionPoisonMarker + "'; return 0; }",
				"SEMLIA_ACCEPTANCE_SOURCE=" + test.source,
				"SEMLIA_ACCEPTANCE_REF=" + test.ref,
			}
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("canonical launcher falsely accepted invalid source/ref through poisoned commands: %s", output)
			}
			if !strings.Contains(string(output), test.want) {
				t.Fatalf("launcher did not reach the real Go invocation validator: %v\n%s", err, output)
			}
			if _, err := os.Stat(functionPoisonMarker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("launcher imported a poisoned Bash function: %v", err)
			}
		})
	}
}

func TestAcceptanceCommandsResolveAgainstProvidedEnvironmentPath(t *testing.T) {
	ambientBin := t.TempDir()
	isolatedBin := t.TempDir()
	t.Setenv("PATH", ambientBin)

	for _, tool := range []string{"make", "go", "docker", "pnpm", "git"} {
		tool := tool
		t.Run(tool, func(t *testing.T) {
			ambientStub := "#!/bin/sh\nprintf 'ambient-" + tool + "\\n'\nexit 0\n"
			isolatedStub := "#!/bin/sh\nprintf 'isolated-" + tool + "\\n'\nexit 41\n"
			if err := os.WriteFile(filepath.Join(ambientBin, tool), []byte(ambientStub), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(isolatedBin, tool), []byte(isolatedStub), 0o755); err != nil {
				t.Fatal(err)
			}

			output, err := executeSubprocess(
				context.Background(),
				t.TempDir(),
				[]string{"PATH=" + isolatedBin},
				tool,
			)
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) || exitError.ExitCode() != 41 {
				t.Fatalf("%s did not execute the failing isolated binary: err=%v output=%q", tool, err, output)
			}
			if got := strings.TrimSpace(string(output)); got != "isolated-"+tool {
				t.Fatalf("%s output = %q, want isolated-path marker", tool, got)
			}
		})
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
		2*time.Second,
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
	const resetGoTestPath = `-exec="/usr/bin/env PATH=${TRUSTED_PATH}"`
	if strings.Count(launcher, resetGoTestPath) != 1 {
		t.Fatalf("canonical launcher must reset the Go test PATH exactly once with %q", resetGoTestPath)
	}
	if !strings.Contains(launcher, "TRUSTED_PATH=${TRUSTED_PATH}:"+trustedAcceptancePath()) {
		t.Fatal("canonical launcher and child acceptance environment use different trusted tool paths")
	}
	if strings.Contains(launcher, "hostedtoolcache/*/bin") {
		t.Fatal("canonical launcher trusts a broad hosted-toolcache wildcard")
	}
	if strings.Contains(launcher, `for DIRECTORY in ${PATH:-}`) {
		t.Fatal("canonical launcher derives its Go candidates from the ambient PATH")
	}
	for _, fragment := range []string{
		`/bin/bash --noprofile --norc -p`,
		"PINNED_GO_VERSION=1.26.5",
		"PINNED_GO_BOOTSTRAP_VERSION=1.26.3",
		`Linux:x86_64)`,
		`EXPECTED_GO_CANDIDATES=/opt/hostedtoolcache/go/1.26.5/x64/bin/go:/usr/local/go/bin/go`,
		`Darwin:arm64)`,
		`EXPECTED_GO_CANDIDATES=/Users/runner/hostedtoolcache/go/1.26.5/arm64/bin/go:/opt/homebrew/bin/go`,
		`for CANDIDATE in ${EXPECTED_GO_CANDIDATES}; do`,
		`GOTOOLCHAIN=local`,
		`"${CANDIDATE}" version`,
		`GO_BIN=${CANDIDATE}`,
		`"${GO_BIN}" -C "${ROOT}" test`,
		`set -m`,
		`CHILD_PID=$!`,
		`/bin/kill -"${LAUNCHER_SIGNAL_NAME}" -- "-${CHILD_PID}"`,
		`if ! /bin/kill -0 -- "-${CHILD_PID}"`,
		`wait_for_child`,
		`"GOTOOLCHAIN=go${PINNED_GO_VERSION}"`,
		"/opt/hostedtoolcache/go/1.26.5/x64/bin",
		"/Users/runner/hostedtoolcache/go/1.26.5/arm64/bin",
		`"HOME=${LAUNCHER_ROOT}/home"`,
		`"TMPDIR=${LAUNCHER_ROOT}/tmp"`,
		`"DOCKER_CONFIG=${LAUNCHER_ROOT}/docker-config"`,
		`"XDG_CONFIG_HOME=${LAUNCHER_ROOT}/config"`,
		`"GOMODCACHE=${LAUNCHER_ROOT}/go-mod"`,
	} {
		if !strings.Contains(launcher, fragment) {
			t.Errorf("canonical launcher missing isolation/version fragment %q", fragment)
		}
	}
	if strings.Contains(launcher, `trap 'exit 130' HUP INT TERM`) {
		t.Fatal("canonical launcher exits from its signal trap without supervising the Go test process group")
	}
	for _, forbidden := range []string{"SEMLIA_ACCEPTANCE_TEST_PATTERN", "SEMLIA_ACCEPTANCE_TEST_PACKAGES", "SEMLIA_LAUNCHER_ROOT_RECORD"} {
		if strings.Contains(launcher, forbidden) {
			t.Fatalf("canonical launcher exposes test-only override %s", forbidden)
		}
	}
	if !strings.Contains(launcher, `if ! cleanup_launcher; then`) || !strings.Contains(launcher, `if [ "${STATUS}" -eq 0 ]; then`) {
		t.Fatal("canonical launcher can report child success after launcher-root cleanup fails")
	}
	if !strings.Contains(readRepositoryFile(t, "tests/acceptance/fresh_clone_test.go"), `"test.timeout": outerAcceptanceTimeout.String()`) {
		t.Fatal("fresh-clone invocation validator does not enforce the canonical outer timeout")
	}
	resourceKinds := (&freshCloneRun{
		project: "semlia-accept-a1b2c3d4-000000000001",
		environment: []string{
			"SEMLIA_DOCKER_RESOURCE_LABEL=" + acceptanceDockerLabelKey + "=semlia-accept-a1b2c3d4-000000000001",
			"TESTCONTAINERS_SESSION_ID=semlia-accept-a1b2c3d4-000000000001",
		},
	}).cleanupResourceKinds()
	cleanupEnvelope := dockerTeardownTimeout +
		2*time.Duration(len(resourceKinds))*dockerInspectionTimeout + // Initial inspection and final exact relist.
		goModCacheCleanupTimeout + checkoutCleanupTimeout +
		time.Duration(len(resourceKinds))*(dockerInspectionTimeout+dockerResourceRemovalTimeout+dockerInspectionTimeout) // Pre-cache list, removal, and failed-removal relist.
	maximumCleanupSubprocessCount := 3 + 5*len(resourceKinds) // Compose down, two cache cleanups, and five per-kind list/remove phases.
	for _, resource := range resourceKinds {
		cleanupEnvelope += time.Duration(resource.maxIdentities) * (dockerIdentityTimeout + dockerInspectionTimeout)
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

func TestPrepareDockerCLIUsesValidatedTaskLocalComposePlugin(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	dockerConfig := filepath.Join(root, "docker-config")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	docker := filepath.Join(bin, "docker")
	dockerScript := `#!/bin/sh
if [ "$1" = version ]; then
  exit 0
fi
if [ "$1" = compose ] && [ "$2" = version ]; then
  plugin="$DOCKER_CONFIG/cli-plugins/docker-compose"
  [ -x "$plugin" ] || exit 42
  exec "$plugin" version
fi
exit 64
`
	if err := os.WriteFile(docker, []byte(dockerScript), 0o755); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(bin, "docker-compose")
	pluginScript := `#!/bin/sh
if [ "$1" = docker-cli-plugin-metadata ]; then
  printf '%s\n' '{"SchemaVersion":"0.1.0","Vendor":"Docker Inc.","Version":"v2.0.0","ShortDescription":"Docker Compose"}'
  exit 0
fi
if [ "$1" = version ]; then
  printf '%s\n' 'Docker Compose version v2.0.0'
  exit 0
fi
exit 64
`
	if err := os.WriteFile(plugin, []byte(pluginScript), 0o755); err != nil {
		t.Fatal(err)
	}
	run := &freshCloneRun{
		t:    t,
		root: root,
		environment: []string{
			"PATH=" + bin,
			"HOME=" + filepath.Join(root, "home"),
			"DOCKER_CONFIG=" + dockerConfig,
		},
	}
	if err := run.prepareDockerCLI(context.Background()); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(dockerConfig, "cli-plugins", "docker-compose")
	info, err := os.Lstat(installed)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("task-local Compose capability was copied instead of symlinked: mode=%s", info.Mode())
	}
	target, err := os.Readlink(installed)
	if err != nil {
		t.Fatal(err)
	}
	if target != plugin {
		t.Fatalf("task-local Compose plugin target = %q, want validated %q", target, plugin)
	}
}

func TestPrepareDockerCLIRejectsUnvalidatedComposePlugin(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	dockerScript := "#!/bin/sh\n[ \"$1\" = version ] && exit 0\nexit 42\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(dockerScript), 0o755); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(bin, "docker-compose")
	if err := os.WriteFile(plugin, []byte("#!/bin/sh\nprintf 'not-json\\n'\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dockerConfig := filepath.Join(root, "docker-config")
	run := &freshCloneRun{
		t:    t,
		root: root,
		environment: []string{
			"PATH=" + bin,
			"HOME=" + filepath.Join(root, "home"),
			"DOCKER_CONFIG=" + dockerConfig,
		},
	}
	err := run.prepareDockerCLI(context.Background())
	if err == nil || !strings.Contains(err.Error(), "validate Docker Compose plugin metadata") {
		t.Fatalf("unvalidated Compose plugin was accepted: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(dockerConfig, "cli-plugins", "docker-compose")); !os.IsNotExist(statErr) {
		t.Fatalf("unvalidated Compose plugin was installed: %v", statErr)
	}
}

func TestAcceptanceDockerCLIIgnoresHostHomePlugin(t *testing.T) {
	hostHome := t.TempDir()
	pluginDirectory := filepath.Join(hostHome, ".docker", "cli-plugins")
	if err := os.MkdirAll(pluginDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(hostHome, "host-plugin-ran")
	plugin := `#!/bin/sh
if [ "$1" = docker-cli-plugin-metadata ]; then
  printf '%s\n' '{"SchemaVersion":"0.1.0","Vendor":"Host Poison","Version":"v0.0.0","ShortDescription":"Docker Compose"}'
  exit 0
fi
: > "$ACCEPTANCE_HOST_DOCKER_PLUGIN_MARKER"
printf '%s\n' 'host Compose plugin'
`
	if err := os.WriteFile(filepath.Join(pluginDirectory, "docker-compose"), []byte(plugin), 0o755); err != nil {
		t.Fatal(err)
	}
	ambientEnvironment := append(
		filterAcceptanceEnvironment(os.Environ()),
		"PATH="+trustedAcceptancePathFrom(os.Environ()),
		"HOME="+hostHome,
		"ACCEPTANCE_HOST_DOCKER_PLUGIN_MARKER="+marker,
	)
	ambient, err := acceptanceCommandContext(context.Background(), ambientEnvironment, "docker", "compose", "version")
	if err != nil {
		t.Fatal(err)
	}
	if output, err := ambient.CombinedOutput(); err != nil {
		t.Fatalf("test precondition did not execute the poisoned host Docker plugin: %v\n%s", err, output)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("test precondition did not execute the poisoned host Docker plugin: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	isolationRoot := t.TempDir()
	isolatedEnvironment := isolatedEnvironmentFrom(
		ambientEnvironment,
		isolationRoot,
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	if err := prepareIsolatedEnvironmentDirectories(isolatedEnvironment); err != nil {
		t.Fatal(err)
	}
	isolated, err := acceptanceCommandContext(context.Background(), isolatedEnvironment, "docker", "compose", "version")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = isolated.CombinedOutput()
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("isolated Docker CLI executed the host HOME plugin: %v", err)
	}
	if got := environmentValue(isolatedEnvironment, "DOCKER_CONFIG"); got != filepath.Join(isolationRoot, "docker-config") {
		t.Fatalf("isolated Docker configuration = %q", got)
	}
}

func TestDockerCLIIsolationProbe(t *testing.T) {
	if os.Getenv("SEMLIA_RUN_DOCKER_CLI_ISOLATION_PROBE") != "1" {
		t.Skip("set SEMLIA_RUN_DOCKER_CLI_ISOLATION_PROBE=1 to validate the active Docker daemon with task-local CLI configuration")
	}
	scratch := t.TempDir()
	environment := isolatedEnvironmentFrom(
		os.Environ(),
		scratch,
		"semlia-docker-cli-probe-"+randomRunID(t),
		38080,
		35432,
	)
	if err := prepareIsolatedEnvironmentDirectories(environment); err != nil {
		t.Fatal(err)
	}
	run := &freshCloneRun{t: t, root: repositoryRoot, environment: environment}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := run.prepareDockerCLI(ctx); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{{"version"}, {"compose", "version"}, {"info", "--format", "{{.Name}}"}} {
		output, err := executeSubprocess(ctx, repositoryRoot, environment, "docker", command...)
		if err != nil || strings.TrimSpace(string(output)) == "" {
			t.Errorf("isolated docker %s: err=%v output=%q", strings.Join(command, " "), err, output)
		}
	}
	if got := environmentValue(environment, "DOCKER_CONFIG"); got == "" || !strings.HasPrefix(got, scratch+string(os.PathSeparator)) {
		t.Fatalf("Docker CLI configuration is not task-local: %q", got)
	}
}

func TestFreshCloneInvocationRejectsNoncanonicalOuterTimeout(t *testing.T) {
	launcherRoot := t.TempDir()
	writeTestCABundle(t, launcherRoot, testCertificatePEM(t), 0o600)
	command := exec.Command(
		os.Args[0],
		"-test.run=^TestFreshCloneAcceptance$",
		"-test.count=1",
		"-test.timeout=99m",
	)
	command.Env = append(
		filterAcceptanceEnvironment(os.Environ()),
		"PATH="+canonicalLauncherTrustedPath(),
		"HOME="+filepath.Join(launcherRoot, "home"),
		"TMPDIR="+filepath.Join(launcherRoot, "tmp"),
		"DOCKER_CONFIG="+filepath.Join(launcherRoot, "docker-config"),
		"XDG_CONFIG_HOME="+filepath.Join(launcherRoot, "config"),
		"XDG_CACHE_HOME="+filepath.Join(launcherRoot, "cache"),
		"XDG_DATA_HOME="+filepath.Join(launcherRoot, "data"),
		"XDG_STATE_HOME="+filepath.Join(launcherRoot, "state"),
		"COREPACK_HOME="+filepath.Join(launcherRoot, "corepack"),
		"NODE_EXTRA_CA_CERTS="+filepath.Join(launcherRoot, "system-ca.pem"),
		"NODE_USE_SYSTEM_CA=1",
		"npm_config_userconfig="+filepath.Join(launcherRoot, "npmrc"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GOENV=off",
		"GOFLAGS=",
		"GOTOOLCHAIN=go"+pinnedGoVersion,
		"GOWORK=off",
		"GOCACHE="+filepath.Join(launcherRoot, "go-build"),
		"GOPATH="+filepath.Join(launcherRoot, "go-path"),
		"GOMODCACHE="+filepath.Join(launcherRoot, "go-mod"),
		"SEMLIA_RUN_FRESH_CLONE=1",
		"SEMLIA_ACCEPTANCE_LAUNCHER_ROOT="+launcherRoot,
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
				if len(args) > 1 && args[1] == "inspect" {
					return json.Marshal(map[string]string{"com.docker.compose.project": project})
				}
				if len(args) > 2 && args[1] == "rm" && args[2] == "task-network" {
					networkExists = false
				}
			case "volume":
				if len(args) > 1 && args[1] == "ls" && volumeExists {
					return []byte("task-volume\n"), nil
				}
				if len(args) > 1 && args[1] == "inspect" {
					return json.Marshal(map[string]string{"com.docker.compose.project": project})
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
	for _, sequence := range []struct {
		inspect string
		remove  string
	}{
		{"docker network inspect --format {{json .Labels}} task-network", "docker network rm task-network"},
		{"docker volume inspect --format {{json .Labels}} task-volume", "docker volume rm --force task-volume"},
	} {
		inspectIndex := commandIndex(calls, sequence.inspect)
		removeIndex := commandIndex(calls, sequence.remove)
		if inspectIndex < 0 || removeIndex <= inspectIndex {
			t.Errorf("Compose resource identity was not inspected before exact removal: inspect=%q remove=%q\n%s", sequence.inspect, sequence.remove, strings.Join(calls, "\n"))
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
			case strings.HasPrefix(joined, "network inspect"):
				return json.Marshal(map[string]string{"com.docker.compose.project": project})
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
			case name == "docker" && (strings.HasPrefix(joined, "inspect --format") || strings.HasPrefix(joined, "network inspect --format") || strings.HasPrefix(joined, "volume inspect --format")):
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
			case strings.HasPrefix(joined, "volume inspect --format"):
				return json.Marshal(map[string]string{"com.docker.compose.project": "semlia-accept-a1b2c3d4-000000000001"})
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

func TestCleanupOwnsExactTestcontainersSessionResources(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	sessionLabel := testcontainersSessionLabelKey + "=" + project
	run := &freshCloneRun{
		project: project,
		environment: []string{
			"SEMLIA_DOCKER_RESOURCE_LABEL=" + acceptanceDockerLabelKey + "=" + project,
			"TESTCONTAINERS_SESSION_ID=" + project,
		},
	}
	resources := run.cleanupResourceKinds()
	wants := map[string]struct {
		command       string
		inspectFormat string
		maximum       int
	}{
		"testcontainers containers": {"ps --all --filter label=" + sessionLabel, "{{json .Config.Labels}}", testcontainersContainerLimit},
		"testcontainers networks":   {"network ls --filter label=" + sessionLabel, "{{json .Labels}}", testcontainersNetworkLimit},
		"testcontainers volumes":    {"volume ls --filter label=" + sessionLabel, "{{json .Labels}}", testcontainersVolumeLimit},
	}
	for _, resource := range resources {
		want, ok := wants[resource.label]
		if !ok {
			continue
		}
		if got := strings.Join(resource.listArgs, " "); !strings.HasPrefix(got, want.command) {
			t.Errorf("%s list command = %q, want prefix %q", resource.label, got, want.command)
		}
		if resource.identityLabel != sessionLabel || resource.inspectFormat != want.inspectFormat || resource.maxIdentities != want.maximum {
			t.Errorf("%s ownership = label %q format %q maximum %d", resource.label, resource.identityLabel, resource.inspectFormat, resource.maxIdentities)
		}
		if resource.label == "testcontainers containers" && strings.Join(resource.removeArgs, " ") != "rm --force --volumes" {
			t.Errorf("Testcontainers container removal = %q, want anonymous-volume cleanup", strings.Join(resource.removeArgs, " "))
		}
		delete(wants, resource.label)
	}
	if len(wants) != 0 {
		t.Fatalf("cleanup omitted Testcontainers session resources: %v", wants)
	}
}

func TestCleanupOwnsExactComposeResources(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	composeLabel := "com.docker.compose.project=" + project
	resources := (&freshCloneRun{project: project}).cleanupResourceKinds()
	wants := map[string]struct {
		inspectFormat string
		maximum       int
	}{
		"containers": {"{{json .Config.Labels}}", composeServiceContainerLimit},
		"networks":   {"{{json .Labels}}", composeNetworkLimit},
		"volumes":    {"{{json .Labels}}", composeVolumeLimit},
	}
	for _, resource := range resources {
		want, ok := wants[resource.label]
		if !ok {
			continue
		}
		if resource.identityLabel != composeLabel || resource.inspectFormat != want.inspectFormat || resource.maxIdentities != want.maximum {
			t.Errorf("%s ownership = label %q format %q maximum %d", resource.label, resource.identityLabel, resource.inspectFormat, resource.maxIdentities)
		}
		delete(wants, resource.label)
	}
	if len(wants) != 0 {
		t.Fatalf("cleanup omitted Compose resources: %v", wants)
	}
}

func TestCleanupRefusesComposeResourceWhoseIdentityChanges(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	composeLabel := "com.docker.compose.project=" + project
	for _, test := range []struct {
		name       string
		kind       string
		identifier string
		label      string
	}{
		{name: "network", kind: "network", identifier: "task-network", label: "networks"},
		{name: "volume", kind: "volume", identifier: "task-volume", label: "volumes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := &freshCloneRun{
				root:    t.TempDir(),
				project: project,
				cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
					if name != "docker" || len(args) < 2 || args[0] != test.kind {
						return nil, nil
					}
					switch {
					case args[1] == "ls" && argumentAfter(args, "--filter") == "label="+composeLabel:
						return []byte(test.identifier + "\n"), nil
					case args[1] == "inspect":
						return json.Marshal(map[string]string{"com.docker.compose.project": project + "-unrelated"})
					case args[1] == "rm":
						t.Fatalf("cleanup removed a Compose %s after its project identity changed", test.name)
					}
					return nil, nil
				},
			}
			err := run.removeRemainingTaskDockerResources()
			if err == nil || !strings.Contains(err.Error(), "refuse to remove "+test.label+" "+test.identifier) {
				t.Fatalf("changed Compose %s identity was not rejected: %v", test.name, err)
			}
		})
	}
}

func TestCleanupInspectsNetworkAndVolumeIdentityBeforeRemoval(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	sessionLabel := testcontainersSessionLabelKey + "=" + project
	networkExists := true
	volumeExists := true
	var calls []string
	run := &freshCloneRun{
		root:    t.TempDir(),
		project: project,
		environment: []string{
			"TESTCONTAINERS_SESSION_ID=" + project,
		},
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			joined := name + " " + strings.Join(args, " ")
			calls = append(calls, joined)
			if name != "docker" || len(args) == 0 {
				return nil, nil
			}
			switch {
			case args[0] == "network" && len(args) > 1 && args[1] == "ls" && argumentAfter(args, "--filter") == "label="+sessionLabel && networkExists:
				return []byte("session-network\n"), nil
			case args[0] == "volume" && len(args) > 1 && args[1] == "ls" && argumentAfter(args, "--filter") == "label="+sessionLabel && volumeExists:
				return []byte("session-volume\n"), nil
			case args[0] == "network" && len(args) > 1 && args[1] == "inspect":
				return json.Marshal(map[string]string{testcontainersSessionLabelKey: project})
			case args[0] == "volume" && len(args) > 1 && args[1] == "inspect":
				return json.Marshal(map[string]string{testcontainersSessionLabelKey: project})
			case args[0] == "network" && len(args) > 2 && args[1] == "rm":
				networkExists = false
			case args[0] == "volume" && len(args) > 2 && args[1] == "rm":
				volumeExists = false
			}
			return nil, nil
		},
	}
	if err := run.removeRemainingTaskDockerResources(); err != nil {
		t.Fatal(err)
	}
	for _, sequence := range []struct {
		inspect string
		remove  string
	}{
		{"docker network inspect --format {{json .Labels}} session-network", "docker network rm session-network"},
		{"docker volume inspect --format {{json .Labels}} session-volume", "docker volume rm --force session-volume"},
	} {
		inspectIndex := commandIndex(calls, sequence.inspect)
		removeIndex := commandIndex(calls, sequence.remove)
		if inspectIndex < 0 || removeIndex <= inspectIndex {
			t.Errorf("resource identity was not inspected before removal: inspect=%q remove=%q\n%s", sequence.inspect, sequence.remove, strings.Join(calls, "\n"))
		}
	}
}

func TestCleanupRefusesTestcontainersResourceWhoseIdentityChanges(t *testing.T) {
	project := "semlia-accept-a1b2c3d4-000000000001"
	sessionLabel := testcontainersSessionLabelKey + "=" + project
	run := &freshCloneRun{
		root:        t.TempDir(),
		project:     project,
		environment: []string{"TESTCONTAINERS_SESSION_ID=" + project},
		cleanupExecutor: func(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, error) {
			if name != "docker" || len(args) < 2 {
				return nil, nil
			}
			switch {
			case args[0] == "network" && args[1] == "ls" && argumentAfter(args, "--filter") == "label="+sessionLabel:
				return []byte("session-network\n"), nil
			case args[0] == "network" && args[1] == "inspect":
				return json.Marshal(map[string]string{testcontainersSessionLabelKey: project + "-unrelated"})
			case args[0] == "network" && args[1] == "rm":
				t.Fatal("cleanup removed a Testcontainers network after its task identity changed")
			}
			return nil, nil
		},
	}
	err := run.removeRemainingTaskDockerResources()
	if err == nil || !strings.Contains(err.Error(), "refuse to remove testcontainers networks session-network") {
		t.Fatalf("changed Testcontainers network identity was not rejected: %v", err)
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
		if err := cleanupDockerProbeContainers(taskLabel, unrelatedLabel); err != nil {
			t.Errorf("clean Docker lifecycle probe containers: %v", err)
		}
	})

	unrelatedID, err := dockerProbeOutput(ctx,
		"run", "--detach", "--rm", "--label", unrelatedLabel,
		image, "sh", "-c", "sleep 300",
	)
	if err != nil {
		t.Fatalf("start unrelated Docker cleanup probe container: %v\n%s", err, unrelatedID)
	}
	unrelatedID = strings.TrimSpace(unrelatedID)

	foreground, resolveErr := acceptanceCommandContext(ctx, dockerProbeEnvironment(),
		"docker", "run", "--rm", "--label", taskLabel,
		image, "sh", "-c", "sleep 300",
	)
	if resolveErr != nil {
		t.Fatalf("resolve Docker cleanup probe CLI: %v", resolveErr)
	}
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

func TestDockerProbeCleanupUsesIndependentTimeoutsAndExactRelists(t *testing.T) {
	timedOutLabel := acceptanceDockerLabelKey + "=timeout-run"
	failedRemoveLabel := acceptanceDockerLabelKey + "=failed-remove-run"
	removedLabel := acceptanceDockerLabelKey + "=removed-run"
	containers := map[string][]string{
		failedRemoveLabel: {"abcdef123456"},
		removedLabel:      {"123456abcdef"},
	}
	var calls []string
	listCalls := map[string]int{}
	executor := func(ctx context.Context, args ...string) (string, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("Docker probe cleanup command has no phase deadline")
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("Docker probe cleanup reused an expired phase context: %v", err)
		}
		joined := strings.Join(args, " ")
		calls = append(calls, joined)
		switch args[0] {
		case "ps":
			label := strings.TrimPrefix(argumentAfter(args, "--filter"), "label=")
			listCalls[label]++
			if label == timedOutLabel && listCalls[label] == 1 {
				<-ctx.Done()
				return "timed out list", ctx.Err()
			}
			return strings.Join(containers[label], "\n"), nil
		case "rm":
			identifier := args[len(args)-1]
			if identifier == "abcdef123456" {
				return "container busy", errors.New("remove failed")
			}
			containers[removedLabel] = nil
			return "", nil
		default:
			return "", fmt.Errorf("unexpected Docker probe cleanup command: %s", joined)
		}
	}

	err := cleanupDockerProbeContainersWithTimeout(
		executor,
		10*time.Millisecond,
		timedOutLabel,
		failedRemoveLabel,
		removedLabel,
	)
	if err == nil {
		t.Fatal("Docker probe cleanup failures and residual containers were not reported")
	}
	for _, fragment := range []string{
		"initial exact list " + timedOutLabel,
		"context deadline exceeded",
		"remove " + failedRemoveLabel,
		"remove failed",
		"container busy",
		"exact relist " + failedRemoveLabel,
		"abcdef123456",
	} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("Docker probe cleanup error missing %q: %v", fragment, err)
		}
	}
	for _, label := range []string{timedOutLabel, failedRemoveLabel, removedLabel} {
		wantList := "ps --all --filter label=" + label + " --format {{.ID}}"
		if listCalls[label] != 2 {
			t.Errorf("exact list calls for %q = %d, want initial list plus final relist; calls:\n%s", label, listCalls[label], strings.Join(calls, "\n"))
		}
		if got := strings.Count(strings.Join(calls, "\n"), wantList); got != 2 {
			t.Errorf("exact list command %q calls = %d, want 2; calls:\n%s", wantList, got, strings.Join(calls, "\n"))
		}
	}
	if containers[removedLabel] != nil {
		t.Fatalf("successful Docker probe cleanup left task container: %v", containers[removedLabel])
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
	command, resolveErr := acceptanceCommandContext(ctx, dockerProbeEnvironment(), "docker", args...)
	if resolveErr != nil {
		return "", resolveErr
	}
	output, err := command.CombinedOutput()
	return string(output), err
}

func dockerProbeEnvironment() []string {
	return append(filteredEnvironment(os.Environ(), "PATH"), "PATH="+trustedAcceptancePath())
}

type dockerProbeExecutor func(context.Context, ...string) (string, error)

func cleanupDockerProbeContainers(labels ...string) error {
	return cleanupDockerProbeContainersWithTimeout(dockerProbeOutput, 30*time.Second, labels...)
}

func cleanupDockerProbeContainersWithTimeout(executor dockerProbeExecutor, timeout time.Duration, labels ...string) error {
	var problems []error
	for _, label := range labels {
		listArgs := []string{"ps", "--all", "--filter", "label=" + label, "--format", "{{.ID}}"}
		output, err := executeDockerProbeCleanupPhase(executor, timeout, listArgs...)
		if err != nil {
			problems = append(problems, fmt.Errorf("initial exact list %s: %w: %s", label, err, strings.TrimSpace(output)))
		} else {
			identifiers, identifierErr := validateDockerProbeContainerIdentifiers(output)
			if identifierErr != nil {
				problems = append(problems, fmt.Errorf("initial exact list %s: %w", label, identifierErr))
			} else if len(identifiers) != 0 {
				removeArgs := append([]string{"rm", "--force", "--volumes"}, identifiers...)
				removeOutput, removeErr := executeDockerProbeCleanupPhase(executor, timeout, removeArgs...)
				if removeErr != nil {
					problems = append(problems, fmt.Errorf("remove %s containers %s: %w: %s", label, strings.Join(identifiers, ","), removeErr, strings.TrimSpace(removeOutput)))
				}
			}
		}

		verifyOutput, verifyErr := executeDockerProbeCleanupPhase(executor, timeout, listArgs...)
		if verifyErr != nil {
			problems = append(problems, fmt.Errorf("exact relist %s: %w: %s", label, verifyErr, strings.TrimSpace(verifyOutput)))
			continue
		}
		remaining, identifierErr := validateDockerProbeContainerIdentifiers(verifyOutput)
		if identifierErr != nil {
			problems = append(problems, fmt.Errorf("exact relist %s: %w", label, identifierErr))
			continue
		}
		if len(remaining) != 0 {
			problems = append(problems, fmt.Errorf("exact relist %s found residual containers: %s", label, strings.Join(remaining, ",")))
		}
	}
	return errors.Join(problems...)
}

func executeDockerProbeCleanupPhase(executor dockerProbeExecutor, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return executor(ctx, args...)
}

func validateDockerProbeContainerIdentifiers(output string) ([]string, error) {
	identifiers := strings.Fields(output)
	for _, identifier := range identifiers {
		if !regexp.MustCompile(`^[a-f0-9]{12,64}$`).MatchString(identifier) {
			return nil, fmt.Errorf("invalid Docker probe container identifier %q", identifier)
		}
	}
	return identifiers, nil
}

func argumentAfter(arguments []string, name string) string {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == name {
			return arguments[index+1]
		}
	}
	return ""
}

func TestFreshCloneUsesIndependentGitObjects(t *testing.T) {
	scratch := t.TempDir()
	source := filepath.Join(scratch, "source")
	checkout := filepath.Join(scratch, "checkout")
	environment := isolatedEnvironmentFrom(
		os.Environ(),
		filepath.Join(scratch, "environment"),
		"semlia-accept-a1b2c3d4-000000000001",
		38080,
		35432,
	)
	if err := prepareIsolatedEnvironmentDirectories(environment); err != nil {
		t.Fatal(err)
	}
	runGit := func(dir string, args ...string) string {
		t.Helper()
		output, err := executeSubprocess(context.Background(), dir, environment, "git", args...)
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
		}
		return strings.TrimSpace(string(output))
	}
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(source, "init", "--quiet")
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("tracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(source, "add", "tracked.txt")
	runGit(source, "-c", "user.name=Semlia Acceptance", "-c", "user.email=acceptance@invalid", "commit", "--quiet", "-m", "initial")
	ref := runGit(source, "rev-parse", "HEAD")
	sourceObject := filepath.Join(source, ".git", "objects", ref[:2], ref[2:])
	sourceInfo, err := os.Stat(sourceObject)
	if err != nil {
		t.Fatal(err)
	}

	run := &freshCloneRun{t: t, root: checkout, environment: environment}
	run.clone(context.Background(), freshCloneConfig{source: source, ref: ref})
	checkoutObject := filepath.Join(checkout, ".git", "objects", ref[:2], ref[2:])
	checkoutInfo, err := os.Stat(checkoutObject)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(sourceInfo, checkoutInfo) {
		t.Fatal("fresh clone reused a hard-linked Git object from the source repository")
	}
	if err := os.Chmod(checkoutObject, 0o400); err != nil {
		t.Fatal(err)
	}
	unchangedSourceInfo, err := os.Stat(sourceObject)
	if err != nil {
		t.Fatal(err)
	}
	if unchangedSourceInfo.Mode().Perm() != sourceInfo.Mode().Perm() {
		t.Fatalf("checkout object mode changed source object: got %04o, want %04o", unchangedSourceInfo.Mode().Perm(), sourceInfo.Mode().Perm())
	}
}

func (run *freshCloneRun) clone(ctx context.Context, config freshCloneConfig) {
	run.t.Helper()
	clone, err := acceptanceCommandContext(ctx, run.environment, "git", "clone", "--no-checkout", "--no-hardlinks", "--", config.source, run.root)
	if err != nil {
		run.t.Fatalf("resolve isolated git clone: %v", err)
	}
	if output, err := clone.CombinedOutput(); err != nil {
		run.t.Fatalf("clone isolated checkout: %v\n%s", err, output)
	}
	ancestor, err := acceptanceCommandContext(ctx, run.environment, "git", "-C", config.source, "merge-base", "--is-ancestor", config.ref, "HEAD")
	if err != nil {
		run.t.Fatalf("resolve isolated git ancestry check: %v", err)
	}
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

	command, commandErr := acceptanceCommandContext(ctx, run.environment, "make", "contracts-check")
	if commandErr != nil {
		run.t.Fatalf("resolve isolated contract-drift command: %v", commandErr)
	}
	command.Dir = run.root
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

	command, commandErr := acceptanceCommandContext(ctx, run.environment, "go", "run", "./cmd/semlia", "server")
	if commandErr != nil {
		run.t.Fatalf("resolve isolated production fail-closed command: %v", commandErr)
	}
	command.Dir = run.root
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
	command, resolveErr := acceptanceCommandContext(ctx, run.environment, name, args...)
	if resolveErr != nil {
		return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), resolveErr)
	}
	command.Dir = run.root
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
		{label: "containers", listArgs: []string{"ps", "--all", "--filter", "label=" + composeLabel, "--format", "{{.ID}}"}, removeArgs: []string{"rm", "--force"}, identityLabel: composeLabel, inspectFormat: "{{json .Config.Labels}}", maxIdentities: composeServiceContainerLimit},
		{label: "networks", listArgs: []string{"network", "ls", "--filter", "label=" + composeLabel, "--format", "{{.ID}}"}, removeArgs: []string{"network", "rm"}, identityLabel: composeLabel, inspectFormat: "{{json .Labels}}", maxIdentities: composeNetworkLimit},
		{label: "volumes", listArgs: []string{"volume", "ls", "--filter", "label=" + composeLabel, "--format", "{{.Name}}"}, removeArgs: []string{"volume", "rm", "--force"}, identityLabel: composeLabel, inspectFormat: "{{json .Labels}}", maxIdentities: composeVolumeLimit},
	}
	if taskLabel := environmentValue(run.environment, "SEMLIA_DOCKER_RESOURCE_LABEL"); taskLabel != "" {
		resources = append(resources, cleanupResourceKind{
			label:         "tool containers",
			listArgs:      []string{"ps", "--all", "--filter", "label=" + taskLabel, "--format", "{{.ID}}"},
			removeArgs:    []string{"rm", "--force"},
			identityLabel: taskLabel,
			inspectFormat: "{{json .Config.Labels}}",
			maxIdentities: toolContainerLimit,
		})
	}
	if sessionID := environmentValue(run.environment, "TESTCONTAINERS_SESSION_ID"); sessionID != "" {
		sessionLabel := testcontainersSessionLabelKey + "=" + sessionID
		resources = append(resources,
			cleanupResourceKind{
				label: "testcontainers containers", listArgs: []string{"ps", "--all", "--filter", "label=" + sessionLabel, "--format", "{{.ID}}"},
				removeArgs: []string{"rm", "--force", "--volumes"}, identityLabel: sessionLabel, inspectFormat: "{{json .Config.Labels}}", maxIdentities: testcontainersContainerLimit,
			},
			cleanupResourceKind{
				label: "testcontainers networks", listArgs: []string{"network", "ls", "--filter", "label=" + sessionLabel, "--format", "{{.ID}}"},
				removeArgs: []string{"network", "rm"}, identityLabel: sessionLabel, inspectFormat: "{{json .Labels}}", maxIdentities: testcontainersNetworkLimit,
			},
			cleanupResourceKind{
				label: "testcontainers volumes", listArgs: []string{"volume", "ls", "--filter", "label=" + sessionLabel, "--format", "{{.Name}}"},
				removeArgs: []string{"volume", "rm", "--force"}, identityLabel: sessionLabel, inspectFormat: "{{json .Labels}}", maxIdentities: testcontainersVolumeLimit,
			},
		)
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
		if len(resource.listArgs) > 0 && resource.listArgs[0] == "ps" {
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
		inspectArgs := []string{"inspect", "--format", resource.inspectFormat, identifier}
		if len(resource.listArgs) > 0 && (resource.listArgs[0] == "network" || resource.listArgs[0] == "volume") {
			inspectArgs = append([]string{resource.listArgs[0]}, inspectArgs...)
		}
		output, err := run.cleanupCommand(dockerIdentityTimeout, "docker", inspectArgs...)
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
		if removeOutput, removeErr := run.cleanupCommand(dockerResourceRemovalTimeout, "docker", removeArgs...); removeErr != nil {
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
	command, err := acceptanceCommandContext(ctx, environment, name, args...)
	if err != nil {
		return nil, err
	}
	command.Dir = dir
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
