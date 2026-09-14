# 安全补丁复核

用户明确授权 grpc 1.83.2、js-yaml 4.3.2 补丁升级及完整门禁复验。增量基线为本轮 security-before 前像，不以脏工作区的 HEAD 差异代替。

## 变更范围

- `go.mod` 和 `go.sum`：仅 grpc 版本及模块、模块声明文件的校验值。
- `pnpm-workspace.yaml` 和 `pnpm-lock.yaml`：仅 js-yaml override、解析版本、引用和 integrity。
- `package.json`、产品业务代码、Dockerfile、安全检查脚本及忽略规则未修改。

精确差异见 `security-dependency-delta.patch`。只读独立复核未发现 P1/P2 或范围外依赖升级。

## 验证

宿主与专用机器的 `go mod verify` 均退出 0。两端 `pnpm install --frozen-lockfile` 均退出 0；锁文件通过 334 项供应链策略检查。锁文件由 pnpm 11.1.3 正常生成，未手工编造 integrity。

首次 `go mod tidy` 因读取与本补丁无关的依赖测试模块而主动终止，退出 143，不计作验证通过；使用精确 `go mod download google.golang.org/grpc@v1.83.2` 获取经校验模块，并复核四文件增量。

第六次完整门禁的失败报告保留在 `security-before-filesystem.txt` 和 `security-before-container.txt`。第七次完整门禁退出 0，原报告为 `security-final-filesystem.txt` 和 `security-final-container.txt`：Go/pnpm 依赖、Debian 12.15 及 Go 发布二进制的 HIGH/CRITICAL 均为 0。完整运行见 `make-check-07.log`，不是仅运行安装或定向扫描。

没有增加扫描忽略项，没有禁用 secret 或开发依赖扫描，HIGH/CRITICAL 门槛和固定 Trivy 镜像摘要不变。新模块仍由校验数据库和 `go.sum` 保护。
