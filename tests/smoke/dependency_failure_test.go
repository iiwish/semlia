package smoke

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"
)

func Test04PostgresFailureAndRecovery(t *testing.T) {
	requireRuntimeSmoke(t)

	containerID := strings.TrimSpace(compose(t, "ps", "-q", "postgres"))
	compose(t, "stop", "postgres")
	t.Cleanup(func() {
		composeWithoutFailure(t, "start", "postgres")
		waitForStatus(t, "/health/ready", http.StatusOK, 90*time.Second)
	})

	waitForStatus(t, "/health/live", http.StatusOK, 30*time.Second)
	waitForStatus(t, "/health/ready", http.StatusServiceUnavailable, 30*time.Second)
	get(t, "/", http.StatusOK)
	if body := get(t, "/health/ready", http.StatusServiceUnavailable); !bytes.Contains(body, []byte("DEPENDENCY_UNAVAILABLE")) {
		t.Fatalf("readiness response = %s", body)
	}

	compose(t, "start", "postgres")
	waitForStatus(t, "/health/ready", http.StatusOK, 90*time.Second)
	if recoveredID := strings.TrimSpace(compose(t, "ps", "-q", "postgres")); recoveredID != containerID {
		t.Fatalf("postgres container changed during stop/start: before=%q after=%q", containerID, recoveredID)
	}
}
