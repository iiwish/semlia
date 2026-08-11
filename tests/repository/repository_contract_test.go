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
		"module":    `(?m)^module github\.com/semlia/semlia$`,
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
