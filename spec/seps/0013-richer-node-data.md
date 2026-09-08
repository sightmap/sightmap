---
sep: 0013
title: Richer node data for extraction — raw_text and interactive state
author: Joel Webber (@joelgwebber)
status: Draft
created: 2026-09-08
updated: 2026-09-08
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Make two kinds of node data deterministically available to extraction. Add a
`raw_text` extract mode — the node's raw text content, distinct from its
accessibility name — and **pin** both `text` and `raw_text` so every consumer
computes them identically. And mandate that a small set of **interactive-state**
attributes (`checked`, `selected`, `disabled`, `expanded`) be carried on the node,
readable through the existing `attr=` form. Closes SEP-0010's *richer text* open
question and the gap where a control's state was unreachable.

## Motivation

Two gaps, both about what the abstract node exposes:

- **Text is ambiguous, so consumers disagree.** SEP-0010 defines `text` as the
  node's accessibility name and leaves it "implementation-defined." So the *same*
  `extract: text` yields the computed accessibility name in one consumer and the
  raw visible text in another — an on-/off-line divergence. And an accessibility
  name can legitimately weld in extra text (an `aria-labelledby` that appends a
  role phrase), so even the correct name is not always the literal value wanted.
- **State is unreachable.** A control's interactive state — checked, selected,
  disabled, expanded — is a native property, not an attribute, so no `extract`
  form can read it, even though "is this toggle on?" is exactly the signal a
  consumer wants.

## Proposal

### `raw_text`, and pinned text

Add `raw_text` to the extract grammar: the node's **raw text content** (its
literal visible text), distinct from `text` (its accessibility name). Both are
**pinned** — every conforming consumer MUST compute `text` as the accessibility
name and `raw_text` as the raw text content by the same rules — so a value never
depends on which tool read it.

```yaml
- name: LanguageSelect
  selector: '[data-testid="language-select"]'
  properties:
    - name: choice
      extract: raw_text        # "English" — not the AX name "Language: English"
```

`text` stays the default and the right choice when the accessibility name *is* the
value; `raw_text` is the deterministic escape when the name is polluted, or when
you want the literal visible text.

### Interactive-state attributes

SEP-0010 has the node carry "a fixed allowlist plus `aria-*`/`data-*`" as
attributes. That carried set MUST include a node's interactive state where
present: `checked`, `selected`, `disabled`, `expanded`. They are read with the
existing `attr=` form — no new grammar.

```yaml
- name: SubscribeToggle
  selector: '[role="switch"]'
  properties:
    - name: on
      extract: attr=checked    # "true" / "false"
```

Native platforms carry their own state under these names, so the tree stays
platform-neutral. The reserved **`value`** property is unchanged and is **not**
duplicated here: a control's current value stays the node's accessibility value
(overridable by a declared `value:`), never a carried `checked`-style attribute.

### JSON Schema

- `$defs.componentProperty.extract`: add `raw_text` as a valid form.
- No schema change for state — it widens the carried-attribute *set* (prose +
  conformance), which `attr=NAME` already reads.

### Semantics

- **Tree-closed / offline (unchanged).** Both `raw_text` and the state attributes
  are carried on the abstract node at capture; extraction reads them offline.
- **`text` / `raw_text` are pinned**, not implementation-defined — the same
  computation on every consumer, live or offline.
- **Omission (unchanged).** An empty `raw_text`, or a state attribute the node
  does not carry (it has no such state), omits the property.

### Conformance

A conforming SDK:

- MUST compute `text` (accessibility name) and `raw_text` (raw text content) by
  the pinned rules, identically live and offline.
- MUST carry `checked`, `selected`, `disabled`, and `expanded` on a node that has
  that state, readable via `attr=`.
- MUST NOT expose a control's current value as a carried state attribute; it
  remains the accessibility value / reserved `value` property.

## Open questions

- **The precise `raw_text` rule** — whitespace normalization, and whether it is the
  node's own text or its subtree's — so "pinned" is well-defined.
- **The state set** — is `checked`/`selected`/`disabled`/`expanded` the right
  minimal set, or do we also carry `pressed`/`readonly`/`required`?

## References

- **SEP-0010** — the extraction model and its "richer text" open question; the
  carried-attribute set this widens; the reserved `value` property this preserves.
