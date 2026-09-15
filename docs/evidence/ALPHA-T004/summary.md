# ALPHA-T004 Delivery Summary

## Outcome

The production source workspace uses authenticated generated-contract APIs for PostgreSQL source
administration, credential rotation, connection tests, asynchronous discovery runs and semantic
candidates. API loss renders an explicit retryable failure and never substitutes fixture data.

An authorized source operator can create, edit, pause, test and delete a source, rotate its
write-only password, start discovery, inspect the exact persisted run and open its findings. The
credential input is always empty when opened and is cleared after either success or failure.

Pending candidates create and submit a real M2 proposal before Semlia appends the immutable
conversion decision. The proposal workbench shows the stable source, source-revision, run and
candidate identifiers. When proposal submission fails the candidate stays pending; when the final
decision write fails the in-session retry reuses the already-created proposal instead of creating a
duplicate.

## Product Boundary

- PostgreSQL source setup, connection testing, discovery runs and candidate decisions are real.
- File import and generic automation remain explicit Prototype surfaces and do not claim server
  persistence.
- Production navigation retains all five desktop product areas at both supported viewports.
- The legacy fixture source view remains available only in explicit fixture governance mode for
  deterministic component coverage.

## Accessibility And Visual Result

The live journey passes at 1440x900 and 1024x768 with no horizontal overflow and zero serious or
critical axe findings. Screenshots are stored under `docs/evidence/ALPHA-T004/screenshots/` for the
source workspace and resulting M2 proposal at both viewports.

## Residual Risk

The production JavaScript bundle remains above Vite's 500 kB advisory threshold. Candidate recovery
after a browser reload between successful proposal submission and failed decision persistence is not
yet automatic; the normal operation and in-session retry are idempotent, and T007 retains the final
recovery audit.
