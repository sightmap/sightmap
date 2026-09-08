---
"@sightmap/sightmap": minor
---

Offline matcher: support `:not(:has(...))` and a combinator inside `:not(...)`, for parity with the live DOM.

The offline component-tree matcher (used by `snapshot`, `coverage`, `validate`, `sel-check`, and the overlay) now honors two standard-CSS relational selectors it previously mishandled:

- **`:not(:has(...))`** and **`:is(...:has(...))`** — a relational pseudo nested inside `:not()`/`:is()` is now evaluated with the subject's subtree instead of being silently dropped. This unblocks the leaf-scoping pattern `X:not(:has(X))` (e.g. `jb-form-field-container:not(:has(jb-form-field-container))`), which previously matched **zero** nodes offline while resolving live.
- **A descendant/child combinator inside `:not()`** (e.g. `button:not(nav button)`) now parses and is evaluated against the subject's ancestor chain. Such selectors previously threw a parse error, so a corpus using valid CSS failed to `validate` even though it resolved live.

`:not()` now accepts a complex-selector list, matching CSS Selectors Level 4. Sibling combinators (`+`, `~`) remain unsupported and are still rejected loudly at parse time.

Note: `SelectorPart.Not` changes from `*SelectorPart` to `[]ParsedSelector` to represent the complex-selector-list argument.
