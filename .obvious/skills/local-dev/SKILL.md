---
name: local-dev
description: Bring the sightmap/sightmap dev stack up — Go CLI, web (Vite), spec validators, docs checks, and the sightmap browser dogfood loop — and verify it end-to-end.
---

# local-dev — sightmap/sightmap

Durable record of the onboarding run (2026-09-03) that produced the sandbox snapshot
`i5gaup1ug947i8s77bxg1` (template `ovak2ontvssfogtm9dj9:default`, Vite dev server
running on :5173). `../../obvious.md` has the short command list; this file records
the working sequence and the gotchas that cost time.

## Verified working sequence (cold start)

1. **Toolchains.** Go ≥ 1.23 (CI pins 1.25.2; snapshot bakes 1.27.1 in `/usr/local/go`),
   Node 20+, pnpm 10.32.1 (`npm i -g pnpm@10.32.1`), `mint` for docs
   (`npm i -g mint`), Chromium for browser flows (`sudo apt-get install -y
   --no-install-recommends chromium` — not in the base image).
2. **Root JS.** `npm ci`. Gotchas:
   - The prebuild left a root-owned `node_modules/` and `bun.lock`; `npm ci` fails
     EACCES until `sudo rm -rf node_modules bun.lock`.
   - `npm ci` re-applies the exec bit on `go/npm/bin/sightmap.js` (committed mode is
     644). `chmod 644 go/npm/bin/sightmap.js` before committing so the tree stays clean.
3. **Spec.** `cd spec && npm ci`, then `npm run validate:examples` (5 files) and
   `npm run validate:conformance` (32 files) — both must report 0 failures.
4. **Web deps.** `cd web && pnpm install`. pnpm's "Ignored build scripts: esbuild"
   warning is harmless — platform binaries arrive as optional deps and Vite works.
5. **Web content pipelines — REQUIRED before vitest.** `pnpm build:blog &&
   pnpm build:atlas`. Without them, vitest fails in `src/pages/BlogIndex.tsx`
   resolving `@/generated/blog-manifest` (the `src/generated/` dir is gitignored and
   built by these scripts). Then `pnpm test` (159 tests) and `pnpm build`
   (tsc + vite + prerender; `upload-sourcemaps` no-ops without REPLAY_API_KEY).
6. **Go.** `cd go && gofmt -l . && go build ./... && go test ./...` (17 packages,
   `clitest` is the slow one, ~11s). Drift check: `go generate ./skills/...` then
   `git status --porcelain -- skills go/skills` must be empty.
7. **CLI.** `cd go && go build -o /tmp/sightmap ./cmd/sightmap`. Smoke: `version`,
   `-h`, `validate --sightmap-dir docs/.sightmap` (clean), and `init` in a temp dir
   followed by `validate`/`lint`/`stats` there.
8. **Dev server.** `cd web && (nohup pnpm dev > /tmp/web-dev.log 2>&1 &)` — Vite
   prints the port in the log (5173 here). Verify with
   `curl -s -o /dev/null -w '%{http_code}' http://localhost:5173/` → 200
   (also spot-check `/atlas` and `/blog`).
9. **Docs.** `node docs/scripts/sync-spec.mjs` + `git diff --exit-code
   docs/reference/schema.md`; `node docs/scripts/build-changelog.mjs` + diff;
   `node scripts/changelog-entry.mjs --check`; `cd docs && mint validate &&
   mint broken-links` (link check needs network).
10. **Browser dogfood loop** — the product's own flow against the local site, from a
    directory holding a valid corpus (e.g. a temp `sightmap init` dir — see the known
    issue below):

    ```bash
    sightmap browser start --headless --detach \
      --chrome-flag=--no-sandbox --chrome-flag=--disable-dev-shm-usage \
      --url http://localhost:5173/
    sightmap browser status        # ● running  cdp=7892  server=7891
    sightmap snapshot --coverage --url http://localhost:5173/
    sightmap console list
    sightmap browser stop
    ```

    Gotchas: `browser start` without `--detach` is a foreground daemon that holds the
    shell (a `timeout` around it kills the session); Chrome in this container needs
    `--no-sandbox`; the overlay extension auto-extracts to `~/.sightmap/extension`.

## Environment facts

- No databases, no Docker Compose, no required env vars; everything runs locally.
- Ports: Vite **5173** (dev), sightmap HTTP server **7891**, CDP **7892**.
- Headless screenshot that works here:
  `chromium --headless=new --no-sandbox --disable-dev-shm-usage --screenshot=/tmp/shot.png --window-size=1440,2600 --virtual-time-budget=15000 <url>`
  (the three.js hero renders via SwiftShader — the GL "GPU stall" stderr line is benign).
- `gh` is authenticated (bot account); git protocol is https.

## Known pre-existing issue

`web/.sightmap/app.yaml` line 8 is invalid YAML — an unquoted `Accept: text/markdown`
colon inside a `memory:` bullet (`sightmap validate --sightmap-dir web/.sightmap` and
js-yaml both reject it). `docs/.sightmap/app.yaml` validates clean. Until it is fixed
(quote the scalar — its own PR), run browser dogfood flows from a different corpus,
not `web/.sightmap`.
