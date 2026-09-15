# FMB-T006 Desktop QA

Status: PASS for the inspected local desktop scope.

Targets: 1440x900 and 1024x768. No mobile scope is inferred.

The real Docker journey creates an isolated workspace, explicit machine identity and
grant, consumer and binding, then issues, rotates and revokes actual credentials.
It verifies old-token401/new-token200, persistent revoked state after refresh, absence
of full secrets from lists/closed dialogs, and focus restoration after immediate Escape.
Credential controls remain server-owned during refresh.

The same journey creates a webhook subscription, edits endpoint/event selection,
disables it, rotates its signing secret, reloads and verifies persisted version4/signing
version2 and disabled state. The workspace produces no asset/release events; this UI
journey makes no public delivery and honestly shows no delivery records. Actual signed
delivery is proven separately by the PostgreSQL/HTTP receiver test under its explicit
local network policy.

Root CUA inspection confirms the connected view has no obsolete prototype banner;
checkboxes are 16px and left aligned; forms fit compact desktop; Shift+Tab wraps within
the modal and Escape dismisses an unsubmitted form and returns focus. Automated real
and deferred-refresh tests prove immediate secret dismissal, delayed focus restoration
and no focus stealing after a user selects another control. Final desktop inspection
sees zero secret dialogs, zero enabled test
subscriptions and no horizontal overflow. The viewport override is reset afterwards.

Root visually inspected the saved desktop and compact-desktop credential/webhook
screenshots. Vertical scrolling is intentional on compact desktop. No incoherent text
overlap is observed. The real journey's integration-region Axe scan reports zero
serious/critical findings. The complete 26-case fixture regression preserves role
inspection, assignment conflict, independent review/publish and keyboard checks.

Representative evidence:

- `screenshots/desktop-final-review.png`
- `screenshots/desktop-persisted-revoked-clients.png`
- `screenshots/compact-desktop-persisted-revoked-clients.png`
- `screenshots/desktop-persisted-disabled-webhook.png`
- `screenshots/compact-desktop-persisted-disabled-webhook.png`
- `screenshots/compact-desktop-credential-form.png`

Credential response bodies and secret dialogs are excluded from retained browser
traces/screenshots. Failed live pages dismiss/redact before diagnostic capture; issued
test credentials are revoked and subscriptions disabled. Fixture regression screenshots
use the sprint-specific directory rather than overwriting prior evidence.
