# V1-T004-A003 Browser Gate Isolation

## Scope

The standard `make check-browser` gate uses independent native and production-acceptance environments. This attempt changes only the configuration and resource-ownership boundaries of that gate, its focused tests, and the existing smoke active-project repository contract. Product behavior, dependency versions, browser assertions and acceptance thresholds are unchanged.

An explicit `SEMLIA_NATIVE_ENV_FILE` is a required independent configuration file. Its child processes receive tool-only environment values and that file's settings, not ordinary `dev.env`, `native.env`, caller model credentials, execution mappings or LAN configuration. The file cannot redirect the reserved configuration/state controls. The supervisor receives the same explicit controls and isolated environment. Without an explicit file, the ordinary developer configuration merge and caller environment remain compatible.

Native validation creates a fresh private directory, marker and nested state directory. It checks their device/inode/mode identity and the returned container ID plus owner label before cleanup. Unknown identity prevents native shutdown and Docker removal. A verified owned PostgreSQL container is removed together with its own anonymous volumes; native shutdown failures remain gate failures. Evidence writes and screenshots require the original trusted evidence root, and existing evidence files are not overwritten.

Production acceptance pins its local Unix Docker endpoint, private anonymous Docker configuration, explicit Compose file, private environment file and generated project. Preflight and teardown inspect both project-labelled resources and the exact configured volume/network names. Generation failures and invalid owner/password/port values stop before creation. Process cleanup checks that the recorded application PID remains a child of the launcher. Protected markers and resource ownership are rechecked before teardown, followed by an absence check.

Raw browser diagnostics can contain credential-bearing locator call logs. Native exceptions are retained only in a verified private evidence directory; production Playwright output is redirected to its private `browser.log`. Console diagnostics are fixed summaries. Generated directories use mode 0700 and private files use mode 0600.

## RED And GREEN

All launcher isolation tests execute the actual repository launcher with disposable fake Docker, Go, native-process and browser fixtures. They do not contact a real Docker daemon, database, model provider or browser. Synthetic values are used throughout.

| Check | Actual Result |
| --- | --- |
| Native configuration baseline | `node --test scripts/dev/native-environment.test.mjs`: valid behavioral RED with two failures, showing inherited canary configuration and fallback when the explicit file was missing. The ordinary default-merge case passed. |
| Reserved native controls | A separate RED showed that configuration-file control keys could override the caller's file/state. Rejecting these reserved keys made the actual child-environment regression pass. |
| Production launcher baseline | `node --test scripts/dev/production-acceptance-isolation.test.mjs`: four failures and one pass. Failures demonstrated inherited private inputs, same-name unlabelled resource adoption, marker drift cleanup and masked generation failure. Existing partial-start cleanup passed. |
| Native state replacement | A real launcher-fixture RED showed `native down` was called after the state directory was replaced by a symlink. Original path identity checks prevent both shutdown and Docker removal. |
| Evidence-root replacement | Two reviewer counterexamples were retained: cleanup overwrote a foreign sentinel after root replacement, and an exception printed a synthetic credential. Both were reproduced as RED before the evidence-write and diagnostic fixes. Regressions cover replacement after native startup and during browser close. |
| Production diagnostic privacy | A RED showed credential-bearing Playwright output reached the console. The same failing browser fixture is nonzero while its raw diagnostics remain private and owned cleanup completes. |
| Combined focused GREEN | `node --test scripts/dev/native-environment.test.mjs scripts/dev/validate-native-isolation.test.mjs scripts/dev/production-acceptance-isolation.test.mjs`: 16/16 passed, exit 0, 48.231 seconds. |
| Independent focused review | The same 16 tests passed independently in 47.087 seconds. Recursive review of configuration, subprocess and cleanup paths found no remaining blocker; independent repository/public-contract tests also passed. |
| Active-project repository contract | `go test -count=1 -run TestSmokeJourneyUsesActiveComposeProject ./tests/repository`: RED for two obsolete environment-fallback fragments. The updated contract requires the verified private smoke receipt, its exact active project and ownership guard, and rejects ordinary development fallback. |
| Repository and public contracts | Go 1.26.6 with `GOENV=off GOWORK=off GOFLAGS= GOTOOLCHAIN=local`, `go test -count=1 ./tests/repository ./tests/contracts`: exit 0, 5.172 seconds and 1.620 seconds respectively. |
| Syntax and whitespace | `bash -n scripts/dev/production-acceptance.sh`, `node --check` for both native entry points, and `git diff --check`: exit 0. |
| Real browser gate | Following separate root approval, `make check-browser` exited 0 in 115.778 seconds with Go 1.26.6 and a clean environment. Native password/member/password-change checks passed; the original production desktop and compact-desktop suite passed all 12 tests, with no filtering or retries. |

Initial fixture setup failures caused by macOS temporary-path canonicalization and an accidentally thenable browser stub were corrected. Those fixture failures are not counted as implementation RED evidence.

## Runtime Status

The first approved real run is recorded under `.semlia/v1-acceptance/current/browser-gate-UZmzfT`. The command, toolchain, raw output and before/after receipts are protected by directory mode 0700 and file mode 0600. A snapshot-driver quoting error was corrected before the gate started; no Docker command or filesystem read/write was executed by that invalid snapshot command. The real gate itself ran once and passed.

The native receipt reports successful password login, member creation and suspension, password change, rejection of the old password, desktop and compact-desktop layouts, and cookie attributes, with zero page errors. Cleanup reports `owned: true` and `cleanupFailed: false`. The fresh native supervisor ownership file and control socket are absent after shutdown, and no container carries its random owner label.

The production suite reports 12 passed and zero failed in 1.3 minutes. Its project-labelled containers, volume and network are absent; exact `${project}_postgres-data` and `${project}_default` names are also absent. The final complete inventory matches all 18 original containers, 72 original volumes and 22 original networks, including removal of the native test's anonymous PostgreSQL volume. External observation occurred after the short-lived resources were cleaned up, so it is not presented as an independent observation of live resource labels; the runtime ownership guards, cleanup receipt and complete before/after inventory establish the cleanup result.

All eight preservation checks passed: original container, volume and network inventories; configuration hashes, sizes, owners, modes, mtime and ctime; the original V1 supervisor; source-mode schema-33 runtime identity; the existing security-image tag; and absence of owned resources. Ordinary `dev.env`, `native.env`, default native ownership state, V1 runtime receipts, Compose inputs and embedded index were included in the fingerprint comparison where present. No real model-provider call, original database migration, candidate publication or remote action was performed.

All command sessions ended and the browser/Docker window was released. Source files are frozen for T005.

The earlier T004 smoke failure and subsequent successful owned smoke run remain in `release-gates.md`; this attempt does not rewrite that evidence. The orchestrator resumes T005 after technical acceptance of this attempt.
