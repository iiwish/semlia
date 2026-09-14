# T006 验证结果

执行目录：仓库根目录 `.`。前端 Vitest 命令在 `web/` 执行。所有结果来自实际执行，不累计重跑数量。

## RED / GREEN

| 反例 | RED | GREEN |
| --- | --- | --- |
| 回滚发布 outbox | 真实 ReleasePublisher 拒绝 `published` + rollback parent，定向测试 exit 1，4.600s | 生产者使用 `rolled_back`；同测试 exit 0，4.591s；每个发布事件重放两次 |
| 已发布资产目录 | 权威物理记录读取报 `resource UUID must be version 7`，exit 1，4.531s；两个浏览器匹配返回 500 | 合法 pfd typed ID 和历史 UUID 双读取；同测试 exit 0，6.007s |
| 来源返回路由 | 返回后仍为 `/governance`，专项 exit 1 | 恢复 `/sources`，专项 exit 0 |
| 保留候选忽略 | 新工作区缺少忽略入口，专项 exit 1 | 显式依据、幂等请求及服务器重读，专项 exit 0；不创建生产对象 |
| 服务端恢复和写入边界 | 冷启动、缓存丢失、未知结果、版本失效、逆序请求与权限失败专项反例 | `semanticProductionRuntime.test.tsx`、`SemanticProductionPanel.test.tsx`、`LiveSourcesView.test.tsx` 随全量通过 |

## 最终命令

| 命令 | 结果 |
| --- | --- |
| `go test -count=1 -p=1 -timeout=20m ./tests/integration/governance ./tests/integration/catalog` | exit 0；governance 66.126s、catalog 5.746s |
| `go test -race -count=1 -p=1 -timeout=10m ./tests/integration/governance -run '^TestProductionCompositeFiveKindsPublishAndRepeatedRollback$'` | exit 0；8.048s |
| `go test -count=1 ./internal/application/projection ./internal/application/catalog ./internal/platform/http ./internal/testsupport/productionacceptance` | 四个包 exit 0；1.033s / 1.379s / 2.172s / 1.700s |
| `pnpm exec vitest run --maxWorkers=2` | exit 0；26 files / 228 tests；23.52s |
| `pnpm --filter @semlia/web typecheck` | exit 0 |
| `pnpm --filter @semlia/web lint` | exit 0；0 errors / 3 既有 warnings |
| `make web-embed` | exit 0；包含 tsc、Vite build 和资源同步 |
| `make web-embed-check` | exit 0；嵌入资源与源码构建一致 |
| `bash -n scripts/dev/production-acceptance.sh` | exit 0 |
| `git diff --check` | exit 0 |

## 浏览器记录

- `spacc_edcba10e920296c4`：4 passed，54.7s；两个尺寸完成十对象生产和重复回滚。
- `spacc_bef0805106d6c69a`：2 passed / 2 failed；目录匹配真实 500，保留反例。
- `spacc_6d4f15ad18a423c3`：4 passed，1.0m；增加既有资产匹配、基础 revision 固定、键盘打开/Escape 返回焦点、生产列表重开、reduced-motion 和视口宽度检查。
- `spacc_2029842c40896b74`：构建阶段因新增测试使用了不适用的 Testing Library `exact` 选项失败，未启动数据库；修正类型后构建通过。
- `spacc_72c6502d7900359d`：2 passed / 2 failed；测试在保存回执出现后、URL 更新前读取操作 ID，形成无效 GET。增加 URL 就绪等待及响应状态断言，不放宽产品断言。
- `spacc_00f9eb3be4c24c2f`：4 passed，1.4m；修正测试就绪等待，reduced-motion 计算样式通过。
- `spacc_817d46e7508d4484`：最终 4 passed，1.1m；焦点截图等待对话框进入最终可见状态，两个尺寸完整通过；owner 清理后资源为空。

## 限制

实际模型未调用，所有生成响应明确为 `protocol_stub`，费用未知不冒充零费用。完整数据库 suite 的五项旧基线失败保留 T005 说明，本轮不宣称完整仓库 `make check` 通过。三个 lint warnings 来自未修改的 KnowledgeViews/embeddingRuntime；大 chunk 提示保留，不为本任务拆分无关前端模块。

通用 `validate_delivery_artifacts.py --root . --task-id SP-T006` exit 1：该工具固定读取 `.ai-platform` 布局，本仓库使用 `docs/specs/semantic-production` 与 `docs/evidence`。未创建假路径绕过检查，按仓库契约人工核对输入、范围和证据。系统无 `python` 别名，使用 `python3` 运行。
