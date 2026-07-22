---
sep: 0000
title: <Short, descriptive title>
author: <Your Name> (@github-handle)
status: Draft
created: YYYY-MM-DD  # replace with today's date in YYYY-MM-DD form
updated: YYYY-MM-DD  # replace with today's date; bump on substantive edits
spec-version-target: 1
related-issues:
  - https://github.com/sightmap/sightmap/issues/...
related-discussions:
  - https://github.com/sightmap/sightmap/discussions/...
---

> Copy this file to `seps/NNNN-short-slug.md`, where `NNNN` is the next unused integer (4 digits). Update the front-matter and fill in the sections below. Delete this blockquote.

## Summary

One paragraph explaining what this SEP proposes. Treat this as the abstract — a maintainer triaging a backlog of SEPs should be able to read just this and decide whether to keep reading.

## Motivation

What user-facing problem does this solve? Who hits it today? What can they not do, or what do they have to work around?

Concrete scenarios beat hypotheticals. If you can name a real app or pattern that breaks under the current spec, do.

## Proposal

### Shape

Show the proposed shape concretely. All applicable subsections are required:

- **YAML** — before/after blocks showing what `.sightmap/*.yaml` looks like under this SEP.
- **JSON Schema diff** (required when the SEP touches `spec/v1/sightmap.schema.json`) — a prose enumeration of the `$defs` entries added, removed, or modified, with required/optional shifts and any `oneOf`/union introductions. The diff prose lets an SDK author implement the schema change from the SEP alone, without reverse-engineering the full JSON. Example: *"`$defs.view`: remove `source: {type:string}`; add `sources: {type:array, items:{type:string}}` (optional)."*
- **Match inputs and expected results** (route-matching changes only) — show URLs and the views/requests they should match under the new rules.

```yaml
# Before
version: 1
…

# After
version: 1
…
```

### Semantics

Walk through how this behaves in practice. What does an SDK do when it sees this? What does an agent see at runtime?

### Conformance

What MUST a conforming SDK do under this proposal? What MAY it do? What MUST it not do?

## Alternatives considered

At least two. For each:

- What it is
- Why you ruled it out

"Do nothing" is always a valid alternative; address why the status quo is insufficient.

## Migration

If this lands, what changes for existing users?

- Existing sightmaps that look like X must be rewritten as Y.
- Existing SDKs that did X now must do Y.
- Tooling we ship (validator, schema, examples) needs these changes: …

If this is a breaking change, propose a deprecation window and tell the story of how a careful user upgrades without breaking their pipeline.

## Open questions

Things you genuinely haven't decided yet. Reviewers can help here.

## References

- Prior art in other specs (MCP, ARIA, GraphQL, etc.)
- Related SEPs
- Issues / discussions / blog posts that influenced this
