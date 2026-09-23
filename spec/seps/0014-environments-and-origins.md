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

Add a file-root `environments` registry of named deploy targets and a file-root `origins` map of
hosts that are the same everywhere, and let views and requests reference both by name. A web
environment maps surface names (`app`, `api`) to the `scheme://host[:port]` URL each surface has in
that environment. A native environment is identified by its app ID and build type, and borrows a
web environment's origins through `backend`. Third-party hosts, like a tracking pixel or a regional
telemetry endpoint, go in the file-root `origins` map. An origin name resolves per environment, so
`origins: [api]` means the API host of whichever environment a session belongs to. References are
declarative: they don't change route matching, and an entity that references none declares no
constraint.

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
4. **Hosts differ per deploy target, and apps share them.** A web app's API is `api.acme.com` in
   production and `api.staging.acme.com` in staging. A native beta build calls the staging API, and
   a native release build calls the production one.
5. **Not every host is yours.** A page fires a tracking pixel at `www.facebook.com`; an app sends
   telemetry to a vendor with separate US and EU hosts. Those hosts are the same in every one of your
   deploy targets.

A deploy target (`staging`, `ios-beta`) and a surface (`app`, `api`) are different things. A session
belongs to a deploy target, and a corpus is published to one. A surface is a host a deploy target
serves pages from or calls: the same surface in two targets is the same part of the product on two
hosts, and an API surface has no sessions of its own. Defining first-party surfaces inside each web
deploy target captures both facts: `api` is one name, and each environment says what its API host
is. Third-party hosts don't vary by deploy target, so they are defined once, outside any environment.

Native deploy targets have no page hosts. Their identifier is an app ID, and an app ID alone can
under-determine the target: an iOS beta and its release build ship under one app ID, so only the
build type tells them apart. What a native build calls is a web deploy target's backend, so it
borrows that target's origins rather than repeating them.

## Proposal

### Shape

Environments and shared origins are defined at a file root, in any file:

```yaml
# .sightmap/environments.yaml
version: 1
environments:
  - name: ios-beta
    platform: ios
    app_id: com.acme.app
    build_type: beta
    backend: staging
  - name: ios-prod
    platform: ios
    app_id: com.acme.app
    build_type: release
    backend: prod
  - name: local
    origins:
      api: https://api.acme.test:8443
      app: https://app.acme.test:8443
  - name: prod
    origins:
      api: https://api.acme.com
      app: https://app.acme.com
  - name: staging
    origins:
      api: https://api.staging.acme.com
      app: https://app.staging.acme.com

origins:
  facebook:     https://www.facebook.com
  telemetry-eu: https://eu.telemetry.example.com
  telemetry-us: https://us.telemetry.example.com
```

Views and requests in any file reference environments and origins by name:

```yaml
# .sightmap/orders.yaml
version: 1
views:
  - name: OrderHistory
    route: "/ui/*/settings/orders/history"
    environments: [local, prod, staging]
    origins: [app]
    requests:
      - name: ListOrders
        route: /orders
        method: GET
        origins: [api]
      - name: FacebookPixel
        route: /tr
        method: GET
        origins: [facebook]
  - name: OrderHistoryScreen
    route: /orders/history
    environments: [ios-beta, ios-prod]
    requests:
      - name: SyncOrders
        route: /orders/sync
        method: POST
        origins: [api]
requests:
  - name: SendTelemetry
    route: /v1/events
    method: POST
    origins: [telemetry-eu, telemetry-us]
```

`OrderHistoryScreen` is the iOS screen. Views currently match by URL route only, so its `route`
stands in for a native screen key, which is outside this SEP.

How each entity resolves:

| | local | staging | prod | ios-beta | ios-prod |
|---|---|---|---|---|---|
| `OrderHistory` | `app.acme.test:8443` | `app.staging.acme.com` | `app.acme.com` | not present | not present |
| `ListOrders` | `api.acme.test:8443` | `api.staging.acme.com` | `api.acme.com` | not present | not present |
| `FacebookPixel` | `www.facebook.com` | `www.facebook.com` | `www.facebook.com` | not present | not present |
| `OrderHistoryScreen` | not present | not present | not present | no surface | no surface |
| `SyncOrders` | not present | not present | not present | `api.staging.acme.com` | `api.acme.com` |
| `SendTelemetry` | EU or US | EU or US | EU or US | EU or US | EU or US |

`ListOrders` and `FacebookPixel` are absent from the iOS environments because their view is.
`SyncOrders` resolves `api` through each iOS environment's `backend`. A session on
`app.staging.acme.com` belongs to `staging`; an iOS session from `com.acme.app` belongs to `ios-beta`
or `ios-prod` by its build type.

### Field reference

| Position | Field | Items | Description |
|---|---|---|---|
| file root | `environments` | environment definitions | Defines deploy targets and their per-environment origins. |
| file root | `origins` | map of origin name to URL | Defines origins with the same URL in every environment. |
| view | `environments` | environment names | Deploy targets this view exists in. |
| view | `origins` | origin names | Surfaces this view is served from. |
| request | `environments` | environment names | Deploy targets this endpoint exists in. |
| request | `origins` | origin names | Hosts this endpoint is called on. |

An environment definition:

| Field | Type | Applies to | Required | Description |
|---|---|---|---|---|
| `name` | string, `^[a-z][a-z0-9_-]*$` | all | yes | Unique among environments. |
| `platform` | `web` \| `ios` \| `android` | all | no, default `web` | Which kind of deploy target this is. |
| `origins` | map of origin name to URL | all | web: yes; native: no | Each surface's URL in this environment. |
| `app_id` | string | native | yes | The bundle ID (iOS) or application ID (Android) the app reports. |
| `build_type` | string | native | no | Build variant (`release`, `beta`, `debug`). Separates environments that share one `app_id`. |
| `backend` | environment name | native | no | A web environment whose origins this environment borrows. |

Origin names match `^[a-z][a-z0-9_-]*$`. Origin URLs are `scheme://host[:port]` with `http` or
`https` and no path, query, or fragment. A web environment MUST NOT set `app_id`, `build_type`, or
`backend`.

### Semantics

#### Environments and origins are separate axes

An environment answers "which deploy target"; an origin answers "which host". A view or request can
reference either, both, or neither: `environments: [staging]` alone says the entity exists only in
staging, and `origins: [api]` alone says it is called on the API host wherever it exists.

#### Definitions and references

Environments and their origins are defined only in a file-root `environments` array; shared origins
only in a file-root `origins` map. A view's or request's `environments` and `origins` arrays hold
bare names. A definition is never valid on a view or request, and a bare name is never valid at a
file root. Every URL and app ID lives in exactly one place, so rotating a hostname is a one-line
edit, and a tool listing a corpus's deploy targets and hosts reads the definitions only.

#### The registries are project-wide

Environment definitions and shared origins from every loaded file form one project-wide registry
each, the same lookup scope [SEP-0002](0002-component-ref.md) uses for components. Two environments
sharing a `name`, or two shared origins sharing a name, collide: the one from the first file by
source path wins, and a conforming SDK SHOULD warn (`environment-name-collision`,
`origin-name-collision`).

An origin name is defined either inside environments or in the shared map, never both. A name in both
places is invalid (`origin-name-collision`), so every origin reference has one definition site.

#### Origin resolution

An origin name resolves, for a given environment, by checking in order:

1. the environment's own `origins`;
2. for a native environment, its `backend`'s `origins`;
3. the shared `origins` map.

The first match is the URL; with no match, the name resolves to nothing in that environment. A
native environment's own `origins` take precedence over its backend's, which lets one native build
override a single host. `backend` MUST name a web environment (`environment-backend-invalid`), so
backends never chain and resolution has no cycles.

A conforming SDK SHOULD warn when an origin name is defined in some web environments' own `origins`
but not others (`origin-environment-gap`): an entity called on that surface has no URL in the rest.

#### Which environment a session belongs to

A web session belongs to the web environment whose own `origins` include the session's hostname.
Shared origins and native environments' origins never identify a session. Several web environments
can legitimately share a host that serves no sessions, like an API host; if a session's hostname
matches more than one web environment, the session belongs to none of them, and a conforming SDK
MAY warn when two web environments share a hostname (`origin-host-shared`).

A native session belongs to the environment whose `platform` and `app_id` match its app, preferring
one whose `build_type` matches the session's build variant; an environment with no `build_type`
covers every variant of its `app_id`. Two native environments with the same `platform`, `app_id`,
and `build_type` are the same target under two names, and a conforming SDK SHOULD warn
(`environment-duplicate`).

Web hostnames compare case-insensitively; app IDs compare byte-for-byte. Origin URLs compare after
lowercasing the scheme and host and dropping a port equal to the scheme's default.

#### Reference resolution

Every environment reference MUST name a defined environment (`environment-ref-unresolved`). Every
origin reference MUST name an origin defined in some environment or in the shared map
(`origin-ref-unresolved`).

#### Absent or empty means unconstrained

A view or request with no `environments` exists in every environment; with no `origins`, it declares
no host. An empty list, `[]`, means the same as an absent one: an accidentally emptied list should
not flip an entity from matching everywhere to matching nothing. A conforming SDK SHOULD warn on an
explicit empty list (`environments-empty`, `origins-empty`).

#### View-scoped requests

A view-scoped request exists only where its view exists: its effective environments are its own
list intersected with its view's, where an absent list on either side contributes no constraint.
`origins` are not inherited, because a page and the endpoints it calls routinely live on different
hosts (`OrderHistory` on `app`, `ListOrders` on `api` above).

#### Third-party hosts

A host outside the corpus's own deploy targets, such as a tracking pixel, a payment provider, or a
vendor's regional endpoint, is a shared origin: defined once in the file-root `origins` map and
referenced by name like any other. A vendor with several regional hosts is one shared origin per
region, and a request that can be sent to any of them lists them all. Because shared origins depend
on no environment, a reusable set of third-party definitions (their origins and their requests) can
be dropped into any corpus unchanged.

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

- MUST accept `environments` and `origins` at the file root (definitions), and `environments` and
  `origins` on a view or request (names).
- MUST reject a definition on a view or request, and a bare name at a file root.
- MUST reject a web environment without `origins` or with `app_id`, `build_type`, or `backend`, and a
  native environment without `app_id` or with an `app_id` that isn't a dot-separated identifier
  (`environment-invalid`).
- MUST reject an origin URL that isn't `scheme://host[:port]` (`origin-invalid`).
- MUST reject a `backend` that names no environment or a native one (`environment-backend-invalid`).
- MUST build one project-wide environment registry and one shared-origin registry. SHOULD warn on a
  collision within either; the first by source-file path wins (`environment-name-collision`,
  `origin-name-collision`).
- MUST reject an origin name defined both inside an environment and in the shared map
  (`origin-name-collision`).
- MUST resolve an origin for an environment from its own `origins`, then its `backend`'s, then the
  shared map.
- MUST reject an environment reference that names no environment (`environment-ref-unresolved`), and
  an origin reference defined nowhere (`origin-ref-unresolved`).
- MUST assign a web session to the one web environment whose own `origins` include its hostname, and
  to none if several do. MAY warn when two web environments share a hostname (`origin-host-shared`).
- SHOULD warn when an origin name is defined in some web environments but not others
  (`origin-environment-gap`), and when two native environments share `platform`, `app_id`, and
  `build_type` (`environment-duplicate`).
- MUST treat an absent or empty view- or request-level list as unconstrained. SHOULD warn on an
  explicit `[]` (`environments-empty`, `origins-empty`).
- MUST intersect a view-scoped request's environments with its view's. MUST NOT inherit a view's
  `origins` into its view-scoped requests.
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
- `$defs.originMap`: **new**.
  ```
  $defs.originMap:
    type: object
    minProperties: 1
    propertyNames: { $ref: "#/$defs/definitionName" }
    additionalProperties: { $ref: "#/$defs/originUrl" }
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
      app_id: { type: string, pattern: "^[A-Za-z0-9_-]+(\\.[A-Za-z0-9_-]+)*$" }
      build_type: { type: string }
      backend: { $ref: "#/$defs/definitionName" }
      origins: { $ref: "#/$defs/originMap" }
    if: { required: [platform], properties: { platform: { enum: [ios, android] } } }
    then: { required: [app_id] }
    else:
      required: [origins]
      not: { anyOf: [{ required: [app_id] }, { required: [build_type] }, { required: [backend] }] }
  ```
- `properties.environments` (root): **added**, optional,
  `{ type: array, items: { $ref: "#/$defs/environment" } }`.
- `properties.origins` (root): **added**, optional, `{ $ref: "#/$defs/originMap" }`.
- `$defs.view.properties.environments`, `$defs.view.properties.origins`,
  `$defs.request.properties.environments`, `$defs.request.properties.origins`: **added**, optional,
  each `{ type: array, items: { $ref: "#/$defs/definitionName" } }`.
- No `required` changes, no removals, no type changes to existing `$defs` entries.
- The Go SDK's unknown-field allowlists `fileRootFields`, `viewFields`, and `requestFields` each
  gain `"environments"` and `"origins"`.

### Canonical format

- Key order gains `environments, origins` after `version` at the top level, after `route` on a view,
  and after `method` on a request.
- An environment definition's key order is `name, platform, app_id, build_type, backend, origins`.
  Keys within any origin map are alphabetized.
- The top-level `environments` sequence is alphabetized by `name`.
- View- and request-level reference lists carry no order, so they are sorted and deduplicated like
  `dependencies`.

## Alternatives considered

### 1. Do nothing: pass hosts to each tool out of band

A consumer takes a host from a command-line flag or options struct instead of the corpus.

**Ruled out:** the information lives only in the author's head, every tool re-asks for it, and a
single flag can't express per-entity differences like a page on the app host calling the API host.

### 2. Surfaces as environments

Define `api` as an environment alongside `prod` and `staging`, with one host.

**Ruled out:** it isn't a deploy target. An API host has no sessions, so publishing a corpus to it or
asking which sessions belong to it means nothing, and one flat list can't say that `api` in staging
and `api` in prod are the same surface on different hosts.

### 3. A top-level origin registry that environments reference

Define every URL as a flat, named origin (`api-prod`, `api-staging`) and have each web environment
point at its origins.

**Ruled out:** each surface needs one name per environment, so a request called on the API lists
`origins: [api-local, api-prod, api-staging]` and has to be edited whenever an environment is added.
Keying origins by surface inside each environment keeps a reference to `[api]`.

### 4. A top-level origin registry qualified by environment

Define origins at the top level as `{name, environment, url}`, several definitions per name.

**Ruled out:** a web environment's identity would be split across its own entry and every origin
entry that names it, so reading one deploy target means scanning the whole origin list. Nesting
first-party origins inside the environment keeps each deploy target in one place. The shared map
covers only hosts that don't vary by environment, so it never needs a qualifier.

### 5. Third-party hosts repeated inside every environment

Define `facebook` in each environment's own `origins`, with the same URL each time.

**Ruled out:** the URL is duplicated once per environment, native environments need it too, and a
reusable set of third-party definitions couldn't be shared across corpora without knowing each
corpus's environment names.

### 6. Native environments with their own copy of every host

Drop `backend` and have each native environment embed the API origins it calls.

**Ruled out:** a native build calls a web deploy target's backend, so its hosts duplicate that
target's, and rotating the production API host would mean editing every native environment too.
Embedding stays available for a native build that genuinely calls a different host, and it
overrides the backend's entry.

### 7. Infer `platform` from an identifier's shape

Guess web or native from whether an identifier looks like a hostname.

**Ruled out:** an app ID like `com.acme.app` is also a valid hostname, so the guess would silently
pick the wrong identifier space.

### 8. Make environments or origins a route-matching constraint

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

1. **Default origins.** Most views are served from `app` and most requests go to `api`, so explicit
   lists repeat. A corpus-wide default per entity kind (`views: app`, `requests: api`) would remove
   the repetition at the cost of making where an entity runs implicit. Deferred until real corpora
   show how much repetition there is.
2. **Hostname wildcards.** Ephemeral preview hosts (`deploy-preview-464--acme.netlify.app`) can't be
   enumerated, and one corpus may want to cover every `*.acme.com` host at once. If wildcards are
   added, they should be a distinct, right-anchored form (`**.acme.com`), not route matching's `*`.
3. **Ports in session membership.** Membership compares hostnames, so two local environments that
   differ only by port can't be told apart. Is that a real case?
4. **A native view key.** Views match by URL route, which native screens don't have. Native screen
   matching needs its own SEP; this one only lets native environments be referenced.
5. **Opt-in environment- or origin-aware matching.** What a separate match entry point would look
   like, and whether it covers requests as well as views.

## References

- [`spec/v1/schema.md#route-matching`](../v1/schema.md#route-matching): the path-only matching rule
  this SEP does not change.
- [SEP-0002](0002-component-ref.md): the project-wide registry and first-by-path collision rule this
  SEP reuses for environments and shared origins.
- [SEP-0001](0001-dependencies-field.md): the sort-and-deduplicate canonical rule reused for
  reference lists.
- RFC 6454 (*The Web Origin Concept*): the scheme, host, and port tuple an origin URL carries.
