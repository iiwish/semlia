package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/iiwish/semlia/internal/application"
)

func TestConfiguredReadinessProbeWithoutDatabaseIsUnavailable(t *testing.T) {
	probe, closeProbe, err := configuredReadinessProbe(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer closeProbe()
	if err := probe.Check(context.Background()); !errors.Is(err, application.ErrDependencyUnavailable) {
		t.Fatalf("readiness error = %v", err)
	}
}

func TestConfiguredReadinessProbeRejectsInvalidURL(t *testing.T) {
	if _, _, err := configuredReadinessProbe(context.Background(), "://database-secret"); err == nil {
		t.Fatal("invalid database URL was accepted")
	}
}

func TestServerDoesNotEchoInvalidDatabaseURL(t *testing.T) {
	const databaseURL = "://database-secret"
	lookup := mapEnvironment(map[string]string{"SEMLIA_DATABASE_URL": databaseURL})
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"server"}, lookup, ioDiscard{}, &stderr); code != 1 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if bytes.Contains(stderr.Bytes(), []byte("database-secret")) {
		t.Fatalf("stderr leaked database URL: %s", stderr.String())
	}
}
