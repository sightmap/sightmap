---
sep: 0021
title: A portable selector profile
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-10-06
updated: 2026-10-14
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

A corpus may use any CSS selector, and sightmap's matcher evaluates them all. Privacy and watch
rules are applied by consumers that run on every page a visitor loads, and what a consumer pays is
the number of nodes times the number of rules: it has to ask of each node it processes whether any
rule applies. Broad selectors make that cost unavoidable, because nothing about a node or its
ancestors says which rules could match it, so every rule is tried. Consumers also differ in what
they can evaluate: a recording client uses the visitor's browser engine, which may be years old,
and a matcher re-implemented outside a browser supports a narrower grammar than CSS and gets parts
of it wrong.

This SEP defines the `capture-baseline` selector profile. Every selector in it depends only on an
element and its ancestors, and names its target by an id, class, attribute or type, so a consumer
can index rules by what they target and test a node against the few rules that could match it,
whichever way it walks the page. Every corpus is validated against the profile. A privacy or watch
rule outside it is an error. Until it is fixed, a privacy rule resolves conservatively and a watch
rule never matches.

## Motivation

Privacy ([SEP-0009](0009-component-privacy.md)) and watch ([SEP-0015](0015-component-watch.md)) are
only as good as the selectors behind them, and they are applied on the hot path of every page. A
consumer that cannot evaluate a selector, or evaluates it differently, records what the corpus said
to withhold, or misses what it said to watch. A consumer that evaluates it too slowly slows the page
it is recording. The goal is a grammar whose rules a consumer can match as cheaply as possible
whatever strategy it uses, and that every consumer evaluates the same way.

**Cost on every node.** A consumer that cannot tell which rules could match a node has to try all of
them. The only shortcut a typical consumer has is to skip the descendants of a node it has already
withheld, so the work for a document is its node count times the corpus's rule count, repeated for
every node inserted afterwards. Three properties of a selector decide whether a consumer can do
better:

- **It reads only the element and its ancestors.** A rule that reads descendants or siblings
  (`:has()`, `+`, `~`, `:nth-child()`) cannot be decided from a node and the path above it, so a
  consumer cannot resolve a node as it meets it, and a node's answer can change when another node
  elsewhere changes. An ancestor-only rule is decided by the path alone, so a consumer can resolve
  each node on its own, a whole document in one pass, or a newly inserted subtree from its parent's
  resolved state, and every one of those gives the same answer.
- **Its target is named.** A selector whose last compound has no id, class, attribute or type
  (`*`, `:not(.x)`) is a candidate at every node, so a consumer cannot skip any rule for any node. A
  selector that names an id, class, attribute or type can be filed under that key, and a node is tested
  only against the rules filed under keys it carries.
- **Its cost is bounded.** A selector of unbounded length multiplies the cost of every test.

**Old browser engines.** A recording client classifies elements with `Element.matches`. Its oldest
supported engines lack `:is()`, `:where()` and `:not()` with a list or compound (2020 and later),
`:has()` (2022 and later) and the attribute `i` flag (2016 and later). One of these on a privacy rule
either throws, losing the rule, or forces the client to withhold everything.

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

Grammar, in ABNF. Values are Unicode code points. `SP` is U+0020, the only whitespace outside a
quoted string.

```abnf
; A selector is one chain. No commas: alternatives are separate selectors.
selector     = *SP compound *( combinator compound ) *SP
combinator   = 1*SP / *SP ">" *SP
compound     = type *simple / 1*simple
simple       = id-sel / class-sel / attribute / negation
type         = ALPHA *( ALPHA / DIGIT / "-" )
id-sel       = "#" ident
class-sel    = "." ident
attribute    = "[" attr-name [ attr-op attr-value ] "]"
attr-name    = ( ALPHA / "_" ) *( ALPHA / DIGIT / "_" / "-" )
attr-op      = "=" / "~=" / "|=" / "^=" / "$=" / "*="
attr-value   = ident / quoted
negation     = ":not(" ( type / id-sel / class-sel / attribute ) ")"

; Identifiers follow CSS, minus hex escapes.
ident        = [ "-" ] ident-start *ident-char
ident-start  = ALPHA / "_" / non-ascii / escape
ident-char   = ident-start / DIGIT / "-"
escape       = "\" printable-non-hex
quoted       = DQUOTE *( qchar / escape ) DQUOTE
             / SQUOTE *( qchar / escape ) SQUOTE
non-ascii    = %x80-10FFFF            ; excluding whitespace
printable-non-hex = %x21-7E           ; except hex digits
qchar        = %x20-7E / %x80-10FFFF  ; except the delimiter and "\"
```

A type selector, when present, comes first in its compound, as the `compound` rule requires; a
type after another simple selector is ambiguous in some matchers. A single escaped printable ASCII
character (`\:`, `\.`, `\/`) is allowed. A component lists
alternatives as separate selectors; the loader already splits a top-level comma into separate
selectors. An attribute may be tested more than once; every test applies.

Seven constraints cannot be written in the grammar and apply during parsing:

| | Constraint | Why |
|---|---|---|
| C1 | A compound has at most one `#id`. | `#a#b` can never match, and matchers commonly let the second id replace the first. |
| C2 | Type, attribute and pseudo-class names are lowercase, except the [SVG names](#svg-names) below. The grammar admits either case so those names can be derived; this constraint narrows it. | HTML folds these names and XML does not; lowercase makes the two agree. |
| C3 | `class` and `id` are never tested for presence alone. | Matchers that store class and id apart from other attributes never see them there, so `[class]` is silently false. |
| C4 | On `class`, only `~=` and `*=` are allowed; anything else is written with `.class`. | Every other operator compares against the whole class attribute as one string, which rarely means what the author intended. |
| C5 | Only `=` may compare against an empty value. | `[a^=""]` and its siblings can never match, so accepting them silently drops a rule. |
| C6 | A `~=` value contains no whitespace. | `~=` matches one whitespace-delimited token, so such a value can never match. |
| C7 | A selector is at most 8 compounds and 1,024 bytes. | Per-element cost grows with document depth, so an unbounded selector is an unbounded cost. |

#### SVG names

The HTML specification adjusts the case of some SVG element and attribute names. These are accepted
with their canonical mixed-case spelling, and match exactly. The lists are closed; any other
mixed-case name is rejected under C2.

Elements: `altGlyph`, `altGlyphDef`, `altGlyphItem`, `animateColor`, `animateMotion`,
`animateTransform`, `clipPath`, `feBlend`, `feColorMatrix`, `feComponentTransfer`, `feComposite`,
`feConvolveMatrix`, `feDiffuseLighting`, `feDisplacementMap`, `feDistantLight`, `feDropShadow`,
`feFlood`, `feFuncA`, `feFuncB`, `feFuncG`, `feFuncR`, `feGaussianBlur`, `feImage`, `feMerge`,
`feMergeNode`, `feMorphology`, `feOffset`, `fePointLight`, `feSpecularLighting`, `feSpotLight`,
`feTile`, `feTurbulence`, `foreignObject`, `glyphRef`, `linearGradient`, `radialGradient`,
`textPath`.

Attributes: `attributeName`, `attributeType`, `baseFrequency`, `baseProfile`, `calcMode`,
`clipPathUnits`, `diffuseConstant`, `edgeMode`, `filterUnits`, `glyphRef`, `gradientTransform`,
`gradientUnits`, `kernelMatrix`, `kernelUnitLength`, `keyPoints`, `keySplines`, `keyTimes`,
`lengthAdjust`, `limitingConeAngle`, `markerHeight`, `markerUnits`, `markerWidth`,
`maskContentUnits`, `maskUnits`, `numOctaves`, `pathLength`, `patternContentUnits`,
`patternTransform`, `patternUnits`, `pointsAtX`, `pointsAtY`, `pointsAtZ`, `preserveAlpha`,
`preserveAspectRatio`, `primitiveUnits`, `refX`, `refY`, `repeatCount`, `repeatDur`,
`requiredExtensions`, `requiredFeatures`, `specularConstant`, `specularExponent`, `spreadMethod`,
`startOffset`, `stdDeviation`, `stitchTiles`, `surfaceScale`, `systemLanguage`, `tableValues`,
`targetX`, `targetY`, `textLength`, `viewBox`, `viewTarget`, `xChannelSelector`,
`yChannelSelector`, `zoomAndPan`.

Lowercasing exists so HTML and XML folding agree. A name whose only spelling is mixed case gives
consumers nothing to disagree about.

#### Matching

For an HTML element, the values of these attributes compare ASCII case-insensitively: `accept`,
`accept-charset`, `align`, `alink`, `axis`, `bgcolor`, `charset`, `checked`, `clear`, `codetype`,
`color`, `compact`, `declare`, `defer`, `dir`, `direction`, `disabled`, `enctype`, `face`, `frame`,
`hreflang`, `http-equiv`, `lang`, `language`, `link`, `media`, `method`, `multiple`, `nohref`,
`noresize`, `noshade`, `nowrap`, `readonly`, `rel`, `rev`, `rules`, `scope`, `scrolling`,
`selected`, `shape`, `target`, `text`, `type`, `valign`, `valuetype`, `vlink`. Every other
attribute, and every attribute on a non-HTML (SVG or MathML) element, compares exactly. A matcher
that compares everything exactly would otherwise mask `input[type=Password]` in one consumer and not
in another.

An unquoted attribute value is an `ident`, which cannot begin with a digit: `[tabindex=0]` is
outside the profile, and `[tabindex="0"]` is inside it.

Browsers in quirks mode match `#id` and `.class` case-insensitively, so a browser may match more
than the profile defines. That only withholds more, and the grammar does not change.

Outside the profile:

| Construct | Why |
|---|---|
| `:is()`, `:where()`, `:not()` with a list or compound, attribute `i`/`s` flags | Missing from baseline engines |
| `:has()` | Missing from baseline engines, and depends on descendants |
| `+`, `~`, `:first-child`, `:nth-child()` and the other structural pseudo-classes, `:empty`, `:root` | Depend on elements other than the target and its ancestors |
| `:hover`, `:focus`, `:checked` and other state pseudo-classes | Depend on live state a captured page does not carry |
| `*`, `[class]`, `[id]` | Commonly evaluated differently outside a browser; name the element, or join its neighbours with a descendant combinator |
| Pseudo-elements, namespaces | Do not select elements, or not portably |

Every selector in the profile depends only on an element and its ancestors, so a consumer resolves
it exactly whether it tests each node on its own, walks a whole document once, resolves an inserted
subtree from its parent, or holds only an element's ancestor path.

**Future profiles (to do).** A later SEP may define profiles that admit newer selectors, such as one
for CSS Selectors Level 4. A corpus would opt in explicitly through the corpus-root key
`selectorProfile`, which this SEP reserves. Choosing such a profile is the author's acceptance that
older engines and re-implemented matchers cannot apply those selectors.

### Rules outside the profile

A privacy rule whose selector is outside the profile never matches loosely:

- a `block` or `mask` applies to the whole document, the conservative reading of a rule that cannot
  be evaluated;
- an `unmask` is ignored.

A watch rule whose selector is outside the profile never matches: a consumer emits no watch events
for it.

A rule's *subject* is its last compound. A subject is *universal* when it has no type, `#id`, class
or attribute outside `:not()` (`:not(.x)`, `.card > :not(.x)`). A privacy rule with a universal
subject is resolved like a rule outside the profile: it applies to nearly every element, so an
`unmask` would reopen the page and a `block` would close it. A watch rule with a universal subject
is evaluated as written, since reporting too much is noise rather than a leak; it is still a
diagnostic, below.

A keyed subject also makes rules indexable. When every subject carries an id, class, attribute or
type, a consumer can bucket rules by subject key and test an element only against the rules whose
key it carries. A universal subject is a candidate at every node and defeats that.

### A conforming parser

A parser for the profile has five properties:

- **Total.** Every byte input, valid UTF-8 or not, returns a result or an error; none panics.
- **Deterministic.** The same input gives the same output, including the error position.
- **Subsumed.** Anything the profile accepts, the SDK's general selector parser accepts with the same
  meaning. The profile is a strict subset, never a dialect.
- **Round-tripping.** Rejoining the parsed compounds with their combinators reparses to the identical
  compounds.
- **First-failure positioned.** An error reports the byte offset of the first construct outside the
  profile, scanning left to right.

The error code (`selector-not-in-profile`) and the position are normative. The message wording is
not. A message SHOULD name the in-profile alternative where one exists; in particular, a value that
begins with a digit SHOULD be reported with advice to quote it, as in `[tabindex="0"]`, at the
offset of the first digit.


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
  document when it is `block` or `mask`, and ignore it when it is `unmask`.
- MUST NOT emit watch events for a watch rule outside the profile.

New conformance fixtures: `032-selector-profile` (validation and diagnostics) and
`036-selector-classification` (parse-only cases, each a selector and its expected code, shared by
every implementation of the parser).

## Evidence

The reference implementation:
[#495](https://github.com/sightmap/sightmap/pull/495) (`ParseProfileSelector`, the profile grammar
with coded rejections), [#496](https://github.com/sightmap/sightmap/pull/496) (validation, fixture
`032-selector-profile`), [#502](https://github.com/sightmap/sightmap/pull/502) (resolution of rules
outside the profile) and [#512](https://github.com/sightmap/sightmap/pull/512) (chain resolution
from a subject-keyed index).

**Semantics checked against a browser.** Chrome agrees with the reference matcher on every construct
in the profile, including the HTML attributes whose values compare case-insensitively
(`input[type=password]` matches `type="PASSWORD"` on an HTML element) and repeated
attribute tests.

**Existing corpora.** None of the 37 `.sightmap` directories in this repository outside
`spec/conformance` gains a profile diagnostic: the docs and web corpora, 14 example corpora under
`web/src`, a web script fixture and 20 CLI test fixtures. One real corpus
tests one attribute twice (`[class*="..."][class*="..."]`) to target generated class names, which is
why the profile keeps every test rather than rejecting the repeat.

**Measured cost.** A Go reference harness, run on a synthetic corpus of 719 rules (563 carrying
privacy) with matchers verified to agree on every chain and node:

| Operation | Result |
|---|---|
| Privacy resolution, subject-keyed index against evaluating every rule | 30-44x faster at the full corpus |
| Watch resolution, same comparison | 3.9-9.5x faster; every matching watch rule reports, so there is no early exit |
| Classifying an inserted subtree by inheriting the insertion point's resolved state | About 2 us per node, flat from 10 to 1,000 nodes |

The indexed result holds whichever way the document is walked: testing each node on its own,
walking the document once, and inheriting from an insertion point all agree, and the index is what
carries the gain. The inherit strategy is exact because every selector depends only on an element
and its ancestors.

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

None.

## References

- [SEP-0009: Component capture privacy](0009-component-privacy.md)
- [SEP-0015: Component watch](0015-component-watch.md)
- [Selectors Level 3](https://www.w3.org/TR/selectors-3/)
- [HTML: case-sensitivity of selectors](https://html.spec.whatwg.org/multipage/semantics-other.html#case-sensitivity-of-selectors)
