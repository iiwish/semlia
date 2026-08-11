package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
)

func TestHealthcheckCommandUsesConfiguredLocalEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/health/ready" {
			t.Errorf("path = %q", request.URL.Path)
		}
		response.WriteHeader(http.StatusOK)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	lookup := mapEnvironment(map[string]string{"SEMLIA_HTTP_ADDR": listener.Addr().String()})
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"healthcheck", "ready"}, lookup, ioDiscard{}, &stderr); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
}

func TestHealthcheckCommandReturnsSafeFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()

	lookup := mapEnvironment(map[string]string{"SEMLIA_HTTP_ADDR": address})
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"healthcheck", "live"}, lookup, ioDiscard{}, &stderr); code != 1 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if stderr.String() != "healthcheck error: unavailable\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestHealthcheckRejectsUnknownKind(t *testing.T) {
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"healthcheck", "unknown"}, emptyEnvironment, ioDiscard{}, &stderr); code != 2 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
}

func mapEnvironment(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
