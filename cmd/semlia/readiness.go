package main

import (
	"context"
	"strings"

	"github.com/iiwish/semlia/internal/application"
	ingestiondomain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/internal/platform/schema"
	"github.com/jackc/pgx/v5/pgxpool"
)

const requiredMigrationVersion = schema.MigrationVersion

const productionGenerationSchemaCheck = `
SELECT to_regclass('public.production_generation_requests') IS NOT NULL
AND NOT EXISTS (
 SELECT 1 FROM (VALUES ('production_generation_request_guard'),('production_generation_result_guard')) AS required(name)
 WHERE NOT EXISTS (SELECT 1 FROM pg_trigger t WHERE t.tgrelid=to_regclass('public.production_generation_requests') AND t.tgname=required.name AND t.tgenabled IN ('O','A'))
)
AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='production_generation_links' AND column_name='model_config_revision' AND data_type='text')
`

const productionAuthoringSchemaCheck = `
WITH expected_tables(table_name) AS (VALUES
 ('production_operations'),
 ('production_versions'),
 ('production_targets'),
 ('production_candidate_links'),
 ('production_contributors'),
 ('production_identity_reservations'),
 ('production_commands'),
 ('production_request_claims'),
 ('production_generation_links'),
 ('production_generation_outputs'),
 ('production_generation_applications')
 ,('production_business_rule_events')
)
SELECT NOT EXISTS (
 SELECT 1 FROM expected_tables e WHERE to_regclass('public.'||e.table_name) IS NULL
) AND EXISTS (
 SELECT 1 FROM information_schema.columns WHERE table_name='proposals' AND column_name='production_operation_id'
)
AND NOT EXISTS (
 SELECT 1 FROM (VALUES ('canonical_input'),('canonical_declarations'),('canonical_baseline'),('baseline_head'),('baseline_digest'),('history_quality')) AS required(column_name)
 WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns c WHERE c.table_schema='public' AND c.table_name='production_versions' AND c.column_name=required.column_name)
)
AND EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid=to_regclass('public.production_versions') AND conname='production_versions_canonical_shape' AND convalidated)
AND EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid=to_regclass('public.production_targets') AND conname='production_targets_proposal_fkey' AND contype='f' AND confrelid=to_regclass('public.proposals'))
AND NOT EXISTS (
 SELECT 1 FROM (VALUES ('production_versions','production_versions_immutable'),('production_targets','production_targets_immutable'),('production_commands','production_commands_immutable'),('production_candidate_links','production_candidate_links_immutable'),('production_operations','production_operations_version_guard'),
 ('production_versions','production_versions_verified_insert'),('production_versions','production_versions_complete'),
 ('production_targets','production_targets_sealed'),('production_candidate_links','production_links_sealed'),
 ('production_identity_reservations','production_reservations_identity_guard'),('production_contributors','production_contributors_immutable'),
 ('production_request_claims','production_claims_immutable'),('production_generation_links','production_generation_links_immutable'),
 ('production_generation_outputs','production_generation_outputs_immutable'),('production_generation_applications','production_generation_applications_immutable'),
 ('production_business_rule_events','production_business_rule_events_immutable'),('production_business_rule_events','production_business_rule_event_guard'),
 ('proposals','production_proposal_content_guard'),('proposals','production_proposal_reservation_guard'),('proposals','production_proposal_transition_guard'),
 ('proposal_changes','production_proposal_changes_guard'),('semantic_assets','production_asset_write_guard'),('asset_revisions','production_revision_write_guard')) AS required(table_name,trigger_name)
 WHERE NOT EXISTS (SELECT 1 FROM pg_trigger t WHERE t.tgrelid=to_regclass('public.'||required.table_name) AND t.tgname=required.trigger_name AND t.tgenabled IN ('O','A'))
)
AND NOT EXISTS (
 SELECT 1 FROM (VALUES ('production_operations','production_operations_creator_fkey'),('production_versions','production_versions_creator_fkey'),
 ('production_commands','production_commands_principal_fkey'),('production_commands','production_commands_version_fkey'),
 ('production_contributors','production_contributors_principal_fkey'),('production_candidate_links','production_candidate_links_candidate_fkey'),
 ('production_candidate_links','production_candidate_links_decision_fkey'),('production_targets','production_targets_revision_workspace_fkey'),
 ('production_targets','production_targets_revision_asset_fkey'),('production_generation_links','production_generation_links_version_fkey'),
 ('production_generation_links','production_generation_links_run_fkey'),('production_generation_outputs','production_generation_outputs_version_fkey'),
 ('production_generation_applications','production_generation_applications_output_fkey'),('production_generation_applications','production_generation_applications_source_fkey'),
 ('production_generation_applications','production_generation_applications_actor_fkey')) AS required(table_name,constraint_name)
 WHERE NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conrelid=to_regclass('public.'||required.table_name) AND c.conname=required.constraint_name AND c.contype='f' AND c.confdeltype='r'
   AND NOT EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgconstraint=c.oid AND t.tgenabled NOT IN ('O','A')))
)
`

const productionPublishingSchemaCheck = `
WITH expected_tables(table_name) AS (VALUES
 ('production_validation_attempts'),
 ('production_validation_bindings'),
 ('production_review_bindings'),
 ('release_proposals'),
 ('production_release_manifests'),
 ('production_release_before_pins'),
 ('production_release_binding_inputs'),
 ('production_validation_seals'),
 ('production_release_integrity')
)
SELECT NOT EXISTS (
 SELECT 1 FROM expected_tables e WHERE to_regclass('public.'||e.table_name) IS NULL
) AND EXISTS (
 SELECT 1 FROM information_schema.columns WHERE table_name='releases' AND column_name='production_root_release_id'
)
AND NOT EXISTS (
 SELECT 1 FROM (VALUES ('production_validation_seals','production_validation_seal_guard'),
 ('production_validation_attempts','production_attempt_completion_guard'),
 ('production_validation_bindings','production_validation_bindings_immutable'),
 ('production_review_bindings','production_review_bindings_immutable'),
 ('release_proposals','release_proposals_immutable'),
 ('production_release_manifests','production_release_manifests_immutable'),
 ('production_release_before_pins','production_release_before_pins_immutable'),
 ('production_release_binding_inputs','production_release_binding_inputs_immutable'),
 ('releases','production_release_commit_guard'),('proposals','production_reintroduction_member_guard'),
 ('production_validation_seals','production_seal_contents_guard'),
 ('production_validation_bindings','production_validation_binding_guard'),
 ('production_review_bindings','production_review_binding_guard'),
 ('release_proposals','production_attribution_guard'),
 ('production_release_before_pins','production_before_pin_guard'),
 ('production_release_binding_inputs','production_binding_input_guard')) AS required(table_name,trigger_name)
 WHERE NOT EXISTS (SELECT 1 FROM pg_trigger t WHERE t.tgrelid=to_regclass('public.'||required.table_name) AND t.tgname=required.trigger_name AND t.tgenabled IN ('O','A'))
)
`

const sourceSnapshotSchemaCheck = `
WITH expected(table_name, columns, foreign_keys, checks) AS (VALUES
 ('source_snapshots','id,workspace_id,source_connection_id,source_revision_id,adapter_version,scope_digest,content_digest,history_quality,coverage_status,created_at',1,6),
 ('source_snapshot_runs','workspace_id,source_connection_id,run_id,snapshot_id',2,0),
 ('source_snapshot_scope','workspace_id,snapshot_id,coverage_key,selector,config_digest,status,enumeration_complete,diagnostic_codes',1,6),
 ('source_code_revisions','id,workspace_id,code_artifact_id,source_revision_id,adapter_version,path,language,blob_oid,content_bytes,content_digest,created_at',2,8),
 ('source_lineage_revisions','id,workspace_id,lineage_edge_id,source_revision_id,adapter_version,upstream_object_id,upstream_revision_id,downstream_object_id,downstream_revision_id,edge_kind,code_revision_id,confidence,content_digest,created_at',5,4),
 ('source_snapshot_members','workspace_id,snapshot_id,kind,object_id,revision_id,historical_name,historical_locator,content_digest,coverage_key,parent_object_id,parent_revision_id,dataset_object_id,dataset_revision_id,field_object_id,field_revision_id,code_object_id,code_revision_id,lineage_object_id,lineage_revision_id,parent_kind',6,5),
 ('source_snapshot_diagnostics','workspace_id,snapshot_id,ordinal,code,severity,coverage_key,locator,message',1,5),
 ('source_effective_snapshots','workspace_id,source_connection_id,scope_digest,snapshot_id,version',1,1),
 ('source_coverage_heads','workspace_id,source_connection_id,coverage_key,selector_digest,latest_attempt_run_id,latest_attempt_status,latest_verified_snapshot_id,version',2,3)
), expected_triggers(table_name, trigger_name) AS (VALUES
 ('source_snapshots','source_snapshots_immutable'),('source_snapshot_runs','source_snapshot_runs_immutable'),
 ('source_snapshot_scope','source_snapshot_scope_immutable'),('source_snapshot_members','source_snapshot_members_immutable'),
 ('source_snapshot_diagnostics','source_snapshot_diagnostics_immutable'),('source_code_revisions','source_code_revisions_immutable'),
 ('source_lineage_revisions','source_lineage_revisions_immutable'),('source_snapshot_members','source_snapshot_member_integrity'),
 ('source_snapshot_runs','source_snapshot_runs_integrity'),('source_effective_snapshots','source_effective_snapshots_valid'),
 ('source_coverage_heads','source_coverage_heads_valid'),('source_snapshot_scope','source_snapshot_scope_sealed'),
 ('source_snapshot_members','source_snapshot_members_sealed'),('source_snapshot_diagnostics','source_snapshot_diagnostics_sealed'),
 ('artifact_object_retention','artifact_object_retention_source_history')
)
SELECT NOT EXISTS (
 SELECT 1 FROM expected e WHERE to_regclass('public.'||e.table_name) IS NULL
 OR (SELECT string_agg(a.attname,',' ORDER BY a.attnum) FROM pg_attribute a WHERE a.attrelid=to_regclass('public.'||e.table_name) AND a.attnum>0 AND NOT a.attisdropped) IS DISTINCT FROM e.columns
 OR (SELECT count(*) FROM pg_constraint c WHERE c.conrelid=to_regclass('public.'||e.table_name) AND c.contype='f' AND c.convalidated AND c.confdeltype='r')<>e.foreign_keys
 OR (SELECT count(*) FROM pg_constraint c WHERE c.conrelid=to_regclass('public.'||e.table_name) AND c.contype='c' AND c.convalidated)<>e.checks
 OR NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conrelid=to_regclass('public.'||e.table_name) AND c.contype='p')
) AND NOT EXISTS (
 SELECT 1 FROM expected_triggers e WHERE NOT EXISTS (SELECT 1 FROM pg_trigger t WHERE t.tgrelid=to_regclass('public.'||e.table_name) AND t.tgname=e.trigger_name AND t.tgenabled IN ('O','A'))
) AND EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid=to_regclass('public.source_code_revisions') AND attname='content_bytes' AND atttypid='bytea'::regtype AND attnotnull)
AND EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid=to_regclass('public.source_code_revisions') AND conname='source_code_revisions_content_bytes_check' AND convalidated AND pg_get_constraintdef(oid)='CHECK (((octet_length(content_bytes) >= 1) AND (octet_length(content_bytes) <= 52428800)))')
AND (SELECT count(*) FROM pg_attribute WHERE attrelid=to_regclass('public.source_snapshot_members') AND attgenerated='s')=9
AND EXISTS (SELECT 1 FROM pg_index WHERE indexrelid=to_regclass('public.source_snapshots_read_idx') AND indisvalid)
`

type postgresReadinessProbe struct {
	pool *pgxpool.Pool
}

type deploymentReadinessProbe struct {
	database      application.ReadinessProbe
	artifactStore ingestiondomain.ArtifactStore
}

func (probe deploymentReadinessProbe) Check(ctx context.Context) error {
	if probe.database == nil || probe.artifactStore == nil || !probe.artifactStore.Configured() {
		return application.ErrDependencyUnavailable
	}
	if err := probe.database.Check(ctx); err != nil {
		return err
	}
	if err := probe.artifactStore.Check(ctx); err != nil {
		return application.ErrDependencyUnavailable
	}
	return nil
}

func configuredReadinessProbe(ctx context.Context, databaseURL string) (application.ReadinessProbe, func(), error) {
	if strings.TrimSpace(databaseURL) == "" {
		return application.UnavailableReadinessProbe{}, func() {}, nil
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, nil, err
	}
	poolConfig.MaxConns = 2
	poolConfig.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, nil, err
	}
	return postgresReadinessProbe{pool: pool}, pool.Close, nil
}

func (probe postgresReadinessProbe) Check(ctx context.Context) error {
	var version int
	var dirty bool
	if err := probe.pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty); err != nil {
		return application.ErrDependencyUnavailable
	}
	if version != requiredMigrationVersion || dirty {
		return application.ErrDependencyUnavailable
	}
	var passwordSchema bool
	if err := probe.pool.QueryRow(ctx, `SELECT to_regclass('public.local_password_credentials') IS NOT NULL AND to_regclass('public.password_login_budgets') IS NOT NULL`).Scan(&passwordSchema); err != nil || !passwordSchema {
		return application.ErrDependencyUnavailable
	}
	var schemaMatches bool
	if err := probe.pool.QueryRow(ctx, sourceSnapshotSchemaCheck).Scan(&schemaMatches); err != nil || !schemaMatches {
		return application.ErrDependencyUnavailable
	}
	var authoringMatches bool
	if err := probe.pool.QueryRow(ctx, productionAuthoringSchemaCheck).Scan(&authoringMatches); err != nil || !authoringMatches {
		return application.ErrDependencyUnavailable
	}
	var publishingMatches bool
	if err := probe.pool.QueryRow(ctx, productionPublishingSchemaCheck).Scan(&publishingMatches); err != nil || !publishingMatches {
		return application.ErrDependencyUnavailable
	}
	var generationMatches bool
	if err := probe.pool.QueryRow(ctx, productionGenerationSchemaCheck).Scan(&generationMatches); err != nil || !generationMatches {
		return application.ErrDependencyUnavailable
	}
	return nil
}
