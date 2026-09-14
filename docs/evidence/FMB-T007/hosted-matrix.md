# FMB-T007 Hosted Matrix

Status: External acceptance not established. Inspection date: 2026-09-05.

The current preview is a local development deployment at `http://127.0.0.1:18081`.
Its `/api/v1/session` endpoint returns HTTP503 at sprint preflight. The local Compose
configuration explicitly enables development identities. This is not hosted OIDC login.
Only configuration-key presence and endpoint status were inspected; no secret values
are included in this evidence.

| Gate | Available local evidence | Required hosted evidence | Status |
| --- | --- | --- | --- |
| HTTPS and login | Local HTTP preview; deterministic session/CSRF/membership tests | Approved hostname, trusted certificate, actual issuer/client registration, login/logout and session policy | Not supplied |
| Chat | Existing application/provider contract and failure-path tests | Approved live model credential and real Ask interpretation against released assets | Not established |
| Embedding | T005 real PostgreSQL/pgvector lifecycle with deterministic provider tests | Approved live embedding provider/model, rebuild and retrieval quality/cost/latency measurement | Not established |
| Webhook | T006 signature, SSRF, retry/lease and persistence tests with controlled receivers | Approved external HTTPS receiver, real delivery/signature verification and retry recovery | Not established |
| Read-only source | T007 local PostgreSQL adapter verification in progress | Approved dedicated source and least-privileged execution credential, pinned-source query proof | Not supplied |
| OpenTelemetry | Existing configuration/contracts | Approved collector endpoint and verified redacted end-to-end trace | Not supplied |
| Recovery | Repository migration and local process/database recovery tests | Hosted backup, restore into an isolated target and documented recovery result | Not established |
| Release artifact | T007 local candidate build/security/provenance gates in progress | Deployment of that exact checked artifact and hosted smoke result | Not deployed |

The inspected `.semlia/dev.env` has no OIDC issuer/client/redirect, public URL or execution
configuration keys. This does not assert that no such configuration exists elsewhere.
The founder has been asked for configuration locations and approved environment names,
not passwords or keys. Until an environment is supplied and verified, the corresponding
hosted gates remain open; deterministic fixtures cannot close them.
