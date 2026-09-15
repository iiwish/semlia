package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDoctorValidatesConfiguration(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"doctor"}, passwordTestEnvironment(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if stdout.String() != "configuration ok\n" {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestDoctorDoesNotEchoInvalidProductionSecrets(t *testing.T) {
	values := map[string]string{
		"SEMLIA_ENV":          "production",
		"SEMLIA_DATABASE_URL": "postgres://user:database-secret@example.internal/semlia?sslmode=disable",
		"SEMLIA_SECRET_KEY":   "secret-value",
	}
	lookup := func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"doctor"}, lookup, ioDiscard{}, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	for _, leaked := range []string{"database-secret", "secret-value"} {
		if strings.Contains(stderr.String(), leaked) {
			t.Errorf("stderr leaked %q: %s", leaked, stderr.String())
		}
	}
}

func TestUnknownCommandPrintsUsage(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"unknown"}, emptyEnvironment, ioDiscard{}, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "server|doctor|worker|migrate") {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
}

func TestMigrateRequiresDatabaseConfiguration(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"migrate", "up"}, emptyEnvironment, ioDiscard{}, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "SEMLIA_DATABASE_URL") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWorkerRequiresDatabaseConfiguration(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"worker"}, emptyEnvironment, ioDiscard{}, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "SEMLIA_DATABASE_URL") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWorkerJSONFailureLogIsStructuredAndSafe(t *testing.T) {
	lookup := passwordTestEnvironment(map[string]string{
		"SEMLIA_DATABASE_URL": "://database-secret",
		"SEMLIA_LOG_FORMAT":   "json",
	})
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"worker"}, lookup, ioDiscard{}, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	lines := bytes.Split(bytes.TrimSpace(stderr.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("log lines = %d, want 2: %s", len(lines), stderr.String())
	}
	var stopped map[string]any
	if err := json.Unmarshal(lines[1], &stopped); err != nil {
		t.Fatalf("decode worker log: %v", err)
	}
	if stopped["msg"] != "worker stopped" || stopped["error_code"] != "OPERATION_FAILED" {
		t.Fatalf("worker log = %#v", stopped)
	}
	if strings.Contains(stderr.String(), "database-secret") {
		t.Fatalf("worker log leaked database URL: %s", stderr.String())
	}
}

func TestMigrateRejectsUnknownAction(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"migrate", "force"}, emptyEnvironment, ioDiscard{}, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "migrate <up|down|version>") {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
}

func emptyEnvironment(string) (string, bool) {
	return "", false
}

func passwordTestEnvironment(overrides map[string]string) func(string) (string, bool) {
	values := map[string]string{"SEMLIA_DATABASE_URL": "postgres://localhost/semlia_test", "SEMLIA_SECRET_KEY": strings.Repeat("s", 64), "SEMLIA_ALLOWED_ORIGINS": "http://127.0.0.1:18081"}
	for key, value := range overrides {
		values[key] = value
	}
	return mapEnvironment(values)
}

type ioDiscard struct{}

func (ioDiscard) Write(body []byte) (int, error) {
	return len(body), nil
}
