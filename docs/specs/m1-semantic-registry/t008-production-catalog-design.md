# T008 Production Catalog Design

## Outcome

T008 turns the accepted M1 backend into the first production user-visible workflow. The root Web
application is a desktop Catalog workspace backed only by generated API contracts and PostgreSQL.
The system status view remains available at `/status`.

## Workflow

1. List workspaces and select a populated workspace first, then fall back to creation order.
2. Bootstrap an empty installation through an audited workspace creation API.
3. Search and filter current semantic assets by generated Catalog API types.
4. Create an asset and immutable first revision through the existing transactional mutation.
5. Open a detail panel showing public TypeIDs, revision content, schema, relation count and evidence.

## Interface Contract

- Desktop product scope is 1440x900 normal and 1024x768 compact; no mobile behavior is added.
- A narrow navigation rail preserves Catalog, Ontology, Sources, Lineage, Status and Settings
  information architecture while only Catalog and Status are active M1 routes.
- Metrics are individual cards; the registry remains a dense table; detail is a single side panel.
- Empty, loading, failure, create and selected states are explicit and keyboard reachable.
- No prototype mock data is embedded in the production bundle.

## Backend Support

`GET/POST /api/v1/workspaces` is the minimum bootstrap surface. Workspace creation allocates a
TypeID/UUIDv7 identity and commits the workspace plus `workspace.created` audit event atomically.
The API never exposes the storage UUID.

## Acceptance

- Generated Go and TypeScript contracts cover workspace list/create.
- PostgreSQL integration proves audit, ordering and duplicate-slug conflict behavior.
- Web unit tests prove registry load and detail interaction.
- The embedded Compose application shows API-created data without horizontal overflow at both
  supported viewports, and create/search/detail interactions work against the live backend.
