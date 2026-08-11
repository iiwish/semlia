package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type manifest struct {
	FormatVersion  int    `json:"formatVersion"`
	Product        string `json:"product"`
	Version        string `json:"version"`
	Commit         string `json:"commit"`
	Target         target `json:"target"`
	Executable     string `json:"executable"`
	APIVersion     string `json:"apiVersion"`
	SchemaVersion  string `json:"schemaVersion"`
	SBOM           string `json:"sbom"`
	ChecksumFile   string `json:"checksumFile"`
	SigningSubject string `json:"signingSubject"`
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
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *output == "" || *version == "" || *commit == "" || *goos == "" || *goarch == "" {
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
		FormatVersion:  1,
		Product:        "Semlia",
		Version:        *version,
		Commit:         *commit,
		Target:         target{OS: *goos, Arch: *goarch},
		Executable:     *executable,
		APIVersion:     "v1",
		SchemaVersion:  "0.1.0",
		SBOM:           "SBOM.cdx.json",
		ChecksumFile:   "SHA256SUMS",
		SigningSubject: "SHA256SUMS",
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
