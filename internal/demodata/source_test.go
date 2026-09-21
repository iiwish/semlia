package demodata

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	discoveryapp "github.com/iiwish/semlia/internal/application/discovery"
	"github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestSourceStatementsAreBoundedSyntheticInsertOnly(t *testing.T) {
	scenario := New()
	workspace, _ := identity.NewWorkspaceID()
	actor, _ := identity.NewPrincipalID()
	scenario.Snapshot.WorkspaceID = workspace
	statements, err := sourceStatements(workspace, actor, &scenario.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	digestPattern := regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	for _, statement := range statements {
		words := strings.Fields(statement.sql)
		if len(words) < 3 || words[0] != "INSERT" || words[1] != "INTO" {
			t.Fatalf("not insert-only: %s", statement.sql)
		}
		table := strings.Split(words[2], "(")[0]
		counts[table]++
		if strings.Contains(statement.sql, "ON CONFLICT") {
			t.Fatal("fixture must not silently adopt existing records")
		}
		for _, arg := range statement.args {
			if value, ok := arg.(string); ok && strings.HasPrefix(value, "sha256:") && !digestPattern.MatchString(value) {
				t.Fatalf("invalid digest: %s", value)
			}
		}
	}
	want := map[string]int{"source_connections": 1, "source_revisions": 1, "source_snapshots": 1, "source_snapshot_scope": 1, "source_snapshot_diagnostics": 1, "physical_datasets": 2, "physical_dataset_revisions": 2, "physical_fields": 8, "physical_field_revisions": 8, "source_snapshot_members": 10, "source_effective_snapshots": 1}
	if len(counts) != len(want) {
		t.Fatalf("unexpected tables: %v", counts)
	}
	for table, count := range want {
		if counts[table] != count {
			t.Errorf("%s: got %d, want %d", table, counts[table], count)
		}
	}
	if !strings.Contains(statements[0].sql, "合成演示：订单与客户") || statements[0].args[2] != demoSourceLocator {
		t.Fatal("source must disclose its synthetic local identity")
	}
	var metadata map[string]any
	if err := json.Unmarshal(statements[0].args[3].([]byte), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["synthetic"] != true || metadata["provenance"] != "declared_local_fixture" || metadata["importedBy"] != actor.String() {
		t.Fatalf("missing fixture provenance: %v", metadata)
	}
}

func TestSourceStatementsRejectInconsistentOrExternalFixtures(t *testing.T) {
	for name, mutate := range map[string]func(*distribution.ReleaseSnapshot){
		"workspace":      func(s *distribution.ReleaseSnapshot) { s.WorkspaceID, _ = identity.NewWorkspaceID() },
		"relation count": func(s *distribution.ReleaseSnapshot) { s.Execution.Relations = s.Execution.Relations[:1] },
		"external locator": func(s *distribution.ReleaseSnapshot) {
			s.Execution.Relations[0].SourceLocator = "remote.example:5432/production"
		},
		"external schema":  func(s *distribution.ReleaseSnapshot) { s.Execution.Relations[0].Schema = "public" },
		"invalid identity": func(s *distribution.ReleaseSnapshot) { s.Execution.Relations[0].Fields[0].FieldID = "not-an-id" },
		"wrong type":       func(s *distribution.ReleaseSnapshot) { s.Execution.Relations[0].Fields[0].DataType = "integer" },
		"wrong source":     func(s *distribution.ReleaseSnapshot) { s.Execution.Relations[1].SourceRevisionID = "another-revision" },
		"duplicate field": func(s *distribution.ReleaseSnapshot) {
			s.Execution.Relations[0].Fields[1].FieldID = s.Execution.Relations[0].Fields[0].FieldID
		},
		"missing data asset": func(s *distribution.ReleaseSnapshot) {
			for i := range s.Assets {
				if s.Assets[i].AssetType == semantic.DataAsset {
					s.Assets = append(s.Assets[:i], s.Assets[i+1:]...)
					return
				}
			}
		},
		"mismatched source reference": func(s *distribution.ReleaseSnapshot) {
			for i := range s.Assets {
				if s.Assets[i].AssetType != semantic.DataAsset {
					continue
				}
				var content map[string]json.RawMessage
				_ = json.Unmarshal(s.Assets[i].Content, &content)
				var spec semantic.KnowledgeSpec
				_ = json.Unmarshal(content["spec"], &spec)
				revision, _ := identity.NewPhysicalFieldRevisionID()
				spec.Members[0].SourceFieldRef.RevisionID = revision.String()
				content["spec"], _ = json.Marshal(spec)
				s.Assets[i].Content, _ = json.Marshal(content)
				return
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			scenario := New()
			workspace, _ := identity.NewWorkspaceID()
			actor, _ := identity.NewPrincipalID()
			scenario.Snapshot.WorkspaceID = workspace
			mutate(&scenario.Snapshot)
			if statements, err := sourceStatements(workspace, actor, &scenario.Snapshot); err == nil || statements != nil {
				t.Fatalf("invalid fixture accepted: %v", err)
			}
		})
	}
}

func TestSourceDigestUsesCompleteSHA256(t *testing.T) {
	if got := sourceDigest([]byte("abc")); got != "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("wrong digest: %s", got)
	}
}

func TestPreviewMetadataPreservesDisclosureAndRejectsConflicts(t *testing.T) {
	initial := []byte(`{"synthetic":true,"provenance":"declared_local_fixture","comment":"synthetic-only","importedBy":"owner"}`)
	configured, existed, err := previewMetadata(initial)
	if err != nil || existed {
		t.Fatalf("initial configuration: %v, existed=%v", err, existed)
	}
	var value map[string]any
	if err := json.Unmarshal(configured, &value); err != nil {
		t.Fatal(err)
	}
	if value["host"] != "127.0.0.1" || value["port"] != float64(5433) || value["database"] != demoSourceSchema || value["username"] != demoSourceSchema || value["sslMode"] != "disable" || value["comment"] != "synthetic-only" || value["importedBy"] != "owner" {
		t.Fatalf("unexpected configuration: %v", value)
	}
	repeated, existed, err := previewMetadata(configured)
	if err != nil || !existed || !bytes.Equal(configured, repeated) {
		t.Fatalf("repeat changed metadata: %v, existed=%v", err, existed)
	}
	for _, raw := range []string{
		`{}`,
		`{"synthetic":false,"provenance":"declared_local_fixture"}`,
		`{"synthetic":true,"provenance":"external"}`,
		`{"synthetic":true,"provenance":"declared_local_fixture","host":"remote.example"}`,
		`{"synthetic":true,"provenance":"declared_local_fixture","host":"127.0.0.1"}`,
		`{"synthetic":true,"provenance":"declared_local_fixture","host":{}}`,
		`{"synthetic":true,"provenance":"declared_local_fixture","port":"5433"}`,
	} {
		if _, _, err := previewMetadata([]byte(raw)); err == nil {
			t.Errorf("accepted conflicting metadata: %s", raw)
		}
	}
}

func TestPreviewCredentialUsesBoundEncryptedEnvelope(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	source, _ := identity.NewSourceConnectionID()
	cipher, err := discoveryapp.NewCredentialCipher([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	password := "synthetic-demo-secret"
	nonce, ciphertext, keyVersion, err := cipher.Seal(workspace, source, 1, discovery.CredentialSecret{Password: password})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte(password)) || len(nonce) != 12 {
		t.Fatal("credential is not a proper encrypted envelope")
	}
	envelope := discovery.CredentialEnvelope{WorkspaceID: workspace, SourceConnectionID: source, Version: 1, KeyVersion: keyVersion, Algorithm: "AES-256-GCM", Nonce: nonce, Ciphertext: ciphertext}
	secret, err := cipher.Open(envelope)
	if err != nil || secret.Password != password {
		t.Fatalf("credential round trip failed: %v", err)
	}
	envelope.WorkspaceID, _ = identity.NewWorkspaceID()
	if _, err := cipher.Open(envelope); err == nil {
		t.Fatal("credential accepted for another workspace")
	}
}
