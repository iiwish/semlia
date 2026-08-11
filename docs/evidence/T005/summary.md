# T005 Evidence Summary

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T005 实现 PostgreSQL 迁移、job 与 outbox |
| Attempt | M0-T005-A001 |
| 状态 | Accepted |
| 执行日期 | 2026-08-10 |
| Branch | main |
| Base state | `2e53e6f` plus accepted, uncommitted P001 through T004 work |
| Packet | `docs/specs/m0-foundation/packets/T005.yaml` |
| Executor | Codex direct execution; delegation was not requested and implicit sub-agents are disabled |

## 1. Scope Compliance

T005 只修改 packet 允许的 Go composition root、PostgreSQL adapter、job application package、SQL/migration、集成测试、Go module 和 Make targets。T003 配置/HTTP、T004 Web、OpenAPI/SDK、产品原型和 M1 schema 均未修改。

Delivered boundaries:

- Migration `000001_m0_foundation` 是唯一 M0 schema revision，应用与 worker 启动不执行 DDL。
- pgx v5 connection pool、sqlc 生成查询和 golang-migrate 各自保留明确边界。
- `workspaces` 是租户根；`audit_events`、`jobs`、`outbox_events` 均要求 `workspace_id` 外键。
- Audit event 的 update/delete 由数据库 trigger 拒绝。
- Job/outbox 使用 `FOR UPDATE SKIP LOCKED`、owner/deadline lease、有界 attempt、retryable 与 dead-letter 状态。
- Audit 和 outbox 只通过同一个显式 pgx transaction 写入。
- Handler/publisher 原始错误只映射为稳定 code，不写入持久化或 CLI 输出。

## 2. Delivered Behavior

- `semlia migrate up|down|version` 在同一构建工件中执行显式迁移；重复 up 为成功 no-op，空 revision 显示 `empty`。
- `semlia worker` 连接已迁移数据库并运行 job lease loop；缺少 schema 时安全失败且不会自动创建表。
- 相同 `(workspace_id, idempotency_key)` 的并发入队返回同一个 job ID，数据库只保留一行。
- 并发 claim 同一个可用 job 只产生一个有效 owner；错误 owner 无法完成任务。
- Handler failure 按注入 backoff 重试，在 `max_attempts` 达到后进入 `dead_letter`。
- 过期 job/outbox lease 会进入 retryable 或 dead-letter；raw error 不落库。
- Outbox 只在 publisher 成功后标记 `published`；失败事件按相同有界策略处理。
- Audit 与 outbox rollback 后均不可见，commit 后同时可见。

## 3. Database Evidence

Isolated engine: `postgres:18-alpine` through Testcontainers Go v0.44.0. Tests never used the existing host port 5432 database.

Migration version 1 inventory:

```text
audit_events
jobs
outbox_events
schema_migrations
workspaces
```

The CLI migration cycle passed on a separate PostgreSQL 18 container with a random non-5432 host port:

```text
up -> version 1 (dirty=false)
down -> version empty
up -> version 1 (dirty=false)
```

With sequential scans disabled, PostgreSQL selected `jobs_claimable_idx` and `outbox_events_claimable_idx` for their ordered claim queries. Expired lease and workspace/status indexes are documented in `db/README.md`, together with bounded retention guidance.

## 4. Review Results

Spec compliance: Pass with no blocking finding.

- M0-FR-005 and M0-FR-006 behavior is covered by real PostgreSQL tests.
- Migration reversibility, tenant ownership, explicit transaction boundaries, idempotency, lease ownership, retry/dead-letter and outbox atomicity match the packet.
- No forbidden surface or host database was touched.

Bug and code quality: Pass with no blocking finding.

- sqlc output is deterministic and checked by `db-generate-check`.
- Database constraints keep attempts, lease fields, terminal timestamps, trace IDs and stable error codes consistent.
- Repeated race-enabled concurrency tests passed three times.
- Worker cancellation drains its timer without a shutdown race; CLI migration/worker errors do not echo driver details or credentials.

QA acceptance: Pass for founder review.

- Empty upgrade, repeated upgrade, current revision, downgrade and re-upgrade passed.
- Worker startup against an unmigrated database failed safely without schema change.
- Concurrent claim/idempotency, retry/dead-letter, immutable audit, transactional rollback and outbox publish paths passed.

## 5. Diff

- Patch: `docs/evidence/T005/diff.patch`
- Patch scope: 29 T005 implementation/test files relative to the accepted T003 baseline; governance files and earlier accepted work are excluded.
- Patch SHA-256: `794d8f7191f3654ab9d34566f75a9cdbaeed1a0ddae3514c0bb341bb3586eca3`
- Patch lines: 3,378.
- Migration identifiers: `000001_m0_foundation.up.sql`, `000001_m0_foundation.down.sql`.

## 6. Rollback Notes

- `semlia migrate down` removes outbox, jobs, audit, workspaces and the audit immutability function in dependency order; the golang-migrate revision table remains to record an empty version.
- Down is intentionally destructive and only runs through an explicit operator command or Make target.
- Application/worker startup never calls the migrator, so rolling application processes cannot silently change schema.

## 7. Residual Risks

- Delivery is at-least-once. There is no lease heartbeat in M0, so handlers and publishers must finish within the configured lease or tolerate replay after expiry.
- The outbox dispatcher foundation is implemented and tested but is not composed with a concrete publisher in `semlia worker`; that integration belongs to T006 or a later bounded adapter task.
- Retention periods are documented but cleanup/partition maintenance is not implemented in M0.
- Testcontainers v0.44.0 requires OpenTelemetry v1.44.0, so Go module selection advances the T003 OpenTelemetry dependency from v1.36.0; full API, race, vet and contract validation passed after the upgrade.

## 8. Acceptance Gate

T005 于 2026-08-10 获得 founder 明确接受。T006 已获继续执行授权。
