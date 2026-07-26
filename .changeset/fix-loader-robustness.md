---
"@sightmap/sightmap": patch
---

Make corpus loading robust against two malformed inputs that previously failed badly:

- A circular `$ref` chain (`A → B → A`, or a component that references itself) sent every corpus-loading command into infinite recursion and hung the process. Loading now detects the cycle, stops expanding, and `validate` reports it as a `ref-circular` error instead of hanging.
- `splitSelectors` only balanced parentheses, so a comma inside an attribute selector or quoted string (`[data-x="a,b"]`) was wrongly treated as a selector-list separator and split into two dead alternatives. Splitting is now aware of `[]` brackets and quoted strings (with backslash escapes), matching how CSS is actually written.
