# Integration Settings Design Contract

Operational desktop surface for administrators and semantic consumers. The view uses the existing neutral canvas, 12px control text, 14px body text, 16px section headings, existing focus rings and icon buttons. Minimum viewport is 1024px; acceptance targets are 1440x900 and 1024x768.

Channels are factual endpoint references, not simulated connection-health toggles. Credentials and subscriptions are individual repeated objects in unframed sections, with compact status, expiry/version and explicit rotate/revoke controls. Dialogs separate identity creation, consumer registration, credential issuance and webhook subscription. Browser state never fabricates persistence or stores secrets outside the currently open one-time dialog.

Loading, empty, denied, stale, pending, command failure, persisted command plus refresh failure and clipboard failure are distinct. A stale authorization signal clears secrets and disables commands. Dialog focus is trapped, Escape dismisses, and focus returns to the invoking control. No animation or new image assets are needed for this dense administrative surface. Existing typography, color tokens, borders and reduced-motion behavior are retained.
