# 验证结果

状态：完整门禁通过，待用户验收。命令在仓库根目录运行；专用 Linux 命令在完整工作区快照运行，Docker ID 见 `environment.md`。真实模型只有单独 opt-in 命令会调用，普通测试中的 skip 不当作模型通过。

| 命令或检查 | 实际结果 |
| --- | --- |
| 六包新鲜集成回归，宿主临时 PostgreSQL | ingestion 10.974s、discovery 7.308s、governance 80.199s、projection 4.860s、catalog 5.585s 通过；db 首轮 235.476s 失败，原五项旧测试修正后 db 338.608s 通过。 |
| 10,000 资产基准，宿主 | 11.455s 通过；分页 p50/p95 18.214/18.968ms，搜索 p50/p95 150.844/234.586ms；p95 门槛 150/250ms。 |
| 实际模型 opt-in | 4 次实际调用成功，均有持久化调用槽和结果；未使用剩余 26 次。最终 run 总计 65.390s；原始与纠正评分见 `model-quality.md`。两次调用前配置失败未保留调用槽、未执行供应商调用。 |
| commerce 冷启动、人工纠正、变化生产和回滚 | 宿主新鲜 15.392s；加强发布读回断言后专用 Linux 新鲜 17.388s 通过，见 `isolated-commerce.log`。 |
| `make lint typecheck`，专用 Linux | 通过；三项既有前端警告，无错误，见 `isolated-lint-typecheck.log`。 |
| `make contracts-check db-generate-check web-embed-check`，专用 Linux | exit 0；契约、SQLC 与嵌入前端漂移检查通过，见 `isolated-drift.log`。 |
| 首次 `make test`，专用 Linux | exit 2，见 `isolated-all-tests.log`。Go 测试完成，因失败未执行后续 pnpm 测试；不能声明全量通过。 |
| Linux 旧测试精度修正 | `TestMachineCredentialLifecycleAndChannelParity` 新鲜 32.595s 通过，见 `isolated-clock-fix.log`。 |
| Linux 运行投影及文档路径修正 | operations 新鲜 44.102s、repository 1.076s 通过，见 `isolated-baseline-fixes.log`。 |
| `make format-check`，宿主 | 五个授权文件 gofmt 后 exit 0；未修改业务行为。 |
| `git diff --check`、验收脚本 `bash -n` | exit 0。 |
| `make check` 首两次，专用 Linux | 都失败，见 `make-check-01.log`、`make-check-02.log`。首次为传输产生的 macOS 元数据，第二次为五个文件的 gofmt；没有跳过格式门禁。最终完整复验见第七次结果。 |
| `production-acceptance.sh --suite full`，专用 Linux | exit 0；浏览器 4/4、六包新鲜回归及 10,000 基准通过。原始日志在 `isolated-full-suite.log` 与 `desktop/`。 |
| 模型直接入口预留保护 RED/GREEN | 原逻辑五个未知/并发反例失败；修复后 25.992s 通过，不调用供应商。最终完整门禁包含增加的 30 次上限断言。 |
| `make check` 第三次，专用 Linux | exit 2：嵌套前端测试 229/230，通过目录按钮的默认等待窗口发生一次失败；日志为 `make-check-03.log`。db、embedding、operations 与 repository 修正均通过。 |
| 原命令前端失败复验 | 不修改代码、超时或断言，`TestAcceptancePnpmIgnoresHostScriptShell` 新鲜 27.065s 通过；见 `isolated-pnpm-recheck.log`。保留偶发等待失败的风险，不隐藏首次失败。 |
| `make check` 第四次，专用 Linux | source gates 全部通过，含前端 230/230；镜像构建因隔离层拒绝 overlay 挂载失败，exit 2，见 `make-check-04.log`。 |
| `make check` 第五次，native 存储 | source gates 全部通过；产品 Dockerfile 的编译成功，但 Compose 并发导出相同 `semlia:local` 首次标签发生 already exists 冲突，exit 2，见 `make-check-05.log`。 |
| `make check` 第六次，native 存储、Compose 串行 | exit 2；源码、全部测试、镜像构建和真实栈 smoke 通过；依赖扫描发现 grpc 1.83.1 与 js-yaml 4.3.1 两项 HIGH，发布镜像 HIGH/CRITICAL 为 0。原报告为 `security-before-filesystem.txt`、`security-before-container.txt`。 |
| 安全补丁及依赖完整性 | 用户授权的四文件最小补丁独立复核通过；两端 `go mod verify` 和冻结锁安装 exit 0，334 项供应链策略通过，见 `security-review.md`。 |
| `make check` 第七次，最终安全补丁快照 | exit 0，见 `make-check-07.log`。format 0s、lint 9s、typecheck 7s、tests 182s、contract 1s、SQLC 3s、embed 6s、release build 7s；前端 26 文件 230/230。真实栈 smoke 21.335s 通过；依赖和发布镜像 HIGH/CRITICAL 均为 0，secret 扫描保持开启。 |
| 最终文档 repository 与差异检查 | 宿主 `go test ./tests/repository` 新鲜 7.373s，清理记录更新后新鲜 4.229s；`git diff --check` exit 0。 |
| 专用机器清理 | ID 形式删除 CLI panic 后核对原机器，名称形式删除 exit 0；机器已不存在，默认 Docker 和三个原开发容器均保持不变，见 `cleanup-after.json`。 |

用户授权 grpc 1.83.2、js-yaml 4.3.2 最小补丁升级及完整复验。升级前四个依赖文件保存在本轮 security-before 前像中；不增加漏洞忽略规则，不降低 HIGH/CRITICAL 标准。

## 首次 Linux 全量失败归因

- `db`：到期时间纳秒/微秒比较，测试时钟修正已定向通过，精确相等与并发断言保留。
- `embedding`：首次 `pgvector/pgvector:pg17` 拉取 TLS 超时，尚未进入业务断言。公共镜像导入专用环境后，后续完整门禁通过。
- `operations`：测试期待手填摘要，规范化发现摘要已持久化；改为核对实际 source revision，原有事务与终态断言保留，定向通过。
- `repository`：两份历史验收文档含本机绝对路径。用户授权仅替换路径后通过，不变更原历史结果。

各失败日志保留。最终第七次完整门禁退出 0，不以定向修正代替完整运行。

full suite 的六包新鲜耗时：ingestion 35.023s、discovery 20.256s、governance 207.343s、projection 6.300s、catalog 9.518s、db 121.389s。基准 8.808s，分页 p50/p95 14.158/15.025ms，搜索 p50/p95 136.402/167.496ms。本轮基准运行时没有其他本任务重型测试或构建并发。

完整 `make check` 使用仓库原命令，因此 Go 会复用允许的测试缓存；日志中 `(cached)` 不声称为新鲜重跑。指定 `-count=1` 的独立回归和 full suite 才计作新鲜行为证据。

full suite 的快照为 `full-suite-snapshot-manifest.json`；最终 `make check` 使用 `snapshot-manifest-final.json`，包含后续模型预留保护及两项安全补丁。补丁后没有追加供应商调用，也不把补丁前浏览器与基准成绩说成补丁后新鲜运行。最终 22 个变更源码及授权历史证据文件的宿主、专用机器和 manifest 哈希一致。
