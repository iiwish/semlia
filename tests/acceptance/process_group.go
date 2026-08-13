package acceptance

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const acceptanceCommandWaitDelay = 2 * time.Second

func acceptanceCommandContext(ctx context.Context, environment []string, name string, args ...string) (*exec.Cmd, error) {
	path, err := acceptanceExecutablePath(environment, name)
	if err != nil {
		return nil, err
	}
	command := newAcceptanceCommandContext(ctx, path, args...)
	command.Env = environment
	return command, nil
}

func acceptanceExecutablePath(environment []string, name string) (string, error) {
	if strings.ContainsAny(name, `/\`) {
		if !filepath.IsAbs(name) {
			return "", fmt.Errorf("resolve acceptance command %q: explicit command path must be absolute", name)
		}
		return name, nil
	}
	path := environmentValue(environment, "PATH")
	if path == "" {
		return "", fmt.Errorf("resolve acceptance command %q: isolated PATH is empty", name)
	}
	for _, directory := range filepath.SplitList(path) {
		if !filepath.IsAbs(directory) {
			continue
		}
		for _, candidate := range acceptanceExecutableCandidates(directory, name) {
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() {
				continue
			}
			if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
				continue
			}
			return candidate, nil
		}
	}
	return "", fmt.Errorf("resolve acceptance command %q in isolated PATH %q: %w", name, path, exec.ErrNotFound)
}

func acceptanceExecutableCandidates(directory, name string) []string {
	base := filepath.Join(directory, name)
	if runtime.GOOS != "windows" || filepath.Ext(name) != "" {
		return []string{base}
	}
	return []string{base + ".com", base + ".exe", base + ".bat", base + ".cmd"}
}
