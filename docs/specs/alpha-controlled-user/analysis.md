# Controlled-User Alpha Planning Analysis

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Completed |
| Requirements | `docs/specs/alpha-controlled-user/spec.md` 0.1.0 Confirmed |
| Plan | `docs/specs/alpha-controlled-user/plan.md` 0.1.0 Ready_For_User_Review |
| Work graph | `docs/specs/alpha-controlled-user/tasks.md` 0.1.0 Ready_For_User_Review |
| Last updated | 2026-09-04 |

## Result

Result: ready for founder technical-plan review. No implementation task is authorized by this
analysis alone.

The plan covers every confirmed Alpha requirement with seven dependency-ordered work units and a
single integrated acceptance task. It uses the existing M1/M2 registry, jobs, authorization,
governance, release and Web foundations rather than building parallel systems. No requirement needs
a product-scope decision before implementation.

## Decision Register

| ID | Decision | Result | Reason |
| --- | --- | --- | --- |
| D1 | OIDC implementation | Generic Authorization Code plus PKCE with `go-oidc/v3` and `x/oauth2` | Matches ADR-0002 and current standards; no custom token verification |
| D2 | Browser session | Opaque hashed PostgreSQL session | Immediate revocation and membership enforcement without Redis or browser JWT authority |
| D3 | Multi-workspace identity | Global user account mapped to existing workspace principals through memberships | Preserves current workspace-scoped authorization while allowing one login to see multiple workspaces |
| D4 | Authorization evaluator | Keep the existing Semlia evaluator for Alpha | It already enforces the owned contract; adding Casbin would create migration risk without reducing the Alpha threat surface |
| D5 | Source secret | Versioned HKDF-SHA256 plus AES-256-GCM local envelope | Satisfies encrypted-at-rest and rotation needs while leaving a future KMS adapter boundary |
| D6 | PostgreSQL discovery | Live catalog collector plus the existing versioned SQL parser | Prevents the current artifact parser from being misrepresented as a database connector |
| D7 | Discovery execution | Existing leased PostgreSQL job framework | Provides retries/restart without new infrastructure |
| D8 | Semantic candidates | Persisted non-authoritative discovery output with append-only decisions | Bridges source evidence to M2 proposals without making discovered objects queryable truth |
| D9 | Resolution authority | Immutable release asset and release object projection | Prevents mutable current pointers or drafts from leaking into production plans |
| D10 | Query interpretation | Model proposes typed selectors; deterministic service resolves them | Enforces SSOT D-015 and explicit ambiguity |
| D11 | Query execution | Not configured in Alpha | The approved scope excludes arbitrary warehouse execution; plan explanation remains honest and testable |
| D12 | Work cadence | Continuous T001 through T007 after one technical-plan approval | Keeps the 4-to-7-day window credible while retaining evidence and founder acceptance gates |

D4 narrows the confirmed access-control TDR's Casbin adapter direction for this Alpha delivery. It
does not change the Semlia-owned `Authorizer` contract or forbid a later Casbin adapter. Founder
approval of the Alpha TDR explicitly accepts that sequencing decision.

## Cross-Contract Checks

| Concern | Resolution |
| --- | --- |
| `principals` are workspace-scoped but one session lists multiple workspaces | `user_accounts` and `external_identities` are login authority; each membership resolves to one existing workspace principal |
| `__Host-` cookies require Secure while local Alpha uses HTTP | Secure environments use `__Host-semlia_session`; explicit local HTTP uses a different development cookie name |
| Current HTTP handlers accept `X-Semlia-Principal` | Session middleware becomes authoritative; the header remains only in explicit development local-UAT mode and is ignored in session mode |
| Existing source table has `credential_ref` but no secret service | A dedicated versioned ciphertext table and worker-only decrypting port are added; source projections stay redacted |
| `postgresql_sql` consumes files, not a live database | A live catalog collector produces a catalog snapshot; the parser continues to own only versioned SQL artifacts |
| M1 promises keys and Join observations but the current discovery schema lacks them | T003 adds physical key and Join observation records; they remain evidence and cannot substitute for governed EntityKey/JoinContract |
| Current discovery creates a run during synchronous snapshot persistence | Start-run creates the queued run and job atomically; worker completion targets that exact run |
| The current schema has no semantic candidate aggregate | T003 adds run/source-revision candidates and append-only conversion/dismissal decisions |
| M2 proposal creation expects an existing semantic asset/revision | Candidate conversion idempotently creates the minimum draft asset/revision when absent, then opens the normal M2 proposal |
| Governed objects are mutable by version | Resolution loads the exact versions pinned in `release_objects`, never the latest row version |
| Ask currently claims Cube execution and fixed numerical results | T006 removes those production fixtures; data intents return a validated unexecuted plan |
| Alpha asks for consumer binding while complete ecosystem work is excluded | T005 implements only minimal consumer and current/pinned binding contracts; MCP, CLI and webhooks remain full-M3 work |
| M2/T011 is Running with live-provider/external-auth gaps | T007 supplies real identity and provider evidence, then moves T011 to `Needs_Review`; acceptance remains a founder verdict |

## Requirement Traceability Check

| Contract | Coverage |
| --- | --- |
| FR-001 through FR-004 | T001, T002 and T007 |
| FR-005 and FR-006 | T003, T004 and T007 |
| FR-007 | T006 and T007 |
| FR-008 and FR-009 | T005, T006 and T007 |
| FR-010 | T006 and T007 |
| FR-011 | T002, T004, T006 and T007 |
| FR-012 | T001, T003, T005 and T007 |
| NFR-001 security/privacy | T001, T003, T005, T006 and T007 |
| NFR-002 reliability/recovery | T001, T003, T005 and T007 |
| NFR-003 performance | T001, T005 and T007 |
| NFR-004 accessibility/desktop | T002, T004, T006 and T007 |
| NFR-005 observability | T001, T003, T005, T006 and T007 |

Coverage gaps: none.

## Constitution Check

| Principle | Result |
| --- | --- |
| Semantic authority is asset- and release-based | Satisfied: only release-pinned revisions and object versions resolve |
| AI cannot directly publish or invent semantic truth | Satisfied: model output enters typed validation and deterministic resolution |
| Git-native released content and PostgreSQL control state | Satisfied: no authority boundary changes |
| Headless-first contracts | Satisfied: OpenAPI and one application resolver precede Web Ask |
| Default security and privacy | Satisfied: server sessions, CSRF, encrypted credentials, metadata-only source reads and no raw prompt/fact retention |
| Explicit failure and refusal | Satisfied: connector, provider, authorization and resolver failures cannot become success |
| Desktop product scope | Satisfied: only 1440x900 and 1024x768 acceptance is added |
| Honest incomplete surfaces | Satisfied: later milestone routes remain visible but marked Prototype |

Constitution violations: none.

## Delivery Risks

| Risk | Impact | Control |
| --- | --- | --- |
| Identity touches every API handler | High | Land middleware/context first; run 401/403 and existing M1/M2 regression suites before Web work |
| Three migrations cross identity, source and distribution data | High | Separate migrations, populated up/down/up fixtures and no rewrite of immutable M1/M2 rows |
| Source permissions differ across PostgreSQL providers | Medium | Capability diagnostics, read-only transaction, bounded catalog queries and one supported fixture contract |
| Candidate-to-proposal exposes an M2 creation assumption | Medium | One idempotent application command with transactional asset/revision/proposal creation and repeat tests |
| Lexical ambiguity thresholds may over-resolve | High | Exact references first, golden cases, conservative margin and refusal by default |
| Live provider availability is external | Medium | Deterministic schema/error tests always run; real provider evidence is recorded when credentials are present and never fabricated |
| Existing worktree contains M2/T011 changes | Medium | Build on those changes, avoid reset/revert, and keep task evidence scoped |
| Seven-day window can be consumed by hosted credential setup | Medium | Local standards-compliant issuer and source fixture prove the implementation; hosted acceptance input stays explicit |

## Scope Accounting

After T007, Controlled-User Alpha 0.1.0 can be recommended for real invited-user testing. Remaining
product work is intentionally substantial: full M3 still needs MCP, CLI, webhooks, optional execution
and independent external-consumer acceptance; M4 and M5 remain untouched. Those items do not block
the approved Alpha journey and must not be reported as completed.

## Review Request

Founder review should approve or reject these artifacts as one technical baseline:

- `docs/specs/alpha-controlled-user/technology-decision-record.md`
- `docs/specs/m3-headless-semantic-distribution/spec.md`
- `docs/specs/m3-headless-semantic-distribution/plan.md`
- `docs/specs/m3-headless-semantic-distribution/tasks.md`
- `docs/specs/alpha-controlled-user/data-model.md`
- `docs/specs/alpha-controlled-user/plan.md`
- `docs/specs/alpha-controlled-user/tasks.md`

Approval authorizes T001 packet creation and continuous sequential execution through T007. It does
not accept M2/T011, M3 or the Alpha release before implementation evidence exists.
