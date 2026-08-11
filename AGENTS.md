# Semlia Codex Project Instructions

## Supported Product Surface

- Semlia is a desktop Web application for high-density semantic governance work.
- Treat `1024px` as the minimum supported product viewport. Optimize normal desktop at `1440x900` and compact desktop at `1024x768`.
- Do not add mobile navigation, touch-only interactions, mobile-specific layouts, mobile screenshots, or mobile acceptance tests unless the user explicitly reopens mobile scope.
- Existing narrow-screen fallback CSS is compatibility code, not a product requirement and must not drive feature or design decisions.

## Visual Direction

- Use a quiet operational workspace with a plain neutral canvas.
- Prefer individual cards for repeated metrics, sources, channels, bindings, events, and decision objects instead of long ruled sections.
- Do not use grid-paper, ruled-line, or decorative linear-gradient backgrounds in the main workspace.
- Keep tables and true data grids structured; use dividers only where row comparison requires them.
- Avoid cards nested inside cards. When repeated objects become cards, keep their containing section unframed.

## Validation

- Frontend visual QA targets desktop and compact-desktop viewports.
- Preserve keyboard access, visible focus, reduced motion, and mock-data disclosure.
