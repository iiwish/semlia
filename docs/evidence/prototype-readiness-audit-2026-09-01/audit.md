# Semlia Prototype Readiness Audit

## Audit scope

- Date: 2026-09-01
- Mode: combined product, UX, accessibility-risk, and open-source-readiness audit
- Target: `prototypes/product` at 1440x900 and 1024x768
- User goal: determine whether the current prototype is complete, whether it represents a world-class open-source semantic platform, and whether RBAC or ABAC belongs in the product direction

## Overall verdict

The prototype has a strong, differentiated product structure and a credible enterprise workspace visual language. It is suitable as a north-star prototype for semantic discovery, governed authoring, immutable release, consumption, and operational traceability.

It is not complete. Identity and authorization are explicitly absent from the member surface, while the product contract requires workspace RBAC, separation of duties, scoped tokens, command-level authorization, and authorization-aware semantic resolution. The current repository also remains an M0 foundation: the public API exposes only liveness, readiness, and system information, and the prototype uses repository-local mock state.

The prototype is above the bar for a serious incubation project, but it does not yet prove world-class open-source product readiness. Security administration, policy administration, real service contracts, self-hosting operations, extension conformance, scale evidence, and a clean end-to-end regression suite are still required.

## Captured flow

| Step | Surface | Health | Evidence |
| --- | --- | --- | --- |
| 1 | Governed semantic answer | Strong | `01-ask.png` |
| 2 | Workbench and prioritized decisions | Strong | `02-workbench.png` |
| 3 | Semantic asset catalog | Strong | `03-knowledge-catalog.png` |
| 4 | Asset revision, trust, and delivery context | Strong | `04-asset-detail.png` |
| 5 | Candidate and immutable asset versions | Strong with terminology risk | `05-change-release.png` |
| 6 | Source connection and ingestion entry | Partial | `06-data-ingestion.png` |
| 7 | Member administration and authorization boundary | Incomplete | `07-members-no-rbac.png` |
| 8 | Compact desktop member administration | Visually stable, functionally incomplete | `08-members-1024.png` |

## Strengths

- The product is organized around durable semantic objects and decisions rather than generic dashboards.
- The answer surface makes release, evidence, source boundaries, and governed execution visible.
- The knowledge catalog supports semantic assets, relations, physical bindings, and JoinContracts in one coherent model.
- Asset detail exposes definition, ontology, implementation, trust, delivery, and impact as task-specific views.
- Change review, immutable versions, release provenance, consumer bindings, audit records, and runtime diagnostics form a credible governance narrative.
- The visual system is quiet, dense, and appropriate for repeated desktop operations. The inspected 1024x768 member surface had no page-level horizontal overflow.

## Highest-impact gaps

1. **Identity and authorization:** there is no role, permission, group, service-account, token-scope, access-policy, separation-of-duties, or effective-permission surface.
2. **Policy administration:** G0-G3 governance is visible in product language, but administrators cannot author, simulate, version, approve, or audit the policy that produces a decision.
3. **Workspace administration:** workspace creation, switching, identity-provider configuration, domain ownership, teams, environments, notifications, secrets, backup, and upgrade posture are not represented as a coherent administrative journey.
4. **Real product contract:** the current OpenAPI contract contains only system endpoints. The prototype does not yet prove real semantic registry, proposal, release, search, resolution, client, or authorization APIs.
5. **Large-scale operations:** bulk assignment, bulk review, access recertification, policy simulation, conflict analysis, and 100,000-asset behavior are not represented or benchmarked.
6. **Extension and open-source readiness:** connector SDK ergonomics, conformance tests, example projects, upgrade paths, plugin discovery, and maintainer/community workflows need product-level proof, not only repository statements.
7. **Regression health:** lint, typecheck, build, and 25 component tests pass, but the current Playwright run has 6 failures and 8 passes across desktop and compact desktop.
8. **Frontend delivery:** the production build emits a 531.27 kB JavaScript chunk and a Vite large-chunk warning. This is acceptable for an evolving prototype but should not become the production baseline.

## Authorization direction

RBAC is a release requirement, not an optional follow-up. Start with workspace-scoped RBAC and separation of duties, then add attribute-aware policy conditions without exposing a general-purpose ABAC builder in the first version.

Recommended first authorization model:

- Principals: users, groups, service accounts, API clients, and agents.
- Scopes: organization, workspace, semantic domain, asset, environment, source, release, and consumer.
- Roles: workspace admin, security admin, semantic steward, asset owner, reviewer, publisher, operator, consumer, and auditor.
- Actions: read, discover, propose, edit, validate, review, publish, rollback, bind, execute, manage source, manage policy, manage identity, and read audit.
- Constraints: explicit deny, least privilege, immutable audit, token expiry and revocation, no self-approval for protected releases, and server-side command authorization.
- Inspection: every denied or allowed decision exposes principal, action, resource, matched role/policy version, and reason code to authorized auditors.

Attribute-aware conditions should follow for cases such as environment, asset classification, business domain, purpose of use, request channel, release risk, ownership, working hours, consumer identity, and source sensitivity. These conditions belong in a versioned policy engine with simulation and explainability. Warehouse row-level and column-level security should remain enforced by the execution system; Semlia should preserve and propagate security context instead of becoming a second data-plane security engine.

## Recommended sequence

1. Close the current prototype: add identity, roles, permissions, API clients/scopes, policy decision explanation, and separation-of-duties flows; restore a clean desktop and compact-desktop end-to-end suite.
2. Build the real RBAC authorization kernel and command matrix before broadening write APIs.
3. Add versioned conditional policies for environment, asset classification, ownership, purpose, and consumer context; include policy simulation and shadow evaluation.
4. Add source-policy propagation and execution-adapter security-context contracts for query planning.
5. Prove open-source readiness with self-hosted OIDC, example tenants, connector conformance, upgrade/backup exercises, security testing, and scale benchmarks.

## Evidence limits

- Screenshots confirm visible hierarchy and current interaction surfaces, not full WCAG 2.2 compliance.
- The audit did not verify screen-reader output, 200% zoom, complete focus restoration, high-contrast mode, or every error and empty state.
- Mock behavior does not prove concurrency, multi-tenant isolation, policy correctness, data-source safety, release atomicity, or production query enforcement.
- The audit evaluates the current workspace, including uncommitted work present during capture.
