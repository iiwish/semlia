package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Environment string

// DefaultBuildVersion is injected into release executables by the linker.
var DefaultBuildVersion = "dev"

const (
	Development Environment = "development"
	Test        Environment = "test"
	Production  Environment = "production"
)

type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

type AuthMode string

const (
	AuthDisabled AuthMode = "disabled"
	AuthLocalUAT AuthMode = "local_uat"
	AuthOIDC     AuthMode = "oidc"
	AuthPassword AuthMode = "password"
)

type Config struct {
	Environment                Environment
	HTTPAddress                string
	DatabaseURL                string
	GitRepository              string
	SQLArtifactRoot            string
	AllowedOrigins             []string
	SecretKey                  string
	BuildVersion               string
	LogFormat                  LogFormat
	LocalUATIdentities         bool
	AuthMode                   AuthMode
	OIDCIssuer                 string
	OIDCClientID               string
	OIDCClientSecret           string
	OIDCRedirectURL            string
	WorkerConfigured           bool
	ArtifactStore              string
	ArtifactRoot               string
	ArtifactS3Bucket           string
	ArtifactS3Region           string
	ArtifactS3Endpoint         string
	ArtifactSpoolDir           string
	ArtifactRetention          time.Duration
	ExecutionSources           string
	ExecutionAllowPlaintext    bool
	DiscoveryAllowUnsafeSource bool
	SemanticProductionEnabled  bool
	ProductionGenerationGrants []domain.ProductionGenerationGrant
	AutoDraftSchemas           []string
	AutoDraftLimit             int
}

type LookupEnv func(string) (string, bool)

func Load(lookup LookupEnv) (Config, error) {
	if lookup == nil {
		return Config{}, fmt.Errorf("environment lookup is required")
	}

	environment := Environment(strings.ToLower(valueOrDefault(lookup, "SEMLIA_ENV", string(Development))))
	logDefault := string(LogFormatText)
	if environment == Production {
		logDefault = string(LogFormatJSON)
	}

	localUATIdentities, err := booleanValue(lookup, "SEMLIA_LOCAL_UAT_IDENTITIES")
	if err != nil {
		return Config{}, err
	}
	workerConfigured, err := booleanValue(lookup, "SEMLIA_WORKER_CONFIGURED")
	if err != nil {
		return Config{}, err
	}
	executionPlaintext, err := booleanValue(lookup, "SEMLIA_EXECUTION_ALLOW_PLAINTEXT")
	if err != nil {
		return Config{}, err
	}
	if executionPlaintext && environment != Development && environment != Test {
		return Config{}, fmt.Errorf("SEMLIA_EXECUTION_ALLOW_PLAINTEXT is restricted to development and test")
	}
	// Development-only relief for source roles that are not provably read-only.
	// Narrower than the execution plaintext flag on purpose: it must never be
	// reachable from a shared or acceptance environment.
	discoveryUnsafeSource, err := booleanValue(lookup, "SEMLIA_DISCOVERY_ALLOW_UNSAFE_SOURCE")
	if err != nil {
		return Config{}, err
	}
	if discoveryUnsafeSource && environment != Development {
		return Config{}, fmt.Errorf("SEMLIA_DISCOVERY_ALLOW_UNSAFE_SOURCE is restricted to development")
	}
	artifactRetention, err := durationValue(lookup, "SEMLIA_ARTIFACT_RETENTION", 30*24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	semanticProductionEnabled, err := booleanValue(lookup, "SEMLIA_SEMANTIC_PRODUCTION_ENABLED")
	if err != nil {
		return Config{}, err
	}
	generationGrants, err := parseProductionGenerationGrants(value(lookup, "SEMLIA_PRODUCTION_GENERATION_GRANTS"))
	if err != nil {
		return Config{}, err
	}
	if len(generationGrants) > 0 && !semanticProductionEnabled {
		return Config{}, fmt.Errorf("SEMLIA_PRODUCTION_GENERATION_GRANTS requires semantic production enabled")
	}
	autoDraftSchemas := splitValues(value(lookup, "SEMLIA_PRODUCTION_AUTO_DRAFT_SCHEMAS"))
	if len(autoDraftSchemas) > 0 && !semanticProductionEnabled {
		return Config{}, fmt.Errorf("SEMLIA_PRODUCTION_AUTO_DRAFT_SCHEMAS requires semantic production enabled")
	}
	autoDraftLimit, err := intValue(lookup, "SEMLIA_PRODUCTION_AUTO_DRAFT_LIMIT", 500)
	if err != nil {
		return Config{}, err
	}
	authDefault := AuthPassword
	cfg := Config{
		Environment:      environment,
		ExecutionSources: value(lookup, "SEMLIA_EXECUTION_SOURCES"), ExecutionAllowPlaintext: executionPlaintext,
		DiscoveryAllowUnsafeSource: discoveryUnsafeSource,
		HTTPAddress:                valueOrDefault(lookup, "SEMLIA_HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL:                value(lookup, "SEMLIA_DATABASE_URL"),
		GitRepository:              value(lookup, "SEMLIA_GIT_REPOSITORY"),
		SQLArtifactRoot:            value(lookup, "SEMLIA_SOURCE_ARTIFACT_ROOT"),
		AllowedOrigins:             splitValues(value(lookup, "SEMLIA_ALLOWED_ORIGINS")),
		SecretKey:                  value(lookup, "SEMLIA_SECRET_KEY"),
		BuildVersion:               valueOrDefault(lookup, "SEMLIA_BUILD_VERSION", DefaultBuildVersion),
		LogFormat:                  LogFormat(strings.ToLower(valueOrDefault(lookup, "SEMLIA_LOG_FORMAT", logDefault))),
		LocalUATIdentities:         localUATIdentities,
		AuthMode:                   AuthMode(strings.ToLower(valueOrDefault(lookup, "SEMLIA_AUTH_MODE", string(authDefault)))),
		OIDCIssuer:                 value(lookup, "SEMLIA_OIDC_ISSUER"),
		OIDCClientID:               value(lookup, "SEMLIA_OIDC_CLIENT_ID"),
		OIDCClientSecret:           value(lookup, "SEMLIA_OIDC_CLIENT_SECRET"),
		OIDCRedirectURL:            value(lookup, "SEMLIA_OIDC_REDIRECT_URL"),
		WorkerConfigured:           workerConfigured,
		ArtifactStore:              strings.ToLower(valueOrDefault(lookup, "SEMLIA_ARTIFACT_STORE", "local")),
		ArtifactRoot:               value(lookup, "SEMLIA_ARTIFACT_ROOT"),
		ArtifactS3Bucket:           value(lookup, "SEMLIA_ARTIFACT_S3_BUCKET"),
		ArtifactS3Region:           value(lookup, "SEMLIA_ARTIFACT_S3_REGION"),
		ArtifactS3Endpoint:         value(lookup, "SEMLIA_ARTIFACT_S3_ENDPOINT"),
		ArtifactSpoolDir:           value(lookup, "SEMLIA_ARTIFACT_SPOOL_DIR"),
		ArtifactRetention:          artifactRetention,
		SemanticProductionEnabled:  semanticProductionEnabled,
		ProductionGenerationGrants: generationGrants,
		AutoDraftSchemas:           autoDraftSchemas,
		AutoDraftLimit:             autoDraftLimit,
	}

	if fields := invalidFields(cfg); len(fields) > 0 {
		sort.Strings(fields)
		return Config{}, fmt.Errorf("invalid configuration: %s", strings.Join(fields, ", "))
	}
	return cfg, nil
}

func parseProductionGenerationGrants(raw string) ([]domain.ProductionGenerationGrant, error) {
	grants := []domain.ProductionGenerationGrant{}
	if strings.TrimSpace(raw) == "" {
		return grants, nil
	}
	invalid := func() ([]domain.ProductionGenerationGrant, error) {
		return nil, fmt.Errorf("invalid SEMLIA_PRODUCTION_GENERATION_GRANTS")
	}
	if len(raw) > 65536 {
		return invalid()
	}
	if _, err := domain.ParseProductionJSON([]byte(raw)); err != nil {
		return invalid()
	}
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&grants); err != nil || grants == nil || len(grants) > 128 {
		return invalid()
	}
	seen := map[string]bool{}
	for _, g := range grants {
		key := g.WorkspaceID.String() + "/" + g.PrincipalID.String() + "/" + g.ModelSettingID.String()
		if g.WorkspaceID.IsZero() || g.PrincipalID.IsZero() || g.ModelSettingID.IsZero() || !domain.IsValidContentDigest(g.ModelConfigRevision) || strings.TrimSpace(g.PricingBasis) == "" || len(g.PricingBasis) > 512 || seen[key] || g.CheckBudget(g.MaxInputBytes, g.MaxOutputTokens, g.MaxCostMicros) != nil {
			return invalid()
		}
		seen[key] = true
	}
	return grants, nil
}

func durationValue(lookup LookupEnv, key string, fallback time.Duration) (time.Duration, error) {
	raw := value(lookup, key)
	if raw == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid configuration: %s", key)
	}
	return parsed, nil
}

func intValue(lookup LookupEnv, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(value(lookup, key))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("invalid configuration: %s", key)
	}
	return parsed, nil
}

func booleanValue(lookup LookupEnv, key string) (bool, error) {
	raw := strings.ToLower(value(lookup, key))
	if raw == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid configuration: %s", key)
	}
	return parsed, nil
}

func value(lookup LookupEnv, key string) string {
	value, _ := lookup(key)
	return strings.TrimSpace(value)
}

func valueOrDefault(lookup LookupEnv, key, fallback string) string {
	if value := value(lookup, key); value != "" {
		return value
	}
	return fallback
}

func splitValues(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		values = append(values, strings.TrimSpace(part))
	}
	return values
}

func invalidFields(cfg Config) []string {
	var fields []string
	if cfg.Environment != Development && cfg.Environment != Test && cfg.Environment != Production {
		fields = append(fields, "SEMLIA_ENV")
	}
	if !validAddress(cfg.HTTPAddress) {
		fields = append(fields, "SEMLIA_HTTP_ADDR")
	}
	if cfg.LogFormat != LogFormatText && cfg.LogFormat != LogFormatJSON {
		fields = append(fields, "SEMLIA_LOG_FORMAT")
	}
	if cfg.BuildVersion == "" {
		fields = append(fields, "SEMLIA_BUILD_VERSION")
	}
	if cfg.ArtifactStore != "local" && cfg.ArtifactStore != "s3" {
		fields = append(fields, "SEMLIA_ARTIFACT_STORE")
	}
	if cfg.ArtifactRetention < 24*time.Hour || cfg.ArtifactRetention > 365*24*time.Hour {
		fields = append(fields, "SEMLIA_ARTIFACT_RETENTION")
	}
	if cfg.ArtifactStore == "s3" {
		if cfg.ArtifactS3Bucket == "" {
			fields = append(fields, "SEMLIA_ARTIFACT_S3_BUCKET")
		}
		if cfg.ArtifactS3Region == "" {
			fields = append(fields, "SEMLIA_ARTIFACT_S3_REGION")
		}
	}
	if cfg.LocalUATIdentities {
		fields = append(fields, "SEMLIA_LOCAL_UAT_IDENTITIES")
	}
	if cfg.AuthMode != AuthPassword && cfg.AuthMode != AuthOIDC {
		fields = append(fields, "SEMLIA_AUTH_MODE")
	}
	if cfg.AuthMode == AuthOIDC || cfg.AuthMode == AuthPassword {
		if cfg.DatabaseURL == "" {
			fields = append(fields, "SEMLIA_DATABASE_URL")
		}
		if len(cfg.SecretKey) < 32 {
			fields = append(fields, "SEMLIA_SECRET_KEY")
		}
		if len(cfg.AllowedOrigins) == 0 {
			fields = append(fields, "SEMLIA_ALLOWED_ORIGINS")
		}
	}
	if cfg.AuthMode == AuthOIDC {
		if !validOIDCIssuer(cfg.OIDCIssuer, cfg.Environment == Production) {
			fields = append(fields, "SEMLIA_OIDC_ISSUER")
		}
		if cfg.OIDCClientID == "" {
			fields = append(fields, "SEMLIA_OIDC_CLIENT_ID")
		}
		if cfg.Environment == Production && cfg.OIDCClientSecret == "" {
			fields = append(fields, "SEMLIA_OIDC_CLIENT_SECRET")
		}
		if !validOIDCRedirectURL(cfg.OIDCRedirectURL, cfg.Environment == Production) {
			fields = append(fields, "SEMLIA_OIDC_REDIRECT_URL")
		}
	}

	for _, origin := range cfg.AllowedOrigins {
		if !validOrigin(origin, cfg.Environment == Production) {
			fields = append(fields, "SEMLIA_ALLOWED_ORIGINS")
			break
		}
	}

	if cfg.Environment == Production {
		if cfg.AuthMode != AuthOIDC && cfg.AuthMode != AuthPassword {
			fields = append(fields, "SEMLIA_AUTH_MODE")
		}
		if !validProductionDatabaseURL(cfg.DatabaseURL) {
			fields = append(fields, "SEMLIA_DATABASE_URL")
		}
		if len(cfg.AllowedOrigins) == 0 {
			fields = append(fields, "SEMLIA_ALLOWED_ORIGINS")
		}
		if len(cfg.SecretKey) < 32 {
			fields = append(fields, "SEMLIA_SECRET_KEY")
		}
		if cfg.ArtifactStore != "s3" {
			fields = append(fields, "SEMLIA_ARTIFACT_STORE")
		}
	}
	return unique(fields)
}

func validOIDCIssuer(raw string, requireHTTPS bool) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if requireHTTPS {
		return parsed.Scheme == "https"
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func validOIDCRedirectURL(raw string, requireHTTPS bool) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if requireHTTPS {
		return parsed.Scheme == "https"
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func validAddress(address string) bool {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	value, err := strconv.Atoi(port)
	return err == nil && value >= 0 && value <= 65535
}

func validOrigin(origin string, requireHTTPS bool) bool {
	if origin == "" || origin == "*" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return false
	}
	if requireHTTPS {
		return parsed.Scheme == "https"
	}
	if parsed.Scheme == "https" {
		return true
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	return parsed.Scheme == "http" && (host == "localhost" || ip != nil && ip.IsLoopback())
}

func validProductionDatabaseURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() == "" {
		return false
	}
	switch parsed.Query().Get("sslmode") {
	case "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
