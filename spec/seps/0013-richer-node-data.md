---
sep: 0013
title: Richer node data for extraction — raw_text and interactive state
author: Joel Webber (@joelgwebber)
status: Accepted
created: 2026-09-08
updated: 2026-09-10
spec-version-target: 1
related-issues: [443]  # interactive-state attrs follow-up (the one unshipped surface)
related-discussions: []
---

## Summary

Make two kinds of node data deterministically available to extraction. Add a
`raw_text` extract mode — the node's own literal text, distinct from its
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
  name can legitimately weld in extra text, so even the correct name is not always
  the literal value wanted.

  A live example makes it concrete. A JetBlue fare-tile heading is `<h3>Main</h3>`
  with `::after { content: "Most popular" }`. That injected text is user-visible
  but absent from `textContent`, so the accessibility name is "Main Most popular"
  while the author-written text is just "Main". An offline consumer reads the CDP
  accessibility tree and gets "Main Most popular"; a runtime consumer that
  approximates the name from the DOM gets "Main". Same `extract: text`, two values,
  neither wrong — they answer *different questions*.

  There are really **three** text notions, not equally computable everywhere:

  | notion | value here | offline (CDP + AX tree) | runtime (DOM only) |
  |---|---|---|---|
  | DOM text (`textContent`) | "Main" | yes | yes |
  | rendered visible text (DOM + `::before`/`::after`) | "Main Most popular" | needs pseudo capture | yes (`getComputedStyle`) |
  | accessibility name (aria + alt + content + pseudo) | "Main Most popular" | yes | not from any DOM API — must reimplement accname |

  The runtime is the **constrained consumer**: it has no accessibility tree, so a
  `text` pinned to "the accessibility name" is satisfiable only if the runtime
  computes accname itself. Any pin must therefore be an *algorithm over the DOM*
  that both consumers can run — not "whatever the capture's AX tree says."

- **State is unreachable.** A control's interactive state — checked, selected,
  disabled, expanded — is a native property, not an attribute, so no `extract`
  form can read it, even though "is this toggle on?" is exactly the signal a
  consumer wants.

## Proposal

### `text` — the pinned accessible name

`text` stays the default and keeps its meaning — the node's **accessibility
name** — but is now **pinned to the accname computation run over the DOM** (the
standard algorithm: `aria-labelledby` / `aria-label` / native labels / `alt` /
name-from-content, the last of which includes CSS `::before`/`::after`), not
"whatever the capture's AX tree returns." Both consumers MUST compute the same
value:

- Offline this matches the CDP accessibility name (Chrome implements accname).
- The runtime MUST compute accname over the DOM — crucially **including pseudo
  content** — rather than approximating it with `innerText` (which silently drops
  `::after`, the divergence above).

`text` is the right choice when the accessibility name *is* the value — e.g. an
icon button whose only label is `aria-label`.

> Faithful runtime accname is the substantive, deferrable half of this SEP: the
> mode and its meaning are fixed here, but an SDK MAY ship an approximation first
> and tighten it toward accname parity, tracked separately. `raw_text` (below)
> lands independently and does not wait on it.

### `raw_text` — the node's own literal text

Add `raw_text` to the extract grammar: the concatenation of the node's **direct
text-node children**, whitespace-normalized. It is pinned to this
`textContent`-style data — explicitly **not** `innerText` (layout-dependent and
historically under-specified) — and it excludes:

- **descendant element text** — it is the node's *own* text, not a subtree
  concatenation, so no `<style>`/`<script>` bleed and no swallowing of nested
  labels; and
- **CSS pseudo content** — `::before`/`::after` are not child nodes.

```yaml
- name: SubFare                        # a JetBlue fare tile
  selector: '.cb-fare-tile__section'
  properties:
    - name: tier
      extract: raw_text               # "Main" — not the AX name "Main Most popular"
```

`raw_text` is the deterministic escape when the accessible name is polluted, or
when you specifically want the author-written text. `text` and `raw_text` are the
two *ends* — everything a human perceives, versus the literal author text.
Addressing a *single* injected fragment (just the `::after`, or one of a
`::before`/`::after` pair) is a **non-goal**: pseudo/badge content is
presentational and i18n-varying, so a corpus that must key on it should reach for
a narrower selector, not a text primitive.

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
  computation on every consumer, live or offline: `text` = accname over the DOM
  (pseudo-inclusive), `raw_text` = the node's own direct-text-node content.
- **Omission (unchanged).** An empty `raw_text`, or a state attribute the node
  does not carry (it has no such state), omits the property.

### Conformance

A conforming SDK:

- MUST compute `raw_text` as the node's own direct-text-node content,
  whitespace-normalized, identically live and offline.
- MUST compute `text` as the accessibility name over the DOM, including CSS
  `::before`/`::after` content, identically live and offline. (An SDK MAY ship a
  partial accname and tighten it over time; the value's *definition* is fixed.)
- MUST carry `checked`, `selected`, `disabled`, and `expanded` on a node that has
  that state, readable via `attr=`.
- MUST NOT expose a control's current value as a carried state attribute; it
  remains the accessibility value / reserved `value` property.

## Implementation status

This SEP is accepted with one of its three surfaces implemented; the other two
are accepted as direction but land later, so the reference implementation (`go/`)
and the normative spec ([`spec/v1/schema.md`](../v1/schema.md)) currently reflect
only what has shipped:

- **`raw_text` — shipped.** The probe computes it from the node's direct
  text-node children, the matcher resolves `extract: raw_text`, validation accepts
  it, and both the JSON Schema and `schema.md`'s extract grammar document it.
- **Faithful runtime accname (`text` pin) — pending.** The runtime still
  approximates the accessible name (the AX tree's name offline, an `innerText`
  fallback) rather than computing pseudo-inclusive accname over the DOM. This is
  the deferrable half called out under [`text`](#text--the-pinned-accessible-name)
  above: the *definition* is fixed here, an SDK MAY tighten toward it over time.
- **Interactive-state attributes — pending.** `checked` / `selected` / `disabled`
  / `expanded` are **not yet carried as `attr=`-readable state**. Capture surfaces
  them as accessibility *properties*, but they are not exposed on the node's
  observed attribute set, so `extract: attr=checked` cannot yet read a
  native-property state. Until it lands, `schema.md` intentionally does **not**
  require the state set — keeping the normative spec and the reference
  implementation in agreement — and the MUST under *Conformance* above is the
  accepted target, not yet a shipped guarantee.

This section is removed once the interactive-state work lands and `schema.md`
gains the state requirement.

## Open questions

- **The state set** — is `checked`/`selected`/`disabled`/`expanded` the right
  minimal set, or do we also carry `pressed`/`readonly`/`required`?
- **Accname parity scope** — how close must a runtime's DOM accname come to the
  full spec algorithm to conform (which steps are MUST vs SHOULD), and is that a
  gate for this SEP or a follow-up? (`raw_text` lands independently of it.)

## References

- **SEP-0010** — the extraction model and its "richer text" open question; the
  carried-attribute set this widens; the reserved `value` property this preserves.
