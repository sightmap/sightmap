---
"@sightmap/sightmap": minor
---

Add `watch` to component entries (SEP-0015, Draft).

`watch: true` asks a capture consumer to report a component's visibility lifecycle — that it rendered, that it became visible, that it went away — rather than reporting it only when someone interacts with it. The components whose appearance is the signal (an empty state, an error, a promotion) are usually the ones nobody clicks, so an interaction-shaped record contains no trace of them.

`watch` applies to the component it is declared on and never to its `children`, the opposite of SEP-0009 `privacy`. Privacy is a restriction, where covering the subtree is the safe default; `watch` generates records, where covering a subtree silently would multiply them.

A consumer's interactivity heuristic must not suppress a watched component. Without that rule the field does nothing for exactly the non-interactive components that motivate it.

The loader carries the authored value onto `ComponentDef.Watch`. `watch: false` is identical to omitting the field.
