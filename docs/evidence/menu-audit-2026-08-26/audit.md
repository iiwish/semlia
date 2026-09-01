# Semlia Menu Audit

Date: 2026-08-26

Scope: primary activity rail, contextual navigation for Ask, Workbench, Knowledge Assets and Data Ingestion, and build-run cross-module destinations at 1024x768.

## Verdict

The primary information architecture is clear and should not be reordered again. The remaining work is about navigation semantics, discoverability and accessibility rather than menu-tree structure.

## Steps

1. Primary navigation: clear order and selected state. The bottom-up trust stack is present without forcing a wizard flow.
2. Knowledge Assets: clear abstraction order: Semantic Assets, Ontology and Relations, Physical Data Graph, Knowledge Blocks and Evidence.
3. Data Ingestion: clear separation between Sources, Build Tasks and Build Runs. Build-run details expose explicit destinations for physical graph, knowledge evidence and governance proposals.
4. Workbench: needs refinement. "Recent Activity" currently scrolls to the runtime topology section, so the label and destination do not match. Workbench anchors also share the same visual grammar as true secondary pages.

## Priority Findings

1. Rename "Recent Activity" to "Runtime Topology" or implement a real activity feed. The current target is "From data sources to Q&A applications".
2. Distinguish contextual page links, object lists and in-page anchors. Ask sessions, Workbench anchors and module pages currently use the same row component.
3. Move "New conversation" out of the recent-session list or style it as a command. It duplicates the top-bar action and currently looks like a saved session.
4. Add visible hover and keyboard-focus tooltips to the icon-only primary rail. `aria-label` is present, but sighted users still depend on delayed browser-native title hints.
5. Increase contextual metadata contrast. Section labels and secondary row text render at approximately 2.82:1 on the panel background at 8-10px; target at least 4.5:1 for normal text.

## Evidence

- `01-primary-navigation.png`
- `02-knowledge-assets-navigation.png`
- `03-data-ingestion-navigation.png`
- `04-build-run-destinations.png`
- `05-workbench-context.png`
- `06-workbench-tasks-anchor.png`

## Limits

This audit confirms rendered structure, target mapping, selection state, overflow and computed contrast at the compact-desktop viewport. It does not claim full WCAG compliance or cover screen-reader behavior across browsers.
