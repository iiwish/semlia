package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

func TestSBOMNormalizationPreservesEvidence(t *testing.T) {
	output, err := exec.Command("node", "--test", "normalize-sbom.test.mjs").CombinedOutput()
	if err != nil {
		t.Fatalf("SBOM normalization: %v\n%s", err, output)
	}
}

func TestReleaseRejectsUnpinnedToolchain(t *testing.T) {
	directory := t.TempDir()
	goPath := filepath.Join(directory, "go")
	stub := "#!/bin/sh\ncase \"$*\" in\n'mod edit -json') printf '{\"Toolchain\":\"go1.26.6\"}' ;;\n'env GOVERSION') printf go1.26.3 ;;\n*) printf test ;;\nesac\n"
	if err := os.WriteFile(goPath, []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", "build.sh")
	command.Env = append(os.Environ(), "GO="+goPath, "SEMLIA_RELEASE_DIR="+filepath.Join(directory, "release"))
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "Release requires go1.26.6") {
		t.Fatalf("unpinned toolchain was not rejected: %v\n%s", err, output)
	}
}

func TestManifestRequiresSourceFingerprintAndMarksCandidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release.json")
	args := []string{"-output", path, "-version", "test", "-commit", strings.Repeat("a", 40), "-os", "linux", "-arch", "amd64"}
	if err := writeManifest(args); err == nil {
		t.Fatal("HEAD without source fingerprint accepted")
	}
	if err := writeManifest(append(args, "-source-digest", "sha256:"+strings.Repeat("b", 64), "-source-dirty=true")); err != nil {
		t.Fatal(err)
	}
	var result manifest
	if err := decodeJSON(path, &result); err != nil {
		t.Fatal(err)
	}
	if result.MigrationVersion != 32 || !result.SourceDirty || result.ArtifactKind != "local_candidate" || result.Acceptance != "unreviewed" {
		t.Fatal("candidate represented as accepted or exact HEAD")
	}
}

func TestWebSBOMRequiresExactTransitiveProductionVersions(t *testing.T) {
	lock := []byte("importers:\n  web:\n    dependencies:\n      react:\n        version: 19.2.8\n  sdk/typescript:\n    dependencies: {}\nsnapshots:\n  react@19.2.8:\n    dependencies:\n      scheduler: 0.27.0\n  scheduler@0.27.0: {}\n")
	components := map[string]bool{"pkg:npm/react@19.2.8": true, "pkg:npm/scheduler@0.27.0": true}
	if err := verifyWebDependencyCoverage(lock, components); err != nil {
		t.Fatal(err)
	}
	delete(components, "pkg:npm/react@19.2.8")
	components["pkg:npm/react@19.2.7"] = true
	if err := verifyWebDependencyCoverage(lock, components); err == nil {
		t.Fatal("wrong React version accepted")
	}
	components["pkg:npm/react@19.2.8"] = true
	delete(components, "pkg:npm/scheduler@0.27.0")
	if err := verifyWebDependencyCoverage(lock, components); err == nil {
		t.Fatal("missing transitive production dependency accepted")
	}
}

func TestGoSBOMRequiresExactCompiledPGXVersion(t *testing.T) {
	dependencies := []*debug.Module{{Path: "github.com/jackc/pgx/v5", Version: "v5.8.0"}}
	components := map[string]bool{"pkg:golang/github.com/jackc/pgx/v5@v5.8.0": true}
	if err := verifyGoDependencies(dependencies, components); err != nil {
		t.Fatal(err)
	}
	delete(components, "pkg:golang/github.com/jackc/pgx/v5@v5.8.0")
	components["pkg:golang/github.com/jackc/pgx/v5@v5.7.0"] = true
	if err := verifyGoDependencies(dependencies, components); err == nil {
		t.Fatal("wrong pgx version accepted")
	}
	dependencies[0].Replace = &debug.Module{Path: "../external"}
	if err := verifyGoDependencies(dependencies, components); err == nil {
		t.Fatal("external unfingerprinted replacement accepted")
	}
}

func TestGoSBOMPreservesModuleCaseWithNormalizedPURL(t *testing.T) {
	sbom := cycloneDX{Components: []json.RawMessage{json.RawMessage(`{"name":"github.com/ProtonMail/go-crypto","version":"v1.1.6","purl":"pkg:golang/github.com/protonmail/go-crypto@v1.1.6"}`)}}
	components, err := dependencyComponents(sbom)
	if err != nil {
		t.Fatal(err)
	}
	dependencies := []*debug.Module{{Path: "github.com/ProtonMail/go-crypto", Version: "v1.1.6"}}
	if err := verifyGoDependencies(dependencies, components); err != nil {
		t.Fatal(err)
	}
	dependencies[0].Version = "v1.1.5"
	if err := verifyGoDependencies(dependencies, components); err == nil {
		t.Fatal("incorrect version accepted")
	}
	dependencies[0].Version, dependencies[0].Path = "v1.1.6", "github.com/protonmail/go-crypto"
	if err := verifyGoDependencies(dependencies, components); err == nil {
		t.Fatal("incorrect module name casing accepted")
	}
	sbom.Components[0] = json.RawMessage(`{"name":"github.com/ProtonMail/go-crypto","version":"v1.1.5","purl":"pkg:golang/github.com/protonmail/go-crypto@v1.1.6"}`)
	if _, err := dependencyComponents(sbom); err == nil {
		t.Fatal("conflicting component and URL versions accepted")
	}
}

func TestArchiveProvenanceMatchesVerifiedInputs(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "bundle")
	if err := os.MkdirAll(staging, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"release.json", "SBOM.cdx.json"} {
		if err := os.WriteFile(filepath.Join(staging, name), []byte("expected"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(root, "bundle.tar.gz")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(file)
	writer := tar.NewWriter(compressed)
	for _, name := range []string{"release.json", "SBOM.cdx.json"} {
		if err := writer.WriteHeader(&tar.Header{Name: "bundle/" + name, Mode: 0600, Size: 8}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte("expected")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	sbom := filepath.Join(staging, "SBOM.cdx.json")
	if err := verifyArchiveInputs(archive, staging, sbom); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "release.json"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyArchiveInputs(archive, staging, sbom); err == nil {
		t.Fatal("different archive manifest accepted")
	}
}

func TestMacOSArchiveExcludesUnfingerprintedAppleDouble(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS packaging metadata")
	}
	root := t.TempDir()
	staging := filepath.Join(root, "bundle")
	if err := os.MkdirAll(staging, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"release.json", "SBOM.cdx.json"} {
		if err := os.WriteFile(filepath.Join(staging, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if output, err := exec.Command("xattr", "-w", "com.semlia.fixture", "metadata-not-in-source", staging).CombinedOutput(); err != nil {
		t.Fatalf("create fixture xattr: %s %v", output, err)
	}
	archive := filepath.Join(root, "bundle.tar.gz")
	command := exec.Command("tar", "-czf", archive, "-C", root, "bundle")
	command.Env = append(os.Environ(), "COPYFILE_DISABLE=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("package fixture: %s %v", output, err)
	}
	if err := verifyArchiveInputs(archive, staging, filepath.Join(staging, "SBOM.cdx.json")); err != nil {
		t.Fatal(err)
	}
}

func TestFingerprintIncludesDirtyWebAndEmbeddedBytes(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"cmd", "internal/platform/web/static", "pkg", "migrations", "api", "sdk", "web/src", "scripts", "deploy", "docs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"go.mod", "go.sum", "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "compose.yaml", "Makefile", "LICENSE", "NOTICE", "docs/SECURITY.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	first, err := sourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "web/src/changed.tsx"), []byte("uncommitted web"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := sourceFingerprint(root)
	if err != nil || first == second {
		t.Fatal("dirty Web ignored")
	}
	if err := os.WriteFile(filepath.Join(root, "internal/platform/web/static/index.html"), []byte("actual embedded bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	third, err := sourceFingerprint(root)
	if err != nil || second == third {
		t.Fatal("embedded artifact ignored")
	}
	if err := os.MkdirAll(filepath.Join(root, "web/dist"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "web/dist/output.json"), json.RawMessage(`{"generated":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	fourth, err := sourceFingerprint(root)
	if err != nil || third != fourth {
		t.Fatal("generated output created fingerprint cycle")
	}
}
