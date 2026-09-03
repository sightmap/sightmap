# sightmap/sightmap — agent guide

Monorepo for the open-source **Sightmap** project: a YAML format + CLI that maintain a
shared memory of a web app for AI agents. A `.sightmap/` corpus in a repo names an
app's views, components, and API requests (any entry may carry a `memory:` notes
list); agents curate it against a live browser with the `sightmap` CLI. This repo
holds the normative spec, the Go reference implementation (published to npm as
`@sightmap/sightmap`), the canonical agent skills, and both websites.

## Stack

| Area | Runtime | Role |
|---|---|---|
| `go/` | Go (`go.mod` requires 1.23; CI pins 1.25.2) | Reference implementation + `sightmap` CLI; npm packaging in `go/npm/` |
| `web/` | Node ≥ 18, pnpm 10.32.1, React 19 + Vite 6 + Tailwind 4 | Marketing site sightmap.org |
| `docs/` | Mintlify (`mint` CLI, `npm i -g mint`) | Documentation site docs.sightmap.org |
| `spec/` | Node (ajv + js-yaml validators) | Normative spec + conformance fixtures |
| root | Node + npm workspaces (`go/npm`), jest, prettier, changesets | Release/tooling |

No databases, no Docker Compose, no external services, **no required env vars**
(`REPLAY_API_KEY` only gates an optional post-build sourcemap upload and no-ops
without it). Every check below runs offline except `mint broken-links`.

## Commands

### Install (from scratch — the sandbox snapshot already has all of this)

```bash
npm ci                        # root: jest + prettier (embedded browser JS tests)
(cd spec && npm ci)           # spec validators
(cd web && pnpm install)      # web site (pnpm 10.32.1: npm i -g pnpm@10.32.1)
```

Go toolchain: any ≥ 1.23 (CI pins 1.25.2). Chrome/Chromium on PATH is needed only
for `sightmap browser` commands.

### Check / test — mirrors the path-filtered CI (`go`, `spec`, `docs`, `web`)

```bash
cd go && gofmt -l . && go build ./... && go test ./...
cd go && go generate ./skills/...    # then: git status --porcelain -- skills go/skills  → must be empty
npm test                              # root: embedded browser JS in jsdom (jest)
cd web && pnpm build:blog && pnpm build:atlas && pnpm test    # vitest — generated manifests REQUIRED first
cd web && pnpm build                  # tsc + vite + prerender + route coverage + feeds
cd spec && npm run validate:examples && npm run validate:conformance
node docs/scripts/sync-spec.mjs       # then: git diff --exit-code docs/reference/schema.md
node docs/scripts/build-changelog.mjs # then: git diff --exit-code docs/changelog.mdx
node scripts/changelog-entry.mjs --check
(cd docs && mint validate && mint broken-links)
```

### Dev servers

- **web** — `cd web && pnpm dev`: runs the blog/atlas generators, then Vite. Port is
  printed at startup — **5173** in this environment. Verify:
  `curl -s -o /dev/null -w '%{http_code}\n' http://localhost:5173/` → `200`.
- **sightmap browser session** (the product's own dogfooding loop; needs Chromium):

```bash
sightmap browser start --headless --detach \
  --chrome-flag=--no-sandbox --chrome-flag=--disable-dev-shm-usage --url http://localhost:5173/
sightmap browser status        # ● running  cdp=7892  server=7891
sightmap snapshot --coverage --url http://localhost:5173/
sightmap console list          # captured console messages
sightmap browser stop
```

  Without `--detach` it is a foreground daemon that holds the shell. In containers
  Chrome needs `--no-sandbox`. sightmap HTTP server: **7891**, CDP: **7892**.

### CLI (reference implementation, built from source)

```bash
cd go && go build -o /tmp/sightmap ./cmd/sightmap
/tmp/sightmap version && /tmp/sightmap -h
/tmp/sightmap init                                             # scaffold a corpus in cwd
/tmp/sightmap validate|lint|stats|report --sightmap-dir DIR    # offline corpus checks
```

## Codebase map

See `codebase-map.md`. Golden rule: `spec/` is the source of truth — never change
spec semantics without an SEP (`spec/seps/`).

## Conventions (from AGENTS.md / CONTRIBUTING.md — read them for detail)

- One concern per PR; every commit signed off: `git commit -s` (DCO).
- PRs affecting the published `@sightmap/sightmap` package add a changeset
  (`npm run changeset`); repo-infra-only PRs skip it. Releases are automated from main.
- Never hand-edit generated artifacts: `go/skills/` (from `skills/`),
  `docs/reference/schema.md` (from `spec/v1/schema.md`), `docs/changelog.mdx`.
- The sites dogfood their own `.sightmap/` corpora — read the relevant YAML before
  UI work; components use `data-component="Name"` attributes.
- Don't commit local tooling dirs (`.yaks/`, `.agents/`, `.claude/`) — gitignored.

## Local Verification Summary

From the onboarding run, 2026-09-03 (sandbox `cmp_oIBPELPB`, live session
`i5gaup1ug947i8s77bxg1`). Dev stack: **healthy** — every CI area replicated locally
and a primary product flow exercised end-to-end.

| Check | Result |
|---|---|
| `gofmt -l .` (go/) | clean |
| `go build ./...` | OK |
| `go test ./...` | 17 packages, all pass (incl. `clitest`, ~11s) |
| Embedded-skills drift (`go generate ./skills/...`) | clean |
| `npm test` (root jest/jsdom) | 6 suites, 67/67 pass |
| `pnpm test` (web vitest) | 14 files, 159/159 pass |
| `pnpm build` (web) | OK — tsc, vite, 21 pages prerendered, route coverage 8/8 |
| Vite dev server | up on :5173; HTTP 200 on `/`, `/atlas`, `/blog` |
| `sightmap` CLI | `version`, `-h`, offline `validate`/`lint`/`stats` OK; `docs/.sightmap` validates clean; `init` corpus validates clean |
| Browser dogfood loop | `browser start --headless --detach` → `snapshot --coverage --url http://localhost:5173/` → annotated tree + coverage (34 interactive nodes, clusters keyed to real `data-component`s); `console list` clean (vite + corpus load, no errors) |
| `spec` validators | examples: 5 files, 0 failures; conformance: 32 files, 0 failures |
| `docs` checks | `mint validate` pass; `mint broken-links` none found; schema page + changelog in sync; changelog coverage OK |
| Screenshot | headless Chromium capture of the landing page (1440×2600 PNG) |

**Known pre-existing issue (not introduced here; out of scope for this PR):**
`web/.sightmap/app.yaml` line 8 is invalid YAML — an unquoted `Accept: text/markdown`
colon inside a `memory:` bullet. Both `sightmap validate --sightmap-dir web/.sightmap`
and js-yaml reject it (`docs/.sightmap/app.yaml` is clean). One-line quote fix for its
own PR.

## Sandbox snapshot

- **Snapshot ID:** `i5gaup1ug947i8s77bxg1` — captured 2026-09-03T17:36:07.011Z
  (template `ovak2ontvssfogtm9dj9:default`)
- **Baked in:** Go 1.27.1 (`/usr/local/go`), Node 20.20.2, pnpm 10.32.1 (global),
  `mint` CLI (global), Chromium 151; installed deps for root + `spec/` + `web/`;
  web content generated (`src/generated/`, `public/atlas/`); Vite dev server
  **running on :5173**.
- **After resume:** `curl -s -o /dev/null -w '%{http_code}' http://localhost:5173/`
  → `200`. If it died, restart with `cd web && pnpm dev`.
- Onboarding details and gotchas: `skills/local-dev/SKILL.md`.
