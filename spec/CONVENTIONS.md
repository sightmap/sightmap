# Spec conventions

This document defines the filename and directory conventions for artefacts under `spec/` — the SEPs and conformance fixtures. The conventions exist so that tooling (and humans) can identify the *kind* of an artefact from its path alone, without parsing frontmatter or reading the file.

These conventions are enforced by maintainer review today. The diagnostic vocabulary in [How a checker should behave](#how-a-checker-should-behave) is a reserved contract for a future `sightmap check-conventions` command; it is not yet implemented.

## SEPs (Sightmap Enhancement Proposals)

```
spec/seps/
  0000-template.md
  NNNN-{slug}.md
```

- **Path:** `spec/seps/NNNN-{slug}.md`.
- **Number (`NNNN`):** four-digit zero-padded integer, monotonically increasing. `0000-template.md` is reserved for the template; real SEPs start at `0001`.
- **Slug:** kebab-case, `[a-z][a-z0-9-]*`. Matches the SEP's title in spirit but does not need to be exact — it should be short, descriptive, and stable across the SEP's lifetime.
- **Extension:** `.md` only.

Process for proposing, accepting, and superseding SEPs lives in [`seps/README.md`](seps/README.md).

## Conformance fixtures

```
spec/conformance/
  README.md
  NNN-{slug}.fixture/
    sightmap/...
    expected.json
```

- **Path:** `spec/conformance/NNN-{slug}.fixture/`.
- **Number (`NNN`):** three-digit zero-padded integer. The number range encodes a coarse category:
  - `0NN` — runtime semantics (`validate`, `match`, `explain`, `lint` behavior)
  - `1NN` — formatter / canonicalization
  - Future ranges (`2NN`, `3NN`, …) reserved for additional categories as the conformance suite grows.
- **Slug:** kebab-case, `[a-z][a-z0-9-]*`.
- **Suffix:** `.fixture` is normative. The suffix lets tooling and humans recognise a conformance fixture from its path alone, even when the dir is shown out of context.
- **Contents:** every fixture directory must contain at least an `expected.json`; most contain a `sightmap/` directory holding the input YAML files. Fixture layout is documented in [`conformance/README.md`](conformance/README.md).

## What's *not* covered here

- Schema files under `spec/v1/` follow the existing JSON-Schema conventions — not codified by these rules.
- Other files under `spec/` (`README.md`, `VERSIONING.md`, this document) have no enforced naming convention.
- `.sightmap/` (the runtime sightmap directory) is governed by the [authoring conventions](v1/authoring-conventions.md), not by this document.

## How a checker should behave

A future `sightmap check-conventions <path>` would walk `spec/seps/` and `spec/conformance/` only, ignoring everything else. Any violation should surface as a structured diagnostic with one of:

- `convention.sep-filename`
- `convention.fixture-dirname`
- `convention.invalid-slug`
- `convention.unexpected-file`

Exit code `0` on a clean tree, `1` on any violation, with `--json` for machine-readable output.

## Updating these conventions

Tightening or extending these rules is a SEP-worthy change — propose via `seps/`, and update this document in the same PR train.
