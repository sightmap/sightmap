# Versioning

Two things in this repo are versioned independently.

| | Versioned by | Current | Bumps when |
|---|---|---|---|
| **The spec** | A single integer in the YAML `version:` field | `1` | A breaking change to the YAML format |
| **The project** | [SemVer](https://semver.org/) (in `package.json` and `CHANGELOG.md`) | `0.1.0` | Anything ships — patch, minor, or major per SemVer |

The spec stream is the contract SDKs implement. The project semver describes how settled that stream currently is and tracks the cadence of repo-level changes (docs, schema clarifications, examples, tooling).

We aim to keep the spec stream at `1` for a long time. The project will move through `0.x`, `1.0.0`, and beyond independently.

## TL;DR

- The spec stream version (`version: 1`) bumps **only on breaking changes** to the YAML format.
- The project semver bumps as `spec/v1/` matures: `0.x` while we tighten in place, `1.0.0` once we commit to compatibility, `2.0.0` only if `spec/v2/` ever ships.
- We support the previous spec stream for **at least 12 months** after a successor stream is released.

## Pre-1.0 (today)

The project is at `0.1.0`. Per SemVer 0.x semantics:

- The shape of `spec/v1/` may tighten or clarify in place.
- Legacy shapes are rejected, not coerced. When a field is renamed, removed, or re-typed, `additionalProperties: false` makes documents using the old shape schema-invalid. **Conversely, documents using a newly-added optional field are schema-invalid in older SDKs** — `additionalProperties: false` rejects the new key as an unexpected property. Every schema evolution, additive or reductive, therefore requires coordinated SDK releases and a host min-version pin. The spec stream's `version:` field is unchanged for additive evolutions; the SDKs bump and adopters pin. Conforming SDKs MUST surface this as an error and MUST NOT silently default or migrate it.
- Such changes are announced in `CHANGELOG.md` and a Discussion before they land.
- We won't introduce a `version: 2` until the project hits `1.0.0` and we have a real break to commit to.

When the project reaches `1.0.0`, the rules below become commitments rather than guidance.

## What counts as a breaking change to the spec stream

These require bumping `version: 1` → `version: 2` and creating `spec/v2/`:

- Removing or renaming a field
- Changing the type of a field (e.g. string → array)
- Making a previously optional field required
- Changing route-matching semantics (`*`, `**`, `:param`) in a way that alters existing matches
- Changing the file-merge rules under `.sightmap/`
- Any change that could make a previously valid `.sightmap/` directory produce different behavior in a conforming SDK

The bar is high. Breaking changes require an accepted SEP with explicit maintainer consensus (see [`GOVERNANCE.md`](https://github.com/sightmap/.github/blob/main/GOVERNANCE.md)) and a stated migration path.

## What is **not** a breaking change to the spec stream

These can land within stream `1` and are reflected in a project minor or patch bump:

- Adding a new optional field at any level
- Adding a new top-level key (a new peer to `views`, `components`, `requests`)
- Adding a new match kind that is opt-in via a new field
- Tightening validation to reject input that was always underspecified — pre-1.0 these land freely; post-1.0 they get a deprecation window
- Clarifying wording that doesn't change conforming behavior

These changes update the current stream's `schema.md`, the JSON Schema, and the examples as needed.

## How a new spec stream gets cut

1. An SEP proposes the breaking change.
2. Discussion and acceptance per [`seps/README.md`](seps/README.md) and [`GOVERNANCE.md`](https://github.com/sightmap/.github/blob/main/GOVERNANCE.md).
3. A new `spec/v<N+1>/` directory is added. It contains its own `schema.md`, `sightmap.schema.json`, and `examples/`.
4. The old `spec/v<N>/` directory stays put. It is **not edited** after a successor stream exists, except for pure clarifications that do not change meaning.
5. The project semver gets a major bump. `CHANGELOG.md` adds a new release section with a migration guide.
6. Official SDKs update to support both streams and dispatch on the `version:` field.
7. The website is updated.

## Support windows

- **Current spec stream**: fully supported. Bug fixes, clarifications, new examples, new optional fields.
- **Previous spec stream**: supported for at least 12 months after the successor is released. Security fixes and clarifications only.
- **Older than that**: best-effort. SDKs may choose to drop support.

These windows are minimums. We may extend them if real-world adoption is slow to migrate.

## Deprecation process

When we intend to remove something in the next stream:

1. Announce the deprecation in `CHANGELOG.md` and the current stream's `schema.md`.
2. Keep the feature working for at least one full project minor-version cycle before cutting the next stream.
3. Official SDKs emit a warning (not an error) when they encounter deprecated usage.
4. Remove it in the next stream, with a migration note.

## Questions

Versioning is one of the hardest parts of an evolving spec. If you're unsure whether a change is breaking, err on the side of cautious and bring it up in the SEP discussion. The maintainers would rather have a conversation than issue a surprise migration.
