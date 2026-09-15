package repository_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type workflowDocument struct {
	Name        string                 `yaml:"name"`
	On          map[string]any         `yaml:"on"`
	Permissions map[string]string      `yaml:"permissions"`
	Jobs        map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Uses        string            `yaml:"uses"`
	Needs       []string          `yaml:"needs"`
	Permissions map[string]string `yaml:"permissions"`
	Steps       []workflowStep    `yaml:"steps"`
}

func TestReleaseRequiresSameRevisionGates(t *testing.T) {
	release := readWorkflow(t, "release.yml")
	for job, file := range map[string]string{"source-and-smoke": "ci.yml", "security": "security.yml"} {
		if release.Jobs[job].Uses != "./.github/workflows/"+file {
			t.Fatalf("missing local reusable gate %s", job)
		}
		found := false
		for _, dependency := range release.Jobs["build"].Needs {
			if dependency == job {
				found = true
			}
		}
		if !found {
			t.Fatalf("release build does not depend on %s", job)
		}
		if _, ok := readWorkflow(t, file).On["workflow_call"]; !ok {
			t.Fatalf("%s cannot be called", file)
		}
	}
}

type workflowStep struct {
	Name string `yaml:"name"`
	Uses string `yaml:"uses"`
	Run  string `yaml:"run"`
}

func readWorkflow(t *testing.T, name string) workflowDocument {
	t.Helper()

	path := filepath.Join(".github", "workflows", name)
	var workflow workflowDocument
	if err := yaml.Unmarshal([]byte(read(t, path)), &workflow); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if workflow.Name == "" || len(workflow.Jobs) == 0 {
		t.Fatalf("%s must define a name and at least one job", path)
	}
	return workflow
}

func workflowRuns(workflow workflowDocument) []string {
	var runs []string
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if step.Run != "" {
				runs = append(runs, strings.TrimSpace(step.Run))
			}
		}
	}
	sort.Strings(runs)
	return runs
}

func TestCIRequiredFilesAndLocalCommandsExist(t *testing.T) {
	required := []string{
		".github/workflows/ci.yml",
		".github/workflows/security.yml",
		".github/workflows/release.yml",
		".github/dependabot.yml",
		".github/pull_request_template.md",
		"scripts/ci/check-source.sh",
		"scripts/ci/check-smoke.sh",
		"scripts/ci/security-check.sh",
		"scripts/release/build.sh",
		"scripts/release/sbom.sh",
	}
	for _, path := range required {
		info, err := os.Stat(filepath.Join(root, path))
		if err != nil || !info.Mode().IsRegular() {
			t.Errorf("required CI file is missing: %s", path)
		}
	}

	makefile := read(t, "Makefile")
	for _, target := range []string{
		"check", "check-source", "check-smoke", "format-check", "lint", "typecheck",
		"test", "security-check", "build-image", "sbom", "release",
	} {
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(target) + `:(?:\s|$)`).MatchString(makefile) {
			t.Errorf("missing Make target %s", target)
		}
	}

	checkRecipe := regexp.MustCompile(`(?ms)^check:.*?(?:\n\S|\z)`).FindString(makefile)
	for _, requiredTarget := range []string{"check-source", "check-browser", "check-smoke", "security-check"} {
		if !strings.Contains(checkRecipe, requiredTarget) {
			t.Errorf("make check does not include %s", requiredTarget)
		}
	}
}

func TestCIWorkflowsMapValidationToMake(t *testing.T) {
	expected := map[string][]string{
		"ci.yml":       {"make check-smoke", "make check-source", "make browser-bootstrap", "make check-browser"},
		"security.yml": {"make security-check"},
		"release.yml":  {"make release"},
	}

	for name, want := range expected {
		workflow := readWorkflow(t, name)
		got := workflowRuns(workflow)
		for _, command := range got {
			if !strings.HasPrefix(command, "make ") {
				t.Errorf("%s validation command must map to Make, got %q", name, command)
			}
		}
		for _, command := range want {
			if !containsString(got, command) {
				t.Errorf("%s missing local validation command %q; got %v", name, command, got)
			}
		}
	}
}

func TestCISmokeJobPreparesWebToolchain(t *testing.T) {
	workflow := readWorkflow(t, "ci.yml")
	smoke, ok := workflow.Jobs["smoke"]
	if !ok {
		t.Fatal("ci.yml must define the smoke job")
	}

	var uses, runs []string
	for _, step := range smoke.Steps {
		uses = append(uses, step.Uses)
		if step.Run != "" {
			runs = append(runs, strings.TrimSpace(step.Run))
		}
	}

	for _, action := range []string{"pnpm/action-setup@", "actions/setup-node@", "actions/setup-go@"} {
		found := false
		for _, value := range uses {
			if strings.HasPrefix(value, action) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("smoke job must prepare %s", strings.TrimSuffix(action, "@"))
		}
	}

	bootstrapIndex, smokeIndex := -1, -1
	for index, command := range runs {
		switch command {
		case "make bootstrap":
			bootstrapIndex = index
		case "make check-smoke":
			smokeIndex = index
		}
	}
	if bootstrapIndex < 0 || smokeIndex < 0 || bootstrapIndex >= smokeIndex {
		t.Errorf("smoke job must run make bootstrap before make check-smoke; got %v", runs)
	}
}

func TestCIWorkflowsPinActionsAndUseLeastPrivilege(t *testing.T) {
	fullSHA := regexp.MustCompile(`^[^@]+@[0-9a-f]{40}$`)
	for _, name := range []string{"ci.yml", "security.yml", "release.yml"} {
		workflow := readWorkflow(t, name)
		if strings.Contains(read(t, filepath.Join(".github", "workflows", name)), "pull_request_target") {
			t.Errorf("%s must not use pull_request_target", name)
		}
		if workflow.Permissions["contents"] != "read" {
			t.Errorf("%s top-level contents permission = %q, want read", name, workflow.Permissions["contents"])
		}
		for permission, access := range workflow.Permissions {
			if permission != "contents" || access != "read" {
				t.Errorf("%s has excessive top-level permission %s=%s", name, permission, access)
			}
		}

		for jobName, job := range workflow.Jobs {
			for permission, access := range job.Permissions {
				allowedReleasePermission := name == "release.yml" &&
					((permission == "contents" && access == "read") ||
						(jobName == "image" && permission == "packages" && access == "write"))
				if !allowedReleasePermission {
					t.Errorf("%s job %s has excessive permission %s=%s", name, jobName, permission, access)
				}
			}
			for _, step := range job.Steps {
				if step.Uses != "" && !strings.HasPrefix(step.Uses, "./") && !fullSHA.MatchString(step.Uses) {
					t.Errorf("%s action is not pinned to a full commit SHA: %s", name, step.Uses)
				}
			}
		}
	}
}

func TestCISecurityAndReleaseTriggersAreExplicit(t *testing.T) {
	ci := readWorkflow(t, "ci.yml")
	if _, ok := ci.On["pull_request"]; !ok {
		t.Error("ci.yml must run on pull_request")
	}

	security := readWorkflow(t, "security.yml")
	for _, trigger := range []string{"pull_request", "schedule", "workflow_dispatch"} {
		if _, ok := security.On[trigger]; !ok {
			t.Errorf("security.yml missing %s trigger", trigger)
		}
	}

	release := readWorkflow(t, "release.yml")
	if _, ok := release.On["workflow_dispatch"]; !ok {
		t.Error("release.yml must support workflow_dispatch")
	}
	if len(release.On) != 1 {
		t.Error("release.yml must remain manual-only for locally built releases")
	}
}

func TestCISecurityScannerAndReleaseArtifactContracts(t *testing.T) {
	security := read(t, "scripts/ci/security-check.sh")
	for _, required := range []string{
		"aquasec/trivy:0.73.0@sha256:",
		"--scanners vuln,secret",
		"--include-dev-deps",
		"--severity HIGH,CRITICAL",
		"--exit-code 1",
		"image",
	} {
		if !strings.Contains(security, required) {
			t.Errorf("security-check.sh missing %q", required)
		}
	}

	release := read(t, "scripts/release/build.sh") + read(t, "scripts/release/sbom.sh")
	for _, required := range []string{"CycloneDX", "SHA256SUMS", "SEMLIA_VERSION", "SEMLIA_COMMIT", "GOHOSTOS", "GOHOSTARCH", "rootfs", "verify -manifest"} {
		if !strings.Contains(release, required) {
			t.Errorf("release tooling missing %q", required)
		}
	}

	var dependabot struct {
		Version int `yaml:"version"`
		Updates []struct {
			Ecosystem string `yaml:"package-ecosystem"`
		} `yaml:"updates"`
	}
	if err := yaml.Unmarshal([]byte(read(t, ".github/dependabot.yml")), &dependabot); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, update := range dependabot.Updates {
		got[update.Ecosystem] = true
	}
	for _, ecosystem := range []string{"gomod", "npm", "docker", "github-actions"} {
		if !got[ecosystem] {
			t.Errorf("Dependabot does not monitor %s", ecosystem)
		}
	}

	template := strings.ToLower(read(t, ".github/pull_request_template.md"))
	for _, phrase := range []string{"requirement", "test evidence", "residual risk"} {
		if !strings.Contains(template, phrase) {
			t.Errorf("pull request template missing %q", phrase)
		}
	}
}

func TestDockerRunScriptsApplyValidatedTaskResourceLabel(t *testing.T) {
	tests := []struct {
		path    string
		runArgs []string
		runs    int
	}{
		{path: "scripts/ci/security-check.sh", runs: 2},
		{path: "scripts/release/sbom.sh", runArgs: []string{"build/semlia", "build/release/semlia.sbom.cdx.json"}, runs: 4},
	}
	for _, test := range tests {
		t.Run(filepath.Base(test.path), func(t *testing.T) {
			unlabeled, output, err := runDockerScriptWithLabel(t, test.path, test.runArgs, "")
			if err != nil {
				t.Fatalf("unlabeled script failed: %v\n%s", err, output)
			}
			assertDockerRunLabel(t, unlabeled, test.runs, "")

			label := "io.semlia.acceptance.run=semlia-accept-a1b2c3d4-000000000001"
			labeled, output, err := runDockerScriptWithLabel(t, test.path, test.runArgs, label)
			if err != nil {
				t.Fatalf("labeled script failed: %v\n%s", err, output)
			}
			assertDockerRunLabel(t, labeled, test.runs, label)

			invalid, output, err := runDockerScriptWithLabel(t, test.path, test.runArgs, "io.semlia.acceptance.run=task --privileged")
			if err == nil {
				t.Fatalf("invalid Docker label was accepted:\n%s", output)
			}
			if !strings.Contains(output, "invalid SEMLIA_DOCKER_RESOURCE_LABEL") {
				t.Fatalf("invalid-label failure is not actionable:\n%s", output)
			}
			if strings.TrimSpace(invalid) != "" {
				t.Fatalf("invalid label reached Docker:\n%s", invalid)
			}
		})
	}
}

func runDockerScriptWithLabel(t *testing.T, relativePath string, args []string, label string) (string, string, error) {
	t.Helper()
	scratch := t.TempDir()
	target := filepath.Join(scratch, relativePath)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(read(t, relativePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(scratch, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scratch, "build", "semlia"), []byte("test binary\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if relativePath == "scripts/release/sbom.sh" {
		if err := os.WriteFile(filepath.Join(scratch, "scripts/release/normalize-sbom.mjs"), []byte(read(t, "scripts/release/normalize-sbom.mjs")), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(scratch, "build/release"), 0o755); err != nil {
			t.Fatal(err)
		}
		// Docker is a call-recording stub in this resource-label test.
		if err := os.WriteFile(filepath.Join(scratch, "build/release/semlia.sbom.cdx.json"), []byte(`{"components":[]}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	bin := filepath.Join(scratch, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(scratch, "docker.log")
	dockerStub := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$DOCKER_LOG\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(dockerStub), 0o755); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(target, args...)
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "SEMLIA_DOCKER_RESOURCE_LABEL=") && !strings.HasPrefix(entry, "PATH=") {
			environment = append(environment, entry)
		}
	}
	environment = append(environment, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "DOCKER_LOG="+logPath)
	if label != "" {
		environment = append(environment, "SEMLIA_DOCKER_RESOURCE_LABEL="+label)
	}
	command.Env = environment
	output, err := command.CombinedOutput()
	log, readErr := os.ReadFile(logPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	return string(log), string(output), err
}

func assertDockerRunLabel(t *testing.T, log string, wantRuns int, wantLabel string) {
	t.Helper()
	var runs []string
	for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
		if strings.HasPrefix(line, "run ") {
			runs = append(runs, line)
		}
	}
	if len(runs) != wantRuns {
		t.Fatalf("docker run calls = %d, want %d:\n%s", len(runs), wantRuns, log)
	}
	for _, run := range runs {
		if wantLabel == "" {
			if strings.Contains(run, "--label") {
				t.Errorf("ordinary Docker run gained a label: %s", run)
			}
			continue
		}
		if got := strings.Count(run, "--label "+wantLabel); got != 1 {
			t.Errorf("Docker run label count = %d, want 1: %s", got, run)
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
