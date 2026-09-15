# FMB-T002 Live UAT Screenshot Manifest

Captured: 2026-09-05 Asia/Shanghai  
Base URL: `http://127.0.0.1:18081`  
Mode: server-mode local UAT; `VITE_CATALOG_FIXTURE` disabled

## Authority Boundary

- workspace: `wsp_01m1nsffrgew9tce0cqcr2k42v`
- actor: `local-author`
- attention item: `ati_01m1q54q7deahb8ewkdet9hdr4`
- asset: `ast_01m1nsffs0ewabg3jxf54hp4wx`
- current revision: `rev_01m1nsffs0ewav5t3x02g8xba6`
- release: `rls_01m1nsfhh1ewjb9w79vjw6r5j7`
- released revision: `rev_01m1nsfhh4ewn9yejweyzw3ezc`

The local stack has no available identity session: `/api/v1/session` returns 503. For each reload,
the exact deep-link URL remained intact and the harness reselected the same authorized workspace
through the explicit UAT actor boundary before asserting the persisted target. This proves T002
route recovery and server authority, but does not claim session-backed workspace restoration.

## Files

| File | Viewport | Surface | SHA-256 |
| --- | --- | --- | --- |
| `desktop-workbench-detail.png` | 1440x900 | persisted Workbench item and runtime target | `d3e6c5bd5f1b47cbded5cae1e4e86ba1a9dd30fb5a28e0a8ce0c1f7c61ba6efc` |
| `desktop-asset-detail.png` | 1440x900 | authoritative asset, revision and section availability | `227be637ac4db80fe9081af2ec338751b1a01fb6d3d16ad5b8267b95405a5633` |
| `desktop-release-detail.png` | 1440x900 | immutable release detail and manifest pin | `4e927245ab1627ea493c5a4ef528b16ed2d5cf4656534d586409f2ebf0b93dd6` |
| `desktop-release-dual-diff.png` | 1440x900 | open comparison dialog with prior-pin and current-registry groups | `2d7412846a2c8ed6c8978e927d62349955ebd21a4bee53064dd60fac9fda3060` |
| `compact-desktop-workbench-detail.png` | 1024x768 | persisted Workbench item and runtime target | `64058f777979e54f4df520468f1be1579575b2475964407eb007f5d926c6161b` |
| `compact-desktop-asset-detail.png` | 1024x768 | authoritative asset, revision and section availability | `658cafa0bb45ecdbc47b37edf2c0ba25adbf2b4500cbe45687a2ef1001446c5d` |
| `compact-desktop-release-detail.png` | 1024x768 | immutable release detail and manifest pin | `1934db9d120617ceab08d361b862a10ff401721b14919634beb75220f2645a30` |
| `compact-desktop-release-dual-diff.png` | 1024x768 | open comparison dialog with prior-pin and current-registry groups | `43de75bbf3ee240020fa75e36884ab48381d913adbb394b7c2223f45cd02a70d` |

## Assertions

- Workbench, asset and release exact URLs remained intact across reload and resolved the same IDs
  after authorized workspace selection.
- The dual-diff dialog visibly separates `与前一发布固定版本比较` from `与当前注册表比较` at both
  supported viewports.
- Global and target horizontal-overflow checks were false at 1440x900 and 1024x768.
- The browser emulated reduced motion and a keyboard Tab moved focus to a visible button.
- No `Prototype` or `Fixture` disclosure was present because this was not a fixture build.
- The compact release publisher/time cell was tight but did not overlap or overflow.

Machine-readable observations are in `uat-observations.json`.
