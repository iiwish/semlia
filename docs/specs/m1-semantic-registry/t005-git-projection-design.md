# T005 Git Content Projection Design

## Decision

T005 uses `go-git/v5` behind a repository-neutral projection port. The control-plane release image is
distroless and has no system Git binary, so shelling out would make production behavior depend on an
undeclared executable. The stable v5 line supports local repositories in-process and keeps transport
and credential concerns outside this M1 local adapter.

PostgreSQL remains authoritative. A committed `catalog.asset.changed` outbox event identifies one
immutable asset revision; the projector loads that revision from PostgreSQL and writes a deterministic
human-reviewable JSON document to:

```text
assets/<namespace segments>/<key>.json
```

The document uses `semlia.io/v1` and `SemanticAsset`, carries TypeID identities, semantic address,
asset type, lifecycle, revision sequence/digest/author/time, and the canonical revision content under
`spec`. It ends with one newline. No database UUID, credential, evidence body or customer fact row is
written.

## Replay And Concurrency

- Asset creation projects with no base revision; revision append includes the previously selected
  revision as `baseRevisionId` in the atomic outbox payload.
- If the file already contains the target revision, replay is a no-op and creates no commit.
- If the file does not contain the expected base revision, projection fails with a conflict and leaves
  the repository unchanged.
- The adapter refuses a dirty worktree and serializes writes in-process. The outbox lease remains the
  cross-process single-delivery boundary for M1.
- A successful write stages only the canonical asset path and commits with a fixed Semlia author,
  revision time and deterministic message.

## Boundaries

- The application port accepts a typed projection, not a Git library object.
- The PostgreSQL loader verifies workspace, asset, revision and sequence together.
- The local adapter initializes a configured directory as a repository when needed; remote clone,
  credentials, push, branch policy and pull-request creation are deferred.
- Unsupported outbox event types are not silently acknowledged. T006 owns the complete event router
  and production dispatcher wiring.

## Acceptance Matrix

| ID | Scenario |
| --- | --- |
| GIT-001 | A committed revision produces the golden canonical document and one commit. |
| GIT-002 | Replaying the same outbox event changes neither file bytes nor HEAD. |
| GIT-003 | An append with the expected base updates the document and creates one commit. |
| GIT-004 | A stale/missing base or dirty worktree fails without modifying tracked content. |
| GIT-005 | PostgreSQL loading is workspace-isolated and rejects event/revision mismatches. |
| GIT-006 | A failed delivery becomes retryable and a later replay succeeds without partial state. |
