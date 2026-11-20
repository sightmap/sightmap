---
"@sightmap/sightmap": patch
---

Selectors the CLI writes for an element's id now escape punctuation instead of emitting `"#" + id`. An id such as `product.1` comes out as `#product\.1` rather than `#product.1` (a parse error) or, for `checkout.submit`, `#checkout.submit` (id `checkout` plus class `submit`, which matches nothing). A leading digit gets a hex escape, as `CSS.escape` writes it: `1.circle` comes out as `#\31 \.circle`. This covers coverage hook candidates, `suggest`, the `sel-probe` and `explain` element lines, the selector hints in `snapshot` and `inspect` output, and the selector the browser's click and scroll fallbacks pass to `querySelector`. The new `sightmap.IDSelector` builds these selectors for library consumers.
