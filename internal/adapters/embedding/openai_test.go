package embedding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	governance "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/embedding"
)

func TestTransportRejectsRedirectOversizeAndSanitizesErrors(t *testing.T) {
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer target.Close()
	for _, scenario := range []string{"redirect", "oversize", "error", "timeout", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch scenario {
				case "redirect":
					http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
				case "oversize":
					_, _ = w.Write([]byte(strings.Repeat("x", 4*1024*1024+1)))
				case "error":
					w.WriteHeader(500)
					_, _ = w.Write([]byte("secret-key private upstream diagnostic"))
				default:
					time.Sleep(100 * time.Millisecond)
				}
			}))
			defer server.Close()
			config := domain.Config{Endpoint: server.URL, Model: "m", Dimension: 2, CredentialEnv: "KEY", CredentialRevision: governance.CredentialRevisionDigest("secret-key")}
			client := NewClient(&http.Client{Timeout: 20 * time.Millisecond}, func(string) string { return "secret-key" })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancel" {
				cancel()
			}
			_, err := client.EmbedBatch(ctx, config, []string{"released"})
			if err == nil || strings.Contains(err.Error(), "secret-key") || strings.Contains(err.Error(), "diagnostic") {
				t.Fatalf("unsafe transport outcome=%v", err)
			}
		})
	}
	if leaked.Load() {
		t.Fatal("redirect target received request")
	}
}

func TestEmbedBatchValidatesIndexesAndPinnedCredentials(t *testing.T) {
	for _, response := range []string{`{"data":[{"index":0,"embedding":[1,2]}]}`, `{"data":[{"index":0,"embedding":[1,2]},{"index":0,"embedding":[2,3]}]}`, `{"data":[{"index":0,"embedding":[1]},{"index":1,"embedding":[2,3]}]}`, `{"data":[{"index":0,"embedding":[null,1]},{"index":1,"embedding":[2,3]}]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/embeddings" {
				t.Errorf("path=%s", r.URL.Path)
			}
			_, _ = w.Write([]byte(response))
		}))
		client := NewClient(server.Client(), func(string) string { return "secret" })
		config := domain.Config{Endpoint: server.URL, Model: "pinned-model", Dimension: 2, CredentialEnv: "TEST_KEY", CredentialRevision: governance.CredentialRevisionDigest("secret")}
		if _, err := client.EmbedBatch(context.Background(), config, []string{"a", "b"}); err == nil {
			t.Fatal("accepted invalid provider result")
		}
		server.Close()
	}
}

func TestEmbedBatchUsesOnePinnedCredentialResolution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer old" {
			t.Errorf("unpinned credential sent")
		}
		_, _ = w.Write([]byte(`{"model":"m","data":[{"index":0,"embedding":[1,2]}]}`))
	}))
	defer server.Close()
	calls := 0
	client := NewClient(server.Client(), func(string) string {
		calls++
		if calls == 1 {
			return "old"
		}
		return "rotated"
	})
	config := domain.Config{Endpoint: server.URL, Model: "m", Dimension: 2, CredentialEnv: "KEY", CredentialRevision: governance.CredentialRevisionDigest("old")}
	if _, err := client.EmbedBatch(context.Background(), config, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("credential resolutions=%d", calls)
	}
}
