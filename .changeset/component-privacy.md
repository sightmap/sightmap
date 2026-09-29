---
"@sightmap/sightmap": minor
---

Add `privacy` to component entries (SEP-0009, Draft).

`privacy: block | mask | unmask` declares whether a capture consumer may retain the matched element's content. It applies to the element **and its subtree**, and the nearest enclosing declaration wins, so the common shape is a `mask` on a form with an `unmask` on the one field inside it that is safe to keep.

Resolution deliberately differs from `tags`, which union across every applicable definition. A union is right for classification, where more labels are additive; it is wrong for a directive, where two applicable values are a contradiction that has to be decided rather than merged.

It governs extracted properties as well as captured content, at the node each value is read from rather than at the component that declared it. Extraction is tree-closed, so a `PATH.prop` reaching a blocked descendant is withheld even when the declaring component is unrestricted. `exists:` is the exception, surviving `mask` because presence is structure. A withheld property is absent, indistinguishable from one that did not resolve.

Omission declares nothing: a component with no `privacy` leaves the consumer's own default untouched, so a corpus can adopt the field one component at a time. A consumer may withhold more than the corpus asks, but never less.

The loader carries the authored value onto `ComponentDef.Privacy` verbatim and does not inherit it to children, matching `tags`/`source`/`memory`. Resolution is the consumer's job, over the flattened chain.
