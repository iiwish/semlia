# V1-T004 Local Release Gates

## Scope

Attempt A001 covers the disposable Docker smoke gate, all nested Go Docker commands, self-host Compose migration ordering and a configurable security-scan image identity. It does not start, stop, migrate or restore the V1 acceptance instance, the original schema-32 database or unrelated services. Final embedded frontend synchronization and candidate packaging belong to T005.

The smoke script bypasses `scripts/dev/compose.sh` and never reads or generates `.semlia/dev.env`. Compose inputs are the two explicit repository files and a temporary ownership-label-only override. Random credentials, project/image identities and loopback ports are generated for each invocation. The Dockerfile builds the frontend internally; smoke does not run `make web-embed` or modify shared embedded assets.

## Isolation Contract

- Docker, Compose, Node helpers and Go smoke tests receive an allowlisted environment, not caller `COMPOSE_*`, private execution/model credentials or Docker host/TLS overrides.
- The normal local Docker context is read only to obtain its endpoint. Only Unix-socket endpoints are accepted. Subsequent commands pin that endpoint and use temporary Docker/Buildx configuration.
- The temporary Docker configuration imports no credentials, proxies or contexts. It has standard CLI plugin discovery and one empty anonymous Docker Hub auth entry. An empty auth map would allow Docker CLI 29.4.0 to discover the native credential store; the nonempty anonymous map prevents that fallback. See the official [config loader](https://github.com/docker/cli/blob/v29.4.0/cli/config/config.go#L154) and [ContainsAuth implementation](https://github.com/docker/cli/blob/v29.4.0/cli/config/configfile/file.go#L110).
- Go runs with `GOENV=off`, `GOWORK=off` and empty `GOFLAGS`, so persisted Go configuration cannot silently narrow or redirect the gate.
- The launcher hands Go a mode-0600 context receipt in its mode-0700 temporary directory. Go checks the current user's ownership, exact paths, local endpoints, random project/image identity and SHA-256 fingerprints of temporary environment, ownership overlay and Docker configuration. A missing, changed or mismatched receipt fails before Docker invocation; ordinary `dev.env` and project/HTTP fallback are forbidden.
- Every nested Compose helper, worker inspection and PostgreSQL failure-cleanup command uses one factory with the fixed endpoint/configuration and an allowlisted environment. Every nested mutation verifies project-label and exact-name resource inventories plus the image owner. Compose context overrides, including attached short flags, are rejected.
- Preflight refuses an existing project resource, configured volume/network name or image tag. Cleanup checks both project-label enumeration and exact configured names, verifies random owner/project labels on every resource and image, and refuses all destructive cleanup on unknown ownership. This is necessary because Compose can reuse and remove a configured volume by name rather than only by project-label discovery. See official [volume creation](https://github.com/docker/compose/blob/v5.1.2/pkg/compose/create.go#L1495) and [volume teardown](https://github.com/docker/compose/blob/v5.1.2/pkg/compose/down.go#L133).
- After owned cleanup, absence is checked again. Failed ownership/absence checks retain the protected temporary directory for inspection; there is no broad Docker prune or cleanup of unrelated resources.
- Self-host server and worker both require `migrate: service_completed_successfully`.
- `SEMLIA_SECURITY_IMAGE` selects the same image for `make build-image` and the existing security scanner. The default remains `semlia:security`; T005 can use a fresh task-owned tag without replacing the existing tag. Scan severities and failure behavior are unchanged.

## TDD Evidence

CLI-isolation tests use disposable filesystem fixtures and fake Docker commands. The launcher propagation cases execute the real Go helpers through the shell launcher, not a fake Go success response. The Compose contract tests invoke only `docker compose config --format json`; they do not contact or mutate a running stack. All canary credentials are synthetic.

| Round | Command | Result |
| --- | --- | --- |
| Initial RED | `node --test scripts/ci/check-smoke.test.mjs deploy/examples/compose.test.mjs` | Exit 1; 4 failures prove missing migration ordering, inherited private environment, acceptance of a remote endpoint and unverified ownership cleanup. One existing failure-cleanup case passed. |
| Hidden-name RED | `node --test --test-name-pattern='configured volume' scripts/ci/check-smoke.test.mjs` | Exit 1; both hidden existing volume and post-start ownership drift allowed unsafe cleanup. |
| Focused GREEN | `node --test scripts/ci/check-smoke.test.mjs deploy/examples/compose.test.mjs` | Exit 0; 9 tests passed, including both hidden-name regressions, owner/port generation failures, Go configuration isolation and owned failure cleanup. |
| Final focused GREEN | `node --test scripts/ci/check-smoke.test.mjs deploy/examples/compose.test.mjs` | Exit 0; 12 tests passed in 24.32 seconds, including exact configured network/image refusal and standard Compose inventory consistency. |
| Shell syntax | `bash -n scripts/ci/check-smoke.sh` | Exit 0. |
| Nested-helper RED | `GOENV=off GOWORK=off GOFLAGS= go test -count=1 -run 'TestSmokeIsolation\|TestComposeCommandUsesDirectDockerCLI' ./tests/smoke` | Exit 1; three behavior failures show Docker executes without a context and nested Compose still selects ordinary `dev.env` instead of the fixed isolated inputs. |
| Nested-helper GREEN | `GOENV=off GOWORK=off GOFLAGS= go test -count=1 ./tests/smoke` | Exit 0; final author run 47.075 seconds, independent run 47.198 seconds. Receipt validation, all mutating helpers and failure cleanup reject foreign ownership; original worker recovery/watchdog assertions are retained. Runtime journey is skipped without explicit opt-in. |
| End-to-end fixture GREEN | `node --test scripts/ci/check-smoke.test.mjs deploy/examples/compose.test.mjs` | Exit 0; 14 tests, 35.00 seconds. Real Go helpers inherit the launcher's exact identity through ps/restart/stop/start/exec/down/up/inspect; removing the receipt fails before any nested Docker command. Both fixtures retain verified shell cleanup. |
| Security-image RED | `node --test scripts/ci/security-image.test.mjs` | Exit 1; environment and Make-variable overrides still built the default image, while the default compatibility case passed. No Docker daemon was used. |
| Security-image GREEN | Same command | Exit 0; 3 tests, 3.07 seconds. Default, environment override and Make-variable override all build and scan the same image; HIGH/CRITICAL and exit-code-1 remain enforced. |
| Independent combined GREEN | `node --test scripts/ci/check-smoke.test.mjs deploy/examples/compose.test.mjs scripts/ci/security-image.test.mjs` | Exit 0; 17 tests, 37.779 seconds. Independent code review found no remaining blocker before the second real run. |
| Real Docker GREEN | `make check-smoke` | Exit 0; 93 seconds total, Go smoke package 69.920 seconds. Fresh owned build/install, all five ordered runtime stages and verified cleanup succeeded. |

## Runtime Status

The first approved `make check-smoke` run exited 2. The image build, isolated installation and embedded Web/API stage passed. Stage 02 failed because nested Go helpers still invoked the development Compose wrapper and selected its ordinary project. Stages 03 worker restart, 04 PostgreSQL fault/recovery and 05 shutdown persistence did not run. The wrapper could execute `chmod 600` against ordinary `dev.env`; this round therefore does not claim zero metadata touches. This failure was retained, diagnosed and fixed before renewed approval for the second run; there was no automatic retry-until-pass loop.

The first run's protected receipt is `.semlia/v1-acceptance/current/release-smoke-wUIe8c` (directory 0700, files 0600). Its before/after allowlisted inventory verified all 18 original containers, 72 volumes and 22 networks unchanged; native V1 supervisor identity was unchanged. Configuration content hashes, byte counts and modes, including ordinary `dev.env`, V1 runtime receipts, standard Compose inputs and embedded index, were unchanged. The disposable project's containers, exact named volumes/network and image were absent after owner-verified cleanup. No broad prune occurred. The Docker window was released, and this failed round remains part of the evidence.

The second approved run is recorded under `.semlia/v1-acceptance/current/release-smoke-zcjuBC`, also protected by 0700/0600 modes. `make check-smoke` exited 0 with `SEMLIA_RUN_SMOKE=1`, an explicit isolation receipt and no Go test narrowing. The ordered runtime journey passed embedded Web/API, required processes, worker restart/database-job persistence, PostgreSQL stop/degraded readiness/recovery with the same container, and shutdown/volume preservation/restart. The final shell cleanup removed the task-owned resources.

An independent runtime inventory observed the four smoke containers, one network, three volumes and image with matching random owner/project labels. The post-run check proved project-label and exact-name resource absence, including the disposable image. Before/after snapshots were equal for all 18 original containers, 72 volumes and 22 networks. Configuration SHA-256, size, uid, mode, mtime and ctime were unchanged; the V1 source-mode schema-33 identity and native supervisor were unchanged; the existing `semlia:security` image ID was unchanged. All command sessions ended and the Docker window was released.

Public registry pulls must succeed anonymously; failure is reported rather than borrowing unrelated credentials. Port allocation is checked but remains subject to the normal bind race; a conflicting listener causes the gate to fail rather than terminating another process. Docker build cache and shared public base-image cache are not pruned.
