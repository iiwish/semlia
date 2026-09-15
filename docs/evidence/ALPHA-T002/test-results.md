# T002 Test Results

Validation date: 2026-09-04

| Gate | Result | Evidence |
| --- | --- | --- |
| SDK typecheck | Pass | Included in `make check-source` typecheck phase |
| Web unit and integration components | Pass | 7 files, 62 tests |
| Web lint | Pass | ESLint completed with no findings |
| Production Web build | Pass | Vite transformed 1,822 modules and emitted the production bundle |
| Auth/member Playwright | Pass | 6 tests across desktop and compact-desktop |
| Existing product Playwright | Pass | 26 tests across desktop and compact-desktop |
| Scoped axe checks | Pass | Sign-in and real member administration reported zero violations at both viewports |
| HTTP callback tests | Pass | Browser redirect and JSON error-contract cases |
| Full source gate | Pass | format, lint, typecheck, tests, contract drift, migration drift, Web embed drift and release build |
| Diff whitespace check | Pass | `git diff --check` produced no findings |

The full `make check-source` run completed all eight phases. Its test phase took 208 seconds; the
other phases completed in 0-6 seconds each.

## Visual Evidence

- `screenshots/desktop-sign-in.png`
- `screenshots/compact-desktop-sign-in.png`
- `screenshots/desktop-member-admin.png`
- `screenshots/compact-desktop-member-admin.png`

Visual inspection confirmed a nonblank, correctly framed sign-in state and a complete member
administration workspace with the primary rail, secondary menu, workspace selector and account
control visible at both supported viewports.
