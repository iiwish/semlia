package config_test

import (
	"strings"
	"testing"

	"github.com/semlia/semlia/internal/platform/config"
)

func TestLoadDevelopmentDefaults(t *testing.T) {
	cfg, err := config.Load(mapLookup(nil))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Environment != config.Development {
		t.Errorf("environment = %q", cfg.Environment)
	}
	if cfg.HTTPAddress != "127.0.0.1:8080" {
		t.Errorf("HTTP address = %q", cfg.HTTPAddress)
	}
	if cfg.BuildVersion != "dev" {
		t.Errorf("build version = %q", cfg.BuildVersion)
	}
	if cfg.LogFormat != config.LogFormatText {
		t.Errorf("log format = %q", cfg.LogFormat)
	}
}

func TestLoadProductionFailsClosedWithoutRequiredSecurityConfiguration(t *testing.T) {
	_, err := config.Load(mapLookup(map[string]string{
		"SEMLIA_ENV": "production",
	}))
	if err == nil {
		t.Fatal("expected production configuration to fail")
	}

	message := err.Error()
	for _, name := range []string{"SEMLIA_DATABASE_URL", "SEMLIA_ALLOWED_ORIGINS", "SEMLIA_SECRET_KEY"} {
		if !strings.Contains(message, name) {
			t.Errorf("error does not name %s: %s", name, message)
		}
	}
}

func TestLoadProductionAcceptsSecureConfiguration(t *testing.T) {
	cfg, err := config.Load(mapLookup(map[string]string{
		"SEMLIA_ENV":             "production",
		"SEMLIA_HTTP_ADDR":       ":8080",
		"SEMLIA_DATABASE_URL":    "postgres://semlia@example.internal/semlia?sslmode=verify-full",
		"SEMLIA_ALLOWED_ORIGINS": "https://app.example.com,https://admin.example.com",
		"SEMLIA_SECRET_KEY":      strings.Repeat("s", 32),
		"SEMLIA_BUILD_VERSION":   "0.1.0",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.LogFormat != config.LogFormatJSON {
		t.Errorf("production log format = %q", cfg.LogFormat)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Fatalf("allowed origins = %v", cfg.AllowedOrigins)
	}
}

func TestLoadProductionRejectsUnsafeValuesWithoutEchoingSecrets(t *testing.T) {
	const databaseURL = "postgres://semlia:database-secret@example.internal/semlia?sslmode=disable"
	const secretKey = "short-secret"
	_, err := config.Load(mapLookup(map[string]string{
		"SEMLIA_ENV":             "production",
		"SEMLIA_DATABASE_URL":    databaseURL,
		"SEMLIA_ALLOWED_ORIGINS": "http://app.example.com,*",
		"SEMLIA_SECRET_KEY":      secretKey,
	}))
	if err == nil {
		t.Fatal("expected unsafe production configuration to fail")
	}

	message := err.Error()
	for _, sensitive := range []string{databaseURL, "database-secret", secretKey} {
		if strings.Contains(message, sensitive) {
			t.Errorf("configuration error leaked %q: %s", sensitive, message)
		}
	}
}

func TestLoadRejectsInvalidEnvironmentAddressAndLogFormat(t *testing.T) {
	_, err := config.Load(mapLookup(map[string]string{
		"SEMLIA_ENV":        "staging",
		"SEMLIA_HTTP_ADDR":  "not-an-address",
		"SEMLIA_LOG_FORMAT": "yaml",
	}))
	if err == nil {
		t.Fatal("expected invalid configuration")
	}
	for _, name := range []string{"SEMLIA_ENV", "SEMLIA_HTTP_ADDR", "SEMLIA_LOG_FORMAT"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not name %s: %s", name, err)
		}
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
