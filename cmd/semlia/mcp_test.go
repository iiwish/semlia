package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestMCPCommandValidatesClientEnvironmentWithoutServerConfig(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"mcp"}, emptyEnvironment, ioDiscard{}, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "machine client configuration is invalid") {
		t.Fatalf("code=%d stderr=%s", code, &stderr)
	}
}
