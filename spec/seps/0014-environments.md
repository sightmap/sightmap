---
sep: 0014
title: Named environments, referenced by views and requests
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-09-08
updated: 2026-09-23
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add an `environments` field in two roles. At a file root it defines named environments,
`{name, value, platform?, build_type?}`, where `value` is the identifier a running app reports about
itself (a hostname on web, a bundle ID or application ID on native) and `build_type` separates build
variants that share one identifier. On a view or a request it is a list of names referencing those
definitions: the environments a view is served on, or an endpoint is called on. Definitions from
every file form one project-wide registry, so an identifier is written once and referenced by name
everywhere else. Environments are declarative: they don't change route matching, and a view or
request that references none declares no constraint.

## Motivation

A sightmap corpus says what a page or endpoint *is*, but nothing about *where* it runs. The only
host-bearing field, `url:`, is a single optional navigation target rather than an identity, and many
corpora never set it. Tools built on a corpus keep needing the missing piece:

1. **A compiled page has no host.** A consumer that compiles views into host-scoped page definitions
   derives each host from `url:`. With no `url:`, every compiled page matches any host.
2. **A request matcher can't be anchored.** `request.route` is path-only. A consumer turning it into
   a matcher against full URLs has no host to anchor on, so the matcher accepts the same path on any
   host.
3. **A corpus spans several apps.** One corpus often describes a web app and its iOS and Android
   counterparts, each released independently. A tool that publishes a corpus per deploy target
   needs a stable name for each target (`staging`, `ios-beta`) and the identifier that target
   reports.
4. **Not everything runs everywhere.** An SSO page is served only from `auth.acme.com`, and the
   app's API is called on `api.acme.com`, not on the host serving the page. Where-it-runs has to
   attach to individual views and requests, not only to the corpus as a whole.

Two properties of real identifiers shape the design. Web and native don't share an identifier: web
reports a hostname, native reports a bundle or application ID, and a URL-shaped field fits only the
first. And an identifier alone can under-determine the environment: an iOS beta and its release
build ship under one bundle ID, so only the build variant tells them apart.

## Proposal

### Shape

Definitions live at a file root, in any file. `environments.yaml` is a convention, not a rule:

```yaml
# .sightmap/environments.yaml
version: 1
environments:
  - { name: api,          value: api.acme.com }
  - { name: auth,         value: auth.acme.com }
  - { name: local,        value: app.acme.test }
  - { name: prod,         value: app.acme.com }
  - { name: staging,      value: app.staging.acme.com }

  - { name: android-prod, platform: android, value: com.acme.app, build_type: release }
  - { name: ios-beta,     platform: ios,     value: com.acme.app, build_type: beta }
  - { name: ios-prod,     platform: ios,     value: com.acme.app, build_type: release }
```

Views and requests in any file reference them by name:

```yaml
# .sightmap/account.yaml
version: 1
views:
  - name: OrderHistory
    route: "/ui/*/settings/orders/history"
    environments: [local, prod, staging]
  - name: SsoLogin
    route: /sso/login
    environments: [auth]
    requests:
      - name: SsoCallback
        route: /sso/callback
        method: POST
        environments: [api]          # its own list; SsoLogin's [auth] is not inherited
requests:
  - name: GetProfile
    route: /settings/profile
    method: GET
    environments: [api]
  - name: Ping
    route: /api/ping
    method: GET                      # no environments: no constraint declared
```

`ios-beta` and `ios-prod` share `com.acme.app` and differ only by `build_type`. Native entries are
referenced the same way as web entries.

### Field reference

| Position | Items | Description |
|---|---|---|
| `environments` (file root) | environment definitions | Defines named environments and registers each `name` project-wide. |
| `environments` (view) | names | Environments this view is served on. |
| `environments` (request) | names | Environments this endpoint is called on. |

An environment definition:

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string, `^[a-z][a-z0-9_-]*$` | yes | (none) | Unique across the corpus. What views, requests, and tools refer to. |
| `platform` | `web` \| `ios` \| `android` | no | `web` | Which kind of identifier `value` is. Stated rather than inferred, because an application ID like `com.acme.app` is also a valid hostname. |
| `value` | string | yes | (none) | The identifier the app reports: a hostname on web, a bundle ID on iOS, an application ID on Android. No scheme, port, or path. |
| `build_type` | string | no | (none) | Build variant (`release`, `beta`, `debug`). Separates environments that share one `value`. Native only. |

A web-only corpus writes `{name, value}` and never sees `platform` or `build_type`.

### Semantics

#### Definitions and references

Items in a file-root `environments` array are definitions (objects). Items in a view's or request's
`environments` array are references (bare names). A definition is never valid on a view or request,
and a bare name is never valid at a file root. Every identifier lives in exactly one definition, so
rotating a hostname is a one-line edit, and a tool listing every environment in a corpus reads
definitions only.

#### The registry is project-wide

Definitions from every loaded file form one registry, the same lookup scope
[SEP-0002](0002-component-ref.md) uses for components. When two definitions share a `name`, the one
from the first file by source path wins, and a conforming SDK SHOULD warn
(`environment-name-collision`).

#### Reference resolution

Every reference MUST resolve to a registry entry; a reference that names no entry is invalid
(`environment-ref-unresolved`). Resolution is a scalar lookup: a definition contains no references,
so there is no expansion order and no possibility of a cycle.

#### Absent or empty means unconstrained

A view or request with no `environments`, or with `environments: []`, declares no constraint. It
does not mean "runs nowhere": an accidentally emptied list should not flip an entity from matching
everywhere to matching nothing. A conforming SDK SHOULD warn on an explicit empty list
(`environments-empty`).

#### No inheritance

A view's or request's list is its own. A view-scoped request does not inherit its enclosing view's
list, because a page and the API it calls routinely run on different hosts (`SsoLogin` on `auth`,
`SsoCallback` on `api` above). Definitions at a file root do not become defaults for that file's
views and requests either; defining `auth` in a file says nothing about where that file's views run.

#### Not a route-matching input

A URL matches a view or request by path alone, as it does today. A session whose identifier matches
none of an entity's environments still matches that entity's route. Making environments a match
filter would silently reclassify existing traffic the moment an author adds a list to a previously
unconstrained entity. Environments are data about an entity for consumers to use as they need:
compiling host-scoped page definitions, anchoring request matchers, choosing a publish target by
name.

#### Identifiers

A web `value` is a hostname and compares case-insensitively. A native `value` is a bundle ID or
application ID and compares byte-for-byte. A definition with no `build_type` covers every build
variant of its `value`; a definition with one covers only that variant. Two definitions with the
same `platform`, `value`, and `build_type` describe the same environment under two names, and a
conforming SDK SHOULD warn (`environment-duplicate`).

#### `environments` and `url` are complementary

`url:` is one concrete, navigable address. `environments` is the set of places an entity runs.
`url:` does not imply an environment, and a consumer MUST NOT derive one from it: doing so would
make every corpus with a `url:` constrained, and "no `environments` declared" would stop meaning
"unconstrained" for every existing corpus.

### Conformance

- MUST accept `environments` at the file root (definitions), on a view (references), and on a
  request (references).
- MUST reject a definition on a view or request, and a bare name at a file root.
- MUST build one project-wide registry from every loaded file's file-root definitions. SHOULD warn
  when two definitions share a `name`; the first by source-file path wins
  (`environment-name-collision`).
- MUST reject a reference that names no registry entry (`environment-ref-unresolved`).
- MUST reject a definition whose `value` isn't a valid hostname (web) or dot-separated identifier
  (native), or that sets `build_type` with `platform: web` (`environment-invalid`).
- MUST treat an absent or empty view- or request-level list as unconstrained. SHOULD warn on an
  explicit `environments: []` (`environments-empty`).
- MUST NOT inherit a view's list into its view-scoped requests, or a file's definitions into its
  views and requests.
- SHOULD warn when two definitions share `platform`, `value`, and `build_type`
  (`environment-duplicate`).
- **MUST NOT treat `environments` as an input to route matching.**
- MUST NOT derive an environment from `url:`.
- MUST NOT emit any diagnostic for a corpus that declares no `environments` anywhere.

### JSON Schema diff

- `$defs.environmentName`: **new**.
  ```
  $defs.environmentName:
    type: string
    pattern: "^[a-z][a-z0-9_-]*$"
  ```
- `$defs.environment`: **new**. The `value` pattern admits both hostnames and native identifiers;
  the stricter per-platform check is `environment-invalid`'s job.
  ```
  $defs.environment:
    type: object
    required: [name, value]
    additionalProperties: false
    properties:
      name: { $ref: "#/$defs/environmentName" }
      platform: { type: string, enum: [web, ios, android], default: web }
      value: { type: string, pattern: "^[A-Za-z0-9_-]+(\\.[A-Za-z0-9_-]+)*$" }
      build_type: { type: string }
  ```
- `properties.environments` (root): **added**, optional,
  `{ type: array, items: { $ref: "#/$defs/environment" } }`. Root `required` unchanged.
- `$defs.view.properties.environments`: **added**, optional,
  `{ type: array, items: { $ref: "#/$defs/environmentName" } }`. `$defs.view.required` unchanged.
- `$defs.request.properties.environments`: **added**, optional,
  `{ type: array, items: { $ref: "#/$defs/environmentName" } }`. `$defs.request.required` unchanged.
- No removals, no type changes to existing `$defs` entries.
- The Go SDK's unknown-field allowlists `fileRootFields`, `viewFields`, and `requestFields` each
  gain `"environments"`.

### Canonical format

- Key order gains `environments` after `version` at the top level, after `route` on a view, and
  after `method` on a request.
- An environment definition's key order is `name, platform, value, build_type`.
- The top-level `environments` sequence is alphabetized by `name`, like `views`.
- View- and request-level reference lists carry no order, so they are sorted and deduplicated like
  `dependencies`.

## Alternatives considered

### 1. Do nothing: pass hosts to each tool out of band

A consumer takes a host from a command-line flag or options struct instead of the corpus.

**Ruled out:** the information lives only in the author's head, every tool re-asks for it, and a
single flag can't express per-entity differences like an SSO page on its own host or an API on
another.

### 2. Inline definitions on views and requests, no registry

Let each view or request write `{value, platform, build_type}` directly.

**Ruled out:** a corpus split across many files repeats the same identifier in every one, so rotating
a hostname becomes a multi-file find-and-replace with nothing keeping the copies in sync. Names also
give publishing tools a stable handle (`--env staging`) independent of any one view.

### 3. A URL-shaped field (`scheme://host[:port]`)

Declare each environment as a URL origin.

**Ruled out:** a bundle ID or application ID has no URL reading, and an origin has no place for a
build variant, so two native environments sharing one bundle ID would collapse into one.

### 4. Infer `platform` from `value`

Guess web or native from whether `value` looks like a hostname.

**Ruled out:** `com.acme.app` is a valid hostname, so the guess would silently pick the wrong
identifier space, with no way for an author to correct it short of renaming the app.

### 5. File-root definitions double as defaults for that file

Treat a file's definitions as the environments of every view and request in that file.

**Ruled out:** it conflates defining an environment with applying it. A file that defines `auth`
would silently constrain every view in it to `auth`. An explicit file-level default, separate from
definitions, is an open question below.

### 6. Make environments a route-matching constraint

Filter view and request lookups on the session's identifier as well as the path.

**Ruled out:** every consumer relies on path-only matching today, and an author adding a list to an
entity would silently change what traffic it matches. An opt-in matching mode could come in a later
SEP as a separate entry point, so adopting it is a call-site change rather than a corpus change.

## Migration

`environments` is additive: no existing corpus declares it, so no existing file changes meaning or
formatting. Under `additionalProperties: false` at the root, `$defs.view`, and `$defs.request`, an
SDK pinned before this SEP rejects any corpus that uses the field, so consumers pin an SDK version
that implements it before authoring `environments`. The Go SDK's `fileRootFields`, `viewFields`,
and `requestFields` allowlists need the same addition, or a conforming corpus produces spurious
`unknown-field` warnings.

## Open questions

1. **A file-level default.** A file whose every view runs on `[local, prod, staging]` repeats that
   list per view. An explicit default, separate from definitions (see alternative 5), would remove
   the repetition. Is it worth a second concept?
2. **Hostname wildcards.** Ephemeral preview hosts (`pr-123.preview.acme.com`) can't be enumerated.
   If wildcards are added, they should be a distinct, right-anchored form (`**.preview.acme.com`),
   not route matching's `*`, which already means something for `/`-separated path segments.
3. **Ports.** `value` forbids a port, so a local environment on a non-default port is identified by
   hostname alone. Is that ever ambiguous in practice?
4. **Opt-in environment-aware matching.** What a separate match entry point would look like, and
   whether it covers requests as well as views.

## References

- [`spec/v1/schema.md#route-matching`](../v1/schema.md#route-matching): the path-only matching rule
  this SEP does not change.
- [SEP-0002](0002-component-ref.md): the project-wide registry and first-by-path collision rule this
  SEP reuses for a scalar definition instead of a component subtree.
- [SEP-0001](0001-dependencies-field.md): the sort-and-deduplicate canonical rule reused for
  reference lists.
