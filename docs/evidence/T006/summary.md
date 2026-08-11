# T006 Evidence Summary

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T006 组装本地完整运行环境 |
| Attempt | M0-T006-A001 |
| 状态 | Accepted |
| 执行日期 | 2026-08-10 |
| Branch | main |
| Base state | `2e53e6f` plus accepted, uncommitted P001 through T005 work |
| Packet | `docs/specs/m0-foundation/packets/T006.yaml` |
| Executor | Codex direct execution; delegation was not requested and implicit sub-agents are disabled |

## 1. Scope Compliance

T006 只修改 packet 允许的 Compose、本地镜像、composition root、嵌入式 Web handler、本地脚本、smoke tests、Make targets 和 README。T003 的 `internal/platform/config` / `internal/platform/http`、T004 Web source/package/design、T005 migration/repository/job/outbox semantics、Go modules、public contracts 和产品原型均未修改。

Delivered boundaries:

- `postgres`、显式 one-shot `migrate`、`server` 和 `worker` 由 Compose 按健康依赖顺序启动。
- `migrate`、`server` 和 `worker` 使用同一个 12.3 MB release image 和同一个 Go executable；runtime 为 distroless nonroot、只读根文件系统且不含 Node.js。
- T004 Web package 在 Node build stage 生成，产物同步到 `internal/platform/web/static` 并嵌入 Go executable；API 和 Web 使用同一个 server port。
- Server startup 不执行 DDL。Readiness 只在 PostgreSQL 可连接且 migration version 为 clean revision 1 时返回 ready；liveness 只表达进程存活。
- 本地凭据只生成到 ignored `.semlia/dev.env`，权限为 `600`；提交内容没有可用密码、个人绝对路径或非 loopback host binding。
- PostgreSQL host port 默认为 `127.0.0.1:5433`，server host port 默认为 `127.0.0.1:8080`，未访问 host port 5432。

## 2. Delivered Behavior

- `make dev` 构建 Web 和单一 release image，启动完整本地栈并等待 server readiness。
- `make smoke` 验证嵌入式 Web、built asset、liveness、readiness、system info、容器角色、migration completion、worker restart、PostgreSQL failure/recovery 和 clean shutdown persistence。
- `make dev-down` 删除 project containers 和 network，但保留 PostgreSQL named volume。
- `semlia healthcheck live|ready` 在 distroless image 内执行 HTTP health check，不需要 shell、curl 或 Node runtime。
- 静态 hashed assets 使用 immutable cache；index 使用 `no-store`；缺失 asset 返回 404，client route 回退到 index。
- Worker 在 Compose JSON log 模式下输出结构化 startup/failure event，不暴露 database URL 或 driver error。

## 3. Runtime Evidence

Final image and role evidence:

```text
migrate  image sha256:f4d0fb59def0...  12.3MB  nonroot:nonroot  readonly=true
server   image sha256:f4d0fb59def0...  12.3MB  nonroot:nonroot  readonly=true
worker   image sha256:f4d0fb59def0...  12.3MB  nonroot:nonroot  readonly=true
node_runtime_files=0
```

Healthy endpoint payloads were served from the same server port:

```json
{"status":"live","traceId":"884f94f8167be0294804f3f1ee2c543e"}
{"status":"ready","traceId":"65c13c03b8a53b51182d591f4a036466"}
{"apiVersion":"v1","buildVersion":"local-2e53e6f","schemaVersion":"0.1.0","service":"semlia","traceId":"ec73245253a40ed24cb10628dcc1b035"}
```

Representative no-load resource snapshot:

```text
postgres cpu=2.36% memory=27.67MiB pids=11
server   cpu=0.00% memory=3.684MiB pids=9
worker   cpu=0.14% memory=2.785MiB pids=9
```

`Test04PostgresFailureAndRecovery` stopped the same PostgreSQL container, observed liveness 200, Web 200 and readiness 503 with `DEPENDENCY_UNAVAILABLE`, then started the same container ID and observed readiness recover to 200. `Test05ShutdownPreservesDataVolume` removed all project containers/network, retained the named volume and verified a job row after the stack restarted.

## 4. Review Results

Spec compliance: Pass with no blocking finding.

- One-image multi-role execution, explicit migration ordering, embedded Web, loopback exposure, safe local credentials and dependency recovery match packet `M0-T006-A001`.
- Server and worker startup do not call the migrator; only the Compose `migrate` role has `SEMLIA_MIGRATIONS_PATH`.
- No forbidden implementation surface changed.

Bug and code quality: Pass with no blocking finding.

- Readiness pool creation is lazy, so PostgreSQL outage cannot prevent server/Web startup; request context bounds dependency checks and pgx reconnects after recovery.
- Healthcheck errors, invalid database URLs and worker failures map to safe stable output without credential leakage.
- Static routing keeps `/api/**` and `/health/**` on the API handler, supports SPA routes and rejects missing asset paths.
- Generated credentials were scanned against the repository and found only in ignored local state.

QA acceptance: Pass for founder review.

- RED, GREEN, independent worker restart, two complete smoke runs, verbose fault-path smoke, clean shutdown, full Go tests, vet, build, contract drift and whitespace checks passed.
- Clean shutdown was verified with `semlia-local_postgres-data` retained; the same image and volume were then started again for founder review.

## 5. Diff

- Patch: `docs/evidence/T006/diff.patch`
- Patch scope: 23 T006 implementation/test files relative to the accepted T005 baseline; governance files and earlier accepted work are excluded.
- Patch SHA-256: `0759c202b6dc7b911fe4b57e40432a1bd4d38149dbdbf698b5c253beb4381a78`
- Patch lines: 1,243.
- Diff summary: 1,009 insertions, 8 deletions.

## 6. Rollback Notes

- `make dev-down` is the non-destructive runtime rollback and preserves the local PostgreSQL volume.
- Removing the T006 Compose/scripts/Web embedding changes returns the independent T003-T005 API, Web and database foundations; no schema downgrade is required.
- `.semlia/dev.env` and the named volume are local state and are intentionally not part of the patch.

## 7. Residual Risks

- A first image build requires the public Docker and npm registries. The initial `golang:bookworm` layer download stalled; the final builder uses the smaller official Alpine image and build-graph-only module downloads, but registry availability remains external.
- Build image tags are version-pinned but not digest-pinned in source. Digest policy, provenance and image scanning belong to T007 supply-chain gates.
- Readiness intentionally requires migration revision 1 for M0; a future migration task must advance the expected revision with its compatibility tests.
- `make dev-down` deliberately preserves data. Destructive local data deletion is not exposed as a root command in T006.

## 8. Acceptance Gate

Implementation, spec-compliance review, bug/code-quality review and QA acceptance are complete. Founder explicitly accepted T006 on 2026-08-10 and authorized T007 execution.
