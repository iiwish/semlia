# 历史数据库测试修复

用户批准四个测试文件扩围。前像为 `.semlia/evidence-work/SP-T007-db-before.tar`；新鲜 RED 在当前 schema 28 下重现五项失败，与既有 T005 基线一致。

修复限定测试：migration 18 和 20 从空库安装到精确的 17 和 19，不以相对最新版本的固定负步数定位；migration 21 专用数据库安装到 21，20 至 21 的重升只执行一步；schema 6 的 populated migration fixture 按版本 6 的列与约束构造提案和变更，不调用要求最新列的 repository。原有降级拒绝、历史记录保留和 dirty marker 断言保留，并补充 proposed 状态及前后值在每个历史阶段保持不变的检查。

五项定向测试及两个 migration 21 子场景通过，55.436s；完整数据库 package 新鲜通过，338.608s。未修改迁移 SQL、正式 repository、触发器或历史保护策略。该修复不意味着旧版应用可访问最新版 schema，也不通过跳过测试掩盖兼容性差异。

## Linux 时间精度

专用 Linux 全量运行另行重现 `TestMachineCredentialLifecycleAndChannelParity` 的两个轮换子场景失败：测试把纳秒级 `time.Now()` 生成的到期时间与 PostgreSQL 回读的微秒值直接比较。业务 Rotate 从持久化凭据读取到期时间；该断言在两种精度不同时即使没有延长有效期也失败。macOS 全库通过不能覆盖这个精度差异。

用户明确授权后，只将此测试的时钟设为 `time.Now().UTC().Truncate(time.Microsecond)`，不放宽相等比较，不修改凭据业务代码。前像为 `.semlia/evidence-work/SP-T007-clock-before.tar`。专用 Linux 定向重跑通过，包含 rotate-rotate、rotate-revoke、原令牌失效与四通道内容一致性，原失败保留在 `isolated-all-tests.log`，修正结果在 `isolated-clock-fix.log`。
