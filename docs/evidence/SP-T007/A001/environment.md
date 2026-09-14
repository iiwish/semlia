# 专用验收环境

用户已授权创建临时隔离环境及完成后的归属清理。机器 `semlia-t007-20260911`，ID `01M27SHHEPNQ4GTQ5FAA1RXBPB`，Ubuntu 24.04 arm64，4 CPU、8 GiB 内存、40 GiB 磁盘限额，创建参数包含 `--isolated --isolate-network`。无主机文件共享、Docker socket 挂载或客户数据库复制。

专用 Docker ID：`18839ed8-aec8-4f47-87e5-a427960b8e96`。默认 OrbStack Docker ID：`d743259d-6bd2-47c5-a84f-bcbfe8d353ad`。完整门禁仅前者执行，允许其内部使用 `semlia-local`、`semlia:local`、`semlia:security` 和内部 8080/5433，不触碰宿主同名资源。

依赖安装只发生于专用机器：Ubuntu 注册软件包中的 Docker/Compose、编译工具及 Git；Go 1.26.6、Node 24.15.0 与 pnpm 11.1.3。官网下载文件 SHA-256 与官方发布清单核对：Go `d0507e9e9d7fe012aae570108cbd76c15de879e17130ab8cb90d4d7445cb1f2e`，Node `f3d5a797b5d210ce8e2cb265544c8e482eaedcb8aa409a8b46da7e8595d0dda0`。

隔离网络阻断宿主代理地址，依赖下载进程显式取消继承的 proxy 环境变量，直接访问软件源；没有解除网络隔离。完整工作区快照排除运行状态、凭据、客户数据、依赖缓存及生成构建目录，包含全部当前已跟踪及未跟踪源码与嵌入资源。执行前核对快照 manifest 与 Docker ID，清理仅该机器和快照归属资源。

## 依赖与快照

专用机器直接拉取 Docker Hub 镜像发生 TLS 超时。PostgreSQL 18/17、Ryuk、Node 24.15.0、Go 1.26.6、pgvector/pgvector:pg17、distroless 和固定 Trivy 公共镜像经宿主 Docker CLI 的 `image save` 流式传入专用 Docker `load`；不挂载宿主 socket，不导入项目镜像或现有卷。宿主仅新增公共依赖缓存，不重标或删除既有镜像。

Trivy 导入后保留精确 OCI digest `7cced7cae583819fc7806d4cbc0dbbc7cad18b99f7d3e235192e6da8c091045c`，但 Docker archive 未保留 repository 引用。只在专用 Docker 的 containerd 存储内将该相同 digest 恢复为原始 `docker.io/aquasec/trivy@sha256:...` 引用；未替换字节、改扫描脚本或绕过摘要固定。随后 Docker 按扫描脚本的精确引用 inspect 成功。Trivy 漏洞数据库由专用环境从工具默认 mirror.gcr.io 源正常下载成功，没有禁用扫描或修改漏洞数据库。

Go 依赖只导入当前 `go.mod` 解析出的 182 个公共模块、728 个下载缓存文件；宿主及专用机器分别执行 `go mod verify` 通过。没有复制整个宿主 Go 缓存。专用机器本地从 lockfile 安装 pnpm 依赖，并安装 Linux arm64 Chromium 与系统字体。

`snapshot-manifest.json` 记录初始 1597 个文件及 SHA-256，`snapshot-manifest-final.json` 记录验收测试更新后的源码。源文件更新前核对旧 SHA-256，再核对新 SHA-256。传输生成的 macOS `._` 元数据导致第一次 format 检查失败，已只在专用机器移除这些非源码文件；第二次真实 `make check` 在五个既有 Go 文件的 gofmt 检查失败，未跳过门禁。

最终安全补丁只同步 `go.mod`、`go.sum`、`pnpm-lock.yaml` 和 `pnpm-workspace.yaml`，传输前后均核对 SHA-256。grpc 1.83.2 由专用机器正常下载并通过 `go mod verify`；pnpm 冻结锁安装通过 334 项供应链策略。`snapshot-manifest-final.json` 是最终第七次完整门禁的源码快照。

## 构建环境

第四次完整门禁的 source gates 全部通过，随后 BuildKit 的 overlay 挂载被隔离层拒绝。专用机器 Docker 无运行或遗留容器时，验证 Docker ID 后把其存储配置改为 containerd `native`，并正常重启该机器内 Docker；ID 不变。`native` 是 containerd 的文件复制型 snapshotter，不需要本轮失败的 overlay 联合挂载，见 [containerd 官方说明](https://github.com/containerd/containerd/blob/main/docs/snapshotters/README.md)。没有移除机器的 `--isolated`、`--isolate-network` 限制，没有挂载宿主 socket 或提高权限。

镜像内容保持原摘要，重新导入公共镜像以在 native snapshotter 解包。专用机器安装 Ubuntu 注册仓库中的 docker-buildx 0.30.1；公共 Go 模块下载缓存通过单独缓存种子构建导入 BuildKit 的 `/go/pkg/mod` cache mount，日志为 `build-cache-seed-02.log`。缓存只含此前核验的公共模块，不替代 `go.sum` 校验，不修改产品 Dockerfile 或门禁脚本。

实际 cgroup 配额核对：`cpu.max=400000 100000`，`memory.max=8589934592`。内核与 Docker 日志可显示底层共享机器的 16 GiB 内存，验收机器的限制仍为 4 CPU、8 GiB。

第五次门禁在编译完成后的相同标签并发导出失败。第六次完整门禁使用 `COMPOSE_BAKE=false COMPOSE_PARALLEL_LIMIT=1` 串行执行相同 Compose 构建，仍使用 BuildKit 与原产品 Dockerfile；这是专用环境的构建调度设置，不是跳过 smoke/security。并发限制按 [Docker Compose 官方 CLI 文档](https://docs.docker.com/reference/cli/docker/compose/)配置；不修改仓库 Compose 文件或主机 Docker 设置。默认并发模式在本专用 native 环境的首次导出失败保留为兼容性限制。

## 最终结果与清理

第七次完整 `make check` 使用相同隔离与串行构建设置，退出 0。清理前确认专用 Docker ID、机器 ID、网络/文件隔离配置，22 个本任务变更文件的宿主、专用快照及 manifest SHA-256 一致。所有测试容器已退出并清理，专用机器内遗留三个 smoke 持久卷随整机删除，不触碰默认 Docker 同名卷。

按机器 ID 调用 OrbStack 删除时 CLI 发生 panic、退出 2，随后读取 inventory 确认机器仍在、ID 与名称匹配。按已核对名称 `semlia-t007-20260911` 删除退出 0；删除后 inventory 中该名称与 ID 均不存在。

默认 Docker ID 保持不变，server `c730f3eb928e`、worker `b271bdc288e7`、PostgreSQL `cd0dac4bcf38` 三个原容器仍持续运行，server 与 PostgreSQL 健康。证据为 `cleanup-before.json` 和 `cleanup-after.json`。本轮未执行宿主 Docker prune、共享镜像删除、默认栈重启、提交、推送或部署。宿主公共依赖缓存与无凭据前像保留。
