# Member Entry Contrast

## Scope And Cause

The member administration surface retains its existing colors, density, layout, keyboard controls and 180 ms entry motion. Its entry animation uses translation only, so readable text remains fully opaque throughout entry. Other surfaces retain their existing animation.

The historical compact-desktop failure reported `.member-state-active` at 4.31:1 with composited colors `#228070` / `#e9f6f3`. The badge's declared colors are `#08715f` / `#e7f6f2`. Computed-style inspection of the badge and every ancestor identifies the member section's inherited `pageEnter` opacity as the cause, not a competing badge color rule. At 60 ms of the actual 180 ms animation, its opacity is `0.858149`; all other inspected ancestors have opacity `1`. Axe then reports badge contrast 4.06:1 and additional faded text failures in the same surface.

## Red And Green

- `auth-baseline.log`: unchanged six-test auth suite, exit 0, 6 passed in 14.2 s. This timing-dependent pass does not invalidate the preserved historical failure.
- `auth-entry-red.log`: existing member test samples actual browser animations at one-third duration, pauses them, captures computed styles, and runs the unchanged full-surface Axe assertion. Exit 1, 4 passed / 2 failed in 19.9 s. Both desktop sizes reproduce contrast failures.
- `auth-entry-green.log`: same strengthened six-test suite after the scoped CSS fix, exit 0, 6 passed in 19.1 s. Badge and all ancestors have opacity `1` at the sampled entry time. The member section retains a 0.18 s animation. Auditor reload also verifies reduced-motion duration at or below 0.01 ms. No blind delay or Axe exclusion is introduced.

Commands run from the repository root, substituting the corresponding `baseline`, `red`, or `green` evidence directories:

```sh
SEMLIA_BROWSER_EVIDENCE_DIR="$PWD/docs/evidence/FMB-T007/attempts/a002/overtime/green-screenshots" pnpm --filter @semlia/web exec playwright test --config playwright.auth.config.ts --workers=1 --output="$PWD/docs/evidence/FMB-T007/attempts/a002/overtime/green-results" --trace=on
```

Baseline and RED omit `--trace=on`, retaining failure traces through the existing configuration. GREEN retains all traces, including the `member-entry-computed-styles` JSON attachment. Evidence and screenshots use separate paths; no earlier A002 artifacts are overwritten.

## Inspection And Handoff

`green-screenshots/desktop-member-admin.png` (1440x900) and `green-screenshots/compact-desktop-member-admin.png` (1024x768) were visually inspected. Member controls and status remain visible without new overlap. Existing no-clipping assertions pass. The auth suite proves local route-fixture session/capability behavior, login keyboard focus, invitation CSRF and logout; it is not hosted OIDC evidence.

Only `web/src/styles.css` and `web/e2e-auth/session-membership.spec.ts` are changed for this correction. Source is frozen after GREEN. No worker test sessions remain active; no Docker, Go, production build or release commands were run for this correction. Root owns independent final source, security, release and preview validation. Hosted validation is deferred under the user's local-only closeout direction.
