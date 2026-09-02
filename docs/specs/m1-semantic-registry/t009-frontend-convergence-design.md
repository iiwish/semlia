# T009 Single Production Frontend Convergence

## Decision

`web` is Semlia's only runnable and embedded frontend. The accepted desktop product experience is
implemented there directly; `prototypes/product` is removed after its source, tests and product
journeys are promoted. The Go binary continues to embed `web/dist`, and `/status` remains an
operational route inside the same application.

## Product Contract

- Preserve the accepted Chinese information architecture, dense desktop workspace, ontology views,
  semantic question flow, release workbench, source workflows and administration surfaces.
- Support 1440x900 and 1024x768. Mobile remains outside the product contract.
- Use the M1 generated TypeScript SDK for workspaces, Catalog search, asset creation, asset detail,
  revisions, evidence and bounded relations.
- Show persistent backend records with their public TypeIDs. Storage UUIDs remain private.
- Label future-milestone and session-only behavior honestly. Preview data may support those views,
  but it cannot act as a production fallback for a failed M1 request.
- A missing workspace is a bootstrap state; an unavailable API is an explicit error state.

## Runtime Boundaries

| Surface | Runtime source |
| --- | --- |
| Workspace selection and bootstrap | M1 API and PostgreSQL |
| Catalog list, filter, create and detail | M1 API and PostgreSQL |
| Revision, evidence and relation facts | M1 API and PostgreSQL |
| System health | Existing status API |
| Ask, release orchestration, source setup and administration | Marked preview/session behavior until their backend milestone lands |
| Unit and journey fixtures | Test-only local fixtures |

The production Catalog never silently substitutes fixtures when a request fails. Catalog summaries
load first; immutable detail and bounded relations load on demand to avoid an N+1 request fan-out.

## Interaction Contract

- The top bar exposes the active workspace and connection state without displacing the accepted
  primary navigation.
- Knowledge Assets is the real M1 registry. Search and type filtering are server-backed.
- Creating an asset writes through the generated client, refreshes the registry and opens the real
  returned detail.
- Backend loading, empty and failure states stay within the existing workspace geometry.
- Preview-only commands identify their scope at the point of action.

## Removal Contract

- Delete `prototypes/product` and the `@semlia/product-prototype` workspace importer.
- Delete root and Make targets that start or build a second frontend.
- Preserve historical product specifications and evidence as records; canonical repository and
  runtime documentation point only to `web`.

## Verification

Run lint, typecheck, unit tests, production build, Playwright desktop journeys, repository checks,
source checks, Compose smoke tests, and live 1440x900 plus 1024x768 visual QA. Verify that the built
Go application serves the product at `/` and system status at `/status`.
