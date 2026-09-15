# SP-T001 独立复核 A002

Reviewer：独立只读子任务 `sp_t001_review`。时间：2026-09-09 05:21:43 至 05:23:16 UTC，1 分 33 秒。四文件 SHA-256 与 `worker/A002-artifact-hashes.json` 一致；未修改文件、调用模型/数据库或部署。

| A001 finding | 结论 | 最新依据 |
| --- | --- | --- |
| P1 旧 rollback 绕过 | Fixed，契约层 | `data-model.md:163,170,177`：生产根与全部回滚后继持久化保护，旧 HTTP/service/repository 拒绝，事务/数据库继承与提交防线 |
| P2 validation attempt | Fixed，契约层 | `data-model.md:143-153`：attempt 身份、legacy/production 部分唯一索引、队列/查询键、完整 active attempt、in_review 重验和重新审核 |
| P2 原始 AI 输出恢复 | Fixed，契约层 | `data-model.md:75,217,221`：规范 JSON 字节正文/摘要/成功状态原子提交、保留与授权、人工应用版本/delta/actor |
| P2 缺席身份再生产 | Fixed，契约层 | `data-model.md:127-131`：create+reuseIdentity 的可信创建/缺席链、head/身份/registry CAS、新验证审核，保留历史并处理既有正式行 |

本轮没有发现影响 T001 详细契约接受决定的新增阻断。独立 reviewer 运行定向契约测试通过；新增四组 schema 正反例覆盖修正，不只核对文字。

PM 的最终新鲜 `go test -json -count=1 ./tests/contracts/... ./tests/repository` 也通过，95 test/subtest pass、0 fail/skip，6.342 秒，见 `pm-final-verification.result.json`。

数据库迁移、旧入口拒绝、撤权/并发、完整 attempt 重审、生成后重启恢复、重引入发布回滚和客户端兼容仍需后续实施证据。结论不标记 Accepted，不授权 SP-T002，不将静态契约通过等同于功能实现。
