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
| `skills/` | Markdown | Canonical agent skills (`sightmap-authoring`, `sightmap-browser`, `sightmap-webmcp`). Installable as a plugin, embedded in the CLI, and vendored downstream. |
| `webmcp/` | Node (CJS) | WebMCP codegen adapter: `.sightmap/` corpus + `webmcp.tools.yaml` → a `document.modelContext` tool bundle. npm workspace, repo-internal (not published). |
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
  that the tag-driven release does **not** touch. `scripts/sync-manifest-versions.mjs`
  writes `go/npm/package.json`'s version into all of them; it runs automatically
  as part of the changesets `version-packages` step (see [Releasing](#releasing)),
  so you normally never hand-edit them. (Gemini is intentionally not a target —
  its extension manifest is MCP-only and has no skills concept.)

### `webmcp/`
- The sightmap → WebMCP codegen adapter (see `webmcp/README.md` for the
  pipeline). Plain CommonJS, an npm workspace of the root `package.json`;
  its only dependency is `js-yaml`.
- **Two generators, one output.** The user-facing CLI is `sightmap webmcp`
  (Go, `go/webmcp/`); this directory is the reference implementation. They
  must emit byte-identical bundles: `go/webmcp/golden_test.go` regenerates
  the committed `webmcp/examples/` from Go and byte-compares, so any output
  change must land in both generators plus regenerated examples. The Go side
  embeds the shared browser runtime via `go generate ./webmcp/...`
  (generate-and-commit, drift-checked in CI like the skills embed).
- Tests run through the **root** jest config: `npx jest webmcp` from the repo
  root. The suite includes a drift check on `webmcp/examples/` — after
  changing the generator, the runtime, an example manifest, or a corpus an
  example hashes, regenerate every committed bundle:
  `for d in webmcp/examples/*/; do node webmcp/bin/sightmap-webmcp.js generate --tools $d/webmcp.tools.yaml --format all; done`.
- Generated bundles are deterministic (no timestamps); provenance is a corpus
  content hash in the banner. Never hand-edit a generated `*.webmcp*.js`.
- The embedded browser runtime (`src/runtime/runtime.js`) mirrors the
  semantics of `go/browser/deepquery.js` and `go/observe/properties.js` —
  keep them behaviorally in sync when either side changes.
- The WebMCP surface it targets (`document.modelContext.registerTool`, the
  legacy `navigator.modelContext` probe, the tool-descriptor shape) lives in
  `src/runtime/runtime.js` (`__smwBoot`/`__smwDescriptor`); update there as
  the proposal evolves.

### `docs/`
- Mintlify site: `npm i -g mint`, then `mint dev` from `docs/`. See `docs/AGENTS.md`
  for page conventions.
- `reference/schema.md` is generated from `spec/v1/schema.md` by
  `docs/scripts/sync-spec.mjs` and checked in — regenerate, never hand-edit.
- Deploys via the Mintlify GitHub app on pushes to `main` (no build pipeline here).

### `web/`
- `pnpm install && pnpm dev` from `web/`.
- Deploys to Netlify from its subdirectory.
- Routes are declared twice — in `src/App.tsx` and again in
  `scripts/prerender.tsx`, which writes one static file per URL. A route added
  to only the first renders in dev and ships as a client-only page.
- Two content pipelines feed the app, both generating into gitignored
  directories on every `pnpm dev` / `pnpm build`:
  `scripts/build-blog.ts` (from `content/blog/`) and `scripts/build-atlas.ts`
  (from `src/data/atlas/`, the vendored community atlas — see that directory's
  `README.md`). Neither makes a network call, at build or at run time; the
  atlas is vendored precisely so a bad community merge cannot break a deploy.
- Atlas READMEs are community-authored, so `scripts/lib/atlas.ts` renders them
  through its own hardened `marked` instance (raw HTML escaped, URL schemes
  allowlisted). Don't route that content through `scripts/lib/posts.ts`, which
  deliberately passes raw HTML for maintainer-written posts.

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
(build), `webmcp` (jest suite + generated-example drift check). On every push to `main`, `release` opens/updates the "Version Packages"
PR when changesets are pending; once that PR is merged (no changesets left), the
same workflow tags the release, runs goreleaser (config `go/.goreleaser.yml`),
and publishes `@sightmap/sightmap` from `go/npm/` to npm. No manual tag push.

## Releasing

Versioning is driven by [changesets](https://github.com/changesets/changesets),
scoped to the `@sightmap/sightmap` package (the `go/npm` workspace). The flow:

1. **Per change:** if a PR affects the published package, add a changeset
   (`npm run changeset`) and commit the resulting `.changeset/*.md`. Infra/docs
   changes that don't affect the package can skip it.
2. **Per release (automatic):** when changesets land on `main`, the `release`
   workflow opens a "Version Packages" PR that bumps `go/npm/package.json`, writes
   `go/npm/CHANGELOG.md`, runs `scripts/sync-manifest-versions.mjs`, and deletes
   the consumed changesets.
3. **Per release (automatic):** merging that PR is itself a push to `main`, so
   `release` runs again; with no changesets left to consume, it tags the commit,
   runs goreleaser, and publishes the npm packages, all in that same run.

So the only hand step is: write a changeset in your PR, and merge the Version
Packages PR when you're ready to ship it. Everything else (version math,
changelog, manifest sync, tagging, build, publish) is automatic.

## Conventions

- One concern per PR; keep diffs focused.
- Commits are signed off (DCO) — `git commit -s`. See `CONTRIBUTING.md`.
- Don't commit local tooling directories (`.yaks/`, `.agents/`, `.claude/`); they're gitignored.
