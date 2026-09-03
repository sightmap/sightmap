# Codebase map — sightmap/sightmap

Folder-level overview, depth ≤ 2. Commands and conventions live in `obvious.md`; the
durable local-dev runbook is `skills/local-dev/SKILL.md`.

| Path | Area | What lives there |
|---|---|---|
| `spec/` | Spec | Normative Sightmap spec — the source of truth |
| `spec/v1/` | Spec | v1 schema: `schema.md` + `sightmap.schema.json` + examples |
| `spec/conformance/` | Spec | Language-agnostic conformance fixtures (`.fixture/` dirs) |
| `spec/seps/` | Spec | SEP process for spec-semantics changes |
| `spec/scripts/` | Spec | `validate-sightmap.mjs` — examples/fixtures vs JSON Schema |
| `go/` | Go impl | Go module `github.com/sightmap/sightmap/go` (`go.mod` lives only here) |
| `go/cmd/` | Go impl | `cmd/sightmap/` — the CLI binary (browser, snapshot, coverage, validate, lint, atlas, skills, …) |
| `go/browser/` | Go impl | Chrome DevTools Protocol session, launcher, page actions |
| `go/sightmap/`, `go/match/`, `go/viewset/`, `go/compquery/` | Go impl | Corpus model + selector matching — exported names are public API |
| `go/authoring/`, `go/coverage/`, `go/render/`, `go/extract/`, `go/observe/`, `go/atlas/` | Go impl | Authoring loop, coverage tiers, snapshot rendering/extraction, community atlas |
| `go/npm/` | Packaging | npm wrapper `@sightmap/sightmap` — `bin/`, platform optional deps, build scripts |
| `go/skills/` | Generated | Committed embed of root `skills/` — regenerate with `go generate`, never hand-edit |
| `go/clitest/`, `go/conformance/`, `go/testdata/` | Tests | CLI integration harness, conformance runner, fixtures |
| `skills/` | Skills | Canonical `sightmap-authoring` + `sightmap-browser` skills (edit here, not `go/skills/`) |
| `docs/` | Docs site | Mintlify site for docs.sightmap.org (`docs.json`) |
| `docs/reference/`, `docs/start/`, `docs/cli/`, `docs/concepts/`, `docs/authoring/` | Docs site | Guides; `reference/schema.md` is generated from `spec/v1/schema.md` |
| `docs/scripts/` | Docs site | `sync-spec.mjs`, `build-changelog.mjs` |
| `web/` | Web | Marketing site sightmap.org — React 19 + Vite 6 + Tailwind 4 |
| `web/src/` | Web | App code; `src/data/atlas/` is the vendored community atlas |
| `web/scripts/` | Web | Content pipelines: `build-blog.ts`, `build-atlas.ts`, prerender, feeds, OG images |
| `web/netlify/` | Web | Netlify config + edge functions (deploys from this subdirectory) |
| `scripts/` | Root | `sync-manifest-versions.mjs`, `changelog-entry.mjs` (release tooling) |
| `.changeset/` | Release | Changesets config + pending changesets |
| `.github/` | CI | Path-filtered workflows: `go`, `spec`, `docs`, `web`, `release`; plugin manifests at `.claude-plugin/` etc. |
| `maintainers/` | Governance | Maintainer guides: reviewing, releasing, triage, spec evolution |
