---
sep: 0011
title: Component property pattern capture
author: Joel Webber (@joelgwebber)
status: Draft
created: 2026-09-08
updated: 2026-09-08
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add an optional `pattern` to a component property: an RE2 regular expression
whose first capture group becomes the property's value. This is the same
`pattern` mechanism SEP-0005/0006 already define for request and message
properties, extended to components — one capture mechanism across every property
type.[^1]

## Motivation

A component `extract` yields a whole string, but the value a consumer wants is
often a fragment of it: a label reads `"12 results"` when the consumer wants
`12`; a price reads `"$24.90"` when a comparator wants `24.90`. Today the only
recourse is to hope a child element isolates the fragment — which frequently does
not exist. Requests and messages already solve exactly this with `pattern`;
components have no equivalent.

## Proposal

### Shape

```yaml
- name: SearchSummary
  selector: '[data-testid="search-summary"]'
  properties:
    - name: count
      extract: ResultLabel.text      # "12 results"
      pattern: '(\d+)'               # -> "12"
    - name: price
      extract: Price.text            # "$24.90"
      pattern: '([\d.,]+)'           # -> "24.90"
```

`pattern` is optional. When present it is an RE2 expression applied to the string
the `extract` directive resolves to; the property's value is the content of the
**first capture group**. It applies to any `extract` form that yields a string
(`text`, `raw_text`, `attr=NAME`); it is not valid on `exists:` (a boolean flag).

### JSON Schema

- `$defs.componentProperty`: add an optional `pattern` (string), mirroring
  `$defs.requestProperty.pattern`.

### Semantics

- **Tree-closed / offline (unchanged).** `pattern` is a pure function of the
  already-resolved string, so it introduces no live-DOM or platform dependency —
  it composes on top of SEP-0010 extraction without touching the invariant.
- **First capture group only.** A `pattern` with no capture group is invalid
  corpus input. A pattern with more than one capture group uses the first; named
  and multiple groups are a forward-compatible extension (see Open questions).
- **Silent omission (unchanged).** If the pattern does not match, or the
  `extract` resolved to nothing, the property is omitted — never an error.

### Conformance

A conforming SDK:

- MUST, when `pattern` is present, apply the RE2 match to the resolved string and
  yield the first capture group, omitting the property on no-match.
- MUST reject a `pattern` with no capture group as invalid corpus input.
- MUST reject `pattern` on an `exists:` extract as invalid.

## Open questions

- **Named / multiple capture groups.** A multi-group `pattern` could feed a
  list-valued property (see the multi-match SEP) or a small map. Deferred; the
  single-first-group form specified here is a forward-compatible subset.

## References

- **SEP-0005 / SEP-0006** — the request/message `pattern` mechanism this reuses.
- **SEP-0010** — the tree-closed extraction model `pattern` composes onto.

[^1]: `pattern` is one general RE2 capture, deliberately *not* a fixed vocabulary
of named string transforms — the mechanism SEP-0010 kept for requests, not the
`transform` field it removed.
