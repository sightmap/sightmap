---
sep: 0014
title: Named environments and origins, referenced by views and requests
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-09-08
updated: 2026-09-23
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add a file-root `environments` registry of named deploy targets, and let views and requests
reference environments and origins by name. A web environment is defined by its origins: a map of
surface names (`app`, `auth`, `api`) to the `scheme://host[:port]` URL each surface has in that
environment. A native environment is defined by the bundle ID or application ID its app reports,
plus an optional build variant. A view lists the environments it exists in and the origins it is
served from; a request lists the origins it is called on. An origin name resolves per environment,
so `origins: [auth]` means the auth host in whichever environment a session belongs to. Both
references are declarative: neither changes route matching, and an entity that references none
declares no constraint.

## Motivation

A sightmap corpus says what a page or endpoint *is*, but nothing about *where* it runs. The only
host-bearing field, `url:`, is a single optional navigation target rather than an identity, and many
corpora never set it. Tools built on a corpus keep needing the missing piece:

1. **A compiled page has no host.** A consumer that compiles views into host-scoped page definitions
   derives each host from `url:`. With no `url:`, every compiled page matches any host.
2. **A request matcher can't be anchored.** `request.route` is path-only. A consumer turning it into
   a matcher against full URLs has no host to anchor on.
3. **A corpus spans several apps and deploy targets.** One corpus often describes a web app and its
   iOS and Android counterparts, each released independently through local, staging, and production.
   A tool that publishes a corpus per deploy target needs a stable name for each one and a way to
   tell which target a session belongs to.
4. **Not every page is served from the app's host.** An SSO page is served from an auth host, and
   the app's API is called on an API host. Each of those hosts also differs per deploy target:
   `auth.acme.com` in production, `auth.staging.acme.com` in staging.

A deploy target (`staging`, `ios-beta`) and a surface (`auth`, `api`) are different things. A session
belongs to a deploy target, and a corpus is published to one. A surface is a host within a deploy
target: the same surface in two targets is the same part of the product on two hosts, and an API
surface has no pages or sessions of its own. Defining surfaces inside each deploy target captures
both facts: `auth` is one name, and each environment says what its auth host is.

Native deploy targets have no origins. Their identifier is a bundle or application ID, and an
identifier alone can under-determine the target: an iOS beta and its release build ship under one
bundle ID, so only the build variant tells them apart.

## Proposal

### Shape

Environments are defined at a file root, in any file:

```yaml
# .sightmap/environments.yaml
version: 1
environments:
  - name: local
    origins:
      api:  https://api.acme.test:8443
      app:  https://app.acme.test:8443
      auth: https://auth.acme.test:8443
  - name: prod
    origins:
      api:  https://api.acme.com
      app:  https://app.acme.com
      auth: https://auth.acme.com
  - name: staging
    origins:
      api:  https://api.staging.acme.com
      app:  https://app.staging.acme.com
      auth: https://auth.staging.acme.com

  - { name: android-prod, platform: android, value: com.acme.app, build_type: release }
  - { name: ios-beta,     platform: ios,     value: com.acme.app, build_type: beta }
  - { name: ios-prod,     platform: ios,     value: com.acme.app, build_type: release }
```

Views and requests in any file reference environments and origins by name:

```yaml
# .sightmap/account.yaml
version: 1
views:
  - name: BetaDashboard
    route: "/ui/*/beta/dashboard"
    environments: [local, staging]     # not shipped to prod
  - name: OrderHistory
    route: "/ui/*/settings/orders/history"
    origins: [app]
  - name: SsoLogin
    route: /sso/login
    origins: [auth]
    requests:
      - name: SsoCallback
        route: /sso/callback
        method: POST
        origins: [api]                 # its own list; SsoLogin's [auth] is not inherited
requests:
  - name: GetProfile
    route: /settings/profile
    method: GET
    origins: [api]
```

Read against the definitions: `SsoLogin` is served from `https://auth.staging.acme.com` in staging
and `https://auth.acme.com` in prod. `GetProfile` is called on the API host of whichever
environment the session belongs to. `BetaDashboard` exists only in local and staging, on no
particular surface. A session on `app.staging.acme.com` or `auth.staging.acme.com` belongs to
`staging`.

### Field reference

| Position | Field | Items | Description |
|---|---|---|---|
| file root | `environments` | environment definitions | Defines deploy targets; registers each environment `name` and each origin name project-wide. |
| view | `environments` | environment names | Deploy targets this view exists in. |
| view | `origins` | origin names | Surfaces this view is served from. |
| request | `environments` | environment names | Deploy targets this endpoint exists in. |
| request | `origins` | origin names | Surfaces this endpoint is called on. |

An environment definition:

| Field | Type | Applies to | Required | Description |
|---|---|---|---|---|
| `name` | string, `^[a-z][a-z0-9_-]*$` | all | yes | Unique among environments. |
| `platform` | `web` \| `ios` \| `android` | all | no, default `web` | Which kind of deploy target this is. |
| `origins` | map of origin name to URL | web | yes | Each surface's URL in this environment. Names match `^[a-z][a-z0-9_-]*$`; URLs are `scheme://host[:port]` with `http` or `https` and no path, query, or fragment. |
| `value` | string | native | yes | The bundle ID (iOS) or application ID (Android) the app reports. |
| `build_type` | string | native | no | Build variant (`release`, `beta`, `debug`). Separates environments that share one `value`. |

A web environment MUST NOT set `value` or `build_type`; a native environment MUST NOT set `origins`.
A web corpus with a single host per environment writes one origin per environment
(`origins: { app: https://app.acme.com }`).

### Semantics

#### Environments and origins are separate axes

An environment answers "which deploy target"; an origin answers "which surface within it". A view
can reference either, both, or neither: `environments: [staging]` alone says the view exists only in
staging, and `origins: [auth]` alone says it is served from the auth surface wherever it exists. With
both, the view is served from auth in staging only.

#### Definitions and references

Environments, and the origins inside them, are defined only in a file-root `environments` array.
A view's or request's `environments` and `origins` arrays hold bare names. A definition is never
valid on a view or request, and a bare name is never valid at a file root. Every URL and identifier
lives in exactly one place, so rotating a hostname is a one-line edit, and a tool listing a corpus's
deploy targets and surfaces reads the environment definitions only.

#### The registry is project-wide

Environment definitions from every loaded file form one registry, the same lookup scope
[SEP-0002](0002-component-ref.md) uses for components. Two definitions sharing a `name` collide: the
one from the first file by source path wins, and a conforming SDK SHOULD warn
(`environment-name-collision`). The set of origin names is the union of the origin names across
every web environment.

#### Origins resolve per environment

An origin reference resolves, for a given web environment, to that environment's URL for the name.
A conforming SDK SHOULD warn when an origin name is defined in some web environments but not others
(`origin-environment-gap`): an entity served from that surface has no URL in the rest. Origins have
no meaning in a native environment; an entity's `origins` simply don't resolve there.

#### Which environment a session belongs to

A web session belongs to the environment that defines an origin with the session's hostname. Each
hostname MUST therefore appear in at most one environment (`origin-host-collision`). A native
session belongs to the environment whose `platform` and `value` match its app, preferring one whose
`build_type` matches the session's build variant; an environment with no `build_type` covers every
variant of its `value`. Two native environments with the same `platform`, `value`, and `build_type`
are the same target under two names, and a conforming SDK SHOULD warn (`environment-duplicate`).

Web hostnames compare case-insensitively; native identifiers compare byte-for-byte. Origin URLs
compare after lowercasing the scheme and host and dropping a port equal to the scheme's default.

#### Reference resolution

Every environment reference MUST name a defined environment (`environment-ref-unresolved`). Every
origin reference MUST name an origin defined in at least one web environment
(`origin-ref-unresolved`). Resolution is a lookup: definitions contain no references, so there is no
expansion order and no possibility of a cycle.

#### Absent or empty means unconstrained

A view or request with no `environments` exists in every environment; with no `origins`, it declares
no surface. An empty list, `[]`, means the same as an absent one: an accidentally emptied list should
not flip an entity from matching everywhere to matching nothing. A conforming SDK SHOULD warn on an
explicit empty list (`environments-empty`, `origins-empty`).

#### No inheritance

A view's or request's lists are its own. A view-scoped request does not inherit its enclosing view's
lists, because a page and the API it calls routinely live on different surfaces (`SsoLogin` on
`auth`, `SsoCallback` on `api` above).

#### Not a route-matching input

A URL matches a view or request by path alone, as it does today. A session outside an entity's
environments, or a URL outside its origins, still matches that entity's route. Making either a match
filter would silently reclassify existing traffic the moment an author adds a list to a previously
unconstrained entity. Environments and origins are data about an entity for consumers to use as
they need: choosing a publish target, compiling host-scoped page definitions per environment,
anchoring request matchers.

#### `url` is complementary

`url:` is one concrete, navigable address. `environments` and `origins` describe where an entity
runs. `url:` implies neither, and a consumer MUST NOT derive an environment or origin from it: doing
so would constrain every existing corpus that sets `url:`.

### Conformance

- MUST accept `environments` at the file root (definitions), and `environments` and `origins` on a
  view or request (names).
- MUST reject a definition on a view or request, and a bare name at a file root.
- MUST reject a web environment without `origins` or with `value` or `build_type`, and a native
  environment without `value` or with `origins` (`environment-invalid`).
- MUST reject an origin URL that isn't `scheme://host[:port]` (`origin-invalid`), and a native
  `value` that isn't a dot-separated identifier (`environment-invalid`).
- MUST build one project-wide environment registry keyed by `name`. SHOULD warn on a collision; the
  first by source-file path wins (`environment-name-collision`).
- MUST reject a hostname defined by origins in more than one environment (`origin-host-collision`).
- MUST reject an environment reference that names no environment (`environment-ref-unresolved`),
  and an origin reference that no web environment defines (`origin-ref-unresolved`).
- SHOULD warn when an origin name is defined in some web environments but not others
  (`origin-environment-gap`).
- SHOULD warn when two native environments share `platform`, `value`, and `build_type`
  (`environment-duplicate`).
- MUST treat an absent or empty view- or request-level list as unconstrained. SHOULD warn on an
  explicit `[]` (`environments-empty`, `origins-empty`).
- MUST NOT inherit a view's lists into its view-scoped requests.
- **MUST NOT treat `environments` or `origins` as an input to route matching.**
- MUST NOT derive an environment or origin from `url:`.
- MUST NOT emit any diagnostic for a corpus that declares neither field anywhere.

### JSON Schema diff

- `$defs.definitionName`: **new**, shared by environment and origin names.
  ```
  $defs.definitionName:
    type: string
    pattern: "^[a-z][a-z0-9_-]*$"
  ```
- `$defs.originUrl`: **new**. The pattern bounds the port to 1-5 digits with no leading zero;
  rejecting a port above 65535 is `origin-invalid`'s job.
  ```
  $defs.originUrl:
    type: string
    pattern: "^https?://[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*(:[1-9][0-9]{0,4})?$"
  ```
- `$defs.environment`: **new**. The web/native split is a conditional on `platform`.
  ```
  $defs.environment:
    type: object
    required: [name]
    additionalProperties: false
    properties:
      name: { $ref: "#/$defs/definitionName" }
      platform: { type: string, enum: [web, ios, android], default: web }
      origins:
        type: object
        minProperties: 1
        propertyNames: { $ref: "#/$defs/definitionName" }
        additionalProperties: { $ref: "#/$defs/originUrl" }
      value: { type: string, pattern: "^[A-Za-z0-9_-]+(\\.[A-Za-z0-9_-]+)*$" }
      build_type: { type: string }
    if: { required: [platform], properties: { platform: { enum: [ios, android] } } }
    then: { required: [value], not: { required: [origins] } }
    else: { required: [origins], not: { anyOf: [{ required: [value] }, { required: [build_type] }] } }
  ```
- `properties.environments` (root): **added**, optional,
  `{ type: array, items: { $ref: "#/$defs/environment" } }`.
- `$defs.view.properties.environments`, `$defs.view.properties.origins`,
  `$defs.request.properties.environments`, `$defs.request.properties.origins`: **added**, optional,
  each `{ type: array, items: { $ref: "#/$defs/definitionName" } }`.
- No `required` changes, no removals, no type changes to existing `$defs` entries.
- The Go SDK's unknown-field allowlists: `fileRootFields` gains `"environments"`; `viewFields` and
  `requestFields` each gain `"environments"` and `"origins"`.

### Canonical format

- Key order gains `environments` after `version` at the top level, and `environments, origins`
  after `route` on a view and after `method` on a request.
- An environment definition's key order is `name, platform, origins, value, build_type`. Keys
  within `origins` are alphabetized.
- The top-level `environments` sequence is alphabetized by `name`.
- View- and request-level reference lists carry no order, so they are sorted and deduplicated like
  `dependencies`.

## Alternatives considered

### 1. Do nothing: pass hosts to each tool out of band

A consumer takes a host from a command-line flag or options struct instead of the corpus.

**Ruled out:** the information lives only in the author's head, every tool re-asks for it, and a
single flag can't express per-entity differences like an SSO page on its own host.

### 2. Surfaces as environments

Define `api` and `auth` as environments alongside `prod` and `staging`, each with one host.

**Ruled out:** they aren't deploy targets. An API host has no pages and no sessions, so publishing a
corpus to it or asking which sessions belong to it means nothing, and one flat list can't say that
`auth` in staging and `auth` in prod are the same surface on different hosts.

### 3. A top-level origin registry that environments reference

Define every URL as a flat, named origin (`app-prod`, `auth-staging`) and have each web environment
point at its origins.

**Ruled out:** each surface needs one name per environment, so a view served from auth lists
`origins: [auth-local, auth-prod, auth-staging]` and has to be edited whenever an environment is
added. Keying origins by surface inside each environment keeps a view's reference to `[auth]`.

### 4. A top-level origin registry qualified by environment

Define origins at the top level as `{name, environment, url}`, several definitions per name.

**Ruled out:** a web environment's identity would be split across its own entry and every origin
entry that names it, and origins would reference environments while environments carry nothing,
so reading one deploy target means scanning the whole origin list. Nesting origins inside the
environment keeps each deploy target in one place.

### 5. A single `value` hostname for web environments

Identify a web environment by one hostname, like a native environment's bundle ID.

**Ruled out:** a web deploy target spans several hosts, and a session on any of them (the app host,
the auth host) belongs to it. The origin map already names every host, so a separate `value` would
repeat one of them.

### 6. Origins only, no environments

Define every host as an origin and drop environments.

**Ruled out:** a native app has no origin, and there is nowhere to put a build variant, so two
native targets sharing one bundle ID would collapse into one.

### 7. Make environments or origins a route-matching constraint

Filter view and request lookups on environment or origin as well as path.

**Ruled out:** every consumer relies on path-only matching today, and an author adding a list to an
entity would silently change what traffic it matches. An opt-in matching mode could come in a later
SEP as a separate entry point, so adopting it is a call-site change rather than a corpus change.

## Migration

Both fields are additive: no existing corpus declares either, so no existing file changes meaning or
formatting. Under `additionalProperties: false` at the root, `$defs.view`, and `$defs.request`, an
SDK pinned before this SEP rejects any corpus that uses them, so consumers pin an SDK version that
implements this SEP before authoring them. The Go SDK's allowlists need the same additions, or a
conforming corpus produces spurious `unknown-field` warnings.

## Open questions

1. **A default surface.** A view with no `origins` declares no surface, so a consumer compiling
   host-scoped pages has to pick one itself. Should an environment mark one origin as the default
   (say, `app`), or should consumers be left to choose?
2. **A file-level default.** A file whose every view is served from `auth` repeats `origins: [auth]`
   per view. An explicit file default would remove the repetition. Is it worth a second concept?
3. **Hostname wildcards.** Ephemeral preview hosts (`pr-123.preview.acme.com`) can't be enumerated.
   If wildcards are added, they should be a distinct, right-anchored form (`**.preview.acme.com`),
   not route matching's `*`.
4. **Ports in session membership.** Membership compares hostnames, so two local environments that
   differ only by port can't be told apart. Is that a real case?
5. **Opt-in environment- or origin-aware matching.** What a separate match entry point would look
   like, and whether it covers requests as well as views.

## References

- [`spec/v1/schema.md#route-matching`](../v1/schema.md#route-matching): the path-only matching rule
  this SEP does not change.
- [SEP-0002](0002-component-ref.md): the project-wide registry and first-by-path collision rule this
  SEP reuses for environment definitions.
- [SEP-0001](0001-dependencies-field.md): the sort-and-deduplicate canonical rule reused for
  reference lists.
- RFC 6454 (*The Web Origin Concept*): the scheme, host, and port tuple an origin URL carries.
