# AGENTS.md

Guidance for coding agents working in this repository. This is the monorepo for
the open Sightmap project: the specification, its reference implementation, and
both websites.

> This file is maintained as the repo is assembled. Areas are landing
> incrementally (see `PLAN.md`); sections appear as their code arrives.

## Repository layout

| Path | Stack | What it is |
|---|---|---|
| `spec/` | Markdown + JSON Schema | Normative specification, SEP process, conformance fixtures. Source of truth. |
| `go/` | Go | Reference implementation: `sightmap` CLI + `go get`-able library. npm: `@sightmap/sightmap`. |
| `docs/` | Astro Starlight | Documentation site (docs.sightmap.org). |
| `web/` | React + Vite | Marketing landing page (sightmap.org). |

Each area is self-contained. `go.mod` lives only under `go/`; each site has its
own `package.json` and `netlify.toml`.

## Golden rule: the spec is the source of truth

`spec/v1/` (the human-readable `schema.md` + the machine-readable
`sightmap.schema.json`) is normative. Docs, websites, and the implementation
describe or implement it — when any of them disagree with `spec/`, `spec/` wins.
Never change spec semantics without an SEP (`spec/seps/`).

## Working in each area

### `spec/`
- Changes to spec semantics (fields, matching, merge rules) require an SEP. Small
  wording/example fixes can be a plain PR.
- Keep `schema.md`, `sightmap.schema.json`, and `spec/v1/examples/` in sync in one change.

### `go/` (module `github.com/sightmap/sightmap/go`)
- `go test ./...` from `go/`.
- `cmd/sightmap/` is the binary; library packages (`match`, `sel`, `comps`, …) at the module root.
- Downstream consumers import the library directly, so treat exported names as public API.

### `docs/` and `web/`
- `pnpm install && pnpm dev` in the respective directory.
- Both deploy to Netlify from their own subdirectory.

## Sightmap dogfooding

This repo curates its own `.sightmap/` corpora (the sites are living examples of
the spec). Before modifying UI code, read the relevant `.sightmap/` YAML to
understand the view structure, components, and any `memory:` entries. When adding
or changing views/components, update the corresponding sightmap file. Components
use `data-component="ComponentName"` attributes for runtime matching.

## CI

Path-filtered GitHub Actions run per area on every PR (`.github/workflows/`):
`go` (gofmt + build + `go test`), `spec` (schema-validate examples + conformance),
`docs` (build + lychee link check), `web` (build). A pushed `v*` tag triggers
`release` — goreleaser (config `go/.goreleaser.yml`) plus the npm publish of
`@sightmap/sightmap` from `go/npm/`.

## Conventions

- One concern per PR; keep diffs focused.
- Commits are signed off (DCO) — `git commit -s`. See `CONTRIBUTING.md`.
- Don't commit local tooling directories (`.yaks/`, `.agents/`, `.claude/`); they're gitignored.
