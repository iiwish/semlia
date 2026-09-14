package config_test

import (
	"github.com/iiwish/semlia/internal/platform/config"
	"strings"
	"testing"
)

func TestPasswordRuntimeRequiresRealIdentity(t *testing.T) {
	base := map[string]string{"SEMLIA_DATABASE_URL": "postgres://localhost/semlia", "SEMLIA_SECRET_KEY": strings.Repeat("a", 64), "SEMLIA_ALLOWED_ORIGINS": "http://127.0.0.1:18081"}
	lookup := func(k string) (string, bool) { v, ok := base[k]; return v, ok }
	cfg, err := config.Load(lookup)
	if err != nil || string(cfg.AuthMode) != "password" {
		t.Fatalf("default auth=%s, err=%v", cfg.AuthMode, err)
	}
	for _, mode := range []string{"disabled", "local_uat"} {
		base["SEMLIA_AUTH_MODE"] = mode
		if _, err := config.Load(lookup); err == nil {
			t.Errorf("accepted %s", mode)
		}
	}
}
