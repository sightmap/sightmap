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

One sightmap corpus can describe several apps at once (web, iOS, Android), and each is released
independently. A publish tool needs a way to turn a human-chosen name (`staging`, `ios-beta`) into
the identifier a session actually reports, so that a corpus published under that name resolves for
the right sessions later. Two problems make that harder than a single string field:

1. **Web and native don't share an identifier shape.** A session reports a hostname on web, and a
   bundle or package name on native. There's no common URL-like reading that covers both, so a
   field designed around one platform's identifier doesn't carry over to the other.
2. **A bundle ID alone under-determines the environment.** An iOS beta and its release build ship
   under one bundle ID. Two environments that need to stay distinguishable would report the same
   identifier and collapse into one, unless something else, a build variant, separates them.

Without a per-corpus name for these identifiers, a publish call has to spell out the same
identifier by hand every time, with no way for a CI script to look one up by a short, memorable
name.

## Proposal

### Shape

```yaml
# .sightmap/environments.yaml
environments:
  - { name: local,        value: app.acme.test }
  - { name: staging,      value: app.staging.acme.com }
  - { name: prod,         value: app.acme.com }

  - { name: ios-beta,     platform: ios,     value: com.acme.app, build_type: beta }
  - { name: ios-prod,     platform: ios,     value: com.acme.app, build_type: release }
  - { name: android-prod, platform: android, value: com.acme.app, build_type: release }
```

Six entries, three apps, one corpus. `environments` lives in its own file, or anywhere at a file
root; it doesn't have to share a file with the views and requests it has nothing to do with:

```yaml
# .sightmap/orders.yaml: an ordinary file, unaware environments.yaml exists
version: 1
views:
  - name: OrderHistory
    route: "/ui/*/settings/orders/history"
requests:
  - name: GetProfile
    route: /settings/profile
    method: GET
```

`OrderHistory` and `GetProfile` don't reference `environments` at all, because there's no view- or
request-level position for this field to occupy (see [Field reference](#field-reference)). Both
still match by route on a session from `local`, `staging`, or `prod` identically, whether or not
`environments.yaml` exists in the corpus. Publishing this same corpus under `--env ios-beta` doesn't
change what `OrderHistory` matches either; it only lets `com.acme.app` (build `beta`) resolve to
whichever corpus was last published there. Declaring `environments` changes what `subtext sightmap
publish --env <name>` resolves to, and nothing else in this file.

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
- No `$defs.view.properties.environments` or `$defs.request.properties.environments`: this field has
  no per-view or per-request position, and no reference/registry mechanism. It is file-root-only,
  exactly like `snapshots`.
- `fileRootFields` (the Go SDK's unknown-field allowlist) gains `"environments"`. `viewFields` and
  `requestFields` are unchanged; this SEP touches neither.

## Alternatives considered

### 1. Do nothing: keep environment identifiers out of the corpus, pass them on the CLI

Have `subtext sightmap publish` take `--scope-kind`/`--scope-value`/`--build-type` directly, with no
corpus-level declaration at all. (This remains available regardless, as an escape hatch; see
[Field reference](#field-reference).)

**Ruled out:** it works, but every publish call has to spell out the same identifiers by hand, with
no way for a CI script to look them up by a short, memorable name. `--env <name>` resolved from a
checked-in `environments:` block is the whole reason this field exists.

### 2. Infer `platform` from `value`'s shape instead of declaring it explicitly

Skip the `platform` field and guess web vs. native from whether `value` parses as a hostname.

**Ruled out:** a package name like `com.acme.app` is also a syntactically valid hostname, so the
guess would silently decide which identifier space a lookup runs in, with no way for an author to
correct a wrong guess short of renaming the app.

### 3. Name fields after the session attributes directly (`domain`, `app_package`) instead of a
generic `value`

Instead of one `value` field whose meaning depends on `platform`, use a differently-named field per
platform.

**Ruled out:** `value`'s role, an identifier a session reports about itself, is the same fact on
every platform; only which identifier space it's read in differs, and `platform` already carries
that. A per-platform field name would mean a schema union keyed on `platform`, for no benefit over
one field whose meaning `platform` already disambiguates.

## Migration

`environments` is additive and reserved: no existing corpus declares it today, so no existing file
needs rewriting. An SDK implementing this SEP needs:

- The `$defs.environment` schema addition and `properties.environments` at the file root (see
  [JSON Schema diff](#json-schema-diff)).
- `"environments"` added to the Go SDK's `fileRootFields` allowlist, or a conforming corpus produces
  spurious `unknown-field` warnings against an older SDK.
- No canonical-format change beyond whatever key-ordering/list-handling rule applies to other
  reserved tooling fields (see [`access`/`snapshots`](../v1/schema.md#reserved-tooling-fields));
  `environments` should follow that existing precedent rather than inventing its own.

## Open questions

1. **Should `environments` eventually feed matching, not just publish resolution?** Today it's
   purely a reserved tooling field, following `access`/`snapshots`. A future SEP proposing
   environment-aware route matching would need to promote some subset of this shape out of
   "reserved" status, as an explicitly opt-in mechanism so it doesn't silently reclassify existing
   pathname-only matches. Not proposed here.
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
