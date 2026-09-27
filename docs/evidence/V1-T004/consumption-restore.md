# V1-T004 A002: Consumption and Recovery

Status: implemented, locally verified and independently reviewed with no remaining A002 blocker; final user acceptance is pending. This is synthetic, source-development-mode acceptance, not final RC bundle acceptance. No real model was called during T004.

## Ownership and Isolation

- Registered PostgreSQL: `semlia-local-postgres-1`; no new database container, global PostgreSQL setting or network change.
- Drill owner: `semlia_v1r_449fa84a1913cbc3`.
- Fresh databases: `semlia_v1r_449fa84a1913cbc3_install`, `semlia_v1r_449fa84a1913cbc3_upgrade`, `semlia_v1r_449fa84a1913cbc3_restore`. Each uses its own `<database>_owner` login without superuser, createdb, createrole, replication, bypass-RLS or memberships.
- Each database and role has an exact protected ownership marker. Missing, mismatched or populated restore targets fail closed. New plan/target directories reject unknown existing contents.
- Protected receipts, dumps, fixture bytes and recovery material: `.semlia/v1-acceptance/current/release-drill/`. Final inventory: 315 regular files, 174 directories, zero symbolic links and zero group/other permissions.
- All three drill databases are retained for inspection; no drill server remains running. Only the original owned V1 source-development runtime remains available at `http://127.0.0.1:52689/`, API `http://127.0.0.1:52688/`.

## Public Machine Consumption

The dedicated machine principal, role binding, consumer, pinned consumer binding, credential issuance and revocation use normal authenticated public APIs. No local-UAT identity or direct application-service shortcut is used. The canonical query comes from the existing successful real-model total Ask response; its semantic request and August half-open date bounds are unchanged. Only the public resolution context selects the dedicated pinned binding.

| Check | Result |
| --- | --- |
| REST Resolve and Execute | Passed; total 1500 |
| MCP initialize, semantic_resolve, semantic_execute | Passed; total 1500 |
| CLI semantic resolve and semantic execute | Passed; total 1500 |
| Release | `rls_01m3f2pbg0f1br2ba4hbtgjgrs` |
| Model revision | `rev_01m3ev2rhdfw2vcm1atafxx7vd` |
| All three plan digests | `sha256:1b4619391a70fa1fcc6c83cc718f28bc71f29073e21fbeaf761e2f24c7b19a30` |
| All three result digests | `sha256:d6525e6b5747fb8018dbc417c81cff426bcb0ac36151b6cb982a67bfb1975989` |
| Revoked REST/MCP | HTTP 401; no execution growth |
| Revoked CLI | Exit 1, no spawn error or signal, exact HTTP 401 diagnostic, `UNAUTHENTICATED` response |

All three results reconcile with independent SQL against the synthetic source. The positive run adds exactly three executions and zero model/agent steps. Subsequent strict revocation checks keep cumulative counts at 17 agent/model steps and 19 executions. Both issued credentials are revoked.

## Fresh Install and Upgrade

Fresh installation reaches schema 33, normal local-admin bootstrap and password login, then reads an empty catalog through the API. The isolated supported upgrade starts at schema 32 with one normal account, one workspace, five principals and one membership; migration to 33 preserves every row fingerprint in those four tables. Normal password login and catalog access succeed afterwards.

This proves a populated identity/workspace schema-32 baseline, not arbitrary historic data compatibility or the unsupported seven-type prototype migration. The user's original `semlia` database remains schema 32 with preservation checks passing. The live V1 schema-33 database was not migrated down.

## Full Backup and Restore

The owned V1 supervisor alone was stopped for a consistent snapshot. Its server, worker and Vite were restarted in source-development mode after backup. Restore uses the fresh isolated owner role; the application never runs as the PostgreSQL administrator.

| Component | Verified snapshot |
| --- | --- |
| Application custom-format dump | 1,248,933 bytes |
| Application database contents | 137 tables: exact row counts and sorted row fingerprints match before restored API activity |
| Content store | 109 files, 164,631 bytes |
| Inputs store | 1 file, 156 bytes |
| Artifact store | 2 files, 317 bytes |
| Required recovery material | Protected snapshot, byte hash checked before restoration; no values in public evidence |

File copying independently rebuilds inventories, rejects absolute/traversal paths and every symbolic-link component, requires empty destinations and checks all content hashes after copying. Recovery keys are loaded from the copied backup into the restored runtime, not silently substituted from the original environment.

Restored normal password login reads exactly 10 knowledge assets, the exact correct release/model revision above and manifest digest `sha256:8afeb8bfc0519241f23d98e06be911050db3a9ecbfd84a85019cfb8f0f79604c`. The normal Markdown artifact preview reproduces its original content hash and full preview projection. The restored PostgreSQL source `/test` succeeds through stored encrypted source credentials. The restored server has `SEMLIA_EXECUTION_SOURCES=[]` and no execution DSN variable, so this result cannot be an execution-environment credential substitute.

The external synthetic source is not part of the backup. Its actual `semlia_demo_202609` schema has customers=4 and orders=7; full row fingerprints match before and after recovery. Stored application session records are included in the full DB snapshot; browser cookie files are not copied or reused, and restored verification creates a fresh normal password session. Query result rows are ephemeral and are not claimed recoverable.

## Commands and Preserved Failures

```text
node --test scripts/acceptance/v1.test.mjs scripts/acceptance/v1-release.test.mjs
node scripts/acceptance/v1-release.mjs plan
node scripts/acceptance/v1-release.mjs provision
node scripts/acceptance/v1-release.mjs install-upgrade
node scripts/acceptance/v1-release.mjs install-upgrade-resume
node scripts/acceptance/v1-release.mjs channels
node scripts/acceptance/v1-release.mjs channels-resume
node scripts/acceptance/v1-release.mjs revocation-check
node scripts/acceptance/v1-release.mjs artifact-fixture
node scripts/acceptance/v1-release.mjs artifact-markdown
node scripts/acceptance/v1-release.mjs backup-restore
node scripts/acceptance/v1.mjs verify
node scripts/acceptance/v1.mjs status
git diff --check
```

Final focused tests: 27 passed across release, browser and baseline V1 guards, including an independent rerun. RED evidence includes initially missing startup/migration guards and CLI revocation guard exports, followed by passing behavioral tests for foreign ownership, missing recovery stores/keys, traversal/symlinks, changed/extra migration files, concurrent startup, runtime identity mismatch and false CLI refusal positives. These are guard tests; actual install/channel/recovery results above are separate live evidence.

Preserved driver failures are not counted as product successes: install inventory used the nonexistent `accounts` table after successful migration/bootstrap, then explicitly resumed with `user_accounts`; channel preparation selected a newer ledger field absent from the historical successful Ask, stopped before Resolve/Execute and revoked its credential, then explicitly resumed existing objects; the first strict revocation assertion expected a different error-code spelling and was corrected to the actual `UNAUTHENTICATED` contract. A command import-path typo failed before any request.

SQL registration succeeds but its adapter does not implement artifact content preview, so the SQL preview HTTP 422 is retained as an explicit adapter boundary. A separate ordinary Markdown upload/finalize supplies preview recovery evidence. Neither source creates knowledge assets or calls a model.

Private evidence includes `install-upgrade.json`, `install-upgrade-failed-inventory-table.json`, `channels.json`, `channels-failed-ledger-field.json`, `channels-revocation-strict.json`, `artifact-fixture-sql-unsupported.json`, `artifact-fixture.json`, `backup-restore.json`, plus per-channel responses and protected recovery files. The explicit resume commands are constrained recovery paths for these named driver failures, not generic retries.

## Candidate Entry and Remaining Gate

`node scripts/acceptance/v1.mjs candidate-check <release.json> <archive> <external-sbom> <checksums>` verifies the local bundle using `scripts/release/manifest.go verify`, the current source fingerprint, host platform, schema 33 and captured file digests. Validate this before an explicit owned `down`. Then `up-candidate` with the same four arguments installs the verified bytes atomically to owned `root/server` without Go build. A separate startup lock protects exported startup use; ready status must match requested mode/digests. Copied migrations reject extra files, content changes and links, and are checked again by the supervisor.

Candidate mode starts only server and worker, checks embedded HTML readiness and returns the direct API URL; it never starts Vite. Status records the actual binary, manifest digest and schema. Source mode remains separate and cannot claim candidate identity. Missing, symbolic, wrong-platform/schema/source inputs fail before launch. Actual versioned candidate bundle and embedded frontend acceptance remain T005, after source freeze; T004 does not claim those gates have run.

The bounded `node scripts/acceptance/v1-browser.mjs candidate-preflight` entry reuses the normal-password read-only `@preflight` tests at both supported desktop sizes. It selects the verified candidate API port, never the inactive Vite port, requires active binary/manifest/schema metadata and records identical runtime mode and digests before/after. Selector and identity-drift guards have RED/GREEN unit evidence. This command has not run against a real candidate yet; that acceptance belongs to T005. It cannot call the final-candidate model cohort or publish governance changes.

Final V1 verification: 10 assets, 3 baseline governed objects, 18 production operations, 21 releases, exact correct head, original environment preserved, 17 model/agent steps, 19 executions and no running agent job.
