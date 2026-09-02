# Ontology Prototype Calibration Evidence

## Scope

The semantic asset detail remains a quiet, desktop-first operational surface. This calibration makes ontology semantics explicit without turning the product into a free-form graph editor.

Design contract:

- Separate concept taxonomy, semantic relations, and dependency/impact into a stable segmented control.
- Keep the graph bounded to one hop and keep the structured relation list authoritative.
- Show assertion state, semantic address, evidence, release, RelationType endpoint constraints, inverse, cardinality, reasoning behavior, and validation result.
- Show the asset revision and ontology revision independently, with a compact adjacent-revision delta.
- Route relationship changes into the governed knowledge revision workbench.
- Preserve the existing neutral canvas, compact typography, 7px maximum card radius, keyboard access, visible state, and 1440x900 / 1024x768 product viewports.

## Screenshots

| File | State |
| --- | --- |
| `01-semantic-1440x900.png` | Metric semantic-relation view at normal desktop |
| `02-dependency-1024x768.png` | Metric dependency-and-impact view at compact desktop |
| `03-taxonomy-1440x900.png` | Business-concept taxonomy with broader, narrower, and disjoint relations |
| `04-relations-and-constraints-1440x900.png` | Auditable relation list and RelationType contracts |
| `05-constraints-1024x768.png` | Relation and constraint density at compact desktop |

## Validation

- `pnpm --filter @semlia/product-prototype test`
- `pnpm --filter @semlia/product-prototype lint`
- `pnpm --filter @semlia/product-prototype build`
- `pnpm --filter @semlia/product-prototype test:e2e`
- Playwright screenshots inspected at `1440x900` and `1024x768` with reduced motion.

The changed surface has no horizontal clipping in the ontology context, graph, relation list, or constraint table at either supported viewport.
