---
sep: 0014
title: Declaring publish targets via an `environments[]` field
author: Clint Ayres (@jurassix)
status: Draft
created: 2026-09-08
updated: 2026-09-23
spec-version-target: 1
related-issues: []
related-discussions: []
---

## Summary

Add an optional, file-root-only `environments` field: a flat list of
`{name, value, platform?, build_type?}` entries, each giving a local name to an identifier a
session already reports about itself, a hostname on web, or a bundle/package name (optionally
split by build variant) on native. `environments` is a **reserved tooling field**, like `access`
and `snapshots`: consumed by the `subtext sightmap publish` CLI to resolve `--env <name>` into the
scope a corpus is published under, but no part of this spec's route matching or merge semantics.
Declaring an environment does not make a corpus specific to it; the corpus still matches by path
everywhere, exactly as it would with no `environments` block at all. The declaration only gives an
environment a local name to publish under.

## Motivation

One sightmap corpus can describe several apps at once, web, iOS, Android, and each is released
independently. A publish tool needs a way to turn a human-chosen name (`staging`, `ios-beta`) into
the identifier a session actually reports, so that a corpus published under that name resolves for
the right sessions later. For web that identifier is a hostname; native has no equivalent of a URL
at all, so the same shape can't just be reused as written.

This SEP was originally drafted as `origins:`, a web-only, URL-keyed registry with per-view and
per-request overrides (see [Alternatives considered](#1-keep-origins-as-drafted-url-keyed-web-only)
for why that shape is replaced rather than extended). Two problems rule out extending it directly:

1. **`url:` doesn't map to native platforms.** A bundle ID or package name isn't a URL, and forcing
   one into `scheme://host[:port]` shape buys nothing: there's no scheme, and treating the bundle ID
   as a "host" invites exactly the ambiguity the original grammar was designed to foreclose.
2. **A rename alone doesn't cover matching.** An iOS beta and an iOS release ship under one bundle
   ID. Two environments that need to stay distinguishable would report the same identifier and
   collapse into one, unless something else, a build variant, separates them.

Replacing the block outright costs nothing: `origins:` is still in the design phase, nothing reads
it, and no corpus has adopted it.

## Proposal

### Shape

```yaml
# Before: SEP-0014 as drafted. Web only.
origins:
  - { name: staging, url: https://app.staging.acme.com }
  - { name: prod,    url: https://app.acme.com }
```

```yaml
# After: .sightmap/environments.yaml
environments:
  - { name: local,        value: app.acme.test }
  - { name: staging,      value: app.staging.acme.com }
  - { name: prod,         value: app.acme.com }

  - { name: ios-beta,     platform: ios,     value: com.acme.app, build_type: beta }
  - { name: ios-prod,     platform: ios,     value: com.acme.app, build_type: release }
  - { name: android-prod, platform: android, value: com.acme.app, build_type: release }
```

Six entries, three apps, one corpus.

### Field reference

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | yes | (none) | A local alias, resolved by `subtext sightmap publish --env <name>`. Not read by matching. |
| `platform` | string, enum `web` \| `ios` \| `android` | no | `web` | Selects the identifier space `value` is read in. Stated explicitly rather than inferred from `value`'s shape, because a package name like `com.acme.app` is also a syntactically valid hostname. |
| `value` | string | yes | (none) | The identifier a session reports for this environment: a hostname (web) or a bundle/package name (native), verbatim. |
| `build_type` | string | no | (none) | Native build variant (`release`, `beta`, `debug`, etc). Separates two environments that share one `value`. Not meaningful for `platform: web`. |

`environments` joins `access` and `snapshots` as a **reserved tooling field**: permitted by the
schema so a corpus that uses it validates, not part of this spec's matching or merge semantics, and
a conforming SDK MAY ignore it entirely. No file-discovery change either: every `*.yaml` under
`.sightmap/` is already discovered and merged, so `environments.yaml` is a naming convention, not a
new rule. A `platform: web` repo writes `{name, value}` and never sees `platform` or `build_type`.

### Semantics

`environments` entries are declarative labels, not a matching input. A corpus that declares
`environments` still matches a session by route exactly as it would with no `environments` block at
all, following the general rule for [reserved tooling fields](../v1/schema.md#reserved-tooling-fields).
What each field maps to on the session side, once a downstream consumer (the publish CLI, and the
storage/resolution layer described in the parent design) reads it:

| Declared | Session attribute | Role |
|---|---|---|
| `platform: web` | `PLATFORM` = `PAGE_TYPE_WEB` | selects the identifier space |
| `platform: ios` | `PLATFORM` = `PAGE_TYPE_IOS` | selects the identifier space |
| `platform: android` | `PLATFORM` = `PAGE_TYPE_ANDROID` | selects the identifier space |
| `value`, web | `DOMAIN` | the identifier matched, verbatim |
| `value`, native | `APP_PACKAGE` | the identifier matched, verbatim |
| `build_type` | `BUILD_TYPE` | separates two environments sharing one `APP_PACKAGE` |
| `name` | (none) | a local alias for `--env`; no session attribute |

Read as a sentence: a **web** session matches on `PLATFORM` plus `DOMAIN`; an **iOS** or **Android**
session matches on `PLATFORM` plus `APP_PACKAGE` plus `BUILD_TYPE`. None of these are new attributes
invented for this spec: each mirrors an attribute a session-capture platform already reports and
already matches on for its own recording-targeting rules (platform, domain or package identifier,
build variant), so this SEP introduces no parallel vocabulary for the same facts.

An app-version attribute is not part of this SEP's shape. A native session is pinned to a corpus by
app version rather than by session start time, because a user decides when to update and two app
versions run in the field for weeks, but that's a resolution-time rule for the consumer reading
`environments`, not a field this spec adds. How a stored, published corpus resolves against a
session (publication history, fallback tiers) is a downstream storage/publish design's concern, out
of scope for this SEP.

### Conformance

- MUST accept an optional `environments` array at the file root: `{name, value}` required,
  `platform` (enum `web`/`ios`/`android`, default `web`) and `build_type` optional.
- MUST treat `environments` as a reserved tooling field: MUST NOT let its presence or contents
  change route-matching or merge-resolution results for any view, request, or component.
- MAY ignore `environments` entirely; no behavior in this spec depends on an SDK implementing it.
- MUST NOT require `platform` or `build_type` when validating; a `platform: web` entry omitting
  `build_type` is valid.

### JSON Schema diff

- `$defs.environment`: **new**, mirroring the shape of `$defs.snapshot` and
  `$defs.view.properties.access`, a reserved-tooling-field object:
  ```
  $defs.environment:
    $comment: "Reserved tooling field. Not part of the spec's matching or merge semantics; conforming SDKs MAY ignore it."
    type: object
    required: [name, value]
    additionalProperties: false
    properties:
      name: { type: string }
      platform: { type: string, enum: [web, ios, android], default: web }
      value: { type: string }
      build_type: { type: string }
    description: "Non-normative tooling field: a local name for an identifier a session reports about itself (hostname on web, bundle/package name on native), consumed by the publish CLI to resolve --env."
  ```
- `properties.environments` (root): **added**, optional, `{ type: array, items: { $ref: "#/$defs/environment" } }`, with a `$comment` matching the reserved-tooling-field wording used for `snapshots`. Root `required` is unchanged (`["version"]`).
- No `$defs.view.properties.origins` or `$defs.request.properties.origins`: unlike the withdrawn
  `origins:` draft, `environments` has no per-view or per-request position and no reference/registry
  mechanism. It is file-root-only, exactly like `snapshots`.
- `fileRootFields` (the Go SDK's unknown-field allowlist) gains `"environments"`. `viewFields` and
  `requestFields` are unchanged; this SEP touches neither.

## Alternatives considered

### 1. Keep `origins:` as drafted (URL-keyed, web-only)

Extend the original SEP-0014 draft in place: keep the `{name, url}` shape and its per-view/per-request
override, and find some way to fit native identifiers into a `url:` field.

**Ruled out:** see [Motivation](#motivation): a bundle ID or package name is not a URL, and the
`scheme://host[:port]` grammar the original draft depends on for unambiguous reference resolution has
no natural reading for either. Nothing implements the draft yet, so there is no migration cost to
avoid by keeping it.

### 2. Rename `url:` to a generic `value:` inside the existing registry/reference design

Keep the named-definition-and-reference registry, per-view/per-request overrides, and origin-tuple
grammar from the original draft, and just widen the value type to admit a bundle or package name
alongside a URL.

**Ruled out:** a rename doesn't solve the collapsing-identifier problem; an iOS beta and release
sharing one bundle ID still need a `build_type` to stay distinct, and that has no place in an origin
*tuple* (scheme/host/port) at all. Once `build_type` is added as a genuinely new axis, the per-view/
per-request override and reference-registry machinery bring no benefit for a purely declarative,
file-root-only field, so most of the original draft's complexity would be carried forward for no
reason.

### 3. Do nothing: keep environment identifiers out of the corpus, pass them on the CLI

Have `subtext sightmap publish` take `--scope-kind`/`--scope-value`/`--build-type` directly, with no
corpus-level declaration at all. (This remains available regardless, as an escape hatch; see
[Field reference](#field-reference).)

**Ruled out:** it works, but every publish call has to spell out the same identifiers by hand, with
no way for a CI script to look them up by a short, memorable name. `--env <name>` resolved from a
checked-in `environments:` block is the whole reason this field exists; see the parent design's
`subtext sightmap publish --env staging` examples.

## Migration

`environments` is additive and reserved: no existing corpus declares `origins` or `environments`
today (the former never shipped past Draft), so no existing file needs rewriting. An SDK
implementing this SEP needs:

- The `$defs.environment` schema addition and `properties.environments` at the file root (see
  [JSON Schema diff](#json-schema-diff)).
- `"environments"` added to the Go SDK's `fileRootFields` allowlist, or a conforming corpus produces
  spurious `unknown-field` warnings against an older SDK.
- No canonical-format change beyond whatever key-ordering/list-handling rule applies to other
  reserved tooling fields (see [`access`/`snapshots`](../v1/schema.md#reserved-tooling-fields));
  `environments` should follow that existing precedent rather than inventing its own.

Because the original `origins:` draft never shipped, this SEP replaces it outright rather than
deprecating a field already in the wild.

## Open questions

1. **Should `environments` eventually feed matching, not just publish resolution?** Today it's
   purely a reserved tooling field, following `access`/`snapshots`. If a future SEP wants
   origin-aware route matching (raised and deferred in the original `origins:` draft), it would need
   to promote some subset of this shape out of "reserved" status. Not proposed here.
2. **Is a flat, file-root-only list sufficient, or will a real multi-file corpus want to split
   `environments.yaml` per app**, the way [SEP-0001](0001-dependencies-field.md)'s `dependencies`
   scopes other definitions to files? Nothing in the shape prevents one `environments.yaml` per
   platform; whether the spec should say anything about that convention is open.
3. **Should an unrecognized `platform` value warn or silently pass through?** As a reserved tooling
   field, the schema's `enum` already rejects anything outside `web`/`ios`/`android` at validation
   time; whether that's the right strictness for a field SDKs are otherwise free to ignore is worth a
   second opinion.

## References

- A downstream storage/publish design (outside this repo): publish/ingest storage, publication
  resolution tiers, and native mobile scope kinds all consume this SEP's `environments:` block, but
  none of that resolution logic is part of this spec.
- [`spec/v1/schema.md#reserved-tooling-fields`](../v1/schema.md#reserved-tooling-fields): the
  `access`/`snapshots` precedent this SEP's `environments` field follows. Permitted by the schema,
  outside matching/merge semantics, ignorable by a conforming SDK.
- A recording-targeting attribute model already in use elsewhere in the consuming platform: the
  session attributes (platform, domain or package identifier, build variant) this SEP's fields are
  chosen to align with, so nothing here is invented in parallel to what targeting already matches
  on.
- [SEP-0001](0001-dependencies-field.md): file-scoped declarations precedent, referenced in
  [Open questions](#open-questions).
