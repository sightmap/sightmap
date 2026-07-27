# 012-id-class-field-attrs

Attribute selectors on `id` and `class` must resolve to the dedicated
`SelectorPart` fields (`Id`, `Classes`), not only to `attrs`. Extraction stores a
node's id in `selector.id` and its classes in `selector.classes` — including SVG
elements, whose `className` is an `SVGAnimatedString` rather than a plain string —
so a matcher that only consulted `attrs` would silently see zero matches for
these selectors even though the live browser matches them.

`IssueRow` (`[id^="issue_"]`) matches `row1` (id `issue_9f1c`) and not `row2`
(id `cycle_42`), confirming prefix matching against the `id` field. `LucideIcon`
(`[class*="lucide"]`) matches the `svg` node `icon1`, whose classes live in
`selector.classes`, confirming substring matching against the `classes` field.
