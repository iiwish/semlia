package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

	localartifacts "github.com/iiwish/semlia/internal/adapters/artifacts/local"
	s3artifacts "github.com/iiwish/semlia/internal/adapters/artifacts/s3"
	dbtartifact "github.com/iiwish/semlia/internal/adapters/discovery/dbt"
	fileartifact "github.com/iiwish/semlia/internal/adapters/discovery/files"
	postgreslive "github.com/iiwish/semlia/internal/adapters/discovery/postgreslive"
	postgresdiscovery "github.com/iiwish/semlia/internal/adapters/discovery/postgresql"
	embeddingadapter "github.com/iiwish/semlia/internal/adapters/embedding"
	executionadapter "github.com/iiwish/semlia/internal/adapters/execution"
	"github.com/iiwish/semlia/internal/adapters/gitcontent"
	mcpadapter "github.com/iiwish/semlia/internal/adapters/mcp"
	oidcadapter "github.com/iiwish/semlia/internal/adapters/oidc"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	webhookadapter "github.com/iiwish/semlia/internal/adapters/webhooks"
	"github.com/iiwish/semlia/internal/application"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	discoveryapp "github.com/iiwish/semlia/internal/application/discovery"
	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	embeddingapp "github.com/iiwish/semlia/internal/application/embedding"
	executionapp "github.com/iiwish/semlia/internal/application/execution"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	identityapp "github.com/iiwish/semlia/internal/application/identity"
	ingestionapp "github.com/iiwish/semlia/internal/application/ingestion"
	"github.com/iiwish/semlia/internal/application/jobs"
	operationsapp "github.com/iiwish/semlia/internal/application/operations"
	projectionapp "github.com/iiwish/semlia/internal/application/projection"
	usageapp "github.com/iiwish/semlia/internal/application/usage"
	webhookapp "github.com/iiwish/semlia/internal/application/webhooks"
	workbenchapp "github.com/iiwish/semlia/internal/application/workbench"
	"github.com/iiwish/semlia/internal/domain"
	discoverydomain "github.com/iiwish/semlia/internal/domain/discovery"
	executiondomain "github.com/iiwish/semlia/internal/domain/execution"
	ingestiondomain "github.com/iiwish/semlia/internal/domain/ingestion"
	operationsdomain "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/internal/platform/config"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	webui "github.com/iiwish/semlia/internal/platform/web"
	"go.opentelemetry.io/otel/sdk/trace"
)

const (
	apiVersion    = "v1"
	schemaVersion = "0.9.0"
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
	if len(args) > 0 && args[0] == "semantic" {
		return runSemantic(ctx, args[1:], lookup, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "mcp" {
		return runMCP(ctx, args[1:], lookup, os.Stdin, stdout, stderr)
	}
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
	case "check-schema":
		probe, closeProbe, err := configuredReadinessProbe(ctx, cfg.DatabaseURL)
		if err != nil {
			fmt.Fprintln(stderr, "schema check failed")
			return 1
		}
		defer closeProbe()
		if err := probe.Check(ctx); err != nil {
			fmt.Fprintln(stderr, "schema check failed; apply verified migrations before starting services")
			return 1
		}
		fmt.Fprintln(stdout, "database schema ready")
		return 0
	case "bootstrap-local-admin", "reset-local-password":
		if err := localPasswordCommand(ctx, cfg, args, os.Stdin, stdout); err != nil {
			fmt.Fprintln(stderr, "local account error: operation failed; check account policy and database readiness")
			return 1
		}
		return 0
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
	case "bootstrap-admin":
		if err := bootstrapAdministrator(ctx, cfg, args[1], args[2], args[3], stdout); err != nil {
			fmt.Fprintln(stderr, "bootstrap error: operation failed")
			return 1
		}
		return 0
	}
	return 2
}

func validCommand(args []string) bool {
	if len(args) == 4 && args[0] == "bootstrap-local-admin" {
		return true
	}
	if len(args) == 2 && args[0] == "reset-local-password" {
		return true
	}
	if len(args) == 1 {
		return args[0] == "doctor" || args[0] == "server" || args[0] == "worker" || args[0] == "check-schema"
	}
	if len(args) == 2 && args[0] == "migrate" {
		return args[1] == "up" || args[1] == "down" || args[1] == "version"
	}
	if len(args) == 2 && args[0] == "healthcheck" {
		return args[1] == "live" || args[1] == "ready"
	}
	if len(args) == 4 && args[0] == "bootstrap-admin" {
		return true
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
	artifactStore, err := configuredArtifactStore(ctx, cfg)
	if err != nil || !artifactStore.Configured() {
		return errors.New("artifact storage configuration is invalid")
	}
	ingestionService := ingestionapp.NewService(store, artifactStore, nil, ingestionapp.ClockFunc(time.Now), ingestionAdapters()).
		WithArtifactRetention(cfg.ArtifactRetention)
	worker := jobs.NewWorker(
		store,
		jobs.ClockFunc(time.Now),
		jobs.BackoffFunc(workerBackoff),
		30*time.Second,
	)
	governanceClock := governanceapp.ClockFunc(time.Now)
	if cfg.SemanticProductionEnabled {
		worker.Register(governanceapp.ProductionGenerationJobType, governanceapp.NewProductionGenerationService(store, cfg.ProductionGenerationGrants).JobHandler())
	}
	governancePolicy := governanceapp.NewPolicyService(
		store, governanceClock, governanceapp.WithRuleSource(store))
	worker.Register(governanceapp.ValidationJobType, governanceapp.NewValidationJobHandler(
		store,
		governanceapp.NewProposalService(store, governanceClock),
		governanceapp.NewValidationService(store, governanceClock),
		governanceapp.NewDefaultRegistry(),
		governancePolicy,
		governanceClock,
	).Handle)
	var credentialCipher *discoveryapp.CredentialCipher
	if len(cfg.SecretKey) >= 32 {
		credentialCipher, err = discoveryapp.NewCredentialCipher([]byte(cfg.SecretKey))
		if err != nil {
			return errors.New("source credential configuration is invalid")
		}
	}
	artifactLoader, err := discoveryapp.NewArtifactLoader(cfg.SQLArtifactRoot)
	if err != nil {
		return errors.New("SQL artifact root configuration is invalid")
	}
	discoveryService := discoveryapp.NewControlService(
		store, credentialCipher, postgreslive.NewLiveCollector(5*time.Second, 15*time.Second),
		artifactLoader, postgresdiscovery.Adapter{}, nil, discoveryapp.ClockFunc(time.Now),
	).WithArtifactStore(artifactStore, ingestionAdapters())
	worker.Register(discoveryapp.DiscoveryJobType, discoveryService.JobHandler())
	worker.Register(embeddingapp.JobType, embeddingapp.NewService(store, embeddingadapter.NewClient(nil, nil), nil).JobHandler())
	owner, err := newWorkerOwner()
	if err != nil {
		return errors.New("worker identity initialization failed")
	}
	scheduleService := ingestionapp.NewScheduleService(store, nil, ingestionapp.ClockFunc(time.Now))
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	lanes := 3
	results := make(chan error, 5)
	go func() { results <- worker.Run(workerCtx, owner+"-jobs", 500*time.Millisecond) }()
	go func() {
		results <- runScheduleLoop(workerCtx, scheduleService, owner+"-schedules", 500*time.Millisecond)
	}()
	go func() {
		results <- runArtifactCleanupLoop(workerCtx, ingestionService, time.Minute)
	}()
	router := jobs.NewRouterPublisher()
	if len(cfg.SecretKey) >= 32 {
		hooks, hookErr := webhookapp.NewService(store, nil, webhookadapter.NewSafeClient(nil), []byte(cfg.SecretKey))
		if hookErr != nil {
			return hookErr
		}
		router.Register(projectionapp.CatalogAssetChanged, hooks)
		router.Register(projectionapp.ReleasePublished, hooks)
		lanes++
		go func() { results <- hooks.Run(workerCtx, owner+"-webhooks") }()
	}
	if strings.TrimSpace(cfg.GitRepository) != "" {
		writer, writerErr := gitcontent.Open(cfg.GitRepository)
		if writerErr != nil {
			return writerErr
		}
		router.Register(projectionapp.CatalogAssetChanged, projectionapp.NewPublisher(store, writer))
		router.Register(projectionapp.ReleasePublished, projectionapp.NewReleasePublisher(store, writer))
	}
	if strings.TrimSpace(cfg.GitRepository) != "" || len(cfg.SecretKey) >= 32 {
		dispatcher := jobs.NewDispatcher(store, router, jobs.ClockFunc(time.Now), jobs.BackoffFunc(workerBackoff), 30*time.Second)
		lanes++
		go func() { results <- dispatcher.Run(workerCtx, owner+"-outbox", 500*time.Millisecond) }()
	}
	first := <-results
	cancel()
	for range lanes - 1 {
		<-results
	}
	if first != nil {
		return first
	}
	return nil
}

func runScheduleLoop(ctx context.Context, service *ingestionapp.ScheduleService, owner string, interval time.Duration) error {
	const batchSize = 100
	for {
		processed, err := service.ProcessDue(ctx, owner, batchSize, 30*time.Second)
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return nil
		}
		if err != nil && !errors.Is(err, ingestiondomain.ErrRetryable) {
			return err
		}
		if err == nil && processed == batchSize {
			continue
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func runArtifactCleanupLoop(ctx context.Context, service *ingestionapp.Service, interval time.Duration) error {
	const batchSize = 100
	consecutiveFailures := 0
	for {
		processed, err := service.RecoverStorage(ctx, batchSize)
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return nil
		}
		if err != nil {
			consecutiveFailures++
			if consecutiveFailures >= 3 {
				return fmt.Errorf("artifact storage recovery failed after retries: %w", err)
			}
		} else {
			consecutiveFailures = 0
			if processed == batchSize {
				continue
			}
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func configuredArtifactStore(ctx context.Context, cfg config.Config) (ingestiondomain.ArtifactStore, error) {
	if cfg.ArtifactStore == "s3" {
		return s3artifacts.New(ctx, s3artifacts.Options{Bucket: cfg.ArtifactS3Bucket, Region: cfg.ArtifactS3Region,
			Endpoint: cfg.ArtifactS3Endpoint, RequireHTTPS: cfg.Environment == config.Production, SpoolDirectory: cfg.ArtifactSpoolDir})
	}
	return localartifacts.New(cfg.ArtifactRoot)
}

func ingestionAdapters() map[ingestiondomain.ArtifactKind]discoverydomain.Adapter {
	return map[ingestiondomain.ArtifactKind]discoverydomain.Adapter{
		ingestiondomain.ArtifactCSV: fileartifact.CSV(), ingestiondomain.ArtifactXLSX: fileartifact.XLSX(),
		ingestiondomain.ArtifactMarkdown: fileartifact.Markdown(), ingestiondomain.ArtifactSQL: postgresdiscovery.Adapter{},
		ingestiondomain.ArtifactDBTManifest: dbtartifact.Adapter{}, ingestiondomain.ArtifactDBTCatalog: dbtartifact.Catalog(),
	}
}

func newWorkerOwner() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "worker-" + hex.EncodeToString(value), nil
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
	tracerProvider := trace.NewTracerProvider()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
			logger.Error("telemetry shutdown failed", "error", err)
		}
	}()

	artifactStore, err := configuredArtifactStore(ctx, cfg)
	if err != nil || (strings.TrimSpace(cfg.DatabaseURL) != "" && !artifactStore.Configured()) {
		return errors.New("artifact storage configuration is invalid")
	}
	probe, closeProbe, err := configuredReadinessProbe(ctx, cfg.DatabaseURL)
	if err != nil {
		return errors.New("database readiness configuration is invalid")
	}
	defer closeProbe()

	service := application.NewSystemService(deploymentReadinessProbe{database: probe, artifactStore: artifactStore}, domain.SystemInfo{
		APIVersion:    apiVersion,
		SchemaVersion: schemaVersion,
		BuildVersion:  cfg.BuildVersion,
	})
	options := make([]httpapi.Option, 0, 3)
	if strings.TrimSpace(cfg.DatabaseURL) != "" {
		catalogPool, openErr := pgstore.Open(ctx, cfg.DatabaseURL)
		if openErr != nil {
			return errors.New("catalog database configuration is invalid")
		}
		defer catalogPool.Close()
		catalogStore := pgstore.NewStore(catalogPool)
		catalogOptions := make([]catalogapp.Option, 0, 2)
		if len(cfg.SecretKey) >= 32 {
			usageService, usageErr := usageapp.NewService(catalogStore, usageapp.ClockFunc(time.Now), []byte(cfg.SecretKey))
			if usageErr != nil {
				return errors.New("usage service configuration is invalid")
			}
			catalogOptions = append(catalogOptions, catalogapp.WithUsage(usageService))
		}
		// Protected commands always evaluate capabilities server-side against
		// the M2 authorization foundation (deny-by-default).
		authorizer := authorizationapp.NewService(
			catalogStore, authorizationapp.ClockFunc(time.Now),
		)
		options = append(options, httpapi.WithAuthorization(authorizer))
		workbenchService := workbenchapp.NewService(catalogStore, authorizer, workbenchapp.ClockFunc(time.Now))
		reconcile, reconcileErr := workbenchService.ReconcileStartup(ctx, 10_000)
		if reconcileErr != nil {
			return errors.New("workbench startup reconciliation failed")
		}
		logger.Info("workbench startup reconciliation complete", "scanned", reconcile.Scanned,
			"upserted", reconcile.Upserted, "resolved", reconcile.Resolved, "truncated", reconcile.Truncated)
		options = append(options, httpapi.WithWorkbench(workbenchService))
		options = append(options, httpapi.WithOperations(operationsapp.NewService(
			catalogStore, authorizer, operationsapp.ClockFunc(time.Now), operationsdomain.DeploymentStatus{
				WorkerConfigured: cfg.WorkerConfigured, TelemetryConfigured: false,
				OIDCConfigured:       cfg.AuthMode == config.AuthOIDC,
				EncryptionConfigured: len(cfg.SecretKey) >= 32, AuditRetention: "deployment_managed",
			},
		)))
		if cfg.AuthMode == config.AuthPassword {
			identityService, identityErr := identityapp.NewLocalService(catalogStore, []byte(cfg.SecretKey), identityapp.WithAuthorizer(authorizer))
			if identityErr != nil {
				return errors.New("identity service configuration is invalid")
			}
			options = append(options, httpapi.WithIdentity(identityService, httpapi.IdentityHTTPConfig{
				SecureCookie: secureSessionCookies(cfg), AllowedOrigins: cfg.AllowedOrigins,
			}))
		} else if cfg.AuthMode == config.AuthOIDC {
			identityProvider, providerErr := oidcadapter.New(ctx, oidcadapter.Config{
				Issuer: cfg.OIDCIssuer, ClientID: cfg.OIDCClientID,
				ClientSecret: cfg.OIDCClientSecret, RedirectURL: cfg.OIDCRedirectURL,
			})
			if providerErr != nil {
				return errors.New("OIDC provider configuration is invalid")
			}
			identityService, identityErr := identityapp.NewService(
				catalogStore, identityProvider, []byte(cfg.SecretKey), identityapp.WithAuthorizer(authorizer),
			)
			if identityErr != nil {
				return errors.New("identity service configuration is invalid")
			}
			options = append(options, httpapi.WithIdentity(identityService, httpapi.IdentityHTTPConfig{
				OIDCEnabled:  true,
				SecureCookie: secureSessionCookies(cfg), AllowedOrigins: cfg.AllowedOrigins,
			}))
		}
		catalogOptions = append(catalogOptions, catalogapp.WithAuthorizer(authorizer))
		catalogService := catalogapp.NewService(catalogStore, catalogapp.ClockFunc(time.Now), catalogOptions...)
		options = append(options, httpapi.WithCatalog(catalogService))
		distributionService := distributionapp.NewService(
			catalogStore, authorizer, distributionapp.ClockFunc(time.Now),
		)
		options = append(options, httpapi.WithDistribution(distributionService))
		executionAdapter, executionErr := executionadapter.NewPostgres(cfg.ExecutionSources)
		if executionErr != nil {
			return errors.New("execution source references are invalid")
		}
		if cfg.ExecutionAllowPlaintext {
			executionAdapter.WithLocalPlaintext()
		}
		executionService := executionapp.NewService(catalogStore, distributionService, authorizer, executionAdapter, executiondomain.DefaultLimits())
		options = append(options, httpapi.WithExecution(executionService))
		options = append(options, httpapi.WithMachineIdentity(identityapp.NewMachineService(catalogStore, authorizer, time.Now)))
		options = append(options, httpapi.WithMCP(mcpadapter.NewHandler(distributionService, executionService)))
		if len(cfg.SecretKey) >= 32 {
			hooks, hookErr := webhookapp.NewService(catalogStore, authorizer, webhookadapter.NewSafeClient(nil), []byte(cfg.SecretKey))
			if hookErr != nil {
				return hookErr
			}
			options = append(options, httpapi.WithWebhooks(hooks))
		}
		var credentialCipher *discoveryapp.CredentialCipher
		if len(cfg.SecretKey) >= 32 {
			credentialCipher, err = discoveryapp.NewCredentialCipher([]byte(cfg.SecretKey))
			if err != nil {
				return errors.New("source credential configuration is invalid")
			}
		}
		artifactLoader, artifactErr := discoveryapp.NewArtifactLoader(cfg.SQLArtifactRoot)
		if artifactErr != nil {
			return errors.New("SQL artifact root configuration is invalid")
		}
		discoveryService := discoveryapp.NewControlService(
			catalogStore, credentialCipher, postgreslive.NewLiveCollector(5*time.Second, 15*time.Second),
			artifactLoader, postgresdiscovery.Adapter{}, authorizer, discoveryapp.ClockFunc(time.Now),
		).WithArtifactStore(artifactStore, ingestionAdapters()).WithSnapshotCursorSecret([]byte(cfg.SecretKey))
		options = append(options, httpapi.WithDiscovery(discoveryService))
		options = append(options, httpapi.WithEmbedding(embeddingapp.NewService(catalogStore, embeddingadapter.NewClient(nil, nil), authorizer)))
		ingestionService := ingestionapp.NewService(catalogStore, artifactStore, authorizer, ingestionapp.ClockFunc(time.Now), ingestionAdapters()).
			WithSQLArtifactLoader(artifactLoader).WithArtifactRetention(cfg.ArtifactRetention)
		options = append(options, httpapi.WithIngestion(ingestionService,
			ingestionapp.NewScheduleService(catalogStore, authorizer, ingestionapp.ClockFunc(time.Now))))
		clock := governanceapp.ClockFunc(time.Now)
		governancePolicy := governanceapp.NewPolicyService(
			catalogStore, clock, governanceapp.WithRuleSource(catalogStore))
		governanceAuthoring := governanceapp.NewAuthoringService(
			catalogStore,
			governanceapp.NewProposalService(catalogStore, clock),
			governanceapp.NewAgentRunService(catalogStore, clock),
			authorizer, clock,
			governanceapp.WithValidationOrchestrator(
				governanceapp.NewValidationOrchestrator(
					governanceapp.NewProposalService(catalogStore, clock), catalogStore, clock,
				),
			),
			governanceapp.WithDecisionRefresher(
				governanceapp.NewPolicyDecisionTrigger(catalogStore, governancePolicy),
			),
		)
		modelConfig := governanceapp.NewModelConfigService(catalogStore, authorizer, clock)
		governanceapp.WithModelConfig(modelConfig)(governanceAuthoring)
		governanceapp.WithGeneration(governanceapp.NewGenerationService(
			catalogStore, modelConfig,
			governanceapp.NewAgentRunService(catalogStore, clock),
			governanceAuthoring, authorizer, clock,
		))(governanceAuthoring)
		options = append(options, httpapi.WithGovernance(governanceAuthoring))
		options = append(options, httpapi.WithAsk(governanceapp.NewAskService(
			catalogStore, modelConfig, governanceapp.NewAgentRunService(catalogStore, clock),
			distributionService, authorizer,
		)))
		if cfg.SemanticProductionEnabled {
			options = append(options, httpapi.WithProduction(governanceapp.NewProductionService(catalogStore)))
			options = append(options, httpapi.WithProductionGeneration(governanceapp.NewProductionGenerationService(catalogStore, cfg.ProductionGenerationGrants)))
		}
	}
	apiHandler := httpapi.NewHandler(service, logger, tracerProvider.Tracer("github.com/iiwish/semlia"), options...)
	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           webui.NewHandler(apiHandler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      70 * time.Second,
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

func bootstrapAdministrator(ctx context.Context, cfg config.Config, slug, name, email string, output io.Writer) error {
	if cfg.AuthMode != config.AuthOIDC || cfg.DatabaseURL == "" {
		return errors.New("OIDC authentication and PostgreSQL are required")
	}
	pool, err := pgstore.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	provider, err := oidcadapter.New(ctx, oidcadapter.Config{
		Issuer: cfg.OIDCIssuer, ClientID: cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret, RedirectURL: cfg.OIDCRedirectURL,
	})
	if err != nil {
		return err
	}
	service, err := identityapp.NewService(pgstore.NewStore(pool), provider, []byte(cfg.SecretKey))
	if err != nil {
		return err
	}
	issued, err := service.BootstrapAdministrator(ctx, slug, name, email)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "workspace invitation: %s\n", issued.Invitation.ID.String())
	return nil
}

func newLogger(format config.LogFormat, output io.Writer) *slog.Logger {
	if format == config.LogFormatJSON {
		return slog.New(slog.NewJSONHandler(output, nil))
	}
	return slog.New(slog.NewTextHandler(output, nil))
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, "       semlia bootstrap-local-admin <workspace-slug> <workspace-name> <username> (password via stdin)")
	fmt.Fprintln(output, "       semlia reset-local-password <username> (password via stdin)")
	fmt.Fprintln(output, "usage: semlia <server|doctor|worker|migrate|healthcheck|bootstrap-admin|semantic|mcp>")
	fmt.Fprintln(output, "       semlia semantic <describe|search|resolve|plan|query> <query-json|search-text|id>")
	fmt.Fprintln(output, "       semlia mcp (stdio; SEMLIA_API_URL, SEMLIA_WORKSPACE_ID, SEMLIA_API_TOKEN)")
	fmt.Fprintln(output, "       semlia migrate <up|down|version>")
	fmt.Fprintln(output, "       semlia healthcheck <live|ready>")
	fmt.Fprintln(output, "       semlia bootstrap-admin <workspace-slug> <workspace-name> <email>")
}
