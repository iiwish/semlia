package repository_test

import (
	"fmt"
	"os"
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
	Permissions map[string]string `yaml:"permissions"`
	Steps       []workflowStep    `yaml:"steps"`
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
	for _, requiredTarget := range []string{"check-source", "check-smoke", "security-check"} {
		if !strings.Contains(checkRecipe, requiredTarget) {
			t.Errorf("make check does not include %s", requiredTarget)
		}
	}
}

func TestCIWorkflowsMapValidationToMake(t *testing.T) {
	expected := map[string][]string{
		"ci.yml":       {"make check-smoke", "make check-source"},
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
						(permission == "id-token" && access == "write") ||
						(permission == "attestations" && access == "write"))
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
	push, ok := release.On["push"].(map[string]any)
	if !ok || fmt.Sprint(push["tags"]) == "<nil>" {
		t.Error("release.yml must build explicit version tags")
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

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
