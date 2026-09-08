---
"@sightmap/sightmap": minor
---

Add the `raw_text` extract mode (SEP-0013, first half).

`extract: raw_text` returns a node's **own literal text** — the concatenation of its direct text-node children, whitespace-normalized — never its accessibility name. It is pinned to `textContent`-style data, **not** the layout-dependent `innerText`, and it excludes both descendant element text (no `<style>`/`<script>` bleed, no subtree concatenation) and CSS pseudo content (`::before`/`::after`).

It is the deterministic escape for when the accessible name welds in extra text: a heading whose AX name appends a CSS `::after` badge resolves `text` to "Main Most popular" but `raw_text` to "Main", so an exact property match (`SubFare[tier="Main"]`) works. `text` stays the default (and the right choice when the accessible name *is* the value); an empty `raw_text` omits the property, like the other extract forms.

(The other half of SEP-0013 — pinning `text` to a faithful, cross-consumer accessible-name computation, and carrying interactive-state attributes — is deferred.)
