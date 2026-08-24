# Signals — grammar & semantics sketch

Draft working notes on the `proposal/range-signals` branch. Companion to `range-signals-examples.md` (the test-bed).
Every production here is checked against EX1–EX6 there. **Draft — pinning shape,
not final spelling.** Where a choice is open it's marked `(?)`.

---

## 1. Structure (EBNF-ish)

```
rule        = "name:" dotted-ident
              [ "within:" duration ]
              "match:" binding+
              [ "until:" matcher ]          # abort + segment
              [ "where:" pred ]             # emit gate; default = true
              "emit:" emit-spec ;

binding     = binding-name ":" matcher [ quantifier ] ;
matcher     = kind-ident [ "where" pred ] ; # kind from the consumer vocabulary
quantifier  = "+" | "*" | "?" ;             # absent = exactly one
duration    = int ("ms"|"s"|"m") ;
```

Two predicate scopes:
- **binding-scope** (`matcher`'s inline `where`): evaluated against *one input*;
  attr refs are unqualified built-ins / `name` / qualified props of *that* input.
- **rule-scope** (`where:`): evaluated against the *accumulated bindings*; attr
  refs are `binding.<...>`, aggregates, `exists`, and lifecycle predicates.

Semantics of evaluation/emission: see `range-signals-examples.md` §"continuous eval,
emit-once". Grammar just supplies the surface.

---

## 2. Expression language (`pred`)

```
pred     = or ;
or       = and { "or" and } ;
and      = unary { "and" unary } ;
unary    = [ "not" ] atom ;
atom     = "(" pred ")"
         | comparison | membership | existence | lifecycle | regex ;

comparison = value cmp literal ;
cmp        = "==" | "!=" | "<" | "<=" | ">" | ">=" ;
membership = nameset ( "has" | "in" | "not in" ) ( ident | list ) ;
existence  = "exists" binding-name ;                 # did that binding match?
lifecycle  = "elapsed" | "closed" | "session.ended" ;# instance/window boundary
regex      = value "~" string ;                      # RE2 (align w/ SEP-0006)

value      = attr-ref | aggregate ;
attr-ref   = [ binding-name "." ] leaf ;
leaf       = builtin-attr                             # e.g. status, selector, url
           | entity-name "." prop-name               # match-contributed
           | "when" ;
aggregate  = ("count"|"sum"|"min"|"max"|"avg") "(" [ value ] ")"
           | binding-name "." ("count"|"first"|"last") [ "." leaf ] ;
nameset    = [ binding-name "." ] "name" ;           # the set-valued subject
```

### Operators by attribute type (d1a8 makes this checkable)
| type | operators |
|---|---|
| string | `==` `!=` `~` (regex) |
| int / duration / number | `==` `!=` `<` `<=` `>` `>=` |
| bool | `==` `!=`, or bare truthiness |
| set (`name`, `tags`) | `has` / `in` / `not in` |

Open: SEP-0007's `filter` used glob (`text: "*declined*"`). Reconcile to `~`
RE2 for one string-match dialect across signals+messages. `(?)`

### Aggregates
`count`, `sum(attr)`, `min/max/avg(attr)`, and positional `binding.first` /
`binding.last` (yield an input → then `.attr` / `.when`). Operate over a
quantified binding's accumulated set. `binding.count` sugar == `count(binding)`.

---

## 3. `emit`

```
emit-spec = [ "tags:" list ]
            [ "attrs:" "{" ( key ":" value )* "}" ]
            [ "as:" ("tag" | "mint") ] ;    # default: arity-based (single→tag, multi→mint)
```

- Emitted signal is itself a typed vocabulary member (its `attrs` schema is what
  it declares here) → it can feed higher strata. Stratification/acyclicity keeps
  it terminating.
- A minted multi-input signal carries `span=[t0,t1)`; its logical `when` = span
  end.
- No implicit merge of member attrs — you *project* (`attrs:`) via bindings, so
  no shadowing.

---

## 4. Point signals are the degenerate case

One binding, no quantifier, `where` optional (= the binding's own match), emit
tags in place. This is exactly today's SEP-0007 `signals:` rule, with
`filter: {k: v}` re-expressed as `where: k == v`. So the point layer is a strict
subset of this grammar — it can ship first (see §6) and the existing SEP-0007
impl (#114) is the seed.

---

## 5. Checks against the catalog

| EX | exercises |
|---|---|
| EX1 | single binding, implicit `where`, `as: tag` default |
| EX2 | `+` quantifier, `exists`, cross-binding `where`, `attrs` projection |
| EX3 | `until` (abort+segment), contrast with `where` |
| EX4 | closer-that-emits as a `where` binding, `.first/.last.when`, span |
| EX5 | monotone `where` (prompt) vs `elapsed` gate (final) |
| EX6 | bounded absence: `elapsed and not exists` |

Every production above is used by at least one EX; every EX parses under the
grammar. Gap to close next: a use-case that needs `sum/avg` or `~` regex (none
of EX1–6 do) — add EX7/EX8 to keep those honest.

---

## 6. Chunking for digestibility (per "manageable chunks")

Proposed split so reviewers don't face the whole space at once, each a small SEP
building on the last:

1. **Vocabulary** — consumer-declared typed per-kind schema. Foundational; unblocks static checking.
2. **Attributes & enrichment** — provenance, qualified `Name.prop`, subject/context, tags-as-set-attrs.
3. **Point signals** — retrofit SEP-0007 onto 1+2 (`filter`→`where`, single binding). *Smallest; ships first; #114 is the seed.*
4. **Composite signals** — the big one: bindings, quantifiers, `where`, `until`, `within`, aggregates, `emit`, `elapsed`, continuous-eval/emit-once.
5. **Areas / usage** — likely docs-only patterns on top of 3+4, not a new construct.

Dependency order is 1 → 2 → 3 → 4 → 5. 3 is independently useful and low-risk;
4 is where the novel semantics concentrate and deserves its own SEP.

---

## 7. Open questions (grammar-specific)

- Ordered vs unordered `match:` — lean unordered set + `.when` in `where`. `(?)`
- glob vs RE2 for string match. `(?)`
- `elapsed`/`closed`/`session.ended` vocabulary + whether `session-end` is a
  bindable primitive input (would fold unbounded absence into a normal binding). `(?)`
- Does `until` need its own `where`, or just a matcher? (EX3 only needs a matcher.) `(?)`
- Multiplicity in `attrs` projection when a binding is quantified but you
  reference a scalar leaf (`decline.amount` when `decline` matched once is fine;
  what about `clicks.amount`? → require an aggregate). `(?)`
