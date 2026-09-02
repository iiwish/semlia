package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/iiwish/semlia/internal/adapters/gitcontent"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/internal/application"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	"github.com/iiwish/semlia/internal/application/jobs"
	projectionapp "github.com/iiwish/semlia/internal/application/projection"
	usageapp "github.com/iiwish/semlia/internal/application/usage"
	"github.com/iiwish/semlia/internal/domain"
	"github.com/iiwish/semlia/internal/platform/config"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	webui "github.com/iiwish/semlia/internal/platform/web"
	"go.opentelemetry.io/otel/sdk/trace"
)

const (
	apiVersion    = "v1"
	schemaVersion = "0.4.0"
)

var (
	errMigrationInitialize = errors.New("migration initialization failed")
	errMigrationApply      = errors.New("migration operation failed")
	errMigrationFinalize   = errors.New("migration finalization failed")
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, lookup config.LookupEnv, stdout, stderr io.Writer) int {
	if !validCommand(args) {
		printUsage(stderr)
		return 2
	}

	cfg, err := config.Load(lookup)
	if err != nil {
		fmt.Fprintf(stderr, "configuration error: %v\n", err)
		return 1
	}

	switch args[0] {
	case "doctor":
		fmt.Fprintln(stdout, "configuration ok")
		return 0
	case "server":
		if err := serve(ctx, cfg, stderr); err != nil {
			fmt.Fprintf(stderr, "server error: %v\n", err)
			return 1
		}
		return 0
	case "migrate":
		if cfg.DatabaseURL == "" {
			fmt.Fprintln(stderr, "migration error: database is not configured")
			return 1
		}
		if err := migrateDatabase(cfg.DatabaseURL, migrationPath(lookup), args[1], stdout); err != nil {
			fmt.Fprintf(stderr, "migration error: %s\n", migrationFailureStage(err))
			return 1
		}
		return 0
	case "worker":
		if cfg.DatabaseURL == "" {
			fmt.Fprintln(stderr, "worker error: database is not configured")
			return 1
		}
		workerLogger := newLogger(cfg.LogFormat, stderr)
		if cfg.LogFormat == config.LogFormatJSON {
			workerLogger.Info("worker starting")
		}
		if err := runWorker(ctx, cfg); err != nil {
			if cfg.LogFormat == config.LogFormatJSON {
				workerLogger.Error("worker stopped", "error_code", "OPERATION_FAILED")
			} else {
				fmt.Fprintln(stderr, "worker error: operation failed")
			}
			return 1
		}
		return 0
	case "healthcheck":
		if err := healthcheck(ctx, cfg.HTTPAddress, args[1]); err != nil {
			fmt.Fprintln(stderr, "healthcheck error: unavailable")
			return 1
		}
		return 0
	}
	return 2
}

func validCommand(args []string) bool {
	if len(args) == 1 {
		return args[0] == "doctor" || args[0] == "server" || args[0] == "worker"
	}
	if len(args) == 2 && args[0] == "migrate" {
		return args[1] == "up" || args[1] == "down" || args[1] == "version"
	}
	if len(args) == 2 && args[0] == "healthcheck" {
		return args[1] == "live" || args[1] == "ready"
	}
	return false
}

func migrationPath(lookup config.LookupEnv) string {
	if value, ok := lookup("SEMLIA_MIGRATIONS_PATH"); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return "migrations"
}

func migrateDatabase(databaseURL, migrationsPath, action string, output io.Writer) (resultErr error) {
	migrator, err := pgstore.NewMigrator(databaseURL, migrationsPath)
	if err != nil {
		return fmt.Errorf("%w: %w", errMigrationInitialize, err)
	}
	defer func() {
		if err := migrator.Close(); resultErr == nil && err != nil {
			resultErr = fmt.Errorf("%w: %w", errMigrationFinalize, err)
		}
	}()

	switch action {
	case "up":
		if err := migrator.Up(); err != nil {
			return fmt.Errorf("%w: %w", errMigrationApply, err)
		}
		fmt.Fprintln(output, "migration up complete")
	case "down":
		if err := migrator.Down(); err != nil {
			return fmt.Errorf("%w: %w", errMigrationApply, err)
		}
		fmt.Fprintln(output, "migration down complete")
	case "version":
		version, dirty, err := migrator.Version()
		if err != nil {
			return fmt.Errorf("%w: %w", errMigrationApply, err)
		}
		if version == 0 {
			fmt.Fprintln(output, "migration version: empty")
			return nil
		}
		fmt.Fprintf(output, "migration version: %d (dirty=%t)\n", version, dirty)
	}
	return nil
}

func migrationFailureStage(err error) string {
	switch {
	case errors.Is(err, errMigrationInitialize):
		return "initialization failed"
	case errors.Is(err, errMigrationFinalize):
		return "finalization failed"
	default:
		return "operation failed"
	}
}

func runWorker(ctx context.Context, cfg config.Config) error {
	pool, err := pgstore.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	store := pgstore.NewStore(pool)
	worker := jobs.NewWorker(
		store,
		jobs.ClockFunc(time.Now),
		jobs.BackoffFunc(workerBackoff),
		30*time.Second,
	)
	owner := fmt.Sprintf("worker-%d", os.Getpid())
	if strings.TrimSpace(cfg.GitRepository) == "" {
		return worker.Run(ctx, owner, 500*time.Millisecond)
	}
	writer, err := gitcontent.Open(cfg.GitRepository)
	if err != nil {
		return err
	}
	router := jobs.NewRouterPublisher()
	router.Register(projectionapp.CatalogAssetChanged, projectionapp.NewPublisher(store, writer))
	dispatcher := jobs.NewDispatcher(
		store, router, jobs.ClockFunc(time.Now), jobs.BackoffFunc(workerBackoff), 30*time.Second,
	)
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, 2)
	go func() { results <- worker.Run(workerCtx, owner+"-jobs", 500*time.Millisecond) }()
	go func() { results <- dispatcher.Run(workerCtx, owner+"-outbox", 500*time.Millisecond) }()
	first := <-results
	cancel()
	second := <-results
	if first != nil {
		return first
	}
	return second
}

func workerBackoff(attempt int32) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Second << (attempt - 1)
}

func serve(ctx context.Context, cfg config.Config, output io.Writer) error {
	logger := newLogger(cfg.LogFormat, output)
	provider := trace.NewTracerProvider()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := provider.Shutdown(shutdownCtx); err != nil {
			logger.Error("telemetry shutdown failed", "error", err)
		}
	}()

	probe, closeProbe, err := configuredReadinessProbe(ctx, cfg.DatabaseURL)
	if err != nil {
		return errors.New("database readiness configuration is invalid")
	}
	defer closeProbe()

	service := application.NewSystemService(probe, domain.SystemInfo{
		APIVersion:    apiVersion,
		SchemaVersion: schemaVersion,
		BuildVersion:  cfg.BuildVersion,
	})
	options := make([]httpapi.Option, 0, 1)
	if strings.TrimSpace(cfg.DatabaseURL) != "" {
		catalogPool, openErr := pgstore.Open(ctx, cfg.DatabaseURL)
		if openErr != nil {
			return errors.New("catalog database configuration is invalid")
		}
		defer catalogPool.Close()
		catalogStore := pgstore.NewStore(catalogPool)
		catalogOptions := make([]catalogapp.Option, 0, 1)
		if len(cfg.SecretKey) >= 32 {
			usageService, usageErr := usageapp.NewService(catalogStore, usageapp.ClockFunc(time.Now), []byte(cfg.SecretKey))
			if usageErr != nil {
				return errors.New("usage service configuration is invalid")
			}
			catalogOptions = append(catalogOptions, catalogapp.WithUsage(usageService))
		}
		catalogService := catalogapp.NewService(catalogStore, catalogapp.ClockFunc(time.Now), catalogOptions...)
		options = append(options, httpapi.WithCatalog(catalogService))
	}
	apiHandler := httpapi.NewHandler(service, logger, provider.Tracer("github.com/iiwish/semlia"), options...)
	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           webui.NewHandler(apiHandler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	result := make(chan error, 1)
	go func() {
		logger.Info("server starting", "address", cfg.HTTPAddress, "environment", cfg.Environment)
		result <- server.ListenAndServe()
	}()

	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return nil
	}
}

func newLogger(format config.LogFormat, output io.Writer) *slog.Logger {
	if format == config.LogFormatJSON {
		return slog.New(slog.NewJSONHandler(output, nil))
	}
	return slog.New(slog.NewTextHandler(output, nil))
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, "usage: semlia <server|doctor|worker|migrate|healthcheck>")
	fmt.Fprintln(output, "       semlia migrate <up|down|version>")
	fmt.Fprintln(output, "       semlia healthcheck <live|ready>")
}
