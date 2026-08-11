# T003 Evidence Summary

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T003 实现控制面 API 纵向基础 |
| Attempt | M0-T003-A001 |
| 状态 | Accepted |
| 执行日期 | 2026-08-10 |
| Branch | main |
| Base state | `2e53e6f` plus the accepted, uncommitted P001 index state |
| Packet | `docs/specs/m0-foundation/packets/T003.yaml` |
| Executor | Codex direct execution; delegation was not requested |

## 1. Scope Compliance

T003 建立 Go 控制面的第一条可运行纵向路径，只修改 packet 允许的 `cmd/semlia/**`、`internal/**`、Go module 和 `Makefile`。OpenAPI、生成客户端、原型、PostgreSQL、worker、Web 和 M1 业务 schema 均未修改。

Architecture boundaries:

- `cmd/semlia` 负责 `server` / `doctor` composition root、信号处理和优雅退出。
- `internal/domain` 保留 transport-independent system identity。
- `internal/application` 定义 readiness probe 和 system service。
- `internal/platform/config` 负责环境配置与 production fail-closed 验证。
- `internal/platform/http` 负责合约响应、W3C trace context、统一错误和安全结构化日志。

## 2. Delivered Behavior

- `GET /health/live` 始终反映进程存活，不调用依赖检查。
- `GET /health/ready` 只在注入的 probe 成功时返回 `ready`。当前 runtime 默认 probe 返回 `503 DEPENDENCY_UNAVAILABLE`，等待 T005/T006 接入真实 PostgreSQL 迁移状态，不会虚报 ready。
- `GET /api/v1/system/info` 只返回 service、API version、schema version、build version 和 trace ID。
- 已知路由的错误方法、未知路由、依赖失败和 panic 分别使用稳定 JSON error envelope。
- 每个响应的 `X-Trace-ID` 与 body `traceId` 一致，合法 `traceparent` 保留原 trace ID。
- production 配置缺少 secure PostgreSQL URL、HTTPS allowed origin 或 32+ 字符 secret key 时拒绝启动，错误只列出环境变量名。
- request logs 只记录 method、route label、status、duration、trace ID 和 stable error code，不记录 query、header、credential、database URL 或 panic 内容。

## 3. Runtime Evidence

Built binary: `make build` -> `build/semlia`.

Observed responses:

```text
GET /health/live
200 {"status":"live","traceId":"a7cf9f9d129b3e6ce9a9e236703050c4"}

GET /health/ready
503 {"code":"DEPENDENCY_UNAVAILABLE",...,"traceId":"8e8d1aba0aa37003ed10bcc7050b887b"}

traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
GET /api/v1/system/info
200 {...,"traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}
```

The built binary handled `SIGINT` and exited with code 0 after graceful shutdown.

## 4. Review Results

Spec compliance: Pass.

- M0-FR-003 的三个 endpoint、统一错误与 trace correlation 已交付。
- M0-FR-007 的 production fail-closed、日志脱敏和最小 OpenTelemetry 传播已交付。
- T003 没有越界实现 T005/T006 的数据库行为。

Bug and code quality: Pass with no blocking finding.

- Race tests、全仓 Go tests、vet、contract drift、module tidy 和 build 全部通过。
- 配置与 HTTP 路径覆盖成功、依赖失败、非法配置、未知路由、错误 method 和 panic。

QA acceptance: Pass for technical review.

- 真实二进制进程已完成 live、unavailable readiness、system info 和 W3C trace 检查。
- production doctor 对安全配置返回成功，对不安全配置返回 exit 1 且未泄露测试凭据。

## 5. Diff

- Patch: `docs/evidence/T003/diff.patch`
- Patch scope: T003 runtime implementation, tests, Go dependencies and Make targets; governance and P001 changes are excluded.
- Patch SHA-256: `22e6721f91797698b15c8a32840a6067df6173295ccf99f70c34a079a632594f`
- Patch lines: 1,081.

## 6. Residual Risks

- Readiness 尚未检查真实 PostgreSQL 连接或 migration revision；这是 T005/T006 的明确交付边界，当前默认 503 是有意的安全状态。
- OpenTelemetry 已创建和传播 span，但没有 exporter 或 collector 配置；外部导出属于集成环境工作。
- Allowed origins 已 fail-closed 验证但尚未安装 CORS middleware；当前没有跨域产品 Web 流量，将由 T004/T006 集成。

## 7. Acceptance Gate

T003 于 2026-08-10 获得 founder 明确接受，状态为 `Accepted`。后续任务已获得继续执行授权。
