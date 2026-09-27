package smoke

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

type smokeIsolation struct {
	Version             int    `json:"version"`
	Root                string `json:"root"`
	Owner               string `json:"owner"`
	Project             string `json:"project"`
	Image               string `json:"image"`
	DockerHost          string `json:"dockerHost"`
	DockerConfig        string `json:"dockerConfig"`
	HTTPURL             string `json:"httpURL"`
	EnvFile             string `json:"envFile"`
	OwnershipFile       string `json:"ownershipFile"`
	EnvFileDigest       string `json:"envFileDigest"`
	OwnershipFileDigest string `json:"ownershipFileDigest"`
	DockerConfigDigest  string `json:"dockerConfigDigest"`
}

func privateSmokePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return errors.New("smoke isolation file is unavailable")
	}
	wantMode := os.FileMode(0600)
	if directory {
		wantMode = 0700
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) || info.Mode().Perm() != wantMode || info.IsDir() != directory || (!directory && (!info.Mode().IsRegular() || info.Size() > 128<<10)) {
		return errors.New("smoke isolation file ownership or mode is invalid")
	}
	return nil
}

func loadSmokeIsolation(root string) (smokeIsolation, error) {
	var proof smokeIsolation
	path := os.Getenv("SEMLIA_SMOKE_CONTEXT")
	dir := filepath.Dir(path)
	if path == "" || filepath.Base(path) != "context.json" || filepath.Dir(dir) != "/tmp" || !strings.HasPrefix(filepath.Base(dir), "semlia-smoke.") {
		return proof, errors.New("explicit private smoke isolation context is required")
	}
	for _, entry := range []struct {
		path      string
		directory bool
	}{{dir, true}, {path, false}} {
		if err := privateSmokePath(entry.path, entry.directory); err != nil {
			return proof, err
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return proof, errors.New("read smoke isolation context failed")
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proof); err != nil {
		return proof, errors.New("invalid smoke isolation context")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return proof, errors.New("invalid trailing smoke isolation context")
	}
	if proof.Version != 1 || proof.Root != root || !filepath.IsAbs(root) || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(proof.Owner) {
		return proof, errors.New("smoke isolation identity mismatch")
	}
	if !regexp.MustCompile(`^semlia-smoke-[0-9]+-[0-9]+-`+proof.Owner[:8]+`$`).MatchString(proof.Project) || proof.Image != "semlia:smoke-"+proof.Owner {
		return proof, errors.New("smoke isolation project or image mismatch")
	}
	host, err := url.Parse(proof.DockerHost)
	if err != nil || host.Scheme != "unix" || host.Host != "" || !filepath.IsAbs(host.Path) || host.RawQuery != "" || host.Fragment != "" {
		return proof, errors.New("smoke requires a local Unix Docker endpoint")
	}
	endpoint, err := url.Parse(proof.HTTPURL)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" || proof.HTTPURL != os.Getenv("SEMLIA_SMOKE_URL") {
		return proof, errors.New("smoke HTTP endpoint mismatch")
	}
	port, err := strconv.Atoi(endpoint.Port())
	if err != nil || port < 1 || port > 65535 {
		return proof, errors.New("invalid smoke HTTP port")
	}
	if proof.DockerConfig != filepath.Join(dir, "docker") || proof.EnvFile != filepath.Join(dir, "smoke.env") || proof.OwnershipFile != filepath.Join(dir, "ownership.yaml") {
		return proof, errors.New("smoke private configuration path mismatch")
	}
	if err := privateSmokePath(proof.DockerConfig, true); err != nil {
		return proof, err
	}
	for _, entry := range []struct{ path, digest string }{
		{proof.EnvFile, proof.EnvFileDigest}, {proof.OwnershipFile, proof.OwnershipFileDigest}, {filepath.Join(proof.DockerConfig, "config.json"), proof.DockerConfigDigest},
	} {
		if err := privateSmokePath(entry.path, false); err != nil {
			return proof, err
		}
		body, err := os.ReadFile(entry.path)
		if err != nil {
			return proof, errors.New("read smoke private configuration failed")
		}
		digest := sha256.Sum256(body)
		if hex.EncodeToString(digest[:]) != entry.digest {
			return proof, errors.New("smoke private configuration fingerprint changed")
		}
	}
	return proof, nil
}

func (proof smokeIsolation) composePrefix() []string {
	return []string{"compose", "--project-directory", proof.Root, "--project-name", proof.Project, "--env-file", proof.EnvFile, "-f", filepath.Join(proof.Root, "compose.yaml"), "-f", filepath.Join(proof.Root, "compose.override.yaml"), "-f", proof.OwnershipFile}
}

func composeCommand(root string, args ...string) (string, []string, error) {
	proof, err := loadSmokeIsolation(root)
	if err != nil {
		return "", nil, err
	}
	return "docker", append(proof.composePrefix(), args...), nil
}

func cleanSmokeEnvironment(proof *smokeIsolation) []string {
	result := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "TMPDIR=/tmp", "LC_ALL=C"}
	if proof != nil {
		result = append(result, "BUILDX_CONFIG="+filepath.Join(filepath.Dir(proof.EnvFile), "buildx"))
	}
	return result
}

func (proof smokeIsolation) dockerOutput(ctx context.Context, args ...string) (string, error) {
	args = append([]string{"--config", proof.DockerConfig, "--host", proof.DockerHost}, args...)
	return runSmokeCommand(ctx, proof.Root, cleanSmokeEnvironment(&proof), "docker", args...)
}

// Check both project labels and exact configured names: Compose may reuse or
// remove an existing named volume even when its project label is missing.
func (proof smokeIsolation) verifyOwned(ctx context.Context) error {
	for _, kind := range []string{"container", "network", "volume"} {
		args := []string{kind, "ls", "--quiet", "--filter", "label=com.docker.compose.project=" + proof.Project}
		if kind == "container" {
			args = append(args, "--all")
		}
		output, err := proof.dockerOutput(ctx, args...)
		if err != nil {
			return errors.New("cannot inventory smoke resource ownership")
		}
		ids := strings.Fields(output)
		var names []string
		if kind == "volume" {
			names = []string{"postgres-data", "git-content", "artifact-data"}
		}
		if kind == "network" {
			names = []string{"backend"}
		}
		for _, name := range names {
			output, err := proof.dockerOutput(ctx, kind, "ls", "--quiet", "--filter", "name=^"+proof.Project+"_"+name+"$")
			if err != nil {
				return errors.New("cannot inventory configured smoke resource")
			}
			ids = append(ids, strings.Fields(output)...)
		}
		format := `{{ index .Labels "io.semlia.smoke.owner" }}|{{ index .Labels "com.docker.compose.project" }}`
		if kind == "container" {
			format = `{{ index .Config.Labels "io.semlia.smoke.owner" }}|{{ index .Config.Labels "com.docker.compose.project" }}`
		}
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			labels, err := proof.dockerOutput(ctx, kind, "inspect", "--format", format, id)
			if err != nil || strings.TrimSpace(labels) != proof.Owner+"|"+proof.Project {
				return errors.New("smoke resource ownership mismatch; mutation refused")
			}
		}
	}
	labels, err := proof.dockerOutput(ctx, "image", "inspect", "--format", `{{ index .Config.Labels "io.semlia.smoke.owner" }}|{{ index .Config.Labels "io.semlia.smoke.project" }}`, proof.Image)
	if err != nil || strings.TrimSpace(labels) != proof.Owner+"|"+proof.Project {
		return errors.New("smoke image ownership mismatch; mutation refused")
	}
	return nil
}

func isolatedDockerOutput(ctx context.Context, root string, args ...string) (string, error) {
	proof, err := loadSmokeIsolation(root)
	if err != nil {
		return "", err
	}
	if len(args) == 0 {
		return "", errors.New("missing smoke Docker operation")
	}
	if args[0] == "compose" {
		prefix := proof.composePrefix()
		if len(args) <= len(prefix) || !reflect.DeepEqual(args[:len(prefix)], prefix) {
			return "", errors.New("smoke Compose context override refused")
		}
		for _, arg := range args[len(prefix):] {
			for _, forbidden := range []string{"-f", "-p", "--file", "--project-name", "--project-directory", "--env-file", "--profile", "--host", "--config", "--context"} {
				if arg == forbidden || strings.HasPrefix(arg, forbidden+"=") || (len(forbidden) == 2 && strings.HasPrefix(arg, forbidden)) {
					return "", errors.New("smoke Compose context override refused")
				}
			}
		}
		switch args[len(prefix)] {
		case "ps":
		case "exec", "restart", "stop", "start", "down", "up":
			if err := proof.verifyOwned(ctx); err != nil {
				return "", err
			}
		default:
			return "", errors.New("unsupported smoke Compose operation")
		}
	} else if len(args) < 2 || !contains([]string{"container", "network", "volume"}, args[0]) || !contains([]string{"ls", "inspect"}, args[1]) {
		return "", fmt.Errorf("unsupported smoke Docker operation")
	}
	return proof.dockerOutput(ctx, args...)
}
