package config

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type Environment string

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

type Config struct {
	Environment    Environment
	HTTPAddress    string
	DatabaseURL    string
	GitRepository  string
	AllowedOrigins []string
	SecretKey      string
	BuildVersion   string
	LogFormat      LogFormat
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

	cfg := Config{
		Environment:    environment,
		HTTPAddress:    valueOrDefault(lookup, "SEMLIA_HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL:    value(lookup, "SEMLIA_DATABASE_URL"),
		GitRepository:  value(lookup, "SEMLIA_GIT_REPOSITORY"),
		AllowedOrigins: splitValues(value(lookup, "SEMLIA_ALLOWED_ORIGINS")),
		SecretKey:      value(lookup, "SEMLIA_SECRET_KEY"),
		BuildVersion:   valueOrDefault(lookup, "SEMLIA_BUILD_VERSION", "dev"),
		LogFormat:      LogFormat(strings.ToLower(valueOrDefault(lookup, "SEMLIA_LOG_FORMAT", logDefault))),
	}

	if fields := invalidFields(cfg); len(fields) > 0 {
		sort.Strings(fields)
		return Config{}, fmt.Errorf("invalid configuration: %s", strings.Join(fields, ", "))
	}
	return cfg, nil
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

	for _, origin := range cfg.AllowedOrigins {
		if !validOrigin(origin, cfg.Environment == Production) {
			fields = append(fields, "SEMLIA_ALLOWED_ORIGINS")
			break
		}
	}

	if cfg.Environment == Production {
		if !validProductionDatabaseURL(cfg.DatabaseURL) {
			fields = append(fields, "SEMLIA_DATABASE_URL")
		}
		if len(cfg.AllowedOrigins) == 0 {
			fields = append(fields, "SEMLIA_ALLOWED_ORIGINS")
		}
		if len(cfg.SecretKey) < 32 {
			fields = append(fields, "SEMLIA_SECRET_KEY")
		}
	}
	return unique(fields)
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
	return parsed.Scheme == "http" || parsed.Scheme == "https"
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
