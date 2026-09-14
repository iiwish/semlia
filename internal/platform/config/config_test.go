package config_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/platform/config"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestProductionGenerationGrantsFailClosed(t *testing.T) {
	w, _ := identity.NewWorkspaceID()
	p, _ := identity.NewPrincipalID()
	m, _ := identity.NewModelSettingID()
	g := domain.ProductionGenerationGrant{WorkspaceID: w, PrincipalID: p, ModelSettingID: m, ModelConfigRevision: "sha256:" + strings.Repeat("a", 64), PricingBasis: "operator conservative bound", MaxInputBytes: 1000, MaxOutputTokens: 100, MaxCostMicros: 1100, InputMicrosPerByte: 1, OutputMicrosPerToken: 1}
	raw, _ := json.Marshal([]domain.ProductionGenerationGrant{g})
	load := func(raw string, enabled string) (config.Config, error) {
		return config.Load(mapLookup(map[string]string{"SEMLIA_SEMANTIC_PRODUCTION_ENABLED": enabled, "SEMLIA_PRODUCTION_GENERATION_GRANTS": raw}))
	}
	if cfg, err := load(string(raw), "true"); err != nil || len(cfg.ProductionGenerationGrants) != 1 {
		t.Fatalf("valid grant: %v", err)
	}
	if _, err := load(string(raw), "false"); err == nil {
		t.Fatal("grant bypasses feature gate")
	}
	if cfg, err := load("", "true"); err != nil || len(cfg.ProductionGenerationGrants) != 0 {
		t.Fatal("default is not off")
	}
	for _, invalid := range []string{"null", "{}", `[{"maxCostMicros":1,"maxCostMicros":2}]`, strings.Replace(string(raw), `"maxCostMicros":1100`, `"maxCostMicros":1099`, 1), strings.Replace(string(raw), `"inputMicrosPerByte":1`, `"inputMicrosPerByte":0`, 1), strings.Replace(string(raw), `"pricingBasis":"operator conservative bound"`, `"pricingBasis":""`, 1), strings.TrimSuffix(string(raw), "]") + "," + strings.TrimPrefix(string(raw), "["), "[" + strings.Repeat(" ", 65537) + "]"} {
		if _, err := load(invalid, "true"); err == nil {
			t.Fatalf("invalid grant accepted: %.128s", invalid)
		}
	}
}

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
	if cfg.AuthMode != config.AuthPassword {
		t.Errorf("auth mode = %q", cfg.AuthMode)
	}
	if cfg.ArtifactRetention != 30*24*time.Hour {
		t.Fatalf("artifact retention = %s", cfg.ArtifactRetention)
	}
	if cfg.SemanticProductionEnabled {
		t.Errorf("expected SemanticProductionEnabled default to false")
	}
}

func TestSemanticProductionFeatureFence(t *testing.T) {
	cfg, err := config.Load(mapLookup(map[string]string{"SEMLIA_SEMANTIC_PRODUCTION_ENABLED": "true"}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SemanticProductionEnabled {
		t.Errorf("expected SemanticProductionEnabled to be true")
	}
}

func TestExecutionPlaintextRequiresExplicitLocalEnvironment(t *testing.T) {
	for _, environment := range []string{"production", "invalid"} {
		if _, err := config.Load(mapLookup(map[string]string{"SEMLIA_ENV": environment, "SEMLIA_EXECUTION_ALLOW_PLAINTEXT": "true"})); err == nil || !strings.Contains(err.Error(), "SEMLIA_EXECUTION_ALLOW_PLAINTEXT") {
			t.Fatalf("plaintext accepted for %s: %v", environment, err)
		}
	}
	for _, environment := range []string{"development", "test"} {
		cfg, err := config.Load(mapLookup(map[string]string{"SEMLIA_ENV": environment, "SEMLIA_EXECUTION_ALLOW_PLAINTEXT": "true"}))
		if err != nil || !cfg.ExecutionAllowPlaintext {
			t.Fatalf("explicit local plaintext refused: %v", err)
		}
	}
}

func TestLoadArtifactRetentionIsBounded(t *testing.T) {
	cfg, err := config.Load(mapLookup(map[string]string{"SEMLIA_ARTIFACT_RETENTION": "168h"}))
	if err != nil || cfg.ArtifactRetention != 7*24*time.Hour {
		t.Fatalf("artifact retention=%s err=%v", cfg.ArtifactRetention, err)
	}
	for _, raw := range []string{"invalid", "23h", "8761h"} {
		if _, err := config.Load(mapLookup(map[string]string{"SEMLIA_ARTIFACT_RETENTION": raw})); err == nil || !strings.Contains(err.Error(), "SEMLIA_ARTIFACT_RETENTION") {
			t.Fatalf("retention %q error=%v", raw, err)
		}
	}
}

func TestLoadDeploymentStatusUsesExplicitReadOnlyConfiguration(t *testing.T) {
	cfg, err := config.Load(mapLookup(map[string]string{
		"SEMLIA_WORKER_CONFIGURED": "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.WorkerConfigured {
		t.Fatal("worker deployment status did not use explicit configuration")
	}
	_, err = config.Load(mapLookup(map[string]string{"SEMLIA_WORKER_CONFIGURED": "sometimes"}))
	if err == nil || !strings.Contains(err.Error(), "SEMLIA_WORKER_CONFIGURED") {
		t.Fatalf("invalid worker status error = %v", err)
	}
}

func TestLoadProductionFailsClosedWithoutRequiredSecurityConfiguration(t *testing.T) {
	_, err := config.Load(mapLookup(map[string]string{
		"SEMLIA_ENV":       "production",
		"SEMLIA_AUTH_MODE": "oidc",
	}))
	if err == nil {
		t.Fatal("expected production configuration to fail")
	}

	message := err.Error()
	for _, name := range []string{"SEMLIA_DATABASE_URL", "SEMLIA_ALLOWED_ORIGINS", "SEMLIA_SECRET_KEY", "SEMLIA_OIDC_ISSUER", "SEMLIA_OIDC_CLIENT_ID", "SEMLIA_OIDC_CLIENT_SECRET", "SEMLIA_OIDC_REDIRECT_URL"} {
		if !strings.Contains(message, name) {
			t.Errorf("error does not name %s: %s", name, message)
		}
	}
}

func TestLoadProductionAcceptsSecureConfiguration(t *testing.T) {
	cfg, err := config.Load(mapLookup(map[string]string{
		"SEMLIA_ENV":                "production",
		"SEMLIA_AUTH_MODE":          "oidc",
		"SEMLIA_HTTP_ADDR":          ":8080",
		"SEMLIA_DATABASE_URL":       "postgres://semlia@example.internal/semlia?sslmode=verify-full",
		"SEMLIA_ALLOWED_ORIGINS":    "https://app.example.com,https://admin.example.com",
		"SEMLIA_SECRET_KEY":         strings.Repeat("s", 32),
		"SEMLIA_BUILD_VERSION":      "0.1.0",
		"SEMLIA_OIDC_ISSUER":        "https://identity.example.com",
		"SEMLIA_OIDC_CLIENT_ID":     "semlia",
		"SEMLIA_OIDC_CLIENT_SECRET": "client-secret",
		"SEMLIA_OIDC_REDIRECT_URL":  "https://app.example.com/api/v1/auth/callback",
		"SEMLIA_ARTIFACT_STORE":     "s3",
		"SEMLIA_ARTIFACT_S3_BUCKET": "semlia-artifacts",
		"SEMLIA_ARTIFACT_S3_REGION": "us-east-1",
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
	if cfg.AuthMode != config.AuthOIDC {
		t.Fatalf("auth mode = %q", cfg.AuthMode)
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

func TestLoadLocalUATIdentitiesAreRejected(t *testing.T) {
	_, err := config.Load(mapLookup(map[string]string{
		"SEMLIA_LOCAL_UAT_IDENTITIES": "true",
	}))
	if err == nil || !strings.Contains(err.Error(), "SEMLIA_LOCAL_UAT_IDENTITIES") {
		t.Fatalf("fake identity configuration must fail: %v", err)
	}

	for name, values := range map[string]map[string]string{
		"invalid boolean": {
			"SEMLIA_LOCAL_UAT_IDENTITIES": "sometimes",
		},
		"production": {
			"SEMLIA_ENV":                  "production",
			"SEMLIA_DATABASE_URL":         "postgres://semlia@example.internal/semlia?sslmode=verify-full",
			"SEMLIA_ALLOWED_ORIGINS":      "https://app.example.com",
			"SEMLIA_SECRET_KEY":           strings.Repeat("s", 32),
			"SEMLIA_LOCAL_UAT_IDENTITIES": "true",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, loadErr := config.Load(mapLookup(values))
			if loadErr == nil || !strings.Contains(loadErr.Error(), "SEMLIA_LOCAL_UAT_IDENTITIES") {
				t.Fatalf("local UAT configuration error = %v", loadErr)
			}
		})
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
		if !ok && values["SEMLIA_ENV"] != "production" {
			value, ok = map[string]string{
				"SEMLIA_DATABASE_URL":    "postgres://localhost/semlia_test",
				"SEMLIA_SECRET_KEY":      strings.Repeat("s", 64),
				"SEMLIA_ALLOWED_ORIGINS": "http://127.0.0.1:18081",
			}[key]
		}
		return value, ok
	}
}
