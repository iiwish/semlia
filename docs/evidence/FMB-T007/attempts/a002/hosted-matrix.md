# FMB-T007 A002 Hosted Acceptance

Status: External acceptance is deferred. Assessment date: 2026-09-05.

The founder confirms no server is available and authorizes local testing and closeout
first. The gates below remain unexecuted externally. They do not block this authorized
local closeout and do not become production acceptance through local test results.

Local PostgreSQL integration tests exercise actual governed publication and bounded
read-only execution with isolated synthetic data. They do not constitute a hosted
deployment, live model-provider acceptance or production-source approval.

| Gate | Local evidence boundary | Required external evidence | Status |
| --- | --- | --- | --- |
| HTTPS and OIDC | Local HTTP and deterministic session/CSRF tests | Approved hostname, trusted certificate, issuer/client registration, actual login/logout and session policy | Environment not supplied |
| Chat | Deterministic provider and application tests | Approved provider/model and actual Ask interpretation against published assets | Not established |
| Embedding | PostgreSQL/pgvector lifecycle and deterministic provider tests | Approved provider/model, rebuild, retrieval quality and cost/latency results | Not established |
| Webhook | Controlled receiver, signature, retry and persistence tests | Approved external HTTPS receiver, real delivery/signature and retry recovery | Not established |
| Read-only source | Isolated PostgreSQL and dedicated execution role | Approved source, pinned identity, dedicated least-privileged credential and server logging policy | Environment not supplied |
| OpenTelemetry | Configuration and redaction contracts | Approved collector and verified end-to-end redacted trace | Environment not supplied |
| Recovery | Local migration/process/database tests | Hosted backup and restore into an isolated approved target | Not established |
| Release deployment | Local candidate verification and development migration21 upgrade/smoke evidence | Deployment and smoke of that exact verified artifact | Deferred, not deployed |

Preflight inspects configuration-key presence and endpoint status, not secret values.
The existing development preview at `http://127.0.0.1:18081` uses explicit local UAT
identities; its session endpoint returns HTTP 503 at preflight. The inspected
`.semlia/dev.env` has no OIDC issuer/client/redirect, public URL or execution
configuration keys. This does not establish whether other configuration exists.

No approved external environment has been provided in this attempt. No external
deployment is authorized or performed. Local fixtures cannot close these gates.
