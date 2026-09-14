package discovery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	domain "github.com/iiwish/semlia/internal/domain/discovery"
	"golang.org/x/sys/unix"
)

const maxArtifactBytes = 8 << 20

type ArtifactLoader struct {
	rootPath string
}

func NewArtifactLoader(rootPath string) (*ArtifactLoader, error) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return &ArtifactLoader{}, nil
	}
	absolute, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, domain.ErrArtifactPath
	}
	root, err := unix.Open(absolute, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, domain.ErrArtifactPath
	}
	_ = unix.Close(root)
	return &ArtifactLoader{rootPath: absolute}, nil
}

func (loader *ArtifactLoader) Load(paths []string) (map[string][]byte, error) {
	return loader.LoadContext(context.Background(), paths)
}

func (loader *ArtifactLoader) LoadContext(ctx context.Context, paths []string) (map[string][]byte, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if loader == nil || loader.rootPath == "" {
		return nil, domain.ErrArtifactPath
	}
	normalized, err := normalizeArtifactPaths(paths)
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte, len(normalized))
	total := 0
	for _, clean := range normalized {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file, err := openArtifactNoFollow(loader.rootPath, clean)
		if err != nil {
			return nil, domain.ErrArtifactPath
		}
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maxArtifactBytes {
			_ = file.Close()
			return nil, domain.ErrArtifactPath
		}
		contents, readErr := io.ReadAll(io.LimitReader(file, int64(maxArtifactBytes-total)+1))
		_ = file.Close()
		total += len(contents)
		if readErr != nil || len(contents) == 0 || total > maxArtifactBytes {
			return nil, domain.ErrArtifactPath
		}
		files[path.Clean(clean)] = contents
	}
	return files, nil
}

func openArtifactNoFollow(rootPath, name string) (*os.File, error) {
	current, err := unix.Open(rootPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, domain.ErrArtifactPath
	}
	components := strings.Split(name, "/")
	for index, component := range components {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if index < len(components)-1 {
			flags |= unix.O_DIRECTORY
		} else {
			flags |= unix.O_NONBLOCK
		}
		next, openErr := unix.Openat(current, component, flags, 0)
		_ = unix.Close(current)
		if openErr != nil {
			return nil, domain.ErrArtifactPath
		}
		current = next
	}
	return os.NewFile(uintptr(current), path.Base(name)), nil
}

func normalizeArtifactPaths(paths []string) ([]string, error) {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, requested := range paths {
		requested = strings.TrimSpace(requested)
		clean := path.Clean(requested)
		if clean != requested || clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") ||
			strings.Contains(clean, "\\") || strings.IndexFunc(clean, func(value rune) bool { return unicode.IsControl(value) || unicode.In(value, unicode.Cf) }) >= 0 ||
			!strings.EqualFold(path.Ext(clean), ".sql") {
			return nil, domain.ErrArtifactPath
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	sort.Strings(result)
	if len(result) > 200 {
		return nil, errors.New("too many SQL artifacts")
	}
	return result, nil
}

func artifactReadError(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("read configured SQL artifact %q: %w", path.Base(name), domain.ErrArtifactPath)
}
