# T002 Evidence Summary

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T002 建立版本化公共契约 |
| Attempt | M0-T002-A001 |
| 状态 | Accepted |
| 执行日期 | 2026-08-08 |
| Branch | main |
| Base commit | `a90f90c` |
| Packet | `docs/specs/m0-foundation/packets/T002.yaml` |

## 1. Scope Compliance

实现范围是 OpenAPI-first 的 M0 公共契约、跨语言生成、兼容性验证和 drift 检查。没有实现 HTTP handler、数据库、worker、Web 页面、Cube connector、MCP、Agent 行为或 M1 语义资产业务 schema。

Implementation files:

- `api/README.md`
- `api/openapi/semlia.v1.yaml`
- `api/gen/go/types.gen.go`
- `sdk/typescript/package.json`
- `sdk/typescript/tsconfig.json`
- `sdk/typescript/src/schema.gen.ts`
- `sdk/typescript/src/client.ts`
- `sdk/typescript/src/client.test.ts`
- `sdk/typescript/src/index.ts`
- `scripts/generate-contracts.sh`
- `tests/contracts/contract_test.go`
- `tests/contracts/fixtures/base.yaml`
- `tests/contracts/fixtures/compatible.yaml`
- `tests/contracts/fixtures/breaking.yaml`
- `Makefile`
- `go.mod`
- `go.sum`
- `pnpm-lock.yaml`

Orchestrator-owned governance and evidence files:

- `docs/specs/m0-foundation/packets/T002.yaml`
- `docs/specs/m0-foundation/tasks.md`
- `docs/evidence/T002/summary.md`
- `docs/evidence/T002/test-results.md`
- `docs/evidence/T002/diff.patch`

## 2. Delivered Behavior

- `api/openapi/semlia.v1.yaml` 是 HTTP 与共享 envelope 的唯一规范来源，固定 OpenAPI 3.0.3、API major version 和契约 bundle version。
- 公共 schema 统一 resource ID、UTC timestamp、trace ID、cursor pagination、error response、event envelope、health response 和 system info。
- `getLiveness`、`getReadiness` 和 `getSystemInfo` 提供 T003、T004 可复用的稳定操作契约。
- oapi-codegen 生成 Go transport 类型；openapi-typescript 生成 TypeScript schema，薄封装使用 openapi-fetch，不存在手写重复 DTO。
- `make contracts` 确定性生成工件；`make contracts-check` 在 schema 与提交工件漂移时失败。
- oasdiff fixtures 区分兼容新增和破坏性 path 删除；当前规范可完整解析且无外部 reference。
- TypeScript 固定为 5.9.3，与 openapi-typescript 7.13.0 的 peer contract 一致。

## 3. Contract Inventory

Schemas:

- `ApiVersion`
- `SchemaVersion`
- `ResourceId`
- `Timestamp`
- `TraceId`
- `Cursor`
- `PageInfo`
- `ErrorCode`
- `ErrorResponse`
- `EventType`
- `EventEnvelope`
- `HealthResponse`
- `SystemInfo`

Operations:

- `GET /health/live` as `getLiveness`
- `GET /health/ready` as `getReadiness`
- `GET /api/v1/system/info` as `getSystemInfo`

Validation matrix 包含 19 个合法与非法 payload case，覆盖 ID 大小写和前缀、UTC timestamp、分页边界与额外字段、错误必填字段和 code、事件类型与额外字段、health 状态以及 API version。

## 4. TDD Evidence

RED:

- 首次运行 `go test ./tests/contracts/...` 按预期失败，缺少 canonical spec、Go/TypeScript 生成工件、TypeScript client 和生成脚本。
- Review hardening 先加入 UTC offset 拒绝用例；schema 未声明 UTC pattern 时该用例按预期失败。

GREEN:

- 实现最小 OpenAPI schema、生成工具、Go 类型和 TypeScript client 后，contract validation matrix、跨语言编译和 compatibility fixtures 全部通过。
- `Timestamp` 同时使用 `date-time` format 和 `Z$` pattern，机器契约与 UTC 边界承诺一致。

REFACTOR:

- 生成脚本使用可移植临时目录，并通过独立函数比较工件，避免路径分隔符解析。
- TypeScript 从未满足 peer contract 的 7.0.2 调整为 5.9.3；冻结安装通过。
- compatibility output assertion 与 oasdiff 的 `removed` 诊断一致。
- shell syntax、race detector、vet、module tidy、frozen lockfile 和全仓测试通过。

## 5. Deterministic Generation

连续两次执行 `make contracts` 后的 SHA-256 完全一致：

| Artifact | Run 1 | Run 2 |
| --- | --- | --- |
| `api/gen/go/types.gen.go` | `ff3839ace16d6f85dcd3954769ccaeec9011a0ffdc72ad4aadc32c159c46ff49` | `ff3839ace16d6f85dcd3954769ccaeec9011a0ffdc72ad4aadc32c159c46ff49` |
| `sdk/typescript/src/schema.gen.ts` | `701bf6d975ce33382455643bb568f02d0e7f3a3e43e3cd23a8367aaa9f7d25b1` | `701bf6d975ce33382455643bb568f02d0e7f3a3e43e3cd23a8367aaa9f7d25b1` |

在已生成 TypeScript 文件中注入 drift probe 后，`make contracts-check` 按预期失败；重新执行 `make contracts` 恢复规范工件，随后 drift check 通过。

## 6. Review Results

Spec compliance: Pass.

- 所有实现文件位于 T002 packet 的 allowed files。
- 公共 schema 只包含 M0 identity、transport、health、error 和 event 基础字段。
- 未创建 M1 semantic asset、proposal、review、release、metric、dimension 或 measure schema。

Bug and code quality: Pass with no blocking finding.

- canonical spec 在禁止 external refs 的 loader 下解析和验证成功。
- 生成工件不含个人绝对路径或生成时间戳。
- 兼容新增通过，破坏性删除返回非零状态和明确诊断。

QA acceptance: Pass for T002 task scope.

- Go schema tests、race detector、全仓测试、vet 和 module tidy check 通过。
- TypeScript strict typecheck、frozen pnpm install、bootstrap、shell syntax 和生成 drift check 通过。

User acceptance: Accepted by founder on 2026-08-08.

## 7. Diff

- Patch: `docs/evidence/T002/diff.patch`
- Patch SHA-256: `bca6252ac85dde35de54d0d0c089cc9bb693950f35b0d752aac0d167d0dc297c`
- Patch lines: 1,821；使用 zero-context unified diff，避免 evidence patch 的上下文前缀被误报为 trailing whitespace。
- Implementation and governance files: 20；1,699 insertions；4 deletions。Evidence files are excluded from the patch to keep its checksum stable.

## 8. Residual Risks

- M0 固定 OpenAPI 3.0.3；升级 3.1 需要等待 Go generator 稳定支持并单独评审兼容性。
- oasdiff 作为固定 Go tool 带来较多 indirect module entries，但不会进入 Semlia 控制面运行时依赖图。
- 当前 compatibility fixtures 验证策略能力；T007 仍需在 CI 中把当前变更与已发布基线进行真实比较。
- 生成类型提供编译期边界，T003 仍必须在 HTTP ingress/egress 执行 schema 约束对应的运行时校验和错误映射。
- 本机 mise 启动时报告 Node 版本未安装，但实际 `node --version` 为固定的 `v24.15.0`，所有 Node/pnpm 验证均成功；该提示属于本机工具管理状态，不影响本任务工件。
