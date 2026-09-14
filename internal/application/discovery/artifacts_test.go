package discovery

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/iiwish/semlia/internal/domain/discovery"
	"golang.org/x/sys/unix"
)

func TestArtifactLoaderAllowsVersionedSQLAndRejectsEscapeAndSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.sql")
	if err := os.WriteFile(outside, []byte("SELECT 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "v1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "v1", "schema.sql"), []byte("CREATE TABLE x(id int)"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.sql")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	loader, err := NewArtifactLoader(root)
	if err != nil {
		t.Fatal(err)
	}
	files, err := loader.Load([]string{"v1/schema.sql"})
	if err != nil || string(files["v1/schema.sql"]) != "CREATE TABLE x(id int)" {
		t.Fatalf("files = %#v, %v", files, err)
	}
	for _, path := range []string{"../outside.sql", "v1/../schema.sql", outside, "linked.sql", "linked-dir/outside.sql", "v1/schema.txt", "v1/schema\u202esql"} {
		if _, err := loader.Load([]string{path}); !errors.Is(err, domain.ErrArtifactPath) {
			t.Fatalf("%q error = %v", path, err)
		}
	}
}

func TestArtifactLoaderRejectsFIFOWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	if err := unix.Mkfifo(filepath.Join(root, "stream.sql"), 0o600); err != nil {
		t.Skipf("create fifo: %v", err)
	}
	loader, err := NewArtifactLoader(root)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, loadErr := loader.Load([]string{"stream.sql"})
		done <- loadErr
	}()
	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrArtifactPath) {
			t.Fatalf("fifo error=%v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("configured-root loader blocked on fifo")
	}
}

func TestArtifactLoaderRequiresConfiguredRoot(t *testing.T) {
	loader, err := NewArtifactLoader("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load([]string{"schema.sql"}); !errors.Is(err, domain.ErrArtifactPath) {
		t.Fatalf("error = %v", err)
	}
}
