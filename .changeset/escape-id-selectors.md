---
"@sightmap/sightmap": patch
---

Selectors the CLI writes for an element's id now escape punctuation instead of emitting `"#" + id`. An id such as `product.1` comes out as `#product\.1` rather than `#product.1` (a parse error) or, for `checkout.submit`, `#checkout.submit` (id `checkout` plus class `submit`, which matches nothing). An id the portable selector profile cannot write after `#`, such as one starting with a digit, comes out as `[id="1.circle"]`. This covers coverage hook candidates, `suggest`, the `sel-probe` and `explain` element lines, the selector hints in `snapshot` and `inspect` output, and the selector the browser's click and scroll fallbacks pass to `querySelector`. The new `sightmap.IDSelector` and `sightmap.QuoteSelectorString` build these selectors for library consumers.
