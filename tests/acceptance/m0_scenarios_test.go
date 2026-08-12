package acceptance

import (
	"strings"
	"testing"
)

func TestM0AcceptanceScenarioContract(t *testing.T) {
	harness := readRepositoryFile(t, "tests/acceptance/fresh_clone_test.go")
	required := map[string][]string{
		"AC-M0-001 fresh checkout": {
			"git\", \"clone",
			"cold bootstrap",
			"warm bootstrap",
			"development readiness",
		},
		"AC-M0-002 runtime and recovery": {
			"100 sequential system-info requests",
			"DEPENDENCY_UNAVAILABLE",
			"make\", \"smoke",
		},
		"AC-M0-003 migrations": {
			"migration upgrade",
			"migration downgrade",
			"migration re-upgrade",
		},
		"AC-M0-004 workers": {
			"worker lease retry dead-letter",
			"tests/integration/worker/",
		},
		"AC-M0-005 contracts and production": {
			"acceptance drift",
			"Generated artifact is stale",
			"contract drift recovery",
			"production fail-closed",
			"SEMLIA_ALLOWED_ORIGINS=*",
		},
		"AC-M0-006 complete delivery gate": {
			"complete pull-request gate",
			"make\", \"check",
			"release bundle",
			"make\", \"release",
			"verifyReleaseBundle",
			"SHA256SUMS",
		},
	}
	for scenario, fragments := range required {
		t.Run(scenario, func(t *testing.T) {
			for _, fragment := range fragments {
				if !strings.Contains(harness, fragment) {
					t.Errorf("fresh-clone harness missing %q", fragment)
				}
			}
		})
	}

	for _, forbidden := range []string{"docker system prune", "docker volume prune", "git clean -fdx"} {
		if strings.Contains(harness, forbidden) {
			t.Errorf("fresh-clone harness contains destructive cleanup %q", forbidden)
		}
	}
	if !strings.Contains(harness, `os.Getenv("SEMLIA_RUN_FRESH_CLONE") != "1"`) {
		t.Error("fresh-clone journey must remain explicitly opt-in")
	}
	if !strings.Contains(harness, `"--volumes", "--remove-orphans"`) {
		t.Error("fresh-clone cleanup must remove only task-owned Compose resources")
	}
}
