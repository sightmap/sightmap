---
"@sightmap/sightmap": patch
---

Selectors match exactly what a browser matches. Every test in a compound now applies instead of only the last: a repeated attribute name (`[class*="a"][class*="b"]`), a repeated `#id` (`#a#a` matches, `#a#b` matches nothing), and a second `:is()`/`:where()` (`:is(.a):is(.b)`). The parser rejects a type selector or `*` after other tokens (`[x]div`), which CSS does not allow. Matching compares the values of the attributes HTML lists as case-insensitive (`type`, `rel`, `target`, `lang` and the rest) ASCII case-insensitively, so `input[type=password]` matches `type="PASSWORD"`; in an HTML document that list applies to every element, SVG and MathML included. Type names fold with ASCII rules only, and a `~=` value that is empty or contains whitespace never matches.
