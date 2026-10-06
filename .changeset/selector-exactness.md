---
"@sightmap/sightmap": patch
---

Selectors match exactly what a browser matches. A repeated attribute name (`[class*="a"][class*="b"]`) now requires every test instead of keeping only the last. The parser rejects compounds it used to loosen: a second `#id`, a type or `*` after other tokens (`[x]div`), and a second `:is()`/`:where()`. Matching compares the values of the attributes HTML treats case-insensitively (`type`, `rel`, `target`, `lang` and the rest of that list) ASCII case-insensitively on HTML elements but not on SVG or MathML ones, so `input[type=password]` matches `type="PASSWORD"`; folds type names with ASCII rules only; and never matches a `~=` value that is empty or contains whitespace.
