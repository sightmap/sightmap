---
"@sightmap/sightmap": patch
---

`snapshot` now surfaces runtime match conflicts in a `[Conflicts]` section — the ambiguities that only exist against a live page, complementing the static corpus-conflict warnings `validate` emits. Two triggers:

- **A single DOM node matched by more than one distinct component name.** Matching is first-match-wins, so only one applied and the others were silently dropped — which is also why a correct-looking component can report `0 matches`. The section names the node, the competing components, and which one won.
- **Two or more views matching the current URL at equal specificity**, where declaration order alone decided the winner.

Adds `match.FindConflicts` and `Corpus.TiedViews`; both are computed during `observe.Page`. (The view-tie half is provisional pending the route-specificity decision in the functional-decomposition proposal.)
