---
sep: 0012
title: Extract path predicates
author: Joel Webber (@joelgwebber)
status: Draft
created: 2026-09-08
updated: 2026-10-05
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Let a segment of a component extract `path` carry a **predicate** that chooses which match it
resolves to: `Tab[selected=true].label` reads the label of the selected tab, not the first one. The
predicate grammar is the one component queries already use (`[prop op value]`), and a predicate
tests only a **declared property** of the component it is attached to. A predicate filters; it never
changes the result's shape. Whether a path is single- or multi-valued is still decided by `Name[]`
alone, as [SEP-0017](0017-extract-object.md) defines.

This fills the bracket syntax SEP-0017 reserved. Every path valid today stays valid and means the
same thing. Array-valued results, the other question SEP-0017 left open, are out of scope.

## Motivation

[SEP-0017](0017-extract-object.md) settled how many matches a path reads: a plain segment takes the
first match in document order, and `Name[]` takes every match. It left open **which** match. A
container often holds several same-shaped children, and the value an author wants is on the one in
a particular state:

- the label of the **selected** tab in a tab strip;
- the quantity field in the **Product** section of a form with several sections;
- the SKUs of the cart items in **one category**.

None of these has an expression today. First-match reads whichever child happens to come first. The
workaround of declaring a narrower component (an `ActiveTab` whose selector matches only the
selected `Tab`) is rejected by SEP-0017 itself: one node matching both `Tab` and `ActiveTab` breaks
the one-match-per-node model, so `Tab` silently loses the node it should have matched.

The case has a second, worse form. A path written against a page with one `Section` resolves
correctly until the page renders two, and then it reads from whichever comes first, or nothing at
all. Whether the property resolves becomes a function of runtime state, which is the hardest kind of
defect to debug. A predicate is the in-place fix: `Form.Section[name=Product].QtyField.value` says
which section was meant.

## Proposal

### Shape

A segment of a `from: component` or `from: component.exists` path may carry one or more predicates
after the component name, and an optional `[]` after those:

```
segment   := Name predicate* ( "[]" )?
predicate := "[" prop op value ( " i" )? "]"
op        := "=" | "^=" | "*="
```

```yaml
# Before: no way to say which tab, so this reads the first
- name: TabStrip
  selector: '[role="tablist"]'
  properties:
    - name: current
      extract: { from: component, path: Tab.label }

# After
- name: TabStrip
  selector: '[role="tablist"]'
  properties:
    - name: current
      extract: { from: component, path: 'Tab[selected=true].label' }
  children:
    - name: Tab
      selector: '[role="tab"]'
      properties:
        - name: label
          extract: { from: dom.text }
        - name: selected
          extract: { from: dom.state, path: selected }
```

The predicate reads `selected`, a property `Tab` declares. It does not read the DOM. Reaching
interactive state, an attribute, or text from a predicate always goes through a property declared on
the component, so that what a predicate can test is listed in one place: the component definition.

### Grammar

Predicates reuse the component-query predicate grammar used by `sightmap browser` interaction
commands, so a corpus has one predicate language:

| Form | Matches when the property |
|---|---|
| `[prop=value]` | equals `value` |
| `[prop^=value]` | starts with `value` |
| `[prop*=value]` | contains `value` |
| `[prop=value i]` | as above, compared case-insensitively (any operator) |
| `[a=1][b=2]` | satisfies every predicate (AND) |

- `prop` is a property name, matching `^[a-z][a-z0-9_]*$` as every property name does. The
  component-query grammar also accepts hyphens; a path predicate does not, since no declared
  property can carry one.
- `value` is unquoted, ending at whitespace or `]`, or double-quoted with `\` escaping the next
  character. A value containing whitespace, `]`, `.` or `"` must be quoted.
- The component-query occurrence index (`#N`) is **not** part of the path grammar. It selects by
  position, which is the brittleness a predicate exists to remove.

A path is tokenized, not split on `.`. A `.` or `]` inside a quoted value is part of the value:
`'Release[version="1.2"].notes'` has two segments.

`[` and `]` are YAML flow indicators, and a predicate value is often double-quoted, so a path with
a predicate is best single-quoted in YAML in both flow and block style:
`path: 'Tab[label="Sign in"].id'`.

### Predicates filter; `[]` decides shape

A predicate narrows the set of candidates a segment considers. It never changes how many it takes,
and a path's shape stays readable from its `[]` segments alone:

| Path | Shape | Resolves to |
|---|---|---|
| `Tab[selected=true].label` | single | the first `Tab` whose `selected` is `true` |
| `Row[state=active][].amount` + `join` | multi | `amount` of every `Row` whose `state` is `active` |
| `Form.Section[name=Product].QtyField.value` | single | the first `QtyField` in the first `Section` named `Product` |
| `Cart.Item[category=sneakers][].sku` + `join` | multi | every `sku` among the `Item`s in that category |

So a single-valued path never produces several values, whatever the page holds, and a multi-valued
one never collapses to a lone scalar that stands in for a list. A multi-valued path still requires
`join`, per SEP-0017.

`[]`, when present, follows the segment's predicates: `Row[state=active][]`, not
`Row[][state=active]`. The second form is invalid.

### Semantics

- **Candidates.** A segment's candidates are the nodes matched as the named component within the
  node or nodes the previous segment produced, in document order, as SEP-0017 defines. A predicate
  removes every candidate whose property fails it.
- **Evaluation.** A predicate is evaluated against the candidate's own resolved property, after that
  property's `pattern` refinement. A property that is absent (it did not resolve, or it is withheld
  under SEP-0009) fails every predicate, including `[prop*=""]`.
- **Ordering and cycles.** Properties already resolve bottom-up, because a path addresses only
  descendants (SEP-0010). A predicate reads a property of a node beneath the one resolving the
  path, so it adds no new ordering constraint and cannot form a cycle.
- **`component.exists`.** A predicate on an `exists` path is valid: `Tab[selected=true]` yields
  `"true"` when some `Tab` is selected. `[]` remains invalid on an `exists` path, since its result
  is not a set of values.
- **Omission (unchanged).** A path whose predicates remove every candidate resolves to nothing and
  omits the property, as any unresolved path does.

### Validation

- A predicate's `prop` must name a property **declared on the component the segment names**, or the
  reserved `value`. Otherwise the path is rejected with `extract-predicate-unknown`, naming the
  component's declared properties. This is the guarantee SEP-0007 gives `signals:` filters
  (`signal-filter-unknown`): without it, a predicate naming a renamed property passes and the path
  silently never resolves.
- A malformed predicate (unknown operator, unterminated quote, empty `prop`) or a misplaced `[]` is
  rejected as an invalid extract, replacing SEP-0017's blanket rejection of bracketed content.
- `extract-privacy-withheld` extends to predicates. A predicate on a property every capture
  consumer withholds (per the SEP-0017 privacy table) never matches, and the warning says so. This
  matters more for a predicate than for a read: privacy decides not only whether a value comes back
  but which node the path selects.

### JSON Schema

None. `path` is a string, and its grammar is validated by the SDK, as SEP-0010 and SEP-0017
already arrange. The schema description of `extract.path` gains a mention of predicates.

## Conformance

A conforming SDK MUST:

- Parse path predicates with the grammar above, tokenizing so that quoted values may contain `.`,
  `]` and whitespace.
- Evaluate each predicate against the candidate's resolved, refined property; treat an absent or
  withheld property as failing; AND repeated predicates; and compare case-insensitively under ` i`.
- Apply predicates before a segment takes its first match or, with `[]`, every match, leaving the
  path's shape as SEP-0017 defines it.
- Accept predicates on `component.exists` paths.
- Reject `extract-predicate-unknown` for an undeclared predicate property, and reject a malformed
  predicate or a `[]` placed before a predicate.
- Extend `extract-privacy-withheld` to predicate properties.

A conforming SDK MUST pass the conformance fixtures this SEP adds.

## Alternatives considered

- **A narrower component per state** (`ActiveTab`). Rejected by SEP-0017: one node matching two
  components breaks one-match-per-node.
- **Predicates over attributes or state directly** (`Tab[@aria-selected=true]`, `Tab[:checked]`).
  Rejected. It would make extraction a second place DOM detail is spelled, beside selectors, and
  the component definition would no longer list everything a path can depend on. Declaring the
  property costs one line and makes the dependency visible.
- **A positional index** (`Tab#2`, `Tab[2]`). Rejected for the brittleness above; positions change
  when a page adds a row, which is the case predicates are for.
- **Letting a predicate imply multi-valued.** Rejected. A path's shape would then depend on how many
  candidates the page holds, so the same corpus would emit a scalar on one page and several values
  on the next.

## Migration

None. Every path SEP-0017 accepts is unchanged; predicates occupy syntax SEP-0017 rejects today.
Older SDKs reject a path with a predicate as invalid, per SEP-0017's reservation, so a corpus using
predicates needs an SDK that implements this SEP.

## Open questions

1. **Backtracking.** A plain segment takes the first candidate even when the rest of the path does
   not resolve beneath it: `Form.Section.QtyField` omits the property when the first `Section` has
   no `QtyField`, even if the second does. Should a segment instead take the first candidate whose
   remaining path resolves **structurally** (nodes exist and predicates hold, not values non-empty)?
   It turns an intermittent omission into a stable value. "Structurally" matters: falling through
   on an empty value or a privacy-withheld one would silently read a different node, which is worse
   than omission.
2. **An ambiguity diagnostic.** Should an SDK report `extract-ambiguous` (SHOULD, at runtime in
   snapshot or coverage output) when a single-valued segment had more than one candidate? It points
   an author at exactly the predicate this SEP adds. It cannot be a validation error, since it
   depends on the page.
3. **More operators.** Suffix (`$=`), negation, and numeric comparison are not in the component-query
   grammar today. Adding one should extend both grammars at once, and SEP-0007's `filter:` should
   probably adopt the same set, so a corpus keeps a single predicate language.
4. **Array-valued results.** A `[]` path without `join` stays reserved, as SEP-0017 leaves it. It
   affects every consumer of property values (snapshot annotation, `signals:` filters, consumers
   that store scalars) and deserves its own SEP.

## References

- [SEP-0017](0017-extract-object.md): the extract object, `Name[]`, `join`, and the bracket syntax
  this SEP fills.
- [SEP-0010](0010-tree-closed-component-properties.md): the tree-closed component path.
- [SEP-0009](0009-component-privacy.md): withholding, which predicates respect.
- [SEP-0013](0013-richer-node-data.md): `dom.state`, the usual source of a predicate's property.
- [SEP-0007](0007-signals.md): `signals:` filters, the other predicate surface in a corpus.
- `docs/cli/interaction.mdx`: the component-query grammar these predicates reuse.
