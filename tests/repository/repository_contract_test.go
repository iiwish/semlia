package repository_test

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

var root = repositoryRoot()

var requiredFiles = []string{
	"README.md",
	"LICENSE",
	"NOTICE",
	".gitignore",
	".editorconfig",
	".tool-versions",
	"go.mod",
	"package.json",
	"pnpm-workspace.yaml",
	"pnpm-lock.yaml",
	"Makefile",
	"CONTRIBUTING.md",
	"CODE_OF_CONDUCT.md",
	"SECURITY.md",
	"scripts/doctor.sh",
}

var expectedTools = map[string]string{
	"golang": "1.26.5",
	"nodejs": "24.15.0",
	"pnpm":   "11.1.3",
}

var forbiddenLegacyFiles = []string{
	".python-version",
	"pyproject.toml",
	"uv.lock",
}

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository contract path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func read(t *testing.T, relativePath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(content)
}

func TestRequiredRepositoryFilesExist(t *testing.T) {
	var missing []string
	for _, path := range requiredFiles {
		info, err := os.Stat(filepath.Join(root, path))
		if err != nil || !info.Mode().IsRegular() {
			missing = append(missing, path)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing repository files: %s", strings.Join(missing, ", "))
	}
}

func TestToolVersionsArePinnedAndConsistent(t *testing.T) {
	entries := map[string]string{}
	for _, line := range strings.Split(read(t, ".tool-versions"), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if len(fields) != 2 {
			t.Fatalf("invalid .tool-versions entry: %q", line)
		}
		entries[fields[0]] = fields[1]
	}

	if !maps.Equal(entries, expectedTools) {
		t.Fatalf("tool versions mismatch: got %v, want %v", entries, expectedTools)
	}

	goMod := read(t, "go.mod")
	for label, pattern := range map[string]string{
		"module":    `(?m)^module github\.com/iiwish/semlia$`,
		"language":  `(?m)^go 1\.26\.0$`,
		"toolchain": `(?m)^toolchain go1\.26\.5$`,
	} {
		if !regexp.MustCompile(pattern).MatchString(goMod) {
			t.Errorf("go.mod %s declaration does not match %s", label, pattern)
		}
	}

	var packageJSON struct {
		Private        bool              `json:"private"`
		PackageManager string            `json:"packageManager"`
		Engines        map[string]string `json:"engines"`
		Scripts        map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(read(t, "package.json")), &packageJSON); err != nil {
		t.Fatalf("parse package.json: %v", err)
	}
	if !packageJSON.Private {
		t.Error("package.json must be private")
	}
	if packageJSON.PackageManager != "pnpm@11.1.3" {
		t.Errorf("packageManager = %q", packageJSON.PackageManager)
	}
	if packageJSON.Engines["node"] != ">=24.15.0 <25" || packageJSON.Engines["pnpm"] != ">=11.1.3 <12" {
		t.Errorf("unexpected package engines: %v", packageJSON.Engines)
	}
	if packageJSON.Scripts["check:repository"] != "go test ./tests/repository" {
		t.Errorf("unexpected repository check: %q", packageJSON.Scripts["check:repository"])
	}
}

func TestDoctorVersionMatchesPinnedToolchain(t *testing.T) {
	toolVersions := map[string]string{}
	for _, line := range strings.Split(read(t, ".tool-versions"), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && !strings.HasPrefix(fields[0], "#") {
			toolVersions[fields[0]] = fields[1]
		}
	}

	match := regexp.MustCompile(`(?m)^readonly REQUIRED_GO="([^"]+)"$`).FindStringSubmatch(read(t, "scripts/doctor.sh"))
	if len(match) != 2 {
		t.Fatal("scripts/doctor.sh must declare REQUIRED_GO")
	}
	if got, want := match[1], toolVersions["golang"]; got != want {
		t.Fatalf("doctor Go version = %s, want pinned .tool-versions value %s", got, want)
	}
}

func TestModuleNamespaceMatchesControlledRepository(t *testing.T) {
	const controlledModule = "github.com/iiwish/semlia"
	goMod := read(t, "go.mod")
	if !regexp.MustCompile(`(?m)^module ` + regexp.QuoteMeta(controlledModule) + `$`).MatchString(goMod) {
		t.Errorf("go.mod module must be %s", controlledModule)
	}

	legacyModule := "github.com/semlia/semlia"
	var legacyImports []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".semlia", "build", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if filepath.ToSlash(relative) == "tests/repository/repository_contract_test.go" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), legacyModule) {
			legacyImports = append(legacyImports, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(legacyImports)
	if len(legacyImports) > 0 {
		t.Errorf("live Go source still imports %s: %s", legacyModule, strings.Join(legacyImports, ", "))
	}
}

func TestSmokeJourneyUsesActiveComposeProject(t *testing.T) {
	smoke := read(t, "tests/smoke/local_stack_test.go")
	if strings.Contains(smoke, "com.docker.compose.project=semlia-local") {
		t.Error("smoke cleanup must not target the default Compose project literally")
	}
	for _, fragment := range []string{"composeProject(t)", `os.Getenv("COMPOSE_PROJECT_NAME")`, "COMPOSE_PROJECT_NAME=", `"label=com.docker.compose.project="+project`} {
		if !strings.Contains(smoke, fragment) {
			t.Errorf("smoke journey missing active-project contract %q", fragment)
		}
	}
}

func TestPythonToolchainIsNotPartOfBaseline(t *testing.T) {
	for _, path := range forbiddenLegacyFiles {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Errorf("legacy Python baseline file must be absent: %s", path)
		}
	}
}

func TestMakefileExposesStableRootCommands(t *testing.T) {
	makefile := read(t, "Makefile")
	for _, target := range []string{"bootstrap", "doctor", "clean"} {
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(target) + `:(?:\s|$)`).MatchString(makefile) {
			t.Errorf("missing Make target %s", target)
		}
		if !regexp.MustCompile(`(?m)^\.PHONY:.*\b` + regexp.QuoteMeta(target) + `\b`).MatchString(makefile) {
			t.Errorf("Make target %s is not phony", target)
		}
	}
}

func TestDoctorScriptIsExecutableAndPortable(t *testing.T) {
	path := filepath.Join(root, "scripts", "doctor.sh")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o100 == 0 {
		t.Error("doctor.sh must be executable by its owner")
	}

	script := read(t, "scripts/doctor.sh")
	for _, required := range []string{"#!/usr/bin/env bash\n", "set -euo pipefail", "REQUIRED_GO", "require_command go"} {
		if !strings.Contains(script, required) {
			t.Errorf("doctor.sh missing %q", required)
		}
	}
	for _, forbidden := range []string{"/Users/", "/home/", "REQUIRED_PYTHON", "require_command python", "require_command uv"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("doctor.sh contains forbidden baseline reference %q", forbidden)
		}
	}
}

func TestFreshCloneLauncherDisablesUserProcessControls(t *testing.T) {
	path := filepath.Join(root, "scripts", "acceptance", "m0-fresh-clone.sh")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o100 == 0 {
		t.Error("fresh-clone launcher must be executable by its owner")
	}

	script := read(t, "scripts/acceptance/m0-fresh-clone.sh")
	for _, required := range []string{
		"GOENV=off",
		"GOFLAGS=",
		"GOWORK=off",
		"SEMLIA_RUN_FRESH_CLONE=1",
		"-run '^TestFreshCloneAcceptance$'",
		"-count=1",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("fresh-clone launcher missing %q", required)
		}
	}
}

func TestDocumentationContainsNoPersonalAbsolutePaths(t *testing.T) {
	patterns := map[string]*regexp.Regexp{
		"macOS user path": regexp.MustCompile(`/Users/[^/\s]+/`),
		"Linux user path": regexp.MustCompile(`/home/[^/\s]+/`),
	}
	var findings []string
	err := filepath.WalkDir(filepath.Join(root, "docs"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() == "diff.patch" {
			return nil
		}
		switch filepath.Ext(path) {
		case ".md", ".yaml", ".yml", ".txt":
		default:
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for label, pattern := range patterns {
			if pattern.Match(content) {
				findings = append(findings, relative+": "+label)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Fatalf("documentation contains personal paths: %s", strings.Join(findings, "; "))
	}
}

func TestOpenSourcePolicyFilesAreActionable(t *testing.T) {
	checks := map[string][]string{
		"LICENSE":            {"Apache License", "Version 2.0, January 2004", "END OF TERMS AND CONDITIONS"},
		"CONTRIBUTING.md":    {"Developer Certificate of Origin", "Signed-off-by:", "make bootstrap"},
		"SECURITY.md":        {"private vulnerability reporting", "public issue"},
		"CODE_OF_CONDUCT.md": {"Contributor Covenant 3.0", "Reporting an Issue"},
	}
	for path, phrases := range checks {
		content := read(t, path)
		for _, phrase := range phrases {
			if !strings.Contains(strings.ToLower(content), strings.ToLower(phrase)) {
				t.Errorf("%s missing %q", path, phrase)
			}
		}
	}
}

func TestRootConfigurationContainsNoPersonalPathsOrSecrets(t *testing.T) {
	patterns := map[string]*regexp.Regexp{
		"macOS user path": regexp.MustCompile(`/Users/[^/\s]+/`),
		"Linux user path": regexp.MustCompile(`/home/[^/\s]+/`),
		"private key":     regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----`),
		"credential URL":  regexp.MustCompile(`postgres(?:ql)?://[^\s:@]+:[^\s@]+@`),
	}

	var findings []string
	for _, path := range requiredFiles {
		if path == "LICENSE" {
			continue
		}
		content := read(t, path)
		for label, pattern := range patterns {
			if pattern.MatchString(content) {
				findings = append(findings, path+": "+label)
			}
		}
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Fatalf("unsafe repository content: %s", strings.Join(findings, "; "))
	}
}
