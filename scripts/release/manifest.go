package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/iiwish/semlia/internal/platform/schema"
	"go.yaml.in/yaml/v3"
)

type target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type manifest struct {
	FormatVersion    int    `json:"formatVersion"`
	Product          string `json:"product"`
	Version          string `json:"version"`
	Commit           string `json:"commit"`
	Target           target `json:"target"`
	Executable       string `json:"executable"`
	APIVersion       string `json:"apiVersion"`
	SchemaVersion    string `json:"schemaVersion"`
	SBOM             string `json:"sbom"`
	ChecksumFile     string `json:"checksumFile"`
	SigningSubject   string `json:"signingSubject"`
	MigrationVersion int    `json:"migrationVersion"`
	SourceDigest     string `json:"sourceDigest"`
	SourceDirty      bool   `json:"sourceDirty"`
	ArtifactKind     string `json:"artifactKind"`
	Acceptance       string `json:"acceptance"`
	BuildEnvironment string `json:"buildEnvironment"`
	GoVersion        string `json:"goVersion"`
}

type cycloneDX struct {
	BOMFormat   string            `json:"bomFormat"`
	SpecVersion string            `json:"specVersion"`
	Components  []json.RawMessage `json:"components"`
}

func main() {
	if len(os.Args) < 2 {
		fail(errors.New("expected manifest or verify subcommand"), 2)
	}

	var err error
	switch os.Args[1] {
	case "manifest":
		err = writeManifest(os.Args[2:])
	case "verify":
		err = verifyRelease(os.Args[2:])
	case "fingerprint":
		flags := flag.NewFlagSet("fingerprint", flag.ContinueOnError)
		root := flags.String("root", ".", "source root")
		if err = flags.Parse(os.Args[2:]); err == nil {
			var digest string
			digest, err = sourceFingerprint(*root)
			if err == nil {
				fmt.Println(digest)
			}
		}
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		fail(err, 1)
	}
}

func writeManifest(args []string) error {
	flags := flag.NewFlagSet("manifest", flag.ContinueOnError)
	output := flags.String("output", "", "manifest output path")
	version := flags.String("version", "", "release version")
	commit := flags.String("commit", "", "source commit")
	goos := flags.String("os", "", "target operating system")
	goarch := flags.String("arch", "", "target architecture")
	executable := flags.String("executable", "semlia", "executable name")
	sourceDigest := flags.String("source-digest", "", "build input fingerprint")
	sourceDirty := flags.Bool("source-dirty", false, "uncommitted workspace state")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *output == "" || *version == "" || *commit == "" || *goos == "" || *goarch == "" || !validDigest(*sourceDigest) {
		return errors.New("output, version, commit, os and arch are required")
	}

	file, err := os.Create(*output)
	if err != nil {
		return fmt.Errorf("create manifest: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest{
		FormatVersion:    1,
		Product:          "Semlia",
		Version:          *version,
		Commit:           *commit,
		Target:           target{OS: *goos, Arch: *goarch},
		Executable:       *executable,
		APIVersion:       "v1",
		SchemaVersion:    "0.9.0",
		SBOM:             "SBOM.cdx.json",
		ChecksumFile:     "SHA256SUMS",
		SigningSubject:   "SHA256SUMS",
		MigrationVersion: schema.MigrationVersion, SourceDigest: *sourceDigest, SourceDirty: *sourceDirty, ArtifactKind: "local_candidate", Acceptance: "unreviewed",
		BuildEnvironment: "GOWORK=off GOENV=off GOFLAGS= GOTOOLCHAIN=local CGO_ENABLED=0",
		GoVersion:        runtime.Version(),
	}); err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	return nil
}

func verifyRelease(args []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	manifestPath := flags.String("manifest", "", "release manifest path")
	sbomPath := flags.String("sbom", "", "CycloneDX SBOM path")
	checksumsPath := flags.String("checksums", "", "SHA-256 checksum file")
	archivePath := flags.String("archive", "", "release archive path")
	version := flags.String("version", "", "expected release version")
	commit := flags.String("commit", "", "expected source commit")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *manifestPath == "" || *sbomPath == "" || *checksumsPath == "" || *archivePath == "" || *version == "" || *commit == "" {
		return errors.New("manifest, sbom, checksums, archive, version and commit are required")
	}

	var release manifest
	if err := decodeJSON(*manifestPath, &release); err != nil {
		return err
	}
	if release.FormatVersion != 1 || release.Product != "Semlia" || release.Version != *version || release.Commit != *commit {
		return fmt.Errorf("release manifest metadata mismatch: version=%q commit=%q", release.Version, release.Commit)
	}
	if release.MigrationVersion != schema.MigrationVersion || !validDigest(release.SourceDigest) || release.ArtifactKind != "local_candidate" || release.Acceptance != "unreviewed" {
		return errors.New("release source/migration/candidate provenance is missing")
	}
	if release.SBOM != "SBOM.cdx.json" || release.ChecksumFile != "SHA256SUMS" || release.SigningSubject != "SHA256SUMS" {
		return errors.New("release manifest does not name the SBOM and signing subject")
	}

	var sbom cycloneDX
	if err := decodeJSON(*sbomPath, &sbom); err != nil {
		return err
	}
	if sbom.BOMFormat != "CycloneDX" || sbom.SpecVersion == "" || len(sbom.Components) == 0 {
		return fmt.Errorf("invalid or empty CycloneDX SBOM: format=%q components=%d", sbom.BOMFormat, len(sbom.Components))
	}
	goDependencies, webDependencies := false, false
	for _, raw := range sbom.Components {
		var component struct {
			PURL string `json:"purl"`
		}
		if json.Unmarshal(raw, &component) != nil {
			return errors.New("invalid SBOM component")
		}
		goDependencies = goDependencies || strings.HasPrefix(component.PURL, "pkg:golang/")
		webDependencies = webDependencies || strings.HasPrefix(component.PURL, "pkg:npm/")
	}
	if !goDependencies || !webDependencies {
		return errors.New("SBOM must include compiled Go and lockfile Web dependency graphs")
	}
	staging := filepath.Dir(*manifestPath)
	build, err := buildinfo.ReadFile(filepath.Join(staging, release.Executable))
	if err != nil {
		return err
	}
	if release.GoVersion == "" || release.GoVersion != build.GoVersion {
		return errors.New("recorded Go toolchain differs from executable build info")
	}
	if err := verifyDependencyCoverage(staging, release.Executable, sbom); err != nil {
		return err
	}
	if err := verifyArchiveInputs(*archivePath, staging, *sbomPath); err != nil {
		return err
	}

	checksums, err := readChecksums(*checksumsPath)
	if err != nil {
		return err
	}
	for _, path := range []string{*archivePath, *sbomPath} {
		want, ok := checksums[filepath.Base(path)]
		if !ok {
			return fmt.Errorf("checksum missing for %s", filepath.Base(path))
		}
		got, err := fileSHA256(path)
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("checksum mismatch for %s", filepath.Base(path))
		}
	}
	return nil
}

func verifyArchiveInputs(archive, staging, externalSBOM string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	seen := map[string]bool{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg {
			return errors.New("archive contains non-regular input")
		}
		name := strings.TrimPrefix(header.Name, filepath.Base(staging)+"/")
		if name == header.Name || !filepath.IsLocal(name) || seen[name] {
			return fmt.Errorf("invalid archive input path %q under %q (local=%t duplicate=%t)", header.Name, filepath.Base(staging), filepath.IsLocal(name), seen[name])
		}
		seen[name] = true
		hash := sha256.New()
		if _, err := io.Copy(hash, reader); err != nil {
			return err
		}
		want, err := fileSHA256(filepath.Join(staging, name))
		if err != nil {
			return err
		}
		if hex.EncodeToString(hash.Sum(nil)) != want {
			return fmt.Errorf("archive input mismatch: %s", name)
		}
	}
	if !seen["release.json"] || !seen["SBOM.cdx.json"] {
		return errors.New("archive provenance files missing")
	}
	if err := filepath.WalkDir(staging, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(staging, path)
		if err != nil {
			return err
		}
		if !seen[filepath.ToSlash(rel)] {
			return fmt.Errorf("archive missing staged input: %s", rel)
		}
		return nil
	}); err != nil {
		return err
	}
	inside, err := fileSHA256(filepath.Join(staging, "SBOM.cdx.json"))
	if err != nil {
		return err
	}
	outside, err := fileSHA256(externalSBOM)
	if err != nil {
		return err
	}
	if inside != outside {
		return errors.New("external SBOM differs from archive SBOM")
	}
	return nil
}

func dependencyComponents(sbom cycloneDX) (map[string]bool, error) {
	components := map[string]bool{}
	for _, raw := range sbom.Components {
		var component struct {
			PURL    string `json:"purl"`
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal(raw, &component); err != nil {
			return nil, err
		}
		purl, err := url.PathUnescape(strings.Split(component.PURL, "?")[0])
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(purl, "pkg:golang/") {
			pathVersion := strings.TrimPrefix(purl, "pkg:golang/")
			if component.Name == "" || component.Version == "" || !strings.HasSuffix(pathVersion, "@"+component.Version) || !strings.EqualFold(strings.TrimSuffix(pathVersion, "@"+component.Version), component.Name) {
				return nil, errors.New("Go SBOM component metadata disagrees with package URL")
			}
			components["pkg:golang/"+component.Name+"@"+component.Version] = true
			continue
		}
		components[purl] = true
	}
	return components, nil
}

func verifyDependencyCoverage(staging, executable string, sbom cycloneDX) error {
	components, err := dependencyComponents(sbom)
	if err != nil {
		return err
	}
	info, err := buildinfo.ReadFile(filepath.Join(staging, executable))
	if err != nil {
		return err
	}
	if err := verifyGoDependencies(info.Deps, components); err != nil {
		return err
	}
	payload, err := os.ReadFile(filepath.Join(staging, "dependency-inputs/pnpm-lock.yaml"))
	if err != nil {
		return err
	}
	return verifyWebDependencyCoverage(payload, components)
}

func verifyGoDependencies(dependencies []*debug.Module, components map[string]bool) error {
	for _, dependency := range dependencies {
		if dependency.Replace != nil {
			dependency = dependency.Replace
		}
		if dependency.Version == "" {
			return errors.New("unfingerprinted local Go replacement is forbidden")
		}
		if !components["pkg:golang/"+dependency.Path+"@"+dependency.Version] {
			return fmt.Errorf("SBOM missing compiled Go dependency %s@%s", dependency.Path, dependency.Version)
		}
	}
	return nil
}

func verifyWebDependencyCoverage(payload []byte, components map[string]bool) error {
	type snapshot struct {
		Dependencies map[string]string `yaml:"dependencies"`
		Optional     map[string]string `yaml:"optionalDependencies"`
	}
	var lock struct {
		Importers map[string]struct {
			Dependencies map[string]struct {
				Version string `yaml:"version"`
			} `yaml:"dependencies"`
		} `yaml:"importers"`
		Snapshots map[string]snapshot `yaml:"snapshots"`
	}
	if err := yaml.Unmarshal(payload, &lock); err != nil {
		return err
	}
	seen := map[string]bool{}
	var visit func(string, string) error
	visit = func(name, version string) error {
		if strings.HasPrefix(version, "link:") {
			return nil
		}
		key := name + "@" + version
		if seen[key] {
			return nil
		}
		seen[key] = true
		plain := strings.Split(version, "(")[0]
		if !components["pkg:npm/"+name+"@"+plain] {
			return fmt.Errorf("SBOM missing production Web dependency %s@%s", name, plain)
		}
		node, ok := lock.Snapshots[key]
		if !ok {
			return fmt.Errorf("lock snapshot missing: %s", key)
		}
		for child, version := range node.Dependencies {
			if err := visit(child, version); err != nil {
				return err
			}
		}
		for child, version := range node.Optional {
			if err := visit(child, version); err != nil {
				return err
			}
		}
		return nil
	}
	for _, importer := range []string{"web", "sdk/typescript"} {
		root, ok := lock.Importers[importer]
		if !ok {
			return errors.New("production lock importer missing")
		}
		for name, dependency := range root.Dependencies {
			if err := visit(name, dependency.Version); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	if err := json.NewDecoder(file).Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func readChecksums(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open checksums: %w", err)
	}
	defer file.Close()

	checksums := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 {
			return nil, fmt.Errorf("invalid checksum line %q", scanner.Text())
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return nil, fmt.Errorf("invalid checksum %q: %w", fields[0], err)
		}
		checksums[strings.TrimPrefix(fields[1], "*")] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	return checksums, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open artifact: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash artifact: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fail(err error, code int) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(code)
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	_, err := hex.DecodeString(value[7:])
	return err == nil
}

func sourceFingerprint(root string) (string, error) {
	inputs := []string{"cmd", "internal", "pkg", "migrations", "api", "sdk", "web", "scripts", "deploy", "go.mod", "go.sum", "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "compose.yaml", "Makefile", "LICENSE", "NOTICE", "docs/SECURITY.md"}
	type entry struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}
	files := []entry{}
	for _, input := range inputs {
		err := filepath.WalkDir(filepath.Join(root, input), func(path string, item os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if item.IsDir() {
				switch item.Name() {
				case "node_modules", "dist", "test-results", "playwright-report":
					return filepath.SkipDir
				}
				return nil
			}
			if item.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("build input symlinks require explicit packaging: %s", path)
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			digest, err := fileSHA256(path)
			if err != nil {
				return err
			}
			files = append(files, entry{filepath.ToSlash(rel), digest})
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	payload, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
