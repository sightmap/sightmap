---
"@sightmap/sightmap": minor
---

Authoring efficacy on hook-poor DOMs (Salesforce Lightning, Angular, framework-generated markup):

- **Generalized selector-candidate generation.** `gap` and `suggest` no longer dead-end when a DOM has no `data-testid`/`data-component`. Candidate generation now ranks custom-element tags, design-system classes, ids, other stable `data-*`, and `name`/`href`/`aria` hooks (data-attributes remain the top-ranked input, not an override), dropping only clearly machine-generated tokens. `gap` also emits a container hook on the hook-poor path.
- **New `explain` command.** Node-first inspection: pick nodes by selector, `--id`, or `--grep` (role/name) — live or offline via `--snap` — and dump each node's facts, ranked selector candidates, coverage tier + owning component, and ancestor hooks. Shadow-transparent (matches the offline matcher), so authors no longer hand-read `*.snap.tree.json`.
- **Honest coverage.** `snapshot --coverage` and `gap` now warn when a clean "0 orphaned" pass is carried entirely by global (chrome) components — i.e. the current view modeled nothing and the pass reflects a global backstop rather than a real view.
- **Authoring skill:** a second mandatory property rule requires a per-instance discriminator on any component whose selector matches more than one instance, so repeated cards/rows/tabs stay individually addressable; plus `explain` documentation.
