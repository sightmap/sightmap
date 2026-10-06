---
sep: 0018
title: A portable selector profile and fast evaluation of privacy and watch
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-10-06
updated: 2026-10-07
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Privacy ([SEP-0009](0009-component-privacy.md)) and watch ([SEP-0015](0015-component-watch.md))
resolve from every component whose selector matches an element. Two consumers make that hard in
practice: a recording client that applies selectors with the visitor's own browser engine, and a
consumer that resolves an element when the user interacts with it, from the element and its
ancestors, without the rest of the page.

This SEP makes both reliable and cheap:

- A **selector profile**, `capture-baseline`, names the selectors every engine a recording client
  still has to support implements identically. Newer selectors become an explicit opt-in for a later
  profile.
- Every selector in the profile depends only on an element and its ancestors, so a consumer can
  resolve privacy and watch from the ancestor path alone, exactly, in a few microseconds.
- A rule the profile rejects, or one that matches nearly every element, never matches loosely.
- A consumer's own privacy rules join the corpus's, resolved the same way.

## Motivation

**A privacy rule has to apply in every browser.** A recording client classifies elements with
`Element.matches`. `:is()`, `:where()` and `:not()` with a list need engines from 2020 or later,
`:has()` from 2022 or later, the attribute `i` flag from 2016. A privacy selector an old engine
rejects either fails open or forces the client to withhold everything.

**Resolving at interaction time has to be cheap.** A consumer that records a click resolves the
target's privacy and watch while the page runs, or later from the target and its ancestors. Most
selectors depend only on that path. A few do not:

```html
<section class="payment">
  <p class="saved-card">Visa ending 4242</p>   ← clicked
  <input name="cardnumber">
</section>
```

```yaml
- name: PaymentSection
  selector: 'section:has(input[name="cardnumber"])'
  privacy: block
```

The note is inside the blocked section, but only the section's other children say so. A consumer
holding the page can evaluate this correctly; it costs a search of the section's subtree on every
interaction. A consumer holding only `section › p` cannot evaluate it at all. Sibling combinators
and positional pseudo-classes have the same shape. A client that decides an element's privacy once,
when the element is inserted, also has to re-evaluate these whenever the siblings or descendants they
depend on change.

**A rule that cannot be evaluated must not match loosely.** Today a privacy selector the matcher
cannot parse silently matches nothing, so its `block` is lost; and a privacy rule on `*` or
`:not(.x)` applies to nearly the whole page, where an `unmask` reopens everything.

## Proposal

### Shape

No new fields. The change is to which selectors validate, and how a rule that does not is resolved.

### The `capture-baseline` profile

A selector profile names the selector features a corpus may use. Every corpus uses
`capture-baseline`: the CSS3 selectors that every engine in its baseline implements identically and
that depend only on an element and its ancestors.

| Engine | Minimum version |
|---|---|
| Chrome | 38 |
| Firefox | 29 |
| Safari | 9 |
| Edge | 80 |

Grammar (whitespace is U+0020 only, outside quoted strings):

```
selector    := compound ( combinator compound ){0,7}
combinator  := " "+ | " "* ">" " "*
compound    := "*"
             | type simple*
             | simple+
type        := [a-z] [a-z0-9-]*
simple      := "#" ident | "." ident | attribute | negation
attribute   := "[" name ( op value )? "]"
name        := [a-z_] [a-z0-9_-]*
op          := "=" | "~=" | "|=" | "^=" | "$=" | "*="
value       := ident | string
negation    := ":not(" ( type | "#" ident | "." ident | attribute ) ")"
```

`ident` and `string` follow CSS, except that hex escapes are not allowed; a single escaped printable
ASCII character (`\:`, `\.`, `\/`) is. In addition:

- A selector is at most 8 compounds and 1,024 bytes, with no leading or trailing combinator and no
  comma. A component lists alternatives as separate selectors; the loader already splits a top-level
  comma into separate selectors.
- A compound has at most one `#id`. An attribute may be tested more than once; every test applies.
- A type selector comes first in its compound, and `*` is a compound on its own.
- Only `=` may compare against an empty value, and a `~=` value contains no whitespace.
- On the `class` attribute, only `[class]`, `[class~=v]` and `[class*=v]` are allowed; anything else
  is written with `.class`.
- Type, attribute and pseudo-class names are written in lowercase.

Outside the profile:

| Construct | Why |
|---|---|
| `:is()`, `:where()`, `:not()` with a list or compound, attribute `i`/`s` flags | Not supported by every baseline engine |
| `:has()` | Not supported by every baseline engine, and depends on descendants |
| `+`, `~`, `:first-child`, `:nth-child()` and the other structural pseudo-classes, `:empty`, `:root` | CSS3 and portable, but depend on elements other than the target and its ancestors |
| `:hover`, `:focus`, `:checked` and other state pseudo-classes | Depend on live state a captured page does not carry |
| Pseudo-elements, namespaces | Do not select elements, or not portably |

**Future profiles (to do).** A later SEP may define profiles that admit newer selectors, such as one
for CSS Selectors Level 4. A corpus would opt in explicitly through the corpus-root key
`selectorProfile`, which this SEP reserves. Choosing such a profile is the author's acceptance that
older engines cannot apply those selectors, and that consumers resolving at interaction time pay the
cost of selectors that look beyond the ancestor path.

### Rules the profile rejects

A privacy rule whose selector is outside the profile never matches loosely:

- a `block` or `mask` applies to the whole document, the conservative reading of a rule that cannot
  be evaluated;
- an `unmask` is ignored.

A rule's *subject* is its last compound. A privacy rule whose subject has no `#id`, class or
attribute outside `:not()` (`*`, `body *`, `.card > *`, `:not(.x)`) is resolved the same way: an
`unmask` would reopen the page and a `block` would close it.

A watch rule outside the profile is not reported.

### Consumer rules

A consumer MAY supply privacy rules of its own, such as built-in defaults or rules configured
outside the corpus. Consistent with SEP-0009's floor, they may only withhold: a consumer `block` or
`mask` joins the corpus rules and resolves by SEP-0009's rules; a consumer `unmask` is ignored. They
are validated against the same profile.

### Resolving from an ancestor path

Every selector in the profile depends only on an element and its ancestors. A consumer that keeps an
element's ancestor path therefore resolves the element's privacy and watch exactly as a consumer
holding the whole page would. It needs each element's tag, id, classes, and every attribute any
privacy or watch rule names; a reference implementation reports that attribute set.

A rule that applies to most of the page, such as a `mask` on a form, is found on one of the target's
ancestors, so no consumer needs to scan the page.

**In a browser (non-normative).** A client can resolve one element without a page scan: walk from the
element toward its document root; at each ancestor, test the subjects of the `block`, `mask` and
`unmask` rules with one native `matches()` call per kind; on a hit, verify the rest of the rule right
to left along the same path. Stop at the first `block`. Otherwise keep the nearest `mask` or
`unmask` and continue only to look for a `block` above it. Matching each compound natively and
walking combinators manually keeps shadow-root crossing consistent with the full-page model and
avoids `Element.closest`, which the baseline's oldest engines lack. The cost is at most three native
calls per ancestor.

### Diagnostics

Applied to every selector a component matches with, after flattening, so a parent's selector is
checked with its child's. A message on a child names the parent the problem came from.

| Code | On a component declaring `privacy` or `watch` | On any other component |
|---|---|---|
| `selector-not-in-profile` | error | warning for the deprecation window, then error |
| `selector-universal-subject` | error | warning |
| `privacy-unmask-broad` (an `unmask` on a bare type selector, such as `body`) | warning | — |

During the deprecation window a selector outside the profile on a component without `privacy` or
`watch` keeps matching for naming. The window closes in the next minor release after this SEP
reaches Final.

### JSON Schema

No structural change.

### Conformance

A conforming SDK:

- MUST report a selector outside the profile, or a universal subject, with the severities in
  [Diagnostics](#diagnostics).
- MUST resolve a privacy rule outside the profile, or with a universal subject, as covering the whole
  document when it is `block` or `mask`, and ignore it when it is `unmask`; and MUST NOT report a watch
  rule outside the profile.
- MUST ignore a consumer `unmask` rule.
- MUST give the same privacy and watch for an element whether it evaluates the whole page or only the
  element's ancestor path.

New conformance fixture: `032-selector-profile` (validation and diagnostics).

## Evidence

The reference implementation: #495 (`ParseProfileSelector`, the profile grammar with coded
rejections), #496 (validation, fixture `032-selector-profile`), and a PR stacked on them with the
out-of-profile handling, consumer rules and ancestor-path resolution (`PrivacyForChain`,
`WatchedForChain`, `PrivacyAttributes`).

**Ancestor path equals the whole page.** For every node of 80 seeded pages, each with a random
corpus in which about half the components declare privacy and a quarter declare watch (some of both
outside the profile), the privacy and watch resolved from the node's ancestor path alone, those
resolved over the whole page, and an independent oracle agree: 24,000 nodes.

**Cost per interaction.** Apple M5 Pro, Go 1.26.7, resolving one element's privacy from its ancestor
path:

| Ancestor depth | 10 rules | 100 rules | 1,000 rules |
|---|---|---|---|
| 8 | 0.52 µs | 0.50 µs | 1.96 µs |
| 32 | 1.74 µs | 1.74 µs | 6.76 µs |
| 128 | 7.22 µs | 8.37 µs | 46.0 µs |

Allocations stay between 11 and 36 per call.

**Existing corpora.** None of the 37 corpora in this repository gains a diagnostic. One real corpus
tests one attribute twice (`[class*="..."][class*="..."]`) to target generated class names, which is
why the profile keeps every test rather than rejecting the repeat.

Reproduce from `go/`:

```sh
go test ./match/ -run '^$' -bench 'PrivacyForChain'
go test ./sightmap/ -run '^$' -fuzz FuzzParseProfileSelector -fuzztime 60s
```

## Alternatives considered

1. **Restrict only components that declare `privacy` or `watch`.** Rejected: turning on privacy for an
   existing component would then require rewriting its selector.
2. **Keep the full grammar and make interaction-time consumers fail closed.** A consumer that cannot
   evaluate `:has()` would withhold every element the rule might cover. Rejected: with no way to bound
   "might", the conservative answer covers the whole page.
3. **Native-engine semantics as the definition.** Define matching as whatever `Element.matches`
   returns. Rejected: engines disagree on newer features, and a consumer without a live page has no
   engine to ask.

## Migration

- Components using selectors outside the profile see warnings for the deprecation window, errors on
  components declaring `privacy` or `watch`. `:is()` and `:where()` rewrite mechanically into separate
  selectors; `:has()` and sibling selectors need an attribute or class on the element itself.
- A privacy rule outside the profile resolves conservatively until fixed: its `block` or `mask` covers
  the whole document.

## Open questions

1. **Quirks mode.** Browsers match `#id` and `.class` case-insensitively in quirks-mode documents, so
   a browser can match more than the profile's definition. Acceptable as withholding more, or should
   the profile ban mixed-case ids and classes?
2. **The case-insensitive attribute list.** The baseline engines should agree on the HTML attributes
   whose values compare case-insensitively; any attribute where they do not is a candidate for
   rejection from the profile.

## References

- [SEP-0009: Component capture privacy](0009-component-privacy.md)
- [SEP-0015: Component watch](0015-component-watch.md)
- [Selectors Level 3](https://www.w3.org/TR/selectors-3/)
- [HTML: case-sensitivity of selectors](https://html.spec.whatwg.org/multipage/semantics-other.html#case-sensitivity-of-selectors)
