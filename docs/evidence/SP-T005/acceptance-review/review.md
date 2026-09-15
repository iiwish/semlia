# T005 验收复核

结论：不能验收；T006 不启动。用户的条件授权为无问题时验收 T005 并完成 T006，当前条件不成立。

## P1：没有业务规则证据的生成口径通过确定性验证

位置：`internal/application/governance/production_validation_worker.go:147` 的语义资产检查仅判断 definition/scope 是否为 null；后续 policy 检查把 decision 写为 info finding，没有将缺失业务规则证据转为 blocker。

真实反例：固定来源只有 orders 表与 id 字段，没有收入、退款、税费的业务规则。协议替身返回新增 metric sales.revenue，定义为 Revenue excludes refunds and taxes，范围为 All completed orders，input.evidence 与 target.evidenceIds 都为空。生成成功，经正常人工 PUT 应用后 submit，再运行真实生产验证 worker，最终 status=succeeded；断言要求 failed，实际失败。

这不是实际模型生成质量问题，也不是请求执行真实模型的必要条件。确定性层接受了没有依据的业务口径，违背 production contract §8 的缺失业务规则保持未决要求，以及 data-model §4 的 submit 验证阻止缺依据规则发布要求。新增 null 检查只能阻止显式未决字段，不能阻止模型用非空文本代替业务依据。

原有测试覆盖 null、结构错误和权限撤销，但成功建议均由协议夹具提供，未包含非空口径却缺失业务证据的反例。因此早前绿色日志不覆盖本问题。缺少业务证据判断在既有共享验证路径也存在；T005 的新生成路径使其成为当前任务明确验收条件的缺口。

修复方向：在共享验证输入及检查中区分来源观察与经授权主体确认的业务规则证据，验证固定证据及适用目标；缺证据时保留未决/产生 blocker，不能仅靠非空字符串或 prompt 指令。补充无证据、仅来源观察、无有效确认、有效规则证据的对照测试。若现有契约不足以确定确认记录的字段与权威来源，先明确该契约，不自行发明 attestation 格式。

## 验证方式

命令：

```sh
go test -overlay=docs/evidence/SP-T005/acceptance-review/overlay.json -count=1 -p=1 -timeout=10m ./tests/integration/governance -run '^TestProductionGenerationReviewMissingBusinessRule$'
```

结果：exit 1，13.724s，`metric with no business-rule evidence passed deterministic validation: succeeded`。见 [原始日志](missing-rule.log)。

使用 Go overlay 将反例附加到现有真实集成夹具，没有修改产品代码或正式测试文件。模型仅为明确标识的本地协议替身，数据库为自动清理的一次性 PostgreSQL 18。没有执行 publish，没有实际模型调用、部署或 Git 操作。本次复核不重复宣称运行全量回归，也不冒充独立代理审查。
