# 本地开发

Semlia 的正常开发环境由本机 Go server、Go worker、Vite 和 OrbStack 中的 PostgreSQL 18 组成。日常启动不构建应用 Docker 镜像。前端通过真实会话进入工作区，不接受假身份请求头或 Fixture 环境开关。

## 环境与端口

- `.semlia/dev.env` 保存现有 PostgreSQL 凭据、应用密钥和默认端口，权限为 `0600`。保留既有文件，不通过删除文件重置数据库凭据。
- `.semlia/native.env` 可配置 `SEMLIA_DATABASE_URL`、`SEMLIA_NATIVE_WEB_PORT`、`SEMLIA_NATIVE_API_PORT` 和模型凭据环境变量，同样使用 `0600`，不提交 Git。
- Web 端口优先使用 `SEMLIA_NATIVE_WEB_PORT`，其次使用 `dev.env` 的 `SEMLIA_HTTP_PORT`，最后默认为 `18081`。API 默认监听 `127.0.0.1:18080`，PostgreSQL 默认连接 `127.0.0.1:5433`。
- 本机内容与产物目录为 `.semlia/native/content` 和 `.semlia/native/artifacts`。已有 Docker 卷必须经过备份、恢复验证和受控复制，不能用空目录替代。

## 启动与停止

```bash
make bootstrap
make dev
make smoke
make dev-down
```

`make dev` 编译 Go 二进制，检查配置及数据库结构，然后启动 server、worker 和 Vite。它不自动迁移数据库、不删除容器，也不接管其他进程占用的端口。任一子进程退出时，监督进程停止同组其他进程。

`make smoke` 检查本机监督进程、Web 代理和 API 就绪状态。`make dev-down` 只停止自有本机进程，不停止 PostgreSQL、不删除数据。日志位于 `.semlia/native/server.log`、`worker.log`、`web.log` 和 `supervisor.log`。

## 数据库迁移

应用要求迁移版本 `29` 且非 dirty。初始化或升级使用显式命令：

```bash
go build -o build/semlia ./cmd/semlia
node scripts/dev/native.mjs migrate version
node scripts/dev/native.mjs migrate up
```

已有数据库必须先保存数据库、应用配置、内容和产物快照，在独立 PostgreSQL 完成恢复与迁移验证。最终切换前停止写入，再获取一致的最终快照。验证失败时保留原服务与数据库，不强制修改迁移版本。密码凭据表非空时拒绝降级。

## 管理员与密码

初始管理员由本机终端建立，不开放匿名注册：

```bash
bash scripts/dev/account.sh bootstrap-local-admin semantic-core "Semantic Core" founder@example.com
```

脚本隐藏输入并确认密码，通过 stdin 传给管理命令。不要将密码放入命令参数、环境变量、聊天或脚本文件。账号支持字母、数字、点、下划线、短横线组成的 1 至 64 字符登录名（如 `admin`），也支持邮箱格式。账号不区分大小写；邮箱格式不代表邮箱已验证，不与同邮箱 OIDC 身份自动合并。

密码至少 6 个字符，最多 1024 字节，禁止控制字符和常见弱密码。数据库只存 Argon2id 摘要。重复初始化同一管理员不重置密码；已有有效管理员的工作区拒绝初始化另一名管理员。

遗失密码时运行：

```bash
bash scripts/dev/account.sh reset-local-password founder@example.com
```

用户可在账户菜单修改本人密码；具备成员管理和相应角色授予权限的管理员可在成员页建号、停用成员。改密和管理恢复原子撤销账号的全部会话。成员停用与撤销沿用工作区授权边界，保留最后一名有效管理员。

## 可选 OIDC

发布环境可设置 `SEMLIA_AUTH_MODE=oidc` 及 issuer、client ID、client secret、redirect URL。OIDC 与本地账号共用同一前端、会话和授权体系。OIDC 身份按 issuer/subject 独立关联；已验证邮箱仅用于匹配受控邀请。

应用使用 HttpOnly、SameSite cookie；写请求需要明确的 Origin 和会话 CSRF 校验。密码登录要求允许的 Origin 及 JSON 请求体。非回环地址必须使用 HTTPS。生产环境还要求安全的数据库连接和产物配置。

## 数据来源凭据

数据来源使用专用只读 PostgreSQL 账号，不复用应用数据库所有者。来源密码仅通过创建或轮换接口进入系统，服务端不返回密码、连接 URI 或密文。发现运行固定凭据版本，读取元数据前验证只读权限。

SQL 证据路径相对于 `SEMLIA_SOURCE_ARTIFACT_ROOT`，禁止路径穿越与符号链接。server 和 worker 使用同一受保护内容副本。

## 合成演示数据

五类知识、订单与客户样本、依赖关系和客单价演算使用独立的合成演示工作区。初始化与界面入口见 [五类知识演示数据](knowledge-demo.md)。该本地种子入口只操作本仓库拥有的开发数据库，不接管真实来源或现有工作区。

## 验收与发布

```bash
make check-source
make browser-bootstrap
make check-browser
make check-smoke
make security-check
make check
```

源码门禁保留格式、静态检查、类型检查、单元/集成测试、契约及生成物一致性和发布构建。`make check-smoke` 使用独立 Compose 项目、端口和镜像验证完整容器发布链，包括 PostgreSQL 故障恢复与持久化；结束后清理自有资源。`make security-check` 构建并扫描发布镜像，不属于日常开发启动。

`make check-browser`（等同 `pnpm --dir web test:e2e`）使用独立测试数据库和随机合成账号验证本机进程、两种桌面尺寸及密码/成员流程，再验证分离角色的语义生产流程。所有角色通过正常密码登录建立会话；模型使用隔离协议桩，不调用真实模型。结束后停止自有服务与测试数据库。首次运行先执行 `make browser-bootstrap` 安装 Chromium 及系统依赖。

发布工作流使用同一提交的源码、浏览器、容器 smoke 和安全门禁；全部通过后才构建、上传和证明发布产物。本地未提交工作区生成的产物仅为待审核候选，不能代替正式提交及其 CI 证据。

发布打包要求实际编译器版本与 `go.mod` 的 `toolchain` 完全一致，不接受宿主较旧编译器。需要从 Go 自动选择的工具链定位可执行文件时，使用 `make release GO="$(go env GOROOT)/bin/go"`。SBOM 保留原始扫描文件及漏洞信息，合并后执行 CycloneDX schema 验证。

容器发布入口保留在 Compose 与 `deploy/local/Dockerfile`。清理必须精确到项目容器和无引用应用镜像；禁止全局 Docker prune，禁止删除 PostgreSQL 数据卷、备份或其他项目资源。
