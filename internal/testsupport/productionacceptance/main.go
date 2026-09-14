// This executable is exclusively for isolated local production acceptance.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	localartifacts "github.com/iiwish/semlia/internal/adapters/artifacts/local"
	dbtartifact "github.com/iiwish/semlia/internal/adapters/discovery/dbt"
	fileartifact "github.com/iiwish/semlia/internal/adapters/discovery/files"
	postgreslive "github.com/iiwish/semlia/internal/adapters/discovery/postgreslive"
	postgresdiscovery "github.com/iiwish/semlia/internal/adapters/discovery/postgresql"
	"github.com/iiwish/semlia/internal/adapters/gitcontent"
	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/internal/application"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	discoveryapp "github.com/iiwish/semlia/internal/application/discovery"
	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	govapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/application/governance/llm"
	identityapp "github.com/iiwish/semlia/internal/application/identity"
	ingestionapp "github.com/iiwish/semlia/internal/application/ingestion"
	"github.com/iiwish/semlia/internal/application/jobs"
	operationsapp "github.com/iiwish/semlia/internal/application/operations"
	projectionapp "github.com/iiwish/semlia/internal/application/projection"
	workbenchapp "github.com/iiwish/semlia/internal/application/workbench"
	"github.com/iiwish/semlia/internal/domain"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	discoverydomain "github.com/iiwish/semlia/internal/domain/discovery"
	gov "github.com/iiwish/semlia/internal/domain/governance"
	ingestiondomain "github.com/iiwish/semlia/internal/domain/ingestion"
	operationsdomain "github.com/iiwish/semlia/internal/domain/operations"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/sdk/trace"
)

type acceptanceConfig struct {
	Owner, Root, DatabaseURL, Address string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	c := acceptanceConfig{Owner: os.Getenv("SEMLIA_ACCEPTANCE_OWNER"), Root: os.Getenv("SEMLIA_ACCEPTANCE_ROOT"), DatabaseURL: os.Getenv("SEMLIA_ACCEPTANCE_DATABASE_URL"), Address: os.Getenv("SEMLIA_ACCEPTANCE_ADDRESS")}
	if err := run(ctx, c); err != nil {
		fmt.Fprintln(os.Stderr, "production acceptance failed:", err)
		os.Exit(1)
	}
}

type suggestionStub struct{}

func (suggestionStub) Protocol() gov.ModelProviderProtocol { return gov.ProtocolOpenAICompatible }
func (suggestionStub) Complete(ctx context.Context, req llm.CompleteRequest) (llm.CompleteResponse, error) {
	if err := ctx.Err(); err != nil {
		return llm.CompleteResponse{}, err
	}
	if len(req.Messages) != 2 {
		return llm.CompleteResponse{}, errors.New("expected bounded production prompt")
	}
	var prompt struct {
		Targets []map[string]any `json:"targetSkeleton"`
	}
	if err := json.Unmarshal([]byte(req.Messages[1].Content), &prompt); err != nil {
		return llm.CompleteResponse{}, err
	}
	for _, target := range prompt.Targets {
		if target["kind"] != "semantic_asset" {
			continue
		}
		content, ok := target["content"].(map[string]any)
		if !ok {
			return llm.CompleteResponse{}, errors.New("missing target content")
		}
		content["definition"] = "Protocol stub suggestion: include all order statuses. Human correction required."
		content["scope"] = "Acceptance fixture source only"
	}
	output, err := json.Marshal(map[string]any{"schemaVersion": gov.ProductionSuggestionsSchema, "targets": prompt.Targets})
	return llm.CompleteResponse{Content: string(output), FinishReason: "stop", Usage: llm.Usage{PromptTokens: 100, CompletionTokens: 100}}, err
}

func run(ctx context.Context, c acceptanceConfig) error {
	if err := c.validate(); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", c.Address)
	if err != nil {
		return errors.New("acceptance HTTP port is unavailable")
	}
	defer listener.Close()
	migrator, err := pgstore.NewMigrator(c.DatabaseURL, "migrations")
	if err != nil {
		return errors.New("acceptance migrator initialization failed")
	}
	err = migrator.Up()
	closeErr := migrator.Close()
	if err != nil || closeErr != nil {
		return errors.New("acceptance migrations failed")
	}
	pool, err := pgstore.Open(ctx, c.DatabaseURL)
	if err != nil {
		return errors.New("acceptance database unavailable")
	}
	defer pool.Close()
	store := pgstore.NewStore(pool)
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	authorizer := authapp.NewService(store, authapp.ClockFunc(time.Now))
	password := fmt.Sprintf("%x", secret)
	identities, err := identityapp.NewLocalService(store, secret, identityapp.WithAuthorizer(authorizer))
	if err != nil {
		return err
	}
	if _, err := identities.BootstrapLocalAdministrator(ctx, c.Owner, "Production Acceptance", "admin", password); err != nil {
		return err
	}
	login := func(ctx context.Context, role string) (identityapp.LoginComplete, error) {
		return identities.PasswordLogin(ctx, role, password, "127.0.0.1")
	}
	adminLogin, err := login(ctx, "admin")
	if err != nil {
		return err
	}
	admin, err := identities.Authenticate(ctx, adminLogin.SessionToken)
	if err != nil {
		return err
	}
	if len(admin.Session.Memberships) != 1 {
		return errors.New("expected one isolated workspace")
	}
	membership := admin.Session.Memberships[0]
	workspace := membership.WorkspaceID
	principals := map[string]identity.PrincipalID{"admin": membership.PrincipalID}
	for _, role := range []string{"author", "reviewer", "publisher"} {
		roleID := role
		if role == "author" {
			roleID = "semantic_steward"
		}
		if _, err := identities.CreatePasswordMember(ctx, workspace, membership.PrincipalID, role, role, password, roleID, strings.Repeat("a", 32)); err != nil {
			return fmt.Errorf("initialize %s invitation: %w", role, err)
		}
		completed, err := login(ctx, role)
		if err != nil {
			return err
		}
		session, err := identities.Authenticate(ctx, completed.SessionToken)
		if err != nil {
			return err
		}
		if len(session.Session.Memberships) != 1 {
			return errors.New("missing acceptance membership")
		}
		principals[role] = session.Session.Memberships[0].PrincipalID
	}
	agent, err := store.WorkspaceAgentPrincipal(ctx, workspace)
	if err != nil {
		return err
	}
	for _, grant := range []struct {
		principal identity.PrincipalID
		role      string
	}{{principals["author"], "source_operator"}, {principals["reviewer"], "auditor"}, {principals["publisher"], "auditor"}, {agent.ID, "source_operator"}, {agent.ID, "semantic_steward"}} {
		id, err := identity.NewBindingID()
		if err != nil {
			return err
		}
		if _, err := store.CreateRoleBinding(ctx, auth.RoleBinding{ID: id, PrincipalID: grant.principal, RoleID: grant.role, ScopeType: auth.ScopeWorkspace, ScopeID: workspace.UUID()}); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	providerID, err := identity.NewModelProviderID()
	if err != nil {
		return err
	}
	provider, err := store.CreateModelProvider(ctx, gov.ModelProvider{ID: providerID, WorkspaceID: workspace, Protocol: gov.ProtocolOpenAICompatible, DisplayName: "Protocol stub (no paid calls)", CredentialEnv: "ACCEPTANCE_STUB_ONLY", CredentialRevision: "sha256:" + strings.Repeat("a", 64), Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return err
	}
	settingID, err := identity.NewModelSettingID()
	if err != nil {
		return err
	}
	setting, err := store.CreateModelSetting(ctx, gov.ModelSetting{ID: settingID, WorkspaceID: workspace, ProviderID: provider.ID, Kind: gov.ModelKindLLM, Model: "protocol-fixture", Capability: "production suggestions", Enabled: true, TokenLimit: 2048, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return err
	}
	revision, err := gov.ProductionGenerationModelRevision(setting, provider)
	if err != nil {
		return err
	}
	grants := []gov.ProductionGenerationGrant{{WorkspaceID: workspace, PrincipalID: principals["author"], ModelSettingID: setting.ID, ModelConfigRevision: revision, PricingBasis: "deterministic protocol stub; no paid calls", MaxInputBytes: 1 << 20, MaxOutputTokens: 2048, MaxCostMicros: 2000000, InputMicrosPerByte: 1, OutputMicrosPerToken: 1}}
	generation := govapp.NewProductionGenerationService(store, grants, govapp.WithProductionGenerationProtocolStub(func(gov.ModelProvider, llm.CredentialResolver, *http.Client) (llm.ProviderClient, error) {
		return suggestionStub{}, nil
	}))
	artifacts, err := localartifacts.New(filepath.Join(c.Root, "artifacts"))
	if err != nil {
		return err
	}
	loader, err := discoveryapp.NewArtifactLoader(filepath.Join(c.Root, "artifacts"))
	if err != nil {
		return err
	}
	cipher, err := discoveryapp.NewCredentialCipher(secret)
	if err != nil {
		return err
	}
	adapters := map[ingestiondomain.ArtifactKind]discoverydomain.Adapter{ingestiondomain.ArtifactCSV: fileartifact.CSV(), ingestiondomain.ArtifactXLSX: fileartifact.XLSX(), ingestiondomain.ArtifactMarkdown: fileartifact.Markdown(), ingestiondomain.ArtifactSQL: postgresdiscovery.Adapter{}, ingestiondomain.ArtifactDBTManifest: dbtartifact.Adapter{}, ingestiondomain.ArtifactDBTCatalog: dbtartifact.Catalog()}
	discovery := discoveryapp.NewControlService(store, cipher, postgreslive.NewLiveCollector(5*time.Second, 15*time.Second), loader, postgresdiscovery.Adapter{}, authorizer, discoveryapp.ClockFunc(time.Now)).WithArtifactStore(artifacts, adapters).WithSnapshotCursorSecret(secret)
	ingestion := ingestionapp.NewService(store, artifacts, authorizer, ingestionapp.ClockFunc(time.Now), adapters).WithSQLArtifactLoader(loader)
	clock := govapp.ClockFunc(time.Now)
	proposals := govapp.NewProposalService(store, clock)
	policy := govapp.NewPolicyService(store, clock, govapp.WithRuleSource(store))
	authoring := govapp.NewAuthoringService(store, proposals, govapp.NewAgentRunService(store, clock), authorizer, clock, govapp.WithValidationOrchestrator(govapp.NewValidationOrchestrator(proposals, store, clock)), govapp.WithDecisionRefresher(govapp.NewPolicyDecisionTrigger(store, policy)))
	govapp.WithModelConfig(govapp.NewModelConfigService(store, authorizer, clock))(authoring)
	traces := trace.NewTracerProvider()
	defer traces.Shutdown(context.Background())
	system := application.NewSystemService(application.ReadinessProbeFunc(pool.Ping), domain.SystemInfo{APIVersion: "v1", SchemaVersion: "0.9.0", BuildVersion: "production-acceptance-protocol-stub"})
	api := httpapi.NewHandler(system, slog.Default(), traces.Tracer("production-acceptance"), httpapi.WithIdentity(identities, httpapi.IdentityHTTPConfig{OIDCEnabled: false, SecureCookie: false, AllowedOrigins: []string{"http://" + c.Address}}), httpapi.WithAuthorization(authorizer), httpapi.WithCatalog(catalogapp.NewService(store, catalogapp.ClockFunc(time.Now), catalogapp.WithAuthorizer(authorizer))), httpapi.WithWorkbench(workbenchapp.NewService(store, authorizer, workbenchapp.ClockFunc(time.Now))), httpapi.WithOperations(operationsapp.NewService(store, authorizer, operationsapp.ClockFunc(time.Now), operationsdomain.DeploymentStatus{WorkerConfigured: true, OIDCConfigured: false, EncryptionConfigured: true})), httpapi.WithDistribution(distributionapp.NewService(store, authorizer, distributionapp.ClockFunc(time.Now))), httpapi.WithDiscovery(discovery), httpapi.WithIngestion(ingestion, ingestionapp.NewScheduleService(store, authorizer, ingestionapp.ClockFunc(time.Now))), httpapi.WithGovernance(authoring), httpapi.WithProduction(govapp.NewProductionService(store)), httpapi.WithProductionGeneration(generation))
	backoff := jobs.BackoffFunc(func(int32) time.Duration { return time.Second })
	worker := jobs.NewWorker(store, jobs.ClockFunc(time.Now), backoff, 30*time.Second)
	worker.Register(govapp.ProductionGenerationJobType, generation.JobHandler())
	worker.Register(govapp.ValidationJobType, govapp.NewValidationJobHandler(store, proposals, govapp.NewValidationService(store, clock), govapp.NewDefaultRegistry(), policy, clock).Handle)
	worker.Register(discoveryapp.DiscoveryJobType, discovery.JobHandler())
	writer, err := gitcontent.Open(filepath.Join(c.Root, "content"))
	if err != nil {
		return err
	}
	router := jobs.NewRouterPublisher()
	router.Register(projectionapp.CatalogAssetChanged, projectionapp.NewPublisher(store, writer))
	router.Register(projectionapp.ReleasePublished, projectionapp.NewReleasePublisher(store, writer))
	dispatcher := jobs.NewDispatcher(store, router, jobs.ClockFunc(time.Now), backoff, 30*time.Second)
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/health/", api)
	files := http.FileServer(http.Dir("web/dist"))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/favicon.svg" {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, "web/dist/index.html")
	})
	bootstrap, err := json.Marshal(map[string]any{"workspaceId": workspace, "principals": principals, "modelSettingId": setting.ID, "modelConfigRevision": revision, "mode": "protocol_stub", "password": password})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.Root, "bootstrap.json"), bootstrap, 0600); err != nil {
		return err
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, 3)
	go func() { results <- worker.Run(runCtx, c.Owner+"-jobs", 250*time.Millisecond) }()
	go func() { results <- dispatcher.Run(runCtx, c.Owner+"-outbox", 250*time.Millisecond) }()
	go func() { results <- server.Serve(listener) }()
	var first error
	consumed := 0
	select {
	case first = <-results:
		consumed = 1
	case <-ctx.Done():
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	for ; consumed < 3; consumed++ {
		err := <-results
		if first == nil && err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) {
			first = err
		}
	}
	if errors.Is(first, http.ErrServerClosed) || errors.Is(first, context.Canceled) {
		return nil
	}
	return first
}

func (c acceptanceConfig) validate() error {
	if !regexp.MustCompile(`^spacc_[a-f0-9]{16}$`).MatchString(c.Owner) {
		return errors.New("a unique acceptance owner is required")
	}
	if !filepath.IsAbs(c.Root) || filepath.Base(c.Root) != c.Owner {
		return errors.New("run root must belong to the acceptance owner")
	}
	info, err := os.Lstat(c.Root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("run root must be an existing non-symlink directory")
	}
	marker, err := os.ReadFile(filepath.Join(c.Root, ".owner"))
	if err != nil || string(marker) != c.Owner {
		return errors.New("run root ownership marker is missing or mismatched")
	}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || u.Scheme != "postgres" || u.Hostname() != "127.0.0.1" || u.User == nil || u.User.Username() != c.Owner || u.Path != "/"+c.Owner {
		return errors.New("database must use the owned loopback database and role")
	}
	if u.RawQuery != "sslmode=disable" || u.Fragment != "" {
		return errors.New("database URL may only specify sslmode=disable")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1024 || port > 65535 || port == 5432 {
		return errors.New("database requires a dedicated non-default port")
	}
	host, addressPort, err := net.SplitHostPort(c.Address)
	if err != nil || host != "127.0.0.1" {
		return errors.New("acceptance HTTP must bind explicitly to loopback")
	}
	port, err = strconv.Atoi(addressPort)
	if err != nil || port < 1024 || port > 65535 {
		return errors.New("acceptance HTTP requires an explicit unprivileged port")
	}
	return nil
}
