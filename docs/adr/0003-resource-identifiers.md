# TDR-0003: Resource Identity And Human-Readable Addressing

## 元数据

| 字段 | 值 |
| --- | --- |
| 版本 | 1.0.0 |
| 状态 | Confirmed |
| 最后更新 | 2026-09-01 |
| 来源 | `docs/SSOT.md` v0.7.0 Confirmed、`docs/specs/semantic-assets/semantic-asset-design.md` |
| 适用范围 | PostgreSQL、OpenAPI、事件、Git 工件、日志、CLI、MCP 与 Web |
| 审核 | 2026-09-01 经创始人确认采用短而可辨识的资源地址，并限制自增 ID 的使用边界 |

## 1. 决策摘要

Semlia 将数据库身份、交换身份和人类语义地址视为同一资源身份的不同表达，而不是让一种字符串同时承担存储、分布式唯一性和可读性。

- PostgreSQL 对核心领域对象使用应用侧生成的 UUIDv7，并以原生 `uuid` 类型保存主键与外键。
- API、事件、Git 引用、日志、CLI 和 MCP 使用 TypeID 表达同一个 UUIDv7，例如 `ast_01k3...`。TypeID 只改变编码和增加资源类型前缀，不产生第二份身份。
- Web 默认展示语义地址，例如 `commerce.net_revenue@12`；技术资源 ID 只出现在详情、复制操作、审计和开发者界面。
- `bigint GENERATED ALWAYS AS IDENTITY` 只用于不离开单一数据库、不会进入公共契约的内部行号或局部序号，不能成为核心领域对象唯一的稳定身份。
- 标识不可承载授权判断。每次读取和写入仍按 workspace、actor、policy 与资源归属执行鉴权。

## 2. 身份模型

| 层次 | 规范形式 | 示例 | 用途 |
| --- | --- | --- | --- |
| 持久化身份 | PostgreSQL `uuid` / UUIDv7 | `019c...` | 主键、外键、唯一性和有序写入 |
| 交换身份 | TypeID | `ast_01k3...` | OpenAPI、事件、日志、Git、CLI、MCP |
| 语义地址 | `namespace.key` | `commerce.net_revenue` | 搜索、导航、Wiki 标题和人工沟通 |
| revision 地址 | `semantic_address@sequence` | `commerce.net_revenue@12` | 界面展示和局部版本引用 |

`SemanticAsset.id` 与 `AssetRevision.id` 始终是两个独立 UUID。`revision.sequence` 只在单个资产范围内递增，`asset_id + sequence` 必须唯一，但 sequence 本身不是资源身份。

语义地址满足以下约束：

- `UNIQUE (workspace_id, namespace, key)`，比较规则固定并由规范定义。
- key 变更必须走受治理 revision，并保留旧地址到稳定资产 ID 的 alias 解析记录。
- 关系、证据、release manifest 和 consumer binding 只引用不可变资源 ID，不以可变 key 作为外键。
- URL 可以显示语义地址，但写命令和不可变 manifest 必须携带 TypeID 或解析后的 UUID。

## 3. TypeID 规则

TypeID 后缀使用 UUIDv7 的 128-bit 值和规范化 Base32 编码。前缀保持短、稳定且只表达资源种类：

| 前缀 | 资源 |
| --- | --- |
| `wsp_` | Workspace |
| `ast_` | SemanticAsset |
| `rev_` | AssetRevision |
| `rel_` | SemanticRelation |
| `ont_` | OntologyRevision |
| `evd_` | EvidenceArtifact |
| `rls_` | Release |
| `run_` | Job、validation 或 agent run 的稳定运行身份 |

前缀不是数据库列，也不参与外键；边界适配器根据资源类型验证前缀并在 TypeID 与 UUID 之间无损转换。禁止截断 UUID、保存任意长度随机字符串或通过字符串前缀推断授权范围。

PostgreSQL 17 兼容目标不依赖数据库原生 UUIDv7 生成函数。Go application boundary 使用经过测试并锁定版本的 TypeID/UUIDv7 实现生成身份；数据库只负责 `uuid` 类型、主外键和唯一约束。PostgreSQL 18 的原生 UUIDv7 能力可以用于数据库维护操作，但不能成为兼容性前提。

## 4. 自增 ID 使用边界

允许使用 `bigint GENERATED ALWAYS AS IDENTITY`：

- 资产内部的 revision sequence、排序位置和 attempt sequence。
- 不进入 API、事件、URL、Git 工件、审计 subject 或跨环境导入导出的实现行。
- 可被删除和重建，且不会改变任何领域引用的投影索引行。

必须使用 UUIDv7/TypeID：

- Workspace、SemanticAsset、AssetRevision、SemanticRelation 和 OntologyRevision。
- EvidenceArtifact、Release、ConsumerBinding 和跨进程可见的 Run。
- 任何会被外部系统保存、跨环境迁移、离线生成或进入幂等契约的对象。

使用 identity column 时必须同时声明 `PRIMARY KEY` 或 `UNIQUE`。禁止使用 `serial` 作为新 schema 的规范写法。

## 5. API、事件与产品体验

- OpenAPI 对 TypeID 使用独立 format 或 pattern，并按资源前缀生成强类型 wrapper，禁止所有 ID 退化为可互换的普通字符串。
- EventEnvelope 的 `id`、`subject` 和领域引用使用 TypeID；W3C trace ID、idempotency key 和内容摘要保持各自协议格式，不转换为 TypeID。
- 错误消息、审计和日志优先输出 TypeID 与语义地址；数据库 UUID 仅用于诊断深挖。
- 列表、关系图和普通详情不展示随机标识。复制技术 ID 使用明确按钮和 tooltip，不让长字符串挤占主信息层级。
- 同名资产通过 workspace、namespace、type 和稳定 ID 消歧，不能依赖随机后缀让用户判断业务含义。

## 6. PostgreSQL 约束模板

```sql
CREATE TABLE semantic_assets (
    id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    namespace text NOT NULL,
    key text NOT NULL,
    asset_type text NOT NULL,
    UNIQUE (workspace_id, namespace, key)
);

CREATE TABLE asset_revisions (
    id uuid PRIMARY KEY,
    asset_id uuid NOT NULL REFERENCES semantic_assets (id),
    sequence integer NOT NULL CHECK (sequence > 0),
    UNIQUE (asset_id, sequence)
);
```

数据库列名使用 `id` 和 `<resource>_id`，不使用 `uuid` 作为列名后缀。UUIDv7 生成、TypeID 编解码和 prefix 校验位于共享 identity package，业务模块不得各自实现。

## 7. 迁移与兼容

- `000001_m0_foundation` 中的 `text` ID 是 M0 的格式无关底座，不代表长期公共契约。
- M1 通过独立的前向迁移和受测 backfill 将 foundation identity 收敛到 UUID；不得改写已经存在的迁移文件。
- backfill 生成映射后必须在同一受控迁移中更新所有外键、outbox 引用和审计 subject，并验证无孤立引用。
- 第一个稳定版发布前完成转换。稳定版之后，资源 ID 永不复用，编码格式变更必须提供双读迁移和兼容窗口。

## 8. 备选方案

### 仅使用自增 bigint

拒绝作为核心身份。它在单库内紧凑高效，但不适合离线生成、环境合并、Git 工件、跨系统引用和未来同步；直接暴露还会泄露创建顺序与近似规模。

### 直接向用户展示 UUID

拒绝。UUID 是可靠的存储身份，但标准十六进制表达冗长，且不携带资源类型或业务含义。

### ULID text 作为 PostgreSQL 主键

不采用。ULID 的 26 字符表达和时间排序可用，但文本主外键增加索引与约束成本；TypeID 在保留原生 UUID 存储的同时提供同类可读编码和类型前缀。

### 截断 UUID、短 NanoID 或 Hashids

拒绝作为核心身份。截断需要独立碰撞预算，Hashids 不是唯一性来源，短随机 ID 会把容量规划变成每个资源类型的隐性风险。

## 9. 结果与验证要求

- 新核心表和公共契约必须通过 UUIDv7 版本、TypeID round-trip、错误 prefix、跨 workspace 同 key、alias 解析和 revision 局部唯一性测试。
- repository 测试必须证明外键使用 UUID，日志和 API snapshot 不泄露裸数据库 UUID。
- 前端测试必须证明主工作流展示语义地址，本体关系和 release 仍可复制或定位技术 ID。
- 任何例外必须在新的 ADR 中说明对象生命周期、唯一性域、迁移和兼容性影响。

## 10. 参考资料

- [RFC 9562: UUIDs](https://www.rfc-editor.org/rfc/rfc9562.html)
- [TypeID specification](https://github.com/jetify-com/typeid)
- [PostgreSQL 17 UUID type](https://www.postgresql.org/docs/17/datatype-uuid.html)
- [PostgreSQL 17 identity columns](https://www.postgresql.org/docs/17/ddl-identity-columns.html)
