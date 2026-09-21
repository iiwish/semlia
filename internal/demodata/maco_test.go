package demodata

import (
	"encoding/json"
	"github.com/iiwish/semlia/pkg/identity"
	"testing"
)

func TestMacoFixtureUsesOnlyRegisteredProjectDatabase(t *testing.T) {
	s := NewMaco()
	w, _ := identity.NewWorkspaceID()
	p, _ := identity.NewPrincipalID()
	s.Snapshot.WorkspaceID = w
	statements, err := sourceStatements(w, p, &s.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if statements[0].args[2] != MacoSourceLocator {
		t.Fatal("wrong database locator")
	}
	b, _, err := previewMetadataFor([]byte(`{"synthetic":true,"provenance":"declared_local_fixture"}`), MacoSourceLocator)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	_ = json.Unmarshal(b, &config)
	if config["host"] != "postgresql" || config["database"] != "semlia_test" || config["username"] != "semlia_test_demo" {
		t.Fatal("unsafe source configuration")
	}
	if _, _, err = previewMetadataFor(b, "postgresql:5432/other_project"); err == nil {
		t.Fatal("accepted another project")
	}
}
