# M2 T011 Test Results

## Final Validation

| Command | Result | Evidence |
| --- | --- | --- |
| `make check-source` | Pass | Format, lint, typecheck, Go tests, Web tests, SDK checks, contract drift, generated DB/API artifacts, embedded Web drift, and build passed. |
| `pnpm --filter @semlia/web exec vitest run --maxWorkers=1` | Pass | 55 Web unit/component tests, including the local-acceptance complete-navigation lock. |
| `pnpm --filter @semlia/web test:e2e` | Pass | 26/26 fixture-contract journeys passed across 1440x900 and 1024x768. |
| `pnpm --filter @semlia/web test:e2e:live` | Pass | 2/2 real-stack journeys passed across 1440x900 and 1024x768. Review, publish, and rollback each asserted the exact HTTP 201 response; actor state, persistence, alerts, and horizontal overflow were checked after reload. |
| `make check-smoke` | Pass | Full Compose smoke journey passed, including migrations, embedded Web/API readiness, worker persistence, database interruption/recovery, and restart persistence. |
| `make security-check` | Pass | Go and pnpm dependency findings: 0; secret findings: 0; release-image Debian and Go-binary vulnerabilities: 0. Reports are under ignored `build/security/`. |
| `make release` | Pass | Darwin arm64 archive, CycloneDX SBOM, and SHA256 checksums were generated under ignored `build/release/`. |
| `git diff --check` | Pass | No whitespace errors. |
| `curl -fsS http://127.0.0.1:18081/health/ready` | Pass | Running stack returned `status: ready`. |

## Behavioral Locks

- Missing `X-Semlia-Principal` is rejected with `403 NO_MATCHING_GRANT`; explicit authorized
  principals succeed.
- Local aliases resolve only to the deterministic principals seeded for that workspace. An older
  ordinary reviewer or publisher with the same role cannot be impersonated by an alias.
- Author, reviewer, and publisher are independent persisted principals. Author-review and
  reviewer-publish violations remain server-side refusals.
- The review-list endpoint returns immutable persisted review facts after actor changes and reloads.
- Real transport catalog paths are normalized and content digests are canonical SHA-256 values.
- Local acceptance keeps all five primary modules and platform settings visible; identity selection
  does not redefine product navigation.
- Review, publish, and rollback browser steps wait for their exact successful network responses;
  unrelated visible labels cannot satisfy the assertions.

## Residual Gates

Alpha T007 supplies current security, cross-target release, source, Ask, resolver, migration and
performance evidence. Local configured-provider runs do not constitute a production provider
account, and local identity aliases are deliberately not authentication. T011 therefore remains
`Running` until a hosted external OIDC run and a founder-approved live provider run are attached.
