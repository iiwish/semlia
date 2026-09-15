package repository_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicationEvidenceOmitsMachineInventories(t *testing.T) {
	for _, path := range []string{
		"docs/evidence/SP-T001/checkout.json",
		"docs/evidence/SP-T002/checkout.json",
		"docs/evidence/SP-T001/environment-after.json",
	} {
		var evidence map[string]json.RawMessage
		if err := json.Unmarshal([]byte(read(t, path)), &evidence); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"containersBefore", "existingContainers", "additionalContainers"} {
			if _, exists := evidence[field]; exists {
				t.Errorf("%s exposes a machine-wide %s inventory", path, field)
			}
		}
	}
}

func TestPublicationDeploymentRequiresOperatorConfiguration(t *testing.T) {
	compose := read(t, "deploy/examples/compose.yaml")
	for _, variable := range []string{
		"SEMLIA_COMPOSE_PROJECT", "SEMLIA_RUNTIME_ENV_FILE", "SEMLIA_MIGRATION_ENV_FILE",
		"SEMLIA_ARTIFACT_HOST_DIR", "SEMLIA_CONTENT_HOST_DIR", "SEMLIA_DATABASE_NETWORK",
	} {
		if !strings.Contains(compose, "${"+variable+":?") {
			t.Errorf("deployment example must require explicit %s", variable)
		}
	}
	for _, value := range []string{"read_only: true", "host_ip: 127.0.0.1", "create_host_path: false", "mode=1777"} {
		if !strings.Contains(compose, value) {
			t.Errorf("deployment example is missing safety constraint %q", value)
		}
	}
}
