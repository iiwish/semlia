# T004 Evidence Summary

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T004 实现 Web system-status 纵向基础 |
| Attempt | M0-T004-A001 |
| 状态 | Accepted |
| 执行日期 | 2026-08-10 |
| Branch | main |
| Base state | `2e53e6f` plus accepted, uncommitted P001 and T003 work |
| Packet | `docs/specs/m0-foundation/packets/T004.yaml` |
| Executor | Codex direct execution; delegation was not requested |

## 1. Scope Compliance

T004 交付独立的 `@semlia/web` React/Vite package，通过 `@semlia/sdk-typescript` 生成客户端读取真实 `/health/live`、`/health/ready` 和 `/api/v1/system/info`。生产代码没有 mock success fallback、手写 API DTO、M1 语义资产页面、认证或数据写入。

Implementation boundaries:

- `web/src/status.ts` 是生成客户端到 UI state 的唯一映射边界。
- `web/src/App.tsx` 只渲染 loading、ready、dependency unavailable 和 configuration error。
- Vite 在本地开发中将 same-origin `/health` 和 `/api` 代理到 `127.0.0.1:8080`；production build 不包含假响应。
- UI 仅支持项目约定的 1440x900 与 1024x768 桌面视口。

## 2. Delivered States

1. **Loading**: 显示 `Checking control plane`，refresh 保持稳定尺寸并禁用，`aria-live` 宣告检查中。
2. **Ready**: 同时展示 process live、dependency ready、API/schema/build version 和 trace ID。
3. **Dependency unavailable**: 展示服务端稳定 code、safe message、retryable 信号与 trace ID，同时保留可用 system info。
4. **Configuration error**: 在网络失败、非法响应或缺失系统信息时显示 `CONFIGURATION_ERROR`，不暴露 stack 或底层 exception。

## 3. Product And Visual Result

- Surface: quiet operational status console, not a marketing page or product dashboard.
- First viewport: Semlia identity, current system state, Service Signal Line, instance cards and trace context are visible without scrolling at supported viewports.
- Signature move: the code-native Service Signal Line connects Browser -> Control API -> Persistence and changes icon, label and semantic color together.
- Palette uses neutral canvas plus cobalt action, teal success, amber dependency warning and red configuration failure; no gradient, remote asset, nested card or one-hue wash.
- Stable three-column status grid remains readable at 1024px; screenshots show no horizontal overflow, overlap or clipped trace ID.
- Keyboard refresh has a visible focus ring; reduced-motion mode disables spinner animation and state transitions.

Design review: Pass, 27/28 (average 1.93/2). The single concern is intentionally restrained visual ambition for an M0 operational surface; product fit, first viewport signal, signature move, state craft, responsiveness and contract fidelity all pass.

## 4. Real Integration Check

The accepted T003 binary and T004 Vite app were started together without request interception:

```text
Browser -> http://127.0.0.1:4174
Vite proxy -> http://127.0.0.1:8080
GET /health/live -> 200 live
GET /health/ready -> 503 DEPENDENCY_UNAVAILABLE
GET /api/v1/system/info -> 200 v1 / 0.1.0 / dev
```

The rendered page truthfully showed `Dependency unavailable`, a live process, unavailable persistence, build `dev` and the readiness response trace ID. Screenshot: `docs/evidence/T004/screenshots/real-api-dependency-unavailable-desktop.png`.

## 5. Review Results

Spec compliance: Pass.

- All four required states exist and use the generated SDK client.
- API error code and trace ID are visible without exposing a stack.
- No mock production fallback or M1 product surface was introduced.

Bug and code quality: Pass with no blocking finding.

- lint, strict typecheck, 5 component tests, production build, 8 Playwright cases, SDK typecheck and contract drift checks pass.
- Loading, state replacement, abort cleanup, refresh, API failure, invalid response and long technical identifiers have explicit handling.

QA acceptance: Pass for founder review.

- Desktop and compact-desktop ready, dependency failure and configuration failure screenshots were inspected.
- Keyboard focus, Enter refresh, reduced motion and horizontal overflow assertions pass in both Playwright projects.

## 6. Screenshots

- `screenshots/ready-desktop.png`
- `screenshots/ready-compact-desktop.png`
- `screenshots/dependency-unavailable-desktop.png`
- `screenshots/dependency-unavailable-compact-desktop.png`
- `screenshots/configuration-error-desktop.png`
- `screenshots/configuration-error-compact-desktop.png`
- `screenshots/real-api-dependency-unavailable-desktop.png`

## 7. Diff

- Patch: `docs/evidence/T004/diff.patch`
- Patch scope: T004 Web source, tests, package manifest and lockfile delta relative to accepted P001/T003 state; governance files are excluded.
- Patch SHA-256: `f5bc13ccf532c750ab580b9388541ce67279df583655b411183110c559d6183f`
- Patch lines: 1,482.

## 8. Residual Risks

- T003 默认 readiness 在真实 PostgreSQL probe 接入前有意返回 503；ready 端到端状态目前通过真实合约响应拦截验证。
- Web build 尚未嵌入 Go binary 或纳入完整本地环境；该集成属于 T006。
- 页面目前为手动 refresh，没有 polling、authentication 或历史状态；这些不在 M0-FR-004 范围。

## 9. Acceptance Gate

T004 于 2026-08-10 获得 founder 明确接受，状态为 `Accepted`。后续任务已获得继续执行授权。
