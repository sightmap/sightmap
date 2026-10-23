---
"@sightmap/sightmap": minor
---

The most specific definition names an element (SEP-0023): when several component definitions match one element, the one whose selector is strictly more specific than each of the others names it, whatever the declaration order, so `span.label` beats `span`, `[data-id$="-total-price"]` beats `[data-id$="-price"]` and a child's chain beats its parent's, without a `:not()`. Selectors are ordered by what they require first and what they exclude second, decided from the selector text alone (`sightmap.ParsedSelector.MoreSpecific`), so every implementation orders the same pairs; definitions it cannot order keep the first-declared tie-break and are still reported as conflicts. `Match`, `Conflicts` and `NamesForChain` all apply the rule.
