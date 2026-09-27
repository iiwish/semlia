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
			"http://127.0.0.1:18081/health/ready",
			"http://127.0.0.1:18080/api/v1/system/info",
			"make dev-down",
			"existing PostgreSQL 18",
			"./scripts/dev/ensure-env.sh",
			"node scripts/dev/native.mjs migrate up",
			"bootstrap-local-admin",
			"does **not** migrate automatically",
			"does not recreate containers or inject an outage",
			"stops only owned native processes",
			"independently review/publish an analysis model",
		},
		"docs/operations/local-development.md": {
			"migrate",
			"server",
			"worker",
			".semlia/dev.env",
			"Vite",
			"Argon2id",
			"account.sh",
			"不停止 PostgreSQL",
			"make check-smoke",
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
	for _, path := range []string{"docs/quickstart.md", "docs/operations/troubleshooting.md"} {
		content := readRepositoryFile(t, path)
		for _, unsafe := range []string{"rm .semlia/dev.env", "`make smoke` is disruptive", "Stop the other listener"} {
			if strings.Contains(content, unsafe) {
				t.Errorf("%s contains obsolete unsafe instruction %q", path, unsafe)
			}
		}
	}
}
