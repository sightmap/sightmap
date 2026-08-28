---
"@sightmap/sightmap": patch
---

Port the DevTools extension's component-property extraction to the SEP-0010 tree-closed model. The embedded overlay (`resolver.js` and its inlined `content.js` copy) now resolves the four extract forms — `text`, `attr=NAME`, `PATH.prop`, `exists:PATH` — over the matched component tree, replacing the removed DOM-shaped modes (`inner_text`/`text_only`/`inner_html`/raw CSS sub-selector) and transforms. Descendant paths resolve a child component's own extracted property (`text` is the element's DOM text content, the extension's implementation-defined accessible text).
