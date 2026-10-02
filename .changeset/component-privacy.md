---
"@sightmap/sightmap": minor
---

Add `privacy` to component entries (SEP-0009).

`privacy: block | mask | unmask` declares whether a capture consumer may retain the matched element's content. It applies to the element **and its subtree**, and the nearest enclosing declaration wins, so the common shape is a `mask` on a form with an `unmask` on the one field inside it that is safe to keep.

Resolution deliberately differs from `tags`, which union across every applicable definition. A union is right for classification, where more labels are additive; it is wrong for a directive, where two applicable values are a contradiction that has to be decided rather than merged.

It governs extracted properties as well as captured content, at the node each value is read from rather than at the component that declared it. Extraction is tree-closed, so a `PATH.prop` reaching a blocked descendant is withheld even when the declaring component is unrestricted. `exists:` survives `mask` because presence is structure, as do the four interactive-state attributes SEP-0013 defines (`checked`, `selected`, `disabled`, `expanded`) — a closed set carrying state rather than a value, without which a masked form could report no control state at all.

`mask` also covers attribute values in the captured recording, not only text and input values. A consumer must withhold every `data-*` attribute, `value`/`title`/`alt`/`placeholder`/`aria-label`, and any attribute the corpus names in an `attr=` extract anywhere — naming an attribute in an extract is itself the author declaring it carries content, which keeps the rule from going stale. Presentational attributes (`class`, `style`, `id`) may be retained because replay needs them to render, and the SEP states plainly that user data in one of those survives a `mask` and should use `block` instead. A withheld property is absent, indistinguishable from one that did not resolve.

Omission declares nothing: a component with no `privacy` leaves the consumer's own default untouched, so a corpus can adopt the field one component at a time. A consumer may withhold more than the corpus asks, but never less.

The loader carries the authored value onto `ComponentDef.Privacy` verbatim and does not inherit it to children, matching `tags`/`source`/`memory`. Resolution is the consumer's job, over the flattened chain.
