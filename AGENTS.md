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
| `skills/` | Markdown | Canonical agent skills (`sightmap-authoring`, `sightmap-browser`). Installable as a plugin, embedded in the CLI, and vendored downstream. |
| `docs/` | Mintlify | Documentation site (docs.sightmap.org). |
| `web/` | React + Vite | Marketing landing page (sightmap.org). |

Each area is self-contained. `go.mod` lives only under `go/`; `web/` has its own
`package.json` and `netlify.toml`; `docs/` is configured by `docs.json`.

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
- `go/skills/<name>/` is a **generated, committed** copy of the canonical `skills/`
  (see below) — never hand-edit it.

### `skills/` (canonical agent skills)
- The source of truth for the `sightmap-authoring` and `sightmap-browser` skills.
  Edit the skill Markdown **here**, at the repo root.
- `go:embed` can't reach outside the `go/` module or follow symlinks, so a copy
  is generated into `go/skills/<name>/` and checked in (the same generate-and-commit
  pattern as the docs schema page). Regenerate with `go generate ./skills/...`
  from `go/`; CI fails on any drift.
- Three delivery paths, one source: (1) **plugin** — root manifests
  (`.claude-plugin/`, `.codex-plugin/`, `.cursor-plugin/`) expose `skills/` so the
  repo installs like any plugin; (2) **CLI** — `sightmap skills install` extracts
  the embedded copy; (3) **npm** — the skills ship inside the `@sightmap/sightmap`
  package (`files` includes `skills/`; `go/npm/scripts/build-npm-packages.mjs`
  copies them into the meta package) so downstream tools like Subtext vendor them
  from a pinned version.
- When adding a new skill, create `skills/<name>/`, add it to the `//go:embed`
  list in `go/skills/embed.go`, and regenerate. (`go generate` also removes any
  `go/skills/<name>` copy whose canonical source was renamed or dropped.)
- The plugin manifests carry their own `version` fields (shown in harness UIs)
  that the tag-driven release does **not** touch. Keep them in sync from
  `go/npm/package.json` with `node scripts/sync-manifest-versions.mjs`. Release
  prep: bump `go/npm/package.json`, run the sync, commit, then tag. (Gemini is
  intentionally not a target — its extension manifest is MCP-only and has no
  skills concept.)

### `docs/`
- Mintlify site: `npm i -g mint`, then `mint dev` from `docs/`. See `docs/AGENTS.md`
  for page conventions.
- `reference/schema.md` is generated from `spec/v1/schema.md` by
  `docs/scripts/sync-spec.mjs` and checked in — regenerate, never hand-edit.
- Deploys via the Mintlify GitHub app on pushes to `main` (no build pipeline here).

### `web/`
- `pnpm install && pnpm dev` from `web/`.
- Deploys to Netlify from its subdirectory.

## Sightmap dogfooding

This repo curates its own `.sightmap/` corpora (the sites are living examples of
the spec). Before modifying UI code, read the relevant `.sightmap/` YAML to
understand the view structure, components, and any `memory:` entries. When adding
or changing views/components, update the corresponding sightmap file. Components
use `data-component="ComponentName"` attributes for runtime matching.

## CI

Path-filtered GitHub Actions run per area on every PR (`.github/workflows/`):
`go` (gofmt + build + `go test` + embedded-skills drift check; also triggered by
`skills/**`), `spec` (schema-validate examples + conformance),
`docs` (schema-page sync check + `mint validate` + `mint broken-links`), `web`
(build). A pushed `v*` tag triggers
`release` — goreleaser (config `go/.goreleaser.yml`) plus the npm publish of
`@sightmap/sightmap` from `go/npm/`.

## Conventions

- One concern per PR; keep diffs focused.
- Commits are signed off (DCO) — `git commit -s`. See `CONTRIBUTING.md`.
- Don't commit local tooling directories (`.yaks/`, `.agents/`, `.claude/`); they're gitignored.
