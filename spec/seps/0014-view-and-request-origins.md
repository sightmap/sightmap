---
sep: 0014
title: Origins — the list of origins a view or request is served from
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-09-08
updated: 2026-09-08
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add an optional `origins` field in three positions: a file-root default, and an override on a `view`
or a `request`. Every origin a corpus ever writes down is a **named definition** —
`{name, url: scheme://host[:port]}`, valid only at the file root — and every other mention of an
origin, anywhere `origins` appears, is a **reference**: a bare name pointing at a definition
registered anywhere in the corpus. There is no freestanding literal form; a URL is never written
except inside exactly one definition. A non-empty entity-level list of references *replaces* the
file-root default; an absent or empty list means unconstrained. The spec stays environment-agnostic —
it enumerates the origins an entity is served from, names each one so a corpus never repeats the same
URL twice, and leaves it to a downstream consumer to decide which one matters for its own
environment. Origins are declarative in v1: they do **not** participate in route matching.

## Motivation

Today the only host-bearing field is `url:` — optional, singular, a concrete navigation target for
tooling, not an identity. Four concrete consequences of having no other way to say what host an
entity lives on:

1. **A compiled page has no hostname.** A real consumer compiles a corpus into host-scoped page
   definitions. It derives a page's host from an explicit override, else from the view's own `url:`.
   When a real project's corpus declares no `url:` anywhere and passes no override, every compiled
   page gets zero domains, which downstream means "matches any host" and renders in a settings UI as
   a bare path with a blank hostname dropdown — the concrete bug this SEP grew out of.
2. **An unanchorable request regex.** A sightmap `request.route` is path-only. A consumer turning
   that into a live-traffic matcher against a full URL (scheme+host+path+query) has to drop the `^`
   anchor from its generated regex, because it has no host to anchor against — an accepted,
   documented false-positive risk in that consumer's own code.
3. **An asymmetric round trip.** The same consumer's reverse direction — adopting an org's existing
   page definitions into a corpus — already synthesizes `url: "https://" + host + route` from a
   page's first domain. Host information survives the trip *out* of a corpus but has nowhere to live
   *in* one, and a page with three domains collapses to one on the way out.
4. **The same origin gets typed out in every file.** A corpus split across many feature files (one
   per top-level route, per the spec's own authoring convention) has no way to define an origin once
   and point every file at it. Without naming, each file's default has to repeat the same literal
   URL, so a single rotated hostname becomes a multi-file find-and-replace — the reason this SEP
   requires a URL to be written exactly once, as a named definition, rather than allowing it to also
   appear as a freestanding literal wherever it's used.

A real production corpus can go entirely without host information — zero `url:` values, zero host
mentions — across dozens of files. The information exists only in the author's head.

## Proposal

### Shape

```yaml
# Before
version: 1
views:
  - name: OrderHistory
    route: "/ui/*/settings/orders/history"
requests:
  - name: GetProfile
    route: /settings/profile
    method: GET

# After — every origin is a named definition; every use of one is a reference by name
version: 1
origins:
  - name: prod
    url: https://app.acme.com
  - name: local
    url: https://app.acme.test:8443
views:
  - name: OrderHistory
    route: "/ui/*/settings/orders/history"     # inherits the file-root origins: prod, local
  - name: SsoLogin
    route: /sso/login
    origins: [auth]                             # references `auth`, defined in another file — replaces, not merges
requests:
  - name: GetProfile
    route: /settings/profile
    method: GET
    origins: [api]                              # references `api`, defined in another file
  - name: Ping
    route: /api/ping
    # inherits the file-root origins (prod, local), not any view's — see Semantics
```

`auth` and `api` are each defined once, in whichever file owns them, and are addressable from
anywhere in the corpus:

```yaml
# origins.yaml — origins shared across files, defined once
version: 1
origins:
  - name: auth
    url: https://auth.acme.com
  - name: api
    url: https://api.acme.com
```

### Field reference

| Position | Entry forms allowed | Required | Description |
|---|---|---|---|
| `origins` (file root) | named definition (`{name, url}`) or reference | no | Default list for every view and request in this file that declares none. A named definition here also registers `name` for reference from anywhere in the corpus. |
| `origins` (view) | reference only | no | Origins this view is served from. A non-empty list replaces the file-root list. |
| `origins` (request) | reference only | no | Origins this endpoint is called on. A non-empty list replaces the file-root list. A view-scoped request resolves against the **file root**, never its enclosing view — see [Semantics](#a-view-scoped-request-resolves-against-the-file-root-not-its-view). |

### Origin grammar

```
origin  = scheme "://" host [ ":" port ]
scheme  = "http" / "https"
host    = label *( "." label )
label   = ALPHA / DIGIT / (ALPHA / DIGIT) *( ALPHA / DIGIT / "-" ) (ALPHA / DIGIT)
port    = 1*5DIGIT           ; 1-65535, no leading zero

name    = LOWER *( LOWER / DIGIT / "_" )     ; e.g. `prod`, `local`, `auth`
```

`scheme` is lowercase. No userinfo, path, trailing slash, query, fragment, or wildcard label. IPv4
literals are admitted incidentally by the label grammar (each octet is a valid label); IPv6 bracket
literals are rejected in v1 (see [Open questions](#open-questions)). `origin` is the web-platform
origin tuple (RFC 6454) — a term of art with a precise, closed definition, chosen specifically so the
grammar has no room for the ambiguity a bare hostname or a full URL would introduce. `name` follows
the same pattern as `requestProperty.name` (`^[a-z][a-z0-9_]*$`).

`origin` appears exactly once per definition, as the value of a named definition's `url` — see
[Two entry forms: definitions and references](#two-entry-forms-definitions-and-references). It is
never valid as a freestanding `origins` array item; every array item, at every position, is either a
named-definition object or a bare `name`. The two grammars are mutually exclusive by construction
(`name` forbids `:` entirely; `origin` requires `scheme://`), which is what lets an SDK give a precise
error — "did you mean to define a named origin?" — when an author writes a literal URL where a
reference was expected, rather than a generic parse failure.

### Semantics

#### An origin is a tuple, not a URL

`origins` entries carry exactly scheme, host, and port — nothing a route, query string, or fragment
would add. This is deliberate: an origin's whole job is to be compared for equality against another
origin, and a closed three-part tuple is what makes that comparison well-defined (see
[Origin normalization and equality](#origin-normalization-and-equality)). It is also what makes
`origins[0] + route` directly navigable, and what makes a full `url:` value trivially reducible to
the origin it implies.

#### Two entry forms: definitions and references

An item in any `origins` array is exactly one of:

1. **A named definition** — an object `{name, url}`, where `url` is a literal per the
   [origin grammar](#origin-grammar) and `name` matches the name grammar. (This `url` is unrelated to
   a view's or request's own top-level `url:` field — see
   [`origins` and `url` are complementary](#origins-and-url-are-complementary).) Registers `name` for
   reference anywhere in the corpus. Valid **only** in a file-root `origins:` array — see
   [The origin registry is project-wide](#the-origin-registry-is-project-wide).
2. **A reference** — a bare string matching the name grammar. Resolved against the registry.

There is no freestanding-literal form: a bare origin string (e.g. `https://api.acme.com`) is never a
valid `origins` array item at any position, including the file root. Every URL a corpus writes down
lives inside exactly one named definition; every other mention of an origin is a reference. This
keeps the registry complete by construction — a tool enumerating "every origin this corpus knows
about" only ever has to read definitions, never also scan for stray inline literals — and means a
rotated hostname is a one-line edit instead of a multi-file find-and-replace (see
[Motivation](#motivation)).

No `{ $ref: Name }` wrapper object is needed for the reference form, unlike
[SEP-0002](0002-component-ref.md)'s component references: a component is already an object, so `$ref`
disambiguates one object shape from another, but a reference here is already a bare string, and at the
file root a definition and a reference are trivially distinguished by JSON type alone (object vs.
string) — no wrapper needed either way.

#### The origin registry is project-wide

Named definitions are collected from the file-root `origins:` array of **every** loaded file into one
project-wide registry — the same lookup-scope rule [SEP-0002](0002-component-ref.md) established for
components. A named definition is not accepted inside a view's or a request's own `origins:` override
(see [JSON Schema diff](#json-schema-diff)); only a file-root array can register a name, mirroring
components being addressable only when defined at file root, not inside `children:`. Two file-root
definitions across the corpus sharing a `name` are a collision: the first, ordered by source-file
path, wins, and a conforming SDK SHOULD warn. Diagnostic code: `origin-name-collision`.

#### Reference resolution

A conforming SDK MUST resolve every reference in a corpus's `origins` arrays to its registry entry's
`url` before applying
[Resolution: nearest non-empty list wins](#resolution-nearest-non-empty-list-wins) — from that point
on, an entity's origins are a plain list of literal values, regardless of whether each entry was
written as a definition or a reference. A reference that names no registry entry is invalid.
Diagnostic code: `origin-ref-unresolved`. Unlike component `$ref`, this is a single scalar
substitution, not a deep copy of a subtree: an origin definition has no children and cannot itself
contain a reference, so there is no expansion order to define and no possibility of a circular
reference.

#### Resolution: nearest non-empty list wins

An entity's effective origins are, in order: its own list if non-empty, else the file-root list, else
the empty (unconstrained) list — each already resolved per
[Reference resolution](#reference-resolution) before this rule applies. A per-entity list **replaces**
the file-root list; it does not merge with it. Merging would make *narrowing* impossible, and
narrowing is the primary reason a per-entity override exists — `SsoLogin` above is served **only**
from `auth.acme.com`, and under a merge rule it would still be considered served from `app.acme.com`
too. `origins` is a constraint set, whose union is always weaker than either input; that is the
opposite of what an author reaching for a per-entity override wants. This deliberately does **not**
follow [SEP-0004](0004-component-tags.md)'s union rule for `tags[]`, which is a collection of distinct
labels, not a constraint.

#### Absent or empty means unconstrained

`origins: []` is equivalent to omitting `origins` entirely — both mean "no constraint," identical to
the field never having existed. This matters because nothing else in the spec has a closed-empty-set
reading (an empty `tags`/`memory`/`components` list means "none declared," not "reject everything"),
and because an author accidentally emptying a list (a commented-out entry, a bad merge) should not
silently turn "matches everywhere" into "matches nowhere." The one place this interacts with
resolution: inheritance from the file root is broken only by a **non-empty** entity-level list. An
explicit `origins: []` on a view does not reset that view to "no origins" if the file root has any —
it is indistinguishable from that view declaring nothing at all.

#### A view-scoped request resolves against the file root, not its view

A `request` nested under `views[].requests[]` resolves its origins against the file root exactly as a
global request would — **never** against the origins of the view it's nested in. A page origin and an
API origin routinely differ (`app.acme.com` serving a page that calls `api.acme.com`), so inheriting
the enclosing view's origins would be wrong more often than right. This also keeps request resolution
identical regardless of where a request is declared, which matters because nothing else in the spec
treats a request's location (global vs. view-scoped) as semantically significant beyond grouping.

#### Origins are not a route-matching input

**A URL's origin has no bearing on whether it matches a view's or request's `route`.** Route matching
today is pathname-only, and this SEP does not change that: a URL whose origin is absent from an
entity's `origins` still matches that entity's `route` if the path does. Making `origins` a match
filter would silently reclassify existing traffic the moment an author adds an `origins:` list to a
previously host-agnostic entity — an additive-looking field change with breaking match-time
consequences. `origins` is normative *data about* an entity, consumed however a downstream tool
chooses (compiling a host-scoped filter, anchoring a generated regex, picking a navigation target);
it is not itself part of this spec's matching algorithm. An origin-aware, opt-in matching mode is
explicitly out of scope for this SEP — see [Open questions](#open-questions).

#### Origin normalization and equality

Two origins are equal iff their normalized forms are byte-equal: scheme and host lowercased, and an
explicit port matching the scheme's default (`:80` for `http`, `:443` for `https`) elided; any other
port is significant. This applies to already-*resolved* values (see
[Reference resolution](#reference-resolution)): two references to the same name are trivially equal,
and two references to *different* names are equal too if their definitions' `url`s share a normalized
form — the case `origin-duplicate` exists to catch (e.g. a view referencing both `prod` and a second,
redundantly-defined name for the same host). Origins are stored as authored — normalization applies
only when comparing or validating, not at load time — so a diagnostic quoting an origin quotes the
author's own spelling (mirroring how `route` is handled).

#### Declaration order is significant

An `origins` list is ordered, and a conforming SDK MUST NOT reorder, sort, or deduplicate it in
canonical output. This is unlike `dependencies`, which is sorted. Order matters because a consumer
selecting a single navigation target in the absence of any other signal treats `origins[0]` as the
default — see [Origin selection is a downstream concern](#origin-selection-is-a-downstream-concern).

#### `origins` and `url` are complementary

`url:` is one concrete, navigable address — possibly with a path and query a snapshot needs — and
`origins:` is the set of origins an entity is served from. They answer different questions and
neither subsumes the other. In particular, **`url:` does not imply an origin**: a consumer MUST NOT
synthesize `origins: [origin(url)]` when `origins` is absent. Auto-deriving would silently make every
existing corpus that has a `url:` single-origin, and would make "no `origins` declared" stop meaning
"unconstrained" the instant any `url:` is present — exactly the compatibility guarantee
[Migration](#migration) depends on. The two MAY disagree (a `url:` pointing through a redirecting
vanity host, or a corpus mid-migration); a conforming SDK SHOULD warn when they do, not reject.

#### Origin selection is a downstream concern

The spec enumerates origins; it does not mandate how a consumer chooses one for a given environment.
A CLI or tool selecting a single navigation target for an entity SHOULD apply, in order: an explicit
selector matching one of the entity's resolved origins (by registered name, or by value), else the
entity's own `url:`, else `origins[0]` joined with a literal (non-glob) `route`, else skip the entity.
An explicit selector that matches no origin anywhere in the corpus SHOULD fail loudly (an unrecognized
selector is more likely a typo than an intentional no-op) rather than silently produce no targets.

### Conformance

- MUST accept an `origins` list at the file root, on a `view`, and on a `request`.
- MUST accept a named definition (`{name, url}`) only in a file-root `origins:` array; MUST reject the
  same shape in a view- or request-level `origins:` array.
- MUST build one project-wide registry of named definitions from every loaded file's file-root
  `origins:` array.
- MUST resolve a bare-name reference against that registry before applying any inheritance or
  replacement rule. MUST reject a reference that names no registry entry. Diagnostic code:
  `origin-ref-unresolved`.
- SHOULD warn when two file-root definitions across the corpus share a `name`; the first, ordered by
  source-file path, wins. Diagnostic code: `origin-name-collision`.
- MUST resolve an entity's effective origins as its own non-empty list, else the file-root list, else
  the empty (unconstrained) list.
- MUST resolve a view-scoped request's origins against the file root, never against its enclosing
  view's origins.
- MUST treat `origins: []` as equivalent to an absent `origins`. SHOULD warn when `origins: []` is
  declared explicitly. Diagnostic code: `origins-empty`.
- MUST reject an `origins` entry that is neither a valid reference nor (at file root) a valid named
  definition — this includes a freestanding literal origin string (e.g. `https://api.acme.com`)
  written directly as an array item, which is valid only as the `url` inside a named definition.
  Diagnostic code: `origin-invalid`.
- MUST compare two (resolved) origins by their normalized forms (see
  [Origin normalization and equality](#origin-normalization-and-equality)). SHOULD warn on a duplicate
  within one resolved list. Diagnostic code: `origin-duplicate`.
- MUST preserve the declared order of an `origins` list; MUST NOT sort or deduplicate it in canonical
  output.
- **MUST NOT treat `origins` as an input to route matching.** A URL whose origin is absent from a
  view's `origins` still matches that view's `route`; a request URL whose origin is absent from a
  request's `origins` still matches that request's `route`.
- SHOULD warn when both `url` and `origins` resolve on the same view or request and the origin
  implied by `url` is absent from the resolved `origins` list. Diagnostic code: `origin-url-mismatch`.
- MUST NOT emit any diagnostic for a corpus that declares no `origins` anywhere — this is the status
  quo and must stay silent.
- MAY use `origins` to select a navigation target (see
  [Origin selection is a downstream concern](#origin-selection-is-a-downstream-concern)); this SEP
  requires no particular selection policy of an SDK that doesn't navigate.

### JSON Schema diff

- `$defs.origin`: **new** — a literal origin string, valid only as a named definition's `url`:
  ```
  $defs.origin:
    type: string
    pattern: "^https?://[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*(:[1-9][0-9]{0,4})?$"
    description: "A literal origin (scheme://host[:port])."
  ```
  The pattern bounds the port to 1-5 digits with no leading zero but does not itself reject a port
  above 65535 — that check is `origin-invalid`'s job, since expressing an upper numeric bound in
  `pattern` alone would make the regex unreadable for no real gain.
- `$defs.originName`: **new** — a bare name reference, and the only form a `view`/`request`
  `origins:` array item may take:
  ```
  $defs.originName:
    type: string
    pattern: "^[a-z][a-z0-9_]*$"
    description: "A bare name referencing a named origin defined in some file's file-root origins: array."
  ```
- `$defs.namedOrigin`: **new** — a file-root-only named definition:
  ```
  $defs.namedOrigin:
    type: object
    required: [name, url]
    additionalProperties: false
    properties:
      name: { $ref: "#/$defs/originName" }
      url: { $ref: "#/$defs/origin" }
    description: "Defines a named, project-wide-addressable origin. Valid only in a file-root origins: array."
  ```
- `$defs.rootOriginEntry`: **new** — what may appear in the file-root `origins:` array; the only
  position where a named definition is accepted, alongside a reference:
  ```
  $defs.rootOriginEntry:
    oneOf:
      - { $ref: "#/$defs/originName" }
      - { $ref: "#/$defs/namedOrigin" }
  ```
- `properties.origins` (root): **added**, optional — `{ type: array, items: { $ref: "#/$defs/rootOriginEntry" } }`.
  Root `required` is unchanged (`["version"]`).
- `$defs.view.properties.origins`: **added**, optional — `{ type: array, items: { $ref: "#/$defs/originName" } }`.
  No `oneOf` needed here: a view's `origins:` items are always references. `$defs.view.required`
  unchanged (`["name", "route"]`).
- `$defs.request.properties.origins`: **added**, optional — `{ type: array, items: { $ref: "#/$defs/originName" } }`.
  `$defs.request.required` unchanged (`["name", "route"]`).
- No removals, no type changes to any existing `$defs` entry.
- `fileRootFields`, `viewFields`, and `requestFields` (the Go SDK's unknown-field allowlists, which
  mirror the schema rather than the Go structs) each gain `"origins"`. No new top-level field *names*
  are introduced beyond `origins` itself — `name`/`url` live inside its value, not as sibling keys.

## Alternatives considered

### 1. Do nothing — keep host information out of the spec, pass it in out of band

The status quo: a consumer wanting a host takes it from a side channel (an options struct, a CLI
flag) rather than the corpus.

**Ruled out:** the information exists only in the corpus author's head and nowhere in the file; the
round trip is already asymmetric (a consumer's adopt path already synthesizes a `url:` from a page's
domain on the way *out* of a corpus, with no way back in); and the observable failure is a page
definition with no host, rendering as a bare path with a blank hostname control.

### 2. A single `host:` or `origin:` scalar

One string instead of a list.

**Ruled out:** every real corpus is served from more than one origin — production and local dev at an
absolute minimum, often an auth or API subdomain too. A scalar forces either one corpus file per
environment or a downstream override flag, which is exactly the gap this SEP closes. A downstream
consumer's own domain-matching structure is already a list ORed together, so a scalar would lose
expressivity right at the boundary where it's consumed.

### 3. Make `origins` a route-matching constraint

Filter `ViewForURL`/`RequestsForURL`-style lookups on origin in addition to path.

**Ruled out:** it silently breaks existing corpora the instant an author adds an `origins:` list,
since pathname-only matching is relied on by every consumer today. Deferred to a possible future SEP
introducing a separate, explicitly opt-in match entry point, so adopting origin-aware matching is a
call-site change rather than a corpus change.

### 4. Put origins in `.sightmap/config.yaml`

A single project-wide default origin (or origin set) in the non-spec config file instead of the
corpus itself.

**Ruled out:** origins are a per-entity property, not a project-wide constant — the `SsoLogin`
example above is served from a different origin than the rest of the app, and a request's API origin
routinely differs from its page's origin. `config.yaml` is also explicitly tooling-only and outside
the spec today. A config-level *default-origin selector for tooling* is a separate, compatible idea,
narrower now that naming solves the value-duplication half of the problem — see
[Open questions](#open-questions).

### 5. Reuse `url:` — allow a list of URLs and derive origins from it

Instead of a new field, let `url:` become an array and treat each entry's origin as implied.

**Ruled out:** conflates "a concrete address to navigate to" with "the set of origins this entity is
served from." A URL list would need to be parsed down to origins to be useful for the matching use
case, while a snapshot's `url:` legitimately points at a deep-linked state whose path is not the
entity's route — that shape doesn't generalize to a list without ambiguity about which entry, if any,
is authoritative for navigation.

### 6. Wrap references in a `{ $ref: Name }` object, mirroring SEP-0002 exactly

Use the same object-wrapper shape [SEP-0002](0002-component-ref.md) uses for components, instead of a
bare name string.

**Ruled out:** an origin item is already a scalar — unlike a component, which is always an object and
needs an object shape to disambiguate a reference from a definition. The origin grammar's mandatory
`scheme://` prefix already gives an unambiguous, lower-ceremony discriminator between a literal and a
name; wrapping a reference in an object would add a syntax layer the type doesn't need.

### 7. Also allow a freestanding literal alongside named definitions

Let an `origins:` array item be a literal origin *or* a named definition *or* a reference — the
three-form design this SEP started from, so even a single-file, single-origin corpus could write
`origins: [https://api.acme.com]` directly, with naming as a purely optional upgrade.

**Ruled out:** it reintroduces exactly the failure mode this SEP exists to close — a URL written down
somewhere other than a named definition is invisible to the registry, so a tool enumerating "every
origin this corpus knows about" has to also scan for stray literals, and nothing stops a second file
from writing the same literal instead of referencing the name that already exists for it. Mandatory
naming costs a few extra lines in the single-origin case (see
[Open questions](#open-questions)); the alternative reopens the multi-file duplication this SEP was
written to close.

## Migration

`origins` is additive: no existing corpus declares it, and this SEP does not change route matching for
a corpus that does — see
[Origins are not a route-matching input](#origins-are-not-a-route-matching-input). `url:` is not
deprecated. The registry and reference-resolution rules are new semantic surface with no prior art in
this field to migrate from; they carry the same purely-additive story as the rest of this SEP.

Under `additionalProperties: false` at three positions (root, `$defs.view`, `$defs.request`), an SDK
pinned before this SEP ships rejects any corpus using `origins` at all three. The Go SDK's
unknown-field allowlists (`fileRootFields`, `viewFields`, `requestFields`) need the same addition, or
a conforming corpus produces spurious `unknown-field` warnings against an older SDK. Canonical key
order gains `origins` at the top level, on `view`, and on `request`; since no existing file declares
the key, no existing file is reformatted by this change. `origins` is explicitly exempt from the
sorted-string-array rule that governs `dependencies`, because its order is significant.

Release playbook, matching SEP-0005/SEP-0006's pattern:

1. This SEP merges (status: Accepted).
2. `sightmap/sightmap` implements schema, registry-building, reference resolution, and validation.
   Bumped to a minor release.
3. Consumers pin `github.com/sightmap/sightmap/go >= <new version>` before authoring `origins:`.
4. Corpora adopt a file-root `origins:` per file, defining each origin once (wherever it naturally
   belongs) and referencing it by name everywhere else, with per-entity overrides only where an
   entity genuinely differs.
5. A consumer that takes a host out of band today (an options override) reads the resolved `origins`
   instead, keeping its own override as a pure escape hatch rather than the only source.

## Open questions

1. **Wildcard host labels.** Rejected in v1 (see [Origin grammar](#origin-grammar)); route-matching
   `*`/`**` already means something for `/`-separated path segments matched left-to-right, and reusing
   the same glyph for `.`-separated host labels matched right-to-left is a likely source of author
   confusion. If added later, it should be a distinct, right-anchored form (e.g. `**.example.com`),
   not `*`.
2. **Origin-aware matching as an opt-in.** What a future, explicitly-opt-in match entry point looks
   like (a parallel function, an options parameter), and whether it would need to cover requests as
   well as views. Not proposed here; see [Alternatives considered #3](#3-make-origins-a-route-matching-constraint).
3. **Should declaration order really be load-bearing?** Making `origins` ordered (for first-wins
   navigation-target selection) also makes it unsortable in canonical output, unlike every other
   string-array field. The alternative is an unordered, canonically-sorted list plus a mandatory
   explicit selector for any tool that needs to pick one. Open to reviewer pushback on which cost is
   worse.
4. **A default-origin selector in `.sightmap/config.yaml`.** Naming solves *value* duplication — a URL
   is written once, referenced by name — but a file still has to declare *which* names apply by
   default (`origins: [prod, local]` at file root) in every file that needs them. Whether a
   corpus-wide default *name set*, layered on top of the registry, belongs in `config.yaml` is still
   open. `config.yaml` today has only a `version` field.
5. **Is file-root-only inheritance right for a view-scoped request?** This SEP says a view-scoped
   request never inherits its enclosing view's origins (see
   [Semantics](#a-view-scoped-request-resolves-against-the-file-root-not-its-view)) because API and
   page origins routinely differ. It's the least obvious rule in this proposal and the one most worth
   a second opinion.
6. **IPv6 literals.** Rejected in v1; the grammar has no bracketed form. Revisit if a real corpus
   needs one.
7. **Should `url:` become derivable?** For a fully-literal `route` (no glob segments), `origins[0] +
   route` could in principle stand in for an unset `url:`. Not proposed here — see
   [`origins` and `url` are complementary](#origins-and-url-are-complementary).
8. **Should named definitions really be restricted to file root?** This mirrors
   [SEP-0002](0002-component-ref.md)'s restriction on components, but components have an established
   reason for it (nesting under `children:`) that an origin — a flat scalar with no nesting at all —
   doesn't obviously share. Worth confirming the restriction earns its keep here rather than being
   imported from precedent by default.
9. **Mandatory naming taxes the trivial case.** A single-file corpus with one origin must still write
   a `{name, url}` block instead of a bare literal (see
   [Alternatives considered #7](#7-also-allow-a-freestanding-literal-alongside-named-definitions)).
   That's a real, deliberate cost for a small corpus in exchange for keeping the registry complete by
   construction as the corpus grows; worth a reviewer's opinion on whether it's worth paying up front
   rather than, say, only once a second file exists.

## References

- [`spec/v1/schema.md#route-matching`](../v1/schema.md#route-matching) — the pathname-only matching
  rule this SEP deliberately does not change.
- [SEP-0002](0002-component-ref.md) — the direct precedent for a named, project-wide registry resolved
  by reference; this SEP's registry-building and first-seen-wins collision rule mirror it exactly,
  adapted for a scalar value instead of an object subtree.
- [SEP-0004](0004-component-tags.md) — the union-resolution precedent for a list field that this SEP
  deliberately does not follow, since `origins` is a constraint set rather than a collection of
  distinct labels.
- [SEP-0005](0005-request-properties.md) / [SEP-0006](0006-message-entity.md) — the house style this
  SEP's Semantics/Conformance/JSON Schema diff sections and Migration playbook follow.
- [SEP-0008](0008-view-route-params.md) — also extends `$defs.view` and the Go SDK's `viewFields`
  allowlist; textual overlap on those two files, no semantic conflict.
- RFC 6454 (*The Web Origin Concept*) — source of the scheme/host/port tuple this SEP's grammar
  adopts.
