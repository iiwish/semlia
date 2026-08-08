# T001 Evidence Summary

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T001 建立仓库与工具链基线 |
| Attempt | M0-T001-A002 |
| 状态 | Accepted |
| 执行日期 | 2026-08-08 |
| Branch | main |
| Base commit | `0d65b28` |
| Packet | `docs/specs/m0-foundation/packets/T001.yaml` |

## 1. Scope Compliance

实现范围是 Go 控制面与 TypeScript Web 的仓库和工具链基线。没有创建 API、Web、worker、数据库、schema、Cube connector 或 Agent 业务能力。

Implementation files:

- `.editorconfig`
- `.gitignore`
- `.tool-versions`
- `README.md`
- `LICENSE`
- `NOTICE`
- `CONTRIBUTING.md`
- `CODE_OF_CONDUCT.md`
- `SECURITY.md`
- `go.mod`
- `package.json`
- `pnpm-workspace.yaml`
- `pnpm-lock.yaml`
- `Makefile`
- `scripts/doctor.sh`
- `tests/repository/repository_contract_test.go`

Orchestrator-owned governance and evidence files:

- `docs/SSOT.md`
- `docs/adr/0001-m0-technical-foundation.md`
- `docs/specs/m0-foundation/plan.md`
- `docs/specs/m0-foundation/tasks.md`
- `docs/specs/m0-foundation/checklists/requirements.md`
- `docs/specs/m0-foundation/analysis.md`
- `docs/specs/m0-foundation/packets/T001.yaml`
- `docs/evidence/T001/summary.md`
- `docs/evidence/T001/diff.patch`
- `docs/evidence/T001/test-results.md`

## 2. Delivered Behavior

- 固定 Go 1.26.3、Node.js 24.15.0 和 pnpm 11.1.3。
- 建立 `github.com/semlia/semlia` Go module 与 pnpm workspace。
- `go.mod` 声明 Go 1.26.0 语言基线和 `go1.26.3` toolchain，repository contract 验证三处版本一致性。
- 项目基线不包含 Python 解释器、包管理器、项目文件或测试运行时。
- 提供 `make doctor`、`make bootstrap`、`make test-repository` 和 `make clean`。
- `doctor` 检查 Go module、Node.js、pnpm、Git、Make、Docker daemon 和默认开发端口。
- 保留 Apache 2.0、DCO、Contributor Covenant 3.0 和私密安全报告治理文件。
- SSOT、TDR、M0 plan、work graph、checklist、analysis 和 T001 packet 使用同一 Go + TypeScript 技术边界。

## 3. TDD Evidence

RED:

- Command: `GO111MODULE=off go test ./tests/repository`
- Result: expected failure；5 个 test case 报告缺失 `go.mod`、错误工具版本、遗留项目文件和 doctor 缺少 Go 检查。

GREEN:

- Command: `go test -v ./tests/repository`
- Result: 7 个 repository contract tests 全部通过。

REFACTOR:

- 使用标准库 `maps.Equal` 比较工具版本，避免依赖 map 格式化顺序。
- `go test -race ./tests/repository`、`go vet ./...` 和 `go mod tidy -diff` 全部通过。
- 根命令、pnpm script 和文档统一调用同一 Go test target。

## 4. Review Results

Spec compliance: Pass.

- 实现文件位于 T001 A002 packet 的 allowed files。
- 技术决策和后续 T002 至 T008 的路径、测试与迁移命令已同步为 Go。
- 没有扩大到后续 task 的业务实现。

Bug and code quality: Pass with no blocking finding.

- shell syntax、Go test、race detector、vet 和 module tidy check 通过。
- repository contract 验证工具版本、无额外语言运行时、可执行权限、开源政策和敏感内容边界。
- 重复 bootstrap 不修改 `go.mod` 或 `pnpm-lock.yaml`。

QA acceptance: Pass for T001 task scope.

- 隔离 Go module cache 且无 `node_modules` 的 bootstrap 用时 0.33 秒。
- Docker 可连接；4173、8080 和 5433 均可用；doctor 为 0 warnings、0 errors。
- `make test-repository`、`pnpm check:repository` 和 `go test ./...` 均通过。

User acceptance: Accepted by founder on 2026-08-08.

## 5. Diff

- Patch: `docs/evidence/T001/diff.patch`
- Patch SHA-256: `0b2a37baef3fb0f5a788c9051b4fbc81f8774755abbeb084436452bf18a94687`
- Patch lines: 1,409；使用 zero-context unified diff，避免 evidence patch 的上下文前缀被误报为 trailing whitespace。
- Implementation and governance files: 23；1,022 insertions；161 deletions。Evidence files are excluded from the patch to keep its checksum stable.

## 6. Residual Risks

- `.tool-versions` 会提示本机 mise 尚未管理 Node.js 24.15.0，但 `node --version` 与 doctor 验证的实际版本一致，不影响当前命令执行。
- 当前 Go module 尚无第三方依赖，因此还没有 `go.sum`；T002 引入第一个生成或契约依赖后必须提交并验证 `go.sum`。
- 当前 pnpm workspace 尚无 JavaScript 包，因此真实依赖下载、Vite 构建和浏览器测试由 T004 与 T008 覆盖。
- `github.com/semlia/semlia` 是当前规范 module path；首次公开仓库前必须确认对应组织与仓库地址，避免发布后迁移 import path。

## 7. Acceptance Gate

T001 已由用户明确接受。T002 可以在独立 execution packet 完成并通过 execute gate 后开始。
