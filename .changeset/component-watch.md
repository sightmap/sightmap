---
"@sightmap/sightmap": minor
---

Add `watch` to component entries (SEP-0015, Draft).

`watch: true` asks a capture consumer to report that a component **became visible to the user**, rather than reporting it only when someone interacts with it. The surrounding lifecycle (rendered, removed) is optional: becoming visible is the only moment that answers the question the field exists for, and the rest is largely churn, since an element can render far off-screen, re-render on every state change, and be removed by a route transition. Repeat reports of one appearance should be collapsed, and a visibility report is passive — never an interaction. The components whose appearance is the signal (an empty state, an error, a promotion) are usually the ones nobody clicks, so an interaction-shaped record contains no trace of them.

`watch` applies to the component it is declared on and never to its `children`, the opposite of SEP-0009 `privacy`. Privacy is a restriction, where covering the subtree is the safe default; `watch` generates records, where covering a subtree silently would multiply them.

A consumer's interactivity heuristic must not suppress a watched component. Without that rule the field does nothing for exactly the non-interactive components that motivate it.

The loader carries the authored value onto `ComponentDef.Watch`. `watch: false` is identical to omitting the field.
