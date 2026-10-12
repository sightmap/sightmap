---
sep: 0021
title: A portable selector profile
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-10-06
updated: 2026-10-12
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

A corpus may use any CSS selector, and sightmap's matcher evaluates them all. Consumers that apply a
corpus's selectors on their own do not. A recording client uses the visitor's browser engine, which
may be years old. A client that classifies an element once, when it is inserted, cannot follow a
rule that depends on the element's siblings or descendants. A matcher re-implemented outside a
browser supports a narrower grammar than CSS, and gets parts of it wrong.

This SEP names the subset all of them evaluate identically, the `capture-baseline` selector profile.
Every corpus is validated against it. A privacy or watch rule outside it is an error, and resolves
conservatively until fixed.

## Motivation

Privacy ([SEP-0009](0009-component-privacy.md)) and watch ([SEP-0015](0015-component-watch.md)) are
only as good as the selectors behind them. A consumer that cannot evaluate a selector, or evaluates it
differently, records what the corpus said to withhold, or misses what it said to watch. Three kinds of
consumer diverge from sightmap's matcher.

**Old browser engines.** A recording client classifies elements with `Element.matches`. Its oldest
supported engines lack `:is()`, `:where()` and `:not()` with a list or compound (2020 and later),
`:has()` (2022 and later) and the attribute `i` flag (2016 and later). One of these on a privacy rule
either throws, losing the rule, or forces the client to withhold everything.

**Classification at insert time.** A recording client decides an element's privacy when the element is
inserted, and re-checks it only when that element's own attributes change. A rule that reads anything
other than the element and its ancestors goes stale:

```html
<section class="payment">
  <p class="saved-card">Visa ending 4242</p>
  <input name="cardnumber">   <!-- inserted after the section was classified -->
</section>
```

```yaml
- name: PaymentSection
  selector: 'section:has(input[name="cardnumber"])'
  privacy: block
```

The section was classified before the input existed, so its note was recorded. Sibling combinators
and positional pseudo-classes (`:first-child`, `:nth-child()`) fail the same way.

**Matchers outside a browser.** Consumers that apply selectors server-side, to captured pages or
indexed elements, re-implement matching. These commonly support type, `#id`, `.class` and attribute
selectors, descendant and child combinators, and `:not()` around one simple selector. They commonly
lack selector lists, attribute flags and every other pseudo-class. They also commonly get these wrong:

| Selector | Browser | A common re-implementation |
|---|---|---|
| `.a > * > input` | `*` matches any element | `*` compared as a literal tag name, never matching |
| `[class]`, `[id]` | true when the element has the attribute | class and id are stored apart from other attributes, so never true |
| `#a#b` | needs both ids, never matches | the second id replaces the first |

The profile keeps the constructs every one of these consumers evaluates identically and rejects the
rest.

## Proposal

### Shape

No new fields. The change is to which selectors validate, and how a privacy or watch rule that does
not is resolved.

### The `capture-baseline` profile

A selector profile names the selector features a corpus may use. Every corpus uses
`capture-baseline`. Its browser baseline:

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
compound    := type simple* | simple+
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
- A type selector comes first in its compound.
- Only `=` may compare against an empty value, and a `~=` value contains no whitespace.
- `class` and `id` are never tested for presence alone. On `class`, only `[class~=v]` and
  `[class*=v]` are allowed; anything else is written with `.class`.
- Type, attribute and pseudo-class names are written in lowercase.

Outside the profile:

| Construct | Why |
|---|---|
| `:is()`, `:where()`, `:not()` with a list or compound, attribute `i`/`s` flags | Missing from baseline engines |
| `:has()` | Missing from baseline engines, and depends on descendants |
| `+`, `~`, `:first-child`, `:nth-child()` and the other structural pseudo-classes, `:empty`, `:root` | Depend on elements other than the target and its ancestors |
| `:hover`, `:focus`, `:checked` and other state pseudo-classes | Depend on live state a captured page does not carry |
| `*`, `[class]`, `[id]` | Commonly evaluated differently outside a browser; name the element, or join its neighbours with a descendant combinator |
| Pseudo-elements, namespaces | Do not select elements, or not portably |

Every selector in the profile depends only on an element and its ancestors, so a consumer that
classifies an element once at insert time, or holds only the element's ancestor path, resolves it
exactly.

**Future profiles (to do).** A later SEP may define profiles that admit newer selectors, such as one
for CSS Selectors Level 4. A corpus would opt in explicitly through the corpus-root key
`selectorProfile`, which this SEP reserves. Choosing such a profile is the author's acceptance that
older engines and re-implemented matchers cannot apply those selectors.

### Rules outside the profile

A privacy rule whose selector is outside the profile never matches loosely:

- a `block` or `mask` applies to the whole document, the conservative reading of a rule that cannot
  be evaluated;
- an `unmask` is ignored.

A rule's *subject* is its last compound. A privacy rule whose subject has no `#id`, class or
attribute outside `:not()` (`:not(.x)`, `.card > :not(.x)`) is resolved the same way: it applies to
nearly every element, so an `unmask` would reopen the page and a `block` would close it.

A watch rule outside the profile is not reported.

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

### Non-goal: indexes without element boundaries

Some consumers match a selector against a search index that flattens an element's ancestor path into
one token stream, with no boundary between elements and only some attributes kept. No selector
profile evaluates exactly there: `div.a` also matches `div span.a`. Such a consumer should resolve
components when it ingests a page, with a conforming matcher, and index the component's name rather
than translate its selectors.

### JSON Schema

No structural change.

### Conformance

A conforming SDK:

- MUST report a selector outside the profile, or a universal subject, with the severities in
  [Diagnostics](#diagnostics).
- MUST resolve a privacy rule outside the profile, or with a universal subject, as covering the whole
  document when it is `block` or `mask`, and ignore it when it is `unmask`; and MUST NOT report a watch
  rule outside the profile.

New conformance fixture: `032-selector-profile` (validation and diagnostics).

## Evidence

The reference implementation: #495 (`ParseProfileSelector`, the profile grammar with coded
rejections), #496 (validation, fixture `032-selector-profile`) and #502 (resolution of rules outside
the profile).

**Semantics checked against a browser.** Chrome agrees with the reference matcher on every construct
in the profile, including the HTML attributes whose values compare case-insensitively
(`input[type=password]` matches `type="PASSWORD"`, on HTML and SVG elements alike) and repeated
attribute tests.

**Existing corpora.** None of the 37 corpora in this repository gains a diagnostic. One real corpus
tests one attribute twice (`[class*="..."][class*="..."]`) to target generated class names, which is
why the profile keeps every test rather than rejecting the repeat.

Reproduce from `go/`:

```sh
go test ./sightmap/ -run 'ProfileSelector|Validate'
go test ./sightmap/ -run '^$' -fuzz FuzzParseProfileSelector -fuzztime 60s
```

## Alternatives considered

1. **Restrict only components that declare `privacy` or `watch`.** Rejected: turning on privacy for an
   existing component would then require rewriting its selector.
2. **Keep the full grammar and make consumers fail closed.** A consumer that cannot evaluate `:has()`
   would withhold every element the rule might cover. Rejected: with no way to bound "might", the
   conservative answer covers the whole page.
3. **Native-engine semantics as the definition.** Define matching as whatever `Element.matches`
   returns. Rejected: engines disagree on newer features, and a consumer outside a browser has no
   engine to ask.

## Migration

- Components using selectors outside the profile see warnings for the deprecation window, errors on
  components declaring `privacy` or `watch`. `:is()` and `:where()` rewrite mechanically into separate
  selectors; `*` is dropped in favour of a descendant combinator or a named element; `:has()` and
  sibling selectors need an attribute or class on the element itself.
- A privacy rule outside the profile resolves conservatively until fixed: its `block` or `mask` covers
  the whole document.

## Open questions

1. **Quirks mode.** Browsers match `#id` and `.class` case-insensitively in quirks-mode documents, so
   a browser can match more than the profile's definition. Acceptable as withholding more, or should
   the profile ban mixed-case ids and classes?
2. **The case-insensitive attribute list.** Browsers compare the values of a fixed list of HTML
   attributes (`type`, `lang`, `rel` and others) case-insensitively. Matchers outside a browser may
   compare them exactly. Should the profile reject a mixed-case value on those attributes, so both
   agree?

## References

- [SEP-0009: Component capture privacy](0009-component-privacy.md)
- [SEP-0015: Component watch](0015-component-watch.md)
- [Selectors Level 3](https://www.w3.org/TR/selectors-3/)
- [HTML: case-sensitivity of selectors](https://html.spec.whatwg.org/multipage/semantics-other.html#case-sensitivity-of-selectors)
