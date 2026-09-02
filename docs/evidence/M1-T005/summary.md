# M1 T005 Evidence Summary

## Metadata

| Field | Value |
| --- | --- |
| Task | T005 Git content projection |
| Status | `Accepted` |
| Implementation | Pending commit |
| Date | 2026-09-02 |
| Packet | `docs/specs/m1-semantic-registry/packets/T005.yaml` |

## Delivered

- Added a repository-neutral typed projection port and exact PostgreSQL immutable-revision loader.
- Added a local `go-git/v5` adapter compatible with the distroless production image, with secure
  path resolution, clean-worktree enforcement and atomic file replacement.
- Added deterministic `semlia.io/v1` `SemanticAsset` JSON documents and commit metadata.
- Added optimistic `baseRevisionId` to revision outbox facts, target-revision replay detection and
  stale-base conflict handling.
- Added a catalog outbox publisher that rejects malformed and unsupported event types rather than
  silently acknowledging them.

## Recovery Proof

- First delivery into a deliberately dirty repository becomes retryable without creating asset
  content.
- Removing the dirty file allows the same leased outbox event to succeed on its second attempt.
- Same-target replay preserves both bytes and HEAD; stale-base replay preserves tracked content.
- Projection loading with another workspace fails before any Git write.

## Validation

| Command | Result |
| --- | --- |
| `go test -race ./internal/domain/projection/... ./internal/application/projection/... ./internal/adapters/gitcontent/...` | Pass |
| `go test -count=1 ./tests/integration/projection/...` | Pass; PostgreSQL 18 container |
| `make db-generate-check` | Pass |
| `make check-source` | Pass; format, lint, typecheck, all tests, drift and release build |
| `git diff --check` | Pass |

## Review

Git is a reviewable projection only. The adapter never receives credentials or PostgreSQL rows,
does not push to a remote, and cannot overwrite a projection whose revision does not match the
expected base. `go-git/v5` is pinned to `v5.19.2`, the current stable security-maintained v5 release
selected for the executable-free distroless runtime.
