# Knowledge Catalog UX Audit

## Scope

- Surface: knowledge catalog, governed-object filtering, relation detail, and return context.
- Viewport: 1440x900 desktop.
- User goal: find a governed knowledge object, understand its relationship or implementation context, and return without losing the search state.

## Verdict

Using one catalog surface is the right information-architecture direction. Treating semantic assets, semantic relations, PhysicalBinding, and JoinContract as completely equal flat rows is not the best default browsing model. The stronger design is a single asset-centric catalog: semantic assets are primary rows; governed relations and implementations remain searchable and addressable but appear as expandable child objects or targeted search matches under their owning asset.

## Flow

1. Knowledge catalog: healthy foundation. Search, filters, status, ownership, and recent assets are discoverable in one workspace.
2. Relation filter: functionally clear, but it exposes that one generic table is comparing heterogeneous object contracts and lifecycle states.
3. Relation detail: context is partially lost because opening a relation lands on the owning asset's relation tab without selecting or emphasizing the relation that was clicked.
4. Return to catalog: filters and scroll position are retained, but the previously opened row is not highlighted.

## Highest-impact recommendations

1. Keep one catalog page, but make it asset-centric rather than completely flat.
2. Use semantic assets as primary rows and reveal relation, binding, and JoinContract children inline; search may auto-expand a parent when a child matches.
3. Split the type filter into grouped semantics: semantic asset types and governed implementation types, without introducing page-level modes.
4. When a child object is opened, anchor and highlight that exact relation or binding in the asset detail.
5. Make relation lists the default decision surface and keep the graph as an on-demand path or impact-analysis tool.
6. Normalize status presentation: asset lifecycle and implementation validation are separate dimensions and should not be compared as one generic status.
7. Add a recent-return highlight so users can immediately locate the row they inspected.

## Accessibility and evidence limits

- The captured DOM exposes search, filters, rows, tabs, and back navigation with semantic roles and accessible names.
- Secondary text is visually small and low contrast; contrast and zoom resilience require measured verification.
- Screenshots do not establish full keyboard order, screen-reader announcements, reduced-motion behavior, or WCAG conformance.

## Evidence

- `01-knowledge-catalog.png`
- `02-relation-filter.png`
- `03-relation-detail.png`
- `04-return-context.png`
- `05-implemented-asset-centric-catalog.png`
