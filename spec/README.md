# The Sightmap specification

This directory contains the canonical Sightmap specification.

## Layout

```
spec/
├── README.md                 ← you are here
├── VERSIONING.md             ← how the spec is versioned and evolved
├── CONVENTIONS.md            ← filename/dir conventions for SEPs and fixtures
├── seps/                     ← Sightmap Enhancement Proposals (the change process)
├── conformance/             ← language-agnostic conformance fixtures
└── v1/
    ├── schema.md             ← human-readable reference for v1
    ├── sightmap.schema.json  ← machine-readable JSON Schema for v1
    ├── config.schema.json    ← schema for the optional .sightmap/config.yaml
    └── examples/             ← example sightmaps, validated in CI
```

## Which version should I use?

Use the latest released version. Right now that's **v1**.

The top-level `version:` field in every `.sightmap/*.yaml` file determines which version of this spec applies. A file with `version: 1` is parsed against `spec/v1/`.

See [`VERSIONING.md`](VERSIONING.md) for the policy on how versions are cut and how long old versions remain supported.

## Relationship to the docs and sites

The documentation site ([docs.sightmap.org](https://docs.sightmap.org)) presents this spec for reading and onboarding, and the landing page ([sightmap.org](https://sightmap.org)) introduces it. Both draw from the files here. When a site and the files under `spec/` ever disagree, **`spec/` is the source of truth**.

## Consuming the schema

SDK authors and tool builders: import `spec/v1/sightmap.schema.json` directly. It is a [JSON Schema Draft 2020-12](https://json-schema.org/draft/2020-12) document. You may fetch it from a pinned commit via GitHub raw URL, or vendor it into your project.

An example reference for YAML editors that support JSON Schema:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/sightmap/sightmap/main/spec/v1/sightmap.schema.json
version: 1
```

## Proposing changes

Open an SEP. See [`seps/README.md`](seps/README.md).
