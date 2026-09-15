# Local PostgreSQL UAT API Recipe

## Boundary

Read-only source inspection plus `GET http://127.0.0.1:18081/api/v1/workspaces` confirms that the local API responds. No database or HTTP mutation was performed by this worker. The sequence below is an initialization recipe for the root operator, not an execution receipt.

There is no complete HTTP-only binding initialization path in the current product. `tests/integration/governance/execution_test.go:81` uses real HTTP governance publication but creates physical bindings with an internal database fixture. Discovery candidate `convert` only associates an existing proposal and changes candidate state; it does not create a physical binding (`internal/adapters/postgres/source_discovery.go:891`). Do not present that path as automatic candidate-to-executable binding creation.

The shortest real-data path is: HTTP workspace/source/discovery; HTTP semantic asset authoring and governance release; one explicit local bootstrap of a draft physical binding using discovered IDs; HTTP governance release of that binding; HTTP resolve and execute. No source revision or release snapshot should be fabricated.

## Roles And Workspace

Base URL: `http://127.0.0.1:18081/api/v1`. Every JSON request uses `Content-Type: application/json`.

- `X-Semlia-Principal: local-author` resolves to the seeded workspace administrator, including source management, authoring, semantic resolution and execution.
- `X-Semlia-Principal: local-reviewer` resolves to a distinct seeded reviewer.
- `X-Semlia-Principal: local-publisher` resolves to a distinct seeded publisher.
- These aliases require local UAT configuration and are not production authentication. Do not submit reviewer or publisher actions as the author.

`POST /workspaces` with `{"slug":"local-pg-demo-20260906","displayName":"Local PostgreSQL Demo 20260906"}` returns `201` and public workspace `id` (`W`). Workspace INSERT triggers seed these three principals and workspace role bindings; no separate principal HTTP bootstrap is needed. On a duplicate slug, use the exact existing workspace returned by GET rather than generating unrelated duplicates. References: `internal/platform/http/catalog.go:399`, `internal/adapters/postgres/workspaces.go:30`, `migrations/000016_fmb_authorization_admin.up.sql:265`.

## Real Source And Discovery

Using the author header:

1. `POST /workspaces/W/sources` with `{"name":"Local business demo","host":"HOST_REACHABLE_FROM_SEMLIA","port":5432,"database":"semlia_demo_20260906","username":"DEDICATED_DISCOVERY_ROLE","password":"PRIVATE_INPUT_ONLY","sslMode":"disable"}`. Capture response `id` as `S`, a public `src_...` SourceConnection ID. Do not put the real password in shell history, evidence, response logs or this document.
2. `POST /workspaces/W/sources/S/test`, no JSON body. Expect `200 {"status":"succeeded"}`.
3. `POST /workspaces/W/sources/S/discovery-runs` with `{"idempotencyKey":"local-demo-discovery-20260906-1"}`. Expect `202`, capture run `id`.
4. Poll `GET /workspaces/W/discovery-runs/RUN` until terminal; require actual success and inspect stats/error codes. The running Semlia worker must process the discovery job. No test helper or direct projection insertion substitutes for this discovery.

The source response intentionally does not expose credentials or a source revision. Successful discovery creates/reuses `source_revisions` by `(source_connection_id, content_digest)` and creates/reuses physical dataset/field projections. Capture exact provenance from the discovered dataset, not the newest source revision by timestamp. Dataset fingerprints can reuse an older immutable revision.

For root's read-only metadata lookup in Semlia's metadata database (not the business database), bind actual UUIDs decoded with the existing `pkg/identity` TypeID helpers:

```sql
SELECT s.id AS source_id, s.normalized_locator,
       d.id AS dataset_id, d.qualified_name,
       dr.id AS dataset_revision_id, dr.source_revision_id,
       f.id AS field_id, f.name, fr.id AS field_revision_id,
       fr.data_type, fr.dataset_revision_id AS field_dataset_revision_id
FROM source_connections s
JOIN physical_datasets d ON d.source_connection_id=s.id AND d.workspace_id=s.workspace_id
JOIN physical_dataset_revisions dr ON dr.id=d.current_revision_id
JOIN source_revisions sr ON sr.id=dr.source_revision_id AND sr.source_connection_id=s.id
JOIN physical_fields f ON f.physical_dataset_id=d.id AND f.workspace_id=d.workspace_id
JOIN physical_field_revisions fr ON fr.id=f.current_revision_id AND fr.dataset_revision_id=dr.id
WHERE s.workspace_id=$1 AND s.id=$2 AND d.qualified_name='demo.orders'
ORDER BY fr.ordinal;
```

Use the exact numeric amount field for the first metric. `S` must come from the real HTTP source response, not a newly minted stand-in. IDs map as SourceConnection -> physical dataset -> current dataset revision -> SourceRevision; each selected field revision must belong to that same dataset revision. The projection write contract is in `db/queries/discovery.sql:1` and `:67`.

## Asset Publication

`POST /workspaces/W/catalog/assets` as author:

```json
{"address":"demo.order_amount","assetType":"metric","lifecycleState":"active","schemaVersion":"1.0.0","content":{"name":"Order amount","definition":"Recorded order amount","execution":{"aggregation":"sum"}},"createdBy":"local-author"}
```

Capture `id` as `A` and `currentRevisionId` as `V`. The compiler accepts typed `execution.aggregation`; do not add SQL expressions. For the first journey, one SUM metric from one relation avoids unrelated join/grain contracts.

`POST /workspaces/W/governance/proposals` as author:

```json
{"targetObjectType":"semantic_asset","targetObjectId":"A","baseRevisionId":"V","title":"Publish order amount","reason":"Local real-data UAT","createdBy":"local-author","changeSet":[{"fieldPath":"definition","op":"update","beforeValue":"Recorded order amount","beforeDigest":"BEFORE_DIGEST","afterValue":"Sum of recorded order amounts","afterDigest":"AFTER_DIGEST"}]}
```

Each digest is `sha256:` followed by SHA-256 of the canonical JSON value bytes, including quotes for strings, with no newline. Example local helper:

```js
const digest = value => "sha256:" + require("node:crypto").createHash("sha256").update(JSON.stringify(value)).digest("hex");
```

For every asset or binding proposal `P`, perform this shared sequence:

1. Author `POST /workspaces/W/governance/proposals/P/submit`, no body; expect `200`.
2. Poll author `GET /workspaces/W/governance/proposals/P` until `in_review`. The real worker performs validation. Inspect `GET .../P/validation-runs` and `GET .../P/policy-decision` for failures; do not bypass them.
3. Reviewer `POST .../P/reviews` with `{"decision":"approve","reason":"Independent review of local demo definition and physical source"}`; expect `201`.
4. Publisher `POST /workspaces/W/governance/releases` with `{"proposalId":"P"}`; expect `201`. Capture the returned release ID.

Reusable test helpers: `release_test.go:76` `proposeToInReview`, `:97` `approveProposal`, `:110` `publishProposal`, and `validation_test.go:18` for digests. The test's `runValidationWorker` is not an HTTP endpoint; use the running local worker.

## Explicit Draft Binding Bootstrap

The operator must disclose this single non-HTTP authoring step. Prefer the existing application service rather than inserting frozen release rows:

```go
objects := governanceapp.NewGovernedObjectService(store, governanceapp.ClockFunc(time.Now))
created, err := objects.Create(ctx, governanceapp.CreateGovernedObjectRequest{
    WorkspaceID: workspaceID, CreatedBy: "local-author",
    Object: governance.GovernedObject{
        Type: governance.TargetPhysicalBinding,
        PhysicalBinding: &governance.PhysicalBinding{
            WorkspaceID: workspaceID, AssetID: assetID, DatasetID: discoveredDatasetID,
            FieldID: &discoveredAmountFieldID, Content: json.RawMessage(`{}`),
        },
    },
})
// Capture created.PhysicalBinding.ID.String() as B; do not populate Transform.
```

This helper creates version 1 only (`internal/application/governance/object.go:59`); no release or audit claim should be fabricated. The root's temporary local bootstrap utility must use exact parsed IDs and the existing store connection, with private configuration. No product endpoint needs to be added for this demo.

Then create a binding proposal as author and run the shared validation/review/publication sequence:

```json
{"targetObjectType":"physical_binding","targetObjectId":"B","title":"Publish order amount binding","reason":"Reviewed mapping to discovered demo.orders amount","createdBy":"local-author","changeSet":[{"fieldPath":"notes","op":"add","afterValue":"Reviewed local order amount binding","afterDigest":"DIGEST_OF_JSON_STRING"}]}
```

The binding release carries already-published asset pins and freezes the binding's physical provenance. The mutable draft binding is not executable before this release. Do not INSERT `release_execution_*` or use live physical metadata to supplement a legacy release. See `execution_test.go:134` for this exact binding proposal pattern.

## Dedicated Execution Configuration

The discovery credential is not an execution credential. Root configures an explicitly separate, truly read-only execution role and matching source mapping in the Semlia server environment:

```text
SEMLIA_EXECUTION_SOURCES=[{"workspaceId":"W","sourceId":"S","dsnEnv":"SEMLIA_EXECUTION_DSN_LOCAL_DEMO"}]
SEMLIA_EXECUTION_DSN_LOCAL_DEMO=<private DSN for the dedicated execution role>
SEMLIA_EXECUTION_ALLOW_PLAINTEXT=true
```

Plaintext is only allowed in development/test. The execution DSN must match the source's exact host, port and database; no alternative host spelling, multi-host/fallback, or `sslmode=prefer`. Source creation must choose an authority usable from both discovery worker and execution server. Mapping is loaded on server startup, so root must apply it through the deployment configuration rather than only exporting it in an unrelated shell.

The role must lack elevated, inherited and settable write privileges, including database CREATE/TEMP, schema CREATE, table/column DML and sequence USAGE/UPDATE. Revoke PUBLIC TEMP in this dedicated business database if inherited. Grant only CONNECT, schema USAGE and selected table SELECT as required. Source-side logging must satisfy `log_statement=none`, `log_min_duration_statement=-1`, `log_min_duration_sample=-1`, `log_parameter_max_length_on_error=0`, `log_min_error_statement=panic`. These are checked by the adapter, not recommendations to skip its gate. Root controls role-level settings privately; never reuse the business database owner.

## Resolve, Execute And Replay

As author, `POST /workspaces/W/semantic-queries:resolve`:

```json
{"channel":"api","idempotencyKey":"local-demo-plan-20260906-1","query":{"schemaVersion":"1.0.0","intent":"aggregate","measures":[{"assetId":"A"}],"filters":[],"context":{"mode":"current"}}}
```

Require response `plan` rather than `refusal`; capture `plan.id` and `plan.planDigest`. `executionStatus=requires_execution_validation` means execution still checks current permissions/source/role/config, not guaranteed readiness. No raw SQL or physical locator is an allowed request input.

`POST /workspaces/W/resolved-semantic-plans/PLAN:execute`:

```json
{"planId":"PLAN","planDigest":"SERVER_RETURNED_DIGEST","idempotencyKey":"local-demo-execution-20260906-1","channel":"api"}
```

Inspect `run.state`, `run.errorCode`, ephemeral `rows`, and exact source/revision IDs. Compare the returned aggregate to a root-owned business SQL control query. For a second fresh query use a new execution key; the same key returns metadata-only replay and must not be interpreted as a second query or cached rows. `GET /workspaces/W/query-executions/RUN` returns metadata; `POST /workspaces/W/query-executions/RUN:cancel` requests durable cancellation.

Publish all intended assets/bindings before resolving. Later publication changes current release freshness, so resolve again with a new key rather than executing a stale plan. Keep the created workspace, discovered source, business schema/data, governed releases and private execution configuration for manual use. Real Ask additionally needs an actual configured Chat provider; REST execution success alone does not prove natural-language Ask.

## Reviewed-Execution Helper

`.semlia/local-pg-uat-20260906/governance.go` is an operator bootstrap prepared for root review, not a product API. It is fixed to workspace `wsp_01m1t6ypf8eye8yn23y0a62q9z`, slug `orbstack-demo-20260906`, and real source `src_01m1t6yphjeyevg3g39zbe9qfv`. It checks that scope in the metadata database and HTTP API before authoring. The HTTP source must remain active at `host.docker.internal:5432/semlia_demo_20260906`.

The helper creates `demo.net_revenue` from the discovered `demo.orders.net_amount` field and `demo.precision_total` from `demo.precision_samples.amount`, both SUM metrics with names explicitly labeled Demo and simulated. Each asset and binding goes through HTTP proposal, real worker validation, independent reviewer and publisher. Its only direct application mutation is `GovernedObjectService.Create` for an unpublished draft binding; it never inserts published snapshots or executes business queries.

Root injects `SEMLIA_UAT_METADATA_DSN` privately, then may run from the repository root after review:

```sh
go run .semlia/local-pg-uat-20260906/governance.go
```

`go test -p 1 .semlia/local-pg-uat-20260906/governance.go` is a compile-only check (`[no test files]`) and does not invoke `main`. The worker ran this check successfully; it did not run the bootstrap or make HTTP/database writes.

The helper atomically checkpoints IDs in a mode-0600 `governance-state.json` next to the existing source receipt, without changing `semlia-state.json`. It uses a session advisory lock to serialize concurrent instances. After uncertain transport failures, a rerun reconciles assets by exact address and marker/content, bindings by exact asset/discovered field/creator, and proposals by exact target and bootstrap title; mismatches fail closed for operator review. Reviews are reconciled using the seeded independent reviewer, and publication state is re-read. No request blindly retries a mutation. Both plans resolve only after all publications, using keys scoped to the resulting current release; the helper does not execute them. Source revisions changing after a checkpoint require manual review. Errors never include raw database driver text, DSNs, API payloads or business rows.
