package acceptance

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var repositoryRoot = resolveRepositoryRoot()

func resolveRepositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve acceptance contract path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func readRepositoryFile(t *testing.T, relativePath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repositoryRoot, relativePath))
	if err != nil {
		t.Errorf("read %s: %v", relativePath, err)
		return ""
	}
	return string(content)
}

func TestM0DocumentationContract(t *testing.T) {
	required := map[string][]string{
		"README.md": {
			"Private incubation",
			"docs/quickstart.md",
			"docs/operations/local-development.md",
			"docs/operations/troubleshooting.md",
		},
		"README.zh-CN.md": {
			"私有孵化",
			"docs/quickstart.md",
			"docs/operations/local-development.md",
			"docs/operations/troubleshooting.md",
		},
		"docs/SECURITY.md": {
			"private vulnerability reporting is not enabled",
			"public issue",
			"not production-ready",
		},
		"docs/quickstart.md": {
			"git clone https://github.com/iiwish/semlia.git",
			"cd semlia",
			"make doctor",
			"make bootstrap",
			"make dev",
			"make smoke",
			"http://127.0.0.1:8080/health/ready",
			"http://127.0.0.1:8080/api/v1/system/info",
			"make dev-down",
			"preview",
			"disruptive",
		},
		"docs/operations/local-development.md": {
			"migrate",
			"server",
			"worker",
			".semlia/dev.env",
			"COMPOSE_PROJECT_NAME",
			"down --volumes",
			"disruptive",
		},
		"docs/operations/troubleshooting.md": {
			"DEPENDENCY_UNAVAILABLE",
			"traceId",
			".semlia/dev.env",
			"make contracts-check",
			"make dev-down",
		},
	}

	for path, fragments := range required {
		content := readRepositoryFile(t, path)
		lowerContent := strings.ToLower(content)
		for _, fragment := range fragments {
			if !strings.Contains(lowerContent, strings.ToLower(fragment)) {
				t.Errorf("%s missing %q", path, fragment)
			}
		}
	}

	security := readRepositoryFile(t, "docs/SECURITY.md")
	if strings.Contains(security, "Use GitHub private vulnerability reporting for the Semlia repository.") {
		t.Error("docs/SECURITY.md must not direct reporters to a feature that is not enabled")
	}
	if strings.Contains(security, "Dependabot monitors") {
		t.Error("docs/SECURITY.md must distinguish version updates from disabled vulnerability alerts")
	}
}
