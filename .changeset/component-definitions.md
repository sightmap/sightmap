---
"@sightmap/sightmap": minor
---

File-root `definitions:` (SEP-0019, draft): shared components that are `$ref` targets but never matched on their own. A view gets a definition only where it references one, scoped by the reference, so a structure whose selectors are only safe in context (a card's `img`, `h3`, `button`) can be shared across views without becoming a global that claims every page. The loader, `validate`, `lint`, `stats` and the unknown-field check all know the key; a global wins a name clash (`definition-shadowed-by-global`), and a duplicate definition warns (`merge-collision-definition`).

Also fixes `$ref` resolution for duplicate global names: the loader let the **last** same-name global (by source-file path) win, contradicting the spec's first-seen rule (and the `merge-collision-component` warning's own message). It now resolves to the first. Corpora with same-name globals that differ in selector may expand `$ref` differently; `validate` already warns on them.
