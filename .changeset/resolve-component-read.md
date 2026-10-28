---
"@sightmap/sightmap": minor
---

Resolve a component property against the corpus's definitions, for consumers that compile properties ahead of time instead of matching a component tree. `Corpus.ResolveComponentRead(owner, extract)` follows a `from: component` or `component.exists` PATH (and any chained reads) to the definition the value is read from. It returns that target, its selectors relative to the declaring component, the `dom.*` read to perform there, the patterns to apply innermost first, and whether the read is multi-valued and its `join`. A PATH segment that names no descendant, or several, is reported (`component-path-unresolved`, `component-path-ambiguous`), as is a missing property or a cycle. A multi-valued read must write every PATH step as `Name[]`: a plain step takes the first match per branch (`Row[].Item.text` reads one Item per Row), which one selector list cannot express, so such reads are reported as `component-read-unsupported`. `ValidateComponentExtract` checks a single component extract with the same codes `Validate` reports.

Component definitions now carry `LocalSelectors`, the component's own selector alternatives as authored before ancestors' are prefixed. The corpus wire includes them as `localSelectors` on nested components, and a reader handles a wire without them.
