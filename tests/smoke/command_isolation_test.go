package smoke

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolationFixture(t *testing.T) (map[string]any, string) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "semlia-smoke.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Mkdir(filepath.Join(dir, "docker"), 0700); err != nil {
		t.Fatal(err)
	}
	owner := strings.Repeat("a", 32)
	value := map[string]any{"version": 1, "root": repositoryRoot(t), "owner": owner, "project": "semlia-smoke-1-2-" + owner[:8], "image": "semlia:smoke-" + owner, "dockerHost": "unix:///tmp/semlia-smoke-test.sock", "dockerConfig": filepath.Join(dir, "docker"), "httpURL": "http://127.0.0.1:19999", "envFile": filepath.Join(dir, "smoke.env"), "ownershipFile": filepath.Join(dir, "ownership.yaml")}
	for key, body := range map[string]string{"envFile": "SYNTHETIC_ONLY=1\n", "ownershipFile": "x-smoke-owner: synthetic\n", "dockerConfig": `{"auths":{"https://index.docker.io/v1/":{}},"cliPluginsExtraDirs":[]}`} {
		path := value[key].(string)
		if key == "dockerConfig" {
			path = filepath.Join(path, "config.json")
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte(body))
		value[key+"Digest"] = hex.EncodeToString(digest[:])
	}
	path := filepath.Join(dir, "context.json")
	body, _ := json.Marshal(value)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SEMLIA_SMOKE_CONTEXT", path)
	t.Setenv("SEMLIA_SMOKE_URL", value["httpURL"].(string))
	return value, dir
}

func TestSmokeIsolationMissingContextRefusesDocker(t *testing.T) {
	t.Setenv("SEMLIA_SMOKE_CONTEXT", "")
	bin := t.TempDir()
	marker := filepath.Join(bin, "called")
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	_, err := commandOutput(context.Background(), repositoryRoot(t), "docker", "volume", "ls")
	if err == nil {
		t.Fatal("missing isolation context must fail before Docker executes")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("Docker executed without an isolation context")
	}
}

func TestSmokeIsolationComposePinsContext(t *testing.T) {
	proof, _ := isolationFixture(t)
	t.Setenv("COMPOSE_FILE", "unrelated-private-overlay")
	t.Setenv("COMPOSE_PROJECT_NAME", "unrelated-project")
	name, args, err := composeCommand(repositoryRoot(t), "ps", "--all")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"compose", "--project-directory", repositoryRoot(t), "--project-name", proof["project"].(string), "--env-file", proof["envFile"].(string), "-f", filepath.Join(repositoryRoot(t), "compose.yaml"), "-f", filepath.Join(repositoryRoot(t), "compose.override.yaml"), "-f", proof["ownershipFile"].(string), "ps", "--all"}
	if name != "docker" || strings.Join(args, "\n") != strings.Join(want, "\n") {
		t.Fatal("nested Compose did not preserve the exact isolated project/files")
	}
}

func fakeIsolationDocker(t *testing.T, proof map[string]any, foreign bool) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	owner := proof["owner"].(string)
	if foreign {
		owner = "foreign"
	}
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> '" + log + "'\n" +
		"if [ -n \"${COMPOSE_FILE-}${DOCKER_CONTEXT-}${PRIVATE_PROVIDER_KEY-}${SEMLIA_EXECUTION_SOURCES-}\" ]; then exit 98; fi\n" +
		"[ \"$1\" = --config ] && [ \"$2\" = '" + proof["dockerConfig"].(string) + "' ] && [ \"$3\" = --host ] && [ \"$4\" = '" + proof["dockerHost"].(string) + "' ] || exit 99\nshift 4\n" +
		"case \"$1:$2\" in\nvolume:ls) case \"$*\" in *'name=^'*) echo hidden-named-volume;; esac;;\n*:inspect) echo '" + owner + "|" + proof["project"].(string) + "';;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	return log
}

func TestSmokeIsolationRejectsInvalidReceiptsBeforeDocker(t *testing.T) {
	for _, scenario := range []string{"root", "endpoint", "remote", "project", "owner", "env-digest", "config-digest", "file-mode", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			proof, dir := isolationFixture(t)
			log := fakeIsolationDocker(t, proof, false)
			switch scenario {
			case "root":
				proof["root"] = "/unrelated"
			case "endpoint":
				t.Setenv("SEMLIA_SMOKE_URL", "http://127.0.0.1:19998")
			case "remote":
				proof["dockerHost"] = "ssh://remote"
			case "project":
				proof["project"] = "unrelated"
			case "owner":
				proof["owner"] = ""
			case "env-digest":
				proof["envFileDigest"] = strings.Repeat("0", 64)
			case "config-digest":
				proof["dockerConfigDigest"] = strings.Repeat("0", 64)
			case "file-mode":
				if err := os.Chmod(proof["envFile"].(string), 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				path := proof["envFile"].(string)
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".original", path); err != nil {
					t.Fatal(err)
				}
			}
			body, _ := json.Marshal(proof)
			if err := os.WriteFile(filepath.Join(dir, "context.json"), body, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := composeOutput(context.Background(), repositoryRoot(t), "down", "--volumes"); err == nil {
				t.Fatal("invalid isolation proof allowed mutation")
			}
			if _, err := os.Stat(log); !os.IsNotExist(err) {
				t.Fatal("invalid proof reached Docker")
			}
		})
	}
}

func TestSmokeIsolationAllNestedMutationsAndFailureCleanupCheckOwnership(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "owned", true: "foreign-named-volume"}[foreign], func(t *testing.T) {
			proof, _ := isolationFixture(t)
			log := fakeIsolationDocker(t, proof, foreign)
			for _, key := range []string{"COMPOSE_FILE", "DOCKER_CONTEXT", "PRIVATE_PROVIDER_KEY", "SEMLIA_EXECUTION_SOURCES"} {
				t.Setenv(key, "synthetic-contamination")
			}
			for _, operation := range []string{"exec", "restart", "stop", "start", "down", "up"} {
				_, err := composeOutput(context.Background(), repositoryRoot(t), operation)
				if (err != nil) != foreign {
					t.Fatalf("operation %s ownership outcome mismatch: %v", operation, err)
				}
			}
			if foreign {
				if err := cleanupPostgresFailure(repositoryRoot(t), workerContainerState{}); err == nil {
					t.Fatal("failure cleanup ignored unknown ownership")
				}
			}
			body, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), " compose ") == foreign {
				t.Fatal("mutation did not respect ownership verification")
			}
		})
	}
}

func TestSmokeIsolationRejectsComposeAndRawDockerOverrides(t *testing.T) {
	proof, _ := isolationFixture(t)
	log := fakeIsolationDocker(t, proof, false)
	for _, args := range [][]string{{"compose", "--env-file", "/unrelated", "down"}, {"container", "rm", "unrelated"}, {"--host", "ssh://remote", "volume", "ls"}} {
		if _, err := commandOutput(context.Background(), repositoryRoot(t), "docker", args...); err == nil {
			t.Fatal("Docker override was accepted")
		}
	}
	for _, override := range []string{"--project-name=unrelated", "-punrelated", "-f/unrelated"} {
		if _, err := composeOutput(context.Background(), repositoryRoot(t), "down", override); err == nil {
			t.Fatal("nested Compose override was accepted")
		}
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("Docker override reached CLI")
	}
}

// The Node launcher fixture invokes the real Go helpers against a fake Docker
// CLI. Normal unit/runtime runs cannot accidentally opt into this fixture.
func TestSmokeIsolationLauncherPropagation(t *testing.T) {
	if os.Getenv("SEMLIA_SMOKE_LAUNCHER_TEST") != "1" {
		t.Skip("launcher fixture only")
	}
	body, err := os.ReadFile(os.Getenv("SEMLIA_SMOKE_CONTEXT"))
	if err != nil {
		t.Fatal("launcher did not pass its isolation context")
	}
	var header struct {
		Root string `json:"root"`
	}
	if err := json.Unmarshal(body, &header); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPOSE_FILE", "synthetic-nested-contamination")
	t.Setenv("DOCKER_HOST", "ssh://unrelated")
	proof, err := loadSmokeIsolation(header.Root)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"ps", "--all"}, {"restart", "worker"}, {"stop", "postgres"}, {"start", "postgres"}, {"exec", "-T", "postgres", "true"}, {"down", "--remove-orphans"}, {"up", "--detach", "--wait"}} {
		if _, err := composeOutput(context.Background(), header.Root, args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := commandOutput(context.Background(), header.Root, "docker", "container", "inspect", "--format", "{{.Name}}", "container-owned"); err != nil {
		t.Fatal(err)
	}
	if err := proof.verifyOwned(context.Background()); err != nil {
		t.Fatal(err)
	}
}
