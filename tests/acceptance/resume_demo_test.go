package acceptance_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestResumeDemoDataAndSafety(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	root := filepath.Join("..", "..")
	args := []string{"--test"}
	for _, pattern := range []string{"examples/resume-demo/*.test.mjs", "scripts/demo/*.test.mjs"} {
		files, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil || len(files) == 0 {
			t.Fatalf("missing demo tests: %s", pattern)
		}
		for _, file := range files {
			rel, err := filepath.Rel(root, file)
			if err != nil {
				t.Fatal(err)
			}
			args = append(args, rel)
		}
	}
	command := exec.CommandContext(ctx, "node", args...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("demo data and safety tests: %v\n%s", err, output)
	}
}
