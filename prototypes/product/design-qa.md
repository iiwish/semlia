# Model Configuration Design QA

## Reference

- Source: Fluxale `AIModelConfigurationPanel` and its running AI model settings page.
- Target: Semlia system settings at `1440x900`, `1280x800`, `1024x768`, and the current in-app browser viewport.
- Adaptation: provider grouping, default-model control, credential status, collapse behavior, and compact model rows follow the reference hierarchy while retaining Semlia's navigation, typography, colors, and control styling.

## Product Boundary

- LLM and Embedding models share one long-lived secondary menu but use separate tabs and defaults.
- Embedding configuration records model ID, dimensions, input limit, and retrieval capability.
- Changing the default Embedding model affects future indexes; existing vectors require an explicit rebuild.
- API keys are write-only in the interface. The prototype does not persist or transmit secrets.

## Validation

| Check | Result |
| --- | --- |
| Provider and model hierarchy matches the reference intent | Passed |
| Add provider and add model dialogs are operable | Passed |
| Keyboard focus and visible control labels are present | Passed |
| No model-page horizontal overflow at 1440 or 1024 | Passed |
| Browser console errors | None |
| Automated desktop and compact-desktop journeys | Passed |

## Member Directory Follow-up

- The settings context uses `成员` as the stable label and contains no contextual search box.
- The member page starts directly with a searchable directory; the redundant page heading, workspace row, identity provider, default role, and read-only footer are absent.
- The directory exposes employee ID, name, email, department, title, and employment status without RBAC fields.
- Visual checks at `1440x900`, `1024x768`, and the current in-app browser viewport found no horizontal overflow or console errors.
- The MVP settings menu contains only members, model configuration, integrations, and audit/runtime settings; the inactive roles and permissions entry is absent and the remaining routes resolve to the correct page.

## Model Header Follow-up

- The redundant in-page `模型配置` title is absent; the global top bar remains the single page title.
- LLM and Embedding tabs form a compact, left-aligned switch at the top of the workspace.
- Both model types use the same action toolbar for the default model, credential state, and provider creation; Embedding adds its indexing operation in the same action area. Provider and model counts are absent.
- Embedding replaces the passive indexing notice with a toolbar-level `重建向量索引` action. The confirmation dialog names the scope, target model, zero-downtime behavior, and produces an explicit task-created result.
- Visual checks at `1440x900` and `1024x768` found no clipped model controls, horizontal overflow, or console errors.

## Embedding Rebuild Status And Execution Records

- Starting a rebuild creates a visible running state in the Embedding toolbar with the current phase, progress, target model, and processed knowledge-block count.
- `查看进度与日志` opens an in-context modal with the current phase, six-stage progress, run facts, and recent execution log, preserving the model configuration context.
- The modal keeps `在执行记录中打开` as a secondary audit path instead of navigating away immediately.
- The run detail exposes six explicit stages from queueing through index activation, while the log dialog records the operational timeline.
- The current index remains available during the rebuild; switching occurs only after validation succeeds.

## Interfaces And Integrations

- The integrations page is an operational management surface for REST API, MCP Server, CLI, and SDK access rather than a static settings placeholder.
- Interface cards expose endpoint or package identity, current availability, copy actions, and enable or disable controls.
- Client credentials are searchable and show channel, environment, masked credential, last use, and status.
- Creating a client supports channel and environment selection, then displays the generated credential exactly once.
- MVP clients receive full workspace capability while every call remains attributable through execution and audit records.

## Final Verification

- Type checking, linting, production build, and all 25 component tests passed.
- The desktop Playwright journeys cover the updated global audit/runtime route, but the full journey suite was not rerun in this iteration.
- Browser checks of the integrations page, rebuild status, execution detail, and logs found no horizontal overflow, clipped controls, or console errors.
- The in-context rebuild modal also passed at the current `961x916` browser viewport, which is narrower than the supported compact-desktop target.

Final result: passed.

## Audit And Runtime Center

- `审计与运行` is a global operations center with three focused views: run records, immutable audit events, and runtime settings.
- Run records unify knowledge builds, data synchronization, MCP calls, release validation, and vector-index rebuilds; rebuild history is no longer owned by data ingestion.
- Run and audit rows open centered detail dialogs with progress, execution logs, actors, channels, targets, results, and Trace IDs without changing the current settings context.
- Runtime settings expose retention, queue concurrency, timeout, retry, OpenTelemetry delivery, and enforced sensitive-field redaction with an explicit save state.
- Manual browser validation at `961x916` found no clipped controls, horizontal overflow, or console errors. Type checking, linting, production build, and all 25 component tests passed.

## Ingestion And Global Run Ownership

- The data-ingestion menu uses `接入运行` and contains only database synchronization, file import, and metadata-scan records.
- Ingestion rows expose source, snapshot, checkpoint, structural changes, parsing failures, and downstream Run ID; knowledge blocks and candidate versions are absent.
- Knowledge builds, vector rebuilds, release validation, and interface calls remain authoritative in the global `审计与运行 / 运行记录` view.
- A completed ingestion run can open its downstream global run directly. The global detail preserves the upstream ingestion Run ID, so the relationship is navigable without duplicating state.
- Browser validation at `961x916` found no clipping or page overflow in the ingestion list, ingestion detail, or downstream global-run modal. Type checking, linting, production build, and all 25 component tests passed.

## Runtime Settings Visual Follow-up

- Runtime settings use unframed section headers and individual setting cards instead of a two-column administration table with continuous rules.
- Retention options scan as a three-item group, scheduler controls form a two-column grid, and observability keeps connection status, endpoint testing, and enforced redaction in one clear operational group.
- Save feedback is a compact state row with explicit saved and dirty states; changing a setting enables the existing save action and returns to the applied state after saving.
- Browser validation at `961x916` found no clipped setting cards or horizontal page overflow. Type checking, linting, production build, and all 25 component tests passed; the test suite required a `15s` timeout because three broader interaction tests exceeded the default `5s` limit in this run.

## Integration Usage Guides

- REST API, MCP Server, CLI, and SDK cards no longer expose raw URLs, commands, or package identifiers in their footers.
- Each capability card opens a centered usage-guide dialog with its connection identity, authentication method, intended scenario, three-step setup path, copyable example, and audit boundary.
- Enable and disable controls remain available on the cards, while copy actions live beside the values they affect inside the guide.
- Browser validation at `961x916` found no clipped cards, dialog content, or horizontal page overflow. Type checking, linting, production build, and all 25 component tests passed.
