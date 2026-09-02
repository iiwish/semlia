# M1 T008 Delivery Evidence

## Outcome

The production root is a usable semantic Catalog backed by the M1 API and PostgreSQL. The shipped
bundle contains no Catalog mock dataset. Users can create or select a workspace, search and filter
semantic assets, create an immutable first revision, and inspect definition content, public TypeIDs,
relations and evidence. Operational status remains available at `/status`.

## Backend And Contract Evidence

- `GET /api/v1/workspaces` prioritizes populated workspaces for Catalog selection.
- `POST /api/v1/workspaces` allocates a TypeID/UUIDv7 identity and atomically commits the workspace
  with a `workspace.created` audit event.
- Generated Go and TypeScript contracts expose workspace list/create without leaking storage UUIDs.
- Real PostgreSQL integration covers creation, listing, duplicate-slug conflict and audit behavior.

## Product Evidence

- The live embedded application loaded the API-created `Semantic Core` workspace and five semantic
  assets at `1440x900` and `1024x768`.
- Both viewports had an exact viewport-width document (`1440/1440` and `1024/1024`) with no
  horizontal overflow or overlapping controls.
- Live search returned revenue-related definitions and excluded unrelated assets.
- The asset detail panel exposed the current revision, structured definition content, relations and
  evidence state using public TypeIDs.
- The new-asset dialog exposed semantic address, title, type and summary fields.
- Browser warning and error logs were empty after interaction.

## Verification

Passed on 2026-09-02:

- `make check-source`
- `make check-smoke`
- `git diff --check`
- Live in-app browser QA at `1440x900` and `1024x768`

T007 already supplies the unchanged M1 dependency security scan, release bundle and 10,000-asset
performance evidence. T008 adds no dependency or migration.
