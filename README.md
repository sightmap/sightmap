# Sightmap

Sightmap is an open YAML format and CLI that maintain a shared memory of a web
app for AI agents. A `.sightmap/` directory in your repo names the app's
**views**, **components**, and **API requests**; any definition can carry a
`memory` list — freeform notes about quirks, invariants, and shortcuts the
source code doesn't record. Agents curate the map against the running app with
the `sightmap` CLI, definitions link back to their source files, and every
agent that works on the app reads the same map.

→ **[sightmap.org](https://sightmap.org)** — overview and landing page
→ **[docs.sightmap.org](https://docs.sightmap.org)** — full documentation
→ [`spec/`](spec/) — the canonical specification
→ [`go/`](go/) — the reference implementation (Go library + `sightmap` CLI)

## This repository

This is the home of the open Sightmap project. It holds the spec, the reference
implementation, and both websites in one place:

| Path | What it is |
|---|---|
| [`spec/`](spec/) | The **normative** specification — `spec/v1/` schema + JSON Schema, the SEP process (`spec/seps/`), and language-agnostic conformance fixtures. Source of truth. |
| [`go/`](go/) | The reference **Go implementation** — the `sightmap` CLI (live browser capture, annotated snapshots, coverage) plus a `go get`-able library for the component model and selector matching. Published to npm as [`@sightmap/sightmap`](https://www.npmjs.com/package/@sightmap/sightmap). |
| [`skills/`](skills/) | The **agent skills** — `sightmap-authoring`, `sightmap-browser`, and `sightmap-webmcp`. This is the canonical, standalone skills directory; the CLI embeds a committed copy under `go/skills/` (regenerated via `go generate`). |
| [`webmcp/`](webmcp/) | Browser runtime, jsdom tests, and example manifests for `sightmap webmcp`. The generator lives in the Go CLI. |
| [`docs/`](docs/) | The documentation site at [docs.sightmap.org](https://docs.sightmap.org) (Mintlify). |
| [`web/`](web/) | The marketing landing page at [sightmap.org](https://sightmap.org) (React + Vite). |

Each area has its own README with build and contribution details.

## Install

The `sightmap` CLI is published to npm. Install it globally:

```bash
npm install -g @sightmap/sightmap
sightmap version
```

Or run it without installing: `npx @sightmap/sightmap <command>`. Building from
source (Go) is covered in [`go/`](go/).

## Quickstart

Install the CLI (above), then drop a `.sightmap/` directory at your project
root. Every `*.yaml` / `*.yml` file under it is discovered recursively and
merged.

```yaml
# .sightmap/home.yaml
version: 1

views:
  - name: FlightSearch
    route: /search
    components:
      - name: DepartureDatePicker
        selector: '[data-picker="departure"]'
        memory:
          - Accepts typed YYYY-MM-DD — skips the calendar
```

Then point your agent at the directory. The [quickstart](https://docs.sightmap.org/start/quickstart)
walks the full loop, and the [`sightmap` CLI](go/) drives curation against a live
browser.

## Skills & plugin

The [`skills/`](skills/) directory is a first-class, installable skill set for
coding agents — `sightmap-authoring` (build and maintain a corpus) and
`sightmap-browser` (drive a live session). It reaches agents three ways, all
from the same source:

- **As a plugin** — install this repo like any other (Claude Code:
  `/plugin marketplace add sightmap/sightmap` then
  `/plugin install sightmap@sightmap-marketplace`). Useful on its own as a
  browser-use agent, no other tooling required.
- **Via the CLI** — `sightmap skills install` extracts the embedded copy into
  `~/.agents/skills/` (handy when you already have the binary).
- **Vendored by downstream tools** — they ship in the published
  [`@sightmap/sightmap`](https://www.npmjs.com/package/@sightmap/sightmap) npm
  package, so consumers like [Subtext](https://subtext.fullstory.com) pull them
  from a pinned version.

## Who reads it

- **Your coding agents** — Claude Code, Cursor, Codex, Windsurf, and anything else that reads repo files.
- **[Subtext](https://subtext.fullstory.com)** — runtime enrichment for live browser sessions and session replays; snapshots and network traces get semantic names, memory guides, and source paths injected automatically.

## Contributing

Sightmap is open source and stewarded by the [Subtext](https://subtext.fullstory.com)
team at Fullstory. We welcome contributions from anyone.

- Start with [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Proposing a spec change? See [`spec/seps/README.md`](spec/seps/README.md)
- Reporting security issues: [`SECURITY.md`](SECURITY.md)
- Who maintains this: [`MAINTAINERS.md`](MAINTAINERS.md)

## License

MIT — see [`LICENSE`](LICENSE).
