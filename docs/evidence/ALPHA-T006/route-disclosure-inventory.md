# ALPHA-T006 Route Disclosure Inventory

Validation date: 2026-09-04

| Navigation surface | Controlled-User Alpha behavior | Visible boundary |
| --- | --- | --- |
| Semantic Ask | Real configured-provider call, schema gate and immutable-release resolution. Questions remain in the current page only. No warehouse execution adapter is configured. | Top bar says `仅本次会话`; the composer says `仅使用已发布知识 · 不执行数据查询`; resolved plans say `已完成语义解析，未执行数据查询`; provider failures say `未回退`. Fixture mode disables input and shows `Prototype 预览`. |
| Workbench | The current queue and aggregation are static product-shape data; they are not authoritative workspace tasks. | Real runtime shows `Prototype 工作台`; fixture runtime shows the common `Prototype 数据环境` notice. |
| Knowledge Assets | Real runtime reads and revises persisted catalog assets through governed proposals. The multi-tab detail still synthesizes release, consumer, trust and implementation projections when their owning APIs are absent. | Fixture runtime shows `Prototype 数据环境`. The Alpha evidence covers catalog assets, revisions and governed proposals only; it is not evidence that every detail tab is authoritative. |
| Changes and Releases | Real runtime uses persisted proposals, reviews, immutable releases and rollback records. | Fixture runtime shows `Prototype 数据环境`; no fixture state is presented as server state. |
| Data Sources | PostgreSQL connections, credential-envelope handling, connection tests, discovery runs and candidate proposals are real. File import and scheduling remain outside Alpha. | File tabs carry a `Prototype` chip; automation has a dedicated `Prototype` boundary; the context panel labels file and automation scope. Fixture runtime also shows `Prototype 数据环境`. |
| Members | Authenticated runtime reads and administers persisted workspace members. | Fixture member data is labeled as prototype authorization data and is covered by `Prototype 数据环境`. |
| Access Control | Protected commands use the real server-side evaluator, but the visible role editor, assignments and effective-access inspector are initialized from fixture data and mutate React state only. | The page labels changes as session-only. This Alpha inventory does not treat the Access Control administration surface as persisted or complete. |
| Model Configuration | Provider and model settings are persisted and used by Ask. Embedding rebuild orchestration is not yet durable. | Real runtime shows `混合能力边界`, identifying persisted configuration and page-only rebuild progress. Fixture runtime shows `Prototype 数据环境`. |
| Interfaces and Integrations | Client credentials, MCP, CLI and SDK administration remain product-shape UI only. | Real runtime shows `Prototype 管理页面`; fixture runtime shows `Prototype 数据环境`. No displayed credential is a real issued credential. |
| Audit and Runs | The dedicated aggregate UI is not yet backed by the Alpha audit API even though backend actions emit audit records. | Real runtime shows `Prototype 管理页面`; fixture runtime shows `Prototype 数据环境`. It must not be used as release evidence. |

The supported product viewport remains desktop only: normal desktop at `1440x900`, compact desktop at
`1024x768`, minimum width `1024px`. No mobile route is part of this inventory.
