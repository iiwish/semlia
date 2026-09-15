package db_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const postgresImage = "postgres:18-alpine"

var (
	databaseURL string
	repoRoot    = repositoryRoot()
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := tcpostgres.Run(
		ctx,
		postgresImage,
		tcpostgres.WithDatabase("semlia_test"),
		tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start isolated PostgreSQL container: failed")
		os.Exit(1)
	}

	databaseURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "resolve isolated PostgreSQL connection: failed")
		os.Exit(1)
	}

	code := m.Run()
	if err := testcontainers.TerminateContainer(container); err != nil && code == 0 {
		fmt.Fprintf(os.Stderr, "terminate isolated PostgreSQL container: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

func TestMigrationLifecycleAndTenantSchema(t *testing.T) {
	migrator := newMigrator(t)

	if err := migrator.Up(); err != nil {
		t.Fatalf("upgrade empty database: %v", err)
	}
	assertVersion(t, migrator, 29, true)
	if err := migrator.Up(); err != nil {
		t.Fatalf("repeat upgrade: %v", err)
	}

	pool := openPool(t)
	firstInventory := tableInventory(t, pool)
	wantTables := []string{
		"local_password_credentials", "password_login_budgets",
		"actions", "agent_runs", "agent_steps", "artifact_object_retention", "artifact_objects", "asset_revisions", "attention_items", "audit_event_targets", "audit_events", "audit_exports", "authorization_events",
		"client_credentials", "code_artifacts", "consumer_bindings", "consumer_machine_principals", "consumers", "custom_role_version_actions", "custom_role_versions", "discovery_findings", "discovery_run_artifacts", "discovery_runs", "embedding_index_versions", "embedding_items", "entity_keys", "evidence_artifacts", "external_identities", "jobs",
		"join_contracts", "join_observations", "knowledge_chunks", "lineage_edges", "model_grains", "model_providers", "model_settings",
		"oidc_login_attempts", "ontology_revision_relations", "ontology_revisions",
		"outbox_events", "physical_bindings", "physical_dataset_revisions", "physical_datasets",
		"physical_field_revisions", "physical_fields", "physical_key_observations", "policy_decisions", "policy_rules", "principals", "proposal_changes",
		"proposals", "query_execution_runs", "query_validation_results", "query_validation_runs", "relation_type_policies", "release_assets", "release_execution_binding_pins", "release_execution_relations", "release_object_snapshots", "release_objects", "releases", "resolved_semantic_plans", "resource_aliases", "review_batch_members",
		"review_batches", "reviews", "revision_evidence_links", "role_actions", "role_bindings", "roles", "runtime_run_events", "runtime_runs", "runtime_settings", "schema_migrations",
		"semantic_assets", "semantic_candidate_decisions", "semantic_candidates", "semantic_queries", "semantic_refusals", "semantic_relations", "semantic_resolution_events", "sessions", "source_artifact_set_members", "source_artifact_sets", "source_artifacts", "source_connections", "source_credentials", "source_revision_artifact_sets", "source_revision_artifacts", "source_revisions", "source_schedule_command_receipts", "source_schedule_occurrences", "source_schedules", "usage_events", "user_accounts",
		"validation_results", "validation_runs", "webhook_deliveries", "webhook_fanout_receipts", "webhook_signing_secrets", "webhook_subscriptions", "workspace_artifact_usage", "workspace_invitations", "workspace_memberships", "workspaces",
		"source_snapshots", "source_snapshot_runs", "source_snapshot_scope", "source_snapshot_members", "source_snapshot_diagnostics", "source_code_revisions", "source_lineage_revisions", "source_effective_snapshots", "source_coverage_heads",
		"production_operations", "production_versions", "production_targets", "production_candidate_links", "production_contributors", "production_identity_reservations", "production_commands", "production_request_claims", "production_generation_links", "production_generation_outputs", "production_generation_applications",
		"production_validation_attempts", "production_validation_bindings", "production_review_bindings", "release_proposals", "production_release_manifests", "production_release_before_pins", "production_release_binding_inputs",
		"production_validation_seals", "production_release_integrity", "production_generation_requests", "production_business_rule_events",
	}
	sort.Strings(wantTables)
	if strings.Join(firstInventory, ",") != strings.Join(wantTables, ",") {
		t.Fatalf("table inventory = %v, want %v", firstInventory, wantTables)
	}
	t.Logf("%s migration version 29 inventory: %v", postgresImage, firstInventory)
	assertTenantForeignKeys(t, pool)

	if err := migrator.Down(); err != nil {
		t.Fatalf("downgrade database: %v", err)
	}
	assertVersion(t, migrator, 0, false)
	if err := migrator.Up(); err != nil {
		t.Fatalf("upgrade database again: %v", err)
	}
	if got := tableInventory(t, pool); strings.Join(got, ",") != strings.Join(firstInventory, ",") {
		t.Fatalf("inventory after up/down/up = %v, want %v", got, firstInventory)
	}
}

func TestPopulatedM0UpgradeAndRollbackPreserveFoundationRows(t *testing.T) {
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := migrator.Up(); err != nil {
			t.Errorf("restore latest schema: %v", err)
		}
	})
	if err := migrator.Steps(1); err != nil {
		t.Fatalf("install M0 schema: %v", err)
	}
	assertVersion(t, migrator, 1, true)

	pool := openPool(t)
	ctx := context.Background()
	createdAt := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name, created_at, updated_at)
		VALUES ('legacy_workspace', 'legacy-workspace', 'Legacy Workspace', $1, $1)`, createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_events (id, workspace_id, event_type, actor_id, payload, trace_id, created_at)
		VALUES ('legacy_audit', 'legacy_workspace', 'source.created', 'founder', '{}'::jsonb, $2, $1)`, createdAt, "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (
			id, workspace_id, job_type, payload, status, attempt, max_attempts, available_at,
			idempotency_key, last_error_code, trace_id, created_at, updated_at
		) VALUES (
			'legacy_job', 'legacy_workspace', 'discover.source', '{}'::jsonb, 'retryable', 1, 3, $1,
			'legacy-job', 'HANDLER_FAILED', $2, $1, $1
		)`, createdAt, "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_events (
			id, workspace_id, event_type, payload, status, attempt, max_attempts, available_at,
			trace_id, created_at, updated_at, published_at
		) VALUES (
			'legacy_outbox', 'legacy_workspace', 'source.created', '{}'::jsonb, 'published', 1, 3, $1,
			$2, $1, $1, $1
		)`, createdAt, "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}

	if err := migrator.Steps(16); err != nil {
		t.Fatalf("upgrade populated M0: %v", err)
	}
	assertVersion(t, migrator, 17, true)

	for _, table := range []string{"workspaces", "audit_events", "jobs", "outbox_events"} {
		var count int
		query := fmt.Sprintf("SELECT count(*) FROM %s WHERE id IS NOT NULL AND substring(id::text, 15, 1) = '7'", table)
		if err := pool.QueryRow(ctx, query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s UUIDv7 rows = %d", table, count)
		}
	}
	var relationshipCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM workspaces workspace
		JOIN audit_events audit ON audit.workspace_id = workspace.id
		JOIN jobs job ON job.workspace_id = workspace.id
		JOIN outbox_events outbox ON outbox.workspace_id = workspace.id
		WHERE workspace.legacy_id = 'legacy_workspace'
		  AND audit.legacy_id = 'legacy_audit'
		  AND job.legacy_id = 'legacy_job' AND job.status = 'retryable' AND job.attempt = 1
		  AND outbox.legacy_id = 'legacy_outbox' AND outbox.status = 'published' AND outbox.attempt = 1`).Scan(&relationshipCount); err != nil {
		t.Fatal(err)
	}
	if relationshipCount != 1 {
		t.Fatalf("preserved foundation relationships = %d", relationshipCount)
	}
	uuidEraWorkspaceID := newWorkspaceID(t)
	newAuditID := newEventID(t)
	newJobID := newRunID(t)
	newOutboxID := newEventID(t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name) VALUES ($1, 'uuid-era', 'UUID Era')`, uuidEraWorkspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_events (id, workspace_id, event_type, payload, trace_id)
		VALUES ($1, $2, 'uuid.created', '{}'::jsonb, $3)`,
		newAuditID.UUID(), uuidEraWorkspaceID.UUID(), "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (id, workspace_id, job_type, payload, max_attempts, idempotency_key, trace_id)
		VALUES ($1, $2, 'uuid.job', '{}'::jsonb, 3, 'uuid-era-job', $3)`,
		newJobID.UUID(), uuidEraWorkspaceID.UUID(), "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_events (id, workspace_id, event_type, payload, max_attempts, trace_id)
		VALUES ($1, $2, 'uuid.event', '{}'::jsonb, 3, $3)`,
		newOutboxID.UUID(), uuidEraWorkspaceID.UUID(), "4bf92f3577b34da6a3ce929d0e0e4736"); err != nil {
		t.Fatal(err)
	}

	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove operations runtime schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove Full-Menu authorization schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove semantic distribution schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove source discovery schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove identity sessions: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove local UAT identities: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove model config schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove release publishing schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove review batches schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove policy rules schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove usage schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove governance objects schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove governed authoring schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove authorization schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove M1 registry schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("rollback UUIDv7 conversion: %v", err)
	}
	assertVersion(t, migrator, 1, true)
	for table, legacyID := range map[string]string{
		"workspaces": "legacy_workspace", "audit_events": "legacy_audit", "jobs": "legacy_job", "outbox_events": "legacy_outbox",
	} {
		var count int
		query := fmt.Sprintf("SELECT count(*) FROM %s WHERE id = $1", table)
		if err := pool.QueryRow(ctx, query, legacyID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s legacy row count after rollback = %d", table, count)
		}
	}
	for table, uuidID := range map[string]string{
		"workspaces": uuidEraWorkspaceID.UUID(), "audit_events": newAuditID.UUID(),
		"jobs": newJobID.UUID(), "outbox_events": newOutboxID.UUID(),
	} {
		var count int
		query := fmt.Sprintf("SELECT count(*) FROM %s WHERE id = $1", table)
		if err := pool.QueryRow(ctx, query, uuidID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s UUID-era row count after rollback = %d", table, count)
		}
	}
	for _, table := range []string{"audit_events", "jobs", "outbox_events"} {
		var count int
		query := fmt.Sprintf("SELECT count(*) FROM %s WHERE workspace_id = $1", table)
		if err := pool.QueryRow(ctx, query, uuidEraWorkspaceID.UUID()).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s UUID-era workspace relationship after rollback = %d", table, count)
		}
	}
}

func TestPostgres17MigrationLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(
		ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("semlia_pg17_test"),
		tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Errorf("terminate PostgreSQL 17 container: %v", err)
		}
	})
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := pgstore.NewMigrator(url, filepath.Join(repoRoot, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.Close() })
	if err := migrator.Up(); err != nil {
		t.Fatalf("PostgreSQL 17 upgrade: %v", err)
	}
	assertVersion(t, migrator, 29, true)
	pool, err := pgstore.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	workspaceID := newWorkspaceID(t)
	createRegistryWorkspace(t, pool, workspaceID, "pg17")
	sourceID, err := identity.NewSourceConnectionID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pgstore.NewStore(pool).CreateSourceConnection(ctx, semantic.SourceConnection{
		ID: sourceID, WorkspaceID: workspaceID, AdapterKind: "postgres", Name: "PostgreSQL 17",
		NormalizedLocator: "postgres://pg17/catalog", Status: "active", Metadata: []byte(`{}`),
	}); err != nil {
		t.Fatalf("PostgreSQL 17 repository write: %v", err)
	}
	if err := migrator.Down(); err != nil {
		t.Fatalf("PostgreSQL 17 downgrade: %v", err)
	}
	if err := migrator.Up(); err != nil {
		t.Fatalf("PostgreSQL 17 re-upgrade: %v", err)
	}
}

func TestAuthorizationMigrationRollbackDropsTemporaryGrantsWithoutChangingDurablePermissions(t *testing.T) {
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := migrator.Up(); err != nil {
			t.Errorf("restore latest schema: %v", err)
		}
	})
	if err := migrator.Steps(15); err != nil {
		t.Fatalf("install migration 15: %v", err)
	}
	pool := openPool(t)
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	createRegistryWorkspace(t, pool, workspaceID, "authorization-rollback")
	if err := migrator.Steps(1); err != nil {
		t.Fatalf("upgrade 15 to 16: %v", err)
	}
	principalID, err := identity.NewPrincipalID()
	if err != nil {
		t.Fatal(err)
	}
	bindingID, err := identity.NewBindingID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO principals (id, workspace_id, kind, display_name, status)
		VALUES ($1, $2, 'human', 'Temporary Consumer', 'active')`,
		principalID.UUID(), workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO role_bindings (
			id, workspace_id, principal_id, role_id, role_version, scope_type, scope_id, expires_at
		) VALUES ($3, $2::uuid, $1, 'consumer_developer', 1, 'workspace', $2::uuid::text, CURRENT_TIMESTAMP + interval '24 hours')`,
		principalID.UUID(), workspaceID.UUID(), bindingID.UUID()); err != nil {
		t.Fatal(err)
	}
	assertReviewerPermission := func(stage string) {
		t.Helper()
		var allowed bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM role_bindings AS binding
				JOIN role_actions AS action ON action.role_id = binding.role_id
				WHERE binding.scope_id = $1 AND binding.role_id = 'reviewer'
				  AND action.action = 'proposal.review'
			)`, workspaceID.UUID()).Scan(&allowed); err != nil {
			t.Fatal(err)
		}
		if !allowed {
			t.Fatalf("reviewer permission missing %s", stage)
		}
	}
	assertReviewerPermission("after 15 to 16 upgrade")
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("rollback 16 to 15: %v", err)
	}
	assertReviewerPermission("after rollback to 15")
	var temporary int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM role_bindings WHERE id = $1`, bindingID.UUID()).Scan(&temporary); err != nil {
		t.Fatal(err)
	}
	if temporary != 0 {
		t.Fatal("future-expiring authorization became permanent during rollback")
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatalf("reapply migration 16: %v", err)
	}
	assertReviewerPermission("after reapplying 16")
}

func TestOperationsRuntimeMigrationPopulatedDownUpPreservesOwningRows(t *testing.T) {
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := migrator.Up(); err != nil {
			t.Errorf("restore latest schema: %v", err)
		}
	})
	if err := migrator.Steps(16); err != nil {
		t.Fatalf("install version 16: %v", err)
	}
	assertVersion(t, migrator, 16, true)

	pool := openPool(t)
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	principalID, err := identity.NewPrincipalID()
	if err != nil {
		t.Fatal(err)
	}
	jobID := newRunID(t)
	auditID := newEventID(t)
	sourceID := newRunID(t)
	discoveryID := newRunID(t)
	assetID := newRunID(t)
	revisionID := newRunID(t)
	proposalID := newRunID(t)
	validationID := newRunID(t)
	agentID := newRunID(t)
	queryID := newRunID(t)
	runtimeID := newRunID(t)
	runtimeEventID := newEventID(t)
	attentionID := newEventID(t)
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"

	createRegistryWorkspace(t, pool, workspaceID, "operations-rollback")
	if _, err := pool.Exec(ctx, `
		INSERT INTO principals (id, workspace_id, kind, display_name, status, created_at)
		VALUES ($1, $2, 'human', 'Operations Owner', 'active', $3)`,
		principalID.UUID(), workspaceID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (id, workspace_id, job_type, payload, status, max_attempts, idempotency_key, trace_id, created_at, updated_at)
		VALUES ($1, $2, 'semantic.resolve', '{}'::jsonb, 'queued', 3, 'operations-owner-job', $3, $4, $4)`,
		jobID.UUID(), workspaceID.UUID(), traceID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO source_connections (id, workspace_id, adapter_kind, name, normalized_locator, created_at, updated_at)
		VALUES ($1, $2, 'postgresql_catalog', 'History source', 'postgresql://history', $3, $3)`,
		sourceID.UUID(), workspaceID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO discovery_runs (
			id, workspace_id, source_connection_id, adapter_version, status, job_id, requested_by,
			trace_id, started_at, completed_at, created_at, updated_at
		) VALUES ($1, $2, $3, '1.0.0', 'succeeded', $4, $5, $6, $7, $7, $7, $7)`,
		discoveryID.UUID(), workspaceID.UUID(), sourceID.UUID(), jobID.UUID(), principalID.String(), traceID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO semantic_assets (id, workspace_id, namespace, key, asset_type, lifecycle_state, created_at, updated_at)
		VALUES ($1, $2, 'history', 'history_metric', 'metric', 'draft', $3, $3)`,
		assetID.UUID(), workspaceID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO asset_revisions (
			id, workspace_id, asset_id, sequence, schema_version, content_digest, content, created_by, created_at
		) VALUES ($1, $2, $3, 1, '1.0.0', $4, '{}'::jsonb, $5, $6)`,
		revisionID.UUID(), workspaceID.UUID(), assetID.UUID(), "sha256:"+strings.Repeat("1", 64), principalID.String(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE semantic_assets SET current_revision_id = $1 WHERE id = $2`, revisionID.UUID(), assetID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO proposals (
			id, workspace_id, asset_id, base_revision_id, target_object_type, target_object_id,
			state, title, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, 'semantic_asset', $3, 'draft', 'History proposal', $5, $6, $6)`,
		proposalID.UUID(), workspaceID.UUID(), assetID.UUID(), revisionID.UUID(), principalID.String(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO validation_runs (
			id, workspace_id, proposal_id, validator_id, validator_version, status, started_at, finished_at
		) VALUES ($1, $2, $3, 'schema.validation', '1.0.0', 'succeeded', $4, $4)`,
		validationID.UUID(), workspaceID.UUID(), proposalID.UUID(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO agent_runs (
			id, workspace_id, principal_id, model, config_revision, input_hash, status,
			output_digest, started_at, finished_at, duration_ms, created_at
		) VALUES ($1, $2, $3, 'history-model', '1', $4, 'succeeded', $5, $6, $6, 0, $6)`,
		agentID.UUID(), workspaceID.UUID(), principalID.UUID(), "sha256:"+strings.Repeat("2", 64), "sha256:"+strings.Repeat("3", 64), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO semantic_queries (
			id, workspace_id, principal_ref, schema_version, resolver_version, canonical_request,
			request_digest, channel, trace_id, idempotency_key, outcome, created_at, finalized_at
		) VALUES ($1, $2, $3, '1.0.0', '1.0.0', '{}'::jsonb, $4, 'api', $5, 'history-query', 'resolved', $6, $6)`,
		queryID.UUID(), workspaceID.UUID(), principalID.String(), "sha256:"+strings.Repeat("4", 64), traceID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_events (id, workspace_id, event_type, actor_id, payload, trace_id, created_at)
		VALUES ($1, $2, 'operations.owner.created', $3, jsonb_build_object('sourceId', $4::text, 'outcome', 'succeeded'), $5, $6)`,
		auditID.UUID(), workspaceID.UUID(), principalID.String(), sourceID.UUID(), traceID, now); err != nil {
		t.Fatal(err)
	}

	if err := migrator.Steps(1); err != nil {
		t.Fatalf("upgrade populated 16 to 17: %v", err)
	}
	assertVersion(t, migrator, 17, true)
	assertHistoricalRuntimeBackfill(t, pool, workspaceID, sourceID.UUID())
	if _, err := pool.Exec(ctx, `INSERT INTO runtime_settings (workspace_id, updated_at) VALUES ($1, $2)`,
		workspaceID.UUID(), now); err != nil {
		t.Fatalf("populate runtime settings: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO runtime_runs (
			id, workspace_id, kind, source_type, source_id, source_version_digest,
			trace_id, idempotency_key, requested_by_principal_id, state, max_attempts, created_at, updated_at
		) VALUES ($3, $1, 'semantic_resolution', 'job', $4::uuid::text, $5, $6, 'runtime-owner-job', $7, 'queued', 3, $2, $2)`,
		workspaceID.UUID(), now, runtimeID.UUID(), jobID.UUID(), strings.Repeat("a", 64), traceID,
		principalID.UUID()); err != nil {
		t.Fatalf("populate runtime run: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO runtime_run_events (
			id, workspace_id, runtime_run_id, event_key, sequence, event_type, state, summary, created_at
		) VALUES ($3, $1, $4, 'queued', 1, 'state', 'queued', 'Queued', $2)`,
		workspaceID.UUID(), now, runtimeEventID.UUID(), runtimeID.UUID()); err != nil {
		t.Fatalf("populate runtime event: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO attention_items (
			id, workspace_id, kind, dedupe_key, state, priority, risk, target_type, target_id,
			target_route, initiator_principal_id, rule_version, title, summary, reason_code, trace_id,
			opened_at, updated_at
		) VALUES ($3, $1, 'runtime', 'runtime-owner-attention', 'open', 'high', 'high', 'runtime_run',
			$4::text, '/operations/runs/' || $4::text, $5, '1', 'Runtime needs attention',
			'Runtime needs attention', 'RUN_REVIEW_REQUIRED', $6, $2, $2)`,
		workspaceID.UUID(), now, attentionID.UUID(), runtimeID.UUID(), principalID.UUID(), traceID); err != nil {
		t.Fatalf("populate attention item: %v", err)
	}

	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("rollback 17 to 16 with populated operations state: %v", err)
	}
	assertVersion(t, migrator, 16, true)
	for name, check := range map[string]struct {
		query string
		id    string
	}{
		"workspace":  {`SELECT count(*) FROM workspaces WHERE id = $1`, workspaceID.UUID()},
		"principal":  {`SELECT count(*) FROM principals WHERE id = $1`, principalID.UUID()},
		"job":        {`SELECT count(*) FROM jobs WHERE id = $1`, jobID.UUID()},
		"audit":      {`SELECT count(*) FROM audit_events WHERE id = $1`, auditID.UUID()},
		"source":     {`SELECT count(*) FROM source_connections WHERE id = $1`, sourceID.UUID()},
		"discovery":  {`SELECT count(*) FROM discovery_runs WHERE id = $1`, discoveryID.UUID()},
		"validation": {`SELECT count(*) FROM validation_runs WHERE id = $1`, validationID.UUID()},
		"agent":      {`SELECT count(*) FROM agent_runs WHERE id = $1`, agentID.UUID()},
		"query":      {`SELECT count(*) FROM semantic_queries WHERE id = $1`, queryID.UUID()},
	} {
		var count int
		if err := pool.QueryRow(ctx, check.query, check.id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s owning row count after operations rollback = %d", name, count)
		}
	}
	var operationsTables int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_class
		WHERE relname IN ('runtime_settings', 'runtime_runs', 'runtime_run_events', 'audit_event_targets', 'audit_exports', 'attention_items')
		  AND relnamespace = 'public'::regnamespace`).Scan(&operationsTables); err != nil {
		t.Fatal(err)
	}
	if operationsTables != 0 {
		t.Fatalf("operations tables remaining after rollback = %d", operationsTables)
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatalf("reapply migration 17: %v", err)
	}
	assertVersion(t, migrator, 17, true)
	assertHistoricalRuntimeBackfill(t, pool, workspaceID, sourceID.UUID())
	if _, err := pool.Exec(ctx, `INSERT INTO runtime_settings (workspace_id, updated_at) VALUES ($1, $2)`, workspaceID.UUID(), now); err != nil {
		t.Fatalf("write operations settings after reapply: %v", err)
	}
}

func assertHistoricalRuntimeBackfill(t *testing.T, pool *pgstore.Pool, workspaceID identity.WorkspaceID, sourceID string) {
	t.Helper()
	ctx := context.Background()
	var runs int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM runtime_runs
		WHERE workspace_id = $1
		  AND kind IN ('discovery', 'validation', 'agent', 'semantic_resolution')`, workspaceID.UUID()).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 4 {
		t.Fatalf("historical runtime run count = %d, want 4", runs)
	}
	var events int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM runtime_run_events AS event
		JOIN runtime_runs AS run ON run.workspace_id = event.workspace_id AND run.id = event.runtime_run_id
		WHERE run.workspace_id = $1
		  AND run.kind IN ('discovery', 'validation', 'agent', 'semantic_resolution')`, workspaceID.UUID()).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 4 {
		t.Fatalf("historical runtime event count = %d, want 4", events)
	}
	var targets int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_event_targets
		WHERE workspace_id = $1 AND object_type = 'source' AND object_id = $2`, workspaceID.UUID(), sourceID).Scan(&targets); err != nil {
		t.Fatal(err)
	}
	if targets != 1 {
		t.Fatalf("historical audit target count = %d, want 1", targets)
	}
}

// Packet red scenario: a populated M1 registry survives the governed-authoring
// upgrade, the downgrade back to the M1 level, and the re-upgrade — with the M2
// proposal FKs composing against the preserved M1 rows in both directions.
func TestPopulatedM1UpgradeAndRollbackPreserveRegistryRows(t *testing.T) {
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := migrator.Up(); err != nil {
			t.Errorf("restore latest schema: %v", err)
		}
	})
	if err := migrator.Steps(3); err != nil {
		t.Fatalf("install M1 registry schema: %v", err)
	}
	assertVersion(t, migrator, 3, true)

	pool := openPool(t)
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	assetID, err := identity.NewAssetID()
	if err != nil {
		t.Fatal(err)
	}
	revisionID, err := identity.NewRevisionID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ($1, 'registry-era', 'Registry Era')`, workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO semantic_assets (id, workspace_id, namespace, key, asset_type, lifecycle_state)
		VALUES ($1, $2, 'finance', 'churn_rate', 'metric', 'draft')`,
		assetID.UUID(), workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO asset_revisions (id, workspace_id, asset_id, sequence, schema_version, content_digest, content, created_by)
		VALUES ($1, $2, $3, 1, '1.0.0', $4, '{"definition":{}}'::jsonb, 'steward')`,
		revisionID.UUID(), workspaceID.UUID(), assetID.UUID(), "sha256:"+strings.Repeat("ab", 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE semantic_assets SET current_revision_id = $1 WHERE id = $2`,
		revisionID.UUID(), assetID.UUID()); err != nil {
		t.Fatal(err)
	}

	if err := migrator.Steps(14); err != nil {
		t.Fatalf("upgrade populated M1 through M2: %v", err)
	}
	assertVersion(t, migrator, 17, true)

	proposalID, err := identity.NewProposalID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO proposals (
			id, workspace_id, asset_id, base_revision_id, target_object_type, target_object_id,
			state, title, created_by
		) VALUES ($1, $2, $3, $4, 'semantic_asset', $3, 'draft', 'Retune churn metric', 'steward')`,
		proposalID.UUID(), workspaceID.UUID(), assetID.UUID(), revisionID.UUID()); err != nil {
		t.Fatalf("M2 proposal must compose with preserved M1 rows: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM proposals WHERE id = $1`, proposalID.UUID()); err != nil {
		t.Fatal(err)
	}

	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove operations runtime schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove Full-Menu authorization schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove semantic distribution schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove source discovery schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove identity sessions: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove local UAT identities: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove model config schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove release publishing schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove review batches schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove policy rules schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove governance objects schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove governed authoring schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove authorization schema: %v", err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("remove usage schema: %v", err)
	}
	assertVersion(t, migrator, 3, true)
	for name, check := range map[string]struct {
		query string
		id    string
	}{
		"workspaces":      {`SELECT count(*) FROM workspaces WHERE id = $1`, workspaceID.UUID()},
		"semantic_assets": {`SELECT count(*) FROM semantic_assets WHERE id = $1`, assetID.UUID()},
		"asset_revisions": {`SELECT count(*) FROM asset_revisions WHERE id = $1`, revisionID.UUID()},
	} {
		var count int
		if err := pool.QueryRow(ctx, check.query, check.id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s row count after rollback = %d", name, count)
		}
	}

	if err := migrator.Up(); err != nil {
		t.Fatalf("re-upgrade populated M1: %v", err)
	}
	assertVersion(t, migrator, 29, true)
	store := pgstore.NewStore(pool)
	detail, err := store.GetCatalogAsset(ctx, workspaceID, assetID)
	if err != nil {
		t.Fatalf("M1 catalog read after re-upgrade: %v", err)
	}
	if detail.ID != assetID || detail.CurrentRevision == nil || detail.CurrentRevision.ID != revisionID {
		t.Fatalf("M1 catalog read = %+v, want preserved asset with current revision", detail)
	}
}

func TestClaimQueriesUsePartialIndexes(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	jobID := newRunID(t)
	outboxID := newEventID(t)

	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ($1, 'index-test', 'Index Test')`, workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (
			id, workspace_id, job_type, payload, max_attempts,
			available_at, idempotency_key, trace_id
		) VALUES (
			$1, $2, 'index.test', '{}'::jsonb, 3,
			CURRENT_TIMESTAMP, 'index-test', '4bf92f3577b34da6a3ce929d0e0e4736'
		)`, jobID.UUID(), workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_events (
			id, workspace_id, event_type, payload, max_attempts, available_at, trace_id
		) VALUES (
			$1, $2, 'index.test', '{}'::jsonb, 3,
			CURRENT_TIMESTAMP, '4bf92f3577b34da6a3ce929d0e0e4736'
		)`, outboxID.UUID(), workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "SET enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}

	assertPlanUsesIndex(t, pool, `
		SELECT id FROM jobs
		WHERE status IN ('queued', 'retryable') AND available_at <= CURRENT_TIMESTAMP
		ORDER BY available_at, created_at, id
		FOR UPDATE SKIP LOCKED LIMIT 1`, "jobs_claimable_idx")
	assertPlanUsesIndex(t, pool, `
		SELECT id FROM outbox_events
		WHERE status IN ('queued', 'retryable') AND available_at <= CURRENT_TIMESTAMP
		ORDER BY available_at, created_at, id
		FOR UPDATE SKIP LOCKED LIMIT 1`, "outbox_events_claimable_idx")
}

func TestAuditEventsAreImmutable(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	eventID := newEventID(t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ($1, 'workspace-audit', 'Workspace Audit')`, workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_events (
			id, workspace_id, event_type, actor_id, payload, trace_id
		) VALUES (
			$1, $2, 'source.created', 'founder',
			'{}'::jsonb, '4bf92f3577b34da6a3ce929d0e0e4736'
		)`, eventID.UUID(), workspaceID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE audit_events SET event_type = 'source.changed' WHERE id = $1", eventID.UUID()); err == nil {
		t.Fatal("audit update was accepted")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM audit_events WHERE id = $1", eventID.UUID()); err == nil {
		t.Fatal("audit delete was accepted")
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE id = $1", eventID.UUID()).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("immutable audit rows = %d", count)
	}
}

func TestOperationsRuntimeRejectsCrossWorkspaceLinksAndInconsistentErrors(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	workspaceA, workspaceB := newWorkspaceID(t), newWorkspaceID(t)
	principalA, _ := identity.NewPrincipalID()
	principalB, _ := identity.NewPrincipalID()
	jobB := newRunID(t)
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	createRegistryWorkspace(t, pool, workspaceA, "runtime-fk-a")
	createRegistryWorkspace(t, pool, workspaceB, "runtime-fk-b")
	for _, principal := range []struct {
		id        identity.PrincipalID
		workspace identity.WorkspaceID
	}{
		{id: principalA, workspace: workspaceA},
		{id: principalB, workspace: workspaceB},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO principals (id, workspace_id, kind, display_name, status, created_at)
			VALUES ($1, $2, 'human', 'Runtime principal', 'active', $3)`, principal.id.UUID(), principal.workspace.UUID(), now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (id, workspace_id, job_type, payload, status, max_attempts, idempotency_key, trace_id, created_at, updated_at)
		VALUES ($1, $2, 'runtime.test', '{}'::jsonb, 'queued', 1, 'runtime-fk-job', $3, $4, $4)`,
		jobB.UUID(), workspaceB.UUID(), strings.Repeat("a", 32), now); err != nil {
		t.Fatal(err)
	}
	insertRun := func(id identity.RunID, jobID, principalID any, state string, finishedAt any, errorCode any) error {
		_, err := pool.Exec(ctx, `
			INSERT INTO runtime_runs (
				id, workspace_id, kind, source_type, source_id, source_version_digest, job_id,
				idempotency_key, requested_by_principal_id, state, max_attempts, finished_at,
				error_code, created_at, updated_at
			) VALUES ($1, $2, 'validation', 'validation_run', $1::text, $3, $4, $5, $6, $7, 1, $8, $9, $10, $10)`,
			id.UUID(), workspaceA.UUID(), strings.Repeat("b", 64), jobID, "runtime-fk-"+id.UUID(), principalID,
			state, finishedAt, errorCode, now)
		return err
	}
	if err := insertRun(newRunID(t), jobB.UUID(), principalA.UUID(), "queued", nil, nil); err == nil {
		t.Fatal("cross-workspace job link was accepted")
	}
	if err := insertRun(newRunID(t), nil, principalB.UUID(), "queued", nil, nil); err == nil {
		t.Fatal("cross-workspace principal link was accepted")
	}
	if err := insertRun(newRunID(t), nil, principalA.UUID(), "succeeded", now, "SHOULD_BE_EMPTY"); err == nil {
		t.Fatal("successful runtime with error code was accepted")
	}
	if err := insertRun(newRunID(t), nil, principalA.UUID(), "failed", now, nil); err == nil {
		t.Fatal("failed runtime without error code was accepted")
	}
}

func TestMigration18ProjectsAndRollsBackIngestionAuditTargets(t *testing.T) {
	resetSchema(t)
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Steps(17); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 17, true)
	pool := openPool(t)
	ctx := context.Background()
	workspace := newWorkspaceID(t)
	event := newEventID(t)
	createRegistryWorkspace(t, pool, workspace, "ingestion-audit-migration")
	payload := `{"artifactId":"art_test","artifactSetId":"ars_test","scheduleId":"sch_test","occurrenceId":"occ_test"}`
	if _, err := pool.Exec(ctx, `INSERT INTO audit_events(id,workspace_id,event_type,actor_id,payload,trace_id)
VALUES($1,$2,'artifact.test','system',$3::jsonb,$4)`, event.UUID(), workspace.UUID(), payload, strings.Repeat("a", 32)); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertIngestionAuditTargetCount(t, pool, event, 4)
	if err := migrator.Steps(-1); err != nil {
		t.Fatalf("rollback migration 18 with immutable targets: %v", err)
	}
	assertVersion(t, migrator, 17, true)
	assertIngestionAuditTargetCount(t, pool, event, 0)
	if err := migrator.Steps(1); err != nil {
		t.Fatalf("reapply migration 18: %v", err)
	}
	assertIngestionAuditTargetCount(t, pool, event, 4)
}

func TestMigration18PreservesPopulatedVersion17PostgreSQLSourceAndRunAcrossRoundTrip(t *testing.T) {
	resetSchema(t)
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Steps(17); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 17, true)
	pool := openPool(t)
	ctx := context.Background()
	workspace := newWorkspaceID(t)
	source, err := identity.NewSourceConnectionID()
	if err != nil {
		t.Fatal(err)
	}
	run := newRunID(t)
	createRegistryWorkspace(t, pool, workspace, "ingestion-v17-roundtrip")
	if _, err := pool.Exec(ctx, `INSERT INTO source_connections
(id,workspace_id,adapter_kind,name,normalized_locator,status,metadata,artifact_paths)
VALUES($1,$2,'postgresql_catalog','Version 17 source','postgresql://db.example.test/warehouse','paused','{"schema":"analytics"}'::jsonb,'["analytics"]'::jsonb)`,
		source.UUID(), workspace.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO discovery_runs
(id,workspace_id,source_connection_id,adapter_version,status,stats)
VALUES($1,$2,$3,'postgresql-catalog/1','queued','{}'::jsonb)`, run.UUID(), workspace.UUID(), source.UUID()); err != nil {
		t.Fatal(err)
	}

	assertRoundTrip := func(stage string) {
		t.Helper()
		var sourceKind string
		var version int64
		var sourceConfig string
		var artifactSet, requestFingerprint, sourceFingerprint *string
		if err := pool.QueryRow(ctx, `SELECT source_kind,version,active_artifact_set_id::text
FROM source_connections WHERE workspace_id=$1 AND id=$2`, workspace.UUID(), source.UUID()).Scan(&sourceKind, &version, &artifactSet); err != nil {
			t.Fatalf("%s source: %v", stage, err)
		}
		if err := pool.QueryRow(ctx, `SELECT source_config::text,artifact_set_id::text,request_fingerprint,source_input_fingerprint
FROM discovery_runs WHERE workspace_id=$1 AND id=$2`, workspace.UUID(), run.UUID()).Scan(&sourceConfig, &artifactSet, &requestFingerprint, &sourceFingerprint); err != nil {
			t.Fatalf("%s run: %v", stage, err)
		}
		if sourceKind != "postgresql" || version != 1 || artifactSet != nil || requestFingerprint != nil || sourceFingerprint != nil ||
			!strings.Contains(sourceConfig, `"sourceVersion": 1`) || !strings.Contains(sourceConfig, `"artifactPaths": ["analytics"]`) {
			t.Fatalf("%s sourceKind=%s version=%d set=%v request=%v source=%v config=%s", stage,
				sourceKind, version, artifactSet, requestFingerprint, sourceFingerprint, sourceConfig)
		}
	}

	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertRoundTrip("first upgrade")
	if err := migrator.Steps(-1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 17, true)
	var sourceCount, runCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM source_connections WHERE id=$1`, source.UUID()).Scan(&sourceCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM discovery_runs WHERE id=$1`, run.UUID()).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if sourceCount != 1 || runCount != 1 {
		t.Fatalf("version 17 source count=%d run count=%d", sourceCount, runCount)
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertRoundTrip("reapplied upgrade")
}

func assertIngestionAuditTargetCount(t *testing.T, pool *pgstore.Pool, event identity.EventID, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_event_targets
WHERE audit_event_id=$1 AND object_type IN('artifact','artifact_set','source_schedule','schedule_occurrence')`, event.UUID()).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("ingestion audit target count=%d, want %d", count, want)
	}
}

func TestWorkerStartupDoesNotRunMigrations(t *testing.T) {
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := migrator.Up(); err != nil {
			t.Errorf("restore migrated schema: %v", err)
		}
	})

	pool := openPool(t)
	before := tableInventory(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", "./cmd/semlia", "worker")
	command.Dir = repoRoot
	command.Env = append(os.Environ(),
		"SEMLIA_ENV=test",
		"SEMLIA_DATABASE_URL="+databaseURL,
		"SEMLIA_SECRET_KEY="+strings.Repeat("s", 64),
		"SEMLIA_ALLOWED_ORIGINS=http://127.0.0.1:18081",
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("worker unexpectedly started against an unmigrated database")
	}
	if ctx.Err() != nil {
		t.Fatal("worker hung instead of failing on the missing schema")
	}
	if !strings.Contains(string(output), "worker error: operation failed\n") {
		t.Fatalf("worker output = %q", output)
	}
	if strings.Contains(string(output), "integration-test-only") {
		t.Fatal("worker output leaked database credentials")
	}
	after := tableInventory(t, pool)
	if strings.Join(after, ",") != strings.Join(before, ",") {
		t.Fatalf("worker startup changed schema: before=%v after=%v", before, after)
	}
}

func newMigrator(t *testing.T) *pgstore.Migrator {
	t.Helper()
	migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repoRoot, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := migrator.Close(); err != nil {
			t.Errorf("close migrator: %v", err)
		}
	})
	return migrator
}

func resetSchema(t *testing.T) {
	t.Helper()
	migrator := newMigrator(t)
	if err := migrator.Down(); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Up(); err != nil {
		t.Fatal(err)
	}
}

func openPool(t *testing.T) *pgstore.Pool {
	t.Helper()
	pool, err := pgstore.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func assertVersion(t *testing.T, migrator *pgstore.Migrator, want uint, exists bool) {
	t.Helper()
	got, dirty, err := migrator.Version()
	if !exists {
		if err != nil {
			t.Fatalf("read empty migration version: %v", err)
		}
		if got != 0 || dirty {
			t.Fatalf("empty migration version = %d dirty=%t", got, dirty)
		}
		return
	}
	if err != nil || got != want || dirty {
		t.Fatalf("migration version = %d dirty=%t err=%v, want %d clean", got, dirty, err, want)
	}
}

func tableInventory(t *testing.T, pool *pgstore.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(tables)
	return tables
}

func assertTenantForeignKeys(t *testing.T, pool *pgstore.Pool) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT DISTINCT tc.table_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.constraint_schema = kcu.constraint_schema
		JOIN information_schema.constraint_column_usage ccu
		  ON tc.constraint_name = ccu.constraint_name
		 AND tc.constraint_schema = ccu.constraint_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND kcu.column_name = 'workspace_id'
		  AND ccu.table_name = 'workspaces'
		ORDER BY tc.table_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	want := []string{
		"agent_runs", "artifact_objects", "attention_items", "audit_events", "audit_exports", "authorization_events", "client_credentials", "consumer_bindings", "consumers", "custom_role_versions", "embedding_index_versions", "entity_keys", "evidence_artifacts", "jobs",
		"join_contracts", "model_grains", "model_providers", "ontology_revisions", "outbox_events", "physical_bindings",
		"principals", "production_operations", "production_release_before_pins", "production_release_binding_inputs", "production_release_manifests", "production_review_bindings", "production_validation_attempts", "production_validation_bindings", "proposals", "query_validation_runs", "release_proposals", "releases", "resolved_semantic_plans", "review_batch_members", "review_batches", "reviews", "role_bindings", "runtime_run_events", "runtime_runs", "runtime_settings",
		"semantic_assets", "semantic_queries", "semantic_refusals", "semantic_relations", "semantic_resolution_events",
		"source_artifacts", "source_connections", "usage_events", "webhook_deliveries", "webhook_subscriptions", "workspace_artifact_usage", "workspace_invitations", "workspace_memberships",
	}
	if strings.Join(tables, ",") != strings.Join(want, ",") {
		t.Fatalf("workspace foreign keys = %v, want %v", tables, want)
	}
}

func newWorkspaceID(t *testing.T) identity.WorkspaceID {
	t.Helper()
	id, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newRunID(t *testing.T) identity.RunID {
	t.Helper()
	id, err := identity.NewRunID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newEventID(t *testing.T) identity.EventID {
	t.Helper()
	id, err := identity.NewEventID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertPlanUsesIndex(t *testing.T, pool *pgstore.Pool, query, index string) {
	t.Helper()
	rows, err := pool.Query(context.Background(), "EXPLAIN "+query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if !strings.Contains(plan.String(), index) {
		t.Fatalf("query plan does not use %s:\n%s", index, plan.String())
	}
	t.Logf("%s plan:\n%s", index, plan.String())
}

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
