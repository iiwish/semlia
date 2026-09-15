package repository_test

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestDeploymentTmpfsOptionsRemainOneMount(t *testing.T) {
	var compose struct {
		App struct {
			Tmpfs []string `yaml:"tmpfs"`
		} `yaml:"x-app"`
	}
	if err := yaml.Unmarshal([]byte(read(t, "deploy/examples/compose.yaml")), &compose); err != nil {
		t.Fatal(err)
	}
	if len(compose.App.Tmpfs) != 1 || compose.App.Tmpfs[0] != "/tmp:size=128m,mode=1777" {
		t.Fatalf("deployment tmpfs must be one absolute mount with both options, got %q", compose.App.Tmpfs)
	}
}
