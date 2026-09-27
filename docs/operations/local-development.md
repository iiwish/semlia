# 本地开发

Semlia 的正常开发环境由本机 Go server、Go worker、Vite 和 OrbStack 中的 PostgreSQL 18 组成。日常启动不构建应用 Docker 镜像。前端通过真实会话进入工作区，不接受假身份请求头或 Fixture 环境开关。

当前交付是[收敛版 1.0 本地候选](../specs/v1-convergence/spec.md)，最终候选门禁与用户验收待完成。全新环境必须先按 [Quickstart](../quickstart.md)准备已有 PostgreSQL 18 中的专用数据库、角色和私有配置，显式迁移及创建账号；`make dev` 不创建数据库。

## 环境与端口

- `.semlia/dev.env` 保存现有 PostgreSQL 凭据、应用密钥和默认端口，权限为 `0600`。保留既有文件，不通过删除文件重置数据库凭据。
- `.semlia/native.env` 可配置 `SEMLIA_DATABASE_URL`、`SEMLIA_NATIVE_WEB_PORT`、`SEMLIA_NATIVE_API_PORT` 和模型凭据环境变量，同样使用 `0600`，不提交 Git。
- 默认入口合并读取 `dev.env`、`native.env` 并保留调用者环境。显式 `SEMLIA_NATIVE_ENV_FILE` 是必需的独立完整配置文件，必须同时传入 `SEMLIA_NATIVE_STATE_DIR`，后者位于当前仓库 `.semlia/` 内的专用目录。缺文件或缺状态目录会拒绝，不会补读普通配置或继承父环境中的运行凭据、执行来源与 LAN 配置；子进程仅保留必要工具环境及该配置。文件内禁止定义保留控制项 `SEMLIA_NATIVE_ENV_FILE`、`SEMLIA_NATIVE_STATE_DIR`，不能重定向自己的配置或状态路径。
- Web 端口优先使用 `SEMLIA_NATIVE_WEB_PORT`，其次使用 `dev.env` 的 `SEMLIA_HTTP_PORT`，最后默认为 `18081`。API 默认监听 `127.0.0.1:18080`，PostgreSQL 默认连接 `127.0.0.1:5433`。
- 本机内容及 SQL 来源输入共用 `.semlia/native/content`，上传产物位于 `.semlia/native/artifacts`；这是 native wrapper 固定路径。其他部署可单独配置 content、inputs、artifacts。已有 Docker 卷必须经过备份、恢复验证和受控复制，不能用空目录替代。

## 启动与停止

```bash
make bootstrap
make dev
make smoke
make dev-down
```

`make dev` 编译 Go 二进制，检查配置及数据库结构，然后启动 server、worker 和 Vite。它不自动迁移数据库、不删除容器，也不接管其他进程占用的端口。任一子进程退出时，监督进程停止同组其他进程。

`make smoke` 检查本机监督进程、Web 代理和 API 就绪状态。`make dev-down` 只停止自有本机进程，不停止 PostgreSQL、不删除数据。日志位于 `.semlia/native/server.log`、`worker.log`、`web.log` 和 `supervisor.log`。

使用显式独立配置时，启动、检查、迁移、账号管理和停止命令必须始终携带相同的配置文件与状态目录控制项。配置需提供其自身完整的数据库、密钥和端口设置；不要在文件内部填写这两个保留控制项。正常默认开发流程无需设置它们。

## 数据库迁移

应用要求迁移版本 `33` 且非 dirty。初始化或升级使用显式命令：

```bash
go build -o build/semlia ./cmd/semlia
node scripts/dev/native.mjs migrate version
node scripts/dev/native.mjs migrate up
```

已有数据库必须按[备份与恢复](backup-recovery.md)保存数据库、完整文件库、密钥和外部凭据，在已有 PostgreSQL 的新专用目标库完成恢复与迁移验证。当前升级证据仅覆盖已初始化身份与工作区的 schema 32→33，不支持外推到任意旧七类原型数据。最终切换前停止写入并获取一致快照；失败时保留原服务与数据库，不强改迁移版本，不对含 Ask 数据的 schema 33 验收库执行降级。

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

应用使用 HttpOnly、SameSite cookie；写请求需要明确的 Origin 和会话 CSRF 校验。密码登录要求允许的 Origin 及 JSON 请求体。非回环地址默认必须使用 HTTPS。生产环境还要求安全的数据库连接和产物配置。

可信局域网临时演示可在 `.semlia/native.env` 中设置 `SEMLIA_DEVELOPMENT_LAN_HTTP=true` 和 `SEMLIA_NATIVE_LAN_IP=<本机局域网 IPv4>`，重启 `make dev` 后通过 `http://<该 IP>:<Web 端口>` 访问。Web 监听所有接口，但服务端仅接受明确列出的 Origin，API 仍绑定回环地址。此开关仅允许 development 环境的私有 IP，不允许公共 IP、域名或通配 Origin，不改变登录、授权和 CSRF 要求。HTTP 会明文传输密码和会话，仅供可信网络临时使用；恢复时删除这两个配置并重启。局域网 IP 变化后需同步配置。

## 数据来源凭据

数据来源使用专用只读 PostgreSQL 账号，不复用应用数据库所有者。来源密码仅通过创建或轮换接口进入系统，服务端不返回密码、连接 URI 或密文。发现运行固定凭据版本，读取元数据前验证只读权限。

只读执行单独配置 `SEMLIA_EXECUTION_SOURCES`，每项固定 `workspaceId`、`sourceId` 和 `dsnEnv`；命名环境变量提供执行专用连接。凭据必须置于受保护配置，不能写入知识、Prompt 或公开回执。TLS 为默认要求，回环开发传输例外 `SEMLIA_EXECUTION_ALLOW_PLAINTEXT=true` 不免除只读角色校验；不得为验收开启 `SEMLIA_DISCOVERY_ALLOW_UNSAFE_SOURCE`。具体执行规则见当前 API 和执行适配器契约。

执行专用角色至少应设置 `default_transaction_read_only=on` 和 `log_min_error_statement=panic`，并只授予所需来源 schema/table 的读权限；缺少这些约束会被执行适配器拒绝。前者限制写入，后者防止错误语句携带敏感查询内容进入 PostgreSQL 日志。明文连接开关仅供受控的本地回环测试，生产必须使用符合策略的 TLS，不能照搬开发例外。

SQL 证据路径相对于 `SEMLIA_SOURCE_ARTIFACT_ROOT`，禁止路径穿越与符号链接。server 和 worker 使用同一受保护内容副本。

## 合成演示数据

收敛版 1.0 的订单、客户、五类知识与模型验收使用独立专属环境，见 [T001](../evidence/V1-T001/summary.md) 至 [T004](../evidence/V1-T004/summary.md)。验收脚本需要匹配的私有 owner/marker 回执，不是通用安装器。不要直接对未知数据库运行老演示种子或将演示工作区当作生产资料。

## 桌面与恢复边界

仅支持宽度至少 1024px 的桌面 Web，验收尺寸为 1440x900、1024x768；保留键盘焦点、reduced-motion 和合成数据披露。自然修订先保存 production draft，再由作者显式确认业务规则、提交、独立审核发布。待完成修订的请求 key 只在当前页面运行实例的内存中保存，刷新或重启后的未知结果须先核对已有 operation，不能据此承诺跨刷新自动恢复且绝不重复创建。

从历史修订寻找可信生产来源最多回溯 64 步；找不到时明确拒绝，不虚构基线或绕过治理。SQL artifact 可以注册与发现，但其 adapter 不提供内容预览，preview HTTP 422 是明确边界；Markdown 等支持的内容类型用于预览。Ask 结果行是临时数据，不能通过完整备份承诺恢复已显示的结果表。

## 验收与发布

```bash
make check-source
make browser-bootstrap
make check-browser
make check-smoke
make security-check SEMLIA_SECURITY_IMAGE="semlia:security-<fresh-owned-id>"
```

源码门禁保留格式、静态检查、类型检查、单元/集成测试、契约及生成物一致性和发布构建。`make check-smoke` 使用独立 Compose 项目、端口和镜像验证完整容器发布链，包括 PostgreSQL 故障恢复与持久化；结束后清理自有资源。`make security-check` 构建并扫描发布镜像，不属于日常开发启动。

将示例中的占位标签替换为实际新建的自有标签后运行。也可用 `make check SEMLIA_SECURITY_IMAGE="semlia:security-<fresh-owned-id>"` 一次运行全部门禁，不必重复上面的逐项命令；完整入口同样必须显式传入安全镜像变量，前一条 `make` 的命令行变量不会传给下一次调用。

`make check-browser` 运行 native 与 production 浏览器验收入口，使用隔离数据库和合成账号；普通 Web 测试命令不代表整个门禁。角色通过正常密码登录，模型使用隔离协议桩。真实模型证据独立记录，不能用协议桩通过率代替。首次运行先执行 `make browser-bootstrap` 安装 Chromium。该入口的独立配置、资源归属与清理及一次完整正常浏览器运行已有 [T004-A003 证据](../evidence/V1-T004/browser-gate-isolation.md)；最终冻结源码的 T005 候选门禁仍待完成，不能把该次源码环境结果归因于尚未构建的 RC。

发布工作流使用同一提交的源码、浏览器、容器 smoke 和安全门禁；全部通过后才构建、上传和证明发布产物。本地未提交工作区生成的产物仅为待审核候选，不能代替正式提交及其 CI 证据。

发布打包要求实际编译器版本与 `go.mod` 的 `toolchain` 完全一致，不接受宿主较旧编译器。需要从 Go 自动选择的工具链定位可执行文件时，使用 `make release GO="$(go env GOROOT)/bin/go"`。SBOM 保留原始扫描文件及漏洞信息，合并后执行 CycloneDX schema 验证。

容器发布入口保留在 Compose 与 `deploy/local/Dockerfile`。清理必须精确到项目容器和无引用应用镜像；禁止全局 Docker prune，禁止删除 PostgreSQL 数据卷、备份或其他项目资源。

安全门禁应使用新建、唯一的 `SEMLIA_SECURITY_IMAGE` 标签，避免覆盖已有 `semlia:security`。开发工具输出发生过私有配置披露，生产前必须安排受影响凭据轮换；加密根密钥轮换需连同密文重加密和恢复验证实施，不能直接更换后遗失来源访问能力。
