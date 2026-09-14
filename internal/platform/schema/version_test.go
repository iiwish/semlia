package schema

import (
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestVersionMatchesLatestMigration(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(file), "../../../migrations/*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	latest := 0
	for _, path := range paths {
		version, err := strconv.Atoi(strings.SplitN(filepath.Base(path), "_", 2)[0])
		if err != nil {
			t.Fatal(err)
		}
		if version > latest {
			latest = version
		}
	}
	if latest != MigrationVersion {
		t.Fatalf("migration version %d, latest migration %d", MigrationVersion, latest)
	}
}
