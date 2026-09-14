# T006 执行预检

Status: Approved
Date: 2026-09-11

T005 已按用户条件授权完成验收复核。T006 的产品目标、桌面尺寸、前端文件及隔离 Compose 验收范围已明确；执行模式为 Direct Execute，不使用子代理。

## 唯一范围缺口

真实桌面路径需要连接实际 HTTP、数据库、worker 和授权边界，并将合成 AI 响应明确记录为 `protocol_stub`。`cmd/semlia/main.go` 的服务端和 worker 均调用默认 `NewProductionGenerationService`，模式为 `actual_model`；配置没有替身切换。受控注入点为 `WithProductionGenerationProtocolStub`，现有使用处仅在 Go 集成测试中。

在外部假模型 HTTP 端点返回固定响应仍会留下 `actual_model` 归因，不能作为本次协议替身的诚实证据。浏览器拦截 API 也不能替代真实后台路径。T006 当前允许的 TypeScript、Shell 和 Compose 文件无法直接使用 Go 注入点；不通过脚本临时生成范围外 Go 源码绕过边界。

## 已批准的增加文件

- `internal/testsupport/productionacceptance/main.go`
- `internal/testsupport/productionacceptance/main_test.go`

该启动器仅供隔离桌面验收，复用实际应用服务、HTTP handler、数据库和 worker，在构造生成服务时注入确定性协议替身。不修改正式 `cmd/semlia`、正式配置或模型供应商客户端，不加入可由用户请求开启的替身开关。

启动器要求显式的验收环境标记和资源归属信息，拒绝缺失或不匹配的配置。资源由任务已允许的 `compose.production-acceptance.yaml` 和 `scripts/dev/production-acceptance.sh` 管理；执行前检查项目、端口及卷所有权，清理只针对本次自建资源。工作区与身份可由测试初始化；语义资产和发布记录只经正常生产 API 创建，不预填业务资产或假发布状态。

新增验证：`go test ./internal/testsupport/productionacceptance`。保留 T006 原有全部组件、构建、嵌入包和双桌面真实浏览器验收命令，不降低标准。

## 执行边界

用户于 2026-09-11 明确批准增加上述文件并继续完成 T006。执行由 [SP-T006-A001 包](../../specs/semantic-production/packets/SP-T006.yaml) 约束，前像为 `.semlia/evidence-work/SP-T006-A001-baseline.tar`。T006 完成后进入 Needs_Review，T007 不自动启动。
