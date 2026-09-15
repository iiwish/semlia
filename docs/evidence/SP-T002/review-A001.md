# SP-T002-A001 独立评审

| 项目 | 结果 |
| --- | --- |
| 对象 | worker/final-delta-03.json 与 final-delta-03.diff；44 个实施文件，不是 HEAD 全量脏 diff |
| Reviewer | 独立 reviewer，未参与 A001 实现 |
| UTC | 2026-09-09 07:21:36 至 07:27:39，6 分 3 秒 |
| 结论 | Needs_Fix；三个阻断，T003 尚未开放 |

## Findings

1. **P1：未解析 lineage 被标为可信完整。** `internal/adapters/postgres/source_snapshot.go:276` 按 code 首次出现去重，先出现的 warning 吞掉后追加的 `UNRESOLVED_LINEAGE` error，且诊断固定归入第一个 coverage unit。实际 SQL view 引用未观察到的 upstream 可达。PM PostgreSQL 18 复现为 `verified/complete`、warning、lineage 成员 0、effective head 1。修复须同时保留正确 unit 和最高严重度；只改 severity 仍可能降级错误的 unit。
2. **P2：down 删除尚未生成 snapshot 的权威 attempt。** `migrations/000022_semantic_production_sources.down.sql:3` 未检查 `source_coverage_heads`；实际 Begin 路径可先写 attempt。PM 构造合法独立 head 后，`heads=1,snapshots=0`，down 成功且表消失。安全拒绝须包含所有独立新增权威记录。
3. **P2：SQL 路径兼容性回退。** `internal/adapters/discovery/postgresql/adapter.go:99` 用 `sql:` 拼原路径，但 key 上限 256 字节；既有 logical path 接受 1024 字节。相同 261 字节路径，checkout 基线 adapter 成功，A001 返回 invalid discovery snapshot。新 declared-run 路径也使用相同不兼容 key；Begin 失败后 queued run 可能阻塞下一次扫描，此生命周期影响由代码推导，返修需补真实覆盖。

F3 的 PM 补查及 worker 复现还确认：既有 dbt raw_code 上限为 10 MiB，A001 的 8 MiB 保留上限拒绝合法 9/10 MiB 内容。SQL 的公开 HTTP artifact upload 拒绝上传，现有 ImportSQL 经 legacy loader，总量上限 8 MiB；50 MiB 只属于内部 Service.Stage/adapter 的既有能力，不是浏览器可用入口。A002 保留该内部边界的有界存储兼容，11 MiB 反例及 50 MiB fixture 按内部组件证据标注，不扩张公开输入。具体命令、旧基线对照、完整字节/摘要及入口拒绝回归纳入 A002 证据，仍需独立复核。

## Evidence

- PM 核验 44 个实施文件 hash 与 handoff 完全一致，均在 packet 范围；见 `pm/handoff-verification-01.json`。
- reviewer 独立运行 identity/domain/application/discovery adapters/HTTP 非 DB 测试，通过；未启动 Docker 或修改应用/正式测试。
- PM 独立串行运行新 source snapshot、migration、readiness runtime 测试，通过，见 `pm/fresh-runtime-01.json`。该 GREEN 没有观察到上述三个反例。
- F1/F2 真实数据库 RED：`pm/review-lineage-red-01.json`、`pm/review-downgrade-red-01.json`。probe 为 evidence 内 Go overlay，未改正式测试。
- F3 reviewer 先独立复现；PM 保留其 probe/overlay 至 `pm/long-path/` 并独立重跑，当前失败记录 `pm/review-long-path-red-01.json`，checkout 基线对照 `pm/review-long-path-baseline-green-01.json`。

PM probe 源文使用 `.go.txt` 后缀，由 overlay 映射为目标包中的虚拟 Go 测试；内容保持不变，不让 evidence fixture 被 `go list ./...` 当作独立应用包编译。

旧 writer 停用仍是部署前提；审查不证明旧二进制混跑安全。未发现其他已证实阻断项。A001 全部原始失败和成功记录保留；A002 仅受限返修，不授权 T003 或部署。
